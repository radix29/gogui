package text

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// The specs of the tables mark the selection in the text: '|' is the head
// and '^' the anchor (absent: a caret).

func parseSpec(t testing.TB, spec string) (text string, anchor, head int) {
	t.Helper()
	h := strings.IndexByte(spec, '|')
	if h < 0 {
		t.Fatalf("spec %q has no head", spec)
	}
	a := strings.IndexByte(spec, '^')
	text = strings.NewReplacer("|", "", "^", "").Replace(spec)
	head, anchor = h, h
	if a >= 0 {
		anchor = a
		if a < h {
			head--
		} else {
			anchor--
		}
	}
	return text, anchor, head
}

func formatSpec(e *Editor) string {
	s := e.Text()
	a, h := e.Anchor(), e.Head()
	if a == h {
		return s[:h] + "|" + s[h:]
	}
	if a < h {
		return s[:a] + "^" + s[a:h] + "|" + s[h:]
	}
	return s[:h] + "|" + s[h:a] + "^" + s[a:]
}

func specEditor(t testing.TB, spec string, single bool) *Editor {
	t.Helper()
	text, a, h := parseSpec(t, spec)
	e := NewEditor(text, single)
	e.SetSelection(a, h)
	if e.Anchor() != a || e.Head() != h {
		t.Fatalf("spec %q: selection is not on grapheme boundaries", spec)
	}
	return e
}

// fakeLines wraps every paragraph after w grapheme clusters, anywhere; each
// cluster is 10 wide.
type fakeLines struct {
	e    *Editor
	w    int
	page int
}

type vline struct {
	bs   []int // the cluster boundaries from the line start to its end
	last bool  // the line ends its paragraph
}

func (f fakeLines) all() []vline {
	var ls []vline
	b := f.e.Buffer()
	for i := range b.ParagraphCount() {
		ps, pe := b.ParagraphStart(i), b.ParagraphEnd(i)
		bs := refGraphemes(b.Slice(ps, pe))
		for k := range bs {
			bs[k] += ps
		}
		for len(bs) > f.w+1 {
			ls = append(ls, vline{bs: bs[:f.w+1]})
			bs = bs[f.w:]
		}
		ls = append(ls, vline{bs: bs, last: true})
	}
	return ls
}

func (f fakeLines) find(off int, up bool) (int, []vline) {
	ls := f.all()
	for i, l := range ls {
		start, end := l.bs[0], l.bs[len(l.bs)-1]
		if start <= off && off < end || off == end && l.last {
			if up && off == start && i > 0 && !ls[i-1].last {
				return i - 1, ls
			}
			return i, ls
		}
	}
	panic(fmt.Sprintf("fakeLines: no line at %d", off))
}

func (f fakeLines) LineRange(off int, up bool) (int, int) {
	i, ls := f.find(off, up)
	return ls[i].bs[0], ls[i].bs[len(ls[i].bs)-1]
}

func (f fakeLines) CaretX(off int, up bool) float64 {
	i, ls := f.find(off, up)
	k := slices.Index(ls[i].bs, off)
	if k < 0 {
		panic(fmt.Sprintf("fakeLines: %d is not a boundary", off))
	}
	return float64(10 * k)
}

func (f fakeLines) OffsetAt(off int, up bool, n int, x float64) (int, bool, bool) {
	i, ls := f.find(off, up)
	j := min(max(i+n, 0), len(ls)-1)
	if j == i {
		return off, false, false
	}
	l := ls[j]
	k := min(max(int(math.Round(x/10)), 0), len(l.bs)-1)
	return l.bs[k], k == len(l.bs)-1 && !l.last, true
}

func (f fakeLines) PageLines() int { return f.page }

type editorCase struct {
	name   string
	single bool
	spec   string
	op     func(e *Editor)
	want   string
}

func runEditorCases(t *testing.T, tests []editorCase) {
	t.Helper()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, span := range []int{0, 1, 3} {
				e := specEditor(t, tc.spec, tc.single)
				e.spanMin = span
				tc.op(e)
				if got := formatSpec(e); got != tc.want {
					t.Errorf("span %d: %q → %q, want %q", span, tc.spec, got, tc.want)
				}
				checkEditor(t, e)
			}
		})
	}
}

