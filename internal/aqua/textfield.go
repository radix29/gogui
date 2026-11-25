package aqua

import (
	"sort"
	"strings"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/aqua/text"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
)

// TextField is the aqua driver.TextField and SecureField (PLAN.md S5.2f): a
// real element over a single-line text.Editor, drawn from the theme's
// TextField tokens (S5.2e). The embedded stub field keeps the state
// (placeholder, font, alignment, read-only flag, control size, handler)
// and its Value mirrors the editor's text; FittingSize stays the stub's
// (defaultWidth × the stub's control height), so the example frames do not
// move.
//
//   - Bezel: the whole frame, pixel-aligned, as an NSTextField's square
//     bezel: filled with ColorTextBackground, the FieldInsetShadow under
//     its top edge and the FieldBorder (FieldBorderWidth, whole device
//     pixels) inside it, FieldCornerRadius round.
//
//   - Text: one line in the field's Font (a zero Size takes the theme's
//     system size for the control size), FieldPaddingX in from either edge
//     and centred vertically, in ColorText (ColorDisabledControlText when
//     disabled), aligned by Alignment (AlignJustified as leading) while it
//     fits. Empty text shows the placeholder instead, in
//     ColorPlaceholderText, truncated with an ellipsis. Everything inside
//     the bezel is clipped to it (its border excluded).
//
//   - Overflow: text wider than the room is drawn leading and scrolled
//     horizontally to keep the caret in view while the field has the
//     focus; without the focus it shows from its start, as an NSTextField
//     does once its field editor is gone.
//
//   - Selection and caret, only while the field holds the focus: the
//     selection rects in Theme.TextSelection (emphasized in a key window,
//     unemphasized otherwise); the caret (CaretWidth wide, the line high,
//     in Theme.Caret, pixel-aligned) when the selection is empty, the
//     window is key and the field is editable (a read-only field, like a
//     selectable NSTextField, shows no insertion point).
//
//   - Caret blink (S5.2i): the caret blinks by the window's
//     Appearance.CaretBlink over Shell.After, one timer per half period
//     and only while the caret is shown (see above), so an unfocused
//     field, an inactive window or a selection runs no timer. Any edit,
//     move, selection, focus or window-state change shows the caret again
//     and restarts the blink; once Timeout has passed since then the caret
//     stays solid. A Period of 0 never blinks. The frame clock is not
//     involved: each phase only invalidates the field.
//
//   - SecureField: one bullet (U+2022) per grapheme cluster, laid out on
//     their own; the editor keeps the real text and the offsets are mapped.
//
//   - Focus: FocusText, and the focus ring around the bezel.
//
//   - Accessibility: a text field with the placeholder as label and the
//     text as value (none for a secure field), read-only when not
//     editable.
//
//   - Keyboard (S5.2g): a key-down or repeat bound by text.Lookup in
//     single-line mode runs through Editor.Do and is taken (moves by
//     every unit, the deletes, SelectAll, Undo, Redo); in a SecureField
//     the word commands act as the line commands (the whole text). A
//     TextEvent replaces the selection (newlines as spaces). Return (no
//     modifiers) fires Submitted on a press and is not taken, so it still
//     reaches the default button; Tab, Shift-Tab, Escape, Space and the
//     character keys go on as well (Space and the characters come back as
//     TextEvents). A Tab focus (FocusEvent.Tab) selects all; a Focus
//     keeps the selection.
//
//   - Clipboard (S5.2i): Copy puts a non-empty selection on the shell's
//     clipboard; Cut does the same and deletes it; Paste reads the
//     clipboard (Shell.ReadClipboardText, always later) and replaces the
//     selection with its text, newlines as spaces, as one undo group. A
//     paste that arrives after the field lost the focus (even if it got
//     it back since), became disabled or read-only, or its window closed
//     is dropped, as is an empty one. A SecureField copies and cuts
//     nothing (the keys are taken) but pastes.
//
//   - Events: TextChanged after every key, text or paste event that
//     changed the text (never SetText); a SetText inside it wins.
//
//   - Read-only: moves, SelectAll and Copy work; edits, typing, Cut,
//     Paste, Undo and Redo are taken and change nothing. Disabled: the
//     router sends it nothing (not focusable, no events).
//
//   - Mouse (S5.2h): a primary press focuses the field (keeping its
//     selection, unlike a Tab focus) and captures the pointer; it places
//     the caret at the nearest cluster boundary under the pointer (by the
//     scroll as shown), Shift extends the selection to it as AppKit does
//     (the far end stays), a double-click selects the word there (the
//     whole text in a SecureField) and a triple-click everything. A drag
//     extends the selection by the press's unit, by clusters from the
//     anchor or by whole words keeping the first word; dragged past
//     either edge the head goes to the text scrolled out there and the
//     text scrolls to show it, by pointer moves only (no timer). Mouse
//     selection works in a read-only field too and fires no event. The
//     cursor is the I-beam over an enabled field (Cursor).
//
//   - IME (S5.3c): an editable field that is not secure takes the
//     PreeditEvents of the IME's composition. The first one deletes a
//     non-empty selection (an undoable edit with TextChanged); the preedit
//     is then drawn spliced in at the caret, in one layout with the text,
//     each clause underlined in the text color (1 pt, 2 pt for the
//     selected one; the whole preedit when the IME reports no clauses),
//     with the caret at the preedit's cursor, kept in view by the scroll
//     and solid (no blink) while composing. An empty preedit ends the
//     composition and leaves the text as it was. The commit (a TextEvent
//     during the composition) replaces the preedit and is an undo group
//     of its own, with TextChanged. A blur, a primary press in the field,
//     the window losing key, a key or a paste arriving during the
//     composition and SetEditable(false) commit the preedit as the IME
//     would (AppKit commits marked text in these cases) and call the
//     surface's CancelComposition, so no late commit of the shell lands
//     elsewhere; SetText and UserType drop it (they replace the text
//     anyway) and cancel too. Secure, read-only and disabled fields take
//     no preedits. The text value and the a11y value never hold the
//     preedit. The field is a TextInputClient (S5.3d): its window turns
//     the IME on while the field takes preedits and has the focus in the
//     key window, with the caret rect as painted, so the candidate window
//     follows the caret, the scroll and the preedit's cursor.
//
// UserType simulates replacing the whole text.
type TextField struct {
	*stub.TextField
	peerBase
	view *textFieldView
}

