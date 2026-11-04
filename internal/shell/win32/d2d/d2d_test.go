//go:build windows

package d2d

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/win32/com"
	"github.com/radix29/gogui/internal/shell/win32/d3d11"
	"github.com/radix29/gogui/internal/shell/win32/dcomp"
	"github.com/radix29/gogui/internal/shell/win32/dxgi"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// The full vtables of d2d1.h and d2d1_1.h in declaration order. A derived
// interface that redeclares a base method (ID2D1DeviceContext::CreateBitmap,
// ID2D1Factory1::CreateStrokeStyle, …) gets a new slot; those carry a "1"
// suffix here so every name is unique. The non-virtual inline overloads of
// the C++ header take no slot.
var (
	vtResource = com.IUnknownVtable.Extend("GetFactory")
	vtFactory  = com.IUnknownVtable.Extend("ReloadSystemMetrics", "GetDesktopDpi",
		"CreateRectangleGeometry", "CreateRoundedRectangleGeometry", "CreateEllipseGeometry",
		"CreateGeometryGroup", "CreateTransformedGeometry", "CreatePathGeometry",
		"CreateStrokeStyle", "CreateDrawingStateBlock", "CreateWicBitmapRenderTarget",
		"CreateHwndRenderTarget", "CreateDxgiSurfaceRenderTarget", "CreateDCRenderTarget")
	vtFactory1 = vtFactory.Extend("CreateDevice", "CreateStrokeStyle1", "CreatePathGeometry1",
		"CreateDrawingStateBlock1", "CreateGdiMetafile", "RegisterEffectFromStream",
		"RegisterEffectFromString", "UnregisterEffect", "GetRegisteredEffects", "GetEffectProperties")
	vtDevice = vtResource.Extend("CreateDeviceContext", "CreatePrintControl",
		"SetMaximumTextureMemory", "GetMaximumTextureMemory", "ClearResources")
	vtRenderTarget = vtResource.Extend("CreateBitmap", "CreateBitmapFromWicBitmap",
		"CreateSharedBitmap", "CreateBitmapBrush", "CreateSolidColorBrush",
		"CreateGradientStopCollection", "CreateLinearGradientBrush", "CreateRadialGradientBrush",
		"CreateCompatibleRenderTarget", "CreateLayer", "CreateMesh", "DrawLine", "DrawRectangle",
		"FillRectangle", "DrawRoundedRectangle", "FillRoundedRectangle", "DrawEllipse",
		"FillEllipse", "DrawGeometry", "FillGeometry", "FillMesh", "FillOpacityMask", "DrawBitmap",
		"DrawText", "DrawTextLayout", "DrawGlyphRun", "SetTransform", "GetTransform",
		"SetAntialiasMode", "GetAntialiasMode", "SetTextAntialiasMode", "GetTextAntialiasMode",
		"SetTextRenderingParams", "GetTextRenderingParams", "SetTags", "GetTags", "PushLayer",
		"PopLayer", "Flush", "SaveDrawingState", "RestoreDrawingState", "PushAxisAlignedClip",
		"PopAxisAlignedClip", "Clear", "BeginDraw", "EndDraw", "GetPixelFormat", "SetDpi",
		"GetDpi", "GetSize", "GetPixelSize", "GetMaximumBitmapSize", "IsSupported")
	vtDeviceContext = vtRenderTarget.Extend("CreateBitmap1", "CreateBitmapFromWicBitmap1",
		"CreateColorContext", "CreateColorContextFromFilename",
		"CreateColorContextFromWicColorContext", "CreateBitmapFromDxgiSurface", "CreateEffect",
		"CreateGradientStopCollection1", "CreateImageBrush", "CreateBitmapBrush1",
		"CreateCommandList", "IsDxgiFormatSupported", "IsBufferPrecisionSupported",
		"GetImageLocalBounds", "GetImageWorldBounds", "GetGlyphRunWorldBounds", "GetDevice",
		"SetTarget", "GetTarget", "SetRenderingControls", "GetRenderingControls",
		"SetPrimitiveBlend", "GetPrimitiveBlend", "SetUnitMode", "GetUnitMode", "DrawGlyphRun1",
		"DrawImage", "DrawGdiMetafile", "DrawBitmap1", "PushLayer1",
		"InvalidateEffectInputRectangle", "GetEffectInvalidRectangleCount",
		"GetEffectInvalidRectangles", "GetEffectRequiredInputRectangles", "FillOpacityMask1")
	vtImage   = vtResource // ID2D1Image adds no methods
	vtBitmap  = vtImage.Extend("GetSize", "GetPixelSize", "GetPixelFormat", "GetDpi", "CopyFromBitmap", "CopyFromRenderTarget", "CopyFromMemory")
	vtBitmap1 = vtBitmap.Extend("GetColorContext", "GetOptions", "GetSurface", "Map", "Unmap")
)

