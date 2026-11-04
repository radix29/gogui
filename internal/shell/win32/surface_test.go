//go:build windows

package win32

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/shellkit"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// recorder is a shell.SurfaceHandler that records its calls.
type recorder struct {
	resized  []driver.Size
	scales   []float64
	paints   int
	closed   int
	requests int
	allow    bool // CloseRequested's answer
	keys     []bool
	events   []shell.Event

	onResized func(size driver.Size, scale float64)
	onClosed  func()
	onPaint   func(c render.Canvas)
}

func (r *recorder) Paint(c render.Canvas) {
	r.paints++
	if r.onPaint != nil {
		r.onPaint(c)
	}
}

func (r *recorder) Resized(size driver.Size, scale float64) {
	r.resized = append(r.resized, size)
	r.scales = append(r.scales, scale)
	if r.onResized != nil {
		r.onResized(size, scale)
	}
}

func (r *recorder) CloseRequested() bool { r.requests++; return r.allow }

func (r *recorder) Closed() {
	r.closed++
	if r.onClosed != nil {
		r.onClosed()
	}
}

func (r *recorder) KeyChanged(isKey bool) { r.keys = append(r.keys, isKey) }
func (r *recorder) Event(ev shell.Event) (handled bool) {
	r.events = append(r.events, ev)
	return false
}

// inShell runs fn in Ready of a new shell and quits afterwards. Once Run
// has returned, the GPU devices must be gone with no references left
// (S2.4c_2).
func inShell(t *testing.T, fn func(s *Shell)) {
	t.Helper()
	s := newTestShell()
	err := run(t, s, &handler{ready: func() {
		defer s.Quit()
		fn(s)
	}})
	if err != nil {
		t.Fatal(err)
	}
	if s.gpu != nil || len(s.leaks) != 0 {
		t.Errorf("after Run: GPU devices %v, leaks %v", s.gpu != nil, s.leaks)
	}
}

func newSurface(t *testing.T, s *Shell, cfg shell.SurfaceConfig, r *recorder) *surface {
	t.Helper()
	sf, err := s.NewSurface(cfg, r)
	if err != nil {
		t.Fatalf("NewSurface(%+v) = %v", cfg, err)
	}
	return sf.(*surface)
}

// clientPx returns the window's client size in pixels.
func clientPx(t *testing.T, sf *surface) (w, h int32) {
	t.Helper()
	rc, err := w32.GetClientRect(sf.hwnd)
	if err != nil {
		t.Fatal(err)
	}
	return rc.Right - rc.Left, rc.Bottom - rc.Top
}

var resizable = shell.SurfaceConfig{Size: driver.Size{W: 400, H: 300},
	Style: driver.StyleTitled | driver.StyleClosable | driver.StyleMiniaturizable | driver.StyleResizable}

func TestSurfaceStyle(t *testing.T) {
	st, ex := surfaceStyle(driver.StyleTitled | driver.StyleClosable | driver.StyleMiniaturizable | driver.StyleResizable)
	if st != w32.WS_OVERLAPPEDWINDOW || ex != w32.WS_EX_APPWINDOW|w32.WS_EX_NOREDIRECTIONBMP {
		t.Errorf("resizable style %#x %#x", st, ex)
	}
	st, _ = surfaceStyle(driver.StyleTitled | driver.StyleClosable)
	if st&(w32.WS_THICKFRAME|w32.WS_MAXIMIZEBOX|w32.WS_MINIMIZEBOX) != 0 || st&w32.WS_SYSMENU == 0 {
		t.Errorf("fixed style %#x", st)
	}
}

