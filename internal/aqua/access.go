package aqua

import (
	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/driver"
)

// AccessibilityTree builds the accessibility tree of the tree rooted at
// root (a window's root element): one node per visible exposed element.
//
// Each node starts as the element's AccessibleNode. The tree then sets
// Frame to the element's frame in the coordinates of the root's parent (the
// surface's, for a window), adds StateDisabled when the element or an
// ancestor is disabled, StateFocusable and ActionFocus for a focusable
// element that is enabled, and StateFocused for the focused element of the
// root's Router. Children are the exposed descendants in tree order: an
// element with RoleNone is left out and its children are hoisted to its
// nearest exposed ancestor. Hidden subtrees are left out. The root's node is
// always returned, even with RoleNone, so the result is never nil.
//
// The tree is a snapshot: it does not follow later changes.
func AccessibilityTree(root Element) *a11y.Node {
	b := root.base()
	b.mustSelf()
	n := accessibleNode(root)
	n.Children = accessibleChildren(b, nil)
	return n
}

func accessibleNode(e Element) *a11y.Node {
	b := e.base()
	n := e.AccessibleNode()
	n.Frame = driver.RectOf(ToRoot(e, driver.Point{}), b.frame.Size())
	n.Children = nil
	enabled := b.EnabledInTree()
	if !enabled {
		n.States |= a11y.StateDisabled
	}
	if b.focusable && enabled {
		n.States |= a11y.StateFocusable
		n.Actions |= a11y.ActionFocus
	}
	if b.HasFocus() {
		n.States |= a11y.StateFocused
	}
	return &n
}

// accessibleChildren appends the nodes of b's visible exposed descendants
// to out.
func accessibleChildren(b *Base, out []*a11y.Node) []*a11y.Node {
	for _, c := range b.children {
		cb := c.base()
		if cb.hidden {
			continue
		}
		if ov, ok := c.(accessibleOverlay); ok {
			out = append(out, ov.overlayNodes()...)
			continue
		}
		n := accessibleNode(c)
		if n.Role == a11y.RoleNone {
			out = accessibleChildren(cb, out)
			continue
		}
		n.Children = accessibleChildren(cb, nil)
		if ai, ok := c.(accessibleItems); ok {
			n.Children = append(n.Children, itemNodes(c, ai)...)
		}
		out = append(out, n)
	}
	return out
}

// accessibleOverlay is implemented by an element that shows a tree of its
// own in place of itself: the window's sheet layer (S5.7c). overlayNodes
// returns the nodes of that tree's exposed elements, in the window's
// coordinates; the layer itself is not exposed.
type accessibleOverlay interface {
	overlayNodes() []*a11y.Node
}

// accessibleItems is implemented by an element whose exposed children are
// not elements of their own: the menu's items (S5.6c). AccessibleItems
// returns their nodes with frames in the element's own points and without
// children; the tree appends them after the element's child elements, in
// window coordinates.
type accessibleItems interface {
	AccessibleItems() []a11y.Node
}

// itemNodes returns e's item nodes in window coordinates.
func itemNodes(e Element, ai accessibleItems) []*a11y.Node {
	origin := ToRoot(e, driver.Point{})
	items := ai.AccessibleItems()
	out := make([]*a11y.Node, len(items))
	for i := range items {
		n := items[i]
		n.Frame = n.Frame.Offset(origin)
		n.Children = nil
		out[i] = &n
	}
	return out
}
