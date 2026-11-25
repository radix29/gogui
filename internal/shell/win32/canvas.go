//go:build windows

package win32

import (
	"errors"
	"fmt"
	"math"

	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/render/record"
	"github.com/radix29/gogui/internal/shell/win32/com"
	"github.com/radix29/gogui/internal/shell/win32/d2d"
)

// The Direct2D canvas (S2.4d_2): a render.Canvas on the shell's device
// context, used between BeginDraw and EndDraw for one frame. A device has
// one canvas (gpu.canvas, reused by every paint; review P3).
//
//   - State: the transform is kept in Go (want) and sent to the context
//     with SetTransform only before a draw or push, and only when it
//     differs from the one D2D has (sent; drawDC, review P4), so
//     Translate/Restore pairs around nothing cost no call (the surface
//     scale stays in SetDpi, so user space is logical points). Clips are
//     D2D pushes: a sharp rect
//     under an axis-preserving transform is a PushAxisAlignedClip,
//     everything else a layer with a rounded-rect geometry mask. Each Save
//     (and each Push*) remembers the transform and how many pushes were
//     open; Restore (Pop) pops the pushes made since, in LIFO order, and
//     finish pops whatever is left.
//   - Resources: solid brushes, stop collections, gradient brushes and
//     stroke styles come from the device's resource cache (rescache.go),
//     kept across paints and trimmed by release after EndDraw; geometries
//     are made per call and released by release.
//   - Contract violations (render.Canvas, PLAN.md §5.3) are ignored and
//     never reach D2D: non-finite values (also after narrowing to float32),
//     negative sizes, semantic colors, a Restore that doesn't close a Save
//     or a Pop that doesn't close a Push*. Leftover saves are closed by
//     finish. With a recorder (GOGUI_DEBUG=1) every call is also recorded,
//     and finish returns the recorder's End errors.
//   - Group opacity (S2.4e_2): PushOpacity pushes a layer with the clamped
//     opacity and infinite bounds on the same push stack, so Pop and finish
//     close it; an opacity of 1 pushes a state frame only. Shadows are in
//     shadow.go, backdrop blur (S2.4e_3) in blur.go: PushBackdropBlur
//     draws the blurred copy and leaves its clip open until Pop. Every
//     push records what it takes to push it again (canvasPush), since the
//     blur closes the open layers for its copy and reopens them.
//   - Images (S2.4e_4) are in image.go: DrawImage draws a shell image's
//     bitmap, made on first draw and kept until the device goes.
//   - Text (S2.5f): DrawText draws a layout of the shell's text system
//     (canvas.text) with DrawTextLayout. Layouts keep no native object
//     (text.go), so DrawText takes its IDWriteTextLayout from the text
//     system's drawn-layout cache (textcache.go, S5.4k), which rebuilds it
//     from the text, font and width when the layout was not drawn lately
//     and keeps it across frames. Text antialiasing is grayscale
//     for the whole frame (set by newCanvas): no ClearType color fringes.

// canvasFrameKind says which call pushed a canvas state frame.
type canvasFrameKind uint8

const (
	canvasSave canvasFrameKind = iota
	canvasOpacity
	canvasBlur
)

// canvasFrame is one level of the state stack.
type canvasFrame struct {
	kind   canvasFrameKind
	xf     render.Transform
	pushes int // len(canvas.pushes) at the push
}

// pushKind is a D2D push that has to be popped.
type pushKind uint8

const (
	pushClip  pushKind = iota // PushAxisAlignedClip
	pushLayer                 // PushLayer
)

// canvasPush is an open D2D push with what it takes to push it again
// (PushBackdropBlur closes and reopens the open layers, blur.go).
type canvasPush struct {
	kind  pushKind
	xf    d2d.Matrix3x2F       // the transform at the push
	rect  d2d.RectF            // pushClip
	aa    d2d.AntialiasMode    // pushClip
	layer d2d.LayerParameters1 // pushLayer (the mask is one of the frame's geometries)
}

// owned is a resource made during the frame, released by release.
type owned struct {
	what string
	p    com.Ptr
}

// canvas is the D2D render.Canvas. Create it with newCanvas after
// BeginDraw, call finish before EndDraw and release after it.
type canvas struct {
	gpu  *gpu
	dc   d2d.DeviceContext
	mask d2d.DeviceContext // the shadow masks' context (shadow.go)
	fac  d2d.Factory1
	dpi  float32 // the surface's

	xf     render.Transform
	frames []canvasFrame
	pushes []canvasPush
	ended  bool

	res   *resCache // the device's brushes and stroke styles (rescache.go)
	geoms []owned   // the frame's geometries and retired backdrops
	live  bool      // between newCanvas and release

	// The transform D2D needs for the next draw (c.xf narrowed) and the one
	// it has: sent only when a draw or push needs it (drawDC).
	want, sent d2d.Matrix3x2F
	xfSets     int // SetTransform calls (tests)

	target blurTarget // blur.go
	blur   pendingBlur
	blurFx blurEffects
	blurs  int // blurred copies drawn (tests)

	shadows      *shadowCache // the device's (shadow.go)
	maskHits     int          // DrawShadow calls that reused a mask (tests)
	masksMade    int          // masks drawn by this canvas (tests)
	shadowsBaked int          // shadows baked by this canvas (tests)

	images *imageSet // image.go; nil draws no images

	text      *textSystem // the shell's; nil draws no text
	textFrame bool        // text.nextFrame was called for this frame

	rec  *record.Canvas // nil unless debugging
	errs []error        // resource creation failures
}

