package aqua

import (
	"math"
	"time"

	"github.com/radix29/gogui/internal/aqua/text"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
)

// The TextView's input (see TextView): its key map and the mouse hooks
// of the shared text element (textEdit), with the drag's auto-scroll.

// key implements editHost: the multi-line key map (see TextView).
func (e *textViewElem) key(ev shell.KeyEvent) bool {
	const ctrlTab = driver.ModControl | driver.ModShift
	if ev.Key == driver.KeyTab && ev.Mods&driver.ModControl != 0 && ev.Mods&^ctrlTab == 0 {
		r := e.routedBy()
		return r != nil && r.FocusNext(ev.Mods&driver.ModShift != 0)
	}
	cmd, ok := text.Lookup(ev.Key, ev.Mods, true)
	if !ok {
		return false
	}
	if cmd.IsClipboard() {
		e.clipboard(cmd)
		return true
	}
	if cmd.Edits() && e.p.ReadOnly {
		return cmd != text.CmdInsertNewline && cmd != text.CmdInsertTab
	}
	e.p.do(cmd)
	return true
}

// Auto-scroll during a drag (see TextView): the speed past the viewport's
// top or bottom edge, in points per second, at the edge and per point
// further out, its cap, and the longest time one frame scrolls for (a
// late frame does not jump). Provisional (S5.4i).
const (
	autoScrollMin     = 60.0
	autoScrollGain    = 12.0
	autoScrollMax     = 6000.0
	autoScrollMaxStep = 100 * time.Millisecond
)

// hitAt implements editHost (TextView.hitAt).
func (e *textViewElem) hitAt(p driver.Point) (int, bool) { return e.p.hitAt(p) }

// selected implements editHost: it repaints and restarts the caret blink
// after the mouse changed the selection (no TextChanged, no scroll: the
// head is under the pointer, or the auto-scroll brings it).
func (e *textViewElem) selected() {
	e.restartBlink()
	e.Invalidate()
}

// autoScroll implements editHost: the drag's auto-scroll on the frame
// clock (see TextView).
func (e *textViewElem) autoScroll(on bool) {
	if on {
		e.p.updateAutoScroll()
	} else {
		e.p.stopAutoScroll()
	}
}

// hitAt returns the editor offset for a press or a drag at pos (the
// element's coordinates) and whether the caret there is upstream (right
// of a soft-wrapped line): the nearest cluster boundary on the visual line
// at pos's height, the first line in the top inset, the text's start above
// the document and its end below the last line. A paragraph laid out for
// it above the viewport keeps the visible lines in place (the anchor).
// Without a viewport it returns the head.
func (t *TextView) hitAt(pos driver.Point) (int, bool) {
	t.settle()
	ed := t.view.ed
	if t.lines == nil {
		return ed.Head(), ed.Upstream()
	}
	p := FromRoot(t.doc, ToRoot(t.view, pos))
	buf, idx := ed.Buffer(), t.lines.Index()
	y := p.Y - tokens.TextView.ContainerInsetY
	switch {
	case p.Y < 0:
		return 0, false
	case y >= idx.Total():
		return buf.Len(), false
	}
	y = max(y, 0)
	i := idx.At(y)
	ly := y - idx.Y(i) // laying out i moves the paragraphs below it, not its top
	var l render.TextLayout
	if top, _, _ := t.visible(); idx.Y(i) < top && !t.settling {
		t.settling = true
		a := t.anchor()
		l = t.lines.Layout(i)
		t.restore(a)
		t.settling = false
	} else {
		l = t.lines.Layout(i)
	}
	k := lineAtY(l, ly)
	m := l.Line(k)
	hit, trailing := l.HitTest(driver.Point{X: p.X - tokens.TextView.LineFragmentPadding, Y: m.Top + m.Height/2})
	if trailing {
		hit = text.NextGrapheme(l.Text(), hit)
	}
	hit = min(max(hit, m.Start), m.End)
	return buf.ParagraphStart(i) + hit, hit == m.End && k < l.LineCount()-1
}

// autoSpeed returns the auto-scroll's speed for the drag's pointer, in
// points per second (negative: up), 0 within the viewport's height.
func (t *TextView) autoSpeed() float64 {
	p := FromRoot(t.sv.viewport, t.view.drag.pos)
	h := t.sv.ViewportSize().H
	var d float64
	switch {
	case p.Y < 0:
		d = p.Y
	case p.Y > h:
		d = p.Y - h
	default:
		return 0
	}
	return math.Copysign(min(autoScrollMin+autoScrollGain*math.Abs(d), autoScrollMax), d)
}

// canAutoScroll reports whether the text can scroll at speed v: up from
// below the top, down from above the bottom.
func (t *TextView) canAutoScroll(v float64) bool {
	y := t.sv.y.Clamped()
	return v < 0 && y > 0 || v > 0 && y < t.sv.y.Max()
}

// updateAutoScroll starts the auto-scroll while a drag's pointer is past
// the viewport's top or bottom and the text can scroll that way, and
// stops it otherwise.
func (t *TextView) updateAutoScroll() {
	v := t.autoSpeed()
	if !t.view.drag.on || v == 0 || !t.canAutoScroll(v) {
		t.stopAutoScroll()
		return
	}
	if t.auto == nil {
		tl := t.view.d.Timeline() // the scroll view's too
		if tl == nil {
			return // before Ready: no frame clock
		}
		t.auto = tl.NewTicker(t.autoTick)
	}
	t.auto.Start()
}

// stopAutoScroll stops the auto-scroll, if it runs.
func (t *TextView) stopAutoScroll() {
	if t.auto != nil {
		t.auto.Stop()
	}
}

// autoTick is a frame of the auto-scroll: the text scrolls by the speed
// for the time since the last frame (at most autoScrollMaxStep), the
// selection follows to the text now under the pointer, and the ticker
// stops at the document's end, and when the drag ended, the view no
// longer shows or is disabled.
func (t *TextView) autoTick(dt time.Duration) {
	if !t.view.drag.on || !t.view.EnabledInTree() || !t.shown() {
		t.stopAutoScroll()
		return
	}
	v := t.autoSpeed()
	if v == 0 || !t.canAutoScroll(v) {
		t.stopAutoScroll()
		return
	}
	y := min(max(t.sv.y.Clamped()+v*min(dt, autoScrollMaxStep).Seconds(), 0), t.sv.y.Max())
	t.sv.SetOffset(driver.Point{Y: y})
	t.view.dragSelectAt()
	if !t.canAutoScroll(v) {
		t.stopAutoScroll() // at the end: no more frames
	}
}
