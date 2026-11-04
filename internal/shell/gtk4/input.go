package gtk4

import (
	"math"
	"math/bits"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/shellkit"
)

// Pointer input (S4.1i). Two controllers on the GGSurface, added by
// window_linux.go at creation, report the pointer over the drawable area
// (the resize borders lie in GTK's CSD shadow, outside it):
//
//   - A GtkEventControllerMotion: "enter" is a PointerEnter, "motion" a
//     PointerMove (after a PointerEnter if the pointer was outside; a
//     move to the same position is dropped), "leave" a PointerLeave.
//   - A GtkGestureClick for every button (button 0): "pressed" is a
//     PointerDown and "released" a PointerUp of the gesture's current
//     button; "cancel" (another gesture claimed the press, a grab broke it,
//     or a second button reset it) ends every held button with a PointerUp.
//
// Capture is GTK's implicit grab: while a button is held, the GGSurface
// keeps getting the motion and the release, also outside the window. A
// "leave" while a button is held is kept until the last release (the
// capture rule of shell.PointerEvent); so is a release outside the
// drawable area.
//
// Chords. GtkGestureClick is a GtkGestureSingle: a press of a second
// button while one is held resets it (GTK drops that press), so the held
// button gets its PointerUp at once and the second button is not
// reported. One held button at a time is what GTK's gestures give.
//
// The caption. A primary press on RegionCaption belongs to the caption
// gestures (chrome.go): it is not delivered, nor its release or cancel;
// moves and hover over the caption are. The press breaks a click series.
// When the caption drag claims the press (beginMove), GTK denies it to
// this gesture without a "cancel" or a "released", so beginMove resets
// the gesture, which cancels it; a new press clears a caption press left
// over all the same.//
// Click counts are the shell's (shellkit.ClickCounter) with GTK's
// gtk-double-click-time and gtk-double-click-distance, so triple clicks
// work as on Win32. Modifiers come from the event's GdkModifierType, times
// from its event time mapped onto the clock (eventTime).

// GDK's button numbers (gdk_button_event_get_button).
const (
	gdkButtonPrimary   = 1
	gdkButtonMiddle    = 2
	gdkButtonSecondary = 3
	gdkButtonBack      = 8
	gdkButtonForward   = 9
)

// pointerButton maps a GDK button number to a PointerButton (ButtonNone
// for the others).
func pointerButton(b uint32) shell.PointerButton {
	switch b {
	case gdkButtonPrimary:
		return shell.ButtonPrimary
	case gdkButtonMiddle:
		return shell.ButtonMiddle
	case gdkButtonSecondary:
		return shell.ButtonSecondary
	case gdkButtonBack:
		return shell.ButtonBack
	case gdkButtonForward:
		return shell.ButtonForward
	}
	return shell.ButtonNone
}

// GdkModifierType bits (gdkenums.h) the shell reads.
const (
	gdkShiftMask   = 1 << 0
	gdkControlMask = 1 << 2
	gdkAltMask     = 1 << 3
	gdkSuperMask   = 1 << 26
	gdkHyperMask   = 1 << 27
	gdkMetaMask    = 1 << 28
)

// modifiersOf maps a GdkModifierType to the physical modifiers: Alt (and
// Meta, which X11 reports with Alt) is Option, Super (and Hyper) is
// Command, as the Windows key is on Win32.
func modifiersOf(state uint32) driver.Modifiers {
	var m driver.Modifiers
	if state&gdkShiftMask != 0 {
		m |= driver.ModShift
	}
	if state&gdkControlMask != 0 {
		m |= driver.ModControl
	}
	if state&(gdkAltMask|gdkMetaMask) != 0 {
		m |= driver.ModOption
	}
	if state&(gdkSuperMask|gdkHyperMask) != 0 {
		m |= driver.ModCommand
	}
	return m
}

// eventTime maps a GDK event time onto the clock. GDK's times are
// milliseconds of the windowing system: CLOCK_MONOTONIC on Wayland
// compositors and on Xorg, the timeline of g_get_monotonic_time, so the
// age is taken against it (shellkit.EventTimeAt; a time from another
// timeline, or 0 for none, is older than MaxEventAge and gives now). The
// times handed out never go backwards.
func (s *Shell) eventTime(evTime uint32) time.Duration {
	us := s.loop.monotonic()
	tick := uint32(us / 1000)
	return s.events.Next(shellkit.EventTimeAt(s.clock.at(us), tick, evTime))
}

// pointerState is a surface's pointer: whether it is over the drawable
// area, where it was last, the buttons held and their click counts.
type pointerState struct {
	inside  bool
	lastPos driver.Point
	buttons shell.PointerButton
	// upClicks are the click counts of the held buttons' presses, for
	// their PointerUp (by bits.TrailingZeros8 of the button).
	upClicks [5]int
	clicks   shellkit.ClickCounter
	// left: GTK reported a leave (or the release was outside) while a
	// button was held; PointerLeave follows the last release.
	left bool
	// caption: the gesture's press is a primary press on RegionCaption,
	// which is not delivered.
	caption bool
}

func buttonIndex(b shell.PointerButton) int { return bits.TrailingZeros8(uint8(b)) }

