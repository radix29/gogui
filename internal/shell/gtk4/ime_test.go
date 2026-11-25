package gtk4

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// fakePreedit is what the fake input method's preedit reads.
type fakePreedit struct {
	text   string
	cursor int
	attrs  []preeditAttr
}

// preedit logs the read in ims and returns fw.pre.
func (fw *fakeWindow) preedit() (string, int, []preeditAttr) {
	fw.use("preedit")
	fw.ims = append(fw.ims, "preedit")
	return fw.pre.text, fw.pre.cursor, fw.pre.attrs
}

// preeditSignal emits a preedit signal through the real sigVoid
// trampoline, without rethrow (imPreedits runs it inside a key event).
func (fw *fakeWindow) preeditSignal(k kind) { sigVoid(testIM, uintptr(fw.sig[k])) }

func (fw *fakeWindow) preeditStart() { fw.preeditSignal(kindIMPreeditStart); rethrow() }

// preeditChange sets the preedit and emits "preedit-changed".
func (fw *fakeWindow) preeditChange(text string, cursor int, attrs ...preeditAttr) {
	fw.pre = fakePreedit{text, cursor, attrs}
	fw.preeditSignal(kindIMPreeditChanged)
	rethrow()
}

// preeditEnd empties the preedit and emits "preedit-end".
func (fw *fakeWindow) preeditEnd() {
	fw.pre = fakePreedit{}
	fw.preeditSignal(kindIMPreeditEnd)
	rethrow()
}

// preeditLog formats a PreeditEvent as `preedit "text" c=cursor
// [start-end …]`, a selected segment with a "*"; the end of a composition
// is `preedit ""`.
func preeditLog(pe shell.PreeditEvent) string {
	if pe.Text == "" && pe.Cursor == 0 && len(pe.Segments) == 0 {
		return `preedit ""`
	}
	s := fmt.Sprintf("preedit %q c=%d", pe.Text, pe.Cursor)
	if len(pe.Segments) > 0 {
		var segs []string
		for _, sg := range pe.Segments {
			seg := fmt.Sprintf("%d-%d", sg.Start, sg.End)
			if sg.Selected {
				seg += "*"
			}
			segs = append(segs, seg)
		}
		s += " [" + strings.Join(segs, " ") + "]"
	}
	return s
}

// The attributes input methods set.
func ul(start, end uint32) preeditAttr {
	return preeditAttr{typ: gtk.AttrUnderline, start: start, end: end, value: 1} // PANGO_UNDERLINE_SINGLE
}
func bg(start, end uint32) preeditAttr {
	return preeditAttr{typ: gtk.AttrBackground, start: start, end: end}
}
func fg(start, end uint32) preeditAttr {
	return preeditAttr{typ: gtk.AttrForeground, start: start, end: end}
}

const textEnd = gtk.AttrIndexToTextEnd

