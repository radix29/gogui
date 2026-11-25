package aqua

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/render/record"
)

// The alerts of the tests (S5.7a).
var (
	oneButtonAlert = driver.AlertSpec{Message: "Saved", Detail: "The document was saved.", Buttons: []string{"OK"}, Cancel: -1}
	twoButtonAlert = driver.AlertSpec{
		Style: driver.AlertWarning, Message: "Delete the item?", Detail: "This cannot be undone.",
		Buttons: []string{"Delete", "Cancel"}, Cancel: 1,
	}
	threeButtonAlert = driver.AlertSpec{
		Message: `Do you want to save the changes made to the document "Untitled"?`,
		Detail:  "Your changes will be lost if you don't save them.",
		Buttons: []string{"Save", "Don't Save", "Cancel"}, Cancel: 2,
	}
	longTitlesAlert = driver.AlertSpec{
		Style: driver.AlertCritical, Message: "The disk is full",
		Buttons: []string{"Open Storage Settings", "Cancel"}, Cancel: 1,
	}
)

// alertInfSize is an unbounded size.
var alertInfSize = driver.Size{W: math.Inf(1), H: math.Inf(1)}

// TestAlertLayout checks the geometry over the fake metrics: the fixed
// width, the icon centred under PaddingTop, the texts wrapped to the
// padded width one below the other, the detail and its gap left out when
// empty, two short buttons side by side with the default on the right,
// long titles and three buttons stacked with the default on top, the
// content height ending PaddingBottom below the last row, and the shadow
// margin around the panel.
func TestAlertLayout(t *testing.T) {
	d := startDriver(t)
	at := &tokens.Alert
	bh := tokens.Metrics.Control(driver.ControlSizeRegular).Height
	inner := at.Width - 2*at.PaddingX
	for _, c := range []struct {
		name    string
		spec    driver.AlertSpec
		stacked bool
	}{
		{"one button", oneButtonAlert, true},
		{"two buttons", twoButtonAlert, false},
		{"three buttons", threeButtonAlert, true},
		{"long titles", longTitlesAlert, true},
	} {
		a := newAlert(d, c.spec)
		fit := a.FittingSize(alertInfSize)
		if fit.W != at.Width {
			t.Errorf("%s: fitting width %v", c.name, fit.W)
		}
		a.SetFrame(driver.Rect{X: 5, Y: 7, W: fit.W, H: fit.H})
		if a.Stacked() != c.stacked {
			t.Errorf("%s: stacked %v", c.name, a.Stacked())
		}
		if icon := a.IconRect(); icon != (driver.Rect{X: (at.Width - at.IconSize) / 2, Y: at.PaddingTop, W: at.IconSize, H: at.IconSize}) {
			t.Errorf("%s: icon %v", c.name, icon)
		}
		y := at.PaddingTop + at.IconSize + at.IconGap
		msg := ElementOf(a.message).base().Frame()
		mh := math.Ceil(d.text.NewLayout(c.spec.Message, a.message.TextFont(), inner).Size().H)
		if msg != (driver.Rect{X: at.PaddingX, Y: y, W: inner, H: mh}) {
			t.Errorf("%s: message %v, want height %v at %v", c.name, msg, mh, y)
		}
		y += mh
		if det := ElementOf(a.detail); c.spec.Detail == "" {
			if !det.base().Hidden() {
				t.Errorf("%s: empty detail shown", c.name)
			}
		} else {
			y += at.DetailGap
			dh := math.Ceil(d.text.NewLayout(c.spec.Detail, a.detail.TextFont(), inner).Size().H)
			if det.base().Frame() != (driver.Rect{X: at.PaddingX, Y: y, W: inner, H: dh}) {
				t.Errorf("%s: detail %v", c.name, det.base().Frame())
			}
			y += dh
		}
		y += at.ButtonsGap
		if c.stacked {
			for i := range c.spec.Buttons {
				if f := ElementOf(a.Button(i)).base().Frame(); f != (driver.Rect{X: at.PaddingX, Y: y, W: inner, H: bh}) {
					t.Errorf("%s: button %d %v", c.name, i, f)
				}
				y += bh + at.ButtonGap
			}
			y -= at.ButtonGap
		} else {
			bw := (inner - at.ButtonGap) / 2
			if f := ElementOf(a.Button(1)).base().Frame(); f != (driver.Rect{X: at.PaddingX, Y: y, W: bw, H: bh}) {
				t.Errorf("%s: left button %v", c.name, f)
			}
			if f := ElementOf(a.Button(0)).base().Frame(); f != (driver.Rect{X: at.PaddingX + bw + at.ButtonGap, Y: y, W: bw, H: bh}) {
				t.Errorf("%s: default button %v", c.name, f)
			}
			y += bh
		}
		y += at.PaddingBottom
		if fit.H != y || a.ContentHeight() != y {
			t.Errorf("%s: fitting height %v, content %v, want %v", c.name, fit.H, a.ContentHeight(), y)
		}
		for i := range c.spec.Buttons {
			if b := a.Button(i); b.IsDefault != (i == 0) || b.Title != c.spec.Buttons[i] {
				t.Errorf("%s: button %d %q default %v", c.name, i, b.Title, b.IsDefault)
			}
		}
		if got := a.FittingSize(driver.Size{W: 100, H: 50}); got != (driver.Size{W: 100, H: 50}) {
			t.Errorf("%s: clamped %v", c.name, got)
		}
	}

	// The shadow margin grows the fitting size and moves the panel and
	// everything in it.
	a := newAlert(d, twoButtonAlert)
	plain := a.FittingSize(alertInfSize)
	a.SetFrame(driver.Rect{W: plain.W, H: plain.H})
	msg := ElementOf(a.message).base().Frame()
	a.SetShadow(true)
	in := a.ShadowMargin()
	if in != (driver.Insets{Top: at.ShadowBlur - at.ShadowOffsetY, Left: at.ShadowBlur, Bottom: at.ShadowBlur + at.ShadowOffsetY, Right: at.ShadowBlur}) {
		t.Errorf("shadow margin %v", in)
	}
	fit := a.FittingSize(alertInfSize)
	if fit != (driver.Size{W: plain.W + in.Left + in.Right, H: plain.H + in.Top + in.Bottom}) {
		t.Errorf("fitting with shadow %v, without %v", fit, plain)
	}
	a.SetFrame(driver.Rect{W: fit.W, H: fit.H})
	if a.Panel() != (driver.Rect{X: in.Left, Y: in.Top, W: plain.W, H: plain.H}) ||
		ElementOf(a.message).base().Frame() != msg.Offset(driver.Point{X: in.Left, Y: in.Top}) {
		t.Errorf("panel %v, message %v", a.Panel(), ElementOf(a.message).base().Frame())
	}

	// No buttons: an "OK" button is supplied.
	a = newAlert(d, driver.AlertSpec{Message: "M", Cancel: -1})
	if len(a.Spec().Buttons) != 1 || a.Button(0).Title != "OK" || !a.Stacked() {
		t.Errorf("default button %v", a.Spec().Buttons)
	}
}

