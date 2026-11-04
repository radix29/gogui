//go:build windows

package win32

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/render/record"
	"github.com/radix29/gogui/internal/shell/win32/com"
	"github.com/radix29/gogui/internal/shell/win32/d2d"
	"github.com/radix29/gogui/internal/shell/win32/dwrite"
)

// The Direct2D canvas (S2.4d_2): a render.Canvas on the shell's device
// context, used between BeginDraw and EndDraw for one frame.
//
//   - State: the transform is kept in Go and mirrored into the context with
//     SetTransform on every change (the surface scale stays in SetDpi, so
//     user space is logical points). Clips are D2D pushes: a sharp rect
//     under an axis-preserving transform is a PushAxisAlignedClip,
//     everything else a layer with a rounded-rect geometry mask. Each Save
//     (and each Push*) remembers the transform and how many pushes were
//     open; Restore (Pop) pops the pushes made since, in LIFO order, and
//     finish pops whatever is left.
//   - Resources: solid brushes by color, stop collections by stops,
//     gradient brushes by stops and end points, and stroke styles by style
//     are cached for the frame; geometries are made per call. All of them
//     are released by release, after EndDraw.
//   - Contract violations (render.Canvas, PLAN.md §5.3) are ignored and
//     never reach D2D: non-finite values (also after narrowing to float32),
//     negative sizes, semantic colors, a Restore that doesn't close a Save
//     or a Pop that doesn't close a Push*. Leftover saves are closed by
//     finish. With a recorder (GOGUI_DEBUG=1) every call is also recorded,
//     and finish returns the recorder's End errors.
//   - Group opacity (S2.4e_2): PushOpacity pushes a layer with the clamped
//     opacity and infinite bounds on the same push stack, so Pop and finish
//     close it; an opacity of 1 pushes a state frame only. Shadows are in
//     shadow.go, backdrop blur (S2.4e_3) in blur.go: PushBackdropBlur
//     draws the blurred copy and leaves its clip open until Pop. Every
//     push records what it takes to push it again (canvasPush), since the
//     blur closes the open layers for its copy and reopens them.
//   - Images (S2.4e_4) are in image.go: DrawImage draws a shell image's
//     bitmap, made on first draw and kept until the device goes.
//   - Text (S2.5f): DrawText draws a layout of the shell's text system
//     (canvas.text) with DrawTextLayout. Layouts keep no native object
//     (text.go), so the first DrawText of a layout in a frame rebuilds its
//     IDWriteTextLayout from the text, font and width, and release gives it
//     back with the frame's other resources. Text antialiasing is grayscale
//     for the whole frame (set by newCanvas): no ClearType color fringes.

// canvasFrameKind says which call pushed a canvas state frame.
type canvasFrameKind uint8

const (
	canvasSave canvasFrameKind = iota
	canvasOpacity
	canvasBlur
)

// canvasFrame is one level of the state stack.
type canvasFrame struct {
	kind   canvasFrameKind
	xf     render.Transform
	pushes int // len(canvas.pushes) at the push
}

// pushKind is a D2D push that has to be popped.
type pushKind uint8

const (
	pushClip  pushKind = iota // PushAxisAlignedClip
	pushLayer                 // PushLayer
)

// canvasPush is an open D2D push with what it takes to push it again
// (PushBackdropBlur closes and reopens the open layers, blur.go).
type canvasPush struct {
	kind  pushKind
	xf    d2d.Matrix3x2F       // the transform at the push
	rect  d2d.RectF            // pushClip
	aa    d2d.AntialiasMode    // pushClip
	layer d2d.LayerParameters1 // pushLayer (the mask is one of the frame's geometries)
}

// strokeKey identifies a cached stroke style.
type strokeKey struct {
	cap        d2d.CapStyle
	join       d2d.LineJoin
	miter      float32
	offset     float32
	dashes     string // the custom dashes' float32 bits
	dashStyled bool
}

// gradKey identifies a cached gradient brush.
type gradKey struct {
	stops      string
	start, end d2d.Point2F
}

