package aqua

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// Reduce Motion and Reduce Transparency (PLAN.md S6.1f): the inventory of
// every animation of the aqua driver, and one test per entry. With Reduce
// Motion no intermediate frame of a motion is painted, and Reduce Motion
// turned on mid-animation ends it at its end state at once.

// motionPath says how an entry of the inventory follows Reduce Motion.
type motionPath string

const (
	// pathTimeline: an anim.Value on the driver's Timeline, which jumps
	// with Reduce Motion and finishes at its target when it is turned on.
	pathTimeline motionPath = "timeline"
	// pathOwn: a documented own path, named in the entry.
	pathOwn motionPath = "own"
	// pathNoMotion: a timer or ticker that is not motion and stays.
	pathNoMotion motionPath = "not motion"
)

// motionInventory lists every place in the aqua packages (internal/aqua/anim,
// the engine, aside) that starts an animation or a timer: a call of
// AnimateTo, AnimateAfter, SpringTo, SpringFrom, Decay, Loop, NewTicker or
// Shell.After, keyed "file:Func" (methods "file:Type.Method"). The test of
// each entry is named after the colon.
var motionInventory = map[string]struct {
	path motionPath
	what string
}{
	"focusring.go:FocusRing.Track":                {pathTimeline, "the focus ring's grow-in (TestReduceMotionFocusRing)"},
	"scroller.go:scroller.armFade":                {pathTimeline, "the overlay scroller's fade-out after its delay; the delay is no motion and stays (TestReduceMotionScrollerFade)"},
	"scroller.go:scroller.setHover":               {pathTimeline, "the overlay scroller's widening on hover (TestReduceMotionScrollerWiden)"},
	"scroller.go:scroller.HandleEvent":            {pathTimeline, "the overlay scroller's narrowing after a knob drag (TestReduceMotionScrollerWiden)"},
	"progress.go:progressView.animate":            {pathTimeline, "the indeterminate bar's band and the spinner's spokes, a loop; static looks with Reduce Motion (TestReduceMotionProgress)"},
	"scroll/axis.go:Axis.SetMax":                  {pathTimeline, "a wheel step retargeted to a new edge (TestReduceMotionScrollStep)"},
	"scroll/axis.go:Axis.ScrollTo":                {pathTimeline, "the animated ScrollTo (TestReduceMotionScrollStep)"},
	"scroll/axis.go:Axis.wheel":                   {pathTimeline, "the wheel step (TestReduceMotionScrollStep)"},
	"scroll/axis.go:Axis.release":                 {pathTimeline, "momentum after a release (TestReduceMotionMomentum)"},
	"scroll/axis.go:Axis.springTo":                {pathTimeline, "the rubber band's spring-back (TestReduceMotionRubberBand; the held drag and OS momentum: Axis.FollowReduceMotion)"},
	"sheet.go:Window.addSheet":                    {pathOwn, "the sheet's slide, on a Ticker: a fade in place with Reduce Motion, the panel in place at once when it is turned on (TestReduceMotionSheet)"},
	"menuinput.go:menu.pick":                      {pathOwn, "the picked item's blink over Shell.After: none with Reduce Motion, reported at once when it is turned on (TestReduceMotionMenuBlink)"},
	"menustrip.go:menuStrip.flash":                {pathOwn, "the accelerator's title flash over Shell.After: none with Reduce Motion, ended when it is turned on (TestReduceMotionStripFlash)"},
	"menuinput.go:menu.setArrow":                  {pathNoMotion, "the scroll arrows of a long menu follow the hover, a Ticker"},
	"textview_input.go:TextView.updateAutoScroll": {pathNoMotion, "the text view's auto-scroll during a drag follows the pointer, a Ticker"},
	"textview_layout.go:TextView.scheduleIdle":    {pathNoMotion, "the idle layout pass, a timer"},
	"textedit.go:textEdit.scheduleBlink":          {pathNoMotion, "the caret blink, the OS's own setting"},
	"control.go:clickControl.flash":               {pathNoMotion, "the key flash of a button, checkbox or radio, a pressed look for 100 ms"},
	"tooltip.go:tooltips.arm":                     {pathNoMotion, "the tooltip's delay; it shows and hides without a fade"},
	"tooltip.go:tooltips.hide":                    {pathNoMotion, "the tooltip's grace period"},
}

