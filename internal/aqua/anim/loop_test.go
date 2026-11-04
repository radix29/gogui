package anim_test

import (
	"math"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/aqua/theme"
)

// phaseAt is a loop's phase at frame time ft for a loop started at start.
func phaseAt(ft, start, period time.Duration) float64 {
	return float64((ft-start)%period) / float64(period)
}

func TestLoopPhase(t *testing.T) {
	r := newRig()
	const period = 250 * time.Millisecond
	var got log
	v := r.tl.NewValue(0, got.fn)
	v.Loop(period)
	if !v.Running() || r.tl.Running() != 1 || !r.clk.Requested() || v.Value() != 0 || len(got) != 0 {
		t.Fatalf("after Loop: running %v, requested %v, value %v", v.Running(), r.clk.Requested(), v.Value())
	}
	if v.Velocity() != 4 || v.Target() != 0 {
		t.Fatalf("velocity %v target %v, want 4 and the current value", v.Velocity(), v.Target())
	}
	wraps := 0
	prev := 0.0
	for k := 1; k <= 40; k++ { // 40 frames: over 2.6 periods
		ft := r.step(t)
		want := phaseAt(ft, 0, period)
		x := v.Value()
		if x != want || x < 0 || x >= 1 || !v.Running() || v.Velocity() != 4 || v.Target() != x {
			t.Fatalf("frame %d: phase %v velocity %v, want %v", k, x, v.Velocity(), want)
		}
		if len(got) != k || got[k-1] != x {
			t.Fatalf("frame %d: %d change calls, last %v", k, len(got), got[len(got)-1])
		}
		if x < prev {
			wraps++
		}
		prev = x
	}
	if wraps != 2 {
		t.Fatalf("%d wrap-arounds in 40 frames, want 2", wraps)
	}
	if !r.clk.Requested() {
		t.Fatal("a running loop stopped requesting frames")
	}
}

// TestLoopLateFrame: the phase depends only on the frame time, so a late
// frame, and one many periods later, land exactly where punctual frames do.
func TestLoopLateFrame(t *testing.T) {
	const period = 700 * time.Millisecond
	const start = 5 * time.Millisecond
	punctual, late := newRig(), newRig()
	punctual.clk.Advance(start)
	late.clk.Advance(start)
	a := punctual.tl.NewValue(0, nil)
	b := late.tl.NewValue(0, nil)
	a.Loop(period)
	b.Loop(period)
	var ft time.Duration
	for range 100 {
		ft = punctual.step(t)
	}
	late.tl.Tick(ft) // one late frame at the same frame time
	if a.Value() != b.Value() || b.Value() != phaseAt(ft, start, period) {
		t.Fatalf("late frame %v, punctual %v, want %v", b.Value(), a.Value(), phaseAt(ft, start, period))
	}

	// A frame a thousand hours of periods later is still exact and in [0, 1).
	far := start + 1000*time.Hour + 123456789
	late.tl.Tick(far)
	if want := phaseAt(far, start, period); b.Value() != want || want < 0 || want >= 1 {
		t.Fatalf("far frame %v, want %v", b.Value(), want)
	}
	late.tl.Tick(start + 1000*period) // a whole number of periods: back at 0
	if b.Value() != 0 || !b.Running() {
		t.Fatalf("after whole periods %v, running %v", b.Value(), b.Running())
	}

	// The phase never rounds up to 1, even where the division would.
	huge := newRig()
	h := huge.tl.NewValue(0, nil)
	h.Loop(1 << 60)
	huge.tl.Tick(1<<60 - 1)
	if x := h.Value(); x >= 1 || x != math.Nextafter(1, 0) {
		t.Fatalf("phase just before the wrap %v, want the largest float below 1", x)
	}
}

