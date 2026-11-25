//go:build windows

package w32

import "golang.org/x/sys/windows"

// ---- the clipboard (S5.2a) ----

// OpenClipboard opens the clipboard for the calling thread, with owner as
// the window that owns what EmptyClipboard and SetClipboardData put there.
// It fails (ERROR_ACCESS_DENIED) while another window has it open.
func OpenClipboard(owner windows.HWND) error {
	r, _, err := callV(procOpenClipboard, uintptr(owner))
	if r == 0 {
		return lastError("OpenClipboard", err)
	}
	return nil
}

// CloseClipboard closes the clipboard OpenClipboard opened.
func CloseClipboard() error {
	r, _, err := callV(procCloseClipboard)
	if r == 0 {
		return lastError("CloseClipboard", err)
	}
	return nil
}

// EmptyClipboard empties the open clipboard and makes the owner passed to
// OpenClipboard its owner.
func EmptyClipboard() error {
	r, _, err := callV(procEmptyClipboard)
	if r == 0 {
		return lastError("EmptyClipboard", err)
	}
	return nil
}

// SetClipboardData puts mem, an unlocked GMEM_MOVEABLE block, on the open
// clipboard as format. On success the system owns mem (do not free it); on
// failure the caller still does.
func SetClipboardData(format uint32, mem windows.Handle) error {
	r, _, err := callV(procSetClipboardData, uintptr(format), uintptr(mem))
	if r == 0 {
		return lastError("SetClipboardData", err)
	}
	return nil
}

// GetClipboardData returns the open clipboard's block of format, or 0 when
// the clipboard has none. The block belongs to the clipboard: lock it to
// read it, and do not free it or use it after CloseClipboard.
func GetClipboardData(format uint32) windows.Handle {
	r, _, _ := callV(procGetClipboardData, uintptr(format))
	return windows.Handle(r)
}

// GlobalAlloc allocates n bytes of global memory with flags (GMEM_*).
func GlobalAlloc(flags uint32, n uintptr) (windows.Handle, error) {
	r, _, err := callV(procGlobalAlloc, uintptr(flags), n)
	if r == 0 {
		return 0, lastError("GlobalAlloc", err)
	}
	return windows.Handle(r), nil
}

// GlobalFree frees a block of GlobalAlloc.
func GlobalFree(mem windows.Handle) error {
	r, _, err := callV(procGlobalFree, uintptr(mem))
	if r != 0 {
		return lastError("GlobalFree", err)
	}
	return nil
}

// GlobalLock locks a global block and returns the address of its first
// byte, native memory valid until GlobalUnlock.
func GlobalLock(mem windows.Handle) (uintptr, error) {
	r, _, err := callV(procGlobalLock, uintptr(mem))
	if r == 0 {
		return 0, lastError("GlobalLock", err)
	}
	return r, nil
}

// GlobalUnlock undoes one GlobalLock. Its BOOL result only tells whether
// the block is still locked, so there is nothing to report.
func GlobalUnlock(mem windows.Handle) {
	callV(procGlobalUnlock, uintptr(mem))
}

// GlobalSize returns the size of a global block in bytes (at least what
// was asked of GlobalAlloc), or 0 for an invalid or discarded block.
func GlobalSize(mem windows.Handle) uintptr {
	r, _, _ := callV(procGlobalSize, uintptr(mem))
	return r
}
