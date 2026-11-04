package layout

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
)

// defaultExpand is the default expansion of the v0.1 controls (PLAN.md
// §11.1, S0.4), keyed by stub type. gogui has the same table keyed by
// widget kind (widget.defaultExpand). Text fields, sliders (along their
// length) and progress bars expand horizontally, a text view on both axes,
// a scroll view on each axis with a scroller, a separator along its line.
// Labels, buttons, checkboxes, radios, pop-up buttons and spinners don't
// expand.
func defaultExpand(w driver.Widget) (h, v bool) {
	switch w := w.(type) {
	case *stub.TextField:
		return true, false
	case *stub.Slider:
		return !w.Vertical, w.Vertical
	case *stub.Progress:
		return w.Style == driver.ProgressBar, false
	case *stub.TextView:
		return true, true
	case *stub.ScrollView:
		return w.HasHorizontal, w.HasVertical
	case *stub.Separator:
		return !w.Vertical, w.Vertical
	}
	return false, false
}

// ctl returns a leaf for w with the default expansion of its control type.
func ctl(w driver.Widget) *Leaf {
	l := NewLeaf(w)
	h, v := defaultExpand(w)
	l.Props().SetExpand(Horizontal, h).SetExpand(Vertical, v)
	return l
}

func label(text string, f driver.Font) *stub.Label {
	l := stub.NewLabel()
	l.Text, l.Font = text, f
	return l
}

// withPeer gives s a stub container peer, as every public stack has one.
func withPeer(s *Stack) (*Stack, *stub.Container) {
	c := stub.NewContainer()
	s.SetPeer(c)
	return s, c
}

func TestDefaultExpand(t *testing.T) {
	vslider, spinner, vsep, scroll := stub.NewSlider(nil), stub.NewProgress(), stub.NewSeparator(), stub.NewScrollView(nil)
	vslider.Vertical, spinner.Style, vsep.Vertical, scroll.HasVertical = true, driver.ProgressSpinner, true, true
	cases := []struct {
		w    driver.Widget
		h, v bool
	}{
		{stub.NewLabel(), false, false}, {button("OK"), false, false},
		{stub.NewCheckbox(nil), false, false}, {stub.NewRadio(nil), false, false},
		{stub.NewPopUpButton(nil), false, false}, {stub.NewTextField(true, nil), true, false},
		{stub.NewSlider(nil), true, false}, {vslider, false, true},
		{stub.NewProgress(), true, false}, {spinner, false, false},
		{stub.NewTextView(nil), true, true}, {stub.NewScrollView(nil), false, false},
		{scroll, false, true}, {stub.NewSeparator(), true, false}, {vsep, false, true},
	}
	for i, c := range cases {
		if l := ctl(c.w); l.Expands(Horizontal) != c.h || l.Expands(Vertical) != c.v {
			t.Errorf("case %d (%T): expands %v/%v, want %v/%v", i, c.w,
				l.Expands(Horizontal), l.Expands(Vertical), c.h, c.v)
		}
	}
}

// helloTree is examples/hello:
//
//	VStack(12, greeting, name, HStack(8, Spacer(), greet)).Padding(20)
type helloTree struct {
	root           *Root
	vstack, hstack *stub.Container
	greeting       *stub.Label
	name           *stub.TextField
	greet          *stub.Button
}

// layoutHello builds the hello tree and lays it out at size.
func layoutHello(size driver.Size) helloTree {
	var t helloTree
	t.greeting = label("Hello!", driver.Font{Size: 15, Weight: driver.FontWeightBold})
	t.name = stub.NewTextField(false, nil)
	t.name.Placeholder = "Your name"
	t.greet = button("Greet")
	t.greet.IsDefault = true

	row, hpeer := withPeer(NewHStack(8, NewSpacer(), ctl(t.greet)))
	col, vpeer := withPeer(NewVStack(12, ctl(t.greeting), ctl(t.name), row))
	col.SetPadding(driver.UniformInsets(20))
	t.root, t.vstack, t.hstack = NewRoot(col), vpeer, hpeer
	t.root.Layout(size)
	return t
}

func wantRect(t *testing.T, name string, got, want driver.Rect) {
	t.Helper()
	if !rectNear(got, want) {
		t.Errorf("%s: frame = %v, want %v", name, got, want)
	}
}

func TestExampleHello(t *testing.T) {
	h := layoutHello(driver.Size{W: 360, H: 180})
	// Bold 15 pt "Hello!": 6 runes × 7 × 15/13 wide, 16 × 15/13 high.
	gw, gh := 630.0/13, 240.0/13
	wantRect(t, "vstack", h.vstack.Frame, driver.Rect{W: 360, H: 180})
	wantRect(t, "greeting", h.greeting.Frame, driver.Rect{X: 20, Y: 20, W: gw, H: gh})
	// The text field fills the 320 pt inner width.
	nameY := 20 + gh + 12
	wantRect(t, "name", h.name.Frame, driver.Rect{X: 20, Y: nameY, W: 320, H: 22})
	// The row fills the width because its spacer expands; the leftover
	// height (the VStack doesn't expand vertically) stays at the bottom.
	wantRect(t, "hstack", h.hstack.Frame, driver.Rect{X: 20, Y: nameY + 22 + 12, W: 320, H: 22})
	// The Greet button (63 pt) is trailing, relative to the row's peer.
	wantRect(t, "greet", h.greet.Frame, driver.Rect{X: 257, W: 63, H: 22})
}
