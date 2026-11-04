//go:build windows

package win32

import (
	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// Pointer events over the chrome regions (S2.3d). WM_NCHITTEST makes the
// title bar HTCAPTION and the traffic lights HTCLOSE/HTMINBUTTON/
// HTMAXBUTTON, so Windows reports the pointer there with non-client
// messages. They become ordinary PointerEvents in surface coordinates:
//
//   - WM_NCMOUSEMOVE over a chrome region is a PointerMove (after a
//     PointerEnter if the pointer was outside). It still goes on to
//     DefWindowProcW, which draws nothing for a window without a caption
//     but keeps the Windows 11 Snap Layouts hover over HTMAXBUTTON.
//   - WM_NCLBUTTONDOWN or WM_NCLBUTTONDBLCLK on a light is a primary
//     press of the client path (input.go): it captures the mouse and is
//     counted like any click, and the default action (the native button's
//     own tracking and its SC_CLOSE, SC_MINIMIZE or SC_MAXIMIZE) is
//     suppressed, so the click is the aqua driver's. While the press lasts,
//     the captured WM_MOUSEMOVE and WM_LBUTTONUP arrive as client messages,
//     and WM_CAPTURECHANGED ends it. A light's WM_NCLBUTTONUP is swallowed.
//   - A press on the caption is not delivered: DefWindowProcW moves the
//     window, and a double-click maximizes it (shell.RegionCaption).
//   - PointerLeave follows when the pointer is no longer over the drawable
//     area (the client area or a chrome region of this window): checked on
//     WM_NCMOUSELEAVE, WM_MOUSELEAVE, a WM_NCMOUSEMOVE over a resize border
//     and the release of the last button.

// signedXY splits a mouse message's lParam into its signed coordinates
// (GET_X_LPARAM, GET_Y_LPARAM).
func signedXY(lp uintptr) w32.Point {
	return w32.Point{X: int32(int16(uint16(lp))), Y: int32(int16(uint16(lp >> 16)))}
}

// modifiers returns the modifier keys held as of the current message.
// AltGr arrives as Control+Option (Windows sends it as left Ctrl + right
// Alt).
func modifiers() driver.Modifiers {
	return modifiersOf(func(vk int32) bool { return w32.GetKeyState(vk) < 0 })
}

// modifiersOf returns the modifiers for a key-state reader.
func modifiersOf(down func(vk int32) bool) driver.Modifiers {
	var m driver.Modifiers
	if down(w32.VK_SHIFT) {
		m |= driver.ModShift
	}
	if down(w32.VK_CONTROL) {
		m |= driver.ModControl
	}
	if down(w32.VK_MENU) {
		m |= driver.ModOption
	}
	if down(w32.VK_LWIN) || down(w32.VK_RWIN) {
		m |= driver.ModCommand
	}
	return m
}

// pointer delivers a pointer event at pos (points) and reports whether the
// surface is still open afterwards. Buttons is the set held after the
// event (sf.buttons, already updated by the caller).
func (sf *surface) pointer(kind shell.PointerKind, pos driver.Point, button shell.PointerButton, clicks int) bool {
	if sf.dead {
		return false
	}
	sf.lastPos = pos
	sf.h.Event(shell.PointerEvent{Kind: kind, Pos: pos, Button: button, Buttons: sf.buttons,
		Clicks: clicks, Mods: modifiers(), Time: sf.s.eventTime()})
	return !sf.dead
}

// enter sends PointerEnter if the pointer was outside.
func (sf *surface) enter(pos driver.Point) bool {
	if sf.inside {
		return true
	}
	sf.inside = true
	return sf.pointer(shell.PointerEnter, pos, shell.ButtonNone, 0)
}

// leaveIfOutside sends PointerLeave if the pointer was inside, no button
// holds it, and it is no longer over the drawable area.
func (sf *surface) leaveIfOutside() {
	if !sf.inside || sf.buttons != 0 || sf.dead {
		return
	}
	pt, err := w32.GetCursorPos()
	if err == nil && sf.pointerOver(pt) {
		return
	}
	sf.inside = false
	sf.pointer(shell.PointerLeave, sf.screenPoint(pt), shell.ButtonNone, 0)
}

// pointerOver reports whether the screen pixel pt is over the drawable
// area: this window is the one there, and it is not a resize border.
func (sf *surface) pointerOver(pt w32.Point) bool {
	if w32.WindowFromPoint(pt) != sf.hwnd {
		return false
	}
	code := sf.hitAt(pt)
	return code == w32.HTCLIENT || isChrome(code)
}

// screenPoint converts a screen pixel to surface points.
func (sf *surface) screenPoint(pt w32.Point) driver.Point {
	cl, err := sf.screenClient()
	if err != nil {
		return driver.Point{}
	}
	return pointAt(pt, cl, sf.px())
}

// screenToClient converts a screen pixel to client pixels.
func (sf *surface) screenToClient(pt w32.Point) w32.Point {
	cl, err := sf.screenClient()
	if err != nil {
		return w32.Point{}
	}
	return w32.Point{X: pt.X - cl.Left, Y: pt.Y - cl.Top}
}

// clientPoint converts a client-pixel lParam to surface points.
func (sf *surface) clientPoint(lp uintptr) driver.Point {
	return pointAt(signedXY(lp), w32.Rect{}, sf.px())
}

// trackNonClient asks for the WM_NCMOUSELEAVE of the chrome regions (see
// trackClient).
func (sf *surface) trackNonClient(h windows.HWND) {
	if sf.ncTracking {
		return
	}
	sf.ncTracking, sf.tracking = true, false
	tme := w32.TrackMouseEventInfo{DwFlags: w32.TME_LEAVE | w32.TME_NONCLIENT, HwndTrack: h}
	_ = w32.TrackMouseEvent(&tme)
}

// ncPointer handles the non-client mouse messages. handled is false for
// the messages that go on to DefWindowProcW.
func (sf *surface) ncPointer(h windows.HWND, msg uint32, wp, lp uintptr) (r uintptr, handled bool) {
	switch msg {
	case w32.WM_NCMOUSEMOVE:
		if sf.buttons != 0 {
			return 0, false
		}
		if !isChrome(wp) {
			sf.leaveIfOutside()
			return 0, false
		}
		sf.trackNonClient(h)
		sf.moveTo(sf.screenPoint(signedXY(lp)))
		return 0, false
	case w32.WM_NCMOUSELEAVE:
		sf.ncTracking = false
		sf.leaveIfOutside()
		return 0, false // DefWindowProcW tracks the Snap Layouts hover too
	case w32.WM_NCLBUTTONDOWN, w32.WM_NCLBUTTONDBLCLK:
		if !isLight(wp) || sf.buttons != 0 {
			sf.clicks.Reset() // a caption press (the OS's) breaks a click series
			return 0, false
		}
		sf.press(h, shell.ButtonPrimary, sf.screenToClient(signedXY(lp)), msg == w32.WM_NCLBUTTONDBLCLK)
		return 0, true
	case w32.WM_NCLBUTTONUP:
		if isLight(wp) {
			return 0, true // the press was the aqua driver's
		}
		return 0, false
	}
	return 0, false
}
