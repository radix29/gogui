package aqua

import (
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
)

// Drawing primitives the controls share (review R4): the one-line title
// cache, the inside border, the disabled fade of the value controls, the
// margin of a panel's own shadow, the pixel-aligned hairline and the system
// font (review R5).

// layoutMemo caches a one-line, untruncated text layout: a control's
// title, the pop-up button's item, a field's placeholder.
type layoutMemo struct {
	layout render.TextLayout
	key    layoutMemoKey
}

type layoutMemoKey struct {
	text string
	font driver.Font
}

// get returns text on one line (oneLine) in font, untruncated, or nil for
// an empty text. The layout is kept until the text or the font changes.
func (m *layoutMemo) get(ts render.TextSystem, text string, font driver.Font) render.TextLayout {
	if text == "" {
		return nil
	}
	k := layoutMemoKey{text: text, font: font}
	if m.layout == nil || m.key != k {
		m.layout = ts.NewLayout(oneLine(k.text), k.font, 0)
		m.key = k
	}
	return m.layout
}

// strokeInside strokes a border bw wide (in points, already whole device
// pixels: pixelWidth) inside the rounded rectangle r of corner radius
// radius, so the border's outer edge is r's. Nothing is drawn when r is
// not wider and higher than two borders.
func strokeInside(c render.Canvas, r driver.Rect, radius, bw float64, col driver.Color) {
	if r.W <= 2*bw || r.H <= 2*bw {
		return
	}
	c.StrokeRoundedRect(r.Inset(driver.UniformInsets(bw/2)), render.UniformRadii(max(0, radius-bw/2)), bw, render.Solid(col))
}

// dimmed returns col at ValueDisabledAlpha when disabled (the slider and
// the progress indicator).
func dimmed(col driver.Color, disabled bool) driver.Color {
	if disabled {
		col.A *= tokens.Controls.ValueDisabledAlpha
	}
	return col
}

// shadowInsets returns the space around a panel its own shadow of blur
// and vertical offset dy needs: the blur on every side, less the offset
// above and more below (the menu, the alert, the tooltip).
func shadowInsets(blur, dy float64) driver.Insets {
	return driver.Insets{Top: max(0, blur-dy), Left: blur, Bottom: blur + dy, Right: blur}
}

// hairline returns a line thickness points thick (rounded to whole device
// pixels, at least one) centred across r, horizontal or vertical, in e's
// coordinates: its leading edges pixel-aligned (pixelAlign) and its
// thickness exactly the whole pixels, so it stays sharp at every scale.
func hairline(e *Base, r driver.Rect, thickness, scale float64, vertical bool) driver.Rect {
	t := pixelWidth(thickness, scale)
	if vertical {
		l := pixelAlign(e, driver.Rect{X: r.X + (r.W-t)/2, Y: r.Y, H: r.H}, scale)
		l.W = t
		return l
	}
	l := pixelAlign(e, driver.Rect{X: r.X, Y: r.Y + (r.H-t)/2, W: r.W}, scale)
	l.H = t
	return l
}

// systemFont returns the system font at the theme's size for the control
// size cs (Typography.FontSize), in the regular weight.
func systemFont(cs driver.ControlSize) driver.Font {
	return driver.Font{Size: tokens.Typography.FontSize.At(cs)}
}

// sizedFont returns f with a zero (or negative) Size replaced by the
// system size for cs: the font of a Label, a TextField or a TextView.
func sizedFont(f driver.Font, cs driver.ControlSize) driver.Font {
	if f.Size <= 0 {
		f.Size = systemFont(cs).Size
	}
	return f
}
