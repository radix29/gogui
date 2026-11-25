package aqua

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// The window publishing its text input (PLAN.md S5.3d), checked on the
// fake surface. fieldWindow's field is at surface (10, 38, 100, 22); with
// the fake metrics (7 pt per rune, 16 pt lines), the padding of 4 and a
// 2 pt caret, the caret before rune n is at (13 + 7n, 41, 2, 16).

// caretAt returns the published caret of a field at fieldWindow's frame
// before rune n, without scroll.
func caretAt(n int) shell.TextInput {
	return shell.TextInput{Enabled: true, Caret: driver.Rect{X: 13 + 7*float64(n), Y: 41, W: 2, H: 16}}
}

// tiChecker checks the surface's text input and that each step made at
// most the calls it expects (only changes are published).
type tiChecker struct {
	t    *testing.T
	sf   *fake.Surface
	sets int
}

// want checks that the text input is ti, set by exactly calls new
// SetTextInput calls since the last check.
func (c *tiChecker) want(what string, ti shell.TextInput, calls int) {
	c.t.Helper()
	if got := c.sf.TextInput(); got != ti {
		c.t.Errorf("%s: text input %+v, want %+v", what, got, ti)
	}
	if n := c.sf.TextInputSets() - c.sets; n != calls {
		c.t.Errorf("%s: %d SetTextInput calls, want %d", what, n, calls)
	}
	c.sets = c.sf.TextInputSets()
}

var tiOff = shell.TextInput{}

// TestTextInputFocusAndTyping: the IME is off without a focus, on with the
// caret at the field's caret when it takes the focus, follows typing and
// moves after the paint, and goes off on a blur; a paint or a focus event
// without a change sets nothing.
func TestTextInputFocusAndTyping(t *testing.T) {
	tf, w, sf, s := fieldWindow(t, false, nil)
	c := &tiChecker{t: t, sf: sf}
	c.want("no focus", tiOff, 0)

	tf.view.Focus()
	c.want("focused", caretAt(0), 1)
	if w.TextInput() != caretAt(0) {
		t.Errorf("Window.TextInput %+v", w.TextInput())
	}
	paint(t, s, sf)
	c.want("painted", caretAt(0), 0)

	sf.Type("abc")
	paint(t, s, sf)
	c.want("typed", caretAt(3), 1)
	sf.Key(driver.KeyLeft, 0)
	paint(t, s, sf)
	c.want("left", caretAt(2), 1)
	sf.Key(driver.KeyLeft, driver.ModShift) // a selection keeps the IME on, at the head
	paint(t, s, sf)
	c.want("selection", caretAt(1), 1)
	tf.view.Focus()
	paint(t, s, sf)
	c.want("focused again", caretAt(1), 0)

	w.Router().SetFocus(nil)
	c.want("blur", tiOff, 1)
	paint(t, s, sf)
	c.want("blur painted", tiOff, 0)
	tf.view.Focus()
	c.want("refocused", caretAt(1), 1)
}

// TestTextInputFocusMoves: moving the focus between fields moves the
// caret; a non-text focus, a SecureField, a read-only and a disabled
// field turn the IME off.
func TestTextInputFocusMoves(t *testing.T) {
	tf, w, sf, s := fieldWindow(t, false, nil)
	d := tf.view.d
	c := &tiChecker{t: t, sf: sf}
	other := d.NewTextField(false, nil).(*TextField)
	secure := d.NewTextField(true, nil).(*TextField)
	cb := d.NewCheckbox(nil).(*Checkbox)
	box := d.NewContainer().(*Container)
	box.SetFrame(driver.Rect{W: 200, H: 72})
	box.SetChildren([]driver.Widget{tf, other, secure, cb})
	w.SetContent(box)
	tf.SetFrame(driver.Rect{X: 10, Y: 10, W: 100, H: 22})
	other.SetFrame(driver.Rect{X: 10, Y: 40, W: 80, H: 22})
	secure.SetFrame(driver.Rect{X: 100, Y: 40, W: 80, H: 22})
	cb.SetFrame(driver.Rect{X: 120, Y: 10, W: 60, H: 20})
	paint(t, s, sf)
	c.sets = sf.TextInputSets()

	tf.view.Focus()
	c.want("first", caretAt(0), 1)
	other.SetText("xy")
	other.view.Focus()
	paint(t, s, sf)
	c.want("second", shell.TextInput{Enabled: true, Caret: driver.Rect{X: 13 + 14, Y: 71, W: 2, H: 16}}, 1)
	cb.view.Focus()
	c.want("checkbox", tiOff, 1)
	secure.view.Focus()
	c.want("secure", tiOff, 0)
	if sf.Send(shell.PreeditEvent{Text: "か", Cursor: 3}) {
		t.Error("the SecureField took a preedit")
	}

	tf.view.Focus()
	c.want("back", caretAt(0), 1)
	tf.SetEditable(false)
	paint(t, s, sf)
	c.want("read-only", tiOff, 1)
	tf.SetEditable(true)
	paint(t, s, sf)
	c.want("editable", caretAt(0), 1)
	tf.SetEnabled(false) // the field loses the focus
	c.want("disabled", tiOff, 1)
	if w.Router().Focused() != nil {
		t.Errorf("focus %v", w.Router().Focused())
	}
}