// TestSurfaceCreate: a new surface is hidden, has the requested size at
// the window's DPI, a live handle, and gets no Resized for its initial size.
func TestSurfaceCreate(t *testing.T) {
	inShell(t, func(s *Shell) {
		before := targets.Len()
		r := &recorder{}
		cfg := resizable
		cfg.Title = "gogui test \x00surface"
		sf := newSurface(t, s, cfg, r)
		if w32.IsWindowVisible(sf.hwnd) {
			t.Error("new surface is visible")
		}
		if !targets.Valid(handleOf(sf.hwnd)) || handleOf(sf.hwnd) != sf.id || targets.Len() != before+1 {
			t.Error("surface handle not in GWLP_USERDATA")
		}
		dpi := w32.GetDpiForWindow(sf.hwnd)
		if want := float64(dpi) / 96; sf.Scale() != want {
			t.Errorf("Scale() = %v, want %v (DPI %d)", sf.Scale(), want, dpi)
		}
		if sf.Size() != cfg.Size {
			t.Errorf("Size() = %v, want %v", sf.Size(), cfg.Size)
		}
		w, h := clientPx(t, sf)
		if w != shellkit.ToPx(400, sf.Scale()) || h != shellkit.ToPx(300, sf.Scale()) {
			t.Errorf("client %d × %d px at scale %v", w, h, sf.Scale())
		}
		if len(r.resized) != 0 {
			t.Errorf("Resized called for the initial size: %v", r.resized)
		}
		t.Logf("host scale %v", sf.Scale())
		sf.Close()
		if targets.Len() != before {
			t.Error("handle not freed after Close")
		}
	})
}

// TestSurfaceSizeRoundTrip: SetSize calls Resized before it returns, and
// Size reads back exactly what was set, fractional points included.
func TestSurfaceSizeRoundTrip(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{}
		sf := newSurface(t, s, resizable, r)
		defer sf.Close()
		for i, want := range []driver.Size{{W: 500, H: 320}, {W: 333.3, H: 211.7}, {W: 120.25, H: 90.5}, {W: 640, H: 480}} {
			sf.SetSize(want)
			if len(r.resized) != i+1 || r.resized[i] != want {
				t.Fatalf("SetSize(%v): Resized calls %v", want, r.resized)
			}
			if sf.Size() != want {
				t.Errorf("Size() = %v after SetSize(%v)", sf.Size(), want)
			}
			w, h := clientPx(t, sf)
			if w != shellkit.ToPx(want.W, sf.Scale()) || h != shellkit.ToPx(want.H, sf.Scale()) {
				t.Errorf("SetSize(%v): client %d × %d px", want, w, h)
			}
		}
		n := len(r.resized)
		sf.SetSize(sf.Size()) // no change, no Resized
		if len(r.resized) != n {
			t.Error("Resized for an unchanged size")
		}
		if !sf.invalid {
			t.Error("not invalidated by a resize")
		}
	})
}

// TestSurfaceMinMax: the limits clamp SetSize, a new limit resizes the
// surface, and Windows applies them (WM_GETMINMAXINFO) to a direct
// SetWindowPos as well.
func TestSurfaceMinMax(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{}
		cfg := resizable
		cfg.MinSize = driver.Size{W: 200, H: 150}
		cfg.MaxSize = driver.Size{W: 800, H: 600}
		sf := newSurface(t, s, cfg, r)
		defer sf.Close()
		sf.SetSize(driver.Size{W: 50, H: 1000})
		if sf.Size() != (driver.Size{W: 200, H: 600}) {
			t.Errorf("SetSize clamped to %v, want 200 × 600", sf.Size())
		}
		sf.SetMinSize(driver.Size{W: 300, H: 100})
		if sf.Size() != (driver.Size{W: 300, H: 600}) || r.resized[len(r.resized)-1] != sf.Size() {
			t.Errorf("after SetMinSize: %v, Resized %v", sf.Size(), r.resized)
		}
		sf.SetMaxSize(driver.Size{W: 500, H: 400})
		if sf.Size() != (driver.Size{W: 300, H: 400}) {
			t.Errorf("after SetMaxSize: %v", sf.Size())
		}
		n := len(r.resized)
		sf.SetMaxSize(driver.Size{}) // no limit: no resize
		if len(r.resized) != n {
			t.Error("Resized after removing the max size")
		}

		// The OS clamps a direct resize through WM_GETMINMAXINFO.
		sf.SetMaxSize(driver.Size{W: 500, H: 400})
		if err := w32.SetWindowPos(sf.hwnd, 0, 0, 0, 10, 10, w32.SWP_NOMOVE|w32.SWP_NOZORDER|w32.SWP_NOACTIVATE); err != nil {
			t.Fatal(err)
		}
		rethrow()
		if w, h := clientPx(t, sf); w != shellkit.ToPx(300, sf.Scale()) || h != shellkit.ToPx(100, sf.Scale()) {
			t.Errorf("direct shrink: client %d × %d px, want the minimum", w, h)
		}
		if sf.Size() != (driver.Size{W: 300, H: 100}) {
			t.Errorf("Size() = %v after the direct shrink", sf.Size())
		}
		if err := w32.SetWindowPos(sf.hwnd, 0, 0, 0, 5000, 5000, w32.SWP_NOMOVE|w32.SWP_NOZORDER|w32.SWP_NOACTIVATE); err != nil {
			t.Fatal(err)
		}
		rethrow()
		if sf.Size() != (driver.Size{W: 500, H: 400}) {
			t.Errorf("Size() = %v after the direct grow, want the maximum", sf.Size())
		}
	})
}

