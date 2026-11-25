package gtk4

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// The pointer input's host tests (S4.1i), over the fakeWindow: the motion
// controller's and the click gesture's signals go through the real
// trampolines (fw.ptrEnter … fw.ptrCancel in surface_test.go).

// openPointer opens a hidden 400×300 surface and clears the logs.
func openPointer(t *testing.T) (*surface, *fakeWindow, *sfHandler, *winFactory) {
	t.Helper()
	_, f := newSurfaceTest(t)
	h := &sfHandler{}
	sf, fw := f.open(t, sfResizable, h)
	pastFirstClick(sf, fw)
	fw.take()
	h.take()
	return sf, fw, h, f
}

// pastFirstClick makes the surface key and uses up its first click (S6.1c),
// so a press acts at once.
func pastFirstClick(sf *surface, fw *fakeWindow) {
	fw.isActive = true
	fw.notify(kindActive)
	sf.ptr.armed = false
}

// at is an event at GDK time t of GDK button b, without modifiers.
func at(t, b uint32) fakeEv { return fakeEv{t: t, button: b} }

// catchPanic runs fn and returns what it panicked with.
func catchPanic(fn func()) (v any) {
	defer func() { v = recover() }()
	fn()
	return nil
}

func TestInputPointerButton(t *testing.T) {
	want := map[uint32]shell.PointerButton{
		1: shell.ButtonPrimary, 2: shell.ButtonMiddle, 3: shell.ButtonSecondary,
		8: shell.ButtonBack, 9: shell.ButtonForward,
	}
	for b := uint32(0); b < 16; b++ {
		if got := pointerButton(b); got != want[b] {
			t.Errorf("pointerButton(%d) = %v, want %v", b, got, want[b])
		}
	}
	if got := pointerButton(1 << 31); got != shell.ButtonNone {
		t.Errorf("pointerButton(1<<31) = %v, want ButtonNone", got)
	}
}

func TestInputModifiersOf(t *testing.T) {
	const lock = 1 << 1
	const button1 = 1 << 8 // GDK_BUTTON1_MASK
	for _, c := range []struct {
		state uint32
		want  driver.Modifiers
	}{
		{0, 0},
		{lock, 0},
		{button1 | 1<<9 | 1<<10, 0},
		{gdkShiftMask, driver.ModShift},
		{gdkControlMask, driver.ModControl},
		{gdkAltMask, driver.ModOption},
		{gdkMetaMask, driver.ModOption},
		{gdkAltMask | gdkMetaMask, driver.ModOption},
		{gdkSuperMask, driver.ModCommand},
		{gdkHyperMask, driver.ModCommand},
		{gdkSuperMask | gdkHyperMask, driver.ModCommand},
		{gdkShiftMask | lock | gdkControlMask, driver.ModShift | driver.ModControl},
		{^uint32(0), driver.ModShift | driver.ModControl | driver.ModOption | driver.ModCommand},
	} {
		if got := modifiersOf(c.state); got != c.want {
			t.Errorf("modifiersOf(%#x) = %v, want %v", c.state, got, c.want)
		}
		if modifiersOf(c.state)&driver.ModPrimary != 0 {
			t.Errorf("modifiersOf(%#x) has ModPrimary", c.state)
		}
	}
}

// TestPointerEnterMoveLeave: enter, moves (a repeated position dropped),
// leave at the last position; a motion from outside enters first, also at
// the position of the last leave; a second leave is dropped; modifiers
// come from the event's state.
func TestPointerEnterMoveLeave(t *testing.T) {
	_, fw, h, _ := openPointer(t)
	fw.ptrEnter(1, 2, fakeEv{})
	fw.ptrMotion(1, 2, fakeEv{})
	fw.ptrMotion(3, 4, fakeEv{})
	fw.ptrMotion(3, 4, fakeEv{})
	fw.ptrLeave(fakeEv{})
	fw.ptrLeave(fakeEv{})
	wantLog(t, "enter, moves, leave", h.take(), "enter 1,2", "move 3,4", "leave 3,4")

	fw.ptrMotion(3, 4, fakeEv{})
	fw.ptrMotion(5.5, 6.25, fakeEv{})
	wantLog(t, "motion from outside", h.take(), "enter 3,4", "move 3,4", "move 5.5,6.25")
	fw.ptrLeave(fakeEv{})
	fw.ptrMotion(7, 8, fakeEv{})
	wantLog(t, "motion without enter", h.take(), "leave 5.5,6.25", "enter 7,8", "move 7,8")

	const lock = 1 << 1
	fw.ptrMotion(9, 9, fakeEv{state: gdkShiftMask | gdkControlMask | lock})
	fw.ptrMotion(10, 9, fakeEv{state: gdkMetaMask})
	fw.ptrMotion(11, 9, fakeEv{state: gdkHyperMask})
	fw.ptrMotion(12, 9, fakeEv{state: lock})
	fw.ptrLeave(fakeEv{state: gdkAltMask | gdkSuperMask})
	wantLog(t, "modifiers", h.take(), "move 9,9 m=3", "move 10,9 m=4", "move 11,9 m=8", "move 12,9", "leave 12,9 m=12")
	wantLog(t, "window calls", fw.take())
}

