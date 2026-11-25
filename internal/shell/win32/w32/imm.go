//go:build windows

package w32

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ---- IMM32 (S5.3e) ----
//
// An HIMC is a windows.Handle. ImmGetContext hands out the context
// associated with a window, to be given back with ImmReleaseContext in the
// same message; the shell never keeps one.

// The negative results of ImmGetCompositionStringW, wrapped by the errors
// of the composition readers.
var (
	ErrIMMNoData  = errors.New("w32: IMM_ERROR_NODATA")
	ErrIMMGeneral = errors.New("w32: IMM_ERROR_GENERAL")
)

// ImmGetContext returns the input context associated with h, or 0 when h
// has none (ImmAssociateContextEx with a 0 context, or IME disabled for
// the thread). A non-zero context must be released with ImmReleaseContext.
func ImmGetContext(h windows.HWND) windows.Handle {
	r, _, _ := callV(procImmGetContext, uintptr(h))
	return windows.Handle(r)
}

// ImmReleaseContext releases a context of ImmGetContext.
func ImmReleaseContext(h windows.HWND, himc windows.Handle) error {
	r, _, err := callV(procImmReleaseContext, uintptr(h), uintptr(himc))
	if r == 0 {
		return lastError("ImmReleaseContext", err)
	}
	return nil
}

// immCompositionString is ImmGetCompositionStringW: with n 0 it returns
// the size of the part index in bytes, otherwise the bytes copied to buf
// (at most n). buf is a Go buffer passed for the call only.
func immCompositionString(himc windows.Handle, index uint32, buf unsafe.Pointer, n int) (int, error) {
	r, _, _ := procImmGetCompositionStringW.Call(uintptr(himc), uintptr(index), uintptr(buf), uintptr(n))
	switch v := int32(r); {
	case v == IMM_ERROR_NODATA:
		return 0, fmt.Errorf("w32: ImmGetCompositionStringW(%#x): %w", index, ErrIMMNoData)
	case v < 0:
		return 0, fmt.Errorf("w32: ImmGetCompositionStringW(%#x) = %d: %w", index, v, ErrIMMGeneral)
	default:
		return int(v), nil
	}
}

// immCompositionBytes reads the part index of the composition into a new
// byte slice (nil when the part is empty).
func immCompositionBytes(himc windows.Handle, index uint32) ([]byte, error) {
	n, err := immCompositionString(himc, index, nil, 0)
	if err != nil || n == 0 {
		return nil, err
	}
	buf := make([]byte, n)
	m, err := immCompositionString(himc, index, unsafe.Pointer(&buf[0]), n)
	if err != nil {
		return nil, err
	}
	return buf[:min(m, n)], nil
}

// ImmGetCompositionString reads a string part of the composition
// (GCS_COMPSTR, GCS_RESULTSTR, GCS_COMPREADSTR, GCS_RESULTREADSTR) as
// UTF-16 units, without a terminating NUL. An empty part is nil.
func ImmGetCompositionString(himc windows.Handle, index uint32) ([]uint16, error) {
	b, err := immCompositionBytes(himc, index)
	if err != nil || len(b) < 2 {
		return nil, err
	}
	s := make([]uint16, len(b)/2)
	for i := range s {
		s[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return s, nil
}

// ImmGetCompositionAttr reads an attribute part of the composition
// (GCS_COMPATTR, GCS_COMPREADATTR): one ATTR_* byte per UTF-16 unit of
// the matching string. An empty part is nil.
func ImmGetCompositionAttr(himc windows.Handle, index uint32) ([]byte, error) {
	return immCompositionBytes(himc, index)
}

// ImmGetCompositionCursorPos returns the cursor of the composition string
// (GCS_CURSORPOS) in UTF-16 units.
func ImmGetCompositionCursorPos(himc windows.Handle) (int, error) {
	return immCompositionString(himc, GCS_CURSORPOS, nil, 0)
}

// ImmSetCompositionWindow places the composition window of himc (which the
// shell hides; the IME still reads the position for its own windows).
func ImmSetCompositionWindow(himc windows.Handle, cf *CompositionForm) error {
	r, _, err := procImmSetCompositionWindow.Call(uintptr(himc), uintptr(unsafe.Pointer(cf)))
	if r == 0 {
		return lastError("ImmSetCompositionWindow", err)
	}
	return nil
}

// ImmSetCandidateWindow places the candidate window cf.DwIndex of himc.
func ImmSetCandidateWindow(himc windows.Handle, cf *CandidateForm) error {
	r, _, err := procImmSetCandidateWindow.Call(uintptr(himc), uintptr(unsafe.Pointer(cf)))
	if r == 0 {
		return lastError("ImmSetCandidateWindow", err)
	}
	return nil
}

// ImmAssociateContextEx associates himc with h by flags (IACE_*):
// IACE_DEFAULT restores the default context (himc ignored), flags 0 with
// himc 0 leaves h without a context, so the IME is off for it.
func ImmAssociateContextEx(h windows.HWND, himc windows.Handle, flags uint32) error {
	r, _, err := callV(procImmAssociateContextEx, uintptr(h), uintptr(himc), uintptr(flags))
	if r == 0 {
		return lastError("ImmAssociateContextEx", err)
	}
	return nil
}

// ImmNotifyIME tells the IME of himc to act (NI_*) with index and value;
// NI_COMPOSITIONSTR with CPS_CANCEL drops the composition. It fails when
// no IME is open on the context (nothing to act on).
func ImmNotifyIME(himc windows.Handle, action, index, value uint32) error {
	r, _, err := callV(procImmNotifyIME, uintptr(himc), uintptr(action), uintptr(index), uintptr(value))
	if r == 0 {
		return lastError("ImmNotifyIME", err)
	}
	return nil
}
