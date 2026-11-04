package anim_test

import (
	"testing"

	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/driver"
)

func TestLerp(t *testing.T) {
	for _, tc := range []struct{ a, b, t, want float64 }{
		{2, 6, 0, 2}, {2, 6, 1, 6}, {2, 6, 0.5, 4}, {2, 6, 0.25, 3},
		{2, 6, -0.5, 0}, {2, 6, 1.5, 8}, {6, 2, 0.25, 5}, {-1, 1, 0.5, 0},
	} {
		if got := anim.Lerp(tc.a, tc.b, tc.t); got != tc.want {
			t.Errorf("Lerp(%v, %v, %v) = %v, want %v", tc.a, tc.b, tc.t, got, tc.want)
		}
	}
	// Exact end points for values where a + (b−a)·t would round.
	for _, p := range [][2]float64{{0.1, 0.7}, {1e16, 1}, {-3.3, 1e-9}} {
		if anim.Lerp(p[0], p[1], 0) != p[0] || anim.Lerp(p[0], p[1], 1) != p[1] {
			t.Errorf("Lerp end points not exact for %v", p)
		}
	}
}

func TestLerpColor(t *testing.T) {
	a := driver.Color{R: 0.1, G: 0.2, B: 0.3, A: 0.4}
	b := driver.Color{R: 0.9, G: 0.6, B: 0.3, A: 1}
	if got := anim.LerpColor(a, b, 0); got != a {
		t.Errorf("t=0: %v", got)
	}
	if got := anim.LerpColor(a, b, 1); got != b {
		t.Errorf("t=1: %v", got)
	}
	got := anim.LerpColor(a, b, 0.5)
	want := driver.Color{R: 0.5, G: 0.4, B: 0.3, A: 0.7}
	for i, d := range []float32{got.R - want.R, got.G - want.G, got.B - want.B, got.A - want.A} {
		if d > 1e-6 || d < -1e-6 {
			t.Errorf("t=0.5: channel %d of %v, want %v", i, got, want)
		}
	}
	// Straight alpha: the color channels do not depend on the alphas.
	c := anim.LerpColor(driver.Color{R: 1, A: 0}, driver.Color{R: 1, A: 1}, 0.5)
	if c.R != 1 || c.A != 0.5 {
		t.Errorf("straight alpha: %v", c)
	}
	// Overshoot is clamped per channel.
	if got := anim.LerpColor(a, b, 2); got.R != 1 || got.A != 1 || got.G != 1 {
		t.Errorf("t=2: %v", got)
	}
	if got := anim.LerpColor(a, b, -1); got.R != 0 || got.G != 0 || got.A != 0 {
		t.Errorf("t=-1: %v", got)
	}
	sem := driver.Color{Semantic: driver.SemanticNone + 1}
	mustPanic(t, func() { anim.LerpColor(sem, b, 0.5) })
	mustPanic(t, func() { anim.LerpColor(a, sem, 0.5) })
}

func TestLerpGeometry(t *testing.T) {
	p := anim.LerpPoint(driver.Point{X: 0, Y: 10}, driver.Point{X: 10, Y: -10}, 0.25)
	if p != (driver.Point{X: 2.5, Y: 5}) {
		t.Errorf("LerpPoint = %v", p)
	}
	s := anim.LerpSize(driver.Size{W: 10, H: 2}, driver.Size{W: 20, H: 0}, 0.5)
	if s != (driver.Size{W: 15, H: 1}) {
		t.Errorf("LerpSize = %v", s)
	}
	if s := anim.LerpSize(driver.Size{W: 10, H: 2}, driver.Size{W: 0, H: 0}, 1.5); s != (driver.Size{}) {
		t.Errorf("LerpSize overshoot = %v, want zero", s)
	}
	a := driver.Rect{X: 0, Y: 0, W: 10, H: 10}
	b := driver.Rect{X: 10, Y: -10, W: 0, H: 30}
	if r := anim.LerpRect(a, b, 0.5); r != (driver.Rect{X: 5, Y: -5, W: 5, H: 20}) {
		t.Errorf("LerpRect = %v", r)
	}
	if r := anim.LerpRect(a, b, 2); r != (driver.Rect{X: 20, Y: -20, W: 0, H: 50}) {
		t.Errorf("LerpRect overshoot = %v", r)
	}
	if anim.LerpRect(a, b, 0) != a || anim.LerpRect(a, b, 1) != b {
		t.Error("LerpRect end points")
	}
}
