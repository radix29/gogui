package shellkit

import (
	"testing"
	"time"

	"github.com/radix29/gogui/internal/shell"
)

func TestEventTimeAt(t *testing.T) {
	const now = 10 * time.Second
	for _, c := range []struct {
		tick, ev uint32
		want     time.Duration
	}{
		{1000, 1000, now},
		{1000, 990, now - 10*time.Millisecond},
		{10000, 10000 - MaxEventAge, now - MaxEventAge*time.Millisecond},
		{10000, 10000 - MaxEventAge - 1, now},      // too old: not trusted
		{1000, 1001, now},                          // from the future: not trusted
		{5, 0xFFFFFFFB, now - 10*time.Millisecond}, // the tick count wrapped
		{100000, 90000, now},                       // 10 s: too old
	} {
		if got := EventTimeAt(now, c.tick, c.ev); got != c.want {
			t.Errorf("EventTimeAt(%v, %d, %d) = %v, want %v", now, c.tick, c.ev, got, c.want)
		}
	}
	if got := EventTimeAt(time.Millisecond, 1000, 900); got != 0 {
		t.Errorf("before the clock's start: %v, want 0", got)
	}
}

func TestMonotonic(t *testing.T) {
	var m Monotonic
	for i, s := range []struct{ in, want time.Duration }{
		{0, 0}, {5, 5}, {3, 5}, {5, 5}, {9, 9}, {0, 9},
	} {
		if got := m.Next(s.in); got != s.want {
			t.Errorf("step %d: Next(%v) = %v, want %v", i, s.in, got, s.want)
		}
	}
}

func TestClickCounter(t *testing.T) {
	r := ClickRules{Time: 500, W: 4, H: 4}
	var c ClickCounter
	p := Point{X: 100, Y: 100}
	L, R := shell.ButtonPrimary, shell.ButtonSecondary
	steps := []struct {
		b    shell.PointerButton
		p    Point
		t    uint32
		dbl  bool
		want int
	}{
		{L, p, 1000, false, 1},
		{L, p, 1200, false, 2},
		{L, Point{X: 102, Y: 98}, 1600, false, 3}, // within the rectangle, 400 ms
		{L, p, 2000, false, 4},
		{L, p, 2501, false, 1},                     // too late
		{L, Point{X: 103, Y: 100}, 2600, false, 1}, // too far
		{R, Point{X: 103, Y: 100}, 2700, false, 1}, // another button
		{R, Point{X: 103, Y: 100}, 2800, false, 2},
		{L, p, 2900, true, 2}, // the OS's double click counts 2 at least
		{L, p, 3000, true, 3}, // and the series goes on
		{L, p, 0xFFFF, false, 1},
	}
	for i, s := range steps {
		if got := c.Press(s.b, s.p, s.t, r, s.dbl); got != s.want {
			t.Errorf("step %d: Press = %d, want %d", i, got, s.want)
		}
	}
	// An odd rectangle: half of 5 is 2 whole pixels.
	odd := ClickRules{Time: 500, W: 5, H: 5}
	c = ClickCounter{}
	c.Press(L, p, 0, odd, false)
	if got := c.Press(L, Point{X: 103, Y: 100}, 10, odd, false); got != 1 {
		t.Errorf("3 px off with a 5 px rectangle: %d, want 1", got)
	}
	// The event time wraps.
	c = ClickCounter{}
	c.Press(L, p, 0xFFFFFF00, r, false)
	if got := c.Press(L, p, 0x10, r, false); got != 2 {
		t.Errorf("across the wrap: %d, want 2", got)
	}
	c.Reset()
	if got := c.Press(L, p, 0x20, r, false); got != 1 {
		t.Errorf("after Reset: %d, want 1", got)
	}
}

func TestGesturePhases(t *testing.T) {
	var g Gesture
	type step struct {
		precise bool
		want    bool
		phase   shell.ScrollPhase
	}
	for i, s := range []step{
		{false, false, shell.PhaseNone}, // a notch outside a gesture
		{true, true, shell.PhaseBegan},
		{true, true, shell.PhaseChanged},
		{false, true, shell.PhaseChanged}, // a whole notch inside the gesture
	} {
		if p, ph := g.Next(s.precise); p != s.want || ph != s.phase {
			t.Errorf("step %d: Next(%v) = %v %v, want %v %v", i, s.precise, p, ph, s.want, s.phase)
		}
	}
	if !g.End() || g.End() {
		t.Error("End: want true once, then false")
	}
	if p, ph := g.Next(true); !p || ph != shell.PhaseBegan {
		t.Errorf("after End: %v %v, want a new gesture", p, ph)
	}
}
