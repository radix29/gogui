package gogui

import (
	"slices"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/headless"
)

// itemRow is the part of a driver menu item the tests compare.
type itemRow struct {
	title    string
	role     driver.ItemRole
	shortcut KeyShortcut
	hasID    bool
	sep      bool
}

func rows(m driver.MenuSpec) []itemRow {
	var out []itemRow
	for _, it := range m.Items {
		out = append(out, itemRow{it.Title, it.Role, it.Shortcut, it.ID != 0, it.Separator})
	}
	return out
}

func wantMenu(t *testing.T, got, want driver.MenuSpec) {
	t.Helper()
	if got.Title != want.Title || got.Role != want.Role || !slices.Equal(rows(got), rows(want)) {
		t.Errorf("menu %q:\n got %+v %v\nwant %+v %v", want.Title, got.Role, rows(got), want.Role, rows(want))
	}
}

// appDriver runs body like runApp and also hands it the headless driver.
func appDriver(t *testing.T, body func(app *App, d *headless.Driver)) {
	t.Helper()
	d := useHeadless(t)
	if err := runTest(t, func(app *App) {
		body(app, d)
		app.Quit()
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestMenuBarControls(t *testing.T) {
	appDriver(t, func(app *App, d *headless.Driver) {
		toggled := 0
		toggle := Shortcut(KeyPrimary|KeyShift, 'd')
		var got *App = app.SetMenuBar(MenuBar(
			AppMenu(),
			EditMenu(),
			Menu("View", MenuItem("Toggle Appearance", toggle, func() { toggled++ })),
		))
		if got != app {
			t.Error("SetMenuBar does not return the app")
		}
		bar := d.MenuBar
		if len(bar.Menus) != 3 {
			t.Fatalf("%d menus, want 3", len(bar.Menus))
		}
		wantMenu(t, bar.Menus[0], driver.DefaultAppMenu())
		wantMenu(t, bar.Menus[1], driver.DefaultEditMenu())
		view := bar.Menus[2]
		wantMenu(t, view, driver.MenuSpec{Title: "View", Items: []driver.MenuItemSpec{
			{Title: "Toggle Appearance", Shortcut: Shortcut(KeyPrimary|KeyShift, 'D'), ID: 1},
		}})
		id := view.Items[0].ID
		if !d.UserSelectMenuItem(id) || toggled != 1 {
			t.Errorf("selecting item %d: %d calls", id, toggled)
		}
		if d.UserSelectMenuItem(0) || toggled != 1 {
			t.Error("ID 0 dispatched")
		}
	})
}

func TestMenuBarNotes(t *testing.T) {
	appDriver(t, func(app *App, d *headless.Driver) {
		var log []string
		item := func(title string, key Key) MenuItemSpec {
			return MenuItem(title, Shortcut(KeyPrimary, key), func() { log = append(log, title) })
		}
		bar := MenuBar(
			AppMenu(),
			Menu("File", item("New", 'N')),
			EditMenu(),
			Menu("Format", item("Bigger", '+'), MenuSeparator(), item("Smaller", '-'),
				MenuItem("Later", KeyShortcut{}, nil)),
			WindowMenu(),
		)
		app.SetMenuBar(bar)
		menus := d.MenuBar.Menus
		roles := []driver.MenuRole{driver.MenuApp, driver.MenuNone, driver.MenuEdit, driver.MenuNone, driver.MenuWindow}
		for i, m := range menus {
			if m.Role != roles[i] {
				t.Errorf("menu %d %q: role %d, want %d", i, m.Title, m.Role, roles[i])
			}
		}
		format := menus[3].Items
		if format[0].Shortcut != (KeyShortcut{Mods: KeyPrimary, Key: '+'}) || !format[1].Separator ||
			format[2].Shortcut != (KeyShortcut{Mods: KeyPrimary, Key: '-'}) {
			t.Errorf("Format items: %+v", format)
		}
		if later := format[3]; later.ID != 0 || !later.Disabled {
			t.Errorf("item without action: %+v, want disabled without ID", later)
		}
		ids := []int{menus[1].Items[0].ID, format[0].ID, format[2].ID}
		if ids[0] == 0 || ids[0] == ids[1] || ids[1] == ids[2] || ids[0] == ids[2] {
			t.Fatalf("IDs %v are not distinct and non-zero", ids)
		}
		wantMenu(t, menus[4], driver.MenuSpec{Title: "Window", Role: driver.MenuWindow, Items: []driver.MenuItemSpec{
			{Role: driver.ItemMinimize}, {Role: driver.ItemZoom}, {Separator: true}, {Role: driver.ItemClose},
		}})
		for _, id := range ids {
			d.UserSelectMenuItem(id)
		}
		if want := []string{"New", "Bigger", "Smaller"}; !slices.Equal(log, want) {
			t.Errorf("dispatched %v, want %v", log, want)
		}

		// Replacing the bar (here: installing the same spec again) gives
		// new IDs, and the old ones do nothing.
		app.SetMenuBar(bar)
		log = nil
		for _, id := range ids {
			if d.UserSelectMenuItem(id) {
				t.Errorf("stale ID %d still in the bar", id)
			}
			app.menuItemSelected(id) // a native layer that reports a stale ID
		}
		if len(log) != 0 {
			t.Errorf("stale IDs dispatched %v", log)
		}
		d.UserSelectMenuItem(d.MenuBar.Menus[1].Items[0].ID)
		if !slices.Equal(log, []string{"New"}) {
			t.Errorf("new ID dispatched %v", log)
		}
	})
}

func TestAlert(t *testing.T) {
	appDriver(t, func(app *App, d *headless.Driver) {
		win := app.NewWindow(WindowConfig{Title: "W", Size: Size{W: 200, H: 100}})
		var results []int
		Alert{Style: AlertWarning, Message: "Delete?", Detail: "Really.", Buttons: []string{"Delete", "Cancel"}}.
			Show(win, func(r int) { results = append(results, r) })
		Alert{Message: "Done"}.Show(nil, nil)
		if len(d.Alerts) != 2 {
			t.Fatalf("%d alerts shown, want 2", len(d.Alerts))
		}
		a, b := d.Alerts[0], d.Alerts[1]
		want := driver.AlertSpec{Style: AlertWarning, Message: "Delete?", Detail: "Really.", Cancel: 1}
		if a.Parent != win.p || a.Spec.Style != want.Style || a.Spec.Message != want.Message ||
			a.Spec.Detail != want.Detail || a.Spec.Cancel != 1 || !slices.Equal(a.Spec.Buttons, []string{"Delete", "Cancel"}) {
			t.Errorf("alert spec %+v (parent %v)", a.Spec, a.Parent)
		}
		if b.Parent != nil || !slices.Equal(b.Spec.Buttons, []string{"OK"}) || b.Spec.Cancel != -1 {
			t.Errorf("default alert spec %+v (parent %v), want app-modal [OK] without Cancel", b.Spec, b.Parent)
		}
		a.Dismiss(1)
		b.Dismiss(0) // nil done
		if !slices.Equal(results, []int{1}) || len(d.Alerts) != 0 {
			t.Errorf("results %v, %d alerts left", results, len(d.Alerts))
		}
		win.Close()
		Alert{Buttons: []string{"Cancel", "OK"}}.Show(win, nil)
		if c := d.Alerts[0]; c.Parent != nil || c.Spec.Cancel != 0 {
			t.Errorf("alert on a closed window: parent %v, cancel %d", c.Parent, c.Spec.Cancel)
		}
	})
}

func TestAppearance(t *testing.T) {
	appDriver(t, func(app *App, d *headless.Driver) {
		var log []Appearance
		unsub := app.OnAppearanceChange(func(a Appearance) { log = append(log, a) })
		if app.Appearance() != AppearanceLight {
			t.Fatalf("start appearance %v", app.Appearance())
		}
		var got *App = app.SetAppearance(app.Appearance().Toggled())
		if got != app || d.Mode != driver.AppearanceDark || app.Appearance() != AppearanceDark {
			t.Errorf("after toggle: mode %d, appearance %v", d.Mode, app.Appearance())
		}
		app.SetAppearance(app.Appearance().Toggled())
		if app.Appearance() != AppearanceLight {
			t.Errorf("toggled twice: %v", app.Appearance())
		}

		// The OS turns dark while the app follows it; an accent change alone
		// fires nothing.
		app.SetAppearance(AppearanceSystem)
		sys := d.System
		sys.Accent = RGB8(255, 0, 0)
		d.SetSystemAppearance(sys)
		sys.Dark = true
		d.SetSystemAppearance(sys)
		if d.Mode != driver.AppearanceSystem || app.Appearance() != AppearanceDark {
			t.Errorf("system dark: mode %d, appearance %v", d.Mode, app.Appearance())
		}
		unsub()
		unsub()
		app.SetAppearance(AppearanceLight)
		if want := []Appearance{AppearanceDark, AppearanceLight, AppearanceDark}; !slices.Equal(log, want) {
			t.Errorf("changes %v, want %v", log, want)
		}
	})
}

// TestAccentColor: SetAccentColor chains and replaces the OS accent of the
// effective appearance (opaque); AccentColor follows the OS accent with no
// override and keeps the override through an OS change; ColorControlAccent
// and the zero Color clear it; OnAppearanceChange never fires for it
// (S6.1g).
func TestAccentColor(t *testing.T) {
	appDriver(t, func(app *App, d *headless.Driver) {
		var log []Appearance
		app.OnAppearanceChange(func(a Appearance) { log = append(log, a) })
		if got := app.AccentColor(); got != driver.DefaultAccent {
			t.Fatalf("start accent %v, want the default", got)
		}
		purple := RGBA(0.6, 0.2, 0.6, 0.5)
		var got *App = app.SetAccentColor(purple)
		want := purple
		want.A = 1
		if got != app || app.AccentColor() != want || d.Accent != purple {
			t.Errorf("override: accent %v (recorded %v), want %v", app.AccentColor(), d.Accent, want)
		}
		sys := d.System
		sys.Accent = RGB8(255, 0, 0)
		d.SetSystemAppearance(sys)
		if app.AccentColor() != want {
			t.Errorf("an OS accent change replaced the override: %v", app.AccentColor())
		}
		app.SetAccentColor(ColorControlAccent)
		if app.AccentColor() != sys.Accent || d.Accent != (Color{}) {
			t.Errorf("ColorControlAccent: accent %v (recorded %v), want the OS one", app.AccentColor(), d.Accent)
		}
		app.SetAccentColor(RGB8(255, 214, 10)).SetAccentColor(Color{})
		if app.AccentColor() != sys.Accent {
			t.Errorf("zero Color: accent %v, want the OS one", app.AccentColor())
		}
		if len(log) != 0 {
			t.Errorf("OnAppearanceChange fired for the accent: %v", log)
		}
	})
}

func TestQuitRequested(t *testing.T) {
	appDriver(t, func(app *App, d *headless.Driver) {
		win := app.NewWindow(WindowConfig{Title: "W"})
		var log []string
		allow := false
		app.OnQuitRequested(func() bool { log = append(log, "a"); return allow })
		unsub := app.OnQuitRequested(func() bool { log = append(log, "b"); return true })
		if d.UserQuit() || d.Quitting() || win.closed {
			t.Fatal("a vetoed quit quit")
		}
		if !slices.Equal(log, []string{"a"}) {
			t.Errorf("asked %v, want [a]: a veto stops the rest", log)
		}
		allow = true
		log = nil
		if !d.UserQuit() || !d.Quitting() || !win.closed {
			t.Error("an allowed quit did not quit")
		}
		if !slices.Equal(log, []string{"a", "b"}) {
			t.Errorf("asked %v, want [a b]", log)
		}
		unsub()
	})
}

// TestAlertCancel is A5: Cancel picks the Escape button explicitly; the
// zero value keeps matching "Cancel".
func TestAlertCancel(t *testing.T) {
	for _, tc := range []struct {
		buttons []string
		cancel  int
		want    int
	}{
		{[]string{"Löschen", "Abbrechen"}, 0, -1}, // no English "Cancel", no index
		{[]string{"Löschen", "Abbrechen"}, 1, 1},  // a localized cancel button
		{[]string{"Delete", "Cancel"}, 0, 1},      // the default match
		{[]string{"Delete", "Cancel"}, AlertNoCancel, -1},
		{[]string{"Delete", "Cancel"}, 2, -1}, // outside Buttons
		{[]string{"Delete", "Cancel"}, -7, -1},
		{nil, 0, -1}, // the default "OK"
	} {
		if got := (Alert{Buttons: tc.buttons, Cancel: tc.cancel}).spec().Cancel; got != tc.want {
			t.Errorf("Buttons %q, Cancel %d: spec Cancel %d, want %d", tc.buttons, tc.cancel, got, tc.want)
		}
	}
}

// TestWindowAfterClose is A9 and A3: a closed window drops its content and
// handlers, ignores new subscriptions, and Content reports nil.
func TestWindowAfterClose(t *testing.T) {
	runApp(t, func(app *App) {
		win := app.NewWindow(WindowConfig{Size: Size{W: 100, H: 50}})
		l := NewLabel("x")
		win.SetContent(l)
		if win.Content() != Widget(l) {
			t.Fatal("Content")
		}
		closed := 0
		win.OnClose(func() { closed++ })
		win.OnShouldClose(func() bool { return true })
		win.Close()
		if closed != 1 || win.Content() != nil || win.root != nil || !l.dead {
			t.Errorf("after close: %d OnClose calls, content %v, root %v", closed, win.Content(), win.root)
		}
		if len(win.onClose.list) != 0 || len(win.onShouldClose.list) != 0 {
			t.Error("a closed window keeps its handlers")
		}
		win.OnClose(func() { closed++ })()
		win.OnShouldClose(func() bool { return false })
		if len(win.onClose.list) != 0 || len(win.onShouldClose.list) != 0 {
			t.Error("a closed window stores new handlers")
		}
		win.SetContent(NewLabel("y")) // does nothing on a closed window
		if win.Content() != nil {
			t.Error("SetContent on a closed window")
		}
	})
}

// TestGettersA3: the symmetric getters return what the setters stored.
func TestGettersA3(t *testing.T) {
	runApp(t, func(*App) {
		b := NewButton("a")
		c := NewCheckbox("b")
		f := NewTextField().SetPlaceholder("p")
		if b.Title() != "a" || c.Title() != "b" || f.Placeholder() != "p" || b.Tooltip() != "" {
			t.Error("getters after construction")
		}
		b.SetTitle("A").SetTooltip("tip")
		c.SetTitle("B")
		g := NewRadioGroup("x", "y").SetTooltip("group")
		if b.Title() != "A" || c.Title() != "B" || b.Tooltip() != "tip" || g.Tooltip() != "group" {
			t.Error("getters after the setters")
		}
	})
}

// TestNewWindowOutsideRun is A6: App.NewWindow panics outside Run like
// every constructor, and the app is current only from Ready on.
func TestNewWindowOutsideRun(t *testing.T) {
	var app *App
	runApp(t, func(a *App) { app = a })
	mustPanic(t, "App.NewWindow called outside Run", func() { app.NewWindow(WindowConfig{}) })
}
