//go:build linux

package gtk4

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
	"github.com/radix29/gogui/internal/shell/shellkit"
)

// The surface over a real GtkWindow (S4.1f). The pure-Go logic of
// surface.go is host-tested over a fake window; these tests check what
// only GTK can say: the window NewSurface builds (realized, hidden, CSD
// with an invisible title bar, the GGSurface as its child), that Show maps
// it at the requested size and paints, that SetSize reaches a mapped
// window (by which path: the open question of spike S0.9c), the size
// request, close-request, Zoom, Minimize, the cursors and Quit.
//
// Every test is a real Run (runSurfaceTest): Ready posts the scenario,
// which runs on the GTK thread inside the running GMainLoop and waits for
// GTK by iterating the default main context (iterateUntil, a nested
// iteration, as a modal loop would), every wait bounded; then it quits.
// run's 10 s guard (shell_test.go) posts Quit if a scenario still hangs.
//
// Outcomes that depend on the windowing system are logged, not failed,
// with a prefix to grep the native log for: SIZE-RESULT (the first
// allocation), RESIZE-RESULT (SetSize and SetSizeLimits on a mapped window),
// KEY-RESULT, ZOOM-RESULT, MINIMIZE-RESULT. Under bare xvfb there is no
// window manager: nothing activates, maximizes or minimizes a window.
//
// GTK semantics assumed (GTK 4.14+):
//   - gtk_window_close returns at once for an unrealized window; otherwise
//     it emits close-request and destroys the window unless a handler
//     returned TRUE. NewSurface realizes the window, so it works on a
//     hidden surface too.
//   - gtk_window_set_titlebar with a widget turns on client-side
//     decorations: the window gets the CSS class "csd", or "solid-csd"
//     where the display cannot draw a client shadow (X11 without a
//     compositor, so bare xvfb).
//   - gtk_window_get_toplevels lists every GtkWindow not yet destroyed;
//     gtk_window_destroy removes it at once.
//   - gtk_widget_set_cursor_from_name always creates a GdkCursor by name,
//     and gdk_cursor_get_name returns that name.

const (
	showWait   = 3 * time.Second        // Show until mapped and painted, Invalidate until painted
	resizeWait = 2 * time.Second        // SetSize until the allocation reaches the size
	stateWait  = 2 * time.Second        // maximize and unmaximize
	shortWait  = time.Second            // KeyChanged, minimize, a lowered size request
	settle     = 100 * time.Millisecond // follow-up layouts after a wait
)

// surfGtk holds functions only the surface tests call.
var surfGtk struct {
	WidgetGetRealized    func(w uintptr) int32 // gboolean
	WidgetGetVisible     func(w uintptr) int32 // gboolean
	WidgetGetSizeRequest func(w uintptr, width, height *int32)
	WidgetHasCSSClass    func(w uintptr, class string) int32                  // gboolean
	WidgetGetCursor      func(w uintptr) uintptr                              // GdkCursor *, transfer none
	WidgetComputeBounds  func(w, target uintptr, out *gtk.GrapheneRect) int32 // gboolean
	CursorGetName        func(c uintptr) uintptr                              // const char *
	WindowGetChild       func(win uintptr) uintptr
	WindowGetTitlebar    func(win uintptr) uintptr
	WindowGetTitle       func(win uintptr) uintptr // const char *
	WindowGetResizable   func(win uintptr) int32   // gboolean
	WindowGetDecorated   func(win uintptr) int32   // gboolean
	WindowGetDefaultSize func(win uintptr, width, height *int32)
	WindowGetToplevels   func() uintptr // GListModel *, transfer none
	ListModelGetNItems   func(list uintptr) uint32
}

const gioLib = "libgio-2.0.so.0"

var bindSurfGtk = sync.OnceValue(func() error {
	var errs []error
	for _, b := range []struct {
		lib, name string
		fptr      any
	}{
		{gtk.GTKLib, "gtk_widget_get_realized", &surfGtk.WidgetGetRealized},
		{gtk.GTKLib, "gtk_widget_get_visible", &surfGtk.WidgetGetVisible},
		{gtk.GTKLib, "gtk_widget_get_size_request", &surfGtk.WidgetGetSizeRequest},
		{gtk.GTKLib, "gtk_widget_has_css_class", &surfGtk.WidgetHasCSSClass},
		{gtk.GTKLib, "gtk_widget_get_cursor", &surfGtk.WidgetGetCursor},
		{gtk.GTKLib, "gtk_widget_compute_bounds", &surfGtk.WidgetComputeBounds},
		{gtk.GTKLib, "gdk_cursor_get_name", &surfGtk.CursorGetName},
		{gtk.GTKLib, "gtk_window_get_child", &surfGtk.WindowGetChild},
		{gtk.GTKLib, "gtk_window_get_titlebar", &surfGtk.WindowGetTitlebar},
		{gtk.GTKLib, "gtk_window_get_title", &surfGtk.WindowGetTitle},
		{gtk.GTKLib, "gtk_window_get_resizable", &surfGtk.WindowGetResizable},
		{gtk.GTKLib, "gtk_window_get_decorated", &surfGtk.WindowGetDecorated},
		{gtk.GTKLib, "gtk_window_get_default_size", &surfGtk.WindowGetDefaultSize},
		{gtk.GTKLib, "gtk_window_get_toplevels", &surfGtk.WindowGetToplevels},
		{gioLib, "g_list_model_get_n_items", &surfGtk.ListModelGetNItems},
	} {
		lib, err := ffi.Dlopen(b.lib)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := lib.Bind(b.fptr, b.name); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
})

// needSurfaceGTK is needGTK plus the test-only bindings.
func needSurfaceGTK(t *testing.T) {
	t.Helper()
	needGTK(t)
	if err := bindSurfGtk(); err != nil {
		t.Fatal(err)
	}
}

// surfRec is a recording shell.SurfaceHandler. It fails the test if any
// method but Closed runs after Closed, or Closed runs twice.
type surfRec struct {
	t     *testing.T
	name  string
	order *[]string // the names in the order Closed ran

	allowClose       bool   // CloseRequested's answer
	onCloseRequested func() // runs inside CloseRequested
	onClosed         func() // runs inside Closed
	onResized        func() // runs inside Resized, after recording it

	paints, closeReqs, closed, events int
	resized                           []resizeCall
	keys                              []bool
}

type resizeCall struct {
	size  driver.Size
	scale float64
}

func (c resizeCall) String() string { return fmt.Sprintf("%v×%v@%v", c.size.W, c.size.H, c.scale) }

var _ shell.SurfaceHandler = (*surfRec)(nil)

func (h *surfRec) alive(what string) {
	if h.closed > 0 {
		h.t.Errorf("%s: %s after Closed", h.name, what)
	}
}

func (h *surfRec) Paint(render.Canvas) { h.alive("Paint"); h.paints++ }

func (h *surfRec) Resized(size driver.Size, scale float64) {
	h.alive("Resized")
	if !(scale > 0) {
		h.t.Errorf("%s: Resized with scale %v", h.name, scale)
	}
	h.resized = append(h.resized, resizeCall{size, scale})
	if h.onResized != nil {
		h.onResized()
	}
}

func (h *surfRec) CloseRequested() bool {
	h.alive("CloseRequested")
	h.closeReqs++
	if h.onCloseRequested != nil {
		h.onCloseRequested()
	}
	return h.allowClose
}

func (h *surfRec) Closed() {
	h.closed++
	if h.closed > 1 {
		h.t.Errorf("%s: Closed called %d times", h.name, h.closed)
		return
	}
	if h.order != nil {
		*h.order = append(*h.order, h.name)
	}
	if h.onClosed != nil {
		h.onClosed()
	}
}

func (h *surfRec) KeyChanged(isKey bool) { h.alive("KeyChanged"); h.keys = append(h.keys, isKey) }

func (h *surfRec) Event(shell.Event) bool { h.alive("Event"); h.events++; return false }

// surfTest is one Run of a surface scenario.
type surfTest struct {
	t     *testing.T
	s     *Shell
	recs  []*surfRec
	order []string // Closed order
}

// open creates a surface with a recording handler.
func (e *surfTest) open(name string, cfg shell.SurfaceConfig) (*surface, *gtkWindow, *surfRec, error) {
	h := &surfRec{t: e.t, name: name, order: &e.order}
	ss, err := e.s.NewSurface(cfg, h)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("NewSurface(%s): %w", name, err)
	}
	e.recs = append(e.recs, h)
	sf := ss.(*surface)
	g, ok := sf.w.(*gtkWindow)
	if !ok {
		return nil, nil, nil, fmt.Errorf("NewSurface(%s): the window is a %T, not a *gtkWindow", name, sf.w)
	}
	return sf, g, h, nil
}

