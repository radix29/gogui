//go:build windows

package win32

import (
	"math"
	"slices"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
)

// The S2.4e_2 tests: group opacity and box shadows on the canvas tests'
// offscreen target (canvas_test.go), at DPI 96 and 192. Every frame also
// checks that no resource outlives it (canvasEnv.frame).

// gray reports the green channel under (x, y): 255 on the white
// background, 0 under an opaque black shadow.
func (s *shot) gray(x, y float64) int { return int(s.px(x, y)[1]) }

func TestCanvasOpacity(t *testing.T) {
	half := render.Solid(driver.RGBA(1, 0, 0, 0.5))
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		s, _, _ := e.frame(scale, false, func(c *canvas) {
			// Group opacity: two overlapping opaque rects at 0.5 compose
			// first, so the overlap is no darker than either rect alone.
			c.PushOpacity(0.5)
			if len(c.pushes) != 1 || c.pushes[0].kind != pushLayer {
				t.Errorf("PushOpacity(0.5) pushed %v, want a layer", c.pushes)
			}
			c.FillRoundedRect(rect(0, 0, 24, 16), render.Radii{}, red)
			c.FillRoundedRect(rect(16, 0, 24, 16), render.Radii{}, red)
			c.Pop()
			// Per-shape opacity: the overlap is darker (0.75).
			c.FillRoundedRect(rect(0, 16, 24, 16), render.Radii{}, half)
			c.FillRoundedRect(rect(16, 16, 24, 16), render.Radii{}, half)
			// Nested opacity multiplies: 0.5 × 0.5 of black.
			c.PushOpacity(0.5)
			c.PushOpacity(0.5)
			c.FillRoundedRect(rect(0, 32, 16, 16), render.Radii{}, black)
			c.Pop()
			c.Pop()
			// An opacity of 1 is a state frame without a layer; NaN is 0.
			c.PushOpacity(1)
			if len(c.pushes) != 0 {
				t.Errorf("PushOpacity(1) pushed %v, want nothing", c.pushes)
			}
			c.FillRoundedRect(rect(16, 32, 16, 16), render.Radii{}, black)
			c.Pop()
			c.PushOpacity(math.NaN())
			c.FillRoundedRect(rect(32, 32, 16, 16), render.Radii{}, black)
			c.Pop()
			// Left open: finish closes the layer, and the fill still shows.
			c.PushOpacity(0.5)
			c.FillRoundedRect(rect(0, 48, 16, 16), render.Radii{}, blue)
		})
		groupRed := [4]byte{127, 127, 255, 255}
		s.is("group, one rect", groupRed, 2, 8, 8, 32, 8)
		s.is("group, overlap", groupRed, 2, 20, 8)
		s.is("per shape, one rect", groupRed, 2, 8, 24, 32, 24)
		s.is("per shape, overlap", [4]byte{63, 63, 255, 255}, 2, 20, 24)
		s.is("nested", [4]byte{191, 191, 191, 255}, 2, 8, 40)
		s.is("opacity 1", cBlack, 1, 24, 40)
		s.is("opacity NaN", cWhite, 1, 40, 40)
		s.is("closed by finish", [4]byte{255, 127, 127, 255}, 2, 8, 56)
	})
}

// shadowRow returns the green channel along y from x0 to x1 (exclusive) in
// steps of one device pixel.
func (s *shot) row(y, x0, x1 float64) []int {
	var out []int
	for x := x0; x < x1; x += 1 / s.scale {
		out = append(out, s.gray(x, y))
	}
	return out
}

func monotone(t *testing.T, scale float64, what string, v []int) {
	t.Helper()
	for i := 1; i < len(v); i++ {
		if v[i] < v[i-1]-1 {
			t.Errorf("scale %v: %s not monotone at %d: %v", scale, what, i, v)
			return
		}
	}
}

