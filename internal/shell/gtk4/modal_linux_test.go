//go:build linux

package gtk4

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// The modal surface over real GtkWindows (S5.7f). The logic of modal.go
// is host-tested over the fake window (modal_test.go); this checks what
// only GTK can say: the window is modal and not resizable, its first Show
// makes it transient for the owner (for the key surface when app-modal),
// the owner's window is insensitive while it is shown and sensitive again
// once it closes, and closing the owner closes the modal first. Where the
// window manager puts it and who gets the key is logged (MODAL-RESULT),
// not failed: under bare xvfb nothing activates a window.

var modalGtk struct {
	WindowGetModal        func(win uintptr) int32   // gboolean
	WindowGetTransientFor func(win uintptr) uintptr // GtkWindow *, transfer none
	WidgetGetSensitive    func(w uintptr) int32     // gboolean
}

var bindModalGtk = sync.OnceValue(func() error {
	lib, err := ffi.Dlopen(gtk.GTKLib)
	if err != nil {
		return err
	}
	return errors.Join(
		lib.Bind(&modalGtk.WindowGetModal, "gtk_window_get_modal"),
		lib.Bind(&modalGtk.WindowGetTransientFor, "gtk_window_get_transient_for"),
		lib.Bind(&modalGtk.WidgetGetSensitive, "gtk_widget_get_sensitive"))
})

func TestModalGTK(t *testing.T) {
	needSurfaceGTK(t)
	if err := bindModalGtk(); err != nil {
		t.Fatal(err)
	}
	runSurfaceTest(t, func(e *surfTest) error {
		a, ga, ha, err := e.open("a", shell.SurfaceConfig{Title: "gogui modal owner", Size: driver.Size{W: 400, H: 300},
			Style: driver.StyleTitled | driver.StyleClosable | driver.StyleResizable})
		if err != nil {
			return err
		}
		if err := show(a, ga, ha); err != nil {
			return err
		}
		m, gm, hm, err := e.open("m", shell.SurfaceConfig{Title: "m", Size: driver.Size{W: 260, H: 180},
			Style: driver.StyleResizable, Modal: true, Owner: a})
		if err != nil {
			return err
		}
		if !isTrue(modalGtk.WindowGetModal(gm.win)) || isTrue(surfGtk.WindowGetResizable(gm.win)) {
			e.t.Error("the modal window is not modal, or resizable")
		}
		if !isTrue(modalGtk.WidgetGetSensitive(ga.win)) {
			e.t.Error("a hidden modal surface made the owner insensitive")
		}
		if err := show(m, gm, hm); err != nil {
			return err
		}
		if got := modalGtk.WindowGetTransientFor(gm.win); got != ga.win {
			e.t.Errorf("transient for %#x, want the owner's window %#x", got, ga.win)
		}
		if isTrue(modalGtk.WidgetGetSensitive(ga.win)) || !isTrue(modalGtk.WidgetGetSensitive(gm.win)) {
			e.t.Error("the owner is sensitive, or the modal insensitive")
		}
		iterateFor(settle)
		e.t.Logf("MODAL-RESULT: shown: owner active %v, modal active %v; owner KeyChanged %v, modal %v",
			ga.active(), gm.active(), ha.keys, hm.keys)

		m.Close()
		if !isTrue(modalGtk.WidgetGetSensitive(ga.win)) {
			e.t.Error("the owner is insensitive after the modal closed")
		}
		iterateFor(settle)
		e.t.Logf("MODAL-RESULT: closed: owner active %v; owner KeyChanged %v", ga.active(), ha.keys)

		// App-modal: transient for the key surface, if there is one.
		m2, gm2, hm2, err := e.open("m2", shell.SurfaceConfig{Title: "m2", Size: driver.Size{W: 200, H: 100}, Modal: true})
		if err != nil {
			return err
		}
		if err := show(m2, gm2, hm2); err != nil {
			return err
		}
		want := uintptr(0)
		if a.key {
			want = ga.win
		}
		if got := modalGtk.WindowGetTransientFor(gm2.win); got != want {
			e.t.Errorf("app-modal: transient for %#x, want %#x (owner key %v)", got, want, a.key)
		}

		// The owner closing closes its modal first.
		m3, gm3, hm3, err := e.open("m3", shell.SurfaceConfig{Title: "m3", Size: driver.Size{W: 200, H: 100}, Modal: true, Owner: a})
		if err != nil {
			return err
		}
		if err := show(m3, gm3, hm3); err != nil {
			return err
		}
		a.Close()
		if got := fmt.Sprint(e.order); got != "[m m3 a]" {
			e.t.Errorf("Closed order %s, want [m m3 a]", got)
		}
		m2.Close()
		return nil
	})
}
