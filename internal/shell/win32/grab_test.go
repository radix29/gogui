//go:build windows

package win32

import (
	"math"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// The grab tests (S5.5c) send the messages the capture and the focus
// would deliver, synchronously and without pumping, so the real cursor's
// own moves (queued input) do not mix in.

// signedPx is the pixel of a point at scale, negative ones included.
func signedPx(v, scale float64) int32 { return int32(math.Round(v * scale)) }

// popLP is the client lParam of the popup point (x, y).
func popLP(p *popup, x, y float64) uintptr {
	return lparamXY(w32.Point{X: signedPx(x, p.px()), Y: signedPx(y, p.px())})
}

// parentLP is the client lParam of the parent point (x, y).
func parentLP(sf *surface, x, y float64) uintptr {
	return lparamXY(w32.Point{X: signedPx(x, sf.px()), Y: signedPx(y, sf.px())})
}

// screenLP is the screen lParam of the parent point (x, y).
func screenLP(t *testing.T, sf *surface, x, y float64) uintptr {
	t.Helper()
	pt := w32.Point{X: signedPx(x, sf.px()), Y: signedPx(y, sf.px())}
	if err := w32.ClientToScreen(sf.hwnd, &pt); err != nil {
		t.Fatal(err)
	}
	return lparamXY(pt)
}

// sendTo sends a message and re-raises a handler's panic.
func sendTo(hwnd windows.HWND, msg uint32, wp, lp uintptr) uintptr {
	r := w32.SendMessage(hwnd, msg, wp, lp)
	rethrow()
	return r
}

// popEvents returns the popup's pointer events and forgets every event.
func popEvents(t *testing.T, r *popRec) []shell.PointerEvent {
	t.Helper()
	var pes []shell.PointerEvent
	for _, ev := range r.events {
		pe, ok := ev.(shell.PointerEvent)
		if !ok {
			t.Fatalf("unexpected popup event %T %+v", ev, ev)
		}
		pes = append(pes, pe)
	}
	r.events = nil
	return pes
}

// menuCfg is a grab popup at (40, 30) of the parent, 120×80.
var menuCfg = shell.PopupConfig{Anchor: driver.Rect{X: 40, Y: 30, W: 100, H: 22},
	Size: driver.Size{W: 120, H: 80}, Grab: true}

const (
	pMove  = shell.PointerMove
	pDown  = shell.PointerDown
	pUp    = shell.PointerUp
	pEnter = shell.PointerEnter
	pLeave = shell.PointerLeave
)

// TestShownPopupGrabPointer: opening a grab popup ends the parent's press
// as a capture cancel and hands the held button to the popup, which holds
// the capture; the popup's pointer events follow the enter, move, leave
// and held-button rules in its points; a press outside is eaten, its
// release too, and dismisses it once, releasing the capture first.
func TestShownPopupGrabPointer(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{}
		sf := shownParent(t, s, r)
		defer sf.Close()
		L := shell.ButtonPrimary
		ph := sf.hwnd

		// The parent holds the primary button (a press on a pop-up button).
		sendTo(ph, w32.WM_MOUSEMOVE, 0, parentLP(sf, 50, 40))
		sendTo(ph, w32.WM_LBUTTONDOWN, w32.MK_LBUTTON, parentLP(sf, 50, 40))
		r.events = nil
		var log []string
		pr := &popRec{log: &log}
		p := newPopup(t, s, sf, menuCfg, pr)
		h := p.hwnd
		want := []pev{{pUp, L, 0, 0}, {pLeave, 0, 0, 0}}
		if got := compact(pointerEvents(t, r)); !samePevs(got, want) {
			t.Fatalf("parent on open:\n got %+v\nwant %+v", got, want)
		}
		if w32.GetCapture() != p.hwnd || sf.buttons != 0 || p.in.buttons != L || s.grab != p {
			t.Fatalf("capture %#x (popup %#x), parent buttons %v, popup buttons %v",
				w32.GetCapture(), p.hwnd, sf.buttons, p.in.buttons)
		}

		// Press-drag-release: the held button's release on the popup has
		// no press before it.
		sendTo(h, w32.WM_MOUSEMOVE, w32.MK_LBUTTON, popLP(p, 10, 10))
		sendTo(h, w32.WM_LBUTTONUP, 0, popLP(p, 10, 10))
		pes := popEvents(t, pr)
		want = []pev{{pEnter, 0, L, 0}, {pMove, 0, L, 0}, {pUp, L, 0, 0}}
		if got := compact(pes); !samePevs(got, want) {
			t.Fatalf("drag release:\n got %+v\nwant %+v", got, want)
		}
		if pes[1].Pos != (driver.Point{X: 10, Y: 10}) {
			t.Errorf("move at %v, want (10, 10) in the popup's points", pes[1].Pos)
		}

		// Out and in again, a click, a drag out with the button held.
		sendTo(h, w32.WM_MOUSEMOVE, 0, popLP(p, -30, 10))
		sendTo(h, w32.WM_MOUSEMOVE, 0, popLP(p, 20, 20))
		sendTo(h, w32.WM_LBUTTONDOWN, w32.MK_LBUTTON, popLP(p, 20, 20))
		sendTo(h, w32.WM_LBUTTONUP, 0, popLP(p, 20, 20))
		sendTo(h, w32.WM_LBUTTONDOWN, w32.MK_LBUTTON, popLP(p, 20, 20))
		sendTo(h, w32.WM_MOUSEMOVE, w32.MK_LBUTTON, popLP(p, 200, 10))
		sendTo(h, w32.WM_LBUTTONUP, 0, popLP(p, 200, 10))
		want = []pev{{pLeave, 0, 0, 0}, {pEnter, 0, 0, 0}, {pMove, 0, 0, 0},
			{pDown, L, L, 1}, {pUp, L, 0, 1}, {pDown, L, L, 2}, {pMove, 0, L, 0}, {pUp, L, 0, 2},
			{pLeave, 0, 0, 0}}
		if got := compact(popEvents(t, pr)); !samePevs(got, want) {
			t.Fatalf("clicks:\n got %+v\nwant %+v", got, want)
		}
		if len(r.events) != 0 || pr.dismissed != 0 {
			t.Fatalf("parent events %v, dismissed %d", r.events, pr.dismissed)
		}

		// A press outside: eaten and dismissing, the capture gone first.
		capAt := windows.HWND(1)
		pr.onDismissed = func() { capAt = w32.GetCapture() }
		sendTo(h, w32.WM_RBUTTONDOWN, w32.MK_RBUTTON, popLP(p, -30, 10))
		if pr.dismissed != 1 || s.grab != nil || capAt == h || w32.GetCapture() == p.hwnd || p.dead {
			t.Fatalf("outside press: dismissed %d, grab %v, capture at Dismissed %#x", pr.dismissed, s.grab != nil, capAt)
		}
		sendTo(h, w32.WM_RBUTTONUP, 0, popLP(p, -30, 10))
		sendTo(h, w32.WM_LBUTTONDOWN, w32.MK_LBUTTON, popLP(p, 20, 20))
		sendTo(h, w32.WM_MOUSEMOVE, 0, popLP(p, 30, 20))
		sendTo(h, w32.WM_CANCELMODE, 0, 0)
		if len(pr.events) != 0 || pr.dismissed != 1 || len(r.events) != 0 {
			t.Errorf("after the dismissal: popup events %v, dismissed %d, parent events %v",
				pr.events, pr.dismissed, r.events)
		}
		if sf.keySink() != sf.h {
			t.Error("keys still go to the dismissed popup")
		}
		p.Close()
		if pr.closed != 1 || pr.dismissed != 1 {
			t.Errorf("Close: closed %d, dismissed %d", pr.closed, pr.dismissed)
		}
	})
}

