package aqua

import (
	"fmt"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// Keyboard editing of TextField and SecureField (PLAN.md S5.2g), through
// the fake shell's key and text events.

const (
	alt  = driver.ModOption
	ctrl = driver.ModControl // Primary on Windows and Linux
	shf  = driver.ModShift
)

// focusedField is fieldWindow with the field focused by a click-like
// Focus (the selection kept).
func focusedField(t *testing.T, secure bool, h driver.TextFieldHandler) (*TextField, *Window, *fake.Surface, *fake.Shell) {
	t.Helper()
	tf, w, sf, s := fieldWindow(t, secure, h)
	if !tf.view.Focus() {
		t.Fatal("field not focused")
	}
	return tf, w, sf, s
}

// selOf returns the editor's text and selection as "text|anchor,head".
func selOf(tf *TextField) string {
	ed := tf.Editor()
	return fmt.Sprintf("%s|%d,%d", ed.Text(), ed.Anchor(), ed.Head())
}

// TestTextFieldTyping: typed text and commits are inserted at the caret,
// replace the selection, fire TextChanged per edit, keep newlines out, and
// the stub's Text follows.
func TestTextFieldTyping(t *testing.T) {
	h := &fieldLog{}
	tf, _, sf, _ := focusedField(t, false, h)
	sf.Type("Hi you")
	if got := selOf(tf); got != "Hi you|6,6" || tf.Text() != "Hi you" {
		t.Errorf("typed %s, Text %q", got, tf.Text())
	}
	if len(h.events) != 6 || h.events[0] != "changed H" || h.events[5] != "changed Hi you" {
		t.Errorf("events %q", h.events)
	}
	h.events = nil
	tf.Editor().SetSelection(3, 6)
	sf.Commit("allé")
	if got := selOf(tf); got != "Hi allé|8,8" || strings.Join(h.events, ";") != "changed Hi allé" {
		t.Errorf("commit over a selection: %s %q", got, h.events)
	}
	// Return and Tab are keys, never inserted; text holding a newline
	// (a paste from an input method) is joined with spaces.
	sf.Send(shell.TextEvent{Text: "a\nb"})
	if got := tf.Editor().Text(); got != "Hi alléa b" {
		t.Errorf("newline in a text event: %q", got)
	}
}

// TestTextFieldMoves: every move unit of the single-line keymap, with and
// without Shift.
func TestTextFieldMoves(t *testing.T) {
	tf, _, sf, _ := focusedField(t, false, nil)
	const s = "one two three" // words at 0–3, 4–7, 8–13
	for _, c := range []struct {
		anchor, head int
		key          driver.Key
		mods         driver.Modifiers
		want         string
	}{
		{5, 5, driver.KeyLeft, 0, "4,4"},
		{5, 5, driver.KeyRight, 0, "6,6"},
		{5, 5, driver.KeyLeft, shf, "5,4"},
		{5, 5, driver.KeyRight, shf, "5,6"},
		{4, 7, driver.KeyLeft, 0, "4,4"}, // a selection collapses to its start
		{4, 7, driver.KeyRight, 0, "7,7"},
		{5, 5, driver.KeyLeft, alt, "4,4"},
		{5, 5, driver.KeyRight, alt, "7,7"},
		{7, 7, driver.KeyRight, alt, "13,13"},
		{5, 5, driver.KeyLeft, alt | shf, "5,4"},
		{5, 5, driver.KeyRight, alt | shf, "5,7"},
		{5, 5, driver.KeyLeft, ctrl, "0,0"},
		{5, 5, driver.KeyRight, ctrl, "13,13"},
		{5, 5, driver.KeyLeft, ctrl | shf, "5,0"},
		{5, 5, driver.KeyRight, ctrl | shf, "5,13"},
		{5, 5, driver.KeyUp, 0, "0,0"}, // one line: to the ends
		{5, 5, driver.KeyDown, 0, "13,13"},
		{5, 5, driver.KeyUp, shf, "5,0"},
		{5, 5, driver.KeyDown, shf, "5,13"},
		{5, 5, driver.KeyUp, alt, "0,0"},
		{5, 5, driver.KeyDown, alt, "13,13"},
		{5, 5, driver.KeyUp, ctrl, "0,0"},
		{5, 5, driver.KeyDown, ctrl, "13,13"},
		{5, 5, driver.KeyHome, 0, "0,0"},
		{5, 5, driver.KeyEnd, 0, "13,13"},
		{5, 5, driver.KeyHome, shf, "5,0"},
		{5, 5, driver.KeyEnd, shf, "5,13"},
		{5, 5, driver.KeyPageUp, 0, "0,0"},
		{5, 5, driver.KeyPageDown, 0, "13,13"},
		{5, 5, driver.KeyPageDown, shf, "5,13"},
		{2, 9, 'a', ctrl, "0,13"},
	} {
		tf.SetText(s)
		tf.Editor().SetSelection(c.anchor, c.head)
		if !sf.Key(c.key, c.mods) {
			t.Errorf("%v %v from %d,%d not taken", c.key, c.mods, c.anchor, c.head)
		}
		if got := selOf(tf); got != s+"|"+c.want {
			t.Errorf("%v %v from %d,%d: %s, want %s", c.key, c.mods, c.anchor, c.head, got, c.want)
		}
	}
	// A repeat moves like a press.
	tf.SetText(s)
	sf.KeyRepeat(driver.KeyLeft, 0)
	sf.KeyRepeat(driver.KeyLeft, 0)
	if got := selOf(tf); got != s+"|11,11" {
		t.Errorf("repeats: %s", got)
	}
}

// TestTextFieldDeletes: the deletes by cluster, word and line, the
// selection deleted whole, and no TextChanged when nothing went.
func TestTextFieldDeletes(t *testing.T) {
	h := &fieldLog{}
	tf, _, sf, _ := focusedField(t, false, h)
	const s = "one two three"
	for _, c := range []struct {
		anchor, head int
		key          driver.Key
		mods         driver.Modifiers
		want         string
	}{
		{5, 5, driver.KeyBackspace, 0, "one wo three|4,4"},
		{5, 5, driver.KeyBackspace, shf, "one wo three|4,4"},
		{5, 5, driver.KeyDelete, 0, "one to three|5,5"},
		{5, 5, driver.KeyBackspace, alt, "one wo three|4,4"},
		{7, 7, driver.KeyBackspace, alt, "one  three|4,4"},
		{5, 5, driver.KeyDelete, alt, "one t three|5,5"},
		{5, 5, driver.KeyBackspace, ctrl, "wo three|0,0"},
		{5, 5, driver.KeyDelete, ctrl, "one t|5,5"},
		{4, 8, driver.KeyBackspace, 0, "one three|4,4"},
		{4, 8, driver.KeyDelete, alt, "one three|4,4"},
	} {
		tf.SetText(s)
		tf.Editor().SetSelection(c.anchor, c.head)
		h.events = nil
		if !sf.Key(c.key, c.mods) {
			t.Errorf("%v %v not taken", c.key, c.mods)
		}
		if got := selOf(tf); got != c.want {
			t.Errorf("%v %v from %d,%d: %s, want %s", c.key, c.mods, c.anchor, c.head, got, c.want)
		}
		if want := "changed " + strings.SplitN(c.want, "|", 2)[0]; strings.Join(h.events, ";") != want || tf.Text() != tf.Editor().Text() {
			t.Errorf("%v %v: events %q, Text %q", c.key, c.mods, h.events, tf.Text())
		}
	}
	// Nothing to delete: taken, no event.
	tf.SetText(s)
	tf.Editor().SetSelection(0, 0)
	h.events = nil
	if !sf.Key(driver.KeyBackspace, 0) || !sf.Key(driver.KeyBackspace, alt) || len(h.events) != 0 {
		t.Errorf("delete at the start: events %q", h.events)
	}
	tf.Editor().SetSelection(13, 13)
	if !sf.Key(driver.KeyDelete, ctrl) || len(h.events) != 0 || tf.Text() != s {
		t.Errorf("delete at the end: events %q", h.events)
	}
}

// TestTextFieldUndo: Primary-Z and Shift-Primary-Z over coalesced typing
// and deletes, each with TextChanged; nothing to undo fires nothing.
func TestTextFieldUndo(t *testing.T) {
	h := &fieldLog{}
	tf, _, sf, _ := focusedField(t, false, h)
	sf.Type("one two")
	sf.Key(driver.KeyLeft, 0)
	sf.Type("x")
	sf.Key(driver.KeyBackspace, 0)
	sf.Key(driver.KeyBackspace, 0)
	if got := selOf(tf); got != "one to|5,5" {
		t.Fatalf("before undo: %s", got)
	}
	h.events = nil
	for _, c := range []struct {
		mods driver.Modifiers
		want string
	}{
		{ctrl, "one twxo|7,7"}, // the two deletes
		{ctrl, "one two|6,6"},  // the x
		{ctrl, "|0,0"},         // the typing run
		{ctrl, "|0,0"},         // nothing left
		{ctrl | shf, "one two|7,7"},
		{ctrl | shf, "one twxo|7,7"},
	} {
		if !sf.Key('z', c.mods) {
			t.Errorf("Z %v not taken", c.mods)
		}
		if got := selOf(tf); got != c.want {
			t.Errorf("Z %v: %s, want %s", c.mods, got, c.want)
		}
	}
	want := "changed one twxo;changed one two;changed ;changed one two;changed one twxo"
	if got := strings.Join(h.events, ";"); got != want {
		t.Errorf("events %q\nwant %q", got, want)
	}
	// SetText clears the history.
	tf.SetText("new")
	h.events = nil
	sf.Key('z', ctrl)
	if tf.Text() != "new" || len(h.events) != 0 {
		t.Errorf("undo after SetText: %q %q", tf.Text(), h.events)
	}
}

// TestTextFieldKeysGoOn: Return fires Submitted on a press and goes on,
// Tab, Escape and the character keys go on, the clipboard keys are taken
// (their effects in textfield_clip_test.go), a key-up is never taken.
func TestTextFieldKeysGoOn(t *testing.T) {
	h := &fieldLog{}
	tf, _, sf, s := focusedField(t, false, h)
	sf.Type("ab")
	h.events = nil
	if sf.Key(driver.KeyReturn, 0) {
		t.Error("Return taken")
	}
	sf.KeyRepeat(driver.KeyReturn, 0)
	sf.Key(driver.KeyReturn, shf)
	if strings.Join(h.events, ";") != "submitted ab" {
		t.Errorf("Return events %q", h.events)
	}
	for _, k := range []struct {
		key  driver.Key
		mods driver.Modifiers
	}{{driver.KeyEscape, 0}, {driver.KeySpace, 0}, {'q', 0}, {'Q', shf}, {'b', ctrl}, {'e', ctrl | alt}, {driver.KeyLeft, driver.ModCommand}} {
		if sf.KeyDown(k.key, k.mods) {
			t.Errorf("%v %v taken", k.key, k.mods)
		}
	}
	s.SetClipboard("clip", true)
	tf.Editor().SetSelection(0, 2)
	h.events = nil
	for _, k := range []driver.Key{'c', 'x', 'v'} {
		if !sf.Key(k, ctrl) {
			t.Errorf("Primary-%c not taken", k)
		}
	}
	// Copy and Cut act at once, Paste later (textfield_clip_test.go).
	if got := selOf(tf); got != "|0,0" || strings.Join(h.events, ";") != "changed " || clipText(s) != "ab" || s.ClipboardReads() != 1 {
		t.Errorf("clipboard keys: %s %q %q", got, h.events, clipText(s))
	}
	if sf.Send(shell.KeyEvent{Key: driver.KeyLeft}) {
		t.Error("key-up taken")
	}
}

// TestTextFieldTab: Tab and Shift-Tab move the focus on and a Tab focus
// selects all; a Focus keeps the selection.
func TestTextFieldTab(t *testing.T) {
	s := startShell(t)
	d := newDriver(s)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 300, H: 100}, Style: allStyles})
	a := d.NewTextField(false, nil).(*TextField)
	b := d.NewTextField(true, nil).(*TextField)
	c := d.NewContainer()
	c.SetChildren([]driver.Widget{a, b})
	w.SetContent(c)
	a.SetFrame(driver.Rect{X: 10, Y: 10, W: 100, H: 22})
	b.SetFrame(driver.Rect{X: 120, Y: 10, W: 100, H: 22})
	a.SetText("alpha")
	b.SetText("secret")
	a.Editor().SetSelection(1, 1)
	b.Editor().SetSelection(2, 2)
	w.Router().SetFocus(ElementOf(a))
	if got := selOf(a); got != "alpha|1,1" {
		t.Errorf("Focus changed the selection: %s", got)
	}
	if sf.Key(driver.KeyTab, 0) != true || !ElementOf(b).base().HasFocus() || selOf(b) != "secret|0,6" {
		t.Errorf("Tab: focus %v, %s", ElementOf(b).base().HasFocus(), selOf(b))
	}
	a.Editor().SetSelection(3, 3)
	if !sf.Key(driver.KeyTab, shf) || !ElementOf(a).base().HasFocus() || selOf(a) != "alpha|0,5" {
		t.Errorf("Shift-Tab: %s", selOf(a))
	}
	// Typing replaces the selected text.
	sf.Type("Z")
	if a.Text() != "Z" {
		t.Errorf("typing over the Tab selection: %q", a.Text())
	}
}

