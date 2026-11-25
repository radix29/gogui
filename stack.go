package gogui

import (
	"slices"

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
// widget, on one that appears twice in ws, on one that would contain
// itself, and on a destroyed widget or parent (D13).
func adopt(what string, p Widget, ws []Widget) {
	if p.base().dead {
		panic("gogui: " + what + " on a destroyed widget")
	}
	seen := make(map[Widget]bool, len(ws))
	for _, w := range ws {
		if w == nil {
			panic("gogui: " + what + " with a nil widget")
		}
		if w.base().dead {
			panic("gogui: " + what + " with a destroyed widget")
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
	detachAll(ws)
	for _, w := range ws {
		w.base().parent = nil
		if win, ok := w.base().host.(*Window); ok {
			win.SetContent(nil)
		}
		w.base().parent = p
	}
}

// detachAll removes each of ws from its parent, as detach does, but takes
// all the children of one stack or form out at once (P19), so each old
// parent syncs once and not once per widget. A widget without a parent is
// skipped. The parent links are left for the caller.
func detachAll(ws []Widget) {
	var olds []parentWidget
	var moved map[parentWidget][]Widget
	for _, w := range ws {
		old, ok := w.base().parent.(parentWidget)
		if !ok {
			continue
		}
		if _, batch := old.(batchRemover); !batch {
			old.removeChild(w)
			continue
		}
		if moved == nil {
			moved = make(map[parentWidget][]Widget)
		}
		if _, seen := moved[old]; !seen {
			olds = append(olds, old)
		}
		moved[old] = append(moved[old], w)
	}
	for _, old := range olds {
		old.(batchRemover).removeChildren(moved[old])
	}
}

// batchRemover is a parentWidget that removes several children at once
// (stacks and forms).
type batchRemover interface {
	// removeChildren removes ws, direct children, without destroying them,
	// and syncs once.
	removeChildren(ws []Widget)
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

// SetPadding sets the same padding, in points, on all four sides.
func (s *Stack) SetPadding(v float64) *Stack {
	s.check("SetPadding")
	s.s.SetPadding(UniformInsets(v))
	s.invalidate()
	return s
}

// SetPaddingInsets sets the padding of each side, in points.
func (s *Stack) SetPaddingInsets(in Insets) *Stack {
	s.check("SetPaddingInsets")
	s.s.SetPadding(in)
	s.invalidate()
	return s
}

// Remove takes children out of the stack without destroying them, so they
// can be placed again; Destroy frees a widget for good (D13). A widget
// that is not a child of the stack is skipped.
func (s *Stack) Remove(children ...Widget) *Stack {
	s.check("Remove")
	var ws []Widget
	for _, w := range children {
		if w != nil && w.base().parent == Widget(s) && !slices.Contains(ws, w) {
			ws = append(ws, w)
		}
	}
	if len(ws) > 0 {
		s.removeChildren(ws) // one sync (P19)
		for _, w := range ws {
			w.base().parent = nil
		}
	}
	return s
}

// Clear takes every child out of the stack without destroying it.
func (s *Stack) Clear() *Stack {
	s.check("Clear")
	for _, w := range s.children {
		w.base().parent = nil
	}
	s.children = nil
	s.sync()
	return s
}

// Children returns the children, in order.
func (s *Stack) Children() []Widget { s.get("Children"); return slices.Clone(s.children) }

func (s *Stack) add(what string, ws []Widget) {
	adopt(what, s, ws) // may take children out of s: it is synced then
	s.children = append(s.children, ws...)
	s.appendSync(ws)
}

func (s *Stack) childWidgets() []Widget { return s.children }

func (s *Stack) removeChild(w Widget) { s.removeChildren([]Widget{w}) }

// removeChildren implements batchRemover.
func (s *Stack) removeChildren(ws []Widget) {
	gone := widgetSet(ws)
	s.children = slices.DeleteFunc(slices.Clone(s.children), func(x Widget) bool { return gone(x) })
	s.sync()
}

// widgetSet returns a membership test for ws: a scan for a few, a map for
// more (a batch of every child must not be quadratic).
func widgetSet(ws []Widget) func(Widget) bool {
	if len(ws) <= 8 {
		return func(w Widget) bool { return slices.Contains(ws, w) }
	}
	m := make(map[Widget]bool, len(ws))
	for _, w := range ws {
		m[w] = true
	}
	return func(w Widget) bool { return m[w] }
}

// appendSync pushes ws, just appended to the children of a synced stack,
// to the layout node and the container peer without resending the others
// (P19: building a stack by Add in a loop stays linear).
func (s *Stack) appendSync(ws []Widget) {
	if len(ws) == 0 {
		s.invalidate()
		return
	}
	nodes := make([]layout.Node, len(ws))
	for i, w := range ws {
		nodes[i] = w.node()
	}
	s.s.AppendChildren(nodes...)
	if s.live() {
		if a, ok := s.c.(childAppender); ok {
			a.AppendChildren(peersOf(ws))
		} else {
			s.c.SetChildren(peersOf(s.children))
		}
	}
	s.invalidate()
}

// childAppender is a container peer that can append children without the
// whole list (the aqua and stub containers).
type childAppender interface {
	AppendChildren(children []driver.Widget)
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
	s.kind, s.n, s.self = kindSpacer, layout.NewSpacer(), s
	return s
}

// FormRow is one row of a form, made by Row: plain data, a label text and
// a control. Form creates the row's label, so an unused row holds no
// native object and the same row can go to several forms (A4); its
// control, like any widget, has one parent and moves to the last form.
type FormRow struct {
	label   string
	control Widget
}

// Row returns a form row with a right-aligned label and a control. An
// empty label leaves the row's label cell empty (for checkboxes under the
// fields); the control still starts at the controls' column.
func Row(label string, control Widget) FormRow {
	if control == nil {
		panic("gogui: Row with a nil control")
	}
	return FormRow{label: label, control: control}
}

// formRow is a row placed in a form: its own label (nil for an empty
// label) and the control.
type formRow struct {
	label   *Label
	control Widget
}

// FormView is the widget Form returns: the row labels right-aligned in one
// column and the controls in a second one (the macOS settings layout). A
// label is centered on the first line of its control, and a hidden
// control hides its row.
type FormView struct {
	common[*FormView]
	c    driver.Container
	f    *layout.Form
	rows []formRow
}

// Form returns a form with rows, top to bottom. It creates each row's
// label.
func Form(rows ...FormRow) *FormView {
	c := driverFor("Form").NewContainer()
	for _, r := range rows {
		if r.control == nil {
			panic("gogui: Form with a zero FormRow; use Row")
		}
	}
	lf := layout.NewForm().SetPeer(c)
	f := &FormView{c: c, f: lf}
	f.init(f, kindForm, c, lf)
	var ws []Widget
	for _, r := range rows {
		ws = append(ws, r.control)
	}
	adopt("Form", f, ws)
	for _, r := range rows {
		fr := formRow{control: r.control}
		if r.label != "" {
			fr.label = NewLabel(r.label)
			fr.label.parent = f
		}
		f.rows = append(f.rows, fr)
	}
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

// removeChild removes the row of a moved or destroyed control and
// destroys the row's label, which only this form refers to.
func (f *FormView) removeChild(w Widget) { f.removeChildren([]Widget{w}) }

// removeChildren implements batchRemover: the rows of ws go, their labels
// destroyed, and the form syncs once.
func (f *FormView) removeChildren(ws []Widget) {
	gone := widgetSet(ws)
	var out []formRow
	for _, r := range f.rows {
		if gone(r.control) {

			if r.label != nil {
				r.label.parent = nil
				r.label.destroy() // Destroy also removes the peer from f.c
			}
			continue
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
			r.label.setHidden(hidden)
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
			r.label.setHidden(r.control.node().Props().Hidden())
		}
	}
	f.f.SetRows(rows...)
	if f.live() {
		f.c.SetChildren(peersOf(f.childWidgets()))
	}
	f.invalidate()
}
