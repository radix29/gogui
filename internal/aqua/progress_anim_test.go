package aqua

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell/fake"
)

// The fake-clock tests of the indeterminate Progress (PLAN.md S3.4f).

// indetEnv is a running driver with one window holding a container with
// a progress indicator, framed at r of the content (surface y + 28).
type indetEnv struct {
	d   *Driver
	sh  *fake.Shell
	w   *Window
	sf  *fake.Surface
	clk *fake.Clock
	c   *Container
	p   *Progress
}

// indetRun runs f on a started driver with the window not shown yet.
func indetRun(t *testing.T, r driver.Rect, f func(e *indetEnv)) {
	t.Helper()
	sh := fake.New()
	d := New(sh)
	h := &appLog{}
	h.onReady = func() {
		defer d.Quit()
		w, err := d.NewWindow(driver.WindowConfig{Title: "W", Size: driver.Size{W: 200, H: 100}, Style: allStyles}, &winLog{})
		if err != nil {
			t.Fatal(err)
		}
		e := &indetEnv{d: d, sh: sh, w: w.(*Window), clk: sh.FakeClock()}
		e.sf = e.w.Surface().(*fake.Surface)
		e.c = d.NewContainer().(*Container)
		e.p = d.NewProgress().(*Progress)
		e.c.SetFrame(driver.Rect{W: 200, H: 72})
		e.p.SetFrame(r)
		e.c.SetChildren([]driver.Widget{e.p})
		w.SetContent(e.c)
		f(e)
	}
	if err := runDriver(t, d, h); err != nil {
		t.Fatal(err)
	}
}

// settle advances a second and fails unless the app has gone idle: at
// most one frame (the repaint after the change) and no frame requested.
func (e *indetEnv) settle(t *testing.T, what string) {
	t.Helper()
	e.clk.Advance(time.Second)
	if e.clk.Requested() || e.p.view.Running() || e.d.Timeline().Running() != 0 {
		t.Fatalf("%s: still requesting frames (running %v)", what, e.p.view.Running())
	}
	if n := e.clk.Advance(time.Second); n != 0 {
		t.Fatalf("%s: %d frames while idle", what, n)
	}
}

// running advances a few frames and fails unless the loop runs and the
// phase moves.
func (e *indetEnv) running(t *testing.T, what string) {
	t.Helper()
	before := e.p.view.phase
	if n := e.clk.Advance(100 * time.Millisecond); n < 5 {
		t.Fatalf("%s: %d frames in 100 ms", what, n)
	}
	if !e.p.view.Running() || !e.clk.Requested() || e.p.view.phase == before {
		t.Fatalf("%s: running %v, requested %v, phase %v → %v", what, e.p.view.Running(), e.clk.Requested(), before, e.p.view.phase)
	}
}

// indetBand returns the band's line of a dump (the accent fill under the
// track's clip), or "".
func indetBand(dump string) string {
	for l := range strings.SplitSeq(dump, "\n") {
		if strings.HasPrefix(l, "fillRRect") && strings.Contains(l, "solid #007aff") {
			return l
		}
	}
	return ""
}

func TestProgressBarBand(t *testing.T) {
	indetRun(t, driver.Rect{X: 10, Y: 10, W: 120, H: 6}, func(e *indetEnv) {
		e.w.Show()
		e.settle(t, "determinate")
		e.p.SetIndeterminate(true)
		if !e.p.view.Running() {
			t.Fatal("SetIndeterminate(true) did not start the loop")
		}
		// The band (30 pt, a quarter of the track) at x = 10 − 30 +
		// phase × 150, clipped to the track.
		for _, d := range []time.Duration{0, 300 * time.Millisecond, 300 * time.Millisecond, 500 * time.Millisecond} {
			e.clk.Advance(d)
			e.clk.Flush()
			ph := e.p.view.phase
			x := 10 - 30 + math.Round(ph*150)
			want := fmt.Sprintf("fillRRect (%s,38,30,6) r=3 solid #007aff", fmtNumber(x))
			if got := indetBand(e.sf.Dump()); !strings.HasPrefix(got, want) || !strings.Contains(got, "clip=") {
				t.Fatalf("phase %v: band %q, want %q (clipped)", ph, got, want)
			}
		}
		if ph := e.p.view.phase; ph < 0.9 || ph >= 1 {
			t.Fatalf("phase after 1.1 s: %v", ph)
		}
		// It wraps around after one period (1.2 s).
		e.clk.Advance(200 * time.Millisecond)
		if ph := e.p.view.phase; ph > 0.2 {
			t.Fatalf("no wrap: %v", ph)
		}
		e.running(t, "indeterminate")

		e.p.SetIndeterminate(false)
		if e.p.view.Running() {
			t.Fatal("SetIndeterminate(false) did not stop at once")
		}
		e.settle(t, "determinate again")
		if got := indetBand(e.sf.Dump()); !strings.HasPrefix(got, "fillRRect (10,38,0") && got != "" {
			// value 0: no fill at all
			t.Fatalf("determinate draws a band %q", got)
		}
	})
}

