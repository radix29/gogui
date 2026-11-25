//go:build windows

package win32

import (
	"time"

	"github.com/radix29/gogui/internal/shell/shellkit"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// The one-shot timers of Shell.After (S5.2c). The shell keeps them in a
// shellkit.Timers (deadline order and the round rule, shared with gtk4 and
// the fake) on the clock's timeline (QPC) and arms one USER timer on the
// listener, timerAfter, for the earliest deadline. Its WM_TIMER comes
// through the listener's window procedure, from the shell's loop and from
// a modal loop alike; the timer is killed, one round of the timers due by
// then fires, and the USER timer is armed again for the next one. One USER
// timer, not one per After, so the order holds even when several are due
// at once (Windows hands out expired timers in its own order), and no
// per-process timer quota is spent. A USER timer can come up to a tick
// early (it counts from the last tick); a timer not yet due then waits for
// the next arming. cancel removes a pending timer and re-arms or kills the
// USER timer. Quit and shutdown drop them all.

// timerAfter is the listener's WM_TIMER id of the After timers, above
// timerFrame (the clock's, in modal loops).
const timerAfter = timerFrame + 1

// afterTimers is the shell's pending After timers and the state of the
// USER timer.
type afterTimers struct {
	list    shellkit.Timers
	armed   bool          // timerAfter is set
	armedAt time.Duration // for this deadline
	// Counters (tests): WM_TIMERs handled, and those that found nothing
	// due (an early USER timer).
	fires, early int
}

// timerMS converts d to SetTimer's elapse: whole milliseconds, rounded up,
// at most USER_TIMER_MAXIMUM (Windows raises anything under
// USER_TIMER_MINIMUM itself).
func timerMS(d time.Duration) uint32 {
	if d <= 0 {
		return 0
	}
	ms := (d + time.Millisecond - 1) / time.Millisecond
	return uint32(min(ms, w32.USER_TIMER_MAXIMUM))
}

// After implements shell.Shell. If the USER timer cannot be set (the
// listener is gone, or the process has used up its USER timers), the
// timer waits for the next successful arming.
func (s *Shell) After(d time.Duration, fn func()) (cancel func()) {
	s.checkReady("After")
	if fn == nil {
		panic("win32: After with a nil func")
	}
	if s.quit {
		return func() { s.q.Check("win32.Shell.After's cancel") }
	}
	t := s.timers.list.Add(s.clock.now()+max(d, 0), fn)
	s.armTimers()
	return func() {
		s.q.Check("win32.Shell.After's cancel")
		if s.timers.list.Remove(t) {
			s.armTimers()
		}
	}
}

// armTimers sets the USER timer for the earliest deadline, or kills it
// when no timer is pending.
func (s *Shell) armTimers() {
	ts := &s.timers
	due, ok := ts.list.Next()
	if !ok || s.quit || s.listener == 0 {
		s.killTimer()
		return
	}
	if ts.armed && ts.armedAt == due {
		return
	}
	// SetTimer on a set id resets it.
	if w32.SetTimer(s.listener, timerAfter, timerMS(due-s.clock.now())) != nil {
		ts.armed = false
		return
	}
	ts.armed, ts.armedAt = true, due
}

func (s *Shell) killTimer() {
	if s.timers.armed {
		_ = w32.KillTimer(s.listener, timerAfter)
		s.timers.armed = false
	}
}

// fireTimers handles timerAfter's WM_TIMER: it fires one round of the
// timers due now (one a fired fn makes waits for the next round, even with
// d <= 0), then arms the USER timer for the rest, also when a fn panics.
func (s *Shell) fireTimers() {
	ts := &s.timers
	s.killTimer()
	ts.fires++
	defer s.armTimers()
	if !ts.list.Fire(s.clock.now(), func() bool { return s.quit }) {
		ts.early++
	}
}

// stopTimers drops every pending After timer (Quit, shutdown).
func (s *Shell) stopTimers() {
	s.killTimer()
	s.timers.list.Clear()
}
