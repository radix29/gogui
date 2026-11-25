package aqua

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// The PopUpButton's menu (PLAN.md S5.5k), over the fake shell's popups.

// popUpMenuWindow is popUpWindow with the window moved to (100, 100) of
// the screen, so a menu has room to open over any item.
func popUpMenuWindow(t *testing.T, picks *[]int, items ...string) (*PopUpButton, *Window, *fake.Surface, *fake.Shell) {
	t.Helper()
	p, w, sf, s := popUpWindow(t, picks, items...)
	sf.UserMove(pt(100, 100))
	return p, w, sf, s
}

// popUpAt is a point over the button (the surface's (10,38) 120×22).
var popUpAt = pt(70, 49)

func centre(r driver.Rect) driver.Point { return pt(r.X+r.W/2, r.Y+r.H/2) }

func near(a, b driver.Point) bool { return math.Abs(a.X-b.X) < 1e-9 && math.Abs(a.Y-b.Y) < 1e-9 }

// finishPick lets a pick's blink end.
func finishPick(s *fake.Shell) { s.FakeClock().Advance(menuBlinkOff + menuBlinkOn) }

// menuPopup returns the open menu's popup, or fails.
func menuPopup(t *testing.T, p *PopUpButton, s *fake.Shell) *fake.Popup {
	t.Helper()
	pp := s.GrabPopup()
	if !p.MenuOpen() || pp == nil || p.open.Content() != Element(p.menu) {
		t.Fatalf("menu not open (open %v, grab popup %v)", p.MenuOpen(), pp)
	}
	return pp
}

// checkTitleAligned checks that item i's title lies on the button's: both
// left edges and both centred on the bezel, in the surface's points.
func checkTitleAligned(t *testing.T, p *PopUpButton, pp *fake.Popup, i int) {
	t.Helper()
	r, _ := p.view.bezel()
	o := ToRoot(p.view, driver.Point{})
	m := p.menu
	l := m.layout(i)
	want := pt(o.X+r.X+tokens.PopUp.PaddingX.At(p.ControlSize), o.Y+r.Y+(r.H-l.Size().H)/2)
	got := pp.ToParent(m.TitleOrigin(i))
	if !near(got, want) {
		t.Errorf("item %d title at %v, the button's at %v", i, got, want)
	}
}

func TestPopUpButtonMenuOverSelected(t *testing.T) {
	var picks []int
	p, _, sf, s := popUpMenuWindow(t, &picks, "Red", "Green", "Blue")
	p.SetSelected(1)
	sf.Press(popUpAt, shell.ButtonPrimary)
	pp := menuPopup(t, p, s)
	cfg := pp.Config()
	if cfg.Anchor != (driver.Rect{X: 10, Y: 38, W: 120, H: 22}) || cfg.Adjust != popUpMenuAdjust || !cfg.Grab {
		t.Errorf("config %+v", cfg)
	}
	if pp.Rect().Size() != cfg.Size {
		t.Errorf("placed %v, asked %v", pp.Rect(), cfg.Size)
	}
	checkTitleAligned(t, p, pp, 1)
	m := p.menu
	// At least as wide as the button: the panel covers the bezel's width.
	if panel := m.Panel(); pp.Rect().X+panel.X > 10 || pp.Rect().X+panel.X+panel.W < 130 {
		t.Errorf("panel %v at %v does not cover the button", panel, pp.Rect())
	}
	var checked []bool
	for _, it := range m.Items() {
		checked = append(checked, it.Checked)
	}
	if !slices.Equal(checked, []bool{false, true, false}) || m.Highlighted() != 1 || m.ControlSize() != p.ControlSize ||
		m.ShadowMargin() != (driver.Insets{}) || !m.Tracking() || !m.HasFocus() {
		t.Errorf("menu: checked %v, highlighted %d, shadow %v, tracking %v", checked, m.Highlighted(), m.ShadowMargin(), m.Tracking())
	}
	// Pressed while open; the popup shows the menu.
	if p.view.State().Interaction != InteractionPressed || p.view.press.Down() {
		t.Errorf("state %v, press down %v", p.view.State(), p.view.press.Down())
	}
	got := paint(t, s, sf)
	if !strings.Contains(got, "fillRRect (10,39,120,20) r=5 solid #dedede") {
		t.Errorf("not drawn pressed:\n%s", got)
	}
	if d := pp.Dump(); !strings.Contains(d, `"Green"`) || !strings.Contains(d, `"Blue"`) {
		t.Errorf("popup paint:\n%s", d)
	}
	// A quick release in place is a click: the menu stays open.
	sf.Release(popUpAt, shell.ButtonPrimary)
	if !p.MenuOpen() || len(picks) != 0 {
		t.Fatalf("quick release: open %v, picks %v", p.MenuOpen(), picks)
	}
	// A click on Blue picks it after the blink.
	sf.Click(pp.ToParent(centre(m.ItemRect(2))))
	if !p.MenuOpen() || len(picks) != 0 {
		t.Fatal("reported before the blink")
	}
	finishPick(s)
	if !slices.Equal(picks, []int{2}) || p.Index != 2 || p.MenuOpen() || !pp.Closed() || m.Parent() != nil {
		t.Fatalf("pick: picks %v, index %d, open %v", picks, p.Index, p.MenuOpen())
	}
	if p.view.State().Interaction == InteractionPressed {
		t.Error("pressed after the close")
	}
	if got := paint(t, s, sf); !strings.Contains(got, `"Blue"`) || strings.Contains(got, "#dedede") {
		t.Errorf("after the pick:\n%s", got)
	}
	// Reopened over the new selection, the same menu element.
	sf.Click(popUpAt)
	pp = menuPopup(t, p, s)
	checkTitleAligned(t, p, pp, 2)
	if p.open.Content() != Element(m) || m.Highlighted() != 2 || !m.Items()[2].Checked || m.Items()[1].Checked {
		t.Error("reopened menu")
	}
}

