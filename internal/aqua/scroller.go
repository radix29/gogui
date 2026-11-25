package aqua

import (
	"math"

	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/aqua/scroll"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
)

// The scrollers of a ScrollView (PLAN.md §5.6, S3.5e, S3.5f): one element
// per axis, a child of the scroll view's element after the viewport, so
// it is drawn on top of the content.
//
//   - Overlay (the default): the scroller's frame is a strip along the
//     viewport's trailing edge (the right one for the vertical scroller,
//     the bottom one for the horizontal), ScrollerHoverThickness plus
//     twice ScrollerInset wide, inside the viewport; when both axes have
//     an active scroller (a scroller and a range to scroll), the vertical
//     one stops short of the horizontal one's strip and the horizontal
//     one short of the vertical one's. The knob is drawn ScrollerInset in
//     from the trailing edge, ScrollerThickness thick (animated to
//     ScrollerHoverThickness while hovered), in ScrollerKnob times the
//     scroller's opacity. Nothing else is drawn.
//   - Legacy (Appearance.LegacyScrollers): the strip is
//     LegacyScrollerThickness wide, outside the viewport (the scroll view
//     takes it from the viewport through ViewportInset), filled with
//     LegacyScrollerTrack, with a LegacyScrollerBorder hairline (one
//     device pixel) on the content's side; the knob is
//     ScrollerHoverThickness thick, centred across the track, opaque. When
//     both axes have a scroller, the scroll view fills the corner between
//     the two tracks with LegacyScrollerTrack.
//   - The knob (both kinds): along the strip it runs between ScrollerInset
//     from either end (the track). Its length is the track's times the
//     visible fraction (viewport / document), at least
//     ScrollerMinKnobLength (at most the track); it moves over the rest
//     of the track as the clamped offset goes from 0 to the maximum.
//     While the content is overscrolled (rubber-banding or springing
//     back) the knob is shortened by the overscroll scaled to the track
//     (overscroll × track / viewport), down to its thickness (a round
//     knob), and stays pinned to the edge it is pushed against. There is
//     no knob when the axis cannot scroll (maximum offset 0) or the track
//     is shorter than the knob's thickness. Rects are pixel-aligned; the
//     knob's corners are half its thickness.
//   - Visibility: an overlay scroller is shown (opacity 1, at once) when
//     the user scrolls (input or momentum moving that scroll view, a knob
//     drag, a track click; not SetOffset, ScrollRectToVisible or a
//     layout change) and while it is hovered or dragged, and only when
//     its axis can scroll. ScrollerFadeDelay after the scrolling (the last
//     offset change) stops, or after the pointer leaves it, it fades out
//     over ScrollerFadeDuration (EaseInOut); at opacity 0 it is hidden, so
//     it is neither painted nor hit-tested and the content under it gets
//     the pointer. With Reduce Motion it is hidden at once after the delay
//     (no fade) and the hover widening jumps. A legacy scroller is always
//     shown while its axis has a scroller (the knob only when it can
//     scroll).
//   - Hover (S3.5f, overlay): the scroll view's element tracks the pointer
//     moves that reach it (from the viewport, the document and the
//     scrollers, which handle no moves outside a drag): while the pointer
//     is over the strip of a scroller whose axis can scroll, the scroller
//     is shown and widened over ScrollerHoverDuration (EaseOut); it narrows
//     and arms the fade when the pointer leaves the strip or the scroll
//     view. Hovering does not change a legacy scroller.
//   - Knob drag (S3.5f): a primary press on the knob's span of the strip
//     (anywhere across it) captures the pointer and grabs the knob where
//     it was hit; while the button is held the offset follows the pointer
//     1:1 along the track (clamped; dragged off the strip too), stopping
//     any momentum or animation, and Scrolled is called on every change.
//     A release or a PointerCancel ends the drag; other buttons are
//     ignored (swallowed during a drag).
//   - Track click (S3.5f): a primary press on the strip before or after
//     the knob pages towards the click (the macOS default "jump to the
//     next page"): the viewport's length minus PageOverlap (at least half
//     the viewport), animated like a wheel step (a jump with Reduce
//     Motion), and captures the pointer until the release.
//   - Disabled (the scroll view or an ancestor): no input reaches a
//     scroller; it is still drawn.
type scroller struct {
	Base
	sv       *ScrollView
	vertical bool
	alpha    *anim.Value // overlay opacity: 0 hidden … 1 shown
	wide     *anim.Value // overlay thickness: 0 ScrollerThickness … 1 ScrollerHoverThickness
	hovered  bool
	press    pressTracker // down while a knob drag or a track click holds the pointer
	dragging bool         // the press is a knob drag
	grab     float64      // the pointer's offset from the knob's start along the strip
}

