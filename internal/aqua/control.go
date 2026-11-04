package aqua

import (
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// Control state and press tracking (PLAN.md S3.3a). A drawn control keeps
// a pressTracker for its clicks and paints from ControlState, which it
// computes on every Paint from the tracker, its router and its window;
// nothing of it is cached, so it cannot go stale.

// Interaction is the pointer part of a ControlState.
type Interaction uint8

const (
	// InteractionNormal: the pointer is not over the control, or a press
	// of it was dragged off.
	InteractionNormal Interaction = iota
	// InteractionHover: the control is in the hover chain and not pressed.
	InteractionHover
	// InteractionPressed: the control holds a primary press and the
	// pointer is over it.
	InteractionPressed
)

var interactionNames = [...]string{"normal", "hover", "pressed"}

// String returns "normal", "hover" or "pressed".
func (i Interaction) String() string {
	if int(i) < len(interactionNames) {
		return interactionNames[i]
	}
	return "interaction(?)"
}

// ControlState is everything a control's look depends on besides its own
// value (title, check state …).
type ControlState struct {
	Interaction Interaction
	// Focus is how the control holds the keyboard focus (Base.FocusState).
	Focus FocusState
	// Disabled: the control or an ancestor is disabled (EnabledInTree).
	// A disabled control is never hovered or pressed.
	Disabled bool
	// Key: the window is key (true outside a window when the tree's router
	// is active, or has no router). Controls draw the inactive look
	// (grey accent) when false.
	Key bool
	// Appearance is the window's effective appearance; outside a window
	// the zero Appearance (light, no accent).
	Appearance driver.Appearance
}

// String returns a compact form for test logs, such as
// "pressed focused key light" or "normal unfocused disabled inactive dark".
func (s ControlState) String() string {
	out := s.Interaction.String() + " " + s.Focus.String()
	if s.Disabled {
		out += " disabled"
	}
	if s.Key {
		out += " key"
	} else {
		out += " inactive"
	}
	if s.Appearance.Dark {
		return out + " dark"
	}
	return out + " light"
}

// controlState computes the element's control state. t is the element's
// press tracker, or nil for an element that cannot be pressed.
func (b *Base) controlState(t *pressTracker) ControlState {
	s := ControlState{Focus: b.FocusState(), Disabled: !b.EnabledInTree(), Key: true}
	r := b.routedBy()
	if w := b.Window(); w != nil {
		s.Key, s.Appearance = w.key, w.appearance
	} else if r != nil {
		s.Key = r.Active()
	}
	switch {
	case s.Disabled:
	case t != nil && t.down:
		// The hover chain is frozen during the capture, so it says
		// nothing about a press dragged off.
		if t.pressed {
			s.Interaction = InteractionPressed
		}
	case r != nil && r.inHover(b):
		s.Interaction = InteractionHover
	}
	return s
}

// ControlState returns the element's control state without a press: the
// hover, focus, disabled, key and appearance parts. Controls that can be
// pressed use their pressTracker's State instead.
func (b *Base) ControlState() ControlState { return b.controlState(nil) }

// inHover reports whether b is in the hover chain.
func (r *Router) inHover(b *Base) bool {
	for _, e := range r.hover {
		if e.base() == b {
			return true
		}
	}
	return false
}

// pressTracker follows the primary-button press of a push-like control
// (PLAN.md §5.5): a primary press captures the pointer and presses the
// control, dragging off un-presses it and dragging back presses it again,
// a release over the control acts, a release outside or a PointerCancel
// does nothing. Other buttons are ignored, but swallowed during a press.
// The zero value is idle. The element is invalidated whenever the pressed
// look changes.
type pressTracker struct {
	down    bool // a primary press is captured
	pressed bool // down and the pointer over the control
}

// Pressed reports whether the control is drawn pressed.
func (t *pressTracker) Pressed() bool { return t.pressed }

// Down reports whether a press is in progress (over the control or not).
func (t *pressTracker) Down() bool { return t.down }

// State returns e's ControlState with this tracker's press.
func (t *pressTracker) State(e Element) ControlState { return e.base().controlState(t) }

// Track feeds one event of e to the tracker. handled is what e's
// HandleEvent should return for it (true for the events of a press, so
// the router captures and keeps the pointer); act reports a release over
// the control, on which e performs its action. Events other than pointer
// events are not handled.
func (t *pressTracker) Track(e Element, ev Event) (handled, act bool) {
	pe, ok := ev.(PointerEvent)
	if !ok {
		return false, false
	}
	switch pe.Kind {
	case PointerDown:
		if pe.Button != shell.ButtonPrimary || t.down {
			return t.down, false
		}
		t.down = true
		t.setPressed(e, true)
		return true, false
	case PointerMove:
		if t.down {
			t.setPressed(e, pe.Over)
		}
		return t.down, false
	case PointerUp:
		if !t.down || pe.Button != shell.ButtonPrimary {
			return t.down, false
		}
		t.down = false
		t.setPressed(e, false)
		return true, pe.Over
	case PointerCancel:
		t.Reset(e)
	}
	return false, false
}

// Reset drops a press in progress without acting.
func (t *pressTracker) Reset(e Element) {
	t.down = false
	t.setPressed(e, false)
}

func (t *pressTracker) setPressed(e Element, p bool) {
	if p != t.pressed {
		t.pressed = p
		e.base().Invalidate()
	}
}

// spaceActivates handles an unhandled key-down of Space on the focused
// element (PLAN.md §5.5): without modifiers, on an element of FocusKind
// FocusControl that holds the focus in the key window, it calls Activate.
// An auto-repeat activates nothing but is consumed. Text elements keep
// Space (it reaches them first and is not taken here). It reports whether
// the key was consumed.
func (r *Router) spaceActivates(ev shell.KeyEvent) bool {
	f := r.focus
	if !ev.Down || ev.Key != driver.KeySpace || ev.Mods != 0 || f == nil || r.inactive ||
		f.base().focusKind != FocusControl || !r.usable(f) {
		return false
	}
	if ev.Repeat {
		return true
	}
	return f.Activate()
}
