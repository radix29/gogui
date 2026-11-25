package shell

import (
	"errors"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
)

// SurfaceConfig configures a new surface.
type SurfaceConfig struct {
	// Title is the native window title: shown by the taskbar, Alt-Tab and
	// the window switcher. The shell does not draw it; the aqua driver draws
	// the centered title in its own title bar.
	Title string

	// Size is the initial size of the drawable area. Unlike
	// driver.WindowConfig.Size it includes the title bar the aqua driver
	// draws, because the surface has no native title bar.
	Size driver.Size

	// MinSize and MaxSize limit Size when the user resizes the surface. Zero
	// components mean "no limit".
	MinSize, MaxSize driver.Size

	// Style selects the window features. Without StyleResizable the user
	// cannot resize the surface (no resize borders, no maximize, no Snap
	// Layouts). StyleTitled, StyleClosable and StyleMiniaturizable only
	// affect what the OS offers outside the drawable area (taskbar and
	// system menu entries); the aqua driver draws the title bar.
	Style driver.WindowStyle

	// Center centers the surface on its screen when it is first shown.
	// Otherwise the OS chooses the position.
	Center bool

	// Modal makes the surface modal (S5.7d, PLAN.md S5.7): a top-level
	// window for an alert, with no taskbar entry, never resizable,
	// minimizable or zoomable (Style's StyleResizable and
	// StyleMiniaturizable are ignored, so Minimize and Zoom do nothing).
	// It is placed on every Show that first shows it, and on Center:
	// centered on Owner when the owner is shown and not minimized, else on
	// the screen of the key surface (the primary screen without one), and
	// kept inside the work area (the Center field is implied).
	//
	// From its first Show until it closes the modal surface blocks every
	// other surface of the shell, those created later and older modal
	// surfaces included: a blocked surface takes no pointer, scroll, key,
	// text or preedit input and the OS refuses its activation (a click on
	// it, the taskbar, Alt-Tab), bringing the modal to the front and making
	// it key instead; the user cannot move, resize or close it (no
	// CloseRequested). Blocking a surface ends its grab popup and its
	// pointer capture (a PointerUp for every held button) and sends
	// PointerLeave if the pointer was inside, as a capture cancel does.
	// Programmatic calls (SetSize, Show, Minimize, Close …) still work, and
	// a blocked surface still paints. Show on a blocked surface shows it
	// but keeps the key on the modal surface.
	//
	// When the modal surface closes and it was key, the key goes to the
	// next modal surface still open, else to Owner, else to the surface
	// that was key when it was first shown, if that one is open, shown,
	// not minimized and not blocked (after the modal's Closed on the fake;
	// before it on Win32, where Windows deactivates a window before it
	// destroys it). Closing the owner closes its modal surfaces first.
	//
	// The fake shell implements it since S5.7d, the Win32 shell since S5.7e
	// (an owned WS_POPUP window, disabled with EnableWindow), the gtk4 shell
	// since S5.7f (a modal GtkWindow transient for Owner, or for the key
	// surface when app-modal; the others made insensitive). GTK 4 cannot
	// place a toplevel: there the window manager places the modal surface,
	// and the key hand-off presents the next surface, after the modal's
	// Closed as on the fake.
	Modal bool

	// Owner is the surface a Modal surface belongs to (an alert's parent
	// window), or nil for an app-modal one. It must be nil unless Modal is
	// set, and else an open surface of the same shell: NewSurface returns
	// ErrSurfaceOwner otherwise.
	Owner Surface
}

// ErrSurfaceOwner is returned by Shell.NewSurface for a SurfaceConfig.Owner
// that is not nil without Modal, or not an open surface of the shell.
var ErrSurfaceOwner = errors.New("shell: surface owner is not an open surface of this shell, or the surface is not modal")

