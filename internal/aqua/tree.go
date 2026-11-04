package aqua

import "slices"

// Parent returns the parent element, or nil for a root.
func (b *Base) Parent() Element {
	if b.parent == nil {
		return nil
	}
	return b.parent.self
}

// Root returns the root of the tree b is in (b's element itself when it has
// no parent). It panics before Init.
func (b *Base) Root() Element {
	e := b
	for e.parent != nil {
		e = e.parent
	}
	return e.mustSelf()
}

// Children returns a copy of the children in paint order.
func (b *Base) Children() []Element { return slices.Clone(b.children) }

// ChildCount returns the number of children.
func (b *Base) ChildCount() int { return len(b.children) }

// Child returns the i-th child in paint order. It panics when i is out of
// range.
func (b *Base) Child(i int) Element { return b.children[i] }

// IndexOf returns the index of c among the children, or -1.
func (b *Base) IndexOf(c Element) int {
	if c == nil || c.base().parent != b {
		return -1
	}
	return slices.IndexFunc(b.children, func(e Element) bool { return e.base() == c.base() })
}

// AddChild appends c as the topmost child. See InsertChild.
func (b *Base) AddChild(c Element) {
	n := len(b.children)
	if c != nil && c.base().parent == b {
		n--
	}
	b.InsertChild(n, c)
}

// InsertChild inserts c at index i in the paint order. If c has a parent it
// is removed there first (re-parenting); when that parent is b, i counts
// without c, so 0 ≤ i ≤ ChildCount()-1, and otherwise 0 ≤ i ≤ ChildCount().
// It panics when c is nil, not initialised, b's element itself or an
// ancestor of it, and when i is out of range. The old and the new parent are
// invalidated.
func (b *Base) InsertChild(i int, c Element) {
	b.mustSelf()
	cb := b.adoptable(c)
	n := len(b.children)
	if cb.parent == b {
		n--
	}
	if i < 0 || i > n {
		panic("aqua: InsertChild index out of range")
	}
	old := cb.routedBy()
	cb.detach()
	b.children = slices.Insert(b.children, i, c)
	cb.parent = b
	b.Invalidate()
	old.validateTree()
	b.treeChanged()
}

// SetChildren replaces the children with cs, in order. Children not in cs
// are detached (not destroyed); elements in cs are removed from their old
// parents first. It panics for a nil, uninitialised or repeated element, and
// for b's element itself or an ancestor of it. It invalidates b (and every
// old parent) when the list changes; the same list again does nothing.
func (b *Base) SetChildren(cs ...Element) {
	b.mustSelf()
	for i, c := range cs {
		cb := b.adoptable(c)
		for _, d := range cs[:i] {
			if d.base() == cb {
				panic("aqua: SetChildren with a repeated element")
			}
		}
	}
	if slices.Equal(cs, b.children) {
		return
	}
	var olds []*Router
	for _, old := range b.children {
		old.base().parent = nil
	}
	b.children = nil
	for _, c := range cs {
		if cb := c.base(); cb.parent != nil {
			olds = append(olds, cb.routedBy())
			cb.detach()
		}
	}
	b.children = slices.Clone(cs)
	for _, c := range cs {
		c.base().parent = b
	}
	b.Invalidate()
	for _, r := range olds {
		r.validateTree()
	}
	b.treeChanged()
}

// RemoveChild detaches c if it is a child of b and reports whether it was.
func (b *Base) RemoveChild(c Element) bool {
	if b.IndexOf(c) < 0 {
		return false
	}
	c.base().RemoveFromParent()
	return true
}

// RemoveFromParent detaches the element from its parent, which is
// invalidated. The element keeps its subtree, frame and flags; it becomes a
// root without an invalidate callback. It does nothing for a root.
func (b *Base) RemoveFromParent() {
	if p := b.detach(); p != nil {
		p.treeChanged()
	}
}

// detach removes b from its parent and invalidates the parent, without
// letting a router check its state (the caller does, once the tree is
// consistent again). It returns the old parent, or nil for a root.
func (b *Base) detach() *Base {
	p := b.parent
	if p == nil {
		return nil
	}
	p.children = slices.DeleteFunc(p.children, func(e Element) bool { return e.base() == b })
	b.parent = nil
	p.Invalidate()
	return p
}

// adoptable checks that c may become a child of b and returns its Base.
func (b *Base) adoptable(c Element) *Base {
	if c == nil {
		panic("aqua: nil child")
	}
	cb := c.base()
	cb.mustSelf()
	if cb.router != nil {
		panic("aqua: the root of a Router cannot become a child")
	}
	for e := b; e != nil; e = e.parent {
		if e == cb {
			panic("aqua: an element cannot be its own descendant")
		}
	}
	return cb
}

// mustSelf returns the embedding element and panics before Init.
func (b *Base) mustSelf() Element {
	if b.self == nil {
		panic("aqua: element used before Init")
	}
	return b.self
}