// TestPointerClicks: presses and releases with the held buttons and click
// counts: double and triple clicks within the time and the distance,
// broken by time, by distance and by another button; a press from outside
// enters first.
func TestPointerClicks(t *testing.T) {
	_, fw, h, _ := openPointer(t)
	fw.ptrPressed(1, 10, 20, at(7000, 1))
	fw.ptrReleased(1, 10, 20, at(7050, 1))
	fw.ptrPressed(2, 14, 24, at(7100, 1)) // 4 away, 100 ms
	fw.ptrReleased(2, 14, 24, at(7150, 1))
	fw.ptrPressed(3, 10, 20, at(7500, 1)) // 4 away from the last, 400 ms
	fw.ptrReleased(3, 10, 20, at(7550, 1))
	wantLog(t, "triple click", h.take(),
		"enter 10,20", "down 10,20 b=1 held=1 n=1", "up 10,20 b=1 held=0 n=1",
		"down 14,24 b=1 held=1 n=2", "up 14,24 b=1 held=0 n=2",
		"down 10,20 b=1 held=1 n=3", "up 10,20 b=1 held=0 n=3")

	fw.ptrPressed(1, 10, 20, at(7901, 1)) // 401 ms
	fw.ptrReleased(1, 10, 20, at(7950, 1))
	fw.ptrPressed(1, 16, 20, at(8000, 1)) // 6 away
	fw.ptrReleased(1, 16, 20, at(8010, 1))
	fw.ptrPressed(1, 16, 14, at(8020, 1)) // 6 away
	fw.ptrReleased(1, 16, 14, at(8030, 1))
	fw.ptrPressed(1, 16, 14, at(8040, 3)) // another button
	fw.ptrReleased(1, 16, 14, at(8050, 3))
	fw.ptrPressed(1, 16, 14, at(8060, 1))
	fw.ptrReleased(1, 16, 14, at(8070, 1))
	fw.ptrPressed(1, 16, 14, at(8080, 2))
	fw.ptrReleased(1, 16, 14, at(8090, 2))
	fw.ptrPressed(1, 16, 14, at(8100, 2))
	fw.ptrReleased(1, 16, 14, at(8110, 2))
	wantLog(t, "broken series", h.take(),
		"down 10,20 b=1 held=1 n=1", "up 10,20 b=1 held=0 n=1",
		"down 16,20 b=1 held=1 n=1", "up 16,20 b=1 held=0 n=1",
		"down 16,14 b=1 held=1 n=1", "up 16,14 b=1 held=0 n=1",
		"down 16,14 b=2 held=2 n=1", "up 16,14 b=2 held=0 n=1",
		"down 16,14 b=1 held=1 n=1", "up 16,14 b=1 held=0 n=1",
		"down 16,14 b=4 held=4 n=1", "up 16,14 b=4 held=0 n=1",
		"down 16,14 b=4 held=4 n=2", "up 16,14 b=4 held=0 n=2")

	fw.ptrPressed(1, 16, 14, at(9000, 8))
	fw.ptrReleased(1, 16, 14, at(9010, 8))
	fw.ptrPressed(1, 16, 14, at(9020, 9))
	fw.ptrReleased(1, 16, 14, fakeEv{t: 9030, button: 9, state: gdkShiftMask})
	wantLog(t, "back and forward", h.take(),
		"down 16,14 b=8 held=8 n=1", "up 16,14 b=8 held=0 n=1",
		"down 16,14 b=16 held=16 n=1", "up 16,14 b=16 held=0 n=1 m=1")

	// A second press of a held button is dropped.
	fw.ptrPressed(1, 16, 14, at(19500, 1))
	fw.ptrPressed(1, 16, 14, at(19510, 1))
	fw.ptrReleased(1, 16, 14, at(9520, 1))
	wantLog(t, "a held button pressed again", h.take(),
		"down 16,14 b=1 held=1 n=1", "up 16,14 b=1 held=0 n=1")
}

// TestPointerUnknownButtons: presses and releases of buttons the shell
// does not know are dropped, also without an enter first.
func TestPointerUnknownButtons(t *testing.T) {
	_, fw, h, _ := openPointer(t)
	for _, b := range []uint32{0, 4, 5, 6, 7, 10, 255} {
		fw.ptrPressed(1, 5, 5, at(7000, b))
		fw.ptrReleased(1, 5, 5, at(7010, b))
	}
	fw.ptrCancel(fakeEv{})
	wantLog(t, "unknown buttons", h.take())
	// The pointer is still outside: a motion enters.
	fw.ptrMotion(5, 5, fakeEv{})
	wantLog(t, "after", h.take(), "enter 5,5", "move 5,5")
}