func TestCanvasShadowOuter(t *testing.T) {
	k := driver.RGB8(0, 0, 0)
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		// The shape 24…40 on both axes; Blur 8 is σ 4.
		s, _, _ := e.frame(scale, false, func(c *canvas) {
			c.DrawShadow(rect(24, 24, 16, 16), render.Radii{}, render.Shadow{Blur: 8, Color: k})
			// The shadow isn't under the shape: a translucent fill over it
			// shows only the background.
			c.FillRoundedRect(rect(24, 24, 16, 16), render.Radii{}, render.Solid(driver.RGBA(1, 0, 0, 0.5)))
		})
		s.is("inside the shape", [4]byte{127, 127, 255, 255}, 2, 25, 25, 32, 32, 39, 39)
		if g := s.gray(23.5, 32); g > 160 {
			t.Errorf("scale %v: just outside the shape green %d, want dark", scale, g)
		}
		// Falloff from the left edge outwards, monotone, about Φ(-d/σ):
		// at 1σ (x 20) about 84 % white, at 3σ (x 12) all but white.
		monotone(t, scale, "falloff", reversed(s.row(32, 4, 24)))
		if g := s.gray(19.5, 32); g < 190 || g > 235 {
			t.Errorf("scale %v: green %d at 1σ outside, want about 214 (σ = Blur/2)", scale, g)
		}
		if g := s.gray(11.5, 32); g < 250 {
			t.Errorf("scale %v: green %d at 3σ outside, want about white", scale, g)
		}
		s.is("far away", cWhite, 1, 4, 4, 60, 4)

		// A larger blur reaches farther.
		wide, _, _ := e.frame(scale, false, func(c *canvas) {
			c.DrawShadow(rect(24, 24, 16, 16), render.Radii{}, render.Shadow{Blur: 16, Color: k})
		})
		if a, b := s.gray(17.5, 32), wide.gray(17.5, 32); b > a-15 {
			t.Errorf("scale %v: green 6.5 pt outside: %d at Blur 8, %d at Blur 16; want the wider one darker", scale, a, b)
		}
		wide.is("inside the wide shadow's shape", cWhite, 1, 32, 32)
	})
}

func reversed(v []int) []int {
	out := make([]int, len(v))
	for i, x := range v {
		out[len(v)-1-i] = x
	}
	return out
}

func TestCanvasShadowInset(t *testing.T) {
	k := driver.RGB8(0, 0, 0)
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		s, _, _ := e.frame(scale, false, func(c *canvas) {
			c.DrawShadow(rect(8, 8, 48, 48), render.UniformRadii(4), render.Shadow{Blur: 8, Color: k, Inset: true})
		})
		if g := s.gray(8.5, 32); g > 160 {
			t.Errorf("scale %v: just inside the edge green %d, want dark", scale, g)
		}
		monotone(t, scale, "inset falloff", s.row(32, 8, 32))
		s.is("centre", cWhite, 2, 32, 32)
		s.is("outside the shape", cWhite, 0, 7.5, 32, 56.5, 32, 32, 7.5, 32, 56.5, 1, 1)
		if g := s.gray(12.5, 32); g < 190 || g > 235 {
			t.Errorf("scale %v: green %d at 1σ inside, want about 214", scale, g)
		}

		// Sharp (Blur 0) inset shadows of 4…28 × 4…60: a spread of 4 is a
		// band 4…8 on every side; on 36…60 an offset of (4, 0) is a band on
		// the left edge only.
		s, _, _ = e.frame(scale, false, func(c *canvas) {
			c.DrawShadow(rect(4, 4, 24, 56), render.Radii{}, render.Shadow{Spread: 4, Color: k, Inset: true})
			c.DrawShadow(rect(36, 4, 24, 56), render.Radii{}, render.Shadow{Offset: render.Point{X: 4}, Color: k, Inset: true})
		})
		s.is("inset spread", cBlack, 1, 4.5, 32, 7.5, 32, 16, 4.5, 16, 7.5, 27.5, 32, 16, 59.5)
		s.is("inside the spread", cWhite, 1, 8.5, 32, 16, 32, 16, 8.5, 23.5, 32)
		s.is("outside", cWhite, 1, 3.5, 32, 28.5, 32, 32, 32)
		s.is("inset offset", cBlack, 1, 36.5, 32, 39.5, 32)
		s.is("inset offset, away from the shift", cWhite, 1, 40.5, 32, 59.5, 32, 48, 4.5, 48, 59.5)
	})
}

func TestCanvasShadowSpreadOffset(t *testing.T) {
	k := driver.RGB8(0, 0, 0)
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		s, _, _ := e.frame(scale, false, func(c *canvas) {
			// Sharp shadows: the shape 8…20 spread by 4 is 4…24.
			c.DrawShadow(rect(8, 8, 12, 12), render.Radii{}, render.Shadow{Spread: 4, Color: k})
			// 36…52 shrunk by 4 is 40…48, moved by 8 it is 48…56: visible
			// 52…56 beside the shape.
			c.DrawShadow(rect(36, 8, 16, 16), render.Radii{}, render.Shadow{Spread: -4, Offset: render.Point{X: 8}, Color: k})
			// Shrunk away entirely: nothing.
			c.DrawShadow(rect(8, 40, 16, 16), render.Radii{}, render.Shadow{Spread: -8, Blur: 4, Color: k})
		})
		s.is("spread grows", cBlack, 1, 4.5, 14, 23.5, 14, 14, 4.5, 14, 23.5, 4.5, 4.5)
		s.is("beyond the spread", cWhite, 1, 3.5, 14, 24.5, 14, 14, 3.5, 14, 24.5)
		s.is("not under the shape", cWhite, 1, 14, 14, 8.5, 8.5)
		s.is("shrunk and offset", cBlack, 1, 52.5, 16, 55.5, 16)
		s.is("beyond the shrunk shadow", cWhite, 1, 56.5, 16, 53, 11.5, 53, 20.5, 35.5, 16)
		s.is("shrunk away", cWhite, 1, 7, 48, 25, 48, 16, 39)
	})
}

