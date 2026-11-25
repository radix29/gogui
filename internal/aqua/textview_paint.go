package aqua

import (
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
)

// The TextView's painting (see TextView): the background and the focus
// ring by its element, the visible paragraphs, the selection and the caret
// by its document.

// Paint settles the layout (a fallback: every change settles it already),
// then fills the frame with ColorTextBackground and draws the focus ring
// around it; the text is the textDoc's.
func (e *textViewElem) Paint(c render.Canvas) {
	if e.Bounds().Empty() {
		return
	}
	e.p.settle()
	r := pixelAlign(&e.Base, e.Bounds(), scaleIn(&e.Base))
	c.FillRoundedRect(r, render.Radii{}, render.Solid(semantic(driver.ColorTextBackground, appearanceIn(&e.Base, e.d))))
	e.ring.Paint(c, e, RectFocusShape(r, render.Radii{}))
}

func (e *textDoc) Paint(c render.Canvas) { e.p.paintText(c) }

// paintText draws the visible paragraphs, the selection and the caret
// (see TextView) in the textDoc's coordinates.
func (t *TextView) paintText(c render.Canvas) {
	top, bottom, ok := t.visible()
	if !ok {
		return
	}
	v := t.view
	a := appearanceIn(&v.Base, v.d)
	inY, pad := tokens.TextView.ContainerInsetY, tokens.TextView.LineFragmentPadding
	focus := v.FocusState()
	buf := v.ed.Buffer()
	start, end := v.ed.Selection()
	selCol := render.Solid(tokens.TextSelection(a, focus != Unfocused, focus == FocusedActive))
	col := t.textColor(a)
	right := t.lines.Width() // the container's right edge, from the text's left
	if right == 0 {
		right = max(0, t.sv.ViewportSize().W-2*pad)
	}
	idx := t.lines.Index()
	for i := idx.At(max(top, 0)); i < idx.Len(); i++ {
		y := idx.Y(i)
		if y >= bottom {
			break
		}
		l := t.lines.Shown(i)
		o := driver.Point{X: pad, Y: inY + y}
		ps, pe := buf.ParagraphStart(i), buf.ParagraphEnd(i)
		if start < end && start <= pe && end > ps {
			for _, r := range l.SelectionRects(t.lines.ShownOffset(i, max(start, ps)-ps), t.lines.ShownOffset(i, min(end, pe)-ps)) {
				c.FillRoundedRect(r.Offset(o), render.Radii{}, selCol)
			}
			if end > pe { // the newline is selected: to the container's edge
				m := l.Line(l.LineCount() - 1)
				if x := l.CaretRect(t.lines.ShownOffset(i, pe-ps)).X; right > x {
					c.FillRoundedRect(driver.Rect{X: o.X + x, Y: o.Y + m.Top, W: right - x, H: m.Height}, render.Radii{}, selCol)
				}
			}
		}
		composed := t.lines.composes(i)
		if ps < pe || composed {
			// ColorSelectedText is ColorText in the theme, so the selected
			// text needs no second pass.
			drawParagraph(c, l, o, col, inY+top, inY+bottom) // a long one: only its chunks in view
		}
		if composed {
			_, at, _, _ := t.lines.Preedit()
			paintPreeditUnderlines(c, &t.doc.Base, l, o, at, v.pre, col)
		}
	}
	if v.CaretVisible() {
		i := buf.ParagraphIndex(v.ed.Head())
		if y := idx.Y(i); y < bottom && y+idx.Height(i) > top {
			c.FillRoundedRect(pixelAlign(&t.doc.Base, t.caretRect(), scaleIn(&t.doc.Base)), render.Radii{},
				render.Solid(tokens.Caret(a)))
		}
	}
}
