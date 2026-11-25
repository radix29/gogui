package aqua

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

func newRadio(t *testing.T, title string) *Radio {
	t.Helper()
	r := startDriver(t).NewRadio(nil).(*Radio)
	r.SetTitle(title)
	return r
}

func TestRadioIsReal(t *testing.T) {
	r := newRadio(t, "Admin")
	if e := ElementOf(r); e == nil || e != Element(r.view) || r.StubPeer() != r.Radio || r.Stub() != &r.Radio.Base {
		t.Error("ElementOf, StubPeer or Stub")
	}
	if got, want := nodeOf(r), `radio "Admin" {press}`; got != want {
		t.Errorf("node %q, want %q", got, want)
	}
	r.SetOn(true)
	if got, want := nodeOf(r), `radio "Admin" [checked] {press}`; got != want {
		t.Errorf("node %q, want %q", got, want)
	}
	if e := ElementOf(r).base(); !e.Focusable() || e.FocusKind() != FocusControl {
		t.Error("not focusable as a control")
	}
}

func TestRadioFittingSize(t *testing.T) {
	r := newRadio(t, "Wrap")
	// The fake text system: 7 pt per rune and 16 pt lines at 13 pt,
	// scaling with the size.
	for _, s := range []struct {
		cs    driver.ControlSize
		title string
		want  driver.Size
	}{
		{driver.ControlSizeRegular, "Wrap", driver.Size{W: 48, H: 16}}, // 14 + 6 + 28; the line is higher than the circle
		{driver.ControlSizeRegular, "", driver.Size{W: 14, H: 14}},     // the circle alone
		{driver.ControlSizeSmall, "Wrap", driver.Size{W: 41, H: 14}},   // 12 + 5 + 23.69, rounded up
		{driver.ControlSizeMini, "Wrap", driver.Size{W: 34, H: 12}},    // 10 + 4 + 19.38
		{driver.ControlSizeLarge, "Wrap", driver.Size{W: 51, H: 16}},   // 16 + 7 + 28 (13 pt)
		{driver.ControlSizeRegular, "a\nb", driver.Size{W: 41, H: 16}}, // on one line: "a b"
	} {
		r.SetControlSize(s.cs)
		r.SetTitle(s.title)
		if got := r.FittingSize(infSize); got != s.want {
			t.Errorf("%v %q: %v, want %v", s.cs, s.title, got, s.want)
		}
	}
	r.SetControlSize(driver.ControlSizeRegular)
	r.SetTitle("Wrap")
	if got := ElementOf(r).FittingSize(driver.Size{W: 30, H: 10}); got != (driver.Size{W: 30, H: 10}) {
		t.Errorf("clamped %v", got)
	}
	withControls(t, func(cc *theme.Controls) { cc.RadioSize = theme.SizeTable{20, 20, 20, 20} }, func() {
		if got := r.FittingSize(infSize); got != (driver.Size{W: 54, H: 20}) {
			t.Errorf("a large circle %v", got)
		}
	})
	r.Fixed = driver.Size{W: 5, H: 6}
	if got := r.FittingSize(infSize); got != r.Fixed {
		t.Errorf("fixed %v", got)
	}
}

func TestRadioTitleCache(t *testing.T) {
	r := newRadio(t, "Wrap")
	l := r.titleLayout()
	if l == nil || r.titleLayout() != l {
		t.Fatal("same title, new layout")
	}
	r.SetControlSize(driver.ControlSizeSmall)
	if r.titleLayout() == l {
		t.Error("new size, same layout")
	}
	r.SetTitle("")
	if r.titleLayout() != nil {
		t.Error("a layout for an empty title")
	}
}

// The radio at (10,20) 100x16 in light with the default accent: the
// circle at (10,21) 14x14, the dot 6x6 in its centre, the title at x 30.
const (
	circleOff    = "fillRRect (10,21,14,14) r=7 solid #ffffff"
	circleBorder = "strokeRRect (10.5,21.5,13,13) r=6.5 w=1 solid #00000026"
	circleOn     = "fillRRect (10,21,14,14) r=7 solid #007aff"
	radioDot     = "fillRRect (14,25,6,6) r=3 solid #ffffff"
)