// TestPopUpButtonMenuNoSelection: without a selection the menu opens over
// the first item, nothing checked.
func TestPopUpButtonMenuNoSelection(t *testing.T) {
	p, _, sf, s := popUpMenuWindow(t, nil, "Red", "Green")
	p.SetSelected(-1)
	sf.Click(popUpAt)
	pp := menuPopup(t, p, s)
	checkTitleAligned(t, p, pp, 0)
	for _, it := range p.menu.Items() {
		if it.Checked {
			t.Error("checked item")
		}
	}
}

func TestPopUpButtonPressDragRelease(t *testing.T) {
	var picks []int
	p, _, sf, s := popUpMenuWindow(t, &picks, "Red", "Green", "Blue")
	sf.Press(popUpAt, shell.ButtonPrimary)
	pp := menuPopup(t, p, s)
	q := pp.ToParent(centre(p.menu.ItemRect(2)))
	sf.Move(q)
	if p.menu.Highlighted() != 2 {
		t.Errorf("highlighted %d", p.menu.Highlighted())
	}
	sf.Release(q, shell.ButtonPrimary)
	finishPick(s)
	if !slices.Equal(picks, []int{2}) || p.MenuOpen() {
		t.Fatalf("drag pick: %v, open %v", picks, p.MenuOpen())
	}

	// Held past the release delay, released in place: the selected item
	// is picked and reported again.
	sf.Press(popUpAt, shell.ButtonPrimary)
	s.FakeClock().Advance(menuReleaseDelay)
	sf.Release(popUpAt, shell.ButtonPrimary)
	finishPick(s)
	if !slices.Equal(picks, []int{2, 2}) || p.MenuOpen() {
		t.Fatalf("held pick: %v, open %v", picks, p.MenuOpen())
	}

	// Dragged off the menu and released: a cancel.
	sf.Press(popUpAt, shell.ButtonPrimary)
	off := pt(195, 95)
	if pp := menuPopup(t, p, s); pp.Rect().Contains(off) {
		t.Fatalf("%v is in the menu %v", off, pp.Rect())
	}
	sf.Move(off)
	sf.Release(off, shell.ButtonPrimary)
	finishPick(s)
	if len(picks) != 2 || p.MenuOpen() || p.Index != 2 {
		t.Fatalf("cancel: %v, open %v", picks, p.MenuOpen())
	}
	// The window takes presses again.
	sf.Press(popUpAt, shell.ButtonPrimary)
	menuPopup(t, p, s)
}

