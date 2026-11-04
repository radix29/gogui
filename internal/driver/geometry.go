package driver

import "math"

// Point is a position in logical points. Y grows downwards.
type Point struct {
	X, Y float64
}

// Add returns p translated by q.
func (p Point) Add(q Point) Point { return Point{p.X + q.X, p.Y + q.Y} }

// Sub returns p translated by -q.
func (p Point) Sub(q Point) Point { return Point{p.X - q.X, p.Y - q.Y} }

// Size is a width and a height in logical points.
type Size struct {
	W, H float64
}

// Unbounded returns the size passed to Widget.FittingSize when there is no
// constraint in either direction. Both components are +Inf.
func Unbounded() Size { return Size{math.Inf(1), math.Inf(1)} }

// IsZero reports whether both components are zero.
func (s Size) IsZero() bool { return s.W == 0 && s.H == 0 }

// Empty reports whether s has no area.
func (s Size) Empty() bool { return s.W <= 0 || s.H <= 0 }

// Min returns the component-wise minimum of s and o.
func (s Size) Min(o Size) Size { return Size{math.Min(s.W, o.W), math.Min(s.H, o.H)} }

// Max returns the component-wise maximum of s and o.
func (s Size) Max(o Size) Size { return Size{math.Max(s.W, o.W), math.Max(s.H, o.H)} }

// Inset returns s shrunk by in. Components never go below zero.
func (s Size) Inset(in Insets) Size {
	return Size{
		math.Max(0, s.W-in.Horizontal()),
		math.Max(0, s.H-in.Vertical()),
	}
}

// Outset returns s grown by in.
func (s Size) Outset(in Insets) Size {
	return Size{s.W + in.Horizontal(), s.H + in.Vertical()}
}

// Rect is an axis-aligned rectangle whose origin (X, Y) is the top-left
// corner.
type Rect struct {
	X, Y, W, H float64
}

// RectOf returns the rectangle with origin p and size s.
func RectOf(p Point, s Size) Rect { return Rect{p.X, p.Y, s.W, s.H} }

// Origin returns the top-left corner of r.
func (r Rect) Origin() Point { return Point{r.X, r.Y} }

// Size returns the size of r.
func (r Rect) Size() Size { return Size{r.W, r.H} }

// MaxX returns the right edge of r.
func (r Rect) MaxX() float64 { return r.X + r.W }

// MaxY returns the bottom edge of r.
func (r Rect) MaxY() float64 { return r.Y + r.H }

// Empty reports whether r has no area.
func (r Rect) Empty() bool { return r.W <= 0 || r.H <= 0 }

// Contains reports whether p lies inside r. The left and top edges are
// inside, the right and bottom edges are outside.
func (r Rect) Contains(p Point) bool {
	return p.X >= r.X && p.X < r.MaxX() && p.Y >= r.Y && p.Y < r.MaxY()
}

// Offset returns r translated by d.
func (r Rect) Offset(d Point) Rect { return Rect{r.X + d.X, r.Y + d.Y, r.W, r.H} }

// Inset returns r shrunk by in. The size never goes below zero. When the
// insets are larger than r in one direction, the result collapses to a zero
// extent centered in the remaining space.
func (r Rect) Inset(in Insets) Rect {
	out := Rect{r.X + in.Left, r.Y + in.Top, r.W - in.Horizontal(), r.H - in.Vertical()}
	if out.W < 0 {
		out.X += out.W / 2
		out.W = 0
	}
	if out.H < 0 {
		out.Y += out.H / 2
		out.H = 0
	}
	return out
}

// Intersect returns the overlap of r and o, or the zero Rect if they do not
// overlap.
func (r Rect) Intersect(o Rect) Rect {
	x0, y0 := math.Max(r.X, o.X), math.Max(r.Y, o.Y)
	x1, y1 := math.Min(r.MaxX(), o.MaxX()), math.Min(r.MaxY(), o.MaxY())
	if x1 <= x0 || y1 <= y0 {
		return Rect{}
	}
	return Rect{x0, y0, x1 - x0, y1 - y0}
}

// Insets are distances from the edges of a rectangle, for example padding.
type Insets struct {
	Top, Left, Bottom, Right float64
}

// UniformInsets returns insets of v on every edge.
func UniformInsets(v float64) Insets { return Insets{v, v, v, v} }

// Horizontal returns Left + Right.
func (in Insets) Horizontal() float64 { return in.Left + in.Right }

// Vertical returns Top + Bottom.
func (in Insets) Vertical() float64 { return in.Top + in.Bottom }

// Add returns the edge-wise sum of in and o.
func (in Insets) Add(o Insets) Insets {
	return Insets{in.Top + o.Top, in.Left + o.Left, in.Bottom + o.Bottom, in.Right + o.Right}
}
