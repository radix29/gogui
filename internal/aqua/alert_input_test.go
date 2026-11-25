package aqua

import (
	"fmt"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// alertEnv is an alert in a bare alertRoot (S5.7b), framed at its fitting
// size, with its answers logged.
type alertEnv struct {
	s    *fake.Shell
	a    *alert
	r    *alertRoot
	got  []int
	spec driver.AlertSpec
}

func newAlertEnv(t *testing.T, spec driver.AlertSpec) *alertEnv {
	t.Helper()
	s := startShell(t)
	e := &alertEnv{s: s, spec: spec}
	e.a = newAlert(newDriver(s), spec)
	e.a.SetPressFunc(func(i int) { e.got = append(e.got, i) })
	e.r = newAlertRoot(e.a)
	e.r.SetFrame(driver.RectOf(driver.Point{X: 40, Y: 30}, e.r.FittingSize(alertInfSize)))
	return e
}

// answers returns the logged answers.
func (e *alertEnv) answers() string { return fmt.Sprint(e.got) }

// center returns the centre of button i in root points.
func (e *alertEnv) center(i int) (float64, float64) {
	v := e.a.Button(i).view
	p := ToRoot(v, driver.Point{X: v.Bounds().W / 2, Y: v.Bounds().H / 2})
	return p.X, p.Y
}

// focused returns the index of the focused button, or -1.
func (e *alertEnv) focused() int {
	for i := range e.a.buttons {
		if e.r.Router().Focused() == Element(e.a.Button(i).view) {
			return i
		}
	}
	return -1
}

// TestAlertClick: a click on a button answers with its index; a press
// dragged off answers nothing; the first answer is the only one.
func TestAlertClick(t *testing.T) {
	e := newAlertEnv(t, threeButtonAlert)
	x, y := e.center(1)
	e.r.Event(down(x, y))
	if e.a.Button(1).view.State().Interaction != InteractionPressed {
		t.Error("button 1 not drawn pressed during the press")
	}
	e.r.Event(move(x, y+200))
	e.r.Event(up(x, y+200))
	if _, ok := e.a.Result(); ok || len(e.got) != 0 {
		t.Fatalf("dragged off: answers %v", e.got)
	}
	if !e.r.Event(down(x, y)) || !e.r.Event(up(x, y)) {
		t.Error("click unhandled")
	}
	x, y = e.center(2)
	e.r.Event(down(x, y))
	e.r.Event(up(x, y))
	e.a.Button(0).UserClick()
	if i, ok := e.a.Result(); !ok || i != 1 || e.answers() != "[1]" {
		t.Errorf("result %d %v, answers %v", i, ok, e.got)
	}
}

// TestAlertKeys: Return presses the default button, Escape and Primary+.
// the Cancel button; with modifiers, as an auto-repeat, without a Cancel
// button or on a key up nothing answers; once answered, the keys are
// still consumed.
func TestAlertKeys(t *testing.T) {
	primary := driver.ModPrimary.Resolve(false)
	for _, c := range []struct {
		name    string
		spec    driver.AlertSpec
		ev      shell.KeyEvent
		handled bool
		want    string
	}{
		{"return", twoButtonAlert, key(driver.KeyReturn, 0), true, "[0]"},
		{"escape", twoButtonAlert, key(driver.KeyEscape, 0), true, "[1]"},
		{"primary-period", threeButtonAlert, key('.', primary), true, "[2]"},
		{"period", threeButtonAlert, key('.', 0), false, "[]"},
		{"shift-return", twoButtonAlert, key(driver.KeyReturn, driver.ModShift), false, "[]"},
		{"primary-escape", twoButtonAlert, key(driver.KeyEscape, primary), false, "[]"},
		{"escape without cancel", oneButtonAlert, key(driver.KeyEscape, 0), false, "[]"},
		{"repeat", twoButtonAlert, shell.KeyEvent{Down: true, Repeat: true, Key: driver.KeyReturn}, true, "[]"},
		{"key up", twoButtonAlert, shell.KeyEvent{Key: driver.KeyReturn}, false, "[]"},
		{"letter", twoButtonAlert, key('d', 0), false, "[]"},
	} {
		e := newAlertEnv(t, c.spec)
		if got := e.r.Event(c.ev); got != c.handled || e.answers() != c.want {
			t.Errorf("%s: handled %v, answers %v; want %v, %s", c.name, got, e.got, c.handled, c.want)
		}
	}

	// A Cancel index out of range is none.
	spec := twoButtonAlert
	spec.Cancel = 5
	if e := newAlertEnv(t, spec); e.r.Event(key(driver.KeyEscape, 0)) || len(e.got) != 0 {
		t.Errorf("cancel out of range: answers %v", e.got)
	}

	e := newAlertEnv(t, twoButtonAlert)
	e.r.Event(key(driver.KeyEscape, 0))
	for _, ev := range []shell.KeyEvent{key(driver.KeyReturn, 0), key(driver.KeySpace, 0), key(driver.KeyEscape, 0)} {
		if !e.r.Event(ev) {
			t.Errorf("%v after the answer unhandled", ev.Key)
		}
	}
	if e.answers() != "[1]" {
		t.Errorf("answers %v", e.got)
	}
}

// TestAlertFocus: with full keyboard access the focus starts on the
// default button's neighbour (the default itself when alone), Tab and
// Shift-Tab cycle through the buttons in spec order, and Space presses the
// focused button, not the default; without full keyboard access there is
// no focus, Tab reaches nothing and Space presses nothing, and turning it
// on again focuses the neighbour.
func TestAlertFocus(t *testing.T) {
	for _, c := range []struct {
		spec  driver.AlertSpec
		start int
	}{{oneButtonAlert, 0}, {twoButtonAlert, 1}, {threeButtonAlert, 1}} {
		if e := newAlertEnv(t, c.spec); e.focused() != c.start {
			t.Errorf("%q: focus starts on %d, want %d", c.spec.Message, e.focused(), c.start)
		}
	}

	e := newAlertEnv(t, threeButtonAlert)
	var order []int
	for range 4 {
		if !e.r.Event(key(driver.KeyTab, 0)) {
			t.Fatal("tab unhandled")
		}
		order = append(order, e.focused())
	}
	for range 2 {
		e.r.Event(key(driver.KeyTab, driver.ModShift))
		order = append(order, e.focused())
	}
	if fmt.Sprint(order) != "[2 0 1 2 1 0]" {
		t.Errorf("tab order %v", order)
	}
	if !e.a.Button(0).view.ring.Visible() {
		t.Error("no focus ring on the focused button")
	}
	e.r.Event(key(driver.KeyTab, 0)) // on 1
	if !e.r.Event(key(driver.KeySpace, 0)) || e.answers() != "[1]" {
		t.Errorf("space: answers %v", e.got)
	}

	e = newAlertEnv(t, twoButtonAlert)
	e.r.SetFullKeyboardAccess(false)
	if e.focused() != -1 || e.r.Event(key(driver.KeyTab, 0)) || e.r.Event(key(driver.KeySpace, 0)) || len(e.got) != 0 {
		t.Errorf("without full keyboard access: focus %d, answers %v", e.focused(), e.got)
	}
	e.r.SetFullKeyboardAccess(true)
	if e.focused() != 1 {
		t.Errorf("full keyboard access back: focus %d", e.focused())
	}
	if !e.r.Event(key(driver.KeyReturn, 0)) || e.answers() != "[0]" {
		t.Errorf("return with the focus on 1: answers %v", e.got)
	}
}

// TestAlertFlash: a press by key draws its button pressed for keyFlash
// and answers at once; a click does not flash.
func TestAlertFlash(t *testing.T) {
	e := newAlertEnv(t, twoButtonAlert)
	b := e.a.Button(1).view
	e.r.Event(key(driver.KeyEscape, 0))
	if e.answers() != "[1]" || !b.Flashing() || b.State().Interaction != InteractionPressed || e.a.Button(0).view.Flashing() {
		t.Fatalf("after escape: answers %v, flashing %v, state %v", e.got, b.Flashing(), b.State())
	}
	clk := e.s.FakeClock()
	clk.Advance(keyFlash / 2)
	if b.State().Interaction != InteractionPressed {
		t.Error("flash ended early")
	}
	clk.Advance(keyFlash / 2)
	if b.Flashing() || b.State().Interaction == InteractionPressed {
		t.Errorf("flash still on: state %v", b.State())
	}

	e = newAlertEnv(t, twoButtonAlert)
	x, y := e.center(0)
	e.r.Event(down(x, y))
	e.r.Event(up(x, y))
	if e.answers() != "[0]" || e.a.Button(0).view.Flashing() {
		t.Errorf("click: answers %v, flashing %v", e.got, e.a.Button(0).view.Flashing())
	}
}

// TestAlertInactive: an inactive router keeps the focus but draws the
// default button plain and no focus ring, Space presses nothing (Return
// still does: keys only come to a key window), and the elements get a
// WindowStateEvent on the change.
func TestAlertInactive(t *testing.T) {
	e := newAlertEnv(t, twoButtonAlert)
	def := e.a.Button(0).view
	if !def.State().Key {
		t.Fatal("default button not key in an active router")
	}
	e.r.SetActive(false)
	if def.State().Key || e.focused() != 1 || e.a.Button(1).view.FocusState() != FocusedInactive {
		t.Errorf("inactive: key %v, focus %d %v", def.State().Key, e.focused(), e.a.Button(1).view.FocusState())
	}
	if e.r.Event(key(driver.KeySpace, 0)) || len(e.got) != 0 {
		t.Errorf("space while inactive: answers %v", e.got)
	}
	e.r.SetActive(true)
	if e.a.Button(1).view.FocusState() != FocusedActive {
		t.Error("focus not active again")
	}
	e.r.SetActive(false)
	if !e.r.Event(key(driver.KeyReturn, 0)) || e.answers() != "[0]" {
		t.Errorf("return while inactive: answers %v", e.got)
	}
}

// TestAlertRootTree: the root frames the alert to its bounds and is the
// alert's only parent and the router's root.
func TestAlertRootTree(t *testing.T) {
	e := newAlertEnv(t, twoButtonAlert)
	if f := e.a.Frame(); f != driver.RectOf(driver.Point{}, e.r.Bounds().Size()) || e.a.Parent() != Element(e.r) {
		t.Errorf("alert frame %v in root %v", f, e.r.Frame())
	}
	if e.r.Router().Root() != Element(e.r) {
		t.Error("router root")
	}
}
