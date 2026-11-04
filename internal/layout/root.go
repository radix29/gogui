package layout

import (
	"math"

	"github.com/radix29/gogui/internal/driver"
)

// Root is the top of a layout tree: the node that fills a window's content
// area. It always gets the full content size, (0, 0, contentSize), whatever
// its expansion flags and size properties.
type Root struct {
	node Node
}

// NewRoot returns a root for n. It panics if n is nil.
func NewRoot(n Node) *Root { return (&Root{}).SetNode(n) }

// SetNode replaces the root node. It panics if n is nil.
func (r *Root) SetNode(n Node) *Root {
	if n == nil {
		panic("layout: nil root node")
	}
	r.node = n
	return r
}

// Node returns the root node.
func (r *Root) Node() Node { return r.node }

// Layout runs one layout pass: it frames the root node at
// (0, 0, contentSize), and the node frames its subtree. Negative, NaN and
// +Inf components of contentSize count as 0. A hidden root frames nothing.
func (r *Root) Layout(contentSize driver.Size) {
	var p Pass
	r.node.Arrange(&p, driver.RectOf(driver.Point{}, finiteSize(contentSize)))
}

// MinSize returns the smallest content size the tree fits in: the width is
// the root measured at (0, +Inf), and the height is the root's natural
// height measured at (that width, +Inf). Each measurement runs in a fresh
// Pass. The height is the natural one at the minimum width, not the
// measured height at 0, because a window must not cut off content that
// can't shrink; with the width at its minimum, wrapping labels are at their
// tallest. A hidden root reports 0 × 0. The public layer sets the window's
// min size to the larger of this and the app's configured minimum (S0.5).
func (r *Root) MinSize() driver.Size {
	inf := math.Inf(1)
	w := (&Pass{}).Measure(r.node, driver.Size{W: 0, H: inf}).W
	h := (&Pass{}).Measure(r.node, driver.Size{W: w, H: inf}).H
	return driver.Size{W: w, H: h}
}
