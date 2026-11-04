//go:build windows

package d2d

import (
	"unsafe"

	"github.com/radix29/gogui/internal/shell/win32/com"
	"github.com/radix29/gogui/internal/shell/win32/dxgi"
	"golang.org/x/sys/windows"
)

var d2d1dll = windows.NewLazySystemDLL("d2d1.dll")

// allProcs lists every proc declared with newProc, for TestProcsResolve.
var allProcs []*windows.LazyProc

func newProc(dll *windows.LazyDLL, name string) *windows.LazyProc {
	p := dll.NewProc(name)
	allProcs = append(allProcs, p)
	return p
}

var procD2D1CreateFactory = newProc(d2d1dll, "D2D1CreateFactory")

// Interface IDs.
var (
	IID_ID2D1Factory       = com.MustGUID("{06152247-6F50-465A-9245-118BFD3B6007}")
	IID_ID2D1Factory1      = com.MustGUID("{BB12D362-DAEE-4B9A-AA1D-14BA401CFA1F}")
	IID_ID2D1Resource      = com.MustGUID("{2CD90691-12E2-11DC-9FED-001143A055F9}")
	IID_ID2D1RenderTarget  = com.MustGUID("{2CD90694-12E2-11DC-9FED-001143A055F9}")
	IID_ID2D1Device        = com.MustGUID("{47DD575D-AC05-4CDD-8049-9B02CD16F44C}")
	IID_ID2D1DeviceContext = com.MustGUID("{E8F7FE7A-191C-466D-AD95-975678BDA998}")
	IID_ID2D1Image         = com.MustGUID("{65019F75-8DA2-497C-B32C-DFA34E48EDE6}")
	IID_ID2D1Bitmap        = com.MustGUID("{A2296057-EA42-4099-983B-539FB6505426}")
	IID_ID2D1Bitmap1       = com.MustGUID("{A898A84C-3873-4588-B08B-EBBF978DF041}")
)

// Vtable slots in d2d1.h / d2d1_1.h order; the tests check each one.
const (
	// ID2D1Factory1 (after ID2D1Factory 3–16).
	slotFactoryCreateDevice = 17

	// ID2D1Device (after ID2D1Resource::GetFactory, 3).
	slotDeviceCreateDeviceContext = 4

	// ID2D1RenderTarget (after ID2D1Resource, 3).
	slotRTClear     = 47
	slotRTBeginDraw = 48
	slotRTEndDraw   = 49
	slotRTSetDpi    = 51 // FLOAT, FLOAT: the float path
	slotRTGetDpi    = 52

	// ID2D1DeviceContext (after ID2D1RenderTarget 0–56).
	slotDCCreateBitmapFromDxgiSurface = 62
	slotDCSetTarget                   = 74
	slotDCGetTarget                   = 75

	// ID2D1Bitmap (after ID2D1Image, which adds nothing to ID2D1Resource).
	slotBitmapGetDpi = 7
	// ID2D1Bitmap1.
	slotBitmap1GetOptions = 12
	slotBitmap1GetSurface = 13
)

// CreateFactory creates a single-threaded ID2D1Factory1 (every D2D object
// lives on the UI thread). debugLevel is a DEBUG_LEVEL_*; anything but
// DEBUG_LEVEL_NONE needs the D2D debug layer (Graphics Tools) installed,
// or the call fails.
func CreateFactory(debugLevel uint32) (Factory1, error) {
	opts := FactoryOptions{DebugLevel: debugLevel}
	var out com.Ptr
	r, _, _ := procD2D1CreateFactory.Call(FACTORY_TYPE_SINGLE_THREADED,
		uintptr(unsafe.Pointer(&IID_ID2D1Factory1)), uintptr(unsafe.Pointer(&opts)),
		uintptr(unsafe.Pointer(&out)))
	if err := com.HRESULT(int32(r)).Err("D2D1CreateFactory"); err != nil {
		return Factory1{}, err
	}
	return Factory1{out}, nil
}

// Factory1 is ID2D1Factory1.
type Factory1 struct{ com.Ptr }

// CreateDevice creates a D2D device over dxgiDevice (the IDXGIDevice of a
// D3D11 device created with CREATE_DEVICE_BGRA_SUPPORT). The device holds
// a reference to the factory.
func (f Factory1) CreateDevice(dxgiDevice dxgi.Device) (Device, error) {
	var out com.Ptr
	err := f.CallHR("ID2D1Factory1::CreateDevice", slotFactoryCreateDevice,
		uintptr(dxgiDevice.Ptr), uintptr(unsafe.Pointer(&out)))
	return Device{out}, err
}

