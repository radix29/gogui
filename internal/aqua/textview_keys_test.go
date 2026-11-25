package aqua

import (
	"fmt"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua/text"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// Keyboard editing of the TextView (PLAN.md S5.4g), through the fake
// shell's key and text events. The view's viewport is 180×80: 5 lines of
// 16 pt, wrapping after 24 fake runes (7 pt each).

// focusedView is viewWindow with the view focused.
func focusedView(t *testing.T, h driver.TextViewHandler) (*TextView, *Window, *fake.Surface, *fake.Shell) {
	t.Helper()
	v, w, sf, s := viewWindow(t, h)
	if !v.view.Focus() {
		t.Fatal("view not focused")
	}
	return v, w, sf, s
}

// viewSel returns the editor's text and selection as "text|anchor,head".
func viewSel(v *TextView) string {
	ed := v.Editor()
	return fmt.Sprintf("%s|%d,%d", ed.Text(), ed.Anchor(), ed.Head())
}

// TestTextViewKeys: every key of the multi-line map, with and without
// Shift, across paragraphs.
func TestTextViewKeys(t *testing.T) {
	v, _, sf, _ := focusedView(t, nil)
	// "one two" 0–7, "three four" 8–18, "five" 19–23; the caret at 10
	// (before the 'r' of "three", x 14) unless the case says otherwise.
	const s = "one two\nthree four\nfive"
	for _, c := range []struct {
		anchor, head int
		key          driver.Key
		mods         driver.Modifiers
		want         string
	}{
		{10, 10, driver.KeyLeft, 0, s + "|9,9"},
		{10, 10, driver.KeyRight, 0, s + "|11,11"},
		{10, 10, driver.KeyLeft, shf, s + "|10,9"},
		{10, 10, driver.KeyRight, shf, s + "|10,11"},
		{8, 8, driver.KeyLeft, 0, s + "|7,7"}, // over the newline
		{7, 7, driver.KeyRight, 0, s + "|8,8"},
		{8, 13, driver.KeyLeft, 0, s + "|8,8"}, // a selection collapses
		{8, 13, driver.KeyRight, 0, s + "|13,13"},
		{10, 10, driver.KeyLeft, alt, s + "|8,8"},
		{10, 10, driver.KeyRight, alt, s + "|13,13"},
		{8, 8, driver.KeyLeft, alt, s + "|4,4"}, // to the word in the paragraph before
		{10, 10, driver.KeyLeft, alt | shf, s + "|10,8"},
		{10, 10, driver.KeyRight, alt | shf, s + "|10,13"},
		{10, 10, driver.KeyLeft, ctrl, s + "|8,8"},
		{10, 10, driver.KeyRight, ctrl, s + "|18,18"},
		{10, 10, driver.KeyLeft, ctrl | shf, s + "|10,8"},
		{10, 10, driver.KeyRight, ctrl | shf, s + "|10,18"},
		{10, 10, driver.KeyUp, 0, s + "|2,2"}, // the goal x into the paragraph above
		{10, 10, driver.KeyDown, 0, s + "|21,21"},
		{10, 10, driver.KeyUp, shf, s + "|10,2"},
		{10, 10, driver.KeyDown, shf, s + "|10,21"},
		{2, 2, driver.KeyUp, 0, s + "|0,0"}, // the first line: to the start
		{21, 21, driver.KeyDown, 0, s + "|23,23"},
		{10, 10, driver.KeyUp, alt, s + "|8,8"},
		{10, 10, driver.KeyDown, alt, s + "|18,18"},
		{8, 8, driver.KeyUp, alt, s + "|0,0"},
		{18, 18, driver.KeyDown, alt, s + "|23,23"},
		{10, 10, driver.KeyUp, alt | shf, s + "|10,8"},
		{10, 10, driver.KeyDown, alt | shf, s + "|10,18"},
		{10, 10, driver.KeyUp, ctrl, s + "|0,0"},
		{10, 10, driver.KeyDown, ctrl, s + "|23,23"},
		{10, 10, driver.KeyUp, ctrl | shf, s + "|10,0"},
		{10, 10, driver.KeyDown, ctrl | shf, s + "|10,23"},
		{10, 10, driver.KeyHome, 0, s + "|0,0"},
		{10, 10, driver.KeyEnd, 0, s + "|23,23"},
		{10, 10, driver.KeyHome, shf, s + "|10,0"},
		{10, 10, driver.KeyEnd, shf, s + "|10,23"},
		{10, 10, driver.KeyPageUp, 0, s + "|2,2"}, // a page past the ends: the first or last line at the goal x
		{10, 10, driver.KeyPageDown, 0, s + "|21,21"},
		{10, 10, driver.KeyPageUp, shf, s + "|10,2"},
		{10, 10, driver.KeyPageDown, shf, s + "|10,21"},
		{10, 10, driver.KeyPageUp, alt, s + "|2,2"},
		{10, 10, driver.KeyPageDown, alt | shf, s + "|10,21"},
		{10, 10, driver.KeyBackspace, 0, "one two\ntree four\nfive|9,9"},
		{10, 10, driver.KeyBackspace, shf, "one two\ntree four\nfive|9,9"},
		{8, 8, driver.KeyBackspace, 0, "one twothree four\nfive|7,7"}, // joins the paragraphs
		{10, 10, driver.KeyDelete, 0, "one two\nthee four\nfive|10,10"},
		{18, 18, driver.KeyDelete, 0, "one two\nthree fourfive|18,18"},
		{10, 10, driver.KeyBackspace, alt, "one two\nree four\nfive|8,8"},
		{10, 10, driver.KeyDelete, alt, "one two\nth four\nfive|10,10"},
		{10, 10, driver.KeyBackspace, ctrl, "one two\nree four\nfive|8,8"},
		{10, 10, driver.KeyDelete, ctrl, "one two\nth\nfive|10,10"},
		{10, 13, driver.KeyBackspace, 0, "one two\nth four\nfive|10,10"},
		{10, 10, driver.KeyReturn, 0, "one two\nth\nree four\nfive|11,11"},
		{10, 10, driver.KeyReturn, shf, "one two\nth\nree four\nfive|11,11"},
		{10, 10, driver.KeyTab, 0, "one two\nth\tree four\nfive|11,11"},
		{4, 15, driver.KeyReturn, 0, "one \nour\nfive|5,5"},
		{2, 9, 'a', ctrl, s + "|0,23"},
	} {
		v.SetText(s)
		v.Editor().SetSelection(c.anchor, c.head)
		if !sf.Key(c.key, c.mods) {
			t.Errorf("%v %v from %d,%d not taken", c.key, c.mods, c.anchor, c.head)
		}
		if got := viewSel(v); got != c.want {
			t.Errorf("%v %v from %d,%d: %q, want %q", c.key, c.mods, c.anchor, c.head, got, c.want)
		}
	}
	// A repeat moves like a press.
	v.SetText(s)
	v.Editor().SetSelection(10, 10)
	if !sf.KeyRepeat(driver.KeyDown, 0) || viewSel(v) != s+"|21,21" {
		t.Errorf("repeat: %q", viewSel(v))
	}
	// Undo and Redo.
	v.SetText(s)
	v.Editor().SetSelection(10, 10)
	sf.Key(driver.KeyReturn, 0)
	sf.Key('z', ctrl)
	if got := viewSel(v); got != s+"|10,10" {
		t.Errorf("undo: %q", got)
	}
	sf.Key('z', ctrl|shf)
	if got := v.Text(); got != "one two\nth\nree four\nfive" {
		t.Errorf("redo: %q", got)
	}
}

// TestTextViewTyping: typed text replaces the selection, Space types,
// newlines in a text event are kept, and TextChanged fires once per edit
// (never for a move or SetText).
func TestTextViewTyping(t *testing.T) {
	h := &viewLog{}
	v, _, sf, _ := focusedView(t, h)
	sf.Type("ab c\nd\te")
	if got := viewSel(v); got != "ab c\nd\te|8,8" || h.changed != 8 {
		t.Errorf("typed %q, %d events", got, h.changed)
	}
	v.Editor().SetSelection(1, 3)
	sf.Commit("XY")
	if got := viewSel(v); got != "aXYc\nd\te|3,3" || h.changed != 9 {
		t.Errorf("commit over a selection: %q, %d events", got, h.changed)
	}
	sf.Send(shell.TextEvent{Text: "1\r\n2"})
	if got := v.Text(); got != "aXY1\n2c\nd\te" || h.changed != 10 {
		t.Errorf("newline in a text event: %q, %d events", got, h.changed)
	}
	sf.Key(driver.KeyLeft, 0)
	sf.Key('z', ctrl|shf) // nothing to redo
	v.SetText("new")
	if h.changed != 10 {
		t.Errorf("a move, an empty redo or SetText fired: %d events", h.changed)
	}
	// A SetText inside TextChanged wins.
	r := &resetView{}
	v2, _, sf2, _ := focusedView(t, r)
	r.v = v2
	sf2.Type("q")
	if v2.Text() != "reset" {
		t.Errorf("SetText inside TextChanged: %q", v2.Text())
	}
}

// resetView sets the view's text from TextChanged.
type resetView struct{ v *TextView }

func (r *resetView) TextChanged() { r.v.SetText("reset") }

// TestTextViewWrappedKeys: the vertical and line moves follow the visual
// lines of a wrapped paragraph, and a move to a line's end stays upstream
// there, with the caret drawn on that line.
func TestTextViewWrappedKeys(t *testing.T) {
	v, _, sf, s := focusedView(t, nil)
	para := strings.Repeat("word ", 12) // 60 runes: 3 lines
	v.SetText(para + "\nend")
	paint(t, s, sf)
	l := v.lines.Layout(0)
	if l.LineCount() != 3 {
		t.Fatalf("%d lines", l.LineCount())
	}
	l1, l2 := l.Line(1).Start, l.Line(2).Start
	ed := v.Editor()
	ed.SetSelection(2, 2)
	sf.Key(driver.KeyDown, 0)
	if ed.Head() != l1+2 {
		t.Errorf("down into the second line: %d, want %d", ed.Head(), l1+2)
	}
	sf.Key(driver.KeyDown, 0)
	if ed.Head() != l2+2 {
		t.Errorf("down into the third line: %d, want %d", ed.Head(), l2+2)
	}
	sf.Key(driver.KeyDown, 0)
	if ed.Head() != len(para)+1+2 {
		t.Errorf("down into the next paragraph: %d", ed.Head())
	}
	sf.Key(driver.KeyUp, 0)
	if ed.Head() != l2+2 {
		t.Errorf("up into the paragraph's last line: %d", ed.Head())
	}
	ed.SetSelection(l1+2, l1+2)
	sf.Key(driver.KeyLeft, ctrl)
	if ed.Head() != l1 {
		t.Errorf("Ctrl-Left on a wrapped line: %d, want %d", ed.Head(), l1)
	}
	sf.Key(driver.KeyRight, ctrl)
	if ed.Head() != l.Line(1).End || !ed.Upstream() {
		t.Errorf("Ctrl-Right on a wrapped line: %d upstream %v, want %d upstream", ed.Head(), ed.Upstream(), l.Line(1).End)
	}
	if top, _ := v.caretSpan(ed.Head(), ed.Upstream(), false); top != 5+16 {
		t.Errorf("the upstream caret's top %v, want the second line's", top)
	}
	// Deleting to the line start on a wrapped line stops at the soft break.
	ed.SetSelection(l1+3, l1+3)
	sf.Key(driver.KeyBackspace, ctrl)
	if ed.Head() != l1 || ed.Len() != len(para)+4-3 {
		t.Errorf("Ctrl-Backspace on a wrapped line: head %d, len %d", ed.Head(), ed.Len())
	}
}

// TestTextViewCaretIntoView: every move and edit scrolls the caret into
// view by the least amount, the container inset showing on the first and
// last lines.
func TestTextViewCaretIntoView(t *testing.T) {
	v, _, sf, s := focusedView(t, nil)
	v.SetText(numberedText(40)) // line i at 5+16i, document 650
	ed := v.Editor()
	ed.SetSelection(0, 0)
	for range 10 {
		sf.Key(driver.KeyDown, 0)
	}
	if got := v.Offset().Y; got != 181-80 {
		t.Errorf("down to line 10: offset %v, want %v", got, 181-80)
	}
	sf.Key(driver.KeyUp, 0) // line 9 is in view: no scroll
	if got := v.Offset().Y; got != 101 {
		t.Errorf("up to line 9: offset %v", got)
	}
	for range 9 {
		sf.Key(driver.KeyUp, 0)
	}
	if got := v.Offset().Y; got != 0 {
		t.Errorf("up to line 0: offset %v, want the inset showing", got)
	}
	sf.Key(driver.KeyEnd, 0)
	if got := v.Offset().Y; got != 650-80 {
		t.Errorf("End: offset %v", got)
	}
	sf.Key(driver.KeyHome, 0)
	sf.Key(driver.KeyEnd, shf) // a selection: its head
	if got := v.Offset().Y; got != 570 {
		t.Errorf("Shift-End: offset %v", got)
	}
	// An edit far from the viewport scrolls back to the caret.
	ed.SetSelection(ed.Buffer().ParagraphStart(3), ed.Buffer().ParagraphStart(3))
	v.sv.SetOffset(driver.Point{Y: 400})
	sf.Type("x")
	if got := v.Offset().Y; got != 53 {
		t.Errorf("typing off screen: offset %v, want line 3's top", got)
	}
	// Typing newlines at the bottom keeps the caret in view.
	ed.SetSelection(ed.Buffer().ParagraphEnd(6), ed.Buffer().ParagraphEnd(6)) // the bottom line in view
	sf.Type("\n\n")
	if got := v.Offset().Y; got != 5+16*9-80 {
		t.Errorf("Return at the bottom: offset %v, want %v", got, 5+16*9-80)
	}
	if dump := fieldLines(paint(t, s, sf)); !strings.Contains(dump, `"Line 6"`) {
		t.Errorf("painted:\n%s", dump)
	}
	// UserType too.
	v.sv.SetOffset(driver.Point{Y: 0})
	ed.SetSelection(ed.Len(), ed.Len())
	v.UserType("!")
	if got := v.Offset().Y; got != v.sv.DocumentSize.H-80 {
		t.Errorf("UserType at the end: offset %v", got)
	}
}

// TestTextViewPaging: Page Down and Page Up move the caret by the
// viewport's lines and scroll by as much, so the caret keeps its place in
// the viewport; at the document's ends the scroll clamps.
func TestTextViewPaging(t *testing.T) {
	v, _, sf, _ := focusedView(t, nil)
	v.SetText(numberedText(40))
	ed := v.Editor()
	buf := ed.Buffer()
	ed.SetSelection(1, 1)
	for i, want := range []struct {
		key    driver.Key
		line   int
		offset float64
	}{
		{driver.KeyPageDown, 5, 80},
		{driver.KeyPageDown, 10, 160},
		{driver.KeyPageUp, 5, 80},
		{driver.KeyPageUp, 0, 0},
	} {
		sf.Key(want.key, 0)
		if p := buf.ParagraphIndex(ed.Head()); p != want.line || ed.Head()-buf.ParagraphStart(p) != 1 {
			t.Errorf("%d: caret at %d (line %d), want line %d", i, ed.Head(), p, want.line)
		}
		if got := v.Offset().Y; got != want.offset {
			t.Errorf("%d: offset %v, want %v", i, got, want.offset)
		}
	}
	// Two lines down in the viewport, a page keeps it two lines down.
	v.sv.SetOffset(driver.Point{Y: 0})
	ed.SetSelection(buf.ParagraphStart(2), buf.ParagraphStart(2))
	sf.Key(driver.KeyPageDown, 0)
	if p := buf.ParagraphIndex(ed.Head()); p != 7 || v.Offset().Y != 80 {
		t.Errorf("page from line 2: line %d, offset %v", p, v.Offset().Y)
	}
	// Near the end the offset clamps and the caret stays in view.
	ed.SetSelection(buf.ParagraphStart(37), buf.ParagraphStart(37))
	sf.Key(driver.KeyPageDown, 0)
	if v.Offset().Y != 570 || ed.Head() != buf.ParagraphStart(39) {
		t.Errorf("page at the end: offset %v, caret %d, want the last line's start %d", v.Offset().Y, ed.Head(), buf.ParagraphStart(39))
	}
	// Shift-Page Down extends.
	ed.SetSelection(1, 1)
	v.sv.SetOffset(driver.Point{})
	sf.Key(driver.KeyPageDown, shf)
	if a, h := ed.Anchor(), ed.Head(); a != 1 || buf.ParagraphIndex(h) != 5 || v.Offset().Y != 80 {
		t.Errorf("Shift-Page Down: %d,%d offset %v", a, h, v.Offset().Y)
	}
}

// TestTextViewFocusKeys: Shift-Tab, Ctrl-Tab and Ctrl-Shift-Tab move the
// focus; Tab and Return stay in the view; Escape goes on.
func TestTextViewFocusKeys(t *testing.T) {
	s := startShell(t)
	d := newDriver(s)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 300, H: 200}, Style: allStyles})
	before, after := d.NewTextField(false, nil), d.NewTextField(false, nil)
	v := d.NewTextView(nil).(*TextView)
	box := d.NewContainer().(*Container)
	box.SetChildren([]driver.Widget{before, v, after})
	w.SetContent(box)
	before.SetFrame(driver.Rect{X: 10, Y: 10, W: 100, H: 22})
	v.SetFrame(driver.Rect{X: 10, Y: 40, W: 180, H: 80})
	after.SetFrame(driver.Rect{X: 10, Y: 130, W: 100, H: 22})
	focused := func() Element { return w.Router().Focused() }
	v.view.Focus()
	if !sf.Key(driver.KeyTab, 0) || focused() != Element(v.view) || v.Text() != "\t" {
		t.Errorf("Tab: focus %T, text %q", focused(), v.Text())
	}
	if !sf.Key(driver.KeyReturn, 0) || focused() != Element(v.view) || v.Text() != "\t\n" {
		t.Errorf("Return: text %q", v.Text())
	}
	if sf.Key(driver.KeyEscape, 0) {
		t.Error("Escape taken")
	}
	sf.Key(driver.KeyTab, shf)
	if focused() != ElementOf(before) {
		t.Errorf("Shift-Tab: focus %T", focused())
	}
	v.view.Focus()
	if !sf.Key(driver.KeyTab, ctrl) || focused() != ElementOf(after) {
		t.Errorf("Ctrl-Tab: focus %T", focused())
	}
	v.view.Focus()
	if !sf.Key(driver.KeyTab, ctrl|shf) || focused() != ElementOf(before) {
		t.Errorf("Ctrl-Shift-Tab: focus %T", focused())
	}
	if v.Text() != "\t\n" {
		t.Errorf("the focus keys typed: %q", v.Text())
	}
}

