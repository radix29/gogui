// Command controls is the v0.1 control gallery. It shows every control in
// its states (normal, disabled, default, mixed …).
//
// Run it on macOS (native AppKit) and on Windows/Linux (aqua) side by side to
// check visual and behavioural parity. See PLAN.md §8.3.
package main

import (
	"fmt"
	"log"

	"github.com/radix29/gogui"
)

func main() {
	if err := gogui.Run(gogui.AppConfig{Name: "Controls"}, func(app *gogui.App) {
		app.SetMenuBar(gogui.MenuBar(
			gogui.AppMenu(),  // About / Hide / Quit (native on macOS, drawn elsewhere)
			gogui.EditMenu(), // Undo / Redo / Cut / Copy / Paste / Select All
			gogui.Menu("View",
				gogui.MenuItem("Toggle Appearance",
					gogui.Shortcut(gogui.KeyPrimary|gogui.KeyShift, 'D'),
					func() { app.SetAppearance(app.Appearance().Toggled()) }),
			),
		))

		status := gogui.NewLabel("Ready").SetTextColor(gogui.ColorSecondaryLabel)
		report := func(format string, a ...any) { status.SetText(fmt.Sprintf(format, a...)) }

		// Buttons
		push := gogui.NewButton("Push Button")
		push.OnClick(func() { report("Push Button clicked") })
		def := gogui.NewButton("Default").SetDefault(true)
		def.OnClick(func() { report("Default clicked") })
		disabled := gogui.NewButton("Disabled").SetEnabled(false)

		// Checkboxes
		check := gogui.NewCheckbox("Checkbox").SetChecked(true)
		check.OnChange(func(on bool) { report("Checkbox: %v", on) })
		mixed := gogui.NewCheckbox("Mixed").SetState(gogui.StateMixed)
		checkOff := gogui.NewCheckbox("Disabled").SetEnabled(false)

		// Radio group
		radios := gogui.NewRadioGroup("Small", "Medium", "Large").SetSelected(1)
		radios.OnChange(func(i int) { report("Radio: %d", i) })

		// Text
		field := gogui.NewTextField().SetPlaceholder("Text Field")
		field.OnSubmit(func(s string) { report("Submitted: %q", s) })
		secure := gogui.NewSecureField().SetPlaceholder("Password")

		// Slider + progress
		progress := gogui.NewProgressBar().SetRange(0, 100).SetValue(40)
		slider := gogui.NewSlider().SetRange(0, 100).SetValue(40)
		slider.OnChange(func(v float64) { progress.SetValue(v) })
		busy := gogui.NewProgressBar().SetIndeterminate(true)

		// Pop-up button
		popup := gogui.NewPopUpButton("Red", "Green", "Blue").SetSelected(2)
		popup.OnChange(func(i int) { report("Pop-up: %s", popup.Item(i)) })

		win := app.NewWindow(gogui.WindowConfig{
			Title:   "Controls",
			Size:    gogui.Size{W: 520, H: 560},
			MinSize: gogui.Size{W: 420, H: 480},
			Style:   gogui.Titled | gogui.Closable | gogui.Miniaturizable | gogui.Resizable,
			Center:  true,
		})

		// Alert
		alert := gogui.NewButton("Show Alert…")
		alert.OnClick(func() {
			gogui.Alert{
				Style:   gogui.AlertWarning,
				Message: "Delete this item?",
				Detail:  "This action cannot be undone.",
				Buttons: []string{"Delete", "Cancel"}, // first = default (Return), "Cancel" = Escape
			}.Show(win, func(result int) { report("Alert result: %d", result) })
		})

		win.SetContent(gogui.VStack(14,
			gogui.NewBox("Buttons").SetContent(
				gogui.HStack(8, push, def, disabled, gogui.Spacer()),
			),
			gogui.NewBox("Toggles").SetContent(
				gogui.HStack(20, gogui.VStack(6, check, mixed, checkOff), radios, gogui.Spacer()),
			),
			gogui.NewBox("Text").SetContent(
				gogui.VStack(8, field, secure),
			),
			gogui.NewBox("Values").SetContent(
				gogui.VStack(10, slider, progress, gogui.HStack(8, busy, gogui.Spacer(), popup)),
			),
			gogui.NewSeparator(),
			gogui.HStack(8, status, gogui.Spacer(), alert),
		).Padding(20))

		win.OnClose(app.Quit)
		win.Show()
	}); err != nil {
		log.Fatal(err)
	}
}