// NewTextField returns a text field, or a secure field.
func (d *Driver) NewTextField(secure bool, h driver.TextFieldHandler) driver.TextField {
	t := &TextField{TextField: stub.NewTextField(secure, h)}
	t.view = &textFieldView{p: t}
	t.view.initEdit(d, t.view, text.NewEditor("", true))
	t.view.Init(t.view)
	t.view.SetFocusable(true)
	t.view.SetFocusKind(FocusText) // Tab reaches it in both keyboard-access modes
	t.initElement(t.TextField, &t.TextField.Base, t.view)
	return t
}

// SetText implements driver.TextField: the editor's text is replaced
// (newlines turned into spaces), the caret goes to its end and the undo
// history is cleared. No event fires.
func (t *TextField) SetText(s string) {
	t.view.dropPreedit()
	t.view.setText(s)
	t.TextField.SetText(t.view.ed.Text())
	t.view.restartBlink()
	t.redraw()
}

// The setters below update the stub and repaint.
func (t *TextField) SetPlaceholder(s string)             { t.TextField.SetPlaceholder(s); t.redraw() }
func (t *TextField) SetFont(f driver.Font)               { t.TextField.SetFont(f); t.redraw() }
func (t *TextField) SetAlignment(a driver.TextAlignment) { t.TextField.SetAlignment(a); t.redraw() }
func (t *TextField) SetEditable(editable bool) {
	if !editable {
		t.view.commitPreedit()
	}
	t.TextField.SetEditable(editable)
	t.view.restartBlink()
	t.redraw()
}
func (t *TextField) SetControlSize(cs driver.ControlSize) {
	t.TextField.SetControlSize(cs)
	t.redraw()
}

