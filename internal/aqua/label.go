package aqua

import (
	"math"
	"strings"
	"unicode/utf8"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/render"
)

// Label is the aqua driver.Label (PLAN.md S3.3c): a real element that
// draws its text through the shell's TextSystem. The embedded stub label
// keeps the state (text, font, color, alignment, wrapping, control size)
// and the User… simulations; FittingSize is the laid-out text, no longer
// the stub's metrics.
//
//   - Font: the peer's Font, a zero Size taking the theme's system size for
//     the control size (Typography.FontSize).
//   - Color: the peer's TextColor, ColorLabel when zero, resolved for the
//     window's appearance; ColorDisabledControlText when the label is
//     disabled in the tree, as an NSTextField draws.
//   - Wrapping: with Wraps the text wraps at the frame width (FittingSize
//     at max.W); without it, lines break only at '\n'. Text that does not
//     fit the frame is clipped to it (AppKit's labelWithString: clips).
//   - Position: the text starts at the top of the frame; each line is
//     aligned on its own (leading, centre, trailing). AlignJustified is
//     drawn leading: the TextSystem has no justification.
//   - Accessibility: a static text with the label's text.
//
// Selectable is recorded but has no effect yet (selection belongs to the
// text controls); the theme's line height and tracking are not applied, as
// the TextSystem lays out with the font's own metrics.
type Label struct {
	*stub.Label
	peerBase
	view *label // the element (peerBase.el is nil)
}

// NewLabel returns a label.
func (d *Driver) NewLabel() driver.Label {
	l := &Label{Label: stub.NewLabel()}
	l.view = &label{d: d, p: l}
	l.view.Init(l.view)
	l.initElement(l.Label, &l.Label.Base, l.view)
	return l
}

// SetText implements driver.Label; the setters below repaint as well.
func (l *Label) SetText(text string)                 { l.Label.SetText(text); l.redraw() }
func (l *Label) SetFont(f driver.Font)               { l.Label.SetFont(f); l.redraw() }
func (l *Label) SetTextColor(c driver.Color)         { l.Label.SetTextColor(c); l.redraw() }
func (l *Label) SetAlignment(a driver.TextAlignment) { l.Label.SetAlignment(a); l.redraw() }
func (l *Label) SetWraps(wraps bool)                 { l.Label.SetWraps(wraps); l.redraw() }
func (l *Label) SetControlSize(cs driver.ControlSize) {
	l.Label.SetControlSize(cs)
	l.redraw()
}

// FittingSize implements driver.Widget: the size of the laid-out text
// (wrapped at max.W when the label wraps), clamped to max. A stub Fixed
// size still wins, as for every peer.
func (l *Label) FittingSize(max driver.Size) driver.Size {
	if !l.Fixed.IsZero() {
		return l.Label.FittingSize(max)
	}
	return l.view.layout(max.W).Size().Min(max)
}

// TextFont returns the font the label draws with: its Font with a zero
// Size replaced by the theme's system size for its control size.
func (l *Label) TextFont() driver.Font {
	f := l.Font
	if f.Size <= 0 {
		f.Size = tokens.Typography.FontSize.At(l.ControlSize)
	}
	return f
}

// label is the element of a Label.
type label struct {
	Base
	d *Driver
	p *Label

	// The last layout and what it was made from.
	cache    render.TextLayout
	cacheKey labelKey
}

type labelKey struct {
	text  string
	font  driver.Font
	width float64 // the wrap width, 0 for none
}

// layout returns the laid-out text, wrapped at width when the label wraps
// (a width that is not positive and finite means no wrapping).
func (e *label) layout(width float64) render.TextLayout {
	k := labelKey{text: e.p.Text, font: e.p.TextFont()}
	if e.p.Wraps && width > 0 && !math.IsInf(width, 1) {
		k.width = width
	}
	if e.cache == nil || e.cacheKey != k {
		e.cache, e.cacheKey = e.d.sh.TextSystem().NewLayout(k.text, k.font, k.width), k
	}
	return e.cache
}

// FittingSize returns the peer's FittingSize.
func (e *label) FittingSize(max driver.Size) driver.Size { return e.p.FittingSize(max) }

// color returns the concrete text color.
func (e *label) color() driver.Color {
	a := appearanceIn(&e.Base, e.d)
	if !e.EnabledInTree() {
		return semantic(driver.ColorDisabledControlText, a)
	}
	c := e.p.TextColor
	if c == (driver.Color{}) {
		c = driver.Semantic(driver.ColorLabel)
	}
	return resolveColor(c, a)
}

// Paint draws the text from the top of the frame, clipped to it when it
// does not fit (nothing for an empty frame or text).
func (e *label) Paint(c render.Canvas) {
	b := e.Bounds()
	if b.Empty() || e.p.Text == "" {
		return
	}
	l := e.layout(b.W)
	if s := l.Size(); s.W > b.W || s.H > b.H {
		c.ClipRoundedRect(b, render.Radii{})
	}
	col := e.color()
	align := e.p.Alignment
	if align == driver.AlignLeading || align == driver.AlignJustified {
		c.DrawText(l, driver.Point{}, col)
		return
	}
	f := 1.0 // AlignTrailing
	if align == driver.AlignCenter {
		f = 0.5
	}
	if l.LineCount() == 1 {
		c.DrawText(l, driver.Point{X: max(0, (b.W-l.Size().W)*f)}, col)
		return
	}
	// One layout per line, each aligned on its own.
	ts := e.d.sh.TextSystem()
	for _, ln := range lines(l) {
		if ln.text == "" {
			continue
		}
		ll := ts.NewLayout(ln.text, l.Font(), 0)
		c.DrawText(ll, driver.Point{X: max(0, (b.W-ll.Size().W)*f), Y: ln.y}, col)
	}
}

// AccessibleNode implements Element: a static text with the label's text.
func (e *label) AccessibleNode() a11y.Node {
	return a11y.Node{Role: a11y.RoleStaticText, Label: e.p.Text}
}

// textLine is one visual line of a layout: its text without the line
// break and the trailing spaces of a wrap, and its top.
type textLine struct {
	text string
	y    float64
}

// lines splits a layout into its visual lines: a line starts where the
// caret moves down (the index of a line's start is drawn on that line).
func lines(l render.TextLayout) []textLine {
	text := l.Text()
	var out []textLine
	start, y := 0, l.CaretRect(0).Y
	flush := func(end int) {
		s := strings.TrimRight(text[start:end], "\r\n")
		out = append(out, textLine{text: trimRight(s), y: y})
	}
	for i := 0; i < len(text); {
		_, n := utf8.DecodeRuneInString(text[i:])
		i += n
		if i == len(text) {
			break
		}
		if ny := l.CaretRect(i).Y; ny > y {
			flush(i)
			start, y = i, ny
		}
	}
	flush(len(text))
	return out
}
