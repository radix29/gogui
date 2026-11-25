package scroll

import (
	"math"
	"time"

	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/shell"
)

// Params are the tokens an Axis runs on (PLAN.md §5.6, theme.Scroll and
// theme.Animation). ThemeParams reads them from a theme.
type Params struct {
	// LineHeight is how far one wheel line scrolls, in points.
	LineHeight float64
	// WheelStep is how long the smooth scroll of a wheel step (and of an
	// animated ScrollTo) takes, along WheelCurve. Not positive: wheel steps
	// jump.
	WheelStep  time.Duration
	WheelCurve anim.Curve

	// DecayRate (per second) and StopVelocity (points per second) shape the
	// momentum after a release (anim.Value.Decay); a release no faster
	// than StopVelocity has no momentum.
	DecayRate, StopVelocity float64

	// Window and EndGap configure the VelocityTracker of a gesture.
	Window, EndGap time.Duration

	// RubberBand is the rubber-band constant c as a fraction of the
	// viewport's length on the axis (SetViewport). Not positive: no
	// overscroll, a drag stops at the edges.
	RubberBand float64

	// Spring brings an overscrolled offset back to its edge.
	Spring anim.Spring
}

// ThemeParams returns the Params of th: its scroll tokens, the EaseOut
// curve for wheel steps, the RubberBandSpring and DefaultEndGap.
func ThemeParams(th *theme.Theme) Params {
	s := &th.Scroll
	return Params{
		LineHeight:   s.WheelLineHeight,
		WheelStep:    s.WheelStepDuration,
		WheelCurve:   anim.Curve(th.Animation.EaseOut),
		DecayRate:    s.MomentumDecayRate,
		StopVelocity: s.MomentumStopVelocity,
		Window:       s.VelocityWindow,
		EndGap:       DefaultEndGap,
		RubberBand:   s.RubberBandFraction,
		Spring:       anim.Spring(th.Animation.RubberBandSpring),
	}
}

// mode is what drives an Axis's offset.
type mode uint8

const (
	modeIdle     mode = iota // at rest, or an animated ScrollTo / wheel step
	modeDrag                 // a precise gesture: the fingers move the content
	modeDecay                // the axis's own momentum after a release
	modeOS                   // OS momentum (GTK): the shell's deltas move it
	modeBounce               // springing back to an edge
	modeOSBounce             // OS momentum hit an edge: its rest is ignored
)

// Axis is the scroll offset of one axis of a scroll view, on the frame
// clock (PLAN.md §5.6, S3.5c). The offset is in [0, Max] at rest; a
// precise drag and momentum can carry it past the edges (overscroll),
// rubber-banded, and it springs back.
//
// Offsets and deltas are in the offset's direction: a positive delta shows
// later content (the opposite sign of shell.ScrollEvent.Delta; Event
// converts). The input:
//
//   - A precise gesture (Event with Phase PhaseBegan, PhaseChanged…,
//     PhaseEnded) moves the offset 1:1 with the fingers inside the range;
//     past an edge the offset is RubberBand of how far the fingers went
//     past it (c = RubberBand × the viewport), so dragging back maps 1:1.
//     A gesture that begins during momentum or a spring-back catches the
//     content where it is (overscroll included, through RubberBandInverse).
//   - The release (PhaseEnded): the VelocityTracker's release velocity
//     (Win32's late, zero-delta end included). Inside the range, faster
//     than StopVelocity, the content decays (anim.Value.Decay); when the
//     decay reaches an edge with speed left, the content springs back from
//     the edge with that speed (Spring), so it overshoots and returns. A
//     release past an edge springs back with the release velocity, unless
//     the fingers fling the content back into the range, which decays.
//   - OS momentum (GTK; Momentum PhaseBegan, PhaseChanged…, PhaseEnded)
//     stops the axis's own decay, which started at PhaseEnded, and moves
//     the offset by the shell's deltas instead; when it reaches an edge,
//     the axis springs back from there with the momentum's speed and
//     ignores the rest of it.
//   - A wheel event (not precise, no phases) scrolls Delta × LineHeight
//     points, animated over WheelStep; further notches during the step
//     extend its target, so rapid notches accumulate. Wheel steps are
//     clamped to the range (no overscroll). A precise event outside a
//     gesture scrolls 1:1, clamped.
//
// The overshoot of a spring-back from an edge stays within 0.9·c: the
// speed it starts with is capped from the peak of the actual spring
// (anim.Motion.Peak, any damping regime). Content caught c or more past
// an edge (the document shrank under it) is caught 0.9·c past it.
//
// With Reduce Motion (the Timeline's) there is no momentum and no rubber
// band: a drag stops at the edges, a release stops, OS momentum is taken
// but moves nothing (S6.1f), wheel steps and animated ScrollTo jump.
// Turned on mid-flight, the timeline ends a wheel step, a decay and a
// spring-back at their targets (the decay's rest clamped to the range);
// FollowReduceMotion ends the rest: a drag held past an edge and OS
// momentum.
//
// An Axis is used on the UI thread only.
type Axis struct {
	tl       *anim.Timeline
	p        Params
	pos      *anim.Value // the shown offset, overscroll included
	onChange func(offset float64)

	max, viewport float64
	mode          mode
	raw           float64 // modeDrag: the fingers' offset, before the rubber band
	tracker       VelocityTracker
}

