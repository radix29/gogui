package aqua

import (
	"fmt"
	"math"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// inputSliderLog records the values a SliderHandler is called with.
type inputSliderLog struct{ values *[]float64 }

func (l inputSliderLog) ValueChanged(v float64) { *l.values = append(*l.values, v) }

// inputSliderWindow opens a key window (320x320) with a horizontal slider
// at frame (10,10) 276x20 of the content, range 0…256. The knob is 20 pt,
// so its centre travels 256 pt: the value v is at surface x 20+v, and the
// knob's line at surface y 48 (under the 28 pt title bar).
func inputSliderWindow(t *testing.T, values *[]float64) (*Slider, *Window, *fake.Surface, *fake.Shell) {
	t.Helper()
	s := startShell(t)
	d := newDriver(s)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 320, H: 320}, Style: allStyles})
	var h driver.SliderHandler
	if values != nil {
		h = inputSliderLog{values}
	}
	sl := d.NewSlider(h).(*Slider)
	sl.SetRange(0, 256)
	w.SetContent(sl)
	sl.SetFrame(driver.Rect{X: 10, Y: 10, W: 276, H: 20})
	return sl, w, sf, s
}

// inputAt returns the surface point of value v on inputSliderWindow's
// slider, dx off the knob's centre along it.
func inputAt(v, dx float64) driver.Point { return pt(20+v+dx, 48) }

