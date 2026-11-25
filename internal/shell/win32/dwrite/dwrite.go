//go:build windows

package dwrite

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/radix29/gogui/internal/shell/win32/com"
	"golang.org/x/sys/windows"
)

var dwritedll = windows.NewLazySystemDLL("dwrite.dll")

// allProcs lists every proc declared with newProc, for TestProcsResolve.
var allProcs []*windows.LazyProc

func newProc(dll *windows.LazyDLL, name string) *windows.LazyProc {
	p := dll.NewProc(name)
	allProcs = append(allProcs, p)
	return p
}

var procDWriteCreateFactory = newProc(dwritedll, "DWriteCreateFactory")

// Interface IDs.
var (
	IID_IDWriteFactory        = com.MustGUID("{B859EE5A-D838-4B5B-A2E8-1ADC7D93DB48}")
	IID_IDWriteFactory1       = com.MustGUID("{30572F99-DAC6-41DB-A16E-0486307E606A}")
	IID_IDWriteFactory2       = com.MustGUID("{0439FC60-CA44-4994-8DEE-3A9AF7B732EC}")
	IID_IDWriteFactory3       = com.MustGUID("{9A1B41C3-D3BB-466A-87FC-FE67556A3B65}")
	IID_IDWriteFactory4       = com.MustGUID("{4B0B5BD3-0797-4549-8AC5-FE915CC53856}")
	IID_IDWriteFactory5       = com.MustGUID("{958DB99A-BE2A-4F09-AF7D-65189803D1D3}")
	IID_IDWriteFontCollection = com.MustGUID("{A84CEE02-3EEA-4EEE-A827-87C1A02A0FCC}")
	IID_IDWriteTextFormat     = com.MustGUID("{9C906818-31D7-4FD3-A151-7C5E225DB55A}")
	IID_IDWriteTextLayout     = com.MustGUID("{53737037-6D14-410B-9BFE-0B182BB70961}")
)

// Vtable slots in dwrite.h order; the tests check each one.
const (
	// IDWriteFactory.
	slotFactoryGetSystemFontCollection = 3
	slotFactoryCreateTextFormat        = 15 // FLOAT size: the float path
	slotFactoryCreateTextLayout        = 18 // FLOAT max width and height: the float path

	// IDWriteFontCollection.
	slotCollectionGetFontFamilyCount = 3

	// IDWriteTextFormat.
	slotFormatSetTextAlignment        = 3
	slotFormatSetParagraphAlignment   = 4
	slotFormatSetWordWrapping         = 5
	slotFormatGetTextAlignment        = 11
	slotFormatGetParagraphAlignment   = 12
	slotFormatGetWordWrapping         = 13
	slotFormatGetFontFamilyNameLength = 20
	slotFormatGetFontFamilyName       = 21
	slotFormatGetFontWeight           = 22
	slotFormatGetFontStyle            = 23
	slotFormatGetFontStretch          = 24
	slotFormatGetFontSize             = 25 // FLOAT result: the float path

	// IDWriteTextLayout (after IDWriteTextFormat 0–27).
	slotLayoutSetMaxWidth         = 28 // FLOAT argument
	slotLayoutGetMaxWidth         = 42 // FLOAT result
	slotLayoutGetMaxHeight        = 43 // FLOAT result
	slotLayoutGetLineMetrics      = 59
	slotLayoutGetMetrics          = 60
	slotLayoutGetClusterMetrics   = 62
	slotLayoutHitTestPoint        = 64 // FLOAT point
	slotLayoutHitTestTextPosition = 65 // FLOATs only through pointers: the integer path
	slotLayoutHitTestTextRange    = 66 // FLOAT origin
)

// ErrNoFactory5 is the error of CreateFactory on a DirectWrite without
// IDWriteFactory5, that is before Windows 10 1703 (outside D7).
var ErrNoFactory5 = errors.New("dwrite: IDWriteFactory5 is missing (DirectWrite of Windows 10 1703 or later is required)")

// CreateFactory returns the shared DirectWrite factory as IDWriteFactory5.
// The shared factory is one object per process: each call adds a reference
// to it, and each returned Factory releases its own.
func CreateFactory() (Factory, error) {
	var base com.Ptr
	r, _, _ := procDWriteCreateFactory.Call(FACTORY_TYPE_SHARED,
		uintptr(unsafe.Pointer(&IID_IDWriteFactory)), uintptr(unsafe.Pointer(&base)))
	if err := com.HRESULT(int32(r)).Err("DWriteCreateFactory"); err != nil {
		return Factory{}, err
	}
	defer base.Release()
	f5, err := base.QueryInterface(&IID_IDWriteFactory5)
	if err != nil {
		if errors.Is(err, com.E_NOINTERFACE) {
			return Factory{}, fmt.Errorf("%w: %w", ErrNoFactory5, err)
		}
		return Factory{}, err
	}
	return Factory{f5}, nil
}

// Factory is IDWriteFactory5; its IDWriteFactory methods are bound on it.
type Factory struct{ com.Ptr }

