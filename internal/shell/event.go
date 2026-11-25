package shell

import (
	"time"

	"github.com/radix29/gogui/internal/driver"
)

// Event is an input event delivered to SurfaceHandler.Event: a PointerEvent,
// ScrollEvent, KeyEvent, TextEvent or PreeditEvent (the set is closed). Positions are in
// logical points relative to the surface's drawable area; times are on the
// FrameClock.Now timeline.
type Event interface {
	// EventTime returns when the OS reported the event.
	EventTime() time.Duration
	isEvent()
}

// Compile-time checks: every event type is an Event.
var (
	_ Event = PointerEvent{}
	_ Event = ScrollEvent{}
	_ Event = KeyEvent{}
	_ Event = TextEvent{}
	_ Event = PreeditEvent{}
)

// PointerKind is the kind of a PointerEvent.
type PointerKind uint8

const (
	// PointerMove: the pointer moved over the surface, or anywhere while a
	// button is held (see PointerDown).
	PointerMove PointerKind = iota
	// PointerDown: a button was pressed over the surface.
	PointerDown
	// PointerUp: a button was released.
	PointerUp
	// PointerEnter: the pointer entered the surface's drawable area.
	PointerEnter
	// PointerLeave: the pointer left the drawable area (or the shell lost
	// track of it, for example when another window took the capture).
	PointerLeave
)

// PointerButton is a mouse button. The values are bits, so a set of held
// buttons is a PointerButton too.
type PointerButton uint8

const (
	ButtonNone      PointerButton = 0
	ButtonPrimary   PointerButton = 1 << 0 // left, or right when the OS swaps buttons
	ButtonSecondary PointerButton = 1 << 1 // right: context menus
	ButtonMiddle    PointerButton = 1 << 2
	ButtonBack      PointerButton = 1 << 3 // X1
	ButtonForward   PointerButton = 1 << 4 // X2
)

// PointerEvent is a mouse, pen or touchpad-pointer event.
//
// Rules every shell follows:
//
//   - While any button is held after a PointerDown, the pointer is captured:
//     PointerMove and PointerUp keep arriving even outside the surface
//     (positions may then be negative or beyond Size), and no PointerLeave is
//     sent until the last button is released. If the OS cancels the capture,
//     the shell sends PointerUp for every held button, then PointerLeave if
//     the pointer is outside.
//   - PointerEnter comes before the first PointerMove inside the surface.
//     PointerMove events may be coalesced: the shell delivers at least the
//     latest position before each frame.
//   - Presses on a RegionCaption are not delivered (the OS moves the window);
//     see RegionKind.
type PointerEvent struct {
	Kind PointerKind
	Pos  driver.Point

	// Button is the button that changed for PointerDown and PointerUp, and
	// ButtonNone for the other kinds.
	Button PointerButton
	// Buttons is the set of buttons held after the event.
	Buttons PointerButton

	// Clicks is the click count for PointerDown and the matching PointerUp:
	// 1 for a single click, 2 for a double click, 3 for a triple click and
	// so on, counted with the OS's double-click time and distance. It is 0
	// for the other kinds.
	Clicks int

	// Mods is the set of physical modifiers held (never ModPrimary).
	Mods driver.Modifiers

	// Activating marks the PointerDown that activated the surface: the
	// press landed in a surface that was not key, and the shell made it key
	// (KeyChanged(true) usually comes first). It is false for every other
	// event, the PointerUp of that press included. The aqua driver lets
	// such a press only activate the window, as AppKit's default
	// acceptsFirstMouse = NO, except over the title bar's controls (PLAN.md
	// S6.1b). A press whose activation the OS redirects to a modal surface
	// is not delivered to the blocked surface at all. Win32 sets it from
	// WM_MOUSEACTIVATE (S6.1c), after WM_ACTIVATE. gtk4 has no such message
	// and marks the first press while the surface is not key (its
	// KeyChanged(true) may then follow, as on Wayland) or the first press
	// shortly after it became key (S6.1c, internal/shell/gtk4/input.go).
	Activating bool

	Time time.Duration
}