func inputNear(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// inputWant checks the slider's value and the handler's calls so far.
func inputWant(t *testing.T, s *Slider, values []float64, v float64, calls ...float64) {
	t.Helper()
	ok := inputNear(s.Value(), v) && len(values) == len(calls)
	for i := 0; ok && i < len(calls); i++ {
		ok = inputNear(values[i], calls[i])
	}
	if !ok {
		t.Fatalf("value %v, calls %v; want %v, %v", s.Value(), values, v, calls)
	}
}

func inputPressed(s *Slider) bool { return s.view.State().Interaction == InteractionPressed }

func TestSliderDragKnob(t *testing.T) {
	var vals []float64
	s, _, sf, _ := inputSliderWindow(t, &vals)
	s.SetValue(128)

	// A press on the knob, 5 pt right of its centre, grabs it there: no
	// jump.
	sf.Press(inputAt(128, 5), shell.ButtonPrimary)
	inputWant(t, s, vals, 128)
	if !s.view.press.Down() || !inputPressed(s) {
		t.Fatal("press on the knob: not dragging, or not drawn pressed")
	}
	// Moving follows the pointer, keeping the grab offset.
	sf.Move(inputAt(138, 5))
	inputWant(t, s, vals, 138, 138)
	sf.Move(inputAt(100, 5))
	inputWant(t, s, vals, 100, 138, 100)
	// A move across the slider changes nothing: no call.
	sf.Move(pt(20+100+5, 40))
	inputWant(t, s, vals, 100, 138, 100)
	// Off the frame (below it, then outside the surface) it still follows,
	// and the knob stays pressed.
	sf.Move(pt(20+64+5, 200))
	inputWant(t, s, vals, 64, 138, 100, 64)
	if !inputPressed(s) {
		t.Fatal("dragged off the frame: not drawn pressed")
	}
	// Past the ends it clamps; once clamped, further moves call nothing.
	sf.Move(pt(-50, 48))
	inputWant(t, s, vals, 0, 138, 100, 64, 0)
	sf.Move(pt(-80, 300))
	inputWant(t, s, vals, 0, 138, 100, 64, 0)
	sf.Move(pt(500, 48))
	inputWant(t, s, vals, 256, 138, 100, 64, 0, 256)
	sf.Move(pt(600, 48))
	inputWant(t, s, vals, 256, 138, 100, 64, 0, 256)
	if !inputPressed(s) {
		t.Fatal("dragged outside the surface: not drawn pressed")
	}

	// A secondary press and release during the drag are swallowed: the
	// drag goes on.
	sf.Move(inputAt(200, 5))
	inputWant(t, s, vals, 200, 138, 100, 64, 0, 256, 200)
	sf.Press(inputAt(200, 5), shell.ButtonSecondary)
	sf.Release(inputAt(200, 5), shell.ButtonSecondary)
	if !s.view.press.Down() || !inputPressed(s) {
		t.Fatal("a secondary click ended the drag")
	}
	sf.Move(inputAt(190, 5))
	inputWant(t, s, vals, 190, 138, 100, 64, 0, 256, 200, 190)

	// The release ends it: no longer pressed, moves change nothing.
	sf.Release(inputAt(180, 5), shell.ButtonPrimary)
	inputWant(t, s, vals, 180, 138, 100, 64, 0, 256, 200, 190, 180)
	if s.view.press.Down() || inputPressed(s) {
		t.Fatal("released: still dragging")
	}
	sf.Move(inputAt(50, 0))
	inputWant(t, s, vals, 180, 138, 100, 64, 0, 256, 200, 190, 180)

	// A press without a move changes nothing (on the knob's centre).
	sf.Click(inputAt(180, 0))
	inputWant(t, s, vals, 180, 138, 100, 64, 0, 256, 200, 190, 180)
}

func TestSliderPressTrack(t *testing.T) {
	var vals []float64
	s, _, sf, _ := inputSliderWindow(t, &vals)
	// The knob is at 0 (surface x 10…30); a press on the track jumps it
	// under the pointer, then the drag goes on from there without an
	// offset.
	sf.Press(inputAt(150, 0), shell.ButtonPrimary)
	inputWant(t, s, vals, 150, 150)
	if !inputPressed(s) {
		t.Fatal("press on the track: not drawn pressed")
	}
	sf.Move(inputAt(160, 0))
	inputWant(t, s, vals, 160, 150, 160)
	sf.Release(inputAt(160, 0), shell.ButtonPrimary)
	inputWant(t, s, vals, 160, 150, 160)

	// Anywhere in the frame counts as the track: above the track line, at
	// the frame's ends (clamped to the range).
	sf.Click(pt(20+40, 39))
	inputWant(t, s, vals, 40, 150, 160, 40)
	sf.Click(pt(285, 48))
	inputWant(t, s, vals, 256, 150, 160, 40, 256)
	sf.Click(pt(11, 48))
	inputWant(t, s, vals, 0, 150, 160, 40, 256, 0)

	// Other buttons are ignored.
	sf.ClickButton(inputAt(100, 0), shell.ButtonSecondary)
	sf.ClickButton(inputAt(100, 0), shell.ButtonMiddle)
	inputWant(t, s, vals, 0, 150, 160, 40, 256, 0)
	if s.view.press.Down() {
		t.Fatal("a secondary press started a drag")
	}
}

func TestSliderDragCancel(t *testing.T) {
	var vals []float64
	s, _, sf, _ := inputSliderWindow(t, &vals)

	// Disabling during a drag ends it; moves and the release change nothing.
	sf.Press(inputAt(100, 0), shell.ButtonPrimary)
	inputWant(t, s, vals, 100, 100)
	s.SetEnabled(false)
	if s.view.press.Down() || inputPressed(s) {
		t.Fatal("disabled during the drag: still dragging")
	}
	sf.Move(inputAt(150, 0))
	s.SetEnabled(true)
	sf.Move(inputAt(160, 0))
	sf.Release(inputAt(170, 0), shell.ButtonPrimary)
	inputWant(t, s, vals, 100, 100)

	// Hiding it during a drag sends PointerCancel to the enabled slider.
	sf.Press(inputAt(50, 0), shell.ButtonPrimary)
	inputWant(t, s, vals, 50, 100, 50)
	s.SetHidden(true)
	if s.view.press.Down() || inputPressed(s) {
		t.Fatal("hidden during the drag: still dragging")
	}
	sf.Move(inputAt(60, 0))
	s.SetHidden(false)
	sf.Move(inputAt(70, 0))
	sf.Release(inputAt(70, 0), shell.ButtonPrimary)
	inputWant(t, s, vals, 50, 100, 50)

	// A PointerCancel delivered straight to the view ends the drag too.
	sf.Press(inputAt(20, 0), shell.ButtonPrimary)
	inputWant(t, s, vals, 20, 100, 50, 20)
	if s.view.HandleEvent(PointerEvent{Kind: PointerCancel}) || s.view.press.Down() || inputPressed(s) {
		t.Fatal("PointerCancel: handled, or still dragging")
	}
	sf.Release(inputAt(20, 0), shell.ButtonPrimary)
	inputWant(t, s, vals, 20, 100, 50, 20)

	// It works again afterwards.
	sf.Click(inputAt(30, 0))
	inputWant(t, s, vals, 30, 100, 50, 20, 30)
}

func TestSliderVerticalDrag(t *testing.T) {
	var vals []float64
	s, _, sf, _ := inputSliderWindow(t, &vals)
	s.SetVertical(true)
	s.SetFrame(driver.Rect{X: 10, Y: 10, W: 20, H: 276})
	// The minimum at the bottom: the value v at surface y 28+10+266-v.
	at := func(v float64) driver.Point { return pt(20, 304-v) }
	sf.Press(at(0), shell.ButtonPrimary) // on the knob at the bottom: no jump
	inputWant(t, s, vals, 0)
	sf.Move(at(100))
	inputWant(t, s, vals, 100, 100)
	sf.Move(at(64)) // down is smaller
	inputWant(t, s, vals, 64, 100, 64)
	sf.Move(pt(200, at(64).Y)) // across: no change
	inputWant(t, s, vals, 64, 100, 64)
	sf.Move(pt(20, 0)) // above the top: the maximum
	inputWant(t, s, vals, 256, 100, 64, 256)
	sf.Move(pt(20, 400)) // below the bottom: the minimum
	inputWant(t, s, vals, 0, 100, 64, 256, 0)
	sf.Release(pt(20, 400), shell.ButtonPrimary)
	// A press on the track jumps.
	sf.Click(at(200))
	inputWant(t, s, vals, 200, 100, 64, 256, 0, 200)
}

func TestSliderSnap(t *testing.T) {
	var vals []float64
	s, _, sf, _ := inputSliderWindow(t, &vals)
	// Five marks: 0, 64, 128, 192, 256.
	s.SetTickMarks(5, true)
	sf.Press(inputAt(70, 0), shell.ButtonPrimary) // on the track: jumps to the nearest mark
	inputWant(t, s, vals, 64, 64)
	sf.Move(inputAt(100, 0))
	inputWant(t, s, vals, 128, 64, 128)
	sf.Move(inputAt(95, 0)) // nearer 64
	inputWant(t, s, vals, 64, 64, 128, 64)
	sf.Move(inputAt(60, 0)) // the same mark: no call
	inputWant(t, s, vals, 64, 64, 128, 64)
	sf.Move(inputAt(1000, 0))
	inputWant(t, s, vals, 256, 64, 128, 64, 256)
	sf.Release(inputAt(1000, 0), shell.ButtonPrimary)
	if s.Step() != 64 || s.PageStep() != 64 {
		t.Fatalf("snapping steps %v, %v; want one mark (64)", s.Step(), s.PageStep())
	}

	// One mark: in the middle, every drag snaps there.
	vals = nil
	s.SetTickMarks(1, true)
	s.SetValue(0)
	sf.Click(inputAt(20, 0))
	inputWant(t, s, vals, 128, 128)
	sf.Press(inputAt(128, 0), shell.ButtonPrimary)
	sf.Move(inputAt(250, 0))
	sf.Move(inputAt(3, 0))
	sf.Release(inputAt(3, 0), shell.ButtonPrimary)
	inputWant(t, s, vals, 128, 128)
	if !inputNear(s.Step(), 256.0/20) {
		t.Fatalf("one mark: Step %v, want range/20", s.Step())
	}

	// Marks without snapping: the value follows freely.
	vals = nil
	s.SetTickMarks(5, false)
	sf.Click(inputAt(70, 0))
	inputWant(t, s, vals, 70, 70)
	if !inputNear(s.Step(), 256.0/20) || !inputNear(s.PageStep(), 256.0/5) {
		t.Fatalf("no snap: steps %v, %v", s.Step(), s.PageStep())
	}
}

func TestSliderEmptyRange(t *testing.T) {
	var vals []float64
	s, w, sf, s0 := inputSliderWindow(t, &vals)
	if !s.view.Focus() {
		t.Fatal("focus")
	}
	w.Show()
	s0.FakeClock().Flush()
	for _, r := range [][2]float64{{50, 50}, {10, 0}} {
		s.SetRange(r[0], r[1])
		if s.Value() != r[0] || s.Step() != 0 || s.PageStep() != 0 {
			t.Fatalf("range %v: value %v, steps %v, %v", r, s.Value(), s.Step(), s.PageStep())
		}
		sf.Press(inputAt(100, 0), shell.ButtonPrimary)
		sf.Move(inputAt(200, 0))
		sf.Move(inputAt(-200, 0))
		sf.Release(inputAt(0, 0), shell.ButtonPrimary)
		for _, k := range []driver.Key{driver.KeyRight, driver.KeyLeft, driver.KeyPageUp, driver.KeyHome, driver.KeyEnd} {
			if !sf.Key(k, 0) {
				t.Errorf("range %v: %v not consumed", r, k)
			}
		}
		s.view.Increment()
		s.view.Decrement()
		inputWant(t, s, vals, r[0])
	}
}

func TestSliderDisabled(t *testing.T) {
	var vals []float64
	s, w, sf, sh := inputSliderWindow(t, &vals)
	s.SetValue(100)
	if !s.view.Focus() {
		t.Fatal("focus")
	}
	w.Show()
	sh.FakeClock().Flush()
	s.SetEnabled(false)
	sf.Click(inputAt(200, 0))
	sf.Press(inputAt(100, 0), shell.ButtonPrimary)
	sf.Move(inputAt(150, 0))
	if s.view.press.Down() || inputPressed(s) {
		t.Fatal("disabled: dragging")
	}
	sf.Release(inputAt(150, 0), shell.ButtonPrimary)
	for _, k := range []driver.Key{driver.KeyRight, driver.KeyEnd} {
		if sf.Key(k, 0) {
			t.Errorf("disabled: %v consumed", k)
		}
	}
	if s.view.Increment() || s.view.Decrement() {
		t.Fatal("Increment or Decrement while disabled")
	}
	s.UserDrag(10)
	inputWant(t, s, vals, 100)
}

func TestSliderKeys(t *testing.T) {
	var vals []float64
	s, w, sf, sh := inputSliderWindow(t, &vals)
	s.SetRange(0, 100)
	s.SetValue(50)
	if !s.view.Focus() {
		t.Fatal("focus")
	}
	w.Show()
	sh.FakeClock().Flush()
	if s.Step() != 5 || s.PageStep() != 20 {
		t.Fatalf("steps %v, %v; want 5, 20", s.Step(), s.PageStep())
	}
	var want []float64
	for _, c := range []struct {
		key driver.Key
		v   float64
	}{
		{driver.KeyRight, 55},
		{driver.KeyUp, 60},
		{driver.KeyLeft, 55},
		{driver.KeyDown, 50},
		{driver.KeyPageUp, 70},
		{driver.KeyPageDown, 50},
		{driver.KeyHome, 0},
		{driver.KeyLeft, 0}, // at the minimum: consumed, no change
		{driver.KeyDown, 0},
		{driver.KeyPageDown, 0},
		{driver.KeyHome, 0},
		{driver.KeyEnd, 100},
		{driver.KeyRight, 100}, // at the maximum
		{driver.KeyUp, 100},
		{driver.KeyPageUp, 100},
		{driver.KeyPageDown, 80},
		{driver.KeyPageUp, 100},
	} {
		changed := !inputNear(c.v, s.Value())
		if !sf.Key(c.key, 0) {
			t.Fatalf("%v not consumed at %v", c.key, s.Value())
		}
		if changed {
			want = append(want, c.v)
		}
		inputWant(t, s, vals, c.v, want...)
	}
	// Near an end a step clamps.
	s.SetValue(97)
	if !sf.Key(driver.KeyRight, 0) {
		t.Fatal("Right not consumed")
	}
	want = append(want, 100)
	inputWant(t, s, vals, 100, want...)

	// With modifiers, or other keys: not consumed, no change.
	for _, k := range []struct {
		key  driver.Key
		mods driver.Modifiers
	}{
		{driver.KeyLeft, driver.ModShift},
		{driver.KeyLeft, driver.ModOption},
		{driver.KeyHome, driver.ModControl},
		{driver.KeyPageDown, driver.ModShift},
		{driver.KeyEnd, driver.ModCommand},
		{driver.Key('A'), 0},
		{driver.KeyReturn, 0},
	} {
		if sf.Key(k.key, k.mods) {
			t.Errorf("%v with %v consumed", k.key, k.mods)
		}
	}
	inputWant(t, s, vals, 100, want...)

	// The key-up alone does nothing.
	sf.KeyUp(driver.KeyLeft, 0)
	inputWant(t, s, vals, 100, want...)

	// Snapping to five marks: a step is one mark (25), Page too.
	s.SetTickMarks(5, true)
	s.SetValue(50)
	for _, c := range []struct {
		key driver.Key
		v   float64
	}{
		{driver.KeyRight, 75}, {driver.KeyLeft, 50}, {driver.KeyPageDown, 25}, {driver.KeyPageUp, 50},
	} {
		sf.Key(c.key, 0)
		want = append(want, c.v)
		inputWant(t, s, vals, c.v, want...)
	}
	// A value between marks steps to the mark nearest value±step.
	s.SetValue(60)
	sf.Key(driver.KeyRight, 0)
	want = append(want, 75)
	inputWant(t, s, vals, 75, want...)
}

func TestSliderIncrementDecrement(t *testing.T) {
	var vals []float64
	s, _, _, _ := inputSliderWindow(t, &vals)
	s.SetRange(-40, 100) // the converter's range: Step 7
	s.SetValue(20)
	if !s.view.Increment() {
		t.Fatal("Increment refused")
	}
	inputWant(t, s, vals, 27, 27)
	for range 2 {
		if !s.view.Decrement() {
			t.Fatal("Decrement refused")
		}
	}
	inputWant(t, s, vals, 13, 27, 20, 13)
	// Clamped at the ends; at an end nothing is reported.
	s.SetValue(98)
	s.view.Increment()
	s.view.Increment()
	inputWant(t, s, vals, 100, 27, 20, 13, 100)
	// Snapping: one mark.
	s.SetTickMarks(3, true) // -40, 30, 100
	s.view.Decrement()
	inputWant(t, s, vals, 30, 27, 20, 13, 100, 30)
	if got := fmt.Sprint(s.view.AccessibleNode().Value); got != "30" {
		t.Errorf("a11y value %q", got)
	}
}

// inputSliderFunc is a SliderHandler calling a func.
type inputSliderFunc func(float64)

func (f inputSliderFunc) ValueChanged(v float64) { f(v) }

func TestSliderExactEnds(t *testing.T) {
	var vals []float64
	s, w, sf, sh := inputSliderWindow(t, &vals)
	if !s.view.Focus() {
		t.Fatal("focus")
	}
	w.Show()
	sh.FakeClock().Flush()

	// Range 0…1: three steps up and three down end exactly at 0, twenty up
	// exactly at 1 (no float drift short of an end).
	s.SetRange(0, 1)
	s.SetValue(0)
	for range 3 {
		sf.Key(driver.KeyRight, 0)
	}
	for range 3 {
		sf.Key(driver.KeyLeft, 0)
	}
	if s.Value() != 0 || len(vals) != 6 || vals[5] != 0 {
		t.Fatalf("Right×3, Left×3: value %v, calls %v; want exactly 0", s.Value(), vals)
	}
	for range 20 {
		sf.Key(driver.KeyUp, 0)
	}
	if s.Value() != 1 || vals[len(vals)-1] != 1 {
		t.Fatalf("Up×20: value %v (%v), want exactly 1", s.Value(), vals[len(vals)-1])
	}
	n := len(vals)
	sf.Key(driver.KeyRight, 0) // at the end: no drift past it, no call
	if s.Value() != 1 || len(vals) != n {
		t.Fatalf("Right at the end: value %v, calls %v", s.Value(), vals[n:])
	}

	// Range -0.3…0.9: dragged to either end, the value is exactly the end.
	vals = nil
	s.SetRange(-0.3, 0.9)
	s.SetValue(0.1)
	sf.Press(inputAt(128, 0), shell.ButtonPrimary) // on the track (the knob is at 0.1, x ≈ 63)
	sf.Move(pt(1000, 48))
	if s.Value() != 0.9 || vals[len(vals)-1] != 0.9 {
		t.Fatalf("dragged to the right end: value %v, calls %v; want exactly 0.9", s.Value(), vals)
	}
	sf.Move(pt(-1000, 48))
	if s.Value() != -0.3 || vals[len(vals)-1] != -0.3 {
		t.Fatalf("dragged to the left end: value %v, calls %v; want exactly -0.3", s.Value(), vals)
	}
	sf.Release(pt(-1000, 48), shell.ButtonPrimary)
	// Page steps from an end land exactly on the other.
	for range 5 {
		sf.Key(driver.KeyPageUp, 0)
	}
	if s.Value() != 0.9 {
		t.Fatalf("PageUp×5: value %v, want exactly 0.9", s.Value())
	}
	// The last tick mark is exactly the maximum.
	s.SetTickMarks(7, true)
	s.SetValue(-0.3)
	for range 6 {
		sf.Key(driver.KeyRight, 0)
	}
	if s.Value() != 0.9 {
		t.Fatalf("snapping Right×6: value %v, want exactly 0.9", s.Value())
	}
}

func TestSliderPressKnobNoRoundTrip(t *testing.T) {
	var vals []float64
	s, _, sf, _ := inputSliderWindow(t, &vals)
	// A value whose fraction does not round-trip through the pointer
	// coordinate: a press on the knob (centre or edge) keeps it exactly,
	// and so does a move across the slider. Only a move along changes it.
	s.SetRange(-0.3, 0.9)
	s.SetValue(0.1)
	c := 20 + 256*(0.1+0.3)/1.2 // the knob's centre, surface x
	for _, dx := range []float64{0, 3, -7} {
		sf.Press(pt(c+dx, 48), shell.ButtonPrimary)
		sf.Move(pt(c+dx, 44)) // across
		sf.Move(pt(c+dx, 200))
		if !inputPressed(s) {
			t.Fatal("not pressed")
		}
		sf.Release(pt(c+dx, 200), shell.ButtonPrimary)
		if s.Value() != 0.1 || len(vals) != 0 {
			t.Fatalf("press on the knob at %+v: value %v, calls %v; want exactly 0.1, none", dx, s.Value(), vals)
		}
	}
	sf.Press(pt(c, 48), shell.ButtonPrimary)
	sf.Move(pt(c+64, 48))
	sf.Release(pt(c+64, 48), shell.ButtonPrimary)
	if !inputNear(s.Value(), 0.4) || len(vals) != 1 {
		t.Fatalf("dragged along: value %v, calls %v; want 0.4", s.Value(), vals)
	}
}

func TestSliderHandlerDisablesOnPress(t *testing.T) {
	for _, what := range []string{"disables", "hides", "removes"} {
		t.Run(what, func(t *testing.T) {
			s, w, sf, _ := inputSliderWindow(t, nil)
			var vals []float64
			s.Handler = inputSliderFunc(func(v float64) {
				vals = append(vals, v)
				switch what {
				case "disables":
					s.SetEnabled(false)
				case "hides":
					s.SetHidden(true)
				default:
					w.SetContent(nil) // B1: removed by its own handler
				}
			})
			// A press on the track: the knob jumps, the handler turns it off,
			// no drag starts.
			sf.Press(inputAt(100, 0), shell.ButtonPrimary)
			inputWant(t, s, vals, 100, 100)
			if s.view.press.Down() || inputPressed(s) {
				t.Fatal("dragging a slider its handler turned off")
			}
			s.SetEnabled(true)
			s.SetHidden(false)
			if what == "removes" {
				// Re-added, it must neither draw pressed nor drag with
				// the old grab.
				s.Handler = inputSliderFunc(func(v float64) { vals = append(vals, v) })
				w.SetContent(s)
				if inputPressed(s) {
					t.Fatal("a re-added slider draws pressed")
				}
			}
			sf.Move(inputAt(150, 0))
			sf.Release(inputAt(150, 0), shell.ButtonPrimary)
			inputWant(t, s, vals, 100, 100)
		})
	}
}

func TestSliderUserDrag(t *testing.T) {
	var vals []float64
	s, _, _, _ := inputSliderWindow(t, &vals)
	s.UserDrag(70)
	inputWant(t, s, vals, 70, 70)
	s.UserDrag(70) // unchanged: no call
	s.UserDrag(1000)
	inputWant(t, s, vals, 256, 70, 256)
	s.UserDrag(-5)
	inputWant(t, s, vals, 0, 70, 256, 0)
	s.SetTickMarks(5, true)
	s.UserDrag(70) // snaps
	s.UserDrag(60) // the same mark: no call
	inputWant(t, s, vals, 64, 70, 256, 0, 64)
	s.UserDrag(math.NaN()) // the minimum
	inputWant(t, s, vals, 0, 70, 256, 0, 64, 0)
	// Ignored while disabled, itself or through an ancestor.
	s.SetEnabled(false)
	s.UserDrag(128)
	inputWant(t, s, vals, 0, 70, 256, 0, 64, 0)
	s.SetEnabled(true)
	s.view.Parent().base().SetEnabled(false)
	s.UserDrag(128)
	inputWant(t, s, vals, 0, 70, 256, 0, 64, 0)
	s.view.Parent().base().SetEnabled(true)
	s.UserDrag(128)
	inputWant(t, s, vals, 128, 70, 256, 0, 64, 0, 128)
}
