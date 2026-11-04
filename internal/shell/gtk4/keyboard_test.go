package gtk4

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// The keyboard's host tests (S4.1k), over the fakeWindow: the key
// controller's signals and the input method's "commit" go through the real
// trampolines (fw.keyPress, fw.keyRelease, fw.imCommit in surface_test.go).

// GdkModifierType bits (modifiersOf).
const (
	kShift = 1
	kCtrl  = 4
	kAlt   = 8
	kSuper = 1 << 26
)

// The keycodes of the fake layout (evdev + 8, as X11 and Wayland report).
const (
	kcA      = 38
	kc1      = 10
	kcX      = 53
	kcE      = 26
	kcQ      = 24
	kcDead   = 48
	kcReturn = 36
	kcShiftL = 50
	kcCyr    = 39
	kcUni    = 40
)

// orderedHandler is an sfHandler that also appends each event's log line
// and KeyChanged to the window's ims, so the handler calls and the input
// method calls form one ordered log; onText runs on TextEvents.
type orderedHandler struct {
	sfHandler
	fw     *fakeWindow
	onText func(shell.TextEvent)
}

func (h *orderedHandler) Event(ev shell.Event) bool {
	n := len(h.log)
	r := h.sfHandler.Event(ev)
	if h.fw != nil {
		h.fw.ims = append(h.fw.ims, h.log[n]) // not what the handler logged after it ("closed")
	}
	if te, ok := ev.(shell.TextEvent); ok && h.onText != nil {
		h.onText(te)
	}
	return r
}

func (h *orderedHandler) KeyChanged(isKey bool) {
	h.sfHandler.KeyChanged(isKey)
	if h.fw != nil {
		h.fw.ims = append(h.fw.ims, h.log[len(h.log)-1])
	}
}

// keyLayout sets fw's fake layout and input method: a US-like layout plus
// a Cyrillic legacy keyval (not in testKeyvalToUnicode) and a Unicode one.
func keyLayout(fw *fakeWindow) {
	fw.base = map[uint32]uint32{
		kcA: 'a', kc1: '1', kcX: 'x', kcE: 'e', kcQ: 'q',
		kcDead: 0xfe51, kcReturn: 0xff0d, kcShiftL: 0xffe1,
		kcCyr: 0x6c1, kcUni: 0x01000430,
	}
	fw.imCommits = map[uint32]string{
		'a': "a", 'A': "A", '1': "1", '!': "!", 'e': "é", 'q': "q",
		0xfe51: "", // dead_acute: taken, no commit
	}
}

// openKey opens a surface with the fake layout and clears the logs.
func openKey(t *testing.T) (*surface, *fakeWindow, *orderedHandler, *winFactory) {
	t.Helper()
	_, f := newSurfaceTest(t)
	h := &orderedHandler{}
	sf, fw := f.open(t, sfResizable, h)
	keyLayout(fw)
	h.fw = fw
	fw.take()
	h.take()
	return sf, fw, h, f
}

// takeIMs returns the window's input-method (and, with orderedHandler,
// handler) log and clears it.
func (fw *fakeWindow) takeIMs() []string {
	l := fw.ims
	fw.ims = nil
	return l
}

