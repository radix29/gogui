package layout

import (
	"math"

	"github.com/radix29/gogui/internal/driver"
)

// Axis selects the horizontal (width, X) or vertical (height, Y) component.
type Axis uint8

const (
	Horizontal Axis = iota
	Vertical
)

// Cross returns the other axis.
func (a Axis) Cross() Axis { return 1 - a }

// Align is a node's alignment inside its slot, per axis.
type Align uint8

const (
	// AlignDefault lets the parent choose (Leading in a VStack, Center in
	// an HStack, Fill when the child expands on that axis).
	AlignDefault Align = iota
	Leading
	Center
	Trailing
	Fill
)

// LayoutDirection is the reading direction of a stack or form. RightToLeft
// is accepted but laid out as LeftToRight for now (PLAN.md §7.4).
type LayoutDirection uint8

const (
	LeftToRight LayoutDirection = iota
	RightToLeft
)

// Item is what a Leaf lays out. Every driver.Widget satisfies it, and aqua
// uses it for internal parts that have no peer.
type Item interface {
	FittingSize(max driver.Size) driver.Size
	SetFrame(r driver.Rect)
}

// Node is an element of the layout tree. The contract is fixed for all
// node types (PLAN.md §11.1, S0.4).
type Node interface {
	// Props returns the node's sizing properties. Never nil.
	Props() *Props

	// Expands reports whether the node wants extra space on axis a: its
	// own flag, or one propagated from its children. Parents skip hidden
	// children, so it does not look at Hidden.
	Expands(a Axis) bool

	// Measure returns the size the node wants within max: its natural
	// size, shrunk towards its minimum when max is smaller, but never
	// below the minimum, so the result can exceed max. Components of max
	// may be +Inf. Nodes measure their children only through p.Measure.
	Measure(p *Pass, max driver.Size) driver.Size

	// Arrange gives the node its final frame. The parent has already
	// aligned it inside its slot (place). Hidden nodes ignore it.
	Arrange(p *Pass, frame driver.Rect)
}

// Props are the sizing properties every node has. The zero value means:
// no min (a leaf then uses its fitting size), no max (+Inf), default
// alignment, no expansion, visible. Setters normalize their argument:
// negative and NaN values count as 0, and a min of +Inf (or a max of +Inf)
// means unset. Min wins over max.
type Props struct {
	min    [2]float64
	hasMin [2]bool
	max    [2]float64
	hasMax [2]bool
	align  [2]Align
	expand [2]bool
	hidden bool
}

// SetMin sets the minimum extent on axis a. +Inf removes it.
func (p *Props) SetMin(a Axis, v float64) *Props {
	v = norm(v)
	if math.IsInf(v, 1) {
		p.min[a], p.hasMin[a] = 0, false
	} else {
		p.min[a], p.hasMin[a] = v, true
	}
	return p
}

// SetMax sets the maximum extent on axis a. +Inf removes it.
func (p *Props) SetMax(a Axis, v float64) *Props {
	v = norm(v)
	p.max[a], p.hasMax[a] = v, !math.IsInf(v, 1)
	return p
}

// SetFixed sets min and max on axis a to v.
func (p *Props) SetFixed(a Axis, v float64) *Props { return p.SetMin(a, v).SetMax(a, v) }

// Min returns the minimum on axis a and whether one is set.
func (p *Props) Min(a Axis) (float64, bool) { return p.min[a], p.hasMin[a] }

// Max returns the maximum on axis a, or +Inf if none is set.
func (p *Props) Max(a Axis) float64 {
	if !p.hasMax[a] {
		return math.Inf(1)
	}
	return p.max[a]
}

// SetAlign sets the alignment on axis a.
func (p *Props) SetAlign(a Axis, al Align) *Props { p.align[a] = al; return p }

// Align returns the alignment on axis a.
func (p *Props) Align(a Axis) Align { return p.align[a] }

// SetExpand sets whether the node wants extra space on axis a.
func (p *Props) SetExpand(a Axis, on bool) *Props { p.expand[a] = on; return p }

// Expand returns the node's own expansion flag on axis a.
func (p *Props) Expand(a Axis) bool { return p.expand[a] }

