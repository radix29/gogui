package menubar

import (
	"reflect"
	"testing"

	"github.com/radix29/gogui/internal/driver"
)

const (
	primary = driver.ModPrimary
	ctrl    = driver.ModControl
	shift   = driver.ModShift
	alt     = driver.ModOption
)

// controlsSpec is the driver spec examples/controls installs (as
// gogui.App.SetMenuBar builds it; gogui's TestMenuBarControls checks that
// side): the app menu, the Edit menu and View with one item of ID 1.
func controlsSpec() *driver.MenuBarSpec {
	return &driver.MenuBarSpec{Menus: []driver.MenuSpec{
		driver.DefaultAppMenu(),
		driver.DefaultEditMenu(),
		{Title: "View", Items: []driver.MenuItemSpec{
			{ID: 1, Title: "Toggle Appearance", Shortcut: driver.Shortcut{Mods: primary | shift, Key: 'D'}},
		}},
	}}
}

// notesSpec is the driver spec examples/notes installs: File with New,
// Edit, and Format with Bigger on Primary+'+' and Smaller on Primary+'-'.
func notesSpec() *driver.MenuBarSpec {
	return &driver.MenuBarSpec{Menus: []driver.MenuSpec{
		driver.DefaultAppMenu(),
		{Title: "File", Items: []driver.MenuItemSpec{
			{ID: 1, Title: "New", Shortcut: driver.Shortcut{Mods: primary, Key: 'N'}},
		}},
		driver.DefaultEditMenu(),
		{Title: "Format", Items: []driver.MenuItemSpec{
			{ID: 2, Title: "Bigger", Shortcut: driver.Shortcut{Mods: primary, Key: '+'}},
			{ID: 3, Title: "Smaller", Shortcut: driver.Shortcut{Mods: primary, Key: '-'}},
		}},
	}}
}

// row is an item flattened for comparison.
type row struct {
	Title, Shortcut string
	Sep, Disabled   bool
}

func rows(m Menu) []row {
	out := make([]row, len(m.Items))
	for i, it := range m.Items {
		out[i] = row{it.Title, it.ShortcutText, it.Separator, it.Disabled}
	}
	return out
}

func TestBuildControls(t *testing.T) {
	b := Build(controlsSpec(), "Controls")
	if got, want := b.Titles(), []string{"Controls", "Edit", "View"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("titles = %q, want %q", got, want)
	}
	if b.Menus[0].Role != driver.MenuApp || b.Menus[1].Role != driver.MenuEdit || b.Menus[2].Role != driver.MenuNone {
		t.Errorf("roles = %v %v %v", b.Menus[0].Role, b.Menus[1].Role, b.Menus[2].Role)
	}
	wantApp := []row{
		{Title: "About Controls"},
		{Sep: true},
		{Title: "Hide Controls", Shortcut: "Ctrl+H"},
		{Title: "Hide Others", Shortcut: "Ctrl+Alt+H"},
		{Title: "Show All"},
		{Sep: true},
		{Title: "Quit Controls", Shortcut: "Ctrl+Q"},
	}
	if got := rows(b.Menus[0]); !reflect.DeepEqual(got, wantApp) {
		t.Errorf("app menu =\n%v\nwant\n%v", got, wantApp)
	}
	wantEdit := []row{
		{Title: "Undo", Shortcut: "Ctrl+Z"},
		{Title: "Redo", Shortcut: "Ctrl+Shift+Z"},
		{Sep: true},
		{Title: "Cut", Shortcut: "Ctrl+X"},
		{Title: "Copy", Shortcut: "Ctrl+C"},
		{Title: "Paste", Shortcut: "Ctrl+V"},
		{Title: "Delete"},
		{Title: "Select All", Shortcut: "Ctrl+A"},
	}
	if got := rows(b.Menus[1]); !reflect.DeepEqual(got, wantEdit) {
		t.Errorf("edit menu =\n%v\nwant\n%v", got, wantEdit)
	}
	view := b.Menus[2].Items[0]
	if view.ID != 1 || view.Title != "Toggle Appearance" || view.ShortcutText != "Ctrl+Shift+D" ||
		view.Shortcut != (driver.Shortcut{Mods: ctrl | shift, Key: 'D'}) {
		t.Errorf("view item = %+v", view)
	}
	if b.Menus[1].Items[0].Role != driver.ItemUndo || b.Menus[0].Items[6].Role != driver.ItemQuit {
		t.Error("role items lost their roles")
	}
}

