package theme

import "github.com/radix29/gogui/internal/driver"

// MenuHighlight returns the fill of a menu's highlighted item (PLAN.md
// S5.5h): the accent, as macOS draws it (selectedContentBackgroundColor in
// a key window; a menu is only open in one). Like the accent rules
// (accent.go) it is computed, not stored, so an accent change reaches it at
// once.
func (t *Theme) MenuHighlight(a driver.Appearance) driver.Color {
	return a.Accent
}

// MenuHighlightedText returns the color of the highlighted item's title,
// checkmark and shortcut on MenuHighlight: the HighlightedText token, or
// AccentDarkText on a light accent, by the AccentText rule (S6.1g).
func (t *Theme) MenuHighlightedText(a driver.Appearance) driver.Color {
	if t.AccentText(a) != t.Accent.DefaultButtonText.Value(a) {
		return t.Accent.AccentDarkText.Value(a)
	}
	return t.Menu.HighlightedText.Value(a)
}
