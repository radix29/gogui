package gogui

import (
	"math"

	"github.com/radix29/gogui/internal/driver"
)

// valueRange is the range and value of a slider or progress indicator.
// The getters return the clamped values the peer shows.
type valueRange struct {
	min, max, val float64
}

// setRange sets the range. A NaN bound keeps the old one, and a max below
// min becomes min. The value is clamped to the new range.
func (r *valueRange) setRange(min, max float64) {
	if !math.IsNaN(min) {
		r.min = min
	}
	if !math.IsNaN(max) {
		r.max = max
	}
	r.max = math.Max(r.min, r.max)
	r.set(r.val)
}

// set sets the value, clamped to the range. NaN becomes the minimum.
func (r *valueRange) set(v float64) {
	if math.IsNaN(v) {
		v = r.min
	}
	r.val = math.Max(r.min, math.Min(r.max, v))
}

// Slider is a linear slider (NSSlider). The default range is 0…1. A
// horizontal slider (the default) expands horizontally, a vertical one
// vertically.
type Slider struct {
	common[*Slider]
	p        driver.Slider
	r        valueRange
	onChange subscribers[func(float64)]
}

// NewSlider returns a horizontal slider with the range 0…1 at 0.
func NewSlider() *Slider {
	s := &Slider{r: valueRange{max: 1}}
	p := driverFor("NewSlider").NewSlider(sliderEvents{s})
	s.p = p
	s.init(s, kindSlider, p, nil)
	p.SetRange(0, 1)
	p.SetValue(0)
	return s
}

// SetRange sets the minimum and maximum values. A max below min becomes
// min, and a NaN bound is ignored. The value is clamped to the new range.
func (s *Slider) SetRange(min, max float64) *Slider {
	s.check("SetRange")
	s.r.setRange(min, max)
	if s.live() {
		s.p.SetRange(s.r.min, s.r.max)
		s.p.SetValue(s.r.val)
	}
	return s
}

// SetValue sets the value, clamped to the range (NaN becomes the
// minimum). It doesn't call the OnChange handlers.
func (s *Slider) SetValue(v float64) *Slider {
	s.check("SetValue")
	s.r.set(v)
	if s.live() {
		s.p.SetValue(s.r.val)
	}
	return s
}

// Value returns the value, including the user's changes.
func (s *Slider) Value() float64 { s.get("Value"); return s.r.val }

// SetTickMarks shows count tick marks (0 hides them). With snap set, the
// knob only stops at tick marks.
func (s *Slider) SetTickMarks(count int, snap bool) *Slider {
	s.check("SetTickMarks")
	if s.live() {
		s.p.SetTickMarks(max(count, 0), snap)
	}
	s.invalidate()
	return s
}

// SetVertical switches between a horizontal (false, the default) and a
// vertical slider. The slider then expands along its new length.
func (s *Slider) SetVertical(vertical bool) *Slider {
	s.check("SetVertical")
	s.vertical = vertical
	s.applyExpand()
	if s.live() {
		s.p.SetVertical(vertical)
	}
	s.invalidate()
	return s
}

// OnChange calls fn with the new value continuously while the user drags
// the knob, and on keyboard changes. SetValue doesn't call it, and neither
// does a disabled slider.
func (s *Slider) OnChange(fn func(v float64)) Unsubscribe {
	s.check("OnChange")
	return s.onChange.add(fn)
}

// sliderEvents implements driver.SliderHandler for a Slider.
type sliderEvents struct{ s *Slider }

func (e sliderEvents) ValueChanged(v float64) {
	s := e.s
	if s.dead {
		return
	}
	s.r.set(v) // the cache follows the native value even when disabled (B14)
	v = s.r.val
	if s.disabled {
		return
	}
	s.onChange.eachLive(&s.widget, func(fn func(float64)) { fn(v) })
}

// progress is the state of ProgressBar and Spinner (NSProgressIndicator).
// T is the public type, so the setters chain.
type progress[T any] struct {
	common[T]
	p     driver.Progress
	r     valueRange
	indet bool
}

func (g *progress[T]) setup(self T, k kind, what string, style driver.ProgressStyle) {
	p := driverFor(what).NewProgress()
	g.p, g.r = p, valueRange{max: 100}
	g.init(self, k, p, nil)
	p.SetStyle(style)
	p.SetRange(0, 100)
	p.SetValue(0)
}

// SetRange sets the minimum and maximum values (the default is 0…100). A
// max below min becomes min, and a NaN bound is ignored. The value is
// clamped to the new range.
func (g *progress[T]) SetRange(min, max float64) T {
	g.check("SetRange")
	g.r.setRange(min, max)
	if g.live() {
		g.p.SetRange(g.r.min, g.r.max)
		g.p.SetValue(g.r.val)
	}
	return g.self
}

// SetValue sets the progress, clamped to the range (NaN becomes the
// minimum).
func (g *progress[T]) SetValue(v float64) T {
	g.check("SetValue")
	g.r.set(v)
	if g.live() {
		g.p.SetValue(g.r.val)
	}
	return g.self
}

// Value returns the progress.
func (g *progress[T]) Value() float64 { g.get("Value"); return g.r.val }

// SetIndeterminate switches to the indeterminate animation, for work of
// unknown length. The animation runs while the indicator is visible and
// stops when Reduce Motion is on.
func (g *progress[T]) SetIndeterminate(indeterminate bool) T {
	g.check("SetIndeterminate")
	g.indet = indeterminate
	if g.live() {
		g.p.SetIndeterminate(indeterminate)
	}
	return g.self
}

// Indeterminate reports whether the indeterminate animation is on.
func (g *progress[T]) Indeterminate() bool { g.get("Indeterminate"); return g.indet }

// ProgressBar is a horizontal progress bar (NSProgressIndicator with the
// bar style). It expands horizontally.
type ProgressBar struct {
	progress[*ProgressBar]
}

// NewProgressBar returns a determinate progress bar with the range 0…100
// at 0.
func NewProgressBar() *ProgressBar {
	b := &ProgressBar{}
	b.setup(b, kindProgressBar, "NewProgressBar", driver.ProgressBar)
	return b
}

// Spinner is a spinning progress indicator (NSProgressIndicator with the
// spinning style). It doesn't expand.
type Spinner struct {
	progress[*Spinner]
}

// NewSpinner returns a determinate spinner with the range 0…100 at 0.
// Most apps call SetIndeterminate(true).
func NewSpinner() *Spinner {
	s := &Spinner{}
	s.setup(s, kindSpinner, "NewSpinner", driver.ProgressSpinner)
	return s
}
