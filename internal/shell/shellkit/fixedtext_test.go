package shellkit

import (
	"math"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/render"
)

func layoutOf(text string, f render.Font, maxW float64) render.TextLayout {
	return FixedText{}.NewLayout(text, f, maxW)
}

func wantRects(t *testing.T, what string, got []render.Rect, want ...render.Rect) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", what, got, want)
		}
	}
}

func r(x, y, w, h float64) render.Rect { return render.Rect{X: x, Y: y, W: w, H: h} }

func TestTextMatchesStubMetrics(t *testing.T) {
	fonts := []render.Font{{}, {Size: 13}, {Size: 26}, {Size: 9.5, Weight: driver.FontWeightBold}, {Size: -1}}
	for _, f := range fonts {
		for _, s := range []string{"", "a", "Hello, world", "héllo ☃ wörld", "tab\there"} {
			l := layoutOf(s, f, 0)
			want := driver.Size{
				W: stub.TextWidth(s, f, driver.ControlSizeRegular),
				H: stub.LineHeight(f, driver.ControlSizeRegular),
			}
			if l.Size() != want || l.LineCount() != 1 {
				t.Errorf("%q in %+v: size %v lines %d, want %v and 1", s, f, l.Size(), l.LineCount(), want)
			}
			if l.Text() != s || l.Font() != f {
				t.Errorf("%q: Text/Font = %q %+v", s, l.Text(), l.Font())
			}
			if got, want := l.Baseline(), 0.75*want.H; got != want {
				t.Errorf("%q: Baseline = %g, want %g", s, got, want)
			}
		}
	}
	if FixedRuneWidth(render.Font{}) != 7 || FixedLineHeight(render.Font{}) != 16 ||
		FixedRuneWidth(render.Font{Size: 26}) != 14 || FixedLineHeight(render.Font{Size: 26}) != 32 {
		t.Error("RuneWidth/LineHeight differ from 7/16 at 13 pt, scaled")
	}
}

func TestTextNoWrapValues(t *testing.T) {
	for _, maxW := range []float64{0, -10, math.NaN(), math.Inf(1)} {
		l := layoutOf("hello world", render.Font{}, maxW)
		if l.LineCount() != 1 || l.Size() != (render.Size{W: 77, H: 16}) {
			t.Errorf("maxWidth %g: %d lines, %v", maxW, l.LineCount(), l.Size())
		}
	}
}

func TestTextNewlines(t *testing.T) {
	l := layoutOf("ab\n\nabcd\n", render.Font{}, 0)
	if l.LineCount() != 4 || l.Size() != (render.Size{W: 28, H: 64}) {
		t.Fatalf("lines %d size %v", l.LineCount(), l.Size())
	}
	for _, c := range []struct {
		i    int
		want render.Rect
	}{
		{0, r(0, 0, 0, 16)}, {2, r(14, 0, 0, 16)}, {3, r(0, 16, 0, 16)},
		{4, r(0, 32, 0, 16)}, {8, r(28, 32, 0, 16)}, {9, r(0, 48, 0, 16)},
		{-5, r(0, 0, 0, 16)}, {99, r(0, 48, 0, 16)},
	} {
		if got := l.CaretRect(c.i); got != c.want {
			t.Errorf("CaretRect(%d) = %v, want %v", c.i, got, c.want)
		}
	}
	// A selection across a '\n' shows it as one rune; the empty line shows
	// only its '\n'.
	wantRects(t, "sel 1..5", l.SelectionRects(1, 5), r(7, 0, 14, 16), r(0, 16, 7, 16), r(0, 32, 7, 16))
	wantRects(t, "sel reversed", l.SelectionRects(5, 1), r(7, 0, 14, 16), r(0, 16, 7, 16), r(0, 32, 7, 16))
	wantRects(t, "sel empty", l.SelectionRects(3, 3))
	wantRects(t, "sel to end", l.SelectionRects(4, 9), r(0, 32, 35, 16))
	// Hit-testing: right of a line is its end before the '\n'.
	for _, c := range []struct {
		p        render.Point
		i        int
		trailing bool
	}{
		{render.Point{X: 100, Y: 1}, 2, false},
		{render.Point{X: 100, Y: 20}, 3, false},
		{render.Point{X: 3, Y: 40}, 4, false},
		{render.Point{X: 4, Y: 40}, 4, true},
		{render.Point{X: 10, Y: 40}, 5, false},
		{render.Point{X: 2, Y: -50}, 0, false},
		{render.Point{X: 5, Y: -50}, 0, true},
		{render.Point{X: 5, Y: 1000}, 9, false},
		{render.Point{X: -5, Y: 40}, 4, false},
		{render.Point{X: math.NaN(), Y: math.NaN()}, 0, false},
	} {
		i, tr := l.HitTest(c.p)
		if i != c.i || tr != c.trailing {
			t.Errorf("HitTest(%v) = %d %v, want %d %v", c.p, i, tr, c.i, c.trailing)
		}
	}
}