// motionCalls are the method names that start an animation or a timer.
var motionCalls = map[string]int{ // name → its argument count
	"AnimateTo": 3, "AnimateAfter": 4, "SpringTo": 2, "SpringFrom": 3,
	"Decay": 3, "Loop": 1, "NewTicker": 1, "After": 2,
}

// TestMotionInventory: every call that starts an animation or a timer in
// the aqua packages is in motionInventory, and every entry still has one,
// so a new animation cannot bypass both the timeline and a documented own
// path.
func TestMotionInventory(t *testing.T) {
	found, err := scanCalls(".", motionCalls, func(path string) bool { return path == "anim" })
	if err != nil {
		t.Fatal(err)
	}
	checkInventory(t, found, motionInventory, "animations or timers not in motionInventory (add each with its Reduce Motion path and a test)", "motionInventory entries with no call left")
}

// scanCalls parses the non-test Go files under root (build tags ignored,
// testdata and the directories skip reports skipped) and returns the
// functions that call a method or function named in calls with its
// argument count, keyed "file:Func" (methods "file:Type.Method"), file
// relative to root with forward slashes.
func scanCalls(root string, calls map[string]int, skip func(path string) bool) (map[string]bool, error) {
	found := map[string]bool{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && (skip(rel) || d.Name() == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			name := fn.Name.Name
			if fn.Recv != nil && len(fn.Recv.List) == 1 {
				rt := fn.Recv.List[0].Type
				if s, ok := rt.(*ast.StarExpr); ok {
					rt = s.X
				}
				if id, ok := rt.(*ast.Ident); ok {
					name = id.Name + "." + name
				}
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				var callee string
				switch f := call.Fun.(type) {
				case *ast.SelectorExpr:
					callee = f.Sel.Name
				case *ast.Ident:
					callee = f.Name
				}
				if args, ok := calls[callee]; ok && len(call.Args) == args {
					found[rel+":"+name] = true
				}
				return true
			})
		}
		return nil
	})
	return found, err
}

// checkInventory checks that the keys of found and inventory are the same.
func checkInventory[V any](t *testing.T, found map[string]bool, inventory map[string]V, missingMsg, staleMsg string) {
	t.Helper()
	var missing, stale []string
	for k := range found {
		if _, ok := inventory[k]; !ok {
			missing = append(missing, k)
		}
	}
	for k := range inventory {
		if !found[k] {
			stale = append(stale, k)
		}
	}
	slices.Sort(missing)
	slices.Sort(stale)
	if len(missing) > 0 {
		t.Errorf("%s:\n%s", missingMsg, strings.Join(missing, "\n"))
	}
	if len(stale) > 0 {
		t.Errorf("%s:\n%s", staleMsg, strings.Join(stale, "\n"))
	}
}

// withMotion returns a with Reduce Motion set to on.
func withMotion(a driver.Appearance, on bool) driver.Appearance {
	a.ReduceMotion = on
	return a
}

// noBlinkAppearance is the default appearance without a caret blink, whose
// repaints would keep the frame clock going.
func noBlinkAppearance() driver.Appearance {
	a := driver.DefaultAppearance()
	a.CaretBlink = driver.CaretBlink{}
	return a
}

// TestReduceMotionFocusRing: with Reduce Motion the ring is whole at the
// focus, no frame runs for it; turned on mid-ring, it is whole at once.
func TestReduceMotionFocusRing(t *testing.T) {
	for _, live := range []bool{false, true} {
		sheetRun(t, withMotion(noBlinkAppearance(), !live), func(e *sheetEnv) {
			ring := &e.field.view.ring
			e.w.Router().SetFocus(nil)
			e.clk.Advance(time.Second)
			e.w.Router().SetFocus(e.field.view)
			if live {
				e.clk.Advance(3 * fake.FrameInterval)
				if p := ring.Progress(); p <= 0 || p >= 1 {
					t.Fatalf("mid-ring progress %v", p)
				}
				e.sh.SetAppearance(withMotion(noBlinkAppearance(), true))
			}
			if ring.Progress() != 1 || ring.v.Running() {
				t.Fatalf("live %v: progress %v, running %v", live, ring.Progress(), ring.v.Running())
			}
			if n := e.clk.Advance(time.Second); n > 1 || e.clk.Requested() {
				t.Errorf("live %v: %d frames after the ring", live, n)
			}
		})
	}
}

