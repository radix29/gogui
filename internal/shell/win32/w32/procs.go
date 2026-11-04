//go:build windows

package w32

import "golang.org/x/sys/windows"

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	dwmapi   = windows.NewLazySystemDLL("dwmapi.dll")
	shcore   = windows.NewLazySystemDLL("shcore.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
)

// allProcs lists every proc declared with newProc, for TestProcsResolve.
var allProcs []*windows.LazyProc

func newProc(dll *windows.LazyDLL, name string) *windows.LazyProc {
	p := dll.NewProc(name)
	allProcs = append(allProcs, p)
	return p
}

// user32: window class and lifecycle.
var (
	procRegisterClassExW = newProc(user32, "RegisterClassExW")
	procUnregisterClassW = newProc(user32, "UnregisterClassW")
	procCreateWindowExW  = newProc(user32, "CreateWindowExW")
	procDestroyWindow    = newProc(user32, "DestroyWindow")
	procDefWindowProcW   = newProc(user32, "DefWindowProcW")
	procShowWindow       = newProc(user32, "ShowWindow")
	procSetWindowTextW   = newProc(user32, "SetWindowTextW")
	procGetWindowLongPtr = newProc(user32, "GetWindowLongPtrW")
	procSetWindowLongPtr = newProc(user32, "SetWindowLongPtrW")
)

// user32: message loop.
var (
	procGetMessageW      = newProc(user32, "GetMessageW")
	procPeekMessageW     = newProc(user32, "PeekMessageW")
	procTranslateMessage = newProc(user32, "TranslateMessage")
	procDispatchMessageW = newProc(user32, "DispatchMessageW")
	procPostMessageW     = newProc(user32, "PostMessageW")
	procPostQuitMessage  = newProc(user32, "PostQuitMessage")
	procSendMessageW     = newProc(user32, "SendMessageW")
	procGetMessageTime   = newProc(user32, "GetMessageTime")
)

// user32: geometry, placement and monitors.
var (
	procSetWindowPos             = newProc(user32, "SetWindowPos")
	procGetWindowRect            = newProc(user32, "GetWindowRect")
	procGetClientRect            = newProc(user32, "GetClientRect")
	procAdjustWindowRectExForDpi = newProc(user32, "AdjustWindowRectExForDpi")
	procGetWindowPlacement       = newProc(user32, "GetWindowPlacement")
	procSetWindowPlacement       = newProc(user32, "SetWindowPlacement")
	procIsZoomed                 = newProc(user32, "IsZoomed")
	procIsIconic                 = newProc(user32, "IsIconic")
	procScreenToClient           = newProc(user32, "ScreenToClient")
	procClientToScreen           = newProc(user32, "ClientToScreen")
	procMonitorFromWindow        = newProc(user32, "MonitorFromWindow")
	procMonitorFromPoint         = newProc(user32, "MonitorFromPoint")
	procGetMonitorInfoW          = newProc(user32, "GetMonitorInfoW")
)

// user32: DPI and metrics.
var (
	procSetProcessDpiAwarenessContext = newProc(user32, "SetProcessDpiAwarenessContext")
	procGetDpiForWindow               = newProc(user32, "GetDpiForWindow")
	procGetDpiForSystem               = newProc(user32, "GetDpiForSystem")
	procGetSystemMetrics              = newProc(user32, "GetSystemMetrics")
	procGetSystemMetricsForDpi        = newProc(user32, "GetSystemMetricsForDpi")
	procSystemParametersInfoW         = newProc(user32, "SystemParametersInfoW")
	procGetDoubleClickTime            = newProc(user32, "GetDoubleClickTime")

	// S2.3b: checking the awareness Run set.
	procGetThreadDpiAwarenessContext = newProc(user32, "GetThreadDpiAwarenessContext")
	procAreDpiAwarenessContextsEqual = newProc(user32, "AreDpiAwarenessContextsEqual")
)

// user32: timers (S2.3b, the interim frame clock's pacing).
var (
	procSetTimer  = newProc(user32, "SetTimer")
	procKillTimer = newProc(user32, "KillTimer")
)

