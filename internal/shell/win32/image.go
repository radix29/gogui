//go:build windows

package win32

import (
	"fmt"
	"math"

	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/win32/d2d"
	"github.com/radix29/gogui/internal/shell/win32/dxgi"
)

// Shell images (S2.4e_4).
//
//   - An image is premultiplied BGRA pixels kept on the CPU, a pixel size
//     and a scale (pixels per point); Size is the pixel size over the
//     scale. There is no public API yet: decoding and icons come later,
//     and the shell and the tests make images with Shell.newImage and
//     imageSet.newImage.
//   - Each image belongs to one imageSet (a shell's, or a test's). Its D2D
//     bitmap is created on first draw for the current device objects (at
//     DPI 96 × scale, so its size in DIPs is Size) and kept until the
//     device goes: releaseGPU releases every bitmap of the set before the
//     devices (device loss and the last surface's Close included), and the
//     next draw makes a new one. image.release frees one early. There are
//     no finalizers: an image the app drops keeps its bitmap until then.
//   - DrawImage draws the bitmap with DrawBitmap scaled into dst (linear
//     interpolation, opacity 1) under the current transform, clip and
//     opacity layers. Images of another set, other render.Images, nil and
//     an empty or invalid dst are ignored. A bitmap that can't be created
//     is a resource failure of the frame and isn't retried on the same
//     device.

// shellImage is the shell's render.Image.
type shellImage struct {
	set   *imageSet
	pix   []byte // premultiplied BGRA, rows w·4 bytes apart
	w, h  int    // pixels
	scale float64

	bmp    d2d.Bitmap1 // nil until drawn on dev
	dev    *gpu        // the device objects bmp (or failed) belongs to
	failed bool        // CreateBitmap failed on dev
}

var _ render.Image = (*shellImage)(nil)

// Size implements render.Image: the pixel size in points.
func (im *shellImage) Size() render.Size {
	return render.Size{W: float64(im.w) / im.scale, H: float64(im.h) / im.scale}
}

// release frees the image's bitmap, if it has one; the next draw makes a
// new one. leak gets a reference left over.
func (im *shellImage) release(leak leakFunc) {
	if im.bmp.IsNil() {
		return
	}
	leak.releaseLast("ID2D1Bitmap1 (image)", &im.bmp.Ptr)
	delete(im.set.live, im)
}

// imageSet is the images of one shell (or test) and the ones among them
// holding a bitmap. It is used on the UI thread only.
type imageSet struct {
	live map[*shellImage]struct{}
	made int // bitmaps created (tests)
}

// newImage returns an image of w×h premultiplied BGRA pixels (pix, rows
// w·4 bytes apart, copied) at scale pixels per point.
func (is *imageSet) newImage(pix []byte, w, h int, scale float64) (*shellImage, error) {
	if w <= 0 || h <= 0 || w > math.MaxInt32/4 || h > math.MaxInt32/4 {
		return nil, fmt.Errorf("win32: image of %d×%d pixels", w, h)
	}
	if !(scale > 0) || math.IsInf(scale, 0) {
		return nil, fmt.Errorf("win32: image scale %v", scale)
	}
	if len(pix) != w*h*4 {
		return nil, fmt.Errorf("win32: %d bytes of pixels for a %d×%d image, want %d", len(pix), w, h, w*h*4)
	}
	return &shellImage{set: is, pix: append([]byte(nil), pix...), w: w, h: h, scale: scale}, nil
}

// bitmap returns im's bitmap on g, creating it on the first call for g.
// ok is false when it can't be made (err is set the first time only).
func (is *imageSet) bitmap(g *gpu, im *shellImage) (bmp d2d.Bitmap1, ok bool, err error) {
	if im.dev != g {
		im.release(nil) // a bitmap of earlier device objects (releaseGPU normally took it)
		im.dev, im.failed = g, false
	}
	if !im.bmp.IsNil() {
		return im.bmp, true, nil
	}
	if im.failed {
		return d2d.Bitmap1{}, false, nil
	}
	dpi := float32(96 * im.scale)
	if !(dpi > 0) || math.IsInf(float64(dpi), 0) {
		dpi = 96
	}
	b, err := g.dc.CreateBitmap(d2d.SizeU{Width: uint32(im.w), Height: uint32(im.h)}, im.pix, uint32(im.w*4),
		&d2d.BitmapProperties1{
			PixelFormat:   d2d.PixelFormat{Format: dxgi.FORMAT_B8G8R8A8_UNORM, AlphaMode: d2d.ALPHA_MODE_PREMULTIPLIED},
			DpiX:          dpi,
			DpiY:          dpi,
			BitmapOptions: d2d.BITMAP_OPTIONS_NONE,
		})
	if err != nil {
		im.failed = true
		return d2d.Bitmap1{}, false, fmt.Errorf("win32: creating a %d×%d image bitmap: %w", im.w, im.h, err)
	}
	im.bmp = b
	if is.live == nil {
		is.live = map[*shellImage]struct{}{}
	}
	is.live[im] = struct{}{}
	is.made++
	return b, true, nil
}

// release frees every image's bitmap (before the device objects go).
func (is *imageSet) release(leak leakFunc) {
	for im := range is.live {
		im.release(leak)
		im.dev = nil
	}
}

// newImage returns an image of the shell (imageSet.newImage).
func (s *Shell) newImage(pix []byte, w, h int, scale float64) (*shellImage, error) {
	return s.images.newImage(pix, w, h, scale)
}

// DrawImage implements render.Canvas.
func (c *canvas) DrawImage(img render.Image, dst render.Rect) {
	if c.rec != nil {
		c.rec.DrawImage(img, dst)
	}
	if c.ended || c.images == nil {
		return
	}
	im, ok := img.(*shellImage)
	if !ok || im == nil || im.set != c.images {
		return
	}
	rf, ok := rectF(dst)
	if !ok || dst.W == 0 || dst.H == 0 {
		return
	}
	bmp, ok, err := c.images.bitmap(c.gpu, im)
	if err != nil {
		c.fail(err)
	}
	if !ok {
		return
	}
	c.drawDC().DrawBitmap(bmp, &rf, 1, d2d.INTERPOLATION_MODE_LINEAR, nil)
}
