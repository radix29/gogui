package gtk4

import (
	"math"
	"os"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	recorder "github.com/radix29/gogui/internal/render/record"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// The canvas (S4.1g; real drawing since S4.2b–S4.2i): a
// render.Canvas over a GtkSnapshot, drawn in GGSurface's snapshot vfunc.
// Like the surface it is untagged and calls GTK through a small interface
// (snapshotter; the GTK one is in canvas_linux.go), so its logic runs in
// host tests.
//
//   - User space is GTK's: logical pixels, which are points; GTK applies
//     the surface scale underneath. The scale only sets one device pixel
//     (hair), the margin of a gradient's cover rect.
//   - State is GtkSnapshot's: Save, Restore and the transforms are
//     gtk_snapshot_save/restore/translate/scale/transform_matrix. Clips are
//     rounded-clip pushes, which GtkSnapshot's restore does not pop, so the
//     canvas counts them per state frame and pops them itself before the
//     frame's restore. The surface's own rect is the first clip.
//     PushOpacity is gtk_snapshot_push_opacity followed by a save.
//     finish closes whatever the paint left open, so the snapshot is always
//     balanced when GtkWidget's own snapshot runs after it.
//   - Rounded rects: a fill is a color (or linear-gradient) node, inside a
//     rounded clip when the corners are rounded. A solid stroke is a border
//     node whose outline is the rect grown by half the width, so the stroke
//     is centered on the outline. A gradient stroke (S4.2b) is a stroke
//     push of the rounded rect's GskPath with the gradient node inside,
//     over the same grown rect.
//   - Paths (S4.2c) are GskPaths built from pathCmd lists, the pure-Go
//     conversion of render.Path (pathCmds). A solid fill or stroke is a
//     fill or stroke node with a color (append_fill/append_stroke); a
//     gradient is a fill or stroke push around a linear-gradient node over
//     the path's bounds (grown by the stroke's reach for strokes, and by a
//     device pixel for the antialiased edge: gradientCover). Fills
//     use the nonzero rule and close open subpaths; strokes take their
//     caps, joins, miter limit and dashes from StrokeStyle.Normalized.
//   - Shadows (S4.2d) are outset and inset shadow nodes of the rounded
//     rect (shadowArgs), which have CSS box-shadow semantics as
//     render.Shadow has: an outset one drawn outside the outline only, an
//     inset one inside it only, GSK's blur radius the CSS one (σ =
//     blur/2), so Shadow.Normalized's values pass through unchanged.
//   - Images (S4.2e, image.go) are scaled texture nodes of the shell's
//     own images, their GdkTextures made on first draw and kept.
//   - The backdrop blur (S4.2f, blur.go) works by segments: the canvas
//     draws into a child snapshot of the paint's (a segment), and a blur
//     ends the segment, appends its node to the paint's snapshot, draws
//     every node so far again, blurred, under the blur's clip, and reopens
//     the open state in a new segment. Every state frame records what it
//     pushed (canvasFrame), so it can be closed and pushed again.
//   - Text (S4.2i) is a native PangoLayout per layout and paint, rebuilt
//     by the text system from the layout's stored text, font and width
//     (texts, released by finish) and drawn with gtk_snapshot_append_layout
//     inside a save and a translate to the origin, so it nests in any open
//     state. Layouts of another text system (fake ones included), empty
//     text, semantic or transparent colors and non-finite origins draw
//     nothing.
//   - Contract violations (render.Canvas) are ignored and never reach GTK:
//     non-finite values (also after narrowing to float32), negative sizes,
//     semantic colors, a Restore that doesn't close a Save or a Pop that
//     doesn't close a Push*. With a recorder (GOGUI_DEBUG=1, or tests) every
//     call is also recorded, and finish returns the recorder's End errors.

// debugChecks makes every paint run the recorder's checks too, and the
// surface log their errors (GOGUI_DEBUG=1, as in package gogui).
var debugChecks = os.Getenv("GOGUI_DEBUG") == "1"

// snapshotter is the GtkSnapshot calls of the canvas, in logical pixels.
// Every value is finite and every size non-negative.
type snapshotter interface {
	save()
	restore()
	translate(dx, dy float32)
	scale(sx, sy float32)
	// matrix applies the affine transform (a, b, c, d, e, f) of
	// render.Transform in the current user space.
	matrix(t [6]float32)
	pushRoundedClip(r gtk.RoundedRect)
	pushOpacity(alpha float64)
	// pop ends the newest push (a clip, an opacity or a stroke).
	pop()
	appendColor(c gtk.RGBA, r gtk.GrapheneRect)
	// appendBorder draws a border node of uniform width inside outline.
	appendBorder(outline gtk.RoundedRect, width float32, c gtk.RGBA)
	// appendLinearGradient fills r with a gradient of two or more stops.
	appendLinearGradient(r gtk.GrapheneRect, start, end gtk.GraphenePoint, stops []gtk.ColorStop)
	// pushStroke limits what is drawn until its pop to the stroke of
	// outline, width wide and centered on it, with GskStroke's defaults
	// otherwise (butt caps, miter joins, miter limit 4).
	pushStroke(outline gtk.RoundedRect, width float32)
	// The path calls (S4.2c). path is never empty; fills use the nonzero
	// (winding) rule.
	//
	// appendFill fills path with c.
	appendFill(path []pathCmd, c gtk.RGBA)
	// pushFill limits what is drawn until its pop to the fill of path.
	pushFill(path []pathCmd)
	// appendStroke strokes path with st in c.
	appendStroke(path []pathCmd, st pathStroke, c gtk.RGBA)
	// pushPathStroke limits what is drawn until its pop to the stroke of
	// path with st.
	pushPathStroke(path []pathCmd, st pathStroke)
	// appendShadow draws an outset or inset shadow node (S4.2d).
	appendShadow(sh shadowCall)
	// The image calls (S4.2e). newTexture makes a texture of w×h
	// straight-alpha RGBA pixels (pix, rows w·4 bytes apart, copied); ok
	// is false when GDK makes none. It may be called outside a paint's
	// pushes: a texture belongs to no snapshot. appendTexture draws t (one
	// newTexture made) scaled into r with the linear filter.
	newTexture(pix []byte, w, h int) (t texture, ok bool)
	appendTexture(t texture, r gtk.GrapheneRect)
	// The segment calls (S4.2f, blur.go). Every other call draws into the
	// current segment, a child snapshot of the paint's. begin starts the
	// first segment (newCanvas). cut ends the current segment, which is
	// balanced, appends its node to the paint's snapshot and starts the
	// next one; it returns the node, a reference the canvas releases, and
	// ok false when the segment drew nothing. end ends the last segment
	// the same way and starts none (finish).
	begin()
	cut() (n renderNode, ok bool)
	end()
	// appendNode draws n (one cut returned) into the current segment.
	appendNode(n renderNode)
	// pushBlur blurs what is drawn until its pop with GSK's blur radius
	// (σ = radius/2), radius > 0.
	pushBlur(radius float64)
	// appendLayout draws l (one pangoLayouter.layout made) with its
	// top-left corner at the user-space origin, in c (S4.2i).
	appendLayout(l nativeLayout, c gtk.RGBA)
}

// frameKind says which call opened a canvas state frame.
type frameKind uint8

const (
	frameBase frameKind = iota // the paint itself: no save, never closed by a call
	frameSave
	frameOpacity
	frameBlur
)

// canvasFrame is one level of the canvas's state stack. It records what
// opening and changing it pushed, so a blur can close it and push it
// again (unwind, rewind).
type canvasFrame struct {
	kind  frameKind
	alpha float64          // frameOpacity: the opacity pushed before the save
	ops   []stateOp        // the frame's transforms and clips, in order
	clips int              // how many of ops are clips
	ctm   render.Transform // the transform when the frame was opened
}

// opKind is the snapshot call of a stateOp.
type opKind uint8

const (
	opTranslate opKind = iota // by m[4], m[5]
	opScale                   // by m[0], m[3]
	opMatrix                  // by m
	opClip                    // to clip
)

// stateOp is one transform or clip call of a frame, its arguments
// narrowed.
type stateOp struct {
	kind opKind
	m    [6]float32
	clip gtk.RoundedRect
}

// transform returns the affine transform of a transform op.
func (op stateOp) transform() render.Transform {
	m := op.m
	switch op.kind {
	case opTranslate:
		return render.Translation(float64(m[4]), float64(m[5]))
	case opScale:
		return render.Scaling(float64(m[0]), float64(m[3]))
	}
	return render.Transform{A: float64(m[0]), B: float64(m[1]), C: float64(m[2]),
		D: float64(m[3]), E: float64(m[4]), F: float64(m[5])}
}

// transformOps returns the calls of the affine transform m: a translation
// with a scaling (no rotation or skew) is a translate and a scale, each
// left out when it does nothing, anything else a matrix.
func transformOps(m [6]float32) []stateOp {
	if m[1] != 0 || m[2] != 0 {
		return []stateOp{{kind: opMatrix, m: m}}
	}
	var ops []stateOp
	if m[4] != 0 || m[5] != 0 {
		ops = append(ops, stateOp{kind: opTranslate, m: [6]float32{1, 0, 0, 1, m[4], m[5]}})
	}
	if m[0] != 1 || m[3] != 1 {
		ops = append(ops, stateOp{kind: opScale, m: [6]float32{m[0], 0, 0, m[3], 0, 0}})
	}
	return ops
}

// canvas is the render.Canvas of one paint.
type canvas struct {
	sn     snapshotter
	size   render.Size      // the surface's, in points
	hair   float64          // one device pixel, in points
	rec    *recorder.Canvas // nil unless debugging
	frames []canvasFrame    // frames[0] is the base
	ctm    render.Transform // the current transform, from the paint's root
	// nodes are the segments ended so far (blur.go), in order: what lies
	// below the current segment. finish releases them.
	nodes []renderNode
	ended bool
	// images is the set whose images DrawImage draws (the shell's; nil
	// draws none).
	images *imageSet
	// text is the text system whose layouts DrawText draws (the shell's;
	// nil draws none), texts the native layouts made in this paint, one per
	// layout (nil after a failure, so it is not retried), released by
	// finish.
	text  *textSystem
	texts map[*textLayout]nativeLayout
}

var _ render.Canvas = (*canvas)(nil)

// newCanvas returns the canvas of one paint of a surface of size at scale
// into sn, clipped to the surface. debug adds a recorder.
func newCanvas(sn snapshotter, size render.Size, scale float64, debug bool) *canvas {
	c := &canvas{sn: sn, size: size, hair: 1, ctm: render.Identity(),
		frames: []canvasFrame{{kind: frameBase, ctm: render.Identity()}}}
	if scale > 0 && !math.IsInf(scale, 1) {
		c.hair = 1 / scale
	}
	if debug {
		c.rec = recorder.New(size)
	}
	sn.begin()
	c.clip(render.Rect{W: size.W, H: size.H}, render.Radii{})
	return c
}

// finish closes every frame still open, the base included, ends the last
// segment, releases the earlier ones and ends the canvas: later calls do
// nothing. It returns the recorder's errors.
func (c *canvas) finish() error {
	if c.ended {
		return nil
	}
	for len(c.frames) > 0 {
		c.closeFrame()
	}
	c.sn.end()
	for _, n := range c.nodes {
		n.release()
	}
	c.nodes = nil
	for _, nl := range c.texts {
		if nl != nil {
			nl.release()
		}
	}
	c.texts = nil
	c.ended = true
	if c.rec != nil {
		return c.rec.End()
	}
	return nil
}

// --- Values --------------------------------------------------------------------

// f32 narrows v; ok is false when the result is not finite.
func f32(v float64) (float32, bool) {
	f := float32(v)
	return f, !math.IsNaN(float64(f)) && !math.IsInf(float64(f), 0)
}

// rectF narrows a finite rect with a non-negative size.
func rectF(r render.Rect) (gtk.GrapheneRect, bool) {
	x, ok1 := f32(r.X)
	y, ok2 := f32(r.Y)
	w, ok3 := f32(r.W)
	h, ok4 := f32(r.H)
	if !ok1 || !ok2 || !ok3 || !ok4 || w < 0 || h < 0 {
		return gtk.GrapheneRect{}, false
	}
	return gtk.Rect(x, y, w, h), true
}

// pointF narrows a finite point.
func pointF(p render.Point) (gtk.GraphenePoint, bool) {
	x, ok1 := f32(p.X)
	y, ok2 := f32(p.Y)
	return gtk.GraphenePoint{X: x, Y: y}, ok1 && ok2
}

// roundedF returns the GskRoundedRect of rf with radii (already clamped).
func roundedF(rf gtk.GrapheneRect, radii render.Radii) gtk.RoundedRect {
	rr := gtk.RoundedRect{Bounds: rf}
	for i, v := range [4]float64{radii.TopLeft, radii.TopRight, radii.BottomRight, radii.BottomLeft} {
		rr.Corner[i] = gtk.GrapheneSize{Width: float32(v), Height: float32(v)}
	}
	return rr
}

// rgba converts a concrete color.
func rgba(c render.Color) gtk.RGBA { return gtk.RGBA{R: c.R, G: c.G, B: c.B, A: c.A} }

// visible reports whether c is concrete and not fully transparent.
func visible(c render.Color) bool { return !c.IsSemantic() && c.A > 0 }

// gradientAt returns the color of the sorted stops at offset t (the
// gradient stroke of a rect without area uses offset 0.5): the first
// stop's color before it, the last one's after it, straight-alpha linear
// interpolation in between.
func gradientAt(stops []render.GradientStop, t float64) (render.Color, bool) {
	if len(stops) == 0 {
		return render.Color{}, false
	}
	if t <= stops[0].Offset {
		return stops[0].Color, true
	}
	for i := 1; i < len(stops); i++ {
		a, b := stops[i-1], stops[i]
		if t > b.Offset {
			continue
		}
		f := float32(0)
		if d := b.Offset - a.Offset; d > 0 {
			f = float32((t - a.Offset) / d)
		}
		mix := func(x, y float32) float32 { return x + (y-x)*f }
		return driver.RGBA(mix(a.Color.R, b.Color.R), mix(a.Color.G, b.Color.G),
			mix(a.Color.B, b.Color.B), mix(a.Color.A, b.Color.A)), true
	}
	return stops[len(stops)-1].Color, true
}

// strokeOutline returns the outer edge of a stroke width w centered on r
// rounded by radii (clamped to r): r grown by w/2 on every side, each
// rounded corner's radius grown by w/2 (sharp corners stay sharp).
func strokeOutline(r render.Rect, radii render.Radii, w float64) (render.Rect, render.Radii) {
	h := w / 2
	grow := func(v float64) float64 {
		if v > 0 {
			return v + h
		}
		return 0
	}
	return render.Rect{X: r.X - h, Y: r.Y - h, W: r.W + w, H: r.H + w},
		render.Radii{TopLeft: grow(radii.TopLeft), TopRight: grow(radii.TopRight),
			BottomRight: grow(radii.BottomRight), BottomLeft: grow(radii.BottomLeft)}
}

// --- Paths ---------------------------------------------------------------------

// pathCmd is one GskPathBuilder call (S4.2c): a move, line, quad, cubic or
// close (render.Op) with its points narrowed to float32 (only the ones the
// op uses are set).
type pathCmd struct {
	op  render.Op
	pts [3]gtk.GraphenePoint
}

// pathStroke is a GskStroke: the line width, GskLineCap, GskLineJoin and
// miter limit, and the dash pattern (nil for a solid line) with its
// offset, in user-space lengths as GSK takes them.
type pathStroke struct {
	width, miter float32
	cap, join    int32
	dash         []float32
	offset       float32
}

// pathCmds converts path into builder calls. Every subpath of a
// render.Path starts with a move. A subpath with nothing but its move (and
// maybe a close) is dropped: it has no segment to fill or stroke, and
// whether caps make a dot of it differs between backends. For a
// fill (fill true) every open subpath is closed, as the nonzero fill
// closes it implicitly; for a stroke open subpaths stay open. ok is false
// for a nil or empty path, a path with a point that is not finite as a
// float32 (the whole path is ignored, as on Win32), and a path left with
// no subpath.
func pathCmds(path *render.Path, fill bool) ([]pathCmd, bool) {
	if path == nil || path.Empty() {
		return nil, false
	}
	var out []pathCmd
	sub := -1 // the index in out of the open subpath's move, or -1
	drawn := false
	end := func(closed bool) {
		switch {
		case sub < 0:
		case !drawn:
			out = out[:sub] // a lone move
		case closed || fill:
			out = append(out, pathCmd{op: render.OpClose})
		}
		sub, drawn = -1, false
	}
	for s := range path.Segments() {
		cmd := pathCmd{op: s.Op}
		for i := range segmentPoints(s.Op) {
			p, ok := pointF(s.Pts[i])
			if !ok {
				return nil, false
			}
			cmd.pts[i] = p
		}
		switch s.Op {
		case render.OpMoveTo:
			end(false)
			sub = len(out)
			out = append(out, cmd)
		case render.OpClose:
			end(true)
		case render.OpLineTo, render.OpQuadTo, render.OpCubicTo:
			if sub >= 0 { // always: render.Path starts every subpath with a move
				out = append(out, cmd)
				drawn = true
			}
		}
	}
	end(false)
	return out, len(out) > 0
}

// segmentPoints returns how many of a segment's points op uses.
func segmentPoints(op render.Op) int {
	switch op {
	case render.OpMoveTo, render.OpLineTo:
		return 1
	case render.OpQuadTo:
		return 2
	case render.OpCubicTo:
		return 3
	}
	return 0
}

// strokeArgs returns the GskStroke of a stroke w wide (render.StrokeWidth
// applied) with the normalized style s. ok is false when the stroke draws
// nothing (w is 0) or a dash length doesn't narrow to a finite float32
// (a miter limit that doesn't is DefaultMiterLimit, as on Win32). GSK's
// caps, joins and dashes mean what render's do: the miter limit is the
// ratio of the miter length to the width, a miter past it becomes a bevel
// (as CoreGraphics and D2D's MITER_OR_BEVEL), dash lengths and the offset
// are user-space lengths, and dashes get the line's caps.
func strokeArgs(w float64, s render.StrokeStyle) (pathStroke, bool) {
	wf, ok := f32(w)
	if !ok || wf == 0 {
		return pathStroke{}, false
	}
	st := pathStroke{width: wf, cap: gtk.LineCapButt, join: gtk.LineJoinMiter}
	switch s.Cap {
	case render.CapRound:
		st.cap = gtk.LineCapRound
	case render.CapSquare:
		st.cap = gtk.LineCapSquare
	}
	switch s.Join {
	case render.JoinRound:
		st.join = gtk.LineJoinRound
	case render.JoinBevel:
		st.join = gtk.LineJoinBevel
	}
	if st.miter, ok = f32(s.MiterLimit); !ok {
		st.miter = render.DefaultMiterLimit // as Win32: the limit overflows float32
	}
	if len(s.Dash) == 0 {
		return st, true
	}
	st.dash = make([]float32, len(s.Dash))
	for i, d := range s.Dash {
		if st.dash[i], ok = f32(d); !ok {
			return pathStroke{}, false
		}
	}
	if st.offset, ok = f32(s.DashOffset); !ok {
		return pathStroke{}, false
	}
	return st, true
}

// strokeReach is how far the stroke st can reach outside its path's
// bounds: half the width, times √2 for square caps (the corner of a cap on
// a diagonal end) and times the miter limit for miter joins (a miter's tip
// is at most limit × w/2 from its corner).
func strokeReach(st pathStroke) float64 {
	k := 1.0
	if st.cap == gtk.LineCapSquare {
		k = math.Sqrt2
	}
	if st.join == gtk.LineJoinMiter {
		k = math.Max(k, float64(st.miter))
	}
	return float64(st.width) / 2 * k
}

// gradientCover is the rect a path's gradient node covers: the path's
// bounds b grown by reach and by one device pixel more, so the
// antialiased edge (and GSK's approximation of curve offsets) stays
// inside it. Coordinates are kept within ±maxCover, so a huge reach still
// narrows; the gradient only sets coverage, the fill or stroke push
// clips it. ok is false for non-finite bounds.
func (c *canvas) gradientCover(b render.Rect, reach float64) (gtk.GrapheneRect, bool) {
	h := reach + c.hair
	x0, y0 := clampCover(b.X-h), clampCover(b.Y-h)
	x1, y1 := clampCover(b.X+b.W+h), clampCover(b.Y+b.H+h)
	return rectF(render.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0})
}

