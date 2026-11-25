package text

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
)

// check verifies the buffer against want and its internal invariants.
func check(t *testing.T, b *Buffer, want string) {
	t.Helper()
	if got := b.String(); got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if b.Len() != len(want) {
		t.Fatalf("Len() = %d, want %d", b.Len(), len(want))
	}
	b.index()
	off, nl := 0, 0
	for i, p := range b.pieces {
		if p.n <= 0 {
			t.Fatalf("piece %d is empty: %+v", i, p)
		}
		var s string
		if p.add {
			s = string(b.add[p.start : p.start+p.n])
		} else {
			s = b.orig[p.start : p.start+p.n]
		}
		if c := strings.Count(s, "\n"); c != p.nl {
			t.Fatalf("piece %d: nl = %d, want %d", i, p.nl, c)
		}
		if p.off != off || p.nlOff != nl {
			t.Fatalf("piece %d: off, nlOff = %d, %d, want %d, %d", i, p.off, p.nlOff, off, nl)
		}
		off += p.n
		nl += p.nl
	}
	if strings.ContainsRune(want, '\r') {
		t.Fatalf("model has a \\r: %q", want)
	}
	paras := strings.Split(want, "\n")
	if b.ParagraphCount() != len(paras) {
		t.Fatalf("ParagraphCount() = %d, want %d", b.ParagraphCount(), len(paras))
	}
	start := 0
	for i, para := range paras {
		end := start + len(para)
		if s, e := b.ParagraphStart(i), b.ParagraphEnd(i); s != start || e != end {
			t.Fatalf("paragraph %d = [%d, %d), want [%d, %d)", i, s, e, start, end)
		}
		for _, o := range []int{start, (start + end) / 2, end} {
			if got := b.ParagraphIndex(o); got != i {
				t.Fatalf("ParagraphIndex(%d) = %d, want %d", o, got, i)
			}
			if s, e := b.ParagraphAt(o); s != start || e != end {
				t.Fatalf("ParagraphAt(%d) = [%d, %d), want [%d, %d)", o, s, e, start, end)
			}
		}
		start = end + 1
	}
}

func TestBufferZero(t *testing.T) {
	var b Buffer
	check(t, &b, "")
	if b.Revision() != 0 {
		t.Fatalf("Revision() = %d, want 0", b.Revision())
	}
	if got := b.Slice(0, 0); got != "" {
		t.Fatalf("Slice(0, 0) = %q", got)
	}
	b.Insert(0, "a")
	check(t, &b, "a")
}