// owned is a resource made during the frame, released by release.
type owned struct {
	what string
	p    com.Ptr
}

// canvas is the D2D render.Canvas. Create it with newCanvas after
// BeginDraw, call finish before EndDraw and release after it.
type canvas struct {
	gpu  *gpu
	dc   d2d.DeviceContext
	mask d2d.DeviceContext // the shadow masks' context (shadow.go)
	fac  d2d.Factory1
	dpi  float32 // the surface's

	xf     render.Transform
	frames []canvasFrame
	pushes []canvasPush
	ended  bool

	solids  map[d2d.ColorF]d2d.SolidColorBrush
	stops   map[string]d2d.GradientStopCollection
	grads   map[gradKey]d2d.LinearGradientBrush
	strokes map[strokeKey]d2d.StrokeStyle
	geoms   []owned

	target blurTarget // blur.go
	blur   pendingBlur
	blurFx blurEffects
	blurs  int // blurred copies drawn (tests)

	masks     map[maskKey]shadowMask // shadow.go
	effects   map[effectKey]shadowEffect
	maskBrush d2d.SolidColorBrush // on the mask context
	maskHits  int                 // DrawShadow calls that reused a mask (tests)

	images *imageSet // image.go; nil draws no images

	text  *textSystem                   // the shell's; nil draws no text
	texts map[*layout]dwrite.TextLayout // the native layouts drawn this frame

	rec  *record.Canvas // nil unless debugging
	errs []error        // resource creation failures
}

var _ render.Canvas = (*canvas)(nil)

// newCanvas returns a canvas on g's device context, which is in BeginDraw
// at dpi, for a surface of size logical points, and sets the identity
// transform. debug adds a recorder.
func newCanvas(g *gpu, size render.Size, dpi float32, debug bool) *canvas {
	dc := g.dc
	c := &canvas{
		gpu: g, dc: dc, mask: g.mask, fac: g.d2f, dpi: dpi, xf: render.Identity(),
		solids:  map[d2d.ColorF]d2d.SolidColorBrush{},
		stops:   map[string]d2d.GradientStopCollection{},
		grads:   map[gradKey]d2d.LinearGradientBrush{},
		strokes: map[strokeKey]d2d.StrokeStyle{},
		masks:   map[maskKey]shadowMask{},
		effects: map[effectKey]shadowEffect{},
		texts:   map[*layout]dwrite.TextLayout{},
	}
	if debug {
		if !(size.W >= 0) || math.IsInf(size.W, 0) {
			size.W = 0
		}
		if !(size.H >= 0) || math.IsInf(size.H, 0) {
			size.H = 0
		}
		c.rec = record.New(size)
	}
	dc.SetTransform(d2d.IdentityMatrix())
	dc.SetTextAntialiasMode(d2d.TEXT_ANTIALIAS_MODE_GRAYSCALE) // no ClearType fringes (PLAN.md §3)
	return c
}

// finish pops every open clip and layer, resets the transform and ends
// the canvas (later calls are ignored). It returns the recorder's errors
// (nil without a recorder). Call it before EndDraw.
func (c *canvas) finish() error {
	if c.ended {
		return nil
	}
	c.popTo(0)
	c.frames = c.frames[:0]
	c.xf = render.Identity()
	c.dc.SetTransform(d2d.IdentityMatrix())
	c.ended = true
	if c.rec != nil {
		if err := c.rec.End(); err != nil {
			return fmt.Errorf("win32: Paint broke the canvas contract: %w", err)
		}
	}
	return nil
}

// failures returns the resource creation errors of the frame, joined.
func (c *canvas) failures() error { return errors.Join(c.errs...) }

// resources returns the number of resources the canvas holds (tests).
func (c *canvas) resources() int {
	n := len(c.solids) + len(c.stops) + len(c.grads) + len(c.strokes) + len(c.geoms) + len(c.masks) + len(c.effects) + len(c.texts)
	if !c.maskBrush.IsNil() {
		n++
	}
	if !c.blurFx.blur.IsNil() {
		n += 2
	}
	return n
}