// TestSurfaceCloseRequested: WM_CLOSE asks CloseRequested; false keeps the
// surface, true closes it with one Closed.
func TestSurfaceCloseRequested(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{}
		sf := newSurface(t, s, resizable, r)
		w32.SendMessage(sf.hwnd, w32.WM_CLOSE, 0, 0)
		rethrow()
		if r.requests != 1 || r.closed != 0 || sf.dead {
			t.Fatalf("refused close: %d requests, %d closed, dead %v", r.requests, r.closed, sf.dead)
		}
		r.allow = true
		h := sf.hwnd
		w32.SendMessage(h, w32.WM_CLOSE, 0, 0)
		rethrow()
		if r.requests != 2 || r.closed != 1 || !sf.dead || len(s.surfaces) != 0 {
			t.Fatalf("allowed close: %d requests, %d closed, dead %v, %d surfaces", r.requests, r.closed, sf.dead, len(s.surfaces))
		}
		// A dead surface ignores everything.
		sf.Close()
		sf.SetSize(driver.Size{W: 1, H: 1})
		sf.Invalidate()
		sf.Show()
		if r.closed != 1 || len(r.resized) != 0 || sf.Zoomed() {
			t.Error("dead surface reacted")
		}
	})
}

// TestSurfaceClose: Close calls Closed once without CloseRequested, and
// Closed may close another surface and create none after Quit.
func TestSurfaceClose(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{allow: true}
		sf := newSurface(t, s, resizable, r)
		sf.Close()
		sf.Close()
		if r.requests != 0 || r.closed != 1 {
			t.Errorf("Close: %d requests, %d closed", r.requests, r.closed)
		}
		if w32.IsWindowVisible(sf.hwnd) || w32.GetDpiForWindow(sf.hwnd) != 0 {
			t.Error("window still exists after Close")
		}
	})
}

