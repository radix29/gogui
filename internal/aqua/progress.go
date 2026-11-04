package aqua

import (
	"math"
	"time"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/render"
)

// Progress is the aqua driver.Progress (PLAN.md S3.4e, S3.4f): a progress
// bar or a spinner, drawn from the theme. The embedded stub keeps the
// state (style, range, value, indeterminate, control size). One peer
// serves both public widgets, ProgressBar and Spinner.
//
//   - Size: a bar's FittingSize is the stub's default length (96 pt) wide
//     and ProgressThickness of the control size high, a spinner's is
//     SpinnerSize square, both rounded up to whole points and clamped. A
//     stub Fixed size still wins.
//   - Bar: the track runs the frame's whole width, ProgressThickness high
//     (at most the frame's height) with ProgressRadius corners, centred
//     vertically, in ProgressTrack. A determinate bar fills it from the
//     left to the value's fraction of the range in Theme.ValueFill (the
//     accent; AccentInactive, grey, in an inactive window). An empty range
//     and a value that is not a number fill nothing. Rects are
//     pixel-aligned.
//   - Spinner: SpinnerSpokes spokes around the centre, the first at twelve
//     o'clock and the others clockwise, each SpinnerSpokeLength long from
//     the outer edge inwards and SpinnerSpokeWidth wide with round caps.
//     The spinner is SpinnerSize across, centred; a smaller frame scales
//     it down to fit. A determinate spinner draws the first
//     ⌊fraction × spokes⌋ spokes in SpinnerSpoke and the others at
//     SpinnerTailAlpha of it, so it fills clockwise as the value grows.
//   - Indeterminate (S3.4f): a bar's band, ProgressBandFraction of the
//     track wide, in ValueFill, crosses the track from left to right once
//     per ProgressBandPeriod (entering at the left edge, leaving at the
//     right, clipped to the track). A spinner's leading spoke goes round
//     clockwise once per SpinnerPeriod, the spokes behind it fading
//     linearly to SpinnerTailAlpha. The phase is an anim.Value.Loop on the
//     window's timeline (see progressView.animate for when it runs). With
//     Reduce Motion the look is static: the bar's whole track filled with
//     ValueFill at ValueDisabledAlpha, the spinner's fade at rest with the
//     leading spoke at twelve o'clock.
//   - Disabled: every color at ValueDisabledAlpha (ValueFill applies it to
//     the filled part), no layer.
//   - Accessibility: a progress indicator with the value; no value while
//     indeterminate. It takes no input and no focus.
type Progress struct {
	*stub.Progress
	peerBase
	view *progressView
}

// NewProgress returns a progress indicator, a bar until SetStyle.
func (d *Driver) NewProgress() driver.Progress {
	p := &Progress{Progress: stub.NewProgress()}
	p.view = &progressView{d: d, p: p}
	p.view.Init(p.view)
	p.initElement(p.Progress, &p.Progress.Base, p.view)
	return p
}

func (p *Progress) SetStyle(s driver.ProgressStyle) {
	p.Progress.SetStyle(s)
	p.view.animate()
	p.redraw()
}
func (p *Progress) SetRange(min, max float64) { p.Progress.SetRange(min, max); p.redraw() }
func (p *Progress) SetValue(v float64)        { p.Progress.SetValue(v); p.redraw() }
func (p *Progress) SetControlSize(cs driver.ControlSize) {
	p.Progress.SetControlSize(cs)
	p.redraw()
}

// SetIndeterminate implements driver.Progress: the loop starts or stops
// at once.
func (p *Progress) SetIndeterminate(on bool) {
	p.Progress.SetIndeterminate(on)
	p.view.animate()
	p.redraw()
}

// SetHidden implements driver.Widget: hiding the indicator stops its loop
// at once, showing it starts it again.
func (p *Progress) SetHidden(hidden bool) {
	p.peerBase.SetHidden(hidden)
	p.view.animate()
}

// FittingSize implements driver.Widget (see Progress).
func (p *Progress) FittingSize(max driver.Size) driver.Size {
	if !p.Fixed.IsZero() {
		return p.Progress.FittingSize(max)
	}
	cs := p.ControlSize
	if p.Style == driver.ProgressSpinner {
		s := math.Ceil(tokens.Controls.SpinnerSize.At(cs))
		return driver.Size{W: s, H: s}.Min(max)
	}
	stubSize := p.Progress.FittingSize(driver.Size{W: math.Inf(1), H: math.Inf(1)})
	return driver.Size{W: stubSize.W, H: math.Ceil(tokens.Controls.ProgressThickness.At(cs))}.Min(max)
}

