//go:build windows

package win32

import (
	"fmt"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/render"
)

// The S5.4b tests: Line and CaretRectUpstream on real DirectWrite layouts.

// checkLines checks the render.LineMetrics contract on l: the lines cover
// the text in order (a wrapped line ends where the next starts, a broken
// one before its break), tile the layout's height, and hold their
// baselines; Line clamps; CaretRect at a line's start is on that line.
// It returns the soft breaks (the starts of lines that follow a wrapped one).
func checkLines(t *testing.T, l render.TextLayout) []int {
	t.Helper()
	n, text := l.LineCount(), l.Text()
	var soft []int
	top := 0.0
	for i := range n {
		m := l.Line(i)
		if i == 0 && (m.Start != 0 || !near(m.Baseline, l.Baseline(), tol)) {
			t.Errorf("%q: Line(0) = %+v, Baseline() %g", text, m, l.Baseline())
		}
		if m.Start > m.End || m.End > len(text) || !near(m.Top, top, tol) || !(m.Height > 0) ||
			!(m.Baseline > m.Top && m.Baseline < m.Top+m.Height) {
			t.Errorf("%q: Line(%d) = %+v (top so far %g)", text, i, m, top)
		}
		if i > 0 {
			p := l.Line(i - 1)
			switch {
			case p.End == m.Start:
				soft = append(soft, m.Start)
			case p.End < m.Start && strings.ContainsAny(text[p.End:m.Start], "\r\n\u2028\u2029"):
			default:
				t.Errorf("%q: Line(%d) %+v does not follow %+v", text, i, m, p)
			}
		}
		if c := l.CaretRect(m.Start); !near(c.Y, m.Top, tol) || !near(c.H, m.Height, tol) {
			t.Errorf("%q: CaretRect(%d) = %v, not on line %d %+v", text, m.Start, c, i, m)
		}
		top += m.Height
	}
	if last := l.Line(n - 1); last.End != len(text) {
		t.Errorf("%q: the last line ends at %d, want %d", text, last.End, len(text))
	}
	if !near(top, l.Size().H, tol) {
		t.Errorf("%q: the lines are %g high, the layout %g", text, top, l.Size().H)
	}
	if l.Line(-1) != l.Line(0) || l.Line(n) != l.Line(n-1) {
		t.Errorf("%q: Line does not clamp", text)
	}
	return soft
}

// checkUpstream checks CaretRectUpstream: at each soft break it is on the
// earlier line at its end x, also for an index inside the cluster there;
// elsewhere it is CaretRect.
func checkUpstream(t *testing.T, l *layout, soft []int, endX func(line int) float64) {
	t.Helper()
	isSoft := map[int]bool{}
	for _, s := range soft {
		isSoft[s] = true
	}
	li := 0
	for i := 0; i <= len(l.Text()); i++ {
		for li+1 < l.LineCount() && l.Line(li+1).Start <= i {
			li++
		}
		got := l.CaretRectUpstream(i)
		if !isSoft[l.normalize(i)] {
			if got != l.CaretRect(i) {
				t.Errorf("%q: CaretRectUpstream(%d) = %v, CaretRect %v", l.Text(), i, got, l.CaretRect(i))
			}
			continue
		}
		p := l.Line(li - 1)
		want := render.Rect{X: endX(li - 1), Y: p.Top, H: p.Height}
		if !nearRect(got, want) {
			t.Errorf("%q: CaretRectUpstream(%d) = %v, want %v", l.Text(), i, got, want)
		}
		if d := l.CaretRect(i); !near(d.Y, l.Line(li).Top, tol) || (!l.geom.bidi && !near(d.X, 0, tol)) {
			t.Errorf("%q: CaretRect(%d) = %v, want the next line's start", l.Text(), i, d)
		}
	}
}

