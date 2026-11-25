//go:build windows

package win32

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/shellkit"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// ---- pure functions ----

func units(s string) []uint16 { return utf16.Encode([]rune(s)) }

func TestIMEText(t *testing.T) {
	for _, c := range []struct {
		name string
		u    []uint16
		text string
		offs []int
	}{
		{"empty", nil, "", []int{0}},
		{"ascii", units("ab"), "ab", []int{0, 1, 2}},
		{"kana", units("かな"), "かな", []int{0, 3, 6}},
		{"pair", units("a😀b"), "a😀b", []int{0, 1, 1, 5, 6}},
		{"lone high", []uint16{'a', 0xD83D, 'b'}, "ab", []int{0, 1, 1, 2}},
		{"lone low", []uint16{0xDE00, 'b'}, "b", []int{0, 0, 1}},
		{"high at end", []uint16{'a', 0xD83D}, "a", []int{0, 1, 1}},
		{"two highs", []uint16{0xD83D, 0xD83D, 0xDE00}, "😀", []int{0, 0, 0, 4}},
		{"controls", []uint16{'a', 0x0D, 0x7F, 0x85, 'b'}, "ab", []int{0, 1, 1, 1, 1, 2}},
	} {
		text, offs := imeText(c.u)
		if text != c.text || !slices.Equal(offs, c.offs) {
			t.Errorf("%s: imeText = %q %v, want %q %v", c.name, text, offs, c.text, c.offs)
		}
	}
}

func TestPreeditOf(t *testing.T) {
	seg := func(s, e int, sel bool) shell.PreeditSegment {
		return shell.PreeditSegment{Start: s, End: e, Selected: sel}
	}
	const (
		in  = w32.ATTR_INPUT
		tc  = w32.ATTR_TARGET_CONVERTED
		cv  = w32.ATTR_CONVERTED
		tnc = w32.ATTR_TARGET_NOTCONVERTED
		fx  = w32.ATTR_FIXEDCONVERTED
	)
	for _, c := range []struct {
		name   string
		comp   []uint16
		attr   []byte
		cursor int
		want   shell.PreeditEvent
	}{
		{"empty", nil, nil, 0, shell.PreeditEvent{}},
		{"only controls", []uint16{0x0D}, []byte{in}, 1, shell.PreeditEvent{}},
		{"input", units("かな"), []byte{in, in}, 2,
			shell.PreeditEvent{Text: "かな", Cursor: 6, Segments: []shell.PreeditSegment{seg(0, 6, false)}}},
		{"no attributes", units("かな"), nil, 1, shell.PreeditEvent{Text: "かな", Cursor: 3}},
		{"no cursor", units("かな"), nil, -1, shell.PreeditEvent{Text: "かな", Cursor: 6}},
		{"cursor past the end", units("かな"), nil, 9, shell.PreeditEvent{Text: "かな", Cursor: 6}},
		{"clauses", units("漢字変換"), []byte{tc, tc, cv, cv}, 2,
			shell.PreeditEvent{Text: "漢字変換", Cursor: 6,
				Segments: []shell.PreeditSegment{seg(0, 6, true), seg(6, 12, false)}}},
		{"target not converted", units("abc"), []byte{fx, tnc, in}, 0,
			shell.PreeditEvent{Text: "abc", Cursor: 0,
				Segments: []shell.PreeditSegment{seg(0, 1, false), seg(1, 2, true), seg(2, 3, false)}}},
		{"short attributes", units("abc"), []byte{tc}, 3,
			shell.PreeditEvent{Text: "abc", Cursor: 3, Segments: []shell.PreeditSegment{seg(0, 1, true)}}},
		{"long attributes", units("ab"), []byte{in, tc, cv, cv}, 1,
			shell.PreeditEvent{Text: "ab", Cursor: 1,
				Segments: []shell.PreeditSegment{seg(0, 1, false), seg(1, 2, true)}}},
		{"surrogates", units("a😀😀b"), []byte{in, tc, tc, cv, cv, in}, 3,
			shell.PreeditEvent{Text: "a😀😀b", Cursor: 5,
				Segments: []shell.PreeditSegment{seg(0, 1, false), seg(1, 5, true), seg(5, 9, false), seg(9, 10, false)}}},
		{"cursor inside a pair", units("😀"), nil, 1, shell.PreeditEvent{Text: "😀", Cursor: 0}},
		{"attribute split inside a pair", units("😀a"), []byte{tc, cv, cv}, 0,
			shell.PreeditEvent{Text: "😀a", Cursor: 0, Segments: []shell.PreeditSegment{seg(0, 5, false)}}},
		{"dropped run", []uint16{'a', 0xD800, 'b'}, []byte{in, tc, in}, 2,
			shell.PreeditEvent{Text: "ab", Cursor: 1,
				Segments: []shell.PreeditSegment{seg(0, 1, false), seg(1, 2, false)}}},
	} {
		got := preeditOf(c.comp, c.attr, c.cursor)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: preeditOf = %+v, want %+v", c.name, got, c.want)
		}
		checkPreeditRules(t, c.name, got)
	}
}

