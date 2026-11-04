package layout

import "github.com/radix29/gogui/internal/driver"

// Spacer is empty space. In a stack it expands along the stack's main axis
// (and counts for the stack's expansion there); it never expands on its
// own, so outside a stack it has zero size. Its natural and min size are 0
// unless its Props say otherwise.
type Spacer struct{ props Props }

// NewSpacer returns a spacer.
func NewSpacer() *Spacer { return &Spacer{} }

// Props implements Node.
func (s *Spacer) Props() *Props { return &s.props }

// Expands implements Node. It is always false; the parent stack treats a
// spacer as expanding on its main axis.
func (s *Spacer) Expands(Axis) bool { return false }

// Measure implements Node.
func (s *Spacer) Measure(_ *Pass, max driver.Size) driver.Size {
	if s.props.hidden {
		return driver.Size{}
	}
	max = normSize(max)
	return driver.Size{W: s.props.resolve(Horizontal, 0, 0, max.W), H: s.props.resolve(Vertical, 0, 0, max.H)}
}

// Arrange implements Node. A spacer has nothing to frame.
func (s *Spacer) Arrange(*Pass, driver.Rect) {}

// firstLiner is the optional FirstLine method of a node.
type firstLiner interface {
	FirstLine(p *Pass, max driver.Size) float64
}

// firstLine returns the height of n's first line within max, for aligning
// form labels: n's FirstLine if it has one, else its measured height.
// Hidden nodes report 0.
func firstLine(p *Pass, n Node, max driver.Size) float64 {
	if n.Props().Hidden() {
		return 0
	}
	if f, ok := n.(firstLiner); ok {
		return f.FirstLine(p, normSize(max))
	}
	return p.Measure(n, max).H
}

// get returns the component of s on axis a.
func get(s driver.Size, a Axis) float64 {
	if a == Horizontal {
		return s.W
	}
	return s.H
}

// mk builds a size from its main-axis and cross-axis components.
func mk(main Axis, m, c float64) driver.Size {
	if main == Horizontal {
		return driver.Size{W: m, H: c}
	}
	return driver.Size{W: c, H: m}
}

func getPt(p driver.Point, a Axis) float64 {
	if a == Horizontal {
		return p.X
	}
	return p.Y
}

func mkPt(main Axis, m, c float64) driver.Point {
	if main == Horizontal {
		return driver.Point{X: m, Y: c}
	}
	return driver.Point{X: c, Y: m}
}
