package anim_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/shell/fake"
)

const frame = fake.FrameInterval

// rig is a timeline on a standalone fake clock, with the timeline's Tick
// as the frame function.
type rig struct {
	clk *fake.Clock
	tl  *anim.Timeline
}

func newRig() rig {
	clk := fake.NewClock()
	tl := anim.NewTimeline(clk)
	clk.SetFrameFunc(tl.Tick)
	return rig{clk, tl}
}

// step runs exactly one frame (one is requested), moving Now to the next
// multiple of the frame interval, and returns that frame time.
func (r rig) step(t *testing.T) time.Duration {
	t.Helper()
	if !r.clk.Requested() {
		t.Fatal("no frame requested")
	}
	next := (r.clk.Now()/frame + 1) * frame
	if n := r.clk.Advance(next - r.clk.Now()); n != 1 {
		t.Fatalf("Advance ran %d frames", n)
	}
	return next
}

// log collects the values a change func is called with.
type log []float64

func (l *log) fn(x float64) { *l = append(*l, x) }

func mustPanicWith(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		if s, _ := r.(string); r == nil || !strings.Contains(s, want) {
			t.Errorf("panic %v, want one containing %q", r, want)
		}
	}()
	fn()
}

func TestTweenPerFrame(t *testing.T) {
	r := newRig()
	ease := anim.Curve(theme.Sequoia.Animation.EaseOut)
	const d = 100 * time.Millisecond
	var got log
	v := r.tl.NewValue(0, got.fn)
	if v.Value() != 0 || v.Running() || v.Target() != 0 || r.clk.Requested() {
		t.Fatal("a new value is not at rest")
	}
	v.AnimateTo(100, d, ease)
	if !v.Running() || v.Target() != 100 || v.Value() != 0 || r.tl.Running() != 1 || !r.clk.Requested() {
		t.Fatalf("after AnimateTo: running %v, target %v, value %v", v.Running(), v.Target(), v.Value())
	}
	v.AnimateTo(100, d, ease) // the same target: no restart
	// Frames at k·FrameInterval: 6·FrameInterval is still 4 ns short of d.
	for k := 1; k <= 6; k++ {
		ft := r.step(t)
		p := float64(ft) / float64(d)
		want := anim.Lerp(0, 100, ease.At(p))
		wantV := 100 * ease.Slope(p) / d.Seconds()
		if v.Value() != want || v.Velocity() != wantV || got[len(got)-1] != want || !v.Running() {
			t.Fatalf("frame %d: value %v velocity %v, want %v %v", k, v.Value(), v.Velocity(), want, wantV)
		}
	}
	r.step(t)
	if v.Value() != 100 || v.Velocity() != 0 || v.Running() || r.tl.Running() != 0 {
		t.Fatalf("after the end: value %v velocity %v running %v", v.Value(), v.Velocity(), v.Running())
	}
	if len(got) != 7 || r.clk.Requested() || r.clk.Advance(time.Second) != 0 {
		t.Fatalf("%d changes, requested %v", len(got), r.clk.Requested())
	}
}

func TestTweenStartsAtNow(t *testing.T) {
	r := newRig()
	r.clk.Advance(5 * time.Millisecond)
	v := r.tl.NewValue(10, nil)
	v.AnimateTo(20, 50*time.Millisecond, anim.Linear)
	ft := r.step(t)
	want := anim.Lerp(10, 20, float64(ft-5*time.Millisecond)/float64(50*time.Millisecond))
	if v.Value() != want {
		t.Fatalf("first frame %v, want %v", v.Value(), want)
	}
}

// TestLateFrame: a timeline that misses frames lands exactly where one that
// ticks every frame does, for a tween and a spring.
func TestLateFrame(t *testing.T) {
	sp := anim.Spring(theme.Sequoia.Animation.ControlSpring)
	ease := anim.Curve(theme.Sequoia.Animation.EaseInOut)
	type pair struct{ tw, sp *anim.Value }
	start := func(tl *anim.Timeline) pair {
		p := pair{tl.NewValue(0, nil), tl.NewValue(0, nil)}
		p.tw.AnimateTo(1, 200*time.Millisecond, ease)
		p.sp.SpringTo(1, sp)
		return p
	}
	punctual := newRig()
	a := start(punctual.tl)
	for range 7 {
		punctual.step(t)
	}
	lateClk := fake.NewClock()
	late := anim.NewTimeline(lateClk)
	b := start(late)
	late.Tick(frame)
	late.Tick(7 * frame) // five frames missed
	if a.tw.Value() != b.tw.Value() || a.sp.Value() != b.sp.Value() || a.sp.Velocity() != b.sp.Velocity() {
		t.Fatalf("punctual %v %v, late %v %v", a.tw.Value(), a.sp.Value(), b.tw.Value(), b.sp.Value())
	}
	x, vel := sp.Motion(0, 1, 0).At(7 * frame)
	if b.sp.Value() != x || b.sp.Velocity() != vel {
		t.Fatalf("spring at 7 frames %v %v, want %v %v", b.sp.Value(), b.sp.Velocity(), x, vel)
	}
	if !lateClk.Requested() {
		t.Error("a running timeline did not request the next frame")
	}
}

