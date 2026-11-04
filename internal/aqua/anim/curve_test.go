package anim_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/aqua/theme"
)

// refAt is an independent reference for Curve.At: the plain Bernstein form,
// solved for x by 200 bisection steps.
func refAt(c anim.Curve, t float64) float64 {
	b := func(p1, p2, s float64) float64 {
		u := 1 - s
		return 3*u*u*s*p1 + 3*u*s*s*p2 + s*s*s
	}
	lo, hi := 0.0, 1.0
	for range 200 {
		mid := (lo + hi) / 2
		if b(c.X1, c.X2, mid) < t {
			lo = mid
		} else {
			hi = mid
		}
	}
	return b(c.Y1, c.Y2, (lo+hi)/2)
}

// curves are the theme's curves, the CSS keywords and a few extreme ones
// (flat derivatives at the ends or in the middle, overshooting y).
func curves() map[string]anim.Curve {
	an := theme.Sequoia.Animation
	return map[string]anim.Curve{
		"linear":      anim.Linear,
		"EaseIn":      anim.Curve(an.EaseIn),
		"EaseOut":     anim.Curve(an.EaseOut),
		"EaseInOut":   anim.Curve(an.EaseInOut),
		"EaseDefault": anim.Curve(an.EaseDefault),
		"flatEnds":    {0, 0, 0, 1},
		"flatMiddle":  {1, 0, 0, 1},
		"steepEnds":   {0, 1, 1, 0},
		"backInOut":   {0.68, -0.6, 0.32, 1.6},
		"x1y0":        {1, 0, 1, 0},
		"x0y1":        {0, 1, 0, 1},
	}
}

func TestCurveEndPoints(t *testing.T) {
	for name, c := range curves() {
		for _, tc := range []struct{ t, want float64 }{
			{0, 0}, {1, 1}, {-0.5, 0}, {1.5, 1}, {math.Inf(-1), 0}, {math.Inf(1), 1}, {math.NaN(), 0},
		} {
			if got := c.At(tc.t); got != tc.want {
				t.Errorf("%s.At(%v) = %v, want %v", name, tc.t, got, tc.want)
			}
		}
	}
}

func TestCurveAgainstReference(t *testing.T) {
	for name, c := range curves() {
		// flatMiddle has dx/ds = 0 at s = 0.5 with dy/ds > 0: near there, x
		// rounded to float64 pins s only to about (1e-16)^(1/3), so no
		// solver (the reference included) gets y closer than a few 1e-6.
		tol := 1e-9
		if name == "flatMiddle" {
			tol = 1e-5
		}
		for i := 1; i < 1000; i++ {
			x := float64(i) / 1000
			if got, want := c.At(x), refAt(c, x); math.Abs(got-want) > tol {
				t.Errorf("%s.At(%v) = %v, want %v", name, x, got, want)
			}
		}
		// Right at a flat end (X2 = 1, say) 1 − t is known to only ~1e-16
		// absolute, which moves s, and so y, by up to ~1e-8.
		for _, x := range []float64{1e-12, 1e-9, 1e-6, 1 - 1e-6, 1 - 1e-9, 1 - 1e-12} {
			if got, want := c.At(x), refAt(c, x); math.Abs(got-want) > max(tol, 1e-7) {
				t.Errorf("%s.At(%v) = %v, want %v", name, x, got, want)
			}
		}
	}
}

func TestCurveMonotonic(t *testing.T) {
	for name, c := range curves() {
		if name == "backInOut" {
			continue // y overshoots on purpose
		}
		prev := 0.0
		for i := 1; i <= 10000; i++ {
			y := c.At(float64(i) / 10000)
			if y < prev-1e-9 || y < 0 || y > 1 {
				t.Fatalf("%s: At(%v) = %v after %v", name, float64(i)/10000, y, prev)
			}
			prev = y
		}
	}
}

func TestCurveKnownValues(t *testing.T) {
	for _, tc := range []struct {
		c       anim.Curve
		t, want float64
	}{
		{anim.Curve{0.42, 0, 0.58, 1}, 0.5, 0.5}, // CSS ease-in-out, symmetric
		{anim.Curve{0.25, 0.1, 0.25, 1}, 0.5, 0.8024033877399112},
		{anim.Linear, 0.3, 0.3},
		{anim.Linear, 0.77, 0.77},
	} {
		if got := tc.c.At(tc.t); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%v.At(%v) = %.16f, want %.16f", tc.c, tc.t, got, tc.want)
		}
	}
	// The symmetric ease-in-out: At(t) + At(1−t) == 1.
	c := anim.Curve(theme.Sequoia.Animation.EaseInOut)
	for i := range 101 {
		x := float64(i) / 100
		if s := c.At(x) + c.At(1-x); math.Abs(s-1) > 1e-9 {
			t.Errorf("EaseInOut.At(%v) + At(%v) = %v", x, 1-x, s)
		}
	}
}

func TestCurveOvershoot(t *testing.T) {
	c := curves()["backInOut"]
	if y := c.At(0.1); y >= 0 {
		t.Errorf("At(0.1) = %v, want below 0", y)
	}
	if y := c.At(0.9); y <= 1 {
		t.Errorf("At(0.9) = %v, want above 1", y)
	}
}

func ExampleCurve_At() {
	ease := anim.Curve{0.25, 0.1, 0.25, 1}
	fmt.Printf("%.4f %.4f %.4f\n", ease.At(0), ease.At(0.5), ease.At(1))
	// Output: 0.0000 0.8024 1.0000
}
