package gtk4

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/radix29/gogui/internal/render"
)

// The S4.2h host tests: the geometry (textgeom.go) on hand-made Pango
// lines and clusters, through textLayout's queries, against every rule of
// the render.TextLayout contract and Win32's textgeom.go; the validation
// of Pango's data; and NewLayout wiring the geometry in.

const (
	tgW  = 10 // the width of a cluster, in points
	tgH  = 16 // the height of a line
	tgNL = 5  // the width of a selected line break
)

// pu converts points to Pango units.
func pu(p float64) int32 { return int32(p * pangoScale) }

// tgLTR returns Pango's lines and clusters for text laid out left to
// right: lines are Pango's {start, length} (a paragraph delimiter left
// out, a forced break kept in), clusters start at cs, each w points wide,
// one after the other from x 0 of their line, and lines are h tall with
// the baseline at 3/4 of it. A
// cluster in a line's break is at the line's end; a cluster is white when
// its first character is white space.
func tgLTR(text string, lines [][2]int, cs []int, w, h float64) ([]pangoLine, []pangoCluster) {
	count := make([]int, len(lines)) // the clusters of each line's content so far
	lineOf := func(c int) int {
		li := 0
		for li+1 < len(lines) && lines[li+1][0] <= c {
			li++
		}
		return li
	}
	pcs := make([]pangoCluster, 0, len(cs))
	for _, c := range cs {
		li := lineOf(c)
		x := float64(count[li]) * w
		if c < lines[li][0]+lines[li][1] {
			count[li]++
		}
		white := false
		if c >= 0 && c < len(text) {
			r, _ := utf8.DecodeRuneInString(text[c:])
			white = unicode.IsSpace(r)
		}
		pcs = append(pcs, pangoCluster{start: int32(c), x: pu(x), width: pu(w), white: white})
	}
	pls := make([]pangoLine, len(lines))
	for li, ln := range lines {
		pls[li] = pangoLine{start: int32(ln[0]), length: int32(ln[1]),
			top: pu(float64(li) * h), bottom: pu(float64(li+1) * h), baseline: pu((float64(li) + 0.75) * h),
			endX: pu(float64(count[li]) * w)}
	}
	return pls, pcs
}

// tgRunes returns the lines and clusters of text unwrapped: one line per
// "\n", one cluster per rune (the "\n" ones included).
func tgRunes(text string) (lines [][2]int, cs []int) {
	start := 0
	for i, r := range text {
		cs = append(cs, i)
		if r == '\n' {
			lines = append(lines, [2]int{start, i - start})
			start = i + 1
		}
	}
	return append(lines, [2]int{start, len(text) - start}), cs
}

// tgLayout returns a textLayout of text over the geometry of lines and
// clusters, failing the test on a geometry error.
func tgLayout(t *testing.T, text string, lines []pangoLine, clusters []pangoCluster) *textLayout {
	t.Helper()
	g, err := newGeometry(text, lines, clusters, tgNL)
	if err != nil {
		t.Fatalf("%q: newGeometry: %v", text, err)
	}
	return &textLayout{text: text, geom: g}
}

// tgLTRLayout is tgLayout over tgLTR at tgW × tgH.
func tgLTRLayout(t *testing.T, text string, lines [][2]int, cs ...int) *textLayout {
	t.Helper()
	pls, pcs := tgLTR(text, lines, cs, tgW, tgH)
	return tgLayout(t, text, pls, pcs)
}

func rc(x, y, w, h float64) render.Rect { return render.Rect{X: x, Y: y, W: w, H: h} }

func tgHit(t *testing.T, l render.TextLayout, x, y float64, i int, trailing bool) {
	t.Helper()
	if gi, gt := l.HitTest(render.Point{X: x, Y: y}); gi != i || gt != trailing {
		t.Errorf("%q: HitTest(%v, %v) = %d %v, want %d %v", l.Text(), x, y, gi, gt, i, trailing)
	}
}

func tgCaret(t *testing.T, l render.TextLayout, i int, want render.Rect) {
	t.Helper()
	if got := l.CaretRect(i); got != want {
		t.Errorf("%q: CaretRect(%d) = %v, want %v", l.Text(), i, got, want)
	}
}

