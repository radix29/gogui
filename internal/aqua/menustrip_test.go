package aqua

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/aqua/menubar"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// stripSpec is a menu bar of five menus: the app menu (titled "Test" by
// stripBar) and four others, two of them with items.
var stripSpec = &driver.MenuBarSpec{Menus: []driver.MenuSpec{
	driver.DefaultAppMenu(),
	{Title: "File", Items: []driver.MenuItemSpec{{ID: 1, Title: "New", Shortcut: driver.Shortcut{Key: 'n', Mods: driver.ModPrimary}}}},
	driver.DefaultEditMenu(),
	{Title: "View"},
	{Title: "Window"},
}}

// stripBar is stripSpec's model for the app "Test".
func stripBar() *menubar.Bar { return menubar.Build(stripSpec, "Test") }

// stripWindow opens a titled window of the given width with the strip of
// bar and returns it painted once.
func stripWindow(t *testing.T, s *fake.Shell, width float64, title string, bar *menubar.Bar) (*Window, *fake.Surface) {
	t.Helper()
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Title: title, Size: driver.Size{W: width, H: 60}, Style: allStyles})
	w.setMenuBar(bar)
	paint(t, s, sf)
	return w, sf
}

// stripTitles returns the shown titles' slots in surface coordinates.
func stripTitles(w *Window) []driver.Rect {
	st := w.menuStrip()
	var out []driver.Rect
	for i := range st.Shown() {
		out = append(out, st.TitleRect(i))
	}
	return out
}

func slot(x, w float64) driver.Rect { return driver.Rect{X: x, W: w, H: 28} }

// TestMenuStripLayout checks the strip's geometry over the fake metrics
// (7 pt per rune): the slots from the lights on (8+52+12 = 72), each the
// title plus 7 pt on both sides; the titles that do not fit dropped from
// the right so 64 pt of caption stay; the window title centred in the room
// the strip leaves (the strip's end plus 16 pt on both sides), truncated or
// hidden by it; and the strip hidden when no title fits.
func TestMenuStripLayout(t *testing.T) {
	s := startShell(t)
	all := []driver.Rect{slot(72, 42), slot(114, 42), slot(156, 42), slot(198, 42), slot(240, 56)}
	for _, c := range []struct {
		width float64
		shown int
		title string
		x     float64
	}{
		{700, 5, "Hello", 332.5}, // room 700-2·312 = 76
		{655, 5, "Hel…", 313.5},  // room 31
		{631, 5, "…", 312},       // room 7
		{600, 5, "", 0},          // no room
		{360, 5, "", 0},          // 296+64 fits exactly
		{359, 4, "", 0},          // Window dropped
		{300, 3, "", 0},          // View too (240+64 > 300)
		{178, 1, "", 0},          // only the app menu: room 178-2·130 < 0
		{177, 0, "Hello", 71},    // no title fits: the strip is hidden, margin 68
	} {
		w, sf := stripWindow(t, s, c.width, "Hello", stripBar())
		if got := stripTitles(w); fmt.Sprint(got) != fmt.Sprint(all[:c.shown]) {
			t.Errorf("width %g: titles %v, want %v", c.width, got, all[:c.shown])
		}
		if st := w.menuStrip(); st.Hidden() != (c.shown == 0) {
			t.Errorf("width %g: strip hidden = %v", c.width, st.Hidden())
		}
		f, txt := w.TitleFrame()
		if txt != c.title || (txt != "" && (f.X != c.x || f.Y != 6)) {
			t.Errorf("width %g: title %q at %v, want %q at x=%g", c.width, txt, f, c.title, c.x)
		}
		// The regions: the caption, a client region per shown title, the
		// lights.
		rs := sf.ChromeRegions()
		if len(rs) != 1+c.shown+3 || rs[0].Kind != shell.RegionCaption || rs[1+c.shown].Kind != shell.RegionClose {
			t.Fatalf("width %g: regions %v", c.width, rs)
		}
		for i := range c.shown {
			if r := rs[1+i]; r.Kind != shell.RegionClient || r.Rect != all[i] {
				t.Errorf("width %g: region %d = %v, want client %v", c.width, i, r, all[i])
			}
		}
		w.Close()
	}

	// Hit-testing: a title is client (its press is delivered), the caption
	// right of the strip still drags, the lights stay buttons.
	w, sf := stripWindow(t, s, 700, "Hello", stripBar())
	for _, c := range []struct {
		p    driver.Point
		want shell.RegionKind
	}{
		{pt(80, 14), shell.RegionClient},
		{pt(113.5, 2), shell.RegionClient},
		{pt(295, 27), shell.RegionClient},
		{pt(297, 14), shell.RegionCaption},
		{pt(66, 14), shell.RegionCaption}, // between the lights and the strip
		{closeAt, shell.RegionClose},
		{pt(150, 40), shell.RegionClient}, // the content
	} {
		if got := sf.HitTest(c.p); got != c.want {
			t.Errorf("HitTest(%v) = %v, want %v", c.p, got, c.want)
		}
	}

	// Resizes re-lay the strip and its regions out.
	sf.UserResize(driver.Size{W: 300, H: 88})
	if n := w.menuStrip().Shown(); n != 3 || len(sf.ChromeRegions()) != 1+3+3 {
		t.Errorf("after the resize: %d titles, regions %v", n, sf.ChromeRegions())
	}
	sf.UserResize(driver.Size{W: 700, H: 88})
	if n := w.menuStrip().Shown(); n != 5 {
		t.Errorf("after growing: %d titles", n)
	}

	// Removing the bar restores the plain title bar.
	w.setMenuBar(nil)
	if w.menuStrip() != nil || len(sf.ChromeRegions()) != 4 {
		t.Errorf("no bar: strip %v, regions %v", w.menuStrip(), sf.ChromeRegions())
	}
	if f, txt := w.TitleFrame(); txt != "Hello" || f.X != 332.5 {
		t.Errorf("no bar: title %q at %v", txt, f)
	}
}

