package aqua

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua/menubar"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// The alerts in modal surfaces (PLAN.md S5.7g), over a running driver on
// the fake shell (sheetRun's window: 400×300, a focused text field).

// showApp runs an app-modal alert that logs its answers.
func (e *sheetEnv) showApp(spec driver.AlertSpec) {
	e.d.RunAlert(nil, spec, func(i int) { e.got = append(e.got, fmt.Sprintf("%s=%d", spec.Message, i)) })
}

// modal returns the shell's active modal surface, failing without one.
func (e *sheetEnv) modal() *fake.Surface {
	e.t.Helper()
	m := e.sh.Modal()
	if m == nil {
		e.t.Fatal("no modal surface")
	}
	return m
}

// TestAppAlertSurface: an app-modal alert opens a modal surface of the
// panel's size, without owner, titled with the app name, key and blocking
// the window; its panel is square and borderless; Return answers it, the
// surface closes before done and the window is key again.
func TestAppAlertSurface(t *testing.T) {
	sheetRun(t, driver.DefaultAppearance(), func(e *sheetEnv) {
		e.showApp(twoButtonAlert)
		e.settle()
		m := e.modal()
		as := e.d.Alerts()
		if len(as) != 1 || as[0].Surface() != shell.Surface(m) || as[0].Window() != nil {
			t.Fatalf("alerts %v", as)
		}
		a := as[0].Root().(*alertRoot).alert
		want := a.FittingSize(driver.Size{W: math.Inf(1), H: math.Inf(1)})
		if !m.Modal() || m.Owner() != nil || m.Size() != want || m.Title() != "Test" || m.Style()&driver.StyleResizable != 0 {
			t.Errorf("surface: modal %v, owner %v, size %v (want %v), title %q", m.Modal(), m.Owner(), m.Size(), want, m.Title())
		}
		if a.ShadowMargin() != (driver.Insets{}) || a.Panel() != driver.RectOf(driver.Point{}, want) {
			t.Errorf("panel %v, margin %v", a.Panel(), a.ShadowMargin())
		}
		if !m.IsKey() || !e.sf.Blocked() || e.w.IsKey() || !as[0].Root().(*alertRoot).router.Active() {
			t.Errorf("key: surface %v, window blocked %v, window key %v", m.IsKey(), e.sf.Blocked(), e.w.IsKey())
		}
		if err := m.PaintErr(); err != nil || m.Paints() == 0 {
			t.Fatalf("paint: %v (%d paints)", err, m.Paints())
		}
		dump := m.Dump()
		if !strings.HasPrefix(dump, fmt.Sprintf("fillRRect (0,0,%g,%g) solid", want.W, want.H)) || strings.Contains(dump, "strokeRRect (0.5,0.5") {
			t.Errorf("the panel draws its own border or shadow:\n%s", dump)
		}
		// The window takes no input meanwhile.
		if e.sf.Key(driver.KeyTab, 0) || len(e.fl.events) != 0 || e.w.Router().Focused() != e.field.view {
			t.Error("the blocked window took a key")
		}
		var closedFirst bool
		e.d.RunAlert(nil, oneButtonAlert, func(int) {})
		e.d.Alerts()[0].done = func(i int) {
			closedFirst = m.Closed()
			e.got = append(e.got, fmt.Sprint("done=", i))
		}
		m.Key(driver.KeyReturn, 0)
		if got := e.answers(); got != "done=0" || !closedFirst {
			t.Errorf("answers %q, closed before done %v", got, closedFirst)
		}
	})
}