// TestTextViewReadOnlyKeys: a read-only view moves and selects, takes the
// edits and typing without a change, and lets Return and Tab go on; a
// disabled one gets nothing.
func TestTextViewReadOnlyKeys(t *testing.T) {
	h := &viewLog{}
	v, _, sf, _ := focusedView(t, h)
	v.SetText("one\ntwo")
	v.SetEditable(false)
	v.Editor().SetSelection(1, 1)
	for _, k := range []struct {
		key  driver.Key
		mods driver.Modifiers
	}{
		{driver.KeyBackspace, 0}, {driver.KeyDelete, alt}, {driver.KeyBackspace, ctrl},
		{'z', ctrl}, {'z', ctrl | shf},
	} {
		if !sf.Key(k.key, k.mods) {
			t.Errorf("%v %v not taken", k.key, k.mods)
		}
	}
	sf.Type("x ")
	if sf.Key(driver.KeyReturn, 0) || sf.Key(driver.KeyReturn, shf) {
		t.Error("Return taken while read-only")
	}
	if v.Text() != "one\ntwo" || h.changed != 0 {
		t.Errorf("read-only edited: %q, %d events", v.Text(), h.changed)
	}
	sf.Key(driver.KeyDown, 0)
	sf.Key(driver.KeyRight, shf)
	if got := viewSel(v); got != "one\ntwo|5,6" {
		t.Errorf("read-only moves: %q", got)
	}
	sf.Key('a', ctrl)
	if got := viewSel(v); got != "one\ntwo|0,7" {
		t.Errorf("read-only SelectAll: %q", got)
	}
	// Tab goes on to the focus (nothing else to focus: it stays or
	// clears, but types nothing).
	sf.Key(driver.KeyTab, 0)
	if v.Text() != "one\ntwo" {
		t.Errorf("Tab typed: %q", v.Text())
	}
	v.SetEditable(true)
	v.view.Focus()
	v.SetEnabled(false)
	if sf.Key(driver.KeyBackspace, 0) || v.view.HandleEvent(KeyEvent{shell.KeyEvent{Down: true, Key: driver.KeyBackspace}}) ||
		v.view.HandleEvent(TextEvent{shell.TextEvent{Text: "x"}}) {
		t.Error("a disabled view took a key or text")
	}
	if v.Text() != "one\ntwo" {
		t.Errorf("disabled edited: %q", v.Text())
	}
}