var _ render.Canvas = (*canvas)(nil)

// newCanvas returns the device's canvas (one per device: its slices and the
// device's caches stay across frames) on g's device context, which is in
// BeginDraw at dpi, for a surface of size logical points, and sets the
// identity transform. debug adds a recorder. Only one canvas of a device
// can be live (between newCanvas and release); paints don't nest.
func newCanvas(g *gpu, size render.Size, dpi float32, debug bool) *canvas {
	dc := g.dc
	c := &g.canvas
	if c.live {
		panic("win32: newCanvas while the device's canvas is live")
	}
	*c = canvas{
		gpu: g, dc: dc, mask: g.mask, fac: g.d2f, dpi: dpi, xf: render.Identity(),
		frames: c.frames[:0], pushes: c.pushes[:0], geoms: c.geoms[:0], errs: c.errs[:0],
		res: &g.res, shadows: &g.shadows, live: true,
	}
	c.res.begin()
	c.shadows.begin()
	if debug {
		if !(size.W >= 0) || math.IsInf(size.W, 0) {
			size.W = 0
		}
		if !(size.H >= 0) || math.IsInf(size.H, 0) {
			size.H = 0
		}
		c.rec = record.New(size)
	}
	c.sendXf(d2d.IdentityMatrix())
	c.want = c.sent
	dc.SetTextAntialiasMode(d2d.TEXT_ANTIALIAS_MODE_GRAYSCALE) // no ClearType fringes (PLAN.md §3)
	return c
}

// finish pops every open clip and layer, resets the transform and ends
// the canvas (later calls are ignored). It returns the recorder's errors
// (nil without a recorder). Call it before EndDraw.
func (c *canvas) finish() error {
	if c.ended {
		return nil
	}
	c.popTo(0)
	c.frames = c.frames[:0]
	c.xf = render.Identity()
	c.want = d2d.IdentityMatrix()
	c.drawDC() // D2D is left at the identity
	c.ended = true
	if c.rec != nil {
		if err := c.rec.End(); err != nil {
			return fmt.Errorf("win32: Paint broke the canvas contract: %w", err)
		}
	}
	return nil
}

// failures returns the resource creation errors of the frame, joined.
func (c *canvas) failures() error { return errors.Join(c.errs...) }

// resources returns the number of resources the canvas holds for the
// frame (tests); the device's caches are not the canvas's.
func (c *canvas) resources() int {
	n := len(c.geoms)
	if !c.blurFx.blur.IsNil() {
		n += 2
	}
	return n
}

// release releases every resource of the frame and trims the device's
// caches; each release must be the last reference, else leak gets the
// rest. Call it after EndDraw.
func (c *canvas) release(leak leakFunc) {
	last := leak.releaseLast
	c.releaseBlur(last)  // before the geometries, which hold retired backdrops
	c.shadows.trim(last) // the cache stays (shadow.go)
	c.res.trim(last)     // so does this one (rescache.go)
	for i := range c.geoms {
		last(c.geoms[i].what, &c.geoms[i].p)
	}
	clear(c.geoms)
	c.geoms = c.geoms[:0]
	c.live = false
}

// fail records a resource creation failure.
func (c *canvas) fail(err error) {
	if len(c.errs) < 8 {
		c.errs = append(c.errs, err)
	}
}

// --- State -----------------------------------------------------------------

func (c *canvas) push(kind canvasFrameKind) {
	c.frames = append(c.frames, canvasFrame{kind: kind, xf: c.xf, pushes: len(c.pushes)})
}

// pop closes the innermost frame if it has one of the kinds.
func (c *canvas) pop(kinds ...canvasFrameKind) {
	if len(c.frames) == 0 {
		return
	}
	top := c.frames[len(c.frames)-1]
	match := false
	for _, k := range kinds {
		match = match || top.kind == k
	}
	if !match {
		return
	}
	c.frames = c.frames[:len(c.frames)-1]
	c.popTo(top.pushes)
	if top.xf != c.xf {
		c.xf = top.xf
		c.want, _ = matrix(top.xf) // every stored transform was narrowed before
	}
}

// addClip pushes an axis-aligned clip under the current transform.
func (c *canvas) addClip(r d2d.RectF, aa d2d.AntialiasMode) {
	c.drawDC().PushAxisAlignedClip(r, aa)
	c.pushes = append(c.pushes, canvasPush{kind: pushClip, xf: c.want, rect: r, aa: aa})
}

// addLayer pushes a layer under the current transform.
func (c *canvas) addLayer(p d2d.LayerParameters1) {
	c.drawDC().PushLayer(&p)
	c.pushes = append(c.pushes, canvasPush{kind: pushLayer, xf: c.want, layer: p})
}