func TestKeyFor(t *testing.T) {
	for _, c := range []struct {
		name     string
		kv, base uint32
		want     driver.Key
	}{
		// The named keys.
		{"BackSpace", 0xff08, 0, driver.KeyBackspace},
		{"Tab", 0xff09, 0, driver.KeyTab},
		{"ISO_Left_Tab", 0xfe20, 0xff09, driver.KeyTab},
		{"Return", 0xff0d, 0xff0d, driver.KeyReturn},
		{"KP_Enter", 0xff8d, 0xff8d, driver.KeyReturn},
		{"Escape", 0xff1b, 0, driver.KeyEscape},
		{"space", 0x20, 0x20, driver.KeySpace},
		{"KP_Space", 0xff80, 0, driver.KeySpace},
		{"Page_Up", 0xff55, 0, driver.KeyPageUp},
		{"Page_Down", 0xff56, 0, driver.KeyPageDown},
		{"End", 0xff57, 0, driver.KeyEnd},
		{"Home", 0xff50, 0, driver.KeyHome},
		{"Left", 0xff51, 0, driver.KeyLeft},
		{"Up", 0xff52, 0, driver.KeyUp},
		{"Right", 0xff53, 0, driver.KeyRight},
		{"Down", 0xff54, 0, driver.KeyDown},
		{"Delete", 0xffff, 0, driver.KeyDelete},
		{"KP_Home", 0xff95, 0xff95, driver.KeyHome},
		{"KP_Left", 0xff96, 0xff96, driver.KeyLeft},
		{"KP_Up", 0xff97, 0xff97, driver.KeyUp},
		{"KP_Right", 0xff98, 0xff98, driver.KeyRight},
		{"KP_Down", 0xff99, 0xff99, driver.KeyDown},
		{"KP_Page_Up", 0xff9a, 0xff9a, driver.KeyPageUp},
		{"KP_Page_Down", 0xff9b, 0xff9b, driver.KeyPageDown},
		{"KP_End", 0xff9c, 0xff9c, driver.KeyEnd},
		{"KP_Delete", 0xff9f, 0xff9f, driver.KeyDelete},
		{"KP_Multiply", 0xffaa, 0, '*'},
		{"KP_Add", 0xffab, 0, '+'},
		{"KP_Separator", 0xffac, 0, ','},
		{"KP_Subtract", 0xffad, 0, '-'},
		{"KP_Decimal", 0xffae, 0, '.'},
		{"KP_Divide", 0xffaf, 0, '/'},
		{"KP_Equal", 0xffbd, 0, '='},
		{"KP_0", 0xffb0, 0xff9e, '0'},
		{"KP_5", 0xffb5, 0xff9d, '5'},
		{"KP_9", 0xffb9, 0xff9a, '9'},
		{"F1", 0xffbe, 0xffbe, driver.KeyF1},
		{"F5", 0xffc2, 0xffc2, driver.KeyF1 + 4},
		{"F12", 0xffc9, 0xffc9, driver.KeyF12},
		{"F13", 0xffca, 0xffca, driver.KeyNone},
		{"F35", 0xffe0, 0, driver.KeyNone},
		// Keys driver.Key cannot name.
		{"Shift_L", 0xffe1, 0xffe1, driver.KeyNone},
		{"Control_L", 0xffe3, 0xffe3, driver.KeyNone},
		{"Super_L", 0xffeb, 0xffeb, driver.KeyNone},
		{"ISO_Level3_Shift", 0xfe03, 0xfe03, driver.KeyNone},
		{"Caps_Lock", 0xffe5, 0, driver.KeyNone},
		{"Insert", 0xff63, 0xff63, driver.KeyNone},
		{"KP_Insert", 0xff9e, 0xff9e, driver.KeyNone},
		{"KP_Begin", 0xff9d, 0xff9d, driver.KeyNone},
		{"Menu", 0xff67, 0, driver.KeyNone},
		// Character keys: the unshifted keyval, normalized.
		{"a", 'a', 'a', 'A'},
		{"Shift+a", 'A', 'a', 'A'},
		{"Ctrl+a", 'a', 'a', 'A'},
		{"Shift+1", '!', '1', '1'},
		{"AZERTY &", '1', '&', '&'},
		{"base 0: kv", 'q', 0, 'Q'},
		{"base 0: punctuation", ';', 0, ';'},
		{"Latin-1 é", 0xe9, 0xe9, 0xc9},
		{"Cyrillic legacy keyval (no Unicode here)", 0x6c1, 0x6c1, driver.KeyNone},
		{"Unicode keyval а", 0x01000430, 0x01000430, 0x410},
		{"Shift+а", 0x01000410, 0x01000430, 0x410},
		{"Unicode keyval ω", 0x010003c9, 0, 0x3a9},
		{"AltGr+space: base space", 0xa0, 0x20, driver.KeySpace},
		{"named kv wins over base", 0xff51, 's', driver.KeyLeft},
		{"named kv wins over a named base", 0xff09, 0xff0d, driver.KeyTab},
		// A named base when the modifiers made the keyval another one.
		{"Ctrl+Alt+F1: XF86Switch_VT_1", 0x1008fe01, 0xffbe, driver.KeyF1},
		{"Ctrl+Alt+F12: XF86Switch_VT_12", 0x1008fe0c, 0xffc9, driver.KeyF12},
		{"Ctrl+Alt+BackSpace: Terminate_Server", 0xfed5, 0xff08, driver.KeyBackspace},
		{"NumLock: KP_1 on base KP_End", 0xffb1, 0xff9c, '1'},
		{"a letter kv on base Left", 'h', 0xff51, driver.KeyLeft},
		{"base KP_0 (a named special)", 0x1234, 0xffb0, '0'},
		{"base F13 stays none", 0x1008ff00, 0xffca, driver.KeyNone},
		{"base Shift_L, kv ISO_Next_Group", 0xfe08, 0xffe1, driver.KeyNone},
		{"Unicode base past 0xffff is no named key", 0x1234, 0x0100ff0d, 0xff0d},
		// Dead keys: their character.
		{"dead_acute", 0xfe51, 0xfe51, '´'},
		{"dead_acute, base 0", 0xfe51, 0, '´'},
		{"Shift+dead: base dead_acute", 0xfe50, 0xfe51, '´'},
		{"dead_grave", 0xfe50, 0, '`'},
		{"dead_circumflex", 0xfe52, 0, '^'},
		{"dead_diaeresis", 0xfe57, 0, '¨'},
		{"dead_ogonek", 0xfe5c, 0, '˛'},
		{"dead_iota (not mapped)", 0xfe5d, 0, driver.KeyNone},
		// No character.
		{"VoidSymbol", 0xffffff, 0, driver.KeyNone},
		{"0", 0, 0, driver.KeyNone},
		{"Unicode surrogate", 0x0100d800, 0, driver.KeyNone},
	} {
		if got := keyFor(c.kv, c.base, testKeyvalToUnicode); got != c.want {
			t.Errorf("%s: keyFor(%#x, %#x) = %#x, want %#x", c.name, c.kv, c.base, got, c.want)
		}
	}
}

