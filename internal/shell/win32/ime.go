//go:build windows

package win32

import (
	"errors"
	"math"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/shellkit"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// IME composition over IMM32 (S5.3f, S5.3g). The IME sends its composition to the
// focus window, which is the surface while it is the key window:
//
//   - WM_IME_SETCONTEXT goes on to DefWindowProcW with
//     ISC_SHOWUICOMPOSITIONWINDOW cleared: the aqua driver draws the preedit
//     in the field, so the IME shows only its candidate window.
//   - While a text input is enabled (Surface.SetTextInput), the
//     composition messages are handled here and never reach
//     DefWindowProcW, so no WM_IME_CHAR (and no WM_CHAR from it) repeats a
//     commit. WM_IME_STARTCOMPOSITION delivers nothing; the first non-empty
//     preedit starts the composition. WM_IME_COMPOSITION reads the parts
//     its lParam names through the shell's imm seam: GCS_RESULTSTR is a
//     TextEvent (the commit), then GCS_COMPSTR, with GCS_COMPATTR and
//     GCS_CURSORPOS, a PreeditEvent; both in one message come in that
//     order, as shell.PreeditEvent allows. An empty composition string, or
//     an lParam without parts (some IMEs clear a cancelled composition so),
//     ends a running composition with an empty PreeditEvent.
//     WM_IME_ENDCOMPOSITION ends it too, then goes on to DefWindowProcW.
//   - While the text input is off, the window has no input context (from
//     NewSurface on), so the IME is off for it and sends none of these;
//     any that still come go to DefWindowProcW.
//   - SetTextInput associates the thread's default context and places the
//     IME's windows by the caret (imeForms), again on a caret change,
//     WM_DPICHANGED and WM_IME_STARTCOMPOSITION; CancelComposition is
//     CPS_CANCEL. The messages these IMM32 calls cause are swallowed
//     without events (muted).
//
// The key-down path is unchanged: VK_PROCESSKEY (a key the IME took) is not
// delivered, and the consumed / dropPendingChars rules of keyboard.go stay
// as they are, since WM_IME_* are not in the WM_CHAR range they peek.

// ime is the text input state of a surface.
type ime struct {
	input     shell.TextInput // the state set last
	composing bool            // a non-empty PreeditEvent was the last composition event
	mute      int             // inside muted: the composition messages are swallowed
	placed    imePlacement    // the last placeIME that reached the context
}

// imeComposition holds the parts of a composition read from the input
// context: UTF-16 strings, the ATTR_* byte of each comp unit, and the
// cursor in comp units (-1 when not read).
type imeComposition struct {
	result, comp []uint16
	attr         []byte
	cursor       int
}

// liveIMM is the shell's imm seam: it reads the parts of h's composition
// that parts (GCS_* flags) names. A part that cannot be read is empty.
func liveIMM(h windows.HWND, parts uint32) imeComposition {
	c := imeComposition{cursor: -1}
	himc := w32.ImmGetContext(h)
	if himc == 0 {
		return c
	}
	defer func() { _ = w32.ImmReleaseContext(h, himc) }()
	if parts&w32.GCS_RESULTSTR != 0 {
		c.result, _ = w32.ImmGetCompositionString(himc, w32.GCS_RESULTSTR)
	}
	if parts&w32.GCS_COMPSTR != 0 {
		c.comp, _ = w32.ImmGetCompositionString(himc, w32.GCS_COMPSTR)
	}
	if parts&w32.GCS_COMPATTR != 0 {
		c.attr, _ = w32.ImmGetCompositionAttr(himc, w32.GCS_COMPATTR)
	}
	if parts&w32.GCS_CURSORPOS != 0 {
		if n, err := w32.ImmGetCompositionCursorPos(himc); err == nil {
			c.cursor = n
		}
	}
	return c
}

// imeText converts IME UTF-16 units to UTF-8 text without control
// characters or lone surrogates (both dropped). offs has len(u)+1 entries:
// the byte offset in text of each unit index, a rune start (the low half of
// a pair and a dropped unit map to where their rune would be).
func imeText(u []uint16) (text string, offs []int) {
	buf := make([]byte, 0, len(u)*3)
	offs = make([]int, len(u)+1)
	for i := 0; i < len(u); i++ {
		offs[i] = len(buf)
		r := rune(u[i])
		if utf16.IsSurrogate(r) {
			if r >= 0xDC00 || i+1 >= len(u) || u[i+1] < 0xDC00 || u[i+1] > 0xDFFF {
				continue // a lone surrogate
			}
			r = utf16.DecodeRune(r, rune(u[i+1]))
			i++
			offs[i] = offs[i-1]
		} else if r < 0x20 || r == 0x7F || (r >= 0x80 && r < 0xA0) {
			continue
		}
		buf = utf8.AppendRune(buf, r)
	}
	offs[len(u)] = len(buf)
	return string(buf), offs
}

// selectedAttr reports whether an ATTR_* byte marks the clause being
// converted.
func selectedAttr(a byte) bool {
	return a == w32.ATTR_TARGET_CONVERTED || a == w32.ATTR_TARGET_NOTCONVERTED
}

// preeditOf builds the PreeditEvent (without Time) of a composition
// string, its attribute bytes and its cursor in units (-1, or out of
// range: the end). The segments are the runs of equal attributes, a run
// with an ATTR_TARGET_* attribute selected; units past the attributes have
// no segment, and no attributes give no segments. Runs that hold only
// dropped units are left out.
func preeditOf(comp []uint16, attr []byte, cursor int) shell.PreeditEvent {
	text, offs := imeText(comp)
	if text == "" {
		return shell.PreeditEvent{}
	}
	if cursor < 0 || cursor > len(comp) {
		cursor = len(comp)
	}
	ev := shell.PreeditEvent{Text: text, Cursor: offs[cursor]}
	n := min(len(attr), len(comp))
	for i := 0; i < n; {
		j := i + 1
		for j < n && attr[j] == attr[i] {
			j++
		}
		if start, end := offs[i], offs[j]; start < end {
			ev.Segments = append(ev.Segments, shell.PreeditSegment{Start: start, End: end,
				Selected: selectedAttr(attr[i])})
		}
		i = j
	}
	return ev
}

// imeMsg handles the IME messages. handled false sends the message on to
// DefWindowProcW.
func (sf *surface) imeMsg(h windows.HWND, msg uint32, wp, lp uintptr) (uintptr, bool) {
	if msg == w32.WM_IME_SETCONTEXT {
		return w32.DefWindowProc(h, msg, wp, lp&^uintptr(w32.ISC_SHOWUICOMPOSITIONWINDOW)), true
	}
	if sf.ime.mute > 0 {
		return 0, true // caused by the shell's own IMM32 call: dropped
	}
	if sf.dead || !sf.ime.input.Enabled {
		return 0, false
	}
	switch msg {
	case w32.WM_IME_STARTCOMPOSITION:
		_ = sf.placeIME() // the caret may have moved since the last SetTextInput
		return 0, true
	case w32.WM_IME_ENDCOMPOSITION:
		sf.endPreedit()
		return 0, false
	}
	sf.composition(uint32(lp))
	return 0, true
}

// composition handles a WM_IME_COMPOSITION whose lParam names parts.
func (sf *surface) composition(parts uint32) {
	if parts&(w32.GCS_RESULTSTR|w32.GCS_COMPSTR) == 0 {
		sf.endPreedit()
		return
	}
	c := sf.s.imm(sf.hwnd, parts)
	if parts&w32.GCS_RESULTSTR != 0 {
		if t, _ := imeText(c.result); t != "" {
			sf.ime.composing = false // the commit empties the preedit
			sf.keySink().Event(shell.TextEvent{Text: t, Time: sf.s.eventTime()})
			if sf.dead || !sf.ime.input.Enabled {
				return
			}
		}
	}
	if parts&w32.GCS_COMPSTR == 0 {
		return
	}
	ev := preeditOf(c.comp, c.attr, c.cursor)
	if ev.Text == "" {
		sf.endPreedit()
		return
	}
	sf.ime.composing = true
	ev.Time = sf.s.eventTime()
	sf.keySink().Event(ev)
}

// endPreedit ends a running composition with an empty PreeditEvent.
func (sf *surface) endPreedit() {
	if !sf.ime.composing {
		return
	}
	sf.ime.composing = false
	sf.keySink().Event(shell.PreeditEvent{Time: sf.s.eventTime()})
}

// SetTextInput implements shell.Surface. Turning the input on associates
// the thread's default input context with the window, so the IME is on for
// it, and places the IME's windows by the caret; turning it off cancels any
// composition and leaves the window without a context (ImmGetContext is
// then 0), so the IME sends nothing. A caret change while on places the
// windows again. The IMM32 calls run muted: a message they cause delivers
// no event, so the call is safe during Paint. Errors are ignored (the IME
// keeps its own placement).
func (sf *surface) SetTextInput(ti shell.TextInput) {
	if !sf.live("SetTextInput") {
		return
	}
	was := sf.ime.input
	sf.ime.input = ti
	switch {
	case ti.Enabled && !was.Enabled:
		sf.muted(func() { _ = w32.ImmAssociateContextEx(sf.hwnd, 0, w32.IACE_DEFAULT) })
		_ = sf.placeIME()
	case ti.Enabled:
		if ti.Caret != was.Caret {
			_ = sf.placeIME()
		}
	case was.Enabled:
		sf.ime.composing = false
		sf.cancelIME()
		sf.disassociateIME()
	}
}

// CancelComposition implements shell.Surface: CPS_CANCEL on the window's
// context, muted, so neither the commit some IMEs make while they reset
// nor the end of the composition is delivered.
func (sf *surface) CancelComposition() {
	if sf.change("CancelComposition") {
		sf.ime.composing = false
		sf.cancelIME()
	}
}

// disassociateIME leaves the window without an input context: the initial
// state of a surface (NewSurface), and the text input off.
func (sf *surface) disassociateIME() {
	sf.muted(func() { _ = w32.ImmAssociateContextEx(sf.hwnd, 0, 0) })
}

// cancelIME drops the composition of the window's context, if it has one.
// ImmNotifyIME fails when no IME is open, which is fine.
func (sf *surface) cancelIME() {
	himc := w32.ImmGetContext(sf.hwnd)
	if himc == 0 {
		return
	}
	sf.muted(func() { _ = w32.ImmNotifyIME(himc, w32.NI_COMPOSITIONSTR, w32.CPS_CANCEL, 0) })
	_ = w32.ImmReleaseContext(sf.hwnd, himc)
}

// muted runs fn with the composition messages swallowed (imeMsg): they
// deliver no event and do not reach DefWindowProcW, whose WM_IME_CHAR
// would type a commit later.
func (sf *surface) muted(fn func()) {
	sf.ime.mute++
	defer func() { sf.ime.mute-- }()
	fn()
}

// langChinese is LANG_CHINESE, the primary language in an HKL's low word.
const langChinese = 0x04

// imeForms returns the composition and candidate window forms for a caret
// in surface points at px pixels per point, in client pixels (the client
// area is the drawable area). The composition window sits at the caret's
// top-left (hidden, but some IMEs place their windows from it); candidate
// list 0 keeps clear of the caret and 1 px below it (CFS_EXCLUDE, read by
// the Japanese and Korean IMEs). Chinese IMEs read only CFS_CANDIDATEPOS,
// so for them (chinese) the candidate form is that, under the caret.
func imeForms(caret driver.Rect, px float64, chinese bool) (w32.CompositionForm, w32.CandidateForm) {
	x, y := imePx(caret.X, px), imePx(caret.Y, px)
	r := max(imePx(caret.X+caret.W, px), x)
	b := max(imePx(caret.Y+caret.H, px)+1, y)
	comp := w32.CompositionForm{DwStyle: w32.CFS_POINT, PtCurrentPos: w32.Point{X: x, Y: y}}
	if chinese {
		return comp, w32.CandidateForm{DwStyle: w32.CFS_CANDIDATEPOS, PtCurrentPos: w32.Point{X: x, Y: b}}
	}
	return comp, w32.CandidateForm{DwStyle: w32.CFS_EXCLUDE, PtCurrentPos: w32.Point{X: x, Y: y},
		RcArea: w32.Rect{Left: x, Top: y, Right: r, Bottom: b}}
}

// imePx converts a coordinate in points to pixels, rounded half away from
// zero, within ±shellkit.MaxPx (a caret may sit left of or above the
// drawable area); NaN gives 0.
func imePx(v, px float64) int32 {
	p := math.Round(v * px)
	if math.IsNaN(p) {
		return 0
	}
	return int32(max(-shellkit.MaxPx, min(p, shellkit.MaxPx)))
}

// placeIME sets the composition and candidate window positions of the
// window's context from the caret at the window's DPI. It runs when the
// text input goes on, when its caret moves, on WM_DPICHANGED and when the
// IME starts a composition, and returns the first error: errNoIMEContext
// when the window has no context (IME disabled for the thread).
func (sf *surface) placeIME() error {
	if sf.dead || !sf.ime.input.Enabled {
		return nil
	}
	himc := w32.ImmGetContext(sf.hwnd)
	if himc == 0 {
		return errNoIMEContext
	}
	defer func() { _ = w32.ImmReleaseContext(sf.hwnd, himc) }()
	chinese := uint32(w32.GetKeyboardLayout(0))&0x3FF == langChinese
	comp, cand := imeForms(sf.ime.input.Caret, sf.px(), chinese)
	var err error
	sf.muted(func() {
		err = w32.ImmSetCompositionWindow(himc, &comp)
		if e := w32.ImmSetCandidateWindow(himc, &cand); err == nil {
			err = e
		}
	})
	sf.ime.placed = imePlacement{n: sf.ime.placed.n + 1, comp: comp, cand: cand, err: err}
	return err
}

// imePlacement records a placeIME (for the tests): the count so far, the
// forms set and the error.
type imePlacement struct {
	n    int
	comp w32.CompositionForm
	cand w32.CandidateForm
	err  error
}

// errNoIMEContext is placeIME's error for a window without a context.
var errNoIMEContext = errors.New("win32: the window has no input context")