// Surface is one native top-level window without a native title bar: an
// HWND with a custom non-client area (WM_NCCALCSIZE) on Windows, a GtkWindow
// with client-side decorations on Linux. The aqua driver draws the whole
// drawable area, title bar and traffic lights included (PLAN.md §5.4); the
// shell keeps the OS resize borders, shadow and rounded corners.
//
// Every method must be called on the UI thread. After Close, or after
// SurfaceHandler.Closed, the surface is dead and every method does nothing.
type Surface interface {
	// SetTitle changes the native window title (see SurfaceConfig.Title).
	SetTitle(title string)

	// Size returns the current size of the drawable area. It is valid from
	// NewSurface on.
	Size() driver.Size

	// SetSize resizes the drawable area, clamped to the min and max sizes.
	// If the size changes, SurfaceHandler.Resized is called before SetSize
	// returns.
	SetSize(size driver.Size)

	// SetSizeLimits replaces both limits from SurfaceConfig (MinSize and
	// MaxSize; zero components mean "no limit"). If the current size
	// violates the new limits, the surface is resized once to the size
	// clamped to both (and Resized called once).
	SetSizeLimits(min, max driver.Size)

	// Scale returns the number of device pixels per logical point (1, 1.25,
	// 1.5, 2 …; fractional on Windows and on Wayland). It is valid from
	// NewSurface on (the scale of the screen the surface will appear on).
	// Backends apply it when painting; callers never see device pixels.
	Scale() float64

	// Center centers the surface on its screen.
	Center()

	// Show shows the surface, brings it to the front and makes it key.
	Show()

	// Minimize minimizes (iconifies) the surface: the yellow traffic light.
	Minimize()

	// Zoom toggles between the maximized and the previous size: the green
	// traffic light. It does nothing without StyleResizable. Resized is
	// called when the size changes.
	Zoom()

	// Zoomed reports whether the surface is maximized.
	Zoomed() bool

	// Close closes the surface without calling CloseRequested, and then
	// calls Closed. Calling Close on a closed surface does nothing. Closing
	// the last surface does not quit the shell.
	Close()

	// Invalidate marks the whole surface as needing a paint and requests a
	// frame (FrameClock.RequestFrame). SurfaceHandler.Paint is called once
	// at the next frame, however often Invalidate was called before it. A
	// hidden or minimized surface is painted when it becomes visible again.
	// Called from inside the frame function it requests no frame: the paint
	// pass of the frame that runs paints the surface, so an animation whose
	// last step invalidates ends without an empty frame after it (S6.2a).
	Invalidate()

	// SetCursor sets the pointer shape shown while the pointer is over the
	// drawable area outside the resize borders.
	SetCursor(c Cursor)

	// SetChromeRegions tells the shell which parts of the drawable area are
	// window chrome, so it can answer the OS hit-tests (WM_NCHITTEST on
	// Windows, the CSD drag area on Linux) by itself, without calling Go on
	// every pointer move. regions is copied. A point is tested against the
	// regions from last to first and gets the kind of the first one that
	// contains it; a point in no region is RegionClient. So the aqua driver
	// sets the title bar as RegionCaption, then each menu title of the menu
	// strip as RegionClient (PLAN.md §5.4.1), then the traffic lights. The
	// regions stay until replaced; nil means "all client". Resize borders
	// are the shell's and always win over the regions.
	SetChromeRegions(regions []ChromeRegion)

	// SetTextInput tells the shell whether the focus takes text through an
	// IME, and where its caret is. While ti.Enabled is false (the initial
	// state) the IME is off for the surface: keys go to the handler and the
	// text they type comes as TextEvents, with no compositions. While it is
	// true, the IME gets the keys first and sends PreeditEvents and commits
	// (see PreeditEvent), and the shell places the IME's candidate window
	// by ti.Caret. Disabling it during a composition drops the composition
	// without any event. It may be called during SurfaceHandler.Paint, and
	// it calls no handler method.
	SetTextInput(ti TextInput)

	// CancelComposition drops the IME's composition, if there is one,
	// without any event: no PreeditEvent, and no TextEvent even if the IME
	// commits while it is reset. The aqua driver calls it when it commits
	// the preedit itself (blur, a click, SetText). The text input stays
	// enabled. It calls no handler method and must not be called during
	// SurfaceHandler.Paint.
	CancelComposition()
}