// TestMenuStripUntitled: an untitled window has no title bar, so no strip
// (D11), and setMenuBar does nothing to it.
func TestMenuStripUntitled(t *testing.T) {
	s := startShell(t)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 300, H: 60}, Style: driver.StyleResizable})
	w.setMenuBar(stripBar())
	if w.menuStrip() != nil || sf.ChromeRegions() != nil {
		t.Errorf("untitled window: strip %v, regions %v", w.menuStrip(), sf.ChromeRegions())
	}
}

// TestMenuStripHighlight: SetHighlighted repaints only on a change and
// clamps an unknown index to none.
func TestMenuStripHighlight(t *testing.T) {
	s := startShell(t)
	w, sf := stripWindow(t, s, 700, "Hello", stripBar())
	st := w.menuStrip()
	if st.Highlighted() != -1 {
		t.Fatalf("highlighted %d at the start", st.Highlighted())
	}
	st.SetHighlighted(2)
	if st.Highlighted() != 2 || !sf.Invalid() {
		t.Errorf("SetHighlighted(2): %d, invalid %v", st.Highlighted(), sf.Invalid())
	}
	d := paint(t, s, sf)
	if !strings.Contains(d, "fillRRect (156,4,42,20) r=5 solid #0000001a") {
		t.Errorf("no highlight behind Edit:\n%s", d)
	}
	st.SetHighlighted(2)
	if sf.Invalid() {
		t.Error("the same highlight repaints")
	}
	st.SetHighlighted(9)
	if st.Highlighted() != -1 {
		t.Errorf("SetHighlighted(9) = %d", st.Highlighted())
	}
	if d := paint(t, s, sf); strings.Contains(d, "r=5 solid #0000001a") {
		t.Errorf("highlight kept:\n%s", d)
	}
}

