//go:build windows

package d2d

import (
	"math"

	"github.com/radix29/gogui/internal/shell/win32/com"
	"github.com/radix29/gogui/internal/shell/win32/dxgi"
)

// Constants and mirrored structs from d2d1.h, d2d1_1.h and dcommon.h. The
// names are the SDK's without the D2D1_ prefix; the layouts are pinned by
// the tests.

// D2D1_FACTORY_TYPE.
const (
	FACTORY_TYPE_SINGLE_THREADED = 0
	FACTORY_TYPE_MULTI_THREADED  = 1
)

// D2D1_DEBUG_LEVEL.
const (
	DEBUG_LEVEL_NONE        = 0
	DEBUG_LEVEL_ERROR       = 1
	DEBUG_LEVEL_WARNING     = 2
	DEBUG_LEVEL_INFORMATION = 3
)

// D2D1_DEVICE_CONTEXT_OPTIONS.
const (
	DEVICE_CONTEXT_OPTIONS_NONE                               = 0
	DEVICE_CONTEXT_OPTIONS_ENABLE_MULTITHREADED_OPTIMIZATIONS = 1
)

// AlphaMode is D2D1_ALPHA_MODE.
type AlphaMode uint32

// Alpha modes.
const (
	ALPHA_MODE_UNKNOWN       AlphaMode = 0
	ALPHA_MODE_PREMULTIPLIED AlphaMode = 1
	ALPHA_MODE_STRAIGHT      AlphaMode = 2
	ALPHA_MODE_IGNORE        AlphaMode = 3
)

// BitmapOptions is D2D1_BITMAP_OPTIONS.
type BitmapOptions uint32

// Bitmap options. A bitmap over a swap chain's back buffer is
// BITMAP_OPTIONS_TARGET | BITMAP_OPTIONS_CANNOT_DRAW.
const (
	BITMAP_OPTIONS_NONE           BitmapOptions = 0
	BITMAP_OPTIONS_TARGET         BitmapOptions = 0x1
	BITMAP_OPTIONS_CANNOT_DRAW    BitmapOptions = 0x2
	BITMAP_OPTIONS_CPU_READ       BitmapOptions = 0x4
	BITMAP_OPTIONS_GDI_COMPATIBLE BitmapOptions = 0x8
)

// D2DERR_RECREATE_TARGET is EndDraw's report of a lost device (the same
// value as com's, named here for the binding's users).
const D2DERR_RECREATE_TARGET = com.D2DERR_RECREATE_TARGET

// FactoryOptions is D2D1_FACTORY_OPTIONS.
type FactoryOptions struct {
	DebugLevel uint32
}

// PixelFormat is D2D1_PIXEL_FORMAT.
type PixelFormat struct {
	Format    dxgi.Format
	AlphaMode AlphaMode
}

// BitmapProperties1 is D2D1_BITMAP_PROPERTIES1. ColorContext is an
// ID2D1ColorContext pointer (native memory) or 0.
type BitmapProperties1 struct {
	PixelFormat   PixelFormat
	DpiX, DpiY    float32
	BitmapOptions BitmapOptions
	ColorContext  uintptr
}

// ColorF is D2D1_COLOR_F (straight, not premultiplied, alpha; 0…1 per
// channel).
type ColorF struct {
	R, G, B, A float32
}

// AntialiasMode is D2D1_ANTIALIAS_MODE.
type AntialiasMode uint32

// Antialias modes.
const (
	ANTIALIAS_MODE_PER_PRIMITIVE AntialiasMode = 0
	ANTIALIAS_MODE_ALIASED       AntialiasMode = 1
)

// TextAntialiasMode is D2D1_TEXT_ANTIALIAS_MODE.
type TextAntialiasMode uint32

// Text antialias modes.
const (
	TEXT_ANTIALIAS_MODE_DEFAULT   TextAntialiasMode = 0
	TEXT_ANTIALIAS_MODE_CLEARTYPE TextAntialiasMode = 1
	TEXT_ANTIALIAS_MODE_GRAYSCALE TextAntialiasMode = 2
	TEXT_ANTIALIAS_MODE_ALIASED   TextAntialiasMode = 3
)

// DrawTextOptions is D2D1_DRAW_TEXT_OPTIONS (flags).
type DrawTextOptions uint32

