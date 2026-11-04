//go:build windows

package d2d

import (
	"runtime"
	"testing"
	"unsafe"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/win32/com"
	"github.com/radix29/gogui/internal/shell/win32/d3d11"
	"github.com/radix29/gogui/internal/shell/win32/dwrite"
	"github.com/radix29/gogui/internal/shell/win32/dxgi"
)

// The drawing interfaces' full vtables in d2d1.h / d2d1_1.h order.
var (
	vtBrush       = vtResource.Extend("SetOpacity", "SetTransform", "GetOpacity", "GetTransform")
	vtSolidBrush  = vtBrush.Extend("SetColor", "GetColor")
	vtLinearBrush = vtBrush.Extend("SetStartPoint", "SetEndPoint", "GetStartPoint", "GetEndPoint",
		"GetGradientStopCollection")
	vtStops = vtResource.Extend("GetGradientStopCount", "GetGradientStops", "GetColorInterpolationGamma",
		"GetExtendMode")
	vtGeometry = vtResource.Extend("GetBounds", "GetWidenedBounds", "StrokeContainsPoint",
		"FillContainsPoint", "CompareWithGeometry", "Simplify", "Tessellate", "CombineWithGeometry",
		"Outline", "ComputeArea", "ComputeLength", "ComputePointAtLength", "Widen")
	vtRoundedGeometry = vtGeometry.Extend("GetRoundedRect")
	vtPathGeometry    = vtGeometry.Extend("Open", "Stream", "GetSegmentCount", "GetFigureCount")
	vtSimplifiedSink  = com.IUnknownVtable.Extend("SetFillMode", "SetSegmentFlags", "BeginFigure",
		"AddLines", "AddBeziers", "EndFigure", "Close")
	vtSink = vtSimplifiedSink.Extend("AddLine", "AddBezier", "AddQuadraticBezier",
		"AddQuadraticBeziers", "AddArc")
	vtStrokeStyle = vtResource.Extend("GetStartCap", "GetEndCap", "GetDashCap", "GetMiterLimit",
		"GetLineJoin", "GetDashOffset", "GetDashStyle", "GetDashesCount", "GetDashes")
	vtStrokeStyle1 = vtStrokeStyle.Extend("GetStrokeTransformType")
)

