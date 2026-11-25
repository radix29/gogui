package driver

import "time"

// AppearanceMode selects the appearance of the app.
type AppearanceMode uint8

const (
	// AppearanceSystem follows the OS setting. It is the default.
	AppearanceSystem AppearanceMode = iota
	// AppearanceLight forces the light appearance (NSAppearanceNameAqua).
	AppearanceLight
	// AppearanceDark forces the dark appearance (NSAppearanceNameDarkAqua).
	AppearanceDark
)

// Appearance is the effective appearance that semantic colors and drawn
// controls resolve against.
type Appearance struct {
	Dark bool

	// Accent is the concrete control accent color. It is never semantic.
	Accent Color

	IncreaseContrast   bool
	ReduceMotion       bool
	ReduceTransparency bool

	// LegacyScrollers is true when the OS asks for always-visible scroll
	// bars instead of overlay scrollers (PLAN.md §5.6).
	LegacyScrollers bool

	// CaretBlink is the OS's blink of the text caret (PLAN.md §5.5).
	CaretBlink CaretBlink
}

// CaretBlink is the blink of the text caret as the OS sets it.
type CaretBlink struct {
	// Period is one full on-and-off cycle; 0 means the caret does not
	// blink (it stays solid).
	Period time.Duration
	// Timeout is how long the caret blinks after the last edit or move
	// before it stays solid; 0 means it never stops blinking.
	Timeout time.Duration
}

// DefaultCaretBlink is the caret blink where the OS provides none: a
// one-second cycle that stops after ten seconds.
var DefaultCaretBlink = CaretBlink{Period: time.Second, Timeout: 10 * time.Second}

// DefaultAccent is the macOS default accent color (system blue, light
// appearance). Drivers use it when the OS provides no accent color.
var DefaultAccent = RGB8(0, 122, 255)

// DefaultAppearance returns the light appearance with the default accent
// color, no accessibility options and DefaultCaretBlink.
func DefaultAppearance() Appearance {
	return Appearance{Accent: DefaultAccent, CaretBlink: DefaultCaretBlink}
}

// ControlSize mirrors NSControl.ControlSize. The numeric values match
// NSControlSize, so the zero value is ControlSizeRegular.
type ControlSize uint8

const (
	ControlSizeRegular ControlSize = 0
	ControlSizeSmall   ControlSize = 1
	ControlSizeMini    ControlSize = 2
	ControlSizeLarge   ControlSize = 3
)

// TextAlignment is the horizontal alignment of text. Leading and trailing
// follow the layout direction (RTL is reserved, PLAN.md §7.4).
type TextAlignment uint8

const (
	AlignLeading TextAlignment = iota // natural alignment, the default
	AlignCenter
	AlignTrailing
	AlignJustified
)
