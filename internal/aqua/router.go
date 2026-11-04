package aqua

import (
	"slices"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// Router routes the shell's input events of one window into its element
// tree and owns the window's pointer and focus state: the hover chain, the
// pointer capture and the focused element. The aqua window (S2.2c) passes
// every shell.SurfaceHandler.Event to Router.Event.
//
// Routing rules:
//
//   - Hover: the hover chain is the topmost element under the pointer
//     (HitTest) and its ancestors, without disabled ones (disabled is
//     inherited, so the chain is the enabled part of the path from the
//     root). When it changes, the elements that left get PointerLeave
//     (deepest first) and those that joined get PointerEnter (root side
//     first). Elements that stay get nothing. The chain is recomputed on
//     every shell pointer event outside a capture and by RefreshHover.
//   - Press and capture: PointerDown goes to the deepest element of the
//     hover chain and bubbles up until one handles it; that element
//     captures the pointer. Until the last button is released every move,
//     press and release goes to it alone (dragging off keeps the target,
//     PLAN.md §5.5), with PointerEvent.Over telling whether the pointer is
//     over it, and the hover chain stays frozen. After the release the hover
//     chain is recomputed. If nothing handles the press there is no capture.
//   - Moves outside a capture go to the deepest element of the hover chain
//     and bubble up until handled.
//   - Scroll events go to the topmost enabled element under the pointer and
//     bubble up through the enabled ancestors until handled.
//   - Scroll latching (PLAN.md §5.6): the element that handles a precise
//     gesture's PhaseBegan is latched (Latched). Until the gesture is over,
//     its continuation goes to that element alone, whatever is under the
//     pointer, and its result is returned without bubbling: the events with
//     Phase PhaseChanged or PhaseEnded, and after them the OS momentum (any
//     event with a Momentum phase). PhaseEnded keeps the latch for that
//     momentum; Momentum PhaseEnded ends it. Any other scroll event ends the
//     latch and is routed by the pointer as usual: a new PhaseBegan (which
//     may latch anew), a wheel event (both phases PhaseNone), or a Phase
//     PhaseChanged or PhaseEnded after the gesture's PhaseEnded (a stray
//     event, not part of the latched gesture). An unhandled PhaseBegan
//     latches nothing, and the rest of its gesture is routed by the pointer.
//   - Key and text events go to the focused element, or to the root when
//     nothing is focused, and bubble up to the root until handled. An
//     unhandled Tab key-down (no modifiers but Shift) moves the focus with
//     FocusNext, and an unhandled Space key-down (no modifiers) presses a
//     focused FocusControl element with Activate (control.go). The result
//     is returned to the shell, so a handled key
//     suppresses its text and the OS default handling.
//   - Tab order: FocusOrder, the focusable, visible and enabled elements in
//     tree order. With full keyboard access (the default, as macOS with
//     "Keyboard navigation" on and the Windows/Linux convention) Tab reaches
//     every one of them; without it (SetFullKeyboardAccess(false), the macOS
//     default) only those of FocusKind FocusText. SetFocus (a click, Focus)
//     can focus any focusable element in both modes.
//   - Disabled elements get no input at all.
//   - Focus hook (scroll-to-focused, PLAN.md §5.6): whenever the focus
//     moves to an element (SetFocus, Focus, FocusNext), after its
//     FocusEvent{true}, every enclosing ScrollView, innermost first,
//     brings it into view (ScrollView). Clearing the focus, SetActive and
//     a focus that is lost on the way scroll nothing.
//   - The focus survives the window becoming inactive (SetActive(false)):
//     Focused keeps the element, and FocusState reports it as
//     FocusedInactive until the window is key again.
//
// The state follows the tree: when the focused, a hovered or the capturing
// element is removed from the tree, hidden or disabled (itself or an
// ancestor), the capturing element gets PointerCancel, the hovered ones
// PointerLeave and the focused one FocusEvent{false}, at once. A latched
// element just loses the latch (it gets no event), and the rest of its
// gesture, momentum included, is dropped (Event returns false) instead of
// being re-routed to whatever is under the pointer, until an event that is
// not part of the gesture ends it as above. Elements that
// move, appear or stop being focusable change the hover only at the next
// pointer event or RefreshHover; a focused element made unfocusable loses
// the focus at once.
//
// A Router is used on the UI thread only.
type Router struct {
	root *Base

	hover    []Element // root side first
	capture  Element
	focus    Element
	inactive bool // the window is not key (SetActive)
	limited  bool // no full keyboard access (SetFullKeyboardAccess)

	// Scroll latching: latch is the latched element; latchLost marks a
	// gesture whose latched element was dropped (its rest is discarded);
	// latchEnded records that the gesture's PhaseEnded came, so only
	// momentum continues it.
	latch      Element
	latchLost  bool
	latchEnded bool

	pos    driver.Point // last pointer position, in the root's parent coordinates
	inside bool         // between shell PointerEnter and PointerLeave
	last   shell.PointerEvent
}

// NewRouter returns the router of the tree rooted at root. It panics when
// root has a parent, already has a router, or is not initialised. A routed
// root cannot be added to another tree.
func NewRouter(root Element) *Router {
	b := root.base()
	b.mustSelf()
	switch {
	case b.parent != nil:
		panic("aqua: NewRouter on an element with a parent")
	case b.router != nil:
		panic("aqua: NewRouter on an element that already has a router")
	}
	r := &Router{root: b}
	b.router = r
	return r
}

// Root returns the root element.
func (r *Router) Root() Element { return r.root.self }

// Hovered returns the deepest element of the hover chain, or nil.
func (r *Router) Hovered() Element {
	if len(r.hover) == 0 {
		return nil
	}
	return r.hover[len(r.hover)-1]
}

// HoverChain returns a copy of the hover chain, root side first.
func (r *Router) HoverChain() []Element { return slices.Clone(r.hover) }

// Captured returns the element that holds the pointer capture, or nil.
func (r *Router) Captured() Element { return r.capture }

// Latched returns the element that holds the scroll latch (see the type's
// rules), or nil.
func (r *Router) Latched() Element { return r.latch }

// Focused returns the focused element, or nil. It is kept while the
// window is inactive.
func (r *Router) Focused() Element { return r.focus }

// Active reports whether the router's window is key. A router made with
// NewRouter starts active; the aqua window keeps it equal to IsKey.
func (r *Router) Active() bool { return !r.inactive }

// SetActive records whether the router's window is key. It changes only
// what FocusState reports: the focus and the hover are kept, and no event
// is sent (the window sends WindowStateEvent).
func (r *Router) SetActive(active bool) { r.inactive = !active }

// FullKeyboardAccess reports whether Tab reaches every focusable element
// (true, the default) or only those of FocusKind FocusText.
func (r *Router) FullKeyboardAccess() bool { return !r.limited }

// SetFullKeyboardAccess sets whether Tab reaches every focusable element
// (true) or only text elements (false, the macOS default with "Keyboard
// navigation" off). The current focus is kept either way.
func (r *Router) SetFullKeyboardAccess(on bool) { r.limited = !on }

// FocusState reports how e holds the focus: Unfocused, FocusedInactive
// (the focused element while the window is not key) or FocusedActive.
func (r *Router) FocusState(e Element) FocusState {
	if e == nil || e.base().routedBy() != r {
		return Unfocused
	}
	return e.base().FocusState()
}

// Event routes one shell event and reports whether an element handled it
// (see the type's rules). It is meant for shell.SurfaceHandler.Event.
func (r *Router) Event(ev shell.Event) bool {
	switch ev := ev.(type) {
	case shell.PointerEvent:
		return r.pointer(ev)
	case shell.ScrollEvent:
		return r.scroll(ev)
	case shell.KeyEvent:
		return r.key(ev)
	case shell.TextEvent:
		return r.bubble(r.keyTarget(), TextEvent{ev})
	}
	return false
}

func (r *Router) pointer(ev shell.PointerEvent) bool {
	r.last = ev
	r.pos = ev.Pos
	switch ev.Kind {
	case shell.PointerEnter:
		r.inside = true
		if r.capture == nil {
			r.setHover(r.hoverAt(ev.Pos))
		}
		return false
	case shell.PointerLeave:
		r.inside = false
		r.cancelCapture()
		r.setHover(nil)
		return false
	case shell.PointerMove:
		if r.capture != nil {
			return r.sendCaptured(PointerMove, ev)
		}
		r.inside = true
		r.setHover(r.hoverAt(ev.Pos))
		return r.bubblePointer(PointerMove, ev)
	case shell.PointerDown:
		if r.capture != nil {
			return r.sendCaptured(PointerDown, ev)
		}
		r.inside = true
		r.setHover(r.hoverAt(ev.Pos))
		for e := r.Hovered(); e != nil; e = parentOf(e) {
			if e.HandleEvent(r.pointerEvent(PointerDown, e, ev, true)) {
				if r.usable(e) { // a handler may have removed or disabled it
					r.capture = e
				}
				return true
			}
		}
		return false
	case shell.PointerUp:
		if r.capture == nil {
			if ev.Buttons == 0 {
				r.setHover(r.hoverAt(ev.Pos))
			}
			return false
		}
		handled := r.sendCaptured(PointerUp, ev)
		if ev.Buttons == 0 {
			r.capture = nil
			r.setHover(r.hoverAt(ev.Pos))
		}
		return handled
	}
	return false
}

// sendCaptured sends ev to the capturing element.
func (r *Router) sendCaptured(kind PointerKind, ev shell.PointerEvent) bool {
	c := r.capture
	return c.HandleEvent(r.pointerEvent(kind, c, ev, r.over(c, ev.Pos)))
}

// bubblePointer sends a move to the deepest hovered element and up until
// one handles it.
func (r *Router) bubblePointer(kind PointerKind, ev shell.PointerEvent) bool {
	for e := r.Hovered(); e != nil; e = parentOf(e) {
		if e.HandleEvent(r.pointerEvent(kind, e, ev, true)) {
			return true
		}
	}
	return false
}

func (r *Router) pointerEvent(kind PointerKind, e Element, ev shell.PointerEvent, over bool) PointerEvent {
	return PointerEvent{
		Kind: kind, Pos: FromRoot(e, ev.Pos), Over: over,
		Button: ev.Button, Buttons: ev.Buttons, Clicks: ev.Clicks, Mods: ev.Mods, Time: ev.Time,
	}
}

// over reports whether the topmost element at p is e or a descendant of e.
func (r *Router) over(e Element, p driver.Point) bool {
	hit, _ := HitTest(r.root.self, p)
	for ; hit != nil; hit = parentOf(hit) {
		if hit.base() == e.base() {
			return true
		}
	}
	return false
}

// hoverAt returns the hover chain for the point p: the enabled part of the
// path from the root to the topmost element under p.
func (r *Router) hoverAt(p driver.Point) []Element {
	hit, _ := HitTest(r.root.self, p)
	var path []Element
	for e := hit; e != nil; e = parentOf(e) {
		path = append(path, e)
	}
	slices.Reverse(path)
	for i, e := range path {
		if e.base().disabled {
			return path[:i]
		}
	}
	return path
}

// setHover replaces the hover chain, sending PointerLeave to the elements
// that left it (deepest first) and PointerEnter to those that joined (root
// side first).
func (r *Router) setHover(chain []Element) {
	common := 0
	for common < len(chain) && common < len(r.hover) && chain[common] == r.hover[common] {
		common++
	}
	left := r.hover[common:]
	r.hover = slices.Clone(chain[:common])
	ev := r.last
	for i := len(left) - 1; i >= 0; i-- {
		e := left[i]
		e.HandleEvent(PointerEvent{Kind: PointerLeave, Pos: FromRoot(e, r.pos), Buttons: ev.Buttons, Mods: ev.Mods, Time: ev.Time})
	}
	for i, e := range chain[common:] {
		// An element joins the chain only as it gets its Enter, so the
		// chain never holds an element that was not told. Stop when a
		// Leave or Enter handler changed the chain or made e unusable.
		if len(r.hover) != common+i || !r.usable(e) {
			return
		}
		r.hover = append(r.hover, e)
		e.HandleEvent(PointerEvent{Kind: PointerEnter, Pos: FromRoot(e, r.pos), Over: true, Buttons: ev.Buttons, Mods: ev.Mods, Time: ev.Time})
	}
}

// RefreshHover recomputes the hover chain at the last pointer position,
// for after a layout or a tree change moved elements under a still pointer.
// It does nothing during a capture or while the pointer is outside.
func (r *Router) RefreshHover() {
	if r.capture != nil || !r.inside {
		return
	}
	r.setHover(r.hoverAt(r.pos))
}

// cancelCapture ends the capture with PointerCancel.
func (r *Router) cancelCapture() {
	c := r.capture
	if c == nil {
		return
	}
	r.capture = nil
	ev := r.last
	c.HandleEvent(PointerEvent{Kind: PointerCancel, Pos: FromRoot(c, r.pos), Buttons: ev.Buttons, Mods: ev.Mods, Time: ev.Time})
}

func (r *Router) scroll(ev shell.ScrollEvent) bool {
	if r.latch != nil || r.latchLost {
		if r.continuesLatch(ev) {
			target := r.latch
			if ev.Phase == shell.PhaseEnded {
				r.latchEnded = true
			}
			if ev.Momentum == shell.PhaseEnded {
				r.endLatch()
			}
			if target == nil {
				return false // the latched element was dropped
			}
			return target.HandleEvent(scrollEventFor(target, ev))
		}
		r.endLatch()
	}
	hit, _ := HitTest(r.root.self, ev.Pos)
	for e := hit; e != nil; e = parentOf(e) {
		if !e.base().EnabledInTree() {
			continue
		}
		if e.HandleEvent(scrollEventFor(e, ev)) {
			if ev.Phase == shell.PhaseBegan {
				if r.usable(e) {
					r.latch = e
				} else {
					r.latchLost = true // its handler removed, hid or disabled it
				}
			}
			return true
		}
	}
	return false
}

// continuesLatch reports whether ev continues the latched gesture: a Phase
// PhaseChanged or PhaseEnded before the gesture's PhaseEnded, or any OS
// momentum event.
func (r *Router) continuesLatch(ev shell.ScrollEvent) bool {
	if ev.Momentum != shell.PhaseNone {
		return true
	}
	return (ev.Phase == shell.PhaseChanged || ev.Phase == shell.PhaseEnded) && !r.latchEnded
}

// endLatch ends the scroll latch and its gesture.
func (r *Router) endLatch() {
	r.latch, r.latchLost, r.latchEnded = nil, false, false
}

// scrollEventFor returns ev as routed to e, with Pos in e's coordinates.
func scrollEventFor(e Element, ev shell.ScrollEvent) ScrollEvent {
	ev.Pos = FromRoot(e, ev.Pos)
	return ScrollEvent{ev}
}

func (r *Router) key(ev shell.KeyEvent) bool {
	if r.bubble(r.keyTarget(), KeyEvent{ev}) {
		return true
	}
	if ev.Down && ev.Key == driver.KeyTab && ev.Mods&^driver.ModShift == 0 {
		return r.FocusNext(ev.Mods&driver.ModShift != 0)
	}
	return r.spaceActivates(ev)
}

// keyTarget returns the focused element, or the root.
func (r *Router) keyTarget() Element {
	if r.focus != nil {
		return r.focus
	}
	return r.root.self
}

// bubble sends ev to e and its ancestors until one handles it, skipping
// disabled elements.
func (r *Router) bubble(e Element, ev Event) bool {
	for ; e != nil; e = parentOf(e) {
		if e.base().EnabledInTree() && e.HandleEvent(ev) {
			return true
		}
	}
	return false
}

// usable reports whether e is in the router's tree, visible and enabled.
func (r *Router) usable(e Element) bool {
	b := e.base()
	return b.routedBy() == r && b.VisibleInTree() && b.EnabledInTree()
}

// focusable reports whether e can take the focus now.
func (r *Router) focusable(e Element) bool { return e.base().focusable && r.usable(e) }

// SetFocus moves the focus to e, or clears it for nil, and reports whether
// e is focused afterwards (true for nil). It fails, without changing the
// focus, when e is not in the router's tree, not focusable, hidden or
// disabled. The old element gets FocusEvent{false}, then the new one
// FocusEvent{true}; focusing the focused element again does nothing.
func (r *Router) SetFocus(e Element) bool {
	if e == nil {
		r.setFocus(nil)
		return true
	}
	if !r.focusable(e) {
		return false
	}
	r.setFocus(e)
	return r.focus == e
}

func (r *Router) setFocus(e Element) {
	old := r.focus
	if old == e {
		return
	}
	r.focus = e
	if old != nil {
		old.HandleEvent(FocusEvent{Focused: false})
	}
	if e != nil && r.focus == e {
		e.HandleEvent(FocusEvent{Focused: true})
	}
	if e != nil {
		r.revealFocus(e)
	}
}

// focusRevealer is an element that brings a newly focused descendant into
// view: the ScrollView's element (scroll-to-focused, PLAN.md S3.5g).
type focusRevealer interface {
	revealFocus(f Element)
}

// revealFocus is the focus-changed hook: after the focus moved to f (and f
// got its FocusEvent), each ancestor of f that is a focusRevealer, the
// innermost first, brings f into view. It stops when f loses the focus
// or leaves the tree on the way.
func (r *Router) revealFocus(f Element) {
	for e := parentOf(f); e != nil; e = parentOf(e) {
		if r.focus != f || !r.usable(f) {
			return
		}
		if fr, ok := e.(focusRevealer); ok {
			fr.revealFocus(f)
		}
	}
}

// tabStop reports whether b is in the Tab order of the current
// keyboard-access mode (its visibility and state aside).
func (r *Router) tabStop(b *Base) bool {
	return b.focusable && (!r.limited || b.focusKind == FocusText)
}

// FocusOrder returns the Tab order: the elements that can take the focus
// now (focusable, visible and enabled) and that Tab reaches in the current
// keyboard-access mode (FullKeyboardAccess), in tree order (pre-order,
// children in paint order).
func (r *Router) FocusOrder() []Element {
	order, _, _ := r.tabOrder()
	return order
}

// tabOrder returns FocusOrder and where the focus is in it: at is its index
// when exact (the focus is a Tab stop), otherwise the index of the first
// Tab stop after it in tree order (len(order) for none); -1 without a focus.
func (r *Router) tabOrder() (order []Element, at int, exact bool) {
	at = -1
	var walk func(b *Base)
	walk = func(b *Base) {
		if b.hidden || b.disabled {
			return
		}
		if r.focus != nil && r.focus.base() == b {
			at, exact = len(order), r.tabStop(b)
		}
		if r.tabStop(b) {
			order = append(order, b.self)
		}
		for _, c := range b.children {
			walk(c.base())
		}
	}
	walk(r.root)
	return order, at, exact
}

// FocusNext moves the focus to the next element of FocusOrder (the previous
// one when backward), wrapping around; without a focus it goes to the first
// (last) one. A focus outside the Tab order (a control clicked without full
// keyboard access) moves to the first Tab stop after it in tree order (the
// last one before it when backward). It reports whether there was an
// element to focus.
func (r *Router) FocusNext(backward bool) bool {
	order, at, exact := r.tabOrder()
	n := len(order)
	if n == 0 {
		return false
	}
	var i int
	switch {
	case at < 0 && backward:
		i = n - 1
	case at < 0:
		i = 0
	case backward:
		i = (at + n - 1) % n
	case exact:
		i = (at + 1) % n
	default:
		i = at % n
	}
	r.setFocus(order[i])
	return true
}

// validate drops the state of elements that left the tree, were hidden or
// disabled, or (for the focus) stopped being focusable. Called after every
// such change in the router's tree.
func (r *Router) validate() {
	if r.capture != nil && !r.usable(r.capture) {
		r.cancelCapture()
	}
	if r.latch != nil && !r.usable(r.latch) {
		r.latch, r.latchLost = nil, true
	}
	for i, e := range r.hover {
		if !r.usable(e) {
			r.setHover(slices.Clone(r.hover[:i]))
			break
		}
	}
	if r.focus != nil && !r.focusable(r.focus) {
		r.setFocus(nil)
	}
}

// routedBy returns the router of b's tree, or nil.
func (b *Base) routedBy() *Router {
	e := b
	for e.parent != nil {
		e = e.parent
	}
	return e.router
}

// validateTree is validate for a possibly nil router.
func (r *Router) validateTree() {
	if r != nil {
		r.validate()
	}
}

// treeChanged lets the tree's router drop state that is no longer valid.
func (b *Base) treeChanged() {
	b.routedBy().validateTree()
}

// Focusable reports whether the element can take the keyboard focus.
func (b *Base) Focusable() bool { return b.focusable }

// SetFocusable sets whether the element can take the keyboard focus. A
// focused element made unfocusable loses the focus.
func (b *Base) SetFocusable(focusable bool) {
	if focusable == b.focusable {
		return
	}
	b.focusable = focusable
	if !focusable {
		b.treeChanged()
	}
}

// FocusKind says in which keyboard-access modes Tab reaches a focusable
// element (Router.SetFullKeyboardAccess).
type FocusKind uint8

const (
	// FocusControl (the zero value): Tab reaches the element only with full
	// keyboard access. Buttons, checkboxes, sliders, pop-ups.
	FocusControl FocusKind = iota
	// FocusText: Tab always reaches the element. Text fields, text views,
	// later lists.
	FocusText
)

// String returns "control" or "text".
func (k FocusKind) String() string {
	switch k {
	case FocusControl:
		return "control"
	case FocusText:
		return "text"
	}
	return "focuskind(?)"
}

// FocusKind returns the element's focus kind (FocusControl by default).
func (b *Base) FocusKind() FocusKind { return b.focusKind }

// SetFocusKind sets in which keyboard-access modes Tab reaches the element.
// It changes only the Tab order; a focused element keeps the focus.
func (b *Base) SetFocusKind(k FocusKind) { b.focusKind = k }

// Focus focuses the element (Router.SetFocus) and reports whether it is
// focused afterwards. It returns false when the element's tree has no
// router.
func (b *Base) Focus() bool {
	r := b.routedBy()
	return r != nil && r.SetFocus(b.mustSelf())
}

// HasFocus reports whether the element is its router's focused element.
func (b *Base) HasFocus() bool {
	r := b.routedBy()
	return r != nil && r.focus != nil && r.focus.base() == b
}

// parentOf returns e's parent element, or nil.
func parentOf(e Element) Element { return e.base().Parent() }
