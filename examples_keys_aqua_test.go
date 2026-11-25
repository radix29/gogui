package gogui

import (
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua"
	"github.com/radix29/gogui/internal/driver"
	fakeshell "github.com/radix29/gogui/internal/shell/fake"
)

// The form and controls examples driven by the keyboard alone on the aqua
// driver over the fake shell (PLAN.md S5.7h): every field and control
// reached by Tab, text typed, the pop-up button by Space and the arrows,
// the radio group by the arrows, the checkboxes by Space, the buttons and
// the alerts' sheets by Return, Escape and Space. No pointer event is
// sent.

// keys is a keyboard-only driver of one aqua window's surface.
type keys struct {
	t   *testing.T
	app *App
	sh  *fakeshell.Shell
	win *Window
	aw  *aqua.Window
	sf  *fakeshell.Surface
}

func newKeys(t *testing.T, app *App, sh *fakeshell.Shell, win *Window) *keys {
	aw := win.p.(*aqua.Window)
	return &keys{t: t, app: app, sh: sh, win: win, aw: aw, sf: aw.Surface().(*fakeshell.Surface)}
}

// press sends a key (down and up), which must be consumed, and lets the
// animations it starts end.
func (k *keys) press(key driver.Key, mods driver.Modifiers) {
	k.t.Helper()
	if !k.sf.Key(key, mods) {
		k.t.Fatalf("key %q mods %v not consumed", key, mods)
	}
	k.settle()
}

// settle runs the UI queue and lets every running animation end.
func (k *keys) settle() {
	k.sh.FakeClock().Advance(time.Second)
	drain(k.app)
}

// tab presses Tab (Shift-Tab when backward) n times.
func (k *keys) tab(n int, backward bool) {
	k.t.Helper()
	var mods driver.Modifiers
	if backward {
		mods = driver.ModShift
	}
	for range n {
		k.press(driver.KeyTab, mods)
	}
}

// wantFocus checks that the content's focus is on the peer of w (a widget
// or a radio).
func (k *keys) wantFocus(what string, p driver.Widget) {
	k.t.Helper()
	if got := k.aw.Router().Focused(); got != aqua.ElementOf(p) {
		k.t.Fatalf("focus: want %s, got %T", what, got)
	}
}

// sheet returns the one alert shown, which must be a sheet of the window
// that has slid in.
func (k *keys) sheet(message string) *aqua.Alert {
	k.t.Helper()
	d := k.app.drv.(*aqua.Driver)
	alerts := d.Alerts()
	if len(alerts) != 1 || alerts[0].Spec.Message != message {
		k.t.Fatalf("want the alert %q, got %+v", message, alerts)
	}
	if alerts[0].Window() != k.aw || k.aw.SheetRoot() == nil || alerts[0].Surface() != nil {
		k.t.Fatalf("alert %q is not shown as a sheet of the window", message)
	}
	return alerts[0]
}

// wantNoAlert checks that no alert is left and the window is not modal.
func (k *keys) wantNoAlert() {
	k.t.Helper()
	if a := k.app.drv.(*aqua.Driver).Alerts(); len(a) != 0 || k.aw.Modal() {
		k.t.Fatalf("alerts left: %+v (modal %v)", a, k.aw.Modal())
	}
}

