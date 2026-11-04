package layout

import (
	"math"

	"github.com/radix29/gogui/internal/driver"
)

// Peer is the native view a stack or form may own. With a peer, the node
// sets the peer's frame and lays its children out in (0, 0, size); without
// one, it is a transparent group and offsets its children's frames by its
// own origin. Every driver.Widget (in practice a driver.Container)
// satisfies it.
type Peer interface {
	SetFrame(r driver.Rect)
}

// Stack lays out its visible children in a column (NewVStack) or a row
// (NewHStack). Along the main axis the children share the space with
// distribute, with Spacers expanding; across it each child is aligned in
// the inner extent (AlignDefault: Fill when the child expands on the cross
// axis, else Leading in a VStack and Center in an HStack). A child's
// main-axis alignment is not used: it always gets exactly its share.
type Stack struct {
	axis     Axis
	spacing  float64
	padding  driver.Insets
	peer     Peer
	dir      LayoutDirection
	children []Node
	props    Props
}

// NewVStack returns a vertical stack. It panics if a child is nil.
func NewVStack(spacing float64, children ...Node) *Stack {
	return (&Stack{axis: Vertical}).SetSpacing(spacing).SetChildren(children...)
}

// NewHStack returns a horizontal stack. It panics if a child is nil.
func NewHStack(spacing float64, children ...Node) *Stack {
	return (&Stack{axis: Horizontal}).SetSpacing(spacing).SetChildren(children...)
}

// SetChildren replaces the children. It panics if a child is nil.
func (s *Stack) SetChildren(children ...Node) *Stack {
	for _, c := range children {
		if c == nil {
			panic("layout: nil stack child")
		}
	}
	s.children = append([]Node(nil), children...)
	return s
}

// SetSpacing sets the gap between neighbouring visible children (spacers
// included). Negative, NaN and +Inf values count as 0.
func (s *Stack) SetSpacing(v float64) *Stack { s.spacing = finite(v); return s }

// SetPadding sets the insets around the children. Negative, NaN and +Inf
// edges count as 0.
func (s *Stack) SetPadding(in driver.Insets) *Stack {
	s.padding = driver.Insets{Top: finite(in.Top), Left: finite(in.Left),
		Bottom: finite(in.Bottom), Right: finite(in.Right)}
	return s
}

// SetPeer sets the stack's native view; nil makes it a transparent group.
func (s *Stack) SetPeer(p Peer) *Stack { s.peer = p; return s }

// SetDirection sets the reading direction (RightToLeft is laid out as
// LeftToRight for now).
func (s *Stack) SetDirection(d LayoutDirection) *Stack { s.dir = d; return s }

// Axis returns the main axis (Vertical for a VStack).
func (s *Stack) Axis() Axis { return s.axis }

// Spacing returns the gap between visible children.
func (s *Stack) Spacing() float64 { return s.spacing }

// Padding returns the insets around the children.
func (s *Stack) Padding() driver.Insets { return s.padding }

// Peer returns the native view, or nil for a transparent group.
func (s *Stack) Peer() Peer { return s.peer }

// Direction returns the reading direction.
func (s *Stack) Direction() LayoutDirection { return s.dir }

// Children returns the children, hidden ones included. Do not modify it.
func (s *Stack) Children() []Node { return s.children }

// Props implements Node.
func (s *Stack) Props() *Props { return &s.props }

// Expands implements Node: the stack's own flag, or any visible child that
// expands on a. On the main axis, a Spacer counts as expanding.
func (s *Stack) Expands(a Axis) bool {
	if s.props.expand[a] {
		return true
	}
	for _, c := range s.children {
		if !c.Props().Hidden() && s.childExpands(c, a) {
			return true
		}
	}
	return false
}

