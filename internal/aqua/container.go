package aqua

import (
	"slices"

	"github.com/radix29/gogui/internal/driver"
)

// Container is the aqua driver.Container (PLAN.md S2.2d): an element that
// draws nothing and hosts the elements of its child widgets. SetChildren
// re-parents them in order, which is the paint order and the focus order.
// A child widget is shown through its element (an aqua element, or the
// element of a placeholder peer); any other widget is kept in Widgets but
// invisible.
type Container struct {
	Base
	widgets     []driver.Widget
	controlSize driver.ControlSize
	tooltip     string
	destroyed   bool
}

var _ driver.Container = (*Container)(nil)

// NewContainer returns an empty container.
func NewContainer() *Container {
	c := &Container{}
	c.Init(c)
	return c
}

// SetChildren implements driver.Container. Elements no longer in the list
// are detached (not destroyed); the others are moved here from their old
// parents, in order. It panics, without changing anything, for a widget
// listed twice or for the container itself or one of its ancestors.
func (c *Container) SetChildren(children []driver.Widget) {
	es := make([]Element, 0, len(children))
	for _, w := range children {
		if e := ElementOf(w); e != nil {
			es = append(es, e)
		}
	}
	c.Base.SetChildren(es...)
	c.widgets = slices.Clone(children)
}

// Widgets returns the child widgets last passed to SetChildren, elements or
// not.
func (c *Container) Widgets() []driver.Widget { return slices.Clone(c.widgets) }

// SetControlSize implements driver.Widget. A container has no control size
// of its own; the value is only recorded.
func (c *Container) SetControlSize(cs driver.ControlSize) { c.controlSize = cs }

// ControlSize returns the value last passed to SetControlSize.
func (c *Container) ControlSize() driver.ControlSize { return c.controlSize }

// SetTooltip implements driver.Widget. Tooltips are drawn from S5.5 on;
// until then the text is only recorded.
func (c *Container) SetTooltip(text string) { c.tooltip = text }

// Tooltip returns the text last passed to SetTooltip.
func (c *Container) Tooltip() string { return c.tooltip }

// Focus implements driver.Widget. A container can't take the focus, so it
// does nothing.
func (c *Container) Focus() {}

// Destroy implements driver.Widget: the container leaves its parent. Its
// children stay attached to it (the public layer destroys them first).
func (c *Container) Destroy() {
	c.RemoveFromParent()
	c.destroyed = true
}

// Destroyed reports whether Destroy has been called.
func (c *Container) Destroyed() bool { return c.destroyed }