func TestRadioPaint(t *testing.T) {
	r := newRadio(t, "Wrap")
	rect := driver.Rect{X: 10, Y: 20, W: 100, H: 16}
	checkDump(t, paintPeer(t, r, rect), circleOff, circleBorder, wrapTitle)
	r.SetOn(true)
	checkDump(t, paintPeer(t, r, rect), circleOn, radioDot, wrapTitle)

	// Disabled: the circle in a layer at ToggleDisabledAlpha, the disabled
	// text.
	r.SetEnabled(false)
	checkDump(t, paintPeer(t, r, rect), "pushOpacity 0.5", circleOn+" alpha=0.5", radioDot+" alpha=0.5", "pop",
		`text (30,20,28,16) "Wrap" 13pt #00000040`)
	r.SetOn(false)
	checkDump(t, paintPeer(t, r, rect), "pushOpacity 0.5", circleOff+" alpha=0.5", circleBorder+" alpha=0.5", "pop",
		`text (30,20,28,16) "Wrap" 13pt #00000040`)
	r.SetEnabled(true)

	// A taller frame: the circle and the title centred vertically.
	checkDump(t, paintPeer(t, r, driver.Rect{X: 10, Y: 20, W: 100, H: 30}),
		"fillRRect (10,28,14,14) r=7 solid #ffffff",
		"strokeRRect (10.5,28.5,13,13) r=6.5 w=1 solid #00000026",
		`text (30,27,28,16) "Wrap" 13pt #000000d9`)
	// A narrow one truncates the title to the frame.
	r.SetTitle("Send me updates")
	if got := paintPeer(t, r, driver.Rect{W: 50, H: 16}); !strings.Contains(got, `"Sen…"`) {
		t.Errorf("narrow radio:\n%s", got)
	}
	// No room for the title, a frame lower than the title's line, a
	// circle smaller than the token, too small for its border, an empty
	// frame.
	checkDump(t, paintPeer(t, r, driver.Rect{W: 24, H: 16}),
		"fillRRect (0,1,14,14) r=7 solid #ffffff",
		"strokeRRect (0.5,1.5,13,13) r=6.5 w=1 solid #00000026")
	checkDump(t, paintPeer(t, r, driver.Rect{W: 100, H: 6}),
		"fillRRect (0,0,6,6) r=3 solid #ffffff",
		"strokeRRect (0.5,0.5,5,5) r=2.5 w=1 solid #00000026")
	checkDump(t, paintPeer(t, r, driver.Rect{W: 100, H: 2}),
		"fillRRect (0,0,2,2) r=1 solid #ffffff")
	checkDump(t, paintPeer(t, r, driver.Rect{W: 100}))
	// The dot scales with the circle.
	r.SetOn(true)
	checkDump(t, paintPeer(t, r, driver.Rect{W: 100, H: 7}),
		"fillRRect (0,0,7,7) r=3.5 solid #007aff",
		"fillRRect (2,2,3,3) r=1.5 solid #ffffff")
	r.SetOn(false)
	r.SetTitle("")
	checkDump(t, paintPeer(t, r, rect), circleOff, circleBorder)

	// The control size picks the circle and the font.
	r.SetTitle("Wrap")
	r.SetControlSize(driver.ControlSizeMini)
	got := paintPeer(t, r, driver.Rect{W: 50, H: 12})
	if !strings.Contains(got, "fillRRect (0,1,10,10) r=5 solid #ffffff") || !strings.Contains(got, `text (14,0.462,`) ||
		!strings.Contains(got, `"Wrap" 9pt`) {
		t.Errorf("mini radio:\n%s", got)
	}

	// Disabled through an ancestor.
	r.SetControlSize(driver.ControlSizeRegular)
	parent := newBox("parent", 0, 0, 200, 100)
	parent.disabled = true
	treeChange() // set without SetEnabled
	parent.AddChild(r.view)
	if got := paintPeer(t, r, rect); !strings.Contains(got, "pushOpacity 0.5") || !strings.Contains(got, `"Wrap" 13pt #00000040`) {
		t.Errorf("disabled ancestor:\n%s", got)
	}
}

