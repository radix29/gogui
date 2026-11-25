package aqua

import (
	"math"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
)

// withControls runs f with the Controls tokens changed by edit, then
// restores the token set.
func withControls(t *testing.T, edit func(c *theme.Controls), f func()) {
	t.Helper()
	old := tokens
	tk := *old
	edit(&tk.Controls)
	tokens = &tk
	defer func() { tokens = old }()
	f()
}

var infSize = driver.Size{W: math.Inf(1), H: math.Inf(1)}

func TestSeparatorIsReal(t *testing.T) {
	d := startDriver(t)
	s := d.NewSeparator().(*Separator)
	if e := ElementOf(s); e == nil || e != Element(s.view) || s.StubPeer() != s.Separator || s.Stub() != &s.Separator.Base {
		t.Error("ElementOf, StubPeer or Stub")
	}
	if got := nodeOf(s); got != `separator ""` {
		t.Errorf("node %q", got)
	}
	if ElementOf(s).base().Focusable() {
		t.Error("focusable")
	}
	if got := s.FittingSize(infSize); got != (driver.Size{W: 1, H: 1}) {
		t.Errorf("FittingSize %v", got)
	}
	if got := ElementOf(s).FittingSize(driver.Size{W: 0.5, H: 4}); got != (driver.Size{W: 0.5, H: 1}) {
		t.Errorf("clamped FittingSize %v", got)
	}
	withControls(t, func(c *theme.Controls) { c.SeparatorThickness = 2 }, func() {
		if got := s.FittingSize(infSize); got != (driver.Size{W: 2, H: 2}) {
			t.Errorf("FittingSize with a 2 pt token %v", got)
		}
	})
	s.Fixed = driver.Size{W: 5, H: 6}
	if got := s.FittingSize(infSize); got != s.Fixed {
		t.Errorf("fixed FittingSize %v", got)
	}
	b := ElementOf(s).base()
	b.dirty = false
	s.SetVertical(true)
	if !s.Vertical || !b.dirty {
		t.Error("SetVertical")
	}
}

func TestSeparatorPaint(t *testing.T) {
	d := startDriver(t)
	s := d.NewSeparator().(*Separator)
	const col = "solid #0000001a"
	// Horizontal, centred in a taller frame (4.5 rounds to 5).
	checkDump(t, paintPeer(t, s, driver.Rect{X: 5, Y: 6, W: 100, H: 1}), "fillRRect (5,6,100,1) "+col)
	checkDump(t, paintPeer(t, s, driver.Rect{W: 100, H: 10}), "fillRRect (0,5,100,1) "+col)
	// Vertical by its frame.
	checkDump(t, paintPeer(t, s, driver.Rect{X: 10, W: 1, H: 50}), "fillRRect (10,0,1,50) "+col)
	checkDump(t, paintPeer(t, s, driver.Rect{X: 10, W: 7, H: 50}), "fillRRect (13,0,1,50) "+col)
	// A square frame follows SetVertical.
	checkDump(t, paintPeer(t, s, driver.Rect{W: 3, H: 3}), "fillRRect (0,1,3,1) "+col)
	s.SetVertical(true)
	checkDump(t, paintPeer(t, s, driver.Rect{W: 3, H: 3}), "fillRRect (1,0,1,3) "+col)
	s.SetVertical(false)
	// Fractional frames land on whole pixels.
	checkDump(t, paintPeer(t, s, driver.Rect{X: 0.3, Y: 2.4, W: 100, H: 1}), "fillRRect (0,2,100,1) "+col)
	checkDump(t, paintPeer(t, s, driver.Rect{X: 0.5, Y: 2.6, W: 99.4, H: 1}), "fillRRect (1,3,99,1) "+col)
	// Also inside a parent at a fractional offset.
	parent := newBox("parent", 0.25, 0.25, 200, 100)
	parent.AddChild(s.view)
	s.SetFrame(driver.Rect{X: 1, Y: 1, W: 50, H: 1})
	checkDump(t, dump(t, parent, driver.Size{W: 400, H: 300}),
		"fillRRect (0.25,0.25,200,100) solid #ff0000",
		"fillRRect (1,1,50,1) "+col)
	s.view.RemoveFromParent()
	// A thinner token is still one pixel; a thicker one is rounded.
	withControls(t, func(c *theme.Controls) { c.SeparatorThickness = 0.3 }, func() {
		checkDump(t, paintPeer(t, s, driver.Rect{W: 100, H: 1}), "fillRRect (0,0,100,1) "+col)
	})
	withControls(t, func(c *theme.Controls) { c.SeparatorThickness = 2.4 }, func() {
		checkDump(t, paintPeer(t, s, driver.Rect{W: 100, H: 4}), "fillRRect (0,1,100,2) "+col)
	})
	// Nothing for an empty frame.
	checkDump(t, paintPeer(t, s, driver.Rect{W: 100}))
}

