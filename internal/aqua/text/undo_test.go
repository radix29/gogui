package text

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
)

func typing(s string) func(*Editor) {
	return func(e *Editor) {
		for _, r := range s {
			e.Insert(string(r))
		}
	}
}

func ops(fs ...func(*Editor)) func(*Editor) {
	return func(e *Editor) {
		for _, f := range fs {
			f(e)
		}
	}
}

const acute = "\xcc\x81" // U+0301, a combining acute accent

func undo(e *Editor) { e.Undo() }
func redo(e *Editor) { e.Redo() }

func bs(u Unit) func(*Editor)  { return func(e *Editor) { e.DeleteBackward(u, nil) } }
func del(u Unit) func(*Editor) { return func(e *Editor) { e.DeleteForward(u, nil) } }

func TestUndo(t *testing.T) {
	left, right := mv(Grapheme, Backward, false), mv(Grapheme, Forward, false)
	runEditorCases(t, []editorCase{
		{"typing coalesces", false, "a|b", ops(typing("xyz"), undo), "a|b"},
		{"typing redo", false, "a|b", ops(typing("xyz"), undo, redo), "axyz|b"},
		{"undo again", false, "a|b", ops(typing("xyz"), undo, undo), "a|b"},
		{"move breaks", false, "|", ops(typing("ab"), left, right, typing("cd"), undo), "ab|"},
		{"move breaks twice", false, "|", ops(typing("ab"), left, right, typing("cd"), undo, undo), "|"},
		{"select breaks", false, "|", ops(typing("ab"), func(e *Editor) { e.SetSelection(2, 2) }, typing("c"), undo), "ab|"},
		{"select all breaks", false, "|", ops(typing("ab"), (*Editor).SelectAll, typing("c"), undo), "^ab|"},
		{"select word breaks", false, "|", ops(typing("ab"), func(e *Editor) { e.SelectWord(1) }, typing("c"), undo), "^ab|"},
		{"select paragraph breaks", false, "|", ops(typing("ab"), func(e *Editor) { e.SelectParagraph(1) }, typing("c"), undo), "^ab|"},
		{"over a selection", false, "a^bc|d", ops(typing("xy"), undo), "a^bc|d"},
		{"over a reversed selection", false, "a|bc^d", ops(typing("xy"), undo), "a|bc^d"},
		{"over a selection redo", false, "a^bc|d", ops(typing("xy"), undo, redo), "axy|d"},
		{"newline is its own group", false, "|", ops(typing("ab"), typing("\n"), typing("cd"), undo), "ab\n|"},
		{"newline undone", false, "|", ops(typing("ab"), typing("\n"), typing("cd"), undo, undo), "ab|"},
		{"newline then typing", false, "|", ops(typing("ab"), typing("\n"), typing("cd"), undo, undo, undo), "|"},
		{"single-line newline is a space", true, "|", ops(typing("a\nb"), undo), "|"},
		{"paste is its own group", false, "|", ops(typing("a"), func(e *Editor) { e.Paste("bc") }, typing("d"), undo), "abc|"},
		{"paste undone", false, "|", ops(typing("a"), func(e *Editor) { e.Paste("bc") }, typing("d"), undo, undo), "a|"},
		{"pastes do not coalesce", false, "|", ops(func(e *Editor) { e.Paste("a"); e.Paste("b") }, undo), "a|"},
		{"cut", false, "a^bc|d", ops(func(e *Editor) { e.Cut() }, undo), "a^bc|d"},
		{"cut redo", false, "a^bc|d", ops(func(e *Editor) { e.Cut() }, undo, redo), "a|d"},
		{"cut then typing", false, "a^bc|d", ops(func(e *Editor) { e.Cut() }, typing("x"), undo), "a|d"},
		{"empty cut records nothing", false, "a|b", ops(typing("x"), func(e *Editor) { e.Cut() }, typing("y"), undo), "a|b"},
		{"backspaces coalesce", false, "abcd|", ops(bs(Grapheme), bs(Grapheme), bs(Grapheme), undo), "abcd|"},
		{"backspaces redo", false, "abcd|", ops(bs(Grapheme), bs(Grapheme), undo, redo), "ab|"},
		{"forward deletes coalesce", false, "|abcd", ops(del(Grapheme), del(Grapheme), del(Grapheme), undo), "|abcd"},
		{"directions break", false, "ab|cd", ops(bs(Grapheme), del(Grapheme), undo), "a|cd"},
		{"directions break twice", false, "ab|cd", ops(bs(Grapheme), del(Grapheme), undo, undo), "ab|cd"},
		{"word deletes coalesce", false, "foo bar baz|", ops(bs(Word), bs(Word), undo), "foo bar baz|"},
		{"units coalesce", false, "foo bar|", ops(bs(Grapheme), bs(Word), undo), "foo bar|"},
		{"backspace across paragraphs", false, "a\n|b", ops(bs(Grapheme), bs(Grapheme), undo), "a\n|b"},
		{"selection then backspace", false, "ab^cd|e", ops(bs(Grapheme), bs(Grapheme), undo), "ab^cd|e"},
		{"typing then deleting", false, "|", ops(typing("ab"), bs(Grapheme), undo), "ab|"},
		{"deleting then typing", false, "ab|", ops(bs(Grapheme), typing("c"), undo), "a|"},
		{"no-op deletes record nothing", false, "|ab", ops(bs(Grapheme), typing("x"), bs(Grapheme), undo), "x|ab"},
		{"cluster join", false, "|" + acute + "x", ops(typing("e"), undo), "|" + acute + "x"},
		{"cluster join redo", false, "|" + acute + "x", ops(typing("e"), undo, redo), "e" + acute + "|x"},
		{"typing after a join", false, "|" + acute + "x", ops(typing("ey"), undo), "|" + acute + "x"},
		{"redo cleared by an edit", false, "|", ops(typing("a"), undo, typing("b"), redo), "b|"},
		{"redo cleared by a delete", false, "x|", ops(typing("a"), undo, bs(Grapheme), redo), "|"},
		{"redo kept by a move", false, "|", ops(typing("a"), undo, right, redo), "a|"},
		{"two groups redo", false, "|", ops(typing("a"), left, right, typing("b"), undo, undo, redo, redo), "ab|"},
		{"nothing to undo", false, "a|b", ops(undo, redo), "a|b"},
		{"long typing", false, "|", ops(typing(strings.Repeat("x", 3*mergeMax+5)), undo), "|"},
		{"long backspacing", false, strings.Repeat("y", 3*mergeMax+5) + "|", ops(repeat(3*mergeMax+5, bs(Grapheme)), undo), strings.Repeat("y", 3*mergeMax+5) + "|"},
		{"long forward deleting", false, "|" + strings.Repeat("y", 3*mergeMax+5), ops(repeat(3*mergeMax+5, del(Grapheme)), undo), "|" + strings.Repeat("y", 3*mergeMax+5)},
	})
}