// popTo pops D2D pushes, innermost first, until n are left.
func (c *canvas) popTo(n int) {
	for len(c.pushes) > n {
		switch c.pushes[len(c.pushes)-1].kind {
		case pushClip:
			c.dc.PopAxisAlignedClip()
		case pushLayer:
			c.dc.PopLayer()
		}
		c.pushes = c.pushes[:len(c.pushes)-1]
	}
}

// setTransform makes t current, unless it doesn't narrow. D2D gets it
// with the next draw or push (drawDC), so transforms nothing draws under
// cost no call (review P4).
func (c *canvas) setTransform(t render.Transform) {
	m, ok := matrix(t)
	if !ok {
		return
	}
	c.xf = t
	c.want = m
}

// drawDC returns the device context with the current transform sent:
// every call that draws or pushes under the canvas's transform goes
// through it.
func (c *canvas) drawDC() d2d.DeviceContext {
	if c.want != c.sent {
		c.sendXf(c.want)
	}
	return c.dc
}

// sendXf sets D2D's transform to m now (the blur's own transforms go
// through it too, so sent stays what D2D has).
func (c *canvas) sendXf(m d2d.Matrix3x2F) {
	c.dc.SetTransform(m)
	c.sent = m
	c.xfSets++
}

// Save implements render.Canvas.
func (c *canvas) Save() {
	if c.rec != nil {
		c.rec.Save()
	}
	if !c.ended {
		c.push(canvasSave)
	}
}

// Restore implements render.Canvas.
func (c *canvas) Restore() {
	if c.rec != nil {
		c.rec.Restore()
	}
	if !c.ended {
		c.pop(canvasSave)
	}
}

// Translate implements render.Canvas.
func (c *canvas) Translate(dx, dy float64) {
	if c.rec != nil {
		c.rec.Translate(dx, dy)
	}
	if !c.ended {
		c.setTransform(c.xf.Pre(render.Translation(dx, dy)))
	}
}

// Scale implements render.Canvas.
func (c *canvas) Scale(sx, sy float64) {
	if c.rec != nil {
		c.rec.Scale(sx, sy)
	}
	if !c.ended {
		c.setTransform(c.xf.Pre(render.Scaling(sx, sy)))
	}
}

// Concat implements render.Canvas.
func (c *canvas) Concat(t render.Transform) {
	if c.rec != nil {
		c.rec.Concat(t)
	}
	if !c.ended {
		c.setTransform(c.xf.Pre(t))
	}
}

// axisPreserving reports whether t maps axis-aligned rects to axis-aligned
// rects (no rotation or skew), so PushAxisAlignedClip clips exactly.
func axisPreserving(t render.Transform) bool { return t.B == 0 && t.C == 0 }

// ClipRoundedRect implements render.Canvas. An empty rect clips
// everything away, and so does an invalid one (render.ValidRect), as in
// the recorder.
func (c *canvas) ClipRoundedRect(r render.Rect, radii render.Radii) {
	if c.rec != nil {
		c.rec.ClipRoundedRect(r, radii)
	}
	if c.ended {
		return
	}
	rf, ok := rectF(r)
	if !ok {
		c.addClip(d2d.RectF{}, d2d.ANTIALIAS_MODE_ALIASED)
		return
	}
	radii = radii.Clamped(r)
	if radii.IsZero() && axisPreserving(c.xf) {
		c.addClip(rf, d2d.ANTIALIAS_MODE_PER_PRIMITIVE)
		return
	}
	g, ok := c.roundedGeometry(r, rf, radii)
	if !ok {
		// Without a mask, clip to the bounds rather than not at all.
		c.addClip(rf, d2d.ANTIALIAS_MODE_PER_PRIMITIVE)
		return
	}
	c.addLayer(d2d.LayerParameters1{
		ContentBounds:     d2d.InfiniteRect(),
		GeometricMask:     g.Ptr,
		MaskAntialiasMode: d2d.ANTIALIAS_MODE_PER_PRIMITIVE,
		MaskTransform:     d2d.IdentityMatrix(),
		Opacity:           1,
	})
}

// PushOpacity implements render.Canvas: a layer with the clamped opacity
// (NaN counts as 0) and infinite bounds, closed by Pop or finish. An
// opacity of 1 only saves.
func (c *canvas) PushOpacity(alpha float64) {
	if c.rec != nil {
		c.rec.PushOpacity(alpha)
	}
	if c.ended {
		return
	}
	c.push(canvasOpacity)
	a := render.Opacity(alpha)
	if a == 1 {
		return
	}
	c.addLayer(d2d.LayerParameters1{
		ContentBounds:     d2d.InfiniteRect(),
		MaskAntialiasMode: d2d.ANTIALIAS_MODE_PER_PRIMITIVE,
		MaskTransform:     d2d.IdentityMatrix(),
		Opacity:           float32(a),
	})
}

// Pop implements render.Canvas.
func (c *canvas) Pop() {
	if c.rec != nil {
		c.rec.Pop()
	}
	if !c.ended {
		c.pop(canvasOpacity, canvasBlur)
	}
}
