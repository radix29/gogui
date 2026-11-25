package aqua

import (
	"time"
	"unicode"

	"github.com/radix29/gogui/internal/driver"
)

// The menu's interaction timings and speeds (PLAN.md S5.5i; provisional
// like the tokens, from memory of macOS).
const (
	// menuReleaseDelay: the opening press released sooner, and without a
	// drag, leaves the menu open for a click; released later it picks.
	menuReleaseDelay = 300 * time.Millisecond
	// menuDragSlop: the pointer moved further than this from where the
	// menu first saw it drags (the release then picks at once).
	menuDragSlop = 3.0
	// menuTypeSelectReset: a character typed later than this after the
	// previous one starts a new type-select prefix.
	menuTypeSelectReset = time.Second
	// menuBlinkOff and menuBlinkOn: a pick turns the item's highlight off
	// for menuBlinkOff, on again for menuBlinkOn, then reports.
	menuBlinkOff = 70 * time.Millisecond
	menuBlinkOn  = 70 * time.Millisecond
	// menuArrowSpeed: how fast a hovered scroll arrow scrolls, in points
	// per second, for at most menuArrowMaxStep of time per frame.
	menuArrowSpeed   = 240.0
	menuArrowMaxStep = 100 * time.Millisecond
)

// menuOpening says how a menu was opened (menu.Begin).
type menuOpening struct {
	// Press is set when the menu opened from a pointer press that is still
	// held (a pop-up button's PointerDown); Time is that press's time.
	// Without Press (opened by a key) the first release is a click's.
	Press bool
	Time  time.Duration
	// With Press, HasPos says that Pos is where the press is, in the
	// menu's points: the start of a drag, as if the menu had seen the
	// pointer there (the shells send a grab popup its first pointer event
	// only with the next move). Without it the first event seen is the
	// start.
	Pos    driver.Point
	HasPos bool
	// With Press, Dragged says that the press has dragged already (across
	// a menu strip's titles, S5.6f): its release picks or cancels at once.
	Dragged bool
}

// menuTrack is the state of a menu's interaction from Begin to its report
// (see menu.Begin).
type menuTrack struct {
	active bool // between Begin and the report or End

	press   bool          // the opening press is still held
	pressAt time.Duration // its time
	start   driver.Point  // where the menu first saw the pointer
	seen    bool          // start is set
	dragged bool          // moved past menuDragSlop from start

	ptr   driver.Point // the pointer's last position
	ptrIn bool         // the pointer is over the menu

	typed typeSelect // the type-select (typeselect.go)

	arrow int // the hovered scroll arrow: -1 up, 1 down, 0 none

	picking int    // the item being picked (blinking), or -1
	cancel  func() // the pending blink timer's cancel, or nil
}

// SetActions sets what the menu reports after Begin: pick with the index
// of the picked item, cancel when the menu is left without a pick (Escape,
// a release outside it). Either may be nil. The owner closes the popup in
// them.
func (m *menu) SetActions(pick func(i int), cancel func()) {
	m.onPick, m.onCancel = pick, cancel
}

// SetSideKeys hands Left and Right to the owner while tracking (S5.6i): f
// gets -1 for Left and 1 for Right (a menu strip switches to the
// neighbour menu). nil, the default, leaves the keys unhandled.
func (m *menu) SetSideKeys(f func(dir int)) { m.onSide = f }

// CancelledByKey reports whether the last cancel came from the keyboard
// (Escape, or Return or Space with nothing highlighted) rather than the
// pointer; the owner may ask it in its cancel action (S5.6i).
func (m *menu) CancelledByKey() bool { return m.keyCancel }

// Tracking reports whether the menu is between Begin and its report.
func (m *menu) Tracking() bool { return m.trk.active }

