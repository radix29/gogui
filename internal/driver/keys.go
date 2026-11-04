package driver

import (
	"strconv"
	"strings"
	"unicode"
)

// Modifiers is a set of modifier keys.
//
// ModPrimary is a logical modifier: ⌘ (ModCommand) on macOS and Ctrl
// (ModControl) on Windows and Linux (PLAN.md §4.2). Drivers call Resolve to
// turn it into a physical modifier before matching or displaying shortcuts.
type Modifiers uint8

const (
	ModShift   Modifiers = 1 << iota // ⇧ Shift
	ModControl                       // ⌃ Control / Ctrl
	ModOption                        // ⌥ Option / Alt
	ModCommand                       // ⌘ Command on macOS; the Windows/Super key elsewhere
	ModPrimary                       // logical: ⌘ on macOS, Ctrl elsewhere
)

// Resolve replaces ModPrimary with ModCommand when mac is true and with
// ModControl otherwise.
func (m Modifiers) Resolve(mac bool) Modifiers {
	if m&ModPrimary == 0 {
		return m
	}
	m &^= ModPrimary
	if mac {
		return m | ModCommand
	}
	return m | ModControl
}

// Key identifies a key in a shortcut.
//
// Character keys use their Unicode code point. Letters are always stored in
// upper case ('A', not 'a'); Shift is a separate modifier. Keys that produce
// no character use the code points of AppKit's NS…FunctionKey constants
// (Unicode private use area), so the appkit driver can pass them through.
type Key rune

// Keys that are not plain characters. The character values match AppKit
// (NSCarriageReturnCharacter, NSTabCharacter, NSBackspaceCharacter …) and
// the function-key values match NSUpArrowFunctionKey and friends.
const (
	KeyNone      Key = 0
	KeyBackspace Key = 0x08 // delete backward (⌫)
	KeyTab       Key = 0x09
	KeyReturn    Key = 0x0D
	KeyEscape    Key = 0x1B
	KeySpace     Key = 0x20

	KeyUp       Key = 0xF700
	KeyDown     Key = 0xF701
	KeyLeft     Key = 0xF702
	KeyRight    Key = 0xF703
	KeyF1       Key = 0xF704 // KeyF1 + n-1 is Fn, up to F12
	KeyF12      Key = 0xF70F
	KeyDelete   Key = 0xF728 // delete forward (⌦)
	KeyHome     Key = 0xF729
	KeyEnd      Key = 0xF72B
	KeyPageUp   Key = 0xF72C
	KeyPageDown Key = 0xF72D
)

// NormalizeKey returns k in canonical form: lower-case letters become upper
// case. All other keys are returned unchanged.
func NormalizeKey(k Key) Key {
	if k < KeyUp && unicode.IsLower(rune(k)) {
		return Key(unicode.ToUpper(rune(k)))
	}
	return k
}

// IsFunctionKey reports whether k is one of KeyF1 … KeyF12.
func (k Key) IsFunctionKey() bool { return k >= KeyF1 && k <= KeyF12 }

// Shortcut is a key equivalent such as ⌘C or Ctrl+Shift+D. The zero
// Shortcut means "no shortcut".
type Shortcut struct {
	Mods Modifiers
	Key  Key
}

// NewShortcut returns a shortcut with a normalized key.
func NewShortcut(mods Modifiers, key Key) Shortcut {
	return Shortcut{Mods: mods, Key: NormalizeKey(key)}
}

// IsZero reports whether s is "no shortcut".
func (s Shortcut) IsZero() bool { return s.Key == KeyNone }

// Resolve returns s with ModPrimary resolved for the platform and the key
// normalized.
func (s Shortcut) Resolve(mac bool) Shortcut {
	return Shortcut{Mods: s.Mods.Resolve(mac), Key: NormalizeKey(s.Key)}
}

// Display returns the text shown next to a menu item: "⇧⌘D" on macOS
// (mac == true) and "Ctrl+Shift+D" on Windows and Linux. It returns "" for
// the zero Shortcut.
func (s Shortcut) Display(mac bool) string {
	if s.IsZero() {
		return ""
	}
	r := s.Resolve(mac)
	var b strings.Builder
	if mac {
		// macOS order: ⌃ ⌥ ⇧ ⌘, then the key, without separators.
		for _, m := range []struct {
			bit   Modifiers
			glyph string
		}{{ModControl, "⌃"}, {ModOption, "⌥"}, {ModShift, "⇧"}, {ModCommand, "⌘"}} {
			if r.Mods&m.bit != 0 {
				b.WriteString(m.glyph)
			}
		}
		b.WriteString(keyName(r.Key, true))
		return b.String()
	}
	// Windows/Linux order: Ctrl+Alt+Shift+Super+Key.
	for _, m := range []struct {
		bit  Modifiers
		name string
	}{{ModControl, "Ctrl"}, {ModOption, "Alt"}, {ModShift, "Shift"}, {ModCommand, "Super"}} {
		if r.Mods&m.bit != 0 {
			b.WriteString(m.name)
			b.WriteByte('+')
		}
	}
	b.WriteString(keyName(r.Key, false))
	return b.String()
}

// String returns the macOS-style display form, for debugging.
func (s Shortcut) String() string { return s.Display(true) }

// specialKeyNames holds the display names of keys that are not printed as
// their character: the macOS glyph and the Windows/Linux name.
var specialKeyNames = map[Key]struct{ mac, other string }{
	KeyBackspace: {"⌫", "Backspace"},
	KeyTab:       {"⇥", "Tab"},
	KeyReturn:    {"↩", "Enter"},
	KeyEscape:    {"⎋", "Esc"},
	KeySpace:     {"Space", "Space"},
	KeyUp:        {"↑", "Up"},
	KeyDown:      {"↓", "Down"},
	KeyLeft:      {"←", "Left"},
	KeyRight:     {"→", "Right"},
	KeyDelete:    {"⌦", "Del"},
	KeyHome:      {"↖", "Home"},
	KeyEnd:       {"↘", "End"},
	KeyPageUp:    {"⇞", "PgUp"},
	KeyPageDown:  {"⇟", "PgDn"},
}

func keyName(k Key, mac bool) string {
	if n, ok := specialKeyNames[k]; ok {
		if mac {
			return n.mac
		}
		return n.other
	}
	if k.IsFunctionKey() {
		return "F" + strconv.Itoa(int(k-KeyF1)+1)
	}
	return string(rune(k))
}
