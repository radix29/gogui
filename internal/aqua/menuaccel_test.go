package aqua

import (
	"slices"
	"testing"

	"github.com/radix29/gogui/internal/driver"
)

// The accelerators of the menu bar (PLAN.md S5.6h), over the fake shell.

// accelSpec has the default app and Edit menus and a View menu with app
// items: a shortcut on a shifted character, a plain one, a disabled one
// and one on Return with no modifier.
var accelSpec = &driver.MenuBarSpec{Menus: []driver.MenuSpec{
	driver.DefaultAppMenu(),
	driver.DefaultEditMenu(),
	{Title: "View", Items: []driver.MenuItemSpec{
		{ID: 20, Title: "Zoom In", Shortcut: driver.Shortcut{Mods: driver.ModPrimary, Key: '+'}},
		{ID: 21, Title: "Zoom Out", Shortcut: driver.Shortcut{Mods: driver.ModPrimary, Key: '-'}},
		{ID: 22, Title: "Bold", Disabled: true, Shortcut: driver.Shortcut{Mods: driver.ModPrimary, Key: 'B'}},
		{ID: 23, Title: "Run", Shortcut: driver.Shortcut{Key: driver.KeyReturn}},
	}},
}}

const viewMenu = 2

// newAccelEnv is newRoleEnv with accelSpec as the bar.
func newAccelEnv(t *testing.T) *roleEnv {
	t.Helper()
	e := newRoleEnv(t)
	e.d.SetMenuBar(accelSpec)
	return e
}

// TestAccelShiftedShortcuts: an item's shortcut runs it with the menus
// closed, consuming the key; Primary+'+' matches Ctrl+Shift+'=' on the US
// layout and Ctrl+'+' on a layout with an unshifted '+'; a disabled item
// and an unknown key go on; auto-repeat runs the item again.
func TestAccelShiftedShortcuts(t *testing.T) {
	e := newAccelEnv(t)
	_, sf, _ := e.window(allStyles, nil)
	ctrl := driver.ModControl
	if !sf.KeyDown('=', ctrl|driver.ModShift) {
		t.Error("Ctrl+Shift+= (US) not consumed")
	}
	sf.KeyUp('=', ctrl|driver.ModShift)
	if !sf.KeyDown('-', ctrl) || !sf.KeyRepeat('-', ctrl) {
		t.Error("Ctrl+- not consumed")
	}
	sf.KeyUp('-', ctrl)
	if sf.Key('=', ctrl) {
		t.Error("Ctrl+= (no Shift) matched Zoom In")
	}
	if sf.Key('B', ctrl) {
		t.Error("the disabled Ctrl+B consumed")
	}
	if sf.Key('K', ctrl) {
		t.Error("Ctrl+K consumed")
	}
	e.s.SetShiftLayout(map[driver.Key]driver.Key{'+': '*'}) // German-like
	if !sf.Key('+', ctrl) {
		t.Error("Ctrl++ (German) not consumed")
	}
	if sf.Key('+', ctrl|driver.ModShift) {
		t.Error("Ctrl+Shift++ (German: '*') matched Zoom In")
	}
	e.s.SetShiftLayout(nil)
	if want := []int{20, 21, 21, 20}; !slices.Equal(e.h.picks, want) {
		t.Errorf("picks %v, want %v", e.h.picks, want)
	}
}

// TestAccelBeforeFocusAndDefault: a shortcut comes before the focused
// TextField (Ctrl+A through the Edit role) and before the default button
// (an item on Return); a disabled role item leaves the key to the field.
func TestAccelBeforeFocusAndDefault(t *testing.T) {
	e := newAccelEnv(t)
	h := &fieldLog{}
	tf := e.d.NewTextField(false, h).(*TextField)
	w, sf, _ := e.window(allStyles, tf)
	tf.view.Focus()
	sf.Type("hello")
	if !sf.Key('A', driver.ModControl) || selOf(tf) != "hello|0,5" {
		t.Errorf("Ctrl+A: %s", selOf(tf))
	}
	if !sf.Key('C', driver.ModControl) || clipText(e.s) != "hello" {
		t.Errorf("Ctrl+C: clipboard %q", clipText(e.s))
	}
	tf.Editor().SetSelection(5, 5)
	// Copy is disabled without a selection: the key reaches the field,
	// whose own Copy does nothing either.
	e.s.SetClipboardText("x")
	sf.Key('C', driver.ModControl)
	if clipText(e.s) != "x" {
		t.Errorf("Ctrl+C without a selection: clipboard %q", clipText(e.s))
	}

	var clicks int
	b := e.d.NewButton(clickCounter{&clicks}).(*Button)
	b.SetDefault(true)
	w.SetContent(b)
	b.SetFrame(driver.Rect{X: 10, Y: 10, W: 80, H: 20})
	if !sf.Key(driver.KeyReturn, 0) || clicks != 0 {
		t.Errorf("Return: %d clicks, want the item", clicks)
	}
	if want := []int{23}; !slices.Equal(e.h.picks, want) {
		t.Errorf("picks %v, want %v", e.h.picks, want)
	}
	e.d.SetMenuBar(roleSpec) // no item on Return
	if !sf.Key(driver.KeyReturn, 0) || clicks != 1 {
		t.Errorf("Return without the item: %d clicks, want 1", clicks)
	}
}

