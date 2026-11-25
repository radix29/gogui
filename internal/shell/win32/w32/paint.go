//go:build windows

package w32

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

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
	r, _, _ := callV(procUpdateWindow, uintptr(h))
	if r == 0 {
		return failed("UpdateWindow")
	}
	return nil
}

// IsWindowVisible reports whether h has WS_VISIBLE (S2.3c). A minimized
// window is visible.
func IsWindowVisible(h windows.HWND) bool {
	r, _, _ := callV(procIsWindowVisible, uintptr(h))
	return r != 0
}

// CreateSolidBrush creates a brush of colorref (0x00BBGGRR). Free it with
// DeleteObject.
func CreateSolidBrush(colorref uint32) (windows.Handle, error) {
	r, _, _ := callV(procCreateSolidBrush, uintptr(colorref))
	if r == 0 {
		return 0, failed("CreateSolidBrush")
	}
	return windows.Handle(r), nil
}

// DeleteObject frees a GDI object.
func DeleteObject(obj windows.Handle) error {
	r, _, _ := callV(procDeleteObject, uintptr(obj))
	if r == 0 {
		return failed("DeleteObject")
	}
	return nil
}

// ---- window capture (S2.4d_3) ----

// CreateCompatibleDC creates a memory DC compatible with dc (0: the
// screen). Free it with DeleteDC.
func CreateCompatibleDC(dc windows.Handle) (windows.Handle, error) {
	r, _, _ := callV(procCreateCompatibleDC, uintptr(dc))
	if r == 0 {
		return 0, failed("CreateCompatibleDC")
	}
	return windows.Handle(r), nil
}

// DeleteDC frees a DC from CreateCompatibleDC.
func DeleteDC(dc windows.Handle) error {
	r, _, _ := callV(procDeleteDC, uintptr(dc))
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
	r, _, _ := callV(procSelectObject, uintptr(dc), uintptr(obj))
	return windows.Handle(r)
}

// GdiFlush runs the calling thread's pending GDI drawing.
func GdiFlush() {
	callV(procGdiFlush)
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
	r, _, _ := callV(procPrintWindow, uintptr(h), uintptr(dc), uintptr(flags))
	if r == 0 {
		return failed("PrintWindow")
	}
	return nil
}
