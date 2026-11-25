package gtk4

import (
	"slices"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

func TestTimeoutMS(t *testing.T) {
	for _, c := range []struct {
		d    time.Duration
		want uint32
	}{
		{-time.Second, 0},
		{0, 0},
		{time.Nanosecond, 1},
		{time.Millisecond, 1},
		{time.Millisecond + 1, 2},
		{530 * time.Millisecond, 530},
		{100 * 24 * time.Hour, 1<<32 - 1},
	} {
		if got := timeoutMS(c.d); got != c.want {
			t.Errorf("timeoutMS(%v) = %d, want %d", c.d, got, c.want)
		}
	}
}

// newAfterTest is newClockTest with the clock begun: After works, and the
// fake loop's timeouts are the After source's (no frame is requested).
func newAfterTest(t *testing.T) (*Shell, *fakeClockLoop) {
	t.Helper()
	s, c, l := newClockTest(t)
	c.begin(0)
	t.Cleanup(s.timers.stop) // before newClockTest's checks
	return s, l
}

// pendingMS returns the interval of the one pending timeout, failing the
// test unless exactly one is.
func pendingMS(t *testing.T, l *fakeClockLoop) uint32 {
	t.Helper()
	if len(l.timeouts) != 1 {
		t.Fatalf("%d timeouts pending, want 1", len(l.timeouts))
	}
	return l.timeouts[0].ms
}

// TestAfterGTKOrder: one timeout source for the earliest deadline; timers
// fire in (deadline, creation) order with Now at or past each deadline,
// never inside After; several due at once fire in one round.
func TestAfterGTKOrder(t *testing.T) {
	s, l := newAfterTest(t)
	var got []string
	at := func(name string, d time.Duration) {
		want := s.clock.Now() + max(d, 0)
		s.After(d, func() {
			if now := s.clock.Now(); now < want {
				t.Errorf("timer %s at %v, before its deadline %v", name, now, want)
			}
			got = append(got, name)
		})
	}
	at("c", 30*time.Millisecond)
	at("a", 10*time.Millisecond)
	at("d", 30*time.Millisecond) // same deadline as c: after it
	at("zero", -time.Second)
	at("b", 20*time.Millisecond)
	if len(got) != 0 {
		t.Fatal("After ran fn inside the call")
	}
	if ms := pendingMS(t, l); ms != 0 {
		t.Fatalf("source armed for %d ms, want 0 (the zero timer)", ms)
	}
	l.fireTimeout(t)
	if ms := pendingMS(t, l); ms != 10 {
		t.Errorf("source armed for %d ms after the zero timer, want 10", ms)
	}
	l.fireTimeout(t)
	l.now += 15_000 // the source fires late, at 25 ms: only b is due
	l.fireTimeout(t)
	l.fireTimeout(t) // c and d in one round
	if want := []string{"zero", "a", "b", "c", "d"}; !slices.Equal(got, want) {
		t.Errorf("fired %v, want %v", got, want)
	}
	if len(l.timeouts) != 0 || s.timers.id != 0 || s.timers.fires != 4 {
		t.Errorf("after the last: %d timeouts, id %d, %d fires (want 0, 0, 4)", len(l.timeouts), s.timers.id, s.timers.fires)
	}
}

// TestAfterGTKEarly: a source that fires before the first deadline (as
// GLib never does, but a clock step could) fires nothing and re-arms for
// the rest.
func TestAfterGTKEarly(t *testing.T) {
	s, l := newAfterTest(t)
	fired := 0
	s.After(50*time.Millisecond, func() { fired++ })
	src := l.timeouts[0]
	l.timeouts = nil
	l.now += 20_000
	if r := sourceFunc(uintptr(src.h)); r != gtk.SourceRemove {
		t.Error("the After source returned G_SOURCE_CONTINUE")
	}
	if fired != 0 || s.timers.early != 1 {
		t.Fatalf("early source: %d fired, %d early", fired, s.timers.early)
	}
	if ms := pendingMS(t, l); ms != 30 {
		t.Errorf("re-armed for %d ms, want 30", ms)
	}
	l.fireTimeout(t)
	if fired != 1 {
		t.Errorf("fired %d times, want 1", fired)
	}
}

// TestAfterGTKCancel: cancel removes a pending timer and moves the source
// (its handle freed); twice, after the fire, from inside another timer's
// fn and from inside its own it does nothing more.
func TestAfterGTKCancel(t *testing.T) {
	s, l := newAfterTest(t)
	base := l.removes
	var got []string
	cancelA := s.After(10*time.Millisecond, func() { got = append(got, "a") })
	cancelB := s.After(20*time.Millisecond, func() { got = append(got, "b") })
	cancelA()
	cancelA()
	if ms := pendingMS(t, l); ms != 20 || l.removes != base+1 {
		t.Fatalf("after cancelling the first: armed for %d ms, %d removes", ms, l.removes-base)
	}
	cancelB()
	if len(l.timeouts) != 0 || s.timers.id != 0 {
		t.Fatalf("no timer left, but %d timeouts (id %d)", len(l.timeouts), s.timers.id)
	}

	var cancelC, cancelSelf func()
	s.After(10*time.Millisecond, func() {
		got = append(got, "x")
		cancelC()
	})
	cancelSelf = s.After(10*time.Millisecond, func() {
		got = append(got, "self")
		cancelSelf()
	})
	cancelC = s.After(10*time.Millisecond, func() { got = append(got, "c") })
	cancelD := s.After(20*time.Millisecond, func() { got = append(got, "d") })
	l.fireTimeout(t)
	l.fireTimeout(t)
	cancelD()
	cancelD()
	if want := []string{"x", "self", "d"}; !slices.Equal(got, want) {
		t.Errorf("fired %v, want %v", got, want)
	}
	if len(l.timeouts) != 0 || s.timers.list.Len() != 0 {
		t.Errorf("%d timeouts and %d timers left", len(l.timeouts), s.timers.list.Len())
	}
}

// TestAfterGTKRearm: a timer a fn makes with d <= 0 fires from the next
// source, not in the same round.
func TestAfterGTKRearm(t *testing.T) {
	s, l := newAfterTest(t)
	rounds := 0
	var tick func()
	tick = func() {
		rounds++
		if rounds < 3 {
			s.After(0, tick)
		}
	}
	s.After(0, tick)
	for i := 1; i <= 3; i++ {
		l.fireTimeout(t)
		if rounds != i {
			t.Fatalf("after source %d: %d rounds", i, rounds)
		}
	}
	if len(l.timeouts) != 0 {
		t.Errorf("%d timeouts left", len(l.timeouts))
	}
}

// TestAfterGTKPanic: a panic in fn is trapped by sourceFunc (the pending
// panic, the source's handle freed); the timer is gone and the source
// armed for the rest.
func TestAfterGTKPanic(t *testing.T) {
	s, l := newAfterTest(t)
	s.After(time.Millisecond, func() { panic("boom") })
	fired := false
	s.After(2*time.Millisecond, func() { fired = true })
	l.fireTimeout(t)
	if pendingPanic == nil || pendingPanic.Value != "boom" || pendingPanic.Callback != nameSourceFunc {
		t.Fatalf("pendingPanic = %+v", pendingPanic)
	}
	pendingPanic = nil
	if s.timers.list.Len() != 1 {
		t.Fatalf("%d timers left, want 1", s.timers.list.Len())
	}
	l.fireTimeout(t)
	if !fired {
		t.Error("the second timer did not fire after the panic")
	}
}

// TestAfterGTKQuit: Quit drops the pending timers and removes the source;
// After after Quit makes none, and its cancel does nothing.
func TestAfterGTKQuit(t *testing.T) {
	s, l := newAfterTest(t)
	fired := 0
	cancel := s.After(time.Millisecond, func() { fired++ })
	s.Quit()
	if len(l.timeouts) != 0 || s.timers.list.Len() != 0 || s.timers.id != 0 {
		t.Errorf("after Quit: %d timeouts, %d timers, id %d", len(l.timeouts), s.timers.list.Len(), s.timers.id)
	}
	late := s.After(0, func() { fired++ })
	cancel()
	late()
	if len(l.timeouts) != 0 || fired != 0 {
		t.Errorf("After after Quit: %d timeouts, %d fired", len(l.timeouts), fired)
	}
}

// TestAfterGTKChecks: After needs Ready and a non-nil fn; cancel needs the
// UI thread.
func TestAfterGTKChecks(t *testing.T) {
	pre, _ := newTestShell(t)
	mustPanic(t, "After before Run", func() { pre.After(0, func() {}) })
	s, _ := newAfterTest(t)
	mustPanic(t, "After with a nil func", func() { s.After(0, nil) })
	cancel := s.After(time.Hour, func() {})
	done := make(chan any)
	go func() {
		defer func() { done <- recover() }()
		cancel()
	}()
	if r := <-done; r == nil {
		t.Error("cancel off the UI thread did not panic")
	}
	cancel()
}

// TestAfterGTKRun: over the running fake loop, a timer made in Ready fires
// from the loop on the UI thread; one still pending at Quit is removed,
// with its handle, by the teardown.
func TestAfterGTKRun(t *testing.T) {
	s, l := newTestShell(t)
	base := dispatch.Len()
	fired := 0
	err := run(t, s, &handler{ready: func() {
		s.After(time.Hour, func() { t.Error("the hour timer fired") })
		s.After(0, func() {
			if !s.q.OnUIThread() {
				t.Error("After's fn off the UI thread")
			}
			fired++
			s.Quit()
		})
	}})
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	if fired != 1 {
		t.Errorf("fired %d times, want 1", fired)
	}
	checkTornDown(t, s, l, base)
}
