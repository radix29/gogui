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
		n := accessibleNode(c)
		if n.Role == a11y.RoleNone {
			out = accessibleChildren(cb, out)
			continue
		}
		n.Children = accessibleChildren(cb, nil)
		out = append(out, n)
	}
	return out
}
