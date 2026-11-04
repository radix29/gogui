package render

import (
	"iter"
	"math"
)

// Op is the kind of a path segment.
type Op uint8

// Path segment kinds. A backend sees only these; ArcTo is stored as cubics.
const (
	// OpMoveTo starts a subpath at Pts[0].
	OpMoveTo Op = iota
	// OpLineTo draws a line to Pts[0].
	OpLineTo
	// OpQuadTo draws a quadratic Bézier with control point Pts[0] to Pts[1].
	OpQuadTo
	// OpCubicTo draws a cubic Bézier with control points Pts[0] and Pts[1]
	// to Pts[2].
	OpCubicTo
	// OpClose closes the current subpath with a line back to its start.
	OpClose
)

// points returns how many points a segment of kind op has.
func (op Op) points() int {
	switch op {
	case OpMoveTo, OpLineTo:
		return 1
	case OpQuadTo:
		return 2
	case OpCubicTo:
		return 3
	}
	return 0
}

// String returns the name used in recorder dumps.
func (op Op) String() string {
	switch op {
	case OpMoveTo:
		return "M"
	case OpLineTo:
		return "L"
	case OpQuadTo:
		return "Q"
	case OpCubicTo:
		return "C"
	case OpClose:
		return "Z"
	}
	return "?"
}

// Segment is one element of a path as seen by a backend. Only the first
// Op.points() entries of Pts are used.
type Segment struct {
	Op  Op
	Pts [3]Point
}

// Path is a sequence of subpaths made of lines and Bézier curves, in user
// space (logical points, Y down). The zero value is an empty path ready to
// use. The building methods return p so calls can be chained.
//
// Subpaths follow the HTML canvas rules, so a backend can rely on every
// subpath starting with an OpMoveTo:
//
//   - A drawing call without a current point first moves to its first point
//     (LineTo(q) acts as MoveTo(q); QuadTo and CubicTo move to their first
//     control point; ArcTo to p1).
//   - After Close, the current point is the start of the closed subpath, and
//     the next drawing call starts a new subpath there.
//
// A Path is not safe for concurrent mutation; a finished path may be read
// (Segments, Bounds) from several goroutines.
type Path struct {
	ops []Op
	pts []Point

	// cur is the current point and start the start of the current subpath,
	// both valid only when hasCur. open reports whether the last segment
	// belongs to an unclosed subpath (false right after Close).
	cur, start   Point
	hasCur, open bool
}

// Reset empties p, keeping its storage.
func (p *Path) Reset() {
	p.ops, p.pts = p.ops[:0], p.pts[:0]
	p.hasCur, p.open = false, false
}

// Empty reports whether p has no segments.
func (p *Path) Empty() bool { return len(p.ops) == 0 }

// Len returns the number of segments, including moves and closes.
func (p *Path) Len() int { return len(p.ops) }

// Current returns the current point; ok is false when there is none.
func (p *Path) Current() (pt Point, ok bool) { return p.cur, p.hasCur }

// MoveTo starts a new subpath at q.
func (p *Path) MoveTo(q Point) *Path {
	p.ops = append(p.ops, OpMoveTo)
	p.pts = append(p.pts, q)
	p.cur, p.start, p.hasCur, p.open = q, q, true, true
	return p
}

// ensure makes sure a subpath is open before a drawing call whose first
// point is first.
func (p *Path) ensure(first Point) {
	switch {
	case !p.hasCur:
		p.MoveTo(first)
	case !p.open:
		p.MoveTo(p.cur)
	}
}

// LineTo draws a line from the current point to q.
func (p *Path) LineTo(q Point) *Path {
	if !p.hasCur {
		return p.MoveTo(q)
	}
	p.ensure(q)
	p.ops = append(p.ops, OpLineTo)
	p.pts = append(p.pts, q)
	p.cur = q
	return p
}

// QuadTo draws a quadratic Bézier curve with control point c to q.
func (p *Path) QuadTo(c, q Point) *Path {
	p.ensure(c)
	p.ops = append(p.ops, OpQuadTo)
	p.pts = append(p.pts, c, q)
	p.cur = q
	return p
}

// CubicTo draws a cubic Bézier curve with control points c1 and c2 to q.
func (p *Path) CubicTo(c1, c2, q Point) *Path {
	p.ensure(c1)
	p.ops = append(p.ops, OpCubicTo)
	p.pts = append(p.pts, c1, c2, q)
	p.cur = q
	return p
}