// TestAlertPress: a click on a button answers with its index in
// AlertSpec.Buttons, once (S5.7b); a press without a press func answers
// all the same.
func TestAlertPress(t *testing.T) {
	d := startDriver(t)
	a := newAlert(d, threeButtonAlert)
	if _, ok := a.Result(); ok {
		t.Fatal("answered before a press")
	}
	var got []int
	a.SetPressFunc(func(i int) { got = append(got, i) })
	for _, i := range []int{2, 0, 1} {
		a.Button(i).UserClick()
	}
	if i, ok := a.Result(); fmt.Sprint(got) != "[2]" || !ok || i != 2 {
		t.Errorf("presses %v, result %d %v", got, i, ok)
	}
	a = newAlert(d, threeButtonAlert)
	a.Button(1).UserClick() // no press func
	a.SetPressFunc(func(i int) { t.Errorf("press %d after the answer", i) })
	a.Button(0).UserClick()
	if i, ok := a.Result(); !ok || i != 1 {
		t.Errorf("result %d %v", i, ok)
	}
}

// TestAlertAccessibility: a dialog labelled with the message, with the
// message, the detail and the buttons as children in spec order; an empty
// detail is left out.
func TestAlertAccessibility(t *testing.T) {
	d := startDriver(t)
	a := newAlert(d, twoButtonAlert)
	a.SetFrame(driver.RectOf(driver.Point{}, a.FittingSize(alertInfSize)))
	tree := AccessibilityTree(a)
	var got []string
	for _, n := range tree.Children {
		got = append(got, n.Role.String()+" "+n.Label)
	}
	if tree.Role != a11y.RoleDialog || tree.Label != twoButtonAlert.Message ||
		strings.Join(got, ", ") != "text Delete the item?, text This cannot be undone., button Delete, button Cancel" {
		t.Errorf("tree:\n%s", tree.Dump())
	}
	if n := tree.Find(a11y.RoleButton, "Cancel"); n == nil || n.Frame != ElementOf(a.Button(1)).base().Frame() {
		t.Errorf("cancel button node %v", n)
	}
	a = newAlert(d, longTitlesAlert)
	if n := len(AccessibilityTree(a).Children); n != 3 {
		t.Errorf("%d children with an empty detail", n)
	}
}

