package text

import (
	"fmt"
	"testing"

	"github.com/radix29/gogui/internal/driver"
)

// wantBindings is the whole keymap, written out independently of
// buildKeymaps. multi marks the bindings of a multi-line editor only.
func wantBindings() map[chord]struct {
	cmd   Command
	multi bool
} {
	const (
		S = driver.ModShift
		O = driver.ModOption
		P = driver.ModControl
	)
	type b = struct {
		key   driver.Key
		mods  driver.Modifiers
		cmd   Command
		multi bool
	}
	list := []b{
		{driver.KeyLeft, 0, CmdMoveLeft, false},
		{driver.KeyLeft, S, CmdSelectLeft, false},
		{driver.KeyRight, 0, CmdMoveRight, false},
		{driver.KeyRight, S, CmdSelectRight, false},
		{driver.KeyLeft, O, CmdMoveWordLeft, false},
		{driver.KeyLeft, S | O, CmdSelectWordLeft, false},
		{driver.KeyRight, O, CmdMoveWordRight, false},
		{driver.KeyRight, S | O, CmdSelectWordRight, false},
		{driver.KeyLeft, P, CmdMoveToLineStart, false},
		{driver.KeyLeft, S | P, CmdSelectToLineStart, false},
		{driver.KeyRight, P, CmdMoveToLineEnd, false},
		{driver.KeyRight, S | P, CmdSelectToLineEnd, false},
		{driver.KeyUp, 0, CmdMoveUp, false},
		{driver.KeyUp, S, CmdSelectUp, false},
		{driver.KeyDown, 0, CmdMoveDown, false},
		{driver.KeyDown, S, CmdSelectDown, false},
		{driver.KeyUp, O, CmdMoveParagraphBack, false},
		{driver.KeyUp, S | O, CmdSelectParagraphBack, false},
		{driver.KeyDown, O, CmdMoveParagraphFwd, false},
		{driver.KeyDown, S | O, CmdSelectParagraphFwd, false},
		{driver.KeyUp, P, CmdMoveToDocumentStart, false},
		{driver.KeyUp, S | P, CmdSelectToDocumentStart, false},
		{driver.KeyDown, P, CmdMoveToDocumentEnd, false},
		{driver.KeyDown, S | P, CmdSelectToDocumentEnd, false},
		{driver.KeyHome, 0, CmdMoveToDocumentStart, false},
		{driver.KeyHome, S, CmdSelectToDocumentStart, false},
		{driver.KeyEnd, 0, CmdMoveToDocumentEnd, false},
		{driver.KeyEnd, S, CmdSelectToDocumentEnd, false},
		{driver.KeyPageUp, 0, CmdPageUp, false},
		{driver.KeyPageUp, S, CmdSelectPageUp, false},
		{driver.KeyPageUp, O, CmdPageUp, false},
		{driver.KeyPageUp, S | O, CmdSelectPageUp, false},
		{driver.KeyPageDown, 0, CmdPageDown, false},
		{driver.KeyPageDown, S, CmdSelectPageDown, false},
		{driver.KeyPageDown, O, CmdPageDown, false},
		{driver.KeyPageDown, S | O, CmdSelectPageDown, false},
		{driver.KeyBackspace, 0, CmdDeleteBackward, false},
		{driver.KeyBackspace, S, CmdDeleteBackward, false},
		{driver.KeyBackspace, O, CmdDeleteWordBackward, false},
		{driver.KeyBackspace, S | O, CmdDeleteWordBackward, false},
		{driver.KeyBackspace, P, CmdDeleteToLineStart, false},
		{driver.KeyBackspace, S | P, CmdDeleteToLineStart, false},
		{driver.KeyDelete, 0, CmdDeleteForward, false},
		{driver.KeyDelete, S, CmdDeleteForward, false},
		{driver.KeyDelete, O, CmdDeleteWordForward, false},
		{driver.KeyDelete, S | O, CmdDeleteWordForward, false},
		{driver.KeyDelete, P, CmdDeleteToLineEnd, false},
		{driver.KeyDelete, S | P, CmdDeleteToLineEnd, false},
		{'A', P, CmdSelectAll, false},
		{'C', P, CmdCopy, false},
		{'X', P, CmdCut, false},
		{'V', P, CmdPaste, false},
		{'Z', P, CmdUndo, false},
		{'Z', S | P, CmdRedo, false},
		{driver.KeyReturn, 0, CmdInsertNewline, true},
		{driver.KeyReturn, S, CmdInsertNewline, true},
		{driver.KeyTab, 0, CmdInsertTab, true},
	}
	m := make(map[chord]struct {
		cmd   Command
		multi bool
	})
	for _, x := range list {
		c := chord{x.key, x.mods}
		if _, dup := m[c]; dup {
			panic(fmt.Sprintf("duplicate binding %v", c))
		}
		m[c] = struct {
			cmd   Command
			multi bool
		}{x.cmd, x.multi}
	}
	return m
}

