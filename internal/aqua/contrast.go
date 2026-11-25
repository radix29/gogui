package aqua

import (
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
)

// Increase Contrast (PLAN.md S6.1d, S6.1e). Borders and separators take
// their contrast values through ColorToken.Value; the shapes whose normal
// look has no border (an accent-filled bezel: a default button, an on
// checkbox or radio, the pop-up button's indicator; the slider's track)
// get a solid outline, Controls.BezelOutline or SliderTrackOutline, drawn
// only with Appearance.IncreaseContrast. The tokens are transparent without
// it, so skipping the stroke changes no pixel and keeps the normal paint
// dumps as they were.

// paintOutline draws the Increase Contrast outline col inside r
// (strokeInside), one device pixel wide at least (1 pt rounded to whole
// pixels, as the BezelBorder), when a has IncreaseContrast, and nothing
// otherwise.
func paintOutline(c render.Canvas, r driver.Rect, radius, scale float64, a driver.Appearance, col driver.Color) {
	if a.IncreaseContrast {
		strokeInside(c, r, radius, pixelWidth(1, scale), col)
	}
}
