//go:build windows

package d3d11

import (
	"strconv"
	"unsafe"

	"github.com/radix29/gogui/internal/shell/win32/com"
	"golang.org/x/sys/windows"
)

var d3d11dll = windows.NewLazySystemDLL("d3d11.dll")

// allProcs lists every proc declared with newProc, for TestProcsResolve.
var allProcs []*windows.LazyProc

func newProc(dll *windows.LazyDLL, name string) *windows.LazyProc {
	p := dll.NewProc(name)
	allProcs = append(allProcs, p)
	return p
}

var procD3D11CreateDevice = newProc(d3d11dll, "D3D11CreateDevice")

// Interface IDs.
var (
	IID_ID3D11Device           = com.MustGUID("{DB6F6DDB-AC77-4E88-8253-819DF9BBF140}")
	IID_ID3D11DeviceContext    = com.MustGUID("{C0BFA96C-E089-44FB-8EAF-26F8796190DA}")
	IID_ID3D11Resource         = com.MustGUID("{DC8E63F3-D12B-4952-B47B-5E45026A862D}")
	IID_ID3D11Texture2D        = com.MustGUID("{6F15AAF2-D208-4E89-9AB4-489535D34F9C}")
	IID_ID3D11RenderTargetView = com.MustGUID("{DFDBA067-0B8D-4865-875B-D7B4516CC164}")
)

// Vtable slots in d3d11.h order; slots_test.go checks each one.
const (
	// ID3D11Device.
	slotDeviceCreateTexture2D        = 5
	slotDeviceCreateRenderTargetView = 9
	slotDeviceGetFeatureLevel        = 37
	slotDeviceGetCreationFlags       = 38
	slotDeviceGetDeviceRemovedReason = 39
	slotDeviceGetImmediateContext    = 40

	// ID3D11DeviceContext (after ID3D11DeviceChild 3–6).
	slotContextMap                   = 14
	slotContextUnmap                 = 15
	slotContextOMSetRenderTargets    = 33
	slotContextCopyResource          = 47
	slotContextClearRenderTargetView = 50
	slotContextClearState            = 110
	slotContextFlush                 = 111

	// ID3D11Texture2D (after ID3D11DeviceChild 3–6 and ID3D11Resource 7–9).
	slotTexture2DGetDesc = 10
)

// DriverType is D3D_DRIVER_TYPE.
type DriverType uint32

// Driver types.
const (
	DRIVER_TYPE_UNKNOWN   DriverType = 0
	DRIVER_TYPE_HARDWARE  DriverType = 1
	DRIVER_TYPE_REFERENCE DriverType = 2
	DRIVER_TYPE_NULL      DriverType = 3
	DRIVER_TYPE_SOFTWARE  DriverType = 4
	DRIVER_TYPE_WARP      DriverType = 5
)

func (d DriverType) String() string {
	switch d {
	case DRIVER_TYPE_HARDWARE:
		return "hardware"
	case DRIVER_TYPE_WARP:
		return "WARP"
	}
	return "driver type " + strconv.Itoa(int(d))
}

// FeatureLevel is D3D_FEATURE_LEVEL.
type FeatureLevel uint32

// Feature levels.
const (
	FEATURE_LEVEL_9_1  FeatureLevel = 0x9100
	FEATURE_LEVEL_9_2  FeatureLevel = 0x9200
	FEATURE_LEVEL_9_3  FeatureLevel = 0x9300
	FEATURE_LEVEL_10_0 FeatureLevel = 0xa000
	FEATURE_LEVEL_10_1 FeatureLevel = 0xa100
	FEATURE_LEVEL_11_0 FeatureLevel = 0xb000
	FEATURE_LEVEL_11_1 FeatureLevel = 0xb100
)

// D3D11_CREATE_DEVICE_FLAG.
const (
	CREATE_DEVICE_SINGLETHREADED = 0x1
	CREATE_DEVICE_DEBUG          = 0x2
	CREATE_DEVICE_BGRA_SUPPORT   = 0x20
)

// SDK_VERSION is D3D11_SDK_VERSION.
const SDK_VERSION = 7

// featureLevels is what CreateDevice asks for: every level D2D can draw
// on, 11_1 first (the default list without one starts at 11_0).
var featureLevels = [...]FeatureLevel{
	FEATURE_LEVEL_11_1, FEATURE_LEVEL_11_0, FEATURE_LEVEL_10_1, FEATURE_LEVEL_10_0,
	FEATURE_LEVEL_9_3, FEATURE_LEVEL_9_2, FEATURE_LEVEL_9_1,
}

// CreateDeviceOfType creates a device of one driver type with flags and
// returns it with its immediate context (each one reference) and feature
// level.
func CreateDeviceOfType(dt DriverType, flags uint32) (Device, DeviceContext, FeatureLevel, error) {
	var dev, ctx com.Ptr
	var level FeatureLevel
	r, _, _ := procD3D11CreateDevice.Call(0, uintptr(dt), 0, uintptr(flags),
		uintptr(unsafe.Pointer(&featureLevels[0])), uintptr(len(featureLevels)), SDK_VERSION,
		uintptr(unsafe.Pointer(&dev)), uintptr(unsafe.Pointer(&level)), uintptr(unsafe.Pointer(&ctx)))
	if err := com.HRESULT(int32(r)).Err("D3D11CreateDevice(" + dt.String() + ")"); err != nil {
		return Device{}, DeviceContext{}, 0, err
	}
	return Device{dev}, DeviceContext{ctx}, level, nil
}

