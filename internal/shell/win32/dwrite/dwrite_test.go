//go:build windows

package dwrite

import (
	"errors"
	"math"
	"os"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/win32/com"
)

// The vtables of dwrite.h in declaration order (no SDK headers on the
// host; the order is the SDK's as mirrored by independent bindings, and
// every bound slot is checked against the live objects below). The C++
// header's `using` declarations and inline overloads take no slot.
// IDWriteFactory1–5 come from dwrite_1.h–dwrite_3.h; the overload pairs
// declared in one interface are listed in MSVC's reverse declaration
// order, with a suffix. Only IDWriteFactory's slots are bound so far; the
// in-memory font slots of IDWriteFactory3/5 are bound and checked live in
// S2.5c.
var (
	vtFactory = com.IUnknownVtable.Extend("GetSystemFontCollection", "CreateCustomFontCollection",
		"RegisterFontCollectionLoader", "UnregisterFontCollectionLoader", "CreateFontFileReference",
		"CreateCustomFontFileReference", "CreateFontFace", "CreateRenderingParams",
		"CreateMonitorRenderingParams", "CreateCustomRenderingParams", "RegisterFontFileLoader",
		"UnregisterFontFileLoader", "CreateTextFormat", "CreateTypography", "GetGdiInterop",
		"CreateTextLayout", "CreateGdiCompatibleTextLayout", "CreateEllipsisTrimmingSign",
		"CreateTextAnalyzer", "CreateNumberSubstitution", "CreateGlyphRunAnalysis")
	vtFactory1 = vtFactory.Extend("GetEudcFontCollection", "CreateCustomRenderingParams1")
	vtFactory2 = vtFactory1.Extend("GetSystemFontFallback", "CreateFontFallbackBuilder",
		"TranslateColorGlyphRun", "CreateCustomRenderingParams2", "CreateGlyphRunAnalysis2")
	vtFactory3 = vtFactory2.Extend("CreateGlyphRunAnalysis3", "CreateCustomRenderingParams3",
		"CreateFontFaceReference(path)", "CreateFontFaceReference(file)", "GetSystemFontSet",
		"CreateFontSetBuilder", "CreateFontCollectionFromFontSet", "GetSystemFontCollection3",
		"GetFontDownloadQueue")
	vtFactory4 = vtFactory3.Extend("TranslateColorGlyphRun4", "ComputeGlyphOrigins(transform)",
		"ComputeGlyphOrigins")
	vtFactory5 = vtFactory4.Extend("CreateFontSetBuilder5", "CreateInMemoryFontFileLoader",
		"CreateHttpFontFileLoader", "AnalyzeContainerType", "UnpackFontFile")

	vtFontCollection = com.IUnknownVtable.Extend("GetFontFamilyCount", "GetFontFamily",
		"FindFamilyName", "GetFontFromFontFace")

	vtTextFormat = com.IUnknownVtable.Extend("SetTextAlignment", "SetParagraphAlignment",
		"SetWordWrapping", "SetReadingDirection", "SetFlowDirection", "SetIncrementalTabStop",
		"SetTrimming", "SetLineSpacing", "GetTextAlignment", "GetParagraphAlignment",
		"GetWordWrapping", "GetReadingDirection", "GetFlowDirection", "GetIncrementalTabStop",
		"GetTrimming", "GetLineSpacing", "GetFontCollection", "GetFontFamilyNameLength",
		"GetFontFamilyName", "GetFontWeight", "GetFontStyle", "GetFontStretch", "GetFontSize",
		"GetLocaleNameLength", "GetLocaleName")
	vtTextLayout = vtTextFormat.Extend("SetMaxWidth", "SetMaxHeight", "SetFontCollection",
		"SetFontFamilyName", "SetFontWeight", "SetFontStyle", "SetFontStretch", "SetFontSize",
		"SetUnderline", "SetStrikethrough", "SetDrawingEffect", "SetInlineObject", "SetTypography",
		"SetLocaleName", "GetMaxWidth", "GetMaxHeight", "GetFontCollection(range)",
		"GetFontFamilyNameLength(range)", "GetFontFamilyName(range)", "GetFontWeight(range)",
		"GetFontStyle(range)", "GetFontStretch(range)", "GetFontSize(range)", "GetUnderline",
		"GetStrikethrough", "GetDrawingEffect", "GetInlineObject", "GetTypography",
		"GetLocaleNameLength(range)", "GetLocaleName(range)", "Draw", "GetLineMetrics",
		"GetMetrics", "GetOverhangMetrics", "GetClusterMetrics", "DetermineMinWidth",
		"HitTestPoint", "HitTestTextPosition", "HitTestTextRange")
)

