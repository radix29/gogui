package gogui

import (
	"math"
	"slices"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/layout"
)

func TestTextField(t *testing.T) {
	runApp(t, func(*App) {
		f := NewTextField()
		p := f.peer.(*stub.TextField)
		if p.Secure || f.Secure() || f.Text() != "" || p.ReadOnly {
			t.Fatalf("new field: %+v", p)
		}
		if !f.node().Expands(layout.Horizontal) || f.node().Expands(layout.Vertical) {
			t.Error("a text field doesn't expand horizontally only")
		}
		h := &countingHost{}
		f.host = h
		var got *TextField = f.SetText("a").SetPlaceholder("Name").SetFont(SystemFont(14)).
			SetAlignment(AlignCenter).SetEditable(false).SetTooltip("t")
		if got != f || p.Value != "a" || p.Placeholder != "Name" || p.Font != SystemFont(14) ||
			p.Alignment != AlignCenter || !p.ReadOnly || f.Text() != "a" || f.Font() != SystemFont(14) {
			t.Fatalf("setters: %+v", p)
		}
		if h.n != 3 { // SetText, SetPlaceholder, SetFont
			t.Errorf("%d invalidations, want 3", h.n)
		}
		f.SetEditable(true)

		var log []string
		f.OnChange(func(s string) { log = append(log, "c:"+s) })
		unsub := f.OnChange(func(s string) { log = append(log, "c2:"+s) })
		f.OnSubmit(func(s string) { log = append(log, "s:"+s) })
		p.UserType("Jane")
		if f.Text() != "Jane" {
			t.Errorf("Text after typing = %q", f.Text())
		}
		unsub()
		unsub()
		p.UserSubmit()
		f.SetText("x") // setters don't fire
		if want := []string{"c:Jane", "c2:Jane", "s:Jane"}; !slices.Equal(log, want) {
			t.Errorf("events %v, want %v", log, want)
		}

		f.SetEnabled(false)
		textFieldEvents{f}.TextChanged("y") // reported anyway
		textFieldEvents{f}.Submitted("y")
		f.destroy()
		textFieldEvents{f}.TextChanged("z")
		if len(log) != 3 || f.Text() != "x" {
			t.Errorf("disabled or destroyed field: events %v, text %q", log, f.Text())
		}
		f.SetText("after")
		if p.Value != "x" || f.Text() != "after" {
			t.Errorf("destroyed field: peer %q, Text %q", p.Value, f.Text())
		}

		s := NewSecureField().SetPlaceholder("Password")
		if sp := s.peer.(*stub.TextField); !sp.Secure || !s.Secure() || sp.Placeholder != "Password" {
			t.Errorf("secure field: %+v", sp)
		}
	})
}

// TestTextFieldInvalidatesWindow checks that SetText marks a real window
// dirty, so one pass runs after the next drain.
func TestTextFieldInvalidatesWindow(t *testing.T) {
	runApp(t, func(app *App) {
		f := NewTextField()
		win, _ := newTestWindow(app, WindowConfig{Size: Size{W: 200, H: 50}})
		win.SetContent(VStack(0, f))
		win.layout() // clean
		if win.pending {
			t.Fatal("window pending after layout")
		}
		f.SetText("hello")
		if !win.pending {
			t.Error("SetText didn't mark the window dirty")
		}
		drain(app)
		if win.pending {
			t.Error("the queued pass didn't run")
		}
	})
}

