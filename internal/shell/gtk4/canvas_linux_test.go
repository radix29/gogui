//go:build linux

package gtk4

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// The canvas over a real GtkSnapshot (S4.1g, S4.2b–S4.2f, S4.2i): the
// node-tree tests, as in spike/gtk. Each test paints through
// newCanvas(newGtkSnapshot(s), …) into a fresh gtk_snapshot_new, finishes
// the canvas, turns the snapshot into a node with gtk_snapshot_free_to_node
// and walks the tree (cnTree).
// The host tests (canvas_test.go) check the calls the canvas makes
// over a fake snapshotter; these check what GTK makes of them.
//
// GTK optimizes node trees, and how depends on the version, so the checks
// are structural where it may: a translation or scale is folded into the
// bounds of color, border and gradient nodes (gtk_snapshot_ensure_affine)
// but could come as a transform node, so positions are compared in the
// root's coordinates (cnNode.global, through every transform node above);
// a clip that contains its child is dropped and a rectilinear rounded
// clip becomes a clip node, so "clipped by r" means a clip or rounded-clip
// ancestor with bounds r; an opacity of 1 may be dropped; nodes may be
// wrapped in containers and debug nodes. Exact checks are used where GTK
// keeps what it is given: color, border widths and outline, the radii of a
// rounded clip, gradient points and stops, a shadow's outline radii,
// color, offset, spread and blur, a blur node's radius, a text node's
// color and the baseline origin of its glyph run (its offset; its bounds
// are the glyphs' ink, so they are only checked against the layout's box
// with a tolerance), and the matrix of a transform node. What documents GTK's own behaviour is logged with the prefix
// CANVAS-RESULT; every test logs its node tree that way too.
//
// An unbalanced snapshot (a push left open) makes gtk_snapshot_free_to_node
// warn and return only the innermost open state's node, so the drawing
// before it is lost: the tests that leave state open check that the
// earlier drawing is still in the tree. GTK criticals are not captured (a
// g_log writer would be a native callback beyond the trampoline set).

// cnGsk holds the GSK getters only these tests call (bound by cnBind).
var cnGsk struct {
	RenderNodeGetNodeType            func(n uintptr) int32                      // GskRenderNodeType (const GskRenderNode *)
	RenderNodeGetBounds              func(n uintptr, b *gtk.GrapheneRect)       // void (GskRenderNode *, graphene_rect_t *)
	ContainerNodeGetNChildren        func(n uintptr) uint32                     // guint (const GskRenderNode *)
	ContainerNodeGetChild            func(n uintptr, i uint32) uintptr          // GskRenderNode * (const GskRenderNode *, guint)
	DebugNodeGetChild                func(n uintptr) uintptr                    // GskRenderNode * (const GskRenderNode *)
	ColorNodeGetColor                func(n uintptr) uintptr                    // const GdkRGBA * (const GskRenderNode *)
	ClipNodeGetChild                 func(n uintptr) uintptr                    // GskRenderNode * (const GskRenderNode *)
	ClipNodeGetClip                  func(n uintptr) uintptr                    // const graphene_rect_t * (const GskRenderNode *)
	RoundedClipNodeGetChild          func(n uintptr) uintptr                    // GskRenderNode * (const GskRenderNode *)
	RoundedClipNodeGetClip           func(n uintptr) uintptr                    // const GskRoundedRect * (const GskRenderNode *)
	OpacityNodeGetChild              func(n uintptr) uintptr                    // GskRenderNode * (const GskRenderNode *)
	OpacityNodeGetOpacity            func(n uintptr) float32                    // float (const GskRenderNode *)
	TransformNodeGetChild            func(n uintptr) uintptr                    // GskRenderNode * (const GskRenderNode *)
	TransformNodeGetTransform        func(n uintptr) uintptr                    // GskTransform * (const GskRenderNode *)
	BorderNodeGetOutline             func(n uintptr) uintptr                    // const GskRoundedRect * (const GskRenderNode *)
	BorderNodeGetWidths              func(n uintptr) uintptr                    // const float * (4) (const GskRenderNode *)
	BorderNodeGetColors              func(n uintptr) uintptr                    // const GdkRGBA * (4) (const GskRenderNode *)
	LinearGradientNodeGetStart       func(n uintptr) uintptr                    // const graphene_point_t * (const GskRenderNode *)
	LinearGradientNodeGetEnd         func(n uintptr) uintptr                    // const graphene_point_t * (const GskRenderNode *)
	LinearGradientNodeGetNColorStops func(n uintptr) uintptr                    // gsize (const GskRenderNode *)
	LinearGradientNodeGetColorStops  func(n uintptr, nStops *uintptr) uintptr   // const GskColorStop * (const GskRenderNode *, gsize *)
	StrokeNodeGetChild               func(n uintptr) uintptr                    // GskRenderNode * (const GskRenderNode *)
	StrokeNodeGetStroke              func(n uintptr) uintptr                    // const GskStroke * (const GskRenderNode *)
	StrokeNodeGetPath                func(n uintptr) uintptr                    // GskPath * (const GskRenderNode *)
	StrokeGetLineWidth               func(s uintptr) float32                    // float (const GskStroke *)
	StrokeGetLineCap                 func(s uintptr) int32                      // GskLineCap (const GskStroke *)
	StrokeGetLineJoin                func(s uintptr) int32                      // GskLineJoin (const GskStroke *)
	StrokeGetMiterLimit              func(s uintptr) float32                    // float (const GskStroke *)
	StrokeGetDash                    func(s uintptr, nDash *uintptr) uintptr    // const float * (const GskStroke *, gsize *)
	StrokeGetDashOffset              func(s uintptr) float32                    // float (const GskStroke *)
	FillNodeGetChild                 func(n uintptr) uintptr                    // GskRenderNode * (const GskRenderNode *)
	FillNodeGetPath                  func(n uintptr) uintptr                    // GskPath * (const GskRenderNode *)
	FillNodeGetFillRule              func(n uintptr) int32                      // GskFillRule (const GskRenderNode *)
	PathGetBounds                    func(p uintptr, b *gtk.GrapheneRect) int32 // gboolean (const GskPath *, graphene_rect_t *)
	OutsetShadowNodeGetOutline       func(n uintptr) uintptr                    // const GskRoundedRect * (const GskRenderNode *)
	OutsetShadowNodeGetColor         func(n uintptr) uintptr                    // const GdkRGBA * (const GskRenderNode *)
	OutsetShadowNodeGetDx            func(n uintptr) float32                    // float (const GskRenderNode *)
	OutsetShadowNodeGetDy            func(n uintptr) float32                    // float (const GskRenderNode *)
	OutsetShadowNodeGetSpread        func(n uintptr) float32                    // float (const GskRenderNode *)
	OutsetShadowNodeGetBlurRadius    func(n uintptr) float32                    // float (const GskRenderNode *)
	InsetShadowNodeGetOutline        func(n uintptr) uintptr                    // const GskRoundedRect * (const GskRenderNode *)
	InsetShadowNodeGetColor          func(n uintptr) uintptr                    // const GdkRGBA * (const GskRenderNode *)
	InsetShadowNodeGetDx             func(n uintptr) float32                    // float (const GskRenderNode *)
	InsetShadowNodeGetDy             func(n uintptr) float32                    // float (const GskRenderNode *)
	InsetShadowNodeGetSpread         func(n uintptr) float32                    // float (const GskRenderNode *)
	InsetShadowNodeGetBlurRadius     func(n uintptr) float32                    // float (const GskRenderNode *)
	BlurNodeGetChild                 func(n uintptr) uintptr                    // GskRenderNode * (const GskRenderNode *)
	BlurNodeGetRadius                func(n uintptr) float32                    // float (const GskRenderNode *)
	TextNodeGetColor                 func(n uintptr) uintptr                    // const GdkRGBA * (const GskRenderNode *)
	TextNodeGetOffset                func(n uintptr) uintptr                    // const graphene_point_t * (const GskRenderNode *)
	TextNodeGetNumGlyphs             func(n uintptr) uint32                     // guint (const GskRenderNode *)
	TextNodeGetFont                  func(n uintptr) uintptr                    // PangoFont * (const GskRenderNode *)
	TransformGetCategory             func(t uintptr) int32                      // GskTransformCategory (GskTransform *)
	TransformToMatrix                func(t, out uintptr)                       // void (GskTransform *, graphene_matrix_t *)
	// TransformTransformPoint is optional: nil if it did not bind.
	TransformTransformPoint func(t uintptr, p, out *gtk.GraphenePoint) // void (GskTransform *, const graphene_point_t *, graphene_point_t *)
	// PathIsClosed is optional: nil if it did not bind.
	PathIsClosed func(p uintptr) int32 // gboolean (GskPath *)
}

var cnBindOnce = sync.OnceValue(func() error {
	lib, err := ffi.Dlopen(gtk.GTKLib)
	if err != nil {
		return err
	}
	var errs []error
	for _, b := range []struct {
		name string
		fptr any
	}{
		{"gsk_render_node_get_node_type", &cnGsk.RenderNodeGetNodeType},
		{"gsk_render_node_get_bounds", &cnGsk.RenderNodeGetBounds},
		{"gsk_container_node_get_n_children", &cnGsk.ContainerNodeGetNChildren},
		{"gsk_container_node_get_child", &cnGsk.ContainerNodeGetChild},
		{"gsk_debug_node_get_child", &cnGsk.DebugNodeGetChild},
		{"gsk_color_node_get_color", &cnGsk.ColorNodeGetColor},
		{"gsk_clip_node_get_child", &cnGsk.ClipNodeGetChild},
		{"gsk_clip_node_get_clip", &cnGsk.ClipNodeGetClip},
		{"gsk_rounded_clip_node_get_child", &cnGsk.RoundedClipNodeGetChild},
		{"gsk_rounded_clip_node_get_clip", &cnGsk.RoundedClipNodeGetClip},
		{"gsk_opacity_node_get_child", &cnGsk.OpacityNodeGetChild},
		{"gsk_opacity_node_get_opacity", &cnGsk.OpacityNodeGetOpacity},
		{"gsk_transform_node_get_child", &cnGsk.TransformNodeGetChild},
		{"gsk_transform_node_get_transform", &cnGsk.TransformNodeGetTransform},
		{"gsk_border_node_get_outline", &cnGsk.BorderNodeGetOutline},
		{"gsk_border_node_get_widths", &cnGsk.BorderNodeGetWidths},
		{"gsk_border_node_get_colors", &cnGsk.BorderNodeGetColors},
		{"gsk_linear_gradient_node_get_start", &cnGsk.LinearGradientNodeGetStart},
		{"gsk_linear_gradient_node_get_end", &cnGsk.LinearGradientNodeGetEnd},
		{"gsk_linear_gradient_node_get_n_color_stops", &cnGsk.LinearGradientNodeGetNColorStops},
		{"gsk_linear_gradient_node_get_color_stops", &cnGsk.LinearGradientNodeGetColorStops},
		{"gsk_stroke_node_get_child", &cnGsk.StrokeNodeGetChild},
		{"gsk_stroke_node_get_stroke", &cnGsk.StrokeNodeGetStroke},
		{"gsk_stroke_node_get_path", &cnGsk.StrokeNodeGetPath},
		{"gsk_stroke_get_line_width", &cnGsk.StrokeGetLineWidth},
		{"gsk_stroke_get_line_cap", &cnGsk.StrokeGetLineCap},
		{"gsk_stroke_get_line_join", &cnGsk.StrokeGetLineJoin},
		{"gsk_stroke_get_miter_limit", &cnGsk.StrokeGetMiterLimit},
		{"gsk_stroke_get_dash", &cnGsk.StrokeGetDash},
		{"gsk_stroke_get_dash_offset", &cnGsk.StrokeGetDashOffset},
		{"gsk_fill_node_get_child", &cnGsk.FillNodeGetChild},
		{"gsk_fill_node_get_path", &cnGsk.FillNodeGetPath},
		{"gsk_fill_node_get_fill_rule", &cnGsk.FillNodeGetFillRule},
		{"gsk_path_get_bounds", &cnGsk.PathGetBounds},
		{"gsk_outset_shadow_node_get_outline", &cnGsk.OutsetShadowNodeGetOutline},
		{"gsk_outset_shadow_node_get_color", &cnGsk.OutsetShadowNodeGetColor},
		{"gsk_outset_shadow_node_get_dx", &cnGsk.OutsetShadowNodeGetDx},
		{"gsk_outset_shadow_node_get_dy", &cnGsk.OutsetShadowNodeGetDy},
		{"gsk_outset_shadow_node_get_spread", &cnGsk.OutsetShadowNodeGetSpread},
		{"gsk_outset_shadow_node_get_blur_radius", &cnGsk.OutsetShadowNodeGetBlurRadius},
		{"gsk_inset_shadow_node_get_outline", &cnGsk.InsetShadowNodeGetOutline},
		{"gsk_inset_shadow_node_get_color", &cnGsk.InsetShadowNodeGetColor},
		{"gsk_inset_shadow_node_get_dx", &cnGsk.InsetShadowNodeGetDx},
		{"gsk_inset_shadow_node_get_dy", &cnGsk.InsetShadowNodeGetDy},
		{"gsk_inset_shadow_node_get_spread", &cnGsk.InsetShadowNodeGetSpread},
		{"gsk_inset_shadow_node_get_blur_radius", &cnGsk.InsetShadowNodeGetBlurRadius},
		{"gsk_blur_node_get_child", &cnGsk.BlurNodeGetChild},
		{"gsk_blur_node_get_radius", &cnGsk.BlurNodeGetRadius},
		{"gsk_text_node_get_color", &cnGsk.TextNodeGetColor},
		{"gsk_text_node_get_offset", &cnGsk.TextNodeGetOffset},
		{"gsk_text_node_get_num_glyphs", &cnGsk.TextNodeGetNumGlyphs},
		{"gsk_text_node_get_font", &cnGsk.TextNodeGetFont},
		{"gsk_transform_get_category", &cnGsk.TransformGetCategory},
		{"gsk_transform_to_matrix", &cnGsk.TransformToMatrix},
	} {
		if err := lib.Bind(b.fptr, b.name); err != nil {
			errs = append(errs, err)
		}
	}
	if err := lib.Bind(&cnGsk.TransformTransformPoint, "gsk_transform_transform_point"); err != nil {
		cnGsk.TransformTransformPoint = nil
	}
	if err := lib.Bind(&cnGsk.PathIsClosed, "gsk_path_is_closed"); err != nil {
		cnGsk.PathIsClosed = nil
	}
	return errors.Join(errs...)
})

// cnNeed is needGTK plus the GSK getters.
func cnNeed(t *testing.T) {
	t.Helper()
	needGTK(t)
	if err := cnBindOnce(); err != nil {
		t.Fatalf("binding the GSK node getters: %v", err)
	}
}

// --- The node tree ---------------------------------------------------------------

// cnNode is one render node of a tree, copied out of GSK while the root is
// alive (ptr is valid only then).
type cnNode struct {
	ptr    uintptr
	typ    int32
	bounds gtk.GrapheneRect // in the parent's coordinates (gsk_render_node_get_bounds)
	global gtk.GrapheneRect // bounds in the root's coordinates, through the transform nodes above
	parent *cnNode
	kids   []*cnNode
}

// cnMaxDepth bounds the walk, against a corrupt tree.
const cnMaxDepth = 64

// cnTree copies the tree under n (0 gives nil). Container, debug, clip,
// rounded-clip, opacity, blur, fill, stroke and transform nodes are
// descended into.
func cnTree(n uintptr) *cnNode { return cnBuild(n, nil, render.Identity(), 0) }

func cnBuild(n uintptr, parent *cnNode, acc render.Transform, depth int) *cnNode {
	if n == 0 || depth > cnMaxDepth {
		return nil
	}
	nd := &cnNode{ptr: n, typ: cnGsk.RenderNodeGetNodeType(n), parent: parent}
	cnGsk.RenderNodeGetBounds(n, &nd.bounds)
	nd.global = cnMapRect(acc, nd.bounds)
	add := func(child uintptr, acc render.Transform) {
		if k := cnBuild(child, nd, acc, depth+1); k != nil {
			nd.kids = append(nd.kids, k)
		}
	}
	switch nd.typ {
	case gtk.NodeContainer:
		for i := range cnGsk.ContainerNodeGetNChildren(n) {
			add(cnGsk.ContainerNodeGetChild(n, i), acc)
		}
	case gtk.NodeDebug:
		add(cnGsk.DebugNodeGetChild(n), acc)
	case gtk.NodeClip:
		add(cnGsk.ClipNodeGetChild(n), acc)
	case gtk.NodeRoundedClip:
		add(cnGsk.RoundedClipNodeGetChild(n), acc)
	case gtk.NodeStroke:
		add(cnGsk.StrokeNodeGetChild(n), acc)
	case gtk.NodeFill:
		add(cnGsk.FillNodeGetChild(n), acc)
	case gtk.NodeOpacity:
		add(cnGsk.OpacityNodeGetChild(n), acc)
	case gtk.NodeBlur:
		add(cnGsk.BlurNodeGetChild(n), acc)
	case gtk.NodeTransform:
		local := render.Identity()
		if m, ok := cnMatrixOf(cnGsk.TransformNodeGetTransform(n)); ok {
			local = cnAffine(m)
		}
		add(cnGsk.TransformNodeGetChild(n), local.Then(acc))
	}
	return nd
}

// cnMatrixOf returns the graphene_matrix_t of the GskTransform tr
// (gsk_transform_to_matrix into 16-byte aligned g_malloc memory): 16
// floats, row-major, translation in the last row. ok is false for NULL
// (which is the identity) or when the memory could not be allocated.
func cnMatrixOf(tr uintptr) (m [16]float32, ok bool) {
	if tr == 0 {
		return m, false
	}
	buf := gtk.GLib.Malloc0(gtk.MatrixSize)
	if buf == 0 {
		return m, false
	}
	defer gtk.GLib.Free(buf)
	cnGsk.TransformToMatrix(tr, buf)
	for i := range m {
		m[i] = gtk.ReadFloat32(buf, uintptr(i)*4)
	}
	return m, true
}

// cnAffine is the 2D part of a graphene matrix as a render.Transform:
// graphene maps the row vector (x, y, 0, 1), so x' = m0·x + m4·y + m12
// and y' = m1·x + m5·y + m13.
func cnAffine(m [16]float32) render.Transform {
	return render.Transform{A: float64(m[0]), B: float64(m[1]), C: float64(m[4]), D: float64(m[5]),
		E: float64(m[12]), F: float64(m[13])}
}

