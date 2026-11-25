//go:build windows

package d3d11

import (
	"errors"
	"os"
	"strings"
	"testing"
	"unsafe"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/win32/com"
)

// The full vtables of d3d11.h, in declaration order.
var (
	vtDevice = com.IUnknownVtable.Extend("CreateBuffer", "CreateTexture1D", "CreateTexture2D",
		"CreateTexture3D", "CreateShaderResourceView", "CreateUnorderedAccessView",
		"CreateRenderTargetView", "CreateDepthStencilView", "CreateInputLayout", "CreateVertexShader",
		"CreateGeometryShader", "CreateGeometryShaderWithStreamOutput", "CreatePixelShader",
		"CreateHullShader", "CreateDomainShader", "CreateComputeShader", "CreateClassLinkage",
		"CreateBlendState", "CreateDepthStencilState", "CreateRasterizerState", "CreateSamplerState",
		"CreateQuery", "CreatePredicate", "CreateCounter", "CreateDeferredContext",
		"OpenSharedResource", "CheckFormatSupport", "CheckMultisampleQualityLevels",
		"CheckCounterInfo", "CheckCounter", "CheckFeatureSupport", "GetPrivateData",
		"SetPrivateData", "SetPrivateDataInterface", "GetFeatureLevel", "GetCreationFlags",
		"GetDeviceRemovedReason", "GetImmediateContext", "SetExceptionMode", "GetExceptionMode")
	vtDeviceChild = com.IUnknownVtable.Extend("GetDevice", "GetPrivateData", "SetPrivateData",
		"SetPrivateDataInterface")
	vtResource  = vtDeviceChild.Extend("GetType", "SetEvictionPriority", "GetEvictionPriority")
	vtTexture2D = vtResource.Extend("GetDesc")
	vtContext   = vtDeviceChild.Extend("VSSetConstantBuffers", "PSSetShaderResources",
		"PSSetShader", "PSSetSamplers", "VSSetShader", "DrawIndexed", "Draw", "Map", "Unmap",
		"PSSetConstantBuffers", "IASetInputLayout", "IASetVertexBuffers", "IASetIndexBuffer",
		"DrawIndexedInstanced", "DrawInstanced", "GSSetConstantBuffers", "GSSetShader",
		"IASetPrimitiveTopology", "VSSetShaderResources", "VSSetSamplers", "Begin", "End",
		"GetData", "SetPredication", "GSSetShaderResources", "GSSetSamplers", "OMSetRenderTargets",
		"OMSetRenderTargetsAndUnorderedAccessViews", "OMSetBlendState", "OMSetDepthStencilState",
		"SOSetTargets", "DrawAuto", "DrawIndexedInstancedIndirect", "DrawInstancedIndirect",
		"Dispatch", "DispatchIndirect", "RSSetState", "RSSetViewports", "RSSetScissorRects",
		"CopySubresourceRegion", "CopyResource", "UpdateSubresource", "CopyStructureCount",
		"ClearRenderTargetView", "ClearUnorderedAccessViewUint", "ClearUnorderedAccessViewFloat",
		"ClearDepthStencilView", "GenerateMips", "SetResourceMinLOD", "GetResourceMinLOD",
		"ResolveSubresource", "ExecuteCommandList", "HSSetShaderResources", "HSSetShader",
		"HSSetSamplers", "HSSetConstantBuffers", "DSSetShaderResources", "DSSetShader",
		"DSSetSamplers", "DSSetConstantBuffers", "CSSetShaderResources",
		"CSSetUnorderedAccessViews", "CSSetShader", "CSSetSamplers", "CSSetConstantBuffers",
		"VSGetConstantBuffers", "PSGetShaderResources", "PSGetShader", "PSGetSamplers",
		"VSGetShader", "PSGetConstantBuffers", "IAGetInputLayout", "IAGetVertexBuffers",
		"IAGetIndexBuffer", "GSGetConstantBuffers", "GSGetShader", "IAGetPrimitiveTopology",
		"VSGetShaderResources", "VSGetSamplers", "GetPredication", "GSGetShaderResources",
		"GSGetSamplers", "OMGetRenderTargets", "OMGetRenderTargetsAndUnorderedAccessViews",
		"OMGetBlendState", "OMGetDepthStencilState", "SOGetTargets", "RSGetState",
		"RSGetViewports", "RSGetScissorRects", "HSGetShaderResources", "HSGetShader",
		"HSGetSamplers", "HSGetConstantBuffers", "DSGetShaderResources", "DSGetShader",
		"DSGetSamplers", "DSGetConstantBuffers", "CSGetShaderResources",
		"CSGetUnorderedAccessViews", "CSGetShader", "CSGetSamplers", "CSGetConstantBuffers",
		"ClearState", "Flush", "GetType", "GetContextFlags", "FinishCommandList")
)