// maxCover bounds gradientCover's coordinates, far beyond any surface.
const maxCover = 1e30

func clampCover(v float64) float64 { return math.Max(-maxCover, math.Min(maxCover, v)) }

// --- Shadows -------------------------------------------------------------------

// shadowCall is one gtk_snapshot_append_outset_shadow (inset false) or
// append_inset_shadow (S4.2d): the shape's outline (radii clamped to it),
// the color, the offset, the spread and GSK's blur radius.
type shadowCall struct {
	outline      gtk.RoundedRect
	color        gtk.RGBA
	dx, dy       float32
	spread, blur float32
	inset        bool
}

// shadowArgs returns the shadow node of DrawShadow(r, radii, s), s
// normalized. GSK's shadow nodes mean what render.Shadow means (CSS
// box-shadow): the offset moves the shadow shape, the spread grows it (an
// inset one's hole shrinks by it) with each rounded corner's radius and
// sharp corners staying sharp, an outset shadow is drawn outside the
// outline only and an inset one inside it only, and the blur radius is the
// CSS one, σ = blur/2. ok is false when the shadow draws nothing, as on
// Win32: a semantic or transparent color, a shape with no area or not
// finite, an outer shape shrunk away by a negative spread, or an offset,
// spread or blur beyond ±maxShadow (GSK adds them up in float32, so
// finite but huge values could make Inf node bounds; Win32 likewise drops
// a shadow whose mask exceeds its texture limit). An inset shadow whose
// hole is shrunk away covers the whole shape, as in CSS.
func shadowArgs(r render.Rect, radii render.Radii, s render.Shadow) (shadowCall, bool) {
	if s.IsSemantic() || !visible(s.Color) {
		return shadowCall{}, false
	}
	for _, v := range [4]float64{s.Offset.X, s.Offset.Y, s.Spread, s.Blur} {
		if !(math.Abs(v) <= maxShadow) {
			return shadowCall{}, false
		}
	}
	rf, ok := rectF(r)
	if !ok || rf.Size.Width == 0 || rf.Size.Height == 0 {
		return shadowCall{}, false
	}
	if !s.Inset && (!(r.W+2*s.Spread > 0) || !(r.H+2*s.Spread > 0)) {
		return shadowCall{}, false
	}
	sh := shadowCall{outline: roundedF(rf, radii.Clamped(r)), color: rgba(s.Color), inset: s.Inset}
	var ok1, ok2, ok3, ok4 bool
	sh.dx, ok1 = f32(s.Offset.X)
	sh.dy, ok2 = f32(s.Offset.Y)
	sh.spread, ok3 = f32(s.Spread)
	sh.blur, ok4 = f32(s.Blur)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return shadowCall{}, false
	}
	return sh, true
}

