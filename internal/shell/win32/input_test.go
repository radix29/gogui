//go:build windows

package win32

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/shellkit"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// ---- pure functions ----

func TestModifiersOf(t *testing.T) {
	for _, c := range []struct {
		down []int32
		want driver.Modifiers
	}{
		{nil, 0},
		{[]int32{w32.VK_SHIFT}, driver.ModShift},
		{[]int32{w32.VK_CONTROL, w32.VK_MENU}, driver.ModControl | driver.ModOption}, // also AltGr
		{[]int32{w32.VK_RWIN}, driver.ModCommand},
		{[]int32{w32.VK_LWIN, w32.VK_SHIFT, w32.VK_CONTROL, w32.VK_MENU},
			driver.ModCommand | driver.ModShift | driver.ModControl | driver.ModOption},
		{[]int32{w32.VK_CAPITAL, w32.VK_LSHIFT}, 0}, // only the generic keys count
	} {
		got := modifiersOf(func(vk int32) bool {
			for _, d := range c.down {
				if d == vk {
					return true
				}
			}
			return false
		})
		if got != c.want || got&driver.ModPrimary != 0 {
			t.Errorf("modifiersOf(%v) = %v, want %v", c.down, got, c.want)
		}
	}
}

func TestButtonOf(t *testing.T) {
	x1, x2 := uintptr(w32.XBUTTON1)<<16, uintptr(w32.XBUTTON2)<<16
	for _, c := range []struct {
		msg           uint32
		wp            uintptr
		b             shell.PointerButton
		down, dbl, ok bool
	}{
		{w32.WM_LBUTTONDOWN, 0, shell.ButtonPrimary, true, false, true},
		{w32.WM_LBUTTONDBLCLK, 0, shell.ButtonPrimary, true, true, true},
		{w32.WM_LBUTTONUP, 0, shell.ButtonPrimary, false, false, true},
		{w32.WM_RBUTTONDOWN, 0, shell.ButtonSecondary, true, false, true},
		{w32.WM_RBUTTONUP, 0, shell.ButtonSecondary, false, false, true},
		{w32.WM_MBUTTONDBLCLK, 0, shell.ButtonMiddle, true, true, true},
		{w32.WM_XBUTTONDOWN, x1, shell.ButtonBack, true, false, true},
		{w32.WM_XBUTTONUP, x2, shell.ButtonForward, false, false, true},
		{w32.WM_XBUTTONDBLCLK, x2, shell.ButtonForward, true, true, true},
		{w32.WM_XBUTTONDOWN, 3 << 16, 0, false, false, false},
		{w32.WM_MOUSEMOVE, 0, 0, false, false, false},
	} {
		b, down, dbl, ok := buttonOf(c.msg, c.wp)
		if b != c.b || down != c.down || dbl != c.dbl || ok != c.ok {
			t.Errorf("buttonOf(%#x, %#x) = %v %v %v %v", c.msg, c.wp, b, down, dbl, ok)
		}
	}
	for b := shell.ButtonPrimary; b <= shell.ButtonForward; b <<= 1 {
		if i := buttonIndex(b); i < 0 || i >= 5 || shell.PointerButton(1)<<i != b {
			t.Errorf("buttonIndex(%v) = %d", b, i)
		}
	}
}

