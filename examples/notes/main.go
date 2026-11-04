// Command notes is a minimal plain-text editor. It shows the multi-line
// TextView (which scrolls itself), a ScrollView holding a long list of
// controls, and a menu bar with keyboard shortcuts. On Windows/Linux the menu
// bar is drawn in the window's title-bar area, next to the traffic lights.
package main

import (
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/radix29/gogui"
)

func main() {
	if err := gogui.Run(gogui.AppConfig{Name: "Notes"}, func(app *gogui.App) {
		editor := gogui.NewTextView().
			SetText("Welcome to Notes.\n\nType here. Scrolling, selection, undo and IME all work like on a Mac.\n").
			SetFont(gogui.SystemFont(13))

		stats := gogui.NewLabel("").SetTextColor(gogui.ColorSecondaryLabel)
		updateStats := func(s string) {
			words := len(strings.Fields(s))
			stats.SetText(fmt.Sprintf("%d words, %d characters", words, utf8.RuneCountInString(s)))
		}
		editor.OnChange(updateStats)
		updateStats(editor.Text())

		// A settings panel longer than the window, to exercise ScrollView.
		settings := gogui.VStack(8)
		for i := 1; i <= 30; i++ {
			settings.Add(gogui.NewCheckbox(fmt.Sprintf("Option %d", i)))
		}
		sidebar := gogui.NewScrollView(settings.Padding(12)).
			SetHasVerticalScroller(true).
			SetMinWidth(180)

		app.SetMenuBar(gogui.MenuBar(
			gogui.AppMenu(),
			gogui.Menu("File",
				gogui.MenuItem("New", gogui.Shortcut(gogui.KeyPrimary, 'N'),
					func() { editor.SetText("") }),
			),
			gogui.EditMenu(), // Undo/Redo/Cut/Copy/Paste/Select All act on the focused TextView
			gogui.Menu("Format",
				gogui.MenuItem("Bigger", gogui.Shortcut(gogui.KeyPrimary, '+'),
					func() { editor.SetFont(gogui.SystemFont(editor.Font().Size + 1)) }),
				gogui.MenuItem("Smaller", gogui.Shortcut(gogui.KeyPrimary, '-'),
					func() { editor.SetFont(gogui.SystemFont(editor.Font().Size - 1)) }),
			),
		))

		win := app.NewWindow(gogui.WindowConfig{
			Title:   "Notes",
			Size:    gogui.Size{W: 720, H: 480},
			MinSize: gogui.Size{W: 480, H: 320},
			Style:   gogui.Titled | gogui.Closable | gogui.Miniaturizable | gogui.Resizable,
			Center:  true,
		})
		win.SetContent(gogui.VStack(0,
			gogui.HStack(0,
				sidebar,
				gogui.NewSeparator().SetVertical(true),
				editor, // expands to fill the remaining space
			),
			gogui.NewSeparator(),
			gogui.HStack(8, stats, gogui.Spacer()).Padding(6),
		))

		win.OnClose(app.Quit)
		win.Show()
		editor.Focus()
	}); err != nil {
		log.Fatal(err)
	}
}