// NewAxis returns an axis at offset 0 with Max and viewport 0, animating
// on tl. onChange, which may be nil, is called with the shown offset
// (Offset) whenever it changes. It panics when tl is nil.
func NewAxis(tl *anim.Timeline, p Params, onChange func(offset float64)) *Axis {
	if tl == nil {
		panic("scroll: NewAxis with a nil timeline")
	}
	a := &Axis{tl: tl, p: p, onChange: onChange}
	a.tracker.Window, a.tracker.EndGap = p.Window, p.EndGap
	a.pos = tl.NewValue(0, a.changed).SetTolerance(anim.PointTolerance)
	return a
}

// Offset returns the shown offset: in [0, Max] at rest, past an edge while
// rubber-banding or springing back.
func (a *Axis) Offset() float64 { return a.pos.Value() }

// Clamped returns the offset clamped to [0, Max]: the scroll view's
// reported offset.
func (a *Axis) Clamped() float64 { return a.clamp(a.pos.Value()) }

// Overscroll returns how far the shown offset is past an edge: negative
// past the leading edge (0), positive past Max, else 0.
func (a *Axis) Overscroll() float64 { return a.pos.Value() - a.Clamped() }

// Max returns the largest offset at rest.
func (a *Axis) Max() float64 { return a.max }

// Target returns where the offset is going: the end of the running
// animation (a wheel step, a decay's rest, an edge), else the offset.
func (a *Axis) Target() float64 { return a.pos.Target() }

// Moving reports whether a gesture, momentum or an animation is moving the
// offset.
func (a *Axis) Moving() bool { return a.mode == modeDrag || a.mode == modeOS || a.pos.Running() }

// Dragging reports whether a precise gesture holds the content.
func (a *Axis) Dragging() bool { return a.mode == modeDrag }

// Stepping reports whether an animated ScrollTo or a wheel step is running
// (not a gesture, momentum or a spring back); Target is where it ends.
func (a *Axis) Stepping() bool { return a.mode == modeIdle && a.pos.Running() }

// SetViewport sets the viewport's length on the axis, for the rubber-band
// constant. A drag in progress is shown again with the new constant.
func (a *Axis) SetViewport(length float64) {
	a.viewport = math.Max(0, length)
	if a.mode == modeDrag {
		a.pos.Set(a.shown(a.raw))
	}
}