// TestPreeditOf: the cursor from characters to bytes (out of range: the
// end), the segments from the clause attributes (underlines that draw,
// backgrounds and foregrounds as the selected clause), clipped to the text
// and snapped to rune starts, and the text cleaned as commits are, with the
// offsets following.
func TestPreeditOf(t *testing.T) {
	seg := func(start, end int, sel bool) shell.PreeditSegment {
		return shell.PreeditSegment{Start: start, End: end, Selected: sel}
	}
	for _, c := range []struct {
		name   string
		text   string
		cursor int
		attrs  []preeditAttr
		want   shell.PreeditEvent
	}{
		{"empty", "", 0, []preeditAttr{ul(0, textEnd)}, shell.PreeditEvent{}},
		{"only control characters", "\x01\x7f", 2, nil, shell.PreeditEvent{}},
		{"simple context: one underline", "abc", 3, []preeditAttr{ul(0, textEnd)},
			shell.PreeditEvent{Text: "abc", Cursor: 3, Segments: []shell.PreeditSegment{seg(0, 3, false)}}},
		{"no attributes", "abc", 1, nil, shell.PreeditEvent{Text: "abc", Cursor: 1}},
		{"IBus: underline, highlighted clause", "にほんご", 2,
			[]preeditAttr{ul(0, 12), bg(0, 6), fg(0, 6)},
			shell.PreeditEvent{Text: "にほんご", Cursor: 6,
				Segments: []shell.PreeditSegment{seg(0, 6, true), seg(6, 12, false)}}},
		{"Fcitx: a clause each, the second highlighted", "日本語", -1,
			[]preeditAttr{ul(0, 6), ul(6, 9), bg(6, 9)},
			shell.PreeditEvent{Text: "日本語", Cursor: 9,
				Segments: []shell.PreeditSegment{seg(0, 6, false), seg(6, 9, true)}}},
		{"cursor past the end", "日本", 99, nil, shell.PreeditEvent{Text: "日本", Cursor: 6}},
		{"cursor at the start", "日本", 0, nil, shell.PreeditEvent{Text: "日本", Cursor: 0}},
		{"underline none and other types", "abc", 0,
			[]preeditAttr{{typ: gtk.AttrUnderline, start: 0, end: 3, value: gtk.UnderlineNone},
				{typ: 1, start: 0, end: 3}}, // PANGO_ATTR_LANGUAGE
			shell.PreeditEvent{Text: "abc"}},
		{"indices inside runes snap to their starts", "日本", 0, []preeditAttr{ul(1, 4)},
			shell.PreeditEvent{Text: "日本", Segments: []shell.PreeditSegment{seg(0, 3, false)}}},
		{"ranges past the text end there", "日本", 0, []preeditAttr{ul(3, 100)},
			shell.PreeditEvent{Text: "日本", Segments: []shell.PreeditSegment{seg(3, 6, false)}}},
		{"empty and reversed ranges", "abc", 0, []preeditAttr{ul(1, 1), ul(2, 1), bg(1, 2)},
			shell.PreeditEvent{Text: "abc", Segments: []shell.PreeditSegment{seg(1, 2, true)}}},
		{"a gap between clauses", "abcd", 0, []preeditAttr{ul(0, 1), ul(3, 4)},
			shell.PreeditEvent{Text: "abcd", Segments: []shell.PreeditSegment{seg(0, 1, false), seg(3, 4, false)}}},
		{"control characters dropped", "a\x01b", 2, []preeditAttr{ul(2, 3)},
			shell.PreeditEvent{Text: "ab", Cursor: 1, Segments: []shell.PreeditSegment{seg(1, 2, false)}}},
		{"invalid bytes replaced", "a\xffb", 3, []preeditAttr{ul(0, textEnd)},
			shell.PreeditEvent{Text: "a�b", Cursor: 5, Segments: []shell.PreeditSegment{seg(0, 5, false)}}},
	} {
		if got := preeditOf(c.text, c.cursor, c.attrs); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: preeditOf(%q, %d, %v) = %+v, want %+v", c.name, c.text, c.cursor, c.attrs, got, c.want)
		}
	}
}

var textInputOn = shell.TextInput{Enabled: true, Caret: driver.Rect{X: 4, Y: 5, W: 1, H: 16}}