// runSurfaceTest runs a fresh shell whose Ready posts scenario; after it
// the shell quits. It then checks that every surface got Closed once and
// that everything is torn down. The scenario runs on the GTK thread and
// reports through e.t (t.Errorf is safe there); its error fails the test.
func runSurfaceTest(t *testing.T, scenario func(e *surfTest) error) *surfTest {
	t.Helper()
	return runSurfaceTestWith(t, nil, scenario)
}

// runSurfaceTestWith is runSurfaceTest with setup run on the fresh shell
// before Run (to set its appearance source, for one); nil does nothing.
func runSurfaceTestWith(t *testing.T, setup func(s *Shell), scenario func(e *surfTest) error) *surfTest {
	t.Helper()
	s, reports := newGTKShell(t)
	if setup != nil {
		setup(s)
	}
	base := dispatch.Len()
	e := &surfTest{t: t, s: s}
	ran := false
	p, err := runGTK(t, s, &handler{ready: func() {
		s.Post(func() {
			ran = true
			if err := scenario(e); err != nil {
				t.Error(err)
			}
			s.Quit()
		})
	}})
	if p != nil || err != nil {
		t.Fatalf("Run = %v, panicked with %v", err, p)
	}
	if !ran {
		t.Error("the scenario did not run")
	}
	for _, h := range e.recs {
		if h.closed != 1 {
			t.Errorf("%s: Closed called %d times by the end of Run, want once", h.name, h.closed)
		}
	}
	if len(s.surfaces) != 0 {
		t.Errorf("%d surfaces left open after Run", len(s.surfaces))
	}
	checkGTKTornDown(t, s, reports, base)
	return e
}

// The GTK-side readers; GTK goroutine only, on a live window.

func isTrue(b int32) bool { return b != gtk.False }

func mapped(w uintptr) bool { return isTrue(gtk.Gtk.WidgetGetMapped(w)) }

func allocOf(w uintptr) [2]int32 {
	return [2]int32{gtk.Gtk.WidgetGetWidth(w), gtk.Gtk.WidgetGetHeight(w)}
}

// borderBoxOf returns the size of w's border box (gtk_widget_compute_bounds
// in its own coordinates): for the window, its CSS padding and border
// included, its margin (the CSD shadow) not. gtk_widget_get_width is the
// content box.
func borderBoxOf(w uintptr) [2]int32 {
	var r gtk.GrapheneRect
	if !isTrue(surfGtk.WidgetComputeBounds(w, w, &r)) {
		return [2]int32{-1, -1}
	}
	return [2]int32{int32(r.Size.Width + 0.5), int32(r.Size.Height + 0.5)}
}

func sub(a, b [2]int32) [2]int32 { return [2]int32{a[0] - b[0], a[1] - b[1]} }

func sizeRequestOf(w uintptr) [2]int32 {
	var a, b int32
	surfGtk.WidgetGetSizeRequest(w, &a, &b)
	return [2]int32{a, b}
}

func defaultSizeOf(win uintptr) [2]int32 {
	var a, b int32
	surfGtk.WindowGetDefaultSize(win, &a, &b)
	return [2]int32{a, b}
}

func toplevels() uint32 { return surfGtk.ListModelGetNItems(surfGtk.WindowGetToplevels()) }

func pxOf(sz driver.Size) [2]int32 {
	w, h := px(sz)
	return [2]int32{w, h}
}

func sizeOf(w, h int32) driver.Size { return driver.Size{W: float64(w), H: float64(h)} }

// csdClass returns the window's CSD style class, "" for none.
func csdClass(win uintptr) string {
	for _, c := range []string{"csd", "solid-csd"} {
		if isTrue(surfGtk.WidgetHasCSSClass(win, c)) {
			return c
		}
	}
	return ""
}

// iterateFor iterates the main context for d.
func iterateFor(d time.Duration) { iterateUntil(d, func() bool { return false }) }

// show shows sf and waits until its GGSurface is mapped and painted.
func show(sf *surface, g *gtkWindow, h *surfRec) error {
	sf.Show()
	if !iterateUntil(showWait, func() bool { return !sf.dead && mapped(g.surf) && h.paints > 0 }) {
		if sf.dead {
			return fmt.Errorf("%s: closed while waiting for Show", h.name)
		}
		return fmt.Errorf("%s: within %v of Show: mapped %v, %d paints", h.name, showWait, mapped(g.surf), h.paints)
	}
	return nil
}

