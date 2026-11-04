package scroll_test

import (
	"math"
	"testing"

	"github.com/radix29/gogui/internal/aqua/scroll"
)

// near reports whether a is within tol of b, relative for |b| > 1.
func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol*math.Max(1, math.Abs(b)) }

func TestRubberBandKnownValues(t *testing.T) {
	inf := math.Inf(1)
	for _, tc := range []struct{ d, c, want float64 }{
		{0, 100, 0},
		{100, 100, 50},   // d = c: halfway to c
		{300, 100, 75},   // d = 3c
		{-100, 100, -50}, // odd
		{50, 200, 40},
		{1e12, 100, 100 * 1e12 / (1e12 + 100)},
		{inf, 100, 100},
		{-inf, 100, -100},
		{50, 0, 0},   // no overscroll
		{50, -10, 0}, // likewise
		{50, math.NaN(), 0},
	} {
		if got := scroll.RubberBand(tc.d, tc.c); !near(got, tc.want, 1e-12) {
			t.Errorf("RubberBand(%v, %v) = %v, want %v", tc.d, tc.c, got, tc.want)
		}
	}
	if got := scroll.RubberBand(math.NaN(), 100); !math.IsNaN(got) {
		t.Errorf("RubberBand(NaN, 100) = %v, want NaN", got)
	}
}

// TestRubberBandEdge checks the continuity at the edge: the offset is 0 at
// d = 0 and its slope is 1 from both sides, so a drag crosses the edge
// without a jump in position or speed.
func TestRubberBandEdge(t *testing.T) {
	for _, c := range []float64{1, 40, 250, 1e4} {
		if got := scroll.RubberBand(0, c); got != 0 {
			t.Errorf("c=%v: RubberBand(0) = %v, want 0", c, got)
		}
		for _, h := range []float64{1e-3, -1e-3, 1e-6, -1e-6} {
			slope := scroll.RubberBand(h, c) / h
			if !near(slope, 1, 2*math.Abs(h)/c) {
				t.Errorf("c=%v: slope at 0 over h=%v = %v, want 1", c, h, slope)
			}
		}
	}
}

// TestRubberBandMonotonicBounded sweeps d: the offset strictly increases,
// never moves further than the drag (|x| ≤ |d|, the band resists) and
// stays below c.
func TestRubberBandMonotonicBounded(t *testing.T) {
	for _, c := range []float64{1, 40, 250} {
		prev := math.Inf(-1)
		for d := -1e6; d <= 1e6; {
			x := scroll.RubberBand(d, c)
			if !(x > prev) {
				t.Fatalf("c=%v: not increasing at d=%v: %v after %v", c, d, x, prev)
			}
			if math.Abs(x) >= c || math.Abs(x) > math.Abs(d) {
				t.Fatalf("c=%v: RubberBand(%v) = %v, want |x| < c and |x| ≤ |d|", c, d, x)
			}
			prev = x
			// Dense near the edge, sparse far out.
			switch a := math.Abs(d); {
			case a < 10:
				d += 0.01
			case a < 1000:
				d++
			default:
				d += 997
			}
		}
	}
}

// TestRubberBandInverse checks the round trip both ways (a drag past the
// edge and back maps 1:1), the unreachable band |x| ≥ c, and c ≤ 0.
func TestRubberBandInverse(t *testing.T) {
	for _, c := range []float64{1, 40, 250, 1e4} {
		for _, d := range []float64{0, 1e-9, 0.5, 1, 10, 40, 100, 1234.5, 1e5} {
			for _, s := range []float64{1, -1} {
				x := scroll.RubberBand(s*d, c)
				if back := scroll.RubberBandInverse(x, c); !near(back, s*d, 1e-9) {
					t.Errorf("c=%v: Inverse(RubberBand(%v)) = %v", c, s*d, back)
				}
				if fwd := scroll.RubberBand(scroll.RubberBandInverse(x, c), c); !near(fwd, x, 1e-12) {
					t.Errorf("c=%v: RubberBand(Inverse(%v)) = %v", c, x, fwd)
				}
			}
		}
		for _, x := range []float64{c, 2 * c, math.Inf(1)} {
			if got := scroll.RubberBandInverse(x, c); !math.IsInf(got, 1) {
				t.Errorf("c=%v: Inverse(%v) = %v, want +Inf", c, x, got)
			}
			if got := scroll.RubberBandInverse(-x, c); !math.IsInf(got, -1) {
				t.Errorf("c=%v: Inverse(%v) = %v, want -Inf", c, -x, got)
			}
		}
	}
	for _, c := range []float64{0, -5, math.NaN()} {
		if got := scroll.RubberBandInverse(3, c); got != 0 {
			t.Errorf("Inverse(3, %v) = %v, want 0", c, got)
		}
	}
	if got := scroll.RubberBandInverse(math.NaN(), 10); !math.IsNaN(got) {
		t.Errorf("Inverse(NaN, 10) = %v, want NaN", got)
	}
}
