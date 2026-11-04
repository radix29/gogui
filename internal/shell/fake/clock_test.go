package fake

import (
	"testing"
	"time"
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
}