// sweepKeys is every key the table test tries: the named keys, the letters
// in both cases, digits and some punctuation.
func sweepKeys() []driver.Key {
	keys := []driver.Key{
		driver.KeyNone, driver.KeyBackspace, driver.KeyTab, driver.KeyReturn,
		driver.KeyEscape, driver.KeySpace, driver.KeyUp, driver.KeyDown,
		driver.KeyLeft, driver.KeyRight, driver.KeyDelete, driver.KeyHome,
		driver.KeyEnd, driver.KeyPageUp, driver.KeyPageDown,
		'-', '=', '[', ']', ';', '\'', ',', '.', '/', '\\', '`',
	}
	for k := driver.KeyF1; k <= driver.KeyF12; k++ {
		keys = append(keys, k)
	}
	for k := driver.Key('0'); k <= '9'; k++ {
		keys = append(keys, k)
	}
	for k := driver.Key('A'); k <= 'Z'; k++ {
		keys = append(keys, k, k+'a'-'A')
	}
	return keys
}

func TestLookupTable(t *testing.T) {
	want := wantBindings()
	n := 0
	for _, key := range sweepKeys() {
		canon := key
		if key >= 'a' && key <= 'z' {
			canon -= 'a' - 'A'
		}
		for mods := driver.Modifiers(0); mods < driver.ModPrimary; mods++ {
			for _, multi := range []bool{false, true} {
				w, bound := want[chord{canon, mods}]
				if bound && w.multi && !multi {
					bound = false
				}
				cmd, ok := Lookup(key, mods, multi)
				switch {
				case bound && (!ok || cmd != w.cmd):
					t.Errorf("Lookup(%#x, %#x, %v) = %v, %v; want %v", key, mods, multi, cmd, ok, w.cmd)
				case !bound && (ok || cmd != CmdNone):
					t.Errorf("Lookup(%#x, %#x, %v) = %v, %v; want none", key, mods, multi, cmd, ok)
				case bound && key == canon:
					n++
				}
			}
		}
	}
	// Every binding was met: both modes for the shared ones, one for the rest.
	shared := 0
	for _, w := range want {
		if !w.multi {
			shared++
		}
	}
	if wantN := 2*shared + (len(want) - shared); n != wantN {
		t.Errorf("met %d bindings, want %d", n, wantN)
	}
}

