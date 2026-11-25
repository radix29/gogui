package gtk4

import (
	"testing"

	"github.com/radix29/gogui/internal/render"
)

// The S5.4b host tests: Line and CaretRectUpstream over the pure geometry
// (textgeom.go) on hand-made Pango lines and clusters.

func tgLine(t *testing.T, l render.TextLayout, i int, want render.LineMetrics) {
	t.Helper()
	if got := l.Line(i); got != want {
		t.Errorf("%q: Line(%d) = %+v, want %+v", l.Text(), i, got, want)
	}
}

func tgUp(t *testing.T, l render.TextLayout, i int, want render.Rect) {
	t.Helper()
	if got := l.CaretRectUpstream(i); got != want {
		t.Errorf("%q: CaretRectUpstream(%d) = %v, want %v", l.Text(), i, got, want)
	}
}

func lm(start, end int, line float64) render.LineMetrics {
	return render.LineMetrics{Start: start, End: end, Top: line * tgH, Height: tgH, Baseline: (line + 0.75) * tgH}
}

// TestLinesWrapWhite: a line wrapped at whitespace ends where the next
// starts, its spaces included; the upstream caret at the wrap is after
// them on the earlier line, the downstream one at the next line's start.
func TestLinesWrapWhite(t *testing.T) {
	l := tgLTRLayout(t, "ab  cd\nef", [][2]int{{0, 4}, {4, 2}, {7, 2}}, 0, 1, 2, 3, 4, 5, 6, 7, 8)
	tgLine(t, l, 0, lm(0, 4, 0))
	tgLine(t, l, 1, lm(4, 6, 1))
	tgLine(t, l, 2, lm(7, 9, 2))
	tgLine(t, l, -1, lm(0, 4, 0))
	tgLine(t, l, 3, lm(7, 9, 2))
	tgUp(t, l, 4, rc(40, 0, 0, tgH))
	tgCaret(t, l, 4, rc(0, 16, 0, tgH))
	tgUp(t, l, 3, l.CaretRect(3))
	tgUp(t, l, 7, rc(0, 32, 0, tgH)) // after the '\n': a hard break
	tgUp(t, l, 0, rc(0, 0, 0, tgH))
	tgUp(t, l, 99, rc(20, 32, 0, tgH))
}

// TestLinesWrapBroken: a line broken between clusters, and an index inside
// the cluster at the break.
func TestLinesWrapBroken(t *testing.T) {
	const s = "ab☃d" // ☃ is bytes 2 to 4
	l := tgLTRLayout(t, s, [][2]int{{0, 2}, {2, 4}}, 0, 1, 2, 5)
	tgLine(t, l, 0, lm(0, 2, 0))
	tgLine(t, l, 1, lm(2, 6, 1))
	tgUp(t, l, 2, rc(20, 0, 0, tgH))
	tgUp(t, l, 3, rc(20, 0, 0, tgH))
	tgUp(t, l, 5, rc(10, 16, 0, tgH))
}

// TestLinesForcedBreak: U+2028 ends a line's text before it and is not a
// soft break.
func TestLinesForcedBreak(t *testing.T) {
	const s = "ab\u2028cd"
	pls, pcs := tgLTR(s, [][2]int{{0, 5}, {5, 2}}, []int{0, 1, 2, 5, 6}, tgW, tgH)
	l := tgLayout(t, s, pls, pcs)
	tgLine(t, l, 0, lm(0, 2, 0))
	tgLine(t, l, 1, lm(5, 7, 1))
	tgUp(t, l, 5, rc(0, 16, 0, tgH))
}

// TestLinesRTL: the upstream caret of a wrapped right-to-left line is at
// its end x as Pango gives it.
func TestLinesRTL(t *testing.T) {
	// Two RTL clusters per line, laid out right to left from x 20.
	pls := []pangoLine{
		{start: 0, length: 4, bottom: pu(tgH), baseline: pu(12), endX: pu(0)},
		{start: 4, length: 4, top: pu(tgH), bottom: pu(2 * tgH), baseline: pu(28), endX: pu(0)},
	}
	pcs := []pangoCluster{
		{start: 0, x: pu(20), width: -pu(10)}, {start: 2, x: pu(10), width: -pu(10)},
		{start: 4, x: pu(20), width: -pu(10)}, {start: 6, x: pu(10), width: -pu(10)},
	}
	l := tgLayout(t, "שלום", pls, pcs)
	tgLine(t, l, 1, render.LineMetrics{Start: 4, End: 8, Top: tgH, Height: tgH, Baseline: 28})
	tgUp(t, l, 4, rc(0, 0, 0, tgH))
	tgCaret(t, l, 4, rc(20, 16, 0, tgH))
}

// TestLinesEmpty: empty text has one line, Line clamps.
func TestLinesEmpty(t *testing.T) {
	l := tgLTRLayout(t, "", [][2]int{{0, 0}})
	tgLine(t, l, 0, lm(0, 0, 0))
	tgLine(t, l, 5, lm(0, 0, 0))
	tgUp(t, l, 0, rc(0, 0, 0, tgH))
}