// TestSurfaceGTKNew: NewSurface builds a realized, hidden GtkWindow with
// CSD and an invisible GGSurface title bar, the GGSurface as its child,
// with the title (NULs dropped), the resizable flag, the default size and
// the size request of the config; it calls no handler method; it adds one
// handle for the GGSurface and one per connected signal, and Close frees
// them all, destroys the window and calls Closed once. A closed surface
// ignores every method.
func TestSurfaceGTKNew(t *testing.T) {
	needSurfaceGTK(t)
	runSurfaceTest(t, func(e *surfTest) error {
		t := e.t
		tops := toplevels()
		resizable := driver.StyleTitled | driver.StyleClosable | driver.StyleMiniaturizable | driver.StyleResizable
		cases := []struct {
			name, title string
			cfg         shell.SurfaceConfig
			request     [2]int32
		}{
			{"resizable", "gogui surface test",
				shell.SurfaceConfig{Title: "gogui\x00 surface test", Size: driver.Size{W: 320, H: 240},
					MinSize: driver.Size{W: 120, H: 90}, Style: resizable}, [2]int32{120, 90}},
			// A fixed surface requests its size; a fractional size reads
			// back unchanged.
			{"fixed", "gogui fixed",
				shell.SurfaceConfig{Title: "gogui fixed", Size: driver.Size{W: 250.4, H: 180},
					Style: driver.StyleTitled}, [2]int32{250, 180}},
			// The size is clamped to the limits.
			{"clamped", "gogui clamped",
				shell.SurfaceConfig{Title: "gogui clamped", Size: driver.Size{W: 900, H: 50},
					MinSize: driver.Size{W: 100, H: 80}, MaxSize: driver.Size{W: 400, H: 300}, Style: resizable},
				[2]int32{100, 80}},
		}
		var open []*surface
		for i, c := range cases {
			before := dispatch.Len()
			sf, g, h, err := e.open(c.name, c.cfg)
			if err != nil {
				return err
			}
			open = append(open, sf)
			want := shellkit.ClampSize(c.cfg.Size, c.cfg.MinSize, c.cfg.MaxSize)
			if g.win == 0 || g.surf == 0 || g.gdk == 0 {
				t.Errorf("%s: window %#x, GGSurface %#x, GdkSurface %#x; want all set", c.name, g.win, g.surf, g.gdk)
				continue
			}
			if !isTrue(surfGtk.WidgetGetRealized(g.win)) {
				t.Errorf("%s: the window is not realized", c.name)
			}
			if mapped(g.win) || isTrue(surfGtk.WidgetGetVisible(g.win)) {
				t.Errorf("%s: a new surface's window is mapped %v, visible %v; want hidden",
					c.name, mapped(g.win), isTrue(surfGtk.WidgetGetVisible(g.win)))
			}
			if child := surfGtk.WindowGetChild(g.win); child != g.surf {
				t.Errorf("%s: the window's child is %#x, want the GGSurface %#x", c.name, child, g.surf)
			}
			if typeOf(g.surf) != class.surfaceType {
				t.Errorf("%s: the child is a %s", c.name, typeName(typeOf(g.surf)))
			}
			switch tb := surfGtk.WindowGetTitlebar(g.win); {
			case tb == 0:
				t.Errorf("%s: gtk_window_get_titlebar is NULL: no CSD title bar", c.name)
			case isTrue(surfGtk.WidgetGetVisible(tb)):
				t.Errorf("%s: the title bar is visible", c.name)
			case typeOf(tb) != class.surfaceType || surfaceHandle(tb) != 0:
				t.Errorf("%s: the title bar is a %s with handle %v, want a peerless GGSurface",
					c.name, typeName(typeOf(tb)), surfaceHandle(tb))
			}
			css := csdClass(g.win)
			if css == "" {
				t.Errorf("%s: the window has neither the csd nor the solid-csd class: CSD is off", c.name)
			}
			if !isTrue(surfGtk.WindowGetDecorated(g.win)) {
				t.Errorf("%s: gtk_window_get_decorated is FALSE", c.name)
			}
			wantResizable := c.cfg.Style&driver.StyleResizable != 0
			if got := isTrue(surfGtk.WindowGetResizable(g.win)); got != wantResizable {
				t.Errorf("%s: gtk_window_get_resizable = %v, want %v", c.name, got, wantResizable)
			}
			if got := sizeRequestOf(g.surf); got != c.request {
				t.Errorf("%s: the GGSurface's size request is %v, want %v", c.name, got, c.request)
			}
			if got := defaultSizeOf(g.win); got != pxOf(want) {
				t.Errorf("%s: the default size is %v, want %v", c.name, got, pxOf(want))
			}
			if got := ffi.GoString(surfGtk.WindowGetTitle(g.win)); got != c.title {
				t.Errorf("%s: title %q, want %q", c.name, got, c.title)
			}
			if got := sf.Size(); got != want {
				t.Errorf("%s: Size() = %v, want %v", c.name, got, want)
			}
			if sc := sf.Scale(); !(sc > 0) {
				t.Errorf("%s: Scale() = %v", c.name, sc)
			}
			if sf.Zoomed() {
				t.Errorf("%s: a new surface is Zoomed", c.name)
			}
			if h.paints+h.closeReqs+h.closed+h.events+len(h.resized)+len(h.keys) != 0 {
				t.Errorf("%s: NewSurface called the handler: %+v", c.name, *h)
			}
			// The handles: the GGSurface's, and one per handler, which are
			// close-request and notify::is-active of the window, and
			// notify::state and notify::scale of the GdkSurface (4); the
			// caption gestures' (3, S4.1h), the pointer controllers' (6,
			// S4.1i), the scroll controller's (4, S4.1j), the key
			// controller's (2, S4.1k) and the input method's commit and
			// preedit signals (4, S5.3i).
			conns := 4 + 3 + 6 + 4 + 2 + 4
			if len(g.conns) != conns {
				t.Errorf("%s: %d signal handlers connected, want %d", c.name, len(g.conns), conns)
			}
			if got := dispatch.Len() - before; got != 1+len(g.conns) {
				t.Errorf("%s: NewSurface added %d handles, want %d (the GGSurface's and one per signal)",
					c.name, got, 1+len(g.conns))
			}
			if got := toplevels(); got != tops+uint32(i)+1 {
				t.Errorf("%s: %d toplevels, want %d", c.name, got, tops+uint32(i)+1)
			}
			t.Logf("%s: scale %v, CSS class %q, %d signal handlers",
				c.name, sf.Scale(), css, len(g.conns))
			sf.SetTitle("renamed\x00!")
			if got := ffi.GoString(surfGtk.WindowGetTitle(g.win)); got != "renamed!" {
				t.Errorf("%s: after SetTitle the title is %q, want %q", c.name, got, "renamed!")
			}
		}

		// Close frees every handle of the surface and destroys its window.
		for i := len(open) - 1; i >= 0; i-- {
			sf, h := open[i], e.recs[i]
			g := sf.w.(*gtkWindow)
			own := 1 + len(g.conns)
			before, topsBefore := dispatch.Len(), toplevels()
			sf.Close()
			if h.closed != 1 || !sf.dead {
				t.Errorf("%s: after Close: Closed %d times, dead %v", h.name, h.closed, sf.dead)
			}
			if got := before - dispatch.Len(); got != own {
				t.Errorf("%s: Close freed %d handles, want %d", h.name, got, own)
			}
			if got := toplevels(); got != topsBefore-1 {
				t.Errorf("%s: after Close %d toplevels, want %d: the window was not destroyed", h.name, got, topsBefore-1)
			}
			if g.win != 0 || g.surf != 0 || g.gdk != 0 || len(g.conns) != 0 {
				t.Errorf("%s: after destroy the gtkWindow still holds %#x, %#x, %#x, %d handlers",
					h.name, g.win, g.surf, g.gdk, len(g.conns))
			}
			// A closed surface ignores everything.
			size := sf.Size()
			sf.Show()
			sf.SetSize(driver.Size{W: 500, H: 500})
			sf.SetSizeLimits(driver.Size{W: 600, H: 600}, driver.Size{W: 50, H: 50})
			sf.SetTitle("dead")
			sf.Invalidate()
			sf.SetCursor(shell.CursorIBeam)
			sf.SetChromeRegions(nil)
			sf.Center()
			sf.Minimize()
			sf.Zoom()
			sf.Close()
			if sf.Zoomed() || sf.Size() != size || h.closed != 1 {
				t.Errorf("%s: a closed surface changed: Zoomed %v, Size %v (was %v), Closed %d times",
					h.name, sf.Zoomed(), sf.Size(), size, h.closed)
			}
		}
		iterateFor(settle) // nothing may reach a closed surface
		if got := toplevels(); got != tops {
			t.Errorf("at the end %d toplevels, want %d", got, tops)
		}
		if got := ffi.Callbacks(); got != numTrampolines {
			t.Errorf("ffi.Callbacks() = %d, want %d", got, numTrampolines)
		}
		return nil
	})
}