func TestSeparatorInWindow(t *testing.T) {
	s := startShell(t)
	d := newDriver(s)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	sep := d.NewSeparator().(*Separator)
	w.SetContent(sep)
	sep.SetFrame(driver.Rect{X: 10, Y: 10.2, W: 100.1, H: 1})
	line := func() string {
		for _, l := range strings.Split(paint(t, s, sf), "\n") {
			if strings.HasPrefix(l, "fillRRect (10,") {
				l, _, _ = strings.Cut(l, " clip=")
				return l
			}
		}
		return ""
	}
	// The title bar is 28 pt, so content y is +28: 38.2 snaps to 38.
	if got := line(); got != "fillRRect (10,38,100,1) solid #0000001a" {
		t.Errorf("scale 1: %q", got)
	}
	// At 2×: half-point steps, the hairline one pixel.
	sf.SetScale(2)
	withControls(t, func(c *theme.Controls) { c.SeparatorThickness = 0.5 }, func() {
		if got := line(); got != "fillRRect (10,38.5,100,0.5) solid #0000001a" { // centred: 38.45 snaps to 38.5
			t.Errorf("scale 2, 0.5 pt: %q", got)
		}
	})
	sep.redraw() // the tokens changed back
	if got := line(); got != "fillRRect (10,38,100,1) solid #0000001a" {
		t.Errorf("scale 2: %q", got)
	}
	// At 1.5×: 1 pt is 1.5 px, so two pixels (1.333 pt) on the 2/3 pt grid.
	sf.SetScale(1.5)
	if got := line(); got != "fillRRect (10,38,100,1.333) solid #0000001a" {
		t.Errorf("scale 1.5: %q", got)
	}
	// Dark.
	sf.SetScale(1)
	w.SetAppearance(driver.Appearance{Dark: true, Accent: driver.DefaultAccent})
	if got := line(); got != "fillRRect (10,38,100,1) solid #ffffff1a" {
		t.Errorf("dark: %q", got)
	}
	tree := AccessibilityTree(w.Root())
	if n := tree.Find(a11y.RoleSeparator, ""); n == nil || n.Frame != (driver.Rect{X: 10, Y: 38.2, W: 100.1, H: 1}) {
		t.Errorf("a11y tree:\n%s", tree.Dump())
	}
	// A closed window's elements fall back to scale 1.
	w.Close()
	if got := scaleIn(&sep.view.Base); got != 1 {
		t.Errorf("scale after close %g", got)
	}
}

// The fake text system lays out 11 pt text 13.538 pt high, so the title
// band is 14 + 4 = 18 pt and the content insets are 26 (top), 10, 8, 10.
var titledInsets = driver.Insets{Top: 26, Left: 10, Bottom: 8, Right: 10}

func TestBoxIsReal(t *testing.T) {
	d := startDriver(t)
	b := d.NewBox().(*Box)
	if e := ElementOf(b); e == nil || e != Element(b.view) || b.StubPeer() != b.Box || b.Stub() != &b.Box.Base {
		t.Error("ElementOf, StubPeer or Stub")
	}
	if got := nodeOf(b); got != `group ""` {
		t.Errorf("node %q", got)
	}
	b.SetTitle("Values")
	if got := nodeOf(b); got != `group "Values"` {
		t.Errorf("node %q", got)
	}
	if ElementOf(b).base().Focusable() {
		t.Error("focusable")
	}
}

func TestBoxInsets(t *testing.T) {
	d := startDriver(t)
	b := d.NewBox().(*Box)
	untitled := driver.Insets{Top: 8, Left: 10, Bottom: 8, Right: 10}
	cases := []struct {
		title string
		want  driver.Insets
	}{
		{"", untitled},
		{" \t", untitled}, // blank: hidden, as on the stub
		{"Values", titledInsets},
		{"Two\nlines", titledInsets}, // one line
	}
	for _, c := range cases {
		b.SetTitle(c.title)
		if got := b.ContentInsets(); got != c.want {
			t.Errorf("title %q: insets %+v, want %+v", c.title, got, c.want)
		}
		if got, want := b.Box.ContentInsets(), stub.NewBox(); c.title == "" && got != want.ContentInsets() {
			t.Errorf("stub insets changed: %+v", got)
		}
	}
	if got := b.FittingSize(infSize); got != (driver.Size{W: 20, H: 34}) {
		t.Errorf("FittingSize %v", got)
	}
	if got := ElementOf(b).FittingSize(driver.Size{W: 15, H: 100}); got != (driver.Size{W: 15, H: 34}) {
		t.Errorf("clamped FittingSize %v", got)
	}
	b.Fixed = driver.Size{W: 5, H: 6}
	if got := b.FittingSize(infSize); got != b.Fixed {
		t.Errorf("fixed FittingSize %v", got)
	}
	// The title layout is cached per title.
	l := b.titleLayout()
	if b.titleLayout() != l {
		t.Error("same title, new layout")
	}
	b.SetTitle("Other")
	if b.titleLayout() == l {
		t.Error("new title, same layout")
	}
}