func TestBuildNotes(t *testing.T) {
	b := Build(notesSpec(), "Notes")
	if got, want := b.Titles(), []string{"Notes", "File", "Edit", "Format"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("titles = %q, want %q", got, want)
	}
	format := rows(b.Menus[3])
	want := []row{{Title: "Bigger", Shortcut: "Ctrl++"}, {Title: "Smaller", Shortcut: "Ctrl+-"}}
	if !reflect.DeepEqual(format, want) {
		t.Errorf("format = %v, want %v", format, want)
	}
}

func TestBuildNilAndDefault(t *testing.T) {
	if Build(nil, "X") != nil {
		t.Error("Build(nil) is not nil")
	}
	var nb *Bar
	if nb.Titles() != nil || nb.Item(Ref{}) != nil {
		t.Error("nil bar has titles or items")
	}
	if _, ok := nb.Lookup('C', driver.KeyNone, ctrl); ok {
		t.Error("nil bar matched")
	}
	b := Build(driver.DefaultMenuBar(), "App")
	if got, want := b.Titles(), []string{"App", "Edit"}; !reflect.DeepEqual(got, want) {
		t.Errorf("default titles = %q, want %q", got, want)
	}
}

func TestBuildDetails(t *testing.T) {
	spec := &driver.MenuBarSpec{Menus: []driver.MenuSpec{
		{Role: driver.MenuApp, Title: "ignored", Items: []driver.MenuItemSpec{
			{Role: driver.ItemQuit, Title: "Exit", Shortcut: driver.Shortcut{Mods: primary | alt, Key: 'x'}},
		}},
		{Title: "More", Items: []driver.MenuItemSpec{
			{ID: 4, Title: "Sub", Shortcut: driver.Shortcut{Mods: primary, Key: 'S'},
				Submenu: &driver.MenuSpec{Items: []driver.MenuItemSpec{{ID: 5, Title: "Deep", Shortcut: driver.Shortcut{Mods: primary, Key: 'K'}}}}},
			{ID: 6, Title: "Off", Disabled: true, Checked: true, Shortcut: driver.Shortcut{Mods: primary, Key: 'O'}},
			{Separator: true, Title: "ignored", ID: 7, Shortcut: driver.Shortcut{Mods: primary, Key: 'P'}},
		}},
	}}
	b := Build(spec, "App")
	if b.Menus[0].Title != "App" {
		t.Errorf("app title = %q", b.Menus[0].Title)
	}
	quit := b.Menus[0].Items[0]
	if quit.Title != "Exit" || quit.ShortcutText != "Ctrl+Alt+X" || quit.Shortcut != (driver.Shortcut{Mods: ctrl | alt, Key: 'X'}) {
		t.Errorf("quit = %+v", quit)
	}
	sub := b.Menus[1].Items[0]
	if !sub.Submenu || !sub.Disabled || !sub.Shortcut.IsZero() || sub.ShortcutText != "" || sub.ID != 4 || sub.Title != "Sub" {
		t.Errorf("submenu title = %+v", sub)
	}
	off := b.Menus[1].Items[1]
	if !off.Disabled || !off.Checked || off.ShortcutText != "Ctrl+O" {
		t.Errorf("disabled item = %+v", off)
	}
	if sep := b.Menus[1].Items[2]; sep != (Item{Separator: true}) {
		t.Errorf("separator = %+v", sep)
	}
	for _, k := range []driver.Key{'S', 'K', 'P'} {
		if r, ok := b.Lookup(k, driver.KeyNone, ctrl); ok {
			t.Errorf("Ctrl+%c matched %v (submenu, its items and separators never match)", k, r)
		}
	}
	if r, ok := b.Lookup('O', driver.KeyNone, ctrl); !ok || r != (Ref{1, 1}) {
		t.Errorf("disabled Ctrl+O = %v %v, want a match (the caller validates)", r, ok)
	}
	if b.Item(Ref{1, 1}) != &b.Menus[1].Items[1] || b.Item(Ref{1, 3}) != nil || b.Item(Ref{2, 0}) != nil || b.Item(Ref{-1, 0}) != nil {
		t.Error("Item out of range")
	}
	// The bar copies what it needs: changing the spec afterwards changes nothing.
	spec.Menus[1].Items[1].Title = "changed"
	if b.Menus[1].Items[1].Title != "Off" {
		t.Error("bar shares the spec")
	}
}

