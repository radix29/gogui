package shell

import (
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
)

// Shell is one OS integration backend. The aqua driver creates exactly one
// per process.
//
// Every method except Post and Caps must be called on the UI thread.
// NewSurface and Appearance may be called only after Handler.Ready, which
// hands over the shell's text system and frame clock once (Services). A
// shell with popups also implements PopupShell. Every handler argument
// must be non-nil.
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

	// Post schedules fn to run on the UI thread, with the contract of a
	// driver's queue (driver.Driver.Queue): it may be called from any
	// goroutine once Run has been called, posted functions run in order,
	// and functions posted after Quit may never run. Wake-ups of the native
	// loop are coalesced (one PostMessage or g_idle_add per drain, however
	// many functions are queued), so a busy goroutine cannot flood the
	// native message queue.
	Post(fn func())

	// NewSurface creates a hidden surface (a native top-level window). Call
	// Surface.Show to show it. A bad SurfaceConfig.Owner returns
	// ErrSurfaceOwner (see SurfaceConfig.Modal).
	NewSurface(cfg SurfaceConfig, h SurfaceHandler) (Surface, error)

	// Appearance returns the current system appearance: dark mode, the OS
	// accent color (driver.DefaultAccent when the OS has none), and the
	// accessibility and scroller preferences, and the caret blink. Forcing
	// light or dark (driver.Driver.SetAppearance) is the aqua driver's job;
	// the shell only reports the OS setting.
	Appearance() driver.Appearance

	// SetClipboardText puts text on the system clipboard as plain text,
	// replacing what was there. Line breaks are passed on as they are. A
	// failure (another process holding the clipboard for too long) leaves
	// the clipboard unchanged and is not reported.
	SetClipboardText(text string)

	// ReadClipboardText reads the clipboard's plain text and calls fn with
	// it on the UI thread, always later, never from inside the call (GTK's
	// read is asynchronous, so every shell delivers through its loop). ok
	// is false when the clipboard holds no text or cannot be read; text is
	// then empty. Line breaks arrive as the clipboard has them ("\r\n" on
	// Windows). fn follows Post's rule at Quit: a read still pending then
	// may never be delivered. fn must be non-nil.
	ReadClipboardText(fn func(text string, ok bool))

	// After calls fn once on the UI thread, d after the call at the
	// earliest (d <= 0: as soon as the loop gets to it), never from inside
	// the call. It is a one-shot timer for work that must not keep the
	// frame clock running, such as the caret blink. The resolution is the
	// OS timer's: on Windows a USER timer (at least 10 ms, and usually up
	// to a system tick late), on Linux a GLib timeout (whole milliseconds).
	//
	// The returned cancel stops a timer that has not fired yet; once fn
	// has started, and on every later call, cancel does nothing. cancel
	// must be called on the UI thread. Timers still pending at Quit are
	// dropped (fn is never called); After after Quit makes no timer and
	// returns a cancel that does nothing. fn must be non-nil.
	After(d time.Duration, fn func()) (cancel func())

	// Caps reports the shell's fixed capabilities. The answer never
	// changes; it may be called from any goroutine, before Run too.
	Caps() Caps
}

// Caps are the fixed capabilities of a shell (Shell.Caps).
type Caps struct {
	// PopupShadow reports whether the shell draws a shadow around every
	// popup itself (Win32: CS_DROPSHADOW and DWM's), so a popup's content
	// paints none. Where it does not (gtk4 clears the popover's own), a
	// menu draws its shadow inside its popup, which grows by the shadow's
	// margin (PLAN.md S5.5k).
	PopupShadow bool
}

// PopupShell is a Shell with popups (popup.go): menus and tooltips. Every
// shell implements it (the fake, Win32 and, since S5.5f, gtk4); on a shell
// without it the aqua driver opens no popups, so menus and tooltips do not
// show.
type PopupShell interface {
	Shell

	// NewPopup creates and shows a popup (popup.go) owned by parent, which
	// must be an open, shown, not minimized surface of the same shell
	// (ErrPopupParent otherwise). The shell places it in the work area of
	// the anchor's screen from cfg (shellkit.PlacePopup on the fake and
	// Win32, GTK's own placement on Linux) and reports the result through
	// h.Placed. Popups are painted on the frame clock like surfaces, after
	// them. NewPopup must not be called during a Paint; h
	// must be non-nil.
	//
	// A Grab popup (at most one per shell, ErrPopupGrab otherwise) takes
	// the pointer and the keys while it is open:
	//
	//   - Opening it ends the parent's pointer capture as an OS cancel
	//     does: the parent gets a PointerUp (Clicks 0) for every held
	//     button, then PointerLeave. The held buttons stay held for the
	//     popup: its first events carry them in Buttons, and their release
	//     comes to the popup as a PointerUp with no PointerDown before it
	//     (a press-drag-release from a pop-up button).
	//   - Every pointer event goes to the popup, in its coordinates: enter,
	//     move and leave for its area (its own rect and its
	//     PopupConfig.Track), moves anywhere while a button is held,
	//     presses and scrolls over its area. The parent gets no pointer
	//     events.
	//   - A press outside the area (over the parent or anywhere else) is
	//     eaten, its release too, and dismisses the popup.
	//   - The parent's key, text and preedit events go to the popup's
	//     handler; the parent stays key and gets no KeyChanged.
	//   - The OS dismisses it once, through Dismissed: on an outside press,
	//     the parent stopping being key (deactivation, minimizing), a move
	//     or resize of the parent, and a mode cancel or a lost capture. The
	//     grab ends before Dismissed (a button still held on the popup gets
	//     its PointerUp first); the popup stays shown until Close.
	//
	// A popup without Grab takes no input at all: pointer events over it
	// go to what is below it, as if it were not there, and the OS never
	// dismisses it.
	NewPopup(parent Surface, cfg PopupConfig, h PopupHandler) (Popup, error)
}

// Services are the shell's UI-thread services, handed to Handler.Ready
// once. Both stay the same for the shell's life.
type Services struct {
	// Text is the shell's text layout engine: DirectWrite on Windows,
	// Pango on Linux, fixed-width fake metrics in shell/fake.
	Text render.TextSystem

	// Clock is the frame clock.
	Clock FrameClock
}

// Handler receives shell-level events. Every method is called on the UI
// thread.
type Handler interface {
	// Ready is called once, when the event loop is running, with the
	// shell's text system and frame clock (both non-nil). The aqua driver
	// calls driver.AppHandler.Ready from here.
	Ready(sv Services)

	// AppearanceChanged is called when the system appearance changes (dark
	// mode, accent color, contrast, motion, transparency or scroller
	// settings, caret blink), with the new value of Shell.Appearance.
	AppearanceChanged(a driver.Appearance)
}
