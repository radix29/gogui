package aqua

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell/fake"
)

// The fake text system (shell/fake) lays out every rune 7 pt wide and every
// line 16 pt high at 13 pt, both scaling with the font size.

func newLabel(t *testing.T, text string) *Label {
	t.Helper()
	l := startDriver(t).NewLabel().(*Label)
	l.SetText(text)
	return l
}

func TestLabelIsReal(t *testing.T) {
	l := newLabel(t, "Hi")
	if _, ok := Caption(l); ok {
		t.Error("the label is still a placeholder peer")
	}
	if e := ElementOf(l); e == nil || e != Element(l.view) || l.AquaElement() != e {
		t.Error("ElementOf / AquaElement")
	}
	if l.StubPeer() != l.Label || l.Stub() != &l.Label.Base {
		t.Error("StubPeer or Stub")
	}
	if got := nodeOf(l); got != `text "Hi"` {
		t.Errorf("node %q", got)
	}
	if ElementOf(l).base().Focusable() {
		t.Error("a label is focusable")
	}
}

func TestLabelFont(t *testing.T) {
	l := newLabel(t, "x")
	sizes := map[driver.ControlSize]float64{
		driver.ControlSizeRegular: 13, driver.ControlSizeSmall: 11,
		driver.ControlSizeMini: 9, driver.ControlSizeLarge: 13,
	}
	for cs, want := range sizes {
		l.SetControlSize(cs)
		if got := l.TextFont(); got != (driver.Font{Size: want}) {
			t.Errorf("control size %d: font %+v, want %g pt", cs, got, want)
		}
		if got := tokens.Typography.FontSize.At(cs); got != (driver.Font{}).EffectiveSize(cs) {
			t.Errorf("control size %d: theme size %g", cs, got)
		}
	}
	f := driver.Font{Family: "Mono", Size: 20, Weight: driver.FontWeightBold}
	l.SetFont(f)
	if got := l.TextFont(); got != f {
		t.Errorf("explicit font %+v", got)
	}
}