func mv(u Unit, d Direction, extend bool) func(*Editor) {
	return func(e *Editor) { e.Move(u, d, extend, nil) }
}

func repeat(n int, op func(*Editor)) func(*Editor) {
	return func(e *Editor) {
		for range n {
			op(e)
		}
	}
}

func TestEditorMoves(t *testing.T) {
	fwd, back := Forward, Backward
	runEditorCases(t, []editorCase{
		{"grapheme", false, "ab|c", mv(Grapheme, fwd, false), "abc|"},
		{"grapheme at end", false, "abc|", mv(Grapheme, fwd, false), "abc|"},
		{"grapheme at start", false, "|abc", mv(Grapheme, back, false), "|abc"},
		{"combining", false, "a|" + combAcute + "b", mv(Grapheme, fwd, false), "a" + combAcute + "|b"},
		{"combining back", false, "a" + combAcute + "|b", mv(Grapheme, back, false), "a|" + combAcute + "b"},
		{"family", false, "|" + family + "x", mv(Grapheme, fwd, false), family + "|x"},
		{"flags", false, flagDE + flagFR + "|", mv(Grapheme, back, false), flagDE + "|" + flagFR},
		{"newline", false, "ab|\ncd", mv(Grapheme, fwd, false), "ab\n|cd"},
		{"newline back", false, "ab\n|cd", mv(Grapheme, back, false), "ab|\ncd"},
		{"empty paragraphs", false, "a\n|\nb", mv(Grapheme, back, false), "a|\n\nb"},
		{"collapse back", false, "a^bc|d", mv(Grapheme, back, false), "a|bcd"},
		{"collapse fwd", false, "a|bc^d", mv(Grapheme, fwd, false), "abc|d"},
		{"extend", false, "a^b|c", mv(Grapheme, fwd, true), "a^bc|"},
		{"extend back over anchor", false, "ab^c|", repeat(2, mv(Grapheme, back, true)), "a|b^c"},
		{"word fwd", false, "foo| bar baz", mv(Word, fwd, false), "foo bar| baz"},
		{"word fwd inside", false, "f|oo bar", mv(Word, fwd, false), "foo| bar"},
		{"word fwd punctuation", false, "foo|, bar", mv(Word, fwd, false), "foo, bar|"},
		{"word fwd apostrophe", false, "|don't stop", mv(Word, fwd, false), "don't| stop"},
		{"word fwd across paragraphs", false, "foo|\n\n  bar x", mv(Word, fwd, false), "foo\n\n  bar| x"},
		{"word fwd none", false, "foo|  .\n\n", mv(Word, fwd, false), "foo  .\n\n|"},
		{"word back", false, "foo bar |baz", mv(Word, back, false), "foo |bar baz"},
		{"word back inside", false, "foo ba|r", mv(Word, back, false), "foo |bar"},
		{"word back across paragraphs", false, "x foo, \n\n|bar", mv(Word, back, false), "x |foo, \n\nbar"},
		{"word back none", false, "  .\n|x", mv(Word, back, false), "|  .\nx"},
		{"word from selection end", false, "foo ^bar| baz", mv(Word, fwd, false), "foo bar baz|"},
		{"word from selection start", false, "foo ^bar| baz", mv(Word, back, false), "|foo bar baz"},
		{"word extend", false, "foo ^bar| baz", mv(Word, fwd, true), "foo ^bar baz|"},
		{"line start", false, "ab\nc|d\nef", mv(Line, back, false), "ab\n|cd\nef"},
		{"line end", false, "ab\nc|d\nef", mv(Line, fwd, false), "ab\ncd|\nef"},
		{"line extend", false, "ab\nc^d|\nef", mv(Line, back, true), "ab\n|c^d\nef"},
		{"paragraph back", false, "ab\nc|d\nef", mv(Paragraph, back, false), "ab\n|cd\nef"},
		{"paragraph back again", false, "ab\n|cd\nef", mv(Paragraph, back, false), "|ab\ncd\nef"},
		{"paragraph back at 0", false, "|ab", mv(Paragraph, back, false), "|ab"},
		{"paragraph fwd", false, "a|b\ncd", mv(Paragraph, fwd, false), "ab|\ncd"},
		{"paragraph fwd again", false, "ab|\ncd", mv(Paragraph, fwd, false), "ab\ncd|"},
		{"paragraph fwd at end", false, "ab|", mv(Paragraph, fwd, false), "ab|"},
		{"paragraph empty", false, "a\n|\n\nb", mv(Paragraph, fwd, false), "a\n\n|\nb"},
		{"document start", false, "ab\nc|d", mv(Document, back, false), "|ab\ncd"},
		{"document end", false, "a|b\ncd", mv(Document, fwd, false), "ab\ncd|"},
		{"document extend", false, "a^b|\ncd", mv(Document, fwd, true), "a^b\ncd|"},
		{"document collapses", false, "a^b|\ncd", mv(Document, back, false), "|ab\ncd"},
		{"up", false, "abc\nd|e\nfghi", mv(Vertical, back, false), "a|bc\nde\nfghi"},
		{"down", false, "abc\nd|e\nfghi", mv(Vertical, fwd, false), "abc\nde\nf|ghi"},
		{"up on first line", false, "a|bc\nd", mv(Vertical, back, false), "|abc\nd"},
		{"down on last line", false, "abc\nd|e", mv(Vertical, fwd, false), "abc\nde|"},
		{"goal x", false, "abc|d\ne\nfghi", repeat(2, mv(Vertical, fwd, false)), "abcd\ne\nfgh|i"},
		{"goal x back", false, "abc|d\ne\nfghi", func(e *Editor) {
			for _, d := range []Direction{fwd, fwd, fwd, back} {
				e.Move(Vertical, d, false, nil)
			}
		}, "abcd\ne|\nfghi"},
		{"goal x dropped", false, "abc|d\ne\nfghi", func(e *Editor) {
			e.Move(Vertical, fwd, false, nil)
			e.Move(Grapheme, back, false, nil)
			e.Move(Vertical, fwd, false, nil)
		}, "abcd\ne\n|fghi"},
		{"down from selection end", false, "a^b\nc|d\nef", mv(Vertical, fwd, false), "ab\ncd\ne|f"},
		{"up from selection start", false, "ab\nc|d\ne^f", mv(Vertical, back, false), "a|b\ncd\nef"},
		{"vertical extend", false, "a|bc\ndef", mv(Vertical, fwd, true), "a^bc\nd|ef"},
		{"vertical combining", false, "a" + combAcute + "|b\nxyz", mv(Vertical, fwd, false), "a" + combAcute + "b\nxy|z"},
		{"page", false, "a|b\nc\nd", mv(Page, fwd, false), "ab\nc|\nd"},
		{"single up", true, "ab|c", mv(Vertical, back, false), "|abc"},
		{"single down", true, "a|bc", mv(Vertical, fwd, false), "abc|"},
		{"single page", true, "a|bc", mv(Page, back, true), "|a^bc"},
		{"single paragraph", true, "a|bc", mv(Paragraph, fwd, false), "abc|"},
		{"single line", true, "a b|c", mv(Line, back, false), "|a bc"},
	})
}

