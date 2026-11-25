package gogui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua"
	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	fakeshell "github.com/radix29/gogui/internal/shell/fake"
)

// The menu strips of the controls and notes examples on the aqua driver
// over the fake shell (PLAN.md S5.6j), end to end through the public API:
// their shortcuts with the menus closed, their menus by mouse and by
// keyboard, the Edit roles on the focused text, and the drag area between
// the titles. The strips' frames are pinned in testdata/aqua.

// stripTitle returns the centre of the strip title labelled title, in
// win's surface points.
func stripTitle(t *testing.T, win *Window, title string) driver.Point {
	t.Helper()
	n := aqua.AccessibilityTree(win.p.(*aqua.Window).Root()).Find(a11y.RoleMenuBarItem, title)
	if n == nil {
		t.Fatalf("no strip title %q", title)
	}
	return centreOf(n.Frame)
}

func centreOf(r driver.Rect) driver.Point { return driver.Point{X: r.X + r.W/2, Y: r.Y + r.H/2} }

// openMenu clicks the strip title labelled title and returns the open
// menu's popup and accessibility tree.
func openMenu(t *testing.T, sh *fakeshell.Shell, win *Window, title string) (*fakeshell.Popup, *a11y.Node) {
	t.Helper()
	sf := win.p.(*aqua.Window).Surface().(*fakeshell.Surface)
	sf.Click(stripTitle(t, win, title))
	return openedMenu(t, sh, win)
}

// openedMenu returns the open menu's popup and accessibility tree.
func openedMenu(t *testing.T, sh *fakeshell.Shell, win *Window) (*fakeshell.Popup, *a11y.Node) {
	t.Helper()
	pp, root := sh.GrabPopup(), win.p.(*aqua.Window).GrabPopupRoot()
	if pp == nil || root == nil {
		t.Fatal("no menu open")
	}
	tree := aqua.AccessibilityTree(root)
	if tree.Find(a11y.RoleMenu, "") == nil {
		t.Fatalf("the grab popup is no menu:\n%s", tree.Dump())
	}
	return pp, tree
}

// pickItem clicks the item labelled item in the open menu (which must be
// enabled) and lets its blink end, so the action has run.
func pickItem(t *testing.T, app *App, sh *fakeshell.Shell, win *Window, item string) {
	t.Helper()
	pp, tree := openedMenu(t, sh, win)
	n := tree.Find(a11y.RoleMenuItem, item)
	if n == nil {
		t.Fatalf("no item %q:\n%s", item, tree.Dump())
	}
	if n.States&a11y.StateDisabled != 0 {
		t.Fatalf("item %q disabled", item)
	}
	win.p.(*aqua.Window).Surface().(*fakeshell.Surface).Click(pp.ToParent(centreOf(n.Frame)))
	sh.FakeClock().Advance(time.Second)
	drain(app)
	if sh.GrabPopup() != nil {
		t.Fatalf("the menu is still open after picking %q", item)
	}
}

// itemDisabled reports whether the open menu's item labelled item is
// disabled.
func itemDisabled(t *testing.T, tree *a11y.Node, item string) bool {
	t.Helper()
	n := tree.Find(a11y.RoleMenuItem, item)
	if n == nil {
		t.Fatalf("no item %q:\n%s", item, tree.Dump())
	}
	return n.States&a11y.StateDisabled != 0
}

// findWidget returns the first widget of win's content tree, depth first,
// for which ok is true.
func findWidget(win *Window, ok func(Widget) bool) Widget {
	var walk func(w Widget) Widget
	walk = func(w Widget) Widget {
		if ok(w) {
			return w
		}
		if p, isParent := w.(parentWidget); isParent {
			for _, c := range p.childWidgets() {
				if f := walk(c); f != nil {
					return f
				}
			}
		}
		return nil
	}
	return walk(win.content)
}