// checkPreeditRules checks a PreeditEvent against shell.PreeditEvent's
// rules.
func checkPreeditRules(t *testing.T, name string, ev shell.PreeditEvent) {
	t.Helper()
	onRune := func(i int) bool {
		return i >= 0 && i <= len(ev.Text) && (i == len(ev.Text) || (ev.Text[i]&0xC0) != 0x80)
	}
	if ev.Text == "" && (ev.Cursor != 0 || ev.Segments != nil) {
		t.Errorf("%s: an empty preedit with a cursor or segments: %+v", name, ev)
	}
	if !onRune(ev.Cursor) {
		t.Errorf("%s: cursor %d not on a rune start", name, ev.Cursor)
	}
	end := 0
	for _, s := range ev.Segments {
		if s.Start < end || s.Start >= s.End || !onRune(s.Start) || !onRune(s.End) {
			t.Errorf("%s: bad segment %+v in %+v", name, s, ev)
		}
		end = s.End
	}
}

// ---- real windows ----

// fakeIMM is a composition the test sets, read through the shell's imm
// seam.
type fakeIMM struct {
	c     imeComposition
	reads []uint32 // the parts asked for
}

func (f *fakeIMM) read(_ windows.HWND, parts uint32) imeComposition {
	f.reads = append(f.reads, parts)
	return f.c
}

// describeIME renders the recorded text and preedit events, compactly.
func describeIME(t *testing.T, r *recorder) string {
	t.Helper()
	var out []string
	for _, ev := range r.events {
		switch e := ev.(type) {
		case shell.TextEvent:
			out = append(out, fmt.Sprintf("text %q", e.Text))
		case shell.PreeditEvent:
			if e.Text == "" {
				out = append(out, "end")
				continue
			}
			s := fmt.Sprintf("preedit %q @%d", e.Text, e.Cursor)
			for _, g := range e.Segments {
				m := ""
				if g.Selected {
					m = "*"
				}
				s += fmt.Sprintf(" [%d,%d)%s", g.Start, g.End, m)
			}
			out = append(out, s)
			checkPreeditRules(t, s, e)
		default:
			out = append(out, fmt.Sprintf("%T", ev))
		}
	}
	r.events = nil
	return strings.Join(out, "; ")
}

const compParts = w32.GCS_COMPSTR | w32.GCS_COMPATTR | w32.GCS_CURSORPOS

