//go:build windows

package win32

import (
	"iter"
	"time"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/shellkit"
	"github.com/radix29/gogui/internal/shell/win32/d2d"
	"github.com/radix29/gogui/internal/shell/win32/dcomp"
	"github.com/radix29/gogui/internal/shell/win32/dxgi"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// swapTarget is a window with its own composition swap chain: the state a
// surface and a popup share (review R8), embedded in both. It holds the
// window, its DPI and size, the rendering objects (present.go), the
// pacing state of its chain's waitable (pacing.go) and the precise-scroll
// gesture (wheel.go). What differs between the two goes through win: the
// content a paint draws, the color it clears to and where scroll events
// go.
type swapTarget struct {
	s   *Shell
	win window // the surface or popup this is embedded in
	id  ffi.Handle
	// hwnd is the window; WM_NCCREATE sets it before CreateWindowExW returns.
	hwnd windows.HWND

	size  driver.Size // the size last reported (or the initial one), in the window's points
	want  driver.Size // the size asked for last, see shellkit.FromPx (surface.go)
	scale float64     // the scale last reported (or the initial one)
	dpi   uint32      // the window's DPI, which converts pixels (px)

	dead    bool // WM_DESTROY has run (a popup: closed)
	invalid bool // waits for a paint

	// Scrolling (wheel.go).
	gesture   shellkit.Gesture
	scrollPos driver.Point // the position of the last precise scroll event

	// Rendering (present.go): the swap chain, its composition target and
	// visual, and the D2D target bitmap over the back buffer, each one
	// reference; the chain's pixel size and the bitmap's DPI.
	chain     dxgi.SwapChain1
	ctarget   dcomp.Target
	visual    dcomp.Visual
	bitmap    d2d.Bitmap1
	backdrop  backdrop // the backdrop blur's scratch bitmap (blur.go)
	pxW, pxH  uint32
	bitmapDPI uint32
	paintErr  error // the recorder errors of the last paint (Shell.debug)
	renderErr error // the rendering error of the last paint or resize (tests)

	// Pacing (pacing.go): the chain's frame-latency waitable object (0
	// without one), a signal taken from it and not yet spent by a Present,
	// the Presents it has not signaled for yet (it signals once for every
	// Present), and the Presents (tests).
	waitable windows.Handle
	ticket   bool
	debt     int
	presents int

	// Presentation feedback (pacing.go, only with a present observer): the
	// latest presents' frame targets by the chain's present count, and the
	// last present count reported shown.
	presentLog [presentLogSize]presentRecord
	lastShown  uint32
}

// window is what embeds a swapTarget: a surface or a popup.
type window interface {
	target
	// swap returns the embedded swap target.
	swap() *swapTarget
	// paintContent runs the handler's Paint.
	paintContent(c *canvas)
	// background is the color a frame is cleared to before Paint.
	background() d2d.ColorF
	// scrollEvent delivers a scroll event from the window (wheel.go).
	scrollEvent(ev shell.ScrollEvent)
}

var (
	_ window = (*surface)(nil)
	_ window = (*popup)(nil)
)

func (t *swapTarget) swap() *swapTarget { return t }

// px returns the pixels per point of the window's DPI, which converts
// sizes. For a surface it differs from scale (the value last reported)
// only inside a WM_DPICHANGED, before sync reports the new one.
func (t *swapTarget) px() float64 { return float64(t.dpi) / w32.USER_DEFAULT_SCREEN_DPI }

// frameDPI returns the DPI of the window's frame: the window's own DPI,
// or the one the window converts with when Windows doesn't know it yet.
func (t *swapTarget) frameDPI() uint32 {
	if t.hwnd != 0 {
		if d := w32.GetDpiForWindow(t.hwnd); d != 0 {
			return d
		}
	}
	return t.dpi
}

// paintable reports whether the window is on screen.
func (t *swapTarget) paintable() bool {
	return !t.dead && w32.IsWindowVisible(t.hwnd) && !w32.IsIconic(t.hwnd)
}

// paintNow paints the whole window through WM_PAINT before it returns.
func (t *swapTarget) paintNow() {
	if !t.paintable() {
		return
	}
	// Both fail only for a destroyed window.
	_ = w32.InvalidateRect(t.hwnd, nil, false)
	_ = w32.UpdateWindow(t.hwnd)
	rethrow()
}

// screenClient returns the client rectangle in screen pixels.
func (t *swapTarget) screenClient() (w32.Rect, error) {
	rc, err := w32.GetClientRect(t.hwnd)
	if err != nil {
		return w32.Rect{}, err
	}
	var o w32.Point
	if err := w32.ClientToScreen(t.hwnd, &o); err != nil {
		return w32.Rect{}, err
	}
	return w32.Rect{Left: o.X, Top: o.Y, Right: o.X + rc.Right, Bottom: o.Y + rc.Bottom}, nil
}

// screenPoint converts a screen pixel to the window's points.
func (t *swapTarget) screenPoint(pt w32.Point) driver.Point {
	cl, err := t.screenClient()
	if err != nil {
		return driver.Point{}
	}
	return pointAt(pt, cl, t.px())
}

// screenToClient converts a screen pixel to client pixels.
func (t *swapTarget) screenToClient(pt w32.Point) w32.Point {
	cl, err := t.screenClient()
	if err != nil {
		return w32.Point{}
	}
	return w32.Point{X: pt.X - cl.Left, Y: pt.Y - cl.Top}
}

// clickRules returns the double-click settings at the window's DPI:
// GetDoubleClickTime and the SM_CXDOUBLECLK × SM_CYDOUBLECLK rectangle in
// pixels.
func (t *swapTarget) clickRules() shellkit.ClickRules {
	dpi := t.frameDPI()
	return shellkit.ClickRules{Time: w32.GetDoubleClickTime(),
		W: w32.GetSystemMetricsForDpi(w32.SM_CXDOUBLECLK, dpi),
		H: w32.GetSystemMetricsForDpi(w32.SM_CYDOUBLECLK, dpi)}
}

// allWindows yields the swap targets of the open windows: the surfaces in
// creation order, then the popups'. It copies nothing (the idle steps use
// it, review P17), so the loop body must not open or close a window.
func (s *Shell) allWindows() iter.Seq[*swapTarget] {
	return func(yield func(*swapTarget) bool) {
		for _, sf := range s.surfaces {
			if !yield(&sf.swapTarget) {
				return
			}
		}
		for _, p := range s.popups {
			if !yield(&p.swapTarget) {
				return
			}
		}
	}
}

// paintInvalid is the clock's paint pass: every invalidated surface on
// screen is painted, then every invalidated popup. Hidden and minimized
// ones stay invalid until shown. The list is copied first (a Paint may
// open or close a window) into the shell's scratch, which a nested pass
// (paintBuf nil meanwhile) does not share.
func (s *Shell) paintInvalid(time.Duration) {
	list := s.paintBuf[:0]
	s.paintBuf = nil
	for t := range s.allWindows() {
		list = append(list, t)
	}
	for _, t := range list {
		if t.invalid {
			t.paintNow()
		}
	}
	clear(list[:cap(list)])
	s.paintBuf = list[:0]
}