// cnMapPoint and cnMapRect apply a render.Transform; a rect maps to the
// bounding box of its corners.
func cnMapPoint(t render.Transform, x, y float64) (float64, float64) {
	return t.A*x + t.C*y + t.E, t.B*x + t.D*y + t.F
}

func cnMapRect(t render.Transform, r gtk.GrapheneRect) gtk.GrapheneRect {
	x0, y0 := float64(r.Origin.X), float64(r.Origin.Y)
	x1, y1 := x0+float64(r.Size.Width), y0+float64(r.Size.Height)
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, p := range [4][2]float64{{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1}} {
		x, y := cnMapPoint(t, p[0], p[1])
		minX, minY = math.Min(minX, x), math.Min(minY, y)
		maxX, maxY = math.Max(maxX, x), math.Max(maxY, y)
	}
	return gtk.Rect(float32(minX), float32(minY), float32(maxX-minX), float32(maxY-minY))
}

// --- Node readers (the root must be alive) ----------------------------------------

func cnColorOf(n *cnNode) gtk.RGBA { return gtk.ReadRGBA(cnGsk.ColorNodeGetColor(n.ptr)) }

func cnClipOf(n *cnNode) gtk.GrapheneRect { return gtk.ReadRect(cnGsk.ClipNodeGetClip(n.ptr), 0) }

func cnRoundedClipOf(n *cnNode) gtk.RoundedRect {
	return gtk.ReadRoundedRect(cnGsk.RoundedClipNodeGetClip(n.ptr))
}

func cnOpacityOf(n *cnNode) float32 { return cnGsk.OpacityNodeGetOpacity(n.ptr) }

func cnBlurOf(n *cnNode) float32 { return cnGsk.BlurNodeGetRadius(n.ptr) }

// cnBorder is what a border node holds.
type cnBorder struct {
	outline gtk.RoundedRect
	widths  [4]float32  // top, right, bottom, left
	colors  [4]gtk.RGBA // top, right, bottom, left
}

func cnBorderOf(n *cnNode) cnBorder {
	b := cnBorder{outline: gtk.ReadRoundedRect(cnGsk.BorderNodeGetOutline(n.ptr))}
	w, c := cnGsk.BorderNodeGetWidths(n.ptr), cnGsk.BorderNodeGetColors(n.ptr)
	for i := range 4 {
		b.widths[i] = gtk.ReadFloat32(w, uintptr(i)*4)
		b.colors[i] = gtk.ReadRGBA(c + uintptr(i)*16) // sizeof(GdkRGBA)
	}
	return b
}

// cnGradient is what a linear-gradient node holds.
type cnGradient struct {
	start, end gtk.GraphenePoint
	nStops     uintptr // gsk_linear_gradient_node_get_n_color_stops
	stops      []gtk.ColorStop
}

// cnColorStopSize is sizeof(GskColorStop): a float and a GdkRGBA.
const cnColorStopSize = 20

func cnGradientOf(n *cnNode) cnGradient {
	point := func(p uintptr) gtk.GraphenePoint {
		return gtk.GraphenePoint{X: gtk.ReadFloat32(p, 0), Y: gtk.ReadFloat32(p, 4)}
	}
	g := cnGradient{
		start:  point(cnGsk.LinearGradientNodeGetStart(n.ptr)),
		end:    point(cnGsk.LinearGradientNodeGetEnd(n.ptr)),
		nStops: cnGsk.LinearGradientNodeGetNColorStops(n.ptr),
	}
	var count uintptr
	p := cnGsk.LinearGradientNodeGetColorStops(n.ptr, &count)
	if p == 0 || count > 64 {
		return g
	}
	for i := range count {
		off := i * cnColorStopSize
		g.stops = append(g.stops, gtk.ColorStop{Offset: gtk.ReadFloat32(p, off), Color: gtk.ReadRGBA(p + off + 4)})
	}
	return g
}

// cnStroke is what a stroke node holds: its GskStroke's line width, cap,
// join, miter limit, dash pattern (nil for none) and dash offset, and its
// path's bounds (ok false if gsk_path_get_bounds found none).
type cnStroke struct {
	width, miter float32
	cap, join    int32
	dash         []float32
	offset       float32
	path         gtk.GrapheneRect
	ok           bool
}

// cnMaxDash bounds the dash pattern read back, against a corrupt count.
const cnMaxDash = 64

func cnStrokeOf(n *cnNode) cnStroke {
	s := cnGsk.StrokeNodeGetStroke(n.ptr)
	st := cnStroke{
		width:  cnGsk.StrokeGetLineWidth(s),
		miter:  cnGsk.StrokeGetMiterLimit(s),
		cap:    cnGsk.StrokeGetLineCap(s),
		join:   cnGsk.StrokeGetLineJoin(s),
		offset: cnGsk.StrokeGetDashOffset(s),
	}
	var count uintptr
	if d := cnGsk.StrokeGetDash(s, &count); d != 0 && count <= cnMaxDash {
		for i := range count {
			st.dash = append(st.dash, gtk.ReadFloat32(d, i*4))
		}
	}
	st.ok = cnGsk.PathGetBounds(cnGsk.StrokeNodeGetPath(n.ptr), &st.path) != 0
	return st
}

// cnFill is what a fill node holds: its GskFillRule and its path's bounds
// (ok false if gsk_path_get_bounds found none). closed is
// gsk_path_is_closed (a single closed contour), known only if it bound.
type cnFill struct {
	rule          int32
	path          gtk.GrapheneRect
	ok            bool
	closed, known bool
}

func cnFillOf(n *cnNode) cnFill {
	p := cnGsk.FillNodeGetPath(n.ptr)
	f := cnFill{rule: cnGsk.FillNodeGetFillRule(n.ptr)}
	f.ok = cnGsk.PathGetBounds(p, &f.path) != 0
	if cnGsk.PathIsClosed != nil {
		f.closed, f.known = cnGsk.PathIsClosed(p) != 0, true
	}
	return f
}

// cnShadow is what an outset or inset shadow node holds: the outline (in
// the node's own coordinates), the color, the offset, the spread and the
// blur radius.
type cnShadow struct {
	outline              gtk.RoundedRect
	color                gtk.RGBA
	dx, dy, spread, blur float32
}

// cnShadowOf reads an outset or inset shadow node (n.typ says which).
func cnShadowOf(n *cnNode) cnShadow {
	if n.typ == gtk.NodeInsetShadow {
		return cnShadow{
			outline: gtk.ReadRoundedRect(cnGsk.InsetShadowNodeGetOutline(n.ptr)),
			color:   gtk.ReadRGBA(cnGsk.InsetShadowNodeGetColor(n.ptr)),
			dx:      cnGsk.InsetShadowNodeGetDx(n.ptr),
			dy:      cnGsk.InsetShadowNodeGetDy(n.ptr),
			spread:  cnGsk.InsetShadowNodeGetSpread(n.ptr),
			blur:    cnGsk.InsetShadowNodeGetBlurRadius(n.ptr),
		}
	}
	return cnShadow{
		outline: gtk.ReadRoundedRect(cnGsk.OutsetShadowNodeGetOutline(n.ptr)),
		color:   gtk.ReadRGBA(cnGsk.OutsetShadowNodeGetColor(n.ptr)),
		dx:      cnGsk.OutsetShadowNodeGetDx(n.ptr),
		dy:      cnGsk.OutsetShadowNodeGetDy(n.ptr),
		spread:  cnGsk.OutsetShadowNodeGetSpread(n.ptr),
		blur:    cnGsk.OutsetShadowNodeGetBlurRadius(n.ptr),
	}
}

func cnIsShadow(n *cnNode) bool { return n.typ == gtk.NodeOutsetShadow || n.typ == gtk.NodeInsetShadow }

// cnNodeText is GSK_TEXT_NODE (gskenums.h, stable since 4.0; the gtk
// package has no constant for it, as the shell never reads nodes).
const cnNodeText int32 = 22

// cnText is what a text node holds: its color, its offset (the baseline
// origin of its glyph run, in the node's coordinates), its glyph count
// and its PangoFont (only compared with 0).
type cnText struct {
	color  gtk.RGBA
	offset gtk.GraphenePoint
	glyphs uint32
	font   uintptr
}

func cnTextOf(n *cnNode) cnText {
	off := cnGsk.TextNodeGetOffset(n.ptr)
	return cnText{
		color:  gtk.ReadRGBA(cnGsk.TextNodeGetColor(n.ptr)),
		offset: gtk.GraphenePoint{X: gtk.ReadFloat32(off, 0), Y: gtk.ReadFloat32(off, 4)},
		glyphs: cnGsk.TextNodeGetNumGlyphs(n.ptr),
		font:   cnGsk.TextNodeGetFont(n.ptr),
	}
}

// --- Searching and comparing ------------------------------------------------------

// find returns every node under n (n included) for which ok is true, in
// depth-first order.
func (n *cnNode) find(ok func(*cnNode) bool) []*cnNode {
	if n == nil {
		return nil
	}
	var out []*cnNode
	if ok(n) {
		out = append(out, n)
	}
	for _, k := range n.kids {
		out = append(out, k.find(ok)...)
	}
	return out
}

// ancestor returns the nearest ancestor of n of one of the types, or nil.
func (n *cnNode) ancestor(types ...int32) *cnNode {
	for a := n.parent; a != nil; a = a.parent {
		for _, typ := range types {
			if a.typ == typ {
				return a
			}
		}
	}
	return nil
}

// clipper returns the nearest clip or rounded-clip ancestor of n whose clip
// rect is near r, or nil.
func (n *cnNode) clipper(r gtk.GrapheneRect) *cnNode {
	for a := n.parent; a != nil; a = a.parent {
		switch a.typ {
		case gtk.NodeClip:
			if cnRectNear(cnClipOf(a), r) {
				return a
			}
		case gtk.NodeRoundedClip:
			if cnRectNear(cnRoundedClipOf(a).Bounds, r) {
				return a
			}
		}
	}
	return nil
}

// colorNodes returns the color nodes under n of color c.
func (n *cnNode) colorNodes(c render.Color) []*cnNode {
	want := rgba(c)
	return n.find(func(k *cnNode) bool { return k.typ == gtk.NodeColor && cnRGBANear(cnColorOf(k), want) })
}

// shadowNodes returns the shadow nodes of type typ (gtk.NodeOutsetShadow
// or gtk.NodeInsetShadow) under n whose color is near want.
func (n *cnNode) shadowNodes(typ int32, want gtk.RGBA) []*cnNode {
	return n.find(func(k *cnNode) bool { return k.typ == typ && cnRGBANear(cnShadowOf(k).color, want) })
}

// toRoot is the transform from n's own coordinates (those its getters
// answer in) to the root's: the matrices of the transform nodes above n,
// innermost first.
func (n *cnNode) toRoot() render.Transform {
	acc := render.Identity()
	for a := n.parent; a != nil; a = a.parent {
		if a.typ != gtk.NodeTransform {
			continue
		}
		if m, ok := cnMatrixOf(cnGsk.TransformNodeGetTransform(a.ptr)); ok {
			acc = acc.Then(cnAffine(m))
		}
	}
	return acc
}

// borderNodes returns the border nodes under n whose four colors are c.
func (n *cnNode) borderNodes(c render.Color) []*cnNode {
	want := rgba(c)
	return n.find(func(k *cnNode) bool {
		if k.typ != gtk.NodeBorder {
			return false
		}
		for _, bc := range cnBorderOf(k).colors {
			if !cnRGBANear(bc, want) {
				return false
			}
		}
		return true
	})
}

// cnOne returns the single node of nodes, or an error naming what.
func cnOne(what string, nodes []*cnNode) (*cnNode, error) {
	if len(nodes) != 1 {
		return nil, fmt.Errorf("%d %s, want exactly one", len(nodes), what)
	}
	return nodes[0], nil
}

const cnEps = 1e-3

func cnNear(a, b float32) bool { return math.Abs(float64(a)-float64(b)) <= cnEps }

func cnRectNear(a, b gtk.GrapheneRect) bool {
	return cnNear(a.Origin.X, b.Origin.X) && cnNear(a.Origin.Y, b.Origin.Y) &&
		cnNear(a.Size.Width, b.Size.Width) && cnNear(a.Size.Height, b.Size.Height)
}

func cnRoundedNear(a, b gtk.RoundedRect) bool {
	if !cnRectNear(a.Bounds, b.Bounds) {
		return false
	}
	for i := range a.Corner {
		if !cnNear(a.Corner[i].Width, b.Corner[i].Width) || !cnNear(a.Corner[i].Height, b.Corner[i].Height) {
			return false
		}
	}
	return true
}

// cnRGBANear compares colors to 1e-4: GTK 4.16+ keeps colors as GdkColor
// and hands GdkRGBA copies back.
func cnRGBANear(a, b gtk.RGBA) bool {
	n := func(x, y float32) bool { return math.Abs(float64(x)-float64(y)) <= 1e-4 }
	return n(a.R, b.R) && n(a.G, b.G) && n(a.B, b.B) && n(a.A, b.A)
}

func cnRect(x, y, w, h float32) gtk.GrapheneRect { return gtk.Rect(x, y, w, h) }

func cnRounded(r gtk.GrapheneRect, tl, tr, br, bl float32) gtk.RoundedRect {
	rr := gtk.RoundedRect{Bounds: r}
	for i, v := range [4]float32{tl, tr, br, bl} {
		rr.Corner[i] = gtk.GrapheneSize{Width: v, Height: v}
	}
	return rr
}

// --- Dumping ----------------------------------------------------------------------

var cnTypeNames = map[int32]string{
	1: "container", 2: "cairo", 3: "color", 4: "linear-gradient", 5: "repeating-linear-gradient",
	6: "radial-gradient", 7: "repeating-radial-gradient", 8: "conic-gradient", 9: "border",
	10: "texture", 11: "inset-shadow", 12: "outset-shadow", 13: "transform", 14: "opacity",
	15: "color-matrix", 16: "repeat", 17: "clip", 18: "rounded-clip", 19: "shadow", 20: "blend",
	21: "cross-fade", 22: "text", 23: "blur", 24: "debug", 25: "gl-shader", 26: "texture-scale",
	27: "mask", 28: "fill", 29: "stroke", 30: "subsurface",
}

func cnF(v float32) string { return fmt.Sprintf("%g", v) }

func cnRectString(r gtk.GrapheneRect) string {
	return "(" + cnF(r.Origin.X) + "," + cnF(r.Origin.Y) + " " + cnF(r.Size.Width) + "×" + cnF(r.Size.Height) + ")"
}

func cnRoundedString(r gtk.RoundedRect) string {
	s := cnRectString(r.Bounds) + " r="
	for i, c := range r.Corner {
		if i > 0 {
			s += "/"
		}
		s += cnF(c.Width)
		if c.Height != c.Width {
			s += "x" + cnF(c.Height)
		}
	}
	return s
}

func cnRGBAString(c gtk.RGBA) string {
	return "rgba(" + cnF(c.R) + "," + cnF(c.G) + "," + cnF(c.B) + "," + cnF(c.A) + ")"
}

// cnTransformCategories names GskTransformCategory.
var cnTransformCategories = []string{"unknown", "any", "3d", "2d", "2d-affine", "2d-translate", "identity"}

func cnCategory(tr uintptr) string {
	if tr == 0 {
		return "NULL"
	}
	c := cnGsk.TransformGetCategory(tr)
	if c >= 0 && int(c) < len(cnTransformCategories) {
		return cnTransformCategories[c]
	}
	return fmt.Sprintf("category %d", c)
}

// dump returns the tree under n, one node per line.
func (n *cnNode) dump() string {
	if n == nil {
		return "  (no node)\n"
	}
	var b strings.Builder
	n.dumpTo(&b, 1)
	return b.String()
}

func (n *cnNode) dumpTo(b *strings.Builder, depth int) {
	name, ok := cnTypeNames[n.typ]
	if !ok {
		name = fmt.Sprintf("type-%d", n.typ)
	}
	fmt.Fprintf(b, "%s%s %s", strings.Repeat("  ", depth), name, cnRectString(n.bounds))
	if n.global != n.bounds {
		fmt.Fprintf(b, " [global %s]", cnRectString(n.global))
	}
	switch n.typ {
	case gtk.NodeColor:
		fmt.Fprintf(b, " %s", cnRGBAString(cnColorOf(n)))
	case gtk.NodeClip:
		fmt.Fprintf(b, " clip %s", cnRectString(cnClipOf(n)))
	case gtk.NodeRoundedClip:
		fmt.Fprintf(b, " clip %s", cnRoundedString(cnRoundedClipOf(n)))
	case gtk.NodeStroke:
		st := cnStrokeOf(n)
		fmt.Fprintf(b, " width %g cap %d join %d miter %g", st.width, st.cap, st.join, st.miter)
		if st.dash != nil {
			fmt.Fprintf(b, " dash %v offset %g", st.dash, st.offset)
		}
		fmt.Fprintf(b, " path %s", cnRectString(st.path))
	case gtk.NodeFill:
		f := cnFillOf(n)
		fmt.Fprintf(b, " rule %d path %s", f.rule, cnRectString(f.path))
		if f.known {
			fmt.Fprintf(b, " closed %v", f.closed)
		}
	case gtk.NodeOutsetShadow, gtk.NodeInsetShadow:
		sh := cnShadowOf(n)
		fmt.Fprintf(b, " outline %s %s offset (%g,%g) spread %g blur %g",
			cnRoundedString(sh.outline), cnRGBAString(sh.color), sh.dx, sh.dy, sh.spread, sh.blur)
	case gtk.NodeOpacity:
		fmt.Fprintf(b, " opacity %g", cnOpacityOf(n))
	case gtk.NodeBlur:
		fmt.Fprintf(b, " radius %g", cnBlurOf(n))
	case cnNodeText:
		tx := cnTextOf(n)
		fmt.Fprintf(b, " %s offset (%g,%g) glyphs %d", cnRGBAString(tx.color), tx.offset.X, tx.offset.Y, tx.glyphs)
	case gtk.NodeBorder:
		bd := cnBorderOf(n)
		fmt.Fprintf(b, " outline %s widths %v colors %s", cnRoundedString(bd.outline), bd.widths, cnRGBAString(bd.colors[0]))
	case gtk.NodeLinearGradient:
		g := cnGradientOf(n)
		fmt.Fprintf(b, " (%g,%g)→(%g,%g)", g.start.X, g.start.Y, g.end.X, g.end.Y)
		for _, s := range g.stops {
			fmt.Fprintf(b, " %g:%s", s.Offset, cnRGBAString(s.Color))
		}
	case gtk.NodeTransform:
		tr := cnGsk.TransformNodeGetTransform(n.ptr)
		fmt.Fprintf(b, " %s", cnCategory(tr))
		if m, ok := cnMatrixOf(tr); ok {
			a := cnAffine(m)
			fmt.Fprintf(b, " 2d(a=%g b=%g c=%g d=%g e=%g f=%g)", a.A, a.B, a.C, a.D, a.E, a.F)
		}
	}
	b.WriteByte('\n')
	for _, k := range n.kids {
		k.dumpTo(b, depth+1)
	}
}

