package layout

import "github.com/radix29/gogui/internal/driver"

// Layout is LayoutMin without the minimum size: the arrange alone.
func (r *Root) Layout(contentSize driver.Size) {
	r.pass.reset()
	r.node.Arrange(&r.pass, driver.RectOf(driver.Point{}, finiteSize(contentSize)))
}

// MinSize is LayoutMin's minimum size without the arrange.
func (r *Root) MinSize() driver.Size {
	r.pass.reset()
	return r.minSize(&r.pass)
}