func TestUndoGroups(t *testing.T) {
	e := NewEditor("", false)
	typing(strings.Repeat("x", 3*mergeMax+5))(e)
	if n := len(e.hist.undo); n != 1 {
		t.Fatalf("%d groups, want 1", n)
	}
	if n := len(e.hist.undo[0].edits); n != 4 {
		t.Errorf("%d edits, want 4 (merged up to %d bytes)", n, mergeMax)
	}
	if !e.CanUndo() || e.CanRedo() {
		t.Error("CanUndo/CanRedo before the undo")
	}
	if !e.Undo() || e.Undo() {
		t.Error("Undo results")
	}
	if e.CanUndo() || !e.CanRedo() {
		t.Error("CanUndo/CanRedo after the undo")
	}
	e.SetText("new")
	if e.CanUndo() || e.CanRedo() || e.Redo() {
		t.Error("SetText did not clear the history")
	}
}

func TestUndoLimit(t *testing.T) {
	e := NewEditor("", false)
	e.SetUndoLimit(3)
	for i := range 10 {
		e.Paste(fmt.Sprint(i))
	}
	for range 3 {
		if !e.Undo() {
			t.Fatal("Undo failed within the limit")
		}
	}
	if e.Undo() {
		t.Error("an undo past the limit")
	}
	if got := formatSpec(e); got != "0123456|" {
		t.Errorf("after the undos %q", got)
	}
	e.SetUndoLimit(1) // trims the redo stack too
	if !e.Redo() || e.Redo() {
		t.Error("the redo stack is not trimmed to 1")
	}
	e.SetUndoLimit(0)
	for i := range 2*DefaultUndoLimit + 1 {
		e.Paste(fmt.Sprint(i % 10))
	}
	if n := len(e.hist.undo); n != 2*DefaultUndoLimit+2 {
		t.Errorf("unlimited history keeps %d groups", n)
	}
	if d := NewEditor("", false); d.hist.limit != DefaultUndoLimit {
		t.Errorf("default limit %d", d.hist.limit)
	}
}