// TestKeyForUnicode: keyvals whose Unicode is a control character or not
// a valid rune give KeyNone; lower-case letters are upper-cased.
func TestKeyForUnicode(t *testing.T) {
	for _, c := range []struct {
		r    rune
		want driver.Key
	}{
		{0, driver.KeyNone}, {0x01, driver.KeyNone}, {0x1b, driver.KeyNone},
		{0x1f, driver.KeyNone}, {0x7f, driver.KeyNone}, {0x80, driver.KeyNone},
		{0x85, driver.KeyNone}, {0x9f, driver.KeyNone}, {0xd800, driver.KeyNone},
		{0x110000, driver.KeyNone}, {-1, driver.KeyNone},
		{0xa0, 0xa0}, {'z', 'Z'}, {'Z', 'Z'}, {'ä', 'Ä'}, {'ж', 'Ж'}, {'7', '7'},
		{0x1f600, 0x1f600},
	} {
		got := keyFor(0x1234, 0, func(uint32) rune { return c.r })
		if got != c.want {
			t.Errorf("keyFor with Unicode %#x = %#x, want %#x", c.r, got, c.want)
		}
	}
	// toUnicode gets the base, not the event's keyval.
	var asked []uint32
	keyFor('A', 'a', func(kv uint32) rune { asked = append(asked, kv); return rune(kv) })
	if len(asked) != 1 || asked[0] != 'a' {
		t.Errorf("toUnicode asked for %#x, want only the base 0x61", asked)
	}
}

func TestTextCommitText(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"a", "a"},
		{"", ""},
		{"é", "é"},
		{"😀", "😀"},
		{"a😀b", "a😀b"},
		{"a\tb\n", "ab"},
		{"\r\n\t", ""},
		{"\x7f", ""},
		{"x\u0085y", "xy"},
		{"\u009f\u0080", ""},
		{"\x1b[A", "[A"},
		{"a\x00b", "ab"},
		{"\b", ""},
		{"\xffa", "�a"},
		{"a\xc3", "a�"},
		{"\xed\xa0\x80", "�"},            // an encoded surrogate
		{"\u00a0\u200b", "\u00a0\u200b"}, // a no-break space and a format character are not controls
	} {
		got := commitText(c.in)
		if got != c.want {
			t.Errorf("commitText(%q) = %q, want %q", c.in, got, c.want)
		}
		if !utf8.ValidString(got) || strings.ContainsFunc(got, isControl) {
			t.Errorf("commitText(%q) = %q: invalid UTF-8 or a control character", c.in, got)
		}
	}
}

