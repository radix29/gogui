package aqua

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// The size and paint tests of the Slider (PLAN.md S3.4c); the pointer
// and key tests are in slider_input_test.go.

func newSlider(t *testing.T) *Slider {
	t.Helper()
	s := startDriver(t).NewSlider(nil).(*Slider)
	s.SetRange(0, 100)
	return s
}

func TestSliderIsReal(t *testing.T) {
	s := newSlider(t)
	if e := ElementOf(s); e == nil || e != Element(s.view) || s.StubPeer() != s.Slider || s.Stub() != &s.Slider.Base {
		t.Error("ElementOf, StubPeer or Stub")
	}
	s.SetValue(40)
	if got := nodeOf(s); got != `slider "" value="40" {increment decrement}` {
		t.Errorf("node %q", got)
	}
	s.UserDrag(12.5)
	if got := nodeOf(s); got != `slider "" value="12.5" {increment decrement}` {
		t.Errorf("node after a drag %q", got)
	}
	if e := ElementOf(s).base(); !e.Focusable() || e.FocusKind() != FocusControl {
		t.Error("not focusable as a control")
	}
}

func TestSliderFittingSize(t *testing.T) {
	s := newSlider(t)
	for _, c := range []struct {
		cs       driver.ControlSize
		ticks    int
		vertical bool
		want     driver.Size
	}{
		{driver.ControlSizeRegular, 0, false, driver.Size{W: 96, H: 20}}, // the knob across, the stub length along
		{driver.ControlSizeRegular, 5, false, driver.Size{W: 96, H: 28}}, // 20 + 2 + 6
		{driver.ControlSizeRegular, 0, true, driver.Size{W: 20, H: 96}},
		{driver.ControlSizeRegular, 3, true, driver.Size{W: 28, H: 96}},
		{driver.ControlSizeSmall, 0, false, driver.Size{W: 96, H: 14}},
		{driver.ControlSizeSmall, 2, false, driver.Size{W: 96, H: 21}}, // 14 + 2 + 5
		{driver.ControlSizeMini, 0, false, driver.Size{W: 96, H: 11}},
		{driver.ControlSizeMini, 1, false, driver.Size{W: 96, H: 16}}, // 11 + 1 + 4
		{driver.ControlSizeLarge, 0, false, driver.Size{W: 96, H: 24}},
		{driver.ControlSizeLarge, 4, true, driver.Size{W: 34, H: 96}}, // 24 + 3 + 7
	} {
		s.SetControlSize(c.cs)
		s.SetTickMarks(c.ticks, false)
		s.SetVertical(c.vertical)
		if got := s.FittingSize(infSize); got != c.want {
			t.Errorf("%v, %d ticks, vertical %v: %v, want %v", c.cs, c.ticks, c.vertical, got, c.want)
		}
	}
	s.SetControlSize(driver.ControlSizeRegular)
	s.SetTickMarks(0, false)
	s.SetVertical(false)
	if got := ElementOf(s).FittingSize(driver.Size{W: 30, H: 10}); got != (driver.Size{W: 30, H: 10}) {
		t.Errorf("clamped %v", got)
	}
	// A fractional sum is rounded up.
	withControls(t, func(cc *theme.Controls) { cc.SliderKnobSize = theme.SizeTable{19.5, 19.5, 19.5, 19.5} }, func() {
		if got := s.FittingSize(infSize); got != (driver.Size{W: 96, H: 20}) {
			t.Errorf("a fractional knob %v", got)
		}
	})
	s.Fixed = driver.Size{W: 5, H: 6}
	if got := s.FittingSize(infSize); got != s.Fixed {
		t.Errorf("fixed %v", got)
	}
}

// The slider at (10,20) 120x20 in light with the default accent: the
// track 4 pt high across the frame at y 28, the 20 pt knob travelling
// 100 pt from x 10 to x 110 (its left edge), so the value v (0…100) puts
// it at x 10+v.
const (
	sliderTrack       = "fillRRect (10,28,120,4) r=2 solid #00000026"
	sliderKnobShadow  = "shadow (35,20,20,20) r=10 off=(0,0.5) blur=1.5 #0000004d"
	sliderKnob        = "fillRRect (35,20,20,20) r=10 solid #ffffff"
	sliderKnobBorder  = "strokeRRect (35.25,20.25,19.5,19.5) r=9.75 w=0.5 solid #00000026"
	sliderFill25      = "fillRRect (10,28,35,4) r=2 solid #007aff"
	sliderFill25Inact = "fillRRect (10,28,35,4) r=2 solid #cbcbcb"
)

