package anim_test

import (
	"math"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/aqua/theme"
)

// after is where AnimateAfter's tween from → to over d along c, moving
// from start (the call's time + delay) on, is at the frame time ft, with
// its velocity.
func after(from, to float64, start, d time.Duration, c anim.Curve, ft time.Duration) (x, vel float64) {
	if ft < start {
		return from, 0
	}
	e := ft - start
	if e >= d {
		return to, 0
	}
	p := float64(e) / float64(d)
	return anim.Lerp(from, to, c.At(p)), (to - from) * c.Slope(p) / d.Seconds()
}

func TestAnimateAfterDelay(t *testing.T) {
	ease := anim.Curve(theme.Sequoia.Animation.EaseOut)
	const delay, d = 100 * time.Millisecond, 50 * time.Millisecond
	r := newRig()
	r.clk.Advance(5 * time.Millisecond) // nothing runs: no frames
	var got log
	v := r.tl.NewValue(0, got.fn)
	v.AnimateAfter(100, delay, d, ease)
	start := 5*time.Millisecond + delay
	if !v.Running() || v.Target() != 100 || v.Value() != 0 || v.Velocity() != 0 || r.tl.Running() != 1 || !r.clk.Requested() || len(got) != 0 {
		t.Fatalf("after AnimateAfter: running %v, target %v, value %v %v, changes %v", v.Running(), v.Target(), v.Value(), v.Velocity(), got)
	}
	held, moved := 0, 0
	for k := 1; ; k++ {
		ft := r.step(t)
		x, vel := after(0, 100, start, d, ease, ft)
		if ft < start {
			held++
			if v.Value() != 0 || v.Velocity() != 0 || len(got) != 0 || !v.Running() || v.Target() != 100 || !r.clk.Requested() {
				t.Fatalf("frame %d in the delay: %v %v, changes %v, requested %v", k, v.Value(), v.Velocity(), got, r.clk.Requested())
			}
			continue
		}
		if v.Value() != x || v.Velocity() != vel {
			t.Fatalf("frame %d: %v %v, want %v %v", k, v.Value(), v.Velocity(), x, vel)
		}
		if x == 100 {
			break
		}
		moved++
		if !v.Running() || got[len(got)-1] != x {
			t.Fatalf("frame %d: running %v, changes %v", k, v.Running(), got)
		}
		if k > 100 {
			t.Fatal("did not end")
		}
	}
	if held != 6 || moved == 0 {
		t.Fatalf("%d frames held, %d moving", held, moved)
	}
	if v.Running() || v.Velocity() != 0 || r.tl.Running() != 0 || r.clk.Requested() || r.clk.Advance(time.Second) != 0 {
		t.Fatalf("after the end: running %v, requested %v", v.Running(), r.clk.Requested())
	}
	if len(got) != moved+1 {
		t.Fatalf("%d changes, want %d", len(got), moved+1)
	}
}

func TestAnimateAfterNoOpAndNoDelay(t *testing.T) {
	r := newRig()
	var got log
	v := r.tl.NewValue(3, got.fn)
	v.AnimateAfter(3, time.Second, time.Second, anim.Linear) // at rest at x: nothing
	if v.Running() || r.clk.Requested() || len(got) != 0 {
		t.Fatal("AnimateAfter at rest at x did something")
	}

	// A delay ≤ 0 is AnimateTo.
	for _, delay := range []time.Duration{0, -time.Second} {
		r := newRig()
		ease := anim.Curve(theme.Sequoia.Animation.EaseInOut)
		a := r.tl.NewValue(0, nil)
		b := r.tl.NewValue(0, nil)
		a.AnimateAfter(10, delay, 5*frame, ease)
		b.AnimateTo(10, 5*frame, ease)
		for k := 1; b.Running(); k++ {
			r.step(t)
			if a.Value() != b.Value() || a.Velocity() != b.Velocity() || a.Running() != b.Running() {
				t.Fatalf("delay %v frame %d: %v %v, AnimateTo %v %v", delay, k, a.Value(), a.Velocity(), b.Value(), b.Velocity())
			}
		}
		if a.Value() != 10 || a.Running() {
			t.Fatalf("delay %v: ended at %v", delay, a.Value())
		}
		a.AnimateAfter(20, delay, 0, ease) // no duration either: a jump
		if a.Value() != 20 || a.Running() || r.clk.Requested() {
			t.Fatalf("delay %v, no duration: %v running %v", delay, a.Value(), a.Running())
		}
	}

	// A duration ≤ 0 after a delay: held, then a jump at the end of it.
	for _, d := range []time.Duration{0, -frame} {
		r := newRig()
		var got log
		v := r.tl.NewValue(1, got.fn)
		v.AnimateAfter(2, 2*frame+frame/2, d, anim.Linear)
		r.step(t)
		r.step(t)
		if v.Value() != 1 || !v.Running() || len(got) != 0 {
			t.Fatalf("d %v: %v in the delay", d, v.Value())
		}
		r.step(t)
		if v.Value() != 2 || v.Running() || r.clk.Requested() || len(got) != 1 {
			t.Fatalf("d %v: %v running %v at the end of the delay", d, v.Value(), v.Running())
		}
	}
}

