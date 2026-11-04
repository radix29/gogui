//go:build windows

package w32

import "golang.org/x/sys/windows"

// The mirrored structs. Each one follows its declaration in the Windows
// SDK headers (WinUser.h, WinDef.h, Uxtheme.h) field by field; the layouts
// are pinned by types_test.go. Native pointers are uintptr, handles
// windows.HWND or windows.Handle. A cbSize (or length) field is set by the
// wrapper that passes the struct to Windows.

// Point is POINT.
type Point struct {
	X, Y int32
}

// Size is SIZE, the lParam of WM_GETDPISCALEDSIZE (S2.3d).
type Size struct {
	Cx, Cy int32
}

// Rect is RECT.
type Rect struct {
	Left, Top, Right, Bottom int32
}

// Msg is MSG.
type Msg struct {
	Hwnd     windows.HWND
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       Point
	LPrivate uint32
}

// WndClassEx is WNDCLASSEXW. RegisterClassEx sets CbSize and
// LpszClassName.
type WndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr // a callback from ffi.RegisterCallback
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     windows.Handle
	HIcon         windows.Handle
	HCursor       windows.Handle
	HbrBackground windows.Handle
	LpszMenuName  uintptr // LPCWSTR
	LpszClassName uintptr // LPCWSTR
	HIconSm       windows.Handle
}

// MinMaxInfo is MINMAXINFO, the lParam of WM_GETMINMAXINFO.
type MinMaxInfo struct {
	PtReserved     Point
	PtMaxSize      Point
	PtMaxPosition  Point
	PtMinTrackSize Point
	PtMaxTrackSize Point
}

// WindowPos is WINDOWPOS, the lParam of WM_WINDOWPOSCHANGING/CHANGED and
// the target of NCCalcSizeParams.Lppos.
type WindowPos struct {
	Hwnd            windows.HWND
	HwndInsertAfter windows.HWND
	X, Y            int32
	Cx, Cy          int32
	Flags           uint32
}

// NCCalcSizeParams is NCCALCSIZE_PARAMS, the lParam of WM_NCCALCSIZE when
// wParam is TRUE.
type NCCalcSizeParams struct {
	Rgrc  [3]Rect
	Lppos uintptr // PWINDOWPOS
}

// TrackMouseEventInfo is TRACKMOUSEEVENT. TrackMouseEvent sets CbSize.
type TrackMouseEventInfo struct {
	CbSize      uint32
	DwFlags     uint32
	HwndTrack   windows.HWND
	DwHoverTime uint32
}

// MonitorInfo is MONITORINFO. GetMonitorInfo sets CbSize.
type MonitorInfo struct {
	CbSize    uint32
	RcMonitor Rect
	RcWork    Rect
	DwFlags   uint32
}

// Margins is MARGINS (Uxtheme.h), for DwmExtendFrameIntoClientArea.
type Margins struct {
	CxLeftWidth    int32
	CxRightWidth   int32
	CyTopHeight    int32
	CyBottomHeight int32
}

// CreateStruct is CREATESTRUCTW, the lParam of WM_NCCREATE and WM_CREATE.
// LpCreateParams is CreateWindowEx's lpParam: the window's ffi.Handle,
// never a Go pointer.
type CreateStruct struct {
	LpCreateParams uintptr
	HInstance      windows.Handle
	HMenu          windows.Handle
	HwndParent     windows.HWND
	Cy, Cx         int32
	Y, X           int32
	Style          int32
	LpszName       uintptr // LPCWSTR
	LpszClass      uintptr // LPCWSTR
	DwExStyle      uint32
}

// PaintStruct is PAINTSTRUCT, for BeginPaint/EndPaint.
type PaintStruct struct {
	Hdc         windows.Handle
	FErase      int32
	RcPaint     Rect
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}

// HighContrast is HIGHCONTRASTW, for SPI_GETHIGHCONTRAST.
// SystemParametersInfoHighContrast sets CbSize.
type HighContrast struct {
	CbSize            uint32
	DwFlags           uint32
	LpszDefaultScheme uintptr // LPWSTR, owned by Windows
}

// BitmapInfoHeader is BITMAPINFOHEADER (wingdi.h). For a 32-bit BI_RGB
// bitmap it is all of BITMAPINFO that CreateDIBSection reads (no color
// table). A negative BiHeight makes the bitmap top-down.
type BitmapInfoHeader struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

// WindowPlacement is WINDOWPLACEMENT. GetWindowPlacement and
// SetWindowPlacement set Length.
type WindowPlacement struct {
	Length           uint32
	Flags            uint32
	ShowCmd          uint32
	PtMinPosition    Point
	PtMaxPosition    Point
	RcNormalPosition Rect
}
