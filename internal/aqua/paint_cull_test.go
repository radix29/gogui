package aqua

import (
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"
)

func TestPaintOverflowCoversRingAndShadows(t *testing.T) {
	m := paintOverflow()
	for i := 0; i <= 100; i++ {
		outset, width := FocusRingBand(float64(i) / 100)
		if outset+width > m {
			t.Fatalf("focus ring at p=%v reaches %v outside the shape, margin %v", float64(i)/100, outset+width, m)
		}
	}
	mt, ct := &tokens.Metrics, &tokens.Controls
	for cs := range len(mt.BezelShadowBlur) {
		if e := mt.BezelShadowOffsetY[cs] + mt.BezelShadowSpread[cs] + 1.5*mt.BezelShadowBlur[cs]; e > m {
			t.Errorf("bezel shadow %d reaches %v, margin %v", cs, e, m)
		}
		if e := ct.SliderKnobShadowOffsetY[cs] + 1.5*ct.SliderKnobShadowBlur[cs]; e > m {
			t.Errorf("knob shadow %d reaches %v, margin %v", cs, e, m)
		}
	}
}

func TestPaintCullOutsideClip(t *testing.T) {
	m := paintOverflow()
	root := colored("root", 0, 0, 400, 400, grey)
	clip := colored("clip", 100, 100, 100, 100, red)
	clip.SetClipsChildren(true)
	root.AddChild(clip)
	inside := colored("inside", 10, 10, 20, 20, blue)
	partly := colored("partly", 90, 90, 20, 20, blue)
	// Just outside the margin: the outset frame only touches the clip.
	above := colored("above", 10, -m-20, 20, 20, green)
	below := colored("below", 10, 100+m, 20, 20, green)
	left := colored("left", -m-20, 10, 20, 20, green)
	right := colored("right", 100+m, 10, 20, 20, green)
	// Just inside the margin: a focus ring or shadow could show.
	nearAbove := colored("nearAbove", 10, -m-19, 20, 20, red)
	nearRight := colored("nearRight", 99+m, 10, 20, 20, red)
	clip.SetChildren(inside, partly, above, below, left, right, nearAbove, nearRight)
	grand := newBox("grand", 0, 0, 5, 5)
	below.AddChild(grand) // a clipping culled element: its subtree is skipped
	below.SetClipsChildren(true)

	for _, e := range []*box{root, clip, inside, partly, above, below, left, right, nearAbove, nearRight, grand} {
		e.Invalidate()
	}
	d := dump(t, root, driver.Size{W: 400, H: 400})
	for _, tc := range []struct {
		e    *box
		want int
	}{
		{root, 1}, {clip, 1}, {inside, 1}, {partly, 1}, {nearAbove, 1}, {nearRight, 1},
		{above, 0}, {below, 0}, {left, 0}, {right, 0}, {grand, 0},
	} {
		if tc.e.paints != tc.want {
			t.Errorf("%s painted %d times, want %d", tc.e.name, tc.e.paints, tc.want)
		}
		if tc.e.Dirty() {
			t.Errorf("%s still dirty", tc.e.name)
		}
	}
	if strings.Contains(d, "#00ff00") {
		t.Fatalf("culled element in the dump:\n%s", d)
	}
	if !strings.Contains(d, "fillRRect (110,110,20,20) solid #0000ff clip=(100,100,100,100)") ||
		!strings.Contains(d, "fillRRect (190,190,20,20) solid #0000ff clip=(100,100,100,100)") {
		t.Fatalf("visible children missing:\n%s", d)
	}
}

func TestPaintCullNestedClips(t *testing.T) {
	m := paintOverflow()
	root := colored("root", 0, 0, 300, 300, grey)
	outer := colored("outer", 0, 0, 100, 100, red)
	outer.SetClipsChildren(true)
	inner := colored("inner", 50, 0, 200, 100, red) // half outside outer
	inner.SetClipsChildren(true)
	root.AddChild(outer)
	outer.AddChild(inner)
	// In inner's coordinates the visible part is (0,0,50,100).
	vis := colored("vis", 10, 10, 20, 20, blue)
	out := colored("out", 50+m, 10, 20, 20, green)      // inside inner, outside outer
	edge := colored("edge", 49+m, 10, 20, 20, blue)     // within the margin
	under := colored("under", 10, 100+m, 20, 20, green) // below both
	inner.SetChildren(vis, out, edge, under)
	dump(t, root, driver.Size{W: 300, H: 300})
	if vis.paints != 1 || edge.paints != 1 || out.paints != 0 || under.paints != 0 {
		t.Fatalf("paints vis %d edge %d out %d under %d", vis.paints, edge.paints, out.paints, under.paints)
	}

	// A clipping element entirely outside its clip skips its subtree.
	inner.SetFrame(driver.Rect{X: 100 + m, W: 200, H: 100})
	vis.paints, edge.paints = 0, 0
	dump(t, root, driver.Size{W: 300, H: 300})
	if inner.paints != 1 || vis.paints != 0 || edge.paints != 0 {
		t.Fatalf("paints inner %d vis %d edge %d", inner.paints, vis.paints, edge.paints)
	}

	// An empty clip culls everything below it.
	inner.SetFrame(driver.Rect{X: 10, W: 0, H: 100})
	inner.paints = 0
	dump(t, root, driver.Size{W: 300, H: 300})
	if inner.paints != 1 || vis.paints != 0 || edge.paints != 0 {
		t.Fatalf("empty clip: paints inner %d vis %d edge %d", inner.paints, vis.paints, edge.paints)
	}
}