func TestSpringSettles(t *testing.T) {
	for name, sp := range springs() {
		r := newRig()
		v := r.tl.NewValue(0, nil)
		v.SpringTo(10, sp)
		m := sp.Motion(0, 10, 0)
		for k := 1; ; k++ {
			ft := r.step(t)
			if m.Settled(ft, anim.SettleEpsilon) {
				if v.Value() != 10 || v.Velocity() != 0 || v.Running() || r.clk.Requested() {
					t.Errorf("%s: settled at frame %d: %v %v running %v", name, k, v.Value(), v.Velocity(), v.Running())
				}
				break
			}
			x, vel := m.At(ft)
			if v.Value() != x || v.Velocity() != vel || !v.Running() {
				t.Fatalf("%s frame %d: %v %v, want %v %v", name, k, v.Value(), v.Velocity(), x, vel)
			}
			if k > 1000 {
				t.Fatalf("%s: not settled after %d frames", name, k)
			}
		}
	}
}

func TestRetarget(t *testing.T) {
	sp := anim.Spring(theme.Sequoia.Animation.ControlSpring)
	ease := anim.Curve(theme.Sequoia.Animation.EaseOut)
	const d = 250 * time.Millisecond

	t.Run("tween", func(t *testing.T) {
		r := newRig()
		v := r.tl.NewValue(0, nil)
		v.AnimateTo(100, d, ease)
		for range 3 {
			r.step(t)
		}
		from, at := v.Value(), r.clk.Now()
		r.clk.Advance(frame / 2) // retarget between frames, from where it is now
		mid := anim.Lerp(0, 100, ease.At(float64(at+frame/2)/float64(d)))
		v.AnimateTo(-50, d, anim.Linear)
		if v.Value() != from || v.Target() != -50 {
			t.Fatalf("retargeting moved the value: %v", v.Value())
		}
		ft := r.step(t)
		want := anim.Lerp(mid, -50, float64(ft-(at+frame/2))/float64(d))
		if v.Value() != want {
			t.Fatalf("after retarget %v, want %v", v.Value(), want)
		}
	})

	t.Run("spring keeps velocity", func(t *testing.T) {
		r := newRig()
		v := r.tl.NewValue(0, nil)
		v.SpringTo(1, sp)
		for range 4 {
			r.step(t)
		}
		x4, v4 := v.Value(), v.Velocity()
		v.SpringTo(0, sp)
		r.step(t)
		x, vel := sp.Motion(x4, 0, v4).At(frame)
		if v.Value() != x || v.Velocity() != vel || v4 <= 0 {
			t.Fatalf("after retarget %v %v, want %v %v (v4 %v)", v.Value(), v.Velocity(), x, vel, v4)
		}
	})

	t.Run("tween to spring", func(t *testing.T) {
		r := newRig()
		v := r.tl.NewValue(0, nil)
		v.AnimateTo(100, d, ease)
		r.step(t)
		r.step(t)
		p := float64(r.clk.Now()) / float64(d)
		x0, v0 := anim.Lerp(0, 100, ease.At(p)), 100*ease.Slope(p)/d.Seconds()
		v.SpringTo(100, sp) // the tween's target: nothing changes
		v.SpringTo(50, sp)
		r.step(t)
		x, _ := sp.Motion(x0, 50, v0).At(frame)
		if v.Value() != x {
			t.Fatalf("after retarget %v, want %v", v.Value(), x)
		}
	})

	t.Run("inside a tick", func(t *testing.T) {
		r := newRig()
		var b *anim.Value
		a := r.tl.NewValue(0, func(x float64) {
			if x >= 0.5 && !b.Running() && b.Value() == 0 {
				b.AnimateTo(1, d, anim.Linear) // starts at the frame time
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
		if want := float64(ft-started) / float64(d); b.Value() != want {
			t.Fatalf("b %v, want %v", b.Value(), want)
		}
	})
}

func TestSetStopAndIdle(t *testing.T) {
	r := newRig()
	r.tl.Tick(0) // nothing runs: no request
	if r.clk.Requested() {
		t.Fatal("an idle Tick requested a frame")
	}
	var got log
	v := r.tl.NewValue(1, got.fn)
	v.AnimateTo(1, time.Second, anim.Linear) // already there
	v.SpringTo(1, anim.Spring(theme.Sequoia.Animation.ControlSpring))
	v.Stop() // not running: nothing
	v.Set(1)
	if r.clk.Requested() || v.Running() || len(got) != 0 {
		t.Fatalf("no-op calls: requested %v, running %v, changes %v", r.clk.Requested(), v.Running(), got)
	}
	v.Set(2)
	if v.Value() != 2 || len(got) != 1 || got[0] != 2 || r.clk.Requested() {
		t.Fatalf("Set: %v, changes %v", v.Value(), got)
	}
	v.AnimateTo(3, 0, anim.Linear) // no duration: a jump
	if v.Value() != 3 || v.Running() || r.clk.Requested() {
		t.Fatalf("zero duration: %v running %v", v.Value(), v.Running())
	}

	v.AnimateTo(10, time.Second, anim.Linear)
	r.step(t)
	at := v.Value()
	v.Stop()
	if v.Value() != at || v.Velocity() != 0 || v.Running() || r.tl.Running() != 0 {
		t.Fatalf("Stop: %v %v", v.Value(), v.Velocity())
	}
	if r.clk.Advance(time.Second) != 1 { // the frame requested before Stop runs once, idle
		t.Fatal("the pending frame did not run")
	}
	if r.clk.Requested() || v.Value() != at {
		t.Fatal("a stopped value kept the clock running")
	}

	v.AnimateTo(20, time.Second, anim.Linear)
	r.step(t)
	v.Set(-1)
	if v.Value() != -1 || v.Running() || v.Target() != -1 {
		t.Fatalf("Set while running: %v", v.Value())
	}
	r.clk.Advance(time.Second)
	if r.clk.Requested() || v.Value() != -1 {
		t.Fatal("Set did not stop the animation")
	}
}

func TestChangeFuncEditsTimeline(t *testing.T) {
	r := newRig()
	var a, b, c *anim.Value
	stops := 0
	a = r.tl.NewValue(0, func(float64) {
		if b.Running() {
			b.Stop() // stop a value later in this tick
			stops++
		}
		a.Stop() // and itself
	})
	b = r.tl.NewValue(0, nil)
	c = r.tl.NewValue(0, func(x float64) {
		if x < 1 {
			c.AnimateTo(5, frame, anim.Linear) // retarget itself in its change func
		}
	})
	a.AnimateTo(1, time.Second, anim.Linear)
	b.AnimateTo(1, time.Second, anim.Linear)
	c.AnimateTo(1, 10*frame, anim.Linear)
	r.step(t)
	if a.Running() || b.Running() || b.Value() != 0 || stops != 1 {
		t.Fatalf("a running %v, b running %v at %v", a.Running(), b.Running(), b.Value())
	}
	if !c.Running() || c.Target() != 5 || r.tl.Running() != 1 {
		t.Fatalf("c running %v target %v, timeline %d", c.Running(), c.Target(), r.tl.Running())
	}
	r.step(t)
	if c.Value() != 5 || c.Running() || r.clk.Requested() {
		t.Fatalf("c %v running %v", c.Value(), c.Running())
	}

	// A stopped value restarted in the same tick stays listed once.
	var d *anim.Value
	d = r.tl.NewValue(0, func(x float64) {
		if x < 1 {
			d.Stop()
			d.AnimateTo(2, 2*frame, anim.Linear)
		}
	})
	d.AnimateTo(1, 4*frame, anim.Linear)
	r.step(t)
	if !d.Running() || r.tl.Running() != 1 {
		t.Fatalf("restarted value: running %v, timeline %d", d.Running(), r.tl.Running())
	}
	r.clk.Advance(time.Second)
	if d.Value() != 2 || r.tl.Running() != 0 || r.clk.Requested() {
		t.Fatalf("restarted value ended at %v, timeline %d", d.Value(), r.tl.Running())
	}
}

func TestReduceMotion(t *testing.T) {
	r := newRig()
	sp := anim.Spring(theme.Sequoia.Animation.ControlSpring)
	var got log
	v := r.tl.NewValue(0, got.fn)
	w := r.tl.NewValue(0, nil)
	v.AnimateTo(1, time.Second, anim.Linear)
	w.SpringTo(-1, sp)
	r.step(t)
	r.tl.SetReduceMotion(true)
	r.tl.SetReduceMotion(true) // no change
	if !r.tl.ReduceMotion() || v.Value() != 1 || w.Value() != -1 || v.Running() || w.Running() || w.Velocity() != 0 {
		t.Fatalf("turning it on: %v %v", v.Value(), w.Value())
	}
	r.clk.Advance(time.Second) // the frame requested before runs, idle
	if r.clk.Requested() {
		t.Fatal("running values left after Reduce Motion")
	}

	n := len(got)
	v.AnimateTo(5, time.Second, anim.Curve(theme.Sequoia.Animation.EaseInOut))
	w.SpringTo(7, sp)
	if v.Value() != 5 || w.Value() != 7 || v.Running() || w.Running() || r.clk.Requested() || len(got) != n+1 {
		t.Fatalf("with Reduce Motion: %v %v, requested %v", v.Value(), w.Value(), r.clk.Requested())
	}

	r.tl.SetReduceMotion(false)
	v.AnimateTo(0, time.Second, anim.Linear)
	if !v.Running() || !r.clk.Requested() {
		t.Fatal("animations do not run again after Reduce Motion")
	}
}

func TestTimelinePanics(t *testing.T) {
	mustPanicWith(t, "nil clock", func() { anim.NewTimeline(nil) })
	r := newRig()
	mustPanicWith(t, "not finite", func() { r.tl.NewValue(math.NaN(), nil) })
	v := r.tl.NewValue(0, nil)
	mustPanicWith(t, "not finite", func() { v.Set(math.Inf(1)) })
	mustPanicWith(t, "not finite", func() { v.AnimateTo(math.NaN(), time.Second, anim.Linear) })
	mustPanicWith(t, "not finite", func() { v.SpringTo(math.Inf(-1), anim.Spring(theme.Sequoia.Animation.ControlSpring)) })
	mustPanicWith(t, "positive", func() { v.SpringTo(1, anim.Spring{Mass: 1, Stiffness: 0, Damping: 1}) })
	r.tl.SetReduceMotion(true)
	mustPanicWith(t, "positive", func() { v.SpringTo(1, anim.Spring{}) }) // validated even when it jumps
	r.tl.SetReduceMotion(false)
	if v.Running() || v.Value() != 0 {
		t.Fatal("a panicking call changed the value")
	}

	// Tick inside Tick panics; a panicking change func leaves the timeline
	// usable.
	w := r.tl.NewValue(0, func(float64) { r.tl.Tick(0) })
	w.AnimateTo(1, time.Second, anim.Linear)
	mustPanicWith(t, "inside a Tick", func() { r.clk.Advance(frame) })
	w.Set(0)
	u := r.tl.NewValue(0, nil)
	u.AnimateTo(1, frame, anim.Linear)
	r.clk.Advance(time.Second)
	if u.Value() != 1 || r.tl.Running() != 0 {
		t.Fatalf("after a panic: %v, timeline %d", u.Value(), r.tl.Running())
	}
}

func TestCurveSlope(t *testing.T) {
	for name, c := range curves() {
		for i := 1; i < 100; i++ {
			x := float64(i) / 100
			const h = 1e-6
			num := (c.At(x+h) - c.At(x-h)) / (2 * h)
			if got := c.Slope(x); math.Abs(got-num) > 1e-4*max(1, math.Abs(num)) {
				// Flat-x curves are steep and badly conditioned in places.
				if c.Slope(x) != 0 || math.Abs(num) < 1e3 {
					t.Errorf("%s: Slope(%v) = %v, numeric %v", name, x, got, num)
				}
			}
		}
	}
	if s := anim.Linear.Slope(0.3); s != 1 {
		t.Errorf("Linear.Slope = %v", s)
	}
	ease := anim.Curve(theme.Sequoia.Animation.EaseOut)
	if ease.Slope(-1) != ease.Slope(0) || ease.Slope(math.NaN()) != ease.Slope(0) || ease.Slope(2) != ease.Slope(1) {
		t.Error("Slope does not clamp t")
	}
	if s := (anim.Curve{X1: 0, Y1: 1, X2: 0.5, Y2: 1}).Slope(0); s != 0 {
		t.Errorf("unbounded slope at a flat end = %v, want 0", s)
	}
}
