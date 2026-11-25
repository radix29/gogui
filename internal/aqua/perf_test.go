package aqua

import (
	"fmt"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
)

// The tests of the aqua perf items (review W13: P8, P9, P10, P12, P13,
// P14, P15, P18). TestFrameAllocs (package gogui) guards their effect on a
// whole frame.

// TestChildrenCopyOnWrite (P8): a slice of children read before a change
// keeps its contents, so PaintTree needs no copy, and a Paint that changes
// the tree paints the children it started with.
func TestChildrenCopyOnWrite(t *testing.T) {
	root := newBox("root", 0, 0, 100, 100)
	a, b, c := newBox("a", 0, 0, 10, 10), newBox("b", 10, 0, 10, 10), newBox("c", 20, 0, 10, 10)
	root.SetChildren(a, b)
	snap := root.children
	root.InsertChild(1, c)
	root.RemoveChild(a)
	if len(snap) != 2 || snap[0] != Element(a) || snap[1] != Element(b) {
		t.Fatalf("snapshot changed: %v", snap)
	}
	if got := root.Children(); len(got) != 2 || got[0] != Element(c) || got[1] != Element(b) {
		t.Fatalf("children %v", got)
	}

	// a's Paint moves c in front of b and drops b: this pass still paints
	// a and b, not c.
	root.SetChildren(a, b)
	a.onPaint = func() { root.SetChildren(a, c) }
	dump(t, root, driver.Size{W: 100, H: 100})
	if a.paints != 1 || b.paints != 1 || c.paints != 0 {
		t.Fatalf("paints a %d b %d c %d", a.paints, b.paints, c.paints)
	}
}

// TestSetChildrenValidatesRouterOnce (P8): moving many children out of
// one routed tree validates that router once, and the moved focus is
// dropped.
func TestSetChildrenValidatesRouterOnce(t *testing.T) {
	src := newBox("src", 0, 0, 100, 100)
	r := NewRouter(src)
	var cs []Element
	for i := range 50 {
		b := newBox(fmt.Sprint(i), 0, 0, 1, 1)
		b.SetFocusable(true)
		src.AddChild(b)
		cs = append(cs, b)
	}
	r.SetFocus(cs[7])
	dst := newBox("dst", 0, 0, 100, 100)
	dst.SetChildren(cs...)
	if r.Focused() != nil || src.ChildCount() != 0 || dst.ChildCount() != 50 {
		t.Fatalf("focus %v, src %d, dst %d", r.Focused(), src.ChildCount(), dst.ChildCount())
	}
}

// TestTreeCacheFollowsChanges (P9): the memoised root, origin, enabled and
// visible state follow every change that alters them.
func TestTreeCacheFollowsChanges(t *testing.T) {
	root := newBox("root", 1, 2, 200, 200)
	a := newBox("a", 10, 10, 100, 100)
	b := newBox("b", 5, 5, 10, 10)
	root.AddChild(a)
	a.AddChild(b)
	at := func(want driver.Point) {
		t.Helper()
		if got := ToRoot(b, driver.Point{X: 1, Y: 1}); got != want.Add(driver.Point{X: 1, Y: 1}) {
			t.Fatalf("ToRoot %v, want %v", got, want)
		}
		if got := FromRoot(b, want); got != (driver.Point{}) {
			t.Fatalf("FromRoot %v", got)
		}
	}
	at(driver.Point{X: 16, Y: 17})
	a.SetFrame(driver.Rect{X: 20, Y: 30, W: 100, H: 100})
	at(driver.Point{X: 26, Y: 37})

	root.SetEnabled(false)
	if b.EnabledInTree() {
		t.Fatal("enabled under a disabled root")
	}
	root.SetEnabled(true)
	if !b.EnabledInTree() {
		t.Fatal("disabled after the root was enabled")
	}
	a.SetHidden(true)
	if b.VisibleInTree() {
		t.Fatal("visible under a hidden parent")
	}
	a.SetHidden(false)
	if !b.VisibleInTree() {
		t.Fatal("hidden after the parent was shown")
	}

	if b.routedBy() != nil {
		t.Fatal("a router before NewRouter")
	}
	r := NewRouter(root)
	if b.routedBy() != r || b.Root() != Element(root) {
		t.Fatal("not routed by the root's router")
	}
	root.AddChild(b) // re-parented
	at(driver.Point{X: 6, Y: 7})
	a.SetEnabled(false)
	if !b.EnabledInTree() {
		t.Fatal("disabled by its old parent")
	}
	b.RemoveFromParent()
	if b.routedBy() != nil || b.Root() != Element(b) {
		t.Fatal("still routed after leaving the tree")
	}
	at(driver.Point{X: 5, Y: 5})
	other := newBox("other", 100, 100, 50, 50)
	other.InsertChild(0, b)
	at(driver.Point{X: 105, Y: 105})
	other.SetChildren()
	at(driver.Point{X: 5, Y: 5})
}

