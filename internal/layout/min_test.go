package layout

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
)

func TestMinSizeLeavesAndStacks(t *testing.T) {
	// A leaf's default min is its fitting size; an app-set min lowers it.
	// The height is the natural one, even when the leaf could shrink.
	a, _ := pl(50, 40)
	wantSize(t, "leaf", NewRoot(a).MinSize(), driver.Size{W: 50, H: 40})
	a.Props().SetMin(Horizontal, 20).SetMin(Vertical, 10)
	wantSize(t, "leaf with min", NewRoot(a).MinSize(), driver.Size{W: 20, H: 40})

	// VStack: widest child + padding; heights, spacing and padding add up.
	b, _ := pl(50, 10)
	c, _ := pl(30, 15)
	v := NewVStack(6, b, c).SetPadding(driver.UniformInsets(2))
	wantSize(t, "vstack", NewRoot(v).MinSize(), driver.Size{W: 54, H: 35})

	// HStack: the minima, spacing and padding add up; the tallest child.
	d, _ := pl(50, 10)
	d.Props().SetMin(Horizontal, 20)
	e, _ := pl(30, 15)
	h := NewHStack(4, d, e).SetPadding(driver.UniformInsets(3))
	wantSize(t, "hstack", NewRoot(h).MinSize(), driver.Size{W: 60, H: 21})

	// Hidden children are not counted and get no spacing.
	f, _ := pl(80, 7)
	f.Props().SetHidden(true)
	v.SetChildren(b, f, c)
	wantSize(t, "hidden child", NewRoot(v).MinSize(), driver.Size{W: 54, H: 35})

	// Spacers are 0 (or their min) but get spacing like any child.
	sp := NewSpacer()
	v.SetChildren(b, sp, c)
	wantSize(t, "spacer", NewRoot(v).MinSize(), driver.Size{W: 54, H: 41})
	sp.Props().SetMin(Vertical, 10)
	wantSize(t, "spacer min", NewRoot(v).MinSize(), driver.Size{W: 54, H: 51})

	v.Props().SetHidden(true)
	wantSize(t, "hidden root", NewRoot(v).MinSize(), driver.Size{})
}

func TestMinSizeFormBoxScroll(t *testing.T) {
	// The label column (63, "Password:") + gap + the fields' 96 pt minimum;
	// two 22 pt rows 8 pt apart.
	name, pass := stub.NewTextField(false, nil), stub.NewTextField(true, nil)
	form := NewForm(Row{Label: lbl("Name:"), Control: ctl(name)},
		Row{Label: lbl("Password:"), Control: ctl(pass)})
	wantSize(t, "form", NewRoot(form).MinSize(), driver.Size{W: 167, H: 52})

	// A titled box is its content's minimum plus 8 pt insets and the title.
	content, _ := pl(50, 20)
	box, _ := titledBox("Title", content)
	wantSize(t, "box", NewRoot(box).MinSize(), driver.Size{W: 66, H: 52})

	// A scroll view is sized only by its props, never by its document.
	doc, _ := pl(300, 500)
	sv := NewScrollView(stub.NewScrollView(nil), doc).SetScrolls(Vertical, true)
	wantSize(t, "scroll view", NewRoot(sv).MinSize(), driver.Size{})
	sv.Props().SetMin(Horizontal, 180)
	row := NewHStack(0, sv, ctl(stub.NewTextView(nil)))
	wantSize(t, "sidebar", NewRoot(row).MinSize(), driver.Size{W: 180})
}

func TestMinSizeWrappingLabel(t *testing.T) {
	lb := stub.NewLabel()
	lb.Text, lb.Wraps = "0123456789", true // 70 × 16 on one line
	l := NewLeaf(lb)
	s := NewVStack(0, l).SetPadding(driver.UniformInsets(5))
	wantSize(t, "one line", NewRoot(s).MinSize(), driver.Size{W: 80, H: 26})
	// With an app-set min width of 30 the label needs ceil(70/30) = 3
	// lines at the minimum width, so the window must be that tall.
	l.Props().SetMin(Horizontal, 30)
	wantSize(t, "wrapped", NewRoot(s).MinSize(), driver.Size{W: 40, H: 58})
}

// deepTree nests a probe leaf depth levels deep in alternating HStacks and
// VStacks, each with one more probe leaf. It returns the root node and
// every probe, the deepest first.
func deepTree(depth int) (Node, []*probe) {
	leaf, p := pl(10, 10)
	var n Node = leaf
	probes := []*probe{p}
	for i := 0; i < depth; i++ {
		side, ps := pl(5, 5)
		probes = append(probes, ps)
		if i%2 == 0 {
			n = NewHStack(2, n, side)
		} else {
			n = NewVStack(2, n, side)
		}
	}
	return n, probes
}

func measures(ps []*probe) (n int) {
	for _, p := range ps {
		n += p.measures
	}
	return n
}