func TestSlider(t *testing.T) {
	runApp(t, func(*App) {
		s := NewSlider()
		p := s.peer.(*stub.Slider)
		if s.Value() != 0 || p.Min != 0 || p.Max != 1 {
			t.Fatalf("new slider: %+v", p)
		}
		var got *Slider = s.SetRange(0, 100).SetValue(40).SetTickMarks(5, true)
		if got != s || s.Value() != 40 || p.Val != 40 || p.Max != 100 || p.TickMarks != 5 || !p.SnapToTick {
			t.Fatalf("setters: %+v", p)
		}
		// Clamping is mirrored in the getter.
		for _, c := range []struct{ in, want float64 }{
			{150, 100}, {-5, 0}, {math.Inf(1), 100}, {math.NaN(), 0}, {12.5, 12.5},
		} {
			if s.SetValue(c.in); s.Value() != c.want || p.Val != c.want {
				t.Errorf("SetValue(%v): Value %v, peer %v, want %v", c.in, s.Value(), p.Val, c.want)
			}
		}
		s.SetValue(80).SetRange(-40, 50)
		if s.Value() != 50 || p.Val != 50 || p.Min != -40 {
			t.Errorf("value after shrinking the range: %v (peer %v)", s.Value(), p.Val)
		}
		s.SetRange(10, 5) // max below min becomes min
		if s.Value() != 10 || p.Min != 10 || p.Max != 10 {
			t.Errorf("inverted range: value %v, peer %v…%v", s.Value(), p.Min, p.Max)
		}
		s.SetRange(math.NaN(), 20) // a NaN bound is kept
		if p.Min != 10 || p.Max != 20 {
			t.Errorf("NaN bound: peer %v…%v", p.Min, p.Max)
		}

		var vals []float64
		unsub := s.OnChange(func(v float64) { vals = append(vals, v) })
		p.UserDrag(15)
		p.UserDrag(99)                   // clamped by the peer to 20
		sliderEvents{s}.ValueChanged(-3) // a native value outside the range
		if want := []float64{15, 20, 10}; !slices.Equal(vals, want) || s.Value() != 10 {
			t.Errorf("OnChange %v (Value %v), want %v", vals, s.Value(), want)
		}
		s.SetEnabled(false)
		sliderEvents{s}.ValueChanged(12)
		s.SetEnabled(true)
		unsub()
		p.UserDrag(16)
		if len(vals) != 3 {
			t.Errorf("disabled or unsubscribed slider fired: %v", vals)
		}
	})
}

func TestSliderVertical(t *testing.T) {
	runApp(t, func(*App) {
		s := NewSlider()
		p := s.peer.(*stub.Slider)
		if !s.node().Expands(layout.Horizontal) || s.node().Expands(layout.Vertical) {
			t.Fatal("a new slider isn't horizontal")
		}
		h := &countingHost{}
		s.host = h
		s.SetVertical(true)
		if !p.Vertical || s.node().Expands(layout.Horizontal) || !s.node().Expands(layout.Vertical) {
			t.Error("SetVertical(true) didn't flip the expansion")
		}
		row := HStack(0, s)
		layoutAt(row, Size{W: 100, H: 200})
		if f := p.Frame; f.H != 200 || f.W != stub.ControlHeight(ControlSizeRegular) {
			t.Errorf("vertical slider frame %v", f)
		}
		s.SetVertical(false)
		if p.Vertical || !s.node().Expands(layout.Horizontal) || h.n != 2 {
			t.Errorf("SetVertical(false): vertical %v, %d invalidations", p.Vertical, h.n)
		}
	})
}