// release releases every resource of the frame; each release must be the
// last reference, else leak gets the rest. Call it after EndDraw.
func (c *canvas) release(leak func(what string, left uint32)) {
	last := func(what string, p *com.Ptr) {
		if n := p.Release(); n != 0 && leak != nil {
			leak(what, n)
		}
	}
	c.releaseBlur(last) // before the geometries, which hold retired backdrops
	c.releaseShadows(last)
	for k, l := range c.texts {
		last("IDWriteTextLayout", &l.Ptr)
		delete(c.texts, k)
	}
	// Brushes before the stop collections they hold.
	for k, b := range c.grads {
		last("ID2D1LinearGradientBrush", &b.Ptr)
		delete(c.grads, k)
	}
	for k, s := range c.stops {
		last("ID2D1GradientStopCollection", &s.Ptr)
		delete(c.stops, k)
	}
	for k, b := range c.solids {
		last("ID2D1SolidColorBrush", &b.Ptr)
		delete(c.solids, k)
	}
	for k, s := range c.strokes {
		last("ID2D1StrokeStyle1", &s.Ptr)
		delete(c.strokes, k)
	}
	for i := range c.geoms {
		last(c.geoms[i].what, &c.geoms[i].p)
	}
	c.geoms = c.geoms[:0]
}

// fail records a resource creation failure.
func (c *canvas) fail(err error) {
	if len(c.errs) < 8 {
		c.errs = append(c.errs, err)
	}
}

// --- Narrowing -------------------------------------------------------------

// f32 narrows v; ok is false when v or the result is not finite.
func f32(v float64) (float32, bool) {
	f := float32(v)
	return f, !math.IsNaN(v) && !math.IsInf(float64(f), 0)
}

func point2F(p render.Point) (d2d.Point2F, bool) {
	x, okx := f32(p.X)
	y, oky := f32(p.Y)
	return d2d.Point2F{X: x, Y: y}, okx && oky
}

// rectF narrows r; ok is false for non-finite values and negative sizes.
func rectF(r render.Rect) (d2d.RectF, bool) {
	if !(r.W >= 0) || !(r.H >= 0) {
		return d2d.RectF{}, false
	}
	l, ok1 := f32(r.X)
	t, ok2 := f32(r.Y)
	rr, ok3 := f32(r.X + r.W)
	b, ok4 := f32(r.Y + r.H)
	return d2d.RectF{Left: l, Top: t, Right: rr, Bottom: b}, ok1 && ok2 && ok3 && ok4
}

// matrix narrows t; ok is false when an entry is not finite.
func matrix(t render.Transform) (d2d.Matrix3x2F, bool) {
	var m d2d.Matrix3x2F
	ok := true
	for i, v := range [...]float64{t.A, t.B, t.C, t.D, t.E, t.F} {
		f, fine := f32(v)
		ok = ok && fine
		switch i {
		case 0:
			m.M11 = f
		case 1:
			m.M12 = f
		case 2:
			m.M21 = f
		case 3:
			m.M22 = f
		case 4:
			m.Dx = f
		case 5:
			m.Dy = f
		}
	}
	return m, ok
}

// colorF converts a concrete color, clamping every channel to [0, 1] (NaN
// counts as 0).
func colorF(c render.Color) d2d.ColorF {
	u := func(v float32) float32 {
		switch {
		case !(v > 0):
			return 0
		case v > 1:
			return 1
		}
		return v
	}
	return d2d.ColorF{R: u(c.R), G: u(c.G), B: u(c.B), A: u(c.A)}
}

// --- State -----------------------------------------------------------------

func (c *canvas) push(kind canvasFrameKind) {
	c.frames = append(c.frames, canvasFrame{kind: kind, xf: c.xf, pushes: len(c.pushes)})
}

// pop closes the innermost frame if it has one of the kinds.
func (c *canvas) pop(kinds ...canvasFrameKind) {
	if len(c.frames) == 0 {
		return
	}
	top := c.frames[len(c.frames)-1]
	match := false
	for _, k := range kinds {
		match = match || top.kind == k
	}
	if !match {
		return
	}
	c.frames = c.frames[:len(c.frames)-1]
	c.popTo(top.pushes)
	if top.xf != c.xf {
		m, _ := matrix(top.xf) // every stored transform was narrowed before
		c.xf = top.xf
		c.dc.SetTransform(m)
	}
}

