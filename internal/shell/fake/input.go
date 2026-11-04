package fake

import (
	"math"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// Click counting of the fake input: a press of the same button within
// DoubleClickTime of the previous press (on the clock's Now timeline) and
// within DoubleClickDistance of it on each axis counts as the next click.
// Time moves only with Clock.Advance, so two Click calls without an Advance
// between them are a double click; advance by DoubleClickTime to get two
// single clicks.
const (
	DoubleClickTime     = 500 * time.Millisecond
	DoubleClickDistance = 4.0
)

// pointer is the input state of a surface.
type pointer struct {
	pos     driver.Point
	inside  bool // PointerEnter sent, PointerLeave not yet
	buttons shell.PointerButton
	caption bool // a primary press on RegionCaption went to the OS
	mods    driver.Modifiers

	lastButton shell.PointerButton
	lastPos    driver.Point
	lastTime   time.Duration
	clicks     int
}

// input checks that input may be delivered: on the UI goroutine, not
// during a Paint, to an open, visible, not minimized surface. It returns
// false (the input is dropped) for a closed, hidden or minimized surface.
func (sf *Surface) input(what string) bool {
	if !sf.change(what) {
		return false
	}
	return sf.visible && !sf.minimized
}

func (sf *Surface) now() time.Duration { return sf.shell.clock.now }

// in reports whether p lies in the drawable area.
func (sf *Surface) in(p driver.Point) bool {
	return p.X >= 0 && p.Y >= 0 && p.X < sf.size.W && p.Y < sf.size.H
}

func (sf *Surface) pointerEvent(kind shell.PointerKind, button shell.PointerButton, clicks int) {
	sf.h.Event(shell.PointerEvent{
		Kind: kind, Pos: sf.ptr.pos, Button: button, Buttons: sf.ptr.buttons,
		Clicks: clicks, Mods: sf.ptr.mods, Time: sf.now(),
	})
}

// moveTo moves the pointer to p with the events a shell sends: PointerEnter
// before the first move inside, PointerMove while inside or captured, and
// PointerLeave when it leaves with no button held.
func (sf *Surface) moveTo(p driver.Point) {
	if math.IsNaN(p.X) || math.IsNaN(p.Y) {
		panic("fake: pointer position is NaN")
	}
	moved := p != sf.ptr.pos
	sf.ptr.pos = p
	switch {
	case sf.ptr.buttons != 0: // captured
		if moved {
			sf.pointerEvent(shell.PointerMove, shell.ButtonNone, 0)
		}
	case sf.in(p):
		if !sf.ptr.inside {
			sf.ptr.inside = true
			sf.pointerEvent(shell.PointerEnter, shell.ButtonNone, 0)
			sf.pointerEvent(shell.PointerMove, shell.ButtonNone, 0)
		} else if moved {
			sf.pointerEvent(shell.PointerMove, shell.ButtonNone, 0)
		}
	case sf.ptr.inside:
		sf.ptr.inside = false
		sf.pointerEvent(shell.PointerLeave, shell.ButtonNone, 0)
	}
}

// Move moves the pointer to p (see PointerEvent for the enter, leave and
// capture rules).
func (sf *Surface) Move(p driver.Point) {
	if sf.input("Move") {
		sf.moveTo(p)
	}
}

// Pointer returns the pointer position, whether the pointer is inside the
// surface (between PointerEnter and PointerLeave) and the held buttons.
func (sf *Surface) Pointer() (pos driver.Point, inside bool, buttons shell.PointerButton) {
	sf.check("Pointer")
	return sf.ptr.pos, sf.ptr.inside, sf.ptr.buttons
}

// SetModifiers sets the physical modifiers held during later pointer and
// scroll events. It panics for ModPrimary.
func (sf *Surface) SetModifiers(m driver.Modifiers) {
	sf.check("SetModifiers")
	noPrimary(m)
	sf.ptr.mods = m
}

func noPrimary(m driver.Modifiers) {
	if m&driver.ModPrimary != 0 {
		panic("fake: ModPrimary in an event (shells report physical modifiers)")
	}
}

func singleButton(b shell.PointerButton) {
	if b == 0 || b&(b-1) != 0 || b > shell.ButtonForward {
		panic("fake: pointer button must be exactly one button")
	}
}

// Press moves the pointer to p and presses button b there. A primary press
// on a RegionCaption is not delivered: it starts the OS window move, and a
// double-click there zooms the surface. Pressing a held button does
// nothing.
func (sf *Surface) Press(p driver.Point, b shell.PointerButton) {
	singleButton(b)
	if !sf.input("Press") {
		return
	}
	sf.moveTo(p)
	if sf.ptr.buttons&b != 0 || (b == shell.ButtonPrimary && sf.ptr.caption) {
		return
	}
	if !sf.in(p) && sf.ptr.buttons == 0 {
		return // presses outside go to another window
	}
	clicks := sf.countClick(p, b)
	if b == shell.ButtonPrimary && sf.ptr.buttons == 0 && sf.HitTest(p) == shell.RegionCaption {
		sf.ptr.caption = true
		if clicks >= 2 {
			sf.Zoom()
		}
		return
	}
	sf.ptr.buttons |= b
	sf.pointerEvent(shell.PointerDown, b, clicks)
}

func (sf *Surface) countClick(p driver.Point, b shell.PointerButton) int {
	pt := &sf.ptr
	now := sf.now()
	if pt.clicks > 0 && b == pt.lastButton && now-pt.lastTime <= DoubleClickTime &&
		math.Abs(p.X-pt.lastPos.X) <= DoubleClickDistance && math.Abs(p.Y-pt.lastPos.Y) <= DoubleClickDistance {
		pt.clicks++
	} else {
		pt.clicks = 1
	}
	pt.lastButton, pt.lastPos, pt.lastTime = b, p, now
	return pt.clicks
}

// Release moves the pointer to p and releases button b. Releasing the last
// held button outside the surface also sends PointerLeave. Releasing a
// button that is not held does nothing.
func (sf *Surface) Release(p driver.Point, b shell.PointerButton) {
	singleButton(b)
	if !sf.input("Release") {
		return
	}
	if b == shell.ButtonPrimary && sf.ptr.caption {
		sf.ptr.caption = false
		sf.ptr.pos = p // the OS move loop had the pointer
		return
	}
	sf.moveTo(p)
	if sf.ptr.buttons&b == 0 {
		return
	}
	sf.ptr.buttons &^= b
	clicks := 0
	if b == sf.ptr.lastButton {
		clicks = sf.ptr.clicks
	}
	sf.pointerEvent(shell.PointerUp, b, clicks)
	sf.leaveIfOutside()
}

func (sf *Surface) leaveIfOutside() {
	if sf.ptr.buttons == 0 && sf.ptr.inside && !sf.in(sf.ptr.pos) {
		sf.ptr.inside = false
		sf.pointerEvent(shell.PointerLeave, shell.ButtonNone, 0)
	}
}

// Click presses and releases the primary button at p.
func (sf *Surface) Click(p driver.Point) { sf.ClickButton(p, shell.ButtonPrimary) }

// ClickButton presses and releases button b at p.
func (sf *Surface) ClickButton(p driver.Point, b shell.PointerButton) {
	sf.Press(p, b)
	sf.Release(p, b)
}

// DoubleClick clicks twice at p without moving time.
func (sf *Surface) DoubleClick(p driver.Point) {
	sf.Click(p)
	sf.Click(p)
}

// Drag presses the primary button at from, moves through the points of via
// and releases it at to.
func (sf *Surface) Drag(from, to driver.Point, via ...driver.Point) {
	sf.Press(from, shell.ButtonPrimary)
	for _, p := range via {
		sf.Move(p)
	}
	sf.Release(to, shell.ButtonPrimary)
}

// CancelCapture simulates the OS cancelling the pointer capture (another
// window took it): PointerUp for every held button, then PointerLeave if
// the pointer is outside.
func (sf *Surface) CancelCapture() {
	if !sf.input("CancelCapture") {
		return
	}
	for b := shell.ButtonPrimary; b <= shell.ButtonForward; b <<= 1 {
		if sf.ptr.buttons&b != 0 {
			sf.ptr.buttons &^= b
			sf.pointerEvent(shell.PointerUp, b, 0)
		}
	}
	sf.ptr.caption = false
	sf.leaveIfOutside()
}

func (sf *Surface) scrollEvent(ev shell.ScrollEvent) {
	ev.Pos, ev.Mods, ev.Time = sf.ptr.pos, sf.ptr.mods, sf.now()
	sf.h.Event(ev)
}

// Wheel moves the pointer to p and turns the wheel by delta lines (no
// phases, not precise).
func (sf *Surface) Wheel(p driver.Point, delta driver.Point) {
	if sf.input("Wheel") {
		sf.moveTo(p)
		sf.scrollEvent(shell.ScrollEvent{Delta: delta})
	}
}

// Swipe moves the pointer to p and sends a precise touchpad gesture, as on
// Windows: PhaseBegan with the first delta, PhaseChanged with each further
// one, and PhaseEnded with a zero delta. Without deltas it does nothing.
func (sf *Surface) Swipe(p driver.Point, deltas ...driver.Point) {
	if !sf.input("Swipe") || len(deltas) == 0 {
		return
	}
	sf.moveTo(p)
	for i, d := range deltas {
		ph := shell.PhaseChanged
		if i == 0 {
			ph = shell.PhaseBegan
		}
		sf.scrollEvent(shell.ScrollEvent{Delta: d, Precise: true, Phase: ph})
	}
	sf.scrollEvent(shell.ScrollEvent{Precise: true, Phase: shell.PhaseEnded})
}

// SwipeEndDelay is how long after the last delta of a TimedSwipe the
// PhaseEnded comes: the Win32 shell's gesture timeout (shell/win32
// wheel.go), which ends a gesture only when no precise event came for that
// long.
const SwipeEndDelay = 100 * time.Millisecond

// TimedSwipe sends a precise touchpad gesture like Swipe, but over time, as
// the Win32 shell delivers it: the clock advances by interval between two
// deltas (running the frames due on the way, Clock.Advance), and the
// zero-delta PhaseEnded follows SwipeEndDelay after the last delta. Each
// event carries the clock's Now. The gesture stops early when a frame
// closes, hides or minimizes the surface. Without deltas it does nothing;
// it panics for a negative interval and when called from inside a frame.
func (sf *Surface) TimedSwipe(p driver.Point, interval time.Duration, deltas ...driver.Point) {
	if interval < 0 {
		panic("fake: TimedSwipe with a negative interval")
	}
	if !sf.input("TimedSwipe") || len(deltas) == 0 {
		return
	}
	sf.moveTo(p)
	clk := sf.shell.clock
	for i, d := range deltas {
		ph := shell.PhaseChanged
		if i == 0 {
			ph = shell.PhaseBegan
		} else {
			clk.Advance(interval)
			if !sf.input("TimedSwipe") {
				return
			}
		}
		sf.scrollEvent(shell.ScrollEvent{Delta: d, Precise: true, Phase: ph})
	}
	clk.Advance(SwipeEndDelay)
	if sf.input("TimedSwipe") {
		sf.scrollEvent(shell.ScrollEvent{Precise: true, Phase: shell.PhaseEnded})
	}
}

// Momentum sends OS momentum after a gesture, as on GTK: Momentum
// PhaseBegan with the first delta, PhaseChanged with each further one, and
// PhaseEnded with a zero delta, all precise, at the current pointer
// position. Without deltas it does nothing.
func (sf *Surface) Momentum(deltas ...driver.Point) {
	if !sf.input("Momentum") || len(deltas) == 0 {
		return
	}
	for i, d := range deltas {
		ph := shell.PhaseChanged
		if i == 0 {
			ph = shell.PhaseBegan
		}
		sf.scrollEvent(shell.ScrollEvent{Delta: d, Precise: true, Momentum: ph})
	}
	sf.scrollEvent(shell.ScrollEvent{Precise: true, Momentum: shell.PhaseEnded})
}

// keyInput checks like input and that the surface is key: key events go to
// the key surface only, so sending them elsewhere is a test bug.
func (sf *Surface) keyInput(what string) bool {
	if !sf.input(what) {
		return false
	}
	if sf.shell.key != sf {
		panic("fake: Surface." + what + " on a surface that is not key (call Show or Activate first)")
	}
	return true
}

// KeyDown sends a key press (Repeat false) and reports whether it was
// handled. It panics if the surface is not key, and for ModPrimary.
func (sf *Surface) KeyDown(k driver.Key, mods driver.Modifiers) bool {
	noPrimary(mods)
	if !sf.keyInput("KeyDown") {
		return false
	}
	return sf.h.Event(shell.KeyEvent{Down: true, Key: driver.NormalizeKey(k), Mods: mods, Time: sf.now()})
}

// KeyRepeat sends an auto-repeated press and reports whether it was
// handled.
func (sf *Surface) KeyRepeat(k driver.Key, mods driver.Modifiers) bool {
	noPrimary(mods)
	if !sf.keyInput("KeyRepeat") {
		return false
	}
	return sf.h.Event(shell.KeyEvent{Down: true, Repeat: true, Key: driver.NormalizeKey(k), Mods: mods, Time: sf.now()})
}

// KeyUp sends a key release. Unlike the presses it is dropped silently
// when the surface is no longer key (a handler moved the focus to another
// surface during the press).
func (sf *Surface) KeyUp(k driver.Key, mods driver.Modifiers) {
	noPrimary(mods)
	if sf.input("KeyUp") && sf.shell.key == sf {
		sf.h.Event(shell.KeyEvent{Key: driver.NormalizeKey(k), Mods: mods, Time: sf.now()})
	}
}

// Key presses and releases k with mods and reports whether the press was
// handled. It sends no text (shortcuts, arrows, Return, Escape).
func (sf *Surface) Key(k driver.Key, mods driver.Modifiers) bool {
	handled := sf.KeyDown(k, mods)
	sf.KeyUp(k, mods)
	return handled
}

// Type types text one rune at a time, as a keyboard with a US layout would
// deliver it: KeyEvent down, TextEvent with the rune unless the key-down
// was handled, KeyEvent up. Upper-case letters hold ModShift; '\n' is
// KeyReturn, '\t' KeyTab and '\b' KeyBackspace, without text. Other runes
// map to the key of the same character with no modifier (the fake knows no
// layout). It panics for invalid UTF-8 and other control characters.
func (sf *Surface) Type(text string) {
	if !utf8.ValidString(text) {
		panic("fake: Type with invalid UTF-8")
	}
	for _, r := range text {
		var k driver.Key
		var mods driver.Modifiers
		emit := true
		switch {
		case r == '\n':
			k, emit = driver.KeyReturn, false
		case r == '\t':
			k, emit = driver.KeyTab, false
		case r == '\b':
			k, emit = driver.KeyBackspace, false
		case unicode.IsControl(r):
			panic("fake: Type with a control character")
		case r == ' ':
			k = driver.KeySpace
		case unicode.IsUpper(r):
			k, mods = driver.Key(r), driver.ModShift
		default:
			k = driver.NormalizeKey(driver.Key(r))
		}
		if sf.KeyDown(k, mods) {
			emit = false
		}
		if emit && sf.input("Type") && sf.shell.key == sf {
			sf.h.Event(shell.TextEvent{Text: string(r), Time: sf.now()})
		}
		sf.KeyUp(k, mods)
		if sf.dead || sf.shell.key != sf {
			return // the handler closed the surface or moved the focus
		}
	}
}

// Commit sends committed text as one TextEvent with no key events, like an
// IME commit or a paste from an input method. It panics for empty text,
// invalid UTF-8 and control characters.
func (sf *Surface) Commit(text string) {
	if text == "" || !utf8.ValidString(text) {
		panic("fake: Commit with empty text or invalid UTF-8")
	}
	for _, r := range text {
		if unicode.IsControl(r) {
			panic("fake: Commit with a control character")
		}
	}
	if sf.keyInput("Commit") {
		sf.h.Event(shell.TextEvent{Text: text, Time: sf.now()})
	}
}

// Send delivers ev as it is, for sequences the helpers do not cover. A zero
// Time is set to the clock's Now. It reports what the handler returned. It
// panics for ModPrimary, and for key and text events when the surface is
// not key.
func (sf *Surface) Send(ev shell.Event) bool {
	switch e := ev.(type) {
	case shell.PointerEvent:
		noPrimary(e.Mods)
		if e.Time == 0 {
			e.Time = sf.now()
		}
		ev = e
	case shell.ScrollEvent:
		noPrimary(e.Mods)
		if e.Time == 0 {
			e.Time = sf.now()
		}
		ev = e
	case shell.KeyEvent:
		noPrimary(e.Mods)
		if e.Time == 0 {
			e.Time = sf.now()
		}
		ev = e
	case shell.TextEvent:
		if e.Time == 0 {
			e.Time = sf.now()
		}
		ev = e
	}
	switch ev.(type) {
	case shell.KeyEvent, shell.TextEvent:
		if !sf.keyInput("Send") {
			return false
		}
	default:
		if !sf.input("Send") {
			return false
		}
	}
	return sf.h.Event(ev)
}
