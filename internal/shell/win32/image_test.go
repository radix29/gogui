//go:build windows

package win32

import (
	"math"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/win32/com"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// The image tests (S2.4e_4) draw shell images on the canvas test target
// at DPI 96 and 192, and on a real surface for the device-loss and
// lifetime checks.

// pixels returns w×h premultiplied BGRA pixels from at(x, y).
func pixels(w, h int, at func(x, y int) [4]byte) []byte {
	pix := make([]byte, 0, w*h*4)
	for y := range h {
		for x := range w {
			p := at(x, y)
			pix = append(pix, p[:]...)
		}
	}
	return pix
}

// checker returns the 2×2 image red, green / blue, black at scale 1.
func checker(t *testing.T, is *imageSet) *shellImage {
	t.Helper()
	q := [4][4]byte{cRed, cGreen, cBlue, cBlack}
	im, err := is.newImage(pixels(2, 2, func(x, y int) [4]byte { return q[2*y+x] }), 2, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	return im
}

// solidImage returns a w×h image of one color at scale.
func solidImage(t *testing.T, is *imageSet, w, h int, scale float64, c [4]byte) *shellImage {
	t.Helper()
	im, err := is.newImage(pixels(w, h, func(int, int) [4]byte { return c }), w, h, scale)
	if err != nil {
		t.Fatal(err)
	}
	return im
}

func TestNewImage(t *testing.T) {
	var is imageSet
	for _, c := range []struct {
		pix   int
		w, h  int
		scale float64
	}{
		{0, 0, 1, 1}, {4, 1, 0, 1}, {4, -1, -1, 1}, {4, 1, 1, 0}, {4, 1, 1, math.NaN()}, {4, 1, 1, math.Inf(1)},
		{3, 1, 1, 1}, {12, 2, 2, 1}, {20, 2, 2, 1},
	} {
		if _, err := is.newImage(make([]byte, c.pix), c.w, c.h, c.scale); err == nil {
			t.Errorf("newImage(%d bytes, %d×%d, scale %v) accepted", c.pix, c.w, c.h, c.scale)
		}
	}
	src := make([]byte, 16)
	im, err := is.newImage(src, 2, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	src[0] = 1
	if im.pix[0] != 0 {
		t.Error("newImage kept the caller's slice")
	}
	if sz := im.Size(); sz != (render.Size{W: 1, H: 1}) {
		t.Errorf("Size of 2×2 px at scale 2 = %+v, want 1×1 pt", sz)
	}
	if im, _ := is.newImage(make([]byte, 6*4*4), 6, 4, 1.5); im.Size() != (render.Size{W: 4, H: 4.0 / 1.5}) {
		t.Errorf("Size at scale 1.5 = %+v", im.Size())
	}
}

// TestCanvasImage: a 2×2 checker scaled to dst (each quadrant's color at
// its centre, linear mixing between them, nothing outside dst), under a
// transform, a clip, inside PushOpacity and scaled non-uniformly; the
// bitmap is made once and reused over frames.
func TestCanvasImage(t *testing.T) {
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		im := checker(t, &e.images)
		made := e.images.made
		const tol = 2 // sampled between dst's edge and the texel centres, where the edge texel is clamped
		// quadrants checks the checker drawn at (8, 8, 32, 32).
		quadrants := func(s *shot, what string, right bool) {
			t.Helper()
			s.is(what+": red quadrant", cRed, tol, 12, 12, 9, 9)
			s.is(what+": blue quadrant", cBlue, tol, 12, 36, 9, 39)
			if right {
				s.is(what+": green quadrant", cGreen, tol, 36, 12, 39, 9)
				s.is(what+": black quadrant", cBlack, tol, 36, 36, 39, 39)
			}
		}
		cases := []struct {
			what  string
			paint func(c *canvas)
			check func(s *shot)
		}{
			{"plain", func(c *canvas) { c.DrawImage(im, rect(8, 8, 32, 32)) }, func(s *shot) {
				quadrants(s, "plain", true)
				// At the quadrant centres (the texel centres) the colors are
				// nearly pure; the device pixel centre is off by half a pixel.
				s.is("red centre", cRed, 20, 16, 16)
				s.is("green centre", cGreen, 20, 32, 16)
				s.is("blue centre", cBlue, 20, 16, 32)
				s.is("black centre", cBlack, 20, 32, 32)
				mid := s.px(24, 16) // between red and green
				if mid[1] < 90 || mid[1] > 165 || mid[2] < 90 || mid[2] > 165 {
					t.Errorf("scale %v: between red and green BGRA %v, want a linear mix", scale, mid)
				}
				s.is("outside dst", cWhite, 1, 6, 20, 41, 20, 20, 6, 20, 41)
			}},
			{"transform", func(c *canvas) {
				c.Translate(8, 8)
				c.Scale(0.5, 0.5)
				c.DrawImage(im, rect(0, 0, 64, 64))
			}, func(s *shot) {
				quadrants(s, "transform", true)
				s.is("outside the transformed dst", cWhite, 1, 41, 20, 20, 41)
			}},
			{"clip", func(c *canvas) {
				c.ClipRoundedRect(rect(8, 8, 16, 32), render.Radii{}) // the left half only
				c.DrawImage(im, rect(8, 8, 32, 32))
			}, func(s *shot) {
				quadrants(s, "clip", false)
				s.is("clipped away", cWhite, 1, 32, 16, 32, 32)
			}},
			{"opacity", func(c *canvas) {
				c.PushOpacity(0.5)
				c.DrawImage(im, rect(8, 8, 32, 32))
				c.Pop()
			}, func(s *shot) {
				s.is("half-opacity red", [4]byte{127, 127, 255, 255}, 3, 12, 12)
				s.is("half-opacity black", [4]byte{127, 127, 127, 255}, 3, 36, 36)
			}},
			{"non-uniform", func(c *canvas) { c.DrawImage(im, rect(8, 8, 48, 16)) }, func(s *shot) {
				s.is("wide red", cRed, tol, 12, 10)
				s.is("wide black", cBlack, tol, 52, 22)
				s.is("below the wide one", cWhite, 1, 30, 25)
			}},
		}
		for _, cs := range cases {
			s, c, err := e.frame(scale, true, cs.paint)
			if err != nil || c.failures() != nil {
				t.Errorf("scale %v: %s: recorder %v, failures %v", scale, cs.what, err, c.failures())
			}
			cs.check(s)
		}
		if e.images.made != made+1 {
			t.Errorf("scale %v: %d bitmaps made over five frames, want 1", scale, e.images.made-made)
		}
	})
}

// TestCanvasImageScale: a scale-2 image keeps its size in points and is
// drawn sharp at DPI 192 (one bitmap pixel per device pixel).
func TestCanvasImageScale(t *testing.T) {
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		// 8×8 px at scale 2: 4×4 pt, a one-pixel stripe pattern by columns.
		im, err := e.images.newImage(pixels(8, 8, func(x, _ int) [4]byte {
			if x%2 == 0 {
				return cBlack
			}
			return cWhite
		}), 8, 8, 2)
		if err != nil {
			t.Fatal(err)
		}
		sz := im.Size()
		if sz != (render.Size{W: 4, H: 4}) {
			t.Fatalf("Size = %+v, want 4×4 pt", sz)
		}
		s, _, _ := e.frame(scale, false, func(c *canvas) {
			c.DrawImage(im, render.Rect{X: 16, Y: 16, W: sz.W, H: sz.H})
		})
		if x, y := im.bmp.Dpi(); x != 192 || y != 192 {
			t.Errorf("the bitmap's DPI is %v×%v, want 192 (scale 2)", x, y)
		}
		s.is("left of the image", cWhite, 1, 15.5, 18)
		s.is("right of the image", cWhite, 1, 20.25, 18)
		s.is("below the image", cWhite, 1, 18, 20.25)
		if scale == 2 {
			s.is("dark columns", cBlack, 2, 16, 18, 17, 18, 18, 18, 19, 18)
			s.is("light columns", cWhite, 2, 16.5, 18, 17.5, 18, 18.5, 18, 19.5, 18)
		} else {
			s.is("columns averaged", [4]byte{127, 127, 127, 255}, 4, 16, 18, 17, 18, 18, 18)
		}
	})
}

