//go:build windows

package win32

import (
	"fmt"
	"math"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/fake"
	"github.com/radix29/gogui/internal/shell/win32/com"
	"github.com/radix29/gogui/internal/shell/win32/d2d"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// The text drawing tests (S2.5f) draw on the canvas tests' offscreen
// target (canvas_test.go) and look at the ink: the pixels that are not the
// white background.

// inkBox is the bounding box of the inked pixels, in pixels; n counts them.
type inkBox struct {
	x0, y0, x1, y1 int // x1, y1 exclusive
	n              int
}

// ink returns the ink of the shot's pixels inside the pixel rect
// [x0, x1) × [y0, y1) (the whole target with ink(s, 0, 0, canvasPx,
// canvasPx)). A pixel is ink when a channel is below 250.
func ink(s *shot, x0, y0, x1, y1 int) inkBox {
	b := inkBox{x0: 1 << 30, y0: 1 << 30, x1: -1, y1: -1}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			o := y*s.pitch + x*4
			if s.pix[o] < 250 || s.pix[o+1] < 250 || s.pix[o+2] < 250 {
				b.n++
				b.x0, b.y0 = min(b.x0, x), min(b.y0, y)
				b.x1, b.y1 = max(b.x1, x+1), max(b.y1, y+1)
			}
		}
	}
	return b
}

func allInk(s *shot) inkBox { return ink(s, 0, 0, canvasPx, canvasPx) }

// inside checks that the ink lies in the logical rect r (±1 px).
func (b inkBox) inside(t *testing.T, what string, scale float64, r render.Rect) {
	t.Helper()
	if b.n == 0 {
		t.Errorf("scale %v: %s: no ink", scale, what)
		return
	}
	l, tp := int(r.X*scale)-1, int(r.Y*scale)-1
	rr, bt := int((r.X+r.W)*scale+0.999)+1, int((r.Y+r.H)*scale+0.999)+1
	if b.x0 < l || b.y0 < tp || b.x1 > rr || b.y1 > bt {
		t.Errorf("scale %v: %s: ink %d…%d × %d…%d px, outside the box %v (%d…%d × %d…%d px)",
			scale, what, b.x0, b.x1, b.y0, b.y1, r, l, rr, tp, bt)
	}
}

var textFont = render.Font{Size: 13}

// TestCanvasDrawText: black text on white inks only inside the layout's
// box, in grays only (R = G = B on every pixel: grayscale antialiasing, no
// ClearType fringes), with dark cores; at scale 2 the ink box is twice the
// one at scale 1.
func TestCanvasDrawText(t *testing.T) {
	e := newCanvasEnv(t)
	l := e.text.NewLayout("Hello", textFont, 0) // 8 + 30 pt: inside 64 pt at scale 2
	if _, ok := l.(*layout); !ok {
		t.Fatalf("layout is %T", l)
	}
	o := render.Point{X: 8, Y: 8}
	box := render.Rect{X: o.X, Y: o.Y, W: l.Size().W, H: l.Size().H}
	boxes := map[float64]inkBox{}
	for _, scale := range []float64{1, 2} {
		s, _, _ := e.frame(scale, false, func(c *canvas) {
			if m := c.dc.TextAntialiasMode(); m != d2d.TEXT_ANTIALIAS_MODE_GRAYSCALE {
				t.Errorf("text antialias mode %d, want grayscale", m)
			}
			c.DrawText(l, o, driver.RGB8(0, 0, 0))
		})
		b := allInk(s)
		b.inside(t, "Hello", scale, box)
		if b.n < 40 {
			t.Errorf("scale %v: %d ink pixels, want text", scale, b.n)
		}
		darkest := 255
		for y := range canvasPx {
			for x := range canvasPx {
				p := s.px(float64(x)/scale, float64(y)/scale)
				if p[0] != p[1] || p[1] != p[2] {
					t.Fatalf("scale %v: pixel (%d, %d) = BGRA %v: color fringe", scale, x, y, p)
				}
				darkest = min(darkest, int(p[0]))
			}
		}
		if darkest > 64 {
			t.Errorf("scale %v: darkest pixel %d, want black glyph cores", scale, darkest)
		}
		boxes[scale] = b
	}
	b1, b2 := boxes[1], boxes[2]
	for i, v := range [...][2]int{{b1.x0, b2.x0}, {b1.y0, b2.y0}, {b1.x1, b2.x1}, {b1.y1, b2.y1}} {
		if d := v[1] - 2*v[0]; d < -3 || d > 3 {
			t.Errorf("ink box edge %d: %d px at scale 2, want about 2 × %d", i, v[1], v[0])
		}
	}
}

