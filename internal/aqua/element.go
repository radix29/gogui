package aqua

import (
	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/layout"
	"github.com/radix29/gogui/internal/render"
)

// Element is one drawn part of a window: a control, a container, or an
// internal part such as a traffic light.
//
// Every element embeds Base, which implements the tree, the flags and the
// default behaviour; the unexported method seals the interface to such
// types. An embedding type overrides FittingSize, Paint and HitSelf as
// needed and calls Init once before the element joins a tree:
//
//	type swatch struct{ aqua.Base }
//
//	func newSwatch() *swatch {
//		s := &swatch{}
//		s.Init(s)
//		return s
//	}
//
// Input reaches elements through a Router (one per window), which calls
// HandleEvent; AccessibilityTree collects the AccessibleNode of each.
type Element interface {
	// FittingSize and SetFrame make the element a layout.Item. Base's
	// FittingSize returns the zero size; SetFrame sets the frame in the
	// parent's coordinates and invalidates on a change.
	layout.Item

	// Paint draws the element itself, not its children, in its own
	// coordinates: the origin is the top-left corner of the frame. Base's
	// Paint draws nothing. Paint is called only by PaintTree, which wraps it
	// in Save/Restore.
	Paint(c render.Canvas)

	// HitSelf reports whether the local point p hits the element itself
	// (its children are tested separately, first). Base's HitSelf is
	// Bounds().Contains(p); round parts override it.
	HitSelf(p driver.Point) bool

	// HandleEvent receives one routed input event (see Event and Router) and
	// reports whether the element handled it. Unhandled scroll, key and text
	// events bubble to the parent; a handled PointerDown captures the pointer.
	// Base's HandleEvent handles nothing.
	HandleEvent(ev Event) bool

	// Activate performs the element's primary action, as a click would (a
	// button's press; the a11y press action, and Return or Escape on a
	// default or cancel button), and reports whether it did anything.
	// Base's Activate does nothing and returns false.
	Activate() bool

	// AccessibleNode returns the element's own accessibility node: Role,
	// Label, Value and the element's own States and Actions. Frame,
	// Children and the tree states (disabled, focusable, focused) are filled
	// in by AccessibilityTree. Base returns a RoleNone node, so the element
	// is not exposed and its children are hoisted.
	AccessibleNode() a11y.Node

	base() *Base
}

// Base implements the element tree. Embed it (by value) in every element
// type and call Init. The zero value is a detached, visible, enabled, clean
// element with a zero frame that does not clip its children.
type Base struct {
	self     Element
	parent   *Base
	children []Element

	frame     driver.Rect
	hidden    bool
	disabled  bool
	clips     bool
	dirty     bool
	focusable bool
	focusKind FocusKind

	defaultMark, cancelMark uint64 // SetDefaultButton, SetCancelButton (buttons.go)

	onInvalidate func()
	router       *Router // set on the root of a window's tree by NewRouter
}

func (b *Base) base() *Base { return b }

// Init binds b to the element that embeds it. It must be called once, with
// the embedding element, before any tree operation; it panics otherwise.
func (b *Base) Init(self Element) {
	switch {
	case self == nil || self.base() != b:
		panic("aqua: Init called with an element that does not embed this Base")
	case b.self != nil:
		panic("aqua: Init called twice")
	}
	b.self = self
}

// Self returns the element that embeds b (nil before Init).
func (b *Base) Self() Element { return b.self }

// Frame returns the frame in the parent's coordinates. A root's frame is in
// the coordinates PaintTree and HitTest work in (the surface's, for a
// window).
func (b *Base) Frame() driver.Rect { return b.frame }

// Bounds returns the frame in the element's own coordinates: the origin is
// (0, 0) and the size is the frame's.
func (b *Base) Bounds() driver.Rect { return driver.Rect{W: b.frame.W, H: b.frame.H} }

// SetFrame implements layout.Item. It invalidates when the frame changes.
func (b *Base) SetFrame(r driver.Rect) {
	if r == b.frame {
		return
	}
	b.frame = r
	b.Invalidate()
}