// TestAppAlertQueue: app-modal alerts open one at a time in order, the
// next after the done of the one before; the window gets the key back
// after the last.
func TestAppAlertQueue(t *testing.T) {
	sheetRun(t, driver.DefaultAppearance(), func(e *sheetEnv) {
		var order []string
		e.d.RunAlert(nil, twoButtonAlert, func(i int) {
			order = append(order, fmt.Sprintf("first=%d next-open=%v", i, e.sh.Modal() != nil))
		})
		e.showApp(oneButtonAlert)
		first := e.modal()
		if len(e.sh.Surfaces()) != 2 || len(e.d.Alerts()) != 2 {
			t.Fatalf("%d surfaces, %d alerts", len(e.sh.Surfaces()), len(e.d.Alerts()))
		}
		first.Key(driver.KeyEscape, 0)
		if strings.Join(order, " ") != "first=1 next-open=false" {
			t.Errorf("order %v", order)
		}
		second := e.modal()
		if second == first || !second.IsKey() {
			t.Fatal("the second alert did not open")
		}
		e.settle()
		second.Key(driver.KeySpace, 0) // its only button, focused
		if e.answers() != "Saved=0" || e.sh.Modal() != nil || !e.w.IsKey() || !e.w.Router().Active() {
			t.Errorf("after the queue: modal %v, window key %v", e.sh.Modal(), e.w.IsKey())
		}
		// Dismiss answers through the element too.
		e.showApp(threeButtonAlert)
		e.d.Alerts()[0].Dismiss(1)
		if e.answers() != threeButtonAlert.Message+"=1" || e.sh.Modal() != nil {
			t.Error("Dismiss did not close the surface")
		}
	})
}

// TestAppAlertMenus: while an alert surface is open every menu item but
// Quit is disabled, in the windows and in the surface's accelerators;
// Primary+Q in the surface quits, answering the shown and the queued
// alerts with their Cancel index.
func TestAppAlertMenus(t *testing.T) {
	sheetRun(t, driver.DefaultAppearance(), func(e *sheetEnv) {
		e.field.SetText("abc")
		e.showApp(twoButtonAlert)
		e.showApp(threeButtonAlert)
		if got := enabled(t, e.w, appMenu); got != "About Test- Hide Test- Hide Others- Show All- Quit Test+" {
			t.Errorf("app menu: %s", got)
		}
		if got := enabled(t, e.w, windowMenu); strings.Contains(got, "+") {
			t.Errorf("window menu: %s", got)
		}
		m := e.modal()
		if m.Key('a', driver.ModPrimary.Resolve(false)) || e.field.view.canEdit(driver.ItemCopy) {
			t.Error("Select All ran under the alert")
		}
		if !m.Key('q', driver.ModPrimary.Resolve(false)) || !e.d.Quitting() {
			t.Fatal("Primary+Q did not quit")
		}
		if got := e.answers(); got != "Delete the item?=1 "+threeButtonAlert.Message+"=2" {
			t.Errorf("answers at Quit %q", got)
		}
	})
}

// TestAppAlertClose: an OS close of the surface presses Cancel when there
// is one and is refused otherwise.
func TestAppAlertClose(t *testing.T) {
	sheetRun(t, driver.DefaultAppearance(), func(e *sheetEnv) {
		e.showApp(oneButtonAlert)
		m := e.modal()
		if m.UserClose() || m.Closed() || e.answers() != "" {
			t.Error("a close without Cancel was not refused")
		}
		m.Key(driver.KeyReturn, 0)
		e.showApp(twoButtonAlert)
		m = e.modal()
		m.UserClose()
		if !m.Closed() || e.answers() != "Saved=0 Delete the item?=1" {
			t.Error("the close did not press Cancel")
		}
	})
}

// TestAppAlertAppearance: an appearance change repaints the surface.
func TestAppAlertAppearance(t *testing.T) {
	sheetRun(t, driver.DefaultAppearance(), func(e *sheetEnv) {
		e.showApp(oneButtonAlert)
		e.settle()
		m := e.modal()
		light := m.Dump()
		e.d.SetAppearance(driver.AppearanceDark)
		e.settle()
		if m.Dump() == light {
			t.Error("the surface did not follow the dark appearance")
		}
	})
}

