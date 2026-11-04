package layout

import (
	"math"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
)

type viewportEvents struct{ got []driver.Size }

func (v *viewportEvents) Scrolled(driver.Point)         {}
func (v *viewportEvents) ViewportResized(s driver.Size) { v.got = append(v.got, s) }

// badPeer reports bad insets and a bad viewport, and records the document
// size.
type badPeer struct{ doc driver.Size }

func (b *badPeer) SetFrame(driver.Rect) {}
func (b *badPeer) ContentInsets() driver.Insets {
	return driver.Insets{Top: -5, Left: math.NaN(), Bottom: math.Inf(1), Right: 4}
}
func (b *badPeer) ViewportSize() driver.Size     { return driver.Size{W: math.NaN(), H: math.Inf(1)} }
func (b *badPeer) SetDocumentSize(s driver.Size) { b.doc = s }

func TestBoxInsets(t *testing.T) {
	peer := stub.NewBox()
	content, pc := pl(50, 20)
	b := NewBox(peer, content)
	var p Pass
	wantSize(t, "untitled", p.Measure(b, driver.Unbounded()), driver.Size{W: 66, H: 36})
	b.Arrange(&p, driver.Rect{X: 10, Y: 20, W: 100, H: 60})
	wantRect(t, "peer", peer.Frame, driver.Rect{X: 10, Y: 20, W: 100, H: 60})
	wantFrame(t, "content", pc, driver.Rect{X: 0, Y: 0, W: 84, H: 44}) // size changes, not origin

	peer.SetTitle("Buttons") // the insets are read again: 16 more on top
	p = Pass{}               // a changed tree needs a fresh pass
	wantSize(t, "titled", p.Measure(b, driver.Unbounded()), driver.Size{W: 66, H: 52})
	b.Arrange(&p, driver.Rect{W: 100, H: 60})
	wantFrame(t, "titled content", pc, driver.Rect{W: 84, H: 28})

	if b.Peer() != BoxPeer(peer) || b.Content() != Node(content) {
		t.Error("getters")
	}
	wantSize(t, "bad insets", p.Measure(NewBox(&badPeer{}, content), driver.Unbounded()),
		driver.Size{W: 54, H: 20})
}

func TestBoxWithoutContent(t *testing.T) {
	peer := stub.NewBox()
	b := NewBox(peer, nil)
	var p Pass
	wantSize(t, "nil content", p.Measure(b, driver.Unbounded()), driver.Size{W: 16, H: 16})
	b.Arrange(&p, driver.Rect{W: 40, H: 30})
	wantRect(t, "peer", peer.Frame, driver.Rect{W: 40, H: 30})

	content, pc := pl(50, 20)
	content.Props().SetExpand(Horizontal, true).SetHidden(true)
	b.SetContent(content)
	wantSize(t, "hidden content", p.Measure(b, driver.Unbounded()), driver.Size{W: 16, H: 16})
	b.Arrange(&p, driver.Rect{W: 40, H: 30})
	if pc.measures != 0 || pc.frames != 0 || b.Expands(Horizontal) {
		t.Errorf("hidden content: %d measures, %d frames, expands %v", pc.measures, pc.frames, b.Expands(Horizontal))
	}

	b.Props().SetHidden(true)
	peer.Frame = driver.Rect{}
	b.Arrange(&p, driver.Rect{W: 40, H: 30})
	if peer.Frame != (driver.Rect{}) || p.Measure(b, driver.Unbounded()) != (driver.Size{}) {
		t.Error("a hidden box must be neither measured nor framed")
	}
	if b.Measure(&p, driver.Unbounded()) != (driver.Size{}) {
		t.Error("hidden box: direct Measure")
	}
}

