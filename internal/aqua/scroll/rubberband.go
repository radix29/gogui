package scroll

import "math"

// RubberBand returns how far the content is shown past an edge when the
// user has dragged it the distance d past that edge (PLAN.md §5.6: offset =
// limit + d·c/(d+c)). c is the rubber-band constant in points: the theme
// token is a fraction of the viewport, and the caller multiplies it by the
// viewport's length on the axis.
//
// For d ≥ 0 the result is d·c/(d+c): 0 at the edge with the slope 1 (so
// the drag continues smoothly from inside the range), strictly increasing,
// and always below c, which it approaches for a very long drag. The
// function is odd: a negative d (past the leading edge) gives the negative
// of RubberBand(−d, c). +Inf gives c.
//
// A c that is not positive (or NaN) means no overscroll: the result is 0.
// A NaN d gives NaN.
func RubberBand(d, c float64) float64 {
	if !(c > 0) {
		return 0
	}
	if d < 0 {
		return -RubberBand(-d, c)
	}
	if math.IsInf(d, 1) {
		return c
	}
	return d * c / (d + c)
}

// RubberBandInverse is the inverse of RubberBand: the drag distance past
// the edge that shows the content x past it, x·c/(c−x) for x ≥ 0 (odd for
// a negative x). A drag that starts on overscrolled content (caught during
// the spring-back) uses it to continue from where the content is, so a
// drag past the edge and back maps 1:1.
//
// RubberBand never reaches c, so |x| ≥ c has no finite preimage: the result
// is ±Inf with the sign of x. A c that is not positive (or NaN) gives 0, as
// RubberBand does. A NaN x gives NaN.
func RubberBandInverse(x, c float64) float64 {
	if !(c > 0) {
		return 0
	}
	if x < 0 {
		return -RubberBandInverse(-x, c)
	}
	if x >= c {
		return math.Inf(1)
	}
	return x * c / (c - x)
}
