package aqua

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell/fake"
)

// The size and paint tests of the determinate Progress (PLAN.md S3.4e);
// the indeterminate loop is tested in progress_anim_test.go.

func newProg(t *testing.T) *Progress {
	t.Helper()
	return startDriver(t).NewProgress().(*Progress)
}

func TestProgressIsReal(t *testing.T) {
	p := newProg(t)
	if _, ok := Caption(p); ok {
		t.Error("the progress indicator is still a placeholder peer")
	}
	if e := ElementOf(p); e == nil || e != Element(p.view) || p.StubPeer() != p.Progress || p.Stub() != &p.Progress.Base {
		t.Error("ElementOf, StubPeer or Stub")
	}
	p.SetValue(40)
	if got := nodeOf(p); got != `progress "" value="40"` {
		t.Errorf("node %q", got)
	}
	p.SetIndeterminate(true)
	if got := nodeOf(p); got != `progress ""` {
		t.Errorf("indeterminate node %q", got)
	}
	p.SetStyle(driver.ProgressSpinner)
	p.SetIndeterminate(false)
	if got := nodeOf(p); got != `progress "" value="40"` {
		t.Errorf("spinner node %q", got)
	}
	if ElementOf(p).base().Focusable() {
		t.Error("focusable")
	}
}

func TestProgressFittingSize(t *testing.T) {
	p := newProg(t)
	for _, c := range []struct {
		cs      driver.ControlSize
		spinner bool
		want    driver.Size
	}{
		{driver.ControlSizeRegular, false, driver.Size{W: 96, H: 6}},
		{driver.ControlSizeSmall, false, driver.Size{W: 96, H: 4}},
		{driver.ControlSizeMini, false, driver.Size{W: 96, H: 3}},
		{driver.ControlSizeLarge, false, driver.Size{W: 96, H: 8}},
		{driver.ControlSizeRegular, true, driver.Size{W: 32, H: 32}},
		{driver.ControlSizeSmall, true, driver.Size{W: 16, H: 16}},
		{driver.ControlSizeMini, true, driver.Size{W: 10, H: 10}},
		{driver.ControlSizeLarge, true, driver.Size{W: 32, H: 32}},
	} {
		p.SetControlSize(c.cs)
		style := driver.ProgressBar
		if c.spinner {
			style = driver.ProgressSpinner
		}
		p.SetStyle(style)
		if got := p.FittingSize(infSize); got != c.want {
			t.Errorf("%v, spinner %v: %v, want %v", c.cs, c.spinner, got, c.want)
		}
	}
	p.SetControlSize(driver.ControlSizeRegular)
	if got := ElementOf(p).FittingSize(driver.Size{W: 20, H: 10}); got != (driver.Size{W: 20, H: 10}) {
		t.Errorf("clamped spinner %v", got)
	}
	p.SetStyle(driver.ProgressBar)
	if got := ElementOf(p).FittingSize(driver.Size{W: 50, H: 2}); got != (driver.Size{W: 50, H: 2}) {
		t.Errorf("clamped bar %v", got)
	}
	withControls(t, func(cc *theme.Controls) { cc.ProgressThickness = theme.SizeTable{5.5, 5.5, 5.5, 5.5} }, func() {
		if got := p.FittingSize(infSize); got != (driver.Size{W: 96, H: 6}) {
			t.Errorf("a fractional thickness %v", got)
		}
	})
	p.Fixed = driver.Size{W: 5, H: 7}
	if got := p.FittingSize(infSize); got != p.Fixed {
		t.Errorf("fixed %v", got)
	}
}

// The bar at (10,20) 120x6 in light with the default accent.
const progTrack = "fillRRect (10,20,120,6) r=3 solid #0000001a"