func TestEditorEdits(t *testing.T) {
	fwd, back := Forward, Backward
	ins := func(s string) func(*Editor) { return func(e *Editor) { e.Insert(s) } }
	del := func(u Unit, d Direction) func(*Editor) {
		return func(e *Editor) {
			if d == Backward {
				e.DeleteBackward(u, nil)
			} else {
				e.DeleteForward(u, nil)
			}
		}
	}
	runEditorCases(t, []editorCase{
		{"insert", false, "a|b", ins("xy"), "axy|b"},
		{"insert empty", false, "a|b", ins(""), "a|b"},
		{"insert replaces", false, "a^bc|d", ins("X"), "aX|d"},
		{"insert replaces backwards", false, "a|bc^d", ins("X"), "aX|d"},
		{"insert crlf", false, "a|b", ins("\r\n\r"), "a\n\n|b"},
		{"insert into empty", false, "|", ins("日本"), "日本|"},
		{"single insert newline", true, "a|b", ins("x\r\ny\nz\r"), "ax y z |b"},
		{"typing", false, "|", repeat(3, ins("a")), "aaa|"},
		{"insert mark", false, "e|", ins("́"), combAcute + "|"},
		{"insert base before mark", false, "|́", ins("e"), combAcute + "|"},
		{"insert regional indicator", false, "|\U0001F1EAx", ins("\U0001F1E9"), flagDE + "|x"},
		{"backspace", false, "ab|c", del(Grapheme, back), "a|c"},
		{"backspace combining", false, "a" + combAcute + "|", del(Grapheme, back), "a|"},
		{"backspace family", false, "x" + family + "|", del(Grapheme, back), "x|"},
		{"backspace newline", false, "a\n|b", del(Grapheme, back), "a|b"},
		{"backspace at start", false, "|ab", del(Grapheme, back), "|ab"},
		{"backspace selection", false, "a^bc|d", del(Grapheme, back), "a|d"},
		{"delete", false, "a|bc", del(Grapheme, fwd), "a|c"},
		{"delete at end", false, "ab|", del(Grapheme, fwd), "ab|"},
		{"delete selection", false, "a|bc^d", del(Word, fwd), "a|d"},
		{"delete cluster", false, "e|x́", del(Grapheme, fwd), "e|"},
		{"delete joins clusters", false, "e|\ń", del(Grapheme, fwd), "|" + combAcute},
		{"delete joins clusters back", false, "e\n|́", del(Grapheme, back), "|" + combAcute},
		{"option backspace", false, "foo bar|", del(Word, back), "foo |"},
		{"option backspace spaces", false, "foo bar,  |", del(Word, back), "foo |"},
		{"option backspace paragraph", false, "x foo\n|", del(Word, back), "x |"},
		{"option backspace at start", false, "|foo", del(Word, back), "|foo"},
		{"option delete", false, "|foo bar", del(Word, fwd), "| bar"},
		{"primary backspace", false, "ab\ncd|ef", del(Line, back), "ab\n|ef"},
		{"primary backspace at line start", false, "ab\n|cd", del(Line, back), "ab|cd"},
		{"primary backspace at 0", false, "|ab", del(Line, back), "|ab"},
		{"primary delete", false, "ab|cd\nef", del(Line, fwd), "ab|\nef"},
		{"primary delete at line end", false, "ab|\ncd", del(Line, fwd), "ab|cd"},
		{"delete paragraph", false, "ab\nc|d\nef", del(Paragraph, back), "ab\n|d\nef"},
		{"delete document", false, "ab\nc|d", del(Document, fwd), "ab\nc|"},
		{"delete down", false, "a|bc\ndef", del(Vertical, fwd), "a|ef"},
		{"cut", false, "a^bc|d", func(e *Editor) {
			if s := e.Cut(); s != "bc" {
				t.Errorf("Cut = %q", s)
			}
		}, "a|d"},
		{"cut caret", false, "a|b", func(e *Editor) {
			if s := e.Cut(); s != "" {
				t.Errorf("Cut = %q", s)
			}
		}, "a|b"},
		{"copy", false, "a|bc^d", func(e *Editor) {
			if s := e.Copy(); s != "bc" {
				t.Errorf("Copy = %q", s)
			}
		}, "a|bc^d"},
		{"paste", false, "a^b|c", func(e *Editor) { e.Paste("x\r\ny") }, "ax\ny|c"},
		{"single paste", true, "a^b|c", func(e *Editor) { e.Paste("x\r\ny") }, "ax y|c"},
	})
}

