package gogui_test

import (
	"fmt"
	"strings"
	"time"

	"github.com/radix29/gogui"
)

// The examples run as tests on the headless driver, which shows nothing,
// and each one ends its app with App.Quit, which a real app leaves to the
// user (the Quit menu item, or closing its main window).

// A complete app: Run starts the event loop and calls the ready function
// on the UI thread, which builds a window and shows it. Run returns after
// Quit.
func ExampleRun() {
	err := gogui.Run(gogui.AppConfig{Name: "Hello"}, func(app *gogui.App) {
		win := app.NewWindow(gogui.WindowConfig{
			Title:  "Hello",
			Size:   gogui.Size{W: 320, H: 120},
			Style:  gogui.Titled | gogui.Closable | gogui.Miniaturizable | gogui.Resizable,
			Center: true,
		})
		win.SetContent(gogui.VStack(12, gogui.NewLabel("Hello, world!")).SetPadding(20))
		win.OnClose(app.Quit) // closing the only window quits
		win.Show()
		fmt.Println(win.Title(), win.ContentSize())

		app.Quit()
	})
	fmt.Println("Run returned", err)
	// Output:
	// Hello {320 120}
	// Run returned <nil>
}

// A window with stacks: a form of labelled fields above a row of buttons
// pushed to the trailing edge by a Spacer. Return presses the default
// button and Escape the cancel button.
func ExampleVStack() {
	gogui.Run(gogui.AppConfig{Name: "Sign Up"}, func(app *gogui.App) {
		name := gogui.NewTextField().SetPlaceholder("Jane Appleseed")
		email := gogui.NewTextField().SetPlaceholder("jane@example.com")
		news := gogui.NewCheckbox("Send me news")

		cancel := gogui.NewButton("Cancel").SetCancel(true)
		create := gogui.NewButton("Create Account").SetDefault(true)
		create.OnClick(func() { fmt.Println("create", name.Text()) })

		win := app.NewWindow(gogui.WindowConfig{
			Title: "Sign Up",
			Size:  gogui.Size{W: 420, H: 200},
			Style: gogui.Titled | gogui.Closable,
		})
		cancel.OnClick(win.Close)
		win.SetContent(gogui.VStack(20,
			gogui.Form(
				gogui.Row("Name:", name),
				gogui.Row("Email:", email),
				gogui.Row("", news),
			),
			gogui.HStack(12, gogui.Spacer(), cancel, create),
		).SetPadding(20))
		win.Show()

		name.SetText("Ada") // setters never call the handlers
		fmt.Println(name.Text(), news.Checked())

		app.Quit()
	})
	// Output:
	// Ada false
}

// Work that takes long runs on its own goroutine, which hands the result
// to the UI thread with Do: widgets may only be touched there.
func ExampleDo() {
	gogui.Run(gogui.AppConfig{Name: "Worker"}, func(app *gogui.App) {
		status := gogui.NewLabel("Working…")
		win := app.NewWindow(gogui.WindowConfig{Title: "Worker", Size: gogui.Size{W: 240, H: 80}})
		win.SetContent(gogui.VStack(0, status).SetPadding(20))
		win.Show()

		go func() {
			time.Sleep(10 * time.Millisecond) // the long work
			sum := 0
			for i := 1; i <= 100; i++ {
				sum += i
			}
			gogui.Do(func() {
				status.SetText(fmt.Sprint("Done: ", sum))
				fmt.Println(status.Text())
				app.Quit()
			})
		}()
	})
	// Output:
	// Done: 5050
}

// DoSync waits for the UI thread, so a goroutine can read a widget's state.
func ExampleDoSync() {
	gogui.Run(gogui.AppConfig{Name: "Reader"}, func(app *gogui.App) {
		field := gogui.NewTextField().SetText("draft")

		go func() {
			var text string
			ok := gogui.DoSync(func() { text = field.Text() })
			fmt.Println(text, ok)
			gogui.Do(app.Quit)
		}()
	})
	// Output:
	// draft true
}

// A menu bar with the standard menus and a File menu of the app's own. On
// macOS it is the screen's menu bar; on Windows and Linux every titled
// window draws it in its title bar. Shortcuts written with KeyPrimary are ⌘
// on macOS and Ctrl elsewhere.
func ExampleApp_SetMenuBar() {
	gogui.Run(gogui.AppConfig{Name: "Notes"}, func(app *gogui.App) {
		newNote := gogui.Shortcut(gogui.KeyPrimary, 'n')
		app.SetMenuBar(gogui.MenuBar(
			gogui.AppMenu(),
			gogui.Menu("File",
				gogui.MenuItem("New Note", newNote, func() { fmt.Println("new note") }),
				gogui.MenuSeparator(),
				gogui.MenuItem("Export…", gogui.KeyShortcut{}, nil), // no action: disabled
			),
			gogui.EditMenu(),
			gogui.WindowMenu(),
		))

		save := gogui.Shortcut(gogui.KeyPrimary|gogui.KeyShift, 's')
		for _, s := range []gogui.KeyShortcut{newNote, save, gogui.ShortcutCopy()} {
			fmt.Printf("%-6s %s\n", s.Display(true), s.Display(false))
		}

		app.Quit()
	})
	// Output:
	// ⌘N     Ctrl+N
	// ⇧⌘S    Ctrl+Shift+S
	// ⌘C     Ctrl+C
}

// An alert attached to a window as a sheet. done gets the index of the
// button the user chose; the headless driver can't answer an alert, so
// here done never runs.
func ExampleAlert_Show() {
	gogui.Run(gogui.AppConfig{Name: "Editor"}, func(app *gogui.App) {
		win := app.NewWindow(gogui.WindowConfig{Title: "Untitled", Size: gogui.Size{W: 480, H: 320}})
		win.Show()

		alert := gogui.Alert{
			Style:   gogui.AlertWarning,
			Message: `Do you want to save the changes made to "Untitled"?`,
			Detail:  "Your changes will be lost if you don't save them.",
			Buttons: []string{"Save", "Cancel", "Don't Save"},
		}
		alert.Show(win, func(result int) {
			switch alert.Buttons[result] {
			case "Save":
				fmt.Println("saving")
			case "Don't Save":
				win.Close()
			}
		})
		fmt.Println("shown:", strings.Join(alert.Buttons, ", "))

		app.Quit()
	})
	// Output:
	// shown: Save, Cancel, Don't Save
}
