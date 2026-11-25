package aqua

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// The overlay scrollers' pointer input (PLAN.md S3.5f): hover-widen, knob
// drag and track click. In scrollRun's window the vertical strip is
// 13 pt wide at x 137…150 of the scroll view (ScrollerHoverThickness 9 +
// 2 × ScrollerInset 2), its track y 2…98 (96 pt); the knob is
// max(18, 96 × 100/400) = 24 pt long and travels 72 pt for the offset
// 0…300.

const (
	inStrip  = 145.0 // x of a point in the vertical strip
	knobLen  = 24.0
	knobTrav = 72.0
)

// knobThickness returns the vertical knob's current width.
func knobThickness(e *scrollEnv) float64 {
	g := e.vbar().geometry()
	return g.thickness
}

func TestScrollerInputHoverWidens(t *testing.T) {
	scrollRun(t, driver.Appearance{}, func(e *scrollEnv) {
		sc := &tokens.Scroll
		an := &tokens.Animation
		b := e.vbar()
		if !b.Hidden() {
			t.Fatal("shown before any scrolling")
		}
		e.sf.Move(e.at(50, 50))
		if !b.Hidden() || b.hovered {
			t.Fatal("over the content: shown")
		}
		e.sf.Move(e.at(inStrip, 50))
		if b.Hidden() || b.alpha.Value() != 1 || !b.hovered {
			t.Fatalf("over the strip: hidden %v alpha %v", b.Hidden(), b.alpha.Value())
		}
		e.clk.Advance(sc.ScrollerHoverDuration / 2)
		if w := knobThickness(e); !(w > sc.ScrollerThickness && w < sc.ScrollerHoverThickness) {
			t.Errorf("mid-widen thickness %v", w)
		}
		e.clk.Advance(sc.ScrollerHoverDuration)
		if w := knobThickness(e); w != sc.ScrollerHoverThickness {
			t.Errorf("widened thickness %v", w)
		}
		// The painted knob: 9 wide, 2 pt in from the strip's right edge.
		if got := controlLines(paint(t, e.sh, e.sf)); !strings.Contains(got, "fillRRect (149,40,9,24) r=4.5 solid #00000073") {
			t.Errorf("hovered knob:\n%s", got)
		}
		// Hovered: it never fades.
		e.clk.Advance(an.ScrollerFadeDelay + an.ScrollerFadeDuration + time.Second)
		if b.Hidden() || b.alpha.Value() != 1 {
			t.Fatalf("hovered: faded (alpha %v)", b.alpha.Value())
		}
		// Off the strip, still over the scroll view: narrows and fades.
		e.sf.Move(e.at(50, 50))
		if b.hovered {
			t.Fatal("still hovered")
		}
		e.clk.Advance(2 * sc.ScrollerHoverDuration)
		if w := knobThickness(e); w != sc.ScrollerThickness {
			t.Errorf("narrowed thickness %v", w)
		}
		e.clk.Advance(an.ScrollerFadeDelay + an.ScrollerFadeDuration)
		if !b.Hidden() || b.alpha.Value() != 0 {
			t.Fatalf("after leaving the strip: hidden %v alpha %v", b.Hidden(), b.alpha.Value())
		}
		if e.clk.Requested() {
			t.Error("frames requested once hidden")
		}
		// Leaving the window from the strip does the same.
		e.sf.Move(e.at(inStrip, 50))
		e.sf.Move(driver.Point{X: 500, Y: 500})
		if b.hovered {
			t.Fatal("hovered after leaving the window")
		}
		e.clk.Advance(an.ScrollerFadeDelay + an.ScrollerFadeDuration + sc.ScrollerHoverDuration)
		if !b.Hidden() {
			t.Fatal("not hidden after leaving the window")
		}
	})
}