// TestCanvasDrawTextColor: the brush's color reaches the glyphs (a red
// glyph core is red) and a translucent color draws lighter.
func TestCanvasDrawTextColor(t *testing.T) {
	e := newCanvasEnv(t)
	l := e.text.NewLayout("█", textFont, 0) // a full block: solid ink
	sz := l.Size()
	s, _, _ := e.frame(1, false, func(c *canvas) {
		c.DrawText(l, render.Point{X: 8, Y: 8}, driver.RGB8(255, 0, 0))
		c.DrawText(l, render.Point{X: 8, Y: 48}, driver.RGBA(0, 0, 0, 0.5))
	})
	cx, cy := 8+sz.W/2, 8+sz.H/2
	s.is("red block", cRed, 2, cx, cy)
	s.is("half black block", grey(128), 3, cx, cy+40)
}

// TestCanvasDrawTextTransformClip: a translation moves the ink by the same
// amount, a scale scales it, and a clip cuts it.
func TestCanvasDrawTextTransformClip(t *testing.T) {
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		l := e.text.NewLayout("Hello", textFont, 0)
		sz := l.Size()
		black := driver.RGB8(0, 0, 0)
		ref, _, _ := e.frame(scale, false, func(c *canvas) { c.DrawText(l, render.Point{X: 4, Y: 4}, black) })
		moved, _, _ := e.frame(scale, false, func(c *canvas) {
			c.Translate(20, 30)
			c.DrawText(l, render.Point{X: 4, Y: 4}, black)
		})
		r, m := allInk(ref), allInk(moved)
		d := int(20 * scale)
		dy := int(30 * scale)
		if m.n != r.n || m.x0 != r.x0+d || m.x1 != r.x1+d || m.y0 != r.y0+dy || m.y1 != r.y1+dy {
			t.Errorf("scale %v: translated ink %+v, want %+v moved by (%d, %d) px", scale, m, r, d, dy)
		}

		scaled, _, _ := e.frame(scale, false, func(c *canvas) {
			c.Scale(2, 2)
			c.DrawText(l, render.Point{X: 2, Y: 2}, black)
		})
		allInk(scaled).inside(t, "scaled by 2", scale, rect(4, 4, 2*sz.W, 2*sz.H))

		// A clip at the middle of the layout: no ink right of it, ink left.
		mid := math.Round(4 + sz.W/2) // on a pixel edge at both scales
		clipped, _, _ := e.frame(scale, false, func(c *canvas) {
			c.Save()
			c.ClipRoundedRect(rect(0, 0, mid, 60), render.Radii{})
			c.DrawText(l, render.Point{X: 4, Y: 4}, black)
			c.Restore()
		})
		px := int(mid * scale)
		if right := ink(clipped, px, 0, canvasPx, canvasPx); right.n != 0 {
			t.Errorf("scale %v: %d ink pixels right of the clip", scale, right.n)
		}
		left, refLeft := ink(clipped, 0, 0, px, canvasPx), ink(ref, 0, 0, px, canvasPx)
		if left != refLeft {
			t.Errorf("scale %v: ink left of the clip %+v, want the unclipped %+v", scale, left, refLeft)
		}

		// Group opacity fades it.
		faded, _, _ := e.frame(scale, false, func(c *canvas) {
			c.PushOpacity(0.25)
			c.DrawText(l, render.Point{X: 4, Y: 4}, black)
			c.Pop()
		})
		darkest := 255
		for y := range canvasPx {
			for x := range canvasPx {
				darkest = min(darkest, int(faded.pix[y*faded.pitch+x*4]))
			}
		}
		if darkest < 180 || darkest > 200 {
			t.Errorf("scale %v: darkest pixel at opacity 0.25: %d, want about 191", scale, darkest)
		}
	})
}