// TestPointerReleaseFallback: a release that does not name a held button
// (0, unknown, or another one) is the release of the held button; a
// release with nothing held is dropped.
func TestPointerReleaseFallback(t *testing.T) {
	_, fw, h, _ := openPointer(t)
	fw.ptrEnter(5, 5, fakeEv{})
	for _, rb := range []uint32{0, 4, 1} {
		fw.ptrPressed(1, 5, 5, at(7000, 3))
		fw.ptrReleased(1, 6, 7, at(7010, rb))
		fw.ptrMotion(5, 5, fakeEv{})
	}
	fw.ptrReleased(1, 5, 5, at(7100, 0))
	fw.ptrReleased(1, 5, 5, at(7110, 1))
	wantLog(t, "fallback", h.take(),
		"enter 5,5",
		"down 5,5 b=2 held=2 n=1", "up 6,7 b=2 held=0 n=1", "move 5,5",
		"down 5,5 b=2 held=2 n=2", "up 6,7 b=2 held=0 n=2", "move 5,5",
		"down 5,5 b=2 held=2 n=3", "up 6,7 b=2 held=0 n=3", "move 5,5")
}

// TestPointerCapture: a leave while a button is held is kept until the
// last release, also when the release is inside; a re-enter while held
// undoes it; a release outside the size leaves after the Up; moves
// outside are delivered while held.
func TestPointerCapture(t *testing.T) {
	_, fw, h, _ := openPointer(t)
	fw.ptrEnter(50, 50, fakeEv{})
	fw.ptrPressed(1, 50, 50, at(7000, 1))
	fw.ptrLeave(fakeEv{})
	wantLog(t, "leave while held", h.take(), "enter 50,50", "down 50,50 b=1 held=1 n=1")
	fw.ptrReleased(1, 50, 50, at(7010, 1))
	wantLog(t, "the release", h.take(), "up 50,50 b=1 held=0 n=1", "leave 50,50")

	fw.ptrMotion(60, 60, fakeEv{})
	fw.ptrPressed(1, 60, 60, at(8000, 1))
	fw.ptrLeave(fakeEv{})
	fw.ptrEnter(61, 61, fakeEv{})
	fw.ptrReleased(1, 61, 61, at(8010, 1))
	fw.ptrLeave(fakeEv{})
	wantLog(t, "re-enter while held", h.take(), "enter 60,60", "move 60,60",
		"down 60,60 b=1 held=1 n=1", "up 61,61 b=1 held=0 n=1", "leave 61,61")

	for i, p := range []driver.Point{{X: -1, Y: 10}, {X: 400, Y: 10}, {X: 10, Y: 300}, {X: 10, Y: -0.5}} {
		fw.ptrMotion(20, 20, fakeEv{})
		fw.ptrPressed(1, 20, 20, at(9000+1000*uint32(i), 1))
		fw.ptrMotion(p.X, p.Y, fakeEv{})
		fw.ptrReleased(1, p.X, p.Y, at(9010+1000*uint32(i), 1))
		pos := formatPos(p)
		wantLog(t, "release outside "+pos, h.take(), "enter 20,20", "move 20,20",
			"down 20,20 b=1 held=1 n=1", "move "+pos, "up "+pos+" b=1 held=0 n=1", "leave "+pos)
	}
	// The release at the last row and column is inside.
	fw.ptrMotion(20, 20, fakeEv{})
	fw.ptrPressed(1, 20, 20, at(19500, 1))
	fw.ptrReleased(1, 399.5, 299.5, at(19510, 1))
	wantLog(t, "release on the edge", h.take(), "enter 20,20", "move 20,20",
		"down 20,20 b=1 held=1 n=1", "up 399.5,299.5 b=1 held=0 n=1")
}

// formatPos formats p as sfHandler logs it.
func formatPos(p driver.Point) string { return fmt.Sprintf("%v,%v", p.X, p.Y) }