func tgSel(t *testing.T, l render.TextLayout, a, b int, want ...render.Rect) {
	t.Helper()
	if got := l.SelectionRects(a, b); !slices.Equal(got, want) {
		t.Errorf("%q: SelectionRects(%d, %d) = %v, want %v", l.Text(), a, b, got, want)
	}
}

func tgNorm(t *testing.T, l *textLayout, i, want int) {
	t.Helper()
	if got := l.normalize(i); got != want {
		t.Errorf("%q: normalize(%d) = %d, want %d", l.Text(), i, got, want)
	}
}

// TestGeomOneLine: both halves of every cluster; points above, below,
// left and right of the line, NaN and infinite ones; clamped carets and
// indexes; selections in either order, empty, clamped.
func TestGeomOneLine(t *testing.T) {
	const s = "Hello"
	l := tgLTRLayout(t, s, [][2]int{{0, 5}}, 0, 1, 2, 3, 4)
	if l.LineCount() != 1 {
		t.Errorf("LineCount %d, want 1", l.LineCount())
	}
	for i := range len(s) {
		x := float64(i * tgW)
		tgHit(t, l, x+2, 8, i, false)
		tgHit(t, l, x+tgW/2, 8, i, true) // the middle is trailing
		tgHit(t, l, x+8, 8, i, true)
		tgCaret(t, l, i, rc(x, 0, 0, tgH))
	}
	tgCaret(t, l, 5, rc(50, 0, 0, tgH))
	tgHit(t, l, 12, -100, 1, false) // above: the first line
	tgHit(t, l, 12, 1e6, 1, false)  // below: the last line
	tgHit(t, l, -50, 8, 0, false)   // left: the line's start
	tgHit(t, l, 0, 8, 0, false)
	tgHit(t, l, 1e6, 8, 5, false) // right: the line's end
	tgHit(t, l, 50, 8, 5, false)
	tgHit(t, l, math.NaN(), math.NaN(), 0, false)
	tgHit(t, l, math.Inf(1), math.Inf(-1), 5, false)
	tgHit(t, l, math.Inf(-1), math.Inf(1), 0, false)
	tgHit(t, l, 12, math.NaN(), 1, false)
	for _, c := range []struct{ i, want int }{{-1, 0}, {-1 << 40, 0}, {0, 0}, {3, 3}, {5, 5}, {6, 5}, {1 << 40, 5}} {
		tgNorm(t, l, c.i, c.want)
	}
	tgCaret(t, l, -1, rc(0, 0, 0, tgH))
	tgCaret(t, l, 99, rc(50, 0, 0, tgH))
	tgSel(t, l, 1, 3, rc(10, 0, 20, tgH))
	tgSel(t, l, 3, 1, rc(10, 0, 20, tgH))
	tgSel(t, l, 2, 2)
	tgSel(t, l, -5, 99, rc(0, 0, 50, tgH))
	tgSel(t, l, 5, 9) // both clamped to the end: empty
	tgSel(t, l, -9, 0)
}

// TestGeomEmpty: empty text has one line; every caret is at its start,
// every hit is 0, and nothing is selected.
func TestGeomEmpty(t *testing.T) {
	l := tgLTRLayout(t, "", [][2]int{{0, 0}})
	if l.LineCount() != 1 {
		t.Errorf("LineCount %d, want 1", l.LineCount())
	}
	for _, i := range []int{0, -1, 5} {
		tgCaret(t, l, i, rc(0, 0, 0, tgH))
		tgNorm(t, l, i, 0)
	}
	tgHit(t, l, 10, 10, 0, false)
	tgHit(t, l, -10, 100, 0, false)
	tgSel(t, l, 0, 5)
}

