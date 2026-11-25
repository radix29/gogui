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
	pass Pass // reused by every call, cleared first (P7)
}

// NewRoot returns a root for n. It panics if n is nil.
func NewRoot(n Node) *Root {
	if n == nil {
		panic("layout: nil root node")
	}
	return &Root{node: n}
}

// LayoutMin runs one layout pass and returns the tree's minimum size, in
// one Pass (S6.2e), so the measurements the arrange made are not made again
// for the minimum size, nor the minimum width's for its height.
//
// The pass frames the root node at (0, 0, contentSize), and the node frames
// its subtree. Negative, NaN and +Inf components of contentSize count as 0.
// A hidden root frames nothing.
//
// The minimum is the smallest content size the tree fits in: the width is
// the root measured at (0, +Inf), and the height is the root's natural
// height measured at (that width, +Inf). The height is the natural one at
// the minimum width, not the measured height at 0, because a window must
// not cut off content that can't shrink; with the width at its minimum,
// wrapping labels are at their tallest. A hidden root reports 0 × 0. The
// public layer sets the window's min size to the larger of this and the
// app's configured minimum (S0.5).
func (r *Root) LayoutMin(contentSize driver.Size) driver.Size {
	r.pass.reset()
	r.node.Arrange(&r.pass, driver.RectOf(driver.Point{}, finiteSize(contentSize)))
	return r.minSize(&r.pass)
}

func (r *Root) minSize(p *Pass) driver.Size {
	inf := math.Inf(1)
	w := p.Measure(r.node, driver.Size{W: 0, H: inf}).W
	h := p.Measure(r.node, driver.Size{W: w, H: inf}).H
	return driver.Size{W: w, H: h}
}
