package aqua

import (
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/render/record"
	"github.com/radix29/gogui/internal/shell/fake"
)

// refRing returns the recorder line of the ring the test expects at
// progress p around the rounded rect r (uniform radius rad) for a, computed
// here from the S3.2d rules: inner edge (1-p)·w further out than the
// offset, width w·(2-p), alpha × p.
func refRing(t *testing.T, r driver.Rect, rad, p float64, a driver.Appearance) string {
	t.Helper()
	m := tokens.Metrics
	w := m.FocusRingWidth
	outset := m.FocusRingOffset + (1-p)*w
	width := w * (2 - p)
	d := outset + width/2
	col := a.Accent
	if !a.IncreaseContrast {
		col.A *= tokens.Accent.FocusRingAlpha
	}
	col.A *= float32(p)
	c := record.New(driver.Size{W: 1000, H: 1000})
	radii := render.Radii{}
	if rad > 0 {
		radii = render.UniformRadii(rad + d)
	}
	c.StrokeRoundedRect(driver.Rect{X: r.X - d, Y: r.Y - d, W: r.W + 2*d, H: r.H + 2*d}, radii, width, render.Solid(col))
	if err := c.End(); err != nil {
		t.Fatal(err)
	}
	return c.Dump()
}

// paintRing paints PaintFocusRing alone and returns the dump.
func paintRing(t *testing.T, shape FocusShape, p float64, a driver.Appearance) string {
	t.Helper()
	c := record.New(driver.Size{W: 1000, H: 1000})
	PaintFocusRing(c, shape, p, a)
	if err := c.End(); err != nil {
		t.Fatal(err)
	}
	return c.Dump()
}

func TestPaintFocusRing(t *testing.T) {
	light := driver.DefaultAppearance()
	dark := light
	dark.Dark = true
	contrast := light
	contrast.IncreaseContrast = true
	darkContrast := dark
	darkContrast.IncreaseContrast = true
	other := light
	other.Accent = driver.RGB8(200, 40, 90)
	r := driver.Rect{X: 10, Y: 20, W: 100, H: 22}

	// The full ring with the provisional tokens (width 3, offset 0, alpha
	// 0.5): centred 1.5 pt outside, radius 4 + 1.5.
	if got, want := paintRing(t, RectFocusShape(r, render.UniformRadii(4)), 1, light),
		"strokeRRect (8.5,18.5,103,25) r=5.5 w=3 solid #007aff80"; got != want {
		t.Errorf("full ring %q, want %q", got, want)
	}
	for _, a := range []driver.Appearance{light, dark, contrast, darkContrast, other} {
		for _, p := range []float64{1, 0.75, 0.5, 0.25, 0.01} {
			got := paintRing(t, RectFocusShape(r, render.UniformRadii(4)), p, a)
			if want := refRing(t, r, 4, p, a); got != want {
				t.Errorf("%+v p=%v: %q, want %q", a, p, got, want)
			}
		}
	}
	// Increased contrast: the opaque accent; dark mode keeps the accent.
	if got := paintRing(t, RectFocusShape(r, render.UniformRadii(4)), 1, contrast); !strings.HasSuffix(got, "solid #007aff") {
		t.Errorf("contrast ring %q", got)
	}
	if paintRing(t, RectFocusShape(r, render.UniformRadii(4)), 1, dark) != paintRing(t, RectFocusShape(r, render.UniformRadii(4)), 1, light) {
		t.Error("the ring differs between light and dark with the same accent")
	}
	// The start of the animation: one width further out, twice as wide.
	if out, w := FocusRingBand(0); out != 3 || w != 6 {
		t.Errorf("band at 0: %v %v", out, w)
	}
	if out, w := FocusRingBand(2); out != 0 || w != 3 {
		t.Errorf("band clamped: %v %v", out, w)
	}
	// Nothing at p ≤ 0 or NaN; p > 1 is the full ring.
	for _, p := range []float64{0, -1, nanF()} {
		if got := paintRing(t, RectFocusShape(r, render.UniformRadii(4)), p, light); got != "" {
			t.Errorf("p=%v drew %q", p, got)
		}
	}
	if paintRing(t, RectFocusShape(r, render.UniformRadii(4)), 3, light) != paintRing(t, RectFocusShape(r, render.UniformRadii(4)), 1, light) {
		t.Error("p > 1 is not the full ring")
	}
	// Mixed radii: sharp corners stay sharp.
	if got, want := paintRing(t, RectFocusShape(r, render.Radii{TopLeft: 4, BottomRight: 2}), 1, light),
		"strokeRRect (8.5,18.5,103,25) r=5.5/0/3.5/0 w=3 solid #007aff80"; got != want {
		t.Errorf("mixed radii %q, want %q", got, want)
	}
	// A path: scaled about its centre so its bounds grow by the outset.
	tri := (&render.Path{}).MoveTo(driver.Point{X: 10, Y: 10}).LineTo(driver.Point{X: 30, Y: 10}).
		QuadTo(driver.Point{X: 30, Y: 20}, driver.Point{X: 20, Y: 30}).
		CubicTo(driver.Point{X: 15, Y: 30}, driver.Point{X: 10, Y: 20}, driver.Point{X: 10, Y: 10}).Close()
	if got, want := paintRing(t, PathFocusShape(tri), 1, light),
		"strokePath [M(8.5,8.5) L(31.5,8.5) Q(31.5,20) (20,31.5) C(14.25,31.5) (8.5,20) (8.5,8.5) Z] w=3 solid #007aff80"; got != want {
		t.Errorf("path ring\n %q\nwant %q", got, want)
	}
	// A degenerate path (a line): the flat direction is not scaled.
	line := (&render.Path{}).MoveTo(driver.Point{X: 0, Y: 5}).LineTo(driver.Point{X: 10, Y: 5})
	if got, want := paintRing(t, PathFocusShape(line), 1, light),
		"strokePath [M(-1.5,5) L(11.5,5)] w=3 solid #007aff80"; got != want {
		t.Errorf("line ring %q, want %q", got, want)
	}
}

