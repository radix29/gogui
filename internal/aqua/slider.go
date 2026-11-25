package aqua

import (
	"math"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
)

// Slider is the aqua driver.Slider (PLAN.md S3.4c, S3.4d): a track with a
// round knob, drawn from the theme. The embedded stub slider keeps the
// state (range, value, tick marks, snapping, orientation, control size)
// and the UserDrag simulation.
//
//   - Size: FittingSize is the stub's default length (96 pt) along the
//     slider, and across it the knob (SliderKnobSize of the control size)
//     plus, with tick marks, SliderTickGap and SliderTickLength, rounded
//     up to whole points. A vertical slider turns it. A stub Fixed size
//     still wins.
//   - Layout: the knob and the marks form a band as wide as that across
//     size, centred across the frame; the tick marks are below the knob
//     (horizontal) or after it (vertical, on the trailing side). The track
//     runs the frame's whole length, SliderTrackThickness thick, centred on
//     the knob's line. The knob's centre travels from half a knob in from
//     one end to half a knob in from the other: the minimum is at the left
//     (horizontal) or the bottom (vertical). An empty range (max ≤ min)
//     puts the knob at the minimum. n tick marks are spread evenly over
//     the travel (one mark sits in the middle). The knob is at most as big
//     as the frame across and along.
//   - Paint: the track in SliderTrack, its part from the minimum to the
//     knob's centre in Theme.ValueFill (the accent; AccentInactive, grey,
//     in an inactive window), with Increase Contrast the SliderTrackOutline
//     around the whole track (S6.1e), the tick marks in SliderTickMark (each
//     SliderTickWidth wide, with round ends), then the knob: its shadow
//     (SliderKnobShadow*), its fill (SliderKnobFill; BezelPressedFill while
//     dragged) and its SliderKnobBorder, SliderKnobBorderWidth wide, inside
//     it. Rects are pixel-aligned.
//   - Disabled: every color at ValueDisabledAlpha (ValueFill applies it to
//     the filled part), no layer; the pointer and the keys are ignored.
//   - Focus: the focus ring around the knob (FocusControl: Tab reaches it
//     with full keyboard access).
//   - Pointer (S3.4d): a primary press captures the pointer. On the knob
//     it grabs the knob where it was hit, and the value stays as it is
//     until the pointer moves along the slider; on the track (anywhere
//     else in the frame) the knob jumps there first (a handler that
//     disables or hides the slider then ends the press). While the button
//     is held the value follows the pointer along the slider (clamped to
//     the range, dragged off the frame too; the ends of the travel are the
//     exact minimum and maximum) and the knob is drawn pressed. With
//     SnapToTick and tick marks the value snaps to the nearest mark (the
//     last one is the exact maximum). The handler's ValueChanged is called
//     on every change of the value, never for a move that leaves it
//     unchanged. A release or a PointerCancel ends the drag; other buttons
//     are ignored (swallowed during a drag). UserDrag simulates a drag to
//     a value with the same rules.
//   - Keys (S3.4d) while focused, without modifiers: Right and Up step up,
//     Left and Down step down (Step), Page Up and Page Down by PageStep,
//     Home and End go to the minimum and the maximum. They are consumed
//     even at an end of the range. The value changes like a drag
//     (snapping, ValueChanged when it changes; a value within a billionth
//     of the range of an end becomes that end, so steps never stop just
//     short of it). A value that is not a number counts as the minimum.
//   - Accessibility: a slider with the value and the increment and
//     decrement actions; Increment and Decrement perform them with Step.
type Slider struct {
	*stub.Slider
	peerBase
	view *sliderView
}

// NewSlider returns a slider.
func (d *Driver) NewSlider(h driver.SliderHandler) driver.Slider {
	s := &Slider{Slider: stub.NewSlider(h)}
	s.view = &sliderView{d: d, p: s}
	s.view.Init(s.view)
	s.view.SetFocusable(true) // FocusControl: Tab reaches it with full keyboard access
	s.initElement(s.Slider, &s.Slider.Base, s.view)
	return s
}

