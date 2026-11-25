package aqua

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua/menubar"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// Opening the strip's menus by mouse (PLAN.md S5.6f), over the fake shell.

// mouseSpec has three menus: the app menu (slot 72+42), File (114+42) with
// two items and Tools (156+49) with an item, a separator and a disabled
// item.
var mouseSpec = &driver.MenuBarSpec{Menus: []driver.MenuSpec{
	driver.DefaultAppMenu(),
	{Title: "File", Items: []driver.MenuItemSpec{
		{ID: 1, Title: "New", Shortcut: driver.Shortcut{Key: 'n', Mods: driver.ModPrimary}},
		{ID: 2, Title: "Open"},
	}},
	{Title: "Tools", Items: []driver.MenuItemSpec{
		{ID: 3, Title: "Run"},
		{Separator: true},
		{ID: 4, Title: "Stop", Disabled: true},
	}},
}}

// The centres of the titles, in surface points.
var (
	appTitleAt   = pt(93, 14)
	fileTitleAt  = pt(135, 14)
	toolsTitleAt = pt(180.5, 14)
)

// mouseStrip opens a 700 pt window with mouseSpec's strip whose picks are
// logged in picks.
func mouseStrip(t *testing.T, picks *[]menubar.Ref) (*menuStrip, *Window, *fake.Surface, *fake.Shell) {
	t.Helper()
	s := startShell(t)
	w, sf := stripWindow(t, s, 700, "Hello", menubar.Build(mouseSpec, "Test"))
	w.menuPicked = func(r menubar.Ref) { *picks = append(*picks, r) }
	return w.menuStrip(), w, sf, s
}

// stripPopup returns the open menu's fake popup and checks the strip's
// state for menu i.
func stripPopup(t *testing.T, st *menuStrip, s *fake.Shell, i int) *fake.Popup {
	t.Helper()
	pp := s.GrabPopup()
	if st.OpenMenu() != i || pp == nil || st.MenuPopup() == nil || st.MenuPopup().Content() != Element(st.menu) {
		t.Fatalf("open menu %d (popup %v), want %d", st.OpenMenu(), pp, i)
	}
	if st.Highlighted() != i || !st.menu.Tracking() || !st.menu.HasFocus() {
		t.Fatalf("menu %d: highlighted %d, tracking %v", i, st.Highlighted(), st.menu.Tracking())
	}
	return pp
}

// wantClosed checks that no menu is open.
func wantClosed(t *testing.T, st *menuStrip, s *fake.Shell) {
	t.Helper()
	if st.OpenMenu() != -1 || st.MenuPopup() != nil || s.GrabPopup() != nil || st.Highlighted() != -1 || st.menu.Parent() != nil {
		t.Fatalf("still open: menu %d, highlighted %d, grab %v", st.OpenMenu(), st.Highlighted(), s.GrabPopup())
	}
}

func itemTitles(m *menu) []string {
	var out []string
	for _, it := range m.Items() {
		out = append(out, it.Title)
	}
	return out
}

// TestMenuStripClickOpens: a click on a title opens its menu below the
// title, the strip as the track, the items with their shortcuts; a click on
// an item picks it after the blink and reports its Ref.
func TestMenuStripClickOpens(t *testing.T) {
	var picks []menubar.Ref
	st, _, sf, s := mouseStrip(t, &picks)
	sf.Click(fileTitleAt)
	pp := stripPopup(t, st, s, 1)
	cfg := pp.Config()
	if cfg.Anchor != slot(114, 42) || cfg.Track != (driver.Rect{X: 72, W: 133, H: 28}) ||
		cfg.Adjust != menuStripAdjust || !cfg.Grab {
		t.Errorf("config %+v", cfg)
	}
	m := st.menu
	if r := pp.Rect(); r.X+m.Panel().X != 114 || r.Y+m.Panel().Y != 28 || r.Size() != cfg.Size {
		t.Errorf("placed %v, panel %v", r, m.Panel())
	}
	if items := m.Items(); len(items) != 2 || items[0].Title != "New" || items[0].Shortcut != "Ctrl+N" || m.Highlighted() != -1 {
		t.Errorf("items %+v, highlighted %d", items, m.Highlighted())
	}
	paint(t, s, sf)
	if d := pp.Dump(); !strings.Contains(d, `"New"`) || !strings.Contains(d, `"Ctrl+N"`) || !strings.Contains(d, `"Open"`) {
		t.Errorf("menu paint:\n%s", d)
	}
	sf.Click(pp.ToParent(centre(m.ItemRect(1))))
	if len(picks) != 0 {
		t.Fatal("reported before the blink")
	}
	finishPick(s)
	if !slices.Equal(picks, []menubar.Ref{{Menu: 1, Item: 1}}) || !pp.Closed() {
		t.Fatalf("picks %v, closed %v", picks, pp.Closed())
	}
	wantClosed(t, st, s)

	// A disabled item, a separator and the padding pick nothing.
	sf.Click(toolsTitleAt)
	pp = stripPopup(t, st, s, 2)
	for _, i := range []int{1, 2} {
		sf.Click(pp.ToParent(centre(m.ItemRect(i))))
	}
	finishPick(s)
	if len(picks) != 1 || st.OpenMenu() != 2 {
		t.Errorf("picks %v, open %d", picks, st.OpenMenu())
	}
}

