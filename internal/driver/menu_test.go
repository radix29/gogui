package driver

import "testing"

func TestMenuBarCloneIsDeep(t *testing.T) {
	sub := &MenuSpec{Title: "Sub", Items: []MenuItemSpec{{ID: 3, Title: "Deep"}}}
	bar := &MenuBarSpec{Menus: []MenuSpec{
		{Title: "File", Items: []MenuItemSpec{{ID: 1, Title: "New"}, {Title: "More", Submenu: sub}}},
	}}
	c := bar.Clone()

	bar.Menus[0].Items[0].Title = "changed"
	sub.Items[0].Title = "changed"

	if got := c.FindItem(1).Title; got != "New" {
		t.Errorf("top-level item shared: %q", got)
	}
	if got := c.FindItem(3).Title; got != "Deep" {
		t.Errorf("submenu item shared: %q", got)
	}
	if (*MenuBarSpec)(nil).Clone() != nil {
		t.Error("nil Clone must be nil")
	}
}

func TestMenuFindItem(t *testing.T) {
	bar := &MenuBarSpec{Menus: []MenuSpec{
		{Items: []MenuItemSpec{{ID: 7, Separator: true}, {ID: 1}}},
		{Items: []MenuItemSpec{{Submenu: &MenuSpec{Items: []MenuItemSpec{{ID: 7, Title: "x"}}}}}},
	}}
	if bar.FindItem(0) != nil {
		t.Error("ID 0 must never be found")
	}
	if it := bar.FindItem(7); it == nil || it.Title != "x" {
		t.Errorf("separators must be skipped, got %+v", it)
	}
	if bar.FindItem(99) != nil || (*MenuBarSpec)(nil).FindItem(1) != nil {
		t.Error("missing item found")
	}
	// FindItem returns a pointer into the spec.
	bar.FindItem(1).Disabled = true
	if !bar.Menus[0].Items[1].Disabled {
		t.Error("FindItem must return a pointer into the spec")
	}
}

func TestDefaultMenuBar(t *testing.T) {
	bar := DefaultMenuBar()
	if !bar.HasMenu(MenuApp) || !bar.HasMenu(MenuEdit) || bar.HasMenu(MenuHelp) {
		t.Fatal("default bar must have the App and Edit menus only")
	}
	if bar.Menus[0].Role != MenuApp {
		t.Error("the app menu must come first")
	}
	for _, m := range bar.Menus {
		for _, it := range m.Items {
			if it.Separator {
				continue
			}
			if it.Role == ItemNone || it.ID != 0 {
				t.Errorf("default item %+v must be a role item without ID", it)
			}
			if it.EffectiveTitle("App") == "" {
				t.Errorf("role %d has no default title", it.Role)
			}
		}
	}
}

func TestRoleDefaults(t *testing.T) {
	if got := ItemQuit.DefaultTitle("Notes"); got != "Quit Notes" {
		t.Errorf("Quit title = %q", got)
	}
	if got := ItemCopy.DefaultShortcut().Display(true); got != "⌘C" {
		t.Errorf("Copy shortcut = %q", got)
	}
	if got := ItemRedo.DefaultShortcut().Display(false); got != "Ctrl+Shift+Z" {
		t.Errorf("Redo shortcut = %q", got)
	}
	if !ItemDelete.DefaultShortcut().IsZero() || ItemNone.DefaultTitle("x") != "" {
		t.Error("roles without defaults returned some")
	}

	it := MenuItemSpec{Role: ItemCopy, Title: "Copy Link", Shortcut: Shortcut{ModPrimary | ModShift, 'C'}}
	if it.EffectiveTitle("x") != "Copy Link" || it.EffectiveShortcut().Mods&ModShift == 0 {
		t.Error("explicit title/shortcut must win over the role defaults")
	}
}
