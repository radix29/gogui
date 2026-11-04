//go:build windows

package w32

import (
	"encoding/binary"
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// lastError turns the error LazyProc.Call returned for a failed call into
// the wrapper's error. Call always returns a windows.Errno; a zero Errno
// means the API failed without setting the last error.
func lastError(name string, err error) error {
	if e, ok := err.(windows.Errno); ok && e == 0 {
		return fmt.Errorf("w32: %s failed (no error code)", name)
	}
	return fmt.Errorf("w32: %s: %w", name, err)
}

// failed is lastError for the APIs that document no last error (BeginPaint,
// ScreenToClient, …).
func failed(name string) error {
	return fmt.Errorf("w32: %s failed", name)
}

// hresult turns an HRESULT into nil (S_OK and other success codes) or an
// error wrapping it as a windows.Errno, as x/sys/windows does.
func hresult(name string, hr uintptr) error {
	if int32(hr) >= 0 {
		return nil
	}
	return fmt.Errorf("w32: %s: %w", name, windows.Errno(hr))
}

func boolArg(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}

// packPoint passes a POINT by value. On amd64 and arm64 an 8-byte struct
// travels in one integer register, X in the low and Y in the high half.
func packPoint(p Point) uintptr {
	return uintptr(uint32(p.X)) | uintptr(uint32(p.Y))<<32
}

// ---- kernel32 ----

// GetModuleHandle returns the module handle of the executable
// (GetModuleHandleW(NULL)), the hInstance of the window class.
func GetModuleHandle() (windows.Handle, error) {
	r, _, err := procGetModuleHandleW.Call(0)
	if r == 0 {
		return 0, lastError("GetModuleHandleW", err)
	}
	return windows.Handle(r), nil
}

// SetLastError sets the calling thread's last-error code.
func SetLastError(code uint32) {
	procSetLastError.Call(uintptr(code))
}

// ---- window class and lifecycle ----

// RegisterClassEx registers wc under name and returns the class atom. It
// sets wc.CbSize and wc.LpszClassName; the name buffer is pinned for the
// call, and Windows copies it.
func RegisterClassEx(wc *WndClassEx, name string) (uint16, error) {
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, fmt.Errorf("w32: RegisterClassExW: %w", err)
	}
	var pin runtime.Pinner
	pin.Pin(p)
	defer pin.Unpin()
	wc.CbSize = uint32(unsafe.Sizeof(*wc))
	wc.LpszClassName = uintptr(unsafe.Pointer(p))
	r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(wc)))
	wc.LpszClassName = 0 // the pinned buffer must not outlive the call in wc
	if r == 0 {
		return 0, lastError("RegisterClassExW", err)
	}
	return uint16(r), nil
}

// UnregisterClass unregisters the class name of module inst. Every window
// of the class must be destroyed first.
func UnregisterClass(name string, inst windows.Handle) error {
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return fmt.Errorf("w32: UnregisterClassW: %w", err)
	}
	r, _, err := procUnregisterClassW.Call(uintptr(unsafe.Pointer(p)), uintptr(inst))
	if r == 0 {
		return lastError("UnregisterClassW", err)
	}
	return nil
}

// CreateWindowEx creates a window of the registered class. param arrives
// as CREATESTRUCTW.lpCreateParams in WM_NCCREATE and must be an ffi.Handle
// or 0, never a Go pointer.
func CreateWindowEx(exStyle uint32, class, title string, style uint32,
	x, y, w, h int32, parent windows.HWND, menu, inst windows.Handle, param uintptr,
) (windows.HWND, error) {
	c, err := windows.UTF16PtrFromString(class)
	if err != nil {
		return 0, fmt.Errorf("w32: CreateWindowExW: %w", err)
	}
	t, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return 0, fmt.Errorf("w32: CreateWindowExW: %w", err)
	}
	r, _, err := procCreateWindowExW.Call(uintptr(exStyle),
		uintptr(unsafe.Pointer(c)), uintptr(unsafe.Pointer(t)), uintptr(style),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		uintptr(parent), uintptr(menu), uintptr(inst), param)
	if r == 0 {
		return 0, lastError("CreateWindowExW", err)
	}
	return windows.HWND(r), nil
}

