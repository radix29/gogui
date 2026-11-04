package driver

// WindowStyle is a set of window features. The bit values match
// NSWindowStyleMask.
type WindowStyle uint32

const (
	StyleTitled         WindowStyle = 1 << 0
	StyleClosable       WindowStyle = 1 << 1
	StyleMiniaturizable WindowStyle = 1 << 2
	StyleResizable      WindowStyle = 1 << 3
)

// WindowConfig configures a new window.
type WindowConfig struct {
	Title string

	// Size is the initial content size (the area below the title bar).
	Size Size

	// MinSize and MaxSize limit the content size when the user resizes the
	// window. Zero components mean "no limit".
	MinSize, MaxSize Size

	Style WindowStyle

	// Center centers the window on the main screen when it is first shown.
	// Otherwise the OS chooses the position.
	Center bool
}

// WindowHandler receives events from a Window.
type WindowHandler interface {
	// ContentResized is called when the content size changes: on user
	// resizing, zoom, DPI changes that change the logical size, and after
	// SetContentSize. The public layer runs the layout here.
	ContentResized(size Size)

	// ShouldClose is called when the user asks to close the window (close
	// button, ⌘W / Ctrl+W, the OS). Returning false keeps it open.
	ShouldClose() bool

	// Closed is called once, when the window has closed, whether by the
	// user, by Window.Close or by Driver.Quit. The window peer is dead
	// afterwards.
	Closed()

	// KeyChanged is called when the window becomes or stops being the key
	// window (the one receiving keyboard input).
	KeyChanged(isKey bool)
}

// Window is a top-level window (NSWindow). On Windows and Linux the aqua
// driver draws the title bar, traffic lights and menu strip itself.
type Window interface {
	SetTitle(title string)

	// SetContent sets the root widget of the window; nil removes it. Its
	// frame is relative to the content area and is set by the public layer,
	// normally to (0, 0, ContentSize()).
	SetContent(w Widget)

	// ContentSize returns the current content size.
	ContentSize() Size

	// SetContentSize resizes the content area. The size is clamped to the
	// min and max sizes.
	SetContentSize(size Size)

	// SetMinSize and SetMaxSize change the limits from WindowConfig.
	SetMinSize(size Size)
	SetMaxSize(size Size)

	// Center centers the window on its screen.
	Center()

	// Show shows the window, brings it to the front and makes it key.
	Show()

	// Close closes the window without calling ShouldClose, and then calls
	// Closed. Calling Close on a closed window does nothing.
	Close()
}