func TestCanvasShadowTransform(t *testing.T) {
	k := driver.RGB8(0, 0, 0)
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		s, _, _ := e.frame(scale, false, func(c *canvas) {
			// Translate and scale: the shape 8…24, its spread-2 shadow 4…28.
			// The mask is drawn at the surface DPI and resampled by the
			// transform, so a sharp edge spreads over about a pixel of the
			// mask; the checks keep a mask pixel away from the edges.
			c.Save()
			c.Translate(8, 8)
			c.Scale(2, 2)
			c.DrawShadow(rect(0, 0, 8, 8), render.Radii{}, render.Shadow{Spread: 2, Color: k})
			c.Restore()
			// Rotated by 45° around (44, 44): a diamond of half-diagonal
			// 6√2 ≈ 8.5, its spread-3 shadow 9√2 ≈ 12.7.
			c.Save()
			c.Translate(44, 44)
			c.Concat(rotation(math.Pi / 4))
			c.DrawShadow(rect(-6, -6, 12, 12), render.Radii{}, render.Shadow{Spread: 3, Color: k})
			c.Restore()
		})
		s.is("scaled shadow", cBlack, 1, 5, 16, 26.5, 16, 16, 5, 16, 26.5)
		s.is("beyond the scaled shadow", cWhite, 1, 2.5, 16, 29.5, 16, 16, 2.5)
		s.is("not under the scaled shape", cWhite, 1, 9.5, 16, 22.5, 16, 16, 16)
		s.is("rotated shadow", cBlack, 1, 44+10.6, 44, 44, 44-10.6, 44-10.6, 44)
		s.is("beyond the rotated shadow", cWhite, 1, 44+14.5, 44, 44, 44+14.5, 44+7.5, 44+7.5)
		s.is("not under the rotated shape", cWhite, 1, 44, 44, 44+6.5, 44)
	})
}

func TestCanvasShadowCache(t *testing.T) {
	k := driver.RGB8(0, 0, 0)
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		s, _, _ := e.frame(scale, false, func(c *canvas) {
			sh := render.Shadow{Blur: 4, Color: k}
			c.DrawShadow(rect(4, 4, 12, 12), render.Radii{}, sh)
			c.DrawShadow(rect(28, 4, 12, 12), render.Radii{}, sh) // same mask and effect
			c.Save()
			c.Translate(0, 24)
			sh.Color = driver.RGB8(255, 0, 0)
			c.DrawShadow(rect(4, 4, 12, 12), render.Radii{}, sh) // same mask, new effect
			c.Restore()
			c.DrawShadow(rect(48, 4, 12, 12), render.UniformRadii(4), sh) // new mask
			if c.masksMade != 2 || c.shadowsBaked != 3 || c.maskHits != 2 {
				t.Errorf("%d masks made, %d shadows baked, %d mask hits; want 2, 3, 2", c.masksMade, c.shadowsBaked, c.maskHits)
			}
			// Nothing drawn and nothing cached for these.
			c.DrawShadow(rect(4, 48, 12, 12), render.Radii{}, render.Shadow{Blur: 4, Color: driver.RGBA(0, 0, 0, 0)})
			c.DrawShadow(rect(4, 48, 0, 12), render.Radii{}, render.Shadow{Blur: 4, Color: k})
			c.DrawShadow(rect(4, 48, -4, 12), render.Radii{}, render.Shadow{Blur: 4, Color: k})
			c.DrawShadow(rect(4, 48, 12, 12), render.Radii{}, render.Shadow{Blur: 4, Color: driver.Semantic(driver.ColorLabel)})
			c.DrawShadow(rect(4, 48, 12, 12), render.Radii{}, render.Shadow{Blur: 1e300, Color: k})
			if c.masksMade != 2 || c.shadowsBaked != 3 {
				t.Errorf("after the empty shadows: %d masks made, %d shadows baked", c.masksMade, c.shadowsBaked)
			}
		})
		if a, b := s.gray(16.5, 10), s.gray(40.5, 10); a > 200 || a != b {
			t.Errorf("scale %v: the reused shadow differs: green %d and %d", scale, a, b)
		}
		if p := s.px(16.5, 34); p[2] < 250 || p[1] > 200 {
			t.Errorf("scale %v: red shadow BGRA %v", scale, p)
		}
		s.is("empty shadows", cWhite, 1, 2, 54, 10, 46, 18, 54)
	})
}