// TestMenuStripCloseOnTitle: a press on the open title closes the menu
// without a report, and its release does not open it again; the next
// click does.
func TestMenuStripCloseOnTitle(t *testing.T) {
	var picks []menubar.Ref
	st, _, sf, s := mouseStrip(t, &picks)
	sf.Click(fileTitleAt)
	stripPopup(t, st, s, 1)
	sf.Press(fileTitleAt, shell.ButtonPrimary)
	wantClosed(t, st, s)
	sf.Release(fileTitleAt, shell.ButtonPrimary)
	wantClosed(t, st, s)
	sf.Click(fileTitleAt)
	stripPopup(t, st, s, 1)
	if len(picks) != 0 {
		t.Errorf("picks %v", picks)
	}
}

// TestMenuStripHoverSwitch: with a menu open, moving over another title
// switches to its menu in the same popup, repositioned under that title;
// moving off the strip and the menu keeps the menu; a press on the new
// title closes it.
func TestMenuStripHoverSwitch(t *testing.T) {
	var picks []menubar.Ref
	st, _, sf, s := mouseStrip(t, &picks)
	sf.Click(fileTitleAt)
	pp := stripPopup(t, st, s, 1)
	m := st.menu
	sf.Move(pp.ToParent(centre(m.ItemRect(0))))
	if m.Highlighted() != 0 {
		t.Fatalf("hover: highlighted %d", m.Highlighted())
	}
	sf.Move(toolsTitleAt)
	if got := stripPopup(t, st, s, 2); got != pp {
		t.Fatal("switching opened another popup")
	}
	if r := pp.Rect(); r.X+m.Panel().X != 156 || r.Size() != pp.Config().Size || pp.Config().Track != (driver.Rect{X: 72, W: 133, H: 28}) {
		t.Errorf("switched: placed %v, config %+v", r, pp.Config())
	}
	if got := itemTitles(m); fmt.Sprint(got) != "[Run  Stop]" || m.Highlighted() != -1 {
		t.Errorf("switched items %q, highlighted %d", got, m.Highlighted())
	}
	sf.Move(appTitleAt)
	stripPopup(t, st, s, 0)
	sf.Move(pt(600, 14)) // the caption: the menu stays
	stripPopup(t, st, s, 0)
	sf.Move(pt(600, 40))
	stripPopup(t, st, s, 0)
	sf.Move(fileTitleAt)
	stripPopup(t, st, s, 1)
	sf.Press(fileTitleAt, shell.ButtonPrimary)
	wantClosed(t, st, s)
	sf.Release(fileTitleAt, shell.ButtonPrimary)
	// Closed, moves over the titles open nothing.
	sf.Move(toolsTitleAt)
	wantClosed(t, st, s)
	if len(picks) != 0 {
		t.Errorf("picks %v", picks)
	}
}

