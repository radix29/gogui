package gogui

import (
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/layout"
)

// ScrollView shows part of one document widget and scrolls it
// (NSScrollView). Its size comes from the layout and from SetMinWidth and
// SetMinHeight, never from the document. It expands on each axis that has
// a scroller. On those axes the document gets its natural size (at least
// the visible area); on the others it fills the visible area.
//
// On Windows and Linux the scrollers follow the OS setting as macOS does:
// overlay scrollers that show while scrolling, or legacy ones that are
// always shown and take room (Windows: Settings, "Always show
// scrollbars"; Linux: gtk-overlay-scrolling off).
type ScrollView struct {
	common[*ScrollView]
	p        driver.ScrollView
	n        *layout.ScrollView
	doc      Widget
	off      Point // the last offset seen, for Offset after destruction
	onScroll subscribers[func(Point)]
}

// NewScrollView returns a scroll view around doc (nil for none) with no
// scrollers; SetHasVerticalScroller and SetHasHorizontalScroller turn them
// on.
func NewScrollView(doc Widget) *ScrollView {
	s := &ScrollView{}
	p := driverFor("NewScrollView").NewScrollView(scrollEvents{s})
	s.p, s.n = p, layout.NewScrollView(p, nil)
	s.init(s, kindScrollView, p, s.n)
	if doc != nil {
		s.SetDocument(doc)
	}
	return s
}

// SetDocument sets the widget that is scrolled; nil removes it. A widget
// that is in a stack, a form, a box, another scroll view or a window is
// moved here. The old document is taken out but not destroyed.
func (s *ScrollView) SetDocument(doc Widget) *ScrollView {
	s.check("SetDocument")
	if doc == s.doc {
		return s
	}
	if doc != nil {
		adopt("ScrollView.SetDocument", s, []Widget{doc})
	}
	s.setDocument(doc)
	return s
}

// Document returns the document widget, or nil.
func (s *ScrollView) Document() Widget { s.get("Document"); return s.doc }

// setDocument puts doc (nil for none) in the layout node and the peer.
// The caller has made s the parent of doc.
func (s *ScrollView) setDocument(doc Widget) {
	if old := s.doc; old != nil && old.base().parent == Widget(s) {
		old.base().parent = nil
	}
	s.doc = doc
	n, peer := nodeAndPeer(doc)
	s.n.SetDocument(n)
	if s.live() {
		s.p.SetDocument(peer)
	}
	s.invalidate()
}

// SetHasVerticalScroller makes the document scroll vertically. The scroll
// view then expands vertically.
func (s *ScrollView) SetHasVerticalScroller(has bool) *ScrollView {
	s.check("SetHasVerticalScroller")
	if s.live() {
		s.p.SetHasVerticalScroller(has)
	}
	s.setScrolls(layout.Vertical, has)
	return s
}

// SetHasHorizontalScroller makes the document scroll horizontally. The
// scroll view then expands horizontally.
func (s *ScrollView) SetHasHorizontalScroller(has bool) *ScrollView {
	s.check("SetHasHorizontalScroller")
	if s.live() {
		s.p.SetHasHorizontalScroller(has)
	}
	s.setScrolls(layout.Horizontal, has)
	return s
}

// setScrolls keeps the axis flag, the layout node and the default
// expansion in sync with the peer's scrollers.
func (s *ScrollView) setScrolls(a layout.Axis, on bool) {
	s.scrolls[a] = on
	s.n.SetScrolls(a, on)
	s.applyExpand()
	s.invalidate()
}

// SetMinWidth sets the smallest width, in points, the layout gives the
// scroll view. Without it the scroll view can shrink to nothing.
func (s *ScrollView) SetMinWidth(w float64) *ScrollView {
	s.check("SetMinWidth")
	s.n.Props().SetMin(layout.Horizontal, w)
	s.invalidate()
	return s
}

// SetMinHeight sets the smallest height, in points, the layout gives the
// scroll view. Without it the scroll view can shrink to nothing.
func (s *ScrollView) SetMinHeight(h float64) *ScrollView {
	s.check("SetMinHeight")
	s.n.Props().SetMin(layout.Vertical, h)
	s.invalidate()
	return s
}

// Offset returns the document point shown at the top-left corner of the
// visible area.
func (s *ScrollView) Offset() Point {
	s.get("Offset")
	if s.live() {
		s.off = s.p.Offset()
	}
	return s.off
}

// SetOffset scrolls to offset without animation. The offset is clamped to
// the document. It doesn't call the OnScroll handlers.
func (s *ScrollView) SetOffset(offset Point) *ScrollView {
	s.check("SetOffset")
	if s.live() {
		s.p.SetOffset(offset)
		s.off = s.p.Offset()
	}
	return s
}

// OnScroll calls fn with the new offset when the user scrolls (wheel,
// touchpad, keyboard or scroller, including momentum). SetOffset doesn't
// call it. As in AppKit, a scroll view scrolls, and reports it, even
// while it is disabled.
func (s *ScrollView) OnScroll(fn func(offset Point)) Unsubscribe {
	s.check("OnScroll")
	return s.onScroll.add(fn)
}

func (s *ScrollView) childWidgets() []Widget { return single(s.doc) }

func (s *ScrollView) removeChild(w Widget) {
	if w == s.doc {
		s.setDocument(nil)
	}
}

// scrollEvents implements driver.ScrollHandler for a ScrollView.
type scrollEvents struct{ s *ScrollView }

func (e scrollEvents) Scrolled(offset driver.Point) {
	s := e.s
	if s.dead {
		return
	}
	s.off = offset
	s.onScroll.eachLive(&s.widget, func(fn func(Point)) { fn(offset) })
}

// ViewportResized marks the window dirty: the document size depends on
// the viewport.
func (e scrollEvents) ViewportResized(driver.Size) { e.s.invalidate() }
