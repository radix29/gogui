package fake

import (
	"math"
	"time"
)

// The one-shot timers of Shell.After (S5.2c). They live on the Clock in a
// shellkit.Timers, the list win32 and gtk4 use (review R6), so Advance can
// fire them on the Now timeline between the frames: a timer is due at the
// Now of its After plus d (d <= 0: at that Now), and Advance fires every
// timer due by its end time in deadline order, timers due at the same time
// in the order they were made. A timer due at or before a requested frame
// fires before that frame. Flush and Step fire no timers (time does not
// move).
//
// Timers fire in rounds, as on the real shells: a round fires the timers
// due at its time that existed when it started, and a timer a fn makes
// waits for the next round even when it is already due (After with
// d <= 0). The real shells run the next round on the next pass of their
// loop; the fake runs it the next time Now moves within the Advance (the
// next deadline, or the next frame or the end time, the round firing
// before that frame), so a fn that re-arms itself with After(0) runs once
// per such step and Advance still ends. Such a timer still pending when
// Advance ends fires first in the next Advance.

// noRound is Clock.round before the first round of an Advance.
const noRound = time.Duration(math.MinInt64)

// after adds a timer due d after Now and returns its cancel.
func (c *Clock) after(d time.Duration, fn func()) (cancel func()) {
	t := c.timers.Add(c.now+max(d, 0), fn)
	return func() {
		c.checkUI("After's cancel")
		c.timers.Remove(t)
	}
}

// fireDue fires the rounds of the timers due at or before limit, each at a
// later time than the round before it in this Advance (c.round), and stops
// after a round that requested a frame (Advance looks at it first); Now moves
// to each round's time. A round's time is its earliest timer's deadline,
// or Now if later; when that time had its round already (a timer a fn of
// it made), the round waits for the next deadline, or for limit. It
// reports whether any timer fired.
func (c *Clock) fireDue(limit time.Duration) bool {
	fired, requested := false, c.requested
	for {
		at, ok := c.timers.Next()
		if !ok || at > limit {
			return fired
		}
		at = max(at, c.now)
		if at <= c.round {
			next, ok := c.timers.NextAfter(c.round)
			if !ok || next > limit {
				next = limit
			}
			if next <= c.round {
				return fired
			}
			at = next
		}
		c.now, c.round = at, at
		c.inTimer = true
		func() {
			defer func() { c.inTimer = false }()
			fired = c.timers.Fire(at, nil) || fired
		}()
		if c.requested && !requested {
			return true // the requested frame may come before the next round
		}
	}
}

// dropTimers drops every pending timer (Quit).
func (c *Clock) dropTimers() { c.timers.Clear() }

// After implements shell.Shell: the timer fires during Clock().Advance,
// once Now has reached the deadline (see the package's timer rules in
// timer.go). After Quit it makes no timer.
func (s *Shell) After(d time.Duration, fn func()) (cancel func()) {
	s.checkReady("Shell.After")
	if fn == nil {
		panic("fake: After with a nil func")
	}
	if s.quit {
		return func() { s.check("After's cancel") }
	}
	return s.clock.after(d, fn)
}

// Timers returns the deadlines of the pending timers on the Now timeline,
// in the order they will fire.
func (s *Shell) Timers() []time.Duration {
	s.check("Shell.Timers")
	return s.clock.timers.Deadlines()
}