func TestSliderPaint(t *testing.T) {
	s := newSlider(t)
	r := driver.Rect{X: 10, Y: 20, W: 120, H: 20}
	s.SetValue(25)
	checkDump(t, paintPeer(t, s, r), sliderTrack, sliderFill25, sliderKnobShadow, sliderKnob, sliderKnobBorder)

	// At the minimum the filled part ends at the knob's centre, half a
	// knob in; at the maximum it reaches half a knob from the end.
	s.SetValue(0)
	checkDump(t, paintPeer(t, s, r), sliderTrack,
		"fillRRect (10,28,10,4) r=2 solid #007aff",
		"shadow (10,20,20,20) r=10 off=(0,0.5) blur=1.5 #0000004d",
		"fillRRect (10,20,20,20) r=10 solid #ffffff",
		"strokeRRect (10.25,20.25,19.5,19.5) r=9.75 w=0.5 solid #00000026")
	s.SetValue(100)
	checkDump(t, paintPeer(t, s, r), sliderTrack,
		"fillRRect (10,28,110,4) r=2 solid #007aff",
		"shadow (110,20,20,20) r=10 off=(0,0.5) blur=1.5 #0000004d",
		"fillRRect (110,20,20,20) r=10 solid #ffffff",
		"strokeRRect (110.25,20.25,19.5,19.5) r=9.75 w=0.5 solid #00000026")
	// An empty range puts the knob at the minimum.
	s.SetRange(5, 5)
	if got := paintPeer(t, s, r); !strings.Contains(got, "fillRRect (10,20,20,20) r=10 solid #ffffff") {
		t.Errorf("empty range:\n%s", got)
	}
	s.SetRange(0, 100)
	s.SetValue(25)

	// Disabled: every color at ValueDisabledAlpha, no layer.
	s.SetEnabled(false)
	checkDump(t, paintPeer(t, s, r),
		"fillRRect (10,28,120,4) r=2 solid #00000013",
		"fillRRect (10,28,35,4) r=2 solid #007aff80",
		"shadow (35,20,20,20) r=10 off=(0,0.5) blur=1.5 #00000027",
		"fillRRect (35,20,20,20) r=10 solid #ffffff80",
		"strokeRRect (35.25,20.25,19.5,19.5) r=9.75 w=0.5 solid #00000013")
	s.SetEnabled(true)

	// A taller frame centres the knob; the track runs through its centre.
	got := paintPeer(t, s, driver.Rect{X: 10, Y: 20, W: 120, H: 40})
	if !strings.Contains(got, "fillRRect (10,38,120,4) r=2") || !strings.Contains(got, "fillRRect (35,30,20,20) r=10 solid #ffffff") {
		t.Errorf("tall frame:\n%s", got)
	}
	// A frame lower than the knob shrinks it; shorter than the knob, the
	// knob has no travel and stays at the minimum. An empty frame draws
	// nothing.
	got = paintPeer(t, s, driver.Rect{W: 120, H: 10})
	if !strings.Contains(got, "fillRRect (0,3,120,4) r=2") || !strings.Contains(got, "fillRRect (28,0,10,10) r=5 solid #ffffff") {
		t.Errorf("low frame:\n%s", got)
	}
	got = paintPeer(t, s, driver.Rect{W: 12, H: 20})
	if !strings.Contains(got, "fillRRect (0,4,12,12) r=6 solid #ffffff") {
		t.Errorf("short frame:\n%s", got)
	}
	checkDump(t, paintPeer(t, s, driver.Rect{W: 120}))

	// The control size picks the knob and the track.
	s.SetControlSize(driver.ControlSizeMini)
	got = paintPeer(t, s, driver.Rect{W: 111, H: 11})
	if !strings.Contains(got, "fillRRect (0,4,111,3) r=1.5") || !strings.Contains(got, "fillRRect (25,0,11,11) r=5.5 solid #ffffff") {
		t.Errorf("mini slider:\n%s", got)
	}
	s.SetControlSize(driver.ControlSizeRegular)

	// Disabled through an ancestor.
	parent := newBox("parent", 0, 0, 200, 100)
	parent.disabled = true
	treeChange() // set without SetEnabled
	parent.AddChild(s.view)
	if got := paintPeer(t, s, r); !strings.Contains(got, "solid #ffffff80") {
		t.Errorf("disabled ancestor:\n%s", got)
	}
}

