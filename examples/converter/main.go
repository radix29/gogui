// Command converter is a temperature converter. It shows live two-way
// updates between text fields and a slider, and updating the UI from a
// goroutine through gogui.Do.
package main

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/radix29/gogui"
)

func main() {
	if err := gogui.Run(gogui.AppConfig{Name: "Converter"}, func(app *gogui.App) {
		celsius := gogui.NewTextField()
		fahrenheit := gogui.NewTextField()
		slider := gogui.NewSlider().SetRange(-40, 100)
		syncBar := gogui.NewProgressBar().SetRange(0, 1)
		updated := gogui.NewLabel("").SetTextColor(gogui.ColorSecondaryLabel)

		const (
			fromNone = iota
			fromCelsius
			fromFahrenheit
			fromSlider
		)
		// Setters never fire OnChange, so the controls can't loop; the guard
		// only keeps a nested update (a handler calling setCelsius) out.
		updating := false
		setCelsius := func(c float64, from int) {
			if updating {
				return
			}
			updating = true
			defer func() { updating = false }()
			if from != fromCelsius {
				celsius.SetText(strconv.FormatFloat(c, 'f', 1, 64))
			}
			if from != fromFahrenheit {
				fahrenheit.SetText(strconv.FormatFloat(c*9/5+32, 'f', 1, 64))
			}
			if from != fromSlider {
				slider.SetValue(c)
			}
		}

		celsius.OnChange(func(s string) {
			if v, err := strconv.ParseFloat(s, 64); err == nil {
				setCelsius(v, fromCelsius)
			}
		})
		fahrenheit.OnChange(func(s string) {
			if v, err := strconv.ParseFloat(s, 64); err == nil {
				setCelsius((v-32)*5/9, fromFahrenheit)
			}
		})
		slider.OnChange(func(v float64) { setCelsius(v, fromSlider) })
		setCelsius(20, fromNone)

		// Background goroutine: simulates a periodic sync and marshals UI
		// updates back to the UI thread with gogui.Do.
		go func() {
			for tick := 0; ; tick++ {
				time.Sleep(100 * time.Millisecond)
				t := tick
				gogui.Do(func() {
					syncBar.SetValue(float64(t%50) / 49)
					if t%50 == 49 {
						updated.SetText(fmt.Sprintf("Synced at %s", time.Now().Format("15:04:05")))
					}
				})
			}
		}()

		win := app.NewWindow(gogui.WindowConfig{
			Title:  "Temperature",
			Size:   gogui.Size{W: 380, H: 220},
			Style:  gogui.Titled | gogui.Closable | gogui.Miniaturizable,
			Center: true,
		})
		win.SetContent(gogui.VStack(12,
			gogui.Form(
				gogui.Row("Celsius:", celsius),
				gogui.Row("Fahrenheit:", fahrenheit),
			),
			slider,
			gogui.NewSeparator(),
			gogui.HStack(8, syncBar, updated),
		).Padding(20))

		win.OnClose(app.Quit)
		win.Show()
	}); err != nil {
		log.Fatal(err)
	}
}