func TestPopUpButtonMenuKeys(t *testing.T) {
	var picks []int
	p, _, sf, s := popUpMenuWindow(t, &picks, "Red", "Green", "Blue")
	if !p.view.Focus() {
		t.Fatal("focus")
	}
	// Modified arrows and other keys do not open it.
	for _, k := range []struct {
		key  driver.Key
		mods driver.Modifiers
	}{{driver.KeyDown, driver.ModShift}, {driver.KeyUp, driver.ModCommand}, {driver.KeySpace, driver.ModShift}, {driver.KeyReturn, 0}, {driver.KeyRight, 0}} {
		sf.Key(k.key, k.mods)
		if p.MenuOpen() {
			t.Fatalf("%v %v opened the menu", k.mods, k.key)
		}
	}
	for _, k := range []driver.Key{driver.KeySpace, driver.KeyUp, driver.KeyDown} {
		if !sf.KeyDown(k, 0) {
			t.Errorf("%v not handled", k)
		}
		pp := menuPopup(t, p, s)
		checkTitleAligned(t, p, pp, 0)
		if p.menu.Highlighted() != 0 || p.view.State().Interaction != InteractionPressed {
			t.Errorf("%v: highlighted %d", k, p.menu.Highlighted())
		}
		sf.KeyUp(k, 0)
		sf.Key(driver.KeyEscape, 0)
		if p.MenuOpen() || len(picks) != 0 {
			t.Fatalf("Escape after %v: open %v, picks %v", k, p.MenuOpen(), picks)
		}
	}
	// Down opens, Down moves, Return picks.
	sf.Key(driver.KeyDown, 0)
	sf.Key(driver.KeyDown, 0)
	sf.Key(driver.KeyReturn, 0)
	finishPick(s)
	if !slices.Equal(picks, []int{1}) || p.Index != 1 || p.MenuOpen() || !p.view.HasFocus() {
		t.Fatalf("Return: picks %v, open %v, focused %v", picks, p.MenuOpen(), p.view.HasFocus())
	}
	// A repeated arrow opens nothing.
	if sf.KeyRepeat(driver.KeyDown, 0); p.MenuOpen() {
		t.Error("auto-repeat opened the menu")
	}
	// Disabled: no key opens it.
	p.SetEnabled(false)
	for _, k := range []driver.Key{driver.KeySpace, driver.KeyUp, driver.KeyDown} {
		if sf.Key(k, 0); p.MenuOpen() {
			t.Errorf("disabled: %v opened the menu", k)
		}
	}
}

// TestPopUpButtonMenuClosesWithoutReport: every change while open closes
// the menu, and nothing is reported.
func TestPopUpButtonMenuClosesWithoutReport(t *testing.T) {
	for _, c := range []struct {
		name string
		do   func(p *PopUpButton, w *Window, sf *fake.Surface, s *fake.Shell)
	}{
		{"SetItems", func(p *PopUpButton, _ *Window, _ *fake.Surface, _ *fake.Shell) { p.SetItems([]string{"A", "B"}) }},
		{"SetSelected", func(p *PopUpButton, _ *Window, _ *fake.Surface, _ *fake.Shell) { p.SetSelected(2) }},
		{"SetControlSize", func(p *PopUpButton, _ *Window, _ *fake.Surface, _ *fake.Shell) {
			p.SetControlSize(driver.ControlSizeSmall)
		}},
		{"disable", func(p *PopUpButton, _ *Window, _ *fake.Surface, _ *fake.Shell) { p.SetEnabled(false) }},
		{"hide", func(p *PopUpButton, _ *Window, _ *fake.Surface, _ *fake.Shell) { p.SetHidden(true) }},
		{"Destroy", func(p *PopUpButton, _ *Window, _ *fake.Surface, _ *fake.Shell) { p.Destroy() }},
		{"window close", func(_ *PopUpButton, w *Window, _ *fake.Surface, _ *fake.Shell) { w.Close() }},
		{"deactivation", func(_ *PopUpButton, _ *Window, _ *fake.Surface, s *fake.Shell) { s.Deactivate() }},
		{"minimize", func(_ *PopUpButton, _ *Window, sf *fake.Surface, _ *fake.Shell) { sf.Minimize() }},
		{"dismissed", func(_ *PopUpButton, _ *Window, _ *fake.Surface, s *fake.Shell) { s.GrabPopup().UserDismiss() }},
		{"outside press", func(_ *PopUpButton, _ *Window, sf *fake.Surface, _ *fake.Shell) {
			sf.Click(pt(195, 95))
		}},
		{"enabled again, shown again", func(p *PopUpButton, _ *Window, _ *fake.Surface, _ *fake.Shell) {
			p.SetEnabled(true) // no change while open: stays open
			p.SetHidden(false)
			if !p.MenuOpen() {
				panic("closed")
			}
			p.CloseMenu()
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var picks []int
			p, w, sf, s := popUpMenuWindow(t, &picks, "Red", "Green", "Blue")
			sf.Click(popUpAt)
			pp := menuPopup(t, p, s)
			sf.Move(pp.ToParent(centre(p.menu.ItemRect(1))))
			c.do(p, w, sf, s)
			finishPick(s)
			if p.MenuOpen() || !pp.Closed() || len(picks) != 0 || p.menu.Tracking() || p.menu.Parent() != nil {
				t.Fatalf("open %v, popup closed %v, picks %v, tracking %v", p.MenuOpen(), pp.Closed(), picks, p.menu.Tracking())
			}
			if p.view.State().Interaction == InteractionPressed {
				t.Error("still pressed")
			}
		})
	}
}

