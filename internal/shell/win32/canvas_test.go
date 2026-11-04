//go:build windows

package win32

import (
	"math"
	"runtime"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/win32/d2d"
	"github.com/radix29/gogui/internal/shell/win32/d3d11"
	"github.com/radix29/gogui/internal/shell/win32/dxgi"
)

// The canvas tests (S2.4d_2) draw on an offscreen 128×128 px render-target
// texture through the shell's device objects (newGPU), at scale 1 (DPI 96,
// 128 pt) and 2 (DPI 192, 64 pt), and read every frame back through a
// staging texture. Points are given in logical points; the pixel checked
// is the one under (x·scale, y·scale), so every check point sits well
// inside or outside a shape at both scales.

const canvasPx = 128

var (
	cWhite = [4]byte{255, 255, 255, 255}
	cRed   = [4]byte{0, 0, 255, 255}
	cGreen = [4]byte{0, 255, 0, 255}
	cBlue  = [4]byte{255, 0, 0, 255}
	cBlack = [4]byte{0, 0, 0, 255}

	red   = render.Solid(driver.RGB8(255, 0, 0))
	green = render.Solid(driver.RGB8(0, 255, 0))
	blue  = render.Solid(driver.RGB8(0, 0, 255))
	black = render.Solid(driver.RGB8(0, 0, 0))
)

// canvasEnv is the offscreen target and the shell's devices.
type canvasEnv struct {
	t        *testing.T
	g        *gpu
	tex      d3d11.Texture2D
	backdrop backdrop                       // kept across frames, as a surface keeps it
	images   imageSet                       // the images drawn, as a shell keeps them
	text     *textSystem                    // the text system drawn from, as a shell's (nil: none)
	leak     func(what string, left uint32) // replaces the frame's leak check when set
}

func newCanvasEnv(t *testing.T) *canvasEnv {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread) // registered first, so it runs last
	g, err := newGPU(false)
	if err != nil {
		t.Fatal(err)
	}
	e := &canvasEnv{t: t, g: g, text: newTestText(t)}
	e.tex, err = g.d3d.CreateTexture2D(&d3d11.Texture2DDesc{
		Width: canvasPx, Height: canvasPx, MipLevels: 1, ArraySize: 1, Format: dxgi.FORMAT_B8G8R8A8_UNORM,
		SampleDesc: dxgi.SampleDesc{Count: 1}, Usage: d3d11.USAGE_DEFAULT,
		BindFlags: d3d11.BIND_RENDER_TARGET | d3d11.BIND_SHADER_RESOURCE,
	})
	if err != nil {
		g.release(nil)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		e.images.release(func(what string, left uint32) { t.Errorf("%s: %d references left", what, left) })
		e.backdrop.release(func(what string, left uint32) { t.Errorf("%s: %d references left", what, left) })
		if n := e.tex.Release(); n != 0 {
			t.Errorf("texture: %d references left", n)
		}
		g.release(func(what string, left uint32) { t.Errorf("%s: %d references left", what, left) })
	})
	return e
}

// shot is one frame read back.
type shot struct {
	t     *testing.T
	scale float64
	pix   []byte
	pitch int
}

// px returns the pixel under the logical point (x, y).
func (s *shot) px(x, y float64) [4]byte {
	ix, iy := int(math.Floor(x*s.scale)), int(math.Floor(y*s.scale))
	o := iy*s.pitch + ix*4
	return [4]byte(s.pix[o : o+4])
}

// is checks the pixels under the points (x0, y0, x1, y1, …) against want
// (±tol per channel).
func (s *shot) is(what string, want [4]byte, tol int, xy ...float64) {
	s.t.Helper()
	for i := 0; i+1 < len(xy); i += 2 {
		got := s.px(xy[i], xy[i+1])
		for c := range 4 {
			if d := int(got[c]) - int(want[c]); d < -tol || d > tol {
				s.t.Errorf("scale %v: %s: pixel at (%v, %v) pt = BGRA %v, want %v", s.scale, what, xy[i], xy[i+1], got, want)
				break
			}
		}
	}
}

