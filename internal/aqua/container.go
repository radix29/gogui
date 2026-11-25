package aqua

import (
	"slices"

	"github.com/radix29/gogui/internal/driver"
)

// Container is the aqua driver.Container (PLAN.md S2.2d): an element that
// draws nothing and hosts the elements of its child widgets. SetChildren
// re-parents them in order, which is the paint order and the focus order.
// A child widget is shown through its element (an aqua element, or the
// element of a control's peer); any other widget is kept in Widgets but
// invisible.
type Container struct {
	Base
	widgets     []driver.Widget
	controlSize driver.ControlSize
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

// AppendChildren appends children after the current ones, as
// SetChildren(append(Widgets(), children...)) would, without resending
// the others (P19). It panics, without changing anything, for a widget
// already a child or listed twice.
func (c *Container) AppendChildren(children []driver.Widget) {
	es := make([]Element, 0, len(children))
	for _, w := range children {
		if e := ElementOf(w); e != nil {
			es = append(es, e)
		}
	}
	for i, e := range es {
		if e.base().parent == &c.Base || slices.ContainsFunc(es[:i], func(d Element) bool { return d.base() == e.base() }) {
			panic("aqua: AppendChildren with a widget already in the list")
		}
		c.adoptable(e)
	}
	for _, e := range es {
		c.AddChild(e)
	}
	c.widgets = append(c.widgets, children...) // Widgets returns a copy
}

// Widgets returns the child widgets last passed to SetChildren, elements or
// not.
func (c *Container) Widgets() []driver.Widget { return slices.Clone(c.widgets) }

// SetControlSize implements driver.Widget. A container has no control size
// of its own; the value is only recorded.
func (c *Container) SetControlSize(cs driver.ControlSize) { c.controlSize = cs }

// ControlSize returns the value last passed to SetControlSize.
func (c *Container) ControlSize() driver.ControlSize { return c.controlSize }

// SetTooltip (driver.Widget) is Base's: the tip shows over the container
// and over its children without a tooltip of their own (S5.5l).

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
