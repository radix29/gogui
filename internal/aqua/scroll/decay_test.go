package scroll_test

import (
	"math"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua/scroll"
)

// decays are (v0, k, vs) releases: typical flings both ways, a slow and a
// fast rate, and one barely above the stop velocity.
var decays = [][3]float64{
	{2000, 4, 10},
	{-2000, 4, 10},
	{500, 2, 5},
	{8000, 10, 1},
	{-300, 0.5, 20},
	{10.5, 4, 10},
}

// TestDecayClosedForm checks position and velocity against
// x = v0/k·(1 − e^(−k·t)) and v = v0·e^(−k·t) before the stop.
func TestDecayClosedForm(t *testing.T) {
	for _, p := range decays {
		v0, k, vs := p[0], p[1], p[2]
		d := scroll.NewDecay(v0, k, vs)
		stop := math.Log(math.Abs(v0)/vs) / k
		for _, f := range []float64{0.001, 0.1, 0.25, 0.5, 0.9, 0.999} {
			tm := time.Duration(f * stop * float64(time.Second))
			x, v := d.At(tm)
			s := tm.Seconds()
			wantX := v0 / k * (1 - math.Exp(-k*s))
			wantV := v0 * math.Exp(-k*s)
			if !near(x, wantX, 1e-9) || !near(v, wantV, 1e-9) {
				t.Errorf("%v at %v: (%v, %v), want (%v, %v)", p, tm, x, v, wantX, wantV)
			}
		}
	}
}

// TestDecayStop checks the stop: Duration is when the speed reaches vs,
// Rest is the closed-form position there (vs/k short of the asymptote),
// there is no jump at the stop, and from Duration on At holds Rest with
// zero velocity.
func TestDecayStop(t *testing.T) {
	for _, p := range decays {
		v0, k, vs := p[0], p[1], p[2]
		d := scroll.NewDecay(v0, k, vs)
		stop := math.Log(math.Abs(v0)/vs) / k
		if got := d.Duration().Seconds(); !near(got, stop, 1e-8) {
			t.Errorf("%v: Duration = %vs, want %vs", p, got, stop)
		}
		wantRest := v0 / k * (1 - math.Exp(-k*stop))
		if !near(d.Rest(), wantRest, 1e-9) {
			t.Errorf("%v: Rest = %v, want %v", p, d.Rest(), wantRest)
		}
		if got := d.Asymptote() - d.Rest(); !near(math.Abs(got), vs/k, 1e-9) {
			t.Errorf("%v: Asymptote − Rest = %v, want ±%v", p, got, vs/k)
		}
		// 1 µs before the stop the content is about vs·1 µs short of Rest.
		x, v := d.At(d.Duration() - time.Microsecond)
		if !near(math.Abs(v), vs, 1e-4) || !near(x, d.Rest(), 2e-6*vs) || math.Signbit(v) != math.Signbit(v0) {
			t.Errorf("%v: At(stop−1µs) = (%v, %v), want ≈ (%v, ±%v)", p, x, v, d.Rest(), vs)
		}
		for _, after := range []time.Duration{0, 1, time.Second, time.Hour} {
			x, v := d.At(d.Duration() + after)
			if x != d.Rest() || v != 0 || !d.Done(d.Duration()+after) {
				t.Errorf("%v: At(stop+%v) = (%v, %v), want (%v, 0) and done", p, after, x, v, d.Rest())
			}
		}
		if d.Done(d.Duration() - 1) {
			t.Errorf("%v: done before the stop", p)
		}
	}
}

// TestDecayIntegrates checks that At's velocity is the derivative of its
// position (central differences) and the position moves monotonically
// towards Rest.
func TestDecayIntegrates(t *testing.T) {
	d := scroll.NewDecay(-3000, 4, 10)
	prev, _ := d.At(0)
	const h = time.Millisecond
	for tm := h; tm < d.Duration()-h; tm += 7 * time.Millisecond {
		x1, _ := d.At(tm - h)
		x2, _ := d.At(tm + h)
		x, v := d.At(tm)
		if fd := (x2 - x1) / (2 * h.Seconds()); !near(fd, v, 1e-5) {
			t.Fatalf("at %v: dx/dt = %v, v = %v", tm, fd, v)
		}
		if !(x < prev) || x < d.Rest() {
			t.Fatalf("at %v: x = %v after %v, rest %v", tm, x, prev, d.Rest())
		}
		prev = x
	}
}

func TestDecayEdgeCases(t *testing.T) {
	// A release at or below the stop velocity has no momentum.
	for _, v0 := range []float64{0, 10, -10, 3} {
		d := scroll.NewDecay(v0, 4, 10)
		if x, v := d.At(0); d.Duration() != 0 || d.Rest() != 0 || x != 0 || v != 0 || !d.Done(0) {
			t.Errorf("v0=%v: Duration %v, Rest %v, At(0) (%v, %v), want all zero and done",
				v0, d.Duration(), d.Rest(), x, v)
		}
	}
	// t ≤ 0 is the release.
	d := scroll.NewDecay(1000, 4, 10)
	if x, v := d.At(-time.Second); x != 0 || v != 1000 {
		t.Errorf("At(−1s) = (%v, %v), want (0, 1000)", x, v)
	}
	// No stop velocity: runs to the asymptote.
	for _, vs := range []float64{0, -1, math.NaN()} {
		d := scroll.NewDecay(1000, 4, vs)
		if d.Duration() != math.MaxInt64 || d.Rest() != 250 || d.Asymptote() != 250 {
			t.Errorf("vs=%v: Duration %v, Rest %v, want forever and 250", vs, d.Duration(), d.Rest())
		}
		if x, _ := d.At(10 * time.Second); !near(x, 250, 1e-12) {
			t.Errorf("vs=%v: At(10s) = %v, want ≈ 250", vs, x)
		}
	}
	// A stop time beyond time.Duration is capped; Rest stays exact.
	d = scroll.NewDecay(1e10, 1e-9, 1e-10)
	if d.Duration() != math.MaxInt64 || !near(d.Rest(), (1e10-1e-10)/1e-9, 1e-12) {
		t.Errorf("huge: Duration %v, Rest %v", d.Duration(), d.Rest())
	}
	// Invalid rates panic.
	for _, k := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("NewDecay with k=%v did not panic", k)
				}
			}()
			scroll.NewDecay(100, k, 1)
		}()
	}
}
