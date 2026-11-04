//go:build windows

package dcomp

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/win32/com"
	"github.com/radix29/gogui/internal/shell/win32/d3d11"
	"github.com/radix29/gogui/internal/shell/win32/dxgi"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// The vtables of dcomp.h as MSVC lays them out: each overload pair comes
// in reverse declaration order, so the IDCompositionAnimation* form of
// SetOffsetX precedes the float form.
var (
	vtDevice = com.IUnknownVtable.Extend("Commit", "WaitForCommitCompletion", "GetFrameStatistics",
		"CreateTargetForHwnd", "CreateVisual", "CreateSurface", "CreateVirtualSurface",
		"CreateSurfaceFromHandle", "CreateSurfaceFromHwnd")
	vtTarget = com.IUnknownVtable.Extend("SetRoot")
	vtVisual = com.IUnknownVtable.Extend("SetOffsetX(animation)", "SetOffsetX(float)",
		"SetOffsetY(animation)", "SetOffsetY(float)", "SetTransform(object)",
		"SetTransform(matrix)", "SetTransformParent", "SetEffect", "SetBitmapInterpolationMode",
		"SetBorderMode", "SetClip(object)", "SetClip(rect)", "SetContent", "AddVisual",
		"RemoveVisual", "RemoveAllVisuals", "SetCompositeMode")
)

func TestSlots(t *testing.T) {
	com.CheckSlots(t, "IDCompositionDevice", vtDevice, map[string]int{
		"Commit":              slotDeviceCommit,
		"CreateTargetForHwnd": slotDeviceCreateTargetForHwnd,
		"CreateVisual":        slotDeviceCreateVisual,
	})
	com.CheckSlots(t, "IDCompositionTarget", vtTarget, map[string]int{"SetRoot": slotTargetSetRoot})
	com.CheckSlots(t, "IDCompositionVisual", vtVisual, map[string]int{"SetContent": slotVisualSetContent})
}

func TestProcsResolve(t *testing.T) {
	src, err := os.ReadFile("dcomp.go")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(src), "newProc(") - 1; len(allProcs) != n || n == 0 {
		t.Fatalf("%d procs registered, %d newProc calls in dcomp.go", len(allProcs), n)
	}
	for _, p := range allProcs {
		if err := p.Find(); err != nil {
			t.Errorf("proc %s: %v", p.Name, err)
		}
	}
}

// hiddenWindow creates a hidden top-level popup of a DefWindowProc class
// on the calling (locked) thread. The caller defers the returned destroy
// while the thread is still locked: a window can only be destroyed by its
// own thread, and t.Cleanup may run on another one once the thread is
// unlocked.
func hiddenWindow(t *testing.T, w, h int32) (hwnd windows.HWND, destroy func()) {
	t.Helper()
	inst, err := w32.GetModuleHandle()
	if err != nil {
		t.Fatal(err)
	}
	const class = "gogui.dcomptest"
	wc := w32.WndClassEx{LpfnWndProc: w32.DefWindowProcAddr(), HInstance: inst}
	if _, err := w32.RegisterClassEx(&wc, class); err != nil {
		t.Fatal(err)
	}
	hwnd, err = w32.CreateWindowEx(w32.WS_EX_NOREDIRECTIONBMP|w32.WS_EX_TOOLWINDOW, class, "", w32.WS_POPUP,
		0, 0, w, h, 0, 0, inst, 0)
	if err != nil {
		t.Fatal(err)
	}
	destroy = func() {
		if err := w32.DestroyWindow(hwnd); err != nil {
			t.Error(err)
		}
		if err := w32.UnregisterClass(class, inst); err != nil {
			t.Error(err)
		}
	}
	return hwnd, destroy
}

// queries checks that p answers each iid (so the IIDs are right) and
// gives back the reference.
func queries(t *testing.T, what string, p com.Ptr, iids ...*com.GUID) {
	t.Helper()
	for _, iid := range iids {
		q, err := p.QueryInterface(iid)
		if err != nil {
			t.Errorf("%s: QueryInterface(%v): %v", what, iid, err)
			continue
		}
		q.Release()
	}
}