// TestPointerCancel: a cancel ends every held button with a PointerUp at
// the last position (lowest button first), then sends the leave kept
// while they were held; with nothing held it does nothing.
func TestPointerCancel(t *testing.T) {
	_, fw, h, _ := openPointer(t)
	fw.ptrCancel(fakeEv{})
	fw.ptrEnter(10, 10, fakeEv{})
	fw.ptrCancel(fakeEv{})
	fw.ptrPressed(1, 10, 10, at(7000, 1))
	fw.ptrMotion(20, 30, fakeEv{})
	fw.ptrCancel(fakeEv{state: gdkShiftMask})
	fw.ptrReleased(1, 20, 30, at(7010, 1)) // GTK sends none, but it is dropped
	wantLog(t, "cancel inside", h.take(), "enter 10,10", "down 10,10 b=1 held=1 n=1",
		"move 20,30", "up 20,30 b=1 held=0 n=1 m=1")

	fw.ptrPressed(1, 20, 30, at(8000, 1))
	fw.ptrMotion(25, 35, fakeEv{})
	fw.ptrLeave(fakeEv{})
	fw.ptrCancel(fakeEv{})
	fw.ptrLeave(fakeEv{})
	wantLog(t, "cancel after a leave", h.take(), "down 20,30 b=1 held=1 n=1",
		"move 25,35", "up 25,35 b=1 held=0 n=1", "leave 25,35")

	// Two held buttons (the state allows it, though GTK resets the first):
	// one Up each, lowest first, with the counts of their presses.
	fw.ptrMotion(5, 5, fakeEv{})
	fw.ptrPressed(1, 5, 5, at(9000, 3))
	fw.ptrPressed(1, 5, 5, at(9010, 1))
	fw.ptrLeave(fakeEv{})
	fw.ptrCancel(fakeEv{})
	wantLog(t, "cancel of two", h.take(), "enter 5,5", "move 5,5",
		"down 5,5 b=2 held=2 n=1", "down 5,5 b=1 held=3 n=1",
		"up 5,5 b=1 held=2 n=1", "up 5,5 b=2 held=0 n=1", "leave 5,5")
}

// TestPointerOutsideWhileHeld: a move outside while a button is held keeps
// the leave GTK reported (only a move back inside undoes it), and a cancel
// with the last position outside leaves, also without GTK's leave.
func TestPointerOutsideWhileHeld(t *testing.T) {
	_, fw, h, _ := openPointer(t)
	fw.ptrEnter(10, 10, fakeEv{})
	fw.ptrPressed(1, 10, 10, at(7000, 1))
	fw.ptrLeave(fakeEv{})
	fw.ptrMotion(-10, 50, fakeEv{}) // the grab's motion outside
	fw.ptrCancel(fakeEv{})
	wantLog(t, "leave, move outside, cancel", h.take(), "enter 10,10", "down 10,10 b=1 held=1 n=1",
		"move -10,50", "up -10,50 b=1 held=0 n=1", "leave -10,50")

	// No leave from GTK: the cancel outside leaves all the same.
	fw.ptrMotion(10, 10, fakeEv{})
	fw.ptrPressed(1, 10, 10, at(8000, 1))
	fw.ptrMotion(500, 10, fakeEv{})
	fw.ptrCancel(fakeEv{})
	wantLog(t, "move outside, cancel", h.take(), "enter 10,10", "move 10,10",
		"down 10,10 b=1 held=1 n=1", "move 500,10", "up 500,10 b=1 held=0 n=1", "leave 500,10")

	// Leave, out and back in: the leave is undone; cancel and release
	// inside then leave nothing.
	fw.ptrMotion(10, 10, fakeEv{})
	fw.ptrPressed(1, 10, 10, at(9000, 1))
	fw.ptrLeave(fakeEv{})
	fw.ptrMotion(-5, 10, fakeEv{})
	fw.ptrMotion(5, 10, fakeEv{})
	fw.ptrCancel(fakeEv{})
	fw.ptrPressed(1, 5, 10, at(10000, 1))
	fw.ptrLeave(fakeEv{})
	fw.ptrMotion(5, 400, fakeEv{})
	fw.ptrMotion(5, 20, fakeEv{})
	fw.ptrReleased(1, 5, 20, at(10010, 1))
	wantLog(t, "out and back in", h.take(), "enter 10,10", "move 10,10",
		"down 10,10 b=1 held=1 n=1", "move -5,10", "move 5,10", "up 5,10 b=1 held=0 n=1",
		"down 5,10 b=1 held=1 n=1", "move 5,400", "move 5,20", "up 5,20 b=1 held=0 n=1")

	// Leave and out, then a release GTK places inside: the leave stays.
	fw.ptrPressed(1, 5, 20, at(11000, 1))
	fw.ptrLeave(fakeEv{})
	fw.ptrMotion(-5, 20, fakeEv{})
	fw.ptrReleased(1, 5, 20, at(11010, 1))
	wantLog(t, "release inside after a move out", h.take(), "down 5,20 b=1 held=1 n=1",
		"move -5,20", "up 5,20 b=1 held=0 n=1", "leave 5,20")

	// A cancel inside with nothing held and no leave sends nothing.
	fw.ptrMotion(5, 5, fakeEv{})
	fw.ptrCancel(fakeEv{})
	wantLog(t, "cancel, nothing held", h.take(), "enter 5,5", "move 5,5")
}

