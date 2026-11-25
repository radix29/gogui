//go:build windows

package w32

import (
	"fmt"

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
	r, _, err := callV(procGetModuleHandleW, 0)
	if r == 0 {
		return 0, lastError("GetModuleHandleW", err)
	}
	return windows.Handle(r), nil
}

// SetLastError sets the calling thread's last-error code.
func SetLastError(code uint32) {
	callV(procSetLastError, uintptr(code))
}
