package gtk4

import (
	"math"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// Scroll input (S4.1j). A GtkEventControllerScroll on the GGSurface, for
// both axes and KINETIC, added by window_linux.go at creation:
//
//   - "scroll" (dx, dy) is a ScrollEvent at the pointer's last position.
//     gtk_event_controller_scroll_get_unit tells the units: SURFACE is
//     precise (touchpads, logical pixels = points, 1:1), WHEEL is notches
//     (fractional on high-resolution wheels), which the event carries in
//     lines, wheelLines per notch (GTK has no lines-per-notch setting; 3
//     is Win32's default). GTK's deltas are positive towards later content
//     (natural scrolling already applied), the opposite of
//     ScrollEvent.Delta, so both axes are negated.
//     Shift with a vertical wheel scrolls horizontally, as macOS does (GTK
//     leaves that to the widgets).
//   - "scroll-begin" and "scroll-end" bracket a touchpad gesture: its first
//     precise event is PhaseBegan, the others PhaseChanged, and scroll-end
//     sends PhaseEnded with a zero delta. A precise event outside them has
//     no phase (it scrolls 1:1 without momentum); wheel events never do.
//   - "decelerate" (vx, vy), which GTK emits right after scroll-end, is
//     the release velocity in the deltas' units per second. GTK leaves the
//     kinetic motion to the widget, so the shell synthesizes it as OS
//     momentum (momentum, below): Momentum PhaseBegan, PhaseChanged… on the
//     frame clock's frames, then PhaseEnded, with Phase PhaseNone. The aqua
//     router keeps the gesture's latch through it (S3.5c–f).
//
// Momentum stops, with a Momentum PhaseEnded of zero delta, when a new
// scroll gesture or scroll event comes, or a pointer button is pressed.
// Its events carry the frame times, clamped with the other events' so
// that no time goes backwards (Shell.events).

// wheelLines is the lines a wheel notch scrolls.
const wheelLines = 3

// GdkScrollUnit (gdkenums.h, GTK 4.8+).
const (
	gdkScrollUnitWheel   = 0
	gdkScrollUnitSurface = 1
)

// The momentum's decay (GTK's kinetic scrolling: GtkScrolledWindow's
// deceleration friction): v(t) = v0·e^(−k·t), stopping at stopVelocity;
// release speeds are capped at maxVelocity on each axis.
const (
	momentumDecay = 4.0     // k, per second
	stopVelocity  = 20.0    // points per second
	maxVelocity   = 15000.0 // points per second
)

// momentum is the motion after a release with the velocity v0 (points per
// second, in ScrollEvent.Delta's direction) at the time start: the
// offset v0/k·(1 − e^(−k·t)) on each axis, until the speed falls to
// stopVelocity at stop. The zero value is not running.
type momentum struct {
	on    bool
	v0    driver.Point
	start time.Duration
	stop  time.Duration // how long it runs
	moved driver.Point  // the offset delivered so far
	began bool          // PhaseBegan was sent
}

// newMomentum returns the momentum of a release at start with the velocity
// v (each axis capped at maxVelocity; non-finite values are 0). It is not
// running when the speed is at most stopVelocity.
func newMomentum(v driver.Point, start time.Duration) momentum {
	capV := func(x float64) float64 {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return 0
		}
		return max(-maxVelocity, min(maxVelocity, x))
	}
	v = driver.Point{X: capV(v.X), Y: capV(v.Y)}
	speed := math.Hypot(v.X, v.Y)
	if !(speed > stopVelocity) {
		return momentum{}
	}
	secs := math.Log(speed/stopVelocity) / momentumDecay
	return momentum{on: true, v0: v, start: start, stop: time.Duration(secs * float64(time.Second))}
}

// at returns the offset at the time t (from start on), and whether the
// motion has stopped by then (the offset is then where it stops).
func (m momentum) at(t time.Duration) (driver.Point, bool) {
	el := min(max(t-m.start, 0), m.stop)
	f := -math.Expm1(-momentumDecay*el.Seconds()) / momentumDecay
	return driver.Point{X: m.v0.X * f, Y: m.v0.Y * f}, t-m.start >= m.stop
}

// scrollState is a surface's scrolling.
type scrollState struct {
	gesture bool // between scroll-begin and scroll-end
	began   bool // the gesture's PhaseBegan was sent
	ended   bool // a gesture with PhaseBegan just ended: decelerate may start momentum
	mods    driver.Modifiers
	mom     momentum
}

// scrollEvent delivers ev at the pointer's last position and reports
// whether the surface is still open.
func (sf *surface) scrollEvent(ev shell.ScrollEvent) bool {
	if sf.dead {
		return false
	}
	ev.Pos = sf.ptr.lastPos
	sf.h.Event(ev)
	return !sf.dead
}

