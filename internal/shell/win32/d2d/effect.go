//go:build windows

package d2d

import (
	"fmt"
	"unsafe"

	"github.com/radix29/gogui/internal/shell/win32/com"
)

// The effect and bitmap bindings (S2.4e_1): bitmaps from CPU pixels,
// DrawBitmap and DrawImage, ID2D1Effect with its ID2D1Properties base, the
// shadow and Gaussian-blur effects, and ID2D1Bitmap::CopyFromBitmap.

// Interface and class IDs of the effect interfaces.
var (
	IID_ID2D1Properties = com.MustGUID("{483473D7-CD46-4F9D-9D3A-3112AA80159D}")
	IID_ID2D1Effect     = com.MustGUID("{28211A43-7D89-476F-8181-2D6159B220AD}")

	CLSID_D2D1Shadow       = com.MustGUID("{C67EA361-1863-4E69-89DB-695D3E9A5B6B}")
	CLSID_D2D1GaussianBlur = com.MustGUID("{1FEB6D69-2FE6-4AC9-8C58-1D7F93E7A6A5}")
	CLSID_D2D1Crop         = com.MustGUID("{E23F7110-0E9A-4324-AF47-6A2C0C46F35B}") // S2.4e_3
)

// Vtable slots of the effect and bitmap methods; the tests check each one.
const (
	// ID2D1RenderTarget.
	slotRTFlush = 42

	// ID2D1DeviceContext.
	slotDCCreateBitmap = 57 // D2D1_SIZE_U by value: one integer register
	slotDCCreateEffect = 63
	slotDCDrawImage    = 83
	slotDCDrawBitmap   = 85 // FLOAT opacity: the float path

	// ID2D1Bitmap.
	slotBitmapCopyFromBitmap = 8

	// ID2D1Properties.
	slotPropsGetPropertyCount = 3
	slotPropsGetType          = 6
	slotPropsSetValue         = 9
	slotPropsGetValue         = 11

	// ID2D1Effect (after ID2D1Properties 3–13).
	slotEffectSetInput      = 14
	slotEffectSetInputCount = 15
	slotEffectGetInputCount = 17
	slotEffectGetOutput     = 18
)

var methodDrawBitmap = com.Method[func(this, bitmap com.Ptr, dst *RectF, opacity float32,
	interp InterpolationMode, src *RectF, perspective uintptr)]{Slot: slotDCDrawBitmap}

// --- Device context ----------------------------------------------------------

// CreateBitmap creates a bitmap of size pixels. src holds the initial
// pixels, rows pitch bytes apart, in props' format (or is nil for
// uninitialized content); it must hold at least pitch × height bytes. A
// bitmap a context draws into needs BITMAP_OPTIONS_TARGET; one that is
// drawn or fed to an effect must not have BITMAP_OPTIONS_CANNOT_DRAW.
func (c DeviceContext) CreateBitmap(size SizeU, src []byte, pitch uint32, props *BitmapProperties1) (Bitmap1, error) {
	var sp *byte
	if src != nil {
		if uint64(len(src)) < uint64(pitch)*uint64(size.Height) {
			return Bitmap1{}, fmt.Errorf("d2d: CreateBitmap: %d bytes of pixels for %d rows of %d", len(src), size.Height, pitch)
		}
		if len(src) > 0 {
			sp = &src[0]
		}
	}
	var out com.Ptr
	err := c.CallHR("ID2D1DeviceContext::CreateBitmap", slotDCCreateBitmap,
		size.packed(), uintptr(unsafe.Pointer(sp)), uintptr(pitch), uintptr(unsafe.Pointer(props)),
		uintptr(unsafe.Pointer(&out)))
	return Bitmap1{out}, err
}

// CreateEffect creates the built-in effect clsid (CLSID_D2D1Shadow,
// CLSID_D2D1GaussianBlur).
func (c DeviceContext) CreateEffect(clsid *com.GUID) (Effect, error) {
	var out com.Ptr
	err := c.CallHR("ID2D1DeviceContext::CreateEffect", slotDCCreateEffect,
		uintptr(unsafe.Pointer(clsid)), uintptr(unsafe.Pointer(&out)))
	return Effect{out}, err
}

// DrawImage draws img (an effect's output or a bitmap) under the current
// transform with its origin at offset (nil: 0, 0); src (nil: all of it)
// selects the part of the image, in the image's space.
func (c DeviceContext) DrawImage(img Image, offset *Point2F, src *RectF, interp InterpolationMode, comp CompositeMode) {
	c.Call(slotDCDrawImage, uintptr(img.Ptr), uintptr(unsafe.Pointer(offset)), uintptr(unsafe.Pointer(src)),
		uintptr(interp), uintptr(comp))
}

// DrawBitmap draws the part src (nil: all of it, in the bitmap's DIPs) of
// b scaled into dst (nil: the bitmap's size at the origin) under the
// current transform, at opacity. It is ID2D1DeviceContext's overload,
// which takes a D2D1_INTERPOLATION_MODE, on the float path.
func (c DeviceContext) DrawBitmap(b Bitmap1, dst *RectF, opacity float32, interp InterpolationMode, src *RectF) {
	methodDrawBitmap.Fn(c.Ptr)(c.Ptr, b.Ptr, dst, opacity, interp, src, 0)
}