func newScroller(s *ScrollView, vertical bool) *scroller {
	b := &scroller{sv: s, vertical: vertical}
	b.Init(b)
	b.alpha = s.tl.NewValue(0, func(x float64) {
		b.sync()
		b.Invalidate()
	}).SetTolerance(anim.AlphaTolerance)
	b.wide = s.tl.NewValue(0, func(float64) { b.Invalidate() }).SetTolerance(anim.AlphaTolerance)
	b.SetHidden(true)
	return b
}

// axis returns the scroller's axis.
func (b *scroller) axis() *scroll.Axis {
	if b.vertical {
		return b.sv.y
	}
	return b.sv.x
}

// has reports whether the scroll view has this scroller.
func (b *scroller) has() bool {
	if b.vertical {
		return b.sv.HasVertical
	}
	return b.sv.HasHorizontal
}

// active reports whether the scroller is there and its axis can scroll.
func (b *scroller) active() bool { return b.has() && b.axis().Max() > 0 }

// viewportLength returns the viewport's length along the scroller.
func (b *scroller) viewportLength() float64 {
	vp := b.sv.ViewportSize()
	if b.vertical {
		return vp.H
	}
	return vp.W
}

// sync shows or hides the element: a legacy scroller while it is there,
// an overlay one while it is there and not fully faded.
func (b *scroller) sync() {
	switch {
	case !b.has():
		b.SetHidden(true)
	case b.sv.legacy:
		b.SetHidden(false)
	default:
		b.SetHidden(b.alpha.Value() == 0 || !b.active())
	}
}

// held reports whether the scroller stays shown: hovered or dragged.
func (b *scroller) held() bool { return b.hovered || b.press.Down() }

// flash shows an overlay scroller and arms its fade (nothing for a legacy
// one or one whose axis cannot scroll).
func (b *scroller) flash() {
	if b.sv.legacy || !b.active() {
		return
	}
	b.alpha.Set(1)
	b.armFade()
}

// armFade starts the fade-out after ScrollerFadeDelay, unless the scroller
// is held (then it stays at 1).
func (b *scroller) armFade() {
	if b.sv.legacy {
		return
	}
	if b.held() {
		b.alpha.Set(1) // stops a fade in progress
		return
	}
	an := &tokens.Animation
	b.alpha.AnimateAfter(0, an.ScrollerFadeDelay, an.ScrollerFadeDuration, anim.Curve(an.EaseInOut))
}

// setHover widens (or narrows) an overlay scroller and shows it (or arms
// its fade).
func (b *scroller) setHover(on bool) {
	if on == b.hovered {
		return
	}
	b.hovered = on
	b.wide.AnimateTo(boolFloat(on), tokens.Scroll.ScrollerHoverDuration, anim.Curve(tokens.Animation.EaseOut))
	if on {
		if !b.sv.legacy && b.active() {
			b.alpha.Set(1)
		}
		return
	}
	if !b.press.Down() && b.alpha.Value() > 0 {
		b.armFade()
	}
}

func boolFloat(on bool) float64 {
	if on {
		return 1
	}
	return 0
}

// reset drops the hover and the press and hides an overlay scroller at
// once (the scroller went away, or the kind changed).
func (b *scroller) reset() {
	b.hovered, b.dragging = false, false
	b.press.Cancel(b)
	b.wide.Set(0)
	b.alpha.Set(0)
	b.sync()
}