func TestSliderTickMarks(t *testing.T) {
	s := newSlider(t)
	s.SetValue(25)
	s.SetTickMarks(5, false)
	// The band (knob, gap, marks: 28 pt) fills the frame: the knob at the
	// top, the marks 2 pt below it, one per quarter of the travel.
	checkDump(t, paintPeer(t, s, driver.Rect{X: 10, Y: 20, W: 120, H: 28}), sliderTrack, sliderFill25,
		"fillRRect (19,42,2,6) r=1 solid #00000040",
		"fillRRect (44,42,2,6) r=1 solid #00000040",
		"fillRRect (69,42,2,6) r=1 solid #00000040",
		"fillRRect (94,42,2,6) r=1 solid #00000040",
		"fillRRect (119,42,2,6) r=1 solid #00000040",
		sliderKnobShadow, sliderKnob, sliderKnobBorder)
	// A taller frame centres the band.
	got := paintPeer(t, s, driver.Rect{X: 10, Y: 20, W: 120, H: 38})
	if !strings.Contains(got, "fillRRect (35,25,20,20) r=10 solid #ffffff") || !strings.Contains(got, "fillRRect (19,47,2,6) r=1") {
		t.Errorf("tall frame with marks:\n%s", got)
	}
	// One mark sits in the middle.
	s.SetTickMarks(1, false)
	got = paintPeer(t, s, driver.Rect{X: 10, Y: 20, W: 120, H: 28})
	if strings.Count(got, "solid #00000040") != 1 || !strings.Contains(got, "fillRRect (69,42,2,6) r=1 solid #00000040") {
		t.Errorf("one mark:\n%s", got)
	}
	// Marks cut off by a low frame are not drawn.
	got = paintPeer(t, s, driver.Rect{X: 10, Y: 20, W: 120, H: 20})
	if strings.Contains(got, "#00000040") {
		t.Errorf("marks outside a low frame:\n%s", got)
	}
}

func TestSliderVertical(t *testing.T) {
	s := newSlider(t)
	s.SetValue(25)
	s.SetVertical(true)
	// The minimum at the bottom: the knob's top at 20+100-25, the filled
	// part from the knob's centre down to the track's end.
	checkDump(t, paintPeer(t, s, driver.Rect{X: 10, Y: 20, W: 20, H: 120}),
		"fillRRect (18,20,4,120) r=2 solid #00000026",
		"fillRRect (18,105,4,35) r=2 solid #007aff",
		"shadow (10,95,20,20) r=10 off=(0,0.5) blur=1.5 #0000004d",
		"fillRRect (10,95,20,20) r=10 solid #ffffff",
		"strokeRRect (10.25,95.25,19.5,19.5) r=9.75 w=0.5 solid #00000026")
	// The marks after the knob, from the bottom up.
	s.SetTickMarks(3, false)
	got := paintPeer(t, s, driver.Rect{X: 10, Y: 20, W: 28, H: 120})
	for _, want := range []string{
		"fillRRect (32,129,6,2) r=1 solid #00000040",
		"fillRRect (32,79,6,2) r=1 solid #00000040",
		"fillRRect (32,29,6,2) r=1 solid #00000040",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("vertical marks: no %q in\n%s", want, got)
		}
	}
	if i, j := strings.Index(got, "fillRRect (32,129,6,2)"), strings.Index(got, "fillRRect (32,29,6,2)"); i > j {
		t.Errorf("marks not from the minimum up:\n%s", got)
	}
}

func TestSliderRedraws(t *testing.T) {
	s := newSlider(t)
	e := ElementOf(s).base()
	for name, set := range map[string]func(){
		"SetRange":       func() { s.SetRange(0, 10) },
		"SetValue":       func() { s.SetValue(3) },
		"SetTickMarks":   func() { s.SetTickMarks(4, true) },
		"SetVertical":    func() { s.SetVertical(true) },
		"SetControlSize": func() { s.SetControlSize(driver.ControlSizeSmall) },
		"UserDrag":       func() { s.Val = s.Min; s.UserDrag(s.Max) }, // a change in any order
	} {
		e.dirty = false
		set()
		if !e.dirty {
			t.Errorf("%s did not invalidate", name)
		}
	}
}

// sliderWindow opens a key window with a slider of range 0…100 at value
// 25, framed at (10,10) 120x20 of the content (surface y 38, under the
// 28 pt title bar): its knob at surface (35,38) 20x20.
func sliderWindow(t *testing.T) (*Slider, *Window, *fake.Surface, *fake.Shell) {
	t.Helper()
	s := startShell(t)
	d := newDriver(s)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	sl := d.NewSlider(nil).(*Slider)
	sl.SetRange(0, 100)
	sl.SetValue(25)
	w.SetContent(sl)
	sl.SetFrame(driver.Rect{X: 10, Y: 10, W: 120, H: 20})
	return sl, w, sf, s
}

