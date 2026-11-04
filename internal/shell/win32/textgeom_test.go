//go:build windows

package win32

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/render"
)

// The S2.5e tests: HitTest, CaretRect and SelectionRects against every
// sentence of the render.TextLayout contract, on real DirectWrite layouts
// (Segoe UI Variable / Segoe UI at 13 DIPs). Expected x positions come from
// summing the layout of prefixes, so the tests do not hard-code font metrics.

const tol = 0.01

// dwLayout returns a DirectWrite layout of s.
func dwLayout(t *testing.T, ts *textSystem, s string, maxWidth float64) *layout {
	t.Helper()
	l, ok := ts.NewLayout(s, render.Font{}, maxWidth).(*layout)
	if !ok {
		t.Fatalf("%q: not a DirectWrite layout", s)
	}
	return l
}

// width returns the unwrapped width of s, trailing whitespace included.
func width(t *testing.T, ts *textSystem, s string) float64 {
	t.Helper()
	l := dwLayout(t, ts, s+"|", 0)
	bar := dwLayout(t, ts, "|", 0).Size().W
	return l.Size().W - bar
}

func nearRect(a, b render.Rect) bool {
	return near(a.X, b.X, tol) && near(a.Y, b.Y, tol) && near(a.W, b.W, tol) && near(a.H, b.H, tol)
}

func wantHit(t *testing.T, l *layout, p render.Point, i int, trailing bool) {
	t.Helper()
	if gi, gt := l.HitTest(p); gi != i || gt != trailing {
		t.Errorf("%q: HitTest(%v) = %d %v, want %d %v", l.Text(), p, gi, gt, i, trailing)
	}
}

func wantCaret(t *testing.T, l *layout, i int, want render.Rect) {
	t.Helper()
	if got := l.CaretRect(i); !nearRect(got, want) {
		t.Errorf("%q: CaretRect(%d) = %v, want %v", l.Text(), i, got, want)
	}
}

func wantSel(t *testing.T, l *layout, a, b int, want ...render.Rect) {
	t.Helper()
	got := l.SelectionRects(a, b)
	ok := len(got) == len(want)
	for k := 0; ok && k < len(got); k++ {
		ok = nearRect(got[k], want[k])
	}
	if !ok {
		t.Errorf("%q: SelectionRects(%d, %d) = %v, want %v", l.Text(), a, b, got, want)
	}
}

// TestHitOneLine: both halves of every cluster, and points above, below,
// left and right of the line.
func TestHitOneLine(t *testing.T) {
	ts := newTestText(t)
	const s = "Hello"
	l := dwLayout(t, ts, s, 0)
	h := l.Size().H
	for i := range len(s) {
		x0, x1 := width(t, ts, s[:i]), width(t, ts, s[:i+1])
		q := (x1 - x0) / 4
		wantHit(t, l, render.Point{X: x0 + q, Y: h / 2}, i, false)
		wantHit(t, l, render.Point{X: x1 - q, Y: h / 2}, i, true)
		wantCaret(t, l, i, rect(x0, 0, 0, h))
	}
	wantCaret(t, l, len(s), rect(l.Size().W, 0, 0, h))
	wantHit(t, l, render.Point{X: 1, Y: -100}, 0, false) // above: the first line
	wantHit(t, l, render.Point{X: 1, Y: 1e6}, 0, false)  // below: the last line
	wantHit(t, l, render.Point{X: -50, Y: 5}, 0, false)  // left: the first cluster
	wantHit(t, l, render.Point{X: 1e6, Y: 5}, len(s), false)
	wantHit(t, l, render.Point{X: l.Size().W + 0.5, Y: -3}, len(s), false)
	wantHit(t, l, render.Point{X: math.NaN(), Y: math.NaN()}, 0, false)
	wantHit(t, l, render.Point{X: math.Inf(1), Y: math.Inf(-1)}, len(s), false)
}

