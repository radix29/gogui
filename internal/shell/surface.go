package shell

import (
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
}

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

	// SetMinSize and SetMaxSize change the limits from SurfaceConfig. If the
	// current size violates a new limit, the surface is resized (and
	// Resized called).
	SetMinSize(size driver.Size)
	SetMaxSize(size driver.Size)

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

	// Event delivers one input event (PointerEvent, ScrollEvent, KeyEvent or
	// TextEvent; see their docs for the order and grouping rules). It
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
