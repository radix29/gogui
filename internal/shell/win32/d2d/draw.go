//go:build windows

package d2d

import (
	"syscall"
	"unsafe"

	"github.com/radix29/gogui/internal/shell/win32/com"
)

// The drawing bindings (S2.4d_1): transforms, clips and layers, brushes,
// geometries and stroke styles, and the fill/draw calls the canvas makes.

// Interface IDs of the drawing interfaces.
var (
	IID_ID2D1Brush                    = com.MustGUID("{2CD906A8-12E2-11DC-9FED-001143A055F9}")
	IID_ID2D1SolidColorBrush          = com.MustGUID("{2CD906A9-12E2-11DC-9FED-001143A055F9}")
	IID_ID2D1LinearGradientBrush      = com.MustGUID("{2CD906AB-12E2-11DC-9FED-001143A055F9}")
	IID_ID2D1GradientStopCollection   = com.MustGUID("{2CD906A7-12E2-11DC-9FED-001143A055F9}")
	IID_ID2D1Geometry                 = com.MustGUID("{2CD906A1-12E2-11DC-9FED-001143A055F9}")
	IID_ID2D1RoundedRectangleGeometry = com.MustGUID("{2CD906A3-12E2-11DC-9FED-001143A055F9}")
	IID_ID2D1PathGeometry             = com.MustGUID("{2CD906A5-12E2-11DC-9FED-001143A055F9}")
	IID_ID2D1SimplifiedGeometrySink   = com.MustGUID("{2CD9069E-12E2-11DC-9FED-001143A055F9}")
	IID_ID2D1GeometrySink             = com.MustGUID("{2CD9069F-12E2-11DC-9FED-001143A055F9}")
	IID_ID2D1StrokeStyle              = com.MustGUID("{2CD9069D-12E2-11DC-9FED-001143A055F9}")
	IID_ID2D1StrokeStyle1             = com.MustGUID("{10A72A66-E91C-43F4-993F-DDF4B82B0B4A}")
)

// Vtable slots of the drawing methods; the tests check each one.
const (
	// ID2D1Factory.
	slotFactoryCreateRoundedRectangleGeometry = 6
	slotFactoryCreatePathGeometry             = 10
	// ID2D1Factory1.
	slotFactory1CreateStrokeStyle = 18

	// ID2D1RenderTarget.
	slotRTCreateSolidColorBrush        = 8
	slotRTCreateGradientStopCollection = 9
	slotRTCreateLinearGradientBrush    = 10
	slotRTFillRectangle                = 17
	slotRTDrawRoundedRectangle         = 18 // FLOAT width: the float path
	slotRTFillRoundedRectangle         = 19
	slotRTDrawGeometry                 = 22 // FLOAT width: the float path
	slotRTFillGeometry                 = 23
	slotRTDrawTextLayout               = 28 // D2D1_POINT_2F by value: the float path
	slotRTSetTransform                 = 30
	slotRTGetTransform                 = 31
	slotRTSetTextAntialiasMode         = 34
	slotRTGetTextAntialiasMode         = 35
	slotRTPopLayer                     = 41
	slotRTPushAxisAlignedClip          = 45
	slotRTPopAxisAlignedClip           = 46

	// ID2D1DeviceContext.
	slotDCPushLayer = 86 // the D2D1_LAYER_PARAMETERS1 overload

	// ID2D1SolidColorBrush (after ID2D1Brush 4–7).
	slotSolidSetColor = 8
	// ID2D1LinearGradientBrush.
	slotLinearGetGradientStopCollection = 12
	// ID2D1GradientStopCollection.
	slotStopsGetGradientStopCount       = 4
	slotStopsGetColorInterpolationGamma = 6
	slotStopsGetExtendMode              = 7

	// ID2D1PathGeometry (after ID2D1Geometry 4–16).
	slotPathOpen            = 17
	slotPathGetSegmentCount = 19
	slotPathGetFigureCount  = 20

	// ID2D1SimplifiedGeometrySink / ID2D1GeometrySink.
	slotSinkSetFillMode        = 3
	slotSinkBeginFigure        = 5 // D2D1_POINT_2F by value: the float path
	slotSinkEndFigure          = 8
	slotSinkClose              = 9
	slotSinkAddLine            = 10 // D2D1_POINT_2F by value: the float path
	slotSinkAddBezier          = 11
	slotSinkAddQuadraticBezier = 12

	// ID2D1StrokeStyle / ID2D1StrokeStyle1.
	slotStrokeGetStartCap       = 4
	slotStrokeGetEndCap         = 5
	slotStrokeGetDashCap        = 6
	slotStrokeGetLineJoin       = 8
	slotStrokeGetDashStyle      = 10
	slotStrokeGetDashesCount    = 11
	slotStroke1GetTransformType = 13
)