// DestroyWindow destroys h (sends WM_DESTROY and WM_NCDESTROY).
func DestroyWindow(h windows.HWND) error {
	r, _, err := procDestroyWindow.Call(uintptr(h))
	if r == 0 {
		return lastError("DestroyWindow", err)
	}
	return nil
}

// DefWindowProc is the default window procedure.
func DefWindowProc(h windows.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	r, _, _ := procDefWindowProcW.Call(uintptr(h), uintptr(msg), wParam, lParam)
	return r
}

// DefWindowProcAddr is the address of DefWindowProcW, usable as a window
// class's wndproc (tests use it for windows that need no Go callback).
func DefWindowProcAddr() uintptr { return procDefWindowProcW.Addr() }

// ShowWindow sets h's show state (SW_*) and reports whether h was visible.
func ShowWindow(h windows.HWND, cmd int32) bool {
	r, _, _ := procShowWindow.Call(uintptr(h), uintptr(cmd))
	return r != 0
}

// SetWindowText sets h's title.
func SetWindowText(h windows.HWND, s string) error {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		return fmt.Errorf("w32: SetWindowTextW: %w", err)
	}
	r, _, err := procSetWindowTextW.Call(uintptr(h), uintptr(unsafe.Pointer(p)))
	if r == 0 {
		return lastError("SetWindowTextW", err)
	}
	return nil
}

// GetWindowLongPtr returns the window value at index (GWLP_USERDATA, …).
// 0 is a valid value, so it reports an error only for a 0 result with a
// non-zero last error, which it clears first. Call it on the window's
// thread (a locked OS thread).
func GetWindowLongPtr(h windows.HWND, index int32) (uintptr, error) {
	SetLastError(0)
	r, _, err := procGetWindowLongPtr.Call(uintptr(h), uintptr(index))
	if r == 0 && err.(windows.Errno) != 0 {
		return 0, lastError("GetWindowLongPtrW", err)
	}
	return r, nil
}

// SetWindowLongPtr stores v at index and returns the previous value, with
// the error rule of GetWindowLongPtr. v must not be a Go pointer; for
// GWLP_USERDATA it is an ffi.Handle.
func SetWindowLongPtr(h windows.HWND, index int32, v uintptr) (uintptr, error) {
	SetLastError(0)
	r, _, err := procSetWindowLongPtr.Call(uintptr(h), uintptr(index), v)
	if r == 0 && err.(windows.Errno) != 0 {
		return 0, lastError("SetWindowLongPtrW", err)
	}
	return r, nil
}

// ---- message loop ----

// GetMessage waits for the next message of the thread (h 0) or window h
// in [minMsg, maxMsg] (both 0 for all). It returns (true, nil) for a
// message, (false, nil) for WM_QUIT (msg.WParam is the exit code) and
// (false, err) when GetMessageW returns -1 (an invalid h or msg).
func GetMessage(msg *Msg, h windows.HWND, minMsg, maxMsg uint32) (bool, error) {
	r, _, err := procGetMessageW.Call(uintptr(unsafe.Pointer(msg)), uintptr(h),
		uintptr(minMsg), uintptr(maxMsg))
	switch int32(r) {
	case -1:
		return false, lastError("GetMessageW", err)
	case 0:
		return false, nil
	}
	return true, nil
}

// PeekMessage checks for a message without waiting (remove: PM_*) and
// reports whether there was one.
func PeekMessage(msg *Msg, h windows.HWND, minMsg, maxMsg, remove uint32) bool {
	r, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(msg)), uintptr(h),
		uintptr(minMsg), uintptr(maxMsg), uintptr(remove))
	return r != 0
}