// Device is ID2D1Device.
type Device struct{ com.Ptr }

// CreateDeviceContext creates a device context with options
// (DEVICE_CONTEXT_OPTIONS_*). It starts with no target and 96 DPI.
func (d Device) CreateDeviceContext(options uint32) (DeviceContext, error) {
	var out com.Ptr
	err := d.CallHR("ID2D1Device::CreateDeviceContext", slotDeviceCreateDeviceContext,
		uintptr(options), uintptr(unsafe.Pointer(&out)))
	return DeviceContext{out}, err
}

// DeviceContext is ID2D1DeviceContext; the bound ID2D1RenderTarget methods
// are on it too.
type DeviceContext struct{ com.Ptr }

// CreateBitmapFromDxgiSurface creates a bitmap over surface (a swap
// chain's back buffer as IDXGISurface). For a back buffer, props are
// BITMAP_OPTIONS_TARGET | BITMAP_OPTIONS_CANNOT_DRAW, the chain's format
// (B8G8R8A8_UNORM), ALPHA_MODE_PREMULTIPLIED and the surface's DPI. The
// bitmap holds a reference to the surface, so it must be released (and
// cleared from SetTarget) before IDXGISwapChain::ResizeBuffers.
func (c DeviceContext) CreateBitmapFromDxgiSurface(surface com.Ptr, props *BitmapProperties1) (Bitmap1, error) {
	var out com.Ptr
	err := c.CallHR("ID2D1DeviceContext::CreateBitmapFromDxgiSurface", slotDCCreateBitmapFromDxgiSurface,
		uintptr(surface), uintptr(unsafe.Pointer(props)), uintptr(unsafe.Pointer(&out)))
	return Bitmap1{out}, err
}

// SetTarget makes image (a bitmap created with BITMAP_OPTIONS_TARGET, or 0
// to clear) the context's target. The context takes a reference of its
// own.
func (c DeviceContext) SetTarget(image com.Ptr) {
	c.Call(slotDCSetTarget, uintptr(image))
}

// Target returns the current target with a reference of its own, or a nil
// Ptr.
func (c DeviceContext) Target() com.Ptr {
	var out com.Ptr
	c.Call(slotDCGetTarget, uintptr(unsafe.Pointer(&out)))
	return out
}

// BeginDraw starts drawing on the target.
func (c DeviceContext) BeginDraw() {
	c.Call(slotRTBeginDraw)
}

// EndDraw ends drawing and returns the first error of the batch:
// D2DERR_RECREATE_TARGET (com) means the device is lost and every
// device-bound resource must be recreated.
func (c DeviceContext) EndDraw() error {
	return c.CallHR("ID2D1RenderTarget::EndDraw", slotRTEndDraw, 0, 0)
}

// Clear sets every pixel of the target (inside the current clip) to color.
// On a premultiplied target the color is stored premultiplied.
func (c DeviceContext) Clear(color ColorF) {
	c.Call(slotRTClear, uintptr(unsafe.Pointer(&color)))
}

var methodSetDpi = com.Method[func(this com.Ptr, x, y float32)]{Slot: slotRTSetDpi}

// SetDpi sets the context's DPI, the scale from DIPs to pixels (96 is 1:1).
// It is the first call on the float path (two FLOATs by value).
func (c DeviceContext) SetDpi(x, y float32) {
	methodSetDpi.Fn(c.Ptr)(c.Ptr, x, y)
}

// Dpi returns the context's DPI.
func (c DeviceContext) Dpi() (x, y float32) {
	c.Call(slotRTGetDpi, uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y)))
	return x, y
}

// Bitmap1 is ID2D1Bitmap1.
type Bitmap1 struct{ com.Ptr }

// Options returns the bitmap's BITMAP_OPTIONS_*.
func (b Bitmap1) Options() BitmapOptions {
	return BitmapOptions(uint32(b.Call(slotBitmap1GetOptions)))
}

// Dpi returns the bitmap's DPI.
func (b Bitmap1) Dpi() (x, y float32) {
	b.Call(slotBitmapGetDpi, uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y)))
	return x, y
}

// Surface returns the DXGI surface under the bitmap, with a reference of
// its own.
func (b Bitmap1) Surface() (com.Ptr, error) {
	var out com.Ptr
	err := b.CallHR("ID2D1Bitmap1::GetSurface", slotBitmap1GetSurface, uintptr(unsafe.Pointer(&out)))
	return out, err
}
