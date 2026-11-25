package record

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
)

// The dump format, one line per op:
//
//	<kind> <geometry> <values> [xf=(a,b,c,d,e,f)] [clip=(x,y,w,h)] [alpha=a]
//
//   - Numbers are rounded to 3 decimals with trailing zeros dropped, and -0
//     prints as 0, so float noise does not change a dump.
//   - When the transform is a pure translation it is applied to the printed
//     geometry (rects, path points, gradient end points) and not printed;
//     otherwise the geometry is printed in user space followed by xf=(…).
//   - Rects print as (x,y,w,h), points as (x,y). Radii print as r=5 when
//     uniform, r=tl/tr/br/bl otherwise, and not at all when zero.
//   - Colors print as #rrggbb, or #rrggbbaa when not opaque.
//   - Paints print as "solid #rrggbb" or
//     "linear (x0,y0)->(x1,y1) [offset:#color …]".
//   - Optional values print only when not the default: stroke cap, join,
//     miter limit and dash; shadow spread and inset; font family, weight,
//     italic and monospaced.
//   - clip=(…) prints when the clip (a device-space bounding box) is not the
//     whole surface, alpha= when the effective opacity of the enclosing layers
//     is not 1.
//
// Examples:
//
//	fillRRect (0,0,80,22) r=5 solid #007aff
//	strokeRRect (0.5,0.5,79,21) r=4.5 w=1 solid #00000026
//	fillPath [M(0,0) L(10,0) L(5,8) Z] solid #ffffff
//	strokePath [M(2,6) L(5,9) L(10,3)] w=2 cap=round join=round solid #ffffff
//	shadow (0,0,80,22) r=5 off=(0,0.5) blur=1 #0000004d
//	text (10,3,35,16) "Greet" 13pt #000000d9
//	image (4,4,16,16) 16x16
//	pushOpacity 0.5
//	pushBlur (0,0,200,300) r=10 blur=30
//	pop

