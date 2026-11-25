package anim_test

import (
	"math"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua/anim"
)

// TestMotionPeak checks Peak against a dense sampling of At in all three
// regimes, from rest, from the target and against the motion.
func TestMotionPeak(t *testing.T) {
	for name, s := range map[string]anim.Spring{
		"under":    {Mass: 1, Stiffness: 200, Damping: 10},
		"nearCrit": {Mass: 1, Stiffness: 200, Damping: 28.28},
		"critical": {Mass: 1, Stiffness: 100, Damping: 20},
		"over":     {Mass: 1, Stiffness: 200, Damping: 60},
	} {
		for _, st := range []struct{ from, v0 float64 }{
			{0, 1000}, {0, -1000}, {50, 0}, {50, 1000}, {50, -1000}, {-50, 3000}, {0, 0},
		} {
			m := s.Motion(st.from, 0, st.v0)
			want := 0.0
			for i := 0; i <= 400000; i++ {
				x, _ := m.At(time.Duration(i) * 10 * time.Microsecond)
				want = math.Max(want, math.Abs(x))
			}
			if got := m.Peak(); math.Abs(got-want) > 1e-6*max(1, want) {
				t.Errorf("%s from %g v0 %g: Peak %g, sampled %g", name, st.from, st.v0, got, want)
			}
		}
	}
}