// Measure implements Node. The children are measured at the inner cross
// extent; their main-axis sizes are shared with distribute, and the cross
// extent is the largest child's cross size at its share (height-for-width).
func (s *Stack) Measure(p *Pass, max driver.Size) driver.Size {
	if s.props.hidden {
		return driver.Size{}
	}
	max = normSize(max)
	main, cross := s.axis, s.axis.Cross()
	kids := s.visible()
	extra := s.pad(main) + s.gaps(len(kids))
	lo, _ := s.props.Min(cross)
	_, hi := s.props.limits(cross, lo)
	innerCross := norm(clamp(get(max, cross), lo, hi) - s.pad(cross))

	spans := s.spans(p, kids, innerCross)
	var natural, minimum float64
	for _, sp := range spans {
		natural += sp.natural
		minimum += sp.min
	}
	m := s.props.resolve(main, natural+extra, minimum+extra, get(max, main))
	sizes := distribute(spans, m-extra)
	var c float64
	for i, k := range kids {
		c = math.Max(c, get(p.Measure(k, mk(main, sizes[i], innerCross)), cross))
	}
	c += s.pad(cross)
	return mk(main, m, s.props.resolve(cross, c, c, get(max, cross)))
}

// Arrange implements Node.
func (s *Stack) Arrange(p *Pass, frame driver.Rect) {
	if s.props.hidden {
		return
	}
	origin := frame.Origin()
	if s.peer != nil {
		s.peer.SetFrame(frame)
		origin = driver.Point{}
	}
	kids := s.visible()
	if len(kids) == 0 {
		return
	}
	main, cross := s.axis, s.axis.Cross()
	inner := driver.Rect{
		X: origin.X + s.padding.Left, Y: origin.Y + s.padding.Top,
		W: norm(frame.W - s.padding.Horizontal()), H: norm(frame.H - s.padding.Vertical()),
	}
	innerCross := get(inner.Size(), cross)
	sizes := distribute(s.spans(p, kids, innerCross), get(inner.Size(), main)-s.gaps(len(kids)))
	pos, crossPos := getPt(inner.Origin(), main), getPt(inner.Origin(), cross)
	for i, k := range kids {
		c := get(p.Measure(k, mk(main, sizes[i], innerCross)), cross)
		slot := driver.RectOf(mkPt(main, pos, crossPos), mk(main, sizes[i], innerCross))
		h, v := Leading, s.crossAlign(k)
		if main == Vertical {
			h, v = v, h
		}
		k.Arrange(p, place(slot, mk(main, sizes[i], c), h, v))
		pos += sizes[i] + s.spacing
	}
}

// FirstLine reports padding.Top plus the first line of the first visible
// child, measured within max minus the padding. A stack without visible
// children reports its own height. (In an HStack this ignores the child's
// cross alignment; real baselines come later.)
func (s *Stack) FirstLine(p *Pass, max driver.Size) float64 {
	if s.props.hidden {
		return 0
	}
	max = normSize(max)
	inner := driver.Size{W: norm(max.W - s.padding.Horizontal()), H: norm(max.H - s.padding.Vertical())}
	for _, k := range s.children {
		if !k.Props().Hidden() {
			return s.padding.Top + firstLine(p, k, inner)
		}
	}
	return p.Measure(s, max).H
}

// spans measures the main-axis natural (unbounded) and min (at 0) size of
// each child at the inner cross extent.
func (s *Stack) spans(p *Pass, kids []Node, innerCross float64) []span {
	main := s.axis
	out := make([]span, len(kids))
	for i, k := range kids {
		out[i] = span{
			natural: get(p.Measure(k, mk(main, math.Inf(1), innerCross)), main),
			min:     get(p.Measure(k, mk(main, 0, innerCross)), main),
			max:     k.Props().Max(main),
			expand:  s.childExpands(k, main),
		}
	}
	return out
}

func (s *Stack) childExpands(c Node, a Axis) bool {
	if _, ok := c.(*Spacer); ok && a == s.axis {
		return true
	}
	return c.Expands(a)
}

func (s *Stack) crossAlign(c Node) Align {
	cross := s.axis.Cross()
	switch {
	case c.Props().Align(cross) != AlignDefault:
		return c.Props().Align(cross)
	case c.Expands(cross):
		return Fill
	case s.axis == Vertical:
		return Leading
	default:
		return Center
	}
}

func (s *Stack) visible() []Node {
	out := make([]Node, 0, len(s.children))
	for _, c := range s.children {
		if !c.Props().Hidden() {
			out = append(out, c)
		}
	}
	return out
}

func (s *Stack) gaps(n int) float64 {
	if n < 2 {
		return 0
	}
	return s.spacing * float64(n-1)
}

func (s *Stack) pad(a Axis) float64 {
	if a == Horizontal {
		return s.padding.Horizontal()
	}
	return s.padding.Vertical()
}
