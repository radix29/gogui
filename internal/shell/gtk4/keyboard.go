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
// creation, and a GtkIMContextSimple for the text:
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
//   - Text. A key-down the handler did not consume goes to the input method
//     (gtk_im_context_filter_keypress of the current event), which commits
//     the text it produces ("commit"), dead keys and compose sequences
//     included; the commit is a TextEvent, with control characters (C0,
//     DEL, C1) dropped, so it follows its KeyEvent. A consumed key-down
//     resets the input method instead, so it produces no text. Key-ups go
//     to the input method too (some sequences end on a release). The
//     input method is GTK's simple one until the full IME (S5.3).
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
}

// keyLive reports whether key input may reach the handler.
func (sf *surface) keyLive() bool { return !sf.dead && !sf.creating && sf.w != nil }

// keyPressed is the key controller's "key-pressed": the KeyEvent, then,
// unless the handler consumed it, the input method. It returns whether
// the key was consumed (by the handler or the input method).
func (sf *surface) keyPressed(c uintptr, kv, keycode, state uint32) bool {
	if sf.kbd.down == nil {
		sf.kbd.down = map[uint32]bool{}
	}
	repeat := sf.kbd.down[keycode]
	sf.kbd.down[keycode] = true
	t := sf.w.eventTime(c)
	sf.kbd.evTime = t
	defer func() { sf.kbd.evTime = 0 }() // a later commit is not this key's
	base := sf.w.keyBase(c, keycode)
	key := keyFor(kv, base, sf.w.keyvalToUnicode)
	mods := modifiersOf(state) | modifierKey(kv) | modifierKey(base) // the state is before the key
	handled := sf.h.Event(shell.KeyEvent{Down: true, Key: key, Repeat: repeat,
		Mods: mods, Time: sf.s.eventTime(t)})
	if sf.dead {
		return true
	}
	if handled {
		sf.w.imReset() // a consumed key produces no text
		return true
	}
	return sf.w.imFilter(c)
}

// keyReleased is the key controller's "key-released": the KeyEvent, then
// the input method.
func (sf *surface) keyReleased(c uintptr, kv, keycode, state uint32) {
	delete(sf.kbd.down, keycode)
	t := sf.w.eventTime(c)
	sf.kbd.evTime = t
	defer func() { sf.kbd.evTime = 0 }() // a later commit is not this key's
	base := sf.w.keyBase(c, keycode)
	key := keyFor(kv, base, sf.w.keyvalToUnicode)
	mods := modifiersOf(state) &^ (modifierKey(kv) | modifierKey(base))
	sf.h.Event(shell.KeyEvent{Key: key, Mods: mods, Time: sf.s.eventTime(t)})
	if !sf.dead {
		sf.w.imFilter(c)
	}
}

// commit is the input method's "commit" of the text s: a TextEvent, at
// the time of the key event that produced it (now outside one).
func (sf *surface) commit(s string) {
	if s = commitText(s); s == "" {
		return
	}
	sf.h.Event(shell.TextEvent{Text: s, Time: sf.s.eventTime(sf.kbd.evTime)})
}

// keyFocus follows the key window: the input method gets focus in or out,
// and on the way out the keys held are forgotten and the input method is
// reset.
func (sf *surface) keyFocus(in bool) {
	if !in {
		clear(sf.kbd.down)
		sf.w.imReset()
	}
	sf.w.imFocus(in)
}
