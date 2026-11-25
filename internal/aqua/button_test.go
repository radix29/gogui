package aqua

import (
	"flag"
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

// updateGolden rewrites the golden dumps of this package (testdata):
//
//	CGO_ENABLED=0 go test -run TestButtonStates -update ./internal/aqua
var updateGolden = flag.Bool("update", false, "rewrite the golden dumps in testdata")

func newButton(t *testing.T, title string, clicks *int) *Button {
	t.Helper()
	var h driver.ButtonHandler
	if clicks != nil {
		h = clickCounter{clicks}
	}
	b := startDriver(t).NewButton(h).(*Button)
	b.SetTitle(title)
	return b
}

func TestButtonIsReal(t *testing.T) {
	b := newButton(t, "OK", nil)
	if e := ElementOf(b); e == nil || e != Element(b.view) || b.StubPeer() != b.Button || b.Stub() != &b.Button.Base {
		t.Error("ElementOf, StubPeer or Stub")
	}
	if got := nodeOf(b); got != `button "OK" {press}` {
		t.Errorf("node %q", got)
	}
	if e := ElementOf(b).base(); !e.Focusable() || e.FocusKind() != FocusControl {
		t.Error("not focusable as a control")
	}
}

func TestButtonFittingSize(t *testing.T) {
	b := newButton(t, "OK", nil)
	// The fake text system: 7 pt per rune at 13 pt, scaling with the size.
	for _, c := range []struct {
		cs    driver.ControlSize
		title string
		want  driver.Size
	}{
		{driver.ControlSizeRegular, "OK", driver.Size{W: 70, H: 20}},              // the minimum width
		{driver.ControlSizeRegular, "Create Account", driver.Size{W: 122, H: 20}}, // 98 + 2 × 12
		{driver.ControlSizeSmall, "Create Account", driver.Size{W: 103, H: 17}},   // 82.92 (11 pt) + 2 × 10, rounded up
		{driver.ControlSizeMini, "Create Account", driver.Size{W: 86, H: 14}},     // 70 + 2 × 8 (9 pt: 4.846 per rune, rounded up)
		{driver.ControlSizeLarge, "Create Account", driver.Size{W: 134, H: 26}},   // 15 pt
		{driver.ControlSizeRegular, "", driver.Size{W: 70, H: 20}},                // no title
		{driver.ControlSizeRegular, strings.Repeat("x", 20), driver.Size{W: 164, H: 20}},
	} {
		b.SetControlSize(c.cs)
		b.SetTitle(c.title)
		got := b.FittingSize(infSize)
		if c.cs == driver.ControlSizeMini || c.cs == driver.ControlSizeLarge {
			// Only the height is exact; the width is the title's rounded up.
			l := b.titleLayout()
			m := tokens.Metrics.Control(c.cs)
			w := max(tokens.Controls.ButtonMinWidth.At(c.cs), ceil(l.Size().W+2*m.PaddingX))
			c.want.W = w
		}
		if got != c.want {
			t.Errorf("%v %q: %v, want %v", c.cs, c.title, got, c.want)
		}
	}
	b.SetControlSize(driver.ControlSizeRegular)
	b.SetTitle("OK")
	if got := ElementOf(b).FittingSize(driver.Size{W: 50, H: 10}); got != (driver.Size{W: 50, H: 10}) {
		t.Errorf("clamped %v", got)
	}
	withControls(t, func(c *theme.Controls) { c.ButtonMinWidth = theme.SizeTable{} }, func() {
		if got := b.FittingSize(infSize); got != (driver.Size{W: 38, H: 20}) {
			t.Errorf("without a minimum width %v", got)
		}
	})
	b.Fixed = driver.Size{W: 5, H: 6}
	if got := b.FittingSize(infSize); got != b.Fixed {
		t.Errorf("fixed %v", got)
	}
}

func ceil(v float64) float64 {
	n := float64(int64(v))
	if n < v {
		n++
	}
	return n
}

func TestButtonTitleCache(t *testing.T) {
	b := newButton(t, "OK", nil)
	l := b.titleLayout()
	if l == nil || b.titleLayout() != l {
		t.Fatal("same title, new layout")
	}
	b.SetControlSize(driver.ControlSizeSmall)
	if b.titleLayout() == l {
		t.Error("new size, same layout")
	}
	b.SetTitle("")
	if b.titleLayout() != nil {
		t.Error("a layout for an empty title")
	}
	b.SetTitle("a\nb")
	if got := b.titleLayout().Text(); got != "a b" {
		t.Errorf("title on one line %q", got)
	}
}

// Colors of the light appearance with the default (blue) accent.
const (
	bezelShadow  = "shadow (10,20,100,20) r=5 off=(0,0.5) blur=1 #00000033"
	bezelFill    = "fillRRect (10,20,100,20) r=5 solid #ffffff"
	bezelBorder  = "strokeRRect (10.5,20.5,99,19) r=4.5 w=1 solid #00000026"
	okTitle      = `text (53,22,14,16) "OK" 13pt #000000d9`
	okTitleWhite = `text (53,22,14,16) "OK" 13pt #ffffff`
)

func TestButtonPaint(t *testing.T) {
	b := newButton(t, "OK", nil)
	r := driver.Rect{X: 10, Y: 20, W: 100, H: 20}
	checkDump(t, paintPeer(t, b, r), bezelShadow, bezelFill, bezelBorder, okTitle)

	// Default: the accent fill, no border, the accent text (outside a
	// window the tree counts as key).
	b.SetDefault(true)
	checkDump(t, paintPeer(t, b, r), bezelShadow,
		"fillRRect (10,20,100,20) r=5 solid #007aff", okTitleWhite)
	// Disabled: the disabled text; a disabled default is a plain button.
	b.SetEnabled(false)
	checkDump(t, paintPeer(t, b, r), bezelShadow, bezelFill, bezelBorder,
		`text (53,22,14,16) "OK" 13pt #00000040`)
	b.SetEnabled(true)
	b.SetDefault(false)

	// A taller frame: the bezel keeps its height, centred.
	checkDump(t, paintPeer(t, b, driver.Rect{X: 10, Y: 20, W: 100, H: 31}),
		"shadow (10,26,100,20) r=5 off=(0,0.5) blur=1 #00000033",
		"fillRRect (10,26,100,20) r=5 solid #ffffff",
		"strokeRRect (10.5,26.5,99,19) r=4.5 w=1 solid #00000026",
		`text (53,28,14,16) "OK" 13pt #000000d9`)
	// A narrow one truncates the title to the bezel minus the padding.
	b.SetTitle("Cancel")
	if got := paintPeer(t, b, driver.Rect{W: 45, H: 20}); !strings.Contains(got, `"Ca…"`) {
		t.Errorf("narrow button:\n%s", got)
	}
	// No room for the title (or the ellipsis), a bezel too thin for its
	// border, a low frame, an empty one.
	checkDump(t, paintPeer(t, b, driver.Rect{W: 24, H: 20}),
		"shadow (0,0,24,20) r=5 off=(0,0.5) blur=1 #00000033",
		"fillRRect (0,0,24,20) r=5 solid #ffffff",
		"strokeRRect (0.5,0.5,23,19) r=4.5 w=1 solid #00000026")
	checkDump(t, paintPeer(t, b, driver.Rect{W: 2, H: 20}),
		"shadow (0,0,2,20) r=1 off=(0,0.5) blur=1 #00000033",
		"fillRRect (0,0,2,20) r=1 solid #ffffff")
	checkDump(t, paintPeer(t, b, driver.Rect{W: 100, H: 10}),
		"shadow (0,0,100,10) r=5 off=(0,0.5) blur=1 #00000033",
		"fillRRect (0,0,100,10) r=5 solid #ffffff",
		"strokeRRect (0.5,0.5,99,9) r=4.5 w=1 solid #00000026")
	checkDump(t, paintPeer(t, b, driver.Rect{W: 100}))
	b.SetTitle("")
	checkDump(t, paintPeer(t, b, r), bezelShadow, bezelFill, bezelBorder)

	// The control size picks the bezel and the font.
	b.SetTitle("OK")
	b.SetControlSize(driver.ControlSizeMini)
	got := paintPeer(t, b, driver.Rect{W: 50, H: 14})
	if !strings.Contains(got, "fillRRect (0,0,50,14) r=3 solid #ffffff") || !strings.Contains(got, `"OK" 9pt`) {
		t.Errorf("mini button:\n%s", got)
	}

	// Disabled through an ancestor.
	b.SetControlSize(driver.ControlSizeRegular)
	parent := newBox("parent", 0, 0, 200, 100)
	parent.disabled = true
	treeChange() // set without SetEnabled
	parent.AddChild(b.view)
	if got := paintPeer(t, b, r); !strings.Contains(got, `"OK" 13pt #00000040`) {
		t.Errorf("disabled ancestor:\n%s", got)
	}
}

func TestButtonRedraws(t *testing.T) {
	b := newButton(t, "OK", nil)
	e := ElementOf(b).base()
	for name, set := range map[string]func(){
		"SetTitle":       func() { b.SetTitle("Go") },
		"SetDefault":     func() { b.SetDefault(true) },
		"SetControlSize": func() { b.SetControlSize(driver.ControlSizeSmall) },
	} {
		e.dirty = false
		set()
		if !e.dirty {
			t.Errorf("%s did not invalidate", name)
		}
	}
	b.SetCancel(true)
	if !b.IsCancel || !b.view.IsCancelButton() {
		t.Error("SetCancel")
	}
}

// buttonWindow opens a key window with b's button at frame r of the content.
func buttonWindow(t *testing.T, title string, clicks *int) (*Button, *Window, *fake.Surface, *fake.Shell) {
	t.Helper()
	s := startShell(t)
	d := newDriver(s)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	var h driver.ButtonHandler
	if clicks != nil {
		h = clickCounter{clicks}
	}
	b := d.NewButton(h).(*Button)
	b.SetTitle(title)
	w.SetContent(b)
	b.SetFrame(driver.Rect{X: 10, Y: 10, W: 100, H: 20})
	return b, w, sf, s
}

// bezelLines returns the button's lines of a window dump (clips cut off):
// the ones inside its frame at content (10,10) 100x20, title bar at 28.
func bezelLines(dump string) string {
	var out []string
	for _, l := range strings.Split(dump, "\n") {
		l, _, _ = strings.Cut(l, " clip=")
		if strings.Contains(l, "(10,38,") || strings.Contains(l, "(10.5,38.5,") ||
			strings.Contains(l, `"`) && !strings.HasPrefix(l, "text (") ||
			strings.HasPrefix(l, "text (") && strings.Contains(l, ",40,") ||
			strings.Contains(l, "(7.5,35.5,") || strings.Contains(l, "(8.5,36.5,") {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

func TestButtonClick(t *testing.T) {
	var clicks int
	b, _, sf, s := buttonWindow(t, "OK", &clicks)
	at := pt(60, 48) // the button's centre in surface coordinates
	off := pt(150, 80)

	sf.Click(at)
	if clicks != 1 {
		t.Fatalf("click: %d", clicks)
	}
	// Pressed while held over it, plain again when dragged off, pressed
	// when dragged back; a release outside does not click.
	sf.Press(at, shell.ButtonPrimary)
	if !b.view.press.Pressed() || b.view.State().Interaction != InteractionPressed {
		t.Fatal("not pressed")
	}
	if got := paint(t, s, sf); !strings.Contains(got, "fillRRect (10,38,100,20) r=5 solid #dedede") {
		t.Errorf("pressed paint:\n%s", got)
	}
	sf.Move(off)
	if b.view.press.Pressed() || !b.view.press.Down() {
		t.Fatal("dragged off: still pressed")
	}
	sf.Move(at)
	if !b.view.press.Pressed() {
		t.Fatal("dragged back: not pressed")
	}
	sf.Release(off, shell.ButtonPrimary)
	if clicks != 1 || b.view.press.Down() {
		t.Fatalf("release outside: %d clicks", clicks)
	}
	// Other buttons do nothing.
	sf.ClickButton(at, shell.ButtonSecondary)
	if clicks != 1 {
		t.Fatal("secondary click")
	}
	// Disabled: no press, no click; disabling during a press drops it.
	b.SetEnabled(false)
	sf.Click(at)
	if clicks != 1 || b.view.press.Down() {
		t.Fatal("disabled click")
	}
	b.SetEnabled(true)
	sf.Press(at, shell.ButtonPrimary)
	b.SetEnabled(false)
	sf.Release(at, shell.ButtonPrimary)
	if clicks != 1 || b.view.press.Down() {
		t.Fatal("disabled during the press")
	}
	b.SetEnabled(true)
	sf.Click(at)
	if clicks != 2 {
		t.Fatalf("after re-enabling: %d", clicks)
	}
	// Activate (Return, Escape, Space, a11y press) clicks too.
	if !b.view.Activate() || clicks != 3 {
		t.Fatal("Activate")
	}
}

func TestButtonFocusRing(t *testing.T) {
	b, w, sf, s := buttonWindow(t, "OK", nil)
	if !b.view.Focus() {
		t.Fatal("focus")
	}
	w.Show()
	s.FakeClock().Flush()
	got := paint(t, s, sf)
	lines := strings.Split(got, "\n")
	ring := "strokeRRect (8.5,36.5,103,23) r=6.5 w=3 solid #007aff80"
	last := ""
	for _, l := range lines {
		if strings.Contains(l, "(10,38,") || strings.Contains(l, `"OK"`) || strings.HasPrefix(l, ring) {
			last = l
		}
	}
	if !strings.HasPrefix(last, ring) {
		t.Errorf("no ring around the bezel, painted last:\n%s", got)
	}
	// In an inactive window the ring goes.
	s.Deactivate()
	if got := paint(t, s, sf); strings.Contains(got, ring) {
		t.Errorf("ring in an inactive window:\n%s", got)
	}
}

func TestButtonInWindow(t *testing.T) {
	b, w, sf, s := buttonWindow(t, "OK", nil)
	tree := AccessibilityTree(w.Root())
	if n := tree.Find(a11y.RoleButton, "OK"); n == nil || n.Frame != (driver.Rect{X: 10, Y: 38, W: 100, H: 20}) ||
		n.States != a11y.StateFocusable || n.Actions&a11y.ActionPress == 0 {
		t.Errorf("a11y tree:\n%s", tree.Dump())
	}
	// A fractional frame is pixel-aligned at scale 2.
	sf.SetScale(2)
	b.SetFrame(driver.Rect{X: 10.3, Y: 10.1, W: 100, H: 20})
	if got := paint(t, s, sf); !strings.Contains(got, "fillRRect (10.5,38,100,20) r=5") {
		t.Errorf("scale 2:\n%s", got)
	}
	b.SetHidden(true)
	if got := paint(t, s, sf); strings.Contains(got, `"OK"`) {
		t.Errorf("hidden button painted:\n%s", got)
	}
	b.Destroy()
	if !b.Destroyed || ElementOf(b).base().Parent() != nil {
		t.Error("Destroy")
	}
}

// TestButtonStates pins the bezel of every state (plain, pressed, default,
// default pressed, disabled, disabled default, focused, focused default)
// in light and dark, in a key and an inactive window, with and without
// Increase Contrast (S6.1e), as record dumps in testdata/button.states.
func TestButtonStates(t *testing.T) {
	var out strings.Builder
	for _, contrast := range []bool{false, true} {
		for _, dark := range []bool{false, true} {
			for _, key := range []bool{true, false} {
				for _, st := range []struct {
					name               string
					isDefault, pressed bool
					disabled, focused  bool
				}{
					{"plain", false, false, false, false},
					{"pressed", false, true, false, false},
					{"default", true, false, false, false},
					{"default pressed", true, true, false, false},
					{"disabled", false, false, true, false},
					{"disabled default", true, false, true, false},
					{"focused", false, false, false, true},
					{"focused default", true, false, false, true},
				} {
					b, w, sf, s := buttonWindow(t, "OK", nil)
					w.SetAppearance(driver.Appearance{Dark: dark, Accent: driver.DefaultAccent, IncreaseContrast: contrast})
					b.SetDefault(st.isDefault)
					b.SetEnabled(!st.disabled)
					if st.focused && !b.view.Focus() {
						t.Fatal("focus")
					}
					if st.pressed {
						sf.Press(pt(60, 48), shell.ButtonPrimary)
					}
					if !key {
						s.Deactivate()
					}
					state := b.view.State()
					fmt.Fprintf(&out, "## %s, %s\n%s\n", st.name, state, bezelLines(paint(t, s, sf)))
					if st.pressed {
						sf.Release(pt(60, 48), shell.ButtonPrimary)
					}
				}
			}
		}
	}
	got := out.String()
	path := filepath.Join("testdata", "button.states")
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
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
		t.Errorf("button states differ from %s (rerun with -update if the change is intended):\n%s", path, got)
	}
}