// TestCanvasDrawTextIgnored: nil layouts, layouts of the fake metrics or
// of another text system, semantic and transparent colors, non-finite
// origins and empty text draw nothing and make no resources; a layout
// drawn twice makes one native layout.
func TestCanvasDrawTextIgnored(t *testing.T) {
	e := newCanvasEnv(t)
	l := e.text.NewLayout("Hello", textFont, 0)
	other := newTestText(t).NewLayout("Hello", textFont, 0)
	black := driver.RGB8(0, 0, 0)
	var nilLayout *layout
	s, _, _ := e.frame(1, false, func(c *canvas) {
		c.DrawText(nil, render.Point{X: 4, Y: 4}, black)
		c.DrawText(nilLayout, render.Point{X: 4, Y: 4}, black)
		c.DrawText(fake.NewTextSystem().NewLayout("Hello", textFont, 0), render.Point{X: 4, Y: 4}, black)
		c.DrawText(other, render.Point{X: 4, Y: 4}, black)
		c.DrawText(l, render.Point{X: 4, Y: 4}, driver.Semantic(driver.ColorLabel))
		c.DrawText(l, render.Point{X: 4, Y: 4}, driver.RGBA(0, 0, 0, 0))
		c.DrawText(l, render.Point{X: math.NaN(), Y: 4}, black)
		c.DrawText(l, render.Point{X: 4, Y: 1e300}, black)
		c.DrawText(e.text.NewLayout("", textFont, 0), render.Point{X: 4, Y: 4}, black)
		if n := c.resources(); n != 0 {
			t.Errorf("%d resources after ignored DrawTexts", n)
		}
	})
	if b := allInk(s); b.n != 0 {
		t.Errorf("ignored DrawTexts inked %+v", b)
	}
	// The text system's drawn-layout cache holds the one reference to the
	// native layout NewLayout measured, and drawing uses it, in this frame
	// and the next: nothing is built for drawing, and the frame releases
	// nothing of it. Once evicted, the first draw builds it again.
	ts := e.text
	d := ts.drawn.m[l.(*layout)]
	if d == nil {
		t.Fatal("NewLayout kept no native layout")
	}
	held := d.nl.Ptr
	made := ts.drawn.made
	for range 2 {
		_, _, _ = e.frame(1, false, func(c *canvas) {
			c.DrawText(l, render.Point{X: 4, Y: 4}, black)
			c.DrawText(l, render.Point{X: 4, Y: 40}, driver.RGB8(255, 0, 0))
		})
		if d := ts.drawn.m[l.(*layout)]; d == nil || d.nl.Ptr != held || ts.drawn.made != made {
			t.Fatalf("after a frame: cached %v, %d native layouts built", d != nil, ts.drawn.made-made)
		}
		if n := held.RefCount(); n != 1 {
			t.Errorf("the cached native layout has %d references, want 1", n)
		}
	}
	ts.releaseDrawn(func(what string, n uint32) { t.Errorf("%s: %d references left", what, n) })
	_, _, _ = e.frame(1, false, func(c *canvas) { c.DrawText(l, render.Point{X: 4, Y: 4}, black) })
	if ts.drawn.made != made+1 || ts.drawn.m[l.(*layout)] == nil {
		t.Errorf("after an eviction: %d built", ts.drawn.made-made)
	}
	// After the frame, ended: ignored.
	_, c, _ := e.frame(1, false, func(*canvas) {})
	frame := ts.drawn.frame
	ts.releaseDrawn(nil)
	c.DrawText(l, render.Point{X: 4, Y: 4}, black)
	if ts.drawn.made != made+1 || ts.drawn.frame != frame || len(ts.drawn.m) != 0 {
		t.Error("DrawText after finish used the cache")
	}
}

// TestCanvasDrawTextCache: the drawn-layout cache keeps at most maxDrawn
// native layouts, evicting the least recently made or drawn ones, never
// one drawn in the current frame (a frame that draws more holds them all
// until the next eviction), and the text system's release empties it with
// every reference the last.
func TestCanvasDrawTextCache(t *testing.T) {
	e := newCanvasEnv(t)
	ts := e.text
	black := driver.RGB8(0, 0, 0)
	ls := make([]render.TextLayout, maxDrawn+10)
	for i := range ls {
		ls[i] = ts.NewLayout(fmt.Sprintf("line %d", i), textFont, 0)
	}
	cached := func(l render.TextLayout) bool { return ts.drawn.m[l.(*layout)] != nil }
	draw := func(ls ...render.TextLayout) {
		_, _, _ = e.frame(1, false, func(c *canvas) {
			for _, l := range ls {
				c.DrawText(l, render.Point{X: 4, Y: 4}, black)
			}
		})
	}
	if n := len(ts.drawn.m); n != maxDrawn || cached(ls[9]) || !cached(ls[10]) {
		t.Fatalf("made: %d cached, the oldest kept %v", n, cached(ls[9]))
	}
	draw(ls[10:20]...) // hits
	if ts.drawn.made != 0 {
		t.Fatalf("hits built %d", ts.drawn.made)
	}
	draw(ls...) // more than maxDrawn in one frame: all kept
	if n := len(ts.drawn.m); n != len(ls) {
		t.Fatalf("%d cached after a frame of %d", n, len(ls))
	}
	draw(ls[5])
	extra := ts.NewLayout("extra", textFont, 0) // down to maxDrawn, ls[5] and extra kept
	if n := len(ts.drawn.m); n != maxDrawn || !cached(ls[5]) || !cached(extra) {
		t.Fatalf("after the next eviction: %d cached, ls[5] %v, extra %v", n, cached(ls[5]), cached(extra))
	}
	var refs []string
	ts.release(func(what string, n uint32) { refs = append(refs, fmt.Sprintf("%s:%d", what, n)) })
	if len(refs) != 0 || len(ts.drawn.m) != 0 {
		t.Errorf("release: %v left, %d cached", refs, len(ts.drawn.m))
	}
}