// TestHitLines: '\n' lines, a blank line and a trailing newline; right of a
// line is its end before the '\n' with trailing false.
func TestHitLines(t *testing.T) {
	ts := newTestText(t)
	l := dwLayout(t, ts, "ab\n\ncd\n", 0)
	h := l.Size().H / 4
	ab, cd := width(t, ts, "ab"), width(t, ts, "cd")
	wantHit(t, l, render.Point{X: 100, Y: 1}, 2, false)
	wantHit(t, l, render.Point{X: 100, Y: h + 1}, 3, false)
	wantHit(t, l, render.Point{X: -1, Y: h + 1}, 3, false)
	wantHit(t, l, render.Point{X: 100, Y: 2*h + 1}, 6, false)
	wantHit(t, l, render.Point{X: 1, Y: 2*h + 1}, 4, false)
	wantHit(t, l, render.Point{X: 100, Y: 3*h + 1}, 7, false)
	wantHit(t, l, render.Point{X: 100, Y: 1e6}, 7, false)
	for _, c := range []struct {
		i    int
		want render.Rect
	}{
		{0, rect(0, 0, 0, h)}, {2, rect(ab, 0, 0, h)}, {3, rect(0, h, 0, h)},
		{4, rect(0, 2*h, 0, h)}, {6, rect(cd, 2*h, 0, h)}, {7, rect(0, 3*h, 0, h)},
		{-5, rect(0, 0, 0, h)}, {99, rect(0, 3*h, 0, h)},
	} {
		wantCaret(t, l, c.i, c.want)
	}
	nl := newlineWidth(13)
	// A selected '\n' shows nl past the line's end; a blank line shows only
	// its '\n'; the empty last line shows nothing.
	b := width(t, ts, "b")
	wantSel(t, l, 1, 5, rect(ab-b, 0, b+nl, h), rect(0, h, nl, h), rect(0, 2*h, width(t, ts, "c"), h))
	wantSel(t, l, 5, 1, rect(ab-b, 0, b+nl, h), rect(0, h, nl, h), rect(0, 2*h, width(t, ts, "c"), h))
	wantSel(t, l, 4, 7, rect(0, 2*h, cd+nl, h))
	wantSel(t, l, 0, 2, rect(0, 0, ab, h)) // up to the '\n', not over it
	wantSel(t, l, 2, 3, rect(ab, 0, nl, h))
	wantSel(t, l, 3, 3)
	wantSel(t, l, 7, 99)
	wantSel(t, l, -10, 99, rect(0, 0, ab+nl, h), rect(0, h, nl, h), rect(0, 2*h, cd+nl, h))
}

// TestHitCRLF: "\r\n" is one break; an index between '\r' and '\n' moves to
// the line's end.
func TestHitCRLF(t *testing.T) {
	ts := newTestText(t)
	l := dwLayout(t, ts, "a\r\nb", 0)
	h := l.Size().H / 2
	a := width(t, ts, "a")
	wantHit(t, l, render.Point{X: 100, Y: 1}, 1, false)
	wantCaret(t, l, 2, rect(a, 0, 0, h))
	wantCaret(t, l, 3, rect(0, h, 0, h))
	if n := l.normalize(2); n != 1 {
		t.Errorf("normalize(2) = %d, want 1", n)
	}
	wantSel(t, l, 0, 4, rect(0, 0, a+newlineWidth(13), h), rect(0, h, width(t, ts, "b"), h))
	wantSel(t, l, 1, 2) // inside the break: empty
}

// TestHitWrap: wrapping at spaces and between clusters; the caret at a wrap
// point is at the next line's start, and right of a wrapped line stays on it.
func TestHitWrap(t *testing.T) {
	ts := newTestText(t)
	const s = "hello world  again"
	maxW := width(t, ts, "hello world") + 1
	l := dwLayout(t, ts, s, maxW)
	if l.LineCount() != 2 {
		// "hello world  " fits on line 1 (the spaces hang), "again" on line 2.
		t.Fatalf("%d lines", l.LineCount())
	}
	h := l.Size().H / 2
	wantCaret(t, l, 13, rect(0, h, 0, h)) // the wrap point
	wantCaret(t, l, 12, rect(width(t, ts, "hello world "), 0, 0, h))
	wantHit(t, l, render.Point{X: 1e6, Y: 1}, 12, false) // before the last space
	wantHit(t, l, render.Point{X: width(t, ts, "hello world") + 1, Y: 1}, 11, false)
	wantHit(t, l, render.Point{X: 1e6, Y: h + 1}, len(s), false)
	wantHit(t, l, render.Point{X: -1, Y: h + 1}, 13, false)
	ws := width(t, ts, "hello world  ")
	wantSel(t, l, 0, len(s), rect(0, 0, ws, h), rect(0, h, width(t, ts, "again"), h))
	wantSel(t, l, 11, 13, rect(width(t, ts, "hello world"), 0, ws-width(t, ts, "hello world"), h))
	wantSel(t, l, 13, 14, rect(0, h, width(t, ts, "a"), h))

	// A word longer than the width breaks between clusters: right of such a
	// line is the next line's start, drawn there.
	w := dwLayout(t, ts, "abcdefghij", width(t, ts, "abc")-0.5)
	if w.LineCount() < 2 {
		t.Fatalf("long word: %d lines", w.LineCount())
	}
	h = w.Size().H / float64(w.LineCount())
	wantHit(t, w, render.Point{X: 1e6, Y: 1}, 2, false)
	wantCaret(t, w, 2, rect(0, h, 0, h))
	wantCaret(t, w, 1, rect(width(t, ts, "a"), 0, 0, h))
	wantSel(t, w, 0, 2, rect(0, 0, width(t, ts, "ab"), h))
	wantSel(t, w, 2, 3, rect(0, h, width(t, ts, "c"), h))
}