// FittingSize implements layout.Item. Base returns the zero size; elements
// with content override it.
func (b *Base) FittingSize(driver.Size) driver.Size { return driver.Size{} }

// Paint draws nothing; elements that draw override it.
func (b *Base) Paint(render.Canvas) {}

// HitSelf reports whether p lies in Bounds.
func (b *Base) HitSelf(p driver.Point) bool { return b.Bounds().Contains(p) }

// HandleEvent handles nothing; elements that take input override it.
func (b *Base) HandleEvent(Event) bool { return false }

// Activate does nothing; elements with a primary action override it.
func (b *Base) Activate() bool { return false }

// AccessibleNode returns a RoleNone node: the element is not exposed.
func (b *Base) AccessibleNode() a11y.Node { return a11y.Node{} }

// Hidden reports the element's own hidden flag.
func (b *Base) Hidden() bool { return b.hidden }

// SetHidden hides or shows the element and its subtree. A hidden element is
// neither painted nor hit, and an element in a hidden subtree loses the
// focus, the hover and the capture (Router). It invalidates on a change (a
// newly hidden element invalidates too, so its parent repaints without it).
func (b *Base) SetHidden(hidden bool) {
	if hidden == b.hidden {
		return
	}
	b.hidden = hidden
	b.Invalidate()
	if hidden {
		b.treeChanged()
	}
}

// Enabled reports the element's own enabled flag.
func (b *Base) Enabled() bool { return !b.disabled }

// SetEnabled enables or disables the element. A disabled element is still
// painted and hit, but gets no input: the Router skips it, and an element
// in a disabled subtree loses the focus, the hover and the capture. It
// invalidates on a change, because disabled controls draw differently.
func (b *Base) SetEnabled(enabled bool) {
	if enabled != b.disabled {
		return
	}
	b.disabled = !enabled
	b.Invalidate()
	if !enabled {
		b.treeChanged()
	}
}

// ClipsChildren reports whether the children are clipped to Bounds.
func (b *Base) ClipsChildren() bool { return b.clips }

// SetClipsChildren sets whether the children are clipped to Bounds, in
// painting and in hit-testing. The element's own Paint is never clipped by
// it. It invalidates on a change.
func (b *Base) SetClipsChildren(clips bool) {
	if clips == b.clips {
		return
	}
	b.clips = clips
	b.Invalidate()
}

// VisibleInTree reports whether neither the element nor an ancestor is
// hidden.
func (b *Base) VisibleInTree() bool {
	for e := b; e != nil; e = e.parent {
		if e.hidden {
			return false
		}
	}
	return true
}

// EnabledInTree reports whether neither the element nor an ancestor is
// disabled.
func (b *Base) EnabledInTree() bool {
	for e := b; e != nil; e = e.parent {
		if e.disabled {
			return false
		}
	}
	return true
}

// Dirty reports whether the element was invalidated since PaintTree last
// visited it.
func (b *Base) Dirty() bool { return b.dirty }

// SetInvalidateFunc sets the callback that runs when the tree rooted at b
// goes from clean to dirty (the window calls Surface.Invalidate there,
// S2.2c). Only the callback of the current root runs; a nil f removes it.
// Setting it does not call it: a root that is already dirty (a new element
// whose frame was set) calls it only after its next PaintTree.
func (b *Base) SetInvalidateFunc(f func()) { b.onInvalidate = f }

// Invalidate marks the element and its ancestors dirty. When the root was
// clean, its invalidate callback runs once; further invalidations before the
// next PaintTree only mark elements. v1 repaints the whole window, so the
// marks matter only for this coalescing (dirty rects are v2, PLAN.md §5.1).
//
// Invariant: a dirty element's ancestors are dirty, so the walk stops at the
// first element that already is.
func (b *Base) Invalidate() {
	e := b
	for {
		if e.dirty {
			return
		}
		e.dirty = true
		if e.parent == nil {
			break
		}
		e = e.parent
	}
	if e.onInvalidate != nil {
		e.onInvalidate()
	}
}
