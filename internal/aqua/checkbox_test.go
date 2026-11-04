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

// toggleLog records the states a ToggleHandler is called with.
type toggleLog struct{ states *[]driver.CheckState }

func (l toggleLog) StateChanged(s driver.CheckState) { *l.states = append(*l.states, s) }

func newCheckbox(t *testing.T, title string) *Checkbox {
	t.Helper()
	c := startDriver(t).NewCheckbox(nil).(*Checkbox)
	c.SetTitle(title)
	return c
}

func TestCheckboxIsReal(t *testing.T) {
	c := newCheckbox(t, "Wrap")
	if _, ok := Caption(c); ok {
		t.Error("the checkbox is still a placeholder peer")
	}
	if e := ElementOf(c); e == nil || e != Element(c.view) || c.StubPeer() != c.Checkbox || c.Stub() != &c.Checkbox.Base {
		t.Error("ElementOf, StubPeer or Stub")
	}
	for _, s := range []struct {
		state driver.CheckState
		node  string
	}{
		{driver.StateOff, `checkbox "Wrap" {press}`},
		{driver.StateOn, `checkbox "Wrap" [checked] {press}`},
		{driver.StateMixed, `checkbox "Wrap" [mixed] {press}`},
	} {
		c.SetState(s.state)
		if got := nodeOf(c); got != s.node {
			t.Errorf("node %q, want %q", got, s.node)
		}
	}
	if e := ElementOf(c).base(); !e.Focusable() || e.FocusKind() != FocusControl {
		t.Error("not focusable as a control")
	}
}

func TestCheckboxFittingSize(t *testing.T) {
	c := newCheckbox(t, "Wrap")
	// The fake text system: 7 pt per rune and 16 pt lines at 13 pt,
	// scaling with the size.
	for _, s := range []struct {
		cs    driver.ControlSize
		title string
		want  driver.Size
	}{
		{driver.ControlSizeRegular, "Wrap", driver.Size{W: 48, H: 16}}, // 14 + 6 + 28; the line is higher than the box
		{driver.ControlSizeRegular, "", driver.Size{W: 14, H: 14}},     // the box alone
		{driver.ControlSizeSmall, "Wrap", driver.Size{W: 41, H: 14}},   // 12 + 5 + 23.69, rounded up; 13.54 high, rounded up
		{driver.ControlSizeMini, "Wrap", driver.Size{W: 34, H: 12}},    // 10 + 4 + 19.38; 11.08 high
		{driver.ControlSizeLarge, "Wrap", driver.Size{W: 51, H: 16}},   // 16 + 7 + 28 (13 pt)
		{driver.ControlSizeRegular, "Send", driver.Size{W: 48, H: 16}}, // the same width per rune
		{driver.ControlSizeRegular, "a\nb", driver.Size{W: 41, H: 16}}, // on one line: "a b"
		{driver.ControlSizeRegular, "Option 1", driver.Size{W: 76, H: 16}},
	} {
		c.SetControlSize(s.cs)
		c.SetTitle(s.title)
		if got := c.FittingSize(infSize); got != s.want {
			t.Errorf("%v %q: %v, want %v", s.cs, s.title, got, s.want)
		}
	}
	c.SetControlSize(driver.ControlSizeRegular)
	c.SetTitle("Wrap")
	if got := ElementOf(c).FittingSize(driver.Size{W: 30, H: 10}); got != (driver.Size{W: 30, H: 10}) {
		t.Errorf("clamped %v", got)
	}
	// A box higher than the title's line sets the height.
	withControls(t, func(cc *theme.Controls) { cc.CheckboxSize = theme.SizeTable{20, 20, 20, 20} }, func() {
		if got := c.FittingSize(infSize); got != (driver.Size{W: 54, H: 20}) {
			t.Errorf("a large box %v", got)
		}
	})
	c.Fixed = driver.Size{W: 5, H: 6}
	if got := c.FittingSize(infSize); got != c.Fixed {
		t.Errorf("fixed %v", got)
	}
}

func TestCheckboxTitleCache(t *testing.T) {
	c := newCheckbox(t, "Wrap")
	l := c.titleLayout()
	if l == nil || c.titleLayout() != l {
		t.Fatal("same title, new layout")
	}
	c.SetControlSize(driver.ControlSizeSmall)
	if c.titleLayout() == l {
		t.Error("new size, same layout")
	}
	c.SetTitle("")
	if c.titleLayout() != nil {
		t.Error("a layout for an empty title")
	}
}

