package aqua

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// popUpLog records the selections a PopUpHandler gets.
type popUpLog struct{ picks *[]int }

func (l popUpLog) SelectionChanged(i int) { *l.picks = append(*l.picks, i) }

func newPopUp(t *testing.T, items ...string) *PopUpButton {
	t.Helper()
	p := startDriver(t).NewPopUpButton(nil).(*PopUpButton)
	p.SetItems(items)
	return p
}

func TestPopUpButtonIsReal(t *testing.T) {
	p := newPopUp(t)
	if e := ElementOf(p); e == nil || e != Element(p.view) || p.StubPeer() != p.PopUpButton || p.Stub() != &p.PopUpButton.Base {
		t.Error("ElementOf, StubPeer or Stub")
	}
	if e := ElementOf(p).base(); !e.Focusable() || e.FocusKind() != FocusControl {
		t.Error("not focusable as a control")
	}
	// The node is the placeholder's: the selected item as the value.
	for _, c := range []struct {
		set  func()
		want string
	}{
		{func() {}, `popup "" {showmenu}`},
		{func() { p.SetItems([]string{"Red", "Blue"}) }, `popup "" value="Red" {showmenu}`},
		{func() { p.SetSelected(1) }, `popup "" value="Blue" {showmenu}`},
		{func() { p.UserSelect(0) }, `popup "" value="Red" {showmenu}`},
		{func() { p.SetSelected(-1) }, `popup "" {showmenu}`},
	} {
		c.set()
		if got := nodeOf(p); got != c.want {
			t.Errorf("node %q, want %q", got, c.want)
		}
	}
}

// TestPopUpButtonFittingSize: the stub's fitting size (the widest item in
// the fake metrics plus 32, the fake control height), so the example
// frames stay where they were.
func TestPopUpButtonFittingSize(t *testing.T) {
	p := newPopUp(t, "Red", "Yellow")
	for _, cs := range []driver.ControlSize{driver.ControlSizeRegular, driver.ControlSizeSmall, driver.ControlSizeMini, driver.ControlSizeLarge} {
		p.SetControlSize(cs)
		want := stub.NewPopUpButton(nil)
		want.SetItems(p.Items)
		want.SetControlSize(cs)
		if got, w := ElementOf(p).FittingSize(infSize), want.FittingSize(infSize); got != w || p.FittingSize(infSize) != w {
			t.Errorf("%v: %v, want the stub's %v", cs, got, w)
		}
	}
	p.SetControlSize(driver.ControlSizeRegular)
	if got := p.FittingSize(infSize); got != (driver.Size{W: 74, H: 22}) { // 6 × 7 + 32
		t.Errorf("regular %v", got)
	}
	if got := ElementOf(p).FittingSize(driver.Size{W: 50, H: 10}); got != (driver.Size{W: 50, H: 10}) {
		t.Errorf("clamped %v", got)
	}
	p.Fixed = driver.Size{W: 5, H: 6}
	if got := p.FittingSize(infSize); got != p.Fixed {
		t.Errorf("fixed %v", got)
	}
}

func TestPopUpButtonTitleCache(t *testing.T) {
	p := newPopUp(t, "One", "Two")
	l := p.titleLayout()
	if l == nil || p.titleLayout() != l || l.Text() != "One" {
		t.Fatal("same title, new layout")
	}
	p.SetSelected(1)
	if l2 := p.titleLayout(); l2 == l || l2.Text() != "Two" {
		t.Error("new selection, same layout")
	}
	l = p.titleLayout()
	p.SetControlSize(driver.ControlSizeSmall)
	if p.titleLayout() == l || p.TitleFont() != (driver.Font{Size: 11}) {
		t.Error("new size, same layout")
	}
	p.SetSelected(-1)
	if p.titleLayout() != nil {
		t.Error("a layout without a selection")
	}
	p.SetItems([]string{"a\nb"})
	if got := p.titleLayout().Text(); got != "a b" {
		t.Errorf("title on one line %q", got)
	}
}