func TestWheelDelta(t *testing.T) {
	std := wheelSettings{lines: 3, chars: 3}
	for _, c := range []struct {
		delta      int16
		horizontal bool
		ws         wheelSettings
		want       driver.Point
	}{
		{120, false, std, driver.Point{Y: 3}},
		{-240, false, std, driver.Point{Y: -6}},
		{120, true, std, driver.Point{X: -3}}, // tilt right moves the content left
		{-120, true, std, driver.Point{X: 3}},
		{120, false, wheelSettings{lines: 1, chars: 3}, driver.Point{Y: 1}},
		{120, false, wheelSettings{lines: w32.WHEEL_PAGESCROLL, chars: 3}, driver.Point{Y: pageLines}},
		{120, false, wheelSettings{lines: 0, chars: 3}, driver.Point{}},
		{120, true, wheelSettings{lines: 3, chars: 5}, driver.Point{X: -5}},
	} {
		if got := wheelDelta(c.delta, c.horizontal, false, c.ws); got != c.want {
			t.Errorf("wheelDelta(%d, h %v, %+v) = %v, want %v", c.delta, c.horizontal, c.ws, got, c.want)
		}
	}
	// Precise: fractional lines times preciseLine points.
	if got := wheelDelta(40, false, true, std); got != (driver.Point{Y: 1 * preciseLine}) {
		t.Errorf("precise 40 = %v, want %v", got, preciseLine)
	}
	if got := wheelDelta(120, false, true, std); got != (driver.Point{Y: 100}) {
		t.Errorf("precise 120 = %v, want 100 points", got)
	}
	if got := wheelDelta(-12, true, true, std); math.Abs(got.X-10) > 1e-9 || got.Y != 0 {
		t.Errorf("precise horizontal -12 = %v", got)
	}
	for d, want := range map[int16]bool{120: false, -120: false, 240: false, 1: true, 60: true, -119: true, 121: true} {
		if got := isPreciseDelta(d); got != want {
			t.Errorf("isPreciseDelta(%d) = %v", d, got)
		}
	}
}

func TestKeyFor(t *testing.T) {
	// A German-like layout: VK_OEM_1 (0xBA) is Ü, VK_OEM_5 (0xDC) the dead ^.
	layout := map[uint32]uint32{'A': 'A', 'Z': 'Z', '1': '1', 0xBA: 0xDC, 0xDC: '^' | mapDeadKey,
		0xBB: '+', 0xBC: ',', 0xE2: '<', 0xC0: 0x1B}
	lc := func(vk uint32) uint32 { return layout[vk] }
	for _, c := range []struct {
		vk   uint32
		want driver.Key
	}{
		{'A', 'A'}, {'Z', 'Z'}, {'1', '1'},
		{0xBA, 'Ü'}, {0xDC, '^'}, {0xBB, '+'}, {0xBC, ','}, {0xE2, '<'},
		{0xC0, driver.KeyNone}, // a control character from the layout
		{0xDF, driver.KeyNone}, // nothing in the layout
		{w32.VK_RETURN, driver.KeyReturn}, {w32.VK_BACK, driver.KeyBackspace},
		{w32.VK_TAB, driver.KeyTab}, {w32.VK_ESCAPE, driver.KeyEscape}, {w32.VK_SPACE, driver.KeySpace},
		{w32.VK_LEFT, driver.KeyLeft}, {w32.VK_UP, driver.KeyUp}, {w32.VK_RIGHT, driver.KeyRight},
		{w32.VK_DOWN, driver.KeyDown}, {w32.VK_HOME, driver.KeyHome}, {w32.VK_END, driver.KeyEnd},
		{w32.VK_PRIOR, driver.KeyPageUp}, {w32.VK_NEXT, driver.KeyPageDown},
		{w32.VK_DELETE, driver.KeyDelete},
		{w32.VK_F1, driver.KeyF1}, {w32.VK_F10, driver.KeyF1 + 9}, {w32.VK_F12, driver.KeyF12},
		{w32.VK_F12 + 1, driver.KeyNone}, {w32.VK_F24, driver.KeyNone},
		{w32.VK_NUMPAD0, '0'}, {w32.VK_NUMPAD9, '9'}, {w32.VK_ADD, '+'}, {w32.VK_DIVIDE, '/'},
		{w32.VK_DECIMAL, '.'},
		{w32.VK_SHIFT, driver.KeyNone}, {w32.VK_CONTROL, driver.KeyNone}, {w32.VK_MENU, driver.KeyNone},
		{w32.VK_LWIN, driver.KeyNone}, {w32.VK_CAPITAL, driver.KeyNone}, {w32.VK_INSERT, driver.KeyNone},
		{w32.VK_PACKET, driver.KeyNone}, {w32.VK_LBUTTON, driver.KeyNone},
	} {
		if got := keyFor(c.vk, lc); got != c.want {
			t.Errorf("keyFor(%#x) = %q (%#x), want %q", c.vk, rune(got), got, rune(c.want))
		}
	}
	// The live layout: letters and digits are their VK codes on every
	// Latin layout.
	for _, vk := range []uint32{'A', 'Q', 'M'} {
		if got := keyFor(vk, liveLayoutChar); got != driver.Key(vk) {
			t.Errorf("live keyFor(%q) = %q", rune(vk), rune(got))
		}
	}
}

