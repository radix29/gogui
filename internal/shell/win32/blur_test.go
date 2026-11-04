//go:build windows

package win32

import (
	"math"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/win32/com"
	"github.com/radix29/gogui/internal/shell/win32/d2d"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// The S2.4e_3 tests: backdrop blur on the canvas tests' offscreen target
// (canvas_test.go) at DPI 96 and 192, and on a real surface through the
// S2.4d_3 present readback. The backdrop is black and white stripes 4 pt
// wide (period 8 pt); a blur radius of 8 (σ 4 pt) mixes them to an even
// gray (about 127), a sharp region keeps them exact.

// stripes fills x 0…w (y 0…h) with black stripes at 8k…8k+4 over the
// white background.
func stripes(c *canvas, w, h float64) {
	for x := 0.0; x < w; x += 8 {
		c.FillRoundedRect(rect(x, 0, 4, h), render.Radii{}, black)
	}
}

// stripe is the backdrop's pixel at x.
func stripe(x float64) [4]byte {
	if math.Mod(x, 8) < 4 {
		return cBlack
	}
	return cWhite
}

// isStripes checks that the points are the exact backdrop.
func (s *shot) isStripes(what string, xy ...float64) {
	s.t.Helper()
	for i := 0; i+1 < len(xy); i += 2 {
		s.is(what, stripe(xy[i]), 1, xy[i], xy[i+1])
	}
}

// isGray checks that the points are gray v ± tol (B = G = R ± 2).
func (s *shot) isGray(what string, v, tol int, xy ...float64) {
	s.t.Helper()
	for i := 0; i+1 < len(xy); i += 2 {
		p := s.px(xy[i], xy[i+1])
		d := int(p[1]) - v
		if d < -tol || d > tol || absInt(int(p[0])-int(p[1])) > 2 || absInt(int(p[2])-int(p[1])) > 2 || p[3] != 255 {
			s.t.Errorf("scale %v: %s: pixel at (%v, %v) pt = BGRA %v, want gray %d ± %d", s.scale, what, xy[i], xy[i+1], p, v, tol)
		}
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// TestCanvasBlur: the stripes are blurred inside the shape and exact
// outside it (also in its rounded corners); content drawn after the push
// is clipped to the shape, content after the Pop isn't.
func TestCanvasBlur(t *testing.T) {
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		s, c, err := e.frame(scale, true, func(c *canvas) {
			stripes(c, 64, 64)
			c.PushBackdropBlur(rect(16, 8, 32, 24), render.UniformRadii(6), 8)
			if len(c.frames) != 1 || len(c.pushes) != 1 || c.pushes[0].kind != pushLayer {
				t.Errorf("a rounded blur pushed %d frames and %v, want one frame and a layer", len(c.frames), c.pushes)
			}
			c.FillRoundedRect(rect(0, 26, 64, 2), render.Radii{}, red) // clipped to 16…48
			c.Pop()
			c.FillRoundedRect(rect(0, 34, 64, 2), render.Radii{}, red) // unclipped
		})
		if err != nil {
			t.Error(err)
		}
		if c.blurs != 1 {
			t.Errorf("%d blurred copies drawn, want 1", c.blurs)
		}
		var mid []float64
		for x := 18.0; x < 46; x += 0.5 {
			mid = append(mid, x, 16)
		}
		s.isGray("inside the shape", 127, 6, mid...)
		s.isGray("inside, near the edges", 127, 12, 16.5, 20, 47.5, 20, 32, 8.5, 32, 24.5)
		s.isStripes("outside the shape", 1, 4, 5, 4, 14, 16, 15.5, 16, 49, 16, 53, 16, 18, 7.5, 22, 7.5, 18, 32.5, 22, 32.5)
		s.isStripes("the rounded corners", 16.5, 8.5, 47.5, 31.5)
		s.is("drawn after the push, inside", cRed, 1, 17, 27, 45, 27)
		s.isStripes("drawn after the push, outside", 2, 27, 13, 27, 53, 27)
		s.is("drawn after the Pop", cRed, 1, 2, 35, 32, 35, 62, 35)
	})
}

// TestCanvasBlurEdges: a blur at the target's edges reads hard borders: the
// blurred copy is opaque up to the edges of the target and of the copied
// region. Drawn a second time over green, no green shows through.
func TestCanvasBlurEdges(t *testing.T) {
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		s, _, _ := e.frame(scale, false, func(c *canvas) {
			c.FillRoundedRect(rect(0, 0, 64, 64), render.Radii{}, blue)
			c.PushBackdropBlur(rect(0, 0, 20, 20), render.Radii{}, 8) // copied 0…32 (3σ = 12)
			c.Pop()
			c.FillRoundedRect(rect(30, 30, 34, 34), render.Radii{}, green)
			c.dc.DrawImage(c.blurFx.out, &d2d.Point2F{X: 31, Y: 31}, nil, d2d.INTERPOLATION_MODE_NEAREST_NEIGHBOR, d2d.COMPOSITE_MODE_SOURCE_OVER)
		})
		s.is("the target's corner", cBlue, 1, 0.5, 0.5, 19.5, 0.5, 0.5, 19.5, 10, 10)
		s.is("the copy, its edges", cBlue, 1, 31.25, 31.25, 31.25, 50, 50, 31.25, 62.75, 40, 40, 62.75, 47, 47)
		s.is("beside the copy", cGreen, 1, 30.5, 40)
	})
}