// --- Painting into a GtkSnapshot --------------------------------------------------

// cnPaint paints draw through the canvas (with the recorder) into
// a fresh GtkSnapshot of a surface of size at scale, finishes the canvas,
// takes the node and passes its tree and finish's error to check, on the
// GTK goroutine. The tree is logged (CANVAS-RESULT), and again with
// check's error. A panic in draw or check fails the test.
func cnPaint(t *testing.T, name string, size render.Size, scale float64, draw func(c *canvas), check func(root *cnNode, finishErr error) error) {
	t.Helper()
	cnNeed(t)
	onGTK(t, func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("%s: panicked: %v", name, r)
			}
		}()
		s := gtk.Gtk.SnapshotNew()
		if s == 0 {
			return fmt.Errorf("%s: gtk_snapshot_new returned NULL", name)
		}
		c := newCanvas(newGtkSnapshot(s), size, scale, true)
		draw(c)
		ferr := c.finish()
		node := gtk.Gtk.SnapshotFreeToNode(s)
		if node == 0 {
			return fmt.Errorf("%s: gtk_snapshot_free_to_node returned NULL: GTK lost the drawing (finish: %v)", name, ferr)
		}
		defer gtk.Gsk.RenderNodeUnref(node)
		root := cnTree(node)
		t.Logf("CANVAS-RESULT: %s (GTK %v):\n%s", name, gtk.Version(), root.dump())
		if err := check(root, ferr); err != nil {
			return fmt.Errorf("%s: %v\nnode tree:\n%s", name, err, root.dump())
		}
		return nil
	})
}

// cnNoErr is the finish check of a balanced paint.
func cnNoErr(err error) error {
	if err != nil {
		return fmt.Errorf("finish returned the recorder errors %v, want none", err)
	}
	return nil
}

func cnSolid(r, g, b, a float32) render.Paint { return render.Solid(driver.RGBA(r, g, b, a)) }

var (
	cnRed    = driver.RGBA(0.9, 0.1, 0.2, 1)
	cnGreen  = driver.RGBA(0.1, 0.8, 0.3, 1)
	cnBlue   = driver.RGBA(0.2, 0.3, 0.9, 1)
	cnOrange = driver.RGBA(1, 0.6, 0.1, 1)
	cnTeal   = driver.RGBA(0.1, 0.6, 0.6, 0.75)
)

var cnSize = render.Size{W: 200, H: 100}

// --- Tests ------------------------------------------------------------------------

// TestCanvasGTKFillSharp: a sharp fill is one color node with the rect's
// bounds and the color, under no rounded clip; an invisible or empty fill
// adds no node.
func TestCanvasGTKFillSharp(t *testing.T) {
	cnPaint(t, "sharp fill", cnSize, 1, func(c *canvas) {
		c.FillRoundedRect(render.Rect{X: 10, Y: 20, W: 30, H: 40}, render.Radii{}, render.Solid(cnRed))
		c.FillRoundedRect(render.Rect{X: 60, Y: 20, W: 30, H: 40}, render.Radii{}, render.Solid(cnGreen))
		c.FillRoundedRect(render.Rect{X: 100, Y: 20, W: 30, H: 40}, render.Radii{}, cnSolid(0, 0, 1, 0)) // transparent
		c.FillRoundedRect(render.Rect{X: 100, Y: 20, W: 0, H: 40}, render.Radii{}, render.Solid(cnBlue)) // empty
	}, func(root *cnNode, ferr error) error {
		for _, w := range []struct {
			name string
			c    render.Color
			r    gtk.GrapheneRect
		}{{"red", cnRed, cnRect(10, 20, 30, 40)}, {"green", cnGreen, cnRect(60, 20, 30, 40)}} {
			n, err := cnOne(w.name+" color nodes", root.colorNodes(w.c))
			if err != nil {
				return err
			}
			if !cnRectNear(n.global, w.r) {
				return fmt.Errorf("the %s color node is at %s, want %s", w.name, cnRectString(n.global), cnRectString(w.r))
			}
			if a := n.ancestor(gtk.NodeRoundedClip, gtk.NodeOpacity, gtk.NodeTransform); a != nil {
				return fmt.Errorf("the sharp %s fill is under a %s node", w.name, cnTypeNames[a.typ])
			}
		}
		if got := root.find(func(k *cnNode) bool { return k.typ == gtk.NodeColor }); len(got) != 2 {
			return fmt.Errorf("%d color nodes, want 2 (a transparent or empty fill drew)", len(got))
		}
		return cnNoErr(ferr)
	})
}

// TestCanvasGTKSurfaceClip: the canvas clips to the surface's rect, so a
// fill crossing its edge is cut there: either under a (rounded) clip node
// of the surface's rect or with its bounds intersected. A fill inside an
// empty clip (negative size) draws nothing; one after it does.
func TestCanvasGTKSurfaceClip(t *testing.T) {
	cnPaint(t, "surface clip", cnSize, 1, func(c *canvas) {
		c.FillRoundedRect(render.Rect{X: 190, Y: 90, W: 30, H: 30}, render.Radii{}, render.Solid(cnRed))
		c.Save()
		c.ClipRoundedRect(render.Rect{X: 10, Y: 10, W: -5, H: 5}, render.Radii{})
		c.FillRoundedRect(render.Rect{X: 0, Y: 0, W: 50, H: 50}, render.Radii{}, render.Solid(cnBlue))
		c.Restore()
		c.FillRoundedRect(render.Rect{X: 20, Y: 20, W: 10, H: 10}, render.Radii{}, render.Solid(cnGreen))
	}, func(root *cnNode, ferr error) error {
		n, err := cnOne("red color nodes", root.colorNodes(cnRed))
		if err != nil {
			return err
		}
		surf := cnRect(0, 0, 200, 100)
		switch clip := n.clipper(surf); {
		case clip != nil:
			t.Logf("CANVAS-RESULT: surface clip: a fill crossing the edge is under a %s node of the surface rect", cnTypeNames[clip.typ])
		case cnRectNear(n.global, cnRect(190, 90, 10, 10)):
			t.Logf("CANVAS-RESULT: surface clip: GTK intersected the color node's bounds with the surface clip")
		default:
			return fmt.Errorf("the fill crossing the surface edge is at %s with no clip of the surface rect %s above it",
				cnRectString(n.global), cnRectString(surf))
		}
		for _, b := range root.colorNodes(cnBlue) {
			// GTK drops a node outside its clip; if it kept one, the clip
			// must leave nothing of it.
			ok := false
			for a := b.parent; a != nil; a = a.parent {
				if a.typ == gtk.NodeClip && (cnClipOf(a).Size.Width <= 0 || cnClipOf(a).Size.Height <= 0) ||
					a.typ == gtk.NodeRoundedClip && (cnRoundedClipOf(a).Bounds.Size.Width <= 0 || cnRoundedClipOf(a).Bounds.Size.Height <= 0) {
					ok = true
				}
			}
			if !ok {
				return fmt.Errorf("a fill inside an empty clip drew a color node at %s with no empty clip above it", cnRectString(b.global))
			}
			t.Logf("CANVAS-RESULT: surface clip: GTK kept a node inside an empty clip")
		}
		g, err := cnOne("green color nodes (the fill after Restore)", root.colorNodes(cnGreen))
		if err != nil {
			return err
		}
		if !cnRectNear(g.global, cnRect(20, 20, 10, 10)) {
			return fmt.Errorf("the fill after the empty clip is at %s, want (20,20 10×10)", cnRectString(g.global))
		}
		return cnNoErr(ferr)
	})
}

// TestCanvasGTKFillRounded: a rounded fill is a color node of the rect
// inside a rounded clip of the rect with the four radii (top-left,
// top-right, bottom-right, bottom-left), clamped to half the shorter side.
func TestCanvasGTKFillRounded(t *testing.T) {
	cnPaint(t, "rounded fill", cnSize, 1, func(c *canvas) {
		c.FillRoundedRect(render.Rect{X: 20, Y: 20, W: 100, H: 60},
			render.Radii{TopLeft: 8, TopRight: 4, BottomRight: 12, BottomLeft: 0}, render.Solid(cnBlue))
		// Radii beyond half the shorter side (10) are clamped.
		c.FillRoundedRect(render.Rect{X: 150, Y: 10, W: 40, H: 20}, render.UniformRadii(50), render.Solid(cnOrange))
	}, func(root *cnNode, ferr error) error {
		for _, w := range []struct {
			name string
			c    render.Color
			clip gtk.RoundedRect
		}{
			{"blue", cnBlue, cnRounded(cnRect(20, 20, 100, 60), 8, 4, 12, 0)},
			{"orange (clamped)", cnOrange, cnRounded(cnRect(150, 10, 40, 20), 10, 10, 10, 10)},
		} {
			n, err := cnOne(w.name+" color nodes", root.colorNodes(w.c))
			if err != nil {
				return err
			}
			if !cnRectNear(n.global, w.clip.Bounds) {
				return fmt.Errorf("the %s color node is at %s, want %s", w.name, cnRectString(n.global), cnRectString(w.clip.Bounds))
			}
			rc := n.ancestor(gtk.NodeRoundedClip)
			if rc == nil {
				return fmt.Errorf("the %s rounded fill is under no rounded-clip node: its corners are sharp", w.name)
			}
			if got := cnRoundedClipOf(rc); !cnRoundedNear(got, w.clip) {
				return fmt.Errorf("the %s fill's rounded clip is %s, want %s", w.name, cnRoundedString(got), cnRoundedString(w.clip))
			}
		}
		return cnNoErr(ferr)
	})
}

// TestCanvasGTKStroke: a stroke is one border node whose outline is the
// rect grown by half the width (rounded corners grown by it too, sharp
// ones kept sharp), with the uniform width and color on all four sides.
func TestCanvasGTKStroke(t *testing.T) {
	cnPaint(t, "stroke", cnSize, 1, func(c *canvas) {
		c.StrokeRoundedRect(render.Rect{X: 20, Y: 20, W: 100, H: 60}, render.UniformRadii(10), 4, render.Solid(cnRed))
		c.StrokeRoundedRect(render.Rect{X: 150, Y: 10, W: 30, H: 30}, render.Radii{}, 2, render.Solid(cnTeal))
		c.StrokeRoundedRect(render.Rect{X: 150, Y: 50, W: 30, H: 30}, render.Radii{}, 0, render.Solid(cnBlue)) // no width
	}, func(root *cnNode, ferr error) error {
		for _, w := range []struct {
			name    string
			c       render.Color
			width   float32
			outline gtk.RoundedRect
		}{
			{"rounded red", cnRed, 4, cnRounded(cnRect(18, 18, 104, 64), 12, 12, 12, 12)},
			{"sharp teal", cnTeal, 2, cnRounded(cnRect(149, 9, 32, 32), 0, 0, 0, 0)},
		} {
			n, err := cnOne(w.name+" border nodes", root.borderNodes(w.c))
			if err != nil {
				return err
			}
			b := cnBorderOf(n)
			if !cnRoundedNear(b.outline, w.outline) {
				return fmt.Errorf("the %s stroke's outline is %s, want %s", w.name, cnRoundedString(b.outline), cnRoundedString(w.outline))
			}
			for i, bw := range b.widths {
				if !cnNear(bw, w.width) {
					return fmt.Errorf("the %s stroke's widths are %v, want %v on every side (side %d)", w.name, b.widths, w.width, i)
				}
			}
			if !cnRectNear(n.global, w.outline.Bounds) {
				return fmt.Errorf("the %s border node's bounds are %s, want the outline %s", w.name, cnRectString(n.global), cnRectString(w.outline.Bounds))
			}
		}
		if got := root.find(func(k *cnNode) bool { return k.typ == gtk.NodeBorder }); len(got) != 2 {
			return fmt.Errorf("%d border nodes, want 2 (a zero-width stroke drew)", len(got))
		}
		return cnNoErr(ferr)
	})
}

// TestCanvasGTKTranslate: translations and scales move fills (exact
// bounds in the root's coordinates), Save/Restore keeps and restores
// them, and a Concat without rotation or skew acts as a translate and a
// scale.
func TestCanvasGTKTranslate(t *testing.T) {
	cnPaint(t, "translate", cnSize, 1, func(c *canvas) {
		c.Translate(30, 40)
		c.FillRoundedRect(render.Rect{X: 5, Y: 5, W: 10, H: 10}, render.Radii{}, render.Solid(cnRed))
		c.Save()
		c.Translate(10, 0)
		c.Scale(2, 2)
		c.FillRoundedRect(render.Rect{X: 1, Y: 1, W: 5, H: 5}, render.Radii{}, render.Solid(cnGreen))
		c.Restore()
		c.FillRoundedRect(render.Rect{X: 0, Y: 0, W: 1, H: 1}, render.Radii{}, render.Solid(cnBlue))
		c.Save()
		c.Concat(render.Transform{A: 2, D: 3, E: 100, F: -30})
		c.FillRoundedRect(render.Rect{X: 1, Y: 1, W: 2, H: 2}, render.Radii{}, render.Solid(cnOrange))
		c.Restore()
	}, func(root *cnNode, ferr error) error {
		for _, w := range []struct {
			name string
			c    render.Color
			r    gtk.GrapheneRect
		}{
			{"translated", cnRed, cnRect(35, 45, 10, 10)},
			{"translated and scaled", cnGreen, cnRect(42, 42, 10, 10)},
			{"after Restore", cnBlue, cnRect(30, 40, 1, 1)},
			{"after Concat(scale+translate)", cnOrange, cnRect(132, 13, 4, 6)},
		} {
			n, err := cnOne(w.name+" color nodes", root.colorNodes(w.c))
			if err != nil {
				return err
			}
			if !cnRectNear(n.global, w.r) {
				return fmt.Errorf("the %s fill is at %s, want %s", w.name, cnRectString(n.global), cnRectString(w.r))
			}
			if n.ancestor(gtk.NodeTransform) != nil {
				t.Logf("CANVAS-RESULT: translate: the %s fill is under a transform node (GTK did not fold it into the bounds)", w.name)
			}
		}
		return cnNoErr(ferr)
	})
}

// TestCanvasGTKConcat: a Concat with a skew or a rotation is a transform
// node whose matrix is graphene's layout of render.Transform (rows
// (a,b,0,0), (c,d,0,0), (0,0,1,0), (e,f,0,1)): x' = a·x + c·y + e,
// y' = b·x + d·y + f. The node maps a point and the fill's corners as
// render.Transform does. A rotation by π/2 after a translation turns
// clockwise on screen (Y down) about the translated origin.
func TestCanvasGTKConcat(t *testing.T) {
	sz := render.Size{W: 200, H: 200}
	m := render.Transform{A: 2, B: 0.5, C: -0.25, D: 1.5, E: 30, F: 40}
	fill := render.Rect{X: 10, Y: 20, W: 20, H: 10}
	cnPaint(t, "concat", sz, 1, func(c *canvas) {
		c.Save()
		c.Concat(m)
		c.FillRoundedRect(fill, render.Radii{}, render.Solid(cnRed))
		c.Restore()
		c.Save()
		c.Translate(100, 100)
		c.Concat(render.Rotation(math.Pi / 2))
		c.FillRoundedRect(render.Rect{X: 10, Y: 0, W: 20, H: 10}, render.Radii{}, render.Solid(cnGreen))
		c.Restore()
		c.FillRoundedRect(render.Rect{X: 1, Y: 1, W: 2, H: 2}, render.Radii{}, render.Solid(cnBlue))
	}, func(root *cnNode, ferr error) error {
		red, err := cnOne("red color nodes", root.colorNodes(cnRed))
		if err != nil {
			return err
		}
		tn := red.ancestor(gtk.NodeTransform)
		if tn == nil {
			return errors.New("the skewed fill is under no transform node")
		}
		tr := cnGsk.TransformNodeGetTransform(tn.ptr)
		got, ok := cnMatrixOf(tr)
		if !ok {
			return errors.New("gsk_transform_node_get_transform gave NULL (or g_malloc0 failed)")
		}
		want := [16]float32{2, 0.5, 0, 0, -0.25, 1.5, 0, 0, 0, 0, 1, 0, 30, 40, 0, 1}
		for i := range want {
			if !cnNear(got[i], want[i]) {
				return fmt.Errorf("the transform node's matrix is %v, want %v (element %d): gtkSnapshot.matrix's float order is wrong", got, want, i)
			}
		}
		t.Logf("CANVAS-RESULT: concat: a skew is a transform node of category %s", cnCategory(tr))
		if cnGsk.TransformTransformPoint != nil {
			var out gtk.GraphenePoint
			in := gtk.GraphenePoint{X: 10, Y: 20}
			cnGsk.TransformTransformPoint(tr, &in, &out)
			if !cnNear(out.X, 45) || !cnNear(out.Y, 75) {
				return fmt.Errorf("gsk_transform_transform_point maps (10,20) to (%g,%g), want (45,75)", out.X, out.Y)
			}
		} else {
			t.Log("CANVAS-RESULT: concat: gsk_transform_transform_point did not bind; the point check is skipped")
		}
		wantBox := cnMapRect(m, cnRect(float32(fill.X), float32(fill.Y), float32(fill.W), float32(fill.H)))
		if !cnRectNear(red.global, wantBox) || !cnRectNear(tn.global, wantBox) {
			return fmt.Errorf("the skewed fill covers %s (transform node %s), want %s",
				cnRectString(red.global), cnRectString(tn.global), cnRectString(wantBox))
		}

		green, err := cnOne("green color nodes", root.colorNodes(cnGreen))
		if err != nil {
			return err
		}
		// (10,0)-(30,10) turned clockwise by 90° is x ∈ [-10,0], y ∈ [10,30],
		// moved by (100,100). The transpose would give (100,70 10×20).
		if w := cnRect(90, 110, 10, 20); !cnRectNear(green.global, w) {
			return fmt.Errorf("the rotated fill is at %s, want %s", cnRectString(green.global), cnRectString(w))
		}
		if green.ancestor(gtk.NodeTransform) == nil {
			return errors.New("the rotated fill is under no transform node")
		}
		blue, err := cnOne("blue color nodes (after Restore)", root.colorNodes(cnBlue))
		if err != nil {
			return err
		}
		if !cnRectNear(blue.global, cnRect(1, 1, 2, 2)) || blue.ancestor(gtk.NodeTransform) != nil {
			return fmt.Errorf("the fill after Restore is at %s (under a transform node: %v), want (1,1 2×2) untransformed",
				cnRectString(blue.global), blue.ancestor(gtk.NodeTransform) != nil)
		}
		return cnNoErr(ferr)
	})
}

