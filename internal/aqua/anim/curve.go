package anim

import (
	"math"

	"github.com/radix29/gogui/internal/aqua/theme"
)

// Curve is a cubic-bezier timing curve from (0, 0) to (1, 1), as in CSS
// cubic-bezier() and CAMediaTimingFunction: the same values as theme.Curve,
// so a token converts with Curve(th.Animation.EaseOut).
//
// X1 and X2 must be in 0..1 (then x is monotonic in the curve parameter and
// At is a function); Y1 and Y2 may lie outside 0..1, and At then overshoots.
type Curve theme.Curve

// Linear is the identity timing curve: At(t) == t.
var Linear = Curve{0, 0, 1, 1}

// At returns the curve's progress for the time fraction t: y at the point
// whose x is t. t is clamped to 0..1 (NaN counts as 0), so At(0) is 0 and
// At(1) is 1 exactly.
//
// The curve parameter is found by Newton's method from s = t; when Newton
// does not converge (a flat derivative, a step out of 0..1) it falls back
// to bisection, which always converges because x is monotonic. Where x is
// flat and y steep (X1 = 0 with Y1 = 1, say) the result is limited by the
// conditioning of the curve in float64, not by the solver.
func (c Curve) At(t float64) float64 {
	if !(t > 0) {
		return 0
	}
	if t >= 1 {
		return 1
	}
	return bezier(c.Y1, c.Y2, c.solve(t))
}

// solve returns the curve parameter s in 0..1 with x(s) = t. Convergence is
// measured on s, not on x: where x is flat in s a tiny error in x is a large
// error in s, and so in y.
func (c Curve) solve(t float64) float64 {
	s := t
	for range 16 {
		d := bezierSlope(c.X1, c.X2, s)
		if d < 1e-3 {
			break // flat: Newton's step is unreliable
		}
		step := (bezier(c.X1, c.X2, s) - t) / d
		s -= step
		if s < 0 || s > 1 {
			break
		}
		if math.Abs(step) < 1e-14 {
			return s
		}
	}
	return c.bisect(t)
}

// bisect solves x(s) = t on 0..1 by bisection, down to the float64
// resolution of s near 1.
func (c Curve) bisect(t float64) float64 {
	lo, hi := 0.0, 1.0
	for hi-lo > 1e-15 {
		mid := (lo + hi) / 2
		if bezier(c.X1, c.X2, mid) < t {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

// bezier returns one coordinate of the cubic bezier with the end values 0
// and 1 and the control values p1 and p2 at the parameter s:
// 3(1−s)²s·p1 + 3(1−s)s²·p2 + s³, in Horner form.
func bezier(p1, p2, s float64) float64 {
	a := 1 + 3*p1 - 3*p2
	b := 3*p2 - 6*p1
	return ((a*s+b)*s + 3*p1) * s
}

// bezierSlope returns the derivative of bezier with respect to s.
func bezierSlope(p1, p2, s float64) float64 {
	a := 1 + 3*p1 - 3*p2
	b := 3*p2 - 6*p1
	return (3*a*s+2*b)*s + 3*p1
}

// Slope returns the curve's derivative dy/dx at the time fraction t, so
// an animation along it from a to b over d moves at (b − a)·Slope(t)/d.
// t is clamped like At.
//
// Where x is flat in the curve parameter (an end with X1 = 0 or X2 = 1)
// dy/dx is the limit of the ratio of the first derivatives in the curve
// parameter that do not vanish there (l'Hôpital), so EaseOut {0, 0, 0.58,
// 1} starts at Y2/X2 and EaseIn ends at (1 − Y1)/(1 − X1). Only where y
// leaves the point at a lower order than x (X1 = 0 with Y1 ≠ 0, say) is
// the slope unbounded; Slope then returns 0, as a velocity that cannot be
// followed is better dropped.
func (c Curve) Slope(t float64) float64 {
	_, d := c.AtSlope(t)
	return d
}

// AtSlope returns At(t) and Slope(t) together, solving for the curve
// parameter once (a tween's frame needs both).
func (c Curve) AtSlope(t float64) (y, slope float64) {
	if !(t > 0) {
		t = 0
	}
	t = min(t, 1)
	s := 0.0
	switch {
	case t == 1:
		s, y = 1, 1
	case t > 0:
		s = c.solve(t)
		y = bezier(c.Y1, c.Y2, s)
	}
	return y, c.slopeAt(s)
}

// slopeAt returns dy/dx at the curve parameter s (see Slope).
func (c Curve) slopeAt(s float64) float64 {
	// The first, second and third derivatives in s. The third is
	// constant; if it vanishes for x too, x is not monotonic.
	const flat = 1e-9
	dx := [3]float64{bezierSlope(c.X1, c.X2, s), bezierCurv(c.X1, c.X2, s), bezierJerk(c.X1, c.X2)}
	dy := [3]float64{bezierSlope(c.Y1, c.Y2, s), bezierCurv(c.Y1, c.Y2, s), bezierJerk(c.Y1, c.Y2)}
	for k := range dx {
		if math.Abs(dx[k]) >= flat {
			return dy[k] / dx[k]
		}
		if math.Abs(dy[k]) >= flat {
			return 0 // unbounded
		}
	}
	return 0
}

// bezierCurv returns the second derivative of bezier with respect to s.
func bezierCurv(p1, p2, s float64) float64 {
	a := 1 + 3*p1 - 3*p2
	b := 3*p2 - 6*p1
	return 6*a*s + 2*b
}

// bezierJerk returns the (constant) third derivative of bezier with
// respect to s.
func bezierJerk(p1, p2 float64) float64 {
	return 6 * (1 + 3*p1 - 3*p2)
}
