package gogui

import (
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/layout"
)

// Widget is a control, stack or form that can be placed in a window. Only
// this package implements it.
type Widget interface {
	// base returns the state every widget shares.
	base() *widget
	// node returns the widget's layout node.
	node() layout.Node
	// peerWidget returns the widget's peer, or nil for widgets that have
	// none (Spacer).
	peerWidget() driver.Widget
}

// kind is the type of a widget. It names the widget in UI-thread check
// messages and selects its default expansion.
type kind uint8

const (
	kindLabel kind = iota
	kindButton
	kindCheckbox
	kindRadio // one radio button of a RadioGroup
	kindRadioGroup
	kindPopUp
	kindTextField
	kindSlider
	kindProgressBar
	kindSpinner
	kindTextView
	kindScrollView
	kindSeparator
	kindBox
	kindStack
	kindForm
	kindSpacer
	numKinds
)

var kindNames = [numKinds]string{
	"Label", "Button", "Checkbox", "Radio", "RadioGroup", "PopUpButton",
	"TextField", "Slider", "ProgressBar", "Spinner", "TextView",
	"ScrollView", "Separator", "Box", "Stack", "Form", "Spacer",
}

func (k kind) String() string {
	if k < numKinds {
		return kindNames[k]
	}
	return "Widget"
}

// layoutHost is the window a widget tree is shown in (a *Window). invalidate
// marks it for a deferred, coalesced layout pass.
type layoutHost interface {
	invalidate()
}

// widget is the state shared by every widget.
type widget struct {
	kind   kind
	peer   driver.Widget // nil for widgets without a peer
	n      layout.Node
	parent Widget     // the widget this one is placed in, or nil
	host   layoutHost // set on the root widget of a window
	dead   bool       // destroyed: setters only record, getters return the last value

	disabled bool // disabled controls fire no events (S0.5g)

	// Axis flags that change the default expansion: vertical for sliders
	// and separators, scrolls (horizontal, vertical) for scroll views.
	vertical bool
	scrolls  [2]bool
}

func (w *widget) base() *widget             { return w }
func (w *widget) node() layout.Node         { return w.n }
func (w *widget) peerWidget() driver.Widget { return w.peer }

// live reports whether the peer may be called.
func (w *widget) live() bool { return w.peer != nil && !w.dead }

// check runs the UI-thread check for the public method kind.method. The
// name is built only when the check fails.
func (w *widget) check(method string) {
	if q := queue(); q != nil && q.Checks() && !q.OnUIThread() {
		q.Check(w.kind.String() + "." + method)
	}
}

// window returns the window the widget's tree is shown in, or nil.
func (w *widget) window() layoutHost {
	for b := w; b != nil; {
		if b.host != nil {
			return b.host
		}
		if b.parent == nil {
			return nil
		}
		b = b.parent.base()
	}
	return nil
}

// invalidate marks the widget's window for a layout pass, after a change
// that can change the widget's size. Outside a window it does nothing.
func (w *widget) invalidate() {
	if w.dead {
		return
	}
	if h := w.window(); h != nil {
		h.invalidate()
	}
}

// destroy destroys the widget's own peer. It does not touch children; the
// window destroys its tree bottom-up (destroyTree, window.go).
func (w *widget) destroy() {
	if w.dead {
		return
	}
	w.dead = true
	if a := cur.Load(); a != nil {
		delete(a.live, w)
	}
	if w.peer != nil {
		w.peer.Destroy()
	}
}

// defaultExpand is the default expansion of each kind of widget (moved
// from the layout tests, PLAN.md §11.1 S0.4). Text fields and progress
// bars expand horizontally, a text view on both axes, sliders and
// separators along their length, and a scroll view on each axis that has a
// scroller. Labels, buttons, checkboxes, radios, radio groups, pop-up
// buttons and spinners don't expand. Boxes, stacks and forms expand only
// through their children (layout propagation), and spacers through the
// stack.
func (w *widget) defaultExpand() (h, v bool) {
	switch w.kind {
	case kindTextField, kindProgressBar:
		return true, false
	case kindTextView:
		return true, true
	case kindSlider, kindSeparator:
		return !w.vertical, w.vertical
	case kindScrollView:
		return w.scrolls[0], w.scrolls[1]
	}
	return false, false
}

