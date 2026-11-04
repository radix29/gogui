package gogui

import "github.com/radix29/gogui/internal/driver"

// Modifiers is a set of modifier keys, such as KeyPrimary|KeyShift.
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
// character have the constants below.
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

// KeyShortcut is a key equivalent such as ⌘C or Ctrl+Shift+D. The zero
// KeyShortcut means "no shortcut". Display(mac) returns its menu text.
type KeyShortcut = driver.Shortcut

// Shortcut returns the shortcut mods+key, with a lower-case letter key
// turned into upper case: Shortcut(KeyPrimary, 's') equals
// Shortcut(KeyPrimary, 'S').
func Shortcut(mods Modifiers, key Key) KeyShortcut {
	return driver.NewShortcut(mods, key)
}

// Standard shortcuts, the same as the standard menu items use. They are
// written with KeyPrimary, so they resolve per platform (ShortcutCopy is
// ⌘C on macOS and Ctrl+C on Windows and Linux).
var (
	ShortcutUndo       = driver.ItemUndo.DefaultShortcut()       // ⌘Z
	ShortcutRedo       = driver.ItemRedo.DefaultShortcut()       // ⇧⌘Z
	ShortcutCut        = driver.ItemCut.DefaultShortcut()        // ⌘X
	ShortcutCopy       = driver.ItemCopy.DefaultShortcut()       // ⌘C
	ShortcutPaste      = driver.ItemPaste.DefaultShortcut()      // ⌘V
	ShortcutSelectAll  = driver.ItemSelectAll.DefaultShortcut()  // ⌘A
	ShortcutHide       = driver.ItemHide.DefaultShortcut()       // ⌘H
	ShortcutHideOthers = driver.ItemHideOthers.DefaultShortcut() // ⌥⌘H
	ShortcutQuit       = driver.ItemQuit.DefaultShortcut()       // ⌘Q
	ShortcutMinimize   = driver.ItemMinimize.DefaultShortcut()   // ⌘M
	ShortcutClose      = driver.ItemClose.DefaultShortcut()      // ⌘W
)