// Draw text options.
const (
	DRAW_TEXT_OPTIONS_NONE              DrawTextOptions = 0
	DRAW_TEXT_OPTIONS_NO_SNAP           DrawTextOptions = 1
	DRAW_TEXT_OPTIONS_CLIP              DrawTextOptions = 2
	DRAW_TEXT_OPTIONS_ENABLE_COLOR_FONT DrawTextOptions = 4
)

// CapStyle is D2D1_CAP_STYLE. The order is FLAT, SQUARE, ROUND, not
// render's Butt, Round, Square: callers map, they don't cast.
type CapStyle uint32

// Cap styles.
const (
	CAP_STYLE_FLAT     CapStyle = 0
	CAP_STYLE_SQUARE   CapStyle = 1
	CAP_STYLE_ROUND    CapStyle = 2
	CAP_STYLE_TRIANGLE CapStyle = 3
)

// LineJoin is D2D1_LINE_JOIN. LINE_JOIN_MITER never bevels; render's (and
// CG's) miter, which bevels past the miter limit, is LINE_JOIN_MITER_OR_BEVEL.
type LineJoin uint32

// Line joins.
const (
	LINE_JOIN_MITER          LineJoin = 0
	LINE_JOIN_BEVEL          LineJoin = 1
	LINE_JOIN_ROUND          LineJoin = 2
	LINE_JOIN_MITER_OR_BEVEL LineJoin = 3
)

// DashStyle is D2D1_DASH_STYLE. Custom dashes and the dash offset are in
// units of the stroke width, not DIPs.
type DashStyle uint32

// Dash styles.
const (
	DASH_STYLE_SOLID        DashStyle = 0
	DASH_STYLE_DASH         DashStyle = 1
	DASH_STYLE_DOT          DashStyle = 2
	DASH_STYLE_DASH_DOT     DashStyle = 3
	DASH_STYLE_DASH_DOT_DOT DashStyle = 4
	DASH_STYLE_CUSTOM       DashStyle = 5
)

// StrokeTransformType is D2D1_STROKE_TRANSFORM_TYPE.
type StrokeTransformType uint32

// Stroke transform types.
const (
	STROKE_TRANSFORM_TYPE_NORMAL   StrokeTransformType = 0
	STROKE_TRANSFORM_TYPE_FIXED    StrokeTransformType = 1
	STROKE_TRANSFORM_TYPE_HAIRLINE StrokeTransformType = 2
)

// FillMode is D2D1_FILL_MODE.
type FillMode uint32

// Fill modes. render's nonzero rule is FILL_MODE_WINDING.
const (
	FILL_MODE_ALTERNATE FillMode = 0
	FILL_MODE_WINDING   FillMode = 1
)

// FigureBegin is D2D1_FIGURE_BEGIN.
type FigureBegin uint32

// Figure begins.
const (
	FIGURE_BEGIN_FILLED FigureBegin = 0
	FIGURE_BEGIN_HOLLOW FigureBegin = 1
)

// FigureEnd is D2D1_FIGURE_END.
type FigureEnd uint32

// Figure ends.
const (
	FIGURE_END_OPEN   FigureEnd = 0
	FIGURE_END_CLOSED FigureEnd = 1
)

// Gamma is D2D1_GAMMA, the space gradient stops are interpolated in.
type Gamma uint32

// Gammas. GAMMA_2_2 interpolates the sRGB values as they are.
const (
	GAMMA_2_2 Gamma = 0
	GAMMA_1_0 Gamma = 1
)

// ExtendMode is D2D1_EXTEND_MODE.
type ExtendMode uint32

// Extend modes.
const (
	EXTEND_MODE_CLAMP  ExtendMode = 0
	EXTEND_MODE_WRAP   ExtendMode = 1
	EXTEND_MODE_MIRROR ExtendMode = 2
)

// LayerOptions1 is D2D1_LAYER_OPTIONS1.
type LayerOptions1 uint32

// Layer options.
const (
	LAYER_OPTIONS1_NONE                       LayerOptions1 = 0
	LAYER_OPTIONS1_INITIALIZE_FROM_BACKGROUND LayerOptions1 = 1
	LAYER_OPTIONS1_IGNORE_ALPHA               LayerOptions1 = 2
)

// Point2F is D2D1_POINT_2F, passed by value to BeginFigure and AddLine
// (an HFA on arm64, eight bytes in one integer register on amd64).
type Point2F struct {
	X, Y float32
}

// RectF is D2D1_RECT_F.
type RectF struct {
	Left, Top, Right, Bottom float32
}

