package aqua

import (
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// scrollWindow: root 0,0 300x100 with the scrollable a at 0,0 150x100
// holding in at 10,10 50x50 (not scrollable unless told), and the
// scrollable b at 150,0 150x100.
type scrollWindow struct {
	log            []string
	root, a, in, b *probe
	r              *Router
}

func newScrollWindow(t *testing.T) *scrollWindow {
	t.Helper()
	w := &scrollWindow{}
	w.root = newProbe(&w.log, "root", 0, 0, 300, 100)
	w.a = newProbe(&w.log, "a", 0, 0, 150, 100, "scroll")
	w.in = newProbe(&w.log, "in", 10, 10, 50, 50)
	w.b = newProbe(&w.log, "b", 150, 0, 150, 100, "scroll")
	w.root.SetChildren(w.a, w.b)
	w.a.AddChild(w.in)
	w.r = NewRouter(w.root)
	return w
}

// gesture returns a precise gesture event at x,y.
func gesture(x, y float64, ph shell.ScrollPhase) shell.ScrollEvent {
	return shell.ScrollEvent{Pos: driver.Point{X: x, Y: y}, Delta: driver.Point{Y: 1}, Precise: true, Phase: ph}
}

// momentum returns an OS momentum event at x,y.
func momentum(x, y float64, ph shell.ScrollPhase) shell.ScrollEvent {
	return shell.ScrollEvent{Pos: driver.Point{X: x, Y: y}, Delta: driver.Point{Y: 1}, Precise: true, Momentum: ph}
}

func wheel(x, y float64) shell.ScrollEvent {
	return shell.ScrollEvent{Pos: driver.Point{X: x, Y: y}, Delta: driver.Point{Y: 1}}
}

func wantLatched(t *testing.T, r *Router, want Element) {
	t.Helper()
	if got := r.Latched(); got != want {
		t.Fatalf("latched %v, want %v", got, want)
	}
}

func wantHandled(t *testing.T, r *Router, ev shell.ScrollEvent, want bool) {
	t.Helper()
	if got := r.Event(ev); got != want {
		t.Fatalf("event %+v handled %v, want %v", ev, got, want)
	}
}

func TestScrollLatchStaysWithFirstElement(t *testing.T) {
	w := newScrollWindow(t)
	r := w.r
	wantLatched(t, r, nil)
	wantHandled(t, r, gesture(100, 50, shell.PhaseBegan), true)
	checkLog(t, &w.log, "a scroll 100,50")
	wantLatched(t, r, w.a)
	// The pointer moves over b, then outside the window: a keeps the gesture,
	// in its own coordinates.
	wantHandled(t, r, gesture(200, 50, shell.PhaseChanged), true)
	wantHandled(t, r, gesture(500, 500, shell.PhaseChanged), true)
	wantHandled(t, r, gesture(200, 50, shell.PhaseEnded), true)
	checkLog(t, &w.log, "a scroll 200,50", "a scroll 500,500", "a scroll 200,50")
	wantLatched(t, r, w.a) // kept for the OS momentum
	// The latched element's result is returned, without bubbling.
	w.a.handles["scroll"] = false
	w.root.handles["scroll"] = true
	wantHandled(t, r, momentum(200, 50, shell.PhaseBegan), false)
	checkLog(t, &w.log, "a scroll 200,50")
}

func TestScrollLatchUnhandledBegan(t *testing.T) {
	w := newScrollWindow(t)
	r := w.r
	w.a.handles["scroll"] = false
	wantHandled(t, r, gesture(100, 50, shell.PhaseBegan), false)
	checkLog(t, &w.log, "a scroll 100,50", "root scroll 100,50")
	wantLatched(t, r, nil)
	// The rest of the gesture routes by the pointer.
	wantHandled(t, r, gesture(200, 50, shell.PhaseChanged), true)
	checkLog(t, &w.log, "b scroll 50,50")
	wantLatched(t, r, nil)
	wantHandled(t, r, gesture(100, 50, shell.PhaseEnded), false)
	checkLog(t, &w.log, "a scroll 100,50", "root scroll 100,50")
}

func TestScrollLatchParentOfDecliningChild(t *testing.T) {
	w := newScrollWindow(t)
	r := w.r
	wantHandled(t, r, gesture(20, 20, shell.PhaseBegan), true)
	checkLog(t, &w.log, "in scroll 10,10", "a scroll 20,20")
	wantLatched(t, r, w.a)
	// in would accept now (an inner scroller reaching its edge and back),
	// but the gesture stays with a.
	w.in.handles["scroll"] = true
	wantHandled(t, r, gesture(20, 20, shell.PhaseChanged), true)
	wantHandled(t, r, gesture(20, 20, shell.PhaseEnded), true)
	checkLog(t, &w.log, "a scroll 20,20", "a scroll 20,20")
	// The next gesture goes to in.
	wantHandled(t, r, gesture(20, 20, shell.PhaseBegan), true)
	checkLog(t, &w.log, "in scroll 10,10")
	wantLatched(t, r, w.in)
}

func TestScrollLatchMomentum(t *testing.T) {
	w := newScrollWindow(t)
	r := w.r
	r.Event(gesture(100, 50, shell.PhaseBegan))
	r.Event(gesture(100, 50, shell.PhaseEnded))
	checkLog(t, &w.log, "a scroll 100,50", "a scroll 100,50")
	for _, ph := range []shell.ScrollPhase{shell.PhaseBegan, shell.PhaseChanged, shell.PhaseChanged} {
		wantHandled(t, r, momentum(200, 50, ph), true)
		wantLatched(t, r, w.a)
	}
	wantHandled(t, r, momentum(200, 50, shell.PhaseEnded), true)
	checkLog(t, &w.log, "a scroll 200,50", "a scroll 200,50", "a scroll 200,50", "a scroll 200,50")
	wantLatched(t, r, nil)
	// Momentum without a latch routes by the pointer.
	wantHandled(t, r, momentum(200, 50, shell.PhaseChanged), true)
	checkLog(t, &w.log, "b scroll 50,50")
}

func TestScrollLatchEndedBy(t *testing.T) {
	w := newScrollWindow(t)
	r := w.r
	// A wheel event after a gesture routes by the pointer and ends the latch.
	r.Event(gesture(100, 50, shell.PhaseBegan))
	r.Event(gesture(100, 50, shell.PhaseEnded))
	wantHandled(t, r, wheel(200, 50), true)
	checkLog(t, &w.log, "a scroll 100,50", "a scroll 100,50", "b scroll 50,50")
	wantLatched(t, r, nil)
	// So does a wheel event during a gesture (a mouse used meanwhile).
	r.Event(gesture(100, 50, shell.PhaseBegan))
	r.Event(wheel(200, 50))
	wantLatched(t, r, nil)
	r.Event(gesture(200, 50, shell.PhaseChanged))
	checkLog(t, &w.log, "a scroll 100,50", "b scroll 50,50", "b scroll 50,50")
	// A new PhaseBegan re-latches by the pointer, with or without momentum
	// in between.
	r.Event(gesture(100, 50, shell.PhaseBegan))
	wantLatched(t, r, w.a)
	r.Event(gesture(100, 50, shell.PhaseEnded))
	r.Event(gesture(200, 50, shell.PhaseBegan))
	wantLatched(t, r, w.b)
	r.Event(gesture(200, 50, shell.PhaseChanged))
	r.Event(gesture(100, 50, shell.PhaseChanged))
	checkLog(t, &w.log, "a scroll 100,50", "a scroll 100,50", "b scroll 50,50", "b scroll 50,50", "b scroll -50,50")
	// A PhaseChanged or PhaseEnded after the gesture's PhaseEnded is not
	// part of it: it ends the latch and routes by the pointer.
	r.Event(gesture(100, 50, shell.PhaseEnded))
	wantLatched(t, r, w.b)
	wantHandled(t, r, gesture(100, 50, shell.PhaseChanged), true)
	checkLog(t, &w.log, "b scroll -50,50", "a scroll 100,50")
	wantLatched(t, r, nil)
}

func TestScrollLatchFollowsTree(t *testing.T) {
	cases := []struct {
		name string
		drop func(w *scrollWindow)
	}{
		{"removed", func(w *scrollWindow) { w.root.RemoveChild(w.a) }},
		{"hidden", func(w *scrollWindow) { w.a.SetHidden(true) }},
		{"disabled", func(w *scrollWindow) { w.a.SetEnabled(false) }},
		{"ancestor disabled", func(w *scrollWindow) { w.a.SetEnabled(false) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := newScrollWindow(t)
			r := w.r
			target := w.a
			if tc.name == "ancestor disabled" {
				w.in.handles["scroll"] = true
				target = w.in
			}
			r.Event(gesture(20, 20, shell.PhaseBegan))
			wantLatched(t, r, target)
			w.log = nil
			tc.drop(w)
			wantLatched(t, r, nil)
			checkLog(t, &w.log) // no event for losing the latch
			// The rest of the gesture and its momentum are dropped, even over
			// b, which would take them.
			wantHandled(t, r, gesture(200, 50, shell.PhaseChanged), false)
			wantHandled(t, r, gesture(200, 50, shell.PhaseEnded), false)
			wantHandled(t, r, momentum(200, 50, shell.PhaseBegan), false)
			wantHandled(t, r, momentum(200, 50, shell.PhaseEnded), false)
			checkLog(t, &w.log)
			// After the momentum ended, routing is back to normal.
			wantHandled(t, r, momentum(200, 50, shell.PhaseChanged), true)
			checkLog(t, &w.log, "b scroll 50,50")
			// A new gesture starts afresh right after a drop, too.
			r.Event(gesture(200, 50, shell.PhaseBegan))
			wantLatched(t, r, w.b)
			w.b.SetHidden(true)
			wantHandled(t, r, gesture(200, 50, shell.PhaseChanged), false)
			w.b.SetHidden(false)
			wantHandled(t, r, gesture(200, 50, shell.PhaseBegan), true)
			wantLatched(t, r, w.b)
			checkLog(t, &w.log, "b scroll 50,50", "b scroll 50,50")
		})
	}
}

func TestScrollLatchDroppedByHandler(t *testing.T) {
	w := newScrollWindow(t)
	r := w.r
	// The latched element disables itself while handling a PhaseChanged.
	r.Event(gesture(100, 50, shell.PhaseBegan))
	w.a.onEvent = func(Event) { w.a.SetEnabled(false) }
	wantHandled(t, r, gesture(100, 50, shell.PhaseChanged), true)
	wantLatched(t, r, nil)
	wantHandled(t, r, gesture(200, 50, shell.PhaseChanged), false)
	checkLog(t, &w.log, "a scroll 100,50", "a scroll 100,50")
	// A handler that hides its element while taking PhaseBegan latches
	// nothing, and the rest of that gesture is dropped.
	w.b.onEvent = func(Event) { w.b.SetHidden(true) }
	wantHandled(t, r, gesture(200, 50, shell.PhaseBegan), true)
	wantLatched(t, r, nil)
	w.b.SetHidden(false)
	w.b.onEvent = nil
	wantHandled(t, r, gesture(200, 50, shell.PhaseChanged), false)
	wantHandled(t, r, gesture(200, 50, shell.PhaseEnded), false)
	checkLog(t, &w.log, "b scroll 50,50")
	wantHandled(t, r, wheel(200, 50), true)
	checkLog(t, &w.log, "b scroll 50,50")
}

// TestScrollLatchFakeSurface drives the latch through a window on the fake
// shell: a gesture that starts over a and moves over b stays with a, the
// shell's momentum after it too; a wheel notch over b then goes to b, and a
// swipe over b latches b.
func TestScrollLatchFakeSurface(t *testing.T) {
	s := startShell(t)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Title: "W", Size: driver.Size{W: 300, H: 200}, Style: allStyles})
	var log []string
	box := newProbe(&log, "box", 0, 0, 300, 172)
	a := newProbe(&log, "a", 0, 0, 150, 100, "scroll")
	b := newProbe(&log, "b", 150, 0, 150, 100, "scroll")
	box.SetChildren(a, b)
	w.SetContent(contentWidget{box})
	r := w.Router()
	overA, overB := ToRoot(a, pt(10, 10)), ToRoot(b, pt(10, 10))
	scrolls := func(want ...string) {
		t.Helper()
		var got []string
		for _, e := range log {
			if strings.Contains(e, " scroll ") {
				got = append(got, e)
			}
		}
		log = got
		checkLog(t, &log, want...)
	}

	sf.Move(overA)
	sf.Send(shell.ScrollEvent{Pos: overA, Delta: pt(0, 1), Precise: true, Phase: shell.PhaseBegan})
	if r.Latched() != Element(a) {
		t.Fatalf("latched %v after the gesture began over a", r.Latched())
	}
	sf.Move(overB)
	sf.Send(shell.ScrollEvent{Pos: overB, Delta: pt(0, 1), Precise: true, Phase: shell.PhaseChanged})
	sf.Send(shell.ScrollEvent{Pos: overB, Precise: true, Phase: shell.PhaseEnded})
	sf.Momentum(pt(0, 2), pt(0, 1))
	scrolls("a scroll 10,10", "a scroll 160,10", "a scroll 160,10", "a scroll 160,10", "a scroll 160,10", "a scroll 160,10")
	if r.Latched() != nil {
		t.Fatalf("latched %v after the momentum ended", r.Latched())
	}
	sf.Wheel(overB, pt(0, -1))
	scrolls("b scroll 10,10")
	sf.Swipe(overB, pt(0, 1), pt(0, 2))
	scrolls("b scroll 10,10", "b scroll 10,10", "b scroll 10,10")
	if r.Latched() != Element(b) {
		t.Fatalf("latched %v after a swipe over b", r.Latched())
	}
}
