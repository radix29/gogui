package gogui

import "github.com/radix29/gogui/internal/driver"

// Modifiers is a set of modifier keys, such as KeyPrimary|KeyShift.
// Resolve(mac) returns the set with KeyPrimary replaced by the physical
// key: KeyCommand when mac is true, KeyControl otherwise.
type Modifiers = driver.Modifiers

// Modifier keys. KeyPrimary is the one to use for shortcuts: it means ⌘ on
// macOS and Ctrl on Windows and Linux. KeyCommand is ⌘ on macOS and the
// Windows/Super key elsewhere.
const (
	KeyPrimary = driver.ModPrimary
	KeyShift   = driver.ModShift
	KeyOption  = driver.ModOption // ⌥ Option; Alt on Windows and Linux
	KeyControl = driver.ModControl
	KeyCommand = driver.ModCommand
)

// Key identifies a key in a shortcut. Character keys are their Unicode
// code point ('S', '+'); letters are case-insensitive (Shortcut stores them
// in upper case, and Shift is a separate modifier). Keys that produce no
// character have the constants below. IsFunctionKey reports whether a key
// is one of KeyF1 … KeyF12.
type Key = driver.Key

// Keys that are not plain characters.
const (
	KeyBackspace = driver.KeyBackspace // delete backward (⌫)
	KeyTab       = driver.KeyTab
	KeyReturn    = driver.KeyReturn
	KeyEscape    = driver.KeyEscape
	KeySpace     = driver.KeySpace
	KeyUp        = driver.KeyUp
	KeyDown      = driver.KeyDown
	KeyLeft      = driver.KeyLeft
	KeyRight     = driver.KeyRight
	KeyDelete    = driver.KeyDelete // delete forward (⌦)
	KeyHome      = driver.KeyHome
	KeyEnd       = driver.KeyEnd
	KeyPageUp    = driver.KeyPageUp
	KeyPageDown  = driver.KeyPageDown

	KeyF1  = driver.KeyF1
	KeyF2  = driver.KeyF1 + 1
	KeyF3  = driver.KeyF1 + 2
	KeyF4  = driver.KeyF1 + 3
	KeyF5  = driver.KeyF1 + 4
	KeyF6  = driver.KeyF1 + 5
	KeyF7  = driver.KeyF1 + 6
	KeyF8  = driver.KeyF1 + 7
	KeyF9  = driver.KeyF1 + 8
	KeyF10 = driver.KeyF1 + 9
	KeyF11 = driver.KeyF1 + 10
	KeyF12 = driver.KeyF12
)

// KeyShortcut is a key equivalent such as ⌘C or Ctrl+Shift+D: the
// modifiers Mods and the Key. The zero KeyShortcut means "no shortcut";
// IsZero reports it. Display(mac) returns the text a menu shows next to
// the item: "⇧⌘D" for mac true, "Ctrl+Shift+D" for mac false, "" for the
// zero shortcut. String returns the macOS form, for debugging. Resolve(mac)
// returns the shortcut with KeyPrimary replaced as Modifiers.Resolve does.
// Build shortcuts with Shortcut, which normalizes the key.
type KeyShortcut = driver.Shortcut

// Shortcut returns the shortcut mods+key, with a lower-case letter key
// turned into upper case: Shortcut(KeyPrimary, 's') equals
// Shortcut(KeyPrimary, 'S').
func Shortcut(mods Modifiers, key Key) KeyShortcut {
	return driver.NewShortcut(mods, key)
}

// The standard shortcuts below are the ones the standard menu items use.
// They are written with KeyPrimary, so they resolve per platform
// (ShortcutCopy() is ⌘C on macOS and Ctrl+C on Windows and Linux). They
// are functions, not variables, so no code can change them (A8).

// ShortcutUndo returns ⌘Z (Ctrl+Z).
func ShortcutUndo() KeyShortcut { return driver.ItemUndo.DefaultShortcut() }

// ShortcutRedo returns ⇧⌘Z (Ctrl+Shift+Z).
func ShortcutRedo() KeyShortcut { return driver.ItemRedo.DefaultShortcut() }

// ShortcutCut returns ⌘X (Ctrl+X).
func ShortcutCut() KeyShortcut { return driver.ItemCut.DefaultShortcut() }

// ShortcutCopy returns ⌘C (Ctrl+C).
func ShortcutCopy() KeyShortcut { return driver.ItemCopy.DefaultShortcut() }

// ShortcutPaste returns ⌘V (Ctrl+V).
func ShortcutPaste() KeyShortcut { return driver.ItemPaste.DefaultShortcut() }

// ShortcutSelectAll returns ⌘A (Ctrl+A).
func ShortcutSelectAll() KeyShortcut { return driver.ItemSelectAll.DefaultShortcut() }

// ShortcutHide returns ⌘H (Ctrl+H).
func ShortcutHide() KeyShortcut { return driver.ItemHide.DefaultShortcut() }

// ShortcutHideOthers returns ⌥⌘H (Ctrl+Alt+H).
func ShortcutHideOthers() KeyShortcut { return driver.ItemHideOthers.DefaultShortcut() }

// ShortcutQuit returns ⌘Q (Ctrl+Q).
func ShortcutQuit() KeyShortcut { return driver.ItemQuit.DefaultShortcut() }

// ShortcutMinimize returns ⌘M (Ctrl+M).
func ShortcutMinimize() KeyShortcut { return driver.ItemMinimize.DefaultShortcut() }

// ShortcutClose returns ⌘W (Ctrl+W).
func ShortcutClose() KeyShortcut { return driver.ItemClose.DefaultShortcut() }
