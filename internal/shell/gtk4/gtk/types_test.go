package gtk

import (
	"reflect"
	"testing"

	"github.com/radix29/gogui/internal/ffi"
)

// The expected values are the same on amd64 and arm64 (LP64): gdkrgba.h,
// graphene-types.h, gskroundedrect.h, gdktypes.h (GdkRectangle), gtype.h,
// gvalue.h and pango-types.h (PangoRectangle). They run on every host; the
// mirrors have no build tags.

func TestRGBALayout(t *testing.T) {
	ffi.CheckLayout(t, RGBA{}, ffi.Layout{Size: 16, Align: 4, Fields: []ffi.Field{
		{Name: "R", Offset: 0, Size: 4}, {Name: "G", Offset: 4, Size: 4},
		{Name: "B", Offset: 8, Size: 4}, {Name: "A", Offset: 12, Size: 4},
	}})
	ffi.CheckNoGoPointers(t, RGBA{})
}

func TestGrapheneLayout(t *testing.T) {
	ffi.CheckLayout(t, GraphenePoint{}, ffi.Layout{Size: 8, Align: 4, Fields: []ffi.Field{
		{Name: "X", Offset: 0, Size: 4}, {Name: "Y", Offset: 4, Size: 4},
	}})
	ffi.CheckLayout(t, GrapheneSize{}, ffi.Layout{Size: 8, Align: 4, Fields: []ffi.Field{
		{Name: "Width", Offset: 0, Size: 4}, {Name: "Height", Offset: 4, Size: 4},
	}})
	ffi.CheckLayout(t, GrapheneRect{}, ffi.Layout{Size: 16, Align: 4, Fields: []ffi.Field{
		{Name: "Origin", Offset: 0, Size: 8}, {Name: "Size", Offset: 8, Size: 8},
	}})
	ffi.CheckNoGoPointers(t, GraphenePoint{})
	ffi.CheckNoGoPointers(t, GrapheneSize{})
	ffi.CheckNoGoPointers(t, GrapheneRect{})
}

func TestRoundedRectLayout(t *testing.T) {
	ffi.CheckLayout(t, RoundedRect{}, ffi.Layout{Size: 48, Align: 4, Fields: []ffi.Field{
		{Name: "Bounds", Offset: 0, Size: 16}, {Name: "Corner", Offset: 16, Size: 32},
	}})
	ffi.CheckNoGoPointers(t, RoundedRect{})
}

func TestColorStopLayout(t *testing.T) {
	ffi.CheckLayout(t, ColorStop{}, ffi.Layout{Size: 20, Align: 4, Fields: []ffi.Field{
		{Name: "Offset", Offset: 0, Size: 4}, {Name: "Color", Offset: 4, Size: 16},
	}})
	ffi.CheckNoGoPointers(t, ColorStop{})
}

// TestRectangleLayout checks the GdkRectangle mirror (S5.5e) and that
// Allocation is the same type, as GtkAllocation is GdkRectangle in C.
func TestRectangleLayout(t *testing.T) {
	ffi.CheckLayout(t, Rectangle{}, ffi.Layout{Size: 16, Align: 4, Fields: []ffi.Field{
		{Name: "X", Offset: 0, Size: 4}, {Name: "Y", Offset: 4, Size: 4},
		{Name: "Width", Offset: 8, Size: 4}, {Name: "Height", Offset: 12, Size: 4},
	}})
	ffi.CheckNoGoPointers(t, Rectangle{})
	if reflect.TypeFor[Allocation]() != reflect.TypeFor[Rectangle]() {
		t.Error("Allocation is not an alias of Rectangle")
	}
}

