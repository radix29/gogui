package render

import (
	"math"
	"slices"
	"testing"

	"github.com/radix29/gogui/internal/driver"
)

const eps = 1e-9

func pt(x, y float64) Point { return Point{X: x, Y: y} }

func rc(x, y, w, h float64) Rect { return Rect{X: x, Y: y, W: w, H: h} }

func near(a, b float64) bool { return math.Abs(a-b) <= eps }

func nearPt(a, b Point) bool { return near(a.X, b.X) && near(a.Y, b.Y) }

func nearRect(a, b Rect) bool {
	return near(a.X, b.X) && near(a.Y, b.Y) && near(a.W, b.W) && near(a.H, b.H)
}

func nearT(a, b Transform) bool {
	return near(a.A, b.A) && near(a.B, b.B) && near(a.C, b.C) && near(a.D, b.D) && near(a.E, b.E) && near(a.F, b.F)
}

// --- Path ---------------------------------------------------------------

func segs(p *Path) []Segment { return slices.Collect(p.Segments()) }

func TestPathEmpty(t *testing.T) {
	var p Path
	if !p.Empty() || p.Len() != 0 || p.Bounds() != (Rect{}) {
		t.Fatalf("zero path: empty=%v len=%d bounds=%v", p.Empty(), p.Len(), p.Bounds())
	}
	if _, ok := p.Current(); ok {
		t.Error("zero path has a current point")
	}
	if p.Close(); !p.Empty() {
		t.Error("Close on an empty path added a segment")
	}
}

func TestPathSegments(t *testing.T) {
	var p Path
	p.MoveTo(pt(1, 2)).LineTo(pt(3, 4)).QuadTo(pt(5, 6), pt(7, 8)).
		CubicTo(pt(9, 10), pt(11, 12), pt(13, 14)).Close()
	want := []Segment{
		{Op: OpMoveTo, Pts: [3]Point{pt(1, 2)}},
		{Op: OpLineTo, Pts: [3]Point{pt(3, 4)}},
		{Op: OpQuadTo, Pts: [3]Point{pt(5, 6), pt(7, 8)}},
		{Op: OpCubicTo, Pts: [3]Point{pt(9, 10), pt(11, 12), pt(13, 14)}},
		{Op: OpClose},
	}
	if got := segs(&p); !slices.Equal(got, want) {
		t.Fatalf("segments:\n got %v\nwant %v", got, want)
	}
	if cur, ok := p.Current(); !ok || cur != pt(1, 2) {
		t.Errorf("current after Close = %v, %v; want start (1,2)", cur, ok)
	}
	// Early stop of the iterator.
	n := 0
	for range p.Segments() {
		n++
		if n == 2 {
			break
		}
	}
	if n != 2 {
		t.Errorf("iterator did not stop: %d", n)
	}
}

func TestPathImplicitMoves(t *testing.T) {
	var p Path
	p.LineTo(pt(5, 5)) // no current point: acts as a move
	p.LineTo(pt(6, 5)).Close()
	p.LineTo(pt(9, 9)) // after Close: new subpath at the closed start
	want := []Op{OpMoveTo, OpLineTo, OpClose, OpMoveTo, OpLineTo}
	var got []Op
	for s := range p.Segments() {
		got = append(got, s.Op)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ops = %v, want %v", got, want)
	}
	if s := segs(&p)[3]; s.Pts[0] != pt(5, 5) {
		t.Errorf("implicit move after Close to %v, want (5,5)", s.Pts[0])
	}

	var q Path
	q.CubicTo(pt(1, 1), pt(2, 2), pt(3, 3))
	if s := segs(&q); s[0].Op != OpMoveTo || s[0].Pts[0] != pt(1, 1) {
		t.Errorf("CubicTo without current point: %v", s)
	}
	var r Path
	r.QuadTo(pt(4, 0), pt(8, 0))
	if s := segs(&r); s[0].Op != OpMoveTo || s[0].Pts[0] != pt(4, 0) {
		t.Errorf("QuadTo without current point: %v", s)
	}
	// Double Close adds nothing.
	p.Close().Close()
	if p.Len() != 6 {
		t.Errorf("len after double Close = %d, want 6", p.Len())
	}
}