// The lines of a regular pop-up button "Red" at (10,20) 120x22 in light,
// key, with the default accent.
const (
	popUpShadow  = "shadow (10,21,120,20) r=5 off=(0,0.5) blur=1 #00000033"
	popUpFill    = "fillRRect (10,21,120,20) r=5 solid #ffffff"
	popUpBorder  = "strokeRRect (10.5,21.5,119,19) r=4.5 w=1 solid #00000026"
	popUpTitle   = `text (18,23,21,16) "Red" 13pt #000000d9`
	popUpInd     = "fillRRect (112,23,16,16) r=3 solid #007aff"
	popUpChevron = "strokePath [M(116.5,30) L(120,26.5) L(123.5,30) M(116.5,32) L(120,35.5) L(123.5,32)] w=1.5 cap=round join=round"
)

func TestPopUpButtonPaint(t *testing.T) {
	p := newPopUp(t, "Red", "Green")
	r := driver.Rect{X: 10, Y: 20, W: 120, H: 22}
	checkDump(t, paintPeer(t, p, r), popUpShadow, popUpFill, popUpBorder, popUpTitle, popUpInd,
		popUpChevron+" solid #ffffff")

	// Disabled: the disabled title and chevrons, no indicator fill; also
	// through a disabled ancestor.
	p.SetEnabled(false)
	checkDump(t, paintPeer(t, p, r), popUpShadow, popUpFill, popUpBorder,
		`text (18,23,21,16) "Red" 13pt #00000040`, popUpChevron+" solid #00000040")
	p.SetEnabled(true)
	parent := newBox("parent", 0, 0, 200, 100)
	parent.disabled = true
	treeChange() // set without SetEnabled
	parent.AddChild(p.view)
	if got := paintPeer(t, p, r); !strings.Contains(got, `"Red" 13pt #00000040`) || strings.Contains(got, "#007aff") {
		t.Errorf("disabled ancestor:\n%s", got)
	}
	p.view.RemoveFromParent()

	// No selection: the bezel and the indicator only.
	p.SetSelected(-1)
	checkDump(t, paintPeer(t, p, r), popUpShadow, popUpFill, popUpBorder, popUpInd, popUpChevron+" solid #ffffff")
	p.SetSelected(0)

	// A narrow frame truncates the title before the indicator: 60 − 8 −
	// (16 + 2) − 4 = 30 pt of room.
	p.SetItems([]string{"Magenta"})
	got := paintPeer(t, p, driver.Rect{W: 60, H: 22})
	if !strings.Contains(got, `"Mag…"`) || !strings.Contains(got, "fillRRect (42,3,16,16)") {
		t.Errorf("narrow pop-up:\n%s", got)
	}
	// Too narrow for the indicator: neither it nor a title (its room ends
	// where the indicator would start).
	checkDump(t, paintPeer(t, p, driver.Rect{W: 19, H: 22}),
		"shadow (0,1,19,20) r=5 off=(0,0.5) blur=1 #00000033",
		"fillRRect (0,1,19,20) r=5 solid #ffffff",
		"strokeRRect (0.5,1.5,18,19) r=4.5 w=1 solid #00000026")
	// Too low for the chevrons: the indicator alone; lower still: none;
	// empty: nothing.
	checkDump(t, paintPeer(t, p, driver.Rect{W: 100, H: 10}),
		"shadow (0,0,100,10) r=5 off=(0,0.5) blur=1 #00000033",
		"fillRRect (0,0,100,10) r=5 solid #ffffff",
		"strokeRRect (0.5,0.5,99,9) r=4.5 w=1 solid #00000026",
		"fillRRect (82,2,16,6) r=3 solid #007aff")
	checkDump(t, paintPeer(t, p, driver.Rect{W: 100, H: 4}),
		"shadow (0,0,100,4) r=2 off=(0,0.5) blur=1 #00000033",
		"fillRRect (0,0,100,4) r=2 solid #ffffff",
		"strokeRRect (0.5,0.5,99,3) r=1.5 w=1 solid #00000026")
	checkDump(t, paintPeer(t, p, driver.Rect{W: 100}))

	// The control size picks the bezel, the font, the padding, the
	// indicator and the chevrons.
	p.SetItems([]string{"Red"})
	p.SetControlSize(driver.ControlSizeMini)
	got = paintPeer(t, p, driver.Rect{W: 60, H: 16})
	for _, want := range []string{
		"fillRRect (0,1,60,14) r=3 solid #ffffff", `text (6,`, `"Red" 9pt`,
		"fillRRect (48,3,10,10) r=1 solid #007aff", "M(50.5,7) L(53,4.5) L(55.5,7) M(50.5,9) L(53,11.5) L(55.5,9)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("mini pop-up: no %q:\n%s", want, got)
		}
	}
}