// fraction returns where the value is in the range, 0 to 1; 0 for an
// empty range and for a value that is not a number.
func (p *Progress) fraction() float64 {
	if !(p.Max > p.Min) || math.IsNaN(p.Value) {
		return 0
	}
	f := (p.Value - p.Min) / (p.Max - p.Min)
	if math.IsNaN(f) {
		return 0
	}
	return clamp01(f)
}

// period returns the loop's period for the style.
func (p *Progress) period() time.Duration {
	if p.Style == driver.ProgressSpinner {
		return tokens.Controls.SpinnerPeriod
	}
	return tokens.Controls.ProgressBandPeriod
}

// progressView is the element of a Progress.
type progressView struct {
	Base
	d        *Driver
	p        *Progress
	loop     *anim.Value    // the indeterminate phase; nil before the first loop
	tl       *anim.Timeline // loop's timeline
	phase    float64        // the phase drawn, 0 to 1
	painting bool           // inside Paint: a phase change needs no repaint
}

func (e *progressView) FittingSize(max driver.Size) driver.Size { return e.p.FittingSize(max) }

// live reports whether the indeterminate loop should run: the indicator is
// indeterminate, visible in the tree, and in a shown window with a
// timeline.
func (e *progressView) live() bool {
	if !e.p.Indeterminate || !e.VisibleInTree() {
		return false
	}
	w := e.Window()
	return w != nil && w.Shown() && w.Timeline() != nil
}

// animate starts or stops the loop for the current state (S3.4f). The
// loop runs only while live; it stops at once when the indicator itself
// is hidden or made determinate (the peer calls animate), and at the next
// frame when an ancestor is hidden, the element leaves its window or the
// window closes (phaseChanged checks live on every frame). It starts again
// from the next paint that finds it live (Paint calls animate: a hidden
// subtree is not painted, a shown one is, so a returning indicator and a
// window shown again restart it). Reduce Motion stops it (the timeline);
// when it is turned off every window repaints and Paint restarts it. A
// style change retimes it to the other period.
func (e *progressView) animate() {
	if !e.live() {
		if e.loop != nil {
			e.loop.Stop()
		}
		return
	}
	tl := e.Window().Timeline()
	if e.loop == nil || e.tl != tl {
		if e.loop != nil {
			e.loop.Stop()
		}
		e.tl = tl
		e.loop = tl.NewValue(0, e.phaseChanged)
	}
	e.loop.Loop(e.p.period())
}

// phaseChanged is the loop's change func: it stops a loop that is no
// longer live and repaints otherwise (not from inside Paint, which draws
// the new phase anyway).
func (e *progressView) phaseChanged(x float64) {
	e.phase = x
	if !e.live() {
		e.loop.Stop()
		return
	}
	if !e.painting {
		e.Invalidate()
	}
}

// Running reports whether the indeterminate loop runs.
func (e *progressView) Running() bool { return e.loop != nil && e.loop.Running() }

// Paint draws the bar or the spinner (nothing for an empty frame). An
// indeterminate indicator first (re)starts its loop when it is live.
func (e *progressView) Paint(c render.Canvas) {
	if e.Bounds().Empty() {
		return
	}
	if e.p.Indeterminate {
		e.painting = true
		e.animate()
		e.painting = false
	}
	a := appearanceIn(&e.Base, e.d)
	key := true
	if w := e.Window(); w != nil {
		key = w.IsKey()
	}
	disabled := !e.EnabledInTree()
	if e.p.Style == driver.ProgressSpinner {
		e.paintSpinner(c, a, disabled)
		return
	}
	e.paintBar(c, a, key, disabled)
}

// dimmed returns col at ValueDisabledAlpha when disabled.
func dimmed(col driver.Color, disabled bool) driver.Color {
	if disabled {
		col.A *= float32(tokens.Controls.ValueDisabledAlpha)
	}
	return col
}

