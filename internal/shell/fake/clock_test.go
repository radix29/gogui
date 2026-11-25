package fake

import (
	"testing"
	"time"

	"github.com/radix29/gogui/internal/shell"
)

func TestClockStandalone(t *testing.T) {
	c := NewClock()
	var times []time.Duration
	c.SetFrameFunc(func(ft time.Duration) { times = append(times, ft) })
	if c.Now() != 0 || c.Requested() || c.Frames() != 0 {
		t.Fatal("new clock not at rest")
	}
	// No request, no frames.
	if n := c.Advance(time.Second); n != 0 || len(times) != 0 || c.Now() != time.Second {
		t.Fatalf("idle Advance ran %d frames, now %v", n, c.Now())
	}
	// Requests are one-shot and coalesced.
	c.RequestFrame()
	c.RequestFrame()
	if !c.Requested() {
		t.Fatal("not requested")
	}
	if n := c.Advance(100 * time.Millisecond); n != 1 {
		t.Fatalf("ran %d frames, want 1", n)
	}
	if want := 61 * FrameInterval; times[0] != want {
		t.Fatalf("frame time %v, want %v (the next frame boundary)", times[0], want)
	}
	if c.Now() != 1100*time.Millisecond || c.Requested() {
		t.Fatalf("now %v requested %v", c.Now(), c.Requested())
	}
}

func TestClockAnimation(t *testing.T) {
	c := NewClock()
	var times []time.Duration
	c.SetFrameFunc(func(ft time.Duration) {
		times = append(times, ft)
		if len(times) < 5 {
			c.RequestFrame() // keep running
		}
	})
	c.RequestFrame()
	if n := c.Advance(time.Second); n != 5 {
		t.Fatalf("ran %d frames, want 5", n)
	}
	for i, ft := range times {
		if want := time.Duration(i+1) * FrameInterval; ft != want {
			t.Errorf("frame %d at %v, want %v", i, ft, want)
		}
	}
	if c.Frames() != 5 || c.Now() != time.Second {
		t.Errorf("frames %d now %v", c.Frames(), c.Now())
	}
	// A short Advance before the boundary runs nothing; the frame stays
	// requested.
	times = nil
	c.SetFrameFunc(func(ft time.Duration) { times = append(times, ft) })
	c.RequestFrame()
	if n := c.Advance(FrameInterval / 4); n != 0 || !c.Requested() {
		t.Fatalf("early Advance ran %d", n)
	}
	if n := c.Advance(FrameInterval); n != 1 || times[0] != 61*FrameInterval {
		t.Fatalf("ran %d at %v", n, times)
	}
	// Exactly on a boundary: the frame runs at the next one.
	c = NewClock()
	c.Advance(FrameInterval)
	c.RequestFrame()
	c.SetFrameFunc(func(ft time.Duration) { times = append(times[:0], ft) })
	c.Advance(FrameInterval)
	if times[0] != 2*FrameInterval {
		t.Errorf("frame at %v, want %v", times[0], 2*FrameInterval)
	}
}

func TestClockFlush(t *testing.T) {
	c := NewClock()
	c.Advance(time.Second / 7)
	if c.Flush() {
		t.Fatal("Flush ran without a request")
	}
	var times []time.Duration
	c.SetFrameFunc(func(ft time.Duration) {
		times = append(times, ft)
		c.RequestFrame()
	})
	c.RequestFrame()
	if !c.Flush() || len(times) != 1 || times[0] != time.Second/7 {
		t.Fatalf("Flush: %v", times)
	}
	if !c.Requested() || c.Now() != time.Second/7 {
		t.Fatal("the request from the frame must wait, time must not move")
	}
	c.SetFrameFunc(nil)
	if !c.Flush() || c.Frames() != 2 {
		t.Fatal("Flush without a frame function")
	}
}

func TestClockMisuse(t *testing.T) {
	c := NewClock()
	mustPanic(t, "negative", func() { c.Advance(-1) })
	c.SetFrameFunc(func(time.Duration) { c.Advance(time.Second) })
	c.RequestFrame()
	mustPanic(t, "inside a frame", func() { c.Flush() })
	c.SetFrameFunc(func(time.Duration) { c.Flush() })
	c.RequestFrame()
	mustPanic(t, "inside a frame", func() { c.Advance(time.Second) })
	// A panicking frame leaves the clock usable.
	c.SetFrameFunc(nil)
	c.RequestFrame()
	if !c.Flush() {
		t.Fatal("clock stuck after a panicking frame")
	}
	if c.inFn {
		t.Error("a panicking frame function left Invalidate deferred")
	}
}

