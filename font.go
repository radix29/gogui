package gogui

import "github.com/radix29/gogui/internal/driver"

// Font describes a font. The zero Font is the regular system font at the
// default size for the widget's control size (13 pt for regular controls).
// An empty Family means the system UI font: SF Pro on macOS, Inter on
// Windows and Linux.
type Font = driver.Font

// FontWeight is a font weight on the CSS scale (100 … 900). Zero means
// FontWeightRegular.
type FontWeight = driver.FontWeight

// Font weights, matching the NSFont.Weight names.
const (
	FontWeightUltraLight = driver.FontWeightUltraLight
	FontWeightThin       = driver.FontWeightThin
	FontWeightLight      = driver.FontWeightLight
	FontWeightRegular    = driver.FontWeightRegular
	FontWeightMedium     = driver.FontWeightMedium
	FontWeightSemibold   = driver.FontWeightSemibold
	FontWeightBold       = driver.FontWeightBold
	FontWeightHeavy      = driver.FontWeightHeavy
	FontWeightBlack      = driver.FontWeightBlack
)

// SystemFont returns the regular system font at size points
// (NSFont.systemFontOfSize:). A size of 0 or less means the default size
// for the widget's control size.
func SystemFont(size float64) Font { return Font{Size: fontSize(size)} }

// BoldSystemFont returns the bold system font at size points
// (NSFont.boldSystemFontOfSize:). A size of 0 or less means the default
// size for the widget's control size.
func BoldSystemFont(size float64) Font {
	return Font{Size: fontSize(size), Weight: FontWeightBold}
}

// fontSize maps sizes that are not positive (including NaN) to 0, the
// default size.
func fontSize(size float64) float64 {
	if !(size > 0) {
		return 0
	}
	return size
}