// paintBar draws a progress bar.
func (e *progressView) paintBar(c render.Canvas, a driver.Appearance, key, disabled bool) {
	b := e.Bounds()
	cs := e.p.ControlSize
	scale := scaleIn(&e.Base)
	t := math.Min(tokens.Controls.ProgressThickness.At(cs), b.H)
	track := pixelAlign(&e.Base, driver.Rect{Y: (b.H - t) / 2, W: b.W, H: t}, scale)
	if track.Empty() {
		return
	}
	r := math.Min(tokens.Controls.ProgressRadius.At(cs), math.Min(track.W, track.H)/2)
	c.FillRoundedRect(track, render.UniformRadii(r), render.Solid(dimmed(tokens.Controls.ProgressTrack.Value(a), disabled)))
	fill := tokens.ValueFill(a, key, disabled)

	if !e.p.Indeterminate {
		w := snapPixel(track.W*e.p.fraction(), scale)
		if w > 0 {
			c.FillRoundedRect(driver.Rect{X: track.X, Y: track.Y, W: w, H: track.H},
				render.UniformRadii(math.Min(r, w/2)), render.Solid(fill))
		}
		return
	}
	if a.ReduceMotion {
		fill.A *= float32(tokens.Controls.ValueDisabledAlpha)
		c.FillRoundedRect(track, render.UniformRadii(r), render.Solid(fill))
		return
	}
	// The band enters at the left edge at phase 0 and has left at the
	// right edge at phase 1.
	bw := snapPixel(track.W*tokens.Controls.ProgressBandFraction, scale)
	if bw <= 0 {
		return
	}
	x := track.X - bw + snapPixel(e.phase*(track.W+bw), scale)
	c.Save()
	c.ClipRoundedRect(track, render.UniformRadii(r))
	c.FillRoundedRect(driver.Rect{X: x, Y: track.Y, W: bw, H: track.H},
		render.UniformRadii(math.Min(r, bw/2)), render.Solid(fill))
	c.Restore()
}

// paintSpinner draws a spinner.
func (e *progressView) paintSpinner(c render.Canvas, a driver.Appearance, disabled bool) {
	b := e.Bounds()
	cs := e.p.ControlSize
	size := tokens.Controls.SpinnerSize.At(cs)
	d := math.Min(size, math.Min(b.W, b.H))
	if !(d > 0) || !(size > 0) {
		return
	}
	k := d / size
	length := tokens.Controls.SpinnerSpokeLength.At(cs) * k
	width := tokens.Controls.SpinnerSpokeWidth.At(cs) * k
	n := max(1, int(math.Round(tokens.Controls.SpinnerSpokes)))
	outer := math.Max(0, d/2-width/2)          // the outer cap's centre
	inner := math.Max(0, outer-(length-width)) // the inner cap's centre
	cx, cy := b.W/2, b.H/2
	col := dimmed(tokens.Controls.SpinnerSpoke.Value(a), disabled)
	tail := tokens.Controls.SpinnerTailAlpha

	alpha := e.spokeAlphas(n, tail, a.ReduceMotion)
	for i := range n {
		th := 2 * math.Pi * float64(i) / float64(n)
		sx, sy := math.Sin(th), -math.Cos(th)
		pt := func(r float64) driver.Point {
			return driver.Point{X: roundCoord(cx + r*sx), Y: roundCoord(cy + r*sy)}
		}
		path := (&render.Path{}).MoveTo(pt(inner)).LineTo(pt(outer))
		sc := col
		sc.A *= float32(alpha[i])
		c.StrokePath(path, width, render.Solid(sc), render.StrokeStyle{Cap: render.CapRound})
	}
}

// spokeAlphas returns the opacity of each of n spokes relative to
// SpinnerSpoke (see Progress).
func (e *progressView) spokeAlphas(n int, tail float64, reduce bool) []float64 {
	out := make([]float64, n)
	if !e.p.Indeterminate {
		lit := int(math.Floor(e.p.fraction()*float64(n) + 1e-9))
		for i := range out {
			out[i] = tail
			if i < lit {
				out[i] = 1
			}
		}
		return out
	}
	lead := 0
	if !reduce {
		lead = min(n-1, int(e.phase*float64(n)))
	}
	for i := range out {
		out[i] = 1
		if n > 1 {
			behind := (lead - i + n) % n
			out[i] = 1 - (1-tail)*float64(behind)/float64(n-1)
		}
	}
	return out
}

// roundCoord rounds v to a millionth of a point, so the spokes' ends do
// not carry the sine's rounding noise (1e-16) into the paint dumps.
func roundCoord(v float64) float64 { return math.Round(v*1e6)/1e6 + 0 }

// AccessibleNode implements Element: a progress indicator with the value;
// no value while indeterminate.
func (e *progressView) AccessibleNode() a11y.Node {
	n := a11y.Node{Role: a11y.RoleProgressIndicator}
	if !e.p.Indeterminate {
		n.Value = fmtNumber(e.p.Value)
	}
	return n
}