// TestAlertIcons checks the procedural icons: the app placeholder alone
// for an informational and a warning alert (as NSAlert), the caution
// triangle with the placeholder as a badge at its bottom right for a
// critical one.
func TestAlertIcons(t *testing.T) {
	d := startDriver(t)
	at := &tokens.Alert
	paintOf := func(spec driver.AlertSpec) string {
		a := newAlert(d, spec)
		a.SetFrame(driver.RectOf(driver.Point{}, a.FittingSize(alertInfSize)))
		c := record.New(render.Size{W: 400, H: 400})
		a.Paint(c)
		return c.Dump()
	}
	ap := driver.DefaultAppearance()
	yellow := hexColor(at.CautionFill.Value(ap))
	x := (at.Width - at.IconSize) / 2
	icon := fmt.Sprintf("(%g,%g,%g,%g)", x, at.PaddingTop, at.IconSize, at.IconSize)
	badge := fmt.Sprintf("(%g,%g,%g,%g)", x+at.IconSize-at.BadgeSize, at.PaddingTop+at.IconSize-at.BadgeSize, at.BadgeSize, at.BadgeSize)
	for _, style := range []driver.AlertStyle{driver.AlertInformational, driver.AlertWarning} {
		spec := oneButtonAlert
		spec.Style = style
		got := paintOf(spec)
		if !strings.Contains(got, "fillRRect "+icon) || strings.Contains(got, yellow) {
			t.Errorf("style %d:\n%s", style, got)
		}
	}
	got := paintOf(longTitlesAlert)
	if !strings.Contains(got, yellow) || !strings.Contains(got, "fillRRect "+badge) || strings.Contains(got, "fillRRect "+icon) {
		t.Errorf("critical:\n%s", got)
	}
}

// TestAlertStates pins the paint of the alert: one, two and three
// buttons, long titles (stacked) with an empty detail and the critical
// icon, the own shadow, in light and dark, with Increase Contrast and in
// an inactive window (the default button drawn plain), as record dumps of
// the alert in a window in testdata/alert.states.
func TestAlertStates(t *testing.T) {
	var out strings.Builder
	for _, dark := range []bool{false, true} {
		for _, st := range []struct {
			name     string
			spec     driver.AlertSpec
			shadow   bool
			contrast bool
			inactive bool
		}{
			{name: "one button", spec: oneButtonAlert},
			{name: "two buttons, warning", spec: twoButtonAlert},
			{name: "three buttons", spec: threeButtonAlert},
			{name: "long titles, no detail, critical", spec: longTitlesAlert},
			{name: "shadow", spec: twoButtonAlert, shadow: true},
			{name: "increased contrast", spec: twoButtonAlert, contrast: true},
			{name: "increased contrast, three buttons", spec: threeButtonAlert, contrast: true},
			{name: "increased contrast, long titles, critical", spec: longTitlesAlert, contrast: true},
			{name: "increased contrast, inactive", spec: twoButtonAlert, contrast: true, inactive: true},
			{name: "inactive", spec: twoButtonAlert, inactive: true},
		} {
			var log []string
			w, _, s, _ := popupWindow(t, &log)
			w.SetAppearance(driver.Appearance{Dark: dark, Accent: driver.DefaultAccent, IncreaseContrast: st.contrast})
			a := newAlert(newDriver(s), st.spec)
			a.SetShadow(st.shadow)
			// In the window's content, so it has the window's key state and
			// appearance; its subtree painted alone.
			w.content.SetChildren(a)
			size := a.FittingSize(alertInfSize)
			a.SetFrame(driver.RectOf(driver.Point{}, size))
			if st.inactive {
				s.Deactivate()
			}
			mode := "light"
			if dark {
				mode = "dark"
			}
			fmt.Fprintf(&out, "## %s, %s, %gx%g\n%s\n", st.name, mode, size.W, size.H, dump(t, a, size))
		}
	}
	got := out.String()
	path := filepath.Join("testdata", "alert.states")
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
		t.Errorf("alert states differ from %s (rerun with -update if the change is intended):\n%s", path, got)
	}
}