// TestTextInputKey: the IME is off while the window is not key and on
// again with the caret when it is; a composition is committed by the key
// loss before the IME goes off.
func TestTextInputKey(t *testing.T) {
	h := &fieldLog{}
	tf, w, sf, s := focusedField(t, false, h)
	c := &tiChecker{t: t, sf: sf}
	c.sets = sf.TextInputSets()
	tf.SetText("ab")
	paint(t, s, sf)
	c.want("typed", caretAt(2), 1)

	sf.Preedit("にほ", 3)
	paint(t, s, sf)
	c.want("composing", caretAt(3), 1) // the preedit's cursor, after に
	s.Deactivate()
	c.want("inactive", tiOff, 1)
	if tf.Text() != "abにほ" || sf.Composing() || sf.Cancels() != 1 {
		t.Errorf("key loss: %q, composing %v, %d cancels", tf.Text(), sf.Composing(), sf.Cancels())
	}
	paint(t, s, sf)
	c.want("inactive painted", tiOff, 0)
	sf.Activate()
	c.want("key again", caretAt(4), 1)
	if w.Router().Focused() != Element(tf.view) {
		t.Error("the focus moved")
	}
	paint(t, s, sf)
	c.want("key again painted", caretAt(4), 0)

	// Closing the window publishes nothing more.
	w.Close()
	if !w.Closed() || sf.TextInputSets() != c.sets {
		t.Errorf("close: closed %v, %d sets", w.Closed(), sf.TextInputSets()-c.sets)
	}
}

// TestTextInputScroll: the published caret follows the field's own
// scroll of overflowing text and the scroll view around the field.
func TestTextInputScroll(t *testing.T) {
	tf, _, sf, s := focusedField(t, false, nil)
	c := &tiChecker{t: t, sf: sf}
	c.sets = sf.TextInputSets()
	tf.SetText("The quick brown fox jumps") // 25 runes, 175 pt in a room of 92
	paint(t, s, sf)
	end := shell.TextInput{Enabled: true, Caret: driver.Rect{X: 10 + 4 + 92 - 1, Y: 41, W: 2, H: 16}}
	c.want("scrolled to the end", end, 1)
	if tf.view.ScrollX() != 175-92 {
		t.Errorf("scroll %v", tf.view.ScrollX())
	}
	tf.Editor().SetSelection(0, 0)
	tf.view.Invalidate()
	paint(t, s, sf)
	c.want("scrolled to the start", caretAt(0), 1)

	scrollRun(t, driver.Appearance{}, func(e *scrollEnv) {
		tf := e.d.NewTextField(false, nil).(*TextField)
		e.doc.AddChild(tf.view)
		tf.SetFrame(driver.Rect{X: 10, Y: 70, W: 100, H: 22})
		tf.view.Focus()
		e.settle()
		// The field at surface (20, 28+10+70); its caret 3 pt below the top.
		at := shell.TextInput{Enabled: true, Caret: driver.Rect{X: 20 + 3, Y: 108 + 3, W: 2, H: 16}}
		if got := e.sf.TextInput(); got != at {
			t.Errorf("in a scroll view: %+v, want %+v", got, at)
		}
		e.sv.SetOffset(driver.Point{Y: 50})
		e.settle()
		at.Caret.Y -= 50
		if got := e.sf.TextInput(); got != at {
			t.Errorf("scrolled by 50: %+v, want %+v", got, at)
		}
	})
}