func TestLabelFittingSize(t *testing.T) {
	l := newLabel(t, "Hello world")
	inf := driver.Size{W: math.Inf(1), H: math.Inf(1)}
	cases := []struct {
		name  string
		setup func()
		max   driver.Size
		want  driver.Size
	}{
		{"one line", func() {}, inf, driver.Size{W: 77, H: 16}},
		{"clamped", func() {}, driver.Size{W: 50, H: 10}, driver.Size{W: 50, H: 10}},
		{"line breaks", func() { l.SetText("ab\nabcd") }, inf, driver.Size{W: 28, H: 32}},
		{"empty", func() { l.SetText("") }, inf, driver.Size{H: 16}},
		{"small", func() { l.SetText("ab"); l.SetControlSize(driver.ControlSizeSmall) }, inf,
			driver.Size{W: 2 * 7 * 11.0 / 13, H: 16 * 11.0 / 13}},
		{"no wrap", func() { l.SetControlSize(driver.ControlSizeRegular); l.SetText("Hello world") },
			driver.Size{W: 50, H: 100}, driver.Size{W: 50, H: 16}},
		// Wrapping: real line widths ("Hello" is 35), not the stub's estimate.
		{"wraps", func() { l.SetWraps(true) }, driver.Size{W: 50, H: 100}, driver.Size{W: 35, H: 32}},
		{"wraps unbounded", func() {}, inf, driver.Size{W: 77, H: 16}},
		{"fixed", func() { l.Fixed = driver.Size{W: 5, H: 6} }, inf, driver.Size{W: 5, H: 6}},
	}
	for _, c := range cases {
		c.setup()
		if got := l.FittingSize(c.max); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	if got := ElementOf(l).FittingSize(inf); got != (driver.Size{W: 5, H: 6}) {
		t.Errorf("element FittingSize %v", got)
	}
}

func TestLabelLayoutCache(t *testing.T) {
	l := newLabel(t, "Hello world")
	l.SetWraps(true)
	a := l.view.layout(50)
	if l.view.layout(50) != a {
		t.Error("same key, new layout")
	}
	if l.view.layout(60) == a {
		t.Error("new width, same layout")
	}
	l.SetWraps(false)
	b := l.view.layout(60)
	if l.view.layout(40) != b {
		t.Error("a width without wrapping made a new layout")
	}
	l.SetText("other")
	if l.view.layout(40) == b {
		t.Error("new text, same layout")
	}
}

func TestLabelPaint(t *testing.T) {
	l := newLabel(t, "Hello")
	r := driver.Rect{X: 10, Y: 20, W: 100, H: 16}
	checkDump(t, paintPeer(t, l, r), `text (10,20,35,16) "Hello" 13pt #000000d9`)

	// Top of a taller frame; centre and trailing.
	tall := driver.Rect{X: 10, Y: 20, W: 100, H: 40}
	checkDump(t, paintPeer(t, l, tall), `text (10,20,35,16) "Hello" 13pt #000000d9`)
	l.SetAlignment(driver.AlignCenter)
	checkDump(t, paintPeer(t, l, tall), `text (42.5,20,35,16) "Hello" 13pt #000000d9`)
	l.SetAlignment(driver.AlignTrailing)
	checkDump(t, paintPeer(t, l, tall), `text (75,20,35,16) "Hello" 13pt #000000d9`)
	l.SetAlignment(driver.AlignJustified) // drawn leading
	checkDump(t, paintPeer(t, l, tall), `text (10,20,35,16) "Hello" 13pt #000000d9`)

	// Too wide: clipped to the frame; a centred overflow starts at the left.
	l.SetAlignment(driver.AlignCenter)
	checkDump(t, paintPeer(t, l, driver.Rect{X: 10, Y: 20, W: 20, H: 16}),
		`text (10,20,35,16) "Hello" 13pt #000000d9 clip=(10,20,20,16)`)
	// Too low.
	l.SetAlignment(driver.AlignLeading)
	checkDump(t, paintPeer(t, l, driver.Rect{W: 100, H: 10}),
		`text (0,0,35,16) "Hello" 13pt #000000d9 clip=(0,0,100,10)`)

	// Font and color.
	l.SetFont(driver.Font{Size: 26, Weight: driver.FontWeightBold})
	l.SetTextColor(driver.Semantic(driver.ColorSecondaryLabel))
	checkDump(t, paintPeer(t, l, driver.Rect{W: 100, H: 32}), `text (0,0,70,32) "Hello" 26pt w700 #00000080`)
	l.SetFont(driver.Font{})
	l.SetTextColor(driver.RGB8(0x12, 0x34, 0x56))
	checkDump(t, paintPeer(t, l, r), `text (10,20,35,16) "Hello" 13pt #123456`)

	// Disabled, also through an ancestor: the disabled control text color.
	l.SetEnabled(false)
	checkDump(t, paintPeer(t, l, r), `text (10,20,35,16) "Hello" 13pt #00000040`)
	l.SetEnabled(true)
	parent := newBox("parent", 0, 0, 200, 100)
	parent.AddChild(l.view)
	parent.SetEnabled(false)
	l.SetFrame(r)
	checkDump(t, dump(t, parent, driver.Size{W: 400, H: 300}),
		"fillRRect (0,0,200,100) solid #ff0000",
		`text (10,20,35,16) "Hello" 13pt #00000040`)

	// Nothing for an empty frame or an empty text.
	checkDump(t, paintPeer(t, l, driver.Rect{W: 100}))
	l.SetText("")
	checkDump(t, paintPeer(t, l, r))
}

func TestLabelPaintLines(t *testing.T) {
	l := newLabel(t, "ab\nabcd\n")
	r := driver.Rect{W: 100, H: 48}
	checkDump(t, paintPeer(t, l, r), `text (0,0,28,48) "ab\nabcd\n" 13pt #000000d9`)
	l.SetAlignment(driver.AlignTrailing)
	checkDump(t, paintPeer(t, l, r),
		`text (86,0,14,16) "ab" 13pt #000000d9`,
		`text (72,16,28,16) "abcd" 13pt #000000d9`)

	// Wrapped at the frame width, each line centred; empty lines skipped.
	l.SetText("Hello big world\n\nend")
	l.SetWraps(true)
	l.SetAlignment(driver.AlignCenter)
	checkDump(t, paintPeer(t, l, driver.Rect{W: 70, H: 100}),
		`text (3.5,0,63,16) "Hello big" 13pt #000000d9`,
		`text (17.5,16,35,16) "world" 13pt #000000d9`,
		`text (24.5,48,21,16) "end" 13pt #000000d9`)
}

func TestLabelLines(t *testing.T) {
	ts := fake.NewTextSystem()
	cases := []struct {
		text  string
		width float64
		want  string
	}{
		{"", 0, `"" 0`},
		{"one", 0, `"one" 0`},
		{"a\r\nb\n", 0, `"a" 0|"b" 16`},
		{"aa bb cc", 21, `"aa" 0|"bb" 16|"cc" 32`},
		{"äöü", 7, `"ä" 0|"ö" 16|"ü" 32`},
	}
	for _, c := range cases {
		var parts []string
		for _, ln := range lines(ts.NewLayout(c.text, driver.Font{}, c.width)) {
			parts = append(parts, fmtLine(ln))
		}
		if got := strings.Join(parts, "|"); got != c.want {
			t.Errorf("lines(%q, %g) = %s, want %s", c.text, c.width, got, c.want)
		}
	}
}

func fmtLine(l textLine) string { return strconv.Quote(l.text) + " " + fmtNumber(l.y) }

func TestLabelRedraws(t *testing.T) {
	l := newLabel(t, "x")
	b := ElementOf(l).base()
	for name, set := range map[string]func(){
		"SetText":        func() { l.SetText("y") },
		"SetFont":        func() { l.SetFont(driver.Font{Size: 20}) },
		"SetTextColor":   func() { l.SetTextColor(driver.Semantic(driver.ColorSecondaryLabel)) },
		"SetAlignment":   func() { l.SetAlignment(driver.AlignCenter) },
		"SetWraps":       func() { l.SetWraps(true) },
		"SetControlSize": func() { l.SetControlSize(driver.ControlSizeSmall) },
	} {
		b.dirty = false
		set()
		if !b.dirty {
			t.Errorf("%s did not invalidate", name)
		}
	}
	if l.Text != "y" || l.Font.Size != 20 || l.Alignment != driver.AlignCenter || !l.Wraps ||
		l.ControlSize != driver.ControlSizeSmall || l.TextColor != driver.Semantic(driver.ColorSecondaryLabel) {
		t.Errorf("stub state %+v", l.Label)
	}
}

func TestLabelInWindow(t *testing.T) {
	s := startShell(t)
	d := New(s)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	w.SetAppearance(driver.Appearance{Dark: true, Accent: driver.DefaultAccent})
	l := d.NewLabel().(*Label)
	l.SetText("Dark")
	w.SetContent(l)
	l.SetFrame(driver.Rect{X: 10, Y: 10, W: 100, H: 16})
	if got := paint(t, s, sf); !strings.Contains(got, `text (10,38,28,16) "Dark" 13pt #ffffffd9`) {
		t.Errorf("dark label:\n%s", got)
	}
	tree := AccessibilityTree(w.Root())
	if n := tree.Find(a11y.RoleStaticText, "Dark"); n == nil || n.Frame != (driver.Rect{X: 10, Y: 38, W: 100, H: 16}) {
		t.Errorf("a11y tree:\n%s", tree.Dump())
	}
	// Focus is counted by the stub but a label never takes it.
	l.Focus()
	if l.FocusCalls != 1 || ElementOf(l).base().HasFocus() {
		t.Error("Focus")
	}
	l.SetHidden(true)
	if !l.Hidden || !ElementOf(l).base().Hidden() {
		t.Error("SetHidden")
	}
	if got := paint(t, s, sf); strings.Contains(got, "Dark") {
		t.Errorf("hidden label painted:\n%s", got)
	}
	l.Destroy()
	if !l.Destroyed || ElementOf(l).base().Parent() != nil {
		t.Error("Destroy")
	}
}
