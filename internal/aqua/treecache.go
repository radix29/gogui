package aqua

import (
	"sync/atomic"

	"github.com/radix29/gogui/internal/driver"
)

// The tree context memo (review P9). Painting a control asks for its
// window, appearance, scale, router, enabled state and root-space origin,
// each an O(depth) walk to the root, several times per control and frame.
// treeCache memoises what those walks compute, on each Base, keyed by
// treeEpoch: every change that could alter an answer (a parent link, a
// frame, the hidden or disabled flag, a root's router) bumps the epoch, so
// every memo is stale at once and is refilled on the next query from its
// parent's, which is itself memoised. Between changes (a steady frame, the
// paint of a whole tree) each query is O(1) after the first per element.
//
// Each tree is used on its UI thread only, but the epoch is shared by every
// tree in the process (a test runs several drivers, one after the other
// or with a previous one still winding down), so it is atomic: a bump from
// another tree only makes a memo refill early, and no bump is lost.

// treeEpoch counts the changes to the state treeCache memoises. A zero
// memo is stale, because the epoch starts at 1.
var treeEpoch atomic.Uint64

func init() { treeEpoch.Store(1) }

// treeChange marks every memo stale. Call it after changing a parent link,
// a frame, a hidden or disabled flag or a root's router.
func treeChange() { treeEpoch.Add(1) }

// treeCache is the memo on a Base, valid while epoch == treeEpoch.
type treeCache struct {
	epoch   uint64
	root    *Base        // the root of the tree
	origin  driver.Point // ToRoot(e, (0, 0))
	enabled bool         // EnabledInTree
	visible bool         // VisibleInTree
}

// tree returns b's memo, refilled first when it is stale.
func (b *Base) tree() *treeCache {
	c := &b.tc
	epoch := treeEpoch.Load()
	if c.epoch == epoch {
		return c
	}
	if b.parent == nil {
		*c = treeCache{epoch: epoch, root: b, origin: b.frame.Origin(), enabled: !b.disabled, visible: !b.hidden}
		return c
	}
	p := b.parent.tree()
	*c = treeCache{
		epoch:   epoch, // never newer than what p was read at
		root:    p.root,
		origin:  p.origin.Add(b.frame.Origin()),
		enabled: p.enabled && !b.disabled,
		visible: p.visible && !b.hidden,
	}
	return c
}
