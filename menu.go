package gogui

import "github.com/radix29/gogui/internal/driver"

// MenuBarSpec describes the whole menu bar: MenuBar returns one and
// App.SetMenuBar installs it. On macOS it is the menu bar at the top of the
// screen; on Windows and Linux it is drawn in the title bar of each titled
// window, next to the window buttons.
//
// A spec is a plain value. It can be built before it is installed and
// installed again; App.SetMenuBar copies what it needs.
type MenuBarSpec struct {
	menus []MenuSpec
}

// MenuSpec describes one menu. Menu, AppMenu, EditMenu and WindowMenu
// return one.
type MenuSpec struct {
	title string
	role  driver.MenuRole
	items []MenuItemSpec
}

// MenuItemSpec describes one menu item. MenuItem and MenuSeparator return
// one.
type MenuItemSpec struct {
	title     string
	shortcut  KeyShortcut
	action    func()
	role      driver.ItemRole
	separator bool
}

// MenuBar returns a menu bar with the given menus, from left to right. The
// first one should be AppMenu(). On macOS an Edit menu is always present,
// because the text editing shortcuts depend on it: without EditMenu() the
// standard one is added.
func MenuBar(menus ...MenuSpec) MenuBarSpec {
	return MenuBarSpec{menus: append([]MenuSpec(nil), menus...)}
}

// Menu returns a menu with the given title and items, from top to bottom.
func Menu(title string, items ...MenuItemSpec) MenuSpec {
	return MenuSpec{title: title, items: append([]MenuItemSpec(nil), items...)}
}

// MenuItem returns an item that calls action on the UI thread when the
// user chooses it, by clicking it or by its shortcut. A zero shortcut
// means none. An item with a nil action is shown disabled, as in AppKit.
func MenuItem(title string, shortcut KeyShortcut, action func()) MenuItemSpec {
	return MenuItemSpec{title: title, shortcut: shortcut, action: action}
}

// MenuSeparator returns a separator line.
func MenuSeparator() MenuItemSpec { return MenuItemSpec{separator: true} }

// AppMenu returns the standard application menu, titled with
// AppConfig.Name: About, Hide, Hide Others, Show All and Quit. Quit (⌘Q,
// Ctrl+Q) asks the App.OnQuitRequested handlers and then quits.
func AppMenu() MenuSpec { return menuFromDriver(driver.DefaultAppMenu()) }

// EditMenu returns the standard Edit menu: Undo, Redo, Cut, Copy, Paste,
// Delete and Select All. They act on the focused text control.
func EditMenu() MenuSpec { return menuFromDriver(driver.DefaultEditMenu()) }

// WindowMenu returns the standard Window menu: Minimize (⌘M, Ctrl+M), Zoom
// and Close (⌘W, Ctrl+W). They act on the key window; Close asks its
// OnShouldClose handlers first.
func WindowMenu() MenuSpec {
	return MenuSpec{title: "Window", role: driver.MenuWindow, items: []MenuItemSpec{
		{role: driver.ItemMinimize},
		{role: driver.ItemZoom},
		{separator: true},
		{role: driver.ItemClose},
	}}
}

// menuFromDriver turns a standard driver menu (role items and separators
// only) into a public one.
func menuFromDriver(m driver.MenuSpec) MenuSpec {
	out := MenuSpec{title: m.Title, role: m.Role, items: make([]MenuItemSpec, len(m.Items))}
	for i, it := range m.Items {
		out.items[i] = MenuItemSpec{title: it.Title, shortcut: it.Shortcut, role: it.Role, separator: it.Separator}
	}
	return out
}

// SetMenuBar replaces the whole menu bar. Each item with an action gets a
// new ID, so an event for an item of an earlier bar does nothing. It must be
// called on the UI thread.
func (a *App) SetMenuBar(bar MenuBarSpec) *App {
	checkUI("App.SetMenuBar")
	actions := make(map[int]func())
	spec := &driver.MenuBarSpec{Menus: make([]driver.MenuSpec, len(bar.menus))}
	for i, m := range bar.menus {
		dm := driver.MenuSpec{Title: m.title, Role: m.role, Items: make([]driver.MenuItemSpec, len(m.items))}
		for j, it := range m.items {
			di := driver.MenuItemSpec{Title: it.title, Role: it.role, Shortcut: it.shortcut, Separator: it.separator}
			switch {
			case it.separator || it.role != driver.ItemNone:
			case it.action == nil:
				di.Disabled = true
			default:
				a.lastMenuID++
				di.ID = a.lastMenuID
				actions[di.ID] = it.action
			}
			dm.Items[j] = di
		}
		spec.Menus[i] = dm
	}
	a.menuActions = actions
	a.drv.SetMenuBar(spec)
	return a
}

// menuItemSelected runs the action of the item with the given ID in the
// current bar. Unknown and stale IDs do nothing.
func (a *App) menuItemSelected(id int) {
	if fn := a.menuActions[id]; fn != nil {
		fn()
	}
}
