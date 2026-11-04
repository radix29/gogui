package gtk4

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// The scroll input's host tests (S4.1j), over the fakeWindow: the scroll
// controller's signals go through the real trampolines (fw.scrollBy …
// fw.decelerate in surface_test.go), the momentum runs on the fake clock's
// frames.

// openScroll opens a shown, painted 400×300 surface whose clock paces on
// its widget, with no frame pending and the logs cleared.
func openScroll(t *testing.T) (*surface, *fakeWindow, *sfHandler, *winFactory) {
	t.Helper()
	_, f := newSurfaceTest(t)
	h := &sfHandler{}
	sf, fw := f.open(t, sfResizable, h)
	sf.Show()
	f.frame(t)
	fw.snap()
	if f.s.clock.requested || sf.invalid {
		t.Fatalf("openScroll: requested %v, invalid %v", f.s.clock.requested, sf.invalid)
	}
	fw.take()
	h.take()
	h.times = nil
	return sf, fw, h, f
}

// openScrollHidden opens a hidden, painted surface: its frames run from
// the clock's fallback timeout, not from a tick callback on its widget.
// (Closing the pacing surface inside its own tick trips the fake clock
// loop, whose destroy check and fireTick do not expect it.)
func openScrollHidden(t *testing.T) (*surface, *fakeWindow, *sfHandler, *winFactory) {
	t.Helper()
	_, f := newSurfaceTest(t)
	h := &sfHandler{}
	sf, fw := f.open(t, sfResizable, h)
	fw.snap()
	if f.s.clock.requested || sf.invalid {
		t.Fatalf("openScrollHidden: requested %v, invalid %v", f.s.clock.requested, sf.invalid)
	}
	fw.take()
	h.take()
	return sf, fw, h, f
}

// takeScroll returns the handler's log with negative zeros shown as 0
// (GTK's 0 negated is −0, which is the same delta).
func takeScroll(h *sfHandler) []string {
	l := h.take()
	for i, s := range l {
		s = strings.ReplaceAll(s, " -0,", " 0,")
		s = strings.ReplaceAll(s, ",-0 ", ",0 ")
		if strings.HasSuffix(s, ",-0") {
			s = strings.TrimSuffix(s, "-0") + "0"
		}
		l[i] = s
	}
	return l
}

// runFrames runs the requested frames until none is requested (at most
// limit) and returns how many ran.
func runFrames(t *testing.T, f *winFactory, limit int) int {
	t.Helper()
	n := 0
	for f.s.clock.requested {
		if n == limit {
			t.Fatalf("still requesting frames after %d", limit)
		}
		f.frame(t)
		n++
	}
	return n
}

// startMomentum runs a touchpad gesture at the pointer position 10,10 and
// releases it with GTK's velocity vx, vy.
func startMomentum(t *testing.T, fw *fakeWindow, h *sfHandler, vx, vy float64) {
	t.Helper()
	fw.ptrMotion(10, 10, fakeEv{})
	fw.scrollBegin(fakeEv{})
	fw.scrollBy(0, 10, gdkScrollUnitSurface, fakeEv{t: 1})
	fw.scrollEnd(fakeEv{t: 2})
	fw.decelerate(vx, vy)
	wantLog(t, "gesture", takeScroll(h), "enter 10,10", "move 10,10",
		"scroll 0,-10 precise began at 10,10", "scroll 0,0 precise ended at 10,10")
}

