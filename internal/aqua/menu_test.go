package aqua

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"

	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/render/record"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// sampleMenuItems is a menu with every kind of item: plain, checked,
// disabled, disabled and checked, a separator, and a long title.
var sampleMenuItems = []menuItem{
	{Title: "Small"},
	{Title: "Medium", Checked: true},
	{Title: "Large", Disabled: true},
	{Title: "Huge", Disabled: true, Checked: true},
	{Separator: true},
	{Title: "Custom size…"},
}

// shortcutMenuItems is a title-bar menu (S5.6b): items with shortcuts of
// different widths, one disabled, one checked, a separator, a submenu and
// an item without a shortcut.
var shortcutMenuItems = []menuItem{
	{Title: "New", Shortcut: "Ctrl+N"},
	{Title: "Open…", Shortcut: "Ctrl+O"},
	{Title: "Save As…", Shortcut: "Ctrl+Shift+S", Disabled: true},
	{Title: "Word Wrap", Shortcut: "Ctrl+W", Checked: true},
	{Separator: true},
	{Title: "Open Recent", Submenu: true, Disabled: true},
	{Title: "Page Setup"},
}

// TestMenuShortcutColumn checks the shortcut column (S5.6b): none without
// shortcuts or submenus, so the pop-up button's menu keeps its width; as
// wide as the widest shortcut or a chevron otherwise, with the gap in the
// fitting width; shortcuts right-aligned in it and chevrons at its right
// edge; the titles truncated before the column, the shortcuts never.
func TestMenuShortcutColumn(t *testing.T) {
	s := startShell(t)
	ts := s.TextSystem()
	mt := &tokens.Menu
	font := toggleFont(driver.ControlSizeRegular)
	m := newMenu(ts)
	m.SetItems(sampleMenuItems)
	if c := m.ShortcutColumn(); c != 0 {
		t.Errorf("column %v without shortcuts", c)
	}
	plain := m.FittingSize(driver.Size{W: 1e6, H: 1e6})

	m.SetItems(shortcutMenuItems)
	widestTitle := ts.NewLayout("Open Recent", font, 0).Size().W
	widestKey := ts.NewLayout("Ctrl+Shift+S", font, 0).Size().W
	if c := m.ShortcutColumn(); c != widestKey {
		t.Fatalf("column %v, want the widest shortcut %v", c, widestKey)
	}
	wantW := titleX() + widestTitle + mt.ShortcutGap + widestKey + mt.TitlePaddingRight
	fit := m.FittingSize(driver.Size{W: 1e6, H: 1e6})
	if fit.W < wantW || fit.W >= wantW+1 {
		t.Fatalf("fitting width %v, want %v rounded up", fit.W, wantW)
	}
	m.SetFrame(driver.Rect{W: fit.W, H: fit.H})
	right := fit.W - mt.TitlePaddingRight
	for i, it := range shortcutMenuItems {
		o := m.ShortcutOrigin(i)
		switch {
		case it.Shortcut != "":
			sz := ts.NewLayout(it.Shortcut, font, 0).Size()
			r := m.ItemRect(i)
			if o != pt(right-sz.W, r.Y+(r.H-sz.H)/2) {
				t.Errorf("item %d shortcut origin %v, want right edge %v", i, o, right)
			}
		case o != (driver.Point{}):
			t.Errorf("item %d without a shortcut has origin %v", i, o)
		}
		cr := m.ChevronRect(i)
		if it.Submenu {
			r := m.ItemRect(i)
			if cr != (driver.Rect{X: right - mt.SubmenuChevronSize/2, Y: r.Y + (r.H-mt.SubmenuChevronSize)/2, W: mt.SubmenuChevronSize / 2, H: mt.SubmenuChevronSize}) {
				t.Errorf("item %d chevron %v", i, cr)
			}
		} else if cr != (driver.Rect{}) {
			t.Errorf("item %d has a chevron %v", i, cr)
		}
	}
	if m.ShortcutOrigin(-1) != (driver.Point{}) || m.ChevronRect(99) != (driver.Rect{}) {
		t.Error("unknown index")
	}

	// A submenu alone gives a chevron-wide column; a shortcut on a submenu
	// item is not shown.
	m.SetItems([]menuItem{{Title: "A"}, {Title: "More", Submenu: true, Shortcut: "Ctrl+M"}})
	if c := m.ShortcutColumn(); c != mt.SubmenuChevronSize/2 {
		t.Errorf("submenu column %v, the shortcut counted", c)
	}
	m.SetItems([]menuItem{{Title: "A"}, {Title: "More", Submenu: true}})
	if c := m.ShortcutColumn(); c != mt.SubmenuChevronSize/2 {
		t.Errorf("submenu column %v, want a chevron", c)
	}
	if m.ShortcutOrigin(1) != (driver.Point{}) {
		t.Error("a submenu item has a shortcut origin")
	}

	// Back to plain items: no column, the old width.
	m.SetItems(sampleMenuItems)
	if got := m.FittingSize(driver.Size{W: 1e6, H: 1e6}); got != plain || m.ShortcutColumn() != 0 {
		t.Errorf("plain items again: %v, column %v; want %v", got, m.ShortcutColumn(), plain)
	}

	// A narrow panel truncates the titles before the column and keeps the
	// shortcuts whole, right-aligned.
	m.SetItems(shortcutMenuItems)
	m.SetFrame(driver.Rect{W: fit.W - 30, H: fit.H})
	rc := record.New(render.Size{W: 400, H: 400})
	m.Paint(rc)
	got := rc.Dump()
	if !strings.Contains(got, `"Page`+ellipsis+`"`) || strings.Contains(got, `"Open Recent"`) || !strings.Contains(got, `"Ctrl+Shift+S"`) {
		t.Errorf("narrow menu:\n%s", got)
	}

	// The column follows the control size.
	m.SetControlSize(driver.ControlSizeSmall)
	if c := m.ShortcutColumn(); c != ts.NewLayout("Ctrl+Shift+S", toggleFont(driver.ControlSizeSmall), 0).Size().W {
		t.Errorf("small column %v", c)
	}
}