func TestTextWrapAtSpaces(t *testing.T) {
	// 50 pt fits 7 runes: "hello w" → break after "hello ".
	l := layoutOf("hello world", render.Font{}, 50)
	if l.LineCount() != 2 || l.Size() != (render.Size{W: 35, H: 32}) {
		t.Fatalf("lines %d size %v", l.LineCount(), l.Size())
	}
	if i, tr := l.HitTest(render.Point{X: 100, Y: 5}); i != 5 || tr {
		t.Errorf("right of line 1: %d %v, want 5 (before the hanging space)", i, tr)
	}
	if i, _ := l.HitTest(render.Point{X: 100, Y: 20}); i != 11 {
		t.Errorf("right of line 2: %d, want 11", i)
	}
	if got := l.CaretRect(6); got != r(0, 16, 0, 16) {
		t.Errorf("caret at the break = %v, want the start of line 2", got)
	}
	if got := l.CaretRect(5); got != r(35, 0, 0, 16) {
		t.Errorf("caret before the space = %v", got)
	}
	wantRects(t, "select all", l.SelectionRects(0, 11), r(0, 0, 42, 16), r(0, 16, 35, 16))

	// The space right after the fitting runes is a break too, and a run of
	// spaces hangs as a whole.
	for _, c := range []struct {
		text  string
		maxW  float64
		lines int
		size  render.Size
	}{
		{"hello world", 35, 2, render.Size{W: 35, H: 32}},
		{"a    b", 14, 2, render.Size{W: 7, H: 32}},
		{"ab   ", 14, 1, render.Size{W: 14, H: 16}},
		{"one two three four", 70, 2, render.Size{W: 70, H: 32}}, // "one two " | "three four"
		{"one two three four", 50, 3, render.Size{W: 49, H: 48}}, // "one two" | "three" | "four"
		{"aaa bbb", 1, 6, render.Size{W: 7, H: 96}},              // one rune per line, the space hangs
	} {
		l := layoutOf(c.text, render.Font{}, c.maxW)
		if l.LineCount() != c.lines || l.Size() != c.size {
			t.Errorf("%q at %g: %d lines %v, want %d %v", c.text, c.maxW, l.LineCount(), l.Size(), c.lines, c.size)
		}
	}
}

func TestTextWrapBetweenRunes(t *testing.T) {
	l := layoutOf("abcdef", render.Font{}, 28)
	if l.LineCount() != 2 || l.Size() != (render.Size{W: 28, H: 32}) {
		t.Fatalf("lines %d size %v", l.LineCount(), l.Size())
	}
	if i, tr := l.HitTest(render.Point{X: 100, Y: 5}); i != 4 || tr {
		t.Errorf("right of the broken line: %d %v, want 4", i, tr)
	}
	if got := l.CaretRect(4); got != r(0, 16, 0, 16) {
		t.Errorf("caret at the break = %v", got)
	}
	wantRects(t, "sel 2..5", l.SelectionRects(2, 5), r(14, 0, 14, 16), r(0, 16, 7, 16))
	// The wrapped label of the layout tests: 10 runes at 35 pt give 2 lines.
	if l := layoutOf("aaaaaaaaaa", render.Font{}, 35); l.LineCount() != 2 || l.Size() != (render.Size{W: 35, H: 32}) {
		t.Errorf("aaaaaaaaaa at 35: %d lines %v", l.LineCount(), l.Size())
	}
}

func TestTextUTF8(t *testing.T) {
	s := "héllo ☃" // h é(2) l l o ' ' ☃(3): 10 bytes, 7 runes
	l := layoutOf(s, render.Font{}, 0)
	if l.Size().W != 49 {
		t.Fatalf("width %g", l.Size().W)
	}
	// Indices inside a rune move back to its start.
	if got := l.CaretRect(2); got != r(7, 0, 0, 16) {
		t.Errorf("CaretRect(2) = %v, want the start of é", got)
	}
	if got := l.CaretRect(9); got != r(42, 0, 0, 16) {
		t.Errorf("CaretRect(9) = %v, want the start of ☃", got)
	}
	if i, tr := l.HitTest(render.Point{X: 11, Y: 0}); i != 1 || !tr {
		t.Errorf("HitTest in é = %d %v", i, tr)
	}
	if i, _ := l.HitTest(render.Point{X: 43, Y: 0}); i != 7 {
		t.Errorf("HitTest in ☃ = %d, want 7", i)
	}
	wantRects(t, "sel é", l.SelectionRects(1, 3), r(7, 0, 7, 16))
	wantRects(t, "sel inside é", l.SelectionRects(2, 2))
	// Wrapping counts runes, not bytes.
	if l := layoutOf("ééé ééé", render.Font{}, 21); l.LineCount() != 2 || l.Size().W != 21 {
		t.Errorf("wrapped UTF-8: %d lines %v", l.LineCount(), l.Size())
	}
}