// SystemFontCollection returns the system font collection. checkForUpdates
// makes DirectWrite look for fonts installed since the last call.
func (f Factory) SystemFontCollection(checkForUpdates bool) (FontCollection, error) {
	var out com.Ptr
	err := f.CallHR("IDWriteFactory::GetSystemFontCollection", slotFactoryGetSystemFontCollection,
		uintptr(unsafe.Pointer(&out)), boolArg(checkForUpdates))
	return FontCollection{out}, err
}

// directFloats selects the direct path (com.DirectFloats) for the float-path
// methods; the tests turn it off to check the bound path on amd64 too.
var directFloats = com.DirectFloats

var (
	methodCreateTextFormat = com.Method[func(this com.Ptr, family *uint16, collection com.Ptr,
		weight FontWeight, style FontStyle, stretch FontStretch, size float32, locale *uint16,
		out *com.Ptr) com.HRESULT]{Slot: slotFactoryCreateTextFormat}
	methodCreateTextLayout = com.Method[func(this com.Ptr, text *uint16, length uint32,
		format com.Ptr, maxWidth, maxHeight float32, out *com.Ptr) com.HRESULT]{Slot: slotFactoryCreateTextLayout}
)

// CreateTextFormat creates a text format: family looked up in collection
// (the system collection when collection is nil), size in DIPs, locale a
// BCP 47 name such as "en-us". A family the collection lacks is no error:
// DirectWrite falls back when the text is laid out.
func (f Factory) CreateTextFormat(family string, collection FontCollection, weight FontWeight,
	style FontStyle, stretch FontStretch, size float32, locale string) (TextFormat, error) {
	fam, err := windows.UTF16PtrFromString(family)
	if err != nil {
		return TextFormat{}, fmt.Errorf("dwrite: CreateTextFormat: family %q: %w", family, err)
	}
	loc, err := windows.UTF16PtrFromString(locale)
	if err != nil {
		return TextFormat{}, fmt.Errorf("dwrite: CreateTextFormat: locale %q: %w", locale, err)
	}
	var out com.Ptr
	var hr com.HRESULT
	if directFloats {
		hr = com.HRESULT(int32(f.Call(slotFactoryCreateTextFormat, uintptr(unsafe.Pointer(fam)), uintptr(collection.Ptr),
			uintptr(weight), uintptr(style), uintptr(stretch), com.Float(size), uintptr(unsafe.Pointer(loc)),
			uintptr(unsafe.Pointer(&out)))))
	} else {
		hr = methodCreateTextFormat.Fn(f.Ptr)(f.Ptr, fam, collection.Ptr, weight, style, stretch, size, loc, &out)
	}
	if err := hr.Err("IDWriteFactory::CreateTextFormat"); err != nil {
		return TextFormat{}, err
	}
	return TextFormat{out}, nil
}

// CreateTextLayout lays out text (UTF-16, without a terminator) in format
// inside a box of maxWidth × maxHeight DIPs. Wrapping follows the format's
// word wrapping; the box's height only matters for paragraph alignment.
// The layout holds no reference to text: DirectWrite copies it.
func (f Factory) CreateTextLayout(text []uint16, format TextFormat, maxWidth, maxHeight float32) (TextLayout, error) {
	// A terminated copy, so even empty text has a valid address (DirectWrite
	// rejects a NULL string).
	buf := make([]uint16, len(text)+1)
	copy(buf, text)
	var out com.Ptr
	var hr com.HRESULT
	if directFloats {
		hr = com.HRESULT(int32(f.Call(slotFactoryCreateTextLayout, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(text)),
			uintptr(format.Ptr), com.Float(maxWidth), com.Float(maxHeight), uintptr(unsafe.Pointer(&out)))))
	} else {
		hr = methodCreateTextLayout.Fn(f.Ptr)(f.Ptr, &buf[0], uint32(len(text)), format.Ptr, maxWidth, maxHeight, &out)
	}
	if err := hr.Err("IDWriteFactory::CreateTextLayout"); err != nil {
		return TextLayout{}, err
	}
	return TextLayout{TextFormat{out}}, nil
}

// FontCollection is IDWriteFontCollection.
type FontCollection struct{ com.Ptr }

// FamilyCount returns the number of font families in the collection.
func (c FontCollection) FamilyCount() uint32 {
	return uint32(c.Call(slotCollectionGetFontFamilyCount))
}

// TextFormat is IDWriteTextFormat.
type TextFormat struct{ com.Ptr }

// SetWordWrapping sets the wrapping mode.
func (f TextFormat) SetWordWrapping(w WordWrapping) error {
	return f.CallHR("IDWriteTextFormat::SetWordWrapping", slotFormatSetWordWrapping, uintptr(w))
}

// FontWeight returns the format's weight.
func (f TextFormat) FontWeight() FontWeight {
	return FontWeight(uint32(f.Call(slotFormatGetFontWeight)))
}

var methodGetFontSize = com.Method[func(this com.Ptr) float32]{Slot: slotFormatGetFontSize}