// InfiniteRect is D2D1::InfiniteRect(), a layer's "no content bounds".
func InfiniteRect() RectF {
	const m = math.MaxFloat32
	return RectF{-m, -m, m, m}
}

// Matrix3x2F is D2D1_MATRIX_3X2_F: x' = x·M11 + y·M21 + Dx,
// y' = x·M12 + y·M22 + Dy.
type Matrix3x2F struct {
	M11, M12, M21, M22, Dx, Dy float32
}

// IdentityMatrix is D2D1::Matrix3x2F::Identity().
func IdentityMatrix() Matrix3x2F {
	return Matrix3x2F{M11: 1, M22: 1}
}

// RoundedRect is D2D1_ROUNDED_RECT (one radius pair for all four corners).
type RoundedRect struct {
	Rect             RectF
	RadiusX, RadiusY float32
}

// BezierSegment is D2D1_BEZIER_SEGMENT (two control points, then the end).
type BezierSegment struct {
	Point1, Point2, Point3 Point2F
}

// QuadraticBezierSegment is D2D1_QUADRATIC_BEZIER_SEGMENT (control point,
// end).
type QuadraticBezierSegment struct {
	Point1, Point2 Point2F
}

// BrushProperties is D2D1_BRUSH_PROPERTIES. The zero value is invisible
// (opacity 0, zero transform); use DefaultBrushProperties.
type BrushProperties struct {
	Opacity   float32
	Transform Matrix3x2F
}

// DefaultBrushProperties is D2D1::BrushProperties(): opaque, identity.
func DefaultBrushProperties() BrushProperties {
	return BrushProperties{Opacity: 1, Transform: IdentityMatrix()}
}

// GradientStop is D2D1_GRADIENT_STOP.
type GradientStop struct {
	Position float32
	Color    ColorF
}

// LinearGradientBrushProperties is D2D1_LINEAR_GRADIENT_BRUSH_PROPERTIES,
// in the brush's space.
type LinearGradientBrushProperties struct {
	StartPoint, EndPoint Point2F
}

// StrokeStyleProperties1 is D2D1_STROKE_STYLE_PROPERTIES1.
type StrokeStyleProperties1 struct {
	StartCap, EndCap, DashCap CapStyle
	LineJoin                  LineJoin
	MiterLimit                float32
	DashStyle                 DashStyle
	DashOffset                float32
	TransformType             StrokeTransformType
}

// LayerParameters1 is D2D1_LAYER_PARAMETERS1. GeometricMask is an
// ID2D1Geometry pointer and OpacityBrush an ID2D1Brush pointer (native
// memory, or 0); the context holds its own references while the layer is
// pushed.
type LayerParameters1 struct {
	ContentBounds     RectF
	GeometricMask     com.Ptr
	MaskAntialiasMode AntialiasMode
	MaskTransform     Matrix3x2F
	Opacity           float32
	OpacityBrush      com.Ptr
	LayerOptions      LayerOptions1
}

// SizeU is D2D1_SIZE_U, passed by value to CreateBitmap: eight bytes in
// one integer register on amd64 and arm64 (Width in the low half).
type SizeU struct {
	Width, Height uint32
}

func (s SizeU) packed() uintptr { return uintptr(s.Width) | uintptr(s.Height)<<32 }

// Point2U is D2D1_POINT_2U.
type Point2U struct {
	X, Y uint32
}

// RectU is D2D1_RECT_U (pixels, right and bottom exclusive).
type RectU struct {
	Left, Top, Right, Bottom uint32
}

// Vector4F is D2D1_VECTOR_4F; effects take colors as one (R, G, B, A
// straight, 0…1).
type Vector4F struct {
	X, Y, Z, W float32
}

// InterpolationMode is D2D1_INTERPOLATION_MODE.
type InterpolationMode uint32

// Interpolation modes.
const (
	INTERPOLATION_MODE_NEAREST_NEIGHBOR    InterpolationMode = 0
	INTERPOLATION_MODE_LINEAR              InterpolationMode = 1
	INTERPOLATION_MODE_CUBIC               InterpolationMode = 2
	INTERPOLATION_MODE_MULTI_SAMPLE_LINEAR InterpolationMode = 3
	INTERPOLATION_MODE_ANISOTROPIC         InterpolationMode = 4
	INTERPOLATION_MODE_HIGH_QUALITY_CUBIC  InterpolationMode = 5
)

// CompositeMode is D2D1_COMPOSITE_MODE.
type CompositeMode uint32