// ArcTo adds a circular arc of the given radius that is tangent to the line
// from the current point to p1 and to the line from p1 to p2, joined to the
// current point by a straight line (HTML canvas arcTo, CGPathAddArcToPoint).
// It is how rounded corners of arbitrary shapes are built.
//
// The arc is stored as at most two cubic Béziers (one per quarter turn or
// less). Without a current point it acts as MoveTo(p1). When the radius is
// zero, negative, NaN or +Inf, or the three points are collinear or coincide, it
// draws a line to p1.
func (p *Path) ArcTo(p1, p2 Point, radius float64) *Path {
	if !p.hasCur {
		return p.MoveTo(p1)
	}
	p.ensure(p1)
	p0 := p.cur
	r := length(radius)
	// Unit vectors from the corner p1 towards p0 and towards p2.
	ax, ay := p0.X-p1.X, p0.Y-p1.Y
	bx, by := p2.X-p1.X, p2.Y-p1.Y
	la, lb := math.Hypot(ax, ay), math.Hypot(bx, by)
	if r == 0 || math.IsInf(r, 1) || la == 0 || lb == 0 {
		return p.LineTo(p1)
	}
	ax, ay, bx, by = ax/la, ay/la, bx/lb, by/lb
	cross := ax*by - ay*bx
	cos := ax*bx + ay*by
	if math.Abs(cross) < 1e-12 {
		return p.LineTo(p1)
	}
	// Half the angle between the two legs; the tangent points lie at
	// distance r/tan(half) from the corner along each leg.
	half := math.Acos(math.Max(-1, math.Min(1, cos))) / 2
	d := r / math.Tan(half)
	t0 := Point{X: p1.X + ax*d, Y: p1.Y + ay*d}
	t1 := Point{X: p1.X + bx*d, Y: p1.Y + by*d}
	// The center lies on the bisector at distance r/sin(half).
	mx, my := ax+bx, ay+by
	ml := math.Hypot(mx, my)
	h := r / math.Sin(half)
	c := Point{X: p1.X + mx/ml*h, Y: p1.Y + my/ml*h}

	p.LineTo(t0)
	a0 := math.Atan2(t0.Y-c.Y, t0.X-c.X)
	a1 := math.Atan2(t1.Y-c.Y, t1.X-c.X)
	sweep := a1 - a0
	// The arc is the short way round (it spans π - 2·half < π).
	for sweep > math.Pi {
		sweep -= 2 * math.Pi
	}
	for sweep < -math.Pi {
		sweep += 2 * math.Pi
	}
	p.arcCubics(c, r, a0, sweep, t1)
	return p
}

// arcCubics appends cubics approximating the circular arc around c of
// radius r from angle a0 through sweep (|sweep| ≤ π), ending exactly at end.
func (p *Path) arcCubics(c Point, r, a0, sweep float64, end Point) {
	n := int(math.Ceil(math.Abs(sweep) / (math.Pi / 2)))
	if n < 1 {
		n = 1
	}
	step := sweep / float64(n)
	k := 4.0 / 3.0 * math.Tan(step/4) // control-point distance factor
	for i := range n {
		s0, c0 := math.Sincos(a0 + float64(i)*step)
		s1, c1 := math.Sincos(a0 + float64(i+1)*step)
		q0 := Point{X: c.X + r*c0, Y: c.Y + r*s0}
		q1 := Point{X: c.X + r*c1, Y: c.Y + r*s1}
		if i == n-1 {
			q1 = end
		}
		p.CubicTo(
			Point{X: q0.X - k*r*s0, Y: q0.Y + k*r*c0},
			Point{X: q1.X + k*r*s1, Y: q1.Y - k*r*c1},
			q1,
		)
	}
}

// Close closes the current subpath with a line back to its start. It does
// nothing when no subpath is open.
func (p *Path) Close() *Path {
	if !p.hasCur || !p.open {
		return p
	}
	p.ops = append(p.ops, OpClose)
	p.cur, p.open = p.start, false
	return p
}

// Segments returns an iterator over the segments of p, in order, for the
// backends. The path must not be changed during the iteration.
func (p *Path) Segments() iter.Seq[Segment] {
	return func(yield func(Segment) bool) {
		i := 0
		for _, op := range p.ops {
			s := Segment{Op: op}
			n := op.points()
			copy(s.Pts[:n], p.pts[i:i+n])
			i += n
			if !yield(s) {
				return
			}
		}
	}
}