// TestSurfaceGTKShowPaint: Show maps the window, its first allocation is
// the requested size (SIZE-RESULT: whether the default size is the
// content size with CSD), Size agrees with the allocation, and Paint runs
// after Show and after every Invalidate; a hidden surface never paints.
// KeyChanged is logged (KEY-RESULT): without a window manager nothing
// activates the window.
func TestSurfaceGTKShowPaint(t *testing.T) {
	needSurfaceGTK(t)
	runSurfaceTest(t, func(e *surfTest) error {
		t := e.t
		cfg := shell.SurfaceConfig{Title: "gogui show", Size: driver.Size{W: 320, H: 240},
			MinSize: driver.Size{W: 100, H: 80}, Style: driver.StyleTitled | driver.StyleResizable}
		sf, g, h, err := e.open("shown", cfg)
		if err != nil {
			return err
		}
		hsf, _, hh, err := e.open("hidden", cfg)
		if err != nil {
			return err
		}
		start := time.Now()
		if err := show(sf, g, h); err != nil {
			return err
		}
		took := time.Since(start)
		iterateFor(settle)
		got, want := allocOf(g.surf), pxOf(cfg.Size)
		first := want // no Resized: the first allocation was the size asked for
		if len(h.resized) > 0 {
			first = pxOf(h.resized[0].size)
		}
		detail := fmt.Sprintf("window %v, default size %v, CSS class %q, Resized %v, mapped and painted after %v",
			allocOf(g.win), defaultSizeOf(g.win), csdClass(g.win), h.resized, took)
		switch {
		case got != want:
			t.Errorf("SIZE-RESULT: MISMATCH: the GGSurface settled at %v (first allocation %v) for a requested %v, "+
				"even after the first-allocation correction (%s)", got, first, want, detail)
		case first != want:
			t.Logf("SIZE-RESULT: CORRECTED: the first allocation was %v, the correction reached the requested %v (%s)", first, want, detail)
		default:
			t.Logf("SIZE-RESULT: the first allocation is the requested %v (%s)", want, detail)
		}
		// The window-vs-surface delta: what the window adds around the
		// GGSurface (solid-csd padding and border without a compositor).
		// gtkWindow.setDefaultSize adds the window's border box
		// (gtk_widget_compute_bounds) − the GGSurface's size; the content
		// boxes and the default size are logged for comparison.
		winBox, surfBox := borderBoxOf(g.win), borderBoxOf(g.surf)
		contentDelta, boxDelta, defaultDelta := sub(allocOf(g.win), got), sub(winBox, surfBox), sub(defaultSizeOf(g.win), got)
		t.Logf("SIZE-RESULT: window-vs-surface delta after Show: border boxes %v (window %v, GGSurface %v; what setDefaultSize adds), "+
			"content boxes %v (window %v), default size − GGSurface %v (CSS class %q)",
			boxDelta, winBox, surfBox, contentDelta, allocOf(g.win), defaultDelta, csdClass(g.win))
		if boxDelta != defaultDelta {
			t.Logf("SIZE-RESULT: the border-box delta %v differs from default size − GGSurface %v", boxDelta, defaultDelta)
		}
		if pxOf(sf.Size()) != got {
			t.Errorf("Size() = %v, but the GGSurface is %v", sf.Size(), got)
		}
		if sf.paints != h.paints || sf.paintErr != nil {
			t.Errorf("the surface counts %d paints (last error %v), the handler %d", sf.paints, sf.paintErr, h.paints)
		}

		for i := range 3 {
			n := h.paints
			sf.Invalidate()
			if i == 1 {
				sf.Invalidate() // coalesced into the same frame
			}
			hsf.Invalidate()
			if !iterateUntil(showWait, func() bool { return h.paints > n }) {
				t.Errorf("Invalidate %d: no paint within %v (%d paints)", i, showWait, h.paints)
			}
		}
		sf.Show() // again: nothing changes
		iterateFor(settle)
		if hh.paints != 0 || hsf.paints != 0 {
			t.Errorf("the hidden surface painted %d times", hh.paints)
		}
		if !sf.dead && mapped(g.surf) {
			t.Logf("%d paints for the shown surface", h.paints)
		}

		iterateUntil(shortWait, func() bool { return len(h.keys) > 0 })
		active := isTrue(gtk.Gtk.WindowIsActive(g.win))
		if len(h.keys) > 0 {
			t.Logf("KEY-RESULT: KeyChanged %v after Show (gtk_window_is_active %v)", h.keys, active)
			if h.keys[len(h.keys)-1] != active {
				t.Errorf("the last KeyChanged(%v) disagrees with gtk_window_is_active %v", h.keys[len(h.keys)-1], active)
			}
		} else {
			t.Logf("KEY-RESULT: no KeyChanged within %v of Show (gtk_window_is_active %v; bare xvfb has no window manager)",
				shortWait, active)
			if active {
				t.Errorf("the window is active but KeyChanged never came")
			}
		}
		if len(hh.keys) != 0 {
			t.Errorf("the hidden surface got KeyChanged %v", hh.keys)
		}
		return nil
	})
}

// resizeOutcome is what one SetSize on a mapped window did.
type resizeOutcome struct {
	reached bool
	final   [2]int32
}

// resizeAndWait calls SetSize(want) on the mapped sf, waits until the
// GGSurface is allocated want or resizeWait passes, and logs the outcome
// as RESIZE-RESULT. Size and Resized report want at once; once the
// allocations settle, Size must agree with the GGSurface whatever GTK did
// (the queued resize brings an allocation even when the size stays).
func resizeAndWait(t *testing.T, path, what string, sf *surface, g *gtkWindow, h *surfRec, want [2]int32) resizeOutcome {
	t.Helper()
	from := allocOf(g.surf)
	n := len(h.resized)
	start := time.Now()
	sf.SetSize(sizeOf(want[0], want[1]))
	if got := pxOf(sf.Size()); got != want {
		t.Errorf("%s %s: Size() right after SetSize = %v, want %v", path, what, got, want)
	}
	if len(h.resized) != n+1 || pxOf(h.resized[n].size) != want {
		t.Errorf("%s %s: SetSize reported Resized %v, want one call with %v", path, what, h.resized[n:], want)
	}
	reached := iterateUntil(resizeWait, func() bool { return sf.dead || allocOf(g.surf) == want })
	if sf.dead {
		t.Errorf("%s %s: the surface closed", path, what)
		return resizeOutcome{}
	}
	took := time.Since(start)
	iterateFor(settle)
	final := allocOf(g.surf)
	if reached {
		t.Logf("RESIZE-RESULT: %s %s %v → %v: REACHED after %v (settled at %v; window %v)",
			path, what, from, want, took.Round(time.Millisecond), final, allocOf(g.win))
	} else {
		t.Logf("RESIZE-RESULT: %s %s %v → %v: NOT REACHED within %v: the GGSurface is %v (window %v, default size %v, size request %v)",
			path, what, from, want, resizeWait, final, allocOf(g.win), defaultSizeOf(g.win), sizeRequestOf(g.surf))
	}
	if got := pxOf(sf.Size()); got != final {
		t.Errorf("%s %s: after the allocations settled Size() is %v (%v px) but the GGSurface is %v",
			path, what, sf.Size(), got, final)
	}
	t.Logf("%s %s: Resized %v", path, what, h.resized[n:])
	return resizeOutcome{reached: reached, final: final}
}

