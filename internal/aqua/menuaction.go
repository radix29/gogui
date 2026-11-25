package aqua

import (
	"github.com/radix29/gogui/internal/aqua/menubar"
	"github.com/radix29/gogui/internal/aqua/text"
	"github.com/radix29/gogui/internal/driver"
)

// The role items of the menu strip (PLAN.md S5.6g): the standard actions
// of driver.ItemRole on Windows and Linux, and their validation.
//
// A role item with an ID is the app's: it is reported to
// AppHandler.MenuItemSelected and enabled by its own flag. A role item
// without one runs the driver's action, enabled only when the action can
// act now (menuEnabled):
//
//   - App menu: Quit asks AppHandler.QuitRequested and quits on true.
//     Hide minimizes the app's shown windows and Show All shows them again
//     (the window of the pick last, so it stays key); both need a shown
//     window. Hide Others is disabled (no other apps to hide on Windows
//     and Linux). About shows an app-modal alert with the app name
//     (AppConfig.Name) and no detail (S5.7g, showAbout).
//   - Edit menu: the items act on the focused element of the window when
//     it is a text element (editActions: TextField, SecureField,
//     TextView), enabled by what it can do now (see textEdit.canEdit).
//     No focused text element: all disabled.
//   - Window menu: Minimize, Zoom and Close act on the window of the pick,
//     enabled by its Miniaturizable, Resizable and Closable styles as its
//     traffic lights are; Close asks WindowHandler.ShouldClose.
//   - While the window has an alert up (S5.7c) the Edit items and Close
//     are disabled: nothing to edit, and the close light is disabled too.
//   - While an alert is open in a modal surface (S5.7g, alertsurface.go)
//     every item but Quit is disabled, the app's own included, in every
//     window and in the alert's surface (decided in S5.7g, as the app
//     menu's Quit stays usable under AppKit's runModal).
//
// The state is computed when a menu opens (menuStrip.menuItems) and again
// when an item runs, as AppKit validates (an accelerator validates its
// item before running it, S5.6h).

// editActions is what the Edit menu's role items act on: a focused text
// element (TextField, SecureField, TextView), through textEdit.
type editActions interface {
	// canEdit reports whether the edit role can act now.
	canEdit(role driver.ItemRole) bool
	// edit runs the edit role if it can act.
	edit(role driver.ItemRole)
}

var (
	_ editActions = (*textFieldView)(nil)
	_ editActions = (*textViewElem)(nil)
)

// isEditRole reports whether r is one of the Edit menu's roles.
func isEditRole(r driver.ItemRole) bool {
	return r >= driver.ItemUndo && r <= driver.ItemSelectAll
}

// editTarget returns the window's focused element when it takes the Edit
// menu's actions, or nil; nil while an alert is up (S5.7c).
func (w *Window) editTarget() editActions {
	if w.closed || w.modal() {
		return nil
	}
	ea, _ := w.router.Focused().(editActions)
	return ea
}

// menuEnabled reports whether item r of the bar can be picked in w now:
// not a separator, a submenu title or disabled by its spec, and, for a role
// item without an ID, its action can act (see above). While an alert is
// open in a modal surface only the Quit item is enabled; a nil w (the
// alert surface's accelerators) allows nothing else.
func (d *Driver) menuEnabled(w *Window, r menubar.Ref) bool {
	it := d.bar.Item(r)
	if it == nil || it.Separator || it.Submenu || it.Disabled || (w != nil && w.closed) {
		return false
	}
	if d.alertModal() || w == nil {
		return it.Role == driver.ItemQuit
	}
	if it.ID != 0 || it.Role == driver.ItemNone {
		return true
	}
	switch role := it.Role; {
	case role == driver.ItemQuit, role == driver.ItemAbout:
		return true
	case role == driver.ItemHide || role == driver.ItemShowAll:
		return d.shownWindows() > 0
	case isEditRole(role):
		ea := w.editTarget()
		return ea != nil && ea.canEdit(role)
	case role == driver.ItemMinimize:
		return w.style&driver.StyleMiniaturizable != 0
	case role == driver.ItemZoom:
		return w.style&driver.StyleResizable != 0
	case role == driver.ItemClose:
		return w.style&driver.StyleClosable != 0 && !w.modal()
	}
	return false // Hide Others
}

