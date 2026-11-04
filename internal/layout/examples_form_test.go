package layout

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
)

// withFormPeer gives f a stub container peer, as every public form has one.
func withFormPeer(f *Form) (*Form, *stub.Container) {
	c := stub.NewContainer()
	f.SetPeer(c)
	return f, c
}

// formTree is examples/form:
//
//	VStack(16,
//		Form(Row("Name:", name), Row("E-mail:", email),
//			Row("Password:", password), Row("Role:", role),
//			Row("Plan:", plan), Row("", newsletter), Row("", terms)),
//		Spacer(),
//		HStack(8, Spacer(), cancel, create),
//	).Padding(20)
type formTree struct {
	root                    *Root
	vstack, form, plan, row *stub.Container
	labels                  [5]*stub.Label // Name, E-mail, Password, Role, Plan
	name, email, password   *stub.TextField
	role                    *stub.PopUpButton
	radios                  [3]*stub.Radio
	newsletter, terms       *stub.Checkbox
	cancel, create          *stub.Button
}

func checkbox(title string) *stub.Checkbox {
	c := stub.NewCheckbox(nil)
	c.Title = title
	return c
}

// layoutForm builds the form tree and lays it out at size.
func layoutForm(size driver.Size) formTree {
	var t formTree
	for i, s := range []string{"Name:", "E-mail:", "Password:", "Role:", "Plan:"} {
		t.labels[i] = lbl(s)
	}
	t.name, t.email = stub.NewTextField(false, nil), stub.NewTextField(false, nil)
	t.password = stub.NewTextField(true, nil)
	t.role = stub.NewPopUpButton(nil)
	t.role.SetItems([]string{"Viewer", "Editor", "Administrator"})
	// A RadioGroup is a VStack of radios with 6 pt spacing (and a peer).
	var radios []Node
	for i, s := range []string{"Free", "Pro", "Team"} {
		t.radios[i] = stub.NewRadio(nil)
		t.radios[i].Title = s
		radios = append(radios, ctl(t.radios[i]))
	}
	t.newsletter = checkbox("Send me product updates")
	t.terms = checkbox("I agree to the terms of service")
	t.cancel, t.create = button("Cancel"), button("Create Account")
	t.cancel.IsCancel, t.create.IsDefault = true, true

	group, plan := withPeer(NewVStack(6, radios...))
	t.plan = plan
	form, fpeer := withFormPeer(NewForm(
		Row{t.labels[0], ctl(t.name)}, Row{t.labels[1], ctl(t.email)},
		Row{t.labels[2], ctl(t.password)}, Row{t.labels[3], ctl(t.role)},
		Row{t.labels[4], group}, Row{Control: ctl(t.newsletter)}, Row{Control: ctl(t.terms)},
	))
	row, rpeer := withPeer(NewHStack(8, NewSpacer(), ctl(t.cancel), ctl(t.create)))
	col, vpeer := withPeer(NewVStack(16, form, NewSpacer(), row))
	col.SetPadding(driver.UniformInsets(20))
	t.root, t.vstack, t.form, t.row = NewRoot(col), vpeer, fpeer, rpeer
	t.root.Layout(size)
	return t
}

func TestExampleForm(t *testing.T) {
	f := layoutForm(driver.Size{W: 460, H: 380})
	wantRect(t, "vstack", f.vstack.Frame, driver.Rect{W: 460, H: 380})
	// The form fills the 420 pt inner width (its fields expand). Rows: four
	// controls of 22, the radio group (3 × 18 + 2 × 6 = 66), two checkboxes
	// of 18, and 6 × 8 between them: 238.
	wantRect(t, "form", f.form.Frame, driver.Rect{X: 20, Y: 20, W: 420, H: 238})
	// Label column 63 ("Password:"), right-aligned, centered on the first
	// line: (22 − 16) / 2 = 3 next to a field, (18 − 16) / 2 = 1 next to the
	// radio group.
	labels := []driver.Rect{
		{X: 28, Y: 3, W: 35, H: 16}, {X: 14, Y: 33, W: 49, H: 16},
		{X: 0, Y: 63, W: 63, H: 16}, {X: 28, Y: 93, W: 35, H: 16},
		{X: 28, Y: 121, W: 35, H: 16},
	}
	for i, want := range labels {
		wantRect(t, f.labels[i].Text, f.labels[i].Frame, want)
	}
	// The controls start at x = 71; the text fields fill the 349 pt column,
	// the others keep their natural width.
	wantRect(t, "name", f.name.Frame, driver.Rect{X: 71, Y: 0, W: 349, H: 22})
	wantRect(t, "email", f.email.Frame, driver.Rect{X: 71, Y: 30, W: 349, H: 22})
	wantRect(t, "password", f.password.Frame, driver.Rect{X: 71, Y: 60, W: 349, H: 22})
	wantRect(t, "role", f.role.Frame, driver.Rect{X: 71, Y: 90, W: 7*13 + 32, H: 22})
	wantRect(t, "plan", f.plan.Frame, driver.Rect{X: 71, Y: 120, W: 46, H: 66})
	radios := []driver.Rect{{W: 46, H: 18}, {Y: 24, W: 39, H: 18}, {Y: 48, W: 46, H: 18}}
	for i, want := range radios {
		wantRect(t, f.radios[i].Title, f.radios[i].Frame, want)
	}
	wantRect(t, "newsletter", f.newsletter.Frame, driver.Rect{X: 71, Y: 194, W: 18 + 23*7, H: 18})
	wantRect(t, "terms", f.terms.Frame, driver.Rect{X: 71, Y: 220, W: 18 + 31*7, H: 18})
	// The spacer takes the leftover 340 − (238 + 22 + 2 × 16) = 48, so the
	// button row ends at the bottom padding: 20 + 238 + 16 + 48 + 16 = 338.
	wantRect(t, "button row", f.row.Frame, driver.Rect{X: 20, Y: 338, W: 420, H: 22})
	// Cancel (70) and Create Account (126) are trailing in the row.
	wantRect(t, "cancel", f.cancel.Frame, driver.Rect{X: 216, W: 70, H: 22})
	wantRect(t, "create", f.create.Frame, driver.Rect{X: 294, W: 126, H: 22})
}

