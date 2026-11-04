//go:build windows

package d3d11

import "github.com/radix29/gogui/internal/shell/win32/dxgi"

// Constants and mirrored structs from d3d11.h. The names are the SDK's
// without the D3D11_ prefix; the layouts are pinned by types_test.go.

// D3D11_USAGE.
const (
	USAGE_DEFAULT   = 0
	USAGE_IMMUTABLE = 1
	USAGE_DYNAMIC   = 2
	USAGE_STAGING   = 3
)

// D3D11_BIND_FLAG.
const (
	BIND_SHADER_RESOURCE = 0x8
	BIND_RENDER_TARGET   = 0x20
)

// D3D11_CPU_ACCESS_FLAG.
const (
	CPU_ACCESS_WRITE = 0x10000
	CPU_ACCESS_READ  = 0x20000
)

// D3D11_MAP.
const (
	MAP_READ       = 1
	MAP_WRITE      = 2
	MAP_READ_WRITE = 3
)

// Texture2DDesc is D3D11_TEXTURE2D_DESC.
type Texture2DDesc struct {
	Width, Height  uint32
	MipLevels      uint32
	ArraySize      uint32
	Format         dxgi.Format
	SampleDesc     dxgi.SampleDesc
	Usage          uint32
	BindFlags      uint32
	CPUAccessFlags uint32
	MiscFlags      uint32
}

// MappedSubresource is D3D11_MAPPED_SUBRESOURCE. PData is native memory,
// valid until Unmap.
type MappedSubresource struct {
	PData      uintptr
	RowPitch   uint32
	DepthPitch uint32
}
