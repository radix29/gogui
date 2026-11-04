package fake

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/shell"
)

// TestTimedSwipe: the deltas come interval apart on the clock, frames due
// on the way run, and PhaseEnded follows SwipeEndDelay after the last one.
func TestTimedSwipe(t *testing.T) {
	s := started(t)
	var w win
	var times []string
	w.event = func(ev shell.Event) bool {
		if e, ok := ev.(shell.ScrollEvent); ok {
			times = append(times, fmt.Sprint(e.Time))
		}
		return false
	}
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(100, 100)})
	clk := s.FakeClock()
	clk.Advance(time.Second)
	frames := clk.Frames()
	clk.RequestFrame() // a running animation: frames run during the swipe
	f.TimedSwipe(pt(10, 10), 10*time.Millisecond, pt(0, 5), pt(0, 6), pt(0, 7))
	want := "enter (10,10); move (10,10); " +
		"scroll (10,10) d=(0,5) precise began; scroll (10,10) d=(0,6) precise changed; " +
		"scroll (10,10) d=(0,7) precise changed; scroll (10,10) d=(0,0) precise ended"
	if got := w.take(); !strings.HasSuffix(got, "scroll (10,10) d=(0,0) precise ended") ||
		strings.ReplaceAll(got, "paint; ", "") != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if got, want := strings.Join(times, " "), "1s 1.01s 1.02s 1.12s"; got != want {
		t.Errorf("times %s, want %s", got, want)
	}
	if clk.Now() != 1120*time.Millisecond || clk.Frames() != frames+1 {
		t.Errorf("now %v, frames %d (want %d)", clk.Now(), clk.Frames(), frames+1)
	}
	f.TimedSwipe(pt(10, 10), time.Millisecond) // no deltas: nothing
	if got := w.take(); got != "" || clk.Now() != 1120*time.Millisecond {
		t.Errorf("empty swipe: %q at %v", got, clk.Now())
	}
	defer func() {
		if recover() == nil {
			t.Error("negative interval: no panic")
		}
	}()
	f.TimedSwipe(pt(10, 10), -1, pt(0, 1))
}

// TestTimedSwipeClosed: a frame that closes the surface ends the swipe.
func TestTimedSwipeClosed(t *testing.T) {
	s := started(t)
	var w win
	n := 0
	w.event = func(ev shell.Event) bool {
		if _, ok := ev.(shell.ScrollEvent); ok {
			n++
		}
		return false
	}
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(100, 100)})
	clk := s.FakeClock()
	clk.SetFrameFunc(func(time.Duration) { f.Close() })
	clk.RequestFrame()
	f.TimedSwipe(pt(10, 10), 20*time.Millisecond, pt(0, 5), pt(0, 6))
	if n != 1 {
		t.Errorf("%d scroll events after the close, want 1", n)
	}
}