func TestPathReset(t *testing.T) {
	var p Path
	p.MoveTo(pt(1, 1)).LineTo(pt(2, 2))
	p.Reset()
	if !p.Empty() {
		t.Fatal("Reset left segments")
	}
	if _, ok := p.Current(); ok {
		t.Error("Reset left a current point")
	}
	p.LineTo(pt(3, 3))
	if s := segs(&p); len(s) != 1 || s[0].Op != OpMoveTo {
		t.Errorf("after Reset: %v", s)
	}
}

func TestPathBounds(t *testing.T) {
	tests := []struct {
		name  string
		build func(p *Path)
		want  Rect
	}{
		{"lines", func(p *Path) { p.MoveTo(pt(1, 5)).LineTo(pt(4, 2)).LineTo(pt(-3, 7)) }, rc(-3, 2, 7, 5)},
		{"move only", func(p *Path) { p.MoveTo(pt(2, 3)) }, rc(2, 3, 0, 0)},
		{"move counts", func(p *Path) { p.MoveTo(pt(0, 0)).MoveTo(pt(10, 10)).LineTo(pt(12, 10)) }, rc(0, 0, 12, 10)},
		// Quadratic (0,0) c(5,10) (10,0): peak y = 5 at t = 0.5, control point excluded.
		{"quad peak", func(p *Path) { p.MoveTo(pt(0, 0)).QuadTo(pt(5, 10), pt(10, 0)) }, rc(0, 0, 10, 5)},
		// Quadratic overshooting in x: (0,0) c(10,5) (0,10): x max 5 at t = 0.5.
		{"quad x", func(p *Path) { p.MoveTo(pt(0, 0)).QuadTo(pt(10, 5), pt(0, 10)) }, rc(0, 0, 5, 10)},
		// Cubic (0,0) c(0,10) c(10,10) (10,0): y max 7.5 at t = 0.5.
		{"cubic arch", func(p *Path) { p.MoveTo(pt(0, 0)).CubicTo(pt(0, 10), pt(10, 10), pt(10, 0)) }, rc(0, 0, 10, 7.5)},
		// S-curve (0,0) c(10,0) c(-10,10) (0,10): two x extrema at ±(√3/9)·10·… computed below.
		{"cubic s", func(p *Path) { p.MoveTo(pt(0, 0)).CubicTo(pt(10, 0), pt(-10, 10), pt(0, 10)) }, sCurveBounds()},
		// Degenerate cubic (a straight line written as a cubic).
		{"cubic line", func(p *Path) { p.MoveTo(pt(0, 0)).CubicTo(pt(1, 1), pt(2, 2), pt(3, 3)) }, rc(0, 0, 3, 3)},
		// A full circle from four ArcTo corners of a 20×20 square, radius 10.
		{"circle", func(p *Path) {
			p.MoveTo(pt(10, 0))
			p.ArcTo(pt(20, 0), pt(20, 20), 10)
			p.ArcTo(pt(20, 20), pt(0, 20), 10)
			p.ArcTo(pt(0, 20), pt(0, 0), 10)
			p.ArcTo(pt(0, 0), pt(20, 0), 10)
			p.Close()
		}, rc(0, 0, 20, 20)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p Path
			tt.build(&p)
			if got := p.Bounds(); !nearRect(got, tt.want) {
				t.Errorf("Bounds = %v, want %v", got, tt.want)
			}
		})
	}
}

// sCurveBounds returns the bounds of the cubic (0,0) (10,0) (-10,10) (0,10)
// by dense sampling, independently of the extrema code.
func sCurveBounds() Rect {
	b := bbox{minX: math.Inf(1), minY: math.Inf(1), maxX: math.Inf(-1), maxY: math.Inf(-1)}
	p0, p1, p2, p3 := pt(0, 0), pt(10, 0), pt(-10, 10), pt(0, 10)
	// x(t) = 30t - 90t² + 60t³ (from the control points); x'(t) = 0 at
	// t = (3 ± √3)/6. Evaluate there exactly.
	for _, t := range []float64{0, 1, (3 - math.Sqrt(3)) / 6, (3 + math.Sqrt(3)) / 6} {
		b.add(cubicAt(p0, p1, p2, p3, t))
	}
	return Rect{X: b.minX, Y: b.minY, W: b.maxX - b.minX, H: b.maxY - b.minY}
}