func TestAnimateAfterRestarts(t *testing.T) {
	const delay, d = 5 * frame, 4 * frame
	ease := anim.Curve(theme.Sequoia.Animation.EaseOut)

	t.Run("mid-delay", func(t *testing.T) {
		r := newRig()
		var got log
		v := r.tl.NewValue(0, got.fn)
		v.AnimateAfter(1, delay, d, ease)
		r.step(t)
		r.step(t)
		r.step(t)
		r.clk.Advance(frame / 2)
		at := r.clk.Now()
		v.AnimateAfter(1, delay, d, ease) // the same x: the delay restarts
		if v.Value() != 0 || v.Velocity() != 0 || v.Target() != 1 || !v.Running() || len(got) != 0 {
			t.Fatalf("re-arming moved the value: %v", v.Value())
		}
		for {
			ft := r.step(t)
			x, vel := after(0, 1, at+delay, d, ease, ft)
			if v.Value() != x || v.Velocity() != vel {
				t.Fatalf("at %v: %v %v, want %v %v", ft, v.Value(), v.Velocity(), x, vel)
			}
			if ft <= 5*frame && x != 0 {
				t.Fatalf("moved at %v, before the first delay's end", ft)
			}
			if !v.Running() {
				break
			}
		}
		if v.Value() != 1 || r.clk.Requested() {
			t.Fatalf("ended at %v", v.Value())
		}
	})

	t.Run("mid-tween", func(t *testing.T) {
		r := newRig()
		var got log
		v := r.tl.NewValue(0, got.fn)
		v.AnimateAfter(100, frame, d, ease)
		r.step(t) // the end of the delay, from frame on
		r.step(t)
		r.step(t)
		r.clk.Advance(frame / 3)
		at := r.clk.Now()
		pos, _ := after(0, 100, frame, d, ease, at)
		n := len(got)
		v.AnimateAfter(-20, delay, d, anim.Linear) // stops where the tween is now
		if v.Value() != pos || v.Velocity() != 0 || v.Target() != -20 || !v.Running() || len(got) != n+1 || got[n] != pos {
			t.Fatalf("AnimateAfter mid-tween: %v %v, want %v; changes %v", v.Value(), v.Velocity(), pos, got[n:])
		}
		for {
			ft := r.step(t)
			x, vel := after(pos, -20, at+delay, d, anim.Linear, ft)
			if v.Value() != x || v.Velocity() != vel {
				t.Fatalf("at %v: %v %v, want %v %v", ft, v.Value(), v.Velocity(), x, vel)
			}
			if ft < at+delay && len(got) != n+1 {
				t.Fatalf("change func called in the delay: %v", got[n:])
			}
			if !v.Running() {
				break
			}
		}
		if v.Value() != -20 || r.clk.Requested() {
			t.Fatalf("ended at %v", v.Value())
		}
	})

	t.Run("from a spring and a loop", func(t *testing.T) {
		sp := anim.Spring(theme.Sequoia.Animation.ControlSpring)
		r := newRig()
		v := r.tl.NewValue(0, nil)
		v.SpringTo(1, sp)
		r.step(t)
		r.clk.Advance(frame / 2)
		x0, _ := sp.Motion(0, 1, 0).At(r.clk.Now())
		v.AnimateAfter(1, delay, d, ease) // the spring's target: still restarts
		if v.Value() != x0 || v.Velocity() != 0 || !v.Running() {
			t.Fatalf("from a spring: %v %v, want %v", v.Value(), v.Velocity(), x0)
		}
		r.step(t)
		if v.Value() != x0 || v.Velocity() != 0 {
			t.Fatalf("spring kept moving in the delay: %v %v", v.Value(), v.Velocity())
		}

		w := r.tl.NewValue(0, nil)
		w.Loop(10 * frame)
		r.step(t)
		r.step(t)
		ph := w.Value()
		w.AnimateAfter(0, delay, d, ease)
		if w.Value() != ph || w.Velocity() != 0 || w.Target() != 0 {
			t.Fatalf("from a loop: %v %v, want %v", w.Value(), w.Velocity(), ph)
		}
		r.step(t)
		if w.Value() != ph || w.Velocity() != 0 {
			t.Fatalf("loop kept running in the delay: %v %v", w.Value(), w.Velocity())
		}
	})
}

