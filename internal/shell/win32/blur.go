//go:build windows

package win32

import (
	"errors"
	"math"

	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/win32/com"
	"github.com/radix29/gogui/internal/shell/win32/d2d"
	"github.com/radix29/gogui/internal/shell/win32/dxgi"
)

// Backdrop blur (S2.4e_3).
//
//   - The region: the device-pixel bounds of the shape under the current
//     transform, padded by 3σ and intersected with the target.
//   - The copy: Flush, then CopyFromBitmap from the target into the
//     target's scratch bitmap (backdrop), at its origin. Inside an open
//     D2D layer the copy reads the layer's own content, not the composed
//     target (finding of S2.4e_3), so the open layers are popped first and
//     pushed again after the copy, each under the transform it was pushed
//     with. Content of an opacity group drawn before the blur is thereby
//     composed as a group of its own.
//   - The blur: a crop effect limits the scratch bitmap to the copied
//     region, a Gaussian blur with hard borders blurs it, σ = BlurRadius/2
//     in user space (scaled by the transform's √|det| into DIPs).
//   - The draw: a geometry layer of the shape under the current transform
//     is pushed (it stays open until Pop, so content drawn on top is
//     clipped to the shape too), and the blurred copy is drawn into it under
//     the identity transform at the region's pixel origin.
//   - The effects are made once per frame and changed only right after a
//     Flush, so no batched draw sees a later change. release frees them.
//   - A zero radius pushes only the clip; an empty shape clips everything
//     away; values that don't narrow push an empty clip too.

// backdrop is a target's scratch bitmap for the blur copies, kept across
// frames: grown when a copy needs more, recreated for another DPI and
// released with the target (or on device loss).
type backdrop struct {
	bmp  d2d.Bitmap1
	w, h uint32
	dpi  float32
	made int // bitmaps created (tests)
}

// release releases the scratch bitmap; leak gets a reference left over.
func (b *backdrop) release(leak leakFunc) {
	leak.releaseLast("ID2D1Bitmap1 (backdrop)", &b.bmp.Ptr)
	b.w, b.h, b.dpi = 0, 0, 0
}

// blurTarget is what the canvas copies the backdrop from: the frame's
// target bitmap (TARGET | CANNOT_DRAW), its pixel size, and the target's
// scratch bitmap. Without one (a zero value) blurs only clip.
type blurTarget struct {
	bmp     d2d.Bitmap1 // no reference of its own
	w, h    uint32
	scratch *backdrop
}

// blurEffects are the frame's crop and blur effects (one reference each,
// and one on the blur's output).
type blurEffects struct {
	crop, blur d2d.Effect
	out        d2d.Image
}

// PushBackdropBlur implements render.Canvas.
func (c *canvas) PushBackdropBlur(r render.Rect, radii render.Radii, radius float64) {
	if c.rec != nil {
		c.rec.PushBackdropBlur(r, radii, radius)
	}
	if c.ended {
		return
	}
	c.push(canvasBlur)
	rf, ok := rectF(r)
	if !ok || r.W == 0 || r.H == 0 {
		// An empty or invalid shape clips everything away.
		c.addClip(d2d.RectF{}, d2d.ANTIALIAS_MODE_ALIASED)
		return
	}
	radii = radii.Clamped(r)
	if sigma := c.blurSigma(radius); sigma > 0 {
		c.blurBackdrop(r, sigma)
	}
	if radii.IsZero() && axisPreserving(c.xf) {
		c.addClip(rf, d2d.ANTIALIAS_MODE_PER_PRIMITIVE)
	} else if g, ok := c.roundedGeometry(r, rf, radii); ok {
		c.addLayer(d2d.LayerParameters1{
			ContentBounds:     d2d.InfiniteRect(),
			GeometricMask:     g.Ptr,
			MaskAntialiasMode: d2d.ANTIALIAS_MODE_PER_PRIMITIVE,
			MaskTransform:     d2d.IdentityMatrix(),
			Opacity:           1,
		})
	} else {
		c.addClip(rf, d2d.ANTIALIAS_MODE_PER_PRIMITIVE)
	}
	c.drawBlur()
}

