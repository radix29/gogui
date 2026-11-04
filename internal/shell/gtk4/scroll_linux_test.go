//go:build linux

package gtk4

import (
	"errors"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// TestSurfaceGTKScroll: the GGSurface carries the scroll controller, a
// GtkEventControllerScroll with its four handlers connected; its signals,
// emitted through g_signal_emitv (no current event, so the unit is GTK's
// initial WHEEL), reach the handler: "scroll" returns TRUE and is a wheel
// ScrollEvent, scroll-begin and scroll-end around a wheel event add no
// phase, and a "decelerate" after them starts no momentum. The handles
// are freed with the window (runSurfaceTest's teardown check).
func TestSurfaceGTKScroll(t *testing.T) {
	needSurfaceGTK(t)
	runSurfaceTest(t, func(e *surfTest) error {
		t := e.t
		sf, g, h, err := e.open("scroll", shell.SurfaceConfig{Title: "gogui scroll",
			Size: driver.Size{W: 320, H: 240}, Style: driver.StyleTitled | driver.StyleResizable})
		if err != nil {
			return err
		}
		if err := show(sf, g, h); err != nil {
			return err
		}
		if g.scroll == 0 {
			return errors.New("no scroll controller")
		}
		if name := typeName(typeOf(g.scroll)); name != "GtkEventControllerScroll" {
			t.Errorf("the scroll controller is a %s", name)
		}
		n := 0
		for _, c := range g.conns {
			if c.inst == g.scroll {
				n++
			}
		}
		if n != 4 {
			t.Errorf("%d handlers on the scroll controller, want 4", n)
		}
		unit := g.scrollUnit(g.scroll)
		t.Logf("SCROLL-RESULT: the unit without an event is %d (0 WHEEL, 1 SURFACE)", unit)

		sig := func(name string) uint32 {
			id := gtk.GObject.SignalLookup(name, typeOf(g.scroll))
			if id == 0 {
				t.Errorf("g_signal_lookup(%s) returned 0", name)
			}
			return id
		}
		scroll, begin, end, decel := sig("scroll"), sig("scroll-begin"), sig("scroll-end"), sig("decelerate")
		if scroll == 0 || begin == 0 || end == 0 || decel == 0 {
			return errors.New("missing scroll signals")
		}
		n0 := h.events
		if r := emitv(g.scroll, scroll, true, setDouble(0), setDouble(1)); r != gtk.True {
			t.Errorf("scroll returned %d, want TRUE", r)
		}
		emitv(g.scroll, begin, false)
		emitv(g.scroll, scroll, true, setDouble(0.5), setDouble(0))
		emitv(g.scroll, end, false)
		emitv(g.scroll, decel, false, setDouble(0), setDouble(3000))
		rethrow()
		if got := h.events - n0; unit == gdkScrollUnitWheel && got != 2 {
			t.Errorf("%d events from two wheel scrolls in and out of begin/end, want 2", got)
		}
		if sf.scr.mom.on {
			t.Error("a decelerate after a wheel gesture started momentum")
		}
		iterateFor(settle)
		if got := h.events - n0; unit == gdkScrollUnitWheel && got != 2 {
			t.Errorf("%d events after the frames settled, want still 2", got)
		}
		return nil
	})
}