// TestCanvasDrawTextWrapped: a wrapped layout draws on as many lines as it
// measures, inside its box, and a recorder sees the call.
func TestCanvasDrawTextWrapped(t *testing.T) {
	e := newCanvasEnv(t)
	l := e.text.NewLayout("one two three four five", textFont, 50)
	if l.LineCount() < 3 {
		t.Fatalf("%d lines, want a wrapped layout", l.LineCount())
	}
	sz := l.Size()
	s, _, err := e.frame(1, true, func(c *canvas) { c.DrawText(l, render.Point{X: 4, Y: 4}, driver.RGB8(0, 0, 0)) })
	if err != nil {
		t.Error(err)
	}
	b := allInk(s)
	b.inside(t, "wrapped", 1, rect(4, 4, sz.W, sz.H))
	if h := float64(b.y1 - b.y0); h < sz.H*0.6 {
		t.Errorf("ink %v px tall for %d lines of %v pt", h, l.LineCount(), sz.H)
	}
}

// TestCanvasDrawTextReleased: once the text system is released, its
// layouts draw nothing.
func TestCanvasDrawTextReleased(t *testing.T) {
	e := newCanvasEnv(t)
	ts := newTestText(t)
	l := ts.NewLayout("Hello", textFont, 0)
	ts.release(func(what string, left uint32) { t.Errorf("%s: %d left", what, left) })
	e.text = ts
	s, _, _ := e.frame(1, false, func(c *canvas) { c.DrawText(l, render.Point{X: 4, Y: 4}, driver.RGB8(0, 0, 0)) })
	if b := allInk(s); b.n != 0 {
		t.Errorf("a released system's layout inked %+v", b)
	}
}

// TestSurfaceText: a surface paints text from the shell's text system, the
// frame's native layouts are gone after it, and after a device loss the
// repaint draws the same text again.
func TestSurfaceText(t *testing.T) {
	inShell(t, func(s *Shell) {
		l := s.TextSystem().NewLayout("Hello", render.Font{Size: 20}, 0)
		sz := l.Size()
		r := &recorder{}
		r.onPaint = func(cv render.Canvas) {
			cv.FillRoundedRect(rect(0, 0, 200, 100), render.Radii{}, render.Solid(driver.RGB8(255, 255, 255)))
			cv.DrawText(l, render.Point{X: 20, Y: 20}, driver.RGB8(0, 0, 0))
		}
		sf := newSurface(t, s, resizable, r)
		defer sf.Close()
		scale := float64(sf.dpi) / 96
		frameInk := func(what string) inkBox {
			t.Helper()
			w, h, pix := framePx(t, sf)
			sh := &shot{t: t, scale: 1, pix: pix, pitch: w * 4}
			b := ink(sh, 0, 0, min(w, int(200*scale)), min(h, int(100*scale)))
			b.inside(t, what, scale, rect(20, 20, sz.W, sz.H))
			return b
		}
		first := frameInk("first frame")
		if len(s.leaks) != 0 {
			t.Errorf("leaks after a text frame: %v", s.leaks)
		}
		s.hooks.faults = []fault{{op: "EndDraw", hr: com.D2DERR_RECREATE_TARGET}}
		w32.SendMessage(sf.hwnd, w32.WM_PAINT, 0, 0)
		rethrow()
		if s.stats.losses != 1 || len(s.leaks) != 0 {
			t.Fatalf("device loss: %d losses, leaks %v", s.stats.losses, s.leaks)
		}
		if again := frameInk("after the device loss"); again != first {
			t.Errorf("ink after the device loss %+v, before %+v", again, first)
		}
	})
}

// TestCanvasDrawTextCacheEvictCost (review P17): with the cache full, a
// new layout evicts the least recently used one without looking at the
// others.
func TestCanvasDrawTextCacheEvictCost(t *testing.T) {
	e := newCanvasEnv(t)
	ts := e.text
	ls := make([]render.TextLayout, maxDrawn)
	for i := range ls {
		ls[i] = ts.NewLayout(fmt.Sprintf("line %d", i), textFont, 0)
	}
	before := ts.drawn.visits
	for i := range 20 {
		ts.NewLayout(fmt.Sprintf("more %d", i), textFont, 0)
	}
	if n := ts.drawn.visits - before; n > 20 {
		t.Errorf("20 evictions looked at %d entries, want at most 20", n)
	}
	if n := len(ts.drawn.m); n != maxDrawn || ts.drawn.m[ls[19].(*layout)] != nil || ts.drawn.m[ls[20].(*layout)] == nil {
		t.Errorf("%d cached; the 20 oldest should be gone, the rest kept", n)
	}
}