func TestSlots(t *testing.T) {
	if len(vtFactory) != 24 || len(vtFactory5) != 48 || len(vtFontCollection) != 7 ||
		len(vtTextFormat) != 28 || len(vtTextLayout) != 67 {
		t.Errorf("vtable lengths %d, %d, %d, %d, %d; want 24, 48, 7, 28, 67", len(vtFactory),
			len(vtFactory5), len(vtFontCollection), len(vtTextFormat), len(vtTextLayout))
	}
	com.CheckSlots(t, "IDWriteFactory5", vtFactory5, map[string]int{
		"GetSystemFontCollection": slotFactoryGetSystemFontCollection,
		"CreateTextFormat":        slotFactoryCreateTextFormat,
		"CreateTextLayout":        slotFactoryCreateTextLayout,
	})
	com.CheckSlots(t, "IDWriteFontCollection", vtFontCollection, map[string]int{
		"GetFontFamilyCount": slotCollectionGetFontFamilyCount,
	})
	format := map[string]int{
		"SetTextAlignment":        slotFormatSetTextAlignment,
		"SetParagraphAlignment":   slotFormatSetParagraphAlignment,
		"SetWordWrapping":         slotFormatSetWordWrapping,
		"GetTextAlignment":        slotFormatGetTextAlignment,
		"GetParagraphAlignment":   slotFormatGetParagraphAlignment,
		"GetWordWrapping":         slotFormatGetWordWrapping,
		"GetFontFamilyNameLength": slotFormatGetFontFamilyNameLength,
		"GetFontFamilyName":       slotFormatGetFontFamilyName,
		"GetFontWeight":           slotFormatGetFontWeight,
		"GetFontStyle":            slotFormatGetFontStyle,
		"GetFontStretch":          slotFormatGetFontStretch,
		"GetFontSize":             slotFormatGetFontSize,
	}
	com.CheckSlots(t, "IDWriteTextFormat", vtTextFormat, format)
	// A layout is a format too: the same slots through the derived table.
	com.CheckSlots(t, "IDWriteTextLayout (format part)", vtTextLayout, format)
	com.CheckSlots(t, "IDWriteTextLayout", vtTextLayout, map[string]int{
		"GetMaxWidth":         slotLayoutGetMaxWidth,
		"GetMaxHeight":        slotLayoutGetMaxHeight,
		"SetMaxWidth":         slotLayoutSetMaxWidth,
		"GetLineMetrics":      slotLayoutGetLineMetrics,
		"GetMetrics":          slotLayoutGetMetrics,
		"GetClusterMetrics":   slotLayoutGetClusterMetrics,
		"HitTestPoint":        slotLayoutHitTestPoint,
		"HitTestTextPosition": slotLayoutHitTestTextPosition,
		"HitTestTextRange":    slotLayoutHitTestTextRange,
	})
}

func f(name string, off, size uintptr) ffi.Field {
	return ffi.Field{Name: name, Offset: off, Size: size}
}