// TestPreeditEvents: through the real trampolines, the preedit reaches the
// handler only while the text input is on; "preedit-start" sends nothing,
// each change is read and sent, a commit empties it so the end after it
// sends nothing, an empty change or "preedit-end" ends a composition once.
// Turning the input off and CancelComposition forget the composition, and
// what the input method does inside them is muted (TestTextInput* below).
// The input method's calls are in ims, not in the window calls.
func TestPreeditEvents(t *testing.T) {
	sf, fw, h, _ := openKey(t)
	fw.takeIMs()
	fw.preeditStart()
	fw.preeditChange("か", 1, ul(0, textEnd))
	fw.preeditEnd()
	wantLog(t, "text input off", fw.takeIMs())

	sf.SetTextInput(textInputOn)
	if sf.ime.input != textInputOn {
		t.Errorf("stored text input %+v, want %+v", sf.ime.input, textInputOn)
	}
	fw.preeditStart()
	fw.preeditChange("か", 1, ul(0, textEnd))
	fw.preeditChange("かな", 2, ul(0, textEnd), bg(3, 6))
	fw.imCommit("仮名")
	fw.preeditChange("", 0)
	fw.preeditEnd()
	wantLog(t, "a committed composition", fw.takeIMs(), "attach true", "cursor 4,5 1x16",
		"preedit", `preedit "か" c=3 [0-3]`,
		"preedit", `preedit "かな" c=6 [0-3 3-6*]`,
		`text "仮名"`, "preedit")

	fw.preeditStart()
	fw.preeditChange("a", 1)
	fw.preeditChange("", 0)
	fw.preeditEnd()
	fw.preeditChange("b", 1)
	fw.preeditEnd()
	fw.preeditEnd()
	wantLog(t, "ended compositions", fw.takeIMs(),
		"preedit", `preedit "a" c=1`, "preedit", `preedit ""`,
		"preedit", `preedit "b" c=1`, `preedit ""`)

	fw.preeditChange("c", 1)
	sf.SetTextInput(shell.TextInput{})
	sf.SetTextInput(textInputOn)
	fw.preeditEnd()
	fw.preeditChange("d", 1)
	sf.CancelComposition()
	fw.preeditEnd()
	wantLog(t, "forgotten compositions", fw.takeIMs(),
		"preedit", `preedit "c" c=1`, "reset", "reset", "attach false", "attach true", "cursor 4,5 1x16",
		"preedit", `preedit "d" c=1`, "reset")
	wantLog(t, "window calls", fw.take())
	_ = h
}

// TestPreeditDuringKey: with the text input on, the key controller gives
// the key to the input method first (S5.3j); a synchronous one composes
// inside it, so the key reaches no handler and the preedit is at the
// key's time (imEventTime). A preedit outside a key event (an asynchronous
// input method) is at now.
func TestPreeditDuringKey(t *testing.T) {
	sf, fw, h, f := openKey(t)
	sf.SetTextInput(textInputOn)
	fw.imCommits['x'] = "" // taken, composing
	fw.imPreedits = func(kv uint32) {
		if kv == 'x' {
			fw.pre = fakePreedit{text: "ｘ", cursor: 1, attrs: []preeditAttr{ul(0, textEnd)}}
			fw.preeditSignal(kindIMPreeditStart)
			fw.preeditSignal(kindIMPreeditChanged)
		}
	}
	const ms = time.Millisecond
	const t0 = 4_000_123 * time.Microsecond
	f.l.now = clockStart + t0.Microseconds()
	tick := uint32(f.l.now / 1000)
	fw.takeIMs()
	h.times = nil
	if r := fw.keyPress('x', kcX, 0, tick-30); r != 1 {
		t.Errorf("key controller composing = %d, want TRUE", r)
	}
	fw.preeditEnd()
	wantLog(t, "composing key", fw.takeIMs(), "im 0x78 true",
		"preedit", `preedit "ｘ" c=3 [0-3]`, `preedit ""`)
	if len(h.times) != 2 {
		t.Fatalf("times %v, want 2", h.times)
	}
	if h.times[0] != t0-30*ms {
		t.Errorf("preedit at %v, want the key's %v", h.times[0], t0-30*ms)
	}
	if e := h.times[1]; e < t0-30*ms || e > t0 {
		t.Errorf("the end outside a key event at %v, want between %v and now %v", e, t0-30*ms, t0)
	}
}

// TestPreeditCloseInHandler: Close inside a preedit handler ends it: the
// signals after it deliver nothing and read nothing.
func TestPreeditCloseInHandler(t *testing.T) {
	sf, fw, h, _ := openKey(t)
	sf.SetTextInput(textInputOn)
	h.onPreedit = func(shell.PreeditEvent) { sf.Close() }
	fw.takeIMs()
	fw.preeditChange("a", 1)
	fw.preeditChange("ab", 2)
	fw.preeditEnd()
	fw.imCommit("ab")
	wantLog(t, "close in preedit", fw.takeIMs(), "preedit", `preedit "a" c=1`)
	wantLog(t, "handler", h.take(), `preedit "a" c=1`, "closed")
	sf.voidSignal(kindIMPreeditChanged, testIM)
	sf.SetTextInput(shell.TextInput{})
	sf.CancelComposition()
	wantLog(t, "after Close", fw.takeIMs())
	if sf.ime.input != textInputOn {
		t.Error("SetTextInput after Close changed the state")
	}
}