func TestLookupPrimary(t *testing.T) {
	// A logical Primary that slips in is Ctrl; the Windows/Super key and
	// AltGr (Ctrl+Alt) are never bound.
	for _, tc := range []struct {
		key  driver.Key
		mods driver.Modifiers
		want Command
		ok   bool
	}{
		{driver.KeyLeft, driver.ModPrimary, CmdMoveToLineStart, true},
		{driver.KeyLeft, driver.ModPrimary | driver.ModShift, CmdSelectToLineStart, true},
		{'Z', driver.ModPrimary | driver.ModShift, CmdRedo, true},
		{'c', driver.ModPrimary | driver.ModControl, CmdCopy, true},
		{'V', driver.ModCommand, CmdNone, false},
		{driver.KeyLeft, driver.ModCommand, CmdNone, false},
		{'V', driver.ModControl | driver.ModOption, CmdNone, false},
		{driver.KeyLeft, driver.ModControl | driver.ModOption, CmdNone, false},
		{driver.KeyTab, driver.ModShift, CmdNone, false},
		{driver.KeyTab, driver.ModControl, CmdNone, false},
		{driver.KeyReturn, driver.ModControl, CmdNone, false},
		{driver.KeyEscape, 0, CmdNone, false},
	} {
		got, ok := Lookup(tc.key, tc.mods, true)
		if got != tc.want || ok != tc.ok {
			t.Errorf("Lookup(%#x, %#x) = %v, %v; want %v, %v", tc.key, tc.mods, got, ok, tc.want, tc.ok)
		}
	}
}

func TestCommandNames(t *testing.T) {
	seen := map[string]bool{}
	reached := map[Command]bool{}
	for _, w := range wantBindings() {
		reached[w.cmd] = true
	}
	for c := CmdNone; c < numCommands; c++ {
		s := c.String()
		if s == "" || seen[s] {
			t.Errorf("command %d: name %q empty or repeated", c, s)
		}
		seen[s] = true
		if c != CmdNone && !reached[c] {
			t.Errorf("%v has no key", c)
		}
	}
	if s := numCommands.String(); s != fmt.Sprintf("Command(%d)", numCommands) {
		t.Errorf("out of range: %q", s)
	}
	if len(moveSpecs)*2 != int(CmdDeleteBackward-CmdMoveLeft) {
		t.Errorf("moveSpecs has %d pairs, the move commands %d", len(moveSpecs), CmdDeleteBackward-CmdMoveLeft)
	}
	for c := CmdNone; c < numCommands; c++ {
		if want := c == CmdCopy || c == CmdCut || c == CmdPaste; c.IsClipboard() != want {
			t.Errorf("%v.IsClipboard() = %v", c, !want)
		}
		edits := map[Command]bool{
			CmdDeleteBackward: true, CmdDeleteForward: true, CmdDeleteWordBackward: true,
			CmdDeleteWordForward: true, CmdDeleteToLineStart: true, CmdDeleteToLineEnd: true,
			CmdInsertNewline: true, CmdInsertTab: true, CmdCut: true, CmdPaste: true,
			CmdUndo: true, CmdRedo: true,
		}
		if c.Edits() != edits[c] {
			t.Errorf("%v.Edits() = %v", c, !edits[c])
		}
		// A command that does not edit leaves the text alone, with a
		// selection, a caret, and an undo and a redo to do.
		if !c.Edits() {
			for _, single := range []bool{false, true} {
				e := NewEditor("", single)
				e.Insert("one two\nthree")
				e.Undo()
				e.Redo()
				e.Insert(" x")
				e.Undo()
				for _, sel := range [][2]int{{4, 4}, {2, 9}} {
					e.SetSelection(sel[0], sel[1])
					rev := e.Buffer().Revision()
					e.Do(c, nil)
					if e.Buffer().Revision() != rev {
						t.Errorf("%v (single %v) changed the text", c, single)
					}
				}
			}
		}
	}
}

type doCase struct {
	name    string
	single  bool
	spec    string
	cmds    []Command
	wrap    int // > 0: a fakeLines wrapping every wrap clusters, a page of 2 lines
	want    string
	handled bool
}