// TestReduceMotionSheet: with Reduce Motion the panel is in place on every
// frame (it fades); turned on mid-slide, it is in place at once and the
// rest fades in.
func TestReduceMotionSheet(t *testing.T) {
	top := tokens.Chrome.TitleBarHeight
	for _, live := range []bool{false, true} {
		sheetRun(t, withMotion(driver.DefaultAppearance(), !live), func(e *sheetEnv) {
			e.show(oneButtonAlert)
			s := e.w.shownSheet()
			if live {
				e.clk.Advance(80 * time.Millisecond)
				if e.panelTop() >= top {
					t.Fatalf("mid-slide: panel top %v, not sliding", e.panelTop())
				}
				e.sh.SetAppearance(withMotion(driver.DefaultAppearance(), true))
				if e.panelTop() != top {
					t.Fatalf("turned on mid-slide: panel top %v, want %v", e.panelTop(), top)
				}
			}
			for i := 0; s.ticker.Running(); i++ {
				if i > 100 {
					t.Fatal("the fade never ends")
				}
				e.clk.Advance(fake.FrameInterval)
				if e.panelTop() != top {
					t.Fatalf("live %v: panel top %v at progress %v", live, e.panelTop(), s.p)
				}
			}
			if s.p != 1 || strings.Contains(e.sf.Dump(), "pushOpacity") {
				t.Errorf("live %v: shown at %v, or still fading", live, s.p)
			}
		})
	}
}

// TestReduceMotionMenuBlink: turned on during a pick's blink, the pick is
// reported at once, the item highlighted, no timer left (with Reduce
// Motion from the start: TestMenuPickReduceMotion).
func TestReduceMotionMenuBlink(t *testing.T) {
	runMenu(t, sampleMenuItems, func(e *menuEnv) {
		e.open(0, menuOpening{})
		e.sf.Key(driver.KeyDown, 0)
		e.sf.Key(driver.KeyReturn, 0)
		if !e.m.Picking() || len(e.s.Timers()) == 0 {
			t.Fatal("no blink")
		}
		e.wantLog("blinking")
		e.s.SetAppearance(withMotion(e.s.Appearance(), true))
		e.wantLog("turned on mid-blink", "pick 0")
		if e.m.Picking() || e.m.Tracking() || len(e.s.Timers()) != 0 {
			t.Errorf("picking %v, tracking %v, timers %v", e.m.Picking(), e.m.Tracking(), e.s.Timers())
		}
		e.finishBlink()
		e.wantLog("after the blink's time")
	})
}

// TestReduceMotionStripFlash: turned on during an accelerator's title
// flash, the flash ends at once (with Reduce Motion from the start:
// TestAccelFlash).
func TestReduceMotionStripFlash(t *testing.T) {
	e := newAccelEnv(t)
	w, sf, _ := e.window(allStyles, nil)
	st := w.menuStrip()
	sf.Key('-', driver.ModControl)
	if !st.Flashing() || st.Highlighted() != viewMenu {
		t.Fatalf("flashing %v, highlighted %d", st.Flashing(), st.Highlighted())
	}
	w.SetAppearance(withMotion(w.Appearance(), true))
	if st.Flashing() || st.Highlighted() != -1 || len(e.s.Timers()) != 0 {
		t.Errorf("turned on: flashing %v, highlighted %d, timers %v", st.Flashing(), st.Highlighted(), e.s.Timers())
	}
	// Turned off again, the next accelerator flashes.
	w.SetAppearance(withMotion(w.Appearance(), false))
	sf.Key('-', driver.ModControl)
	if !st.Flashing() {
		t.Error("no flash with Reduce Motion off again")
	}
}

// TestReduceMotionProgress: with Reduce Motion from the start an
// indeterminate indicator paints its static look and requests no frames
// (turned on mid-loop: TestProgressReduceMotion).
func TestReduceMotionProgress(t *testing.T) {
	indetRun(t, driver.Rect{X: 10, Y: 10, W: 120, H: 6}, func(e *indetEnv) {
		e.sh.SetAppearance(withMotion(e.w.Appearance(), true))
		e.p.SetIndeterminate(true)
		e.w.Show()
		e.settle(t, "Reduce Motion from the start")
		if got := controlLines(e.sf.Dump()); !strings.Contains(got, "fillRRect (10,38,120,6) r=3 solid #007aff80") {
			t.Fatalf("static bar:\n%s", got)
		}
	})
}