// TestPathBoundsSampled checks Bounds against dense sampling of random
// curves: the box contains every sample and is no larger than needed.
func TestPathBoundsSampled(t *testing.T) {
	seed := uint64(1)
	rnd := func() float64 {
		seed = seed*6364136223846793005 + 1442695040888963407
		return float64(seed>>11)/float64(1<<53)*200 - 100
	}
	pt := func() Point { return pt(rnd(), rnd()) }
	for i := range 500 {
		var p Path
		p.MoveTo(pt())
		if i%2 == 0 {
			p.CubicTo(pt(), pt(), pt())
		} else {
			p.QuadTo(pt(), pt())
		}
		got := p.Bounds()
		ss := segs(&p)
		b := bbox{minX: math.Inf(1), minY: math.Inf(1), maxX: math.Inf(-1), maxY: math.Inf(-1)}
		const n = 4000
		for k := 0; k <= n; k++ {
			tt := float64(k) / n
			var q Point
			if ss[1].Op == OpCubicTo {
				q = cubicAt(ss[0].Pts[0], ss[1].Pts[0], ss[1].Pts[1], ss[1].Pts[2], tt)
			} else {
				q = quadAt(ss[0].Pts[0], ss[1].Pts[0], ss[1].Pts[1], tt)
			}
			b.add(q)
		}
		sampled := Rect{X: b.minX, Y: b.minY, W: b.maxX - b.minX, H: b.maxY - b.minY}
		const tol = 1e-3 // sampling error near an extremum
		if got.X > sampled.X+eps || got.Y > sampled.Y+eps || got.MaxX() < sampled.MaxX()-eps || got.MaxY() < sampled.MaxY()-eps {
			t.Fatalf("curve %d: Bounds %v does not contain samples %v", i, got, sampled)
		}
		if sampled.X-got.X > tol || sampled.Y-got.Y > tol || got.MaxX()-sampled.MaxX() > tol || got.MaxY()-sampled.MaxY() > tol {
			t.Fatalf("curve %d: Bounds %v looser than samples %v", i, got, sampled)
		}
	}
}

func TestPathArcTo(t *testing.T) {
	// Rounded corner: from (0,0) along +x to the corner (10,0), then down.
	var p Path
	p.MoveTo(pt(0, 0)).ArcTo(pt(10, 0), pt(10, 10), 4)
	ss := segs(&p)
	if len(ss) != 3 || ss[1].Op != OpLineTo || ss[2].Op != OpCubicTo {
		t.Fatalf("segments = %v, want M L C", ss)
	}
	if !nearPt(ss[1].Pts[0], pt(6, 0)) {
		t.Errorf("first tangent point = %v, want (6,0)", ss[1].Pts[0])
	}
	if !nearPt(ss[2].Pts[2], pt(10, 4)) {
		t.Errorf("arc end = %v, want (10,4)", ss[2].Pts[2])
	}
	// The quarter arc's midpoint lies on the circle around (6,4), radius 4.
	mid := cubicAt(ss[1].Pts[0], ss[2].Pts[0], ss[2].Pts[1], ss[2].Pts[2], 0.5)
	if r := math.Hypot(mid.X-6, mid.Y-4); math.Abs(r-4) > 1e-3 {
		t.Errorf("arc midpoint radius = %v, want 4", r)
	}
	if cur, _ := p.Current(); !nearPt(cur, pt(10, 4)) {
		t.Errorf("current = %v, want (10,4)", cur)
	}
	// Counter-clockwise corner (turning the other way).
	var q Path
	q.MoveTo(pt(0, 10)).ArcTo(pt(10, 10), pt(10, 0), 4)
	if cur, _ := q.Current(); !nearPt(cur, pt(10, 6)) {
		t.Errorf("ccw arc end = %v, want (10,6)", cur)
	}
	if b := q.Bounds(); !nearRect(b, rc(0, 6, 10, 4)) {
		t.Errorf("ccw corner bounds = %v", b)
	}
	// An acute corner: the arc sweeps more than 90° and needs two cubics.
	var a Path
	a.MoveTo(pt(0, 0)).ArcTo(pt(10, 0), pt(0, 1), 1)
	n := 0
	for s := range a.Segments() {
		if s.Op == OpCubicTo {
			n++
		}
	}
	if n != 2 {
		t.Errorf("acute corner: %d cubics, want 2", n)
	}
}

