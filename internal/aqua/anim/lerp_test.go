package anim_test

import (
	"testing"

	"github.com/radix29/gogui/internal/aqua/anim"
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