// openMenuPopup opens m in a grab popup of w at (20, 40) of the content
// area, sized to the menu's fitting size.
func openMenuPopup(t *testing.T, w *Window, m *menu) *popupRoot {
	t.Helper()
	p, err := w.openPopup(popupOptions{
		Config: shell.PopupConfig{
			Anchor: driver.Rect{X: 20, Y: 40 + w.titleBarHeight(), W: 10, H: 10},
			Size:   m.FittingSize(driver.Size{W: 1e6, H: 1e6}),
			Grab:   true,
		},
		Content: m,
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// popupDump paints p's popup and returns its dump.
func popupDump(t *testing.T, s *fake.Shell, p *popupRoot) string {
	t.Helper()
	s.FakeClock().Flush()
	fp := p.Popup().(*fake.Popup)
	if err := fp.PaintErr(); err != nil {
		t.Fatalf("popup paint: %v\n%s", err, fp.Dump())
	}
	return fp.Dump()
}

// TestMenuGeometry checks the item model and the layout: the fitting
// size from the widest title, the item rects and hits, the title origins,
// the highlight skipping what cannot be picked, the minimum width, the
// shadow margin and the control size.
func TestMenuGeometry(t *testing.T) {
	s := startShell(t)
	ts := s.TextSystem()
	m := newMenu(ts)
	if got := m.FittingSize(driver.Size{W: 1e6, H: 1e6}); got != (driver.Size{W: tokens.Menu.HighlightInsetX + tokens.Menu.CheckColumnWidth + tokens.Menu.TitlePaddingRight, H: 2 * tokens.Menu.PaddingY}) {
		t.Errorf("empty menu fits %v", got)
	}
	m.SetItems(sampleMenuItems)
	mt := &tokens.Menu
	ih := mt.ItemHeight.At(driver.ControlSizeRegular)
	font := toggleFont(driver.ControlSizeRegular)
	widest := ts.NewLayout("Custom size…", font, 0).Size().W
	wantW := mt.HighlightInsetX + mt.CheckColumnWidth + widest + mt.TitlePaddingRight
	wantH := 2*mt.PaddingY + 5*ih + mt.SeparatorHeight
	fit := m.FittingSize(driver.Size{W: 1e6, H: 1e6})
	if fit.W < wantW || fit.W >= wantW+1 || fit.H != wantH {
		t.Fatalf("fitting size %v, want %g×%g (width rounded up)", fit, wantW, wantH)
	}
	if got := m.FittingSize(driver.Size{W: 50, H: 40}); got != (driver.Size{W: 50, H: 40}) {
		t.Errorf("clamped fitting size %v", got)
	}
	m.SetFrame(driver.Rect{X: 3, Y: 4, W: fit.W, H: fit.H})
	if m.Panel() != (driver.Rect{W: fit.W, H: fit.H}) || m.ContentHeight() != wantH {
		t.Errorf("panel %v, content height %v", m.Panel(), m.ContentHeight())
	}
	y := mt.PaddingY
	for i, it := range sampleMenuItems {
		h := ih
		if it.Separator {
			h = mt.SeparatorHeight
		}
		if r := m.ItemRect(i); r != (driver.Rect{Y: y, W: fit.W, H: h}) {
			t.Errorf("item %d rect %v, want y %g h %g", i, r, y, h)
		}
		if got := m.ItemAt(pt(10, y+h/2)); got != i {
			t.Errorf("ItemAt the middle of item %d = %d", i, got)
		}
		if got := m.ItemAt(pt(10, y)); got != i {
			t.Errorf("ItemAt the top of item %d = %d", i, got)
		}
		o := m.TitleOrigin(i)
		if it.Separator {
			if o != (driver.Point{}) {
				t.Errorf("separator title origin %v", o)
			}
		} else if lh := ts.NewLayout(it.Title, font, 0).Size().H; o != pt(mt.HighlightInsetX+mt.CheckColumnWidth, y+(h-lh)/2) {
			t.Errorf("item %d title origin %v", i, o)
		}
		if want := !it.Separator && !it.Disabled; m.Selectable(i) != want {
			t.Errorf("item %d selectable %v", i, !want)
		}
		y += h
	}
	for _, q := range []driver.Point{pt(10, 1), pt(10, wantH-1), pt(-1, 20), pt(fit.W, 20), pt(10, wantH+5)} {
		if got := m.ItemAt(q); got != -1 {
			t.Errorf("ItemAt %v = %d", q, got)
		}
	}
	if m.ItemRect(-1) != (driver.Rect{}) || m.ItemRect(6) != (driver.Rect{}) || m.Selectable(6) || m.Selectable(-1) {
		t.Error("unknown index")
	}

	// The highlight skips disabled items and separators.
	m.SetHighlighted(1)
	if m.Highlighted() != 1 {
		t.Errorf("highlighted %d", m.Highlighted())
	}
	for _, i := range []int{2, 3, 4, 9, -5} {
		m.SetHighlighted(i)
		if m.Highlighted() != -1 {
			t.Errorf("SetHighlighted(%d) highlights %d", i, m.Highlighted())
		}
	}
	m.SetHighlighted(5)
	m.SetItems(sampleMenuItems[:2])
	if m.Highlighted() != -1 || len(m.Items()) != 2 {
		t.Errorf("SetItems kept highlight %d, %d items", m.Highlighted(), len(m.Items()))
	}
	m.SetItems(sampleMenuItems)

	// The minimum width wins over a narrower content, not over a wider.
	m.SetMinWidth(300)
	if got := m.FittingSize(driver.Size{W: 1e6, H: 1e6}); got != (driver.Size{W: 300, H: wantH}) {
		t.Errorf("min width 300: %v", got)
	}
	m.SetMinWidth(10)
	if got := m.FittingSize(driver.Size{W: 1e6, H: 1e6}); got != fit {
		t.Errorf("min width 10: %v", got)
	}

	// The shadow margin is added around the panel.
	m.SetShadow(true)
	in := m.ShadowMargin()
	if in != (driver.Insets{Top: mt.ShadowBlur - mt.ShadowOffsetY, Left: mt.ShadowBlur, Bottom: mt.ShadowBlur + mt.ShadowOffsetY, Right: mt.ShadowBlur}) {
		t.Errorf("shadow margin %+v", in)
	}
	sfit := m.FittingSize(driver.Size{W: 1e6, H: 1e6})
	if sfit != (driver.Size{W: fit.W + in.Left + in.Right, H: fit.H + in.Top + in.Bottom}) {
		t.Errorf("fitting size with the shadow %v", sfit)
	}
	m.SetFrame(driver.Rect{W: sfit.W, H: sfit.H})
	if m.Panel() != (driver.Rect{X: in.Left, Y: in.Top, W: fit.W, H: fit.H}) || m.ItemRect(0).Y != in.Top+mt.PaddingY || m.ItemAt(pt(in.Left-1, in.Top+10)) != -1 {
		t.Errorf("panel %v, first item %v", m.Panel(), m.ItemRect(0))
	}
	m.SetShadow(false)

	// A smaller control size has lower items and a smaller font.
	m.SetControlSize(driver.ControlSizeSmall)
	if m.ControlSize() != driver.ControlSizeSmall || m.Font() != toggleFont(driver.ControlSizeSmall) {
		t.Error("control size")
	}
	if got := m.ContentHeight(); got != 2*mt.PaddingY+5*mt.ItemHeight.At(driver.ControlSizeSmall)+mt.SeparatorHeight {
		t.Errorf("small content height %v", got)
	}
	if got := m.FittingSize(driver.Size{W: 1e6, H: 1e6}); got.W >= fit.W {
		t.Errorf("small menu %v not narrower than %v", got, fit)
	}
}

// TestMenuInPopup: in a popup the menu paints with its window's
// appearance and the popup's scale, opaque or translucent as the popup
// reports, and repaints on an appearance change.
func TestMenuInPopup(t *testing.T) {
	var log []string
	w, sf, s, _ := popupWindow(t, &log)
	m := newMenu(s.TextSystem())
	m.SetItems(sampleMenuItems)
	p := openMenuPopup(t, w, m)
	a := w.Appearance()
	if got := popupDump(t, s, p); !strings.Contains(got, "solid "+hexColor(tokens.Menu.OpaqueBackground.Value(a))) {
		t.Errorf("opaque menu:\n%s", got)
	}
	a.Dark = true
	w.SetAppearance(a)
	if got := popupDump(t, s, p); !strings.Contains(got, "solid "+hexColor(tokens.Menu.OpaqueBackground.Value(a))) ||
		!strings.Contains(got, hexColor(tokens.Menu.Text.Value(a))) {
		t.Errorf("dark menu:\n%s", got)
	}
	p.Close()

	s.SetPopupTranslucent(true)
	sf.SetScale(1.5)
	p = openMenuPopup(t, w, m)
	if !p.Translucent() || p.Scale() != 1.5 || scaleIn(&m.Base) != 1.5 {
		t.Fatalf("translucent %v, scale %v", p.Translucent(), p.Scale())
	}
	got := popupDump(t, s, p)
	if !strings.Contains(got, "solid "+hexColor(tokens.Menu.Background.Value(a))) {
		t.Errorf("translucent menu:\n%s", got)
	}
	// The border and the separator are whole device pixels at scale 1.5
	// (two for one point), the separator on the pixel grid.
	if !strings.Contains(got, "strokeRRect (0.667,0.667,124.667,129.667) r=5.333 w=1.333 solid "+hexColor(tokens.Menu.Border.Value(a))) {
		t.Errorf("border at scale 1.5:\n%s", got)
	}
	if want := "fillRRect (10,98,106,1.333) solid " + hexColor(tokens.Menu.Separator.Value(a)); !strings.Contains(got, want) {
		t.Errorf("separator at scale 1.5, want %q:\n%s", want, got)
	}
	p.Close()
	if m.Parent() != nil || m.translucent() {
		t.Error("the closed popup kept its menu")
	}
}

// TestMenuOutsidePopup: outside a popup (a menu not yet opened) the menu
// paints opaque, in the default appearance; an empty frame paints nothing.
func TestMenuOutsidePopup(t *testing.T) {
	s := startShell(t)
	m := newMenu(s.TextSystem())
	m.SetItems(sampleMenuItems)
	rc := record.New(render.Size{W: 400, H: 400})
	m.Paint(rc)
	if len(rc.Ops()) != 0 {
		t.Errorf("empty frame painted %d ops", len(rc.Ops()))
	}
	fit := m.FittingSize(driver.Size{W: 1e6, H: 1e6})
	m.SetFrame(driver.Rect{W: fit.W, H: fit.H})
	rc = record.New(render.Size{W: 400, H: 400})
	m.Paint(rc)
	if got := rc.Dump(); !strings.Contains(got, "solid "+hexColor(tokens.Menu.OpaqueBackground.Value(driver.DefaultAppearance()))) {
		t.Errorf("menu outside a popup:\n%s", got)
	}
}

// TestMenuTruncatesAndClips: a panel narrower than the titles truncates
// them with an ellipsis; one lower than the items clips them to the part
// above the down arrow (S5.5i) and paints none below it.
func TestMenuTruncatesAndClips(t *testing.T) {
	s := startShell(t)
	m := newMenu(s.TextSystem())
	m.SetItems([]menuItem{{Title: "A rather long menu item title"}, {Title: "B"}, {Title: "C"}})
	fit := m.FittingSize(driver.Size{W: 1e6, H: 1e6})
	m.SetFrame(driver.Rect{W: fit.W - 40, H: tokens.Menu.PaddingY + 2*tokens.Menu.ItemHeight.At(0) + 4})
	rc := record.New(render.Size{W: 400, H: 400})
	m.Paint(rc)
	got := rc.Dump()
	if !strings.Contains(got, ellipsis+`"`) || strings.Contains(got, `"A rather long menu item title"`) {
		t.Errorf("not truncated:\n%s", got)
	}
	if !strings.Contains(got, `"B"`) || strings.Contains(got, `"C"`) || !strings.Contains(got, "clip=") {
		t.Errorf("not clipped to the panel:\n%s", got)
	}
	// Widening the panel again shows the whole title.
	m.SetFrame(driver.Rect{W: fit.W, H: fit.H})
	rc = record.New(render.Size{W: 400, H: 400})
	m.Paint(rc)
	if got := rc.Dump(); !strings.Contains(got, `"A rather long menu item title"`) || strings.Contains(got, "clip=") {
		t.Errorf("wide menu:\n%s", got)
	}
}

// TestMenuStates pins the paint of every item state (plain, checked,
// disabled, disabled checked, a separator, highlighted, highlighted
// checked) on the opaque and the translucent background, with and without
// the menu's own shadow, in light and dark and with Increase Contrast, at
// the regular and the small size, and scrolled with one or both scroll
// arrows (S5.5i), and with the shortcut column and a submenu chevron
// (S5.6b), and opened from an inactive window, whose highlight keeps the
// accent as a macOS menu's does (S6.1a), as record dumps in
// testdata/menu.states.
func TestMenuStates(t *testing.T) {
	var out strings.Builder
	for _, dark := range []bool{false, true} {
		for _, st := range []struct {
			name        string
			translucent bool
			shadow      bool
			contrast    bool
			highlight   int
			cs          driver.ControlSize
			height      float64 // a lower popup than the fitting one, scrolled by top
			top         float64
			items       []menuItem // nil: sampleMenuItems
			inactive    bool       // the parent window is not key (S6.1a)
		}{
			{name: "opaque", highlight: -1},
			{name: "opaque, highlighted", highlight: 0},
			{name: "opaque, highlighted checked", highlight: 1},
			{name: "translucent, highlighted", translucent: true, highlight: 5},
			{name: "shadow", shadow: true, highlight: -1},
			{name: "increased contrast", contrast: true, highlight: 0},
			{name: "small", cs: driver.ControlSizeSmall, highlight: 1},
			{name: "scrolled, both arrows", highlight: 1, height: 90, top: 20},
			{name: "scrolled to the end", highlight: 5, height: 90, top: 1e6},
			{name: "shortcuts", items: shortcutMenuItems, highlight: -1},
			{name: "shortcuts, highlighted", items: shortcutMenuItems, highlight: 1},
			{name: "shortcuts, highlighted checked", items: shortcutMenuItems, highlight: 3},
			{name: "shortcuts, small", items: shortcutMenuItems, cs: driver.ControlSizeSmall, highlight: 0},
			{name: "inactive parent, highlighted", highlight: 0, inactive: true},
			{name: "inactive parent, highlighted checked", highlight: 1, inactive: true},
			{name: "increased contrast, highlighted checked", contrast: true, highlight: 1},
			{name: "increased contrast, inactive parent, highlighted", contrast: true, highlight: 0, inactive: true},
		} {
			var log []string
			w, _, s, _ := popupWindow(t, &log)
			w.SetAppearance(driver.Appearance{Dark: dark, Accent: driver.DefaultAccent, IncreaseContrast: st.contrast})
			s.SetPopupTranslucent(st.translucent)
			m := newMenu(s.TextSystem())
			items := st.items
			if items == nil {
				items = sampleMenuItems
			}
			m.SetItems(items)
			m.SetControlSize(st.cs)
			m.SetShadow(st.shadow)
			m.SetHighlighted(st.highlight)
			if st.inactive {
				s.Deactivate()
			}
			p := openMenuPopupSized(t, w, m, st.height)
			if w.IsKey() == st.inactive || p.Popup() == nil {
				t.Fatalf("%s: key %v", st.name, w.IsKey())
			}
			m.SetScrollOffset(st.top)
			mode := "light"
			if dark {
				mode = "dark"
			}
			fmt.Fprintf(&out, "## %s, %s, %gx%g\n%s\n", st.name, mode, p.Rect().W, p.Rect().H, popupDump(t, s, p))
			p.Close()
		}
	}
	got := out.String()
	path := filepath.Join("testdata", "menu.states")
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
		t.Errorf("menu states differ from %s (rerun with -update if the change is intended):\n%s", path, got)
	}
}

// hexColor formats c as the recorder's dumps do.
func hexColor(c driver.Color) string {
	ch := func(v float32) int { return int(math.Min(math.Max(math.Round(float64(v)*255), 0), 255)) }
	s := fmt.Sprintf("#%02x%02x%02x", ch(c.R), ch(c.G), ch(c.B))
	if a := ch(c.A); a != 255 {
		s += fmt.Sprintf("%02x", a)
	}
	return s
}

// openMenuPopupSized is openMenuPopup with a popup h points high (0: the
// fitting height).
func openMenuPopupSized(t *testing.T, w *Window, m *menu, h float64) *popupRoot {
	t.Helper()
	size := m.FittingSize(driver.Size{W: 1e6, H: 1e6})
	if h > 0 {
		size.H = h
	}
	p, err := w.openPopup(popupOptions{
		Config: shell.PopupConfig{
			Anchor: driver.Rect{X: 20, Y: 40 + w.titleBarHeight(), W: 10, H: 10},
			Size:   size,
			Grab:   true,
		},
		Content: m,
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