// TestPopoverEnums checks the GtkPositionType values and the style
// provider priorities of S5.5e and the GtkAlign and GtkTextDirection
// values of S5.5f (gtkenums.h, gtkstyleprovider.h).
func TestPopoverEnums(t *testing.T) {
	for _, c := range []struct {
		name      string
		got, want int64
	}{
		{"PosLeft", int64(PosLeft), 0}, {"PosRight", int64(PosRight), 1},
		{"PosTop", int64(PosTop), 2}, {"PosBottom", int64(PosBottom), 3},
		{"StyleProviderPriorityFallback", int64(StyleProviderPriorityFallback), 1},
		{"StyleProviderPriorityTheme", int64(StyleProviderPriorityTheme), 200},
		{"StyleProviderPrioritySettings", int64(StyleProviderPrioritySettings), 400},
		{"StyleProviderPriorityApplication", int64(StyleProviderPriorityApplication), 600},
		{"StyleProviderPriorityUser", int64(StyleProviderPriorityUser), 800},
		{"AlignStart", int64(AlignStart), 1}, {"TextDirLTR", int64(TextDirLTR), 1},
	} {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestTypeQueryLayout(t *testing.T) {
	ffi.CheckLayout(t, TypeQuery{}, ffi.Layout{Size: 24, Align: 8, Fields: []ffi.Field{
		{Name: "Type", Offset: 0, Size: 8}, {Name: "TypeName", Offset: 8, Size: 8},
		{Name: "ClassSize", Offset: 16, Size: 4}, {Name: "InstanceSize", Offset: 20, Size: 4},
	}})
	ffi.CheckNoGoPointers(t, TypeQuery{})
}

func TestGValueLayout(t *testing.T) {
	ffi.CheckLayout(t, GValue{}, ffi.Layout{Size: 24, Align: 8, Fields: []ffi.Field{
		{Name: "GType", Offset: 0, Size: 8}, {Name: "Data", Offset: 8, Size: 16},
	}})
	ffi.CheckNoGoPointers(t, GValue{})
}

func TestGErrorLayout(t *testing.T) {
	ffi.CheckLayout(t, GError{}, ffi.Layout{Size: 16, Align: 8, Fields: []ffi.Field{
		{Name: "Domain", Offset: 0, Size: 4}, {Name: "Code", Offset: 4, Size: 4},
		{Name: "Message", Offset: 8, Size: 8},
	}})
	ffi.CheckNoGoPointers(t, GError{})
}

func TestRect(t *testing.T) {
	r := Rect(1, 2, 3, 4)
	if r.Origin.X != 1 || r.Origin.Y != 2 || r.Size.Width != 3 || r.Size.Height != 4 {
		t.Errorf("Rect(1, 2, 3, 4) = %+v", r)
	}
}

// TestFundamentalTypes pins the GType constants to G_TYPE_MAKE_FUNDAMENTAL
// of their gtype.h numbers.
func TestFundamentalTypes(t *testing.T) {
	for _, c := range []struct {
		name string
		got  uintptr
		n    uintptr
	}{
		{"TypeInvalid", TypeInvalid, 0}, {"TypeBoolean", TypeBoolean, 5},
		{"TypeInt", TypeInt, 6}, {"TypeDouble", TypeDouble, 15},
		{"TypeString", TypeString, 16}, {"TypeObject", TypeObject, 20},
	} {
		if c.got != c.n<<2 {
			t.Errorf("%s = %d, want %d << 2", c.name, c.got, c.n)
		}
	}
}

func TestPangoRectangleLayout(t *testing.T) {
	ffi.CheckLayout(t, PangoRectangle{}, ffi.Layout{Size: 16, Align: 4, Fields: []ffi.Field{
		{Name: "X", Offset: 0, Size: 4}, {Name: "Y", Offset: 4, Size: 4},
		{Name: "Width", Offset: 8, Size: 4}, {Name: "Height", Offset: 12, Size: 4},
	}})
	ffi.CheckNoGoPointers(t, PangoRectangle{})
}

// TestPangoItemLayout: pango-glyph-item.h and pango-item.h (PangoItem with
// its PangoAnalysis inline, the two engine pointers kept for the ABI).
func TestPangoItemLayout(t *testing.T) {
	ffi.CheckLayout(t, GlyphItem{}, ffi.Layout{Size: 32, Align: 8, Fields: []ffi.Field{
		{Name: "Item", Offset: 0, Size: 8}, {Name: "Glyphs", Offset: 8, Size: 8},
		{Name: "YOffset", Offset: 16, Size: 4}, {Name: "StartXOffset", Offset: 20, Size: 4},
		{Name: "EndXOffset", Offset: 24, Size: 4},
	}})
	ffi.CheckLayout(t, PangoItem{}, ffi.Layout{Size: 64, Align: 8, Fields: []ffi.Field{
		{Name: "Offset", Offset: 0, Size: 4}, {Name: "Length", Offset: 4, Size: 4},
		{Name: "NumChars", Offset: 8, Size: 4}, {Name: "ShapeEngine", Offset: 16, Size: 8},
		{Name: "LangEngine", Offset: 24, Size: 8}, {Name: "Font", Offset: 32, Size: 8},
		{Name: "Level", Offset: 40, Size: 1}, {Name: "Gravity", Offset: 41, Size: 1},
		{Name: "Flags", Offset: 42, Size: 1}, {Name: "Script", Offset: 43, Size: 1},
		{Name: "Language", Offset: 48, Size: 8}, {Name: "ExtraAttrs", Offset: 56, Size: 8},
	}})
	ffi.CheckNoGoPointers(t, GlyphItem{})
	ffi.CheckNoGoPointers(t, PangoItem{})
}

// TestPangoAttributeLayout: gslist.h and pango-attributes.h (the
// PangoAttribute header, and PangoAttrClass with its type first), S5.3h;
// PangoAttrInt (the underline's value), S5.3i.
func TestPangoAttributeLayout(t *testing.T) {
	ffi.CheckLayout(t, GSList{}, ffi.Layout{Size: 16, Align: 8, Fields: []ffi.Field{
		{Name: "Data", Offset: 0, Size: 8}, {Name: "Next", Offset: 8, Size: 8},
	}})
	ffi.CheckLayout(t, PangoAttribute{}, ffi.Layout{Size: 16, Align: 8, Fields: []ffi.Field{
		{Name: "Klass", Offset: 0, Size: 8},
		{Name: "StartIndex", Offset: 8, Size: 4}, {Name: "EndIndex", Offset: 12, Size: 4},
	}})
	ffi.CheckLayout(t, PangoAttrClass{}, ffi.Layout{Size: 32, Align: 8, Fields: []ffi.Field{
		{Name: "Type", Offset: 0, Size: 4}, {Name: "Copy", Offset: 8, Size: 8},
		{Name: "Destroy", Offset: 16, Size: 8}, {Name: "Equal", Offset: 24, Size: 8},
	}})
	ffi.CheckNoGoPointers(t, GSList{})
	ffi.CheckNoGoPointers(t, PangoAttribute{})
	ffi.CheckNoGoPointers(t, PangoAttrClass{})
	ffi.CheckLayout(t, PangoAttrInt{}, ffi.Layout{Size: 24, Align: 8, Fields: []ffi.Field{
		{Name: "Attr", Offset: 0, Size: 16}, {Name: "Value", Offset: 16, Size: 4},
	}})
	ffi.CheckNoGoPointers(t, PangoAttrInt{})
}

// TestPangoAttrTypes pins the PangoAttrType values of pango-attributes.h
// (an enum counted from PANGO_ATTR_INVALID = 0) and the text-end index.
func TestPangoAttrTypes(t *testing.T) {
	if AttrForeground != 9 || AttrBackground != 10 || AttrUnderline != 11 {
		t.Errorf("AttrForeground, AttrBackground, AttrUnderline = %d, %d, %d, want 9, 10, 11",
			AttrForeground, AttrBackground, AttrUnderline)
	}
	if UnderlineNone != 0 {
		t.Errorf("UnderlineNone = %d, want 0", UnderlineNone)
	}
	if AttrIndexToTextEnd != 1<<32-1 {
		t.Errorf("AttrIndexToTextEnd = %#x, want G_MAXUINT", AttrIndexToTextEnd)
	}
}
