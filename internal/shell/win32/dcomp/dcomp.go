//go:build windows

package dcomp

import (
	"unsafe"

	"github.com/radix29/gogui/internal/shell/win32/com"
	"golang.org/x/sys/windows"
)

var dcompdll = windows.NewLazySystemDLL("dcomp.dll")

// allProcs lists every proc declared with newProc, for TestProcsResolve.
var allProcs []*windows.LazyProc

func newProc(dll *windows.LazyDLL, name string) *windows.LazyProc {
	p := dll.NewProc(name)
	allProcs = append(allProcs, p)
	return p
}

var procDCompositionCreateDevice = newProc(dcompdll, "DCompositionCreateDevice")

// Interface IDs.
var (
	IID_IDCompositionDevice = com.MustGUID("{C37EA93A-E7AA-450D-B16F-9746CB0407F3}")
	IID_IDCompositionTarget = com.MustGUID("{EACDD04C-117E-4E17-88F4-D1B12B0E3D89}")
	IID_IDCompositionVisual = com.MustGUID("{4D93059D-097B-4651-9A60-F0F25116E2F3}")
)

// Vtable slots in dcomp.h order (MSVC layout); slots_test.go checks each one.
const (
	slotDeviceCommit              = 3
	slotDeviceCreateTargetForHwnd = 6
	slotDeviceCreateVisual        = 7

	slotTargetSetRoot = 3

	slotVisualSetContent = 15
)

// CreateDevice creates a composition device over dxgiDevice (the
// IDXGIDevice of the shell's D3D11 device).
func CreateDevice(dxgiDevice com.Ptr) (Device, error) {
	var out com.Ptr
	r, _, _ := procDCompositionCreateDevice.Call(uintptr(dxgiDevice),
		uintptr(unsafe.Pointer(&IID_IDCompositionDevice)), uintptr(unsafe.Pointer(&out)))
	if err := com.HRESULT(int32(r)).Err("DCompositionCreateDevice"); err != nil {
		return Device{}, err
	}
	return Device{out}, nil
}

// Device is IDCompositionDevice.
type Device struct{ com.Ptr }

// Commit applies every change since the last Commit.
func (d Device) Commit() error {
	return d.CallHR("IDCompositionDevice::Commit", slotDeviceCommit)
}

// CreateTargetForHwnd creates the composition target of h. topmost puts
// the tree above the window's GDI content; an HWND has at most one target
// of each kind (a second fails with DCOMPOSITION_ERROR_WINDOW_ALREADY_COMPOSED).
func (d Device) CreateTargetForHwnd(h windows.HWND, topmost bool) (Target, error) {
	var out com.Ptr
	var top uintptr
	if topmost {
		top = 1
	}
	err := d.CallHR("IDCompositionDevice::CreateTargetForHwnd", slotDeviceCreateTargetForHwnd,
		uintptr(h), top, uintptr(unsafe.Pointer(&out)))
	return Target{out}, err
}

// CreateVisual creates a visual.
func (d Device) CreateVisual() (Visual, error) {
	var out com.Ptr
	err := d.CallHR("IDCompositionDevice::CreateVisual", slotDeviceCreateVisual, uintptr(unsafe.Pointer(&out)))
	return Visual{out}, err
}

// Target is IDCompositionTarget.
type Target struct{ com.Ptr }

// SetRoot makes v the root of the target's tree (a nil v clears it). The
// target takes a reference of its own.
func (t Target) SetRoot(v Visual) error {
	return t.CallHR("IDCompositionTarget::SetRoot", slotTargetSetRoot, uintptr(v.Ptr))
}

// Visual is IDCompositionVisual.
type Visual struct{ com.Ptr }

// SetContent makes content (a composition swap chain, or 0 to clear) the
// visual's content. The visual takes a reference of its own.
func (v Visual) SetContent(content com.Ptr) error {
	return v.CallHR("IDCompositionVisual::SetContent", slotVisualSetContent, uintptr(content))
}

// DCOMPOSITION_ERROR_WINDOW_ALREADY_COMPOSED is the failure of a second
// target of the same kind on one HWND.
const DCOMPOSITION_ERROR_WINDOW_ALREADY_COMPOSED com.HRESULT = -0x7767f800 // 0x88980800