// TestFormByKeyboardOnAqua fills in the form example from start to end
// with keys only: the fields by typing, Tab through every field and
// control, the role by Space, Down and Return, the plan by the arrows,
// the checkboxes by Space, Return to submit; the validation sheet is
// dismissed by Return, the e-mail fixed, and the success sheet's Return
// closes the window.
func TestFormByKeyboardOnAqua(t *testing.T) {
	runAquaNamed(t, "Account", func(app *App, sh *fakeshell.Shell) {
		f := newFormExample(app)
		showExample(app, f.win)
		k := newKeys(t, app, sh, f.win)
		k.wantFocus("name", f.name.p)
		k.sf.Type("Jane")

		k.tab(1, false)
		k.wantFocus("e-mail", f.email.p)
		k.sf.Type("jane")
		k.tab(1, false)
		k.wantFocus("password", f.password.p)
		k.sf.Type("secret")

		// The role: Space opens the menu over Viewer, Down and Return pick
		// Editor.
		k.tab(1, false)
		k.wantFocus("role", f.role.p)
		k.press(driver.KeySpace, 0)
		if sh.GrabPopup() == nil {
			t.Fatal("Space did not open the role menu")
		}
		k.press(driver.KeyDown, 0)
		k.press(driver.KeyReturn, 0)
		if sh.GrabPopup() != nil || f.role.Selected() != 1 {
			t.Fatalf("role menu open %v, selected %d", sh.GrabPopup() != nil, f.role.Selected())
		}
		k.wantFocus("role after the menu", f.role.p)

		// The plan: Tab reaches the selected radio only; Down selects Pro,
		// and Team, and stops there; Up goes back to Pro.
		k.tab(1, false)
		k.wantFocus("plan (Free)", f.plan.radios[0].p)
		var plans []int
		f.plan.OnChange(func(i int) { plans = append(plans, i) })
		k.press(driver.KeyDown, 0)
		k.press(driver.KeyDown, 0)
		k.press(driver.KeyDown, 0)
		k.press(driver.KeyUp, 0)
		k.wantFocus("plan (Pro)", f.plan.radios[1].p)
		if f.plan.Selected() != 1 || len(plans) != 3 || plans[0] != 1 || plans[1] != 2 || plans[2] != 1 {
			t.Fatalf("plan %d, changes %v", f.plan.Selected(), plans)
		}

		// The checkboxes by Space; Return while Create Account is disabled
		// does nothing.
		k.tab(1, false)
		k.wantFocus("newsletter", f.newsletter.p)
		k.press(driver.KeySpace, 0)
		if !f.newsletter.Checked() {
			t.Fatal("Space did not check the newsletter")
		}
		k.tab(1, false)
		k.wantFocus("terms", f.terms.p)
		if k.sf.Key(driver.KeyReturn, 0) {
			t.Fatal("Return consumed while Create Account is disabled")
		}
		k.press(driver.KeySpace, 0)
		if !f.terms.Checked() || stubBase(f.create).Disabled {
			t.Fatal("Space did not check the terms")
		}

		// Cancel and Create Account follow, then the name again.
		k.tab(1, false)
		k.wantFocus("cancel", f.cancel.p)
		k.tab(1, false)
		k.wantFocus("create", f.create.p)
		k.tab(1, false)
		k.wantFocus("name again", f.name.p)
		k.tab(1, true)
		k.wantFocus("create again", f.create.p)
		k.tab(2, true)
		k.wantFocus("terms again", f.terms.p)

		// Return submits: the validation sheet, dismissed by Return.
		k.press(driver.KeyReturn, 0)
		k.sheet("Please check your details")
		if !k.aw.Modal() {
			t.Fatal("the window is not modal under the sheet")
		}
		// Tab cycles in the sheet (its only button), not in the content.
		k.tab(1, false)
		k.wantFocus("terms under the sheet", f.terms.p)
		k.press(driver.KeyReturn, 0)
		k.wantNoAlert()
		k.wantFocus("terms after the sheet", f.terms.p)

		// Back to the e-mail: Tab selects its text, typing replaces it.
		k.tab(5, true)
		k.wantFocus("e-mail again", f.email.p)
		k.sf.Type("jane@example.com")
		if f.email.Text() != "jane@example.com" {
			t.Fatalf("e-mail %q", f.email.Text())
		}

		// Return in the field submits; the success sheet's Return closes
		// the window.
		k.press(driver.KeyReturn, 0)
		k.sheet("Account created")
		if a := app.drv.(*aqua.Driver).Alerts()[0]; a.Spec.Detail != "Welcome, Jane (Editor)." {
			t.Fatalf("detail %q", a.Spec.Detail)
		}
		k.press(driver.KeyReturn, 0)
		if !k.aw.Closed() {
			t.Fatal("the success sheet's Return did not close the window")
		}
		k.wantNoAlert()
	})
}

// TestControlsByKeyboardOnAqua tabs through every enabled control of the
// controls example in order, then opens "Show Alert…" by Space three
// times and answers its sheet by keys: Escape (Cancel), Return (Delete),
// and Tab to Delete then Space; the status reports each result.
func TestControlsByKeyboardOnAqua(t *testing.T) {
	runAquaNamed(t, "Controls", func(app *App, sh *fakeshell.Shell) {
		c := newControlsExample(app)
		showExample(app, c.win)
		k := newKeys(t, app, sh, c.win)
		k.wantFocus("the push button first", c.push.p)
		order := []struct {
			what string
			p    driver.Widget
		}{
			{"default", c.def.p}, {"checkbox", c.check.p}, {"mixed", c.mixed.p},
			{"radios (Medium)", c.radios.radios[1].p}, {"field", c.field.p}, {"secure", c.secure.p},
			{"slider", c.slider.p}, {"popup", c.popup.p}, {"alert", c.alert.p}, {"push", c.push.p},
		}
		for _, o := range order {
			k.tab(1, false)
			k.wantFocus(o.what, o.p)
		}
		k.tab(1, true)
		k.wantFocus("alert again", c.alert.p)

		answers := []struct {
			name string
			keys func()
			want string
		}{
			{"Escape", func() { k.press(driver.KeyEscape, 0) }, "Alert result: 1"},
			{"Return", func() { k.press(driver.KeyReturn, 0) }, "Alert result: 0"},
			{"Tab and Space", func() {
				a := k.sheet("Delete this item?")
				r := a.Root().(interface{ Router() *aqua.Router }).Router()
				if r.Focused() == nil {
					t.Fatal("no focus in the sheet")
				}
				k.tab(1, false)
				k.press(driver.KeySpace, 0)
			}, "Alert result: 0"},
		}
		for _, ans := range answers {
			k.press(driver.KeySpace, 0)
			k.sheet("Delete this item?")
			ans.keys()
			k.wantNoAlert()
			if c.status.Text() != ans.want {
				t.Errorf("%s: status %q, want %q", ans.name, c.status.Text(), ans.want)
			}
			k.wantFocus("alert after the sheet", c.alert.p)
		}
	})
}
