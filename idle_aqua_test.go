package gogui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua"
	"github.com/radix29/gogui/internal/driver"
	fakeshell "github.com/radix29/gogui/internal/shell/fake"
)

// The idle audit of the examples (PLAN.md S6.2a): each example on the aqua
// driver over the fake shell is driven into every state a user can leave
// a window in, then left alone for idleWait; it must then request no
// frame, arm no timer and run no animation, except the sources of
// internal/aqua's idleInventory that run by design: the indeterminate
// indicators' loops (the controls example's busy bar). The caret blink
// ends by its timeout within idleWait.

// idleWait is how long a window is left alone before it must be idle: the
// default caret blink's timeout (driver.DefaultCaretBlink, 10 s), and a
// second for the repaint of the solid caret at its end.
const idleWait = 11 * time.Second

// idleEnv drives one example's window for the idle audit.
type idleEnv struct {
	t     *testing.T
	app   *App
	sh    *fakeshell.Shell
	d     *aqua.Driver
	win   *Window
	aw    *aqua.Window
	sf    *fakeshell.Surface
	loops int // the indeterminate indicators shown, which loop by design
}

func newIdleEnv(t *testing.T, app *App, sh *fakeshell.Shell, win *Window) *idleEnv {
	aw := win.p.(*aqua.Window)
	return &idleEnv{t: t, app: app, sh: sh, d: app.drv.(*aqua.Driver), win: win, aw: aw,
		sf: aw.Surface().(*fakeshell.Surface)}
}

// at returns the centre of w in its window's surface points.
func (e *idleEnv) at(w Widget) driver.Point {
	el := aqua.ElementOf(w.peerWidget())
	f := el.(interface{ Frame() driver.Rect }).Frame()
	return aqua.ToRoot(el, driver.Point{X: f.W / 2, Y: f.H / 2})
}

// idle leaves the app alone for idleWait, then checks that it is idle in
// state: no timer armed, no animation running but the loops, and without
// loops no frame requested and none run in the next second.
func (e *idleEnv) idle(state string) {
	e.t.Helper()
	clk := e.sh.FakeClock()
	clk.Advance(idleWait)
	drain(e.app)
	var why []string
	if ts := e.sh.Timers(); len(ts) > 0 {
		why = append(why, fmt.Sprintf("%d timers armed (due at %v, now %v)", len(ts), ts, clk.Now()))
	}
	if n := e.d.Timeline().Running(); n != e.loops {
		why = append(why, fmt.Sprintf("%d animations running, want %d", n, e.loops))
	}
	if e.loops == 0 {
		if clk.Requested() {
			why = append(why, "a frame requested")
		}
		if n := clk.Advance(time.Second); n > 0 {
			why = append(why, fmt.Sprintf("%d frames in the next second", n))
		}
	}
	if len(why) > 0 {
		e.t.Errorf("%s: not idle %v after: %s", state, idleWait, strings.Join(why, "; "))
	}
}

// key sends a key to the key surface (a sheet's window, an app-modal
// alert's surface) and drains the queue.
func (e *idleEnv) key(k driver.Key, mods driver.Modifiers) {
	e.t.Helper()
	sf := e.sh.Key()
	if sf == nil {
		e.t.Fatalf("key %q: no key surface", k)
	}
	sf.Key(k, mods)
	drain(e.app)
}

// common drives the states every example window has: hovered, clicked,
// resized (if it can be), appearance changes, inactive, minimized and
// restored, and an app-modal alert.
func (e *idleEnv) common(hover Widget) {
	e.t.Helper()
	e.sf.Move(e.at(hover))
	e.idle("hovered")
	e.sf.Click(e.at(hover))
	drain(e.app)
	e.idle("after a click")
	e.sf.Move(driver.Point{X: 2, Y: e.sf.Size().H - 2})
	if e.sf.Style()&driver.StyleResizable != 0 {
		sz := e.sf.Size()
		e.sf.UserResize(driver.Size{W: sz.W + 40, H: sz.H + 30})
		drain(e.app)
		e.idle("after a resize")
	}
	e.app.SetAppearance(AppearanceDark)
	drain(e.app)
	e.idle("after an appearance change")
	e.app.SetAppearance(AppearanceSystem)
	drain(e.app)
	e.sh.Deactivate()
	drain(e.app)
	e.idle("inactive")
	if e.sf.Style()&driver.StyleMiniaturizable != 0 {
		e.sf.Minimize()
		drain(e.app)
		e.idle("minimized")
	}
	e.sf.Activate()
	drain(e.app)
	e.idle("restored")
	Alert{Message: "Idle"}.Show(nil, nil)
	drain(e.app)
	e.idle("an app-modal alert")
	e.key(driver.KeyReturn, 0)
	if a := e.d.Alerts(); len(a) != 0 {
		e.t.Fatalf("the app-modal alert is still shown: %+v", a)
	}
	e.idle("after the app-modal alert")
}