// maxShadow bounds a shadow's offset, spread and blur in points, far
// beyond any surface and far below where float32 sums overflow.
const maxShadow = 1e6

// --- Boxes ---------------------------------------------------------------------

// boxOK keeps a finite box with an area (blur.go).
func boxOK(r render.Rect) (render.Rect, bool) {
	if _, ok := rectF(r); !ok || !(r.W > 0) || !(r.H > 0) {
		return render.Rect{}, false
	}
	return r, true
}

// --- State ---------------------------------------------------------------------

func (c *canvas) top() *canvasFrame { return &c.frames[len(c.frames)-1] }

// open starts a frame of kind; PushOpacity has pushed its opacity first.
func (c *canvas) open(kind frameKind) {
	c.sn.save()
	c.frames = append(c.frames, canvasFrame{kind: kind, ctm: c.ctm})
}

// emit makes the snapshot call of op.
func (c *canvas) emit(op stateOp) {
	switch op.kind {
	case opTranslate:
		c.sn.translate(op.m[4], op.m[5])
	case opScale:
		c.sn.scale(op.m[0], op.m[3])
	case opMatrix:
		c.sn.matrix(op.m)
	case opClip:
		c.sn.pushRoundedClip(op.clip)
	}
}

// do makes op's call, records it in the newest frame and keeps the
// transform.
func (c *canvas) do(op stateOp) {
	c.emit(op)
	f := c.top()
	f.ops = append(f.ops, op)
	if op.kind == opClip {
		f.clips++
		return
	}
	c.ctm = c.ctm.Pre(op.transform())
}