func (s *Slider) SetRange(min, max float64)     { s.Slider.SetRange(min, max); s.redraw() }
func (s *Slider) SetValue(v float64)            { s.Slider.SetValue(v); s.redraw() }
func (s *Slider) SetVertical(vertical bool)     { s.Slider.SetVertical(vertical); s.redraw() }
func (s *Slider) SetTickMarks(n int, snap bool) { s.Slider.SetTickMarks(n, snap); s.redraw() }
func (s *Slider) SetControlSize(cs driver.ControlSize) {
	s.Slider.SetControlSize(cs)
	s.redraw()
}

// UserDrag simulates the user dragging the knob to v: like a real drag,
// v is clamped and snapped, ValueChanged is called only when the value
// changes, and a slider disabled in the tree ignores it.
func (s *Slider) UserDrag(v float64) {
	if s.view.EnabledInTree() {
		s.userSet(v)
	}
}

// across returns the slider's size across its length: the knob, and with
// tick marks the gap and the marks.
func (s *Slider) across() float64 {
	cs := s.ControlSize
	a := tokens.Controls.SliderKnobSize.At(cs)
	if s.TickMarks > 0 {
		a += tokens.Controls.SliderTickGap.At(cs) + tokens.Controls.SliderTickLength.At(cs)
	}
	return math.Ceil(a)
}

// FittingSize implements driver.Widget: the stub's default length along,
// the knob and the tick marks across, clamped to max; a stub Fixed size
// wins.
func (s *Slider) FittingSize(max driver.Size) driver.Size {
	if !s.Fixed.IsZero() {
		return s.Slider.FittingSize(max)
	}
	stubSize := s.Slider.FittingSize(driver.Size{W: math.Inf(1), H: math.Inf(1)})
	if s.Vertical {
		return driver.Size{W: s.across(), H: stubSize.H}.Min(max)
	}
	return driver.Size{W: stubSize.W, H: s.across()}.Min(max)
}

// fraction returns where the value is in the range, 0 to 1; 0 for an
// empty range and for a value that is not a number.
func (s *Slider) fraction() float64 {
	if !(s.Max > s.Min) || math.IsNaN(s.Val) {
		return 0
	}
	f := (s.Val - s.Min) / (s.Max - s.Min)
	if math.IsNaN(f) {
		return 0
	}
	return clamp01(f)
}

func clamp01(f float64) float64 { return math.Max(0, math.Min(1, f)) }

// valueAt returns the value at the fraction f of the range: exactly the
// minimum at 0 and the maximum at 1.
func (s *Slider) valueAt(f float64) float64 {
	switch {
	case f <= 0:
		return s.Min
	case f >= 1:
		return s.Max
	}
	return s.Min + f*(s.Max-s.Min)
}

// tickValue returns the value of tick mark i of n (n ≥ 1): spread evenly
// from the minimum to the maximum (the last one exactly the maximum), one
// mark in the middle.
func (s *Slider) tickValue(i, n int) float64 {
	if n == 1 {
		return s.valueAt(0.5)
	}
	return s.valueAt(float64(i) / float64(n-1))
}

// snapping reports whether the value snaps to tick marks.
func (s *Slider) snapping() bool { return s.SnapToTick && s.TickMarks > 0 }

// constrain returns v clamped to the range and, when snapping, moved to
// the nearest tick mark. A value within a billionth of the range of an
// end becomes that end, so repeated steps cannot stop just short of it.
// An empty range and a value that is not a number give the minimum.
func (s *Slider) constrain(v float64) float64 {
	if !(s.Max > s.Min) || math.IsNaN(v) {
		return s.Min
	}
	v = math.Max(s.Min, math.Min(s.Max, v))
	if eps := (s.Max - s.Min) * 1e-9; v-s.Min <= eps {
		v = s.Min
	} else if s.Max-v <= eps {
		v = s.Max
	}
	if s.snapping() {
		if s.TickMarks == 1 {
			return s.tickValue(0, 1)
		}
		step := (s.Max - s.Min) / float64(s.TickMarks-1)
		i := math.Round((v - s.Min) / step)
		v = s.tickValue(int(i), s.TickMarks)
	}
	return v
}