// CreateDevice creates the shell's device: hardware with
// CREATE_DEVICE_BGRA_SUPPORT (which D2D needs), and WARP if no hardware
// device can be made (no GPU driver, a remote session without one). It
// returns the driver type it used; with both failing, the error of the
// hardware attempt is kept in the result.
func CreateDevice() (Device, DeviceContext, DriverType, error) {
	dev, ctx, _, herr := CreateDeviceOfType(DRIVER_TYPE_HARDWARE, CREATE_DEVICE_BGRA_SUPPORT)
	if herr == nil {
		return dev, ctx, DRIVER_TYPE_HARDWARE, nil
	}
	dev, ctx, _, werr := CreateDeviceOfType(DRIVER_TYPE_WARP, CREATE_DEVICE_BGRA_SUPPORT)
	if werr != nil {
		return Device{}, DeviceContext{}, 0, &CreateError{Hardware: herr, WARP: werr}
	}
	return dev, ctx, DRIVER_TYPE_WARP, nil
}

// CreateError is the failure of both CreateDevice attempts.
type CreateError struct {
	Hardware, WARP error
}

func (e *CreateError) Error() string {
	return "d3d11: no device: " + e.Hardware.Error() + "; " + e.WARP.Error()
}

// Unwrap returns both errors, so errors.Is finds either HRESULT.
func (e *CreateError) Unwrap() []error { return []error{e.Hardware, e.WARP} }

// Device is ID3D11Device.
type Device struct{ com.Ptr }

// CreateTexture2D creates a texture without initial data.
func (d Device) CreateTexture2D(desc *Texture2DDesc) (Texture2D, error) {
	var out com.Ptr
	err := d.CallHR("ID3D11Device::CreateTexture2D", slotDeviceCreateTexture2D,
		uintptr(unsafe.Pointer(desc)), 0, uintptr(unsafe.Pointer(&out)))
	return Texture2D{out}, err
}

// CreateRenderTargetView creates a view of the whole resource in its own
// format.
func (d Device) CreateRenderTargetView(res com.Ptr) (RenderTargetView, error) {
	var out com.Ptr
	err := d.CallHR("ID3D11Device::CreateRenderTargetView", slotDeviceCreateRenderTargetView,
		uintptr(res), 0, uintptr(unsafe.Pointer(&out)))
	return RenderTargetView{out}, err
}

// FeatureLevel returns the device's feature level.
func (d Device) FeatureLevel() FeatureLevel {
	return FeatureLevel(uint32(d.Call(slotDeviceGetFeatureLevel)))
}

// CreationFlags returns the flags the device was created with.
func (d Device) CreationFlags() uint32 {
	return uint32(d.Call(slotDeviceGetCreationFlags))
}

// RemovedReason returns nil while the device works, else the reason it
// was removed (DXGI_ERROR_DEVICE_REMOVED, _HUNG, _RESET, …).
func (d Device) RemovedReason() error {
	return d.CallHR("ID3D11Device::GetDeviceRemovedReason", slotDeviceGetDeviceRemovedReason)
}

// ImmediateContext returns the device's immediate context with a
// reference of its own.
func (d Device) ImmediateContext() DeviceContext {
	var out com.Ptr
	d.Call(slotDeviceGetImmediateContext, uintptr(unsafe.Pointer(&out)))
	return DeviceContext{out}
}

// DeviceContext is ID3D11DeviceContext.
type DeviceContext struct{ com.Ptr }

// Map maps subresource sub of res for the CPU. The mapping is valid until
// Unmap; copy the data out with ffi.CopyBytes before that.
func (c DeviceContext) Map(res com.Ptr, sub uint32, mapType uint32, flags uint32) (MappedSubresource, error) {
	var m MappedSubresource
	err := c.CallHR("ID3D11DeviceContext::Map", slotContextMap,
		uintptr(res), uintptr(sub), uintptr(mapType), uintptr(flags), uintptr(unsafe.Pointer(&m)))
	return m, err
}

// Unmap ends a Map.
func (c DeviceContext) Unmap(res com.Ptr, sub uint32) {
	c.Call(slotContextUnmap, uintptr(res), uintptr(sub))
}

// SetRenderTarget binds rtv as the only render target, with no depth
// buffer; a nil rtv unbinds.
func (c DeviceContext) SetRenderTarget(rtv RenderTargetView) {
	if rtv.IsNil() {
		c.Call(slotContextOMSetRenderTargets, 0, 0, 0)
		return
	}
	p := rtv.Ptr
	c.Call(slotContextOMSetRenderTargets, 1, uintptr(unsafe.Pointer(&p)), 0)
}

// CopyResource copies all of src into dst (same size and format).
func (c DeviceContext) CopyResource(dst, src com.Ptr) {
	c.Call(slotContextCopyResource, uintptr(dst), uintptr(src))
}

// ClearRenderTargetView sets every pixel of rtv to rgba (in the view's
// format; for an UNORM format 0…1 per channel).
func (c DeviceContext) ClearRenderTargetView(rtv RenderTargetView, rgba [4]float32) {
	c.Call(slotContextClearRenderTargetView, uintptr(rtv.Ptr), uintptr(unsafe.Pointer(&rgba)))
}

// ClearState unbinds everything from the pipeline.
func (c DeviceContext) ClearState() {
	c.Call(slotContextClearState)
}

// Flush sends the queued commands to the GPU. It also runs D3D11's
// deferred destruction of released objects (swap chains among them).
func (c DeviceContext) Flush() {
	c.Call(slotContextFlush)
}

// Texture2D is ID3D11Texture2D.
type Texture2D struct{ com.Ptr }

// Desc returns the texture's description.
func (t Texture2D) Desc() Texture2DDesc {
	var d Texture2DDesc
	t.Call(slotTexture2DGetDesc, uintptr(unsafe.Pointer(&d)))
	return d
}

// RenderTargetView is ID3D11RenderTargetView.
type RenderTargetView struct{ com.Ptr }
