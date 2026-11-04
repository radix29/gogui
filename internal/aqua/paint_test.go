package aqua

import (
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render/record"
)

func dump(t *testing.T, root Element, size driver.Size) string {
	t.Helper()
	c := record.New(size)
	PaintTree(c, root)
	if c.Depth() != 0 {
		t.Fatalf("depth %d after PaintTree", c.Depth())
	}
	if err := c.End(); err != nil {
		t.Fatal(err)
	}
	return c.Dump()
}

func checkDump(t *testing.T, got string, want ...string) {
	t.Helper()
	if w := strings.Join(want, "\n"); got != w {
		t.Fatalf("dump:\n%s\nwant:\n%s", got, w)
	}
}

func colored(name string, x, y, w, h float64, c driver.Color) *box {
	e := newBox(name, x, y, w, h)
	e.color = c
	return e
}

var (
	red   = driver.RGB8(0xff, 0, 0)
	green = driver.RGB8(0, 0xff, 0)
	blue  = driver.RGB8(0, 0, 0xff)
	grey  = driver.RGB8(0x80, 0x80, 0x80)
)

func TestPaintOrderAndTranslation(t *testing.T) {
	root := colored("root", 0, 0, 200, 100, grey)
	a := colored("a", 10, 10, 50, 50, red)
	b := colored("b", 40, 20, 50, 50, green) // overlaps a, on top
	inner := colored("inner", 5, 6, 10, 10, blue)
	root.SetChildren(a, b)
	a.AddChild(inner)
	checkDump(t, dump(t, root, driver.Size{W: 200, H: 100}),
		"fillRRect (0,0,200,100) solid #808080",
		"fillRRect (10,10,50,50) solid #ff0000",
		"fillRRect (15,16,10,10) solid #0000ff",
		"fillRRect (40,20,50,50) solid #00ff00",
	)

	// A root with an origin is painted at it (coordinates of its parent).
	root.SetFrame(driver.Rect{X: 3, Y: 4, W: 200, H: 100})
	root.SetChildren(b, a) // reorder: a now on top
	checkDump(t, dump(t, root, driver.Size{W: 300, H: 200}),
		"fillRRect (3,4,200,100) solid #808080",
		"fillRRect (43,24,50,50) solid #00ff00",
		"fillRRect (13,14,50,50) solid #ff0000",
		"fillRRect (18,20,10,10) solid #0000ff",
	)
}

func TestPaintHidden(t *testing.T) {
	root := colored("root", 0, 0, 100, 100, grey)
	a := colored("a", 0, 0, 10, 10, red)
	b := colored("b", 20, 0, 10, 10, green)
	c := colored("c", 1, 1, 5, 5, blue)
	root.SetChildren(a, b)
	a.AddChild(c)
	a.SetHidden(true)
	checkDump(t, dump(t, root, driver.Size{W: 100, H: 100}),
		"fillRRect (0,0,100,100) solid #808080",
		"fillRRect (20,0,10,10) solid #00ff00",
	)
	if a.paints != 0 || c.paints != 0 {
		t.Fatal("hidden subtree painted")
	}
	// Disabled elements still paint.
	a.SetHidden(false)
	a.SetEnabled(false)
	if d := dump(t, root, driver.Size{W: 100, H: 100}); !strings.Contains(d, "#0000ff") {
		t.Fatalf("disabled subtree not painted:\n%s", d)
	}
}