// A scroll view's document host is framed at minus the offset inside a
// clipping viewport: only the rows near the visible part paint.
func TestPaintCullTranslatedDocument(t *testing.T) {
	m := paintOverflow()
	const rowH, n, offset, viewH = 20.0, 50, 200.0, 50.0
	viewport := colored("viewport", 0, 0, 100, viewH, grey)
	viewport.SetClipsChildren(true)
	doc := colored("doc", 0, -offset, 100, rowH*n, grey)
	viewport.AddChild(doc)
	rows := make([]*box, n)
	for i := range rows {
		rows[i] = colored("row", 0, float64(i)*rowH, 100, rowH, blue)
		doc.AddChild(rows[i])
	}
	d := dump(t, viewport, driver.Size{W: 100, H: viewH})
	painted := 0
	for i, r := range rows {
		y := float64(i) * rowH
		want := 0
		if y+rowH+m > offset && y-m < offset+viewH {
			want = 1
			painted++
		}
		if r.paints != want {
			t.Errorf("row %d (y %v) painted %d times, want %d", i, y, r.paints, want)
		}
	}
	if doc.paints != 1 || painted == 0 || painted > 6 {
		t.Fatalf("doc painted %d, %d rows painted", doc.paints, painted)
	}
	// Row 10 is at the top of the viewport (the clip covers the whole
	// surface, so the dump shows none).
	if !strings.Contains(d, "fillRRect (0,0,100,20) solid #0000ff") {
		t.Fatalf("first visible row missing:\n%s", d)
	}
}

func TestPaintCullNonClippingParent(t *testing.T) {
	root := colored("root", 0, 0, 400, 400, grey)
	clip := colored("clip", 0, 0, 100, 100, red)
	clip.SetClipsChildren(true)
	root.AddChild(clip)
	// The parent lies outside the clip but does not clip its child, which
	// lies inside: the parent is culled, the child painted.
	parent := colored("parent", 200, 200, 10, 10, green)
	child := colored("child", -180, -180, 10, 10, blue)
	clip.AddChild(parent)
	parent.AddChild(child)
	parent.Invalidate()
	child.Invalidate()
	d := dump(t, root, driver.Size{W: 400, H: 400})
	if parent.paints != 0 || child.paints != 1 || parent.Dirty() || child.Dirty() {
		t.Fatalf("paints parent %d child %d, dirty %v %v", parent.paints, child.paints, parent.Dirty(), child.Dirty())
	}
	if !strings.Contains(d, "fillRRect (20,20,10,10) solid #0000ff clip=(0,0,100,100)") {
		t.Fatalf("child missing:\n%s", d)
	}
	// Once the parent clips, its child is cut to the parent's bounds, so the
	// subtree is skipped.
	parent.SetClipsChildren(true)
	dump(t, root, driver.Size{W: 400, H: 400})
	if parent.paints != 0 || child.paints != 1 {
		t.Fatalf("clipping parent: paints parent %d child %d", parent.paints, child.paints)
	}

	// Without a clipping ancestor nothing is culled, however far outside.
	free := colored("free", 0, 0, 10, 10, grey)
	far := colored("far", 5000, -5000, 10, 10, blue)
	farChild := colored("farChild", 1000, 1000, 10, 10, blue)
	free.AddChild(far)
	far.AddChild(farChild)
	dump(t, free, driver.Size{W: 10, H: 10})
	if free.paints != 1 || far.paints != 1 || farChild.paints != 1 {
		t.Fatalf("unclipped tree culled: %d %d %d", free.paints, far.paints, farChild.paints)
	}
}
