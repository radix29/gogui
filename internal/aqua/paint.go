package aqua

import (
	"math"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
)

// PaintTree paints the tree rooted at e on c, in the coordinates of e's
// parent (for a window root, the surface's). A visible element is translated
// to its frame's origin and painted; then its children are painted in order,
// so later children are on top. A hidden element and its subtree are
// skipped. An element with ClipsChildren clips its children (not itself) to
// its Bounds. Each element is drawn inside its own Save/Restore, so state
// its Paint leaves behind (a transform, a clip) does not leak to its
// children or siblings.
//
// Culling (PLAN.md S3.5d): below a clipping ancestor, the clip is the
// intersection of the Bounds of all clipping ancestors. An element whose
// frame, grown by paintOverflow on every side, does not overlap the clip
// is not painted, since nothing it draws could show. If it clips its
// children too, its whole subtree is skipped (the children are cut to its
// bounds, which lie outside the clip); otherwise its children are still
// visited, because a non-clipping element's children may lie outside its
// frame. An empty clip (a zero-size clipping ancestor) culls everything
// below it. Elements under no clipping ancestor are never culled, the root
// included. v1 repaints the whole window on every frame, so a culled
// element simply paints again once it scrolls into view.
//
// Every element of the tree, hidden, culled or not, is clean afterwards. A
// flag is cleared before the element paints, so an element that
// invalidates during Paint (an animation asking for the next frame) stays
// dirty and its root's callback runs again. The children are read once
// before they are painted, so a Paint that changes the tree does not break
// the traversal.
func PaintTree(c render.Canvas, e Element) {
	paintTree(c, e, nil, paintOverflow())
}

// paintOverflow returns how far an element may draw outside its own frame,
// in points: the margin PaintTree's culling grows a frame by, so that
// nothing visible is ever culled. It is the larger of
//
//   - the focus ring's outer edge outside the shape: FocusRingBand starts at
//     FocusRingOffset + FocusRingWidth and twice as wide while animating in,
//     so at most FocusRingOffset + 3·FocusRingWidth (the shape is at most
//     the element's bounds; a slider's knob stays inside its frame);
//   - the widest outer shadow (bezel and slider knob, every control size):
//     |offset| + spread + 2·blur, the Gaussian (σ = blur/2) being invisible
//     beyond 4σ.
//
// It is computed from the current theme tokens, plus one point for glyph
// overhang and anti-aliasing.
func paintOverflow() float64 {
	m := &tokens.Metrics
	ct := &tokens.Controls
	ring := max(0, m.FocusRingOffset+3*m.FocusRingWidth)
	shadow := 0.0
	for cs := range len(m.BezelShadowBlur) {
		bezel := math.Abs(m.BezelShadowOffsetY[cs]) + max(0, m.BezelShadowSpread[cs]) + 2*m.BezelShadowBlur[cs]
		knob := math.Abs(ct.SliderKnobShadowOffsetY[cs]) + 2*ct.SliderKnobShadowBlur[cs]
		shadow = max(shadow, bezel, knob)
	}
	return max(ring, shadow) + 1
}

// paintTree paints e as PaintTree does. clip is the visible rect in the
// coordinates of e's parent (nil: no clipping ancestor) and margin the
// paintOverflow margin.
func paintTree(c render.Canvas, e Element, clip *driver.Rect, margin float64) {
	b := e.base()
	b.mustSelf()
	if b.hidden {
		clean(b)
		return
	}
	culled := clip != nil && !overlaps(b.frame.Inset(driver.UniformInsets(-margin)), *clip)
	if culled && (b.clips || len(b.children) == 0) {
		clean(b)
		return
	}
	b.dirty = false
	c.Save()
	c.Translate(b.frame.X, b.frame.Y)
	if !culled {
		c.Save()
		e.Paint(c)
		c.Restore()
	}
	if len(b.children) > 0 {
		var inner *driver.Rect
		if clip != nil {
			r := clip.Offset(driver.Point{X: -b.frame.X, Y: -b.frame.Y})
			inner = &r
		}
		if b.clips {
			c.ClipRoundedRect(b.Bounds(), render.Radii{})
			r := b.Bounds()
			if inner != nil {
				r = clipIntersect(*inner, r)
			}
			inner = &r
		}
		for _, ch := range b.Children() {
			paintTree(c, ch, inner, margin)
		}
	}
	c.Restore()
}

// clipIntersect returns the intersection of two clip rects, empty (zero
// size) when they do not overlap.
func clipIntersect(a, b driver.Rect) driver.Rect {
	if a.Empty() || b.Empty() {
		return driver.Rect{}
	}
	return a.Intersect(b)
}

// overlaps reports whether r (non-empty) and the clip overlap with a
// positive area; nothing overlaps an empty clip.
func overlaps(r, clip driver.Rect) bool {
	if clip.Empty() || r.Empty() {
		return false
	}
	return r.X < clip.MaxX() && clip.X < r.MaxX() && r.Y < clip.MaxY() && clip.Y < r.MaxY()
}

// clean clears the dirty flags of a subtree that is not painted.
func clean(b *Base) {
	b.dirty = false
	for _, ch := range b.children {
		clean(ch.base())
	}
}