// SetHidden mirrors the widget's hidden state. Hidden nodes are neither
// measured nor framed, and they get no spacing.
func (p *Props) SetHidden(h bool) *Props { p.hidden = h; return p }

// Hidden reports whether the node is hidden.
func (p *Props) Hidden() bool { return p.hidden }

// limits returns the effective [lo, hi] on axis a. defMin is used when no
// min is set. hi is never below lo (min wins).
func (p *Props) limits(a Axis, defMin float64) (lo, hi float64) {
	lo, hi = defMin, p.Max(a)
	if p.hasMin[a] {
		lo = p.min[a]
	}
	return lo, math.Max(lo, hi)
}

// resolve returns the extent on axis a for a node whose natural extent is
// natural, given defMin (used when no min is set) and the available space.
func (p *Props) resolve(a Axis, natural, defMin, avail float64) float64 {
	lo, hi := p.limits(a, defMin)
	return math.Max(lo, math.Min(clamp(natural, lo, hi), avail))
}

// Leaf is a node that wraps one Item, usually a widget peer.
type Leaf struct {
	item  Item
	props Props
}

// NewLeaf returns a leaf for item. It panics if item is nil.
func NewLeaf(item Item) *Leaf {
	if item == nil {
		panic("layout: NewLeaf with nil item")
	}
	return &Leaf{item: item}
}

// Item returns the wrapped item.
func (l *Leaf) Item() Item { return l.item }

// Props implements Node.
func (l *Leaf) Props() *Props { return &l.props }

// Expands implements Node: a leaf expands only by its own flag.
func (l *Leaf) Expands(a Axis) bool { return l.props.expand[a] }

// Measure implements Node. The default min is the fitting size. The width
// is resolved first. When it differs from the fitting width, the natural
// height (and the default min height) is the fitting height at that width
// (height-for-width, for wrapping labels).
func (l *Leaf) Measure(_ *Pass, max driver.Size) driver.Size {
	if l.props.hidden {
		return driver.Size{}
	}
	max = normSize(max)
	fit := finiteSize(l.item.FittingSize(driver.Unbounded()))
	w := l.props.resolve(Horizontal, fit.W, fit.W, max.W)
	natH := fit.H
	if w != fit.W {
		natH = finite(l.item.FittingSize(driver.Size{W: w, H: math.Inf(1)}).H)
	}
	h := l.props.resolve(Vertical, natH, natH, max.H)
	return driver.Size{W: w, H: h}
}

// Arrange implements Node.
func (l *Leaf) Arrange(_ *Pass, frame driver.Rect) {
	if l.props.hidden {
		return
	}
	l.item.SetFrame(frame)
}

// place positions a node of the given (measured) size inside slot.
// AlignDefault counts as Leading; parents resolve defaults first. Fill
// stretches to the slot but never shrinks below size. A node larger than
// its slot starts at the slot's origin and overflows at the trailing edge.
func place(slot driver.Rect, size driver.Size, h, v Align) driver.Rect {
	x, w := place1(slot.X, slot.W, size.W, h)
	y, ht := place1(slot.Y, slot.H, size.H, v)
	return driver.Rect{X: x, Y: y, W: w, H: ht}
}

func place1(origin, slot, size float64, al Align) (pos, ext float64) {
	free := math.Max(0, slot-size)
	switch al {
	case Center:
		return origin + free/2, size
	case Trailing:
		return origin + free, size
	case Fill:
		return origin, math.Max(slot, size)
	default:
		return origin, size
	}
}

// clamp limits v to [lo, hi]. lo wins when lo > hi.
func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(v, hi)) }

// norm maps negative and NaN values to 0. +Inf stays.
func norm(v float64) float64 {
	if !(v > 0) {
		return 0
	}
	return v
}

// finite is norm that also maps +Inf to 0, for sizes reported by items.
func finite(v float64) float64 {
	if math.IsInf(v, 1) {
		return 0
	}
	return norm(v)
}

func normSize(s driver.Size) driver.Size   { return driver.Size{W: norm(s.W), H: norm(s.H)} }
func finiteSize(s driver.Size) driver.Size { return driver.Size{W: finite(s.W), H: finite(s.H)} }