// TestShownPopupGrabParentInput: during the grab the parent's keys,
// characters and wheel go to the popup, and mouse messages that reach the
// parent are the popup's in its points; a non-client press on the parent's
// caption is an outside press (no window drag starts). Afterwards the
// parent gets its keys again.
func TestShownPopupGrabParentInput(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{}
		sf := shownParent(t, s, r)
		defer sf.Close()
		ph := sf.hwnd
		pr := &popRec{handled: true}
		p := newPopup(t, s, sf, menuCfg, pr)
		h := p.hwnd
		r.events = nil

		sendTo(ph, w32.WM_KEYDOWN, w32.VK_DOWN, 1)
		sendTo(ph, w32.WM_KEYUP, w32.VK_DOWN, 1|3<<30)
		sendTo(ph, w32.WM_CHAR, 'a', 1)
		if len(pr.events) != 3 || len(r.events) != 0 {
			t.Fatalf("keys: popup %+v, parent %+v", pr.events, r.events)
		}
		if k, ok := pr.events[0].(shell.KeyEvent); !ok || !k.Down || k.Key != driver.KeyDown {
			t.Errorf("key-down %+v", pr.events[0])
		}
		if k, ok := pr.events[1].(shell.KeyEvent); !ok || k.Down || k.Key != driver.KeyDown {
			t.Errorf("key-up %+v", pr.events[1])
		}
		if te, ok := pr.events[2].(shell.TextEvent); !ok || te.Text != "a" {
			t.Errorf("text %+v", pr.events[2])
		}
		pr.events = nil

		// Mouse messages sent to the parent are the popup's.
		L := shell.ButtonPrimary
		sendTo(ph, w32.WM_MOUSEMOVE, 0, parentLP(sf, 60, 50))
		sendTo(ph, w32.WM_LBUTTONDOWN, w32.MK_LBUTTON, parentLP(sf, 60, 50))
		sendTo(ph, w32.WM_LBUTTONUP, 0, parentLP(sf, 60, 50))
		pes := popEvents(t, pr)
		want := []pev{{pEnter, 0, 0, 0}, {pMove, 0, 0, 0}, {pDown, L, L, 1}, {pUp, L, 0, 1}}
		if got := compact(pes); !samePevs(got, want) || pes[1].Pos != (driver.Point{X: 20, Y: 20}) {
			t.Fatalf("parent mouse:\n got %+v at %v\nwant %+v", got, pes[1].Pos, want)
		}

		// The wheel, to the parent (the focus) or to the popup (the
		// capture): over the popup it is the popup's, elsewhere dropped.
		notch := uintptr(w32.WHEEL_DELTA) << 16
		sendTo(ph, w32.WM_MOUSEWHEEL, notch, screenLP(t, sf, 60, 50))
		sendTo(h, w32.WM_MOUSEWHEEL, notch, screenLP(t, sf, 70, 60))
		sendTo(ph, w32.WM_MOUSEWHEEL, notch, screenLP(t, sf, 10, 10))
		sendTo(h, w32.WM_MOUSEWHEEL, notch, screenLP(t, sf, 10, 10))
		if len(pr.events) != 2 || len(r.events) != 0 {
			t.Fatalf("wheel: popup %+v, parent %+v", pr.events, r.events)
		}
		for i, at := range []driver.Point{{X: 20, Y: 20}, {X: 30, Y: 30}} {
			se, ok := pr.events[i].(shell.ScrollEvent)
			if !ok || se.Pos != at || !(se.Delta.Y > 0) || se.Precise {
				t.Errorf("scroll %d: %+v, want a notch at %v", i, pr.events[i], at)
			}
		}
		pr.events = nil

		// A press on the parent's caption: an outside press.
		sendTo(ph, w32.WM_NCLBUTTONDOWN, w32.HTCAPTION, screenLP(t, sf, 5, 5))
		if pr.dismissed != 1 || s.grab != nil {
			t.Fatalf("caption press: dismissed %d", pr.dismissed)
		}
		want = []pev{{pLeave, 0, 0, 0}}
		if got := compact(popEvents(t, pr)); !samePevs(got, want) {
			t.Errorf("caption press, popup:\n got %+v\nwant %+v", got, want)
		}
		sendTo(ph, w32.WM_KEYDOWN, w32.VK_DOWN, 1)
		if len(pr.events) != 0 || len(r.events) != 1 {
			t.Errorf("keys after the dismissal: popup %+v, parent %+v", pr.events, r.events)
		}
		p.Close()
	})
}