// num formats v rounded to 3 decimals.
func num(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	v = math.Round(v*1000) / 1000
	if v == 0 {
		v = 0 // -0
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func fmtPoint(p render.Point) string { return "(" + num(p.X) + "," + num(p.Y) + ")" }

func fmtRect(r render.Rect) string {
	return "(" + num(r.X) + "," + num(r.Y) + "," + num(r.W) + "," + num(r.H) + ")"
}

func fmtRadii(rd render.Radii) string {
	switch {
	case rd.IsZero():
		return ""
	case rd.IsUniform():
		return " r=" + num(rd.TopLeft)
	}
	return " r=" + num(rd.TopLeft) + "/" + num(rd.TopRight) + "/" + num(rd.BottomRight) + "/" + num(rd.BottomLeft)
}

func channel(v float32) int {
	c := math.Round(float64(v) * 255)
	if !(c > 0) {
		return 0
	}
	return int(math.Min(c, 255))
}

func fmtColor(c driver.Color) string {
	s := fmt.Sprintf("#%02x%02x%02x", channel(c.R), channel(c.G), channel(c.B))
	if a := channel(c.A); a != 255 {
		s += fmt.Sprintf("%02x", a)
	}
	return s
}

// printer formats geometry in user space or, for a translation, in device
// space.
type printer struct {
	xf    render.Transform
	apply bool // xf is a translation applied to the geometry
}

func (p printer) point(q render.Point) string {
	if p.apply {
		q = p.xf.Apply(q)
	}
	return fmtPoint(q)
}

func (p printer) rect(r render.Rect) string {
	if p.apply {
		r = p.xf.ApplyRect(r)
	}
	return fmtRect(r)
}

func (p printer) paint(pt render.Paint) string {
	if pt.Kind != render.PaintLinear {
		return "solid " + fmtColor(pt.Color)
	}
	var b strings.Builder
	b.WriteString("linear " + p.point(pt.Start) + "->" + p.point(pt.End) + " [")
	for i, s := range pt.Stops {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(num(s.Offset) + ":" + fmtColor(s.Color))
	}
	b.WriteByte(']')
	return b.String()
}

func (p printer) path(segs []render.Segment) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, s := range segs {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(s.Op.String())
		for j := range s.Op.Points() {
			if j > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(p.point(s.Pts[j]))
		}
	}
	b.WriteByte(']')
	return b.String()
}

func fmtStroke(s render.StrokeStyle) string {
	var b strings.Builder
	switch s.Cap {
	case render.CapRound:
		b.WriteString(" cap=round")
	case render.CapSquare:
		b.WriteString(" cap=square")
	}
	switch s.Join {
	case render.JoinRound:
		b.WriteString(" join=round")
	case render.JoinBevel:
		b.WriteString(" join=bevel")
	}
	if s.MiterLimit != render.DefaultMiterLimit {
		b.WriteString(" miter=" + num(s.MiterLimit))
	}
	if len(s.Dash) > 0 {
		b.WriteString(" dash=[")
		for i, d := range s.Dash {
			if i > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(num(d))
		}
		b.WriteByte(']')
		if s.DashOffset != 0 {
			b.WriteString("@" + num(s.DashOffset))
		}
	}
	return b.String()
}

func fmtShadow(s render.Shadow) string {
	out := " off=" + fmtPoint(s.Offset) + " blur=" + num(s.Blur)
	if s.Spread != 0 {
		out += " spread=" + num(s.Spread)
	}
	out += " " + fmtColor(s.Color)
	if s.Inset {
		out += " inset"
	}
	return out
}

func fmtFont(f render.Font) string {
	out := num(f.EffectiveSize(driver.ControlSizeRegular)) + "pt"
	if f.Family != "" {
		out += " " + strconv.Quote(f.Family)
	} else if f.Monospaced {
		out += " mono"
	}
	if w := f.EffectiveWeight(); w != driver.FontWeightRegular {
		out += " w" + strconv.Itoa(int(w))
	}
	if f.Italic {
		out += " italic"
	}
	return out
}

func (o Op) format(surface render.Rect) string {
	p := printer{xf: o.Transform, apply: o.Transform.IsTranslation()}
	var b strings.Builder
	b.WriteString(o.Kind.String())
	switch o.Kind {
	case FillRoundedRect:
		b.WriteString(" " + p.rect(o.Rect) + fmtRadii(o.Radii) + " " + p.paint(o.Paint))
	case StrokeRoundedRect:
		b.WriteString(" " + p.rect(o.Rect) + fmtRadii(o.Radii) + " w=" + num(o.Width) + " " + p.paint(o.Paint))
	case FillPath:
		b.WriteString(" " + p.path(o.Path) + " " + p.paint(o.Paint))
	case StrokePath:
		b.WriteString(" " + p.path(o.Path) + " w=" + num(o.Width) + fmtStroke(o.Stroke) + " " + p.paint(o.Paint))
	case DrawShadow:
		b.WriteString(" " + p.rect(o.Rect) + fmtRadii(o.Radii) + fmtShadow(o.Shadow))
	case DrawText:
		b.WriteString(" " + p.rect(o.Rect))
		if o.Layout != nil {
			b.WriteString(" " + strconv.Quote(o.Layout.Text()) + " " + fmtFont(o.Layout.Font()))
		}
		b.WriteString(" " + fmtColor(o.Color))
	case DrawImage:
		b.WriteString(" " + p.rect(o.Rect))
		if o.Image != nil {
			sz := o.Image.Size()
			b.WriteString(" " + num(sz.W) + "x" + num(sz.H))
		}
	case PushOpacity:
		b.WriteString(" " + num(o.Alpha))
	case PushBackdropBlur:
		b.WriteString(" " + p.rect(o.Rect) + fmtRadii(o.Radii) + " blur=" + num(o.Blur))
	}
	if !p.apply && o.Kind != Pop && o.Kind != PushOpacity {
		t := o.Transform
		b.WriteString(" xf=(" + num(t.A) + "," + num(t.B) + "," + num(t.C) + "," + num(t.D) + "," + num(t.E) + "," + num(t.F) + ")")
	}
	if o.Clip != surface {
		b.WriteString(" clip=" + fmtRect(o.Clip))
	}
	if o.Opacity != 1 {
		b.WriteString(" alpha=" + num(o.Opacity))
	}
	return b.String()
}
