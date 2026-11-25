//go:build windows

package w32

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// ---- geometry, placement and monitors ----

// SetWindowPos moves, sizes and reorders h (flags: SWP_*). after is an
// HWND or one of the HWND_TOP… values.
func SetWindowPos(h windows.HWND, after uintptr, x, y, cx, cy int32, flags uint32) error {
	r, _, err := callV(procSetWindowPos, uintptr(h), after,
		uintptr(x), uintptr(y), uintptr(cx), uintptr(cy), uintptr(flags))
	if r == 0 {
		return lastError("SetWindowPos", err)
	}
	return nil
}

// GetWindowRect returns h's window rectangle in screen pixels.
func GetWindowRect(h windows.HWND) (Rect, error) {
	var rc Rect
	r, _, err := procGetWindowRect.Call(uintptr(h), uintptr(unsafe.Pointer(&rc)))
	if r == 0 {
		return Rect{}, lastError("GetWindowRect", err)
	}
	return rc, nil
}

// GetClientRect returns h's client rectangle (origin 0, 0) in pixels.
func GetClientRect(h windows.HWND) (Rect, error) {
	var rc Rect
	r, _, err := procGetClientRect.Call(uintptr(h), uintptr(unsafe.Pointer(&rc)))
	if r == 0 {
		return Rect{}, lastError("GetClientRect", err)
	}
	return rc, nil
}

// AdjustWindowRectExForDpi grows the client rectangle rc to the window
// rectangle for the styles at dpi.
func AdjustWindowRectExForDpi(rc *Rect, style uint32, menu bool, exStyle, dpi uint32) error {
	r, _, err := procAdjustWindowRectExForDpi.Call(uintptr(unsafe.Pointer(rc)),
		uintptr(style), boolArg(menu), uintptr(exStyle), uintptr(dpi))
	if r == 0 {
		return lastError("AdjustWindowRectExForDpi", err)
	}
	return nil
}

// GetWindowPlacement returns h's show state and restored rectangle.
func GetWindowPlacement(h windows.HWND) (WindowPlacement, error) {
	wp := WindowPlacement{Length: uint32(unsafe.Sizeof(WindowPlacement{}))}
	r, _, err := procGetWindowPlacement.Call(uintptr(h), uintptr(unsafe.Pointer(&wp)))
	if r == 0 {
		return WindowPlacement{}, lastError("GetWindowPlacement", err)
	}
	return wp, nil
}

// SetWindowPlacement sets h's show state and restored rectangle. It sets
// wp.Length.
func SetWindowPlacement(h windows.HWND, wp *WindowPlacement) error {
	wp.Length = uint32(unsafe.Sizeof(*wp))
	r, _, err := procSetWindowPlacement.Call(uintptr(h), uintptr(unsafe.Pointer(wp)))
	if r == 0 {
		return lastError("SetWindowPlacement", err)
	}
	return nil
}

// IsZoomed reports whether h is maximized.
func IsZoomed(h windows.HWND) bool {
	r, _, _ := callV(procIsZoomed, uintptr(h))
	return r != 0
}

// IsIconic reports whether h is minimized.
func IsIconic(h windows.HWND) bool {
	r, _, _ := callV(procIsIconic, uintptr(h))
	return r != 0
}

// ScreenToClient converts pt from screen to h's client coordinates.
func ScreenToClient(h windows.HWND, pt *Point) error {
	r, _, _ := procScreenToClient.Call(uintptr(h), uintptr(unsafe.Pointer(pt)))
	if r == 0 {
		return failed("ScreenToClient")
	}
	return nil
}

// ClientToScreen converts pt from h's client to screen coordinates.
func ClientToScreen(h windows.HWND, pt *Point) error {
	r, _, _ := procClientToScreen.Call(uintptr(h), uintptr(unsafe.Pointer(pt)))
	if r == 0 {
		return failed("ClientToScreen")
	}
	return nil
}

// MonitorFromWindow returns the monitor h is mostly on (flags:
// MONITOR_DEFAULTTO*); 0 only with MONITOR_DEFAULTTONULL.
func MonitorFromWindow(h windows.HWND, flags uint32) windows.Handle {
	r, _, _ := callV(procMonitorFromWindow, uintptr(h), uintptr(flags))
	return windows.Handle(r)
}

// MonitorFromPoint returns the monitor that contains pt (screen pixels).
func MonitorFromPoint(pt Point, flags uint32) windows.Handle {
	r, _, _ := callV(procMonitorFromPoint, packPoint(pt), uintptr(flags))
	return windows.Handle(r)
}

// MonitorFromRect returns the monitor with the largest intersection with
// rc (screen pixels; flags: MONITOR_DEFAULTTO*).
func MonitorFromRect(rc *Rect, flags uint32) windows.Handle {
	r, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(rc)), uintptr(flags))
	return windows.Handle(r)
}

