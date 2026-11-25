// Command gallery shows the scene of the golden-image tests
// (internal/aqua/gallery) built through the public API: labels, push buttons
// in every control size, checkboxes in their states, a radio group in a box,
// sliders, a progress bar, an indeterminate bar, a spinner and a scroll view
// scrolled off both axes. The internal scene places its widgets at fixed
// frames; this one lays out the same three columns with stacks, so the
// sizes come from the layout and may differ by a few points.
//
// Nothing here reacts to input beyond the controls' own behaviour; it is a
// window to look at, in light and dark and next to native macOS.
package main

import (
	"fmt"
	"log"

	"github.com/radix29/gogui"
)

func main() {
	if err := gogui.Run(gogui.AppConfig{Name: "Gallery"}, func(app *gogui.App) {
		// Column A: labels, a separator and push buttons.
		colA := gogui.VStack(6,
			gogui.NewLabel("Label"),
			gogui.NewLabel("Secondary Label").SetTextColor(gogui.ColorSecondaryLabel),
			gogui.NewLabel("Disabled Label").SetTextColor(gogui.ColorDisabledControlText),
			gogui.NewLabel("Small Label").SetFont(gogui.SystemFont(11)),
			gogui.NewLabel("Bold Label").SetFont(gogui.BoldSystemFont(0)),
			gogui.NewSeparator(),
			gogui.NewButton("Push Button"),
			gogui.NewButton("Default").SetDefault(true),
			gogui.NewButton("Cancel").SetCancel(true),
			gogui.NewButton("Disabled").SetEnabled(false),
			gogui.NewButton("Small").SetControlSize(gogui.ControlSizeSmall),
			gogui.NewButton("Mini").SetControlSize(gogui.ControlSizeMini),
			gogui.NewButton("Large").SetControlSize(gogui.ControlSizeLarge),
			gogui.Spacer(),
		)

		// Column B: checkboxes and a radio group in a box.
		focused := gogui.NewCheckbox("Focused")
		colB := gogui.VStack(6,
			gogui.NewCheckbox("Checkbox"),
			gogui.NewCheckbox("Checked").SetChecked(true),
			gogui.NewCheckbox("Mixed").SetState(gogui.StateMixed),
			gogui.NewCheckbox("Disabled").SetChecked(true).SetEnabled(false),
			focused,
			gogui.NewBox("Radio Group").SetContent(
				gogui.NewRadioGroup("Small", "Medium", "Large").SetSelected(1),
			),
			gogui.Spacer(),
		)

		// Column C: sliders, progress indicators and a scroll view whose
		// document, a 3 × 8 grid of labels, is larger than the viewport on
		// both axes.
		rows := make([]gogui.Widget, 0, 8)
		for row := range 8 {
			cells := make([]gogui.Widget, 0, 3)
			for col := range 3 {
				cells = append(cells, gogui.NewLabel(fmt.Sprintf("Item %d.%d", row+1, col+1)))
			}
			rows = append(rows, gogui.HStack(40, cells...))
		}
		scroll := gogui.NewScrollView(gogui.VStack(14, rows...).SetPadding(10)).
			SetHasVerticalScroller(true).
			SetHasHorizontalScroller(true).
			SetMinHeight(120)
		colC := gogui.VStack(10,
			gogui.NewSlider().SetRange(0, 100).SetValue(40),
			gogui.NewSlider().SetRange(0, 100).SetValue(75).SetTickMarks(5, false),
			gogui.NewSlider().SetRange(0, 100).SetValue(25).SetEnabled(false),
			gogui.NewProgressBar().SetRange(0, 100).SetValue(40),
			gogui.NewProgressBar().SetIndeterminate(true),
			gogui.HStack(8, gogui.NewSpinner().SetIndeterminate(true), gogui.Spacer()),
			scroll,
		)

		win := app.NewWindow(gogui.WindowConfig{
			Title:   "Gallery",
			Size:    gogui.Size{W: 640, H: 400},
			MinSize: gogui.Size{W: 560, H: 380},
			Style:   gogui.Titled | gogui.Closable | gogui.Miniaturizable | gogui.Resizable,
			Center:  true,
		})
		win.SetContent(gogui.HStack(16,
			colA, gogui.NewSeparator().SetVertical(true),
			colB, gogui.NewSeparator().SetVertical(true),
			colC,
		).SetPadding(20))
		win.OnClose(app.Quit)
		win.Show()
		focused.Focus()
		scroll.SetOffset(gogui.Point{X: 60, Y: 40})
	}); err != nil {
		log.Fatal(err)
	}
}