// TestPopUpButtonMenuPickAfterParentChange: a pick reported while the
// button cannot take it (hidden or disabled through a parent, removed
// from its window) closes the menu without a report.
func TestPopUpButtonMenuPickAfterParentChange(t *testing.T) {
	for _, c := range []struct {
		name string
		do   func(p *PopUpButton)
	}{
		{"parent hidden", func(p *PopUpButton) { p.view.Parent().base().SetHidden(true) }},
		{"parent disabled", func(p *PopUpButton) { p.view.Parent().base().SetEnabled(false) }},
		{"removed", func(p *PopUpButton) { p.view.RemoveFromParent() }},
	} {
		t.Run(c.name, func(t *testing.T) {
			var picks []int
			p, _, sf, s := popUpMenuWindow(t, &picks, "Red", "Green", "Blue")
			p.view.Focus()
			sf.Key(driver.KeyDown, 0)
			menuPopup(t, p, s)
			sf.Key(driver.KeyDown, 0)
			c.do(p)
			sf.Key(driver.KeyReturn, 0)
			finishPick(s)
			if p.MenuOpen() || len(picks) != 0 || p.Index != 0 {
				t.Fatalf("open %v, picks %v, index %d", p.MenuOpen(), picks, p.Index)
			}
		})
	}
}

// TestPopUpButtonMenuDoesNotOpen: no items, a disabled button or a hidden
// window open no menu; the press then only presses.
func TestPopUpButtonMenuDoesNotOpen(t *testing.T) {
	p, _, sf, s := popUpMenuWindow(t, nil)
	sf.Press(popUpAt, shell.ButtonPrimary)
	if p.MenuOpen() || s.GrabPopup() != nil || !p.view.press.Down() || p.view.Activate() {
		t.Fatal("opened without items")
	}
	sf.Release(popUpAt, shell.ButtonPrimary)
	p.SetItems([]string{"Red"})
	p.SetEnabled(false)
	if sf.Click(popUpAt); p.MenuOpen() || p.view.Activate() {
		t.Fatal("opened disabled")
	}
	// Outside a window.
	q := newPopUp(t, "Red")
	if q.view.Activate() || q.MenuOpen() {
		t.Fatal("opened outside a window")
	}
}

