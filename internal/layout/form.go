package layout

import (
	"math"

	"github.com/radix29/gogui/internal/driver"
)

// Form spacing, in points, after Apple's Human Interface Guidelines and the
// AppKit layout guides (8 pt between a label and its control, and between
// rows of controls). Written from memory, to be checked against the current
// HIG (no internet access when written, 2026-10-07).
const (
	FormLabelGap   = 8.0 // between the label column and the controls
	FormRowSpacing = 8.0 // between neighbouring visible rows
)

// Row is one row of a Form: a label and a control. A nil Label is an empty
// row label; the control still sits in the control column. The row is
// hidden when its control is hidden.
type Row struct {
	Label   Item
	Control Node
}

// Form lays out rows of right-aligned labels and left-aligned controls. The
// label column is as wide as the widest visible label and never shrinks;
// the controls start at a shared x, FormLabelGap after it (no gap when the
// column is empty), and only they shrink, down to their minima. Across, a
// control takes its explicit horizontal alignment in the control column,
// else Fill when it expands horizontally, else Leading at its natural
// width. Down, the row heights are shared with distribute; a row expands
// when its control expands vertically. A row is max(label, control) tall,
// and the label is centered on the control's first line, never above the
// row's top. A Form has no padding.
type Form struct {
	rows   []Row
	labels []*Leaf // one per row; nil for a nil Label
	peer   Peer
	props  Props
}

// NewForm returns a form. It panics if a row has a nil Control.
func NewForm(rows ...Row) *Form { return (&Form{}).SetRows(rows...) }

// SetRows replaces the rows. It panics if a row has a nil Control.
func (f *Form) SetRows(rows ...Row) *Form {
	labels := make([]*Leaf, len(rows))
	for i, r := range rows {
		if r.Control == nil {
			panic("layout: form row with nil control")
		}
		if r.Label != nil {
			labels[i] = NewLeaf(r.Label)
		}
	}
	f.rows, f.labels = append([]Row(nil), rows...), labels
	return f
}

// SetPeer sets the form's native view; nil makes it a transparent group.
func (f *Form) SetPeer(p Peer) *Form { f.peer = p; return f }

// Rows returns the rows, hidden ones included. Do not modify it.
func (f *Form) Rows() []Row { return f.rows }

// Peer returns the native view, or nil for a transparent group.
func (f *Form) Peer() Peer { return f.peer }

// Props implements Node.
func (f *Form) Props() *Props { return &f.props }

// Expands implements Node: the form's own flag, or any visible row's
// control that expands on a. Labels never expand.
func (f *Form) Expands(a Axis) bool { return f.expandsIn(nil, a) }

// expandsIn is Expands with the controls asked through p (passExpander).
func (f *Form) expandsIn(p *Pass, a Axis) bool {
	if f.props.expand[a] {
		return true
	}
	for _, r := range f.rows {
		if !r.Control.Props().Hidden() && p.Expands(r.Control, a) {
			return true
		}
	}
	return false
}

// formRow is a visible row during a pass; lsize is zero without a label.
type formRow struct {
	ctrl  Node
	label *Leaf
	lsize driver.Size
	w     float64 // the control's width (set by widths)
}

// formCols are the visible rows and the column geometry of a pass.
type formCols struct {
	rows []formRow
	col  float64 // the label column width
	lead float64 // the controls' x: col plus the gap (0 when col is 0)
}

// Measure implements Node. The natural width is the label column, the gap
// and the widest natural control; the minimum uses the widest control
// minimum. The height is the sum of the row heights (measured at the
// resolved control width) plus FormRowSpacing between rows.
func (f *Form) Measure(p *Pass, max driver.Size) driver.Size {
	if f.props.hidden {
		return driver.Size{}
	}
	max = normSize(max)
	defer p.release(p.mark())
	c := f.columns(p)
	var nat, lo float64
	for _, r := range c.rows {
		nat = math.Max(nat, p.Measure(r.ctrl, driver.Unbounded()).W)
		lo = math.Max(lo, p.Measure(r.ctrl, driver.Size{H: math.Inf(1)}).W)
	}
	w := f.props.resolve(Horizontal, c.lead+nat, c.lead+lo, max.W)
	widths(p, c.rows, norm(w-c.lead))
	nat, lo = rowGaps(len(c.rows)), rowGaps(len(c.rows))
	for _, sp := range spans(p, c.rows) {
		nat += sp.natural
		lo += sp.min
	}
	return driver.Size{W: w, H: f.props.resolve(Vertical, nat, lo, max.H)}
}