// edState is a snapshot of an editor for the undo model.
type edState struct {
	text         string
	anchor, head int
}

func snap(e *Editor) edState { return edState{e.Text(), e.Anchor(), e.Head()} }

// runUndoOps applies ops, with Undo and Redo among them, to an editor with
// the default span and one with tiny spans, which must agree. A model keeps a
// snapshot before and after each undo group (a group is new when the undo
// stack grows, else an edit that changes the text joined the top group):
// Undo must give back the snapshot before, Redo the one after. At the end
// undoing everything gives the start text, and redoing everything the end.
func runUndoOps(t testing.TB, text string, single bool, ops []byte) {
	e, small := NewEditor(text, single), NewEditor(text, single)
	small.spanMin = 1
	m := observe(t, e.Buffer())
	type entry struct{ before, after edState }
	var model, redoModel []entry
	start := snap(e)
	var log []string
	for i := 0; i+1 < len(ops); i += 2 {
		op, arg := ops[i], ops[i+1]
		before, groups := snap(e), len(e.hist.undo)
		switch op % 16 {
		case 13:
			s := pieces[int(arg)%len(pieces)]
			e.Paste(s)
			small.Paste(s)
			log = append(log, fmt.Sprintf("Paste(%q)", s))
		case 14:
			log = append(log, "Undo")
			ok := e.Undo()
			small.Undo()
			if ok != (len(model) > 0) {
				t.Fatalf("after %v: Undo = %v with %d groups in the model", log, ok, len(model))
			}
			if ok {
				m := model[len(model)-1]
				model = model[:len(model)-1]
				if got := snap(e); got != m.before {
					t.Fatalf("after %v: %+v, want %+v", log, got, m.before)
				}
				redoModel = append(redoModel, m)
			}
		case 15:
			log = append(log, "Redo")
			ok := e.Redo()
			small.Redo()
			if ok != (len(redoModel) > 0) {
				t.Fatalf("after %v: Redo = %v with %d groups in the redo model", log, ok, len(redoModel))
			}
			if ok {
				m := redoModel[len(redoModel)-1]
				redoModel = redoModel[:len(redoModel)-1]
				if got := snap(e); got != m.after {
					t.Fatalf("after %v: %+v, want %+v", log, got, m.after)
				}
				model = append(model, m)
			}
		default:
			var lines, smallLines Lines
			if op&0x80 != 0 {
				lines, smallLines = fakeLines{e, 3, 2}, fakeLines{small, 3, 2}
			}
			log = append(log, editorOp(t, e, lines, op, arg))
			editorOp(t, small, smallLines, op, arg)
		}
		if op%16 < 14 {
			switch now := snap(e); {
			case len(e.hist.undo) > groups:
				model = append(model, entry{before, now})
				redoModel = nil
			case now.text != before.text:
				if len(model) == 0 {
					t.Fatalf("after %v: an edit with no undo group", log)
				}
				model[len(model)-1].after = now
				redoModel = nil
			}
			if len(redoModel) != len(e.hist.redo) {
				t.Fatalf("after %v: %d redo groups, model %d", log, len(e.hist.redo), len(redoModel))
			}
		}
		checkEditor(t, e)
		m.check()
		if a, b := formatSpec(e), formatSpec(small); a != b {
			t.Fatalf("after %v: %q, tiny spans %q", log, a, b)
		}
	}
	end := e.Text()
	if len(redoModel) > 0 {
		end = redoModel[0].after.text // redoing all goes past the undos at the end
	}
	for e.Undo() {
	}
	m.check()
	if got := snap(e); len(model) > 0 && got != model[0].before || got.text != start.text {
		t.Fatalf("after %v and undoing all: %+v, want %q", log, got, start.text)
	}
	for e.Redo() {
	}
	m.check()
	if e.Text() != end {
		t.Fatalf("after %v, undoing and redoing all: %q, want %q", log, e.Text(), end)
	}
}

func TestUndoRandom(t *testing.T) {
	r := rand.New(rand.NewPCG(11, 13))
	for range 400 {
		text := randPieces(r, r.IntN(30))
		ops := make([]byte, 2*(1+r.IntN(60)))
		for i := range ops {
			ops[i] = byte(r.Uint32())
		}
		runUndoOps(t, text, r.IntN(4) == 0, ops)
	}
}

