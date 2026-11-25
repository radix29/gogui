package menubar

import "github.com/radix29/gogui/internal/driver"

// Bar is a built menu bar: the menus in strip order.
type Bar struct {
	Menus []Menu
}

// Menu is one top-level menu of the strip.
type Menu struct {
	// Title is the effective title: the app name for the app menu, the
	// spec's title otherwise.
	Title string
	// Role is the spec's role. The strip draws the MenuApp title in bold.
	Role  driver.MenuRole
	Items []Item
}

// Item is one item of a menu.
type Item struct {
	// ID is the spec's ID: non-zero items are reported through
	// driver.AppHandler.MenuItemSelected.
	ID int
	// Title is the effective title (the role's default when the spec's is
	// empty). Empty for a separator.
	Title string
	Role  driver.ItemRole
	// Shortcut is the effective shortcut resolved for Windows and Linux
	// (ModPrimary is ModControl, the key normalized), zero for none.
	Shortcut driver.Shortcut
	// ShortcutText is the shortcut's display text ("Ctrl+Shift+D"), "" for
	// none.
	ShortcutText string

	Separator bool
	// Disabled is the spec's flag; a submenu title is always disabled.
	// Role items are further validated by the driver when a menu opens and
	// when an accelerator fires (S5.6g).
	Disabled bool
	Checked  bool
	// Submenu marks the title of a submenu: drawn with a chevron, never
	// enabled, without a shortcut.
	Submenu bool
}

// Ref names an item of a Bar by its menu and item indexes.
type Ref struct {
	Menu, Item int
}

// Build returns the bar of spec. appName titles the app menu and replaces
// "Name" in the app menu's role titles. A nil spec gives a nil Bar (no
// strip): the driver passes driver.DefaultMenuBar for SetMenuBar(nil).
func Build(spec *driver.MenuBarSpec, appName string) *Bar {
	if spec == nil {
		return nil
	}
	b := &Bar{Menus: make([]Menu, len(spec.Menus))}
	for i, ms := range spec.Menus {
		m := Menu{Title: ms.Title, Role: ms.Role, Items: make([]Item, len(ms.Items))}
		if ms.Role == driver.MenuApp {
			m.Title = appName
		}
		for j, is := range ms.Items {
			m.Items[j] = buildItem(is, appName)
		}
		b.Menus[i] = m
	}
	return b
}

func buildItem(is driver.MenuItemSpec, appName string) Item {
	if is.Separator {
		return Item{Separator: true}
	}
	it := Item{
		ID:       is.ID,
		Title:    is.EffectiveTitle(appName),
		Role:     is.Role,
		Disabled: is.Disabled,
		Checked:  is.Checked,
	}
	if is.Submenu != nil {
		it.Submenu, it.Disabled = true, true
		return it
	}
	if sc := is.EffectiveShortcut(); !sc.IsZero() {
		it.Shortcut = sc.Resolve(false)
		it.ShortcutText = sc.Display(false)
	}
	return it
}

// Item returns the item r names, or nil when r is out of range.
func (b *Bar) Item(r Ref) *Item {
	if b == nil || r.Menu < 0 || r.Menu >= len(b.Menus) {
		return nil
	}
	items := b.Menus[r.Menu].Items
	if r.Item < 0 || r.Item >= len(items) {
		return nil
	}
	return &items[r.Item]
}

// Lookup returns the item a key-down triggers, the accelerator index of
// the strip. key is shell.KeyEvent.Key (the unshifted key in canonical
// form), mods the physical modifiers held (never ModPrimary), and shifted
// the character the key gives with Shift on the active layout (KeyNone when
// unknown; the shell provides it from S5.6h).
//
// An item matches when its shortcut is key with exactly mods, or (the
// shifted-shortcut rule) when its shortcut has no Shift, mods is its
// modifiers plus Shift, and its key is shifted while shifted is not just
// the upper case of key: so Primary+'+' matches Ctrl+Shift+'=' on a US
// layout and Ctrl+'+' on a layout with an unshifted '+', while Ctrl+Shift+A
// never matches Ctrl+A. Exact matches win over shifted ones; among either,
// the first item in menu order wins. Separators, submenu titles and items
// without a shortcut never match. Disabled items do match: the caller
// validates the item and, when it is disabled, passes the key on to the
// focus.
func (b *Bar) Lookup(key, shifted driver.Key, mods driver.Modifiers) (Ref, bool) {
	if b == nil || key == driver.KeyNone {
		return Ref{}, false
	}
	key = driver.NormalizeKey(key)
	if r, ok := b.find(func(s driver.Shortcut) bool { return s.Key == key && s.Mods == mods }); ok {
		return r, true
	}
	shifted = driver.NormalizeKey(shifted)
	if shifted == driver.KeyNone || shifted == key || mods&driver.ModShift == 0 {
		return Ref{}, false
	}
	base := mods &^ driver.ModShift
	return b.find(func(s driver.Shortcut) bool {
		return s.Key == shifted && s.Mods == base
	})
}

// find returns the first item in menu order whose shortcut satisfies
// match.
func (b *Bar) find(match func(driver.Shortcut) bool) (Ref, bool) {
	for i, m := range b.Menus {
		for j, it := range m.Items {
			if it.Separator || it.Submenu || it.Shortcut.IsZero() {
				continue
			}
			if match(it.Shortcut) {
				return Ref{Menu: i, Item: j}, true
			}
		}
	}
	return Ref{}, false
}
