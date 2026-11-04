package shellkit

import (
	"time"

	"github.com/radix29/gogui/internal/shell"
)

// MaxEventAge bounds how old an event time may be, in milliseconds. Older
// (or newer) times are not trusted: the time belongs to another event (on
// Windows, a sent message has no time of its own, and GetMessageTime is
// that of an earlier message).
const MaxEventAge = 5000 // ms

// EventTimeAt maps an event time onto the clock: now minus the event's age,
// both tick and evTime in milliseconds on the OS's event timeline (they may
// wrap, so the age is a uint32 difference). An age beyond MaxEventAge gives
// now; the result is never negative.
func EventTimeAt(now time.Duration, tick, evTime uint32) time.Duration {
	age := tick - evTime
	if age > MaxEventAge {
		return now
	}
	return max(0, now-time.Duration(age)*time.Millisecond)
}

// Monotonic keeps the event times a shell hands out from going backwards
// (OS tick counts are coarse, and some events carry no time of their own).
// The zero value is ready to use.
type Monotonic struct{ last time.Duration }

// Next returns t, or the last time returned if that is later.
func (m *Monotonic) Next(t time.Duration) time.Duration {
	if t < m.last {
		t = m.last
	}
	m.last = t
	return t
}

// Point is a position in device pixels (the OS's coordinates of a press).
// It converts directly from a struct of two int32 fields X and Y.
type Point struct{ X, Y int32 }

// ClickRules are the OS's double-click settings: the longest time between
// two presses in milliseconds, and the double-click rectangle (W × H, in
// the units of the press positions) centered on the previous press.
type ClickRules struct {
	Time uint32
	W, H int32
}

// ClickCounter counts the clicks of consecutive presses: a press of the
// same button as the previous one, within the double-click time after it
// and within the double-click rectangle centered on it is the next click.
// The zero value is ready to use.
type ClickCounter struct {
	button shell.PointerButton
	at     Point  // position of the previous press
	time   uint32 // event time of the previous press, ms
	n      int    // its count; 0 after Reset
}

// Press returns the click count of a press of b at p at time t (ms, may
// wrap). atLeast2 is true when the OS already judged the press a double
// click (a Windows DBLCLK message), so the count is at least 2.
func (c *ClickCounter) Press(b shell.PointerButton, p Point, t uint32, r ClickRules, atLeast2 bool) int {
	abs := func(v int32) int32 { return max(v, -v) }
	if c.n > 0 && b == c.button && t-c.time <= r.Time &&
		abs(p.X-c.at.X) <= r.W/2 && abs(p.Y-c.at.Y) <= r.H/2 {
		c.n++
	} else {
		c.n = 1
	}
	if atLeast2 && c.n < 2 {
		c.n = 2
	}
	c.button, c.at, c.time = b, p, t
	return c.n
}

// Reset starts a new count: the next press is a single click.
func (c *ClickCounter) Reset() { c.n = 0 }

// Gesture is the phase state of the precise scroll events of a surface.
// The zero value has no gesture.
type Gesture struct{ active bool }

// Next returns whether an event with a delta precise by itself is precise
// and its phase, and starts or continues the gesture: inside a gesture
// every event is precise and PhaseChanged; a precise event outside one
// begins it (PhaseBegan); anything else is a plain event (PhaseNone).
func (g *Gesture) Next(precise bool) (bool, shell.ScrollPhase) {
	switch {
	case g.active:
		return true, shell.PhaseChanged
	case precise:
		g.active = true
		return true, shell.PhaseBegan
	}
	return false, shell.PhaseNone
}

// End ends the gesture and reports whether one was active.
func (g *Gesture) End() bool {
	was := g.active
	g.active = false
	return was
}
