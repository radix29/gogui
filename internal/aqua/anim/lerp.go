package anim

import "github.com/radix29/gogui/internal/driver"

// Lerp interpolates linearly from a (t = 0) to b (t = 1). t is not clamped,
// so an overshooting curve or spring extrapolates. The end points are
// exact: Lerp(a, b, 0) == a and Lerp(a, b, 1) == b.
func Lerp(a, b, t float64) float64 {
	return (1-t)*a + t*b
}

// LerpColor interpolates each sRGB channel and the straight alpha of two
// concrete colors (resolve semantic colors through the theme first). Each
// channel is clamped to 0..1, so an overshooting t cannot make an invalid
// color. It panics when a or b is semantic.
func LerpColor(a, b driver.Color, t float64) driver.Color {
	if a.Semantic != driver.SemanticNone || b.Semantic != driver.SemanticNone {
		panic("anim: LerpColor needs concrete colors; resolve semantic colors first")
	}
	ch := func(x, y float32) float32 {
		return float32(min(max(Lerp(float64(x), float64(y), t), 0), 1))
	}
	return driver.Color{R: ch(a.R, b.R), G: ch(a.G, b.G), B: ch(a.B, b.B), A: ch(a.A, b.A)}
}

// LerpPoint interpolates both coordinates.
func LerpPoint(a, b driver.Point, t float64) driver.Point {
	return driver.Point{X: Lerp(a.X, b.X, t), Y: Lerp(a.Y, b.Y, t)}
}

// LerpSize interpolates both dimensions; a negative result (an overshoot
// past zero) is clamped to zero.
func LerpSize(a, b driver.Size, t float64) driver.Size {
	return driver.Size{W: max(Lerp(a.W, b.W, t), 0), H: max(Lerp(a.H, b.H, t), 0)}
}

// LerpRect interpolates the origin (as LerpPoint) and the size (as
// LerpSize).
func LerpRect(a, b driver.Rect, t float64) driver.Rect {
	return driver.Rect{
		X: Lerp(a.X, b.X, t),
		Y: Lerp(a.Y, b.Y, t),
		W: max(Lerp(a.W, b.W, t), 0),
		H: max(Lerp(a.H, b.H, t), 0),
	}
}
