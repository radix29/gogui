package aqua

import (
	"time"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/aqua/text"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/shell"
)

// TextView is the aqua driver.TextView (PLAN.md S5.4): a real element over
// a multi-line text.Editor, as an NSTextView in a borderless NSScrollView.
// The embedded stub text view keeps the font, the text color, the
// read-only flag and the handler; the text and the selection live in the
// editor only (Text and Selection read it, no copy per edit), and
// FittingSize stays the stub's, so the example frames do not move.
// S5.4f draws it, S5.4g edits it from the keyboard, S5.4h adds the
// clipboard, the caret blink and the idle layout pass, S5.4i the mouse,
// S5.4j the IME.
//
//   - Elements: the TextView's element (textViewElem, focused, the
//     accessible node) holds an internal aqua ScrollView over its whole
//     frame, with a vertical scroller only (the text wraps), whose
//     document is one textDoc element: the viewport's width, and as tall
//     as the text (at least the viewport). The scroll view brings the
//     physics, the overlay and legacy scrollers and the scroll events;
//     its scroll area is not exposed to accessibility.
//
//   - Geometry: the text container is inset by the TextView tokens:
//     ContainerInsetY above the first line and below the last,
//     LineFragmentPadding left and right of every line, so the text wraps
//     at the viewport width minus twice the padding (legacy scrollers take
//     their track from it). Paragraph i is drawn at y = ContainerInsetY +
//     its top in the paragraph index.
//
//   - Layout: one render.TextLayout per paragraph in a paraLayouts cache
//     (S5.4e), made only for the paragraphs that show and those an
//     operation needs; the other heights are estimates. Every change of
//     the frame, the viewport, the offset (scrolling, momentum, rubber
//     band), the text or the font settles the view: the paragraphs in the
//     viewport are laid out (their heights become exact) and the
//     document's height follows the index, until nothing changes, so a
//     load or a resize of a 1 MB text lays out only what shows. Paint
//     settles too, as a fallback.
//
//   - Idle layout pass (S5.4h): while a height is an estimate and the view
//     shows in an open window, a timer (Shell.After, idleLayoutDelay)
//     measures the next estimated paragraphs, about idleLayoutBytes of
//     text per tick and for at most idleLayoutTime on the shell's clock
//     (at least one paragraph; S5.4k), from where the last tick stopped and around to the
//     start (paraLayouts.Measure: no layout is cached, so the visible ones
//     stay), and keeps the visual line at the top of the viewport where it
//     is, as a wrap change does: the visible lines never move, only the
//     scroller's knob. A tick while the offset moves (a gesture, momentum,
//     a wheel step, the rubber band) or a scroller is pressed measures
//     nothing and waits for the next. Once every height is exact no timer
//     is left; an edit, a wrap width or a font change that makes estimates
//     again restarts it. A hidden view or a closed window stops it (a
//     paint starts it again).
//
//   - Scroll anchoring: a wrap width or font change keeps the visual line
//     at the top of the viewport where it is (its first character and the
//     distance of the viewport's top below the line's), so the content
//     does not jump while every other height becomes an estimate again.
//
//   - Painting: ColorTextBackground over the frame (pixel-aligned), then
//     only the paragraphs that overlap the viewport, found in the index:
//     the selection (Theme.TextSelection, emphasized only while focused in
//     the key window; a selected newline runs to the container's right
//     edge, as NSTextView), the text in TextColor (ColorText when zero;
//     ColorDisabledControlText when disabled) and the caret (CaretWidth
//     wide, the line high, in Theme.Caret, pixel-aligned, upstream at a
//     soft break when the editor says so) when the selection is empty, the
//     view holds the focus in the key window and is enabled and editable.
//     Unlike a text field, an unfocused text view still shows its
//     selection, unemphasized.
//
//   - Focus: FocusText; the focus ring goes around the frame (the scroll
//     view's), drawn by the element before its children (it lies outside
//     the frame).
//
//   - SetText replaces the text (newlines normalized), puts the caret at
//     its end, clears the undo history and scrolls to the top; no event.
//     SetSelection selects [start, end) (swapped when start > end, each
//     clamped and moved back to its cluster's start) and scrolls the
//     range into view by the least amount (its start when it is taller
//     than the viewport).
//
//   - Keyboard (S5.4g): a key-down or repeat bound by text.Lookup in
//     multi-line mode runs through Editor.Do with the paragraph layouts
//     as text.Lines (moves by every unit across paragraphs and wrapped
//     lines, the deletes, SelectAll, Undo, Redo) and is taken. Return and
//     Shift-Return insert a newline (no default button), Tab inserts a
//     tab, as NSTextView; Shift-Tab is not taken (the router moves the
//     focus back), and Ctrl-Tab and Ctrl-Shift-Tab move the focus
//     forward and back, out of the text. A TextEvent (Space and the
//     characters come back as one) replaces the selection, newlines kept.
//     Page Up and Page Down move the caret by the viewport's lines
//     (paraLayouts.PageLines) and scroll by as much as the caret moved, so
//     it stays where it was in the viewport (as far as the document
//     allows), as NSTextView pages. The internal scroll view never sees a
//     key (it is inside the focused element), so Space types and does not
//     page.
//
//   - Clipboard (S5.4h), through the glue shared with TextField
//     (textEdit): Copy puts a non-empty selection on the shell's
//     clipboard, Cut does the same and deletes it, Paste reads the
//     clipboard (always later) and replaces the selection with its text,
//     newlines kept (line breaks normalized), as one undo group; each
//     scrolls the caret into view when the text changed. A paste that
//     arrives after the view lost the focus (even if it got it back),
//     became disabled or read-only, or its window closed is dropped, as is
//     an empty one. Read-only: Copy works, Cut and Paste are taken and do
//     nothing.
//
//   - Caret blink (S5.4h): TextField's rules (textEdit), by the window's
//     Appearance.CaretBlink over Shell.After, one timer per half period
//     and only while the caret is shown; every key, typed text, paste,
//     SetText, SetSelection, SetEditable, focus and window-state change
//     shows the caret and restarts the blink.
//
//   - Caret into view: every key, typed text and UserType scrolls the
//     caret (the selection's head, upstream at a soft break) into view by
//     the least amount, the container inset included on the first and
//     the last line; a key that changes nothing too. Select All (its key
//     and the Edit menu's) scrolls nothing, as in NSTextView (review B5).
//
//   - Mouse (S5.4i): a primary press focuses the view (keeping its
//     selection) and captures the pointer; it places the caret at the
//     nearest cluster boundary on the visual line under the pointer
//     (upstream right of a soft-wrapped line; the first line in the top
//     inset, the text's end below the last line), Shift extends the
//     selection to it as AppKit does (the far end stays), a double-click
//     selects the word there and a triple-click the paragraph with its
//     newline (SelectParagraph, as NSTextView). A drag extends the
//     selection by the press's unit, by clusters from the anchor or by
//     whole words or paragraphs keeping the first one. Held past the
//     viewport's top or bottom, the text auto-scrolls on the frame clock
//     (an anim.Ticker, so Reduce Motion does not stop it), even without
//     pointer moves, at autoScrollMin pt/s at the edge plus autoScrollGain
//     per point further out (at most autoScrollMax), and the selection
//     follows to the text under the pointer; it stops back inside, at the
//     document's end, at the release, a capture cancel, a focus loss, and
//     when the view is disabled or no longer shows, and runs no frame
//     then. A click scrolls nothing. Mouse selection works read-only too
//     and fires no event. The cursor is the I-beam over the text of an
//     enabled view (textDoc.Cursor).
//
//   - IME (S5.4j): TextField's rules (textEdit). An enabled, editable
//     view takes the PreeditEvents of the IME's composition; the first one
//     deletes a non-empty selection (an undoable edit with TextChanged).
//     The preedit is spliced into its paragraph's layout only
//     (paraLayouts.SetPreedit: a second layout of that paragraph, the
//     index holding its height, so a preedit that wraps the paragraph onto
//     more lines moves the text below it), its clauses underlined in the
//     text color below the
//     baseline of each line they run over, the caret at the preedit's
//     cursor, solid and scrolled into view. An empty preedit ends the
//     composition and leaves the text as it was; the commit (a TextEvent
//     during the composition) replaces the preedit as an undo group of its
//     own with TextChanged. A blur, a primary press, the window losing key,
//     a key, a paste arriving during the composition, SetSelection and
//     SetEditable(false) commit the preedit as the IME would and cancel
//     the surface's composition; SetText and UserType drop it and cancel.
//     Read-only and disabled views take no preedits. Text, the a11y value
//     and the moves never see the preedit. The view is a TextInputClient:
//     its window turns the IME on while it takes preedits and holds the
//     focus in the key window, with the caret rect as painted (by the
//     scroll offset, at the preedit's cursor while composing).
//
//   - Read-only: the moves and SelectAll work; the edits and typed text
//     are taken and change nothing, but Return and Tab are not taken (they
//     reach the default button and the focus). Disabled: the router sends
//     it nothing.
//
//   - Events: TextChanged after every key or text event that changed the
//     text (and UserType), never SetText; a SetText inside it wins.
//
//   - Accessibility: a text area with the text as value, read-only when
//     not editable; the scroll view inside is not exposed.
//
// UserType simulates the user replacing the selection.
type TextView struct {
	*stub.TextView
	peerBase
	view  *textViewElem
	sv    *ScrollView
	doc   *textDoc
	lines *paraLayouts // made on first use (the text system needs the running shell)

	settling bool // in settle, or in a change that settles at its end

	idle     func()               // the idle layout pass's pending timer's cancel, or nil
	idleFrom int                  // the paragraph the next idle tick starts looking from
	idleNow  func() time.Duration // the idle pass's clock; nil: the shell's (tests)

	auto *anim.Ticker // the drag's auto-scroll, made on first use

	reveals int // revealCaret runs, for the tests
}