func TestDo(t *testing.T) {
	c := func(cmds ...Command) []Command { return cmds }
	const doc = "one two\nthree four\nfive"
	at := func(i int) string { return doc[:i] + "|" + doc[i:] }
	for _, tc := range []doCase{
		{"left", false, at(5), c(CmdMoveLeft), 0, at(4), true},
		{"select left", false, at(5), c(CmdSelectLeft), 0, "one |t^wo\nthree four\nfive", true},
		{"right", false, at(5), c(CmdMoveRight), 0, at(6), true},
		{"select right", false, at(5), c(CmdSelectRight), 0, "one t^w|o\nthree four\nfive", true},
		{"left collapses", false, "one ^two|\nthree four\nfive", c(CmdMoveLeft), 0, at(4), true},
		{"word left", false, at(6), c(CmdMoveWordLeft), 0, at(4), true},
		{"select word left", false, at(6), c(CmdSelectWordLeft), 0, "one |tw^o\nthree four\nfive", true},
		{"word right", false, at(5), c(CmdMoveWordRight), 0, at(7), true},
		{"select word right", false, at(5), c(CmdSelectWordRight), 0, "one t^wo|\nthree four\nfive", true},
		{"line start", false, at(12), c(CmdMoveToLineStart), 0, at(8), true},
		{"select line start", false, at(12), c(CmdSelectToLineStart), 0, "one two\n|thre^e four\nfive", true},
		{"line end", false, at(12), c(CmdMoveToLineEnd), 0, at(18), true},
		{"select line end", false, at(12), c(CmdSelectToLineEnd), 0, "one two\nthre^e four|\nfive", true},
		{"wrapped line end", false, at(12), c(CmdMoveToLineEnd), 4, at(16), true},
		{"wrapped line start", false, at(13), c(CmdMoveToLineStart), 4, at(12), true},
		{"up", false, at(12), c(CmdMoveUp), 0, at(4), true},
		{"select up", false, at(12), c(CmdSelectUp), 0, "one |two\nthre^e four\nfive", true},
		{"down", false, at(12), c(CmdMoveDown), 0, at(23), true},
		{"select down", false, at(12), c(CmdSelectDown), 0, "one two\nthre^e four\nfive|", true},
		{"down twice keeps x", false, at(4), c(CmdMoveDown, CmdMoveDown), 0, at(23), true},
		{"wrapped down", false, at(9), c(CmdMoveDown), 4, at(13), true},
		{"paragraph back", false, at(12), c(CmdMoveParagraphBack), 0, at(8), true},
		{"paragraph back again", false, at(12), c(CmdMoveParagraphBack, CmdMoveParagraphBack), 0, at(0), true},
		{"select paragraph back", false, at(12), c(CmdSelectParagraphBack), 0, "one two\n|thre^e four\nfive", true},
		{"paragraph fwd", false, at(12), c(CmdMoveParagraphFwd), 0, at(18), true},
		{"select paragraph fwd", false, at(12), c(CmdSelectParagraphFwd), 0, "one two\nthre^e four|\nfive", true},
		{"document start", false, at(12), c(CmdMoveToDocumentStart), 0, at(0), true},
		{"select document start", false, at(12), c(CmdSelectToDocumentStart), 0, "|one two\nthre^e four\nfive", true},
		{"document end", false, at(12), c(CmdMoveToDocumentEnd), 0, at(23), true},
		{"select document end", false, at(12), c(CmdSelectToDocumentEnd), 0, "one two\nthre^e four\nfive|", true},
		{"page up", false, at(20), c(CmdPageUp), 4, at(13), true},
		{"select page up", false, at(20), c(CmdSelectPageUp), 4, "one two\nthree| four\nf^ive", true},
		{"page down", false, at(1), c(CmdPageDown), 4, at(9), true},
		{"select page down", false, at(1), c(CmdSelectPageDown), 4, "o^ne two\nt|hree four\nfive", true},
		{"single up", true, "one t|wo", c(CmdMoveUp), 0, "|one two", true},
		{"single page down", true, "one t|wo", c(CmdPageDown), 0, "one two|", true},

		{"backspace", false, at(5), c(CmdDeleteBackward), 0, "one |wo\nthree four\nfive", true},
		{"delete", false, at(5), c(CmdDeleteForward), 0, "one t|o\nthree four\nfive", true},
		{"backspace selection", false, "one ^two|\nthree four\nfive", c(CmdDeleteBackward), 0, "one |\nthree four\nfive", true},
		{"word backspace", false, at(7), c(CmdDeleteWordBackward), 0, "one |\nthree four\nfive", true},
		{"word delete", false, at(3), c(CmdDeleteWordForward), 0, "one|\nthree four\nfive", true},
		{"line backspace", false, at(12), c(CmdDeleteToLineStart), 0, "one two\n|e four\nfive", true},
		{"line delete", false, at(12), c(CmdDeleteToLineEnd), 0, "one two\nthre|\nfive", true},
		{"line backspace joins", false, at(8), c(CmdDeleteToLineStart), 0, "one two|three four\nfive", true},
		{"wrapped line delete", false, at(9), c(CmdDeleteToLineEnd), 4, "one two\nt|e four\nfive", true},

		{"newline", false, at(7), c(CmdInsertNewline), 0, "one two\n|\nthree four\nfive", true},
		{"newline single", true, "ab|c", c(CmdInsertNewline), 0, "ab|c", false},
		{"tab", false, at(3), c(CmdInsertTab), 0, "one\t| two\nthree four\nfive", true},
		{"tab single", true, "ab|c", c(CmdInsertTab), 0, "ab\t|c", true},
		{"select all", false, at(5), c(CmdSelectAll), 0, "^" + doc + "|", true},
		{"undo", false, at(5), c(CmdDeleteBackward, CmdDeleteBackward, CmdUndo), 0, at(5), true},
		{"redo", false, at(5), c(CmdDeleteBackward, CmdUndo, CmdRedo), 0, "one |wo\nthree four\nfive", true},
		{"undo nothing", false, at(5), c(CmdUndo), 0, at(5), true},
		{"redo nothing", false, at(5), c(CmdRedo), 0, at(5), true},
		{"newline undoes alone", false, "ab|", c(CmdInsertTab, CmdInsertNewline, CmdUndo), 0, "ab\t|", true},

		{"none", false, at(5), c(CmdNone), 0, at(5), false},
		{"copy", false, "one ^two|\nthree four\nfive", c(CmdCopy), 0, "one ^two|\nthree four\nfive", false},
		{"cut", false, "one ^two|\nthree four\nfive", c(CmdCut), 0, "one ^two|\nthree four\nfive", false},
		{"paste", false, "one ^two|\nthree four\nfive", c(CmdPaste), 0, "one ^two|\nthree four\nfive", false},
		{"unknown", false, at(5), c(numCommands), 0, at(5), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, span := range []int{0, 1, 3} {
				e := specEditor(t, tc.spec, tc.single)
				e.spanMin = span
				var lines Lines
				if tc.wrap > 0 {
					lines = fakeLines{e: e, w: tc.wrap, page: 2}
				}
				handled := true
				for _, cmd := range tc.cmds {
					handled = e.Do(cmd, lines)
				}
				if got := formatSpec(e); got != tc.want || handled != tc.handled {
					t.Errorf("span %d: %q → %q, %v; want %q, %v", span, tc.spec, got, handled, tc.want, tc.handled)
				}
				checkEditor(t, e)
			}
		})
	}
}

