//go:build linux

package gtk4

import (
	"fmt"
	"testing"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// The signal and source tests (spike/gtk phase 1): real emissions through
// g_signal_emitv and a real idle source, through the trampolines.

func setInt(i int32) func(uintptr) {
	return func(v uintptr) { gtk.GObject.ValueInit(v, gtk.TypeInt); gtk.GObject.ValueSetInt(v, i) }
}

func setDouble(d float64) func(uintptr) {
	return func(v uintptr) { gtk.GObject.ValueInit(v, gtk.TypeDouble); gtk.GObject.ValueSetDouble(v, d) }
}

// TestSignalPressed connects sigIntDoubleDouble to GtkGestureClick::pressed
// and emits it: exact doubles and ints arrive with the entry's kind; after
// the disconnect closureNotify has freed the handle and the handler no
// longer runs.
func TestSignalPressed(t *testing.T) {
	needGTK(t)
	onGTK(t, func() error {
		base := dispatch.Len()
		gesture := gtk.Gtk.GestureClickNew()
		if gesture == 0 {
			return fmt.Errorf("gtk_gesture_click_new returned NULL")
		}
		defer gtk.GObject.ObjectUnref(gesture)
		p := &pressPeer{}
		id := connect(gesture, "pressed", tramp.sigIntDoubleDouble, p, kindTestB)
		if id == 0 {
			return fmt.Errorf("connect(pressed) returned 0")
		}
		if dispatch.Len() != base+1 {
			return fmt.Errorf("connect added %d handles, want 1", dispatch.Len()-base)
		}
		sig := gtk.GObject.SignalLookup("pressed", typeOf(gesture))
		if sig == 0 {
			return fmt.Errorf("g_signal_lookup(pressed) returned 0")
		}
		emit := func(n int32, x, y float64) { emitv(gesture, sig, false, setInt(n), setDouble(x), setDouble(y)) }
		emit(2, 12.5, -3.25)
		emit(-7, 1e300, 0.1) // values a float32 or a swapped register would break
		want := []recCall{
			{sig: "idd", k: kindTestB, instance: gesture, n: 2, x: 12.5, y: -3.25},
			{sig: "idd", k: kindTestB, instance: gesture, n: -7, x: 1e300, y: 0.1},
		}
		if len(p.calls) != 2 || p.calls[0] != want[0] || p.calls[1] != want[1] {
			return fmt.Errorf("calls %+v, want %+v", p.calls, want)
		}
		gtk.GObject.SignalHandlerDisconnect(gesture, id)
		if dispatch.Len() != base {
			return fmt.Errorf("after the disconnect dispatch.Len() = %d, want %d: GClosureNotify did not free the handle",
				dispatch.Len(), base)
		}
		emit(1, 0, 0)
		if len(p.calls) != 2 {
			return fmt.Errorf("the handler ran after g_signal_handler_disconnect")
		}
		// An unknown signal is refused before g_signal_connect_data (which
		// would g_critical) and leaves no handle.
		if id := connect(gesture, "no-such-signal", tramp.sigIntDoubleDouble, p, kindTestB); id != 0 {
			return fmt.Errorf("connect(no-such-signal) = %d", id)
		}
		if dispatch.Len() != base {
			return fmt.Errorf("a refused connect left a handle")
		}
		return nil
	})
}

// TestSignalFinalizeFrees: unreffing the instance drops its handlers, and
// closureNotify frees their handles.
func TestSignalFinalizeFrees(t *testing.T) {
	needGTK(t)
	onGTK(t, func() error {
		base := dispatch.Len()
		gesture := gtk.Gtk.GestureClickNew()
		connect(gesture, "pressed", tramp.sigIntDoubleDouble, &pressPeer{}, kindTestA)
		connect(gesture, "released", tramp.sigIntDoubleDouble, &pressPeer{}, kindTestB)
		gtk.GObject.ObjectUnref(gesture)
		if dispatch.Len() != base {
			return fmt.Errorf("after finalizing dispatch.Len() = %d, want %d", dispatch.Len(), base)
		}
		return nil
	})
}

// TestSignalPanicRethrown: a panic in a handler does not unwind through
// g_signal_emitv; it is kept, emitv returns, and rethrow raises it.
func TestSignalPanicRethrown(t *testing.T) {
	needGTK(t)
	var raised any
	onGTK(t, func() error {
		gesture := gtk.Gtk.GestureClickNew()
		defer gtk.GObject.ObjectUnref(gesture)
		p := &pressPeer{panics: "handler"}
		connect(gesture, "pressed", tramp.sigIntDoubleDouble, p, kindTestA)
		sig := gtk.GObject.SignalLookup("pressed", typeOf(gesture))
		emitv(gesture, sig, false, setInt(1), setDouble(1), setDouble(2))
		if len(p.calls) != 1 {
			return fmt.Errorf("the handler ran %d times", len(p.calls))
		}
		func() {
			defer func() { raised = recover() }()
			rethrow()
		}()
		return nil
	})
	pe, ok := raised.(*PanicError)
	if !ok || pe.Value != "handler" || pe.Callback != nameSigIntDoubleDouble {
		t.Errorf("rethrow raised %#v, want the handler's panic", raised)
	}
}

// closePeer answers GtkWindow::close-request.
type closePeer struct {
	calls int
	keep  bool // TRUE: stop the close
}

func (p *closePeer) boolSignal(k kind, _ uintptr) bool {
	p.calls++
	return p.keep
}

// TestSignalBool emits GtkWindow::close-request through sigBool: the
// gboolean reaches GTK both ways, and a FALSE lets gtk_window_close
// destroy the window.
func TestSignalBool(t *testing.T) {
	needGTK(t)
	if testGtk.ValueGetBoolean == nil || testGtk.WindowDestroy == nil {
		t.Fatal("g_value_get_boolean or gtk_window_destroy did not bind")
	}
	onGTK(t, func() error {
		win := gtk.Gtk.WindowNew()
		p := &closePeer{keep: true}
		if connect(win, "close-request", tramp.sigBool, p, kindTestA) == 0 {
			testGtk.WindowDestroy(win)
			return fmt.Errorf("connect(close-request) returned 0")
		}
		sig := gtk.GObject.SignalLookup("close-request", typeOf(win))
		if r := emitv(win, sig, true); r != gtk.True || p.calls != 1 {
			testGtk.WindowDestroy(win)
			return fmt.Errorf("close-request returned %d after %d calls, want TRUE after 1", r, p.calls)
		}
		p.keep = false
		if r := emitv(win, sig, true); r != gtk.False {
			testGtk.WindowDestroy(win)
			return fmt.Errorf("close-request returned %d, want FALSE", r)
		}
		testGtk.WindowDestroy(win)
		return nil
	})
}

// countdown is a source that stays for n calls, then goes.
type countdown struct{ n, calls int }

func (c *countdown) source(kind) bool { c.calls++; return c.calls < c.n }

// TestIdleSource: G_SOURCE_CONTINUE keeps the idle source,
// G_SOURCE_REMOVE removes it, and its handle is freed.
func TestIdleSource(t *testing.T) {
	needGTK(t)
	onGTK(t, func() error {
		// Drain what earlier tests left (a late finalization frees handles),
		// so base is stable; the check below only asks that Len did not grow.
		for i := 0; i < 1000 && gtk.GLib.MainContextIteration(0, gtk.False) != gtk.False; i++ {
		}
		base := dispatch.Len()
		c := &countdown{n: 3}
		id, h := addIdle(c, kindTestA)
		if id == 0 || h == 0 {
			return fmt.Errorf("addIdle = %d, %v", id, h)
		}
		for i := 0; i < 100 && c.calls < 3; i++ {
			gtk.GLib.MainContextIteration(0, gtk.False)
		}
		if c.calls != 3 {
			gtk.GLib.SourceRemove(id)
			dispatch.Free(h)
			return fmt.Errorf("the source ran %d times in 100 iterations, want 3", c.calls)
		}
		for range 10 {
			gtk.GLib.MainContextIteration(0, gtk.False)
		}
		switch {
		case c.calls != 3:
			return fmt.Errorf("the source ran %d times; G_SOURCE_REMOVE did not remove it", c.calls)
		case gtk.GLib.MainContextFindSourceByID(0, id) != 0:
			gtk.GLib.SourceRemove(id)
			return fmt.Errorf("source %d is still attached", id)
		case dispatch.Valid(h) || dispatch.Len() > base:
			return fmt.Errorf("the removed source kept its handle")
		}
		return nil
	})
}

// TestCallbacksStillPinned runs after the others (file order): connecting
// signals and adding sources created no callbacks.
func TestCallbacksStillPinned(t *testing.T) {
	if got := ffi.Callbacks(); got != numTrampolines {
		t.Errorf("ffi.Callbacks() = %d, want %d", got, numTrampolines)
	}
}