// stopMomentum ends a running momentum with a Momentum PhaseEnded of zero
// delta, and reports whether the surface is still open.
func (sf *surface) stopMomentum() bool {
	if !sf.scr.mom.on {
		return !sf.dead
	}
	sf.scr.mom.on = false
	return sf.scrollEvent(shell.ScrollEvent{Precise: true, Momentum: shell.PhaseEnded,
		Mods: sf.scr.mods, Time: sf.s.events.Next(sf.s.clock.now())})
}

// scrollBegin is the scroll controller's "scroll-begin".
func (sf *surface) scrollBegin() {
	if !sf.stopMomentum() {
		return
	}
	sf.scr.gesture, sf.scr.began, sf.scr.ended = true, false, false
}

// scroll is the scroll controller's "scroll" of dx, dy in GTK's units.
func (sf *surface) scroll(dx, dy float64, c uintptr) {
	sf.scr.ended = false
	if !sf.stopMomentum() {
		return
	}
	precise := sf.w.scrollUnit(c) == gdkScrollUnitSurface
	d := driver.Point{X: 0 - dx, Y: 0 - dy} // 0 - 0 is +0, not -0
	if !precise {
		d = driver.Point{X: d.X * wheelLines, Y: d.Y * wheelLines}
	}
	phase := shell.PhaseNone
	if precise && sf.scr.gesture {
		phase = shell.PhaseChanged
		if !sf.scr.began {
			phase, sf.scr.began = shell.PhaseBegan, true
		}
	}
	if d == (driver.Point{}) && phase == shell.PhaseNone {
		return
	}
	sf.scr.mods = modifiersOf(sf.w.eventState(c))
	if !precise && sf.scr.mods&driver.ModShift != 0 && d.X == 0 {
		d.X, d.Y = d.Y, 0 // Shift turns a vertical wheel horizontal, as on macOS
	}
	sf.scrollEvent(shell.ScrollEvent{Delta: d, Precise: precise, Phase: phase,
		Mods: sf.scr.mods, Time: sf.s.eventTime(sf.w.eventTime(c))})
}

// scrollEnd is the scroll controller's "scroll-end": PhaseEnded for a
// gesture that began.
func (sf *surface) scrollEnd(c uintptr) {
	began := sf.scr.gesture && sf.scr.began
	sf.scr.gesture, sf.scr.began, sf.scr.ended = false, false, began
	if began {
		sf.scrollEvent(shell.ScrollEvent{Precise: true, Phase: shell.PhaseEnded,
			Mods: sf.scr.mods, Time: sf.s.eventTime(sf.w.eventTime(c))})
	}
}

// decelerate is the scroll controller's "decelerate" with the release
// velocity vx, vy (GTK's direction): the momentum of the gesture that just
// ended, which the frame clock runs (stepMomentum).
func (sf *surface) decelerate(vx, vy float64) {
	if !sf.scr.ended {
		return
	}
	sf.scr.ended = false
	sf.scr.mom = newMomentum(driver.Point{X: -vx, Y: -vy}, sf.s.clock.now())
	if sf.scr.mom.on {
		sf.s.clock.RequestFrame()
	}
}

// stepMomentum delivers a running momentum's motion up to the frame time
// t: PhaseBegan on its first frame, PhaseChanged on the next ones (a frame
// that adds nothing sends nothing), PhaseEnded with the rest when it
// stops (a momentum that stops within its first frame sends PhaseBegan
// with all of it, then PhaseEnded). It asks for the next frame while it
// runs.
func (sf *surface) stepMomentum(t time.Duration) {
	m := &sf.scr.mom
	if !m.on || sf.dead {
		return
	}
	off, done := m.at(t)
	d := driver.Point{X: off.X - m.moved.X, Y: off.Y - m.moved.Y}
	send := func(d driver.Point, ph shell.ScrollPhase) bool {
		return sf.scrollEvent(shell.ScrollEvent{Delta: d, Precise: true, Momentum: ph,
			Mods: sf.scr.mods, Time: sf.s.events.Next(t)})
	}
	switch {
	case done:
		began := m.began
		*m = momentum{}
		if !began { // stopped within its first frame: Began with all of it
			if send(d, shell.PhaseBegan) {
				send(driver.Point{}, shell.PhaseEnded)
			}
			return
		}
		send(d, shell.PhaseEnded)
		return
	case !m.began:
		m.moved, m.began = off, true
		if !send(d, shell.PhaseBegan) {
			return
		}
	case d == (driver.Point{}):
	default:
		m.moved = off
		if !send(d, shell.PhaseChanged) {
			return
		}
	}
	if sf.scr.mom.on {
		sf.s.clock.RequestFrame()
	}
}

// stepMomentum runs the momentum of every surface for the frame at t,
// before the paint pass queues the draws, so the content it moves is
// drawn in the same frame.
func (s *Shell) stepMomentum(t time.Duration) {
	for _, o := range append([]openSurface(nil), s.surfaces...) {
		if sf, ok := o.(*surface); ok {
			sf.stepMomentum(t)
		}
	}
}
