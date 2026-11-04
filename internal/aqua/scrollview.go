package aqua

import (
	"math"
	"time"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/aqua/scroll"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
)

// ScrollView is the aqua driver.ScrollView (PLAN.md §5.6, S3.5d–S3.5f).
// The embedded stub scroll view keeps the state (document, document size,
// scrollers, viewport inset, the reported offset) and the User…
// simulations; two scroll.Axis drive the offset on the window's timeline.
//
//   - Elements: the scroll view's element (scrollArea) holds a clipping
//     viewport host at the frame minus ViewportInset, which holds the
//     document host, framed at minus the shown offset with the document
//     size: the document is clipped to the viewport and translated by the
//     offset (PaintTree culls the rows outside it). The shown offset
//     includes the overscroll while rubber-banding; Offset (the stub's)
//     is the offset clamped to the document. After the viewport come the
//     two scrollers (scroller.go: overlay or legacy, their knob, fade,
//     hover, knob drag and track click), drawn on top of the content.
//   - Legacy scrollers (S3.5f): while the window's appearance (the
//     driver's outside a window) has LegacyScrollers, each axis with a
//     scroller takes LegacyScrollerThickness from the viewport
//     (ViewportInset: Right for the vertical one, Bottom for the
//     horizontal one; it depends only on the appearance and the scroller
//     flags, never on the document, so it cannot oscillate). The kind is
//     read when the scroll view is made, on every SetFrame (so a scroll
//     view joining a window follows it; the layout reads ViewportSize
//     right after), on SetHasVerticalScroller and SetHasHorizontalScroller,
//     and on a WindowStateEvent (the "Always show scrollbars" switch at
//     runtime). A change outside SetFrame calls ViewportResized once, so
//     the public layer lays the window out again; a change during SetFrame
//     does not (the layout is running). The insets are the only ones the
//     scroll view sets itself; UserResizeViewport still sets any.
//   - Input: a ScrollEvent anywhere over the scroll view (its viewport,
//     the document's elements and the scrollers, which do not handle
//     scrolling) drives the axes that have a scroller
//     (SetHasVerticalScroller, SetHasHorizontalScroller), as in AppKit;
//     the other axis's delta is ignored. A wheel step or a precise event
//     outside a gesture is taken when an axis with a scroller can move by
//     it; otherwise (at an edge, no scroller on that axis) the event
//     bubbles to the parent. A precise gesture is taken at its PhaseBegan,
//     after which the router latches the rest of it (and its OS momentum)
//     to the scroll view: it is declined only when its axes cannot move by
//     the first delta and an enclosing scroll view can, which then takes
//     the whole gesture; else the scroll view rubber-bands at its edge. A
//     taken event shows the overlay scrollers. Pointer moves that reach the
//     scroll view's element drive the scrollers' hover; it handles none.
//   - Scrolled is called whenever the reported (clamped) offset changes by
//     user input or momentum (the scrollers' knob drag and track click
//     included), once per change. SetOffset, ScrollRectToVisible,
//     UserScroll (which reports through the stub), SetFrame,
//     SetDocumentSize and UserResizeViewport stop any gesture or animation
//     where they apply and do not call it; of them only UserScroll, which
//     simulates the user, shows the overlay scrollers.
//   - Without the driver's timeline (a driver that is not running), the
//     axes and the scrollers run on a still timeline with Reduce Motion:
//     everything jumps, and an overlay scroller once shown never fades.
//   - Keys (S3.5g) that bubble to the scroll view (the focused element
//     and its ancestors inside did not take them), on key-down and
//     repeat: Page Up and Page Down scroll by the viewport minus
//     PageOverlap, Home and End to the start and the end, Space and
//     Shift-Space as Page Down and Page Up but only when the focused
//     element is not a FocusControl (other than the scroll view itself),
//     so Space still presses a focused button or checkbox; the arrows
//     scroll by WheelLineHeight, only when the scroll view itself has the
//     focus. Paging, Home and End move the vertical axis when it has a
//     scroller, else the horizontal one; Up/Down the vertical and
//     Left/Right the horizontal axis, each only with its scroller. A key
//     step starts from where a running key or wheel step is going (so
//     repeats add up) and is animated like a wheel step
//     (WheelStepDuration), a jump under Reduce Motion. A modifier (other
//     than Shift on Space) leaves the key alone, and so does a pressed
//     scroller. A key is taken only when it moves the offset; at an edge
//     it bubbles on (to an enclosing scroll view). It is user input:
//     Scrolled is called and the overlay scrollers show.
//   - Scroll-to-focused (S3.5g): when the focus moves to an element inside
//     the document, the router's focus hook calls the enclosing scroll
//     views innermost first, and each scrolls at once (no animation, so
//     an outer one sees the inner one's final offset) by the least amount
//     that brings the element's frame grown by the focus ring
//     (FocusRingBand at full progress) into its viewport, or its top-left
//     corner when it does not fit; only the axes with a scroller move. It
//     starts from where a running key or wheel step is going. A scroll it
//     makes is user input as above. Clearing the focus or the window
//     becoming key again scrolls nothing.
//   - Accessibility: a scroll area; its descendants' frames follow the
//     offset. The scrollers are not exposed.
type ScrollView struct {
	*stub.ScrollView
	peerBase
	d        *Driver
	tl       *anim.Timeline
	view     *scrollArea
	viewport *host
	document *host
	x, y     *scroll.Axis
	bars     [2]*scroller // horizontal, vertical
	legacy   bool         // legacy scrollers
	legacyIn driver.Insets
	quiet    int // > 0: offset changes do not call Scrolled
}

