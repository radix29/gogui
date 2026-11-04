package aqua

import (
	"math"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/layout"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/render/record"
)

// box is a test element: it fills its bounds with a color, optionally
// leaves canvas state behind, and can be round for HitSelf.
type box struct {
	Base
	name    string
	color   driver.Color
	round   bool
	fitting driver.Size
	paints  int
	leak    bool   // leaves a translation and clip behind in Paint
	onPaint func() // runs during Paint
}

func newBox(name string, x, y, w, h float64) *box {
	e := &box{name: name, color: driver.RGBA(1, 0, 0, 1)}
	e.Init(e)
	e.SetFrame(driver.Rect{X: x, Y: y, W: w, H: h})
	return e
}

func (e *box) FittingSize(driver.Size) driver.Size { return e.fitting }

func (e *box) Paint(c render.Canvas) {
	e.paints++
	if e.onPaint != nil {
		e.onPaint()
	}
	c.FillRoundedRect(e.Bounds(), render.Radii{}, render.Solid(e.color))
	if e.leak {
		c.Translate(1000, 1000)
		c.ClipRoundedRect(driver.Rect{W: 1, H: 1}, render.Radii{})
	}
}

func (e *box) HitSelf(p driver.Point) bool {
	if !e.round {
		return e.Base.HitSelf(p)
	}
	r := e.Bounds()
	cx, cy := r.W/2, r.H/2
	return math.Hypot(p.X-cx, p.Y-cy) < math.Min(cx, cy)
}

// Compile-time checks: an element is a layout.Item.
var (
	_ Element     = (*box)(nil)
	_ layout.Item = Element(nil)
)

func mustPanic(t *testing.T, what string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		if recover() == nil {
			t.Errorf("%s: no panic", what)
		}
	}()
	f()
}

func names(es []Element) string {
	var s []string
	for _, e := range es {
		s = append(s, e.(*box).name)
	}
	return strings.Join(s, " ")
}

func TestZeroBase(t *testing.T) {
	e := &box{}
	if e.Self() != nil || e.Parent() != nil || e.ChildCount() != 0 {
		t.Fatal("zero Base not detached")
	}
	if e.Hidden() || !e.Enabled() || e.ClipsChildren() || e.Dirty() {
		t.Fatal("zero Base flags wrong")
	}
	if !e.VisibleInTree() || !e.EnabledInTree() {
		t.Fatal("zero Base not visible and enabled")
	}
	if e.Base.FittingSize(driver.Unbounded()) != (driver.Size{}) {
		t.Fatal("Base.FittingSize not zero")
	}
	mustPanic(t, "Root before Init", func() { e.Root() })
	mustPanic(t, "AddChild before Init", func() { e.AddChild(newBox("c", 0, 0, 1, 1)) })
	mustPanic(t, "child before Init", func() { newBox("p", 0, 0, 1, 1).AddChild(&box{}) })
	mustPanic(t, "PaintTree before Init", func() { PaintTree(record.New(driver.Size{}), &box{}) })
	mustPanic(t, "HitTest before Init", func() { HitTest(&box{}, driver.Point{}) })
}

func TestInit(t *testing.T) {
	e := &box{}
	other := &box{}
	mustPanic(t, "Init(nil)", func() { e.Init(nil) })
	mustPanic(t, "Init(other)", func() { e.Init(other) })
	e.Init(e)
	if e.Self() != Element(e) || e.Root() != Element(e) {
		t.Fatal("Self/Root after Init")
	}
	mustPanic(t, "Init twice", func() { e.Init(e) })
}

