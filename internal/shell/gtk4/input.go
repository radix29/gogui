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
// over all the same.
//
// Click counts are the shell's (shellkit.ClickCounter) with GTK's
// gtk-double-click-time and gtk-double-click-distance, so triple clicks
// work as on Win32. Modifiers come from the event's GdkModifierType, times
// from its event time mapped onto the clock (eventTime).
//
// The first click (S6.1c). GTK has no WM_MOUSEACTIVATE: the window
// manager or compositor activates the window on the press, and
// notify::is-active may come before the press (X11 window managers that
// grab and replay the click, usually) or after it (the activation is a
// configure of its own, so on Wayland it can follow the button). A
// delivered press of a toplevel surface is Activating when
//
//   - it arrives while the surface is not key, and it is the first press
//     since the surface lost the key: the activation is expected to
//     follow. A second press while still not key is not marked, so a
//     window manager that never activates on click (or a focus that
//     follows the mouse elsewhere) leaves the window usable; or
//   - it is the first press after is-active turned true, within
//     firstClickWindow, and no press came while the surface was not key.
//
// Any press (the caption's and unknown buttons' included) uses up the
// mark; a key loss resets it. KeyChanged(true) therefore precedes an
// Activating press only in the second case. A blocked surface gets no
// presses (modal.go); a popup's presses are never marked.

// firstClickWindow is how long after the activation a press still counts
// as the activating one.
const firstClickWindow = 500 * time.Millisecond

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
	// mods are the modifiers of the last pointer event.
	mods driver.Modifiers
	// The first click (above): armed is set when is-active turned true
	// (at armedAt) with no press since the key loss, until the next press;
	// pressedInactive is set by a press while the surface is not key.
	armed           bool
	armedAt         time.Duration
	pressedInactive bool
}

// firstClick reports whether the press being handled is the activating
// one (the first click above) and uses up the mark.
func (sf *surface) firstClick() bool {
	p := &sf.ptr
	armed := p.armed && sf.s.clock.now()-p.armedAt <= firstClickWindow
	p.armed = false
	if sf.pop != nil {
		return false
	}
	if !sf.key {
		first := !p.pressedInactive
		p.pressedInactive = true
		return first
	}
	return armed
}

// keyMarks follows a change of the reported key state for firstClick.
func (sf *surface) keyMarks(key bool) {
	p := &sf.ptr
	p.armed = key && !p.pressedInactive
	p.armedAt = sf.s.clock.now()
	p.pressedInactive = false
}

func buttonIndex(b shell.PointerButton) int { return bits.TrailingZeros8(uint8(b)) }

// pointer delivers a pointer event and reports whether the surface is
// still open afterwards.
func (sf *surface) pointer(kind shell.PointerKind, pos driver.Point, b shell.PointerButton, clicks int, c uintptr) bool {
	return sf.deliver(kind, pos, b, clicks, c, false)
}

// deliver is pointer with the event's Activating flag (press).
func (sf *surface) deliver(kind shell.PointerKind, pos driver.Point, b shell.PointerButton, clicks int, c uintptr, activating bool) bool {
	if sf.dead {
		return false
	}
	sf.ptr.lastPos = pos
	// c is 0 for the events a popup's grab makes up (popup.go): the
	// modifiers of the last event, the time now.
	var t uint32
	if c != 0 {
		sf.ptr.mods = modifiersOf(sf.w.eventState(c))
		t = sf.w.eventTime(c)
	}
	sf.h.Event(shell.PointerEvent{Kind: kind, Pos: pos, Button: b, Buttons: sf.ptr.buttons,
		Clicks: clicks, Mods: sf.ptr.mods, Activating: activating, Time: sf.s.eventTime(t)})
	return !sf.dead
}

// pointerLive reports whether pointer input may reach the handler (not
// while a modal surface blocks it, modal.go).
func (sf *surface) pointerLive() bool {
	return !sf.dead && !sf.creating && sf.w != nil && !sf.inputBlocked()
}

// enterAt sends PointerEnter at pos if the pointer was outside.
func (sf *surface) enterAt(pos driver.Point, c uintptr) bool {
	if !sf.outside(pos) {
		if sf.pop != nil {
			sf.pop.cancelLeave() // back in the area before a deferred leave
		}
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
		if sf.pop != nil {
			sf.pop.cancelLeave()
		}
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

// outside reports whether pos lies outside the drawable area; for a grab
// popup's child, outside its area: its own rect and the track (popup.go,
// S5.6e).
func (sf *surface) outside(pos driver.Point) bool {
	return !sf.inOwn(pos) && (sf.pop == nil || !sf.pop.onTrack(pos))
}

// inOwn reports whether pos lies in the drawable area itself.
func (sf *surface) inOwn(pos driver.Point) bool {
	return (driver.Rect{W: sf.size.W, H: sf.size.H}).Contains(pos)
}

// press is the click gesture's "pressed" of GDK button gb at pos.
func (sf *surface) press(gb uint32, pos driver.Point, c uintptr) {
	if !sf.stopMomentum() { // a press stops the scroll momentum
		return
	}
	sf.ptr.caption = false // a new press: an old caption press has ended
	activating := sf.firstClick()
	b := pointerButton(gb)
	if p := sf.pop; p != nil && p.inherited&b != 0 { // its release never came: drop it
		p.inherited &^= b
		sf.ptr.buttons &^= b
	}
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
	sf.deliver(shell.PointerDown, pos, b, n, c, activating)
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
	if p := sf.pop; p != nil {
		p.inherited &^= b
	}
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
