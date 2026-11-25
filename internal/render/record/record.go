// Package record is a render.Canvas that records what is drawn, for tests
// (PLAN.md S2.1b). It is the backend of the fake shell (shell/fake) and of the
// aqua element tests.
//
// Every drawing call becomes an Op with its values normalized by the render
// validation rules and the canvas state resolved: the current transform, the
// clip as a device-space bounding box, and the effective opacity (the product
// of the open opacity layers). Save, Restore, the transform calls and
// ClipRoundedRect leave no op of their own; they only show in the state of
// later ops. Layers do: PushOpacity, PushBackdropBlur and Pop are ops, because
// group opacity and backdrop blur are not per-op properties.
//
// The canvas checks the Canvas contract and collects every violation as an
// error instead of panicking: Restore without a matching Save, Pop without a
// matching Push*, pairs still open at End, semantic colors, nil images and
// layouts, non-finite transform arguments, invalid geometry (a rect that is
// not finite or has a negative size, a non-finite point: render.ValidRect,
// ValidPoint, ValidPath), and calls after End. An offending
// call is ignored (nothing is recorded), as a real backend would. End returns
// all errors joined.
//
// Dump prints the ops as stable text, one line per op, for assertions:
//
//	fillRRect (0,0,80,22) r=5 solid #007aff
//
// The format (rounding, translations folded into the geometry, optional
// fields) is described at the top of dump.go.
package record

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/radix29/gogui/internal/render"
)

type rect = render.Rect

// Kind is the kind of a recorded op.
type Kind uint8

// Op kinds, one per drawing or layer call of render.Canvas.
const (
	FillRoundedRect Kind = iota
	StrokeRoundedRect
	FillPath
	StrokePath
	DrawShadow
	DrawText
	DrawImage
	PushOpacity
	PushBackdropBlur
	Pop
)

// String returns the name used in dumps.
func (k Kind) String() string {
	switch k {
	case FillRoundedRect:
		return "fillRRect"
	case StrokeRoundedRect:
		return "strokeRRect"
	case FillPath:
		return "fillPath"
	case StrokePath:
		return "strokePath"
	case DrawShadow:
		return "shadow"
	case DrawText:
		return "text"
	case DrawImage:
		return "image"
	case PushOpacity:
		return "pushOpacity"
	case PushBackdropBlur:
		return "pushBlur"
	case Pop:
		return "pop"
	}
	return "unknown"
}

// Op is one recorded call. Geometry fields are in user space, as passed (and
// normalized); Transform maps them to device space (logical points of the
// surface). Fields that do not apply to Kind are zero.
type Op struct {
	Kind Kind

	// Rect is the shape (FillRoundedRect, StrokeRoundedRect, DrawShadow,
	// PushBackdropBlur), the destination (DrawImage) or the layout's box at
	// its origin (DrawText).
	Rect  render.Rect
	Radii render.Radii // clamped to Rect

	Paint  render.Paint       // normalized; fills and strokes
	Width  float64            // stroke width (render.StrokeWidth)
	Stroke render.StrokeStyle // normalized; StrokePath
	Path   []render.Segment   // a copy of the path; FillPath, StrokePath
	Shadow render.Shadow      // normalized; DrawShadow

	Layout render.TextLayout // DrawText
	Color  render.Color      // DrawText
	Image  render.Image      // DrawImage
	Alpha  float64           // PushOpacity (render.Opacity)
	Blur   float64           // PushBackdropBlur radius (render.BlurRadius)

	// The canvas state at the call. For PushOpacity and PushBackdropBlur it
	// is the state outside the new layer; for Pop the state it restores.
	Transform render.Transform
	Clip      render.Rect // device-space bounding box of the clip
	Opacity   float64     // effective opacity of the enclosing layers
}

// Bounds returns the device-space bounding box of the op's geometry (the
// tight path bounds for paths, Rect otherwise), ignoring stroke width, shadow
// offset and blur, and the clip.
func (o Op) Bounds() render.Rect {
	if o.Kind == FillPath || o.Kind == StrokePath {
		return o.Transform.ApplyRect(render.SegmentsBounds(o.Path))
	}
	return o.Transform.ApplyRect(o.Rect)
}

// frameKind says which call pushed a state frame.
type frameKind uint8

