//go:build windows

package w32

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/radix29/gogui/internal/ffi"
)

// The expected layouts come from the Windows SDK headers. amd64 and arm64
// are both LLP64 (int and long 4 bytes, pointers and handles 8), so one
// table serves both; the arm64 run happens through scripts/native.sh on a
// windows/arm64 machine. gogui has no 32-bit Windows target.

type layoutCase struct {
	v    any
	want ffi.Layout
}

func f(name string, off, size uintptr) ffi.Field {
	return ffi.Field{Name: name, Offset: off, Size: size}
}

func layoutCases() []layoutCase {
	return []layoutCase{
		{Point{}, ffi.Layout{Size: 8, Align: 4, Fields: []ffi.Field{
			f("X", 0, 4), f("Y", 4, 4)}}},
		{Rect{}, ffi.Layout{Size: 16, Align: 4, Fields: []ffi.Field{
			f("Left", 0, 4), f("Top", 4, 4), f("Right", 8, 4), f("Bottom", 12, 4)}}},
		{Size{}, ffi.Layout{Size: 8, Align: 4, Fields: []ffi.Field{
			f("Cx", 0, 4), f("Cy", 4, 4)}}},
		{Msg{}, ffi.Layout{Size: 48, Align: 8, Fields: []ffi.Field{
			f("Hwnd", 0, 8), f("Message", 8, 4), f("WParam", 16, 8), f("LParam", 24, 8),
			f("Time", 32, 4), f("Pt", 36, 8), f("LPrivate", 44, 4)}}},
		{WndClassEx{}, ffi.Layout{Size: 80, Align: 8, Fields: []ffi.Field{
			f("CbSize", 0, 4), f("Style", 4, 4), f("LpfnWndProc", 8, 8),
			f("CbClsExtra", 16, 4), f("CbWndExtra", 20, 4), f("HInstance", 24, 8),
			f("HIcon", 32, 8), f("HCursor", 40, 8), f("HbrBackground", 48, 8),
			f("LpszMenuName", 56, 8), f("LpszClassName", 64, 8), f("HIconSm", 72, 8)}}},
		{MinMaxInfo{}, ffi.Layout{Size: 40, Align: 4, Fields: []ffi.Field{
			f("PtReserved", 0, 8), f("PtMaxSize", 8, 8), f("PtMaxPosition", 16, 8),
			f("PtMinTrackSize", 24, 8), f("PtMaxTrackSize", 32, 8)}}},
		{WindowPos{}, ffi.Layout{Size: 40, Align: 8, Fields: []ffi.Field{
			f("Hwnd", 0, 8), f("HwndInsertAfter", 8, 8), f("X", 16, 4), f("Y", 20, 4),
			f("Cx", 24, 4), f("Cy", 28, 4), f("Flags", 32, 4)}}},
		{NCCalcSizeParams{}, ffi.Layout{Size: 56, Align: 8, Fields: []ffi.Field{
			f("Rgrc", 0, 48), f("Lppos", 48, 8)}}},
		{TrackMouseEventInfo{}, ffi.Layout{Size: 24, Align: 8, Fields: []ffi.Field{
			f("CbSize", 0, 4), f("DwFlags", 4, 4), f("HwndTrack", 8, 8), f("DwHoverTime", 16, 4)}}},
		{MonitorInfo{}, ffi.Layout{Size: 40, Align: 4, Fields: []ffi.Field{
			f("CbSize", 0, 4), f("RcMonitor", 4, 16), f("RcWork", 20, 16), f("DwFlags", 36, 4)}}},
		{Margins{}, ffi.Layout{Size: 16, Align: 4, Fields: []ffi.Field{
			f("CxLeftWidth", 0, 4), f("CxRightWidth", 4, 4),
			f("CyTopHeight", 8, 4), f("CyBottomHeight", 12, 4)}}},
		{CreateStruct{}, ffi.Layout{Size: 80, Align: 8, Fields: []ffi.Field{
			f("LpCreateParams", 0, 8), f("HInstance", 8, 8), f("HMenu", 16, 8),
			f("HwndParent", 24, 8), f("Cy", 32, 4), f("Cx", 36, 4), f("Y", 40, 4),
			f("X", 44, 4), f("Style", 48, 4), f("LpszName", 56, 8), f("LpszClass", 64, 8),
			f("DwExStyle", 72, 4)}}},
		{PaintStruct{}, ffi.Layout{Size: 72, Align: 8, Fields: []ffi.Field{
			f("Hdc", 0, 8), f("FErase", 8, 4), f("RcPaint", 12, 16), f("FRestore", 28, 4),
			f("FIncUpdate", 32, 4), f("RgbReserved", 36, 32)}}},
		{HighContrast{}, ffi.Layout{Size: 16, Align: 8, Fields: []ffi.Field{
			f("CbSize", 0, 4), f("DwFlags", 4, 4), f("LpszDefaultScheme", 8, 8)}}},
		{BitmapInfoHeader{}, ffi.Layout{Size: 40, Align: 4, Fields: []ffi.Field{
			f("BiSize", 0, 4), f("BiWidth", 4, 4), f("BiHeight", 8, 4), f("BiPlanes", 12, 2),
			f("BiBitCount", 14, 2), f("BiCompression", 16, 4), f("BiSizeImage", 20, 4),
			f("BiXPelsPerMeter", 24, 4), f("BiYPelsPerMeter", 28, 4), f("BiClrUsed", 32, 4),
			f("BiClrImportant", 36, 4)}}},
		{WindowPlacement{}, ffi.Layout{Size: 44, Align: 4, Fields: []ffi.Field{
			f("Length", 0, 4), f("Flags", 4, 4), f("ShowCmd", 8, 4),
			f("PtMinPosition", 12, 8), f("PtMaxPosition", 20, 8),
			f("RcNormalPosition", 28, 16)}}},
	}
}