func TestSpinnerSpins(t *testing.T) {
	indetRun(t, driver.Rect{X: 10, Y: 10, W: 32, H: 32}, func(e *indetEnv) {
		e.p.SetStyle(driver.ProgressSpinner)
		e.p.SetIndeterminate(true)
		e.w.Show()
		e.clk.Flush()
		lead := func() int {
			for i, a := range progSpokes(t, e.sf.Dump()) {
				if a == "8c" {
					return i
				}
			}
			t.Fatalf("no leading spoke:\n%s", e.sf.Dump())
			return -1
		}
		if lead() != 0 {
			t.Fatalf("lead %d at the start", lead())
		}
		// 12 spokes, one turn per second: the lead moves a spoke per
		// 1/12 s; the spokes behind fade to the tail.
		e.clk.Advance(250 * time.Millisecond)
		e.clk.Flush()
		if want := int(e.p.view.phase * 12); lead() != want || want < 2 {
			t.Fatalf("lead %d, want %d (phase %v)", lead(), want, e.p.view.phase)
		}
		sp := progSpokes(t, e.sf.Dump())
		if l := lead(); sp[(l+1)%12] != "23" || sp[(l+11)%12] == "23" {
			t.Fatalf("fade: %v", sp)
		}
		e.running(t, "spinner")

		// A style change retimes the loop to the bar's period.
		e.p.SetStyle(driver.ProgressBar)
		e.running(t, "retimed")
		e.p.SetStyle(driver.ProgressSpinner)
		e.p.SetValue(50)
		e.p.SetIndeterminate(false)
		e.settle(t, "determinate spinner")
	})
}

func TestProgressLoopStops(t *testing.T) {
	indetRun(t, driver.Rect{X: 10, Y: 10, W: 120, H: 6}, func(e *indetEnv) {
		// A window that was never shown runs no loop.
		e.p.SetIndeterminate(true)
		if e.p.view.Running() {
			t.Fatal("loop in a window not shown")
		}
		if n := e.clk.Advance(time.Second); n > 0 && e.clk.Requested() {
			t.Fatal("frames requested for a window not shown")
		}
		// Showing the window starts it with the first paint.
		e.w.Show()
		e.clk.Flush()
		e.running(t, "shown")

		// Hiding the indicator stops it at once; showing starts it.
		e.p.SetHidden(true)
		if e.p.view.Running() {
			t.Fatal("hidden: still running")
		}
		e.settle(t, "hidden")
		e.p.SetHidden(false)
		e.running(t, "shown again")

		// Hiding an ancestor stops it at the next frame; showing it
		// restarts it with the paint.
		e.c.SetHidden(true)
		e.settle(t, "ancestor hidden")
		e.c.SetHidden(false)
		e.running(t, "ancestor shown")

		// Leaving the window stops it.
		e.w.SetContent(nil)
		e.settle(t, "out of the window")
		e.w.SetContent(e.c)
		e.running(t, "back in the window")

		// Destroying it stops it.
		e.p.Destroy()
		e.settle(t, "destroyed")
	})
}

func TestProgressLoopStopsOnClose(t *testing.T) {
	indetRun(t, driver.Rect{X: 10, Y: 10, W: 120, H: 6}, func(e *indetEnv) {
		e.p.SetIndeterminate(true)
		e.w.Show()
		e.clk.Flush()
		e.running(t, "shown")
		e.w.Close()
		if !e.w.Closed() || e.w.Shown() {
			t.Fatal("Close")
		}
		e.settle(t, "closed")
	})
}

func TestProgressReduceMotion(t *testing.T) {
	indetRun(t, driver.Rect{X: 10, Y: 10, W: 120, H: 6}, func(e *indetEnv) {
		e.p.SetIndeterminate(true)
		e.w.Show()
		e.clk.Flush()
		e.running(t, "shown")

		rm := e.w.Appearance()
		rm.ReduceMotion = true
		e.sh.SetAppearance(rm)
		e.settle(t, "Reduce Motion")
		// The static look: the whole track in the fill at half alpha.
		if got := controlLines(e.sf.Dump()); !strings.Contains(got, "fillRRect (10,38,120,6) r=3 solid #007aff80") || strings.Contains(got, "clip=") {
			t.Fatalf("static bar:\n%s", got)
		}
		e.p.SetStyle(driver.ProgressSpinner)
		e.p.SetFrame(driver.Rect{X: 10, Y: 10, W: 32, H: 32})
		e.settle(t, "Reduce Motion spinner")
		if sp := progSpokes(t, e.sf.Dump()); sp[0] != "8c" || sp[1] != "23" || sp[11] == "23" {
			t.Fatalf("static spinner: %v", sp)
		}

		// Turned off, the next paint starts the loop again.
		rm.ReduceMotion = false
		e.sh.SetAppearance(rm)
		e.clk.Flush()
		e.running(t, "Reduce Motion off")
	})
}

// TestProgressIdle: an app with only determinate indicators (and one
// indeterminate but hidden) requests no frames once painted.
func TestProgressIdle(t *testing.T) {
	indetRun(t, driver.Rect{X: 10, Y: 10, W: 120, H: 6}, func(e *indetEnv) {
		sp := e.d.NewProgress()
		sp.SetStyle(driver.ProgressSpinner)
		sp.SetIndeterminate(true)
		sp.SetHidden(true)
		sp.SetFrame(driver.Rect{X: 10, Y: 30, W: 32, H: 32})
		e.c.SetChildren([]driver.Widget{e.p, sp})
		e.p.SetValue(30)
		e.w.Show()
		e.settle(t, "idle")
	})
}
