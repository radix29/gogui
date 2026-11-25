//go:build windows

package win32

import (
	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/shellkit"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// The surface's window procedure (handle) and the end of its life:
// WM_DESTROY (destroyed), removeSurface and closeAll. The state and the
// shell.Surface methods are in surface.go.

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
	if sf.blocked {
		if r, done := sf.blockedMsg(h, msg, wp, lp); done {
			return r, true
		}
	} else if sf.modal && msg == w32.WM_SYSCOMMAND && modalSysCommand(wp) {
		return 0, true // never resized, minimized or zoomed
	}
	if g := sf.grabbed(); g != nil {
		if r, done := sf.parentGrabMsg(g, msg, wp, lp); done {
			return r, true
		}
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
	case w32.WM_IME_SETCONTEXT, w32.WM_IME_STARTCOMPOSITION, w32.WM_IME_COMPOSITION,
		w32.WM_IME_ENDCOMPOSITION:
		return sf.imeMsg(h, msg, wp, lp)
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
	case w32.WM_MOUSEACTIVATE:
		sf.mouseActivate(lp)
		return 0, false // DefWindowProcW answers MA_ACTIVATE
	case w32.WM_ACTIVATE:
		if key := loword(wp) != w32.WA_INACTIVE; key != sf.key {
			sf.key = key
			if !key {
				sf.activating = false
				sf.consumed = [256]bool{} // no key-ups come for the held keys
				sf.highSurrogate = 0
			}
			sf.h.KeyChanged(key)
		}
		return 0, false // DefWindowProcW sets the focus
	case w32.WM_CLOSE:
		if sf.h.CloseRequested() && !sf.dead {
			sf.destroy()
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
	_ = sf.placeIME() // the caret in pixels at the new DPI (none once dead)
}

// destroyed handles WM_DESTROY: the surface's popups close (Windows
// destroys owned windows first, so this only catches stragglers), then the
// surface is dead, leaves the shell and gets Closed once. The handle is
// freed in WM_NCDESTROY.
func (sf *surface) destroyed() {
	if sf.dead {
		return
	}
	sf.s.closePopupsOf(sf)
	if sf.modal && !sf.dead {
		sf.leaveModality() // destroyed without destroy (its owner, Quit)
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

// closeAll destroys every open surface, their popups first (destroying
// a parent first would dismiss a grab popup on the way). With silent,
// Closed is not called (the teardown after a panic).
func (s *Shell) closeAll(silent bool) {
	s.silentClose = silent
	defer func() { s.silentClose = false }()
	for len(s.popups) > 0 {
		s.popups[len(s.popups)-1].destroy()
		if !silent {
			rethrow()
		}
	}
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
