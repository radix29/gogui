//go:build windows

package win32

import (
	"errors"
	"runtime"
	"testing"
	"unsafe"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/shellkit"
	"github.com/radix29/gogui/internal/shell/win32/com"
	"github.com/radix29/gogui/internal/shell/win32/d3d11"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// readBack copies the surface's back buffer into a staging texture and
// returns the BGRA bytes of the pixels at pts. It runs between EndDraw and
// Present (Shell.onDrawn), while buffer 0 holds the frame just drawn.
func readBack(t *testing.T, sf *surface, pts [][2]int) [][4]byte {
	t.Helper()
	w, h, pix := readAll(t, sf)
	out := make([][4]byte, len(pts))
	for i, p := range pts {
		x, y := min(p[0], w-1), min(p[1], h-1)
		o := (y*w + x) * 4
		out[i] = [4]byte(pix[o : o+4])
	}
	return out
}

// readAll returns the whole back buffer as tightly packed BGRA rows
// (premultiplied), read back as readBack does.
func readAll(t *testing.T, sf *surface) (w, h int, pix []byte) {
	t.Helper()
	g := sf.s.gpu
	back, err := sf.backBuffer()
	if err != nil {
		t.Fatal(err)
	}
	defer back.Release()
	sd := back.Desc()
	sd.Usage, sd.BindFlags, sd.CPUAccessFlags, sd.MiscFlags = d3d11.USAGE_STAGING, 0, d3d11.CPU_ACCESS_READ, 0
	staging, err := g.d3d.CreateTexture2D(&sd)
	if err != nil {
		t.Fatal(err)
	}
	defer staging.Release()
	g.ctx.CopyResource(staging.Ptr, back.Ptr)
	m, err := g.ctx.Map(staging.Ptr, 0, d3d11.MAP_READ, 0)
	if err != nil {
		t.Fatal(err)
	}
	w, h = int(sd.Width), int(sd.Height)
	raw := ffi.CopyBytes(m.PData, int(m.RowPitch)*(h-1)+w*4)
	g.ctx.Unmap(staging.Ptr, 0)
	pix = make([]byte, w*h*4)
	for y := range h {
		copy(pix[y*w*4:(y+1)*w*4], raw[y*int(m.RowPitch):])
	}
	return w, h, pix
}

// corners are the pixels readBack checks: the corners and the centre
// (clamped to the buffer).
var corners = [][2]int{{0, 0}, {1 << 20, 0}, {0, 1 << 20}, {1 << 20, 1 << 20}, {200, 150}}

// paintAndRead sends WM_PAINT and returns the frame's pixels at corners,
// read back before Present; ok is false if the frame was not drawn.
func paintAndRead(t *testing.T, sf *surface) (px [][4]byte, ok bool) {
	t.Helper()
	sf.s.onDrawn = func(d *surface) {
		if d == sf {
			px, ok = readBack(t, sf, corners), true
		}
	}
	defer func() { sf.s.onDrawn = nil }()
	w32.SendMessage(sf.hwnd, w32.WM_PAINT, 0, 0)
	rethrow()
	return px, ok
}

func grey(v byte) [4]byte { return [4]byte{v, v, v, 255} }

// checkPixels checks every pixel against want (±1).
func checkPixels(t *testing.T, what string, px [][4]byte, want [4]byte) {
	t.Helper()
	for i, p := range px {
		for c := range 4 {
			if d := int(p[c]) - int(want[c]); d < -1 || d > 1 {
				t.Errorf("%s: pixel %v = BGRA %v, want %v", what, corners[i], p, want)
				break
			}
		}
	}
}

// chainPx checks that the chain and the target bitmap match the window:
// the client area's pixels and the window's DPI.
func chainPx(t *testing.T, what string, sf *surface) {
	t.Helper()
	d, err := sf.chain.Desc1()
	if err != nil {
		t.Fatal(err)
	}
	w, h := clientPx(t, sf)
	if d.Width != uint32(w) || d.Height != uint32(h) || sf.pxW != d.Width || sf.pxH != d.Height {
		t.Errorf("%s: chain %d×%d (kept %d×%d), client %d×%d", what, d.Width, d.Height, sf.pxW, sf.pxH, w, h)
	}
	if x, y := sf.bitmap.Dpi(); x != float32(sf.dpi) || y != float32(sf.dpi) {
		t.Errorf("%s: bitmap DPI %v×%v, window %d", what, x, y, sf.dpi)
	}
}

// TestSurfaceBackground: a surface's WM_PAINT clears its back buffer to
// the light and the dark window background, read back from the buffer.
func TestSurfaceBackground(t *testing.T) {
	inShell(t, func(s *Shell) {
		sf := newSurface(t, s, resizable, &recorder{})
		defer sf.Close()
		if s.gpu == nil || sf.chain.IsNil() || sf.bitmap.IsNil() || sf.ctarget.IsNil() || sf.visual.IsNil() {
			t.Fatal("NewSurface left the surface without its rendering objects")
		}
		t.Logf("driver %v", s.gpu.driver)
		chainPx(t, "new surface", sf)
		for _, dark := range []bool{false, true, false} {
			s.appearance.Dark = dark
			px, ok := paintAndRead(t, sf)
			if !ok || sf.renderErr != nil {
				t.Fatalf("dark %v: drawn %v, error %v", dark, ok, sf.renderErr)
			}
			want := grey(236)
			if dark {
				want = grey(50)
			}
			checkPixels(t, "dark "+map[bool]string{false: "off", true: "on"}[dark], px, want)
		}
		if !s.gpu.dc.Target().IsNil() {
			t.Error("the context keeps the target after the frame")
		}
	})
}

// TestSurfaceChainResize: SetSize and a DPI change give the buffers the
// new pixel size and the bitmap the new DPI before Resized; the next paint
// draws the whole new buffer.
func TestSurfaceChainResize(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{}
		sf := newSurface(t, s, resizable, r)
		defer sf.Close()
		r.onResized = func(driver.Size, float64) { chainPx(t, "in Resized", sf) }
		for _, size := range []driver.Size{{W: 640, H: 480}, {W: 120.5, H: 90.25}, {W: 400, H: 300}} {
			sf.SetSize(size)
			if sf.renderErr != nil {
				t.Fatalf("SetSize(%v): %v", size, sf.renderErr)
			}
			chainPx(t, "after SetSize", sf)
			px, ok := paintAndRead(t, sf)
			if !ok {
				t.Fatalf("SetSize(%v): not drawn (%v)", size, sf.renderErr)
			}
			checkPixels(t, "after SetSize", px, background(s))
		}

		// A synthetic WM_DPICHANGED, as in TestSurfaceDPIChanged.
		size := sf.Size()
		newDPI := sf.dpi * 2
		wr, err := w32.GetWindowRect(sf.hwnd)
		if err != nil {
			t.Fatal(err)
		}
		nw, nh := sf.nonClient(w32.GetDpiForWindow(sf.hwnd))
		scale := float64(newDPI) / 96
		rc := w32.Rect{Left: wr.Left, Top: wr.Top,
			Right: wr.Left + shellkit.ToPx(size.W, scale) + nw, Bottom: wr.Top + shellkit.ToPx(size.H, scale) + nh}
		var pin runtime.Pinner
		pin.Pin(&rc)
		defer pin.Unpin()
		w32.SendMessage(sf.hwnd, w32.WM_DPICHANGED, uintptr(newDPI)<<16|uintptr(newDPI), uintptr(unsafe.Pointer(&rc)))
		rethrow()
		if sf.bitmapDPI != newDPI {
			t.Errorf("bitmap DPI %d after WM_DPICHANGED to %d", sf.bitmapDPI, newDPI)
		}
		chainPx(t, "after WM_DPICHANGED", sf)
		if px, ok := paintAndRead(t, sf); !ok {
			t.Errorf("not drawn after WM_DPICHANGED (%v)", sf.renderErr)
		} else {
			checkPixels(t, "after WM_DPICHANGED", px, background(s))
		}
		if x, y := s.gpu.dc.Dpi(); x != float32(newDPI) || y != float32(newDPI) {
			t.Errorf("context DPI %v×%v, want %d", x, y, newDPI)
		}
	})
}

