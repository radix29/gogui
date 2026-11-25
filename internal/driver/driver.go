package driver

import (
	"time"

	"github.com/radix29/gogui/internal/mainthread"
)

// AppConfig configures the application. It is passed to Driver.Run.
type AppConfig struct {
	// Name is the application name. It is the title of the app menu and is
	// used in default menu titles ("Quit Name", "About Name", "Hide Name").
	Name string
}

// Driver is a platform backend. The public layer creates exactly one Driver
// per process and talks to the OS only through it.
//
// Every method except Queue must be called on the UI thread. The factory
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

	// Queue returns the UI-thread queue that Run drains, or nil before Run
	// has created it. It may be called from any goroutine. Functions queued
	// with Do run on the UI thread, in order; after Quit the queue is
	// closed, so Do and DoSync drop their functions and return false.
	// gogui.Do, gogui.DoSync and the UI-thread checks use it.
	Queue() *mainthread.Queue

	// ObserveFrames sets the function called with the timing of each frame
	// after it, on the UI thread (the debug frame-time log, PLAN.md
	// S6.2c); nil removes it. It returns the frame clock's Now, the
	// timeline FrameTiming.Time is on, or nil if the driver has no frame
	// clock (headless, appkit); fn is then never called. It may be called
	// only after AppHandler.Ready has been called.
	ObserveFrames(fn func(FrameTiming)) (now func() time.Duration)

	// Appearance returns the current effective appearance.
	Appearance() Appearance

	// SetAppearance forces light or dark mode, or follows the OS again
	// (AppearanceSystem). If the effective appearance changes, the driver
	// calls AppHandler.AppearanceChanged.
	SetAppearance(mode AppearanceMode)

	// SetAccentColor overrides the OS accent color for the whole app
	// (PLAN.md D10, S6.1g), or clears the override with the zero Color.
	// A semantic c is resolved against the effective appearance each time
	// it changes, with DefaultAccent for the colors that follow the accent
	// (so ColorSystemBlue is blue); the result is made opaque.
	// Appearance().Accent is then the override, and every accent-derived
	// color follows it. If the effective appearance changes, the driver
	// calls AppHandler.AppearanceChanged. A driver without an app-wide
	// accent (appkit) records the color.
	SetAccentColor(c Color)

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

// FrameTiming is the wall time the parts of one frame took on the UI
// thread (PLAN.md S6.2c), as a Driver's frame observer (ObserveFrames)
// receives it.
// shell.FrameTiming is an alias of it, so the aqua driver passes a
// FrameClock's records on as they are.
//
// A frame is the frame function, then the paint pass: on Win32 and the
// fake shell every invalidated surface is painted inside the frame; on
// gtk4 the paint pass queues draws, which GTK runs in the paint phase of
// the same GDK frame, so the record is reported after the last of them
// (or at the next frame, if one never came). A paint outside any frame (a
// synchronous resize paint, a redraw GTK asks for by itself) is reported
// as a record of its own with Time -1 and no frame function.
type FrameTiming struct {
	// Time is the frame's target time on the clock's Now timeline, -1
	// for a paint outside a frame.
	Time time.Duration
	// FrameFunc is the frame function (the driver's animations).
	FrameFunc time.Duration
	// Paint is the SurfaceHandler.Paint and PopupHandler.Paint calls of
	// the frame: the drawing on the canvas.
	Paint time.Duration
	// Submit is the rest of the CPU work of drawing: setting the canvas
	// up and finishing it (Win32: BeginDraw and Clear, then the canvas's
	// finish, EndDraw and the release of its resources; gtk4: the
	// canvas's finish into the GtkSnapshot; the fake: the recorder's
	// checks). GSK's rendering on gtk4 happens after the snapshot, in
	// GTK, and is not in it.
	Submit time.Duration
	// Present is Present and the composition's Commit on Win32, which
	// may wait for the display; 0 elsewhere. It is not part of the
	// frame's budget.
	Present time.Duration
	// Surfaces is the number of surfaces and popups painted.
	Surfaces int
}
