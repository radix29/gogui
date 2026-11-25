package aqua

import (
	"sort"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
)

// The TextView's layout (see TextView): settling the paragraph layouts
// on the frame, the viewport and the offset, the idle layout pass, scroll
// anchoring and scrolling ranges and the caret into view.

// wrapWidth returns the wrap width for a viewport w wide: w minus the line
// padding on both sides (at least 1 pt), or no wrapping (0) without a
// viewport.
func (t *TextView) wrapWidth(w float64) float64 {
	if !(w > 0) {
		return 0
	}
	return wrapWidth(max(w-2*tokens.TextView.LineFragmentPadding, 1))
}

// settle brings the layout in line with the frame, the viewport and the
// offset (see TextView): the wrap width (scroll-anchored), the page, the
// layouts of the visible paragraphs and the document's size, repeated
// while a layout or the size changes (a shorter document can clamp the
// offset and show other paragraphs). It runs after every place of the
// internal scroll view; the changes it makes itself do not re-enter it.
func (t *TextView) settle() {
	if t.settling {
		return
	}
	vp := t.sv.ViewportSize()
	if t.lines == nil {
		if !(vp.W > 0 && vp.H > 0) {
			return // nothing shows: no layout yet
		}
		t.lines = newParaLayouts(t.view.d.text, t.view.ed.Buffer(), t.TextFont(), 0)
	}
	t.settling = true
	defer func() { t.settling = false }()
	if w := t.wrapWidth(vp.W); w != t.lines.Width() {
		a := t.anchor()
		t.lines.SetWidth(w)
		t.restore(a)
	}
	t.syncPreedit()
	t.lines.SetViewport(vp.H)
	for range 16 {
		made := t.lines.Made()
		t.layoutVisible()
		if !t.syncDocument() && t.lines.Made() == made {
			break
		}
	}
	t.scheduleIdle()
}

// syncPreedit brings the composition spliced into the paragraph layouts
// in line with the editor's (the preedit at the head), keeping the visual
// line at the top of the viewport in place when a height changes. It runs
// inside settle.
func (t *TextView) syncPreedit() {
	v := t.view
	para, at, s, ok := t.lines.Preedit()
	if !v.composing() {
		if ok {
			a := t.anchor()
			t.lines.ClearPreedit()
			t.restore(a)
		}
		return
	}
	buf := v.ed.Buffer()
	head := v.ed.Head()
	i := buf.ParagraphIndex(head)
	if ok && para == i && at == head-buf.ParagraphStart(i) && s == v.pre.text {
		return
	}
	a := t.anchor()
	t.lines.SetPreedit(i, head-buf.ParagraphStart(i), v.pre.text)
	t.restore(a)
}

// shown reports whether the view shows: in an open window and not hidden.
func (t *TextView) shown() bool {
	r, ok := t.view.Root().(*windowRoot)
	return ok && !r.w.Closed() && t.view.VisibleInTree()
}

// scheduleIdle starts the idle layout pass's timer (see TextView) unless
// one is pending, every height is exact or the view does not show.
func (t *TextView) scheduleIdle() {
	if t.idle != nil || t.lines == nil || t.lines.Index().Inexact() == 0 || !t.shown() {
		return
	}
	t.idle = t.view.d.sh.After(idleLayoutDelay, t.idleTick)
}

// idleTick is a tick of the idle layout pass (see TextView): it measures
// the next estimated paragraphs, keeps the top visual line in place and
// settles, which starts the next tick while estimates are left.
func (t *TextView) idleTick() {
	t.idle = nil
	if t.lines == nil || !t.shown() {
		return
	}
	if t.sv.y.Moving() || t.sv.y.Overscroll() != 0 || t.sv.pressed() {
		t.scheduleIdle() // the user scrolls: an anchor now would fight the motion
		return
	}
	idx, buf := t.lines.Index(), t.view.ed.Buffer()
	now := t.idleNow
	if now == nil {
		now = t.view.d.clock.Now
	}
	stop := now() + idleLayoutTime
	t.settling = true
	a := t.anchor()
	for budget := idleLayoutBytes; budget > 0; {
		i := idx.NextInexact(t.idleFrom)
		if i < 0 {
			if i = idx.NextInexact(0); i < 0 {
				break
			}
		}
		t.lines.Measure(i)
		budget -= buf.ParagraphEnd(i) - buf.ParagraphStart(i) + idleLayoutParaCost
		t.idleFrom = i + 1
		if now() >= stop {
			break
		}
	}
	t.restore(a)
	t.settling = false
	t.settle()
}