// TestMenuStripAccessibility: the strip is a menubar node in the title bar
// with a menubaritem per shown title; an open menu is a menu node with a
// menuitem per title item and a separator per separator.
func TestMenuStripAccessibility(t *testing.T) {
	s := startShell(t)
	w, _ := stripWindow(t, s, 300, "Hello", stripBar())
	tree := AccessibilityTree(w.Root())
	tb := tree.Find(a11y.RoleTitleBar, "Hello")
	if tb == nil || len(tb.Children) == 0 || tb.Children[0].Role != a11y.RoleMenuBar {
		t.Fatalf("no menu bar in the title bar:\n%s", tree.Dump())
	}
	var got []string
	for _, c := range tb.Children[0].Children {
		got = append(got, c.String())
	}
	want := []string{
		`menubaritem "Test" @72,0 42x28`,
		`menubaritem "File" @114,0 42x28`,
		`menubaritem "Edit" @156,0 42x28`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("strip nodes:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	m := newMenu(s.TextSystem())
	m.SetItems(shortcutMenuItems)
	m.SetHighlighted(1)
	p := openMenuPopup(t, w, m)
	mt := AccessibilityTree(p.Root())
	var items []string
	mt.Walk(func(n *a11y.Node, depth int) bool {
		if depth > 0 {
			items = append(items, strings.Repeat("  ", depth-1)+n.String())
		}
		return true
	})
	wantItems := []string{
		`menu "" [focusable] {focus} @0,0 ` + fmt.Sprintf("%gx%g", p.Rect().W, p.Rect().H),
	}
	if len(items) != 1+len(shortcutMenuItems) || items[0] != wantItems[0] {
		t.Fatalf("menu nodes:\n%s", strings.Join(items, "\n"))
	}
	for i, want := range []string{
		`  menuitem "New" value="Ctrl+N" {press} @`,
		`  menuitem "Open…" value="Ctrl+O" [selected] {press} @`,
		`  menuitem "Save As…" value="Ctrl+Shift+S" [disabled] @`,
		`  menuitem "Word Wrap" value="Ctrl+W" [checked] {press} @`,
		`  separator "" @`,
		`  menuitem "Open Recent" [disabled] @`,
		`  menuitem "Page Setup" {press} @`,
	} {
		if !strings.HasPrefix(items[1+i], want) {
			t.Errorf("item %d: %s, want %s…", i, items[1+i], want)
		}
	}
	r := m.ItemRect(1).Offset(ToRoot(m, driver.Point{}))
	if want := fmt.Sprintf("@%g,%g %gx%g", r.X, r.Y, r.W, r.H); !strings.HasSuffix(items[2], want) {
		t.Errorf("item frame %s, want %s", items[2], want)
	}
	p.Close()
}

// TestMenuStripStates pins the strip's paint (light and dark, key and
// inactive, a highlighted title, Increase Contrast, a narrow window with
// titles dropped and the window title truncated) as record dumps in
// testdata/menustrip.states, the title bar's commands only.
func TestMenuStripStates(t *testing.T) {
	var out strings.Builder
	for _, dark := range []bool{false, true} {
		for _, st := range []struct {
			name      string
			width     float64
			inactive  bool
			contrast  bool
			highlight int
		}{
			{name: "key", width: 700, highlight: -1},
			{name: "key, File highlighted", width: 700, highlight: 1},
			{name: "inactive", width: 700, inactive: true, highlight: -1},
			{name: "increased contrast", width: 700, contrast: true, highlight: -1},
			{name: "increased contrast, app menu highlighted", width: 700, contrast: true, highlight: 0},
			{name: "increased contrast, inactive", width: 700, inactive: true, contrast: true, highlight: -1},
			{name: "title truncated", width: 655, highlight: -1},
			{name: "narrow: three titles, no window title", width: 300, highlight: -1},
		} {
			s := startShell(t)
			w, sf := stripWindow(t, s, st.width, "Hello", stripBar())
			w.SetAppearance(driver.Appearance{Dark: dark, Accent: driver.DefaultAccent, IncreaseContrast: st.contrast})
			w.menuStrip().SetHighlighted(st.highlight)
			if st.inactive {
				s.Deactivate()
			}
			mode := "light"
			if dark {
				mode = "dark"
			}
			fmt.Fprintf(&out, "## %s, %s, %g wide\n%s\n", st.name, mode, st.width, titleBarLines(paint(t, s, sf)))
		}
	}
	got := out.String()
	path := filepath.Join("testdata", "menustrip.states")
	if *updateGolden {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to pin the dump)", err)
	}
	if w := strings.ReplaceAll(string(want), "\r\n", "\n"); got != w {
		t.Errorf("menu strip states differ from %s (rerun with -update if the change is intended):\n%s", path, got)
	}
}

// titleBarLines returns the lines of a window dump after the content
// area's fill (the first line): the title bar's.
func titleBarLines(dump string) string {
	lines := strings.Split(strings.TrimRight(dump, "\n"), "\n")
	return strings.Join(lines[1:], "\n")
}

// TestDriverMenuStrip: SetMenuBar gives every titled window the strip of
// the one bar, titled with the app name, before and after the windows
// were made; a window made later gets it too; untitled windows have none;
// without SetMenuBar there is no strip (D11).
func TestDriverMenuStrip(t *testing.T) {
	d := New(fake.New())
	h := &appLog{onReady: func() {
		mk := func(style driver.WindowStyle) *Window {
			w, err := d.NewWindow(driver.WindowConfig{Title: "W", Size: driver.Size{W: 500, H: 50}, Style: style}, &winLog{})
			if err != nil {
				t.Fatal(err)
			}
			return w.(*Window)
		}
		plain := mk(allStyles)
		if plain.menuStrip() != nil || d.MenuBarModel() != nil {
			t.Error("a strip without SetMenuBar")
		}
		untitled := mk(driver.StyleResizable)
		d.SetMenuBar(nil) // the default App and Edit menus
		if st := plain.menuStrip(); st == nil || st.Shown() != 2 || st.bar != d.MenuBarModel() ||
			st.bar.Menus[0].Title != "Test" || st.bar.Menus[1].Title != "Edit" {
			t.Fatalf("after SetMenuBar(nil): %+v", plain.menuStrip())
		}
		later := mk(allStyles)
		if st := later.menuStrip(); st == nil || st.bar != d.MenuBarModel() {
			t.Error("a later window has no strip")
		}
		d.SetMenuBar(stripSpec)
		for _, w := range []*Window{plain, later} {
			if st := w.menuStrip(); st == nil || st.Shown() != 5 || st.bar.Menus[3].Title != "View" {
				t.Errorf("after SetMenuBar: %+v", st)
			}
		}
		if untitled.menuStrip() != nil {
			t.Error("an untitled window has a strip")
		}
		d.Queue().Do(d.Quit)
	}}
	if err := runDriver(t, d, h); err != nil {
		t.Fatal(err)
	}
}

// TestDriverMenuStripBeforeRun: a bar set before Run is titled with the
// app name of Run's config.
func TestDriverMenuStripBeforeRun(t *testing.T) {
	d := New(fake.New())
	d.SetMenuBar(stripSpec)
	h := &appLog{onReady: func() {
		w, err := d.NewWindow(driver.WindowConfig{Title: "W", Size: driver.Size{W: 500, H: 50}, Style: allStyles}, &winLog{})
		if err != nil {
			t.Fatal(err)
		}
		if st := w.(*Window).menuStrip(); st == nil || st.bar.Menus[0].Title != "Test" {
			t.Errorf("strip %+v", st)
		}
		d.Queue().Do(d.Quit)
	}}
	if err := runDriver(t, d, h); err != nil {
		t.Fatal(err)
	}
}
