package aqua

import (
	"errors"
	"slices"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
)

// popupOptions describes a popup to open (Window.openPopup).
type popupOptions struct {
	// Config places the popup (shell.PopupConfig); Config.Grab makes it a
	// grab popup (a menu), without Grab it takes no input (a tooltip).
	Config shell.PopupConfig

	// Content is the popup's only element, framed to the whole popup on
	// every Placed. It must be detached (no parent, not a router's root);
	// it is detached again when the popup closes, so its owner can reuse
	// it for the next opening.
	Content Element

	// Placed, if set, is called after every placement, once the content
	// has its new frame: rect in the window's surface points (possibly
	// moved and shorter than Config.Size), scale in device pixels per
	// point. The first call may come from inside openPopup.
	Placed func(rect driver.Rect, scale float64)

	// Closed, if set, is called once when the popup has closed, for any
	// reason (Close, a dismissal, the window's key loss or close, Quit),
	// after the content was detached.
	Closed func()

	// TrackEvent, if set, gets a grab popup's pointer events over its
	// track (Config.Track, S5.6d) and outside its placed rect, in the
	// window's surface points (S5.6f: a menu strip's titles). Enter and
	// moves there reach the popup's router first (so its content sees the
	// pointer leave it), then TrackEvent; presses and releases there go to
	// TrackEvent only (a release is not a stray one ending the content's
	// press-drag-release); leaves and scrolls go to the router only. The
	// popup may be closed or repositioned from inside it.
	TrackEvent func(ev shell.PointerEvent)
}

// popupRoot is an open popup of a window (PLAN.md S5.5g): a shell.Popup
// plus an element tree of its own, with its own router and paint. The
// root covers the whole popup at its origin, so root coordinates are popup
// points; its only child is the content element.
//
// The tree belongs to the window: Base.Window of its elements returns the
// owning window, so they paint with its appearance, key state, theme and
// timeline. The window repaints the tree and sends it WindowStateEvent on
// every appearance change (stateChanged), and the router's active state
// follows the window's key state.
//
// A grab popup gets the window's input from the shell (shell.NewPopup):
// pointer events in its own points and the window's keys, which its router
// sends to its focus (or the root) as a window's router does. Its router
// also routes a release whose press it never saw (Router.SetStrayReleases):
// the press that opened the popup, released over it, or outside it, which
// the root passes on to the content with Over false (S5.5i). While it is
// open the window's own router has no capture and no hover (they are
// cancelled when it opens) and the window drops the pointer and scroll
// events the shell still sends it. A dismissal by the OS (Dismissed)
// closes the popup.
//
// A popup closes with its window: on the window's close (and so on Quit),
// and when the window stops being key (deactivation, hiding, minimizing),
// grab or not. A popupRoot is used on the UI thread only; after it has
// closed every method does nothing.
type popupRoot struct {
	Base
	w       *Window
	popup   shell.Popup // nil until PopupShell.NewPopup returns
	router  *Router
	content Element
	opts    popupOptions
	grab    bool

	rect   driver.Rect // the placed rect, in the window's surface points
	placed bool        // Placed was called
	scale  float64
	closed bool
}

// errNoPopups is openPopup's error on a shell without popups (no
// shell.PopupShell): menus and tooltips then do not open.
var errNoPopups = errors.New("aqua: the shell has no popups")

