//go:build windows

package dxgi

import "github.com/radix29/gogui/internal/shell/win32/com"

// Constants and mirrored structs from dxgiformat.h, dxgitype.h, dxgi.h and
// dxgi1_2.h. The names are the SDK's without the DXGI_ prefix. The layouts
// are pinned by types_test.go.

// Format is DXGI_FORMAT.
type Format uint32

// Formats.
const (
	FORMAT_UNKNOWN        Format = 0
	FORMAT_R8G8B8A8_UNORM Format = 28
	FORMAT_B8G8R8A8_UNORM Format = 87
)

// DXGI_USAGE flags.
const (
	USAGE_SHADER_INPUT         = 0x10
	USAGE_RENDER_TARGET_OUTPUT = 0x20
)

// DXGI_SCALING.
const (
	SCALING_STRETCH = 0
	SCALING_NONE    = 1
)

// DXGI_SWAP_EFFECT.
const (
	SWAP_EFFECT_DISCARD         = 0
	SWAP_EFFECT_SEQUENTIAL      = 1
	SWAP_EFFECT_FLIP_SEQUENTIAL = 3
	SWAP_EFFECT_FLIP_DISCARD    = 4
)

// DXGI_ALPHA_MODE.
const (
	ALPHA_MODE_UNSPECIFIED   = 0
	ALPHA_MODE_PREMULTIPLIED = 1
	ALPHA_MODE_STRAIGHT      = 2
	ALPHA_MODE_IGNORE        = 3
)

// DXGI_SWAP_CHAIN_FLAG.
const SWAP_CHAIN_FLAG_FRAME_LATENCY_WAITABLE_OBJECT = 0x40

// DXGI_PRESENT flags.
const (
	PRESENT_TEST        = 0x1
	PRESENT_RESTART     = 0x4
	PRESENT_DO_NOT_WAIT = 0x8
)

// STATUS_OCCLUDED is the success code Present returns when no part of the
// chain is visible (DXGI_STATUS_OCCLUDED).
const STATUS_OCCLUDED com.HRESULT = 0x087A0001

// SampleDesc is DXGI_SAMPLE_DESC.
type SampleDesc struct {
	Count, Quality uint32
}

// SwapChainDesc1 is DXGI_SWAP_CHAIN_DESC1.
type SwapChainDesc1 struct {
	Width, Height uint32
	Format        Format
	Stereo        int32 // BOOL
	SampleDesc    SampleDesc
	BufferUsage   uint32
	BufferCount   uint32
	Scaling       uint32
	SwapEffect    uint32
	AlphaMode     uint32
	Flags         uint32
}

// Luid is LUID.
type Luid struct {
	LowPart  uint32
	HighPart int32
}

// AdapterDesc is DXGI_ADAPTER_DESC.
type AdapterDesc struct {
	Description           [128]uint16
	VendorID              uint32
	DeviceID              uint32
	SubSysID              uint32
	Revision              uint32
	DedicatedVideoMemory  uintptr // SIZE_T
	DedicatedSystemMemory uintptr
	SharedSystemMemory    uintptr
	AdapterLuid           Luid
}

// The Microsoft Basic Render Driver (WARP) reports these IDs.
const (
	VendorMicrosoft = 0x1414
	DeviceWARP      = 0x8C
)

// IsWARP reports whether the adapter is the WARP software rasterizer.
func (d *AdapterDesc) IsWARP() bool {
	return d.VendorID == VendorMicrosoft && d.DeviceID == DeviceWARP
}

// FrameStatistics is DXGI_FRAME_STATISTICS: the chain's most recent
// present the display has shown. PresentCount numbers the presents as
// GetLastPresentCount does; PresentRefreshCount is the vblank count when
// it was shown, SyncRefreshCount the vblank count when the statistics
// were sampled, at SyncQPCTime (performance-counter ticks).
type FrameStatistics struct {
	PresentCount        uint32
	PresentRefreshCount uint32
	SyncRefreshCount    uint32
	SyncQPCTime         int64 // LARGE_INTEGER
	SyncGPUTime         int64 // LARGE_INTEGER
}