// addClip pushes an axis-aligned clip under the current transform.
func (c *canvas) addClip(r d2d.RectF, aa d2d.AntialiasMode) {
	m, _ := matrix(c.xf) // c.xf was narrowed when it was set
	c.dc.PushAxisAlignedClip(r, aa)
	c.pushes = append(c.pushes, canvasPush{kind: pushClip, xf: m, rect: r, aa: aa})
}

// addLayer pushes a layer under the current transform.
func (c *canvas) addLayer(p d2d.LayerParameters1) {
	m, _ := matrix(c.xf)
	c.dc.PushLayer(&p)
	c.pushes = append(c.pushes, canvasPush{kind: pushLayer, xf: m, layer: p})
}

// popTo pops D2D pushes, innermost first, until n are left.
func (c *canvas) popTo(n int) {
	for len(c.pushes) > n {
		switch c.pushes[len(c.pushes)-1].kind {
		case pushClip:
			c.dc.PopAxisAlignedClip()
		case pushLayer:
			c.dc.PopLayer()
		}
		c.pushes = c.pushes[:len(c.pushes)-1]
	}
}

// setTransform makes t current, unless it doesn't narrow.
func (c *canvas) setTransform(t render.Transform) {
	m, ok := matrix(t)
	if !ok {
		return
	}
	c.xf = t
	c.dc.SetTransform(m)
}

// Save implements render.Canvas.
func (c *canvas) Save() {
	if c.rec != nil {
		c.rec.Save()
	}
	if !c.ended {
		c.push(canvasSave)
	}
}

// Restore implements render.Canvas.
func (c *canvas) Restore() {
	if c.rec != nil {
		c.rec.Restore()
	}
	if !c.ended {
		c.pop(canvasSave)
	}
}

// Translate implements render.Canvas.
func (c *canvas) Translate(dx, dy float64) {
	if c.rec != nil {
		c.rec.Translate(dx, dy)
	}
	if !c.ended {
		c.setTransform(c.xf.Pre(render.Translation(dx, dy)))
	}
}

// Scale implements render.Canvas.
func (c *canvas) Scale(sx, sy float64) {
	if c.rec != nil {
		c.rec.Scale(sx, sy)
	}
	if !c.ended {
		c.setTransform(c.xf.Pre(render.Scaling(sx, sy)))
	}
}

// Concat implements render.Canvas.
func (c *canvas) Concat(t render.Transform) {
	if c.rec != nil {
		c.rec.Concat(t)
	}
	if !c.ended {
		c.setTransform(c.xf.Pre(t))
	}
}

// axisPreserving reports whether t maps axis-aligned rects to axis-aligned
// rects (no rotation or skew), so PushAxisAlignedClip clips exactly.
func axisPreserving(t render.Transform) bool { return t.B == 0 && t.C == 0 }

// ClipRoundedRect implements render.Canvas. An empty rect clips
// everything away.
func (c *canvas) ClipRoundedRect(r render.Rect, radii render.Radii) {
	if c.rec != nil {
		c.rec.ClipRoundedRect(r, radii)
	}
	if c.ended {
		return
	}
	rf, ok := rectF(r)
	if !ok {
		if !(r.W >= 0) || !(r.H >= 0) {
			// A negative or NaN size clips everything away, as the
			// recorder's empty intersection does. Other non-finite rects
			// are ignored.
			c.addClip(d2d.RectF{}, d2d.ANTIALIAS_MODE_ALIASED)
		}
		return
	}
	radii = radii.Clamped(r)
	if radii.IsZero() && axisPreserving(c.xf) {
		c.addClip(rf, d2d.ANTIALIAS_MODE_PER_PRIMITIVE)
		return
	}
	g, ok := c.roundedGeometry(r, rf, radii)
	if !ok {
		// Without a mask, clip to the bounds rather than not at all.
		c.addClip(rf, d2d.ANTIALIAS_MODE_PER_PRIMITIVE)
		return
	}
	c.addLayer(d2d.LayerParameters1{
		ContentBounds:     d2d.InfiniteRect(),
		GeometricMask:     g.Ptr,
		MaskAntialiasMode: d2d.ANTIALIAS_MODE_PER_PRIMITIVE,
		MaskTransform:     d2d.IdentityMatrix(),
		Opacity:           1,
	})
}

