//go:build windows

package win32

import (
	"unicode/utf16"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// Keyboard input (S2.3e). Windows sends key messages to the focus window,
// which is the surface while it is the key window:
//
//   - WM_KEYDOWN/UP and WM_SYSKEYDOWN/UP (the SYS forms come with Alt held,
//     and for F10) are KeyEvents. The key is keyFor's: the named keys from
//     a VK table, character keys as the unshifted character of the active
//     layout (MapVirtualKeyExW, which leaves the dead-key state alone,
//     unlike ToUnicodeEx), KeyNone for the rest (modifier keys alone,
//     Insert, F13 and up). Repeat is lParam bit 30, the previous key state.
//     A key-down of a character key also carries Shifted (S5.6h), the
//     character the layout gives it with Shift alone: ToUnicodeEx over a
//     Shift-only keyboard state with TOUNICODE_NOSTATE (Windows 10 1607
//     on, below D7's floor), so a pending dead key is not consumed.
//     A modifier key alone carries its Modifier (modifierFor, S5.6i),
//     on the press and the release, so the aqua menu strip tells a lone
//     Alt from Insert or F13.
//     A key that went to the IME (VK_PROCESSKEY) is not delivered; the
//     composition arrives as WM_IME_* messages (ime.go).
//   - The message loop's TranslateMessage has already posted the key's
//     WM_CHAR when the key-down arrives. If Event returns true, the key was
//     consumed: its pending WM_CHAR and WM_SYSCHAR messages are removed
//     (PeekMessageW with PM_REMOVE; only this key's are in the posted
//     queue, as later keys are not translated yet), and DefWindowProcW does
//     not see the key-down nor, later, its key-up, so a consumed Alt or F10
//     does not enter the system-menu mode. If Event returns false, both go
//     to DefWindowProcW: Alt+F4 closes the surface through CloseRequested.
//   - WM_CHAR is a TextEvent. A surrogate pair (two WM_CHARs) is joined,
//     a lone surrogate dropped, and control characters (C0, DEL, C1) are
//     dropped, as Return, Tab, Backspace and Escape arrive as KeyEvents.
//   - WM_SYSCHAR (a character typed with Alt) is not text on Windows. Alt+
//     Space goes on to DefWindowProcW, which opens the system menu; the
//     others are swallowed, because DefWindowProcW would look for a native
//     menu mnemonic and beep (the aqua menu strip handles its own, S3).
//
// Windows sends no key-ups for keys still held when the focus moves to
// another window, and neither does the shell: the aqua driver must not
// rely on seeing every release (KeyChanged(false) is the signal).

// vkKeys maps the virtual keys that name a driver.Key. The numeric keypad
// gives its characters; VK_RETURN covers both Enter keys.
var vkKeys = map[uint32]driver.Key{
	w32.VK_BACK:     driver.KeyBackspace,
	w32.VK_TAB:      driver.KeyTab,
	w32.VK_RETURN:   driver.KeyReturn,
	w32.VK_ESCAPE:   driver.KeyEscape,
	w32.VK_SPACE:    driver.KeySpace,
	w32.VK_PRIOR:    driver.KeyPageUp,
	w32.VK_NEXT:     driver.KeyPageDown,
	w32.VK_END:      driver.KeyEnd,
	w32.VK_HOME:     driver.KeyHome,
	w32.VK_LEFT:     driver.KeyLeft,
	w32.VK_UP:       driver.KeyUp,
	w32.VK_RIGHT:    driver.KeyRight,
	w32.VK_DOWN:     driver.KeyDown,
	w32.VK_DELETE:   driver.KeyDelete,
	w32.VK_MULTIPLY: '*',
	w32.VK_ADD:      '+',
	w32.VK_SUBTRACT: '-',
	w32.VK_DECIMAL:  '.',
	w32.VK_DIVIDE:   '/',
}

// noCharKeys are the virtual keys that are never a character key, so
// keyFor does not ask the layout about them.
var noCharKeys = map[uint32]bool{
	w32.VK_SHIFT: true, w32.VK_CONTROL: true, w32.VK_MENU: true, w32.VK_CAPITAL: true,
	w32.VK_LSHIFT: true, w32.VK_RSHIFT: true, w32.VK_LCONTROL: true, w32.VK_RCONTROL: true,
	w32.VK_LMENU: true, w32.VK_RMENU: true, w32.VK_LWIN: true, w32.VK_RWIN: true,
	w32.VK_APPS: true, w32.VK_NUMLOCK: true, w32.VK_SCROLL: true, w32.VK_PAUSE: true,
	w32.VK_SNAPSHOT: true, w32.VK_INSERT: true, w32.VK_CLEAR: true, w32.VK_HELP: true,
	w32.VK_PACKET: true, w32.VK_PROCESSKEY: true,
}

// mapDeadKey is the bit MAPVK_VK_TO_CHAR sets for a dead key.
const mapDeadKey = 0x80000000

// keyFor returns the driver.Key of virtual key vk. layoutChar is
// MapVirtualKeyExW(vk, MAPVK_VK_TO_CHAR) of the active layout: the
// unshifted character in the low word (the dead-key bit set for a dead
// key, whose character is the key all the same), or 0.
func keyFor(vk uint32, layoutChar func(vk uint32) uint32) driver.Key {
	if k, ok := vkKeys[vk]; ok {
		return k
	}
	switch {
	case vk >= w32.VK_F1 && vk <= w32.VK_F12:
		return driver.KeyF1 + driver.Key(vk-w32.VK_F1)
	case vk >= w32.VK_NUMPAD0 && vk <= w32.VK_NUMPAD9:
		return driver.Key('0' + vk - w32.VK_NUMPAD0)
	case !layoutKey(vk):
		return driver.KeyNone
	}
	return charKey(rune(layoutChar(vk) &^ mapDeadKey))
}

// layoutKey reports whether vk is a character key, whose character the
// keyboard layout gives (not a named key, the keypad, a function or a
// modifier key).
func layoutKey(vk uint32) bool {
	if _, ok := vkKeys[vk]; ok {
		return false
	}
	switch {
	case vk >= w32.VK_F1 && vk <= w32.VK_F24, vk >= w32.VK_NUMPAD0 && vk <= w32.VK_NUMPAD9,
		noCharKeys[vk], vk < 0x20:
		return false
	}
	return true
}

// charKey returns the key of a layout's character c, KeyNone for none
// and for control characters and surrogates.
func charKey(c rune) driver.Key {
	if c < 0x20 || c == 0x7F || (c >= 0x80 && c < 0xA0) || utf16.IsSurrogate(c) {
		return driver.KeyNone
	}
	return driver.NormalizeKey(driver.Key(c))
}

// shiftedFor returns KeyEvent.Shifted of virtual key vk with scan code
// scan (S5.6h): for a character key, the character the layout gives it
// with Shift alone, from shiftedChar (ToUnicodeEx), KeyNone otherwise.
func shiftedFor(vk, scan uint32, shiftedChar func(vk, scan uint32) rune) driver.Key {
	if !layoutKey(vk) {
		return driver.KeyNone
	}
	return charKey(shiftedChar(vk, scan))
}

// liveLayoutChar maps vk with the calling thread's keyboard layout.
func liveLayoutChar(vk uint32) uint32 {
	return w32.MapVirtualKeyEx(vk, w32.MAPVK_VK_TO_CHAR, w32.GetKeyboardLayout(0))
}

// liveShiftedChar is the character of vk with Shift alone held in the
// calling thread's keyboard layout (0 for none or more than one).
func liveShiftedChar(vk, scan uint32) rune {
	return shiftedCharIn(vk, scan, w32.GetKeyboardLayout(0))
}

// shiftedCharIn is the character of vk with Shift alone held in layout
// hkl: ToUnicodeEx over a keyboard state of Shift only (so Ctrl, Alt and
// Caps Lock do not change it) with TOUNICODE_NOSTATE, which leaves a
// pending dead key alone; a dead key gives its spacing character, as its
// Key does; 0 for no character or a ligature of several.
func shiftedCharIn(vk, scan uint32, hkl windows.Handle) rune {
	var state [256]byte
	state[w32.VK_SHIFT] = 0x80
	state[w32.VK_LSHIFT] = 0x80
	var buf [4]uint16
	switch w32.ToUnicodeEx(vk, scan, &state, buf[:], w32.TOUNICODE_NOSTATE, hkl) {
	case 1, -1:
		return rune(buf[0])
	}
	return 0
}

// scanCode returns the scan code of a key message's lParam (bits 16–23,
// the extended-key bit as 0xE000).
func scanCode(lp uintptr) uint32 {
	sc := uint32(lp>>16) & 0xFF
	if lp&(1<<24) != 0 {
		sc |= 0xE000
	}
	return sc
}

// isRepeat reports whether a key-down's lParam marks an auto-repeat
// (bit 30: the key was already down).
func isRepeat(lp uintptr) bool { return lp&(1<<30) != 0 }

// modifierFor returns the modifier a modifier key's virtual key stands for
// (S5.6i; 0 for other keys), as modifiersOf maps the key state: the
// generic VK_SHIFT, VK_CONTROL and VK_MENU of the key messages and their
// left and right forms, and the Windows keys. Under AltGr Windows sends a
// left Control and a right Alt, so AltGr is a Control and an Option key.
func modifierFor(vk uint32) driver.Modifiers {
	switch vk {
	case w32.VK_SHIFT, w32.VK_LSHIFT, w32.VK_RSHIFT:
		return driver.ModShift
	case w32.VK_CONTROL, w32.VK_LCONTROL, w32.VK_RCONTROL:
		return driver.ModControl
	case w32.VK_MENU, w32.VK_LMENU, w32.VK_RMENU:
		return driver.ModOption
	case w32.VK_LWIN, w32.VK_RWIN:
		return driver.ModCommand
	}
	return 0
}

// keyMsg handles the key messages. handled false sends the message on to
// DefWindowProcW.
func (sf *surface) keyMsg(msg uint32, wp, lp uintptr) (uintptr, bool) {
	vk := uint32(wp)
	if vk == w32.VK_PROCESSKEY || sf.dead {
		return 0, false
	}
	down := msg == w32.WM_KEYDOWN || msg == w32.WM_SYSKEYDOWN
	if !down {
		consumed := vk < 256 && sf.consumed[vk]
		if vk < 256 {
			sf.consumed[vk] = false
		}
		sf.keySink().Event(shell.KeyEvent{Key: keyFor(vk, liveLayoutChar), Modifier: modifierFor(vk), Mods: modifiers(),
			Time: sf.s.eventTime()})
		return 0, consumed
	}
	handled := sf.keySink().Event(shell.KeyEvent{Down: true, Key: keyFor(vk, liveLayoutChar),
		Shifted:  shiftedFor(vk, scanCode(lp), liveShiftedChar),
		Modifier: modifierFor(vk), Repeat: isRepeat(lp), Mods: modifiers(), Time: sf.s.eventTime()})
	if vk < 256 {
		sf.consumed[vk] = handled
	}
	if !handled || sf.dead {
		return 0, handled
	}
	sf.dropPendingChars()
	return 0, true
}

// dropPendingChars removes the character messages TranslateMessage posted
// for the key being handled.
func (sf *surface) dropPendingChars() {
	var m w32.Msg
	for w32.PeekMessage(&m, sf.hwnd, w32.WM_CHAR, w32.WM_DEADCHAR, w32.PM_REMOVE) ||
		w32.PeekMessage(&m, sf.hwnd, w32.WM_SYSCHAR, w32.WM_SYSDEADCHAR, w32.PM_REMOVE) {
		rethrow() // PeekMessageW delivers sent messages
	}
	rethrow()
	sf.highSurrogate = 0
}

// textUnit is the text-event logic of WM_CHAR: it takes one UTF-16 code
// unit and returns the text to deliver, or "" (control characters, the
// first half of a surrogate pair, a lone surrogate). high is the pending
// high surrogate, updated.
func textUnit(high *uint16, u uint16) string {
	r := rune(u)
	switch {
	case utf16.IsSurrogate(r) && u < 0xDC00: // high
		*high = u
		return ""
	case utf16.IsSurrogate(r): // low
		h := *high
		*high = 0
		if h == 0 {
			return ""
		}
		return string(utf16.DecodeRune(rune(h), r))
	}
	*high = 0
	if r < 0x20 || r == 0x7F || (r >= 0x80 && r < 0xA0) {
		return ""
	}
	return string(r)
}

// char handles WM_CHAR, WM_SYSCHAR and the dead-character messages.
func (sf *surface) char(msg uint32, wp uintptr) (uintptr, bool) {
	switch msg {
	case w32.WM_SYSCHAR:
		return 0, wp != ' ' // Alt+Space: the system menu
	case w32.WM_DEADCHAR, w32.WM_SYSDEADCHAR:
		return 0, false
	}
	if t := textUnit(&sf.highSurrogate, uint16(wp)); t != "" && !sf.dead {
		sf.keySink().Event(shell.TextEvent{Text: t, Time: sf.s.eventTime()})
	}
	return 0, true
}
