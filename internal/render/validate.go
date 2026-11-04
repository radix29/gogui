package render

import (
	"math"
	"slices"
)

// This file holds every validation rule of the package (PLAN.md S2.1a), so
// that backends and the recorder agree on what a value means.

// length returns v as a non-negative length: negative values and NaN count
// as 0, +Inf stays +Inf.
func length(v float64) float64 {
	if !(v > 0) { // also catches NaN
		return 0
	}
	return v
}

// finite returns v, or 0 when v is NaN or ±Inf. It is used for signed
// values such as offsets and spread, where no sign rule applies.
func finite(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

// unit clamps v to [0, 1]. NaN counts as 0.
func unit(v float64) float64 {
	switch {
	case !(v > 0):
		return 0
	case v > 1:
		return 1
	}
	return v
}

// StrokeWidth returns w as a stroke width: negative values and NaN count as
// 0 (nothing is drawn).
func StrokeWidth(w float64) float64 { return length(w) }

// Opacity returns alpha as a layer opacity: clamped to [0, 1], NaN counts as
// 0 (the layer draws nothing).
func Opacity(alpha float64) float64 { return unit(alpha) }

// BlurRadius returns r as a blur radius: negative, NaN and +Inf radii count
// as 0 (no blur), as for Shadow.Blur.
func BlurRadius(r float64) float64 { return length(finite(r)) }

// Clamped returns rd fitted to r: every radius becomes a non-negative length
// of at most half the shorter side of r (the CSS/D2D rule), so adjacent
// corners never overlap. A rect with no area gets zero radii.
func (rd Radii) Clamped(r Rect) Radii {
	limit := math.Min(length(r.W), length(r.H)) / 2
	c := func(v float64) float64 { return math.Min(length(v), limit) }
	return Radii{
		TopLeft:     c(rd.TopLeft),
		TopRight:    c(rd.TopRight),
		BottomRight: c(rd.BottomRight),
		BottomLeft:  c(rd.BottomLeft),
	}
}

// normalizeStops returns a sorted copy of stops with every offset clamped to
// [0, 1]. The sort is stable, so stops at the same offset keep their order
// (a hard color change).
func normalizeStops(stops []GradientStop) []GradientStop {
	if len(stops) == 0 {
		return nil
	}
	out := make([]GradientStop, len(stops))
	for i, s := range stops {
		out[i] = GradientStop{Offset: unit(s.Offset), Color: s.Color}
	}
	slices.SortStableFunc(out, func(a, b GradientStop) int {
		switch {
		case a.Offset < b.Offset:
			return -1
		case a.Offset > b.Offset:
			return 1
		}
		return 0
	})
	return out
}

// Normalized returns p with its gradient stops clamped and sorted (a new
// slice; p's is not changed) and non-finite gradient end points set to 0.
// A solid paint is returned unchanged except that Stops is dropped.
func (p Paint) Normalized() Paint {
	if p.Kind != PaintLinear {
		return Paint{Kind: PaintSolid, Color: p.Color}
	}
	return Paint{
		Kind:  PaintLinear,
		Start: Point{X: finite(p.Start.X), Y: finite(p.Start.Y)},
		End:   Point{X: finite(p.End.X), Y: finite(p.End.Y)},
		Stops: normalizeStops(p.Stops),
	}
}

// Normalized returns s with the validation rules applied: an unknown cap or
// join becomes the default, a miter limit below 1 (or NaN) becomes
// DefaultMiterLimit, negative or NaN dash entries count as 0, a dash pattern
// whose entries are all 0 means a solid line (nil), an odd-length pattern is
// repeated once to make it even (the SVG rule), and a non-finite dash offset
// becomes 0. The dash slice is always a copy.
func (s StrokeStyle) Normalized() StrokeStyle {
	out := StrokeStyle{Cap: s.Cap, Join: s.Join, MiterLimit: s.MiterLimit, DashOffset: finite(s.DashOffset)}
	if out.Cap > CapSquare {
		out.Cap = CapButt
	}
	if out.Join > JoinBevel {
		out.Join = JoinMiter
	}
	if !(out.MiterLimit >= 1) || math.IsInf(out.MiterLimit, 1) {
		out.MiterLimit = DefaultMiterLimit
	}
	sum := 0.0
	for _, d := range s.Dash {
		d = length(d)
		if math.IsInf(d, 1) {
			d = 0
		}
		sum += d
		out.Dash = append(out.Dash, d)
	}
	switch {
	case sum == 0:
		out.Dash, out.DashOffset = nil, 0
	case len(out.Dash)%2 == 1:
		out.Dash = append(out.Dash, out.Dash...)
	}
	return out
}

// Normalized returns s with a negative or NaN blur counted as 0 and
// non-finite offsets and spread set to 0. Spread may be negative (it shrinks
// the shadow shape, as in CSS).
func (s Shadow) Normalized() Shadow {
	return Shadow{
		Offset: Point{X: finite(s.Offset.X), Y: finite(s.Offset.Y)},
		Blur:   length(finite(s.Blur)),
		Spread: finite(s.Spread),
		Color:  s.Color,
		Inset:  s.Inset,
	}
}