func TestTextUnit(t *testing.T) {
	var high uint16
	feed := func(units ...uint16) []string {
		var out []string
		for _, u := range units {
			if s := textUnit(&high, u); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	eq := func(a []string, b ...string) bool {
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
	for _, c := range []struct {
		units []uint16
		want  []string
	}{
		{[]uint16{'a', 'B', 0xE9}, []string{"a", "B", "é"}},
		{[]uint16{0xD83D, 0xDE00}, []string{"😀"}},
		{[]uint16{0x08, 0x09, 0x0D, 0x1B, 0x7F, 0x85, ' '}, []string{" "}}, // control characters
		{[]uint16{0xDE00}, nil},                                            // a lone low surrogate
		{[]uint16{0xD83D, 'x'}, []string{"x"}},                             // a lone high surrogate
		{[]uint16{0xD83D, 0xD83D, 0xDE00}, []string{"😀"}},                  // the second high wins
		{[]uint16{0x20AC}, []string{"€"}},
	} {
		high = 0
		if got := feed(c.units...); !eq(got, c.want...) {
			t.Errorf("units %#x: %q, want %q", c.units, got, c.want)
		}
	}
	if isRepeat(1) || !isRepeat(1|1<<30) {
		t.Error("isRepeat")
	}
}

// ---- real windows ----

// inputRecorder is a recorder whose Event answer for key-downs is handle.
type inputRecorder struct {
	*recorder
	handle func(ev shell.KeyEvent) bool
}

func (r *inputRecorder) Event(ev shell.Event) bool {
	r.recorder.Event(ev)
	if k, ok := ev.(shell.KeyEvent); ok && k.Down && r.handle != nil {
		return r.handle(k)
	}
	return false
}

// pump dispatches the thread's queued messages, without TranslateMessage.
func pump() {
	var m w32.Msg
	for w32.PeekMessage(&m, 0, 0, 0, w32.PM_REMOVE) {
		w32.DispatchMessage(&m)
		rethrow()
	}
}

// post posts a message to sf's window.
func post(t *testing.T, sf *surface, msg uint32, wp, lp uintptr) {
	t.Helper()
	if err := w32.PostMessage(sf.hwnd, msg, wp, lp); err != nil {
		t.Fatal(err)
	}
}

// px returns the client lParam of the surface point (x, y).
func pxAt(sf *surface, x, y float64) uintptr {
	return lparamXY(w32.Point{X: shellkit.ToPx(x, sf.Scale()), Y: shellkit.ToPx(y, sf.Scale())})
}

// kinds returns the kinds of pointer events and their buttons, compactly.
type pev struct {
	kind    shell.PointerKind
	button  shell.PointerButton
	buttons shell.PointerButton
	clicks  int
}

func compact(pes []shell.PointerEvent) []pev {
	out := make([]pev, len(pes))
	for i, pe := range pes {
		out[i] = pev{pe.Kind, pe.Button, pe.Buttons, pe.Clicks}
	}
	return out
}

func samePevs(a, b []pev) bool {
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

// TestSurfaceMouse: posted client mouse messages give the PointerEvent
// stream of shell.PointerEvent's rules: enter before the first move, the
// capture while buttons are held, the ups for a lost capture, the leave
// after the last release (the window is hidden, so the pointer is never
// over it), shell-counted clicks and monotonic event times.
func TestSurfaceMouse(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{}
		sf := newSurface(t, s, resizable, r)
		defer sf.Close()
		L, R, M := shell.ButtonPrimary, shell.ButtonSecondary, shell.ButtonMiddle
		const (
			enter = shell.PointerEnter
			move  = shell.PointerMove
			down  = shell.PointerDown
			up    = shell.PointerUp
			leave = shell.PointerLeave
		)

		// Move, two buttons, a move outside while captured, releases.
		post(t, sf, w32.WM_MOUSEMOVE, 0, pxAt(sf, 10, 10))
		post(t, sf, w32.WM_MOUSEMOVE, 0, pxAt(sf, 10, 10)) // same position: dropped
		post(t, sf, w32.WM_LBUTTONDOWN, w32.MK_LBUTTON, pxAt(sf, 10, 10))
		post(t, sf, w32.WM_RBUTTONDOWN, w32.MK_LBUTTON|w32.MK_RBUTTON, pxAt(sf, 10, 10))
		post(t, sf, w32.WM_MOUSEMOVE, w32.MK_LBUTTON|w32.MK_RBUTTON, lparamXY(w32.Point{X: -20, Y: 5}))
		post(t, sf, w32.WM_LBUTTONUP, w32.MK_RBUTTON, lparamXY(w32.Point{X: -20, Y: 5}))
		post(t, sf, w32.WM_LBUTTONUP, 0, lparamXY(w32.Point{X: -20, Y: 5})) // not held: dropped
		pump()
		if w32.GetCapture() != sf.hwnd {
			t.Error("no capture while the secondary button is held")
		}
		post(t, sf, w32.WM_RBUTTONUP, 0, lparamXY(w32.Point{X: -20, Y: 5}))
		pump()
		pes := pointerEvents(t, r)
		// While the secondary button keeps the capture, the OS may deliver a
		// move of its own (the real cursor, seen twice in stress runs); a move
		// while captured is valid, so moves after the first release are not
		// part of what this test pins.
		for i := len(pes) - 1; i > 0; i-- {
			if pes[i].Kind == move && slices.ContainsFunc(pes[:i], func(pe shell.PointerEvent) bool { return pe.Kind == up }) {
				pes = slices.Delete(pes, i, i+1)
			}
		}
		want := []pev{{enter, 0, 0, 0}, {move, 0, 0, 0}, {down, L, L, 1}, {down, R, L | R, 1},
			{move, 0, L | R, 0}, {up, L, R, 1}, {up, R, 0, 1}, {leave, 0, 0, 0}}
		if got := compact(pes); !samePevs(got, want) {
			t.Fatalf("press and release:\n got %+v\nwant %+v", got, want)
		}
		if pes[1].Pos != (driver.Point{X: 10, Y: 10}) || pes[4].Pos.X >= 0 {
			t.Errorf("positions: %v, %v", pes[1].Pos, pes[4].Pos)
		}
		for i := 1; i < len(pes); i++ {
			if pes[i].Time < pes[i-1].Time {
				t.Errorf("event %d time %v before %v", i, pes[i].Time, pes[i-1].Time)
			}
		}
		if w32.GetCapture() == sf.hwnd {
			t.Error("capture kept after the last release")
		}

		// Losing the capture ends every held button, then leaves.
		post(t, sf, w32.WM_MBUTTONDOWN, w32.MK_MBUTTON, pxAt(sf, 20, 20))
		post(t, sf, w32.WM_LBUTTONDOWN, w32.MK_MBUTTON|w32.MK_LBUTTON, pxAt(sf, 20, 20))
		pump()
		if err := w32.ReleaseCapture(); err != nil {
			t.Fatal(err)
		}
		rethrow()
		want = []pev{{enter, 0, 0, 0}, {down, M, M, 1}, {down, L, L | M, 1},
			{up, L, M, 1}, {up, M, 0, 1}, {leave, 0, 0, 0}}
		if got := compact(pointerEvents(t, r)); !samePevs(got, want) {
			t.Fatalf("capture lost:\n got %+v\nwant %+v", got, want)
		}

		// X buttons; the X-button messages return TRUE.
		if res := w32.SendMessage(sf.hwnd, w32.WM_XBUTTONDOWN, uintptr(w32.XBUTTON1)<<16, pxAt(sf, 5, 5)); res != 1 {
			t.Errorf("WM_XBUTTONDOWN = %d, want 1", res)
		}
		w32.SendMessage(sf.hwnd, w32.WM_XBUTTONUP, uintptr(w32.XBUTTON1)<<16, pxAt(sf, 5, 5))
		rethrow()
		B := shell.ButtonBack
		want = []pev{{enter, 0, 0, 0}, {down, B, B, 1}, {up, B, 0, 1}, {leave, 0, 0, 0}}
		if got := compact(pointerEvents(t, r)); !samePevs(got, want) {
			t.Fatalf("X button:\n got %+v\nwant %+v", got, want)
		}

		// Click counting on the shell's message times: three presses in a
		// row count 1, 2, 3, the matching ups carry the count, and a
		// press after the double-click time starts again.
		var now uint32 = 50000
		s.msgTime = func() uint32 { return now }
		dbl := w32.GetDoubleClickTime()
		var counts []int
		for i, dt := range []uint32{dbl + 1, 10, 10, dbl + 1} {
			now += dt
			msg := uint32(w32.WM_LBUTTONDOWN)
			if i == 1 {
				msg = w32.WM_LBUTTONDBLCLK // as Windows sends the second click
			}
			w32.SendMessage(sf.hwnd, msg, w32.MK_LBUTTON, pxAt(sf, 30, 30))
			w32.SendMessage(sf.hwnd, w32.WM_LBUTTONUP, 0, pxAt(sf, 30, 30))
			rethrow()
			for _, pe := range pointerEvents(t, r) {
				if pe.Kind == down {
					counts = append(counts, pe.Clicks)
				}
				if pe.Kind == up && pe.Clicks != counts[len(counts)-1] {
					t.Errorf("up clicks %d after down %d", pe.Clicks, counts[len(counts)-1])
				}
			}
		}
		if len(counts) != 4 || counts[0] != 1 || counts[1] != 2 || counts[2] != 3 || counts[3] != 1 {
			t.Errorf("click counts %v, want [1 2 3 1]", counts)
		}
		s.msgTime = liveMsgTime

		// A handler that closes the surface on PointerDown frees the
		// capture with the window.
		r2 := &recorder{}
		sf2 := newSurface(t, s, resizable, r2)
		sf2.h = &closingRecorder{recorder: r2, sf: sf2}
		w32.SendMessage(sf2.hwnd, w32.WM_LBUTTONDOWN, w32.MK_LBUTTON, pxAt(sf2, 30, 30))
		rethrow()
		if !sf2.dead || r2.closed != 1 || w32.GetCapture() != 0 {
			t.Errorf("close in PointerDown: dead %v, closed %d, capture %v", sf2.dead, r2.closed, w32.GetCapture())
		}
	})
}

// TestSurfaceWheel: wheel notches are lines without phases; precise
// deltas are points in a gesture that the timer ends with PhaseEnded.
func TestSurfaceWheel(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{}
		sf := newSurface(t, s, resizable, r)
		defer sf.Close()
		ws := readWheelSettings()
		at := screenOf(t, sf, 50, 60)
		wheel := func(msg uint32, delta int16) {
			w32.SendMessage(sf.hwnd, msg, uintptr(uint16(delta))<<16, lparamXY(at))
			rethrow()
		}
		scrolls := func() []shell.ScrollEvent {
			var out []shell.ScrollEvent
			for _, ev := range r.events {
				se, ok := ev.(shell.ScrollEvent)
				if !ok {
					t.Fatalf("unexpected event %T", ev)
				}
				out = append(out, se)
			}
			r.events = nil
			return out
		}

		wheel(w32.WM_MOUSEWHEEL, 120)
		wheel(w32.WM_MOUSEHWHEEL, -240)
		got := scrolls()
		if ws.lines == 0 {
			t.Skip("wheel scrolling is off in this session's settings")
		}
		if len(got) != 2 || got[0].Precise || got[0].Phase != 0 || got[0].Delta != wheelDelta(120, false, false, ws) ||
			got[0].Pos != (driver.Point{X: 50, Y: 60}) || got[1].Delta != wheelDelta(-240, true, false, ws) {
			t.Fatalf("notches: %+v (settings %+v)", got, ws)
		}

		// A gesture: Began, Changed (also for a whole notch), then the
		// timer's Ended with a zero delta.
		wheel(w32.WM_MOUSEWHEEL, 30)
		wheel(w32.WM_MOUSEWHEEL, -120)
		wheel(w32.WM_MOUSEHWHEEL, 7)
		got = scrolls()
		if len(got) != 3 || !got[0].Precise || got[0].Phase != shell.PhaseBegan ||
			got[1].Phase != shell.PhaseChanged || !got[1].Precise || got[1].Delta != wheelDelta(-120, false, true, ws) ||
			got[2].Phase != shell.PhaseChanged || got[2].Delta.X >= 0 {
			t.Fatalf("gesture: %+v", got)
		}
		for _, se := range got {
			if se.Momentum != shell.PhaseNone {
				t.Errorf("momentum %v set", se.Momentum)
			}
		}
		deadline := time.Now().Add(2 * time.Second)
		for len(r.events) == 0 && time.Now().Before(deadline) {
			pump()
			time.Sleep(5 * time.Millisecond)
		}
		got = scrolls()
		if len(got) != 1 || got[0].Phase != shell.PhaseEnded || got[0].Delta != (driver.Point{}) || !got[0].Precise {
			t.Fatalf("end of the gesture: %+v", got)
		}
		// The next notch is a notch again.
		wheel(w32.WM_MOUSEWHEEL, 120)
		if got := scrolls(); len(got) != 1 || got[0].Precise || got[0].Phase != shell.PhaseNone {
			t.Errorf("notch after the gesture: %+v", got)
		}
	})
}

