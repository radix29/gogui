package render

// PaintKind says how a Paint fills a shape.
type PaintKind uint8

// Paint kinds.
const (
	// PaintSolid fills with Paint.Color.
	PaintSolid PaintKind = iota
	// PaintLinear fills with a linear gradient from Paint.Start to
	// Paint.End through Paint.Stops.
	PaintLinear
)

// String returns the name used in recorder dumps.
func (k PaintKind) String() string {
	switch k {
	case PaintSolid:
		return "solid"
	case PaintLinear:
		return "linear"
	}
	return "unknown"
}

// GradientStop is one color of a gradient at Offset in [0, 1] along the
// gradient line.
type GradientStop struct {
	Offset float64
	Color  Color
}

// Paint is how a shape is filled or stroked: a solid color or a linear
// gradient. The zero value is solid transparent black, which draws nothing.
//
// Gradient end points are in the canvas's current user space (the same
// coordinates as the shape), as in Direct2D. Before the first stop the
// gradient has the first stop's color, after the last stop the last one's
// (pad extend mode). A gradient without stops draws nothing, and one with a
// single stop is that color.
type Paint struct {
	Kind  PaintKind
	Color Color // PaintSolid only

	Start, End Point          // PaintLinear only
	Stops      []GradientStop // PaintLinear only
}

// Solid returns a solid paint of color c.
func Solid(c Color) Paint { return Paint{Kind: PaintSolid, Color: c} }

// LinearGradient returns a linear gradient from start to end. The stops are
// copied; they may be in any order (Normalized sorts them).
func LinearGradient(start, end Point, stops ...GradientStop) Paint {
	return Paint{Kind: PaintLinear, Start: start, End: end, Stops: append([]GradientStop(nil), stops...)}
}

// IsSemantic reports whether any color of p is semantic. Such a paint must
// never reach a canvas (PLAN.md S2.1a): the theme resolves colors first.
func (p Paint) IsSemantic() bool {
	if p.Kind != PaintLinear {
		return p.Color.IsSemantic()
	}
	for _, s := range p.Stops {
		if s.Color.IsSemantic() {
			return true
		}
	}
	return false
}

// LineCap is the shape at the open ends of a stroke and of dashes.
type LineCap uint8

// Line caps (D2D_CAP_STYLE / GskLineCap order).
const (
	CapButt LineCap = iota
	CapRound
	CapSquare
)

// LineJoin is the shape where two stroke segments meet.
type LineJoin uint8

// Line joins.
const (
	JoinMiter LineJoin = iota
	JoinRound
	JoinBevel
)

// DefaultMiterLimit is the miter limit used when StrokeStyle.MiterLimit is
// unset (the CoreGraphics and Direct2D default).
const DefaultMiterLimit = 10

// StrokeStyle describes how a path is stroked, apart from its width and
// paint. The zero value is a solid line with butt caps and miter joins at
// DefaultMiterLimit.
type StrokeStyle struct {
	Cap        LineCap
	Join       LineJoin
	MiterLimit float64 // ratio of miter length to width; < 1 means DefaultMiterLimit

	// Dash alternates dash and gap lengths in logical points, starting with
	// a dash. Nil (or all zeros) is a solid line. DashOffset shifts the
	// pattern's start along the path.
	Dash       []float64
	DashOffset float64
}

// Shadow is a box shadow of a rounded rectangle, with CSS box-shadow
// semantics: an outer shadow is drawn outside the shape only, an inset one
// inside it only.
type Shadow struct {
	Offset Point   // shift of the shadow shape
	Blur   float64 // blur radius; the Gaussian's standard deviation is Blur/2
	Spread float64 // grows (or, negative, shrinks) the shadow shape before blurring
	Color  Color
	Inset  bool
}

// IsSemantic reports whether the shadow color is semantic (a bug at the
// canvas, as for Paint).
func (s Shadow) IsSemantic() bool { return s.Color.IsSemantic() }