// blurSigma returns the blur's σ in DIPs: radius/2 scaled by the current
// transform; 0 for no blur.
func (c *canvas) blurSigma(radius float64) float64 {
	s := render.BlurRadius(radius) / 2 * math.Sqrt(math.Abs(c.xf.Det()))
	if !(s > 0) || math.IsInf(s, 0) {
		return 0
	}
	return s
}

// errNoBlur marks a copy that wasn't made (the failure is recorded).
var errNoBlur = errors.New("win32: no backdrop copy")

// pendingBlur is a blurred copy waiting to be drawn into the blur's clip.
type pendingBlur struct {
	at d2d.Point2F // the region's origin in DIPs
	ok bool
}

// blurBackdrop copies the region of r (σ DIPs of blur) into the scratch
// bitmap and sets up the effects; drawBlur draws the result.
func (c *canvas) blurBackdrop(r render.Rect, sigma float64) {
	c.blur = pendingBlur{}
	if c.target.bmp.IsNil() || c.target.scratch == nil {
		return
	}
	dpi := c.dpi
	if !(dpi > 0) {
		dpi = 96
	}
	px := float64(dpi) / 96
	dev := c.xf.ApplyRect(r)
	pad := 3 * sigma * px
	x0 := math.Max(math.Floor(dev.X*px-pad), 0)
	y0 := math.Max(math.Floor(dev.Y*px-pad), 0)
	x1 := math.Min(math.Ceil(dev.MaxX()*px+pad), float64(c.target.w))
	y1 := math.Min(math.Ceil(dev.MaxY()*px+pad), float64(c.target.h))
	if !(x1 > x0 && y1 > y0) {
		return // nothing of the target behind the shape
	}
	w, h := uint32(x1-x0), uint32(y1-y0)

	// Pop the open layers (and the clips above them), so the target holds
	// the composed frame, flush, copy, and push them again. Every change to
	// the scratch bitmap and the effects comes after the Flush, so no
	// batched draw (an earlier blur's) sees it.
	low := len(c.pushes)
	for i := len(c.pushes) - 1; i >= 0; i-- {
		if c.pushes[i].kind == pushLayer {
			low = i
		}
	}
	reopen := append([]canvasPush(nil), c.pushes[low:]...)
	c.popTo(low)
	if err := c.dc.Flush(); err != nil {
		c.fail(err)
	}
	bmp, okB := c.scratchBitmap(w, h, dpi)
	e, okE := c.blurEffects()
	copyErr := errNoBlur
	if okB && okE {
		src := d2d.RectU{Left: uint32(x0), Top: uint32(y0), Right: uint32(x1), Bottom: uint32(y1)}
		if copyErr = bmp.CopyFromBitmap(nil, c.target.bmp, &src); copyErr != nil {
			c.fail(copyErr)
		}
	}
	for _, p := range reopen {
		c.sendXf(p.xf)
		switch p.kind {
		case pushClip:
			c.dc.PushAxisAlignedClip(p.rect, p.aa)
		case pushLayer:
			c.dc.PushLayer(&p.layer)
		}
		c.pushes = append(c.pushes, p)
	}
	if copyErr != nil {
		return
	}

	e.crop.SetInput(0, bmp.Image())
	dip := func(v float64) float32 { return float32(v / px) }
	if err := firstErr(
		e.crop.SetVector4(d2d.CROP_PROP_RECT, d2d.Vector4F{Z: dip(float64(w)), W: dip(float64(h))}),
		e.blur.SetFloat(d2d.GAUSSIANBLUR_PROP_STANDARD_DEVIATION, float32(sigma)),
	); err != nil {
		c.fail(err)
		return
	}
	c.blur = pendingBlur{at: d2d.Point2F{X: dip(x0), Y: dip(y0)}, ok: true}
}

