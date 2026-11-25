package aqua

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// Mouse selection and auto-scroll in the TextView (PLAN.md S5.4i), through
// the fake shell's pointer input and frame clock. The view is at surface
// (10, 38, 180, 80): 5 lines of 16 pt below a 5 pt inset, wrapping after
// 24 fake runes of 7 pt, the text 5 pt in from the left.

// viewPoint returns the surface point of the caret at the editor offset
// off (upstream at a soft break), on the middle of its line, by the
// offset as shown; dx moves it right.
func viewPoint(v *TextView, off int, upstream bool, dx float64) driver.Point {
	v.settle()
	buf := v.Editor().Buffer()
	i := buf.ParagraphIndex(off)
	l := v.lines.Layout(i)
	cr := l.CaretRect(off - buf.ParagraphStart(i))
	if upstream {
		cr = l.CaretRectUpstream(off - buf.ParagraphStart(i))
	}
	y := tokens.TextView.ContainerInsetY + v.lines.Index().Y(i) + cr.Y + cr.H/2
	return ToRoot(v.doc, driver.Point{X: tokens.TextView.LineFragmentPadding + cr.X + dx, Y: y})
}

// viewCharW is the fake text system's width of one cluster in v's font.
func viewCharW(v *TextView) float64 { return fake.RuneWidth(v.TextFont()) }

// TestTextViewClick: a click focuses the view, places the caret at the
// nearest boundary of the visual line under it (upstream right of a
// wrapped line), fires nothing, scrolls nothing, and repaints.
func TestTextViewClick(t *testing.T) {
	h := &viewLog{}
	v, w, sf, s := viewWindow(t, h)
	long := strings.Repeat("abcdef ", 6) // 42 runes: wraps after "abcdef abcdef abcdef "
	v.SetText("one two\n" + long + "\nend")
	v.SetSelection(0, 0)
	ed := v.Editor()
	sf.Click(viewPoint(v, 5, false, 0))
	if !v.view.HasFocus() || ed.Anchor() != 5 || ed.Head() != 5 {
		t.Fatalf("click: focus %v, %s", v.view.HasFocus(), viewSel(v))
	}
	if w.Router().Captured() != nil {
		t.Error("capture kept after the release")
	}
	s.Flush()
	cw := viewCharW(v)
	wrap := 8 + 21 // the second paragraph's soft break
	for _, c := range []struct {
		what     string
		p        driver.Point
		off      int
		upstream bool
	}{
		{"leading half", viewPoint(v, 5, false, 0.4*cw), 5, false},
		{"trailing half", viewPoint(v, 5, false, 0.6*cw), 6, false},
		{"right of the first paragraph", viewPoint(v, 7, false, 40), 7, false},
		{"in the left padding", viewPoint(v, 8, false, -4), 8, false},
		// Right of a line wrapped at a space: before the hanging space, as
		// AppKit, on the same line.
		{"right of a wrapped line", viewPoint(v, wrap, true, 20), wrap - 1, false},
		{"start of the next line", viewPoint(v, wrap, false, -2), wrap, false},
		{"end of the wrapped paragraph", viewPoint(v, 8+len(long), false, 10), 8 + len(long), false},
		{"below the last line", ToRoot(v.view, driver.Point{X: 20, Y: 78}), ed.Len(), false},
		{"in the top inset", ToRoot(v.view, driver.Point{X: 5 + 1.6*cw, Y: 1}), 2, false},
	} {
		apart(s)
		sf.Click(c.p)
		if ed.Anchor() != c.off || ed.Head() != c.off || ed.Upstream() != c.upstream {
			t.Errorf("%s: %s upstream %v, want %d %v", c.what, viewSel(v), ed.Upstream(), c.off, c.upstream)
		}
		if !sf.Invalid() {
			t.Errorf("%s: no repaint", c.what)
		}
		s.Flush()
	}
	// Right of a line broken inside a word: its end, upstream.
	v.SetText(strings.Repeat("x", 30))
	apart(s)
	sf.Click(viewPoint(v, 24, true, 3))
	if ed.Head() != 24 || !ed.Upstream() {
		t.Errorf("right of a broken word: %s upstream %v", viewSel(v), ed.Upstream())
	}
	if h.changed != 0 || v.Offset().Y != 0 {
		t.Errorf("clicks fired %d events, offset %v", h.changed, v.Offset())
	}
	// A click on a partly shown line scrolls nothing.
	v.SetText(numberedText(40))
	apart(s)
	sf.Click(ToRoot(v.view, driver.Point{X: 20, Y: 79}))
	if ed.Head() != ed.Buffer().ParagraphStart(4)+2 || v.Offset().Y != 0 {
		t.Errorf("click on the bottom line: %d, offset %v", ed.Head(), v.Offset())
	}
	// Scrolled: the click hits the text shown.
	v.sv.SetOffset(driver.Point{Y: 165}) // line 10 at the top
	apart(s)
	sf.Click(ToRoot(v.view, driver.Point{X: 5 + 4.5*cw, Y: 8}))
	if want := ed.Buffer().ParagraphStart(10) + 5; ed.Head() != want || v.Offset().Y != 165 {
		t.Errorf("click scrolled: %d, want %d, offset %v", ed.Head(), want, v.Offset())
	}
}

