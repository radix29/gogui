package layout

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
)

// pl returns a leaf over a probe with fitting size w×h.
func pl(w, h float64) (*Leaf, *probe) {
	p := &probe{fit: driver.Size{W: w, H: h}}
	return NewLeaf(p), p
}

func wantFrame(t *testing.T, name string, p *probe, want driver.Rect) {
	t.Helper()
	if !rectNear(p.frame, want) {
		t.Errorf("%s: frame = %v, want %v", name, p.frame, want)
	}
}

func wantSize(t *testing.T, name string, got, want driver.Size) {
	t.Helper()
	if !sizeNear(got, want) {
		t.Errorf("%s: size = %v, want %v", name, got, want)
	}
}

func TestStackSpacingPaddingPeer(t *testing.T) {
	a, pa := pl(50, 20)
	b, pb := pl(30, 10)
	s := NewVStack(10, a, b).SetPadding(driver.Insets{Top: 5, Left: 6, Bottom: 7, Right: 8})
	var p Pass
	wantSize(t, "vstack", p.Measure(s, driver.Unbounded()), driver.Size{W: 64, H: 52})
	s.Arrange(&p, driver.Rect{X: 100, Y: 200, W: 64, H: 52}) // transparent: offsets
	wantFrame(t, "a", pa, driver.Rect{X: 106, Y: 205, W: 50, H: 20})
	wantFrame(t, "b", pb, driver.Rect{X: 106, Y: 235, W: 30, H: 10})

	peer := stub.NewContainer()
	s.SetPeer(peer).Arrange(&p, driver.Rect{X: 100, Y: 200, W: 64, H: 52})
	if peer.Frame != (driver.Rect{X: 100, Y: 200, W: 64, H: 52}) || s.Peer() != peer {
		t.Errorf("peer frame = %v", peer.Frame)
	}
	wantFrame(t, "a in peer", pa, driver.Rect{X: 6, Y: 5, W: 50, H: 20})

	h := NewHStack(4, a, b) // default cross alignment: Center
	h.SetPeer(nil)
	wantSize(t, "hstack", p.Measure(h, driver.Unbounded()), driver.Size{W: 84, H: 20})
	h.Arrange(&p, driver.Rect{W: 84, H: 20})
	wantFrame(t, "ha", pa, driver.Rect{X: 0, Y: 0, W: 50, H: 20})
	wantFrame(t, "hb", pb, driver.Rect{X: 54, Y: 5, W: 30, H: 10})
}

func TestStackAlignment(t *testing.T) {
	// A 20×20 leaf in a VStack 100 wide and in an HStack 50 tall.
	aligns := []Align{AlignDefault, Leading, Center, Trailing, Fill}
	cases := []struct {
		main       Axis
		crossSpace float64
		pos        []float64
	}{
		{Vertical, 100, []float64{0, 0, 40, 80, 0}},   // default Leading
		{Horizontal, 50, []float64{15, 0, 15, 30, 0}}, // default Center
	}
	for _, c := range cases {
		cross := c.main.Cross()
		for i, al := range aligns {
			l, pr := pl(20, 20)
			l.Props().SetAlign(cross, al).SetAlign(c.main, Center) // main-axis align is ignored
			s := NewVStack(0, l)
			if c.main == Horizontal {
				s = NewHStack(0, l)
			}
			s.Arrange(nil, driver.RectOf(driver.Point{}, mk(c.main, 20, c.crossSpace)))
			ext := 20.0
			if al == Fill {
				ext = c.crossSpace
			}
			want := driver.RectOf(mkPt(c.main, 0, c.pos[i]), mk(c.main, 20, ext))
			if !rectNear(pr.frame, want) {
				t.Errorf("main %d align %d: frame %v, want %v", c.main, al, pr.frame, want)
			}
		}
	}
}

