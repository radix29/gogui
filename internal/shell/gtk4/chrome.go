package gtk4

import (
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/shellkit"
)

// The chrome regions (S4.1h). The aqua driver draws the title bar inside
// the GGSurface, so GTK knows nothing about it: the shell makes the
// RegionCaption parts of the drawable area behave like GTK's own title
// bar (GtkWindowHandle), with two primary-button gestures on the
// GGSurface that window_linux.go adds at creation:
//
//   - A GtkGestureDrag. A press that begins on RegionCaption hands the
//     window to the compositor's (or window manager's) interactive move,
//     gdk_toplevel_begin_move, once the pointer has passed GTK's drag
//     threshold, as GtkWindowHandle does. A press that does not move is
//     left alone, so a double-click can follow.
//   - A GtkGestureClick. The second press of a double-click on
//     RegionCaption (GTK's double-click time and distance) is Zoom, and
//     that press starts no move.
//
// Hit-testing is shellkit.HitRegion over the regions SetChromeRegions set
// last, at the press position in points (GTK's widget coordinates of the
// GGSurface are the drawable area's points). The traffic lights and every
// other region are ordinary pointer events (S4.1i), not touched here. The
// resize borders stay GTK's: they lie in the CSD shadow around the
// GGSurface, outside the drawable area, so no press there reaches it.
//
// Both gestures see a press, in an order GTK does not document, so the
// click and the drag are matched by the press's event time
// (gtk_event_controller_get_current_event_time): either order works. A
// time of 0 (no current event) matches nothing, so such a press is never
// kept from moving.

// captionDrag is the state of the drag gesture's press.
type captionDrag struct {
	on   bool    // a press on RegionCaption that has not moved the window yet
	x, y float64 // where it began, in points
	time uint32  // its event time, ms
}

// captionAt reports whether pos (points) lies in a RegionCaption.
func (sf *surface) captionAt(pos driver.Point) bool {
	k, ok := shellkit.HitRegion(sf.regions, pos)
	return ok && k == shell.RegionCaption
}

// intDoubleDouble implements intDoubleDoublePeer: the pointer gesture's
// "pressed" and "released" (input.go), and GtkGestureClick::pressed of the
// caption click gesture.
func (sf *surface) intDoubleDouble(k kind, instance uintptr, n int32, x, y float64) {
	if !sf.pointerLive() {
		return
	}
	pos := driver.Point{X: x, Y: y}
	switch k {
	case kindPointerPressed:
		sf.press(sf.w.currentButton(instance), pos, instance)
	case kindPointerReleased:
		sf.release(sf.w.currentButton(instance), pos, instance)
	case kindCaptionPress:
		sf.captionPress(instance, n, pos)
	}
}

// captionPress is the caption click gesture's n-th press at pos. The
// second press on the caption zooms the surface (Zoom: only a shown,
// resizable one) and keeps its own drag from moving the window.
func (sf *surface) captionPress(instance uintptr, n int32, pos driver.Point) {
	if n != 2 || !sf.captionAt(pos) {
		return
	}
	t := sf.w.eventTime(instance)
	sf.zoomTime = t
	sf.zooms++
	if sf.drag.on && samePress(sf.drag.time, t) { // the drag gesture saw the press first
		sf.drag.on = false
	}
	sf.Zoom()
}

// doubleDouble implements doubleDoublePeer: the motion controller's
// "enter" and "motion" (input.go), the scroll controller's "decelerate"
// (scroll.go), and GtkGestureDrag::drag-begin
// (x, y the start) and ::drag-update (x, y the offset from the start) of
// the caption drag gesture.
func (sf *surface) doubleDouble(k kind, instance uintptr, x, y float64) {
	if !sf.pointerLive() {
		return
	}
	switch k {
	case kindPointerEnter:
		sf.enterAt(driver.Point{X: x, Y: y}, instance)
	case kindPointerMotion:
		sf.motion(driver.Point{X: x, Y: y}, instance)
	case kindScrollDecelerate:
		sf.decelerate(x, y)
	case kindCaptionDragBegin:
		t := sf.w.eventTime(instance)
		sf.drag = captionDrag{x: x, y: y, time: t,
			on: sf.captionAt(driver.Point{X: x, Y: y}) && !samePress(sf.zoomTime, t)}
	case kindCaptionDragUpdate:
		d := sf.drag
		if !d.on || !sf.w.pastDragThreshold(x, y) {
			return
		}
		sf.drag.on = false
		sf.w.beginMove(instance, d.x, d.y) // a panic in it is the trampoline's
	}
}

// samePress reports whether two event times belong to the same press: equal
// and known (GTK gives 0 without a current event).
func samePress(a, b uint32) bool { return a != 0 && a == b }