// TestCanvasGTKOpacity: PushOpacity(0.5) … Pop is an opacity node of 0.5
// around what was drawn in between, and a fill after Pop is not under it.
// Opacities 1 and 0 are logged: GTK may drop the node or the content.
func TestCanvasGTKOpacity(t *testing.T) {
	cnPaint(t, "opacity", cnSize, 1, func(c *canvas) {
		c.PushOpacity(0.5)
		c.FillRoundedRect(render.Rect{X: 10, Y: 10, W: 20, H: 20}, render.Radii{}, render.Solid(cnRed))
		c.Pop()
		c.FillRoundedRect(render.Rect{X: 60, Y: 10, W: 20, H: 20}, render.Radii{}, render.Solid(cnGreen))
		c.PushOpacity(1)
		c.FillRoundedRect(render.Rect{X: 100, Y: 10, W: 20, H: 20}, render.Radii{}, render.Solid(cnBlue))
		c.Pop()
		c.PushOpacity(0)
		c.FillRoundedRect(render.Rect{X: 140, Y: 10, W: 20, H: 20}, render.Radii{}, render.Solid(cnOrange))
		c.Pop()
	}, func(root *cnNode, ferr error) error {
		red := root.colorNodes(cnRed)
		switch {
		case len(red) == 1:
			op := red[0].ancestor(gtk.NodeOpacity)
			if op == nil {
				return errors.New("the fill inside PushOpacity(0.5) is under no opacity node")
			}
			if got := cnOpacityOf(op); got != 0.5 {
				return fmt.Errorf("the opacity node's opacity is %g, want 0.5", got)
			}
			if !cnRectNear(op.global, cnRect(10, 10, 20, 20)) {
				return fmt.Errorf("the opacity node is at %s, want (10,10 20×20)", cnRectString(op.global))
			}
		case len(root.colorNodes(driver.RGBA(cnRed.R, cnRed.G, cnRed.B, 0.5))) == 1:
			t.Log("CANVAS-RESULT: opacity: GTK folded the opacity 0.5 into the color node")
		default:
			return fmt.Errorf("%d red color nodes and no half-transparent one, want one", len(red))
		}
		green, err := cnOne("green color nodes (after Pop)", root.colorNodes(cnGreen))
		if err != nil {
			return err
		}
		if green.ancestor(gtk.NodeOpacity) != nil {
			return errors.New("the fill after Pop is still under an opacity node")
		}
		if blue := root.colorNodes(cnBlue); len(blue) == 1 {
			op := blue[0].ancestor(gtk.NodeOpacity)
			kept := op != nil
			t.Logf("CANVAS-RESULT: opacity: PushOpacity(1) kept an opacity node: %v", kept)
		} else {
			return fmt.Errorf("%d blue color nodes (inside PushOpacity(1)), want one", len(blue))
		}
		t.Logf("CANVAS-RESULT: opacity: PushOpacity(0) left %d color nodes of its content", len(root.colorNodes(cnOrange)))
		return cnNoErr(ferr)
	})
}

// TestCanvasGTKLinearGradient: a gradient fill is a linear-gradient node
// with the rect's bounds, the start and end points and the stops (sorted);
// a rounded one is inside a rounded clip (gradient strokes:
// TestCanvasGTKGradientStroke).
func TestCanvasGTKLinearGradient(t *testing.T) {
	stops := []render.GradientStop{{Offset: 1, Color: cnBlue}, {Offset: 0, Color: cnRed}, {Offset: 0.25, Color: cnGreen}}
	cnPaint(t, "linear gradient", cnSize, 1, func(c *canvas) {
		c.FillRoundedRect(render.Rect{X: 10, Y: 10, W: 100, H: 40}, render.Radii{},
			render.LinearGradient(render.Point{X: 10, Y: 10}, render.Point{X: 110, Y: 30}, stops...))
		c.FillRoundedRect(render.Rect{X: 120, Y: 10, W: 60, H: 40}, render.UniformRadii(6),
			render.LinearGradient(render.Point{X: 120, Y: 10}, render.Point{X: 120, Y: 50},
				render.GradientStop{Offset: 0, Color: cnOrange}, render.GradientStop{Offset: 1, Color: cnTeal}))
	}, func(root *cnNode, ferr error) error {
		grads := root.find(func(k *cnNode) bool { return k.typ == gtk.NodeLinearGradient })
		if len(grads) != 2 {
			return fmt.Errorf("%d linear-gradient nodes, want 2", len(grads))
		}
		var first, second *cnNode
		for _, g := range grads {
			if cnRectNear(g.global, cnRect(10, 10, 100, 40)) {
				first = g
			} else if cnRectNear(g.global, cnRect(120, 10, 60, 40)) {
				second = g
			}
		}
		if first == nil || second == nil {
			return errors.New("no linear-gradient node at (10,10 100×40) and (120,10 60×40)")
		}
		g := cnGradientOf(first)
		if !cnNear(g.start.X, 10) || !cnNear(g.start.Y, 10) || !cnNear(g.end.X, 110) || !cnNear(g.end.Y, 30) {
			return fmt.Errorf("the gradient runs (%g,%g)→(%g,%g), want (10,10)→(110,30)", g.start.X, g.start.Y, g.end.X, g.end.Y)
		}
		want := []gtk.ColorStop{{Offset: 0, Color: rgba(cnRed)}, {Offset: 0.25, Color: rgba(cnGreen)}, {Offset: 1, Color: rgba(cnBlue)}}
		if g.nStops != uintptr(len(want)) || len(g.stops) != len(want) {
			return fmt.Errorf("the gradient has %d stops (get_n_color_stops %d), want %d", len(g.stops), g.nStops, len(want))
		}
		for i, s := range g.stops {
			if !cnNear(s.Offset, want[i].Offset) || !cnRGBANear(s.Color, want[i].Color) {
				return fmt.Errorf("stop %d is %g %s, want %g %s", i, s.Offset, cnRGBAString(s.Color), want[i].Offset, cnRGBAString(want[i].Color))
			}
		}
		if first.ancestor(gtk.NodeRoundedClip) != nil {
			return errors.New("the sharp gradient fill is under a rounded clip")
		}
		rc := second.ancestor(gtk.NodeRoundedClip)
		if rc == nil {
			return errors.New("the rounded gradient fill is under no rounded clip")
		}
		if got, w := cnRoundedClipOf(rc), cnRounded(cnRect(120, 10, 60, 40), 6, 6, 6, 6); !cnRoundedNear(got, w) {
			return fmt.Errorf("the rounded gradient's clip is %s, want %s", cnRoundedString(got), cnRoundedString(w))
		}
		return cnNoErr(ferr)
	})
}

// TestCanvasGTKGradientStroke (S4.2b): a gradient stroke of a rounded rect
// is a stroke node whose GskStroke has the width and whose path bounds
// are the rect, holding a linear-gradient node over the stroke's outer
// bounds with the gradient's points and stops; the stroke node itself lies
// within those bounds, nests under an open opacity, and no border node is
// drawn in its place.
func TestCanvasGTKGradientStroke(t *testing.T) {
	type want struct {
		name  string
		rect  gtk.GrapheneRect // the rect stroked, the path's bounds
		outer gtk.GrapheneRect // grown by half the width
		width float32
		from  render.Point
		to    render.Point
		stops []gtk.ColorStop
		layer bool // under the opacity node
	}
	ws := []want{
		{"rounded, in an opacity", cnRect(20, 20, 100, 60), cnRect(18, 18, 104, 64), 4,
			render.Point{X: 20, Y: 20}, render.Point{X: 120, Y: 20},
			[]gtk.ColorStop{{Offset: 0, Color: rgba(cnRed)}, {Offset: 1, Color: rgba(cnBlue)}}, true},
		{"sharp", cnRect(150, 10, 30, 30), cnRect(149, 9, 32, 32), 2,
			render.Point{X: 150, Y: 10}, render.Point{X: 150, Y: 40},
			[]gtk.ColorStop{{Offset: 0, Color: rgba(cnOrange)}, {Offset: 0.5, Color: rgba(cnGreen)}, {Offset: 1, Color: rgba(cnTeal)}}, false},
	}
	paint := func(w want) render.Paint {
		stops := make([]render.GradientStop, len(w.stops))
		for i, s := range w.stops {
			stops[len(stops)-1-i] = render.GradientStop{Offset: float64(s.Offset), // reversed: the canvas sorts
				Color: driver.RGBA(s.Color.R, s.Color.G, s.Color.B, s.Color.A)}
		}
		return render.LinearGradient(w.from, w.to, stops...)
	}
	rect := func(r gtk.GrapheneRect) render.Rect {
		return render.Rect{X: float64(r.Origin.X), Y: float64(r.Origin.Y), W: float64(r.Size.Width), H: float64(r.Size.Height)}
	}
	cnPaint(t, "gradient stroke", cnSize, 1, func(c *canvas) {
		c.PushOpacity(0.5)
		c.StrokeRoundedRect(rect(ws[0].rect), render.UniformRadii(10), float64(ws[0].width), paint(ws[0]))
		c.Pop()
		c.StrokeRoundedRect(rect(ws[1].rect), render.Radii{}, float64(ws[1].width), paint(ws[1]))
	}, func(root *cnNode, ferr error) error {
		strokes := root.find(func(k *cnNode) bool { return k.typ == gtk.NodeStroke })
		if len(strokes) != len(ws) {
			return fmt.Errorf("%d stroke nodes, want %d", len(strokes), len(ws))
		}
		for _, w := range ws {
			var sn *cnNode
			for _, k := range strokes {
				if st := cnStrokeOf(k); st.ok && cnRectNear(st.path, w.rect) {
					sn = k
				}
			}
			if sn == nil {
				return fmt.Errorf("%s: no stroke node whose path bounds are %s", w.name, cnRectString(w.rect))
			}
			if st := cnStrokeOf(sn); !cnNear(st.width, w.width) {
				return fmt.Errorf("%s: the stroke's line width is %g, want %g", w.name, st.width, w.width)
			}
			g := sn.global
			o := w.outer
			if g.Size.Width <= 0 || g.Size.Height <= 0 ||
				g.Origin.X < o.Origin.X-cnEps || g.Origin.Y < o.Origin.Y-cnEps ||
				g.Origin.X+g.Size.Width > o.Origin.X+o.Size.Width+cnEps ||
				g.Origin.Y+g.Size.Height > o.Origin.Y+o.Size.Height+cnEps {
				return fmt.Errorf("%s: the stroke node's bounds %s are not within the outer bounds %s", w.name, cnRectString(g), cnRectString(o))
			}
			gn, err := cnOne(w.name+": linear-gradient nodes in the stroke", sn.find(func(k *cnNode) bool { return k.typ == gtk.NodeLinearGradient }))
			if err != nil {
				return err
			}
			if !cnRectNear(gn.global, w.outer) {
				return fmt.Errorf("%s: the gradient's bounds are %s, want %s", w.name, cnRectString(gn.global), cnRectString(w.outer))
			}
			gr := cnGradientOf(gn)
			if !cnNear(gr.start.X, float32(w.from.X)) || !cnNear(gr.start.Y, float32(w.from.Y)) ||
				!cnNear(gr.end.X, float32(w.to.X)) || !cnNear(gr.end.Y, float32(w.to.Y)) {
				return fmt.Errorf("%s: the gradient runs (%g,%g)→(%g,%g), want (%g,%g)→(%g,%g)", w.name,
					gr.start.X, gr.start.Y, gr.end.X, gr.end.Y, w.from.X, w.from.Y, w.to.X, w.to.Y)
			}
			if len(gr.stops) != len(w.stops) {
				return fmt.Errorf("%s: the gradient has %d stops, want %d", w.name, len(gr.stops), len(w.stops))
			}
			for i, s := range gr.stops {
				if !cnNear(s.Offset, w.stops[i].Offset) || !cnRGBANear(s.Color, w.stops[i].Color) {
					return fmt.Errorf("%s: stop %d is %g %s, want %g %s", w.name, i, s.Offset, cnRGBAString(s.Color),
						w.stops[i].Offset, cnRGBAString(w.stops[i].Color))
				}
			}
			if op := sn.ancestor(gtk.NodeOpacity); (op != nil) != w.layer {
				return fmt.Errorf("%s: under an opacity node = %v, want %v", w.name, op != nil, w.layer)
			}
		}
		if got := root.find(func(k *cnNode) bool { return k.typ == gtk.NodeBorder }); len(got) != 0 {
			return fmt.Errorf("%d border nodes, want none (a gradient stroke fell back to a border)", len(got))
		}
		return cnNoErr(ferr)
	})
}

// cnPoly is the polyline through pts (x, y pairs), closed if closed.
func cnPoly(closed bool, pts ...float64) *render.Path {
	p := new(render.Path)
	for i := 0; i+1 < len(pts); i += 2 {
		q := render.Point{X: pts[i], Y: pts[i+1]}
		if i == 0 {
			p.MoveTo(q)
		} else {
			p.LineTo(q)
		}
	}
	if closed {
		p.Close()
	}
	return p
}

// cnRectF is r as a graphene_rect_t.
func cnRectF(r render.Rect) gtk.GrapheneRect {
	return cnRect(float32(r.X), float32(r.Y), float32(r.W), float32(r.H))
}

// cnWithin reports whether a lies within b (to cnEps).
func cnWithin(a, b gtk.GrapheneRect) bool {
	return a.Origin.X >= b.Origin.X-cnEps && a.Origin.Y >= b.Origin.Y-cnEps &&
		a.Origin.X+a.Size.Width <= b.Origin.X+b.Size.Width+cnEps &&
		a.Origin.Y+a.Size.Height <= b.Origin.Y+b.Size.Height+cnEps
}

// cnStops returns the gradient's stops reversed (the canvas sorts them),
// and the sorted ones as GTK should hold them.
func cnStops(sorted ...render.GradientStop) ([]render.GradientStop, []gtk.ColorStop) {
	rev := make([]render.GradientStop, len(sorted))
	want := make([]gtk.ColorStop, len(sorted))
	for i, s := range sorted {
		rev[len(sorted)-1-i] = s
		want[i] = gtk.ColorStop{Offset: float32(s.Offset), Color: rgba(s.Color)}
	}
	return rev, want
}

// cnCheckGradient checks the single linear-gradient node under n: its
// bounds (in the root's coordinates), points and stops.
func cnCheckGradient(name string, n *cnNode, bounds gtk.GrapheneRect, from, to render.Point, stops []gtk.ColorStop) error {
	gn, err := cnOne(name+": linear-gradient nodes", n.find(func(k *cnNode) bool { return k.typ == gtk.NodeLinearGradient }))
	if err != nil {
		return err
	}
	if !cnRectNear(gn.global, bounds) {
		return fmt.Errorf("%s: the gradient's bounds are %s, want %s", name, cnRectString(gn.global), cnRectString(bounds))
	}
	g := cnGradientOf(gn)
	if !cnNear(g.start.X, float32(from.X)) || !cnNear(g.start.Y, float32(from.Y)) ||
		!cnNear(g.end.X, float32(to.X)) || !cnNear(g.end.Y, float32(to.Y)) {
		return fmt.Errorf("%s: the gradient runs (%g,%g)→(%g,%g), want (%g,%g)→(%g,%g)", name,
			g.start.X, g.start.Y, g.end.X, g.end.Y, from.X, from.Y, to.X, to.Y)
	}
	if len(g.stops) != len(stops) {
		return fmt.Errorf("%s: the gradient has %d stops, want %d", name, len(g.stops), len(stops))
	}
	for i, s := range g.stops {
		if !cnNear(s.Offset, stops[i].Offset) || !cnRGBANear(s.Color, stops[i].Color) {
			return fmt.Errorf("%s: stop %d is %g %s, want %g %s", name, i, s.Offset, cnRGBAString(s.Color),
				stops[i].Offset, cnRGBAString(stops[i].Color))
		}
	}
	return nil
}

// cnNoBorders fails if the tree has a border node (the path placeholder
// outline of S4.1g is gone).
func cnNoBorders(root *cnNode) error {
	if got := root.find(func(k *cnNode) bool { return k.typ == gtk.NodeBorder }); len(got) != 0 {
		return fmt.Errorf("%d border nodes, want none (a path drew as a border)", len(got))
	}
	return nil
}

