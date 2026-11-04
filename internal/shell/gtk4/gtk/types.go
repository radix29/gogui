package gtk

import (
	"math"
	"structs"
	"unsafe"

	"github.com/radix29/gogui/internal/ffi"
)

// Go mirrors of the C structs the shell passes to GTK, GSK and GObject.
// They go to C only as pointers to Go values that contain no Go pointers,
// valid for the call (GTK copies them), or are read field by field from
// native memory. No struct crosses a callback (PLAN.md S0.9).
// types_test.go checks the layouts, which are the same on amd64 and arm64.

// RGBA mirrors GdkRGBA: four floats, 16 bytes.
type RGBA struct {
	_          structs.HostLayout
	R, G, B, A float32
}

// GraphenePoint mirrors graphene_point_t.
type GraphenePoint struct {
	_    structs.HostLayout
	X, Y float32
}

// GrapheneSize mirrors graphene_size_t.
type GrapheneSize struct {
	_             structs.HostLayout
	Width, Height float32
}

// GrapheneRect mirrors graphene_rect_t: 16 bytes, float-aligned (unlike
// graphene's vector types it has no SIMD alignment).
type GrapheneRect struct {
	_      structs.HostLayout
	Origin GraphenePoint
	Size   GrapheneSize
}

// RoundedRect mirrors GskRoundedRect: the bounds and the four corner
// radii (top-left, top-right, bottom-right, bottom-left), 48 bytes.
type RoundedRect struct {
	_      structs.HostLayout
	Bounds GrapheneRect
	Corner [4]GrapheneSize
}

// ColorStop mirrors GskColorStop: an offset and a GdkRGBA, 20 bytes.
type ColorStop struct {
	_      structs.HostLayout
	Offset float32
	Color  RGBA
}

// MatrixSize is sizeof(graphene_matrix_t): 16 floats, row-major, with
// graphene's row-vector convention (the translation is the last row). Its
// SIMD alignment is 16, which Go can't promise for its own values.
const MatrixSize = 64

// Allocation mirrors GtkAllocation (GdkRectangle): four C ints, 16 bytes.
type Allocation struct {
	_                   structs.HostLayout
	X, Y, Width, Height int32
}

// PangoRectangle mirrors PangoRectangle: four C ints in Pango units, 16
// bytes (S4.2h: the iterator's extents).
type PangoRectangle struct {
	_                   structs.HostLayout
	X, Y, Width, Height int32
}

// GlyphItem mirrors the head of PangoGlyphItem (a PangoLayoutRun): its
// PangoItem and glyph string, then three ints, 32 bytes (S4.2h; read
// from native memory, never written).
type GlyphItem struct {
	_                                 structs.HostLayout
	Item, Glyphs                      uintptr
	YOffset, StartXOffset, EndXOffset int32
}

// PangoItem mirrors PangoItem with its PangoAnalysis inline, 64 bytes
// (S4.2h; read from native memory). Level is the bidi embedding level:
// odd is right to left.
type PangoItem struct {
	_                             structs.HostLayout
	Offset, Length, NumChars      int32
	ShapeEngine, LangEngine, Font uintptr
	Level, Gravity, Flags, Script uint8
	Language, ExtraAttrs          uintptr
}

// RunLevel returns the bidi level of the PangoLayoutRun run (native
// memory): its item's analysis.level.
func RunLevel(run uintptr) uint8 {
	item := ffi.ReadUintptr(run, unsafe.Offsetof(GlyphItem{}.Item))
	// The int32 holding level, gravity, flags and script; level is its low
	// byte on the little-endian targets.
	return uint8(ffi.ReadInt32(item, unsafe.Offsetof(PangoItem{}.Level)))
}

// TypeQuery mirrors GTypeQuery: 24 bytes on 64-bit.
type TypeQuery struct {
	_            structs.HostLayout
	Type         uintptr // GType
	TypeName     uintptr // const gchar *, native memory
	ClassSize    uint32
	InstanceSize uint32
}

// GValue mirrors GValue: the GType and a two-word data union, 24 bytes.
// A GValue that native code may hold while it calls back into Go
// (g_signal_emitv) lives in g_malloc0 memory; the mirror gives the stride.
type GValue struct {
	_     structs.HostLayout
	GType uintptr
	Data  [2]uint64
}

// GError mirrors GError (S4.2j): its domain (a GQuark), code and message
// (a NUL-terminated string in native memory), 16 bytes. gogui only reads
// one that GLib allocated, through its offsets.
type GError struct {
	_       structs.HostLayout
	Domain  uint32
	Code    int32
	Message uintptr
}

// Rect returns the graphene_rect_t (x, y, w, h).
func Rect(x, y, w, h float32) GrapheneRect {
	return GrapheneRect{Origin: GraphenePoint{X: x, Y: y}, Size: GrapheneSize{Width: w, Height: h}}
}

// ReadFloat32 reads the C float at p+off (native memory, 4-byte aligned).
func ReadFloat32(p, off uintptr) float32 {
	return math.Float32frombits(uint32(ffi.ReadInt32(p, off)))
}

// ReadRGBA copies the GdkRGBA at p (native memory).
func ReadRGBA(p uintptr) RGBA {
	return RGBA{R: ReadFloat32(p, 0), G: ReadFloat32(p, 4), B: ReadFloat32(p, 8), A: ReadFloat32(p, 12)}
}

