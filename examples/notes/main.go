// Command notes is a minimal plain-text editor. It shows the multi-line
// TextView (which scrolls itself), a ScrollView holding a long list of
// controls, and a menu bar with keyboard shortcuts. On Windows/Linux the menu
// bar is drawn in the window's title-bar area, next to the traffic lights;
// Alt or F10 reaches it from the keyboard.
//
// Usage: notes [file]. With a file, the editor opens with its text (it is
// not saved back). The word count of a large text runs on a goroutine, so
// typing stays smooth: the text is an immutable string, safe to read there,
// and the label is set back on the UI thread with gogui.Do.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"unicode"
	"unicode/utf8"

	"github.com/radix29/gogui"
)

const welcome = "Welcome to Notes.\n\nType here. Scrolling, selection, undo and IME all work like on a Mac.\n"

func main() {
	text, title := welcome, "Notes"
	if len(os.Args) > 1 {
		b, err := os.ReadFile(os.Args[1])
		if err != nil {
			log.Fatal(err)
		}
		text, title = string(b), filepath.Base(os.Args[1])+" – Notes"
	}
	if err := gogui.Run(gogui.AppConfig{Name: "Notes"}, func(app *gogui.App) {
		editor := gogui.NewTextView().
			SetText(text).
			SetFont(gogui.SystemFont(13))

		stats := gogui.NewLabel(statsOf(text)).SetTextColor(gogui.ColorSecondaryLabel)
		// One counting goroutine; latest holds the newest text not yet
		// counted, so a burst of keystrokes is counted once. closed is set on
		// the UI thread when the window closes, which destroys its widgets.
		latest := make(chan string, 1)
		closed := false
		go func() {
			for s := range latest {
				line := statsOf(s)
				gogui.Do(func() {
					if !closed {
						stats.SetText(line)
					}
				})
			}
		}()
		recount := func(s string) {
			select {
			case <-latest: // replaced by the newer text
			default:
			}
			latest <- s
		}
		editor.OnChange(recount)

		// A settings panel longer than the window, to exercise ScrollView.
		settings := gogui.VStack(8)
		for i := 1; i <= 30; i++ {
			settings.Add(gogui.NewCheckbox(fmt.Sprintf("Option %d", i)))
		}
		sidebar := gogui.NewScrollView(settings.SetPadding(12)).
			SetHasVerticalScroller(true).
			SetMinWidth(180)

		app.SetMenuBar(gogui.MenuBar(
			gogui.AppMenu(),
			gogui.Menu("File",
				gogui.MenuItem("New", gogui.Shortcut(gogui.KeyPrimary, 'N'),
					func() { editor.SetText(""); recount("") }), // SetText calls no OnChange
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
			Title:   title,
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
			gogui.HStack(8, stats, gogui.Spacer()).SetPadding(6),
		))

		win.OnClose(func() { closed = true; app.Quit() })
		win.Show()
		editor.Focus()
	}); err != nil {
		log.Fatal(err)
	}
}

// statsOf returns the label's text for s: its words (runs of non-space
// characters, as strings.Fields counts them, but without allocating) and
// its characters.
func statsOf(s string) string {
	words, chars, inWord := 0, 0, false
	for i := 0; i < len(s); {
		r, n := rune(s[i]), 1
		if r >= utf8.RuneSelf {
			r, n = utf8.DecodeRuneInString(s[i:])
		}
		i += n
		chars++
		space := unicode.IsSpace(r)
		if !space && !inWord {
			words++
		}
		inWord = !space
	}
	return fmt.Sprintf("%d words, %d characters", words, chars)
}