// TestCanvasBlurZero: a zero radius only clips, an empty or invalid shape
// clips everything away.
func TestCanvasBlurZero(t *testing.T) {
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		s, c, _ := e.frame(scale, false, func(c *canvas) {
			stripes(c, 64, 64)
			c.PushBackdropBlur(rect(16, 8, 32, 8), render.Radii{}, 0)
			if len(c.pushes) != 1 || c.pushes[0].kind != pushClip {
				t.Errorf("a sharp blur pushed %v, want an axis-aligned clip", c.pushes)
			}
			c.Pop()
			c.PushBackdropBlur(rect(16, 24, 32, 8), render.Radii{}, math.NaN())
			c.FillRoundedRect(rect(0, 28, 64, 4), render.Radii{}, red)
			c.Pop()
			for _, r := range []render.Rect{rect(0, 40, 0, 8), rect(0, 40, -8, 8), rect(math.Inf(1), 40, 8, 8)} {
				c.PushBackdropBlur(r, render.Radii{}, 8)
				c.FillRoundedRect(rect(0, 40, 64, 8), render.Radii{}, red)
				c.Pop()
			}
		})
		if c.blurs != 0 {
			t.Errorf("%d blurred copies, want none", c.blurs)
		}
		s.isStripes("zero radius", 18, 12, 22, 12, 46, 12, 18, 26, 22, 26)
		s.is("zero radius clips", cRed, 1, 17, 30, 47, 30)
		s.isStripes("outside the clip", 2, 30, 14, 30, 53, 30)
		s.isStripes("empty shapes", 2, 44, 6, 44, 30, 44, 34, 44)
	})
}

// TestCanvasBlurOpacity: blur inside an opacity group (the backdrop holds
// what the group drew before it) and opacity inside a blur.
func TestCanvasBlurOpacity(t *testing.T) {
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		s, _, err := e.frame(scale, true, func(c *canvas) {
			stripes(c, 64, 64)
			// Blur in a half-opacity group: inside, half the blurred gray
			// over the stripes: 64 over black, 191 over white.
			c.PushOpacity(0.5)
			c.PushBackdropBlur(rect(8, 4, 48, 12), render.Radii{}, 8)
			c.Pop()
			c.Pop()
			// Half-opacity red inside a blur: the gray and red mixed, clipped.
			c.PushBackdropBlur(rect(8, 20, 48, 12), render.UniformRadii(2), 8)
			c.PushOpacity(0.5)
			c.FillRoundedRect(rect(0, 20, 64, 12), render.Radii{}, red)
			c.Pop()
			c.Pop()
			// The group's own content before the blur is in the backdrop:
			// blue at half over the stripes, then the blur of that at half.
			c.PushOpacity(0.5)
			c.FillRoundedRect(rect(0, 36, 64, 12), render.Radii{}, blue)
			c.PushBackdropBlur(rect(8, 36, 48, 12), render.Radii{}, 8)
			c.Pop()
			c.Pop()
		})
		if err != nil {
			t.Error(err)
		}
		s.isGray("blur in opacity, over black", 64, 8, 9, 10, 25, 10, 41, 10)
		s.isGray("blur in opacity, over white", 191, 8, 13, 10, 29, 10, 53, 10)
		s.isStripes("blur in opacity, outside", 2, 10, 6, 10, 58, 10, 18, 2)
		for _, x := range []float64{12, 16, 32, 44} {
			p := s.px(x, 26)
			if absInt(int(p[2])-191) > 8 || absInt(int(p[1])-64) > 8 || absInt(int(p[0])-64) > 8 {
				t.Errorf("scale %v: opacity in blur at x %v: BGRA %v, want about 64, 64, 191", scale, x, p)
			}
		}
		s.isStripes("opacity in blur, outside", 2, 26, 6, 26, 58, 26)
		// Under the blur: B 127/255 and G 0/127 (black/white stripe); the
		// blurred copy B 191, G 64; half of each.
		for _, c := range []struct {
			x    float64
			b, g int
		}{{9, 159, 32}, {25, 159, 32}, {13, 223, 95}, {29, 223, 95}} {
			p := s.px(c.x, 42)
			if absInt(int(p[0])-c.b) > 8 || absInt(int(p[1])-c.g) > 8 {
				t.Errorf("scale %v: blur after group content at x %v: BGRA %v, want B %d, G %d", scale, c.x, p, c.b, c.g)
			}
		}
		s.is("group content outside the blur, over black", [4]byte{127, 0, 0, 255}, 2, 2, 42)
		s.is("group content outside the blur, over white", [4]byte{255, 127, 127, 255}, 2, 6, 42)
	})
}