// TestShownPopupDismissal: each dismissal message dismisses the grab popup
// once (a second one does nothing), a button held on the popup ends with
// its PointerUp and PointerLeave before Dismissed, and a z-order change of
// the parent is no dismissal. A popup closed from Dismissed is fine, and
// Close ends a grab with no events.
func TestShownPopupDismissal(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{}
		sf := shownParent(t, s, r)
		defer sf.Close()
		ph := sf.hwnd
		moveBy := func(dx int32) {
			wr, _ := w32.GetWindowRect(sf.hwnd)
			_ = w32.SetWindowPos(sf.hwnd, 0, wr.Left+dx, wr.Top, 0, 0,
				w32.SWP_NOSIZE|w32.SWP_NOZORDER|w32.SWP_NOACTIVATE)
			rethrow()
		}
		for _, c := range []struct {
			what string
			do   func(p *popup)
		}{
			{"outside press on the parent", func(*popup) {
				sendTo(ph, w32.WM_LBUTTONDOWN, w32.MK_LBUTTON, parentLP(sf, 5, 5))
				sendTo(ph, w32.WM_LBUTTONUP, 0, parentLP(sf, 5, 5))
			}},
			{"deactivation", func(*popup) { sendTo(ph, w32.WM_ACTIVATE, w32.WA_INACTIVE, 0) }},
			{"another application", func(*popup) { sendTo(ph, w32.WM_ACTIVATEAPP, 0, 0) }},
			{"cancel mode to the parent", func(*popup) { sendTo(ph, w32.WM_CANCELMODE, 0, 0) }},
			{"cancel mode to the popup", func(p *popup) { sendTo(p.hwnd, w32.WM_CANCELMODE, 0, 0) }},
			{"capture taken", func(*popup) {
				w32.SetCapture(sf.hwnd)
				rethrow()
				_ = w32.ReleaseCapture()
				rethrow()
			}},
			{"capture released", func(*popup) { _ = w32.ReleaseCapture(); rethrow() }},
			{"size-move loop", func(*popup) {
				sendTo(ph, w32.WM_ENTERSIZEMOVE, 0, 0)
				sendTo(ph, w32.WM_EXITSIZEMOVE, 0, 0)
			}},
			{"menu loop", func(*popup) {
				sendTo(ph, w32.WM_ENTERMENULOOP, 0, 0)
				sendTo(ph, w32.WM_EXITMENULOOP, 0, 0)
			}},
			{"parent moved", func(*popup) { moveBy(10) }},
			{"parent resized", func(*popup) { sf.SetSize(driver.Size{W: sf.size.W + 10, H: sf.size.H}) }},
		} {
			pr := &popRec{}
			p := newPopup(t, s, sf, menuCfg, pr)
			// A z-order change does not dismiss it.
			_ = w32.SetWindowPos(sf.hwnd, 0, 0, 0, 0, 0, w32.SWP_NOMOVE|w32.SWP_NOSIZE|w32.SWP_NOACTIVATE)
			rethrow()
			if pr.dismissed != 0 || w32.GetCapture() != p.hwnd {
				t.Fatalf("%s: dismissed %d before, capture %#x", c.what, pr.dismissed, w32.GetCapture())
			}
			c.do(p)
			if pr.dismissed != 1 || s.grab != nil || w32.GetCapture() == p.hwnd || p.dead {
				t.Errorf("%s: dismissed %d, grab %v, capture %#x", c.what, pr.dismissed, s.grab != nil, w32.GetCapture())
			}
			c.do(p)
			sendTo(ph, w32.WM_ACTIVATE, w32.WA_INACTIVE, 0)
			if pr.dismissed != 1 {
				t.Errorf("%s: dismissed %d after a second time", c.what, pr.dismissed)
			}
			p.Close()
			if pr.closed != 1 {
				t.Errorf("%s: closed %d", c.what, pr.closed)
			}
		}

		// A held button ends before Dismissed; closing from Dismissed.
		var log []string
		pr := &popRec{log: &log}
		p := newPopup(t, s, sf, menuCfg, pr)
		pr.onDismissed = func() { p.Close() }
		h := p.hwnd
		sendTo(h, w32.WM_MOUSEMOVE, 0, popLP(p, 10, 10))
		sendTo(h, w32.WM_LBUTTONDOWN, w32.MK_LBUTTON, popLP(p, 10, 10))
		log = nil
		sendTo(ph, w32.WM_CANCELMODE, 0, 0)
		if want := []string{"up", "leave", "dismissed", "closed"}; !equalStrings(log, want) {
			t.Errorf("held button at the dismissal: %v, want %v", log, want)
		}
		if up := pr.events[len(pr.events)-2].(shell.PointerEvent); up.Clicks != 0 || up.Buttons != 0 {
			t.Errorf("the cancelled release %+v, want Clicks 0", up)
		}
		if !p.dead || s.grab != nil || w32.GetCapture() == p.hwnd {
			t.Errorf("closed from Dismissed: dead %v, grab %v", p.dead, s.grab != nil)
		}

		// Close ends the grab without events; the parent is key again.
		log = nil
		pr = &popRec{log: &log}
		p = newPopup(t, s, sf, menuCfg, pr)
		sendTo(p.hwnd, w32.WM_MOUSEMOVE, 0, popLP(p, 10, 10))
		log = nil
		p.Close()
		if want := []string{"closed"}; !equalStrings(log, want) || w32.GetCapture() == p.hwnd || s.grab != nil ||
			sf.keySink() != sf.h {
			t.Errorf("Close during the grab: %v, capture %#x", log, w32.GetCapture())
		}

		// Minimizing dismisses once (it deactivates and moves).
		pr = &popRec{}
		p = newPopup(t, s, sf, menuCfg, pr)
		sf.Minimize()
		if pr.dismissed != 1 {
			t.Errorf("minimize: dismissed %d", pr.dismissed)
		}
		p.Close()
	})
}