// TestSurfaceComposition sends the IME messages to a real window, with the
// composition strings from a faked imm seam: start, changes with clauses,
// a commit with and without a new composition in the same message, the
// end, the empty and part-less compositions, and the rules around them
// (no events while the text input is off, none after a cancel, the
// message results that keep DefWindowProcW and so WM_IME_CHAR away).
func TestSurfaceComposition(t *testing.T) {
	inShell(t, func(s *Shell) {
		f := &fakeIMM{}
		s.imm = f.read
		r := &recorder{}
		sf := newSurface(t, s, resizable, r)
		defer sf.Close()
		send := func(msg uint32, lp uintptr) {
			w32.SendMessage(sf.hwnd, msg, 0, lp)
			rethrow()
		}
		expect := func(step, want string) {
			t.Helper()
			if got := describeIME(t, r); got != want {
				t.Errorf("%s: got %s, want %s", step, got, want)
			}
		}
		comp := func(s string, attr []byte, cursor int) {
			f.c = imeComposition{comp: units(s), attr: attr, cursor: cursor}
			send(w32.WM_IME_COMPOSITION, compParts)
		}

		// Off: nothing delivered, nothing read, DefWindowProcW gets them.
		f.c = imeComposition{comp: units("か"), cursor: 1}
		for _, m := range []uint32{w32.WM_IME_STARTCOMPOSITION, w32.WM_IME_COMPOSITION, w32.WM_IME_ENDCOMPOSITION} {
			if _, handled := sf.handle(sf.hwnd, m, 0, compParts); handled {
				t.Errorf("message %#x handled while the text input is off", m)
			}
		}
		expect("off", "")
		if len(f.reads) != 0 {
			t.Errorf("read the composition while off: %v", f.reads)
		}

		sf.SetTextInput(shell.TextInput{Enabled: true, Caret: driver.Rect{H: 16}})
		if _, handled := sf.handle(sf.hwnd, w32.WM_IME_STARTCOMPOSITION, 0, 0); !handled {
			t.Error("WM_IME_STARTCOMPOSITION passed on")
		}
		if _, handled := sf.handle(sf.hwnd, w32.WM_IME_ENDCOMPOSITION, 0, 0); handled {
			t.Error("WM_IME_ENDCOMPOSITION not passed on")
		}
		expect("start", "")

		// Typing kana, then converting into two clauses.
		comp("k", []byte{w32.ATTR_INPUT}, 1)
		comp("か", []byte{w32.ATTR_INPUT}, 1)
		comp("かんじ", []byte{0, 0, 0}, 3)
		comp("漢字を", []byte{w32.ATTR_TARGET_CONVERTED, w32.ATTR_TARGET_CONVERTED, w32.ATTR_CONVERTED}, 2)
		expect("compose", `preedit "k" @1 [0,1); preedit "か" @3 [0,3); preedit "かんじ" @9 [0,9); `+
			`preedit "漢字を" @6 [0,6)* [6,9)`)
		if got := f.reads[len(f.reads)-1]; got != compParts {
			t.Errorf("read parts %#x, want %#x", got, compParts)
		}

		// A commit and the next composition in one message: commit first.
		f.c = imeComposition{result: units("漢字を"), comp: units("😀"), attr: []byte{0, 0}, cursor: 2}
		if _, handled := sf.handle(sf.hwnd, w32.WM_IME_COMPOSITION, 0, w32.GCS_RESULTSTR|compParts); !handled {
			t.Error("WM_IME_COMPOSITION passed on (WM_IME_CHAR would repeat the commit)")
		}
		expect("commit and compose", `text "漢字を"; preedit "😀" @4 [0,4)`)

		// A commit alone, then the end: no extra empty preedit.
		f.c = imeComposition{result: units("😀")}
		send(w32.WM_IME_COMPOSITION, w32.GCS_RESULTSTR|w32.GCS_RESULTREADSTR)
		send(w32.WM_IME_ENDCOMPOSITION, 0)
		expect("commit", `text "😀"`)

		// Ends: an empty composition string, a message without parts,
		// WM_IME_ENDCOMPOSITION; each once, only while composing.
		comp("a", nil, -1)
		comp("", nil, 0)
		comp("", nil, 0)
		comp("b", nil, 1)
		send(w32.WM_IME_COMPOSITION, 0)
		send(w32.WM_IME_COMPOSITION, 0)
		comp("c", nil, 1)
		send(w32.WM_IME_ENDCOMPOSITION, 0)
		send(w32.WM_IME_ENDCOMPOSITION, 0)
		expect("ends", `preedit "a" @1; end; preedit "b" @1; end; preedit "c" @1; end`)

		// A result of control characters only is no commit; a composition
		// of them ends the preedit.
		comp("d", nil, 1)
		f.c = imeComposition{result: []uint16{0x0D}, comp: []uint16{0x1B}}
		send(w32.WM_IME_COMPOSITION, w32.GCS_RESULTSTR|compParts)
		expect("controls", `preedit "d" @1; end`)

		// CancelComposition and turning the input off drop the
		// composition without events.
		comp("e", nil, 1)
		sf.CancelComposition()
		send(w32.WM_IME_ENDCOMPOSITION, 0)
		comp("f", nil, 1)
		sf.SetTextInput(shell.TextInput{})
		sf.SetTextInput(shell.TextInput{Enabled: true})
		send(w32.WM_IME_ENDCOMPOSITION, 0)
		expect("cancel", `preedit "e" @1; preedit "f" @1`)

		// WM_IME_SETCONTEXT goes on to DefWindowProcW itself, handled.
		if _, handled := sf.handle(sf.hwnd, w32.WM_IME_SETCONTEXT, 1, uintptr(w32.ISC_SHOWUIALL)); !handled {
			t.Error("WM_IME_SETCONTEXT not handled")
		}
		expect("set context", "")
	})
}