// TestClockHidden: with open surfaces and none on screen (hidden, then
// minimized) an animation runs at HiddenInterval and its frame times keep
// moving; a timer that restores the surface ends the low rate at once,
// with the next frame on the 60 Hz grid; with no surface at all the clock
// keeps 60 Hz (S6.2b).
func TestClockHidden(t *testing.T) {
	s := started(t)
	c := s.FakeClock()
	var times []time.Duration
	c.SetFrameFunc(func(ft time.Duration) {
		times = append(times, ft)
		c.RequestFrame()
	})
	run := func(d time.Duration) []time.Duration {
		times = nil
		c.Advance(d)
		return times
	}
	steps := func(ts []time.Duration, want time.Duration) bool {
		for i := 1; i < len(ts); i++ {
			if ts[i]-ts[i-1] != want {
				return false
			}
		}
		return true
	}

	c.RequestFrame()
	if ts := run(time.Second); len(ts) != 60 || !steps(ts, FrameInterval) {
		t.Fatalf("no surface: %d frames in 1 s, want 60 at %v", len(ts), FrameInterval)
	}
	f := open(t, s, &win{}, shell.SurfaceConfig{Size: sz(200, 100)})
	g, _ := s.NewSurface(shell.SurfaceConfig{Size: sz(200, 100)}, &win{}) // never shown
	shown := run(time.Second)
	if ts := shown; len(ts) != 60 {
		t.Fatalf("shown: %d frames in 1 s, want 60", len(ts))
	}
	f.Minimize()
	ts := run(time.Second)
	if len(ts) != 4 || !steps(ts, HiddenInterval) || ts[0]-shown[59] != HiddenInterval {
		t.Fatalf("minimized: frames at %v, want 4 at %v", ts, HiddenInterval)
	}
	// A timer restores it between two low-rate frames: the next frame is
	// the grid's next, not the low rate's.
	last := ts[len(ts)-1]
	s.After(last+5*FrameInterval-c.Now(), func() { f.Show() })
	if ts := run(10 * FrameInterval); len(ts) != 5 || ts[0] != last+6*FrameInterval || !steps(ts, FrameInterval) {
		t.Fatalf("restored by a timer: frames at %v, want 5 from %v", ts, last+6*FrameInterval)
	}
	f.Close()
	if ts := run(time.Second); len(ts) != 4 { // g is open and not shown
		t.Fatalf("only a hidden surface: %d frames in 1 s, want 4", len(ts))
	}
	g.(*Surface).Close()
	if ts := run(time.Second); len(ts) != 60 {
		t.Fatalf("no surface left: %d frames in 1 s, want 60", len(ts))
	}
	c.SetFrameFunc(nil)
}

// TestClockFrameObserver: with an observer, each frame reports its
// timing after its paint pass: the frame function (here one that sleeps)
// and one paint per invalidated surface; a frame that paints nothing
// reports no surface; without the observer nothing more is reported.
func TestClockFrameObserver(t *testing.T) {
	s := started(t)
	c := s.FakeClock()
	f := open(t, s, &win{}, shell.SurfaceConfig{Size: sz(200, 100)})
	g := open(t, s, &win{}, shell.SurfaceConfig{Size: sz(100, 50)})
	c.Advance(FrameInterval) // the first paints
	var got []shell.FrameTiming
	c.SetFrameObserver(func(ft shell.FrameTiming) { got = append(got, ft) })
	sleep := false
	c.SetFrameFunc(func(time.Duration) {
		if sleep {
			time.Sleep(2 * time.Millisecond)
		}
	})
	f.Invalidate()
	g.Invalidate()
	sleep = true
	c.Advance(FrameInterval)
	if len(got) != 1 || got[0].Time != c.Now() || got[0].Surfaces != 2 ||
		got[0].FrameFunc < 2*time.Millisecond || got[0].Paint < 0 || got[0].Submit < 0 || got[0].Present != 0 {
		t.Fatalf("a frame painting two surfaces: %+v", got)
	}
	sleep = false
	got = nil
	c.RequestFrame()
	c.Advance(FrameInterval)
	if len(got) != 1 || got[0].Surfaces != 0 || got[0].Paint != 0 {
		t.Fatalf("a frame painting nothing: %+v", got)
	}
	c.SetFrameObserver(nil)
	got = nil
	f.Invalidate()
	c.Advance(FrameInterval)
	if len(got) != 0 {
		t.Fatalf("without the observer: %+v", got)
	}
	c.SetFrameFunc(nil)
}

// TestClockPresentObserver: each paint of a frame is shown at once, on the
// frame's target (S6.2d); a paint outside a frame reports nothing.
func TestClockPresentObserver(t *testing.T) {
	s := started(t)
	c := s.FakeClock()
	f := open(t, s, &win{}, shell.SurfaceConfig{Size: sz(200, 100)})
	g := open(t, s, &win{}, shell.SurfaceConfig{Size: sz(100, 50)})
	c.Advance(FrameInterval) // the first paints
	var got []shell.PresentTiming
	c.SetPresentObserver(func(pt shell.PresentTiming) { got = append(got, pt) })
	f.Invalidate()
	g.Invalidate()
	c.Advance(FrameInterval)
	want := shell.PresentTiming{Target: c.Now(), Shown: c.Now(), Period: FrameInterval,
		Refresh: uint64(c.Now() / FrameInterval)}
	if len(got) != 2 || got[0] != want || got[1] != want {
		t.Fatalf("a frame painting two surfaces: %+v, want twice %+v", got, want)
	}
	got = nil
	c.RequestFrame()
	c.Advance(FrameInterval)
	if len(got) != 0 {
		t.Fatalf("a frame painting nothing: %+v", got)
	}
	f.UserResize(sz(210, 110)) // paints at once, outside a frame
	if len(got) != 0 {
		t.Fatalf("a paint outside a frame: %+v", got)
	}
	c.Advance(time.Second)
	c.SetPresentObserver(nil)
	got = nil
	f.Invalidate()
	c.Advance(FrameInterval)
	if len(got) != 0 {
		t.Fatalf("without the observer: %+v", got)
	}
}