// TestPopUpButtonMenuPlacement: the menu slides to stay on the screen; a
// menu taller than the screen gets shorter and scrolls so its items stay
// where they were, the selected one on the button.
func TestPopUpButtonMenuPlacement(t *testing.T) {
	items := make([]string, 40)
	for i := range items {
		items[i] = fmt.Sprint("Item ", i)
	}
	t.Run("tall", func(t *testing.T) {
		var picks []int
		p, _, sf, s := popUpMenuWindow(t, &picks, items...)
		s.SetWorkArea(driver.Rect{W: 800, H: 400})
		p.SetSelected(20)
		sf.Click(popUpAt)
		pp := menuPopup(t, p, s)
		m := p.menu
		r := pp.Rect()
		if top, bottom := r.Y+100, r.Y+r.H+100; top != 0 || bottom != 400 || r.H >= pp.Config().Size.H {
			t.Fatalf("placed %v (screen %g to %g), asked %v", r, top, bottom, pp.Config().Size)
		}
		if m.MaxScroll() == 0 || m.ScrollOffset() == 0 {
			t.Fatalf("not scrolled: %g of %g", m.ScrollOffset(), m.MaxScroll())
		}
		checkTitleAligned(t, p, pp, 20)
		if ir, a := m.ItemRect(20), m.itemsArea(); ir.Y < a.Y || ir.Y+ir.H > a.Y+a.H {
			t.Errorf("item 20 %v outside the items area %v", ir, a)
		}
		// The arrow scrolls; a pick reports the item under the pointer.
		sf.Click(pp.ToParent(centre(m.ItemRect(21))))
		finishPick(s)
		if !slices.Equal(picks, []int{21}) {
			t.Errorf("picks %v", picks)
		}
	})
	t.Run("selected near the end", func(t *testing.T) {
		p, _, sf, s := popUpMenuWindow(t, nil, items...)
		s.SetWorkArea(driver.Rect{W: 800, H: 400})
		p.SetSelected(39)
		sf.Click(popUpAt)
		pp := menuPopup(t, p, s)
		m := p.menu
		// The items are cut above the screen: the selected one shows, at
		// the least scroll.
		if ir, a := m.ItemRect(39), m.itemsArea(); ir.Y < a.Y || ir.Y+ir.H > a.Y+a.H {
			t.Errorf("item 39 %v outside the items area %v (scroll %g of %g)", ir, a, m.ScrollOffset(), m.MaxScroll())
		}
		if pp.Rect().Y+100 != 0 {
			t.Errorf("placed %v", pp.Rect())
		}
	})
	t.Run("slides", func(t *testing.T) {
		p, _, sf, s := popUpWindow(t, nil, "Red", "Green", "Blue")
		sf.UserMove(pt(700, 0))
		s.SetWorkArea(driver.Rect{W: 800, H: 600})
		p.SetSelected(2)
		sf.Click(popUpAt)
		pp := menuPopup(t, p, s)
		r := pp.Rect()
		// Slid down to the screen's top (the item over the button would be
		// above it) and left of its right edge (the button ends at 830).
		if r.Y != 0 || r.X+r.W+700 != 800 || r.Size() != pp.Config().Size || p.menu.ScrollOffset() != 0 {
			t.Errorf("placed %v, asked %+v", r, pp.Config())
		}
	})
}

// TestPopUpButtonMenuOwnShadow: where the shell draws no popup shadow
// (gtk4), the menu draws its own and the popup grows by its margin, the
// titles still aligned.
func TestPopUpButtonMenuOwnShadow(t *testing.T) {
	p, _, sf, s := popUpMenuWindow(t, nil, "Red", "Green", "Blue")
	s.SetCaps(shell.Caps{})
	p.SetSelected(1)
	sf.Click(popUpAt)
	pp := menuPopup(t, p, s)
	m := p.menu
	in := m.ShadowMargin()
	if in == (driver.Insets{}) {
		t.Fatal("no own shadow")
	}
	if panel := m.Panel(); panel.X != in.Left || panel.Y != in.Top || pp.Rect().W != panel.W+in.Left+in.Right ||
		pp.Rect().H != panel.H+in.Top+in.Bottom {
		t.Errorf("panel %v in %v, margin %v", panel, pp.Rect(), in)
	}
	checkTitleAligned(t, p, pp, 1)
	if s.FakeClock().Flush(); !strings.HasPrefix(pp.Dump(), "shadow (") {
		t.Errorf("no shadow painted:\n%s", pp.Dump())
	}
	// Back on: the next opening has none.
	p.CloseMenu()
	s.SetCaps(shell.Caps{PopupShadow: true})
	sf.Click(popUpAt)
	menuPopup(t, p, s)
	if m.ShadowMargin() != (driver.Insets{}) {
		t.Error("shadow kept")
	}
}

// TestPopUpButtonMenuSmall: the menu takes the button's control size.
func TestPopUpButtonMenuSmall(t *testing.T) {
	p, _, sf, s := popUpMenuWindow(t, nil, "Red", "Green")
	p.SetControlSize(driver.ControlSizeSmall)
	sf.Click(popUpAt)
	pp := menuPopup(t, p, s)
	if p.menu.ControlSize() != driver.ControlSizeSmall || p.menu.Font() != p.TitleFont() {
		t.Error("control size")
	}
	checkTitleAligned(t, p, pp, 0)
}