// applyExpand writes the default expansion into the layout node.
func (w *widget) applyExpand() {
	h, v := w.defaultExpand()
	w.n.Props().SetExpand(layout.Horizontal, h).SetExpand(layout.Vertical, v)
}

// driverFor returns the running app's driver for constructor what (for
// example "NewLabel"). It panics before Run's ready callback, after Run
// has returned, and (with checks on) off the UI thread.
func driverFor(what string) driver.Driver {
	a := cur.Load()
	if a == nil {
		panic("gogui: " + what + " called outside Run; create widgets in the ready callback or on the UI thread")
	}
	checkUI(what)
	return a.drv
}

// common holds the setters every widget shares. A control embeds
// common[*Control], so the setters return the concrete type for chaining.
type common[T any] struct {
	widget
	self T
}

// init sets up the shared state. A nil n means a leaf around peer.
func (c *common[T]) init(self T, k kind, peer driver.Widget, n layout.Node) {
	if n == nil {
		n = layout.NewLeaf(peer)
	}
	c.self, c.kind, c.peer, c.n = self, k, peer, n
	c.applyExpand()
	if a := cur.Load(); a != nil && a.live != nil {
		a.live[&c.widget] = any(self).(Widget)
	}
}

// SetEnabled enables or disables the widget. A disabled control is dimmed
// and does not react to input or fire events.
func (c *common[T]) SetEnabled(enabled bool) T {
	c.check("SetEnabled")
	c.disabled = !enabled
	if c.live() {
		c.peer.SetEnabled(enabled)
	}
	return c.self
}

// SetHidden hides or shows the widget. A hidden widget takes no space in
// its stack or form.
func (c *common[T]) SetHidden(hidden bool) T {
	c.check("SetHidden")
	c.n.Props().SetHidden(hidden)
	if c.live() {
		c.peer.SetHidden(hidden)
	}
	if f, ok := c.parent.(*FormView); ok {
		f.childHidden(&c.widget, hidden)
	}
	c.invalidate()
	return c.self
}

// SetTooltip sets the text shown when the pointer rests on the widget. An
// empty string removes it.
func (c *common[T]) SetTooltip(text string) T {
	c.check("SetTooltip")
	if c.live() {
		c.peer.SetTooltip(text)
	}
	return c.self
}

// SetControlSize sets the size class of the control and its default font
// (NSControl.ControlSize). The default is ControlSizeRegular.
func (c *common[T]) SetControlSize(cs ControlSize) T {
	c.check("SetControlSize")
	if c.live() {
		c.peer.SetControlSize(cs)
	}
	c.invalidate()
	return c.self
}

// Focus makes the widget the focused control of its window. It does
// nothing for widgets that can't take focus.
func (c *common[T]) Focus() T {
	c.check("Focus")
	if c.live() {
		c.peer.Focus()
	}
	return c.self
}

// Unsubscribe removes an event handler. Calling it again does nothing, and
// it is safe to call while the event is being dispatched.
type Unsubscribe func()

// subscribers is the list of handlers of one event, called in the order
// they subscribed.
type subscribers[F any] struct {
	list []*subscriber[F]
}

type subscriber[F any] struct {
	fn   F
	gone bool
}

// add subscribes fn. Handlers added during a dispatch are first called by
// the next one.
func (s *subscribers[F]) add(fn F) Unsubscribe {
	sub := &subscriber[F]{fn: fn}
	s.list = append(s.list, sub)
	return func() {
		if sub.gone {
			return
		}
		sub.gone = true
		// Build a new slice, so a dispatch in progress keeps its snapshot.
		out := make([]*subscriber[F], 0, len(s.list)-1)
		for _, x := range s.list {
			if x != sub {
				out = append(out, x)
			}
		}
		s.list = out
	}
}

// each calls call with every handler, in order. A handler unsubscribed
// during the dispatch is not called anymore.
func (s *subscribers[F]) each(call func(F)) {
	for _, sub := range s.list {
		if !sub.gone {
			call(sub.fn)
		}
	}
}
