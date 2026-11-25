package aqua

import (
	"fmt"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// IME compositions in TextField (PLAN.md S5.3c), driven through the fake
// shell's Preedit, EndPreedit and CommitPreedit. The window turns the
// surface's text input on for the focused field (S5.3d).

// imeField is focusedField, checking that the window enabled the
// surface's text input (off for a SecureField: preedits reach it only
// through Send).
func imeField(t *testing.T, secure bool, h driver.TextFieldHandler) (*TextField, *Window, *fake.Surface, *fake.Shell) {
	t.Helper()
	tf, w, sf, s := focusedField(t, secure, h)
	if sf.TextInput().Enabled == secure {
		t.Fatalf("text input %+v for a field secure %v", sf.TextInput(), secure)
	}
	return tf, w, sf, s
}

// shownOf returns the shown text with the caret as "|" while it is
// visible, and the composition state: "Hか|o composing".
func shownOf(tf *TextField) string {
	v := tf.view
	s := v.textLayout().Text()
	if v.CaretVisible() {
		at := v.caretOffset()
		s = s[:at] + "|" + s[at:]
	}
	if v.composing() {
		s += " composing"
	}
	return s
}

// events joins the field's events and clears them.
func events(h *fieldLog) string {
	s := strings.Join(h.events, ";")
	h.events = nil
	return s
}

// TestPreeditComposition: a composition deletes the selection at its
// start (TextChanged), shows the preedit spliced in at the caret with the
// caret at its cursor, changes without events or text, and its commit is
// one edit with TextChanged; the value and the a11y value never hold the
// preedit.
func TestPreeditComposition(t *testing.T) {
	h := &fieldLog{}
	tf, _, sf, _ := imeField(t, false, h)
	tf.SetText("Hello")
	tf.Editor().SetSelection(1, 4)

	sf.Preedit("k", 1)
	if got := shownOf(tf); got != "Hk|o composing" || selOf(tf) != "Ho|1,1" || events(h) != "changed Ho" {
		t.Errorf("start over a selection: %s %s %q", got, selOf(tf), h.events)
	}
	sf.Preedit("か", 3)
	sf.Preedit("かん", 3) // the IME's caret inside the preedit
	if got := shownOf(tf); got != "Hか|んo composing" || selOf(tf) != "Ho|1,1" || events(h) != "" || tf.Text() != "Ho" {
		t.Errorf("change: %s %s %q", got, selOf(tf), h.events)
	}
	if got := nodeOf(tf); got != `textfield "" value="Ho"` {
		t.Errorf("a11y while composing: %s", got)
	}
	sf.CommitPreedit("漢")
	if got := shownOf(tf); got != "H漢|o" || selOf(tf) != "H漢o|4,4" || events(h) != "changed H漢o" || tf.Text() != "H漢o" {
		t.Errorf("commit: %s %s %q", got, selOf(tf), h.events)
	}
	if sf.Composing() || sf.Cancels() != 0 {
		t.Errorf("after the commit: composing %v, %d cancels", sf.Composing(), sf.Cancels())
	}

	// A composition at an empty selection deletes nothing.
	sf.Preedit("ご", 3)
	if got := shownOf(tf); got != "H漢ご|o composing" || events(h) != "" {
		t.Errorf("start at the caret: %s %q", got, h.events)
	}
	// An IME without clauses or with a bad cursor still shows its preedit.
	sf.Send(shell.PreeditEvent{Text: "ごは", Cursor: 1}) // inside a rune: the end
	if got := shownOf(tf); got != "H漢ごは|o composing" {
		t.Errorf("bad cursor: %s", got)
	}
}

// TestPreeditEnd: an empty preedit ends the composition and leaves the
// text as it was; one outside a composition does nothing. Both orders of
// a commit work: GTK (the commit, then the empty preedit) and Win32 (the
// commit, then the next composition).
func TestPreeditEnd(t *testing.T) {
	h := &fieldLog{}
	tf, _, sf, _ := imeField(t, false, h)
	tf.SetText("ab")
	sf.EndPreedit()
	sf.Preedit("に", 3)
	sf.EndPreedit()
	if got := shownOf(tf); got != "ab|" || events(h) != "" || tf.Editor().CanUndo() {
		t.Errorf("cancelled: %s %q", got, h.events)
	}
	// The Win32 order: the commit and the next composition in one go.
	sf.Preedit("にほん", 9)
	sf.Commit("日本")
	sf.Preedit("ご", 3)
	if got := shownOf(tf); got != "ab日本ご| composing" || events(h) != "changed ab日本" {
		t.Errorf("Win32 order: %s %q", got, h.events)
	}
	sf.CommitPreedit("語")
	if got := shownOf(tf); got != "ab日本語|" || events(h) != "changed ab日本語" {
		t.Errorf("GTK order: %s %q", got, h.events)
	}
}

// TestPreeditUndo: each commit is one undo group, apart from the typing
// around it and from the selection deleted at the start, and redo brings
// them back in order.
func TestPreeditUndo(t *testing.T) {
	h := &fieldLog{}
	tf, _, sf, _ := imeField(t, false, h)
	sf.Type("ab")
	tf.Editor().SetSelection(0, 1)
	sf.Preedit("に", 3)
	sf.CommitPreedit("日")
	sf.Preedit("ほ", 3)
	sf.CommitPreedit("本")
	sf.Type("cd")
	h.events = nil
	for _, want := range []string{"日本b|6,6", "日b|3,3", "b|0,0", "ab|0,1", "|0,0"} {
		sf.Key('z', ctrl)
		if got := selOf(tf); got != want {
			t.Errorf("undo: %s, want %s", got, want)
		}
	}
	for _, want := range []string{"ab|2,2", "b|0,0", "日b|3,3", "日本b|6,6", "日本cdb|8,8"} {
		sf.Key('z', ctrl|shf)
		if got := selOf(tf); got != want {
			t.Errorf("redo: %s, want %s", got, want)
		}
	}
	if len(h.events) != 10 || h.events[9] != "changed 日本cdb" {
		t.Errorf("undo and redo events %q", h.events)
	}
}

// TestPreeditInterruptions: a blur, a click in the field, the window
// losing key, a key, a late paste and SetEditable(false) commit the
// preedit themselves (TextChanged) and cancel the shell's composition;
// SetText and UserType drop it and cancel too.
func TestPreeditInterruptions(t *testing.T) {
	for _, c := range []struct {
		name   string
		do     func(tf *TextField, w *Window, sf *fake.Surface, s *fake.Shell)
		want   string // the text and selection after
		events string
	}{
		{"blur", func(_ *TextField, w *Window, _ *fake.Surface, _ *fake.Shell) { w.Router().SetFocus(nil) },
			"abに|5,5", "changed abに"},
		{"click", func(tf *TextField, _ *Window, sf *fake.Surface, _ *fake.Shell) {
			sf.Click(fieldPoint(tf, 0, 0))
		}, "abに|0,0", "changed abに"},
		{"key loss", func(_ *TextField, _ *Window, _ *fake.Surface, s *fake.Shell) { s.Deactivate() },
			"abに|5,5", "changed abに"},
		{"key", func(_ *TextField, _ *Window, sf *fake.Surface, _ *fake.Shell) { sf.Key(driver.KeyLeft, 0) },
			"abに|2,2", "changed abに"},
		{"late paste", func(_ *TextField, _ *Window, sf *fake.Surface, s *fake.Shell) {
			sf.EndPreedit()
			s.SetClipboard("P", true)
			sf.Key('v', ctrl)
			sf.Preedit("に", 3) // the composition starts before the paste arrives
			s.Step()
		}, "abにP|6,6", "changed abに;changed abにP"},
		{"read-only", func(tf *TextField, _ *Window, _ *fake.Surface, _ *fake.Shell) { tf.SetEditable(false) },
			"abに|5,5", "changed abに"},
		{"SetText", func(tf *TextField, _ *Window, _ *fake.Surface, _ *fake.Shell) { tf.SetText("xy") },
			"xy|2,2", ""},
		{"UserType", func(tf *TextField, _ *Window, _ *fake.Surface, _ *fake.Shell) { tf.UserType("xy") },
			"xy|2,2", "changed xy"},
	} {
		h := &fieldLog{}
		tf, w, sf, s := imeField(t, false, h)
		tf.SetText("ab")
		sf.Preedit("に", 3)
		c.do(tf, w, sf, s)
		if got := selOf(tf); got != c.want || events(h) != c.events {
			t.Errorf("%s: %s %q, want %s %q", c.name, got, h.events, c.want, c.events)
		}
		if tf.view.composing() || sf.Composing() || sf.Cancels() != 1 {
			t.Errorf("%s: composing %v %v, %d cancels", c.name, tf.view.composing(), sf.Composing(), sf.Cancels())
		}
		if tf.Text() != tf.Editor().Text() {
			t.Errorf("%s: Text %q", c.name, tf.Text())
		}
	}
	// Nothing to interrupt: no cancel.
	tf, w, sf, s := imeField(t, false, nil)
	tf.SetText("a")
	sf.Click(fieldPoint(tf, 0, 0))
	s.Deactivate()
	sf.Activate()
	w.Router().SetFocus(nil)
	if sf.Cancels() != 0 || tf.Text() != "a" {
		t.Errorf("no composition: %d cancels, %q", sf.Cancels(), tf.Text())
	}
}

// TestPreeditIgnoredFields: secure, read-only and disabled fields take no
// preedit; the event goes on unhandled and changes nothing.
func TestPreeditIgnoredFields(t *testing.T) {
	for _, c := range []struct {
		name   string
		secure bool
		setup  func(tf *TextField)
	}{
		{"secure", true, func(*TextField) {}},
		{"read-only", false, func(tf *TextField) { tf.SetEditable(false) }},
		{"disabled", false, func(tf *TextField) { tf.SetEnabled(false) }},
	} {
		h := &fieldLog{}
		tf, _, sf, _ := imeField(t, c.secure, h)
		tf.SetText("ab")
		tf.Editor().SetSelection(0, 2)
		c.setup(tf)
		if sf.Send(shell.PreeditEvent{Text: "か", Cursor: 3}) {
			t.Errorf("%s: preedit handled", c.name)
		}
		if tf.view.composing() || selOf(tf) != "ab|0,2" || len(h.events) != 0 {
			t.Errorf("%s: composing %v, %s %q", c.name, tf.view.composing(), selOf(tf), h.events)
		}
	}
}

// TestPreeditHandlerChanges: a TextChanged handler that disables the field
// or takes its focus when the selection is deleted at the start keeps the
// composition from starting.
func TestPreeditHandlerChanges(t *testing.T) {
	for _, c := range []struct {
		name string
		do   func(tf *TextField, w *Window)
	}{
		{"disable", func(tf *TextField, _ *Window) { tf.SetEnabled(false) }},
		{"blur", func(_ *TextField, w *Window) { w.Router().SetFocus(nil) }},
		{"read-only", func(tf *TextField, _ *Window) { tf.SetEditable(false) }},
	} {
		h := &hookField{}
		tf, w, sf, _ := imeField(t, false, h)
		tf.SetText("abc")
		tf.Editor().SetSelection(0, 2)
		h.fn = func() { c.do(tf, w) }
		sf.Preedit("か", 3)
		if tf.view.composing() || tf.Text() != "c" || h.calls != 1 {
			t.Errorf("%s: composing %v, %q, %d calls", c.name, tf.view.composing(), tf.Text(), h.calls)
		}
	}
}

// hookField runs fn on the first TextChanged.
type hookField struct {
	fn    func()
	calls int
}

func (h *hookField) TextChanged(string) {
	h.calls++
	if h.calls == 1 && h.fn != nil {
		h.fn()
	}
}
func (h *hookField) Submitted(string) {}

// TestPreeditCaretAndScroll: the caret stays solid with no blink timer
// while composing and blinks again after the commit; an overflowing
// composition is scrolled to keep the preedit's cursor in view.
func TestPreeditCaretAndScroll(t *testing.T) {
	tf, _, sf, s := imeField(t, false, nil)
	clk := s.FakeClock()
	clk.Flush()
	wantBlink(t, "focused", tf, s, true, halfBlink)
	sf.Preedit("に", 3)
	wantBlink(t, "composing", tf, s, true, -1)
	clk.Advance(halfBlink)
	clk.Flush()
	wantBlink(t, "composing, later", tf, s, true, -1)
	sf.CommitPreedit("日")
	wantBlink(t, "committed", tf, s, true, halfBlink)
	sf.Preedit("に", 3)
	sf.EndPreedit()
	wantBlink(t, "cancelled", tf, s, true, halfBlink)

	// 20 runes of text and 6 of preedit, 7 pt each, in a room of 92.
	tf.SetText("The quick brown fox ")
	tf.Editor().SetSelection(4, 4)
	paint(t, s, sf)
	sf.Preedit("jumped", 6)
	paint(t, s, sf)
	if got := tf.view.ScrollX(); got != 0 { // the caret at x 70 fits
		t.Errorf("scroll %v", got)
	}
	tf.Editor().SetSelection(20, 20)
	sf.EndPreedit()
	sf.Preedit("jumped", 0)
	paint(t, s, sf)
	if got := tf.view.ScrollX(); got != 140-92 { // the cursor at the preedit's start, x 140
		t.Errorf("scroll with the cursor at the preedit's start %v", got)
	}
	sf.Preedit("jumped", 6)
	paint(t, s, sf)
	if got := tf.view.ScrollX(); got != 182-92 { // at its end, x 182
		t.Errorf("scroll with the cursor at the preedit's end %v", got)
	}
}

// TestPreeditUnderlines: the clauses are underlined below the baseline in
// the text color, the selected one thicker, each a gap short of its end;
// no clauses underline the whole preedit; the same rects at scale 2.
func TestPreeditUnderlines(t *testing.T) {
	for _, c := range []struct {
		scale float64
		segs  []shell.PreeditSegment
		want  []string
	}{
		{1, nil, []string{"fillRRect (21,54,20,1) solid #000000"}},
		{1, []shell.PreeditSegment{{Start: 0, End: 3}, {Start: 3, End: 9, Selected: true}}, []string{
			"fillRRect (21,54,6,1) solid #000000", "fillRRect (28,54,13,2) solid #000000",
		}},
		{2, []shell.PreeditSegment{{Start: 0, End: 3}, {Start: 3, End: 9, Selected: true}}, []string{
			"fillRRect (21,54,6,1) solid #000000", "fillRRect (28,54,13,2) solid #000000",
		}},
	} {
		tf, _, sf, s := imeField(t, false, nil)
		sf.SetScale(c.scale)
		tf.SetText("a")
		sf.Preedit("にほん", 9, c.segs...)
		got := fieldLines(paint(t, s, sf))
		for _, want := range c.want {
			if !strings.Contains(got, want) {
				t.Errorf("scale %v, %v: no %q in\n%s", c.scale, c.segs, want, got)
			}
		}
		if n := strings.Count(got, "solid #000000"); n != len(c.want)+1 { // the text too
			t.Errorf("scale %v, %v: %d text-colored fills\n%s", c.scale, c.segs, n, fmt.Sprint(got))
		}
	}
}