// TestSurfaceKeyboard: key-downs come with their text after them; a
// handled key-down drops its pending WM_CHAR; surrogate pairs are joined
// and control characters dropped; an unhandled Alt+F4 asks to close.
func TestSurfaceKeyboard(t *testing.T) {
	inShell(t, func(s *Shell) {
		handled := false
		r := &inputRecorder{recorder: &recorder{}, handle: func(shell.KeyEvent) bool { return handled }}
		sf := newSurface(t, s, resizable, r.recorder)
		sf.h = r
		defer sf.Close()
		describe := func() []string {
			var out []string
			for _, ev := range r.events {
				switch e := ev.(type) {
				case shell.KeyEvent:
					d := "up"
					if e.Down {
						d = "down"
					}
					if e.Repeat {
						d += "*"
					}
					out = append(out, d+":"+string(rune(e.Key)))
				case shell.TextEvent:
					out = append(out, "text:"+e.Text)
				default:
					t.Fatalf("unexpected event %T", ev)
				}
			}
			r.events = nil
			return out
		}
		same := func(got []string, want ...string) bool {
			if len(got) != len(want) {
				return false
			}
			for i := range got {
				if got[i] != want[i] {
					return false
				}
			}
			return true
		}

		// Unhandled: the text follows the key-down (as TranslateMessage
		// posts it), a repeat, then the release.
		post(t, sf, w32.WM_KEYDOWN, 'A', 1)
		post(t, sf, w32.WM_CHAR, 'a', 1)
		post(t, sf, w32.WM_KEYDOWN, 'A', 1|1<<30)
		post(t, sf, w32.WM_CHAR, 'a', 1|1<<30)
		post(t, sf, w32.WM_KEYUP, 'A', 1|1<<30|1<<31)
		pump()
		if got := describe(); !same(got, "down:A", "text:a", "down*:A", "text:a", "up:A") {
			t.Errorf("unhandled key: %q", got)
		}

		// Handled: the pending WM_CHAR is removed.
		handled = true
		post(t, sf, w32.WM_KEYDOWN, 'A', 1)
		post(t, sf, w32.WM_CHAR, 'a', 1)
		post(t, sf, w32.WM_KEYUP, 'A', 1|1<<30|1<<31)
		pump()
		if got := describe(); !same(got, "down:A", "up:A") {
			t.Errorf("handled key: %q", got)
		}
		handled = false

		// Surrogate pairs, control characters, a lone surrogate.
		for _, u := range []uintptr{0xD83D, 0xDE00, 0x08, 0x0D, 0x1B, 0xDE00, 0xE9} {
			post(t, sf, w32.WM_CHAR, u, 1)
		}
		pump()
		if got := describe(); !same(got, "text:😀", "text:é") {
			t.Errorf("text: %q", got)
		}

		// Named keys and F10 as a system key.
		post(t, sf, w32.WM_KEYDOWN, w32.VK_RETURN, 1)
		post(t, sf, w32.WM_KEYDOWN, w32.VK_LEFT, 1)
		pump()
		if got := describe(); !same(got, "down:"+string(rune(driver.KeyReturn)), "down:"+string(rune(driver.KeyLeft))) {
			t.Errorf("named keys: %q", got)
		}
		handled = true // a handled F10 does not enter the menu mode
		w32.SendMessage(sf.hwnd, w32.WM_SYSKEYDOWN, w32.VK_F10, 1)
		w32.SendMessage(sf.hwnd, w32.WM_SYSKEYUP, w32.VK_F10, 1|1<<30|1<<31)
		rethrow()
		if got := describe(); !same(got, "down:"+string(rune(driver.KeyF1+9)), "up:"+string(rune(driver.KeyF1+9))) {
			t.Errorf("F10: %q", got)
		}

		// A handled system key is not passed on; an unhandled one goes to
		// DefWindowProcW (handled false), which turns a real Alt+F4 into
		// SC_CLOSE. DefWindowProcW checks the physical Alt state, so a
		// synthetic message cannot show that part; S2.3f checks it by hand.
		if _, ok := sf.keyMsg(w32.WM_SYSKEYDOWN, w32.VK_F4, 1|1<<29); !ok {
			t.Error("handled Alt+F4 passed on to DefWindowProcW")
		}
		handled = false
		if _, ok := sf.keyMsg(w32.WM_SYSKEYDOWN, w32.VK_F4, 1|1<<29); ok {
			t.Error("unhandled Alt+F4 not passed on to DefWindowProcW")
		}
		if _, ok := sf.keyMsg(w32.WM_SYSKEYUP, w32.VK_F4, 1|1<<29|1<<30|1<<31); ok {
			t.Error("key-up of an unhandled key not passed on")
		}
		if r.requests != 0 {
			t.Errorf("%d close requests", r.requests)
		}
		describe()

		// WM_SYSCHAR is not text; a key that went to the IME is not a key.
		w32.SendMessage(sf.hwnd, w32.WM_SYSCHAR, 'x', 1|1<<29)
		w32.SendMessage(sf.hwnd, w32.WM_KEYDOWN, w32.VK_PROCESSKEY, 1)
		rethrow()
		if got := describe(); len(got) != 0 {
			t.Errorf("WM_SYSCHAR and VK_PROCESSKEY: %q", got)
		}
	})
}