// --- Drawing -----------------------------------------------------------------

// FillRoundedRect implements render.Canvas.
func (c *canvas) FillRoundedRect(r render.Rect, radii render.Radii, p render.Paint) {
	if c.rec != nil {
		c.rec.FillRoundedRect(r, radii, p)
	}
	if c.ended || p.IsSemantic() {
		return
	}
	rf, ok := rectF(r)
	if !ok || r.W == 0 || r.H == 0 {
		return
	}
	brush, ok := c.brush(p.Normalized())
	if !ok {
		return
	}
	radii = radii.Clamped(r)
	switch {
	case radii.IsZero():
		c.dc.FillRectangle(rf, brush)
	case radii.IsUniform():
		rad := float32(radii.TopLeft)
		c.dc.FillRoundedRectangle(d2d.RoundedRect{Rect: rf, RadiusX: rad, RadiusY: rad}, brush)
	default:
		if g, ok := c.roundedGeometry(r, rf, radii); ok {
			c.dc.FillGeometry(g, brush)
		}
	}
}

// StrokeRoundedRect implements render.Canvas.
func (c *canvas) StrokeRoundedRect(r render.Rect, radii render.Radii, width float64, p render.Paint) {
	if c.rec != nil {
		c.rec.StrokeRoundedRect(r, radii, width, p)
	}
	if c.ended || p.IsSemantic() {
		return
	}
	c.strokeRect(r, radii, width, p)
}

func (c *canvas) strokeRect(r render.Rect, radii render.Radii, width float64, p render.Paint) {
	w, ok := f32(render.StrokeWidth(width))
	if !ok || w == 0 {
		return
	}
	rf, ok := rectF(r)
	if !ok {
		return
	}
	brush, ok := c.brush(p.Normalized())
	if !ok {
		return
	}
	radii = radii.Clamped(r)
	if radii.IsUniform() {
		rad := float32(radii.TopLeft)
		c.dc.DrawRoundedRectangle(d2d.RoundedRect{Rect: rf, RadiusX: rad, RadiusY: rad}, brush, w, d2d.StrokeStyle{})
		return
	}
	if g, ok := c.roundedGeometry(r, rf, radii); ok {
		c.dc.DrawGeometry(g, brush, w, d2d.StrokeStyle{})
	}
}

// FillPath implements render.Canvas.
func (c *canvas) FillPath(path *render.Path, p render.Paint) {
	if c.rec != nil {
		c.rec.FillPath(path, p)
	}
	if c.ended || p.IsSemantic() {
		return
	}
	brush, ok := c.brush(p.Normalized())
	if !ok {
		return
	}
	if g, ok := c.pathGeometry(path); ok {
		c.dc.FillGeometry(g.Geometry(), brush)
	}
}

// StrokePath implements render.Canvas.
func (c *canvas) StrokePath(path *render.Path, width float64, p render.Paint, s render.StrokeStyle) {
	if c.rec != nil {
		c.rec.StrokePath(path, width, p, s)
	}
	if c.ended || p.IsSemantic() {
		return
	}
	w, ok := f32(render.StrokeWidth(width))
	if !ok || w == 0 {
		return
	}
	style, ok := c.strokeStyle(s.Normalized(), float64(w))
	if !ok {
		return
	}
	brush, ok := c.brush(p.Normalized())
	if !ok {
		return
	}
	if g, ok := c.pathGeometry(path); ok {
		c.dc.DrawGeometry(g.Geometry(), brush, w, style)
	}
}