// TestPointerStaleCaption: a caption press whose release and cancel never
// came ends at the next press: a press on the content is delivered, its
// leave is not kept for the old caption press, and a new caption press is
// again not delivered.
func TestPointerStaleCaption(t *testing.T) {
	_, fw, h := openChrome(t, sfResizable)
	fw.ptrMotion(100, 10, fakeEv{})
	fw.ptrPressed(1, 100, 10, at(7000, 1)) // no release follows
	fw.ptrPressed(1, 100, 100, at(7100, 1))
	fw.ptrReleased(1, 100, 100, at(7110, 1))
	fw.ptrLeave(fakeEv{})
	wantLog(t, "after a stale caption press", h.take(), "enter 100,10", "move 100,10",
		"down 100,100 b=1 held=1 n=1", "up 100,100 b=1 held=0 n=1", "leave 100,100")

	fw.ptrMotion(100, 10, fakeEv{})
	fw.ptrPressed(1, 100, 10, at(8000, 1))
	fw.ptrPressed(1, 150, 10, at(8100, 1)) // a new caption press
	fw.ptrReleased(1, 150, 10, at(8110, 1))
	fw.ptrLeave(fakeEv{})
	wantLog(t, "a caption press after a stale one", h.take(), "enter 100,10", "move 100,10", "leave 100,10")

	// A press of an unknown button ends it too: the next leave is not kept.
	fw.ptrMotion(100, 10, fakeEv{})
	fw.ptrPressed(1, 100, 10, at(9000, 1))
	fw.ptrPressed(1, 100, 10, at(9100, 4))
	fw.ptrLeave(fakeEv{})
	wantLog(t, "ended by an unknown button", h.take(), "enter 100,10", "move 100,10", "leave 100,10")

	// A secondary press after a stale caption press is delivered, and the
	// primary's late release is not the secondary's.
	fw.ptrMotion(100, 10, fakeEv{})
	fw.ptrPressed(1, 100, 10, at(10000, 1))
	fw.ptrPressed(1, 100, 10, at(10100, 3))
	fw.ptrReleased(1, 100, 10, at(10110, 3))
	wantLog(t, "secondary after a stale caption press", h.take(), "enter 100,10", "move 100,10",
		"down 100,10 b=2 held=2 n=1", "up 100,10 b=2 held=0 n=1")
}

// TestPointerCaption: a primary press on the caption is not delivered,
// nor its release or cancel, and breaks a click series; moves and hover
// over the caption are delivered, and a leave during it is kept until it
// ends; a secondary press on the caption and primary presses on the light
// and the client cut-out are delivered.
func TestPointerCaption(t *testing.T) {
	_, fw, h := openChrome(t, sfResizable)
	fw.ptrMotion(100, 10, fakeEv{})
	fw.ptrPressed(1, 100, 10, at(7000, 1))
	fw.ptrMotion(150, 12, fakeEv{})
	fw.ptrReleased(1, 150, 12, at(7010, 1))
	fw.ptrPressed(1, 150, 12, at(7020, 1))
	fw.ptrCancel(fakeEv{})
	wantLog(t, "caption press", h.take(), "enter 100,10", "move 100,10", "move 150,12")

	fw.ptrPressed(1, 150, 12, at(7100, 1))
	fw.ptrLeave(fakeEv{})
	wantLog(t, "leave during a caption press", h.take())
	fw.ptrReleased(1, 150, 12, at(7110, 1))
	wantLog(t, "its release", h.take(), "leave 150,12")

	fw.ptrPressed(1, 100, 10, at(7200, 3))
	fw.ptrReleased(1, 100, 10, at(7210, 3))
	fw.ptrPressed(1, 10, 10, at(7300, 1))
	fw.ptrReleased(1, 10, 10, at(7310, 1))
	fw.ptrPressed(1, 210, 10, at(7400, 1))
	fw.ptrReleased(1, 210, 10, at(7410, 1))
	wantLog(t, "delivered presses", h.take(), "enter 100,10",
		"down 100,10 b=2 held=2 n=1", "up 100,10 b=2 held=0 n=1",
		"down 10,10 b=1 held=1 n=1", "up 10,10 b=1 held=0 n=1",
		"down 210,10 b=1 held=1 n=1", "up 210,10 b=1 held=0 n=1")

	// A caption press between two presses at the same place breaks the
	// series, and its cancel leaves nothing held.
	fw.ptrPressed(1, 100, 100, at(8000, 1))
	fw.ptrReleased(1, 100, 100, at(8010, 1))
	fw.ptrPressed(1, 100, 10, at(8050, 1))
	fw.ptrCancel(fakeEv{})
	fw.ptrPressed(1, 100, 100, at(8100, 1))
	fw.ptrReleased(1, 100, 100, at(8110, 1))
	fw.ptrPressed(1, 100, 100, at(8200, 1))
	fw.ptrReleased(1, 100, 100, at(8210, 1))
	wantLog(t, "series broken by the caption", h.take(),
		"down 100,100 b=1 held=1 n=1", "up 100,100 b=1 held=0 n=1",
		"down 100,100 b=1 held=1 n=1", "up 100,100 b=1 held=0 n=1",
		"down 100,100 b=1 held=1 n=2", "up 100,100 b=1 held=0 n=2")
}

