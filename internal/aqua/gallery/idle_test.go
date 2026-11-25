package gallery_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua"
	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/aqua/gallery"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell/fake"
)

// idleWait is how long the gallery is left alone before it must be idle:
// the default caret blink's timeout and a second (as gogui's idle audit).
var idleWait = driver.DefaultCaretBlink.Timeout + time.Second

// TestIdleGallery: the gallery scene, with motion and overlay scrollers
// (not as the golden renders), is idle in every state a user can leave
// it in (PLAN.md S6.2a): its two indeterminate indicators loop by design
// and are the only animations when shown; made determinate, the scene
// requests no frame and arms no timer after idleWait, also after a hover,
// a click, a wheel step (the overlay scroller's fade), a resize, an
// appearance change, inactive and minimized.
func TestIdleGallery(t *testing.T) {
	s := fake.New()
	ad := aqua.New(s)
	d := &recDriver{Driver: ad}
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-stop:
		case <-time.After(10 * time.Second):
			panic("gallery: Driver.Run did not return")
		}
	}()
	err := ad.Run(driver.AppConfig{Name: "Gallery"}, app{ready: func() {
		defer ad.Quit()
		win, err := gallery.Open(d, winHandler{})
		if err != nil {
			t.Error(err)
			return
		}
		w := win.(*aqua.Window)
		sf := w.Surface().(*fake.Surface)
		clk := s.FakeClock()
		loops := 2
		idle := func(state string) {
			t.Helper()
			clk.Advance(idleWait)
			for ad.Queue().Drain() > 0 {
			}
			var why []string
			if ts := s.Timers(); len(ts) > 0 {
				why = append(why, fmt.Sprintf("%d timers armed (due at %v, now %v)", len(ts), ts, clk.Now()))
			}
			if n := ad.Timeline().Running(); n != loops {
				why = append(why, fmt.Sprintf("%d animations running, want %d", n, loops))
			}
			if loops == 0 {
				if clk.Requested() {
					why = append(why, "a frame requested")
				}
				if n := clk.Advance(time.Second); n > 0 {
					why = append(why, fmt.Sprintf("%d frames in the next second", n))
				}
			}
			if len(why) > 0 {
				t.Errorf("%s: not idle %v after: %s", state, idleWait, strings.Join(why, "; "))
			}
		}
		at := func(role a11y.Role, title string) driver.Point {
			t.Helper()
			n := aqua.AccessibilityTree(w.Root()).Find(role, title)
			if n == nil {
				t.Fatalf("no %v %q", role, title)
			}
			f := n.Frame
			return driver.Point{X: f.X + f.W/2, Y: f.Y + f.H/2}
		}
		var indet []*aqua.Progress // the scene's indeterminate indicators
		for _, wd := range d.widgets {
			if p, ok := wd.(*aqua.Progress); ok && p.Indeterminate {
				indet = append(indet, p)
			}
		}
		if len(indet) != 2 {
			t.Fatalf("%d indeterminate indicators, want 2", len(indet))
		}
		setIndeterminate := func(on bool) {
			for _, p := range indet {
				p.SetIndeterminate(on)
			}
		}

		idle("just shown, the indicators looping")
		setIndeterminate(false)
		loops = 0
		idle("just shown, the indicators determinate")
		push := at(a11y.RoleButton, "Push Button")
		sf.Move(push)
		idle("hovered")
		sf.Click(push)
		idle("after a click")
		sf.Click(at(a11y.RoleCheckBox, "Checkbox"))
		idle("after a checkbox click")
		sf.Wheel(at(a11y.RoleScrollArea, ""), driver.Point{Y: -1})
		if ad.Timeline().Running() == 0 {
			t.Fatal("the wheel step does not animate")
		}
		idle("after a wheel step and the scroller's fade")
		sf.Move(driver.Point{X: 2, Y: gallery.Size.H})
		sf.UserResize(driver.Size{W: gallery.Size.W + 40, H: gallery.Size.H + 30})
		idle("after a resize")
		ad.SetAppearance(driver.AppearanceDark)
		idle("after an appearance change")
		s.Deactivate()
		idle("inactive")
		sf.Minimize()
		idle("minimized")
		sf.Activate()
		idle("restored")
		setIndeterminate(true)
		loops = 2
		idle("the indicators looping again")
	}})
	if err != nil {
		t.Fatal(err)
	}
}