// directFloats selects the direct path for the float-path methods
// (com.DirectFloats; the tests turn it off to check the bound path on
// amd64 too). Off, they go through their com.Method.
var directFloats = com.DirectFloats

// The float-path methods.
var (
	methodDrawRoundedRectangle = com.Method[func(this com.Ptr, rr *RoundedRect, brush com.Ptr, width float32, style com.Ptr)]{Slot: slotRTDrawRoundedRectangle}
	methodDrawGeometry         = com.Method[func(this com.Ptr, geometry, brush com.Ptr, width float32, style com.Ptr)]{Slot: slotRTDrawGeometry}
	methodBeginFigure          = com.Method[func(this com.Ptr, start Point2F, begin FigureBegin)]{Slot: slotSinkBeginFigure}
	methodAddLine              = com.Method[func(this com.Ptr, p Point2F)]{Slot: slotSinkAddLine}
	methodDrawTextLayout       = com.Method[func(this com.Ptr, origin Point2F, layout, brush com.Ptr, options DrawTextOptions)]{Slot: slotRTDrawTextLayout}
)

// --- Factory ---------------------------------------------------------------

// CreatePathGeometry creates an empty path geometry; fill it through Open.
func (f Factory1) CreatePathGeometry() (PathGeometry, error) {
	out, err := f.CallOut("ID2D1Factory::CreatePathGeometry", slotFactoryCreatePathGeometry)
	return PathGeometry{out}, err
}

// CreateRoundedRectangleGeometry creates the geometry of rr (one radius
// pair for every corner), for a layer's mask.
func (f Factory1) CreateRoundedRectangleGeometry(rr RoundedRect) (RoundedRectangleGeometry, error) {
	out, err := f.CallOut("ID2D1Factory::CreateRoundedRectangleGeometry", slotFactoryCreateRoundedRectangleGeometry,
		uintptr(unsafe.Pointer(&rr)))
	return RoundedRectangleGeometry{out}, err
}

// CreateStrokeStyle creates an ID2D1StrokeStyle1. dashes are used only
// with DASH_STYLE_CUSTOM (and must be empty otherwise); like DashOffset
// they are in units of the stroke width.
func (f Factory1) CreateStrokeStyle(props StrokeStyleProperties1, dashes []float32) (StrokeStyle, error) {
	var dp *float32
	if len(dashes) > 0 {
		dp = &dashes[0]
	}
	out, err := f.CallOut("ID2D1Factory1::CreateStrokeStyle", slotFactory1CreateStrokeStyle,
		uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(dp)), uintptr(len(dashes)))
	return StrokeStyle{out}, err
}

// --- Device context: state ---------------------------------------------------

// SetTransform sets the transform from the caller's DIPs to the target's
// DIPs (the DPI scale comes after it).
func (c DeviceContext) SetTransform(m Matrix3x2F) {
	syscall.SyscallN(c.Slot(slotRTSetTransform), uintptr(c.Ptr), uintptr(unsafe.Pointer(&m)))
}

// Transform returns the current transform.
func (c DeviceContext) Transform() Matrix3x2F {
	var m Matrix3x2F
	c.Call(slotRTGetTransform, uintptr(unsafe.Pointer(&m)))
	return m
}

// PushAxisAlignedClip clips to r, transformed by the current transform
// (to its bounding box if the transform rotates). Pop it with
// PopAxisAlignedClip, nested LIFO with the layers.
func (c DeviceContext) PushAxisAlignedClip(r RectF, aa AntialiasMode) {
	syscall.SyscallN(c.Slot(slotRTPushAxisAlignedClip), uintptr(c.Ptr), uintptr(unsafe.Pointer(&r)), uintptr(aa))
}

