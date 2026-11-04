package scroll_test

import (
	"math"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua/scroll"
)

const (
	window = 80 * time.Millisecond
	frame  = 8 * time.Millisecond // a precision touchpad's report rate
	gap    = 100 * time.Millisecond
)

func tracker() *scroll.VelocityTracker {
	return &scroll.VelocityTracker{Window: window, EndGap: scroll.DefaultEndGap}
}

// burst feeds n precise events every frame from t0 with the deltas
// delta(i) and returns the time of the last one.
func burst(vt *scroll.VelocityTracker, t0 time.Duration, n int, delta func(i int) float64) time.Duration {
	t := t0
	for i := 0; i < n; i++ {
		t = t0 + time.Duration(i)*frame
		vt.Add(t, delta(i))
	}
	return t
}

// TestVelocitySteady: a steady stream gives exactly delta/frame, the
// Win32-style zero-delta PhaseEnded 100 ms later keeps it, and the sign
// follows the direction.
func TestVelocitySteady(t *testing.T) {
	for _, d := range []float64{4, -4, 0.5, 30} {
		vt := tracker()
		last := burst(vt, time.Second, 30, func(int) float64 { return d })
		want := d / frame.Seconds()
		if got := vt.Velocity(); !near(got, want, 1e-12) {
			t.Errorf("d=%v: Velocity = %v, want %v", d, got, want)
		}
		if got := vt.End(last + gap); !near(got, want, 1e-12) {
			t.Errorf("d=%v: End after the Win32 gap = %v, want %v", d, got, want)
		}
	}
}

// TestVelocityAccelerating: the velocity is that of the last window, not
// the gesture's average.
func TestVelocityAccelerating(t *testing.T) {
	vt := tracker()
	last := burst(vt, 0, 40, func(i int) float64 { return float64(i) })
	// The samples within 80 ms of the last (i = 29…39) span 80 ms; the
	// deltas after the anchor are 30…39.
	var dist float64
	for i := 30; i < 40; i++ {
		dist += float64(i)
	}
	want := dist / window.Seconds()
	if got := vt.End(last + gap); !near(got, want, 1e-12) {
		t.Errorf("End = %v, want %v", got, want)
	}
	if avg := 39 * 40 / 2 / (39 * frame).Seconds(); !(want > avg) {
		t.Errorf("recent velocity %v not above the average %v", want, avg)
	}
	// Slowing down: the recent small deltas win.
	vt.Reset()
	last = burst(vt, 0, 40, func(i int) float64 { return float64(40 - i) })
	if got := vt.End(last + gap); !(got > 0 && got < 10/frame.Seconds()) {
		t.Errorf("slowing: End = %v", got)
	}
}

// TestVelocityStopped: fingers held still before the lift give no
// momentum, whether the OS reports the rest as zero deltas (Win32 keeps
// sending events while the fingers touch) or as silence until the end.
func TestVelocityStopped(t *testing.T) {
	// Zero deltas for a whole window, then the late PhaseEnded.
	vt := tracker()
	last := burst(vt, 0, 20, func(i int) float64 {
		if i < 8 {
			return 10
		}
		return 0
	})
	if got := vt.End(last + gap); got != 0 {
		t.Errorf("zero deltas then ended: End = %v, want 0", got)
	}
	// Moving, then no events for longer than EndGap, then the end.
	vt.Reset()
	last = burst(vt, 0, 20, func(int) float64 { return 10 })
	if got := vt.End(last + scroll.DefaultEndGap + time.Millisecond); got != 0 {
		t.Errorf("silence then ended: End = %v, want 0", got)
	}
	// Exactly at EndGap still counts as the OS delay.
	if got := vt.End(last + scroll.DefaultEndGap); got == 0 {
		t.Error("End at EndGap = 0, want the velocity")
	}
}

// TestVelocityEdgeCases: too few samples, equal and out-of-order
// timestamps, the zero value, Reset.
func TestVelocityEdgeCases(t *testing.T) {
	var zero scroll.VelocityTracker
	zero.Add(0, 5)
	zero.Add(frame, 5)
	if zero.Velocity() != 0 || zero.End(frame) != 0 {
		t.Error("the zero value (no window) reports a velocity")
	}
	vt := tracker()
	if vt.Velocity() != 0 || vt.End(0) != 0 {
		t.Error("no samples: velocity not 0")
	}
	vt.Add(time.Second, 7)
	if vt.Velocity() != 0 || vt.End(time.Second) != 0 {
		t.Error("one sample: velocity not 0")
	}
	// Equal and earlier timestamps merge into the latest sample: no
	// division by zero, the distance kept.
	vt.Add(time.Second, 3)
	vt.Add(time.Second-time.Millisecond, 2)
	if v := vt.Velocity(); v != 0 || math.IsNaN(v) {
		t.Errorf("merged samples: Velocity = %v, want 0", v)
	}
	vt.Add(time.Second+10*time.Millisecond, 10)
	if got, want := vt.Velocity(), 10/0.01; !near(got, want, 1e-12) {
		t.Errorf("after merging: Velocity = %v, want %v", got, want)
	}
	// Samples older than the window are dropped: a fast start then a slow
	// steady tail gives the tail's speed.
	vt.Reset()
	vt.Add(0, 0)
	vt.Add(frame, 1000)
	last := burst(vt, time.Second, 20, func(int) float64 { return 1 })
	if got, want := vt.End(last+gap), 1/frame.Seconds(); !near(got, want, 1e-12) {
		t.Errorf("after the window moved on: End = %v, want %v", got, want)
	}
	// Reset forgets everything but keeps Window.
	vt.Reset()
	if vt.Velocity() != 0 || vt.Window != window {
		t.Error("Reset kept samples or dropped the window")
	}
	// A negative window reports 0.
	neg := &scroll.VelocityTracker{Window: -time.Second}
	neg.Add(0, 1)
	neg.Add(frame, 1)
	if neg.Velocity() != 0 {
		t.Error("negative window: velocity not 0")
	}
}