// TestSurfaceGTKResize: SetSize on a mapped window, through each
// mappedResize path (and a fixed surface, whose size request is its
// size), growing and then shrinking. Which path reaches the size is the
// open question of spike S0.9c: the outcomes are logged as RESIZE-RESULT,
// and the test fails only if neither resizable path grows the window.
func TestSurfaceGTKResize(t *testing.T) {
	needSurfaceGTK(t)
	saved := mappedResize
	t.Cleanup(func() { mappedResize = saved })
	grew := map[string]bool{}
	for _, c := range []struct {
		name      string
		mode      resizeMode
		resizable bool
	}{
		{"resizeDefaultSize", resizeDefaultSize, true},
		{"resizeSizeRequest", resizeSizeRequest, true},
		{"fixed", resizeDefaultSize, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			mappedResize = c.mode // read on the GTK goroutine, after this write (onGTK's channel)
			defer func() { mappedResize = saved }()
			runSurfaceTest(t, func(e *surfTest) error {
				t := e.t
				style := driver.StyleTitled
				minSz := driver.Size{W: 100, H: 80}
				if c.resizable {
					style |= driver.StyleResizable
				} else {
					minSz = driver.Size{}
				}
				sf, g, h, err := e.open(c.name, shell.SurfaceConfig{Title: "gogui resize " + c.name,
					Size: driver.Size{W: 320, H: 240}, MinSize: minSz, Style: style})
				if err != nil {
					return err
				}
				if err := show(sf, g, h); err != nil {
					return err
				}
				iterateFor(settle)
				a0 := allocOf(g.surf)
				out := resizeAndWait(t, c.name, "grow", sf, g, h, [2]int32{a0[0] + 120, a0[1] + 90})
				if out.final[0] > a0[0] || out.final[1] > a0[1] {
					grew[c.name] = true
				}
				if c.mode == resizeSizeRequest {
					// Once an allocation reached the forced size, the request
					// goes back to the minimum (posted from size_allocate).
					lowered := iterateUntil(shortWait, func() bool { return !sf.forced && sizeRequestOf(g.surf) == pxOf(minSz) })
					t.Logf("RESIZE-RESULT: %s after the grow: forced %v, size request %v (minimum %v), lowered %v",
						c.name, sf.forced, sizeRequestOf(g.surf), pxOf(minSz), lowered)
					if out.reached && !lowered {
						t.Errorf("%s: the grow reached its size but the size request stayed %v (forced %v), want the minimum %v",
							c.name, sizeRequestOf(g.surf), sf.forced, pxOf(minSz))
					}
				}
				if !c.resizable {
					if got, want := sizeRequestOf(g.surf), pxOf(sf.want); got != want {
						t.Errorf("fixed: the size request is %v, want the size %v", got, want)
					}
				}
				resizeAndWait(t, c.name, "shrink", sf, g, h, [2]int32{a0[0] - 60, a0[1] - 50})
				return nil
			})
		})
	}
	if !grew["resizeDefaultSize"] && !grew["resizeSizeRequest"] {
		t.Error("RESIZE-RESULT: neither resizeDefaultSize nor resizeSizeRequest grew a mapped window")
	} else {
		t.Logf("RESIZE-RESULT: grew a mapped window: resizeDefaultSize %v, resizeSizeRequest %v, fixed %v",
			grew["resizeDefaultSize"], grew["resizeSizeRequest"], grew["fixed"])
	}
}

// TestSurfaceGTKMinSize: SetSizeLimits sets the GGSurface's size
// request and clamp the size (Resized at once); a fixed surface requests
// its size; on a mapped window a raised minimum is logged (RESIZE-RESULT)
// as GTK grows the window to it.
func TestSurfaceGTKMinSize(t *testing.T) {
	needSurfaceGTK(t)
	runSurfaceTest(t, func(e *surfTest) error {
		t := e.t
		resizable := driver.StyleTitled | driver.StyleResizable
		sf, g, h, err := e.open("limits", shell.SurfaceConfig{Title: "gogui limits",
			Size: driver.Size{W: 300, H: 200}, Style: resizable})
		if err != nil {
			return err
		}
		check := func(what string, request [2]int32, size driver.Size) {
			t.Helper()
			if got := sizeRequestOf(g.surf); got != request {
				t.Errorf("%s: the size request is %v, want %v", what, got, request)
			}
			if got := sf.Size(); got != size {
				t.Errorf("%s: Size() = %v, want %v", what, got, size)
			}
			if got := defaultSizeOf(g.win); got != pxOf(size) {
				t.Errorf("%s: the default size is %v, want %v", what, got, pxOf(size))
			}
		}
		check("new", [2]int32{0, 0}, driver.Size{W: 300, H: 200})
		sf.SetSizeLimits(driver.Size{W: 150, H: 120}, driver.Size{})
		check("min below the size", [2]int32{150, 120}, driver.Size{W: 300, H: 200})
		n := len(h.resized)
		sf.SetSizeLimits(driver.Size{W: 400, H: 250}, driver.Size{})
		check("min above the size", [2]int32{400, 250}, driver.Size{W: 400, H: 250})
		if len(h.resized) != n+1 || h.resized[n].size != (driver.Size{W: 400, H: 250}) {
			t.Errorf("min above the size: Resized %v, want one call with 400×250", h.resized[n:])
		}
		sf.SetSizeLimits(driver.Size{}, driver.Size{})
		check("no limits", [2]int32{0, 0}, driver.Size{W: 400, H: 250})
		sf.SetSizeLimits(driver.Size{}, driver.Size{W: 200, H: 150})
		check("max below the size", [2]int32{0, 0}, driver.Size{W: 200, H: 150})
		sf.SetSize(driver.Size{W: 500, H: 500})
		check("SetSize above the maximum", [2]int32{0, 0}, driver.Size{W: 200, H: 150})
		sf.SetSizeLimits(driver.Size{}, driver.Size{})

		fsf, fg, _, err := e.open("fixed-limits", shell.SurfaceConfig{Title: "gogui fixed limits",
			Size: driver.Size{W: 220, H: 160}, Style: driver.StyleTitled})
		if err != nil {
			return err
		}
		if got := sizeRequestOf(fg.surf); got != [2]int32{220, 160} {
			t.Errorf("fixed: the size request is %v, want its size 220×160", got)
		}
		fsf.SetSize(driver.Size{W: 260, H: 170})
		if got := sizeRequestOf(fg.surf); got != [2]int32{260, 170} {
			t.Errorf("fixed after SetSize: the size request is %v, want 260×170", got)
		}

		// A raised minimum on a mapped window.
		sf.SetSize(driver.Size{W: 300, H: 200})
		if err := show(sf, g, h); err != nil {
			return err
		}
		iterateFor(settle)
		a0 := allocOf(g.surf)
		want := [2]int32{a0[0] + 80, a0[1] + 60}
		sf.SetSizeLimits(sizeOf(want[0], want[1]), driver.Size{})
		if got := sizeRequestOf(g.surf); got != want {
			t.Errorf("mapped raised minimum: the size request is %v, want %v", got, want)
		}
		start := time.Now()
		reached := iterateUntil(resizeWait, func() bool {
			a := allocOf(g.surf)
			return a[0] >= want[0] && a[1] >= want[1]
		})
		iterateFor(settle)
		t.Logf("RESIZE-RESULT: SetSizeLimits min %v on a mapped %v window: reached %v after %v, GGSurface %v, Size() %v",
			want, a0, reached, time.Since(start).Round(time.Millisecond), allocOf(g.surf), sf.Size())
		if got := pxOf(sf.Size()); got != allocOf(g.surf) {
			t.Errorf("mapped raised minimum: Size() %v disagrees with the GGSurface %v", got, allocOf(g.surf))
		}
		return nil
	})
}