func TestRadioRedraws(t *testing.T) {
	r := newRadio(t, "Wrap")
	e := ElementOf(r).base()
	for name, set := range map[string]func(){
		"SetTitle":       func() { r.SetTitle("Go") },
		"SetOn":          func() { r.SetOn(true) },
		"SetControlSize": func() { r.SetControlSize(driver.ControlSizeSmall) },
		"UserClick":      func() { r.UserClick() },
	} {
		e.dirty = false
		set()
		if !e.dirty {
			t.Errorf("%s did not invalidate", name)
		}
	}
}

// radioWindow opens a key window with a radio at frame (10,10) 100x16 of
// the content (surface y 38, under the 28 pt title bar).
func radioWindow(t *testing.T, title string, states *[]driver.CheckState) (*Radio, *Window, *fake.Surface, *fake.Shell) {
	t.Helper()
	s := startShell(t)
	d := newDriver(s)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	var h driver.ToggleHandler
	if states != nil {
		h = toggleLog{states}
	}
	r := d.NewRadio(h).(*Radio)
	r.SetTitle(title)
	w.SetContent(r)
	r.SetFrame(driver.Rect{X: 10, Y: 10, W: 100, H: 16})
	return r, w, sf, s
}

func TestRadioClick(t *testing.T) {
	var states []driver.CheckState
	r, _, sf, s := radioWindow(t, "Wrap", &states)
	circle, title := pt(17, 46), pt(40, 46) // in surface coordinates
	off := pt(150, 80)
	want := func(on bool, n int) {
		t.Helper()
		if r.IsOn != on || len(states) != n {
			t.Fatalf("on %v after %d calls (%v), want %v after %d", r.IsOn, len(states), states, on, n)
		}
		for _, st := range states {
			if st != driver.StateOn {
				t.Fatalf("handler states %v", states)
			}
		}
	}

	sf.Click(circle)
	want(true, 1)
	// A click on an on radio reports StateOn again and keeps it on.
	sf.Click(title)
	want(true, 2)
	r.SetOn(false)

	// Pressed while held over it, dragged off and back; a release outside
	// does nothing.
	sf.Press(title, shell.ButtonPrimary)
	if r.view.State().Interaction != InteractionPressed {
		t.Fatal("not pressed")
	}
	if got := paint(t, s, sf); !strings.Contains(got, "fillRRect (10,39,14,14) r=7 solid #dedede") {
		t.Errorf("pressed paint:\n%s", got)
	}
	sf.Move(off)
	if r.view.press.Pressed() || !r.view.press.Down() {
		t.Fatal("dragged off: still pressed")
	}
	sf.Move(circle)
	if !r.view.press.Pressed() {
		t.Fatal("dragged back: not pressed")
	}
	sf.Release(off, shell.ButtonPrimary)
	want(false, 2)
	// Other buttons do nothing.
	sf.ClickButton(circle, shell.ButtonSecondary)
	want(false, 2)
	// Disabled: no press, no change; disabling during a press drops it.
	r.SetEnabled(false)
	sf.Click(circle)
	want(false, 2)
	r.SetEnabled(true)
	sf.Press(circle, shell.ButtonPrimary)
	r.SetEnabled(false)
	sf.Release(circle, shell.ButtonPrimary)
	if r.view.press.Down() {
		t.Fatal("disabled during the press: still down")
	}
	want(false, 2)
	if r.view.Activate() {
		t.Fatal("Activate while disabled")
	}
	r.SetEnabled(true)
	// Activate (Space, the a11y press) turns it on too.
	if !r.view.Activate() {
		t.Fatal("Activate")
	}
	want(true, 3)
}

