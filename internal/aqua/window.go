package aqua

import (
	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
)

// TitleBarHeight returns the height of the title bar the aqua driver draws
// at the top of a titled window's surface (PLAN.md §5.4.1): the theme's
// Chrome.TitleBarHeight token.
func TitleBarHeight() float64 { return tokens.Chrome.TitleBarHeight }

// Window is an aqua window (PLAN.md S2.2c): a shell.Surface plus a tree of
// elements. The root covers the whole surface and holds the content area
// and, for a titled window, the title bar on top of it with the traffic
// lights and the centered title.
//
// Window implements driver.Window in content coordinates: ContentSize,
// SetContentSize, SetMinSize, SetMaxSize and the ContentResized it reports
// are the surface's minus the title bar (an untitled window has no title
// bar, so both are the same). Its surface handler (an unexported adapter,
// so the handler methods stay out of Window's method set) paints the tree,
// routes input through the window's Router, follows resizes and the key
// state, and asks driver.WindowHandler.ShouldClose before an OS close.
//
// A Window is used on the UI thread only. After it has closed every method
// does nothing; the getters keep returning the last state.
type Window struct {
	sh      shell.Shell
	surface shell.Surface
	h       driver.WindowHandler
	style   driver.WindowStyle
	titled  bool

	title      string
	appearance driver.Appearance
	key        bool
	closed     bool
	shown      bool // Show was called (S3.4f)

	min, max driver.Size // content limits, as set

	root    *windowRoot
	content *contentArea
	bar     *titleBar // nil for an untitled window
	router  *Router

	timeline *anim.Timeline // the driver's; nil outside a Driver

	resizing bool // inside SetContentSize: Resized reported ContentResized
	reported bool
}

var _ driver.Window = (*Window)(nil)

// NewWindow creates a hidden aqua window on sh, which must be running
// (after shell.Handler.Ready). The window starts with sh's appearance;
// the driver passes later changes with SetAppearance. h must be non-nil.
func NewWindow(sh shell.Shell, cfg driver.WindowConfig, h driver.WindowHandler) (*Window, error) {
	if h == nil {
		panic("aqua: NewWindow with a nil handler")
	}
	w := &Window{
		sh:         sh,
		h:          h,
		style:      cfg.Style,
		titled:     cfg.Style&driver.StyleTitled != 0,
		title:      cfg.Title,
		appearance: sh.Appearance(),
		min:        cfg.MinSize,
		max:        cfg.MaxSize,
	}
	sf, err := sh.NewSurface(shell.SurfaceConfig{
		Title:   cfg.Title,
		Size:    w.toSurface(cfg.Size),
		MinSize: w.surfaceLimit(cfg.MinSize, true),
		MaxSize: w.surfaceLimit(cfg.MaxSize, false),
		Style:   cfg.Style,
		Center:  cfg.Center,
	}, surfaceHandler{w})
	if err != nil {
		return nil, err
	}
	w.surface = sf

	w.root = &windowRoot{w: w}
	w.root.Init(w.root)
	w.content = &contentArea{w: w}
	w.content.Init(w.content)
	w.content.SetClipsChildren(true)
	w.root.AddChild(w.content)
	if w.titled {
		w.bar = newTitleBar(w)
		w.root.AddChild(w.bar)
	}
	w.router = NewRouter(w.root)
	w.router.SetActive(false) // not key until the shell says so
	w.root.SetInvalidateFunc(sf.Invalidate)
	w.layout(sf.Size())
	return w, nil
}

// titleBarHeight returns the height of the title bar: TitleBarHeight() for a
// titled window, 0 otherwise.
func (w *Window) titleBarHeight() float64 {
	if w.titled {
		return TitleBarHeight()
	}
	return 0
}

// toSurface converts a content size into a surface size.
func (w *Window) toSurface(s driver.Size) driver.Size {
	return driver.Size{W: s.W, H: s.H + w.titleBarHeight()}
}

// toContent converts a surface size into a content size (never negative).
func (w *Window) toContent(s driver.Size) driver.Size {
	return driver.Size{W: s.W, H: max(0, s.H-w.titleBarHeight())}
}

