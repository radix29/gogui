//go:build windows

package win32

import (
	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/shellkit"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// Modal surfaces (S5.7e, shell.SurfaceConfig.Modal), as the fake shell
// models them (S5.7d):
//
//   - The window is an owned WS_POPUP top level with WS_CAPTION and
//     WS_SYSMENU (modalStyle): the caption is taken away by WM_NCCALCSIZE
//     as for every surface, but it keeps the DWM shadow, the border and the
//     rounded corners (applyFrame). Its owner is Owner's window or, for an
//     app-modal surface, the shell's hidden listener; an owned window
//     without WS_EX_APPWINDOW has no taskbar button and stays above its
//     owner. Without WS_THICKFRAME, WS_MINIMIZEBOX and WS_MAXIMIZEBOX it is
//     neither resizable, minimizable nor zoomable, and WM_SYSCOMMAND's
//     SC_SIZE, SC_MINIMIZE and SC_MAXIMIZE are refused (Win+arrow).
//   - The shell's shown modal surfaces are kept in first-Show order; the
//     last one is the active modal. applyBlocking disables (EnableWindow)
//     every other open surface and enables the active modal, or every
//     surface without one; it runs at a modal's first Show, when a modal
//     closes and for each new surface. A disabled window gets no mouse or
//     keyboard input from Windows, which also refuses to give it the
//     focus; becoming blocked first ends the surface's grab popup, its
//     capture (a PointerUp with Clicks 0 for every held button) and its
//     pointer presence (PointerLeave), as a capture cancel does.
//   - What still reaches a blocked surface: an activation (the taskbar,
//     Alt-Tab, SetForegroundWindow) is not reported as KeyChanged and the
//     listener makes the active modal key instead (msgModalKey); a click on
//     it (WM_SETCURSOR with HTERROR and a button-down) does the same and
//     shows the arrow; WM_CLOSE and the WM_SYSCOMMANDs that move, size,
//     maximize or close it are refused (no CloseRequested). The input
//     messages are dropped too, in case one is sent to it.
//   - Placement (placeModal, at the first Show and on Center): centered on
//     Owner's drawable area when it is shown and not minimized, else on the
//     work area of the key surface's monitor (the primary's without one),
//     then kept inside the work area of the monitor it lands on; placed
//     again once if that monitor has another DPI.
//   - Closing (leaveModality, before DestroyWindow): the surface leaves the
//     modal list and applyBlocking enables the others first (Windows would
//     otherwise activate another application's window), then, if it was
//     key, the key goes to the next modal, else to Owner, else to the
//     surface key at its first Show (open, shown, not minimized, not
//     blocked). Windows deactivates a window before it destroys it, so on
//     Win32 this hand-off comes before the modal's Closed (after it on the
//     fake). Closing the owner closes its modal surfaces first (destroy).
//     Quit does neither.

// msgModalKey asks the listener to make the active modal surface key
// (posted from a blocked surface's activation).
const msgModalKey = w32.WM_APP + 3

// modalStyle is the window style of a modal surface.
const modalStyle = w32.WS_POPUP | w32.WS_CAPTION | w32.WS_SYSMENU

// modalOwner resolves SurfaceConfig.Owner for NewSurface.
func (s *Shell) modalOwner(cfg shell.SurfaceConfig) (*surface, error) {
	if cfg.Owner == nil {
		return nil, nil
	}
	o, ok := cfg.Owner.(*surface)
	if !cfg.Modal || !ok || o == nil || o.s != s || o.dead {
		return nil, shell.ErrSurfaceOwner
	}
	return o, nil
}

// activeModal returns the modal surface that blocks the others, or nil.
func (s *Shell) activeModal() *surface {
	if n := len(s.modals); n > 0 {
		return s.modals[n-1]
	}
	return nil
}

// keySurface returns the surface reported key last, or nil.
func (s *Shell) keySurface() *surface {
	for _, sf := range s.surfaces {
		if sf.key && !sf.dead {
			return sf
		}
	}
	return nil
}

// applyBlocking disables every open surface but the active modal and
// enables the rest (every surface without an active modal). A closing
// surface is left alone.
func (s *Shell) applyBlocking() {
	m := s.activeModal()
	for _, sf := range append([]*surface(nil), s.surfaces...) {
		if !sf.dead && !sf.closing {
			sf.setBlocked(m != nil && sf != m)
		}
	}
}

// setBlocked disables or enables the surface's window. Becoming blocked
// ends its grab popup, capture and pointer presence first.
func (sf *surface) setBlocked(b bool) {
	if b == sf.blocked || sf.dead {
		return
	}
	sf.blocked = b
	if b {
		if g := sf.grabbed(); g != nil {
			g.dismiss()
			rethrow()
		}
		sf.endPointer()
		if sf.dead {
			return
		}
	}
	w32.EnableWindow(sf.hwnd, !b) // disabling sends WM_CANCELMODE
	rethrow()
}

// endPointer ends the surface's pointer state as an OS capture cancel
// does: a PointerUp with Clicks 0 for every held button, the capture
// released, then PointerLeave if the pointer was inside. It returns the
// buttons that were held.
func (sf *surface) endPointer() shell.PointerButton {
	held := sf.buttons
	sf.buttons = 0
	if w32.GetCapture() == sf.hwnd {
		_ = w32.ReleaseCapture() // its WM_CAPTURECHANGED finds no buttons
		rethrow()
	}
	for b := shell.ButtonPrimary; b <= shell.ButtonForward && !sf.dead; b <<= 1 {
		if held&b != 0 {
			sf.pointer(shell.PointerUp, sf.lastPos, b, 0)
		}
	}
	if sf.inside && !sf.dead {
		sf.inside = false
		sf.pointer(shell.PointerLeave, sf.lastPos, shell.ButtonNone, 0)
	}
	// The WM_MOUSELEAVE may or may not come: track anew later.
	sf.tracking, sf.ncTracking, sf.overClient = false, false, false
	return held
}

// showModal runs at the first Show of a modal surface: it places it,
// becomes the active modal and blocks the other surfaces.
func (sf *surface) showModal() {
	s := sf.s
	sf.modalShown = true
	sf.prevKey = s.keySurface()
	sf.placeModal()
	if sf.dead {
		return
	}
	s.modals = append(s.modals, sf)
	s.applyBlocking()
}

// placeModal centers a modal surface on its owner, when that is shown and
// not minimized, or else on the work area of the key surface's monitor,
// and keeps it inside the work area of the monitor it lands on (its
// top-left wins when it is larger). If the window's DPI changes on the
// way, it is placed again at its new size.
func (sf *surface) placeModal() {
	for range 2 {
		dpi := sf.dpi
		sf.moveModal()
		if sf.dead {
			return
		}
		if d := w32.GetDpiForWindow(sf.hwnd); d != 0 && d != sf.dpi {
			// No WM_DPICHANGED came (a hidden window): rescale here.
			sf.want, sf.dpi = sf.size, d
			sf.setWindowSize(sf.want)
			if !sf.dead {
				sf.sync()
			}
		}
		if sf.dead || sf.dpi == dpi {
			return
		}
	}
}

// moveModal moves the window once to its centered place.
func (sf *surface) moveModal() {
	wr, err := w32.GetWindowRect(sf.hwnd)
	if err != nil {
		return
	}
	area, ok := sf.modalArea()
	if !ok {
		return
	}
	w, h := wr.Right-wr.Left, wr.Bottom-wr.Top
	x, y := shellkit.CenterIn(shellkit.Rect(area), w, h)
	r := w32.Rect{Left: x, Top: y, Right: x + w, Bottom: y + h}
	if mi, err := w32.GetMonitorInfo(w32.MonitorFromRect(&r, w32.MONITOR_DEFAULTTONEAREST)); err == nil {
		wk := mi.RcWork
		x = max(min(x, wk.Right-w), wk.Left)
		y = max(min(y, wk.Bottom-h), wk.Top)
	}
	_ = w32.SetWindowPos(sf.hwnd, 0, x, y, 0, 0,
		w32.SWP_NOSIZE|w32.SWP_NOZORDER|w32.SWP_NOACTIVATE|w32.SWP_NOOWNERZORDER)
	rethrow()
}

// modalArea returns the screen rectangle (pixels) a modal surface is
// centered in: the owner's drawable area, or the work area of the key
// surface's monitor, or the primary monitor's.
func (sf *surface) modalArea() (w32.Rect, bool) {
	if o := sf.owner; o.shownNormal() {
		if cl, err := o.screenClient(); err == nil {
			return cl, true
		}
	}
	mon := w32.MonitorFromPoint(w32.Point{}, w32.MONITOR_DEFAULTTOPRIMARY)
	if k := sf.s.keySurface(); k != nil && k != sf {
		mon = w32.MonitorFromWindow(k.hwnd, w32.MONITOR_DEFAULTTONEAREST)
	}
	mi, err := w32.GetMonitorInfo(mon)
	if err != nil {
		return w32.Rect{}, false
	}
	return mi.RcWork, true
}

// shownNormal reports whether sf is open, not closing, shown and not
// minimized.
func (sf *surface) shownNormal() bool {
	return sf != nil && !sf.dead && !sf.closing && w32.IsWindowVisible(sf.hwnd) && !w32.IsIconic(sf.hwnd)
}

// leaveModality runs before a modal surface's window is destroyed: it
// stops blocking and, if it was key, hands the key on. Under Quit it only
// leaves the list.
func (sf *surface) leaveModality() {
	s := sf.s
	i := -1
	for j, m := range s.modals {
		if m == sf {
			i = j
		}
	}
	if i < 0 {
		return
	}
	s.modals = append(s.modals[:i], s.modals[i+1:]...)
	if s.quit {
		return
	}
	wasKey := sf.key || w32.GetActiveWindow() == sf.hwnd
	s.applyBlocking()
	if !wasKey || sf.dead {
		return
	}
	var to *surface
	switch m := s.activeModal(); {
	case m != nil:
		to = m
	case sf.owner.shownNormal() && !sf.owner.blocked:
		to = sf.owner
	case sf.prevKey.shownNormal() && !sf.prevKey.blocked:
		to = sf.prevKey
	}
	if to != nil {
		activate(to.hwnd)
	}
}

// closeOwned closes the modal surfaces sf owns, newest first.
func (sf *surface) closeOwned() {
	for {
		var last *surface
		for _, o := range sf.s.surfaces {
			if o.owner == sf && !o.dead {
				last = o
			}
		}
		if last == nil {
			return
		}
		last.destroy()
	}
}

// activate makes h (a window of the UI thread) the foreground window or,
// when Windows refuses that (another process has the foreground), the
// thread's active window, which becomes the foreground when the app does.
func activate(h windows.HWND) {
	if !w32.SetForegroundWindow(h) {
		w32.SetActiveWindow(h)
	}
	rethrow()
}

// modalKey makes the active modal surface key (msgModalKey).
func (s *Shell) modalKey() {
	if m := s.activeModal(); m != nil && !m.dead && w32.GetActiveWindow() != m.hwnd {
		activate(m.hwnd)
	}
}

// blockedMsg handles a message to a blocked surface; handled reports that
// it was refused or dropped.
func (sf *surface) blockedMsg(h windows.HWND, msg uint32, wp, lp uintptr) (r uintptr, handled bool) {
	switch msg {
	case w32.WM_ACTIVATE:
		if loword(wp) != w32.WA_INACTIVE {
			// Fails only once the listener is gone, after Run.
			_ = w32.PostMessage(sf.s.listener, msgModalKey, 0, 0)
			return 0, true // not key: no KeyChanged, no focus
		}
	case w32.WM_SETCURSOR:
		if windows.HWND(wp) == h {
			if isButtonDown(uint32(lp>>16) & 0xFFFF) {
				sf.s.modalKey()
			}
			w32.SetCursor(cursorHandle(shell.CursorArrow))
			return 1, true
		}
	case w32.WM_CLOSE:
		sf.s.modalKey()
		return 0, true
	case w32.WM_SYSCOMMAND:
		switch wp & 0xFFF0 {
		case w32.SC_MOVE, w32.SC_SIZE, w32.SC_MAXIMIZE, w32.SC_CLOSE:
			sf.s.modalKey()
			return 0, true
		}
	case w32.WM_MOUSEMOVE, w32.WM_MOUSEWHEEL, w32.WM_MOUSEHWHEEL,
		w32.WM_LBUTTONDOWN, w32.WM_LBUTTONUP, w32.WM_LBUTTONDBLCLK,
		w32.WM_RBUTTONDOWN, w32.WM_RBUTTONUP, w32.WM_RBUTTONDBLCLK,
		w32.WM_MBUTTONDOWN, w32.WM_MBUTTONUP, w32.WM_MBUTTONDBLCLK,
		w32.WM_XBUTTONDOWN, w32.WM_XBUTTONUP, w32.WM_XBUTTONDBLCLK,
		w32.WM_NCMOUSEMOVE, w32.WM_NCLBUTTONDOWN, w32.WM_NCLBUTTONDBLCLK, w32.WM_NCLBUTTONUP,
		w32.WM_KEYDOWN, w32.WM_KEYUP, w32.WM_SYSKEYDOWN, w32.WM_SYSKEYUP,
		w32.WM_CHAR, w32.WM_SYSCHAR, w32.WM_DEADCHAR, w32.WM_SYSDEADCHAR,
		w32.WM_IME_STARTCOMPOSITION, w32.WM_IME_COMPOSITION, w32.WM_IME_ENDCOMPOSITION:
		return 0, true
	}
	return 0, false
}

// modalSysCommand refuses the WM_SYSCOMMANDs a modal surface does not
// have (Win+arrow sends them without the style bits).
func modalSysCommand(wp uintptr) bool {
	switch wp & 0xFFF0 {
	case w32.SC_SIZE, w32.SC_MINIMIZE, w32.SC_MAXIMIZE:
		return true
	}
	return false
}

// isButtonDown reports whether msg is a mouse button-down message (the
// high word of a WM_SETCURSOR's lParam).
func isButtonDown(msg uint32) bool {
	switch msg {
	case w32.WM_LBUTTONDOWN, w32.WM_RBUTTONDOWN, w32.WM_MBUTTONDOWN, w32.WM_XBUTTONDOWN,
		w32.WM_NCLBUTTONDOWN, w32.WM_NCRBUTTONDOWN:
		return true
	}
	return false
}