func TestMeasureCache(t *testing.T) {
	// Without the cache (nil *Pass) every level measures its subtree about
	// three times, so the deepest leaf is measured thousands of times.
	n, ps := deepTree(8)
	n.Arrange(nil, driver.Rect{W: 200, H: 200})
	if got := ps[0].measures; got < 1000 {
		t.Fatalf("uncached deepest leaf: %d FittingSize calls, want the blow-up", got)
	}
	// With the cache the count no longer depends on the depth.
	for _, depth := range []int{2, 4, 8, 16} {
		n, ps := deepTree(depth)
		NewRoot(n).Layout(driver.Size{W: 200, H: 200})
		if got := ps[0].measures; got > 9 {
			t.Errorf("depth %d: deepest leaf measured %d times, want at most 9", depth, got)
		}
		if got := measures(ps); got > 9*len(ps) {
			t.Errorf("depth %d: %d FittingSize calls for %d leaves", depth, got, len(ps))
		}
	}

	// Within one pass the same node and max are measured once, and the
	// leaf's unbounded fitting size is asked once (P7).
	l, pr := pl(30, 10)
	var p Pass
	entries := func() int {
		e, ok := p.lookup(l.Props())
		if !ok {
			return 0
		}
		return e.n
	}
	p.Measure(l, driver.Size{W: 50, H: 50})
	before := pr.measures
	p.Measure(l, driver.Size{W: 50, H: 50})
	if pr.measures != before || entries() != 1 {
		t.Error("the same measurement was not cached")
	}
	p.Measure(l, driver.Size{W: 40, H: 50})
	if entries() != 2 {
		t.Error("a different max must be measured again")
	}
	if pr.measures != before {
		t.Errorf("the unbounded fitting size was asked %d more times", pr.measures-before)
	}
	p.Measure(l, driver.Size{W: -1, H: 50}) // normalized: same key as W 0
	p.Measure(l, driver.Size{W: 0, H: 50})
	if entries() != 3 {
		t.Errorf("normalized keys: %d measurements, want 3", entries())
	}
	p.reset()
	if entries() != 0 {
		t.Error("reset kept a measurement")
	}
	before = pr.measures
	p.Measure(l, driver.Size{W: 50, H: 50})
	if pr.measures != before+1 {
		t.Error("the fitting size survived reset")
	}
	var nilPass *Pass
	before = pr.measures
	nilPass.Measure(l, driver.Size{W: 50, H: 50})
	nilPass.Measure(l, driver.Size{W: 50, H: 50})
	if pr.measures != before+2 {
		t.Error("a nil *Pass must not cache")
	}
}

func TestMeasureCacheNotReused(t *testing.T) {
	lb := label("Hi", driver.Font{}) // 14 × 16
	fld := stub.NewTextField(false, nil)
	s, _ := withPeer(NewHStack(8, NewLeaf(lb), ctl(fld)))
	r := NewRoot(s)
	r.Layout(driver.Size{W: 200, H: 30})
	wantRect(t, "field", fld.Frame, driver.Rect{X: 22, Y: 4, W: 178, H: 22})
	// Every Layout and MinSize is a fresh pass: a changed label is seen.
	lb.Text = "Hello"
	r.Layout(driver.Size{W: 200, H: 30})
	wantRect(t, "field after change", fld.Frame, driver.Rect{X: 43, Y: 4, W: 157, H: 22})
	wantSize(t, "min after change", r.MinSize(), driver.Size{W: 35 + 8 + 96, H: 22})
	lb.Text = "Hello, world"
	wantSize(t, "min after second change", r.MinSize(), driver.Size{W: 84 + 8 + 96, H: 22})
}

// BenchmarkLayout1000 lays out 100 HStacks of 9 leaves each (900 leaves,
// about 1000 nodes) in a padded VStack.
func BenchmarkLayout1000(b *testing.B) {
	rows := make([]Node, 100)
	for i := range rows {
		kids := make([]Node, 9)
		for j := range kids {
			switch j % 3 {
			case 0:
				kids[j] = NewLeaf(label("Label", driver.Font{}))
			case 1:
				kids[j] = ctl(stub.NewTextField(false, nil))
			default:
				kids[j] = NewLeaf(button("OK"))
			}
		}
		rows[i] = NewHStack(8, kids...).SetPeer(stub.NewContainer())
	}
	root := NewRoot(NewVStack(6, rows...).SetPadding(driver.UniformInsets(20)))
	b.ReportAllocs()
	for b.Loop() {
		root.Layout(driver.Size{W: 1600, H: 3000})
	}
}

// TestLayoutMin: LayoutMin frames the tree as Layout does and returns
// MinSize's result, measuring each leaf no more often than Layout and
// MinSize together, and less where their measurements meet (S6.2e: the
// window's layout pass shares one Pass).
func TestLayoutMin(t *testing.T) {
	tree := func() (*Root, []*probe) {
		a, pa := pl(50, 10)
		b, pb := pl(30, 15)
		c, pc := pl(80, 20)
		c.Props().SetMin(Horizontal, 20)
		v := NewVStack(6, NewHStack(4, a, b), c).SetPadding(driver.UniformInsets(2))
		return NewRoot(v), []*probe{pa, pb, pc}
	}
	size := driver.Size{W: 300, H: 200}
	sep, sp := tree()
	sep.Layout(size)
	wantMin := sep.MinSize()
	one, op := tree()
	gotMin := one.LayoutMin(size)
	wantSize(t, "LayoutMin", gotMin, wantMin)
	sepN, oneN := 0, 0
	for i := range sp {
		if op[i].frame != sp[i].frame {
			t.Errorf("leaf %d: frame %v, want %v", i, op[i].frame, sp[i].frame)
		}
		sepN += sp[i].measures
		oneN += op[i].measures
		if op[i].measures > sp[i].measures {
			t.Errorf("leaf %d: measured %d times, Layout and MinSize %d", i, op[i].measures, sp[i].measures)
		}
	}
	if oneN >= sepN {
		t.Errorf("LayoutMin measured the leaves %d times, Layout and MinSize %d", oneN, sepN)
	}
}

// TestLayoutMinExample: the controls example's tree, as TestLayoutMin.
func TestLayoutMinExample(t *testing.T) {
	size := driver.Size{W: 520, H: 560}
	sep := layoutControls(size).root
	want := sep.MinSize()
	one := layoutControls(size).root
	wantSizeNear(t, "controls LayoutMin", one.LayoutMin(size), want)
}
