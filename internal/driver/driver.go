package driver

// AppConfig configures the application. It is passed to Driver.Run.
type AppConfig struct {
	// Name is the application name. It is the title of the app menu and is
	// used in default menu titles ("Quit Name", "About Name", "Hide Name").
	Name string
}

// Driver is a platform backend. The public layer creates exactly one Driver
// per process and talks to the OS only through it.
//
// Every method except Post must be called on the UI thread. The factory
// methods may be called only after AppHandler.Ready has been called. Every
// handler argument must be non-nil.
type Driver interface {
	// Name identifies the backend for diagnostics: "appkit", "aqua" or
	// "headless".
	Name() string

	// Run starts the native event loop on the calling goroutine, which must
	// be the locked main OS thread. It calls h.Ready once the loop is
	// running and returns after Quit, or with an error if the backend cannot
	// start (for example, when GTK 4 is missing on Linux).
	Run(cfg AppConfig, h AppHandler) error

	// Quit makes Run return after the current event has been handled. Open
	// windows are closed (each gets WindowHandler.Closed). Quit does not
	// call AppHandler.QuitRequested.
	Quit()

	// Post schedules fn to run on the UI thread. It may be called from any
	// goroutine once Run has been called. Posted functions run in order.
	// Functions posted after Quit may never run.
	Post(fn func())

	// Appearance returns the current effective appearance.
	Appearance() Appearance

	// SetAppearance forces light or dark mode, or follows the OS again
	// (AppearanceSystem). If the effective appearance changes, the driver
	// calls AppHandler.AppearanceChanged.
	SetAppearance(mode AppearanceMode)

	// SetMenuBar replaces the whole menu bar. A nil bar installs the default
	// menu bar: the app menu and the Edit menu. On macOS an Edit menu is
	// always present, because text editing shortcuts depend on it
	// (PLAN.md §6.2); if bar has no menu with MenuEdit, the driver adds the
	// default one.
	SetMenuBar(bar *MenuBarSpec)

	// RunAlert shows a. With a parent window it is shown as a sheet (or a
	// modal centered on the parent); with a nil parent it is app-modal.
	// RunAlert returns immediately. When the user dismisses the alert, done
	// is called on the UI thread with the index of the chosen button in
	// a.Buttons. done may be nil.
	RunAlert(parent Window, a AlertSpec, done func(result int))

	// NewWindow creates a hidden window. Call Window.Show to show it.
	NewWindow(cfg WindowConfig, h WindowHandler) (Window, error)

	// NewContainer creates a plain container view (a flipped NSView on
	// macOS). Stacks and forms are containers whose child frames are set by
	// the shared layout engine.
	NewContainer() Container

	NewLabel() Label
	NewButton(h ButtonHandler) Button
	NewCheckbox(h ToggleHandler) Checkbox
	NewRadio(h ToggleHandler) Radio

	// NewTextField creates a single-line text field. With secure set it is a
	// password field (NSSecureTextField) that hides its text and disallows
	// copying it.
	NewTextField(secure bool, h TextFieldHandler) TextField

	NewSlider(h SliderHandler) Slider
	NewProgress() Progress
	NewPopUpButton(h PopUpHandler) PopUpButton
	NewSeparator() Separator
	NewBox() Box

	// NewScrollView creates a scroll view that hosts one document widget.
	NewScrollView(h ScrollHandler) ScrollView

	// NewTextView creates a multi-line, plain-text view that scrolls itself
	// (on macOS, an NSTextView inside an NSScrollView).
	NewTextView(h TextViewHandler) TextView
}

// AppHandler receives application-level events from the driver. Every
// method is called on the UI thread.
type AppHandler interface {
	// Ready is called once, when the event loop is running. The public
	// layer builds its UI here.
	Ready()

	// AppearanceChanged is called when the effective appearance changes,
	// either because the OS setting changed or because of SetAppearance.
	AppearanceChanged(a Appearance)

	// QuitRequested is called when the user asks to quit: the Quit menu
	// item, ⌘Q, the Dock menu, or the OS logging out. Returning false
	// cancels. Returning true makes the driver behave as if Quit was called.
	QuitRequested() bool

	// MenuItemSelected is called when the user chooses a menu item with a
	// non-zero ID, by clicking it or by its shortcut.
	MenuItemSelected(id int)
}
