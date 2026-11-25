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
//     CancelPointer (a grab popup opening, S5.5g) ends the capture, the
//     hover and the latch; a release without a captured press goes
//     nowhere unless SetStrayReleases routes it like a move, or to the
//     root outside every element (a grab popup's router, for the press
//     that opened it).
//   - First click (S6.1b): a PointerDown with Activating set (the press
//     that made the window key) only activates the window, as AppKit's
//     default acceptsFirstMouse = NO: it updates the hover chain but goes
//     to no element, and its release is dropped too (no press, no release,
//     no drag, no focus move). Over an element that AcceptsFirstMouse (set
//     on it or an ancestor; the title bar's traffic lights and menu strip)
//     it is routed as usual. Hover and scroll events need no activation.
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
//   - Key, text and preedit events go to the focused element, or to the
//     root when nothing is focused, and bubble up to the root until
//     handled. An unhandled Tab key-down (no modifiers but Shift) moves the
//     focus with FocusNext, and an unhandled Space key-down (no modifiers)
//     presses a focused FocusControl element with Activate (control.go).
//     The result is returned to the shell, so a handled key suppresses its
//     text and the OS default handling. Only a focused text element that
//     takes IME input handles a preedit (S5.3c).
//   - Tab order: FocusOrder, the focusable, visible and enabled elements in
//     tree order. With full keyboard access (the default, as macOS with
//     "Keyboard navigation" on and the Windows/Linux convention) Tab reaches
//     every one of them; without it (SetFullKeyboardAccess(false), the macOS
//     default) only those of FocusKind FocusText. A group that is one Tab
//     stop (groupTabStop: the radios of a group, S5.7h) is reached at its
//     stop only, its other members by their own keys. SetFocus (a click, Focus)
//     can focus any focusable element in both modes.
//   - Disabled elements get no input at all.
//   - Cursor: the pointer shape is the Cursor of the deepest element of
//     the hover chain (CursorArrow for an empty chain). Whenever the chain
//     changes outside a capture the router compares that shape with the
//     one it set last and, on a change, calls the function of
//     SetCursorFunc (the window passes Surface.SetCursor). During a
//     capture the shape is frozen with the hover chain (dragging a
//     selection out of a text field keeps the I-beam); the release
//     recomputes both. RefreshCursor re-reads it for an element whose
//     Cursor changed under a still pointer.
//   - Focus hook (scroll-to-focused, PLAN.md §5.6): whenever the focus
//     moves to an element (SetFocus, Focus, FocusNext), after its
//     FocusEvent{true}, every enclosing ScrollView, innermost first,
//     brings it into view (ScrollView). Clearing the focus, SetActive and
//     a focus that is lost on the way scroll nothing.
//   - Focus-changed hook (PLAN.md S5.3d): after every change of Focused
//     (a move, a clear, a focus dropped with the tree), once the focus
//     events and the scroll-to-focused are done, the router calls the
//     function of SetFocusChangedFunc (the window publishes its text input
//     there).
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
	scratch  []Element // hoverAt's buffer
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

	// The pointer shape: setCursor applies it (nil: none), cursor is the
	// shape applied last.
	setCursor func(shell.Cursor)
	cursor    shell.Cursor

	focusChanged func() // SetFocusChangedFunc's, or nil
	treeChanged  func() // SetTreeChangedFunc's, or nil

	cancels  uint64 // CancelPointer calls, so a press in flight sees one
	strayUps bool   // SetStrayReleases

	// swallowed holds the buttons whose activating press was dropped (the
	// first-click rule), so their release is dropped too.
	swallowed shell.PointerButton

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
	treeChange()
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
	case shell.PreeditEvent:
		return r.bubble(r.keyTarget(), PreeditEvent{ev})
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
		r.swallowed = 0
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
		r.swallowed &^= ev.Button // a new press: an old release was lost
		if r.capture != nil {
			return r.sendCaptured(PointerDown, ev)
		}
		r.inside = true
		r.setHover(r.hoverAt(ev.Pos))
		if ev.Activating && (r.Hovered() == nil || !r.Hovered().base().AcceptsFirstMouse()) {
			r.swallowed |= ev.Button // the first click only activates the window
			return false
		}
		gen := r.cancels
		for e := r.Hovered(); e != nil; e = parentOf(e) {
			if e.HandleEvent(r.pointerEvent(PointerDown, e, ev, true)) {
				if r.cancels == gen && r.usable(e) {
					r.capture = e
				} else {
					// The handler opened a grab popup (CancelPointer) or
					// removed, hid or disabled its own element: the press
					// is over before it captured, and the element must
					// not stay pressed (B1).
					e.HandleEvent(PointerEvent{Kind: PointerCancel, Pos: FromRoot(e, ev.Pos), Buttons: ev.Buttons, Mods: ev.Mods, Time: ev.Time})
				}
				return true
			}
			if r.cancels != gen {
				return false
			}
		}
		return false
	case shell.PointerUp:
		if r.swallowed&ev.Button != 0 {
			r.swallowed &^= ev.Button
			if r.capture == nil && ev.Buttons == 0 {
				r.setHover(r.hoverAt(ev.Pos))
			}
			return false
		}
		if r.capture == nil {
			if ev.Buttons == 0 {
				r.setHover(r.hoverAt(ev.Pos))
			}
			if r.strayUps {
				if r.Hovered() == nil {
					// Released outside every element (a grab popup's
					// press-drag-release ending off the popup): the root
					// gets it, not over it.
					root := r.root.self
					return root.HandleEvent(r.pointerEvent(PointerUp, root, ev, false))
				}
				return r.bubblePointer(PointerUp, ev)
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
// path from the root to the topmost element under p. The chain is in a
// scratch buffer that the next call overwrites (P15).
func (r *Router) hoverAt(p driver.Point) []Element {
	hit, _ := HitTest(r.root.self, p)
	n := 0
	for e := hit; e != nil; e = parentOf(e) {
		n++
	}
	path := slices.Grow(r.scratch[:0], n)[:n]
	r.scratch = path
	i := n
	for e := hit; e != nil; e = parentOf(e) {
		i--
		path[i] = e
	}
	for i, e := range path {
		if e.base().disabled {
			return path[:i]
		}
	}
	return path
}

// setHover replaces the hover chain, sending PointerLeave to the elements
// that left it (deepest first) and PointerEnter to those that joined (root
// side first). An unchanged chain (a move inside the same element) sends
// nothing and allocates nothing.
func (r *Router) setHover(chain []Element) {
	defer r.RefreshCursor()
	if slices.Equal(chain, r.hover) {
		return
	}
	chain = slices.Clone(chain) // a handler below may reuse the scratch
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

// Cursor returns the pointer shape the router set last (CursorArrow
// initially).
func (r *Router) Cursor() shell.Cursor { return r.cursor }

// SetCursorFunc sets the function that applies a new pointer shape (the
// window passes its Surface.SetCursor); nil applies nothing. It is called
// only on a change, not for the current shape.
func (r *Router) SetCursorFunc(f func(shell.Cursor)) { r.setCursor = f }

// SetFocusChangedFunc sets the function called after each change of the
// focus (see Router); nil calls nothing.
func (r *Router) SetFocusChangedFunc(f func()) { r.focusChanged = f }

// RefreshCursor re-reads the Cursor of the deepest element of the hover
// chain and applies it if it changed. It does nothing during a capture,
// which freezes the shape.
func (r *Router) RefreshCursor() {
	if r.capture != nil {
		return
	}
	c := shell.CursorArrow
	if e := r.Hovered(); e != nil {
		c = e.Cursor()
	}
	if c == r.cursor {
		return
	}
	r.cursor = c
	if r.setCursor != nil {
		r.setCursor(c)
	}
}

// SetStrayReleases sets whether a PointerUp outside a capture (a release
// whose press the router never saw) goes to the deepest element of the
// hover chain and bubbles up like a move, or to the root with Over false
// when the pointer is over no element (outside the root). Off by default;
// the router of a grab popup turns it on (S5.5g), because the press that
// opened the popup is released over it or, ending a press-drag-release
// that cancels the menu, outside it (S5.5i).
func (r *Router) SetStrayReleases(on bool) { r.strayUps = on }

// CancelPointer ends the router's pointer state as if the pointer had left
// the window: the capture gets PointerCancel, the hover chain PointerLeave
// and the scroll latch ends; the pointer counts as outside until the next
// shell pointer event. A press that is being routed (CancelPointer called
// from a PointerDown handler) captures nothing: the element that handles
// it gets PointerCancel right after. The window calls it when a grab popup
// opens (S5.5g), whose input the window then no longer gets.
func (r *Router) CancelPointer() {
	r.cancels++
	r.cancelCapture()
	r.swallowed = 0
	r.endLatch()
	r.inside = false
	r.setHover(nil)
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
		r.setFocus(nil, false)
		return true
	}
	if !r.focusable(e) {
		return false
	}
	r.setFocus(e, false)
	return r.focus == e
}

// setFocus moves the focus; tab marks a move by FocusNext (FocusEvent.Tab).
func (r *Router) setFocus(e Element, tab bool) {
	old := r.focus
	if old == e {
		return
	}
	r.focus = e
	if old != nil {
		old.HandleEvent(FocusEvent{Focused: false})
	}
	if e != nil && r.focus == e {
		e.HandleEvent(FocusEvent{Focused: true, Tab: tab})
	}
	if e != nil {
		r.revealFocus(e)
	}
	if r.focusChanged != nil {
		r.focusChanged()
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
// keyboard-access mode (its visibility and state aside); of a group that
// is one Tab stop only the group's stop is. gs memoises the group stop
// among the children of b's parent, so it is found once per parent, not
// once per radio (P14).
func (r *Router) tabStop(b *Base, gs *groupStopMemo) bool {
	if !b.focusable || (r.limited && b.focusKind != FocusText) {
		return false
	}
	g, ok := b.self.(groupTabStop)
	if !ok {
		return true
	}
	if !gs.done {
		gs.stop, gs.done = g.groupStop(), true
	}
	return gs.stop == b.self
}

// groupTabStop is an element of a group that is one Tab stop (the radios
// of a group, radio.go, all among the children of one parent): groupStop
// returns the group's stop, or nil when it has none.
type groupTabStop interface{ groupStop() Element }

// groupStopMemo is the group stop among the children of one parent.
type groupStopMemo struct {
	stop Element
	done bool
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
	var walk func(b *Base, gs *groupStopMemo)
	walk = func(b *Base, gs *groupStopMemo) {
		if b.hidden || b.disabled {
			return
		}
		stop := r.tabStop(b, gs)
		if r.focus != nil && r.focus.base() == b {
			at, exact = len(order), stop
		}
		if stop {
			order = append(order, b.self)
		}
		var cgs groupStopMemo
		for _, c := range b.children {
			walk(c.base(), &cgs)
		}
	}
	walk(r.root, &groupStopMemo{})
	return order, at, exact
}

// FocusNext moves the focus to the next element of FocusOrder (the previous
// one when backward), wrapping around; without a focus it goes to the first
// (last) one. A focus outside the Tab order (a control clicked without full
// keyboard access) moves to the first Tab stop after it in tree order (the
// last one before it when backward). It reports whether there was an
// element to focus. The element it focuses gets FocusEvent{Focused: true,
// Tab: true}.
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
	r.setFocus(order[i], true)
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
		r.setFocus(nil, false)
	}
	r.RefreshCursor() // a cancelled capture unfreezes the shape
	if r.treeChanged != nil {
		r.treeChanged()
	}
}

// SetTreeChangedFunc sets the function called after the router dropped
// the state a tree change made invalid: after an element joined or left
// the tree or was hidden, shown, disabled, enabled or made unfocusable
// (the window's tooltips check theirs there, S5.5l); nil calls nothing.
// It must not change the tree.
func (r *Router) SetTreeChangedFunc(f func()) { r.treeChanged = f }

// routedBy returns the router of b's tree, or nil.
func (b *Base) routedBy() *Router { return b.tree().root.router }

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

// AcceptsFirstMouse reports whether the element takes the press that
// activates its window (shell.PointerEvent.Activating), set by
// SetAcceptsFirstMouse on it or an ancestor.
func (b *Base) AcceptsFirstMouse() bool {
	for ; b != nil; b = b.parent {
		if b.firstMouse {
			return true
		}
	}
	return false
}

// SetAcceptsFirstMouse sets whether the element and its descendants take
// the press that activates their window, like AppKit's acceptsFirstMouse:
// off by default, so that press only activates the window (S6.1b); the
// title bar turns it on for the traffic lights and the menu strip.
func (b *Base) SetAcceptsFirstMouse(on bool) { b.firstMouse = on }

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