// Live slots not bound by the package, called by the smoke test to check
// the tables against the real objects.
var (
	slotResourceGetFactory = vtResource.Slot("GetFactory")
	slotDCGetDevice        = vtDeviceContext.Slot("GetDevice")
)

func TestSlots(t *testing.T) {
	if len(vtFactory1) != 27 || len(vtRenderTarget) != 57 || len(vtDeviceContext) != 92 ||
		len(vtDevice) != 9 || len(vtBitmap1) != 16 {
		t.Errorf("vtable lengths %d, %d, %d, %d, %d; want 27, 57, 92, 9, 16",
			len(vtFactory1), len(vtRenderTarget), len(vtDeviceContext), len(vtDevice), len(vtBitmap1))
	}
	com.CheckSlots(t, "ID2D1Factory1", vtFactory1, map[string]int{"CreateDevice": slotFactoryCreateDevice})
	com.CheckSlots(t, "ID2D1Device", vtDevice, map[string]int{"CreateDeviceContext": slotDeviceCreateDeviceContext})
	com.CheckSlots(t, "ID2D1DeviceContext", vtDeviceContext, map[string]int{
		"Clear":                       slotRTClear,
		"BeginDraw":                   slotRTBeginDraw,
		"EndDraw":                     slotRTEndDraw,
		"SetDpi":                      slotRTSetDpi,
		"GetDpi":                      slotRTGetDpi,
		"CreateBitmapFromDxgiSurface": slotDCCreateBitmapFromDxgiSurface,
		"SetTarget":                   slotDCSetTarget,
		"GetTarget":                   slotDCGetTarget,
	})
	com.CheckSlots(t, "ID2D1Bitmap1", vtBitmap1, map[string]int{
		"GetDpi":     slotBitmapGetDpi,
		"GetOptions": slotBitmap1GetOptions,
		"GetSurface": slotBitmap1GetSurface,
	})
}

func f(name string, off, size uintptr) ffi.Field {
	return ffi.Field{Name: name, Offset: off, Size: size}
}

// The layouts come from the SDK headers; amd64 and arm64 are both LLP64
// (arm64 runs through native.sh).
func TestStructLayouts(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Fatal("the layout tables are for 64-bit Windows only")
	}
	for _, c := range []struct {
		v    any
		want ffi.Layout
	}{
		{FactoryOptions{}, ffi.Layout{Size: 4, Align: 4, Fields: []ffi.Field{f("DebugLevel", 0, 4)}}},
		{PixelFormat{}, ffi.Layout{Size: 8, Align: 4, Fields: []ffi.Field{f("Format", 0, 4), f("AlphaMode", 4, 4)}}},
		{BitmapProperties1{}, ffi.Layout{Size: 32, Align: 8, Fields: []ffi.Field{
			f("PixelFormat", 0, 8), f("DpiX", 8, 4), f("DpiY", 12, 4), f("BitmapOptions", 16, 4),
			f("ColorContext", 24, 8)}}},
		{ColorF{}, ffi.Layout{Size: 16, Align: 4, Fields: []ffi.Field{
			f("R", 0, 4), f("G", 4, 4), f("B", 8, 4), f("A", 12, 4)}}},
	} {
		ffi.CheckLayout(t, c.v, c.want)
		ffi.CheckNoGoPointers(t, c.v)
	}
}

func TestProcsResolve(t *testing.T) {
	src, err := os.ReadFile("d2d.go")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(src), "newProc(") - 1; len(allProcs) != n || n == 0 {
		t.Fatalf("%d procs registered, %d newProc calls in d2d.go", len(allProcs), n)
	}
	for _, p := range allProcs {
		if err := p.Find(); err != nil {
			t.Errorf("proc %s: %v", p.Name, err)
		}
	}
}

