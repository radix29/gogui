package anim_test

import (
	"math"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/aqua/theme"
)

// TestToleranceSettles (review P11): a spring settles once it can no
// longer move farther than its value's tolerance from its target, not once
// it is also slower than an absolute epsilon. The rubber band's spring
// from 50 pt past an edge used to run 70 frames, about twice as long as it
// visibly moves.
func TestToleranceSettles(t *testing.T) {
	sp := anim.Spring(theme.Sequoia.Animation.RubberBandSpring)
	for _, tol := range []float64{anim.DefaultTolerance, anim.AlphaTolerance, anim.PointTolerance} {
		r := newRig()
		v := r.tl.NewValue(50, nil).SetTolerance(tol)
		v.SpringTo(0, sp)
		m := sp.Motion(50, 0, 0)
		frames := 0
		for v.Running() {
			ft := r.step(t)
			frames++
			if m.Settled(ft, tol) == v.Running() {
				t.Fatalf("tol %v frame %d: settled %v, running %v", tol, frames, m.Settled(ft, tol), v.Running())
			}
			if x, _ := m.At(ft); math.Abs(v.Value()-x) > tol {
				t.Fatalf("tol %v frame %d: shown %v, motion at %v", tol, frames, v.Value(), x)
			}
		}
		if v.Value() != 0 || v.Velocity() != 0 || r.clk.Requested() {
			t.Errorf("tol %v: stopped at %v %v", tol, v.Value(), v.Velocity())
		}
		if tol == anim.PointTolerance && frames > 45 {
			t.Errorf("point tolerance: %d frames, want at most 45 (70 with the absolute epsilon)", frames)
		}
		t.Logf("tol %v: %d frames", tol, frames)
	}
}

// TestToleranceSkipsInvisibleFrames (review P11): a frame that moves a
// value by less than its tolerance from what was last shown does not call
// the change func and leaves Value; the end is shown exactly.
func TestToleranceSkipsInvisibleFrames(t *testing.T) {
	r := newRig()
	var got log
	v := r.tl.NewValue(0, got.fn).SetTolerance(1)
	v.AnimateTo(3, 20*frame, anim.Linear) // 0.15 per frame
	shown := 0.0
	for v.Running() {
		r.step(t)
		if v.Value() != shown && math.Abs(v.Value()-shown) < 1 && v.Running() {
			t.Fatalf("shown %v after %v: a move below the tolerance", v.Value(), shown)
		}
		if v.Running() && v.Velocity() == 0 {
			t.Fatal("the velocity does not follow the skipped frames")
		}
		shown = v.Value()
	}
	if len(got) > 4 || got[len(got)-1] != 3 || v.Value() != 3 {
		t.Errorf("changes %v, want a few ending at 3", got)
	}
}

func TestSetTolerancePanics(t *testing.T) {
	v := newRig().tl.NewValue(0, nil)
	if v.Tolerance() != anim.DefaultTolerance {
		t.Errorf("default tolerance %v", v.Tolerance())
	}
	for _, tol := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		mustPanicWith(t, "tolerance that is not positive", func() { v.SetTolerance(tol) })
	}
}

// TestCurveAtSlope: AtSlope is At and Slope from one solve.
func TestCurveAtSlope(t *testing.T) {
	for name, c := range curves() {
		for _, p := range []float64{math.NaN(), -1, 0, 1e-9, 0.1, 0.25, 0.5, 0.77, 1 - 1e-9, 1, 2} {
			y, s := c.AtSlope(p)
			if y != c.At(p) || s != c.Slope(p) {
				t.Errorf("%s at %v: AtSlope %v %v, At %v Slope %v", name, p, y, s, c.At(p), c.Slope(p))
			}
		}
	}
}

// BenchmarkTweenFrame: one frame of a running tween.
func BenchmarkTweenFrame(b *testing.B) {
	r := newRig()
	v := r.tl.NewValue(0, nil)
	ease := anim.Curve(theme.Sequoia.Animation.EaseInOut)
	ft := time.Duration(0)
	for b.Loop() {
		if !v.Running() {
			v.AnimateTo(100-v.Value(), time.Hour, ease)
		}
		ft += frame
		r.tl.Tick(ft)
	}
}
