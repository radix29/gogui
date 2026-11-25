//go:build windows

package win32

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/shell/win32/w32"
)

func TestTimerMS(t *testing.T) {
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
		{24 * 365 * time.Hour, w32.USER_TIMER_MAXIMUM},
	} {
		if got := timerMS(c.d); got != c.want {
			t.Errorf("timerMS(%v) = %d, want %d", c.d, got, c.want)
		}
	}
}

// timerSlack is how early a USER timer may seem to fire against
// time.Now: both clocks are read at a tick's granularity.
const timerSlack = 2 * time.Millisecond

// TestAfterOrder: timers made out of order fire in deadline order, once
// each, on the UI thread, never inside After, and not before their delay.
func TestAfterOrder(t *testing.T) {
	inShellAfter(t, func(s *Shell, done func()) {
		start := time.Now()
		var got []string
		after := func(name string, d time.Duration, last bool) {
			s.After(d, func() {
				if !s.q.OnUIThread() {
					t.Error("After's fn off the UI thread")
				}
				if el := time.Since(start); el < d-timerSlack {
					t.Errorf("timer %s fired after %v, want at least %v", name, el, d)
				}
				got = append(got, name)
				if last {
					if want := []string{"zero", "a", "b", "b2", "c"}; !slices.Equal(got, want) {
						t.Errorf("fired %v, want %v", got, want)
					}
					done()
				}
			})
		}
		after("c", 90*time.Millisecond, true)
		after("a", 30*time.Millisecond, false)
		after("b", 60*time.Millisecond, false)
		after("b2", 60*time.Millisecond, false) // made after b, due after it
		after("zero", 0, false)
		if len(got) != 0 {
			t.Error("After ran fn inside the call")
		}
		if s.timers.list.Len() != 5 || !s.timers.armed {
			t.Errorf("%d timers pending (armed %v), want 5 armed", s.timers.list.Len(), s.timers.armed)
		}
	})
}

// TestAfterCancel: a cancelled timer never fires; cancel twice, after the
// fire, from inside another timer's fn and from inside its own does
// nothing more.
func TestAfterCancel(t *testing.T) {
	inShellAfter(t, func(s *Shell, done func()) {
		var got []string
		cancelA := s.After(10*time.Millisecond, func() { got = append(got, "a") })
		cancelA()
		cancelA()
		var cancelC, cancelSelf func()
		cancelSelf = s.After(30*time.Millisecond, func() {
			got = append(got, "self")
			cancelSelf()
		})
		s.After(40*time.Millisecond, func() {
			got = append(got, "b")
			cancelC()
			cancelSelf() // fired already
		})
		cancelC = s.After(60*time.Millisecond, func() { got = append(got, "c") })
		s.After(120*time.Millisecond, func() {
			if want := []string{"self", "b"}; !slices.Equal(got, want) {
				t.Errorf("fired %v, want %v", got, want)
			}
			done()
		})
	})
}

// TestAfterQuit: timers pending at Quit are killed and never fire; After
// after Quit makes none, and its cancel does nothing.
func TestAfterQuit(t *testing.T) {
	s := newTestShell()
	fired := 0
	var late func()
	err := run(t, s, &handler{ready: func() {
		s.After(0, func() { fired++ })
		cancel := s.After(time.Hour, func() { fired++ })
		s.Quit()
		cancel()
		late = s.After(0, func() { fired++ })
		if s.timers.list.Len() != 0 {
			t.Errorf("%d timers pending after Quit", s.timers.list.Len())
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if fired != 0 {
		t.Errorf("%d timers fired after Quit", fired)
	}
	late()
}

// TestAfterPanic: a panic in fn (inside the listener's window procedure)
// ends Run with a *PanicError carrying it; the timer is gone.
func TestAfterPanic(t *testing.T) {
	s := newTestShell()
	boom := errors.New("boom")
	var got any
	func() {
		defer func() { got = recover() }()
		_ = run(t, s, &handler{ready: func() {
			s.After(time.Millisecond, func() { panic(boom) })
			s.After(time.Hour, func() {})
		}})
	}()
	pe, ok := got.(*PanicError)
	if !ok {
		t.Fatalf("Run panicked with %T %v, want *PanicError", got, got)
	}
	if !errors.Is(pe, boom) || pe.Msg != w32.WM_TIMER {
		t.Errorf("PanicError = %v (msg %#x), want boom in WM_TIMER", pe.Value, pe.Msg)
	}
	if s.timers.list.Len() != 0 {
		t.Errorf("%d timers left after the panic", s.timers.list.Len())
	}
}

// TestAfterChecks: After needs Ready and a non-nil fn; cancel needs the
// UI thread.
func TestAfterChecks(t *testing.T) {
	s := newTestShell()
	mustPanic(t, "After before Run", func() { s.After(0, func() {}) })
	inShell(t, func(s *Shell) {
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
	})
}

// TestAfterMany: a few hundred timers in five deadline groups, some
// cancelled, all fire once in (deadline, creation) order, also when many
// are due in the same WM_TIMER; no timer is left.
func TestAfterMany(t *testing.T) {
	inShellAfter(t, func(s *Shell, done func()) {
		const n = 300
		var fired, want []int
		for i := range n {
			d := time.Duration(i%5) * 40 * time.Millisecond
			cancel := s.After(d, func() {
				fired = append(fired, i)
				if len(fired) < len(want) {
					return
				}
				if !slices.Equal(fired, want) {
					t.Errorf("fired %v,\nwant %v", fired, want)
				}
				if s.timers.list.Len() != 1 { // the watchdog
					t.Errorf("%d timers left, want the watchdog", s.timers.list.Len())
				}
				done()
			})
			if i%7 == 0 {
				cancel()
				continue
			}
			want = append(want, i)
		}
		slices.SortStableFunc(want, func(a, b int) int { return a%5 - b%5 })
	})
}

// inShellAfter runs fn in Ready and quits when fn's done is called (from a
// timer), or fails the test after 5 s.
func inShellAfter(t *testing.T, fn func(s *Shell, done func())) {
	t.Helper()
	s := newTestShell()
	err := run(t, s, &handler{ready: func() {
		finished := false
		fn(s, func() {
			finished = true
			s.Quit()
		})
		// The watchdog fires only if done never came: Quit drops it otherwise.
		s.After(5*time.Second, func() {
			if !finished {
				t.Error("the test's timers did not finish within 5 s")
				s.Quit()
			}
		})
	}})
	if err != nil {
		t.Fatal(err)
	}
	if s.timers.list.Len() != 0 || s.timers.armed {
		t.Errorf("after Run: %d timers, armed %v", s.timers.list.Len(), s.timers.armed)
	}
}

// TestAfterRearm: a fn that makes a timer with d <= 0 gets it in a later
// WM_TIMER, not in the same round, so it cannot starve the loop; posted
// work runs in between.
func TestAfterRearm(t *testing.T) {
	inShellAfter(t, func(s *Shell, done func()) {
		rounds, posted := 0, 0
		fires := s.timers.fires
		var tick func()
		tick = func() {
			rounds++
			s.Post(func() { posted++ })
			if rounds < 5 {
				s.After(0, tick)
				return
			}
			if n := s.timers.fires - fires; n < 5 {
				t.Errorf("5 rounds in %d WM_TIMERs", n)
			}
			if posted < 4 {
				t.Errorf("%d posted functions ran between the rounds, want 4", posted)
			}
			t.Logf("%d WM_TIMERs for 5 rounds, %d early", s.timers.fires-fires, s.timers.early)
			done()
		}
		s.After(0, tick)
	})
}
