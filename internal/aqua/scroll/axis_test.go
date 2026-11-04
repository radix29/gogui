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

// axisRig is an Axis on a timeline on a standalone fake clock: viewport
// 200 (c = 110 with the Sequoia fraction 0.55), Max 1000.
type axisRig struct {
	t    *testing.T
	clk  *fake.Clock
	tl   *anim.Timeline
	a    *scroll.Axis
	p    scroll.Params
	seen []float64 // every offset onChange reported
}

func newAxis(t *testing.T) *axisRig {
	t.Helper()
	r := &axisRig{t: t, clk: fake.NewClock(), p: scroll.ThemeParams(&theme.Sequoia)}
	r.tl = anim.NewTimeline(r.clk)
	r.clk.SetFrameFunc(r.tl.Tick)
	r.a = scroll.NewAxis(r.tl, r.p, func(x float64) { r.seen = append(r.seen, x) })
	r.a.SetViewport(200)
	r.a.SetMax(1000)
	return r
}

func (r *axisRig) c() float64 { return r.p.RubberBand * 200 }

// send delivers one scroll event at the clock's Now. d is in the offset's
// direction (the axis negates ScrollEvent deltas), so the test reads like
// the offset.
func (r *axisRig) send(ev shell.ScrollEvent, d float64) bool {
	ev.Time = r.clk.Now()
	return r.a.Event(ev, -d)
}

func (r *axisRig) phase(ph shell.ScrollPhase, d float64) bool {
	return r.send(shell.ScrollEvent{Precise: true, Phase: ph}, d)
}

func (r *axisRig) momentum(ph shell.ScrollPhase, d float64) bool {
	return r.send(shell.ScrollEvent{Precise: true, Momentum: ph}, d)
}

func (r *axisRig) wheel(lines float64) bool { return r.send(shell.ScrollEvent{}, lines) }

// swipe sends a Win32 gesture: deltas every interval, PhaseEnded endGap
// after the last one.
func (r *axisRig) swipe(interval, endGap time.Duration, deltas ...float64) {
	for i, d := range deltas {
		ph := shell.PhaseChanged
		if i == 0 {
			ph = shell.PhaseBegan
		} else {
			r.clk.Advance(interval)
		}
		r.phase(ph, d)
	}
	r.clk.Advance(endGap)
	r.phase(shell.PhaseEnded, 0)
}

// settle runs frames until nothing moves (at most 10 s) and returns the
// largest and smallest offsets seen on the way.
func (r *axisRig) settle() (lo, hi float64) {
	r.t.Helper()
	lo, hi = r.a.Offset(), r.a.Offset()
	r.seen = r.seen[:0]
	for i := 0; i < 600 && r.a.Moving(); i++ {
		r.clk.Advance(fake.FrameInterval)
	}
	if r.a.Moving() || r.clk.Requested() {
		r.t.Fatalf("still moving after 10 s at %g", r.a.Offset())
	}
	for _, x := range r.seen {
		lo, hi = math.Min(lo, x), math.Max(hi, x)
	}
	return lo, hi
}

func same(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func repeat(d float64, n int) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = d
	}
	return s
}

func TestAxisDragOneToOne(t *testing.T) {
	r := newAxis(t)
	r.a.ScrollTo(100, false)
	want := 100.0
	for i, d := range []float64{5, 12, -3, 40} {
		ph := shell.PhaseChanged
		if i == 0 {
			ph = shell.PhaseBegan
		}
		if !r.phase(ph, d) {
			t.Fatal("gesture event not taken")
		}
		want += d
		if r.a.Offset() != want || !r.a.Dragging() {
			t.Fatalf("after %g: offset %g, want %g", d, r.a.Offset(), want)
		}
		r.clk.Advance(10 * time.Millisecond)
	}
	// Fingers rest longer than EndGap before the lift: no momentum.
	r.clk.Advance(time.Second)
	r.phase(shell.PhaseEnded, 0)
	if r.a.Moving() || r.a.Offset() != want {
		t.Errorf("rested release: moving %v at %g", r.a.Moving(), r.a.Offset())
	}
}