// The layouts come from dwrite.h; amd64 and arm64 are both LLP64 (arm64
// runs through native.sh).
func TestStructLayouts(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Fatal("the layout tables are for 64-bit Windows only")
	}
	for _, c := range []struct {
		v    any
		want ffi.Layout
	}{
		{TextMetrics{}, ffi.Layout{Size: 36, Align: 4, Fields: []ffi.Field{
			f("Left", 0, 4), f("Top", 4, 4), f("Width", 8, 4),
			f("WidthIncludingTrailingWhitespace", 12, 4), f("Height", 16, 4),
			f("LayoutWidth", 20, 4), f("LayoutHeight", 24, 4), f("MaxBidiReorderingDepth", 28, 4),
			f("LineCount", 32, 4)}}},
		{LineMetrics{}, ffi.Layout{Size: 24, Align: 4, Fields: []ffi.Field{
			f("Length", 0, 4), f("TrailingWhitespaceLength", 4, 4), f("NewlineLength", 8, 4),
			f("Height", 12, 4), f("Baseline", 16, 4), f("IsTrimmed", 20, 4)}}},
		{HitTestMetrics{}, ffi.Layout{Size: 36, Align: 4, Fields: []ffi.Field{
			f("TextPosition", 0, 4), f("Length", 4, 4), f("Left", 8, 4), f("Top", 12, 4),
			f("Width", 16, 4), f("Height", 20, 4), f("BidiLevel", 24, 4), f("IsText", 28, 4),
			f("IsTrimmed", 32, 4)}}},
		{ClusterMetrics{}, ffi.Layout{Size: 8, Align: 4, Fields: []ffi.Field{
			f("Width", 0, 4), f("Length", 4, 2), f("Flags", 6, 2)}}},
	} {
		ffi.CheckLayout(t, c.v, c.want)
		ffi.CheckNoGoPointers(t, c.v)
	}
}

func TestProcsResolve(t *testing.T) {
	src, err := os.ReadFile("dwrite.go")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(src), "newProc(") - 1; len(allProcs) != n || n == 0 {
		t.Fatalf("%d procs registered, %d newProc calls in dwrite.go", len(allProcs), n)
	}
	for _, p := range allProcs {
		if err := p.Find(); err != nil {
			t.Errorf("proc %s: %v", p.Name, err)
		}
	}
}

func TestErrNoFactory5(t *testing.T) {
	if !strings.Contains(ErrNoFactory5.Error(), "Windows 10 1703") {
		t.Errorf("ErrNoFactory5 = %q, does not name Windows 10 1703", ErrNoFactory5)
	}
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

func u16(s string) []uint16 { return windows.StringToUTF16(s)[:len(windows.StringToUTF16(s))-1] }

func near(got, want, tol float32) bool { return float32(math.Abs(float64(got-want))) <= tol }

// TestFactory checks the shared factory: one object per process, every
// IID up to IDWriteFactory5, and the system collection.
func TestFactory(t *testing.T) {
	fa, err := CreateFactory()
	if err != nil {
		t.Fatal(err)
	}
	fb, err := CreateFactory()
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "factory", fa.Ptr, &IID_IDWriteFactory, &IID_IDWriteFactory1, &IID_IDWriteFactory2,
		&IID_IDWriteFactory3, &IID_IDWriteFactory4, &IID_IDWriteFactory5, &com.IID_IUnknown)
	// The shared factory is one object: same IUnknown identity.
	ua, err := fa.QueryInterface(&com.IID_IUnknown)
	if err != nil {
		t.Fatal(err)
	}
	ub, err := fb.QueryInterface(&com.IID_IUnknown)
	if err != nil {
		t.Fatal(err)
	}
	if ua != ub {
		t.Errorf("two shared factories: IUnknown %#x and %#x", ua, ub)
	}
	ua.Release()
	ub.Release()
	n := fa.RefCount()
	fb.Release()
	if m := fa.RefCount(); m != n-1 {
		t.Errorf("Release of the second factory: count %d -> %d", n, m)
	}

	sys, err := fa.SystemFontCollection(false)
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "system collection", sys.Ptr, &IID_IDWriteFontCollection)
	if c := sys.FamilyCount(); c < 10 {
		t.Errorf("system collection: %d families", c)
	} else {
		t.Logf("system collection: %d families", c)
	}
	sys.Release()
	fa.Release()
}

