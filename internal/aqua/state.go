package aqua

import (
	"slices"

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
// NewRouter directly). An element in a popup of a window (popupRoot,
// S5.5g) or in a sheet of it (alertRoot, S5.7c) is in that window's
// tree. Through it an element reads the window context: IsKey,
// Appearance, Theme, Timeline and Router. A closed window keeps answering
// with its last state.
func (b *Base) Window() *Window {
	if rc := b.rootContext(); rc != nil {
		return rc.rootWindow()
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
// element of it, after a key or appearance change; then the same for the
// tree of each open popup, in opening order.
func (w *Window) stateChanged() {
	broadcastState(w.root, w.router, w.state, w.isClosed)
	for _, p := range slices.Clone(w.popups) {
		if w.closed {
			return
		}
		p.stateChanged()
	}
	for _, s := range slices.Clone(w.sheets) {
		if w.closed {
			return
		}
		s.stateChanged()
	}
}

// state returns the WindowStateEvent of the window's trees now.
func (w *Window) state() WindowStateEvent {
	return WindowStateEvent{Key: w.key, Appearance: w.appearance}
}

func (w *Window) isClosed() bool { return w.closed }

// broadcastState repaints root's tree and sends each of its elements, in
// pre-order, the WindowStateEvent state returns at that moment (review
// R3; current if a handler changed the state again). It skips an element
// a handler moved out of the tree (no longer routed by router) and stops
// once gone reports the tree closed.
func broadcastState(root Element, router *Router, state func() WindowStateEvent, gone func() bool) {
	if gone() {
		return
	}
	invalidateAll(root)
	for _, e := range preOrder(root, nil) {
		if gone() {
			return
		}
		if e.base().routedBy() == router {
			e.HandleEvent(state())
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