func TestPaintClip(t *testing.T) {
	root := colored("root", 0, 0, 100, 100, grey)
	clip := colored("clip", 10, 10, 30, 30, red)
	child := colored("child", 20, 20, 40, 40, blue) // sticks out of clip
	grand := colored("grand", -5, -5, 10, 10, green)
	root.AddChild(clip)
	clip.AddChild(child)
	child.AddChild(grand)

	checkDump(t, dump(t, root, driver.Size{W: 100, H: 100}),
		"fillRRect (0,0,100,100) solid #808080",
		"fillRRect (10,10,30,30) solid #ff0000",
		"fillRRect (30,30,40,40) solid #0000ff",
		"fillRRect (25,25,10,10) solid #00ff00",
	)

	clip.SetClipsChildren(true)
	// The element itself is not clipped; its children (and their children)
	// are clipped to its bounds in device space (10,10,30,30).
	checkDump(t, dump(t, root, driver.Size{W: 100, H: 100}),
		"fillRRect (0,0,100,100) solid #808080",
		"fillRRect (10,10,30,30) solid #ff0000",
		"fillRRect (30,30,40,40) solid #0000ff clip=(10,10,30,30)",
		"fillRRect (25,25,10,10) solid #00ff00 clip=(10,10,30,30)",
	)

	// Nested clips intersect; a sibling after the clipped subtree is not
	// clipped.
	child.SetClipsChildren(true)
	sib := colored("sib", 50, 50, 5, 5, red)
	root.AddChild(sib)
	checkDump(t, dump(t, root, driver.Size{W: 100, H: 100}),
		"fillRRect (0,0,100,100) solid #808080",
		"fillRRect (10,10,30,30) solid #ff0000",
		"fillRRect (30,30,40,40) solid #0000ff clip=(10,10,30,30)",
		"fillRRect (25,25,10,10) solid #00ff00 clip=(30,30,10,10)",
		"fillRRect (50,50,5,5) solid #ff0000",
	)

	// A clipping element without children adds no clip and no state.
	leafClip := colored("leafClip", 0, 0, 1, 1, red)
	leafClip.SetClipsChildren(true)
	checkDump(t, dump(t, leafClip, driver.Size{W: 10, H: 10}), "fillRRect (0,0,1,1) solid #ff0000")
}

func TestPaintStateDoesNotLeak(t *testing.T) {
	root := colored("root", 0, 0, 100, 100, grey)
	a := colored("a", 10, 10, 10, 10, red)
	c := colored("c", 1, 1, 2, 2, blue)
	b := colored("b", 30, 30, 10, 10, green)
	root.SetChildren(a, b)
	a.AddChild(c)
	a.leak = true // translation and clip left behind in a's Paint
	checkDump(t, dump(t, root, driver.Size{W: 100, H: 100}),
		"fillRRect (0,0,100,100) solid #808080",
		"fillRRect (10,10,10,10) solid #ff0000",
		"fillRRect (11,11,2,2) solid #0000ff",
		"fillRRect (30,30,10,10) solid #00ff00",
	)
}

func TestPaintTreeChangedDuringPaint(t *testing.T) {
	root := colored("root", 0, 0, 100, 100, grey)
	a := colored("a", 0, 0, 10, 10, red)
	b := colored("b", 20, 0, 10, 10, green)
	root.SetChildren(a, b)
	// a removes b while painting: b was read before, so it still paints this
	// frame, and the tree changes are seen by the next one.
	a.onPaint = func() { b.RemoveFromParent() }
	checkDump(t, dump(t, root, driver.Size{W: 100, H: 100}),
		"fillRRect (0,0,100,100) solid #808080",
		"fillRRect (0,0,10,10) solid #ff0000",
		"fillRRect (20,0,10,10) solid #00ff00",
	)
	if !root.Dirty() {
		t.Fatal("tree change during paint did not invalidate")
	}
	checkDump(t, dump(t, root, driver.Size{W: 100, H: 100}),
		"fillRRect (0,0,100,100) solid #808080",
		"fillRRect (0,0,10,10) solid #ff0000",
	)
}