func TestPopUpButtonRedraws(t *testing.T) {
	p := newPopUp(t, "A", "B")
	e := ElementOf(p).base()
	for name, set := range map[string]func(){
		"SetItems":       func() { p.SetItems([]string{"C"}) },
		"SetSelected":    func() { p.SetSelected(-1) },
		"SetControlSize": func() { p.SetControlSize(driver.ControlSizeSmall) },
		"UserSelect":     func() { p.UserSelect(0) },
	} {
		e.dirty = false
		set()
		if !e.dirty {
			t.Errorf("%s did not invalidate", name)
		}
	}
}

// popUpWindow opens a key window with a pop-up button of items at frame
// (10,10) 120x22 of the content (the surface's (10,38)).
func popUpWindow(t *testing.T, picks *[]int, items ...string) (*PopUpButton, *Window, *fake.Surface, *fake.Shell) {
	t.Helper()
	s := startShell(t)
	d := newDriver(s)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	var h driver.PopUpHandler
	if picks != nil {
		h = popUpLog{picks}
	}
	p := d.NewPopUpButton(h).(*PopUpButton)
	p.SetItems(items)
	w.SetContent(p)
	p.SetFrame(driver.Rect{X: 10, Y: 10, W: 120, H: 22})
	return p, w, sf, s
}