// unwind makes the calls that close f: its clips, its save and its
// opacity push.
func (c *canvas) unwind(f *canvasFrame) {
	for range f.clips {
		c.sn.pop()
	}
	if f.kind != frameBase {
		c.sn.restore()
	}
	if f.kind == frameOpacity {
		c.sn.pop()
	}
}

// rewind makes the calls that open f again as unwind found it: its
// opacity push, its save and its ops in order.
func (c *canvas) rewind(f *canvasFrame) {
	if f.kind == frameOpacity {
		c.sn.pushOpacity(f.alpha)
	}
	if f.kind != frameBase {
		c.sn.save()
	}
	for _, op := range f.ops {
		c.emit(op)
	}
}

// closeFrame closes the newest frame and restores the transform it was
// opened with.
func (c *canvas) closeFrame() {
	f := &c.frames[len(c.frames)-1]
	c.unwind(f)
	c.ctm = f.ctm
	c.frames = c.frames[:len(c.frames)-1]
}

// closeIf closes the newest frame when it is of one of kinds and not the
// base.
func (c *canvas) closeIf(kinds ...frameKind) {
	if len(c.frames) < 2 {
		return
	}
	for _, k := range kinds {
		if c.top().kind == k {
			c.closeFrame()
			return
		}
	}
}