// TranslateMessage posts WM_CHAR for a key message and reports whether it
// did.
func TranslateMessage(msg *Msg) bool {
	r, _, _ := procTranslateMessage.Call(uintptr(unsafe.Pointer(msg)))
	return r != 0
}

// DispatchMessage sends msg to its window procedure and returns its
// result.
func DispatchMessage(msg *Msg) uintptr {
	r, _, _ := procDispatchMessageW.Call(uintptr(unsafe.Pointer(msg)))
	return r
}

// PostMessage queues a message for h's thread and returns at once. It is
// safe from any thread.
func PostMessage(h windows.HWND, msg uint32, wParam, lParam uintptr) error {
	r, _, err := procPostMessageW.Call(uintptr(h), uintptr(msg), wParam, lParam)
	if r == 0 {
		return lastError("PostMessageW", err)
	}
	return nil
}

// PostQuitMessage makes the calling thread's GetMessage return WM_QUIT
// with code once its queue is empty.
func PostQuitMessage(code int32) {
	procPostQuitMessage.Call(uintptr(code))
}

// SendMessage calls h's window procedure and returns its result.
func SendMessage(h windows.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	r, _, _ := procSendMessageW.Call(uintptr(h), uintptr(msg), wParam, lParam)
	return r
}

// GetMessageTime returns the time of the last message GetMessage
// retrieved, in milliseconds since system start (it wraps).
func GetMessageTime() int32 {
	r, _, _ := procGetMessageTime.Call()
	return int32(r)
}

// ---- geometry, placement and monitors ----

