package aqua

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua/text"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/fake"
)

// Long paragraphs in chunks (review P1).

// byteCountingText is a text system that counts the bytes it lays out.
type byteCountingText struct {
	render.TextSystem
	bytes int
}

func (c *byteCountingText) NewLayout(s string, f render.Font, w float64) render.TextLayout {
	c.bytes += len(s)
	return c.TextSystem.NewLayout(s, f, w)
}

// longParagraph returns n bytes of words without a newline, with runs of
// spaces and an over-long word now and then.
func longParagraph(r *rand.Rand, n int) string {
	words := []string{"the", "quick", "brown", "fox", "jumps", "over", "a", "lazy", "dog", "é€", "supercalifragilisticexpialidocious-and-then-some"}
	var sb strings.Builder
	for sb.Len() < n {
		sb.WriteString(words[r.Intn(len(words))])
		sb.WriteString(strings.Repeat(" ", 1+r.Intn(2)/1+r.Intn(8)/7))
	}
	return sb.String()[:n]
}

// sameLayout fails unless got answers every TextLayout query as want does.
func sameLayout(t testing.TB, got, want render.TextLayout) {
	t.Helper()
	if got.Text() != want.Text() {
		t.Fatalf("text differs: %d bytes, want %d", len(got.Text()), len(want.Text()))
	}
	// A chunk's hidden last line can only widen a chunked layout.
	if got.Size().H != want.Size().H || got.Size().W < want.Size().W || got.LineCount() != want.LineCount() || got.Baseline() != want.Baseline() {
		t.Fatalf("size %v, %d lines, baseline %v; want %v, %d, %v", got.Size(), got.LineCount(), got.Baseline(), want.Size(), want.LineCount(), want.Baseline())
	}
	for k := range want.LineCount() {
		if g, w := got.Line(k), want.Line(k); g != w {
			t.Fatalf("line %d: %+v, want %+v", k, g, w)
		}
	}
	s := want.Text()
	for off := 0; off <= len(s); off += 1 + off%13 {
		for off < len(s) && s[off]&0xC0 == 0x80 {
			off++
		}
		if g, w := got.CaretRect(off), want.CaretRect(off); g != w {
			t.Fatalf("CaretRect(%d) = %v, want %v", off, g, w)
		}
		if g, w := got.CaretRectUpstream(off), want.CaretRectUpstream(off); g != w {
			t.Fatalf("CaretRectUpstream(%d) = %v, want %v", off, g, w)
		}
		end := min(len(s), off+1+off%300)
		for end < len(s) && s[end]&0xC0 == 0x80 {
			end++
		}
		if g, w := fmt.Sprint(got.SelectionRects(off, end)), fmt.Sprint(want.SelectionRects(off, end)); g != w {
			t.Fatalf("SelectionRects(%d, %d) = %s, want %s", off, end, g, w)
		}
	}
	h := want.Size().H
	for y := -5.0; y < h+20; y += 7 {
		for _, x := range []float64{-3, 0, 10.5, 70, 139, 300} {
			gi, gt := got.HitTest(render.Point{X: x, Y: y})
			wi, wt := want.HitTest(render.Point{X: x, Y: y})
			if gi != wi || gt != wt {
				t.Fatalf("HitTest(%v, %v) = %d %v, want %d %v", x, y, gi, gt, wi, wt)
			}
		}
	}
}

// TestParaChunksSameAsWhole: a long wrapped paragraph laid out in chunks
// answers exactly as one layout of it, and so does every relayout after an
// edit (with only the chunks around the edit laid out again).
func TestParaChunksSameAsWhole(t *testing.T) {
	ts := fake.NewTextSystem()
	f := render.Font{}
	r := rand.New(rand.NewSource(7))
	for _, runes := range []int{1, 7, 20, 61} {
		w := float64(runes) * fake.RuneWidth(f)
		s := longParagraph(r, 3*longParaMin)
		l := newParaLayout(ts, s, f, w)
		if _, ok := l.(*chunkedLayout); !ok {
			t.Fatalf("a %d-byte paragraph is not chunked", len(s))
		}
		sameLayout(t, l, ts.NewLayout(s, f, w))
		for k := range 60 {
			start := r.Intn(len(s) + 1)
			end := min(len(s), start+r.Intn(3)*r.Intn(40))
			ins := []string{"", "x", " ", "  ", "ab cd", "é", strings.Repeat("z", 90), "q "}[r.Intn(8)]
			for start < len(s) && s[start]&0xC0 == 0x80 {
				start++
			}
			end = max(end, start)
			for end < len(s) && s[end]&0xC0 == 0x80 {
				end++
			}
			s = s[:start] + ins + s[end:]
			l = relayoutPara(ts, l, s, f, w, start, end, start+len(ins))
			if k%10 == 0 || runes == 20 {
				sameLayout(t, l, ts.NewLayout(s, f, w))
			}
		}
		sameLayout(t, l, ts.NewLayout(s, f, w))
	}
}