func TestLoopStopAndSet(t *testing.T) {
	r := newRig()
	const period = time.Second
	var got log
	v := r.tl.NewValue(0, got.fn)
	v.Loop(period)
	r.step(t)
	r.step(t)
	at := v.Value()
	v.Stop()
	if v.Running() || r.tl.Running() != 0 || v.Value() != at || v.Velocity() != 0 {
		t.Fatalf("Stop: running %v value %v velocity %v", v.Running(), v.Value(), v.Velocity())
	}
	frames := r.clk.Frames()
	if r.clk.Advance(time.Second) != 1 { // the frame requested before Stop runs once, idle
		t.Fatal("the pending frame did not run")
	}
	if r.clk.Requested() || r.clk.Advance(time.Second) != 0 || r.clk.Frames() != frames+1 || v.Value() != at {
		t.Fatalf("a stopped loop kept the clock running: requested %v, value %v", r.clk.Requested(), v.Value())
	}

	// Set stops it too, and Loop starts again at 0.
	n := len(got)
	v.Loop(period)
	if v.Value() != 0 || len(got) != n+1 || got[n] != 0 || !v.Running() {
		t.Fatalf("Loop again: value %v, changes %v", v.Value(), got[n:])
	}
	r.step(t)
	v.Set(0.5)
	if v.Running() || v.Value() != 0.5 || v.Velocity() != 0 || v.Target() != 0.5 {
		t.Fatalf("Set: running %v value %v", v.Running(), v.Value())
	}
	r.clk.Advance(time.Second)
	if r.clk.Requested() || r.clk.Advance(time.Second) != 0 || v.Value() != 0.5 {
		t.Fatal("Set did not stop the loop")
	}
}

func TestLoopReduceMotion(t *testing.T) {
	r := newRig()
	const period = time.Second
	var got log
	v := r.tl.NewValue(0.7, got.fn)
	r.tl.SetReduceMotion(true)
	v.Loop(period) // stays at its start
	if v.Running() || v.Value() != 0 || len(got) != 1 || got[0] != 0 || r.clk.Requested() {
		t.Fatalf("Loop with Reduce Motion: running %v value %v, requested %v", v.Running(), v.Value(), r.clk.Requested())
	}
	mustPanicWith(t, "not positive", func() { v.Loop(0) })

	r.tl.SetReduceMotion(false)
	v.Loop(period)
	r.step(t)
	r.step(t)
	if v.Value() == 0 || !v.Running() {
		t.Fatalf("the loop does not run without Reduce Motion: %v", v.Value())
	}
	n := len(got)
	r.tl.SetReduceMotion(true) // stops it at its start
	if v.Running() || v.Value() != 0 || v.Velocity() != 0 || r.tl.Running() != 0 || len(got) != n+1 {
		t.Fatalf("turning it on: running %v value %v", v.Running(), v.Value())
	}
	r.clk.Advance(time.Second) // the frame requested before runs, idle
	if r.clk.Requested() || r.clk.Advance(time.Second) != 0 {
		t.Fatal("frames requested after Reduce Motion stopped the loop")
	}

	r.tl.SetReduceMotion(false) // does not restart it
	if v.Running() || v.Value() != 0 || r.clk.Requested() {
		t.Fatalf("turning it off restarted the loop: running %v", v.Running())
	}
}