// PopAxisAlignedClip pops the last PushAxisAlignedClip.
func (c DeviceContext) PopAxisAlignedClip() {
	syscall.SyscallN(c.Slot(slotRTPopAxisAlignedClip), uintptr(c.Ptr))
}

// PushLayer pushes a layer with no ID2D1Layer (allowed on a device
// context; D2D manages the memory). Pop it with PopLayer.
func (c DeviceContext) PushLayer(p *LayerParameters1) {
	syscall.SyscallN(c.Slot(slotDCPushLayer), uintptr(c.Ptr), uintptr(unsafe.Pointer(p)), 0)
}

// PopLayer pops the last PushLayer.
func (c DeviceContext) PopLayer() {
	syscall.SyscallN(c.Slot(slotRTPopLayer), uintptr(c.Ptr))
}

// --- Device context: resources -------------------------------------------------

// CreateSolidColorBrush creates a brush of color (straight alpha).
func (c DeviceContext) CreateSolidColorBrush(color ColorF, props BrushProperties) (SolidColorBrush, error) {
	out, err := c.CallOut("ID2D1RenderTarget::CreateSolidColorBrush", slotRTCreateSolidColorBrush,
		uintptr(unsafe.Pointer(&color)), uintptr(unsafe.Pointer(&props)))
	return SolidColorBrush{out}, err
}

// CreateGradientStopCollection creates a stop collection (at least one
// stop; D2D sorts them by position).
func (c DeviceContext) CreateGradientStopCollection(stops []GradientStop, gamma Gamma, extend ExtendMode) (GradientStopCollection, error) {
	var sp *GradientStop
	if len(stops) > 0 {
		sp = &stops[0]
	}
	out, err := c.CallOut("ID2D1RenderTarget::CreateGradientStopCollection", slotRTCreateGradientStopCollection,
		uintptr(unsafe.Pointer(sp)), uintptr(len(stops)), uintptr(gamma), uintptr(extend))
	return GradientStopCollection{out}, err
}

// CreateLinearGradientBrush creates a linear gradient over stops; the
// brush holds a reference to them.
func (c DeviceContext) CreateLinearGradientBrush(lp LinearGradientBrushProperties, props BrushProperties, stops GradientStopCollection) (LinearGradientBrush, error) {
	out, err := c.CallOut("ID2D1RenderTarget::CreateLinearGradientBrush", slotRTCreateLinearGradientBrush,
		uintptr(unsafe.Pointer(&lp)), uintptr(unsafe.Pointer(&props)), uintptr(stops.Ptr))
	return LinearGradientBrush{out}, err
}

// --- Device context: drawing -----------------------------------------------------

// FillRectangle fills r with brush.
func (c DeviceContext) FillRectangle(r RectF, brush Brush) {
	syscall.SyscallN(c.Slot(slotRTFillRectangle), uintptr(c.Ptr), uintptr(unsafe.Pointer(&r)), uintptr(brush.Ptr))
}

// FillRoundedRectangle fills rr with brush.
func (c DeviceContext) FillRoundedRectangle(rr RoundedRect, brush Brush) {
	syscall.SyscallN(c.Slot(slotRTFillRoundedRectangle), uintptr(c.Ptr), uintptr(unsafe.Pointer(&rr)), uintptr(brush.Ptr))
}

// DrawRoundedRectangle strokes rr's outline, centred on it, width DIPs
// wide; style may be the zero StrokeStyle (solid, flat caps, miter joins).
func (c DeviceContext) DrawRoundedRectangle(rr RoundedRect, brush Brush, width float32, style StrokeStyle) {
	if directFloats {
		syscall.SyscallN(c.Slot(slotRTDrawRoundedRectangle), uintptr(c.Ptr), uintptr(unsafe.Pointer(&rr)),
			uintptr(brush.Ptr), com.Float(width), uintptr(style.Ptr))
		return
	}
	drawRoundedRectangleBound(c, rr, brush, width, style)
}