// tooltip hovers w, which has a tooltip, until it shows, then leaves it.
func (e *idleEnv) tooltip(w Widget) {
	e.t.Helper()
	e.sf.Move(e.at(w))
	e.idle("a tooltip shown")
	if len(e.sf.Popups()) == 0 {
		e.t.Fatal("no tooltip shown")
	}
	e.sf.Move(driver.Point{X: 2, Y: e.sf.Size().H - 2})
	e.idle("after a tooltip")
}

// TestIdleHelloOnAqua: the hello example is idle in every state.
func TestIdleHelloOnAqua(t *testing.T) {
	runAqua(t, func(app *App, sh *fakeshell.Shell) {
		win := exampleHello(t, app)
		e := newIdleEnv(t, app, sh, win)
		e.idle("just shown")
		name := findWidget(win, func(w Widget) bool { _, ok := w.(*TextField); return ok })
		greet := findWidget(win, func(w Widget) bool { _, ok := w.(*Button); return ok }).(*Button)
		e.sf.Click(e.at(name))
		e.sf.Type("Jane")
		drain(app)
		e.idle("a focused field")
		// An inactive window shows no caret: its blink stops within a half
		// period, not at its timeout.
		e.sf.Type("!")
		sh.Deactivate()
		sh.FakeClock().Advance(driver.DefaultCaretBlink.Period)
		if ts := sh.Timers(); len(ts) > 0 {
			t.Errorf("inactive with a focused field: %d timers after a blink period", len(ts))
		}
		e.idle("inactive with a focused field")
		e.sf.Activate()
		drain(app)
		e.key(driver.KeyReturn, 0) // the default button
		e.idle("after the default button's key flash")
		greet.SetTooltip("Say hello")
		e.tooltip(greet)
		e.common(greet)
	})
}

// TestIdleFormOnAqua: the form example is idle in every state, its
// validation sheet and its pop-up menu included.
func TestIdleFormOnAqua(t *testing.T) {
	runAquaNamed(t, "Account", func(app *App, sh *fakeshell.Shell) {
		f := newFormExample(app)
		showExample(app, f.win)
		e := newIdleEnv(t, app, sh, f.win)
		e.idle("just shown, the name field focused")
		e.sf.Type("Jane")
		drain(app)
		e.idle("a focused field")
		e.sf.Click(e.at(f.role))
		drain(app)
		if sh.GrabPopup() == nil {
			t.Fatal("no pop-up menu")
		}
		e.idle("a pop-up menu")
		e.key(driver.KeyEscape, 0)
		e.idle("after a pop-up menu")
		e.sf.Click(e.at(f.terms))
		e.sf.Click(e.at(f.create))
		drain(app)
		if len(e.d.Alerts()) != 1 || e.aw.SheetRoot() == nil {
			t.Fatal("no validation sheet")
		}
		e.idle("a sheet")
		e.key(driver.KeyReturn, 0)
		e.idle("after a sheet")
		e.common(f.newsletter)
	})
}

// TestIdleConverterOnAqua: the converter example is idle in every state,
// a slider drag included.
func TestIdleConverterOnAqua(t *testing.T) {
	runAqua(t, func(app *App, sh *fakeshell.Shell) {
		win := exampleConverter(t, app)
		e := newIdleEnv(t, app, sh, win)
		e.idle("just shown")
		field := findWidget(win, func(w Widget) bool { _, ok := w.(*TextField); return ok })
		slider := findWidget(win, func(w Widget) bool { _, ok := w.(*Slider); return ok })
		e.sf.Click(e.at(field))
		e.sf.Type("21")
		drain(app)
		e.idle("a focused field")
		p := e.at(slider)
		e.sf.Drag(p, p.Add(driver.Point{X: 40}), p.Add(driver.Point{X: 20}))
		drain(app)
		e.idle("after a slider drag")
		e.common(slider)
	})
}

