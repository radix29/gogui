package gogui

import (
	"fmt"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/layout"
)

// WindowStyle is a set of window features for WindowConfig.Style, such as
// Titled|Closable|Resizable. The bit values match NSWindowStyleMask.
//
// Without Titled a window has no title bar, and on Windows and Linux no
// window buttons and no menu strip (the menu shortcuts still work in it).
// Closable, Miniaturizable and Resizable enable the close, minimize and
// zoom buttons; without Resizable the user can't resize the window.
type WindowStyle = driver.WindowStyle

// Window styles.
const (
	Titled         = driver.StyleTitled
	Closable       = driver.StyleClosable
	Miniaturizable = driver.StyleMiniaturizable
	Resizable      = driver.StyleResizable
)

// WindowConfig configures a new window: its Title, its initial content
// Size (the area below the title bar), the MinSize and MaxSize limits of
// the content size (zero components mean no limit), its Style, and
// whether it is centered on the screen when first shown (Center). The
// window never gets smaller than its content's minimum size, whatever
// MinSize says.
//
// On Linux the window manager places every window, so Center does
// nothing there, and GTK 4 has no maximum-size hint: MaxSize only limits
// SetContentSize, not the user's resizing.
type WindowConfig = driver.WindowConfig

// Window is a top-level window (NSWindow). It is created hidden; Show
// shows it. Its content is laid out to fill the content area, again
// whenever the window is resized or a widget in it changes size.
//
// Closing the window destroys its widget tree (whose widgets then panic
// when used, D13) and drops its content and handlers. After that, the
// window's setters only record their values, Content is nil, and the
// other methods do nothing.
//
// On Windows and Linux the toolkit draws the whole window: the title bar
// with its title, the window buttons (close, minimize and zoom, which
// maximizes) and the menu strip, inside a native window without the OS
// title bar (the OS still gives it its shadow). On the headless driver
// (macOS until the AppKit driver exists) a window is never shown, but its
// content is laid out at its content size.
type Window struct {
	drv     driver.Driver
	p       driver.Window
	cfg     WindowConfig
	title   string
	content Widget
	root    *layout.Root // nil without content
	size    Size         // the content size at the last pass or at close
	minSize Size         // the min size last given to the peer
	pending bool         // a deferred layout pass is queued
	closed  bool
	passes  int // layout passes run, for the tests

	onClose       subscribers[func()]
	onShouldClose subscribers[func() bool]
}

// NewWindow creates a hidden window. Show shows it. Like every
// constructor it panics outside Run (before the ready callback or after
// Run has returned); it also panics if the platform can't create the
// window, which means the native layer is broken.
func (a *App) NewWindow(cfg WindowConfig) *Window {
	driverFor("App.NewWindow") // panics outside Run, like every constructor (A6)
	d := a.drv
	w := &Window{drv: d, cfg: cfg, title: cfg.Title, minSize: cfg.MinSize}
	p, err := d.NewWindow(cfg, windowEvents{w})
	if err != nil {
		panic(fmt.Errorf("gogui: NewWindow: %w", err))
	}
	w.p, w.size = p, p.ContentSize()
	return w
}

// SetTitle sets the title shown in the title bar. On Windows and Linux the
// title is centred in the title bar and truncated, or hidden, where the
// menu strip needs the room; the OS's own title (taskbar, Alt-Tab) is
// always the whole title.
func (w *Window) SetTitle(title string) *Window {
	checkUI("Window.SetTitle")
	w.title = title
	if !w.closed {
		w.p.SetTitle(title)
	}
	return w
}

// Title returns the title.
func (w *Window) Title() string { checkUI("Window.Title"); return w.title }

// SetContent sets the widget that fills the window's content area; nil
// removes it. A widget that is in a stack, a form or another window is
// moved here. The old content is taken out of the window but not
// destroyed, so it can be shown again; Destroy frees it (D13). It does
// nothing on a closed window, and panics with a destroyed widget.
func (w *Window) SetContent(c Widget) *Window {
	checkUI("Window.SetContent")
	if c != nil && c.base().dead {
		panic("gogui: Window.SetContent with a destroyed widget")
	}
	if w.closed || c == w.content {
		return w
	}
	if c != nil {
		detach(c)
	}
	if old := w.content; old != nil {
		old.base().host = nil
	}
	w.content, w.root = c, nil
	var peer driver.Widget
	if c != nil {
		c.base().host = w
		w.root = layout.NewRoot(c.node())
		peer = c.peerWidget()
	}
	w.p.SetContent(peer)
	w.invalidate()
	return w
}

// Content returns the content widget, or nil (always nil once the window
// has closed).
func (w *Window) Content() Widget { checkUI("Window.Content"); return w.content }