// SetMax sets the largest offset (negative counts as 0). An offset at rest
// beyond it is clamped at once; one that is overscrolled (rubber-banding
// or springing back) springs back to the new edge, and one that the new
// edge leaves overscrolled while it decays springs back from there. A
// drag in progress is shown again against the new edge; a wheel step's
// target is clamped. With Reduce Motion everything is clamped at once.
func (a *Axis) SetMax(m float64) {
	if !(m > 0) {
		m = 0
	}
	a.max = m
	x := a.pos.Value()
	switch a.mode {
	case modeDrag:
		a.pos.Set(a.shown(a.raw))
	case modeBounce, modeOSBounce:
		// Springing back to an edge: retarget it if the edge moved.
		if t := a.pos.Target(); t != a.clamp(t) {
			a.springTo(a.clamp(t), a.pos.Velocity())
		}
	case modeDecay, modeOS:
		// The decay that a new edge leaves past it springs back from
		// there; one still inside meets the edge as it moves on.
		if x != a.clamp(x) {
			os := a.mode == modeOS
			a.springTo(a.clamp(x), a.pos.Velocity())
			if os && a.mode == modeBounce {
				a.mode = modeOSBounce // the rest of the OS momentum is ignored
			}
		}
	default:
		switch {
		case a.pos.Running(): // a wheel step or an animated ScrollTo
			a.pos.AnimateTo(a.clamp(a.pos.Target()), a.p.WheelStep, a.p.WheelCurve)
		case x != a.clamp(x):
			a.stopAt(a.clamp(x)) // the document shrank under a scrolled offset
		}
	}
}

// ScrollTo scrolls to x, clamped to [0, Max]: animated over WheelStep, or
// at once. It ends any gesture or momentum first.
func (a *Axis) ScrollTo(x float64, animated bool) {
	x = a.clamp(x)
	a.mode = modeIdle
	if animated {
		a.pos.AnimateTo(x, a.p.WheelStep, a.p.WheelCurve)
		return
	}
	a.pos.Set(x)
}

// Stop stops any gesture, momentum or animation where the offset is,
// clamped to [0, Max] (an overscroll ends at its edge).
func (a *Axis) Stop() { a.stopAt(a.Clamped()) }

// FollowReduceMotion applies the Timeline's Reduce Motion turned on to
// what the timeline does not end itself (the owner calls it after the
// change; it does nothing with Reduce Motion off): a drag in progress
// loses its rubber band at once, its fingers' offset clamped to the
// range, and OS momentum (or the spring-back that ignores the rest of it)
// stops.
func (a *Axis) FollowReduceMotion() {
	if !a.tl.ReduceMotion() {
		return
	}
	switch a.mode {
	case modeIdle:
	case modeDrag:
		a.raw = a.clamp(a.raw)
		a.pos.Set(a.raw)
	default:
		a.Stop()
	}
}

func (a *Axis) stopAt(x float64) {
	a.mode = modeIdle
	a.pos.Set(x)
}

// CanScroll reports whether the offset can move by d (in the offset's
// direction) without leaving the range: false at the edge d pushes
// against, and for d == 0. A scroll view passes a delta it cannot take on
// to its parent.
func (a *Axis) CanScroll(d float64) bool {
	x := a.Clamped()
	return (d < 0 && x > 0) || (d > 0 && x < a.max)
}

// Event moves the axis by a scroll event whose delta on this axis is d,
// in shell.ScrollEvent.Delta's direction (positive moves the content
// towards the end: the offset decreases). It reports whether the axis
// took it: a gesture and OS momentum are always taken; a wheel step or a
// precise event outside a gesture only when the offset can move by it.
func (a *Axis) Event(ev shell.ScrollEvent, d float64) bool {
	d = -d // into the offset's direction
	switch {
	case ev.Phase != shell.PhaseNone:
		a.gesture(ev.Phase, ev.Time, d)
		return true
	case ev.Momentum != shell.PhaseNone:
		a.osMomentum(ev.Momentum, ev.Time, d)
		return true
	case ev.Precise:
		if !a.CanScroll(d) {
			return false
		}
		a.mode = modeIdle
		a.pos.Set(a.clamp(a.Clamped() + d))
		return true
	}
	return a.wheel(d * a.p.LineHeight)
}