// Composite modes.
const (
	COMPOSITE_MODE_SOURCE_OVER         CompositeMode = 0
	COMPOSITE_MODE_DESTINATION_OVER    CompositeMode = 1
	COMPOSITE_MODE_SOURCE_IN           CompositeMode = 2
	COMPOSITE_MODE_DESTINATION_IN      CompositeMode = 3
	COMPOSITE_MODE_SOURCE_OUT          CompositeMode = 4
	COMPOSITE_MODE_DESTINATION_OUT     CompositeMode = 5
	COMPOSITE_MODE_SOURCE_ATOP         CompositeMode = 6
	COMPOSITE_MODE_DESTINATION_ATOP    CompositeMode = 7
	COMPOSITE_MODE_XOR                 CompositeMode = 8
	COMPOSITE_MODE_PLUS                CompositeMode = 9
	COMPOSITE_MODE_SOURCE_COPY         CompositeMode = 10
	COMPOSITE_MODE_BOUNDED_SOURCE_COPY CompositeMode = 11
	COMPOSITE_MODE_MASK_INVERT         CompositeMode = 12
)

// D2D1_BORDER_MODE: SOFT fades to transparent past the input's edge, HARD
// clamps to the edge pixels (no darkened border on a blurred copy).
const (
	BORDER_MODE_SOFT = 0
	BORDER_MODE_HARD = 1
)

// PropertyType is D2D1_PROPERTY_TYPE.
type PropertyType uint32

// Property types.
const (
	PROPERTY_TYPE_UNKNOWN       PropertyType = 0
	PROPERTY_TYPE_STRING        PropertyType = 1
	PROPERTY_TYPE_BOOL          PropertyType = 2
	PROPERTY_TYPE_UINT32        PropertyType = 3
	PROPERTY_TYPE_INT32         PropertyType = 4
	PROPERTY_TYPE_FLOAT         PropertyType = 5
	PROPERTY_TYPE_VECTOR2       PropertyType = 6
	PROPERTY_TYPE_VECTOR3       PropertyType = 7
	PROPERTY_TYPE_VECTOR4       PropertyType = 8
	PROPERTY_TYPE_BLOB          PropertyType = 9
	PROPERTY_TYPE_IUNKNOWN      PropertyType = 10
	PROPERTY_TYPE_ENUM          PropertyType = 11
	PROPERTY_TYPE_ARRAY         PropertyType = 12
	PROPERTY_TYPE_CLSID         PropertyType = 13
	PROPERTY_TYPE_MATRIX_3X2    PropertyType = 14
	PROPERTY_TYPE_MATRIX_4X3    PropertyType = 15
	PROPERTY_TYPE_MATRIX_4X4    PropertyType = 16
	PROPERTY_TYPE_MATRIX_5X4    PropertyType = 17
	PROPERTY_TYPE_COLOR_CONTEXT PropertyType = 18
)

// PROPERTY_CLSID is the system property D2D1_PROPERTY_CLSID.
const PROPERTY_CLSID = 0x80000000

// Property indices of the shadow effect (D2D1_SHADOW_PROP).
const (
	SHADOW_PROP_BLUR_STANDARD_DEVIATION = 0 // FLOAT, DIPs, default 3
	SHADOW_PROP_COLOR                   = 1 // VECTOR4, default opaque black
	SHADOW_PROP_OPTIMIZATION            = 2 // ENUM, an OPTIMIZATION_*
)

// Property indices of the Gaussian-blur effect (D2D1_GAUSSIANBLUR_PROP).
const (
	GAUSSIANBLUR_PROP_STANDARD_DEVIATION = 0 // FLOAT, DIPs, default 3
	GAUSSIANBLUR_PROP_OPTIMIZATION       = 1 // ENUM, an OPTIMIZATION_*
	GAUSSIANBLUR_PROP_BORDER_MODE        = 2 // ENUM, a BORDER_MODE_*
)

// D2D1_SHADOW_OPTIMIZATION and D2D1_GAUSSIANBLUR_OPTIMIZATION (the same
// values).
const (
	OPTIMIZATION_SPEED    = 0
	OPTIMIZATION_BALANCED = 1
	OPTIMIZATION_QUALITY  = 2
)

// Property indices of the crop effect (D2D1_CROP_PROP, S2.4e_3).
const (
	CROP_PROP_RECT        = 0 // VECTOR4 (left, top, right, bottom), DIPs, default infinite
	CROP_PROP_BORDER_MODE = 1 // ENUM, a BORDER_MODE_*
)