// UserType simulates the user replacing the whole text with s
// (stub.TextField.UserType: refused while disabled or read-only, then
// TextChanged) and repaints.
func (t *TextField) UserType(s string) {
	if t.Disabled || t.ReadOnly {
		return
	}
	t.view.dropPreedit()
	t.view.setText(s)
	t.view.restartBlink()
	t.TextField.UserType(t.view.ed.Text())
	t.redraw()
}

// Editor returns the field's editor (tests and the editing steps).
func (t *TextField) Editor() *text.Editor { return t.view.ed }

// TextFont returns the font the field draws with: its Font with a zero
// Size replaced by the theme's system size for its control size.
func (t *TextField) TextFont() driver.Font { return sizedFont(t.Font, t.ControlSize) }

// bullet is what a SecureField shows for each grapheme cluster.
const bullet = "•"

// textFieldView is the element of a TextField.
type textFieldView struct {
	Base
	textEdit // the editor, events, ring, blink, clipboard, composition and drag (textedit.go)
	p        *TextField

	scroll float64 // the horizontal scroll of overflowing text, ≥ 0

	// The text layout and what it was made from.
	layout    render.TextLayout
	layoutKey fieldLayoutKey
	ph        layoutMemo // the placeholder (untruncated)
	phTrunc   truncMemo  // the placeholder truncated to the room
	memo      textMemo   // the text and its clusters for the revision
}

type fieldLayoutKey struct {
	rev    uint64
	font   driver.Font
	secure bool
	pre    string // the preedit spliced in at preAt
	preAt  int
}

// textMemo is the editor's text for one revision and, in a secure field,
// the start of each of its grapheme clusters (P18): a paint, a key and the
// offset mappings read them without copying the text or counting the
// clusters again.
type textMemo struct {
	ok       bool
	rev      uint64
	s        string
	clusters []int // nil until a secure field needs them
}

// text returns the editor's text, copied once per revision.
func (e *textFieldView) text() string {
	if rev := e.ed.Buffer().Revision(); !e.memo.ok || e.memo.rev != rev {
		e.memo = textMemo{ok: true, rev: rev, s: e.ed.Text()}
	}
	return e.memo.s
}

// clusters returns the start offset of each grapheme cluster of text(),
// found once per revision.
func (e *textFieldView) clusters() []int {
	s := e.text()
	if e.memo.clusters == nil {
		cs := make([]int, 0, len(s))
		for off := 0; off < len(s); off = text.NextGrapheme(s, off) {
			cs = append(cs, off)
		}
		e.memo.clusters = cs
	}
	return e.memo.clusters
}

// setText replaces the editor's text with the caret at its end.
func (e *textFieldView) setText(s string) {
	e.ed.SetText(s)
	n := e.ed.Len()
	e.ed.SetSelection(n, n)
}

// FittingSize returns the peer's FittingSize.
func (e *textFieldView) FittingSize(max driver.Size) driver.Size { return e.p.FittingSize(max) }

// textLayout returns the laid-out text as shown (bullets in a secure
// field), cached on the editor's revision and the font.
func (e *textFieldView) textLayout() render.TextLayout {
	k := fieldLayoutKey{rev: e.ed.Buffer().Revision(), font: e.p.TextFont(), secure: e.p.Secure}
	if e.composing() {
		k.pre, k.preAt = e.pre.text, e.ed.Head()
	}
	if e.layout == nil || e.layoutKey != k {
		s := e.text()
		switch {
		case k.secure:
			s = strings.Repeat(bullet, len(e.clusters()))
		case k.pre != "":
			s = s[:k.preAt] + k.pre + s[k.preAt:]
		}
		e.layout, e.layoutKey = e.d.text.NewLayout(s, k.font, 0), k
	}
	return e.layout
}

