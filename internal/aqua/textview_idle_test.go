package aqua

import (
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell/fake"
)

// The TextView's clipboard, caret blink and idle layout pass (PLAN.md
// S5.4h) over the fake shell: clipboard reads are delivered on Step,
// timers fire on the fake clock's Advance.

// TestTextViewCopyCut: Copy puts the selection on the clipboard, newlines
// kept; Cut also deletes it as one undo group with TextChanged; an empty
// selection leaves the clipboard alone.
func TestTextViewCopyCut(t *testing.T) {
	h := &viewLog{}
	v, _, sf, s := focusedView(t, h)
	v.SetText("one two\nthree")
	s.SetClipboard("old", true)
	v.Editor().SetSelection(3, 3)
	if !sf.Key('c', ctrl) || !sf.Key('x', ctrl) {
		t.Error("Copy or Cut not taken")
	}
	if clipText(s) != "old" || h.changed != 0 {
		t.Errorf("empty selection: clip %q, %d changes", clipText(s), h.changed)
	}
	v.Editor().SetSelection(10, 4) // "two\nth", backwards
	sf.Key('c', ctrl)
	if clipText(s) != "two\nth" || viewSel(v) != "one two\nthree|10,4" || h.changed != 0 {
		t.Errorf("copy: clip %q %s", clipText(s), viewSel(v))
	}
	sf.Key('x', ctrl)
	if clipText(s) != "two\nth" || viewSel(v) != "one ree|4,4" || h.changed != 1 {
		t.Errorf("cut: clip %q %s, %d changes", clipText(s), viewSel(v), h.changed)
	}
	sf.Key('z', ctrl)
	if v.Text() != "one two\nthree" {
		t.Errorf("undo of the cut: %q", v.Text())
	}
	if s.ClipboardReads() != 0 {
		t.Error("copy or cut read the clipboard")
	}
}

// TestTextViewPaste: Paste replaces the selection once the read arrives,
// newlines kept and Windows line breaks normalized, as an undo group of
// its own, and scrolls the caret into view; no text changes nothing.
func TestTextViewPaste(t *testing.T) {
	h := &viewLog{}
	v, _, sf, s := focusedView(t, h)
	v.SetText("ab")
	v.Editor().SetSelection(1, 1)
	s.SetClipboard("one\r\ntwo\nthree", true)
	sf.Key('v', ctrl)
	if viewSel(v) != "ab|1,1" || s.ClipboardReads() != 1 {
		t.Fatalf("paste applied before the read arrived: %s", viewSel(v))
	}
	s.Step()
	if got := viewSel(v); got != "aone\ntwo\nthreeb|14,14" || h.changed != 1 {
		t.Errorf("paste: %s, %d changes", got, h.changed)
	}
	sf.Type("x")
	sf.Key('z', ctrl)
	sf.Key('z', ctrl)
	if v.Text() != "ab" {
		t.Errorf("undo of the paste: %q", v.Text())
	}

	// A long paste scrolls its end into view.
	v.Editor().SetSelection(2, 2)
	s.SetClipboard(numberedText(40), true)
	sf.Key('v', ctrl)
	s.Step()
	if got, want := v.Offset().Y, v.sv.DocumentSize.H-80; got != want || want <= 0 {
		t.Errorf("long paste: offset %v, want %v", got, want)
	}

	h.changed = 0
	for _, ok := range []bool{false, true} {
		s.SetClipboard("", ok)
		sf.Key('v', ctrl)
		s.Step()
	}
	if h.changed != 0 {
		t.Errorf("paste of no text: %d changes", h.changed)
	}
}

// TestTextViewPasteLate: a paste whose read arrives after the view lost
// the focus (even if it has it back), was disabled or made read-only, or
// whose window closed, is dropped; one across a key window change lands.
func TestTextViewPasteLate(t *testing.T) {
	for _, c := range []struct {
		name string
		then func(v *TextView, w *Window, sf *fake.Surface)
	}{
		{"blur", func(_ *TextView, w *Window, _ *fake.Surface) { w.Router().SetFocus(nil) }},
		{"refocus", func(v *TextView, w *Window, _ *fake.Surface) {
			w.Router().SetFocus(nil)
			v.view.Focus()
		}},
		{"disabled", func(v *TextView, _ *Window, _ *fake.Surface) { v.SetEnabled(false) }},
		{"read-only", func(v *TextView, _ *Window, _ *fake.Surface) { v.SetEditable(false) }},
		{"closed", func(_ *TextView, _ *Window, sf *fake.Surface) { sf.Close() }},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := &viewLog{}
			v, w, sf, s := focusedView(t, h)
			s.SetClipboard("late", true)
			sf.Key('v', ctrl)
			c.then(v, w, sf)
			s.Step()
			if v.Text() != "" || h.changed != 0 {
				t.Errorf("late paste applied: %q, %d changes", v.Text(), h.changed)
			}
		})
	}
	v, _, sf, s := focusedView(t, nil)
	s.SetClipboard("kept", true)
	sf.Key('v', ctrl)
	s.Deactivate()
	s.Step()
	if v.Text() != "kept" {
		t.Errorf("paste after deactivation: %q", v.Text())
	}
}