func TestAnimateAfterRetarget(t *testing.T) {
	const delay, d = 6 * frame, 4 * frame
	sp := anim.Spring(theme.Sequoia.Animation.ControlSpring)
	start := func(t *testing.T) (rig, *anim.Value, *log) {
		r := newRig()
		got := new(log)
		v := r.tl.NewValue(2, got.fn)
		v.AnimateAfter(8, delay, d, anim.Linear)
		r.step(t)
		r.step(t)
		r.clk.Advance(frame / 4)
		return r, v, got
	}

	t.Run("AnimateTo", func(t *testing.T) {
		r, v, _ := start(t)
		at := r.clk.Now()
		v.AnimateTo(-4, d, anim.Linear) // starts now, from the held position
		if v.Value() != 2 || v.Target() != -4 || !v.Running() {
			t.Fatalf("AnimateTo in the delay: %v target %v", v.Value(), v.Target())
		}
		ft := r.step(t)
		if want := anim.Lerp(2, -4, anim.Linear.At(float64(ft-at)/float64(d))); v.Value() != want {
			t.Fatalf("after AnimateTo %v, want %v", v.Value(), want)
		}
		r.clk.Advance(time.Second)
		if v.Value() != -4 || v.Running() || r.clk.Requested() {
			t.Fatalf("ended at %v", v.Value())
		}
	})

	t.Run("AnimateTo the same x", func(t *testing.T) {
		r, v, _ := start(t)
		v.AnimateTo(8, d, anim.Linear) // AnimateAfter's target: nothing, the delay stays
		ft := r.step(t)
		r.step(t)
		r.step(t)
		if ft >= 6*frame || v.Value() != 2 {
			t.Fatalf("AnimateTo to the target started a tween: %v", v.Value())
		}
		r.step(t) // 6·frame: the end of the delay, the tween's start
		r.step(t)
		if want, _ := after(2, 8, delay, d, anim.Linear, 7*frame); v.Value() != want {
			t.Fatalf("after the delay %v, want %v", v.Value(), want)
		}
	})

	t.Run("Set and Stop", func(t *testing.T) {
		r, v, got := start(t)
		v.Set(7)
		if v.Value() != 7 || v.Running() || v.Target() != 7 || len(*got) != 1 {
			t.Fatalf("Set in the delay: %v running %v", v.Value(), v.Running())
		}
		if r.clk.Advance(time.Second) != 1 || r.clk.Requested() || v.Value() != 7 { // the pending frame runs idle
			t.Fatal("Set did not stop AnimateAfter")
		}

		r, v, got = start(t)
		v.Stop()
		if v.Value() != 2 || v.Running() || v.Target() != 2 || len(*got) != 0 {
			t.Fatalf("Stop in the delay: %v running %v", v.Value(), v.Running())
		}
		r.clk.Advance(time.Second)
		if r.clk.Requested() || v.Value() != 2 {
			t.Fatal("Stop did not stop AnimateAfter")
		}
	})

	t.Run("SpringTo", func(t *testing.T) {
		r, v, _ := start(t)
		at := r.clk.Now()
		v.SpringTo(0, sp) // released from the held position, with no velocity
		ft := r.step(t)
		if x, _ := sp.Motion(2, 0, 0).At(ft - at); v.Value() != x {
			t.Fatalf("after SpringTo %v, want %v", v.Value(), x)
		}
	})
}

func TestAnimateAfterReduceMotion(t *testing.T) {
	const delay, d = 3*frame + frame/2, 10 * frame
	ease := anim.Curve(theme.Sequoia.Animation.EaseOut)

	t.Run("on", func(t *testing.T) {
		r := newRig()
		r.tl.SetReduceMotion(true)
		var got log
		v := r.tl.NewValue(0, got.fn)
		v.AnimateAfter(5, delay, d, ease) // the delay still runs
		if !v.Running() || v.Value() != 0 || v.Target() != 5 || !r.clk.Requested() {
			t.Fatalf("with Reduce Motion: running %v at %v", v.Running(), v.Value())
		}
		for range 3 {
			r.step(t)
			if v.Value() != 0 || !v.Running() || len(got) != 0 {
				t.Fatalf("moved in the delay: %v", v.Value())
			}
		}
		r.step(t) // the first frame after the delay: a jump
		if v.Value() != 5 || v.Velocity() != 0 || v.Running() || r.clk.Requested() || len(got) != 1 {
			t.Fatalf("at the end of the delay: %v running %v, changes %v", v.Value(), v.Running(), got)
		}
	})

	t.Run("turned on in the delay", func(t *testing.T) {
		r := newRig()
		var got log
		v := r.tl.NewValue(0, got.fn)
		v.AnimateAfter(5, delay, d, ease)
		r.step(t)
		r.clk.Advance(frame / 2)
		r.tl.SetReduceMotion(true) // keeps the delay
		if !v.Running() || v.Value() != 0 || v.Target() != 5 || len(got) != 0 {
			t.Fatalf("SetReduceMotion in the delay: running %v at %v", v.Running(), v.Value())
		}
		r.step(t)
		r.step(t)
		if v.Value() != 0 || !v.Running() {
			t.Fatalf("moved in the delay: %v", v.Value())
		}
		r.step(t) // 4·frame, past the delay: jumps
		if v.Value() != 5 || v.Running() || r.clk.Requested() || len(got) != 1 {
			t.Fatalf("at the end of the delay: %v running %v", v.Value(), v.Running())
		}
	})

	t.Run("turned on in the tween", func(t *testing.T) {
		r := newRig()
		v := r.tl.NewValue(0, nil)
		v.AnimateAfter(5, delay, d, ease)
		for range 5 {
			r.step(t)
		}
		if v.Value() == 0 || v.Value() == 5 {
			t.Fatalf("not in the tween: %v", v.Value())
		}
		r.tl.SetReduceMotion(true) // jumps at once
		if v.Value() != 5 || v.Velocity() != 0 || v.Running() || v.Target() != 5 {
			t.Fatalf("SetReduceMotion in the tween: %v running %v", v.Value(), v.Running())
		}
		r.clk.Advance(time.Second) // the frame requested before runs, idle
		if r.clk.Requested() {
			t.Fatal("frames requested after the jump")
		}
	})
}