// surfaceLimit converts a content limit into a surface limit. Zero
// components mean "no limit" on both sides, except that a titled surface is
// never smaller than its title bar.
func (w *Window) surfaceLimit(s driver.Size, isMin bool) driver.Size {
	out := driver.Size{W: s.W}
	switch {
	case s.H > 0:
		out.H = s.H + w.titleBarHeight()
	case isMin:
		out.H = w.titleBarHeight()
	}
	return out
}

// layout places the root, the content area and the chrome for the surface
// size s, and recomputes the chrome regions.
func (w *Window) layout(s driver.Size) {
	tb := w.titleBarHeight()
	w.root.SetFrame(driver.Rect{W: s.W, H: s.H})
	w.content.SetFrame(driver.Rect{Y: tb, W: s.W, H: max(0, s.H-tb)})
	if w.bar == nil {
		return
	}
	w.bar.SetFrame(driver.Rect{W: s.W, H: tb})
	w.bar.layout()
	w.surface.SetChromeRegions(w.bar.regions())
}

// Root returns the root element (the whole surface).
func (w *Window) Root() Element { return w.root }

// Content returns the content element set with SetContent, or nil.
func (w *Window) Content() Element {
	if w.content.ChildCount() == 0 {
		return nil
	}
	return w.content.Child(0)
}

// Timeline returns the timeline the window's elements animate on: the
// driver's (Driver.Timeline) for a window made by Driver.NewWindow, nil for
// one made with NewWindow directly.
func (w *Window) Timeline() *anim.Timeline { return w.timeline }

// Router returns the window's input router.
func (w *Window) Router() *Router { return w.router }

// Surface returns the window's surface.
func (w *Window) Surface() shell.Surface { return w.surface }

// Title returns the title.
func (w *Window) Title() string { return w.title }

// IsKey reports whether the window is the key window.
func (w *Window) IsKey() bool { return w.key }

// Closed reports whether the window has closed.
func (w *Window) Closed() bool { return w.closed }

// Shown reports whether the window is shown: Show was called and it has
// not closed (S3.4f: a looping animation runs only in a shown window). A
// minimized window still counts as shown; the shell does not report it.
func (w *Window) Shown() bool { return w.shown && !w.closed }

// Appearance returns the appearance the window paints with.
func (w *Window) Appearance() driver.Appearance { return w.appearance }

// SetAppearance sets the effective appearance (the driver's merge of the OS
// setting and SetAppearance). On a change the whole window repaints and
// every element gets a WindowStateEvent.
func (w *Window) SetAppearance(a driver.Appearance) {
	if w.closed || a == w.appearance {
		return
	}
	w.appearance = a
	w.stateChanged()
}

// invalidateAll invalidates every element of the subtree, so elements that
// cache appearance-dependent state repaint too.
func invalidateAll(e Element) {
	b := e.base()
	b.Invalidate()
	for _, c := range b.children {
		invalidateAll(c)
	}
}

// SetTitle implements driver.Window: the native title and the drawn one.
func (w *Window) SetTitle(title string) {
	if w.closed || title == w.title {
		return
	}
	w.title = title
	w.surface.SetTitle(title)
	if w.bar != nil {
		w.bar.layout()
	}
}

// SetContent implements driver.Window. The element of the widget (an aqua
// Element, or a placeholder peer's) becomes the content area's only child
// (moved from its old parent); its frame, set by the public layer, is
// relative to the content area. Any other widget is not drawn, and nil
// removes the content. A focus inside the old content is dropped (the old
// element gets FocusEvent{false}); a focus inside the new one is kept, and
// nothing else is focused until the next Show or Tab.
func (w *Window) SetContent(c driver.Widget) {
	if w.closed {
		return
	}
	if e := ElementOf(c); e != nil {
		w.content.SetChildren(e)
	} else {
		w.content.SetChildren()
	}
	w.router.RefreshHover()
}

// ContentSize implements driver.Window.
func (w *Window) ContentSize() driver.Size { return w.toContent(w.surface.Size()) }

// SetContentSize implements driver.Window: the surface is resized to size
// plus the title bar, clamped to the limits, and ContentResized is called
// once, even when the size did not change.
func (w *Window) SetContentSize(size driver.Size) {
	if w.closed {
		return
	}
	w.resizing, w.reported = true, false
	w.surface.SetSize(w.toSurface(size))
	w.resizing = false
	if !w.reported && !w.closed {
		w.h.ContentResized(w.ContentSize())
	}
}