// TestCanvasBlurTransform: a blur under translate and scale blurs the
// transformed shape, with σ scaled by the transform; two blurs in a frame
// keep their own σ.
func TestCanvasBlurTransform(t *testing.T) {
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		s, c, _ := e.frame(scale, false, func(c *canvas) {
			stripes(c, 64, 64)
			// 16…48 × 8…24 in the surface; radius 4 in user space is σ 4 pt.
			c.Save()
			c.Translate(16, 8)
			c.Scale(2, 2)
			c.PushBackdropBlur(rect(0, 0, 16, 8), render.Radii{}, 4)
			c.FillRoundedRect(rect(-8, 0, 4, 8), render.Radii{}, red) // 0…8: outside the shape
			c.Pop()
			c.Restore()
			// A second, weaker blur (σ 1 pt) below: the stripes survive
			// softened, and the first blur stays fully mixed.
			c.PushBackdropBlur(rect(16, 36, 32, 16), render.Radii{}, 2)
			c.Pop()
		})
		if c.blurs != 2 {
			t.Errorf("%d blurred copies, want 2", c.blurs)
		}
		s.isGray("scaled blur", 127, 8, 20, 16, 22, 16, 26, 16, 30, 16, 40, 16)
		s.isStripes("outside the scaled blur", 2, 16, 6, 16, 14, 16, 50, 16, 20, 4, 20, 26)
		dark, light := s.gray(41, 44), s.gray(45, 44)
		if dark < 15 || dark > 90 || light < 165 || light > 240 {
			t.Errorf("scale %v: the weak blur: %d over black, %d over white; want softened stripes", scale, dark, light)
		}
	})
}

// TestCanvasBlurScratch: the scratch bitmap is reused at the same size and
// DPI, grown for a larger copy, recreated at another DPI, and released with
// the target.
func TestCanvasBlurScratch(t *testing.T) {
	e := newCanvasEnv(t)
	blur := func(r render.Rect) func(c *canvas) {
		return func(c *canvas) {
			stripes(c, 64, 64)
			c.PushBackdropBlur(r, render.Radii{}, 8)
			c.Pop()
		}
	}
	steps := []struct {
		scale  float64
		paint  func(c *canvas)
		made   int
		w, h   uint32
		reason string
	}{
		{1, blur(rect(16, 16, 16, 16)), 1, 40, 40, "first copy (16 + 2·12 pt)"},
		{1, blur(rect(24, 24, 8, 8)), 1, 40, 40, "a smaller copy reuses it"},
		{1, func(c *canvas) {
			blur(rect(16, 16, 16, 16))(c)
			c.PushBackdropBlur(rect(16, 16, 32, 8), render.Radii{}, 8) // wider: grown mid-frame, the old one retired
			c.Pop()
		}, 2, 56, 40, "grown"},
		{2, blur(rect(16, 16, 16, 16)), 3, 80, 80, "another DPI"},
	}
	for _, st := range steps {
		s, _, _ := e.frame(st.scale, false, st.paint)
		b := &e.backdrop
		if b.made != st.made || b.w != st.w || b.h != st.h || b.dpi != float32(96*st.scale) {
			t.Errorf("%s: %d made, %d×%d px at DPI %v; want %d, %d×%d", st.reason, b.made, b.w, b.h, b.dpi, st.made, st.w, st.h)
		}
		s.isGray(st.reason, 127, 8, 24, 24)
	}
	if n := e.backdrop.bmp.RefCount(); n != 1 {
		t.Errorf("the scratch bitmap has %d references between frames, want 1", n)
	}
}