// The idle layout pass (see TextView): the time between two ticks and the
// text a tick measures, in bytes, each paragraph counting
// idleLayoutParaCost more, and the time on the shell's clock after which
// a tick stops early (S5.4k: DirectWrite measures about 1 µs a byte, so
// 16 KiB would hold the UI thread for most of a frame).
const (
	idleLayoutDelay    = 20 * time.Millisecond
	idleLayoutBytes    = 16 << 10
	idleLayoutParaCost = 64
	idleLayoutTime     = 4 * time.Millisecond
)

// NewTextView returns a text view.
func (d *Driver) NewTextView(h driver.TextViewHandler) driver.TextView {
	t := &TextView{TextView: stub.NewTextView(h)}
	t.view = &textViewElem{p: t}
	t.view.initEdit(d, t.view, text.NewEditor("", false))
	t.view.Init(t.view)
	t.view.SetFocusable(true)
	t.view.SetFocusKind(FocusText) // Tab reaches it in both keyboard-access modes
	t.doc = &textDoc{p: t}
	t.doc.Init(t.doc)

	t.sv = d.NewScrollView(nil).(*ScrollView)
	t.sv.inner = true
	t.sv.SetHasVerticalScroller(true)
	t.sv.document.SetChildren(t.doc)
	t.view.AddChild(t.sv.view)

	t.sv.placed = t.settle
	t.initElement(t.TextView, &t.TextView.Base, t.view)
	return t
}