// Begin starts the interaction of a menu just opened in a grab popup
// (PLAN.md S5.5i, as an NSMenu): it takes the popup's focus, so the keys
// reach it, and remembers how it was opened.
//
//   - Hover: the enabled item under the pointer is highlighted as the
//     pointer moves, nothing over a disabled item, a separator, the
//     padding, a scroll arrow or outside; a pointer standing still keeps
//     a highlight the keyboard made.
//   - Press-drag-release (op.Press): the opening press's release over an
//     enabled item picks it, and outside the menu cancels, once the
//     pointer dragged past menuDragSlop or menuReleaseDelay passed; a
//     release sooner and without a drag leaves the menu open for a click.
//     Click: a release over an enabled item picks it, outside the menu (a
//     press inside dragged off) cancels, elsewhere in it does nothing.
//   - Keyboard: Up and Down move the highlight to the previous or next
//     enabled item (from none: the last or the first), not wrapping;
//     Home and Page Up to the first, End and Page Down to the last; typed
//     characters highlight an enabled item by type-select
//     (typeSelect: a prefix growing while the characters come within
//     menuTypeSelectReset of each other, the same letter again stepping
//     to the next item with it); Return and Space (not while a
//     prefix grows: it is typed then) pick the highlighted item, or cancel
//     without one; Escape cancels. A highlight the keyboard made is
//     scrolled into view. Left and Right go to the owner
//     (SetSideKeys), or stay unhandled.
//   - Pick: the item blinks once (its highlight off for menuBlinkOff, on
//     for menuBlinkOn, over Shell.After), then the pick is reported; with
//     Reduce Motion (or outside a window) it is reported at once, and
//     Reduce Motion turned on during the blink (the WindowStateEvent)
//     reports it at once too (S6.1f). Input is
//     eaten meanwhile.
//   - Scrolling, in a panel lower than its items: a hovered scroll arrow
//     scrolls that way at menuArrowSpeed on the frame clock (an
//     anim.Ticker of the window's timeline; none without one) until the
//     end, where its strip goes; the wheel and the touchpad scroll at
//     once, a wheel line by WheelLineHeight, the highlight following the
//     pointer.
//
// The interaction ends at the report, with End, and when the menu loses
// the focus (its popup closed); nothing is reported then.
func (m *menu) Begin(op menuOpening) {
	m.End()
	m.trk = menuTrack{active: true, press: op.Press, pressAt: op.Time, dragged: op.Press && op.Dragged, picking: -1}
	if op.Press && op.HasPos {
		m.moved(op.Pos)
	}
	m.Focus()
}

// EndPress ends the opening press without a report, as a quick release
// that leaves the menu open for a click (a menu strip's title released
// on, S5.6f).
func (m *menu) EndPress() { m.trk.press = false }

// Picking reports whether a pick is blinking (input is eaten until its
// report).
func (m *menu) Picking() bool { return m.trk.picking >= 0 }

// End stops the interaction without a report: the blink's timer and the
// arrow's ticker stop. The highlight stays.
func (m *menu) End() {
	t := &m.trk
	if t.cancel != nil {
		t.cancel()
	}
	if m.ticker != nil {
		m.ticker.Stop()
	}
	*t = menuTrack{picking: -1}
}

// HandleEvent implements Element (see Begin).
func (m *menu) HandleEvent(ev Event) bool {
	t := &m.trk
	if fe, ok := ev.(FocusEvent); ok {
		if !fe.Focused {
			m.End()
		}
		return false
	}
	if se, ok := ev.(WindowStateEvent); ok {
		if t.active && t.picking >= 0 && se.Appearance.ReduceMotion {
			m.report(t.picking) // Reduce Motion turned on mid-blink (S6.1f)
		}
		return false
	}
	if !t.active {
		return false
	}
	if t.picking >= 0 { // the blink eats the input
		switch ev := ev.(type) {
		case PointerEvent:
			return ev.Kind != PointerLeave
		case KeyEvent:
			return ev.Down
		}
		return true
	}
	switch ev := ev.(type) {
	case PointerEvent:
		return m.pointer(ev)
	case ScrollEvent:
		return m.wheel(ev)
	case KeyEvent:
		if ev.Down {
			return m.key(ev)
		}
	case TextEvent:
		return m.typeSelect(ev)
	}
	return false
}