// drawBlur draws the pending blurred copy under the identity transform.
func (c *canvas) drawBlur() {
	if !c.blur.ok {
		return
	}
	c.blur.ok = false
	at := c.blur.at
	c.sendXf(d2d.IdentityMatrix())
	c.dc.DrawImage(c.blurFx.out, &at, nil, d2d.INTERPOLATION_MODE_NEAREST_NEIGHBOR, d2d.COMPOSITE_MODE_SOURCE_OVER)
	c.blurs++
}

// scratchBitmap returns the target's scratch bitmap with room for w×h
// pixels at dpi, growing or recreating it if needed.
func (c *canvas) scratchBitmap(w, h uint32, dpi float32) (d2d.Bitmap1, bool) {
	b := c.target.scratch
	if !b.bmp.IsNil() && b.dpi == dpi && b.w >= w && b.h >= h {
		return b.bmp, true
	}
	nw, nh := w, h
	if b.dpi == dpi {
		nw, nh = max(nw, b.w), max(nh, b.h)
	}
	if !b.bmp.IsNil() {
		// Flushed draws may still hold it: released with the frame.
		if !c.blurFx.crop.IsNil() {
			c.blurFx.crop.SetInput(0, d2d.Image{})
		}
		c.geoms = append(c.geoms, owned{"ID2D1Bitmap1 (old backdrop)", b.bmp.Ptr})
		b.bmp = d2d.Bitmap1{}
	}
	bmp, err := c.dc.CreateBitmap(d2d.SizeU{Width: nw, Height: nh}, nil, 0, &d2d.BitmapProperties1{
		PixelFormat: d2d.PixelFormat{Format: dxgi.FORMAT_B8G8R8A8_UNORM, AlphaMode: d2d.ALPHA_MODE_PREMULTIPLIED},
		DpiX:        dpi,
		DpiY:        dpi,
	})
	if err != nil {
		c.fail(err)
		return d2d.Bitmap1{}, false
	}
	b.bmp, b.w, b.h, b.dpi = bmp, nw, nh, dpi
	b.made++
	return bmp, true
}

// blurEffects returns the frame's crop and blur effects, making them on
// first use.
func (c *canvas) blurEffects() (blurEffects, bool) {
	if !c.blurFx.blur.IsNil() {
		return c.blurFx, true
	}
	crop, err := c.dc.CreateEffect(&d2d.CLSID_D2D1Crop)
	if err != nil {
		c.fail(err)
		return blurEffects{}, false
	}
	blur, err := c.dc.CreateEffect(&d2d.CLSID_D2D1GaussianBlur)
	if err != nil {
		c.fail(err)
		crop.Release()
		return blurEffects{}, false
	}
	if err := firstErr(
		crop.SetEnum(d2d.CROP_PROP_BORDER_MODE, d2d.BORDER_MODE_HARD),
		blur.SetEnum(d2d.GAUSSIANBLUR_PROP_BORDER_MODE, d2d.BORDER_MODE_HARD),
	); err != nil {
		c.fail(err)
		crop.Release()
		blur.Release()
		return blurEffects{}, false
	}
	cropOut := crop.Output()
	blur.SetInput(0, cropOut)
	cropOut.Release() // the blur holds it
	c.blurFx = blurEffects{crop: crop, blur: blur, out: blur.Output()}
	return c.blurFx, true
}

// releaseBlur releases the frame's blur effects (the blur first: it holds
// the crop's output). The crop lets go of the scratch bitmap.
func (c *canvas) releaseBlur(last func(what string, p *com.Ptr)) {
	e := &c.blurFx
	if e.blur.IsNil() {
		return
	}
	e.out.Release()
	last("ID2D1Effect (blur)", &e.blur.Ptr)
	last("ID2D1Effect (crop)", &e.crop.Ptr)
}