// TestExampleControlsMenusOnAqua: the controls strip's View item toggles
// the appearance by its shortcut, by mouse and by keyboard (Alt, arrows,
// Down, Return); the Edit roles act on the focused text field; the caption
// between the strip and the window title still drags.
func TestExampleControlsMenusOnAqua(t *testing.T) {
	runAquaNamed(t, "Controls", func(app *App, sh *fakeshell.Shell) {
		win := exampleControls(t, app)
		sf := paintWindow(t, sh, win)
		ctrl := driver.ModControl
		dark := func(want bool) {
			t.Helper()
			drain(app)
			if got := app.Appearance() == AppearanceDark; got != want {
				t.Fatalf("dark %v, want %v", got, want)
			}
		}
		dark(false)

		// The shortcut, with the menus closed.
		if !sf.Key('D', ctrl|driver.ModShift) {
			t.Fatal("Ctrl+Shift+D not consumed")
		}
		dark(true)

		// By mouse.
		openMenu(t, sh, win, "View")
		pickItem(t, app, sh, win, "Toggle Appearance")
		dark(false)

		// By keyboard: Alt focuses the strip on the app menu, Right twice
		// to View, Down opens it on its first item, Return picks it.
		if !sf.TapModifier(driver.ModOption) {
			t.Fatal("a lone Alt not consumed")
		}
		sf.Key(driver.KeyRight, 0)
		sf.Key(driver.KeyRight, 0)
		sf.Key(driver.KeyDown, 0)
		openedMenu(t, sh, win)
		sf.Key(driver.KeyReturn, 0)
		sh.FakeClock().Advance(time.Second)
		dark(true)
		if sh.GrabPopup() != nil {
			t.Fatal("the menu is still open after Return")
		}

		// The Edit menu on the focused text field.
		field := findWidget(win, func(w Widget) bool {
			_, ok := w.(*TextField) // the first, above the secure field
			return ok
		}).(*TextField)
		field.Focus()
		drain(app)
		sf.Type("Hello")
		_, tree := openMenu(t, sh, win, "Edit")
		for item, want := range map[string]bool{"Undo": false, "Cut": true, "Copy": true, "Paste": false, "Select All": false} {
			if got := itemDisabled(t, tree, item); got != want {
				t.Errorf("Edit %s disabled %v, want %v (no selection)", item, got, want)
			}
		}
		pickItem(t, app, sh, win, "Select All")
		openMenu(t, sh, win, "Edit")
		pickItem(t, app, sh, win, "Cut")
		if field.Text() != "" {
			t.Errorf("after Select All and Cut: %q", field.Text())
		}
		if s, ok := sh.Clipboard(); !ok || s != "Hello" {
			t.Errorf("clipboard %q %v, want Hello", s, ok)
		}
		openMenu(t, sh, win, "Edit")
		pickItem(t, app, sh, win, "Undo")
		if field.Text() != "Hello" {
			t.Errorf("after Undo: %q", field.Text())
		}

		// The caption right of the strip drags; the titles do not.
		if k := sf.HitTest(driver.Point{X: 232, Y: 14}); k != shell.RegionCaption {
			t.Errorf("between the strip and the title: region %d, want the caption", k)
		}
		if k := sf.HitTest(stripTitle(t, win, "Edit")); k != shell.RegionClient {
			t.Errorf("on a title: region %d, want client", k)
		}
	})
}