// SetWindowPos moves, sizes and reorders h (flags: SWP_*). after is an
// HWND or one of the HWND_TOP… values.
func SetWindowPos(h windows.HWND, after uintptr, x, y, cx, cy int32, flags uint32) error {
	r, _, err := procSetWindowPos.Call(uintptr(h), after,
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
	r, _, _ := procIsZoomed.Call(uintptr(h))
	return r != 0
}

// IsIconic reports whether h is minimized.
func IsIconic(h windows.HWND) bool {
	r, _, _ := procIsIconic.Call(uintptr(h))
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
	r, _, _ := procMonitorFromWindow.Call(uintptr(h), uintptr(flags))
	return windows.Handle(r)
}

// MonitorFromPoint returns the monitor that contains pt (screen pixels).
func MonitorFromPoint(pt Point, flags uint32) windows.Handle {
	r, _, _ := procMonitorFromPoint.Call(packPoint(pt), uintptr(flags))
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
	r, _, err := procSetProcessDpiAwarenessContext.Call(ctx)
	if r == 0 {
		return lastError("SetProcessDpiAwarenessContext", err)
	}
	return nil
}

// GetThreadDpiAwarenessContext returns the calling thread's DPI awareness
// context (a pseudo-handle; compare it with AreDpiAwarenessContextsEqual).
func GetThreadDpiAwarenessContext() uintptr {
	r, _, _ := procGetThreadDpiAwarenessContext.Call()
	return r
}

// AreDpiAwarenessContextsEqual reports whether two DPI awareness contexts
// mean the same awareness.
func AreDpiAwarenessContextsEqual(a, b uintptr) bool {
	r, _, _ := procAreDpiAwarenessContextsEqual.Call(a, b)
	return r != 0
}

// GetDpiForWindow returns h's DPI (96 = 100 %), or 0 for an invalid h.
func GetDpiForWindow(h windows.HWND) uint32 {
	r, _, _ := procGetDpiForWindow.Call(uintptr(h))
	return uint32(r)
}

// GetDpiForSystem returns the system DPI of the calling thread's awareness.
func GetDpiForSystem() uint32 {
	r, _, _ := procGetDpiForSystem.Call()
	return uint32(r)
}

// GetSystemMetrics returns metric index (SM_*) at the system DPI.
func GetSystemMetrics(index int32) int32 {
	r, _, _ := procGetSystemMetrics.Call(uintptr(index))
	return int32(r)
}

// GetSystemMetricsForDpi returns metric index (SM_*) at dpi.
func GetSystemMetricsForDpi(index int32, dpi uint32) int32 {
	r, _, _ := procGetSystemMetricsForDpi.Call(uintptr(index), uintptr(dpi))
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
	r, _, _ := procGetDoubleClickTime.Call()
	return uint32(r)
}

// ---- timers ----

// SetTimer starts (or restarts) timer id of h: WM_TIMER with wParam id
// arrives every ms milliseconds (at least USER_TIMER_MINIMUM, 10 ms) until
// KillTimer.
func SetTimer(h windows.HWND, id uintptr, ms uint32) error {
	r, _, err := procSetTimer.Call(uintptr(h), id, uintptr(ms), 0)
	if r == 0 {
		return lastError("SetTimer", err)
	}
	return nil
}

// KillTimer stops timer id of h and removes its pending WM_TIMER.
func KillTimer(h windows.HWND, id uintptr) error {
	r, _, err := procKillTimer.Call(uintptr(h), id)
	if r == 0 {
		return lastError("KillTimer", err)
	}
	return nil
}

// ---- input, capture, cursor and keyboard layout ----

// SetCapture captures the mouse for h and returns the previous capture
// window (0 if none).
func SetCapture(h windows.HWND) windows.HWND {
	r, _, _ := procSetCapture.Call(uintptr(h))
	return windows.HWND(r)
}

// ReleaseCapture releases the mouse capture of the calling thread.
func ReleaseCapture() error {
	r, _, err := procReleaseCapture.Call()
	if r == 0 {
		return lastError("ReleaseCapture", err)
	}
	return nil
}

// GetCapture returns the calling thread's capture window, or 0.
func GetCapture() windows.HWND {
	r, _, _ := procGetCapture.Call()
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
	r, _, err := procLoadCursorW.Call(0, id)
	if r == 0 {
		return 0, lastError("LoadCursorW", err)
	}
	return windows.Handle(r), nil
}

// SetCursor sets the cursor (0 hides it) and returns the previous one.
func SetCursor(c windows.Handle) windows.Handle {
	r, _, _ := procSetCursor.Call(uintptr(c))
	return windows.Handle(r)
}

// GetKeyState returns the state of virtual key vk as of the current
// message: the high bit is set while it is down, the low bit when it is
// toggled (Caps Lock).
func GetKeyState(vk int32) int16 {
	r, _, _ := procGetKeyState.Call(uintptr(vk))
	return int16(r)
}

// GetKeyboardLayout returns the active keyboard layout (HKL) of thread
// (0: the calling thread).
func GetKeyboardLayout(thread uint32) windows.Handle {
	r, _, _ := procGetKeyboardLayout.Call(uintptr(thread))
	return windows.Handle(r)
}

// MapVirtualKeyEx translates code with mapType (MAPVK_*) in layout hkl;
// 0 means no translation.
func MapVirtualKeyEx(code, mapType uint32, hkl windows.Handle) uint32 {
	r, _, _ := procMapVirtualKeyExW.Call(uintptr(code), uintptr(mapType), uintptr(hkl))
	return uint32(r)
}

// SetForegroundWindow brings h to the foreground if Windows allows it and
// reports whether it did.
func SetForegroundWindow(h windows.HWND) bool {
	r, _, _ := procSetForegroundWindow.Call(uintptr(h))
	return r != 0
}

// ---- interim GDI painting ----

// BeginPaint starts painting h in WM_PAINT and returns the DC.
func BeginPaint(h windows.HWND, ps *PaintStruct) (windows.Handle, error) {
	r, _, _ := procBeginPaint.Call(uintptr(h), uintptr(unsafe.Pointer(ps)))
	if r == 0 {
		return 0, failed("BeginPaint")
	}
	return windows.Handle(r), nil
}

// EndPaint ends the painting BeginPaint started.
func EndPaint(h windows.HWND, ps *PaintStruct) {
	procEndPaint.Call(uintptr(h), uintptr(unsafe.Pointer(ps)))
}

// FillRect fills rc on dc with brush.
func FillRect(dc windows.Handle, rc *Rect, brush windows.Handle) error {
	r, _, _ := procFillRect.Call(uintptr(dc), uintptr(unsafe.Pointer(rc)), uintptr(brush))
	if r == 0 {
		return failed("FillRect")
	}
	return nil
}

// InvalidateRect adds rc (nil: the whole client area) to h's update region.
func InvalidateRect(h windows.HWND, rc *Rect, erase bool) error {
	r, _, _ := procInvalidateRect.Call(uintptr(h), uintptr(unsafe.Pointer(rc)), boolArg(erase))
	if r == 0 {
		return failed("InvalidateRect")
	}
	return nil
}

// UpdateWindow sends h a WM_PAINT at once if its update region is not
// empty (S2.3c).
func UpdateWindow(h windows.HWND) error {
	r, _, _ := procUpdateWindow.Call(uintptr(h))
	if r == 0 {
		return failed("UpdateWindow")
	}
	return nil
}

// IsWindowVisible reports whether h has WS_VISIBLE (S2.3c). A minimized
// window is visible.
func IsWindowVisible(h windows.HWND) bool {
	r, _, _ := procIsWindowVisible.Call(uintptr(h))
	return r != 0
}

// CreateSolidBrush creates a brush of colorref (0x00BBGGRR). Free it with
// DeleteObject.
func CreateSolidBrush(colorref uint32) (windows.Handle, error) {
	r, _, _ := procCreateSolidBrush.Call(uintptr(colorref))
	if r == 0 {
		return 0, failed("CreateSolidBrush")
	}
	return windows.Handle(r), nil
}

// DeleteObject frees a GDI object.
func DeleteObject(obj windows.Handle) error {
	r, _, _ := procDeleteObject.Call(uintptr(obj))
	if r == 0 {
		return failed("DeleteObject")
	}
	return nil
}

// ---- window capture (S2.4d_3) ----

// CreateCompatibleDC creates a memory DC compatible with dc (0: the
// screen). Free it with DeleteDC.
func CreateCompatibleDC(dc windows.Handle) (windows.Handle, error) {
	r, _, _ := procCreateCompatibleDC.Call(uintptr(dc))
	if r == 0 {
		return 0, failed("CreateCompatibleDC")
	}
	return windows.Handle(r), nil
}

// DeleteDC frees a DC from CreateCompatibleDC.
func DeleteDC(dc windows.Handle) error {
	r, _, _ := procDeleteDC.Call(uintptr(dc))
	if r == 0 {
		return failed("DeleteDC")
	}
	return nil
}

// CreateDIBSection creates a DIB of bi (BI_RGB, no color table) and
// returns it with the address of its pixels, which GDI owns until the
// bitmap is freed with DeleteObject.
func CreateDIBSection(dc windows.Handle, bi *BitmapInfoHeader) (bmp windows.Handle, bits uintptr, err error) {
	r, _, _ := procCreateDIBSection.Call(uintptr(dc), uintptr(unsafe.Pointer(bi)), DIB_RGB_COLORS,
		uintptr(unsafe.Pointer(&bits)), 0, 0)
	if r == 0 {
		return 0, 0, failed("CreateDIBSection")
	}
	return windows.Handle(r), bits, nil
}

// SelectObject selects obj into dc and returns the object it replaces.
func SelectObject(dc, obj windows.Handle) windows.Handle {
	r, _, _ := procSelectObject.Call(uintptr(dc), uintptr(obj))
	return windows.Handle(r)
}

// GdiFlush runs the calling thread's pending GDI drawing.
func GdiFlush() {
	procGdiFlush.Call()
}

// FindWindowEx returns the next top-level window after after (0: the
// first) of class and title, or 0.
func FindWindowEx(after windows.HWND, class, title string) windows.HWND {
	c, err := windows.UTF16PtrFromString(class)
	if err != nil {
		return 0
	}
	n, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return 0
	}
	r, _, _ := procFindWindowExW.Call(0, uintptr(after), uintptr(unsafe.Pointer(c)), uintptr(unsafe.Pointer(n)))
	runtime.KeepAlive(c)
	runtime.KeepAlive(n)
	return windows.HWND(r)
}