// gesture handles a precise gesture event.
func (a *Axis) gesture(ph shell.ScrollPhase, t time.Duration, d float64) {
	if ph == shell.PhaseBegan || a.mode != modeDrag {
		// A new gesture, or one whose PhaseBegan went elsewhere: catch
		// the content where it is.
		a.mode = modeDrag
		a.raw = a.unshown(a.pos.Value())
		a.tracker.Reset()
		if ph == shell.PhaseEnded {
			a.release(t)
			return
		}
	}
	if ph == shell.PhaseEnded {
		if d != 0 {
			a.drag(t, d)
		}
		a.release(t)
		return
	}
	a.drag(t, d)
}

func (a *Axis) drag(t time.Duration, d float64) {
	a.raw += d
	if a.tl.ReduceMotion() || a.c() <= 0 {
		a.raw = a.clamp(a.raw)
	}
	a.tracker.Add(t, d)
	a.pos.Set(a.shown(a.raw))
}

// release ends a drag with the tracker's release velocity.
func (a *Axis) release(t time.Duration) {
	a.mode = modeIdle
	if a.tl.ReduceMotion() {
		a.Stop()
		return
	}
	x := a.pos.Value()
	v := a.tracker.End(t) * a.slope(a.raw) // the content's speed, not the fingers'
	over := a.Overscroll()
	switch {
	case over != 0 && (v*over >= 0 || math.Abs(v) <= a.p.StopVelocity):
		a.springBack(v) // outwards, or too slow to come back by itself
	case math.Abs(v) > a.p.StopVelocity && a.p.DecayRate > 0:
		a.mode = modeDecay
		a.pos.Decay(v, a.p.DecayRate, a.p.StopVelocity)
	case over != 0:
		a.springBack(v)
	default:
		a.pos.Set(x)
	}
}

// osMomentum handles an OS momentum event.
func (a *Axis) osMomentum(ph shell.ScrollPhase, t time.Duration, d float64) {
	if a.tl.ReduceMotion() {
		// No momentum: the OS's moves nothing and ends what moves.
		if a.mode != modeIdle {
			a.Stop()
		}
		return
	}
	switch a.mode {
	case modeBounce:
		// The release past an edge is springing back: it ignores the
		// OS momentum that follows.
		if ph != shell.PhaseEnded {
			a.mode = modeOSBounce
		}
		return
	case modeOSBounce:
		if ph == shell.PhaseEnded {
			a.mode = modeBounce // the spring-back runs on
			if !a.pos.Running() {
				a.mode = modeIdle
			}
		}
		return
	case modeOS:
	default:
		if a.mode == modeDecay {
			a.pos.Stop() // the OS's momentum replaces the axis's own
		}
		a.mode = modeOS
	}
	if d != 0 {
		a.tracker.Add(t, d)
	}
	x := a.pos.Value() + d
	switch {
	case x >= 0 && x <= a.max:
		a.pos.Set(x)
	default:
		// Spring back from the edge with the momentum's speed.
		a.pos.Set(a.clamp(x))
		a.springBack(a.tracker.Velocity())
		a.mode = modeOSBounce
	}
	if ph == shell.PhaseEnded && a.mode == modeOS {
		a.mode = modeIdle
		if a.Overscroll() != 0 {
			a.springBack(0)
		}
	}
}

// wheel scrolls by d points, animated; further notches extend the target.
func (a *Axis) wheel(d float64) bool {
	base := a.Clamped()
	if a.mode == modeIdle && a.pos.Running() {
		base = a.clamp(a.pos.Target())
	}
	target := a.clamp(base + d)
	if target == base || d == 0 {
		return false
	}
	if a.mode != modeIdle {
		a.Stop()
	}
	a.pos.AnimateTo(target, a.p.WheelStep, a.p.WheelCurve)
	return true
}

// springBack springs the offset back to its nearest edge (the clamped
// offset) with the start velocity v.
func (a *Axis) springBack(v float64) { a.springTo(a.Clamped(), v) }