// TestTextViewShiftClick: Shift-click extends the selection, keeping its
// far end, across paragraphs.
func TestTextViewShiftClick(t *testing.T) {
	v, _, sf, s := viewWindow(t, nil)
	v.SetText("one two\nthree four\nfive")
	sf.Click(viewPoint(v, 4, false, 0))
	for _, c := range []struct{ off, anchor int }{
		{14, 4}, // the second paragraph
		{1, 14}, // before: the far end is the anchor
		{20, 1},
		{9, 1}, // inside: the anchor stays
	} {
		apart(s)
		sf.SetModifiers(shf)
		sf.Click(viewPoint(v, c.off, false, 0))
		sf.SetModifiers(0)
		if ed := v.Editor(); ed.Anchor() != c.anchor || ed.Head() != c.off {
			t.Errorf("Shift-click at %d: %s, want %d,%d", c.off, viewSel(v), c.anchor, c.off)
		}
	}
}

// TestTextViewMultiClick: a double-click selects the word and drags by
// words, a triple-click the paragraph with its newline and drags by
// paragraphs, keeping the first one selected both ways.
func TestTextViewMultiClick(t *testing.T) {
	v, _, sf, s := viewWindow(t, nil)
	const text = "one two\nthree four\nfive six\nseven"
	v.SetText(text)
	sf.DoubleClick(viewPoint(v, 9, false, 0))
	if got := viewSel(v); got != text+"|8,13" {
		t.Errorf("double-click: %s", got)
	}
	// A double-click drag by words, to the right and back past the first word.
	apart(s)
	sf.Click(viewPoint(v, 15, false, 0))
	sf.Press(viewPoint(v, 15, false, 0), shell.ButtonPrimary)
	sf.Move(viewPoint(v, 21, false, 0)) // in "five"
	if got := viewSel(v); got != text+"|14,23" {
		t.Errorf("word drag right: %s", got)
	}
	sf.Move(viewPoint(v, 1, false, 0)) // in "one"
	if got := viewSel(v); got != text+"|18,0" {
		t.Errorf("word drag left: %s", got)
	}
	sf.Release(viewPoint(v, 1, false, 0), shell.ButtonPrimary)
	// A triple-click: the paragraph and its newline; a drag by paragraphs.
	apart(s)
	p := viewPoint(v, 10, false, 0)
	sf.Click(p)
	sf.Click(p)
	sf.Press(p, shell.ButtonPrimary)
	if got := viewSel(v); got != text+"|8,19" {
		t.Errorf("triple-click: %s", got)
	}
	sf.Move(viewPoint(v, 30, false, 0))
	if got := viewSel(v); got != text+"|8,"+strconv.Itoa(len(text)) {
		t.Errorf("paragraph drag down: %s", got)
	}
	sf.Move(viewPoint(v, 2, false, 0))
	if got := viewSel(v); got != text+"|19,0" {
		t.Errorf("paragraph drag up: %s", got)
	}
	sf.Release(viewPoint(v, 2, false, 0), shell.ButtonPrimary)
	// Triple-click on the last paragraph: no newline to take.
	apart(s)
	p = viewPoint(v, len(text)-1, false, 0)
	sf.Click(p)
	sf.Click(p)
	sf.Click(p)
	if got := viewSel(v); got != text+"|28,"+strconv.Itoa(len(text)) {
		t.Errorf("triple-click on the last paragraph: %s", got)
	}
}

