//go:build windows

package d2d

import (
	"syscall"
	"unsafe"

	"github.com/radix29/gogui/internal/shell/win32/com"
)

// Test-only bindings (review §4): nothing outside this package's tests
// calls them.

// Options returns the bitmap's BITMAP_OPTIONS_*.
func (b Bitmap1) Options() BitmapOptions {
	return BitmapOptions(uint32(b.Call(slotBitmap1GetOptions)))
}

// SetColor changes the brush's color.
func (b SolidColorBrush) SetColor(color ColorF) {
	syscall.SyscallN(b.Slot(slotSolidSetColor), uintptr(b.Ptr), uintptr(unsafe.Pointer(&color)))
}

// FigureCount returns the number of figures (after the sink is closed).
func (g PathGeometry) FigureCount() (uint32, error) {
	var n uint32
	err := g.CallHR("ID2D1PathGeometry::GetFigureCount", slotPathGetFigureCount, uintptr(unsafe.Pointer(&n)))
	return n, err
}

// SegmentCount returns the number of segments (after the sink is closed).
func (g PathGeometry) SegmentCount() (uint32, error) {
	var n uint32
	err := g.CallHR("ID2D1PathGeometry::GetSegmentCount", slotPathGetSegmentCount, uintptr(unsafe.Pointer(&n)))
	return n, err
}

// PropertyCount returns the number of the effect's own properties
// (system properties, PROPERTY_CLSID and up, not counted).
func (e Effect) PropertyCount() uint32 {
	return uint32(e.Call(slotPropsGetPropertyCount))
}

// Enum reads an enum property.
func (e Effect) Enum(index uint32) (uint32, error) {
	var v uint32
	err := e.Value(index, PROPERTY_TYPE_ENUM, bytesOf(&v))
	return v, err
}

// Vector4 reads a VECTOR4 property.
func (e Effect) Vector4(index uint32) (Vector4F, error) {
	var v Vector4F
	err := e.Value(index, PROPERTY_TYPE_VECTOR4, bytesOf(&v))
	return v, err
}

// CLSID reads the effect's class ID (the system property PROPERTY_CLSID).
func (e Effect) CLSID() (com.GUID, error) {
	var g com.GUID
	err := e.Value(PROPERTY_CLSID, PROPERTY_TYPE_CLSID, bytesOf(&g))
	return g, err
}

// InputCount returns the number of inputs.
func (e Effect) InputCount() uint32 {
	return uint32(e.Call(slotEffectGetInputCount))
}

// Gamma returns the interpolation gamma.
func (s GradientStopCollection) Gamma() Gamma {
	return Gamma(uint32(s.Call(slotStopsGetColorInterpolationGamma)))
}

// ExtendMode returns the extend mode.
func (s GradientStopCollection) ExtendMode() ExtendMode {
	return ExtendMode(uint32(s.Call(slotStopsGetExtendMode)))
}

// TransformType returns the stroke transform type (ID2D1StrokeStyle1).
func (s StrokeStyle) TransformType() StrokeTransformType {
	return StrokeTransformType(uint32(s.Call(slotStroke1GetTransformType)))
}

// SetInputCount changes the number of inputs (fails for effects with a
// fixed count).
func (e Effect) SetInputCount(n uint32) error {
	return e.CallHR("ID2D1Effect::SetInputCount", slotEffectSetInputCount, uintptr(n))
}