func TestBufferEdits(t *testing.T) {
	type op struct {
		ins        bool
		off, end   int
		text, want string
	}
	tests := []struct {
		name string
		init string
		ops  []op
	}{
		{"insert empty", "", []op{{ins: true, off: 0, text: "hello", want: "hello"}}},
		{"insert front", "world", []op{{ins: true, off: 0, text: "hello ", want: "hello world"}}},
		{"insert back", "hello", []op{{ins: true, off: 5, text: " world", want: "hello world"}}},
		{"insert middle", "held", []op{{ins: true, off: 2, text: "llo wor", want: "hello world"}}},
		{"typing", "ac", []op{
			{ins: true, off: 1, text: "b", want: "abc"},
			{ins: true, off: 2, text: "b", want: "abbc"},
			{ins: true, off: 3, text: "\n", want: "abb\nc"},
		}},
		{"delete all", "hello", []op{{off: 0, end: 5, want: ""}}},
		{"delete front", "hello", []op{{off: 0, end: 2, want: "llo"}}},
		{"delete back", "hello", []op{{off: 3, end: 5, want: "hel"}}},
		{"delete middle", "hello", []op{{off: 1, end: 4, want: "ho"}}},
		{"delete empty range", "hello", []op{{off: 2, end: 2, want: "hello"}}},
		{"delete across pieces", "aaaa", []op{
			{ins: true, off: 2, text: "bb", want: "aabbaa"},
			{ins: true, off: 0, text: "cc", want: "ccaabbaa"},
			{off: 1, end: 7, want: "ca"},
		}},
		{"delete whole pieces", "aaaa", []op{
			{ins: true, off: 2, text: "bb", want: "aabbaa"},
			{off: 2, end: 4, want: "aaaa"},
			{off: 0, end: 2, want: "aa"},
		}},
		{"newlines", "a\nb", []op{
			{ins: true, off: 1, text: "\n\n", want: "a\n\n\nb"},
			{off: 2, end: 4, want: "a\nb"},
			{ins: true, off: 3, text: "\n", want: "a\nb\n"},
		}},
		{"crlf", "", []op{
			{ins: true, off: 0, text: "a\r\nb\rc\r", want: "a\nb\nc\n"},
			{ins: true, off: 0, text: "\r\r\n", want: "\n\na\nb\nc\n"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer(tt.init)
			check(t, b, tt.init)
			for _, o := range tt.ops {
				rev := b.Revision()
				changed := false
				if o.ins {
					n := b.Insert(o.off, o.text)
					if want := len(normalizeNewlines(o.text)); n != want {
						t.Fatalf("Insert(%d, %q) = %d, want %d", o.off, o.text, n, want)
					}
					changed = n > 0
				} else {
					b.Delete(o.off, o.end)
					changed = o.end > o.off
				}
				check(t, b, o.want)
				if got := b.Revision() != rev; got != changed {
					t.Fatalf("revision advanced = %v, want %v", got, changed)
				}
			}
		})
	}
}

func TestBufferTypingExtendsPiece(t *testing.T) {
	b := NewBuffer("hello world")
	for i, c := range "abcdef" {
		b.Insert(5+i, string(c))
	}
	check(t, b, "helloabcdef world")
	if len(b.pieces) != 3 {
		t.Fatalf("%d pieces after typing at one spot, want 3", len(b.pieces))
	}
	// Typing at the end of the text extends as well.
	b.Insert(b.Len(), "x")
	b.Insert(b.Len(), "y")
	check(t, b, "helloabcdef worldxy")
	if len(b.pieces) != 4 {
		t.Fatalf("%d pieces, want 4", len(b.pieces))
	}
}

func TestBufferSetText(t *testing.T) {
	b := NewBuffer("one")
	b.Insert(3, " two")
	rev := b.Revision()
	b.SetText("x\r\ny")
	check(t, b, "x\ny")
	if b.Revision() == rev {
		t.Fatal("SetText did not advance the revision")
	}
	if len(b.add) != 0 || len(b.addNL) != 0 {
		t.Fatal("SetText kept the add buffer")
	}
	rev = b.Revision()
	b.SetText("x\ny")
	if b.Revision() == rev {
		t.Fatal("SetText of the same text did not advance the revision")
	}
	b.SetText("")
	check(t, b, "")
	if b.Insert(0, "") != 0 {
		t.Fatal("empty insert inserted")
	}
}

func TestBufferSlice(t *testing.T) {
	b := NewBuffer("0123456789")
	b.Insert(5, "abc")
	b.Delete(1, 2)
	want := "0234abc56789"
	for s := 0; s <= len(want); s++ {
		for e := s; e <= len(want); e++ {
			if got := b.Slice(s, e); got != want[s:e] {
				t.Fatalf("Slice(%d, %d) = %q, want %q", s, e, got, want[s:e])
			}
		}
	}
}

func TestBufferParagraphs(t *testing.T) {
	b := NewBuffer("\n")
	check(t, b, "\n")
	if s, e := b.ParagraphAt(0); s != 0 || e != 0 {
		t.Fatalf("ParagraphAt(0) = [%d, %d)", s, e)
	}
	if s, e := b.ParagraphAt(1); s != 1 || e != 1 {
		t.Fatalf("ParagraphAt(1) = [%d, %d)", s, e)
	}
	b.SetText("ab\ncd\n\nef")
	check(t, b, "ab\ncd\n\nef")
	if got := b.ParagraphIndex(2); got != 0 {
		t.Fatalf("ParagraphIndex at the first '\\n' = %d, want 0", got)
	}
	if got := b.ParagraphIndex(3); got != 1 {
		t.Fatalf("ParagraphIndex after the first '\\n' = %d, want 1", got)
	}
}

func TestBufferPanics(t *testing.T) {
	b := NewBuffer("abc")
	cases := map[string]func(){
		"insert -1":         func() { b.Insert(-1, "x") },
		"insert past end":   func() { b.Insert(4, "x") },
		"delete reversed":   func() { b.Delete(2, 1) },
		"delete past end":   func() { b.Delete(0, 4) },
		"slice negative":    func() { b.Slice(-1, 1) },
		"index past end":    func() { b.ParagraphIndex(4) },
		"start -1":          func() { b.ParagraphStart(-1) },
		"start past count":  func() { b.ParagraphStart(1) },
		"end past count":    func() { b.ParagraphEnd(1) },
		"at negative":       func() { b.ParagraphAt(-1) },
		"insert empty past": func() { b.Insert(4, "") },
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("no panic")
				}
			}()
			f()
		})
	}
	check(t, b, "abc")
}