// TestSurfaceDeviceLoss: a device loss injected into EndDraw, Present and
// ResizeBuffers releases and recreates the devices and every surface's
// chain, invalidates every surface, and the repaint draws again; no
// references are left behind.
func TestSurfaceDeviceLoss(t *testing.T) {
	cases := []struct {
		op string
		hr com.HRESULT
	}{
		{"EndDraw", com.D2DERR_RECREATE_TARGET},
		{"Present", com.DXGI_ERROR_DEVICE_REMOVED},
		{"Present", com.DXGI_ERROR_DEVICE_RESET},
		{"ResizeBuffers", com.DXGI_ERROR_DEVICE_REMOVED},
	}
	for _, c := range cases {
		t.Run(c.op+"/"+c.hr.Error(), func(t *testing.T) {
			inShell(t, func(s *Shell) {
				a := newSurface(t, s, resizable, &recorder{})
				defer a.Close()
				b := newSurface(t, s, resizable, &recorder{})
				defer b.Close()
				gen := s.gpuGen
				s.fault = fault{op: c.op, hr: c.hr}
				if c.op == "ResizeBuffers" {
					a.SetSize(driver.Size{W: 500, H: 350})
				} else {
					w32.SendMessage(a.hwnd, w32.WM_PAINT, 0, 0)
					rethrow()
				}
				if s.fault.op != "" {
					t.Fatal("the fault was not hit")
				}
				if !errors.Is(a.renderErr, c.hr) {
					t.Errorf("renderErr %v, want %v", a.renderErr, c.hr)
				}
				if s.losses != 1 || s.gpuGen != gen+1 || s.gpu == nil {
					t.Fatalf("losses %d, generation %d (was %d)", s.losses, s.gpuGen, gen)
				}
				if len(s.leaks) != 0 {
					t.Errorf("references left by the recovery: %v", s.leaks)
				}
				for _, sf := range []*surface{a, b} {
					if sf.chain.IsNil() || sf.bitmap.IsNil() || !sf.invalid {
						t.Errorf("surface not recreated or not invalid: chain %v, bitmap %v, invalid %v",
							!sf.chain.IsNil(), !sf.bitmap.IsNil(), sf.invalid)
					}
					chainPx(t, "recreated", sf)
				}
				for _, sf := range []*surface{a, b} {
					px, ok := paintAndRead(t, sf)
					if !ok || sf.renderErr != nil {
						t.Fatalf("repaint: drawn %v, error %v", ok, sf.renderErr)
					}
					checkPixels(t, "repaint", px, background(s))
				}
			})
		})
	}
	if !isDeviceLost(com.DXGI_ERROR_DEVICE_HUNG) || isDeviceLost(com.E_FAIL) || isDeviceLost(nil) {
		t.Error("isDeviceLost")
	}
}