// TestExampleNotesMenusOnAqua: the notes strip's Format items change the
// editor's font by their shortcuts, Primary+'+' on a US layout (Shift+'=')
// and on a German-like one (an unshifted '+'); File > New clears the text
// by mouse and by Ctrl+N; the Edit roles act on the focused TextView;
// hover switches between open menus.
func TestExampleNotesMenusOnAqua(t *testing.T) {
	runAquaNamed(t, "Notes", func(app *App, sh *fakeshell.Shell) {
		win := exampleNotes(t, app)
		sf := paintWindow(t, sh, win)
		editor := findWidget(win, func(w Widget) bool { _, ok := w.(*TextView); return ok }).(*TextView)
		ctrl := driver.ModControl
		size := func(want float64) {
			t.Helper()
			drain(app)
			if got := editor.Font().Size; got != want {
				t.Fatalf("font size %g, want %g", got, want)
			}
		}

		if !sf.Key('=', ctrl|driver.ModShift) { // US: '+' is Shift+'='
			t.Fatal("Ctrl+Shift+= not consumed")
		}
		size(14)
		sh.SetShiftLayout(map[driver.Key]driver.Key{'+': '*'}) // German-like
		if !sf.Key('+', ctrl) {
			t.Fatal("Ctrl++ (German) not consumed")
		}
		size(15)
		sh.SetShiftLayout(nil)
		if !sf.Key('-', ctrl) {
			t.Fatal("Ctrl+- not consumed")
		}
		size(14)
		openMenu(t, sh, win, "Format")
		pickItem(t, app, sh, win, "Smaller")
		size(13)

		// Typing goes to the focused editor (after the welcome text, where
		// the caret starts); the Edit roles act on it.
		typed := notesWelcome + "one two"
		sf.Type("one two")
		drain(app)
		if editor.Text() != typed {
			t.Fatalf("typed %q", editor.Text())
		}
		openMenu(t, sh, win, "Edit")
		pickItem(t, app, sh, win, "Select All")
		if s, e := editor.Selection(); s != 0 || e != len(typed) {
			t.Errorf("selection %d–%d after Select All", s, e)
		}
		openMenu(t, sh, win, "Edit")
		pickItem(t, app, sh, win, "Copy")
		if s, ok := sh.Clipboard(); !ok || s != typed {
			t.Errorf("clipboard %q %v", s, ok)
		}

		// File > New by mouse, after hovering over from the Edit menu.
		openMenu(t, sh, win, "Edit")
		sf.Move(stripTitle(t, win, "File"))
		_, tree := openedMenu(t, sh, win)
		if tree.Find(a11y.RoleMenuItem, "New") == nil {
			t.Fatalf("hovering File did not switch:\n%s", tree.Dump())
		}
		pickItem(t, app, sh, win, "New")
		if editor.Text() != "" {
			t.Errorf("after New: %q", editor.Text())
		}
		sf.Type("x")
		if !sf.Key('N', ctrl) {
			t.Fatal("Ctrl+N not consumed")
		}
		drain(app)
		if editor.Text() != "" {
			t.Errorf("after Ctrl+N: %q", editor.Text())
		}

		// F10 focuses the strip, Escape leaves it, and typing goes to the
		// editor again.
		sf.Key(driver.KeyF1+9, 0)
		sf.Type("z") // eaten: no title starts with z
		sf.Key(driver.KeyEscape, 0)
		sf.Type("y")
		drain(app)
		if editor.Text() != "y" {
			t.Errorf("after F10, z, Escape, y: %q", editor.Text())
		}
	})
}

// TestExampleControlsAccentOnAqua: the controls View menu's accent items
// (S6.1g) set the app's accent through the public API: the default button
// is filled with it and repainted at once, its title turns dark on the
// light yellow, and System Accent brings back the OS accent; no
// OnAppearanceChange fires.
func TestExampleControlsAccentOnAqua(t *testing.T) {
	runAquaNamed(t, "Controls", func(app *App, sh *fakeshell.Shell) {
		win := exampleControls(t, app)
		changes := 0
		app.OnAppearanceChange(func(Appearance) { changes++ })
		sf := paintWindow(t, sh, win)
		osAccent := app.AccentColor()
		defaultTitle := func() string {
			t.Helper()
			paintWindow(t, sh, win)
			for _, l := range strings.Split(sf.Dump(), "\n") {
				if strings.HasPrefix(l, "text ") && strings.Contains(l, `"Default"`) {
					return hexRE.FindString(l[strings.Index(l, `"Default"`):])
				}
			}
			t.Fatalf("no default button title in:\n%s", sf.Dump())
			return ""
		}
		if c := defaultTitle(); c != "#ffffff" {
			t.Errorf("default button title %s on the OS accent, want white", c)
		}
		for _, c := range []struct {
			item  string
			want  Color
			title string
		}{
			{"Purple Accent", RGB8(149, 61, 150), "#ffffff"},
			{"Yellow Accent", RGB8(255, 197, 0), "#000000d9"},
			{"Green Accent", RGB8(98, 186, 70), "#000000d9"},
			{"System Accent", osAccent, "#ffffff"},
		} {
			openMenu(t, sh, win, "View")
			pickItem(t, app, sh, win, c.item)
			if got := app.AccentColor(); got != c.want {
				t.Errorf("%s: accent %v, want %v", c.item, got, c.want)
			}
			if got := defaultTitle(); got != c.title {
				t.Errorf("%s: default button title %s, want %s", c.item, got, c.title)
			}
		}
		if changes != 0 {
			t.Errorf("%d OnAppearanceChange calls for the accent", changes)
		}
	})
}

// hexRE matches a color in a record dump.
var hexRE = regexp.MustCompile(`#[0-9a-f]{6}([0-9a-f]{2})?\b`)