// TestPointerEventTimes: an event's time is its age before now on the
// clock (GDK's ms against g_get_monotonic_time); time 0, future times and
// ages beyond MaxEventAge are now; times never go backwards.
func TestPointerEventTimes(t *testing.T) {
	_, fw, h, f := openPointer(t)
	const ms = time.Millisecond
	const t0 = 2_000_456 * time.Microsecond // Now at the first events
	f.l.now = clockStart + t0.Microseconds()
	tick := uint32(f.l.now / 1000) // 9000
	fw.ptrMotion(1, 1, fakeEv{t: tick - 30})
	fw.ptrMotion(2, 1, fakeEv{t: tick})
	fw.ptrMotion(3, 1, fakeEv{t: tick - 40}) // older than the last
	f.l.now += 1_000_000
	tick += 1000
	fw.ptrMotion(4, 1, fakeEv{t: tick - 30})
	fw.ptrMotion(5, 1, fakeEv{t: 0})
	f.l.now += 1_000_000
	tick += 1000
	fw.ptrMotion(6, 1, fakeEv{t: tick + 10}) // in the future
	f.l.now += 10_000_000
	tick += 10_000
	fw.ptrPressed(1, 6, 1, at(tick-5000, 1)) // MaxEventAge old: still trusted
	fw.ptrReleased(1, 6, 1, at(tick-5001, 1))
	h.take()
	want := []time.Duration{
		t0 - 30*ms, t0 - 30*ms, t0, t0,
		t0 + time.Second - 30*ms, t0 + time.Second,
		t0 + 2*time.Second,
		t0 + 7*time.Second, t0 + 12*time.Second,
	}
	if len(h.times) != len(want) {
		t.Fatalf("times %v, want %v", h.times, want)
	}
	for i := range want {
		if h.times[i] != want[i] {
			t.Errorf("time %d = %v, want %v", i, h.times[i], want[i])
		}
	}
}

// TestPointerCloseInDown: a handler that closes the surface in its
// PointerDown gets no further events, and the window is not used after
// destroy (the factory checks it at cleanup).
func TestPointerCloseInDown(t *testing.T) {
	sf, fw, h, _ := openPointer(t)
	h.onEvent = func(pe shell.PointerEvent) {
		if pe.Kind == shell.PointerDown {
			sf.Close()
		}
	}
	fw.ptrPressed(1, 5, 5, at(7000, 1))
	fw.ptrMotion(6, 6, fakeEv{})
	fw.ptrLeave(fakeEv{})
	fw.ptrReleased(1, 6, 6, at(7010, 1))
	fw.ptrCancel(fakeEv{})
	wantLog(t, "close in PointerDown", h.take(), "enter 5,5", "down 5,5 b=1 held=1 n=1", "closed")
	wantLog(t, "window calls", fw.take(), "destroy")

	// Closing in the Enter of a press: no Down.
	sf2, fw2, h2, _ := openPointer(t)
	h2.onEvent = func(pe shell.PointerEvent) {
		if pe.Kind == shell.PointerEnter {
			sf2.Close()
		}
	}
	fw2.ptrPressed(1, 5, 5, at(7000, 1))
	wantLog(t, "close in the press's enter", h2.take(), "enter 5,5", "closed")

	// Closing in an Up with a leave kept: no leave.
	sf3, fw3, h3, _ := openPointer(t)
	h3.onEvent = func(pe shell.PointerEvent) {
		if pe.Kind == shell.PointerUp {
			sf3.Close()
		}
	}
	fw3.ptrPressed(1, 5, 5, at(7000, 1))
	fw3.ptrLeave(fakeEv{})
	fw3.ptrReleased(1, 5, 5, at(7010, 1))
	wantLog(t, "close in an Up", h3.take(), "enter 5,5", "down 5,5 b=1 held=1 n=1",
		"up 5,5 b=1 held=0 n=1", "closed")
}