// stillClock is the clock of a scroll view made without the driver's
// timeline: no time passes and no frame ever runs.
type stillClock struct{}

func (stillClock) Now() time.Duration { return 0 }
func (stillClock) RequestFrame()      {}

// NewScrollView returns a scroll view.
func (d *Driver) NewScrollView(h driver.ScrollHandler) driver.ScrollView {
	s := &ScrollView{ScrollView: stub.NewScrollView(h), d: d, viewport: newHost(), document: newHost()}
	s.view = &scrollArea{p: s}
	s.view.Init(s.view)
	s.viewport.SetClipsChildren(true)
	s.viewport.AddChild(s.document)
	s.view.AddChild(s.viewport)
	s.initElement(s.ScrollView, &s.ScrollView.Base, s.view)

	s.tl = d.timeline
	if s.tl == nil {
		s.tl = anim.NewTimeline(stillClock{})
		s.tl.SetReduceMotion(true)
	}
	p := scroll.ThemeParams(tokens)
	s.x = scroll.NewAxis(s.tl, p, func(float64) { s.moved() })
	s.y = scroll.NewAxis(s.tl, p, func(float64) { s.moved() })
	s.bars = [2]*scroller{newScroller(s, false), newScroller(s, true)}
	s.view.AddChild(s.bars[0])
	s.view.AddChild(s.bars[1])
	s.updateLegacy(false)
	return s
}

// moved follows the axes: the document's frame and the scrollers, the
// reported offset and Scrolled when it changed; outside sync the user
// scrolled, so the overlay scrollers show.
func (s *ScrollView) moved() {
	s.place()
	if s.quiet == 0 {
		s.flash()
	}
	off := driver.Point{X: s.x.Clamped(), Y: s.y.Clamped()}
	if off == s.Off {
		return
	}
	s.Off = off
	if s.quiet == 0 && s.Handler != nil {
		s.Handler.Scrolled(off)
	}
}

// flash shows the overlay scrollers whose axis can scroll and arms their
// fade.
func (s *ScrollView) flash() {
	for _, b := range s.bars {
		b.flash()
	}
}

// userScrollTo scrolls a to x as the user (Scrolled, the scrollers show):
// the knob drag and the track click.
func (s *ScrollView) userScrollTo(a *scroll.Axis, x float64, animated bool) {
	a.ScrollTo(x, animated)
	s.flash()
}

// place frames the viewport, the document host and the scrollers for the
// current frame, viewport inset, document size and shown offset.
func (s *ScrollView) place() {
	vp := s.view.Bounds().Inset(s.ViewportInset)
	s.viewport.SetFrame(vp)
	s.document.SetFrame(driver.RectOf(driver.Point{X: 0 - s.x.Offset(), Y: 0 - s.y.Offset()}, s.DocumentSize)) // 0 - x: no -0 in dumps
	both := s.HasVertical && s.HasHorizontal
	corner := both && (s.legacy || (s.bars[0].active() && s.bars[1].active()))
	for _, b := range s.bars {
		b.SetFrame(b.strip(vp, corner))
		b.sync()
		b.Invalidate() // the knob follows the offset
	}
	if s.legacy && both {
		s.view.Invalidate() // the corner
	}
}

