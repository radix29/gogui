package driver

// MenuBarSpec describes the whole menu bar. On macOS it becomes
// NSApp.mainMenu; on Windows and Linux the aqua driver draws it in the
// title-bar area of each titled window (PLAN.md §5.4.1).
//
// Specs are plain values. The driver copies what it needs; the public layer
// may reuse or change a spec after SetMenuBar returns.
type MenuBarSpec struct {
	Menus []MenuSpec
}

// MenuRole marks a menu with a special meaning to the OS.
type MenuRole uint8

const (
	MenuNone MenuRole = iota
	// MenuApp is the application menu. It must be the first menu. Its
	// title is ignored: the driver shows the app name (in bold on Windows
	// and Linux).
	MenuApp
	// MenuEdit is the Edit menu. Its role items act on the focused text
	// control.
	MenuEdit
	// MenuWindow is the Window menu (NSApp.windowsMenu).
	MenuWindow
	// MenuHelp is the Help menu (NSApp.helpMenu).
	MenuHelp
)

// MenuSpec describes one menu or submenu.
type MenuSpec struct {
	Title string
	Role  MenuRole
	Items []MenuItemSpec
}

// ItemRole marks a menu item that has a standard action. The driver
// provides the action, a default title and a default shortcut. A role
// item with a non-zero ID is reported through AppHandler.MenuItemSelected
// instead of running the standard action.
type ItemRole uint8

const (
	ItemNone ItemRole = iota

	// App menu.
	ItemAbout      // About Name
	ItemHide       // Hide Name, ⌘H
	ItemHideOthers // Hide Others, ⌥⌘H
	ItemShowAll    // Show All
	ItemQuit       // Quit Name, ⌘Q; calls AppHandler.QuitRequested

	// Edit menu: act on the focused text control.
	ItemUndo      // ⌘Z
	ItemRedo      // ⇧⌘Z
	ItemCut       // ⌘X
	ItemCopy      // ⌘C
	ItemPaste     // ⌘V
	ItemDelete    // no shortcut
	ItemSelectAll // ⌘A

	// Window menu: act on the key window.
	ItemMinimize // ⌘M
	ItemZoom     // no shortcut
	ItemClose    // ⌘W; calls WindowHandler.ShouldClose
)

// MenuItemSpec describes one menu item.
type MenuItemSpec struct {
	// ID identifies the item in AppHandler.MenuItemSelected. Zero means the
	// item is never reported (role items, separators, submenu titles).
	ID int

	// Title is the item text. For role items, an empty title selects the
	// driver's default title.
	Title string

	Role ItemRole

	// Shortcut is the key equivalent. For role items, the zero Shortcut
	// selects the standard shortcut.
	Shortcut Shortcut

	// Separator makes this item a separator line; all other fields are
	// ignored.
	Separator bool

	Disabled bool
	Checked  bool

	// Submenu, if not nil, makes this item open a submenu.
	Submenu *MenuSpec
}

// Clone returns a deep copy of b. It returns nil for a nil b.
func (b *MenuBarSpec) Clone() *MenuBarSpec {
	if b == nil {
		return nil
	}
	out := &MenuBarSpec{Menus: make([]MenuSpec, len(b.Menus))}
	for i := range b.Menus {
		out.Menus[i] = b.Menus[i].clone()
	}
	return out
}

func (m MenuSpec) clone() MenuSpec {
	items := make([]MenuItemSpec, len(m.Items))
	copy(items, m.Items)
	for i := range items {
		if sub := items[i].Submenu; sub != nil {
			c := sub.clone()
			items[i].Submenu = &c
		}
	}
	m.Items = items
	return m
}

// FindItem returns the item with the given non-zero ID, searching submenus
// depth-first, or nil if there is none.
func (b *MenuBarSpec) FindItem(id int) *MenuItemSpec {
	if b == nil || id == 0 {
		return nil
	}
	for i := range b.Menus {
		if it := b.Menus[i].FindItem(id); it != nil {
			return it
		}
	}
	return nil
}

// FindItem returns the item with the given non-zero ID in m or its
// submenus, or nil if there is none.
func (m *MenuSpec) FindItem(id int) *MenuItemSpec {
	if id == 0 {
		return nil
	}
	for i := range m.Items {
		it := &m.Items[i]
		if it.ID == id && !it.Separator {
			return it
		}
		if it.Submenu != nil {
			if found := it.Submenu.FindItem(id); found != nil {
				return found
			}
		}
	}
	return nil
}