// TestTextFieldSecureKeys: a secure field types, deletes and undoes like
// a plain one, but its word moves and deletes take the whole text.
func TestTextFieldSecureKeys(t *testing.T) {
	h := &fieldLog{}
	tf, _, sf, _ := focusedField(t, true, h)
	sf.Type("pa ss")
	if got := selOf(tf); got != "pa ss|5,5" || len(h.events) != 5 {
		t.Errorf("typed %s %q", got, h.events)
	}
	for _, c := range []struct {
		anchor, head int
		key          driver.Key
		mods         driver.Modifiers
		want         string
	}{
		{4, 4, driver.KeyLeft, alt, "pa ss|0,0"},
		{1, 1, driver.KeyRight, alt, "pa ss|5,5"},
		{4, 4, driver.KeyLeft, alt | shf, "pa ss|4,0"},
		{1, 1, driver.KeyRight, alt | shf, "pa ss|1,5"},
		{4, 4, driver.KeyLeft, 0, "pa ss|3,3"},
		{4, 4, driver.KeyBackspace, alt, "s|0,0"},
		{1, 1, driver.KeyDelete, alt, "p|1,1"},
		{4, 4, driver.KeyBackspace, 0, "pa s|3,3"},
	} {
		tf.SetText("pa ss")
		tf.Editor().SetSelection(c.anchor, c.head)
		sf.Key(c.key, c.mods)
		if got := selOf(tf); got != c.want {
			t.Errorf("%v %v from %d,%d: %s, want %s", c.key, c.mods, c.anchor, c.head, got, c.want)
		}
	}
	sf.Key('z', ctrl)
	if got := selOf(tf); got != "pa ss|4,4" {
		t.Errorf("undo: %s", got)
	}
	// Copy and Cut do nothing (as from S5.2i on).
	tf.Editor().SelectAll()
	sf.Key('x', ctrl)
	if tf.Text() != "pa ss" {
		t.Errorf("cut in a secure field: %q", tf.Text())
	}
}