// springTo springs the offset to the edge with the start velocity v,
// capped so the overshoot stays below c. Without a rubber band (c ≤ 0) or
// with Reduce Motion the offset jumps there.
func (a *Axis) springTo(edge, v float64) {
	c := a.c()
	if a.tl.ReduceMotion() || c <= 0 {
		a.stopAt(edge)
		return
	}
	v = a.capBounce(edge, v)
	a.mode = modeBounce
	a.pos.SpringFrom(edge, v, a.p.Spring)
	if !a.pos.Running() {
		a.mode = modeIdle // already at rest at the edge
	}
}

// bounceLimit is the fraction of c that a spring-back's overshoot stays
// within (capBounce), and the farthest past an edge that a gesture catches
// the content (unshown).
const bounceLimit = 0.9

// capBounce returns the start velocity v of a spring from the shown offset
// to edge, reduced so that the motion's peak (in its actual regime, from
// anim.Motion.Peak) stays within bounceLimit·c, or within where the
// offset already is if that is farther. The peak is not monotonic in v
// (an inward velocity can overshoot inwards), so the bisection keeps the
// largest scale it has seen pass, never one that fails.
func (a *Axis) capBounce(edge, v float64) float64 {
	from := a.pos.Value()
	lim := math.Max(bounceLimit*a.c(), math.Abs(from-edge))
	peak := func(k float64) float64 { return a.p.Spring.Motion(from, edge, k*v).Peak() }
	if v == 0 || peak(1) <= lim {
		return v
	}
	lo, hi := 0.0, 1.0
	for range 60 {
		mid := (lo + hi) / 2
		if peak(mid) <= lim {
			lo = mid
		} else {
			hi = mid
		}
	}
	return lo * v
}

// changed is the value's change func: it watches the decay for the edges
// and the end, then tells the owner.
func (a *Axis) changed(x float64) {
	if a.mode == modeDecay {
		switch over := x - a.clamp(x); {
		case over != 0 && a.pos.Velocity()*over > 0:
			a.springBack(a.pos.Velocity()) // reached an edge with speed left
		case !a.pos.Running():
			a.mode = modeIdle
			if over != 0 {
				a.springBack(0) // came to rest past an edge
			}
		}
	} else if a.mode == modeBounce && !a.pos.Running() {
		a.mode = modeIdle
	}
	if a.onChange != nil {
		a.onChange(x)
	}
}

// c returns the rubber-band constant in points.
func (a *Axis) c() float64 { return a.p.RubberBand * a.viewport }

func (a *Axis) clamp(x float64) float64 { return math.Max(0, math.Min(a.max, x)) }

// shown maps the fingers' offset to the shown one: 1:1 inside the range,
// rubber-banded past the edges.
func (a *Axis) shown(raw float64) float64 {
	c := a.c()
	switch {
	case raw < 0:
		return RubberBand(raw, c)
	case raw > a.max:
		return a.max + RubberBand(raw-a.max, c)
	}
	return raw
}

// unshown is the inverse of shown, for catching overscrolled content.
// Content shown c or more past an edge (the document shrank under a
// spring-back by more than c) has no inverse; it is caught bounceLimit·c
// past the edge, so the next drag event moves it there rather than to the
// edge.
func (a *Axis) unshown(x float64) float64 {
	c := a.c()
	inv := func(d float64) float64 {
		if math.Abs(d) >= c {
			d = math.Copysign(bounceLimit*c, d)
		}
		return RubberBandInverse(d, c)
	}
	switch {
	case x < 0:
		return inv(x)
	case x > a.max:
		return a.max + inv(x-a.max)
	}
	return x
}

// slope returns d shown / d raw at the fingers' offset raw: 1 inside the
// range, the rubber band's slope c²/(d+c)² past an edge.
func (a *Axis) slope(raw float64) float64 {
	c := a.c()
	var d float64
	switch {
	case raw < 0:
		d = -raw
	case raw > a.max:
		d = raw - a.max
	default:
		return 1
	}
	if c <= 0 {
		return 0
	}
	return c * c / ((d + c) * (d + c))
}
