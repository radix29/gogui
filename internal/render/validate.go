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

// StrokeWidth returns w as a stroke width: negative, NaN and +Inf widths
// count as 0 (nothing is drawn; no backend can stroke an infinite width).
func StrokeWidth(w float64) float64 { return length(finite(w)) }

// ValidPoint reports whether p is finite. A canvas draws nothing for a
// call with a non-finite point (a text origin, a path point) and the
// recorder reports it: a NaN from layout is a bug, not an empty drawing.
func ValidPoint(p Point) bool { return isFinite(p.X) && isFinite(p.Y) }

// ValidRect reports whether r is finite (its far edges included) and has
// a non-negative size. A zero size is valid and draws nothing; a canvas
// draws nothing for a call with an invalid rect (ClipRoundedRect and
// PushBackdropBlur clip everything away) and the recorder reports it.
func ValidRect(r Rect) bool {
	return isFinite(r.X) && isFinite(r.Y) && r.W >= 0 && r.H >= 0 &&
		isFinite(r.X+r.W) && isFinite(r.Y+r.H)
}

// ValidPath reports whether every point of p is finite (ValidPoint). A
// nil or empty path is valid.
func ValidPath(p *Path) bool {
	if p == nil {
		return true
	}
	for _, q := range p.pts {
		if !ValidPoint(q) {
			return false
		}
	}
	return true
}

func isFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

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

// stopsNormal reports whether stops are already clamped to [0, 1] and
// sorted, so normalizeStops would return an equal slice.
func stopsNormal(stops []GradientStop) bool {
	prev := 0.0
	for _, s := range stops {
		if !(s.Offset >= prev) || s.Offset > 1 { // NaN fails the first test
			return false
		}
		prev = s.Offset
	}
	return true
}

// normalizeStops returns stops sorted with every offset clamped to [0, 1]:
// stops itself when they already are, else a sorted copy. The sort is
// stable, so stops at the same offset keep their order (a hard color
// change).
func normalizeStops(stops []GradientStop) []GradientStop {
	if len(stops) == 0 {
		return nil
	}
	if stopsNormal(stops) {
		return stops
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

// Normalized returns p with its gradient stops clamped and sorted and
// non-finite gradient end points set to 0. Stops that already are clamped
// and sorted are returned as they are (the result shares p's slice, so a
// canvas normalizing a paint per call allocates nothing); otherwise the
// result has a sorted copy and p's slice is not changed. A solid paint is
// returned unchanged except that Stops is dropped.
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
// becomes 0. A dash pattern that already follows these rules is returned
// as it is (the result shares s's slice); otherwise the result has a new
// one and s's is not changed.
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
	sum, normal := 0.0, len(s.Dash)%2 == 0
	for _, d := range s.Dash {
		if !(d >= 0) || math.IsInf(d, 1) {
			normal = false
			continue // counts as 0
		}
		sum += d
	}
	switch {
	case sum == 0:
		out.Dash, out.DashOffset = nil, 0
	case normal:
		out.Dash = s.Dash
	default:
		out.Dash = make([]float64, len(s.Dash), 2*len(s.Dash))
		for i, d := range s.Dash {
			if d = length(d); !math.IsInf(d, 1) {
				out.Dash[i] = d
			}
		}
		if len(out.Dash)%2 == 1 {
			out.Dash = append(out.Dash, out.Dash...)
		}
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