func TestProgressBarPaint(t *testing.T) {
	p := newProg(t)
	r := driver.Rect{X: 10, Y: 20, W: 120, H: 6}
	p.SetValue(0)
	checkDump(t, paintPeer(t, p, r), progTrack)
	p.SetValue(50)
	checkDump(t, paintPeer(t, p, r), progTrack, "fillRRect (10,20,60,6) r=3 solid #007aff")
	p.SetValue(100)
	checkDump(t, paintPeer(t, p, r), progTrack, "fillRRect (10,20,120,6) r=3 solid #007aff")
	// A short fill rounds its corners to half its width.
	p.SetValue(2)
	checkDump(t, paintPeer(t, p, r), progTrack, "fillRRect (10,20,2,6) r=1 solid #007aff")
	// An empty range fills nothing.
	p.SetRange(5, 5)
	checkDump(t, paintPeer(t, p, r), progTrack)
	p.SetRange(0, 100)
	p.SetValue(50)

	// Disabled: the track and the fill at ValueDisabledAlpha, no layer.
	p.SetEnabled(false)
	checkDump(t, paintPeer(t, p, r),
		"fillRRect (10,20,120,6) r=3 solid #0000000d",
		"fillRRect (10,20,60,6) r=3 solid #007aff80")
	p.SetEnabled(true)

	// A taller frame centres the track; an empty frame draws nothing.
	checkDump(t, paintPeer(t, p, driver.Rect{X: 10, Y: 20, W: 120, H: 22}),
		"fillRRect (10,28,120,6) r=3 solid #0000001a",
		"fillRRect (10,28,60,6) r=3 solid #007aff")
	checkDump(t, paintPeer(t, p, driver.Rect{W: 120}))

	// The control size picks the thickness and the radius.
	p.SetControlSize(driver.ControlSizeMini)
	checkDump(t, paintPeer(t, p, driver.Rect{W: 100, H: 3}),
		"fillRRect (0,0,100,3) r=1.5 solid #0000001a",
		"fillRRect (0,0,50,3) r=1.5 solid #007aff")
}

// progWindow opens a window (no timeline) with a progress indicator at
// value 50 of 0…100 framed at r of the content (surface y + 28).
func progWindow(t *testing.T, r driver.Rect) (*Progress, *Window, *fake.Surface, *fake.Shell) {
	t.Helper()
	s := startShell(t)
	d := New(s)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	p := d.NewProgress().(*Progress)
	p.SetValue(50)
	w.SetContent(p)
	p.SetFrame(r)
	return p, w, sf, s
}

func TestProgressBarInWindow(t *testing.T) {
	p, w, sf, s := progWindow(t, driver.Rect{X: 10, Y: 10, W: 120, H: 6})
	if got := paint(t, s, sf); !strings.Contains(got, "fillRRect (10,38,60,6) r=3 solid #007aff") {
		t.Errorf("key:\n%s", got)
	}
	// Inactive: the fill grey.
	s.Deactivate()
	if got := paint(t, s, sf); !strings.Contains(got, "fillRRect (10,38,60,6) r=3 solid #cbcbcb") {
		t.Errorf("inactive:\n%s", got)
	}
	sf.Activate()
	// Dark: the track light on dark.
	w.SetAppearance(driver.Appearance{Dark: true, Accent: driver.DefaultAccent})
	if got := paint(t, s, sf); !strings.Contains(got, "fillRRect (10,38,120,6) r=3 solid #ffffff1a") {
		t.Errorf("dark:\n%s", got)
	}
	// A fractional width is pixel-aligned at scale 2.
	sf.SetScale(2)
	p.SetValue(50.3)
	if got := paint(t, s, sf); !strings.Contains(got, "fillRRect (10,38,60.5,6) r=3 solid #007aff") {
		t.Errorf("scale 2:\n%s", got)
	}
	p.SetHidden(true)
	if got := paint(t, s, sf); strings.Contains(got, "solid #007aff") {
		t.Errorf("hidden bar painted:\n%s", got)
	}
	p.Destroy()
	if !p.Destroyed || ElementOf(p).base().Parent() != nil {
		t.Error("Destroy")
	}
}

// progSpokes returns the alpha byte of each spoke of a spinner dump, in
// spoke order (as hex strings), and fails when there are not 12.
func progSpokes(t *testing.T, dump string) []string {
	t.Helper()
	var out []string
	for l := range strings.SplitSeq(dump, "\n") {
		if !strings.HasPrefix(l, "strokePath") {
			continue
		}
		l, _, _ = strings.Cut(l, " clip=")
		i := strings.LastIndex(l, "#")
		out = append(out, l[i+7:])
	}
	if len(out) != 12 {
		t.Fatalf("%d spokes in\n%s", len(out), dump)
	}
	return out
}