// TestGeomClusters: an index inside a cluster (a multi-byte rune, a
// combining mark, a ZWJ emoji sequence) is moved to its start by every
// query; hits never land inside one.
func TestGeomClusters(t *testing.T) {
	// "\u00e9" is two bytes.
	l := tgLTRLayout(t, "a\u00e9b", [][2]int{{0, 4}}, 0, 1, 3)
	tgNorm(t, l, 2, 1)
	tgNorm(t, l, 3, 3)
	tgCaret(t, l, 2, rc(10, 0, 0, tgH))
	tgHit(t, l, 12, 8, 1, false)
	tgHit(t, l, 18, 8, 1, true)
	tgSel(t, l, 2, 3, rc(10, 0, 10, tgH))

	// "e" and U+0301 (two bytes) are one grapheme.
	l = tgLTRLayout(t, "e\u0301x", [][2]int{{0, 4}}, 0, 3)
	for _, c := range []struct{ i, want int }{{0, 0}, {1, 0}, {2, 0}, {3, 3}, {4, 4}} {
		tgNorm(t, l, c.i, c.want)
	}
	tgCaret(t, l, 2, rc(0, 0, 0, tgH))
	tgCaret(t, l, 3, rc(10, 0, 0, tgH))
	tgHit(t, l, 8, 8, 0, true)
	tgSel(t, l, 1, 4, rc(0, 0, 20, tgH))
	tgSel(t, l, 1, 2) // both at the grapheme's start: empty

	// 👩 ZWJ 💻 (4 + 3 + 4 bytes) is one cluster.
	const zwj = "a\U0001F469\u200d\U0001F4BBb"
	l = tgLTRLayout(t, zwj, [][2]int{{0, len(zwj)}}, 0, 1, 12)
	for i := 1; i < 12; i++ {
		tgNorm(t, l, i, 1)
		tgCaret(t, l, i, rc(10, 0, 0, tgH))
	}
	tgNorm(t, l, 12, 12)
	tgHit(t, l, 18, 8, 1, true)
	tgHit(t, l, 21, 8, 12, false)
	tgSel(t, l, 3, 12, rc(10, 0, 10, tgH))
	tgSel(t, l, 5, 13, rc(10, 0, 20, tgH))
}

// TestGeomNewlines: "\n" lines, a blank line and a trailing "\n" (an empty
// last line): right of a line is its end before the "\n"; a selected "\n"
// shows tgNL past the line's end, a blank line only its "\n", the empty
// last line nothing.
func TestGeomNewlines(t *testing.T) {
	const s = "ab\n\ncd\n"
	lines, cs := tgRunes(s)
	l := tgLTRLayout(t, s, lines, cs...)
	if l.LineCount() != 4 {
		t.Fatalf("LineCount %d, want 4", l.LineCount())
	}
	tgHit(t, l, 100, 1, 2, false)
	tgHit(t, l, 100, 17, 3, false)
	tgHit(t, l, -1, 17, 3, false)
	tgHit(t, l, 100, 16, 3, false) // a line's top is in it
	tgHit(t, l, 100, 33, 6, false)
	tgHit(t, l, 1, 33, 4, false)
	tgHit(t, l, 100, 49, 7, false)
	tgHit(t, l, 100, 1e6, 7, false)
	for _, c := range []struct {
		i    int
		want render.Rect
	}{
		{0, rc(0, 0, 0, tgH)}, {2, rc(20, 0, 0, tgH)}, {3, rc(0, 16, 0, tgH)},
		{4, rc(0, 32, 0, tgH)}, {6, rc(20, 32, 0, tgH)}, {7, rc(0, 48, 0, tgH)},
		{-5, rc(0, 0, 0, tgH)}, {99, rc(0, 48, 0, tgH)},
	} {
		tgCaret(t, l, c.i, c.want)
	}
	tgNorm(t, l, 2, 2)
	tgNorm(t, l, 3, 3)
	tgSel(t, l, 0, 7, rc(0, 0, 20+tgNL, tgH), rc(0, 16, tgNL, tgH), rc(0, 32, 20+tgNL, tgH))
	tgSel(t, l, 7, 0, rc(0, 0, 20+tgNL, tgH), rc(0, 16, tgNL, tgH), rc(0, 32, 20+tgNL, tgH))
	tgSel(t, l, 2, 3, rc(20, 0, tgNL, tgH))
	tgSel(t, l, 1, 2, rc(10, 0, 10, tgH)) // the "\n" not selected
	tgSel(t, l, 3, 4, rc(0, 16, tgNL, tgH))
	tgSel(t, l, 4, 6, rc(0, 32, 20, tgH))
	tgSel(t, l, 6, 7, rc(20, 32, tgNL, tgH))
	tgSel(t, l, 7, 7)
	tgSel(t, l, 7, 99)
}

