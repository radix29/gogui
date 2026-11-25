package stub

import "github.com/radix29/gogui/internal/driver"

// Base implements driver.Widget. Every stub widget embeds it.
type Base struct {
	Frame       driver.Rect
	Disabled    bool
	Hidden      bool
	ControlSize driver.ControlSize
	Tooltip     string

	// FocusCalls counts the calls to Focus.
	FocusCalls int

	Destroyed bool

	// Fixed, if not zero, is returned by FittingSize (clamped to max)
	// instead of the fake metrics of the control.
	Fixed driver.Size

	natural func(max driver.Size) driver.Size
	parent  parent
}

// SetFrame implements driver.Widget.
func (b *Base) SetFrame(r driver.Rect) { b.Frame = r }

// FittingSize implements driver.Widget. It returns Fixed, or the fake
// metrics of the control, clamped to max.
func (b *Base) FittingSize(max driver.Size) driver.Size {
	var s driver.Size
	switch {
	case !b.Fixed.IsZero():
		s = b.Fixed
	case b.natural != nil:
		s = b.natural(max)
	}
	return s.Min(max)
}

// SetEnabled implements driver.Widget. Disabled holds the widget's own
// state; Enabled also looks at its ancestors.
func (b *Base) SetEnabled(enabled bool) { b.Disabled = !enabled }

// Enabled reports whether the widget accepts user input: it and every stub
// container, box or scroll view above it are enabled. A disabled container
// disables its subtree, as on aqua, so the User* methods of a control in
// it do nothing.
func (b *Base) Enabled() bool {
	for w := b; w != nil; {
		if w.Disabled {
			return false
		}
		p, ok := w.parent.(interface{ stubBase() *Base })
		if !ok {
			return true
		}
		w = p.stubBase()
	}
	return true
}

// SetHidden implements driver.Widget.
func (b *Base) SetHidden(hidden bool) { b.Hidden = hidden }

// SetControlSize implements driver.Widget.
func (b *Base) SetControlSize(cs driver.ControlSize) { b.ControlSize = cs }

// SetTooltip implements driver.Widget.
func (b *Base) SetTooltip(text string) { b.Tooltip = text }

// Focus implements driver.Widget.
func (b *Base) Focus() { b.FocusCalls++ }

// Destroy implements driver.Widget. It removes the widget from its parent.
func (b *Base) Destroy() {
	if b.parent != nil {
		b.parent.removeChild(b)
		b.parent = nil
	}
	b.Destroyed = true
}

// HasParent reports whether the widget is currently the child of a stub
// container, box, scroll view or window.
func (b *Base) HasParent() bool { return b.parent != nil }

func (b *Base) stubBase() *Base { return b }

// baseOf returns the Base of a stub widget (or of a type that embeds one),
// or nil for widgets that are not stubs.
func baseOf(w driver.Widget) *Base {
	if p, ok := w.(interface{ stubBase() *Base }); ok {
		return p.stubBase()
	}
	return nil
}

// parent is implemented by the stubs that hold child widgets.
type parent interface {
	removeChild(child *Base)
}

// adopt makes p the parent of w, removing w from its previous parent first.
// Widgets that are not stubs are not tracked.
func adopt(p parent, w driver.Widget) {
	b := baseOf(w)
	if b == nil {
		return
	}
	if b.parent != nil && b.parent != p {
		b.parent.removeChild(b)
	}
	b.parent = p
}

// release clears the parent of w if it is p.
func release(p parent, w driver.Widget) {
	if b := baseOf(w); b != nil && b.parent == p {
		b.parent = nil
	}
}

// Container implements driver.Container.
type Container struct {
	Base
	Children []driver.Widget
}

// NewContainer returns a stub container.
func NewContainer() *Container { return &Container{} }

// SetChildren implements driver.Container.
func (c *Container) SetChildren(children []driver.Widget) {
	keep := make(map[driver.Widget]bool, len(children))
	for _, w := range children {
		keep[w] = true
	}
	for _, w := range c.Children {
		if !keep[w] {
			release(c, w)
		}
	}
	c.Children = append([]driver.Widget(nil), children...)
	for _, w := range c.Children {
		adopt(c, w)
	}
}

// AppendChildren appends children after the current ones (the gogui
// stack's incremental sync).
func (c *Container) AppendChildren(children []driver.Widget) {
	c.Children = append(c.Children, children...)
	for _, w := range children {
		adopt(c, w)
	}
}

func (c *Container) removeChild(child *Base) {
	out := c.Children[:0]
	for _, w := range c.Children {
		if baseOf(w) != child {
			out = append(out, w)
		}
	}
	clear(c.Children[len(out):])
	c.Children = out
}
