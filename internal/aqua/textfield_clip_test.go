package aqua

import (
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell/fake"
)

// Copy, Cut and Paste in TextField and SecureField (PLAN.md S5.2i),
// through the fake shell's clipboard: reads are delivered on Step.

// TestTextFieldCopyCut: Copy puts the selection on the clipboard, Cut
// also deletes it (one undo group, TextChanged); an empty selection
// leaves the clipboard alone.
func TestTextFieldCopyCut(t *testing.T) {
	h := &fieldLog{}
	tf, _, sf, s := focusedField(t, false, h)
	sf.Type("hello world")
	h.events = nil
	s.SetClipboard("old", true)

	tf.Editor().SetSelection(5, 5)
	sf.Key('c', ctrl)
	sf.Key('x', ctrl)
	if clipText(s) != "old" || tf.Text() != "hello world" || len(h.events) != 0 {
		t.Errorf("empty selection: clip %q text %q events %q", clipText(s), tf.Text(), h.events)
	}

	tf.Editor().SetSelection(11, 6) // "world", backwards
	sf.Key('c', ctrl)
	if clipText(s) != "world" || selOf(tf) != "hello world|11,6" || len(h.events) != 0 {
		t.Errorf("copy: clip %q %s %q", clipText(s), selOf(tf), h.events)
	}
	tf.Editor().SetSelection(0, 6)
	sf.Key('x', ctrl)
	if clipText(s) != "hello " || selOf(tf) != "world|0,0" || strings.Join(h.events, ";") != "changed world" {
		t.Errorf("cut: clip %q %s %q", clipText(s), selOf(tf), h.events)
	}
	sf.Key('z', ctrl)
	if selOf(tf) != "hello world|0,6" {
		t.Errorf("undo of the cut: %s", selOf(tf))
	}
	if s.ClipboardReads() != 0 {
		t.Error("copy or cut read the clipboard")
	}
}

// TestTextFieldPaste: Paste replaces the selection with the clipboard's
// text once the read arrives, newlines (and Windows line breaks) as
// spaces, as an undo group of its own; no text, or an empty one, changes
// nothing.
func TestTextFieldPaste(t *testing.T) {
	h := &fieldLog{}
	tf, _, sf, s := focusedField(t, false, h)
	sf.Type("ab")
	tf.Editor().SetSelection(1, 1)
	h.events = nil

	s.SetClipboard("one\r\ntwo\nthree", true)
	sf.Key('v', ctrl)
	if selOf(tf) != "ab|1,1" || s.ClipboardReads() != 1 {
		t.Fatalf("paste applied before the read arrived: %s", selOf(tf))
	}
	s.Step()
	const want = "aone two threeb"
	if got := selOf(tf); got != want+"|14,14" || strings.Join(h.events, ";") != "changed "+want || tf.Text() != want {
		t.Errorf("paste: %s %q", got, h.events)
	}
	// Over a selection; the paste is its own undo group, after typing.
	sf.Type("x")
	tf.Editor().SetSelection(0, 4)
	s.SetClipboard("Z", true)
	sf.Key('v', ctrl)
	s.Step()
	if tf.Text() != "Z two threexb" {
		t.Errorf("paste over a selection: %q", tf.Text())
	}
	sf.Key('z', ctrl)
	if tf.Text() != "aone two threexb" {
		t.Errorf("undo of the paste: %q", tf.Text())
	}

	h.events = nil
	for _, c := range []struct {
		text string
		ok   bool
	}{{"", false}, {"", true}} {
		s.SetClipboard(c.text, c.ok)
		sf.Key('v', ctrl)
		s.Step()
	}
	if tf.Text() != "aone two threexb" || len(h.events) != 0 {
		t.Errorf("paste of no text: %q %q", tf.Text(), h.events)
	}

	// Two pastes in flight both land, in order.
	tf.Editor().SetSelection(0, 0)
	s.SetClipboard("1", true)
	sf.Key('v', ctrl)
	s.SetClipboard("2", true)
	sf.Key('v', ctrl)
	s.Step()
	if !strings.HasPrefix(tf.Text(), "12a") {
		t.Errorf("two pastes: %q", tf.Text())
	}
}

