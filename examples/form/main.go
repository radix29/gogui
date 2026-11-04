// Command form is a macOS-style account form. It shows the Form layout
// (right-aligned labels, left-aligned controls), keyboard navigation,
// Cancel/OK button semantics (Escape/Return) and validation alerts.
package main

import (
	"log"
	"strings"

	"github.com/radix29/gogui"
)

func main() {
	if err := gogui.Run(gogui.AppConfig{Name: "Account"}, func(app *gogui.App) {
		name := gogui.NewTextField().SetPlaceholder("Jane Appleseed")
		email := gogui.NewTextField().SetPlaceholder("name@example.com")
		password := gogui.NewSecureField()
		role := gogui.NewPopUpButton("Viewer", "Editor", "Administrator")
		plan := gogui.NewRadioGroup("Free", "Pro", "Team").SetSelected(0)
		newsletter := gogui.NewCheckbox("Send me product updates")
		terms := gogui.NewCheckbox("I agree to the terms of service")

		cancel := gogui.NewButton("Cancel").SetCancel(true)
		create := gogui.NewButton("Create Account").SetDefault(true).SetEnabled(false)
		terms.OnChange(func(on bool) { create.SetEnabled(on) })

		win := app.NewWindow(gogui.WindowConfig{
			Title:  "New Account",
			Size:   gogui.Size{W: 460, H: 380},
			Style:  gogui.Titled | gogui.Closable,
			Center: true,
		})

		cancel.OnClick(win.Close)
		create.OnClick(func() {
			if strings.TrimSpace(name.Text()) == "" || !strings.Contains(email.Text(), "@") {
				gogui.Alert{
					Style:   gogui.AlertCritical,
					Message: "Please check your details",
					Detail:  "A name and a valid e-mail address are required.",
				}.Show(win, nil)
				return
			}
			gogui.Alert{
				Message: "Account created",
				Detail:  "Welcome, " + name.Text() + " (" + role.Item(role.Selected()) + ").",
			}.Show(win, func(int) { win.Close() })
		})

		win.SetContent(gogui.VStack(16,
			gogui.Form(
				gogui.Row("Name:", name),
				gogui.Row("E-mail:", email),
				gogui.Row("Password:", password),
				gogui.Row("Role:", role),
				gogui.Row("Plan:", plan),
				gogui.Row("", newsletter),
				gogui.Row("", terms),
			),
			gogui.Spacer(),
			gogui.HStack(8, gogui.Spacer(), cancel, create),
		).Padding(20))

		win.OnClose(app.Quit)
		win.Show()
	}); err != nil {
		log.Fatal(err)
	}
}
