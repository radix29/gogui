// Package gogui is a cross-platform desktop GUI toolkit with a macOS look
// and feel (PLAN.md). Apps import only this package; the same code builds on
// macOS, Windows and Linux without build tags or cgo.
//
// A program calls Run from main. Run starts the UI event loop on the main OS
// thread and calls the ready callback, which builds the UI. Every callback
// runs on the UI thread. Other goroutines touch widgets only through Do or
// DoSync. With GOGUI_DEBUG=1 set, using the toolkit off the UI thread
// panics.
//
// Setters return their receiver, so calls chain. The setters every widget
// shares (SetEnabled, SetHidden, SetTooltip, SetControlSize and Focus) are
// defined once on an embedded generic type, so go doc shows them returning
// T: T is always the widget's own type (*Button for a Button).
package gogui

import (
	"errors"
	"runtime"
	"sync/atomic"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/mainthread"
)

func init() {
	// The native event loops (AppKit in particular) must run on the main
	// OS thread. init runs on the main goroutine, so locking here keeps
	// main, and therefore Run, on that thread.
	runtime.LockOSThread()
}

// AppConfig configures the application. Name is the application name: the
// title of the app menu and part of the default menu titles ("Quit Name").
type AppConfig = driver.AppConfig

// errRunTwice is returned by Run when it is called a second time.
var errRunTwice = errors.New("gogui: Run called more than once")

// ran is set by the first Run; the tests reset it.
var ran atomic.Bool

// cur is the running application, nil before Run and after it returns.
var cur atomic.Pointer[App]

// App is the running application. Run passes it to the ready callback.
type App struct {
	drv uiDriver

	appearance  Appearance     // the effective appearance last reported
	lastMenuID  int            // the last menu item ID handed out
	menuActions map[int]func() // the actions of the current menu bar

	onAppearance subscribers[func(Appearance)]
	onQuit       subscribers[func() bool]

	// live holds every public widget that has not been destroyed, so Run can
	// destroy the ones that were never placed in a window (or were taken
	// out of one) when it returns.
	live map[*widget]Widget
}

// Run starts the application and blocks until Quit. It must be called once,
// from main. When the event loop is running, Run calls ready on the UI
// thread with the App; ready builds the UI and may be nil.
//
// Run returns nil after Quit. It returns the driver's error if the platform
// backend cannot start (for example, when GTK 4 is missing on Linux), and
// an error if it is called a second time. It never calls os.Exit.
func Run(cfg AppConfig, ready func(*App)) error {
	if !ran.CompareAndSwap(false, true) {
		return errRunTwice
	}
	d, err := selectDriver()
	if err != nil {
		return err
	}
	app := &App{drv: d, live: map[*widget]Widget{}}
	cur.Store(app)
	defer cur.Store(nil)
	err = d.Run(cfg, &appHandler{app: app, ready: ready})
	app.destroyLeftovers()
	return err
}

// Quit closes every window and makes Run return once the current event
// has been handled. It must be called on the UI thread.
func (a *App) Quit() {
	checkUI("App.Quit")
	a.drv.Quit()
}

// SetAppearance forces the light or dark appearance, or follows the OS
// again (AppearanceSystem). If the effective appearance changes, the
// OnAppearanceChange handlers are called.
func (a *App) SetAppearance(ap Appearance) *App {
	checkUI("App.SetAppearance")
	a.drv.SetAppearance(ap.mode())
	return a
}

// Appearance returns the effective appearance: AppearanceLight or
// AppearanceDark, never AppearanceSystem.
func (a *App) Appearance() Appearance {
	checkUI("App.Appearance")
	return effectiveAppearance(a.drv.Appearance())
}

// OnAppearanceChange calls fn with the new effective appearance when it
// switches between light and dark, because the OS setting changed or
// because of SetAppearance. Semantic colors and system fonts follow on
// their own; fn is for anything the app picks or draws itself.
func (a *App) OnAppearanceChange(fn func(Appearance)) Unsubscribe {
	checkUI("App.OnAppearanceChange")
	return a.onAppearance.add(fn)
}

// OnQuitRequested calls fn when the user asks to quit: the Quit menu item,
// ⌘Q (Ctrl+Q), the Dock menu or the OS logging out. If fn returns false the
// app keeps running, and the handlers subscribed after it are not called.
// App.Quit doesn't ask.
func (a *App) OnQuitRequested(fn func() bool) Unsubscribe {
	checkUI("App.OnQuitRequested")
	return a.onQuit.add(fn)
}

// Do queues fn to run on the UI thread and returns at once. Functions
// queued with Do run in order. Do may be called from any goroutine. Before
// Run has started and after it has returned, fn is dropped; so is fn
// queued after Quit.
func Do(fn func()) {
	if q := queue(); q != nil {
		q.Do(fn)
	}
}

// DoSync runs fn on the UI thread and waits until it has returned. Called
// on the UI thread itself, it runs fn at once. A panic in fn is re-raised in
// the caller.
//
// DoSync reports whether fn ran. It returns false, without running fn,
// before Run has started, after it has returned, and from other goroutines
// after Quit.
func DoSync(fn func()) bool {
	if q := queue(); q != nil {
		return q.DoSync(fn)
	}
	return false
}

// queue returns the UI-thread queue of the running app, or nil.
func queue() *mainthread.Queue {
	if a := cur.Load(); a != nil {
		return a.drv.Queue()
	}
	return nil
}

// checkUI panics if UI-thread checks are on and the caller is not on the UI
// thread. what names the public method, for example "Button.SetTitle".
// Outside Run it does nothing; with checks off it costs one atomic load.
func checkUI(what string) {
	if q := queue(); q != nil {
		q.Check(what)
	}
}

// appHandler implements driver.AppHandler for an App, so the handler
// methods stay out of the public API.
type appHandler struct {
	app   *App
	ready func(*App)
}

func (h *appHandler) Ready() {
	h.app.drv.Queue().SetChecks(debugChecks)
	h.app.appearance = effectiveAppearance(h.app.drv.Appearance())
	if h.ready != nil {
		h.ready(h.app)
	}
}

// AppearanceChanged fires OnAppearanceChange when light and dark switch.
// Other changes (the accent color, accessibility options) are handled by
// the drivers.
func (h *appHandler) AppearanceChanged(ap driver.Appearance) {
	a := h.app
	next := effectiveAppearance(ap)
	if next == a.appearance {
		return
	}
	a.appearance = next
	a.onAppearance.each(func(fn func(Appearance)) { fn(next) })
}

// QuitRequested asks the OnQuitRequested handlers in order; the first false
// cancels.
func (h *appHandler) QuitRequested() bool {
	ok := true
	h.app.onQuit.each(func(fn func() bool) {
		if ok {
			ok = fn()
		}
	})
	return ok
}

func (h *appHandler) MenuItemSelected(id int) { h.app.menuItemSelected(id) }

// destroyLeftovers destroys, bottom-up, every widget tree that is still
// alive when the event loop has ended: widgets that were never placed in a
// window, or were taken out of one and not placed again. Closed windows
// have already destroyed theirs.
func (a *App) destroyLeftovers() {
	for _, w := range a.live {
		if b := w.base(); !b.dead && b.parent == nil {
			destroyTree(w)
		}
	}
	a.live = nil
}
