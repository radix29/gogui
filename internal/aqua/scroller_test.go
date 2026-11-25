package aqua

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// The paint and fade tests of the overlay scrollers (PLAN.md S3.5e),
// against the spec in scroller.go's doc comment, on the scrollEnv harness
// (scrollenv_test.go): a 150 × 100 scroll view at content (10, 10), i.e.
// surface (10, 38), with a vertical scroller and a 150 × 400 document.
//
// With the provisional tokens (ScrollerThickness 6, ScrollerHoverThickness
// 9, ScrollerInset 2, ScrollerMinKnobLength 18) the vertical strip is
// 9 + 2·2 = 13 wide at the viewport's right edge: (137, 0, 13, 100) in the
// scroll view, surface x 147…160. The knob is 6 thick, ScrollerInset in
// from the right: across 13 − 2 − 6 = 5, surface x 152. The track runs
// from 2 to 98 (96 long); the knob is 96 · 100/400 = 24 long and travels
// over 96 − 24 = 72 as the offset goes 0…300.

// scrollerLines returns what the content of a scrollEnv window (300 wide)
// draws: the dump lines between the content background and the title bar,
// clips cut off (controlLines for a 200-wide window).
func scrollerLines(dump string) string {
	lines := strings.Split(dump, "\n")
	var out []string
	for _, l := range lines[1:] {
		if strings.HasPrefix(l, "fillRRect (0,0,300,28)") {
			break
		}
		l, _, _ = strings.Cut(l, " clip=")
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// fadeSettle advances frame by frame until neither axis moves and returns the
// time of the last change of the shown offset (the last flash of the
// scrollers); start is the time to report when nothing changed.
func (e *scrollEnv) fadeSettle(t *testing.T) time.Duration {
	t.Helper()
	last := e.clk.Now()
	off := driver.Point{X: e.sv.x.Offset(), Y: e.sv.y.Offset()}
	for i := 0; e.sv.x.Moving() || e.sv.y.Moving(); i++ {
		if i > 2000 {
			t.Fatal("the scroll view never comes to rest")
		}
		e.clk.Advance(fake.FrameInterval)
		if now := (driver.Point{X: e.sv.x.Offset(), Y: e.sv.y.Offset()}); now != off {
			off, last = now, e.clk.Now()
		}
	}
	return last
}

// fadeAdvanceTo advances the clock to t (not before Now).
func (e *scrollEnv) fadeAdvanceTo(t *testing.T, at time.Duration) int {
	t.Helper()
	d := at - e.clk.Now()
	if d < 0 {
		t.Fatalf("fadeAdvanceTo %v: already at %v", at, e.clk.Now())
	}
	return e.clk.Advance(d)
}

// fadeHold sends a precise gesture's PhaseBegan with delta dy over the scroll
// view and no PhaseEnded: the fingers stay down.
func (e *scrollEnv) fadeHold(dy float64) {
	e.sf.Send(shell.ScrollEvent{Pos: e.at(50, 50), Delta: pt(0, dy), Precise: true, Phase: shell.PhaseBegan})
}

func scrFadeDelay() time.Duration { return tokens.Animation.ScrollerFadeDelay }
func scrFadeDur() time.Duration   { return tokens.Animation.ScrollerFadeDuration }

// knobShown fails unless b is shown at full opacity.
func knobShown(t *testing.T, b *scroller, what string) {
	t.Helper()
	if b.Hidden() || b.alpha.Value() != 1 {
		t.Fatalf("%s: hidden %v, opacity %v, want shown at 1", what, b.Hidden(), b.alpha.Value())
	}
}

// knobHidden fails unless b is hidden at opacity 0.
func knobHidden(t *testing.T, b *scroller, what string) {
	t.Helper()
	if !b.Hidden() || b.alpha.Value() != 0 {
		t.Fatalf("%s: hidden %v, opacity %v, want hidden at 0", what, b.Hidden(), b.alpha.Value())
	}
}

func TestScrollerStates(t *testing.T) {
	var out strings.Builder
	light := driver.DefaultAppearance()
	dump := func(name string, sys driver.Appearance, f func(e *scrollEnv)) {
		scrollRun(t, sys, func(e *scrollEnv) {
			f(e)
			fmt.Fprintf(&out, "## %s\n%s\n", name, scrollerLines(paint(t, e.sh, e.sf)))
		})
	}

	// Never scrolled: the scroller is hidden, nothing is drawn.
	dump("idle", light, func(e *scrollEnv) {})

	// One wheel notch down: offset 16 (WheelLineHeight). The knob starts at
	// 2 + 72 · 16/300 = 5.84, pixel-aligned to 6: surface y 38 + 6 = 44,
	// (152, 44, 6, 24), r = 3, ScrollerKnob light #00000073.
	dump("visible (wheel)", light, func(e *scrollEnv) {
		e.sf.Wheel(e.at(50, 50), pt(0, -1))
		e.fadeSettle(t)
		g := e.vbar().geometry()
		if e.sv.Offset().Y != 16 || g.knobLen != 24 || math.Abs(g.knob0-5.84) > 1e-9 || g.across0 != 5 || g.thickness != 6 {
			t.Errorf("wheel: offset %v, knob %v at %v, across %v, thick %v", e.sv.Offset(), g.knobLen, g.knob0, g.across0, g.thickness)
		}
	})
	// A swipe (no momentum: one event and its end at the same time).
	dump("visible (swipe)", light, func(e *scrollEnv) {
		e.sf.Swipe(e.at(50, 50), pt(0, -150))
		e.fadeSettle(t)
	})

	// Half way through the fade: EaseInOut(0.5) = 0.5, so the knob's alpha
	// is 115 · 0.5 ≈ 57 (#39 or #3a).
	dump("fading", light, func(e *scrollEnv) {
		e.sf.Wheel(e.at(50, 50), pt(0, -1))
		t0 := e.fadeSettle(t)
		e.fadeAdvanceTo(t, t0+scrFadeDelay()+scrFadeDur()/2)
		if a := e.vbar().alpha.Value(); math.Abs(a-0.5) > 0.06 {
			t.Errorf("mid-fade opacity %v", a)
		}
	})

	// Rubber-banding at the top, fingers down: the knob is pinned at the
	// track's start (surface y 40) and shortened by overscroll · 96/100.
	dump("overscroll top", light, func(e *scrollEnv) {
		e.fadeHold(10)
		o := e.sv.y.Overscroll()
		g := e.vbar().geometry()
		want := math.Max(6, 24-math.Abs(o)*96/100)
		if o >= 0 || g.knob0 != 2 || math.Abs(g.knobShown-want) > 1e-9 {
			t.Errorf("top: overscroll %v, knob %v at %v, want %v at 2", o, g.knobShown, g.knob0, want)
		}
	})
	dump("overscroll top far (round knob)", light, func(e *scrollEnv) {
		e.fadeHold(400)
		if g := e.vbar().geometry(); g.knobShown != 6 || g.knob0 != 2 {
			t.Errorf("far top: knob %v at %v, want 6 at 2", g.knobShown, g.knob0)
		}
	})
	// At the bottom: pinned to the track's end (98, surface y 136).
	dump("overscroll bottom", light, func(e *scrollEnv) {
		e.sv.SetOffset(driver.Point{Y: 300})
		e.fadeHold(-10)
		o := e.sv.y.Overscroll()
		g := e.vbar().geometry()
		want := math.Max(6, 24-math.Abs(o)*96/100)
		if o <= 0 || math.Abs(g.knobShown-want) > 1e-9 || math.Abs(g.knob0+g.knobShown-98) > 1e-9 {
			t.Errorf("bottom: overscroll %v, knob %v at %v, want %v ending at 98", o, g.knobShown, g.knob0, want)
		}
	})

	for _, c := range []struct {
		name string
		a    driver.Appearance
	}{
		{"dark", driver.Appearance{Dark: true, Accent: driver.DefaultAccent}},
		{"increased contrast", driver.Appearance{IncreaseContrast: true, Accent: driver.DefaultAccent}},
		{"dark, increased contrast", driver.Appearance{Dark: true, IncreaseContrast: true, Accent: driver.DefaultAccent}},
	} {
		// The window's appearance and the shell's (system) one.
		dump(c.name+" (window)", light, func(e *scrollEnv) {
			e.w.SetAppearance(c.a)
			e.sf.Wheel(e.at(50, 50), pt(0, -1))
			e.fadeSettle(t)
		})
		dump(c.name+" (system)", c.a, func(e *scrollEnv) {
			e.sf.Wheel(e.at(50, 50), pt(0, -1))
			e.fadeSettle(t)
		})
	}

	// Both scrollers, a 300 × 400 document, offset (0, 16). The corner is
	// cut: the vertical strip is (137, 0, 13, 87), its track 83 long, its
	// knob 83 · 100/400 = 20.75 at 2 + 62.25 · 16/300 = 5.32 (surface
	// 152, 43.32 → aligned); the horizontal strip is (0, 87, 137, 13), its
	// track 133 long, its knob 133 · 150/300 = 66.5 at 2, across 87 + 5:
	// surface (12, 130, 66.5, 6) before alignment; aligned: (152, 43, 6, 21)
	// and (12, 130, 67, 6).
	dump("both scrollers", light, func(e *scrollEnv) {
		e.sv.SetHasHorizontalScroller(true)
		e.sv.SetDocumentSize(driver.Size{W: 300, H: 400})
		e.sf.Wheel(e.at(50, 50), pt(0, -1))
		e.fadeSettle(t)
		if v, h := e.vbar().Frame(), e.hbar().Frame(); v != (driver.Rect{X: 137, W: 13, H: 87}) || h != (driver.Rect{Y: 87, W: 137, H: 13}) {
			t.Errorf("strips %v, %v", v, h)
		}
		if g := e.hbar().geometry(); g.knobLen != 66.5 || g.knob0 != 2 || g.across0 != 5 {
			t.Errorf("horizontal knob %v at %v across %v", g.knobLen, g.knob0, g.across0)
		}
		if g := e.vbar().geometry(); g.knobLen != 20.75 || math.Abs(g.knob0-5.32) > 1e-9 {
			t.Errorf("vertical knob %v at %v", g.knobLen, g.knob0)
		}
	})
	// Both scrollers, but only the vertical axis can scroll: no corner,
	// no horizontal knob.
	dump("both scrollers, horizontal cannot scroll", light, func(e *scrollEnv) {
		e.sv.SetHasHorizontalScroller(true)
		e.sf.Wheel(e.at(50, 50), pt(0, -1))
		e.fadeSettle(t)
		if v := e.vbar().Frame(); v != (driver.Rect{X: 137, W: 13, H: 100}) || !e.hbar().Hidden() {
			t.Errorf("strip %v, horizontal hidden %v", v, e.hbar().Hidden())
		}
	})
	// In an inactive window (S6.1a): the wheel scrolls and shows the
	// scroller, drawn as in a key window, light and dark, one and both
	// scrollers.
	for _, ap := range []driver.Appearance{light, {Dark: true, Accent: driver.DefaultAccent},
		{IncreaseContrast: true, Accent: driver.DefaultAccent}, {Dark: true, IncreaseContrast: true, Accent: driver.DefaultAccent}} {
		mode := "light"
		if ap.Dark {
			mode = "dark"
		}
		if ap.IncreaseContrast {
			mode += ", increased contrast"
		}
		dump("inactive, "+mode, ap, func(e *scrollEnv) {
			e.sh.Deactivate()
			e.sf.Wheel(e.at(50, 50), pt(0, -1))
			e.fadeSettle(t)
			if e.w.IsKey() || e.sv.Offset().Y != 16 {
				t.Errorf("inactive: key %v, offset %v", e.w.IsKey(), e.sv.Offset())
			}
		})
		dump("inactive, both scrollers, "+mode, ap, func(e *scrollEnv) {
			e.sh.Deactivate()
			e.sv.SetHasHorizontalScroller(true)
			e.sv.SetDocumentSize(driver.Size{W: 300, H: 400})
			e.sf.Wheel(e.at(50, 50), pt(0, -1))
			e.fadeSettle(t)
		})
	}

	got := out.String()
	path := filepath.Join("testdata", "scroll.states")
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
		t.Errorf("scroller states differ from %s (rerun with -update if the change is intended):\n%s", path, got)
	}
}

// TestScrollerFade: a wheel step shows the scroller at once; it stays
// opaque until ScrollerFadeDelay after the last offset change, fades over
// ScrollerFadeDuration, then is hidden, unpainted and not hit-tested, and
// the app goes idle.
func TestScrollerFade(t *testing.T) {
	scrollRun(t, driver.DefaultAppearance(), func(e *scrollEnv) {
		var log []string
		content := newProbe(&log, "content", 100, 20, 50, 60) // under the strip at offsets 0 and 16
		e.doc.SetChildren([]driver.Widget{contentWidget{content}})
		strip := e.at(145, 50) // in the strip (137…150), below the knob at offset 16

		b := e.vbar()
		knobHidden(t, b, "idle")
		if hit, _ := HitTest(e.w.Root(), strip); hit != Element(content) {
			t.Fatalf("idle: the strip hits %T, want the content", hit)
		}
		e.sf.Wheel(e.at(50, 50), pt(0, -1))
		knobShown(t, b, "at the wheel event")
		t0 := e.fadeSettle(t)
		if t0 < tokens.Scroll.WheelStepDuration-fake.FrameInterval {
			t.Errorf("the step's last change at %v", t0)
		}
		knobShown(t, b, "after the step")
		if hit, _ := HitTest(e.w.Root(), strip); hit != Element(b) {
			t.Fatalf("shown: the strip hits %T, want the scroller", hit)
		}
		e.fadeAdvanceTo(t, t0+scrFadeDelay()-time.Millisecond)
		knobShown(t, b, "just before the delay")
		e.fadeAdvanceTo(t, t0+scrFadeDelay()+scrFadeDur()/4)
		if a := b.alpha.Value(); a <= 0 || a >= 1 || b.Hidden() {
			t.Fatalf("a quarter into the fade: opacity %v, hidden %v", a, b.Hidden())
		}
		q := b.alpha.Value()
		e.fadeAdvanceTo(t, t0+scrFadeDelay()+3*scrFadeDur()/4)
		if a := b.alpha.Value(); a <= 0 || a >= q {
			t.Fatalf("three quarters into the fade: opacity %v (was %v)", a, q)
		}
		e.fadeAdvanceTo(t, t0+scrFadeDelay()+scrFadeDur()+fake.FrameInterval)
		knobHidden(t, b, "after the fade")
		if l := scrollerLines(paint(t, e.sh, e.sf)); strings.Contains(l, "(152,") {
			t.Errorf("a hidden knob is painted:\n%s", l)
		}
		if e.clk.Requested() || e.d.Timeline().Running() != 0 {
			t.Errorf("hidden: requested %v, running %d", e.clk.Requested(), e.d.Timeline().Running())
		}
		if n := e.clk.Advance(time.Second); n != 0 {
			t.Errorf("hidden: %d frames in a second", n)
		}
		if hit, _ := HitTest(e.w.Root(), strip); hit != Element(content) {
			t.Fatalf("hidden: the strip hits %T, want the content", hit)
		}
		// A real click first moves the pointer into the strip, which shows
		// the scroller (hover, S3.5f), so only the hit test is checked here.
	})
}

// TestScrollerFadeMomentum: the fade counts from the end of the motion, so
// the driver's momentum after a swipe keeps the scroller opaque.
func TestScrollerFadeMomentum(t *testing.T) {
	scrollRun(t, driver.DefaultAppearance(), func(e *scrollEnv) {
		b := e.vbar()
		e.sv.SetOffset(driver.Point{Y: 50})
		e.sf.TimedSwipe(e.at(50, 50), fake.FrameInterval, pt(0, -8), pt(0, -8), pt(0, -8), pt(0, -8))
		if !e.sv.y.Moving() {
			t.Fatal("no momentum after the swipe")
		}
		start := e.clk.Now()
		last := start
		off := e.sv.y.Offset()
		for i := 0; e.sv.y.Moving(); i++ {
			if i > 2000 {
				t.Fatal("the momentum never ends")
			}
			e.clk.Advance(fake.FrameInterval)
			if o := e.sv.y.Offset(); o != off {
				off, last = o, e.clk.Now()
				knobShown(t, b, fmt.Sprintf("momentum at %v", last))
			}
		}
		if last-start < scrFadeDelay()/4 {
			t.Fatalf("momentum too short to test (%v): retune the swipe", last-start)
		}
		e.fadeAdvanceTo(t, last+scrFadeDelay()-time.Millisecond)
		knobShown(t, b, "just before the delay after the momentum")
		e.fadeAdvanceTo(t, last+scrFadeDelay()+scrFadeDur()/2)
		if a := b.alpha.Value(); a <= 0 || a >= 1 {
			t.Fatalf("mid-fade after the momentum: opacity %v", a)
		}
		e.fadeAdvanceTo(t, last+scrFadeDelay()+scrFadeDur()+fake.FrameInterval)
		knobHidden(t, b, "after the fade")
	})
	// OS momentum (GTK) re-arms the delay too.
	scrollRun(t, driver.DefaultAppearance(), func(e *scrollEnv) {
		b := e.vbar()
		e.sv.SetOffset(driver.Point{Y: 50})
		e.sf.Swipe(e.at(50, 50), pt(0, -10))
		e.fadeSettle(t)
		e.clk.Advance(scrFadeDelay() - 100*time.Millisecond)
		knobShown(t, b, "before the OS momentum")
		e.sf.Momentum(pt(0, -5), pt(0, -5))
		t0 := e.clk.Now()
		e.fadeSettle(t)
		e.fadeAdvanceTo(t, t0+scrFadeDelay()-time.Millisecond)
		knobShown(t, b, "just before the delay after the OS momentum")
		e.fadeAdvanceTo(t, t0+scrFadeDelay()+scrFadeDur()+fake.FrameInterval)
		knobHidden(t, b, "after the fade")
	})
}

// TestScrollerFadeRestart: scrolling again mid-fade restores opacity 1 at
// once and restarts the delay.
func TestScrollerFadeRestart(t *testing.T) {
	scrollRun(t, driver.DefaultAppearance(), func(e *scrollEnv) {
		b := e.vbar()
		e.sf.Wheel(e.at(50, 50), pt(0, -1))
		t0 := e.fadeSettle(t)
		e.fadeAdvanceTo(t, t0+scrFadeDelay()+scrFadeDur()/2)
		if a := b.alpha.Value(); a <= 0 || a >= 1 {
			t.Fatalf("mid-fade: opacity %v", a)
		}
		e.sf.Wheel(e.at(50, 50), pt(0, -1))
		knobShown(t, b, "scrolled again mid-fade")
		t1 := e.fadeSettle(t)
		e.fadeAdvanceTo(t, t1+scrFadeDelay()-time.Millisecond)
		knobShown(t, b, "just before the new delay")
		e.fadeAdvanceTo(t, t1+scrFadeDelay()+scrFadeDur()+fake.FrameInterval)
		knobHidden(t, b, "after the new fade")
		// A precise swipe mid-fade too.
		e.sf.Wheel(e.at(50, 50), pt(0, -1))
		t2 := e.fadeSettle(t)
		e.fadeAdvanceTo(t, t2+scrFadeDelay()+scrFadeDur()/2)
		e.sf.Swipe(e.at(50, 50), pt(0, -3))
		knobShown(t, b, "swiped mid-fade")
	})
}

// TestScrollerNotShownByProgram: SetOffset, ScrollRectToVisible and
// SetDocumentSize move the knob but do not show the scroller; a shown one
// keeps its fade.
func TestScrollerNotShownByProgram(t *testing.T) {
	scrollRun(t, driver.DefaultAppearance(), func(e *scrollEnv) {
		b := e.vbar()
		e.sv.SetOffset(driver.Point{Y: 100})
		knobHidden(t, b, "SetOffset")
		e.sv.ScrollRectToVisible(driver.Rect{Y: 350, W: 10, H: 20})
		knobHidden(t, b, "ScrollRectToVisible")
		if e.sv.Offset().Y != 270 {
			t.Errorf("ScrollRectToVisible: offset %v", e.sv.Offset())
		}
		e.sv.SetDocumentSize(driver.Size{W: 150, H: 600})
		knobHidden(t, b, "SetDocumentSize")
		e.clk.Advance(time.Second)
		knobHidden(t, b, "a second later")
		if l := scrollerLines(paint(t, e.sh, e.sf)); l != "" {
			t.Errorf("painted:\n%s", l)
		}
		if len(e.h.scrolled) != 0 {
			t.Errorf("Scrolled %v", e.h.scrolled)
		}
		// Shown by the user, then moved by the program: the fade is not
		// re-armed.
		e.sf.Wheel(e.at(50, 50), pt(0, -1))
		t0 := e.fadeSettle(t)
		e.fadeAdvanceTo(t, t0+scrFadeDelay()/2)
		e.sv.SetOffset(driver.Point{Y: 10})
		e.sv.SetDocumentSize(driver.Size{W: 150, H: 500})
		e.fadeAdvanceTo(t, t0+scrFadeDelay()+scrFadeDur()+fake.FrameInterval)
		knobHidden(t, b, "the fade armed by the user")
	})
}

// TestScrollerCannotScroll: an axis whose document fits the viewport never
// shows its scroller; one that becomes unscrollable while shown hides.
func TestScrollerCannotScroll(t *testing.T) {
	scrollRun(t, driver.DefaultAppearance(), func(e *scrollEnv) {
		b := e.vbar()
		e.sv.SetDocumentSize(driver.Size{W: 150, H: 100})
		e.sf.Wheel(e.at(50, 50), pt(0, -1))
		e.sf.Swipe(e.at(50, 50), pt(0, -20))
		knobHidden(t, b, "document = viewport")
		e.fadeSettle(t)
		knobHidden(t, b, "document = viewport, settled")
		e.sv.UserScroll(driver.Point{})
		knobHidden(t, b, "UserScroll")
		if g := b.geometry(); g.ok {
			t.Error("a knob on an axis that cannot scroll")
		}
		if l := scrollerLines(paint(t, e.sh, e.sf)); l != "" {
			t.Errorf("painted:\n%s", l)
		}

		// Both scrollers, the horizontal axis cannot scroll: only the
		// vertical one shows.
		e.sv.SetHasHorizontalScroller(true)
		e.sv.SetDocumentSize(driver.Size{W: 100, H: 400})
		e.sf.Wheel(e.at(50, 50), pt(0, -1))
		knobShown(t, b, "vertical")
		if !e.hbar().Hidden() || e.hbar().alpha.Value() != 0 {
			t.Errorf("horizontal shown (opacity %v)", e.hbar().alpha.Value())
		}
		e.fadeSettle(t)

		// Shown, then the document shrinks to the viewport: hidden.
		e.sv.SetDocumentSize(driver.Size{W: 100, H: 80})
		if !b.Hidden() {
			t.Error("shown after the document shrank to the viewport")
		}
		if l := scrollerLines(paint(t, e.sh, e.sf)); l != "" {
			t.Errorf("painted:\n%s", l)
		}
	})
}

// TestScrollerReduceMotion: no intermediate opacity; opaque until the
// delay, then hidden at once.
func TestScrollerReduceMotion(t *testing.T) {
	scrollRun(t, driver.Appearance{Accent: driver.DefaultAccent, ReduceMotion: true}, func(e *scrollEnv) {
		b := e.vbar()
		e.sf.Wheel(e.at(50, 50), pt(0, -1))
		if e.sv.Offset().Y != 16 {
			t.Fatalf("the wheel step does not jump: %v", e.sv.Offset())
		}
		knobShown(t, b, "at the wheel event")
		t0 := e.clk.Now()
		end := t0 + scrFadeDelay() + scrFadeDur() + time.Second
		for e.clk.Now() < end {
			e.clk.Advance(fake.FrameInterval)
			a := b.alpha.Value()
			switch {
			case a != 0 && a != 1:
				t.Fatalf("opacity %v at %v", a, e.clk.Now()-t0)
			case a == 1 && e.clk.Now()-t0 > scrFadeDelay()+fake.FrameInterval:
				t.Fatalf("still shown at %v", e.clk.Now()-t0)
			case a == 0 && e.clk.Now()-t0 < scrFadeDelay():
				t.Fatalf("hidden at %v, before the delay", e.clk.Now()-t0)
			}
			if b.Hidden() != (a == 0) {
				t.Fatalf("hidden %v at opacity %v", b.Hidden(), a)
			}
		}
		knobHidden(t, b, "after the delay")
		if e.clk.Requested() {
			t.Error("frames requested once hidden")
		}
	})
	// Turned on at runtime mid-delay: the delay still runs, then it jumps.
	scrollRun(t, driver.DefaultAppearance(), func(e *scrollEnv) {
		b := e.vbar()
		e.sf.Wheel(e.at(50, 50), pt(0, -1))
		t0 := e.fadeSettle(t)
		e.fadeAdvanceTo(t, t0+scrFadeDelay()/2)
		e.sh.SetAppearance(driver.Appearance{Accent: driver.DefaultAccent, ReduceMotion: true})
		e.clk.Advance(0)
		knobShown(t, b, "Reduce Motion turned on mid-delay")
		e.fadeAdvanceTo(t, t0+scrFadeDelay()-time.Millisecond)
		knobShown(t, b, "just before the delay")
		e.fadeAdvanceTo(t, t0+scrFadeDelay()+fake.FrameInterval)
		knobHidden(t, b, "just after the delay")
	})
}

// TestScrollerKnobGeometry: the knob's length is proportional to the
// visible fraction, at least ScrollerMinKnobLength, and it reaches the
// track's end at the maximum offset.
func TestScrollerKnobGeometry(t *testing.T) {
	scrollRun(t, driver.DefaultAppearance(), func(e *scrollEnv) {
		b := e.vbar()
		knob := func(what string, len0, at float64, line string) {
			t.Helper()
			g := b.geometry()
			if !g.ok || math.Abs(g.knobLen-len0) > 1e-9 || math.Abs(g.knob0-at) > 1e-9 || g.knobShown != g.knobLen {
				t.Errorf("%s: knob %v (shown %v) at %v, want %v at %v", what, g.knobLen, g.knobShown, g.knob0, len0, at)
			}
			if l := scrollerLines(paint(t, e.sh, e.sf)); l != line {
				t.Errorf("%s: painted\n%s\nwant\n%s", what, l, line)
			}
		}
		// 100/400 of 96 = 24, at the top, at the bottom (2 + 72 = 74,
		// ending at 98 = the track's end), in the middle (2 + 36).
		e.sv.UserScroll(driver.Point{})
		knob("top", 24, 2, "fillRRect (152,40,6,24) r=3 solid #00000073")
		e.sv.UserScroll(driver.Point{Y: 300})
		knob("bottom", 24, 74, "fillRRect (152,112,6,24) r=3 solid #00000073")
		e.sv.UserScroll(driver.Point{Y: 150})
		knob("middle", 24, 38, "fillRRect (152,76,6,24) r=3 solid #00000073")
		// 100/200 of 96 = 48.
		e.sv.SetDocumentSize(driver.Size{W: 150, H: 200})
		e.sv.UserScroll(driver.Point{Y: 100})
		knob("half", 48, 50, "fillRRect (152,88,6,48) r=3 solid #00000073")
		// A huge document: 96 · 100/100000 < 18, so 18, ending at 98 at the
		// maximum offset (2 + 96 − 18 = 80).
		e.sv.SetDocumentSize(driver.Size{W: 150, H: 100000})
		e.sv.UserScroll(driver.Point{Y: 0})
		knob("huge, top", 18, 2, "fillRRect (152,40,6,18) r=3 solid #00000073")
		e.sv.UserScroll(driver.Point{Y: 99900})
		knob("huge, bottom", 18, 80, "fillRRect (152,118,6,18) r=3 solid #00000073")
		// A track shorter than ScrollerMinKnobLength: the knob is the track.
		e.sv.SetFrame(driver.Rect{X: 10, Y: 10, W: 150, H: 14})
		e.sv.UserScroll(driver.Point{Y: 50})
		if g := b.geometry(); !g.ok || g.knobLen != 10 || g.knob0 != 2 {
			t.Errorf("short track: knob %v at %v, ok %v", g.knobLen, g.knob0, g.ok)
		}
		// A track shorter than the knob's thickness: no knob.
		e.sv.SetFrame(driver.Rect{X: 10, Y: 10, W: 150, H: 9})
		e.sv.UserScroll(driver.Point{Y: 50})
		if g := b.geometry(); g.ok {
			t.Errorf("track %v < thickness: a knob", g.track)
		}
	})
}