// TestFormat checks a Segoe UI format: the float path of CreateTextFormat
// (the size and the arguments after it), the getters and setters.
func TestFormat(t *testing.T) {
	fac, err := CreateFactory()
	if err != nil {
		t.Fatal(err)
	}
	defer fac.Release()

	tf, err := fac.CreateTextFormat("Segoe UI", FontCollection{}, FONT_WEIGHT_BOLD, FONT_STYLE_ITALIC,
		FONT_STRETCH_CONDENSED, 13, "en-us")
	if err != nil {
		t.Fatal(err)
	}
	queries(t, "format", tf.Ptr, &IID_IDWriteTextFormat)
	if s := tf.FontSize(); s != 13 {
		t.Errorf("FontSize %v, want 13", s)
	}
	if w, s, st := tf.FontWeight(), tf.FontStyle(), tf.FontStretch(); w != FONT_WEIGHT_BOLD ||
		s != FONT_STYLE_ITALIC || st != FONT_STRETCH_CONDENSED {
		t.Errorf("weight %d, style %d, stretch %d; want 700, 2, 3", w, s, st)
	}
	if name, err := tf.FontFamilyName(); err != nil || name != "Segoe UI" {
		t.Errorf("FontFamilyName = %q, %v", name, err)
	}
	// Defaults, then round trips.
	if tf.WordWrapping() != WORD_WRAPPING_WRAP || tf.TextAlignment() != TEXT_ALIGNMENT_LEADING ||
		tf.ParagraphAlignment() != PARAGRAPH_ALIGNMENT_NEAR {
		t.Errorf("defaults: wrap %d, align %d, paragraph %d", tf.WordWrapping(), tf.TextAlignment(), tf.ParagraphAlignment())
	}
	for _, w := range []WordWrapping{WORD_WRAPPING_NO_WRAP, WORD_WRAPPING_CHARACTER, WORD_WRAPPING_WRAP} {
		if err := tf.SetWordWrapping(w); err != nil || tf.WordWrapping() != w {
			t.Errorf("SetWordWrapping(%d): %v, read back %d", w, err, tf.WordWrapping())
		}
	}
	for _, a := range []TextAlignment{TEXT_ALIGNMENT_CENTER, TEXT_ALIGNMENT_JUSTIFIED, TEXT_ALIGNMENT_TRAILING} {
		if err := tf.SetTextAlignment(a); err != nil || tf.TextAlignment() != a {
			t.Errorf("SetTextAlignment(%d): %v, read back %d", a, err, tf.TextAlignment())
		}
	}
	for _, a := range []ParagraphAlignment{PARAGRAPH_ALIGNMENT_CENTER, PARAGRAPH_ALIGNMENT_FAR} {
		if err := tf.SetParagraphAlignment(a); err != nil || tf.ParagraphAlignment() != a {
			t.Errorf("SetParagraphAlignment(%d): %v, read back %d", a, err, tf.ParagraphAlignment())
		}
	}
	// Out-of-range values are refused and leave the value alone.
	if err := tf.SetWordWrapping(99); !errors.Is(err, com.E_INVALIDARG) || tf.WordWrapping() != WORD_WRAPPING_WRAP {
		t.Errorf("SetWordWrapping(99) = %v, read back %d", err, tf.WordWrapping())
	}
	if err := tf.SetTextAlignment(99); !errors.Is(err, com.E_INVALIDARG) {
		t.Errorf("SetTextAlignment(99) = %v", err)
	}
	if err := tf.SetParagraphAlignment(99); !errors.Is(err, com.E_INVALIDARG) {
		t.Errorf("SetParagraphAlignment(99) = %v", err)
	}
	release(t, "format", &tf.Ptr, 0)

	// A size of 0 or below is refused; a NUL in a name is a Go error.
	if _, err := fac.CreateTextFormat("Segoe UI", FontCollection{}, FONT_WEIGHT_NORMAL, FONT_STYLE_NORMAL,
		FONT_STRETCH_NORMAL, -1, "en-us"); !errors.Is(err, com.E_INVALIDARG) {
		t.Errorf("size -1: %v", err)
	}
	if _, err := fac.CreateTextFormat("Segoe\x00UI", FontCollection{}, FONT_WEIGHT_NORMAL, FONT_STYLE_NORMAL,
		FONT_STRETCH_NORMAL, 13, "en-us"); err == nil {
		t.Error("family with NUL: no error")
	}
	// An unknown family is fine for the format (fallback happens in layout).
	unk, err := fac.CreateTextFormat("No Such Family gogui", FontCollection{}, FONT_WEIGHT_NORMAL,
		FONT_STYLE_NORMAL, FONT_STRETCH_NORMAL, 13, "en-us")
	if err != nil {
		t.Errorf("unknown family: %v", err)
	}
	unk.Release()
}

