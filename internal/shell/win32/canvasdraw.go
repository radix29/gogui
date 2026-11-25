//go:build windows

package win32

import (
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/win32/d2d"
)

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
		c.drawDC().FillRectangle(rf, brush)
	case radii.IsUniform():
		rad := float32(radii.TopLeft)
		c.drawDC().FillRoundedRectangle(d2d.RoundedRect{Rect: rf, RadiusX: rad, RadiusY: rad}, brush)
	default:
		if g, ok := c.roundedGeometry(r, rf, radii); ok {
			c.drawDC().FillGeometry(g, brush)
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
		c.drawDC().DrawRoundedRectangle(d2d.RoundedRect{Rect: rf, RadiusX: rad, RadiusY: rad}, brush, w, d2d.StrokeStyle{})
		return
	}
	if g, ok := c.roundedGeometry(r, rf, radii); ok {
		c.drawDC().DrawGeometry(g, brush, w, d2d.StrokeStyle{})
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
		c.drawDC().FillGeometry(g.Geometry(), brush)
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
		c.drawDC().DrawGeometry(g.Geometry(), brush, w, style)
	}
}

// DrawText implements render.Canvas: the layout's native DirectWrite
// layout, from the text system's drawn-layout cache (textcache.go; rebuilt
// from its text, font and width when it is not there), drawn in a solid brush with its top-left
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
	if !c.textFrame {
		c.textFrame = true
		c.text.nextFrame()
	}
	nl, err := c.text.drawable(dl)
	if err != nil {
		c.fail(err)
		return
	}
	c.drawDC().DrawTextLayout(o, nl.Ptr, brush, d2d.DRAW_TEXT_OPTIONS_ENABLE_COLOR_FONT)
}