// TestCanvasShadowCacheFrames: the shadow cache across frames (S6.2e): a
// second identical frame makes no mask and no effect and draws the same
// pixels; another DPI makes its own masks; an entry unused for shadowKeep
// paints goes with its effects, and the masks beyond shadowMaxMasks go
// oldest first but never one drawn by the paint; the device's release
// frees the rest (newCanvasEnv's cleanup checks every last reference).
func TestCanvasShadowCacheFrames(t *testing.T) {
	k := driver.RGB8(0, 0, 0)
	e := newCanvasEnv(t)
	sc := &e.g.shadows
	scene := func(c *canvas) {
		c.DrawShadow(rect(4, 4, 12, 12), render.Radii{}, render.Shadow{Blur: 4, Color: k})
		c.DrawShadow(rect(28, 4, 12, 12), render.UniformRadii(4), render.Shadow{Blur: 4, Color: driver.RGB8(255, 0, 0)})
		c.DrawShadow(rect(4, 28, 24, 24), render.UniformRadii(4), render.Shadow{Blur: 2, Spread: 2, Inset: true, Color: k})
	}
	// The device has one canvas (review P3): each frame's counts are read
	// before the next frame resets them.
	first, c1, _ := e.frame(1, false, scene)
	if c1.masksMade != 3 || c1.shadowsBaked != 3 {
		t.Errorf("first frame: %d masks made, %d shadows baked; want 3, 3", c1.masksMade, c1.shadowsBaked)
	}
	second, c2, _ := e.frame(1, false, scene)
	if c2.masksMade != 0 || c2.shadowsBaked != 0 || c2.maskHits != 3 {
		t.Errorf("second frame: %d masks made, %d shadows baked, %d hits; want 0, 0, 3", c2.masksMade, c2.shadowsBaked, c2.maskHits)
	}
	if !slices.Equal(first.pix, second.pix) {
		t.Error("the cached shadows draw other pixels")
	}
	_, c3, _ := e.frame(2, false, scene)
	if c3.masksMade != 3 {
		t.Errorf("at 2×: %d masks made, want 3", c3.masksMade)
	}
	if m, fx := sc.len(); m != 6 || fx != 6 {
		t.Errorf("%d masks, %d shadows cached; want 6, 6", m, fx)
	}
	// Age: the 2× ones are drawn on, the 1× ones not.
	for range shadowKeep {
		e.frame(2, false, scene)
	}
	if m, fx := sc.len(); m != 3 || fx != 3 {
		t.Errorf("after %d paints without the 1× shadows: %d masks, %d shadows; want 3, 3", shadowKeep, m, fx)
	}
	// Count: more masks in one paint than the bound keeps them all; the
	// next paint's trim drops the oldest down to the bound.
	many := func(n int) func(c *canvas) {
		return func(c *canvas) {
			for i := range n {
				c.DrawShadow(rect(4, 4, 8+float64(i)/16, 8), render.Radii{}, render.Shadow{Blur: 2, Color: k})
			}
		}
	}
	e.frame(1, false, many(shadowMaxMasks+10))
	if m, _ := sc.len(); m != shadowMaxMasks+10 {
		t.Errorf("one paint of %d masks keeps %d, want them all", shadowMaxMasks+10, m)
	}
	e.frame(1, false, many(1))
	if m, _ := sc.len(); m != shadowMaxMasks {
		t.Errorf("the next paint keeps %d masks, want %d", m, shadowMaxMasks)
	}
	px := 0
	for _, m := range sc.masks {
		px += int(m.pw) * int(m.ph)
	}
	for k := range sc.baked {
		m := sc.masks[k.mask]
		px += int(m.pw) * int(m.ph)
	}
	if px != sc.pixels {
		t.Errorf("the cache counts %d pixels, its masks and shadows have %d", sc.pixels, px)
	}
}

// TestCanvasShadowDebugClean: shadows and opacity are clean for the
// recorder, and a shadow inside an opacity layer and a clip is drawn with
// both.
func TestCanvasShadowDebugClean(t *testing.T) {
	eachScale(t, func(t *testing.T, e *canvasEnv, scale float64) {
		s, _, err := e.frame(scale, true, func(c *canvas) {
			c.Save()
			c.ClipRoundedRect(rect(0, 0, 32, 64), render.Radii{})
			c.PushOpacity(0.5)
			c.DrawShadow(rect(32, 8, 16, 16), render.Radii{}, render.Shadow{Spread: 4, Color: driver.RGB8(0, 0, 0)})
			c.Pop()
			c.Restore()
		})
		if err != nil {
			t.Error(err)
		}
		s.is("half opacity", [4]byte{127, 127, 127, 255}, 2, 29.5, 16)
		s.is("clipped", cWhite, 1, 34, 16, 50, 16)
	})
}
