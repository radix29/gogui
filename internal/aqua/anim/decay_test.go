package anim_test

import (
	"math"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/aqua/theme"
)

// decayAt is the reference decay: position and velocity at e after a
// release at from with v0, at the rate k.
func decayAt(from, v0, k float64, e time.Duration) (x, v float64) {
	s := e.Seconds()
	return from + v0/k*(1-math.Exp(-k*s)), v0 * math.Exp(-k*s)
}

func near(a, b float64) bool { return math.Abs(a-b) <= 1e-9*max(1, math.Abs(b)) }

func TestDecayPerFrame(t *testing.T) {
	for _, v0 := range []float64{2000, -1500} {
		r := newRig()
		r.clk.Advance(3 * time.Millisecond) // start between frames
		start := r.clk.Now()
		const k, vs, from = 4.0, 20.0, 10.0
		var got log
		v := r.tl.NewValue(from, got.fn)
		v.Decay(v0, k, vs)
		rest := from + math.Copysign(math.Abs(v0)-vs, v0)/k
		stop := time.Duration(math.Log(math.Abs(v0)/vs) / k * float64(time.Second))
		if !v.Running() || v.Target() != rest || v.Value() != from || !r.clk.Requested() || len(got) != 0 {
			t.Fatalf("v0 %v: after Decay running %v target %v (want %v) value %v", v0, v.Running(), v.Target(), rest, v.Value())
		}
		for k2 := 1; ; k2++ {
			ft := r.step(t)
			if ft-start >= stop {
				if v.Value() != rest || v.Velocity() != 0 || v.Running() || r.clk.Requested() || r.tl.Running() != 0 {
					t.Fatalf("v0 %v: at the end %v %v running %v requested %v", v0, v.Value(), v.Velocity(), v.Running(), r.clk.Requested())
				}
				break
			}
			x, vel := decayAt(from, v0, k, ft-start)
			if !near(v.Value(), x) || !near(v.Velocity(), vel) || !v.Running() || v.Target() != rest {
				t.Fatalf("v0 %v frame %d: %v %v, want %v %v", v0, k2, v.Value(), v.Velocity(), x, vel)
			}
			if math.Signbit(v.Velocity()) != math.Signbit(v0) || math.Abs(v.Velocity()) <= vs {
				t.Fatalf("v0 %v frame %d: velocity %v", v0, k2, v.Velocity())
			}
			if (v0 > 0 && v.Value() >= rest) || (v0 < 0 && v.Value() <= rest) {
				t.Fatalf("v0 %v frame %d: %v passed the rest %v", v0, k2, v.Value(), rest)
			}
			if k2 > 1000 {
				t.Fatal("decay did not end")
			}
		}
		if got[len(got)-1] != rest {
			t.Fatalf("last change %v, want %v", got[len(got)-1], rest)
		}
	}
}

func TestDecayLateFrame(t *testing.T) {
	r := newRig()
	v := r.tl.NewValue(0, nil)
	v.Decay(1000, 3, 10)
	ft := 250 * time.Millisecond
	r.tl.Tick(ft) // one late frame: the first after the start
	x, vel := decayAt(0, 1000, 3, ft)
	if !near(v.Value(), x) || !near(v.Velocity(), vel) {
		t.Fatalf("late frame %v %v, want %v %v", v.Value(), v.Velocity(), x, vel)
	}
	r.clk.Advance(time.Hour)
	if want := (1000.0 - 10) / 3; v.Value() != want || v.Running() {
		t.Fatalf("after the end %v, want %v", v.Value(), want)
	}
}

func TestDecayNoMomentum(t *testing.T) {
	r := newRig()
	var got log
	v := r.tl.NewValue(5, got.fn)
	v.Decay(10, 4, 10) // |v0| == vs
	v.Decay(-3, 4, 10)
	if v.Running() || v.Value() != 5 || r.clk.Requested() || len(got) != 0 {
		t.Fatalf("at rest: running %v value %v requested %v", v.Running(), v.Value(), r.clk.Requested())
	}

	// From a running tween: it stops where the tween is at Now.
	v.AnimateTo(105, 10*frame, anim.Linear)
	r.step(t)
	r.clk.Advance(frame / 2)
	v.Decay(1, 4, 10)
	want := 5 + 100*float64(r.clk.Now())/float64(10*frame)
	if v.Running() || !near(v.Value(), want) || v.Velocity() != 0 || v.Target() != v.Value() {
		t.Fatalf("from a tween: running %v value %v (want %v) velocity %v", v.Running(), v.Value(), want, v.Velocity())
	}
	n := len(got)
	r.clk.Advance(time.Second) // the frame requested before runs, idle
	if r.clk.Requested() || len(got) != n {
		t.Fatal("a stopped decay kept running")
	}
}

func TestDecayRestart(t *testing.T) {
	r := newRig()
	v := r.tl.NewValue(0, nil)
	v.Decay(1000, 4, 10)
	r.step(t)
	r.step(t)
	at := r.clk.Now()
	x0 := v.Value()
	v.Decay(-500, 4, 10) // restarts from the current position
	if v.Target() != x0-490.0/4 {
		t.Fatalf("restart target %v", v.Target())
	}
	ft := r.step(t)
	if x, _ := decayAt(x0, -500, 4, ft-at); !near(v.Value(), x) {
		t.Fatalf("after restart %v, want %v", v.Value(), x)
	}
}

