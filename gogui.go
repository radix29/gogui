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
// title of the app menu and part of the standard menu titles ("About
// Name", "Hide Name", "Quit Name"), and on Windows and Linux the title of
// the app-modal alerts.
type AppConfig = driver.AppConfig

// errRunTwice is returned by Run when it is called a second time.
var errRunTwice = errors.New("gogui: Run called more than once")

// ran is set by the first Run; the tests reset it.
var ran atomic.Bool

// cur is the running application, nil before Run and after it returns.
var cur atomic.Pointer[App]

// App is the running application. Run passes it to the ready callback.
type App struct {
	drv driver.Driver

	appearance  Appearance     // the effective appearance last reported
	lastMenuID  int            // the last menu item ID handed out
	menuActions map[int]func() // the actions of the current menu bar

	onAppearance subscribers[func(Appearance)]
	onQuit       subscribers[func() bool]

	// live holds every public widget that has not been destroyed, so Run can
	// destroy the ones that were never placed in a window (or were taken
	// out of one) when it returns.
	live map[*widget]Widget

	// frames is the debug frame-time log (framelog.go), nil when off.
	frames *frameLog
}

// Run starts the application and blocks until Quit. It must be called once,
// from main. When the event loop is running, Run calls ready on the UI
// thread with the App; ready builds the UI and may be nil.
//
// Run returns nil after Quit. It returns the driver's error if the platform
// backend cannot start (for example, when GTK 4 is missing on Linux), and
// an error if it is called a second time. It never calls os.Exit.
//
// On macOS, until the AppKit driver exists, Run uses the headless driver:
// it prints one notice line to stderr and shows nothing (see the package
// documentation, Platforms).
func Run(cfg AppConfig, ready func(*App)) error {
	if !ran.CompareAndSwap(false, true) {
		return errRunTwice
	}
	d, err := newDriver()
	if err != nil {
		return err
	}
	app := &App{drv: d, live: map[*widget]Widget{}}
	defer cur.Store(nil) // Ready stores app (A6)
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

// SetAccentColor makes c the app's accent color instead of the OS one:
// the default button, checked checkboxes and radios, the slider's track,
// the focus ring, the selection, highlighted menu items and
// ColorControlAccent all follow it. A semantic c (ColorSystemPurple …)
// resolves against the current appearance and follows light and dark; an
// accent is always opaque. ColorControlAccent or the zero Color removes
// the override, so the OS accent applies again. On a light accent the
// labels drawn on it turn dark to stay readable.
//
// On macOS the accent is a user setting with no app-wide override: the
// color is recorded for the controls that take a tint (the headless
// driver that stands in for AppKit for now applies only a concrete c).
// OnAppearanceChange is not called.
func (a *App) SetAccentColor(c Color) *App {
	checkUI("App.SetAccentColor")
	if c == ColorControlAccent {
		c = Color{}
	}
	a.drv.SetAccentColor(c)
	return a
}

// AccentColor returns the effective accent color, always concrete: the
// override of SetAccentColor, or else the OS accent (macOS blue where the
// OS has none).
func (a *App) AccentColor() Color {
	checkUI("App.AccentColor")
	return a.drv.Appearance().Accent
}

// OnAppearanceChange calls fn with the new effective appearance when it
// switches between light and dark, because the OS setting changed or
// because of SetAppearance. Semantic colors and system fonts follow on
// their own; fn is for anything the app picks or draws itself. A change of
// the accent color (the OS one or SetAccentColor) does not call fn.
func (a *App) OnAppearanceChange(fn func(Appearance)) Unsubscribe {
	checkUI("App.OnAppearanceChange")
	return a.onAppearance.add(fn)
}

// OnQuitRequested calls fn when the user asks to quit: the Quit menu item
// or ⌘Q (Ctrl+Q), and on macOS also the Dock menu and logging out. If fn
// returns false the app keeps running, and the handlers subscribed after
// it are not called. App.Quit doesn't ask. On Windows, logging out asks
// each window's OnShouldClose handlers instead.
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
// on the UI thread itself, it runs fn at once. A panic in fn is re-raised
// in the caller as a *PanicError, on every thread (A7); its Value is the
// original panic value and its Stack the stack where fn panicked.
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

// PanicError is the panic value DoSync re-raises when fn panicked. It
// unwraps to the original value when that is an error.
type PanicError = mainthread.PanicError

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
	// The app becomes current only now, so constructors and Do work from
	// the ready callback on, as driverFor documents, and not while the
	// driver is still starting.
	cur.Store(h.app)
	h.app.drv.Queue().SetChecks(debugChecks)
	h.app.appearance = effectiveAppearance(h.app.drv.Appearance())
	startFrameLog(h.app)
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
// have already destroyed theirs. Each live widget's tree is destroyed from
// its topmost live ancestor, so no widget is skipped for having a parent
// (B3). Deleting from the map while ranging over it is allowed.
func (a *App) destroyLeftovers() {
	for _, w := range a.live {
		top := w.base()
		for top.parent != nil && !top.parent.base().dead {
			top = top.parent.base()
		}
		if !top.dead {
			top.host = nil
			destroyTree(top.self)
		}
	}
	a.live = nil
}