// placeholderLayout returns the placeholder on one line, untruncated, or
// nil for none.
func (e *textFieldView) placeholderLayout() render.TextLayout {
	return e.ph.get(e.d.text, e.p.Placeholder, e.p.TextFont())
}

// shownOffset maps an offset of the editor's text to the shown text: the
// same offset (moved past a preedit spliced in at or before it), or in a
// secure field that of the bullet for the cluster starting there.
func (e *textFieldView) shownOffset(off int) int {
	if !e.p.Secure {
		if e.composing() && off >= e.ed.Head() {
			off += len(e.pre.text)
		}
		return off
	}
	// The clusters starting before off.
	return sort.SearchInts(e.clusters(), off) * len(bullet)
}

// caretOffset returns the caret's offset in the shown text: the
// preedit's cursor while composing, else the selection's head.
func (e *textFieldView) caretOffset() int {
	if e.composing() {
		return e.ed.Head() + e.pre.cursor
	}
	return e.shownOffset(e.ed.Head())
}

// bezel returns the bezel rect in the element's coordinates (the frame,
// pixel-aligned) and its corner radius.
func (e *textFieldView) bezel() (driver.Rect, float64) {
	r := pixelAlign(&e.Base, e.Bounds(), scaleIn(&e.Base))
	return r, min(tokens.TextField.FieldCornerRadius.At(e.p.ControlSize), r.W/2, r.H/2)
}

// ScrollX returns the horizontal scroll of the text as last painted.
func (e *textFieldView) ScrollX() float64 { return e.scroll }

// HandleEvent is the shared text element's (textEdit.handleEvent; see
// TextField).
func (e *textFieldView) HandleEvent(ev Event) bool { return e.handleEvent(ev) }

// Cursor implements Element: the I-beam over an enabled field (editable
// or read-only), else the arrow.
func (e *textFieldView) Cursor() shell.Cursor { return e.textCursor() }

// key implements editHost: a key bound in a single-line editor
// (text.Lookup) runs and is taken. Return fires Submitted (on a press, not
// a repeat) and goes on to the default button; unbound keys (Tab, Escape,
// Space, the characters) go on too, the characters to come back as a
// TextEvent.
func (e *textFieldView) key(ev shell.KeyEvent) bool {
	if ev.Key == driver.KeyReturn && ev.Mods == 0 {
		if !ev.Repeat {
			e.p.UserSubmit()
		}
		return false
	}
	cmd, ok := text.Lookup(ev.Key, ev.Mods, false)
	if !ok {
		return false
	}
	if e.p.Secure {
		cmd = secureCommand(cmd)
	}
	switch {
	case cmd.IsClipboard():
		e.clipboard(cmd)
	case cmd.Edits() && e.p.ReadOnly:
	default:
		e.command(cmd)
	}
	return true
}

// command implements editHost: the editor's Do (no layout: one line),
// then edited.
func (e *textFieldView) command(cmd text.Command) {
	rev := e.ed.Buffer().Revision()
	e.ed.Do(cmd, nil)
	e.edited(rev)
}

// secureCommand maps the word commands to the line commands, as an
// NSSecureTextField does: a secure field has no words to reveal.
func secureCommand(c text.Command) text.Command {
	switch c {
	case text.CmdMoveWordLeft:
		return text.CmdMoveToLineStart
	case text.CmdSelectWordLeft:
		return text.CmdSelectToLineStart
	case text.CmdMoveWordRight:
		return text.CmdMoveToLineEnd
	case text.CmdSelectWordRight:
		return text.CmdSelectToLineEnd
	case text.CmdDeleteWordBackward:
		return text.CmdDeleteToLineStart
	case text.CmdDeleteWordForward:
		return text.CmdDeleteToLineEnd
	}
	return c
}