func TestLookup(t *testing.T) {
	none := driver.KeyNone
	controls := Build(controlsSpec(), "Controls")
	notes := Build(notesSpec(), "Notes")
	cases := []struct {
		name         string
		bar          *Bar
		key, shifted driver.Key
		mods         driver.Modifiers
		want         Ref
		ok           bool
	}{
		{"copy", controls, 'C', 'C', ctrl, Ref{1, 4}, true},
		{"copy lower-case key", controls, 'c', none, ctrl, Ref{1, 4}, true},
		{"undo", controls, 'Z', 'Z', ctrl, Ref{1, 0}, true},
		{"redo", controls, 'Z', 'Z', ctrl | shift, Ref{1, 1}, true},
		{"toggle", controls, 'D', 'D', ctrl | shift, Ref{2, 0}, true},
		{"Ctrl+D is not Ctrl+Shift+D", controls, 'D', 'D', ctrl, Ref{}, false},
		{"Ctrl+Shift+C is not Ctrl+C", controls, 'C', 'C', ctrl | shift, Ref{}, false},
		{"Alt+C", controls, 'C', 'C', alt, Ref{}, false},
		{"plain C", controls, 'C', 'C', 0, Ref{}, false},
		{"Ctrl+Alt+C (AltGr)", controls, 'C', 'C', ctrl | alt, Ref{}, false},
		{"hide others", controls, 'H', 'H', ctrl | alt, Ref{0, 3}, true},
		{"quit", controls, 'Q', 'Q', ctrl, Ref{0, 6}, true},
		{"Super+C is not Ctrl+C", controls, 'C', 'C', driver.ModCommand, Ref{}, false},
		{"no key", controls, none, none, ctrl, Ref{}, false},
		{"modifier only", controls, none, none, ctrl | shift, Ref{}, false},

		{"new", notes, 'N', 'N', ctrl, Ref{1, 0}, true},
		{"smaller", notes, '-', '_', ctrl, Ref{3, 1}, true},
		// US layout: '+' is Shift+'='.
		{"bigger on US", notes, '=', '+', ctrl | shift, Ref{3, 0}, true},
		{"Ctrl+= on US", notes, '=', '+', ctrl, Ref{}, false},
		{"bigger, shift unknown", notes, '=', none, ctrl | shift, Ref{}, false},
		// German layout: '+' is unshifted.
		{"bigger on DE", notes, '+', '*', ctrl, Ref{3, 0}, true},
		{"Ctrl+Shift++ on DE", notes, '+', '*', ctrl | shift, Ref{}, false},
		// The keypad's '+'.
		{"bigger on keypad", notes, '+', '+', ctrl, Ref{3, 0}, true},
		{"shifted Ctrl+Shift+-", notes, '-', '_', ctrl | shift, Ref{}, false},
	}
	for _, c := range cases {
		r, ok := c.bar.Lookup(c.key, c.shifted, c.mods)
		if ok != c.ok || r != c.want {
			t.Errorf("%s: Lookup(%q, %q, %v) = %v %v, want %v %v", c.name, c.key, c.shifted, c.mods, r, ok, c.want, c.ok)
		}
	}
}

