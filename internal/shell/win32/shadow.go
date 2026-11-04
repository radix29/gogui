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
//   - The shadow effect gets the mask, σ = Blur/2 (DIPs) and the color; its
//     output is drawn at the shape's origin plus Offset under the current
//     transform, so the shadow follows translations, scales and rotations.
//   - The clip: a geometry layer, the outside of the shape (a rect around
//     the drawn shadow with the shape cut out) for outer shadows, the shape
//     itself for inset ones.
//   - Caches for the frame: masks by shape size, radii, spread, blur, inset
//     and the inset ring's margin; effects by mask and color (an effect's
//     input and properties are only set when it is made, so two shadows in a
//     batch never share a mutated effect). release frees them after EndDraw.
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
}

// shadowMask is a mask bitmap and where its origin lies relative to the
// shape's top-left corner, in user-space points.
type shadowMask struct {
	bmp    d2d.Bitmap1
	origin render.Point
}

// effectKey identifies a cached shadow effect.
type effectKey struct {
	mask  maskKey
	color d2d.ColorF
}

// shadowEffect is a shadow effect on a mask and its output (one reference
// each).
type shadowEffect struct {
	effect d2d.Effect
	out    d2d.Image
}

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
	key := maskKey{w: r.W, h: r.H, radii: radii, spread: s.Spread, blur: s.Blur, inset: s.Inset}
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
	e, ok := c.shadowEffect(effectKey{mask: key, color: color}, m)
	if !ok {
		return
	}
	clip, ok := c.shadowClip(r, radii, s)
	if !ok {
		return
	}
	c.dc.PushLayer(&d2d.LayerParameters1{
		ContentBounds:     d2d.InfiniteRect(),
		GeometricMask:     clip.Ptr,
		MaskAntialiasMode: d2d.ANTIALIAS_MODE_PER_PRIMITIVE,
		MaskTransform:     d2d.IdentityMatrix(),
		Opacity:           1,
	})
	c.dc.DrawImage(e.out, &at, nil, d2d.INTERPOLATION_MODE_LINEAR, d2d.COMPOSITE_MODE_SOURCE_OVER)
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

// shadowMask returns the frame's mask for key, drawing it on the mask
// context if there is none yet.
func (c *canvas) shadowMask(key maskKey) (shadowMask, bool) {
	if m, ok := c.masks[key]; ok {
		c.maskHits++
		return m, true
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
	dpi := c.dpi
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
	if c.maskBrush.IsNil() {
		b, err := c.mask.CreateSolidColorBrush(d2d.ColorF{R: 1, G: 1, B: 1, A: 1}, d2d.DefaultBrushProperties())
		if err != nil {
			c.fail(err)
			return shadowMask{}, false
		}
		c.maskBrush = b
	}
	bmp, err := c.mask.CreateBitmap(d2d.SizeU{Width: uint32(pw), Height: uint32(ph)}, nil, 0, &d2d.BitmapProperties1{
		PixelFormat:   d2d.PixelFormat{Format: dxgi.FORMAT_B8G8R8A8_UNORM, AlphaMode: d2d.ALPHA_MODE_PREMULTIPLIED},
		DpiX:          dpi,
		DpiY:          dpi,
		BitmapOptions: d2d.BITMAP_OPTIONS_TARGET,
	})
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
		mc.FillRoundedRectangle(rr, c.maskBrush.Brush())
	} else {
		mc.FillGeometry(geom, c.maskBrush.Brush())
	}
	mc.SetTransform(d2d.IdentityMatrix())
	err = mc.EndDraw()
	mc.SetTarget(0)
	if err != nil {
		c.fail(err)
		bmp.Release()
		return shadowMask{}, false
	}
	m := shadowMask{bmp: bmp, origin: render.Point{X: bounds.X, Y: bounds.Y}}
	c.masks[key] = m
	return m, true
}

// shadowEffect returns the frame's shadow effect for key on the mask m.
func (c *canvas) shadowEffect(key effectKey, m shadowMask) (shadowEffect, bool) {
	if e, ok := c.effects[key]; ok {
		return e, true
	}
	eff, err := c.dc.CreateEffect(&d2d.CLSID_D2D1Shadow)
	if err != nil {
		c.fail(err)
		return shadowEffect{}, false
	}
	col := key.color
	if err := firstErr(
		eff.SetFloat(d2d.SHADOW_PROP_BLUR_STANDARD_DEVIATION, float32(key.mask.blur/2)),
		eff.SetVector4(d2d.SHADOW_PROP_COLOR, d2d.Vector4F{X: col.R, Y: col.G, Z: col.B, W: col.A}),
	); err != nil {
		c.fail(err)
		eff.Release()
		return shadowEffect{}, false
	}
	eff.SetInput(0, m.bmp.Image())
	e := shadowEffect{effect: eff, out: eff.Output()}
	c.effects[key] = e
	return e, true
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

// releaseShadows releases the frame's effects (before the masks they hold
// as inputs), masks and the mask brush; last releases what must be the last
// reference.
func (c *canvas) releaseShadows(last func(what string, p *com.Ptr)) {
	for k, e := range c.effects {
		e.out.Release() // the effect holds its output too
		last("ID2D1Effect (shadow)", &e.effect.Ptr)
		delete(c.effects, k)
	}
	for k, m := range c.masks {
		last("ID2D1Bitmap1 (shadow mask)", &m.bmp.Ptr)
		delete(c.masks, k)
	}
	if !c.maskBrush.IsNil() {
		last("ID2D1SolidColorBrush (mask)", &c.maskBrush.Ptr)
	}
}