// drawRoundedRectangleBound is DrawRoundedRectangle's bound path, apart so
// that the rr the bound func gets a pointer to (which escapes into it) is
// this function's copy, not the caller's on the direct path.
//
//go:noinline
func drawRoundedRectangleBound(c DeviceContext, rr RoundedRect, brush Brush, width float32, style StrokeStyle) {
	methodDrawRoundedRectangle.Fn(c.Ptr)(c.Ptr, &rr, brush.Ptr, width, style.Ptr)
}

// FillGeometry fills g with brush (no opacity brush).
func (c DeviceContext) FillGeometry(g Geometry, brush Brush) {
	syscall.SyscallN(c.Slot(slotRTFillGeometry), uintptr(c.Ptr), uintptr(g.Ptr), uintptr(brush.Ptr), 0)
}

// DrawGeometry strokes g, centred on its outline; style may be zero.
func (c DeviceContext) DrawGeometry(g Geometry, brush Brush, width float32, style StrokeStyle) {
	if directFloats {
		syscall.SyscallN(c.Slot(slotRTDrawGeometry), uintptr(c.Ptr), uintptr(g.Ptr), uintptr(brush.Ptr),
			com.Float(width), uintptr(style.Ptr))
		return
	}
	methodDrawGeometry.Fn(c.Ptr)(c.Ptr, g.Ptr, brush.Ptr, width, style.Ptr)
}

// DrawTextLayout draws an IDWriteTextLayout (layout: d2d doesn't import
// dwrite) with its top-left corner at origin, in brush, under the current
// transform, clips and layers.
func (c DeviceContext) DrawTextLayout(origin Point2F, layout com.Ptr, brush Brush, options DrawTextOptions) {
	if directFloats {
		syscall.SyscallN(c.Slot(slotRTDrawTextLayout), uintptr(c.Ptr), com.Float2(origin.X, origin.Y),
			uintptr(layout), uintptr(brush.Ptr), uintptr(options))
		return
	}
	methodDrawTextLayout.Fn(c.Ptr)(c.Ptr, origin, layout, brush.Ptr, options)
}

// SetTextAntialiasMode sets how text is antialiased, until the next call.
func (c DeviceContext) SetTextAntialiasMode(m TextAntialiasMode) {
	syscall.SyscallN(c.Slot(slotRTSetTextAntialiasMode), uintptr(c.Ptr), uintptr(m))
}

// TextAntialiasMode returns the text antialias mode.
func (c DeviceContext) TextAntialiasMode() TextAntialiasMode {
	return TextAntialiasMode(uint32(c.Call(slotRTGetTextAntialiasMode)))
}

// --- Brushes ---------------------------------------------------------------------

// Brush is ID2D1Brush, the base the fill and draw calls take. It doesn't
// own a reference of its own: it is a view of a SolidColorBrush or a
// LinearGradientBrush (single inheritance, same pointer).
type Brush struct{ com.Ptr }

// SolidColorBrush is ID2D1SolidColorBrush.
type SolidColorBrush struct{ com.Ptr }

// Brush returns b as an ID2D1Brush (no new reference).
func (b SolidColorBrush) Brush() Brush { return Brush(b) }

// LinearGradientBrush is ID2D1LinearGradientBrush.
type LinearGradientBrush struct{ com.Ptr }

// Brush returns b as an ID2D1Brush (no new reference).
func (b LinearGradientBrush) Brush() Brush { return Brush(b) }

// Stops returns the brush's stop collection with a reference of its own.
func (b LinearGradientBrush) Stops() GradientStopCollection {
	return GradientStopCollection{b.GetOut(slotLinearGetGradientStopCollection)}
}

// GradientStopCollection is ID2D1GradientStopCollection.
type GradientStopCollection struct{ com.Ptr }

// Count returns the number of stops.
func (s GradientStopCollection) Count() uint32 {
	return uint32(s.Call(slotStopsGetGradientStopCount))
}

// --- Geometries ------------------------------------------------------------------

// Geometry is ID2D1Geometry, the base the geometry calls take; like Brush
// it is a view without a reference of its own.
type Geometry struct{ com.Ptr }

// RoundedRectangleGeometry is ID2D1RoundedRectangleGeometry.
type RoundedRectangleGeometry struct{ com.Ptr }