// pointer handles a pointer event while tracking.
func (m *menu) pointer(ev PointerEvent) bool {
	t := &m.trk
	switch ev.Kind {
	case PointerEnter, PointerMove:
		if t.ptrIn && ev.Pos == t.ptr {
			return true // no movement: the keyboard's highlight stays
		}
		m.moved(ev.Pos)
		return true
	case PointerLeave:
		if t.press {
			t.dragged = true // dragged off the menu
		}
		t.ptrIn = false
		m.setArrow(0)
		m.SetHighlighted(-1)
		return false
	case PointerDown:
		m.moved(ev.Pos)
		return true
	case PointerCancel:
		return true
	case PointerUp:
		if ev.Buttons != 0 {
			return true // until the last button
		}
		if t.press {
			t.press = false
			if t.seen && !t.dragged && ev.Pos != t.ptr {
				m.moved(ev.Pos) // moved with the release
			}
			if !t.dragged && ev.Time-t.pressAt < menuReleaseDelay {
				return true // a click: the menu stays open
			}
		}
		m.release(ev.Pos, ev.Over && m.Panel().Contains(ev.Pos))
		return true
	}
	return false
}

// moved follows the pointer to p, over the menu or captured outside it:
// the drag, the scroll arrow and the highlight.
func (m *menu) moved(p driver.Point) {
	t := &m.trk
	if !t.seen {
		t.start, t.seen = p, true
	} else if dx, dy := p.X-t.start.X, p.Y-t.start.Y; dx*dx+dy*dy > menuDragSlop*menuDragSlop {
		t.dragged = true
	}
	t.ptr = p
	t.ptrIn = m.Bounds().Contains(p)
	m.follow()
}

// follow updates the hovered scroll arrow and the highlight from the
// pointer's last position.
func (m *menu) follow() {
	t := &m.trk
	p := t.ptr
	switch {
	case !t.ptrIn:
		m.setArrow(0)
	case m.ArrowRect(-1).Contains(p):
		m.setArrow(-1)
	case m.ArrowRect(1).Contains(p):
		m.setArrow(1)
	default:
		m.setArrow(0)
	}
	if i := m.ItemAt(p); t.ptrIn && m.Selectable(i) {
		m.SetHighlighted(i)
	} else {
		m.SetHighlighted(-1)
	}
}

// release ends a click or a press-drag-release at p: a pick over an
// enabled item, a cancel outside the panel, nothing elsewhere.
func (m *menu) release(p driver.Point, inside bool) {
	if !inside {
		m.cancelMenu()
		return
	}
	if i := m.ItemAt(p); m.Selectable(i) {
		m.pick(i)
	}
}

// key handles a key-down while tracking.
func (m *menu) key(ev KeyEvent) bool {
	t := &m.trk
	switch ev.Key {
	case driver.KeyUp:
		m.keyHighlight(m.step(m.highlight, -1))
	case driver.KeyDown:
		m.keyHighlight(m.step(m.highlight, 1))
	case driver.KeyHome, driver.KeyPageUp:
		m.keyHighlight(m.step(-1, 1))
	case driver.KeyEnd, driver.KeyPageDown:
		m.keyHighlight(m.step(len(m.items), -1))
	case driver.KeySpace:
		if t.typed.growing(ev.Time) {
			return false // typed: its text extends the prefix
		}
		m.pickHighlighted()
	case driver.KeyReturn:
		m.pickHighlighted()
	case driver.KeyEscape:
		m.cancelBy(true)
	case driver.KeyLeft, driver.KeyRight:
		if m.onSide == nil {
			return false
		}
		dir := 1
		if ev.Key == driver.KeyLeft {
			dir = -1
		}
		m.onSide(dir)
	default:
		return false
	}
	return true
}

// step returns the next enabled item after i in direction dir (from none,
// -1, Up starts at the end), or i when there is none.
func (m *menu) step(i, dir int) int {
	if i < 0 && dir < 0 {
		i = len(m.items)
	}
	for j := i + dir; j >= 0 && j < len(m.items); j += dir {
		if m.Selectable(j) {
			return j
		}
	}
	return i
}