func TestEditorSelect(t *testing.T) {
	at := func(f func(*Editor, int), off int) func(*Editor) { return func(e *Editor) { f(e, off) } }
	word, para := (*Editor).SelectWord, (*Editor).SelectParagraph
	runEditorCases(t, []editorCase{
		{"word", false, "|foo bar", at(word, 5), "foo ^bar|"},
		{"word at its end", false, "|foo bar", at(word, 3), "foo^ |bar"},
		{"word end of text", false, "|foo bar", at(word, 7), "foo ^bar|"},
		{"spaces", false, "|a   b", at(word, 2), "a^   |b"},
		{"punctuation", false, "|a,, b", at(word, 1), "a^,|, b"},
		{"emoji", false, "|a" + family + "b", at(word, 3), "a^" + family + "|b"},
		{"empty paragraph", false, "|a\n\nb", at(word, 2), "a\n|\nb"},
		{"paragraph end", false, "|ab\ncd", at(word, 2), "^ab|\ncd"},
		{"word clamps", false, "|ab", at(word, 99), "^ab|"},
		{"paragraph", false, "|ab\ncd\nef", at(para, 4), "ab\n^cd\n|ef"},
		{"last paragraph", false, "|ab\ncd", at(para, 4), "ab\n^cd|"},
		{"empty last paragraph", false, "|ab\n", at(para, 3), "ab\n|"},
		{"all", false, "a|b\ncd", (*Editor).SelectAll, "^ab\ncd|"},
		{"set selection clamps", false, "|x" + family, func(e *Editor) { e.SetSelection(-5, 5) }, "^x|" + family},
	})
}