func TestTree(t *testing.T) {
	p := newBox("p", 0, 0, 100, 100)
	a, b, c := newBox("a", 0, 0, 1, 1), newBox("b", 0, 0, 1, 1), newBox("c", 0, 0, 1, 1)
	p.AddChild(a)
	p.AddChild(b)
	p.InsertChild(0, c)
	if got := names(p.Children()); got != "c a b" {
		t.Fatalf("children %q", got)
	}
	if p.ChildCount() != 3 || p.Child(1) != Element(a) || p.IndexOf(b) != 2 {
		t.Fatal("ChildCount/Child/IndexOf")
	}
	if a.Parent() != Element(p) || a.Root() != Element(p) || p.Parent() != nil {
		t.Fatal("Parent/Root")
	}
	if p.IndexOf(nil) != -1 || p.IndexOf(newBox("x", 0, 0, 1, 1)) != -1 {
		t.Fatal("IndexOf of a stranger")
	}

	// Children returns a copy.
	cs := p.Children()
	cs[0] = nil
	if p.Child(0) != Element(c) {
		t.Fatal("Children not a copy")
	}

	// Moving within the same parent.
	p.AddChild(c) // c moves to the top
	if got := names(p.Children()); got != "a b c" {
		t.Fatalf("after AddChild(c): %q", got)
	}
	p.InsertChild(1, c)
	if got := names(p.Children()); got != "a c b" {
		t.Fatalf("after InsertChild(1, c): %q", got)
	}
	mustPanic(t, "InsertChild own child past end", func() { p.InsertChild(3, c) })
	mustPanic(t, "InsertChild -1", func() { p.InsertChild(-1, newBox("x", 0, 0, 1, 1)) })
	mustPanic(t, "InsertChild past end", func() { p.InsertChild(4, newBox("x", 0, 0, 1, 1)) })
	if got := names(p.Children()); got != "a c b" {
		t.Fatalf("a failed insert changed the tree: %q", got)
	}

	// Re-parenting removes from the old parent.
	q := newBox("q", 0, 0, 10, 10)
	q.AddChild(c)
	if got := names(p.Children()); got != "a b" || c.Parent() != Element(q) {
		t.Fatalf("re-parent: p has %q", got)
	}

	// Cycles and nils.
	q.AddChild(p)
	mustPanic(t, "self as child", func() { q.AddChild(q) })
	mustPanic(t, "ancestor as child", func() { a.AddChild(q) })
	mustPanic(t, "nil child", func() { p.AddChild(nil) })
	if a.Root() != Element(q) {
		t.Fatal("Root of a grandchild")
	}

	// RemoveChild and RemoveFromParent.
	if q.RemoveChild(a) {
		t.Fatal("RemoveChild of a grandchild")
	}
	if !p.RemoveChild(a) || a.Parent() != nil || p.IndexOf(a) != -1 {
		t.Fatal("RemoveChild")
	}
	if p.RemoveChild(a) {
		t.Fatal("RemoveChild twice")
	}
	a.RemoveFromParent() // root: nothing
	p.RemoveFromParent()
	if p.Parent() != nil || names(q.Children()) != "c" || names(p.Children()) != "b" {
		t.Fatal("RemoveFromParent keeps the subtree")
	}
}

func TestSetChildren(t *testing.T) {
	p := newBox("p", 0, 0, 100, 100)
	a, b, c, d := newBox("a", 0, 0, 1, 1), newBox("b", 0, 0, 1, 1), newBox("c", 0, 0, 1, 1), newBox("d", 0, 0, 1, 1)
	q := newBox("q", 0, 0, 1, 1)
	q.AddChild(d)
	p.SetChildren(a, b, c)
	p.SetChildren(c, d, a) // b dropped, d stolen from q, order changed
	if got := names(p.Children()); got != "c d a" {
		t.Fatalf("children %q", got)
	}
	if b.Parent() != nil || q.ChildCount() != 0 || d.Parent() != Element(p) {
		t.Fatal("old parents not updated")
	}
	for _, e := range []*box{a, c, d} {
		if e.Parent() != Element(p) {
			t.Fatalf("%s: wrong parent", e.name)
		}
	}

	// Panics leave the tree alone.
	mustPanic(t, "repeated", func() { p.SetChildren(a, b, a) })
	mustPanic(t, "nil", func() { p.SetChildren(a, nil) })
	mustPanic(t, "self", func() { p.SetChildren(p) })
	mustPanic(t, "uninitialised", func() { p.SetChildren(&box{}) })
	if got := names(p.Children()); got != "c d a" || b.Parent() != nil {
		t.Fatalf("a failed SetChildren changed the tree: %q", got)
	}

	// The same list does not invalidate.
	PaintTree(record.New(driver.Size{W: 100, H: 100}), p)
	p.SetChildren(c, d, a)
	if p.Dirty() {
		t.Fatal("same list invalidated")
	}
	p.SetChildren()
	if p.ChildCount() != 0 || a.Parent() != nil || !p.Dirty() {
		t.Fatal("SetChildren() did not clear")
	}
}