// TestMenuStripPressDragRelease: a press on a title, a drag into its menu
// and the release over an item pick it; a press dragged across titles
// switches with the button still held and picks in the new menu; a late
// release over the strip leaves the menu open, one off the menu and the
// strip cancels.
func TestMenuStripPressDragRelease(t *testing.T) {
	var picks []menubar.Ref
	st, _, sf, s := mouseStrip(t, &picks)
	sf.Press(fileTitleAt, shell.ButtonPrimary)
	pp := stripPopup(t, st, s, 1)
	m := st.menu
	sf.Move(pp.ToParent(centre(m.ItemRect(0))))
	sf.Release(pp.ToParent(centre(m.ItemRect(0))), shell.ButtonPrimary)
	finishPick(s)
	if !slices.Equal(picks, []menubar.Ref{{Menu: 1, Item: 0}}) {
		t.Fatalf("picks %v", picks)
	}
	wantClosed(t, st, s)

	// Across the titles, quickly: the release picks at once in Tools.
	picks = nil
	sf.Press(fileTitleAt, shell.ButtonPrimary)
	pp = stripPopup(t, st, s, 1)
	sf.Move(toolsTitleAt)
	stripPopup(t, st, s, 2)
	if _, _, b := pp.Pointer(); b != shell.ButtonPrimary {
		t.Fatalf("held buttons after the switch: %v", b)
	}
	run := pp.ToParent(centre(m.ItemRect(0)))
	sf.Move(run)
	sf.Release(run, shell.ButtonPrimary)
	finishPick(s)
	if !slices.Equal(picks, []menubar.Ref{{Menu: 2, Item: 0}}) {
		t.Fatalf("across the titles: picks %v", picks)
	}
	wantClosed(t, st, s)

	// A late release on the title, and one after dragging into the menu
	// and back, leave the menu open for a click.
	picks = nil
	sf.Press(fileTitleAt, shell.ButtonPrimary)
	s.FakeClock().Advance(time.Second)
	sf.Release(fileTitleAt, shell.ButtonPrimary)
	stripPopup(t, st, s, 1)
	sf.Press(fileTitleAt, shell.ButtonPrimary) // closes
	sf.Release(fileTitleAt, shell.ButtonPrimary)
	sf.Press(fileTitleAt, shell.ButtonPrimary)
	pp = stripPopup(t, st, s, 1)
	sf.Move(pp.ToParent(centre(m.ItemRect(1))))
	sf.Move(pt(140, 10))
	sf.Release(pt(140, 10), shell.ButtonPrimary)
	stripPopup(t, st, s, 1)
	// Then a click picks.
	sf.Click(pp.ToParent(centre(m.ItemRect(1))))
	finishPick(s)
	if !slices.Equal(picks, []menubar.Ref{{Menu: 1, Item: 1}}) {
		t.Fatalf("click after the release on the strip: picks %v", picks)
	}

	// Dragged off the menu and the strip: the release cancels.
	picks = nil
	sf.Press(fileTitleAt, shell.ButtonPrimary)
	pp = stripPopup(t, st, s, 1)
	sf.Move(pp.ToParent(centre(m.ItemRect(1))))
	sf.Move(pt(600, 50))
	sf.Release(pt(600, 50), shell.ButtonPrimary)
	wantClosed(t, st, s)
	if len(picks) != 0 {
		t.Errorf("cancel: picks %v", picks)
	}
}

// TestMenuStripClosesWithoutReport: the open menu closes without a report,
// the strip's state reset, on a dismissal, an outside press, the key loss,
// minimizing, the window's close, SetMenuBar and a relayout that drops the
// open title; a pick blinking then is not reported.
func TestMenuStripClosesWithoutReport(t *testing.T) {
	for _, c := range []struct {
		name string
		do   func(w *Window, sf *fake.Surface, s *fake.Shell)
	}{
		{"dismissed", func(_ *Window, _ *fake.Surface, s *fake.Shell) { s.GrabPopup().UserDismiss() }},
		{"outside press", func(_ *Window, sf *fake.Surface, _ *fake.Shell) { sf.Click(pt(600, 50)) }},
		{"deactivation", func(_ *Window, _ *fake.Surface, s *fake.Shell) { s.Deactivate() }},
		{"minimize", func(_ *Window, sf *fake.Surface, _ *fake.Shell) { sf.Minimize() }},
		{"window close", func(w *Window, _ *fake.Surface, _ *fake.Shell) { w.Close() }},
		{"SetMenuBar", func(w *Window, _ *fake.Surface, _ *fake.Shell) { w.setMenuBar(stripBar()) }},
		{"SetMenuBar nil", func(w *Window, _ *fake.Surface, _ *fake.Shell) { w.setMenuBar(nil) }},
		{"relayout", func(w *Window, _ *fake.Surface, _ *fake.Shell) {
			w.layout(driver.Size{W: 210, H: 60}) // only the app menu fits
		}},
		{"blinking", func(w *Window, sf *fake.Surface, s *fake.Shell) {
			m := w.menuStrip().menu
			sf.Click(s.GrabPopup().ToParent(centre(m.ItemRect(0))))
			w.menuStrip().CloseMenu()
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var picks []menubar.Ref
			st, w, sf, s := mouseStrip(t, &picks)
			sf.Click(fileTitleAt)
			pp := stripPopup(t, st, s, 1)
			m := st.menu
			sf.Move(pp.ToParent(centre(m.ItemRect(0))))
			c.do(w, sf, s)
			finishPick(s)
			if !pp.Closed() || len(picks) != 0 || m.Tracking() || m.Parent() != nil || st.OpenMenu() != -1 || st.Highlighted() != -1 {
				t.Fatalf("popup closed %v, picks %v, tracking %v, open %d, highlighted %d",
					pp.Closed(), picks, m.Tracking(), st.OpenMenu(), st.Highlighted())
			}
		})
	}
}

