package aqua

import (
	"slices"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// The caret blink of TextField (PLAN.md S5.2i) over Shell.After, driven
// by the fake clock. The fake shell's appearance has the default blink: a
// 1 s cycle (500 ms per half) that stops after 10 s.

const halfBlink = 500 * time.Millisecond

// blinkTimers reports the field's pending blink as the deadlines of the
// shell's timers relative to Now (the fields' timers are the only ones in
// these tests).
func blinkTimers(s *fake.Shell) []time.Duration {
	now := s.FakeClock().Now()
	var out []time.Duration
	for _, at := range s.Timers() {
		out = append(out, at-now)
	}
	return out
}

// wantBlink checks the caret's visibility and that exactly one timer is
// pending (at d from Now) or none (d < 0).
func wantBlink(t *testing.T, what string, tf *TextField, s *fake.Shell, visible bool, d time.Duration) {
	t.Helper()
	timers := blinkTimers(s)
	var want []time.Duration
	if d >= 0 {
		want = []time.Duration{d}
	}
	if tf.view.CaretVisible() != visible || !slices.Equal(timers, want) {
		t.Errorf("%s: caret visible %v, timers %v; want %v, %v", what, tf.view.CaretVisible(), timers, visible, want)
	}
}

// TestCaretBlink: the caret blinks by half periods from the focus, each
// phase repaints (the dump loses and regains the caret), and after the
// timeout it stays solid with no timer left.
func TestCaretBlink(t *testing.T) {
	tf, _, sf, s := focusedField(t, false, nil)
	clk := s.FakeClock()
	clk.Flush()
	wantBlink(t, "focused", tf, s, true, halfBlink)
	on := sf.Dump()

	paints := sf.Paints()
	clk.Advance(halfBlink)
	clk.Flush()
	wantBlink(t, "first half", tf, s, false, halfBlink)
	off := sf.Dump()
	if sf.Paints() == paints || off == on {
		t.Fatalf("the hidden phase did not repaint without the caret (%d paints)", sf.Paints()-paints)
	}
	clk.Advance(halfBlink)
	clk.Flush()
	wantBlink(t, "second half", tf, s, true, halfBlink)
	if sf.Dump() != on {
		t.Error("the shown phase differs from the first paint")
	}

	// 10 s after the focus: 19 flips so far, then solid.
	clk.Advance(9*time.Second - time.Millisecond)
	wantBlink(t, "just before the timeout", tf, s, false, time.Millisecond)
	clk.Advance(time.Millisecond)
	clk.Flush()
	wantBlink(t, "timeout", tf, s, true, -1)
	if sf.Dump() != on {
		t.Error("the solid caret differs from the first paint")
	}
	if n := clk.Advance(time.Minute); n != 0 || clk.Requested() {
		t.Errorf("%d frames after the timeout", n)
	}
}

// TestCaretBlinkRestart: typing, a move, a click, SetText and a focus
// restart the blink: the caret shows at once and the timeout counts from
// there.
func TestCaretBlinkRestart(t *testing.T) {
	tf, w, sf, s := focusedField(t, false, nil)
	clk := s.FakeClock()
	sf.Type("abc")
	for _, c := range []struct {
		name string
		do   func()
	}{
		{"typing", func() { sf.Type("d") }},
		{"move", func() { sf.Key(driver.KeyLeft, 0) }},
		{"undo", func() { sf.Key('z', ctrl) }},
		{"click", func() { sf.Click(driver.Point{X: 20, Y: 49}) }},
		{"SetText", func() { tf.SetText("new") }},
		{"refocus", func() { w.Router().SetFocus(nil); tf.view.Focus() }},
		{"key again", func() { s.Deactivate(); sf.Activate() }},
	} {
		clk.Advance(9*time.Second + halfBlink) // hidden, the timeout near
		if tf.view.CaretVisible() {
			t.Fatalf("%s: caret shown before", c.name)
		}
		c.do()
		wantBlink(t, c.name, tf, s, true, halfBlink)
		clk.Advance(10*time.Second - time.Millisecond)
		if len(s.Timers()) != 1 {
			t.Errorf("%s: the timeout did not restart", c.name)
		}
		clk.Advance(time.Millisecond)
		wantBlink(t, c.name+" timeout", tf, s, true, -1)
		c.do() // blinking again for the next case
	}
}

// TestCaretBlinkIdle: no timer runs and the caret does not blink while
// the field is unfocused, has a selection, is read-only or disabled, or
// its window is not key; the timer comes back with the caret.
func TestCaretBlinkIdle(t *testing.T) {
	tf, w, sf, s := focusedField(t, false, nil)
	sf.Type("abc")
	for _, c := range []struct {
		name      string
		stop, run func()
	}{
		{"blur", func() { w.Router().SetFocus(nil) }, func() { tf.view.Focus() }},
		{"selection", func() { sf.Key(driver.KeyLeft, shf) }, func() { sf.Key(driver.KeyRight, 0) }},
		{"select all", func() { sf.Key('a', ctrl) }, func() { sf.Key(driver.KeyRight, 0) }},
		{"read-only", func() { tf.SetEditable(false) }, func() { tf.SetEditable(true) }},
		{"inactive", func() { s.Deactivate() }, func() { sf.Activate() }},
		{"disabled", func() { tf.SetEnabled(false) }, func() { tf.SetEnabled(true); tf.view.Focus() }},
	} {
		s.FakeClock().Advance(halfBlink) // hidden
		c.stop()
		wantBlink(t, c.name, tf, s, false, -1)
		if n := s.FakeClock().Advance(time.Minute); n > 1 {
			t.Errorf("%s: %d frames while idle", c.name, n)
		}
		c.run()
		wantBlink(t, c.name+" back", tf, s, true, halfBlink)
	}
}

// TestCaretBlinkSetting: the window's CaretBlink rules: no period, no
// blink; no timeout, blinking on; a new setting restarts the blink.
func TestCaretBlinkSetting(t *testing.T) {
	tf, w, _, s := focusedField(t, false, nil)
	clk := s.FakeClock()
	a := w.Appearance()

	a.CaretBlink = driver.CaretBlink{}
	w.SetAppearance(a)
	wantBlink(t, "off", tf, s, true, -1)
	if n := clk.Advance(time.Minute); n > 1 {
		t.Errorf("%d frames without a blink", n)
	}

	a.CaretBlink = driver.CaretBlink{Period: 200 * time.Millisecond}
	w.SetAppearance(a)
	wantBlink(t, "fast", tf, s, true, 100*time.Millisecond)
	clk.Advance(time.Hour + 100*time.Millisecond)
	wantBlink(t, "no timeout", tf, s, false, 100*time.Millisecond)

	a.CaretBlink = driver.CaretBlink{Period: time.Second, Timeout: 2 * time.Second}
	w.SetAppearance(a)
	wantBlink(t, "changed", tf, s, true, halfBlink)
	clk.Advance(2 * time.Second)
	wantBlink(t, "short timeout", tf, s, true, -1)
}

// TestCaretBlinkSecureAndClose: a secure field blinks like a plain one;
// a window closing with a blinking field leaves no timer once the pending
// one has fired.
func TestCaretBlinkSecureAndClose(t *testing.T) {
	tf, _, sf, s := focusedField(t, true, nil)
	sf.Type("pw")
	clk := s.FakeClock()
	clk.Advance(halfBlink)
	wantBlink(t, "secure", tf, s, false, halfBlink)
	sf.Close()
	clk.Advance(halfBlink)
	wantBlink(t, "closed", tf, s, false, -1)
}

// TestCaretBlinkNoCursorChange: the blink leaves the pointer alone and
// fires no field events (it is paint only).
func TestCaretBlinkNoCursorChange(t *testing.T) {
	h := &fieldLog{}
	tf, _, sf, s := focusedField(t, false, h)
	sf.Move(driver.Point{X: 20, Y: 49})
	if sf.Cursor() != shell.CursorIBeam {
		t.Fatalf("cursor %v", sf.Cursor())
	}
	s.FakeClock().Advance(3 * time.Second)
	if sf.Cursor() != shell.CursorIBeam || len(h.events) != 0 || tf.Text() != "" {
		t.Errorf("blink: cursor %v, events %q", sf.Cursor(), h.events)
	}
}