// The checkbox at (10,20) 100x16 in light with the default accent: the
// box at (10,21) 14x14, the title at x 30.
const (
	boxOff    = "fillRRect (10,21,14,14) r=3.5 solid #ffffff"
	boxBorder = "strokeRRect (10.5,21.5,13,13) r=3 w=1 solid #00000026"
	boxOn     = "fillRRect (10,21,14,14) r=3.5 solid #007aff"
	boxCheck  = "strokePath [M(13.5,28.5) L(16,31) L(20.5,25)] w=1.75 cap=round join=round solid #ffffff"
	boxDash   = "strokePath [M(13.5,28) L(20.5,28)] w=1.75 cap=round join=round solid #ffffff"
	wrapTitle = `text (30,20,28,16) "Wrap" 13pt #000000d9`
)

func TestCheckboxPaint(t *testing.T) {
	c := newCheckbox(t, "Wrap")
	r := driver.Rect{X: 10, Y: 20, W: 100, H: 16}
	checkDump(t, paintPeer(t, c, r), boxOff, boxBorder, wrapTitle)
	c.SetState(driver.StateOn)
	checkDump(t, paintPeer(t, c, r), boxOn, boxCheck, wrapTitle)
	c.SetState(driver.StateMixed)
	checkDump(t, paintPeer(t, c, r), boxOn, boxDash, wrapTitle)

	// Disabled: the box in a layer at ToggleDisabledAlpha, the disabled
	// text.
	c.SetState(driver.StateOn)
	c.SetEnabled(false)
	checkDump(t, paintPeer(t, c, r), "pushOpacity 0.5", boxOn+" alpha=0.5", boxCheck+" alpha=0.5", "pop",
		`text (30,20,28,16) "Wrap" 13pt #00000040`)
	c.SetState(driver.StateOff)
	checkDump(t, paintPeer(t, c, r), "pushOpacity 0.5", boxOff+" alpha=0.5", boxBorder+" alpha=0.5", "pop",
		`text (30,20,28,16) "Wrap" 13pt #00000040`)
	c.SetEnabled(true)

	// A taller frame: the box and the title centred vertically.
	checkDump(t, paintPeer(t, c, driver.Rect{X: 10, Y: 20, W: 100, H: 30}),
		"fillRRect (10,28,14,14) r=3.5 solid #ffffff",
		"strokeRRect (10.5,28.5,13,13) r=3 w=1 solid #00000026",
		`text (30,27,28,16) "Wrap" 13pt #000000d9`)
	// A narrow one truncates the title to the frame.
	c.SetTitle("Send me updates")
	if got := paintPeer(t, c, driver.Rect{W: 50, H: 16}); !strings.Contains(got, `"Sen…"`) {
		t.Errorf("narrow checkbox:\n%s", got)
	}
	// No room for the title (or the ellipsis), a frame lower than the
	// title's line, a box smaller than the token, too small for its
	// border, an empty frame.
	checkDump(t, paintPeer(t, c, driver.Rect{W: 24, H: 16}),
		"fillRRect (0,1,14,14) r=3.5 solid #ffffff",
		"strokeRRect (0.5,1.5,13,13) r=3 w=1 solid #00000026")
	checkDump(t, paintPeer(t, c, driver.Rect{W: 100, H: 14}),
		"fillRRect (0,0,14,14) r=3.5 solid #ffffff",
		"strokeRRect (0.5,0.5,13,13) r=3 w=1 solid #00000026")
	checkDump(t, paintPeer(t, c, driver.Rect{W: 100, H: 6}),
		"fillRRect (0,0,6,6) r=3 solid #ffffff",
		"strokeRRect (0.5,0.5,5,5) r=2.5 w=1 solid #00000026")
	checkDump(t, paintPeer(t, c, driver.Rect{W: 100, H: 2}),
		"fillRRect (0,0,2,2) r=1 solid #ffffff")
	checkDump(t, paintPeer(t, c, driver.Rect{W: 100}))
	// The mark scales with the box.
	c.SetState(driver.StateOn)
	checkDump(t, paintPeer(t, c, driver.Rect{W: 100, H: 7}),
		"fillRRect (0,0,7,7) r=3.5 solid #007aff",
		"strokePath [M(1.75,3.75) L(3,5) L(5.25,2)] w=1.75 cap=round join=round solid #ffffff")
	c.SetState(driver.StateOff)
	c.SetTitle("")
	checkDump(t, paintPeer(t, c, r), boxOff, boxBorder)

	// The control size picks the box and the font.
	c.SetTitle("Wrap")
	c.SetControlSize(driver.ControlSizeMini)
	got := paintPeer(t, c, driver.Rect{W: 50, H: 12})
	if !strings.Contains(got, "fillRRect (0,1,10,10) r=2.5 solid #ffffff") || !strings.Contains(got, `text (14,0.462,`) ||
		!strings.Contains(got, `"Wrap" 9pt`) {
		t.Errorf("mini checkbox:\n%s", got)
	}

	// Disabled through an ancestor.
	c.SetControlSize(driver.ControlSizeRegular)
	parent := newBox("parent", 0, 0, 200, 100)
	parent.disabled = true
	parent.AddChild(c.view)
	if got := paintPeer(t, c, r); !strings.Contains(got, "pushOpacity 0.5") || !strings.Contains(got, `"Wrap" 13pt #00000040`) {
		t.Errorf("disabled ancestor:\n%s", got)
	}
}