// TestBufferRandom runs long random edit sequences against a string model.
func TestBufferRandom(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for round := range 20 {
		b := NewBuffer(randText(r, r.IntN(200)))
		model := normalizeNewlines(b.String())
		m := observe(t, b)
		for range 300 {
			model = randomOp(r, b, model)
			m.check()
		}
		t.Run(fmt.Sprint(round), func(t *testing.T) { check(t, b, model) })
	}
}

// randomOp applies one random insert, delete or replace to b and returns
// the model after it.
func randomOp(r *rand.Rand, b *Buffer, model string) string {
	if r.IntN(4) == 0 {
		s := r.IntN(len(model) + 1)
		e := s + r.IntN(min(len(model)-s, 20)+1)
		txt := randText(r, r.IntN(8))
		b.Replace(s, e, txt)
		return model[:s] + normalizeNewlines(txt) + model[e:]
	}
	if r.IntN(3) > 0 || len(model) == 0 {
		off := r.IntN(len(model) + 1)
		s := randText(r, r.IntN(8))
		b.Insert(off, s)
		return model[:off] + normalizeNewlines(s) + model[off:]
	}
	s := r.IntN(len(model))
	e := s + r.IntN(min(len(model)-s, 20)+1)
	b.Delete(s, e)
	return model[:s] + model[e:]
}

func randText(r *rand.Rand, n int) string {
	const alphabet = "ab \n\r\nxé€"
	var sb strings.Builder
	for range n {
		sb.WriteByte(alphabet[r.IntN(len(alphabet))])
	}
	return sb.String()
}

// FuzzBuffer decodes ops from the input and checks the buffer against a
// string model after each one.
func FuzzBuffer(f *testing.F) {
	f.Add("hello\nworld", []byte{0, 3, 'x', 1, 2, 5, 0, 0, '\r', 0, 9, '\n'})
	f.Add("", []byte{0, 0, '\r', 0, 1, '\n', 1, 0, 1})
	f.Add("a\r\nb", []byte{1, 0, 2, 0, 0, 'c'})
	f.Fuzz(func(t *testing.T, init string, ops []byte) {
		b := NewBuffer(init)
		model := normalizeNewlines(init)
		m := observe(t, b)
		for len(ops) >= 3 {
			kind, a, c := ops[0], int(ops[1]), ops[2]
			ops = ops[3:]
			if kind%2 == 0 {
				off := a % (len(model) + 1)
				s := string([]byte{c})
				if kind%4 == 2 {
					s = strings.Repeat(s, 3) + "\n"
				}
				b.Insert(off, s)
				model = model[:off] + normalizeNewlines(s) + model[off:]
			} else if len(model) > 0 {
				s := a % len(model)
				e := s + int(c)%(len(model)-s+1)
				b.Delete(s, e)
				model = model[:s] + model[e:]
			}
			check(t, b, model)
			m.check()
		}
	})
}