// sync quietly gives the axes the stub's viewport, range and offset, after
// a change the stub made (it clamps its offset itself); stop also stops
// any gesture or animation at that offset.
func (s *ScrollView) sync(stop bool) {
	s.quiet++
	defer func() { s.quiet-- }()
	off := s.Off // the stub's, clamped by it
	vp, m := s.ViewportSize(), s.MaxOffset()
	s.x.SetViewport(vp.W)
	s.y.SetViewport(vp.H)
	s.x.SetMax(m.X)
	s.y.SetMax(m.Y)
	if stop {
		s.x.ScrollTo(off.X, false)
		s.y.ScrollTo(off.Y, false)
	}
	// Without stop the axes clamp (or spring back) themselves.
	s.Off = driver.Point{X: s.x.Clamped(), Y: s.y.Clamped()}
	s.place()
}

// updateLegacy follows the appearance's LegacyScrollers (see ScrollView);
// fire reports a changed viewport through ViewportResized.
func (s *ScrollView) updateLegacy(fire bool) {
	on := appearanceIn(&s.view.Base, s.d).LegacyScrollers
	var in driver.Insets
	if on {
		t := tokens.Scroll.LegacyScrollerThickness
		if s.HasVertical {
			in.Right = t
		}
		if s.HasHorizontal {
			in.Bottom = t
		}
	}
	kind := on != s.legacy
	s.legacy = on
	if kind {
		for _, b := range s.bars {
			b.reset()
		}
	}
	switch {
	case in != s.legacyIn:
		s.legacyIn = in
		if fire {
			s.ScrollView.UserResizeViewport(in) // calls ViewportResized
		} else {
			s.ViewportInset = in
		}
		s.sync(false)
	case kind:
		s.place()
	}
}

func (s *ScrollView) SetFrame(r driver.Rect) {
	s.peerBase.SetFrame(r)
	s.updateLegacy(false)
	s.sync(false)
}

func (s *ScrollView) SetDocument(w driver.Widget) {
	s.ScrollView.SetDocument(w)
	s.document.setChild(w)
}

func (s *ScrollView) SetDocumentSize(size driver.Size) {
	s.ScrollView.SetDocumentSize(size)
	s.sync(false)
}

// SetHasVerticalScroller sets whether the vertical axis scrolls and has a
// scroller; legacy scrollers resize the viewport (ViewportResized).
func (s *ScrollView) SetHasVerticalScroller(has bool) {
	s.ScrollView.SetHasVerticalScroller(has)
	s.scrollersChanged(s.bars[1], has)
}

// SetHasHorizontalScroller is SetHasVerticalScroller for the horizontal
// axis.
func (s *ScrollView) SetHasHorizontalScroller(has bool) {
	s.ScrollView.SetHasHorizontalScroller(has)
	s.scrollersChanged(s.bars[0], has)
}

func (s *ScrollView) scrollersChanged(b *scroller, has bool) {
	if !has {
		b.reset()
	}
	s.updateLegacy(true)
	s.place()
}

func (s *ScrollView) SetOffset(offset driver.Point) {
	s.ScrollView.SetOffset(offset)
	s.sync(true)
}

func (s *ScrollView) ScrollRectToVisible(r driver.Rect) {
	s.ScrollView.ScrollRectToVisible(r)
	s.sync(true)
}

// UserScroll simulates scrolling (stub.ScrollView.UserScroll, which calls
// Scrolled), moves the document there and shows the overlay scrollers.
func (s *ScrollView) UserScroll(offset driver.Point) {
	s.ScrollView.UserScroll(offset)
	s.sync(true)
	s.flash()
}

// UserResizeViewport simulates a viewport change
// (stub.ScrollView.UserResizeViewport) and moves the viewport.
func (s *ScrollView) UserResizeViewport(in driver.Insets) {
	s.ScrollView.UserResizeViewport(in)
	s.sync(false)
}

// Legacy reports whether the scroll view shows legacy scrollers.
func (s *ScrollView) Legacy() bool { return s.legacy }

// axes returns the axes with a scroller and the event's delta on each.
func (s *ScrollView) axes(ev shell.ScrollEvent) (ax [2]*scroll.Axis, d [2]float64) {
	if s.HasHorizontal {
		ax[0], d[0] = s.x, ev.Delta.X
	}
	if s.HasVertical {
		ax[1], d[1] = s.y, ev.Delta.Y
	}
	return ax, d
}