// SetFrame implements driver.Widget: the internal scroll view fills the
// frame, and the text is wrapped to its viewport (scroll-anchored).
func (t *TextView) SetFrame(r driver.Rect) {
	t.peerBase.SetFrame(r)
	t.sv.SetFrame(driver.Rect{W: r.W, H: r.H})
	t.settle()
}

// SetText implements driver.TextView (see TextView).
func (t *TextView) SetText(s string) {
	t.view.dropPreedit()
	ed := t.view.ed
	ed.SetText(s)
	n := ed.Len()
	ed.SetSelection(n, n)
	t.sv.SetOffset(driver.Point{})
	t.settle()
	t.view.restartBlink()
	t.redraw()
}

// Text implements driver.TextView: the editor's text.
func (t *TextView) Text() string { return t.view.ed.Text() }

// Selection implements driver.TextView: the editor's selection.
func (t *TextView) Selection() (start, end int) { return t.view.ed.Selection() }

// SetSelection implements driver.TextView (see TextView); a composition is
// committed first.
func (t *TextView) SetSelection(start, end int) {
	t.view.commitPreedit()
	if start > end {
		start, end = end, start
	}
	t.view.ed.SetSelection(start, end)
	t.scrollRangeToVisible(t.view.ed.Selection())
	t.view.restartBlink()
	t.redraw()
}