func TestEditorSetText(t *testing.T) {
	e := NewEditor("a\r\nb", true)
	if got := e.Text(); got != "a b" {
		t.Errorf("single-line text %q", got)
	}
	e.SetSelection(1, 3)
	e.SetText("xy\rz")
	if got := formatSpec(e); got != "|xy z" {
		t.Errorf("after SetText %q", got)
	}
	if !e.SingleLine() || e.Len() != 4 || e.Buffer().String() != "xy z" {
		t.Error("accessors")
	}
	m := NewEditor("a\r\nb", false)
	if m.Text() != "a\nb" || m.SingleLine() {
		t.Errorf("multi-line text %q", m.Text())
	}
}

// TestEditorWrapped moves over soft-wrapped lines of the fake layout.
func TestEditorWrapped(t *testing.T) {
	// Lines of 4 clusters: [abcd][efgh][ij] [xy]
	const text = "abcdefghij\nxy"
	type step struct {
		u    Unit
		d    Direction
		del  bool
		head int
		up   bool
	}
	tests := []struct {
		name  string
		start int
		steps []step
	}{
		{"line end is upstream", 1, []step{
			{Line, Forward, false, 4, true},
			{Line, Forward, false, 4, true}, // stays on the line
			{Line, Backward, false, 0, false},
		}},
		{"line start of the next line", 4, []step{
			{Line, Backward, false, 4, false},
			{Line, Forward, false, 8, true},
		}},
		{"down and up keep the goal", 1, []step{
			{Vertical, Forward, false, 5, false},
			{Vertical, Forward, false, 9, false},
			{Vertical, Forward, false, 12, false},
			{Vertical, Forward, false, 13, false}, // last line: the end
			{Vertical, Backward, false, 9, false},
		}},
		{"down from an upstream end", 1, []step{
			{Line, Forward, false, 4, true},
			{Vertical, Forward, false, 8, true},
			{Vertical, Forward, false, 10, false}, // [ij] is shorter
			{Vertical, Backward, false, 8, true},
		}},
		{"page", 1, []step{
			{Page, Forward, false, 9, false},
			{Page, Forward, false, 12, false}, // clamped to the last line
			{Page, Forward, false, 13, false}, // on the last line: the end
			{Page, Backward, false, 5, false},
			{Page, Backward, false, 1, false}, // clamped to the first line
			{Page, Backward, false, 0, false},
		}},
		{"primary backspace on a wrapped line", 6, []step{
			{Line, Backward, true, 4, false},
		}},
		{"primary delete on a wrapped line", 5, []step{
			{Line, Forward, true, 5, false},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := NewEditor(text, false)
			l := fakeLines{e, 4, 2}
			e.SetSelection(tc.start, tc.start)
			for i, s := range tc.steps {
				if s.del {
					if s.d == Backward {
						e.DeleteBackward(s.u, l)
					} else {
						e.DeleteForward(s.u, l)
					}
				} else {
					e.Move(s.u, s.d, false, l)
				}
				if e.Head() != s.head || e.Upstream() != s.up {
					t.Fatalf("step %d: head %d upstream %v, want %d %v (%q)", i, e.Head(), e.Upstream(), s.head, s.up, formatSpec(e))
				}
			}
		})
	}
	e := NewEditor(text, false)
	e.SetSelection(6, 6)
	e.DeleteBackward(Line, fakeLines{e, 4, 2})
	if e.Text() != "abcdghij\nxy" {
		t.Errorf("Primary-⌫ left %q", e.Text())
	}
}

// checkEditor checks that the selection is on grapheme boundaries of the
// whole text and that a single-line text has no newline.
func checkEditor(t testing.TB, e *Editor) {
	t.Helper()
	s := e.Text()
	bs := refGraphemes(s)
	for _, off := range []int{e.Anchor(), e.Head()} {
		if _, ok := slices.BinarySearch(bs, off); !ok {
			t.Fatalf("offset %d of %q is not a grapheme boundary", off, s)
		}
	}
	if e.SingleLine() && strings.ContainsRune(s, '\n') {
		t.Fatalf("single-line text %q has a newline", s)
	}
}

