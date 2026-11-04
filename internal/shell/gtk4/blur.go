package gtk4

import (
	"math"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// The backdrop blur (S4.2f, as Win32's S2.4e_3). GSK has no backdrop
// filter: a blur node blurs its child, not what lies below it. So the
// canvas draws by segments, child snapshots of the paint's GtkSnapshot,
// and a blur re-appends what is drawn so far:
//
//   - newCanvas starts the first segment (snapshotter.begin) and every
//     draw goes into the current one. finish ends the last one into the
//     paint's snapshot (end) and releases the earlier ones' nodes.
//   - PushBackdropBlur closes every open frame in the segment (unwind,
//     innermost first), ends the segment (cut: gtk_snapshot_free_to_node,
//     the node appended to the paint's snapshot and kept in c.nodes),
//     starts the next one and opens the frames again (rewind: the opacity
//     push, the save, then the frame's transforms and clips in order). So
//     the state is what it was, and an opacity group's content before the
//     blur is composed as a group of its own (the blur sees it at the
//     group's opacity), as on Win32.
//   - Then, in the blur's frame and under its rounded clip of r (which
//     stays pushed until Pop, so what is drawn on top is clipped too),
//     every node so far is appended again blurred: under the inverse of
//     the current transform (the nodes are in the paint's root space), a
//     blur push with GSK's blur radius (σ = radius/2, as render's), the
//     radius scaled by √|det| of the transform (so it is in user space, as
//     on Win32), and a clip to the cover rect: the root bounds of r padded
//     by 3σ, within the surface padded the same.
//   - The edge rule (Win32's hard borders): GSK reads transparency past a
//     node's content, so a blur at the surface's edge would darken there.
//     Where the cover leaves the surface, the nodes are appended again
//     mirrored at that edge (and at a corner where it leaves past two), so
//     the blur reads the content reflected; for even content that is the
//     hard border.
//   - A zero (or negative, NaN, infinite) radius, a shape with no area or
//     not finite, a singular transform, a shape outside the surface and
//     values that don't narrow to float32 push only the clip, as on Win32
//     (an empty shape clips everything away, an invalid one is ignored).
//     That is also the fallback: no step after the cut can fail (GLib
//     aborts when memory runs out), and render.Canvas's blur has no tint
//     of its own; what the caller draws on top (the tint) still draws.

// renderNode is a GskRenderNode a segment ended in (gskNode,
// canvas_linux.go; a fake in the host tests). release drops the canvas's
// reference.
type renderNode interface{ release() }

// backdrop is how a blur draws the nodes again, all in the paint's root
// space: the inverse of the transform at the push, GSK's blur radius, the
// cover clip and the edge mirrors (the identity not included).
type backdrop struct {
	inv     [6]float32
	radius  float64
	cover   gtk.GrapheneRect
	mirrors [][6]float32
}

// maxBlur bounds a blur radius in the root, in points, far beyond any
// surface and far below where float32 sums overflow.
const maxBlur = 1e6

// backdropArgs returns the backdrop of PushBackdropBlur(r, _, radius)
// under the current transform; ok is false when the blur only clips.
func (c *canvas) backdropArgs(r render.Rect, radius float64) (backdrop, bool) {
	radius = render.BlurRadius(radius)
	if radius == 0 {
		return backdrop{}, false
	}
	if _, ok := boxOK(r); !ok {
		return backdrop{}, false
	}
	surf := render.Rect{W: c.size.W, H: c.size.H}
	if _, ok := boxOK(surf); !ok {
		return backdrop{}, false
	}
	inv, ok := c.ctm.Invert()
	if !ok {
		return backdrop{}, false
	}
	var b backdrop
	for i, v := range [6]float64{inv.A, inv.B, inv.C, inv.D, inv.E, inv.F} {
		if b.inv[i], ok = f32(v); !ok {
			return backdrop{}, false
		}
	}
	rr := radius * math.Sqrt(math.Abs(c.ctm.Det()))
	if f, ok := f32(rr); !ok || f <= 0 || rr > maxBlur {
		return backdrop{}, false
	}
	b.radius = rr
	bounds := c.ctm.ApplyRect(r)
	if bounds.Intersect(surf).Empty() {
		return backdrop{}, false
	}
	grow := driver.UniformInsets(-1.5 * rr) // 3σ, σ = rr/2
	cover := bounds.Inset(grow).Intersect(surf.Inset(grow))
	if b.cover, ok = rectF(cover); !ok || cover.Empty() {
		return backdrop{}, false
	}
	b.mirrors = mirrors(cover, surf)
	return b, true
}

// mirrors returns the reflections at the edges of surf (at the origin)
// that cover crosses, and at the corners where it crosses two: the
// transforms x → -x (left), x → 2W - x (right), and the same for y.
func mirrors(cover, surf render.Rect) [][6]float32 {
	type flip struct{ k, d float32 } // v → k·v + d
	xs := []flip{{1, 0}}
	if cover.X < 0 {
		xs = append(xs, flip{-1, 0})
	}
	if cover.MaxX() > surf.W {
		xs = append(xs, flip{-1, float32(2 * surf.W)})
	}
	ys := []flip{{1, 0}}
	if cover.Y < 0 {
		ys = append(ys, flip{-1, 0})
	}
	if cover.MaxY() > surf.H {
		ys = append(ys, flip{-1, float32(2 * surf.H)})
	}
	var out [][6]float32
	for _, y := range ys {
		for _, x := range xs {
			if x.k == 1 && y.k == 1 {
				continue
			}
			out = append(out, [6]float32{x.k, 0, 0, y.k, x.d, y.d})
		}
	}
	return out
}

// PushBackdropBlur implements render.Canvas (see above).
func (c *canvas) PushBackdropBlur(r render.Rect, radii render.Radii, radius float64) {
	if c.rec != nil {
		c.rec.PushBackdropBlur(r, radii, radius)
	}
	if c.ended {
		return
	}
	b, ok := c.backdropArgs(r, radius)
	if ok {
		c.cut()
	}
	c.open(frameBlur)
	c.clip(r, radii)
	if ok && len(c.nodes) > 0 {
		c.drawBackdrop(b)
	}
}

// cut ends the current segment with every frame closed, keeps its node
// (when it drew something) and opens the frames again in the next one.
func (c *canvas) cut() {
	for i := len(c.frames) - 1; i >= 0; i-- {
		c.unwind(&c.frames[i])
	}
	if n, ok := c.sn.cut(); ok {
		c.nodes = append(c.nodes, n)
	}
	for i := range c.frames {
		c.rewind(&c.frames[i])
	}
}

// drawBackdrop appends every node so far blurred, in the root space, then
// mirrored at the edges b.mirrors names.
func (c *canvas) drawBackdrop(b backdrop) {
	c.sn.save()
	for _, op := range transformOps(b.inv) {
		c.emit(op)
	}
	c.sn.pushBlur(b.radius)
	c.sn.pushRoundedClip(gtk.RoundedRect{Bounds: b.cover})
	c.appendNodes()
	for _, m := range b.mirrors {
		c.sn.save()
		for _, op := range transformOps(m) {
			c.emit(op)
		}
		c.appendNodes()
		c.sn.restore()
	}
	c.sn.pop()
	c.sn.pop()
	c.sn.restore()
}

func (c *canvas) appendNodes() {
	for _, n := range c.nodes {
		c.sn.appendNode(n)
	}
}