// TestTextViewDrag: a drag selects by clusters across paragraphs and
// wrapped lines, both ways, in a read-only view too, with no event; moves
// after the release select nothing.
func TestTextViewDrag(t *testing.T) {
	h := &viewLog{}
	v, w, sf, _ := viewWindow(t, h)
	long := strings.Repeat("abcdef ", 6)
	v.SetText("one two\n" + long + "\nend")
	v.SetEditable(false)
	sf.Press(viewPoint(v, 1, false, 0), shell.ButtonPrimary)
	if w.Router().Captured() != v.view {
		t.Fatal("a press does not capture")
	}
	sf.Move(viewPoint(v, 8+23, false, 0)) // on the wrapped line
	if ed := v.Editor(); ed.Anchor() != 1 || ed.Head() != 31 {
		t.Errorf("drag down: %s", viewSel(v))
	}
	sf.Move(viewPoint(v, 8+21, true, 15)) // right of the first visual line: before its hanging space
	if ed := v.Editor(); ed.Head() != 28 || ed.Upstream() {
		t.Errorf("drag to a line end: %s upstream %v", viewSel(v), ed.Upstream())
	}
	sf.Move(ToRoot(v.view, driver.Point{X: 100, Y: 75})) // below the text
	if ed := v.Editor(); ed.Head() != ed.Len() {
		t.Errorf("drag below the text: %s", viewSel(v))
	}
	sf.Release(viewPoint(v, 0, false, 0), shell.ButtonPrimary)
	if ed := v.Editor(); ed.Anchor() != 1 || ed.Head() != 0 {
		t.Errorf("release: %s", viewSel(v))
	}
	sf.Move(viewPoint(v, 5, false, 0))
	if ed := v.Editor(); ed.Head() != 0 {
		t.Errorf("move after the release: %s", viewSel(v))
	}
	if h.changed != 0 || v.Offset().Y != 0 {
		t.Errorf("drags fired %d events, offset %v", h.changed, v.Offset())
	}
}

// below returns the surface point dy below the view's bottom edge, at
// x 30 in the view.
func below(v *TextView, dy float64) driver.Point {
	return ToRoot(v.view, driver.Point{X: 30, Y: 80 + dy})
}

// above returns the surface point dy above the view's top edge.
func above(v *TextView, dy float64) driver.Point {
	return ToRoot(v.view, driver.Point{X: 30, Y: -dy})
}

// autoRunning reports whether the view's auto-scroll runs.
func autoRunning(v *TextView) bool { return v.auto != nil && v.auto.Running() }

// runView runs f on a started driver (so with the frame clock's timeline)
// with a shown 200×100 window holding a text view at content
// (10, 10, 180, 80), surface (10, 38, 180, 80), as viewWindow.
func runView(t *testing.T, f func(v *TextView, w *Window, sf *fake.Surface, s *fake.Shell)) {
	t.Helper()
	sh := fake.New()
	d := New(sh)
	h := &appLog{}
	h.onReady = func() {
		defer d.Quit()
		dw, err := d.NewWindow(driver.WindowConfig{Size: driver.Size{W: 200, H: 100}, Style: allStyles}, &winLog{})
		if err != nil {
			t.Fatal(err)
		}
		w := dw.(*Window)
		v := d.NewTextView(nil).(*TextView)
		w.SetContent(v)
		v.SetFrame(driver.Rect{X: 10, Y: 10, W: 180, H: 80})
		w.Show()
		sh.FakeClock().Advance(0)
		f(v, w, w.Surface().(*fake.Surface), sh)
	}
	if err := runDriver(t, d, h); err != nil {
		t.Fatal(err)
	}
}