// Step returns the step of the arrow keys and of the a11y increment and
// decrement: one tick mark's distance when snapping to two or more marks,
// otherwise a twentieth of the range (0 for an empty range).
func (s *Slider) Step() float64 {
	if !(s.Max > s.Min) {
		return 0
	}
	if s.snapping() && s.TickMarks > 1 {
		return (s.Max - s.Min) / float64(s.TickMarks-1)
	}
	return (s.Max - s.Min) / 20
}

// PageStep returns the step of Page Up and Page Down: a fifth of the
// range, at least Step.
func (s *Slider) PageStep() float64 {
	if !(s.Max > s.Min) {
		return 0
	}
	return math.Max(s.Step(), (s.Max-s.Min)/5)
}

// userSet sets the value from the user's input: constrained (clamped and
// snapped), and reported to the handler and repainted when it changes. It
// reports whether the value changed.
func (s *Slider) userSet(v float64) bool {
	v = s.constrain(v)
	if v == s.Val {
		return false
	}
	s.Val = v
	if s.Handler != nil {
		s.Handler.ValueChanged(v)
	}
	s.redraw()
	return true
}

// sliderView is the element of a Slider.
type sliderView struct {
	Base
	d       *Driver
	p       *Slider
	press   pressTracker // down while dragging; pressed for the whole drag
	grab    float64      // the pointer's offset from the knob's centre along the slider at the press
	pressAt float64      // the pointer's coordinate along the slider at the press
	still   bool         // a press on the knob that has not moved yet
	ring    FocusRing
}

func (e *sliderView) FittingSize(max driver.Size) driver.Size { return e.p.FittingSize(max) }

// sliderGeometry is where the parts of a slider are, in the element's
// coordinates.
type sliderGeometry struct {
	vertical bool
	knobD    float64 // the knob's diameter
	start    float64 // the knob centre's coordinate along the slider at the minimum
	travel   float64 // how far it moves to the maximum (≥ 0); vertical: upwards
	line     float64 // the knob centre's coordinate across the slider
	track    driver.Rect
	trackR   float64 // the track's corner radius
	ticks    float64 // where the tick marks start across the slider
	depth    float64 // the frame's size across the slider
}

// geometry lays out the slider in its bounds (see Slider).
func (e *sliderView) geometry() sliderGeometry {
	b := e.Bounds()
	cs := e.p.ControlSize
	g := sliderGeometry{vertical: e.p.Vertical}
	length, depth := b.W, b.H
	if g.vertical {
		length, depth = b.H, b.W
	}
	across := e.p.across()
	g.knobD = math.Max(0, math.Min(tokens.Controls.SliderKnobSize.At(cs), math.Min(depth, length)))
	band := math.Min(across, depth)
	top := (depth - band) / 2 // the band's start across
	if e.p.TickMarks == 0 {
		top = (depth - g.knobD) / 2
	}
	g.line = top + g.knobD/2
	g.ticks = top + g.knobD + tokens.Controls.SliderTickGap.At(cs)
	g.depth = depth
	g.travel = math.Max(0, length-g.knobD)
	g.start = g.knobD / 2
	if g.vertical {
		g.start = length - g.knobD/2
	}
	t := math.Min(tokens.Controls.SliderTrackThickness.At(cs), depth)
	g.trackR = math.Min(tokens.Controls.SliderTrackRadius.At(cs), t/2)
	g.track = g.rect(0, length, g.line-t/2, t)
	return g
}

// rect returns the rect from along0 to along1 along and from a0, d deep,
// across, turned for a vertical slider.
func (g *sliderGeometry) rect(along0, along1, a0, d float64) driver.Rect {
	lo, hi := math.Min(along0, along1), math.Max(along0, along1)
	if g.vertical {
		return driver.Rect{X: a0, Y: lo, W: d, H: hi - lo}
	}
	return driver.Rect{X: lo, Y: a0, W: hi - lo, H: d}
}

// at returns the coordinate along the slider of the fraction f.
func (g *sliderGeometry) at(f float64) float64 {
	if g.vertical {
		return g.start - f*g.travel
	}
	return g.start + f*g.travel
}