func TestCheckboxRedraws(t *testing.T) {
	c := newCheckbox(t, "Wrap")
	e := ElementOf(c).base()
	for name, set := range map[string]func(){
		"SetTitle":       func() { c.SetTitle("Go") },
		"SetState":       func() { c.SetState(driver.StateMixed) },
		"SetControlSize": func() { c.SetControlSize(driver.ControlSizeSmall) },
		"UserClick":      func() { c.UserClick() },
	} {
		e.dirty = false
		set()
		if !e.dirty {
			t.Errorf("%s did not invalidate", name)
		}
	}
}

// checkboxWindow opens a key window with a checkbox at frame (10,10)
// 100x16 of the content (surface y 38, under the 28 pt title bar).
func checkboxWindow(t *testing.T, title string, states *[]driver.CheckState) (*Checkbox, *Window, *fake.Surface, *fake.Shell) {
	t.Helper()
	s := startShell(t)
	d := New(s)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	var h driver.ToggleHandler
	if states != nil {
		h = toggleLog{states}
	}
	c := d.NewCheckbox(h).(*Checkbox)
	c.SetTitle(title)
	w.SetContent(c)
	c.SetFrame(driver.Rect{X: 10, Y: 10, W: 100, H: 16})
	return c, w, sf, s
}

// controlLines returns the lines of a window dump between the content
// background and the title bar, clips cut off: what the content draws.
func controlLines(dump string) string {
	lines := strings.Split(dump, "\n")
	var out []string
	for _, l := range lines[1:] {
		if strings.HasPrefix(l, "fillRRect (0,0,200,28)") {
			break
		}
		l, _, _ = strings.Cut(l, " clip=")
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func TestCheckboxClick(t *testing.T) {
	var states []driver.CheckState
	c, _, sf, s := checkboxWindow(t, "Wrap", &states)
	box, title := pt(17, 46), pt(40, 46) // in surface coordinates
	off := pt(150, 80)
	want := func(v driver.CheckState, n int) {
		t.Helper()
		if c.Value != v || len(states) != n {
			t.Fatalf("state %v after %d calls (%v), want %v after %d", c.Value, len(states), states, v, n)
		}
	}

	sf.Click(box)
	want(driver.StateOn, 1)
	sf.Click(title)
	want(driver.StateOff, 2)
	// Mixed turns on.
	c.SetState(driver.StateMixed)
	sf.Click(box)
	want(driver.StateOn, 3)
	if states[2] != driver.StateOn {
		t.Fatalf("handler states %v", states)
	}

	// Pressed while held over it, dragged off and back; a release outside
	// does not toggle.
	sf.Press(title, shell.ButtonPrimary)
	if c.view.State().Interaction != InteractionPressed {
		t.Fatal("not pressed")
	}
	if got := paint(t, s, sf); !strings.Contains(got, "fillRRect (10,39,14,14) r=3.5 solid #0068d9") {
		t.Errorf("pressed paint:\n%s", got)
	}
	sf.Move(off)
	if c.view.press.Pressed() || !c.view.press.Down() {
		t.Fatal("dragged off: still pressed")
	}
	sf.Move(box)
	if !c.view.press.Pressed() {
		t.Fatal("dragged back: not pressed")
	}
	sf.Release(off, shell.ButtonPrimary)
	want(driver.StateOn, 3)
	// Other buttons do nothing.
	sf.ClickButton(box, shell.ButtonSecondary)
	want(driver.StateOn, 3)
	// Disabled: no press, no toggle; disabling during a press drops it.
	c.SetEnabled(false)
	sf.Click(box)
	want(driver.StateOn, 3)
	c.SetEnabled(true)
	sf.Press(box, shell.ButtonPrimary)
	c.SetEnabled(false)
	sf.Release(box, shell.ButtonPrimary)
	if c.view.press.Down() {
		t.Fatal("disabled during the press: still down")
	}
	want(driver.StateOn, 3)
	if c.view.Activate() {
		t.Fatal("Activate while disabled")
	}
	c.SetEnabled(true)
	sf.Click(box)
	want(driver.StateOff, 4)
	// Activate (Space, the a11y press) toggles too.
	if !c.view.Activate() {
		t.Fatal("Activate")
	}
	want(driver.StateOn, 5)
}

func TestCheckboxFocusRing(t *testing.T) {
	c, w, sf, s := checkboxWindow(t, "Wrap", nil)
	if !c.view.Focus() {
		t.Fatal("focus")
	}
	w.Show()
	s.FakeClock().Flush()
	got := controlLines(paint(t, s, sf))
	lines := strings.Split(got, "\n")
	ring := "strokeRRect (8.5,37.5,17,17) r=5 w=3 solid #007aff80"
	if !strings.HasPrefix(lines[len(lines)-1], ring) {
		t.Errorf("no ring around the box, painted last:\n%s", got)
	}
	// Space toggles the focused checkbox.
	sf.Key(driver.KeySpace, 0)
	if c.Value != driver.StateOn {
		t.Error("Space did not toggle")
	}
	s.Deactivate()
	if got := paint(t, s, sf); strings.Contains(got, ring) {
		t.Errorf("ring in an inactive window:\n%s", got)
	}
}

func TestCheckboxInWindow(t *testing.T) {
	c, w, sf, s := checkboxWindow(t, "Wrap", nil)
	c.SetState(driver.StateMixed)
	tree := AccessibilityTree(w.Root())
	if n := tree.Find(a11y.RoleCheckBox, "Wrap"); n == nil || n.Frame != (driver.Rect{X: 10, Y: 38, W: 100, H: 16}) ||
		n.States != a11y.StateFocusable|a11y.StateMixed || n.Actions&a11y.ActionPress == 0 {
		t.Errorf("a11y tree:\n%s", tree.Dump())
	}
	// A fractional frame is pixel-aligned at scale 2.
	sf.SetScale(2)
	c.SetFrame(driver.Rect{X: 10.3, Y: 10.1, W: 100, H: 16})
	if got := paint(t, s, sf); !strings.Contains(got, "fillRRect (10.5,39,14,14) r=3.5") {
		t.Errorf("scale 2:\n%s", got)
	}
	c.SetHidden(true)
	if got := paint(t, s, sf); strings.Contains(got, `"Wrap"`) {
		t.Errorf("hidden checkbox painted:\n%s", got)
	}
	c.Destroy()
	if !c.Destroyed || ElementOf(c).base().Parent() != nil {
		t.Error("Destroy")
	}
}

// TestCheckboxStates pins every state (off, on, mixed, each plain and
// pressed; disabled off and on; focused on) in light and dark, in a key
// and an inactive window, as record dumps in testdata/checkbox.states.
func TestCheckboxStates(t *testing.T) {
	var out strings.Builder
	for _, dark := range []bool{false, true} {
		for _, key := range []bool{true, false} {
			for _, st := range []struct {
				name              string
				value             driver.CheckState
				pressed           bool
				disabled, focused bool
			}{
				{"off", driver.StateOff, false, false, false},
				{"off pressed", driver.StateOff, true, false, false},
				{"on", driver.StateOn, false, false, false},
				{"on pressed", driver.StateOn, true, false, false},
				{"mixed", driver.StateMixed, false, false, false},
				{"mixed pressed", driver.StateMixed, true, false, false},
				{"disabled off", driver.StateOff, false, true, false},
				{"disabled on", driver.StateOn, false, true, false},
				{"focused on", driver.StateOn, false, false, true},
			} {
				c, w, sf, s := checkboxWindow(t, "Wrap", nil)
				w.SetAppearance(driver.Appearance{Dark: dark, Accent: driver.DefaultAccent})
				c.SetState(st.value)
				c.SetEnabled(!st.disabled)
				if st.focused && !c.view.Focus() {
					t.Fatal("focus")
				}
				if st.pressed {
					sf.Press(pt(17, 46), shell.ButtonPrimary)
				}
				if !key {
					s.Deactivate()
				}
				state := c.view.State()
				fmt.Fprintf(&out, "## %s, %s\n%s\n", st.name, state, controlLines(paint(t, s, sf)))
			}
		}
	}
	got := out.String()
	path := filepath.Join("testdata", "checkbox.states")
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
		t.Errorf("checkbox states differ from %s (rerun with -update if the change is intended):\n%s", path, got)
	}
}
