package shell

import (
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
)

// Shell is one OS integration backend. The aqua driver creates exactly one
// per process.
//
// Every method except Post must be called on the UI thread. NewSurface,
// TextSystem, Appearance and Clock may be called only after Handler.Ready
// has been called. Every handler argument must be non-nil.
type Shell interface {
	// Run starts the native event loop on the calling goroutine, which must
	// be the locked main OS thread. It calls h.Ready once the loop is
	// running and returns nil after Quit, or an error if the shell cannot
	// start (for example, when GTK 4 is missing on Linux). Run may be called
	// only once.
	Run(h Handler) error

	// Quit makes Run return after the current event has been handled. Open
	// surfaces are closed first, each getting SurfaceHandler.Closed, without
	// CloseRequested being asked.
	Quit()

	// Post schedules fn to run on the UI thread, with the same contract as
	// driver.Driver.Post: it may be called from any goroutine once Run has
	// been called, posted functions run in order, and functions posted after
	// Quit may never run. Wake-ups of the native loop are coalesced (one
	// PostMessage or g_idle_add per drain, however many functions are
	// queued), so a busy goroutine cannot flood the native message queue.
	Post(fn func())

	// NewSurface creates a hidden surface (a native top-level window). Call
	// Surface.Show to show it.
	NewSurface(cfg SurfaceConfig, h SurfaceHandler) (Surface, error)

	// TextSystem returns the shell's text layout engine: DirectWrite on
	// Windows, Pango on Linux, fixed-width fake metrics in shell/fake. The
	// same value is returned on every call.
	TextSystem() render.TextSystem

	// Appearance returns the current system appearance: dark mode, the OS
	// accent color (driver.DefaultAccent when the OS has none), and the
	// accessibility and scroller preferences. Forcing light or dark
	// (driver.Driver.SetAppearance) is the aqua driver's job; the shell only
	// reports the OS setting.
	Appearance() driver.Appearance

	// Clock returns the frame clock. The same value is returned on every
	// call.
	Clock() FrameClock
}

// Handler receives shell-level events. Every method is called on the UI
// thread.
type Handler interface {
	// Ready is called once, when the event loop is running. The aqua driver
	// calls driver.AppHandler.Ready from here.
	Ready()

	// AppearanceChanged is called when the system appearance changes (dark
	// mode, accent color, contrast, motion, transparency or scroller
	// settings), with the new value of Shell.Appearance.
	AppearanceChanged(a driver.Appearance)
}
