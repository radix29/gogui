package gogui

import (
	"strconv"

	"github.com/radix29/gogui/internal/driver"
)

// Appearance is the light or dark appearance of the app.
type Appearance uint8

const (
	// AppearanceSystem follows the OS setting. It is the default.
	AppearanceSystem Appearance = iota
	// AppearanceLight forces the light appearance (NSAppearanceNameAqua).
	AppearanceLight
	// AppearanceDark forces the dark appearance (NSAppearanceNameDarkAqua).
	AppearanceDark
)

// Toggled returns AppearanceDark for AppearanceLight and AppearanceLight
// for AppearanceDark; any other value is returned unchanged. App.Appearance
// never returns AppearanceSystem, so
// app.SetAppearance(app.Appearance().Toggled()) always switches.
func (a Appearance) Toggled() Appearance {
	switch a {
	case AppearanceLight:
		return AppearanceDark
	case AppearanceDark:
		return AppearanceLight
	}
	return a
}

// String returns "system", "light" or "dark".
func (a Appearance) String() string {
	switch a {
	case AppearanceSystem:
		return "system"
	case AppearanceLight:
		return "light"
	case AppearanceDark:
		return "dark"
	}
	return "Appearance(" + strconv.Itoa(int(a)) + ")"
}

// mode returns the driver's appearance mode for a. Unknown values map to
// the system appearance.
func (a Appearance) mode() driver.AppearanceMode {
	switch a {
	case AppearanceLight:
		return driver.AppearanceLight
	case AppearanceDark:
		return driver.AppearanceDark
	}
	return driver.AppearanceSystem
}

// effectiveAppearance maps the driver's effective appearance to
// AppearanceLight or AppearanceDark, never AppearanceSystem.
func effectiveAppearance(ap driver.Appearance) Appearance {
	if ap.Dark {
		return AppearanceDark
	}
	return AppearanceLight
}

// ControlSize mirrors NSControl.ControlSize. The zero value is
// ControlSizeRegular.
type ControlSize = driver.ControlSize

// Control sizes.
const (
	ControlSizeRegular = driver.ControlSizeRegular
	ControlSizeSmall   = driver.ControlSizeSmall
	ControlSizeMini    = driver.ControlSizeMini
	ControlSizeLarge   = driver.ControlSizeLarge
)

// TextAlignment is the horizontal alignment of text. Leading and trailing
// follow the layout direction. The zero value is AlignLeading.
type TextAlignment = driver.TextAlignment

// Text alignments.
const (
	AlignLeading   = driver.AlignLeading
	AlignCenter    = driver.AlignCenter
	AlignTrailing  = driver.AlignTrailing
	AlignJustified = driver.AlignJustified
)