// TestSurfaceGPULifetime: the first surface creates the devices, the
// others share them, the last Close releases them with every count at 0,
// and the next surface creates them again.
func TestSurfaceGPULifetime(t *testing.T) {
	inShell(t, func(s *Shell) {
		if s.gpu != nil {
			t.Fatal("GPU devices before the first surface")
		}
		a := newSurface(t, s, resizable, &recorder{})
		g := s.gpu
		b := newSurface(t, s, resizable, &recorder{})
		if s.gpu != g || s.gpuGen != 1 {
			t.Errorf("second surface: devices %p (want %p), generation %d", s.gpu, g, s.gpuGen)
		}
		a.Close()
		if s.gpu != g || !a.chain.IsNil() || !a.bitmap.IsNil() || !a.visual.IsNil() || !a.ctarget.IsNil() {
			t.Error("closing one surface: devices gone or its chain kept")
		}
		b.Close()
		if s.gpu != nil || len(s.leaks) != 0 {
			t.Errorf("after the last Close: devices %v, leaks %v", s.gpu != nil, s.leaks)
		}
		c := newSurface(t, s, resizable, &recorder{})
		if s.gpu == nil || s.gpuGen != 2 {
			t.Errorf("new surface after the last: generation %d", s.gpuGen)
		}
		c.Close()
		if s.gpu != nil || len(s.leaks) != 0 {
			t.Errorf("after Close: devices %v, leaks %v", s.gpu != nil, s.leaks)
		}
	})
}

// background is the pixel the shell clears to in s's appearance: the
// theme's ColorWindowBackground, pinned here as literal greys.
func background(s *Shell) [4]byte {
	if s.appearance.Dark {
		return grey(50)
	}
	return grey(236)
}