// IdleLayoutTick runs one tick of the idle layout pass now, in place of
// the pending timer's, and returns the number of estimated heights left
// (benchmarks, PLAN.md S5.4k).
func (t *TextView) IdleLayoutTick() int {
	if t.idle != nil {
		t.idle()
		t.idle = nil
	}
	t.idleTick()
	if t.lines == nil {
		return 0
	}
	return t.lines.Index().Inexact()
}

// visible returns the viewport's vertical extent in the paragraph index's
// coordinates (the document's minus ContainerInsetY), by the offset as
// shown (overscroll included), and whether the viewport has an area.
func (t *TextView) visible() (top, bottom float64, ok bool) {
	vp := t.sv.ViewportSize()
	top = t.sv.y.Offset() - tokens.TextView.ContainerInsetY
	return top, top + vp.H, vp.W > 0 && vp.H > 0
}

// layoutVisible lays out the paragraphs that overlap the viewport.
func (t *TextView) layoutVisible() {
	top, bottom, ok := t.visible()
	if !ok {
		return
	}
	idx := t.lines.Index()
	for i := idx.At(max(top, 0)); i < idx.Len() && idx.Y(i) < bottom; i++ {
		t.lines.Layout(i)
	}
}

// syncDocument sizes the document (and the textDoc) to the text: the
// viewport's width, the index's total height plus the container insets,
// at least the viewport's height. It reports whether the size changed.
func (t *TextView) syncDocument() bool {
	vp := t.sv.ViewportSize()
	inY := tokens.TextView.ContainerInsetY
	size := driver.Size{W: max(vp.W, 0), H: max(2*inY+t.lines.Index().Total(), vp.H, 0)}
	if size == t.sv.DocumentSize {
		return false
	}
	t.sv.SetDocumentSize(size)
	t.doc.SetFrame(driver.RectOf(driver.Point{}, size))
	return true
}

// textAnchor is the visual line at the top of the viewport: its first
// character and how far the viewport's top is below the line's (ok false:
// the viewport is at the top of the text, nothing to keep).
type textAnchor struct {
	off int
	dy  float64
	ok  bool
}

// anchor returns the line at the top of the viewport, by the clamped
// offset.
func (t *TextView) anchor() textAnchor {
	top := t.sv.y.Clamped() - tokens.TextView.ContainerInsetY
	if !(top > 0) {
		return textAnchor{}
	}
	idx := t.lines.Index()
	i := idx.At(top)
	l := t.lines.Shown(i) // may make its height exact; its Y stays
	y := top - idx.Y(i)
	m := l.Line(lineAtY(l, y))
	off := t.view.ed.Buffer().ParagraphStart(i) + t.lines.TextOffset(i, m.Start)
	return textAnchor{off: off, dy: max(0, y-m.Top), ok: true}
}

// restore resizes the document after the heights changed and scrolls the
// anchor's line back to where anchor found it.
func (t *TextView) restore(a textAnchor) {
	t.syncDocument()
	if !a.ok {
		return
	}
	buf := t.view.ed.Buffer()
	i := buf.ParagraphIndex(a.off)
	l := t.lines.Shown(i)
	m := l.Line(lineAt(l, t.lines.ShownOffset(i, a.off-buf.ParagraphStart(i)), false))
	y := tokens.TextView.ContainerInsetY + t.lines.Index().Y(i) + m.Top + min(a.dy, m.Height)
	t.syncDocument()
	t.sv.SetOffset(driver.Point{Y: y})
}