// TestKeyDownText: an unhandled key-down is its KeyEvent, then the input
// method, then the text it commits; the gboolean is whether the input
// method took it. Key-ups go to the input method after their KeyEvent.
func TestKeyDownText(t *testing.T) {
	_, fw, h, _ := openKey(t)
	if r := fw.keyPress('a', kcA, 0, 1000); r != 1 {
		t.Errorf("key-pressed a = %d, want TRUE (the input method took it)", r)
	}
	fw.keyRelease('a', kcA, 0, 1010)
	wantLog(t, "a", fw.takeIMs(), "key-down 0x41", "filter 0x61 true", "text \"a\"",
		"key-up 0x41", "filter-up 0x61")

	if r := fw.keyPress('A', kcA, kShift, 1020); r != 1 {
		t.Errorf("key-pressed Shift+a = %d, want TRUE", r)
	}
	fw.keyRelease('A', kcA, kShift, 1030)
	wantLog(t, "Shift+a", fw.takeIMs(), "key-down 0x41 m=1", "filter 0x41 true", "text \"A\"",
		"key-up 0x41 m=1", "filter-up 0x41")

	fw.keyPress('!', kc1, kShift, 1040)
	wantLog(t, "Shift+1", fw.takeIMs(), "key-down 0x31 m=1", "filter 0x21 true", "text \"!\"")
	fw.keyRelease('!', kc1, kShift, 1041)
	fw.takeIMs()

	// A key the input method does not take: FALSE, GTK goes on.
	if r := fw.keyPress(0xff0d, kcReturn, 0, 1050); r != 0 {
		t.Errorf("key-pressed Return = %d, want FALSE", r)
	}
	fw.keyRelease(0xff0d, kcReturn, 0, 1060)
	wantLog(t, "Return", fw.takeIMs(), "key-down 0xd", "filter 0xff0d false",
		"key-up 0xd", "filter-up 0xff0d")

	// A dead key: taken without text; the next key commits the composed one.
	if r := fw.keyPress(0xfe51, kcDead, 0, 1070); r != 1 {
		t.Errorf("key-pressed dead_acute = %d, want TRUE", r)
	}
	fw.keyRelease(0xfe51, kcDead, 0, 1080)
	fw.keyPress('e', kcE, 0, 1090)
	wantLog(t, "dead key", fw.takeIMs(), "key-down 0xb4", "filter 0xfe51 true",
		"key-up 0xb4", "filter-up 0xfe51", "key-down 0x45", "filter 0x65 true", "text \"é\"")
	fw.keyRelease('e', kcE, 0, 1091)
	fw.takeIMs()

	// A modifier alone: KeyNone; its own modifier is added on the key-down
	// (the state is before the key) and removed on the key-up; all four
	// modifiers.
	fw.keyPress(0xffe1, kcShiftL, 0, 1100)
	fw.keyPress('a', kcA, kShift|kCtrl|kAlt|kSuper, 1110)
	fw.keyRelease('a', kcA, kShift|kCtrl|kAlt|kSuper, 1120)
	fw.keyRelease(0xffe1, kcShiftL, kShift, 1130)
	wantLog(t, "modifiers", fw.takeIMs(), "key-down 0x0 m=1", "filter 0xffe1 false",
		"key-down 0x41 m=15", "filter 0x61 true", "text \"a\"",
		"key-up 0x41 m=15", "filter-up 0x61", "key-up 0x0", "filter-up 0xffe1")

	// The key comes from the keycode's base: a Unicode keyval, and a legacy
	// one the fake cannot map.
	h.take()
	fw.keyPress(0x01000410, kcUni, kShift, 1140)
	fw.keyPress(0x6c1, kcCyr, 0, 1150)
	wantLog(t, "base keyvals", h.take(), "key-down 0x410 m=1", "key-down 0x0")
	fw.takeIMs()
	wantLog(t, "window calls", fw.take())
}