// frame draws one frame at scale through a fresh canvas on a white
// background, with the recorder when debug is set, and reads it back.
// It fails the test if EndDraw fails or a resource outlives the frame,
// and returns the canvas (released) and the recorder's errors.
func (e *canvasEnv) frame(scale float64, debug bool, paint func(c *canvas)) (*shot, *canvas, error) {
	t := e.t
	t.Helper()
	g := e.g
	surf, err := e.tex.QueryInterface(&dxgi.IID_IDXGISurface)
	if err != nil {
		t.Fatal(err)
	}
	dpi := float32(96 * scale)
	bmp, err := g.dc.CreateBitmapFromDxgiSurface(surf, &d2d.BitmapProperties1{
		PixelFormat:   d2d.PixelFormat{Format: dxgi.FORMAT_B8G8R8A8_UNORM, AlphaMode: d2d.ALPHA_MODE_PREMULTIPLIED},
		DpiX:          dpi,
		DpiY:          dpi,
		BitmapOptions: d2d.BITMAP_OPTIONS_TARGET | d2d.BITMAP_OPTIONS_CANNOT_DRAW,
	})
	surf.Release()
	if err != nil {
		t.Fatal(err)
	}
	g.dc.SetTarget(bmp.Ptr)
	g.dc.SetDpi(dpi, dpi)
	g.dc.BeginDraw()
	g.dc.Clear(d2d.ColorF{R: 1, G: 1, B: 1, A: 1})
	c := newCanvas(g, render.Size{W: canvasPx / scale, H: canvasPx / scale}, dpi, debug)
	c.target = blurTarget{bmp: bmp, w: canvasPx, h: canvasPx, scratch: &e.backdrop}
	c.images = &e.images
	c.text = e.text
	paint(c)
	recErr := c.finish()
	if m := g.dc.Transform(); m != d2d.IdentityMatrix() {
		t.Errorf("scale %v: transform %+v after finish, want the identity", scale, m)
	}
	if err := g.dc.EndDraw(); err != nil {
		t.Fatalf("scale %v: EndDraw: %v", scale, err)
	}
	if err := c.failures(); err != nil {
		t.Errorf("scale %v: resource failures: %v", scale, err)
	}
	leak := e.leak
	if leak == nil {
		leak = func(what string, left uint32) { t.Errorf("scale %v: %s: %d references left", scale, what, left) }
	}
	c.release(leak)
	if n := c.resources(); n != 0 {
		t.Errorf("scale %v: %d resources after release", scale, n)
	}
	g.dc.SetTarget(0)
	if n := bmp.Release(); n != 0 {
		t.Errorf("bitmap: %d references left", n)
	}
	pix, pitch := e.read()
	return &shot{t: t, scale: scale, pix: pix, pitch: pitch}, c, recErr
}

// read copies the texture through staging and returns its bytes and row
// pitch.
func (e *canvasEnv) read() ([]byte, int) {
	t, g := e.t, e.g
	t.Helper()
	sd := e.tex.Desc()
	sd.Usage, sd.BindFlags, sd.CPUAccessFlags, sd.MiscFlags = d3d11.USAGE_STAGING, 0, d3d11.CPU_ACCESS_READ, 0
	staging, err := g.d3d.CreateTexture2D(&sd)
	if err != nil {
		t.Fatal(err)
	}
	defer staging.Release()
	g.ctx.CopyResource(staging.Ptr, e.tex.Ptr)
	m, err := g.ctx.Map(staging.Ptr, 0, d3d11.MAP_READ, 0)
	if err != nil {
		t.Fatal(err)
	}
	pix := ffi.CopyBytes(m.PData, int(m.RowPitch)*int(sd.Height-1)+int(sd.Width)*4)
	g.ctx.Unmap(staging.Ptr, 0)
	return pix, int(m.RowPitch)
}

// eachScale runs fn at scale 1 and 2.
func eachScale(t *testing.T, fn func(t *testing.T, e *canvasEnv, scale float64)) {
	e := newCanvasEnv(t)
	for _, scale := range []float64{1, 2} {
		e.t = t
		fn(t, e, scale)
	}
}

func rect(x, y, w, h float64) render.Rect { return render.Rect{X: x, Y: y, W: w, H: h} }

