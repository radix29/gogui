package gogui

import "github.com/radix29/gogui/internal/driver"

// Color is a concrete sRGB color or a semantic color. Semantic colors
// (ColorLabel, ColorControlAccent …) mirror NSColor: they resolve against
// the current appearance (light or dark, accent color, increased contrast)
// every time they are drawn, so an app that uses them follows appearance
// changes without doing anything.
//
// A concrete color has R, G, B and A, its sRGB components with straight
// (not premultiplied) alpha, each in 0..1; RGBA and RGB8 make one. A
// semantic color has its Semantic field set, which only the ColorX
// variables do, and then R, G, B and A are ignored; IsSemantic reports
// which kind c is. The zero Color is transparent black; the setters that
// take a color document what zero means for them (SetTextColor: the
// default text color).
//
// On the headless driver (macOS until the AppKit driver exists) nothing is
// drawn, so semantic colors are never resolved.
type Color = driver.Color

// RGBA returns a concrete sRGB color with straight alpha; each component
// is in 0..1.
func RGBA(r, g, b, a float32) Color { return driver.RGBA(r, g, b, a) }

// RGB8 returns an opaque sRGB color from 8-bit components.
func RGB8(r, g, b uint8) Color { return driver.RGB8(r, g, b) }

// Semantic colors. ColorX corresponds to NSColor.xColor (ColorLabel is
// labelColor, ColorSystemBlue is systemBlueColor …).
var (
	// Label colors.
	ColorLabel           = driver.Semantic(driver.ColorLabel)
	ColorSecondaryLabel  = driver.Semantic(driver.ColorSecondaryLabel)
	ColorTertiaryLabel   = driver.Semantic(driver.ColorTertiaryLabel)
	ColorQuaternaryLabel = driver.Semantic(driver.ColorQuaternaryLabel)

	// Text colors.
	ColorText                               = driver.Semantic(driver.ColorText)
	ColorPlaceholderText                    = driver.Semantic(driver.ColorPlaceholderText)
	ColorSelectedText                       = driver.Semantic(driver.ColorSelectedText)
	ColorTextBackground                     = driver.Semantic(driver.ColorTextBackground)
	ColorSelectedTextBackground             = driver.Semantic(driver.ColorSelectedTextBackground)
	ColorUnemphasizedSelectedTextBackground = driver.Semantic(driver.ColorUnemphasizedSelectedTextBackground)

	// Content and window colors.
	ColorWindowBackground                      = driver.Semantic(driver.ColorWindowBackground)
	ColorUnderPageBackground                   = driver.Semantic(driver.ColorUnderPageBackground)
	ColorControlBackground                     = driver.Semantic(driver.ColorControlBackground)
	ColorSelectedContentBackground             = driver.Semantic(driver.ColorSelectedContentBackground)
	ColorUnemphasizedSelectedContentBackground = driver.Semantic(driver.ColorUnemphasizedSelectedContentBackground)
	ColorSeparator                             = driver.Semantic(driver.ColorSeparator)
	ColorGrid                                  = driver.Semantic(driver.ColorGrid)
	ColorLink                                  = driver.Semantic(driver.ColorLink)
	ColorFindHighlight                         = driver.Semantic(driver.ColorFindHighlight)

	// Control colors.
	ColorControl                = driver.Semantic(driver.ColorControl)
	ColorControlText            = driver.Semantic(driver.ColorControlText)
	ColorDisabledControlText    = driver.Semantic(driver.ColorDisabledControlText)
	ColorSelectedControl        = driver.Semantic(driver.ColorSelectedControl)
	ColorSelectedControlText    = driver.Semantic(driver.ColorSelectedControlText)
	ColorControlAccent          = driver.Semantic(driver.ColorControlAccent)
	ColorKeyboardFocusIndicator = driver.Semantic(driver.ColorKeyboardFocusIndicator)

	// Adaptable system colors.
	ColorSystemRed    = driver.Semantic(driver.ColorSystemRed)
	ColorSystemOrange = driver.Semantic(driver.ColorSystemOrange)
	ColorSystemYellow = driver.Semantic(driver.ColorSystemYellow)
	ColorSystemGreen  = driver.Semantic(driver.ColorSystemGreen)
	ColorSystemMint   = driver.Semantic(driver.ColorSystemMint)
	ColorSystemTeal   = driver.Semantic(driver.ColorSystemTeal)
	ColorSystemCyan   = driver.Semantic(driver.ColorSystemCyan)
	ColorSystemBlue   = driver.Semantic(driver.ColorSystemBlue)
	ColorSystemIndigo = driver.Semantic(driver.ColorSystemIndigo)
	ColorSystemPurple = driver.Semantic(driver.ColorSystemPurple)
	ColorSystemPink   = driver.Semantic(driver.ColorSystemPink)
	ColorSystemBrown  = driver.Semantic(driver.ColorSystemBrown)
	ColorSystemGray   = driver.Semantic(driver.ColorSystemGray)
)
