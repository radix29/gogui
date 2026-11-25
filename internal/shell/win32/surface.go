//go:build windows

package win32

import (
	"strings"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/shellkit"
	"github.com/radix29/gogui/internal/shell/win32/d2d"
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
//
// The window, its size and DPI and its rendering state are the embedded
// swapTarget (swaptarget.go), which popups share.
type surface struct {
	swapTarget
	h shell.SurfaceHandler

	style, exStyle uint32
	resizable      bool

	min, max driver.Size

	creating   bool // inside NewSurface: no Resized
	dpiChange  bool // inside WM_DPICHANGED: WM_SIZE waits for it
	sizing     bool // between WM_ENTERSIZEMOVE and WM_EXITSIZEMOVE
	menuLoop   bool // between WM_ENTERMENULOOP and WM_EXITMENULOOP
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
	// activating: a WM_MOUSEACTIVATE came while the surface was not key, so
	// the next button-down is the press that activates it (S6.1c).
	activating bool

	// Keyboard (keyboard.go).
	consumed      [256]bool // the key-down of the virtual key was handled
	highSurrogate uint16    // the first half of a surrogate pair from WM_CHAR

	// Modal surfaces (modal.go, S5.7e): Modal and Owner from the config,
	// whether it has been shown (placed and blocking), the key surface at
	// that Show, whether a modal surface disables the window, and whether
	// destroy runs.
	modal, modalShown, blocked, closing bool
	owner, prevKey                      *surface

	cursor  shell.Cursor
	regions []shell.ChromeRegion // hit-tested by WM_NCHITTEST (chrome.go)
	ime     ime                  // the text input (ime.go)
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
// devices (a lost device is recovered and the chain tried once more,
// newChain); if that fails, the window is destroyed again and the error
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
	owner, err := s.modalOwner(cfg)
	if err != nil {
		return nil, err
	}
	inst, err := registerClass()
	if err != nil {
		return nil, err
	}
	sf := &surface{swapTarget: swapTarget{s: s, invalid: true}, h: h, min: cfg.MinSize, max: cfg.MaxSize,
		resizable: cfg.Style&driver.StyleResizable != 0, creating: true,
		modal: cfg.Modal, owner: owner}
	sf.win = sf
	sf.style, sf.exStyle = surfaceStyle(cfg.Style)
	var ownerHwnd windows.HWND
	if cfg.Modal { // an owned top level without a taskbar button (modal.go)
		sf.resizable = false
		sf.style, sf.exStyle = modalStyle, w32.WS_EX_NOREDIRECTIONBMP
		ownerHwnd = s.listener
		if owner != nil {
			ownerHwnd = owner.hwnd
		}
	}
	sf.want = shellkit.ClampSize(cfg.Size, sf.min, sf.max)
	sf.dpi = primaryDPI()
	sf.scale = sf.px()
	ww, wh := sf.windowSize(sf.want)

	sf.id = targets.New(sf)
	hwnd, err := w32.CreateWindowEx(sf.exStyle, className, noNUL(cfg.Title), sf.style,
		w32.CW_USEDEFAULT, 0, ww, wh, ownerHwnd, 0, inst, uintptr(sf.id))
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
	if err := sf.newChain(); err != nil {
		_ = w32.DestroyWindow(hwnd) // still creating: no Closed
		rethrow()
		return nil, err
	}
	sf.disassociateIME()          // the text input starts off (shell.Surface.SetTextInput)
	if cfg.Center && !cfg.Modal { // a modal surface is placed at its first Show
		sf.Center()
	}
	sf.creating = false
	s.applyBlocking() // a surface created under a modal one is blocked
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

// SetSizeLimits implements shell.Surface: WM_GETMINMAXINFO reads the new
// limits; a size outside them is clamped with one resize.
func (sf *surface) SetSizeLimits(min, max driver.Size) {
	if sf.change("SetSizeLimits") {
		sf.min, sf.max = min, max
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
// of its monitor. A maximized or minimized surface is not moved. A modal
// surface that has been shown is placed again (on its owner, modal.go);
// before its first Show, Center does nothing for it.
func (sf *surface) Center() {
	if !sf.change("Center") || w32.IsZoomed(sf.hwnd) || w32.IsIconic(sf.hwnd) {
		return
	}
	if sf.modal {
		if sf.modalShown {
			sf.placeModal()
		}
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

// Show implements shell.Surface. It also restores a minimized surface. The
// first Show of a modal surface places it and blocks the others; a blocked
// surface is shown without activation and the active modal stays key
// (modal.go).
func (sf *surface) Show() {
	if !sf.change("Show") {
		return
	}
	if sf.modal && !sf.modalShown {
		sf.showModal()
		if sf.dead {
			return
		}
	}
	cmd := int32(w32.SW_SHOW)
	if w32.IsIconic(sf.hwnd) {
		cmd = w32.SW_RESTORE
	}
	if sf.blocked {
		cmd = w32.SW_SHOWNA
		if w32.IsIconic(sf.hwnd) {
			cmd = w32.SW_SHOWNOACTIVATE
		}
	}
	w32.ShowWindow(sf.hwnd, cmd)
	rethrow()
	if sf.dead {
		return
	}
	if sf.blocked {
		sf.s.modalKey()
	} else {
		// Windows refuses this when another process has the foreground;
		// the taskbar button flashes instead.
		w32.SetForegroundWindow(sf.hwnd)
		rethrow()
	}
	if !sf.dead && sf.invalid {
		sf.s.clock.RequestFrame()
	}
}

// Minimize implements shell.Surface. It does nothing for a hidden or a
// modal surface.
func (sf *surface) Minimize() {
	if !sf.change("Minimize") || sf.modal || !w32.IsWindowVisible(sf.hwnd) || w32.IsIconic(sf.hwnd) {
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

// Close implements shell.Surface: destroy, whose WM_DESTROY calls Closed.
func (sf *surface) Close() {
	if !sf.change("Close") {
		return
	}
	sf.destroy()
}

// destroy closes the surface's modal surfaces and popups, then destroys
// its window: Windows would hide and deactivate it before it destroys the
// owned windows, and that would dismiss a grab popup on the way. A modal
// surface stops blocking and hands the key on first (modal.go).
func (sf *surface) destroy() {
	if sf.closing {
		return
	}
	sf.closing = true
	sf.closeOwned()
	sf.s.closePopupsOf(sf)
	if sf.dead {
		return
	}
	if sf.modal {
		sf.leaveModality()
		if sf.dead {
			return
		}
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
	sf.s.clock.invalidated()
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

// notPainting panics while a SurfaceHandler.Paint runs.
func (s *Shell) notPainting(what string) {
	if s.painting != nil {
		panic("win32: " + what + " called during SurfaceHandler.Paint")
	}
}

// paintContent implements window: the surface handler's Paint.
func (sf *surface) paintContent(c *canvas) { sf.h.Paint(c) }

// background implements window: the window background of the appearance.
func (sf *surface) background() d2d.ColorF { return clearColor(sf.s.appearance) }

// scrollEvent implements window: to the surface's handler.
func (sf *surface) scrollEvent(ev shell.ScrollEvent) { sf.h.Event(ev) }