func TestCanvasFills(t *testing.T) {
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		s, _, _ := e.frame(scale, false, func(c *canvas) {
			c.FillRoundedRect(rect(4, 4, 16, 16), render.Radii{}, red)
			c.FillRoundedRect(rect(24, 4, 16, 16), render.Radii{}, red) // the same brush
			c.FillRoundedRect(rect(44, 4, 16, 16), render.Radii{}, render.Solid(driver.RGBA(0, 0, 1, 0.5)))
			if n := len(c.solids); n != 2 {
				t.Errorf("%d solid brushes for two colors", n)
			}
			c.FillRoundedRect(rect(4, 24, 16, 16), render.Radii{}, render.Solid(driver.RGBA(0, 1, 0, 0))) // transparent
		})
		s.is("sharp fill", cRed, 1, 4.5, 4.5, 12, 12, 19.5, 19.5, 30, 10)
		s.is("outside", cWhite, 1, 2, 2, 21, 12, 12, 21, 12, 30)
		s.is("half alpha", [4]byte{255, 127, 127, 255}, 2, 52, 12)
	})
}

func TestCanvasRoundedRects(t *testing.T) {
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		s, _, _ := e.frame(scale, false, func(c *canvas) {
			// Uniform: the D2D rounded rect; a radius past half the side is
			// clamped to a circle.
			c.FillRoundedRect(rect(0, 0, 32, 32), render.UniformRadii(100), blue)
			// Non-uniform: a path geometry, only the top-left corner round.
			c.FillRoundedRect(rect(32, 0, 32, 32), render.Radii{TopLeft: 12}, green)
			if len(c.geoms) != 1 {
				t.Errorf("%d geometries, want 1 (only the non-uniform rect)", len(c.geoms))
			}
		})
		s.is("circle", cBlue, 1, 16, 16, 2, 16, 16, 2, 30, 16)
		s.is("circle corners", cWhite, 1, 2, 2, 30, 30)
		s.is("non-uniform", cGreen, 1, 48, 16, 63.5, 0.5, 63.5, 31.5, 32.5, 31.5)
		s.is("non-uniform corner", cWhite, 1, 33, 1)
	})
}

func TestCanvasGradient(t *testing.T) {
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		grad := render.LinearGradient(render.Point{X: 16}, render.Point{X: 48},
			render.GradientStop{Offset: 1, Color: driver.RGB8(0, 0, 255)}, // unsorted: Normalized sorts
			render.GradientStop{Offset: 0, Color: driver.RGB8(255, 0, 0)})
		s, _, _ := e.frame(scale, false, func(c *canvas) {
			c.FillRoundedRect(rect(0, 0, 64, 16), render.Radii{}, grad)
			c.FillRoundedRect(rect(0, 16, 64, 16), render.Radii{}, grad) // cached
			// One stop is that color; none draws nothing.
			c.FillRoundedRect(rect(0, 32, 32, 16), render.Radii{},
				render.LinearGradient(render.Point{}, render.Point{X: 1}, render.GradientStop{Offset: 0.3, Color: driver.RGB8(0, 255, 0)}))
			c.FillRoundedRect(rect(32, 32, 32, 16), render.Radii{}, render.LinearGradient(render.Point{}, render.Point{X: 1}))
			if len(c.grads) != 1 || len(c.stops) != 1 {
				t.Errorf("%d gradient brushes and %d stop collections, want 1 and 1", len(c.grads), len(c.stops))
			}
		})
		s.is("padding before the start", cRed, 1, 2, 8, 15, 8, 8, 24)
		s.is("padding after the end", cBlue, 1, 49, 8, 62, 24)
		mid := s.px(32, 8)
		if !(mid[2] > 100 && mid[2] < 155 && mid[0] > 100 && mid[0] < 155) {
			t.Errorf("scale %v: gradient middle BGRA %v, want about half red, half blue", scale, mid)
		}
		if q := s.px(24, 8); !(q[2] > mid[2] && q[0] < mid[0]) {
			t.Errorf("scale %v: gradient not monotonic: %v at 24, %v at 32", scale, q, mid)
		}
		s.is("single stop", cGreen, 1, 16, 40)
		s.is("no stops", cWhite, 1, 48, 40)
	})
}