// TestPreeditDuringCreate: the preedit signals while the window is being
// created deliver nothing and read nothing.
func TestPreeditDuringCreate(t *testing.T) {
	_, f := newSurfaceTest(t)
	h := &sfHandler{}
	f.during = func(fw *fakeWindow) {
		fw.preeditStart()
		fw.preeditChange("a", 1)
		fw.preeditEnd()
	}
	_, fw := f.open(t, sfResizable, h)
	wantLog(t, "ims during creation", fw.takeIMs())
	for _, l := range h.take() {
		if strings.HasPrefix(l, "preedit") {
			t.Errorf("a preedit during creation: %q", l)
		}
	}
}

// TestPreeditPanic: a panicking preedit handler is re-raised by rethrow as
// sigVoid's *PanicError, and the composition goes on.
func TestPreeditPanic(t *testing.T) {
	sf, fw, h, _ := openKey(t)
	sf.SetTextInput(textInputOn)
	h.onPreedit = func(pe shell.PreeditEvent) {
		if pe.Text == "a" {
			panic("in preedit")
		}
	}
	v := catchPanic(func() { fw.preeditChange("a", 1) })
	if pe, ok := v.(*PanicError); !ok || pe.Value != "in preedit" || pe.Callback != nameSigVoid {
		t.Fatalf("preedit panicked with %#v, want the *PanicError of sigVoid", v)
	}
	fw.takeIMs()
	fw.preeditChange("ab", 2)
	fw.preeditEnd()
	wantLog(t, "after the panic", fw.takeIMs(), "preedit", `preedit "ab" c=2`, `preedit ""`)
}

// keyWindow makes the fake window the key window.
func keyWindow(fw *fakeWindow, on bool) {
	fw.isActive = on
	fw.notify(kindActive)
}

// TestTextInputOrder (S5.3j): turning the text input on gives the key
// controller the input method, then the cursor location, then (in the key
// window) the focus; a new caret moves the cursor location only; turning
// it off resets, takes the focus away and detaches (which resets again,
// as GTK does). Unchanged states call nothing. Outside the key window the
// focus waits for it and goes with it, before KeyChanged.
func TestTextInputOrder(t *testing.T) {
	sf, fw, _, _ := openKey(t)
	keyWindow(fw, true)
	fw.takeIMs()
	sf.SetTextInput(textInputOn)
	wantLog(t, "on", fw.takeIMs(), "attach true", "cursor 4,5 1x16", "focus true")
	sf.SetTextInput(textInputOn)
	wantLog(t, "unchanged", fw.takeIMs())
	moved := textInputOn
	moved.Caret = driver.Rect{X: 10.5, Y: 20.25, W: 1, H: 15.5}
	sf.SetTextInput(moved)
	wantLog(t, "caret moved", fw.takeIMs(), "cursor 10,20 2x16")
	sf.SetTextInput(shell.TextInput{Caret: moved.Caret})
	wantLog(t, "off", fw.takeIMs(), "reset", "focus false", "reset", "attach false")
	sf.SetTextInput(shell.TextInput{})
	wantLog(t, "still off", fw.takeIMs())
	if sf.ime.attached || sf.ime.focused || fw.imAttached {
		t.Errorf("off: attached %v, focused %v, fake attached %v", sf.ime.attached, sf.ime.focused, fw.imAttached)
	}

	keyWindow(fw, false)
	fw.takeIMs()
	sf.SetTextInput(textInputOn)
	wantLog(t, "on outside the key window", fw.takeIMs(), "attach true", "cursor 4,5 1x16")
	keyWindow(fw, true)
	wantLog(t, "becoming key", fw.takeIMs(), "focus true", "key true")
	keyWindow(fw, false)
	wantLog(t, "losing key", fw.takeIMs(), "reset", "focus false", "key false")
	if !fw.imAttached {
		t.Error("losing key detached the input method; it stays attached while the input is on")
	}
	sf.SetTextInput(shell.TextInput{})
	wantLog(t, "off outside the key window", fw.takeIMs(), "reset", "reset", "attach false")
	wantLog(t, "window calls", fw.take())
}