// TestGeomCRLF: "\r\n" is one break: an index between its bytes is its
// start.
func TestGeomCRLF(t *testing.T) {
	const s = "ab\r\ncd"
	l := tgLTRLayout(t, s, [][2]int{{0, 2}, {4, 2}}, 0, 1, 2, 4, 5)
	tgNorm(t, l, 3, 2)
	tgNorm(t, l, 4, 4)
	tgCaret(t, l, 3, rc(20, 0, 0, tgH))
	tgCaret(t, l, 4, rc(0, 16, 0, tgH))
	tgHit(t, l, 100, 1, 2, false)
	tgHit(t, l, 100, 20, 6, false)
	tgSel(t, l, 1, 4, rc(10, 0, 10+tgNL, tgH))
	tgSel(t, l, 3, 5, rc(20, 0, tgNL, tgH), rc(0, 16, 10, tgH))
	tgSel(t, l, 2, 3) // both the break's start
}

// TestGeomWrapWhite: a line wrapped at whitespace keeps the caret on it:
// right of it (or on its last white cluster) is the start of that
// cluster, trailing false; the wrap point's caret is at the next line's
// start.
func TestGeomWrapWhite(t *testing.T) {
	l := tgLTRLayout(t, "ab cd", [][2]int{{0, 3}, {3, 2}}, 0, 1, 2, 3, 4)
	tgHit(t, l, 100, 1, 2, false)
	tgHit(t, l, 25, 1, 2, false) // on the space
	tgHit(t, l, 18, 1, 1, true)
	tgHit(t, l, 100, 17, 5, false)
	tgHit(t, l, -1, 17, 3, false)
	tgCaret(t, l, 2, rc(20, 0, 0, tgH))
	tgCaret(t, l, 3, rc(0, 16, 0, tgH))
	tgCaret(t, l, 5, rc(20, 16, 0, tgH))
	tgSel(t, l, 0, 5, rc(0, 0, 30, tgH), rc(0, 16, 20, tgH))
	tgSel(t, l, 2, 3, rc(20, 0, 10, tgH))
	tgSel(t, l, 3, 4, rc(0, 16, 10, tgH))

	// Two spaces: the last one's start.
	l = tgLTRLayout(t, "ab  cd", [][2]int{{0, 4}, {4, 2}}, 0, 1, 2, 3, 4, 5)
	tgHit(t, l, 100, 1, 3, false)
	tgHit(t, l, 22, 1, 2, false)
	tgHit(t, l, 28, 1, 2, true)
}

// TestGeomWrapBroken: a line broken between clusters: right of it is the
// next line's start, whose caret is drawn there.
func TestGeomWrapBroken(t *testing.T) {
	l := tgLTRLayout(t, "abcd", [][2]int{{0, 2}, {2, 2}}, 0, 1, 2, 3)
	tgHit(t, l, 100, 1, 2, false)
	tgHit(t, l, -1, 17, 2, false)
	tgHit(t, l, 100, 17, 4, false)
	tgCaret(t, l, 1, rc(10, 0, 0, tgH))
	tgCaret(t, l, 2, rc(0, 16, 0, tgH))
	tgCaret(t, l, 4, rc(20, 16, 0, tgH))
	tgSel(t, l, 0, 2, rc(0, 0, 20, tgH))
	tgSel(t, l, 1, 3, rc(10, 0, 10, tgH), rc(0, 16, 10, tgH))
	tgSel(t, l, 2, 4, rc(0, 16, 20, tgH))
}