// TestKeyModifierKeys: a modifier key's own modifier is in its key-down's
// Mods (by its keyval or its base) and not in its key-up's.
func TestKeyModifierKeys(t *testing.T) {
	_, fw, h, _ := openKey(t)
	for _, c := range []struct {
		name     string
		kv, base uint32
		down, up uint32 // the states
		wantDown string
		wantUp   string
		keycode  uint32
	}{
		{"Shift_L", 0xffe1, 0xffe1, 0, kShift, "key-down 0x0 m=1", "key-up 0x0", 50},
		{"Shift_R", 0xffe2, 0xffe2, 0, kShift, "key-down 0x0 m=1", "key-up 0x0", 62},
		{"Control_L", 0xffe3, 0xffe3, 0, kCtrl, "key-down 0x0 m=2", "key-up 0x0", 37},
		{"Control_R", 0xffe4, 0xffe4, 0, kCtrl, "key-down 0x0 m=2", "key-up 0x0", 105},
		{"Alt_L", 0xffe9, 0xffe9, 0, kAlt, "key-down 0x0 m=4", "key-up 0x0", 64},
		{"Alt_R", 0xffea, 0xffea, 0, kAlt, "key-down 0x0 m=4", "key-up 0x0", 108},
		{"Meta_L", 0xffe7, 0xffe7, 0, kAlt, "key-down 0x0 m=4", "key-up 0x0", 64},
		{"Meta_R", 0xffe8, 0xffe8, 0, kAlt, "key-down 0x0 m=4", "key-up 0x0", 108},
		{"Super_L", 0xffeb, 0xffeb, 0, kSuper, "key-down 0x0 m=8", "key-up 0x0", 133},
		{"Super_R", 0xffec, 0xffec, 0, kSuper, "key-down 0x0 m=8", "key-up 0x0", 134},
		{"Hyper_L", 0xffed, 0xffed, 0, kSuper, "key-down 0x0 m=8", "key-up 0x0", 207},
		{"Hyper_R", 0xffee, 0xffee, 0, kSuper, "key-down 0x0 m=8", "key-up 0x0", 208},
		{"Shift_L with Ctrl held", 0xffe1, 0xffe1, kCtrl, kCtrl | kShift, "key-down 0x0 m=3", "key-up 0x0 m=2", 50},
		{"by the base: ISO_Next_Group on Shift_R", 0xfe08, 0xffe2, kShift, kShift, "key-down 0x0 m=1", "key-up 0x0", 62},
		{"Shift_L without the bit on its key-up", 0xffe1, 0xffe1, 0, 0, "key-down 0x0 m=1", "key-up 0x0", 50},
		{"ISO_Level3_Shift: no modifier", 0xfe03, 0xfe03, 0, 0x80, "key-down 0x0", "key-up 0x0", 92},
		{"Caps_Lock: no modifier", 0xffe5, 0xffe5, 0, 2, "key-down 0x0", "key-up 0x0", 66},
		{"a letter: state only", 'a', 'a', kShift, kShift, "key-down 0x41 m=1", "key-up 0x41 m=1", kcA},
	} {
		fw.base[c.keycode] = c.base
		fw.keyPress(c.kv, c.keycode, c.down, 10_000)
		fw.keyRelease(c.kv, c.keycode, c.up, 10_010)
		got := keyLines(h.take())
		wantLog(t, c.name, got, c.wantDown, c.wantUp)
	}
	fw.takeIMs()
}

// TestKeyConsumed: a consumed key-down returns TRUE, resets the input
// method and produces no text; its key-up still reaches the input method.
func TestKeyConsumed(t *testing.T) {
	_, fw, h, _ := openKey(t)
	h.onKey = func(ke shell.KeyEvent) bool { return true }
	if r := fw.keyPress('a', kcA, kCtrl, 2000); r != 1 {
		t.Errorf("consumed key-pressed = %d, want TRUE", r)
	}
	fw.keyRelease('a', kcA, kCtrl, 2010)
	wantLog(t, "consumed", fw.takeIMs(), "key-down 0x41 m=2", "reset", "key-up 0x41 m=2", "filter-up 0x61")

	// A consumed key the input method would not take: still TRUE.
	if r := fw.keyPress(0xff1b, 9, 0, 2020); r != 1 {
		t.Errorf("consumed Escape = %d, want TRUE", r)
	}
	wantLog(t, "consumed Escape", fw.takeIMs(), "key-down 0x1b", "reset")

	// Only the key-downs the handler consumes.
	h.onKey = func(ke shell.KeyEvent) bool { return ke.Key == 'Q' }
	fw.keyPress('a', kcA, 0, 2030)
	fw.keyPress('q', kcQ, 0, 2040)
	wantLog(t, "selective", fw.takeIMs(), "key-down 0x41", "filter 0x61 true", "text \"a\"",
		"key-down 0x51", "reset")
}