func TestDrawSlots(t *testing.T) {
	if len(vtSink) != 15 || len(vtPathGeometry) != 21 || len(vtStrokeStyle1) != 14 || len(vtLinearBrush) != 13 {
		t.Errorf("vtable lengths %d, %d, %d, %d; want 15, 21, 14, 13",
			len(vtSink), len(vtPathGeometry), len(vtStrokeStyle1), len(vtLinearBrush))
	}
	com.CheckSlots(t, "ID2D1Factory1", vtFactory1, map[string]int{
		"CreateRoundedRectangleGeometry": slotFactoryCreateRoundedRectangleGeometry,
		"CreatePathGeometry":             slotFactoryCreatePathGeometry,
		"CreateStrokeStyle1":             slotFactory1CreateStrokeStyle,
	})
	com.CheckSlots(t, "ID2D1DeviceContext", vtDeviceContext, map[string]int{
		"CreateSolidColorBrush":        slotRTCreateSolidColorBrush,
		"CreateGradientStopCollection": slotRTCreateGradientStopCollection,
		"CreateLinearGradientBrush":    slotRTCreateLinearGradientBrush,
		"FillRectangle":                slotRTFillRectangle,
		"DrawRoundedRectangle":         slotRTDrawRoundedRectangle,
		"FillRoundedRectangle":         slotRTFillRoundedRectangle,
		"DrawGeometry":                 slotRTDrawGeometry,
		"FillGeometry":                 slotRTFillGeometry,
		"DrawTextLayout":               slotRTDrawTextLayout,
		"SetTextAntialiasMode":         slotRTSetTextAntialiasMode,
		"GetTextAntialiasMode":         slotRTGetTextAntialiasMode,
		"SetTransform":                 slotRTSetTransform,
		"GetTransform":                 slotRTGetTransform,
		"PopLayer":                     slotRTPopLayer,
		"PushAxisAlignedClip":          slotRTPushAxisAlignedClip,
		"PopAxisAlignedClip":           slotRTPopAxisAlignedClip,
		"PushLayer1":                   slotDCPushLayer,
	})
	com.CheckSlots(t, "ID2D1SolidColorBrush", vtSolidBrush, map[string]int{"SetColor": slotSolidSetColor})
	com.CheckSlots(t, "ID2D1LinearGradientBrush", vtLinearBrush, map[string]int{
		"GetGradientStopCollection": slotLinearGetGradientStopCollection,
	})
	com.CheckSlots(t, "ID2D1GradientStopCollection", vtStops, map[string]int{
		"GetGradientStopCount":       slotStopsGetGradientStopCount,
		"GetColorInterpolationGamma": slotStopsGetColorInterpolationGamma,
		"GetExtendMode":              slotStopsGetExtendMode,
	})
	com.CheckSlots(t, "ID2D1RoundedRectangleGeometry", vtRoundedGeometry, nil)
	com.CheckSlots(t, "ID2D1PathGeometry", vtPathGeometry, map[string]int{
		"Open":            slotPathOpen,
		"GetSegmentCount": slotPathGetSegmentCount,
		"GetFigureCount":  slotPathGetFigureCount,
	})
	com.CheckSlots(t, "ID2D1GeometrySink", vtSink, map[string]int{
		"SetFillMode":        slotSinkSetFillMode,
		"BeginFigure":        slotSinkBeginFigure,
		"EndFigure":          slotSinkEndFigure,
		"Close":              slotSinkClose,
		"AddLine":            slotSinkAddLine,
		"AddBezier":          slotSinkAddBezier,
		"AddQuadraticBezier": slotSinkAddQuadraticBezier,
	})
	com.CheckSlots(t, "ID2D1StrokeStyle1", vtStrokeStyle1, map[string]int{
		"GetStartCap":            slotStrokeGetStartCap,
		"GetEndCap":              slotStrokeGetEndCap,
		"GetDashCap":             slotStrokeGetDashCap,
		"GetLineJoin":            slotStrokeGetLineJoin,
		"GetDashStyle":           slotStrokeGetDashStyle,
		"GetDashesCount":         slotStrokeGetDashesCount,
		"GetStrokeTransformType": slotStroke1GetTransformType,
	})
}