// Save implements render.Canvas.
func (c *canvas) Save() {
	if c.rec != nil {
		c.rec.Save()
	}
	if !c.ended {
		c.open(frameSave)
	}
}

// Restore implements render.Canvas.
func (c *canvas) Restore() {
	if c.rec != nil {
		c.rec.Restore()
	}
	if !c.ended {
		c.closeIf(frameSave)
	}
}

// Translate implements render.Canvas.
func (c *canvas) Translate(dx, dy float64) {
	if c.rec != nil {
		c.rec.Translate(dx, dy)
	}
	x, ok1 := f32(dx)
	y, ok2 := f32(dy)
	if !c.ended && ok1 && ok2 {
		c.do(stateOp{kind: opTranslate, m: [6]float32{1, 0, 0, 1, x, y}})
	}
}

// Scale implements render.Canvas.
func (c *canvas) Scale(sx, sy float64) {
	if c.rec != nil {
		c.rec.Scale(sx, sy)
	}
	x, ok1 := f32(sx)
	y, ok2 := f32(sy)
	if !c.ended && ok1 && ok2 {
		c.do(stateOp{kind: opScale, m: [6]float32{x, 0, 0, y, 0, 0}})
	}
}

// Concat implements render.Canvas. A translation with a scaling (no
// rotation or skew) is a translate and a scale, anything else a matrix.
func (c *canvas) Concat(t render.Transform) {
	if c.rec != nil {
		c.rec.Concat(t)
	}
	if c.ended {
		return
	}
	var m [6]float32
	for i, v := range [6]float64{t.A, t.B, t.C, t.D, t.E, t.F} {
		f, ok := f32(v)
		if !ok {
			return
		}
		m[i] = f
	}
	for _, op := range transformOps(m) {
		c.do(op)
	}
}