// TestSurfaceGTKCloseRequest: gtk_window_close emits close-request, which
// reaches CloseRequested; false keeps the window, true closes the surface
// (Closed once, the window destroyed, every handle freed). Close from
// inside CloseRequested closes it once, whatever the answer.
func TestSurfaceGTKCloseRequest(t *testing.T) {
	needSurfaceGTK(t)
	runSurfaceTest(t, func(e *surfTest) error {
		t := e.t
		cfg := shell.SurfaceConfig{Title: "gogui close", Size: driver.Size{W: 240, H: 160},
			Style: driver.StyleTitled | driver.StyleClosable | driver.StyleResizable}
		sf, g, h, err := e.open("shown", cfg)
		if err != nil {
			return err
		}
		if err := show(sf, g, h); err != nil {
			return err
		}
		win := g.win
		tops := toplevels()
		h.allowClose = false
		gtk.Gtk.WindowClose(win)
		if h.closeReqs != 1 || h.closed != 0 || sf.dead {
			t.Errorf("refused: CloseRequested %d, Closed %d, dead %v; want 1, 0, false", h.closeReqs, h.closed, sf.dead)
		}
		iterateFor(settle)
		if sf.dead || toplevels() != tops || !mapped(g.surf) {
			t.Errorf("refused: the window went (dead %v, %d toplevels, want %d)", sf.dead, toplevels(), tops)
		}

		own := 1 + len(g.conns)
		before := dispatch.Len()
		h.allowClose = true
		gtk.Gtk.WindowClose(win) // win is gone after this
		if h.closeReqs != 2 || h.closed != 1 || !sf.dead {
			t.Errorf("accepted: CloseRequested %d, Closed %d, dead %v; want 2, 1, true", h.closeReqs, h.closed, sf.dead)
		}
		if got := toplevels(); got != tops-1 {
			t.Errorf("accepted: %d toplevels, want %d: the window was not destroyed", got, tops-1)
		}
		if got := before - dispatch.Len(); got != own {
			t.Errorf("accepted: %d handles freed, want %d", got, own)
		}
		if g.win != 0 {
			t.Error("accepted: the gtkWindow was not destroyed")
		}

		// Close inside CloseRequested, on hidden (realized) windows.
		for _, allow := range []bool{true, false} {
			name := fmt.Sprintf("close-inside-%v", allow)
			sf, g, h, err := e.open(name, cfg)
			if err != nil {
				return err
			}
			h.allowClose = allow
			h.onCloseRequested = func() { sf.Close() }
			tops := toplevels()
			gtk.Gtk.WindowClose(g.win)
			iterateFor(settle)
			if h.closeReqs != 1 || h.closed != 1 || !sf.dead || toplevels() != tops-1 {
				t.Errorf("%s: CloseRequested %d, Closed %d, dead %v, %d toplevels (want 1, 1, true, %d)",
					name, h.closeReqs, h.closed, sf.dead, toplevels(), tops-1)
			}
		}
		return nil
	})
}

// TestSurfaceGTKZoom: Zoom does nothing for a hidden or a fixed surface;
// on a shown resizable one it toggles maximize. Whether the window manager
// maximizes is logged (ZOOM-RESULT); bare xvfb has none, so Zoomed stays
// false there. SetSize on a maximized surface restores it first.
func TestSurfaceGTKZoom(t *testing.T) {
	needSurfaceGTK(t)
	runSurfaceTest(t, func(e *surfTest) error {
		t := e.t
		hidden, _, _, err := e.open("hidden", shell.SurfaceConfig{Title: "gogui zoom hidden",
			Size: driver.Size{W: 200, H: 150}, Style: driver.StyleTitled | driver.StyleResizable})
		if err != nil {
			return err
		}
		fsf, fg, fh, err := e.open("fixed", shell.SurfaceConfig{Title: "gogui zoom fixed",
			Size: driver.Size{W: 200, H: 150}, Style: driver.StyleTitled})
		if err != nil {
			return err
		}
		sf, g, h, err := e.open("resizable", shell.SurfaceConfig{Title: "gogui zoom",
			Size: driver.Size{W: 320, H: 240}, Style: driver.StyleTitled | driver.StyleResizable})
		if err != nil {
			return err
		}
		if err := show(fsf, fg, fh); err != nil {
			return err
		}
		if err := show(sf, g, h); err != nil {
			return err
		}
		hidden.Zoom()
		fsf.Zoom()
		iterateFor(settle)
		if hidden.Zoomed() || fsf.Zoomed() {
			t.Errorf("Zoom maximized a hidden (%v) or a fixed (%v) surface", hidden.Zoomed(), fsf.Zoomed())
		}

		a0 := allocOf(g.surf)
		n := len(h.resized)
		start := time.Now()
		sf.Zoom()
		maxed := iterateUntil(stateWait, func() bool { return sf.dead || sf.Zoomed() })
		iterateFor(settle)
		if sf.dead {
			return errors.New("the surface closed during Zoom")
		}
		if !maxed {
			t.Logf("ZOOM-RESULT: not maximized within %v (no window manager?); the GGSurface stays %v", stateWait, allocOf(g.surf))
			sf.Zoom() // asks again: no crash
			iterateFor(settle)
			return nil
		}
		t.Logf("ZOOM-RESULT: maximized after %v: the GGSurface %v → %v, Resized %v",
			time.Since(start).Round(time.Millisecond), a0, allocOf(g.surf), h.resized[n:])
		if pxOf(sf.Size()) != allocOf(g.surf) {
			t.Errorf("maximized: Size() %v disagrees with the GGSurface %v", sf.Size(), allocOf(g.surf))
		}
		sf.Zoom()
		unmaxed := iterateUntil(stateWait, func() bool { return sf.dead || !sf.Zoomed() })
		iterateFor(settle)
		t.Logf("ZOOM-RESULT: Zoom again: unmaximized %v, the GGSurface %v (was %v)", unmaxed, allocOf(g.surf), a0)
		if !unmaxed {
			t.Errorf("the second Zoom did not unmaximize within %v", stateWait)
		}
		// SetSize on a maximized surface restores it first.
		sf.Zoom()
		if iterateUntil(stateWait, func() bool { return sf.dead || sf.Zoomed() }) && !sf.dead {
			sf.SetSize(driver.Size{W: 300, H: 220})
			restored := iterateUntil(stateWait, func() bool { return sf.dead || !sf.Zoomed() })
			iterateFor(settle)
			t.Logf("ZOOM-RESULT: SetSize(300×220) on a maximized surface: unmaximized %v, the GGSurface %v", restored, allocOf(g.surf))
			if !restored {
				t.Errorf("SetSize did not unmaximize the surface within %v", stateWait)
			}
		}
		return nil
	})
}