// megabyte returns about 1 MB of text in lines of up to 80 bytes.
func megabyte() string {
	r := rand.New(rand.NewPCG(3, 4))
	words := []string{"the", "quick", "brown", "fox", "jumps", "over", "lazy", "dog", "naïve", "café"}
	var sb strings.Builder
	line := 0
	for sb.Len() < 1<<20 {
		w := words[r.IntN(len(words))]
		if line+len(w) > 80 {
			sb.WriteByte('\n')
			line = 0
		} else if line > 0 {
			sb.WriteByte(' ')
			line++
		}
		sb.WriteString(w)
		line += len(w)
	}
	return sb.String()
}

func BenchmarkSetText1MB(b *testing.B) {
	s := megabyte()
	var buf Buffer
	b.SetBytes(int64(len(s)))
	for b.Loop() {
		buf.SetText(s)
	}
}

// edited returns a 1 MB buffer after n random inserts and deletes.
func edited(n int) *Buffer {
	r := rand.New(rand.NewPCG(5, 6))
	buf := NewBuffer(megabyte())
	for range n {
		randomEdit(r, buf)
	}
	return buf
}

func randomEdit(r *rand.Rand, buf *Buffer) {
	if r.IntN(2) == 0 {
		buf.Insert(r.IntN(buf.Len()+1), "word\n")
	} else {
		s := r.IntN(buf.Len())
		buf.Delete(s, min(s+5, buf.Len()))
	}
}

// BenchmarkRandomEdit is one random insert or delete in a 1 MB buffer that
// already has 1000 edits, followed by the paragraph lookup a widget does.
func BenchmarkRandomEdit(b *testing.B) {
	r := rand.New(rand.NewPCG(7, 8))
	buf := edited(1000)
	for i := 0; b.Loop(); i++ {
		if i%1000 == 999 {
			b.StopTimer()
			buf = edited(1000)
			b.StartTimer()
		}
		randomEdit(r, buf)
		buf.ParagraphAt(r.IntN(buf.Len() + 1))
	}
}

// BenchmarkTyping types one byte at a time in the middle of a 1 MB buffer.
func BenchmarkTyping(b *testing.B) {
	buf := NewBuffer(megabyte())
	off := buf.Len() / 2
	for b.Loop() {
		buf.Insert(off, "x")
		off++
		buf.ParagraphAt(off)
	}
}

func BenchmarkParagraphAt(b *testing.B) {
	r := rand.New(rand.NewPCG(9, 10))
	buf := edited(1000)
	for b.Loop() {
		buf.ParagraphAt(r.IntN(buf.Len() + 1))
	}
}

func BenchmarkParagraphStart(b *testing.B) {
	r := rand.New(rand.NewPCG(11, 12))
	buf := edited(1000)
	n := buf.ParagraphCount()
	for b.Loop() {
		buf.ParagraphStart(r.IntN(n))
	}
}

// BenchmarkSliceParagraph reads one paragraph's text, as the boundary
// functions of S5.1b will.
func BenchmarkSliceParagraph(b *testing.B) {
	r := rand.New(rand.NewPCG(13, 14))
	buf := edited(1000)
	for b.Loop() {
		buf.Slice(buf.ParagraphAt(r.IntN(buf.Len() + 1)))
	}
}

// mirror applies the changes a Buffer reports to a string model, so a test
// can check that the reported ranges reproduce the text. Every report must
// come with exactly one revision step and every revision step with a report.
type mirror struct {
	t     testing.TB
	b     *Buffer
	model string
	rev   uint64
	calls int
}

// observe attaches a mirror to b, starting from its current text.
func observe(t testing.TB, b *Buffer) *mirror {
	m := &mirror{t: t, b: b, model: b.String(), rev: b.Revision()}
	b.SetObserver(m.change)
	return m
}