// Geometry returns g as an ID2D1Geometry (no new reference).
func (g RoundedRectangleGeometry) Geometry() Geometry { return Geometry(g) }

// PathGeometry is ID2D1PathGeometry.
type PathGeometry struct{ com.Ptr }

// Geometry returns g as an ID2D1Geometry (no new reference).
func (g PathGeometry) Geometry() Geometry { return Geometry(g) }

// Open returns the sink that fills the geometry. A path geometry can be
// opened once; it is usable after the sink's Close (and Release).
func (g PathGeometry) Open() (GeometrySink, error) {
	out, err := g.CallOut("ID2D1PathGeometry::Open", slotPathOpen)
	return GeometrySink{out}, err
}

// GeometrySink is ID2D1GeometrySink. Errors in the figure calls surface in
// Close.
type GeometrySink struct{ com.Ptr }

// SetFillMode sets the fill mode; call it before the first BeginFigure.
func (s GeometrySink) SetFillMode(m FillMode) {
	syscall.SyscallN(s.Slot(slotSinkSetFillMode), uintptr(s.Ptr), uintptr(m))
}

// BeginFigure starts a figure at start.
func (s GeometrySink) BeginFigure(start Point2F, begin FigureBegin) {
	if directFloats {
		syscall.SyscallN(s.Slot(slotSinkBeginFigure), uintptr(s.Ptr), com.Float2(start.X, start.Y), uintptr(begin))
		return
	}
	methodBeginFigure.Fn(s.Ptr)(s.Ptr, start, begin)
}

// AddLine adds a line to p.
func (s GeometrySink) AddLine(p Point2F) {
	if directFloats {
		syscall.SyscallN(s.Slot(slotSinkAddLine), uintptr(s.Ptr), com.Float2(p.X, p.Y))
		return
	}
	methodAddLine.Fn(s.Ptr)(s.Ptr, p)
}

// AddBezier adds a cubic Bézier.
func (s GeometrySink) AddBezier(b BezierSegment) {
	syscall.SyscallN(s.Slot(slotSinkAddBezier), uintptr(s.Ptr), uintptr(unsafe.Pointer(&b)))
}

// AddQuadraticBezier adds a quadratic Bézier.
func (s GeometrySink) AddQuadraticBezier(q QuadraticBezierSegment) {
	syscall.SyscallN(s.Slot(slotSinkAddQuadraticBezier), uintptr(s.Ptr), uintptr(unsafe.Pointer(&q)))
}

// EndFigure ends the current figure, closing it with a line to its start
// for FIGURE_END_CLOSED.
func (s GeometrySink) EndFigure(end FigureEnd) {
	syscall.SyscallN(s.Slot(slotSinkEndFigure), uintptr(s.Ptr), uintptr(end))
}

// Close finishes the geometry and returns the first error of the figure
// calls (D2DERR_WRONG_STATE for an unbalanced figure, for example). The
// sink still has to be released.
func (s GeometrySink) Close() error {
	return s.CallHR("ID2D1SimplifiedGeometrySink::Close", slotSinkClose)
}

// --- Stroke styles ---------------------------------------------------------------

// StrokeStyle is ID2D1StrokeStyle1 (passed where an ID2D1StrokeStyle is
// taken). The zero StrokeStyle is the NULL style.
type StrokeStyle struct{ com.Ptr }

// Caps returns the start, end and dash caps.
func (s StrokeStyle) Caps() (start, end, dash CapStyle) {
	return CapStyle(uint32(s.Call(slotStrokeGetStartCap))), CapStyle(uint32(s.Call(slotStrokeGetEndCap))),
		CapStyle(uint32(s.Call(slotStrokeGetDashCap)))
}

// LineJoin returns the line join.
func (s StrokeStyle) LineJoin() LineJoin {
	return LineJoin(uint32(s.Call(slotStrokeGetLineJoin)))
}

// DashStyle returns the dash style.
func (s StrokeStyle) DashStyle() DashStyle {
	return DashStyle(uint32(s.Call(slotStrokeGetDashStyle)))
}

// DashesCount returns the number of entries of the dash array (the
// custom one, or the preset's).
func (s StrokeStyle) DashesCount() uint32 {
	return uint32(s.Call(slotStrokeGetDashesCount))
}