// TestSurfaceCompositionHandlerReacts: a handler that closes the surface or
// turns the text input off during a commit gets no preedit from the same
// message.
func TestSurfaceCompositionHandlerReacts(t *testing.T) {
	for _, c := range []struct {
		name  string
		react func(sf *surface)
		want  string
	}{
		{"nothing", func(*surface) {}, `text "a"; preedit "b" @1`},
		{"off", func(sf *surface) { sf.SetTextInput(shell.TextInput{}) }, `text "a"`},
		{"close", func(sf *surface) { sf.Close() }, `text "a"; closed`},
	} {
		inShell(t, func(s *Shell) {
			s.imm = (&fakeIMM{c: imeComposition{result: units("a"), comp: units("b"), cursor: 1}}).read
			r := &recorder{}
			sf := newSurface(t, s, resizable, r)
			defer sf.Close()
			sf.SetTextInput(shell.TextInput{Enabled: true})
			var out []string
			sf.h = handlerFunc(func(ev shell.Event) {
				switch e := ev.(type) {
				case shell.TextEvent:
					out = append(out, fmt.Sprintf("text %q", e.Text))
					c.react(sf)
					if sf.dead {
						out = append(out, "closed")
					}
				case shell.PreeditEvent:
					out = append(out, fmt.Sprintf("preedit %q @%d", e.Text, e.Cursor))
				}
			}, r)
			w32.SendMessage(sf.hwnd, w32.WM_IME_COMPOSITION, 0, w32.GCS_RESULTSTR|compParts)
			rethrow()
			if got := strings.Join(out, "; "); got != c.want {
				t.Errorf("%s: got %s, want %s", c.name, got, c.want)
			}
			if c.name == "off" && sf.ime.composing {
				t.Errorf("%s: composing after the input went off", c.name)
			}
		})
	}
}

// eventFunc is a recorder whose Event calls fn.
type eventFunc struct {
	*recorder
	fn func(ev shell.Event)
}

func (e eventFunc) Event(ev shell.Event) bool { e.fn(ev); return false }

func handlerFunc(fn func(ev shell.Event), r *recorder) shell.SurfaceHandler {
	return eventFunc{recorder: r, fn: fn}
}

// TestLiveIMM reads the composition of a real window through the live
// seam: no IME composes on the host, so every part is empty and the
// cursor unread.
func TestLiveIMM(t *testing.T) {
	inShell(t, func(s *Shell) {
		sf := newSurface(t, s, resizable, &recorder{})
		defer sf.Close()
		got := liveIMM(sf.hwnd, w32.GCS_RESULTSTR|compParts)
		if got.result != nil || got.comp != nil || got.attr != nil {
			t.Errorf("liveIMM = %+v, want empty parts", got)
		}
		if got := liveIMM(0, compParts); got.comp != nil || got.cursor != -1 {
			t.Errorf("liveIMM without a window = %+v", got)
		}
	})
}

// ---- the context and the IME's windows (S5.3g) ----

