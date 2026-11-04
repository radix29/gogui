package gogui

import "github.com/radix29/gogui/internal/driver"

// AlertStyle is the severity of an alert. The zero value is
// AlertInformational.
type AlertStyle = driver.AlertStyle

// Alert styles.
const (
	AlertInformational = driver.AlertInformational
	AlertWarning       = driver.AlertWarning
	// AlertCritical shows the caution badge over the app icon on macOS.
	AlertCritical = driver.AlertCritical
)

// Alert describes a modal alert (NSAlert): a bold Message, an optional
// Detail text below it, and Buttons, in order. The first button is the
// default button (Return). The first button titled exactly "Cancel" is
// triggered by Escape; a localized title is not recognized yet.
// Without Buttons the alert has a single "OK" button.
type Alert struct {
	Style   AlertStyle
	Message string
	Detail  string
	Buttons []string
}

// Show shows the alert and returns at once. With a window it is attached to
// it (a sheet on macOS); with a nil or closed window it is app-modal. When
// the user dismisses the alert, done is called on the UI thread with the
// index of the chosen button in Buttons (0 for the default "OK"). done may
// be nil. Show must be called on the UI thread, inside Run.
func (a Alert) Show(win *Window, done func(result int)) {
	d := driverFor("Alert.Show")
	var parent driver.Window
	if win != nil && !win.closed {
		parent = win.p
	}
	d.RunAlert(parent, a.spec(), done)
}

// spec returns the driver's description of a: "OK" when there are no
// buttons, and the index of the first "Cancel" button (or -1) as Cancel.
func (a Alert) spec() driver.AlertSpec {
	buttons := append([]string(nil), a.Buttons...)
	if len(buttons) == 0 {
		buttons = []string{"OK"}
	}
	cancel := -1
	for i, b := range buttons {
		if b == "Cancel" {
			cancel = i
			break
		}
	}
	return driver.AlertSpec{Style: a.Style, Message: a.Message, Detail: a.Detail, Buttons: buttons, Cancel: cancel}
}