// TestTextViewReadOnlyClipboard: a read-only view copies; Cut and Paste
// are taken, change nothing and Paste does not read the clipboard.
func TestTextViewReadOnlyClipboard(t *testing.T) {
	h := &viewLog{}
	v, _, sf, s := focusedView(t, h)
	v.SetText("fixed\ntext")
	v.SetEditable(false)
	v.Editor().SetSelection(3, 8)
	sf.Key('c', ctrl)
	if clipText(s) != "ed\nte" {
		t.Errorf("read-only copy: clip %q", clipText(s))
	}
	s.SetClipboard("new", true)
	for _, k := range []driver.Key{'x', 'v'} {
		if !sf.Key(k, ctrl) {
			t.Errorf("Primary-%c not taken", k)
		}
	}
	s.Step()
	if v.Text() != "fixed\ntext" || clipText(s) != "new" || s.ClipboardReads() != 0 || h.changed != 0 {
		t.Errorf("read-only cut/paste: %q clip %q reads %d, %d changes", v.Text(), clipText(s), s.ClipboardReads(), h.changed)
	}
}

// wantViewBlink checks the caret's visibility and the pending timers: one
// at d from Now, or none (d < 0). The view's text is all laid out, so no
// idle layout timer runs.
func wantViewBlink(t *testing.T, what string, v *TextView, s *fake.Shell, visible bool, d time.Duration) {
	t.Helper()
	var want []time.Duration
	if d >= 0 {
		want = []time.Duration{d}
	}
	if timers := blinkTimers(s); v.view.CaretVisible() != visible || !slices.Equal(timers, want) {
		t.Errorf("%s: caret visible %v, timers %v; want %v, %v", what, v.view.CaretVisible(), timers, visible, want)
	}
}

// TestTextViewCaretBlink: the caret blinks by half periods from the focus
// and each phase repaints the caret away and back; after the timeout it
// stays solid with no timer left.
func TestTextViewCaretBlink(t *testing.T) {
	v, _, sf, s := focusedView(t, nil)
	v.SetText("one\ntwo")
	clk := s.FakeClock()
	clk.Flush()
	wantViewBlink(t, "focused", v, s, true, halfBlink)
	on := sf.Dump()
	clk.Advance(halfBlink)
	clk.Flush()
	wantViewBlink(t, "first half", v, s, false, halfBlink)
	if off := sf.Dump(); off == on || strings.Contains(off, "#007aff clip") {
		t.Errorf("the hidden phase still shows the caret:\n%s", off)
	}
	clk.Advance(halfBlink)
	clk.Flush()
	if sf.Dump() != on {
		t.Error("the shown phase differs from the first paint")
	}
	clk.Advance(10 * time.Second)
	clk.Flush()
	wantViewBlink(t, "timeout", v, s, true, -1)
	if n := clk.Advance(time.Minute); n != 0 || clk.Requested() {
		t.Errorf("%d frames after the timeout", n)
	}
}

// TestTextViewCaretBlinkRestart: the keys, typing, a paste, SetText,
// SetSelection, a refocus and the window becoming key again show the
// caret and restart the blink.
func TestTextViewCaretBlinkRestart(t *testing.T) {
	v, w, sf, s := focusedView(t, nil)
	clk := s.FakeClock()
	sf.Type("abc\ndef")
	for _, c := range []struct {
		name string
		do   func()
	}{
		{"typing", func() { sf.Type("g") }},
		{"move", func() { sf.Key(driver.KeyUp, 0) }},
		{"return", func() { sf.Key(driver.KeyReturn, 0) }},
		{"paste", func() { s.SetClipboard("p", true); sf.Key('v', ctrl); s.Step() }},
		{"SetText", func() { v.SetText("new\ntext") }},
		{"SetSelection", func() { v.SetSelection(2, 2) }},
		{"refocus", func() { w.Router().SetFocus(nil); v.view.Focus() }},
		{"key again", func() { s.Deactivate(); sf.Activate() }},
	} {
		clk.Advance(9*time.Second + halfBlink) // hidden, the timeout near
		if v.view.CaretVisible() {
			t.Fatalf("%s: caret shown before", c.name)
		}
		c.do()
		wantViewBlink(t, c.name, v, s, true, halfBlink)
		clk.Advance(10 * time.Second)
		wantViewBlink(t, c.name+" timeout", v, s, true, -1)
		c.do()
	}
}