// TestKeyRepeat: a press of a keycode already down is a Repeat; not after
// its release, not for another keycode; the result does not matter.
func TestKeyRepeat(t *testing.T) {
	_, fw, h, _ := openKey(t)
	fw.keyPress('a', kcA, 0, 3000)
	fw.keyPress('a', kcA, 0, 3030)
	h.onKey = func(ke shell.KeyEvent) bool { return true }
	fw.keyPress('a', kcA, 0, 3060) // consumed: still held
	h.onKey = nil
	fw.keyPress('1', kc1, 0, 3070)
	fw.keyRelease('a', kcA, 0, 3080)
	fw.keyPress('a', kcA, 0, 3090)
	fw.keyPress('1', kc1, 0, 3100)
	fw.keyRelease('1', kc1, 0, 3110)
	fw.keyRelease('1', kc1, 0, 3120) // an unmatched release
	fw.keyPress('1', kc1, 0, 3130)
	wantLog(t, "repeats", keyLines(h.take()),
		"key-down 0x41", "key-down 0x41 repeat", "key-down 0x41 repeat",
		"key-down 0x31", "key-up 0x41", "key-down 0x41", "key-down 0x31 repeat",
		"key-up 0x31", "key-up 0x31", "key-down 0x31")
	// A repeat produces text as a press does.
	fw.takeIMs()
	fw.keyPress('a', kcA, 0, 3140)
	wantLog(t, "repeat text", fw.takeIMs(), "key-down 0x41 repeat", "filter 0x61 true", "text \"a\"")
}

// TestKeyFocus: the key window's changes focus the input method in and
// out (out: reset first), before KeyChanged; focus loss forgets the held
// keys.
func TestKeyFocus(t *testing.T) {
	_, fw, h, _ := openKey(t)
	fw.isActive = true
	fw.notify(kindActive)
	wantLog(t, "focus in", fw.takeIMs(), "focus true", "key true")
	fw.notify(kindActive) // no change
	wantLog(t, "no change", fw.takeIMs())

	fw.keyPress('a', kcA, 0, 4000)
	fw.keyPress(0xffe1, kcShiftL, 0, 4010)
	fw.isActive = false
	fw.notify(kindActive)
	fw.keyPress('a', kcA, 0, 4020)
	fw.keyPress(0xffe1, kcShiftL, 0, 4030)
	wantLog(t, "focus out", fw.takeIMs(),
		"key-down 0x41", "filter 0x61 true", "text \"a\"", "key-down 0x0 m=1", "filter 0xffe1 false",
		"reset", "focus false", "key false",
		"key-down 0x41", "filter 0x61 true", "text \"a\"", "key-down 0x0 m=1", "filter 0xffe1 false")
	h.take()

	fw.isActive = true
	fw.notify(kindActive)
	fw.keyPress('a', kcA, 0, 4040)
	wantLog(t, "focus in again", h.take(), "key true", "key-down 0x41 repeat", "text \"a\"")
	wantLog(t, "window calls", fw.take())
}

// TestTextCommit: the commits are filtered (only controls: no TextEvent);
// a commit outside a key event is a TextEvent too.
func TestTextCommit(t *testing.T) {
	_, fw, h, _ := openKey(t)
	fw.imCommits['x'] = "\t\x7f\n"
	if r := fw.keyPress('x', kcX, 0, 5000); r != 1 {
		t.Errorf("key-pressed x = %d, want TRUE", r)
	}
	wantLog(t, "controls only", fw.takeIMs(), "key-down 0x58", "filter 0x78 true")
	fw.imCommits['x'] = "a\x01b\xff"
	fw.keyPress('x', kcX, 0, 5010)
	wantLog(t, "filtered", fw.takeIMs(), "key-down 0x58 repeat", "filter 0x78 true", "text \"ab�\"")
	fw.keyRelease('x', kcX, 0, 5020)
	fw.takeIMs()

	h.take()
	fw.imCommit("ü")
	fw.imCommit("\n")
	fw.imCommit("")
	fw.imCommit("x\u0085y")
	fw.imCommit("日本")
	wantLog(t, "outside key events", h.take(), "text \"ü\"", "text \"xy\"", "text \"日本\"")
	wantLog(t, "window calls", fw.take())
}