// ClipRoundedRect implements render.Canvas.
func (c *canvas) ClipRoundedRect(r render.Rect, radii render.Radii) {
	if c.rec != nil {
		c.rec.ClipRoundedRect(r, radii)
	}
	if !c.ended {
		c.clip(r, radii)
	}
}

// clip pushes a rounded clip in the newest frame. A negative or NaN size
// clips everything away, as the recorder's empty intersection does; other
// non-finite rects are ignored.
func (c *canvas) clip(r render.Rect, radii render.Radii) {
	rf, ok := rectF(r)
	if !ok {
		if !(r.W >= 0) || !(r.H >= 0) {
			c.do(stateOp{kind: opClip})
		}
		return
	}
	c.do(stateOp{kind: opClip, clip: roundedF(rf, radii.Clamped(r))})
}

// PushOpacity implements render.Canvas.
func (c *canvas) PushOpacity(alpha float64) {
	if c.rec != nil {
		c.rec.PushOpacity(alpha)
	}
	if !c.ended {
		a := render.Opacity(alpha)
		c.sn.pushOpacity(a)
		c.open(frameOpacity)
		c.top().alpha = a
	}
}

// Pop implements render.Canvas.
func (c *canvas) Pop() {
	if c.rec != nil {
		c.rec.Pop()
	}
	if !c.ended {
		c.closeIf(frameOpacity, frameBlur)
	}
}