// TestParaChunksShortOrUnwrapped: short paragraphs and unwrapped text are
// one layout, as before.
func TestParaChunksShortOrUnwrapped(t *testing.T) {
	ts := fake.NewTextSystem()
	s := longParagraph(rand.New(rand.NewSource(1)), 3*longParaMin)
	if _, ok := newParaLayout(ts, s, render.Font{}, 0).(*chunkedLayout); ok {
		t.Error("an unwrapped paragraph is chunked")
	}
	if _, ok := newParaLayout(ts, s[:longParaMin], render.Font{}, 100).(*chunkedLayout); ok {
		t.Error("a short paragraph is chunked")
	}
}

// TestParaLayoutsLongParagraphKeystroke (review P1): a keystroke in the
// middle of one long paragraph lays out only a few chunks again, not the
// paragraph, and the cached layout stays exact.
func TestParaLayoutsLongParagraphKeystroke(t *testing.T) {
	e := text.NewEditor(longParagraph(rand.New(rand.NewSource(3)), 256<<10), false)
	ts := &byteCountingText{TextSystem: fake.NewTextSystem()}
	f := render.Font{}
	c := newParaLayouts(ts, e.Buffer(), f, 60*fake.RuneWidth(f))
	c.SetViewport(600)
	c.Layout(0)
	mid := e.Len() / 2
	e.SetSelection(mid, mid)
	for k := range 20 {
		ts.bytes = 0
		if k%2 == 0 {
			e.Insert("x")
		} else {
			e.DeleteBackward(text.Grapheme, c)
		}
		c.Layout(0)
		if ts.bytes > 6*longParaChunk {
			t.Fatalf("keystroke %d laid out %d bytes of a %d-byte paragraph", k, ts.bytes, e.Len())
		}
		checkParaLayouts(t, c)
	}
	sameLayout(t, c.Layout(0), ts.TextSystem.NewLayout(e.Text(), f, c.Width()))
}

// BenchmarkParaLayoutsKeystrokeLongParagraph types one rune in the middle
// of a 1 MB paragraph (a minified file, a log line) and relays it out.
func BenchmarkParaLayoutsKeystrokeLongParagraph(b *testing.B) {
	e := text.NewEditor(longParagraph(rand.New(rand.NewSource(3)), 1<<20), false)
	c := newTestParaLayouts(e, 60)
	c.SetViewport(600)
	c.Layout(0)
	mid := e.Len() / 2
	e.SetSelection(mid, mid)
	b.ReportAllocs()
	for b.Loop() {
		e.Insert("x")
		c.Layout(0)
		e.DeleteBackward(text.Grapheme, c)
		c.Layout(0)
	}
}

// TestTextViewLongParagraph (review P1): a TextView over one long
// paragraph draws only the chunks in view; typing in the middle keeps the layout chunked and lays out only
// around the caret, and a composition there is spliced into the chunks.
func TestTextViewLongParagraph(t *testing.T) {
	v, _, sf, s := focusedView(t, nil)
	v.SetText(longParagraph(rand.New(rand.NewSource(5)), 64<<10))
	ed := v.Editor()
	mid := ed.Len() / 2
	v.SetSelection(mid, mid)
	dump := paint(t, s, sf)
	if n := strings.Count(dump, "text "); n == 0 || n > 3 {
		t.Errorf("%d text draws for the 5 lines in view:\n%s", n, dump)
	}
	for _, ln := range strings.Split(dump, "\n") {
		var x, y, w, h float64
		if _, err := fmt.Sscanf(ln, "text (%g,%g,%g,%g)", &x, &y, &w, &h); err == nil && h >= v.lines.Index().Height(0)/4 {
			t.Errorf("a draw %v high of a paragraph %v high, not a chunk", h, v.lines.Index().Height(0))
		}
	}
	at := v.Offset().Y
	sf.Type("x")
	if _, ok := v.lines.Layout(0).(*chunkedLayout); !ok || v.lines.Made() == 0 {
		t.Fatal("the long paragraph is not chunked after typing")
	}
	if got := v.Offset().Y; got != at {
		t.Errorf("typing in view scrolled from %v to %v", at, got)
	}
	sameLayout(t, v.lines.Layout(0), fake.NewTextSystem().NewLayout(ed.Text(), v.TextFont(), v.lines.Width()))
	sf.Preedit("か", 3)
	if l := v.lines.Shown(0); !strings.Contains(l.Text(), "か") {
		t.Error("the preedit is not in the shown layout")
	} else if _, ok := l.(*chunkedLayout); !ok {
		t.Error("the composed layout is not chunked")
	}
	paint(t, s, sf)
}
