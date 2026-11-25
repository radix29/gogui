//go:build windows

package w32

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

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