// keyHighlight highlights item i from the keyboard and scrolls it into
// view; a non-selectable i changes nothing.
func (m *menu) keyHighlight(i int) {
	if !m.Selectable(i) {
		return
	}
	m.SetHighlighted(i)
	m.ScrollToItem(i)
}

// pickHighlighted picks the highlighted item, or cancels without one (a
// key's cancel: only the keys call it).
func (m *menu) pickHighlighted() {
	if m.Selectable(m.highlight) {
		m.pick(m.highlight)
	} else {
		m.cancelBy(true)
	}
}

// typeSelect types ev's text into the type-select (typeSelect) and
// highlights the enabled item it picks.
func (m *menu) typeSelect(ev TextEvent) bool {
	taken := false
	for _, r := range ev.Text {
		if unicode.IsControl(r) {
			continue
		}
		taken = true
		i := m.trk.typed.next(r, ev.Time, len(m.items), m.highlight,
			func(i int) string { return m.items[i].Title }, m.Selectable)
		if i >= 0 {
			m.keyHighlight(i)
		}
	}
	return taken
}

// pick picks item i: the blink, then the report (see Begin).
func (m *menu) pick(i int) {
	t := &m.trk
	m.setArrow(0)
	t.picking = i
	w := m.Window()
	if w == nil || w.closed || m.appearance().ReduceMotion {
		m.report(i)
		return
	}
	m.SetHighlighted(-1)
	t.cancel = w.sh.After(menuBlinkOff, func() {
		m.SetHighlighted(i)
		t.cancel = w.sh.After(menuBlinkOn, func() {
			t.cancel = nil
			if m.Window() != w {
				m.End() // detached without the focus: no report
				return
			}
			m.report(i)
		})
	})
}

// report ends the interaction with the pick of item i, left highlighted.
func (m *menu) report(i int) {
	m.End()
	m.SetHighlighted(i)
	if m.onPick != nil {
		m.onPick(i)
	}
}

// cancelMenu ends the interaction with a cancel by the pointer.
func (m *menu) cancelMenu() { m.cancelBy(false) }

// cancelBy ends the interaction with a cancel, by a key or not
// (CancelledByKey).
func (m *menu) cancelBy(key bool) {
	m.End()
	m.keyCancel = key
	if m.onCancel != nil {
		m.onCancel()
	}
}

// wheel scrolls the items by a scroll event, at once, and reports whether
// they can scroll at all.
func (m *menu) wheel(ev ScrollEvent) bool {
	if m.MaxScroll() == 0 {
		return false
	}
	d := ev.Delta.Y
	if !ev.Precise {
		d *= tokens.Scroll.WheelLineHeight
	}
	m.SetScrollOffset(m.top - d)
	if m.trk.ptrIn {
		m.follow()
	}
	return true
}

// setArrow sets the hovered scroll arrow; the ticker runs while one is.
func (m *menu) setArrow(dir int) {
	t := &m.trk
	if dir == t.arrow {
		return
	}
	t.arrow = dir
	if dir == 0 {
		if m.ticker != nil {
			m.ticker.Stop()
		}
		return
	}
	if m.ticker == nil {
		w := m.Window()
		if w == nil || w.Timeline() == nil {
			return // no frame clock: the arrow does not scroll
		}
		m.ticker = w.Timeline().NewTicker(m.arrowTick)
	}
	m.ticker.Start()
}

// arrowTick scrolls towards the hovered arrow for one frame; at the end
// the arrow's strip goes and the pointer is followed again.
func (m *menu) arrowTick(dt time.Duration) {
	t := &m.trk
	if t.arrow == 0 {
		m.ticker.Stop()
		return
	}
	m.SetScrollOffset(m.top + float64(t.arrow)*menuArrowSpeed*min(dt, menuArrowMaxStep).Seconds())
	if m.arrowStrip(t.arrow) == 0 {
		m.follow()
	}
}
