//go:build windows

package dwrite

import (
	"unsafe"

	"github.com/radix29/gogui/internal/shell/win32/com"
	"golang.org/x/sys/windows"
)

// Test-only bindings (review §4): nothing outside this package's tests
// calls them.

// FontFamilyName returns the family name the format was created with.
func (f TextFormat) FontFamilyName() (string, error) {
	n := uint32(f.Call(slotFormatGetFontFamilyNameLength))
	buf := make([]uint16, n+1) // the size passed includes the terminator
	if err := f.CallHR("IDWriteTextFormat::GetFontFamilyName", slotFormatGetFontFamilyName,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf), nil
}

// MaxWidth returns the layout box's width (a FLOAT result).
func (l TextLayout) MaxWidth() float32 {
	if directFloats {
		return l.CallFloat(slotLayoutGetMaxWidth)
	}
	return methodGetMaxWidth.Fn(l.Ptr)(l.Ptr)
}

// MaxHeight returns the layout box's height (a FLOAT result).
func (l TextLayout) MaxHeight() float32 {
	if directFloats {
		return l.CallFloat(slotLayoutGetMaxHeight)
	}
	return methodGetMaxHeight.Fn(l.Ptr)(l.Ptr)
}

// FileCount returns the number of files made with the loader so far.
func (l InMemoryFontFileLoader) FileCount() uint32 {
	return uint32(l.Call(slotMemLoaderGetFileCount))
}

// CanWrapLineAfter is whether a line may break after the cluster.
func (c ClusterMetrics) CanWrapLineAfter() bool { return c.Flags&clusterCanWrapLineAfter != 0 }

// IsWhitespace is whether the cluster is whitespace.
func (c ClusterMetrics) IsWhitespace() bool { return c.Flags&clusterIsWhitespace != 0 }

// IsNewline is whether the cluster is a line break ("\n", "\r\n", U+2028 …).
func (c ClusterMetrics) IsNewline() bool { return c.Flags&clusterIsNewline != 0 }

// IsSoftHyphen is whether the cluster is a soft hyphen (U+00AD).
func (c ClusterMetrics) IsSoftHyphen() bool { return c.Flags&clusterIsSoftHyphen != 0 }

// SetTextAlignment sets the horizontal alignment.
func (f TextFormat) SetTextAlignment(a TextAlignment) error {
	return f.CallHR("IDWriteTextFormat::SetTextAlignment", slotFormatSetTextAlignment, uintptr(a))
}

// SetParagraphAlignment sets the vertical alignment.
func (f TextFormat) SetParagraphAlignment(a ParagraphAlignment) error {
	return f.CallHR("IDWriteTextFormat::SetParagraphAlignment", slotFormatSetParagraphAlignment, uintptr(a))
}

// WordWrapping returns the wrapping mode.
func (f TextFormat) WordWrapping() WordWrapping {
	return WordWrapping(uint32(f.Call(slotFormatGetWordWrapping)))
}

// TextAlignment returns the horizontal alignment.
func (f TextFormat) TextAlignment() TextAlignment {
	return TextAlignment(uint32(f.Call(slotFormatGetTextAlignment)))
}

// ParagraphAlignment returns the vertical alignment.
func (f TextFormat) ParagraphAlignment() ParagraphAlignment {
	return ParagraphAlignment(uint32(f.Call(slotFormatGetParagraphAlignment)))
}

// FontStyle returns the format's style.
func (f TextFormat) FontStyle() FontStyle { return FontStyle(uint32(f.Call(slotFormatGetFontStyle))) }

// FontStretch returns the format's stretch.
func (f TextFormat) FontStretch() FontStretch {
	return FontStretch(uint32(f.Call(slotFormatGetFontStretch)))
}

// SetMaxWidth changes the layout box's width; the text is laid out again
// on the next query. A negative or NaN width is refused with E_INVALIDARG.
func (l TextLayout) SetMaxWidth(w float32) error {
	if directFloats {
		return l.CallHR("IDWriteTextLayout::SetMaxWidth", slotLayoutSetMaxWidth, com.Float(w))
	}
	return methodSetMaxWidth.Fn(l.Ptr)(l.Ptr, w).Err("IDWriteTextLayout::SetMaxWidth")
}

// HitTestPoint returns the text position at the point (x, y), in the
// layout's coordinates (DIPs, origin at the box's top left).
func (l TextLayout) HitTestPoint(x, y float32) (PointHit, error) {
	var trailing, inside int32
	var h PointHit
	var hr com.HRESULT
	if directFloats {
		hr = com.HRESULT(int32(l.Call(slotLayoutHitTestPoint, com.Float(x), com.Float(y), uintptr(unsafe.Pointer(&trailing)),
			uintptr(unsafe.Pointer(&inside)), uintptr(unsafe.Pointer(&h.Metrics)))))
	} else {
		hr = methodHitTestPoint.Fn(l.Ptr)(l.Ptr, x, y, &trailing, &inside, &h.Metrics)
	}
	if err := hr.Err("IDWriteTextLayout::HitTestPoint"); err != nil {
		return PointHit{}, err
	}
	h.Trailing, h.Inside = trailing != 0, inside != 0
	return h, nil
}

// HitTestTextRange returns the boxes covering the text range [pos,
// pos+length), offset by (originX, originY): at least one per line the
// range touches (more with bidi runs), in the layout's order.
func (l TextLayout) HitTestTextRange(pos, length uint32, originX, originY float32) ([]HitTestMetrics, error) {
	if directFloats {
		return fetchArray("IDWriteTextLayout::HitTestTextRange", func(buf *HitTestMetrics, n uint32, actual *uint32) com.HRESULT {
			return com.HRESULT(int32(l.Call(slotLayoutHitTestTextRange, uintptr(pos), uintptr(length), com.Float(originX),
				com.Float(originY), uintptr(unsafe.Pointer(buf)), uintptr(n), uintptr(unsafe.Pointer(actual)))))
		})
	}
	fn := methodHitTestTextRange.Fn(l.Ptr)
	return fetchArray("IDWriteTextLayout::HitTestTextRange", func(buf *HitTestMetrics, n uint32, actual *uint32) com.HRESULT {
		return fn(l.Ptr, pos, length, originX, originY, buf, n, actual)
	})
}

// Analyze reads the file's header: whether DirectWrite supports it, its
// file and face types and the number of faces (more than one for a TTC).
// Unsupported data is no error, only Supported false.
func (f FontFile) Analyze() (FontFileAnalysis, error) {
	var supported int32
	var a FontFileAnalysis
	err := f.CallHR("IDWriteFontFile::Analyze", slotFontFileAnalyze, uintptr(unsafe.Pointer(&supported)),
		uintptr(unsafe.Pointer(&a.FileType)), uintptr(unsafe.Pointer(&a.FaceType)), uintptr(unsafe.Pointer(&a.FaceCount)))
	if err != nil {
		return FontFileAnalysis{}, err
	}
	a.Supported = supported != 0
	return a, nil
}
