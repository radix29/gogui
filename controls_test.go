package gogui

import (
	"slices"
	"testing"

	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/layout"
)

func TestButton(t *testing.T) {
	runApp(t, func(*App) {
		b := NewButton("Greet")
		p := b.peer.(*stub.Button)
		var got *Button = b.SetTitle("Hello").SetDefault(true).SetCancel(true).SetTooltip("t")
		if got != b || p.Title != "Hello" || !p.IsDefault || !p.IsCancel || p.Tooltip != "t" {
			t.Fatalf("button peer: %+v", p)
		}
		if b.node().Expands(layout.Horizontal) || b.node().Expands(layout.Vertical) {
			t.Error("a button expands")
		}
		var log []string
		b.OnClick(func() { log = append(log, "a") })
		unsub := b.OnClick(func() { log = append(log, "b") })
		p.UserClick()
		unsub()
		unsub()
		p.UserClick()
		if want := []string{"a", "b", "a"}; !slices.Equal(log, want) {
			t.Errorf("clicks %v, want %v", log, want)
		}
		b.SetEnabled(false)
		p.UserClick()
		buttonEvents{b}.Clicked() // a native layer that still reports a click
		b.destroy()
		buttonEvents{b}.Clicked()
		if len(log) != 3 {
			t.Errorf("disabled or destroyed button fired: %v", log)
		}
		b.SetTitle("x") // only records
		if p.Title != "Hello" {
			t.Error("setter reached a destroyed peer")
		}
	})
}

func TestCheckbox(t *testing.T) {
	runApp(t, func(*App) {
		c := NewCheckbox("Check")
		p := c.peer.(*stub.Checkbox)
		if p.Title != "Check" || c.Checked() || c.State() != StateOff {
			t.Fatalf("new checkbox: %+v", p)
		}
		var got []bool
		unsub := c.OnChange(func(on bool) { got = append(got, on) })
		var chained *Checkbox = c.SetChecked(true).SetTitle("Box")
		if chained != c || !c.Checked() || p.Value != StateOn || p.Title != "Box" {
			t.Fatalf("SetChecked: %+v", p)
		}
		c.SetState(StateMixed)
		if c.State() != StateMixed || !c.Checked() || p.Value != StateMixed {
			t.Errorf("mixed: state %v, peer %v", c.State(), p.Value)
		}
		if len(got) != 0 {
			t.Errorf("setters fired OnChange: %v", got)
		}
		p.UserClick() // mixed → on
		p.UserClick() // on → off
		if want := []bool{true, false}; !slices.Equal(got, want) {
			t.Errorf("OnChange %v, want %v", got, want)
		}
		if c.State() != StateOff || c.Checked() {
			t.Errorf("state after clicks: %v", c.State())
		}
		c.SetState(CheckState(7))
		if c.State() != StateOn {
			t.Errorf("unknown state became %v, want on", c.State())
		}
		c.SetEnabled(false)
		checkboxEvents{c}.StateChanged(StateOff)
		c.SetEnabled(true)
		unsub()
		p.UserClick()
		if len(got) != 2 {
			t.Errorf("disabled or unsubscribed handler fired: %v", got)
		}
	})
}