var units = []Unit{Grapheme, Word, Line, Paragraph, Document, Vertical, Page}

// editorOp applies the op coded by op and arg to e (and returns its name).
func editorOp(t testing.TB, e *Editor, lines Lines, op, arg byte) string {
	u, d, ext := units[int(arg)%len(units)], Direction(arg>>3&1), arg&0x80 != 0
	switch op % 11 {
	case 0, 1:
		e.Move(u, d, ext, lines)
		return fmt.Sprintf("Move(%d,%d,%v)", u, d, ext)
	case 2:
		e.DeleteBackward(u, lines)
		return fmt.Sprintf("DeleteBackward(%d)", u)
	case 3:
		e.DeleteForward(u, lines)
		return fmt.Sprintf("DeleteForward(%d)", u)
	case 4, 5:
		s := pieces[int(arg)%len(pieces)]
		start, end := e.Selection()
		before := e.Text()
		e.Insert(s)
		if want := before[:start] + e.clean(s) + before[end:]; e.Text() != want {
			t.Fatalf("Insert(%q) gave %q, want %q", s, e.Text(), want)
		}
		return fmt.Sprintf("Insert(%q)", s)
	case 6:
		e.SetSelection(int(arg)%(e.Len()+1), e.Head())
		return fmt.Sprintf("SetSelection(%d)", int(arg)%(e.Len()+1))
	case 7:
		e.SelectWord(int(arg) % (e.Len() + 1))
		return "SelectWord"
	case 8:
		e.SelectParagraph(int(arg) % (e.Len() + 1))
		return "SelectParagraph"
	case 9:
		e.Cut()
		return "Cut"
	default:
		e.SelectAll()
		return "SelectAll"
	}
}

// runEditorOps applies ops to an editor with the default span and one with
// tiny spans, which must agree, checking the invariants after each.
func runEditorOps(t testing.TB, text string, single bool, ops []byte) {
	e, small := NewEditor(text, single), NewEditor(text, single)
	small.spanMin = 1
	m := observe(t, e.Buffer())
	var log []string
	for i := 0; i+1 < len(ops); i += 2 {
		var lines, smallLines Lines
		if ops[i]&0x80 != 0 {
			lines, smallLines = fakeLines{e, 3, 2}, fakeLines{small, 3, 2}
		}
		log = append(log, editorOp(t, e, lines, ops[i], ops[i+1]))
		editorOp(t, small, smallLines, ops[i], ops[i+1])
		checkEditor(t, e)
		m.check()
		if a, b := formatSpec(e), formatSpec(small); a != b || e.Upstream() != small.Upstream() {
			t.Fatalf("after %v: %q (up %v), tiny spans %q (up %v)", log, a, e.Upstream(), b, small.Upstream())
		}
	}
}

func TestEditorRandom(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 9))
	for range 400 {
		text := randPieces(r, r.IntN(40))
		ops := make([]byte, 2*(1+r.IntN(40)))
		for i := range ops {
			ops[i] = byte(r.Uint32())
		}
		runEditorOps(t, text, r.IntN(4) == 0, ops)
	}
}

func FuzzEditor(f *testing.F) {
	f.Add("foo bar\nbaz", []byte{0, 1, 4, 3, 2, 1, 0x80, 0x05})
	f.Add(family+" don't\r\n3.14 日本", []byte{1, 0x88, 7, 2, 0x82, 9, 4, 22})
	f.Add("á́ "+flagDE+flagFR, []byte{0x83, 0x81, 5, 16, 0x86, 0x0e})
	f.Fuzz(func(t *testing.T, text string, ops []byte) {
		if len(text) > 256 {
			text = text[:256]
		}
		if len(ops) > 128 {
			ops = ops[:128]
		}
		single := len(ops) > 0 && ops[0]%5 == 0
		runEditorOps(t, text, single, ops)
	})
}

