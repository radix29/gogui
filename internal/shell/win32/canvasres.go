//go:build windows

package win32

import (
	"encoding/binary"
	"math"

	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/win32/d2d"
)

// --- Resources -----------------------------------------------------------------

// brush returns the device's brush for a normalized paint (rescache.go);
// ok is false when the paint draws nothing or the brush can't be made.
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
	rc := c.res
	key := rc.key[:0]
	for _, s := range p.Stops {
		cf := colorF(s.Color)
		for _, v := range [...]float32{float32(s.Offset), cf.R, cf.G, cf.B, cf.A} {
			key = binary.LittleEndian.AppendUint32(key, math.Float32bits(v))
		}
	}
	rc.key = key
	se, ok := rc.stops[string(key)]
	if ok {
		if ge, ok := rc.grads[gradKey{stops: se, start: start, end: end}]; ok {
			hit(rc, se)
			return hit(rc, ge).Brush(), true
		}
	} else {
		stops := rc.stopBuf[:0]
		for _, s := range p.Stops {
			stops = append(stops, d2d.GradientStop{Position: float32(s.Offset), Color: colorF(s.Color)})
		}
		rc.stopBuf = stops
		coll, err := c.dc.CreateGradientStopCollection(stops, d2d.GAMMA_2_2, d2d.EXTEND_MODE_CLAMP)
		if err != nil {
			c.fail(err)
			return d2d.Brush{}, false
		}
		se = add(rc, rc.stops, string(key), coll)
	}
	hit(rc, se)
	b, err := c.dc.CreateLinearGradientBrush(d2d.LinearGradientBrushProperties{StartPoint: start, EndPoint: end},
		d2d.DefaultBrushProperties(), se.r)
	if err != nil {
		c.fail(err)
		return d2d.Brush{}, false
	}
	ge := add(rc, rc.grads, gradKey{stops: se, start: start, end: end}, b)
	ge.stops = se
	se.refs++
	return b.Brush(), true
}

// solid returns the device's solid brush of color; a transparent color
// draws nothing.
func (c *canvas) solid(color render.Color) (d2d.Brush, bool) {
	cf := colorF(color)
	if cf.A == 0 {
		return d2d.Brush{}, false
	}
	rc := c.res
	if e, ok := rc.solids[cf]; ok {
		return hit(rc, e).Brush(), true
	}
	b, err := c.dc.CreateSolidColorBrush(cf, d2d.DefaultBrushProperties())
	if err != nil {
		c.fail(err)
		return d2d.Brush{}, false
	}
	add(rc, rc.solids, cf, b)
	return b.Brush(), true
}

// strokeStyle returns the device's stroke style for a normalized style at
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
	rc := c.res
	key := strokeKey{cap: props.StartCap, join: props.LineJoin, miter: props.MiterLimit}
	if len(s.Dash) > 0 {
		props.DashStyle = d2d.DASH_STYLE_CUSTOM
		if props.DashOffset, ok = f32(s.DashOffset / w); !ok {
			return d2d.StrokeStyle{}, false
		}
		b := rc.key[:0]
		for _, d := range s.Dash {
			v, ok := f32(d / w)
			if !ok {
				return d2d.StrokeStyle{}, false
			}
			b = binary.LittleEndian.AppendUint32(b, math.Float32bits(v))
		}
		rc.key = b
		dk, ok := rc.dashes[string(b)]
		if !ok {
			dk = string(b)
			rc.dashes[dk] = dk
		}
		key.dashStyled, key.offset, key.dashes = true, props.DashOffset, dk
	}
	if e, ok := rc.strokes[key]; ok {
		return hit(rc, e), true
	}
	var dashes []float32
	if key.dashStyled {
		dashes = rc.dashBuf[:0]
		for i := 0; i+4 <= len(key.dashes); i += 4 {
			dashes = append(dashes, math.Float32frombits(binary.LittleEndian.Uint32([]byte(key.dashes[i:i+4]))))
		}
		rc.dashBuf = dashes
	}
	st, err := c.fac.CreateStrokeStyle(props, dashes)
	if err != nil {
		c.fail(err)
		return d2d.StrokeStyle{}, false
	}
	add(rc, rc.strokes, key, st)
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

// --- Narrowing -------------------------------------------------------------

// f32 narrows v; ok is false when v or the result is not finite.
func f32(v float64) (float32, bool) {
	f := float32(v)
	return f, !math.IsNaN(v) && !math.IsInf(float64(f), 0)
}

// point2F narrows p; ok is false when p is not finite (render.ValidPoint)
// or does not narrow to a finite float32.
func point2F(p render.Point) (d2d.Point2F, bool) {
	x, okx := f32(p.X)
	y, oky := f32(p.Y)
	return d2d.Point2F{X: x, Y: y}, render.ValidPoint(p) && okx && oky
}

// rectF narrows r; ok is false for an invalid rect (render.ValidRect: not
// finite or a negative size) and for one that does not narrow.
func rectF(r render.Rect) (d2d.RectF, bool) {
	if !render.ValidRect(r) {
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
