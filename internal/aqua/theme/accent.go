package theme

import (
	"math"

	"github.com/radix29/gogui/internal/driver"
)

// The accent rules (PLAN.md S3.1d). Every color that depends on the accent
// is computed from Appearance.Accent here, never stored, so the OS accent
// (or the app's override, S6.1g) changes all of them at once. The parameters
// are the Accent tokens; the semantic-color tokens of the accent colors keep
// their default-accent values only for reference (a test checks the rules
// reproduce them with driver.DefaultAccent).
//
//   - Accent fills (ColorControlAccent, ColorSelectedContentBackground,
//     ColorSystemBlue; a default button, a checked checkbox, a slider's
//     track): the accent itself.
//   - FocusRing: the accent with its alpha scaled by FocusRingAlpha; with
//     IncreaseContrast the accent itself, at full strength (S3.2d,
//     provisional like the tokens).
//   - Selection (ColorSelectedTextBackground, ColorSelectedControl): the
//     accent at SelectionAlpha over the opaque Base, so it is opaque; with
//     IncreaseContrast at the stronger SelectionContrastAlpha (S6.1e). The
//     unemphasized selection is a token and keeps its value.
//   - AccentPressed (a pressed accent fill): the accent's channels scaled by
//     PressedShade.
//   - AccentInactive (an accent fill in an inactive window, which macOS
//     greys out): the luminance of driver.DefaultAccent as a grey, at
//     InactiveAlpha over Base. It does not follow Appearance.Accent: in an
//     inactive window no color derives from the accent (S6.1a), so every
//     accent greys to the same fill. A default button in an inactive window
//     is drawn as a plain push button instead (S3.3).
//   - AccentText (the label on an accent fill): the DefaultButtonText token,
//     or AccentDarkText on a light accent, where DefaultButtonText has a
//     WCAG contrast ratio below AccentTextMinContrast (S6.1g).

// FocusRing returns the keyboard focus ring color for a: the accent at
// FocusRingAlpha, or the accent itself with IncreaseContrast.
func (t *Theme) FocusRing(a driver.Appearance) driver.Color {
	f := a.Accent
	if !a.IncreaseContrast {
		f.A *= t.Accent.FocusRingAlpha
	}
	return f
}

// Selection returns the selection background (text and controls) for a.
func (t *Theme) Selection(a driver.Appearance) driver.Color {
	k := t.Accent.SelectionAlpha
	if a.IncreaseContrast {
		k = t.Accent.SelectionContrastAlpha
	}
	return over(a.Accent, k, t.Accent.Base.Value(a))
}

// AccentPressed returns the fill of a pressed accent-colored control.
func (t *Theme) AccentPressed(a driver.Appearance) driver.Color {
	c, f := a.Accent, t.Accent.PressedShade
	return driver.Color{R: c.R * f, G: c.G * f, B: c.B * f, A: c.A}
}

// AccentInactive returns the fill of an accent-colored control in an
// inactive window, the same for every accent.
func (t *Theme) AccentInactive(a driver.Appearance) driver.Color {
	c := driver.DefaultAccent
	y := luminance(c)
	return over(driver.Color{R: y, G: y, B: y, A: c.A}, t.Accent.InactiveAlpha, t.Accent.Base.Value(a))
}

// AccentText returns the color of text and glyphs on an accent fill: the
// DefaultButtonText token, or AccentDarkText when DefaultButtonText has a
// contrast ratio below AccentTextMinContrast on a.Accent (a light accent,
// S6.1g). The rule looks at the accent only, so a pressed fill keeps the
// label color of the unpressed one. Accent fills in an inactive window are
// grey and take ToggleMark's DefaultButtonText instead.
func (t *Theme) AccentText(a driver.Appearance) driver.Color {
	txt := t.Accent.DefaultButtonText.Value(a)
	if ContrastRatio(txt, a.Accent) < float64(t.Accent.AccentTextMinContrast) {
		return t.Accent.AccentDarkText.Value(a)
	}
	return txt
}

// ContrastRatio returns the WCAG 2 contrast ratio of the colors a and b,
// from 1 to 21, by their relative luminance. Alpha is ignored: both are
// taken as opaque.
func ContrastRatio(a, b driver.Color) float64 {
	la, lb := RelativeLuminance(a), RelativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// RelativeLuminance returns the WCAG 2 relative luminance of c's sRGB
// channels, linearised (0 for black, 1 for white).
func RelativeLuminance(c driver.Color) float64 {
	lin := func(v float32) float64 {
		x := math.Min(1, math.Max(0, float64(v)))
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// luminance returns the Rec. 709 luma of c's sRGB channels (no
// linearisation: a grey that looks about as light as c).
func luminance(c driver.Color) float32 {
	return 0.2126*c.R + 0.7152*c.G + 0.0722*c.B
}

// over composites c, with its alpha scaled by alpha, over the backdrop b
// and returns an opaque color when b is opaque (source-over, straight
// alpha).
func over(c driver.Color, alpha float32, b driver.Color) driver.Color {
	sa := c.A * alpha
	oa := sa + b.A*(1-sa)
	if oa == 0 {
		return driver.Color{}
	}
	mix := func(s, d float32) float32 { return (s*sa + d*b.A*(1-sa)) / oa }
	return driver.Color{R: mix(c.R, b.R), G: mix(c.G, b.G), B: mix(c.B, b.B), A: oa}
}