func TestBoxExpansionAndMinimum(t *testing.T) {
	// A box around a text field fills a VStack's width by propagation.
	field := stub.NewTextField(false, nil)
	b := NewBox(stub.NewBox(), ctl(field))
	if !b.Expands(Horizontal) || b.Expands(Vertical) {
		t.Errorf("expands = %v, %v", b.Expands(Horizontal), b.Expands(Vertical))
	}
	own := NewBox(stub.NewBox(), nil)
	own.Props().SetExpand(Vertical, true)
	if !own.Expands(Vertical) {
		t.Error("own flag")
	}
	v, peer := withPeer(NewVStack(0, b))
	NewRoot(v).Layout(driver.Size{W: 200, H: 100})
	wantRect(t, "stack", peer.Frame, driver.Rect{W: 200, H: 100})
	wantRect(t, "field", field.Frame, driver.Rect{W: 184, H: 22})

	// The minimum is the content's minimum plus the insets, not the
	// peer's FittingSize; the height follows the inner width.
	wrap := label("aaaaaaaaaa", driver.Font{}) // 70 pt on one line
	wrap.Wraps = true
	l := NewLeaf(wrap)
	l.Props().SetMin(Horizontal, 30)
	wb := NewBox(stub.NewBox(), l)
	var p Pass
	wantSize(t, "natural", p.Measure(wb, driver.Unbounded()), driver.Size{W: 86, H: 32})
	wantSize(t, "minimum", p.Measure(wb, driver.Size{}), driver.Size{W: 46, H: 64})                          // 30 pt: 3 lines
	wantSize(t, "three lines", p.Measure(wb, driver.Size{W: 49, H: math.Inf(1)}), driver.Size{W: 49, H: 64}) // 33 pt
	wantSize(t, "two lines", p.Measure(wb, driver.Size{W: 51, H: 0}), driver.Size{W: 51, H: 48})             // 35 pt, overflows H
}

func TestNewHostNilPeerPanics(t *testing.T) {
	for name, f := range map[string]func(){
		"box":    func() { NewBox(nil, nil) },
		"scroll": func() { NewScrollView(nil, nil) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: no panic", name)
				}
			}()
			f()
		}()
	}
}

func TestScrollViewMeasure(t *testing.T) {
	doc, pd := pl(300, 500)
	doc.Props().SetExpand(Horizontal, true).SetExpand(Vertical, true)
	s := NewScrollView(stub.NewScrollView(nil), doc)
	var p Pass
	wantSize(t, "no min", p.Measure(s, driver.Unbounded()), driver.Size{})
	s.Props().SetMin(Horizontal, 180)
	p = Pass{}
	wantSize(t, "min width", p.Measure(s, driver.Unbounded()), driver.Size{W: 180})
	wantSize(t, "overflow", p.Measure(s, driver.Size{W: 100, H: 50}), driver.Size{W: 180})
	if pd.measures != 0 || s.Expands(Horizontal) || s.Expands(Vertical) {
		t.Errorf("the document must not size the scroll view: %d measures", pd.measures)
	}
	s.Props().SetExpand(Vertical, true)
	if !s.Expands(Vertical) || s.Expands(Horizontal) {
		t.Error("own flag")
	}

	// The notes sidebar: 180 pt wide next to an expanding text view.
	tv := stub.NewTextView(nil)
	h, peer := withPeer(NewHStack(0, s, ctl(tv)))
	NewRoot(h).Layout(driver.Size{W: 500, H: 300})
	wantRect(t, "hstack", peer.Frame, driver.Rect{W: 500, H: 300})
	wantRect(t, "sidebar", s.Peer().(*stub.ScrollView).Frame, driver.Rect{W: 180, H: 300})
	wantRect(t, "editor", tv.Frame, driver.Rect{X: 180, W: 320, H: 300})
}