func TestAnimateAfterInTick(t *testing.T) {
	const delay, d = 2*frame + frame/3, 3 * frame
	r := newRig()
	w := r.tl.NewValue(0, nil)
	w.AnimateTo(10, 10*frame, anim.Linear)
	var started time.Duration
	var fired bool
	u := r.tl.NewValue(0, func(float64) {
		if !fired {
			fired = true
			started = r.clk.Now() // the frame time
			w.AnimateAfter(-5, delay, d, anim.Linear)
		}
	})
	u.AnimateTo(1, 2*frame, anim.Linear)
	r.step(t)
	wx := w.Value() // w ticked before u's change func in this frame
	if want := anim.Lerp(0, 10, anim.Linear.At(float64(started)/float64(10*frame))); wx != want || started != frame {
		t.Fatalf("w at %v when AnimateAfter was called, want %v", wx, want)
	}
	if !w.Running() || w.Target() != -5 || w.Velocity() != 0 {
		t.Fatalf("AnimateAfter in a tick: running %v target %v velocity %v", w.Running(), w.Target(), w.Velocity())
	}
	for {
		ft := r.step(t)
		x, vel := after(wx, -5, started+delay, d, anim.Linear, ft)
		if w.Value() != x || w.Velocity() != vel {
			t.Fatalf("at %v: %v %v, want %v %v", ft, w.Value(), w.Velocity(), x, vel)
		}
		if !w.Running() {
			break
		}
	}
	if w.Value() != -5 || r.clk.Requested() {
		t.Fatalf("ended at %v", w.Value())
	}
}

func TestAnimateAfterPanics(t *testing.T) {
	r := newRig()
	v := r.tl.NewValue(1, nil)
	mustPanicWith(t, "not finite", func() { v.AnimateAfter(math.NaN(), time.Second, time.Second, anim.Linear) })
	mustPanicWith(t, "not finite", func() { v.AnimateAfter(math.Inf(1), time.Second, time.Second, anim.Linear) })
	mustPanicWith(t, "not finite", func() { v.AnimateAfter(math.Inf(-1), 0, time.Second, anim.Linear) })
	mustPanicWith(t, "AnimateAfter", func() { v.AnimateAfter(math.NaN(), time.Second, 0, anim.Linear) })
	if v.Running() || v.Value() != 1 || r.clk.Requested() {
		t.Fatal("a panicking call changed the value")
	}
}

// TestReduceMotionTweenStartedAheadOfNow: a frame's time is its expected
// presentation time (shell.FrameClock), so it can lie ahead of Now. An
// AnimateTo begun inside such a Tick starts at the frame time; turning
// Reduce Motion on after the Tick, before Now reaches that time, must
// finish it at once like any running tween.
func TestReduceMotionTweenStartedAheadOfNow(t *testing.T) {
	r := newRig()
	w := r.tl.NewValue(0, nil)
	u := r.tl.NewValue(0, func(float64) {
		if !w.Running() && w.Value() == 0 {
			w.AnimateTo(1, time.Second, anim.Linear) // starts at the frame time
		}
	})
	u.AnimateTo(1, time.Second, anim.Linear)
	r.tl.Tick(frame) // presented at frame, while Now is still 0
	if !w.Running() || r.clk.Now() != 0 {
		t.Fatal("setup: w did not start")
	}
	r.tl.SetReduceMotion(true)
	if w.Value() != 1 || w.Running() {
		t.Fatalf("SetReduceMotion: w at %v running %v, want 1 and stopped", w.Value(), w.Running())
	}
}
