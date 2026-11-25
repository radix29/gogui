//go:build linux

package gtk4

import (
	"slices"
	"testing"
	"time"
)

// TestAfterRealGTK: over the real GLib main loop, timers made out of order
// (one cancelled, one cancelled from inside another's fn) fire in deadline
// order, on the UI thread, not before their delay; one still pending at
// Quit is removed by the teardown with its handle.
func TestAfterRealGTK(t *testing.T) {
	s, reports := newGTKShell(t)
	base := dispatch.Len()
	var (
		got   []string
		start time.Time
	)
	p, err := runGTK(t, s, &handler{ready: func() {
		start = time.Now()
		after := func(name string, d time.Duration) func() {
			return s.After(d, func() {
				if !s.q.OnUIThread() {
					t.Error("After's fn off the UI thread")
				}
				if el := time.Since(start); el < d-time.Millisecond {
					t.Errorf("timer %s fired after %v, want at least %v", name, el, d)
				}
				got = append(got, name)
			})
		}
		after("c", 90*time.Millisecond)
		after("a", 30*time.Millisecond)
		cancelled := after("x", 40*time.Millisecond)
		cancelled()
		var cancelY func()
		s.After(50*time.Millisecond, func() { got = append(got, "b"); cancelY() })
		cancelY = after("y", 70*time.Millisecond)
		after("zero", 0)
		s.After(120*time.Millisecond, s.Quit)
		s.After(time.Hour, func() { t.Error("the hour timer fired") })
		if len(got) != 0 {
			t.Error("After ran fn inside the call")
		}
	}})
	if p != nil || err != nil {
		t.Fatalf("Run = %v, panicked with %v", err, p)
	}
	t.Logf("AFTER-RESULT: fired %v in %v, %d sources, %d early", got, time.Since(start), s.timers.fires, s.timers.early)
	if want := []string{"zero", "a", "b", "c"}; !slices.Equal(got, want) {
		t.Errorf("fired %v, want %v", got, want)
	}
	if s.timers.list.Len() != 0 || s.timers.id != 0 {
		t.Errorf("after Run: %d timers, source %d", s.timers.list.Len(), s.timers.id)
	}
	checkGTKTornDown(t, s, reports, base)
}