// TestCanvasGTKFillPath (S4.2c): every FillPath is one fill node with the
// winding rule whose path bounds are the render.Path's bounds; an open
// path is filled closed (if gsk_path_is_closed bound: the fill's path is
// a closed contour); a solid fill holds a color node in the paint's color,
// a gradient fill a linear-gradient node over the path's bounds (grown by
// one device pixel) with the gradient's points and sorted stops; the fill
// inside PushOpacity is under an opacity node, the others are not; a
// curved path's GSK bounds lie between its tight bounds and its
// control-point hull (logged); no border node is drawn.
func TestCanvasGTKFillPath(t *testing.T) {
	curve := new(render.Path).MoveTo(render.Point{X: 20, Y: 60}).
		CubicTo(render.Point{X: 40, Y: 100}, render.Point{X: 80, Y: 100}, render.Point{X: 100, Y: 60}).Close()
	from, to := render.Point{X: 125, Y: 10}, render.Point{X: 190, Y: 60}
	rev, sorted := cnStops(render.GradientStop{Offset: 0, Color: cnOrange},
		render.GradientStop{Offset: 0.4, Color: cnTeal}, render.GradientStop{Offset: 1, Color: cnRed})
	type want struct {
		name   string
		path   *render.Path
		color  render.Color // solid fills
		grad   bool
		layer  bool // under the opacity node
		curved bool // GSK bounds may be loose: checked against the hull
		hull   gtk.GrapheneRect
	}
	ws := []want{
		{name: "closed triangle", path: cnPoly(true, 10, 10, 60, 10, 35, 50), color: cnRed},
		{name: "open path", path: cnPoly(false, 70, 10, 120, 20, 90, 50), color: cnGreen},
		{name: "gradient pentagon, in an opacity", path: cnPoly(true, 130, 10, 180, 10, 190, 40, 155, 60, 125, 40),
			grad: true, layer: true},
		{name: "cubic", path: curve, color: cnBlue, curved: true, hull: cnRect(20, 60, 80, 40)},
	}
	cnPaint(t, "fill path", cnSize, 1, func(c *canvas) {
		for _, w := range ws {
			if w.layer {
				c.PushOpacity(0.5)
			}
			if w.grad {
				c.FillPath(w.path, render.LinearGradient(from, to, rev...))
			} else {
				c.FillPath(w.path, render.Solid(w.color))
			}
			if w.layer {
				c.Pop()
			}
		}
	}, func(root *cnNode, ferr error) error {
		fills := root.find(func(k *cnNode) bool { return k.typ == gtk.NodeFill })
		if len(fills) != len(ws) {
			return fmt.Errorf("%d fill nodes, want %d", len(fills), len(ws))
		}
		for _, w := range ws {
			tight := cnRectF(w.path.Bounds())
			var fn *cnNode
			for _, k := range fills {
				f := cnFillOf(k)
				if !f.ok {
					continue
				}
				if (!w.curved && cnRectNear(f.path, tight)) || (w.curved && cnWithin(tight, f.path) && cnWithin(f.path, w.hull)) {
					fn = k
				}
			}
			if fn == nil {
				return fmt.Errorf("%s: no fill node whose path bounds match %s", w.name, cnRectString(tight))
			}
			f := cnFillOf(fn)
			if w.curved {
				t.Logf("CANVAS-RESULT: gsk_path_get_bounds of a cubic: %s (tight %s, control-point hull %s)",
					cnRectString(f.path), cnRectString(tight), cnRectString(w.hull))
			}
			if f.rule != gtk.FillRuleWinding {
				return fmt.Errorf("%s: the fill rule is %d, want %d (winding)", w.name, f.rule, gtk.FillRuleWinding)
			}
			if f.known && !f.closed {
				return fmt.Errorf("%s: the fill's path is not a closed contour (gsk_path_is_closed)", w.name)
			}
			if w.grad {
				if err := cnCheckGradient(w.name, fn, cnGrow(tight, 1), from, to, sorted); err != nil {
					return err
				}
			} else if _, err := cnOne(w.name+": color nodes of the paint in the fill", fn.colorNodes(w.color)); err != nil {
				return err
			}
			if op := fn.ancestor(gtk.NodeOpacity); (op != nil) != w.layer {
				return fmt.Errorf("%s: under an opacity node = %v, want %v", w.name, op != nil, w.layer)
			}
		}
		if cnGsk.PathIsClosed == nil {
			t.Log("CANVAS-RESULT: gsk_path_is_closed did not bind; the open path's closing is not checked")
		}
		if err := cnNoBorders(root); err != nil {
			return err
		}
		return cnNoErr(ferr)
	})
}

// TestCanvasGTKStrokePath (S4.2c): every StrokePath is one stroke node
// whose path bounds are the render.Path's and whose GskStroke has the
// width, cap, join, miter limit (the default 10 when unset), dash pattern
// (an odd-length one doubled by StrokeStyle.Normalized) and dash offset;
// a solid stroke holds a color node in the paint's color, a gradient
// stroke a linear-gradient node over the path's bounds grown by
// strokeReach and one device pixel, with the gradient's points and sorted
// stops; an open path stays open (if gsk_path_is_closed bound); no border
// node is drawn.
func TestCanvasGTKStrokePath(t *testing.T) {
	from, to := render.Point{X: 120, Y: 50}, render.Point{X: 180, Y: 50}
	rev, sorted := cnStops(render.GradientStop{Offset: 0, Color: cnRed},
		render.GradientStop{Offset: 0.5, Color: cnGreen}, render.GradientStop{Offset: 1, Color: cnBlue})
	type want struct {
		name   string
		path   *render.Path
		width  float64
		style  render.StrokeStyle
		color  render.Color // solid strokes
		grad   bool
		cap    int32
		join   int32
		miter  float32
		dash   []float32
		offset float32
	}
	ws := []want{
		{name: "butt/miter, default limit", path: cnPoly(false, 10, 10, 30, 40, 50, 10), width: 3, color: cnRed,
			cap: gtk.LineCapButt, join: gtk.LineJoinMiter, miter: render.DefaultMiterLimit},
		{name: "round/round", path: cnPoly(false, 60, 10, 80, 40, 100, 10), width: 4, color: cnGreen,
			style: render.StrokeStyle{Cap: render.CapRound, Join: render.JoinRound},
			cap:   gtk.LineCapRound, join: gtk.LineJoinRound, miter: render.DefaultMiterLimit},
		{name: "square/bevel, limit 3", path: cnPoly(false, 110, 10, 130, 40, 150, 10), width: 2, color: cnBlue,
			style: render.StrokeStyle{Cap: render.CapSquare, Join: render.JoinBevel, MiterLimit: 3},
			cap:   gtk.LineCapSquare, join: gtk.LineJoinBevel, miter: 3},
		{name: "dashed, offset", path: cnPoly(false, 10, 55, 100, 70), width: 2, color: cnOrange,
			style: render.StrokeStyle{Dash: []float64{6, 3}, DashOffset: 2},
			cap:   gtk.LineCapButt, join: gtk.LineJoinMiter, miter: render.DefaultMiterLimit, dash: []float32{6, 3}, offset: 2},
		{name: "odd dash, doubled", path: cnPoly(false, 10, 80, 100, 95), width: 1.5, color: cnTeal,
			style: render.StrokeStyle{Cap: render.CapRound, Dash: []float64{5, 2, 1}, DashOffset: 1.5},
			cap:   gtk.LineCapRound, join: gtk.LineJoinMiter, miter: render.DefaultMiterLimit,
			dash: []float32{5, 2, 1, 5, 2, 1}, offset: 1.5},
		{name: "gradient, square/miter limit 2", path: cnPoly(false, 120, 50, 150, 90, 180, 50), width: 4, grad: true,
			style: render.StrokeStyle{Cap: render.CapSquare, Join: render.JoinMiter, MiterLimit: 2},
			cap:   gtk.LineCapSquare, join: gtk.LineJoinMiter, miter: 2},
	}
	cnPaint(t, "stroke path", cnSize, 1, func(c *canvas) {
		for _, w := range ws {
			p := render.Solid(w.color)
			if w.grad {
				p = render.LinearGradient(from, to, rev...)
			}
			c.StrokePath(w.path, w.width, p, w.style)
		}
	}, func(root *cnNode, ferr error) error {
		strokes := root.find(func(k *cnNode) bool { return k.typ == gtk.NodeStroke })
		if len(strokes) != len(ws) {
			return fmt.Errorf("%d stroke nodes, want %d", len(strokes), len(ws))
		}
		for _, w := range ws {
			b := w.path.Bounds()
			pb := cnRectF(b)
			var sn *cnNode
			for _, k := range strokes {
				if st := cnStrokeOf(k); st.ok && cnRectNear(st.path, pb) {
					sn = k
				}
			}
			if sn == nil {
				return fmt.Errorf("%s: no stroke node whose path bounds are %s", w.name, cnRectString(pb))
			}
			st := cnStrokeOf(sn)
			if !cnNear(st.width, float32(w.width)) || st.cap != w.cap || st.join != w.join || !cnNear(st.miter, w.miter) {
				return fmt.Errorf("%s: the stroke is width %g cap %d join %d miter %g, want %g %d %d %g", w.name,
					st.width, st.cap, st.join, st.miter, w.width, w.cap, w.join, w.miter)
			}
			if len(st.dash) != len(w.dash) {
				return fmt.Errorf("%s: the dash pattern is %v, want %v", w.name, st.dash, w.dash)
			}
			for i := range st.dash {
				if !cnNear(st.dash[i], w.dash[i]) {
					return fmt.Errorf("%s: the dash pattern is %v, want %v", w.name, st.dash, w.dash)
				}
			}
			if w.dash != nil && !cnNear(st.offset, w.offset) {
				return fmt.Errorf("%s: the dash offset is %g, want %g", w.name, st.offset, w.offset)
			}
			if cnGsk.PathIsClosed != nil && cnGsk.PathIsClosed(cnGsk.StrokeNodeGetPath(sn.ptr)) != 0 {
				return fmt.Errorf("%s: the open path was stroked closed (gsk_path_is_closed)", w.name)
			}
			if w.grad {
				sa, _ := strokeArgs(render.StrokeWidth(w.width), w.style.Normalized())
				h := strokeReach(sa) + 1 // and one device pixel (gradientCover)
				outer := cnRectF(render.Rect{X: b.X - h, Y: b.Y - h, W: b.W + 2*h, H: b.H + 2*h})
				if err := cnCheckGradient(w.name, sn, outer, from, to, sorted); err != nil {
					return err
				}
				if g := sn.global; g.Size.Width <= 0 || g.Size.Height <= 0 || !cnWithin(g, outer) {
					return fmt.Errorf("%s: the stroke node's bounds %s are not within the gradient's %s", w.name, cnRectString(g), cnRectString(outer))
				}
			} else if _, err := cnOne(w.name+": color nodes of the paint in the stroke", sn.colorNodes(w.color)); err != nil {
				return err
			}
		}
		if err := cnNoBorders(root); err != nil {
			return err
		}
		return cnNoErr(ferr)
	})
}

// TestCanvasGTKShadow (S4.2d): every visible DrawShadow is one outset or
// inset shadow node whose outline is the rect with its radii clamped to
// half the shorter side, in the root's coordinates (GTK may fold a
// translation into the outline or keep a transform node: toRoot), and
// whose color, offset, spread and blur radius are the render.Shadow's
// (the blur unchanged: GSK's blur radius is the CSS one). An inset
// shadow's bounds are its outline's; an outset one's contain the outline
// and the offset, spread shape. A shadow inside PushOpacity(0.5) is under
// an opacity node of 0.5, or GTK folded the opacity into its color
// (logged). An inset shadow whose hole the spread shrinks away is still
// drawn; a transparent shadow, one of a shape with no area and an outset
// one whose shape a negative spread shrinks away draw nothing. No
// placeholder border is drawn.
func TestCanvasGTKShadow(t *testing.T) {
	gray := driver.RGBA(0.3, 0.3, 0.3, 0.5)
	purple := driver.RGBA(0.5, 0.2, 0.7, 0.6)
	type want struct {
		name                 string
		inset                bool
		color                render.Color
		outline              gtk.RoundedRect // in the root's coordinates
		dx, dy, spread, blur float32
		opacity              bool // drawn inside PushOpacity(0.5)
	}
	ws := []want{
		{name: "outset, rounded", color: cnRed, outline: cnRounded(cnRect(20, 20, 60, 40), 8, 4, 20, 0),
			dx: 3, dy: 5, spread: 2, blur: 6},
		{name: "inset", inset: true, color: cnGreen, outline: cnRounded(cnRect(100, 20, 60, 40), 10, 10, 10, 10),
			dx: -2, dy: 4, spread: 3, blur: 5},
		{name: "outset, no blur", color: cnBlue, outline: cnRounded(cnRect(180, 20, 40, 30), 0, 0, 0, 0),
			dx: 4, dy: 4},
		{name: "outset, negative spread", color: cnOrange, outline: cnRounded(cnRect(240, 20, 40, 30), 6, 6, 6, 6),
			dy: 6, spread: -4, blur: 8},
		{name: "inset, hole shrunk away", inset: true, color: gray, outline: cnRounded(cnRect(20, 90, 20, 20), 0, 0, 0, 0),
			dx: 1, dy: 1, spread: 15, blur: 2},
		{name: "outset, translated, in an opacity", color: cnTeal, outline: cnRounded(cnRect(60, 110, 50, 30), 5, 5, 5, 5),
			dx: 2, dy: 2, spread: 1, blur: 4, opacity: true},
		{name: "inset, translated", inset: true, color: purple, outline: cnRounded(cnRect(140, 110, 40, 30), 15, 15, 15, 15),
			dx: -3, dy: -1, spread: 2, blur: 3},
	}
	cnPaint(t, "shadow", render.Size{W: 300, H: 200}, 1, func(c *canvas) {
		sh := func(r render.Rect, radii render.Radii, dx, dy, spread, blur float64, col render.Color, inset bool) {
			c.DrawShadow(r, radii, render.Shadow{Offset: render.Point{X: dx, Y: dy}, Blur: blur, Spread: spread, Color: col, Inset: inset})
		}
		sh(render.Rect{X: 20, Y: 20, W: 60, H: 40}, render.Radii{TopLeft: 8, TopRight: 4, BottomRight: 30}, 3, 5, 2, 6, cnRed, false)
		sh(render.Rect{X: 100, Y: 20, W: 60, H: 40}, render.UniformRadii(10), -2, 4, 3, 5, cnGreen, true)
		sh(render.Rect{X: 180, Y: 20, W: 40, H: 30}, render.Radii{}, 4, 4, 0, 0, cnBlue, false)
		sh(render.Rect{X: 240, Y: 20, W: 40, H: 30}, render.UniformRadii(6), 0, 6, -4, 8, cnOrange, false)
		sh(render.Rect{X: 20, Y: 90, W: 20, H: 20}, render.Radii{}, 1, 1, 15, 2, gray, true)
		c.Save()
		c.Translate(30, 100)
		c.PushOpacity(0.5)
		sh(render.Rect{X: 30, Y: 10, W: 50, H: 30}, render.UniformRadii(5), 2, 2, 1, 4, cnTeal, false)
		c.Pop()
		sh(render.Rect{X: 110, Y: 10, W: 40, H: 30}, render.UniformRadii(40), -3, -1, 2, 3, purple, true)
		c.Restore()
		// Ignored: transparent, no area (outset and inset), the outer shape
		// shrunk away (10 + 2·-5 = 0).
		sh(render.Rect{X: 200, Y: 100, W: 30, H: 30}, render.Radii{}, 2, 2, 1, 4, driver.RGBA(0, 0, 1, 0), false)
		sh(render.Rect{X: 200, Y: 100, W: 0, H: 30}, render.Radii{}, 2, 2, 1, 4, cnRed, false)
		sh(render.Rect{X: 200, Y: 100, W: 30, H: 0}, render.Radii{}, 2, 2, 1, 4, cnGreen, true)
		sh(render.Rect{X: 250, Y: 100, W: 10, H: 30}, render.Radii{}, 2, 2, -5, 4, cnBlue, false)
	}, func(root *cnNode, ferr error) error {
		for _, w := range ws {
			typ := gtk.NodeOutsetShadow
			if w.inset {
				typ = gtk.NodeInsetShadow
			}
			nodes := root.shadowNodes(typ, rgba(w.color))
			folded := false
			if w.opacity && len(nodes) == 0 {
				half := rgba(w.color)
				half.A *= 0.5
				if nodes = root.shadowNodes(typ, half); len(nodes) > 0 {
					folded = true
					t.Logf("CANVAS-RESULT: shadow: GTK folded the opacity 0.5 into the %s shadow's color", w.name)
				}
			}
			n, err := cnOne(w.name+": "+cnTypeNames[typ]+" nodes of its color", nodes)
			if err != nil {
				return err
			}
			sh := cnShadowOf(n)
			tr := n.toRoot()
			if !cnNear(float32(tr.A), 1) || !cnNear(float32(tr.B), 0) || !cnNear(float32(tr.C), 0) || !cnNear(float32(tr.D), 1) {
				return fmt.Errorf("%s: the shadow is under a transform that is not a translation (%+v)", w.name, tr)
			}
			if n.ancestor(gtk.NodeTransform) != nil {
				t.Logf("CANVAS-RESULT: shadow: the %s shadow is under a transform node (GTK did not fold the translation into its outline)", w.name)
			}
			got := sh.outline
			got.Bounds.Origin.X += float32(tr.E)
			got.Bounds.Origin.Y += float32(tr.F)
			if !cnRoundedNear(got, w.outline) {
				return fmt.Errorf("%s: the shadow's outline is %s in the root's coordinates (%s in the node's), want %s",
					w.name, cnRoundedString(got), cnRoundedString(sh.outline), cnRoundedString(w.outline))
			}
			if !cnNear(sh.dx, w.dx) || !cnNear(sh.dy, w.dy) || !cnNear(sh.spread, w.spread) || !cnNear(sh.blur, w.blur) {
				return fmt.Errorf("%s: the shadow has offset (%g,%g) spread %g blur %g, want (%g,%g) %g %g",
					w.name, sh.dx, sh.dy, sh.spread, sh.blur, w.dx, w.dy, w.spread, w.blur)
			}
			ob := w.outline.Bounds
			if w.inset {
				if !cnRectNear(n.global, ob) {
					return fmt.Errorf("%s: the inset shadow node's bounds are %s, want its outline's %s", w.name, cnRectString(n.global), cnRectString(ob))
				}
			} else {
				shape := cnRect(ob.Origin.X+w.dx-w.spread, ob.Origin.Y+w.dy-w.spread, ob.Size.Width+2*w.spread, ob.Size.Height+2*w.spread)
				if !cnWithin(ob, n.global) || !cnWithin(shape, n.global) {
					return fmt.Errorf("%s: the outset shadow node's bounds %s do not contain its outline %s and its shape %s",
						w.name, cnRectString(n.global), cnRectString(ob), cnRectString(shape))
				}
				t.Logf("CANVAS-RESULT: shadow: the %s shadow (outline %s, blur %g) has bounds %s", w.name, cnRectString(ob), w.blur, cnRectString(n.global))
			}
			op := n.ancestor(gtk.NodeOpacity)
			switch {
			case w.opacity && !folded:
				if op == nil {
					return fmt.Errorf("%s: the shadow inside PushOpacity(0.5) is under no opacity node", w.name)
				}
				if got := cnOpacityOf(op); got != 0.5 {
					return fmt.Errorf("%s: the opacity node's opacity is %g, want 0.5", w.name, got)
				}
			case !w.opacity && op != nil:
				return fmt.Errorf("%s: the shadow is under an opacity node of %g", w.name, cnOpacityOf(op))
			}
		}
		if got := root.find(cnIsShadow); len(got) != len(ws) {
			return fmt.Errorf("%d shadow nodes, want %d (an ignored shadow drew)", len(got), len(ws))
		}
		if got := root.find(func(k *cnNode) bool { return k.typ == gtk.NodeBorder }); len(got) != 0 {
			return fmt.Errorf("%d border nodes, want none (a shadow drew as a border)", len(got))
		}
		return cnNoErr(ferr)
	})
}