// Bounds returns the tight axis-aligned bounding box of p: every on-path
// point (including move targets) and the extrema of the curves, but not
// control points that lie off the curve. An empty path has the zero Rect.
// The box ignores stroke width.
func (p *Path) Bounds() Rect {
	if len(p.pts) == 0 {
		return Rect{}
	}
	b := bbox{minX: math.Inf(1), minY: math.Inf(1), maxX: math.Inf(-1), maxY: math.Inf(-1)}
	var cur Point
	for s := range p.Segments() {
		switch s.Op {
		case OpMoveTo, OpLineTo:
			cur = s.Pts[0]
			b.add(cur)
		case OpQuadTo:
			b.add(s.Pts[1])
			for _, t := range quadExtrema(cur, s.Pts[0], s.Pts[1]) {
				b.add(quadAt(cur, s.Pts[0], s.Pts[1], t))
			}
			cur = s.Pts[1]
		case OpCubicTo:
			b.add(s.Pts[2])
			for _, t := range cubicExtrema(cur, s.Pts[0], s.Pts[1], s.Pts[2]) {
				b.add(cubicAt(cur, s.Pts[0], s.Pts[1], s.Pts[2], t))
			}
			cur = s.Pts[2]
		}
	}
	return Rect{X: b.minX, Y: b.minY, W: b.maxX - b.minX, H: b.maxY - b.minY}
}

type bbox struct{ minX, minY, maxX, maxY float64 }

func (b *bbox) add(q Point) {
	b.minX, b.maxX = math.Min(b.minX, q.X), math.Max(b.maxX, q.X)
	b.minY, b.maxY = math.Min(b.minY, q.Y), math.Max(b.maxY, q.Y)
}

func quadAt(p0, p1, p2 Point, t float64) Point {
	u := 1 - t
	return Point{
		X: u*u*p0.X + 2*u*t*p1.X + t*t*p2.X,
		Y: u*u*p0.Y + 2*u*t*p1.Y + t*t*p2.Y,
	}
}

func cubicAt(p0, p1, p2, p3 Point, t float64) Point {
	u := 1 - t
	a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
	return Point{
		X: a*p0.X + b*p1.X + c*p2.X + d*p3.X,
		Y: a*p0.Y + b*p1.Y + c*p2.Y + d*p3.Y,
	}
}

// quadExtrema returns the parameters in (0, 1) where the quadratic's x or y
// derivative is zero.
func quadExtrema(p0, p1, p2 Point) []float64 {
	var ts []float64
	for _, v := range [2][3]float64{{p0.X, p1.X, p2.X}, {p0.Y, p1.Y, p2.Y}} {
		// B'(t) = 2[(p1-p0) + t(p2-2p1+p0)]
		den := v[0] - 2*v[1] + v[2]
		if den != 0 {
			if t := (v[0] - v[1]) / den; t > 0 && t < 1 {
				ts = append(ts, t)
			}
		}
	}
	return ts
}

// cubicExtrema returns the parameters in (0, 1) where the cubic's x or y
// derivative is zero.
func cubicExtrema(p0, p1, p2, p3 Point) []float64 {
	var ts []float64
	for _, v := range [2][4]float64{{p0.X, p1.X, p2.X, p3.X}, {p0.Y, p1.Y, p2.Y, p3.Y}} {
		// B'(t)/3 = a t² + b t + c
		a := -v[0] + 3*v[1] - 3*v[2] + v[3]
		b := 2 * (v[0] - 2*v[1] + v[2])
		c := v[1] - v[0]
		ts = appendQuadRoots(ts, a, b, c)
	}
	return ts
}

// appendQuadRoots appends the roots in (0, 1) of a t² + b t + c.
func appendQuadRoots(ts []float64, a, b, c float64) []float64 {
	in := func(t float64) {
		if t > 0 && t < 1 {
			ts = append(ts, t)
		}
	}
	const eps = 1e-12
	if math.Abs(a) < eps {
		if b != 0 {
			in(-c / b)
		}
		return ts
	}
	disc := b*b - 4*a*c
	if disc < 0 {
		return ts
	}
	sq := math.Sqrt(disc)
	in((-b + sq) / (2 * a))
	in((-b - sq) / (2 * a))
	return ts
}
