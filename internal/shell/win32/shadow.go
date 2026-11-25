//go:build windows

package win32

import (
	"math"

	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/win32/com"
	"github.com/radix29/gogui/internal/shell/win32/d2d"
	"github.com/radix29/gogui/internal/shell/win32/dxgi"
)

// Box shadows (S2.4e_2), CSS box-shadow semantics.
//
//   - The mask: on the shell's mask context (gpu.mask, so the main context
//     never changes target while clips or layers are pushed), at the surface
//     DPI, an opaque bitmap of the shadow shape in the shape's own
//     coordinates. Outer: the shape grown by Spread (radii grown with it,
//     sharp corners stay sharp), padded by 3σ. Inset: a ring, a rect around
//     the shape (grown by 3σ plus the offset, so the shifted ring still
//     covers the shape) with the shape shrunk by Spread cut out.
//   - The shadow: the Shadow effect gets the mask, σ = Blur/2 (DIPs) and the
//     color; its output is baked (S6.2e) into a bitmap of the mask's size
//     and DPI on the mask context, once, and that bitmap is drawn at the
//     shape's origin plus Offset under the current transform, so the
//     shadow follows translations, scales and rotations. The mask's padding
//     holds the blur's 3σ, so the bitmap loses nothing that shows. Baking
//     takes the blur off the frame: before it the effect ran on every
//     EndDraw, about 0.4 ms of the gallery's 1 ms submit on the host.
//   - The clip: a geometry layer, the outside of the shape (a rect around
//     the drawn shadow with the shape cut out) for outer shadows, the shape
//     itself for inset ones.
//   - The cache (S6.2e) is the device's (gpu.shadows), kept across frames:
//     masks by shape size, radii, spread, blur, inset, the inset ring's
//     margin and the DPI; baked shadows by mask and color. A steady frame
//     makes no bitmap and runs no effect: before the cache the masks'
//     CreateBitmap and their EndDraw on the mask context were about half of
//     the gallery's PaintTree (S6.2c). After each paint (trim, from
//     canvas.release, after EndDraw) an entry unused for shadowKeep paints
//     goes, and so do the least recently used masks (with their shadows)
//     beyond shadowMaxMasks or shadowMaxPixels, never one the paint used.
//     The device's release (device loss, the last surface closed) frees
//     the rest.
//   - A semantic or transparent color, a shape with no area, an outer shape
//     shrunk away by a negative spread, or values that don't narrow draw
//     nothing.

// maxMaskPx bounds a mask's side in pixels (D3D11's texture limit).
const maxMaskPx = 16384

// maskKey identifies a cached shadow mask.
type maskKey struct {
	w, h   float64
	radii  render.Radii
	spread float64
	blur   float64
	ext    float64 // the inset ring's margin around the shape (0 for outer)
	inset  bool
	dpi    float32 // the mask's (the surface's)
}

// shadowMask is a mask bitmap, its size in pixels and where its origin
// lies relative to the shape's top-left corner, in user-space points.
type shadowMask struct {
	bmp    d2d.Bitmap1
	pw, ph uint32
	origin render.Point
}

// shadowKey identifies a baked shadow.
type shadowKey struct {
	mask  maskKey
	color d2d.ColorF
}

// The cache's bounds (shadowCache.trim): an entry unused for shadowKeep
// paints goes (4 s of a 60 Hz animation in one window); beyond
// shadowMaxMasks masks or shadowMaxPixels pixels of masks and baked shadows
// (32 MB) the least recently used masks go.
const (
	shadowKeep      = 240
	shadowMaxMasks  = 256
	shadowMaxPixels = 8 << 20
)

// shadowCache is the device's masks and baked shadows, kept across frames.
type shadowCache struct {
	paint  uint64 // counts the canvases (newCanvas)
	masks  map[maskKey]*cachedMask
	baked  map[shadowKey]*cachedShadow
	pixels int                 // of the masks and baked shadows
	brush  d2d.SolidColorBrush // on the mask context
}

type cachedMask struct {
	shadowMask
	used uint64 // the last paint that drew it
}

// cachedShadow is a baked shadow: its bitmap, the size of its mask's.
type cachedShadow struct {
	bmp  d2d.Bitmap1
	used uint64
}

// begin starts a paint (newCanvas).
func (sc *shadowCache) begin() {
	sc.paint++
	if sc.masks == nil {
		sc.masks = map[maskKey]*cachedMask{}
		sc.baked = map[shadowKey]*cachedShadow{}
	}
}

