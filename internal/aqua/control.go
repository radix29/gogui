package aqua

import (
	"time"

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
// "pressed focused key light" or "normal unfocused disabled inactive dark";
// " contrast" is appended with Appearance.IncreaseContrast.
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
		out += " dark"
	} else {
		out += " light"
	}
	if s.Appearance.IncreaseContrast {
		out += " contrast"
	}
	return out
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
		t.Begin(e)
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
		t.End(e)
		return true, pe.Over
	case PointerCancel:
		t.Cancel(e)
	}
	return false, false
}

// Begin starts a press over the control: down and drawn pressed. Controls
// with their own press rules (the slider's drag, the scroller's knob and
// track) call it instead of Track for their PointerDown.
func (t *pressTracker) Begin(e Element) {
	t.down = true
	t.setPressed(e, true)
}

// End ends the press on its release; the caller decides whether it acts.
func (t *pressTracker) End(e Element) {
	t.down = false
	t.setPressed(e, false)
}

// Cancel drops a press in progress without acting (a PointerCancel, the
// control disabled during the press). It does nothing when idle.
func (t *pressTracker) Cancel(e Element) {
	t.down = false
	t.setPressed(e, false)
}

func (t *pressTracker) setPressed(e Element, p bool) {
	if p != t.pressed {
		t.pressed = p
		e.base().Invalidate()
	}
}

// clickable is a push-like control whose click is its primary action: what
// a release over it, Activate (Space, the default and cancel keys) and
// the accessibility press do.
type clickable interface {
	Element
	click()
}

// clickControl is the click core of the push-like controls (review R2),
// embedded by the push button, the checkbox, the radio button and the
// traffic lights: the press, the focus ring and one HandleEvent and
// Activate for all of them. A disabled control (EnabledInTree) drops a
// press, handles nothing and refuses Activate.
type clickControl struct {
	press pressTracker
	ring  FocusRing

	// The key flash (flash): the timer that ends it, or nil.
	flashEnd func()
}

// handleClick is the HandleEvent of e: it tracks the focus ring, then
// (enabled only) offers a key event to keys (nil: none) and feeds the
// press; a release over e clicks it.
func (c *clickControl) handleClick(e clickable, ev Event, keys func(KeyEvent) bool) bool {
	c.ring.Track(e, ev)
	if !e.base().EnabledInTree() {
		c.press.Cancel(e)
		return false
	}
	if k, ok := ev.(KeyEvent); ok && keys != nil && keys(k) {
		return true
	}
	handled, act := c.press.Track(e, ev)
	if act {
		e.click()
	}
	return handled
}

// activate is the Activate of e: it clicks e, refused while disabled.
func (c *clickControl) activate(e clickable) bool {
	if !e.base().EnabledInTree() {
		return false
	}
	e.click()
	return true
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
	return activateByKey(f)
}

// keyFlash is how long a press by key shows its control pressed, as
// AppKit's performClick: highlights it (S5.7b, review R2; provisional, as
// menuFlash, from memory of macOS).
const keyFlash = 100 * time.Millisecond

// flash draws e pressed for keyFlash, as a click shows it, for a press by
// key (Return, Escape, Space; the alert's keys). A running flash restarts.
// It changes nothing else: the caller performs the action. d is the
// driver whose shell runs the timer.
func (c *clickControl) flash(e Element, d *Driver) {
	if c.flashEnd != nil {
		c.flashEnd()
	}
	e.base().Invalidate()
	c.flashEnd = d.sh.After(keyFlash, func() {
		c.flashEnd = nil
		e.base().Invalidate()
	})
}

// Flashing reports whether a key flash runs.
func (c *clickControl) Flashing() bool { return c.flashEnd != nil }

// state returns e's control state: its press, and pressed during a key
// flash unless disabled.
func (c *clickControl) state(e Element) ControlState {
	s := c.press.State(e)
	if c.flashEnd != nil && !s.Disabled {
		s.Interaction = InteractionPressed
	}
	return s
}

// keyFlasher is an element that shows a press by key (keyFlash): the
// push button, the checkbox and the radio button.
type keyFlasher interface {
	Element
	keyFlash()
}

// activateByKey activates e for a key press (the default and cancel keys,
// Space on the focused control): a keyFlasher that is enabled flashes
// first, then e's Activate runs.
func activateByKey(e Element) bool {
	if f, ok := e.(keyFlasher); ok && e.base().EnabledInTree() {
		f.keyFlash()
	}
	return e.Activate()
}