func TestPathArcToDegenerate(t *testing.T) {
	tests := []struct {
		name   string
		p1, p2 Point
		r      float64
	}{
		{"zero radius", pt(10, 0), pt(10, 10), 0},
		{"negative radius", pt(10, 0), pt(10, 10), -3},
		{"NaN radius", pt(10, 0), pt(10, 10), math.NaN()},
		{"Inf radius", pt(10, 0), pt(10, 10), math.Inf(1)},
		{"collinear", pt(10, 0), pt(20, 0), 4},
		{"reversing", pt(10, 0), pt(5, 0), 4},
		{"p1 = current", pt(0, 0), pt(10, 10), 4},
		{"p2 = p1", pt(10, 0), pt(10, 0), 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p Path
			p.MoveTo(pt(0, 0)).ArcTo(tt.p1, tt.p2, tt.r)
			ss := segs(&p)
			if len(ss) != 2 || ss[1].Op != OpLineTo || ss[1].Pts[0] != tt.p1 {
				t.Errorf("segments = %v, want M then L to %v", ss, tt.p1)
			}
		})
	}
	var p Path
	p.ArcTo(pt(3, 3), pt(4, 4), 2)
	if ss := segs(&p); len(ss) != 1 || ss[0].Op != OpMoveTo || ss[0].Pts[0] != pt(3, 3) {
		t.Errorf("ArcTo without current point = %v, want MoveTo(3,3)", ss)
	}
}

// --- Radii --------------------------------------------------------------

func TestRadii(t *testing.T) {
	if UniformRadii(3) != (Radii{3, 3, 3, 3}) {
		t.Error("UniformRadii")
	}
	if !(Radii{}).IsZero() || !(Radii{-1, math.NaN(), 0, 0}).IsZero() || (Radii{0, 0, 1, 0}).IsZero() {
		t.Error("IsZero")
	}
	if !UniformRadii(2).IsUniform() || (Radii{1, 1, 1, 2}).IsUniform() {
		t.Error("IsUniform")
	}
}