// TestLineMetricsWrap: wrapped Latin with trailing spaces at the wrap, and
// a word broken between clusters.
func TestLineMetricsWrap(t *testing.T) {
	ts := newTestText(t)
	const s = "hello world  again\nnext"
	l := dwLayout(t, ts, s, width(t, ts, "hello world")+1)
	soft := checkLines(t, l)
	if l.LineCount() != 3 || fmt.Sprint(soft) != "[13]" {
		t.Fatalf("%d lines, soft breaks %v", l.LineCount(), soft)
	}
	if m := l.Line(0); m.End != 13 {
		t.Errorf("Line(0) = %+v, want the trailing spaces in it", m)
	}
	if m := l.Line(1); m.Start != 13 || m.End != 18 {
		t.Errorf("Line(1) = %+v, want before the newline", m)
	}
	if m := l.Line(2); m.Start != 19 || m.End != len(s) {
		t.Errorf("Line(2) = %+v", m)
	}
	ws := width(t, ts, "hello world  ")
	want := render.Rect{X: ws, Y: 0, H: l.Line(0).Height}
	if got := l.CaretRectUpstream(13); !nearRect(got, want) {
		t.Errorf("CaretRectUpstream(13) = %v, want %v (after the hanging spaces)", got, want)
	}
	checkUpstream(t, l, soft, func(li int) float64 { return width(t, ts, s[l.Line(li).Start:l.Line(li).End]) })

	w := dwLayout(t, ts, "abcdefghij", width(t, ts, "abc")-0.5)
	soft = checkLines(t, w)
	if len(soft) == 0 || soft[0] != 2 {
		t.Fatalf("long word: soft breaks %v", soft)
	}
	checkUpstream(t, w, soft, func(li int) float64 { return width(t, ts, w.Text()[w.Line(li).Start:w.Line(li).End]) })
}

// TestLineMetricsCJK: CJK wraps between any two ideographs; the byte
// offsets are UTF-8 (three bytes each).
func TestLineMetricsCJK(t *testing.T) {
	ts := newTestText(t)
	const s = "日本語のテキストを折り返す"
	l := dwLayout(t, ts, s, width(t, ts, "日本語の")+0.5)
	soft := checkLines(t, l)
	if l.LineCount() < 3 || len(soft) != l.LineCount()-1 {
		t.Fatalf("%d lines, soft breaks %v", l.LineCount(), soft)
	}
	for i := range l.LineCount() {
		if m := l.Line(i); m.Start%3 != 0 || m.End%3 != 0 {
			t.Errorf("Line(%d) = %+v, not at ideograph starts", i, m)
		}
	}
	if m := l.Line(0); m.End != 12 {
		t.Errorf("Line(0) = %+v, want 4 ideographs", m)
	}
	checkUpstream(t, l, soft, func(li int) float64 { return width(t, ts, s[l.Line(li).Start:l.Line(li).End]) })
}

// TestLineMetricsBidi: a mixed-direction line, wrapped and as a paragraph
// of its own, keeps the contract; the upstream caret is at the line's
// visual end from DirectWrite's geometry.
func TestLineMetricsBidi(t *testing.T) {
	ts := newTestText(t)
	const s = "ab שלום עולם cd\nאבג"
	for _, maxW := range []float64{0, width(t, ts, "ab שלום") + 1} {
		l := dwLayout(t, ts, s, maxW)
		if !l.geom.bidi {
			t.Fatal("no bidi geometry")
		}
		soft := checkLines(t, l)
		if maxW == 0 && (l.LineCount() != 2 || len(soft) != 0) {
			t.Errorf("unwrapped: %d lines, soft breaks %v", l.LineCount(), soft)
		}
		if maxW > 0 && len(soft) == 0 {
			t.Errorf("wrapped at %g: no soft break", maxW)
		}
		checkUpstream(t, l, soft, func(li int) float64 { return l.geom.lines[li].endX })
		for _, i := range soft {
			if c := l.CaretRectUpstream(i); c.X < -tol || c.X > l.Size().W+width(t, ts, " ")+tol {
				t.Errorf("upstream caret at %d = %v outside %v", i, c, l.Size())
			}
		}
	}
}

// TestLineMetricsShapes: the contract on the round-trip texts, CRLF and
// empty paragraphs included.
func TestLineMetricsShapes(t *testing.T) {
	ts := newTestText(t)
	for _, c := range []struct {
		s string
		w float64
	}{
		{"", 0}, {"", 50}, {"Hello", 0}, {"ab\n\ncd\n", 0}, {"a\r\nb", 0}, {"xéy😀z", 0},
		{"hello world again and again", 60}, {"a\u2028b", 0}, {"    ", 5},
	} {
		t.Run(fmt.Sprintf("%q/%g", c.s, c.w), func(t *testing.T) {
			l := dwLayout(t, ts, c.s, c.w)
			soft := checkLines(t, l)
			checkUpstream(t, l, soft, func(li int) float64 { return l.geom.lines[li].endX })
		})
	}
}