// HasMenu reports whether b has a top-level menu with the given role.
func (b *MenuBarSpec) HasMenu(role MenuRole) bool {
	if b == nil {
		return false
	}
	for _, m := range b.Menus {
		if m.Role == role {
			return true
		}
	}
	return false
}

// DefaultAppMenu returns the standard application menu: About, Hide,
// Hide Others, Show All and Quit.
func DefaultAppMenu() MenuSpec {
	return MenuSpec{Role: MenuApp, Items: []MenuItemSpec{
		{Role: ItemAbout},
		{Separator: true},
		{Role: ItemHide},
		{Role: ItemHideOthers},
		{Role: ItemShowAll},
		{Separator: true},
		{Role: ItemQuit},
	}}
}

// DefaultEditMenu returns the standard Edit menu: Undo, Redo, Cut, Copy,
// Paste, Delete and Select All.
func DefaultEditMenu() MenuSpec {
	return MenuSpec{Title: "Edit", Role: MenuEdit, Items: []MenuItemSpec{
		{Role: ItemUndo},
		{Role: ItemRedo},
		{Separator: true},
		{Role: ItemCut},
		{Role: ItemCopy},
		{Role: ItemPaste},
		{Role: ItemDelete},
		{Role: ItemSelectAll},
	}}
}

// DefaultMenuBar returns the menu bar installed by Driver.SetMenuBar(nil):
// the app menu and the Edit menu.
func DefaultMenuBar() *MenuBarSpec {
	return &MenuBarSpec{Menus: []MenuSpec{DefaultAppMenu(), DefaultEditMenu()}}
}

// DefaultTitle returns the standard title of a role item. appName replaces
// "Name" in the app menu titles. It returns "" for ItemNone.
func (r ItemRole) DefaultTitle(appName string) string {
	switch r {
	case ItemAbout:
		return "About " + appName
	case ItemHide:
		return "Hide " + appName
	case ItemHideOthers:
		return "Hide Others"
	case ItemShowAll:
		return "Show All"
	case ItemQuit:
		return "Quit " + appName
	case ItemUndo:
		return "Undo"
	case ItemRedo:
		return "Redo"
	case ItemCut:
		return "Cut"
	case ItemCopy:
		return "Copy"
	case ItemPaste:
		return "Paste"
	case ItemDelete:
		return "Delete"
	case ItemSelectAll:
		return "Select All"
	case ItemMinimize:
		return "Minimize"
	case ItemZoom:
		return "Zoom"
	case ItemClose:
		return "Close"
	}
	return ""
}

// DefaultShortcut returns the standard shortcut of a role item, with
// ModPrimary unresolved. It returns the zero Shortcut for roles without
// one.
func (r ItemRole) DefaultShortcut() Shortcut {
	p := ModPrimary
	switch r {
	case ItemHide:
		return Shortcut{p, 'H'}
	case ItemHideOthers:
		return Shortcut{p | ModOption, 'H'}
	case ItemQuit:
		return Shortcut{p, 'Q'}
	case ItemUndo:
		return Shortcut{p, 'Z'}
	case ItemRedo:
		return Shortcut{p | ModShift, 'Z'}
	case ItemCut:
		return Shortcut{p, 'X'}
	case ItemCopy:
		return Shortcut{p, 'C'}
	case ItemPaste:
		return Shortcut{p, 'V'}
	case ItemSelectAll:
		return Shortcut{p, 'A'}
	case ItemMinimize:
		return Shortcut{p, 'M'}
	case ItemClose:
		return Shortcut{p, 'W'}
	}
	return Shortcut{}
}

// EffectiveTitle returns the item title, or the role's default title when
// the title is empty.
func (it MenuItemSpec) EffectiveTitle(appName string) string {
	if it.Title != "" {
		return it.Title
	}
	return it.Role.DefaultTitle(appName)
}

// EffectiveShortcut returns the item shortcut, or the role's default
// shortcut when the shortcut is zero.
func (it MenuItemSpec) EffectiveShortcut() Shortcut {
	if !it.Shortcut.IsZero() {
		return it.Shortcut
	}
	return it.Role.DefaultShortcut()
}