// TestReduceMotionScrollerFade: turned on mid-fade, the overlay scroller
// is hidden at once (mid-delay and from the start: TestScrollerReduceMotion).
func TestReduceMotionScrollerFade(t *testing.T) {
	scrollRun(t, driver.DefaultAppearance(), func(e *scrollEnv) {
		b := e.vbar()
		e.sf.Wheel(e.at(50, 50), pt(0, -1))
		t0 := e.fadeSettle(t)
		e.fadeAdvanceTo(t, t0+scrFadeDelay()+scrFadeDur()/2)
		if a := b.alpha.Value(); a <= 0 || a >= 1 {
			t.Fatalf("mid-fade opacity %v", a)
		}
		e.sh.SetAppearance(withMotion(driver.DefaultAppearance(), true))
		knobHidden(t, b, "turned on mid-fade")
		if e.clk.Advance(time.Second) > 1 || e.clk.Requested() {
			t.Error("frames after the fade")
		}
	})
}

// TestReduceMotionScrollerWiden: with Reduce Motion the hovered scroller
// is wide at once; turned on mid-widening, it is wide at once.
func TestReduceMotionScrollerWiden(t *testing.T) {
	for _, live := range []bool{false, true} {
		scrollRun(t, withMotion(driver.DefaultAppearance(), !live), func(e *scrollEnv) {
			b := e.vbar()
			e.sf.Wheel(e.at(50, 50), pt(0, -1))
			e.fadeSettle(t)
			e.sf.Move(e.at(e.frame.W-3, 50))
			if live {
				e.clk.Advance(fake.FrameInterval)
				if x := b.wide.Value(); x <= 0 || x >= 1 {
					t.Fatalf("mid-widening %v", x)
				}
				e.sh.SetAppearance(withMotion(driver.DefaultAppearance(), true))
			}
			if b.wide.Value() != 1 || b.wide.Running() {
				t.Errorf("live %v: wide %v, running %v", live, b.wide.Value(), b.wide.Running())
			}
		})
	}
}

// TestReduceMotionScrollStep: with Reduce Motion a wheel step and an
// animated scroll jump; turned on mid-step, the offset is at its target at
// once.
func TestReduceMotionScrollStep(t *testing.T) {
	for _, live := range []bool{false, true} {
		scrollRun(t, withMotion(driver.DefaultAppearance(), !live), func(e *scrollEnv) {
			e.sf.Wheel(e.at(50, 50), pt(0, -2))
			want := e.sv.y.Target()
			if live {
				e.clk.Advance(fake.FrameInterval)
				if y := e.sv.y.Offset(); y <= 0 || y >= want {
					t.Fatalf("mid-step offset %v (target %v)", y, want)
				}
				e.sh.SetAppearance(withMotion(driver.DefaultAppearance(), true))
			}
			if e.sv.y.Offset() != want || e.sv.y.Moving() {
				t.Errorf("live %v: wheel offset %v, want %v", live, e.sv.y.Offset(), want)
			}
			e.sv.y.ScrollTo(100, true)
			if e.sv.y.Offset() != 100 || e.sv.y.Moving() {
				t.Errorf("live %v: animated ScrollTo at %v", live, e.sv.y.Offset())
			}
		})
	}
}

// fling sends a fast swipe down the document over the scroll view, timed
// like the Win32 shell's.
func (e *scrollEnv) fling(dy float64, n int) {
	ds := make([]driver.Point, n)
	for i := range ds {
		ds[i] = pt(0, dy)
	}
	e.sf.TimedSwipe(e.at(50, 50), 10*time.Millisecond, ds...)
}

// TestReduceMotionMomentum: with Reduce Motion a release stops; turned on
// mid-momentum, the content is at the momentum's rest (clamped) at once.
func TestReduceMotionMomentum(t *testing.T) {
	for _, live := range []bool{false, true} {
		scrollRun(t, withMotion(driver.DefaultAppearance(), !live), func(e *scrollEnv) {
			e.fling(-8, 6)
			y := e.sv.y.Offset()
			if !live {
				if e.sv.y.Moving() || y != 48 {
					t.Errorf("released at %v, moving %v; want 48 at rest", y, e.sv.y.Moving())
				}
				return
			}
			if !e.sv.y.Moving() {
				t.Fatal("no momentum")
			}
			rest := e.sv.y.Clamped()
			if tg := e.sv.y.Target(); tg > rest {
				rest = min(tg, e.sv.y.Max())
			}
			e.sh.SetAppearance(withMotion(driver.DefaultAppearance(), true))
			if e.sv.y.Moving() || e.sv.y.Offset() != rest {
				t.Errorf("turned on mid-momentum: at %v, moving %v; want %v at rest", e.sv.y.Offset(), e.sv.y.Moving(), rest)
			}
		})
	}
}

