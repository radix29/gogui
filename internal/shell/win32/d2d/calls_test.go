//go:build windows

package d2d

import (
	"runtime"
	"testing"

	"github.com/radix29/gogui/internal/shell/win32/d3d11"
	"github.com/radix29/gogui/internal/shell/win32/dwrite"
	"github.com/radix29/gogui/internal/shell/win32/dxgi"
)

// withBoundFloats runs f with the float-path methods on com.Bind (purego),
// the path windows/arm64 takes, so its results can be checked on amd64
// too (review P2).
func withBoundFloats(t *testing.T, f func(t *testing.T)) {
	t.Helper()
	old := directFloats
	directFloats = false
	defer func() { directFloats = old }()
	f(t)
}

// TestDrawBound runs TestDraw's, TestEffects' and TestSmoke's checks on the
// default device through the bound float path.
func TestDrawBound(t *testing.T) {
	if !directFloats {
		t.Skip("the bound path is the only path here; TestDraw covers it")
	}
	for name, check := range map[string]func(*testing.T, d3d11.Device, d3d11.DeviceContext){
		"draw": draw, "effects": effects, "smoke": smoke,
	} {
		t.Run(name, func(t *testing.T) {
			dev, ctx, _, err := d3d11.CreateDevice() // each check releases its device
			if err != nil {
				t.Fatal(err)
			}
			withBoundFloats(t, func(t *testing.T) { check(t, dev, ctx) })
		})
	}
}

// offscreen returns a device context drawing into a 64×64 texture of dev,
// the factory of its device, and the cleanup.
func offscreen(t *testing.T, dev d3d11.Device) (DeviceContext, Factory1, func()) {
	t.Helper()
	tex, err := dev.CreateTexture2D(&d3d11.Texture2DDesc{
		Width: 64, Height: 64, MipLevels: 1, ArraySize: 1, Format: dxgi.FORMAT_B8G8R8A8_UNORM,
		SampleDesc: dxgi.SampleDesc{Count: 1}, Usage: d3d11.USAGE_DEFAULT,
		BindFlags: d3d11.BIND_RENDER_TARGET | d3d11.BIND_SHADER_RESOURCE,
	})
	if err != nil {
		t.Fatal(err)
	}
	surf, err := tex.QueryInterface(&dxgi.IID_IDXGISurface)
	if err != nil {
		t.Fatal(err)
	}
	xdev, err := dxgi.DeviceFrom(dev.Ptr)
	if err != nil {
		t.Fatal(err)
	}
	fac, err := CreateFactory(DEBUG_LEVEL_NONE)
	if err != nil {
		t.Fatal(err)
	}
	d2dev, err := fac.CreateDevice(xdev)
	if err != nil {
		t.Fatal(err)
	}
	dc, err := d2dev.CreateDeviceContext(DEVICE_CONTEXT_OPTIONS_NONE)
	if err != nil {
		t.Fatal(err)
	}
	bmp, err := dc.CreateBitmapFromDxgiSurface(surf, &BitmapProperties1{
		PixelFormat: PixelFormat{Format: dxgi.FORMAT_B8G8R8A8_UNORM, AlphaMode: ALPHA_MODE_PREMULTIPLIED},
		DpiX:        96, DpiY: 96, BitmapOptions: BITMAP_OPTIONS_TARGET | BITMAP_OPTIONS_CANNOT_DRAW,
	})
	if err != nil {
		t.Fatal(err)
	}
	dc.SetTarget(bmp.Ptr)
	return dc, fac, func() {
		dc.SetTarget(0)
		bmp.Release()
		dc.Release()
		d2dev.Release()
		fac.Release()
		xdev.Release()
		surf.Release()
		tex.Release()
	}
}