// TestSurfaceGTKMinimize: Minimize does nothing for a hidden surface and
// does not crash on a shown one; Show restores it. Whether the window
// manager minimizes (the GdkToplevel state, X11 only) is logged
// (MINIMIZE-RESULT). A surface that is not minimized paints after
// Invalidate.
func TestSurfaceGTKMinimize(t *testing.T) {
	needSurfaceGTK(t)
	runSurfaceTest(t, func(e *surfTest) error {
		t := e.t
		cfg := shell.SurfaceConfig{Title: "gogui minimize", Size: driver.Size{W: 240, H: 160},
			Style: driver.StyleTitled | driver.StyleMiniaturizable | driver.StyleResizable}
		hidden, _, _, err := e.open("hidden", cfg)
		if err != nil {
			return err
		}
		hidden.Minimize()
		if hidden.minimized {
			t.Error("Minimize minimized a hidden surface")
		}
		sf, g, h, err := e.open("shown", cfg)
		if err != nil {
			return err
		}
		if err := show(sf, g, h); err != nil {
			return err
		}
		sf.Minimize()
		mini := iterateUntil(shortWait, func() bool { return sf.dead || sf.minimized })
		if sf.dead {
			return errors.New("the surface closed during Minimize")
		}
		t.Logf("MINIMIZE-RESULT: minimized %v within %v (mapped %v, GdkToplevel state %#x)",
			mini, shortWait, mapped(g.surf), gtk.Gdk.ToplevelGetState(g.gdk))
		sf.Minimize() // again
		sf.Show()     // gtk_window_present restores
		restored := iterateUntil(shortWait, func() bool { return sf.dead || !sf.minimized })
		iterateFor(settle)
		t.Logf("MINIMIZE-RESULT: after Show: minimized %v, mapped %v, GdkToplevel state %#x",
			!restored, mapped(g.surf), gtk.Gdk.ToplevelGetState(g.gdk))
		if mini && !restored {
			t.Errorf("Show did not restore the minimized surface within %v", shortWait)
		}
		if !sf.minimized && mapped(g.surf) {
			n := h.paints
			sf.Invalidate()
			if !iterateUntil(showWait, func() bool { return h.paints > n }) {
				t.Errorf("after the restore: no paint within %v of Invalidate", showWait)
			}
		}
		return nil
	})
}

// TestSurfaceGTKCursor: SetCursor with every cursor (and an unknown one,
// which is the arrow) sets the GGSurface's cursor by its CSS name; the
// same cursor twice does nothing. On a hidden and a shown surface.
func TestSurfaceGTKCursor(t *testing.T) {
	needSurfaceGTK(t)
	runSurfaceTest(t, func(e *surfTest) error {
		t := e.t
		cfg := shell.SurfaceConfig{Title: "gogui cursor", Size: driver.Size{W: 200, H: 150},
			Style: driver.StyleTitled | driver.StyleResizable}
		hidden, hg, _, err := e.open("hidden", cfg)
		if err != nil {
			return err
		}
		sf, g, h, err := e.open("shown", cfg)
		if err != nil {
			return err
		}
		if err := show(sf, g, h); err != nil {
			return err
		}
		cursors := []shell.Cursor{}
		for c := shell.Cursor(1); int(c) < len(cursorNames); c++ {
			cursors = append(cursors, c)
		}
		cursors = append(cursors, shell.CursorArrow, shell.Cursor(200), shell.CursorIBeam, shell.CursorIBeam)
		for _, target := range []struct {
			name string
			sf   *surface
			g    *gtkWindow
		}{{"hidden", hidden, hg}, {"shown", sf, g}} {
			for _, c := range cursors {
				target.sf.SetCursor(c)
				iterateFor(10 * time.Millisecond)
				cur := surfGtk.WidgetGetCursor(target.g.surf)
				if cur == 0 {
					t.Errorf("%s: SetCursor(%d): the GGSurface has no cursor", target.name, c)
					continue
				}
				if got := ffi.GoString(surfGtk.CursorGetName(cur)); got != cursorName(c) {
					t.Errorf("%s: SetCursor(%d): the cursor is %q, want %q", target.name, c, got, cursorName(c))
				}
				if target.sf.cursor != c {
					t.Errorf("%s: SetCursor(%d) kept %d", target.name, c, target.sf.cursor)
				}
			}
		}
		return nil
	})
}

// TestSurfaceGTKQuit: Quit with open surfaces, shown and hidden, closes
// them last to first (Closed each, no CloseRequested) and destroys their
// windows; NewSurface in a Closed handler and after Quit returns ErrQuit;
// Run returns and everything is torn down.
func TestSurfaceGTKQuit(t *testing.T) {
	needSurfaceGTK(t)
	var errInClosed error
	e := runSurfaceTest(t, func(e *surfTest) error {
		t := e.t
		tops := toplevels()
		cfg := shell.SurfaceConfig{Title: "gogui quit", Size: driver.Size{W: 200, H: 150},
			Style: driver.StyleTitled | driver.StyleResizable}
		var sfs []*surface
		for i, name := range []string{"first", "second", "third"} {
			sf, g, h, err := e.open(name, cfg)
			if err != nil {
				return err
			}
			sfs = append(sfs, sf)
			if i != 1 {
				if err := show(sf, g, h); err != nil {
					return err
				}
			}
		}
		sfs[2].SetCursor(shell.CursorPointingHand)
		sfs[0].Invalidate()
		e.recs[0].onClosed = func() {
			_, errInClosed = e.s.NewSurface(cfg, &surfRec{t: t, name: "late"})
		}
		e.s.Quit()
		for i, sf := range sfs {
			if !sf.dead {
				t.Errorf("%s is not dead after Quit", e.recs[i].name)
			}
		}
		if got := toplevels(); got != tops {
			t.Errorf("after Quit %d toplevels, want %d", got, tops)
		}
		if _, err := e.s.NewSurface(cfg, &surfRec{t: t, name: "after"}); !errors.Is(err, ErrQuit) {
			t.Errorf("NewSurface after Quit = %v, want ErrQuit", err)
		}
		return nil
	})
	if got := fmt.Sprint(e.order); got != "[third second first]" {
		t.Errorf("Closed order %s, want [third second first]", got)
	}
	for _, h := range e.recs {
		if h.closeReqs != 0 {
			t.Errorf("%s: CloseRequested %d times during Quit", h.name, h.closeReqs)
		}
	}
	if !errors.Is(errInClosed, ErrQuit) {
		t.Errorf("NewSurface in a Closed handler during Quit = %v, want ErrQuit", errInClosed)
	}
}

