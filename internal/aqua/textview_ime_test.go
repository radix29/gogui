package aqua

import (
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// IME compositions in the TextView (PLAN.md S5.4j): the TextField's IME
// tests (textfield_ime_test.go) repeated for the view over the fake shell,
// plus a preedit in a wrapped paragraph and one that grows its paragraph.
// The view is at surface (10, 38, 180, 80): 5 lines of 16 pt below a 5 pt
// inset, wrapping after 24 fake runes of 7 pt, the text 5 pt in from the
// left, so the caret before rune n of the first line is at surface
// (15 + 7n − 1, 43, 2, 16).

// imeView is focusedView, checking that the window enabled the surface's
// text input.
func imeView(t *testing.T, h driver.TextViewHandler) (*TextView, *Window, *fake.Surface, *fake.Shell) {
	t.Helper()
	v, w, sf, s := focusedView(t, h)
	if !sf.TextInput().Enabled {
		t.Fatalf("text input %+v for a focused view", sf.TextInput())
	}
	return v, w, sf, s
}

// viewShown returns the paragraphs as shown (the preedit spliced in) with
// the caret as "|" while it is visible, and the composition state.
func viewShown(v *TextView) string {
	v.settle()
	ed := v.Editor()
	buf := ed.Buffer()
	head := buf.ParagraphIndex(ed.Head())
	var b strings.Builder
	for i := range buf.ParagraphCount() {
		if i > 0 {
			b.WriteByte('\n')
		}
		s := v.lines.Shown(i).Text()
		if i == head && v.view.CaretVisible() {
			_, _, lo, _ := v.caretIn(ed.Head(), ed.Upstream())
			s = s[:lo] + "|" + s[lo:]
		}
		b.WriteString(s)
	}
	if v.view.composing() {
		b.WriteString(" composing")
	}
	return b.String()
}

// changes returns the TextChanged count since the last call.
func changes(h *viewLog) int {
	n := h.changed
	h.changed = 0
	return n
}

// TestTextViewPreeditComposition: a composition deletes the selection at
// its start (TextChanged), shows the preedit spliced into its paragraph
// with the caret at its cursor, changes without events or text, and its
// commit is one edit with TextChanged; Text and the a11y value never hold
// the preedit.
func TestTextViewPreeditComposition(t *testing.T) {
	h := &viewLog{}
	v, _, sf, _ := imeView(t, h)
	v.SetText("one\nHello\nend")
	v.Editor().SetSelection(5, 8)

	sf.Preedit("k", 1)
	if got := viewShown(v); got != "one\nHk|o\nend composing" || viewSel(v) != "one\nHo\nend|5,5" || changes(h) != 1 {
		t.Errorf("start over a selection: %s %s", got, viewSel(v))
	}
	sf.Preedit("か", 3)
	sf.Preedit("かん", 3) // the IME's caret inside the preedit
	if got := viewShown(v); got != "one\nHか|んo\nend composing" || changes(h) != 0 || v.Text() != "one\nHo\nend" {
		t.Errorf("change: %s %q", got, v.Text())
	}
	if n := v.view.AccessibleNode(); n.Value != "one\nHo\nend" {
		t.Errorf("a11y while composing: %q", n.Value)
	}
	sf.CommitPreedit("漢")
	if got := viewShown(v); got != "one\nH漢|o\nend" || viewSel(v) != "one\nH漢o\nend|8,8" || changes(h) != 1 {
		t.Errorf("commit: %s %s", got, viewSel(v))
	}
	if sf.Composing() || sf.Cancels() != 0 {
		t.Errorf("after the commit: composing %v, %d cancels", sf.Composing(), sf.Cancels())
	}
	if _, _, _, ok := v.lines.Preedit(); ok {
		t.Error("the layouts still compose")
	}

	sf.Preedit("ご", 3) // at an empty selection: nothing deleted
	if got := viewShown(v); got != "one\nH漢ご|o\nend composing" || changes(h) != 0 {
		t.Errorf("start at the caret: %s", got)
	}
	sf.Send(shell.PreeditEvent{Text: "ごは", Cursor: 1}) // inside a rune: the end
	if got := viewShown(v); got != "one\nH漢ごは|o\nend composing" {
		t.Errorf("bad cursor: %s", got)
	}
}

// TestTextViewPreeditEnd: an empty preedit ends the composition and leaves
// the text as it was; both commit orders work.
func TestTextViewPreeditEnd(t *testing.T) {
	h := &viewLog{}
	v, _, sf, _ := imeView(t, h)
	v.SetText("ab")
	sf.EndPreedit()
	sf.Preedit("に", 3)
	sf.EndPreedit()
	if got := viewShown(v); got != "ab|" || changes(h) != 0 || v.Editor().CanUndo() {
		t.Errorf("cancelled: %s", got)
	}
	sf.Preedit("にほん", 9)
	sf.Commit("日本")
	sf.Preedit("ご", 3)
	if got := viewShown(v); got != "ab日本ご| composing" || changes(h) != 1 {
		t.Errorf("Win32 order: %s", got)
	}
	sf.CommitPreedit("語")
	if got := viewShown(v); got != "ab日本語|" || changes(h) != 1 {
		t.Errorf("GTK order: %s", got)
	}
}

// TestTextViewPreeditUndo: each commit is one undo group, apart from the
// typing around it and from the selection deleted at the start.
func TestTextViewPreeditUndo(t *testing.T) {
	h := &viewLog{}
	v, _, sf, _ := imeView(t, h)
	sf.Type("ab")
	v.Editor().SetSelection(0, 1)
	sf.Preedit("に", 3)
	sf.CommitPreedit("日")
	sf.Preedit("ほ", 3)
	sf.CommitPreedit("本")
	sf.Type("cd")
	changes(h)
	for _, want := range []string{"日本b|6,6", "日b|3,3", "b|0,0", "ab|0,1", "|0,0"} {
		sf.Key('z', ctrl)
		if got := viewSel(v); got != want {
			t.Errorf("undo: %s, want %s", got, want)
		}
	}
	for _, want := range []string{"ab|2,2", "b|0,0", "日b|3,3", "日本b|6,6", "日本cdb|8,8"} {
		sf.Key('z', ctrl|shf)
		if got := viewSel(v); got != want {
			t.Errorf("redo: %s, want %s", got, want)
		}
	}
	if n := changes(h); n != 10 {
		t.Errorf("%d undo and redo events", n)
	}
}

// TestTextViewPreeditInterruptions: a blur, a click in the view, the
// window losing key, a key, a late paste, SetSelection and
// SetEditable(false) commit the preedit themselves (TextChanged) and
// cancel the shell's composition; SetText and UserType drop it and cancel.
func TestTextViewPreeditInterruptions(t *testing.T) {
	for _, c := range []struct {
		name    string
		do      func(v *TextView, w *Window, sf *fake.Surface, s *fake.Shell)
		want    string
		changed int
	}{
		{"blur", func(_ *TextView, w *Window, _ *fake.Surface, _ *fake.Shell) { w.Router().SetFocus(nil) },
			"ab\nに|6,6", 1},
		{"click", func(v *TextView, _ *Window, sf *fake.Surface, _ *fake.Shell) {
			sf.Click(viewPoint(v, 0, false, 0))
		}, "ab\nに|0,0", 1},
		{"key loss", func(_ *TextView, _ *Window, _ *fake.Surface, s *fake.Shell) { s.Deactivate() },
			"ab\nに|6,6", 1},
		{"key", func(_ *TextView, _ *Window, sf *fake.Surface, _ *fake.Shell) { sf.Key(driver.KeyLeft, 0) },
			"ab\nに|3,3", 1},
		{"Return", func(_ *TextView, _ *Window, sf *fake.Surface, _ *fake.Shell) { sf.Key(driver.KeyReturn, 0) },
			"ab\nに\n|7,7", 2},
		{"late paste", func(_ *TextView, _ *Window, sf *fake.Surface, s *fake.Shell) {
			sf.EndPreedit()
			s.SetClipboard("P", true)
			sf.Key('v', ctrl)
			sf.Preedit("に", 3) // the composition starts before the paste arrives
			s.Step()
		}, "ab\nにP|7,7", 2},
		{"SetSelection", func(v *TextView, _ *Window, _ *fake.Surface, _ *fake.Shell) { v.SetSelection(0, 1) },
			"ab\nに|0,1", 1},
		{"read-only", func(v *TextView, _ *Window, _ *fake.Surface, _ *fake.Shell) { v.SetEditable(false) },
			"ab\nに|6,6", 1},
		{"SetText", func(v *TextView, _ *Window, _ *fake.Surface, _ *fake.Shell) { v.SetText("xy") },
			"xy|2,2", 0},
		{"UserType", func(v *TextView, _ *Window, _ *fake.Surface, _ *fake.Shell) { v.UserType("xy") },
			"ab\nxy|5,5", 1},
	} {
		h := &viewLog{}
		v, w, sf, s := imeView(t, h)
		v.SetText("ab\n")
		sf.Preedit("に", 3)
		c.do(v, w, sf, s)
		if got := viewSel(v); got != c.want || h.changed != c.changed {
			t.Errorf("%s: %s, %d changes; want %s, %d", c.name, got, h.changed, c.want, c.changed)
		}
		if v.view.composing() || sf.Composing() || sf.Cancels() != 1 {
			t.Errorf("%s: composing %v %v, %d cancels", c.name, v.view.composing(), sf.Composing(), sf.Cancels())
		}
		if _, _, _, ok := v.lines.Preedit(); ok {
			t.Errorf("%s: the layouts still compose", c.name)
		}
		checkParaLayouts(t, v.lines)
	}
	// Nothing to interrupt: no cancel.
	v, w, sf, s := imeView(t, nil)
	v.SetText("a")
	sf.Click(viewPoint(v, 0, false, 0))
	s.Deactivate()
	sf.Activate()
	w.Router().SetFocus(nil)
	if sf.Cancels() != 0 || v.Text() != "a" {
		t.Errorf("no composition: %d cancels, %q", sf.Cancels(), v.Text())
	}
}

// TestTextViewPreeditIgnored: read-only and disabled views take no
// preedit; the event goes on unhandled and changes nothing.
func TestTextViewPreeditIgnored(t *testing.T) {
	for _, c := range []struct {
		name  string
		setup func(v *TextView)
	}{
		{"read-only", func(v *TextView) { v.SetEditable(false) }},
		{"disabled", func(v *TextView) { v.SetEnabled(false) }},
	} {
		h := &viewLog{}
		v, _, sf, _ := imeView(t, h)
		v.SetText("ab")
		v.Editor().SetSelection(0, 2)
		c.setup(v)
		if sf.Send(shell.PreeditEvent{Text: "か", Cursor: 3}) {
			t.Errorf("%s: preedit handled", c.name)
		}
		if v.view.composing() || viewSel(v) != "ab|0,2" || h.changed != 0 {
			t.Errorf("%s: composing %v, %s, %d changes", c.name, v.view.composing(), viewSel(v), h.changed)
		}
	}
}

// hookView runs fn on the first TextChanged.
type hookView struct {
	fn    func()
	calls int
}

func (h *hookView) TextChanged() {
	h.calls++
	if h.calls == 1 && h.fn != nil {
		h.fn()
	}
}

// TestTextViewPreeditHandlerChanges: a TextChanged handler that disables
// the view, takes its focus or makes it read-only when the selection is
// deleted at the start keeps the composition from starting.
func TestTextViewPreeditHandlerChanges(t *testing.T) {
	for _, c := range []struct {
		name string
		do   func(v *TextView, w *Window)
	}{
		{"disable", func(v *TextView, _ *Window) { v.SetEnabled(false) }},
		{"blur", func(_ *TextView, w *Window) { w.Router().SetFocus(nil) }},
		{"read-only", func(v *TextView, _ *Window) { v.SetEditable(false) }},
	} {
		h := &hookView{}
		v, w, sf, _ := imeView(t, h)
		v.SetText("abc")
		v.Editor().SetSelection(0, 2)
		h.fn = func() { c.do(v, w) }
		sf.Preedit("か", 3)
		if v.view.composing() || v.Text() != "c" || h.calls != 1 {
			t.Errorf("%s: composing %v, %q, %d calls", c.name, v.view.composing(), v.Text(), h.calls)
		}
	}
}

// TestTextViewPreeditCaret: the caret stays solid with no blink timer
// while composing and blinks again after the commit or the end.
func TestTextViewPreeditCaret(t *testing.T) {
	v, _, sf, s := imeView(t, nil)
	clk := s.FakeClock()
	clk.Flush()
	wantViewBlink(t, "focused", v, s, true, halfBlink)
	sf.Preedit("に", 3)
	wantViewBlink(t, "composing", v, s, true, -1)
	clk.Advance(halfBlink)
	clk.Flush()
	wantViewBlink(t, "composing, later", v, s, true, -1)
	sf.CommitPreedit("日")
	wantViewBlink(t, "committed", v, s, true, halfBlink)
	sf.Preedit("に", 3)
	sf.EndPreedit()
	wantViewBlink(t, "cancelled", v, s, true, halfBlink)
}

// viewCaret is the published caret before rune n of line k (no scroll).
func viewCaret(n, k int) shell.TextInput {
	return shell.TextInput{Enabled: true, Caret: driver.Rect{X: 14 + 7*float64(n), Y: 43 + 16*float64(k), W: 2, H: 16}}
}

// TestTextViewTextInput: the window turns the IME on for the focused view
// with the caret as painted, at the preedit's cursor while composing, on
// a wrapped line, by the scroll offset; off when read-only or blurred.
func TestTextViewTextInput(t *testing.T) {
	v, w, sf, s := imeView(t, nil)
	c := &tiChecker{t: t, sf: sf, sets: sf.TextInputSets()}
	paint(t, s, sf)
	c.want("focused", viewCaret(0, 0), 0)
	sf.Type("abc")
	paint(t, s, sf)
	c.want("typed", viewCaret(3, 0), 1)
	sf.Preedit("にほん", 6)
	paint(t, s, sf)
	c.want("composing", viewCaret(5, 0), 1)

	// A preedit that runs onto the second line: the cursor follows it.
	sf.Preedit(strings.Repeat("に", 23), 3*23)
	paint(t, s, sf)
	c.want("wrapped", viewCaret(2, 1), 1)
	sf.EndPreedit()
	paint(t, s, sf)
	c.want("ended", viewCaret(3, 0), 1)

	v.SetText(numberedText(20))
	v.SetSelection(0, 0)
	v.sv.SetOffset(driver.Point{Y: 37})
	paint(t, s, sf)
	c.want("scrolled", shell.TextInput{Enabled: true, Caret: driver.Rect{X: 14, Y: 43 - 37, W: 2, H: 16}}, 1)
	v.SetEditable(false)
	paint(t, s, sf)
	c.want("read-only", tiOff, 1)
	v.SetEditable(true)
	paint(t, s, sf)
	w.Router().SetFocus(nil)
	c.want("blur", tiOff, 2)
}

// TestTextViewPreeditWrapped: a preedit in a wrapped paragraph lays out
// with the paragraph's text, its clause underlined on the line it wrapped
// to, the caret at its cursor there, and the text without it unchanged.
func TestTextViewPreeditWrapped(t *testing.T) {
	v, _, sf, s := imeView(t, nil)
	v.SetText("aaaa bbbb cccc dddd eeee ffff\nend") // wraps after "eeee "
	v.SetSelection(20, 20)                          // before "eeee"
	sf.Preedit("かきくけ", 12, shell.PreeditSegment{Start: 0, End: 12, Selected: true})
	l := v.lines.Shown(0)
	if l.Text() != "aaaa bbbb cccc dddd かきくけeeee ffff" || l.LineCount() != 2 || l.Line(1).Start != 20 {
		t.Fatalf("composed %q, %d lines, second from %d", l.Text(), l.LineCount(), l.Line(1).Start)
	}
	if got := viewShown(v); got != "aaaa bbbb cccc dddd かきくけ|eeee ffff\nend composing" {
		t.Errorf("shown %s", got)
	}
	if top, _ := v.caretSpan(v.Editor().Head(), false, false); top != 5+16 {
		t.Errorf("caret top %v, want the second line's", top)
	}
	// The underline on the second line: at its left, 4 runes less the gap,
	// 2 pt (the selected clause) below its baseline (12 pt down) + 1.
	got := paint(t, s, sf)
	if want := "fillRRect (15,72,27,2) solid #000000"; !strings.Contains(got, want) {
		t.Errorf("no %q in\n%s", want, got)
	}
	if v.lines.Layout(0).Text() != "aaaa bbbb cccc dddd eeee ffff" || v.Text() != "aaaa bbbb cccc dddd eeee ffff\nend" {
		t.Errorf("the text holds the preedit: %q", v.Text())
	}
	checkParaLayouts(t, v.lines)
}

// TestTextViewPreeditGrows: a preedit that wraps its paragraph onto more
// lines moves the paragraphs below it and grows the document; the caret
// at its cursor scrolls into view; ending the composition gives the
// paragraph its height back.
func TestTextViewPreeditGrows(t *testing.T) {
	v, _, sf, s := imeView(t, nil)
	v.SetText(numberedText(4)) // 4 lines, 16 pt each, in a viewport of 80
	v.SetSelection(6, 6)       // the end of "Line 0"
	idx := v.lines.Index()
	doc := v.sv.DocumentSize.H
	if idx.Y(1) != 16 || doc != 80 {
		t.Fatalf("before: paragraph 1 at %v, document %v", idx.Y(1), doc)
	}
	// "Line 0" and 25 runes: "Line ", then 24 runes, then 2.
	pre := strings.Repeat("あ", 25)
	sf.Preedit(pre, len(pre))
	if idx.Height(0) != 48 || idx.Y(1) != 48 || idx.Y(3) != 80 {
		t.Errorf("composing: paragraph 0 %v high, paragraph 1 at %v, 3 at %v", idx.Height(0), idx.Y(1), idx.Y(3))
	}
	if got := v.sv.DocumentSize.H; got != 2*5+48+48 {
		t.Errorf("document %v while composing", got)
	}
	if top, _ := v.caretSpan(v.Editor().Head(), false, false); top != 5+32 {
		t.Errorf("caret top %v, want the third line's", top)
	}
	got := paint(t, s, sf)
	for _, want := range []string{`"Line 1"`, `"Line 2"`} { // moved down a line each, still shown
		if !strings.Contains(got, want) {
			t.Errorf("no %s painted\n%s", want, got)
		}
	}
	checkParaLayouts(t, v.lines)

	// Growing past the viewport: the cursor's line scrolls into view.
	long := strings.Repeat("あ", 24*4)
	sf.Preedit(long, len(long))
	_, bottom := v.caretSpan(v.Editor().Head(), false, false)
	if off := v.Offset().Y; !(off > 0) || bottom > off+80 {
		t.Errorf("offset %v, caret bottom %v: not in view", off, bottom)
	}
	sf.EndPreedit()
	v.SetSelection(6, 6)
	if idx.Height(0) != 16 || idx.Y(1) != 16 || v.sv.DocumentSize.H != 80 || v.Text() != numberedText(4) {
		t.Errorf("ended: paragraph 0 %v high, 1 at %v, document %v", idx.Height(0), idx.Y(1), v.sv.DocumentSize.H)
	}
	checkParaLayouts(t, v.lines)
}

// TestTextViewPreeditUnderlines: the clauses are underlined below the
// baseline in the text color, the selected one thicker, each a gap short
// of its end; no clauses underline the whole preedit.
func TestTextViewPreeditUnderlines(t *testing.T) {
	for _, c := range []struct {
		segs []shell.PreeditSegment
		want []string
	}{
		{nil, []string{"fillRRect (22,56,20,1) solid #000000"}},
		{[]shell.PreeditSegment{{Start: 0, End: 3}, {Start: 3, End: 9, Selected: true}}, []string{
			"fillRRect (22,56,6,1) solid #000000", "fillRRect (29,56,13,2) solid #000000",
		}},
	} {
		v, _, sf, s := imeView(t, nil)
		v.SetText("a")
		sf.Preedit("にほん", 9, c.segs...)
		got := paint(t, s, sf)
		for _, want := range c.want {
			if !strings.Contains(got, want) {
				t.Errorf("%v: no %q in\n%s", c.segs, want, got)
			}
		}
	}
}
