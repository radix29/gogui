package scroll_test

import (
	"math"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/aqua/scroll"
	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// TestAxisBounceUnderdamped is B7: the overshoot cap follows the spring's
// actual regime, so an underdamped token still stays within c.
func TestAxisBounceUnderdamped(t *testing.T) {
	for name, s := range map[string]anim.Spring{
		"under":  {Mass: 1, Stiffness: 200, Damping: 10},
		"theme":  {}, // the Sequoia token, ζ ≈ 0.99985
		"over":   {Mass: 1, Stiffness: 200, Damping: 60},
		"bouncy": {Mass: 1, Stiffness: 400, Damping: 4},
	} {
		for _, dir := range []float64{1, -1} {
			p := scroll.ThemeParams(&theme.Sequoia)
			if s != (anim.Spring{}) {
				p.Spring = s
			}
			r := newAxisParams(t, p)
			r.a.SetMax(400)
			start, edge := 300.0, 400.0
			if dir < 0 {
				start, edge = 100, 0
			}
			r.a.ScrollTo(start, false)
			r.swipe(10*time.Millisecond, fake.SwipeEndDelay, repeat(dir*60, 3)...) // 6000 pt/s
			lo, hi := r.settle()
			over := hi - edge
			if dir < 0 {
				over = edge - lo
			}
			if r.a.Offset() != edge || !(over > 1) || over > 0.9*r.c()+1e-9 {
				t.Errorf("%s dir %g: rest %g (want %g), overshoot %g (limit %g)", name, dir, r.a.Offset(), edge, over, 0.9*r.c())
			}
		}
	}
}

// TestAxisCatchFarPastEdge is B7's second half: content left more than c
// past the edge (SetMax shrank the document mid-bounce) is caught close to
// where it is, not snapped to the edge.
func TestAxisCatchFarPastEdge(t *testing.T) {
	r := newAxis(t)
	c := r.c()
	r.a.SetMax(400)
	r.a.ScrollTo(300, false)
	r.swipe(10*time.Millisecond, fake.SwipeEndDelay, repeat(30, 3)...)
	for i := 0; i < 60 && r.a.Overscroll() <= 1; i++ {
		r.clk.Advance(fake.FrameInterval)
	}
	if r.a.Overscroll() <= 1 {
		t.Fatalf("no bounce: offset %g", r.a.Offset())
	}
	r.a.SetMax(400 - 3*c) // the new edge is more than c away
	if over := r.a.Overscroll(); !(over > c) {
		t.Fatalf("overscroll after SetMax %g, want > c (%g)", over, c)
	}
	r.phase(shell.PhaseBegan, 0)
	r.phase(shell.PhaseChanged, -1) // a 1 pt move shows the caught position
	want := r.a.Max() + 0.9*c
	if got := r.a.Offset(); math.Abs(got-want) > 1 {
		t.Errorf("caught at %g, want about %g (edge %g)", got, want, r.a.Max())
	}
}

// TestAxisSpringBackSettles (review P11): a spring-back stops once the
// content can no longer move a visible fraction of a point
// (anim.PointTolerance), not after the spring has also crept below an
// absolute speed: from 50 pt past the edge that is about 40 frames, not
// 70, and the last frames still move it.
func TestAxisSpringBackSettles(t *testing.T) {
	r := newAxis(t)
	r.a.SetMax(400)
	r.a.ScrollTo(400, false)
	r.phase(shell.PhaseBegan, 0)
	for r.a.Overscroll() < 50 {
		r.phase(shell.PhaseChanged, 5)
	}
	r.clk.Advance(time.Second) // rest, then lift: no release velocity
	r.phase(shell.PhaseEnded, 0)
	frames := 0
	last := r.a.Offset()
	r.seen = r.seen[:0]
	for ; r.a.Moving(); frames++ {
		if frames > 600 {
			t.Fatal("still moving after 10 s")
		}
		r.clk.Advance(fake.FrameInterval)
	}
	if r.a.Offset() != 400 || frames > 45 {
		t.Errorf("rest %g after %d frames, want 400 within 45", r.a.Offset(), frames)
	}
	for _, x := range r.seen {
		if x != 400 && math.Abs(x-last) < anim.PointTolerance {
			t.Errorf("a move of %g pt below the tolerance was shown", x-last)
		}
		last = x
	}
	t.Logf("%d frames, %d changes", frames, len(r.seen))
}