// TestDoMatchesMethods runs every command through Do and through the Editor
// method it stands for, from several selections, with and without Lines,
// and wants the same text, selection, affinity and undo state.
func TestDoMatchesMethods(t *testing.T) {
	direct := func(e *Editor, cmd Command, lines Lines) bool {
		switch cmd {
		case CmdDeleteBackward:
			e.DeleteBackward(Grapheme, lines)
		case CmdDeleteForward:
			e.DeleteForward(Grapheme, lines)
		case CmdDeleteWordBackward:
			e.DeleteBackward(Word, lines)
		case CmdDeleteWordForward:
			e.DeleteForward(Word, lines)
		case CmdDeleteToLineStart:
			e.DeleteBackward(Line, lines)
		case CmdDeleteToLineEnd:
			e.DeleteForward(Line, lines)
		case CmdInsertNewline:
			if e.SingleLine() {
				return false
			}
			e.Insert("\n")
		case CmdInsertTab:
			e.Insert("\t")
		case CmdSelectAll:
			e.SelectAll()
		case CmdUndo:
			e.Undo()
		case CmdRedo:
			e.Redo()
		case CmdNone, CmdCopy, CmdCut, CmdPaste:
			return false
		default:
			u, d, extend, ok := cmd.move()
			if !ok {
				t.Fatalf("%v: not a move", cmd)
			}
			e.Move(u, d, extend, lines)
		}
		return true
	}
	specs := []string{
		"|ab cd\nef gh ij\n\nk" + combAcute + "l",
		"ab c|d\nef gh ij\n\nkl",
		"ab cd\nef ^gh i|j\n\nkl",
		"ab cd\nef g|h ij^\n\nkl",
		"ab cd\nef gh ij\n\nkl|",
	}
	type state struct {
		text           string
		anchor, head   int
		up, undo, redo bool
	}
	st := func(e *Editor) state {
		return state{e.Text(), e.Anchor(), e.Head(), e.Upstream(), e.CanUndo(), e.CanRedo()}
	}
	for cmd := CmdNone; cmd < numCommands; cmd++ {
		for _, spec := range specs {
			for _, single := range []bool{false, true} {
				for _, wrap := range []int{0, 3} {
					a, b := specEditor(t, spec, single), specEditor(t, spec, single)
					// A history, so that undo and redo have something to do.
					for _, e := range []*Editor{a, b} {
						e.Insert("x")
						e.Undo()
						e.Insert("y")
					}
					var la, lb Lines
					if wrap > 0 {
						la, lb = fakeLines{e: a, w: wrap, page: 2}, fakeLines{e: b, w: wrap, page: 2}
					}
					ha, hb := a.Do(cmd, la), direct(b, cmd, lb)
					// A second step: the goal x and the open undo group carry over.
					ha2, hb2 := a.Do(cmd, la), direct(b, cmd, lb)
					if ha != hb || ha2 != hb2 || st(a) != st(b) {
						t.Errorf("%v on %q (single %v, wrap %d): Do %v %v %+v, methods %v %v %+v",
							cmd, spec, single, wrap, ha, ha2, st(a), hb, hb2, st(b))
					}
				}
			}
		}
	}
}

