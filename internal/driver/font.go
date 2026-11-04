package driver

// FontWeight is a font weight on the CSS scale (100 … 900). Zero means
// FontWeightRegular.
type FontWeight int16

// Standard font weights, matching the NSFont.Weight names.
const (
	FontWeightUltraLight FontWeight = 100
	FontWeightThin       FontWeight = 200
	FontWeightLight      FontWeight = 300
	FontWeightRegular    FontWeight = 400
	FontWeightMedium     FontWeight = 500
	FontWeightSemibold   FontWeight = 600
	FontWeightBold       FontWeight = 700
	FontWeightHeavy      FontWeight = 800
	FontWeightBlack      FontWeight = 900
)

// DefaultFontSize is the system font size of regular-size controls on macOS.
const DefaultFontSize = 13

// Font describes a font. Drivers resolve it to a native font object.
//
// The zero Font is the regular system font at the default size for the
// widget's control size.
type Font struct {
	// Family is a font family name. Empty means the system UI font: SF Pro
	// on macOS, Inter on Windows and Linux (PLAN.md §3.1).
	Family string

	// Size in points. Zero means the default size for the control size.
	Size float64

	// Weight. Zero means FontWeightRegular.
	Weight FontWeight

	Italic bool

	// Monospaced selects the system monospaced font when Family is empty.
	// It is ignored when Family is set.
	Monospaced bool
}

// IsSystem reports whether f refers to a system font (Family is empty).
func (f Font) IsSystem() bool { return f.Family == "" }

// EffectiveWeight returns the weight with zero mapped to FontWeightRegular.
func (f Font) EffectiveWeight() FontWeight {
	if f.Weight == 0 {
		return FontWeightRegular
	}
	return f.Weight
}

// EffectiveSize returns the size with zero mapped to the default system
// font size for the control size cs (13, 11, 9 and 13 pt for regular,
// small, mini and large, as in AppKit).
func (f Font) EffectiveSize(cs ControlSize) float64 {
	if f.Size > 0 {
		return f.Size
	}
	switch cs {
	case ControlSizeSmall:
		return 11
	case ControlSizeMini:
		return 9
	default:
		return DefaultFontSize
	}
}
