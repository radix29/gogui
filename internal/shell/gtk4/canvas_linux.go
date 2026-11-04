//go:build linux

package gtk4

import (
	"math"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// gtkSnapshot is the snapshotter over a GtkSnapshot (canvas.go). It is
// used only inside the snapshot vfunc that passed the paint's snapshot.
// GTK copies every struct it is given, so they are Go values valid for the
// call. The canvas draws into cur, the current segment (S4.2f, blur.go): a
// gtk_snapshot_new child whose node is appended to paint when it ends.
type gtkSnapshot struct {
	paint uintptr // the vfunc's GtkSnapshot
	cur   uintptr // the current segment; 0 before begin and after end
}

var _ snapshotter = (*gtkSnapshot)(nil)

// newGtkSnapshot returns the snapshotter of a paint into the GtkSnapshot s.
func newGtkSnapshot(s uintptr) *gtkSnapshot { return &gtkSnapshot{paint: s} }

// gskNode is a GskRenderNode a segment ended in; release drops the
// canvas's reference (the paint's snapshot and other nodes keep theirs).
type gskNode uintptr

func (n gskNode) release() { gtk.Gsk.RenderNodeUnref(uintptr(n)) }

// begin starts the first segment. (gtk_snapshot_new is g_object_new, which
// doesn't return NULL: GLib aborts when memory runs out.)
func (s *gtkSnapshot) begin() { s.cur = gtk.Gtk.SnapshotNew() }

// endSegment ends the current segment (transfer full: free_to_node frees
// the child) and appends its node, if any, to the paint's snapshot.
func (s *gtkSnapshot) endSegment() uintptr {
	n := gtk.Gtk.SnapshotFreeToNode(s.cur)
	s.cur = 0
	if n != 0 {
		gtk.Gtk.SnapshotAppendNode(s.paint, n)
	}
	return n
}

func (s *gtkSnapshot) cut() (renderNode, bool) {
	n := s.endSegment()
	s.cur = gtk.Gtk.SnapshotNew()
	if n == 0 {
		return nil, false
	}
	return gskNode(n), true
}

func (s *gtkSnapshot) end() {
	if n := s.endSegment(); n != 0 {
		gtk.Gsk.RenderNodeUnref(n)
	}
}

func (s *gtkSnapshot) appendNode(n renderNode) {
	if g, ok := n.(gskNode); ok && g != 0 {
		gtk.Gtk.SnapshotAppendNode(s.cur, uintptr(g))
	}
}

func (s *gtkSnapshot) pushBlur(radius float64) { gtk.Gtk.SnapshotPushBlur(s.cur, radius) }

func (s *gtkSnapshot) save()    { gtk.Gtk.SnapshotSave(s.cur) }
func (s *gtkSnapshot) restore() { gtk.Gtk.SnapshotRestore(s.cur) }
func (s *gtkSnapshot) pop()     { gtk.Gtk.SnapshotPop(s.cur) }

func (s *gtkSnapshot) translate(dx, dy float32) {
	p := gtk.GraphenePoint{X: dx, Y: dy}
	gtk.Gtk.SnapshotTranslate(s.cur, &p)
}

func (s *gtkSnapshot) scale(sx, sy float32) { gtk.Gtk.SnapshotScale(s.cur, sx, sy) }

// matrixScratch is the graphene_matrix_t that matrix fills, in g_malloc
// memory (16-byte aligned on 64-bit glibc and musl, as graphene's SIMD
// loads need). It is allocated at the first use, on the GTK thread, and
// kept for the life of the process.
var matrixScratch uintptr

// matrix writes t as graphene_matrix_init_from_2d does, rows
// (a, b, 0, 0), (c, d, 0, 0), (0, 0, 1, 0), (e, f, 0, 1), and applies it.
func (s *gtkSnapshot) matrix(t [6]float32) {
	if matrixScratch == 0 {
		if matrixScratch = gtk.GLib.Malloc0(gtk.MatrixSize); matrixScratch == 0 {
			return
		}
	}
	m := [16]float32{
		t[0], t[1], 0, 0,
		t[2], t[3], 0, 0,
		0, 0, 1, 0,
		t[4], t[5], 0, 1,
	}
	for i, v := range m {
		ffi.WriteInt32(matrixScratch, uintptr(i)*4, int32(math.Float32bits(v)))
	}
	gtk.Gtk.SnapshotTransformMatrix(s.cur, matrixScratch)
}

func (s *gtkSnapshot) pushRoundedClip(r gtk.RoundedRect) {
	gtk.Gtk.SnapshotPushRoundedClip(s.cur, &r)
}

func (s *gtkSnapshot) pushOpacity(alpha float64) { gtk.Gtk.SnapshotPushOpacity(s.cur, alpha) }

func (s *gtkSnapshot) appendColor(c gtk.RGBA, r gtk.GrapheneRect) {
	gtk.Gtk.SnapshotAppendColor(s.cur, &c, &r)
}

func (s *gtkSnapshot) appendBorder(outline gtk.RoundedRect, width float32, c gtk.RGBA) {
	widths := [4]float32{width, width, width, width}
	colors := [4]gtk.RGBA{c, c, c, c}
	gtk.Gtk.SnapshotAppendBorder(s.cur, &outline, &widths, &colors)
}

func (s *gtkSnapshot) appendLinearGradient(r gtk.GrapheneRect, start, end gtk.GraphenePoint, stops []gtk.ColorStop) {
	if len(stops) < 2 {
		return
	}
	gtk.Gtk.SnapshotAppendLinearGradient(s.cur, &r, &start, &end, &stops[0], uintptr(len(stops)))
}

// pushStroke builds the outline's GskPath and a GskStroke of width,
// pushes them and drops its own references: the snapshot keeps a
// reference to the path and a copy of the stroke until the pop. A sharp
// outline is added as a plain rect, so its corners are miter joins
// whatever add_rounded_rect makes of zero radii. (GLib aborts when memory
// runs out, so the constructors don't return NULL.)
func (s *gtkSnapshot) pushStroke(outline gtk.RoundedRect, width float32) {
	b := gtk.Gsk.PathBuilderNew()
	if outline.Corner == [4]gtk.GrapheneSize{} {
		gtk.Gsk.PathBuilderAddRect(b, &outline.Bounds)
	} else {
		gtk.Gsk.PathBuilderAddRoundedRect(b, &outline)
	}
	path := gtk.Gsk.PathBuilderFreeToPath(b)
	st := gtk.Gsk.StrokeNew(width)
	gtk.Gtk.SnapshotPushStroke(s.cur, path, st)
	gtk.Gsk.StrokeFree(st)
	gtk.Gsk.PathUnref(path)
}

// buildPath returns the GskPath of cmds (transfer full).
func buildPath(cmds []pathCmd) uintptr {
	b := gtk.Gsk.PathBuilderNew()
	for _, c := range cmds {
		p := &c.pts
		switch c.op {
		case render.OpMoveTo:
			gtk.Gsk.PathBuilderMoveTo(b, p[0].X, p[0].Y)
		case render.OpLineTo:
			gtk.Gsk.PathBuilderLineTo(b, p[0].X, p[0].Y)
		case render.OpQuadTo:
			gtk.Gsk.PathBuilderQuadTo(b, p[0].X, p[0].Y, p[1].X, p[1].Y)
		case render.OpCubicTo:
			gtk.Gsk.PathBuilderCubicTo(b, p[0].X, p[0].Y, p[1].X, p[1].Y, p[2].X, p[2].Y)
		case render.OpClose:
			gtk.Gsk.PathBuilderClose(b)
		}
	}
	return gtk.Gsk.PathBuilderFreeToPath(b)
}

// newStroke returns the GskStroke of st, to be freed with StrokeFree.
// gsk_stroke_set_dash copies the lengths.
func newStroke(st pathStroke) uintptr {
	s := gtk.Gsk.StrokeNew(st.width)
	gtk.Gsk.StrokeSetLineCap(s, st.cap)
	gtk.Gsk.StrokeSetLineJoin(s, st.join)
	gtk.Gsk.StrokeSetMiterLimit(s, st.miter)
	if len(st.dash) > 0 {
		gtk.Gsk.StrokeSetDash(s, &st.dash[0], uintptr(len(st.dash)))
		gtk.Gsk.StrokeSetDashOffset(s, st.offset)
	}
	return s
}

// The path calls build the GskPath (and GskStroke) for the one call and
// drop them after it: GtkSnapshot keeps its own reference to the path and
// a copy of the stroke.

func (s *gtkSnapshot) appendFill(cmds []pathCmd, c gtk.RGBA) {
	path := buildPath(cmds)
	gtk.Gtk.SnapshotAppendFill(s.cur, path, gtk.FillRuleWinding, &c)
	gtk.Gsk.PathUnref(path)
}

func (s *gtkSnapshot) pushFill(cmds []pathCmd) {
	path := buildPath(cmds)
	gtk.Gtk.SnapshotPushFill(s.cur, path, gtk.FillRuleWinding)
	gtk.Gsk.PathUnref(path)
}

func (s *gtkSnapshot) appendStroke(cmds []pathCmd, st pathStroke, c gtk.RGBA) {
	path, stroke := buildPath(cmds), newStroke(st)
	gtk.Gtk.SnapshotAppendStroke(s.cur, path, stroke, &c)
	gtk.Gsk.StrokeFree(stroke)
	gtk.Gsk.PathUnref(path)
}

func (s *gtkSnapshot) pushPathStroke(cmds []pathCmd, st pathStroke) {
	path, stroke := buildPath(cmds), newStroke(st)
	gtk.Gtk.SnapshotPushStroke(s.cur, path, stroke)
	gtk.Gsk.StrokeFree(stroke)
	gtk.Gsk.PathUnref(path)
}

// appendShadow appends an outset or inset shadow node; GTK copies the
// outline and the color.
func (s *gtkSnapshot) appendShadow(sh shadowCall) {
	if sh.inset {
		gtk.Gtk.SnapshotAppendInsetShadow(s.cur, &sh.outline, &sh.color, sh.dx, sh.dy, sh.spread, sh.blur)
		return
	}
	gtk.Gtk.SnapshotAppendOutsetShadow(s.cur, &sh.outline, &sh.color, sh.dx, sh.dy, sh.spread, sh.blur)
}

// gdkTexture is a GdkTexture the canvas made (S4.2e); release drops the
// shell's reference (render nodes keep their own).
type gdkTexture uintptr

func (t gdkTexture) release() { gtk.GObject.ObjectUnref(uintptr(t)) }

// newTexture copies pix into a GBytes and makes a GDK_MEMORY_R8G8B8A8
// memory texture of it, which keeps the GBytes; the GBytes reference of
// this call is dropped.
func (s *gtkSnapshot) newTexture(pix []byte, w, h int) (texture, bool) {
	if w <= 0 || h <= 0 || w > maxImageSide || h > maxImageSide || len(pix) != w*h*4 {
		return nil, false
	}
	b := gtk.GLib.BytesNew(&pix[0], uintptr(len(pix)))
	t := gtk.Gdk.MemoryTextureNew(int32(w), int32(h), gtk.MemoryR8G8B8A8, b, uintptr(w*4))
	gtk.GLib.BytesUnref(b)
	if t == 0 {
		return nil, false
	}
	return gdkTexture(t), true
}

// appendTexture appends a texture-scale node of t into r, linear filter.
func (s *gtkSnapshot) appendTexture(t texture, r gtk.GrapheneRect) {
	g, ok := t.(gdkTexture)
	if !ok || g == 0 {
		return
	}
	gtk.Gtk.SnapshotAppendScaledTexture(s.cur, uintptr(g), gtk.ScalingFilterLinear, &r)
}

// appendLayout appends the text node of l, a PangoLayout (S4.2i), at the
// current origin in c; GTK copies the color and the node keeps the glyphs
// and fonts it needs, not the layout.
func (s *gtkSnapshot) appendLayout(l nativeLayout, c gtk.RGBA) {
	p, ok := l.(pangoLayout)
	if !ok || p == 0 {
		return
	}
	gtk.Gtk.SnapshotAppendLayout(s.cur, uintptr(p), &c)
}