func TestFlags(t *testing.T) {
	p := newBox("p", 0, 0, 10, 10)
	c := newBox("c", 0, 0, 1, 1)
	p.AddChild(c)
	p.SetHidden(true)
	if !p.Hidden() || c.Hidden() || c.VisibleInTree() || p.VisibleInTree() {
		t.Fatal("hidden")
	}
	p.SetHidden(false)
	p.SetEnabled(false)
	if p.Enabled() || !c.Enabled() || c.EnabledInTree() {
		t.Fatal("disabled")
	}
	p.SetEnabled(true)
	if !c.EnabledInTree() || !c.VisibleInTree() {
		t.Fatal("re-enabled")
	}
	p.SetClipsChildren(true)
	if !p.ClipsChildren() {
		t.Fatal("clips")
	}
	if p.Frame() != (driver.Rect{W: 10, H: 10}) || c.Bounds() != (driver.Rect{W: 1, H: 1}) {
		t.Fatal("frame/bounds")
	}
	c.SetFrame(driver.Rect{X: 3, Y: 4, W: 5, H: 6})
	if c.Bounds() != (driver.Rect{W: 5, H: 6}) {
		t.Fatal("bounds ignore the origin")
	}
	var it layout.Item = c
	c.fitting = driver.Size{W: 7, H: 8}
	if it.FittingSize(driver.Unbounded()) != c.fitting {
		t.Fatal("FittingSize through layout.Item")
	}
}

// TestLayoutLeaf checks that the layout engine places an element.
func TestLayoutLeaf(t *testing.T) {
	e := newBox("e", 0, 0, 0, 0)
	e.fitting = driver.Size{W: 40, H: 20}
	r := layout.NewRoot(layout.NewVStack(0, layout.NewLeaf(e)))
	r.Layout(driver.Size{W: 100, H: 50})
	if e.Frame().W != 40 || e.Frame().H != 20 {
		t.Fatalf("frame %v", e.Frame())
	}
}