func TestCanvasStrokes(t *testing.T) {
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		s, _, _ := e.frame(scale, false, func(c *canvas) {
			// A 4 pt stroke centred on x = 8 covers 6…10.
			c.StrokeRoundedRect(rect(8, 8, 48, 48), render.UniformRadii(4), 4, black)
			// A non-uniform rounded rect stroked through a path.
			c.StrokeRoundedRect(rect(16, 16, 32, 32), render.Radii{TopLeft: 8}, 2, red)
		})
		s.is("stroke inside the outline", cBlack, 1, 6.5, 32, 9.5, 32, 32, 6.5, 32, 9.5, 57.5, 32)
		s.is("stroke outside the outline", cWhite, 1, 5.5, 32, 10.5, 32, 32, 5.5, 32, 10.5, 32, 32)
		s.is("path stroke", cRed, 1, 15.5, 32, 32, 47.5, 47.5, 47.5)
		s.is("path stroke's round corner", cWhite, 1, 15.5, 15.5)
	})
}

func TestCanvasPathStrokes(t *testing.T) {
	line := func(y float64) *render.Path {
		var p render.Path
		return p.MoveTo(render.Point{X: 16, Y: y}).LineTo(render.Point{X: 48, Y: y})
	}
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		s, _, _ := e.frame(scale, false, func(c *canvas) {
			c.StrokePath(line(8), 8, black, render.StrokeStyle{})                       // butt
			c.StrokePath(line(24), 8, black, render.StrokeStyle{Cap: render.CapRound})  // round
			c.StrokePath(line(40), 8, black, render.StrokeStyle{Cap: render.CapSquare}) // square
			// Dashes in points at width 4: 8 on, 8 off; then shifted by 4.
			var full render.Path
			full.MoveTo(render.Point{Y: 54}).LineTo(render.Point{X: 64, Y: 54})
			c.StrokePath(&full, 4, black, render.StrokeStyle{Dash: []float64{8, 8}})
			var shifted render.Path
			shifted.MoveTo(render.Point{Y: 60}).LineTo(render.Point{X: 64, Y: 60})
			c.StrokePath(&shifted, 4, black, render.StrokeStyle{Dash: []float64{8, 8}, DashOffset: 4})
			if len(c.strokes) != 5 {
				t.Errorf("%d stroke styles, want 5 (three caps, two dash offsets)", len(c.strokes))
			}
		})
		s.is("butt cap", cBlack, 1, 16.5, 8)
		s.is("butt end", cWhite, 1, 14.5, 8, 49.5, 8)
		s.is("round cap", cBlack, 1, 13, 24, 50, 24)
		s.is("round cap corner", cWhite, 1, 12.5, 20.5, 51.5, 27.5)
		s.is("square cap corner", cBlack, 1, 12.5, 36.5, 51.5, 43.5)
		s.is("dashes on", cBlack, 1, 4, 54, 20, 54, 36, 54)
		s.is("dashes off", cWhite, 1, 12, 54, 28, 54, 44, 54)
		s.is("offset dashes on", cBlack, 1, 2, 60, 16, 60)
		s.is("offset dashes off", cWhite, 1, 8, 60, 24, 60)
	})
}

func TestCanvasStrokeStyleMapping(t *testing.T) {
	e := newCanvasEnv(t)
	_, _, _ = e.frame(1, false, func(c *canvas) {
		for _, m := range []struct {
			s    render.StrokeStyle
			cap  d2d.CapStyle
			join d2d.LineJoin
			dash d2d.DashStyle
			n    uint32
		}{
			{render.StrokeStyle{}, d2d.CAP_STYLE_FLAT, d2d.LINE_JOIN_MITER_OR_BEVEL, d2d.DASH_STYLE_SOLID, 0},
			{render.StrokeStyle{Cap: render.CapRound, Join: render.JoinRound}, d2d.CAP_STYLE_ROUND, d2d.LINE_JOIN_ROUND, d2d.DASH_STYLE_SOLID, 0},
			{render.StrokeStyle{Cap: render.CapSquare, Join: render.JoinBevel, Dash: []float64{1, 2, 3}}, d2d.CAP_STYLE_SQUARE, d2d.LINE_JOIN_BEVEL, d2d.DASH_STYLE_CUSTOM, 6},
		} {
			st, ok := c.strokeStyle(m.s.Normalized(), 2)
			if !ok {
				t.Fatalf("%+v: no stroke style", m.s)
			}
			if s, en, d := st.Caps(); s != m.cap || en != m.cap || d != m.cap {
				t.Errorf("%+v: caps %d %d %d, want %d", m.s, s, en, d, m.cap)
			}
			if j, ds, n := st.LineJoin(), st.DashStyle(), st.DashesCount(); j != m.join || ds != m.dash || n != m.n {
				t.Errorf("%+v: join %d, dash style %d, %d dashes; want %d, %d, %d", m.s, j, ds, n, m.join, m.dash, m.n)
			}
		}
	})
}

