package gogui

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/native"
)

// The frame-pacing run (PLAN.md S6.2d): scripted scrolls and animations on
// the OS's own shell (real HWNDs and Direct2D on Windows, GTK 4 on Linux),
// each scenario in an app of its own. The script runs on a goroutine of
// its own and sends its events through DoSync, in real time, to the
// window's router (the shell's own input translation is tested in the
// shells). The frame clock's two observers feed a shell.Pacing: every
// frame of the scenario (its target time, and whether an animation still
// runs after it) and every present the display reports shown. A
// scenario's result is logged as a PACING-RESULT line (the refresh period
// from the OS, DWM's on Windows; the vblanks compared, missed on screen
// and skipped by the clock; the latency from a frame's target to its
// showing; the CPU work per frame), each frame that came more than a
// period after a running animation's previous one as a PACING-SKIP line
// (with that previous frame's split), the run's sum as PACING-TOTAL. On
// Windows a scenario fails when it misses more vblanks than
// shell.PacingTolerance, the run when all of them together miss more than
// shell.PacingRunTolerance; on Linux the numbers are only logged (the
// user's log; under xvfb there is no presentation feedback at all).
//
// The scenarios: in notes, the sidebar's ScrollView and the editor's
// TextView holding a 1 MB text, each with wheel steps, a momentum fling
// and a rubber band at the top with its spring-back; in controls, its
// indeterminate bar for two seconds and an alert sheet sliding in and
// out. The controls example has no ScrollView, so the sidebar of notes
// stands for it.
//
// It runs only with GOGUI_PACING=1 (native.sh sets it): its verdict needs
// a quiet host, which check.sh's parallel jobs are not.

// pacingScenario is one scripted run: build opens the scene in app and
// returns its window; script drives it from its own goroutine (the env's
// helpers synchronise with the UI thread).
type pacingScenario struct {
	scene, name, app string
	build            func(tb testing.TB, app *App) *Window
	script           func(e *pacingEnv)
}

// pacingEnv is a scenario's state.
type pacingEnv struct {
	t      *testing.T
	app    *App
	d      *aqua.Driver
	win    *Window
	aw     *aqua.Window
	base   int // the animations running by design (the indeterminate bar)
	pacing shell.Pacing
	work   nativeFrames
	// The previous counted frame, whether its animation went on, and the
	// frames that came more than a period after such a frame (UI thread).
	prevFrame shell.FrameTiming
	prevGoes  bool
	skips     []string
	period    time.Duration // the last present's refresh period (60 Hz before one)
	on        bool          // frames are counted (UI thread)
}

// ui runs fn on the UI thread and waits for it (the script's goroutine
// only).
func (e *pacingEnv) ui(fn func()) {
	if !DoSync(fn) {
		e.t.Error("the app is gone")
		runtime.Goexit() // the script's goroutine: its deferred Quit runs
	}
}

// widget returns find(win), found on the UI thread.
func (e *pacingEnv) widget(find func(*Window) Widget) (w Widget) {
	e.ui(func() { w = find(e.win) })
	return w
}

// at returns the centre of w in its window's surface points (UI thread).
func (e *pacingEnv) at(w Widget) driver.Point {
	el := aqua.ElementOf(w.peerWidget())
	f := el.(interface{ Frame() driver.Rect }).Frame()
	return aqua.ToRoot(el, driver.Point{X: f.W / 2, Y: f.H / 2})
}

// scroll sends a scroll event at the centre of w, stamped with the clock's
// Now.
func (e *pacingEnv) scroll(w Widget, ev shell.ScrollEvent) {
	e.ui(func() {
		ev.Pos = e.at(w)
		ev.Time = e.d.Clock().Now()
		e.aw.Router().Event(ev)
	})
}

// wheel turns the wheel n times by lines lines (not precise: the aqua
// driver animates each step), interval apart.
func (e *pacingEnv) wheel(w Widget, lines float64, n int, interval time.Duration) {
	for range n {
		e.scroll(w, shell.ScrollEvent{Delta: driver.Point{Y: lines}})
		time.Sleep(interval)
	}
}

