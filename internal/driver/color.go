package driver

// Color is either a concrete sRGB color or a semantic color that the driver
// resolves against the current appearance (light/dark, accent, increased
// contrast) every time it draws.
//
// When Semantic is not SemanticNone, R, G, B and A are ignored. The zero
// Color is transparent black.
type Color struct {
	Semantic SemanticColor

	// R, G, B and A are sRGB components with straight (not premultiplied)
	// alpha, each in 0..1.
	R, G, B, A float32
}

// RGBA returns a concrete sRGB color.
func RGBA(r, g, b, a float32) Color { return Color{R: r, G: g, B: b, A: a} }

// RGB8 returns an opaque sRGB color from 8-bit components.
func RGB8(r, g, b uint8) Color {
	return Color{R: float32(r) / 255, G: float32(g) / 255, B: float32(b) / 255, A: 1}
}

// Semantic returns the semantic color s.
func Semantic(s SemanticColor) Color { return Color{Semantic: s} }

// IsSemantic reports whether c must be resolved by the driver.
func (c Color) IsSemantic() bool { return c.Semantic != SemanticNone }

// SemanticColor names a color that depends on the appearance. The names
// mirror NSColor's UI element and adaptable system colors: ColorX
// corresponds to NSColor.xColor (ColorLabel is labelColor, ColorSystemBlue is
// systemBlueColor, …).
type SemanticColor uint16

// Semantic colors.
const (
	SemanticNone SemanticColor = iota

	// Label colors.
	ColorLabel
	ColorSecondaryLabel
	ColorTertiaryLabel
	ColorQuaternaryLabel

	// Text colors.
	ColorText
	ColorPlaceholderText
	ColorSelectedText
	ColorTextBackground
	ColorSelectedTextBackground
	ColorUnemphasizedSelectedTextBackground

	// Content and window colors.
	ColorWindowBackground
	ColorUnderPageBackground
	ColorControlBackground
	ColorSelectedContentBackground
	ColorUnemphasizedSelectedContentBackground
	ColorSeparator
	ColorGrid
	ColorLink
	ColorFindHighlight

	// Control colors.
	ColorControl
	ColorControlText
	ColorDisabledControlText
	ColorSelectedControl
	ColorSelectedControlText
	ColorControlAccent
	ColorKeyboardFocusIndicator

	// Adaptable system colors.
	ColorSystemRed
	ColorSystemOrange
	ColorSystemYellow
	ColorSystemGreen
	ColorSystemMint
	ColorSystemTeal
	ColorSystemCyan
	ColorSystemBlue
	ColorSystemIndigo
	ColorSystemPurple
	ColorSystemPink
	ColorSystemBrown
	ColorSystemGray

	semanticColorCount // keep last
)

// NumSemanticColors is the number of semantic colors, SemanticNone
// excluded. Drivers use it to size lookup tables.
const NumSemanticColors = int(semanticColorCount) - 1

// Valid reports whether s is SemanticNone or a known semantic color.
func (s SemanticColor) Valid() bool { return s < semanticColorCount }