// openPopup opens a popup of w showing opts.Content (see popupRoot). It
// fails with shell.ErrPopupParent for a closed window (and whatever the
// shell reports: a hidden or minimized window, another grab popup), and
// with shell.ErrPopupGrab when the window already has a grab popup, and
// with errNoPopups on a shell that is no shell.PopupShell. On a
// failure the content stays detached and nothing else changes. It must
// not be called during a Paint; it panics for a nil or attached content.
func (w *Window) openPopup(opts popupOptions) (*popupRoot, error) {
	if opts.Content == nil {
		panic("aqua: openPopup without content")
	}
	if w.closed {
		return nil, shell.ErrPopupParent
	}
	ps, ok := w.sh.(shell.PopupShell)
	if !ok {
		return nil, errNoPopups
	}
	grab := opts.Config.Grab
	if grab && w.grab != nil {
		return nil, shell.ErrPopupGrab
	}
	if grab {
		w.tips.mute() // a menu hides the tooltip (S5.5l)
	}
	p := &popupRoot{w: w, content: opts.Content, opts: opts, grab: grab}
	p.Init(p)
	p.AddChild(opts.Content) // panics for an attached content or a router root
	p.router = NewRouter(p)
	p.router.SetActive(w.key)
	p.router.SetStrayReleases(grab)
	p.router.SetCursorFunc(p.setCursor)
	p.SetInvalidateFunc(p.invalidate)

	// The shell ends the window's pointer capture from inside NewPopup (a
	// PointerUp per held button, PointerLeave); the window drops those
	// events, its router's state is cancelled below instead.
	if grab {
		w.grabOpening = true
	}
	pp, err := ps.NewPopup(w.surface, opts.Config, popupHandler{p})
	w.grabOpening = false
	if err != nil {
		p.closed = true
		p.SetChildren()
		return nil, err
	}
	p.popup = pp
	if p.closed {
		// A handler closed it from inside NewPopup, before p.popup was
		// set: close the shell popup too, or a grab popup is orphaned
		// (B12). Its Closed finds p torn down already.
		pp.Close()
		return p, nil
	}
	w.popups = append(w.popups, p)
	if grab {
		w.grab = p
		w.router.CancelPointer()
	}
	pp.Invalidate()
	return p, nil
}

// Router returns the popup's input router.
func (p *popupRoot) Router() *Router { return p.router }

// Content returns the content element (nil after the close).
func (p *popupRoot) Content() Element {
	if p.closed {
		return nil
	}
	return p.content
}

// Popup returns the shell popup (nil when NewPopup failed).
func (p *popupRoot) Popup() shell.Popup { return p.popup }

// Grab reports whether the popup is a grab popup.
func (p *popupRoot) Grab() bool { return p.grab }

// Rect returns the rect of the last placement, in the window's surface
// points.
func (p *popupRoot) Rect() driver.Rect { return p.rect }

// Scale returns the scale of the last placement.
func (p *popupRoot) Scale() float64 { return p.scale }

// Closed reports whether the popup has closed.
func (p *popupRoot) Closed() bool { return p.closed }

// Translucent reports whether the OS blurs what is behind the popup, so
// its content paints a translucent fill (shell.Popup.Translucent); false
// after the close.
func (p *popupRoot) Translucent() bool {
	return !p.closed && p.popup != nil && p.popup.Translucent()
}

// Reposition places the popup again from cfg (cfg.Grab and cfg.Track are
// ignored: they stay the opening's). Placed follows when the rect or the
// scale changed.
func (p *popupRoot) Reposition(cfg shell.PopupConfig) {
	if !p.closed && p.popup != nil {
		cfg.Grab, cfg.Track = p.opts.Config.Grab, p.opts.Config.Track
		p.opts.Config = cfg
		p.popup.Reposition(cfg)
	}
}

// Close closes the popup; Closed follows (see popupOptions.Closed).
// Calling it again does nothing. It must not be called during a Paint.
func (p *popupRoot) Close() {
	if p.closed {
		return
	}
	if p.popup != nil {
		p.popup.Close() // calls popupHandler.Closed
	}
	p.teardown()
}

// teardown is the close on the aqua side, once: the popup leaves its
// window, the content is detached (so its capture, hover and focus end
// with PointerCancel, PointerLeave and FocusEvent{false}), then Closed.
func (p *popupRoot) teardown() {
	if p.closed {
		return
	}
	p.closed = true
	w := p.w
	w.popups = slices.DeleteFunc(w.popups, func(x *popupRoot) bool { return x == p })
	if w.grab == p {
		w.grab = nil
	}
	p.SetChildren()
	if p.opts.Closed != nil {
		p.opts.Closed()
	}
}

// invalidate is the root's invalidate callback.
func (p *popupRoot) invalidate() {
	if !p.closed && p.popup != nil {
		p.popup.Invalidate()
	}
}

// setCursor applies the router's pointer shape.
func (p *popupRoot) setCursor(c shell.Cursor) {
	if !p.closed && p.popup != nil {
		p.popup.SetCursor(c)
	}
}