// PrintWindow draws h into dc (flags: PW_RENDERFULLCONTENT for windows
// that DWM composes, such as a DirectComposition swap chain).
func PrintWindow(h windows.HWND, dc windows.Handle, flags uint32) error {
	r, _, _ := procPrintWindow.Call(uintptr(h), uintptr(dc), uintptr(flags))
	if r == 0 {
		return failed("PrintWindow")
	}
	return nil
}

// ---- DWM and shcore ----

// DwmExtendFrameIntoClientArea extends the DWM frame (and its shadow) by m
// into h's client area.
func DwmExtendFrameIntoClientArea(h windows.HWND, m *Margins) error {
	r, _, _ := procDwmExtendFrameIntoClientArea.Call(uintptr(h), uintptr(unsafe.Pointer(m)))
	return hresult("DwmExtendFrameIntoClientArea", r)
}

// DwmGetColorizationColor returns the DWM colorization color (0xAARRGGBB)
// and whether it is opaque.
func DwmGetColorizationColor() (color uint32, opaque bool, err error) {
	var c uint32
	var o int32
	r, _, _ := procDwmGetColorizationColor.Call(uintptr(unsafe.Pointer(&c)), uintptr(unsafe.Pointer(&o)))
	if err := hresult("DwmGetColorizationColor", r); err != nil {
		return 0, false, err
	}
	return c, o != 0, nil
}