func TestStructLayouts(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Fatal("the layout tables are for 64-bit Windows (amd64, arm64) only")
	}
	for _, c := range layoutCases() {
		ffi.CheckLayout(t, c.v, c.want)
		ffi.CheckNoGoPointers(t, c.v)
	}
}

// TestLayoutCasesComplete keeps the table in step with types.go: every
// mirrored struct the S2.3a list names (and Size, S2.3d) has a case.
func TestLayoutCasesComplete(t *testing.T) {
	want := []string{"Point", "Rect", "Msg", "WndClassEx", "MinMaxInfo", "WindowPos",
		"NCCalcSizeParams", "TrackMouseEventInfo", "MonitorInfo", "Margins",
		"CreateStruct", "PaintStruct", "HighContrast", "WindowPlacement", "Size", "BitmapInfoHeader"}
	have := map[string]bool{}
	for _, c := range layoutCases() {
		have[typeName(c.v)] = true
	}
	for _, n := range want {
		if !have[n] {
			t.Errorf("no layout case for %s", n)
		}
	}
	if len(have) != len(want) {
		t.Errorf("%d layout cases, want %d", len(have), len(want))
	}
}

func typeName(v any) string { return reflect.TypeOf(v).Name() }

func TestPackPoint(t *testing.T) {
	for _, c := range []struct {
		p    Point
		want uintptr
	}{
		{Point{0, 0}, 0},
		{Point{1, 2}, 0x2_0000_0001},
		{Point{-1, 0}, 0xFFFF_FFFF},
		{Point{0, -1}, 0xFFFF_FFFF_0000_0000},
		{Point{-2, -3}, 0xFFFF_FFFD_FFFF_FFFE},
	} {
		if got := packPoint(c.p); got != c.want {
			t.Errorf("packPoint(%v) = %#x, want %#x", c.p, got, c.want)
		}
	}
}