// TestGeomForcedBreak: Pango keeps a forced break inside a paragraph
// (U+2028) in its line's content, so the content reaches the next line's
// start; the geometry leaves it out: it is a break like "\n", the line's
// end at its box's leading edge (not Pango's endX), or at the last
// cluster's trailing edge without a box, or at 0 on a line of only the
// break.
func TestGeomForcedBreak(t *testing.T) {
	const s = "ab\u2028cd" // U+2028 is bytes 2 to 4
	pls, pcs := tgLTR(s, [][2]int{{0, 5}, {5, 2}}, []int{0, 1, 2, 5, 6}, tgW, tgH)
	pls[0].endX = pu(99) // past the break: not used
	pcs[2].x = pu(21)    // the break's box
	l := tgLayout(t, s, pls, pcs)
	if l.LineCount() != 2 {
		t.Fatalf("LineCount %d, want 2", l.LineCount())
	}
	tgNorm(t, l, 3, 2)
	tgNorm(t, l, 4, 2)
	tgNorm(t, l, 5, 5)
	tgCaret(t, l, 2, rc(21, 0, 0, tgH))
	tgCaret(t, l, 4, rc(21, 0, 0, tgH))
	tgCaret(t, l, 5, rc(0, 16, 0, tgH))
	tgHit(t, l, 100, 1, 2, false)
	tgHit(t, l, 25, 1, 2, false) // on the break's box
	tgHit(t, l, 100, 17, 7, false)
	tgSel(t, l, 0, 7, rc(0, 0, 21+tgNL, tgH), rc(0, 16, 20, tgH))
	tgSel(t, l, 2, 5, rc(21, 0, tgNL, tgH))
	tgSel(t, l, 0, 2, rc(0, 0, 21, tgH))

	// Without the break's box: the last cluster's trailing edge.
	pls, pcs = tgLTR(s, [][2]int{{0, 5}, {5, 2}}, []int{0, 1, 5, 6}, tgW, tgH)
	pls[0].endX = pu(99)
	l = tgLayout(t, s, pls, pcs)
	tgCaret(t, l, 2, rc(20, 0, 0, tgH))
	tgNorm(t, l, 3, 2)
	tgSel(t, l, 1, 5, rc(10, 0, 10+tgNL, tgH))

	// A line of only the break, with its box and without.
	const only = "\u2028x"
	for _, cs := range [][]int{{0, 3}, {3}} {
		pls, pcs = tgLTR(only, [][2]int{{0, 3}, {3, 1}}, cs, tgW, tgH)
		pls[0].endX = pu(10)
		l = tgLayout(t, only, pls, pcs)
		tgCaret(t, l, 1, rc(0, 0, 0, tgH))
		tgNorm(t, l, 2, 0)
		tgHit(t, l, 100, 1, 0, false)
		tgSel(t, l, 0, 3, rc(0, 0, tgNL, tgH))
		tgSel(t, l, 0, 4, rc(0, 0, tgNL, tgH), rc(0, 16, 10, tgH))
	}

	// VT, FF and NEL are not forced breaks (Pango does not break at
	// them): ordinary white clusters, kept in the content, so a line
	// wrapped after one is wrapped at whitespace and shows no break.
	for _, s := range []string{"a\vb", "a\fb", "a\u0085b"} {
		k := strings.IndexByte(s, 'b')
		l := tgLTRLayout(t, s, [][2]int{{0, len(s)}}, 0, 1, k)
		tgNorm(t, l, k-1, 1)
		tgCaret(t, l, k, rc(20, 0, 0, tgH))
		tgHit(t, l, 15, 1, 1, true)
		tgHit(t, l, 100, 1, len(s), false)
		tgSel(t, l, 0, len(s), rc(0, 0, 30, tgH))

		l = tgLTRLayout(t, s, [][2]int{{0, k}, {k, 1}}, 0, 1, k)
		tgNorm(t, l, k-1, 1)
		tgCaret(t, l, 1, rc(10, 0, 0, tgH))
		tgHit(t, l, 100, 1, 1, false) // the start of the white cluster
		tgSel(t, l, 0, k, rc(0, 0, 20, tgH))
	}
	// A wrapped line ending in another character keeps its content.
	l = tgLTRLayout(t, "ab\u2027cd", [][2]int{{0, 5}, {5, 2}}, 0, 1, 2, 5, 6)
	tgHit(t, l, 100, 1, 5, false) // broken between clusters
	tgCaret(t, l, 2, rc(20, 0, 0, tgH))
	tgSel(t, l, 0, 5, rc(0, 0, 30, tgH))
}

// TestIsForcedBreak: only LS breaks inside a paragraph (Pango does not
// break at VT, FF or NEL); the paragraph delimiters do not either.
func TestIsForcedBreak(t *testing.T) {
	for r, want := range map[rune]bool{
		0x2028: true, '\v': false, '\f': false, 0x85: false,
		'\n': false, '\r': false, 0x2029: false, ' ': false, 'a': false, 0x2027: false,
	} {
		if got := isForcedBreak(r); got != want {
			t.Errorf("isForcedBreak(%U) = %v, want %v", r, got, want)
		}
	}
}