func TestLookupConflicts(t *testing.T) {
	sc := func(m driver.Modifiers, k driver.Key) driver.Shortcut { return driver.Shortcut{Mods: m, Key: k} }
	b := Build(&driver.MenuBarSpec{Menus: []driver.MenuSpec{
		{Title: "A", Items: []driver.MenuItemSpec{
			{ID: 1, Title: "first", Shortcut: sc(primary, 'K')},
			{ID: 2, Title: "plus", Shortcut: sc(primary, '+')},
		}},
		{Title: "B", Items: []driver.MenuItemSpec{
			{ID: 3, Title: "second", Shortcut: sc(primary, 'K')},
			{ID: 4, Title: "shift equals", Shortcut: sc(primary|shift, '=')},
			// A role item keeps its default shortcut and may conflict too.
			{Role: driver.ItemCopy},
			{ID: 5, Title: "physical ctrl", Shortcut: sc(ctrl, 'C')},
		}},
	}}, "App")
	for _, c := range []struct {
		name         string
		key, shifted driver.Key
		mods         driver.Modifiers
		want         Ref
	}{
		{"first in menu order", 'K', 'K', ctrl, Ref{0, 0}},
		{"exact beats shifted", '=', '+', ctrl | shift, Ref{1, 1}},
		{"role default first", 'C', 'C', ctrl, Ref{1, 2}},
	} {
		if r, ok := b.Lookup(c.key, c.shifted, c.mods); !ok || r != c.want {
			t.Errorf("%s: got %v %v, want %v", c.name, r, ok, c.want)
		}
	}
}

func TestLayout(t *testing.T) {
	m := Metrics{Width: 600, Start: 80, Pad: 7, MinDrag: 60, TitleGap: 12, TitleMargin: 80}
	widths := []float64{50, 30, 30, 40} // "Notes", "File", "Edit", "Format"

	s := Layout(widths, m)
	want := []Span{{80, 64}, {144, 44}, {188, 44}, {232, 54}}
	if !reflect.DeepEqual(s.Titles, want) {
		t.Fatalf("titles = %v, want %v", s.Titles, want)
	}
	if s.End != 286 {
		t.Errorf("end = %v, want 286", s.End)
	}
	// The title keeps End+TitleGap from both edges.
	if s.TitleRoom != 600-2*298 {
		t.Errorf("title room = %v, want %v", s.TitleRoom, 600-2*298)
	}

	// No strip: the window title keeps its usual margin.
	if s := Layout(nil, m); len(s.Titles) != 0 || s.End != 80 || s.TitleRoom != 600-160 {
		t.Errorf("empty strip = %+v", s)
	}
	// A short strip does not move the title past its usual margin.
	if s := Layout([]float64{0}, Metrics{Width: 600, Start: 60, Pad: 4, MinDrag: 60, TitleGap: 8, TitleMargin: 80}); s.TitleRoom != 600-160 || s.End != 68 {
		t.Errorf("short strip = %+v", s)
	}

	// Narrower: the titles that leave less than MinDrag are dropped from the right.
	for _, c := range []struct {
		width float64
		shown int
	}{
		{346, 4}, // 286 + 60 exactly fits
		{345.9, 3},
		{292, 3},
		{291, 2},
		{248, 2},
		{247, 1},
		{204, 1},
		{203, 0},
		{0, 0},
	} {
		mm := m
		mm.Width = c.width
		s := Layout(widths, mm)
		if len(s.Titles) != c.shown {
			t.Errorf("width %v: %d titles shown, want %d", c.width, len(s.Titles), c.shown)
			continue
		}
		end := m.Start
		if c.shown > 0 {
			end = s.Titles[c.shown-1].End()
		}
		if s.End != end {
			t.Errorf("width %v: end = %v, want %v", c.width, s.End, end)
		}
		if c.shown > 0 && s.End+m.MinDrag > c.width {
			t.Errorf("width %v: drag area %v < MinDrag", c.width, c.width-s.End)
		}
		if c.shown == 0 && s.TitleRoom != c.width-2*m.TitleMargin {
			t.Errorf("width %v: title room = %v", c.width, s.TitleRoom)
		}
	}

	// A wide first title is dropped even when a later one would fit: titles go left to right.
	if s := Layout([]float64{500, 10}, m); len(s.Titles) != 0 {
		t.Errorf("wide first title: %v", s.Titles)
	}
}