// TestTextViewAutoScroll: held past the bottom the text scrolls on the
// frame clock without pointer moves, at a speed growing with the distance
// outside, the selection following; back inside it stops; past the top it
// scrolls back and stops at the start, with no frames left.
func TestTextViewAutoScroll(t *testing.T) {
	runView(t, func(v *TextView, _ *Window, sf *fake.Surface, s *fake.Shell) {
		v.SetText(numberedText(200)) // document 3210 pt
		clk := s.FakeClock()
		clk.Advance(time.Second) // the idle layout pass ends
		sf.Press(viewPoint(v, 2, false, 0), shell.ButtonPrimary)
		if autoRunning(v) {
			t.Fatal("auto-scroll inside the viewport")
		}
		sf.Move(below(v, 10))
		if !autoRunning(v) || !clk.Requested() {
			t.Fatal("no auto-scroll past the bottom")
		}
		speed := func(d float64) float64 { return autoScrollMin + autoScrollGain*d }
		y0 := v.Offset().Y
		clk.Advance(500 * time.Millisecond)
		got := v.Offset().Y - y0
		if want := speed(10) / 2; math.Abs(got-want) > speed(10)*fake.FrameInterval.Seconds() {
			t.Errorf("half a second 10 pt below: scrolled %v, want about %v", got, want)
		}
		// The head is on the line under the pointer, below the viewport.
		ed := v.Editor()
		if off, _ := v.hitAt(FromRoot(v.view, below(v, 10))); ed.Anchor() != 2 || ed.Head() != off || off <= 2 {
			t.Errorf("selection while scrolling: %d,%d, want 2,%d under the pointer", ed.Anchor(), ed.Head(), off)
		}
		// Further out: faster.
		sf.Move(below(v, 40))
		y0 = v.Offset().Y
		clk.Advance(500 * time.Millisecond)
		if got2 := v.Offset().Y - y0; got2 < 2*got {
			t.Errorf("40 pt below: scrolled %v in half a second, at 10 pt %v", got2, got)
		}
		// Back inside: it stops, with no frames.
		sf.Move(ToRoot(v.view, driver.Point{X: 30, Y: 40}))
		if autoRunning(v) {
			t.Error("auto-scroll inside the viewport")
		}
		y0 = v.Offset().Y
		clk.Advance(time.Second) // the scrollers fade, the idle pass
		if n := clk.Advance(time.Second); n != 0 || v.Offset().Y != y0 {
			t.Errorf("back inside: %d frames, offset %v (was %v)", n, v.Offset().Y, y0)
		}
		// Past the top: back up to the start, where it stops.
		sf.Move(above(v, 30))
		if !autoRunning(v) {
			t.Fatal("no auto-scroll past the top")
		}
		clk.Advance(10 * time.Second)
		if v.Offset().Y != 0 || autoRunning(v) {
			t.Errorf("at the top: offset %v, running %v", v.Offset().Y, autoRunning(v))
		}
		if ed.Head() != 0 || ed.Anchor() != 2 {
			t.Errorf("at the top: %d,%d", ed.Anchor(), ed.Head())
		}
		clk.Advance(time.Second)
		if n := clk.Advance(time.Second); n != 0 {
			t.Errorf("%d frames at the top with the button held", n)
		}
		sf.Release(above(v, 30), shell.ButtonPrimary)
		if autoRunning(v) || ed.Head() != 0 {
			t.Errorf("after the release: running %v, head %d", autoRunning(v), ed.Head())
		}
	})
}

// TestTextViewAutoScrollEnd: the auto-scroll stops on release mid-way and
// at the document's end with the selection at the text's end, with no
// frames afterwards.
func TestTextViewAutoScrollEnd(t *testing.T) {
	runView(t, func(v *TextView, _ *Window, sf *fake.Surface, s *fake.Shell) {
		v.SetText(numberedText(60))
		clk := s.FakeClock()
		clk.Advance(time.Second)
		ed := v.Editor()
		// Released mid-way.
		sf.Press(viewPoint(v, 0, false, 0), shell.ButtonPrimary)
		sf.Move(below(v, 5))
		clk.Advance(200 * time.Millisecond)
		sf.Release(below(v, 5), shell.ButtonPrimary)
		y := v.Offset().Y
		if autoRunning(v) || y <= 0 {
			t.Fatalf("release: running %v, offset %v", autoRunning(v), y)
		}
		clk.Advance(time.Second)
		if n := clk.Advance(time.Second); n != 0 || v.Offset().Y != y {
			t.Errorf("after the release: %d frames, offset %v (was %v)", n, v.Offset().Y, y)
		}
		// Far below: to the end, where it stops.
		apart(s)
		sf.Press(ToRoot(v.view, driver.Point{X: 30, Y: 30}), shell.ButtonPrimary)
		sf.Move(below(v, 15))
		clk.Advance(10 * time.Second)
		end := v.sv.DocumentSize.H - 80
		if v.Offset().Y != end || autoRunning(v) {
			t.Errorf("at the end: offset %v (max %v), running %v", v.Offset().Y, end, autoRunning(v))
		}
		if ed.Head() != ed.Len() {
			t.Errorf("at the end: head %d, want %d", ed.Head(), ed.Len())
		}
		clk.Advance(time.Second)
		if n := clk.Advance(time.Second); n != 0 {
			t.Errorf("%d frames at the end with the button held", n)
		}
		// Back inside the text: the head follows the pointer, nothing scrolls.
		sf.Move(ToRoot(v.view, driver.Point{X: 30, Y: 30}))
		if ed.Head() == ed.Len() || autoRunning(v) || v.Offset().Y != end {
			t.Errorf("back inside: head %d, running %v, offset %v", ed.Head(), autoRunning(v), v.Offset().Y)
		}
		sf.Release(ToRoot(v.view, driver.Point{X: 30, Y: 30}), shell.ButtonPrimary)
	})
}