// trim drops what the bounds rule out (the file comment). A shadow is
// drawn only with its mask, so a mask that goes takes its shadows along.
func (sc *shadowCache) trim(last func(what string, p *com.Ptr)) {
	stale := func(used uint64) bool { return used+shadowKeep < sc.paint }
	for k, m := range sc.masks {
		if stale(m.used) {
			sc.dropMask(k, last)
		}
	}
	for len(sc.masks) > shadowMaxMasks || sc.pixels > shadowMaxPixels {
		var oldest maskKey
		at := uint64(math.MaxUint64)
		for k, m := range sc.masks {
			if m.used < at {
				oldest, at = k, m.used
			}
		}
		if at >= sc.paint {
			break // every one drawn by this paint
		}
		sc.dropMask(oldest, last)
	}
	for k, b := range sc.baked {
		if stale(b.used) {
			sc.dropShadow(k, b, last)
		}
	}
}

func (sc *shadowCache) dropShadow(k shadowKey, b *cachedShadow, last func(what string, p *com.Ptr)) {
	m := sc.masks[k.mask]
	sc.pixels -= int(m.pw) * int(m.ph)
	last("ID2D1Bitmap1 (shadow)", &b.bmp.Ptr)
	delete(sc.baked, k)
}

// dropMask releases a mask and every shadow baked from it.
func (sc *shadowCache) dropMask(k maskKey, last func(what string, p *com.Ptr)) {
	for bk, b := range sc.baked {
		if bk.mask == k {
			sc.dropShadow(bk, b, last)
		}
	}
	m := sc.masks[k]
	sc.pixels -= int(m.pw) * int(m.ph)
	last("ID2D1Bitmap1 (shadow mask)", &m.bmp.Ptr)
	delete(sc.masks, k)
}

// release frees every entry and the mask brush (the device's release).
func (sc *shadowCache) release(last func(what string, p *com.Ptr)) {
	for k := range sc.masks {
		sc.dropMask(k, last)
	}
	if !sc.brush.IsNil() {
		last("ID2D1SolidColorBrush (mask)", &sc.brush.Ptr)
	}
}

// len returns the number of cached masks and baked shadows (tests).
func (sc *shadowCache) len() (masks, baked int) { return len(sc.masks), len(sc.baked) }

// grow returns r grown by d on every side (shrunk for negative d).
func grow(r render.Rect, d float64) render.Rect {
	return render.Rect{X: r.X - d, Y: r.Y - d, W: r.W + 2*d, H: r.H + 2*d}
}

// growRadii returns rd grown by d, sharp corners kept sharp and none below
// 0 (CSS spread), clamped to r.
func growRadii(rd render.Radii, d float64, r render.Rect) render.Radii {
	g := func(v float64) float64 {
		if v <= 0 {
			return 0
		}
		return math.Max(v+d, 0)
	}
	return render.Radii{TopLeft: g(rd.TopLeft), TopRight: g(rd.TopRight),
		BottomRight: g(rd.BottomRight), BottomLeft: g(rd.BottomLeft)}.Clamped(r)
}

// DrawShadow implements render.Canvas.
func (c *canvas) DrawShadow(r render.Rect, radii render.Radii, s render.Shadow) {
	if c.rec != nil {
		c.rec.DrawShadow(r, radii, s)
	}
	if c.ended || s.IsSemantic() {
		return
	}
	s = s.Normalized()
	color := colorF(s.Color)
	if color.A == 0 {
		return
	}
	if _, ok := rectF(r); !ok || r.W == 0 || r.H == 0 {
		return
	}
	radii = radii.Clamped(r)
	key := maskKey{w: r.W, h: r.H, radii: radii, spread: s.Spread, blur: s.Blur, inset: s.Inset, dpi: c.dpi}
	if s.Inset {
		key.ext = 3*s.Blur/2 + math.Abs(s.Offset.X) + math.Abs(s.Offset.Y) + math.Max(-s.Spread, 0) + 1
	}
	m, ok := c.shadowMask(key)
	if !ok {
		return
	}
	at, ok := point2F(render.Point{X: r.X + s.Offset.X + m.origin.X, Y: r.Y + s.Offset.Y + m.origin.Y})
	if !ok {
		return
	}
	b, ok := c.shadow(shadowKey{mask: key, color: color}, m)
	if !ok {
		return
	}
	clip, ok := c.shadowClip(r, radii, s)
	if !ok {
		return
	}
	c.drawDC().PushLayer(&d2d.LayerParameters1{
		ContentBounds:     d2d.InfiniteRect(),
		GeometricMask:     clip.Ptr,
		MaskAntialiasMode: d2d.ANTIALIAS_MODE_PER_PRIMITIVE,
		MaskTransform:     d2d.IdentityMatrix(),
		Opacity:           1,
	})
	c.drawDC().DrawImage(b.Image(), &at, nil, d2d.INTERPOLATION_MODE_LINEAR, d2d.COMPOSITE_MODE_SOURCE_OVER)
	c.dc.PopLayer()
}