func TestAxisFlingDecaysToRest(t *testing.T) {
	r := newAxis(t)
	r.a.SetMax(5000)
	r.a.ScrollTo(100, false)
	r.swipe(10*time.Millisecond, fake.SwipeEndDelay, repeat(20, 10)...) // 2000 pt/s
	at := r.a.Offset()
	if at != 300 || !r.a.Moving() {
		t.Fatalf("release at %g, moving %v", at, r.a.Moving())
	}
	rest := at + (2000-r.p.StopVelocity)/r.p.DecayRate
	if !same(r.a.Target(), rest) {
		t.Errorf("target %g, want %g", r.a.Target(), rest)
	}
	r.clk.Advance(100 * time.Millisecond)
	d := scroll.NewDecay(2000, r.p.DecayRate, r.p.StopVelocity)
	if x, _ := d.At(r.clk.Now() - r.clk.Now()%fake.FrameInterval - (r.clk.Now() - 100*time.Millisecond)); math.Abs(r.a.Offset()-(at+x)) > 1e-6 {
		t.Errorf("decay at 100 ms: %g, want %g", r.a.Offset(), at+x)
	}
	lo, hi := r.settle()
	if r.a.Offset() != rest || lo < at || hi > rest {
		t.Errorf("rest %g (want %g), range %g..%g", r.a.Offset(), rest, lo, hi)
	}
}

func TestAxisFlingIntoEdgeSpringsBack(t *testing.T) {
	for _, dir := range []float64{1, -1} {
		r := newAxis(t)
		r.a.SetMax(400)
		start, edge := 300.0, 400.0
		if dir < 0 {
			start, edge = 100, 0
		}
		r.a.ScrollTo(start, false)
		r.swipe(10*time.Millisecond, fake.SwipeEndDelay, repeat(dir*30, 3)...) // 3000 pt/s
		lo, hi := r.settle()
		over := hi - edge
		if dir < 0 {
			over = edge - lo
		}
		if r.a.Offset() != edge || !(over > 1) || over >= r.c() {
			t.Errorf("dir %g: rest %g (want %g), overshoot %g (c %g)", dir, r.a.Offset(), edge, over, r.c())
		}
	}
}

func TestAxisOverscrollDragAndRelease(t *testing.T) {
	r := newAxis(t)
	c := r.c()
	r.phase(shell.PhaseBegan, -50)
	for range 3 {
		r.clk.Advance(10 * time.Millisecond)
		r.phase(shell.PhaseChanged, -50)
	}
	if want := scroll.RubberBand(-200, c); !same(r.a.Offset(), want) || r.a.Clamped() != 0 || !same(r.a.Overscroll(), want) {
		t.Fatalf("overscrolled to %g (clamped %g), want %g", r.a.Offset(), r.a.Clamped(), want)
	}
	r.clk.Advance(10 * time.Millisecond)
	r.phase(shell.PhaseChanged, 100) // back 1:1 in finger space
	if want := scroll.RubberBand(-100, c); !same(r.a.Offset(), want) {
		t.Fatalf("dragged back to %g, want %g", r.a.Offset(), want)
	}
	r.clk.Advance(time.Second) // rest, then lift
	r.phase(shell.PhaseEnded, 0)
	lo, hi := r.settle()
	if r.a.Offset() != 0 || hi > 0 {
		t.Errorf("spring-back to %g, range %g..%g", r.a.Offset(), lo, hi)
	}

	// Past the end, and caught during the spring-back: the new gesture
	// starts where the content is.
	r.a.ScrollTo(1000, false)
	r.phase(shell.PhaseBegan, 80)
	r.clk.Advance(time.Second)
	r.phase(shell.PhaseEnded, 0)
	r.clk.Advance(5 * fake.FrameInterval)
	caught := r.a.Offset()
	if !(caught > 1000) {
		t.Fatalf("spring-back already done: %g", caught)
	}
	r.phase(shell.PhaseBegan, 0)
	if !same(r.a.Offset(), caught) || !r.a.Dragging() {
		t.Errorf("caught at %g, want %g", r.a.Offset(), caught)
	}
	r.phase(shell.PhaseChanged, 10)
	raw := 1000 + scroll.RubberBandInverse(caught-1000, c) + 10
	if want := 1000 + scroll.RubberBand(raw-1000, c); !same(r.a.Offset(), want) {
		t.Errorf("drag after catch: %g, want %g", r.a.Offset(), want)
	}
}