// strip returns the scroller's frame in the scroll view's element: see
// scroller. vp is the viewport's frame there.
func (b *scroller) strip(vp driver.Rect, corner bool) driver.Rect {
	sc := &tokens.Scroll
	w := sc.ScrollerHoverThickness + 2*sc.ScrollerInset
	if b.sv.legacy {
		w = sc.LegacyScrollerThickness
		if b.vertical {
			return driver.Rect{X: vp.MaxX(), Y: vp.Y, W: w, H: vp.H}
		}
		return driver.Rect{X: vp.X, Y: vp.MaxY(), W: vp.W, H: w}
	}
	cut := 0.0
	if corner {
		cut = w
	}
	if b.vertical {
		return driver.Rect{X: vp.MaxX() - w, Y: vp.Y, W: w, H: math.Max(0, vp.H-cut)}
	}
	return driver.Rect{X: vp.X, Y: vp.MaxY() - w, W: math.Max(0, vp.W-cut), H: w}
}

// scrollerGeometry is where a scroller's parts are, in its own
// coordinates, along and across the strip.
type scrollerGeometry struct {
	vertical  bool
	length    float64 // the strip along
	depth     float64 // the strip across
	thickness float64 // the knob across
	track0    float64 // where the track starts along
	track     float64 // the track's length
	knobLen   float64 // the knob's length at rest (unshortened)
	knob0     float64 // where the shown knob starts along
	knobShown float64 // the shown knob's length (shortened while overscrolled)
	across0   float64 // where the knob starts across
	ok        bool    // there is a knob
}

// rect returns the rect from along0, along long, and from a0, d deep
// across, turned for a vertical scroller.
func (g *scrollerGeometry) rect(along0, along, a0, d float64) driver.Rect {
	if g.vertical {
		return driver.Rect{X: a0, Y: along0, W: d, H: along}
	}
	return driver.Rect{X: along0, Y: a0, W: along, H: d}
}

// along returns p's coordinate along the strip.
func (g *scrollerGeometry) along(p driver.Point) float64 {
	if g.vertical {
		return p.Y
	}
	return p.X
}

// knobRect returns the shown knob's rect (unaligned).
func (g *scrollerGeometry) knobRect() driver.Rect {
	return g.rect(g.knob0, g.knobShown, g.across0, g.thickness)
}

// geometry lays the scroller out for the current offset (see scroller).
func (b *scroller) geometry() scrollerGeometry {
	sc := &tokens.Scroll
	r := b.Bounds()
	g := scrollerGeometry{vertical: b.vertical, length: r.W, depth: r.H}
	if b.vertical {
		g.length, g.depth = r.H, r.W
	}
	if b.sv.legacy {
		g.thickness = sc.ScrollerHoverThickness
		g.across0 = (g.depth - g.thickness) / 2
	} else {
		g.thickness = anim.Lerp(sc.ScrollerThickness, sc.ScrollerHoverThickness, b.wide.Value())
		g.across0 = g.depth - sc.ScrollerInset - g.thickness
	}
	g.track0 = sc.ScrollerInset
	g.track = g.length - 2*sc.ScrollerInset
	a := b.axis()
	vp := b.viewportLength()
	if a.Max() <= 0 || vp <= 0 || g.track < g.thickness {
		return g
	}
	g.ok = true
	g.knobLen = math.Min(g.track, math.Max(sc.ScrollerMinKnobLength, g.track*vp/(vp+a.Max())))
	travel := g.track - g.knobLen
	g.knob0 = g.track0 + travel*a.Clamped()/a.Max()
	g.knobShown = g.knobLen
	if over := a.Overscroll(); over != 0 {
		g.knobShown = math.Max(math.Min(g.thickness, g.knobLen), g.knobLen-math.Abs(over)*g.track/vp)
		if over < 0 {
			g.knob0 = g.track0
		} else {
			g.knob0 = g.track0 + g.track - g.knobShown
		}
	}
	return g
}