func TestSpinnerPaint(t *testing.T) {
	p := newProg(t)
	p.SetStyle(driver.ProgressSpinner)
	r := driver.Rect{X: 10, Y: 20, W: 32, H: 32}
	lit, faint := "8c", "23" // 140 and 140 × 0.25
	count := func(v float64) (on, off int) {
		p.SetValue(v)
		for _, a := range progSpokes(t, paintPeer(t, p, r)) {
			switch a {
			case lit:
				on++
			case faint:
				off++
			default:
				t.Fatalf("value %v: spoke alpha %s", v, a)
			}
		}
		return on, off
	}
	for _, c := range []struct {
		v       float64
		on, off int
	}{{0, 0, 12}, {50, 6, 6}, {100, 12, 0}, {8.4, 1, 11}, {8.2, 0, 12}} {
		if on, off := count(c.v); on != c.on || off != c.off {
			t.Errorf("value %v: %d lit, %d faint; want %d, %d", c.v, on, off, c.on, c.off)
		}
	}

	// The first spoke at twelve o'clock, the next clockwise; the outer cap
	// touches the frame's edge.
	p.SetValue(50)
	got := paintPeer(t, p, r)
	checkDump(t, strings.Join(strings.Split(got, "\n")[:2], "\n"),
		"strokePath [M(26,27.75) L(26,21.25)] w=2.5 cap=round solid #0000008c",
		"strokePath [M(30.125,28.855) L(33.375,23.226)] w=2.5 cap=round solid #0000008c")
	if !strings.Contains(got, "strokePath [M(34.25,36) L(40.75,36)]") || !strings.Contains(got, "M(26,44.25) L(26,50.75)] w=2.5 cap=round solid #00000023") {
		t.Errorf("spokes:\n%s", got)
	}

	// An empty range: no spoke lit.
	p.SetRange(3, 3)
	for _, a := range progSpokes(t, paintPeer(t, p, r)) {
		if a != faint {
			t.Errorf("empty range: spoke %s", a)
		}
	}
	p.SetRange(0, 100)
	p.SetValue(50)

	// A bigger frame centres it; a smaller one scales it down.
	got = paintPeer(t, p, driver.Rect{W: 100, H: 40})
	if !strings.Contains(got, "strokePath [M(50,11.75) L(50,5.25)] w=2.5") {
		t.Errorf("centred:\n%s", got)
	}
	got = paintPeer(t, p, driver.Rect{W: 16, H: 16})
	if !strings.Contains(got, "strokePath [M(8,3.875) L(8,0.625)] w=1.25") {
		t.Errorf("scaled:\n%s", got)
	}
	checkDump(t, paintPeer(t, p, driver.Rect{W: 16}))

	// Small: its own size; disabled: dimmed.
	p.SetControlSize(driver.ControlSizeSmall)
	got = paintPeer(t, p, driver.Rect{W: 16, H: 16})
	if !strings.Contains(got, "strokePath [M(8,3.75) L(8,0.75)] w=1.5 cap=round solid #0000008c") {
		t.Errorf("small:\n%s", got)
	}
	p.SetEnabled(false)
	if got := paintPeer(t, p, driver.Rect{W: 16, H: 16}); !strings.Contains(got, "solid #00000046") || strings.Contains(got, "solid #0000008c") {
		t.Errorf("disabled:\n%s", got)
	}
}

func TestProgressRedraws(t *testing.T) {
	p := newProg(t)
	e := ElementOf(p).base()
	for _, c := range []struct {
		name string
		set  func()
	}{
		{"SetRange", func() { p.SetRange(0, 10) }},
		{"SetValue", func() { p.SetValue(3) }},
		{"SetStyle", func() { p.SetStyle(driver.ProgressSpinner) }},
		{"SetIndeterminate", func() { p.SetIndeterminate(true) }},
		{"SetControlSize", func() { p.SetControlSize(driver.ControlSizeSmall) }},
	} {
		e.dirty = false
		c.set()
		if !e.dirty {
			t.Errorf("%s did not invalidate", c.name)
		}
	}
}

// TestProgressStates pins the determinate bar and spinner at value 50,
// enabled and disabled, in light and dark, in a key and an inactive
// window, as record dumps in testdata/progress.states.
func TestProgressStates(t *testing.T) {
	var out strings.Builder
	for _, dark := range []bool{false, true} {
		for _, key := range []bool{true, false} {
			for _, st := range []struct {
				name     string
				spinner  bool
				disabled bool
			}{
				{"bar", false, false},
				{"bar disabled", false, true},
				{"spinner", true, false},
				{"spinner disabled", true, true},
			} {
				r := driver.Rect{X: 10, Y: 10, W: 120, H: 6}
				if st.spinner {
					r = driver.Rect{X: 10, Y: 10, W: 32, H: 32}
				}
				p, w, sf, s := progWindow(t, r)
				w.SetAppearance(driver.Appearance{Dark: dark, Accent: driver.DefaultAccent})
				if st.spinner {
					p.SetStyle(driver.ProgressSpinner)
				}
				p.SetEnabled(!st.disabled)
				if !key {
					s.Deactivate()
				}
				fmt.Fprintf(&out, "## %s, dark %v, key %v\n%s\n", st.name, dark, key, controlLines(paint(t, s, sf)))
			}
		}
	}
	got := out.String()
	path := filepath.Join("testdata", "progress.states")
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
		t.Errorf("progress states differ from %s (rerun with -update if the change is intended):\n%s", path, got)
	}
}