// A release past an edge that flings back into the range decays there.
func TestAxisFlingBackFromOverscroll(t *testing.T) {
	r := newAxis(t)
	r.phase(shell.PhaseBegan, -60)
	r.clk.Advance(time.Second)
	r.swipe(10*time.Millisecond, 50*time.Millisecond, repeat(40, 6)...)
	if !r.a.Moving() {
		t.Fatal("no momentum")
	}
	r.settle()
	if !(r.a.Offset() > 100) {
		t.Errorf("rest %g: the fling did not carry into the range", r.a.Offset())
	}
}

func TestAxisWheel(t *testing.T) {
	r := newAxis(t)
	lh := r.p.LineHeight
	if !r.wheel(3) {
		t.Fatal("wheel not taken")
	}
	if r.a.Offset() != 0 || r.a.Target() != 3*lh {
		t.Fatalf("wheel step: at %g, target %g", r.a.Offset(), r.a.Target())
	}
	r.clk.Advance(r.p.WheelStep / 2)
	if x := r.a.Offset(); !(x > 0 && x < 3*lh) {
		t.Errorf("half-way: %g", x)
	}
	r.clk.Advance(r.p.WheelStep)
	if r.a.Offset() != 3*lh || r.a.Moving() {
		t.Errorf("after the step: %g, moving %v", r.a.Offset(), r.a.Moving())
	}

	// Rapid notches accumulate.
	r.wheel(1)
	r.clk.Advance(30 * time.Millisecond)
	r.wheel(1)
	r.clk.Advance(30 * time.Millisecond)
	r.wheel(-0.5) // a high-resolution wheel turning back
	if r.a.Target() != 4.5*lh {
		t.Errorf("accumulated target %g, want %g", r.a.Target(), 4.5*lh)
	}
	r.settle()
	if r.a.Offset() != 4.5*lh {
		t.Errorf("rapid notches end at %g", r.a.Offset())
	}

	// At an edge the wheel is not taken, and steps clamp to the range.
	r.a.ScrollTo(0, false)
	if r.wheel(-1) || r.a.Moving() {
		t.Error("wheel past the leading edge taken")
	}
	r.a.ScrollTo(990, false)
	if !r.wheel(2) || r.a.Target() != 1000 {
		t.Errorf("clamped step target %g", r.a.Target())
	}
	r.settle()
	if r.wheel(1) {
		t.Error("wheel past the end taken")
	}
	if !r.a.CanScroll(-1) || r.a.CanScroll(1) || r.a.CanScroll(0) {
		t.Error("CanScroll at the end")
	}
}

// A precise event outside a gesture scrolls 1:1, clamped.
func TestAxisPreciseWithoutPhases(t *testing.T) {
	r := newAxis(t)
	if !r.send(shell.ScrollEvent{Precise: true}, 25) || r.a.Offset() != 25 || r.a.Moving() {
		t.Errorf("precise: %g", r.a.Offset())
	}
	if !r.send(shell.ScrollEvent{Precise: true}, -100) || r.a.Offset() != 0 {
		t.Errorf("clamped: %g", r.a.Offset())
	}
	if r.send(shell.ScrollEvent{Precise: true}, -1) {
		t.Error("taken at the edge")
	}
}

func TestAxisOSMomentum(t *testing.T) {
	r := newAxis(t)
	r.a.ScrollTo(100, false)
	// GTK: a fling, then the OS momentum at once.
	r.swipe(10*time.Millisecond, 0, repeat(20, 5)...)
	at := r.a.Offset()
	if !r.a.Moving() {
		t.Fatal("no own momentum after the release")
	}
	r.momentum(shell.PhaseBegan, 15)
	want := at + 15
	for _, d := range []float64{10, 6, 3, 1} {
		r.clk.Advance(fake.FrameInterval)
		r.momentum(shell.PhaseChanged, d)
		want += d
		if r.a.Offset() != want {
			t.Fatalf("OS momentum: %g, want %g", r.a.Offset(), want)
		}
	}
	r.momentum(shell.PhaseEnded, 0)
	if r.a.Moving() {
		t.Error("moving after the OS momentum ended")
	}
	r.clk.Advance(time.Second)
	if r.a.Offset() != want {
		t.Errorf("moved on by itself to %g", r.a.Offset())
	}

	// Into an edge: spring back from there, the rest is ignored.
	r.a.SetMax(want + 20)
	edge := r.a.Max()
	r.swipe(10*time.Millisecond, 0, 5, 5)
	r.a.Stop()
	r.a.ScrollTo(edge-20, false)
	r.momentum(shell.PhaseBegan, 15)
	r.clk.Advance(fake.FrameInterval)
	r.momentum(shell.PhaseChanged, 15) // reaches the edge
	if r.a.Offset() != edge || !r.a.Moving() {
		t.Fatalf("at the edge: %g, moving %v", r.a.Offset(), r.a.Moving())
	}
	r.clk.Advance(fake.FrameInterval)
	r.momentum(shell.PhaseChanged, 15)
	r.momentum(shell.PhaseEnded, 0)
	lo, hi := r.settle()
	if r.a.Offset() != edge || !(hi > edge) || hi-edge >= r.c() {
		t.Errorf("bounce: rest %g (edge %g), range %g..%g", r.a.Offset(), edge, lo, hi)
	}
}