// TestSpinnerRepaintsPerSpoke (P10): an indeterminate spinner repaints
// once per spoke step (12 a second), not on every frame, and builds its
// spoke paths once; the bar still moves every frame.
func TestSpinnerRepaintsPerSpoke(t *testing.T) {
	indetRun(t, driver.Rect{X: 10, Y: 10, W: 32, H: 32}, func(e *indetEnv) {
		e.p.SetStyle(driver.ProgressSpinner)
		e.p.SetIndeterminate(true)
		e.w.Show()
		e.clk.Flush()
		e.clk.Advance(time.Second)
		spokes := e.p.view.spokes
		paints := e.sf.Paints()
		e.clk.Advance(time.Second)
		if n := e.sf.Paints() - paints; n < 11 || n > 13 {
			t.Errorf("spinner: %d paints in a second, want 12", n)
		}
		if len(spokes) != 12 || &e.p.view.spokes[0] != &spokes[0] {
			t.Error("spoke paths rebuilt")
		}
		e.p.SetStyle(driver.ProgressBar)
		e.clk.Advance(time.Second)
		paints = e.sf.Paints()
		e.clk.Advance(time.Second)
		if n := e.sf.Paints() - paints; n < 55 {
			t.Errorf("bar: %d paints in a second, want 60", n)
		}
	})
}

// TestLabelTwoLayouts (P13): a label keeps the layouts of two widths (the
// measure's and the paint's), and a centred multi-line label lays out its
// lines once.
func TestLabelTwoLayouts(t *testing.T) {
	l := newLabel(t, "Hello world, a longer line\nand a second")
	l.SetWraps(true)
	a, b := l.view.layout(50), l.view.layout(80)
	for range 3 {
		if l.view.layout(50) != a || l.view.layout(80) != b {
			t.Fatal("alternating widths made new layouts")
		}
	}
	l.view.layout(90) // evicts 50, used less recently than 80
	if l.view.layout(80) != b {
		t.Fatal("the more recent entry was evicted")
	}
	if l.view.layout(50) == a {
		t.Fatal("the older entry was kept")
	}

	l.SetWraps(false)
	l.SetAlignment(driver.AlignCenter)
	l.SetFrame(driver.Rect{W: 300, H: 60})
	dump(t, l.view, driver.Size{W: 300, H: 60})
	ent := l.view.entry(300)
	lines := ent.lines
	if len(lines) != 2 {
		t.Fatalf("%d line layouts", len(lines))
	}
	dump(t, l.view, driver.Size{W: 300, H: 60})
	if ent := l.view.entry(300); len(ent.lines) != 2 || ent.lines[0].layout != lines[0].layout {
		t.Fatal("line layouts rebuilt")
	}
}

// groupProbe is a focusable element of a one-stop group; its group stop
// is the parent's first child, and calls counts the lookups.
type groupProbe struct {
	Base
	calls *int
}

func (g *groupProbe) groupStop() Element {
	*g.calls++
	return g.Parent().base().Child(0)
}

// TestTabOrderGroupStopOnce (P14): the Tab order finds the stop of a group
// once per parent, and only the stop is in the order.
func TestTabOrderGroupStopOnce(t *testing.T) {
	root := newBox("root", 0, 0, 100, 100)
	calls := 0
	var first Element
	for range 2 {
		box := newBox("group", 0, 0, 100, 100)
		for range 20 {
			g := &groupProbe{calls: &calls}
			g.Init(g)
			g.SetFocusable(true)
			box.AddChild(g)
		}
		root.AddChild(box)
		if first == nil {
			first = box.Child(0)
		}
	}
	r := NewRouter(root)
	r.SetFullKeyboardAccess(true)
	order := r.FocusOrder()
	if len(order) != 2 || order[0] != first || calls != 2 {
		t.Fatalf("order %d elements (first %v), %d group lookups", len(order), order[0] == first, calls)
	}
}

