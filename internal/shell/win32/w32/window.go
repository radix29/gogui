//go:build windows

package w32

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

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
	r, _, err := callV(procDestroyWindow, uintptr(h))
	if r == 0 {
		return lastError("DestroyWindow", err)
	}
	return nil
}

// DefWindowProc is the default window procedure.
func DefWindowProc(h windows.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	r, _, _ := callV(procDefWindowProcW, uintptr(h), uintptr(msg), wParam, lParam)
	return r
}

// DefWindowProcAddr is the address of DefWindowProcW, usable as a window
// class's wndproc (tests use it for windows that need no Go callback).
func DefWindowProcAddr() uintptr { return procDefWindowProcW.Addr() }

// ShowWindow sets h's show state (SW_*) and reports whether h was visible.
func ShowWindow(h windows.HWND, cmd int32) bool {
	r, _, _ := callV(procShowWindow, uintptr(h), uintptr(cmd))
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
	r, _, err := callV(procGetWindowLongPtr, uintptr(h), uintptr(index))
	if r == 0 && err != 0 {
		return 0, lastError("GetWindowLongPtrW", err)
	}
	return r, nil
}

// SetWindowLongPtr stores v at index and returns the previous value, with
// the error rule of GetWindowLongPtr. v must not be a Go pointer; for
// GWLP_USERDATA it is an ffi.Handle.
func SetWindowLongPtr(h windows.HWND, index int32, v uintptr) (uintptr, error) {
	SetLastError(0)
	r, _, err := callV(procSetWindowLongPtr, uintptr(h), uintptr(index), v)
	if r == 0 && err != 0 {
		return 0, lastError("SetWindowLongPtrW", err)
	}
	return r, nil
}

// ---- message loop ----

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
	r, _, err := callV(procPostMessageW, uintptr(h), uintptr(msg), wParam, lParam)
	if r == 0 {
		return lastError("PostMessageW", err)
	}
	return nil
}

// PostQuitMessage makes the calling thread's GetMessage return WM_QUIT
// with code once its queue is empty.
func PostQuitMessage(code int32) {
	callV(procPostQuitMessage, uintptr(code))
}

// SendMessage calls h's window procedure and returns its result.
func SendMessage(h windows.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	r, _, _ := callV(procSendMessageW, uintptr(h), uintptr(msg), wParam, lParam)
	return r
}

// GetMessageTime returns the time of the last message GetMessage
// retrieved, in milliseconds since system start (it wraps).
func GetMessageTime() int32 {
	r, _, _ := callV(procGetMessageTime)
	return int32(r)
}

// ---- modal surfaces (S5.7e) ----

// EnableWindow enables or disables mouse and keyboard input to h and
// reports whether it was disabled before. Disabling sends WM_CANCELMODE.
func EnableWindow(h windows.HWND, enable bool) (wasDisabled bool) {
	r, _, _ := callV(procEnableWindow, uintptr(h), boolArg(enable))
	return r != 0
}

// IsWindowEnabled reports whether h takes mouse and keyboard input.
func IsWindowEnabled(h windows.HWND) bool {
	r, _, _ := callV(procIsWindowEnabled, uintptr(h))
	return r != 0
}

// GetActiveWindow returns the active window of the calling thread's
// message queue, or 0.
func GetActiveWindow() windows.HWND {
	r, _, _ := callV(procGetActiveWindow)
	return windows.HWND(r)
}

// SetActiveWindow activates h, a window of the calling thread, and returns
// the window active before (0 on failure or without one).
func SetActiveWindow(h windows.HWND) windows.HWND {
	r, _, _ := callV(procSetActiveWindow, uintptr(h))
	return windows.HWND(r)
}

// GetForegroundWindow returns the window the user works with, or 0.
func GetForegroundWindow() windows.HWND {
	r, _, _ := callV(procGetForegroundWindow)
	return windows.HWND(r)
}