// TestReduceMotionRubberBand: turned on while a drag holds the content past
// the edge, it is at the edge at once and the drag goes on without a rubber
// band; turned on during OS momentum, the momentum ends and moves nothing
// more (the spring-back: scroll's TestAxisReduceMotion).
func TestReduceMotionRubberBand(t *testing.T) {
	scrollRun(t, driver.DefaultAppearance(), func(e *scrollEnv) {
		e.fadeHold(20) // content pulled down past the top
		if e.sv.y.Offset() >= 0 {
			t.Fatalf("no overscroll: %v", e.sv.y.Offset())
		}
		e.sh.SetAppearance(withMotion(driver.DefaultAppearance(), true))
		if e.sv.y.Offset() != 0 {
			t.Fatalf("turned on mid-drag: offset %v", e.sv.y.Offset())
		}
		e.sf.Send(shell.ScrollEvent{Pos: e.at(50, 50), Delta: pt(0, -10), Precise: true, Phase: shell.PhaseChanged})
		if e.sv.y.Offset() != 10 {
			t.Errorf("the drag after: offset %v, want 10", e.sv.y.Offset())
		}
		e.sf.Send(shell.ScrollEvent{Pos: e.at(50, 50), Precise: true, Phase: shell.PhaseEnded})
		e.sf.Momentum(pt(0, -10), pt(0, -10))
		if e.sv.y.Offset() != 10 || e.sv.y.Moving() {
			t.Errorf("OS momentum with Reduce Motion: offset %v, moving %v", e.sv.y.Offset(), e.sv.y.Moving())
		}
	})
	scrollRun(t, driver.DefaultAppearance(), func(e *scrollEnv) {
		e.sf.Swipe(e.at(50, 50), pt(0, -10))
		e.sf.Send(shell.ScrollEvent{Pos: e.at(50, 50), Delta: pt(0, -10), Precise: true, Momentum: shell.PhaseBegan})
		if !e.sv.y.Moving() || e.sv.y.Offset() != 20 {
			t.Fatalf("OS momentum: offset %v, moving %v", e.sv.y.Offset(), e.sv.y.Moving())
		}
		e.sh.SetAppearance(withMotion(driver.DefaultAppearance(), true))
		e.sf.Send(shell.ScrollEvent{Pos: e.at(50, 50), Delta: pt(0, -10), Precise: true, Momentum: shell.PhaseChanged})
		if e.sv.y.Moving() || e.sv.y.Offset() != 20 {
			t.Errorf("turned on during OS momentum: offset %v, moving %v", e.sv.y.Offset(), e.sv.y.Moving())
		}
	})
}

// TestReduceTransparencyMenu: with Reduce Transparency a menu paints its
// opaque fill even in a popup the shell blurs behind, and follows the
// setting switched live.
func TestReduceTransparencyMenu(t *testing.T) {
	runMenu(t, sampleMenuItems, func(e *menuEnv) {
		e.s.SetPopupTranslucent(true)
		e.open(0, menuOpening{})
		a := e.s.Appearance()
		wantFill := func(what string, translucent bool) {
			t.Helper()
			tok := tokens.Menu.OpaqueBackground
			if translucent {
				tok = tokens.Menu.Background
			}
			if got := popupDump(t, e.s, e.p); !strings.Contains(got, "solid "+hexColor(tok.Value(a))) || e.m.translucent() != translucent {
				t.Errorf("%s: want translucent %v:\n%s", what, translucent, got)
			}
		}
		wantFill("normal", true)
		a.ReduceTransparency = true
		e.s.SetAppearance(a)
		wantFill("Reduce Transparency turned on", false)
		a.ReduceTransparency = false
		e.s.SetAppearance(a)
		wantFill("turned off", true)
	})
}