func TestDecayReduceMotion(t *testing.T) {
	r := newRig()
	var got log
	v := r.tl.NewValue(1, got.fn)
	v.Decay(1000, 4, 10)
	r.step(t)
	r.tl.SetReduceMotion(true) // finishes at the rest position
	if v.Value() != 1+990.0/4 || v.Running() || v.Velocity() != 0 {
		t.Fatalf("SetReduceMotion during a decay: %v running %v", v.Value(), v.Running())
	}
	r.clk.Advance(time.Second)
	if r.clk.Requested() {
		t.Fatal("frames requested after Reduce Motion")
	}

	v.Set(0)
	n := len(got)
	v.Decay(-1000, 4, 10) // jumps
	if v.Value() != -990.0/4 || v.Running() || r.clk.Requested() || len(got) != n+1 {
		t.Fatalf("Decay with Reduce Motion: %v running %v", v.Value(), v.Running())
	}
	v.Decay(5, 4, 10) // no momentum: stays
	if v.Value() != -990.0/4 || len(got) != n+1 {
		t.Fatalf("no momentum with Reduce Motion: %v", v.Value())
	}
}

func TestDecayRetarget(t *testing.T) {
	sp := anim.Spring(theme.Sequoia.Animation.ControlSpring)

	t.Run("SpringTo keeps velocity", func(t *testing.T) {
		r := newRig()
		v := r.tl.NewValue(0, nil)
		v.Decay(1000, 4, 10)
		for range 3 {
			r.step(t)
		}
		r.clk.Advance(frame / 2)
		at := r.clk.Now()
		x0, v0 := decayAt(0, 1000, 4, at)
		v.SpringTo(v.Target(), sp) // the decay's target: nothing changes
		if v.Target() != 990.0/4 {
			t.Fatal("SpringTo the rest position changed the decay")
		}
		v.SpringTo(50, sp)
		if v.Target() != 50 || !v.Running() {
			t.Fatalf("target %v", v.Target())
		}
		ft := r.step(t)
		x, vel := sp.Motion(x0, 50, v0).At(ft - at)
		if !near(v.Value(), x) || !near(v.Velocity(), vel) {
			t.Fatalf("after SpringTo %v %v, want %v %v", v.Value(), v.Velocity(), x, vel)
		}
	})

	t.Run("SpringFrom", func(t *testing.T) {
		r := newRig()
		v := r.tl.NewValue(0, nil)
		v.Decay(-1000, 4, 10)
		r.step(t)
		r.step(t)
		at := r.clk.Now()
		x0, _ := decayAt(0, -1000, 4, at)
		v.SpringFrom(0, 300, sp)
		ft := r.step(t)
		x, vel := sp.Motion(x0, 0, 300).At(ft - at)
		if !near(v.Value(), x) || !near(v.Velocity(), vel) || v.Target() != 0 {
			t.Fatalf("after SpringFrom %v %v, want %v %v", v.Value(), v.Velocity(), x, vel)
		}
	})

	t.Run("AnimateTo", func(t *testing.T) {
		r := newRig()
		v := r.tl.NewValue(0, nil)
		v.Decay(1000, 4, 10)
		r.step(t)
		at := r.clk.Now()
		x0, _ := decayAt(0, 1000, 4, at)
		v.AnimateTo(0, 10*frame, anim.Linear)
		ft := r.step(t)
		if want := anim.Lerp(x0, 0, float64(ft-at)/float64(10*frame)); !near(v.Value(), want) {
			t.Fatalf("after AnimateTo %v, want %v", v.Value(), want)
		}
	})

	t.Run("inside a tick", func(t *testing.T) {
		r := newRig()
		var b *anim.Value
		a := r.tl.NewValue(0, func(x float64) {
			if x >= 0.5 && !b.Running() && b.Value() == 0 {
				b.Decay(1000, 4, 10) // starts at the frame time
			}
		})
		b = r.tl.NewValue(0, nil)
		a.AnimateTo(1, 4*frame, anim.Linear)
		r.step(t)
		r.step(t)
		started := r.clk.Now()
		if !b.Running() || b.Value() != 0 {
			t.Fatalf("b running %v at %v", b.Running(), b.Value())
		}
		ft := r.step(t)
		if want, _ := decayAt(0, 1000, 4, ft-started); !near(b.Value(), want) {
			t.Fatalf("b %v, want %v", b.Value(), want)
		}
	})
}

func TestDecayPanics(t *testing.T) {
	r := newRig()
	v := r.tl.NewValue(0, nil)
	mustPanicWith(t, "not finite", func() { v.Decay(math.NaN(), 4, 10) })
	mustPanicWith(t, "not finite", func() { v.Decay(math.Inf(1), 4, 10) })
	for _, k := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		mustPanicWith(t, "rate", func() { v.Decay(100, k, 10) })
	}
	for _, vs := range []float64{0, -1, math.NaN()} {
		mustPanicWith(t, "stop velocity", func() { v.Decay(100, 4, vs) })
	}
	mustPanicWith(t, "rest", func() { v.Decay(math.MaxFloat64, 1e-300, 1) })
	r.tl.SetReduceMotion(true)
	mustPanicWith(t, "stop velocity", func() { v.Decay(100, 4, 0) }) // validated even when it jumps
	r.tl.SetReduceMotion(false)
	if v.Running() || v.Value() != 0 || r.clk.Requested() {
		t.Fatal("a panicking call changed the value")
	}
}