// TestTextFieldReadOnlyKeys: a read-only field moves and selects, takes
// the editing keys and text without changing anything, and fires nothing
// but Submitted.
func TestTextFieldReadOnlyKeys(t *testing.T) {
	h := &fieldLog{}
	tf, _, sf, _ := focusedField(t, false, h)
	sf.Type("one two")
	tf.SetEditable(false)
	h.events = nil
	sf.Key(driver.KeyLeft, alt)
	sf.Key(driver.KeyLeft, alt|shf)
	if got := selOf(tf); got != "one two|4,0" {
		t.Errorf("moves: %s", got)
	}
	for _, k := range []struct {
		key  driver.Key
		mods driver.Modifiers
	}{{driver.KeyBackspace, 0}, {driver.KeyDelete, alt}, {driver.KeyBackspace, ctrl}, {'z', ctrl}, {'z', ctrl | shf}, {'x', ctrl}, {'v', ctrl}} {
		if !sf.Key(k.key, k.mods) {
			t.Errorf("%v %v not taken", k.key, k.mods)
		}
	}
	sf.Type("q ")
	sf.Commit("w")
	if got := selOf(tf); got != "one two|4,0" || len(h.events) != 0 {
		t.Errorf("edits in a read-only field: %s %q", got, h.events)
	}
	sf.Key('a', ctrl)
	if got := selOf(tf); got != "one two|0,7" {
		t.Errorf("select all: %s", got)
	}
	sf.Key(driver.KeyReturn, 0)
	if strings.Join(h.events, ";") != "submitted one two" {
		t.Errorf("Return: %q", h.events)
	}
	// Editable again: the history from before is still there.
	tf.SetEditable(true)
	sf.Key('z', ctrl)
	if tf.Text() != "" {
		t.Errorf("undo after read-only: %q", tf.Text())
	}
}