func near(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestMomentumNew(t *testing.T) {
	for _, v := range []driver.Point{{}, {Y: stopVelocity}, {X: -stopVelocity}, {X: 12, Y: 16},
		{X: math.NaN(), Y: math.NaN()}, {X: math.Inf(1), Y: math.Inf(-1)}, {X: math.NaN(), Y: 3}} {
		if m := newMomentum(v, time.Second); m.on || m != (momentum{}) {
			t.Errorf("newMomentum(%v) = %+v, want the zero momentum", v, m)
		}
	}
	m := newMomentum(driver.Point{Y: stopVelocity + 1e-6}, 0)
	if !m.on {
		t.Error("a speed just above stopVelocity does not run")
	}
	m = newMomentum(driver.Point{X: 1e9, Y: -1e9}, 3*time.Second)
	if m.v0 != (driver.Point{X: maxVelocity, Y: -maxVelocity}) || m.start != 3*time.Second || m.began || m.moved != (driver.Point{}) {
		t.Errorf("capped momentum %+v", m)
	}
	m = newMomentum(driver.Point{X: math.NaN(), Y: 100}, 0)
	if !m.on || m.v0 != (driver.Point{Y: 100}) {
		t.Errorf("NaN axis: %+v, want v0 0,100", m)
	}
	m = newMomentum(driver.Point{X: math.Inf(-1), Y: -500}, 0)
	if !m.on || m.v0 != (driver.Point{Y: -500}) {
		t.Errorf("Inf axis: %+v, want v0 0,-500", m)
	}
	// stop: the speed falls to stopVelocity.
	m = newMomentum(driver.Point{X: 600, Y: -800}, 0)
	want := math.Log(1000/stopVelocity) / momentumDecay
	if !near(m.stop.Seconds(), want, 1e-9) {
		t.Errorf("stop = %v, want %vs", m.stop, want)
	}
}

func TestMomentumAt(t *testing.T) {
	start := 2 * time.Second
	for _, v := range []driver.Point{{X: 600, Y: -800}, {Y: 1000}, {X: -maxVelocity, Y: maxVelocity}, {X: 25}} {
		m := newMomentum(v, start)
		if !m.on {
			t.Fatalf("newMomentum(%v) not running", v)
		}
		for _, tt := range []time.Duration{0, start - time.Millisecond, start} {
			if off, done := m.at(tt); off != (driver.Point{}) || done {
				t.Errorf("%v: at(%v) = %v, %v; want 0, false", v, tt, off, done)
			}
		}
		speed := math.Hypot(m.v0.X, m.v0.Y)
		f := (1 - stopVelocity/speed) / momentumDecay
		wx, wy := m.v0.X*f, m.v0.Y*f
		for _, tt := range []time.Duration{start + m.stop, start + m.stop + time.Nanosecond, start + m.stop + time.Hour} {
			off, done := m.at(tt)
			if !done || !near(off.X, wx, 1e-6*math.Abs(wx)+1e-9) || !near(off.Y, wy, 1e-6*math.Abs(wy)+1e-9) {
				t.Errorf("%v: at(%v) = %v, %v; want %v,%v, true", v, tt, off, done, wx, wy)
			}
		}
		end, _ := m.at(start + m.stop)
		if off, _ := m.at(start + m.stop + time.Hour); off != end {
			t.Errorf("%v: offset after stop %v, at stop %v", v, off, end)
		}
		if _, done := m.at(start + m.stop - time.Nanosecond); done {
			t.Errorf("%v: done before stop", v)
		}
		// Monotone towards the rest, on each axis.
		prev := driver.Point{}
		for i := 1; i <= 200; i++ {
			off, _ := m.at(start + m.stop*time.Duration(i)/200)
			if math.Abs(off.X) < math.Abs(prev.X) || math.Abs(off.Y) < math.Abs(prev.Y) ||
				off.X*m.v0.X < 0 || off.Y*m.v0.Y < 0 {
				t.Fatalf("%v: at step %d: %v after %v", v, i, off, prev)
			}
			prev = off
		}
	}
}

func TestScrollWheel(t *testing.T) {
	_, fw, h, _ := openScroll(t)
	if r := fw.scrollBy(0, 1, gdkScrollUnitWheel, fakeEv{}); r != gtk.True {
		t.Errorf("scroll returned %d, want TRUE", r)
	}
	fw.scrollBy(-1, 0, gdkScrollUnitWheel, fakeEv{})
	fw.scrollBy(-0.5, 2, gdkScrollUnitWheel, fakeEv{})
	fw.scrollBy(0.25, 0, gdkScrollUnitWheel, fakeEv{})
	if r := fw.scrollBy(0, 0, gdkScrollUnitWheel, fakeEv{}); r != gtk.True {
		t.Errorf("zero scroll returned %d, want TRUE", r)
	}
	wantLog(t, "wheel", takeScroll(h), "scroll 0,-3", "scroll 3,0", "scroll 1.5,-6", "scroll -0.75,0")

	// Inside a gesture: still no phase, and the gesture has not begun.
	fw.scrollBegin(fakeEv{})
	fw.scrollBy(0, -1, gdkScrollUnitWheel, fakeEv{})
	fw.scrollEnd(fakeEv{})
	wantLog(t, "wheel in a gesture", takeScroll(h), "scroll 0,3")
	wantLog(t, "window calls", fw.take())
}

// TestScrollWheelShift: Shift turns a vertical wheel horizontal; a wheel
// with an X delta and a precise event are not swapped.
func TestScrollWheelShift(t *testing.T) {
	_, fw, h, _ := openScroll(t)
	shift := fakeEv{state: gdkShiftMask}
	fw.scrollBy(0, 1, gdkScrollUnitWheel, shift)
	fw.scrollBy(0, -0.5, gdkScrollUnitWheel, fakeEv{state: gdkShiftMask | gdkControlMask})
	fw.scrollBy(1, 0, gdkScrollUnitWheel, shift)
	fw.scrollBy(1, 2, gdkScrollUnitWheel, shift)
	fw.scrollBy(0, 0, gdkScrollUnitWheel, shift)
	fw.scrollBy(0, 1, gdkScrollUnitWheel, fakeEv{state: gdkControlMask})
	fw.scrollBy(0, 10, gdkScrollUnitSurface, shift)
	fw.scrollBegin(fakeEv{})
	fw.scrollBy(0, 10, gdkScrollUnitSurface, shift)
	fw.scrollBy(0, 1, gdkScrollUnitWheel, shift)
	fw.scrollEnd(fakeEv{})
	wantLog(t, "shift", takeScroll(h),
		"scroll -3,0 m=1", "scroll 1.5,0 m=3", "scroll -3,0 m=1", "scroll -3,-6 m=1",
		"scroll 0,-3 m=2", "scroll 0,-10 precise m=1",
		"scroll 0,-10 precise began m=1", "scroll -3,0 m=1", "scroll 0,0 precise ended m=1")
}

func TestScrollPrecise(t *testing.T) {
	_, fw, h, _ := openScroll(t)
	fw.scrollBy(0, 30, gdkScrollUnitSurface, fakeEv{})
	fw.scrollBy(-2.5, 0, gdkScrollUnitSurface, fakeEv{})
	fw.scrollBy(0, 0, gdkScrollUnitSurface, fakeEv{}) // dropped
	wantLog(t, "precise outside a gesture", takeScroll(h), "scroll 0,-30 precise", "scroll 2.5,0 precise")

	fw.scrollBegin(fakeEv{})
	fw.scrollBy(0, 30, gdkScrollUnitSurface, fakeEv{})
	fw.scrollBy(1, -2, gdkScrollUnitSurface, fakeEv{})
	fw.scrollBy(0, 0, gdkScrollUnitSurface, fakeEv{})
	fw.scrollEnd(fakeEv{})
	wantLog(t, "gesture", takeScroll(h), "scroll 0,-30 precise began", "scroll -1,2 precise changed",
		"scroll 0,0 precise changed", "scroll 0,0 precise ended")

	// A zero first delta still begins the gesture.
	fw.scrollBegin(fakeEv{})
	fw.scrollBy(0, 0, gdkScrollUnitSurface, fakeEv{})
	fw.scrollEnd(fakeEv{})
	wantLog(t, "zero first delta", takeScroll(h), "scroll 0,0 precise began", "scroll 0,0 precise ended")

	fw.scrollBegin(fakeEv{})
	fw.scrollEnd(fakeEv{})
	fw.scrollEnd(fakeEv{})
	wantLog(t, "begin/end without scroll, end without begin", takeScroll(h))

	// After the end the next precise event has no phase again.
	fw.scrollBegin(fakeEv{})
	fw.scrollBy(0, 1, gdkScrollUnitSurface, fakeEv{})
	fw.scrollEnd(fakeEv{})
	fw.scrollBy(0, 1, gdkScrollUnitSurface, fakeEv{})
	wantLog(t, "after the end", takeScroll(h), "scroll 0,-1 precise began", "scroll 0,0 precise ended",
		"scroll 0,-1 precise")

	// A second begin restarts the gesture.
	fw.scrollBegin(fakeEv{})
	fw.scrollBy(0, 1, gdkScrollUnitSurface, fakeEv{})
	fw.scrollBegin(fakeEv{})
	fw.scrollBy(0, 2, gdkScrollUnitSurface, fakeEv{})
	fw.scrollEnd(fakeEv{})
	wantLog(t, "restarted", takeScroll(h), "scroll 0,-1 precise began", "scroll 0,-2 precise began",
		"scroll 0,0 precise ended")
	wantLog(t, "window calls", fw.take())
}

func TestScrollPosModsTime(t *testing.T) {
	_, fw, h, _ := openScroll(t)
	fw.scrollBy(0, 1, gdkScrollUnitWheel, fakeEv{})
	fw.ptrMotion(12, 34.5, fakeEv{})
	fw.scrollBy(0, 1, gdkScrollUnitWheel, fakeEv{state: gdkShiftMask | gdkControlMask})
	fw.ptrLeave(fakeEv{})
	fw.scrollBy(0, 1, gdkScrollUnitWheel, fakeEv{state: gdkSuperMask})
	wantLog(t, "position and mods", takeScroll(h), "scroll 0,-3", "enter 12,34.5", "move 12,34.5",
		"scroll -3,0 at 12,34.5 m=3", "leave 12,34.5", "scroll 0,-3 at 12,34.5 m=8")

	// The gesture's end keeps the last scroll's mods.
	fw.scrollBegin(fakeEv{})
	fw.scrollBy(0, 1, gdkScrollUnitSurface, fakeEv{state: gdkAltMask})
	fw.scrollEnd(fakeEv{})
	wantLog(t, "end mods", takeScroll(h), "scroll 0,-1 precise began at 12,34.5 m=4",
		"scroll 0,0 precise ended at 12,34.5 m=4")

	// Times never go backwards.
	for i := 1; i < len(h.times); i++ {
		if h.times[i] < h.times[i-1] {
			t.Errorf("times went backwards: %v", h.times)
		}
	}
}

// TestScrollMomentum: decelerate after an ended gesture runs the momentum
// over frames.
func TestScrollMomentum(t *testing.T) {
	sf, fw, h, f := openScroll(t)
	var evs []shell.ScrollEvent
	h.onScroll = func(se shell.ScrollEvent) { evs = append(evs, se) }
	startMomentum(t, fw, h, 300, 1000)
	evs = nil
	if !f.s.clock.requested || !sf.scr.mom.on {
		t.Fatalf("decelerate: requested %v, running %v", f.s.clock.requested, sf.scr.mom.on)
	}
	m := sf.scr.mom
	want, _ := m.at(m.start + m.stop)
	if m.v0 != (driver.Point{X: -300, Y: -1000}) {
		t.Errorf("v0 = %v, want -300,-1000 (GTK's direction negated)", m.v0)
	}
	frames := runFrames(t, f, 1000)
	if len(evs) < 3 {
		t.Fatalf("%d momentum events in %d frames", len(evs), frames)
	}
	var sum driver.Point
	for i, se := range evs {
		wantPh := shell.PhaseChanged
		switch i {
		case 0:
			wantPh = shell.PhaseBegan
		case len(evs) - 1:
			wantPh = shell.PhaseEnded
		}
		if se.Momentum != wantPh || se.Phase != shell.PhaseNone || !se.Precise || se.Pos != (driver.Point{X: 10, Y: 10}) {
			t.Errorf("event %d: %+v, want Momentum %v", i, se, wantPh)
		}
		if i > 0 && se.Time <= evs[i-1].Time {
			t.Errorf("event %d at %v after %v", i, se.Time, evs[i-1].Time)
		}
		if se.Delta.Y > 0 || se.Delta.X > 0 {
			t.Errorf("event %d: delta %v against the release", i, se.Delta)
		}
		sum.X += se.Delta.X
		sum.Y += se.Delta.Y
	}
	if !near(sum.X, want.X, 1e-9) || !near(sum.Y, want.Y, 1e-9) {
		t.Errorf("sum of the deltas %v, want %v", sum, want)
	}
	if rest := -1000 / momentumDecay * (1 - stopVelocity/math.Hypot(300, 1000)); !near(sum.Y, rest, 1e-6) {
		t.Errorf("sum Y %v, want %v", sum.Y, rest)
	}
	if sf.scr.mom != (momentum{}) {
		t.Errorf("momentum left %+v", sf.scr.mom)
	}
	if f.s.clock.requested {
		t.Error("a frame is still requested after the momentum ended")
	}
	if frames < len(evs) {
		t.Errorf("%d events in %d frames", len(evs), frames)
	}
	// Nothing interrupts an ended momentum.
	h.take()
	fw.ptrPressed(1, 10, 10, at(9000, 1))
	wantLog(t, "press after the momentum", takeScroll(h), "down 10,10 b=1 held=1 n=1")
}

// TestScrollMomentumShort: a momentum that stops within its first frame
// still begins before it ends.
func TestScrollMomentumShort(t *testing.T) {
	sf, fw, h, f := openScroll(t)
	startMomentum(t, fw, h, 0, stopVelocity+1)
	if !sf.scr.mom.on {
		t.Fatal("not running")
	}
	runFrames(t, f, 10)
	got := takeScroll(h)
	if len(got) == 0 || !strings.HasSuffix(got[len(got)-1], "mom-ended at 10,10") {
		t.Fatalf("short momentum: %q", got)
	}
	if !strings.Contains(got[0], "mom-began") {
		t.Errorf("a momentum done on its first frame does not begin: %q", got)
	}
}

// TestScrollMomentumNotStarted: decelerate starts nothing without a gesture
// that just ended with a PhaseBegan, or with a slow velocity.
func TestScrollMomentumNotStarted(t *testing.T) {
	sf, fw, h, f := openScroll(t)
	check := func(what string, want ...string) {
		t.Helper()
		wantLog(t, what, takeScroll(h), want...)
		if sf.scr.mom.on || f.s.clock.requested {
			t.Errorf("%s: running %v, frame requested %v", what, sf.scr.mom.on, f.s.clock.requested)
		}
	}
	fw.decelerate(0, 1000)
	check("decelerate alone")

	fw.scrollBy(0, 1, gdkScrollUnitSurface, fakeEv{})
	fw.decelerate(0, 1000)
	check("after a precise scroll", "scroll 0,-1 precise")

	fw.scrollBegin(fakeEv{})
	fw.scrollEnd(fakeEv{})
	fw.decelerate(0, 1000)
	check("after an empty gesture")

	fw.scrollBegin(fakeEv{})
	fw.scrollBy(0, 1, gdkScrollUnitSurface, fakeEv{})
	fw.scrollEnd(fakeEv{})
	fw.scrollBy(0, 1, gdkScrollUnitWheel, fakeEv{})
	fw.decelerate(0, 1000)
	check("a scroll in between", "scroll 0,-1 precise began", "scroll 0,0 precise ended", "scroll 0,-3")

	fw.scrollBegin(fakeEv{})
	fw.scrollBy(0, 1, gdkScrollUnitSurface, fakeEv{})
	fw.scrollEnd(fakeEv{})
	fw.scrollBy(0, 0, gdkScrollUnitWheel, fakeEv{}) // dropped, but a scroll
	fw.decelerate(0, 1000)
	check("a dropped scroll in between", "scroll 0,-1 precise began", "scroll 0,0 precise ended")

	fw.scrollBegin(fakeEv{})
	fw.scrollBy(0, 1, gdkScrollUnitSurface, fakeEv{})
	fw.scrollEnd(fakeEv{})
	fw.scrollBegin(fakeEv{})
	fw.decelerate(0, 1000)
	check("a new gesture in between", "scroll 0,-1 precise began", "scroll 0,0 precise ended")
	fw.scrollEnd(fakeEv{})

	fw.scrollBegin(fakeEv{})
	fw.scrollBy(0, 1, gdkScrollUnitSurface, fakeEv{})
	fw.scrollEnd(fakeEv{})
	fw.decelerate(12, -16) // speed 20
	fw.decelerate(0, 1000) // only the first decelerate counts
	check("slow", "scroll 0,-1 precise began", "scroll 0,0 precise ended")

	fw.scrollBegin(fakeEv{})
	fw.scrollBy(0, 1, gdkScrollUnitSurface, fakeEv{})
	fw.scrollEnd(fakeEv{})
	fw.decelerate(math.NaN(), math.Inf(1))
	check("non-finite", "scroll 0,-1 precise began", "scroll 0,0 precise ended")

	// A second decelerate does not restart a running momentum.
	startMomentum(t, fw, h, 0, 1000)
	m := sf.scr.mom
	fw.decelerate(0, -5000)
	if sf.scr.mom != m {
		t.Errorf("second decelerate changed the momentum: %+v, was %+v", sf.scr.mom, m)
	}
	wantLog(t, "window calls", fw.take())
}

// TestScrollMomentumInterrupted: a scroll-begin, a scroll event and a
// pointer press end the momentum with a zero Momentum PhaseEnded first.
func TestScrollMomentumInterrupted(t *testing.T) {
	for _, c := range []struct {
		name string
		do   func(fw *fakeWindow)
		want []string
	}{
		{"scroll-begin", func(fw *fakeWindow) {
			fw.scrollBegin(fakeEv{})
			fw.scrollBy(0, 5, gdkScrollUnitSurface, fakeEv{})
		}, []string{"scroll 0,0 precise mom-ended at 10,10", "scroll 0,-5 precise began at 10,10"}},
		{"wheel", func(fw *fakeWindow) {
			fw.scrollBy(0, 1, gdkScrollUnitWheel, fakeEv{state: gdkControlMask})
		}, []string{"scroll 0,0 precise mom-ended at 10,10", "scroll 0,-3 at 10,10 m=2"}},
		{"precise", func(fw *fakeWindow) {
			fw.scrollBy(0, 1, gdkScrollUnitSurface, fakeEv{})
		}, []string{"scroll 0,0 precise mom-ended at 10,10", "scroll 0,-1 precise at 10,10"}},
		{"zero wheel", func(fw *fakeWindow) {
			fw.scrollBy(0, 0, gdkScrollUnitWheel, fakeEv{})
		}, []string{"scroll 0,0 precise mom-ended at 10,10"}},
		{"press", func(fw *fakeWindow) {
			fw.ptrPressed(1, 10, 10, at(9000, 1))
		}, []string{"scroll 0,0 precise mom-ended at 10,10", "down 10,10 b=1 held=1 n=1"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			sf, fw, h, f := openScroll(t)
			startMomentum(t, fw, h, 0, 2000)
			f.frame(t)
			f.frame(t)
			if got := takeScroll(h); len(got) != 2 {
				t.Fatalf("two frames: %q", got)
			}
			var evs []shell.ScrollEvent
			h.onScroll = func(se shell.ScrollEvent) { evs = append(evs, se) }
			h.times = nil
			c.do(fw)
			wantLog(t, "interrupted", takeScroll(h), c.want...)
			if len(evs) > 0 && (evs[0].Delta != (driver.Point{}) || evs[0].Phase != shell.PhaseNone) {
				t.Errorf("the momentum's end %+v", evs[0])
			}
			if sf.scr.mom.on {
				t.Error("still running")
			}
			runFrames(t, f, 10)
			wantLog(t, "frames after", takeScroll(h))
			fw.decelerate(0, 2000)
			if sf.scr.mom.on {
				t.Error("decelerate after an interruption started a momentum")
			}
		})
	}
}

// TestScrollMomentumTimes: the momentum's events, an interruption's end
// and the input events after them never go backwards in time, although a
// frame's target time is ahead of the clock.
func TestScrollMomentumTimes(t *testing.T) {
	for _, c := range []struct {
		name string
		do   func(fw *fakeWindow)
	}{
		{"wheel", func(fw *fakeWindow) { fw.scrollBy(0, 1, gdkScrollUnitWheel, fakeEv{}) }},
		{"precise", func(fw *fakeWindow) { fw.scrollBy(0, 1, gdkScrollUnitSurface, fakeEv{}) }},
		{"scroll-begin", func(fw *fakeWindow) {
			fw.scrollBegin(fakeEv{})
			fw.scrollBy(0, 1, gdkScrollUnitSurface, fakeEv{})
		}},
		{"press", func(fw *fakeWindow) { fw.ptrPressed(1, 10, 10, fakeEv{button: 1}) }},
		{"press with an old time", func(fw *fakeWindow) { fw.ptrPressed(1, 10, 10, at(1, 1)) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, fw, h, f := openScroll(t)
			startMomentum(t, fw, h, 0, 2000)
			h.times = nil
			f.frame(t)
			f.frame(t)
			if len(h.times) != 2 {
				t.Fatalf("%d momentum events in two frames", len(h.times))
			}
			if now := f.s.clock.now(); h.times[1] <= now {
				t.Errorf("the frame's time %v is not ahead of now %v: the test shows nothing", h.times[1], now)
			}
			c.do(fw)
			got := takeScroll(h)
			if len(got) < 4 || !strings.HasPrefix(got[2], "scroll 0,0 precise mom-ended") {
				t.Fatalf("events %q", got)
			}
			for i := 1; i < len(h.times); i++ {
				if h.times[i] < h.times[i-1] {
					t.Errorf("times went backwards: %v (%q)", h.times, got)
				}
			}
		})
	}
}

// TestScrollMomentumPhases checks the interrupted momentum's earlier shape:
// Began then Changed on the first two frames.
func TestScrollMomentumPhases(t *testing.T) {
	_, fw, h, f := openScroll(t)
	startMomentum(t, fw, h, 0, 2000)
	f.frame(t)
	f.frame(t)
	got := takeScroll(h)
	if len(got) != 2 || !strings.HasSuffix(got[0], "precise mom-began at 10,10") ||
		!strings.HasSuffix(got[1], "precise mom-changed at 10,10") {
		t.Errorf("first two frames: %q", got)
	}
}

// TestScrollMomentumClose: a handler that closes the surface inside a
// momentum event, or inside the end of an interrupted one, stops it all.
func TestScrollMomentumClose(t *testing.T) {
	for _, ph := range []shell.ScrollPhase{shell.PhaseBegan, shell.PhaseChanged, shell.PhaseEnded} {
		sf, fw, h, f := openScrollHidden(t)
		startMomentum(t, fw, h, 0, 1000)
		n := 0
		h.onScroll = func(se shell.ScrollEvent) {
			if se.Momentum == ph && n == 0 {
				n++
				sf.Close()
			}
		}
		runFrames(t, f, 1000)
		got := takeScroll(h)
		if len(got) < 2 || got[len(got)-1] != "closed" || n != 1 {
			t.Fatalf("close in %v: %q", ph, got)
		}
		fw.scrollBy(0, 1, gdkScrollUnitWheel, fakeEv{})
		fw.ptrPressed(1, 10, 10, at(9000, 1))
		runFrames(t, f, 10)
		wantLog(t, "after the close", takeScroll(h))
		if c := fw.take(); len(c) != 1 || c[0] != "destroy" {
			t.Errorf("close in %v: window calls %q", ph, c)
		}
	}

	for _, c := range []struct {
		name string
		do   func(fw *fakeWindow)
	}{
		{"scroll-begin", func(fw *fakeWindow) { fw.scrollBegin(fakeEv{}) }},
		{"scroll", func(fw *fakeWindow) { fw.scrollBy(0, 1, gdkScrollUnitSurface, fakeEv{}) }},
		{"press", func(fw *fakeWindow) { fw.ptrPressed(1, 10, 10, at(9000, 1)) }},
	} {
		sf, fw, h, f := openScroll(t)
		startMomentum(t, fw, h, 0, 1000)
		f.frame(t)
		takeScroll(h)
		h.onScroll = func(se shell.ScrollEvent) {
			if se.Momentum == shell.PhaseEnded {
				sf.Close()
			}
		}
		c.do(fw)
		fw.scrollBy(0, 1, gdkScrollUnitSurface, fakeEv{})
		fw.scrollEnd(fakeEv{})
		runFrames(t, f, 10)
		wantLog(t, "close in the end of "+c.name, takeScroll(h), "scroll 0,0 precise mom-ended at 10,10", "closed")
		wantLog(t, "window calls ("+c.name+")", fw.take(), "destroy")
	}
}

// TestScrollDuringCreate: the scroll signals during the window's creation
// deliver nothing and leave no gesture behind.
func TestScrollDuringCreate(t *testing.T) {
	_, f := newSurfaceTest(t)
	h := &sfHandler{}
	f.during = func(fw *fakeWindow) {
		fw.scrollBegin(fakeEv{})
		if r := fw.scrollBy(0, 1, gdkScrollUnitSurface, fakeEv{}); r != gtk.True {
			t.Errorf("scroll during creation returned %d", r)
		}
		fw.scrollBy(0, 1, gdkScrollUnitWheel, fakeEv{})
		fw.scrollEnd(fakeEv{})
		fw.decelerate(0, 1000)
	}
	sf, fw := f.open(t, sfResizable, h)
	f.during = nil
	wantLog(t, "during creation", takeScroll(h))
	if sf.scr != (scrollState{}) {
		t.Errorf("scroll state after creation %+v", sf.scr)
	}
	fw.decelerate(0, 1000)
	fw.scrollBy(0, 1, gdkScrollUnitSurface, fakeEv{})
	fw.scrollEnd(fakeEv{})
	wantLog(t, "after creation", takeScroll(h), "scroll 0,-1 precise")
}

// TestScrollAfterClose: after Close neither the signals nor the peer
// methods deliver anything or use the window; a running momentum stops
// silently.
func TestScrollAfterClose(t *testing.T) {
	sf, fw, h, f := openScroll(t)
	startMomentum(t, fw, h, 0, 1000)
	f.frame(t)
	sf.Close()
	fw.take()
	h.take()
	runFrames(t, f, 10)
	fw.scrollBegin(fakeEv{})
	if r := fw.scrollBy(0, 1, gdkScrollUnitSurface, fakeEv{}); r != gtk.False {
		t.Errorf("scroll with a stale handle returned %d, want FALSE", r)
	}
	fw.scrollEnd(fakeEv{})
	fw.decelerate(0, 1000)
	sf.voidSignal(kindScrollBegin, testScroll)
	if !sf.boolDoubleDouble(kindScroll, testScroll, 0, 1) {
		t.Error("boolDoubleDouble after Close returned false")
	}
	sf.voidSignal(kindScrollEnd, testScroll)
	sf.doubleDouble(kindScrollDecelerate, testScroll, 0, 1000)
	sf.stepMomentum(time.Hour)
	f.s.stepMomentum(time.Hour)
	wantLog(t, "handler calls after Close", h.take())
	wantLog(t, "window calls after Close", fw.take())
}

// TestScrollPanic: a panicking scroll handler is re-raised by rethrow as
// the trampoline's *PanicError; the scroll state stays usable.
func TestScrollPanic(t *testing.T) {
	sf, fw, h, f := openScroll(t)
	h.onScroll = func(se shell.ScrollEvent) {
		if se.Phase == shell.PhaseBegan {
			panic("in PhaseBegan")
		}
	}
	fw.scrollBegin(fakeEv{})
	v := catchPanic(func() { fw.scrollBy(0, 1, gdkScrollUnitSurface, fakeEv{}) })
	if pe, ok := v.(*PanicError); !ok || pe.Value != "in PhaseBegan" || pe.Callback != nameSigBoolDoubleDouble {
		t.Fatalf("scroll panicked with %#v, want the *PanicError of sigBoolDoubleDouble", v)
	}
	if pendingPanic != nil || sf.dead {
		t.Fatalf("after rethrow: pending %v, dead %v", pendingPanic, sf.dead)
	}
	h.onScroll = nil
	fw.scrollBy(0, 2, gdkScrollUnitSurface, fakeEv{})
	fw.scrollEnd(fakeEv{})
	fw.decelerate(0, 1000)
	wantLog(t, "after a panic in Began", takeScroll(h), "scroll 0,-1 precise began",
		"scroll 0,-2 precise changed", "scroll 0,0 precise ended")
	if !sf.scr.mom.on {
		t.Fatal("no momentum after the panic")
	}

	// A panic in a momentum event, in the frame's paint pass: trapped by
	// the tick callback; a later scroll ends the momentum first.
	h.onScroll = func(se shell.ScrollEvent) {
		if se.Momentum == shell.PhaseBegan {
			panic("in mom-began")
		}
	}
	f.frame(t)
	if pe, ok := rethrown().(*PanicError); !ok || pe.Value != "in mom-began" || pe.Callback != nameTick {
		t.Fatalf("frame panicked with %#v, want the *PanicError of the tick callback", pe)
	}
	h.onScroll = nil
	fw.scrollBy(0, 1, gdkScrollUnitWheel, fakeEv{})
	got := takeScroll(h)
	if len(got) != 3 || !strings.HasSuffix(got[0], " precise mom-began") ||
		got[1] != "scroll 0,0 precise mom-ended" || got[2] != "scroll 0,-3" {
		t.Errorf("after a panic in mom-began: %q, want mom-began, a zero mom-ended, the wheel", got)
	}
	if pendingPanic != nil || sf.scr.mom.on {
		t.Errorf("pending %v, running %v", pendingPanic, sf.scr.mom.on)
	}
	runFrames(t, f, 10)
	wantLog(t, "frames after", takeScroll(h))
}
