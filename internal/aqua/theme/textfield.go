package theme

import "github.com/radix29/gogui/internal/driver"

// The colors of a text field that follow the accent and the focus (PLAN.md
// S5.2e). They are computed here, like the accent rules (accent.go), so an
// accent change reaches them at once.

// Caret returns the color of the insertion point: the accent, as macOS 14
// draws it (NSColor.textInsertionPointColor).
func (t *Theme) Caret(a driver.Appearance) driver.Color {
	return a.Accent
}

// TextSelection returns the background of selected text: Selection (the
// accent rule behind ColorSelectedTextBackground) while the text has the
// focus in a key window, otherwise the
// ColorUnemphasizedSelectedTextBackground token.
func (t *Theme) TextSelection(a driver.Appearance, focused, key bool) driver.Color {
	if focused && key {
		return t.Selection(a)
	}
	return t.Resolve(driver.Semantic(driver.ColorUnemphasizedSelectedTextBackground), a)
}