// GetMonitorInfo returns the monitor and work-area rectangles of mon.
func GetMonitorInfo(mon windows.Handle) (MonitorInfo, error) {
	mi := MonitorInfo{CbSize: uint32(unsafe.Sizeof(MonitorInfo{}))}
	r, _, _ := procGetMonitorInfoW.Call(uintptr(mon), uintptr(unsafe.Pointer(&mi)))
	if r == 0 {
		return MonitorInfo{}, failed("GetMonitorInfoW")
	}
	return mi, nil
}

// ---- DPI and metrics ----

// SetProcessDpiAwarenessContext sets the process's DPI awareness
// (DPI_AWARENESS_CONTEXT_*). It fails with ERROR_ACCESS_DENIED once the
// awareness is set (by a manifest or an earlier call).
func SetProcessDpiAwarenessContext(ctx uintptr) error {
	r, _, err := callV(procSetProcessDpiAwarenessContext, ctx)
	if r == 0 {
		return lastError("SetProcessDpiAwarenessContext", err)
	}
	return nil
}

// GetThreadDpiAwarenessContext returns the calling thread's DPI awareness
// context (a pseudo-handle; compare it with AreDpiAwarenessContextsEqual).
func GetThreadDpiAwarenessContext() uintptr {
	r, _, _ := callV(procGetThreadDpiAwarenessContext)
	return r
}

// AreDpiAwarenessContextsEqual reports whether two DPI awareness contexts
// mean the same awareness.
func AreDpiAwarenessContextsEqual(a, b uintptr) bool {
	r, _, _ := callV(procAreDpiAwarenessContextsEqual, a, b)
	return r != 0
}

// GetDpiForWindow returns h's DPI (96 = 100 %), or 0 for an invalid h.
func GetDpiForWindow(h windows.HWND) uint32 {
	r, _, _ := callV(procGetDpiForWindow, uintptr(h))
	return uint32(r)
}

// GetDpiForSystem returns the system DPI of the calling thread's awareness.
func GetDpiForSystem() uint32 {
	r, _, _ := callV(procGetDpiForSystem)
	return uint32(r)
}

// GetSystemMetrics returns metric index (SM_*) at the system DPI.
func GetSystemMetrics(index int32) int32 {
	r, _, _ := callV(procGetSystemMetrics, uintptr(index))
	return int32(r)
}

// GetSystemMetricsForDpi returns metric index (SM_*) at dpi.
func GetSystemMetricsForDpi(index int32, dpi uint32) int32 {
	r, _, _ := callV(procGetSystemMetricsForDpi, uintptr(index), uintptr(dpi))
	return int32(r)
}

// SystemParametersInfo is SystemParametersInfoW. pv points at the
// action's out-value (a uint32, a BOOL, a Rect, …) for the call.
func SystemParametersInfo(action, uiParam uint32, pv unsafe.Pointer, winIni uint32) error {
	r, _, err := procSystemParametersInfoW.Call(uintptr(action), uintptr(uiParam),
		uintptr(pv), uintptr(winIni))
	if r == 0 {
		return lastError("SystemParametersInfoW", err)
	}
	return nil
}

// SystemParametersInfoUint32 reads an action whose value is a UINT or a
// BOOL (SPI_GETWHEELSCROLLLINES, SPI_GETCLIENTAREAANIMATION, …).
func SystemParametersInfoUint32(action uint32) (uint32, error) {
	var v uint32
	err := SystemParametersInfo(action, 0, unsafe.Pointer(&v), 0)
	return v, err
}

// SystemParametersInfoHighContrast reads SPI_GETHIGHCONTRAST. The scheme
// name pointer is Windows' and is not read here.
func SystemParametersInfoHighContrast() (HighContrast, error) {
	hc := HighContrast{CbSize: uint32(unsafe.Sizeof(HighContrast{}))}
	err := SystemParametersInfo(SPI_GETHIGHCONTRAST, hc.CbSize, unsafe.Pointer(&hc), 0)
	return hc, err
}

// GetDoubleClickTime returns the double-click interval in milliseconds.
func GetDoubleClickTime() uint32 {
	r, _, _ := callV(procGetDoubleClickTime)
	return uint32(r)
}

// GetCaretBlinkTime returns the caret's half-period in milliseconds (the
// time it is shown, then hidden): INFINITE when the caret does not blink.
// It fails with 0.
func GetCaretBlinkTime() (uint32, error) {
	r, _, err := callV(procGetCaretBlinkTime)
	if r == 0 {
		return 0, lastError("GetCaretBlinkTime", err)
	}
	return uint32(r), nil
}
