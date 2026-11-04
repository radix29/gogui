package aqua

import (
	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/driver"
)

// WindowStateEvent tells an element that its window's state changed: the
// window became key or inactive, or its effective appearance changed
// (S3.2c). Key and Appearance are the window's state when the event is
// delivered.
//
// The window sends it to every element of its tree in pre-order (children
// in paint order), hidden and disabled ones included, after invalidating
// the whole tree; it does not bubble, and the result of HandleEvent is
// ignored. Controls use it to restart or drop animations (a focus ring
// disappears in an inactive window). An element that joins a window later
// gets no event: it reads the current state from Base.Window.
type WindowStateEvent struct {
	Key        bool
	Appearance driver.Appearance
}

func (WindowStateEvent) isEvent() {}

// FocusState is how an element holds the keyboard focus (Router.FocusState).
type FocusState uint8

const (
	// Unfocused: the element is not its router's focused element.
	Unfocused FocusState = iota
	// FocusedInactive: the element is the focused element, but its window
	// is not key. The focus is kept and comes back with the key state; a
	// control draws no focus ring meanwhile.
	FocusedInactive
	// FocusedActive: the element is the focused element of the key window.
	FocusedActive
)

var focusStateNames = [...]string{"unfocused", "focused-inactive", "focused"}

// String returns "unfocused", "focused-inactive" or "focused".
func (s FocusState) String() string {
	if int(s) < len(focusStateNames) {
		return focusStateNames[s]
	}
	return "focusstate(?)"
}

// Window returns the window whose tree the element is in, or nil when the
// element is not in a window's tree (detached, or under a root made with
// NewRouter directly). Through it an element reads the window context:
// IsKey, Appearance, Theme, Timeline and Router. A closed window keeps
// answering with its last state.
func (b *Base) Window() *Window {
	e := b
	for e.parent != nil {
		e = e.parent
	}
	if r, ok := e.self.(*windowRoot); ok {
		return r.w
	}
	return nil
}

// FocusState reports how the element holds its router's focus; Unfocused
// when its tree has no router.
func (b *Base) FocusState() FocusState {
	r := b.routedBy()
	if r == nil || !b.HasFocus() {
		return Unfocused
	}
	if r.inactive {
		return FocusedInactive
	}
	return FocusedActive
}

// Theme returns the theme the window's elements draw with.
func (w *Window) Theme() *theme.Theme { return tokens }

// stateChanged repaints the whole tree and sends WindowStateEvent to every
// element of it, after a key or appearance change.
func (w *Window) stateChanged() {
	invalidateAll(w.root)
	for _, e := range preOrder(w.root, nil) {
		if w.closed {
			return
		}
		// A handler may have moved e out of the window.
		if e.base().routedBy() == w.router {
			e.HandleEvent(WindowStateEvent{Key: w.key, Appearance: w.appearance})
		}
	}
}

// preOrder appends e and its descendants to out in pre-order, children in
// paint order, hidden ones included.
func preOrder(e Element, out []Element) []Element {
	out = append(out, e)
	for _, c := range e.base().children {
		out = preOrder(c, out)
	}
	return out
}