func TestCanvasFillPath(t *testing.T) {
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		s, _, _ := e.frame(scale, false, func(c *canvas) {
			// Two squares in the same direction: nonzero fills the inner
			// one (even-odd would leave it empty). The second subpath is
			// left open: fills close it.
			var p render.Path
			p.MoveTo(render.Point{X: 4, Y: 4}).LineTo(render.Point{X: 36, Y: 4}).LineTo(render.Point{X: 36, Y: 36}).LineTo(render.Point{X: 4, Y: 36}).Close()
			p.MoveTo(render.Point{X: 12, Y: 12}).LineTo(render.Point{X: 28, Y: 12}).LineTo(render.Point{X: 28, Y: 28}).LineTo(render.Point{X: 12, Y: 28})
			c.FillPath(&p, green)
			// Curves: a quadratic and a cubic bulge.
			var q render.Path
			q.MoveTo(render.Point{X: 40, Y: 40}).QuadTo(render.Point{X: 52, Y: 28}, render.Point{X: 64, Y: 40}).
				CubicTo(render.Point{X: 64, Y: 56}, render.Point{X: 40, Y: 56}, render.Point{X: 40, Y: 40})
			c.FillPath(&q, blue)
			c.FillPath(nil, red)
			c.FillPath(&render.Path{}, red)
		})
		s.is("outer", cGreen, 1, 6, 6, 34, 34)
		s.is("inner (nonzero)", cGreen, 1, 20, 20)
		s.is("outside", cWhite, 1, 2, 20, 38, 20)
		s.is("curves", cBlue, 1, 52, 36, 52, 50)
		s.is("outside the curves", cWhite, 1, 42, 30, 62, 30)
	})
}

func TestCanvasTransforms(t *testing.T) {
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		s, _, _ := e.frame(scale, false, func(c *canvas) {
			c.Save()
			c.Translate(16, 16)
			c.Scale(2, 2)
			c.FillRoundedRect(rect(0, 0, 8, 8), render.Radii{}, red) // 16…32
			c.Save()
			c.Concat(render.Translation(8, 0)) // in the scaled space: +16 pt
			c.FillRoundedRect(rect(0, 0, 8, 8), render.Radii{}, green)
			c.Restore()
			c.FillRoundedRect(rect(0, 10, 8, 4), render.Radii{}, blue) // 16…32, 36…44
			c.Restore()
			c.FillRoundedRect(rect(0, 0, 8, 8), render.Radii{}, black) // the identity again
		})
		s.is("scaled", cRed, 1, 17, 17, 31, 31)
		s.is("nested", cGreen, 1, 33, 17, 47, 31)
		s.is("after the inner Restore", cBlue, 1, 17, 37, 31, 43)
		s.is("after the outer Restore", cBlack, 1, 1, 1, 7, 7)
		s.is("untouched", cWhite, 1, 12, 12, 50, 20, 20, 34, 9, 9)
	})
}