// --- Text (S4.2i) -----------------------------------------------------------------

// DrawText appends the layout's PangoLayout with gtk_snapshot_append_layout
// inside a save and a translate to the origin. GTK's Pango renderer turns
// every glyph run into a text node (gtk_snapshot_append_text) whose offset
// is the run's baseline origin plus the snapshot's current translation
// (gtk_snapshot_ensure_translate: a translation is folded in, any other
// transform stays a transform node above the text node), whose color is
// the one passed (the layouts carry no color attributes) and whose bounds
// are the glyphs' ink rect, padded by GSK. The tests draw real layouts of
// a text system over the GTK thread's Pango (tlSystem).

// cnTextSystem returns a text system over real Pango for one test
// (tlSystem), its layouter wrapped by wrap unless nil. It is released at
// the end of the test, on the GTK goroutine, and the test fails if it
// logged a failure (a layout fell back to fixed-width metrics, or a
// native layout to draw could not be made).
func cnTextSystem(t *testing.T, wrap func(pangoLayouter) pangoLayouter) *textSystem {
	t.Helper()
	cnNeed(t)
	logs := new([]string)
	ts := tlSystem(logs)
	if wrap != nil {
		open := ts.open
		ts.open = func() (pangoLayouter, error) {
			pl, err := open()
			if err != nil {
				return nil, err
			}
			return wrap(pl), nil
		}
	}
	t.Cleanup(func() {
		onGTK(t, func() error {
			ts.release()
			if len(*logs) > 0 {
				return fmt.Errorf("the text system logged %q", *logs)
			}
			return nil
		})
	})
	return ts
}

// cnIsPango fails unless l is a Pango layout (a fake one draws nothing).
func cnIsPango(what string, l render.TextLayout) error {
	if _, ok := l.(*textLayout); !ok {
		return fmt.Errorf("the %s layout is a %T, not a Pango layout", what, l)
	}
	return nil
}

func cnIsText(n *cnNode) bool { return n.typ == cnNodeText }

// textNodes returns the text nodes under n of color c, in drawing order.
func (n *cnNode) textNodes(c gtk.RGBA) []*cnNode {
	return n.find(func(k *cnNode) bool { return k.typ == cnNodeText && cnRGBANear(cnTextOf(k).color, c) })
}

// cnTextTol is the tolerance of a run's baseline origin, in user points:
// the offset is the layout's own Pango units (1/1024) as a float.
const cnTextTol = 0.01

// cnTextInk is how far, in user points, a text node's bounds may reach
// beyond the layout's box: GSK pads the ink rect by a pixel, and ink may
// overhang the logical box a little (accents, italics, side bearings).
const cnTextInk = 2

// cnCheckText checks that nodes, in drawing order, are the drawing of l at
// origin under the user-to-root transform tr (a scale and a translation):
//   - at least one node, each with a font and glyphs, in all at most one
//     glyph per byte of the text;
//   - the first run's baseline origin (its offset, mapped to the root's
//     coordinates through the transform nodes above it) is at origin +
//     (0, Baseline) in user space, to cnTextTol: the first line starts at
//     x = 0 (left to right, not aligned) and its baseline is the measured
//     one, so the drawn layout is the measured one;
//   - every node's bounds, in the root's coordinates, lie within the
//     layout's box at origin grown by cnTextInk user points.
func cnCheckText(name string, nodes []*cnNode, l render.TextLayout, tr render.Transform, origin render.Point) error {
	if len(nodes) == 0 {
		return fmt.Errorf("%s: no text node of its color", name)
	}
	sz := l.Size()
	s := math.Max(math.Abs(tr.A), math.Abs(tr.D))
	box := cnMapRect(tr, cnRect(float32(origin.X), float32(origin.Y), float32(sz.W), float32(sz.H)))
	slack := cnGrow(box, float32(cnTextInk*s))
	var glyphs uint32
	for i, n := range nodes {
		tx := cnTextOf(n)
		if tx.font == 0 || tx.glyphs == 0 {
			return fmt.Errorf("%s: text node %d has font %#x and %d glyphs, want a font and glyphs", name, i, tx.font, tx.glyphs)
		}
		glyphs += tx.glyphs
		if !cnWithin(n.global, slack) {
			return fmt.Errorf("%s: text node %d has bounds %s in the root's coordinates, beyond the layout's box %s grown by %g",
				name, i, cnRectString(n.global), cnRectString(box), cnTextInk*s)
		}
	}
	if int(glyphs) > len(l.Text()) {
		return fmt.Errorf("%s: %d glyphs for the %d bytes of %q", name, glyphs, len(l.Text()), l.Text())
	}
	tx := cnTextOf(nodes[0])
	gx, gy := cnMapPoint(nodes[0].toRoot(), float64(tx.offset.X), float64(tx.offset.Y))
	wx, wy := cnMapPoint(tr, origin.X, origin.Y+l.Baseline())
	if math.Abs(gx-wx) > cnTextTol*s || math.Abs(gy-wy) > cnTextTol*s {
		return fmt.Errorf("%s: the first run's baseline origin is (%g,%g) in the root's coordinates (offset (%g,%g)), want (%g,%g) (origin %v, baseline %g)",
			name, gx, gy, tx.offset.X, tx.offset.Y, wx, wy, origin, l.Baseline())
	}
	return nil
}

// TestCanvasGTKText: real text is text nodes, checked by cnCheckText, in
// the color drawn (exact): at an origin; under a Translate and a Scale
// (the offset and bounds through the transform nodes above; whether the
// scale stays a transform node is logged); inside a rounded clip that
// cuts it (under a clip or rounded-clip ancestor with the clip's rect,
// which GTK keeps, as the clip does not contain the text); and inside
// PushOpacity(0.5) (under an opacity node of 0.5, or GTK folded the
// opacity into the color: logged). No other text node and no border is
// drawn.
func TestCanvasGTKText(t *testing.T) {
	ts := cnTextSystem(t, nil)
	font := render.Font{Size: 13}
	var plain, scaled, clipped, faded render.TextLayout
	var clip render.Rect
	cnPaint(t, "text", render.Size{W: 300, H: 150}, 1, func(c *canvas) {
		c.text = ts
		plain = ts.NewLayout("Hello", font, 0)
		scaled = ts.NewLayout("Aqua", font, 0)
		clipped = ts.NewLayout("Clipped text", font, 0)
		faded = ts.NewLayout("Faded", font, 0)
		c.DrawText(plain, render.Point{X: 10, Y: 20}, cnRed)
		c.Save()
		c.Translate(100, 10)
		c.Scale(2, 2)
		c.DrawText(scaled, render.Point{X: 5, Y: 5}, cnGreen)
		c.Restore()
		c.Save()
		clip = render.Rect{X: 10, Y: 80, W: math.Round(clipped.Size().W / 2), H: math.Ceil(clipped.Size().H)}
		c.ClipRoundedRect(clip, render.UniformRadii(3))
		c.DrawText(clipped, render.Point{X: 10, Y: 80}, cnBlue)
		c.Restore()
		c.Save()
		c.PushOpacity(0.5)
		c.DrawText(faded, render.Point{X: 200, Y: 80}, cnOrange)
		c.Pop()
		c.Restore()
	}, func(root *cnNode, ferr error) error {
		ws := []struct {
			name   string
			l      render.TextLayout
			color  render.Color
			origin render.Point
			tr     render.Transform // user space to the root's
		}{
			{"plain", plain, cnRed, render.Point{X: 10, Y: 20}, render.Identity()},
			{"translated and scaled", scaled, cnGreen, render.Point{X: 5, Y: 5}, render.Transform{A: 2, D: 2, E: 100, F: 10}},
			{"clipped", clipped, cnBlue, render.Point{X: 10, Y: 80}, render.Identity()},
			{"in an opacity", faded, cnOrange, render.Point{X: 200, Y: 80}, render.Identity()},
		}
		total := 0
		for _, w := range ws {
			if err := cnIsPango(w.name, w.l); err != nil {
				return err
			}
			opacity := w.l == faded
			nodes := root.textNodes(rgba(w.color))
			folded := false
			if opacity && len(nodes) == 0 {
				half := rgba(w.color)
				half.A *= 0.5
				if nodes = root.textNodes(half); len(nodes) > 0 {
					folded = true
					t.Logf("CANVAS-RESULT: text: GTK folded the opacity 0.5 into the text node's color")
				}
			}
			if err := cnCheckText(w.name, nodes, w.l, w.tr, w.origin); err != nil {
				return err
			}
			total += len(nodes)
			tx := cnTextOf(nodes[0])
			t.Logf("CANVAS-RESULT: text: %s %q (size %v, baseline %g): %d text node(s), the first with bounds %s offset (%g,%g) %d glyphs, under a transform node: %v",
				w.name, w.l.Text(), w.l.Size(), w.l.Baseline(), len(nodes), cnRectString(nodes[0].global),
				tx.offset.X, tx.offset.Y, tx.glyphs, nodes[0].ancestor(gtk.NodeTransform) != nil)
			for _, n := range nodes {
				if w.l == clipped && n.clipper(cnRectF(clip)) == nil {
					return fmt.Errorf("%s: a text node is under no clip of %s", w.name, cnRectString(cnRectF(clip)))
				}
				op := n.ancestor(gtk.NodeOpacity)
				switch {
				case opacity && !folded:
					if op == nil {
						return fmt.Errorf("%s: a text node inside PushOpacity(0.5) is under no opacity node", w.name)
					}
					if got := cnOpacityOf(op); got != 0.5 {
						return fmt.Errorf("%s: the opacity node's opacity is %g, want 0.5", w.name, got)
					}
				case !opacity && op != nil:
					return fmt.Errorf("%s: a text node is under an opacity node of %g", w.name, cnOpacityOf(op))
				}
			}
		}
		if got := root.find(cnIsText); len(got) != total {
			return fmt.Errorf("%d text nodes, want the %d of the four layouts", len(got), total)
		}
		if err := cnNoBorders(root); err != nil {
			return err
		}
		return cnNoErr(ferr)
	})
}

// TestCanvasGTKTextIgnored: next to a drawn layout (checked by
// cnCheckText), a fake layout, a layout of another text system, empty
// text, a transparent color and origins that are not finite (NaN, +Inf,
// beyond float32) draw no text node; neither does a semantic color (which
// the recorder reports), a canvas without a text system, or a layout of a
// released text system.
func TestCanvasGTKTextIgnored(t *testing.T) {
	ts := cnTextSystem(t, nil)
	other := cnTextSystem(t, nil)
	released := cnTextSystem(t, nil)
	font := render.Font{Size: 13}
	var drawn, foreign, empty render.TextLayout
	cnPaint(t, "text ignored", cnSize, 1, func(c *canvas) {
		c.text = ts
		drawn = ts.NewLayout("Drawn", font, 0)
		foreign = other.NewLayout("Foreign", font, 0)
		empty = ts.NewLayout("", font, 0)
		c.DrawText(drawn, render.Point{X: 10, Y: 10}, cnRed)
		c.DrawText(fake.NewTextSystem().NewLayout("Fake", font, 0), render.Point{X: 10, Y: 40}, cnGreen)
		c.DrawText(foreign, render.Point{X: 10, Y: 70}, cnBlue)
		c.DrawText(empty, render.Point{X: 100, Y: 10}, cnOrange)
		c.DrawText(drawn, render.Point{X: 100, Y: 40}, driver.RGBA(0.9, 0.1, 0.2, 0))
		c.DrawText(drawn, render.Point{X: math.NaN(), Y: 40}, cnTeal)
		c.DrawText(drawn, render.Point{X: 100, Y: math.Inf(1)}, cnTeal)
		c.DrawText(drawn, render.Point{X: 1e39, Y: 40}, cnTeal)
	}, func(root *cnNode, ferr error) error {
		for _, l := range []struct {
			what string
			l    render.TextLayout
		}{{"drawn", drawn}, {"other system's", foreign}, {"empty", empty}} {
			if err := cnIsPango(l.what, l.l); err != nil {
				return err
			}
		}
		red := root.textNodes(rgba(cnRed))
		if err := cnCheckText("drawn", red, drawn, render.Identity(), render.Point{X: 10, Y: 10}); err != nil {
			return err
		}
		if got := root.find(cnIsText); len(got) != len(red) {
			return fmt.Errorf("%d text nodes, want only the %d of the drawn layout", len(got), len(red))
		}
		if err := cnNoBorders(root); err != nil {
			return err
		}
		return cnNoErr(ferr)
	})
	cnPaint(t, "text semantic", cnSize, 1, func(c *canvas) {
		c.text = ts
		c.DrawText(drawn, render.Point{X: 10, Y: 10}, driver.Semantic(driver.ColorLabel))
	}, func(root *cnNode, ferr error) error {
		if got := root.find(cnIsText); len(got) != 0 {
			return fmt.Errorf("a semantic color drew %d text nodes", len(got))
		}
		if ferr == nil {
			return errors.New("finish returned no recorder error for the semantic color")
		}
		return nil
	})
	cnPaint(t, "text without a text system", cnSize, 1, func(c *canvas) {
		c.DrawText(drawn, render.Point{X: 10, Y: 10}, cnRed)
	}, func(root *cnNode, ferr error) error {
		if got := root.find(cnIsText); len(got) != 0 {
			return fmt.Errorf("a canvas without a text system drew %d text nodes", len(got))
		}
		return cnNoErr(ferr)
	})
	var gone render.TextLayout
	cnPaint(t, "text of a released system", cnSize, 1, func(c *canvas) {
		c.text = released
		gone = released.NewLayout("Released", font, 0)
		released.release()
		c.DrawText(gone, render.Point{X: 10, Y: 10}, cnRed)
	}, func(root *cnNode, ferr error) error {
		if err := cnIsPango("released system's", gone); err != nil {
			return err
		}
		if got := root.find(cnIsText); len(got) != 0 {
			return fmt.Errorf("a layout of a released text system drew %d text nodes", len(got))
		}
		return cnNoErr(ferr)
	})
}

// cnCounts counts the native layouts a cnCountingLayouter made and the
// releases of them.
type cnCounts struct{ made, released int }

// cnCountingLayouter is a real pangoLayouter whose native layouts are
// wrapped in cnCounted and counted.
type cnCountingLayouter struct {
	pangoLayouter
	n *cnCounts
}

func (p cnCountingLayouter) layout(text string, f pangoFont, width int32) (nativeLayout, error) {
	nl, err := p.pangoLayouter.layout(text, f, width)
	if err != nil || nl == nil {
		return nl, err
	}
	p.n.made++
	return &cnCounted{inner: nl, n: p.n}, nil
}

// cnCounted is a counted native layout. Only its first release drops the
// PangoLayout, so a second one is counted, not a double unref.
type cnCounted struct {
	inner    nativeLayout
	n        *cnCounts
	releases int
}

func (l *cnCounted) release() {
	l.releases++
	l.n.released++
	if l.releases == 1 {
		l.inner.release()
	}
}

// cnLayoutSnap is the GTK snapshotter recording the native layouts
// appendLayout is given, and handing a cnCounted's PangoLayout on
// (gtkSnapshot.appendLayout draws only a pangoLayout).
type cnLayoutSnap struct {
	*gtkSnapshot
	drawn []nativeLayout
}

func (s *cnLayoutSnap) appendLayout(l nativeLayout, c gtk.RGBA) {
	s.drawn = append(s.drawn, l)
	if cl, ok := l.(*cnCounted); ok {
		l = cl.inner
	}
	s.gtkSnapshot.appendLayout(l, c)
}

// cnSwapSnap puts a cnLayoutSnap over c's snapshotter (cnPaint's
// *gtkSnapshot, already begun) into c, in a draw function, and returns it.
func cnSwapSnap(c *canvas) *cnLayoutSnap {
	s := &cnLayoutSnap{gtkSnapshot: c.sn.(*gtkSnapshot)}
	c.sn = s
	return s
}

// TestCanvasGTKTextOneLayoutPerPaint: a paint makes one native layout per
// layout drawn, however often it is drawn (a layout drawn twice is text
// nodes at both origins from one PangoLayout), and finish releases each
// exactly once; the next paint makes a new one.
func TestCanvasGTKTextOneLayoutPerPaint(t *testing.T) {
	var n cnCounts
	ts := cnTextSystem(t, func(pl pangoLayouter) pangoLayouter { return cnCountingLayouter{pl, &n} })
	font := render.Font{Size: 13}
	var twice, once render.TextLayout
	var first []nativeLayout
	releasedOnce := func(what string, nls []nativeLayout) error {
		for i, nl := range nls {
			cl, ok := nl.(*cnCounted)
			if !ok {
				return fmt.Errorf("%s: native layout %d is a %T, not a counted one", what, i, nl)
			}
			if cl.releases != 1 {
				return fmt.Errorf("%s: native layout %d was released %d times by finish, want once", what, i, cl.releases)
			}
		}
		return nil
	}
	cnPaint(t, "text, one layout per paint", cnSize, 1, func(c *canvas) {
		snap := cnSwapSnap(c)
		c.text = ts
		twice = ts.NewLayout("Twice", font, 0)
		once = ts.NewLayout("Once", font, 0)
		c.DrawText(twice, render.Point{X: 10, Y: 10}, cnRed)
		c.DrawText(twice, render.Point{X: 10, Y: 50}, cnRed)
		c.DrawText(once, render.Point{X: 100, Y: 10}, cnGreen)
		first = snap.drawn
	}, func(root *cnNode, ferr error) error {
		if err := errors.Join(cnIsPango("twice", twice), cnIsPango("once", once)); err != nil {
			return err
		}
		if n.made != 2 {
			return fmt.Errorf("%d native layouts made for two layouts drawn three times in a paint, want 2", n.made)
		}
		if len(first) != 3 || first[0] != first[1] || first[0] == first[2] {
			return fmt.Errorf("appendLayout got %v, want three: the twice drawn layout's native layout twice, then another", first)
		}
		if n.released != 2 {
			return fmt.Errorf("finish released %d native layouts, want the 2 made", n.released)
		}
		if err := releasedOnce("first paint", []nativeLayout{first[0], first[2]}); err != nil {
			return err
		}
		var upper, lower []*cnNode
		for _, k := range root.textNodes(rgba(cnRed)) {
			if k.global.Origin.Y+k.global.Size.Height/2 < 40 {
				upper = append(upper, k)
			} else {
				lower = append(lower, k)
			}
		}
		if err := errors.Join(
			cnCheckText("drawn twice, first", upper, twice, render.Identity(), render.Point{X: 10, Y: 10}),
			cnCheckText("drawn twice, second", lower, twice, render.Identity(), render.Point{X: 10, Y: 50}),
			cnCheckText("drawn once", root.textNodes(rgba(cnGreen)), once, render.Identity(), render.Point{X: 100, Y: 10}),
		); err != nil {
			return err
		}
		t.Logf("CANVAS-RESULT: text: a layout drawn twice in a paint is %d+%d text nodes from one PangoLayout", len(upper), len(lower))
		if err := cnNoBorders(root); err != nil {
			return err
		}
		return cnNoErr(ferr)
	})
	var second []nativeLayout
	cnPaint(t, "text, the next paint", cnSize, 1, func(c *canvas) {
		snap := cnSwapSnap(c)
		c.text = ts
		c.DrawText(twice, render.Point{X: 10, Y: 10}, cnRed)
		second = snap.drawn
	}, func(root *cnNode, ferr error) error {
		if n.made != 3 || n.released != 3 {
			return fmt.Errorf("after the next paint %d native layouts were made and %d released, want 3 and 3", n.made, n.released)
		}
		if len(second) != 1 || (len(first) > 0 && second[0] == first[0]) {
			return fmt.Errorf("appendLayout got %v in the next paint, want one new native layout (the first paint's were %v)", second, first)
		}
		if err := releasedOnce("next paint", second); err != nil {
			return err
		}
		if err := cnCheckText("next paint", root.textNodes(rgba(cnRed)), twice, render.Identity(), render.Point{X: 10, Y: 10}); err != nil {
			return err
		}
		return cnNoErr(ferr)
	})
}