func TestRecreateTarget(t *testing.T) {
	if D2DERR_RECREATE_TARGET.Facility() != 0x899 || D2DERR_RECREATE_TARGET.Code() != 0xC {
		t.Errorf("D2DERR_RECREATE_TARGET = %v", D2DERR_RECREATE_TARGET)
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
	const class = "gogui.d2dtest"
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

func release(t *testing.T, what string, p *com.Ptr, want uint32) {
	t.Helper()
	if n := p.Release(); n != want {
		t.Errorf("Release(%s) = %d, want %d", what, n, want)
	}
}

// same reports whether a and b are one COM object (IUnknown identity).
func same(t *testing.T, a, b com.Ptr) bool {
	t.Helper()
	ua, err := a.QueryInterface(&com.IID_IUnknown)
	if err != nil {
		t.Fatal(err)
	}
	ub, err := b.QueryInterface(&com.IID_IUnknown)
	if err != nil {
		t.Fatal(err)
	}
	ok := ua == ub
	ua.Release()
	ub.Release()
	return ok
}

// readPixels copies the texture into a staging texture and returns its
// bytes and row pitch.
func readPixels(t *testing.T, dev d3d11.Device, ctx d3d11.DeviceContext, tex d3d11.Texture2D) ([]byte, int) {
	t.Helper()
	sd := tex.Desc()
	sd.Usage = d3d11.USAGE_STAGING
	sd.BindFlags = 0
	sd.CPUAccessFlags = d3d11.CPU_ACCESS_READ
	sd.MiscFlags = 0
	staging, err := dev.CreateTexture2D(&sd)
	if err != nil {
		t.Fatal(err)
	}
	defer release(t, "staging texture", &staging.Ptr, 0)
	ctx.CopyResource(staging.Ptr, tex.Ptr)
	m, err := ctx.Map(staging.Ptr, 0, d3d11.MAP_READ, 0)
	if err != nil {
		t.Fatal(err)
	}
	pix := ffi.CopyBytes(m.PData, int(m.RowPitch)*int(sd.Height-1)+int(sd.Width)*4)
	ctx.Unmap(staging.Ptr, 0)
	return pix, int(m.RowPitch)
}

// near reports whether a and b differ by at most 1. (In ints: in bytes,
// 0-255 wraps to 1, so black would match white.)
func near(a, b byte) bool { d := int(a) - int(b); return d >= -1 && d <= 1 }

// TestSmoke builds S2.4b's device, chain and composition tree on a hidden
// window, adds the D2D factory, device, context and a target bitmap over
// the back buffer, sets and reads the DPI (the float path), clears to known
// colors, reads them back, presents, commits and tears everything down with
// every count at 0.
func TestSmoke(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		dev, ctx, _, err := d3d11.CreateDevice()
		if err != nil {
			t.Fatal(err)
		}
		smoke(t, dev, ctx)
	})
	t.Run("WARP", func(t *testing.T) {
		dev, ctx, _, err := d3d11.CreateDeviceOfType(d3d11.DRIVER_TYPE_WARP, d3d11.CREATE_DEVICE_BGRA_SUPPORT)
		if err != nil {
			t.Fatal(err)
		}
		smoke(t, dev, ctx)
	})
}

