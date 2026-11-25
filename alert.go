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
// default button (Return). Cancel picks the button Escape triggers.
// Without Buttons the alert has a single "OK" button. Style picks the
// icon: the app icon, or for AlertCritical a caution sign with the app
// icon as its badge; AlertWarning looks like AlertInformational, as in
// NSAlert. On Windows and Linux the app icon is a drawn placeholder.
type Alert struct {
	Style   AlertStyle
	Message string
	Detail  string
	Buttons []string

	// Cancel is the index in Buttons of the button Escape triggers, or
	// AlertNoCancel for none. The zero value picks the first button
	// titled exactly "Cancel" (none if there is none), so an English
	// alert needs no Cancel; a localized one sets it. The first button is
	// the default button and can't also be the cancel button; an index
	// outside Buttons means none.
	Cancel int
}

// AlertNoCancel is the Alert.Cancel value for an alert that Escape does
// not answer.
const AlertNoCancel = -1

// Show shows the alert and returns at once. With a window it is attached to
// it as a sheet; with a nil or closed window it is app-modal. When the user
// dismisses the alert, done is called on the UI thread with the index of
// the chosen button in Buttons (0 for the default "OK"). done may be nil.
// Show must be called on the UI thread, inside Run.
//
// On Windows and Linux a sheet hangs from the bottom of the window's title
// bar, and a window shows one sheet at a time: a second Show waits until
// the first is answered. An app-modal alert is a window of its own, titled
// with AppConfig.Name, and while it is shown the other windows take no
// input and only Quit is enabled in the menus. On the headless driver
// (macOS until the AppKit driver exists) nobody can answer an alert, so
// done is never called.
func (a Alert) Show(win *Window, done func(result int)) {
	d := driverFor("Alert.Show")
	var parent driver.Window
	if win != nil && !win.closed {
		parent = win.p
	}
	d.RunAlert(parent, a.spec(), done)
}

// spec returns the driver's description of a: "OK" when there are no
// buttons, and the cancel index of Alert.Cancel (or -1 for none).
func (a Alert) spec() driver.AlertSpec {
	buttons := append([]string(nil), a.Buttons...)
	if len(buttons) == 0 {
		buttons = []string{"OK"}
	}
	cancel := -1
	switch {
	case a.Cancel == 0:
		for i, b := range buttons {
			if b == "Cancel" {
				cancel = i
				break
			}
		}
	case a.Cancel > 0 && a.Cancel < len(buttons):
		cancel = a.Cancel
	}
	return driver.AlertSpec{Style: a.Style, Message: a.Message, Detail: a.Detail, Buttons: buttons, Cancel: cancel}
}
