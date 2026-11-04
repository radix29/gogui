//go:build windows

package win32

import (
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// Wheel and touchpad scrolling (S2.3e). WM_MOUSEWHEEL and WM_MOUSEHWHEEL
// carry a signed delta in units of WHEEL_DELTA (120) per notch:
//
//   - A delta that is a multiple of 120 is a wheel notch (or several). The
//     ScrollEvent is in lines: the notches times SPI_GETWHEELSCROLLLINES
//     (vertical) or SPI_GETWHEELSCROLLCHARS (horizontal), not Precise.
//   - Any other delta is precise: precision touchpads and high-resolution
//     wheels send those. Its ScrollEvent is in points, the same lines times
//     preciseLine points per line, so a full precise "notch" scrolls as far
//     as a wheel notch at the aqua driver's default line. Precise events
//     form a gesture: PhaseBegan on the first, PhaseChanged on the next
//     ones, and PhaseEnded with a zero delta when none came for
//     gestureTimeout (a timer of the surface window). Inside a gesture
//     every event is precise, also one whose delta happens to be a
//     multiple of 120. Windows reports no momentum, so Momentum stays
//     PhaseNone and the aqua driver runs its own (PLAN.md §5.6).
//
// Windows has applied the touchpad's scroll-direction setting already. A
// positive WM_MOUSEWHEEL delta (wheel away from the user) shows content
// further up, so it moves the content down: a positive Delta.Y. A
// positive WM_MOUSEHWHEEL delta (tilt right) shows content further right,
// so it moves the content left: a negative Delta.X.

// gestureTimeout is how long after the last precise event the gesture
// ends.
const gestureTimeout = 100 // ms

// timerGesture is the surface window's timer ID for gestureTimeout.
const timerGesture = 1

// preciseLine is the points per line of a precise delta (Chromium's 100 px
// per notch of three lines).
const preciseLine = 100.0 / 3

// pageLines are the lines of a notch when the wheel is set to scroll a page
// at a time (WHEEL_PAGESCROLL): ScrollEvent has no page unit.
const pageLines = 20

// wheelSettings are the OS's scroll settings: lines per vertical notch
// (WHEEL_PAGESCROLL for a page) and characters per horizontal notch.
type wheelSettings struct {
	lines, chars uint32
}

// readWheelSettings reads the scroll settings; failures give the defaults
// (3 and 3).
func readWheelSettings() wheelSettings {
	ws := wheelSettings{lines: 3, chars: 3}
	if v, err := w32.SystemParametersInfoUint32(w32.SPI_GETWHEELSCROLLLINES); err == nil {
		ws.lines = v
	}
	if v, err := w32.SystemParametersInfoUint32(w32.SPI_GETWHEELSCROLLCHARS); err == nil {
		ws.chars = v
	}
	return ws
}

// isPreciseDelta reports whether a wheel delta is not a whole number of
// notches.
func isPreciseDelta(delta int16) bool { return delta%w32.WHEEL_DELTA != 0 }

// wheelDelta converts a wheel delta into a ScrollEvent delta: lines for a
// notch, points for a precise delta. It is zero when the setting turns
// scrolling off (0 lines or characters).
func wheelDelta(delta int16, horizontal, precise bool, ws wheelSettings) driver.Point {
	per := float64(ws.lines)
	if horizontal {
		per = float64(ws.chars)
	} else if ws.lines == w32.WHEEL_PAGESCROLL {
		per = pageLines
	}
	v := float64(delta) / w32.WHEEL_DELTA * per
	if precise {
		v *= preciseLine
	}
	if horizontal {
		return driver.Point{X: -v}
	}
	return driver.Point{Y: v}
}

// wheel handles WM_MOUSEWHEEL and WM_MOUSEHWHEEL (the position in lp is in
// screen pixels).
func (sf *surface) wheel(msg uint32, wp, lp uintptr) {
	delta := int16(uint16(wp >> 16))
	if delta == 0 || sf.dead {
		return
	}
	precise, phase := sf.gesture.Next(isPreciseDelta(delta))
	d := wheelDelta(delta, msg == w32.WM_MOUSEHWHEEL, precise, readWheelSettings())
	pos := sf.screenPoint(signedXY(lp))
	sf.scrollPos = pos
	if precise {
		// Restarts the timer; fails only for a destroyed window.
		_ = w32.SetTimer(sf.hwnd, timerGesture, gestureTimeout)
	}
	if d == (driver.Point{}) && phase == shell.PhaseNone {
		return // scrolling is off
	}
	sf.h.Event(shell.ScrollEvent{Pos: pos, Delta: d, Precise: precise, Phase: phase,
		Mods: modifiers(), Time: sf.s.eventTime()})
}

// endGesture ends a precise gesture with a PhaseEnded event (the timer).
func (sf *surface) endGesture() {
	_ = w32.KillTimer(sf.hwnd, timerGesture)
	if !sf.gesture.End() || sf.dead {
		return
	}
	sf.h.Event(shell.ScrollEvent{Pos: sf.scrollPos, Precise: true, Phase: shell.PhaseEnded,
		Mods: modifiers(), Time: sf.s.eventTime()})
}
