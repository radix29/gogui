package fake

import (
	"slices"
	"testing"
	"time"
)

// TestAfterOrder: timers fire during Advance in deadline order (equal
// deadlines in creation order), with Now at each deadline, never inside
// After, Step or Flush; d <= 0 is due at once.
func TestAfterOrder(t *testing.T) {
	s := started(t)
	c := s.FakeClock()
	var got []string
	at := func(name string) func() {
		return func() { got = append(got, name+"@"+c.Now().String()) }
	}
	s.After(30*time.Millisecond, at("c"))
	s.After(10*time.Millisecond, at("a"))
	s.After(30*time.Millisecond, at("d")) // same deadline as c: after it
	s.After(-time.Second, at("now1"))
	s.After(0, at("now2"))
	s.After(20*time.Millisecond, at("b"))
	if want := []time.Duration{0, 0, 10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond, 30 * time.Millisecond}; !slices.Equal(s.Timers(), want) {
		t.Fatalf("Timers() = %v, want %v", s.Timers(), want)
	}
	s.Step()
	c.RequestFrame()
	c.Flush()
	if len(got) != 0 {
		t.Fatalf("timers fired outside Advance: %v", got)
	}
	c.Advance(25 * time.Millisecond)
	want := []string{"now1@0s", "now2@0s", "a@10ms", "b@20ms"}
	if !slices.Equal(got, want) {
		t.Fatalf("fired %v, want %v", got, want)
	}
	if c.Now() != 25*time.Millisecond {
		t.Errorf("Now = %v, want 25ms", c.Now())
	}
	got = nil
	c.Advance(5 * time.Millisecond) // exactly at the deadline
	if want := []string{"c@30ms", "d@30ms"}; !slices.Equal(got, want) {
		t.Errorf("fired %v, want %v", got, want)
	}
	if len(s.Timers()) != 0 {
		t.Errorf("timers left: %v", s.Timers())
	}
}

// TestAfterFrames: timers and frames interleave on the Now timeline; a
// timer due at a pending frame's time fires before that frame, and a frame
// a timer requests runs at the next frame boundary after it.
func TestAfterFrames(t *testing.T) {
	s := started(t)
	c := s.FakeClock()
	var log []string
	c.SetFrameFunc(func(ft time.Duration) { log = append(log, "frame@"+ft.String()) })
	c.RequestFrame()
	s.After(FrameInterval, func() { log = append(log, "t1@"+c.Now().String()) })
	s.After(FrameInterval+time.Millisecond, func() {
		log = append(log, "t2@"+c.Now().String())
		c.RequestFrame()
		s.After(0, func() { log = append(log, "t3@"+c.Now().String()) }) // due now: the next round, when Now moves
	})
	if n := c.Advance(time.Second); n != 2 {
		t.Errorf("Advance ran %d frames, want 2", n)
	}
	f1, f2 := FrameInterval.String(), (2 * FrameInterval).String()
	t2 := (FrameInterval + time.Millisecond).String()
	want := []string{"t1@" + f1, "frame@" + f1, "t2@" + t2, "t3@" + f2, "frame@" + f2}
	if !slices.Equal(log, want) {
		t.Errorf("log %v, want %v", log, want)
	}
}

// TestAfterCancel: cancel stops a pending timer; after the timer fired, a
// second time, or for a timer cancelled from inside another timer's fn (or
// its own), it does nothing more.
func TestAfterCancel(t *testing.T) {
	s := started(t)
	c := s.FakeClock()
	var got []string
	cancelA := s.After(10*time.Millisecond, func() { got = append(got, "a") })
	cancelA()
	cancelA()
	var cancelC, cancelSelf func()
	s.After(20*time.Millisecond, func() {
		got = append(got, "b")
		cancelC()
		cancelSelf()
	})
	cancelSelf = s.After(20*time.Millisecond, func() { got = append(got, "self") }) // after b: b cancels it
	cancelC = s.After(30*time.Millisecond, func() { got = append(got, "c") })
	cancelD := s.After(40*time.Millisecond, func() { got = append(got, "d") })
	var cancelE func()
	cancelE = s.After(50*time.Millisecond, func() {
		got = append(got, "e")
		cancelE() // its own: fired already
	})
	c.Advance(time.Second)
	cancelD() // after it fired
	cancelD()
	if want := []string{"b", "d", "e"}; !slices.Equal(got, want) {
		t.Errorf("fired %v, want %v", got, want)
	}
	if len(s.Timers()) != 0 {
		t.Errorf("timers left: %v", s.Timers())
	}
}

