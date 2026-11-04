//go:build windows

package dxgi

import (
	"unsafe"

	"github.com/radix29/gogui/internal/shell/win32/com"
	"golang.org/x/sys/windows"
)

// Interface IDs.
var (
	IID_IDXGIObject     = com.MustGUID("{AEC22FB8-76F3-4639-9BE0-28EB43A67A2E}")
	IID_IDXGIDevice     = com.MustGUID("{54EC77FA-1377-44E6-8C32-88FD5F44C84C}")
	IID_IDXGIAdapter    = com.MustGUID("{2411E7E1-12AC-4CCF-BD14-9798E8534DC0}")
	IID_IDXGIFactory    = com.MustGUID("{7B7166EC-21C7-44AE-B21A-C9AE321AE369}")
	IID_IDXGIFactory1   = com.MustGUID("{770AAE78-F26F-4DBA-A829-253C83D1B387}")
	IID_IDXGIFactory2   = com.MustGUID("{50C83A1C-E072-4C48-87B0-3630FA36A6D0}")
	IID_IDXGISwapChain  = com.MustGUID("{310D36A0-D2E7-4C0A-AA04-6A9D23B8886A}")
	IID_IDXGISwapChain1 = com.MustGUID("{790A45F7-0D42-4876-983A-0A55CFE6F4AA}")
	IID_IDXGISwapChain2 = com.MustGUID("{A8BE2AC4-199F-4946-B331-79599FB98DE7}")
	IID_IDXGISurface    = com.MustGUID("{CAFCB56C-6AC3-4889-BF47-9E23BBD260EC}")
)

// Vtable slots, in header order: IUnknown 0–2, IDXGIObject 3–6
// (SetPrivateData, SetPrivateDataInterface, GetPrivateData, GetParent),
// then the interface's own methods. slots_test.go checks each one.
const (
	slotGetParent = 6

	slotDeviceGetAdapter = 7 // IDXGIDevice

	slotAdapterGetDesc = 8 // IDXGIAdapter

	slotFactoryCreateSwapChainForComposition = 24 // IDXGIFactory2

	// IDXGISwapChain (after IDXGIDeviceSubObject::GetDevice, 7).
	slotSwapChainPresent       = 8
	slotSwapChainGetBuffer     = 9
	slotSwapChainResizeBuffers = 13
	slotSwapChainGetDesc1      = 18 // IDXGISwapChain1

	// IDXGISwapChain2.
	slotSwapChainSetMaximumFrameLatency        = 31
	slotSwapChainGetMaximumFrameLatency        = 32
	slotSwapChainGetFrameLatencyWaitableObject = 33
)

// Device is IDXGIDevice.
type Device struct{ com.Ptr }

// DeviceFrom returns the IDXGIDevice of a D3D11 device through
// QueryInterface; p keeps its own reference.
func DeviceFrom(p com.Ptr) (Device, error) {
	q, err := p.QueryInterface(&IID_IDXGIDevice)
	return Device{q}, err
}

// Adapter returns the adapter the device was created on.
func (d Device) Adapter() (Adapter, error) {
	var out com.Ptr
	err := d.CallHR("IDXGIDevice::GetAdapter", slotDeviceGetAdapter, uintptr(unsafe.Pointer(&out)))
	return Adapter{out}, err
}

// Adapter is IDXGIAdapter.
type Adapter struct{ com.Ptr }

// Desc returns the adapter's description.
func (a Adapter) Desc() (AdapterDesc, error) {
	var d AdapterDesc
	err := a.CallHR("IDXGIAdapter::GetDesc", slotAdapterGetDesc, uintptr(unsafe.Pointer(&d)))
	return d, err
}

// Factory returns the factory that created the adapter as IDXGIFactory2
// (IDXGIObject::GetParent).
func (a Adapter) Factory() (Factory2, error) {
	var out com.Ptr
	err := a.CallHR("IDXGIAdapter::GetParent(IDXGIFactory2)", slotGetParent,
		uintptr(unsafe.Pointer(&IID_IDXGIFactory2)), uintptr(unsafe.Pointer(&out)))
	return Factory2{out}, err
}

// Factory2 is IDXGIFactory2.
type Factory2 struct{ com.Ptr }