// pointer delivers a pointer event and reports whether the surface is
// still open afterwards.
func (sf *surface) pointer(kind shell.PointerKind, pos driver.Point, b shell.PointerButton, clicks int, c uintptr) bool {
	if sf.dead {
		return false
	}
	sf.ptr.lastPos = pos
	sf.h.Event(shell.PointerEvent{Kind: kind, Pos: pos, Button: b, Buttons: sf.ptr.buttons,
		Clicks: clicks, Mods: modifiersOf(sf.w.eventState(c)), Time: sf.s.eventTime(sf.w.eventTime(c))})
	return !sf.dead
}

// pointerLive reports whether pointer input may reach the handler.
func (sf *surface) pointerLive() bool { return !sf.dead && !sf.creating && sf.w != nil }

// enterAt sends PointerEnter at pos if the pointer was outside.
func (sf *surface) enterAt(pos driver.Point, c uintptr) bool {
	if !sf.outside(pos) {
		sf.ptr.left = false // back inside: a held button's leave is undone
	}
	if sf.ptr.inside {
		return true
	}
	sf.ptr.inside = true
	return sf.pointer(shell.PointerEnter, pos, shell.ButtonNone, 0, c)
}

// motion is the motion controller's "motion": a PointerMove, after a
// PointerEnter if the pointer was outside, none if it did not move.
func (sf *surface) motion(pos driver.Point, c uintptr) {
	if sf.ptr.inside && pos == sf.ptr.lastPos {
		return
	}
	if sf.enterAt(pos, c) {
		sf.pointer(shell.PointerMove, pos, shell.ButtonNone, 0, c)
	}
}

// leave is the motion controller's "leave": a PointerLeave, or, while a
// button is held, one after the last release.
func (sf *surface) leave(c uintptr) {
	if !sf.ptr.inside {
		return
	}
	if sf.ptr.buttons != 0 || sf.ptr.caption {
		sf.ptr.left = true
		return
	}
	sf.ptr.inside, sf.ptr.left = false, false
	sf.pointer(shell.PointerLeave, sf.ptr.lastPos, shell.ButtonNone, 0, c)
}

// leaveIfLeft sends the PointerLeave kept while a button was held.
func (sf *surface) leaveIfLeft(c uintptr) {
	if sf.ptr.left && sf.ptr.buttons == 0 && !sf.ptr.caption && sf.ptr.inside {
		sf.ptr.inside, sf.ptr.left = false, false
		sf.pointer(shell.PointerLeave, sf.ptr.lastPos, shell.ButtonNone, 0, c)
	}
}

// outside reports whether pos lies outside the drawable area.
func (sf *surface) outside(pos driver.Point) bool {
	return !(driver.Rect{W: sf.size.W, H: sf.size.H}).Contains(pos)
}

// press is the click gesture's "pressed" of GDK button gb at pos.
func (sf *surface) press(gb uint32, pos driver.Point, c uintptr) {
	if !sf.stopMomentum() { // a press stops the scroll momentum
		return
	}
	sf.ptr.caption = false // a new press: an old caption press has ended
	b := pointerButton(gb)
	if b == shell.ButtonNone || sf.ptr.buttons&b != 0 {
		return
	}
	if b == shell.ButtonPrimary && sf.captionAt(pos) {
		sf.ptr.caption = true
		sf.ptr.clicks.Reset() // the caption's press is the window's
		return
	}
	if !sf.enterAt(pos, c) {
		return
	}
	pt := shellkit.Point{X: int32(math.Round(pos.X)), Y: int32(math.Round(pos.Y))}
	n := sf.ptr.clicks.Press(b, pt, sf.w.eventTime(c), sf.w.clickRules(), false)
	sf.ptr.buttons |= b
	sf.ptr.upClicks[buttonIndex(b)] = n
	sf.pointer(shell.PointerDown, pos, b, n, c)
}

// release is the click gesture's "released" at pos: the release of the
// gesture's button gb, or, when GTK no longer names it, of the button the
// gesture holds (a GtkGestureSingle holds one).
func (sf *surface) release(gb uint32, pos driver.Point, c uintptr) {
	if sf.ptr.caption {
		sf.ptr.caption = false
		sf.leaveIfLeft(c)
		return
	}
	b := pointerButton(gb)
	if b == shell.ButtonNone || sf.ptr.buttons&b == 0 {
		b = sf.ptr.buttons & -sf.ptr.buttons
	}
	if b == 0 {
		return
	}
	sf.ptr.buttons &^= b
	if sf.ptr.buttons == 0 && sf.outside(pos) {
		sf.ptr.left = true
	}
	if !sf.pointer(shell.PointerUp, pos, b, sf.ptr.upClicks[buttonIndex(b)], c) {
		return
	}
	sf.leaveIfLeft(c)
}

// cancel is the click gesture's "cancel": a PointerUp for every held
// button at the last position, then the PointerLeave kept while they were
// held, or because the pointer is outside. A caption press just ends.
func (sf *surface) cancel(c uintptr) {
	sf.ptr.caption = false
	for sf.ptr.buttons != 0 {
		b := sf.ptr.buttons & -sf.ptr.buttons // the lowest held button
		sf.ptr.buttons &^= b
		if !sf.pointer(shell.PointerUp, sf.ptr.lastPos, b, sf.ptr.upClicks[buttonIndex(b)], c) {
			return
		}
	}
	if sf.outside(sf.ptr.lastPos) {
		sf.ptr.left = true
	}
	sf.leaveIfLeft(c)
}