// DrawText implements render.Canvas: the layout's native DirectWrite
// layout, rebuilt from its text, font and width (made once per layout and
// frame, released by release), drawn in a solid brush with its top-left
// corner at origin under the current transform, clips and layers, with
// grayscale antialiasing (set by newCanvas) and color fonts. nil layouts,
// layouts of another text system (or of the fake fallback), semantic and
// transparent colors, non-finite origins, empty text and layouts of a
// released text system draw nothing.
func (c *canvas) DrawText(l render.TextLayout, origin render.Point, color render.Color) {
	if c.rec != nil {
		c.rec.DrawText(l, origin, color)
	}
	if c.ended || color.IsSemantic() {
		return
	}
	dl, ok := l.(*layout)
	if !ok || dl == nil || dl.ts == nil || dl.ts != c.text || dl.ts.closed || dl.text == "" {
		return
	}
	o, ok := point2F(origin)
	if !ok {
		return
	}
	brush, ok := c.solid(color)
	if !ok {
		return
	}
	nl, ok := c.texts[dl]
	if !ok {
		var err error
		if nl, err = dl.native(); err != nil {
			c.fail(err)
			return
		}
		c.texts[dl] = nl
	}
	c.dc.DrawTextLayout(o, nl.Ptr, brush, d2d.DRAW_TEXT_OPTIONS_ENABLE_COLOR_FONT)
}

// PushOpacity implements render.Canvas: a layer with the clamped opacity
// (NaN counts as 0) and infinite bounds, closed by Pop or finish. An
// opacity of 1 only saves.
func (c *canvas) PushOpacity(alpha float64) {
	if c.rec != nil {
		c.rec.PushOpacity(alpha)
	}
	if c.ended {
		return
	}
	c.push(canvasOpacity)
	a := render.Opacity(alpha)
	if a == 1 {
		return
	}
	c.addLayer(d2d.LayerParameters1{
		ContentBounds:     d2d.InfiniteRect(),
		MaskAntialiasMode: d2d.ANTIALIAS_MODE_PER_PRIMITIVE,
		MaskTransform:     d2d.IdentityMatrix(),
		Opacity:           float32(a),
	})
}

// Pop implements render.Canvas.
func (c *canvas) Pop() {
	if c.rec != nil {
		c.rec.Pop()
	}
	if !c.ended {
		c.pop(canvasOpacity, canvasBlur)
	}
}

// --- Resources -----------------------------------------------------------------

// brush returns the frame's brush for a normalized paint; ok is false
// when the paint draws nothing or the brush can't be made.
func (c *canvas) brush(p render.Paint) (d2d.Brush, bool) {
	if p.Kind != render.PaintLinear {
		return c.solid(p.Color)
	}
	switch len(p.Stops) {
	case 0:
		return d2d.Brush{}, false
	case 1:
		return c.solid(p.Stops[0].Color)
	}
	start, ok1 := point2F(p.Start)
	end, ok2 := point2F(p.End)
	if !ok1 || !ok2 {
		return d2d.Brush{}, false
	}
	stops := make([]d2d.GradientStop, len(p.Stops))
	key := make([]byte, 0, len(p.Stops)*20)
	for i, s := range p.Stops {
		stops[i] = d2d.GradientStop{Position: float32(s.Offset), Color: colorF(s.Color)}
		for _, v := range [...]float32{stops[i].Position, stops[i].Color.R, stops[i].Color.G, stops[i].Color.B, stops[i].Color.A} {
			key = binary.LittleEndian.AppendUint32(key, math.Float32bits(v))
		}
	}
	gk := gradKey{stops: string(key), start: start, end: end}
	if b, ok := c.grads[gk]; ok {
		return b.Brush(), true
	}
	coll, ok := c.stops[gk.stops]
	if !ok {
		var err error
		coll, err = c.dc.CreateGradientStopCollection(stops, d2d.GAMMA_2_2, d2d.EXTEND_MODE_CLAMP)
		if err != nil {
			c.fail(err)
			return d2d.Brush{}, false
		}
		c.stops[gk.stops] = coll
	}
	b, err := c.dc.CreateLinearGradientBrush(d2d.LinearGradientBrushProperties{StartPoint: start, EndPoint: end},
		d2d.DefaultBrushProperties(), coll)
	if err != nil {
		c.fail(err)
		return d2d.Brush{}, false
	}
	c.grads[gk] = b
	return b.Brush(), true
}