// --- The backdrop blur (S4.2f) ----------------------------------------------------

// under reports whether a is an ancestor of n.
func (n *cnNode) under(a *cnNode) bool {
	for p := n.parent; p != nil; p = p.parent {
		if p == a {
			return true
		}
	}
	return false
}

// cnDistinct returns nodes without repeats of a GSK node: a node a blur
// appends again is in the tree once per append.
func cnDistinct(nodes []*cnNode) []*cnNode {
	seen := map[uintptr]bool{}
	var out []*cnNode
	for _, n := range nodes {
		if !seen[n.ptr] {
			seen[n.ptr] = true
			out = append(out, n)
		}
	}
	return out
}

// blurNodes returns the blur nodes under n, every append of each.
func (n *cnNode) blurNodes() []*cnNode {
	return n.find(func(k *cnNode) bool { return k.typ == gtk.NodeBlur })
}

// cnBlurGlobal is the radius of the blur node n in the root's
// coordinates: scaled by √|det| of the transform nodes above it.
func cnBlurGlobal(n *cnNode) float32 {
	return float32(float64(cnBlurOf(n)) * math.Sqrt(math.Abs(n.toRoot().Det())))
}

// cnRoundedClipGlobal is the clip of the rounded-clip node n in the root's
// coordinates: its bounds mapped, its radii scaled by the axis scales of
// the transform nodes above it (which the tests keep free of rotation).
func cnRoundedClipGlobal(n *cnNode) gtk.RoundedRect {
	rr, tr := cnRoundedClipOf(n), n.toRoot()
	out := gtk.RoundedRect{Bounds: cnMapRect(tr, rr.Bounds)}
	sx, sy := float32(math.Abs(tr.A)), float32(math.Abs(tr.D))
	for i, c := range rr.Corner {
		out.Corner[i] = gtk.GrapheneSize{Width: c.Width * sx, Height: c.Height * sy}
	}
	return out
}

// cnSplitBlur splits nodes into those under no blur node and those under
// one.
func cnSplitBlur(nodes []*cnNode) (plain, blurred []*cnNode) {
	for _, n := range nodes {
		if n.ancestor(gtk.NodeBlur) == nil {
			plain = append(plain, n)
		} else {
			blurred = append(blurred, n)
		}
	}
	return plain, blurred
}

// cnBlurOne is what one backdrop blur over red content must give: the
// blur's clip in root coordinates (radii exact), its radius in the root,
// and the red fill's root bounds.
type cnBlurOne struct {
	clip   gtk.RoundedRect
	radius float32
	red    gtk.GrapheneRect
}

// check finds the one blur node and checks that it is under a rounded clip
// of w.clip, has w.radius and holds a copy of the red fill at its root
// position, while the original is under no blur and not under the clip;
// the blue fill drawn inside the blur is under the clip but not blurred,
// and the orange one after Pop is under neither.
func (w cnBlurOne) check(t *testing.T, name string, root *cnNode) error {
	bl, err := cnOne("blur nodes", cnDistinct(root.blurNodes()))
	if err != nil {
		return err
	}
	if got := cnBlurGlobal(bl); !cnNear(got, w.radius) {
		return fmt.Errorf("the blur node's radius is %g in the root, want %g", got, w.radius)
	}
	rc := bl.ancestor(gtk.NodeRoundedClip)
	if rc == nil {
		return errors.New("the blur node is under no rounded clip")
	}
	if got := cnRoundedClipGlobal(rc); !cnRoundedNear(got, w.clip) {
		return fmt.Errorf("the blur node's rounded clip is %s in the root, want the blur's %s", cnRoundedString(got), cnRoundedString(w.clip))
	}
	plain, blurred := cnSplitBlur(root.colorNodes(cnRed))
	if len(plain) != 1 || len(blurred) != 1 {
		return fmt.Errorf("%d red color nodes under no blur and %d under one, want one each (the content drawn before the blur and its blurred copy)", len(plain), len(blurred))
	}
	for _, n := range []*cnNode{plain[0], blurred[0]} {
		if !cnRectNear(n.global, w.red) {
			return fmt.Errorf("a red color node is at %s in the root, want %s", cnRectString(n.global), cnRectString(w.red))
		}
	}
	if a := blurred[0].ancestor(gtk.NodeBlur); a.ptr != bl.ptr || !blurred[0].under(rc) {
		return errors.New("the blurred red copy is not under the blur node inside the blur's rounded clip")
	}
	if plain[0].under(rc) {
		return errors.New("the red content drawn before the blur is under the blur's rounded clip")
	}
	blue, err := cnOne("blue color nodes (inside the blur)", root.colorNodes(cnBlue))
	if err != nil {
		return err
	}
	if blue.ancestor(gtk.NodeBlur) != nil {
		return errors.New("the fill inside PushBackdropBlur is under a blur node")
	}
	if !blue.under(rc) {
		return errors.New("the fill inside PushBackdropBlur is not under the blur's rounded clip")
	}
	orange, err := cnOne("orange color nodes (after Pop)", root.colorNodes(cnOrange))
	if err != nil {
		return err
	}
	if a := orange.ancestor(gtk.NodeRoundedClip, gtk.NodeBlur); a != nil {
		return fmt.Errorf("the fill after the blur's Pop is under a %s node", cnTypeNames[a.typ])
	}
	t.Logf("CANVAS-RESULT: %s: the blur node (radius %g, bounds %s) is under the rounded clip %s, the red copy at %s",
		name, cnBlurOf(bl), cnRectString(bl.global), cnRoundedString(cnRoundedClipOf(rc)), cnRectString(blurred[0].global))
	return nil
}

// TestCanvasGTKBackdropBlur: PushBackdropBlur appends again what was drawn
// before it (blur.go). The earlier content is in the tree twice: as drawn,
// under no blur node, and as a copy at the same root position under a
// blur node of the blur's radius in the root (radius·√|det| under a
// transform), which is under a rounded clip of the blur's rounded rect;
// what is drawn inside the blur is under that clip but not blurred, and
// what is drawn after Pop under neither. A blur at the surface's left
// edge adds a copy mirrored there; a blur inside another blurs the first
// blur's node too; a blur inside PushOpacity reopens the opacity group;
// a zero radius only clips. Every paint finishes balanced.
func TestCanvasGTKBackdropBlur(t *testing.T) {
	radii := render.Radii{TopLeft: 2, TopRight: 4, BottomRight: 6, BottomLeft: 8}
	for _, scale := range []float64{1, 2} {
		name := fmt.Sprintf("backdrop blur at scale %g", scale)
		cnPaint(t, name, cnSize, scale, func(c *canvas) {
			c.FillRoundedRect(render.Rect{X: 20, Y: 10, W: 120, H: 80}, render.Radii{}, render.Solid(cnRed))
			c.PushBackdropBlur(render.Rect{X: 60, Y: 30, W: 60, H: 30}, radii, 4)
			c.FillRoundedRect(render.Rect{X: 50, Y: 20, W: 80, H: 50}, render.Radii{}, render.Solid(cnBlue))
			c.Pop()
			c.FillRoundedRect(render.Rect{X: 150, Y: 50, W: 20, H: 20}, render.Radii{}, render.Solid(cnOrange))
		}, func(root *cnNode, ferr error) error {
			w := cnBlurOne{clip: cnRounded(cnRect(60, 30, 60, 30), 2, 4, 6, 8), radius: 4, red: cnRect(20, 10, 120, 80)}
			if err := w.check(t, name, root); err != nil {
				return err
			}
			return cnNoErr(ferr)
		})
	}

	// Translate(10, 5) and Scale(2, 2): the blur's rect (20,10 20×15) is
	// (50,25 40×30) in the root, its radii and its radius doubled.
	cnPaint(t, "backdrop blur under a transform", cnSize, 1, func(c *canvas) {
		c.Translate(10, 5)
		c.Scale(2, 2)
		c.FillRoundedRect(render.Rect{X: 0, Y: 0, W: 30, H: 20}, render.Radii{}, render.Solid(cnRed))
		c.PushBackdropBlur(render.Rect{X: 20, Y: 10, W: 20, H: 15}, render.UniformRadii(3), 4)
		c.FillRoundedRect(render.Rect{X: 15, Y: 5, W: 30, H: 25}, render.Radii{}, render.Solid(cnBlue))
		c.Pop()
		c.FillRoundedRect(render.Rect{X: 70, Y: 22, W: 10, H: 10}, render.Radii{}, render.Solid(cnOrange))
	}, func(root *cnNode, ferr error) error {
		w := cnBlurOne{clip: cnRounded(cnRect(50, 25, 40, 30), 6, 6, 6, 6), radius: 8, red: cnRect(10, 5, 60, 40)}
		if err := w.check(t, "backdrop blur under a transform", root); err != nil {
			return err
		}
		return cnNoErr(ferr)
	})

	// At the left edge the cover (the shape padded by 3σ = 6) leaves the
	// surface, so the content is appended again mirrored at x = 0: the red
	// fill (0,0 100×100) once more at (-100,0 100×100).
	cnPaint(t, "backdrop blur at the edge", cnSize, 1, func(c *canvas) {
		c.FillRoundedRect(render.Rect{X: 0, Y: 0, W: 100, H: 100}, render.Radii{}, render.Solid(cnRed))
		c.PushBackdropBlur(render.Rect{X: 0, Y: 30, W: 40, H: 40}, render.UniformRadii(6), 4)
		c.Pop()
	}, func(root *cnNode, ferr error) error {
		if _, err := cnOne("blur nodes", cnDistinct(root.blurNodes())); err != nil {
			return err
		}
		plain, blurred := cnSplitBlur(root.colorNodes(cnRed))
		if len(plain) != 1 || len(blurred) != 2 {
			return fmt.Errorf("%d red color nodes under no blur and %d under one, want one and two (the copy and its mirror at the left edge)", len(plain), len(blurred))
		}
		var mirrored bool
		for _, n := range blurred {
			switch {
			case cnRectNear(n.global, cnRect(0, 0, 100, 100)):
			case cnRectNear(n.global, cnRect(-100, 0, 100, 100)):
				mirrored = true
			default:
				return fmt.Errorf("a blurred red copy is at %s in the root, want (0,0 100×100) or its mirror (-100,0 100×100)", cnRectString(n.global))
			}
		}
		if !mirrored {
			return errors.New("no blurred red copy is mirrored at the left edge (-100,0 100×100)")
		}
		t.Logf("CANVAS-RESULT: backdrop blur at the edge: %d red color nodes, %d of them blurred", len(plain)+len(blurred), len(blurred))
		return cnNoErr(ferr)
	})

	// A blur inside a blur: the inner one's backdrop is everything so far,
	// the outer blur's node included.
	cnPaint(t, "nested backdrop blur", cnSize, 1, func(c *canvas) {
		c.FillRoundedRect(render.Rect{X: 10, Y: 10, W: 180, H: 80}, render.Radii{}, render.Solid(cnRed))
		c.PushBackdropBlur(render.Rect{X: 40, Y: 20, W: 120, H: 60}, render.UniformRadii(8), 4)
		c.FillRoundedRect(render.Rect{X: 30, Y: 15, W: 140, H: 70}, render.Radii{}, render.Solid(cnBlue))
		c.PushBackdropBlur(render.Rect{X: 70, Y: 35, W: 60, H: 30}, render.UniformRadii(4), 6)
		c.FillRoundedRect(render.Rect{X: 60, Y: 30, W: 80, H: 40}, render.Radii{}, render.Solid(cnGreen))
		c.Pop()
		c.Pop()
		c.FillRoundedRect(render.Rect{X: 180, Y: 85, W: 10, H: 10}, render.Radii{}, render.Solid(cnOrange))
	}, func(root *cnNode, ferr error) error {
		blurs := cnDistinct(root.blurNodes())
		if len(blurs) != 2 {
			return fmt.Errorf("%d blur nodes, want two", len(blurs))
		}
		var outer, inner *cnNode
		for _, b := range blurs {
			switch r := cnBlurOf(b); {
			case cnNear(r, 4):
				outer = b
			case cnNear(r, 6):
				inner = b
			default:
				return fmt.Errorf("a blur node's radius is %g, want 4 or 6", r)
			}
		}
		if outer == nil || inner == nil {
			return errors.New("the blur nodes are not one of radius 4 and one of radius 6")
		}
		if n := len(root.find(func(k *cnNode) bool { return k.ptr == inner.ptr })); n != 1 || inner.ancestor(gtk.NodeBlur) != nil {
			return fmt.Errorf("the inner blur node is in the tree %d times or under a blur, want once under none", n)
		}
		if len(inner.colorNodes(cnRed)) == 0 || len(inner.colorNodes(cnBlue)) == 0 {
			return errors.New("the inner blur does not hold the red and blue content drawn before it")
		}
		if len(inner.find(func(k *cnNode) bool { return k.ptr == outer.ptr })) == 0 {
			return errors.New("the inner blur does not hold the outer blur's node")
		}
		rc := inner.ancestor(gtk.NodeRoundedClip)
		want := cnRounded(cnRect(70, 35, 60, 30), 4, 4, 4, 4)
		if rc == nil || !cnRoundedNear(cnRoundedClipOf(rc), want) {
			return fmt.Errorf("the inner blur node is not under a rounded clip of %s", cnRoundedString(want))
		}
		green, err := cnOne("green color nodes (inside the inner blur)", root.colorNodes(cnGreen))
		if err != nil {
			return err
		}
		if green.ancestor(gtk.NodeBlur) != nil || !green.under(rc) {
			return errors.New("the fill inside the inner blur is under a blur node or not under the inner blur's rounded clip")
		}
		orange, err := cnOne("orange color nodes (after both Pops)", root.colorNodes(cnOrange))
		if err != nil {
			return err
		}
		if a := orange.ancestor(gtk.NodeRoundedClip, gtk.NodeBlur); a != nil {
			return fmt.Errorf("the fill after both Pops is under a %s node", cnTypeNames[a.typ])
		}
		t.Logf("CANVAS-RESULT: nested backdrop blur: %d blur nodes in the tree, %d distinct", len(root.blurNodes()), len(blurs))
		return cnNoErr(ferr)
	})

	// A blur inside PushOpacity(0.5): the group is closed at the blur and
	// opened again, so its content before the blur is an opacity node of
	// its own (two overlapping fills, which GTK can't fold into a color),
	// and the blur and what follows it are in a second one.
	cnPaint(t, "backdrop blur in an opacity group", cnSize, 1, func(c *canvas) {
		c.PushOpacity(0.5)
		c.FillRoundedRect(render.Rect{X: 20, Y: 10, W: 80, H: 60}, render.Radii{}, render.Solid(cnRed))
		c.FillRoundedRect(render.Rect{X: 60, Y: 30, W: 80, H: 60}, render.Radii{}, render.Solid(cnGreen))
		c.PushBackdropBlur(render.Rect{X: 50, Y: 30, W: 80, H: 40}, render.UniformRadii(6), 4)
		c.FillRoundedRect(render.Rect{X: 40, Y: 20, W: 100, H: 60}, render.Radii{}, render.Solid(cnBlue))
		c.Pop()
		c.Pop()
		c.FillRoundedRect(render.Rect{X: 170, Y: 10, W: 20, H: 20}, render.Radii{}, render.Solid(cnOrange))
	}, func(root *cnNode, ferr error) error {
		ops := cnDistinct(root.find(func(k *cnNode) bool { return k.typ == gtk.NodeOpacity }))
		if len(ops) != 2 {
			return fmt.Errorf("%d opacity nodes, want two (the group before the blur and the one reopened)", len(ops))
		}
		for _, op := range ops {
			if got := cnOpacityOf(op); got != 0.5 {
				return fmt.Errorf("an opacity node's opacity is %g, want 0.5", got)
			}
		}
		bl, err := cnOne("blur nodes", cnDistinct(root.blurNodes()))
		if err != nil {
			return err
		}
		reopened := bl.ancestor(gtk.NodeOpacity)
		if reopened == nil {
			return errors.New("the blur node is under no opacity node")
		}
		blue, err := cnOne("blue color nodes (inside the blur)", root.colorNodes(cnBlue))
		if err != nil {
			return err
		}
		if a := blue.ancestor(gtk.NodeOpacity); a == nil || a.ptr != reopened.ptr || blue.ancestor(gtk.NodeBlur) != nil {
			return errors.New("the fill inside the blur is not in the reopened opacity group, or is blurred")
		}
		plain, blurred := cnSplitBlur(root.colorNodes(cnRed))
		if len(plain) != 1 || len(blurred) != 1 {
			return fmt.Errorf("%d red color nodes under no blur and %d under one, want one each", len(plain), len(blurred))
		}
		first := plain[0].ancestor(gtk.NodeOpacity)
		if first == nil || first.ptr == reopened.ptr {
			return errors.New("the red fill before the blur is not in an opacity group of its own")
		}
		if a := blurred[0].ancestor(gtk.NodeOpacity); a == nil || a.ptr != first.ptr {
			return errors.New("the blurred red copy is not in a copy of the first opacity group")
		}
		orange, err := cnOne("orange color nodes (after both Pops)", root.colorNodes(cnOrange))
		if err != nil {
			return err
		}
		if a := orange.ancestor(gtk.NodeOpacity, gtk.NodeBlur); a != nil {
			return fmt.Errorf("the fill after both Pops is under a %s node", cnTypeNames[a.typ])
		}
		return cnNoErr(ferr)
	})

	// A zero radius only clips: no blur node, the content once.
	cnPaint(t, "backdrop blur of radius 0", cnSize, 1, func(c *canvas) {
		c.FillRoundedRect(render.Rect{X: 20, Y: 10, W: 120, H: 80}, render.Radii{}, render.Solid(cnRed))
		c.PushBackdropBlur(render.Rect{X: 60, Y: 30, W: 60, H: 30}, render.UniformRadii(6), 0)
		c.FillRoundedRect(render.Rect{X: 50, Y: 20, W: 80, H: 50}, render.Radii{}, render.Solid(cnBlue))
		c.Pop()
	}, func(root *cnNode, ferr error) error {
		if got := root.blurNodes(); len(got) != 0 {
			return fmt.Errorf("%d blur nodes, want none for a zero radius", len(got))
		}
		if _, err := cnOne("red color nodes", root.colorNodes(cnRed)); err != nil {
			return err
		}
		blue, err := cnOne("blue color nodes (inside the blur)", root.colorNodes(cnBlue))
		if err != nil {
			return err
		}
		rc := blue.ancestor(gtk.NodeRoundedClip)
		if want := cnRounded(cnRect(60, 30, 60, 30), 6, 6, 6, 6); rc == nil || !cnRoundedNear(cnRoundedClipOf(rc), want) {
			return fmt.Errorf("the fill inside the zero-radius blur is not under a rounded clip of %s", cnRoundedString(want))
		}
		return cnNoErr(ferr)
	})
}