// shadowClip returns the layer geometry that keeps an outer shadow outside
// the shape and an inset one inside it.
func (c *canvas) shadowClip(r render.Rect, radii render.Radii, s render.Shadow) (d2d.Geometry, bool) {
	rf, _ := rectF(r) // checked by DrawShadow
	if s.Inset {
		return c.roundedGeometry(r, rf, radii)
	}
	// A rect around everything the shadow can reach, the shape cut out.
	var p render.Path
	reach := math.Abs(s.Spread) + 3*s.Blur/2 + math.Abs(s.Offset.X) + math.Abs(s.Offset.Y) + 2
	addRoundedRect(&p, grow(r, reach), render.Radii{})
	addRoundedRect(&p, r, radii)
	g, ok := c.pathGeometryMode(&p, d2d.FILL_MODE_ALTERNATE)
	return g.Geometry(), ok
}

// shadowMask returns the cached mask for key, drawing it on the mask
// context if there is none yet.
func (c *canvas) shadowMask(key maskKey) (shadowMask, bool) {
	sc := c.shadows
	if m, ok := sc.masks[key]; ok {
		c.maskHits++
		m.used = sc.paint
		return m.shadowMask, true
	}
	shape := render.Rect{W: key.w, H: key.h}
	pad := 3*key.blur/2 + 1 // 3σ, and a pixel for the antialiased edge
	var bounds render.Rect  // the mask's extent in the shape's coordinates
	var path render.Path    // the mask's outline, unless fill is set
	var fill *render.Rect   // a uniform rounded rect drawn directly
	var fillRadius float64
	if key.inset {
		bounds = grow(shape, key.ext)
		addRoundedRect(&path, bounds, render.Radii{})
		if hole := grow(shape, -key.spread); hole.W > 0 && hole.H > 0 {
			addRoundedRect(&path, hole, growRadii(key.radii, -key.spread, hole))
		}
	} else {
		g := grow(shape, key.spread)
		if !(g.W > 0) || !(g.H > 0) {
			return shadowMask{}, false
		}
		gr := growRadii(key.radii, key.spread, g)
		bounds = grow(g, pad)
		if gr.IsUniform() {
			fill, fillRadius = &g, gr.TopLeft
		} else {
			addRoundedRect(&path, g, gr)
		}
	}
	dpi := key.dpi
	if !(dpi > 0) {
		dpi = 96
	}
	scale := float64(dpi) / 96
	pw, ph := math.Ceil(bounds.W*scale), math.Ceil(bounds.H*scale)
	if !(pw >= 1 && pw <= maxMaskPx && ph >= 1 && ph <= maxMaskPx) {
		return shadowMask{}, false
	}
	xf, ok := matrix(render.Translation(-bounds.X, -bounds.Y))
	if !ok {
		return shadowMask{}, false
	}
	var geom d2d.Geometry
	var rr d2d.RoundedRect
	if fill != nil {
		f, ok := rectF(*fill)
		if !ok {
			return shadowMask{}, false
		}
		rr = d2d.RoundedRect{Rect: f, RadiusX: float32(fillRadius), RadiusY: float32(fillRadius)}
	} else {
		g, ok := c.pathGeometryMode(&path, d2d.FILL_MODE_ALTERNATE)
		if !ok {
			return shadowMask{}, false
		}
		geom = g.Geometry()
	}
	if sc.brush.IsNil() {
		b, err := c.mask.CreateSolidColorBrush(d2d.ColorF{R: 1, G: 1, B: 1, A: 1}, d2d.DefaultBrushProperties())
		if err != nil {
			c.fail(err)
			return shadowMask{}, false
		}
		sc.brush = b
	}
	bmp, err := c.maskBitmap(uint32(pw), uint32(ph), dpi)
	if err != nil {
		c.fail(err)
		return shadowMask{}, false
	}
	mc := c.mask
	mc.SetTarget(bmp.Ptr)
	mc.SetDpi(dpi, dpi)
	mc.BeginDraw()
	mc.Clear(d2d.ColorF{})
	mc.SetTransform(xf)
	if fill != nil {
		mc.FillRoundedRectangle(rr, sc.brush.Brush())
	} else {
		mc.FillGeometry(geom, sc.brush.Brush())
	}
	mc.SetTransform(d2d.IdentityMatrix())
	err = mc.EndDraw()
	mc.SetTarget(0)
	if err != nil {
		c.fail(err)
		bmp.Release()
		return shadowMask{}, false
	}
	m := shadowMask{bmp: bmp, pw: uint32(pw), ph: uint32(ph), origin: render.Point{X: bounds.X, Y: bounds.Y}}
	sc.masks[key] = &cachedMask{shadowMask: m, used: sc.paint}
	sc.pixels += int(pw) * int(ph)
	c.masksMade++
	return m, true
}