// TestTextViewCaretBlinkIdle: no blink timer while the view is unfocused,
// has a selection, is read-only or disabled, or its window is not key;
// it comes back with the caret.
func TestTextViewCaretBlinkIdle(t *testing.T) {
	v, w, sf, s := focusedView(t, nil)
	sf.Type("abc")
	for _, c := range []struct {
		name      string
		stop, run func()
	}{
		{"blur", func() { w.Router().SetFocus(nil) }, func() { v.view.Focus() }},
		{"selection", func() { sf.Key(driver.KeyLeft, shf) }, func() { sf.Key(driver.KeyRight, 0) }},
		{"SetSelection", func() { v.SetSelection(0, 2) }, func() { v.SetSelection(1, 1) }},
		{"read-only", func() { v.SetEditable(false) }, func() { v.SetEditable(true) }},
		{"inactive", func() { s.Deactivate() }, func() { sf.Activate() }},
		{"disabled", func() { v.SetEnabled(false) }, func() { v.SetEnabled(true); v.view.Focus() }},
	} {
		s.FakeClock().Advance(halfBlink) // hidden
		c.stop()
		wantViewBlink(t, c.name, v, s, false, -1)
		if n := s.FakeClock().Advance(time.Minute); n > 1 {
			t.Errorf("%s: %d frames while idle", c.name, n)
		}
		c.run()
		wantViewBlink(t, c.name+" back", v, s, true, halfBlink)
	}
	sf.Close()
	s.FakeClock().Advance(time.Minute)
	wantViewBlink(t, "closed", v, s, false, -1)
}

// textOps returns the text draws of a dump: the visible lines and where
// they are.
func textOps(dump string) []string {
	var out []string
	for _, l := range strings.Split(dump, "\n") {
		if strings.HasPrefix(l, "text ") {
			out = append(out, l)
		}
	}
	return out
}

// fullPaint repaints the whole window and returns the dump.
func fullPaint(t *testing.T, v *TextView, s *fake.Shell, sf *fake.Surface) string {
	t.Helper()
	v.view.Invalidate()
	return paint(t, s, sf)
}

