//go:build windows

package win32

import (
	"math/bits"
	"time"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/shellkit"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// Mouse input in the client area (S2.3e). The chrome regions report the
// pointer through non-client messages (ncinput.go); everything else in the
// drawable area arrives as client mouse messages:
//
//   - WM_MOUSEMOVE is a PointerMove, after a PointerEnter when the pointer
//     was outside. Repeated moves to the same position are dropped
//     (Windows sends some when windows change under the pointer).
//     TrackMouseEvent(TME_LEAVE) asks for the WM_MOUSELEAVE that ends it.
//   - WM_*BUTTONDOWN and WM_*BUTTONDBLCLK (left, right, middle, X1, X2) are
//     a PointerDown. The first held button captures the mouse (SetCapture),
//     so the moves and the release keep arriving outside the window; the
//     last release frees it. A press of a held button and a release of a
//     button that is not held (a press that began in another window) are
//     dropped.
//   - WM_CAPTURECHANGED while buttons are held means the OS (or another
//     window) took the capture: a PointerUp for every held button, then a
//     PointerLeave if the pointer is outside.
//   - Click counts are the shell's (shellkit.ClickCounter), with the OS's
//     double-click time and rectangle, so triple clicks work; a DBLCLK
//     message counts at least 2, because Windows already judged it so.
//   - The events carry GetMessageTime mapped onto the clock (eventTime).
//   - The first click (S6.1c): Windows sends WM_MOUSEACTIVATE to a window
//     that is not active before the button-down, then activates it
//     (WM_ACTIVATE, so KeyChanged(true) comes first) and posts the press.
//     A WM_MOUSEACTIVATE over the client area or a traffic light while the
//     surface is not key and not blocked marks the next button-down
//     (client or light) as Activating, if the surface is key by then; that
//     button-down, a caption press (the OS's) and a deactivation clear the
//     mark. DefWindowProcW still answers MA_ACTIVATE, so the window
//     activates as before; the aqua driver drops the press (S6.1b). A
//     blocked surface's press is not delivered at all (modal.go).
//
// Left and right are the primary and secondary buttons: Windows applies the
// swapped-buttons setting before the messages. Handling the right button
// up and the X buttons in the shell means DefWindowProcW sends no
// WM_CONTEXTMENU or WM_APPCOMMAND for them; the aqua driver handles context
// menus and back/forward itself.

// eventTime returns the time of the message being handled on the clock's
// timeline: GetMessageTime and GetTickCount are milliseconds since system
// start (they wrap after 49.7 days), mapped by shellkit.EventTimeAt. A
// message older than shellkit.MaxEventAge was sent, not posted, and its
// GetMessageTime belongs to an earlier message. The times the shell hands
// out never go backwards (tick counts have a resolution of about 16 ms,
// and sent messages have no time of their own).
func (s *Shell) eventTime() time.Duration {
	return s.events.Next(shellkit.EventTimeAt(s.clock.Now(), s.tick(), s.msgTime()))
}

func liveMsgTime() uint32 { return uint32(w32.GetMessageTime()) }

// buttonOf returns the button of a button message, whether it is a press,
// and whether it is a DBLCLK. ok is false for other messages.
func buttonOf(msg uint32, wp uintptr) (b shell.PointerButton, down, dbl, ok bool) {
	switch msg {
	case w32.WM_LBUTTONDOWN, w32.WM_LBUTTONDBLCLK, w32.WM_LBUTTONUP:
		b = shell.ButtonPrimary
	case w32.WM_RBUTTONDOWN, w32.WM_RBUTTONDBLCLK, w32.WM_RBUTTONUP:
		b = shell.ButtonSecondary
	case w32.WM_MBUTTONDOWN, w32.WM_MBUTTONDBLCLK, w32.WM_MBUTTONUP:
		b = shell.ButtonMiddle
	case w32.WM_XBUTTONDOWN, w32.WM_XBUTTONDBLCLK, w32.WM_XBUTTONUP:
		switch uint16(wp >> 16) {
		case w32.XBUTTON1:
			b = shell.ButtonBack
		case w32.XBUTTON2:
			b = shell.ButtonForward
		default:
			return 0, false, false, false
		}
	default:
		return 0, false, false, false
	}
	switch msg {
	case w32.WM_LBUTTONDBLCLK, w32.WM_RBUTTONDBLCLK, w32.WM_MBUTTONDBLCLK, w32.WM_XBUTTONDBLCLK:
		return b, true, true, true
	case w32.WM_LBUTTONDOWN, w32.WM_RBUTTONDOWN, w32.WM_MBUTTONDOWN, w32.WM_XBUTTONDOWN:
		return b, true, false, true
	}
	return b, false, false, true
}

// buttonIndex is the index of a single button in surface.upClicks.
func buttonIndex(b shell.PointerButton) int { return bits.TrailingZeros8(uint8(b)) }

// mouse handles the client mouse messages.
func (sf *surface) mouse(h windows.HWND, msg uint32, wp, lp uintptr) (uintptr, bool) {
	switch msg {
	case w32.WM_MOUSEMOVE:
		sf.trackClient(h)
		sf.moveTo(sf.clientPoint(lp))
		return 0, true
	case w32.WM_CAPTURECHANGED:
		if sf.buttons != 0 && windows.HWND(lp) != h {
			sf.cancelButtons()
		}
		return 0, true
	}
	b, down, dbl, ok := buttonOf(msg, wp)
	if !ok {
		return 0, false
	}
	if down {
		sf.press(h, b, signedXY(lp), dbl)
	} else {
		sf.release(b, sf.clientPoint(lp))
	}
	if msg >= w32.WM_XBUTTONDOWN {
		return 1, true // the X-button messages return TRUE
	}
	return 0, true
}

// trackClient asks for the WM_MOUSELEAVE of the client area. A request
// for one area may cancel the other's, so asking for one forgets the other.
func (sf *surface) trackClient(h windows.HWND) {
	if sf.tracking {
		return
	}
	sf.tracking, sf.ncTracking = true, false
	tme := w32.TrackMouseEventInfo{DwFlags: w32.TME_LEAVE, HwndTrack: h}
	_ = w32.TrackMouseEvent(&tme)
}

// moveTo delivers a pointer move to pos (points): PointerEnter first if the
// pointer was outside, nothing if it did not move.
func (sf *surface) moveTo(pos driver.Point) {
	if sf.inside && pos == sf.lastPos {
		return
	}
	if sf.enter(pos) {
		sf.pointer(shell.PointerMove, pos, shell.ButtonNone, 0)
	}
}

// press delivers a press of b at the client pixel p. The first held button
// captures the mouse.
func (sf *surface) press(h windows.HWND, b shell.PointerButton, p w32.Point, dbl bool) {
	activating := sf.activating && sf.key
	sf.activating = false // only the press right after WM_MOUSEACTIVATE
	if sf.buttons&b != 0 || sf.dead {
		return
	}
	pos := pointAt(p, w32.Rect{}, sf.px())
	if !sf.enter(pos) {
		return
	}
	n := sf.clicks.Press(b, shellkit.Point(p), sf.s.msgTime(), sf.clickRules(), dbl)
	if sf.buttons == 0 && w32.GetCapture() != h {
		w32.SetCapture(h)
	}
	sf.buttons |= b
	sf.upClicks[buttonIndex(b)] = n
	sf.deliver(shell.PointerDown, pos, b, n, activating)
}

// mouseActivate handles WM_MOUSEACTIVATE (the first click above): the low
// word of lp is the hit-test code of the press.
func (sf *surface) mouseActivate(lp uintptr) {
	code := uintptr(loword(lp))
	sf.activating = !sf.key && !sf.blocked && !sf.dead && (code == w32.HTCLIENT || isLight(code))
}

// release delivers the release of a held button at pos. The last one frees
// the capture and checks whether the pointer is still inside.
func (sf *surface) release(b shell.PointerButton, pos driver.Point) {
	if sf.buttons&b == 0 || sf.dead {
		return
	}
	sf.buttons &^= b
	if sf.buttons == 0 && w32.GetCapture() == sf.hwnd {
		_ = w32.ReleaseCapture() // its WM_CAPTURECHANGED finds no buttons
	}
	if !sf.pointer(shell.PointerUp, pos, b, sf.upClicks[buttonIndex(b)]) {
		return
	}
	if sf.buttons == 0 {
		sf.leaveIfOutside()
	}
}

// cancelButtons ends every held button with a PointerUp at the pointer's
// position after the capture was lost, then sends PointerLeave if the
// pointer is outside.
func (sf *surface) cancelButtons() {
	pt, _ := w32.GetCursorPos()
	pos := sf.screenPoint(pt)
	for sf.buttons != 0 {
		b := sf.buttons & -sf.buttons // the lowest held button
		sf.buttons &^= b
		if !sf.pointer(shell.PointerUp, pos, b, sf.upClicks[buttonIndex(b)]) {
			return
		}
	}
	sf.leaveIfOutside()
}