// TestAfterPanic: a panic in fn leaves Advance; the timer is gone and the
// clock usable.
func TestAfterPanic(t *testing.T) {
	s := started(t)
	c := s.FakeClock()
	s.After(time.Millisecond, func() { panic("boom") })
	fired := false
	s.After(2*time.Millisecond, func() { fired = true })
	mustPanic(t, "boom", func() { c.Advance(time.Second) })
	if len(s.Timers()) != 1 {
		t.Fatalf("timers %v, want the second one left", s.Timers())
	}
	c.Advance(time.Second)
	if !fired {
		t.Error("the second timer did not fire after the panic")
	}
}

// TestAfterRearmZero (review B9): a fn that re-arms itself with After(0)
// fires once per round, and a round waits for Now to move, so Advance
// ends: the re-armed timer fires at the next deadline, frame or end time.
// The one still pending fires first in the next Advance.
func TestAfterRearmZero(t *testing.T) {
	s := started(t)
	c := s.FakeClock()
	var at []time.Duration
	var tick func()
	tick = func() {
		at = append(at, c.Now())
		s.After(0, tick)
	}
	const ms = time.Millisecond
	s.After(5*ms, tick)
	c.Advance(10 * ms)
	if want := []time.Duration{5 * ms, 10 * ms}; !slices.Equal(at, want) {
		t.Fatalf("fired at %v, want %v", at, want)
	}
	if got := s.Timers(); !slices.Equal(got, []time.Duration{10 * ms}) {
		t.Fatalf("pending %v, want the re-armed timer at 10ms", got)
	}

	// With a frame function that keeps requesting: one round before each
	// frame, at the frame's time.
	at = nil
	c.SetFrameFunc(func(time.Duration) { c.RequestFrame() })
	c.RequestFrame()
	if n := c.Advance(3 * FrameInterval); n != 3 {
		t.Errorf("Advance ran %d frames, want 3", n)
	}
	want := []time.Duration{10 * ms, FrameInterval, 2 * FrameInterval, 3 * FrameInterval, 10*ms + 3*FrameInterval}
	if !slices.Equal(at, want) {
		t.Errorf("fired at %v, want %v", at, want)
	}
}

// TestAfterQuit: pending timers are dropped at Quit; After after Quit
// makes none, and its cancel does nothing.
func TestAfterQuit(t *testing.T) {
	s := started(t)
	c := s.FakeClock()
	fired := 0
	cancel := s.After(time.Millisecond, func() { fired++ })
	s.Quit()
	if len(s.Timers()) != 0 {
		t.Errorf("timers left after Quit: %v", s.Timers())
	}
	late := s.After(0, func() { fired++ })
	c.Advance(time.Second)
	cancel()
	late()
	if fired != 0 {
		t.Errorf("%d timers fired after Quit", fired)
	}
}

// TestAfterChecks: After needs Ready and the UI goroutine and a non-nil
// fn; its cancel needs the UI goroutine; Advance and Flush may not be
// called from inside a timer.
func TestAfterChecks(t *testing.T) {
	s := New()
	mustPanic(t, "off the UI thread", func() { s.After(0, func() {}) })
	bound := New()
	bound.q.Bind()
	mustPanic(t, "before Handler.Ready", func() { bound.After(0, func() {}) })

	s = started(t)
	mustPanic(t, "nil func", func() { s.After(0, nil) })
	cancel := s.After(time.Second, func() {})
	done := make(chan any)
	go func() {
		defer func() { done <- recover() }()
		cancel()
	}()
	if r := <-done; r == nil {
		t.Error("cancel off the UI goroutine did not panic")
	}
	c := s.FakeClock()
	s.After(0, func() {
		mustPanic(t, "Advance called from inside a timer", func() { c.Advance(0) })
		mustPanic(t, "Flush called from inside a timer", func() { c.Flush() })
	})
	c.Advance(0)
}

// TestAfterStress: many timers with random deadlines, some cancelled, fire
// in (deadline, creation) order however Advance slices the time.
func TestAfterStress(t *testing.T) {
	s := started(t)
	c := s.FakeClock()
	type want struct {
		at  time.Duration
		seq int
	}
	var fired, expect []want
	cancels := map[int]func(){}
	for i := range 500 {
		d := time.Duration((i*7919)%997) * time.Millisecond
		at := c.Now() + d
		cancels[i] = s.After(d, func() {
			if c.Now() != at {
				t.Errorf("timer %d at %v, want %v", i, c.Now(), at)
			}
			fired = append(fired, want{at, i})
		})
		if i%3 == 0 {
			continue // cancelled below
		}
		expect = append(expect, want{at, i})
	}
	for i, cancel := range cancels {
		if i%3 == 0 {
			cancel()
		}
	}
	slices.SortStableFunc(expect, func(a, b want) int { return int(a.at - b.at) })
	for c.Now() < 2*time.Second {
		c.Advance(time.Duration(len(fired)%13+1) * time.Millisecond)
	}
	if !slices.Equal(fired, expect) {
		t.Errorf("fired %d timers out of order or wrong (want %d)", len(fired), len(expect))
	}
}