// TestIdleControlsOnAqua: the controls example is idle in every state but
// for its indeterminate bar's loop, which runs by design: shown, the loop
// is the one animation; made determinate, the window is idle. The other
// states run with the bar determinate, so their check is strict (and 11 s
// of looping frames per state would cost seconds).
func TestIdleControlsOnAqua(t *testing.T) {
	runAqua(t, func(app *App, sh *fakeshell.Shell) {
		c := newControlsExample(app)
		showExample(app, c.win)
		e := newIdleEnv(t, app, sh, c.win)
		e.loops = 1
		e.idle("just shown, the bar looping")
		c.busy.SetIndeterminate(false)
		drain(app)
		e.loops = 0
		e.idle("just shown, the bar determinate")
		e.sf.Click(e.at(c.field))
		e.sf.Type("text")
		drain(app)
		e.idle("a focused field")
		e.sf.Click(e.at(c.popup))
		drain(app)
		e.idle("a pop-up menu")
		e.key(driver.KeyEscape, 0)
		e.idle("after a pop-up menu")
		openMenu(t, sh, c.win, "View")
		e.idle("an open menu")
		e.key(driver.KeyEscape, 0)
		e.idle("after an open menu")
		openMenu(t, sh, c.win, "View")
		pickItem(t, app, sh, c.win, "Green Accent")
		e.idle("after a menu pick")
		openMenu(t, sh, c.win, "View")
		pickItem(t, app, sh, c.win, "System Accent")
		e.sf.Click(e.at(c.alert))
		drain(app)
		if e.aw.SheetRoot() == nil {
			t.Fatal("no sheet")
		}
		e.idle("a sheet")
		e.key(driver.KeyEscape, 0)
		e.idle("after a sheet")
		e.tooltip(c.disabled)
		e.common(c.check)
		c.busy.SetIndeterminate(true)
		drain(app)
		e.loops = 1
		e.idle("the bar looping again")
		// Minimized with the bar looping (S6.2b): the loop keeps running,
		// its frames at the low rate (4 Hz); restored, at 60 Hz at once.
		clk := sh.FakeClock()
		e.sf.Minimize()
		drain(app)
		if n := clk.Advance(time.Second); n < 3 || n > 5 || e.d.Timeline().Running() != 1 {
			t.Errorf("minimized, the bar looping: %d frames in 1 s and %d animations, want 4 and 1",
				n, e.d.Timeline().Running())
		}
		e.sf.Activate()
		drain(app)
		if n := clk.Advance(time.Second); n < 59 {
			t.Errorf("restored, the bar looping: %d frames in 1 s, want 60", n)
		}
	})
}

// TestIdleNotesOnAqua: the notes example is idle in every state: a focused
// text view, a wheel step, a momentum fling and a rubber band in the
// sidebar, an open menu and the About panel included.
func TestIdleNotesOnAqua(t *testing.T) {
	runAquaNamed(t, "Notes", func(app *App, sh *fakeshell.Shell) {
		win := exampleNotes(t, app)
		e := newIdleEnv(t, app, sh, win)
		e.idle("just shown, the text view focused")
		e.sf.Type("Some notes.")
		e.key(driver.KeyReturn, 0)
		e.idle("a focused text view")
		sidebar := findWidget(win, func(w Widget) bool { _, ok := w.(*ScrollView); return ok }).(*ScrollView)
		in := e.at(sidebar)
		e.sf.Wheel(in, driver.Point{Y: -2})
		if e.d.Timeline().Running() == 0 {
			t.Fatal("the wheel step does not animate")
		}
		e.idle("after a wheel step")
		fling := make([]driver.Point, 6)
		for i := range fling {
			fling[i] = driver.Point{Y: -8}
		}
		y := sidebar.Offset().Y
		e.sf.TimedSwipe(in, 10*time.Millisecond, fling...)
		if e.d.Timeline().Running() == 0 {
			t.Fatal("no momentum after the fling")
		}
		e.idle("after a momentum fling")
		if sidebar.Offset().Y <= y {
			t.Fatalf("the fling did not scroll: %v, was %v", sidebar.Offset().Y, y)
		}
		// Back at the top, a swipe past it: the rubber band, then its
		// spring-back.
		sidebar.SetOffset(Point{})
		e.sf.TimedSwipe(in, 10*time.Millisecond, driver.Point{Y: 30}, driver.Point{Y: 30})
		if e.d.Timeline().Running() == 0 {
			t.Fatal("no spring-back after the rubber band")
		}
		e.idle("after a rubber band")
		if sidebar.Offset().Y != 0 {
			t.Fatalf("the rubber band did not spring back: %v", sidebar.Offset())
		}
		openMenu(t, sh, win, "Format")
		e.idle("an open menu")
		e.key(driver.KeyEscape, 0)
		e.idle("after an open menu")
		openMenu(t, sh, win, "Notes")
		pickItem(t, app, sh, win, "About Notes")
		if len(e.d.Alerts()) != 1 {
			t.Fatalf("no About panel: %+v", e.d.Alerts())
		}
		e.idle("the About panel")
		e.key(driver.KeyReturn, 0)
		e.idle("after the About panel")
		editor := findWidget(win, func(w Widget) bool { _, ok := w.(*TextView); return ok })
		e.common(editor)
	})
}