func TestBoxPaint(t *testing.T) {
	d := startDriver(t)
	box := d.NewBox().(*Box)
	inner := d.NewPopUpButton(nil)
	inner.SetItems([]string{"In"})
	box.SetContent(inner)
	box.SetFrame(driver.Rect{X: 10, Y: 10, W: 200, H: 60})
	inner.SetFrame(driver.Rect{W: 180, H: 22}) // as the layout does: content coordinates
	if box.Frame != (driver.Rect{X: 10, Y: 10, W: 200, H: 60}) || box.Content != inner {
		t.Errorf("stub state: %v %v", box.Frame, box.Content)
	}
	if got := box.content.Frame(); got != (driver.Rect{X: 10, Y: 8, W: 180, H: 44}) {
		t.Errorf("content host %v", got)
	}
	content := []string{
		"shadow (20,19,180,20) r=5 off=(0,0.5) blur=1 #00000033",
		"fillRRect (20,19,180,20) r=5 solid #ffffff",
		"strokeRRect (20.5,19.5,179,19) r=4.5 w=1 solid #00000026",
		`text (28,21,14,16) "In" 13pt #000000d9`,
		"fillRRect (182,21,16,16) r=3 solid #007aff",
		"strokePath [M(186.5,28) L(190,24.5) L(193.5,28) M(186.5,30) L(190,33.5) L(193.5,30)] w=1.5 cap=round join=round solid #ffffff",
	}
	checkDump(t, dump(t, ElementOf(box), driver.Size{W: 400, H: 300}), append([]string{
		"fillRRect (10,10,200,60) r=6 solid #0000000a",
		"strokeRRect (10.5,10.5,199,59) r=5.5 w=1 solid #00000014",
	}, content...)...)

	// A title above the group moves the group down by the band and the
	// content by the band as well.
	box.SetTitle("Group")
	if got := box.content.Frame(); got != (driver.Rect{X: 10, Y: 26, W: 180, H: 26}) {
		t.Errorf("titled content host %v", got)
	}
	content = []string{
		"shadow (20,37,180,20) r=5 off=(0,0.5) blur=1 #00000033",
		"fillRRect (20,37,180,20) r=5 solid #ffffff",
		"strokeRRect (20.5,37.5,179,19) r=4.5 w=1 solid #00000026",
		`text (28,39,14,16) "In" 13pt #000000d9`,
		"fillRRect (182,39,16,16) r=3 solid #007aff",
		"strokePath [M(186.5,46) L(190,42.5) L(193.5,46) M(186.5,48) L(190,51.5) L(193.5,48)] w=1.5 cap=round join=round solid #ffffff",
	}
	checkDump(t, dump(t, ElementOf(box), driver.Size{W: 400, H: 300}), append([]string{
		`text (10,10,29.615,13.538) "Group" 11pt #000000d9`,
		"fillRRect (10,28,200,42) r=6 solid #0000000a",
		"strokeRRect (10.5,28.5,199,41) r=5.5 w=1 solid #00000014",
	}, content...)...)

	// Without content: the title and group only.
	box.SetContent(nil)
	if box.content.ChildCount() != 0 || ElementOf(inner).base().Parent() != nil {
		t.Error("SetContent(nil) kept the element")
	}
	box.SetContent(stub.NewLabel())
	if box.content.ChildCount() != 0 {
		t.Error("stub content got an element")
	}
	group := []string{
		"fillRRect (0,18,200,42) r=6 solid #0000000a",
		"strokeRRect (0.5,18.5,199,41) r=5.5 w=1 solid #00000014",
	}
	checkDump(t, paintPeer(t, box, driver.Rect{W: 200, H: 60}),
		append([]string{`text (0,0,29.615,13.538) "Group" 11pt #000000d9`}, group...)...)

	// Disabled, also through an ancestor: the disabled text color.
	box.SetEnabled(false)
	checkDump(t, paintPeer(t, box, driver.Rect{W: 200, H: 60}),
		append([]string{`text (0,0,29.615,13.538) "Group" 11pt #00000040`}, group...)...)
	box.SetEnabled(true)

	// A narrow box truncates the title; one too narrow for "…" drops it.
	box.SetTitle("A long title")
	if got := paintPeer(t, box, driver.Rect{W: 30, H: 60}); !strings.Contains(got, `"A lo…"`) {
		t.Errorf("narrow box:\n%s", got)
	}
	checkDump(t, paintPeer(t, box, driver.Rect{W: 2, H: 60}),
		"fillRRect (0,18,2,42) r=1 solid #0000000a")

	// Too low for the group, or just the title band: the title only.
	box.SetTitle("Group")
	checkDump(t, paintPeer(t, box, driver.Rect{W: 200, H: 18}),
		`text (0,0,29.615,13.538) "Group" 11pt #000000d9`)
	// Small: the radius follows the group; the border needs room.
	box.SetTitle("")
	checkDump(t, paintPeer(t, box, driver.Rect{W: 200, H: 8}),
		"fillRRect (0,0,200,8) r=4 solid #0000000a",
		"strokeRRect (0.5,0.5,199,7) r=3.5 w=1 solid #00000014")
	checkDump(t, paintPeer(t, box, driver.Rect{W: 200, H: 2}),
		"fillRRect (0,0,200,2) r=1 solid #0000000a")
	// Fractional frames land on whole pixels.
	checkDump(t, paintPeer(t, box, driver.Rect{X: 0.4, Y: 0.6, W: 100, H: 50.2}),
		"fillRRect (0,1,100,50) r=6 solid #0000000a",
		"strokeRRect (0.5,1.5,99,49) r=5.5 w=1 solid #00000014")
	// No border with a zero width; a wider one is rounded to pixels.
	withControls(t, func(c *theme.Controls) { c.BoxBorderWidth = 0 }, func() {
		checkDump(t, paintPeer(t, box, driver.Rect{W: 100, H: 50}),
			"fillRRect (0,0,100,50) r=6 solid #0000000a")
	})
	withControls(t, func(c *theme.Controls) { c.BoxBorderWidth = 1.6 }, func() {
		checkDump(t, paintPeer(t, box, driver.Rect{W: 100, H: 50}),
			"fillRRect (0,0,100,50) r=6 solid #0000000a",
			"strokeRRect (1,1,98,48) r=5 w=2 solid #00000014")
	})
	// Nothing for an empty frame.
	checkDump(t, paintPeer(t, box, driver.Rect{W: 100}))
	// A group thinner than half a pixel snaps to nothing.
	checkDump(t, paintPeer(t, box, driver.Rect{W: 100, H: 0.3}))
}