func TestProgress(t *testing.T) {
	runApp(t, func(*App) {
		b := NewProgressBar()
		p := b.peer.(*stub.Progress)
		if p.Style != driver.ProgressBar || p.Max != 100 || b.Value() != 0 || b.Indeterminate() {
			t.Fatalf("new bar: %+v", p)
		}
		if !b.node().Expands(layout.Horizontal) || b.node().Expands(layout.Vertical) {
			t.Error("a progress bar doesn't expand horizontally only")
		}
		h := &countingHost{}
		b.host = h
		var got *ProgressBar = b.SetRange(0, 1).SetValue(0.5).SetIndeterminate(true).SetTooltip("t")
		if got != b || p.Max != 1 || p.Value != 0.5 || !p.Indeterminate || !b.Indeterminate() {
			t.Fatalf("setters: %+v", p)
		}
		b.SetValue(2)
		if b.Value() != 1 || p.Value != 1 {
			t.Errorf("clamped value %v (peer %v), want 1", b.Value(), p.Value)
		}
		b.SetRange(0, 0.25)
		if b.Value() != 0.25 || p.Value != 0.25 {
			t.Errorf("value after shrinking the range %v (peer %v)", b.Value(), p.Value)
		}
		if h.n != 0 {
			t.Errorf("progress setters invalidated %d times", h.n)
		}

		s := NewSpinner()
		sp := s.peer.(*stub.Progress)
		var chained *Spinner = s.SetIndeterminate(true).SetValue(30)
		if chained != s || sp.Style != driver.ProgressSpinner || !sp.Indeterminate || s.Value() != 30 {
			t.Errorf("spinner: %+v", sp)
		}
		if s.node().Expands(layout.Horizontal) || s.node().Expands(layout.Vertical) {
			t.Error("a spinner expands")
		}
	})
}

func TestPopUpButton(t *testing.T) {
	runApp(t, func(*App) {
		b := NewPopUpButton("Red", "Green", "Blue")
		p := b.peer.(*stub.PopUpButton)
		if !slices.Equal(p.Items, []string{"Red", "Green", "Blue"}) || b.Selected() != 0 || p.Index != 0 {
			t.Fatalf("new pop-up: %+v", p)
		}
		if b.node().Expands(layout.Horizontal) || b.node().Expands(layout.Vertical) {
			t.Error("a pop-up button expands")
		}
		var got *PopUpButton = b.SetSelected(2)
		if got != b || b.Selected() != 2 || p.Index != 2 || b.Item(2) != "Blue" {
			t.Fatalf("SetSelected(2): %+v", p)
		}
		if b.Item(-1) != "" || b.Item(3) != "" {
			t.Error("Item out of range isn't empty")
		}
		items := b.Items()
		items[0] = "changed"
		if b.Item(0) != "Red" {
			t.Error("Items doesn't return a copy")
		}
		b.SetSelected(5)
		if b.Selected() != -1 || p.Index != -1 {
			t.Errorf("SetSelected(5): %d (peer %d)", b.Selected(), p.Index)
		}

		h := &countingHost{}
		b.host = h
		b.SetSelected(1).SetItems("A", "B") // kept in range
		if b.Selected() != 1 || p.Index != 1 || !slices.Equal(b.Items(), []string{"A", "B"}) || h.n != 1 {
			t.Errorf("SetItems keeping the selection: %d, %v, %d invalidations", b.Selected(), b.Items(), h.n)
		}
		b.SetItems("Only")
		if b.Selected() != 0 || p.Index != 0 {
			t.Errorf("SetItems resetting the selection: %d", b.Selected())
		}
		b.SetItems()
		if b.Selected() != -1 || p.Index != -1 || len(p.Items) != 0 {
			t.Errorf("SetItems(): %d, %v", b.Selected(), p.Items)
		}

		b.SetItems("Viewer", "Editor", "Administrator")
		var picks []int
		unsub := b.OnChange(func(i int) { picks = append(picks, i) })
		p.UserSelect(2)
		p.UserSelect(2)                    // the selected item again: no event
		popUpEvents{b}.SelectionChanged(7) // out of range from the native layer
		p.UserSelect(1)
		if want := []int{2, 1}; !slices.Equal(picks, want) || b.Selected() != 1 {
			t.Errorf("OnChange %v (selected %d), want %v", picks, b.Selected(), want)
		}
		b.SetEnabled(false)
		popUpEvents{b}.SelectionChanged(0)
		b.SetEnabled(true)
		unsub()
		p.UserSelect(0)
		if len(picks) != 2 || b.Selected() != 0 {
			t.Errorf("disabled or unsubscribed pop-up: %v, selected %d", picks, b.Selected())
		}
		if NewPopUpButton().Selected() != -1 {
			t.Error("an empty pop-up has a selection")
		}
	})
}
