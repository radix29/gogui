package anim

// Lerp interpolates linearly from a (t = 0) to b (t = 1). t is not clamped,
// so an overshooting curve or spring extrapolates. The end points are
// exact: Lerp(a, b, 0) == a and Lerp(a, b, 1) == b.
func Lerp(a, b, t float64) float64 {
	return (1-t)*a + t*b
}