// TestGeomRTL: clusters Pango gives with a negative width are right to
// left: the caret at their leading (right) edge, the right half of a
// cluster is its leading one, and a selection spans its clusters' boxes.
func TestGeomRTL(t *testing.T) {
	const s = "ab\u05d0\u05d1" // U+05D0 at 2, U+05D1 at 4: shown right to left after "ab"
	pls := []pangoLine{{start: 0, length: 6, bottom: pu(tgH), endX: pu(40)}}
	pcs := []pangoCluster{
		{start: 0, x: 0, width: pu(10)},
		{start: 1, x: pu(10), width: pu(10)},
		{start: 2, x: pu(40), width: -pu(10)}, // [30, 40]
		{start: 4, x: pu(30), width: -pu(10)}, // [20, 30]
	}
	l := tgLayout(t, s, pls, pcs)
	if !l.geom.bidi {
		t.Error("not bidi")
	}
	tgCaret(t, l, 1, rc(10, 0, 0, tgH))
	tgCaret(t, l, 2, rc(40, 0, 0, tgH))
	tgCaret(t, l, 3, rc(40, 0, 0, tgH))
	tgCaret(t, l, 4, rc(30, 0, 0, tgH))
	tgCaret(t, l, 6, rc(40, 0, 0, tgH))
	tgNorm(t, l, 5, 4)
	tgHit(t, l, 38, 8, 2, false)
	tgHit(t, l, 35, 8, 2, false) // the middle of an RTL cluster is its leading half
	tgHit(t, l, 32, 8, 2, true)
	tgHit(t, l, 28, 8, 4, false)
	tgHit(t, l, 22, 8, 4, true)
	tgHit(t, l, 12, 8, 1, false)
	tgHit(t, l, 100, 8, 6, false)
	tgSel(t, l, 2, 6, rc(20, 0, 20, tgH))
	tgSel(t, l, 6, 2, rc(20, 0, 20, tgH))
	tgSel(t, l, 1, 4, rc(10, 0, 30, tgH)) // b and 05d0: one span
	tgSel(t, l, 4, 6, rc(20, 0, 10, tgH))
	tgSel(t, l, 0, 1, rc(0, 0, 10, tgH))
	tgSel(t, l, 3, 3)

	// Bidi spans per line, with the selected break.
	const s2 = "\u05d0\u05d1\nab"
	pls = []pangoLine{
		{start: 0, length: 4, bottom: pu(tgH), endX: 0},
		{start: 5, length: 2, top: pu(tgH), bottom: pu(2 * tgH), endX: pu(20)},
	}
	pcs = []pangoCluster{
		{start: 0, x: pu(20), width: -pu(10)},
		{start: 2, x: pu(10), width: -pu(10)},
		{start: 4, x: 0, width: pu(5)}, // the "\n"
		{start: 5, x: 0, width: pu(10)},
		{start: 6, x: pu(10), width: pu(10)},
	}
	l = tgLayout(t, s2, pls, pcs)
	tgCaret(t, l, 0, rc(20, 0, 0, tgH))
	tgCaret(t, l, 4, rc(0, 0, 0, tgH))
	tgHit(t, l, 100, 1, 4, false)
	tgSel(t, l, 0, 6, rc(0, 0, 20, tgH), rc(0, 16, 10, tgH))
	tgSel(t, l, 2, 7, rc(0, 0, 10, tgH), rc(0, 16, 20, tgH))
}