func TestSliderFocusRing(t *testing.T) {
	sl, w, sf, s := sliderWindow(t)
	if !sl.view.Focus() {
		t.Fatal("focus")
	}
	w.Show()
	s.FakeClock().Flush()
	lines := strings.Split(controlLines(paint(t, s, sf)), "\n")
	ring := "strokeRRect (33.5,36.5,23,23) r=11.5 w=3 solid #007aff80"
	if !strings.HasPrefix(lines[len(lines)-1], ring) {
		t.Errorf("no ring around the knob, painted last:\n%s", strings.Join(lines, "\n"))
	}
	s.Deactivate()
	if got := paint(t, s, sf); strings.Contains(got, ring) {
		t.Errorf("ring in an inactive window:\n%s", got)
	}
}

func TestSliderInWindow(t *testing.T) {
	sl, w, sf, s := sliderWindow(t)
	tree := AccessibilityTree(w.Root())
	if n := tree.Find(a11y.RoleSlider, ""); n == nil || n.Frame != (driver.Rect{X: 10, Y: 38, W: 120, H: 20}) ||
		n.Value != "25" || n.States != a11y.StateFocusable || n.Actions&(a11y.ActionIncrement|a11y.ActionDecrement) == 0 {
		t.Errorf("a11y tree:\n%s", tree.Dump())
	}
	// Inactive: the filled part grey.
	s.Deactivate()
	if got := paint(t, s, sf); !strings.Contains(got, "fillRRect (10,46,35,4) r=2 solid #cbcbcb") {
		t.Errorf("inactive:\n%s", got)
	}
	// A fractional value is pixel-aligned at scale 2.
	sf.SetScale(2)
	sl.SetValue(25.3)
	if got := paint(t, s, sf); !strings.Contains(got, "fillRRect (35.5,38,20,20) r=10") {
		t.Errorf("scale 2:\n%s", got)
	}
	sl.SetHidden(true)
	if got := paint(t, s, sf); strings.Contains(got, "r=10 solid #ffffff") {
		t.Errorf("hidden slider painted:\n%s", got)
	}
	sl.Destroy()
	if !sl.Destroyed || ElementOf(sl).base().Parent() != nil {
		t.Error("Destroy")
	}
}

// TestSliderStates pins every state (normal, pressed, disabled, focused;
// with and without tick marks; horizontal and vertical) in light and dark,
// in a key and an inactive window, with and without Increase Contrast
// (S6.1e), as record dumps in testdata/slider.states.
func TestSliderStates(t *testing.T) {
	var out strings.Builder
	for _, contrast := range []bool{false, true} {
		for _, dark := range []bool{false, true} {
			for _, key := range []bool{true, false} {
				for _, st := range []struct {
					name              string
					vertical          bool
					ticks             int
					pressed           bool
					disabled, focused bool
				}{
					{"normal", false, 0, false, false, false},
					{"pressed", false, 0, true, false, false},
					{"disabled", false, 0, false, true, false},
					{"focused", false, 0, false, false, true},
					{"tick marks", false, 3, false, false, false},
					{"vertical", true, 0, false, false, false},
					{"vertical tick marks pressed", true, 3, true, false, false},
				} {
					sl, w, sf, s := sliderWindow(t)
					w.SetAppearance(driver.Appearance{Dark: dark, Accent: driver.DefaultAccent, IncreaseContrast: contrast})
					sl.SetVertical(st.vertical)
					sl.SetTickMarks(st.ticks, false)
					knob := pt(45, 48) // the knob's centre at value 25
					if st.vertical {
						sl.SetFrame(driver.Rect{X: 10, Y: 10, W: 28, H: 60})
						knob = pt(20, 38+50-10)
					} else if st.ticks > 0 {
						sl.SetFrame(driver.Rect{X: 10, Y: 10, W: 120, H: 28})
					}
					sl.SetEnabled(!st.disabled)
					if st.focused && !sl.view.Focus() {
						t.Fatal("focus")
					}
					if st.pressed {
						sf.Press(knob, shell.ButtonPrimary)
					}
					if !key {
						s.Deactivate()
					}
					state := sl.view.State()
					fmt.Fprintf(&out, "## %s, %s\n%s\n", st.name, state, controlLines(paint(t, s, sf)))
				}
			}
		}
	}
	got := out.String()
	path := filepath.Join("testdata", "slider.states")
	if *updateGolden {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to pin the dump)", err)
	}
	if w := strings.ReplaceAll(string(want), "\r\n", "\n"); got != w {
		t.Errorf("slider states differ from %s (rerun with -update if the change is intended):\n%s", path, got)
	}
}
