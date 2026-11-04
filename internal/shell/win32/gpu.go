//go:build windows

package win32

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/radix29/gogui/internal/shell/win32/com"
	"github.com/radix29/gogui/internal/shell/win32/d2d"
	"github.com/radix29/gogui/internal/shell/win32/d3d11"
	"github.com/radix29/gogui/internal/shell/win32/dcomp"
	"github.com/radix29/gogui/internal/shell/win32/dxgi"
)

// The shell's device objects (S2.4c_2). One D3D11 device (hardware with
// BGRA support, else WARP), its IDXGIDevice and IDXGIFactory2, one D2D
// factory, device and device context, a second device context for shadow
// masks (S2.4e_2: so the main context never changes target while clips or
// layers are pushed), and one DComp device serve every
// surface. They are created with the first surface (NewSurface) and
// released after the last one closes; a lost device releases and
// recreates them with every surface's chain (deviceLost). Each surface's
// own objects are in present.go.

// debugChecks logs the recorder's contract errors and the rendering
// errors (GOGUI_DEBUG=1, as in package gogui).
var debugChecks = os.Getenv("GOGUI_DEBUG") == "1"

// gpu is the shell's device objects. Every field holds one reference.
type gpu struct {
	gen     int // counts the creations of the shell's devices (tests)
	driver  d3d11.DriverType
	d3d     d3d11.Device
	ctx     d3d11.DeviceContext
	dxgi    dxgi.Device
	factory dxgi.Factory2
	d2f     d2d.Factory1
	d2dev   d2d.Device
	dc      d2d.DeviceContext
	mask    d2d.DeviceContext // draws shadow masks (shadow.go)
	comp    dcomp.Device
}

// newGPU creates the device objects. On failure nothing is left alive.
// With warp the D3D11 device is always WARP (the golden renders, S3.6c:
// one rasterizer whatever the GPU); otherwise it is the hardware device,
// else WARP (d3d11.CreateDevice), which is what the shell uses.
func newGPU(warp bool) (_ *gpu, err error) {
	g := &gpu{}
	defer func() {
		if err != nil {
			g.release(nil) // the local g: the result is already nil
		}
	}()
	if warp {
		g.driver = d3d11.DRIVER_TYPE_WARP
		g.d3d, g.ctx, _, err = d3d11.CreateDeviceOfType(d3d11.DRIVER_TYPE_WARP, d3d11.CREATE_DEVICE_BGRA_SUPPORT)
	} else {
		g.d3d, g.ctx, g.driver, err = d3d11.CreateDevice()
	}
	if err != nil {
		return nil, err
	}
	if g.dxgi, err = dxgi.DeviceFrom(g.d3d.Ptr); err != nil {
		return nil, err
	}
	adapter, err := g.dxgi.Adapter()
	if err != nil {
		return nil, err
	}
	g.factory, err = adapter.Factory()
	adapter.Release() // the device keeps the adapter, the adapter the factory
	if err != nil {
		return nil, err
	}
	if g.d2f, err = d2d.CreateFactory(d2d.DEBUG_LEVEL_NONE); err != nil {
		return nil, err
	}
	if g.d2dev, err = g.d2f.CreateDevice(g.dxgi); err != nil {
		return nil, err
	}
	if g.dc, err = g.d2dev.CreateDeviceContext(d2d.DEVICE_CONTEXT_OPTIONS_NONE); err != nil {
		return nil, err
	}
	if g.mask, err = g.d2dev.CreateDeviceContext(d2d.DEVICE_CONTEXT_OPTIONS_NONE); err != nil {
		return nil, err
	}
	if g.comp, err = dcomp.CreateDevice(g.dxgi.Ptr); err != nil {
		return nil, err
	}
	return g, nil
}

