package layout

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
)

func TestNestedPeersAndOffsets(t *testing.T) {
	// VStack(peer, pad 10) > HStack(pad 5) > VStack(spacing 2) > a, b.
	a, pa := pl(30, 10)
	b, pb := pl(20, 10)
	inner := NewVStack(2, a, b)
	mid := NewHStack(0, inner).SetPadding(driver.UniformInsets(5))
	outer, op := withPeer(NewVStack(0, mid).SetPadding(driver.UniformInsets(10)))
	root := NewRoot(outer)

	// Without peers in mid and inner, frames are offsets into outer's peer.
	root.Layout(driver.Size{W: 200, H: 100})
	wantRect(t, "outer", op.Frame, driver.Rect{W: 200, H: 100})
	wantFrame(t, "a", pa, driver.Rect{X: 15, Y: 15, W: 30, H: 10})
	wantFrame(t, "b", pb, driver.Rect{X: 15, Y: 27, W: 20, H: 10})

	// With peers, each level starts a new coordinate space.
	_, mp := withPeer(mid)
	_, ip := withPeer(inner)
	root.Layout(driver.Size{W: 200, H: 100})
	wantRect(t, "mid", mp.Frame, driver.Rect{X: 10, Y: 10, W: 40, H: 32})
	wantRect(t, "inner", ip.Frame, driver.Rect{X: 5, Y: 5, W: 30, H: 22})
	wantFrame(t, "a in peer", pa, driver.Rect{W: 30, H: 10})
	wantFrame(t, "b in peer", pb, driver.Rect{Y: 12, W: 20, H: 10})

	// A peer-less group between two peers offsets into the outer one.
	mid.SetPeer(nil)
	root.Layout(driver.Size{W: 200, H: 100})
	wantRect(t, "inner via group", ip.Frame, driver.Rect{X: 15, Y: 15, W: 30, H: 22})
	wantFrame(t, "a again", pa, driver.Rect{W: 30, H: 10})
}

func TestNestedExpansionPropagates(t *testing.T) {
	// Three levels: VStack > HStack > VStack > field (expands horizontally).
	fixed, pf := pl(40, 20)
	field := ctl(stub.NewTextField(false, nil))
	deep := NewVStack(0, field)
	mid := NewHStack(4, fixed, deep)
	top := NewVStack(0, mid)
	for i, n := range []Node{deep, mid, top} {
		if !n.Expands(Horizontal) || n.Expands(Vertical) {
			t.Errorf("level %d: expands %v/%v", i, n.Expands(Horizontal), n.Expands(Vertical))
		}
	}
	NewRoot(top).Layout(driver.Size{W: 300, H: 50})
	wantFrame(t, "fixed (centered)", pf, driver.Rect{Y: 1, W: 40, H: 20})
	// mid fills top's width (cross Fill), deep takes mid's leftover, the
	// field fills deep.
	if f := field.Item().(*stub.TextField).Frame; !rectNear(f, driver.Rect{X: 44, W: 256, H: 22}) {
		t.Errorf("field = %v", f)
	}

	// A text view deep down makes every level expand vertically too.
	tv := ctl(stub.NewTextView(nil))
	deep.SetChildren(field, tv)
	if !top.Expands(Vertical) {
		t.Error("vertical expansion not propagated")
	}
	NewRoot(top).Layout(driver.Size{W: 300, H: 200})
	if f := tv.Item().(*stub.TextView).Frame; !rectNear(f, driver.Rect{X: 44, Y: 22, W: 256, H: 178}) {
		t.Errorf("text view = %v", f)
	}
	tv.Props().SetHidden(true) // hidden subtrees stop propagating
	if top.Expands(Vertical) {
		t.Error("hidden child propagated")
	}
}