// TestDrawAllocs: the calls the canvas makes per draw allocate nothing
// (review P2). Integer-only methods go straight to syscall.SyscallN, whose
// arguments don't escape; the float-path ones do the same on amd64 (FLOAT
// bits in the integer slots, which SyscallN mirrors into XMM0–3), and go
// through purego on arm64, which allocates, so they are checked on amd64
// only.
func TestDrawAllocs(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	dev, _, _, err := d3d11.CreateDeviceOfType(d3d11.DRIVER_TYPE_WARP, d3d11.CREATE_DEVICE_BGRA_SUPPORT)
	if err != nil {
		t.Fatal(err)
	}
	defer dev.Release()
	dc, fac, done := offscreen(t, dev)
	defer done()
	brush, err := dc.CreateSolidColorBrush(ColorF{1, 0, 0, 1}, DefaultBrushProperties())
	if err != nil {
		t.Fatal(err)
	}
	defer brush.Release()
	rr, err := fac.CreateRoundedRectangleGeometry(RoundedRect{RectF{4, 4, 40, 40}, 6, 6})
	if err != nil {
		t.Fatal(err)
	}
	defer rr.Release()
	dwf, err := dwrite.CreateFactory()
	if err != nil {
		t.Fatal(err)
	}
	defer dwf.Release()
	tf, err := dwf.CreateTextFormat("Consolas", dwrite.FontCollection{}, dwrite.FONT_WEIGHT_NORMAL,
		dwrite.FONT_STYLE_NORMAL, dwrite.FONT_STRETCH_NORMAL, 12, "en-us")
	if err != nil {
		t.Fatal(err)
	}
	defer tf.Release()
	tl, err := dwf.CreateTextLayout([]uint16{'a', 'b'}, tf, 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Release()
	img, err := dc.CreateBitmap(SizeU{Width: 2, Height: 2}, make([]byte, 16), 8, &BitmapProperties1{
		PixelFormat: PixelFormat{Format: dxgi.FORMAT_B8G8R8A8_UNORM, AlphaMode: ALPHA_MODE_PREMULTIPLIED},
		DpiX:        96, DpiY: 96,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer img.Release()

	b := brush.Brush()
	r := RectF{8, 8, 24, 24}
	m := Matrix3x2F{M11: 1, M22: 1, Dx: 2, Dy: 3}
	layer := LayerParameters1{ContentBounds: InfiniteRect(), MaskTransform: IdentityMatrix(), Opacity: 0.5}
	calls := []struct {
		name  string
		float bool
		f     func()
	}{
		{"SetTransform", false, func() { dc.SetTransform(m) }},
		{"FillRectangle", false, func() { dc.FillRectangle(r, b) }},
		{"FillRoundedRectangle", false, func() { dc.FillRoundedRectangle(RoundedRect{r, 3, 3}, b) }},
		{"FillGeometry", false, func() { dc.FillGeometry(rr.Geometry(), b) }},
		{"PushAxisAlignedClip+Pop", false, func() {
			dc.PushAxisAlignedClip(r, ANTIALIAS_MODE_ALIASED)
			dc.PopAxisAlignedClip()
		}},
		{"PushLayer+Pop", false, func() { dc.PushLayer(&layer); dc.PopLayer() }},
		{"SetColor", false, func() { brush.SetColor(ColorF{0, 1, 0, 1}) }},
		{"SetTextAntialiasMode", false, func() { dc.SetTextAntialiasMode(TEXT_ANTIALIAS_MODE_GRAYSCALE) }},
		{"DrawImage", false, func() {
			dc.DrawImage(img.Image(), nil, nil, INTERPOLATION_MODE_LINEAR, COMPOSITE_MODE_SOURCE_OVER)
		}},
		{"DrawRoundedRectangle", true, func() { dc.DrawRoundedRectangle(RoundedRect{r, 3, 3}, b, 2, StrokeStyle{}) }},
		{"DrawGeometry", true, func() { dc.DrawGeometry(rr.Geometry(), b, 2, StrokeStyle{}) }},
		{"DrawTextLayout", true, func() { dc.DrawTextLayout(Point2F{4, 4}, tl.Ptr, b, DRAW_TEXT_OPTIONS_NONE) }},
		{"DrawBitmap", true, func() { dc.DrawBitmap(img, &r, 0.5, INTERPOLATION_MODE_LINEAR, nil) }},
	}
	dc.BeginDraw()
	for _, c := range calls {
		if c.float && !directFloats {
			continue
		}
		c.f() // binds and warms up
		if n := testing.AllocsPerRun(50, c.f); n != 0 {
			t.Errorf("%s: %v allocations per call, want 0", c.name, n)
		}
		if err := dc.Flush(); err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if err := dc.EndDraw(); err != nil {
		t.Fatalf("EndDraw: %v", err)
	}

	// The sink's figure calls, on one open geometry.
	g, err := fac.CreatePathGeometry()
	if err != nil {
		t.Fatal(err)
	}
	defer g.Release()
	sink, err := g.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Release()
	sink.SetFillMode(FILL_MODE_WINDING)
	figure := func() {
		sink.BeginFigure(Point2F{1, 1}, FIGURE_BEGIN_FILLED)
		sink.AddLine(Point2F{9, 1})
		sink.AddBezier(BezierSegment{Point2F{9, 5}, Point2F{5, 9}, Point2F{1, 9}})
		sink.AddQuadraticBezier(QuadraticBezierSegment{Point2F{0, 5}, Point2F{1, 1}})
		sink.EndFigure(FIGURE_END_CLOSED)
	}
	figure()
	if n := testing.AllocsPerRun(50, figure); n != 0 && directFloats {
		t.Errorf("a figure: %v allocations, want 0", n)
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}
	if n, err := g.FigureCount(); err != nil || n != 52 {
		t.Errorf("FigureCount = %d, %v; want 52", n, err)
	}
}