func TestIMEForms(t *testing.T) {
	pt := func(x, y int32) w32.Point { return w32.Point{X: x, Y: y} }
	rc := func(l, t, r, b int32) w32.Rect { return w32.Rect{Left: l, Top: t, Right: r, Bottom: b} }
	for _, c := range []struct {
		name    string
		caret   driver.Rect
		px      float64
		chinese bool
		comp    w32.CompositionForm
		cand    w32.CandidateForm
	}{
		{"100 %", driver.Rect{X: 10, Y: 20, W: 1, H: 16}, 1, false,
			w32.CompositionForm{DwStyle: w32.CFS_POINT, PtCurrentPos: pt(10, 20)},
			w32.CandidateForm{DwStyle: w32.CFS_EXCLUDE, PtCurrentPos: pt(10, 20), RcArea: rc(10, 20, 11, 37)}},
		{"150 %", driver.Rect{X: 10, Y: 20.5, W: 1, H: 16}, 1.5, false,
			w32.CompositionForm{DwStyle: w32.CFS_POINT, PtCurrentPos: pt(15, 31)},
			w32.CandidateForm{DwStyle: w32.CFS_EXCLUDE, PtCurrentPos: pt(15, 31), RcArea: rc(15, 31, 17, 56)}},
		{"200 %, chinese", driver.Rect{X: 3, Y: 4, W: 1, H: 16}, 2, true,
			w32.CompositionForm{DwStyle: w32.CFS_POINT, PtCurrentPos: pt(6, 8)},
			w32.CandidateForm{DwStyle: w32.CFS_CANDIDATEPOS, PtCurrentPos: pt(6, 41)}},
		{"left of and above the area", driver.Rect{X: -2, Y: -20, W: 1, H: 16}, 1, false,
			w32.CompositionForm{DwStyle: w32.CFS_POINT, PtCurrentPos: pt(-2, -20)},
			w32.CandidateForm{DwStyle: w32.CFS_EXCLUDE, PtCurrentPos: pt(-2, -20), RcArea: rc(-2, -20, -1, -3)}},
		{"empty caret", driver.Rect{X: 5, Y: 5}, 1, false,
			w32.CompositionForm{DwStyle: w32.CFS_POINT, PtCurrentPos: pt(5, 5)},
			w32.CandidateForm{DwStyle: w32.CFS_EXCLUDE, PtCurrentPos: pt(5, 5), RcArea: rc(5, 5, 5, 6)}},
		{"negative size", driver.Rect{X: 5, Y: 5, W: -3, H: -3}, 1, false,
			w32.CompositionForm{DwStyle: w32.CFS_POINT, PtCurrentPos: pt(5, 5)},
			w32.CandidateForm{DwStyle: w32.CFS_EXCLUDE, PtCurrentPos: pt(5, 5), RcArea: rc(5, 5, 5, 5)}},
		{"huge and NaN", driver.Rect{X: 1e12, Y: math.NaN(), W: 1, H: 16}, 1, false,
			w32.CompositionForm{DwStyle: w32.CFS_POINT, PtCurrentPos: pt(shellkit.MaxPx, 0)},
			w32.CandidateForm{DwStyle: w32.CFS_EXCLUDE, PtCurrentPos: pt(shellkit.MaxPx, 0),
				RcArea: rc(shellkit.MaxPx, 0, shellkit.MaxPx, 1)}},
	} {
		comp, cand := imeForms(c.caret, c.px, c.chinese)
		if comp != c.comp || cand != c.cand {
			t.Errorf("%s: imeForms = %+v %+v, want %+v %+v", c.name, comp, cand, c.comp, c.cand)
		}
	}
}

// hasContext reports whether a window has an input context.
func hasContext(h windows.HWND) bool {
	himc := w32.ImmGetContext(h)
	if himc == 0 {
		return false
	}
	_ = w32.ImmReleaseContext(h, himc)
	return true
}