// TestAroundIsLocal checks that the boundaries of a long paragraph are read
// from a short stretch of it.
func TestAroundIsLocal(t *testing.T) {
	for _, unit := range []string{"Grüße, the café's naïve résumé. ", "日本語の文章を書きます。"} {
		e := NewEditor(paragraph1MB(unit), false)
		mid := e.clusterStart(e.Len() / 2)
		longest := 0
		e.around(mid, 0, e.Len(), func(s string, lo int) bool {
			longest = max(longest, len(s))
			return true
		})
		if longest > 2*defaultSpan {
			t.Errorf("%q: a span of %d bytes", unit, longest)
		}
		// The moves agree with a whole-paragraph read.
		whole := NewEditor(e.Text(), false)
		whole.spanMin = e.Len()
		for _, u := range []Unit{Grapheme, Word} {
			for _, d := range []Direction{Backward, Forward} {
				e.SetSelection(mid, mid)
				whole.SetSelection(mid, mid)
				e.Move(u, d, false, nil)
				whole.Move(u, d, false, nil)
				if e.Head() != whole.Head() {
					t.Errorf("%q: Move(%d, %d) to %d, whole paragraph %d", unit, u, d, e.Head(), whole.Head())
				}
			}
		}
		e.SelectWord(mid)
		whole.SelectWord(mid)
		if e.Anchor() != whole.Anchor() || e.Head() != whole.Head() {
			t.Errorf("%q: SelectWord differs", unit)
		}
	}
}

func TestEditorPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("an unknown unit did not panic")
		}
	}()
	NewEditor("ab", false).Move(Unit(99), Forward, false, nil)
}

func benchEditor(b *testing.B, text string) {
	e := NewEditor(text, false)
	mid := e.clusterStart(e.Len() / 2)
	for _, u := range []Unit{Grapheme, Word, Line, Paragraph, Vertical} {
		for _, d := range []Direction{Backward, Forward} {
			b.Run(fmt.Sprintf("Move%d/%d", u, d), func(b *testing.B) {
				for b.Loop() {
					e.SetSelection(mid, mid)
					e.Move(u, d, false, nil)
				}
			})
		}
	}
	b.Run("SelectWord", func(b *testing.B) {
		for b.Loop() {
			e.SelectWord(mid)
		}
	})
	b.Run("TypeAndBackspace", func(b *testing.B) {
		e.SetSelection(mid, mid)
		for b.Loop() {
			e.Insert("x")
			e.DeleteBackward(Grapheme, nil)
		}
	})
	b.Run("DeleteWordAndRetype", func(b *testing.B) {
		e.SetSelection(mid, mid)
		for b.Loop() {
			start := e.Head()
			e.DeleteBackward(Word, nil)
			e.Insert(e.Buffer().Slice(0, 0) + strings.Repeat("y", start-e.Head()))
		}
	})
}

// BenchmarkEditor1MB moves and edits in the middle of 1 MB of 80-column
// text.
func BenchmarkEditor1MB(b *testing.B) { benchEditor(b, megabyte()) }

// BenchmarkEditorLongParagraph1MB does the same in one 1 MB paragraph,
// where the spans keep the reads short (the vertical moves of the fallback
// Lines go straight to the document ends: there is no other line).
func BenchmarkEditorLongParagraph1MB(b *testing.B) {
	benchEditor(b, paragraph1MB("Grüße, the café's naïve résumé costs 3.14 € today. "))
}

// TestEditorSetSelectionUpstream: the head is upstream only when asked
// and where it was asked for; SetSelection clears it.
func TestEditorSetSelectionUpstream(t *testing.T) {
	e := NewEditor("ab"+family+"cd", false)
	for _, c := range []struct {
		anchor, head int
		up           bool
		wantHead     int
		wantUp       bool
	}{
		{1, 3, true, 2, false}, // inside the family: moved to its start, not upstream
		{1, 2, true, 2, true},
		{len("ab" + family), len("ab" + family), false, len("ab" + family), false},
		{0, 99, true, e.Len(), false}, // clamped
		{2, 2, true, 2, true},
	} {
		e.SetSelectionUpstream(c.anchor, c.head, c.up)
		if e.Head() != c.wantHead || e.Upstream() != c.wantUp {
			t.Errorf("SetSelectionUpstream(%d, %d, %v): head %d up %v", c.anchor, c.head, c.up, e.Head(), e.Upstream())
		}
	}
	e.SetSelection(2, 2)
	if e.Upstream() {
		t.Error("SetSelection kept upstream")
	}
}