// TestLookupDo drives an editor by keys only, as a widget does.
func TestLookupDo(t *testing.T) {
	e := NewEditor("", false)
	key := func(k driver.Key, mods driver.Modifiers) {
		t.Helper()
		cmd, ok := Lookup(k, mods, true)
		if !ok {
			t.Fatalf("key %#x %#x unbound", k, mods)
		}
		if !e.Do(cmd, nil) {
			t.Fatalf("%v not handled", cmd)
		}
	}
	e.Insert("hello world")
	key(driver.KeyLeft, driver.ModOption)                 // hello |world
	key(driver.KeyLeft, driver.ModOption|driver.ModShift) // |hello ^world
	key(driver.KeyBackspace, 0)                           // |world
	key(driver.KeyReturn, 0)                              // \n|world
	key(driver.KeyUp, 0)                                  // |\nworld
	e.Insert("hi")                                        // hi|\nworld
	key(driver.KeyDown, driver.ModControl)                // hi\nworld|
	key(driver.KeyBackspace, driver.ModControl)           // hi\n|
	if got := formatSpec(e); got != "hi\n|" {
		t.Fatalf("after the keys: %q", got)
	}
	for range 4 {
		key('Z', driver.ModControl)
	}
	if got := formatSpec(e); got != "|hello ^world" {
		t.Fatalf("after four undos: %q", got)
	}
	key('Z', driver.ModControl|driver.ModShift)
	if got := formatSpec(e); got != "|world" {
		t.Fatalf("after a redo: %q", got)
	}
	key('A', driver.ModControl)
	if cmd, _ := Lookup('C', driver.ModControl, true); e.Do(cmd, nil) || !cmd.IsClipboard() || e.Copy() != "world" {
		t.Fatal("copy is not the caller's")
	}
}
