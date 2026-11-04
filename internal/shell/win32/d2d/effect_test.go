//go:build windows

package d2d

import (
	"runtime"
	"testing"
	"unsafe"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/win32/com"
	"github.com/radix29/gogui/internal/shell/win32/d3d11"
	"github.com/radix29/gogui/internal/shell/win32/dxgi"
)

// The effect interfaces' full vtables in d2d1_1.h order.
var (
	vtProperties = com.IUnknownVtable.Extend("GetPropertyCount", "GetPropertyName",
		"GetPropertyNameLength", "GetType", "GetPropertyIndex", "SetValueByName", "SetValue",
		"GetValueByName", "GetValue", "GetValueSize", "GetSubProperties")
	vtEffect = vtProperties.Extend("SetInput", "SetInputCount", "GetInput", "GetInputCount", "GetOutput")
)

func TestEffectSlots(t *testing.T) {
	if len(vtProperties) != 14 || len(vtEffect) != 19 {
		t.Errorf("vtable lengths %d, %d; want 14, 19", len(vtProperties), len(vtEffect))
	}
	com.CheckSlots(t, "ID2D1DeviceContext", vtDeviceContext, map[string]int{
		"Flush":         slotRTFlush,
		"CreateBitmap1": slotDCCreateBitmap,
		"CreateEffect":  slotDCCreateEffect,
		"DrawImage":     slotDCDrawImage,
		"DrawBitmap1":   slotDCDrawBitmap,
	})
	com.CheckSlots(t, "ID2D1Bitmap1", vtBitmap1, map[string]int{"CopyFromBitmap": slotBitmapCopyFromBitmap})
	com.CheckSlots(t, "ID2D1Effect", vtEffect, map[string]int{
		"GetPropertyCount": slotPropsGetPropertyCount,
		"GetType":          slotPropsGetType,
		"SetValue":         slotPropsSetValue,
		"GetValue":         slotPropsGetValue,
		"SetInput":         slotEffectSetInput,
		"SetInputCount":    slotEffectSetInputCount,
		"GetInputCount":    slotEffectGetInputCount,
		"GetOutput":        slotEffectGetOutput,
	})
}

func TestEffectStructLayouts(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Fatal("the layout tables are for 64-bit Windows only")
	}
	u4 := func(names ...string) []ffi.Field {
		var fs []ffi.Field
		for i, n := range names {
			fs = append(fs, f(n, uintptr(4*i), 4))
		}
		return fs
	}
	for _, c := range []struct {
		v    any
		want ffi.Layout
	}{
		{SizeU{}, ffi.Layout{Size: 8, Align: 4, Fields: u4("Width", "Height")}},
		{Point2U{}, ffi.Layout{Size: 8, Align: 4, Fields: u4("X", "Y")}},
		{RectU{}, ffi.Layout{Size: 16, Align: 4, Fields: u4("Left", "Top", "Right", "Bottom")}},
		{Vector4F{}, ffi.Layout{Size: 16, Align: 4, Fields: u4("X", "Y", "Z", "W")}},
	} {
		ffi.CheckLayout(t, c.v, c.want)
		ffi.CheckNoGoPointers(t, c.v)
	}
	if p := (SizeU{Width: 3, Height: 5}).packed(); p != 5<<32|3 {
		t.Errorf("SizeU{3, 5} packed as %#x", p)
	}
}

// TestEffects runs the effect and bitmap bindings on the S2.4d_1
// offscreen target on the default device and on WARP.
func TestEffects(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		dev, ctx, _, err := d3d11.CreateDevice()
		if err != nil {
			t.Fatal(err)
		}
		effects(t, dev, ctx)
	})
	t.Run("WARP", func(t *testing.T) {
		dev, ctx, _, err := d3d11.CreateDeviceOfType(d3d11.DRIVER_TYPE_WARP, d3d11.CREATE_DEVICE_BGRA_SUPPORT)
		if err != nil {
			t.Fatal(err)
		}
		effects(t, dev, ctx)
	})
}