// TestTextViewAutoScrollStops: a capture cancel, a focus loss, disabling
// and closing the window stop the auto-scroll; Reduce Motion does not.
func TestTextViewAutoScrollStops(t *testing.T) {
	for _, c := range []struct {
		what string
		do   func(v *TextView, w *Window, sf *fake.Surface)
		stop bool
	}{
		{"capture cancel", func(_ *TextView, _ *Window, sf *fake.Surface) { sf.CancelCapture() }, true},
		{"focus loss", func(_ *TextView, w *Window, _ *fake.Surface) { w.Router().SetFocus(nil) }, true},
		{"disabled", func(v *TextView, _ *Window, _ *fake.Surface) { v.SetEnabled(false) }, true},
		{"closed", func(_ *TextView, w *Window, _ *fake.Surface) { w.Close() }, true},
		{"Reduce Motion", func(v *TextView, _ *Window, _ *fake.Surface) { v.view.d.Timeline().SetReduceMotion(true) }, false},
	} {
		runView(t, func(v *TextView, w *Window, sf *fake.Surface, s *fake.Shell) {
			v.SetText(numberedText(100))
			clk := s.FakeClock()
			clk.Advance(time.Second)
			sf.Press(viewPoint(v, 0, false, 0), shell.ButtonPrimary)
			sf.Move(below(v, 10))
			clk.Advance(100 * time.Millisecond)
			if !autoRunning(v) {
				t.Fatalf("%s: not running", c.what)
			}
			c.do(v, w, sf)
			clk.Advance(100 * time.Millisecond)
			y := v.Offset().Y
			clk.Advance(200 * time.Millisecond)
			if c.stop && (autoRunning(v) || v.Offset().Y != y) {
				t.Errorf("%s: running %v, offset %v (was %v)", c.what, autoRunning(v), v.Offset().Y, y)
			}
			if !c.stop && (!autoRunning(v) || v.Offset().Y <= y) {
				t.Errorf("%s: running %v, offset %v (was %v)", c.what, autoRunning(v), v.Offset().Y, y)
			}
		})
	}
}

// TestTextViewCursor: the I-beam over the text of an enabled view,
// editable or read-only; the arrow when disabled.
func TestTextViewCursor(t *testing.T) {
	v, _, sf, _ := viewWindow(t, nil)
	v.SetText("text")
	in := ToRoot(v.view, driver.Point{X: 40, Y: 40})
	sf.Move(in)
	if c := sf.Cursor(); c != shell.CursorIBeam {
		t.Errorf("over the text: %v", c)
	}
	v.SetEditable(false)
	sf.Move(in.Add(driver.Point{X: 1}))
	if c := sf.Cursor(); c != shell.CursorIBeam {
		t.Errorf("read-only: %v", c)
	}
	v.SetEnabled(false)
	sf.Move(in)
	if c := sf.Cursor(); c != shell.CursorArrow {
		t.Errorf("disabled: %v", c)
	}
}

// TestTextViewAutoScrollEstimates: scrolling up through paragraphs whose
// heights are still estimates (wrapped, so the estimates are off), the
// text at the top of the viewport only ever moves back toward the start,
// frame by frame, and the head stays under the pointer.
func TestTextViewAutoScrollEstimates(t *testing.T) {
	runView(t, func(v *TextView, _ *Window, sf *fake.Surface, s *fake.Shell) {
		var b strings.Builder
		for i := range 300 {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(strings.Repeat("word ", 1+i*7%23))
		}
		v.SetText(b.String())
		v.sv.SetOffset(driver.Point{Y: v.sv.y.Max() / 2})
		if v.lines.Index().Inexact() == 0 {
			t.Fatal("every height is exact")
		}
		clk := s.FakeClock()
		sf.Press(ToRoot(v.view, driver.Point{X: 30, Y: 30}), shell.ButtonPrimary)
		sf.Move(above(v, 60))
		prev := v.anchor().off
		for i := 0; autoRunning(v); i++ {
			if i > 2000 {
				t.Fatal("never reaches the start")
			}
			clk.Advance(fake.FrameInterval)
			a := v.anchor()
			if a.ok && a.off > prev {
				t.Fatalf("frame %d: the top line moved down the text, from %d to %d", i, prev, a.off)
			}
			prev = a.off
			if off, _ := v.hitAt(FromRoot(v.view, above(v, 60))); v.Editor().Head() != off {
				t.Fatalf("frame %d: head %d, want %d under the pointer", i, v.Editor().Head(), off)
			}
		}
		if v.Offset().Y != 0 || v.Editor().Head() != 0 {
			t.Errorf("at the start: offset %v, head %d", v.Offset().Y, v.Editor().Head())
		}
		sf.Release(above(v, 60), shell.ButtonPrimary)
	})
}