func nanF() float64 { var z float64; return z / z }

// ringOf returns the focus-ring line of a surface dump (the stroke in the
// accent's color), without its clip, or "".
func ringOf(dump string) string {
	for _, l := range strings.Split(dump, "\n") {
		if strings.HasPrefix(l, "strokeRRect") && strings.Contains(l, " solid #007aff") {
			l, _, _ = strings.Cut(l, " clip=")
			return l
		}
	}
	return ""
}

// TestFocusRingAnimation runs a text field's ring frame by frame on the
// driver's timeline: in on focus, at once out on blur and when the window
// becomes inactive, in again when it is key, not restarted by an
// appearance change, at once with Reduce Motion.
func TestFocusRingAnimation(t *testing.T) {
	sh := fake.New()
	// No caret blink: its repaints would be frames after the ring settled.
	noBlink := driver.DefaultAppearance()
	noBlink.CaretBlink = driver.CaretBlink{}
	sh.SetAppearance(noBlink)
	d := New(sh)
	h := &appLog{}
	h.onReady = func() {
		defer d.Quit()
		w, err := d.NewWindow(driver.WindowConfig{Title: "W", Size: driver.Size{W: 300, H: 100}, Style: allStyles}, &winLog{})
		if err != nil {
			t.Fatal(err)
		}
		aw := w.(*Window)
		sf := aw.Surface().(*fake.Surface)
		clk := sh.FakeClock()
		c := d.NewContainer()
		tf := d.NewTextField(false, nil).(*TextField)
		other := d.NewTextField(false, nil).(*TextField)
		c.SetFrame(driver.Rect{W: 300, H: 100})
		tf.SetFrame(driver.Rect{X: 10, Y: 10, W: 100, H: 22})
		other.SetFrame(driver.Rect{X: 10, Y: 50, W: 100, H: 22})
		c.SetChildren([]driver.Widget{tf, other})
		w.SetContent(c)
		w.Show()
		clk.Advance(time.Second)
		// Show focused tf (initial focus, S3.2e); start without a focus.
		if !tf.view.HasFocus() {
			t.Fatal("Show did not focus the first field")
		}
		aw.Router().SetFocus(nil)
		clk.Advance(time.Second)
		onSurface := driver.Rect{X: 10, Y: 38, W: 100, H: 22}

		// align moves Now to a frame time, so the frames after a start are at
		// whole multiples of FrameInterval from it.
		align := func() {
			if r := clk.Now() % fake.FrameInterval; r != 0 {
				clk.Advance(fake.FrameInterval - r)
			}
		}
		// Frames at FrameInterval (1/60 s rounded down to whole ns) from the
		// focus: the ring is exact at each of the first 12; 12 frames are 8 ns
		// short of the 200 ms, so the 13th ends it at the full ring; after its
		// one empty frame (the S3.2b finding) the clock stops.
		a := aw.Appearance()
		runIn := func(what string) {
			t.Helper()
			ease := anim.Curve(tokens.Animation.EaseOut)
			for k := 1; k <= 12; k++ {
				if n := clk.Advance(fake.FrameInterval); n != 1 {
					t.Fatalf("%s frame %d: %d frames ran", what, k, n)
				}
				p := ease.At(float64(time.Duration(k)*fake.FrameInterval) / float64(200*time.Millisecond))
				if got := tf.view.ring.Progress(); abs(got-p) > 1e-9 {
					t.Fatalf("%s frame %d: progress %v, want %v", what, k, got, p)
				}
				if got, want := ringOf(sf.Dump()), refRing(t, onSurface, 0, p, a); got != want {
					t.Fatalf("%s frame %d:\n got %q\nwant %q", what, k, got, want)
				}
			}
			clk.Advance(fake.FrameInterval)
			if got, want := ringOf(sf.Dump()), refRing(t, onSurface, 0, 1, a); got != want || tf.view.ring.Progress() != 1 {
				t.Fatalf("%s: last frame %q", what, got)
			}
			if clk.Advance(time.Second) > 1 || clk.Requested() {
				t.Fatalf("%s: frames after the ring settled", what)
			}
		}
		if ringOf(sf.Dump()) != "" {
			t.Fatal("a ring before the focus")
		}
		align()
		tf.Focus()
		if !tf.view.ring.Visible() || tf.view.ring.Progress() != 0 || !clk.Requested() {
			t.Fatal("focus did not start the ring")
		}
		runIn("focus")
		if got := ringOf(sf.Dump()); got != "strokeRRect (8.5,36.5,103,25) w=3 solid #007aff80" {
			t.Fatalf("settled ring %q", got)
		}

		// Blur (focus moves to the other field): gone at once, and the other
		// field's ring animates in.
		other.Focus()
		if tf.view.ring.Visible() || tf.view.ring.Progress() != 0 || !sf.Invalid() {
			t.Fatal("blur kept the ring")
		}
		clk.Advance(fake.FrameInterval)
		if strings.Contains(sf.Dump(), "(8.5,36.5,103,25)") || other.view.ring.Progress() <= 0 {
			t.Fatal("blur: old ring painted or new one not running")
		}
		clk.Advance(time.Second)

		// Back to tf, mid-flight the window becomes inactive: gone at once,
		// the focus kept; key again: it animates in from the start.
		tf.Focus()
		clk.Advance(3 * fake.FrameInterval)
		sh.Deactivate()
		if tf.view.ring.Visible() || tf.view.ring.Progress() != 0 || aw.Router().Focused() != ElementOf(tf) {
			t.Fatal("inactive window kept the ring or lost the focus")
		}
		clk.Advance(time.Second)
		if ringOf(sf.Dump()) != "" || clk.Requested() {
			t.Fatal("ring painted in an inactive window")
		}
		align()
		sf.Activate()
		runIn("key again")

		// An appearance change while shown repaints but does not restart.
		dark := aw.Appearance()
		dark.Dark = true
		sh.SetAppearance(dark)
		if tf.view.ring.Progress() != 1 || clk.Advance(time.Second) > 1 {
			t.Fatal("appearance change restarted the ring")
		}
		if got := ringOf(sf.Dump()); got != refRing(t, onSurface, 0, 1, dark) {
			t.Fatalf("dark ring %q", got)
		}

		// Increased contrast: the opaque accent.
		hc := dark
		hc.IncreaseContrast = true
		sh.SetAppearance(hc)
		clk.Advance(time.Second)
		if got := ringOf(sf.Dump()); got != "strokeRRect (8.5,36.5,103,25) w=3 solid #007aff" {
			t.Fatalf("contrast ring %q", got)
		}

		// Reduce Motion: the ring appears at once.
		rm := hc
		rm.ReduceMotion = true
		sh.SetAppearance(rm)
		other.Focus()
		tf.Focus()
		if tf.view.ring.Progress() != 1 || tf.view.ring.v.Running() {
			t.Fatalf("Reduce Motion: progress %v", tf.view.ring.Progress())
		}
		clk.Advance(time.Second)
		if got := ringOf(sf.Dump()); got != "strokeRRect (8.5,36.5,103,25) w=3 solid #007aff" {
			t.Fatalf("Reduce Motion ring %q", got)
		}

		// Hidden: the focus and the ring go.
		tf.SetHidden(true)
		if tf.view.ring.Visible() || tf.view.ring.Progress() != 0 {
			t.Fatal("hidden field kept its ring")
		}
		if err := sf.PaintErr(); err != nil {
			t.Fatal(err)
		}
	}
	if err := runDriver(t, d, h); err != nil {
		t.Fatal(err)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// TestFocusRingWithoutTimeline: a window made with NewWindow has no
// timeline, so the ring appears at once; focus before Show (an inactive
// window) shows it only when the window becomes key. Every focusable
// control draws it.
func TestFocusRingWithoutTimeline(t *testing.T) {
	s := startShell(t)
	d := newDriver(s)
	h := &winLog{}
	w, err := NewWindow(s, services(s), driver.WindowConfig{Title: "W", Size: driver.Size{W: 300, H: 200}, Style: allStyles}, h)
	if err != nil {
		t.Fatal(err)
	}
	sf := w.Surface().(*fake.Surface)
	tf := d.NewTextField(false, nil).(*TextField)
	tf.SetFrame(driver.Rect{X: 10, Y: 10, W: 100, H: 22})
	w.SetContent(tf)
	tf.Focus()
	if tf.view.ring.Visible() || tf.view.ring.Progress() != 0 {
		t.Fatal("a ring in a window that is not key")
	}
	w.Show()
	if tf.view.ring.Progress() != 1 {
		t.Fatalf("after Show: %v", tf.view.ring.Progress())
	}
	if got := ringOf(paint(t, s, sf)); got != "strokeRRect (8.5,36.5,103,25) w=3 solid #007aff80" {
		t.Fatalf("ring %q", got)
	}

	// The PopUpButton draws it too, around its bezel, last in its Paint
	// (the TextView: textview_test.go).
	for _, wd := range []driver.Widget{d.NewPopUpButton(nil)} {
		wd.SetFrame(driver.Rect{X: 20, Y: 20, W: 60, H: 6})
		w.SetContent(wd)
		e := ElementOf(wd)
		if !e.base().Focusable() || !e.base().Focus() {
			t.Fatalf("%T is not focusable", wd)
		}
		lines := strings.Split(paint(t, s, sf), "\n")
		want := "strokeRRect (18.5,46.5,63,9) r=4.5 w=3 solid #007aff80"
		found := -1
		for i, l := range lines {
			if strings.HasPrefix(l, want) {
				found = i
			}
		}
		// The ring follows the indicator (too low for its chevrons).
		if found < 0 || !strings.HasPrefix(lines[found-1], "fillRRect (62,50,16,2) r=1 solid #007aff") {
			t.Errorf("%T: ring missing or out of order:\n%s", wd, strings.Join(lines, "\n"))
		}
	}
	for _, wd := range []driver.Widget{d.NewLabel(), d.NewProgress(), d.NewSeparator(), d.NewBox(), d.NewScrollView(nil)} {
		if ElementOf(wd).base().Focusable() {
			t.Errorf("%T is focusable", wd)
		}
	}

	// An empty field draws no ring, even focused.
	tf.SetFrame(driver.Rect{X: 10, Y: 10})
	w.SetContent(tf)
	tf.Focus()
	if got := ringOf(paint(t, s, sf)); got != "" {
		t.Errorf("empty field ring %q", got)
	}
}

// TestFocusRingTrack: other events are ignored and not handled; outside a
// window the ring appears at once and paints with the default appearance.
func TestFocusRingTrack(t *testing.T) {
	var log []string
	root := newProbe(&log, "root", 0, 0, 100, 100)
	r := NewRouter(root)
	e := &ringProbe{}
	e.Init(e)
	e.SetFrame(driver.Rect{X: 10, Y: 10, W: 20, H: 10})
	e.SetFocusable(true)
	root.AddChild(e)
	for _, ev := range []Event{PointerEvent{}, KeyEvent{}, TextEvent{}, ScrollEvent{}} {
		if e.HandleEvent(ev) || e.ring.Visible() {
			t.Fatalf("%T changed the ring", ev)
		}
	}
	// FocusEvent{true} sent by hand to an element that is not focused does
	// nothing: the ring follows FocusState, not the event.
	e.HandleEvent(FocusEvent{Focused: true})
	if e.ring.Visible() {
		t.Fatal("a ring without the focus")
	}
	r.SetFocus(e)
	if e.ring.Progress() != 1 {
		t.Fatalf("plain router: %v", e.ring.Progress())
	}
	if got := dump(t, root, driver.Size{W: 100, H: 100}); got != "strokeRRect (8.5,8.5,23,13) r=3.5 w=3 solid #007aff80" {
		t.Errorf("plain router ring %q", got)
	}
	r.SetActive(false)
	e.HandleEvent(WindowStateEvent{})
	if e.ring.Visible() || dump(t, root, driver.Size{W: 100, H: 100}) != "" {
		t.Error("inactive plain router kept the ring")
	}
}

// ringProbe is a minimal element with a focus ring.
type ringProbe struct {
	Base
	ring FocusRing
}

func (p *ringProbe) HandleEvent(ev Event) bool { p.ring.Track(p, ev); return false }

func (p *ringProbe) Paint(c render.Canvas) {
	p.ring.Paint(c, p, RectFocusShape(p.Bounds(), render.UniformRadii(2)))
}