// SetFont implements driver.TextView: the text is laid out again in the
// font (scroll-anchored).
func (t *TextView) SetFont(f driver.Font) {
	t.TextView.SetFont(f)
	if tf := t.TextFont(); t.lines != nil && tf != t.lines.Font() {
		t.settling = true
		a := t.anchor()
		t.lines.SetFont(tf)
		t.restore(a)
		t.settling = false
	}
	t.settle()
	t.redraw()
}

func (t *TextView) SetTextColor(c driver.Color) { t.TextView.SetTextColor(c); t.redraw() }
func (t *TextView) SetEditable(editable bool) {
	if !editable {
		t.view.commitPreedit()
	}
	t.TextView.SetEditable(editable)
	t.view.restartBlink()
	t.redraw()
}

// UserType simulates the user replacing the selection with s (refused
// while disabled or read-only; a composition is dropped), with
// TextChanged, and repaints.
func (t *TextView) UserType(s string) {
	if t.Disabled || t.ReadOnly {
		return
	}
	t.view.dropPreedit()
	rev := t.view.ed.Buffer().Revision()
	t.view.ed.Insert(s)
	t.view.edited(rev)
}

// textLines returns the paragraph layouts as text.Lines, or nil before the
// view first showed (each paragraph is then one line).
func (t *TextView) textLines() text.Lines {
	if t.lines == nil {
		return nil
	}
	return t.lines
}

// isPage reports whether cmd is a Page Up or Page Down move.
func isPage(cmd text.Command) bool {
	switch cmd {
	case text.CmdPageUp, text.CmdSelectPageUp, text.CmdPageDown, text.CmdSelectPageDown:
		return true
	}
	return false
}

// do runs the bound command cmd (see TextView): the editor's Do with the
// layouts, the page scroll for a page move, the caret into view once (by
// edited after an edit, here after a move; never for Select All, which
// scrolls nothing in NSTextView, review B5) and TextChanged when the text
// changed.
func (t *TextView) do(cmd text.Command) {
	ed := t.view.ed
	rev := ed.Buffer().Revision()
	_, _, shown := t.visible()
	page := isPage(cmd) && t.lines != nil && shown
	var at float64 // the caret's top in the viewport before a page move
	if page {
		top, _ := t.caretSpan(ed.Head(), ed.Upstream(), false)
		at = top - t.sv.y.Clamped()
	}
	ed.Do(cmd, t.textLines())
	if page {
		t.settle()
		top, _ := t.caretSpan(ed.Head(), ed.Upstream(), false)
		t.settling = true
		t.syncDocument()
		t.sv.SetOffset(driver.Point{Y: top - at})
		t.settling = false
	}
	if ed.Buffer().Revision() == rev && cmd != text.CmdSelectAll {
		t.revealCaret() // else edited does it
	}
	t.view.edited(rev)
}

// revealCaret settles the layout and scrolls the caret into view (see
// TextView).
func (t *TextView) revealCaret() {
	t.reveals++
	t.settle()
	ed := t.view.ed
	t.scrollSpanToVisible(func() (float64, float64) { return t.caretSpan(ed.Head(), ed.Upstream(), true) })
}

// Editor returns the view's editor (tests and the editing steps).
func (t *TextView) Editor() *text.Editor { return t.view.ed }

// Offset returns the internal scroll view's offset (tests).
func (t *TextView) Offset() driver.Point { return t.sv.Offset() }