func TestRadioFocusRing(t *testing.T) {
	r, w, sf, s := radioWindow(t, "Wrap", nil)
	if !r.view.Focus() {
		t.Fatal("focus")
	}
	w.Show()
	s.FakeClock().Flush()
	got := controlLines(paint(t, s, sf))
	lines := strings.Split(got, "\n")
	ring := "strokeRRect (8.5,37.5,17,17) r=8.5 w=3 solid #007aff80"
	if !strings.HasPrefix(lines[len(lines)-1], ring) {
		t.Errorf("no ring around the circle, painted last:\n%s", got)
	}
	// Space turns the focused radio on; the arrow keys are consumed, and a
	// radio alone has nowhere to move (TestRadioGroupKeys).
	for _, k := range []driver.Key{driver.KeyDown, driver.KeyRight} {
		if !sf.Key(k, 0) || r.IsOn {
			t.Errorf("arrow key %v: not consumed, or turned the radio on", k)
		}
	}
	sf.Key(driver.KeySpace, 0)
	if !r.IsOn {
		t.Error("Space did not turn it on")
	}
	s.Deactivate()
	if got := paint(t, s, sf); strings.Contains(got, ring) {
		t.Errorf("ring in an inactive window:\n%s", got)
	}
}

func TestRadioInWindow(t *testing.T) {
	r, w, sf, s := radioWindow(t, "Wrap", nil)
	r.SetOn(true)
	tree := AccessibilityTree(w.Root())
	if n := tree.Find(a11y.RoleRadioButton, "Wrap"); n == nil || n.Frame != (driver.Rect{X: 10, Y: 38, W: 100, H: 16}) ||
		n.States != a11y.StateFocusable|a11y.StateChecked || n.Actions&a11y.ActionPress == 0 {
		t.Errorf("a11y tree:\n%s", tree.Dump())
	}
	// A fractional frame is pixel-aligned at scale 2.
	sf.SetScale(2)
	r.SetFrame(driver.Rect{X: 10.3, Y: 10.1, W: 100, H: 16})
	if got := paint(t, s, sf); !strings.Contains(got, "fillRRect (10.5,39,14,14) r=7") {
		t.Errorf("scale 2:\n%s", got)
	}
	r.SetHidden(true)
	if got := paint(t, s, sf); strings.Contains(got, `"Wrap"`) {
		t.Errorf("hidden radio painted:\n%s", got)
	}
	r.Destroy()
	if !r.Destroyed || ElementOf(r).base().Parent() != nil {
		t.Error("Destroy")
	}
}

// TestRadioStates pins every state (off, on, each plain and pressed;
// disabled off and on; focused on) in light and dark, in a key and an
// inactive window, with and without Increase Contrast (S6.1e), as record
// dumps in testdata/radio.states.
func TestRadioStates(t *testing.T) {
	var out strings.Builder
	for _, contrast := range []bool{false, true} {
		for _, dark := range []bool{false, true} {
			for _, key := range []bool{true, false} {
				for _, st := range []struct {
					name                       string
					on, pressed, disabled, foc bool
				}{
					{"off", false, false, false, false},
					{"off pressed", false, true, false, false},
					{"on", true, false, false, false},
					{"on pressed", true, true, false, false},
					{"disabled off", false, false, true, false},
					{"disabled on", true, false, true, false},
					{"focused on", true, false, false, true},
				} {
					r, w, sf, s := radioWindow(t, "Wrap", nil)
					w.SetAppearance(driver.Appearance{Dark: dark, Accent: driver.DefaultAccent, IncreaseContrast: contrast})
					r.SetOn(st.on)
					r.SetEnabled(!st.disabled)
					if st.foc && !r.view.Focus() {
						t.Fatal("focus")
					}
					if st.pressed {
						sf.Press(pt(17, 46), shell.ButtonPrimary)
					}
					if !key {
						s.Deactivate()
					}
					state := r.view.State()
					fmt.Fprintf(&out, "## %s, %s\n%s\n", st.name, state, controlLines(paint(t, s, sf)))
				}
			}
		}
	}
	got := out.String()
	path := filepath.Join("testdata", "radio.states")
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
		t.Errorf("radio states differ from %s (rerun with -update if the change is intended):\n%s", path, got)
	}
}

// groupHandler makes radio i of rs exclusive, as the public RadioGroup
// does, and logs the selections.
type groupHandler struct {
	rs  *[]*Radio
	i   int
	log *[]int
}

func (h groupHandler) StateChanged(s driver.CheckState) {
	if s != driver.StateOn {
		return
	}
	for j, r := range *h.rs {
		r.SetOn(j == h.i)
	}
	*h.log = append(*h.log, h.i)
}