// --- Drawing -------------------------------------------------------------------

// FillRoundedRect implements render.Canvas.
func (c *canvas) FillRoundedRect(r render.Rect, radii render.Radii, p render.Paint) {
	if c.rec != nil {
		c.rec.FillRoundedRect(r, radii, p)
	}
	if c.ended {
		return
	}
	rf, ok := rectF(r)
	if !ok || rf.Size.Width == 0 || rf.Size.Height == 0 || p.IsSemantic() {
		return
	}
	draw, ok := c.filler(rf, p.Normalized())
	if !ok {
		return
	}
	if radii = radii.Clamped(r); radii.IsZero() {
		draw()
		return
	}
	c.sn.pushRoundedClip(roundedF(rf, radii))
	draw()
	c.sn.pop()
}

// filler returns what fills rf with the normalized paint p; ok is false
// when it draws nothing.
func (c *canvas) filler(rf gtk.GrapheneRect, p render.Paint) (func(), bool) {
	col, g, ok := paintArgs(p)
	switch {
	case !ok:
		return nil, false
	case g != nil:
		return func() { c.sn.appendLinearGradient(rf, g.start, g.end, g.stops) }, true
	}
	return func() { c.sn.appendColor(rgba(col), rf) }, true
}

// linearArgs are the arguments of a linear-gradient node: two or more
// stops, at least one of them visible, along a line of non-zero length.
type linearArgs struct {
	start, end gtk.GraphenePoint
	stops      []gtk.ColorStop
}

// paintArgs resolves the normalized, concrete paint p into what GSK draws:
// a solid color (g nil) or a linear gradient (g set). A gradient of one
// stop is that stop's color, and one whose line has length 0 is its last
// stop's color (CSS). ok is false when p draws nothing: a transparent
// color, no stops, only transparent stops, or a non-finite gradient line.
func paintArgs(p render.Paint) (col render.Color, g *linearArgs, ok bool) {
	solid := func(col render.Color) (render.Color, *linearArgs, bool) {
		return col, nil, visible(col)
	}
	if p.Kind != render.PaintLinear {
		return solid(p.Color)
	}
	switch len(p.Stops) {
	case 0:
		return render.Color{}, nil, false
	case 1:
		return solid(p.Stops[0].Color)
	}
	start, ok1 := pointF(p.Start)
	end, ok2 := pointF(p.End)
	if !ok1 || !ok2 {
		return render.Color{}, nil, false
	}
	if start == end {
		return solid(p.Stops[len(p.Stops)-1].Color)
	}
	stops := make([]gtk.ColorStop, len(p.Stops))
	seen := false
	for i, s := range p.Stops {
		stops[i] = gtk.ColorStop{Offset: float32(s.Offset), Color: rgba(s.Color)}
		seen = seen || s.Color.A > 0
	}
	if !seen {
		return render.Color{}, nil, false
	}
	return render.Color{}, &linearArgs{start: start, end: end, stops: stops}, true
}

// StrokeRoundedRect implements render.Canvas. A solid stroke is a border
// node. A gradient stroke is the gradient node over the stroke's outer
// bounds inside a stroke push of the rounded rect's path; only a rect
// without area keeps a border node, in the gradient's middle color (GSK
// strokes such a path without the border's square ends).
func (c *canvas) StrokeRoundedRect(r render.Rect, radii render.Radii, width float64, p render.Paint) {
	if c.rec != nil {
		c.rec.StrokeRoundedRect(r, radii, width, p)
	}
	if c.ended {
		return
	}
	w := render.StrokeWidth(width)
	rf, rok := rectF(r)
	if !rok || w == 0 || p.IsSemantic() {
		return
	}
	np := p.Normalized()
	col, g, ok := paintArgs(np)
	if !ok {
		return
	}
	radii = radii.Clamped(r)
	outer, oradii := strokeOutline(r, radii, w)
	of, ok1 := rectF(outer)
	wf, ok2 := f32(w)
	if !ok1 || !ok2 {
		return
	}
	if g != nil {
		if rf.Size.Width > 0 && rf.Size.Height > 0 {
			c.sn.pushStroke(roundedF(rf, radii), wf)
			c.sn.appendLinearGradient(of, g.start, g.end, g.stops)
			c.sn.pop()
			return
		}
		if col, ok = gradientAt(np.Stops, 0.5); !ok || !visible(col) {
			return
		}
	}
	c.sn.appendBorder(roundedF(of, oradii.Clamped(outer)), wf, rgba(col))
}