func TestNestedSpacerNextToExpanders(t *testing.T) {
	// HStack(0, field, Spacer, slider): the spacer shares the leftover
	// equally with both expanders.
	field := ctl(stub.NewTextField(false, nil))
	slider := ctl(stub.NewSlider(nil))
	NewRoot(NewHStack(0, field, NewSpacer(), slider)).Layout(driver.Size{W: 342, H: 22})
	// 342 − 2 × 96 = 150, so 50 each.
	if f := field.Item().(*stub.TextField).Frame; !rectNear(f, driver.Rect{W: 146, H: 22}) {
		t.Errorf("field = %v", f)
	}
	if f := slider.Item().(*stub.Slider).Frame; !rectNear(f, driver.Rect{X: 196, W: 146, H: 22}) {
		t.Errorf("slider = %v", f)
	}

	// Vertically: a spacer between a text view and a button row.
	tv := ctl(stub.NewTextView(nil))
	ok := ctl(button("OK")) // 2 × 7 + 28 = 42
	col := NewVStack(10, tv, NewSpacer(), NewHStack(0, NewSpacer(), ok))
	NewRoot(col).Layout(driver.Size{W: 100, H: 142})
	// 142 − 2 × 10 − 22 = 100 leftover, shared by the text view and spacer.
	if f := tv.Item().(*stub.TextView).Frame; !rectNear(f, driver.Rect{W: 100, H: 50}) {
		t.Errorf("text view = %v", f)
	}
	if f := ok.Item().(*stub.Button).Frame; !rectNear(f, driver.Rect{X: 58, Y: 120, W: 42, H: 22}) {
		t.Errorf("ok = %v", f)
	}
}

func TestNestedContainerMinimum(t *testing.T) {
	// Leaves 50 wide with min 20; inner HStack spacing 4, padding 3;
	// outer VStack spacing 6, padding 2.
	a, _ := pl(50, 10)
	a.Props().SetMin(Horizontal, 20)
	b, _ := pl(50, 10)
	b.Props().SetMin(Horizontal, 20).SetMin(Vertical, 4)
	c, _ := pl(30, 15)
	row := NewHStack(4, a, b).SetPadding(driver.UniformInsets(3))
	col := NewVStack(6, row, c, NewSpacer()).SetPadding(driver.UniformInsets(2))
	var p Pass
	// Natural: row 50+4+50+6 = 110 × 16; col 110+4 wide, 2+16+6+15+6+0+2 tall.
	wantSize(t, "natural", p.Measure(col, driver.Unbounded()), driver.Size{W: 114, H: 47})
	// Min width (width 0): row 20+4+20+6 = 50, col 54; heights unchanged.
	wantSize(t, "min width", p.Measure(col, driver.Size{W: 0, H: inf}), driver.Size{W: 54, H: 47})
	// Min height (height 0): b's min 4 doesn't lower the row (a is 10), so
	// the same 47; a min-height leaf would lower it.
	wantSize(t, "min height", p.Measure(col, driver.Size{W: inf, H: 0}), driver.Size{W: 114, H: 47})
	c.Props().SetMin(Vertical, 5)
	p = Pass{}
	wantSize(t, "min height c", p.Measure(col, driver.Size{W: inf, H: 0}), driver.Size{W: 114, H: 37})
	wantSize(t, "min both", p.Measure(col, driver.Size{}), driver.Size{W: 54, H: 37})
}

func TestRootGetsFullSize(t *testing.T) {
	a, pa := pl(30, 10)
	s, peer := withPeer(NewVStack(0, a))
	// Neither expanding nor in a parent: the root still fills the content.
	s.Props().SetMax(Horizontal, 50).SetMax(Vertical, 20)
	r := NewRoot(s)
	if r.Node() != s {
		t.Fatal("Node")
	}
	r.Layout(driver.Size{W: 400, H: 300})
	wantRect(t, "root", peer.Frame, driver.Rect{W: 400, H: 300})
	wantFrame(t, "a", pa, driver.Rect{W: 30, H: 10})

	// A leaf as root fills the content too, below its fitting size as well.
	l, pl2 := pl(30, 10)
	r.SetNode(l).Layout(driver.Size{W: 12, H: 5})
	wantFrame(t, "leaf root", pl2, driver.Rect{W: 12, H: 5})
	r.Layout(driver.Size{W: -3, H: inf}) // bad sizes count as 0
	wantFrame(t, "bad size", pl2, driver.Rect{})

	l.Props().SetHidden(true)
	r.Layout(driver.Size{W: 99, H: 99})
	if pl2.frames != 2 {
		t.Errorf("hidden root framed: %d frames", pl2.frames)
	}
	defer func() {
		if recover() == nil {
			t.Error("nil root did not panic")
		}
	}()
	NewRoot(nil)
}