// TestRadioGroupKeys (S5.7h): the radios of one container are one Tab
// stop, the selected one (the first usable one without a selection); the
// arrow keys move the focus and the selection to the next or previous
// usable radio, skipping a disabled one, without wrapping.
func TestRadioGroupKeys(t *testing.T) {
	s := startShell(t)
	d := newDriver(s)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 300, H: 200}, Style: allStyles})
	var rs []*Radio
	var log []int
	before := d.NewButton(nil).(*Button)
	before.SetTitle("Before")
	group := NewContainer()
	var peers []driver.Widget
	for i, title := range []string{"A", "B", "C", "D"} {
		r := d.NewRadio(groupHandler{&rs, i, &log}).(*Radio)
		r.SetTitle(title)
		r.SetFrame(driver.Rect{Y: float64(22 * i), W: 80, H: 16})
		rs = append(rs, r)
		peers = append(peers, r)
	}
	group.SetChildren(peers)
	group.SetFrame(driver.Rect{X: 10, Y: 40, W: 100, H: 100})
	after := d.NewButton(nil).(*Button)
	after.SetTitle("After")
	top := NewContainer()
	top.SetChildren([]driver.Widget{before, group, after})
	before.SetFrame(driver.Rect{X: 10, Y: 10, W: 80, H: 20})
	after.SetFrame(driver.Rect{X: 10, Y: 150, W: 80, H: 20})
	w.SetContent(top)
	w.Show()
	focused := func() Element { return w.Router().Focused() }
	order := func() []Element { return w.Router().FocusOrder() }

	// No selection: the first usable radio is the stop.
	if o := order(); len(o) != 3 || o[1] != ElementOf(rs[0]) {
		t.Fatalf("Tab order without a selection: %v", o)
	}
	rs[2].SetOn(true)
	if o := order(); len(o) != 3 || o[0] != ElementOf(before) || o[1] != ElementOf(rs[2]) || o[2] != ElementOf(after) {
		t.Fatalf("Tab order with C selected: %v", o)
	}
	w.Router().SetFocus(nil)
	sf.Key(driver.KeyTab, 0)
	sf.Key(driver.KeyTab, 0)
	if focused() != ElementOf(rs[2]) {
		t.Fatalf("Tab: focus %T", focused())
	}
	// Up to B, Left to A; at the top nothing moves but the key is consumed.
	for _, k := range []driver.Key{driver.KeyUp, driver.KeyLeft, driver.KeyUp} {
		if !sf.Key(k, 0) {
			t.Fatalf("arrow %v not consumed", k)
		}
	}
	if focused() != ElementOf(rs[0]) || !rs[0].IsOn || rs[2].IsOn || !slices.Equal(log, []int{1, 0}) {
		t.Fatalf("after Up, Left, Up: focus %T, log %v", focused(), log)
	}
	// Modifiers: not the group's.
	if sf.Key(driver.KeyDown, driver.ModShift) || focused() != ElementOf(rs[0]) {
		t.Fatal("Shift-Down moved")
	}
	// A disabled radio is skipped; at the bottom nothing moves.
	rs[1].SetEnabled(false)
	sf.Key(driver.KeyDown, 0)
	sf.Key(driver.KeyRight, 0)
	sf.Key(driver.KeyDown, 0)
	if focused() != ElementOf(rs[3]) || !rs[3].IsOn || !slices.Equal(log, []int{1, 0, 2, 3}) {
		t.Fatalf("after Down, Right, Down: focus %T, log %v", focused(), log)
	}
	// Tab leaves the group, Shift-Tab comes back to its selected radio.
	sf.Key(driver.KeyTab, 0)
	if focused() != ElementOf(after) {
		t.Fatalf("Tab out: focus %T", focused())
	}
	sf.Key(driver.KeyTab, driver.ModShift)
	if focused() != ElementOf(rs[3]) {
		t.Fatalf("Shift-Tab back: focus %T", focused())
	}
	// A disabled selected radio: the first usable one is the stop.
	rs[3].SetEnabled(false)
	if o := order(); len(o) != 3 || o[1] != ElementOf(rs[0]) {
		t.Fatalf("Tab order with the selected radio disabled: %v", o)
	}
}