func TestCanvasClips(t *testing.T) {
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		s, _, _ := e.frame(scale, false, func(c *canvas) {
			// Nested sharp clips: 0…32 ∩ 16…64 = 16…32 on x.
			c.Save()
			c.ClipRoundedRect(rect(0, 0, 32, 64), render.Radii{})
			c.Save()
			c.ClipRoundedRect(rect(16, 0, 48, 64), render.Radii{})
			if c.pushes[0].kind != pushClip || c.pushes[1].kind != pushClip {
				t.Errorf("sharp clips pushed %v, want axis-aligned clips", c.pushes)
			}
			c.FillRoundedRect(rect(0, 0, 64, 16), render.Radii{}, red)
			c.Restore()
			c.FillRoundedRect(rect(0, 16, 64, 8), render.Radii{}, green) // 0…32
			c.Restore()
			c.FillRoundedRect(rect(0, 24, 64, 4), render.Radii{}, blue) // unclipped

			// A sharp clip under a rotation takes the layer path: a diamond
			// around (32, 48), half-diagonal 8·√2 ≈ 11.3.
			c.Save()
			c.Translate(32, 48)
			c.Concat(render.Rotation(math.Pi / 4))
			c.ClipRoundedRect(rect(-8, -8, 16, 16), render.Radii{})
			if len(c.pushes) != 1 || c.pushes[0].kind != pushLayer {
				t.Errorf("rotated clip pushed %v, want a layer", c.pushes)
			}
			c.FillRoundedRect(rect(-20, -20, 40, 40), render.Radii{}, black)
			c.Restore()

			// A rounded clip: a layer with a rounded-rect mask.
			c.Save()
			c.ClipRoundedRect(rect(0, 32, 16, 16), render.UniformRadii(8))
			c.FillRoundedRect(rect(0, 32, 16, 16), render.Radii{}, green)
			c.Restore()
			if len(c.pushes) != 0 {
				t.Errorf("%d pushes left after the last Restore", len(c.pushes))
			}
		})
		s.is("clip intersection", cRed, 1, 17, 8, 31, 8)
		s.is("clipped away", cWhite, 1, 8, 8, 40, 8)
		s.is("outer clip", cGreen, 1, 4, 20, 28, 20)
		s.is("outside the outer clip", cWhite, 1, 40, 20)
		s.is("unclipped", cBlue, 1, 2, 26, 62, 26)
		s.is("diamond", cBlack, 1, 32, 48, 40, 48, 32, 39)
		s.is("diamond's bounding-box corners", cWhite, 1, 41.5, 57.5, 22.5, 38.5)
		s.is("rounded clip", cGreen, 1, 8, 40)
		s.is("rounded clip's corner", cWhite, 1, 0.5, 32.5, 15.5, 47.5)
	})
}

// TestCanvasBalance: open saves and clips are closed by finish (EndDraw
// succeeds and the next frame is unclipped); PushOpacity(1) and a sharp
// PushBackdropBlur save and restore.
func TestCanvasBalance(t *testing.T) {
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		_, c, _ := e.frame(scale, false, func(c *canvas) {
			c.Save()
			c.ClipRoundedRect(rect(0, 0, 8, 8), render.Radii{})
			c.Save()
			c.ClipRoundedRect(rect(0, 0, 4, 4), render.UniformRadii(2))
			c.Translate(5, 5)
			c.PushOpacity(0.5)
			c.ClipRoundedRect(rect(0, 0, 2, 2), render.Radii{})
		})
		if len(c.frames) != 0 || len(c.pushes) != 0 || !c.ended {
			t.Errorf("after finish: %d frames, %d pushes, ended %v", len(c.frames), len(c.pushes), c.ended)
		}
		s, _, _ := e.frame(scale, false, func(c *canvas) {
			c.PushOpacity(1) // a state frame, no layer
			c.Translate(32, 0)
			c.PushBackdropBlur(rect(0, 0, 8, 8), render.Radii{}, 4)
			c.Translate(0, 32)
			c.Pop()
			c.FillRoundedRect(rect(0, 0, 8, 8), render.Radii{}, red) // at (32, 0)
			c.Pop()
			c.FillRoundedRect(rect(0, 0, 8, 8), render.Radii{}, green)                                                           // at (0, 0)
			c.DrawShadow(rect(16, 16, 16, 16), render.Radii{}, render.Shadow{Blur: 4, Color: driver.RGB8(0, 0, 0), Inset: true}) // inside the shape only
			c.FillRoundedRect(rect(16, 16, 16, 16), render.Radii{}, render.Solid(driver.RGB8(255, 255, 255)))                    // covers it
			c.DrawImage(nil, rect(16, 16, 16, 16))
			c.FillRoundedRect(rect(60, 60, 4, 4), render.Radii{}, blue) // unclipped
		})
		s.is("inside PushOpacity", cRed, 1, 36, 4)
		s.is("after Pop", cGreen, 1, 4, 4)
		s.is("covered shadow and nil image", cWhite, 1, 24, 24, 36, 36)
		s.is("previous frame's clips gone", cBlue, 1, 62, 62)
	})
}