// TestIMCaret: the caret widened to whole points, empty sizes kept,
// clamped to the C int range, NaN as the empty rectangle at 0, 0.
func TestIMCaret(t *testing.T) {
	nan := math.NaN()
	for _, c := range []struct {
		r    driver.Rect
		want gtk.Allocation
	}{
		{driver.Rect{X: 4, Y: 5, W: 1, H: 16}, gtk.Allocation{X: 4, Y: 5, Width: 1, Height: 16}},
		{driver.Rect{X: 10.5, Y: 20.25, W: 1, H: 15.5}, gtk.Allocation{X: 10, Y: 20, Width: 2, Height: 16}},
		{driver.Rect{X: -1.5, Y: -0.5, W: 0, H: 0}, gtk.Allocation{X: -2, Y: -1, Width: 1, Height: 1}},
		{driver.Rect{X: 3, Y: 3, W: -5, H: 0}, gtk.Allocation{X: 3, Y: 3}},
		{driver.Rect{X: 1e12, Y: -1e12, W: 1, H: 1}, gtk.Allocation{X: math.MaxInt32, Y: math.MinInt32, Width: 1, Height: 1}},
		{driver.Rect{X: nan, Y: 1, W: 1, H: 1}, gtk.Allocation{}},
	} {
		if got := imCaret(c.r); got != c.want {
			t.Errorf("imCaret(%+v) = %+v, want %+v", c.r, got, c.want)
		}
	}
}

// TestTextInputKeyOrder: with the text input on, the input method sees a
// key before the handler: a key it takes reaches no handler (its commit
// does, at the key's time), a key it declines reaches the handler and is
// neither filtered nor reset again, consumed or not; releases likewise.
// Off again, the S4.1k order is back. A handler that turns the input on
// inside a key does not filter that key afterwards.
func TestTextInputKeyOrder(t *testing.T) {
	sf, fw, h, f := openKey(t)
	keyWindow(fw, true)
	sf.SetTextInput(textInputOn)
	fw.takeIMs()
	h.times = nil
	const t0 = 5_000_123 * time.Microsecond
	f.l.now = clockStart + t0.Microseconds()
	tick := uint32(f.l.now / 1000)
	fw.keyPress('a', kcA, 0, tick-30)
	fw.keyRelease('a', kcA, 0, tick-20)
	wantLog(t, "typing", fw.takeIMs(), "im 0x61 true", `text "a"`, "im-up 0x61", "key-up 0x41")
	if len(h.times) != 2 || h.times[0] != t0-30*time.Millisecond || h.times[1] != t0-20*time.Millisecond {
		t.Errorf("times %v, want the commit at the press (%v) and the release", h.times, t0-30*time.Millisecond)
	}
	h.onKey = func(ke shell.KeyEvent) bool { return ke.Key == driver.KeyReturn }
	if r := fw.keyPress(0xff0d, kcReturn, 0, 5020); r != 1 {
		t.Errorf("consumed Return = %d, want TRUE", r)
	}
	if r := fw.keyPress(0xffe1, kcShiftL, 0, 5030); r != 0 {
		t.Errorf("declined Shift = %d, want FALSE", r)
	}
	wantLog(t, "declined keys", fw.takeIMs(),
		"im 0xff0d false", "key-down 0xd", "im 0xffe1 false", "key-down 0x0 m=1")

	sf.SetTextInput(shell.TextInput{})
	fw.takeIMs()
	fw.keyPress('a', kcA, 0, 5040)
	fw.keyPress(0xff0d, kcReturn, 0, 5050)
	wantLog(t, "off: S4.1k order", fw.takeIMs(),
		"key-down 0x41", "filter 0x61 true", `text "a"`, "key-down 0xd repeat", "reset")

	h.onKey = func(ke shell.KeyEvent) bool {
		if ke.Down && ke.Key == 'Q' {
			sf.SetTextInput(textInputOn)
		}
		return false
	}
	fw.keyPress('q', kcQ, 0, 5060)
	fw.keyRelease('q', kcQ, 0, 5070)
	wantLog(t, "turned on in the handler", fw.takeIMs(),
		"attach true", "cursor 4,5 1x16", "focus true", "key-down 0x51", // the handler's line follows its calls
		"im-up 0x71", "key-up 0x51")
}