// EventTime returns e.Time.
func (e PointerEvent) EventTime() time.Duration { return e.Time }
func (PointerEvent) isEvent()                   {}

// ScrollPhase is the phase of a touchpad gesture or of the momentum after it
// (PLAN.md §5.6). Wheel mice have no phases.
type ScrollPhase uint8

const (
	// PhaseNone: the event is not part of a gesture (a wheel notch), or, as
	// ScrollEvent.Momentum, not part of OS momentum.
	PhaseNone ScrollPhase = iota
	// PhaseBegan: the first event of a gesture (fingers down and moving) or
	// of OS momentum.
	PhaseBegan
	// PhaseChanged: a further event of the gesture or momentum.
	PhaseChanged
	// PhaseEnded: the gesture or momentum ended (fingers lifted, momentum
	// stopped or was interrupted). Its Delta may be zero.
	PhaseEnded
)

// ScrollEvent is a scroll-wheel or touchpad scroll.
//
// The aqua scroll physics (PLAN.md §5.6) use the phases: a gesture runs
// Phase PhaseBegan, PhaseChanged…, PhaseEnded with Momentum PhaseNone. If
// the OS generates momentum after it (GTK's kinetic scrolling), those events
// follow with Phase PhaseNone and Momentum PhaseBegan, PhaseChanged…,
// PhaseEnded. A shell whose OS reports no momentum (Windows) never sets
// Momentum; the aqua driver then runs its own momentum after PhaseEnded.
// Wheel notches have both phases PhaseNone.
type ScrollEvent struct {
	// Pos is the pointer position.
	Pos driver.Point

	// Delta is how far the content should move, with the OS's scroll
	// direction setting (natural scrolling) already applied: a positive Y
	// moves the content down (towards the top of the document), a positive
	// X moves it right. For a precise event it is in logical points. For a
	// wheel event it is in lines, with the OS's lines-per-notch setting
	// applied and possibly fractional (high-resolution wheels); the aqua
	// driver chooses the line height.
	Delta driver.Point

	// Precise is true for pixel-precise deltas (touchpads, precision
	// wheels): they scroll 1:1. False means wheel line steps, which the aqua
	// driver animates.
	Precise bool

	// Phase is the gesture phase and Momentum the OS momentum phase; at most
	// one of them is not PhaseNone.
	Phase, Momentum ScrollPhase

	// Mods is the set of physical modifiers held (never ModPrimary).
	Mods driver.Modifiers

	Time time.Duration
}

// EventTime returns e.Time.
func (e ScrollEvent) EventTime() time.Duration { return e.Time }
func (ScrollEvent) isEvent()                   {}

// KeyEvent is a key press or release, sent to the key surface.
//
// The order is: KeyEvent{Down: true}, then the TextEvent with the text the
// key produced (if any), and KeyEvent{Down: false} on release. If Event
// returns true for a key-down, the key was consumed (a shortcut, a focus
// move, Escape): the shell drops the text the key would have produced and
// skips the OS default handling (F10 or Alt activating the system menu).
// If it returns false, the OS default handling happens (Alt+F4 closing the
// surface through CloseRequested, Alt+Space opening the system menu) and the
// text follows. Key-ups are not affected by the result.
type KeyEvent struct {
	// Down is true for a press (or a repeat) and false for a release.
	Down bool

	// Key identifies the key as a driver.Key, in canonical form: letters in
	// upper case, other character keys as the unshifted character of the
	// active keyboard layout, the named keys (arrows, Return, F1–F12 …) as
	// the driver constants. It is driver.KeyNone for keys driver.Key cannot
	// name, such as a modifier key alone; Mods still tells which modifiers
	// are held.
	Key driver.Key

	// Shifted is the character key gives with Shift on the active layout,
	// in canonical form (a letter's upper case, '+' for '=' on a US
	// layout, '*' for '+' on a German one), on key-downs of character
	// keys; driver.KeyNone on key-ups, for the named keys and when the
	// layout gives no single character (S5.6h). The menu strip's
	// accelerators match a shortcut on a shifted character with it
	// (menubar.Bar.Lookup). Reading it never disturbs a pending dead key.
	Shifted driver.Key

	// Modifier is the modifier a modifier key alone stands for (S5.6i):
	// exactly one of ModShift, ModControl, ModOption (Alt) and ModCommand
	// (the Windows or Super key), on its presses and its release, left
	// and right keys alike; 0 for every other key (Key is then not
	// KeyNone, or the key is another one driver.Key cannot name, as
	// Insert). AltGr is no modifier on gtk4; Win32 reports it as a
	// Control and an Option key. The menu strip tells a lone Alt with it.
	Modifier driver.Modifiers

	// Repeat is true for auto-repeated presses (Down is then true).
	Repeat bool

	// Mods is the set of physical modifiers held during the event (never
	// ModPrimary), as after it: a modifier key's own modifier is in Mods
	// on its press and not on its release (S5.6i). The aqua driver
	// resolves shortcuts against it with driver.Shortcut.Resolve(false).
	Mods driver.Modifiers

	Time time.Duration
}

