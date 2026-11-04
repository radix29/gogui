//go:build windows

package win32

import (
	"strings"
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

// surface is the Windows shell.Surface: one top-level window of className
// (S2.3c) with the custom chrome of chrome.go (S2.3d): the drawable area is
// the client area, which WM_NCCALCSIZE extends over the whole window except
// the side and bottom resize frame.
//
// Sizes are kept in logical points. The window has whole pixels, so a size
// that is not a whole number of pixels at the current scale cannot be read
// back from the window; want remembers the size asked for last (SetSize,
// creation, or the size before a DPI change), and a client size whose pixels
// match it reads back as exactly that size (shellkit.FromPx).
type surface struct {
	s    *Shell
	h    shell.SurfaceHandler
	id   ffi.Handle
	hwnd windows.HWND

	style, exStyle uint32
	resizable      bool

	size     driver.Size // the size last reported (or the initial one)
	want     driver.Size // the size asked for last, see shellkit.FromPx
	scale    float64     // the scale last reported (or the initial one)
	dpi      uint32      // the window's DPI, which converts pixels (px)
	min, max driver.Size

	creating   bool // inside NewSurface: no Resized
	dpiChange  bool // inside WM_DPICHANGED: WM_SIZE waits for it
	sizing     bool // between WM_ENTERSIZEMOVE and WM_EXITSIZEMOVE
	menuLoop   bool // between WM_ENTERMENULOOP and WM_EXITMENULOOP
	dead       bool // WM_DESTROY has run
	invalid    bool // waits for a paint
	key        bool // the key state last reported
	overClient bool // the pointer is over the client area (WM_SETCURSOR)
	inside     bool // between the PointerEnter and PointerLeave sent (ncinput.go)
	tracking   bool // TrackMouseEvent(TME_LEAVE) of the client area is pending
	ncTracking bool // TrackMouseEvent(TME_NONCLIENT) is pending

	// Pointer input (input.go).
	buttons  shell.PointerButton // the held buttons; the mouse is captured while not 0
	upClicks [5]int              // the click count of each held button's press, by buttonIndex
	clicks   shellkit.ClickCounter
	lastPos  driver.Point // the position of the last pointer event

	// Scrolling (wheel.go).
	gesture   shellkit.Gesture
	scrollPos driver.Point // the position of the last precise scroll event

	// Keyboard (keyboard.go).
	consumed      [256]bool // the key-down of the virtual key was handled
	highSurrogate uint16    // the first half of a surrogate pair from WM_CHAR

	cursor   shell.Cursor
	regions  []shell.ChromeRegion // hit-tested by WM_NCHITTEST (chrome.go)
	paintErr error                // the recorder errors of the last paint (Shell.debug)

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
	renderErr error // the rendering error of the last paint or resize (tests)

	// Pacing (clock.go): the chain's frame-latency waitable object (0
	// without one), a signal taken from it and not yet spent by a Present,
	// the Presents it has not signaled for yet (it signals once for every
	// Present), and the Presents (tests).
	waitable windows.Handle
	ticket   bool
	debt     int
	presents int
}

var _ shell.Surface = (*surface)(nil)

// surfaceStyle returns the window styles for a surface style.
func surfaceStyle(st driver.WindowStyle) (style, exStyle uint32) {
	style = w32.WS_OVERLAPPEDWINDOW
	if st&driver.StyleResizable == 0 {
		style &^= w32.WS_THICKFRAME | w32.WS_MAXIMIZEBOX
	}
	if st&driver.StyleMiniaturizable == 0 {
		style &^= w32.WS_MINIMIZEBOX
	}
	// No redirection bitmap: the window shows only its composition swap
	// chain (present.go), and GDI never paints it.
	return style, w32.WS_EX_APPWINDOW | w32.WS_EX_NOREDIRECTIONBMP
}

// NewSurface implements shell.Surface. The window is created hidden on the
// monitor Windows picks (CW_USEDEFAULT), at the primary monitor's DPI
// first; once it exists, it is sized again at its own DPI. Then it gets its
// swap chain (present.go), the first surface creating the shell's GPU
// devices; if that fails, the window is destroyed again and the error
// returned.
func (s *Shell) NewSurface(cfg shell.SurfaceConfig, h shell.SurfaceHandler) (shell.Surface, error) {
	s.checkReady("NewSurface")
	if h == nil {
		panic("win32: NewSurface with a nil handler")
	}
	s.notPainting("NewSurface")
	if s.quit {
		return nil, ErrQuit
	}
	inst, err := registerClass()
	if err != nil {
		return nil, err
	}
	sf := &surface{s: s, h: h, min: cfg.MinSize, max: cfg.MaxSize,
		resizable: cfg.Style&driver.StyleResizable != 0, creating: true, invalid: true}
	sf.style, sf.exStyle = surfaceStyle(cfg.Style)
	sf.want = shellkit.ClampSize(cfg.Size, sf.min, sf.max)
	sf.dpi = primaryDPI()
	sf.scale = sf.px()
	ww, wh := sf.windowSize(sf.want)

	sf.id = targets.New(sf)
	hwnd, err := w32.CreateWindowEx(sf.exStyle, className, noNUL(cfg.Title), sf.style,
		w32.CW_USEDEFAULT, 0, ww, wh, 0, 0, inst, uintptr(sf.id))
	rethrow()
	if err != nil {
		targets.Free(sf.id)
		return nil, err
	}
	sf.hwnd = hwnd
	s.surfaces = append(s.surfaces, sf)
	_, _, _ = sf.applyFrame() // Windows 10 lacks some attributes (D7)

	// The window's own DPI, and its size at that DPI (the minimum track
	// size of DefWindowProcW applied during CreateWindowExW, too, before
	// WM_NCCREATE set the handle).
	if d := w32.GetDpiForWindow(hwnd); d != 0 {
		sf.dpi = d
	}
	sf.scale = sf.px()
	sf.setWindowSize(sf.want)
	sf.size = sf.clientSize()
	sf.want = sf.size
	if err := sf.createChain(); err != nil {
		_ = w32.DestroyWindow(hwnd) // still creating: no Closed
		rethrow()
		return nil, err
	}
	if cfg.Center {
		sf.Center()
	}
	sf.creating = false
	return sf, nil
}

// noNUL drops the NUL characters a Win32 string cannot hold.
func noNUL(s string) string { return strings.ReplaceAll(s, "\x00", "") }

// primaryDPI returns the effective DPI of the primary monitor (96 if
// unknown).
func primaryDPI() uint32 {
	mon := w32.MonitorFromPoint(w32.Point{}, w32.MONITOR_DEFAULTTOPRIMARY)
	if x, _, err := w32.GetDpiForMonitor(mon, w32.MDT_EFFECTIVE_DPI); err == nil && x != 0 {
		return x
	}
	return w32.USER_DEFAULT_SCREEN_DPI
}

// windowSize returns the window size in pixels for a drawable size.
func (sf *surface) windowSize(size driver.Size) (w, h int32) {
	nw, nh := sf.nonClient(sf.frameDPI())
	return shellkit.ToPx(size.W, sf.px()) + nw, shellkit.ToPx(size.H, sf.px()) + nh
}

// setWindowSize resizes the window so its drawable area is size.
func (sf *surface) setWindowSize(size driver.Size) {
	w, h := sf.windowSize(size)
	// Fails only for a destroyed window.
	_ = w32.SetWindowPos(sf.hwnd, 0, 0, 0, w, h,
		w32.SWP_NOMOVE|w32.SWP_NOZORDER|w32.SWP_NOACTIVATE|w32.SWP_NOOWNERZORDER)
	rethrow()
}

// clientSize reads the drawable size from the window.
func (sf *surface) clientSize() driver.Size {
	rc, err := w32.GetClientRect(sf.hwnd)
	if err != nil {
		return sf.size
	}
	return shellkit.FromPx(rc.Right-rc.Left, rc.Bottom-rc.Top, sf.px(), sf.want)
}

// px returns the pixels per point of the window's DPI, which converts
// sizes. It differs from scale (the value last reported) only inside a
// WM_DPICHANGED, before sync reports the new one.
func (sf *surface) px() float64 { return float64(sf.dpi) / w32.USER_DEFAULT_SCREEN_DPI }

// sync reads the window's size and reports a change of size or scale
// through Resized, then invalidates and, if the surface is on screen,
// paints and presents at once (S2.4f): the buffers, the content and the
// window change in the same message, so a live resize neither stretches
// nor flickers.
func (sf *surface) sync() {
	scale := sf.px()
	if sf.creating || sf.dead || w32.IsIconic(sf.hwnd) {
		return
	}
	size := sf.clientSize()
	sf.want = size
	if size == sf.size && scale == sf.scale {
		return
	}
	sf.size, sf.scale = size, scale
	sf.renderErr = sf.fitChain() // the buffers follow the window at once
	sf.h.Resized(size, scale)
	if sf.dead {
		return
	}
	sf.Invalidate()
	sf.paintNow()
}

// live checks the thread and reports whether the surface is open.
func (sf *surface) live(what string) bool {
	sf.s.q.Check("win32.Surface." + what)
	return !sf.dead
}

// change is live for the methods that must not run during a Paint.
func (sf *surface) change(what string) bool {
	if !sf.live(what) {
		return false
	}
	sf.s.notPainting("Surface." + what)
	return true
}

// SetTitle implements shell.Surface.
func (sf *surface) SetTitle(title string) {
	if sf.live("SetTitle") {
		// Fails only for a destroyed window.
		_ = w32.SetWindowText(sf.hwnd, noNUL(title))
		rethrow()
	}
}

// Size implements shell.Surface.
func (sf *surface) Size() driver.Size {
	sf.s.q.Check("win32.Surface.Size")
	return sf.size
}

// SetSize implements shell.Surface. A maximized or minimized surface is
// restored first.
func (sf *surface) SetSize(size driver.Size) {
	if !sf.change("SetSize") {
		return
	}
	sf.resizeTo(size)
}

func (sf *surface) resizeTo(size driver.Size) {
	if w32.IsZoomed(sf.hwnd) || w32.IsIconic(sf.hwnd) {
		w32.ShowWindow(sf.hwnd, w32.SW_RESTORE)
		rethrow()
		if sf.dead {
			return
		}
	}
	sf.want = shellkit.ClampSize(size, sf.min, sf.max)
	sf.setWindowSize(sf.want)
	if !sf.dead {
		sf.sync() // a change smaller than a pixel sends no WM_SIZE
	}
}

// SetMinSize implements shell.Surface.
func (sf *surface) SetMinSize(size driver.Size) {
	if sf.change("SetMinSize") {
		sf.min = size
		sf.applyLimits()
	}
}

// SetMaxSize implements shell.Surface.
func (sf *surface) SetMaxSize(size driver.Size) {
	if sf.change("SetMaxSize") {
		sf.max = size
		sf.applyLimits()
	}
}

func (sf *surface) applyLimits() {
	if c := shellkit.ClampSize(sf.size, sf.min, sf.max); c != sf.size {
		sf.resizeTo(c)
	}
}

// Scale implements shell.Surface: the window's DPI / 96.
func (sf *surface) Scale() float64 {
	sf.s.q.Check("win32.Surface.Scale")
	return sf.scale
}

// Center implements shell.Surface: the window is centered in the work area
// of its monitor. A maximized or minimized surface is not moved.
func (sf *surface) Center() {
	if !sf.change("Center") || w32.IsZoomed(sf.hwnd) || w32.IsIconic(sf.hwnd) {
		return
	}
	mi, err := w32.GetMonitorInfo(w32.MonitorFromWindow(sf.hwnd, w32.MONITOR_DEFAULTTONEAREST))
	if err != nil {
		return
	}
	wr, err := w32.GetWindowRect(sf.hwnd)
	if err != nil {
		return
	}
	x, y := shellkit.CenterIn(shellkit.Rect(mi.RcWork), wr.Right-wr.Left, wr.Bottom-wr.Top)
	_ = w32.SetWindowPos(sf.hwnd, 0, x, y, 0, 0,
		w32.SWP_NOSIZE|w32.SWP_NOZORDER|w32.SWP_NOACTIVATE|w32.SWP_NOOWNERZORDER)
	rethrow()
}

// Show implements shell.Surface. It also restores a minimized surface.
func (sf *surface) Show() {
	if !sf.change("Show") {
		return
	}
	cmd := int32(w32.SW_SHOW)
	if w32.IsIconic(sf.hwnd) {
		cmd = w32.SW_RESTORE
	}
	w32.ShowWindow(sf.hwnd, cmd)
	rethrow()
	if sf.dead {
		return
	}
	// Windows refuses this when another process has the foreground; the
	// taskbar button flashes instead.
	w32.SetForegroundWindow(sf.hwnd)
	rethrow()
	if !sf.dead && sf.invalid {
		sf.s.clock.RequestFrame()
	}
}

// Minimize implements shell.Surface. It does nothing for a hidden surface.
func (sf *surface) Minimize() {
	if !sf.change("Minimize") || !w32.IsWindowVisible(sf.hwnd) || w32.IsIconic(sf.hwnd) {
		return
	}
	w32.ShowWindow(sf.hwnd, w32.SW_MINIMIZE)
	rethrow()
}

// Zoom implements shell.Surface. It does nothing without StyleResizable
// and, on Windows, for a hidden surface (maximizing shows a window).
func (sf *surface) Zoom() {
	if !sf.change("Zoom") || !sf.resizable || !w32.IsWindowVisible(sf.hwnd) {
		return
	}
	cmd := int32(w32.SW_MAXIMIZE)
	if w32.IsZoomed(sf.hwnd) {
		cmd = w32.SW_RESTORE
	}
	w32.ShowWindow(sf.hwnd, cmd)
	rethrow()
}

// Zoomed implements shell.Surface.
func (sf *surface) Zoomed() bool {
	if !sf.live("Zoomed") {
		return false
	}
	return w32.IsZoomed(sf.hwnd)
}

// Close implements shell.Surface: DestroyWindow, whose WM_DESTROY calls
// Closed.
func (sf *surface) Close() {
	if !sf.change("Close") {
		return
	}
	// Fails only for a window that is already gone.
	_ = w32.DestroyWindow(sf.hwnd)
	rethrow()
}

// Invalidate implements shell.Surface.
func (sf *surface) Invalidate() {
	if !sf.live("Invalidate") {
		return
	}
	sf.invalid = true
	sf.s.clock.RequestFrame()
}

// paintable reports whether the surface is on screen.
func (sf *surface) paintable() bool {
	return !sf.dead && w32.IsWindowVisible(sf.hwnd) && !w32.IsIconic(sf.hwnd)
}

// paintNow paints the whole surface through WM_PAINT before it returns.
func (sf *surface) paintNow() {
	if !sf.paintable() {
		return
	}
	// Both fail only for a destroyed window.
	_ = w32.InvalidateRect(sf.hwnd, nil, false)
	_ = w32.UpdateWindow(sf.hwnd)
	rethrow()
}

// SetCursor implements shell.Surface. The cursor shows at once if the
// pointer is over the client area, else when it gets there.
func (sf *surface) SetCursor(c shell.Cursor) {
	if !sf.live("SetCursor") || c == sf.cursor {
		return
	}
	sf.cursor = c
	if sf.overClient {
		w32.SetCursor(cursorHandle(c))
	}
}

// SetChromeRegions implements shell.Surface. WM_NCHITTEST tests them
// (chrome.go), and the pointer over them gets PointerEvents (ncinput.go).
func (sf *surface) SetChromeRegions(regions []shell.ChromeRegion) {
	if sf.live("SetChromeRegions") {
		sf.regions = append([]shell.ChromeRegion(nil), regions...)
	}
}

// cursorIDs maps the cursors to the system cursors (0: hidden).
var cursorIDs = [...]uintptr{
	shell.CursorArrow:           w32.IDC_ARROW,
	shell.CursorIBeam:           w32.IDC_IBEAM,
	shell.CursorPointingHand:    w32.IDC_HAND,
	shell.CursorResizeLeftRight: w32.IDC_SIZEWE,
	shell.CursorResizeUpDown:    w32.IDC_SIZENS,
	shell.CursorCrosshair:       w32.IDC_CROSS,
	shell.CursorNotAllowed:      w32.IDC_NO,
	shell.CursorHidden:          0,
}

// cursors caches the loaded system cursors (shared handles, never
// destroyed). UI thread only.
var cursors [len(cursorIDs)]windows.Handle

// cursorHandle returns the system cursor for c (0 for CursorHidden; the
// arrow for an unknown c).
func cursorHandle(c shell.Cursor) windows.Handle {
	if int(c) >= len(cursorIDs) {
		c = shell.CursorArrow
	}
	if cursorIDs[c] == 0 {
		return 0
	}
	if cursors[c] == 0 {
		h, err := w32.LoadCursor(cursorIDs[c])
		if err != nil {
			return 0
		}
		cursors[c] = h
	}
	return cursors[c]
}

// boolResult is a BOOL message result.
func boolResult(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}

// loword returns the low word of a WPARAM or LPARAM.
func loword(v uintptr) uint16 { return uint16(v) }

// handle is the surface's window procedure.
func (sf *surface) handle(h windows.HWND, msg uint32, wp, lp uintptr) (uintptr, bool) {
	if sf.s.silentClose && msg != w32.WM_DESTROY {
		return 0, false // the teardown after a panic calls no handlers
	}
	switch msg {
	case w32.WM_NCCREATE:
		sf.hwnd = h
	case w32.WM_NCCALCSIZE:
		sf.ncCalcSize(h, wp, lp)
		return 0, true
	case w32.WM_NCHITTEST:
		return sf.hitAt(signedXY(lp)), true
	case w32.WM_GETDPISCALEDSIZE:
		return boolResult(sf.dpiScaledSize(uint32(wp), ffi.At[w32.Size](lp))), true
	case w32.WM_DWMCOMPOSITIONCHANGED:
		_, _, _ = sf.applyFrame()
		return 0, true
	case w32.WM_NCMOUSEMOVE, w32.WM_NCMOUSELEAVE, w32.WM_NCLBUTTONDOWN, w32.WM_NCLBUTTONDBLCLK,
		w32.WM_NCLBUTTONUP:
		return sf.ncPointer(h, msg, wp, lp)
	case w32.WM_MOUSEMOVE, w32.WM_CAPTURECHANGED,
		w32.WM_LBUTTONDOWN, w32.WM_LBUTTONUP, w32.WM_LBUTTONDBLCLK,
		w32.WM_RBUTTONDOWN, w32.WM_RBUTTONUP, w32.WM_RBUTTONDBLCLK,
		w32.WM_MBUTTONDOWN, w32.WM_MBUTTONUP, w32.WM_MBUTTONDBLCLK,
		w32.WM_XBUTTONDOWN, w32.WM_XBUTTONUP, w32.WM_XBUTTONDBLCLK:
		return sf.mouse(h, msg, wp, lp)
	case w32.WM_MOUSEWHEEL, w32.WM_MOUSEHWHEEL:
		sf.wheel(msg, wp, lp)
		return 0, true
	case w32.WM_TIMER:
		if wp == timerGesture {
			sf.endGesture()
			return 0, true
		}
	case w32.WM_KEYDOWN, w32.WM_KEYUP, w32.WM_SYSKEYDOWN, w32.WM_SYSKEYUP:
		return sf.keyMsg(msg, wp, lp)
	case w32.WM_CHAR, w32.WM_SYSCHAR, w32.WM_DEADCHAR, w32.WM_SYSDEADCHAR:
		return sf.char(msg, wp)
	case w32.WM_GETMINMAXINFO:
		sf.minMaxInfo(ffi.At[w32.MinMaxInfo](lp))
		return 0, true
	case w32.WM_SIZE:
		if wp != w32.SIZE_MINIMIZED && !sf.dpiChange {
			sf.sync()
			if sf.invalid && !sf.dead { // invalidated while minimized
				sf.s.clock.RequestFrame()
			}
		}
		return 0, true
	case w32.WM_DPICHANGED:
		sf.dpiChanged(uint32(loword(wp)), ffi.At[w32.Rect](lp))
		return 0, true
	case w32.WM_ENTERSIZEMOVE, w32.WM_EXITSIZEMOVE:
		sf.sizing = msg == w32.WM_ENTERSIZEMOVE
		sf.s.clock.modalChanged()
	case w32.WM_ENTERMENULOOP, w32.WM_EXITMENULOOP:
		sf.menuLoop = msg == w32.WM_ENTERMENULOOP
		sf.s.clock.modalChanged()
	case w32.WM_ACTIVATE:
		if key := loword(wp) != w32.WA_INACTIVE; key != sf.key {
			sf.key = key
			if !key {
				sf.consumed = [256]bool{} // no key-ups come for the held keys
				sf.highSurrogate = 0
			}
			sf.h.KeyChanged(key)
		}
		return 0, false // DefWindowProcW sets the focus
	case w32.WM_CLOSE:
		if sf.h.CloseRequested() && !sf.dead {
			_ = w32.DestroyWindow(h)
		}
		return 0, true
	case w32.WM_DESTROY:
		sf.destroyed()
		return 0, true
	case w32.WM_SETCURSOR:
		if windows.HWND(wp) == h && loword(lp) == w32.HTCLIENT {
			sf.overClient = true
			sf.trackClient(h)
			w32.SetCursor(cursorHandle(sf.cursor))
			return 1, true
		}
		sf.overClient = false
	case w32.WM_MOUSELEAVE:
		sf.overClient, sf.tracking = false, false
		sf.leaveIfOutside()
		return 0, true
	case w32.WM_ERASEBKGND:
		return 1, true // WM_PAINT paints everything
	case w32.WM_PAINT:
		sf.paint()
		return 0, true
	}
	return 0, false
}

// minMaxInfo writes the size limits, converted to window pixels, into a
// WM_GETMINMAXINFO. The minimum track size is always set, so Windows' own
// minimum (SM_CXMINTRACK) does not apply; the maximum only with a limit.
func (sf *surface) minMaxInfo(mmi *w32.MinMaxInfo) {
	nw, nh := sf.nonClient(sf.frameDPI())
	mmi.PtMinTrackSize = w32.Point{X: shellkit.ToPx(sf.min.W, sf.px()) + nw, Y: shellkit.ToPx(sf.min.H, sf.px()) + nh}
	if sf.max.W > 0 {
		mmi.PtMaxTrackSize.X = max(shellkit.ToPx(sf.max.W, sf.px())+nw, mmi.PtMinTrackSize.X)
		mmi.PtMaxSize.X = mmi.PtMaxTrackSize.X
	}
	if sf.max.H > 0 {
		mmi.PtMaxTrackSize.Y = max(shellkit.ToPx(sf.max.H, sf.px())+nh, mmi.PtMinTrackSize.Y)
		mmi.PtMaxSize.Y = mmi.PtMaxTrackSize.Y
	}
}

// dpiChanged applies a WM_DPICHANGED: the new DPI, then the suggested
// window rectangle, then one Resized with the new size and scale. The
// logical size is kept when the suggested rectangle has its pixels.
func (sf *surface) dpiChanged(dpi uint32, suggested *w32.Rect) {
	if dpi == 0 {
		return
	}
	sf.want = sf.size
	sf.dpi = dpi
	sf.dpiChange = true
	_ = w32.SetWindowPos(sf.hwnd, 0, suggested.Left, suggested.Top,
		suggested.Right-suggested.Left, suggested.Bottom-suggested.Top,
		w32.SWP_NOZORDER|w32.SWP_NOACTIVATE|w32.SWP_NOOWNERZORDER)
	sf.dpiChange = false
	if sf.dead {
		return
	}
	sf.sync()
}

// destroyed handles WM_DESTROY: the surface is dead, leaves the shell and
// gets Closed once. The handle is freed in WM_NCDESTROY.
func (sf *surface) destroyed() {
	if sf.dead {
		return
	}
	sf.dead = true
	sf.s.removeSurface(sf)
	if sf.s.silentClose || sf.creating { // teardown, or a failed CreateWindowExW
		return
	}
	sf.h.Closed()
}

// removeSurface drops sf from the open surfaces and releases its
// rendering objects; after the last surface the shell's GPU devices go
// too.
func (s *Shell) removeSurface(sf *surface) {
	sf.releaseChain()
	for i, o := range s.surfaces {
		if o == sf {
			s.surfaces = append(s.surfaces[:i], s.surfaces[i+1:]...)
			break
		}
	}
	if len(s.surfaces) == 0 {
		s.releaseGPU()
	}
}

// closeAll destroys every open surface. With silent, Closed is not
// called (the teardown after a panic).
func (s *Shell) closeAll(silent bool) {
	s.silentClose = silent
	defer func() { s.silentClose = false }()
	for len(s.surfaces) > 0 {
		sf := s.surfaces[len(s.surfaces)-1]
		// Fails only for a window that is already gone.
		_ = w32.DestroyWindow(sf.hwnd)
		if !sf.dead { // gone without WM_DESTROY reaching us: drop it
			sf.dead = true
			s.removeSurface(sf)
		}
		if !silent {
			rethrow()
		}
	}
}

// paintInvalid is the clock's paint pass: every invalidated surface on
// screen is painted. Hidden and minimized ones stay invalid until shown.
func (s *Shell) paintInvalid(time.Duration) {
	for _, sf := range append([]*surface(nil), s.surfaces...) {
		if sf.invalid {
			sf.paintNow()
		}
	}
}

// notPainting panics while a SurfaceHandler.Paint runs.
func (s *Shell) notPainting(what string) {
	if s.painting != nil {
		panic("win32: " + what + " called during SurfaceHandler.Paint")
	}
}
