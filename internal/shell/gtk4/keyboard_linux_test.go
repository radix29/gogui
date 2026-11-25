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
// method (a GtkIMMulticontext with use-preedit, its commit and its three
// preedit handlers, S5.3i); without a current key event keyBase gives 0
// and the input method takes nothing; its preedit reads as empty, before
// and after a focus-in (a slave context exists then); the
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
		if name := typeName(typeOf(g.im)); name != "GtkIMMulticontext" {
			t.Errorf("the input method is a %s", name)
		}
		conns := map[uintptr]int{}
		for _, c := range g.conns {
			conns[c.inst]++
		}
		if conns[g.key] != 2 || conns[g.im] != 4 {
			t.Errorf("handlers on the key controller %d, on the input method %d; want 2 and 4",
				conns[g.key], conns[g.im])
		}
		if kv := g.keyBase(g.key, 38); kv != 0 {
			t.Errorf("keyBase without a key event = %#x, want 0", kv)
		}
		if kv := g.keyShifted(g.key, 38); kv != 0 {
			t.Errorf("keyShifted without a key event = %#x, want 0", kv)
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
		emptyPreedit := func(when string) {
			text, cursor, attrs := g.preedit()
			if text != "" || cursor != 0 || len(attrs) != 0 {
				t.Errorf("the preedit %s is %q, cursor %d, %d attributes; want empty", when, text, cursor, len(attrs))
			}
			if ev := preeditOf(text, cursor, attrs); ev.Text != "" || len(ev.Segments) != 0 {
				t.Errorf("the preedit %s converts to %+v", when, ev)
			}
		}
		emptyPreedit("before focus")
		g.imFocus(true)
		emptyPreedit("after focus-in")
		g.imReset()
		g.imFocus(false)
		g.imFocus(true)
		iterateFor(settle)
		return nil
	})
}

// TestSurfaceGTKTextInput (S5.3j): turning the text input on attaches the
// input method to the key controller, sets its cursor location and, in the
// key window, its focus; a new caret, CancelComposition and turning it off
// again (focus out, reset, detach) run without a GTK warning; there is no
// current key event outside one (imEventTime 0). Closing with the input on
// detaches it before the window goes and releases the input method once
// the queue runs.
func TestSurfaceGTKTextInput(t *testing.T) {
	needSurfaceGTK(t)
	runSurfaceTest(t, func(e *surfTest) error {
		t := e.t
		sf, g, h, err := e.open("textinput", shell.SurfaceConfig{Title: "gogui text input",
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
		on := shell.TextInput{Enabled: true, Caret: driver.Rect{X: 10.5, Y: 20, W: 1, H: 16}}
		sf.SetTextInput(on)
		if !g.imAttached || !sf.ime.attached {
			t.Errorf("on: window attached %v, surface attached %v", g.imAttached, sf.ime.attached)
		}
		if tm := g.imEventTime(); tm != 0 {
			t.Errorf("imEventTime outside a key event = %d, want 0", tm)
		}
		on.Caret.X = 40
		sf.SetTextInput(on)
		sf.CancelComposition()
		sf.SetTextInput(shell.TextInput{})
		if g.imAttached || sf.ime.attached || sf.ime.focused {
			t.Errorf("off: window attached %v, surface attached %v, focused %v",
				g.imAttached, sf.ime.attached, sf.ime.focused)
		}
		sf.SetTextInput(on)
		iterateFor(settle)
		t.Logf("TEXTINPUT-RESULT: key window %v, input method focused %v", sf.key, sf.ime.focused)
		sf.Close()
		if g.im == 0 {
			t.Error("the input method was released inside Close")
		}
		iterateFor(settle)
		if g.im != 0 {
			t.Error("the input method was not released after the queue ran")
		}
		return nil
	})
}
