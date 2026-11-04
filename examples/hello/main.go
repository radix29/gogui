// Command hello is the smallest gogui application: a window with a label,
// a text field and a default button (triggered by Return).
package main

import (
	"log"

	"github.com/radix29/gogui"
)

func main() {
	if err := gogui.Run(gogui.AppConfig{Name: "Hello"}, func(app *gogui.App) {
		greeting := gogui.NewLabel("Hello!").SetFont(gogui.BoldSystemFont(15))
		name := gogui.NewTextField().SetPlaceholder("Your name")

		greet := gogui.NewButton("Greet").SetDefault(true)
		greet.OnClick(func() {
			greeting.SetText("Hello, " + name.Text() + "!")
		})

		win := app.NewWindow(gogui.WindowConfig{
			Title:  "Hello",
			Size:   gogui.Size{W: 360, H: 180},
			Style:  gogui.Titled | gogui.Closable | gogui.Miniaturizable | gogui.Resizable,
			Center: true,
		})
		win.SetContent(
			gogui.VStack(12,
				greeting,
				name,
				gogui.HStack(8, gogui.Spacer(), greet),
			).Padding(20),
		)
		win.OnClose(app.Quit)
		win.Show()
	}); err != nil {
		log.Fatal(err)
	}
}