// TestGeomErrors: Pango data that does not cover the text is rejected.
func TestGeomErrors(t *testing.T) {
	ln := func(start, length int32) pangoLine { return pangoLine{start: start, length: length, bottom: pu(tgH)} }
	cl := func(starts ...int32) []pangoCluster {
		cs := make([]pangoCluster, len(starts))
		for i, s := range starts {
			cs[i] = pangoCluster{start: s, x: pu(float64(i * tgW)), width: pu(tgW)}
		}
		return cs
	}
	for _, tc := range []struct {
		name     string
		text     string
		lines    []pangoLine
		clusters []pangoCluster
	}{
		{"no lines", "a", nil, cl(0)},
		{"no lines, empty text", "", nil, nil},
		{"first line not at 0", "abc", []pangoLine{ln(1, 2)}, cl(1, 2)},
		{"negative length", "abc", []pangoLine{ln(0, -1), ln(1, 2)}, cl(0, 1, 2)},
		{"last line short of the end", "abc", []pangoLine{ln(0, 2)}, cl(0, 1)},
		{"last line past the end", "abc", []pangoLine{ln(0, 4)}, cl(0, 1, 2)},
		{"content past the next line's start", "abc", []pangoLine{ln(0, 3), ln(2, 1)}, cl(0, 1, 2)},
		{"next line past the end", "abc", []pangoLine{ln(0, 3), ln(5, 0)}, cl(0, 1, 2)},
		{"lines out of order", "abcd", []pangoLine{ln(0, 2), ln(3, 1), ln(2, 1)}, cl(0, 1, 2, 3)},
		{"clusters out of order", "abc", []pangoLine{ln(0, 3)}, cl(0, 2, 1)},
		{"duplicate cluster", "abc", []pangoLine{ln(0, 3)}, cl(0, 1, 1, 2)},
		{"clusters out of order across lines", "ab\ncd", []pangoLine{ln(0, 2), ln(3, 2)}, cl(0, 1, 3, 2, 4)},
		{"negative cluster", "abc", []pangoLine{ln(0, 3)}, cl(-1, 0, 1, 2)},
		{"no cluster at the start", "abc", []pangoLine{ln(0, 3)}, cl(1, 2)},
		{"no clusters", "abc", []pangoLine{ln(0, 3)}, nil},
		{"no cluster at a line's start", "ab\ncd", []pangoLine{ln(0, 2), ln(3, 2)}, cl(0, 1, 2, 4)},
		{"no cluster at a wrapped line's start", "abcd", []pangoLine{ln(0, 2), ln(2, 2)}, cl(0, 1, 3)},
		{"cluster at the end", "ab", []pangoLine{ln(0, 2)}, cl(0, 1, 2)},
		{"cluster past the end", "ab", []pangoLine{ln(0, 2)}, cl(0, 1, 5)},
		{"cluster in an empty text", "", []pangoLine{ln(0, 0)}, cl(0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := newGeometry(tc.text, tc.lines, tc.clusters, tgNL); !errors.Is(err, errGeometry) {
				t.Errorf("newGeometry = %v, want errGeometry", err)
			}
		})
	}
	// The valid neighbours of those cases.
	for _, tc := range []struct {
		name     string
		text     string
		lines    []pangoLine
		clusters []pangoCluster
	}{
		{"empty text", "", []pangoLine{ln(0, 0)}, nil},
		{"empty last line", "a\n", []pangoLine{ln(0, 1), ln(2, 0)}, cl(0, 1)},
		{"blank line without a cluster", "a\n\nb", []pangoLine{ln(0, 1), ln(2, 0), ln(3, 1)}, cl(0, 3)},
	} {
		if _, err := newGeometry(tc.text, tc.lines, tc.clusters, tgNL); err != nil {
			t.Errorf("%s: newGeometry = %v, want no error", tc.name, err)
		}
	}
}

// TestGeomNewLayout: NewLayout builds the geometry from the layouter's
// lines and clusters, with the break width of the font's size, and the
// layout's queries answer from it.
func TestGeomNewLayout(t *testing.T) {
	const s = "ab cd\nef"
	pls, pcs := tgLTR(s, [][2]int{{0, 3}, {3, 2}, {6, 2}}, []int{0, 1, 2, 3, 4, 5, 6, 7}, tgW, tgH)
	pl := &txLayouter{m: pangoMetrics{width: pu(30), height: pu(3 * tgH), baseline: pu(12), lines: pls, clusters: pcs}}
	ts, lg := txSystem(pl, nil, nil)
	l := ts.NewLayout(s, render.Font{Size: 20}, 30)
	tl, ok := l.(*textLayout)
	if !ok {
		t.Fatalf("layout %T, want *textLayout", l)
	}
	if len(lg.msgs) != 0 {
		t.Errorf("logged %q", lg.msgs)
	}
	if l.LineCount() != 3 || tl.geom.nlWidth != 10 {
		t.Errorf("LineCount %d, nlWidth %v; want 3, 10 (half of 20 pt)", l.LineCount(), tl.geom.nlWidth)
	}
	tgHit(t, l, 100, 1, 2, false) // wrapped at the space
	tgHit(t, l, 100, 20, 5, false)
	tgHit(t, l, 12, 40, 7, false)
	tgCaret(t, l, 3, rc(0, 16, 0, tgH))
	tgCaret(t, l, 99, rc(20, 32, 0, tgH))
	tgSel(t, l, 4, 7, rc(10, 16, 10+10, tgH), rc(0, 32, 10, tgH))
	tgNorm(t, tl, -4, 0)
	tgNorm(t, tl, 99, len(s))
}