// lineAtY returns the line of l at y from its top: the first whose bottom
// is below y, the last one below the text.
func lineAtY(l render.TextLayout, y float64) int {
	n := l.LineCount()
	k := sort.Search(n, func(k int) bool { m := l.Line(k); return m.Top+m.Height > y })
	return min(k, n-1)
}

// scrollRangeToVisible scrolls [start, end) into view by the least amount
// (its start when it is taller than the viewport); nothing without a
// viewport.
func (t *TextView) scrollRangeToVisible(start, end int) {
	t.settle()
	t.scrollSpanToVisible(func() (float64, float64) {
		top, _ := t.caretSpan(start, false, true)
		_, bottom := t.caretSpan(end, false, true) // after start's: its layout can move end's top
		return top, bottom
	})
}

// caretSpan returns the top and the bottom of the caret at off (upstream
// at a soft break) in the document's coordinates, its paragraph laid out;
// with edges, on the first line the top is the document's and on the last
// line the bottom is (the container inset shows with them).
func (t *TextView) caretSpan(off int, upstream, edges bool) (top, bottom float64) {
	i, l, lo, upstream := t.caretIn(off, upstream)
	cr := l.CaretRect(lo)
	if upstream {
		cr = l.CaretRectUpstream(lo)
	}
	idx := t.lines.Index()
	inY := tokens.TextView.ContainerInsetY
	y := inY + idx.Y(i)
	top, bottom = y+cr.Y, y+cr.Y+cr.H
	k := lineAt(l, lo, upstream)
	if edges && i == 0 && k == 0 {
		top = 0
	}
	if edges && i == idx.Len()-1 && k == l.LineCount()-1 {
		bottom += inY
	}
	return top, bottom
}

// caretIn returns where the caret at the editor offset off (upstream at a
// soft break) is drawn: its paragraph i, the paragraph's layout as shown,
// the caret's byte in that layout and whether it is upstream. While
// composing, the caret at the head is the preedit's cursor in the composed
// layout (never upstream).
func (t *TextView) caretIn(off int, upstream bool) (i int, l render.TextLayout, lo int, up bool) {
	v := t.view
	buf := v.ed.Buffer()
	i = buf.ParagraphIndex(off)
	lo = off - buf.ParagraphStart(i)
	if v.composing() && off == v.ed.Head() && t.lines.composes(i) {
		return i, t.lines.Shown(i), lo + v.pre.cursor, false
	}
	return i, t.lines.Layout(i), lo, upstream
}

// caretRect returns the caret as paintText draws it, before pixel
// alignment, in the textDoc's coordinates: CaretWidth wide, the line high,
// at the selection's head (see caretIn).
func (t *TextView) caretRect() driver.Rect {
	ed := t.view.ed
	i, l, lo, up := t.caretIn(ed.Head(), ed.Upstream())
	cr := l.CaretRect(lo)
	if up {
		cr = l.CaretRectUpstream(lo)
	}
	w := tokens.TextField.CaretWidth
	y := tokens.TextView.ContainerInsetY + t.lines.Index().Y(i)
	return driver.Rect{X: tokens.TextView.LineFragmentPadding + cr.X - w/2, Y: y + cr.Y, W: w, H: cr.H}
}

// scrollSpanToVisible scrolls the vertical span that span returns into
// view by the least amount (its top when it is taller than the viewport)
// and settles; nothing without a viewport. The paragraphs that come into
// view are laid out and can move the span (their heights were estimates),
// so it scrolls again from the new span until the offset stays.
func (t *TextView) scrollSpanToVisible(span func() (top, bottom float64)) {
	if _, _, ok := t.visible(); !ok || t.lines == nil {
		return
	}
	for range 8 {
		t.settling = true
		y0, y1 := span()
		t.syncDocument()
		before := t.sv.Offset()
		t.sv.ScrollRectToVisible(driver.Rect{Y: y0, H: y1 - y0})
		moved := t.sv.Offset() != before
		t.settling = false
		t.settle()
		if !moved {
			return
		}
	}
}