// FontSize returns the format's size in DIPs (a FLOAT result).
func (f TextFormat) FontSize() float32 {
	if directFloats {
		return f.CallFloat(slotFormatGetFontSize)
	}
	return methodGetFontSize.Fn(f.Ptr)(f.Ptr)
}

// TextLayout is IDWriteTextLayout. It derives from IDWriteTextFormat, so
// the format's methods work on it (they answer for the whole text).
type TextLayout struct{ TextFormat }

var (
	methodGetMaxWidth  = com.Method[func(this com.Ptr) float32]{Slot: slotLayoutGetMaxWidth}
	methodGetMaxHeight = com.Method[func(this com.Ptr) float32]{Slot: slotLayoutGetMaxHeight}
)

// Metrics returns the metrics of the formatted text.
func (l TextLayout) Metrics() (TextMetrics, error) {
	var m TextMetrics
	err := l.CallHR("IDWriteTextLayout::GetMetrics", slotLayoutGetMetrics, uintptr(unsafe.Pointer(&m)))
	return m, err
}

// LineMetrics returns the metrics of every line (at least one, also for
// empty text).
func (l TextLayout) LineMetrics() ([]LineMetrics, error) {
	return fetchArray("IDWriteTextLayout::GetLineMetrics", func(buf *LineMetrics, n uint32, actual *uint32) com.HRESULT {
		return com.HRESULT(int32(l.Call(slotLayoutGetLineMetrics, uintptr(unsafe.Pointer(buf)), uintptr(n),
			uintptr(unsafe.Pointer(actual)))))
	})
}

// ClusterMetrics returns the metrics of every cluster in text order: the
// smallest units the caret moves over and a line breaks between (a base
// letter with its combining marks, a surrogate pair). A "\r\n" is two
// clusters, of which only the '\n' is a newline; the text's last cluster
// always has CanWrapLineAfter. The lengths, in UTF-16 units, add up to the
// text's length; empty text has no clusters.
func (l TextLayout) ClusterMetrics() ([]ClusterMetrics, error) {
	return fetchArray("IDWriteTextLayout::GetClusterMetrics", func(buf *ClusterMetrics, n uint32, actual *uint32) com.HRESULT {
		return com.HRESULT(int32(l.Call(slotLayoutGetClusterMetrics, uintptr(unsafe.Pointer(buf)), uintptr(n),
			uintptr(unsafe.Pointer(actual)))))
	})
}

// fetchArray reads an array out of a method that takes a buffer, its
// capacity and an out count. It asks with an empty buffer (nil) first; the
// "buffer too small" answer of that call carries the count. A "buffer too
// small" answer on the retry without a larger count is returned as the
// error, so a misbehaving method can't loop forever.
func fetchArray[T any](op string, call func(buf *T, n uint32, actual *uint32) com.HRESULT) ([]T, error) {
	var out []T
	for {
		var n uint32
		var p *T
		if len(out) > 0 {
			p = &out[0]
		}
		hr := call(p, uint32(len(out)), &n)
		switch {
		case hr == com.E_NOT_SUFFICIENT_BUFFER && int(n) > len(out):
			out = make([]T, n)
		case hr.Failed():
			return nil, hr.Err(op)
		default:
			return out[:n], nil
		}
	}
}

var methodSetMaxWidth = com.Method[func(this com.Ptr, w float32) com.HRESULT]{Slot: slotLayoutSetMaxWidth}

var methodHitTestPoint = com.Method[func(this com.Ptr, x, y float32, trailing, inside *int32,
	m *HitTestMetrics) com.HRESULT]{Slot: slotLayoutHitTestPoint}

// PointHit is the answer of HitTestPoint.
type PointHit struct {
	// Trailing is whether the point lies in the trailing half (in reading
	// direction) of the cluster at Metrics.TextPosition.
	Trailing bool
	// Inside is whether the point lies inside the formatted text; outside
	// it, the nearest position is reported.
	Inside bool
	// Metrics is the hit cluster's box in the layout's coordinates.
	Metrics HitTestMetrics
}

// HitTestTextPosition returns the caret point of the text position pos
// (its leading edge, or its trailing edge when trailing is set) in the
// layout's coordinates, and the box of the cluster holding pos. The FLOATs
// come back through pointers, so it takes the integer path.
func (l TextLayout) HitTestTextPosition(pos uint32, trailing bool) (x, y float32, m HitTestMetrics, err error) {
	err = l.CallHR("IDWriteTextLayout::HitTestTextPosition", slotLayoutHitTestTextPosition, uintptr(pos),
		boolArg(trailing), uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y)), uintptr(unsafe.Pointer(&m)))
	if err != nil {
		return 0, 0, HitTestMetrics{}, err
	}
	return x, y, m, nil
}

var methodHitTestTextRange = com.Method[func(this com.Ptr, pos, length uint32, originX, originY float32,
	m *HitTestMetrics, max uint32, actual *uint32) com.HRESULT]{Slot: slotLayoutHitTestTextRange}

func boolArg(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}