// DwmSetWindowAttributeInt sets a DWORD- or BOOL-valued DWM attribute
// (DWMWA_USE_IMMERSIVE_DARK_MODE, DWMWA_WINDOW_CORNER_PREFERENCE). An
// attribute this Windows doesn't know fails with E_INVALIDARG.
func DwmSetWindowAttributeInt(h windows.HWND, attr uint32, v int32) error {
	if err := windows.DwmSetWindowAttribute(h, attr, unsafe.Pointer(&v), 4); err != nil {
		return fmt.Errorf("w32: DwmSetWindowAttribute(%d): %w", attr, err)
	}
	return nil
}

// GetDpiForMonitor returns mon's DPI of type typ (MDT_*), before a window
// on it exists.
func GetDpiForMonitor(mon windows.Handle, typ uint32) (x, y uint32, err error) {
	r, _, _ := procGetDpiForMonitor.Call(uintptr(mon), uintptr(typ),
		uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y)))
	if err := hresult("GetDpiForMonitor", r); err != nil {
		return 0, 0, err
	}
	return x, y, nil
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
	r, _, _ := procWindowFromPoint.Call(packPoint(pt))
	return windows.HWND(r)
}

// GetTickCount returns the milliseconds since system start, on the same
// wrapping timeline as GetMessageTime.
func GetTickCount() uint32 {
	r, _, _ := procGetTickCount.Call()
	return uint32(r)
}

// ---- the paced frame clock (S2.4f) ----

// MsgWaitForMultipleObjectsEx waits until one of handles is signaled, the
// thread's queue has input of wakeMask (QS_*), or ms milliseconds have
// passed (INFINITE: no limit). It returns WAIT_OBJECT_0+i for handles[i],
// WAIT_OBJECT_0+len(handles) for input and WAIT_TIMEOUT; it dispatches
// nothing. At most MAXIMUM_WAIT_OBJECTS-1 handles.
func MsgWaitForMultipleObjectsEx(handles []windows.Handle, ms, wakeMask, flags uint32) (uint32, error) {
	var p uintptr
	if len(handles) > 0 {
		p = uintptr(unsafe.Pointer(&handles[0]))
	}
	r, _, err := procMsgWaitForMultipleObjectsEx.Call(uintptr(len(handles)), p, uintptr(ms),
		uintptr(wakeMask), uintptr(flags))
	runtime.KeepAlive(handles)
	if uint32(r) == WAIT_FAILED {
		return WAIT_FAILED, lastError("MsgWaitForMultipleObjectsEx", err)
	}
	return uint32(r), nil
}