// TestTextFieldPasteLate: a paste whose read arrives after the field lost
// the focus (even if it has it back), was disabled or made read-only, or
// whose window closed, is dropped.
func TestTextFieldPasteLate(t *testing.T) {
	for _, c := range []struct {
		name string
		then func(tf *TextField, w *Window, sf *fake.Surface)
	}{
		{"blur", func(tf *TextField, w *Window, _ *fake.Surface) { w.Router().SetFocus(nil) }},
		{"refocus", func(tf *TextField, w *Window, _ *fake.Surface) {
			w.Router().SetFocus(nil)
			tf.view.Focus()
		}},
		{"disabled", func(tf *TextField, _ *Window, _ *fake.Surface) { tf.SetEnabled(false) }},
		{"read-only", func(tf *TextField, _ *Window, _ *fake.Surface) { tf.SetEditable(false) }},
		{"closed", func(_ *TextField, _ *Window, sf *fake.Surface) { sf.Close() }},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := &fieldLog{}
			tf, w, sf, s := focusedField(t, false, h)
			s.SetClipboard("late", true)
			sf.Key('v', ctrl)
			c.then(tf, w, sf)
			s.Step()
			if tf.Editor().Text() != "" || len(h.events) != 0 {
				t.Errorf("late paste applied: %q %q", tf.Editor().Text(), h.events)
			}
		})
	}
	// A key window change alone does not drop it: the field kept the focus.
	h := &fieldLog{}
	tf, _, sf, s := focusedField(t, false, h)
	s.SetClipboard("kept", true)
	sf.Key('v', ctrl)
	s.Deactivate()
	s.Step()
	if tf.Text() != "kept" {
		t.Errorf("paste after deactivation: %q", tf.Text())
	}
}

// TestSecureFieldClipboard: a secure field copies and cuts nothing (the
// keys are still taken) but pastes, its newlines as spaces.
func TestSecureFieldClipboard(t *testing.T) {
	h := &fieldLog{}
	tf, _, sf, s := focusedField(t, true, h)
	sf.Type("secret")
	h.events = nil
	s.SetClipboard("old", true)
	tf.Editor().SelectAll()
	if !sf.Key('c', ctrl) || !sf.Key('x', ctrl) {
		t.Error("Copy or Cut not taken")
	}
	if clipText(s) != "old" || selOf(tf) != "secret|0,6" || len(h.events) != 0 {
		t.Errorf("secure copy/cut: clip %q %s %q", clipText(s), selOf(tf), h.events)
	}
	s.SetClipboard("pass\nword", true)
	sf.Key('v', ctrl)
	s.Step()
	if selOf(tf) != "pass word|9,9" || strings.Join(h.events, ";") != "changed pass word" {
		t.Errorf("secure paste: %s %q", selOf(tf), h.events)
	}
	if n := tf.view.AccessibleNode(); n.Value != "" {
		t.Errorf("secure a11y value %q", n.Value)
	}
}

// TestReadOnlyFieldClipboard: a read-only field copies; Cut and Paste
// change nothing and Paste does not even read the clipboard.
func TestReadOnlyFieldClipboard(t *testing.T) {
	h := &fieldLog{}
	tf, _, sf, s := focusedField(t, false, h)
	tf.SetText("fixed text")
	tf.SetEditable(false)
	tf.Editor().SetSelection(0, 5)
	s.SetClipboard("old", true)
	if !sf.Key('c', ctrl) || clipText(s) != "fixed" {
		t.Errorf("read-only copy: clip %q", clipText(s))
	}
	s.SetClipboard("new", true)
	for _, k := range []driver.Key{'x', 'v'} {
		if !sf.Key(k, ctrl) {
			t.Errorf("Primary-%c not taken", k)
		}
	}
	s.Step()
	if tf.Text() != "fixed text" || clipText(s) != "new" || s.ClipboardReads() != 0 || len(h.events) != 0 {
		t.Errorf("read-only cut/paste: %q clip %q reads %d %q", tf.Text(), clipText(s), s.ClipboardReads(), h.events)
	}
}