// TestTextFieldDisabledKeys: a disabled field loses the focus and gets no
// keys or text; the events go to the root and change nothing.
func TestTextFieldDisabledKeys(t *testing.T) {
	h := &fieldLog{}
	tf, w, sf, _ := focusedField(t, false, h)
	sf.Type("ab")
	tf.SetEnabled(false)
	h.events = nil
	if w.Router().Focused() != nil || tf.view.Focus() {
		t.Error("a disabled field holds or takes the focus")
	}
	sf.Type("cd\n")
	sf.Key(driver.KeyBackspace, 0)
	// Sent to the element directly (as an enclosing handler might), it
	// still refuses.
	if tf.view.HandleEvent(KeyEvent{shell.KeyEvent{Down: true, Key: driver.KeyBackspace}}) ||
		tf.view.HandleEvent(TextEvent{shell.TextEvent{Text: "x"}}) {
		t.Error("a disabled field took an event")
	}
	tf.view.HandleEvent(KeyEvent{shell.KeyEvent{Down: true, Key: driver.KeyReturn}})
	if tf.Text() != "ab" || len(h.events) != 0 {
		t.Errorf("disabled: %q %q", tf.Text(), h.events)
	}
}

// TestTextFieldConverterKeys: a SetText from inside TextChanged (the
// converter example) wins over the typed text and does not loop.
func TestTextFieldConverterKeys(t *testing.T) {
	conv := &convField{}
	tf, _, sf, _ := focusedField(t, false, conv)
	conv.f = tf
	sf.Type("5")
	if tf.Text() != "5.0" || tf.Editor().Text() != "5.0" || conv.calls != 1 {
		t.Errorf("typed: %q %q, %d calls", tf.Text(), tf.Editor().Text(), conv.calls)
	}
	sf.Key(driver.KeyBackspace, 0)
	if tf.Text() != "5..0" || conv.calls != 2 {
		t.Errorf("deleted: %q, %d calls", tf.Text(), conv.calls)
	}
}

// TestTextFieldKeysRepaint: a move, an edit and a Tab focus repaint, and
// the caret is drawn where the editor has it.
func TestTextFieldKeysRepaint(t *testing.T) {
	tf, _, sf, s := focusedField(t, false, nil)
	sf.Type("abc")
	caret := func() string {
		for _, l := range strings.Split(paint(t, s, sf), "\n") {
			if strings.HasPrefix(l, "fillRRect") && strings.Contains(l, "#007aff") {
				return strings.SplitN(l, " ", 3)[1]
			}
		}
		return ""
	}
	end := caret()
	sf.Key(driver.KeyLeft, 0)
	if !sf.Invalid() {
		t.Error("a move did not invalidate")
	}
	if left := caret(); left == end || left == "" {
		t.Errorf("caret %q after ←, %q at the end", left, end)
	}
	sf.Key(driver.KeyBackspace, 0)
	if !sf.Invalid() || tf.Text() != "ac" {
		t.Errorf("a delete did not invalidate (%q)", tf.Text())
	}
}

func clipText(s *fake.Shell) string { t, _ := s.Clipboard(); return t }