func TestScrollViewDocumentSize(t *testing.T) {
	frame := driver.Rect{X: 10, Y: 10, W: 100, H: 200}
	cases := []struct {
		name       string
		h, v       bool
		doc, want  driver.Size
		viewportIn driver.Insets
	}{
		{"vertical long", false, true, driver.Size{W: 50, H: 500}, driver.Size{W: 100, H: 500}, driver.Insets{}},
		{"vertical short", false, true, driver.Size{W: 50, H: 20}, driver.Size{W: 100, H: 200}, driver.Insets{}},
		{"horizontal long", true, false, driver.Size{W: 300, H: 20}, driver.Size{W: 300, H: 200}, driver.Insets{}},
		{"horizontal short", true, false, driver.Size{W: 50, H: 20}, driver.Size{W: 100, H: 200}, driver.Insets{}},
		{"both long", true, true, driver.Size{W: 300, H: 500}, driver.Size{W: 300, H: 500}, driver.Insets{}},
		{"both short", true, true, driver.Size{W: 50, H: 20}, driver.Size{W: 100, H: 200}, driver.Insets{}},
		{"neither", false, false, driver.Size{W: 300, H: 500}, driver.Size{W: 100, H: 200}, driver.Insets{}},
		{"legacy scrollers", false, true, driver.Size{W: 50, H: 500}, driver.Size{W: 85, H: 500},
			driver.Insets{Right: 15, Bottom: 15}},
		{"legacy short", true, true, driver.Size{W: 50, H: 20}, driver.Size{W: 85, H: 185},
			driver.Insets{Right: 15, Bottom: 15}},
	}
	for _, c := range cases {
		peer := stub.NewScrollView(nil)
		peer.ViewportInset = c.viewportIn
		doc, pd := pl(c.doc.W, c.doc.H)
		s := NewScrollView(peer, doc).SetScrolls(Horizontal, c.h).SetScrolls(Vertical, c.v)
		if s.Scrolls(Horizontal) != c.h || s.Scrolls(Vertical) != c.v || s.Document() != Node(doc) {
			t.Errorf("%s: getters", c.name)
		}
		s.Arrange(nil, frame)
		wantRect(t, c.name+" peer", peer.Frame, frame)
		wantSize(t, c.name+" document", peer.DocumentSize, c.want)
		wantFrame(t, c.name+" doc frame", pd, driver.RectOf(driver.Point{}, c.want))
	}
}

func TestScrollViewDocumentHeightForWidth(t *testing.T) {
	// A vertically scrolling document is measured at the viewport width.
	wrap := label("aaaaaaaaaa", driver.Font{}) // 70 pt
	wrap.Wraps = true
	l := NewLeaf(wrap)
	l.Props().SetMin(Horizontal, 30)
	peer := stub.NewScrollView(nil)
	NewScrollView(peer, l).SetScrolls(Vertical, true).Arrange(nil, driver.Rect{W: 35, H: 20})
	wantSize(t, "document", peer.DocumentSize, driver.Size{W: 35, H: 32}) // two lines
	wantRect(t, "label", wrap.Frame, driver.Rect{W: 35, H: 32})
}

func TestScrollViewWithoutDocument(t *testing.T) {
	ev := &viewportEvents{}
	peer := stub.NewScrollView(ev)
	s := NewScrollView(peer, nil).SetScrolls(Vertical, true)
	s.Arrange(nil, driver.Rect{W: 100, H: 200})
	wantSize(t, "no document", peer.DocumentSize, driver.Size{W: 100, H: 200})

	doc, pd := pl(50, 500)
	doc.Props().SetHidden(true)
	s.SetDocument(doc)
	peer.UserResizeViewport(driver.Insets{Right: 15})
	if len(ev.got) != 1 || ev.got[0] != (driver.Size{W: 85, H: 200}) {
		t.Errorf("ViewportResized = %v", ev.got)
	}
	s.Arrange(nil, driver.Rect{W: 100, H: 200}) // what S0.5 does on ViewportResized
	wantSize(t, "hidden document", peer.DocumentSize, driver.Size{W: 85, H: 200})
	if pd.measures != 0 || pd.frames != 0 || len(ev.got) != 1 {
		t.Errorf("hidden document: %d measures, %d frames, %d events", pd.measures, pd.frames, len(ev.got))
	}

	bad := &badPeer{}
	NewScrollView(bad, doc).Arrange(nil, driver.Rect{W: 10, H: 10})
	wantSize(t, "bad viewport", bad.doc, driver.Size{})

	s.Props().SetHidden(true)
	peer.Frame, peer.DocumentSize = driver.Rect{}, driver.Size{}
	s.Arrange(nil, driver.Rect{W: 100, H: 200})
	if peer.Frame != (driver.Rect{}) || peer.DocumentSize != (driver.Size{}) ||
		s.Measure(nil, driver.Unbounded()) != (driver.Size{}) {
		t.Error("a hidden scroll view must be neither measured nor framed")
	}
}