// TestTextViewSpace: Space types a space and does not page the internal
// scroll view, nor an enclosing one.
func TestTextViewSpace(t *testing.T) {
	s := startShell(t)
	d := newDriver(s)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	outer := d.NewScrollView(nil).(*ScrollView)
	outer.SetHasVerticalScroller(true)
	v := d.NewTextView(nil).(*TextView)
	outer.SetDocument(v)
	w.SetContent(outer)
	outer.SetFrame(driver.Rect{W: 200, H: 72})
	outer.SetDocumentSize(driver.Size{W: 200, H: 400})
	v.SetFrame(driver.Rect{W: 180, H: 80})
	v.SetText(numberedText(40))
	v.Editor().SetSelection(0, 0)
	v.view.Focus()
	inner, out := v.Offset(), outer.Offset()
	sf.Type(" ")
	if sf.Key(driver.KeySpace, shf) { // no text: the key alone
		t.Error("Shift-Space taken")
	}
	if !strings.HasPrefix(v.Text(), " Line 0") {
		t.Errorf("Space typed nothing: %q", v.Text()[:8])
	}
	if v.Offset() != inner || outer.Offset() != out {
		t.Errorf("Space paged: inner %v → %v, outer %v → %v", inner, v.Offset(), out, outer.Offset())
	}
}