// TestSurfaceGTKPanicInCloseRequested: a panic in CloseRequested is
// trapped in the close-request trampoline, which still returns TRUE, so
// GTK keeps the window; the trap quits the loop and Run re-raises it as a
// *PanicError from that trampoline, after closing the surface silently.
func TestSurfaceGTKPanicInCloseRequested(t *testing.T) {
	needSurfaceGTK(t)
	s, reports := newGTKShell(t)
	base := dispatch.Len()
	boom := errors.New("boom in CloseRequested")
	var h *surfRec
	var sf *surface
	stayed, trapped := false, false
	e := &surfTest{t: t, s: s}
	p, err := runGTK(t, s, &handler{ready: func() {
		s.Post(func() {
			var g *gtkWindow
			var err error
			sf, g, h, err = e.open("panics", shell.SurfaceConfig{Title: "gogui panic",
				Size: driver.Size{W: 200, H: 150}, Style: driver.StyleTitled | driver.StyleClosable})
			if err != nil {
				t.Error(err)
				s.Quit()
				return
			}
			if err := show(sf, g, h); err != nil {
				t.Error(err)
			}
			h.allowClose = true
			h.onCloseRequested = func() { panic(boom) }
			tops := toplevels()
			gtk.Gtk.WindowClose(g.win)
			// Only reads from here: a surface method would re-raise the
			// panic at once.
			trapped = pendingPanic != nil
			stayed = !sf.dead && toplevels() == tops && mapped(g.surf)
		})
	}})
	pe, ok := p.(*PanicError)
	if !ok {
		t.Fatalf("Run = %v, panicked with %T %v; want a *PanicError", err, p, p)
	}
	if !errors.Is(pe, boom) || pe.Callback != nameSigBool {
		t.Errorf("PanicError = %v in %q, want the CloseRequested panic in %s", pe.Value, pe.Callback, nameSigBool)
	}
	if !trapped {
		t.Error("no panic was pending after gtk_window_close")
	}
	if !stayed {
		t.Error("GTK closed the window after the panicking close-request handler: the trampoline did not return TRUE")
	}
	if h == nil || sf == nil {
		t.Fatal("the surface was not created")
	}
	if !sf.dead || h.closed != 0 || h.closeReqs != 1 {
		t.Errorf("after Run: dead %v, Closed %d, CloseRequested %d; want closed silently after one request",
			sf.dead, h.closed, h.closeReqs)
	}
	if len(s.surfaces) != 0 {
		t.Errorf("%d surfaces left open after Run", len(s.surfaces))
	}
	checkGTKTornDown(t, s, reports, base)
}

// allocateBy allocates the mapped GGSurface of g d pixels smaller than
// now, directly through gtk_widget_size_allocate (after measuring, as
// GTK requires), so its size_allocate runs and reports a Resized from
// inside it, as a GTK layout would; then it queues a resize, so GTK
// allocates it again.
func allocateBy(g *gtkWindow, d [2]int32) {
	gtk.Gtk.WidgetMeasure(g.surf, gtk.OrientationHorizontal, -1, 0, 0, 0, 0)
	gtk.Gtk.WidgetMeasure(g.surf, gtk.OrientationVertical, -1, 0, 0, 0, 0)
	a := allocOf(g.surf)
	alloc := gtk.Allocation{Width: a[0] - d[0], Height: a[1] - d[1]}
	gtk.Gtk.WidgetSizeAllocate(g.surf, &alloc, -1)
	gtk.Gtk.WidgetQueueResize(g.surf) // GTK allocates it again at its next layout
}

// TestSurfaceGTKResizedReentry: SetSize, Close and Quit called from a
// Resized handler that runs inside size_allocate (Shell.inLayout) report
// at once where they report, but leave the window alone until GTK's
// allocation has returned: the native part is posted and runs at the next
// iteration.
func TestSurfaceGTKResizedReentry(t *testing.T) {
	needSurfaceGTK(t)
	runSurfaceTest(t, func(e *surfTest) error {
		t := e.t
		cfg := shell.SurfaceConfig{Title: "gogui reentry", Size: driver.Size{W: 320, H: 240},
			MinSize: driver.Size{W: 100, H: 80}, Style: driver.StyleTitled | driver.StyleResizable}
		var sfs []*surface
		var gs []*gtkWindow
		var hs []*surfRec
		for _, name := range []string{"set-size", "close", "quit"} {
			sf, g, h, err := e.open(name, cfg)
			if err != nil {
				return err
			}
			if err := show(sf, g, h); err != nil {
				return err
			}
			sfs, gs, hs = append(sfs, sf), append(gs, g), append(hs, h)
		}
		iterateFor(settle)

		// SetSize: the size is reported at once (a nested Resized), the
		// default size changes only after the allocation.
		sf, g, h := sfs[0], gs[0], hs[0]
		inLayout := -1
		h.onResized = func() {
			if inLayout < 0 {
				inLayout = e.s.inLayout
				sf.SetSize(driver.Size{W: 400, H: 300})
			}
		}
		def0 := defaultSizeOf(g.win)
		n := len(h.resized)
		allocateBy(g, [2]int32{20, 10})
		h.onResized = nil
		if inLayout <= 0 {
			t.Errorf("set-size: Resized from size_allocate ran with inLayout %d, want > 0", inLayout)
		}
		if got := sf.Size(); got != (driver.Size{W: 400, H: 300}) {
			t.Errorf("set-size: Size() = %v right after SetSize in Resized, want 400×300 (Resized %v)", got, h.resized[n:])
		}
		if got := defaultSizeOf(g.win); got != def0 {
			t.Errorf("set-size: the default size changed to %v inside the allocation, want it posted (was %v)", got, def0)
		}
		if !iterateUntil(shortWait, func() bool { return defaultSizeOf(g.win) != def0 }) {
			t.Errorf("set-size: the posted resize did not run within %v (default size %v)", shortWait, def0)
		}
		reached := iterateUntil(resizeWait, func() bool { return allocOf(g.surf) == [2]int32{400, 300} })
		t.Logf("set-size: SetSize(400×300) from Resized (after a hand-made allocation, so not a RESIZE-RESULT): default size %v → %v, reached %v, GGSurface %v, Resized %v",
			def0, defaultSizeOf(g.win), reached, allocOf(g.surf), h.resized[n:])

		// Close: the window stays until the allocation has returned.
		sf, g, h = sfs[1], gs[1], hs[1]
		h.onResized = func() { sf.Close() }
		tops := toplevels()
		allocateBy(g, [2]int32{20, 10})
		if sf.dead || h.closed != 0 || toplevels() != tops {
			t.Errorf("close: inside the allocation dead %v, Closed %d, %d toplevels (want %d): Close was not posted",
				sf.dead, h.closed, toplevels(), tops)
		}
		if !iterateUntil(shortWait, func() bool { return sf.dead }) {
			t.Errorf("close: the posted Close did not run within %v", shortWait)
		} else if h.closed != 1 || toplevels() != tops-1 {
			t.Errorf("close: after the posted Close: Closed %d, %d toplevels (want 1, %d)", h.closed, toplevels(), tops-1)
		}

		// Quit: posted the same way; then it closes the rest.
		sf, g, h = sfs[2], gs[2], hs[2]
		h.onResized = func() { e.s.Quit() }
		allocateBy(g, [2]int32{20, 10})
		if e.s.quit || sf.dead {
			t.Errorf("quit: inside the allocation quit %v, dead %v: Quit was not posted", e.s.quit, sf.dead)
		}
		if !iterateUntil(shortWait, func() bool { return e.s.quit }) {
			t.Errorf("quit: the posted Quit did not run within %v", shortWait)
		}
		for i, sf := range sfs {
			if !sf.dead {
				t.Errorf("%s: open after Quit", hs[i].name)
			}
		}
		return nil
	})
}
