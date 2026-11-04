package stub

import (
	"math"
	"unicode/utf8"

	"github.com/radix29/gogui/internal/driver"
)

// Fake metrics used by FittingSize. They are loosely based on AppKit's
// regular control sizes, but only their stability matters: tests of the
// layout and of the public layer depend on them.
//
//   - Text: each rune is 7 pt wide and a line is 16 pt high at 13 pt; both
//     scale with the font size (driver.Font.EffectiveSize).
//   - Control height: 22 (regular), 19 (small), 16 (mini), 28 (large).
//   - Button: text + 28. Checkbox and radio: 18 + text, 18 high.
//   - Text field, slider and progress bar: 96 wide. Spinner: 32 × 32.
//   - Pop-up button: widest item + 32.
//   - Separator: 1 × 1. Box: its content insets (8, plus 16 at the top
//     when titled).
//   - Container, scroll view and text view: zero (they are sized by the
//     layout).
const (
	charWidth13    = 7.0
	lineHeight13   = 16.0
	buttonPadding  = 28.0
	toggleGlyph    = 18.0
	defaultWidth   = 96.0
	spinnerSize    = 32.0
	popUpPadding   = 32.0
	boxInset       = 8.0
	boxTitleHeight = 16.0
)

func fontScale(f driver.Font, cs driver.ControlSize) float64 {
	return f.EffectiveSize(cs) / driver.DefaultFontSize
}

// TextWidth returns the fake width of s in font f.
func TextWidth(s string, f driver.Font, cs driver.ControlSize) float64 {
	return float64(utf8.RuneCountInString(s)) * charWidth13 * fontScale(f, cs)
}

// LineHeight returns the fake line height of font f.
func LineHeight(f driver.Font, cs driver.ControlSize) float64 {
	return lineHeight13 * fontScale(f, cs)
}

// ControlHeight returns the fake height of a bezeled control.
func ControlHeight(cs driver.ControlSize) float64 {
	switch cs {
	case driver.ControlSizeSmall:
		return 19
	case driver.ControlSizeMini:
		return 16
	case driver.ControlSizeLarge:
		return 28
	default:
		return 22
	}
}

// wrappedText returns the fake size of text wrapped at maxW.
func wrappedText(s string, f driver.Font, cs driver.ControlSize, maxW float64) driver.Size {
	w, lh := TextWidth(s, f, cs), LineHeight(f, cs)
	if w == 0 {
		return driver.Size{H: lh}
	}
	if math.IsInf(maxW, 1) || maxW <= 0 || w <= maxW {
		return driver.Size{W: w, H: lh}
	}
	lines := math.Ceil(w / maxW)
	return driver.Size{W: maxW, H: lines * lh}
}