// TestHitClusters: indices inside a rune or a cluster move to its start, a
// cluster is hit as a whole, and a selection never splits one.
func TestHitClusters(t *testing.T) {
	ts := newTestText(t)
	const s = "xéy😀z" // x, e+U+0301 (3 bytes: 1+2), y, U+1F600 (4 bytes), z
	l := dwLayout(t, ts, s, 0)
	h := l.Size().H
	starts := []int{0, 1, 4, 5, 9, 10}
	for i := 0; i <= len(s); i++ {
		want := 0
		for _, st := range starts {
			if st <= i {
				want = st
			}
		}
		if got := l.normalize(i); got != want {
			t.Errorf("normalize(%d) = %d, want %d", i, got, want)
		}
		wantCaret(t, l, i, rect(width(t, ts, s[:want]), 0, 0, h))
	}
	ex, e1 := width(t, ts, "x"), width(t, ts, "xé")
	wantHit(t, l, render.Point{X: ex + 0.5, Y: 1}, 1, false)
	wantHit(t, l, render.Point{X: e1 - 0.5, Y: 1}, 1, true)
	y1, em := width(t, ts, "xéy"), width(t, ts, "xéy😀")
	wantHit(t, l, render.Point{X: y1 + 1, Y: 1}, 5, false)
	wantHit(t, l, render.Point{X: em - 1, Y: 1}, 5, true)
	// Selecting from inside the accent to inside the emoji covers e+accent
	// and y, not the emoji.
	wantSel(t, l, 2, 7, rect(ex, 0, y1-ex, h))
	wantSel(t, l, 6, 8) // both ends in the emoji
	wantSel(t, l, 7, 9, rect(y1, 0, em-y1, h))
	wantSel(t, l, 7, 10, rect(y1, 0, l.Size().W-y1, h))
	// An invalid byte is one cluster of its own.
	b := dwLayout(t, ts, "a\xffb", 0)
	if b.normalize(2) != 2 || b.normalize(1) != 1 {
		t.Errorf("invalid byte: normalize 1→%d, 2→%d", b.normalize(1), b.normalize(2))
	}
}

// TestHitEmpty: empty text hits 0, its caret is at 0 one line high, and
// nothing is selected; clamping applies.
func TestHitEmpty(t *testing.T) {
	ts := newTestText(t)
	for _, w := range []float64{0, 50} {
		l := dwLayout(t, ts, "", w)
		h := l.Size().H
		for _, p := range []render.Point{{}, {X: 50, Y: 50}, {X: -5, Y: -5}, {X: math.NaN(), Y: 3}} {
			wantHit(t, l, p, 0, false)
		}
		for _, i := range []int{-1, 0, 1, 99} {
			wantCaret(t, l, i, rect(0, 0, 0, h))
		}
		wantSel(t, l, 0, 0)
		wantSel(t, l, -5, 5)
	}
}

// TestHitBidi: mixed directions take each cluster's box from DirectWrite;
// hits round-trip and selection rects stay inside the text.
func TestHitBidi(t *testing.T) {
	ts := newTestText(t)
	const s = "ab שלום cd"
	l := dwLayout(t, ts, s, 0)
	if !l.geom.bidi {
		t.Fatal("no bidi geometry")
	}
	h := l.Size().H
	heb := strings.Index(s, "ש")
	// The first Hebrew letter is the rightmost of its run: its leading
	// (right) half is trailing false.
	c := l.CaretRect(heb)
	wantHit(t, l, render.Point{X: c.X - 1, Y: 1}, heb, false)
	wantHit(t, l, render.Point{X: 1e6, Y: 1}, len(s), false)
	for _, r := range l.SelectionRects(0, len(s)) {
		if r.X < -tol || r.X+r.W > l.Size().W+tol || r.H != h {
			t.Errorf("selection rect %v outside %v", r, l.Size())
		}
	}
	roundTrip(t, l)
}

// roundTrip: HitTest just inside each cluster start's caret (in its
// cluster's leading half) gives back that index with trailing false.
func roundTrip(t *testing.T, l *layout) {
	t.Helper()
	for i := 0; i <= len(l.text); i++ {
		n := l.normalize(i)
		if n != i {
			continue
		}
		c := l.CaretRect(i)
		x := c.X + 0.25
		if u := l.idx.unit(i); u < l.geom.n {
			ln := &l.geom.lines[l.geom.lineAt(u)]
			if u < ln.content && l.geom.cl[l.geom.cluster(ln, u)].rtl {
				x = c.X - 0.25 // the leading half of a right-to-left cluster is left of its caret
			}
		}
		got, trailing := l.HitTest(render.Point{X: x, Y: c.Y + c.H/2})
		if got != i || trailing {
			t.Errorf("%q: HitTest on CaretRect(%d) = %v gives %d %v", l.text, i, c, got, trailing)
		}
	}
}

// TestHitRoundTrip: the round trip on several texts, wrapped and not.
func TestHitRoundTrip(t *testing.T) {
	ts := newTestText(t)
	for _, c := range []struct {
		s string
		w float64
	}{
		{"Hello, World", 0}, {"ab\n\ncd\n", 0}, {"a\r\nb", 0}, {"xéy😀z", 0},
		{"hello world again and again", 60}, {"abcdefghij", 20}, {"日本語のテキスト", 40},
	} {
		t.Run(fmt.Sprintf("%q/%g", c.s, c.w), func(t *testing.T) { roundTrip(t, dwLayout(t, ts, c.s, c.w)) })
	}
}