// SetMinSize implements driver.Window (a content size).
func (w *Window) SetMinSize(size driver.Size) {
	if w.closed {
		return
	}
	w.min = size
	w.surface.SetMinSize(w.surfaceLimit(size, true))
}

// SetMaxSize implements driver.Window (a content size).
func (w *Window) SetMaxSize(size driver.Size) {
	if w.closed {
		return
	}
	w.max = size
	w.surface.SetMaxSize(w.surfaceLimit(size, false))
}

// MinSize and MaxSize return the content limits set last.
func (w *Window) MinSize() driver.Size { return w.min }
func (w *Window) MaxSize() driver.Size { return w.max }

// Center implements driver.Window.
func (w *Window) Center() {
	if !w.closed {
		w.surface.Center()
	}
}

// Show implements driver.Window. When nothing is focused it first focuses
// the first element of the Tab order (Router.FocusOrder), so a focus set
// before Show (Widget.Focus after SetContent) wins and a window shown again
// keeps its focus.
func (w *Window) Show() {
	if w.closed {
		return
	}
	if w.router.Focused() == nil {
		w.router.FocusNext(false)
	}
	w.shown = true
	w.surface.Show()
}

// Close implements driver.Window: the surface closes without ShouldClose,
// and Closed is called.
func (w *Window) Close() {
	if !w.closed {
		w.surface.Close()
	}
}

// performClose is the red traffic light: ask ShouldClose, then close.
func (w *Window) performClose() {
	if !w.closed && w.h.ShouldClose() && !w.closed {
		w.surface.Close()
	}
}

// surfaceHandler implements shell.SurfaceHandler for a Window.
type surfaceHandler struct{ w *Window }

var _ shell.SurfaceHandler = surfaceHandler{}

func (s surfaceHandler) Paint(c render.Canvas) { PaintTree(c, s.w.root) }

func (s surfaceHandler) Resized(size driver.Size, _ float64) {
	w := s.w
	if w.closed {
		return
	}
	old := w.content.Frame().Size()
	w.layout(size)
	w.router.RefreshHover()
	if cs := w.toContent(size); cs != old || w.resizing {
		w.reported = true
		w.h.ContentResized(cs)
	}
}

// CloseRequested asks ShouldClose. A window without StyleClosable refuses
// OS close requests, as AppKit's performClose does.
func (s surfaceHandler) CloseRequested() bool {
	w := s.w
	return !w.closed && w.style&driver.StyleClosable != 0 && w.h.ShouldClose()
}

func (s surfaceHandler) Closed() {
	w := s.w
	if w.closed {
		return
	}
	w.closed, w.key = true, false
	w.router.SetActive(false)
	w.h.Closed()
}

func (s surfaceHandler) KeyChanged(isKey bool) {
	w := s.w
	if w.closed || isKey == w.key {
		return
	}
	w.key = isKey
	w.router.SetActive(isKey)
	w.stateChanged()
	if !w.closed {
		w.h.KeyChanged(isKey)
	}
}

func (s surfaceHandler) Event(ev shell.Event) bool {
	w := s.w
	if w.closed {
		return false
	}
	if w.router.Event(ev) {
		return true
	}
	if k, ok := ev.(shell.KeyEvent); ok && !w.closed {
		return w.keyEquivalent(k) // default and cancel buttons (buttons.go)
	}
	return false
}

// windowRoot is the root element of a window: the whole surface. It is
// exposed as the window node of the accessibility tree, with StateFocused
// while the window is key; the focused element inside keeps its own
// StateFocused while the window is inactive.
type windowRoot struct {
	Base
	w *Window
}

func (r *windowRoot) AccessibleNode() a11y.Node {
	n := a11y.Node{Role: a11y.RoleWindow, Label: r.w.title}
	if r.w.key {
		n.States |= a11y.StateFocused
	}
	return n
}

// contentArea is the part of the surface below the title bar. It paints the
// window background and clips the content element.
type contentArea struct {
	Base
	w *Window
}

func (a *contentArea) Paint(c render.Canvas) {
	c.FillRoundedRect(a.Bounds(), render.Radii{},
		render.Solid(semantic(driver.ColorWindowBackground, a.w.appearance)))
}