func TestScrollerInputHoverNoRange(t *testing.T) {
	scrollRun(t, driver.Appearance{}, func(e *scrollEnv) {
		e.sv.SetDocumentSize(driver.Size{W: 150, H: 80}) // fits: nothing to scroll
		e.sf.Move(e.at(inStrip, 50))
		if b := e.vbar(); !b.Hidden() || b.hovered || b.alpha.Value() != 0 {
			t.Fatalf("no range: hidden %v hovered %v alpha %v", b.Hidden(), b.hovered, b.alpha.Value())
		}
	})
}

func TestScrollerInputHoverReduceMotion(t *testing.T) {
	scrollRun(t, driver.Appearance{ReduceMotion: true}, func(e *scrollEnv) {
		e.sf.Move(e.at(inStrip, 50))
		if w := knobThickness(e); w != tokens.Scroll.ScrollerHoverThickness {
			t.Fatalf("reduce motion: thickness %v, want a jump", w)
		}
		e.sf.Move(e.at(50, 50))
		if w := knobThickness(e); w != tokens.Scroll.ScrollerThickness {
			t.Fatalf("reduce motion: thickness %v after leaving", w)
		}
	})
}

func TestScrollerInputKnobDrag(t *testing.T) {
	scrollRun(t, driver.Appearance{}, func(e *scrollEnv) {
		an := &tokens.Animation
		b := e.vbar()
		// The knob spans y 2…26 at offset 0; grab it at 14 (12 into it).
		e.sf.Move(e.at(inStrip, 14))
		e.sf.Press(e.at(inStrip, 14), shell.ButtonPrimary)
		if !b.dragging || e.w.router.Captured() != Element(b) {
			t.Fatalf("press on the knob: dragging %v, captured %T", b.dragging, e.w.router.Captured())
		}
		if len(e.h.scrolled) != 0 {
			t.Fatalf("press scrolled: %v", e.h.scrolled)
		}
		// 36 pt down: half the travel, offset 150.
		e.sf.Move(e.at(inStrip, 50))
		if got := e.sv.Offset().Y; got != 150 {
			t.Errorf("drag to the middle: offset %v, want 150", got)
		}
		n := len(e.h.scrolled)
		if n != 1 || e.h.scrolled[0].Y != 150 {
			t.Errorf("scrolled %v", e.h.scrolled)
		}
		// 1:1: one more point of pointer is 300/72 of offset.
		e.sf.Move(e.at(inStrip, 51))
		if got, want := e.sv.Offset().Y, 150+300.0/knobTrav; abs(got-want) > 1e-9 {
			t.Errorf("1 pt more: offset %v, want %v", got, want)
		}
		// Off the strip, far below: clamped at the end; it keeps following.
		e.sf.Move(e.at(20, 400))
		if got := e.sv.Offset().Y; got != 300 {
			t.Errorf("dragged past the end: offset %v", got)
		}
		n = len(e.h.scrolled)
		e.sf.Move(e.at(20, 500)) // still clamped: no change, no Scrolled
		if len(e.h.scrolled) != n {
			t.Errorf("no-op move scrolled: %v", e.h.scrolled[n:])
		}
		e.sf.Move(e.at(20, -300))
		if got := e.sv.Offset().Y; got != 0 {
			t.Errorf("dragged past the start: offset %v", got)
		}
		// Held: visible past the fade.
		e.clk.Advance(an.ScrollerFadeDelay + an.ScrollerFadeDuration + time.Second)
		if b.Hidden() || b.alpha.Value() != 1 {
			t.Fatalf("faded during the drag: alpha %v", b.alpha.Value())
		}
		// Release (off the strip): ends the drag, arms the fade.
		e.sf.Release(e.at(20, -300), shell.ButtonPrimary)
		if b.dragging || b.press.Down() || e.w.router.Captured() != nil {
			t.Fatal("drag not ended by the release")
		}
		e.clk.Advance(an.ScrollerFadeDelay + an.ScrollerFadeDuration + tokens.Scroll.ScrollerHoverDuration)
		if !b.Hidden() {
			t.Fatalf("not faded after the release: alpha %v", b.alpha.Value())
		}
	})
}

