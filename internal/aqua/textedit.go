package aqua

import (
	"time"
	"unicode/utf8"

	"github.com/radix29/gogui/internal/aqua/text"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// textEdit is the text element base (PLAN.md S5.4a, review R5): what the
// TextField and the TextView share. It holds the editor and everything
// around it that does not depend on how the text is laid out: the event
// dispatch (focus, window state, keys, typed text, the IME's composition,
// the mouse), the focus ring, the caret blink over Shell.After, the
// clipboard commands with the late-paste guard, the composition (cleaned,
// committed, dropped, cancelled) and the mouse selection by its unit. The
// element embeds it (next to its Base) and sets host to itself, which
// answers what differs between the elements; see editHost.
//
// The rules are those documented on TextField: the caret blinks by the
// window's Appearance.CaretBlink, one timer per half period and only while
// the caret is shown and no composition is; a Paste is dropped when it
// arrives after the element lost the focus (even if it got it back),
// became disabled or read-only, or its window closed; an interruption
// commits the composition as the IME would and cancels the shell's, so no
// late commit lands elsewhere.
type textEdit struct {
	d    *Driver
	ed   *text.Editor
	host editHost
	ring FocusRing

	blink caretBlink  // the caret's blink
	focus uint64      // counts the focus events, so a late paste can tell
	pre   textPreedit // the IME's composition, if any
	drag  textDrag    // the mouse selection in progress
}

// editHost is what textEdit needs from the element that embeds it.
type editHost interface {
	// base returns the element's Base (focus, enabled state, root and
	// invalidation).
	base() *Base

	// takesPreedit reports whether the element takes IME compositions now.
	takesPreedit() bool

	// secureText reports whether the text is secret: no copy, no cut, and
	// a double-click selects everything.
	secureText() bool

	// readOnlyText reports whether the text is read-only: no cut, no
	// paste, no typing, no caret.
	readOnlyText() bool

	// key handles an enabled element's key-down or repeat by its key map
	// (the composition is committed already) and reports whether it took
	// it.
	key(ev shell.KeyEvent) bool

	// edited repaints after a command, restarts the caret blink and, when
	// the text changed since the revision rev, reports the change.
	edited(rev uint64)

	// preeditChanged tells the element the composition (textEdit.pre)
	// started, changed or ended, before it repaints: the TextView splices
	// it into its paragraph's layout; the TextField lays it out lazily.
	preeditChanged()

	// command runs a bound command that is not a clipboard one, as its key
	// does (the Edit menu, S5.6g): the editor's Do, then edited.
	command(cmd text.Command)

	// The three mouse hooks: hitAt returns the editor offset for a press
	// or a drag at p (the element's coordinates) and whether the caret
	// there is upstream; selected repaints after the mouse changed the
	// selection (the TextField also scrolls the head into view);
	// autoScroll(true) starts or stops the drag's auto-scroll by where the
	// pointer (textDrag.pos) is, autoScroll(false) stops it.
	hitAt(p driver.Point) (off int, upstream bool)
	selected()
	autoScroll(on bool)
}

// initEdit sets up the glue over a new editor for the element host.
func (t *textEdit) initEdit(d *Driver, host editHost, ed *text.Editor) {
	t.d, t.host, t.ed = d, host, ed
}

// handleEvent is the element's HandleEvent (see TextField and TextView). It
// tracks the focus ring; a focus change or a change of the window's state
// repaints and restarts the caret blink (the caret, the selection and its
// color depend on them), a blur ends the drag and commits the composition,
// as does the window losing key, and a Tab focus of a single-line editor
// selects all (a Focus keeps the selection). An enabled element's key-down
// or repeat commits the composition and goes to the host's key map; typed
// text, compositions and the mouse are handled here.
func (t *textEdit) handleEvent(ev Event) bool {
	b := t.host.base()
	t.ring.Track(b.Self(), ev)
	switch ev := ev.(type) {
	case FocusEvent:
		t.focus++ // a paste read before it is dropped
		if !ev.Focused {
			t.endDrag()
			t.commitPreedit()
		}
		if ev.Focused && ev.Tab && t.ed.SingleLine() {
			t.ed.SelectAll()
		}
		t.restartBlink()
		b.Invalidate()
	case WindowStateEvent:
		if b.FocusState() != FocusedActive {
			t.commitPreedit() // the window lost key
		}
		t.restartBlink()
		b.Invalidate()
	case KeyEvent:
		if !ev.Down || !b.EnabledInTree() {
			return false
		}
		t.commitPreedit() // the shells give the keys of a composition to the IME; this one passed it
		return t.host.key(ev.KeyEvent)
	case TextEvent:
		return t.typeText(ev.Text)
	case PreeditEvent:
		return t.preedit(ev.PreeditEvent)
	case PointerEvent:
		return t.pointer(ev)
	}
	return false
}

// typeText inserts typed text in place of the selection; a read-only
// element takes it and changes nothing. During a composition s is the
// IME's commit: it replaces the preedit as an undo group of its own.
func (t *textEdit) typeText(s string) bool {
	if !t.host.base().EnabledInTree() {
		return false
	}
	if t.composing() {
		t.typeCommit(s)
		return true
	}
	if !t.host.readOnlyText() {
		rev := t.ed.Buffer().Revision()
		t.ed.Insert(s)
		t.host.edited(rev)
	}
	return true
}

// caretShown reports whether the element shows a caret, blink aside: while
// it holds the focus in a key window, the selection is empty, and it is
// enabled and editable.
func (t *textEdit) caretShown() bool {
	b := t.host.base()
	start, end := t.ed.Selection()
	return start == end && !t.host.readOnlyText() && b.FocusState() == FocusedActive && b.EnabledInTree()
}

// textCursor is the element's Cursor: the I-beam over an enabled element
// (editable or read-only), else the arrow.
func (t *textEdit) textCursor() shell.Cursor {
	if !t.host.base().EnabledInTree() {
		return shell.CursorArrow
	}
	return shell.CursorIBeam
}

// textPreedit is the IME's composition in a text element: its text (empty:
// none), the IME's caret and the clauses, offsets in bytes of text. It is
// shown at the editor's caret (the selection is empty while composing).
type textPreedit struct {
	text   string
	cursor int
	segs   []shell.PreeditSegment
}

// composing reports whether a composition is shown.
func (t *textEdit) composing() bool { return t.pre.text != "" }

// preedit shows, changes or ends the IME's composition (see TextField) and
// reports whether the element took the event. The first preedit deletes a
// non-empty selection as an edit of its own.
func (t *textEdit) preedit(ev shell.PreeditEvent) bool {
	if !t.host.takesPreedit() {
		return false
	}
	if ev.Text == "" {
		if t.composing() {
			t.pre = textPreedit{}
			t.composed()
		}
		return true
	}
	if !t.composing() {
		t.drag.on = false
		if start, end := t.ed.Selection(); start != end {
			rev := t.ed.Buffer().Revision()
			t.ed.Cut()
			t.host.edited(rev)
			if !t.host.takesPreedit() || !t.host.base().HasFocus() { // the handler changed the element
				return true
			}
		}
	}
	t.pre = cleanPreedit(ev)
	t.composed()
	return true
}

// cleanPreedit returns the composition of ev with the cursor clamped to
// the text and the clauses that break the PreeditEvent rules dropped.
func cleanPreedit(ev shell.PreeditEvent) textPreedit {
	s := ev.Text
	at := func(i int) bool { return i >= 0 && i <= len(s) && (i == len(s) || utf8.RuneStart(s[i])) }
	p := textPreedit{text: s, cursor: len(s)}
	if at(ev.Cursor) {
		p.cursor = ev.Cursor
	}
	prev := 0
	for _, sg := range ev.Segments {
		if at(sg.Start) && at(sg.End) && sg.Start < sg.End && sg.Start >= prev {
			p.segs = append(p.segs, sg)
			prev = sg.End
		}
	}
	return p
}

// composed tells the host the composition changed, repaints and shows the
// caret (solid while composing, blinking again once it ended).
func (t *textEdit) composed() {
	t.host.preeditChanged()
	t.host.base().Invalidate()
	t.restartBlink()
}

// typeCommit ends the shown composition with the IME's commit s, which
// replaces the preedit.
func (t *textEdit) typeCommit(s string) {
	t.pre = textPreedit{}
	t.host.preeditChanged()
	t.insertCommit(s)
}

// insertCommit inserts an IME commit at the caret as an undo group of its
// own, reported through edited.
func (t *textEdit) insertCommit(s string) {
	rev := t.ed.Buffer().Revision()
	t.ed.BreakUndoGroup()
	t.ed.Insert(s)
	t.ed.BreakUndoGroup()
	t.host.edited(rev)
}

// commitPreedit commits the shown preedit, if any, as the IME would (a
// blur, a click, the window losing key; see TextField): the shell's
// composition is cancelled first, so the IME's own commit never arrives,
// and the preedit is inserted as a commit.
func (t *textEdit) commitPreedit() {
	if !t.composing() {
		return
	}
	s := t.pre.text
	t.pre = textPreedit{}
	t.host.preeditChanged()
	t.cancelComposition()
	t.insertCommit(s)
}

// dropPreedit forgets the shown preedit, if any, and cancels the shell's
// composition (SetText, UserType).
func (t *textEdit) dropPreedit() {
	if t.composing() {
		t.pre = textPreedit{}
		t.cancelComposition()
		t.composed()
	}
}

// cancelComposition tells the element's surface to drop the IME's
// composition (nothing outside an open window).
func (t *textEdit) cancelComposition() {
	if r, ok := t.host.base().Root().(*windowRoot); ok && !r.w.Closed() {
		r.w.surface.CancelComposition()
	}
}

// clipboard runs Copy, Cut or Paste (see TextField).
func (t *textEdit) clipboard(cmd text.Command) {
	switch cmd {
	case text.CmdCopy:
		if s := t.ed.Copy(); s != "" && !t.host.secureText() {
			t.d.sh.SetClipboardText(s)
		}
	case text.CmdCut:
		if t.host.secureText() || t.host.readOnlyText() {
			return
		}
		rev := t.ed.Buffer().Revision()
		if s := t.ed.Cut(); s != "" {
			t.d.sh.SetClipboardText(s)
		}
		t.host.edited(rev)
	case text.CmdPaste:
		if t.host.readOnlyText() {
			return
		}
		focus := t.focus
		t.d.sh.ReadClipboardText(func(s string, ok bool) { t.paste(focus, s, ok) })
	}
}

// paste inserts the clipboard's text s, read for a Paste made while the
// focus count was focus, unless the read failed or came too late: the
// element lost the focus since (the count moved on), is disabled or
// read-only, or its window closed.
func (t *textEdit) paste(focus uint64, s string, ok bool) {
	b := t.host.base()
	if !ok || s == "" || focus != t.focus || !b.HasFocus() || !b.EnabledInTree() || t.host.readOnlyText() {
		return
	}
	if r, isWin := b.Root().(*windowRoot); !isWin || r.w.Closed() {
		return
	}
	if t.composing() { // a composition started since the Paste
		t.commitPreedit()
		if !b.HasFocus() || !b.EnabledInTree() || t.host.readOnlyText() {
			return
		}
	}
	rev := t.ed.Buffer().Revision()
	t.ed.Paste(s)
	t.host.edited(rev)
}

// dragUnit is what a mouse drag in a text element selects by.
type dragUnit uint8

const (
	dragChars dragUnit = iota // after a single click
	dragWords                 // after a double-click
	dragParas                 // after a triple-click in a text view
	dragAll                   // after a triple-click in a field (a double-click in a secure one)
)

// textDrag is the state of a mouse selection, from the press to the
// release of the primary button.
type textDrag struct {
	on         bool
	unit       dragUnit
	start, end int          // the selection the press made (the anchor, or the first word)
	pos        driver.Point // the pointer, in the root's coordinates
}

// pointer selects with the mouse (see TextField and TextView) and reports
// whether it took the event. A primary press is taken (and so captures the
// pointer); while it is held every pointer event is taken.
func (t *textEdit) pointer(ev PointerEvent) bool {
	b := t.host.base()
	switch ev.Kind {
	case PointerDown:
		if ev.Button != shell.ButtonPrimary || t.drag.on {
			return t.drag.on
		}
		if !b.EnabledInTree() {
			return false
		}
		t.commitPreedit() // a click ends the composition, as in AppKit
		b.Focus()
		if !b.HasFocus() || !b.EnabledInTree() { // refused, or a focus handler changed the element
			return false
		}
		t.press(ev)
		return true
	case PointerMove:
		if t.drag.on && ev.Buttons&shell.ButtonPrimary != 0 {
			t.dragTo(ev.Pos)
		}
		return t.drag.on
	case PointerUp:
		on := t.drag.on
		if ev.Button == shell.ButtonPrimary {
			t.endDrag()
		}
		return on
	case PointerCancel:
		t.endDrag()
	}
	return false
}

// press places the caret at a click (Shift extends the selection to it),
// selects the word at a double-click (everything in a secure field) and at
// a triple-click everything in a single-line editor or the paragraph with
// its newline in a multi-line one, and starts the drag by that unit.
func (t *textEdit) press(ev PointerEvent) {
	off, up := t.host.hitAt(ev.Pos)
	unit := dragChars
	switch {
	case ev.Clicks >= 3 || ev.Clicks == 2 && t.host.secureText():
		if t.ed.SingleLine() {
			unit = dragAll
			t.ed.SelectAll()
		} else {
			unit = dragParas
			t.ed.SelectParagraph(off)
		}
	case ev.Clicks == 2:
		unit = dragWords
		t.ed.SelectWord(off)
	case ev.Mods&driver.ModShift != 0:
		t.extendTo(off, up)
	default:
		t.ed.SetSelectionUpstream(off, off, up)
	}
	t.startDrag(unit)
	t.drag.pos = ToRoot(t.host.base().Self(), ev.Pos)
	t.host.selected()
	t.host.autoScroll(true)
}

// dragTo follows a drag to p (the element's coordinates): the selection to
// the text there, and the auto-scroll by where the pointer is.
func (t *textEdit) dragTo(p driver.Point) {
	t.drag.pos = ToRoot(t.host.base().Self(), p)
	t.dragSelectAt()
	t.host.autoScroll(true)
}

// dragSelectAt extends the drag's selection to the text under the pointer
// (textDrag.pos), after a move or an auto-scroll step.
func (t *textEdit) dragSelectAt() {
	off, up := t.host.hitAt(FromRoot(t.host.base().Self(), t.drag.pos))
	if t.dragSelect(off, up) {
		t.host.selected()
	}
}

// endDrag ends the mouse selection and its auto-scroll.
func (t *textEdit) endDrag() {
	t.drag.on = false
	t.host.autoScroll(false)
}

// startDrag starts a drag by unit from the selection the press just made.
func (t *textEdit) startDrag(unit dragUnit) {
	t.drag = textDrag{on: true, unit: unit}
	if unit == dragChars {
		t.drag.start = t.ed.Anchor()
	} else {
		t.drag.start, t.drag.end = t.ed.Selection()
	}
}

// dragSelect extends the drag's selection to the editor offset off (upstream:
// the end of a soft-wrapped line, see text.Editor.Upstream): by clusters
// from the anchor, or by words or paragraphs keeping the first one
// selected. It reports whether the unit selects by the pointer (not
// dragAll).
func (t *textEdit) dragSelect(off int, upstream bool) bool {
	switch t.drag.unit {
	case dragChars:
		t.ed.SetSelectionUpstream(t.drag.start, off, upstream)
	case dragWords, dragParas:
		if t.drag.unit == dragWords {
			t.ed.SelectWord(off)
		} else {
			t.ed.SelectParagraph(off)
		}
		ws, we := t.ed.Selection()
		if ws < t.drag.start {
			t.ed.SetSelection(t.drag.end, ws)
		} else {
			t.ed.SetSelection(t.drag.start, max(we, t.drag.end))
		}
	default:
		return false
	}
	return true
}

// extendTo extends the selection to off as a Shift-click does in AppKit:
// the selection grows to take in off, keeping its far end as the anchor;
// a click inside it keeps the anchor. upstream is dragSelect's.
func (t *textEdit) extendTo(off int, upstream bool) {
	start, end := t.ed.Selection()
	anchor := t.ed.Anchor()
	switch {
	case off <= start:
		anchor = end
	case off >= end:
		anchor = start
	}
	t.ed.SetSelectionUpstream(anchor, off, upstream)
}

// caretBlink is the state of a caret's blink.
type caretBlink struct {
	off     bool          // the caret is in the hidden half of its period
	elapsed time.Duration // since the blink (re)started
	cancel  func()        // the pending timer's cancel, or nil
}

// CaretVisible reports whether the caret is drawn now: shown and not in
// the hidden half of its blink.
func (t *textEdit) CaretVisible() bool { return t.caretShown() && !t.blink.off }

// restartBlink shows the caret and blinks it anew from now: the pending
// timer is cancelled, and a new one starts only while the caret is shown
// and the blink setting has a period.
func (t *textEdit) restartBlink() {
	if t.blink.cancel != nil {
		t.blink.cancel()
	}
	if t.blink.off {
		t.host.base().Invalidate()
	}
	t.blink = caretBlink{}
	t.scheduleBlink()
}

// blinkSetting returns the caret blink of the element's window (the
// driver's outside one).
func (t *textEdit) blinkSetting() driver.CaretBlink {
	return appearanceIn(t.host.base(), t.d).CaretBlink
}

// scheduleBlink starts the timer of the next half period while the caret
// is shown and the setting blinks.
func (t *textEdit) scheduleBlink() {
	if half := t.blinkSetting().Period / 2; half > 0 && t.caretShown() && !t.composing() {
		t.blink.cancel = t.d.sh.After(half, t.blinkTick)
	}
}

// blinkTick ends a half period: the caret flips and the next timer
// starts, until Timeout has passed since the restart (the caret then
// stays solid) or the caret is no longer shown (a window that closed
// with the element focused leaves at most this one timer behind).
func (t *textEdit) blinkTick() {
	t.blink.cancel = nil
	b := t.blinkSetting()
	half := b.Period / 2
	t.blink.elapsed += half
	if half <= 0 || !t.caretShown() || b.Timeout > 0 && t.blink.elapsed >= b.Timeout {
		if t.blink.off {
			t.blink.off = false
			t.host.base().Invalidate()
		}
		return
	}
	t.blink.off = !t.blink.off
	t.host.base().Invalidate()
	t.scheduleBlink()
}
