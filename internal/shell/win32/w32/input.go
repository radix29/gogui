//go:build windows

package w32

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// ---- timers ----

// SetTimer starts (or restarts) timer id of h: WM_TIMER with wParam id
// arrives every ms milliseconds (at least USER_TIMER_MINIMUM, 10 ms) until
// KillTimer.
func SetTimer(h windows.HWND, id uintptr, ms uint32) error {
	r, _, err := callV(procSetTimer, uintptr(h), id, uintptr(ms), 0)
	if r == 0 {
		return lastError("SetTimer", err)
	}
	return nil
}

// KillTimer stops timer id of h and removes its pending WM_TIMER.
func KillTimer(h windows.HWND, id uintptr) error {
	r, _, err := callV(procKillTimer, uintptr(h), id)
	if r == 0 {
		return lastError("KillTimer", err)
	}
	return nil
}

// ---- input, capture, cursor and keyboard layout ----

// SetCapture captures the mouse for h and returns the previous capture
// window (0 if none).
func SetCapture(h windows.HWND) windows.HWND {
	r, _, _ := callV(procSetCapture, uintptr(h))
	return windows.HWND(r)
}

// ReleaseCapture releases the mouse capture of the calling thread.
func ReleaseCapture() error {
	r, _, err := callV(procReleaseCapture)
	if r == 0 {
		return lastError("ReleaseCapture", err)
	}
	return nil
}

// GetCapture returns the calling thread's capture window, or 0.
func GetCapture() windows.HWND {
	r, _, _ := callV(procGetCapture)
	return windows.HWND(r)
}

// TrackMouseEvent asks for WM_MOUSELEAVE/WM_MOUSEHOVER (flags TME_*). It
// sets t.CbSize.
func TrackMouseEvent(t *TrackMouseEventInfo) error {
	t.CbSize = uint32(unsafe.Sizeof(*t))
	r, _, err := procTrackMouseEvent.Call(uintptr(unsafe.Pointer(t)))
	if r == 0 {
		return lastError("TrackMouseEvent", err)
	}
	return nil
}

// LoadCursor loads the standard cursor id (IDC_*). The handle is shared
// and must not be destroyed.
func LoadCursor(id uintptr) (windows.Handle, error) {
	r, _, err := callV(procLoadCursorW, 0, id)
	if r == 0 {
		return 0, lastError("LoadCursorW", err)
	}
	return windows.Handle(r), nil
}

// SetCursor sets the cursor (0 hides it) and returns the previous one.
func SetCursor(c windows.Handle) windows.Handle {
	r, _, _ := callV(procSetCursor, uintptr(c))
	return windows.Handle(r)
}

// GetKeyState returns the state of virtual key vk as of the current
// message: the high bit is set while it is down, the low bit when it is
// toggled (Caps Lock).
func GetKeyState(vk int32) int16 {
	r, _, _ := callV(procGetKeyState, uintptr(vk))
	return int16(r)
}

// GetKeyboardLayout returns the active keyboard layout (HKL) of thread
// (0: the calling thread).
func GetKeyboardLayout(thread uint32) windows.Handle {
	r, _, _ := callV(procGetKeyboardLayout, uintptr(thread))
	return windows.Handle(r)
}

// MapVirtualKeyEx translates code with mapType (MAPVK_*) in layout hkl;
// 0 means no translation.
func MapVirtualKeyEx(code, mapType uint32, hkl windows.Handle) uint32 {
	r, _, _ := callV(procMapVirtualKeyExW, uintptr(code), uintptr(mapType), uintptr(hkl))
	return uint32(r)
}

// ToUnicodeEx translates virtual key vk with scan code scan under the
// keyboard state (256 key bytes, the high bit for a held key) in layout
// hkl into buf and returns ToUnicodeEx's result: the number of UTF-16
// units written, negative for a dead key (its spacing character written),
// 0 for none. flags is ToUnicodeEx's wFlags; TOUNICODE_NOSTATE leaves the
// kernel's keyboard state, a pending dead key included, unchanged.
func ToUnicodeEx(vk, scan uint32, state *[256]byte, buf []uint16, flags uint32, hkl windows.Handle) int32 {
	if len(buf) == 0 {
		return 0
	}
	r, _, _ := procToUnicodeEx.Call(uintptr(vk), uintptr(scan), uintptr(unsafe.Pointer(state)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(flags), uintptr(hkl))
	return int32(r)
}

// GetKeyboardLayoutList returns the keyboard layouts loaded for the
// session.
func GetKeyboardLayoutList() []windows.Handle {
	n, _, _ := callV(procGetKeyboardLayoutList, 0, 0)
	if n == 0 {
		return nil
	}
	out := make([]windows.Handle, n)
	n, _, _ = procGetKeyboardLayoutList.Call(n, uintptr(unsafe.Pointer(&out[0])))
	return out[:n]
}

// SetForegroundWindow brings h to the foreground if Windows allows it and
// reports whether it did.
func SetForegroundWindow(h windows.HWND) bool {
	r, _, _ := callV(procSetForegroundWindow, uintptr(h))
	return r != 0
}

// ---- custom chrome (S2.3d) ----

// GetCursorPos returns the cursor position in screen pixels.
func GetCursorPos() (Point, error) {
	var p Point
	r, _, err := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	if r == 0 {
		return Point{}, lastError("GetCursorPos", err)
	}
	return p, nil
}

// WindowFromPoint returns the window at the screen point pt (0 if none).
func WindowFromPoint(pt Point) windows.HWND {
	r, _, _ := callV(procWindowFromPoint, packPoint(pt))
	return windows.HWND(r)
}

// GetTickCount returns the milliseconds since system start, on the same
// wrapping timeline as GetMessageTime.
func GetTickCount() uint32 {
	r, _, _ := callV(procGetTickCount)
	return uint32(r)
}