// TestCanvasMisuse: every contract violation is ignored without a D2D
// error, the state stays intact, and the recorder reports them.
func TestCanvasMisuse(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)
	semantic := render.Solid(driver.Semantic(driver.ColorLabel))
	misuse := func(c *canvas) {
		c.Restore() // nothing open
		c.Pop()     // nothing open
		c.PushOpacity(1)
		c.Restore() // closes a Push*: ignored
		c.Pop()
		c.Save()
		c.Pop() // closes a Save: ignored
		c.Restore()
		c.Translate(nan, 0)
		c.Scale(inf, 1)
		c.Concat(render.Transform{A: 1, D: nan})
		c.Translate(1e300, 0) // doesn't narrow to float32
		c.FillRoundedRect(rect(nan, 0, 8, 8), render.Radii{}, red)
		c.FillRoundedRect(rect(0, 0, -8, 8), render.Radii{}, red)
		c.FillRoundedRect(rect(0, 0, inf, 8), render.Radii{}, red)
		c.FillRoundedRect(rect(0, 0, 64, 64), render.Radii{}, semantic)
		c.FillRoundedRect(rect(0, 0, 64, 64), render.UniformRadii(nan), render.LinearGradient(render.Point{X: nan}, render.Point{X: 1},
			render.GradientStop{Color: driver.RGB8(255, 0, 0)}, render.GradientStop{Offset: 1, Color: driver.Semantic(driver.ColorLabel)}))
		c.StrokeRoundedRect(rect(0, 0, 64, 64), render.Radii{}, nan, red)
		c.StrokeRoundedRect(rect(0, 0, 64, 64), render.Radii{}, -1, red)
		c.StrokeRoundedRect(rect(0, 0, 64, 64), render.Radii{}, 1e300, red)
		c.FillPath(nil, red)
		var bad render.Path
		bad.MoveTo(render.Point{X: 0, Y: 0}).LineTo(render.Point{X: inf, Y: 8}).LineTo(render.Point{X: 0, Y: 8})
		c.FillPath(&bad, red)
		var ok render.Path
		ok.MoveTo(render.Point{X: 40, Y: 4}).LineTo(render.Point{X: 56, Y: 4})
		c.StrokePath(&ok, 4, red, render.StrokeStyle{Dash: []float64{nan, inf}, DashOffset: nan, MiterLimit: nan})
		c.StrokePath(&ok, 1e-300, red, render.StrokeStyle{Dash: []float64{1, 1}}) // narrows to 0
		c.StrokePath(&ok, 4, semantic, render.StrokeStyle{})
		c.DrawText(nil, render.Point{}, driver.RGB8(255, 0, 0))
		c.DrawShadow(rect(0, 0, 8, 8), render.Radii{}, render.Shadow{Color: driver.Semantic(driver.ColorLabel)})
		c.ClipRoundedRect(rect(inf, 0, 8, 8), render.Radii{}) // ignored
		// A valid fill after all that: the transform is still the identity.
		c.FillRoundedRect(rect(8, 8, 16, 16), render.Radii{}, green)
		// A clip with a negative size clips everything away.
		c.Save()
		c.ClipRoundedRect(rect(32, 32, -8, 8), render.Radii{})
		c.FillRoundedRect(rect(0, 0, 64, 64), render.Radii{}, red)
		c.Restore()
		c.Save() // left open
	}
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		for _, debug := range []bool{false, true} {
			s, _, err := e.frame(scale, debug, misuse)
			s.is("after the misuses", cGreen, 1, 9, 9, 23, 23)
			s.is("nothing else drawn", cWhite, 1, 4, 4, 30, 30, 60, 60, 4, 60)
			s.is("bad dashes normalized to a solid line", cRed, 1, 48, 4)
			switch {
			case !debug && err != nil:
				t.Errorf("without the recorder: %v", err)
			case debug && err == nil:
				t.Error("the recorder reported nothing")
			case debug:
				msg := err.Error()
				for _, want := range []string{"nothing open", "innermost open call", "non-finite", "semantic color", "nil layout", "not closed"} {
					if !strings.Contains(msg, want) {
						t.Errorf("recorder errors lack %q:\n%s", want, msg)
					}
				}
			}
		}
	})
}

// TestCanvasDebugClean: a correct paint gives no recorder errors.
func TestCanvasDebugClean(t *testing.T) {
	e := newCanvasEnv(t)
	_, _, err := e.frame(1, true, func(c *canvas) {
		c.Save()
		c.ClipRoundedRect(rect(0, 0, 32, 32), render.UniformRadii(4))
		c.FillRoundedRect(rect(0, 0, 32, 32), render.Radii{}, red)
		c.PushOpacity(0.5)
		c.Pop()
		c.Restore()
	})
	if err != nil {
		t.Error(err)
	}
}
