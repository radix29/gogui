//go:build linux

package gtk4

import (
	"errors"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// TestSurfaceGTKKeyboard: the window carries the key controller (a
// GtkEventControllerKey with its two handlers) and the surface an input
// method (a GtkIMContextSimple with its commit handler); without a current
// key event keyBase gives 0 and the input method takes nothing; the
// keyval mapping of GDK holds for keyFor's character path (Latin, Cyrillic,
// a dead key's keyval). Focus changes and resets reach the input method
// without a GTK warning. The handles are freed with the window
// (runSurfaceTest's teardown check).
func TestSurfaceGTKKeyboard(t *testing.T) {
	needSurfaceGTK(t)
	runSurfaceTest(t, func(e *surfTest) error {
		t := e.t
		sf, g, h, err := e.open("keyboard", shell.SurfaceConfig{Title: "gogui keyboard",
			Size: driver.Size{W: 320, H: 240}, Style: driver.StyleTitled | driver.StyleResizable})
		if err != nil {
			return err
		}
		if err := show(sf, g, h); err != nil {
			return err
		}
		if g.key == 0 || g.im == 0 {
			return errors.New("no key controller or no input method")
		}
		if name := typeName(typeOf(g.key)); name != "GtkEventControllerKey" {
			t.Errorf("the key controller is a %s", name)
		}
		if name := typeName(typeOf(g.im)); name != "GtkIMContextSimple" {
			t.Errorf("the input method is a %s", name)
		}
		conns := map[uintptr]int{}
		for _, c := range g.conns {
			conns[c.inst]++
		}
		if conns[g.key] != 2 || conns[g.im] != 1 {
			t.Errorf("handlers on the key controller %d, on the input method %d; want 2 and 1",
				conns[g.key], conns[g.im])
		}
		if kv := g.keyBase(g.key, 38); kv != 0 {
			t.Errorf("keyBase without a key event = %#x, want 0", kv)
		}
		if g.imFilter(g.key) {
			t.Error("imFilter took a key without a key event")
		}
		for kv, want := range map[uint32]driver.Key{0x61: 'A', 0x6c1: 'А', 0x10020ac: '€', 0xff0d: driver.KeyReturn} {
			if got := keyFor(kv, 0, g.keyvalToUnicode); got != want {
				t.Errorf("keyFor(%#x) over gdk_keyval_to_unicode = %q, want %q", kv, got, want)
			}
		}
		t.Logf("KEY-RESULT: gdk_keyval_to_unicode(dead_acute) = %#x", g.keyvalToUnicode(0xfe51))
		g.imFocus(true)
		g.imReset()
		g.imFocus(false)
		g.imFocus(true)
		iterateFor(settle)
		return nil
	})
}
