package aqua

import (
	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/driver"
)

// The colors of window chrome and the control elements come from the theme
// tokens (internal/aqua/theme, PLAN.md S3.1b): a semantic color must never
// reach a canvas (the recorder rejects it), so every element resolves it
// through tokens first.

// tokens is the token set the driver draws with.
var tokens = &theme.Sequoia

// resolveColor returns c as a concrete color for the appearance a
// (theme.Theme.Resolve on tokens).
func resolveColor(c driver.Color, a driver.Appearance) driver.Color {
	return tokens.Resolve(c, a)
}

// semantic resolves the semantic color s for a.
func semantic(s driver.SemanticColor, a driver.Appearance) driver.Color {
	return resolveColor(driver.Semantic(s), a)
}

// titleBarColor is the title-bar fill of a key or an inactive window.
func titleBarColor(a driver.Appearance, key bool) driver.Color {
	if key {
		return tokens.Chrome.TitleBar.Value(a)
	}
	return tokens.Chrome.TitleBarInactive.Value(a)
}

// lightColors returns the fill, border and glyph color of an active traffic
// light of kind k.
func lightColors(k lightKind, a driver.Appearance) (fill, border, glyph driver.Color) {
	ch := &tokens.Chrome
	switch k {
	case lightClose:
		return ch.CloseFill.Value(a), ch.CloseBorder.Value(a), ch.CloseGlyph.Value(a)
	case lightMinimize:
		return ch.MinimizeFill.Value(a), ch.MinimizeBorder.Value(a), ch.MinimizeGlyph.Value(a)
	}
	return ch.ZoomFill.Value(a), ch.ZoomBorder.Value(a), ch.ZoomGlyph.Value(a)
}

// inactiveLightColors returns the fill and border of a grey traffic light:
// in an inactive window, or disabled because its style flag is missing.
func inactiveLightColors(a driver.Appearance) (fill, border driver.Color) {
	return tokens.Chrome.LightInactiveFill.Value(a), tokens.Chrome.LightInactiveBorder.Value(a)
}

// darken scales the color channels of c by f (pressed traffic lights).
func darken(c driver.Color, f float32) driver.Color {
	return driver.Color{R: c.R * f, G: c.G * f, B: c.B * f, A: c.A}
}