// release releases p and checks the count left.
func release(t *testing.T, what string, p *com.Ptr, want uint32) {
	t.Helper()
	if n := p.Release(); n != want {
		t.Errorf("Release(%s) = %d, want %d", what, n, want)
	}
}

// TestSmoke builds the shell's pipeline by hand on a hidden window: a
// D3D11 device, its DXGI device, adapter and factory, a composition swap
// chain, a DComp device, target and visual. It clears the back buffer,
// reads the pixels back through a staging texture, presents and commits,
// resizes, and releases everything with every count back at 0.
func TestSmoke(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		dev, ctx, dt, err := d3d11.CreateDevice()
		if err != nil {
			t.Fatal(err)
		}
		smoke(t, dev, ctx, dt)
	})
	// The fallback device, whatever the host has.
	t.Run("WARP", func(t *testing.T) {
		dev, ctx, _, err := d3d11.CreateDeviceOfType(d3d11.DRIVER_TYPE_WARP, d3d11.CREATE_DEVICE_BGRA_SUPPORT)
		if err != nil {
			t.Fatal(err)
		}
		smoke(t, dev, ctx, d3d11.DRIVER_TYPE_WARP)
	})
}

func smoke(t *testing.T, dev d3d11.Device, ctx d3d11.DeviceContext, dt d3d11.DriverType) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const w, h = 64, 48
	hwnd, destroy := hiddenWindow(t, w, h)
	defer destroy()

	t.Logf("device: %v, feature level %#x", dt, dev.FeatureLevel())
	if dev.FeatureLevel() < d3d11.FEATURE_LEVEL_9_1 {
		t.Errorf("feature level %#x", dev.FeatureLevel())
	}
	if dev.CreationFlags()&d3d11.CREATE_DEVICE_BGRA_SUPPORT == 0 {
		t.Errorf("creation flags %#x lack BGRA_SUPPORT", dev.CreationFlags())
	}
	if err := dev.RemovedReason(); err != nil {
		t.Errorf("RemovedReason: %v", err)
	}
	queries(t, "device", dev.Ptr, &d3d11.IID_ID3D11Device, &dxgi.IID_IDXGIObject, &com.IID_IUnknown)
	queries(t, "context", ctx.Ptr, &d3d11.IID_ID3D11DeviceContext)
	// The context from CreateDevice and ImmediateContext are one object.
	ctx2 := dev.ImmediateContext()
	if ctx2.Ptr != ctx.Ptr {
		t.Errorf("ImmediateContext %#x, CreateDevice gave %#x", ctx2.Ptr, ctx.Ptr)
	}
	ctx2.Release()

	xdev, err := dxgi.DeviceFrom(dev.Ptr)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := xdev.Adapter()
	if err != nil {
		t.Fatal(err)
	}
	desc, err := adapter.Desc()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("adapter: %s (vendor %#x, device %#x, WARP %v)",
		windows.UTF16ToString(desc.Description[:]), desc.VendorID, desc.DeviceID, desc.IsWARP())
	if (dt == d3d11.DRIVER_TYPE_WARP) != desc.IsWARP() {
		t.Errorf("driver type %v, adapter IsWARP %v", dt, desc.IsWARP())
	}
	queries(t, "adapter", adapter.Ptr, &dxgi.IID_IDXGIAdapter, &dxgi.IID_IDXGIObject)
	factory, err := adapter.Factory()
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "factory", factory.Ptr, &dxgi.IID_IDXGIFactory, &dxgi.IID_IDXGIFactory1, &dxgi.IID_IDXGIFactory2)

	scd := dxgi.SwapChainDesc1{
		Width: w, Height: h, Format: dxgi.FORMAT_B8G8R8A8_UNORM,
		SampleDesc:  dxgi.SampleDesc{Count: 1},
		BufferUsage: dxgi.USAGE_RENDER_TARGET_OUTPUT, BufferCount: 2,
		Scaling: dxgi.SCALING_STRETCH, SwapEffect: dxgi.SWAP_EFFECT_FLIP_SEQUENTIAL,
		AlphaMode: dxgi.ALPHA_MODE_PREMULTIPLIED,
		Flags:     dxgi.SWAP_CHAIN_FLAG_FRAME_LATENCY_WAITABLE_OBJECT,
	}
	chain, err := factory.CreateSwapChainForComposition(dev.Ptr, &scd)
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "swap chain", chain.Ptr, &dxgi.IID_IDXGISwapChain, &dxgi.IID_IDXGISwapChain1)
	if got, err := chain.Desc1(); err != nil || got != scd {
		t.Errorf("Desc1 = %+v, %v; want %+v", got, err, scd)
	}
	chain2, err := chain.SwapChain2()
	if err != nil {
		t.Fatal(err)
	}
	if err := chain2.SetMaximumFrameLatency(1); err != nil {
		t.Error(err)
	}
	if n, err := chain2.MaximumFrameLatency(); n != 1 || err != nil {
		t.Errorf("MaximumFrameLatency = %d, %v", n, err)
	}
	waitable := chain2.FrameLatencyWaitableObject()
	if waitable == 0 {
		t.Fatal("no frame-latency waitable object")
	}
	// A fresh chain can take a frame: the object is signaled.
	if ev, err := windows.WaitForSingleObject(waitable, 1000); ev != windows.WAIT_OBJECT_0 {
		t.Errorf("WaitForSingleObject(waitable) = %#x, %v", ev, err)
	}

	// The composition tree.
	cdev, err := CreateDevice(xdev.Ptr)
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "dcomp device", cdev.Ptr, &IID_IDCompositionDevice)
	target, err := cdev.CreateTargetForHwnd(hwnd, false)
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "dcomp target", target.Ptr, &IID_IDCompositionTarget)
	if extra, err := cdev.CreateTargetForHwnd(hwnd, false); !errors.Is(err, DCOMPOSITION_ERROR_WINDOW_ALREADY_COMPOSED) {
		t.Errorf("second CreateTargetForHwnd: %v", err)
		extra.Release()
	}
	visual, err := cdev.CreateVisual()
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "dcomp visual", visual.Ptr, &IID_IDCompositionVisual)
	// SetContent keeps the chain through an object of its own: the chain's
	// count doesn't change (finding, S2.4b).
	before := chain.RefCount()
	if err := visual.SetContent(chain.Ptr); err != nil {
		t.Fatal(err)
	}
	if n := chain.RefCount(); n != before {
		t.Logf("SetContent changed the chain's count %d → %d", before, n)
	}
	if err := target.SetRoot(visual); err != nil {
		t.Fatal(err)
	}

	// Clear the back buffer, copy it to a staging texture and read it.
	tex, err := chain.Buffer(0, &d3d11.IID_ID3D11Texture2D)
	if err != nil {
		t.Fatal(err)
	}
	back := d3d11.Texture2D{Ptr: tex}
	queries(t, "back buffer", back.Ptr, &d3d11.IID_ID3D11Resource, &dxgi.IID_IDXGISurface)
	bd := back.Desc()
	if bd.Width != w || bd.Height != h || bd.Format != dxgi.FORMAT_B8G8R8A8_UNORM {
		t.Errorf("back buffer %+v", bd)
	}
	rtv, err := dev.CreateRenderTargetView(back.Ptr)
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "render target view", rtv.Ptr, &d3d11.IID_ID3D11RenderTargetView)
	ctx.SetRenderTarget(rtv)
	// Premultiplied: B 0.2, G 0.4, R 0.6 at alpha 1, as 51, 102, 153, 255.
	ctx.ClearRenderTargetView(rtv, [4]float32{0.6, 0.4, 0.2, 1})
	sd := bd
	sd.Usage = d3d11.USAGE_STAGING
	sd.BindFlags = 0
	sd.CPUAccessFlags = d3d11.CPU_ACCESS_READ
	sd.MiscFlags = 0
	staging, err := dev.CreateTexture2D(&sd)
	if err != nil {
		t.Fatal(err)
	}
	ctx.CopyResource(staging.Ptr, back.Ptr)
	m, err := ctx.Map(staging.Ptr, 0, d3d11.MAP_READ, 0)
	if err != nil {
		t.Fatal(err)
	}
	if m.RowPitch < w*4 {
		t.Errorf("RowPitch %d", m.RowPitch)
	}
	pix := ffi.CopyBytes(m.PData, int(m.RowPitch)*(h-1)+w*4)
	ctx.Unmap(staging.Ptr, 0)
	for _, pt := range [][2]int{{0, 0}, {w - 1, 0}, {w / 2, h / 2}, {0, h - 1}, {w - 1, h - 1}} {
		o := pt[1]*int(m.RowPitch) + pt[0]*4
		if got := [4]byte(pix[o : o+4]); got != [4]byte{51, 102, 153, 255} {
			t.Errorf("pixel %v = %v, want BGRA 51 102 153 255", pt, got)
		}
	}

	ctx.SetRenderTarget(d3d11.RenderTargetView{})
	release(t, "render target view", &rtv.Ptr, 0)
	if _, err := chain.Present(1, 0); err != nil {
		t.Errorf("Present: %v", err)
	}
	if err := cdev.Commit(); err != nil {
		t.Errorf("Commit: %v", err)
	}

	// ResizeBuffers fails while a buffer is referenced, then succeeds.
	flags := uint32(dxgi.SWAP_CHAIN_FLAG_FRAME_LATENCY_WAITABLE_OBJECT)
	if err := chain.ResizeBuffers(0, 2*w, 2*h, dxgi.FORMAT_UNKNOWN, flags); !errors.Is(err, com.DXGI_ERROR_INVALID_CALL) {
		t.Errorf("ResizeBuffers with a live buffer: %v", err)
	}
	release(t, "back buffer", &back.Ptr, 0)
	if err := chain.ResizeBuffers(0, 2*w, 2*h, dxgi.FORMAT_UNKNOWN, flags); err != nil {
		t.Fatalf("ResizeBuffers: %v", err)
	}
	if got, err := chain.Desc1(); err != nil || got.Width != 2*w || got.Height != 2*h || got.BufferCount != 2 {
		t.Errorf("after the resize: %+v, %v", got, err)
	}
	if tex, err := chain.Buffer(0, &d3d11.IID_ID3D11Texture2D); err != nil {
		t.Error(err)
	} else {
		back = d3d11.Texture2D{Ptr: tex}
		if bd := back.Desc(); bd.Width != 2*w || bd.Height != 2*h {
			t.Errorf("resized back buffer %dx%d", bd.Width, bd.Height)
		}
		release(t, "resized back buffer", &back.Ptr, 0)
	}

	// Teardown: the tree first, then the chain, the DXGI objects and the
	// device, whose last Release must find no reference left.
	if err := target.SetRoot(Visual{}); err != nil {
		t.Error(err)
	}
	if err := visual.SetContent(0); err != nil {
		t.Error(err)
	}
	if err := cdev.Commit(); err != nil {
		t.Error(err)
	}
	if err := windows.CloseHandle(waitable); err != nil {
		t.Error(err)
	}
	release(t, "staging texture", &staging.Ptr, 0)
	release(t, "visual", &visual.Ptr, 0)
	release(t, "target", &target.Ptr, 0)
	release(t, "dcomp device", &cdev.Ptr, 0)
	chain2.Release()
	release(t, "swap chain", &chain.Ptr, 0)
	// The adapter holds its parent factory, and the device its adapter, so
	// those two counts reach 0 only with the device.
	release(t, "factory", &factory.Ptr, 1)
	release(t, "adapter", &adapter.Ptr, 1)
	xdev.Release()
	ctx.ClearState()
	ctx.Flush()
	ctx.Release()
	release(t, "device", &dev.Ptr, 0)
}