func (m *mirror) change(start, oldEnd, newEnd int) {
	m.t.Helper()
	m.calls++
	if !(0 <= start && start <= oldEnd && oldEnd <= len(m.model) && start <= newEnd && newEnd <= m.b.Len()) {
		m.t.Fatalf("change(%d, %d, %d) out of range: model %d bytes, buffer %d", start, oldEnd, newEnd, len(m.model), m.b.Len())
	}
	if got := len(m.model) - (oldEnd - start) + (newEnd - start); got != m.b.Len() {
		m.t.Fatalf("change(%d, %d, %d): the model would have %d bytes, the buffer has %d", start, oldEnd, newEnd, got, m.b.Len())
	}
	if m.b.Revision() != m.rev+1 {
		m.t.Fatalf("change(%d, %d, %d) at revision %d, the last report was at %d", start, oldEnd, newEnd, m.b.Revision(), m.rev)
	}
	m.rev = m.b.Revision()
	m.model = m.model[:start] + m.b.Slice(start, newEnd) + m.model[oldEnd:]
}

// check fails unless the model is the buffer's text and every revision step
// was reported.
func (m *mirror) check() {
	m.t.Helper()
	if got := m.b.String(); got != m.model {
		m.t.Fatalf("the reported changes give %q, the buffer holds %q", m.model, got)
	}
	if m.b.Revision() != m.rev {
		m.t.Fatalf("revision %d, the last report was at %d", m.b.Revision(), m.rev)
	}
}

func TestBufferObserver(t *testing.T) {
	b := NewBuffer("hello\nworld")
	type call struct{ start, oldEnd, newEnd int }
	var got []call
	b.SetObserver(func(s, o, n int) { got = append(got, call{s, o, n}) })
	steps := []struct {
		name string
		do   func()
		want []call
	}{
		{"insert", func() { b.Insert(5, ", there") }, []call{{5, 5, 12}}},
		{"insert crlf", func() { b.Insert(0, "a\r\nb") }, []call{{0, 0, 3}}},
		{"empty insert", func() { b.Insert(2, "") }, nil},
		{"delete", func() { b.Delete(1, 4) }, []call{{1, 4, 1}}},
		{"empty delete", func() { b.Delete(3, 3) }, nil},
		{"reads", func() {
			_ = b.String()
			_ = b.Slice(1, 3)
			_, _ = b.ParagraphAt(4)
			_ = b.ParagraphStart(1)
			_ = b.ParagraphEnd(1)
			_ = b.ParagraphIndex(2)
			_ = b.ParagraphCount()
		}, nil},
		{"set text", func() { b.SetText("x\r\ny") }, []call{{0, 18, 3}}},
		{"set same text", func() { b.SetText("x\ny") }, []call{{0, 3, 3}}},
		{"clear", func() { b.SetText("") }, []call{{0, 3, 0}}},
	}
	for _, s := range steps {
		got = nil
		s.do()
		if fmt.Sprint(got) != fmt.Sprint(s.want) {
			t.Errorf("%s: reported %v, want %v", s.name, got, s.want)
		}
	}
	b.SetObserver(nil)
	got = nil
	b.Insert(0, "quiet")
	if got != nil {
		t.Errorf("after SetObserver(nil): reported %v", got)
	}
}

// TestEditorObserver checks that edits, undo, redo and SetText through an
// Editor each reach its buffer's observer exactly once (a replace is one
// report, review R5), and moves do not.
func TestEditorObserver(t *testing.T) {
	e := NewEditor("foo bar\nbaz", false)
	m := observe(t, e.Buffer())
	steps := []struct {
		name  string
		do    func()
		calls bool
	}{
		{"move", func() { e.Move(Word, Forward, false, nil) }, false},
		{"select", func() { e.SetSelection(0, 3) }, false},
		{"type over", func() { e.Insert("qux") }, true},
		{"select all", func() { e.SelectAll() }, false},
		{"backspace", func() { e.SetSelection(5, 5); e.DeleteBackward(Grapheme, nil) }, true},
		{"paste", func() { e.Paste("one\r\ntwo") }, true},
		{"type over a selection", func() { e.SetSelection(0, 4); e.Insert("z") }, true},
		{"undo", func() { e.Undo() }, true},
		{"redo", func() { e.Redo() }, true},
		{"set text", func() { e.SetText("new\ntext") }, true},
		{"nothing to undo", func() { e.Undo() }, false},
	}
	for _, s := range steps {
		before := m.calls
		s.do()
		m.check()
		if want := map[bool]int{true: 1}[s.calls]; m.calls-before != want {
			t.Errorf("%s: %d reports, want %d", s.name, m.calls-before, want)
		}
	}
}