// TextInput is the text input state of a surface (Surface.SetTextInput).
type TextInput struct {
	// Enabled turns the IME on for the surface: the focus is an editable
	// text element that takes compositions.
	Enabled bool

	// Caret is the caret rectangle in logical points relative to the
	// drawable area (at the preedit's cursor while composing). The shell
	// puts the IME's candidate window next to it without covering it. It
	// is ignored while Enabled is false.
	Caret driver.Rect
}

// SurfaceHandler receives the events of a surface. Every method is called on
// the UI thread.
type SurfaceHandler interface {
	// Paint draws the whole surface on c, in logical points with a top-left
	// origin, Size() points large (the canvas applies Scale()). c is valid
	// only during the call, and every Save and Push* must be closed before
	// Paint returns (render.Canvas). Paint only draws: it must not resize,
	// show or close a surface, or create one. Invalidate is allowed and
	// schedules another paint at the following frame.
	Paint(c render.Canvas)

	// Resized is called when the size or the scale changes: user resizing,
	// Zoom, Snap, a move to a screen with another scale, SetSize. It is not
	// called for the size and scale the surface was created with. The shell
	// invalidates the surface itself; during an interactive resize it paints
	// before returning to the OS, so the content never lags the frame
	// (PLAN.md S2.4).
	Resized(size driver.Size, scale float64)

	// CloseRequested is called when the user or the OS asks to close the
	// surface (Alt+F4, the taskbar, the window switcher, logging out).
	// Returning true closes it, as if Close was called; false keeps it open.
	// The aqua driver's own close button does not go through here: the
	// driver calls Close itself.
	CloseRequested() bool

	// Closed is called once, when the surface has closed, whether by
	// CloseRequested, Close or Shell.Quit. The surface is dead afterwards.
	Closed()

	// KeyChanged is called when the surface becomes or stops being the key
	// window (the foreground window receiving keyboard input). The aqua
	// driver draws inactive chrome and controls while it is false.
	KeyChanged(isKey bool)

	// Event delivers one input event (PointerEvent, ScrollEvent, KeyEvent,
	// TextEvent or PreeditEvent; see their docs for the order and grouping
	// rules). It
	// returns whether the event was handled; this matters only for key
	// events (KeyEvent) and is ignored for the others.
	Event(ev Event) (handled bool)
}

// RegionKind is the meaning of a chrome region for OS hit-testing.
type RegionKind uint8

const (
	// RegionClient is ordinary content: pointer events go to
	// SurfaceHandler.Event. It is the kind of every point outside the
	// regions.
	RegionClient RegionKind = iota

	// RegionCaption is the title-bar drag area (HTCAPTION). A primary-button
	// press there starts the native window move, and a double-click does
	// the OS's title-bar action (maximize, like Zoom). Neither press is
	// delivered as a PointerEvent; moves and hover still are.
	RegionCaption

	// RegionClose, RegionMinimize and RegionMaximize are the traffic lights
	// (HTCLOSE, HTMINBUTTON, HTMAXBUTTON). Pointer events there are
	// delivered as ordinary PointerEvents in surface coordinates, so the
	// aqua driver tracks hover and press and calls Close, Minimize or Zoom
	// itself. The shell uses the kinds only for OS features: hovering
	// RegionMaximize opens the Windows 11 Snap Layouts flyout.
	RegionClose
	RegionMinimize
	RegionMaximize
)

// ChromeRegion is one hit-test region of a surface, in logical points
// relative to the drawable area.
type ChromeRegion struct {
	Rect driver.Rect
	Kind RegionKind
}

// Cursor is a pointer shape. The shells map it to the system cursors
// (IDC_* on Windows, CSS cursor names on GTK).
type Cursor uint8

const (
	CursorArrow           Cursor = iota // the default arrow
	CursorIBeam                         // text: text fields and text views
	CursorPointingHand                  // links
	CursorResizeLeftRight               // vertical split views, column edges
	CursorResizeUpDown                  // horizontal split views
	CursorCrosshair
	CursorNotAllowed
	CursorHidden // no cursor, for example while typing
)