// release drops every reference, dependants first. Every surface's chain
// must be gone. leak (if not nil) gets the objects whose last reference
// was not the one released here.
func (g *gpu) release(leak func(what string, left uint32)) {
	last := func(what string, p *com.Ptr) {
		if n := p.Release(); n != 0 && leak != nil {
			leak(what, n)
		}
	}
	last("ID2D1DeviceContext (mask)", &g.mask.Ptr)
	last("ID2D1DeviceContext", &g.dc.Ptr)
	last("ID2D1Device", &g.d2dev.Ptr)
	last("ID2D1Factory1", &g.d2f.Ptr)
	last("IDCompositionDevice", &g.comp.Ptr)
	g.factory.Release() // held by the adapter until the device goes
	g.dxgi.Release()    // the D3D11 device's own object
	if !g.ctx.IsNil() {
		// D3D11 destroys released objects (swap chains among them) late;
		// this runs it now, so the device's count can reach 0.
		g.ctx.ClearState()
		g.ctx.Flush()
	}
	g.ctx.Release()
	last("ID3D11Device", &g.d3d.Ptr)
}

// device returns the shell's device objects, creating them if there are
// none (on WARP if s.warp is set).
func (s *Shell) device() (*gpu, error) {
	if s.gpu != nil {
		return s.gpu, nil
	}
	g, err := newGPU(s.warp)
	if err != nil {
		return nil, fmt.Errorf("win32: creating the GPU devices: %w", err)
	}
	s.gpuGen++
	g.gen = s.gpuGen
	s.gpu = g
	return g, nil
}

// releaseGPU releases every surface's chain, the images' bitmaps
// (image.go) and the device objects.
func (s *Shell) releaseGPU() {
	for _, sf := range s.surfaces {
		sf.releaseChain()
	}
	s.images.release(s.leak)
	if s.gpu == nil {
		return
	}
	s.gpu.release(s.leak)
	s.gpu = nil
}

// release drops the reference *p holds, which must be the last one.
func (s *Shell) release(what string, p *com.Ptr) {
	if n := p.Release(); n != 0 {
		s.leak(what, n)
	}
}

// leak records an object that outlived its last expected reference
// (tests check that there are none; GOGUI_DEBUG=1 logs them).
func (s *Shell) leak(what string, left uint32) {
	msg := fmt.Sprintf("%s: %d references left", what, left)
	s.leaks = append(s.leaks, msg)
	if debugChecks {
		log.Printf("win32: %s", msg)
	}
}

// isDeviceLost reports whether err means the GPU device is gone: a
// removed, reset or hung device from DXGI, or D2D's EndDraw asking for
// the target to be recreated.
func isDeviceLost(err error) bool {
	return errors.Is(err, com.DXGI_ERROR_DEVICE_REMOVED) || errors.Is(err, com.DXGI_ERROR_DEVICE_RESET) ||
		errors.Is(err, com.DXGI_ERROR_DEVICE_HUNG) || errors.Is(err, com.D2DERR_RECREATE_TARGET)
}

// deviceLost recovers from a lost device: every surface's chain and the
// device objects are released and recreated, and every surface is
// invalidated, so the next frame repaints the ones on screen. A surface
// whose chain cannot be recreated keeps the error and tries again at its
// next paint.
func (s *Shell) deviceLost(cause error) {
	s.losses++
	if debugChecks {
		log.Printf("win32: GPU device lost (%v); recreating", cause)
	}
	s.releaseGPU()
	for _, sf := range s.surfaces {
		if sf.dead {
			continue
		}
		sf.renderErr = sf.createChain()
		sf.invalid = true
	}
	if len(s.surfaces) > 0 {
		s.clock.RequestFrame()
	}
}

// fault is the test hook for device loss: the next call named op
// ("EndDraw", "Present" or "ResizeBuffers") fails with hr.
type fault struct {
	op string
	hr com.HRESULT
}

// inject returns err, or the injected failure for op once.
func (s *Shell) inject(op string, err error) error {
	if s.fault.op != op {
		return err
	}
	s.fault.op = ""
	return &com.Error{Op: op + " (injected)", HR: s.fault.hr}
}