// canMove reports whether an axis with a scroller can move by the event's
// delta (shell direction: the offset moves by −delta).
func (s *ScrollView) canMove(ev shell.ScrollEvent) bool {
	ax, d := s.axes(ev)
	for i, a := range ax {
		if a != nil && d[i] != 0 && a.CanScroll(-d[i]) {
			return true
		}
	}
	return false
}

// scroll handles a scroll event routed to the scroll view.
func (s *ScrollView) scroll(ev shell.ScrollEvent) bool {
	ax, d := s.axes(ev)
	if ax[0] == nil && ax[1] == nil {
		return false // no scroller: the parent scrolls
	}
	if s.pressed() {
		// A knob drag or track click holds the offset: latched OS
		// momentum or a wheel must not move it under the pointer.
		return true
	}
	if ev.Phase == shell.PhaseBegan && !s.canMove(ev) && s.outerTakes(ev) {
		return false
	}
	took := false
	for i, a := range ax {
		if a != nil && a.Event(ev, d[i]) {
			took = true
		}
	}
	if took {
		s.flash()
	}
	return took
}

// key handles a key event that bubbled to the scroll view (see
// ScrollView) and reports whether it scrolled.
func (s *ScrollView) key(ev shell.KeyEvent) bool {
	if !ev.Down || s.pressed() {
		return false
	}
	shift := ev.Mods == driver.ModShift
	if ev.Mods != 0 && (!shift || ev.Key != driver.KeySpace) {
		return false
	}
	page := s.axisIf(s.y, s.HasVertical)
	if page == nil {
		page = s.axisIf(s.x, s.HasHorizontal)
	}
	focus := s.focused()
	self := focus != nil && focus.base() == &s.view.Base
	var a *scroll.Axis
	var by float64 // the direction (paging, Home/End) or the step (arrows)
	end := false   // Home or End
	switch ev.Key {
	case driver.KeyPageDown:
		a, by = page, 1
	case driver.KeyPageUp:
		a, by = page, -1
	case driver.KeyHome:
		a, end = page, true
	case driver.KeyEnd:
		a, by, end = page, 1, true
	case driver.KeySpace:
		if focus != nil && !self && focus.base().focusKind == FocusControl {
			return false // Space presses the focused control
		}
		a, by = page, 1
		if shift {
			by = -1
		}
	case driver.KeyUp, driver.KeyDown, driver.KeyLeft, driver.KeyRight:
		if !self {
			return false
		}
		switch ev.Key {
		case driver.KeyUp:
			a, by = s.axisIf(s.y, s.HasVertical), -1
		case driver.KeyDown:
			a, by = s.axisIf(s.y, s.HasVertical), 1
		case driver.KeyLeft:
			a, by = s.axisIf(s.x, s.HasHorizontal), -1
		case driver.KeyRight:
			a, by = s.axisIf(s.x, s.HasHorizontal), 1
		}
		by *= tokens.Scroll.WheelLineHeight
	}
	if a == nil {
		return false
	}
	from := keyBase(a)
	var x float64
	switch {
	case end && by > 0:
		x = a.Max()
	case end:
		x = 0
	case ev.Key == driver.KeyPageDown, ev.Key == driver.KeyPageUp, ev.Key == driver.KeySpace:
		x = from + by*s.pageLength(a)
	default:
		x = from + by
	}
	x = math.Max(0, math.Min(a.Max(), x))
	if x == from {
		return false // at the edge: an enclosing scroll view may move
	}
	s.userScrollTo(a, x, true) // jumps under Reduce Motion
	return true
}

// keyBase returns where a key step or a reveal starts: where a running key
// or wheel step is going, else the clamped offset.
func keyBase(a *scroll.Axis) float64 {
	if a.Stepping() {
		return a.Target()
	}
	return a.Clamped()
}

// axisIf returns a when has, else nil.
func (s *ScrollView) axisIf(a *scroll.Axis, has bool) *scroll.Axis {
	if has {
		return a
	}
	return nil
}

// pageLength returns how far a page key scrolls a: its viewport length
// minus PageOverlap (the whole viewport when it is not longer than that).
func (s *ScrollView) pageLength(a *scroll.Axis) float64 {
	vp := s.ViewportSize()
	l := vp.H
	if a == s.x {
		l = vp.W
	}
	if o := tokens.Scroll.PageOverlap; l > o {
		return l - o
	}
	return l
}