// CreateSwapChainForComposition creates a swap chain for DirectComposition
// on device (the D3D11 device itself, not its IDXGIDevice). desc's Scaling
// must be SCALING_STRETCH and its SwapEffect a flip model; the chain is not
// restricted to an output.
func (f Factory2) CreateSwapChainForComposition(device com.Ptr, desc *SwapChainDesc1) (SwapChain1, error) {
	var out com.Ptr
	err := f.CallHR("IDXGIFactory2::CreateSwapChainForComposition", slotFactoryCreateSwapChainForComposition,
		uintptr(device), uintptr(unsafe.Pointer(desc)), 0, uintptr(unsafe.Pointer(&out)))
	return SwapChain1{out}, err
}

// SwapChain1 is IDXGISwapChain1.
type SwapChain1 struct{ com.Ptr }

// Present presents the back buffer. It returns the HRESULT, because a
// success code such as STATUS_OCCLUDED also matters to the caller, and an
// error for failures only.
func (s SwapChain1) Present(syncInterval, flags uint32) (com.HRESULT, error) {
	hr := com.HRESULT(int32(s.Call(slotSwapChainPresent, uintptr(syncInterval), uintptr(flags))))
	return hr, hr.Err("IDXGISwapChain::Present")
}

// Buffer returns back buffer i as the interface iid (ID3D11Texture2D or
// IDXGISurface). With a flip-model chain only buffer 0 can be written.
func (s SwapChain1) Buffer(i uint32, iid *com.GUID) (com.Ptr, error) {
	var out com.Ptr
	err := s.CallHR("IDXGISwapChain::GetBuffer", slotSwapChainGetBuffer,
		uintptr(i), uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	return out, err
}

// ResizeBuffers resizes the buffers to w×h pixels. count 0 and
// FORMAT_UNKNOWN keep the current ones; flags must repeat
// SWAP_CHAIN_FLAG_FRAME_LATENCY_WAITABLE_OBJECT if the chain was created
// with it. Every reference to a buffer must be released first (the D2D
// target bitmap included), or it fails with DXGI_ERROR_INVALID_CALL.
func (s SwapChain1) ResizeBuffers(count, w, h uint32, format Format, flags uint32) error {
	return s.CallHR("IDXGISwapChain::ResizeBuffers", slotSwapChainResizeBuffers,
		uintptr(count), uintptr(w), uintptr(h), uintptr(format), uintptr(flags))
}

// Desc1 returns the swap chain's description.
func (s SwapChain1) Desc1() (SwapChainDesc1, error) {
	var d SwapChainDesc1
	err := s.CallHR("IDXGISwapChain1::GetDesc1", slotSwapChainGetDesc1, uintptr(unsafe.Pointer(&d)))
	return d, err
}

// SwapChain2 returns the chain's IDXGISwapChain2 (Windows 8.1 and later).
func (s SwapChain1) SwapChain2() (SwapChain2, error) {
	q, err := s.QueryInterface(&IID_IDXGISwapChain2)
	return SwapChain2{q}, err
}

// SwapChain2 is IDXGISwapChain2. Only its own methods are bound on it; the
// inherited ones are reached through SwapChain1, which holds the same
// object.
type SwapChain2 struct{ com.Ptr }

// SetMaximumFrameLatency sets how many frames may be queued. The chain
// must have been created with SWAP_CHAIN_FLAG_FRAME_LATENCY_WAITABLE_OBJECT.
func (s SwapChain2) SetMaximumFrameLatency(n uint32) error {
	return s.CallHR("IDXGISwapChain2::SetMaximumFrameLatency", slotSwapChainSetMaximumFrameLatency, uintptr(n))
}

// MaximumFrameLatency returns the frame latency.
func (s SwapChain2) MaximumFrameLatency() (uint32, error) {
	var n uint32
	err := s.CallHR("IDXGISwapChain2::GetMaximumFrameLatency", slotSwapChainGetMaximumFrameLatency,
		uintptr(unsafe.Pointer(&n)))
	return n, err
}

// FrameLatencyWaitableObject returns the object that is signaled when the
// chain can take another frame. The caller owns the handle and closes it
// with windows.CloseHandle. 0 means the chain wasn't created with
// SWAP_CHAIN_FLAG_FRAME_LATENCY_WAITABLE_OBJECT.
func (s SwapChain2) FrameLatencyWaitableObject() windows.Handle {
	return windows.Handle(s.Call(slotSwapChainGetFrameLatencyWaitableObject))
}