// TestKeyEventTimes: the key events carry their GDK times on the clock,
// and the text a key produces carries its key's time.
func TestKeyEventTimes(t *testing.T) {
	_, fw, h, f := openKey(t)
	const ms = time.Millisecond
	const t0 = 3_000_789 * time.Microsecond
	f.l.now = clockStart + t0.Microseconds()
	tick := uint32(f.l.now / 1000)
	fw.keyPress('a', kcA, 0, tick-50)   // key-down, text
	fw.keyRelease('a', kcA, 0, tick-40) // key-up
	h.onKey = func(shell.KeyEvent) bool { return true }
	fw.keyPress('q', kcQ, 0, tick-30) // consumed: no text
	h.onKey = nil
	fw.keyRelease('q', kcQ, 0, 0) // no time: now
	f.l.now += 500_000
	tick += 500
	fw.keyPress('1', kc1, 0, tick-20) // key-down, text
	fw.imCommit("z")                  // outside a key event
	wantLog(t, "events", h.take(), "key-down 0x41", "text \"a\"", "key-up 0x41",
		"key-down 0x51", "key-up 0x51", "key-down 0x31", "text \"1\"", "text \"z\"")
	want := []time.Duration{t0 - 50*ms, t0 - 50*ms, t0 - 40*ms, t0 - 30*ms, t0,
		t0 + 500*ms - 20*ms, t0 + 500*ms - 20*ms}
	if len(h.times) != len(want)+1 {
		t.Fatalf("times %v, want %v and the commit's", h.times, want)
	}
	for i := range want {
		if h.times[i] != want[i] {
			t.Errorf("time %d = %v, want %v", i, h.times[i], want[i])
		}
	}
	if c := h.times[len(want)]; c < want[len(want)-1] || c > t0+500*ms {
		t.Errorf("the commit outside a key event at %v, want between %v and now %v",
			c, want[len(want)-1], t0+500*ms)
	}
}

// TestKeyCloseInHandler: Close inside a key handler ends the key's
// handling: no input method, no text, nothing after.
func TestKeyCloseInHandler(t *testing.T) {
	sf, fw, h, _ := openKey(t)
	h.onKey = func(ke shell.KeyEvent) bool {
		if ke.Down {
			sf.Close()
		}
		return false
	}
	if r := fw.keyPress('a', kcA, 0, 6000); r != 1 {
		t.Errorf("key-pressed closing in its handler = %d, want TRUE", r)
	}
	wantLog(t, "close in key-down", fw.takeIMs(), "key-down 0x41")
	wantLog(t, "handler", h.take(), "key-down 0x41", "closed")
	wantLog(t, "window calls", fw.take(), "destroy")
	if r := fw.keyPress('a', kcA, 0, 6010); r != 0 {
		t.Errorf("key-pressed after Close = %d, want FALSE", r)
	}
	fw.keyRelease('a', kcA, 0, 6020)
	fw.imCommit("a")
	fw.isActive = true
	fw.notify(kindActive)
	wantLog(t, "after Close", h.take())
	wantLog(t, "ims after Close", fw.takeIMs())

	// Close in a key-up: no filter-up.
	sf2, fw2, h2, _ := openKey(t)
	h2.onKey = func(ke shell.KeyEvent) bool {
		if !ke.Down {
			sf2.Close()
		}
		return false
	}
	fw2.keyPress('a', kcA, 0, 6100)
	fw2.keyRelease('a', kcA, 0, 6110)
	wantLog(t, "close in key-up", fw2.takeIMs(), "key-down 0x41", "filter 0x61 true", "text \"a\"", "key-up 0x41")
	wantLog(t, "handler", h2.take(), "key-down 0x41", "text \"a\"", "key-up 0x41", "closed")

	// Close in the text of a key-down.
	sf3, fw3, h3, _ := openKey(t)
	h3.onText = func(shell.TextEvent) { sf3.Close() }
	fw3.keyPress('a', kcA, 0, 6200)
	fw3.keyRelease('a', kcA, 0, 6210)
	fw3.imCommit("b")
	wantLog(t, "close in text", h3.take(), "key-down 0x41", "text \"a\"", "closed")
	wantLog(t, "ims", fw3.takeIMs(), "key-down 0x41", "filter 0x61 true", "text \"a\"")
}

// TestKeyDuringCreate: the key signals while the window is being created
// deliver nothing, use no input method and leave no key held.
func TestKeyDuringCreate(t *testing.T) {
	_, f := newSurfaceTest(t)
	h := &orderedHandler{}
	var rs []int32
	f.during = func(fw *fakeWindow) {
		keyLayout(fw)
		rs = append(rs, fw.keyPress('a', kcA, 0, 7000))
		fw.keyRelease('a', kcA, 0, 7010)
		rs = append(rs, fw.keyPress('a', kcA, 0, 7020))
		fw.imCommit("x")
		fw.isActive = true
		fw.notify(kindActive)
		fw.isActive = false
		fw.notify(kindActive)
	}
	_, fw := f.open(t, sfResizable, h)
	f.during = nil
	h.fw = fw
	wantLog(t, "handler calls during creation", h.take())
	wantLog(t, "ims during creation", fw.takeIMs())
	for i, r := range rs {
		if r != 0 {
			t.Errorf("key-pressed %d during creation = %d, want FALSE", i, r)
		}
	}
	fw.keyPress('a', kcA, 0, 7100)
	wantLog(t, "after creation", fw.takeIMs(), "key-down 0x41", "filter 0x61 true", "text \"a\"")
}