// maskBitmap makes a premultiplied BGRA target bitmap on the mask context.
func (c *canvas) maskBitmap(pw, ph uint32, dpi float32) (d2d.Bitmap1, error) {
	return c.mask.CreateBitmap(d2d.SizeU{Width: pw, Height: ph}, nil, 0, &d2d.BitmapProperties1{
		PixelFormat:   d2d.PixelFormat{Format: dxgi.FORMAT_B8G8R8A8_UNORM, AlphaMode: d2d.ALPHA_MODE_PREMULTIPLIED},
		DpiX:          dpi,
		DpiY:          dpi,
		BitmapOptions: d2d.BITMAP_OPTIONS_TARGET,
	})
}

// shadow returns the cached baked shadow for key on the mask m, baking it
// if there is none: a Shadow effect on the mask context (an effect belongs
// to the context that made it), drawn once into a bitmap of the mask's
// size and DPI, then released.
func (c *canvas) shadow(key shadowKey, m shadowMask) (d2d.Bitmap1, bool) {
	sc := c.shadows
	if b, ok := sc.baked[key]; ok {
		b.used = sc.paint
		return b.bmp, true
	}
	mc := c.mask
	eff, err := mc.CreateEffect(&d2d.CLSID_D2D1Shadow)
	if err != nil {
		c.fail(err)
		return d2d.Bitmap1{}, false
	}
	defer eff.Release()
	col := key.color
	if err := firstErr(
		eff.SetFloat(d2d.SHADOW_PROP_BLUR_STANDARD_DEVIATION, float32(key.mask.blur/2)),
		eff.SetVector4(d2d.SHADOW_PROP_COLOR, d2d.Vector4F{X: col.R, Y: col.G, Z: col.B, W: col.A}),
	); err != nil {
		c.fail(err)
		return d2d.Bitmap1{}, false
	}
	dpi := key.mask.dpi
	if !(dpi > 0) {
		dpi = 96
	}
	bmp, err := c.maskBitmap(m.pw, m.ph, dpi)
	if err != nil {
		c.fail(err)
		return d2d.Bitmap1{}, false
	}
	eff.SetInput(0, m.bmp.Image())
	out := eff.Output()
	mc.SetTarget(bmp.Ptr)
	mc.SetDpi(dpi, dpi)
	mc.BeginDraw()
	mc.Clear(d2d.ColorF{})
	mc.SetTransform(d2d.IdentityMatrix())
	mc.DrawImage(out, &d2d.Point2F{}, nil, d2d.INTERPOLATION_MODE_LINEAR, d2d.COMPOSITE_MODE_SOURCE_OVER)
	err = mc.EndDraw()
	mc.SetTarget(0)
	out.Release()
	eff.SetInput(0, d2d.Image{}) // the effect lets go of the mask
	if err != nil {
		c.fail(err)
		bmp.Release()
		return d2d.Bitmap1{}, false
	}
	sc.baked[key] = &cachedShadow{bmp: bmp, used: sc.paint}
	sc.pixels += int(m.pw) * int(m.ph)
	c.shadowsBaked++
	return bmp, true
}

// firstErr returns the first non-nil error.
func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