// TestShownPopupNoGrab: a popup without Grab is transparent to the
// mouse (HTTRANSPARENT), takes none of its messages and no keys, never
// holds the capture and is never dismissed; the parent's input stays the
// parent's.
func TestShownPopupNoGrab(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{}
		sf := shownParent(t, s, r)
		defer sf.Close()
		ph := sf.hwnd
		pr := &popRec{}
		cfg := menuCfg
		cfg.Grab = false
		p := newPopup(t, s, sf, cfg, pr)
		h := p.hwnd
		if got := sendTo(h, w32.WM_NCHITTEST, 0, screenLP(t, sf, 60, 50)); got != htTransparent {
			t.Errorf("WM_NCHITTEST = %#x, want HTTRANSPARENT", got)
		}
		if w32.GetCapture() == p.hwnd || sf.keySink() != sf.h {
			t.Error("a tooltip took the capture or the keys")
		}
		sendTo(h, w32.WM_MOUSEMOVE, 0, popLP(p, 10, 10))
		sendTo(h, w32.WM_LBUTTONDOWN, w32.MK_LBUTTON, popLP(p, 10, 10))
		sendTo(h, w32.WM_LBUTTONUP, 0, popLP(p, 10, 10))
		sendTo(h, w32.WM_MOUSEWHEEL, uintptr(w32.WHEEL_DELTA)<<16, screenLP(t, sf, 60, 50))
		sendTo(ph, w32.WM_MOUSEMOVE, 0, parentLP(sf, 60, 50))
		sendTo(ph, w32.WM_KEYDOWN, w32.VK_DOWN, 1)
		sendTo(ph, w32.WM_ACTIVATE, w32.WA_INACTIVE, 0)
		sendTo(ph, w32.WM_CANCELMODE, 0, 0)
		if len(pr.events) != 0 || pr.dismissed != 0 {
			t.Errorf("tooltip: events %+v, dismissed %d", pr.events, pr.dismissed)
		}
		if len(r.events) != 3 { // enter, move, the key
			t.Errorf("parent events %+v, want enter, move and the key", r.events)
		}

		g := newPopup(t, s, sf, menuCfg, &popRec{})
		if got := sendTo(g.hwnd, w32.WM_NCHITTEST, 0, screenLP(t, sf, 60, 50)); got != w32.HTCLIENT {
			t.Errorf("grab popup WM_NCHITTEST = %#x, want HTCLIENT", got)
		}
		g.Close()
		p.Close()
	})
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestShownPopupGrabTrack: the grab's track area (S5.6d), a strip of the
// parent above the menu: converted into the popup's points; the capture's
// moves over it are the popup's in its points, with enter and leave for
// the whole area and none between the track and the popup touching it; a
// press there (also a non-client one on the parent's caption, also a
// press-drag into the popup) is delivered and does not dismiss; the wheel
// over it is the popup's; a press outside the area dismisses. Reposition
// keeps the track; an empty one is none.
func TestShownPopupGrabTrack(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{}
		sf := shownParent(t, s, r)
		defer sf.Close()
		L := shell.ButtonPrimary
		ph := sf.hwnd
		cfg := menuCfg
		cfg.Track = driver.Rect{X: 20, Y: 4, W: 200, H: 26} // touches the menu at y 30
		pr := &popRec{}
		p := newPopup(t, s, sf, cfg, pr)
		h := p.hwnd
		r.events = nil
		if p.px() != sf.px() {
			t.Skipf("the popup's scale %g is not the parent's %g", p.px(), sf.px())
		}
		near := func(a, b driver.Point) bool { return math.Abs(a.X-b.X) < 1e-9 && math.Abs(a.Y-b.Y) < 1e-9 }
		wantTrack := driver.Rect{X: 20 - p.rect.X, Y: 4 - p.rect.Y, W: 200, H: 26}
		if tr := p.track; !near(driver.Point{X: tr.X, Y: tr.Y}, driver.Point{X: wantTrack.X, Y: wantTrack.Y}) ||
			math.Abs(tr.W-200) > 1e-9 || math.Abs(tr.H-26) > 1e-9 {
			t.Fatalf("track %+v in the popup's points, want %+v (popup at %+v)", tr, wantTrack, p.rect)
		}
		// at is the popup lParam of the parent point (x, y).
		at := func(x, y float64) uintptr { return popLP(p, x-p.rect.X, y-p.rect.Y) }

		sendTo(h, w32.WM_MOUSEMOVE, 0, at(30, 10))  // the track: enter
		sendTo(h, w32.WM_MOUSEMOVE, 0, at(60, 50))  // the popup below: no leave
		sendTo(h, w32.WM_MOUSEMOVE, 0, at(30, 10))  // back
		sendTo(h, w32.WM_MOUSEMOVE, 0, at(300, 10)) // right of the track: leave
		sendTo(h, w32.WM_MOUSEMOVE, 0, at(30, 10))
		sendTo(h, w32.WM_LBUTTONDOWN, w32.MK_LBUTTON, at(30, 10))
		sendTo(h, w32.WM_LBUTTONUP, 0, at(30, 10))
		pes := popEvents(t, pr)
		want := []pev{{pEnter, 0, 0, 0}, {pMove, 0, 0, 0}, {pMove, 0, 0, 0}, {pMove, 0, 0, 0}, {pLeave, 0, 0, 0},
			{pEnter, 0, 0, 0}, {pMove, 0, 0, 0}, {pDown, L, L, 1}, {pUp, L, 0, 1}}
		if got := compact(pes); !samePevs(got, want) {
			t.Fatalf("over the track:\n got %+v\nwant %+v", got, want)
		}
		if q := (driver.Point{X: 30 - p.rect.X, Y: 10 - p.rect.Y}); !near(pes[1].Pos, q) || !near(pes[7].Pos, q) {
			t.Errorf("track positions %v, %v, want %v", pes[1].Pos, pes[7].Pos, q)
		}
		if pr.dismissed != 0 || s.grab != p || w32.GetCapture() != h || len(r.events) != 0 {
			t.Fatalf("after a press on the track: dismissed %d, grab %v, parent events %v", pr.dismissed, s.grab == p, r.events)
		}

		// A non-client press on the parent's caption over the track, then a
		// drag into the popup and its release there.
		sendTo(ph, w32.WM_NCLBUTTONDOWN, w32.HTCAPTION, screenLP(t, sf, 100, 8))
		sendTo(h, w32.WM_MOUSEMOVE, w32.MK_LBUTTON, at(100, 60))
		sendTo(h, w32.WM_LBUTTONUP, 0, at(100, 60))
		want = []pev{{pMove, 0, 0, 0}, {pDown, L, L, 1}, {pMove, 0, L, 0}, {pUp, L, 0, 1}}
		if got := compact(popEvents(t, pr)); !samePevs(got, want) || pr.dismissed != 0 {
			t.Fatalf("caption press on the track:\n got %+v\nwant %+v (dismissed %d)", got, want, pr.dismissed)
		}

		// The wheel over the track is the popup's.
		notch := uintptr(w32.WHEEL_DELTA) << 16
		sendTo(ph, w32.WM_MOUSEWHEEL, notch, screenLP(t, sf, 200, 20))
		if len(pr.events) != 1 {
			t.Fatalf("wheel over the track: %+v", pr.events)
		}
		if se, ok := pr.events[0].(shell.ScrollEvent); !ok || !near(se.Pos, driver.Point{X: 200 - p.rect.X, Y: 20 - p.rect.Y}) {
			t.Errorf("wheel over the track: %+v", pr.events[0])
		}
		pr.events = nil

		// Reposition keeps the track, moved with the popup.
		moved := cfg
		moved.Anchor.X, moved.Track = 80, driver.Rect{}
		p.Reposition(moved)
		if p.cfg.Track != cfg.Track || !near(driver.Point{X: p.track.X, Y: p.track.Y}, driver.Point{X: 20 - p.rect.X, Y: 4 - p.rect.Y}) {
			t.Errorf("after Reposition: cfg track %+v, track %+v, popup %+v", p.cfg.Track, p.track, p.rect)
		}

		// Outside the area a press dismisses.
		sendTo(h, w32.WM_LBUTTONDOWN, w32.MK_LBUTTON, at(300, 10))
		sendTo(h, w32.WM_LBUTTONUP, 0, at(300, 10))
		if pr.dismissed != 1 || s.grab != nil {
			t.Fatalf("outside press: dismissed %d", pr.dismissed)
		}
		p.Close()

		// An empty track is none.
		for _, bad := range []driver.Rect{{}, {X: 20, Y: 4, W: -200, H: 26}, {X: math.NaN(), Y: 4, W: 200, H: 26}} {
			cfg.Track = bad
			pr := &popRec{}
			p := newPopup(t, s, sf, cfg, pr)
			if p.track != (driver.Rect{}) {
				t.Errorf("track %+v: converted to %+v", bad, p.track)
			}
			sendTo(p.hwnd, w32.WM_LBUTTONDOWN, w32.MK_LBUTTON, at(30, 10))
			sendTo(p.hwnd, w32.WM_LBUTTONUP, 0, at(30, 10))
			if pr.dismissed != 1 {
				t.Errorf("track %+v: a press above the popup did not dismiss", bad)
			}
			p.Close()
		}

		// Without Grab it is ignored.
		cfg.Grab, cfg.Track = false, driver.Rect{X: 20, Y: 4, W: 200, H: 26}
		np := newPopup(t, s, sf, cfg, &popRec{})
		if np.track != (driver.Rect{}) {
			t.Errorf("a popup without Grab has the track %+v", np.track)
		}
		np.Close()
	})
}