// Paint draws a legacy track and its border, then the knob.
func (b *scroller) Paint(c render.Canvas) {
	r := b.Bounds()
	if r.Empty() {
		return
	}
	a := appearanceIn(&b.Base, b.sv.d)
	scale := scaleIn(&b.Base)
	sc := &tokens.Scroll
	if b.sv.legacy {
		c.FillRoundedRect(pixelAlign(&b.Base, r, scale), render.Radii{}, render.Solid(sc.LegacyScrollerTrack.Value(a)))
		hair := 1 / scale
		border := driver.Rect{W: hair, H: r.H}
		if !b.vertical {
			border = driver.Rect{W: r.W, H: hair}
		}
		c.FillRoundedRect(pixelAlign(&b.Base, border, scale), render.Radii{}, render.Solid(sc.LegacyScrollerBorder.Value(a)))
	}
	g := b.geometry()
	if !g.ok {
		return
	}
	knob := pixelAlign(&b.Base, g.knobRect(), scale)
	if knob.Empty() {
		return
	}
	col := sc.ScrollerKnob.Value(a)
	if !b.sv.legacy {
		col.A *= float32(b.alpha.Value())
	}
	c.FillRoundedRect(knob, render.UniformRadii(min(knob.W, knob.H)/2), render.Solid(col))
}

// HandleEvent takes the knob drag and the track clicks (see scroller).
func (b *scroller) HandleEvent(ev Event) bool {
	pe, ok := ev.(PointerEvent)
	if !ok {
		return false
	}
	if !b.EnabledInTree() {
		b.endPress()
		return false
	}
	switch pe.Kind {
	case PointerDown:
		if pe.Button != shell.ButtonPrimary || b.press.Down() {
			return b.press.Down()
		}
		g := b.geometry()
		if !g.ok {
			return false
		}
		at := g.along(pe.Pos)
		b.press.Begin(b)
		b.alpha.Set(1)
		b.armFade() // held: stays shown
		switch {
		case at >= g.knob0 && at < g.knob0+g.knobShown:
			b.dragging = true
			b.grab = at - g.knob0
			b.axis().Stop()
		case at < g.knob0:
			b.page(-1)
		default:
			b.page(1)
		}
		return true
	case PointerMove:
		if b.dragging {
			b.follow(pe.Pos)
		}
		return b.press.Down()
	case PointerUp:
		if !b.press.Down() || pe.Button != shell.ButtonPrimary {
			return b.press.Down()
		}
		// The hover froze during the capture: it is where the release is.
		b.hovered = b.hovered && pe.Over
		if !b.hovered {
			b.wide.AnimateTo(0, tokens.Scroll.ScrollerHoverDuration, anim.Curve(tokens.Animation.EaseOut))
		}
		b.endPress()
		return true
	case PointerCancel:
		b.endPress()
	}
	return false
}

// endPress ends a drag or a track click and arms the fade.
func (b *scroller) endPress() {
	if !b.press.Down() {
		return
	}
	b.dragging = false
	b.press.Cancel(b)
	if b.alpha.Value() > 0 {
		b.armFade()
	}
}

// follow moves the offset so the knob's grab point is at the pointer.
func (b *scroller) follow(p driver.Point) {
	g := b.geometry()
	travel := g.track - g.knobLen
	a := b.axis()
	if !g.ok || travel <= 0 {
		return
	}
	f := clamp01((g.along(p) - b.grab - g.track0) / travel)
	b.sv.userScrollTo(a, f*a.Max(), false)
}

// page scrolls a page towards the end (dir 1) or the start (dir −1).
func (b *scroller) page(dir float64) {
	a := b.axis()
	vp := b.viewportLength()
	step := math.Max(vp-tokens.Scroll.PageOverlap, vp/2)
	base := a.Clamped()
	if a.Stepping() { // not momentum: its rest may be pages away
		base = math.Max(0, math.Min(a.Max(), a.Target())) // a running page or wheel step
	}
	b.sv.userScrollTo(a, base+dir*step, true)
}
