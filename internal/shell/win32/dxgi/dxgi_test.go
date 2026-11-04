//go:build windows

package dxgi

import (
	"testing"
	"unsafe"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/win32/com"
)

// The full vtables from dxgi.h, dxgi1_2.h and dxgi1_3.h, in declaration
// order; every slot constant of the package is checked against them.
var (
	vtObject          = com.IUnknownVtable.Extend("SetPrivateData", "SetPrivateDataInterface", "GetPrivateData", "GetParent")
	vtDeviceSubObject = vtObject.Extend("GetDevice")
	vtDevice          = vtObject.Extend("GetAdapter", "CreateSurface", "QueryResourceResidency",
		"SetGPUThreadPriority", "GetGPUThreadPriority")
	vtAdapter = vtObject.Extend("EnumOutputs", "GetDesc", "CheckInterfaceSupport")
	vtFactory = vtObject.Extend("EnumAdapters", "MakeWindowAssociation", "GetWindowAssociation",
		"CreateSwapChain", "CreateSoftwareAdapter")
	vtFactory1 = vtFactory.Extend("EnumAdapters1", "IsCurrent")
	vtFactory2 = vtFactory1.Extend("IsWindowedStereoEnabled", "CreateSwapChainForHwnd",
		"CreateSwapChainForCoreWindow", "GetSharedResourceAdapterLuid", "RegisterStereoStatusWindow",
		"RegisterStereoStatusEvent", "UnregisterStereoStatus", "RegisterOcclusionStatusWindow",
		"RegisterOcclusionStatusEvent", "UnregisterOcclusionStatus", "CreateSwapChainForComposition")
	vtSwapChain = vtDeviceSubObject.Extend("Present", "GetBuffer", "SetFullscreenState",
		"GetFullscreenState", "GetDesc", "ResizeBuffers", "ResizeTarget", "GetContainingOutput",
		"GetFrameStatistics", "GetLastPresentCount")
	vtSwapChain1 = vtSwapChain.Extend("GetDesc1", "GetFullscreenDesc", "GetHwnd", "GetCoreWindow",
		"Present1", "IsTemporaryMonoSupported", "GetRestrictToOutput", "SetBackgroundColor",
		"GetBackgroundColor", "SetRotation", "GetRotation")
	vtSwapChain2 = vtSwapChain1.Extend("SetSourceSize", "GetSourceSize", "SetMaximumFrameLatency",
		"GetMaximumFrameLatency", "GetFrameLatencyWaitableObject", "SetMatrixTransform",
		"GetMatrixTransform")
)

func TestSlots(t *testing.T) {
	if len(vtFactory2) != 25 || len(vtSwapChain2) != 36 {
		t.Errorf("vtable lengths %d, %d; want 25, 36", len(vtFactory2), len(vtSwapChain2))
	}
	com.CheckSlots(t, "IDXGIObject", vtObject, map[string]int{"GetParent": slotGetParent})
	com.CheckSlots(t, "IDXGIDevice", vtDevice, map[string]int{"GetAdapter": slotDeviceGetAdapter})
	com.CheckSlots(t, "IDXGIAdapter", vtAdapter, map[string]int{"GetDesc": slotAdapterGetDesc})
	com.CheckSlots(t, "IDXGIFactory2", vtFactory2, map[string]int{
		"CreateSwapChainForComposition": slotFactoryCreateSwapChainForComposition})
	com.CheckSlots(t, "IDXGISwapChain1", vtSwapChain1, map[string]int{
		"Present":       slotSwapChainPresent,
		"GetBuffer":     slotSwapChainGetBuffer,
		"ResizeBuffers": slotSwapChainResizeBuffers,
		"GetDesc1":      slotSwapChainGetDesc1,
	})
	com.CheckSlots(t, "IDXGISwapChain2", vtSwapChain2, map[string]int{
		"SetMaximumFrameLatency":        slotSwapChainSetMaximumFrameLatency,
		"GetMaximumFrameLatency":        slotSwapChainGetMaximumFrameLatency,
		"GetFrameLatencyWaitableObject": slotSwapChainGetFrameLatencyWaitableObject,
	})
}

func f(name string, off, size uintptr) ffi.Field {
	return ffi.Field{Name: name, Offset: off, Size: size}
}

// The expected layouts come from the SDK headers; amd64 and arm64 are
// both LLP64, so one table serves both (arm64 runs through native.sh).
func TestStructLayouts(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Fatal("the layout tables are for 64-bit Windows only")
	}
	for _, c := range []struct {
		v    any
		want ffi.Layout
	}{
		{SampleDesc{}, ffi.Layout{Size: 8, Align: 4, Fields: []ffi.Field{f("Count", 0, 4), f("Quality", 4, 4)}}},
		{SwapChainDesc1{}, ffi.Layout{Size: 48, Align: 4, Fields: []ffi.Field{
			f("Width", 0, 4), f("Height", 4, 4), f("Format", 8, 4), f("Stereo", 12, 4),
			f("SampleDesc", 16, 8), f("BufferUsage", 24, 4), f("BufferCount", 28, 4),
			f("Scaling", 32, 4), f("SwapEffect", 36, 4), f("AlphaMode", 40, 4), f("Flags", 44, 4)}}},
		{Luid{}, ffi.Layout{Size: 8, Align: 4, Fields: []ffi.Field{f("LowPart", 0, 4), f("HighPart", 4, 4)}}},
		{AdapterDesc{}, ffi.Layout{Size: 304, Align: 8, Fields: []ffi.Field{
			f("Description", 0, 256), f("VendorID", 256, 4), f("DeviceID", 260, 4),
			f("SubSysID", 264, 4), f("Revision", 268, 4), f("DedicatedVideoMemory", 272, 8),
			f("DedicatedSystemMemory", 280, 8), f("SharedSystemMemory", 288, 8),
			f("AdapterLuid", 296, 8)}}},
	} {
		ffi.CheckLayout(t, c.v, c.want)
		ffi.CheckNoGoPointers(t, c.v)
	}
}

func TestStatusOccluded(t *testing.T) {
	if !STATUS_OCCLUDED.Succeeded() || STATUS_OCCLUDED.Facility() != 0x87A || STATUS_OCCLUDED.Code() != 1 {
		t.Errorf("STATUS_OCCLUDED = %v", STATUS_OCCLUDED)
	}
}