// --- Images and bitmaps ---------------------------------------------------------

// Image is ID2D1Image, the base DrawImage and SetInput take. Like Brush it
// is a view without a reference of its own, except where a method says it
// returns one (Effect.Output).
type Image struct{ com.Ptr }

// Image returns b as an ID2D1Image (no new reference).
func (b Bitmap1) Image() Image { return Image(b) }

// CopyFromBitmap copies the part srcRect (nil: all) of src's pixels into b
// at dst (nil: 0, 0). Both have the same format; src may be a
// BITMAP_OPTIONS_CANNOT_DRAW target (a back buffer). Inside BeginDraw the
// drawing so far is flushed to src first.
func (b Bitmap1) CopyFromBitmap(dst *Point2U, src Bitmap1, srcRect *RectU) error {
	return b.CallHR("ID2D1Bitmap::CopyFromBitmap", slotBitmapCopyFromBitmap,
		uintptr(unsafe.Pointer(dst)), uintptr(src.Ptr), uintptr(unsafe.Pointer(srcRect)))
}

// --- Effects --------------------------------------------------------------------

// Effect is ID2D1Effect with its ID2D1Properties methods.
type Effect struct{ com.Ptr }

// PropertyCount returns the number of the effect's own properties
// (system properties, PROPERTY_CLSID and up, not counted).
func (e Effect) PropertyCount() uint32 {
	return uint32(e.Call(slotPropsGetPropertyCount))
}

// Type returns the type of property index, or PROPERTY_TYPE_UNKNOWN for
// none.
func (e Effect) Type(index uint32) PropertyType {
	return PropertyType(uint32(e.Call(slotPropsGetType, uintptr(index))))
}

// SetValue sets property index from data, which holds a value of typ.
func (e Effect) SetValue(index uint32, typ PropertyType, data []byte) error {
	var dp *byte
	if len(data) > 0 {
		dp = &data[0]
	}
	return e.CallHR("ID2D1Properties::SetValue", slotPropsSetValue,
		uintptr(index), uintptr(typ), uintptr(unsafe.Pointer(dp)), uintptr(len(data)))
}

// Value reads property index, of typ, into data (exactly its size).
func (e Effect) Value(index uint32, typ PropertyType, data []byte) error {
	var dp *byte
	if len(data) > 0 {
		dp = &data[0]
	}
	return e.CallHR("ID2D1Properties::GetValue", slotPropsGetValue,
		uintptr(index), uintptr(typ), uintptr(unsafe.Pointer(dp)), uintptr(len(data)))
}

// bytesOf views *v as bytes (Go memory, for a call expression).
func bytesOf[T any](v *T) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(v)), unsafe.Sizeof(*v))
}

// SetFloat sets a FLOAT property.
func (e Effect) SetFloat(index uint32, v float32) error {
	return e.SetValue(index, PROPERTY_TYPE_FLOAT, bytesOf(&v))
}

// SetEnum sets an enum property (a UINT32 on the wire).
func (e Effect) SetEnum(index, v uint32) error {
	return e.SetValue(index, PROPERTY_TYPE_ENUM, bytesOf(&v))
}

// SetVector4 sets a VECTOR4 property (a shadow's color, for one).
func (e Effect) SetVector4(index uint32, v Vector4F) error {
	return e.SetValue(index, PROPERTY_TYPE_VECTOR4, bytesOf(&v))
}

// Float reads a FLOAT property.
func (e Effect) Float(index uint32) (float32, error) {
	var v float32
	err := e.Value(index, PROPERTY_TYPE_FLOAT, bytesOf(&v))
	return v, err
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

// SetInput sets input index to img (or the zero Image to clear it); the
// effect takes a reference of its own.
func (e Effect) SetInput(index uint32, img Image) {
	e.Call(slotEffectSetInput, uintptr(index), uintptr(img.Ptr), 1)
}

// SetInputCount changes the number of inputs (fails for effects with a
// fixed count).
func (e Effect) SetInputCount(n uint32) error {
	return e.CallHR("ID2D1Effect::SetInputCount", slotEffectSetInputCount, uintptr(n))
}

// InputCount returns the number of inputs.
func (e Effect) InputCount() uint32 {
	return uint32(e.Call(slotEffectGetInputCount))
}

// Output returns the effect's output image with a reference of its own
// (release it); it stays valid while the effect lives and follows its
// inputs and properties.
func (e Effect) Output() Image {
	var out com.Ptr
	e.Call(slotEffectGetOutput, uintptr(unsafe.Pointer(&out)))
	return Image{out}
}

// Flush executes the batched drawing commands now and returns the first
// error since the last flush or BeginDraw. ID2D1Bitmap::CopyFromBitmap
// from the context's target inside BeginDraw reads what the GPU holds, not
// the batch: call Flush first (finding of S2.4e_1).
func (c DeviceContext) Flush() error {
	return c.CallHR("ID2D1RenderTarget::Flush", slotRTFlush, 0, 0)
}