// takesPreedit implements editHost: the field takes IME compositions
// while enabled, editable and not secure.
func (e *textFieldView) takesPreedit() bool {
	return e.EnabledInTree() && !e.p.ReadOnly && !e.p.Secure
}

// secureText, readOnlyText and preeditChanged implement editHost (the
// preedit is spliced in by textLayout, keyed by it).
func (e *textFieldView) secureText() bool   { return e.p.Secure }
func (e *textFieldView) readOnlyText() bool { return e.p.ReadOnly }
func (e *textFieldView) preeditChanged()    {}

// edited implements editHost: it repaints after a command, restarts the
// caret blink and, when the text changed since rev, mirrors it to the stub
// and calls TextChanged.
// Nothing of the field is used after the handler, so a SetText from
// inside it (the converter case) wins and fires nothing.
func (e *textFieldView) edited(rev uint64) {
	e.Invalidate()
	e.restartBlink()
	if e.ed.Buffer().Revision() == rev {
		return
	}
	t := e.p.TextField
	t.Value = e.text()
	if t.Handler != nil {
		t.Handler.TextChanged(t.Value)
	}
}

// Paint draws the bezel, the selection, the text or the placeholder, the
// caret and the focus ring (nothing for an empty frame).
func (e *textFieldView) Paint(c render.Canvas) {
	r, radius := e.bezel()
	if e.Bounds().Empty() || r.Empty() {
		return
	}
	a := appearanceIn(&e.Base, e.d)
	tf := &tokens.TextField
	cs := e.p.ControlSize
	radii := render.UniformRadii(radius)
	scale := scaleIn(&e.Base)
	bw := pixelWidth(tf.FieldBorderWidth, scale)

	c.FillRoundedRect(r, radii, render.Solid(semantic(driver.ColorTextBackground, a)))
	c.DrawShadow(r, radii, render.Shadow{
		Offset: driver.Point{Y: tf.FieldInsetShadowOffsetY}, Blur: tf.FieldInsetShadowBlur,
		Color: tf.FieldInsetShadow.Value(a), Inset: true,
	})
	if r.W > 2*bw && r.H > 2*bw {
		strokeInside(c, r, radius, bw, tf.FieldBorder.Value(a))
		inner := r.Inset(driver.UniformInsets(bw))
		pad := tf.FieldPaddingX.At(cs)
		if room := r.W - 2*pad; room > 0 {
			c.Save()
			c.ClipRoundedRect(inner, render.UniformRadii(max(0, radius-bw)))
			e.paintText(c, r, pad, room, a)
			c.Restore()
		}
	}
	e.ring.Paint(c, e, RectFocusShape(r, radii))
}

// paintText draws the selection, the text (or the placeholder) and the
// caret inside the bezel r, whose text area starts pad in and is room wide.
func (e *textFieldView) paintText(c render.Canvas, r driver.Rect, pad, room float64, a driver.Appearance) {
	focus := e.FocusState()
	disabled := !e.EnabledInTree()
	l := e.textLayout()
	if e.ed.Len() == 0 && !e.composing() {
		if ph := e.placeholderLayout(); ph != nil {
			if ph.Size().W > room {
				ph = e.phTrunc.truncate(e.d.text, e.p.Placeholder, e.p.TextFont(), room)
			}
			if ph != nil {
				x := r.X + pad + e.alignOffset(room-ph.Size().W)
				c.DrawText(ph, driver.Point{X: x, Y: r.Y + (r.H-ph.Size().H)/2},
					semantic(driver.ColorPlaceholderText, a))
			}
		}
	}

	start, end := e.ed.Selection()
	e.updateScroll(l, room)
	origin := e.origin(l, r, pad, room)

	if focus != Unfocused && start != end {
		col := tokens.TextSelection(a, true, focus == FocusedActive)
		for _, sr := range l.SelectionRects(e.shownOffset(start), e.shownOffset(end)) {
			c.FillRoundedRect(sr.Offset(origin), render.Radii{}, render.Solid(col))
		}
	}
	if e.ed.Len() > 0 || e.composing() {
		col := semantic(driver.ColorText, a)
		if disabled {
			col = semantic(driver.ColorDisabledControlText, a)
		}
		// ColorSelectedText is ColorText in the theme, so the selected
		// text needs no second pass.
		c.DrawText(l, origin, col)
		e.paintPreedit(c, l, origin, col)
	}
	if e.CaretVisible() {
		c.FillRoundedRect(pixelAlign(&e.Base, e.caretRect(l, origin), scaleIn(&e.Base)), render.Radii{},
			render.Solid(tokens.Caret(a)))
	}
}