// TestQuitClosesAll: Quit closes every surface with Closed and without
// CloseRequested; NewSurface fails from then on.
func TestQuitClosesAll(t *testing.T) {
	s := newTestShell()
	before := targets.Len()
	var rs []*recorder
	var newErr error
	err := run(t, s, &handler{ready: func() {
		for range 3 {
			r := &recorder{allow: true}
			rs = append(rs, r)
			newSurface(t, s, resizable, r)
		}
		rs[0].onClosed = func() {
			_, newErr = s.NewSurface(resizable, &recorder{})
		}
		s.Quit()
		if len(s.surfaces) != 0 {
			t.Errorf("%d surfaces open after Quit", len(s.surfaces))
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range rs {
		if r.closed != 1 || r.requests != 0 {
			t.Errorf("surface %d: %d closed, %d requests", i, r.closed, r.requests)
		}
	}
	if !errors.Is(newErr, ErrQuit) {
		t.Errorf("NewSurface in Closed after Quit = %v, want ErrQuit", newErr)
	}
	if targets.Len() != before {
		t.Errorf("targets.Len() = %d after Run, want %d", targets.Len(), before)
	}
}

// TestSurfacePanicTeardown: a panic leaves surfaces open; Run destroys them
// without calling their handlers.
func TestSurfacePanicTeardown(t *testing.T) {
	s := newTestShell()
	before := targets.Len()
	r := &recorder{}
	var got any
	func() {
		defer func() { got = recover() }()
		_ = run(t, s, &handler{ready: func() {
			newSurface(t, s, resizable, r)
			panic("boom")
		}})
	}()
	if got != "boom" {
		t.Fatalf("Run panicked with %v", got)
	}
	if r.closed != 0 || len(s.surfaces) != 0 || targets.Len() != before {
		t.Errorf("teardown: %d closed, %d surfaces, targets %d (want %d)", r.closed, len(s.surfaces), targets.Len(), before)
	}
}

// TestSurfaceStaleHandle: once the handle is stale, the window's messages
// go to DefWindowProcW (WM_CLOSE destroys it without CloseRequested or
// Closed).
func TestSurfaceStaleHandle(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{}
		sf := newSurface(t, s, resizable, r)
		targets.Free(sf.id)
		s.removeSurface(sf)
		w32.SendMessage(sf.hwnd, w32.WM_CLOSE, 0, 0)
		rethrow()
		if r.requests != 0 || r.closed != 0 {
			t.Errorf("stale surface: %d requests, %d closed", r.requests, r.closed)
		}
		if w32.GetDpiForWindow(sf.hwnd) != 0 {
			t.Error("DefWindowProcW did not destroy the window on WM_CLOSE")
		}
	})
}

// TestSurfaceDPIChanged: WM_DPICHANGED applies the suggested rectangle
// and reports one Resized with the new scale and the same logical size.
func TestSurfaceDPIChanged(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{}
		sf := newSurface(t, s, resizable, r)
		defer sf.Close()
		size := sf.Size()
		newDPI := sf.dpi * 2
		wr, err := w32.GetWindowRect(sf.hwnd)
		if err != nil {
			t.Fatal(err)
		}
		// The message is synthetic: the window stays at the host DPI, so
		// Windows keeps the frame of that DPI, and the suggested rectangle
		// (which a real change computes for the new DPI) has to use it.
		nw, nh := sf.nonClient(w32.GetDpiForWindow(sf.hwnd))
		scale := float64(newDPI) / 96
		rc := w32.Rect{Left: wr.Left, Top: wr.Top,
			Right: wr.Left + shellkit.ToPx(size.W, scale) + nw, Bottom: wr.Top + shellkit.ToPx(size.H, scale) + nh}
		var pin runtime.Pinner // rc is read by the window procedure through ffi.At
		pin.Pin(&rc)
		defer pin.Unpin()
		w32.SendMessage(sf.hwnd, w32.WM_DPICHANGED, uintptr(newDPI)<<16|uintptr(newDPI), uintptr(unsafe.Pointer(&rc)))
		rethrow()
		if len(r.resized) != 1 || r.resized[0] != size || r.scales[0] != scale {
			t.Fatalf("Resized calls %v %v, want one with %v at %v", r.resized, r.scales, size, scale)
		}
		if sf.Scale() != scale || sf.Size() != size {
			t.Errorf("Scale %v, Size %v after WM_DPICHANGED", sf.Scale(), sf.Size())
		}
		if w, h := clientPx(t, sf); w != shellkit.ToPx(size.W, scale) || h != shellkit.ToPx(size.H, scale) {
			t.Errorf("client %d × %d px at the new DPI", w, h)
		}
	})
}

// TestSurfaceFixed: without StyleResizable, Zoom does nothing.
func TestSurfaceFixed(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{}
		sf := newSurface(t, s, shell.SurfaceConfig{Size: driver.Size{W: 300, H: 200}, Style: driver.StyleTitled | driver.StyleClosable}, r)
		defer sf.Close()
		sf.Zoom()
		if sf.Zoomed() || len(r.resized) != 0 {
			t.Error("Zoom of a fixed surface did something")
		}
		st, _ := w32.GetWindowLongPtr(sf.hwnd, w32.GWL_STYLE)
		if st&(w32.WS_THICKFRAME|w32.WS_MAXIMIZEBOX) != 0 {
			t.Errorf("fixed surface style %#x", st)
		}
		if sf.Size() != (driver.Size{W: 300, H: 200}) {
			t.Errorf("Size() = %v", sf.Size())
		}
	})
}