// TestAppAlertBeforeReady: an alert run before the shell is ready opens
// after AppHandler.Ready.
func TestAppAlertBeforeReady(t *testing.T) {
	sh := fake.New()
	d := New(sh)
	got := -1
	d.RunAlert(nil, oneButtonAlert, func(i int) { got = i })
	h := &appLog{}
	h.onReady = func() {
		if sh.Modal() != nil {
			t.Error("the alert opened before Ready")
		}
		d.Queue().Do(func() {
			m := sh.Modal()
			if m == nil {
				t.Error("the alert did not open after Ready")
			} else {
				m.Key(driver.KeyReturn, 0)
			}
			d.Quit()
		})
	}
	if err := runDriver(t, d, h); err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Errorf("answer %d", got)
	}
}

// TestSheetSmallWindowSurface: the sheet of a window too small for its
// panel opens in a modal surface owned by the window instead of sliding
// in; the window stays modal, its layer paints nothing; the answer closes
// the surface, and the next sheet of the queue opens after it; a sheet
// that fits slides in as before. Closing the window answers it.
func TestSheetSmallWindowSurface(t *testing.T) {
	sheetRun(t, driver.DefaultAppearance(), func(e *sheetEnv) {
		e.w.SetContentSize(driver.Size{W: 200, H: 100})
		e.show(oneButtonAlert)
		e.show(twoButtonAlert)
		e.settle()
		m := e.modal()
		as := e.d.Alerts()
		if m.Owner() != e.sf || as[0].Surface() != shell.Surface(m) || as[0].Window() != e.w || !e.w.Modal() {
			t.Fatalf("owner %v, surface %v, modal %v", m.Owner(), as[0].Surface(), e.w.Modal())
		}
		if e.w.SheetRoot() != nil || strings.Contains(e.sf.Dump(), "Saved") {
			t.Error("the layer shows the sheet too")
		}
		if e.w.bar.lights[lightClose].Enabled() || e.sf.UserClose() {
			t.Error("the window can close")
		}
		m.Key(driver.KeyReturn, 0)
		if e.answers() != "Saved=0" || !m.Closed() {
			t.Fatal("Return did not answer the sheet's surface")
		}
		next := e.modal()
		if next.Owner() != e.sf || !e.w.Modal() {
			t.Fatal("the next sheet did not open in a surface")
		}
		next.Key(driver.KeyEscape, 0)
		if e.answers() != "Delete the item?=1" || e.sh.Modal() != nil || e.w.Modal() || !e.w.IsKey() {
			t.Error("the queue did not end")
		}

		e.w.SetContentSize(driver.Size{W: 400, H: 300})
		e.show(oneButtonAlert)
		if e.sh.Modal() != nil || e.w.SheetRoot() == nil {
			t.Error("a sheet that fits did not slide in")
		}
		e.sf.Key(driver.KeyReturn, 0)
		e.settle()
		e.answers()

		e.w.SetContentSize(driver.Size{W: 200, H: 100})
		e.show(twoButtonAlert)
		e.show(oneButtonAlert)
		e.w.Close()
		if e.answers() != "Delete the item?=1 Saved=0" || e.sh.Modal() != nil || len(e.sh.Surfaces()) != 0 {
			t.Errorf("close: modal %v, %d surfaces", e.sh.Modal(), len(e.sh.Surfaces()))
		}
	})
}

// TestAboutAlert: the App menu's About is enabled and shows an app-modal
// alert with the app name, no detail and an OK button Escape presses too.
func TestAboutAlert(t *testing.T) {
	sheetRun(t, driver.DefaultAppearance(), func(e *sheetEnv) {
		e.w.menuPicked(menubar.Ref{Menu: appMenu, Item: itemAbout})
		as := e.d.Alerts()
		if len(as) != 1 || as[0].Parent != nil || as[0].Spec.Message != "Test" || as[0].Spec.Detail != "" ||
			len(as[0].Spec.Buttons) != 1 || as[0].Spec.Buttons[0] != "OK" {
			t.Fatalf("about: %+v", as)
		}
		e.modal().Key(driver.KeyEscape, 0)
		if len(e.d.Alerts()) != 0 || e.sh.Modal() != nil {
			t.Error("Escape did not close About")
		}
	})
}