// solid returns the frame's solid brush of color; a transparent color
// draws nothing.
func (c *canvas) solid(color render.Color) (d2d.Brush, bool) {
	cf := colorF(color)
	if cf.A == 0 {
		return d2d.Brush{}, false
	}
	if b, ok := c.solids[cf]; ok {
		return b.Brush(), true
	}
	b, err := c.dc.CreateSolidColorBrush(cf, d2d.DefaultBrushProperties())
	if err != nil {
		c.fail(err)
		return d2d.Brush{}, false
	}
	c.solids[cf] = b
	return b.Brush(), true
}

// strokeStyle returns the frame's stroke style for a normalized style at
// stroke width w (D2D's dashes and dash offset are in units of the width).
func (c *canvas) strokeStyle(s render.StrokeStyle, w float64) (d2d.StrokeStyle, bool) {
	props := d2d.StrokeStyleProperties1{DashStyle: d2d.DASH_STYLE_SOLID}
	switch s.Cap {
	case render.CapRound:
		props.StartCap = d2d.CAP_STYLE_ROUND
	case render.CapSquare:
		props.StartCap = d2d.CAP_STYLE_SQUARE
	default:
		props.StartCap = d2d.CAP_STYLE_FLAT
	}
	props.EndCap, props.DashCap = props.StartCap, props.StartCap
	switch s.Join {
	case render.JoinRound:
		props.LineJoin = d2d.LINE_JOIN_ROUND
	case render.JoinBevel:
		props.LineJoin = d2d.LINE_JOIN_BEVEL
	default:
		props.LineJoin = d2d.LINE_JOIN_MITER_OR_BEVEL // CG's miter
	}
	var ok bool
	if props.MiterLimit, ok = f32(s.MiterLimit); !ok {
		props.MiterLimit = render.DefaultMiterLimit
	}
	var dashes []float32
	key := strokeKey{cap: props.StartCap, join: props.LineJoin, miter: props.MiterLimit}
	if len(s.Dash) > 0 {
		props.DashStyle = d2d.DASH_STYLE_CUSTOM
		if props.DashOffset, ok = f32(s.DashOffset / w); !ok {
			return d2d.StrokeStyle{}, false
		}
		dashes = make([]float32, len(s.Dash))
		b := make([]byte, 0, 4*len(s.Dash))
		for i, d := range s.Dash {
			if dashes[i], ok = f32(d / w); !ok {
				return d2d.StrokeStyle{}, false
			}
			b = binary.LittleEndian.AppendUint32(b, math.Float32bits(dashes[i]))
		}
		key.dashStyled, key.offset, key.dashes = true, props.DashOffset, string(b)
	}
	if st, ok := c.strokes[key]; ok {
		return st, true
	}
	st, err := c.fac.CreateStrokeStyle(props, dashes)
	if err != nil {
		c.fail(err)
		return d2d.StrokeStyle{}, false
	}
	c.strokes[key] = st
	return st, true
}

// roundedGeometry returns a geometry of r (narrowed to rf) rounded by
// clamped radii: the D2D rounded rect for uniform radii, a path otherwise.
// It is released with the frame.
func (c *canvas) roundedGeometry(r render.Rect, rf d2d.RectF, radii render.Radii) (d2d.Geometry, bool) {
	if radii.IsUniform() {
		rad := float32(radii.TopLeft)
		g, err := c.fac.CreateRoundedRectangleGeometry(d2d.RoundedRect{Rect: rf, RadiusX: rad, RadiusY: rad})
		if err != nil {
			c.fail(err)
			return d2d.Geometry{}, false
		}
		c.geoms = append(c.geoms, owned{"ID2D1RoundedRectangleGeometry", g.Ptr})
		return g.Geometry(), true
	}
	g, ok := c.pathGeometry(roundedRectPath(r, radii))
	return g.Geometry(), ok
}

