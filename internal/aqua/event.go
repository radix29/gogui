package aqua

import (
	"strconv"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// Event is an input event delivered to Element.HandleEvent by a Router: a
// PointerEvent, ScrollEvent, KeyEvent, TextEvent or FocusEvent, or a
// WindowStateEvent sent by the window (not routed); the set is closed.
// Positions are in the receiving element's own coordinates.
type Event interface{ isEvent() }

// Compile-time checks: every event type is an Event.
var (
	_ Event = PointerEvent{}
	_ Event = ScrollEvent{}
	_ Event = KeyEvent{}
	_ Event = TextEvent{}
	_ Event = FocusEvent{}
	_ Event = WindowStateEvent{}
)

// PointerKind is the kind of a PointerEvent.
type PointerKind uint8

const (
	// PointerEnter: the element joined the hover chain (the hit element and
	// its enabled ancestors). Ancestors get it before descendants.
	PointerEnter PointerKind = iota
	// PointerLeave: the element left the hover chain. Descendants get it
	// before ancestors. It is also sent to an element that was just
	// disabled, hidden or removed while hovered, so it can drop its hover
	// state.
	PointerLeave
	// PointerDown: a button was pressed. The first element that handles
	// the press captures the pointer until the last button is released.
	PointerDown
	// PointerMove: the pointer moved, over the element (not captured) or
	// anywhere (captured; see PointerEvent.Over).
	PointerMove
	// PointerUp: a button was released; sent to the capturing element.
	PointerUp
	// PointerCancel: the capture ended without a release, because the
	// capturing element was disabled, hidden or removed. A pressed control
	// returns to normal without acting.
	PointerCancel
)

var pointerKindNames = [...]string{"enter", "leave", "down", "move", "up", "cancel"}

// String returns "enter", "leave", "down", "move", "up" or "cancel".
func (k PointerKind) String() string {
	if int(k) < len(pointerKindNames) {
		return pointerKindNames[k]
	}
	return "kind(" + strconv.Itoa(int(k)) + ")"
}

// PointerEvent is a pointer event routed to one element.
type PointerEvent struct {
	Kind PointerKind

	// Pos is the pointer position in the element's own coordinates. For
	// PointerCancel it is the last known position.
	Pos driver.Point

	// Over reports whether the pointer is over the element: the topmost
	// element under it is the element or one of its descendants. A captured
	// button uses it to un-highlight while dragged off and to not click on
	// a release outside (PLAN.md §5.5). It is true for PointerEnter and
	// PointerDown and false for PointerLeave.
	Over bool

	// Button, Buttons, Clicks and Mods are passed through from the shell
	// event (shell.PointerEvent); they are zero for PointerEnter,
	// PointerLeave and PointerCancel, except Buttons.
	Button  shell.PointerButton
	Buttons shell.PointerButton
	Clicks  int
	Mods    driver.Modifiers

	Time time.Duration
}

// ScrollEvent is a shell.ScrollEvent routed to one element, with Pos in the
// element's coordinates. It goes to the element under the pointer and
// bubbles up until an element handles it; the rest of a precise gesture
// (and its OS momentum) goes to the element that handled its PhaseBegan
// alone (Router latching, S3.5d).
type ScrollEvent struct{ shell.ScrollEvent }

// KeyEvent is a shell.KeyEvent routed to the focused element (or the root
// when nothing is focused). It bubbles up to the root until handled.
type KeyEvent struct{ shell.KeyEvent }

// TextEvent is a shell.TextEvent routed like KeyEvent.
type TextEvent struct{ shell.TextEvent }

// FocusEvent tells an element that it gained (Focused true) or lost the
// window's keyboard focus.
type FocusEvent struct{ Focused bool }

func (PointerEvent) isEvent() {}
func (ScrollEvent) isEvent()  {}
func (KeyEvent) isEvent()     {}
func (TextEvent) isEvent()    {}
func (FocusEvent) isEvent()   {}
