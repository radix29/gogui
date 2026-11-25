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

// newTestParaLayouts is a cache over e's buffer with the fake text system,
// wrapped after runes runes (0: no wrapping).
func newTestParaLayouts(e *text.Editor, runes int) *paraLayouts {
	f := render.Font{}
	return newParaLayouts(fake.NewTextSystem(), e.Buffer(), f, float64(runes)*fake.RuneWidth(f))
}

// checkParaLayouts checks the cache against its buffer: the index has a
// paragraph per paragraph, every cached layout is of its paragraph's
// current text, its height is exact in the index, and the cache is within
// its bound.
func checkParaLayouts(t testing.TB, c *paraLayouts) {
	t.Helper()
	b := c.buf
	if c.idx.Len() != b.ParagraphCount() {
		t.Fatalf("index has %d paragraphs, the buffer %d", c.idx.Len(), b.ParagraphCount())
	}
	if len(c.m) > c.bound {
		t.Fatalf("%d layouts cached, bound %d", len(c.m), c.bound)
	}
	n := 0
	for e := c.lru.next; e != &c.lru; e = e.next {
		n++
		if c.m[e.para] != e {
			t.Fatalf("LRU entry of paragraph %d not in the map", e.para)
		}
		if want := b.Slice(b.ParagraphStart(e.para), b.ParagraphEnd(e.para)); e.l.Text() != want {
			t.Fatalf("layout of paragraph %d is %q, the paragraph %q", e.para, e.l.Text(), want)
		}
		if c.composes(e.para) {
			continue // its height is the composed layout's, checked below
		}
		if !c.idx.Exact(e.para) || c.idx.Height(e.para) != e.l.Size().H {
			t.Fatalf("paragraph %d: index height %v exact %v, layout %v", e.para, c.idx.Height(e.para), c.idx.Exact(e.para), e.l.Size().H)
		}
	}
	if n != len(c.m) {
		t.Fatalf("LRU has %d entries, the map %d", n, len(c.m))
	}
	if p := c.pre; p.l != nil {
		para := b.Slice(b.ParagraphStart(p.para), b.ParagraphEnd(p.para))
		if want := para[:p.at] + p.s + para[p.at:]; p.l.Text() != want {
			t.Fatalf("composed layout of paragraph %d is %q, want %q", p.para, p.l.Text(), want)
		}
		if !c.idx.Exact(p.para) || c.idx.Height(p.para) != p.l.Size().H {
			t.Fatalf("composed paragraph %d: index height %v exact %v, layout %v", p.para, c.idx.Height(p.para), c.idx.Exact(p.para), p.l.Size().H)
		}
	}
}

type paraStep struct {
	u    text.Unit
	d    text.Direction
	del  bool
	head int
	up   bool
}

func runParaSteps(t *testing.T, txt string, start int, steps []paraStep) {
	t.Helper()
	e := text.NewEditor(txt, false)
	c := newTestParaLayouts(e, 4)
	c.page = 2
	e.SetSelection(start, start)
	for i, s := range steps {
		if s.del {
			if s.d == text.Backward {
				e.DeleteBackward(s.u, c)
			} else {
				e.DeleteForward(s.u, c)
			}
		} else {
			e.Move(s.u, s.d, false, c)
		}
		if e.Head() != s.head || e.Upstream() != s.up {
			t.Fatalf("step %d: head %d upstream %v, want %d %v (text %q)", i, e.Head(), e.Upstream(), s.head, s.up, e.Text())
		}
		checkParaLayouts(t, c)
	}
}

