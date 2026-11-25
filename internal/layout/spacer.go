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
