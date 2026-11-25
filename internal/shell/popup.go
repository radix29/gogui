package shell

import (
	"errors"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
)

// Errors of PopupShell.NewPopup.
var (
	// ErrPopupParent is returned when the parent is not an open, shown and
	// not minimized surface of this shell.
	ErrPopupParent = errors.New("shell: popup parent is not an open, visible surface of this shell")

	// ErrPopupGrab is returned for a Grab popup while another Grab popup of
	// the shell is open.
	ErrPopupGrab = errors.New("shell: another grab popup is open")
)

// PopupAdjust is a set of constraint adjustments of a popup (PopupConfig):
// what the shell may do when the popup's rect does not fit the work area of
// its screen, as the Wayland xdg_positioner's constraint_adjustment. There
// is no flip: a menu over its item slides, it does not jump.
type PopupAdjust uint8

const (
	// AdjustSlideX moves the popup left or right until it fits the work
	// area horizontally; a popup wider than the work area is aligned with
	// its left edge.
	AdjustSlideX PopupAdjust = 1 << iota

	// AdjustSlideY moves the popup up or down until it fits vertically; a
	// popup taller than the work area is aligned with its top edge.
	AdjustSlideY

	// AdjustResizeY cuts the popup's height to the work area, after any
	// slide: the parts above its top and below its bottom are cut off.
	// Nothing is cut when the popup lies wholly above or below the work
	// area.
	AdjustResizeY
)

// PopupConfig places a popup (PopupShell.NewPopup, Popup.Reposition).
// Rects and points are in logical points relative to the parent's
// drawable area.
type PopupConfig struct {
	// Anchor is the rect the popup belongs to (a pop-up button, the pointer
	// for a tooltip). A zero size is allowed.
	Anchor driver.Rect

	// Size is the popup's requested size. Negative and NaN components count
	// as zero.
	Size driver.Size

	// Offset is the popup's top-left corner relative to the anchor's
	// top-left corner before any adjustment. A menu over its selected item
	// has a negative Y.
	Offset driver.Point

	// Adjust is what the shell may do when the popup does not fit the work
	// area of the anchor's screen (none: it stays where Offset puts it,
	// partly off-screen).
	Adjust PopupAdjust

	// Grab makes the popup take the pointer and the keys (menus; see
	// PopupShell.NewPopup). Without it the popup takes no input at all
	// (tooltips). It is fixed at NewPopup; Reposition ignores it.
	Grab bool

	// Track is the grab's track area (S5.6d): a rect in the parent's
	// points (a menu bar's strip) that counts as part of a Grab popup's
	// area, so hover-switching and a click on another menu title work.
	// Pointer events over it go to the popup's handler, in the popup's
	// coordinates (outside its own rect, so possibly negative), as over
	// the popup itself: enter and leave for the whole area (none when the
	// pointer crosses between the track and the popup where they touch or
	// overlap), moves, presses (delivered: they do not dismiss) and
	// scrolls. The parent still gets no pointer events. An empty rect (a
	// size that is not positive and finite, or a position that is not
	// finite) is no track. Without Grab it is ignored. It is fixed at
	// NewPopup, at the parent's place then (a move of the parent dismisses
	// the grab anyway); Reposition ignores it.
	//
	// On gtk4 (S5.6e) GDK's autohide grab drops a press on the parent and
	// pops the popover down before GTK sees it, so a press on the track
	// normally dismisses the popup (an outside press: Dismissed); the
	// moves, enter and leave and the scrolls work as described, a leave
	// between the popover and the track deferred to an idle source.
	Track driver.Rect
}

// Popup is a borderless native surface owned by a parent Surface: a menu or
// a tooltip. It is never in the taskbar, never becomes key itself (the
// parent stays key and draws active) and closes with its parent. It is
// shown from NewPopup on.
//
// Every method must be called on the UI thread. After Close, or after
// PopupHandler.Closed, the popup is dead and every method does nothing.
type Popup interface {
	// Invalidate marks the whole popup as needing a paint and requests a
	// frame, as Surface.Invalidate.
	Invalidate()

	// SetCursor sets the pointer shape over a Grab popup.
	SetCursor(c Cursor)

	// Reposition places the popup again from cfg (cfg.Grab is ignored).
	// PopupHandler.Placed is called when the placed rect or the scale
	// changes; a change of size invalidates the popup. A grab popup's
	// pointer stays where it is on the screen: the events after the move
	// (also those the shell sends right after a handler repositioned it,
	// as an Enter's Move) are in the new place's points (S5.6f).
	Reposition(cfg PopupConfig)

	// Translucent reports whether the OS blurs what is behind the popup
	// (PLAN.md §3): the popup then paints a translucent fill, otherwise
	// its opaque fallback. It changes only with the appearance (high
	// contrast, transparency effects): before the shell invalidates the
	// popup and calls Handler.AppearanceChanged, so a Paint reads it.
	Translucent() bool

	// Close closes the popup and then calls PopupHandler.Closed, without
	// Dismissed. Calling Close on a closed popup does nothing. It must not
	// be called during a Paint.
	Close()
}

// PopupHandler receives the events of a popup. Every method is called on
// the UI thread.
type PopupHandler interface {
	// Paint draws the whole popup on c, in logical points relative to the
	// popup's top-left corner, as large as the rect of the last Placed.
	// The rules of SurfaceHandler.Paint apply.
	Paint(c render.Canvas)

	// Placed reports where the popup landed after the adjustments: rect in
	// the parent's points (possibly moved, and shorter than the requested
	// size with AdjustResizeY), and the popup's scale (device pixels per
	// point; on Windows the scale of the anchor's monitor). It is called
	// at least once before the first Paint, and again whenever the rect or
	// the scale changes (Reposition, the OS). It may be called from inside
	// NewPopup and Reposition (Win32, fake) or later from the loop (gtk4),
	// never during a Paint.
	Placed(rect driver.Rect, scale float64)

	// Event delivers the popup's input (a Grab popup only; see
	// PopupShell.NewPopup), with positions relative to the popup's top-left
	// corner. The result matters only for KeyEvents, as with
	// SurfaceHandler.Event.
	Event(ev Event) (handled bool)

	// Dismissed is called at most once, when the OS or the user dismisses
	// a Grab popup (see PopupShell.NewPopup). The grab has ended by then;
	// the popup stays shown until the handler calls Close, which it may do
	// from inside Dismissed.
	Dismissed()

	// Closed is called once, when the popup has closed: by Close, or with
	// its parent (before the parent's SurfaceHandler.Closed) or Shell.Quit.
	Closed()
}