// TestHoverUnchangedNoAlloc (P15): a move that keeps the hover chain
// allocates nothing in the hit-test and the hover update.
func TestHoverUnchangedNoAlloc(t *testing.T) {
	w := newWindow(t)
	w.r.Event(move(25, 25))
	chain := w.r.HoverChain()
	p := driver.Point{X: 26, Y: 25}
	if n := testing.AllocsPerRun(100, func() { w.r.setHover(w.r.hoverAt(p)) }); n != 0 {
		t.Errorf("%v allocations per unchanged hover", n)
	}
	if got := w.r.HoverChain(); len(got) != len(chain) || got[len(got)-1] != chain[len(chain)-1] {
		t.Fatalf("chain changed: %v", got)
	}
	w.r.Event(move(160, 20)) // over sb: the chain changes
	if w.r.Hovered() != Element(w.sb) {
		t.Fatalf("hovered %v", w.r.Hovered())
	}
}

// TestMenuItemAtScan (P12): ItemAt by binary search finds the item whose
// rect holds the point, at every height, scrolled or not, for regular items
// and separators.
func TestMenuItemAtScan(t *testing.T) {
	s := startShell(t)
	m := newMenu(s.TextSystem())
	var items []menuItem
	for i := range 40 {
		if i%7 == 3 {
			items = append(items, menuItem{Separator: true})
			continue
		}
		items = append(items, menuItem{Title: fmt.Sprint("Item ", i)})
	}
	m.SetItems(items)
	fit := m.FittingSize(driver.Size{W: 1e6, H: 1e6})
	m.SetFrame(driver.Rect{W: fit.W, H: 200})
	for _, top := range []float64{0, 37.5, m.MaxScroll()} {
		m.SetScrollOffset(top)
		area := m.itemsArea()
		x := area.X + area.W/2
		for y := -5.0; y < 210; y += 0.25 {
			p := driver.Point{X: x, Y: y}
			want := -1
			if area.Contains(p) {
				for i := range items {
					if m.ItemRect(i).Contains(p) {
						want = i
						break
					}
				}
			}
			if got := m.ItemAt(p); got != want {
				t.Fatalf("scroll %g: ItemAt(%v) = %d, want %d", top, p, got, want)
			}
		}
	}
	if m.ContentHeight() != m.itemY(len(items))+tokens.Menu.PaddingY {
		t.Error("content height")
	}
}

// TestSecureFieldClusters (P18): a secure field maps offsets through its
// cluster table both ways, and the table follows an edit.
func TestSecureFieldClusters(t *testing.T) {
	f, _, _, _ := fieldWindow(t, true, nil)
	f.SetText("pé🇨🇭👨‍👩‍👧!")
	v := f.view
	starts := []int{0, 1, len("pé"), len("pé🇨🇭"), len("pé🇨🇭👨‍👩‍👧")}
	for i, off := range starts {
		if got := v.shownOffset(off); got != i*len(bullet) {
			t.Errorf("shownOffset(%d) = %d, want %d", off, got, i*len(bullet))
		}
		if got := v.editorOffset(i * len(bullet)); got != off {
			t.Errorf("editorOffset(%d) = %d, want %d", i*len(bullet), got, off)
		}
	}
	if got := v.editorOffset(5 * len(bullet)); got != len(v.text()) {
		t.Errorf("editorOffset past the end = %d", got)
	}
	f.Editor().Insert("x") // the caret is at the end
	if got := v.textLayout().Text(); got != "••••••" || v.text() != "pé🇨🇭👨‍👩‍👧!x" {
		t.Fatalf("after typing: layout %q, text %q", got, v.text())
	}
	if got := v.shownOffset(len(v.text())); got != 6*len(bullet) {
		t.Errorf("shownOffset(end) = %d", got)
	}
}