// TestTextViewMegaKeys: moving and typing in a 1 MB text lays out only the
// paragraphs that show and those the moves pass, and the caret stays in
// view.
func TestTextViewMegaKeys(t *testing.T) {
	v, _, sf, s := focusedView(t, nil)
	v.SetFrame(driver.Rect{X: 10, Y: 10, W: 180, H: 600})
	v.SetText(megaText(1))
	ed := v.Editor()
	mid := ed.Len() / 2
	ed.SetSelection(mid, mid)
	paint(t, s, sf)
	made := v.lines.Made()
	for range 20 {
		sf.Key(driver.KeyDown, 0)
	}
	sf.Type("hello\n")
	for range 3 {
		sf.Key(driver.KeyPageDown, 0)
	}
	sf.Key(driver.KeyEnd, 0)
	paint(t, s, sf)
	if n := v.lines.Made() - made; n > 6*v.lines.PageLines() {
		t.Errorf("%d layouts made by the keys (page %d lines)", n, v.lines.PageLines())
	}
	if got, want := v.Offset().Y, v.sv.DocumentSize.H-600; got != want {
		t.Errorf("End: offset %v, want %v", got, want)
	}
	if v.lines.Cached() > v.lines.Bound() {
		t.Errorf("%d layouts cached, bound %d", v.lines.Cached(), v.lines.Bound())
	}
}