// TestShownPopupRepositionKeepsPointer: a grab popup moved by Reposition
// from inside its Enter handler (a menu strip switching menus, S5.6f) gets
// the Move that follows in the new place's points, and later events too.
func TestShownPopupRepositionKeepsPointer(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{}
		sf := shownParent(t, s, r)
		defer sf.Close()
		cfg := menuCfg
		cfg.Track = driver.Rect{X: 20, Y: 4, W: 200, H: 26}
		pr := &popRec{}
		p := newPopup(t, s, sf, cfg, pr)
		defer p.Close()
		h := p.hwnd
		if p.px() != sf.px() {
			t.Skipf("the popup's scale %g is not the parent's %g", p.px(), sf.px())
		}
		moved := cfg
		moved.Anchor.X = 80
		pr.onEvent = func(ev shell.Event) {
			if pe, ok := ev.(shell.PointerEvent); ok && pe.Kind == shell.PointerEnter {
				p.Reposition(moved)
			}
		}
		old := p.rect
		sendTo(h, w32.WM_MOUSEMOVE, 0, popLP(p, 30-old.X, 10-old.Y)) // the track
		if p.rect.X == old.X {
			t.Fatalf("not moved: %+v", p.rect)
		}
		pr.onEvent = nil
		sendTo(h, w32.WM_LBUTTONDOWN, w32.MK_LBUTTON, popLP(p, 30-p.rect.X, 10-p.rect.Y))
		pes := popEvents(t, pr)
		want := []pev{{pEnter, 0, 0, 0}, {pMove, 0, 0, 0}, {pDown, shell.ButtonPrimary, shell.ButtonPrimary, 1}}
		if got := compact(pes); !samePevs(got, want) {
			t.Fatalf("events:\n got %+v\nwant %+v", got, want)
		}
		near := func(a, b driver.Point) bool { return math.Abs(a.X-b.X) < 1e-9 && math.Abs(a.Y-b.Y) < 1e-9 }
		q := driver.Point{X: 30 - p.rect.X, Y: 10 - p.rect.Y}
		if !near(pes[0].Pos, driver.Point{X: 30 - old.X, Y: 10 - old.Y}) || !near(pes[1].Pos, q) || !near(pes[2].Pos, q) {
			t.Errorf("positions %v %v %v, want the enter in the old place, then %v", pes[0].Pos, pes[1].Pos, pes[2].Pos, q)
		}
		sendTo(h, w32.WM_LBUTTONUP, 0, popLP(p, 30-p.rect.X, 10-p.rect.Y))
	})
}
