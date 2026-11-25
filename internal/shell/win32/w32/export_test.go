//go:build windows

package w32

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Test-only bindings (review §4): nothing outside this package's tests
// calls them.

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