func TestStackExpansion(t *testing.T) {
	a, pa := pl(20, 10)
	b, pb := pl(20, 10)
	b.Props().SetExpand(Horizontal, true).SetExpand(Vertical, true)
	s := NewVStack(0, a, b)
	if !s.Expands(Horizontal) || !s.Expands(Vertical) {
		t.Error("expansion not propagated")
	}
	s.Arrange(nil, driver.Rect{W: 100, H: 100})
	wantFrame(t, "a", pa, driver.Rect{W: 20, H: 10})
	wantFrame(t, "b fills", pb, driver.Rect{Y: 10, W: 100, H: 90})

	b.Props().SetAlign(Horizontal, Trailing).SetMax(Vertical, 50) // explicit wins; max caps
	s.Arrange(nil, driver.Rect{W: 100, H: 100})
	wantFrame(t, "b trailing", pb, driver.Rect{X: 80, Y: 10, W: 20, H: 50})

	b.Props().SetHidden(true) // hidden children don't propagate
	if s.Expands(Horizontal) || s.Expands(Vertical) {
		t.Error("hidden child propagated")
	}
	if !NewHStack(0).SetChildren(NewSpacer()).Expands(Horizontal) {
		t.Error("spacer does not propagate")
	}
	if s.Props().SetExpand(Vertical, true); !s.Expands(Vertical) {
		t.Error("own flag ignored")
	}
}

func TestStackSpacers(t *testing.T) {
	b := NewLeaf(button("Greet"))
	h := NewHStack(8, NewSpacer(), b) // the hello button row
	if !h.Expands(Horizontal) || h.Expands(Vertical) {
		t.Error("spacer expansion")
	}
	wantSize(t, "row", h.Measure(nil, driver.Unbounded()), driver.Size{W: 71, H: 22})
	h.Arrange(nil, driver.Rect{W: 320, H: 22})
	if f := b.Item().(*stub.Button).Frame; f != (driver.Rect{X: 257, W: 63, H: 22}) {
		t.Errorf("greet = %v", f)
	}

	x, px := pl(20, 10)
	y, py := pl(20, 10)
	z, pz := pl(10, 10)
	z.Props().SetExpand(Horizontal, true)
	capped := NewSpacer()
	capped.Props().SetMax(Horizontal, 5)
	NewHStack(0, NewSpacer(), x, NewSpacer(), y, z, capped).Arrange(nil, driver.Rect{W: 125, H: 10})
	// 125 − 50 = 75 leftover; the capped spacer takes 5, the other three 70/3 each.
	third := 70.0 / 3
	wantFrame(t, "x", px, driver.Rect{X: third, W: 20, H: 10})
	wantFrame(t, "y", py, driver.Rect{X: 2*third + 20, W: 20, H: 10})
	wantFrame(t, "z", pz, driver.Rect{X: 2*third + 40, W: 10 + third, H: 10})

	sp := NewSpacer() // outside a stack: zero size, never expands
	if sp.Expands(Horizontal) || sp.Measure(nil, driver.Size{W: 100, H: 100}) != (driver.Size{}) {
		t.Error("lone spacer")
	}
	sp.Props().SetMin(Vertical, 7)
	sp.Arrange(nil, driver.Rect{W: 9})
	wantSize(t, "spacer min", sp.Measure(nil, driver.Unbounded()), driver.Size{H: 7})
	if sp.Props().SetHidden(true); sp.Measure(nil, driver.Unbounded()) != (driver.Size{}) {
		t.Error("hidden spacer")
	}
}

func TestStackHidden(t *testing.T) {
	a, pa := pl(20, 10)
	hid, ph := pl(40, 40)
	hid.Props().SetHidden(true)
	b, pb := pl(20, 10)
	s := NewVStack(10, a, hid, b)
	wantSize(t, "measure", s.Measure(nil, driver.Unbounded()), driver.Size{W: 20, H: 30})
	s.Arrange(nil, driver.Rect{W: 20, H: 30})
	wantFrame(t, "a", pa, driver.Rect{W: 20, H: 10})
	wantFrame(t, "b", pb, driver.Rect{Y: 20, W: 20, H: 10})
	if ph.measures != 0 || ph.frames != 0 {
		t.Errorf("hidden child: %d measures, %d frames", ph.measures, ph.frames)
	}

	a.Props().SetHidden(true)
	b.Props().SetHidden(true)
	s.SetPadding(driver.UniformInsets(3))
	wantSize(t, "all hidden", s.Measure(nil, driver.Unbounded()), driver.Size{W: 6, H: 6})
	s.Arrange(nil, driver.Rect{W: 6, H: 6})
	if pa.frames != 1 {
		t.Error("arranged a hidden child")
	}

	peer := stub.NewContainer()
	s.SetPeer(peer).Props().SetHidden(true)
	s.Arrange(nil, driver.Rect{W: 9, H: 9})
	if s.Measure(nil, driver.Unbounded()) != (driver.Size{}) || peer.Frame != (driver.Rect{}) || s.FirstLine(nil, driver.Unbounded()) != 0 {
		t.Error("hidden stack")
	}
}