// TestSurfacePaint: a hidden surface is not painted by frames; WM_PAINT
// paints it, Paint may Invalidate, and changes during Paint panic.
func TestSurfacePaint(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{}
		sf := newSurface(t, s, resizable, r)
		defer sf.Close()
		sf.Invalidate()
		s.paintInvalid(0)
		if r.paints != 0 || !sf.invalid {
			t.Errorf("hidden surface painted by a frame (%d paints)", r.paints)
		}
		r.onPaint = func(c render.Canvas) {
			mustPanic(t, "SetSize in Paint", func() { sf.SetSize(driver.Size{W: 10, H: 10}) })
			mustPanic(t, "NewSurface in Paint", func() { _, _ = s.NewSurface(resizable, &recorder{}) })
			sf.Invalidate()
			c.Save()
		}
		w32.SendMessage(sf.hwnd, w32.WM_PAINT, 0, 0)
		rethrow()
		if r.paints != 1 || !sf.invalid || sf.paintErr == nil || s.painting != nil {
			t.Errorf("WM_PAINT: %d paints, invalid %v, error %v", r.paints, sf.invalid, sf.paintErr)
		}
	})
}

// TestSurfaceCursorTitleCenter: the smaller setters work on a hidden
// surface.
func TestSurfaceCursorTitleCenter(t *testing.T) {
	inShell(t, func(s *Shell) {
		sf := newSurface(t, s, shell.SurfaceConfig{Size: driver.Size{W: 300, H: 200}, Center: true,
			Style: driver.StyleResizable}, &recorder{})
		defer sf.Close()
		wr, _ := w32.GetWindowRect(sf.hwnd)
		mi, err := w32.GetMonitorInfo(w32.MonitorFromWindow(sf.hwnd, w32.MONITOR_DEFAULTTONEAREST))
		if err != nil {
			t.Fatal(err)
		}
		if x, y := shellkit.CenterIn(shellkit.Rect(mi.RcWork), wr.Right-wr.Left, wr.Bottom-wr.Top); wr.Left != x || wr.Top != y {
			t.Errorf("window at %d, %d, want centered at %d, %d", wr.Left, wr.Top, x, y)
		}
		sf.SetTitle("new title")
		for c := range cursorIDs {
			sf.SetCursor(shell.Cursor(c))
			if (cursorHandle(shell.Cursor(c)) == 0) != (shell.Cursor(c) == shell.CursorHidden) {
				t.Errorf("cursor %d: handle %v", c, cursorHandle(shell.Cursor(c)))
			}
		}
		if cursorHandle(200) != cursorHandle(shell.CursorArrow) {
			t.Error("unknown cursor is not the arrow")
		}
		sf.SetChromeRegions([]shell.ChromeRegion{{Rect: driver.Rect{W: 300, H: 28}, Kind: shell.RegionCaption}})
		if len(sf.regions) != 1 {
			t.Error("regions not kept")
		}
		sf.Minimize() // hidden: nothing
		if w32.IsIconic(sf.hwnd) {
			t.Error("Minimize of a hidden surface minimized it")
		}
	})
}