// TestSurfaceTextInputContext: a new surface has no input context; turning
// the text input on associates one and places the IME's windows without
// error, at 100 % and after a WM_DPICHANGED to 144 DPI, again on a caret
// change and a composition start only; turning it off leaves none, also
// after a cancel.
func TestSurfaceTextInputContext(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{}
		sf := newSurface(t, s, resizable, r)
		defer sf.Close()
		if hasContext(sf.hwnd) {
			t.Fatal("a new surface has an input context; want none (the text input is off)")
		}
		setDPI(t, sf, 96)
		placed := func(step string, n int, x, y, bottom int32) {
			t.Helper()
			p := sf.ime.placed
			if p.n != n || p.err != nil {
				t.Errorf("%s: placed %d times (err %v), want %d without error", step, p.n, p.err, n)
			}
			if p.comp.PtCurrentPos != (w32.Point{X: x, Y: y}) {
				t.Errorf("%s: composition window at %+v, want (%d, %d)", step, p.comp.PtCurrentPos, x, y)
			}
			if b := max(p.cand.RcArea.Bottom, p.cand.PtCurrentPos.Y); p.cand.DwStyle == w32.CFS_EXCLUDE && b != bottom ||
				p.cand.DwStyle == w32.CFS_CANDIDATEPOS && p.cand.PtCurrentPos.Y != bottom {
				t.Errorf("%s: candidate form %+v, want its bottom at %d", step, p.cand, bottom)
			}
		}

		caret := driver.Rect{X: 10, Y: 20, W: 1, H: 16}
		sf.SetTextInput(shell.TextInput{Enabled: true, Caret: caret})
		if !hasContext(sf.hwnd) {
			t.Fatal("no input context after SetTextInput(Enabled)")
		}
		placed("on", 1, 10, 20, 37)
		sf.SetTextInput(shell.TextInput{Enabled: true, Caret: caret})
		placed("same caret", 1, 10, 20, 37)
		caret.X = 30
		sf.SetTextInput(shell.TextInput{Enabled: true, Caret: caret})
		placed("caret moved", 2, 30, 20, 37)

		setDPI(t, sf, 144)
		placed("144 DPI", 3, 45, 30, 55)
		w32.SendMessage(sf.hwnd, w32.WM_IME_STARTCOMPOSITION, 0, 0)
		rethrow()
		placed("composition start", 4, 45, 30, 55)

		sf.CancelComposition()
		if !hasContext(sf.hwnd) {
			t.Error("CancelComposition dropped the input context")
		}
		sf.SetTextInput(shell.TextInput{Caret: caret})
		if hasContext(sf.hwnd) {
			t.Error("an input context after SetTextInput(off)")
		}
		sf.SetTextInput(shell.TextInput{Caret: driver.Rect{X: 1}})
		setDPI(t, sf, 96)
		w32.SendMessage(sf.hwnd, w32.WM_IME_STARTCOMPOSITION, 0, 0)
		rethrow()
		if sf.ime.placed.n != 4 {
			t.Errorf("placed %d times while off, want 4", sf.ime.placed.n)
		}
		sf.SetTextInput(shell.TextInput{Enabled: true, Caret: caret})
		if !hasContext(sf.hwnd) {
			t.Error("no input context after turning the text input on again")
		}
		placed("on again", 5, 30, 20, 37)
		if got := describeIME(t, r); got != "" {
			t.Errorf("events from the IMM32 calls: %s", got)
		}
	})
}

// TestSurfaceIMEMuted: the composition messages an IMM32 call of the shell
// causes (as an IME's commit while it is cancelled) are swallowed: no
// event, and not passed on to DefWindowProcW.
func TestSurfaceIMEMuted(t *testing.T) {
	inShell(t, func(s *Shell) {
		f := &fakeIMM{}
		s.imm = f.read
		r := &recorder{}
		sf := newSurface(t, s, resizable, r)
		defer sf.Close()
		sf.SetTextInput(shell.TextInput{Enabled: true})
		f.c = imeComposition{comp: units("か"), cursor: 1}
		w32.SendMessage(sf.hwnd, w32.WM_IME_COMPOSITION, 0, compParts)
		rethrow()
		if got := describeIME(t, r); got != `preedit "か" @3` {
			t.Fatalf("compose: %s", got)
		}
		f.c = imeComposition{result: units("か")}
		sf.muted(func() {
			for _, m := range []uint32{w32.WM_IME_STARTCOMPOSITION, w32.WM_IME_COMPOSITION, w32.WM_IME_ENDCOMPOSITION} {
				if _, handled := sf.handle(sf.hwnd, m, 0, w32.GCS_RESULTSTR); !handled {
					t.Errorf("message %#x passed on while muted", m)
				}
			}
		})
		if got := describeIME(t, r); got != "" || len(f.reads) != 1 {
			t.Errorf("muted: events %q, reads %v", got, f.reads)
		}
		if _, handled := sf.handle(sf.hwnd, w32.WM_IME_ENDCOMPOSITION, 0, 0); handled {
			t.Error("WM_IME_ENDCOMPOSITION not passed on after muted")
		}
		if got := describeIME(t, r); got != "end" {
			t.Errorf("after muted: %s, want end", got)
		}
	})
}