// EventTime returns e.Time.
func (e KeyEvent) EventTime() time.Duration { return e.Time }
func (KeyEvent) isEvent()                   {}

// TextEvent is committed text input: typed characters, dead-key results
// and IME commits. It carries no control characters: Return, Tab, Backspace
// and Escape arrive only as KeyEvents. The IME's composition text before
// the commit comes as PreeditEvents; a TextEvent during a composition is
// its commit (see PreeditEvent).
type TextEvent struct {
	// Text is the committed text, valid UTF-8, never empty. It may hold
	// several characters (a surrogate pair joined, an IME commit).
	Text string

	Time time.Duration
}

// EventTime returns e.Time.
func (e TextEvent) EventTime() time.Duration { return e.Time }
func (TextEvent) isEvent()                   {}

// PreeditEvent is the IME's composition (preedit) text, sent to the key
// surface while a text input is enabled (Surface.SetTextInput). The aqua
// driver draws it at the caret; it is not part of the text until the IME
// commits it.
//
// The rules every shell follows:
//
//   - A composition starts with the first PreeditEvent with a non-empty
//     Text. Each further PreeditEvent replaces the whole preedit.
//   - A PreeditEvent with an empty Text ends the composition (Cursor and
//     Segments are then zero). The IME may send one when nothing is
//     composed; the aqua driver ignores it then.
//   - The commit is a TextEvent. A TextEvent during a composition replaces
//     the preedit, which counts as empty until the next PreeditEvent. So a
//     commit may come before or after the empty PreeditEvent that ends the
//     composition, and a commit followed by a non-empty PreeditEvent starts
//     the next composition (Win32 reports both in one message, the commit
//     first).
//   - Keys that the IME takes (VK_PROCESSKEY on Windows, the keys the GTK
//     input method filters) are not delivered as KeyEvents.
//   - Surface.SetTextInput with Enabled false and Surface.CancelComposition
//     drop the composition without any event, not even a commit.
type PreeditEvent struct {
	// Text is the composition text, valid UTF-8 without control
	// characters; empty ends the composition.
	Text string

	// Cursor is the IME's caret in Text as a UTF-8 byte offset on a rune
	// start, from 0 to len(Text).
	Cursor int

	// Segments are the clauses of Text, in order and not overlapping, with
	// byte offsets on rune starts and Start < End. They need not cover
	// Text; an IME that reports no clauses sends none, and the aqua driver
	// then underlines the whole text as one clause.
	Segments []PreeditSegment

	Time time.Duration
}

// PreeditSegment is one clause of a PreeditEvent: Text[Start:End].
type PreeditSegment struct {
	Start, End int
	// Selected marks the clause being converted (the target clause),
	// which the aqua driver underlines thicker.
	Selected bool
}

// EventTime returns e.Time.
func (e PreeditEvent) EventTime() time.Duration { return e.Time }
func (PreeditEvent) isEvent()                   {}