// TestTextViewIdleLayout: on a 1 MB text scrolled to the middle, the idle
// pass makes every height exact tick by tick without moving a visible
// line, keeps the visible layouts cached and leaves no timer once done;
// an edit that makes an estimate starts it again.
func TestTextViewIdleLayout(t *testing.T) {
	v, _, sf, s := viewWindow(t, nil)
	v.SetText(megaText(1))
	mid := v.Editor().Len() / 2
	v.SetSelection(mid, mid)
	clk := s.FakeClock()
	idx := v.lines.Index()
	want := textOps(fullPaint(t, v, s, sf))
	if len(want) == 0 || idx.Inexact() == 0 || len(s.Timers()) != 1 {
		t.Fatalf("before the pass: %d lines shown, %d estimates, timers %v", len(want), idx.Inexact(), s.Timers())
	}
	cached := v.lines.Cached()
	inexact, ticks := idx.Inexact(), 0
	for idx.Inexact() > 0 {
		if ticks++; ticks > 200 {
			t.Fatalf("not exact after %d ticks: %d estimates", ticks, idx.Inexact())
		}
		clk.Advance(idleLayoutDelay)
		if n := idx.Inexact(); n >= inexact {
			t.Fatalf("tick %d measured nothing (%d estimates)", ticks, n)
		} else {
			inexact = n
		}
		if got := textOps(fullPaint(t, v, s, sf)); !slices.Equal(got, want) {
			t.Fatalf("tick %d moved the visible lines:\n%s\nwant\n%s", ticks, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
	if got, want := v.sv.DocumentSize.H, 2*tokens.TextView.ContainerInsetY+idx.Total(); math.Abs(got-want) > 1e-6 {
		t.Errorf("document %v high, want %v", got, want)
	}
	if v.lines.Cached() > max(cached, 8) {
		t.Errorf("the pass cached %d layouts (%d before)", v.lines.Cached(), cached)
	}
	if len(s.Timers()) != 0 {
		t.Errorf("timers left once exact: %v", s.Timers())
	}
	if n := clk.Advance(time.Minute); n != 0 || clk.Requested() {
		t.Errorf("%d frames once exact", n)
	}
	t.Logf("%d paragraphs exact in %d ticks", idx.Len(), ticks)

	// A long paste far away makes estimates again: the pass runs once more.
	ed := v.Editor()
	ed.SetSelection(0, 0)
	ed.Paste(strings.Repeat("a long new paragraph of words\n", 2000))
	if idx.Inexact() == 0 {
		t.Fatal("no estimates after the edit")
	}
	v.settle()
	if len(s.Timers()) != 1 {
		t.Fatalf("pass not restarted: timers %v", s.Timers())
	}
	for range 200 {
		clk.Advance(idleLayoutDelay)
	}
	if idx.Inexact() != 0 || len(s.Timers()) != 0 {
		t.Errorf("second pass: %d estimates, timers %v", idx.Inexact(), s.Timers())
	}
}

// TestTextViewIdleLayoutWaits: the pass measures nothing while the offset
// moves (momentum after a swipe, on a running driver's timeline) and goes
// on once it rests.
func TestTextViewIdleLayoutWaits(t *testing.T) {
	scrollRun(t, driver.Appearance{}, func(e *scrollEnv) {
		v := e.d.NewTextView(nil).(*TextView)
		e.c.SetChildren([]driver.Widget{v})
		v.SetFrame(e.frame)
		v.SetText(megaText(2))
		e.clk.Flush()
		idx := v.lines.Index()
		e.sf.TimedSwipe(e.at(50, 50), 8*time.Millisecond,
			driver.Point{Y: -20}, driver.Point{Y: -20}, driver.Point{Y: -20}, driver.Point{Y: -20})
		e.clk.Advance(16 * time.Millisecond)
		if !v.sv.y.Moving() {
			t.Fatal("no momentum after the swipe")
		}
		// Scrolling lays out the paragraphs that come into view; the pass
		// has not measured one while idleFrom is 0.
		ticks := 0
		for ; v.sv.y.Moving(); ticks++ {
			e.clk.Advance(idleLayoutDelay)
			if v.sv.y.Moving() && v.idleFrom != 0 {
				t.Fatal("measured while the offset moves")
			}
		}
		e.clk.Advance(2 * idleLayoutDelay)
		if ticks < 2 || v.idleFrom == 0 || idx.Inexact() == 0 {
			t.Errorf("%d ticks of momentum; once at rest idleFrom %d, %d estimates", ticks, v.idleFrom, idx.Inexact())
		}
	})
}

// TestTextViewIdleLayoutStops: the pass stops while the view is hidden or
// its window closed, and starts again once the view shows.
func TestTextViewIdleLayoutStops(t *testing.T) {
	v, _, sf, s := viewWindow(t, nil)
	v.SetText(megaText(2))
	paint(t, s, sf)
	clk := s.FakeClock()
	idx := v.lines.Index()
	v.view.SetHidden(true)
	clk.Advance(time.Second)
	if len(s.Timers()) != 0 {
		t.Errorf("hidden: timers %v", s.Timers())
	}
	n := idx.Inexact()
	v.view.SetHidden(false)
	paint(t, s, sf)
	clk.Advance(idleLayoutDelay)
	if idx.Inexact() >= n {
		t.Error("the pass did not start again once shown")
	}
	sf.Close()
	clk.Advance(time.Second)
	if len(s.Timers()) != 0 {
		t.Errorf("closed: timers %v", s.Timers())
	}
}

// TestTextViewIdleLayoutSmall: a text that shows whole is exact at once
// and runs no idle timer.
func TestTextViewIdleLayoutSmall(t *testing.T) {
	v, _, sf, s := viewWindow(t, nil)
	v.SetText("one\ntwo\nthree")
	paint(t, s, sf)
	if v.lines.Index().Inexact() != 0 || len(s.Timers()) != 0 {
		t.Errorf("%d estimates, timers %v", v.lines.Index().Inexact(), s.Timers())
	}
}

// TestTextViewIdleLayoutTime: a tick stops once idleLayoutTime has passed
// on its clock, after at least one paragraph, even with byte budget left.
func TestTextViewIdleLayoutTime(t *testing.T) {
	v, _, sf, s := viewWindow(t, nil)
	v.SetText(megaText(1))
	paint(t, s, sf)
	idx := v.lines.Index()
	var now time.Duration
	step := time.Millisecond
	v.idleNow = func() time.Duration { now += step; return now }
	for _, c := range []struct {
		step time.Duration
		want int
	}{
		{time.Millisecond, 4},  // the start, then 4 paragraphs of 1 ms
		{time.Hour, 1},         // at least one
		{time.Microsecond, -1}, // the byte budget first
	} {
		step = c.step
		before := idx.Inexact()
		s.FakeClock().Advance(idleLayoutDelay)
		got := before - idx.Inexact()
		if c.want > 0 && got != c.want || c.want < 0 && (got < 16 || got > 16<<10/idleLayoutParaCost) {
			t.Errorf("step %v: %d paragraphs measured in a tick, want %d", c.step, got, c.want)
		}
	}
}