// caretRect returns the caret of the layout l drawn at origin, before
// pixel alignment: CaretWidth wide, centred on the caret offset's edge.
func (e *textFieldView) caretRect(l render.TextLayout, origin driver.Point) driver.Rect {
	cr := l.CaretRect(e.caretOffset())
	w := tokens.TextField.CaretWidth
	return driver.Rect{X: origin.X + cr.X - w/2, Y: origin.Y + cr.Y, W: w, H: cr.H}
}

var _ TextInputClient = (*textFieldView)(nil)

// TextInput implements TextInputClient: on while the field takes preedits
// (enabled, editable, not secure), with the caret as Paint places it by
// the scroll of the last paint (the selection's head, the preedit's
// cursor while composing), shown or not.
func (e *textFieldView) TextInput() (bool, driver.Rect) {
	if !e.takesPreedit() {
		return false, driver.Rect{}
	}
	r, pad, room := e.textArea()
	l := e.textLayout()
	return true, e.caretRect(l, e.origin(l, r, pad, room))
}

// The preedit's clause underlines: their thickness (whole device pixels,
// at least one), how far below the baseline they start, and the gap left
// at the end of each clause so that neighbours stay apart.
const (
	preeditUnderline         = 1.0
	preeditUnderlineSelected = 2.0
	preeditUnderlineOffset   = 1.0
	preeditClauseGap         = 1.0
)

// paintPreedit underlines the clauses of the shown preedit (the whole
// preedit when the IME reported none) in the layout l drawn at origin, in
// col: preeditUnderline thick, preeditUnderlineSelected for the clause
// being converted.
func (e *textFieldView) paintPreedit(c render.Canvas, l render.TextLayout, origin driver.Point, col driver.Color) {
	if !e.composing() {
		return
	}
	paintPreeditUnderlines(c, &e.Base, l, origin, e.ed.Head(), e.pre, col)
}

// paintPreeditUnderlines underlines the clauses of the preedit pre (the
// whole preedit when the IME reported none), spliced in at the byte at of
// the layout l drawn at origin in b, in col: preeditUnderline thick,
// preeditUnderlineSelected for the clause being converted, below the
// baseline of each line a clause runs over (TextField, TextView).
func paintPreeditUnderlines(c render.Canvas, b *Base, l render.TextLayout, origin driver.Point, at int, pre textPreedit, col driver.Color) {
	segs := pre.segs
	if len(segs) == 0 {
		segs = []shell.PreeditSegment{{Start: 0, End: len(pre.text)}}
	}
	scale := scaleIn(b)
	for _, sg := range segs {
		th := preeditUnderline
		if sg.Selected {
			th = preeditUnderlineSelected
		}
		th = pixelWidth(th, scale)
		for _, sr := range l.SelectionRects(at+sg.Start, at+sg.End) {
			w := sr.W
			if w > 2*preeditClauseGap {
				w -= preeditClauseGap
			}
			m := l.Line(lineAtY(l, sr.Y+sr.H/2))
			u := pixelAlign(b, driver.Rect{
				X: origin.X + sr.X, Y: origin.Y + sr.Y + m.Baseline - m.Top + preeditUnderlineOffset, W: w, H: th,
			}, scale)
			u.H = th
			c.FillRoundedRect(u, render.Radii{}, render.Solid(col))
		}
	}
}