const (
	frameSave frameKind = iota
	frameOpacity
	frameBlur
)

func (k frameKind) String() string {
	switch k {
	case frameOpacity:
		return "PushOpacity"
	case frameBlur:
		return "PushBackdropBlur"
	}
	return "Save"
}

type state struct {
	xf      render.Transform
	clip    render.Rect
	opacity float64
}

type frame struct {
	saved state
	kind  frameKind
	call  int // call number of the push, for error messages
}

// Canvas is a recording render.Canvas. Create one with New; the zero value is
// not usable. Like every canvas it is used from one goroutine.
type Canvas struct {
	surface render.Rect
	cur     state
	stack   []frame
	ops     []Op
	segs    []render.Segment      // the arena of the ops' paths
	stops   []render.GradientStop // the arena of their gradient stops
	dash    []float64             // the arena of their dash patterns
	errs    []error
	calls   int
	ended   bool
}

var _ render.Canvas = (*Canvas)(nil)

// New returns a canvas for a surface of the given size in logical points:
// identity transform, the whole surface as clip, opacity 1.
func New(size render.Size) *Canvas {
	s := rect{W: size.W, H: size.H}
	return &Canvas{surface: s, cur: state{xf: render.Identity(), clip: s, opacity: 1}}
}

// Ops returns a copy of the recorded ops, in call order.
func (c *Canvas) Ops() []Op { return slices.Clone(c.ops) }

// Errors returns the contract violations found so far.
func (c *Canvas) Errors() []error { return slices.Clone(c.errs) }

// Depth returns the number of open Save and Push* calls.
func (c *Canvas) Depth() int { return len(c.stack) }

// End finishes the recording: every Save and Push* still open is an error,
// and later calls are errors. It returns all errors found (nil if none),
// joined. Calling End again returns an error.
func (c *Canvas) End() error {
	if c.ended {
		return errors.New("record: End called twice")
	}
	c.calls++
	for i := len(c.stack) - 1; i >= 0; i-- {
		f := c.stack[i]
		c.fail("End", fmt.Sprintf("%s of call %d not closed", f.kind, f.call))
	}
	c.ended = true
	return errors.Join(c.errs...)
}

// fail records a violation of call number c.calls.
func (c *Canvas) fail(name, msg string) {
	c.errs = append(c.errs, fmt.Errorf("record: call %d (%s): %s", c.calls, name, msg))
}

// begin counts a call and reports whether it may proceed.
func (c *Canvas) begin(name string) bool {
	c.calls++
	if c.ended {
		c.fail(name, "call after End")
		return false
	}
	return true
}

func (c *Canvas) semantic(name string, semantic bool) bool {
	if semantic {
		c.fail(name, "semantic color reached the canvas (resolve it with the theme first)")
	}
	return semantic
}

func (c *Canvas) record(o Op) {
	o.Transform, o.Clip, o.Opacity = c.cur.xf, c.cur.clip, c.cur.opacity
	c.ops = append(c.ops, o)
}