// TestTextViewSelectAllKeepsScroll (review B5, P5): Select All, by its key
// and by the Edit menu's command, scrolls nothing, as in NSTextView; every
// other key brings the caret into view once (an edit too, not again after
// it).
func TestTextViewSelectAllKeepsScroll(t *testing.T) {
	v, _, sf, _ := focusedView(t, nil)
	v.SetText(numberedText(40))
	ed := v.Editor()
	mid := ed.Buffer().ParagraphStart(20)
	v.SetSelection(mid, mid)
	at := v.Offset().Y
	if at == 0 {
		t.Fatal("the middle of the text is at the top")
	}
	reveals := v.reveals
	sf.Key('a', ctrl)
	if s, e := ed.Selection(); s != 0 || e != ed.Len() {
		t.Fatalf("Ctrl-A selected [%d, %d)", s, e)
	}
	if got := v.Offset().Y; got != at {
		t.Errorf("Ctrl-A scrolled from %v to %v", at, got)
	}
	v.SetSelection(mid, mid)
	v.view.command(text.CmdSelectAll)
	if got := v.Offset().Y; got != at {
		t.Errorf("Edit ▸ Select All scrolled from %v to %v", at, got)
	}
	if v.reveals != reveals {
		t.Errorf("Select All revealed the caret %d times", v.reveals-reveals)
	}
	for _, k := range []struct {
		name string
		do   func()
	}{
		{"Down", func() { sf.Key(driver.KeyDown, 0) }},
		{"typing", func() { sf.Type("x") }},
		{"Backspace", func() { sf.Key(driver.KeyBackspace, 0) }},
		{"Return", func() { sf.Key(driver.KeyReturn, 0) }},
		{"End", func() { sf.Key(driver.KeyEnd, 0) }},
	} {
		v.SetSelection(mid, mid)
		reveals = v.reveals
		k.do()
		if n := v.reveals - reveals; n != 1 {
			t.Errorf("%s: the caret revealed %d times, want once", k.name, n)
		}
	}
}
