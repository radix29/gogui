// Package menubar is the pure model of the title-bar menu strip of the
// drawn aqua driver (PLAN.md §5.4.1, S5.6). As scroll holds the scroll
// physics and aqua the scrollers, this package holds what the strip shows
// and matches, and aqua (menustrip.go, menu.go, popuproot.go) draws it and
// runs it.
//
//   - Build turns a driver.MenuBarSpec into a Bar: the menus with their
//     effective titles (the app menu titled with the app name), the items
//     with their effective titles, their shortcuts resolved for Windows and
//     Linux (driver.Shortcut.Resolve(false)) and their display text
//     ("Ctrl+Shift+D"), the roles and the disabled and checked flags.
//   - Bar.Lookup is the accelerator index: a key-down to the item whose
//     shortcut it is.
//   - Layout is the strip arithmetic: where the titles go, which of them fit
//     and how much room the centred window title keeps.
//
// Submenus: the public API cannot build one, so an item with a
// driver.MenuItemSpec.Submenu shows its title with a chevron (Item.Submenu)
// and stays disabled; its items are not in the model and never match (no
// cascading menus in v0.1).
//
// Which windows show the strip is decision D11 (PLAN.md §12): every titled
// window shows the one bar, and accelerators work in every key window.
//
// Positions and widths are in logical points. The package imports only
// driver; it has no build tags.
package menubar