func TestDrawStructLayouts(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Fatal("the layout tables are for 64-bit Windows only")
	}
	pt := func(name string, off uintptr) ffi.Field { return f(name, off, 8) }
	for _, c := range []struct {
		v    any
		want ffi.Layout
	}{
		{Point2F{}, ffi.Layout{Size: 8, Align: 4, Fields: []ffi.Field{f("X", 0, 4), f("Y", 4, 4)}}},
		{RectF{}, ffi.Layout{Size: 16, Align: 4, Fields: []ffi.Field{
			f("Left", 0, 4), f("Top", 4, 4), f("Right", 8, 4), f("Bottom", 12, 4)}}},
		{Matrix3x2F{}, ffi.Layout{Size: 24, Align: 4, Fields: []ffi.Field{
			f("M11", 0, 4), f("M12", 4, 4), f("M21", 8, 4), f("M22", 12, 4), f("Dx", 16, 4), f("Dy", 20, 4)}}},
		{RoundedRect{}, ffi.Layout{Size: 24, Align: 4, Fields: []ffi.Field{
			f("Rect", 0, 16), f("RadiusX", 16, 4), f("RadiusY", 20, 4)}}},
		{BezierSegment{}, ffi.Layout{Size: 24, Align: 4, Fields: []ffi.Field{
			pt("Point1", 0), pt("Point2", 8), pt("Point3", 16)}}},
		{QuadraticBezierSegment{}, ffi.Layout{Size: 16, Align: 4, Fields: []ffi.Field{
			pt("Point1", 0), pt("Point2", 8)}}},
		{BrushProperties{}, ffi.Layout{Size: 28, Align: 4, Fields: []ffi.Field{
			f("Opacity", 0, 4), f("Transform", 4, 24)}}},
		{GradientStop{}, ffi.Layout{Size: 20, Align: 4, Fields: []ffi.Field{
			f("Position", 0, 4), f("Color", 4, 16)}}},
		{LinearGradientBrushProperties{}, ffi.Layout{Size: 16, Align: 4, Fields: []ffi.Field{
			pt("StartPoint", 0), pt("EndPoint", 8)}}},
		{StrokeStyleProperties1{}, ffi.Layout{Size: 32, Align: 4, Fields: []ffi.Field{
			f("StartCap", 0, 4), f("EndCap", 4, 4), f("DashCap", 8, 4), f("LineJoin", 12, 4),
			f("MiterLimit", 16, 4), f("DashStyle", 20, 4), f("DashOffset", 24, 4), f("TransformType", 28, 4)}}},
		{LayerParameters1{}, ffi.Layout{Size: 72, Align: 8, Fields: []ffi.Field{
			f("ContentBounds", 0, 16), f("GeometricMask", 16, 8), f("MaskAntialiasMode", 24, 4),
			f("MaskTransform", 28, 24), f("Opacity", 52, 4), f("OpacityBrush", 56, 8), f("LayerOptions", 64, 4)}}},
	} {
		ffi.CheckLayout(t, c.v, c.want)
		ffi.CheckNoGoPointers(t, c.v)
	}
}

// TestDraw runs the drawing bindings on an offscreen target on the default
// device and on WARP.
func TestDraw(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		dev, ctx, _, err := d3d11.CreateDevice()
		if err != nil {
			t.Fatal(err)
		}
		draw(t, dev, ctx)
	})
	t.Run("WARP", func(t *testing.T) {
		dev, ctx, _, err := d3d11.CreateDeviceOfType(d3d11.DRIVER_TYPE_WARP, d3d11.CREATE_DEVICE_BGRA_SUPPORT)
		if err != nil {
			t.Fatal(err)
		}
		draw(t, dev, ctx)
	})
}

var (
	white = ColorF{1, 1, 1, 1}
	bgraW = [4]byte{255, 255, 255, 255}
	bgraR = [4]byte{0, 0, 255, 255}
	bgraG = [4]byte{0, 255, 0, 255}
	bgraB = [4]byte{255, 0, 0, 255}
	bgraK = [4]byte{0, 0, 0, 255}
)

