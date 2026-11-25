package aqua

import (
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"
)

// TestAppAlertFocusRingAppearance (R1): the focus ring of a button in an
// app-modal alert (no window) paints in the driver's appearance, as the
// button does, not in the default one.
func TestAppAlertFocusRingAppearance(t *testing.T) {
	sys := driver.DefaultAppearance()
	sys.Accent = driver.Color{R: 1, G: 0, B: 0, A: 1}
	sheetRun(t, sys, func(e *sheetEnv) {
		e.showApp(twoButtonAlert)
		e.settle()
		dump := e.modal().Dump()
		if !strings.Contains(dump, "w=3 solid #ff000080") {
			t.Errorf("no ring in the driver's accent")
		}
		if strings.Contains(dump, "#007aff") {
			t.Errorf("ring in the default accent")
		}
	})
}

// TestAlertBroadcastGuards (R3): the WindowStateEvent broadcast of an
// app-modal alert skips an element a handler moved out of the tree and
// stops once a handler closed the surface, as a window's does.
func TestAlertBroadcastGuards(t *testing.T) {
	for _, closeIt := range []bool{false, true} {
		sheetRun(t, driver.DefaultAppearance(), func(e *sheetEnv) {
			e.showApp(twoButtonAlert)
			e.settle()
			root := e.d.Alerts()[0].Root().(*alertRoot)
			var log []string
			first := newProbe(&log, "first", 0, 0, 1, 1)
			second := newProbe(&log, "second", 0, 0, 1, 1)
			root.AddChild(first)
			root.AddChild(second)
			first.onEvent = func(ev Event) {
				if _, ok := ev.(WindowStateEvent); !ok {
					return
				}
				if closeIt {
					root.surf.close()
				} else {
					root.RemoveChild(second)
				}
			}
			dark := driver.DefaultAppearance()
			dark.Dark = true
			e.sh.SetAppearance(dark)
			e.settle()
			if got := strings.Join(log, "; "); got != "first state key=true dark=true" {
				t.Errorf("close %v: log %q", closeIt, got)
			}
		})
	}
}