// FillPath implements render.Canvas: a fill node of the path's GskPath
// (nonzero rule, open subpaths closed) with the paint's color, or a fill
// push around the gradient node over the path's bounds (gradientCover).
// Nil and empty
// paths, paths with a non-finite point, paths whose bounds have no area,
// semantic paints and paints that draw nothing are ignored.
func (c *canvas) FillPath(path *render.Path, p render.Paint) {
	if c.rec != nil {
		c.rec.FillPath(path, p)
	}
	if c.ended || p.IsSemantic() {
		return
	}
	col, g, ok := paintArgs(p.Normalized())
	if !ok {
		return
	}
	cmds, ok := pathCmds(path, true)
	if !ok {
		return
	}
	bounds := path.Bounds()
	if b, ok := rectF(bounds); !ok || b.Size.Width == 0 || b.Size.Height == 0 {
		return
	}
	if g == nil {
		c.sn.appendFill(cmds, rgba(col))
		return
	}
	cover, ok := c.gradientCover(bounds, 0)
	if !ok {
		return
	}
	c.sn.pushFill(cmds)
	c.sn.appendLinearGradient(cover, g.start, g.end, g.stops)
	c.sn.pop()
}

// StrokePath implements render.Canvas: a stroke node of the path's GskPath
// with the GskStroke of the width and s (strokeArgs) in the paint's color,
// or a stroke push around the gradient node over the path's bounds grown
// by the stroke's reach (gradientCover). Nil and empty paths, paths with
// a non-finite
// point, a zero (or NaN) width, lengths that don't narrow, semantic paints
// and paints that draw nothing are ignored.
func (c *canvas) StrokePath(path *render.Path, width float64, p render.Paint, s render.StrokeStyle) {
	if c.rec != nil {
		c.rec.StrokePath(path, width, p, s)
	}
	if c.ended || p.IsSemantic() {
		return
	}
	st, ok := strokeArgs(render.StrokeWidth(width), s.Normalized())
	if !ok {
		return
	}
	col, g, ok := paintArgs(p.Normalized())
	if !ok {
		return
	}
	cmds, ok := pathCmds(path, false)
	if !ok {
		return
	}
	if g == nil {
		c.sn.appendStroke(cmds, st, rgba(col))
		return
	}
	bf, ok := c.gradientCover(path.Bounds(), strokeReach(st))
	if !ok {
		return
	}
	c.sn.pushPathStroke(cmds, st)
	c.sn.appendLinearGradient(bf, g.start, g.end, g.stops)
	c.sn.pop()
}

// DrawShadow implements render.Canvas: an outset or inset shadow node
// (shadowArgs), which nests in any open state and opens none.
func (c *canvas) DrawShadow(r render.Rect, radii render.Radii, s render.Shadow) {
	if c.rec != nil {
		c.rec.DrawShadow(r, radii, s)
	}
	if c.ended {
		return
	}
	if sh, ok := shadowArgs(r, radii, s.Normalized()); ok {
		c.sn.appendShadow(sh)
	}
}

// DrawText implements render.Canvas: the layout's native PangoLayout
// (made once per paint, texts) drawn at origin in color, under the
// current state. Nil layouts, layouts that are not this canvas's text
// system's (fake ones and those of another shell), layouts of a released
// text system, empty text, semantic or transparent colors and origins that
// are not finite as float32 draw nothing; so does a layout whose native
// one can't be made.
func (c *canvas) DrawText(l render.TextLayout, origin render.Point, color render.Color) {
	if c.rec != nil {
		c.rec.DrawText(l, origin, color)
	}
	if c.ended || c.text == nil || !visible(color) {
		return
	}
	tl, ok := l.(*textLayout)
	if !ok || tl == nil || tl.ts != c.text || tl.text == "" {
		return
	}
	o, ok := pointF(origin)
	if !ok {
		return
	}
	nl, made := c.texts[tl]
	if !made {
		nl = c.text.native(tl)
		if c.texts == nil {
			c.texts = make(map[*textLayout]nativeLayout)
		}
		c.texts[tl] = nl
	}
	if nl == nil {
		return
	}
	c.sn.save()
	c.sn.translate(o.X, o.Y)
	c.sn.appendLayout(nl, rgba(color))
	c.sn.restore()
}