// HandleEvent implements Element: a release outside the popup that the
// router sends the root (Router.SetStrayReleases: the opening press
// released off the popup) goes on to the content, with Over false, so a
// menu ends its press-drag-release (S5.5i).
func (p *popupRoot) HandleEvent(ev Event) bool {
	pe, ok := ev.(PointerEvent)
	if !ok || pe.Kind != PointerUp || pe.Over || p.closed {
		return false
	}
	pe.Pos = FromRoot(p.content, pe.Pos)
	return p.content.HandleEvent(pe)
}

// stateChanged repaints the tree and sends WindowStateEvent to every
// element of it, after a key or appearance change of the window.
func (p *popupRoot) stateChanged() {
	if p.closed {
		return
	}
	p.router.SetActive(p.w.key)
	broadcastState(p, p.router, p.w.state, func() bool { return p.closed || p.w.closed })
}

// popupHandler implements shell.PopupHandler for a popupRoot.
type popupHandler struct{ p *popupRoot }

var _ shell.PopupHandler = popupHandler{}

func (h popupHandler) Paint(c render.Canvas) {
	if !h.p.closed {
		PaintTree(c, h.p)
	}
}

func (h popupHandler) Placed(rect driver.Rect, scale float64) {
	p := h.p
	if p.closed {
		return
	}
	p.rect, p.scale, p.placed = rect, scale, true
	b := driver.Rect{W: rect.W, H: rect.H}
	p.SetFrame(b)
	p.content.SetFrame(b)
	p.router.RefreshHover()
	if p.opts.Placed != nil && !p.closed {
		p.opts.Placed(rect, scale)
	}
}

func (h popupHandler) Event(ev shell.Event) bool {
	p := h.p
	if p.closed || p.w.closed {
		return false
	}
	if handled, done := p.w.menuAccess(ev, p); done || p.closed || p.w.closed {
		return handled // the menu strip's keyboard access (S5.6i)
	}
	if pe, ok := ev.(shell.PointerEvent); ok && p.onTrack(pe.Pos) {

		q := pe
		q.Pos = pe.Pos.Add(p.origin())
		switch pe.Kind {
		case shell.PointerDown, shell.PointerUp:
			p.opts.TrackEvent(q)
			return true
		case shell.PointerEnter, shell.PointerMove:
			p.router.Event(ev)
			if !p.closed && !p.w.closed {
				p.opts.TrackEvent(q)
			}
			return true
		}
	}
	return p.router.Event(ev)
}

// origin returns the popup's top-left in the window's surface points: the
// placed rect's, or before the first placement the one asked for.
func (p *popupRoot) origin() driver.Point {
	if p.placed {
		return p.rect.Origin()
	}
	c := p.opts.Config
	return c.Anchor.Origin().Add(c.Offset)
}

// onTrack reports whether pos, in the popup's points, is over the track of
// a grab popup with a TrackEvent and outside the popup's placed rect.
func (p *popupRoot) onTrack(pos driver.Point) bool {
	if !p.grab || p.opts.TrackEvent == nil || p.opts.Config.Track.Empty() {
		return false
	}
	if p.placed && driver.RectOf(driver.Point{}, p.rect.Size()).Contains(pos) {
		return false
	}
	return p.opts.Config.Track.Contains(pos.Add(p.origin()))
}

func (h popupHandler) Dismissed() { h.p.Close() }

func (h popupHandler) Closed() { h.p.teardown() }

// closePopups closes the window's popups, last to first.
func (w *Window) closePopups() {
	for len(w.popups) > 0 {
		w.popups[len(w.popups)-1].Close()
	}
}

// GrabPopupOpen reports whether a grab popup (a menu) of the window is
// open.
func (w *Window) GrabPopupOpen() bool { return w.grab != nil }

// GrabPopupRoot returns the root element of the window's open grab popup
// (a menu), in the popup's coordinates, or nil: for tests above the
// driver, which find a menu's items in its accessibility tree.
func (w *Window) GrabPopupRoot() Element {
	if w.grab == nil {
		return nil
	}
	return w.grab
}
