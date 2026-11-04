package gtk4

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// The caption gestures' host tests (S4.1h), over the fakeWindow: the
// signals go through the real trampolines.

// testChrome is an aqua-like title bar: the caption, a menu title cut out
// of it as client, and a traffic light on top.
var testChrome = []shell.ChromeRegion{
	{Rect: driver.Rect{W: 400, H: 28}, Kind: shell.RegionCaption},
	{Rect: driver.Rect{X: 200, W: 40, H: 28}, Kind: shell.RegionClient},
	{Rect: driver.Rect{X: 8, Y: 8, W: 12, H: 12}, Kind: shell.RegionClose},
}

// openChrome opens a shown surface with testChrome.
func openChrome(t *testing.T, cfg shell.SurfaceConfig) (*surface, *fakeWindow, *sfHandler) {
	t.Helper()
	_, f := newSurfaceTest(t)
	h := &sfHandler{}
	sf, fw := f.open(t, cfg, h)
	sf.Show()
	sf.SetChromeRegions(testChrome)
	fw.take()
	h.take()
	return sf, fw, h
}

// TestCaptionMove: a press on the caption moves the window once it passes
// the drag threshold, from where it began, and only once per press;
// presses on a light, on the client cut-out, below the title bar or with
// no regions move nothing. No handler call comes from any of it.
func TestCaptionMove(t *testing.T) {
	sf, fw, h := openChrome(t, sfResizable)

	fw.dragBegin(100, 10, 1000)
	fw.dragUpdate(3, -4)
	fw.dragUpdate(8, 8) // at the threshold, not past it
	wantLog(t, "within the threshold", fw.take())
	fw.dragUpdate(9, 0)
	wantLog(t, "past the threshold", fw.take(), "beginMove 100,10")
	fw.dragUpdate(40, 0)
	fw.dragUpdate(80, 3)
	wantLog(t, "after the move began", fw.take())

	fw.dragBegin(50.5, 27.75, 1100) // fractional points, the caption's last row
	fw.dragUpdate(0, -9)
	wantLog(t, "a second press", fw.take(), "beginMove 50.5,27.75")

	for _, p := range []driver.Point{{X: 10, Y: 10}, {X: 210, Y: 10}, {X: 100, Y: 28}, {X: 100, Y: 200}, {X: -1, Y: 5}} {
		fw.dragBegin(p.X, p.Y, 1200)
		fw.dragUpdate(30, 30)
		wantLog(t, "a press outside the caption", fw.take())
	}

	sf.SetChromeRegions(nil)
	fw.dragBegin(100, 10, 1300)
	fw.dragUpdate(30, 0)
	wantLog(t, "no regions", fw.take())

	// A press already on the caption keeps moving when the regions change.
	sf.SetChromeRegions(testChrome)
	fw.dragBegin(100, 10, 1400)
	sf.SetChromeRegions(nil)
	fw.dragUpdate(30, 0)
	wantLog(t, "regions changed during the press", fw.take(), "beginMove 100,10")
	wantLog(t, "handler calls", h.take())
}

// TestCaptionDoubleClick: the second press on the caption zooms a shown,
// resizable surface and does not move it, whichever gesture sees the
// press first; the press after it moves again. Single presses, triple
// presses and double-clicks elsewhere do nothing. Event time 0 matches
// nothing.
func TestCaptionDoubleClick(t *testing.T) {
	sf, fw, _ := openChrome(t, sfResizable)

	fw.pressAt(1, 100, 10, 1000)
	fw.dragBegin(100, 10, 1000)
	wantLog(t, "a single press", fw.take())

	// The click gesture first.
	fw.pressAt(2, 100, 10, 1200)
	fw.dragBegin(100, 10, 1200)
	fw.dragUpdate(30, 0)
	wantLog(t, "a double-click, click first", fw.take(), "setMaximized true")
	if !sf.Zoomed() {
		t.Error("not zoomed after a double-click on the caption")
	}

	// The drag gesture first.
	fw.dragBegin(100, 10, 1400)
	fw.pressAt(2, 100, 10, 1400)
	fw.dragUpdate(30, 0)
	wantLog(t, "a double-click, drag first", fw.take(), "setMaximized false")

	fw.dragBegin(100, 10, 1600)
	fw.dragUpdate(30, 0)
	wantLog(t, "the press after", fw.take(), "beginMove 100,10")

	fw.pressAt(3, 100, 10, 1700)
	fw.pressAt(2, 10, 10, 1800)  // a light
	fw.pressAt(2, 210, 10, 1900) // the client cut-out
	fw.pressAt(2, 100, 50, 2000) // the content
	wantLog(t, "other presses", fw.take())

	// Without an event time (0) the presses cannot be matched: the
	// double-click zooms and its drag moves all the same, also after the
	// zoom.
	fw.pressAt(2, 100, 10, 0)
	fw.dragBegin(100, 10, 0)
	fw.dragUpdate(30, 0)
	wantLog(t, "time 0", fw.take(), "setMaximized true", "beginMove 100,10")
	fw.dragBegin(100, 10, 0)
	fw.dragUpdate(0, 30)
	wantLog(t, "time 0, the next press", fw.take(), "beginMove 100,10")
}

// TestCaptionDoubleClickNoZoom: Zoom's rules hold: a fixed-size or hidden
// surface does not zoom, but its double-click press still does not move.
func TestCaptionDoubleClickNoZoom(t *testing.T) {
	_, fw, _ := openChrome(t, sfFixed)
	fw.pressAt(2, 100, 10, 1000)
	fw.dragBegin(100, 10, 1000)
	fw.dragUpdate(30, 0)
	wantLog(t, "fixed size", fw.take())
	fw.dragBegin(100, 10, 1100)
	fw.dragUpdate(30, 0)
	wantLog(t, "fixed size, the next press", fw.take(), "beginMove 100,10")

	_, f := newSurfaceTest(t)
	sf, fw2 := f.open(t, sfResizable, &sfHandler{})
	sf.SetChromeRegions(testChrome)
	fw2.pressAt(2, 100, 10, 1000)
	wantLog(t, "hidden", fw2.take())
}

// TestCaptionAfterClose: after Close the gestures' handles are gone, and
// the peer methods do nothing.
func TestCaptionAfterClose(t *testing.T) {
	sf, fw, h := openChrome(t, sfResizable)
	fw.dragBegin(100, 10, 1000)
	sf.Close()
	fw.take()
	h.take()
	fw.dragUpdate(30, 0)
	fw.pressAt(2, 100, 10, 1100)
	sf.doubleDouble(kindCaptionDragBegin, testDrag, 100, 10)
	sf.doubleDouble(kindCaptionDragUpdate, testDrag, 30, 0)
	sf.intDoubleDouble(kindCaptionPress, testClick, 2, 100, 10)
	wantLog(t, "window calls after Close", fw.take())
	wantLog(t, "handler calls after Close", h.take())
}