// TestCanvasGTKRestoreUnclips: a clip set after Save ends at Restore: a
// fill after it is not under the clip. A sharp clip is logged as a clip or
// a rounded-clip node. Unmatched Restore and Pop calls reach no GTK call
// (they are counted by the recorder only), so the tree stays intact.
func TestCanvasGTKRestoreUnclips(t *testing.T) {
	cnPaint(t, "restore unclips", cnSize, 1, func(c *canvas) {
		c.Restore() // unmatched: ignored
		c.Pop()     // unmatched: ignored
		c.FillRoundedRect(render.Rect{X: 150, Y: 70, W: 10, H: 10}, render.Radii{}, render.Solid(cnTeal))
		c.Save()
		c.ClipRoundedRect(render.Rect{X: 10, Y: 10, W: 50, H: 50}, render.UniformRadii(5))
		c.FillRoundedRect(render.Rect{X: 0, Y: 0, W: 100, H: 100}, render.Radii{}, render.Solid(cnRed))
		c.Restore()
		c.FillRoundedRect(render.Rect{X: 0, Y: 0, W: 100, H: 90}, render.Radii{}, render.Solid(cnGreen))
		c.Save()
		c.ClipRoundedRect(render.Rect{X: 120, Y: 10, W: 30, H: 30}, render.Radii{})
		c.FillRoundedRect(render.Rect{X: 110, Y: 0, W: 80, H: 80}, render.Radii{}, render.Solid(cnBlue))
		c.Restore()
		c.FillRoundedRect(render.Rect{X: 110, Y: 0, W: 80, H: 80}, render.Radii{}, render.Solid(cnOrange))
	}, func(root *cnNode, ferr error) error {
		if _, err := cnOne("teal color nodes (drawn first)", root.colorNodes(cnTeal)); err != nil {
			return fmt.Errorf("%v: the unmatched Restore or Pop broke the snapshot", err)
		}
		red, err := cnOne("red color nodes", root.colorNodes(cnRed))
		if err != nil {
			return err
		}
		clipRect := cnRect(10, 10, 50, 50)
		rc := red.clipper(clipRect)
		if rc == nil || rc.typ != gtk.NodeRoundedClip {
			return errors.New("the fill after ClipRoundedRect(r=5) is under no rounded clip of (10,10 50×50)")
		}
		if got, w := cnRoundedClipOf(rc), cnRounded(clipRect, 5, 5, 5, 5); !cnRoundedNear(got, w) {
			return fmt.Errorf("the clip is %s, want %s", cnRoundedString(got), cnRoundedString(w))
		}
		green, err := cnOne("green color nodes (after Restore)", root.colorNodes(cnGreen))
		if err != nil {
			return err
		}
		if green.clipper(clipRect) != nil {
			return errors.New("the fill after Restore is still under the clip of (10,10 50×50)")
		}
		if !cnRectNear(green.global, cnRect(0, 0, 100, 90)) {
			return fmt.Errorf("the fill after Restore is at %s, want (0,0 100×90)", cnRectString(green.global))
		}
		blue, err := cnOne("blue color nodes", root.colorNodes(cnBlue))
		if err != nil {
			return err
		}
		sharp := cnRect(120, 10, 30, 30)
		switch sc := blue.clipper(sharp); {
		case sc != nil:
			t.Logf("CANVAS-RESULT: restore unclips: a sharp ClipRoundedRect is a %s node", cnTypeNames[sc.typ])
		case cnRectNear(blue.global, sharp):
			t.Log("CANVAS-RESULT: restore unclips: GTK intersected the color node with the sharp clip")
		default:
			return fmt.Errorf("the fill inside the sharp clip is at %s with no clip of %s above it", cnRectString(blue.global), cnRectString(sharp))
		}
		orange, err := cnOne("orange color nodes (after the second Restore)", root.colorNodes(cnOrange))
		if err != nil {
			return err
		}
		if orange.clipper(sharp) != nil || !cnRectNear(orange.global, cnRect(110, 0, 80, 80)) {
			return fmt.Errorf("the fill after the second Restore is at %s, clipped %v; want (110,0 80×80) unclipped",
				cnRectString(orange.global), orange.clipper(sharp) != nil)
		}
		if ferr == nil {
			return errors.New("finish returned no recorder errors for the unmatched Restore and Pop")
		}
		t.Logf("CANVAS-RESULT: restore unclips: the recorder reported %v", ferr)
		return nil
	})
}

// TestCanvasGTKUnbalanced: a paint that leaves Save, a clip and
// PushOpacity open still gives the whole drawing after finish: what was
// drawn before the Save (which an unbalanced snapshot would lose), and the
// open layer's fill under its opacity and rounded clip. finish reports the
// open frames; calls after it draw nothing, and a second finish returns
// nil.
func TestCanvasGTKUnbalanced(t *testing.T) {
	var first, second error
	cnPaint(t, "unbalanced", cnSize, 1, func(c *canvas) {
		c.FillRoundedRect(render.Rect{X: 5, Y: 5, W: 10, H: 10}, render.Radii{}, render.Solid(cnGreen))
		c.Save()
		c.Translate(10, 0)
		c.ClipRoundedRect(render.Rect{X: 0, Y: 0, W: 100, H: 100}, render.UniformRadii(10))
		c.PushOpacity(0.5)
		c.FillRoundedRect(render.Rect{X: 40, Y: 50, W: 80, H: 40}, render.Radii{}, render.Solid(cnRed))
		first = c.finish()
		c.FillRoundedRect(render.Rect{X: 150, Y: 10, W: 20, H: 20}, render.Radii{}, render.Solid(cnBlue))
		c.Pop()
		c.Restore()
		second = c.finish()
	}, func(root *cnNode, ferr error) error {
		if first == nil {
			return errors.New("finish returned no recorder errors for the open Save and PushOpacity")
		}
		t.Logf("CANVAS-RESULT: unbalanced: finish reported %v", first)
		if second != nil || ferr != nil {
			return fmt.Errorf("the second and third finish returned %v, %v; want nil", second, ferr)
		}
		green, err := cnOne("green color nodes (drawn before the Save)", root.colorNodes(cnGreen))
		if err != nil {
			return fmt.Errorf("%v: the snapshot was left unbalanced", err)
		}
		if !cnRectNear(green.global, cnRect(5, 5, 10, 10)) || green.ancestor(gtk.NodeOpacity) != nil {
			return fmt.Errorf("the first fill is at %s (under an opacity node: %v), want (5,5 10×10) opaque",
				cnRectString(green.global), green.ancestor(gtk.NodeOpacity) != nil)
		}
		redRect := cnRect(50, 50, 80, 40)
		red := root.colorNodes(cnRed)
		switch {
		case len(red) == 1:
			op := red[0].ancestor(gtk.NodeOpacity)
			if op == nil || cnOpacityOf(op) != 0.5 {
				return errors.New("the open layer's fill is under no opacity node of 0.5")
			}
		case len(root.colorNodes(driver.RGBA(cnRed.R, cnRed.G, cnRed.B, 0.5))) == 1:
			red = root.colorNodes(driver.RGBA(cnRed.R, cnRed.G, cnRed.B, 0.5))
			t.Log("CANVAS-RESULT: unbalanced: GTK folded the opacity into the color node")
		default:
			return fmt.Errorf("%d red color nodes, want one (the open layer's fill)", len(red))
		}
		if !cnRectNear(red[0].global, redRect) {
			return fmt.Errorf("the open layer's fill is at %s, want %s", cnRectString(red[0].global), cnRectString(redRect))
		}
		rc := red[0].ancestor(gtk.NodeRoundedClip)
		if rc == nil {
			return errors.New("the open layer's fill is under no rounded clip")
		}
		// The clip was set after Translate(10, 0); compare in the root's
		// coordinates through any transform node above the clip.
		if got := cnMapRect(cnAbove(rc), cnRoundedClipOf(rc).Bounds); !cnRectNear(got, cnRect(10, 0, 100, 100)) {
			return fmt.Errorf("the open layer's clip is at %s, want (10,0 100×100)", cnRectString(got))
		}
		if n := len(root.colorNodes(cnBlue)); n != 0 {
			return fmt.Errorf("a fill after finish drew %d color nodes", n)
		}
		return nil
	})
}

// cnAbove returns the transform from n's coordinates (those its bounds
// and clip are in) to the root's.
func cnAbove(n *cnNode) render.Transform {
	acc := render.Identity()
	for a := n.parent; a != nil; a = a.parent {
		if a.typ != gtk.NodeTransform {
			continue
		}
		if m, ok := cnMatrixOf(cnGsk.TransformNodeGetTransform(a.ptr)); ok {
			acc = acc.Then(cnAffine(m))
		}
	}
	return acc
}

// cnPainter is a surface handler that paints like an aqua control: a
// background, a rounded fill, a stroke, a clipped translucent layer and
// text (layout, made by the shell's text system).
type cnPainter struct {
	*surfRec
	layout render.TextLayout
}

var (
	cnSurfBg     = driver.RGBA(0.95, 0.95, 0.96, 1)
	cnSurfFill   = driver.RGBA(0.2, 0.5, 0.95, 1)
	cnSurfStroke = driver.RGBA(0.3, 0.3, 0.32, 1)
	cnSurfLayer  = driver.RGBA(0.9, 0.4, 0.1, 1)
	cnSurfText   = driver.RGBA(0.1, 0.1, 0.1, 1)
)

func (h *cnPainter) Paint(c render.Canvas) {
	h.surfRec.Paint(c)
	c.FillRoundedRect(render.Rect{W: 4000, H: 4000}, render.Radii{}, render.Solid(cnSurfBg))
	c.FillRoundedRect(render.Rect{X: 20, Y: 20, W: 120, H: 24}, render.UniformRadii(6), render.Solid(cnSurfFill))
	c.StrokeRoundedRect(render.Rect{X: 20, Y: 20, W: 120, H: 24}, render.UniformRadii(6), 2, render.Solid(cnSurfStroke))
	c.Save()
	c.Translate(20, 60)
	c.ClipRoundedRect(render.Rect{W: 60, H: 30}, render.UniformRadii(4))
	c.PushOpacity(0.5)
	c.FillRoundedRect(render.Rect{W: 80, H: 40}, render.Radii{}, render.Solid(cnSurfLayer))
	c.Pop()
	c.Restore()
	c.DrawText(h.layout, render.Point{X: 30, Y: 100}, cnSurfText)
}

// TestCanvasGTKSurfacePaint drives a real surface: with debugChecks on,
// Show paints it through the canvas into GTK's snapshot with no
// recorder errors, and the GGSurface's render node (its widget paintable,
// as spike/gtk's renderWidget takes it) holds the fill, the stroke's
// border, the translucent layer and the text: the canvas draws the layouts
// of the shell's text system (surface.snapshot sets it), so a layout the
// handler made with the shell's TextSystem is text nodes in its color
// (cnCheckText, where the fill places the paint's origin in the widget).
func TestCanvasGTKSurfacePaint(t *testing.T) {
	needSurfaceGTK(t)
	cnNeed(t)
	old := debugChecks
	debugChecks = true
	t.Cleanup(func() { debugChecks = old })
	runSurfaceTest(t, func(e *surfTest) error {
		t := e.t
		layout := e.s.TextSystem().NewLayout("Aqua", render.Font{Size: 13}, 0)
		if err := cnIsPango("shell text system's", layout); err != nil {
			return err
		}
		h := &cnPainter{surfRec: &surfRec{t: t, name: "painted", order: &e.order}, layout: layout}
		ss, err := e.s.NewSurface(shell.SurfaceConfig{Title: "gogui canvas", Size: driver.Size{W: 240, H: 160},
			Style: driver.StyleTitled}, h)
		if err != nil {
			return fmt.Errorf("NewSurface: %w", err)
		}
		e.recs = append(e.recs, h.surfRec)
		sf := ss.(*surface)
		g, ok := sf.w.(*gtkWindow)
		if !ok {
			return fmt.Errorf("the window is a %T, not a *gtkWindow", sf.w)
		}
		if err := show(sf, g, h.surfRec); err != nil {
			return err
		}
		iterateFor(settle)
		if sf.paints == 0 || sf.paints != h.paints {
			t.Errorf("the surface counts %d paints, the handler %d; want the same, at least one", sf.paints, h.paints)
		}
		if sf.paintErr != nil {
			t.Errorf("the last paint's recorder errors: %v", sf.paintErr)
		}
		scale := sf.Scale()
		alloc := allocOf(g.surf)

		paintable := gtk.Gtk.WidgetPaintableNew(g.surf)
		if paintable == 0 {
			return errors.New("gtk_widget_paintable_new returned NULL")
		}
		defer gtk.GObject.ObjectUnref(paintable)
		s := gtk.Gtk.SnapshotNew()
		gtk.Gdk.PaintableSnapshot(paintable, s, float64(alloc[0]), float64(alloc[1]))
		node := gtk.Gtk.SnapshotFreeToNode(s)
		if node == 0 {
			t.Logf("CANVAS-RESULT: surface paint: the GGSurface's widget paintable gave no node (allocation %v, %d paints); the node checks are skipped",
				alloc, h.paints)
			return nil
		}
		defer gtk.Gsk.RenderNodeUnref(node)
		root := cnTree(node)
		t.Logf("CANVAS-RESULT: surface paint (GTK %v, scale %v, allocation %v):\n%s", gtk.Version(), scale, alloc, root.dump())
		fail := func(format string, args ...any) error {
			return fmt.Errorf("surface paint: "+format+"\nnode tree:\n%s", append(args, root.dump())...)
		}
		fill, err := cnOne("fill color nodes", root.colorNodes(cnSurfFill))
		if err != nil {
			return fail("%v", err)
		}
		if fill.ancestor(gtk.NodeRoundedClip) == nil {
			return fail("the rounded fill is under no rounded clip")
		}
		if !cnNear(fill.global.Size.Width, 120) || !cnNear(fill.global.Size.Height, 24) {
			return fail("the fill is %s, want 120×24", cnRectString(fill.global))
		}
		stroke, err := cnOne("stroke border nodes", root.borderNodes(cnSurfStroke))
		if err != nil {
			return fail("%v", err)
		}
		if b := cnBorderOf(stroke); !cnNear(b.widths[0], 2) || !cnNear(b.outline.Bounds.Size.Width, 122) ||
			!cnNear(b.outline.Bounds.Size.Height, 26) || !cnNear(b.outline.Corner[0].Width, 7) {
			return fail("the stroke's border is %s widths %v, want 122×26 r=7, width 2", cnRoundedString(b.outline), b.widths)
		}
		layer := root.colorNodes(cnSurfLayer)
		if len(layer) == 0 {
			layer = root.colorNodes(driver.RGBA(cnSurfLayer.R, cnSurfLayer.G, cnSurfLayer.B, 0.5))
		} else if layer[0].ancestor(gtk.NodeOpacity) == nil {
			return fail("the translucent layer's fill is under no opacity node")
		}
		if len(layer) != 1 || layer[0].ancestor(gtk.NodeRoundedClip) == nil {
			return fail("%d layer color nodes, want one under a rounded clip", len(layer))
		}
		// The paint's origin in the node's coordinates is where the fill
		// at (20,20) landed.
		at := render.Transform{A: 1, D: 1, E: float64(fill.global.Origin.X) - 20, F: float64(fill.global.Origin.Y) - 20}
		text := root.textNodes(rgba(cnSurfText))
		if err := cnCheckText("the surface's text", text, layout, at, render.Point{X: 30, Y: 100}); err != nil {
			return fail("%v", err)
		}
		if got := root.find(cnIsText); len(got) != len(text) {
			return fail("%d text nodes, want only the %d of the text drawn", len(got), len(text))
		}
		if n := len(root.borderNodes(cnSurfText)); n != 0 {
			return fail("%d border nodes in the text's color, want none (text drew no placeholder outline since S4.2i)", n)
		}
		return nil
	})
}

// cnGrow is r grown by d on every side.
func cnGrow(r gtk.GrapheneRect, d float32) gtk.GrapheneRect {
	return cnRect(r.Origin.X-d, r.Origin.Y-d, r.Size.Width+2*d, r.Size.Height+2*d)
}