func TestScrollerInputKnobDragCancelAndButtons(t *testing.T) {
	scrollRun(t, driver.Appearance{}, func(e *scrollEnv) {
		b := e.vbar()
		e.sf.Move(e.at(inStrip, 14))
		// Another button: ignored.
		e.sf.Press(e.at(inStrip, 14), shell.ButtonSecondary)
		if b.press.Down() {
			t.Fatal("secondary press started a drag")
		}
		e.sf.Release(e.at(inStrip, 14), shell.ButtonSecondary)
		e.sf.Press(e.at(inStrip, 14), shell.ButtonPrimary)
		e.sf.Move(e.at(inStrip, 50))
		e.sf.CancelCapture()
		if b.dragging || b.press.Down() {
			t.Fatal("cancel did not end the drag")
		}
		off := e.sv.Offset()
		e.sf.Move(e.at(inStrip, 80))
		if e.sv.Offset() != off {
			t.Errorf("moved after the cancel: %v → %v", off, e.sv.Offset())
		}
	})
}

func TestScrollerInputKnobDragStopsMomentum(t *testing.T) {
	scrollRun(t, driver.Appearance{}, func(e *scrollEnv) {
		// A fast swipe up (content up, offset down) leaves momentum.
		e.sf.TimedSwipe(e.at(50, 50), 8*time.Millisecond,
			driver.Point{Y: -20}, driver.Point{Y: -20}, driver.Point{Y: -20}, driver.Point{Y: -20})
		e.clk.Advance(16 * time.Millisecond)
		if !e.sv.y.Moving() {
			t.Fatal("no momentum after the swipe")
		}
		g := e.vbar().geometry()
		y := g.knob0 + g.knobShown/2
		e.sf.Move(e.at(inStrip, y))
		e.sf.Press(e.at(inStrip, y), shell.ButtonPrimary)
		if e.sv.y.Moving() {
			t.Fatal("momentum runs on after grabbing the knob")
		}
		off := e.sv.Offset()
		e.clk.Advance(200 * time.Millisecond)
		if e.sv.Offset() != off {
			t.Errorf("offset moved while the knob is held: %v → %v", off, e.sv.Offset())
		}
		e.sf.Release(e.at(inStrip, y), shell.ButtonPrimary)
	})
}

func TestScrollerInputDisabled(t *testing.T) {
	scrollRun(t, driver.Appearance{}, func(e *scrollEnv) {
		e.sv.UserScroll(driver.Point{}) // show it
		e.sv.SetEnabled(false)
		e.sf.Press(e.at(inStrip, 14), shell.ButtonPrimary)
		if e.vbar().press.Down() || e.w.router.Captured() != nil {
			t.Fatal("disabled scroll view: press taken")
		}
		e.sf.Move(e.at(inStrip, 60))
		e.sf.Release(e.at(inStrip, 60), shell.ButtonPrimary)
		if e.sv.Offset().Y != 0 || len(e.h.scrolled) != 1 { // the UserScroll's
			t.Errorf("disabled: offset %v, scrolled %v", e.sv.Offset(), e.h.scrolled)
		}
	})
}

