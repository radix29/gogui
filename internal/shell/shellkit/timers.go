package shellkit

import (
	"cmp"
	"slices"
	"time"
)

// Timers is the list of pending one-shot timers behind shell.Shell.After
// (S5.2c, review R6), shared by shell/win32, shell/gtk4 and shell/fake. The
// shell owns the clock and the OS timer that wakes it for Next; Timers
// keeps the order and the round rule:
//
//   - Timers fire in deadline order, timers with the same deadline in the
//     order they were added.
//   - Fire runs one round: the timers due at now that existed when the
//     round started. A timer added by a fn of the round waits for the next
//     round, even when it is already due (After with d <= 0), so a fn that
//     re-arms itself cannot starve the loop.
//
// The zero value is empty and ready. Timers is not safe for concurrent use;
// the shells use it on the UI thread.
type Timers struct {
	list []*Timer
	seq  uint64
}

// Timer is one pending timer of a Timers, the handle Remove takes.
type Timer struct {
	at  time.Duration // the deadline on the shell's clock
	seq uint64        // the order it was added in, for equal deadlines
	fn  func()
}

// Add adds a timer that runs fn in the first round at or after at and
// returns it.
func (ts *Timers) Add(at time.Duration, fn func()) *Timer {
	ts.seq++
	t := &Timer{at: at, seq: ts.seq, fn: fn}
	i, _ := slices.BinarySearchFunc(ts.list, t, func(a, b *Timer) int {
		return cmp.Or(cmp.Compare(a.at, b.at), cmp.Compare(a.seq, b.seq))
	})
	ts.list = slices.Insert(ts.list, i, t)
	return t
}

// Remove removes t if it is still pending and reports whether it was.
func (ts *Timers) Remove(t *Timer) bool {
	i := slices.Index(ts.list, t)
	if i < 0 {
		return false
	}
	ts.list = slices.Delete(ts.list, i, i+1)
	return true
}

// Len returns the number of pending timers.
func (ts *Timers) Len() int { return len(ts.list) }

// Next returns the earliest deadline; ok is false when no timer is pending.
func (ts *Timers) Next() (at time.Duration, ok bool) {
	if len(ts.list) == 0 {
		return 0, false
	}
	return ts.list[0].at, true
}

// NextAfter returns the earliest deadline later than floor; ok is false
// when there is none.
func (ts *Timers) NextAfter(floor time.Duration) (at time.Duration, ok bool) {
	for _, t := range ts.list {
		if t.at > floor {
			return t.at, true
		}
	}
	return 0, false
}

// Deadlines returns the deadlines of the pending timers in firing order.
func (ts *Timers) Deadlines() []time.Duration {
	at := make([]time.Duration, len(ts.list))
	for i, t := range ts.list {
		at[i] = t.at
	}
	return at
}

// Fire runs one round at now: every timer due at or before now that was
// pending when the round started, in order, each removed before its fn
// runs. It stops early when stop (nil: never) reports true before a timer,
// and when a fn panics (the panic goes on; the timers not yet run stay
// pending). due reports whether a timer was due when the round started (a
// shell's OS timer that fired early finds none).
func (ts *Timers) Fire(now time.Duration, stop func() bool) (due bool) {
	// The round is the due prefix of the list as it is now; a timer a fn
	// adds is not in it wherever it sorts, and one a fn removes is skipped.
	n, _ := slices.BinarySearchFunc(ts.list, now, func(t *Timer, now time.Duration) int {
		return cmp.Or(cmp.Compare(t.at, now), -1) // after every timer due at now
	})
	if n == 0 {
		return false
	}
	round := slices.Clone(ts.list[:n])
	for _, t := range round {
		if stop != nil && stop() {
			break
		}
		if ts.Remove(t) {
			t.fn()
		}
	}
	return true
}

// Clear drops every pending timer.
func (ts *Timers) Clear() { ts.list = nil }