// user32: input, capture, cursor and keyboard layout.
var (
	procSetCapture          = newProc(user32, "SetCapture")
	procReleaseCapture      = newProc(user32, "ReleaseCapture")
	procGetCapture          = newProc(user32, "GetCapture")
	procTrackMouseEvent     = newProc(user32, "TrackMouseEvent")
	procLoadCursorW         = newProc(user32, "LoadCursorW")
	procSetCursor           = newProc(user32, "SetCursor")
	procGetKeyState         = newProc(user32, "GetKeyState")
	procGetKeyboardLayout   = newProc(user32, "GetKeyboardLayout")
	procMapVirtualKeyExW    = newProc(user32, "MapVirtualKeyExW")
	procSetForegroundWindow = newProc(user32, "SetForegroundWindow")
)

// user32: interim GDI painting (S2.3b, until D2D in S2.4).
var (
	procBeginPaint     = newProc(user32, "BeginPaint")
	procEndPaint       = newProc(user32, "EndPaint")
	procFillRect       = newProc(user32, "FillRect")
	procInvalidateRect = newProc(user32, "InvalidateRect")
)

// user32: surfaces (S2.3c).
var (
	procUpdateWindow    = newProc(user32, "UpdateWindow")
	procIsWindowVisible = newProc(user32, "IsWindowVisible")
)

// user32: custom chrome (S2.3d), the pointer check after a leave.
var (
	procGetCursorPos    = newProc(user32, "GetCursorPos")
	procWindowFromPoint = newProc(user32, "WindowFromPoint")
)

// gdi32.
var (
	procCreateSolidBrush = newProc(gdi32, "CreateSolidBrush")
	procDeleteObject     = newProc(gdi32, "DeleteObject")
)

// user32 and gdi32: window capture (S2.4d_3, the tests' PrintWindow
// cross-check).
var (
	procPrintWindow        = newProc(user32, "PrintWindow")
	procCreateCompatibleDC = newProc(gdi32, "CreateCompatibleDC")
	procCreateDIBSection   = newProc(gdi32, "CreateDIBSection")
	procSelectObject       = newProc(gdi32, "SelectObject")
	procDeleteDC           = newProc(gdi32, "DeleteDC")
	procGdiFlush           = newProc(gdi32, "GdiFlush")
	procFindWindowExW      = newProc(user32, "FindWindowExW")
)

// dwmapi. DwmSetWindowAttribute comes from x/sys/windows; it is listed
// here only so TestProcsResolve covers it.
var (
	procDwmExtendFrameIntoClientArea = newProc(dwmapi, "DwmExtendFrameIntoClientArea")
	procDwmGetColorizationColor      = newProc(dwmapi, "DwmGetColorizationColor")
	_                                = newProc(dwmapi, "DwmSetWindowAttribute")
)

// shcore.
var procGetDpiForMonitor = newProc(shcore, "GetDpiForMonitor")

// kernel32. GetCurrentThreadId comes from x/sys/windows.
var (
	procGetModuleHandleW = newProc(kernel32, "GetModuleHandleW")
	procSetLastError     = newProc(kernel32, "SetLastError")
	_                    = newProc(kernel32, "GetCurrentThreadId")

	// S2.3e: mapping GetMessageTime onto the frame clock.
	procGetTickCount = newProc(kernel32, "GetTickCount")
)

// The paced frame clock (S2.4f): waiting on the swap chains' waitable
// objects and a high-resolution timer while messages keep flowing, the QPC
// timeline and DWM's refresh timing.
var (
	procMsgWaitForMultipleObjectsEx = newProc(user32, "MsgWaitForMultipleObjectsEx")
	procCreateWaitableTimerExW      = newProc(kernel32, "CreateWaitableTimerExW")
	procSetWaitableTimer            = newProc(kernel32, "SetWaitableTimer")
	procCancelWaitableTimer         = newProc(kernel32, "CancelWaitableTimer")
	procQueryPerformanceCounter     = newProc(kernel32, "QueryPerformanceCounter")
	procQueryPerformanceFrequency   = newProc(kernel32, "QueryPerformanceFrequency")
	procDwmGetCompositionTimingInfo = newProc(dwmapi, "DwmGetCompositionTimingInfo")
)