// shownWindows returns the number of open windows that have been shown.
func (d *Driver) shownWindows() int {
	n := 0
	for _, w := range d.windows {
		if w.Shown() {
			n++
		}
	}
	return n
}

// runRole runs the standard action of role, picked in w and validated.
func (d *Driver) runRole(w *Window, role driver.ItemRole) {
	switch {
	case role == driver.ItemQuit:
		if d.h.QuitRequested() {
			d.Quit()
		}
	case role == driver.ItemAbout:
		d.showAbout()
	case role == driver.ItemHide:
		for _, x := range d.Windows() {
			if x.Shown() {
				x.surface.Minimize()
			}
		}
	case role == driver.ItemShowAll:
		for _, x := range d.Windows() {
			if x != w && x.Shown() {
				x.surface.Show()
			}
		}
		if w.Shown() {
			w.surface.Show()
		}
	case isEditRole(role):
		if ea := w.editTarget(); ea != nil {
			ea.edit(role)
		}
	case role == driver.ItemMinimize:
		w.surface.Minimize()
	case role == driver.ItemZoom:
		w.surface.Zoom()
	case role == driver.ItemClose:
		w.performClose()
	}
}

// canEdit implements editActions for the text elements, as AppKit's
// NSText validates: nothing while the element is disabled; Undo and Redo
// when the editor has a group to undo or redo (Undo also during a
// composition, which it commits first) and the text is editable; Cut with
// a selection in editable, non-secure text; Copy with a selection in
// non-secure text; Paste whenever the text is editable (the clipboard is
// read asynchronously, so its content cannot be checked); Delete with a
// selection in editable text; Select All with any text.
func (t *textEdit) canEdit(role driver.ItemRole) bool {
	if !t.host.base().EnabledInTree() {
		return false
	}
	start, end := t.ed.Selection()
	sel := start != end
	ro, secure := t.host.readOnlyText(), t.host.secureText()
	switch role {
	case driver.ItemUndo:
		return !ro && (t.ed.CanUndo() || t.composing())
	case driver.ItemRedo:
		return !ro && t.ed.CanRedo() && !t.composing()
	case driver.ItemCut:
		return sel && !ro && !secure
	case driver.ItemCopy:
		return sel && !secure
	case driver.ItemPaste:
		return !ro
	case driver.ItemDelete:
		return sel && !ro
	case driver.ItemSelectAll:
		return t.ed.Len() > 0 || t.composing()
	}
	return false
}

// edit implements editActions: a composition is committed first, as a key
// does, then the role runs as its key binding would (Cut, Copy and Paste
// through the clipboard glue, the others through the element's command
// path, so a TextView scrolls the caret into view); Delete deletes the
// selection as an undo group of its own. It does nothing when the role
// cannot act.
func (t *textEdit) edit(role driver.ItemRole) {
	if !t.canEdit(role) {
		return
	}
	t.commitPreedit()
	if !t.canEdit(role) { // the commit changed the element
		return
	}
	switch role {
	case driver.ItemUndo:
		t.host.command(text.CmdUndo)
	case driver.ItemRedo:
		t.host.command(text.CmdRedo)
	case driver.ItemCut:
		t.clipboard(text.CmdCut)
	case driver.ItemCopy:
		t.clipboard(text.CmdCopy)
	case driver.ItemPaste:
		t.clipboard(text.CmdPaste)
	case driver.ItemDelete:
		rev := t.ed.Buffer().Revision()
		t.ed.Cut()
		t.host.edited(rev)
	case driver.ItemSelectAll:
		t.host.command(text.CmdSelectAll)
	}
}

// showAbout is the About role's action on Windows and Linux (S5.7g): an
// app-modal alert with the app name (AppConfig.Name) as its message, no
// detail and one OK button, which Escape presses too.
func (d *Driver) showAbout() {
	d.RunAlert(nil, driver.AlertSpec{Message: d.cfg.Name, Buttons: []string{"OK"}, Cancel: 0}, nil)
}
