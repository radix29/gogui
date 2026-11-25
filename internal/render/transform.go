package render

import "math"

// Transform is a 2×3 affine matrix in the SVG/Direct2D layout:
//
//	x' = A·x + C·y + E
//	y' = B·x + D·y + F
//
// The zero value is not the identity (it maps everything to the origin);
// start from Identity.
type Transform struct {
	A, B, C, D, E, F float64
}

// Identity returns the identity transform.
func Identity() Transform { return Transform{A: 1, D: 1} }

// Translation returns a transform that moves by (dx, dy).
func Translation(dx, dy float64) Transform { return Transform{A: 1, D: 1, E: dx, F: dy} }

// Scaling returns a transform that scales by (sx, sy) around the origin.
func Scaling(sx, sy float64) Transform { return Transform{A: sx, D: sy} }

// Then returns the transform that applies t first and u second.
func (t Transform) Then(u Transform) Transform {
	return Transform{
		A: u.A*t.A + u.C*t.B,
		B: u.B*t.A + u.D*t.B,
		C: u.A*t.C + u.C*t.D,
		D: u.B*t.C + u.D*t.D,
		E: u.A*t.E + u.C*t.F + u.E,
		F: u.B*t.E + u.D*t.F + u.F,
	}
}

// Pre returns the transform that applies u first and t second. A canvas's
// Translate, Scale and Concat use it: the new transform acts in the current
// user space.
func (t Transform) Pre(u Transform) Transform { return u.Then(t) }

// Det returns the determinant of the linear part.
func (t Transform) Det() float64 { return t.A*t.D - t.B*t.C }

// Invert returns the inverse of t. ok is false, and the result the zero
// Transform, when t is singular or not finite.
func (t Transform) Invert() (inv Transform, ok bool) {
	det := t.Det()
	if det == 0 || math.IsNaN(det) || math.IsInf(det, 0) {
		return Transform{}, false
	}
	inv = Transform{
		A: t.D / det,
		B: -t.B / det,
		C: -t.C / det,
		D: t.A / det,
	}
	inv.E = -(inv.A*t.E + inv.C*t.F)
	inv.F = -(inv.B*t.E + inv.D*t.F)
	for _, v := range [...]float64{inv.A, inv.B, inv.C, inv.D, inv.E, inv.F} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return Transform{}, false
		}
	}
	return inv, true
}

// IsIdentity reports whether t is exactly the identity.
func (t Transform) IsIdentity() bool { return t == Identity() }

// IsTranslation reports whether t only translates, so a backend can keep
// rectangles axis-aligned and pixel-snapped.
func (t Transform) IsTranslation() bool { return t.A == 1 && t.B == 0 && t.C == 0 && t.D == 1 }

// Apply maps the point p.
func (t Transform) Apply(p Point) Point {
	return Point{X: t.A*p.X + t.C*p.Y + t.E, Y: t.B*p.X + t.D*p.Y + t.F}
}

// ApplyRect returns the axis-aligned bounding box of r mapped by t. It is
// exact for translations and scalings and a bound for rotations.
func (t Transform) ApplyRect(r Rect) Rect {
	p0 := t.Apply(Point{X: r.X, Y: r.Y})
	minX, minY, maxX, maxY := p0.X, p0.Y, p0.X, p0.Y
	for _, q := range [...]Point{{X: r.MaxX(), Y: r.Y}, {X: r.X, Y: r.MaxY()}, {X: r.MaxX(), Y: r.MaxY()}} {
		p := t.Apply(q)
		minX, maxX = math.Min(minX, p.X), math.Max(maxX, p.X)
		minY, maxY = math.Min(minY, p.Y), math.Max(maxY, p.Y)
	}
	return Rect{X: minX, Y: minY, W: maxX - minX, H: maxY - minY}
}