// focused returns the focused element of the scroll view's window, or nil.
func (s *ScrollView) focused() Element {
	if r := s.view.routedBy(); r != nil {
		return r.Focused()
	}
	return nil
}

// reveal scrolls at once by the least amount that brings f's frame, grown
// by the focus ring, into the viewport (see ScrollView); f is inside the
// document.
func (s *ScrollView) reveal(f Element) {
	outset, width := FocusRingBand(1)
	g := outset + width
	o := ToRoot(f, driver.Point{}).Sub(ToRoot(s.document, driver.Point{}))
	b := f.base().Bounds()
	r := driver.Rect{X: o.X - g, Y: o.Y - g, W: b.W + 2*g, H: b.H + 2*g}
	vp := s.ViewportSize()
	moved := false
	for _, ax := range [2]struct {
		a              *scroll.Axis
		lo, size, view float64
	}{
		{s.axisIf(s.x, s.HasHorizontal), r.X, r.W, vp.W},
		{s.axisIf(s.y, s.HasVertical), r.Y, r.H, vp.H},
	} {
		if ax.a == nil {
			continue
		}
		from := keyBase(ax.a)
		x := from
		// The far edge first, then the near one: a rect longer than the
		// viewport shows its start.
		if ax.lo+ax.size > x+ax.view {
			x = ax.lo + ax.size - ax.view
		}
		if ax.lo < x {
			x = ax.lo
		}
		x = math.Max(0, math.Min(ax.a.Max(), x))
		if x != from {
			ax.a.ScrollTo(x, false)
			moved = true
		}
	}
	if moved {
		s.flash()
	}
}

// pressed reports whether a scroller is pressed (a knob drag or a held
// track click).
func (s *ScrollView) pressed() bool {
	for _, b := range s.bars {
		if b != nil && b.press.down {
			return true
		}
	}
	return false
}

// outerTakes reports whether an enclosing scroll view could move by the
// event's delta.
func (s *ScrollView) outerTakes(ev shell.ScrollEvent) bool {
	for e := parentOf(s.view); e != nil; e = parentOf(e) {
		if o, ok := e.(*scrollArea); ok && o.p.canMove(ev) {
			return true
		}
	}
	return false
}

// hover follows the pointer over the scrollers' strips (pos in the scroll
// view's element; over false: the pointer left it).
func (s *ScrollView) hover(pos driver.Point, over bool) {
	for _, b := range s.bars {
		b.setHover(over && !s.legacy && b.active() && b.Frame().Contains(pos))
	}
}

// scrollArea is the element of a ScrollView.
type scrollArea struct {
	Base
	p *ScrollView
}

func (e *scrollArea) FittingSize(max driver.Size) driver.Size { return e.p.FittingSize(max) }

// HandleEvent takes the scroll events (ScrollView.scroll), follows the
// pointer for the scrollers' hover (handling no pointer event) and the
// appearance for the legacy scrollers.
func (e *scrollArea) HandleEvent(ev Event) bool {
	switch ev := ev.(type) {
	case ScrollEvent:
		return e.p.scroll(ev.ScrollEvent)
	case PointerEvent:
		switch ev.Kind {
		case PointerEnter, PointerMove:
			e.p.hover(ev.Pos, e.EnabledInTree())
		case PointerLeave:
			e.p.hover(ev.Pos, false)
		}
	case KeyEvent:
		return e.p.key(ev.KeyEvent)
	case WindowStateEvent:
		e.p.updateLegacy(true)
	}
	return false
}

// revealFocus brings the newly focused element f, inside the document,
// into view (the router's focus hook, focusRevealer).
func (e *scrollArea) revealFocus(f Element) { e.p.reveal(f) }

// Paint fills the corner between two legacy tracks.
func (e *scrollArea) Paint(c render.Canvas) {
	s := e.p
	if !s.legacy || !s.HasVertical || !s.HasHorizontal {
		return
	}
	vp := s.viewport.Frame()
	t := tokens.Scroll.LegacyScrollerThickness
	r := pixelAlign(&e.Base, driver.Rect{X: vp.MaxX(), Y: vp.MaxY(), W: t, H: t}, scaleIn(&e.Base))
	if !r.Empty() {
		c.FillRoundedRect(r, render.Radii{}, render.Solid(tokens.Scroll.LegacyScrollerTrack.Value(appearanceIn(&e.Base, s.d))))
	}
}

func (e *scrollArea) AccessibleNode() a11y.Node { return a11y.Node{Role: a11y.RoleScrollArea} }