// TestKeyAfterClose: after Close neither the signals nor the peer methods
// deliver anything or use the window; signals of other kinds are ignored.
func TestKeyAfterClose(t *testing.T) {
	sf, fw, h, _ := openKey(t)
	if sf.boolUintUintUint(kindScroll, testKey, 'a', kcA, 0) {
		t.Error("boolUintUintUint of another kind = true")
	}
	sf.uintUintUint(kindScroll, testKey, 'a', kcA, 0)
	wantLog(t, "other kinds", h.take())
	wantLog(t, "other kinds' ims", fw.takeIMs())

	fw.keyPress('a', kcA, 0, 8000)
	sf.Close()
	fw.take()
	h.take()
	fw.takeIMs()
	if r := fw.keyPress('a', kcA, 0, 8010); r != 0 {
		t.Errorf("key-pressed after Close = %d, want FALSE", r)
	}
	fw.keyRelease('a', kcA, 0, 8020)
	fw.imCommit("a")
	if sf.boolUintUintUint(kindKeyPressed, testKey, 'a', kcA, 0) {
		t.Error("boolUintUintUint after Close = true")
	}
	sf.uintUintUint(kindKeyReleased, testKey, 'a', kcA, 0)
	sf.pointerSignal(kindIMCommit, testIM, 0x7e58)
	sf.pointerSignal(kindActive, fw.addr, 0)
	wantLog(t, "handler calls after Close", h.take())
	wantLog(t, "ims after Close", fw.takeIMs())
	wantLog(t, "window calls after Close", fw.take())
}

// TestKeyPanic: a panicking key handler is re-raised by rethrow as the
// trampoline's *PanicError, the trampoline returned FALSE, and the
// keyboard stays usable.
func TestKeyPanic(t *testing.T) {
	sf, fw, h, _ := openKey(t)
	h.onKey = func(ke shell.KeyEvent) bool {
		if ke.Down {
			panic("in key-down")
		}
		return false
	}
	fw.evTime, fw.evKeyval, fw.evDown = 9000, 'a', true
	r := sigBoolUintUintUint(testKey, 'a', kcA, 0, uintptr(fw.sig[kindKeyPressed]))
	if r != 0 {
		t.Errorf("the panicking key-pressed returned %d, want FALSE", r)
	}
	v := catchPanic(rethrow)
	if pe, ok := v.(*PanicError); !ok || pe.Value != "in key-down" || pe.Callback != nameSigBoolUintUintUint {
		t.Fatalf("rethrow panicked with %#v, want the *PanicError of sigBoolUintUintUint", v)
	}
	if pendingPanic != nil || sf.dead {
		t.Fatalf("after rethrow: pending %v, dead %v", pendingPanic, sf.dead)
	}
	wantLog(t, "handler", h.take(), "key-down 0x41")
	wantLog(t, "ims (no filter after the panic)", fw.takeIMs())

	// A panic in a key-up, through fw.keyRelease.
	h.onKey = func(ke shell.KeyEvent) bool {
		if !ke.Down {
			panic("in key-up")
		}
		return false
	}
	v = catchPanic(func() { fw.keyRelease('a', kcA, 0, 9010) })
	if pe, ok := v.(*PanicError); !ok || pe.Value != "in key-up" || pe.Callback != nameSigUintUintUint {
		t.Fatalf("key-up panicked with %#v, want the *PanicError of sigUintUintUint", v)
	}
	wantLog(t, "ims (no filter-up after the panic)", fw.takeIMs())
	h.take()

	h.onKey = nil
	fw.keyPress('a', kcA, 0, 9020)
	fw.keyRelease('a', kcA, 0, 9030)
	wantLog(t, "after the panics", fw.takeIMs(), "key-down 0x41", "filter 0x61 true", "text \"a\"",
		"key-up 0x41", "filter-up 0x61")
	if pendingPanic != nil {
		t.Errorf("pending panic left: %v", pendingPanic)
	}
	wantLog(t, "window calls", fw.take())
}

// keyLines returns the KeyEvent lines of a handler log.
func keyLines(l []string) []string {
	var keys []string
	for _, s := range l {
		if strings.HasPrefix(s, "key-") {
			keys = append(keys, s)
		}
	}
	return keys
}