// converterTree is examples/converter:
//
//	VStack(12,
//		Form(Row("Celsius:", celsius), Row("Fahrenheit:", fahrenheit)),
//		slider, NewSeparator(), HStack(8, syncBar, updated),
//	).Padding(20)
type converterTree struct {
	root                *Root
	vstack, form, row   *stub.Container
	cLabel, fLabel      *stub.Label
	celsius, fahrenheit *stub.TextField
	slider              *stub.Slider
	sep                 *stub.Separator
	syncBar             *stub.Progress
	updated             *stub.Label
}

// layoutConverter builds the converter tree with the status text updated
// and lays it out at size.
func layoutConverter(size driver.Size, updated string) converterTree {
	var t converterTree
	t.cLabel, t.fLabel = lbl("Celsius:"), lbl("Fahrenheit:")
	t.celsius, t.fahrenheit = stub.NewTextField(false, nil), stub.NewTextField(false, nil)
	t.slider, t.sep, t.syncBar = stub.NewSlider(nil), stub.NewSeparator(), stub.NewProgress()
	t.updated = lbl(updated)

	form, fpeer := withFormPeer(NewForm(Row{t.cLabel, ctl(t.celsius)}, Row{t.fLabel, ctl(t.fahrenheit)}))
	row, rpeer := withPeer(NewHStack(8, ctl(t.syncBar), ctl(t.updated)))
	col, vpeer := withPeer(NewVStack(12, form, ctl(t.slider), ctl(t.sep), row))
	col.SetPadding(driver.UniformInsets(20))
	t.root, t.vstack, t.form, t.row = NewRoot(col), vpeer, fpeer, rpeer
	t.root.Layout(size)
	return t
}

func TestExampleConverter(t *testing.T) {
	c := layoutConverter(driver.Size{W: 380, H: 220}, "")
	wantRect(t, "vstack", c.vstack.Frame, driver.Rect{W: 380, H: 220})
	// Label column 77 ("Fahrenheit:"); the fields start at x = 85 and fill
	// the 340 − 85 = 255 pt column.
	wantRect(t, "form", c.form.Frame, driver.Rect{X: 20, Y: 20, W: 340, H: 52})
	wantRect(t, "celsius label", c.cLabel.Frame, driver.Rect{X: 21, Y: 3, W: 56, H: 16})
	wantRect(t, "fahrenheit label", c.fLabel.Frame, driver.Rect{X: 0, Y: 33, W: 77, H: 16})
	wantRect(t, "celsius", c.celsius.Frame, driver.Rect{X: 85, Y: 0, W: 255, H: 22})
	wantRect(t, "fahrenheit", c.fahrenheit.Frame, driver.Rect{X: 85, Y: 30, W: 255, H: 22})
	// The slider and the separator fill the inner width.
	wantRect(t, "slider", c.slider.Frame, driver.Rect{X: 20, Y: 84, W: 340, H: 22})
	wantRect(t, "separator", c.sep.Frame, driver.Rect{X: 20, Y: 118, W: 340, H: 1})
	// The row fills the width because the bar expands; the leftover height
	// (nothing expands vertically) stays below it.
	wantRect(t, "row", c.row.Frame, driver.Rect{X: 20, Y: 131, W: 340, H: 22})
	// The empty status label is 0 wide, so the bar takes 340 − 8.
	wantRect(t, "sync bar", c.syncBar.Frame, driver.Rect{W: 332, H: 22})
	wantRect(t, "updated", c.updated.Frame, driver.Rect{X: 340, Y: 3, W: 0, H: 16})

	// After a sync the label is 18 × 7 = 126 wide and the bar shrinks.
	c = layoutConverter(driver.Size{W: 380, H: 220}, "Synced at 15:04:05")
	wantRect(t, "sync bar after sync", c.syncBar.Frame, driver.Rect{W: 206, H: 22})
	wantRect(t, "updated after sync", c.updated.Frame, driver.Rect{X: 214, Y: 3, W: 126, H: 16})
	wantRect(t, "row after sync", c.row.Frame, driver.Rect{X: 20, Y: 131, W: 340, H: 22})
}