// fractionAt returns the fraction at the coordinate along, clamped to 0…1;
// 0 without travel.
func (g *sliderGeometry) fractionAt(along float64) float64 {
	if g.travel <= 0 {
		return 0
	}
	if g.vertical {
		return clamp01((g.start - along) / g.travel)
	}
	return clamp01((along - g.start) / g.travel)
}

// along returns p's coordinate along the slider.
func (g *sliderGeometry) along(p driver.Point) float64 {
	if g.vertical {
		return p.Y
	}
	return p.X
}

// knob returns the knob's rect (unaligned) for the current value.
func (e *sliderView) knob(g *sliderGeometry) driver.Rect {
	c := g.at(e.p.fraction())
	return g.rect(c-g.knobD/2, c+g.knobD/2, g.line-g.knobD/2, g.knobD)
}

// HandleEvent tracks the focus ring, the drag and the keys. A disabled
// slider drops a drag and handles nothing.
func (e *sliderView) HandleEvent(ev Event) bool {
	e.ring.Track(e, ev)
	if !e.EnabledInTree() {
		e.press.Cancel(e)
		return false
	}
	switch ev := ev.(type) {
	case PointerEvent:
		return e.pointer(ev)
	case KeyEvent:
		return e.key(ev.KeyEvent)
	}
	return false
}

// pointer handles a pointer event of a drag.
func (e *sliderView) pointer(ev PointerEvent) bool {
	switch ev.Kind {
	case PointerDown:
		if ev.Button != shell.ButtonPrimary || e.press.Down() {
			return e.press.Down()
		}
		g := e.geometry()
		e.grab, e.pressAt, e.still = 0, g.along(ev.Pos), false
		onKnob := e.knob(&g).Contains(ev.Pos)
		if onKnob {
			// Grabbed where it was hit: the knob does not move until the
			// pointer does (no value round trip, no ValueChanged).
			e.grab = e.pressAt - g.at(e.p.fraction())
			e.still = true
		} else {
			// The track: the knob jumps there first.
			r := e.routedBy()
			e.follow(ev.Pos)
			if !e.EnabledInTree() || !e.VisibleInTree() || e.routedBy() != r {
				return false // the handler disabled, hid or removed it: no drag
			}
		}
		e.press.Begin(e)
		return true
	case PointerMove:
		if e.press.Down() && !(e.still && e.alongOf(ev.Pos) == e.pressAt) {
			e.still = false
			e.follow(ev.Pos)
		}
		return e.press.Down()
	case PointerUp:
		if !e.press.Down() || ev.Button != shell.ButtonPrimary {
			return e.press.Down()
		}
		e.press.End(e)
		return true
	case PointerCancel:
		e.press.Cancel(e)
	}
	return false
}

// follow moves the value to the pointer at p (minus the grab offset).
func (e *sliderView) follow(p driver.Point) {
	g := e.geometry()
	f := g.fractionAt(g.along(p) - e.grab)
	e.p.userSet(e.p.valueAt(f))
}

// key handles a key-down while focused (see Slider).
func (e *sliderView) key(ev shell.KeyEvent) bool {
	if !ev.Down || ev.Mods != 0 {
		return false
	}
	s := e.p
	switch ev.Key {
	case driver.KeyRight, driver.KeyUp:
		s.userSet(s.Val + s.Step())
	case driver.KeyLeft, driver.KeyDown:
		s.userSet(s.Val - s.Step())
	case driver.KeyPageUp:
		s.userSet(s.Val + s.PageStep())
	case driver.KeyPageDown:
		s.userSet(s.Val - s.PageStep())
	case driver.KeyHome:
		s.userSet(s.Min)
	case driver.KeyEnd:
		s.userSet(s.Max)
	default:
		return false
	}
	return true
}

// Increment performs the a11y increment action: the value up by Step.
// It is refused while the slider is disabled.
func (e *sliderView) Increment() bool {
	if !e.EnabledInTree() {
		return false
	}
	e.p.userSet(e.p.Val + e.p.Step())
	return true
}

// Decrement performs the a11y decrement action: the value down by Step.
// It is refused while the slider is disabled.
func (e *sliderView) Decrement() bool {
	if !e.EnabledInTree() {
		return false
	}
	e.p.userSet(e.p.Val - e.p.Step())
	return true
}

