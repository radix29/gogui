package theme

import "github.com/radix29/gogui/internal/driver"

// PopUpIndicatorFill returns the fill of a pop-up button's indicator
// (PLAN.md S5.5j) and whether it has one: the accent in a key window
// (AccentPressed while pressed), as macOS 11+ draws it. In an inactive
// window and while disabled the indicator has no fill and its chevrons are
// drawn in the title's color. Like the accent rules (accent.go) it is
// computed, not stored.
func (t *Theme) PopUpIndicatorFill(a driver.Appearance, pressed, key, disabled bool) (driver.Color, bool) {
	switch {
	case disabled || !key:
		return driver.Color{}, false
	case pressed:
		return t.AccentPressed(a), true
	}
	return a.Accent, true
}

// PopUpChevron returns the color of a pop-up button's chevrons: AccentText
// on the accent-filled indicator, otherwise the title's color
// (ColorControlText, ColorDisabledControlText when disabled).
func (t *Theme) PopUpChevron(a driver.Appearance, key, disabled bool) driver.Color {
	if _, filled := t.PopUpIndicatorFill(a, false, key, disabled); filled {
		return t.AccentText(a)
	}
	return t.PopUpText(a, disabled)
}

// PopUpText returns the color of a pop-up button's title: ColorControlText,
// or ColorDisabledControlText when disabled (the bezel is never filled with
// the accent).
func (t *Theme) PopUpText(a driver.Appearance, disabled bool) driver.Color {
	if disabled {
		return t.Resolve(driver.Semantic(driver.ColorDisabledControlText), a)
	}
	return t.Resolve(driver.Semantic(driver.ColorControlText), a)
}