// TestAccelFlash: the accelerator flashes its menu's title for menuFlash
// (again on a repeat); not with Reduce Motion; the title stays
// highlighted when the menu opens during the flash.
func TestAccelFlash(t *testing.T) {
	e := newAccelEnv(t)
	w, sf, _ := e.window(allStyles, nil)
	st := w.menuStrip()
	clk := e.s.FakeClock()
	sf.Key('-', driver.ModControl)
	if st.Highlighted() != viewMenu || !st.Flashing() {
		t.Fatalf("highlighted %d, flashing %v; want the View title", st.Highlighted(), st.Flashing())
	}
	clk.Advance(menuFlash / 2)
	sf.KeyRepeat('-', driver.ModControl)
	clk.Advance(menuFlash / 2)
	if st.Highlighted() != viewMenu {
		t.Error("the repeat did not restart the flash")
	}
	clk.Advance(menuFlash / 2)
	if st.Highlighted() != -1 || st.Flashing() {
		t.Errorf("after the flash: highlighted %d, flashing %v", st.Highlighted(), st.Flashing())
	}

	sf.Key('-', driver.ModControl)
	if !st.openMenu(0, menuOpening{}) {
		t.Fatal("app menu did not open")
	}
	if st.Flashing() || st.Highlighted() != 0 {
		t.Errorf("opening: flashing %v, highlighted %d", st.Flashing(), st.Highlighted())
	}
	clk.Advance(menuFlash)
	if st.Highlighted() != 0 {
		t.Errorf("the flash's end cleared the open menu's title: %d", st.Highlighted())
	}
	st.CloseMenu()

	a := w.appearance
	a.ReduceMotion = true
	w.SetAppearance(a)
	sf.Key('-', driver.ModControl)
	if st.Highlighted() != -1 || st.Flashing() {
		t.Errorf("Reduce Motion: highlighted %d, flashing %v", st.Highlighted(), st.Flashing())
	}
	if want := []int{21, 21, 21, 21}; !slices.Equal(e.h.picks, want) {
		t.Errorf("picks %v, want %v", e.h.picks, want)
	}
}

// TestAccelOpenMenuAndUntitled: while a menu is open the keys go to it,
// not to the accelerators; an untitled window has no strip but takes the
// accelerators (D11); a closed window takes none.
func TestAccelOpenMenuAndUntitled(t *testing.T) {
	e := newAccelEnv(t)
	w, sf, _ := e.window(allStyles, nil)
	st := w.menuStrip()
	if !st.openMenu(viewMenu, menuOpening{}) {
		t.Fatal("View did not open")
	}
	sf.Key('-', driver.ModControl)
	sf.Key('Q', driver.ModControl)
	if len(e.h.picks) != 0 || len(e.h.log) != 0 {
		t.Errorf("accelerators ran with a menu open: picks %v, log %v", e.h.picks, e.h.log)
	}
	st.CloseMenu()

	u, usf, _ := e.window(0, nil)
	if u.menuStrip() != nil {
		t.Fatal("an untitled window has a strip")
	}
	if !usf.Key('-', driver.ModControl) {
		t.Error("untitled: Ctrl+- not consumed")
	}
	if want := []int{21}; !slices.Equal(e.h.picks, want) {
		t.Errorf("picks %v, want %v", e.h.picks, want)
	}
	e.h.quit = false
	if !usf.Key('Q', driver.ModControl) || !slices.Equal(e.h.log, []string{"quitRequested"}) {
		t.Errorf("Ctrl+Q: log %v", e.h.log)
	}
}
