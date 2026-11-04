package gtk4

import (
	"fmt"
	"math"

	"github.com/radix29/gogui/internal/render"
)

// Shell images (S4.2e, as Win32's S2.4e_4).
//
//   - An image is straight-alpha RGBA pixels kept on the CPU (copied at
//     creation), a pixel size and a scale (pixels per point); Size is the
//     pixel size over the scale. There is no public API yet: decoding and
//     icons come later, and the shell and the tests make images with
//     Shell.newImage and imageSet.newImage.
//   - Each image belongs to one imageSet (the shell's, or a test's). Its
//     GdkTexture (g_bytes_new + gdk_memory_texture_new,
//     GDK_MEMORY_R8G8B8A8) is made on first draw through the snapshotter
//     and kept across frames; GSK uploads and caches it per renderer. The
//     set releases every texture when the shell's last surface closes and
//     when Run ends, and the next draw makes a new one. There are no
//     finalizers: an image the app drops keeps its texture until then.
//   - DrawImage is gtk_snapshot_append_scaled_texture into dst with the
//     linear filter, under the current transform, clips and opacity. GTK
//     takes its own reference to the texture, so the node outlives a
//     release. Images of another set, other render.Images, nil (typed
//     too), an empty or invalid dst and a canvas without a set draw
//     nothing. A texture that can't be made draws nothing and isn't
//     retried until the set is released.

// texture is a GdkTexture the canvas made (gdkTexture, canvas_linux.go;
// a fake in the host tests). release drops the shell's reference.
type texture interface{ release() }

// shellImage is the shell's render.Image.
type shellImage struct {
	set   *imageSet
	pix   []byte // straight-alpha RGBA, rows w·4 bytes apart
	w, h  int    // pixels
	scale float64

	tex    texture // nil until drawn (or after a release)
	failed bool    // making tex failed; reset by a release
}

var _ render.Image = (*shellImage)(nil)

// Size implements render.Image: the pixel size in points (zero for a nil
// image, which the recorder's dump may ask).
func (im *shellImage) Size() render.Size {
	if im == nil {
		return render.Size{}
	}
	return render.Size{W: float64(im.w) / im.scale, H: float64(im.h) / im.scale}
}

// imageSet is the images of one shell (or test) that hold a texture or
// failed to make one. It is used on the UI thread only.
type imageSet struct {
	held map[*shellImage]struct{}
	made int // textures made (tests)
}

// maxImageSide bounds an image's width and height, so the stride (w·4)
// and the GDK sizes fit an int32.
const maxImageSide = math.MaxInt32 / 4

// newImage returns an image of w×h straight-alpha RGBA pixels (pix, rows
// w·4 bytes apart, copied) at scale pixels per point.
func (is *imageSet) newImage(pix []byte, w, h int, scale float64) (*shellImage, error) {
	if w <= 0 || h <= 0 || w > maxImageSide || h > maxImageSide {
		return nil, fmt.Errorf("gtk4: image of %d×%d pixels", w, h)
	}
	if !(scale > 0) || math.IsInf(scale, 0) {
		return nil, fmt.Errorf("gtk4: image scale %v", scale)
	}
	if len(pix) != w*h*4 {
		return nil, fmt.Errorf("gtk4: %d bytes of pixels for a %d×%d image, want %d", len(pix), w, h, w*h*4)
	}
	return &shellImage{set: is, pix: append([]byte(nil), pix...), w: w, h: h, scale: scale}, nil
}

// texture returns im's texture, making it through sn on the first call
// after a release. ok is false when it can't be made.
func (is *imageSet) texture(sn snapshotter, im *shellImage) (texture, bool) {
	if im.tex != nil {
		return im.tex, true
	}
	if im.failed {
		return nil, false
	}
	if is.held == nil {
		is.held = map[*shellImage]struct{}{}
	}
	is.held[im] = struct{}{}
	t, ok := sn.newTexture(im.pix, im.w, im.h)
	if !ok || t == nil {
		im.failed = true
		return nil, false
	}
	im.tex = t
	is.made++
	return t, true
}

// release drops every image's texture (and failure): the shell's last
// surface closed, or Run ended.
func (is *imageSet) release() {
	for im := range is.held {
		if im.tex != nil {
			im.tex.release()
		}
		im.tex, im.failed = nil, false
	}
	clear(is.held)
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
	if !ok || rf.Size.Width == 0 || rf.Size.Height == 0 {
		return
	}
	if t, ok := c.images.texture(c.sn, im); ok {
		c.sn.appendTexture(t, rf)
	}
}