// swipe sends a precise gesture as the Win32 shell does: deltas interval
// apart, then the zero-delta PhaseEnded after the gesture timeout.
func (e *pacingEnv) swipe(w Widget, interval time.Duration, deltas ...driver.Point) {
	for i, d := range deltas {
		ph := shell.PhaseChanged
		if i == 0 {
			ph = shell.PhaseBegan
		} else {
			time.Sleep(interval)
		}
		e.scroll(w, shell.ScrollEvent{Delta: d, Precise: true, Phase: ph})
	}
	time.Sleep(100 * time.Millisecond)
	e.scroll(w, shell.ScrollEvent{Precise: true, Phase: shell.PhaseEnded})
}

// settle waits until only the animations running by design are left (at
// most limit), then a little longer, so the last presents are reported.
func (e *pacingEnv) settle(limit time.Duration) {
	end := time.Now().Add(limit)
	for time.Now().Before(end) {
		running := 0
		e.ui(func() { running = e.d.Timeline().Running() })
		if running <= e.base {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
}

// keepPresenting invalidates the window for d, so the presents of the
// scenario's last frames are reported (the statistics come later).
func (e *pacingEnv) keepPresenting(d time.Duration) {
	e.ui(func() {
		e.on = false
		e.pacing.Break()
	})
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		e.ui(func() { e.aw.Surface().Invalidate() })
		time.Sleep(10 * time.Millisecond)
	}
}

// pacingNotes opens the notes example with a 1 MB text in its editor.
func pacingNotes(tb testing.TB, app *App) *Window {
	win := exampleNotes(tb, app)
	var b strings.Builder
	for i := 0; b.Len() < 1<<20; i++ {
		fmt.Fprintf(&b, "Line %d: the quick brown fox jumps over the lazy dog, again and again.\n", i)
	}
	pacingEditor(win).SetText(b.String())
	return win
}

func pacingEditor(win *Window) *TextView {
	return findWidget(win, func(w Widget) bool { _, ok := w.(*TextView); return ok }).(*TextView)
}

func pacingSidebar(win *Window) *ScrollView {
	return findWidget(win, func(w Widget) bool { _, ok := w.(*ScrollView); return ok }).(*ScrollView)
}

// scrollScenarios are the wheel steps, the fling and the rubber band in
// target (the sidebar or the editor of notes).
func scrollScenarios(name string, target func(*Window) Widget) []pacingScenario {
	fling := make([]driver.Point, 6)
	for i := range fling {
		fling[i] = driver.Point{Y: -12}
	}
	return []pacingScenario{
		{"notes", name + "-wheel", "Notes", pacingNotes, func(e *pacingEnv) {
			e.wheel(e.widget(target), -1, 6, 150*time.Millisecond)
			e.settle(2 * time.Second)
		}},
		{"notes", name + "-fling", "Notes", pacingNotes, func(e *pacingEnv) {
			e.swipe(e.widget(target), 10*time.Millisecond, fling...)
			e.settle(4 * time.Second)
		}},
		{"notes", name + "-rubberband", "Notes", pacingNotes, func(e *pacingEnv) {
			e.swipe(e.widget(target), 10*time.Millisecond, driver.Point{Y: 30}, driver.Point{Y: 30}, driver.Point{Y: 20})
			e.settle(2 * time.Second)
		}},
	}
}

var pacingScenarios = append(append(
	scrollScenarios("sidebar", func(w *Window) Widget { return pacingSidebar(w) }),
	scrollScenarios("editor", func(w *Window) Widget { return pacingEditor(w) })...),
	pacingScenario{"controls", "spinner", "Controls", exampleControls, func(e *pacingEnv) {
		time.Sleep(2 * time.Second)
	}},
	pacingScenario{"controls", "sheet", "Controls", exampleControls, func(e *pacingEnv) {
		e.ui(func() {
			Alert{Style: AlertWarning, Message: "Delete this item?", Buttons: []string{"Delete", "Cancel"}}.
				Show(e.win, nil)
		})
		e.settle(time.Second)
		e.ui(func() {
			if a := e.d.Alerts(); len(a) == 1 {
				a[0].Dismiss(1)
			} else {
				e.t.Errorf("%d alerts shown, want the sheet", len(a))
			}
		})
		e.settle(time.Second)
	}},
)

// TestFramePacingNative runs every pacing scenario on the OS's own shell.
func TestFramePacingNative(t *testing.T) {
	if os.Getenv("GOGUI_PACING") != "1" {
		t.Skip("set GOGUI_PACING=1 for the frame-pacing run (native.sh does)")
	}
	var missed, compared int
	for _, sc := range pacingScenarios {
		t.Run(sc.scene+"/"+sc.name, func(t *testing.T) {
			s := runPacing(t, sc)
			missed += s.Missed
			compared += s.Compared
		})
	}
	t.Logf("PACING-TOTAL missed=%d compared=%d (%.1f %%)", missed, compared, 100*float64(missed)/float64(max(compared, 1)))
	if tol := shell.PacingRunTolerance(compared); runtime.GOOS == "windows" && missed > tol {
		t.Errorf("%d vblanks missed over %d in all, tolerance %d", missed, compared, tol)
	}
}

// runPacing runs one scenario and returns its summary.
func runPacing(t *testing.T, sc pacingScenario) shell.PacingSummary {
	sh, err := native.New()
	if err != nil {
		t.Skip(err)
	}
	d := aqua.New(sh)
	setDriver(t, d)
	debugChecks = false // as in an app
	e := &pacingEnv{t: t, d: d, period: time.Second / 60}
	err = runTestNamed(t, sc.app, func(app *App) {
		e.app = app
		e.win = sc.build(t, app)
		e.aw = e.win.p.(*aqua.Window)
		e.base = d.Timeline().Running() // the controls bar loops
		clk := d.Clock()
		clk.SetFrameObserver(func(ft shell.FrameTiming) {
			if !e.on || ft.Time < 0 {
				return
			}
			goes := d.Timeline().Running() > 0
			if gap := ft.Time - e.prevFrame.Time; e.prevGoes && gap > 3*e.period/2 && len(e.skips) < 10 {
				p := e.prevFrame
				e.skips = append(e.skips, fmt.Sprintf("at %v, %.2f ms after the previous frame (its frame function %.2f ms, paint %.2f ms, submit %.2f ms, present %.2f ms)",
					ft.Time, ms(gap), ms(p.FrameFunc), ms(p.Paint), ms(p.Submit), ms(p.Present)))
			}
			e.prevFrame, e.prevGoes = ft, goes
			e.pacing.Frame(ft.Time, goes)
			if ft.Surfaces > 0 {
				e.work.add(ft)
			}
		})
		clk.SetPresentObserver(func(pt shell.PresentTiming) {
			if pt.Period > 0 {
				e.period = pt.Period
			}
			e.pacing.Present(pt)
		})
		go func() {
			defer Do(app.Quit)
			time.Sleep(500 * time.Millisecond) // shown, the first paints done
			e.ui(func() { e.on = true })
			sc.script(e)
			e.keepPresenting(150 * time.Millisecond)
		}()
	})
	if err != nil {
		t.Skip(err) // no display
	}
	s := e.pacing.Summary()
	per := time.Duration(0)
	if e.work.n > 0 {
		per = e.work.work / time.Duration(e.work.n)
	}
	t.Logf("PACING-RESULT %s/%s %v work-mean=%.2fms work-max=%.2fms", sc.scene, sc.name, s,
		float64(per)/1e6, float64(e.work.most)/1e6)
	for _, sk := range e.skips {
		t.Logf("PACING-SKIP %s/%s %s", sc.scene, sc.name, sk)
	}
	if s.Frames == 0 {
		t.Errorf("no frame counted")
	}
	if tol := shell.PacingTolerance(s.Compared); runtime.GOOS == "windows" {
		if s.Presents == 0 {
			t.Errorf("no present reported shown")
		}
		if s.Missed > tol {
			t.Errorf("%d vblanks missed over %d, tolerance %d", s.Missed, s.Compared, tol)
		}
	}
	return s
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