// TestParaLayoutsWrapped is text.TestEditorWrapped through the cache over
// the fake text system: four runes a line, a page of two lines.
func TestParaLayoutsWrapped(t *testing.T) {
	const (
		L, V, P = text.Line, text.Vertical, text.Page
		B, F    = text.Backward, text.Forward
	)
	// Lines of 4 runes: [abcd][efgh][ij] [xy]
	const txt = "abcdefghij\nxy"
	tests := []struct {
		name  string
		start int
		steps []paraStep
	}{
		{"line end is upstream", 1, []paraStep{
			{L, F, false, 4, true},
			{L, F, false, 4, true},
			{L, B, false, 0, false},
		}},
		{"line start of the next line", 4, []paraStep{
			{L, B, false, 4, false},
			{L, F, false, 8, true},
		}},
		{"down and up keep the goal", 1, []paraStep{
			{V, F, false, 5, false},
			{V, F, false, 9, false},
			{V, F, false, 12, false},
			{V, F, false, 13, false},
			{V, B, false, 9, false},
		}},
		{"down from an upstream end", 1, []paraStep{
			{L, F, false, 4, true},
			{V, F, false, 8, true},
			{V, F, false, 10, false},
			{V, B, false, 8, true},
		}},
		{"page", 1, []paraStep{
			{P, F, false, 9, false},
			{P, F, false, 12, false},
			{P, F, false, 13, false},
			{P, B, false, 5, false},
			{P, B, false, 1, false},
			{P, B, false, 0, false},
		}},
		{"primary backspace on a wrapped line", 6, []paraStep{
			{L, B, true, 4, false},
		}},
		{"primary delete on a wrapped line", 5, []paraStep{
			{L, F, true, 5, false},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { runParaSteps(t, txt, tc.start, tc.steps) })
	}
}

// TestParaLayoutsAcross moves across paragraph boundaries, through an
// empty paragraph, and after edits that merge paragraphs.
func TestParaLayoutsAcross(t *testing.T) {
	const (
		G, L, V, P = text.Grapheme, text.Line, text.Vertical, text.Page
		B, F       = text.Backward, text.Forward
	)
	// 0 abcd 4 efgh 8 ij 10 \n 11 klmn 15 op 17 \n 18 (empty) \n 19 xy 21
	// Lines: [abcd][efgh][ij] [klmn][op] [] [xy]
	const txt = "abcdefghij\nklmnop\n\nxy"
	tests := []struct {
		name  string
		start int
		steps []paraStep
	}{
		{"down through every line", 2, []paraStep{
			{V, F, false, 6, false},
			{V, F, false, 10, false}, // [ij] is shorter: its end
			{V, F, false, 13, false},
			{V, F, false, 17, false},
			{V, F, false, 18, false}, // the empty paragraph
			{V, F, false, 21, false},
			{V, F, false, 21, false}, // the last line: the end
			{V, B, false, 18, false},
			{V, B, false, 17, false},
			{V, B, false, 13, false},
			{V, B, false, 10, false},
		}},
		{"up from an upstream end across paragraphs", 12, []paraStep{
			{L, F, false, 15, true},
			{V, B, false, 10, false},
			{V, B, false, 8, true}, // right of [efgh]: its upstream end
			{V, B, false, 4, true},
			{V, B, false, 0, false}, // the first line: the start
		}},
		{"page across paragraphs", 1, []paraStep{
			{P, F, false, 9, false},
			{P, F, false, 16, false},
			{P, F, false, 20, false},
			{P, F, false, 21, false},
			{P, B, false, 16, false},
			{P, B, false, 9, false},
			{P, B, false, 1, false},
			{P, B, false, 0, false},
		}},
		{"line moves in the empty paragraph", 18, []paraStep{
			{L, F, false, 18, false},
			{L, B, false, 18, false},
		}},
		{"backspace merges and the lines follow", 11, []paraStep{
			// abcdefghijklmnop\n\nxy: [abcd][efgh][ijkl][mnop] [] [xy]
			{G, B, true, 10, false},
			{V, F, false, 14, false},
			{V, F, false, 17, false},
			{V, F, false, 20, false},
			{V, B, false, 17, false},
			{V, B, false, 14, false},
			{V, B, false, 10, false},
		}},
		{"delete forward merges the empty paragraph", 17, []paraStep{
			// abcdefghij\nklmnop\nxy: ... [op] [xy]
			{G, F, true, 17, false},
			{V, F, false, 20, false},
			{V, B, false, 17, false},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { runParaSteps(t, txt, tc.start, tc.steps) })
	}
}

// layoutsOf is the cached layout of every paragraph.
func layoutsOf(c *paraLayouts) map[int]render.TextLayout {
	m := map[int]render.TextLayout{}
	for p, e := range c.m {
		m[p] = e.l
	}
	return m
}

// TestParaLayoutsEdits checks that an edit drops exactly the layouts of
// the paragraphs it touched and moves the others to their new numbers.
func TestParaLayoutsEdits(t *testing.T) {
	var sb strings.Builder
	for i := range 10 {
		if i > 0 {
			sb.WriteByte('\n')
		}
		fmt.Fprintf(&sb, "para %d", i)
	}
	e := text.NewEditor(sb.String(), false)
	c := newTestParaLayouts(e, 0)
	b := e.Buffer()
	all := func() {
		for i := range b.ParagraphCount() {
			c.Layout(i)
		}
	}
	// keep maps an old paragraph to its new one, -1 when it was touched.
	check := func(name string, before map[int]render.TextLayout, keep func(int) int) {
		t.Helper()
		checkParaLayouts(t, c)
		want := map[int]render.TextLayout{}
		for p, l := range before {
			if q := keep(p); q >= 0 {
				want[q] = l
			}
		}
		got := layoutsOf(c)
		if len(got) != len(want) {
			t.Fatalf("%s: %d layouts kept, want %d", name, len(got), len(want))
		}
		for q, l := range want {
			if got[q] != l {
				t.Fatalf("%s: paragraph %d lost its layout", name, q)
			}
		}
		for p := range b.ParagraphCount() {
			if _, ok := got[p]; ok != c.idx.Exact(p) {
				t.Fatalf("%s: paragraph %d cached %v, exact %v", name, p, ok, c.idx.Exact(p))
			}
		}
	}
	shift := func(lo, hi, d int) func(int) int { // drops [lo, hi), shifts the rest from hi by d
		return func(p int) int {
			switch {
			case p >= hi:
				return p + d
			case p >= lo:
				return -1
			}
			return p
		}
	}
	edit := func(name string, keep func(int) int, f func()) {
		t.Helper()
		all()
		before := layoutsOf(c)
		f()
		check(name, before, keep)
	}
	ps := b.ParagraphStart

	edit("type in one paragraph", shift(3, 4, 0), func() {
		e.SetSelection(ps(3)+2, ps(3)+2)
		e.Insert("x")
	})
	edit("insert two newlines", shift(5, 6, 2), func() {
		e.SetSelection(ps(5)+1, ps(5)+1)
		e.Insert("a\nb\nc")
	})
	edit("delete across paragraphs", shift(2, 5, -2), func() {
		e.SetSelection(ps(2)+1, ps(4)+3)
		e.Insert("")
	})
	edit("join at a newline", shift(0, 2, -1), func() {
		e.SetSelection(b.ParagraphEnd(0), b.ParagraphEnd(0))
		e.DeleteForward(text.Grapheme, c)
	})
	edit("undo the join", shift(0, 1, 1), func() { e.Undo() })
	edit("redo the join", shift(0, 2, -1), func() { e.Redo() })
	edit("newline at the end", shift(b.ParagraphCount()-1, b.ParagraphCount(), 1), func() {
		e.SetSelection(b.Len(), b.Len())
		e.Insert("\n")
	})
	edit("same width", func(p int) int { return p }, func() { c.SetWidth(0) })
	edit("set text", shift(0, 100, 0), func() { e.SetText("one\ntwo") })
	edit("new width", shift(0, 100, 0), func() { c.SetWidth(100) })
	edit("new font", shift(0, 100, 0), func() { c.SetFont(render.Font{Size: 20}) })
	if c.idx.Inexact() != c.idx.Len() {
		t.Errorf("after a font change %d of %d heights are estimates", c.idx.Inexact(), c.idx.Len())
	}
}

// TestParaLayoutsEstimates checks the estimates and that a layout makes
// its paragraph's height exact.
func TestParaLayoutsEstimates(t *testing.T) {
	// Runes without spaces wrap between runes on the fake, so the estimate
	// of an ASCII paragraph is its height.
	e := text.NewEditor("abcdefghij\n\nxyz\n"+strings.Repeat("q", 41), false)
	c := newTestParaLayouts(e, 4)
	lh := c.LineHeight()
	want := []float64{3 * lh, lh, lh, 11 * lh}
	if c.idx.Inexact() != len(want) {
		t.Fatalf("%d estimates, want %d", c.idx.Inexact(), len(want))
	}
	for i, h := range want {
		if got := c.idx.Height(i); got != h {
			t.Errorf("estimate %d = %v, want %v", i, got, h)
		}
		c.Layout(i)
		if got := c.idx.Height(i); !c.idx.Exact(i) || got != h {
			t.Errorf("height %d = %v (exact %v), want %v", i, got, c.idx.Exact(i), h)
		}
	}
	if c.idx.Total() != 16*lh {
		t.Errorf("Total = %v, want %v", c.idx.Total(), 16*lh)
	}
	if c.Made() != len(want) || c.Layout(3) == nil || c.Made() != len(want) {
		t.Errorf("made %d layouts, want %d (a cached one is not remade)", c.Made(), len(want))
	}
	c.SetViewport(10 * lh)
	if c.PageLines() != 10 || c.Bound() != paraLayoutsMinBound {
		t.Errorf("viewport of 10 lines: page %d, bound %d", c.PageLines(), c.Bound())
	}
	c.SetViewport(50 * lh)
	if c.PageLines() != 50 || c.Bound() != 153 {
		t.Errorf("viewport of 50 lines: page %d, bound %d", c.PageLines(), c.Bound())
	}
	c.SetBound(2)
	if c.Cached() != 2 || c.cached(2) == nil || c.cached(3) == nil {
		t.Errorf("bound 2 kept %d layouts, not the two most recent", c.Cached())
	}
}

// refLines answers text.Lines from one fake layout of the whole text, the
// reference the cache's per-paragraph answers must agree with.
type refLines struct {
	l render.TextLayout
}

func (r refLines) LineRange(off int, up bool) (int, int) {
	m := r.l.Line(lineAt(r.l, off, up))
	return m.Start, m.End
}

func (r refLines) CaretX(off int, up bool) float64 {
	if up {
		return r.l.CaretRectUpstream(off).X
	}
	return r.l.CaretRect(off).X
}

func (r refLines) OffsetAt(off int, up bool, n int, x float64) (int, bool, bool) {
	k := lineAt(r.l, off, up)
	j := min(max(k+n, 0), r.l.LineCount()-1)
	if j == k {
		return off, false, false
	}
	m := r.l.Line(j)
	hit, trailing := r.l.HitTest(render.Point{X: x, Y: m.Top + m.Height/2})
	if trailing {
		hit = text.NextGrapheme(r.l.Text(), hit)
	}
	hit = min(max(hit, m.Start), m.End)
	last := j == r.l.LineCount()-1 || r.l.Line(j+1).Start != m.End
	return hit, hit == m.End && !last, true
}

// TestParaLayoutsRandom edits a text at random through an Editor with the
// cache as its Lines and a small bound, and checks the cache after every
// op and its Lines against a layout of the whole text.
func TestParaLayoutsRandom(t *testing.T) {
	pieces := []string{"a", "bc", " ", "word ", "\n", "x\ny", "\n\n", "long piece of text "}
	f := render.Font{}
	ts := fake.NewTextSystem()
	for seed := int64(1); seed <= 20; seed++ {
		r := rand.New(rand.NewSource(seed))
		e := text.NewEditor("start text\nsecond", false)
		c := newTestParaLayouts(e, 5+r.Intn(6))
		c.SetBound(1 + r.Intn(4))
		c.page = 1 + r.Intn(3)
		for op := range 300 {
			n := e.Len() + 1
			switch r.Intn(10) {
			case 0, 1, 2:
				e.Insert(pieces[r.Intn(len(pieces))])
			case 3:
				e.SetSelection(r.Intn(n), r.Intn(n))
			case 4:
				e.DeleteBackward(text.Unit(r.Intn(7)), c)
			case 5:
				e.DeleteForward(text.Unit(r.Intn(7)), c)
			case 6:
				e.Undo()
			case 7:
				e.Redo()
			default:
				e.Move([]text.Unit{text.Line, text.Vertical, text.Page}[r.Intn(3)], text.Direction(r.Intn(2)), r.Intn(2) == 0, c)
			}
			checkParaLayouts(t, c)
			ref := refLines{ts.NewLayout(e.Text(), f, c.Width())}
			for range 30 {
				off := r.Intn(e.Len() + 1)
				for _, up := range []bool{false, true} {
					s0, e0 := ref.LineRange(off, up)
					s1, e1 := c.LineRange(off, up)
					x0, x1 := ref.CaretX(off, up), c.CaretX(off, up)
					if s0 != s1 || e0 != e1 || x0 != x1 {
						t.Fatalf("seed %d op %d off %d up %v: LineRange %d,%d CaretX %v, want %d,%d %v (%q)",
							seed, op, off, up, s1, e1, x1, s0, e0, x0, e.Text())
					}
					for _, dn := range []int{-3, -1, 1, 2} {
						x := float64(r.Intn(60))
						t0, u0, ok0 := ref.OffsetAt(off, up, dn, x)
						t1, u1, ok1 := c.OffsetAt(off, up, dn, x)
						if t0 != t1 || u0 != u1 || ok0 != ok1 {
							t.Fatalf("seed %d op %d: OffsetAt(%d, %v, %d, %v) = %d %v %v, want %d %v %v (%q)",
								seed, op, off, up, dn, x, t1, u1, ok1, t0, u0, ok0, e.Text())
						}
					}
				}
			}
		}
	}
}

// megaText is about 1 MB of paragraphs of 0 to 80 words.
func megaText(seed int64) string {
	r := rand.New(rand.NewSource(seed))
	words := []string{"the", "quick", "brown", "fox", "jumps", "over", "a", "lazy", "dog", "lorem", "ipsum"}
	var sb strings.Builder
	for sb.Len() < 1<<20 {
		for n := r.Intn(81); n > 0; n-- {
			sb.WriteString(words[r.Intn(len(words))])
			if n > 1 {
				sb.WriteByte(' ')
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// TestParaLayoutsMegaWalk walks a 1 MB text by pages and by lines, both
// ways, and checks that the cache never holds more than its bound.
func TestParaLayoutsMegaWalk(t *testing.T) {
	e := text.NewEditor(megaText(1), false)
	c := newTestParaLayouts(e, 60)
	c.SetViewport(600)
	bound := c.Bound()
	most := 0
	walk := func(u text.Unit, d text.Direction, end int) int {
		steps := 0
		for prev := -1; e.Head() != prev; steps++ {
			prev = e.Head()
			e.Move(u, d, false, c)
			if c.Cached() > bound {
				t.Fatalf("%d layouts cached at %d, bound %d", c.Cached(), e.Head(), bound)
			}
			most = max(most, c.Cached())
		}
		if e.Head() != end {
			t.Fatalf("walk stopped at %d, want %d", e.Head(), end)
		}
		return steps
	}
	pages := walk(text.Page, text.Forward, e.Len())
	walk(text.Page, text.Backward, 0)
	lines := walk(text.Vertical, text.Forward, e.Len())
	walk(text.Vertical, text.Backward, 0)
	checkParaLayouts(t, c)
	if most != bound {
		t.Errorf("at most %d layouts cached, bound %d: the bound was never reached", most, bound)
	}
	if c.idx.Inexact() != 0 {
		t.Errorf("%d paragraphs never laid out after walking every line", c.idx.Inexact())
	}
	t.Logf("%d bytes, %d paragraphs, %d pages, %d lines, bound %d, %d layouts made",
		e.Len(), c.idx.Len(), pages, lines, bound, c.Made())
}

// BenchmarkParaLayoutsKeystroke types one rune in the middle of a 1 MB
// text with the visible paragraphs cached: the splice and the relayout of
// the touched paragraph.
func BenchmarkParaLayoutsKeystroke(b *testing.B) {
	e := text.NewEditor(megaText(1), false)
	c := newTestParaLayouts(e, 60)
	c.SetViewport(600)
	mid := e.Buffer().ParagraphIndex(e.Len() / 2)
	for i := mid; i < mid+40; i++ {
		c.Layout(i)
	}
	off := e.Buffer().ParagraphStart(mid) + 1
	e.SetSelection(off, off)
	b.ReportAllocs()
	for b.Loop() {
		e.Insert("x")
		c.Layout(mid)
		e.DeleteBackward(text.Grapheme, c)
		c.Layout(mid)
	}
}

// BenchmarkParaLayoutsPageDown pages down a 1 MB text from the top.
func BenchmarkParaLayoutsPageDown(b *testing.B) {
	e := text.NewEditor(megaText(1), false)
	c := newTestParaLayouts(e, 60)
	c.SetViewport(600)
	b.ReportAllocs()
	for b.Loop() {
		if e.Head() == e.Len() {
			e.SetSelection(0, 0)
		}
		e.Move(text.Page, text.Forward, false, c)
	}
}

// TestParaLayoutsMeasure: Measure makes a height exact, the same as its
// layout's, without caching a layout or touching the LRU order; an empty
// paragraph is one line high without a layout, and an exact height stays.
func TestParaLayoutsMeasure(t *testing.T) {
	e := text.NewEditor("the quick brown fox jumps over\n\nlazy dog\nend", false)
	c := newTestParaLayouts(e, 10)
	c.Layout(3)
	made := c.Made()
	for i := range 3 {
		laid := c.Measure(i)
		if laid != (i != 1) {
			t.Errorf("paragraph %d: Measure made a layout %v", i, laid)
		}
	}
	if c.Measure(3) || c.Made() != made+2 || c.Cached() != 1 || c.cached(3) == nil {
		t.Errorf("made %d, cached %d", c.Made()-made, c.Cached())
	}
	if c.idx.Inexact() != 0 {
		t.Fatalf("%d estimates left", c.idx.Inexact())
	}
	want := c.idx.Total()
	c.dropAll()
	for i := range 4 {
		c.Layout(i)
	}
	if c.idx.Total() != want || c.idx.Height(1) != c.LineHeight() || c.idx.Height(0) != 3*c.LineHeight() {
		t.Errorf("measured total %v, laid out %v (heights %v, %v)", want, c.idx.Total(), c.idx.Height(0), c.idx.Height(1))
	}
	checkParaLayouts(t, c)
}

// TestParaLayoutsPreedit: a composition is spliced into its paragraph's
// layout only, with that layout's height in the index, mapped both ways
// by ShownOffset and TextOffset; it is rebuilt on a width change, follows
// its paragraph when an edit before it moves it, goes with an edit of its
// paragraph, and ClearPreedit gives the paragraph its own height back.
func TestParaLayoutsPreedit(t *testing.T) {
	e := text.NewEditor("ab\ncd\nef", false)
	c := newTestParaLayouts(e, 4)
	lh := c.LineHeight()
	for i := range 3 {
		c.Layout(i)
	}
	c.SetPreedit(0, 2, "xxxxxx") // over a cached layout: two lines, then one again
	if c.idx.Height(0) != 2*lh {
		t.Errorf("composed over a cached layout: height %v", c.idx.Height(0))
	}
	c.ClearPreedit()
	if c.idx.Height(0) != lh {
		t.Errorf("cleared over a cached layout: height %v", c.idx.Height(0))
	}
	checkParaLayouts(t, c)
	c.SetPreedit(1, 1, "xyz")
	checkParaLayouts(t, c)
	if got := c.Shown(1).Text(); got != "cxyzd" || c.Layout(1).Text() != "cd" || c.Shown(0) != c.Layout(0) {
		t.Fatalf("shown %q, layout %q", got, c.Layout(1).Text())
	}
	if c.idx.Height(1) != 2*lh || c.idx.Y(2) != 3*lh { // "cxyz" and "d"
		t.Errorf("composed height %v, next at %v", c.idx.Height(1), c.idx.Y(2))
	}
	for _, m := range []struct{ i, text, shown int }{{1, 0, 0}, {1, 1, 4}, {1, 2, 5}, {0, 1, 1}, {2, 2, 2}} {
		if got := c.ShownOffset(m.i, m.text); got != m.shown {
			t.Errorf("ShownOffset(%d, %d) = %d, want %d", m.i, m.text, got, m.shown)
		}
		if got := c.TextOffset(m.i, m.shown); got != m.text {
			t.Errorf("TextOffset(%d, %d) = %d, want %d", m.i, m.shown, got, m.text)
		}
	}
	if got := c.TextOffset(1, 3); got != 1 { // inside the preedit: its place
		t.Errorf("TextOffset inside the preedit %d", got)
	}
	if c.Measure(1) {
		t.Error("Measure laid out the composed paragraph")
	}
	c.SetWidth(8 * fake.RuneWidth(render.Font{}))
	checkParaLayouts(t, c)
	if c.Shown(1).Text() != "cxyzd" || c.idx.Height(1) != lh {
		t.Errorf("after a width change: %q, height %v", c.Shown(1).Text(), c.idx.Height(1))
	}
	c.SetWidth(4 * fake.RuneWidth(render.Font{}))

	e.SetSelection(0, 0)
	e.Insert("\n") // an edit before it: the composition moves along
	if p, at, s, ok := c.Preedit(); p != 2 || at != 1 || s != "xyz" || !ok {
		t.Fatalf("after an edit before it: %d %d %q %v", p, at, s, ok)
	}
	checkParaLayouts(t, c)
	if c.Shown(2).Text() != "cxyzd" {
		t.Errorf("moved composition shows %q", c.Shown(2).Text())
	}
	c.ClearPreedit()
	checkParaLayouts(t, c)
	if c.Shown(2) != c.Layout(2) || c.idx.Height(2) != lh {
		t.Errorf("cleared: %q, height %v", c.Shown(2).Text(), c.idx.Height(2))
	}

	c.SetPreedit(2, 0, "q")
	e.SetSelection(e.Buffer().ParagraphStart(2), e.Buffer().ParagraphStart(2))
	e.Insert("z") // an edit of its paragraph: gone
	if _, _, _, ok := c.Preedit(); ok {
		t.Error("the composition survived an edit of its paragraph")
	}
	checkParaLayouts(t, c)
	c.SetPreedit(3, 1, "w")
	c.SetPreedit(3, 1, "")
	if _, _, _, ok := c.Preedit(); ok || c.Shown(3).Text() != "ef" {
		t.Errorf("an empty preedit left %q", c.Shown(3).Text())
	}
	checkParaLayouts(t, c)
}