func TestHitTest(t *testing.T) {
	root := newBox("root", 0, 0, 200, 100)
	a := newBox("a", 10, 10, 50, 50)
	b := newBox("b", 40, 20, 50, 50) // on top of a where they overlap
	inner := newBox("inner", 5, 6, 10, 10)
	round := newBox("round", 100, 0, 40, 40)
	round.round = true
	root.SetChildren(a, b, round)
	a.AddChild(inner)

	tests := []struct {
		p    driver.Point
		want string
		at   driver.Point
	}{
		{driver.Point{X: 1, Y: 1}, "root", driver.Point{X: 1, Y: 1}},
		{driver.Point{X: 12, Y: 12}, "a", driver.Point{X: 2, Y: 2}},
		{driver.Point{X: 16, Y: 17}, "inner", driver.Point{X: 1, Y: 1}},
		{driver.Point{X: 45, Y: 25}, "b", driver.Point{X: 5, Y: 5}}, // overlap: last child wins
		{driver.Point{X: 10, Y: 10}, "a", driver.Point{}},           // left/top edge inside
		{driver.Point{X: 89.5, Y: 69.5}, "b", driver.Point{X: 49.5, Y: 49.5}},
		{driver.Point{X: 90, Y: 70}, "root", driver.Point{X: 90, Y: 70}}, // right/bottom edge outside b
		{driver.Point{X: 120, Y: 20}, "round", driver.Point{X: 20, Y: 20}},
		{driver.Point{X: 101, Y: 1}, "root", driver.Point{X: 101, Y: 1}}, // corner of round: HitSelf says no
	}
	for _, tc := range tests {
		hit, at := HitTest(root, tc.p)
		if hit == nil || hit.(*box).name != tc.want || at != tc.at {
			name := "<nil>"
			if hit != nil {
				name = hit.(*box).name
			}
			t.Errorf("HitTest(%v) = %s %v, want %s %v", tc.p, name, at, tc.want, tc.at)
		}
	}
	for _, p := range []driver.Point{{X: -1, Y: 0}, {X: 200, Y: 0}, {X: 0, Y: 100}} {
		if hit, at := HitTest(root, p); hit != nil || at != (driver.Point{}) {
			t.Errorf("HitTest(%v) outside = %v", p, hit)
		}
	}

	// A root with an origin works in its parent's coordinates.
	root.SetFrame(driver.Rect{X: 100, Y: 100, W: 200, H: 100})
	if hit, at := HitTest(root, driver.Point{X: 116, Y: 117}); hit != Element(inner) || at != (driver.Point{X: 1, Y: 1}) {
		t.Errorf("offset root: %v %v", hit, at)
	}
}

func TestHitTestHiddenDisabledAndOverflow(t *testing.T) {
	root := newBox("root", 0, 0, 100, 100)
	a := newBox("a", 10, 10, 20, 20)
	b := newBox("b", 10, 10, 20, 20) // on top of a
	out := newBox("out", 15, 15, 20, 20)
	root.SetChildren(a, b)
	b.AddChild(out) // sticks out of b to (45,45)

	if hit, _ := HitTest(root, driver.Point{X: 15, Y: 15}); hit != Element(b) {
		t.Fatal("top sibling")
	}
	// Overflow is hit where it is painted, unless the parent clips.
	if hit, at := HitTest(root, driver.Point{X: 40, Y: 40}); hit != Element(out) || at != (driver.Point{X: 15, Y: 15}) {
		t.Fatalf("overflow: %v %v", hit, at)
	}
	b.SetClipsChildren(true)
	if hit, _ := HitTest(root, driver.Point{X: 40, Y: 40}); hit != Element(root) {
		t.Fatal("clipped overflow hit")
	}
	if hit, _ := HitTest(root, driver.Point{X: 26, Y: 26}); hit != Element(out) {
		t.Fatal("clipped child inside the clip")
	}

	// Disabled elements are hit.
	b.SetEnabled(false)
	if hit, _ := HitTest(root, driver.Point{X: 26, Y: 26}); hit != Element(out) {
		t.Fatal("disabled subtree not hit")
	}
	// Hidden subtrees are skipped: a is hit below b.
	b.SetHidden(true)
	if hit, _ := HitTest(root, driver.Point{X: 26, Y: 26}); hit != Element(a) {
		t.Fatal("hidden subtree hit")
	}
	root.SetHidden(true)
	if hit, _ := HitTest(root, driver.Point{X: 1, Y: 1}); hit != nil {
		t.Fatal("hidden root hit")
	}
}

func TestCoordinateConversion(t *testing.T) {
	root := newBox("root", 100, 50, 300, 300)
	a := newBox("a", 10, 20, 100, 100)
	inner := newBox("inner", 3, 4, 10, 10)
	root.AddChild(a)
	a.AddChild(inner)
	p := driver.Point{X: 1, Y: 2}
	if got := ToRoot(inner, p); got != (driver.Point{X: 114, Y: 76}) {
		t.Fatalf("ToRoot = %v", got)
	}
	if got := FromRoot(inner, ToRoot(inner, p)); got != p {
		t.Fatalf("FromRoot(ToRoot) = %v", got)
	}
	// HitTest and ToRoot agree.
	hit, at := HitTest(root, ToRoot(inner, p))
	if hit != Element(inner) || at != p {
		t.Fatalf("HitTest(ToRoot) = %v %v", hit, at)
	}
}
