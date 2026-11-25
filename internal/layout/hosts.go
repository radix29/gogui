package layout

import (
	"math"

	"github.com/radix29/gogui/internal/driver"
)

// BoxPeer is what a Box node needs from its native view. Every driver.Box
// satisfies it.
type BoxPeer interface {
	SetFrame(r driver.Rect)
	// ContentInsets is read again on every measure and arrange, because
	// it depends on the title and the control size.
	ContentInsets() driver.Insets
}

// ScrollPeer is what a ScrollView node needs from its native view. Every
// driver.ScrollView satisfies it.
type ScrollPeer interface {
	SetFrame(r driver.Rect)
	// ViewportSize depends only on the frame, the enabled scrollers and
	// the appearance, so SetDocumentSize never changes it.
	ViewportSize() driver.Size
	SetDocumentSize(s driver.Size)
}

var (
	_ BoxPeer    = driver.Box(nil)
	_ ScrollPeer = driver.ScrollView(nil)
)

// Box is a group box that holds one content node. It always owns its peer
// and starts a new coordinate space: the content is framed at
// (0, 0, frame.Inset(insets).Size()) in the box's content area, so the
// insets change the content's size, not its origin. The box measures as its
// content plus the insets (its default minimum is the content's minimum
// plus the insets, not the peer's FittingSize), and it expands on an axis
// when it or its visible content does.
type Box struct {
	peer    BoxPeer
	content Node
	props   Props
}

// NewBox returns a box node for peer with content (nil for none). It panics
// if peer is nil.
func NewBox(peer BoxPeer, content Node) *Box {
	if peer == nil {
		panic("layout: NewBox with nil peer")
	}
	return &Box{peer: peer, content: content}
}

// SetContent replaces the content node; nil removes it.
func (b *Box) SetContent(n Node) *Box { b.content = n; return b }

// Content returns the content node, or nil.
func (b *Box) Content() Node { return b.content }

// Peer returns the box's native view.
func (b *Box) Peer() BoxPeer { return b.peer }

// Props implements Node.
func (b *Box) Props() *Props { return &b.props }

// Expands implements Node: the own flag, or the visible content's.
func (b *Box) Expands(a Axis) bool { return b.expandsIn(nil, a) }

// expandsIn is Expands with the content asked through p (passExpander).
func (b *Box) expandsIn(p *Pass, a Axis) bool {
	return b.props.expand[a] || (visible(b.content) && p.Expands(b.content, a))
}

// Measure implements Node. The width is resolved first (content natural
// and minimum width plus the insets), then the height at the resulting
// inner width (height-for-width).
func (b *Box) Measure(p *Pass, max driver.Size) driver.Size {
	if b.props.hidden {
		return driver.Size{}
	}
	max = normSize(max)
	in := b.insets()
	inf := math.Inf(1)
	natW := b.measureContent(p, driver.Size{W: inf, H: inf}).W
	minW := b.measureContent(p, driver.Size{W: 0, H: inf}).W
	w := b.props.resolve(Horizontal, natW+in.Horizontal(), minW+in.Horizontal(), max.W)
	innerW := norm(w - in.Horizontal())
	natH := b.measureContent(p, driver.Size{W: innerW, H: inf}).H
	minH := b.measureContent(p, driver.Size{W: innerW, H: 0}).H
	h := b.props.resolve(Vertical, natH+in.Vertical(), minH+in.Vertical(), max.H)
	return driver.Size{W: w, H: h}
}

// Arrange implements Node.
func (b *Box) Arrange(p *Pass, frame driver.Rect) {
	if b.props.hidden {
		return
	}
	b.peer.SetFrame(frame)
	if visible(b.content) {
		b.content.Arrange(p, driver.RectOf(driver.Point{}, frame.Inset(b.insets()).Size()))
	}
}

func (b *Box) insets() driver.Insets {
	in := b.peer.ContentInsets()
	return driver.Insets{Top: finite(in.Top), Left: finite(in.Left),
		Bottom: finite(in.Bottom), Right: finite(in.Right)}
}

func (b *Box) measureContent(p *Pass, max driver.Size) driver.Size {
	if b.content == nil {
		return driver.Size{}
	}
	return p.Measure(b.content, max)
}

// ScrollView is a scroll view that shows one document node. It always owns
// its peer and starts a new coordinate space. Its own size comes only from
// its Props (a min width, for example); with no min it measures 0. It
// expands only by its own flag, never by its document's.
//
// Arrange frames the peer, reads its viewport, measures the document
// unbounded on each axis that scrolls and at the viewport size on the
// others, and sets the document size: on scrolling axes the larger of the
// measured size and the viewport, on the others the viewport. The document
// is framed at (0, 0, documentSize). Without a visible document the
// document size is the viewport.
//
// The scroll axes mirror the peer's scrollers; the public layer keeps them
// in sync and re-runs the layout on ViewportResized.
type ScrollView struct {
	peer    ScrollPeer
	doc     Node
	scrolls [2]bool
	props   Props
}

// NewScrollView returns a scroll view node for peer with doc (nil for
// none). No axis scrolls until SetScrolls. It panics if peer is nil.
func NewScrollView(peer ScrollPeer, doc Node) *ScrollView {
	if peer == nil {
		panic("layout: NewScrollView with nil peer")
	}
	return &ScrollView{peer: peer, doc: doc}
}

// SetDocument replaces the document node; nil removes it.
func (s *ScrollView) SetDocument(n Node) *ScrollView { s.doc = n; return s }

// Document returns the document node, or nil.
func (s *ScrollView) Document() Node { return s.doc }

// SetScrolls sets whether the document scrolls on axis a (the peer has a
// scroller there).
func (s *ScrollView) SetScrolls(a Axis, on bool) *ScrollView { s.scrolls[a] = on; return s }

// Scrolls reports whether the document scrolls on axis a.
func (s *ScrollView) Scrolls(a Axis) bool { return s.scrolls[a] }

// Peer returns the scroll view's native view.
func (s *ScrollView) Peer() ScrollPeer { return s.peer }

// Props implements Node.
func (s *ScrollView) Props() *Props { return &s.props }

// Expands implements Node: only the own flag.
func (s *ScrollView) Expands(a Axis) bool { return s.props.expand[a] }

// Measure implements Node. The document is not measured.
func (s *ScrollView) Measure(_ *Pass, max driver.Size) driver.Size {
	if s.props.hidden {
		return driver.Size{}
	}
	max = normSize(max)
	return driver.Size{W: s.props.resolve(Horizontal, 0, 0, max.W), H: s.props.resolve(Vertical, 0, 0, max.H)}
}

// Arrange implements Node.
func (s *ScrollView) Arrange(p *Pass, frame driver.Rect) {
	if s.props.hidden {
		return
	}
	s.peer.SetFrame(frame)
	vp := finiteSize(s.peer.ViewportSize())
	size := vp
	if visible(s.doc) {
		limit := vp
		if s.scrolls[Horizontal] {
			limit.W = math.Inf(1)
		}
		if s.scrolls[Vertical] {
			limit.H = math.Inf(1)
		}
		m := p.Measure(s.doc, limit)
		if s.scrolls[Horizontal] {
			size.W = math.Max(m.W, vp.W)
		}
		if s.scrolls[Vertical] {
			size.H = math.Max(m.H, vp.H)
		}
	}
	s.peer.SetDocumentSize(size)
	if visible(s.doc) {
		s.doc.Arrange(p, driver.RectOf(driver.Point{}, size))
	}
}

// visible reports whether n is a node that is not hidden.
func visible(n Node) bool { return n != nil && !n.Props().Hidden() }
