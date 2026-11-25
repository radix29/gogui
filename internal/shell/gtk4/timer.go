package gtk4

import (
	"time"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/shellkit"
)

// The one-shot timers of Shell.After (S5.2c). The shell keeps them in a
// shellkit.Timers (deadline order and the round rule, shared with win32 and
// the fake) on the clock's timeline (g_get_monotonic_time) and adds one
// GLib timeout source (loop.timeout, kindAfter) for the earliest deadline.
// When it fires, one round of the timers due by then fires and a new
// source is added for the next one. One source, not one per After, so the
// order holds even when several are due at once (GLib dispatches ready
// sources of one priority in its own order). A timer a fn makes waits for
// the next source, even with d <= 0, so a fn that re-arms itself cannot
// starve the loop. cancel removes a pending timer and moves or removes the
// source. The source's handle is freed by sourceFunc when it fires (the
// source is one-shot) and by the shell when it removes the source itself.
// Quit and shutdown drop every timer.

// afterTimers is the shell's pending After timers and the timeout source
// armed for the first; it is the source's peer.
type afterTimers struct {
	s    *Shell
	list shellkit.Timers

	id      uint32     // the timeout source; 0: none
	h       ffi.Handle // its handle
	armedAt time.Duration

	// Counters (tests): sources fired, and those that found nothing due.
	fires, early int
}

var _ sourcePeer = (*afterTimers)(nil)

// timeoutMS converts d to g_timeout_add's interval: whole milliseconds,
// rounded up, at most the largest guint.
func timeoutMS(d time.Duration) uint32 {
	if d <= 0 {
		return 0
	}
	ms := (d + time.Millisecond - 1) / time.Millisecond
	return uint32(min(ms, 1<<32-1))
}

// After implements shell.Shell. If GLib refuses the timeout source, the
// timer waits for the next successful arming.
func (s *Shell) After(d time.Duration, fn func()) (cancel func()) {
	s.checkReady("After")
	if fn == nil {
		panic("gtk4: After with a nil func")
	}
	if s.quit {
		return func() { s.q.Check("gtk4.Shell.After's cancel") }
	}
	ts := &s.timers
	t := ts.list.Add(s.clock.now()+max(d, 0), fn)
	ts.arm()
	return func() {
		s.q.Check("gtk4.Shell.After's cancel")
		if ts.list.Remove(t) {
			ts.arm()
		}
	}
}

// arm keeps one timeout source for the earliest deadline, or none when no
// timer is pending.
func (ts *afterTimers) arm() {
	due, ok := ts.list.Next()
	if !ok || ts.s.quit {
		ts.disarm()
		return
	}
	if ts.id != 0 && ts.armedAt == due {
		return
	}
	ts.disarm()
	ts.id, ts.h = ts.s.loop.timeout(timeoutMS(due-ts.s.clock.now()), ts, kindAfter)
	ts.armedAt = due
}

// disarm removes the timeout source and frees its handle.
func (ts *afterTimers) disarm() {
	if ts.id == 0 {
		return
	}
	ts.s.loop.remove(ts.id)
	dispatch.Free(ts.h)
	ts.id, ts.h = 0, 0
}

// source implements sourcePeer: the timeout fired. It fires one round of
// the timers due now, then arms for the rest, also when a fn panics. The
// source goes (false), and sourceFunc frees its handle.
func (ts *afterTimers) source(k kind) bool {
	if k != kindAfter {
		return false
	}
	ts.id, ts.h = 0, 0
	ts.fires++
	defer ts.arm()
	if !ts.list.Fire(ts.s.clock.now(), func() bool { return ts.s.quit }) {
		ts.early++
	}
	return false
}

// stop drops every pending timer and removes the source (Quit, shutdown).
func (ts *afterTimers) stop() {
	ts.list.Clear()
	ts.disarm()
}
