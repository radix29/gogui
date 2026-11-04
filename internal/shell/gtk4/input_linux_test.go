//go:build linux

package gtk4

import (
	"errors"
	"fmt"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// TestSurfaceGTKPointer: the GGSurface carries the pointer controllers, a
// GtkEventControllerMotion and a GtkGestureClick for every button, with
// their handlers connected; their signals, emitted through g_signal_emitv
// (no current event: time 0, no modifiers, no current button), reach the
// handler as pointer events. GTK's double-click settings read as
// positive. The handles are freed with the window (runSurfaceTest's
// teardown check).
func TestSurfaceGTKPointer(t *testing.T) {
	needSurfaceGTK(t)
	runSurfaceTest(t, func(e *surfTest) error {
		t := e.t
		sf, g, h, err := e.open("pointer", shell.SurfaceConfig{Title: "gogui pointer",
			Size: driver.Size{W: 320, H: 240}, Style: driver.StyleTitled | driver.StyleResizable})
		if err != nil {
			return err
		}
		if err := show(sf, g, h); err != nil {
			return err
		}
		if g.motion == 0 || g.pointer == 0 {
			return fmt.Errorf("pointer controllers motion %#x, pointer %#x", g.motion, g.pointer)
		}
		for _, c := range []struct {
			inst uintptr
			typ  string
		}{{g.motion, "GtkEventControllerMotion"}, {g.pointer, "GtkGestureClick"}} {
			if name := typeName(typeOf(c.inst)); name != c.typ {
				t.Errorf("a pointer controller is a %s, want %s", name, c.typ)
			}
		}
		conns := map[uintptr]int{}
		for _, c := range g.conns {
			conns[c.inst]++
		}
		if conns[g.motion] != 3 || conns[g.pointer] != 3 {
			t.Errorf("handlers on the motion controller %d, on the pointer gesture %d; want 3 and 3",
				conns[g.motion], conns[g.pointer])
		}
		r := g.clickRules()
		t.Logf("POINTER-RESULT: double-click time %d ms, rectangle %d×%d px", r.Time, r.W, r.H)
		if r.Time == 0 || r.W <= 0 || r.H <= 0 {
			t.Errorf("clickRules() = %+v, want positive values", r)
		}

		sig := func(inst uintptr, name string) uint32 {
			id := gtk.GObject.SignalLookup(name, typeOf(inst))
			if id == 0 {
				t.Errorf("g_signal_lookup(%s) returned 0", name)
			}
			return id
		}
		enter, motion, leave := sig(g.motion, "enter"), sig(g.motion, "motion"), sig(g.motion, "leave")
		pressed, released := sig(g.pointer, "pressed"), sig(g.pointer, "released")
		if enter == 0 || motion == 0 || leave == 0 || pressed == 0 || released == 0 {
			return errors.New("missing controller signals")
		}
		n0 := h.events
		emitv(g.motion, enter, false, setDouble(50), setDouble(60))
		emitv(g.motion, motion, false, setDouble(51), setDouble(60))
		emitv(g.motion, motion, false, setDouble(51), setDouble(60)) // a repeat: dropped
		// No current button: the press names none and is dropped.
		emitv(g.pointer, pressed, false, setInt(1), setDouble(51), setDouble(60))
		emitv(g.pointer, released, false, setInt(1), setDouble(51), setDouble(60))
		emitv(g.motion, leave, false)
		rethrow()
		if got := h.events - n0; got != 3 {
			t.Errorf("%d events from enter, motion ×2, press, release, leave; want 3 (enter, move, leave)", got)
		}
		if sf.ptr.inside || sf.ptr.buttons != 0 {
			t.Errorf("after leave: inside %v, buttons %v", sf.ptr.inside, sf.ptr.buttons)
		}
		iterateFor(settle)
		return nil
	})
}