func TestRadiiClamped(t *testing.T) {
	inf, nan := math.Inf(1), math.NaN()
	tests := []struct {
		name string
		rd   Radii
		r    Rect
		want Radii
	}{
		{"fits", Radii{1, 2, 3, 4}, rc(0, 0, 80, 22), Radii{1, 2, 3, 4}},
		{"half height", UniformRadii(20), rc(0, 0, 80, 22), UniformRadii(11)},
		{"half width", UniformRadii(20), rc(0, 0, 10, 50), UniformRadii(5)},
		{"exact half", UniformRadii(11), rc(0, 0, 80, 22), UniformRadii(11)},
		{"per corner", Radii{30, 2, 11, 12}, rc(0, 0, 80, 22), Radii{11, 2, 11, 11}},
		{"negative", Radii{-1, 2, -3, 4}, rc(0, 0, 80, 22), Radii{0, 2, 0, 4}},
		{"NaN and Inf", Radii{nan, inf, 1, 0}, rc(0, 0, 80, 22), Radii{0, 11, 1, 0}},
		{"empty rect", UniformRadii(5), rc(0, 0, 0, 22), Radii{}},
		{"negative rect", UniformRadii(5), rc(0, 0, -10, 22), Radii{}},
		{"NaN rect", UniformRadii(5), rc(0, 0, nan, 22), Radii{}},
		{"unbounded rect", UniformRadii(5), rc(0, 0, inf, inf), UniformRadii(5)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rd.Clamped(tt.r); got != tt.want {
				t.Errorf("Clamped = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Paint, stroke, shadow ----------------------------------------------

var (
	red   = driver.RGBA(1, 0, 0, 1)
	green = driver.RGBA(0, 1, 0, 1)
	blue  = driver.RGBA(0, 0, 1, 1)
)

func TestPaintNormalized(t *testing.T) {
	stops := []GradientStop{{0.8, blue}, {-0.5, red}, {1.5, green}, {0.8, green}, {math.NaN(), blue}}
	p := LinearGradient(pt(0, 0), pt(0, math.Inf(1)), stops...)
	stops[0].Offset = 0.1 // LinearGradient copied
	n := p.Normalized()
	want := []GradientStop{{0, red}, {0, blue}, {0.8, blue}, {0.8, green}, {1, green}}
	if !slices.Equal(n.Stops, want) {
		t.Errorf("stops = %v, want %v", n.Stops, want)
	}
	if n.End != pt(0, 0) {
		t.Errorf("non-finite end = %v, want (0,0)", n.End)
	}
	if p.Stops[0].Offset != 0.8 {
		t.Error("Normalized changed the original stops")
	}
	if got := LinearGradient(Point{}, pt(1, 0)).Normalized().Stops; got != nil {
		t.Errorf("no stops = %v, want nil", got)
	}
	s := Paint{Kind: PaintSolid, Color: red, Stops: stops}.Normalized()
	if s.Kind != PaintSolid || s.Color != red || s.Stops != nil {
		t.Errorf("solid = %+v", s)
	}
	if u := (Paint{Kind: 7, Color: red}).Normalized(); u.Kind != PaintSolid || u.Color != red {
		t.Errorf("unknown kind = %+v", u)
	}
	if sp := Solid(red); sp.Kind != PaintSolid || sp.Color != red || sp.Stops != nil {
		t.Error("Solid")
	}
	if PaintSolid.String() != "solid" || PaintLinear.String() != "linear" || PaintKind(9).String() != "unknown" {
		t.Error("PaintKind.String")
	}
}

func TestSemanticDetection(t *testing.T) {
	sem := driver.Semantic(driver.ColorControlAccent)
	if Solid(red).IsSemantic() || !Solid(sem).IsSemantic() {
		t.Error("solid IsSemantic")
	}
	if LinearGradient(Point{}, pt(1, 1), GradientStop{0, red}).IsSemantic() ||
		!LinearGradient(Point{}, pt(1, 1), GradientStop{0, red}, GradientStop{1, sem}).IsSemantic() {
		t.Error("gradient IsSemantic")
	}
	// A gradient's Color field is unused, so a semantic value there is ignored.
	if (Paint{Kind: PaintLinear, Color: sem}).IsSemantic() {
		t.Error("gradient with semantic unused Color")
	}
	if (Shadow{Color: red}).IsSemantic() || !(Shadow{Color: sem}).IsSemantic() {
		t.Error("shadow IsSemantic")
	}
}

func TestStrokeWidth(t *testing.T) {
	for _, tt := range []struct{ in, want float64 }{{1.5, 1.5}, {0, 0}, {-2, 0}, {math.NaN(), 0}, {math.Inf(1), 0}, {math.Inf(-1), 0}} {
		if got := StrokeWidth(tt.in); got != tt.want {
			t.Errorf("StrokeWidth(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestOpacityAndBlurRadius(t *testing.T) {
	for _, tt := range []struct{ in, want float64 }{{0.5, 0.5}, {0, 0}, {1, 1}, {2, 1}, {-1, 0}, {math.NaN(), 0}, {math.Inf(1), 1}} {
		if got := Opacity(tt.in); got != tt.want {
			t.Errorf("Opacity(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
	for _, tt := range []struct{ in, want float64 }{{8, 8}, {0, 0}, {-1, 0}, {math.NaN(), 0}, {math.Inf(1), 0}, {math.Inf(-1), 0}} {
		if got := BlurRadius(tt.in); got != tt.want {
			t.Errorf("BlurRadius(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestStrokeStyleNormalized(t *testing.T) {
	z := StrokeStyle{}.Normalized()
	if z.Cap != CapButt || z.Join != JoinMiter || z.MiterLimit != DefaultMiterLimit || z.Dash != nil {
		t.Errorf("zero = %+v", z)
	}
	tests := []struct {
		name string
		in   StrokeStyle
		want StrokeStyle
	}{
		{"kept", StrokeStyle{Cap: CapRound, Join: JoinBevel, MiterLimit: 4, Dash: []float64{2, 1}, DashOffset: 1},
			StrokeStyle{Cap: CapRound, Join: JoinBevel, MiterLimit: 4, Dash: []float64{2, 1}, DashOffset: 1}},
		{"unknown enums", StrokeStyle{Cap: 9, Join: 9, MiterLimit: 4},
			StrokeStyle{Cap: CapButt, Join: JoinMiter, MiterLimit: 4}},
		{"miter < 1", StrokeStyle{MiterLimit: 0.5}, StrokeStyle{MiterLimit: DefaultMiterLimit}},
		{"miter NaN", StrokeStyle{MiterLimit: math.NaN()}, StrokeStyle{MiterLimit: DefaultMiterLimit}},
		{"miter Inf", StrokeStyle{MiterLimit: math.Inf(1)}, StrokeStyle{MiterLimit: DefaultMiterLimit}},
		{"miter 1", StrokeStyle{MiterLimit: 1}, StrokeStyle{MiterLimit: 1}},
		{"negative dashes", StrokeStyle{Dash: []float64{3, -1, math.NaN(), 2}},
			StrokeStyle{MiterLimit: DefaultMiterLimit, Dash: []float64{3, 0, 0, 2}}},
		{"odd dashes", StrokeStyle{Dash: []float64{3, 1, 2}},
			StrokeStyle{MiterLimit: DefaultMiterLimit, Dash: []float64{3, 1, 2, 3, 1, 2}}},
		{"all zero", StrokeStyle{Dash: []float64{0, -1, math.Inf(1)}, DashOffset: 5},
			StrokeStyle{MiterLimit: DefaultMiterLimit}},
		{"bad offset", StrokeStyle{Dash: []float64{1, 1}, DashOffset: math.NaN()},
			StrokeStyle{MiterLimit: DefaultMiterLimit, Dash: []float64{1, 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.Normalized()
			if got.Cap != tt.want.Cap || got.Join != tt.want.Join || got.MiterLimit != tt.want.MiterLimit ||
				got.DashOffset != tt.want.DashOffset || !slices.Equal(got.Dash, tt.want.Dash) {
				t.Errorf("Normalized = %+v, want %+v", got, tt.want)
			}
		})
	}
	in := StrokeStyle{Dash: []float64{1, -2, 3}}
	out := in.Normalized()
	out.Dash[0] = 9
	if in.Dash[0] != 1 || in.Dash[1] != -2 {
		t.Error("Normalized changed or shares a dash slice it had to fix")
	}
}

// TestNormalizedShares (review P16): a paint or a stroke style that is
// already normal comes back with its own slices, so a canvas normalizing
// it on every call allocates nothing.
func TestNormalizedShares(t *testing.T) {
	stops := []GradientStop{{0, red}, {0.5, green}, {0.5, blue}, {1, red}}
	p := LinearGradient(pt(0, 0), pt(0, 10), stops...)
	s := StrokeStyle{Cap: CapRound, Dash: []float64{2, 0, 1, 3}}
	if n := p.Normalized(); &n.Stops[0] != &p.Stops[0] || !slices.Equal(n.Stops, stops) {
		t.Errorf("normal stops copied or changed: %v", n.Stops)
	}
	if n := s.Normalized(); &n.Dash[0] != &s.Dash[0] || !slices.Equal(n.Dash, s.Dash) {
		t.Errorf("normal dash copied or changed: %v", n.Dash)
	}
	if n := testing.AllocsPerRun(100, func() {
		_ = p.Normalized()
		_ = s.Normalized()
	}); n != 0 {
		t.Errorf("Normalized of normal values: %v allocations", n)
	}
	for _, bad := range [][]GradientStop{{{0.5, red}, {0.2, red}}, {{-0.1, red}}, {{1.1, red}}, {{math.NaN(), red}}} {
		if !stopsNormal(stops) || stopsNormal(bad) {
			t.Errorf("stopsNormal(%v)", bad)
		}
	}
}

func TestShadowNormalized(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)
	tests := []struct{ in, want Shadow }{
		{Shadow{Offset: pt(0, 1), Blur: 2, Spread: -1, Color: red, Inset: true},
			Shadow{Offset: pt(0, 1), Blur: 2, Spread: -1, Color: red, Inset: true}},
		{Shadow{Blur: -3}, Shadow{}},
		{Shadow{Blur: nan, Spread: nan}, Shadow{}},
		{Shadow{Blur: inf, Spread: -inf, Offset: pt(nan, inf)}, Shadow{}},
	}
	for i, tt := range tests {
		if got := tt.in.Normalized(); got != tt.want {
			t.Errorf("%d: Normalized = %+v, want %+v", i, got, tt.want)
		}
	}
}

// --- Transform ----------------------------------------------------------

func TestTransformBasics(t *testing.T) {
	id := Identity()
	if !id.IsIdentity() || !id.IsTranslation() || (Transform{}).IsIdentity() {
		t.Error("identity")
	}
	if p := Translation(3, 4).Apply(pt(1, 1)); p != pt(4, 5) {
		t.Errorf("translate = %v", p)
	}
	if p := Scaling(2, 3).Apply(pt(1, 1)); p != pt(2, 3) {
		t.Errorf("scale = %v", p)
	}
	// Positive angles turn clockwise on screen: +X towards +Y.
	if p := rotation(math.Pi / 2).Apply(pt(1, 0)); !nearPt(p, pt(0, 1)) {
		t.Errorf("rotate = %v, want (0,1)", p)
	}
	if !Translation(1, 2).IsTranslation() || Scaling(2, 2).IsTranslation() {
		t.Error("IsTranslation")
	}
	if Scaling(2, 3).Det() != 6 {
		t.Error("Det")
	}
}

func TestTransformComposition(t *testing.T) {
	tr, sc := Translation(10, 0), Scaling(2, 2)
	p := pt(1, 1)
	// Then: t first, u second.
	if got := tr.Then(sc).Apply(p); got != pt(22, 2) {
		t.Errorf("translate then scale = %v, want (22,2)", got)
	}
	if got := sc.Then(tr).Apply(p); got != pt(12, 2) {
		t.Errorf("scale then translate = %v, want (12,2)", got)
	}
	// Pre: the canvas form, the new op acts in the current user space.
	ctm := Translation(10, 0)
	if got := ctm.Pre(sc).Apply(p); got != pt(12, 2) {
		t.Errorf("ctm.Pre(scale) = %v, want (12,2)", got)
	}
	// Composition matches applying one after the other, for a mixed chain.
	a, b, c := rotation(0.3), Translation(-4, 7), Scaling(1.5, -2)
	chain := a.Then(b).Then(c)
	q := pt(3, -2)
	if got, want := chain.Apply(q), c.Apply(b.Apply(a.Apply(q))); !nearPt(got, want) {
		t.Errorf("chain = %v, want %v", got, want)
	}
	// Associativity.
	if !nearT(a.Then(b).Then(c), a.Then(b.Then(c))) {
		t.Error("Then is not associative")
	}
	if !nearT(Identity().Then(chain), chain) || !nearT(chain.Then(Identity()), chain) {
		t.Error("identity is not neutral")
	}
}

func TestTransformInvert(t *testing.T) {
	for i, m := range []Transform{
		Identity(),
		Translation(3, -4),
		Scaling(2, 0.5),
		rotation(1.1),
		rotation(0.7).Then(Scaling(3, -1)).Then(Translation(5, 9)),
		{A: 1, B: 2, C: 3, D: 4, E: 5, F: 6},
	} {
		inv, ok := m.Invert()
		if !ok {
			t.Fatalf("%d: not invertible", i)
		}
		if !nearT(m.Then(inv), Identity()) || !nearT(inv.Then(m), Identity()) {
			t.Errorf("%d: m·inv = %v, inv·m = %v", i, m.Then(inv), inv.Then(m))
		}
		p := pt(7, -3)
		if got := inv.Apply(m.Apply(p)); !nearPt(got, p) {
			t.Errorf("%d: round trip = %v", i, got)
		}
	}
	for i, m := range []Transform{
		{},
		Scaling(0, 1),
		{A: 1, B: 2, C: 2, D: 4}, // rows linearly dependent
		{A: math.NaN(), D: 1},
		{A: math.Inf(1), D: 1},
		{A: 1, D: 1, E: math.Inf(1)},
		{A: 1e-300, D: 1e-300}, // det underflows to 0
	} {
		if inv, ok := m.Invert(); ok || inv != (Transform{}) {
			t.Errorf("%d: Invert(%v) = %v, %v; want zero, false", i, m, inv, ok)
		}
	}
}

func TestTransformApplyRect(t *testing.T) {
	r := rc(1, 2, 3, 4)
	if got := Translation(10, 20).ApplyRect(r); got != (rc(11, 22, 3, 4)) {
		t.Errorf("translate = %v", got)
	}
	if got := Scaling(-2, 1).ApplyRect(r); !nearRect(got, rc(-8, 2, 6, 4)) {
		t.Errorf("mirror = %v", got)
	}
	if got := rotation(math.Pi / 2).ApplyRect(rc(0, 0, 2, 1)); !nearRect(got, rc(-1, 0, 1, 2)) {
		t.Errorf("rotate = %v", got)
	}
}

func TestOpString(t *testing.T) {
	want := "MLQCZ?"
	got := ""
	for _, op := range []Op{OpMoveTo, OpLineTo, OpQuadTo, OpCubicTo, OpClose, 9} {
		got += op.String()
	}
	if got != want {
		t.Errorf("ops = %q, want %q", got, want)
	}
}

// TestPathArcToNonFinite is B15: a non-finite point never becomes NaN
// cubics; ArcTo draws a line to p1, and a NaN p1 is left for ValidPath.
func TestPathArcToNonFinite(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)
	for _, tt := range []struct {
		name   string
		p1, p2 Point
	}{
		{"NaN p2", pt(10, 0), pt(nan, 10)},
		{"Inf p2", pt(10, 0), pt(10, inf)},
		{"NaN p1", pt(nan, 0), pt(10, 10)},
	} {
		var p Path
		p.MoveTo(pt(0, 0)).ArcTo(tt.p1, tt.p2, 4)
		ss := segs(&p)
		if len(ss) != 2 || ss[1].Op != OpLineTo {
			t.Errorf("%s: segments = %v, want M then L", tt.name, ss)
		}
		if ValidPath(&p) != ValidPoint(tt.p1) {
			t.Errorf("%s: ValidPath %v", tt.name, ValidPath(&p))
		}
	}
	// A NaN current point (a NaN MoveTo) is not finite either.
	var p Path
	p.MoveTo(pt(nan, 0)).ArcTo(pt(10, 0), pt(10, 10), 4)
	for _, s := range segs(&p) {
		if s.Op == OpCubicTo {
			t.Errorf("cubic from a NaN current point: %v", segs(&p))
		}
	}
}

// TestValidGeometry is B8's shared rules.
func TestValidGeometry(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)
	for _, tt := range []struct {
		r    Rect
		want bool
	}{
		{rc(0, 0, 10, 10), true},
		{rc(-5, -5, 0, 0), true},
		{rc(0, 0, -1, 10), false},
		{rc(0, 0, 10, -1), false},
		{rc(nan, 0, 10, 10), false},
		{rc(0, 0, nan, 10), false},
		{rc(0, inf, 10, 10), false},
		{rc(0, 0, inf, 10), false},
		{rc(1e308, 0, 1e308, 10), false}, // the far edge overflows
	} {
		if got := ValidRect(tt.r); got != tt.want {
			t.Errorf("ValidRect(%v) = %v, want %v", tt.r, got, tt.want)
		}
	}
	if !ValidPoint(pt(-3, 1e300)) || ValidPoint(pt(nan, 0)) || ValidPoint(pt(0, -inf)) {
		t.Error("ValidPoint")
	}
	var p Path
	if !ValidPath(nil) || !ValidPath(&p) {
		t.Error("ValidPath of nil or empty")
	}
	p.MoveTo(pt(0, 0)).CubicTo(pt(1, 1), pt(nan, 2), pt(3, 3))
	if ValidPath(&p) {
		t.Error("ValidPath with a NaN control point")
	}
}

// TestSegmentsBounds (review R12, P16): SegmentsBounds of a path's
// segments is its Bounds, Op.Points counts the points each kind uses, and
// Bounds allocates nothing (the extrema are arrays).
func TestSegmentsBounds(t *testing.T) {
	var p Path
	p.MoveTo(pt(0, 0)).QuadTo(pt(5, -10), pt(10, 0)).CubicTo(pt(12, 8), pt(20, 8), pt(22, 0)).
		LineTo(pt(22, 5)).ArcTo(pt(22, 12), pt(15, 12), 3).Close().MoveTo(pt(-3, 2))
	segs := slices.Collect(p.Segments())
	if got, want := SegmentsBounds(segs), p.Bounds(); got != want {
		t.Errorf("SegmentsBounds %v, Bounds %v", got, want)
	}
	if got := SegmentsBounds(nil); got != (Rect{}) {
		t.Errorf("no segments: %v", got)
	}
	if got := SegmentsBounds([]Segment{{Op: OpClose}}); got != (Rect{}) {
		t.Errorf("a lone close: %v", got)
	}
	for op, n := range map[Op]int{OpMoveTo: 1, OpLineTo: 1, OpQuadTo: 2, OpCubicTo: 3, OpClose: 0, Op(9): 0} {
		if op.Points() != n {
			t.Errorf("%v.Points() = %d, want %d", op, op.Points(), n)
		}
	}
	if n := testing.AllocsPerRun(100, func() { _ = p.Bounds(); _ = SegmentsBounds(segs) }); n != 0 {
		t.Errorf("Bounds: %v allocations", n)
	}
}

// rotation turns by angle radians around the origin, clockwise on screen
// (render has no Rotation; nothing but the tests rotates).
func rotation(angle float64) Transform {
	s, c := math.Sincos(angle)
	return Transform{A: c, B: s, C: -s, D: c}
}