func TestScrollerInputTrackClick(t *testing.T) {
	scrollRun(t, driver.Appearance{}, func(e *scrollEnv) {
		page := 100 - tokens.Scroll.PageOverlap // 90
		step := tokens.Scroll.WheelStepDuration
		b := e.vbar()
		e.sf.Move(e.at(inStrip, 80))
		e.sf.Click(e.at(inStrip, 80)) // below the knob (2…26)
		if b.dragging {
			t.Fatal("a track click is not a drag")
		}
		if e.sv.y.Target() != page {
			t.Fatalf("page down: target %v, want %v", e.sv.y.Target(), page)
		}
		e.clk.Advance(step / 2)
		if y := e.sv.Offset().Y; !(y > 0 && y < page) {
			t.Errorf("mid-step offset %v", y)
		}
		e.clk.Advance(step)
		if y := e.sv.Offset().Y; y != page {
			t.Errorf("page down: offset %v", y)
		}
		if len(e.h.scrolled) == 0 || e.h.scrolled[len(e.h.scrolled)-1].Y != page {
			t.Errorf("scrolled %v", e.h.scrolled)
		}
		// Clicks accumulate from the running page's target, and clamp.
		for range 4 {
			e.sf.Click(e.at(inStrip, 95))
		}
		if e.sv.y.Target() != 300 {
			t.Errorf("4 more pages: target %v, want 300 (clamped)", e.sv.y.Target())
		}
		e.clk.Advance(2 * step)
		// Above the knob (now at the bottom): pages up.
		e.sf.Click(e.at(inStrip, 10))
		e.clk.Advance(2 * step)
		if y := e.sv.Offset().Y; y != 300-page {
			t.Errorf("page up: offset %v, want %v", y, 300-page)
		}
		// The content under the strip got none of it.
		if e.w.router.Captured() != nil {
			t.Error("capture left after the clicks")
		}
	})
}

func TestScrollerInputTrackClickReduceMotion(t *testing.T) {
	scrollRun(t, driver.Appearance{ReduceMotion: true}, func(e *scrollEnv) {
		e.sf.Move(e.at(inStrip, 80))
		e.sf.Click(e.at(inStrip, 80))
		if y := e.sv.Offset().Y; y != 100-tokens.Scroll.PageOverlap {
			t.Fatalf("reduce motion page: offset %v, want a jump", y)
		}
	})
}

// While the knob is held, latched OS momentum and the wheel do not move
// the offset under the pointer (they are swallowed, not bubbled).
func TestScrollerInputKnobDragSwallowsScroll(t *testing.T) {
	scrollRun(t, driver.Appearance{}, func(e *scrollEnv) {
		e.sf.Swipe(e.at(50, 50), driver.Point{Y: -10}, driver.Point{Y: -10})
		g := e.vbar().geometry()
		y := g.knob0 + g.knobShown/2
		e.sf.Move(e.at(inStrip, y))
		e.sf.Press(e.at(inStrip, y), shell.ButtonPrimary)
		off := e.sv.Offset()
		e.sf.Momentum(driver.Point{Y: -30}, driver.Point{Y: -20}, driver.Point{Y: -10})
		e.sf.Wheel(e.at(inStrip, y), driver.Point{Y: -3})
		e.clk.Advance(500 * time.Millisecond)
		if e.sv.Offset() != off {
			t.Errorf("offset moved while the knob is held: %v → %v", off, e.sv.Offset())
		}
		e.sf.Release(e.at(inStrip, y), shell.ButtonPrimary)
		e.sf.Wheel(e.at(50, 50), driver.Point{Y: -3})
		e.clk.Advance(time.Second)
		if e.sv.Offset() == off {
			t.Error("the wheel does nothing after the release")
		}
	})
}

// A track click during the axis's own momentum pages from the offset, not
// from the momentum's rest.
func TestScrollerInputTrackClickDuringMomentum(t *testing.T) {
	scrollRun(t, driver.Appearance{}, func(e *scrollEnv) {
		e.sf.TimedSwipe(e.at(50, 50), 8*time.Millisecond,
			driver.Point{Y: -2}, driver.Point{Y: -2}, driver.Point{Y: -2}, driver.Point{Y: -2})
		e.clk.Advance(16 * time.Millisecond)
		a := e.sv.y
		if !a.Moving() || a.Stepping() {
			t.Fatal("no momentum after the swipe")
		}
		page := 100 - tokens.Scroll.PageOverlap
		want := math.Min(a.Max(), a.Clamped()+page)
		if math.Abs(a.Target()-want) < 1 {
			t.Fatalf("setup: momentum rest %v is a page away", a.Target())
		}
		e.sf.Move(e.at(inStrip, 90))
		e.sf.Click(e.at(inStrip, 90))
		if math.Abs(a.Target()-want) > 1e-9 {
			t.Errorf("page during momentum: target %v, want %v", a.Target(), want)
		}
	})
}