// TestPointerCloseInCancel: closing in the first Up of a cancel ends it.
func TestPointerCloseInCancel(t *testing.T) {
	sf, fw, h, _ := openPointer(t)
	h.onEvent = func(pe shell.PointerEvent) {
		if pe.Kind == shell.PointerUp {
			sf.Close()
		}
	}
	fw.ptrPressed(1, 5, 5, at(7000, 3))
	fw.ptrPressed(1, 5, 5, at(7010, 1))
	fw.ptrLeave(fakeEv{})
	fw.ptrCancel(fakeEv{})
	fw.ptrCancel(fakeEv{})
	wantLog(t, "close in cancel", h.take(), "enter 5,5",
		"down 5,5 b=2 held=2 n=1", "down 5,5 b=1 held=3 n=1",
		"up 5,5 b=1 held=2 n=1", "closed")
	wantLog(t, "window calls", fw.take(), "destroy")
}

// TestPointerDuringCreate: the pointer signals while the window is being
// created deliver nothing and leave the pointer outside.
func TestPointerDuringCreate(t *testing.T) {
	_, f := newSurfaceTest(t)
	h := &sfHandler{}
	f.during = func(fw *fakeWindow) {
		fw.ptrEnter(1, 1, fakeEv{})
		fw.ptrMotion(2, 2, fakeEv{})
		fw.ptrPressed(1, 2, 2, at(7000, 1))
		fw.ptrLeave(fakeEv{})
		fw.ptrReleased(1, 2, 2, at(7010, 1))
		fw.ptrPressed(1, 2, 2, at(7020, 1))
		fw.ptrCancel(fakeEv{})
	}
	_, fw := f.open(t, sfResizable, h)
	f.during = nil
	wantLog(t, "handler calls during creation", h.take())
	fw.ptrMotion(2, 2, fakeEv{})
	fw.ptrPressed(1, 2, 2, at(7100, 1))
	fw.ptrReleased(1, 2, 2, at(7110, 1))
	wantLog(t, "after creation", h.take(), "enter 2,2", "move 2,2",
		"down 2,2 b=1 held=1 n=1 activating", "up 2,2 b=1 held=0 n=1") // not key: the first click (S6.1c)
}

// TestPointerAfterClose: after Close neither the signals nor the peer
// methods deliver anything or use the window.
func TestPointerAfterClose(t *testing.T) {
	sf, fw, h, _ := openPointer(t)
	fw.ptrEnter(5, 5, fakeEv{})
	fw.ptrPressed(1, 5, 5, at(7000, 1))
	sf.Close()
	fw.take()
	h.take()
	fw.ptrMotion(6, 6, fakeEv{})
	fw.ptrLeave(fakeEv{})
	fw.ptrReleased(1, 6, 6, at(7010, 1))
	fw.ptrCancel(fakeEv{})
	sf.doubleDouble(kindPointerEnter, testMotion, 1, 1)
	sf.doubleDouble(kindPointerMotion, testMotion, 2, 2)
	sf.voidSignal(kindPointerLeave, testMotion)
	sf.intDoubleDouble(kindPointerPressed, testPointer, 1, 2, 2)
	sf.intDoubleDouble(kindPointerReleased, testPointer, 1, 2, 2)
	sf.pointerSignal(kindPointerCancel, testPointer, 0)
	wantLog(t, "handler calls after Close", h.take())
	wantLog(t, "window calls after Close", fw.take())
}

// TestPointerPanic: a panicking handler is re-raised by rethrow as the
// trampoline's *PanicError; the pointer state stays usable.
func TestPointerPanic(t *testing.T) {
	sf, fw, h, _ := openPointer(t)
	h.onEvent = func(pe shell.PointerEvent) {
		if pe.Kind == shell.PointerDown {
			panic("in PointerDown")
		}
	}
	v := catchPanic(func() { fw.ptrPressed(1, 5, 5, at(7000, 1)) })
	pe, ok := v.(*PanicError)
	if !ok || pe.Value != "in PointerDown" || pe.Callback != nameSigIntDoubleDouble {
		t.Fatalf("press panicked with %#v, want the *PanicError of sigIntDoubleDouble", v)
	}
	if pendingPanic != nil || sf.dead {
		t.Fatalf("after rethrow: pending %v, dead %v", pendingPanic, sf.dead)
	}
	h.onEvent = nil
	fw.ptrMotion(19, 9, fakeEv{})
	fw.ptrReleased(1, 19, 9, at(7010, 1))
	fw.ptrPressed(1, 19, 9, at(7100, 1))
	fw.ptrReleased(1, 19, 9, at(7110, 1))
	fw.ptrLeave(fakeEv{})
	wantLog(t, "after a panic in Down", h.take(), "enter 5,5", "down 5,5 b=1 held=1 n=1",
		"move 19,9", "up 19,9 b=1 held=0 n=1",
		"down 19,9 b=1 held=1 n=1", "up 19,9 b=1 held=0 n=1", "leave 19,9")

	// A panic in the enter of a motion: the move is lost, the next one
	// arrives without a second enter.
	h.onEvent = func(pe shell.PointerEvent) {
		if pe.Kind == shell.PointerEnter {
			panic("in PointerEnter")
		}
	}
	v = catchPanic(func() { fw.ptrMotion(20, 20, fakeEv{}) })
	if pe, ok := v.(*PanicError); !ok || pe.Value != "in PointerEnter" || pe.Callback != nameSigDoubleDouble {
		t.Fatalf("motion panicked with %#v", v)
	}
	h.onEvent = nil
	fw.ptrMotion(21, 20, fakeEv{})
	wantLog(t, "after a panic in Enter", h.take(), "enter 20,20", "move 21,20")

	// A panic in a cancel's Up: rethrown from the cancel trampoline.
	h.onEvent = func(pe shell.PointerEvent) {
		if pe.Kind == shell.PointerUp {
			panic("in PointerUp")
		}
	}
	fw.ptrPressed(1, 21, 20, at(8000, 1))
	v = catchPanic(func() { fw.ptrCancel(fakeEv{}) })
	if pe, ok := v.(*PanicError); !ok || pe.Value != "in PointerUp" || pe.Callback != nameSigPointer {
		t.Fatalf("cancel panicked with %#v", v)
	}
	h.onEvent = nil
	fw.ptrCancel(fakeEv{})
	fw.ptrPressed(1, 21, 20, at(9000, 1))
	fw.ptrReleased(1, 21, 20, at(9010, 1))
	wantLog(t, "after a panic in cancel", h.take(), "down 21,20 b=1 held=1 n=1",
		"up 21,20 b=1 held=0 n=1", "down 21,20 b=1 held=1 n=1", "up 21,20 b=1 held=0 n=1")
	if pendingPanic != nil {
		t.Errorf("pending panic left: %v", pendingPanic)
	}
	wantLog(t, "window calls", fw.take())
}