// TestBufferReplace: Replace is Delete then Insert as one change: one
// revision, one report, the inserted length returned; a no-op changes and
// reports nothing.
func TestBufferReplace(t *testing.T) {
	b := NewBuffer("hello\nworld")
	m := observe(t, b)
	type call struct{ start, oldEnd, newEnd int }
	var got []call
	b.SetObserver(func(s, o, n int) { m.change(s, o, n); got = append(got, call{s, o, n}) })
	steps := []struct {
		start, end int
		text, want string
		n          int
		calls      []call
	}{
		{0, 5, "howdy", "howdy\nworld", 5, []call{{0, 5, 5}}},
		{5, 6, " \r\n", "howdy \nworld", 2, []call{{5, 6, 7}}},
		{3, 3, "", "howdy \nworld", 0, nil},
		{3, 3, "!", "how!dy \nworld", 1, []call{{3, 3, 4}}},
		{0, 4, "", "dy \nworld", 0, []call{{0, 4, 0}}},
		{0, 9, "", "", 0, []call{{0, 9, 0}}},
		{0, 0, "a\nb", "a\nb", 3, []call{{0, 0, 3}}},
	}
	for _, s := range steps {
		got = nil
		rev := b.Revision()
		if n := b.Replace(s.start, s.end, s.text); n != s.n {
			t.Errorf("Replace(%d, %d, %q) = %d, want %d", s.start, s.end, s.text, n, s.n)
		}
		check(t, b, s.want)
		m.check()
		if fmt.Sprint(got) != fmt.Sprint(s.calls) {
			t.Errorf("Replace(%d, %d, %q): reported %v, want %v", s.start, s.end, s.text, got, s.calls)
		}
		if d := b.Revision() - rev; d != uint64(len(s.calls)) {
			t.Errorf("Replace(%d, %d, %q): revision moved by %d", s.start, s.end, s.text, d)
		}
	}
}

// TestBufferTypingBackspaceReusesAdd (review B13): deleting text that ends
// at the add buffer's end gives that tail back, so cycles of typing and
// backspace neither add pieces nor grow the add buffer, and the next insert
// there extends the piece before it.
func TestBufferTypingBackspaceReusesAdd(t *testing.T) {
	b := NewBuffer("hello world")
	b.Insert(5, ",")
	pieces, add := len(b.pieces), len(b.add)
	for i := range 1000 {
		at := 6 + i%3
		b.Insert(at, "ab\n")
		b.Delete(at+1, at+3)
		b.Delete(at, at+1)
		if len(b.pieces) > pieces+2 || len(b.add) > add || len(b.addNL) != 0 {
			t.Fatalf("cycle %d: %d pieces (from %d), add buffer %d bytes (from %d), %d newlines",
				i, len(b.pieces), pieces, len(b.add), add, len(b.addNL))
		}
	}
	check(t, b, "hello, world")
	b.Insert(12, "xyz")
	b.Delete(0, 1)
	b.Delete(13, 14) // the "z": the add buffer's tail
	check(t, b, "ello, worldxy")
	n := len(b.pieces)
	b.Insert(13, "!")
	check(t, b, "ello, worldxy!")
	if len(b.pieces) != n {
		t.Errorf("the insert after the trimmed tail made a piece: %d pieces, want %d", len(b.pieces), n)
	}
	// Deleting text that does not end the add buffer keeps it.
	b.Insert(0, "E")
	add = len(b.add)
	b.Delete(12, 14)
	check(t, b, "Eello, world!")
	if len(b.add) != add {
		t.Errorf("add buffer %d bytes after deleting inside an earlier insert, want %d", len(b.add), add)
	}
}