func TestLoopIdempotentAndRetime(t *testing.T) {
	r := newRig()
	const period = 300 * time.Millisecond
	var got log
	v := r.tl.NewValue(0, got.fn)
	v.Loop(period)
	for range 3 {
		r.step(t)
	}
	v.Loop(period) // the same period: nothing changes
	ft := r.step(t)
	if want := phaseAt(ft, 0, period); v.Value() != want {
		t.Fatalf("Loop with the same period restarted: %v, want %v", v.Value(), want)
	}

	// Another period retimes from the current phase, without a jump.
	at, n := v.Value(), len(got)
	v.Loop(2 * period)
	if v.Value() != at || len(got) != n || v.Velocity() != 1/(2*period).Seconds() || !v.Running() {
		t.Fatalf("retime: value %v (was %v), velocity %v, changes %v", v.Value(), at, v.Velocity(), got[n:])
	}
	r.step(t)
	if want := at + float64(frame)/float64(2*period); math.Abs(v.Value()-want) > 1e-9 {
		t.Fatalf("after the retime: %v, want %v", v.Value(), want)
	}

	// From a running tween the loop starts at 0.
	w := r.tl.NewValue(0, nil)
	w.AnimateTo(10, time.Second, anim.Linear)
	r.step(t)
	w.Loop(period)
	if w.Value() != 0 || w.Target() != 0 || w.Velocity() != 1/period.Seconds() {
		t.Fatalf("Loop from a tween: value %v target %v", w.Value(), w.Target())
	}
	start := r.clk.Now()
	ft = r.step(t)
	if want := phaseAt(ft, start, period); w.Value() != want {
		t.Fatalf("loop after the tween: %v, want %v", w.Value(), want)
	}

	for _, p := range []time.Duration{0, -time.Second} {
		mustPanicWith(t, "not positive", func() { w.Loop(p) })
	}
}

// TestLoopRetarget: AnimateTo and SpringTo from a loop start at its phase
// (and end it), even to the loop's current value.
func TestLoopRetarget(t *testing.T) {
	r := newRig()
	const period = 400 * time.Millisecond
	const d = 100 * time.Millisecond
	v := r.tl.NewValue(0, nil)
	v.Loop(period)
	for range 5 {
		r.step(t)
	}
	from := v.Value()
	start := r.clk.Now()
	v.AnimateTo(1, d, anim.Linear)
	if v.Target() != 1 || !v.Running() || r.tl.Running() != 1 {
		t.Fatalf("AnimateTo from a loop: target %v", v.Target())
	}
	ft := r.step(t)
	if want := anim.Lerp(from, 1, anim.Linear.At(float64(ft-start)/float64(d))); v.Value() != want {
		t.Fatalf("tween from the loop's phase: %v, want %v", v.Value(), want)
	}
	r.clk.Advance(time.Second)
	if v.Value() != 1 || v.Running() || r.clk.Requested() {
		t.Fatalf("the tween did not end: %v", v.Value())
	}

	// AnimateTo the loop's current value still ends the loop.
	v.Loop(period)
	r.step(t)
	at := v.Value()
	v.AnimateTo(at, d, anim.Linear)
	r.clk.Advance(time.Second)
	if v.Value() != at || v.Running() || r.clk.Requested() {
		t.Fatalf("AnimateTo the loop's value: %v (want %v), running %v", v.Value(), at, v.Running())
	}

	// A spring is released at the phase with velocity 1/period.
	sp := anim.Spring(theme.Sequoia.Animation.ControlSpring)
	v.Loop(period)
	for range 3 {
		r.step(t)
	}
	from, start = v.Value(), r.clk.Now()
	v.SpringTo(0, sp)
	ft = r.step(t)
	want, _ := sp.Motion(from, 0, 1/period.Seconds()).At(ft - start)
	if v.Value() != want || v.Target() != 0 {
		t.Fatalf("spring from the loop: %v, want %v", v.Value(), want)
	}
}

// TestLoopInTick: a loop started by a change func starts at the frame time
// and moves from the next frame on.
func TestLoopInTick(t *testing.T) {
	r := newRig()
	const period = 500 * time.Millisecond
	b := r.tl.NewValue(0.25, nil)
	a := r.tl.NewValue(0, func(float64) { b.Loop(period) })
	a.AnimateTo(1, 3*frame, anim.Linear)
	ft1 := r.step(t)
	if !b.Running() || b.Value() != 0 || r.tl.Running() != 2 || !r.clk.Requested() {
		t.Fatalf("loop started in Tick: running %v value %v", b.Running(), b.Value())
	}
	ft2 := r.step(t)
	if want := phaseAt(ft2, ft1, period); b.Value() != want {
		t.Fatalf("next frame %v, want %v", b.Value(), want)
	}
}