// draw renders one frame per primitive on a 64×64 render-target texture
// (through CreateBitmapFromDxgiSurface, as the shell does with a back
// buffer), reads each frame back through staging and checks pixels away
// from the antialiased edges. At the end every count is 0.
func draw(t *testing.T, dev d3d11.Device, ctx d3d11.DeviceContext) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const n = 64

	tex, err := dev.CreateTexture2D(&d3d11.Texture2DDesc{
		Width: n, Height: n, MipLevels: 1, ArraySize: 1, Format: dxgi.FORMAT_B8G8R8A8_UNORM,
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

	var pix []byte
	var pitch int
	frame := func(name string, body func()) {
		t.Helper()
		dc.BeginDraw()
		dc.Clear(white)
		body()
		if err := dc.EndDraw(); err != nil {
			t.Fatalf("%s: EndDraw: %v", name, err)
		}
		pix, pitch = readPixels(t, dev, ctx, d3d11.Texture2D{Ptr: tex.Ptr})
	}
	expect := func(name string, want [4]byte, pts ...[2]int) {
		t.Helper()
		for _, p := range pts {
			o := p[1]*pitch + p[0]*4
			got := [4]byte(pix[o : o+4])
			if !near(got[0], want[0]) || !near(got[1], want[1]) || !near(got[2], want[2]) || !near(got[3], want[3]) {
				t.Errorf("%s: pixel %v = BGRA %v, want %v", name, p, got, want)
			}
		}
	}

	// Brushes.
	solid := func(c ColorF) SolidColorBrush {
		b, err := dc.CreateSolidColorBrush(c, DefaultBrushProperties())
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	red, green, blue, black := solid(ColorF{1, 0, 0, 1}), solid(ColorF{0, 1, 0, 1}), solid(ColorF{0, 0, 1, 1}), solid(ColorF{0, 0, 0, 1})
	queries(t, "solid brush", red.Ptr, &IID_ID2D1SolidColorBrush, &IID_ID2D1Brush, &IID_ID2D1Resource)

	// FillRectangle, and SetColor on a live brush.
	frame("FillRectangle", func() { dc.FillRectangle(RectF{8, 8, 24, 24}, red.Brush()) })
	expect("FillRectangle", bgraR, [2]int{9, 9}, [2]int{16, 16}, [2]int{22, 22})
	expect("FillRectangle", bgraW, [2]int{4, 4}, [2]int{25, 16}, [2]int{16, 25})
	frame("SetColor", func() {
		red.SetColor(ColorF{0, 0, 0, 1})
		dc.FillRectangle(RectF{8, 8, 24, 24}, red.Brush())
		red.SetColor(ColorF{1, 0, 0, 1})
	})
	expect("SetColor", bgraK, [2]int{16, 16})

	// FillRoundedRectangle: the corner pixel (1,1) lies outside a 16 radius.
	frame("FillRoundedRectangle", func() {
		dc.FillRoundedRectangle(RoundedRect{RectF{0, 0, n, n}, 16, 16}, blue.Brush())
	})
	expect("FillRoundedRectangle", bgraB, [2]int{32, 32}, [2]int{1, 32}, [2]int{32, 1})
	expect("FillRoundedRectangle", bgraW, [2]int{1, 1}, [2]int{62, 62})

	// DrawRoundedRectangle (float path): a 4-wide stroke centred on x = 8
	// covers 6…10.
	frame("DrawRoundedRectangle", func() {
		dc.DrawRoundedRectangle(RoundedRect{RectF{8, 8, 56, 56}, 4, 4}, black.Brush(), 4, StrokeStyle{})
	})
	expect("DrawRoundedRectangle", bgraK, [2]int{7, 32}, [2]int{8, 32}, [2]int{32, 55}, [2]int{55, 32})
	expect("DrawRoundedRectangle", bgraW, [2]int{4, 32}, [2]int{12, 32}, [2]int{32, 32})

	// Transforms: a translation moves the rect; GetTransform reads it back.
	moved := Matrix3x2F{M11: 1, M22: 1, Dx: 32, Dy: 16}
	frame("SetTransform", func() {
		dc.SetTransform(moved)
		if m := dc.Transform(); m != moved {
			t.Errorf("Transform() = %+v, want %+v", m, moved)
		}
		dc.FillRectangle(RectF{0, 0, 8, 8}, red.Brush())
		dc.SetTransform(IdentityMatrix())
	})
	expect("SetTransform", bgraR, [2]int{36, 20})
	expect("SetTransform", bgraW, [2]int{4, 4}, [2]int{36, 4})

	// Linear gradient black → white across x, gamma 2.2, clamp.
	stops, err := dc.CreateGradientStopCollection([]GradientStop{{0, ColorF{0, 0, 0, 1}}, {1, white}}, GAMMA_2_2, EXTEND_MODE_CLAMP)
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "stops", stops.Ptr, &IID_ID2D1GradientStopCollection)
	if c, g, e := stops.Count(), stops.Gamma(), stops.ExtendMode(); c != 2 || g != GAMMA_2_2 || e != EXTEND_MODE_CLAMP {
		t.Errorf("stops: count %d, gamma %d, extend %d", c, g, e)
	}
	grad, err := dc.CreateLinearGradientBrush(LinearGradientBrushProperties{Point2F{8, 0}, Point2F{56, 0}},
		DefaultBrushProperties(), stops)
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "gradient brush", grad.Ptr, &IID_ID2D1LinearGradientBrush, &IID_ID2D1Brush)
	if s := grad.Stops(); !same(t, s.Ptr, stops.Ptr) {
		t.Error("gradient brush's stops are not the collection")
		s.Release()
	} else {
		s.Release()
	}
	frame("LinearGradient", func() { dc.FillRectangle(RectF{0, 0, n, n}, grad.Brush()) })
	expect("LinearGradient", bgraK, [2]int{0, 32}, [2]int{4, 32})   // clamped before the start
	expect("LinearGradient", bgraW, [2]int{59, 32}, [2]int{63, 32}) // clamped after the end
	mid := pix[32*pitch+32*4]
	if o := 32*pitch + 16*4; !(pix[o] < mid && mid > 112 && mid < 144) || pix[32*pitch+48*4] <= mid {
		t.Errorf("gradient not monotonic around the middle: %d, %d, %d", pix[o], mid, pix[32*pitch+48*4])
	}

	// A path with every segment kind: a 16…48 square drawn as a line, a
	// degenerate cubic, a degenerate quadratic and the closing line.
	path, err := fac.CreatePathGeometry()
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "path geometry", path.Ptr, &IID_ID2D1PathGeometry, &IID_ID2D1Geometry, &IID_ID2D1Resource)
	sink, err := path.Open()
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "sink", sink.Ptr, &IID_ID2D1GeometrySink, &IID_ID2D1SimplifiedGeometrySink)
	sink.SetFillMode(FILL_MODE_WINDING)
	sink.BeginFigure(Point2F{16, 16}, FIGURE_BEGIN_FILLED)
	sink.AddLine(Point2F{48, 16})
	sink.AddBezier(BezierSegment{Point2F{48, 24}, Point2F{48, 40}, Point2F{48, 48}})
	sink.AddQuadraticBezier(QuadraticBezierSegment{Point2F{32, 48}, Point2F{16, 48}})
	sink.EndFigure(FIGURE_END_CLOSED)
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}
	release(t, "sink", &sink.Ptr, 0)
	if fc, err := path.FigureCount(); err != nil || fc != 1 {
		t.Errorf("FigureCount = %d, %v; want 1", fc, err)
	}
	if sc, err := path.SegmentCount(); err != nil || sc != 4 {
		t.Errorf("SegmentCount = %d, %v; want 4 (closing line included)", sc, err)
	}
	frame("FillGeometry", func() { dc.FillGeometry(path.Geometry(), green.Brush()) })
	expect("FillGeometry", bgraG, [2]int{17, 17}, [2]int{32, 32}, [2]int{46, 46})
	expect("FillGeometry", bgraW, [2]int{14, 32}, [2]int{50, 32}, [2]int{32, 50})

	// An open line stroked 4 wide with custom dashes {2, 2}: in units of
	// the width, so 8 on, 8 off, flat caps.
	line, err := fac.CreatePathGeometry()
	if err != nil {
		t.Fatal(err)
	}
	if sink, err = line.Open(); err != nil {
		t.Fatal(err)
	}
	sink.BeginFigure(Point2F{0, 32}, FIGURE_BEGIN_HOLLOW)
	sink.AddLine(Point2F{n, 32})
	sink.EndFigure(FIGURE_END_OPEN)
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}
	release(t, "line sink", &sink.Ptr, 0)
	dashed, err := fac.CreateStrokeStyle(StrokeStyleProperties1{
		StartCap: CAP_STYLE_FLAT, EndCap: CAP_STYLE_FLAT, DashCap: CAP_STYLE_FLAT, LineJoin: LINE_JOIN_MITER_OR_BEVEL,
		MiterLimit: 10, DashStyle: DASH_STYLE_CUSTOM,
	}, []float32{2, 2})
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "stroke style", dashed.Ptr, &IID_ID2D1StrokeStyle1, &IID_ID2D1StrokeStyle)
	if s, e, d := dashed.Caps(); s != CAP_STYLE_FLAT || e != CAP_STYLE_FLAT || d != CAP_STYLE_FLAT {
		t.Errorf("caps %d %d %d", s, e, d)
	}
	if j, ds, c, tt := dashed.LineJoin(), dashed.DashStyle(), dashed.DashesCount(), dashed.TransformType(); j != LINE_JOIN_MITER_OR_BEVEL ||
		ds != DASH_STYLE_CUSTOM || c != 2 || tt != STROKE_TRANSFORM_TYPE_NORMAL {
		t.Errorf("join %d, dash style %d, dashes %d, transform type %d", j, ds, c, tt)
	}
	round, err := fac.CreateStrokeStyle(StrokeStyleProperties1{
		StartCap: CAP_STYLE_ROUND, EndCap: CAP_STYLE_SQUARE, DashCap: CAP_STYLE_TRIANGLE, LineJoin: LINE_JOIN_ROUND,
		MiterLimit: 10, DashStyle: DASH_STYLE_DOT, TransformType: STROKE_TRANSFORM_TYPE_HAIRLINE,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s, e, d := round.Caps(); s != CAP_STYLE_ROUND || e != CAP_STYLE_SQUARE || d != CAP_STYLE_TRIANGLE ||
		round.LineJoin() != LINE_JOIN_ROUND || round.DashStyle() != DASH_STYLE_DOT ||
		round.TransformType() != STROKE_TRANSFORM_TYPE_HAIRLINE {
		t.Errorf("second style: caps %d %d %d, join %d, dash style %d, transform %d",
			s, e, d, round.LineJoin(), round.DashStyle(), round.TransformType())
	}
	release(t, "round stroke style", &round.Ptr, 0)
	frame("DrawGeometry", func() { dc.DrawGeometry(line.Geometry(), black.Brush(), 4, dashed) })
	expect("DrawGeometry", bgraK, [2]int{2, 31}, [2]int{4, 32}, [2]int{20, 33}, [2]int{36, 30}, [2]int{52, 32})
	expect("DrawGeometry", bgraW, [2]int{12, 32}, [2]int{28, 32}, [2]int{44, 32}, [2]int{4, 28}, [2]int{4, 35})

	// Clip and layer, nested: an axis-aligned clip to x < 32, inside it a
	// layer masked by a circle (a rounded rect with radius = half side).
	circle, err := fac.CreateRoundedRectangleGeometry(RoundedRect{RectF{16, 16, 48, 48}, 16, 16})
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "rounded geometry", circle.Ptr, &IID_ID2D1RoundedRectangleGeometry, &IID_ID2D1Geometry)
	frame("Clip", func() {
		dc.PushAxisAlignedClip(RectF{0, 0, 32, n}, ANTIALIAS_MODE_ALIASED)
		dc.FillRectangle(RectF{0, 0, n, 8}, red.Brush())
		dc.PushLayer(&LayerParameters1{
			ContentBounds: InfiniteRect(), GeometricMask: circle.Ptr,
			MaskAntialiasMode: ANTIALIAS_MODE_PER_PRIMITIVE, MaskTransform: IdentityMatrix(), Opacity: 1,
		})
		dc.FillRectangle(RectF{0, 0, n, n}, green.Brush())
		dc.PopLayer()
		dc.PopAxisAlignedClip()
		dc.FillRectangle(RectF{0, 56, n, n}, blue.Brush()) // after the pops: unclipped
	})
	expect("Clip", bgraR, [2]int{4, 4}, [2]int{30, 4})
	expect("Clip", bgraG, [2]int{24, 32}, [2]int{31, 20}, [2]int{30, 44})
	expect("Clip", bgraW, [2]int{34, 4}, [2]int{40, 32}, [2]int{17, 17}, [2]int{8, 32}, [2]int{4, 50})
	expect("Clip", bgraB, [2]int{4, 60}, [2]int{60, 60})

	// The layer's mask transform and a layer under a world transform: the
	// circle moved right by 16 lands at 32…64.
	frame("LayerTransform", func() {
		dc.SetTransform(Matrix3x2F{M11: 1, M22: 1, Dx: 16})
		dc.PushLayer(&LayerParameters1{
			ContentBounds: InfiniteRect(), GeometricMask: circle.Ptr,
			MaskAntialiasMode: ANTIALIAS_MODE_PER_PRIMITIVE, MaskTransform: IdentityMatrix(), Opacity: 1,
		})
		dc.FillRectangle(RectF{-16, 0, n, n}, green.Brush())
		dc.PopLayer()
		dc.SetTransform(IdentityMatrix())
	})
	expect("LayerTransform", bgraG, [2]int{48, 32})
	expect("LayerTransform", bgraW, [2]int{24, 32}, [2]int{34, 18})

	// SetTextAntialiasMode / DrawTextLayout (float path): a grayscale
	// "█" (U+2588, a full block) at (16, 8) inks around (16…, 8…) only.
	if m := dc.TextAntialiasMode(); m != TEXT_ANTIALIAS_MODE_DEFAULT {
		t.Errorf("initial text antialias mode %d", m)
	}
	dc.SetTextAntialiasMode(TEXT_ANTIALIAS_MODE_GRAYSCALE)
	if m := dc.TextAntialiasMode(); m != TEXT_ANTIALIAS_MODE_GRAYSCALE {
		t.Errorf("text antialias mode %d, want grayscale", m)
	}
	dwf, err := dwrite.CreateFactory()
	if err != nil {
		t.Fatal(err)
	}
	tf, err := dwf.CreateTextFormat("Consolas", dwrite.FontCollection{}, dwrite.FONT_WEIGHT_NORMAL,
		dwrite.FONT_STYLE_NORMAL, dwrite.FONT_STRETCH_NORMAL, 24, "en-us")
	if err != nil {
		t.Fatal(err)
	}
	tl, err := dwf.CreateTextLayout([]uint16{0x2588}, tf, 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	frame("DrawTextLayout", func() {
		dc.DrawTextLayout(Point2F{16, 8}, tl.Ptr, black.Brush(), DRAW_TEXT_OPTIONS_NONE)
	})
	ink := 0
	for y := range n {
		for x := range n {
			o := y*pitch + x*4
			if pix[o] < 128 {
				ink++
				if x < 16 || y < 8 {
					t.Errorf("DrawTextLayout: ink at (%d, %d), left of or above the origin", x, y)
				}
			}
		}
	}
	if ink < 50 {
		t.Errorf("DrawTextLayout: %d dark pixels, want a block", ink)
	}
	tl.Release()
	tf.Release()
	dwf.Release()

	// Teardown: resources first, then the target, the bitmap and the
	// device objects; everything at 0.
	release(t, "circle", &circle.Ptr, 0)
	release(t, "dashed stroke style", &dashed.Ptr, 0)
	release(t, "line", &line.Ptr, 0)
	release(t, "path", &path.Ptr, 0)
	release(t, "gradient brush", &grad.Ptr, 0)
	release(t, "stops", &stops.Ptr, 0)
	for _, b := range []*SolidColorBrush{&red, &green, &blue, &black} {
		release(t, "solid brush", &b.Ptr, 0)
	}
	dc.SetTarget(0)
	release(t, "bitmap", &bmp.Ptr, 0)
	release(t, "device context", &dc.Ptr, 0)
	release(t, "d2d device", &d2dev.Ptr, 0)
	release(t, "d2d factory", &fac.Ptr, 0)
	release(t, "surface", &surf, 1)
	release(t, "texture", &tex.Ptr, 0)
	xdev.Release()
	ctx.ClearState()
	ctx.Flush()
	ctx.Release()
	release(t, "device", &dev.Ptr, 0)
}