// State returns the slider's control state; it is pressed while dragged.
func (e *sliderView) State() ControlState { return e.press.State(e) }

// Paint draws the track, its filled part, the tick marks, the knob and
// the focus ring around the knob (nothing for an empty frame).
func (e *sliderView) Paint(c render.Canvas) {
	if e.Bounds().Empty() {
		return
	}
	g := e.geometry()
	st := e.State()
	a := appearanceIn(&e.Base, e.d)
	cs := e.p.ControlSize
	scale := scaleIn(&e.Base)
	dim := func(col driver.Color) driver.Color { return dimmed(col, st.Disabled) }

	// The track and its filled part.
	knob := pixelAlign(&e.Base, e.knob(&g), scale)
	if track := pixelAlign(&e.Base, g.track, scale); !track.Empty() {
		radii := render.UniformRadii(g.trackR)
		c.FillRoundedRect(track, radii, render.Solid(dim(tokens.Controls.SliderTrack.Value(a))))
		var fill driver.Rect
		if g.vertical {
			mid := knob.Y + knob.H/2
			fill = driver.Rect{X: track.X, Y: mid, W: track.W, H: track.Y + track.H - mid}
		} else {
			fill = driver.Rect{X: track.X, Y: track.Y, W: knob.X + knob.W/2 - track.X, H: track.H}
		}
		if !fill.Empty() {
			c.FillRoundedRect(fill, radii, render.Solid(tokens.ValueFill(a, st.Key, st.Disabled)))
		}
		paintOutline(c, track, g.trackR, scale, a, dim(tokens.Controls.SliderTrackOutline.Value(a)))
	}

	// The tick marks, unless they would leave the frame.
	if n, tl := e.p.TickMarks, tokens.Controls.SliderTickLength.At(cs); n > 0 && g.ticks+tl <= g.depth {
		tw := tokens.Controls.SliderTickWidth.At(cs)
		col := render.Solid(dim(tokens.Controls.SliderTickMark.Value(a)))
		for i := range n {
			f := 0.5
			if n > 1 {
				f = float64(i) / float64(n-1)
			}
			at := g.at(f)
			r := pixelAlign(&e.Base, g.rect(at-tw/2, at+tw/2, g.ticks, tl), scale)
			if !r.Empty() {
				c.FillRoundedRect(r, render.UniformRadii(min(r.W, r.H)/2), col)
			}
		}
	}

	// The knob.
	if knob.Empty() {
		return
	}
	round := render.UniformRadii(min(knob.W, knob.H) / 2)
	c.DrawShadow(knob, round, render.Shadow{
		Offset: driver.Point{Y: tokens.Controls.SliderKnobShadowOffsetY.At(cs)},
		Blur:   tokens.Controls.SliderKnobShadowBlur.At(cs),
		Color:  dim(tokens.Controls.SliderKnobShadow.Value(a)),
	})
	fill := tokens.Controls.SliderKnobFill.Value(a)
	if st.Interaction == InteractionPressed {
		fill = tokens.Controls.BezelPressedFill.Value(a)
	}
	c.FillRoundedRect(knob, round, render.Solid(dim(fill)))
	if bw := tokens.Controls.SliderKnobBorderWidth; bw > 0 && knob.W > 2*bw && knob.H > 2*bw {
		in := knob.Inset(driver.UniformInsets(bw / 2))
		c.StrokeRoundedRect(in, render.UniformRadii(min(in.W, in.H)/2), bw,
			render.Solid(dim(tokens.Controls.SliderKnobBorder.Value(a))))
	}
	e.ring.Paint(c, e, RectFocusShape(knob, round))
}

// AccessibleNode implements Element: a slider with its value and the
// increment and decrement actions.
func (e *sliderView) AccessibleNode() a11y.Node {
	return a11y.Node{Role: a11y.RoleSlider, Value: fmtNumber(e.p.Val),
		Actions: a11y.ActionIncrement | a11y.ActionDecrement}
}

// alongOf returns p's coordinate along the slider.
func (e *sliderView) alongOf(p driver.Point) float64 {
	g := e.geometry()
	return g.along(p)
}
