package gogui

import (
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/layout"
)

// Box is a titled group box (NSBox) around one content widget. It expands
// only when its content does.
type Box struct {
	common[*Box]
	p       driver.Box
	n       *layout.Box
	title   string
	content Widget
}

// NewBox returns a box with title and no content. An empty title hides
// the title line.
func NewBox(title string) *Box {
	p := driverFor("NewBox").NewBox()
	b := &Box{p: p, n: layout.NewBox(p, nil), title: title}
	b.init(b, kindBox, p, b.n)
	p.SetTitle(title)
	return b
}

// SetTitle sets the title shown above the content; an empty title hides
// it.
func (b *Box) SetTitle(title string) *Box {
	b.check("SetTitle")
	b.title = title
	if b.live() {
		b.p.SetTitle(title)
	}
	b.invalidate() // the title changes the content insets
	return b
}

// Title returns the title.
func (b *Box) Title() string { b.get("Title"); return b.title }

// SetContent sets the widget shown inside the box; nil removes it. A
// widget that is in a stack, a form, another box or scroll view, or a
// window is moved here. The old content is taken out but not destroyed.
func (b *Box) SetContent(w Widget) *Box {
	b.check("SetContent")
	if w == b.content {
		return b
	}
	if w != nil {
		adopt("Box.SetContent", b, []Widget{w})
	}
	b.setContent(w)
	return b
}

// Content returns the content widget, or nil.
func (b *Box) Content() Widget { b.get("Content"); return b.content }

// setContent puts w (nil for none) in the layout node and the peer. The
// caller has made b the parent of w.
func (b *Box) setContent(w Widget) {
	if old := b.content; old != nil && old.base().parent == Widget(b) {
		old.base().parent = nil
	}
	b.content = w
	n, peer := nodeAndPeer(w)
	b.n.SetContent(n)
	if b.live() {
		b.p.SetContent(peer)
	}
	b.invalidate()
}

func (b *Box) childWidgets() []Widget { return single(b.content) }

func (b *Box) removeChild(w Widget) {
	if w == b.content {
		b.setContent(nil)
	}
}

// single returns w as a list of children, empty for nil.
func single(w Widget) []Widget {
	if w == nil {
		return nil
	}
	return []Widget{w}
}

// nodeAndPeer returns w's layout node and peer, or nils for a nil w.
func nodeAndPeer(w Widget) (layout.Node, driver.Widget) {
	if w == nil {
		return nil, nil
	}
	return w.node(), w.peerWidget()
}
