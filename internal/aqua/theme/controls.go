package theme

import "github.com/radix29/gogui/internal/driver"

// The fills of the drawn controls (PLAN.md S3.3b, S3.4a). The
// accent-colored ones come from the accent rules (accent.go), so they
// follow Appearance.Accent; the others are Controls tokens.

// ButtonFill returns the bezel fill of a push button. A default button in
// a key window is filled with the accent (AccentPressed while pressed); in
// an inactive window a default button is drawn as a plain push button, as
// on macOS. A plain button uses BezelFill, or BezelPressedFill while
// pressed. A disabled default button is drawn as a plain one: pass
// isDefault false for it.
func (t *Theme) ButtonFill(a driver.Appearance, isDefault, pressed, key bool) driver.Color {
	switch {
	case isDefault && key && pressed:
		return t.AccentPressed(a)
	case isDefault && key:
		return a.Accent
	case pressed:
		return t.Controls.BezelPressedFill.Value(a)
	}
	return t.Controls.BezelFill.Value(a)
}

// ButtonText returns the title color of a push button: AccentText on an
// accent fill (a default button in a key window), otherwise
// ColorControlText, or ColorDisabledControlText when disabled. A disabled
// default button is drawn as a plain one.
func (t *Theme) ButtonText(a driver.Appearance, isDefault, key, disabled bool) driver.Color {
	switch {
	case disabled:
		return t.Resolve(driver.Semantic(driver.ColorDisabledControlText), a)
	case isDefault && key:
		return t.AccentText(a)
	}
	return t.Resolve(driver.Semantic(driver.ColorControlText), a)
}

// ToggleFill returns the fill of a checkbox's box or a radio's circle. On
// (or mixed): the accent, AccentPressed while pressed and AccentInactive in
// an inactive window. Off: BezelFill, or BezelPressedFill while pressed.
func (t *Theme) ToggleFill(a driver.Appearance, on, pressed, key bool) driver.Color {
	switch {
	case on && !key:
		return t.AccentInactive(a)
	case on && pressed:
		return t.AccentPressed(a)
	case on:
		return a.Accent
	case pressed:
		return t.Controls.BezelPressedFill.Value(a)
	}
	return t.Controls.BezelFill.Value(a)
}

// ToggleMark returns the color of a checked checkbox's mark and a selected
// radio's dot: AccentText on the accent fill of a key window, the
// DefaultButtonText token on the grey fill of an inactive one, which does
// not follow the accent (S6.1a, S6.1g).
func (t *Theme) ToggleMark(a driver.Appearance, key bool) driver.Color {
	if !key {
		return t.Accent.DefaultButtonText.Value(a)
	}
	return t.AccentText(a)
}

// ValueFill returns the fill of the filled part of a slider's track (from
// its minimum end to the knob) and of a determinate progress bar's track,
// and of an indeterminate progress bar's band: the accent in a key window,
// AccentInactive in an inactive one. Disabled, that color's alpha is
// scaled by ValueDisabledAlpha (the control draws its other parts with the
// same opacity). Unlike ToggleFill it has no pressed shade: dragging a
// slider's knob does not change the track.
func (t *Theme) ValueFill(a driver.Appearance, key, disabled bool) driver.Color {
	c := a.Accent
	if !key {
		c = t.AccentInactive(a)
	}
	if disabled {
		c.A *= t.Controls.ValueDisabledAlpha
	}
	return c
}

// BoxContentInsets returns the insets of a Box's content from the edges of
// its group (the title above the group is not included).
func (c *Controls) BoxContentInsets() driver.Insets {
	return driver.Insets{
		Top: c.BoxContentInsetY, Bottom: c.BoxContentInsetY,
		Left: c.BoxContentInsetX, Right: c.BoxContentInsetX,
	}
}