// TestTextInputMuted: what the input method commits or composes inside
// SetTextInput, CancelComposition and the loss of the key window reaches
// no handler, and the composition they end sends no end afterwards. A
// CancelComposition with the input off calls nothing.
func TestTextInputMuted(t *testing.T) {
	sf, fw, h, _ := openKey(t)
	keyWindow(fw, true)
	fw.takeIMs()
	sf.CancelComposition()
	wantLog(t, "cancel while off", fw.takeIMs())
	fw.imHook = func(string) {
		fw.imCommit("x")
		fw.preeditChange("y", 1)
	}
	sf.SetTextInput(textInputOn)
	fw.imHook = nil
	fw.preeditChange("か", 1)
	h.take()
	fw.imHook = func(string) {
		fw.imCommit("か")
		fw.preeditChange("z", 1)
	}
	sf.CancelComposition()
	fw.imHook = nil
	fw.preeditEnd()
	fw.preeditChange("き", 1)
	fw.imHook = func(string) { fw.imCommit("き") }
	keyWindow(fw, false)
	fw.imHook = nil
	fw.preeditEnd()
	keyWindow(fw, true)
	fw.preeditChange("く", 1)
	fw.imHook = func(string) { fw.imCommit("く") }
	sf.SetTextInput(shell.TextInput{})
	fw.imHook = nil
	fw.preeditEnd()
	wantLog(t, "handler", h.take(), `preedit "き" c=3`, "key false", "key true", `preedit "く" c=3`)
	if sf.ime.mute != 0 {
		t.Errorf("mute %d after the calls", sf.ime.mute)
	}
}

// TestTextInputClose: closing takes the input method's focus away before
// the window is destroyed (destroy detaches it), and releases the input
// method only after the event being handled, once; a close inside a
// commit the key controller's filter makes works the same way.
func TestTextInputClose(t *testing.T) {
	sf, fw, h, f := openKey(t)
	keyWindow(fw, true)
	sf.SetTextInput(textInputOn)
	fw.takeIMs()
	fw.take()
	h.onText = func(shell.TextEvent) {
		sf.Close()
		if fw.imReleased {
			t.Error("the input method released inside its commit")
		}
	}
	fw.keyPress('a', kcA, 0, 6000)
	wantLog(t, "close in a commit", fw.takeIMs(), "im 0x61 true", `text "a"`, "focus false")
	wantLog(t, "window calls", fw.take(), "destroy")
	if fw.imReleased {
		t.Error("released before the queue ran")
	}
	f.s.q.Drain()
	if !fw.imReleased {
		t.Error("not released after the queue ran")
	}
	sf.SetTextInput(shell.TextInput{})
	sf.CancelComposition()
	wantLog(t, "after Close", fw.takeIMs())
}

// TestTextInputQuitReleases: Quit inside a commit closes the window but
// leaves the input method to shutdown, as the queue closes with Quit.
func TestTextInputQuitReleases(t *testing.T) {
	sf, fw, h, f := openKey(t)
	sf.SetTextInput(textInputOn)
	h.onText = func(shell.TextEvent) { f.s.Quit() }
	fw.keyPress('a', kcA, 0, 6000)
	if !fw.destroyed || fw.imReleased {
		t.Fatalf("after Quit in a commit: destroyed %v, released %v", fw.destroyed, fw.imReleased)
	}
	f.s.q.Drain()
	f.s.shutdown()
	if !fw.imReleased || len(f.s.imReleases) != 0 {
		t.Errorf("after shutdown: released %v, %d pending", fw.imReleased, len(f.s.imReleases))
	}
}