// TestMenuStripOwnShadow: where the shell draws no popup shadow the menu
// draws its own and the popup grows by its margin, the panel still on the
// title's left edge and right below the title bar.
func TestMenuStripOwnShadow(t *testing.T) {
	var picks []menubar.Ref
	st, _, sf, s := mouseStrip(t, &picks)
	s.SetCaps(shell.Caps{})
	sf.Click(toolsTitleAt)
	pp := stripPopup(t, st, s, 2)
	m := st.menu
	if m.ShadowMargin() == (driver.Insets{}) {
		t.Fatal("no own shadow")
	}
	if r := pp.Rect(); r.X+m.Panel().X != 156 || r.Y+m.Panel().Y != 28 {
		t.Errorf("placed %v, panel %v", r, m.Panel())
	}
}

// TestMenuStripScreenEdges: at the right edge of the screen the menu
// slides left; on a short screen it gets shorter and scrolls.
func TestMenuStripScreenEdges(t *testing.T) {
	var picks []menubar.Ref
	st, _, sf, s := mouseStrip(t, &picks)
	s.SetWorkArea(driver.Rect{W: 200, H: 80})
	sf.Click(toolsTitleAt)
	pp := stripPopup(t, st, s, 2)
	r := pp.Rect()
	if r.MaxX() != 200 || r.Y != 28 || r.H != 52 || st.menu.MaxScroll() <= 0 {
		t.Errorf("placed %v, max scroll %g", r, st.menu.MaxScroll())
	}
}

// TestMenuStripDisabledWindow: no menu opens in a hidden strip or a closed
// window, and the strip does not open over another grab popup.
func TestMenuStripDoesNotOpen(t *testing.T) {
	var picks []menubar.Ref
	st, w, _, s := mouseStrip(t, &picks)
	st.SetHidden(true)
	if st.openMenu(1, menuOpening{}) {
		t.Error("opened while hidden")
	}
	st.SetHidden(false)
	if st.openMenu(5, menuOpening{}) {
		t.Error("opened a title that is not shown")
	}
	w.Close()
	if st.openMenu(1, menuOpening{}) || s.GrabPopup() != nil {
		t.Error("opened in a closed window")
	}
}

// TestDriverMenuPicks: the driver reports a pick of an enabled item with
// an ID to MenuItemSelected; disabled items, role items without an ID
// (S5.6g), separators and refs out of range report nothing.
func TestDriverMenuPicks(t *testing.T) {
	d := New(fake.New())
	var ids []int
	h := &menuApp{ids: &ids}
	h.onReady = func() {
		d.SetMenuBar(mouseSpec)
		w, err := d.NewWindow(driver.WindowConfig{Title: "W", Size: driver.Size{W: 500, H: 50}, Style: allStyles}, &winLog{})
		if err != nil {
			t.Fatal(err)
		}
		pick := w.(*Window).menuPicked
		for _, r := range []menubar.Ref{{Menu: 1, Item: 1}, {Menu: 2, Item: 2}, {Menu: 2, Item: 1}, {Menu: 0, Item: 3}, {Menu: 0, Item: 7}, {Menu: 9, Item: 0}, {Menu: 2, Item: 0}} {
			pick(r)
		}
		d.Queue().Do(d.Quit)
	}
	if err := runDriver(t, d, h); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids, []int{2, 3}) {
		t.Errorf("reported %v", ids)
	}
}

type menuApp struct {
	appLog
	ids *[]int
}

func (a *menuApp) MenuItemSelected(id int) { *a.ids = append(*a.ids, id) }