func TestStackShrinkAndOverflow(t *testing.T) {
	a, pa := pl(50, 10)
	a.Props().SetMin(Horizontal, 20)
	b, pb := pl(50, 10)
	b.Props().SetMin(Horizontal, 40)
	s := NewHStack(0, a, b)
	wantSize(t, "min", s.Measure(nil, driver.Size{H: 10}), driver.Size{W: 60, H: 10})
	wantSize(t, "short", s.Measure(nil, driver.Size{W: 70, H: inf}), driver.Size{W: 70, H: 10})
	s.Arrange(nil, driver.Rect{W: 70, H: 10}) // deficit 30 over flex 30 + 10
	wantFrame(t, "a", pa, driver.Rect{W: 27.5, H: 10})
	wantFrame(t, "b", pb, driver.Rect{X: 27.5, W: 42.5, H: 10})
	s.Arrange(nil, driver.Rect{W: 40, H: 5}) // overflow at the trailing edges
	wantFrame(t, "a over", pa, driver.Rect{W: 20, H: 10})
	wantFrame(t, "b over", pb, driver.Rect{X: 20, W: 40, H: 10})

	s.Props().SetFixed(Horizontal, 200).SetMin(Vertical, 30)
	wantSize(t, "own props", s.Measure(nil, driver.Unbounded()), driver.Size{W: 200, H: 30})
}

func TestStackHeightForWidth(t *testing.T) {
	label := stub.NewLabel()
	label.Text, label.Wraps = "0123456789", true // 70 × 16, wraps
	l := NewLeaf(label)
	l.Props().SetMin(Horizontal, 30)
	v := NewVStack(0, l).SetPadding(driver.Insets{Left: 5, Right: 5})
	wantSize(t, "vstack", v.Measure(nil, driver.Size{W: 40, H: inf}), driver.Size{W: 40, H: 48})

	fixed, _ := pl(40, 10)
	h := NewHStack(0, l, fixed)
	wantSize(t, "hstack", h.Measure(nil, driver.Size{W: 70, H: inf}), driver.Size{W: 70, H: 48})
	h.Arrange(nil, driver.Rect{W: 70, H: 48})
	if label.Frame != (driver.Rect{W: 30, H: 48}) {
		t.Errorf("label = %v", label.Frame)
	}
}

func TestStackFirstLine(t *testing.T) {
	hid, _ := pl(10, 99)
	hid.Props().SetHidden(true)
	a, _ := pl(10, 18)
	b, _ := pl(10, 30)
	inner := NewHStack(0, a, b).SetPadding(driver.Insets{Top: 2})
	outer := NewVStack(0, hid, inner).SetPadding(driver.Insets{Top: 3})
	if got := firstLine(nil, outer, driver.Unbounded()); got != 23 {
		t.Errorf("nested first line = %v, want 23", got)
	}
	if got := firstLine(nil, b, driver.Unbounded()); got != 30 {
		t.Errorf("leaf first line = %v", got)
	}
	empty := NewVStack(0, hid).SetPadding(driver.Insets{Top: 4, Bottom: 1})
	if got := empty.FirstLine(nil, driver.Unbounded()); got != 5 {
		t.Errorf("empty first line = %v", got)
	}
	if got := firstLine(nil, hid, driver.Unbounded()); got != 0 {
		t.Errorf("hidden first line = %v", got)
	}
}

func TestStackSettersAndPanics(t *testing.T) {
	a, _ := pl(10, 10)
	s := NewHStack(-3, a).SetPadding(driver.Insets{Top: -1, Left: inf, Bottom: 2})
	if s.Spacing() != 0 || s.Padding() != (driver.Insets{Bottom: 2}) || s.Axis() != Horizontal ||
		len(s.Children()) != 1 {
		t.Errorf("getters: %v %v %v", s.Spacing(), s.Padding(), s.Axis())
	}
	wantSize(t, "measure", s.Measure(nil, driver.Unbounded()), driver.Size{W: 10, H: 12})
	defer func() {
		if recover() == nil {
			t.Error("nil child did not panic")
		}
	}()
	NewVStack(0, a, nil)
}
