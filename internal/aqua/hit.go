package aqua

import "github.com/radix29/gogui/internal/driver"

// HitTest returns the topmost visible element of the tree rooted at e under
// p, and p converted into that element's coordinates. p is in the
// coordinates of e's parent, as for PaintTree. It returns nil and the zero
// point when nothing is hit.
//
// Children are tested before their parent, last child first (the reverse of
// the paint order), and an element is hit when its HitSelf accepts the
// point. Hidden subtrees are skipped. An element with ClipsChildren passes
// no point outside its Bounds to its children; otherwise a child that sticks
// out of its parent is hit there, as it is painted there. Disabled elements
// are hit like any other; input routing (S2.2b) decides what they get.
func HitTest(e Element, p driver.Point) (Element, driver.Point) {
	b := e.base()
	b.mustSelf()
	if b.hidden {
		return nil, driver.Point{}
	}
	local := p.Sub(b.frame.Origin())
	if !b.clips || b.Bounds().Contains(local) {
		for i := len(b.children) - 1; i >= 0; i-- {
			if hit, q := HitTest(b.children[i], local); hit != nil {
				return hit, q
			}
		}
	}
	if e.HitSelf(local) {
		return e, local
	}
	return nil, driver.Point{}
}

// ToRoot converts p from e's own coordinates into the coordinates of its
// root's parent (the ones PaintTree and HitTest use).
func ToRoot(e Element, p driver.Point) driver.Point { return e.base().tree().origin.Add(p) }

// FromRoot converts p from the coordinates of e's root's parent into e's
// own; it is the inverse of ToRoot.
func FromRoot(e Element, p driver.Point) driver.Point { return p.Sub(e.base().tree().origin) }