// CreateWaitableTimer creates an unnamed auto-reset waitable timer, a
// high-resolution one (Windows 10 1803 and later) if highRes is set. Close
// it with windows.CloseHandle.
func CreateWaitableTimer(highRes bool) (windows.Handle, error) {
	var flags uint32
	if highRes {
		flags = CREATE_WAITABLE_TIMER_HIGH_RESOLUTION
	}
	r, _, err := procCreateWaitableTimerExW.Call(0, 0, uintptr(flags), TIMER_ALL_ACCESS)
	if r == 0 {
		return 0, lastError("CreateWaitableTimerExW", err)
	}
	return windows.Handle(r), nil
}

// SetWaitableTimer arms timer h to be signaled once, after100ns units of
// 100 ns from now (at least 1).
func SetWaitableTimer(h windows.Handle, after100ns int64) error {
	due := -max(after100ns, 1) // negative: relative
	r, _, err := procSetWaitableTimer.Call(uintptr(h), uintptr(unsafe.Pointer(&due)), 0, 0, 0, 0)
	if r == 0 {
		return lastError("SetWaitableTimer", err)
	}
	return nil
}

// CancelWaitableTimer disarms timer h (its signaled state is unchanged).
func CancelWaitableTimer(h windows.Handle) error {
	r, _, err := procCancelWaitableTimer.Call(uintptr(h))
	if r == 0 {
		return lastError("CancelWaitableTimer", err)
	}
	return nil
}

// QueryPerformanceCounter returns the performance counter, the timeline
// of DWM's timing (QueryPerformanceFrequency ticks per second).
func QueryPerformanceCounter() int64 {
	var v int64
	procQueryPerformanceCounter.Call(uintptr(unsafe.Pointer(&v)))
	return v
}

// QueryPerformanceFrequency returns the performance counter's ticks per
// second (fixed at boot).
func QueryPerformanceFrequency() int64 {
	var v int64
	procQueryPerformanceFrequency.Call(uintptr(unsafe.Pointer(&v)))
	return v
}

// CompositionTiming is the part of DWM_TIMING_INFO the frame clock uses,
// in performance-counter ticks.
type CompositionTiming struct {
	RefreshPeriod uint64 // qpcRefreshPeriod: the monitor's refresh period
	VBlank        uint64 // qpcVBlank: the time of a recent vblank
}

// dwmTimingInfoSize is sizeof(DWM_TIMING_INFO). dwmapi.h declares it under
// #pragma pack(1), so its 64-bit fields are unaligned and it is read from
// bytes rather than mirrored as a Go struct.
const dwmTimingInfoSize = 292

// Offsets in DWM_TIMING_INFO (packed): cbSize 0, rateRefresh 4,
// qpcRefreshPeriod 12, rateCompose 20, qpcVBlank 28.
const (
	dwmTimingRefreshPeriod = 12
	dwmTimingVBlank        = 28
)

// DwmGetCompositionTimingInfo returns DWM's composition timing for the
// desktop (hwnd NULL, as Windows 8.1 and later require).
func DwmGetCompositionTimingInfo() (CompositionTiming, error) {
	var buf [dwmTimingInfoSize]byte
	binary.LittleEndian.PutUint32(buf[0:], dwmTimingInfoSize)
	r, _, _ := procDwmGetCompositionTimingInfo.Call(0, uintptr(unsafe.Pointer(&buf[0])))
	if err := hresult("DwmGetCompositionTimingInfo", r); err != nil {
		return CompositionTiming{}, err
	}
	return CompositionTiming{
		RefreshPeriod: binary.LittleEndian.Uint64(buf[dwmTimingRefreshPeriod:]),
		VBlank:        binary.LittleEndian.Uint64(buf[dwmTimingVBlank:]),
	}, nil
}
