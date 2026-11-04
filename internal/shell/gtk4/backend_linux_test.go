//go:build linux

package gtk4

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// TestShellGTKBackend checks which GDK backend the gtk4 tests run on, as
// spike/gtk's phase 2 did (S0.9c; ported in S4.1l when the spike was
// deleted): with GDK_BACKEND=x11 or wayland the display must be
// GdkX11Display or GdkWaylandDisplay, so native.sh's xvfb and weston runs
// really test both backends. A shown window must have a GSK renderer. The
// display, the renderer GSK picked (an open question of S0.9: Mesa
// llvmpipe GL is expected under xvfb and headless weston), the GTK version
// and the scale are logged as BACKEND-RESULT.
func TestShellGTKBackend(t *testing.T) {
	needSurfaceGTK(t)
	runSurfaceTest(t, func(e *surfTest) error {
		t := e.t
		sf, g, h, err := e.open("backend", shell.SurfaceConfig{Title: "gogui backend",
			Size: driver.Size{W: 200, H: 120}, Style: driver.StyleTitled})
		if err != nil {
			return err
		}
		if err := show(sf, g, h); err != nil {
			return err
		}
		d := gtk.Gtk.WidgetGetDisplay(g.win)
		if d == 0 {
			return errors.New("gtk_widget_get_display returned NULL")
		}
		display := typeName(typeOf(d))
		renderer := "none"
		if r := gtk.Gtk.NativeGetRenderer(g.win); r != 0 {
			renderer = typeName(typeOf(r))
		}
		backend := os.Getenv("GDK_BACKEND")
		t.Logf("BACKEND-RESULT: display %s, renderer %s, GTK %v, scale %v, GDK_BACKEND=%q",
			display, renderer, gtk.Version(), sf.Scale(), backend)
		want := map[string]string{"x11": "GdkX11Display", "wayland": "GdkWaylandDisplay"}[backend]
		switch {
		case backend == "" || strings.Contains(backend, ","):
			t.Logf("GDK_BACKEND=%q names no single backend; nothing to compare", backend)
		case want == "":
			t.Errorf("GDK_BACKEND=%s is not x11 or wayland", backend)
		case display != want:
			t.Errorf("GDK_BACKEND=%s but the display is %s, want %s", backend, display, want)
		}
		if renderer == "none" {
			t.Error("gtk_native_get_renderer returned NULL for a shown window")
		}
		return nil
	})
}