func TestAxisReduceMotion(t *testing.T) {
	r := newAxis(t)
	r.tl.SetReduceMotion(true)
	r.phase(shell.PhaseBegan, -50) // no rubber band
	if r.a.Offset() != 0 {
		t.Errorf("drag past the edge: %g", r.a.Offset())
	}
	r.swipe(10*time.Millisecond, fake.SwipeEndDelay, repeat(20, 10)...)
	if r.a.Offset() != 200 || r.a.Moving() || r.clk.Requested() {
		t.Errorf("fling: at %g, moving %v", r.a.Offset(), r.a.Moving())
	}
	r.wheel(2)
	if r.a.Offset() != 200+2*r.p.LineHeight || r.a.Moving() {
		t.Errorf("wheel: %g", r.a.Offset())
	}
	r.a.ScrollTo(10, true)
	if r.a.Offset() != 10 {
		t.Errorf("animated ScrollTo: %g", r.a.Offset())
	}

	// Turned on during a spring-back: at the edge at once.
	r = newAxis(t)
	r.phase(shell.PhaseBegan, -50)
	r.clk.Advance(time.Second)
	r.phase(shell.PhaseEnded, 0)
	r.clk.Advance(fake.FrameInterval)
	r.tl.SetReduceMotion(true)
	if r.a.Offset() != 0 || r.a.Moving() {
		t.Errorf("spring-back under Reduce Motion: %g", r.a.Offset())
	}
}

func TestAxisSetMax(t *testing.T) {
	// Shrinking under an overscrolled drag: shown against the new edge,
	// released back to it.
	r := newAxis(t)
	c := r.c()
	r.a.ScrollTo(1000, false)
	r.phase(shell.PhaseBegan, 50)
	r.a.SetMax(500)
	if want := 500 + scroll.RubberBand(1050-500, c); !same(r.a.Offset(), want) {
		t.Errorf("drag against the new edge: %g, want %g", r.a.Offset(), want)
	}
	r.clk.Advance(time.Second)
	r.phase(shell.PhaseEnded, 0)
	r.settle()
	if r.a.Offset() != 500 {
		t.Errorf("released to %g", r.a.Offset())
	}

	// Shrinking during a spring-back: the spring retargets.
	r = newAxis(t)
	r.a.ScrollTo(1000, false)
	r.phase(shell.PhaseBegan, 60)
	r.clk.Advance(time.Second)
	r.phase(shell.PhaseEnded, 0)
	r.clk.Advance(3 * fake.FrameInterval)
	r.a.SetMax(800)
	_, hi := r.settle()
	if r.a.Offset() != 800 || hi > 1000+c {
		t.Errorf("retargeted spring-back to %g (peak %g)", r.a.Offset(), hi)
	}

	// At rest beyond the new edge: clamped at once.
	r.a.ScrollTo(700, false)
	r.a.SetMax(300)
	if r.a.Offset() != 300 || r.a.Moving() {
		t.Errorf("clamped to %g", r.a.Offset())
	}
	// A wheel step's target is clamped.
	r.a.SetMax(1000)
	r.wheel(10)
	r.a.SetMax(350)
	if r.a.Target() != 350 {
		t.Errorf("wheel target %g", r.a.Target())
	}
	r.settle()
	// A decay cut by the new edge springs back from there.
	r.a.SetMax(5000)
	r.a.ScrollTo(0, false)
	r.swipe(10*time.Millisecond, fake.SwipeEndDelay, repeat(20, 10)...)
	r.clk.Advance(200 * time.Millisecond)
	r.a.SetMax(r.a.Offset() - 30)
	m := r.a.Max()
	r.settle()
	if r.a.Offset() != m {
		t.Errorf("decay past the new edge rests at %g, want %g", r.a.Offset(), m)
	}
	r.a.SetMax(-5)
	if r.a.Max() != 0 || r.a.Offset() != 0 {
		t.Errorf("negative max: %g at %g", r.a.Max(), r.a.Offset())
	}
}

