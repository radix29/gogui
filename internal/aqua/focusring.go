package aqua

import (
	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
)

// FocusShape is the outline a focus ring is drawn around, in the element's
// own coordinates: a rounded rect (Rect and Radii), or Path when it is not
// nil (Rect and Radii are then ignored).
type FocusShape struct {
	Rect  driver.Rect
	Radii render.Radii
	Path  *render.Path
}

// RectFocusShape returns the shape of a rounded rect.
func RectFocusShape(r driver.Rect, radii render.Radii) FocusShape {
	return FocusShape{Rect: r, Radii: radii}
}

// PathFocusShape returns the shape of a path (a non-rectangular control).
func PathFocusShape(p *render.Path) FocusShape { return FocusShape{Path: p} }

// FocusRing is the keyboard focus ring of one element (PLAN.md S3.2d), an
// embeddable helper. The element passes its events to Track and paints the
// ring last in its Paint with FocusRing.Paint:
//
//	type field struct {
//		aqua.Base
//		ring aqua.FocusRing
//	}
//
//	func (f *field) HandleEvent(ev aqua.Event) bool {
//		f.ring.Track(f, ev)
//		…
//	}
//
//	func (f *field) Paint(c render.Canvas) {
//		… // the bezel
//		f.ring.Paint(c, f, aqua.RectFocusShape(f.Bounds(), radii))
//	}
//
// The ring is shown while the element is the focused element of the key
// window (FocusState FocusedActive). It animates in over the theme's
// FocusRingDuration with EaseOut on the window's timeline (from a wider,
// faint outline further out to its final band, as on macOS), each time the
// element gains the focus or its window becomes key again; it disappears at
// once on blur and when the window becomes inactive. With Reduce Motion,
// or in a window without a timeline (one made with NewWindow directly), it
// appears at once. The zero value is ready to use.
type FocusRing struct {
	on bool
	p  float64        // progress, 0 (none) to 1 (full ring)
	v  *anim.Value    // the animation; nil before the first one
	tl *anim.Timeline // v's timeline
}

// Progress returns how far the ring has animated in: 0 for no ring, 1 for
// the full ring.
func (f *FocusRing) Progress() float64 { return f.p }

// Visible reports whether the ring is shown or animating in.
func (f *FocusRing) Visible() bool { return f.on }

// Track updates the ring for an event of owner (the element that embeds
// it): FocusEvent and WindowStateEvent may show or hide it, other events
// are ignored. It never handles the event, so the element still answers
// it.
func (f *FocusRing) Track(owner Element, ev Event) {
	switch ev.(type) {
	case FocusEvent, WindowStateEvent:
	default:
		return
	}
	b := owner.base()
	want := b.FocusState() == FocusedActive
	if want == f.on {
		return
	}
	f.on = want
	if f.v != nil {
		f.v.Stop()
	}
	if !want {
		f.set(b, 0)
		return
	}
	w := b.Window()
	if w == nil || w.Timeline() == nil {
		f.set(b, 1)
		return
	}
	tl := w.Timeline()
	if f.v == nil || f.tl != tl {
		f.tl = tl
		f.v = tl.NewValue(0, func(x float64) { f.set(b, x) })
	}
	f.v.Set(0)
	an := &tokens.Animation
	f.v.AnimateTo(1, an.Duration(an.FocusRingDuration, w.Appearance()), anim.Curve(an.EaseOut))
}

// set changes the progress and repaints owner on a change.
func (f *FocusRing) set(owner *Base, p float64) {
	if p == f.p {
		return
	}
	f.p = p
	owner.Invalidate()
}

// Paint draws the ring around shape with the current progress, in the
// appearance of owner's window (the driver's outside one). It draws
// nothing while the ring is hidden.
func (f *FocusRing) Paint(c render.Canvas, owner Element, shape FocusShape) {
	if f.p <= 0 {
		return
	}
	a := driver.DefaultAppearance()
	if w := owner.base().Window(); w != nil {
		a = w.Appearance()
	}
	PaintFocusRing(c, shape, f.p, a)
}

// FocusRingBand returns where the ring is drawn at progress p (clamped to
// 0..1): its inner edge's distance outside the shape and its width. The
// full ring (p = 1) is the theme's FocusRingWidth wide at FocusRingOffset;
// at p = 0 it starts one width further out and twice as wide.
func FocusRingBand(p float64) (outset, width float64) {
	p = min(max(p, 0), 1)
	m := &tokens.Metrics
	grow := 1 - p
	return m.FocusRingOffset + grow*m.FocusRingWidth, m.FocusRingWidth * (1 + grow)
}

// PaintFocusRing draws a focus ring at progress p (0..1) around shape, in
// the coordinates shape is in, for the appearance a: the band of
// FocusRingBand(p) in Theme.FocusRing(a) with its alpha scaled by p, so
// the ring fades in as it settles. A rounded rect's ring follows its
// outline exactly (each rounded corner's radius grows by the outset, sharp
// corners stay sharp); a path's ring is the path scaled about the centre of
// its bounds so that they grow by the outset, which is exact for rects and
// ellipses and close for convex shapes. The ring lies outside the shape,
// so an element painting it in its own Paint is not clipped by itself; a
// clipping ancestor may cut it. It draws nothing for p ≤ 0.
func PaintFocusRing(c render.Canvas, shape FocusShape, p float64, a driver.Appearance) {
	if !(p > 0) {
		return
	}
	p = min(p, 1)
	outset, width := FocusRingBand(p)
	d := outset + width/2 // the stroke's centre line
	col := tokens.FocusRing(a)
	col.A *= float32(p)
	paint := render.Solid(col)
	if shape.Path == nil {
		r := shape.Rect.Inset(driver.UniformInsets(-d))
		grow := func(x float64) float64 {
			if x > 0 {
				return x + d
			}
			return 0
		}
		rd := shape.Radii
		radii := render.Radii{TopLeft: grow(rd.TopLeft), TopRight: grow(rd.TopRight),
			BottomRight: grow(rd.BottomRight), BottomLeft: grow(rd.BottomLeft)}
		c.StrokeRoundedRect(r, radii, width, paint)
		return
	}
	bb := shape.Path.Bounds()
	sx, sy := 1.0, 1.0
	if bb.W > 0 {
		sx = (bb.W + 2*d) / bb.W
	}
	if bb.H > 0 {
		sy = (bb.H + 2*d) / bb.H
	}
	cx, cy := bb.X+bb.W/2, bb.Y+bb.H/2
	t := render.Translation(-cx, -cy).Then(render.Scaling(sx, sy)).Then(render.Translation(cx, cy))
	c.StrokePath(transformPath(shape.Path, t), width, paint, render.StrokeStyle{})
}

// transformPath returns a copy of p with every point mapped by t.
func transformPath(p *render.Path, t render.Transform) *render.Path {
	out := &render.Path{}
	for s := range p.Segments() {
		q := func(i int) render.Point { return t.Apply(s.Pts[i]) }
		switch s.Op {
		case render.OpMoveTo:
			out.MoveTo(q(0))
		case render.OpLineTo:
			out.LineTo(q(0))
		case render.OpQuadTo:
			out.QuadTo(q(0), q(1))
		case render.OpCubicTo:
			out.CubicTo(q(0), q(1), q(2))
		case render.OpClose:
			out.Close()
		}
	}
	return out
}