// contentLines returns the lines of a window dump painted by the content
// elements: after the content background, before the title bar, clips cut
// off.
func contentLines(dump string) string {
	lines := strings.Split(dump, "\n")
	var out []string
	for _, l := range lines[1:] {
		if strings.HasPrefix(l, "fillRRect (0,0,") {
			break
		}
		l, _, _ = strings.Cut(l, " clip=")
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// TestPopUpButtonPress: a primary press over it opens the menu (S5.5k),
// and it draws pressed while the menu is open; another button does
// nothing. Disabled, it takes no press.
func TestPopUpButtonPress(t *testing.T) {
	var picks []int
	p, _, sf, s := popUpWindow(t, &picks, "Red", "Green")
	at, off := pt(70, 49), pt(195, 95)
	sf.Press(at, shell.ButtonSecondary)
	sf.Release(at, shell.ButtonSecondary)
	if p.MenuOpen() || p.view.press.Down() {
		t.Fatal("secondary press")
	}
	sf.Press(at, shell.ButtonPrimary)
	if !p.MenuOpen() || p.view.State().Interaction != InteractionPressed {
		t.Fatal("not open and pressed")
	}
	got := paint(t, s, sf)
	if !strings.Contains(got, "fillRRect (10,39,120,20) r=5 solid #dedede") || !strings.Contains(got, "fillRRect (112,41,16,16) r=3 solid #0068d9") {
		t.Errorf("pressed paint:\n%s", got)
	}
	sf.Move(off)
	sf.Release(off, shell.ButtonPrimary)
	if p.MenuOpen() || p.view.State().Interaction == InteractionPressed || len(picks) != 0 || p.Index != 0 {
		t.Fatalf("released off: open %v, picks %v, index %d", p.MenuOpen(), picks, p.Index)
	}
	p.SetEnabled(false)
	sf.Press(at, shell.ButtonPrimary)
	if p.view.press.Down() || p.MenuOpen() {
		t.Fatal("disabled press")
	}
	sf.Release(at, shell.ButtonPrimary)
}

func TestPopUpButtonFocusRing(t *testing.T) {
	p, w, sf, s := popUpWindow(t, nil, "Red")
	if !p.view.Focus() {
		t.Fatal("focus")
	}
	w.Show()
	s.FakeClock().Flush()
	got := contentLines(paint(t, s, sf))
	ring := "strokeRRect (8.5,37.5,123,23) r=6.5 w=3 solid #007aff80"
	if lines := strings.Split(got, "\n"); lines[len(lines)-1] != ring {
		t.Errorf("no ring around the bezel, painted last:\n%s", got)
	}
	s.Deactivate()
	if got := paint(t, s, sf); strings.Contains(got, ring) {
		t.Errorf("ring in an inactive window:\n%s", got)
	}
}

func TestPopUpButtonInWindow(t *testing.T) {
	p, w, sf, s := popUpWindow(t, nil, "Red")
	w.SetAppearance(driver.Appearance{Dark: true, Accent: driver.DefaultAccent})
	tree := AccessibilityTree(w.Root())
	if n := tree.Find(a11y.RolePopUpButton, ""); n == nil || n.Frame != (driver.Rect{X: 10, Y: 38, W: 120, H: 22}) ||
		n.Value != "Red" || n.States != a11y.StateFocusable || n.Actions&a11y.ActionShowMenu == 0 {
		t.Errorf("a11y tree:\n%s", tree.Dump())
	}
	// Dark: the window's appearance, not the driver's.
	if got := paint(t, s, sf); !strings.Contains(got, `"Red" 13pt #ffffffd9`) {
		t.Errorf("dark pop-up:\n%s", got)
	}
	// Focus reaches the element and is counted; the tree shows it.
	p.Focus()
	if p.FocusCalls != 1 || !p.view.HasFocus() {
		t.Errorf("Focus: calls %d, focused %v", p.FocusCalls, p.view.HasFocus())
	}
	if n := AccessibilityTree(w.Root()).Find(a11y.RolePopUpButton, ""); n == nil || n.States&a11y.StateFocused == 0 {
		t.Error("not focused in the a11y tree")
	}
	// A fractional frame is pixel-aligned at scale 2.
	sf.SetScale(2)
	p.SetFrame(driver.Rect{X: 10.3, Y: 10.1, W: 120, H: 22})
	if got := paint(t, s, sf); !strings.Contains(got, "fillRRect (10.5,39,120,20) r=5") || !strings.Contains(got, "fillRRect (112.5,41,16,16) r=3") {
		t.Errorf("scale 2:\n%s", got)
	}
	// Hidden: neither side painted, the focus gone; Destroy leaves both
	// parents.
	p.SetHidden(true)
	if got := paint(t, s, sf); strings.Contains(got, `"Red"`) || !p.Hidden || p.view.HasFocus() {
		t.Errorf("hidden pop-up painted or focused:\n%s", got)
	}
	p.SetHidden(false)
	p.Destroy()
	if !p.Destroyed || p.view.Parent() != nil {
		t.Error("Destroy")
	}
}

// TestPopUpButtonStates pins every state (plain, pressed, disabled,
// focused, no selection, small) in light and dark, in a key and an
// inactive window, with and without Increase Contrast (S6.1e), as record
// dumps in testdata/popup.states.
func TestPopUpButtonStates(t *testing.T) {
	var out strings.Builder
	for _, contrast := range []bool{false, true} {
		for _, dark := range []bool{false, true} {
			for _, key := range []bool{true, false} {
				for _, st := range []struct {
					name              string
					pressed, disabled bool
					focused, untitled bool
					small             bool
				}{
					{name: "plain"},
					{name: "pressed", pressed: true},
					{name: "disabled", disabled: true},
					{name: "focused", focused: true},
					{name: "no selection", untitled: true},
					{name: "small", small: true},
				} {
					if st.pressed && !key {
						continue // the deactivation closes the menu: never pressed
					}
					p, w, sf, s := popUpWindow(t, nil, "Medium", "Large")
					w.SetAppearance(driver.Appearance{Dark: dark, Accent: driver.DefaultAccent, IncreaseContrast: contrast})
					p.SetEnabled(!st.disabled)
					if st.untitled {
						p.SetSelected(-1)
					}
					if st.small {
						p.SetControlSize(driver.ControlSizeSmall)
					}
					if st.focused && !p.view.Focus() {
						t.Fatal("focus")
					}
					if st.pressed {
						sf.Press(pt(70, 49), shell.ButtonPrimary)
					}
					if !key {
						s.Deactivate()
					}
					fmt.Fprintf(&out, "## %s, %s\n%s\n", st.name, p.view.State(), contentLines(paint(t, s, sf)))
					if st.pressed {
						sf.Release(pt(70, 49), shell.ButtonPrimary)
					}
				}
			}
		}
	}
	got := out.String()
	path := filepath.Join("testdata", "popup.states")
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
		t.Errorf("pop-up states differ from %s (rerun with -update if the change is intended):\n%s", path, got)
	}
}