func TestSlots(t *testing.T) {
	if len(vtDevice) != 43 || len(vtContext) != 115 {
		t.Errorf("vtable lengths %d, %d; want 43, 115", len(vtDevice), len(vtContext))
	}
	com.CheckSlots(t, "ID3D11Device", vtDevice, map[string]int{
		"CreateTexture2D":        slotDeviceCreateTexture2D,
		"CreateRenderTargetView": slotDeviceCreateRenderTargetView,
	})
	com.CheckSlots(t, "ID3D11DeviceContext", vtContext, map[string]int{
		"Map":                   slotContextMap,
		"Unmap":                 slotContextUnmap,
		"OMSetRenderTargets":    slotContextOMSetRenderTargets,
		"CopyResource":          slotContextCopyResource,
		"ClearRenderTargetView": slotContextClearRenderTargetView,
		"ClearState":            slotContextClearState,
		"Flush":                 slotContextFlush,
	})
	com.CheckSlots(t, "ID3D11Texture2D", vtTexture2D, map[string]int{"GetDesc": slotTexture2DGetDesc})
}

func f(name string, off, size uintptr) ffi.Field {
	return ffi.Field{Name: name, Offset: off, Size: size}
}

func TestStructLayouts(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Fatal("the layout tables are for 64-bit Windows only")
	}
	for _, c := range []struct {
		v    any
		want ffi.Layout
	}{
		{Texture2DDesc{}, ffi.Layout{Size: 44, Align: 4, Fields: []ffi.Field{
			f("Width", 0, 4), f("Height", 4, 4), f("MipLevels", 8, 4), f("ArraySize", 12, 4),
			f("Format", 16, 4), f("SampleDesc", 20, 8), f("Usage", 28, 4), f("BindFlags", 32, 4),
			f("CPUAccessFlags", 36, 4), f("MiscFlags", 40, 4)}}},
		{MappedSubresource{}, ffi.Layout{Size: 16, Align: 8, Fields: []ffi.Field{
			f("PData", 0, 8), f("RowPitch", 8, 4), f("DepthPitch", 12, 4)}}},
	} {
		ffi.CheckLayout(t, c.v, c.want)
		ffi.CheckNoGoPointers(t, c.v)
	}
}

// TestProcsResolve finds every declared proc (as w32's test does).
func TestProcsResolve(t *testing.T) {
	src, err := os.ReadFile("d3d11.go")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(src), "newProc(") - 1; len(allProcs) != n || n == 0 {
		t.Fatalf("%d procs registered, %d newProc calls in d3d11.go", len(allProcs), n)
	}
	for _, p := range allProcs {
		if err := p.Find(); err != nil {
			t.Errorf("proc %s: %v", p.Name, err)
		}
	}
}

func TestDriverTypeString(t *testing.T) {
	if DRIVER_TYPE_HARDWARE.String() != "hardware" || DRIVER_TYPE_WARP.String() != "WARP" ||
		DriverType(12).String() != "driver type 12" {
		t.Error("DriverType.String")
	}
}

func TestCreateError(t *testing.T) {
	e := &CreateError{Hardware: com.E_FAIL.Err("hw"), WARP: com.E_OUTOFMEMORY.Err("warp")}
	if !errors.Is(e, com.E_FAIL) || !errors.Is(e, com.E_OUTOFMEMORY) ||
		!strings.Contains(e.Error(), "E_FAIL") || !strings.Contains(e.Error(), "E_OUTOFMEMORY") {
		t.Errorf("CreateError %v", e)
	}
}
