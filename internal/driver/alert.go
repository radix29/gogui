package driver

// AlertStyle is the severity of an alert. The zero value is
// AlertInformational. (The numeric values do not match NSAlertStyle;
// drivers map them.)
type AlertStyle uint8

const (
	AlertInformational AlertStyle = iota
	AlertWarning
	// AlertCritical shows the caution badge over the app icon on macOS.
	AlertCritical
)

// AlertSpec describes a modal alert (NSAlert).
type AlertSpec struct {
	Style AlertStyle

	// Message is the bold message text.
	Message string

	// Detail is the informative text below the message. It may be empty.
	Detail string

	// Buttons are the button titles, in order, and are never empty (the
	// public layer supplies "OK"). The first button is the default button
	// (Return). As in AppKit, the buttons are laid out right to left on
	// macOS and Windows/Linux alike.
	Buttons []string

	// Cancel is the index of the button triggered by Escape, or -1 for none.
	Cancel int
}
