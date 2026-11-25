package gtk4

import (
	"strings"
	"unicode/utf8"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// Keyboard input (S4.1k). A GtkEventControllerKey on the GtkWindow, in the
// CAPTURE phase so it sees every key before GTK's own handling (the
// window's key bindings, focus moves), added by window_linux.go at
// creation, and an input method for the text:
//
//   - "key-pressed" (keyval, keycode, state) is a KeyEvent{Down: true}; its
//     gboolean result is the handler's, so a consumed key stops GTK. A
//     key-down of a keycode that is already down is a Repeat (GTK 4 has no
//     repeat flag; X11 and Wayland repeat the press without a release).
//   - "key-released" is a KeyEvent{Down: false}.
//   - The key is keyFor's: the named keys from a keyval table (keyKeys,
//     the keypad included), F1–F12, and character keys as the unshifted
//     keyval of the active layout (level 0 of the event's keycode and
//     layout group, gdk_display_translate_key) mapped to Unicode,
//     KeyNone for the rest (modifier keys alone, F13 and up, Insert).
//   - A key-down of a character key also carries Shifted (S5.6h): the
//     keyval of the same keycode at the Shift level of the event's layout
//     group (gdk_display_translate_key with GDK_SHIFT_MASK alone, so Caps
//     Lock and Ctrl do not change it) mapped to Unicode (shiftedFor).
//   - Text. A key-down the handler did not consume goes to the input method
//     (gtk_im_context_filter_keypress of the current event), which commits
//     the text it produces ("commit"), dead keys and compose sequences
//     included; the commit is a TextEvent, with control characters (C0,
//     DEL, C1) dropped, so it follows its KeyEvent. A consumed key-down
//     resets the input method instead, so it produces no text. Key-ups go
//     to the input method too (some sequences end on a release). The
//     input method is a GtkIMMulticontext since S5.3i (the user's IBus or
//     Fcitx, GTK's simple context as its fallback), whose preedit is the
//     IME's (ime.go). While the text input is on (S5.3j) the order turns:
//     GTK gives every key to the input method first, and only the keys it
//     declines reach "key-pressed" and "key-released" (ime.go).
//
// A modifier key alone carries its Modifier (keyModifier, S5.6i) on the
// press and the release, so the aqua menu strip tells a lone Alt from
// Insert or F13.
//
// Mods are the modifiers held after the key, as on Win32: GTK's state is
// the one before it, so a modifier key's own modifier is added on its
// press and removed on its release. AltGr (ISO_Level3, Mod5) is no
// modifier here; Win32 reports it as Control+Option.
//
// When the surface stops being the key window, the keys held are
// forgotten (no key-ups come for them) and the input method is reset.

// GDK keyvals (gdkkeysyms.h) of the named keys.
var keyKeys = map[uint32]driver.Key{
	0xff08: driver.KeyBackspace, // BackSpace
	0xff09: driver.KeyTab,       // Tab
	0xfe20: driver.KeyTab,       // ISO_Left_Tab (Shift+Tab)
	0xff0d: driver.KeyReturn,    // Return
	0xff8d: driver.KeyReturn,    // KP_Enter
	0xff1b: driver.KeyEscape,    // Escape
	0x0020: driver.KeySpace,     // space
	0xff80: driver.KeySpace,     // KP_Space
	0xff55: driver.KeyPageUp,    // Page_Up (Prior)
	0xff56: driver.KeyPageDown,  // Page_Down (Next)
	0xff57: driver.KeyEnd,       // End
	0xff50: driver.KeyHome,      // Home
	0xff51: driver.KeyLeft,      // Left
	0xff52: driver.KeyUp,        // Up
	0xff53: driver.KeyRight,     // Right
	0xff54: driver.KeyDown,      // Down
	0xffff: driver.KeyDelete,    // Delete
	0xff95: driver.KeyHome,      // KP_Home
	0xff96: driver.KeyLeft,      // KP_Left
	0xff97: driver.KeyUp,        // KP_Up
	0xff98: driver.KeyRight,     // KP_Right
	0xff99: driver.KeyDown,      // KP_Down
	0xff9a: driver.KeyPageUp,    // KP_Page_Up
	0xff9b: driver.KeyPageDown,  // KP_Page_Down
	0xff9c: driver.KeyEnd,       // KP_End
	0xff9f: driver.KeyDelete,    // KP_Delete
	0xffaa: '*',                 // KP_Multiply
	0xffab: '+',                 // KP_Add
	0xffac: ',',                 // KP_Separator
	0xffad: '-',                 // KP_Subtract
	0xffae: '.',                 // KP_Decimal
	0xffaf: '/',                 // KP_Divide
	0xffbd: '=',                 // KP_Equal
}

// deadKeys maps the dead keyvals (dead_grave …) to the characters they
// stand for, so a dead key is the key of its character, as on Win32.
var deadKeys = map[uint32]rune{
	0xfe50: '`', 0xfe51: '´', 0xfe52: '^', 0xfe53: '~', 0xfe54: '¯', 0xfe55: '˘',
	0xfe56: '˙', 0xfe57: '¨', 0xfe58: '˚', 0xfe59: '˝', 0xfe5a: 'ˇ', 0xfe5b: '¸',
	0xfe5c: '˛',
}

// The keyval ranges keyFor tests.
const (
	keyvalF1       = 0xffbe
	keyvalF12      = 0xffc9
	keyvalF35      = 0xffe0
	keyvalKP0      = 0xffb0
	keyvalKP9      = 0xffb9
	keyvalSpecials = 0xfe00 // from here to 0xffff: function, modifier and dead keys
)

// namedKey returns the key of a named keyval: keyKeys, F1–F12 and the
// keypad digits.
func namedKey(kv uint32) (driver.Key, bool) {
	if k, ok := keyKeys[kv]; ok {
		return k, true
	}
	switch {
	case kv >= keyvalF1 && kv <= keyvalF12:
		return driver.KeyF1 + driver.Key(kv-keyvalF1), true
	case kv >= keyvalKP0 && kv <= keyvalKP9:
		return driver.Key('0' + kv - keyvalKP0), true
	}
	return driver.KeyNone, false
}

// modifierKey returns the modifier a modifier key's keyval stands for
// (0 for other keys), as modifiersOf maps the state.
func modifierKey(kv uint32) driver.Modifiers {
	switch kv {
	case 0xffe1, 0xffe2: // Shift_L, Shift_R
		return driver.ModShift
	case 0xffe3, 0xffe4: // Control_L, Control_R
		return driver.ModControl
	case 0xffe7, 0xffe8, 0xffe9, 0xffea: // Meta_L, Meta_R, Alt_L, Alt_R
		return driver.ModOption
	case 0xffeb, 0xffec, 0xffed, 0xffee: // Super_L, Super_R, Hyper_L, Hyper_R
		return driver.ModCommand
	}
	return 0
}

// keyModifier returns KeyEvent.Modifier (S5.6i) of a key event with keyval
// kv and level-0 keyval base: the modifier of the key's base keyval (Alt_L
// for the Meta_L that Shift+Alt gives), else of kv, so at most one.
func keyModifier(kv, base uint32) driver.Modifiers {
	if m := modifierKey(base); m != 0 {
		return m
	}
	return modifierKey(kv)
}

// isControl reports whether r is a control character (C0, DEL, C1).
func isControl(r rune) bool { return r < 0x20 || r == 0x7F || (r >= 0x80 && r < 0xA0) }

// keyFor returns the driver.Key of a key event with keyval kv (as the
// event reports it, modifiers applied) and base, the keyval at level 0 of
// the same key in the active layout (0 if unknown). toUnicode is
// gdk_keyval_to_unicode (0 for none).
func keyFor(kv, base uint32, toUnicode func(uint32) rune) driver.Key {
	if k, ok := namedKey(kv); ok {
		return k
	}
	if k, ok := namedKey(base); ok && base >= keyvalSpecials && base <= 0xffff {
		return k // the modifiers made it another keyval (Ctrl+Alt+F1: XF86Switch_VT_1)
	}
	k := base
	if k == 0 {
		k = kv
	}
	if r, ok := deadKeys[k]; ok {
		return driver.Key(r)
	}
	if k >= keyvalSpecials && k <= 0xffff {
		return driver.KeyNone // modifiers, F13 and up, Insert, the other function keys
	}
	if k == 0x20 {
		return driver.KeySpace
	}
	r := toUnicode(k)
	if r == 0 || isControl(r) || !utf8.ValidRune(r) {
		return driver.KeyNone
	}
	return driver.NormalizeKey(driver.Key(r))
}

// shiftedFor returns KeyEvent.Shifted (S5.6h) of a key-down with keyval
// kv and base keyval base (as keyFor): for a character key, the
// character of shifted, the keyval of the same key at the Shift level of
// the active layout (0 if unknown), a dead keyval as its character;
// KeyNone for the named keys, the keypad, modifiers and function keys, and
// when shifted is no character.
func shiftedFor(kv, base, shifted uint32, toUnicode func(uint32) rune) driver.Key {
	if _, named := namedKey(kv); named || base == 0 || base == 0x20 {
		return driver.KeyNone
	}
	if _, named := namedKey(base); named {
		return driver.KeyNone
	}
	if _, dead := deadKeys[base]; !dead && base >= keyvalSpecials && base <= 0xffff {
		return driver.KeyNone
	}
	if r, ok := deadKeys[shifted]; ok {
		return driver.Key(r)
	}
	if shifted == 0 || shifted == 0x20 || (shifted >= keyvalSpecials && shifted <= 0xffff) {
		return driver.KeyNone
	}
	r := toUnicode(shifted)
	if r == 0 || isControl(r) || !utf8.ValidRune(r) {
		return driver.KeyNone
	}
	return driver.NormalizeKey(driver.Key(r))
}

// commitText returns the text of an input-method commit with the control
// characters dropped and invalid UTF-8 replaced, or "".
func commitText(s string) string {
	s = strings.ToValidUTF8(s, "�")
	return strings.Map(func(r rune) rune {
		if isControl(r) {
			return -1
		}
		return r
	}, s)
}

// keyState is a surface's keyboard.
type keyState struct {
	down   map[uint32]bool // the keycodes held, for Repeat
	evTime uint32          // the time of the key event being handled (for its text)
	// last is the key event handled last and its result: during a popup's
	// grab both the window's and the popover's key controller may see one
	// (popover_linux.go), and the second gets the first's result.
	last    keySeen
	lastSet bool
}

// keySeen identifies a key event: its time, keycode and direction (a
// repeat has a time of its own).
type keySeen struct {
	t, keycode uint32
	down       bool
	result     bool
}

// seen reports whether the key event t, keycode, down was handled already
// and its result; otherwise it records it. Events without a time are
// never the same.
func (k *keyState) seen(t, keycode uint32, down bool) (bool, bool) {
	if t != 0 && k.lastSet && k.last.t == t && k.last.keycode == keycode && k.last.down == down {
		return true, k.last.result
	}
	k.last, k.lastSet = keySeen{t: t, keycode: keycode, down: down}, true
	return false, false
}

// keyLive reports whether key input may reach the handler (not while a
// modal surface blocks it, modal.go).
func (sf *surface) keyLive() bool {
	return !sf.dead && !sf.creating && sf.w != nil && !sf.inputBlocked()
}

// keyPressed is the key controller's "key-pressed": the KeyEvent, then,
// unless the handler consumed it, the input method. It returns whether
// the key was consumed (by the handler or the input method). While the
// key controller has the input method (the text input on, ime.go), GTK
// filtered the key through it already and it declined, so it is not
// filtered or reset again; the same holds when the handler turned the
// text input on (the key would reach the input method a second time if
// IBus put it back).
func (sf *surface) keyPressed(c uintptr, kv, keycode, state uint32) bool {
	t := sf.w.eventTime(c)
	if dup, r := sf.kbd.seen(t, keycode, true); dup {
		return r
	}
	r := sf.keyPressedOnce(c, t, kv, keycode, state)
	sf.kbd.last.result = r
	return r
}

func (sf *surface) keyPressedOnce(c uintptr, t, kv, keycode, state uint32) bool {
	if sf.kbd.down == nil {
		sf.kbd.down = map[uint32]bool{}
	}
	repeat := sf.kbd.down[keycode]
	sf.kbd.down[keycode] = true
	sf.kbd.evTime = t
	defer func() { sf.kbd.evTime = 0 }() // a later commit is not this key's
	viaIM := sf.ime.attached
	base := sf.w.keyBase(c, keycode)
	key := keyFor(kv, base, sf.w.keyvalToUnicode)
	shifted := shiftedFor(kv, base, sf.w.keyShifted(c, keycode), sf.w.keyvalToUnicode)
	modifier := keyModifier(kv, base)
	mods := modifiersOf(state) | modifier // the state is before the key
	handled := sf.keySink().Event(shell.KeyEvent{Down: true, Key: key, Shifted: shifted,
		Modifier: modifier, Repeat: repeat, Mods: mods, Time: sf.s.eventTime(t)})
	switch {
	case sf.dead:
		return true
	case viaIM || sf.ime.attached:
		return handled
	case handled:
		sf.w.imReset() // a consumed key produces no text
		return true
	}
	return sf.w.imFilter(c)
}

// keyReleased is the key controller's "key-released": the KeyEvent, then
// the input method (unless it had the key first, as in keyPressed).
func (sf *surface) keyReleased(c uintptr, kv, keycode, state uint32) {
	t := sf.w.eventTime(c)
	if dup, _ := sf.kbd.seen(t, keycode, false); dup {
		return
	}
	delete(sf.kbd.down, keycode)
	sf.kbd.evTime = t
	defer func() { sf.kbd.evTime = 0 }() // a later commit is not this key's
	viaIM := sf.ime.attached
	base := sf.w.keyBase(c, keycode)
	key := keyFor(kv, base, sf.w.keyvalToUnicode)
	modifier := keyModifier(kv, base)
	mods := modifiersOf(state) &^ modifier
	sf.keySink().Event(shell.KeyEvent{Key: key, Modifier: modifier, Mods: mods, Time: sf.s.eventTime(t)})
	if !sf.dead && !viaIM && !sf.ime.attached {
		sf.w.imFilter(c)
	}
}

// commit is the input method's "commit" of the text s: a TextEvent, at
// the time of the key event that produced it (textTime). Commits are
// dropped while muted (ime.go).
func (sf *surface) commit(s string) {
	if s = commitText(s); s == "" || sf.ime.mute > 0 {
		return
	}
	sf.ime.composing = false // a commit during a composition empties the preedit
	sf.keySink().Event(shell.TextEvent{Text: s, Time: sf.textTime()})
}

// keyFocus follows the key window: on the way out the keys held are
// forgotten, the composition too, and the input method is reset; the
// input method has the focus only in the key window with the text input
// on (syncIMFocus). Muted: aqua commits its preedit on the key loss.
func (sf *surface) keyFocus(in bool) {
	sf.muted(func() {
		if !in {
			clear(sf.kbd.down)
			sf.ime.composing = false
			sf.w.imReset()
		}
		sf.syncIMFocus()
	})
}
