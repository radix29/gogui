package aqua

import (
	"slices"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// The menu strip's keyboard access (PLAN.md S5.6i), over the fake shell.

// keysSpec has the default app menu (slot 72+42), File (114+42), Format
// (156+56) with an accelerator and Tools (212+49) with an item, a
// separator and a disabled item: two titles on 'F', two on 'T'.
var keysSpec = &driver.MenuBarSpec{Menus: []driver.MenuSpec{
	driver.DefaultAppMenu(),
	{Title: "File", Items: []driver.MenuItemSpec{{ID: 1, Title: "New"}, {ID: 2, Title: "Open"}}},
	{Title: "Format", Items: []driver.MenuItemSpec{
		{ID: 5, Title: "Bold", Shortcut: driver.Shortcut{Mods: driver.ModPrimary, Key: 'B'}},
	}},
	{Title: "Tools", Items: []driver.MenuItemSpec{
		{ID: 3, Title: "Run"}, {Separator: true}, {ID: 4, Title: "Stop", Disabled: true},
	}},
}}

const (
	keysApp, keysFile, keysFormat, keysTools = 0, 1, 2, 3
)

var (
	keysFileAt   = pt(135, 14)
	keysFormatAt = pt(184, 14)
)

// keysEnv opens a driver window with keysSpec's strip and a focused
// TextField with text.
func keysEnv(t *testing.T) (*roleEnv, *Window, *fake.Surface, *menuStrip, *TextField) {
	t.Helper()
	e := newRoleEnv(t)
	e.d.SetMenuBar(keysSpec)
	tf := e.d.NewTextField(false, nil).(*TextField)
	w, sf, _ := e.window(allStyles, tf)
	tf.view.Focus()
	sf.Type("hi")
	paint(t, e.s, sf)
	if !w.TextInput().Enabled {
		t.Fatal("no text input before the strip")
	}
	return e, w, sf, w.menuStrip(), tf
}

// wantStripFocus checks that the strip has the keys with title i
// highlighted (or, for -1, that it has not and nothing is highlighted),
// and the field's focus state with it.
func wantStripFocus(t *testing.T, what string, w *Window, st *menuStrip, tf *TextField, i int) {
	t.Helper()
	focused := i >= 0
	if st.KeyboardFocused() != focused || st.Highlighted() != i {
		t.Fatalf("%s: strip focused %v, highlighted %d; want %d", what, st.KeyboardFocused(), st.Highlighted(), i)
	}
	want := FocusedActive
	if focused {
		want = FocusedInactive
	}
	if w.Router().Focused() != Element(tf.view) || tf.view.FocusState() != want || w.TextInput().Enabled == focused {
		t.Fatalf("%s: field focus %v (want %v), text input %v", what, tf.view.FocusState(), want, w.TextInput().Enabled)
	}
}

// TestMenuKeysAltAndF10: a lone Alt and F10 focus the strip and leave it,
// the field's focus kept but inactive; Alt with another key, a press, a
// modifier or AltGr is no lone Alt; Escape and a click leave; an untitled
// window leaves Alt and F10 alone.
func TestMenuKeysAltAndF10(t *testing.T) {
	e, w, sf, st, tf := keysEnv(t)
	if !sf.TapModifier(driver.ModOption) {
		t.Error("lone Alt not consumed")
	}
	wantStripFocus(t, "Alt", w, st, tf, keysApp)
	paint(t, e.s, sf)
	sf.TapModifier(driver.ModOption)
	wantStripFocus(t, "Alt again", w, st, tf, -1)
	if !sf.Key(driver.KeyF1+9, 0) {
		t.Error("F10 not consumed")
	}
	wantStripFocus(t, "F10", w, st, tf, keysApp)
	sf.KeyRepeat(driver.KeyF1+9, 0)
	wantStripFocus(t, "F10 repeat", w, st, tf, keysApp)
	sf.Key(driver.KeyF1+9, 0)
	wantStripFocus(t, "F10 again", w, st, tf, -1)
	sf.Key(driver.KeyF1+9, driver.ModShift) // Shift+F10: the context menu's
	wantStripFocus(t, "Shift+F10", w, st, tf, -1)

	// No lone Alt.
	alt := driver.ModOption
	sf.ModifierDown(alt, alt)
	sf.Key('X', alt)
	sf.ModifierUp(alt, 0)
	wantStripFocus(t, "Alt+X", w, st, tf, -1)
	sf.ModifierDown(alt, alt)
	sf.ModifierDown(driver.ModControl, alt|driver.ModControl)
	sf.ModifierUp(driver.ModControl, alt)
	sf.ModifierUp(alt, 0)
	wantStripFocus(t, "Alt+Ctrl", w, st, tf, -1)
	sf.ModifierDown(alt, alt)
	sf.Click(pt(300, 60))
	sf.ModifierUp(alt, 0)
	wantStripFocus(t, "Alt and a click", w, st, tf, -1)
	sf.ModifierDown(driver.ModShift, driver.ModShift)
	sf.ModifierDown(alt, alt|driver.ModShift)
	sf.ModifierUp(alt, driver.ModShift)
	sf.ModifierUp(driver.ModShift, 0)
	wantStripFocus(t, "Shift+Alt", w, st, tf, -1)
	sf.ModifierDown(driver.ModControl, driver.ModControl) // AltGr on Win32
	sf.ModifierDown(alt, alt|driver.ModControl)
	sf.ModifierUp(driver.ModControl, alt)
	sf.ModifierUp(alt, 0)
	wantStripFocus(t, "AltGr", w, st, tf, -1)
	sf.ModifierDown(alt, alt) // held: the repeats change nothing
	sf.Send(shell.KeyEvent{Down: true, Repeat: true, Modifier: alt, Mods: alt})
	sf.ModifierUp(alt, 0)
	wantStripFocus(t, "held Alt", w, st, tf, keysApp)

	// Escape and a click leave; Alt+F4 and the like go on and leave.
	sf.Key(driver.KeyEscape, 0)
	wantStripFocus(t, "Escape", w, st, tf, -1)
	sf.TapModifier(alt)
	sf.Click(pt(300, 60))
	wantStripFocus(t, "click", w, st, tf, -1)
	sf.TapModifier(alt)
	if sf.Key(driver.KeyF1+3, alt) {
		t.Error("Alt+F4 consumed")
	}
	wantStripFocus(t, "Alt+F4", w, st, tf, -1)

	// Leaving with the key, a new bar, a relayout dropping the title.
	sf.TapModifier(alt)
	e.s.Deactivate()
	if st.KeyboardFocused() || st.Highlighted() != -1 || w.Router().Active() {
		t.Errorf("key loss: focused %v, highlighted %d", st.KeyboardFocused(), st.Highlighted())
	}
	sf.Activate()
	sf.TapModifier(alt)
	e.d.SetMenuBar(keysSpec)
	if st.KeyboardFocused() || w.menuStrip().KeyboardFocused() || !w.Router().Active() {
		t.Error("a new bar kept the keyboard focus")
	}
	st = w.menuStrip()
	sf.TapModifier(alt)
	st.moveKeyboard(-1) // Tools
	w.SetContentSize(driver.Size{W: 250, H: 100})
	if st.Shown() > keysTools || st.KeyboardFocused() || !w.Router().Active() {
		t.Errorf("relayout: shown %d, focused %v", st.Shown(), st.KeyboardFocused())
	}

	// An untitled window has no strip: Alt and F10 go on.
	u, usf, _ := e.window(driver.StyleClosable, nil)
	if usf.TapModifier(alt) || usf.Key(driver.KeyF1+9, 0) || u.menuStrip() != nil {
		t.Error("untitled window took Alt or F10")
	}
}

// TestMenuKeysStrip: in the focused strip Left and Right move, wrapping;
// characters highlight by prefix; other keys and their text are eaten; a
// shortcut leaves the strip and runs.
func TestMenuKeysStrip(t *testing.T) {
	e, w, sf, st, tf := keysEnv(t)
	sf.Key(driver.KeyF1+9, 0)
	for _, c := range []struct {
		key  driver.Key
		mods driver.Modifiers
		want int
	}{
		{driver.KeyRight, 0, keysFile}, {driver.KeyRight, 0, keysFormat}, {driver.KeyLeft, 0, keysFile},
		{driver.KeyLeft, 0, keysApp}, {driver.KeyLeft, 0, keysTools}, {driver.KeyRight, driver.ModShift, keysApp},
		{'F', 0, keysFile}, {'F', 0, keysFormat}, {'F', 0, keysFile}, {'O', 0, keysFormat}, {'R', 0, keysFormat},
	} {
		if !sf.Key(c.key, c.mods) {
			t.Errorf("%q not consumed", rune(c.key))
		}
		wantStripFocus(t, "key "+string(rune(c.key)), w, st, tf, c.want)
	}
	e.s.FakeClock().Advance(time.Second)
	for _, c := range []struct {
		key  driver.Key
		want int
	}{{'T', keysTools}, {'T', keysApp}, {'T', keysTools}, {'Q', keysTools}, {'1', keysTools}} {
		sf.Key(c.key, 0)
		wantStripFocus(t, "type-select "+string(rune(c.key)), w, st, tf, c.want)
	}
	sf.Type("ab\t")
	sf.Commit("c")
	sf.Key(driver.KeyUp, 0)
	if tf.Text() != "hi" || st.OpenMenu() != -1 {
		t.Errorf("eaten keys: text %q, open %d", tf.Text(), st.OpenMenu())
	}
	wantStripFocus(t, "eaten keys", w, st, tf, keysTools)
	sf.ModifierDown(driver.ModControl, driver.ModControl)
	wantStripFocus(t, "Ctrl alone", w, st, tf, keysTools)
	if !sf.Key('B', driver.ModControl) {
		t.Error("Ctrl+B not run")
	}
	sf.ModifierUp(driver.ModControl, 0)
	if !slices.Equal(e.h.picks, []int{5}) || st.KeyboardFocused() || !st.Flashing() {
		t.Errorf("picks %v, strip focused %v, flashing %v", e.h.picks, st.KeyboardFocused(), st.Flashing())
	}
	e.s.FakeClock().Advance(menuFlash)
	wantStripFocus(t, "Ctrl+B", w, st, tf, -1)
}

// TestMenuKeysMenus: Down, Return and Space open the highlighted menu with
// its first enabled item; Left and Right switch menus, wrapping; Escape
// goes back to the strip, a pick, Alt and F10 leave it.
func TestMenuKeysMenus(t *testing.T) {
	e, w, sf, st, tf := keysEnv(t)
	sf.Key(driver.KeyF1+9, 0)
	sf.Key(driver.KeyRight, 0)
	sf.Key(driver.KeyDown, 0)
	stripPopup(t, st, e.s, keysFile)
	if st.menu.Highlighted() != 0 || !st.KeyboardFocused() || tf.view.FocusState() != FocusedInactive {
		t.Fatalf("opened: item %d, strip focused %v", st.menu.Highlighted(), st.KeyboardFocused())
	}
	for _, c := range []struct {
		key        driver.Key
		menu, item int
	}{
		{driver.KeyRight, keysFormat, 0}, {driver.KeyRight, keysTools, 0}, {driver.KeyRight, keysApp, -2},
		{driver.KeyLeft, keysTools, 0}, {driver.KeyDown, keysTools, 0}, {driver.KeyLeft, keysFormat, 0},
	} {
		if !sf.Key(c.key, 0) {
			t.Errorf("%q not consumed in the menu", rune(c.key))
		}
		stripPopup(t, st, e.s, c.menu)
		if c.item >= -1 && st.menu.Highlighted() != c.item {
			t.Errorf("menu %d: item %d highlighted, want %d", c.menu, st.menu.Highlighted(), c.item)
		}
		if c.item == -2 && !st.menu.Selectable(st.menu.Highlighted()) {
			t.Errorf("app menu: item %d highlighted", st.menu.Highlighted())
		}
	}
	if e.s.GrabPopup() == nil || len(e.s.Popups()) != 1 {
		t.Fatal("not one grab popup throughout")
	}

	// Escape: back to the strip on the title; Return opens again, Space
	// too after Escape; Return picks.
	sf.Key(driver.KeyEscape, 0)
	if st.OpenMenu() != -1 || e.s.GrabPopup() != nil {
		t.Fatal("Escape did not close the menu")
	}
	wantStripFocus(t, "Escape", w, st, tf, keysFormat)
	sf.Key(driver.KeySpace, 0)
	stripPopup(t, st, e.s, keysFormat)
	sf.Key(driver.KeyEscape, 0)
	sf.Key(driver.KeyReturn, 0)
	stripPopup(t, st, e.s, keysFormat)
	sf.Key(driver.KeyReturn, 0)
	finishPick(e.s)
	if !slices.Equal(e.h.picks, []int{5}) {
		t.Errorf("picks %v", e.h.picks)
	}
	wantStripFocus(t, "pick", w, st, tf, -1)

	// Alt and F10 in an open menu close it and leave.
	for _, leave := range []func(){
		func() { sf.TapModifier(driver.ModOption) },
		func() { sf.Key(driver.KeyF1+9, 0) },
	} {
		sf.Key(driver.KeyF1+9, 0)
		sf.Key(driver.KeyDown, 0)
		stripPopup(t, st, e.s, keysApp)
		leave()
		if st.OpenMenu() != -1 || e.s.GrabPopup() != nil {
			t.Fatal("menu still open")
		}
		wantStripFocus(t, "Alt or F10 in the menu", w, st, tf, -1)
	}

	// A press in a keyboard menu leaves the strip, the menu stays open as
	// a mouse menu: Escape then closes it without the strip.
	sf.Key(driver.KeyF1+9, 0)
	sf.Key(driver.KeyDown, 0)
	pp := stripPopup(t, st, e.s, keysApp)
	others := pp.ToParent(centre(st.menu.ItemRect(3))) // Hide Others: disabled
	sf.Press(others, shell.ButtonPrimary)
	sf.Release(others, shell.ButtonPrimary)

	if st.KeyboardFocused() || st.OpenMenu() != keysApp || !w.Router().Active() {
		t.Fatalf("press: strip focused %v, open %d", st.KeyboardFocused(), st.OpenMenu())
	}
	sf.Key(driver.KeyEscape, 0)
	wantClosed(t, st, e.s)
	wantStripFocus(t, "Escape after a press", w, st, tf, -1)
}

// TestMenuKeysMouseMenu: Left and Right switch a menu opened by mouse too;
// Escape closes it with no strip focus; a popup button's menu keeps Left,
// Right, Alt and F10 to itself.
func TestMenuKeysMouseMenu(t *testing.T) {
	e, w, sf, st, tf := keysEnv(t)
	sf.Click(keysFileAt)
	stripPopup(t, st, e.s, keysFile)
	sf.Key(driver.KeyRight, 0)
	stripPopup(t, st, e.s, keysFormat)
	if st.menu.Highlighted() != 0 || st.KeyboardFocused() {
		t.Errorf("switched: item %d, strip focused %v", st.menu.Highlighted(), st.KeyboardFocused())
	}
	sf.Key(driver.KeyEscape, 0)
	wantClosed(t, st, e.s)
	wantStripFocus(t, "Escape", w, st, tf, -1)
	sf.Click(keysFormatAt)
	sf.TapModifier(driver.ModOption)
	wantClosed(t, st, e.s)
	wantStripFocus(t, "Alt in a mouse menu", w, st, tf, -1)

	m := newMenu(w.text)
	m.SetItems([]menuItem{{Title: "A"}})
	if m.key(KeyEvent{shell.KeyEvent{Down: true, Key: driver.KeyLeft}}) {
		t.Error("a menu without side keys took Left")
	}
	var sides []int
	m.SetSideKeys(func(d int) { sides = append(sides, d) })
	m.key(KeyEvent{shell.KeyEvent{Down: true, Key: driver.KeyLeft}})
	m.key(KeyEvent{shell.KeyEvent{Down: true, Key: driver.KeyRight}})
	if !slices.Equal(sides, []int{-1, 1}) {
		t.Errorf("side keys %v", sides)
	}
}