// layout makes a Segoe UI layout of text at size with weight in a w × h box.
func layout(t *testing.T, fac Factory, text string, size float32, weight FontWeight, wrap WordWrapping, w, h float32) TextLayout {
	t.Helper()
	tf, err := fac.CreateTextFormat("Segoe UI", FontCollection{}, weight, FONT_STYLE_NORMAL,
		FONT_STRETCH_NORMAL, size, "en-us")
	if err != nil {
		t.Fatal(err)
	}
	defer tf.Release()
	if err := tf.SetWordWrapping(wrap); err != nil {
		t.Fatal(err)
	}
	l, err := fac.CreateTextLayout(u16(text), tf, w, h)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func metrics(t *testing.T, l TextLayout) (TextMetrics, []LineMetrics) {
	t.Helper()
	m, err := l.Metrics()
	if err != nil {
		t.Fatal(err)
	}
	lines, err := l.LineMetrics()
	if err != nil {
		t.Fatal(err)
	}
	if int(m.LineCount) != len(lines) {
		t.Errorf("LineCount %d, %d line metrics", m.LineCount, len(lines))
	}
	return m, lines
}

// TestLayout measures text in Segoe UI: "Hello" at 13 DIPs on one line
// with the font's line height (Segoe UI's ascent plus descent is 1.33 em),
// two lines through "\n", empty text, scaling with the size, a bold weight,
// and wrapping.
func TestLayout(t *testing.T) {
	fac, err := CreateFactory()
	if err != nil {
		t.Fatal(err)
	}
	defer fac.Release()
	const size = 13

	hello := layout(t, fac, "Hello", size, FONT_WEIGHT_NORMAL, WORD_WRAPPING_WRAP, 1000, 500)
	queries(t, "layout", hello.Ptr, &IID_IDWriteTextLayout, &IID_IDWriteTextFormat)
	if w, h := hello.MaxWidth(), hello.MaxHeight(); w != 1000 || h != 500 {
		t.Errorf("MaxWidth/MaxHeight %v, %v; want 1000, 500", w, h)
	}
	if s := hello.FontSize(); s != size {
		t.Errorf("layout FontSize %v", s)
	}
	m, lines := metrics(t, hello)
	t.Logf("Hello at %d: %+v, line %+v", size, m, lines)
	if m.LineCount != 1 || m.Width <= 0 || m.Width > 60 || m.Left != 0 || m.Top != 0 ||
		m.LayoutWidth != 1000 || m.LayoutHeight != 500 {
		t.Errorf("Hello: %+v", m)
	}
	if r := m.Height / size; r < 1.30 || r > 1.36 {
		t.Errorf("Hello: height %v = %v × size, want near 1.33", m.Height, r)
	}
	if len(lines) == 1 {
		l := lines[0]
		if l.Length != 5 || l.NewlineLength != 0 || l.TrailingWhitespaceLength != 0 ||
			l.Height != m.Height || l.Baseline <= size/2 || l.Baseline >= l.Height || l.IsTrimmed != 0 {
			t.Errorf("Hello line: %+v", l)
		}
	}

	two := layout(t, fac, "Hello\nWorld", size, FONT_WEIGHT_NORMAL, WORD_WRAPPING_WRAP, 1000, 500)
	m2, lines2 := metrics(t, two)
	if m2.LineCount != 2 || !near(m2.Height, 2*m.Height, 0.01) {
		t.Errorf("two lines: %+v (one line %v high)", m2, m.Height)
	}
	if len(lines2) == 2 && (lines2[0].Length != 6 || lines2[0].NewlineLength != 1 ||
		lines2[0].TrailingWhitespaceLength != 1 || lines2[1].Length != 5 || lines2[1].NewlineLength != 0) {
		t.Errorf("two lines: %+v", lines2)
	}

	empty := layout(t, fac, "", size, FONT_WEIGHT_NORMAL, WORD_WRAPPING_WRAP, 1000, 500)
	me, le := metrics(t, empty)
	if me.LineCount != 1 || me.Width != 0 || !near(me.Height, m.Height, 0.01) || len(le) != 1 || le[0].Length != 0 {
		t.Errorf("empty: %+v, %+v", me, le)
	}

	// Twice the size: twice the width and height (the FLOAT size arrives).
	big := layout(t, fac, "Hello", 2*size, FONT_WEIGHT_NORMAL, WORD_WRAPPING_WRAP, 1000, 500)
	mb, _ := metrics(t, big)
	if !near(mb.Width, 2*m.Width, 0.5) || !near(mb.Height, 2*m.Height, 0.05) {
		t.Errorf("size %d: %v × %v, at %d: %v × %v", 2*size, mb.Width, mb.Height, size, m.Width, m.Height)
	}
	// Bold is wider (the arguments before the FLOAT arrive).
	bold := layout(t, fac, "Hello", size, FONT_WEIGHT_BOLD, WORD_WRAPPING_WRAP, 1000, 500)
	mbo, _ := metrics(t, bold)
	if mbo.Width <= m.Width {
		t.Errorf("bold width %v, normal %v", mbo.Width, m.Width)
	}

	// Wrapping at a space in a narrow box (the FLOAT max width arrives),
	// none with NO_WRAP; the trailing space stays out of Width.
	wrap := layout(t, fac, "Hello World", size, FONT_WEIGHT_NORMAL, WORD_WRAPPING_WRAP, 1.5*m.Width, 500)
	mw, lw := metrics(t, wrap)
	if mw.LineCount != 2 || len(lw) != 2 || lw[0].Length != 6 || lw[0].TrailingWhitespaceLength != 1 ||
		mw.Width > 1.5*m.Width || mw.WidthIncludingTrailingWhitespace < mw.Width {
		t.Errorf("wrapped: %+v, %+v", mw, lw)
	}
	nowrap := layout(t, fac, "Hello World", size, FONT_WEIGHT_NORMAL, WORD_WRAPPING_NO_WRAP, 1.5*m.Width, 500)
	mn, _ := metrics(t, nowrap)
	if mn.LineCount != 1 || mn.Width <= 1.5*m.Width {
		t.Errorf("no wrap: %+v", mn)
	}
	// Centered in the box: Left moves to the middle.
	if err := nowrap.SetTextAlignment(TEXT_ALIGNMENT_CENTER); err != nil {
		t.Fatal(err)
	}
	if err := nowrap.SetParagraphAlignment(PARAGRAPH_ALIGNMENT_CENTER); err != nil {
		t.Fatal(err)
	}
	mc, _ := metrics(t, nowrap)
	if !near(mc.Left, (1.5*m.Width-mc.Width)/2, 0.5) || !near(mc.Top, (500-mc.Height)/2, 0.5) {
		t.Errorf("centered: %+v", mc)
	}

	for _, l := range []*TextLayout{&hello, &two, &empty, &big, &bold, &wrap, &nowrap} {
		release(t, "layout", &l.Ptr, 0)
	}
}