func smoke(t *testing.T, dev d3d11.Device, ctx d3d11.DeviceContext) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const w, h = 64, 48
	hwnd, destroy := hiddenWindow(t, w, h)
	defer destroy()

	// S2.4b's pipeline.
	xdev, err := dxgi.DeviceFrom(dev.Ptr)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := xdev.Adapter()
	if err != nil {
		t.Fatal(err)
	}
	factory, err := adapter.Factory()
	if err != nil {
		t.Fatal(err)
	}
	const chainFlags = dxgi.SWAP_CHAIN_FLAG_FRAME_LATENCY_WAITABLE_OBJECT
	chain, err := factory.CreateSwapChainForComposition(dev.Ptr, &dxgi.SwapChainDesc1{
		Width: w, Height: h, Format: dxgi.FORMAT_B8G8R8A8_UNORM,
		SampleDesc:  dxgi.SampleDesc{Count: 1},
		BufferUsage: dxgi.USAGE_RENDER_TARGET_OUTPUT, BufferCount: 2,
		Scaling: dxgi.SCALING_STRETCH, SwapEffect: dxgi.SWAP_EFFECT_FLIP_SEQUENTIAL,
		AlphaMode: dxgi.ALPHA_MODE_PREMULTIPLIED, Flags: chainFlags,
	})
	if err != nil {
		t.Fatal(err)
	}
	cdev, err := dcomp.CreateDevice(xdev.Ptr)
	if err != nil {
		t.Fatal(err)
	}
	target, err := cdev.CreateTargetForHwnd(hwnd, false)
	if err != nil {
		t.Fatal(err)
	}
	visual, err := cdev.CreateVisual()
	if err != nil {
		t.Fatal(err)
	}
	if err := visual.SetContent(chain.Ptr); err != nil {
		t.Fatal(err)
	}
	if err := target.SetRoot(visual); err != nil {
		t.Fatal(err)
	}

	// D2D: factory, device, context.
	d2f, err := CreateFactory(DEBUG_LEVEL_NONE)
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "factory", d2f.Ptr, &IID_ID2D1Factory, &IID_ID2D1Factory1)
	d2dev, err := d2f.CreateDevice(xdev)
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "device", d2dev.Ptr, &IID_ID2D1Device, &IID_ID2D1Resource)
	// ID2D1Resource::GetFactory (slot 3) returns our factory.
	var gotFactory com.Ptr
	d2dev.Call(slotResourceGetFactory, uintptr(unsafe.Pointer(&gotFactory)))
	if !same(t, gotFactory, d2f.Ptr) {
		t.Errorf("device's GetFactory %#x is not the factory %#x", gotFactory, d2f.Ptr)
	}
	gotFactory.Release()
	dc, err := d2dev.CreateDeviceContext(DEVICE_CONTEXT_OPTIONS_NONE)
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "device context", dc.Ptr, &IID_ID2D1DeviceContext, &IID_ID2D1RenderTarget, &IID_ID2D1Resource)
	// ID2D1DeviceContext::GetDevice (slot 73) returns our device.
	var gotDevice com.Ptr
	dc.Call(slotDCGetDevice, uintptr(unsafe.Pointer(&gotDevice)))
	if !same(t, gotDevice, d2dev.Ptr) {
		t.Errorf("context's GetDevice %#x is not the device %#x", gotDevice, d2dev.Ptr)
	}
	gotDevice.Release()
	if x, y := dc.Dpi(); x != 96 || y != 96 {
		t.Errorf("fresh context DPI %v×%v, want 96×96", x, y)
	}
	if !dc.Target().IsNil() {
		t.Error("fresh context has a target")
	}

	// The target bitmap over the back buffer.
	surf, err := chain.Buffer(0, &dxgi.IID_IDXGISurface)
	if err != nil {
		t.Fatal(err)
	}
	props := BitmapProperties1{
		PixelFormat:   PixelFormat{Format: dxgi.FORMAT_B8G8R8A8_UNORM, AlphaMode: ALPHA_MODE_PREMULTIPLIED},
		DpiX:          144,
		DpiY:          144,
		BitmapOptions: BITMAP_OPTIONS_TARGET | BITMAP_OPTIONS_CANNOT_DRAW,
	}
	bmp, err := dc.CreateBitmapFromDxgiSurface(surf, &props)
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "bitmap", bmp.Ptr, &IID_ID2D1Bitmap1, &IID_ID2D1Bitmap, &IID_ID2D1Image, &IID_ID2D1Resource)
	if o := bmp.Options(); o != props.BitmapOptions {
		t.Errorf("bitmap options %#x, want %#x", o, props.BitmapOptions)
	}
	if x, y := bmp.Dpi(); x != 144 || y != 144 {
		t.Errorf("bitmap DPI %v×%v, want 144×144", x, y)
	}
	if s, err := bmp.Surface(); err != nil {
		t.Error(err)
	} else {
		if !same(t, s, surf) {
			t.Error("bitmap's surface is not the back buffer")
		}
		s.Release()
	}
	dc.SetTarget(bmp.Ptr)
	if tg := dc.Target(); !same(t, tg, bmp.Ptr) {
		t.Error("GetTarget is not the bitmap")
		tg.Release()
	} else {
		tg.Release()
	}

	// The float path: SetDpi(144, 144), then other values, read back.
	for _, d := range [][2]float32{{144, 144}, {120, 192}, {144, 144}} {
		dc.SetDpi(d[0], d[1])
		if x, y := dc.Dpi(); x != d[0] || y != d[1] {
			t.Errorf("SetDpi(%v, %v): GetDpi %v×%v", d[0], d[1], x, y)
		}
	}

	back := d3d11.Texture2D{}
	if tex, err := surf.QueryInterface(&d3d11.IID_ID3D11Texture2D); err != nil {
		t.Fatal(err)
	} else {
		back.Ptr = tex
	}
	check := func(want [4]byte) {
		t.Helper()
		pix, pitch := readPixels(t, dev, ctx, back)
		for _, pt := range [][2]int{{0, 0}, {w - 1, 0}, {w / 2, h / 2}, {0, h - 1}, {w - 1, h - 1}} {
			o := pt[1]*pitch + pt[0]*4
			got := [4]byte(pix[o : o+4])
			if !near(got[0], want[0]) || !near(got[1], want[1]) || !near(got[2], want[2]) || !near(got[3], want[3]) {
				t.Errorf("pixel %v = BGRA %v, want %v", pt, got, want)
			}
		}
	}
	// Opaque: R .6, G .4, B .2 as BGRA 51 102 153 255.
	dc.BeginDraw()
	dc.Clear(ColorF{R: 0.6, G: 0.4, B: 0.2, A: 1})
	if err := dc.EndDraw(); err != nil {
		t.Fatalf("EndDraw: %v", err)
	}
	check([4]byte{51, 102, 153, 255})
	// Half alpha is stored premultiplied: 0.3, 0.2, 0.1 at 0.5.
	dc.BeginDraw()
	dc.Clear(ColorF{R: 0.6, G: 0.4, B: 0.2, A: 0.5})
	if err := dc.EndDraw(); err != nil {
		t.Fatalf("EndDraw: %v", err)
	}
	check([4]byte{26, 51, 77, 128})
	// EndDraw without BeginDraw is D2DERR_WRONG_STATE, not a device loss.
	if err := dc.EndDraw(); err == nil || errors.Is(err, com.D2DERR_RECREATE_TARGET) {
		t.Errorf("unpaired EndDraw: %v", err)
	}
	if _, err := chain.Present(1, 0); err != nil {
		t.Errorf("Present: %v", err)
	}
	if err := cdev.Commit(); err != nil {
		t.Errorf("Commit: %v", err)
	}

	// Teardown. The bitmap and the context's target hold the back buffer
	// (finding: 3 references beyond ours while both are set), so the
	// surface's own count reaches 0 only after SetTarget(0) and the
	// bitmap's Release; then the chain is free to resize.
	if n := back.Release(); n == 0 {
		t.Error("the bitmap holds no reference to the back buffer")
	} else {
		t.Logf("back buffer count with the bitmap set as target: %d", n)
	}
	dc.SetTarget(0)
	if !dc.Target().IsNil() {
		t.Error("SetTarget(0) left a target")
	}
	release(t, "bitmap", &bmp.Ptr, 0)
	release(t, "back buffer surface", &surf, 0)
	if err := chain.ResizeBuffers(0, 2*w, 2*h, dxgi.FORMAT_UNKNOWN, chainFlags); err != nil {
		t.Errorf("ResizeBuffers after releasing the bitmap: %v", err)
	}
	release(t, "device context", &dc.Ptr, 0)
	release(t, "d2d device", &d2dev.Ptr, 0)
	release(t, "d2d factory", &d2f.Ptr, 0)

	if err := target.SetRoot(dcomp.Visual{}); err != nil {
		t.Error(err)
	}
	if err := visual.SetContent(0); err != nil {
		t.Error(err)
	}
	if err := cdev.Commit(); err != nil {
		t.Error(err)
	}
	release(t, "visual", &visual.Ptr, 0)
	release(t, "target", &target.Ptr, 0)
	release(t, "dcomp device", &cdev.Ptr, 0)
	release(t, "swap chain", &chain.Ptr, 0)
	release(t, "factory", &factory.Ptr, 1)
	release(t, "adapter", &adapter.Ptr, 1)
	xdev.Release()
	ctx.ClearState()
	ctx.Flush()
	ctx.Release()
	release(t, "device", &dev.Ptr, 0)
}