// TextFont returns the font the view draws with: its Font with a zero
// Size replaced by the theme's system size (regular controls).
func (t *TextView) TextFont() driver.Font { return sizedFont(t.Font, driver.ControlSizeRegular) }

// textColor returns the color of the text in a.
func (t *TextView) textColor(a driver.Appearance) driver.Color {
	if !t.view.EnabledInTree() {
		return semantic(driver.ColorDisabledControlText, a)
	}
	c := t.TextColor
	if c == (driver.Color{}) {
		c = driver.Semantic(driver.ColorText)
	}
	return resolveColor(c, a)
}

// textViewElem is the element of a TextView: the focus, the background,
// the focus ring and the accessible node; the internal scroll view is its
// only child.
type textViewElem struct {
	Base
	textEdit // the editor, events, ring, blink, clipboard, composition and drag (textedit.go)
	p        *TextView
}

// FittingSize returns the peer's FittingSize.
func (e *textViewElem) FittingSize(max driver.Size) driver.Size { return e.p.FittingSize(max) }

// HandleEvent is the shared text element's (textEdit.handleEvent; see
// TextView).
func (e *textViewElem) HandleEvent(ev Event) bool { return e.handleEvent(ev) }

// AccessibleNode implements Element: a text area with the text as value.
func (e *textViewElem) AccessibleNode() a11y.Node {
	n := a11y.Node{Role: a11y.RoleTextArea, Value: e.ed.Text()}
	if e.p.ReadOnly {
		n.States = a11y.StateReadOnly
	}
	return n
}

// takesPreedit implements editHost: the view takes IME compositions while
// enabled and editable.
func (e *textViewElem) takesPreedit() bool { return e.EnabledInTree() && !e.p.ReadOnly }

// preeditChanged implements editHost: the composition is spliced into its
// paragraph's layout (settle) and, while it lasts, its cursor scrolled
// into view.
func (e *textViewElem) preeditChanged() {
	if e.composing() {
		e.p.revealCaret()
	} else {
		e.p.settle()
	}
}

var _ TextInputClient = (*textViewElem)(nil)

// TextInput implements TextInputClient: on while the view takes preedits
// (enabled, editable), with the caret as paintText places it by the
// current scroll offset (the selection's head, the preedit's cursor while
// composing), shown or not; before the view first showed, a caret-wide
// rect at the text's origin.
func (e *textViewElem) TextInput() (bool, driver.Rect) {
	if !e.takesPreedit() {
		return false, driver.Rect{}
	}
	t := e.p
	var r driver.Rect
	if t.lines == nil {
		w := tokens.TextField.CaretWidth
		r = driver.Rect{X: tokens.TextView.LineFragmentPadding - w/2, Y: tokens.TextView.ContainerInsetY, W: w}
	} else {
		r = t.caretRect()
	}
	o := FromRoot(e, ToRoot(t.doc, r.Origin()))
	r.X, r.Y = o.X, o.Y
	return true, r
}

// secureText and readOnlyText implement editHost.
func (e *textViewElem) secureText() bool   { return false }
func (e *textViewElem) readOnlyText() bool { return e.p.ReadOnly }

// command implements editHost: TextView.do (the layouts, the caret into
// view).
func (e *textViewElem) command(cmd text.Command) { e.p.do(cmd) }

// edited implements editHost: it repaints, restarts the caret blink and,
// when the text changed since rev, scrolls the caret into view and calls
// TextChanged. Nothing of the view is used after the handler, so a SetText
// from inside it wins.
func (e *textViewElem) edited(rev uint64) {
	e.Invalidate()
	e.restartBlink()
	if e.ed.Buffer().Revision() == rev {
		return
	}
	e.p.revealCaret()
	if h := e.p.Handler; h != nil {
		h.TextChanged()
	}
}

// textDoc is the document of a TextView's scroll view: it paints the
// visible text (TextView.paintText).
type textDoc struct {
	Base
	p *TextView
}

// Cursor implements Element: the I-beam over the text of an enabled view
// (editable or read-only), else the arrow.
func (e *textDoc) Cursor() shell.Cursor { return e.p.view.textCursor() }