func finite(vs ...float64) bool {
	for _, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// Save implements render.Canvas.
func (c *Canvas) Save() {
	if c.begin("Save") {
		c.stack = append(c.stack, frame{saved: c.cur, kind: frameSave, call: c.calls})
	}
}

// Restore implements render.Canvas.
func (c *Canvas) Restore() {
	if c.begin("Restore") {
		c.close("Restore", frameSave)
	}
}

// Pop implements render.Canvas.
func (c *Canvas) Pop() {
	if c.begin("Pop") && c.close("Pop", frameOpacity, frameBlur) {
		c.record(Op{Kind: Pop})
	}
}

// close pops the innermost frame if it has one of the kinds, and reports
// whether it did.
func (c *Canvas) close(name string, kinds ...frameKind) bool {
	if len(c.stack) == 0 {
		c.fail(name, "nothing open")
		return false
	}
	top := c.stack[len(c.stack)-1]
	if !slices.Contains(kinds, top.kind) {
		c.fail(name, fmt.Sprintf("innermost open call is %s of call %d", top.kind, top.call))
		return false
	}
	c.stack = c.stack[:len(c.stack)-1]
	c.cur = top.saved
	return true
}

// Translate implements render.Canvas.
func (c *Canvas) Translate(dx, dy float64) {
	if !c.begin("Translate") {
		return
	}
	if !finite(dx, dy) {
		c.fail("Translate", "non-finite argument")
		return
	}
	c.cur.xf = c.cur.xf.Pre(render.Translation(dx, dy))
}

// Scale implements render.Canvas.
func (c *Canvas) Scale(sx, sy float64) {
	if !c.begin("Scale") {
		return
	}
	if !finite(sx, sy) {
		c.fail("Scale", "non-finite argument")
		return
	}
	c.cur.xf = c.cur.xf.Pre(render.Scaling(sx, sy))
}

// Concat implements render.Canvas.
func (c *Canvas) Concat(t render.Transform) {
	if !c.begin("Concat") {
		return
	}
	if !finite(t.A, t.B, t.C, t.D, t.E, t.F) {
		c.fail("Concat", "non-finite transform")
		return
	}
	c.cur.xf = c.cur.xf.Pre(t)
}

// ClipRoundedRect implements render.Canvas. The recorder tracks the clip as
// a device-space bounding box; the radii only show in the box's corners not
// being cut. An invalid rect (render.ValidRect) is reported and clips
// everything away, as on the backends.
func (c *Canvas) ClipRoundedRect(r render.Rect, _ render.Radii) {
	if c.begin("ClipRoundedRect") {
		c.clip("ClipRoundedRect", r)
	}
}

func (c *Canvas) clip(name string, r render.Rect) {
	if !c.validRect(name, r) {
		c.cur.clip = rect{}
		return
	}
	c.cur.clip = c.cur.clip.Intersect(c.cur.xf.ApplyRect(r))
}

// validRect reports whether r is a valid rect, and reports it if not.
func (c *Canvas) validRect(name string, r render.Rect) bool {
	if render.ValidRect(r) {
		return true
	}
	c.fail(name, fmt.Sprintf("invalid rect %v (not finite or a negative size)", r))
	return false
}

// validPath reports whether every point of p is finite, and reports it if
// not.
func (c *Canvas) validPath(name string, p *render.Path) bool {
	if render.ValidPath(p) {
		return true
	}
	c.fail(name, "path with a non-finite point")
	return false
}

// FillRoundedRect implements render.Canvas.
func (c *Canvas) FillRoundedRect(r render.Rect, radii render.Radii, p render.Paint) {
	const name = "FillRoundedRect"
	if c.begin(name) && !c.semantic(name, p.IsSemantic()) && c.validRect(name, r) {
		c.record(Op{Kind: FillRoundedRect, Rect: r, Radii: radii.Clamped(r), Paint: c.paint(p)})
	}
}

// StrokeRoundedRect implements render.Canvas.
func (c *Canvas) StrokeRoundedRect(r render.Rect, radii render.Radii, width float64, p render.Paint) {
	const name = "StrokeRoundedRect"
	if c.begin(name) && !c.semantic(name, p.IsSemantic()) && c.validRect(name, r) {
		c.record(Op{Kind: StrokeRoundedRect, Rect: r, Radii: radii.Clamped(r), Width: render.StrokeWidth(width), Paint: c.paint(p)})
	}
}

// segments copies the segments of p into the canvas's segment arena (one
// growing slice for every path of the recording, so a path costs no
// allocation of its own) and returns them, capped so that the slice of one
// op never grows into the next.
func (c *Canvas) segments(p *render.Path) []render.Segment {
	if p == nil || p.Empty() {
		return nil
	}
	i := len(c.segs)
	c.segs = slices.AppendSeq(c.segs, p.Segments())
	return c.segs[i:len(c.segs):len(c.segs)]
}

// paint returns p normalized, its stops copied into the canvas's stop
// arena: Normalized may return the caller's slice, which a recording must
// not keep.
func (c *Canvas) paint(p render.Paint) render.Paint {
	p = p.Normalized()
	if len(p.Stops) > 0 {
		i := len(c.stops)
		c.stops = append(c.stops, p.Stops...)
		p.Stops = c.stops[i:len(c.stops):len(c.stops)]
	}
	return p
}

// stroke returns s normalized, its dash pattern copied into the canvas's
// dash arena (as paint does for stops).
func (c *Canvas) stroke(s render.StrokeStyle) render.StrokeStyle {
	s = s.Normalized()
	if len(s.Dash) > 0 {
		i := len(c.dash)
		c.dash = append(c.dash, s.Dash...)
		s.Dash = c.dash[i:len(c.dash):len(c.dash)]
	}
	return s
}

// FillPath implements render.Canvas.
func (c *Canvas) FillPath(path *render.Path, p render.Paint) {
	const name = "FillPath"
	if c.begin(name) && !c.semantic(name, p.IsSemantic()) && c.validPath(name, path) {
		c.record(Op{Kind: FillPath, Path: c.segments(path), Paint: c.paint(p)})
	}
}

// StrokePath implements render.Canvas.
func (c *Canvas) StrokePath(path *render.Path, width float64, p render.Paint, s render.StrokeStyle) {
	const name = "StrokePath"
	if c.begin(name) && !c.semantic(name, p.IsSemantic()) && c.validPath(name, path) {
		c.record(Op{Kind: StrokePath, Path: c.segments(path), Width: render.StrokeWidth(width), Paint: c.paint(p), Stroke: c.stroke(s)})
	}
}

// DrawShadow implements render.Canvas.
func (c *Canvas) DrawShadow(r render.Rect, radii render.Radii, s render.Shadow) {
	const name = "DrawShadow"
	if c.begin(name) && !c.semantic(name, s.IsSemantic()) && c.validRect(name, r) {
		c.record(Op{Kind: DrawShadow, Rect: r, Radii: radii.Clamped(r), Shadow: s.Normalized()})
	}
}

// DrawText implements render.Canvas.
func (c *Canvas) DrawText(l render.TextLayout, origin render.Point, color render.Color) {
	if !c.begin("DrawText") || c.semantic("DrawText", color.IsSemantic()) {
		return
	}
	if l == nil {
		c.fail("DrawText", "nil layout")
		return
	}
	if !render.ValidPoint(origin) {
		c.fail("DrawText", fmt.Sprintf("non-finite origin %v", origin))
		return
	}
	sz := l.Size()
	c.record(Op{Kind: DrawText, Rect: rect{X: origin.X, Y: origin.Y, W: sz.W, H: sz.H}, Layout: l, Color: color})
}

// DrawImage implements render.Canvas.
func (c *Canvas) DrawImage(img render.Image, dst render.Rect) {
	if !c.begin("DrawImage") {
		return
	}
	if img == nil {
		c.fail("DrawImage", "nil image")
		return
	}
	if c.validRect("DrawImage", dst) {
		c.record(Op{Kind: DrawImage, Rect: dst, Image: img})
	}
}

// PushOpacity implements render.Canvas.
func (c *Canvas) PushOpacity(alpha float64) {
	if !c.begin("PushOpacity") {
		return
	}
	a := render.Opacity(alpha)
	c.record(Op{Kind: PushOpacity, Alpha: a})
	c.stack = append(c.stack, frame{saved: c.cur, kind: frameOpacity, call: c.calls})
	c.cur.opacity *= a
}

// PushBackdropBlur implements render.Canvas. An invalid rect is reported;
// the layer still opens (its Pop must match), recorded with an empty rect,
// and clips everything away.
func (c *Canvas) PushBackdropBlur(r render.Rect, radii render.Radii, radius float64) {
	if !c.begin("PushBackdropBlur") {
		return
	}
	o := Op{Kind: PushBackdropBlur, Blur: render.BlurRadius(radius)}
	if render.ValidRect(r) {
		o.Rect, o.Radii = r, radii.Clamped(r)
	}
	c.record(o)
	c.stack = append(c.stack, frame{saved: c.cur, kind: frameBlur, call: c.calls})
	c.clip("PushBackdropBlur", r)
}

// Dump returns the recorded ops as text, one Op.String per line, joined by
// "\n" without a trailing newline. An empty recording dumps as "".
func (c *Canvas) Dump() string { return strings.Join(c.Lines(), "\n") }

// Lines returns the dump as one string per op.
func (c *Canvas) Lines() []string {
	lines := make([]string, len(c.ops))
	for i, o := range c.ops {
		lines[i] = o.format(c.surface)
	}
	return lines
}

// String formats the op as in a dump of a canvas whose surface is the op's
// clip (so the clip is not printed).
func (o Op) String() string { return o.format(o.Clip) }