// TestPointerFirstClick (S6.1c): the press that activates a surface is
// Activating, whether it comes while the surface is not key (once: a
// second press while still not key acts) or within firstClickWindow after
// is-active turned true; its release, later presses, a press long after a
// keyboard activation and a press after a caption press are not marked;
// any button activates; a key loss resets the rule.
func TestPointerFirstClick(t *testing.T) {
	sf, fw, h := openChrome(t, sfResizable) // not key
	key := func(on bool) {
		fw.isActive = on
		fw.notify(kindActive)
	}
	click := func(b uint32, t0 uint32) {
		fw.ptrPressed(1, 100, 100, at(t0, b))
		fw.ptrReleased(1, 100, 100, at(t0+5, b))
	}

	// The press comes before the activation (Wayland, usually).
	fw.ptrMotion(100, 100, fakeEv{})
	click(1, 1000)
	key(true)
	click(1, 2000)
	wantLog(t, "press, then the activation", h.take(), "enter 100,100", "move 100,100",
		"down 100,100 b=1 held=1 n=1 activating", "up 100,100 b=1 held=0 n=1",
		"key true", "down 100,100 b=1 held=1 n=1", "up 100,100 b=1 held=0 n=1")

	// The activation comes before the press (X11, usually); any button.
	key(false)
	key(true)
	click(3, 3000)
	click(3, 4000)
	wantLog(t, "the activation, then the press", h.take(), "key false", "key true",
		"down 100,100 b=2 held=2 n=1 activating", "up 100,100 b=2 held=0 n=1",
		"down 100,100 b=2 held=2 n=1", "up 100,100 b=2 held=0 n=1")

	// A press long after the activation (the keyboard's) acts.
	key(false)
	key(true)
	sf.ptr.armedAt -= firstClickWindow + time.Millisecond
	click(1, 5000)
	wantLog(t, "a press after the window", h.take(), "key false", "key true",
		"down 100,100 b=1 held=1 n=1", "up 100,100 b=1 held=0 n=1")

	// No activation comes (the window manager does not activate on click):
	// the second press acts.
	key(false)
	click(1, 6000)
	click(1, 7000)
	wantLog(t, "never activated", h.take(), "key false",
		"down 100,100 b=1 held=1 n=1 activating", "up 100,100 b=1 held=0 n=1",
		"down 100,100 b=1 held=1 n=1", "up 100,100 b=1 held=0 n=1")

	// The caption press activates (the window manager's move); the next
	// press acts, before and after the activation.
	key(true)
	key(false)
	fw.ptrPressed(1, 150, 10, at(8000, 1))
	fw.ptrReleased(1, 150, 10, at(8005, 1))
	key(true)
	click(1, 9000)
	key(false)
	key(true)
	fw.ptrPressed(1, 150, 10, at(10000, 1))
	fw.ptrReleased(1, 150, 10, at(10005, 1))
	click(1, 11000)
	got := h.take()
	for _, l := range got {
		if strings.Contains(l, "activating") {
			t.Errorf("after a caption press: %q", got)
			break
		}
	}
}