// TestSurfaceBlur: the stripe check on a real surface, the swap chain's
// back buffer as the source, read back before Present; a device loss
// releases the scratch bitmap with every count at 0 and the repaint makes
// a new one.
func TestSurfaceBlur(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{}
		r.onPaint = func(cv render.Canvas) {
			cv.FillRoundedRect(rect(0, 0, 120, 100), render.Radii{}, render.Solid(driver.RGB8(255, 255, 255)))
			for x := 0.0; x < 120; x += 8 {
				cv.FillRoundedRect(rect(x, 0, 4, 100), render.Radii{}, black)
			}
			cv.PushBackdropBlur(rect(24, 24, 64, 48), render.UniformRadii(8), 8)
			cv.FillRoundedRect(rect(0, 60, 120, 4), render.Radii{}, render.Solid(driver.RGB8(255, 0, 0)))
			cv.Pop()
		}
		sf := newSurface(t, s, resizable, r)
		defer sf.Close()
		check := func(what string) {
			t.Helper()
			w, _, pix := framePx(t, sf)
			px := float64(sf.dpi) / 96
			at := func(x, y float64) [4]byte {
				o := (int(y*px)*w + int(x*px)) * 4
				return [4]byte(pix[o : o+4])
			}
			for _, x := range []float64{30, 34, 50, 62, 80} {
				if p := at(x, 40); absInt(int(p[1])-127) > 8 || absInt(int(p[0])-int(p[2])) > 2 {
					t.Errorf("%s: inside at x %v: BGRA %v, want gray", what, x, p)
				}
			}
			for _, pt := range [][2]float64{{2, 40}, {6, 40}, {18, 40}, {22, 40}, {90, 40}, {94, 40}, {26, 20}, {30, 20}, {24.5, 24.5}} {
				want := stripe(pt[0])
				if p := at(pt[0], pt[1]); absInt(int(p[1])-int(want[1])) > 1 {
					t.Errorf("%s: outside at %v: BGRA %v, want %v", what, pt, p, want)
				}
			}
			if p := at(50, 62); p[2] < 250 || p[1] > 5 {
				t.Errorf("%s: drawn inside the blur: BGRA %v, want red", what, p)
			}
			if p := at(10, 62); absInt(int(p[1])-int(stripe(10)[1])) > 1 {
				t.Errorf("%s: drawn inside the blur but outside the shape: BGRA %v", what, p)
			}
		}
		check("first frame")
		if sf.backdrop.bmp.IsNil() || sf.backdrop.made != 1 {
			t.Fatalf("after the first frame: scratch %v, %d made", !sf.backdrop.bmp.IsNil(), sf.backdrop.made)
		}
		check("second frame")
		if sf.backdrop.made != 1 {
			t.Errorf("the second frame made a new scratch bitmap (%d made)", sf.backdrop.made)
		}

		s.fault = fault{op: "EndDraw", hr: com.D2DERR_RECREATE_TARGET}
		w32.SendMessage(sf.hwnd, w32.WM_PAINT, 0, 0)
		rethrow()
		if s.losses != 1 || len(s.leaks) != 0 {
			t.Fatalf("device loss: %d losses, leaks %v", s.losses, s.leaks)
		}
		if !sf.backdrop.bmp.IsNil() {
			t.Error("the scratch bitmap survived the device loss")
		}
		check("after the device loss")
		if sf.backdrop.bmp.IsNil() || sf.backdrop.made != 2 {
			t.Errorf("after the repaint: scratch %v, %d made; want a new one", !sf.backdrop.bmp.IsNil(), sf.backdrop.made)
		}
	})
}

// TestCanvasBlurStale: a copy smaller than the scratch bitmap leaves the
// rest of it stale (here red from an earlier frame); the crop keeps it out
// of the blur, also at the target's edge, where the copy ends at the shape.
func TestCanvasBlurStale(t *testing.T) {
	e := newCanvasEnv(t)
	e.frame(1, false, func(c *canvas) {
		c.FillRoundedRect(rect(0, 0, 128, 128), render.Radii{}, red)
		c.PushBackdropBlur(rect(8, 8, 48, 48), render.Radii{}, 8)
		c.Pop()
	})
	s, _, _ := e.frame(1, false, func(c *canvas) {
		stripes(c, 128, 128)
		c.PushBackdropBlur(rect(104, 104, 24, 24), render.Radii{}, 8) // copied 92…128
		c.Pop()
	})
	if e.backdrop.made != 1 {
		t.Errorf("%d scratch bitmaps made, want 1 (reused)", e.backdrop.made)
	}
	for _, pt := range [][2]float64{{127.5, 127.5}, {127.5, 110}, {110, 127.5}, {116, 116}} {
		if p := s.px(pt[0], pt[1]); absInt(int(p[2])-int(p[1])) > 3 || absInt(int(p[0])-int(p[1])) > 3 {
			t.Errorf("at %v: BGRA %v, want gray (no stale red)", pt, p)
		}
	}
}
