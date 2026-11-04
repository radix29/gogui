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

// TestSurfaceGTKCaption: the GGSurface carries the caption gestures, a
// GtkGestureClick and a GtkGestureDrag for the primary button, with their
// handlers connected; their signals, emitted through g_signal_emitv,
// reach the surface: a drag from the caption past GTK's threshold runs
// beginMove (without a real press there is no device, so GDK is not
// asked to move, and nothing g_criticals), and a double-click zooms. The
// handles are freed with the window (runSurfaceTest's teardown check).
func TestSurfaceGTKCaption(t *testing.T) {
	needSurfaceGTK(t)
	runSurfaceTest(t, func(e *surfTest) error {
		t := e.t
		sf, g, h, err := e.open("caption", shell.SurfaceConfig{Title: "gogui caption",
			Size: driver.Size{W: 320, H: 240}, Style: driver.StyleTitled | driver.StyleResizable})
		if err != nil {
			return err
		}
		if err := show(sf, g, h); err != nil {
			return err
		}
		sf.SetChromeRegions([]shell.ChromeRegion{{Rect: driver.Rect{W: 320, H: 28}, Kind: shell.RegionCaption}})
		if g.click == 0 || g.drag == 0 {
			return fmt.Errorf("caption gestures click %#x, drag %#x", g.click, g.drag)
		}
		for _, c := range []struct {
			inst uintptr
			typ  string
		}{{g.click, "GtkGestureClick"}, {g.drag, "GtkGestureDrag"}} {
			if name := typeName(typeOf(c.inst)); name != c.typ {
				t.Errorf("a caption gesture is a %s, want %s", name, c.typ)
			}
		}
		conns := map[uintptr]int{}
		for _, c := range g.conns {
			conns[c.inst]++
		}
		if conns[g.click] != 1 || conns[g.drag] != 2 {
			t.Errorf("handlers on the click gesture %d, on the drag gesture %d; want 1 and 2",
				conns[g.click], conns[g.drag])
		}

		sig := func(inst uintptr, name string) uint32 {
			id := gtk.GObject.SignalLookup(name, typeOf(inst))
			if id == 0 {
				t.Errorf("g_signal_lookup(%s) returned 0", name)
			}
			return id
		}
		begin, update := sig(g.drag, "drag-begin"), sig(g.drag, "drag-update")
		pressed := sig(g.click, "pressed")
		if begin == 0 || update == 0 || pressed == 0 {
			return errors.New("missing gesture signals")
		}
		emitv(g.drag, begin, false, setDouble(100), setDouble(10))
		if !sf.drag.on || sf.drag.x != 100 || sf.drag.y != 10 {
			t.Errorf("after drag-begin on the caption, drag = %+v", sf.drag)
		}
		emitv(g.drag, update, false, setDouble(1), setDouble(0))
		if !sf.drag.on {
			t.Error("a 1-pixel drag-update passed the drag threshold")
		}
		emitv(g.drag, update, false, setDouble(60), setDouble(0))
		rethrow()
		if sf.drag.on {
			t.Error("a 60-pixel drag-update did not begin the move")
		}
		emitv(g.drag, begin, false, setDouble(100), setDouble(100)) // below the caption
		if sf.drag.on {
			t.Error("a drag-begin below the caption may move the window")
		}
		emitv(g.drag, update, false, setDouble(60), setDouble(0))
		emitv(g.click, pressed, false, setInt(2), setDouble(100), setDouble(10))
		rethrow()
		if sf.zooms != 1 {
			t.Error("a double-click on the caption did not zoom")
		}
		// emitv has no current event, so the time is 0 and matches no
		// zoom: the drag of this press may still move.
		emitv(g.drag, begin, false, setDouble(100), setDouble(10))
		if !sf.drag.on {
			t.Error("after a zoom, a drag-begin at event time 0 may not move")
		}
		iterateFor(settle)
		return nil
	})
}