func TestBoxRedraws(t *testing.T) {
	d := startDriver(t)
	box := d.NewBox().(*Box)
	box.SetFrame(driver.Rect{W: 100, H: 50})
	b := ElementOf(box).base()
	b.dirty = false
	box.SetTitle("T")
	if !b.dirty || box.Title != "T" {
		t.Error("SetTitle did not invalidate")
	}
	if got := box.content.Frame(); got != (driver.Rect{X: 10, Y: 26, W: 80, H: 16}) {
		t.Errorf("content host after SetTitle %v", got)
	}
}

func TestBoxInWindow(t *testing.T) {
	s := startShell(t)
	d := newDriver(s)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 300, H: 200}, Style: allStyles})
	w.SetAppearance(driver.Appearance{Dark: true, Accent: driver.DefaultAccent})
	box := d.NewBox().(*Box)
	box.SetTitle("Dark")
	lbl := d.NewLabel()
	lbl.SetText("in")
	box.SetContent(lbl)
	w.SetContent(box)
	box.SetFrame(driver.Rect{X: 10, Y: 10, W: 200, H: 60})
	lbl.SetFrame(driver.Rect{W: 50, H: 16})
	got := paint(t, s, sf)
	for _, want := range []string{
		`text (10,38,23.692,13.538) "Dark" 11pt #ffffffd9`,
		"fillRRect (10,56,200,42) r=6 solid #ffffff0d",
		"strokeRRect (10.5,56.5,199,41) r=5.5 w=1 solid #ffffff1a",
		`text (20,64,14,16) "in" 13pt #ffffffd9`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	tree := AccessibilityTree(w.Root())
	g := tree.Find(a11y.RoleGroup, "Dark")
	if g == nil || g.Frame != (driver.Rect{X: 10, Y: 38, W: 200, H: 60}) ||
		len(g.Children) != 1 || g.Children[0].Role != a11y.RoleStaticText {
		t.Errorf("a11y tree:\n%s", tree.Dump())
	}
	box.Destroy()
	if !box.Destroyed || ElementOf(box).base().Parent() != nil {
		t.Error("Destroy")
	}
}