// fakeImage is a render.Image the shell didn't make.
type fakeImage struct{}

func (fakeImage) Size() render.Size { return render.Size{W: 8, H: 8} }

// TestCanvasImageIgnored: images of another set, other render.Images, nil
// and empty or invalid destinations draw nothing, make no bitmap and don't
// fail EndDraw; a canvas without an image set draws none.
func TestCanvasImageIgnored(t *testing.T) {
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		var other imageSet
		foreign := solidImage(t, &other, 2, 2, 1, cRed)
		own := solidImage(t, &e.images, 2, 2, 1, cRed)
		var nilImage *shellImage
		made := e.images.made
		nan := math.NaN()
		s, c, _ := e.frame(scale, false, func(c *canvas) {
			c.DrawImage(foreign, rect(8, 8, 16, 16))
			c.DrawImage(fakeImage{}, rect(8, 8, 16, 16))
			c.DrawImage(nil, rect(8, 8, 16, 16))
			c.DrawImage(nilImage, rect(8, 8, 16, 16))
			for _, r := range []render.Rect{rect(8, 8, 0, 16), rect(8, 8, 16, 0), rect(8, 8, -16, 16),
				rect(nan, 8, 16, 16), rect(8, 8, math.Inf(1), 16), rect(1e300, 8, 16, 16)} {
				c.DrawImage(own, r)
			}
			c.FillRoundedRect(rect(40, 8, 8, 8), render.Radii{}, green) // the canvas still draws
		})
		if c.failures() != nil {
			t.Error(c.failures())
		}
		if e.images.made != made || len(other.live) != 0 || !foreign.bmp.IsNil() || !own.bmp.IsNil() {
			t.Errorf("scale %v: bitmaps made for ignored draws (%d, foreign %d)", scale, e.images.made-made, len(other.live))
		}
		s.is("nothing drawn", cWhite, 1, 9, 9, 16, 16, 23, 23)
		s.is("after the ignored draws", cGreen, 1, 44, 12)

		images := e.images
		e.images = imageSet{}
		s, _, _ = e.frame(scale, false, func(c *canvas) {
			c.images = nil
			c.DrawImage(own, rect(8, 8, 16, 16))
		})
		e.images = images
		s.is("no image set", cWhite, 1, 16, 16)
	})
}