// roundedRectPath returns the outline of r with per-corner radii (already
// clamped), clockwise from the top-left corner's end.
func roundedRectPath(r render.Rect, rd render.Radii) *render.Path {
	var p render.Path
	return addRoundedRect(&p, r, rd)
}

// addRoundedRect adds roundedRectPath's closed subpath to p and returns p.
func addRoundedRect(p *render.Path, r render.Rect, rd render.Radii) *render.Path {
	x0, y0, x1, y1 := r.X, r.Y, r.MaxX(), r.MaxY()
	p.MoveTo(render.Point{X: x0 + rd.TopLeft, Y: y0})
	p.ArcTo(render.Point{X: x1, Y: y0}, render.Point{X: x1, Y: y1}, rd.TopRight)
	p.ArcTo(render.Point{X: x1, Y: y1}, render.Point{X: x0, Y: y1}, rd.BottomRight)
	p.ArcTo(render.Point{X: x0, Y: y1}, render.Point{X: x0, Y: y0}, rd.BottomLeft)
	p.ArcTo(render.Point{X: x0, Y: y0}, render.Point{X: x1, Y: y0}, rd.TopLeft)
	return p.Close()
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

// pathGeometry converts path into a path geometry (winding fill; open
// subpaths stay open for strokes and are closed implicitly by fills). ok
// is false for an empty or nil path, a point that doesn't narrow, or a
// D2D failure. The geometry is released with the frame.
func (c *canvas) pathGeometry(path *render.Path) (d2d.PathGeometry, bool) {
	return c.pathGeometryMode(path, d2d.FILL_MODE_WINDING)
}

// pathGeometryMode is pathGeometry with the fill mode mode (shadow clips
// and rings use FILL_MODE_ALTERNATE, so a nested subpath is a hole).
func (c *canvas) pathGeometryMode(path *render.Path, mode d2d.FillMode) (d2d.PathGeometry, bool) {
	if path == nil || path.Empty() {
		return d2d.PathGeometry{}, false
	}
	for s := range path.Segments() {
		for _, q := range s.Pts[:segmentPoints(s.Op)] {
			if _, ok := point2F(q); !ok {
				return d2d.PathGeometry{}, false
			}
		}
	}
	g, err := c.fac.CreatePathGeometry()
	if err != nil {
		c.fail(err)
		return d2d.PathGeometry{}, false
	}
	c.geoms = append(c.geoms, owned{"ID2D1PathGeometry", g.Ptr})
	sink, err := g.Open()
	if err != nil {
		c.fail(err)
		return d2d.PathGeometry{}, false
	}
	defer sink.Release()
	sink.SetFillMode(mode)
	open := false
	pt := func(q render.Point) d2d.Point2F { p, _ := point2F(q); return p }
	for s := range path.Segments() {
		switch s.Op {
		case render.OpMoveTo:
			if open {
				sink.EndFigure(d2d.FIGURE_END_OPEN)
			}
			sink.BeginFigure(pt(s.Pts[0]), d2d.FIGURE_BEGIN_FILLED)
			open = true
		case render.OpLineTo:
			sink.AddLine(pt(s.Pts[0]))
		case render.OpQuadTo:
			sink.AddQuadraticBezier(d2d.QuadraticBezierSegment{Point1: pt(s.Pts[0]), Point2: pt(s.Pts[1])})
		case render.OpCubicTo:
			sink.AddBezier(d2d.BezierSegment{Point1: pt(s.Pts[0]), Point2: pt(s.Pts[1]), Point3: pt(s.Pts[2])})
		case render.OpClose:
			if open {
				sink.EndFigure(d2d.FIGURE_END_CLOSED)
				open = false
			}
		}
	}
	if open {
		sink.EndFigure(d2d.FIGURE_END_OPEN)
	}
	if err := sink.Close(); err != nil {
		c.fail(err)
		return d2d.PathGeometry{}, false
	}
	return g, true
}
