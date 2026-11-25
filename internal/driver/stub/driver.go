package stub

import (
	"slices"

	"github.com/radix29/gogui/internal/driver"
)

// Driver implements the UI-thread part of driver.Driver with stub peers:
// the factories, the appearance, the menu bar and alerts, plus the User…
// simulations of the app-level events. It has no run loop: Name, Run,
// Quit, Queue and ObserveFrames come from the driver that embeds it (the
// headless driver), which also sets Config, Handler and Running.
type Driver struct {
	// Config is the AppConfig passed to Run.
	Config driver.AppConfig
	// Handler is the AppHandler passed to Run.
	Handler driver.AppHandler

	// Mode is the mode last passed to SetAppearance.
	Mode driver.AppearanceMode
	// System is the simulated OS appearance, used for AppearanceSystem.
	System driver.Appearance
	// Accent is the color last passed to SetAccentColor; the zero Color
	// means no override.
	Accent driver.Color

	// MenuBar is a copy of the spec last passed to SetMenuBar, with nil
	// replaced by driver.DefaultMenuBar.
	MenuBar *driver.MenuBarSpec

	// Windows lists the windows that have been created and not closed.
	Windows []*Window

	// Alerts lists the alerts that are shown and not dismissed yet.
	Alerts []*Alert

	// Running is true while Run is executing.
	Running bool

	current driver.Appearance
}

// Alert is an alert shown by Driver.RunAlert.
type Alert struct {
	Parent driver.Window
	Spec   driver.AlertSpec
	done   func(int)
	drv    *Driver
}

// NewDriver returns a stub driver with the default (light) appearance.
func NewDriver() *Driver {
	return &Driver{
		System:  driver.DefaultAppearance(),
		current: driver.DefaultAppearance(),
	}
}

// Appearance implements driver.Driver.
func (d *Driver) Appearance() driver.Appearance { return d.current }

// SetAppearance implements driver.Driver.
func (d *Driver) SetAppearance(mode driver.AppearanceMode) {
	d.Mode = mode
	d.update()
}

// SetSystemAppearance simulates the OS appearance changing to a. It calls
// AppearanceChanged if the effective appearance changes.
func (d *Driver) SetSystemAppearance(a driver.Appearance) {
	d.System = a
	d.update()
}

// SetAccentColor implements driver.Driver. The stub has no palette: a
// concrete c (made opaque) replaces the accent of the effective
// appearance; a semantic one is only recorded in Accent and leaves the OS
// accent in place.
func (d *Driver) SetAccentColor(c driver.Color) {
	d.Accent = c
	d.update()
}

func (d *Driver) update() {
	a := d.System
	switch d.Mode {
	case driver.AppearanceLight:
		a.Dark = false
	case driver.AppearanceDark:
		a.Dark = true
	}
	if d.Accent != (driver.Color{}) && !d.Accent.IsSemantic() {
		a.Accent = d.Accent
		a.Accent.A = 1
	}
	if a == d.current {
		return
	}
	d.current = a
	if d.Handler != nil {
		d.Handler.AppearanceChanged(a)
	}
}

// SetMenuBar implements driver.Driver.
func (d *Driver) SetMenuBar(bar *driver.MenuBarSpec) {
	if bar == nil {
		d.MenuBar = driver.DefaultMenuBar()
		return
	}
	d.MenuBar = bar.Clone()
}

// UserSelectMenuItem simulates the user choosing the menu item with the
// given ID. It reports whether an enabled item with that ID exists in the
// current menu bar.
func (d *Driver) UserSelectMenuItem(id int) bool {
	if id == 0 || d.MenuBar == nil || d.Handler == nil {
		return false
	}
	it := d.MenuBar.FindItem(id)
	if it == nil || it.Disabled {
		return false
	}
	d.Handler.MenuItemSelected(id)
	return true
}

// RunAlert implements driver.Driver. The alert stays in Alerts until
// Dismiss is called on it.
func (d *Driver) RunAlert(parent driver.Window, a driver.AlertSpec, done func(result int)) {
	a.Buttons = slices.Clone(a.Buttons)
	d.Alerts = append(d.Alerts, &Alert{Parent: parent, Spec: a, done: done, drv: d})
}

// Dismiss simulates the user pressing the button at index result. It
// removes the alert from the driver and calls the done function.
func (a *Alert) Dismiss(result int) {
	if a.drv == nil {
		return
	}
	a.drv.Alerts = slices.DeleteFunc(a.drv.Alerts, func(x *Alert) bool { return x == a })
	a.drv = nil
	if a.done != nil {
		a.done(result)
	}
}

// NewWindow implements driver.Driver.
func (d *Driver) NewWindow(cfg driver.WindowConfig, h driver.WindowHandler) (driver.Window, error) {
	w := NewWindow(cfg, h)
	w.drv = d
	d.Windows = append(d.Windows, w)
	return w, nil
}

func (d *Driver) forget(w *Window) {
	d.Windows = slices.DeleteFunc(d.Windows, func(x *Window) bool { return x == w })
}

// KeyWindow returns the current key window, or nil.
func (d *Driver) KeyWindow() *Window {
	for _, w := range d.Windows {
		if w.IsKey {
			return w
		}
	}
	return nil
}

// Factory methods. They implement driver.Driver.

func (d *Driver) NewContainer() driver.Container { return NewContainer() }
func (d *Driver) NewLabel() driver.Label         { return NewLabel() }

func (d *Driver) NewButton(h driver.ButtonHandler) driver.Button     { return NewButton(h) }
func (d *Driver) NewCheckbox(h driver.ToggleHandler) driver.Checkbox { return NewCheckbox(h) }
func (d *Driver) NewRadio(h driver.ToggleHandler) driver.Radio       { return NewRadio(h) }

func (d *Driver) NewTextField(secure bool, h driver.TextFieldHandler) driver.TextField {
	return NewTextField(secure, h)
}

func (d *Driver) NewSlider(h driver.SliderHandler) driver.Slider { return NewSlider(h) }
func (d *Driver) NewProgress() driver.Progress                   { return NewProgress() }

func (d *Driver) NewPopUpButton(h driver.PopUpHandler) driver.PopUpButton {
	return NewPopUpButton(h)
}

func (d *Driver) NewSeparator() driver.Separator { return NewSeparator() }
func (d *Driver) NewBox() driver.Box             { return NewBox() }

func (d *Driver) NewScrollView(h driver.ScrollHandler) driver.ScrollView {
	return NewScrollView(h)
}

func (d *Driver) NewTextView(h driver.TextViewHandler) driver.TextView {
	return NewTextView(h)
}