// Arrange implements Node.
func (f *Form) Arrange(p *Pass, frame driver.Rect) {
	if f.props.hidden {
		return
	}
	origin := frame.Origin()
	if f.peer != nil {
		f.peer.SetFrame(frame)
		origin = driver.Point{}
	}
	defer p.release(p.mark())
	c := f.columns(p)
	if len(c.rows) == 0 {
		return
	}
	cw := norm(frame.W - c.lead)
	widths(p, c.rows, cw)
	sizes := distributeIn(p, spans(p, c.rows), frame.H-rowGaps(len(c.rows)))
	y := origin.Y
	for i, r := range c.rows {
		slot := driver.Rect{X: origin.X + c.lead, Y: y, W: cw, H: sizes[i]}
		h := p.Measure(r.ctrl, driver.Size{W: r.w, H: sizes[i]}).H
		cf := controlFrame(p, r.ctrl, slot, driver.Size{W: r.w, H: h})
		r.ctrl.Arrange(p, cf)
		if r.label != nil {
			fl := firstLine(p, r.ctrl, cf.Size())
			ly := math.Max(y, cf.Y+(fl-r.lsize.H)/2)
			lx := origin.X + c.col - r.lsize.W // right-aligned in the column
			r.label.Arrange(p, driver.Rect{X: lx, Y: ly, W: r.lsize.W, H: r.lsize.H})
		}
		y += sizes[i] + FormRowSpacing
	}
}

// FirstLine reports the first line of the first visible row within max:
// the taller of its label and its control's first line. A form without
// visible rows reports its own height.
func (f *Form) FirstLine(p *Pass, max driver.Size) float64 {
	if f.props.hidden {
		return 0
	}
	max = normSize(max)
	defer p.release(p.mark())
	c := f.columns(p)
	if len(c.rows) == 0 {
		return p.Measure(f, max).H
	}
	first := c.rows[:1]
	widths(p, first, norm(p.Measure(f, max).W-c.lead))
	return math.Max(first[0].lsize.H, firstLine(p, first[0].ctrl, driver.Size{W: first[0].w, H: math.Inf(1)}))
}

// columns collects the visible rows and measures their labels. Labels are
// measured unbounded: they never shrink or wrap.
func (f *Form) columns(p *Pass) formCols {
	c := formCols{rows: p.rowBuf(len(f.rows))}
	for i, r := range f.rows {
		if r.Control.Props().Hidden() {
			continue
		}
		fr := formRow{ctrl: r.Control, label: f.labels[i]}
		if fr.label != nil {
			fr.lsize = p.Measure(fr.label, driver.Unbounded())
			c.col = math.Max(c.col, fr.lsize.W)
		}
		c.rows = append(c.rows, fr)
	}
	if c.lead = c.col; c.col > 0 {
		c.lead += FormLabelGap
	}
	return c
}

// widths sets each row's control width for a control column cw wide.
func widths(p *Pass, rows []formRow, cw float64) {
	for i := range rows {
		ctrl := rows[i].ctrl
		w := p.Measure(ctrl, driver.Size{W: cw, H: math.Inf(1)}).W
		if alignOf(p, ctrl, Horizontal) == Fill {
			w = math.Max(w, math.Min(cw, ctrl.Props().Max(Horizontal)))
		}
		rows[i].w = w
	}
}

// spans returns the row-height spans at the rows' control widths. A row is
// at least as tall as its label and expands when its control expands
// vertically.
func spans(p *Pass, rows []formRow) []span {
	out := p.spanBuf(len(rows))
	for i, r := range rows {
		lh := r.lsize.H
		out[i] = span{
			natural: math.Max(lh, p.Measure(r.ctrl, driver.Size{W: r.w, H: math.Inf(1)}).H),
			min:     math.Max(lh, p.Measure(r.ctrl, driver.Size{W: r.w}).H),
			max:     math.Max(lh, r.ctrl.Props().Max(Vertical)),
			expand:  p.Expands(r.ctrl, Vertical),
		}
	}
	return out
}

// controlFrame places a control of the given size in its row slot. The
// width already includes horizontal Fill (widths). Vertical Fill stretches
// to the slot, capped by the control's max but never below size.
func controlFrame(p *Pass, c Node, slot driver.Rect, size driver.Size) driver.Rect {
	h, v := alignOf(p, c, Horizontal), alignOf(p, c, Vertical)
	if h == Fill {
		h = Leading
	}
	if v == Fill {
		size.H = math.Max(size.H, math.Min(slot.H, c.Props().Max(Vertical)))
		v = Leading
	}
	return place(slot, size, h, v)
}

// alignOf resolves a control's alignment in its row on axis a: explicit,
// else Fill when it expands on a, else Leading.
func alignOf(p *Pass, c Node, a Axis) Align {
	switch {
	case c.Props().Align(a) != AlignDefault:
		return c.Props().Align(a)
	case p.Expands(c, a):
		return Fill
	default:
		return Leading
	}
}

func rowGaps(n int) float64 {
	if n < 2 {
		return 0
	}
	return FormRowSpacing * float64(n-1)
}