// textArea returns the bezel r, the text area's padding from either edge
// and its width (≤ 0: no room for text).
func (e *textFieldView) textArea() (r driver.Rect, pad, room float64) {
	r, _ = e.bezel()
	pad = tokens.TextField.FieldPaddingX.At(e.p.ControlSize)
	return r, pad, r.W - 2*pad
}

// updateScroll sets the horizontal scroll for the layout l in a text
// area room wide: none while unfocused or when the text fits, else the
// least change that keeps the caret (the selection's head) in view,
// never past the end of the text.
func (e *textFieldView) updateScroll(l render.TextLayout, room float64) {
	size := l.Size()
	if e.FocusState() == Unfocused || size.W <= room {
		e.scroll = 0
		return
	}
	cx := l.CaretRect(e.caretOffset()).X
	e.scroll = min(max(e.scroll, cx-room), cx)
	e.scroll = min(max(e.scroll, 0), size.W-room)
}

// origin returns where the layout l is drawn in the bezel r, by the
// current scroll and the alignment.
func (e *textFieldView) origin(l render.TextLayout, r driver.Rect, pad, room float64) driver.Point {
	size := l.Size()
	o := driver.Point{X: r.X + pad - e.scroll, Y: r.Y + (r.H-size.H)/2}
	if size.W <= room {
		o.X = r.X + pad + e.alignOffset(room-size.W)
	}
	return o
}

// hitAt implements editHost: the editor offset nearest to the local point
// p, as the text is shown (the scroll as last placed), never upstream (one
// line): the cluster boundary the caret goes to for a click there. Points
// left or right of the text area hit the clusters scrolled out on that
// side, so a drag past either edge moves the head there and selected
// scrolls the text to show it, by pointer moves only (no timer).
func (e *textFieldView) hitAt(p driver.Point) (int, bool) {
	r, pad, room := e.textArea()
	l := e.textLayout()
	o := e.origin(l, r, pad, room)
	i, trailing := l.HitTest(driver.Point{X: p.X - o.X, Y: p.Y - o.Y})
	if trailing {
		i = text.NextGrapheme(l.Text(), i)
	}
	return e.editorOffset(i), false
}

// editorOffset maps an offset of the shown text back to the editor's
// text (shownOffset's inverse): the same offset, or in a secure field the
// start of the cluster whose bullet starts there.
func (e *textFieldView) editorOffset(shown int) int {
	if !e.p.Secure {
		return shown
	}
	if cs, n := e.clusters(), shown/len(bullet); n < len(cs) {
		return cs[n]
	}
	return len(e.text())
}

// selected implements editHost: it scrolls the new head into view,
// restarts the caret blink and repaints after the mouse changed the
// selection (no TextChanged: the text did not change).
func (e *textFieldView) selected() {
	_, _, room := e.textArea()
	if room > 0 {
		e.updateScroll(e.textLayout(), room)
	}
	e.restartBlink()
	e.Invalidate()
}

// autoScroll implements editHost: a field scrolls by pointer moves only.
func (e *textFieldView) autoScroll(bool) {}

// alignOffset returns the x offset of text with free room left over in
// the text area, by the field's alignment.
func (e *textFieldView) alignOffset(free float64) float64 {
	switch e.p.Alignment {
	case driver.AlignCenter:
		return max(0, free/2)
	case driver.AlignTrailing:
		return max(0, free)
	}
	return 0
}

// AccessibleNode implements Element: a text field with the placeholder as
// label and the text as value (none for a secure field).
func (e *textFieldView) AccessibleNode() a11y.Node {
	n := a11y.Node{Role: a11y.RoleTextField, Label: e.p.Placeholder}
	if !e.p.Secure {
		n.Value = e.text()
	}
	if e.p.ReadOnly {
		n.States = a11y.StateReadOnly
	}
	return n
}