func FuzzUndo(f *testing.F) {
	f.Add("foo bar\nbaz", []byte{4, 1, 4, 2, 14, 0, 15, 0, 2, 0, 2, 0, 14, 0})
	f.Add(family+" don't\r\n3.14", []byte{13, 3, 0x80, 0x88, 9, 0, 14, 0, 14, 0, 15, 0})
	f.Add("á́ "+flagDE, []byte{3, 0, 3, 0, 5, 7, 14, 1, 15, 1})
	f.Fuzz(func(t *testing.T, text string, ops []byte) {
		if len(text) > 256 {
			text = text[:256]
		}
		if len(ops) > 128 {
			ops = ops[:128]
		}
		single := len(ops) > 0 && ops[0]%5 == 0
		runUndoOps(t, text, single, ops)
	})
}

func benchUndo(b *testing.B, text string) {
	var e *Editor
	var mid int
	fresh := func() {
		e = NewEditor(text, false) // each case on the untouched text
		mid = e.clusterStart(e.Len() / 2)
	}
	b.Run("TypeWordAndUndo", func(b *testing.B) {
		fresh()
		e.SetSelection(mid, mid)
		for b.Loop() {
			typing("hello")(e)
			e.Undo()
		}
	})
	b.Run("UndoRedoWord", func(b *testing.B) {
		fresh()
		e.SetSelection(mid, mid)
		typing("hello")(e)
		for b.Loop() {
			e.Undo()
			e.Redo()
		}
	})
	b.Run("UndoRedoParagraphDelete", func(b *testing.B) {
		fresh()
		e.SelectParagraph(mid)
		e.DeleteBackward(Grapheme, nil)
		for b.Loop() {
			e.Undo()
			e.Redo()
		}
	})
	b.Run("UndoRedoTyping1000", func(b *testing.B) {
		fresh()
		e.SetSelection(mid, mid)
		typing(strings.Repeat("x", 1000))(e)
		for b.Loop() {
			e.Undo()
			e.Redo()
		}
	})
	b.Run("UndoRedoSelectAllDelete", func(b *testing.B) {
		fresh()
		e.SelectAll()
		e.DeleteBackward(Grapheme, nil)
		for b.Loop() {
			e.Undo()
			e.Redo()
		}
	})
}

// BenchmarkUndo1MB types, undoes and redoes in the middle of 1 MB of
// 80-column text.
func BenchmarkUndo1MB(b *testing.B) { benchUndo(b, megabyte()) }

// BenchmarkUndoLongParagraph1MB does the same in one 1 MB paragraph.
func BenchmarkUndoLongParagraph1MB(b *testing.B) {
	benchUndo(b, paragraph1MB("Grüße, the café's naïve résumé costs 3.14 € today. "))
}

// TestBreakUndoGroup: BreakUndoGroup seals the open group, so an IME commit
// (an Insert between two breaks, S5.3) undoes on its own, apart from the
// typing before and after it; it changes neither the text nor the
// selection nor the redo stack.
func TestBreakUndoGroup(t *testing.T) {
	brk := (*Editor).BreakUndoGroup
	commit := func(s string) func(*Editor) {
		return ops(brk, func(e *Editor) { e.Insert(s) }, brk)
	}
	runEditorCases(t, []editorCase{
		{"no-op", false, "a^b|c", brk, "a^b|c"},
		{"keeps redo", false, "|", ops(typing("a"), undo, brk, redo), "a|"},
		{"splits typing", false, "|", ops(typing("ab"), brk, typing("cd"), undo), "ab|"},
		{"splits deletes", false, "abcd|", ops(bs(Grapheme), brk, bs(Grapheme), undo), "abc|"},
		{"commit after typing", false, "|", ops(typing("ab"), commit("日本"), undo), "ab|"},
		{"commit after typing, twice", false, "|", ops(typing("ab"), commit("日本"), undo, undo), "|"},
		{"commit redo", false, "|", ops(typing("ab"), commit("日本"), undo, redo), "ab日本|"},
		{"typing after a commit", false, "|", ops(commit("日本"), typing("x"), undo), "日本|"},
		{"typing after a commit, twice", false, "|", ops(commit("日本"), typing("x"), undo, undo), "|"},
		{"commits apart", false, "|", ops(commit("日"), commit("本"), undo), "日|"},
		{"commit over a selection", false, "a^bc|d", ops(commit("語"), undo), "a^bc|d"},
		{"single-line commit", true, "x|", ops(typing("y"), commit("中文"), undo), "xy|"},
	})
}