func TestInvalidate(t *testing.T) {
	root := newBox("root", 0, 0, 100, 100)
	mid := newBox("mid", 10, 10, 50, 50)
	leaf := newBox("leaf", 5, 5, 10, 10)
	root.AddChild(mid)
	mid.AddChild(leaf)
	calls := 0
	root.SetInvalidateFunc(func() { calls++ })
	paint := func() {
		t.Helper()
		c := record.New(driver.Size{W: 100, H: 100})
		PaintTree(c, root)
		if err := c.End(); err != nil {
			t.Fatal(err)
		}
	}
	paint()
	for _, e := range []*box{root, mid, leaf} {
		if e.Dirty() {
			t.Fatalf("%s dirty after paint", e.name)
		}
	}

	// Each change marks the path to the root and calls back once per frame.
	changes := []struct {
		name   string
		change func()
		same   func() // the same value again: no invalidation
	}{
		{"SetFrame", func() { leaf.SetFrame(driver.Rect{X: 6, Y: 5, W: 10, H: 10}) }, func() { leaf.SetFrame(driver.Rect{X: 6, Y: 5, W: 10, H: 10}) }},
		{"SetHidden", func() { leaf.SetHidden(true) }, func() { leaf.SetHidden(true) }},
		{"SetHidden(false)", func() { leaf.SetHidden(false) }, func() { leaf.SetHidden(false) }},
		{"SetEnabled", func() { leaf.SetEnabled(false) }, func() { leaf.SetEnabled(false) }},
		{"SetEnabled(true)", func() { leaf.SetEnabled(true) }, func() { leaf.SetEnabled(true) }},
		{"SetClipsChildren", func() { leaf.SetClipsChildren(true) }, func() { leaf.SetClipsChildren(true) }},
		{"Invalidate", func() { leaf.Invalidate() }, func() {}},
	}
	for _, tc := range changes {
		calls = 0
		tc.change()
		if !leaf.Dirty() || !mid.Dirty() || !root.Dirty() || calls != 1 {
			t.Fatalf("%s: dirty %v %v %v, calls %d", tc.name, leaf.Dirty(), mid.Dirty(), root.Dirty(), calls)
		}
		leaf.Invalidate()
		mid.Invalidate()
		if calls != 1 {
			t.Fatalf("%s: not coalesced (%d calls)", tc.name, calls)
		}
		paint()
		tc.same()
		if root.Dirty() || calls != 1 {
			t.Fatalf("%s: same value invalidated", tc.name)
		}
	}

	// A sibling's change marks only its own path.
	sib := newBox("sib", 0, 0, 1, 1)
	root.AddChild(sib)
	paint()
	sib.Invalidate()
	if mid.Dirty() || leaf.Dirty() || !root.Dirty() {
		t.Fatal("sibling path")
	}
	paint()

	// Tree changes invalidate the parents involved.
	calls = 0
	other := newBox("other", 0, 0, 1, 1)
	otherCalls := 0
	other.SetInvalidateFunc(func() { otherCalls++ })
	PaintTree(record.New(driver.Size{W: 1, H: 1}), other) // new elements start dirty
	other.AddChild(sib)                                   // removes sib from root
	if calls != 1 || otherCalls != 1 || !root.Dirty() || !other.Dirty() {
		t.Fatalf("re-parent: calls %d/%d", calls, otherCalls)
	}
	paint()
	PaintTree(record.New(driver.Size{W: 1, H: 1}), other)
	calls = 0
	sib.RemoveFromParent()
	root.RemoveChild(mid)
	if calls != 1 || otherCalls != 2 { // each parent once
		t.Fatalf("remove: calls %d/%d", calls, otherCalls)
	}

	// A detached subtree has its own root: mid's changes no longer reach
	// root, and mid has no callback.
	paint()
	calls = 0
	leaf.Invalidate()
	if root.Dirty() || !mid.Dirty() || calls != 0 {
		t.Fatal("detached subtree reached the old root")
	}
	// Re-attaching a dirty subtree invalidates the new parent.
	root.AddChild(mid)
	if !root.Dirty() || calls != 1 {
		t.Fatal("re-attach")
	}

	// Removing the callback.
	paint()
	root.SetInvalidateFunc(nil)
	leaf.Invalidate()
	if calls != 1 || !root.Dirty() {
		t.Fatal("nil callback")
	}
}

func TestInvalidateDuringPaint(t *testing.T) {
	root := newBox("root", 0, 0, 10, 10)
	anim := newBox("anim", 0, 0, 5, 5)
	root.AddChild(anim)
	calls := 0
	root.SetInvalidateFunc(func() { calls++ })
	anim.onPaint = func() { anim.Invalidate() }
	PaintTree(record.New(driver.Size{W: 10, H: 10}), root)
	if !anim.Dirty() || !root.Dirty() || calls != 1 { // the root started dirty, so only the paint calls back
		t.Fatalf("dirty %v %v, calls %d", anim.Dirty(), root.Dirty(), calls)
	}
}

func TestHiddenSubtreeCleaned(t *testing.T) {
	root := newBox("root", 0, 0, 10, 10)
	h := newBox("h", 0, 0, 5, 5)
	c := newBox("c", 0, 0, 1, 1)
	root.AddChild(h)
	h.AddChild(c)
	h.SetHidden(true)
	PaintTree(record.New(driver.Size{W: 10, H: 10}), root)
	if h.Dirty() || c.Dirty() || root.Dirty() || h.paints != 0 || c.paints != 0 {
		t.Fatal("hidden subtree painted or left dirty")
	}
	// The root itself hidden: nothing drawn, all clean.
	root.SetHidden(true)
	rc := record.New(driver.Size{W: 10, H: 10})
	PaintTree(rc, root)
	if len(rc.Ops()) != 0 || root.Dirty() {
		t.Fatal("hidden root painted")
	}
}