// TestSurfaceShowZoom shows a real window: Show, Zoom and back, Minimize
// and Show again, each reported through Resized as the spec says, and
// WM_ACTIVATE reported as KeyChanged once per change. It flashes a window
// on the desktop, so the stress run skips it.
func TestSurfaceShowZoom(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{}
		cfg := resizable
		cfg.Center = true
		sf := newSurface(t, s, cfg, r)
		defer sf.Close()
		sf.Show()
		if !w32.IsWindowVisible(sf.hwnd) || sf.Zoomed() {
			t.Fatal("Show: not visible, or zoomed")
		}
		size := sf.Size()
		sf.Zoom()
		if !sf.Zoomed() || len(r.resized) != 1 || sf.Size().W <= size.W {
			t.Fatalf("Zoom: zoomed %v, Resized %v", sf.Zoomed(), r.resized)
		}
		// Custom chrome (S2.3d): maximized, the drawable area is exactly the
		// monitor's work area (the frame Windows puts beyond the edges is
		// clipped away), and the top edge is caption, not a resize border.
		if mi, err := w32.GetMonitorInfo(w32.MonitorFromWindow(sf.hwnd, w32.MONITOR_DEFAULTTONEAREST)); err != nil {
			t.Error(err)
		} else if cl, err := sf.screenClient(); err != nil || cl != mi.RcWork {
			t.Errorf("maximized client %+v (%v), want the work area %+v", cl, err, mi.RcWork)
		}
		sf.SetChromeRegions([]shell.ChromeRegion{{Rect: driver.Rect{W: sf.Size().W, H: 28}, Kind: shell.RegionCaption}})
		top := screenOf(t, sf, sf.Size().W/2, 0)
		if got := w32.SendMessage(sf.hwnd, w32.WM_NCHITTEST, 0, lparamXY(top)); got != w32.HTCAPTION {
			t.Errorf("maximized top edge: WM_NCHITTEST = %d, want HTCAPTION", got)
		}
		sf.Zoom()
		if sf.Zoomed() || sf.Size() != size || len(r.resized) != 2 {
			t.Errorf("unzoom: zoomed %v, size %v (want %v), Resized %v", sf.Zoomed(), sf.Size(), size, r.resized)
		}
		sf.Zoom()
		sf.SetSize(driver.Size{W: 420, H: 310}) // restores first
		if sf.Zoomed() || sf.Size() != (driver.Size{W: 420, H: 310}) {
			t.Errorf("SetSize while zoomed: zoomed %v, size %v", sf.Zoomed(), sf.Size())
		}
		n := len(r.resized)
		sf.Minimize()
		if !w32.IsIconic(sf.hwnd) || len(r.resized) != n {
			t.Errorf("Minimize: iconic %v, Resized %v", w32.IsIconic(sf.hwnd), r.resized[n:])
		}
		sf.Invalidate()
		sf.Show()
		if w32.IsIconic(sf.hwnd) || sf.Size() != (driver.Size{W: 420, H: 310}) || len(r.resized) != n {
			t.Errorf("Show after Minimize: iconic %v, size %v, Resized %v", w32.IsIconic(sf.hwnd), sf.Size(), r.resized[n:])
		}

		r.keys = nil
		sf.key = false
		for _, wa := range []uintptr{w32.WA_ACTIVE, w32.WA_CLICKACTIVE, w32.WA_INACTIVE, w32.WA_INACTIVE} {
			w32.SendMessage(sf.hwnd, w32.WM_ACTIVATE, wa, 0)
			rethrow()
		}
		if len(r.keys) != 2 || !r.keys[0] || r.keys[1] {
			t.Errorf("KeyChanged calls %v, want [true false]", r.keys)
		}
	})
}

// TestSurfaceFramePaint: through the running loop, Invalidate on a shown
// surface gives one Paint at the next frame, after the frame function,
// however often it was called; a Paint that invalidates gets the next
// frame. The stress run skips it (a visible window, real frames).
func TestSurfaceFramePaint(t *testing.T) {
	s := newTestShell()
	var order []string
	err := run(t, s, &handler{ready: func() {
		r := &recorder{}
		sf := newSurface(t, s, resizable, r)
		s.Clock().SetFrameFunc(func(time.Duration) { order = append(order, "frame") })
		r.onPaint = func(render.Canvas) {
			order = append(order, "paint")
			if r.paints == 1 {
				sf.Invalidate()
				return
			}
			s.Post(func() { sf.Close(); s.Quit() })
		}
		sf.Show()
		for range 5 {
			sf.Invalidate()
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	// Show itself may paint once through WM_PAINT before the first frame.
	got := strings.Join(order, " ")
	t.Logf("order %q", got)
	if got != "frame paint frame paint" && got != "paint frame paint" {
		t.Errorf("order %q, want frame paint frame paint", got)
	}
}