func TestRadioGroup(t *testing.T) {
	runApp(t, func(*App) {
		g := NewRadioGroup("Small", "Medium", "Large")
		radios := children(g)
		if len(radios) != 3 || radios[1].(*stub.Radio).Title != "Medium" {
			t.Fatalf("group container children: %v", radios)
		}
		on := func() []bool {
			out := make([]bool, len(radios))
			for i, r := range radios {
				out[i] = r.(*stub.Radio).IsOn
			}
			return out
		}
		click := func(i int) { radios[i].(*stub.Radio).UserClick() }
		if g.Selected() != 0 || !slices.Equal(on(), []bool{true, false, false}) {
			t.Fatalf("new group: selected %d, on %v", g.Selected(), on())
		}
		var got []int
		g.OnChange(func(i int) { got = append(got, i) })
		unsub := g.OnChange(func(i int) { got = append(got, 10+i) })

		var chained *RadioGroup = g.SetSelected(1)
		if chained != g || g.Selected() != 1 || !slices.Equal(on(), []bool{false, true, false}) {
			t.Errorf("SetSelected(1): selected %d, on %v", g.Selected(), on())
		}
		g.SetSelected(-1)
		if g.Selected() != -1 || !slices.Equal(on(), []bool{false, false, false}) {
			t.Errorf("SetSelected(-1): selected %d, on %v", g.Selected(), on())
		}
		g.SetSelected(3)
		if g.Selected() != -1 || len(got) != 0 {
			t.Errorf("out of range or events from setters: selected %d, events %v", g.Selected(), got)
		}

		click(2) // exclusivity: the others turn off
		if g.Selected() != 2 || !slices.Equal(on(), []bool{false, false, true}) {
			t.Errorf("click 2: selected %d, on %v", g.Selected(), on())
		}
		click(2) // already selected: no event
		click(0)
		unsub()
		click(1)
		if want := []int{2, 12, 0, 10, 1}; !slices.Equal(got, want) {
			t.Errorf("OnChange %v, want %v", got, want)
		}
		if !slices.Equal(on(), []bool{false, true, false}) {
			t.Errorf("on %v after clicks", on())
		}

		g.SetEnabled(false)
		for _, r := range radios {
			if !r.(*stub.Radio).Disabled {
				t.Error("radio not disabled with its group")
			}
		}
		radioEvents{g, 0}.StateChanged(StateOn) // reported anyway
		if g.Selected() != 1 || len(got) != 5 {
			t.Errorf("disabled group: selected %d, events %v", g.Selected(), got)
		}
		g.SetEnabled(true).SetControlSize(ControlSizeSmall).SetTooltip("Size").Focus()
		if r := radios[1].(*stub.Radio); r.ControlSize != ControlSizeSmall || r.FocusCalls != 1 || r.Tooltip != "Size" {
			t.Errorf("radio 1: %+v", r.Base)
		}
		if NewRadioGroup().Selected() != -1 {
			t.Error("an empty group has a selection")
		}
	})
}

// TestRadioGroupLayout checks the group as a 6 pt VStack in a form and its
// bottom-up destruction with the window.
func TestRadioGroupLayout(t *testing.T) {
	runApp(t, func(app *App) {
		g := NewRadioGroup("Free", "Pro", "Team")
		form := Form(Row("Plan:", g))
		layoutAt(form, Size{W: 200, H: 100})
		wantFrame(t, "group", g, Rect{X: 43, Y: 0, W: 46, H: 66})
		radios := slices.Clone(children(g)) // the stub compacts Children in place
		if f := radios[1].(*stub.Radio).Frame; f != (Rect{Y: 24, W: 39, H: 18}) {
			t.Errorf("Pro frame %v", f)
		}
		if g.node().Expands(layout.Horizontal) || g.node().Expands(layout.Vertical) {
			t.Error("a radio group expands")
		}

		win := app.NewWindow(WindowConfig{Size: Size{W: 200, H: 100}})
		win.SetContent(form)
		box := g.peer.(*stub.Container)
		var aliveOrder []bool
		win.OnClose(func() { aliveOrder = append(aliveOrder, box.Destroyed) })
		win.Close()
		for _, r := range radios {
			if !r.(*stub.Radio).Destroyed {
				t.Error("radio not destroyed with the window")
			}
		}
		if !box.Destroyed || len(aliveOrder) != 1 || aliveOrder[0] {
			t.Error("group container not destroyed after OnClose")
		}
		// The radios were destroyed first, each removing itself from the
		// container (stub Destroy), so it ends up empty.
		if len(box.Children) != 0 {
			t.Errorf("container still has %d children", len(box.Children))
		}
		radioEvents{g, 0}.StateChanged(StateOn)
		g.SetSelected(2)
		if g.Selected() != 2 {
			t.Error("destroyed group doesn't record the selection")
		}
	})
}

func TestSeparator(t *testing.T) {
	runApp(t, func(*App) {
		s := NewSeparator()
		p := s.peer.(*stub.Separator)
		if !s.node().Expands(layout.Horizontal) || s.node().Expands(layout.Vertical) || p.Vertical {
			t.Fatal("a new separator isn't horizontal")
		}
		h := &countingHost{}
		s.host = h
		var got *Separator = s.SetVertical(true)
		if got != s || !p.Vertical || s.node().Expands(layout.Horizontal) || !s.node().Expands(layout.Vertical) {
			t.Error("SetVertical(true) didn't flip the expansion")
		}
		s.SetVertical(false)
		if p.Vertical || !s.node().Expands(layout.Horizontal) || s.node().Expands(layout.Vertical) {
			t.Error("SetVertical(false) didn't flip it back")
		}
		if h.n != 2 {
			t.Errorf("%d invalidations, want 2", h.n)
		}
		row := HStack(0, NewLabel("a"), NewSeparator().SetVertical(true), NewLabel("b"))
		layoutAt(row, Size{W: 100, H: 40})
		if f := children(row)[1].(*stub.Separator).Frame; f.W != 1 || f.H != 40 {
			t.Errorf("vertical separator frame %v, want 1 × 40", f)
		}
	})
}