// detach takes w out of its stack, form or window, without destroying it.
func detach(w Widget) {
	b := w.base()
	if old, ok := b.parent.(parentWidget); ok {
		old.removeChild(w)
	}
	b.parent = nil
	if win, ok := b.host.(*Window); ok {
		win.SetContent(nil)
	}
}

// ContentSize returns the size of the content area, in points.
func (w *Window) ContentSize() Size {
	checkUI("Window.ContentSize")
	if w.closed {
		return w.size
	}
	return w.p.ContentSize()
}

// SetContentSize resizes the content area, within the window's minimum and
// maximum size, and lays the content out at the new size.
func (w *Window) SetContentSize(size Size) *Window {
	checkUI("Window.SetContentSize")
	if !w.closed {
		w.p.SetContentSize(size)
	}
	return w
}

// Show lays out the content if it has changed, then shows the window,
// brings it to the front and makes it the key window.
func (w *Window) Show() {
	checkUI("Window.Show")
	if w.closed {
		return
	}
	if w.pending {
		w.layout()
	}
	w.p.Show()
}

// Close closes the window without asking OnShouldClose, calls the OnClose
// handlers and destroys the widget tree. Closing a closed window does
// nothing.
func (w *Window) Close() {
	checkUI("Window.Close")
	if !w.closed {
		w.p.Close() // calls windowEvents.Closed
	}
}

// OnClose calls fn when the window has closed, whether by the user, by
// Close or by App.Quit. The widgets are still alive while fn runs.
func (w *Window) OnClose(fn func()) Unsubscribe {
	checkUI("Window.OnClose")
	if w.closed {
		return func() {} // it has closed: fn would never run (A9)
	}
	return w.onClose.add(fn)
}

// OnShouldClose calls fn when the user asks to close the window (the close
// button, ⌘W / Ctrl+W through the Window menu, and on Windows and Linux
// also Alt+F4 and the taskbar's or the window manager's close command; on
// Windows logging out too). If fn returns false the window stays open, and the
// handlers subscribed after it are not called.
func (w *Window) OnShouldClose(fn func() bool) Unsubscribe {
	checkUI("Window.OnShouldClose")
	if w.closed {
		return func() {}
	}
	return w.onShouldClose.add(fn)
}

// invalidate queues one layout pass for the next turn of the UI loop. More
// calls before it runs add nothing.
func (w *Window) invalidate() {
	if w.closed || w.pending {
		return
	}
	w.pending = true
	w.drv.Queue().Do(func() {
		if w.pending {
			w.layout()
		}
	})
}

// layout runs a layout pass at the current content size. Then it sets the
// window's minimum size to max(cfg.MinSize, the content's minimum), when
// that has changed.
func (w *Window) layout() {
	w.pending = false
	if w.closed {
		return
	}
	w.passes++
	if a := cur.Load(); a != nil && a.frames != nil {
		defer a.frames.addLayout(a.frames.now())
	}
	w.size = w.p.ContentSize()
	min := w.cfg.MinSize
	if w.root != nil {
		min = min.Max(w.root.LayoutMin(w.size))
	}
	if min == w.minSize {
		return
	}
	w.minSize = min
	w.p.SetMinSize(min)
}

// closedByPeer runs when the peer has closed: the OnClose handlers, then the
// destruction of the tree, children first.
func (w *Window) closedByPeer() {
	if w.closed {
		return
	}
	w.closed, w.pending = true, false
	w.onClose.each(func(fn func()) { fn() })
	if c := w.content; c != nil {
		c.base().host = nil
		destroyTree(c)
	}
	// Drop what the closed window no longer needs (A9), so a window the
	// app keeps holds no widgets or handlers alive.
	w.content, w.root = nil, nil
	w.onClose, w.onShouldClose = subscribers[func()]{}, subscribers[func() bool]{}
}

// destroyTree destroys w's subtree bottom-up, as driver.Widget.Destroy
// requires.
func destroyTree(w Widget) {
	if p, ok := w.(parentWidget); ok {
		for _, c := range p.childWidgets() {
			destroyTree(c)
		}
	}
	w.base().destroy()
}

// windowEvents implements driver.WindowHandler for a Window, so the
// handler methods stay out of the public API.
type windowEvents struct{ w *Window }

// ContentResized lays out at once, so the frame being drawn is right.
func (e windowEvents) ContentResized(driver.Size) { e.w.layout() }

func (e windowEvents) ShouldClose() bool {
	ok := true
	e.w.onShouldClose.each(func(fn func() bool) {
		if ok {
			ok = fn()
		}
	})
	return ok
}

func (e windowEvents) Closed()         { e.w.closedByPeer() }
func (e windowEvents) KeyChanged(bool) {}
