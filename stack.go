package gogui

import (
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/layout"
)

// parentWidget is implemented by the widgets that hold children: stacks
// and forms, radio groups, boxes and scroll views.
type parentWidget interface {
	Widget
	// childWidgets returns the direct children in peer order.
	childWidgets() []Widget
	// removeChild removes w, a direct child, without destroying it.
	removeChild(w Widget)
}

// adopt makes p the parent of ws. A widget has one parent, so each is
// first removed from its old one (which may be p: re-adding moves it to
// the end), or from the window it is the content of. It panics on a nil
// widget, on one that appears twice in ws, and on one that would contain
// itself.
func adopt(what string, p Widget, ws []Widget) {
	seen := make(map[Widget]bool, len(ws))
	for _, w := range ws {
		if w == nil {
			panic("gogui: " + what + " with a nil widget")
		}
		if seen[w] {
			panic("gogui: " + what + " with the same widget twice")
		}
		seen[w] = true
		for a := p; a != nil; a = a.base().parent {
			if a == w {
				panic("gogui: " + what + ": a widget can't contain itself")
			}
		}
	}
	for _, w := range ws {
		detach(w)
		w.base().parent = p
	}
}

// peersOf returns the peers of ws in order, skipping widgets without one.
func peersOf(ws []Widget) []driver.Widget {
	out := make([]driver.Widget, 0, len(ws))
	for _, w := range ws {
		if p := w.peerWidget(); p != nil {
			out = append(out, p)
		}
	}
	return out
}

// Stack lays out its children in a column (VStack) or a row (HStack),
// spacing points apart. It expands when one of its visible children does,
// and a Spacer in it takes the leftover space along its axis.
type Stack struct {
	common[*Stack]
	c        driver.Container
	s        *layout.Stack
	children []Widget
}

// VStack returns a stack that places children top to bottom. A child is
// aligned leading, or fills the width if it expands horizontally.
func VStack(spacing float64, children ...Widget) *Stack {
	return newStack("VStack", layout.NewVStack(spacing), children)
}

// HStack returns a stack that places children leading to trailing. A
// child is centered vertically, or fills the height if it expands
// vertically.
func HStack(spacing float64, children ...Widget) *Stack {
	return newStack("HStack", layout.NewHStack(spacing), children)
}

func newStack(what string, ls *layout.Stack, children []Widget) *Stack {
	c := driverFor(what).NewContainer()
	s := &Stack{c: c, s: ls.SetPeer(c)}
	s.init(s, kindStack, c, ls)
	s.add(what, children)
	return s
}

// Add appends children to the stack. A child that is already in a stack
// or form (this one included) is moved to the end of this one.
func (s *Stack) Add(children ...Widget) *Stack {
	s.check("Add")
	s.add("Stack.Add", children)
	return s
}

// Padding sets the same padding, in points, on all four sides.
func (s *Stack) Padding(v float64) *Stack {
	s.check("Padding")
	s.s.SetPadding(UniformInsets(v))
	s.invalidate()
	return s
}

// PaddingInsets sets the padding of each side, in points.
func (s *Stack) PaddingInsets(in Insets) *Stack {
	s.check("PaddingInsets")
	s.s.SetPadding(in)
	s.invalidate()
	return s
}

func (s *Stack) add(what string, ws []Widget) {
	adopt(what, s, ws)
	s.children = append(s.children, ws...)
	s.sync()
}

func (s *Stack) childWidgets() []Widget { return s.children }

func (s *Stack) removeChild(w Widget) {
	out := make([]Widget, 0, len(s.children))
	for _, x := range s.children {
		if x != w {
			out = append(out, x)
		}
	}
	s.children = out
	s.sync()
}

// sync pushes the children to the layout node and the container peer.
func (s *Stack) sync() {
	nodes := make([]layout.Node, len(s.children))
	for i, w := range s.children {
		nodes[i] = w.node()
	}
	s.s.SetChildren(nodes...)
	if s.live() {
		s.c.SetChildren(peersOf(s.children))
	}
	s.invalidate()
}

// spacer is the widget Spacer returns. It has no peer.
type spacer struct{ widget }

// Spacer returns an empty widget that takes the leftover space along its
// stack's axis; several spacers share it equally. Outside a stack it has
// zero size.
func Spacer() Widget {
	s := &spacer{}
	s.kind, s.n = kindSpacer, layout.NewSpacer()
	return s
}

// FormRow is one row of a form, made by Row.
type FormRow struct {
	label   *Label // nil for an empty label
	control Widget
}

// Row returns a form row with a right-aligned label and a control. An
// empty label leaves the row's label cell empty (for checkboxes under the
// fields); the control still starts at the controls' column.
func Row(label string, control Widget) FormRow {
	if control == nil {
		panic("gogui: Row with a nil control")
	}
	r := FormRow{control: control}
	if label != "" {
		driverFor("Row")
		r.label = NewLabel(label)
	}
	return r
}

// FormView is the widget Form returns: the row labels right-aligned in one
// column and the controls in a second one (the macOS settings layout). A
// label is centered on the first line of its control, and a hidden
// control hides its row.
type FormView struct {
	common[*FormView]
	c    driver.Container
	f    *layout.Form
	rows []FormRow
}

// Form returns a form with rows, top to bottom.
func Form(rows ...FormRow) *FormView {
	c := driverFor("Form").NewContainer()
	lf := layout.NewForm().SetPeer(c)
	f := &FormView{c: c, f: lf}
	f.init(f, kindForm, c, lf)
	var ws []Widget
	for _, r := range rows {
		if r.control == nil {
			panic("gogui: Form with a zero FormRow; use Row")
		}
		if r.label != nil {
			ws = append(ws, r.label)
		}
		ws = append(ws, r.control)
	}
	adopt("Form", f, ws)
	f.rows = append([]FormRow(nil), rows...)
	f.sync()
	return f
}

// childWidgets returns each row's label (if any) and control, in order.
func (f *FormView) childWidgets() []Widget {
	var out []Widget
	for _, r := range f.rows {
		if r.label != nil {
			out = append(out, r.label)
		}
		out = append(out, r.control)
	}
	return out
}

// removeChild removes the row of a moved control and destroys the row's
// label, which only this form refers to. A label moved away (the same
// FormRow passed to another Form) leaves its row without a label.
func (f *FormView) removeChild(w Widget) {
	var out []FormRow
	for _, r := range f.rows {
		switch {
		case r.control == w:
			if r.label != nil && r.label.parent == Widget(f) {
				r.label.parent = nil
				r.label.destroy() // Destroy also removes the peer from f.c
			}
			continue
		case r.label != nil && Widget(r.label) == w:
			r.label = nil
		}
		out = append(out, r)
	}
	f.rows = out
	f.sync()
}

// childHidden hides a row's label with its control: the layout engine
// skips the row, but the label's peer would stay visible.
func (f *FormView) childHidden(b *widget, hidden bool) {
	for _, r := range f.rows {
		if r.control.base() == b && r.label != nil {
			r.label.SetHidden(hidden)
		}
	}
}

// sync pushes the rows to the layout node and the container peer.
func (f *FormView) sync() {
	rows := make([]layout.Row, len(f.rows))
	for i, r := range f.rows {
		rows[i].Control = r.control.node()
		if r.label != nil {
			rows[i].Label = r.label.peer
			r.label.SetHidden(r.control.node().Props().Hidden())
		}
	}
	f.f.SetRows(rows...)
	if f.live() {
		f.c.SetChildren(peersOf(f.childWidgets()))
	}
	f.invalidate()
}