func TestTextEmpty(t *testing.T) {
	l := layoutOf("", render.Font{Size: 26}, 100)
	if l.LineCount() != 1 || l.Size() != (render.Size{H: 32}) {
		t.Fatalf("empty: %d lines %v", l.LineCount(), l.Size())
	}
	if i, tr := l.HitTest(render.Point{X: 50, Y: 50}); i != 0 || tr {
		t.Errorf("HitTest = %d %v", i, tr)
	}
	if got := l.CaretRect(3); got != r(0, 0, 0, 32) {
		t.Errorf("CaretRect = %v", got)
	}
	wantRects(t, "sel", l.SelectionRects(0, 5))
}

func TestTextLineMetrics(t *testing.T) {
	// "hello " | "world\n" | "" ; "ab" broken between runes at 7 pt: "a" | "b".
	l := layoutOf("hello world\n\nabcdef", render.Font{}, 50)
	want := []render.LineMetrics{
		{Start: 0, End: 6, Top: 0, Height: 16, Baseline: 12},
		{Start: 6, End: 11, Top: 16, Height: 16, Baseline: 28},
		{Start: 12, End: 12, Top: 32, Height: 16, Baseline: 44},
		{Start: 13, End: 19, Top: 48, Height: 16, Baseline: 60},
	}
	if l.LineCount() != len(want) {
		t.Fatalf("%d lines, want %d", l.LineCount(), len(want))
	}
	for i, w := range want {
		if got := l.Line(i); got != w {
			t.Errorf("Line(%d) = %+v, want %+v", i, got, w)
		}
	}
	if got := l.Line(-3); got != want[0] {
		t.Errorf("Line(-3) = %+v, want the first", got)
	}
	if got := l.Line(99); got != want[3] {
		t.Errorf("Line(99) = %+v, want the last", got)
	}
	// Broken between runes: the lines meet without a gap.
	b := layoutOf("abcdef", render.Font{}, 28)
	if a, c := b.Line(0), b.Line(1); a.End != 4 || c.Start != 4 || c.End != 6 {
		t.Errorf("broken lines %+v %+v", a, c)
	}
	if e := layoutOf("", render.Font{Size: 26}, 0).Line(0); e != (render.LineMetrics{Height: 32, Baseline: 24}) {
		t.Errorf("empty: %+v", e)
	}
}

func TestTextCaretRectUpstream(t *testing.T) {
	l := layoutOf("hello world\nabc", render.Font{}, 50)
	for _, c := range []struct {
		i    int
		want render.Rect
	}{
		{6, r(42, 0, 0, 16)},  // the soft break: after "hello " on line 1, the hanging space included
		{5, r(35, 0, 0, 16)},  // not a break: as CaretRect
		{0, r(0, 0, 0, 16)},   // the first line
		{12, r(0, 32, 0, 16)}, // after a '\n': a hard break, as CaretRect
		{11, r(35, 16, 0, 16)},
		{99, r(21, 32, 0, 16)}, // clamped
	} {
		if got := l.CaretRectUpstream(c.i); got != c.want {
			t.Errorf("CaretRectUpstream(%d) = %v, want %v", c.i, got, c.want)
		}
	}
	if got := l.CaretRect(6); got != r(0, 16, 0, 16) {
		t.Errorf("CaretRect(6) = %v, want the next line's start", got)
	}
	// Broken between runes: after the earlier line's last rune.
	b := layoutOf("abcdef", render.Font{}, 28)
	if got := b.CaretRectUpstream(4); got != r(28, 0, 0, 16) {
		t.Errorf("broken: %v", got)
	}
	// Inside a rune at the break: moved back to the rune's start, not a break.
	u := layoutOf("ab☃", render.Font{}, 14)
	if got := u.CaretRectUpstream(3); got != r(14, 0, 0, 16) {
		t.Errorf("☃ at the break: %v", got)
	}
	if got := u.CaretRectUpstream(4); got != r(14, 0, 0, 16) {
		t.Errorf("inside ☃: %v", got)
	}
}
