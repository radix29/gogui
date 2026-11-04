package aqua

import (
	"math"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
)

// What the Checkbox (checkbox.go) and the Radio (radio.go) share: a mark
// (the box or the circle) at the leading edge and a one-line title after
// it.

// toggleStates returns the a11y states of a check state (the Checkbox's
// node).
func toggleStates(s driver.CheckState) a11y.States {
	switch s {
	case driver.StateOn:
		return a11y.StateChecked
	case driver.StateMixed:
		return a11y.StateMixed
	}
	return 0
}

// toggleFont returns the font of a toggle's title: the system font size of
// the control size.
func toggleFont(cs driver.ControlSize) driver.Font {
	return driver.Font{Size: tokens.Typography.FontSize.At(cs)}
}

// toggleTitle caches the title layout of a checkbox or a radio.
type toggleTitle struct {
	layout render.TextLayout
	key    buttonTitleKey
}

// get returns text on one line in font, untruncated, or nil for an empty
// text. The layout is kept until the text or the font changes.
func (t *toggleTitle) get(ts render.TextSystem, text string, font driver.Font) render.TextLayout {
	if text == "" {
		return nil
	}
	k := buttonTitleKey{text: text, font: font}
	if t.layout == nil || t.key != k {
		t.layout = ts.NewLayout(oneLine(k.text), k.font, 0)
		t.key = k
	}
	return t.layout
}

// toggleSize returns the fitting size of a mark of side mark followed by
// gap and the title l (nil for none): the widths rounded up to whole
// points, as high as the mark or the title's line, whichever is higher.
func toggleSize(mark, gap float64, l render.TextLayout) driver.Size {
	s := driver.Size{W: mark, H: mark}
	if l != nil {
		ts := l.Size()
		s.W = math.Ceil(mark + gap + ts.W)
		s.H = math.Max(mark, math.Ceil(ts.H))
	}
	return s
}

// paintToggleTitle draws the title l of text at x in the bounds b, centred
// vertically, truncated with an ellipsis to the width left, in
// ColorControlText (ColorDisabledControlText when disabled). Nothing is
// drawn without room or when the line is higher than b.
func paintToggleTitle(c render.Canvas, ts render.TextSystem, l render.TextLayout, text string, font driver.Font,
	x float64, b driver.Rect, disabled bool, a driver.Appearance) {
	avail := b.W - x
	if l == nil || avail <= 0 {
		return
	}
	if l.Size().W > avail {
		l = truncate(ts, text, font, avail)
	}
	if l == nil || l.Size().H > b.H {
		return
	}
	col := driver.ColorControlText
	if disabled {
		col = driver.ColorDisabledControlText
	}
	c.DrawText(l, driver.Point{X: x, Y: (b.H - l.Size().H) / 2}, semantic(col, a))
}
