package aqua

import (
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

func activating(ev shell.PointerEvent) shell.PointerEvent { ev.Activating = true; return ev }

// TestRouterFirstClick (S6.1b): an activating press updates the hover but
// reaches no element, and its release and the drag in between capture
// nothing; the next press is routed. Over an element that accepts the first
// mouse (itself or through an ancestor) it is routed as usual. A lost
// release (CancelPointer, a new press of the button) drops nothing later.
func TestRouterFirstClick(t *testing.T) {
	w := newWindow(t)
	r := w.r
	w.btn.SetFocusable(true)
	w.btn.onEvent = func(ev Event) {
		if p, ok := ev.(PointerEvent); ok && p.Kind == PointerDown {
			w.btn.Focus()
		}
	}
	if r.Event(activating(down(25, 25))) {
		t.Fatal("an activating press reported handled")
	}
	checkLog(t, &w.log, "root enter 25,25", "panel enter 15,15", "btn enter 5,5")
	r.Event(shell.PointerEvent{Kind: shell.PointerMove, Pos: driver.Point{X: 30, Y: 25}, Buttons: shell.ButtonPrimary})
	checkLog(t, &w.log, "btn move 10,5") // the hover move, not a drag: nothing captured
	if r.Captured() != nil || r.Event(up(30, 25)) || r.Focused() != nil {
		t.Fatal("the activating press captured, its release was handled or the focus moved")
	}
	checkLog(t, &w.log)
	r.Event(down(30, 25))
	r.Event(up(30, 25))
	checkLog(t, &w.log, "btn down 10,5 x1", "btn focus true", "btn up 10,5 x1")

	// Accepted through an ancestor: routed and captured as usual.
	if w.btn.AcceptsFirstMouse() {
		t.Fatal("first mouse on by default")
	}
	w.panel.SetAcceptsFirstMouse(true)
	if !w.btn.AcceptsFirstMouse() || w.sb.AcceptsFirstMouse() {
		t.Fatal("AcceptsFirstMouse is not inherited from the ancestors only")
	}
	if !r.Event(activating(down(30, 25))) || r.Captured() != Element(w.btn) {
		t.Fatal("an accepted activating press was not routed")
	}
	r.Event(up(30, 25))
	checkLog(t, &w.log, "btn down 10,5 x1", "btn up 10,5 x1")
	w.panel.SetAcceptsFirstMouse(false)

	// A swallowed press whose release is lost: a new press of the button
	// and CancelPointer both forget it.
	r.Event(activating(down(30, 25)))
	r.Event(down(30, 25))
	r.Event(up(30, 25))
	checkLog(t, &w.log, "btn down 10,5 x1", "btn up 10,5 x1")
	r.Event(activating(down(30, 25)))
	r.CancelPointer()
	w.log = nil
	r.Event(move(30, 25))
	w.root.handles["up"] = true
	r.SetStrayReleases(true)
	r.Event(up(30, 25))
	checkLog(t, &w.log, "root enter 30,25", "panel enter 20,15", "btn enter 10,5", "btn move 10,5", "btn up 10,5 x1")

	// Over no element: dropped as well.
	w.log = nil
	r.Event(activating(down(500, 500)))
	if r.Event(up(500, 500)) {
		t.Fatal("the release of an activating press outside was handled")
	}
}

// viewCentre returns the centre of e in its root's (the surface's) points.
func viewCentre(e Element) driver.Point {
	return ToRoot(e, driver.Point{X: e.base().Bounds().W / 2, Y: e.base().Bounds().H / 2})
}

// TestWindowFirstClick (S6.1b): over the fake shell, the click that
// activates a window only activates it: a button is not clicked, a field is
// not focused, a drag captures nothing, a sheet's button does not answer;
// the next click acts. The traffic lights and the menu strip's titles act
// on the first click. A press into a window that an app-modal alert blocks
// makes the alert key and reaches neither.
func TestWindowFirstClick(t *testing.T) {
	sheetRun(t, driver.DefaultAppearance(), func(e *sheetEnv) {
		btn := viewCentre(e.btn.view)
		field := viewCentre(e.field.view)
		r := e.w.Router()

		e.sh.Deactivate()
		e.sf.Click(btn)
		if !e.w.IsKey() || e.n != 0 || r.Focused() != Element(e.field.view) {
			t.Fatalf("first click: key %v, clicks %d, focus %v", e.w.IsKey(), e.n, r.Focused())
		}
		e.clk.Advance(time.Second) // not a double click
		e.sf.Click(btn)
		if e.n != 1 {
			t.Fatalf("second click: clicks %d", e.n)
		}

		r.SetFocus(nil)
		e.sh.Deactivate()
		e.sf.Click(field)
		if r.Focused() != nil {
			t.Fatal("the first click focused the field")
		}
		e.clk.Advance(time.Second)
		e.sf.Click(field)
		if r.Focused() != Element(e.field.view) {
			t.Fatal("the second click did not focus the field")
		}

		e.sh.Deactivate()
		e.sf.Press(btn, shell.ButtonPrimary)
		e.sf.Move(btn.Add(driver.Point{X: 3}))
		if r.Captured() != nil || e.btn.view.State().Interaction == InteractionPressed {
			t.Fatal("the activating press captured or pressed the button")
		}
		e.sf.Release(btn, shell.ButtonPrimary)
		if e.n != 1 {
			t.Fatalf("the activating drag clicked: %d", e.n)
		}
		e.clk.Advance(time.Second)

		// The menu strip: a title opens its menu on the first click.
		st := e.w.menuStrip()
		e.sh.Deactivate()
		e.sf.Click(centre(st.TitleRect(1)))
		if st.OpenMenu() != 1 || e.sh.GrabPopup() == nil {
			t.Fatalf("first click on a strip title: open %d", st.OpenMenu())
		}
		e.sh.Deactivate() // dismisses the menu
		e.clk.Advance(time.Second)

		// A sheet's button needs a second click.
		e.show(driver.AlertSpec{Message: "Sheet", Buttons: []string{"OK", "Cancel"}})
		e.settle()
		e.sh.Deactivate()
		e.sf.Click(e.sheetButton(0))
		if e.answers() != "" || !e.w.IsKey() {
			t.Fatal("the first click answered the sheet")
		}
		e.clk.Advance(time.Second)
		e.sf.Click(e.sheetButton(0))
		if got := e.answers(); got != "Sheet=0" {
			t.Fatalf("second click on the sheet: %q", got)
		}
		e.settle()

		// An app-modal alert: a press on the blocked window activates the
		// alert, which gets no press; its own first click only activates it.
		e.showApp(driver.AlertSpec{Message: "App", Buttons: []string{"OK"}})
		e.settle()
		m := e.modal()
		e.sh.Deactivate()
		e.sf.Click(btn)
		if !m.IsKey() || e.w.IsKey() || e.n != 1 || e.answers() != "" {
			t.Fatalf("press on the blocked window: alert key %v, clicks %d", m.IsKey(), e.n)
		}
		ok := viewCentre(e.d.alertSurfaces[0].root.alert.Button(0).view)
		e.sh.Deactivate()
		m.Click(ok)
		if !m.IsKey() || e.answers() != "" {
			t.Fatal("the first click answered the app-modal alert")
		}
		e.clk.Advance(time.Second)
		m.Click(ok)
		if got := e.answers(); got != "App=0" {
			t.Fatalf("second click on the app-modal alert: %q", got)
		}
		e.settle()

		// The traffic lights act on the first click.
		e.sh.Deactivate()
		e.sf.Click(viewCentre(e.w.bar.lights[lightMinimize]))
		if !e.sf.Minimized() {
			t.Fatal("the first click on the minimize light did nothing")
		}
	})
}