// pixelBGRA builds premultiplied B8G8R8A8 pixels of w×h from fn(x, y).
// firstErrT returns the first non-nil error.
func firstErrT(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func pixelBGRA(w, h int, fn func(x, y int) [4]byte) []byte {
	pix := make([]byte, 0, w*h*4)
	for y := range h {
		for x := range w {
			c := fn(x, y)
			pix = append(pix, c[:]...)
		}
	}
	return pix
}

func props96(opts BitmapOptions) *BitmapProperties1 {
	return &BitmapProperties1{
		PixelFormat: PixelFormat{Format: dxgi.FORMAT_B8G8R8A8_UNORM, AlphaMode: ALPHA_MODE_PREMULTIPLIED},
		DpiX:        96, DpiY: 96, BitmapOptions: opts,
	}
}

func effects(t *testing.T, dev d3d11.Device, ctx d3d11.DeviceContext) {
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
	target, err := dc.CreateBitmapFromDxgiSurface(surf, props96(BITMAP_OPTIONS_TARGET|BITMAP_OPTIONS_CANNOT_DRAW))
	if err != nil {
		t.Fatal(err)
	}
	dc.SetTarget(target.Ptr)

	var pix []byte
	var pitch int
	frame := func(name string, bg ColorF, body func()) {
		t.Helper()
		dc.BeginDraw()
		dc.Clear(bg)
		body()
		if err := dc.EndDraw(); err != nil {
			t.Fatalf("%s: EndDraw: %v", name, err)
		}
		pix, pitch = readPixels(t, dev, ctx, d3d11.Texture2D{Ptr: tex.Ptr})
	}
	at := func(x, y int) [4]byte { o := y*pitch + x*4; return [4]byte(pix[o : o+4]) }
	expect := func(name string, want [4]byte, pts ...[2]int) {
		t.Helper()
		for _, p := range pts {
			got := at(p[0], p[1])
			if !near(got[0], want[0]) || !near(got[1], want[1]) || !near(got[2], want[2]) || !near(got[3], want[3]) {
				t.Errorf("%s: pixel %v = BGRA %v, want %v", name, p, got, want)
			}
		}
	}

	// A 2×2 bitmap from CPU pixels: red, green / blue, black.
	quad := [4][4]byte{bgraR, bgraG, bgraB, bgraK}
	if _, err := dc.CreateBitmap(SizeU{2, 2}, make([]byte, 15), 8, props96(BITMAP_OPTIONS_NONE)); err == nil {
		t.Error("CreateBitmap accepted 15 bytes for 2 rows of 8")
	}
	checker, err := dc.CreateBitmap(SizeU{2, 2}, pixelBGRA(2, 2, func(x, y int) [4]byte { return quad[2*y+x] }), 8,
		props96(BITMAP_OPTIONS_NONE))
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "CPU bitmap", checker.Ptr, &IID_ID2D1Bitmap1, &IID_ID2D1Bitmap, &IID_ID2D1Image)
	if o := checker.Options(); o != BITMAP_OPTIONS_NONE {
		t.Errorf("CPU bitmap options %#x", o)
	}

	// DrawBitmap (float path) scaled 16× into 8…40: each quadrant at its
	// centre, white around it; then at half opacity over white.
	dst := RectF{8, 8, 40, 40}
	frame("DrawBitmap", white, func() { dc.DrawBitmap(checker, &dst, 1, INTERPOLATION_MODE_NEAREST_NEIGHBOR, nil) })
	expect("DrawBitmap", bgraR, [2]int{8, 8}, [2]int{16, 16}, [2]int{23, 23})
	expect("DrawBitmap", bgraG, [2]int{32, 16}, [2]int{39, 8})
	expect("DrawBitmap", bgraB, [2]int{16, 32}, [2]int{8, 39})
	expect("DrawBitmap", bgraK, [2]int{32, 32}, [2]int{39, 39})
	expect("DrawBitmap", bgraW, [2]int{7, 7}, [2]int{40, 40}, [2]int{4, 24}, [2]int{50, 24})
	frame("DrawBitmap half", white, func() { dc.DrawBitmap(checker, &dst, 0.5, INTERPOLATION_MODE_NEAREST_NEIGHBOR, nil) })
	expect("DrawBitmap half", [4]byte{128, 128, 255, 255}, [2]int{16, 16})
	expect("DrawBitmap half", [4]byte{128, 255, 128, 255}, [2]int{32, 16})
	expect("DrawBitmap half", [4]byte{255, 128, 128, 255}, [2]int{16, 32})
	expect("DrawBitmap half", [4]byte{128, 128, 128, 255}, [2]int{32, 32})
	// Linear interpolation mixes all four at the centre; a source rect
	// picks one texel.
	frame("DrawBitmap linear", white, func() {
		dc.DrawBitmap(checker, &dst, 1, INTERPOLATION_MODE_LINEAR, nil)
		dc.DrawBitmap(checker, &RectF{48, 48, 56, 56}, 1, INTERPOLATION_MODE_NEAREST_NEIGHBOR, &RectF{1, 1, 2, 2})
	})
	if c := at(24, 24); c[0] < 40 || c[0] > 100 || c[1] < 40 || c[1] > 100 || c[2] < 40 || c[2] > 100 {
		t.Errorf("DrawBitmap linear: centre BGRA %v, want a mix of all four", c)
	}
	expect("DrawBitmap source rect", bgraK, [2]int{48, 48}, [2]int{55, 55})
	// DrawImage of the bitmap: its 2×2 DIPs at the offset, and a source
	// rect of one texel.
	frame("DrawImage", white, func() {
		dc.DrawImage(checker.Image(), &Point2F{40, 10}, nil, INTERPOLATION_MODE_NEAREST_NEIGHBOR, COMPOSITE_MODE_SOURCE_OVER)
		dc.DrawImage(checker.Image(), &Point2F{20, 30}, &RectF{1, 0, 2, 1}, INTERPOLATION_MODE_NEAREST_NEIGHBOR, COMPOSITE_MODE_SOURCE_OVER)
	})
	expect("DrawImage", bgraR, [2]int{40, 10})
	expect("DrawImage", bgraG, [2]int{41, 10})
	expect("DrawImage", bgraB, [2]int{40, 11})
	expect("DrawImage", bgraK, [2]int{41, 11})
	expect("DrawImage", bgraG, [2]int{20, 30}) // the source rect's corner lands at the offset
	expect("DrawImage", bgraW, [2]int{19, 30}, [2]int{21, 30}, [2]int{20, 31}, [2]int{39, 10}, [2]int{42, 10})

	// The shadow effect on a bitmap mask (an opaque square 24…40).
	mask, err := dc.CreateBitmap(SizeU{n, n}, pixelBGRA(n, n, func(x, y int) [4]byte {
		if x >= 24 && x < 40 && y >= 24 && y < 40 {
			return bgraW
		}
		return [4]byte{}
	}), 4*n, props96(BITMAP_OPTIONS_NONE))
	if err != nil {
		t.Fatal(err)
	}
	shadow, err := dc.CreateEffect(&CLSID_D2D1Shadow)
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "shadow effect", shadow.Ptr, &IID_ID2D1Effect, &IID_ID2D1Properties)
	if g, err := shadow.CLSID(); err != nil || g != CLSID_D2D1Shadow {
		t.Errorf("shadow CLSID %v, %v", g, err)
	}
	if c := shadow.PropertyCount(); c != 3 {
		t.Errorf("shadow has %d properties, want 3", c)
	}
	for i, want := range []PropertyType{PROPERTY_TYPE_FLOAT, PROPERTY_TYPE_VECTOR4, PROPERTY_TYPE_ENUM, PROPERTY_TYPE_UNKNOWN} {
		if got := shadow.Type(uint32(i)); got != want {
			t.Errorf("shadow property %d type %d, want %d", i, got, want)
		}
	}
	if c := shadow.InputCount(); c != 1 {
		t.Errorf("shadow has %d inputs", c)
	}
	shadow.SetInput(0, mask.Image())
	color := Vector4F{0, 0, 0.5, 1} // dark blue
	if err := shadow.SetVector4(SHADOW_PROP_COLOR, color); err != nil {
		t.Fatal(err)
	}
	if err := shadow.SetEnum(SHADOW_PROP_OPTIMIZATION, OPTIMIZATION_QUALITY); err != nil {
		t.Fatal(err)
	}
	if v, err := shadow.Vector4(SHADOW_PROP_COLOR); err != nil || v != color {
		t.Errorf("shadow color %v, %v", v, err)
	}
	if v, err := shadow.Enum(SHADOW_PROP_OPTIMIZATION); err != nil || v != OPTIMIZATION_QUALITY {
		t.Errorf("shadow optimization %v, %v", v, err)
	}
	if err := shadow.SetFloat(SHADOW_PROP_COLOR, 1); err == nil {
		t.Error("a FLOAT into the VECTOR4 color succeeded")
	}
	shadowOut := shadow.Output()
	queries(t, "effect output", shadowOut.Ptr, &IID_ID2D1Image)
	// The blue channel along the row y = 32 at σ = 1 and σ = 4.
	row := map[float32][]byte{}
	for _, sigma := range []float32{1, 4} {
		if err := shadow.SetFloat(SHADOW_PROP_BLUR_STANDARD_DEVIATION, sigma); err != nil {
			t.Fatal(err)
		}
		if v, err := shadow.Float(SHADOW_PROP_BLUR_STANDARD_DEVIATION); err != nil || v != sigma {
			t.Errorf("shadow σ %v, %v; want %v", v, err, sigma)
		}
		frame("shadow", white, func() {
			dc.DrawImage(shadowOut, nil, nil, INTERPOLATION_MODE_LINEAR, COMPOSITE_MODE_SOURCE_OVER)
		})
		for x := range n {
			row[sigma] = append(row[sigma], at(x, 32)[1]) // green: 255 on white, 0 under the shadow
		}
		// Far away nothing; at σ 1 the shadow is solid under the square.
		if sigma == 1 {
			expect("shadow", [4]byte{128, 0, 0, 255}, [2]int{32, 32}, [2]int{28, 28})
		}
		expect("shadow", bgraW, [2]int{2, 2}, [2]int{60, 32}, [2]int{32, 61})
	}
	for _, sigma := range []float32{1, 4} {
		r := row[sigma]
		// Dark just outside the mask, falling off monotonically to white.
		if r[40] > 200 {
			t.Errorf("σ %v: just outside the mask (x 40) green %d, want darkened", sigma, r[40])
		}
		for x := 40; x < n-1; x++ {
			if r[x+1] < r[x] {
				t.Errorf("σ %v: falloff not monotone at x %d: %d then %d", sigma, x, r[x], r[x+1])
			}
		}
	}
	// A wider σ reaches farther out.
	if !(int(row[4][42]) < int(row[1][42])-40) || row[1][44] < 250 || row[4][44] > 240 {
		t.Errorf("green at x 42 and 44: %d, %d at σ 1, %d, %d at σ 4; want σ 4 darker and reaching farther",
			row[1][42], row[1][44], row[4][42], row[4][44])
	}

	// A Gaussian blur with hard borders on a sharp black|white edge at
	// x = 32, drawn over red: the target's edges keep the edge pixels'
	// colors (no red showing through), the edge goes intermediate.
	edge, err := dc.CreateBitmap(SizeU{n, n}, pixelBGRA(n, n, func(x, y int) [4]byte {
		if x < 32 {
			return bgraK
		}
		return bgraW
	}), 4*n, props96(BITMAP_OPTIONS_NONE))
	if err != nil {
		t.Fatal(err)
	}
	blur, err := dc.CreateEffect(&CLSID_D2D1GaussianBlur)
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "blur effect", blur.Ptr, &IID_ID2D1Effect, &IID_ID2D1Properties)
	if g, err := blur.CLSID(); err != nil || g != CLSID_D2D1GaussianBlur {
		t.Errorf("blur CLSID %v, %v", g, err)
	}
	if err := blur.SetInputCount(2); err == nil {
		t.Error("SetInputCount(2) on the one-input blur succeeded")
	}
	blur.SetInput(0, edge.Image())
	if err := blur.SetFloat(GAUSSIANBLUR_PROP_STANDARD_DEVIATION, 3); err != nil {
		t.Fatal(err)
	}
	if err := blur.SetEnum(GAUSSIANBLUR_PROP_BORDER_MODE, BORDER_MODE_HARD); err != nil {
		t.Fatal(err)
	}
	if v, err := blur.Enum(GAUSSIANBLUR_PROP_BORDER_MODE); err != nil || v != BORDER_MODE_HARD {
		t.Errorf("blur border mode %v, %v", v, err)
	}
	blurOut := blur.Output()
	frame("blur", ColorF{1, 0, 0, 1}, func() {
		dc.DrawImage(blurOut, nil, nil, INTERPOLATION_MODE_LINEAR, COMPOSITE_MODE_SOURCE_OVER)
	})
	expect("blur", bgraK, [2]int{0, 0}, [2]int{0, 32}, [2]int{2, 63}, [2]int{20, 32})
	expect("blur", bgraW, [2]int{63, 0}, [2]int{63, 32}, [2]int{61, 63}, [2]int{44, 32})
	for _, x := range []int{30, 31, 32, 33} {
		if c := at(x, 32); c[0] < 20 || c[0] > 235 || c[0] != c[2] {
			t.Errorf("blur: edge pixel x %d = BGRA %v, want an intermediate gray", x, c)
		}
	}
	if !(at(29, 32)[0] < at(31, 32)[0] && at(31, 32)[0] < at(33, 32)[0] && at(33, 32)[0] < at(35, 32)[0]) {
		t.Errorf("blur: edge not increasing: %v %v %v %v", at(29, 32), at(31, 32), at(33, 32), at(35, 32))
	}

	// CopyFromBitmap from the CANNOT_DRAW target inside BeginDraw: the copy
	// of the top-left 32×32 holds the red square drawn just before it (not
	// the previous frame's blue), and drawn back at 32, 32 after a clear it
	// shows there.
	frame("previous", ColorF{0, 0, 1, 1}, func() {})
	scratch, err := dc.CreateBitmap(SizeU{32, 32}, nil, 0, props96(BITMAP_OPTIONS_NONE))
	if err != nil {
		t.Fatal(err)
	}
	red, err := dc.CreateSolidColorBrush(ColorF{1, 0, 0, 1}, DefaultBrushProperties())
	if err != nil {
		t.Fatal(err)
	}
	green, err := dc.CreateSolidColorBrush(ColorF{0, 1, 0, 1}, DefaultBrushProperties())
	if err != nil {
		t.Fatal(err)
	}
	frame("CopyFromBitmap", white, func() {
		dc.FillRectangle(RectF{8, 8, 24, 24}, red.Brush())
		if err := dc.Flush(); err != nil {
			t.Errorf("Flush: %v", err)
		}
		if err := scratch.CopyFromBitmap(nil, target, &RectU{0, 0, 32, 32}); err != nil {
			t.Errorf("CopyFromBitmap inside BeginDraw: %v", err)
		}
		dc.Clear(ColorF{0, 0, 1, 1})
		dc.DrawBitmap(scratch, &RectF{32, 32, 64, 64}, 1, INTERPOLATION_MODE_NEAREST_NEIGHBOR, nil)
	})
	expect("CopyFromBitmap", bgraR, [2]int{40, 40}, [2]int{55, 55})
	expect("CopyFromBitmap", bgraW, [2]int{33, 33}, [2]int{60, 40}, [2]int{40, 60})
	expect("CopyFromBitmap", bgraB, [2]int{16, 16}, [2]int{4, 40}, [2]int{40, 4})
	// A destination point and source rect: the 8×8 at 12, 12 (red) into
	// the scratch at 0, 0 replaces only that corner.
	frame("CopyFromBitmap offsets", white, func() {
		dc.FillRectangle(RectF{8, 8, 24, 24}, red.Brush())
		if err := dc.Flush(); err != nil {
			t.Errorf("Flush: %v", err)
		}
		if err := scratch.CopyFromBitmap(&Point2U{24, 24}, target, &RectU{12, 12, 20, 20}); err != nil {
			t.Errorf("CopyFromBitmap with offsets: %v", err)
		}
		dc.Clear(white)
		dc.DrawBitmap(scratch, &RectF{32, 32, 64, 64}, 1, INTERPOLATION_MODE_NEAREST_NEIGHBOR, nil)
	})
	expect("CopyFromBitmap offsets", bgraR, [2]int{40, 40}, [2]int{58, 58}) // left from the first copy and the new corner
	expect("CopyFromBitmap offsets", bgraW, [2]int{33, 33}, [2]int{60, 40})

	// Group opacity through PushLayer: two overlapping opaque squares at
	// 0.5 show only the top one in the overlap (blended once with the
	// background), unlike two half-opaque fills, which mix all three.
	frame("group opacity", white, func() {
		dc.PushLayer(&LayerParameters1{ContentBounds: InfiniteRect(), MaskTransform: IdentityMatrix(), Opacity: 0.5})
		dc.FillRectangle(RectF{8, 8, 40, 40}, red.Brush())
		dc.FillRectangle(RectF{24, 24, 56, 56}, green.Brush())
		dc.PopLayer()
	})
	expect("group opacity", [4]byte{128, 128, 255, 255}, [2]int{16, 16})
	expect("group opacity", [4]byte{128, 255, 128, 255}, [2]int{32, 32}, [2]int{48, 48})
	frame("per-shape opacity", white, func() {
		dc.DrawBitmap(checker, nil, 1, INTERPOLATION_MODE_NEAREST_NEIGHBOR, nil) // unrelated: a nil dst draws at the origin
		red.SetColor(ColorF{1, 0, 0, 0.5})
		green.SetColor(ColorF{0, 1, 0, 0.5})
		dc.FillRectangle(RectF{8, 8, 40, 40}, red.Brush())
		dc.FillRectangle(RectF{24, 24, 56, 56}, green.Brush())
		red.SetColor(ColorF{1, 0, 0, 1})
		green.SetColor(ColorF{0, 1, 0, 1})
	})
	expect("per-shape opacity", [4]byte{64, 191, 128, 255}, [2]int{32, 32})
	expect("per-shape opacity", bgraR, [2]int{0, 0})
	expect("per-shape opacity", bgraK, [2]int{1, 1})

	// A second device context of the same device draws into a target
	// bitmap that the first one then draws (bitmaps are shared within a
	// device): a 16×8 bitmap, its left 12 columns green on transparent, drawn at
	// its own size at 16, 16 (a swapped SizeU would make it 8×16).
	dc2, err := d2dev.CreateDeviceContext(DEVICE_CONTEXT_OPTIONS_NONE)
	if err != nil {
		t.Fatal(err)
	}
	offscreen, err := dc2.CreateBitmap(SizeU{16, 8}, nil, 0, props96(BITMAP_OPTIONS_TARGET))
	if err != nil {
		t.Fatal(err)
	}
	green2, err := dc2.CreateSolidColorBrush(ColorF{0, 1, 0, 1}, DefaultBrushProperties())
	if err != nil {
		t.Fatal(err)
	}
	dc2.SetTarget(offscreen.Ptr)
	dc2.BeginDraw()
	dc2.Clear(ColorF{})
	dc2.FillRectangle(RectF{0, 0, 12, 8}, green2.Brush())
	if err := dc2.EndDraw(); err != nil {
		t.Fatalf("second context EndDraw: %v", err)
	}
	dc2.SetTarget(0)
	frame("second context", white, func() {
		dc.DrawImage(offscreen.Image(), &Point2F{16, 16}, nil, INTERPOLATION_MODE_NEAREST_NEIGHBOR, COMPOSITE_MODE_SOURCE_OVER)
	})
	expect("second context", bgraG, [2]int{16, 16}, [2]int{27, 23})
	expect("second context", bgraW, [2]int{28, 16}, [2]int{31, 23}, [2]int{20, 24}, [2]int{20, 28}, [2]int{15, 20})
	release(t, "second context brush", &green2.Ptr, 0)
	release(t, "offscreen bitmap", &offscreen.Ptr, 0)
	release(t, "second context", &dc2.Ptr, 0)
	release(t, "red", &red.Ptr, 0)
	release(t, "green", &green.Ptr, 0)

	// The crop effect (S2.4e_3): the 2×2 checker's top-left texel cropped
	// and drawn 1:1 at 40, 40 leaves its neighbours out.
	crop, err := dc.CreateEffect(&CLSID_D2D1Crop)
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "crop effect", crop.Ptr, &IID_ID2D1Effect, &IID_ID2D1Properties)
	if g, err := crop.CLSID(); err != nil || g != CLSID_D2D1Crop {
		t.Errorf("crop CLSID %v, %v", g, err)
	}
	if crop.Type(CROP_PROP_RECT) != PROPERTY_TYPE_VECTOR4 || crop.Type(CROP_PROP_BORDER_MODE) != PROPERTY_TYPE_ENUM {
		t.Errorf("crop property types %d, %d", crop.Type(CROP_PROP_RECT), crop.Type(CROP_PROP_BORDER_MODE))
	}
	crop.SetInput(0, checker.Image())
	if err := firstErrT(crop.SetVector4(CROP_PROP_RECT, Vector4F{0, 0, 1, 1}), crop.SetEnum(CROP_PROP_BORDER_MODE, BORDER_MODE_HARD)); err != nil {
		t.Fatal(err)
	}
	if v, err := crop.Vector4(CROP_PROP_RECT); err != nil || v != (Vector4F{0, 0, 1, 1}) {
		t.Errorf("crop rect %v, %v", v, err)
	}
	cropOut := crop.Output()
	frame("crop", white, func() {
		dc.DrawImage(cropOut, &Point2F{40, 40}, nil, INTERPOLATION_MODE_NEAREST_NEIGHBOR, COMPOSITE_MODE_SOURCE_OVER)
	})
	expect("crop", bgraR, [2]int{40, 40})
	expect("crop", bgraW, [2]int{41, 40}, [2]int{40, 41}, [2]int{39, 40}, [2]int{41, 41})
	release(t, "crop output", &cropOut.Ptr, 1)
	release(t, "crop", &crop.Ptr, 0)

	// Teardown.
	release(t, "blur output", &blurOut.Ptr, 1) // the effect still holds it
	release(t, "blur", &blur.Ptr, 0)
	release(t, "edge", &edge.Ptr, 0)
	release(t, "shadow output", &shadowOut.Ptr, 1)
	release(t, "shadow", &shadow.Ptr, 0)
	release(t, "mask", &mask.Ptr, 0)
	release(t, "scratch", &scratch.Ptr, 0)
	release(t, "checker", &checker.Ptr, 0)
	dc.SetTarget(0)
	release(t, "target", &target.Ptr, 0)
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
