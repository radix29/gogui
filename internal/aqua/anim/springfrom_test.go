package anim_test

import (
	"math"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/aqua/theme"
)

func TestSpringFromVelocity(t *testing.T) {
	for name, sp := range springs() {
		r := newRig()
		v := r.tl.NewValue(3, nil)
		v.SpringFrom(3, -40, sp) // at x already, but with a velocity: it moves
		if !v.Running() || v.Target() != 3 || !r.clk.Requested() {
			t.Fatalf("%s: SpringFrom did not start", name)
		}
		m := sp.Motion(3, 3, -40)
		for k := 1; ; k++ {
			ft := r.step(t)
			if m.Settled(ft, anim.DefaultTolerance) {
				if v.Value() != 3 || v.Velocity() != 0 || v.Running() || r.clk.Requested() {
					t.Fatalf("%s: settled at %v %v running %v", name, v.Value(), v.Velocity(), v.Running())
				}
				break
			}
			x, vel := m.At(ft)
			if math.Abs(v.Value()-x) >= anim.DefaultTolerance || v.Velocity() != vel {
				t.Fatalf("%s frame %d: %v %v, want %v %v", name, k, v.Value(), v.Velocity(), x, vel)
			}
			if k == 1 && v.Value() >= 3 {
				t.Fatalf("%s: the start velocity was not used: %v", name, v.Value())
			}
			if k > 1000 {
				t.Fatalf("%s: not settled", name)
			}
		}
	}
}

func TestSpringFromRetarget(t *testing.T) {
	sp := anim.Spring(theme.Sequoia.Animation.ControlSpring)
	r := newRig()
	v := r.tl.NewValue(0, nil)
	v.SpringTo(100, sp)
	r.step(t)
	r.step(t)
	r.clk.Advance(frame / 3)
	at := r.clk.Now()
	x0, _ := sp.Motion(0, 100, 0).At(at)
	v.SpringFrom(100, -250, sp) // the same target: restarts with the new velocity
	ft := r.step(t)
	x, vel := sp.Motion(x0, 100, -250).At(ft - at)
	if math.Abs(v.Value()-x) >= anim.DefaultTolerance || v.Velocity() != vel {
		t.Fatalf("after SpringFrom %v %v, want %v %v", v.Value(), v.Velocity(), x, vel)
	}

	// From a tween, inside a Tick: released at the frame time.
	w := r.tl.NewValue(0, nil)
	w.AnimateTo(10, 10*frame, anim.Linear)
	var started time.Duration
	var fired bool
	u := r.tl.NewValue(0, func(float64) {
		if !fired {
			fired = true
			started = r.clk.Now()
			w.SpringFrom(-5, 60, sp)
		}
	})
	u.AnimateTo(1, 2*frame, anim.Linear)
	r.step(t)
	wx := w.Value() // w ticked before u's change func in this frame
	ft = r.step(t)
	x, _ = sp.Motion(wx, -5, 60).At(ft - started)
	if w.Value() != x {
		t.Fatalf("SpringFrom in a tick: %v, want %v", w.Value(), x)
	}
}

func TestSpringFromNoOpAndReduceMotion(t *testing.T) {
	sp := anim.Spring(theme.Sequoia.Animation.ControlSpring)
	r := newRig()
	var got log
	v := r.tl.NewValue(2, got.fn)
	v.SpringFrom(2, 0, sp) // at rest at x, no velocity: nothing
	if v.Running() || r.clk.Requested() || len(got) != 0 {
		t.Fatal("SpringFrom at rest did something")
	}
	v.SpringFrom(4, 0, sp) // elsewhere: moves
	if !v.Running() {
		t.Fatal("SpringFrom to another value did not start")
	}

	r.tl.SetReduceMotion(true)
	if v.Value() != 4 || v.Running() {
		t.Fatalf("SetReduceMotion: %v", v.Value())
	}
	r.clk.Advance(time.Second)
	v.SpringFrom(-1, 500, sp)
	if v.Value() != -1 || v.Running() || v.Velocity() != 0 || r.clk.Requested() {
		t.Fatalf("SpringFrom with Reduce Motion: %v running %v", v.Value(), v.Running())
	}
	mustPanicWith(t, "positive", func() { v.SpringFrom(1, 0, anim.Spring{}) })
	r.tl.SetReduceMotion(false)

	mustPanicWith(t, "not finite", func() { v.SpringFrom(math.NaN(), 0, sp) })
	mustPanicWith(t, "not finite", func() { v.SpringFrom(1, math.Inf(1), sp) })
	mustPanicWith(t, "positive", func() { v.SpringFrom(1, 0, anim.Spring{Mass: 1, Stiffness: 1}) })
	if v.Running() || v.Value() != -1 {
		t.Fatal("a panicking call changed the value")
	}
}