// TestCanvasImageRecorded: with the recorder a drawn image passes its
// checks; a nil image is reported by it and still not drawn.
func TestCanvasImageRecorded(t *testing.T) {
	e := newCanvasEnv(t)
	im := checker(t, &e.images)
	if _, _, err := e.frame(1, true, func(c *canvas) { c.DrawImage(im, rect(8, 8, 16, 16)) }); err != nil {
		t.Errorf("a valid DrawImage: %v", err)
	}
	if _, _, err := e.frame(1, true, func(c *canvas) { c.DrawImage(nil, rect(8, 8, 16, 16)) }); err == nil {
		t.Error("the recorder didn't report a nil image")
	}
}

// TestSurfaceImage: on a real surface the image's bitmap is made once,
// kept over frames, released by a device loss and recreated by the next
// draw, and released with the last surface, with no references left.
func TestSurfaceImage(t *testing.T) {
	inShell(t, func(s *Shell) {
		im, err := s.newImage(pixels(2, 2, func(x, y int) [4]byte {
			if x == y {
				return cRed
			}
			return cBlue
		}), 2, 2, 1)
		if err != nil {
			t.Fatal(err)
		}
		r := &recorder{}
		r.onPaint = func(cv render.Canvas) {
			cv.FillRoundedRect(rect(0, 0, 200, 200), render.Radii{}, render.Solid(driver.RGB8(255, 255, 255)))
			cv.DrawImage(im, rect(40, 40, 80, 80))
		}
		sf := newSurface(t, s, resizable, r)
		check := func(what string) {
			t.Helper()
			w, h, pix := framePx(t, sf)
			px := float64(sf.dpi) / 96
			for _, c := range []struct {
				x, y float64
				want [4]byte
			}{{50, 50, cRed}, {110, 50, cBlue}, {50, 110, cBlue}, {110, 110, cRed}, {30, 80, cWhite}, {130, 80, cWhite}} {
				if p := framePxAt(w, h, pix, px, c.x, c.y); !nearPx(p, c.want, 2) {
					t.Errorf("%s: at (%v, %v): BGRA %v, want %v", what, c.x, c.y, p, c.want)
				}
			}
		}
		check("first frame")
		check("second frame")
		if im.bmp.IsNil() || s.images.made != 1 || im.dev != s.gpu {
			t.Fatalf("after two frames: bitmap %v, %d made", !im.bmp.IsNil(), s.images.made)
		}
		if n := im.bmp.RefCount(); n != 1 {
			t.Errorf("the image bitmap has %d references between frames, want 1", n)
		}

		s.fault = fault{op: "EndDraw", hr: com.D2DERR_RECREATE_TARGET}
		w32.SendMessage(sf.hwnd, w32.WM_PAINT, 0, 0)
		rethrow()
		if s.losses != 1 || len(s.leaks) != 0 {
			t.Fatalf("device loss: %d losses, leaks %v", s.losses, s.leaks)
		}
		if !im.bmp.IsNil() || len(s.images.live) != 0 {
			t.Error("the image bitmap survived the device loss")
		}
		check("after the device loss")
		if im.bmp.IsNil() || s.images.made != 2 || im.dev != s.gpu {
			t.Errorf("after the repaint: bitmap %v, %d made; want a new one on the new devices", !im.bmp.IsNil(), s.images.made)
		}

		sf.Close()
		if s.gpu != nil || !im.bmp.IsNil() || len(s.images.live) != 0 || len(s.leaks) != 0 {
			t.Errorf("after the last Close: devices %v, bitmap %v, %d live, leaks %v",
				s.gpu != nil, !im.bmp.IsNil(), len(s.images.live), s.leaks)
		}
		// A new surface draws it again on new devices.
		sf = newSurface(t, s, resizable, r)
		defer sf.Close()
		check("on a new surface")
		if s.images.made != 3 {
			t.Errorf("on a new surface: %d made, want 3", s.images.made)
		}
	})
}