// ReadRect copies the graphene_rect_t at p+off (native memory).
func ReadRect(p, off uintptr) GrapheneRect {
	return Rect(ReadFloat32(p, off), ReadFloat32(p, off+4), ReadFloat32(p, off+8), ReadFloat32(p, off+12))
}

// ReadRoundedRect copies the GskRoundedRect at p (native memory).
func ReadRoundedRect(p uintptr) RoundedRect {
	r := RoundedRect{Bounds: ReadRect(p, 0)}
	for i := range r.Corner {
		off := 16 + uintptr(i)*8
		r.Corner[i] = GrapheneSize{Width: ReadFloat32(p, off), Height: ReadFloat32(p, off+4)}
	}
	return r
}

// Fundamental GTypes (G_TYPE_MAKE_FUNDAMENTAL(n) = n << 2, gtype.h).
const (
	TypeInvalid uintptr = 0
	TypeBoolean uintptr = 5 << 2
	TypeInt     uintptr = 6 << 2
	TypeDouble  uintptr = 15 << 2
	TypeString  uintptr = 16 << 2
	TypeObject  uintptr = 20 << 2
)

// gboolean values.
const (
	False int32 = 0
	True  int32 = 1
)

// GSourceFunc results (gmain.h).
const (
	SourceRemove   int32 = 0
	SourceContinue int32 = 1
)

// GConnectFlags (gsignal.h).
const (
	ConnectAfter   uint32 = 1 << 0
	ConnectSwapped uint32 = 1 << 1
)

// GdkToplevelState flags (gdktoplevel.h), the ones the shell reads.
const (
	ToplevelStateMinimized uint32 = 1 << 0
	ToplevelStateMaximized uint32 = 1 << 1
	ToplevelStateSuspended uint32 = 1 << 16 // GTK 4.12+
)

// GtkEventSequenceState (gtkenums.h).
const (
	EventSequenceNone    int32 = 0
	EventSequenceClaimed int32 = 1
	EventSequenceDenied  int32 = 2
)

// GtkEventControllerScrollFlags (gtkenums.h).
const (
	ScrollVertical   uint32 = 1 << 0
	ScrollHorizontal uint32 = 1 << 1
	ScrollDiscrete   uint32 = 1 << 2
	ScrollKinetic    uint32 = 1 << 3
	ScrollBothAxes          = ScrollVertical | ScrollHorizontal
)

// GtkPropagationPhase.
const (
	PhaseNone    int32 = 0
	PhaseCapture int32 = 1
	PhaseBubble  int32 = 2
	PhaseTarget  int32 = 3
)

// ButtonPrimary is GDK_BUTTON_PRIMARY.
const ButtonPrimary uint32 = 1

// GtkOrientation.
const (
	OrientationHorizontal int32 = 0
	OrientationVertical   int32 = 1
)

// GskRenderNodeType values (gskenums.h, stable since 4.0), for the
// node-tree tests.
const (
	NodeContainer      int32 = 1
	NodeColor          int32 = 3
	NodeLinearGradient int32 = 4
	NodeBorder         int32 = 9
	NodeTexture        int32 = 10
	NodeInsetShadow    int32 = 11
	NodeOutsetShadow   int32 = 12
	NodeTransform      int32 = 13
	NodeOpacity        int32 = 14
	NodeClip           int32 = 17
	NodeRoundedClip    int32 = 18
	NodeBlur           int32 = 23
	NodeDebug          int32 = 24
	NodeTextureScale   int32 = 26 // GTK 4.10
	NodeFill           int32 = 28 // GTK 4.14
	NodeStroke         int32 = 29 // GTK 4.14
)

// GskLineCap, GskLineJoin and GskFillRule (gskenums.h, GTK 4.14), for
// GskStroke and the fill calls (S4.2c).
const (
	LineCapButt   int32 = 0
	LineCapRound  int32 = 1
	LineCapSquare int32 = 2

	LineJoinMiter int32 = 0
	LineJoinRound int32 = 1
	LineJoinBevel int32 = 2

	FillRuleWinding int32 = 0
	FillRuleEvenOdd int32 = 1
)

// GskScalingFilter (gskenums.h, since 4.0) and the GdkMemoryFormat of the
// shell's images (gdkenums.h: straight-alpha RGBA bytes), for S4.2e.
const (
	ScalingFilterLinear    int32 = 0
	ScalingFilterNearest   int32 = 1
	ScalingFilterTrilinear int32 = 2

	MemoryR8G8B8A8 int32 = 5
)

// Pango's scale and the enums of the text system (pango-types.h,
// pango-font.h, pango-layout.h), for S4.2g.
const (
	PangoScale = 1024 // Pango units per logical pixel

	PangoStyleNormal  int32 = 0
	PangoStyleOblique int32 = 1
	PangoStyleItalic  int32 = 2

	PangoWrapWord     int32 = 0
	PangoWrapChar     int32 = 1
	PangoWrapWordChar int32 = 2
)

// PangoLogAttr (pango-break.h), for S4.2h: a guint of one-bit fields,
// LogAttrSize bytes, allocated from the least significant bit on the
// LP64 ABIs gogui targets (amd64, arm64; GCC and Clang).
const (
	LogAttrSize = 4

	LogAttrIsWhite          uint32 = 1 << 3
	LogAttrIsCursorPosition uint32 = 1 << 4
)

// cairo_hint_metrics_t (cairo.h), for the text system's font options
// (S4.2g).
const (
	HintMetricsDefault int32 = 0
	HintMetricsOff     int32 = 1
	HintMetricsOn      int32 = 2
)