func TestAxisScrollToAndStop(t *testing.T) {
	r := newAxis(t)
	r.a.ScrollTo(2000, false)
	if r.a.Offset() != 1000 {
		t.Errorf("clamped ScrollTo: %g", r.a.Offset())
	}
	r.a.ScrollTo(400, true)
	if r.a.Offset() != 1000 || r.a.Target() != 400 {
		t.Errorf("animated: %g → %g", r.a.Offset(), r.a.Target())
	}
	r.settle()
	if r.a.Offset() != 400 {
		t.Errorf("animated ScrollTo ends at %g", r.a.Offset())
	}
	r.swipe(10*time.Millisecond, fake.SwipeEndDelay, repeat(20, 10)...)
	r.clk.Advance(50 * time.Millisecond)
	x := r.a.Offset()
	r.a.Stop()
	r.clk.Advance(time.Second)
	if r.a.Offset() != x || r.a.Moving() {
		t.Errorf("Stop during the decay: %g, want %g", r.a.Offset(), x)
	}
	// Stop while overscrolled ends at the edge.
	r.a.ScrollTo(0, false)
	r.phase(shell.PhaseBegan, -40)
	r.a.Stop()
	if r.a.Offset() != 0 || r.a.Dragging() {
		t.Errorf("Stop past the edge: %g", r.a.Offset())
	}
}

func TestNewAxisNilTimeline(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("no panic")
		}
	}()
	scroll.NewAxis(nil, scroll.Params{}, nil)
}

// A gesture whose PhaseBegan went elsewhere starts at its first event; a
// lone PhaseEnded releases nothing.
func TestAxisGestureWithoutBegan(t *testing.T) {
	r := newAxis(t)
	r.a.ScrollTo(100, false)
	r.phase(shell.PhaseChanged, 10)
	if r.a.Offset() != 110 || !r.a.Dragging() {
		t.Errorf("changed without began: %g", r.a.Offset())
	}
	r.a.Stop()
	r.phase(shell.PhaseEnded, 0)
	if r.a.Offset() != 110 || r.a.Moving() {
		t.Errorf("lone ended: %g, moving %v", r.a.Offset(), r.a.Moving())
	}
}

// A release past an edge flung further out springs back with the speed
// (overshooting more than a release at rest), and the OS momentum after it
// is ignored.
func TestAxisOverscrollFlingOut(t *testing.T) {
	peak := func(fling bool, osMomentum bool) float64 {
		r := newAxis(t)
		r.a.ScrollTo(1000, false)
		if fling {
			r.swipe(10*time.Millisecond, 0, 10, 10, 10)
		} else {
			r.swipe(10*time.Millisecond, time.Second, 10, 10, 10)
		}
		if osMomentum {
			r.momentum(shell.PhaseBegan, 30)
			r.momentum(shell.PhaseChanged, 30)
			r.momentum(shell.PhaseEnded, 0)
		}
		_, hi := r.settle()
		if r.a.Offset() != 1000 {
			t.Errorf("fling %v: rest %g", fling, r.a.Offset())
		}
		return hi
	}
	rested, flung := peak(false, false), peak(true, false)
	if !(flung > rested) {
		t.Errorf("flung peak %g not above rested %g", flung, rested)
	}
	if got := peak(true, true); got != flung {
		t.Errorf("OS momentum during the spring-back changed the peak: %g, want %g", got, flung)
	}
}

// Without a rubber band (no viewport) a drag stops at the edges.
func TestAxisNoRubberBand(t *testing.T) {
	r := newAxis(t)
	r.a.SetViewport(0)
	r.phase(shell.PhaseBegan, -30)
	if r.a.Offset() != 0 {
		t.Errorf("overscroll without c: %g", r.a.Offset())
	}
	r.a.SetViewport(200)
	r.phase(shell.PhaseChanged, -30)
	if want := scroll.RubberBand(-30, r.c()); !same(r.a.Offset(), want) {
		t.Errorf("with c again: %g, want %g", r.a.Offset(), want)
	}
}
