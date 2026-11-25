//go:build windows

package win32

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"

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
	shadows shadowCache // shadow masks and effects across frames (shadow.go)
	res     resCache    // brushes, stop collections, stroke styles across frames (rescache.go)
	canvas  canvas      // the one canvas, reused by every paint (newCanvas)
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

// leakFunc gets an object whose release left references behind
// (Shell.leak; tests pass their own). A nil leakFunc ignores them.
type leakFunc func(what string, left uint32)

// releaseLast releases the reference *p holds, which must be the last one;
// leak (if not nil) gets the references left otherwise.
func (leak leakFunc) releaseLast(what string, p *com.Ptr) {
	if n := p.Release(); n != 0 && leak != nil {
		leak(what, n)
	}
}

// release drops every reference, dependants first. Every surface's chain
// must be gone. leak (if not nil) gets the objects whose last reference
// was not the one released here.
func (g *gpu) release(leak leakFunc) {
	last := leak.releaseLast
	g.shadows.release(last) // before the contexts
	g.res.release(last)
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
// none (on WARP if s.hooks.warp or s.fallback is set).
func (s *Shell) device() (*gpu, error) {
	if s.gpu != nil {
		return s.gpu, nil
	}
	g, err := newGPU(s.hooks.warp || s.fallback)
	if err != nil {
		return nil, fmt.Errorf("win32: creating the GPU devices: %w", err)
	}
	s.stats.gpuGen++
	g.gen = s.stats.gpuGen
	s.gpu = g
	return g, nil
}

// releaseGPU releases every surface's chain, the images' bitmaps
// (image.go) and the device objects.
func (s *Shell) releaseGPU() {
	for sf := range s.allWindows() {
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
	leakFunc(s.leak).releaseLast(what, p)
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

// The device-loss policy (review-plan B2). Every create and commit error
// of the rendering goes through lost, which starts the recovery for a lost
// device: the chain's creation (render; a new surface or popup retries it
// once, newChain), the target bitmap and ResizeBuffers (fitChain), EndDraw,
// Present and the frame's Commit (render). deviceLost recovers in one
// place, guarded against re-entry. A device lost again while it recreates
// the chains is rebuilt once more (recoverTries in all); after that the
// surfaces without a chain retry at their next paint. After lossLimit
// losses within lossWindow the shell gives up on the hardware device and
// runs on WARP (fallback) for the rest of its life, so a GPU that keeps
// hanging is not rebuilt every frame.
const (
	recoverTries = 2
	lossLimit    = 3
	lossWindow   = 30 * time.Second
)

// lost starts the recovery if err means a lost device, and returns err.
func (s *Shell) lost(err error) error {
	if isDeviceLost(err) {
		s.deviceLost(err)
	}
	return err
}

// deviceLost recovers from a lost device: every surface's chain and the
// device objects are released and recreated, and every surface is
// invalidated, so the next frame repaints the ones on screen. If the
// device is lost again while the chains are recreated, it starts over, at
// most recoverTries times in all; a surface whose chain cannot be
// recreated keeps the error and tries again at its next paint (render).
// Called while a recovery runs, it only records the cause for that one.
func (s *Shell) deviceLost(cause error) {
	if s.recovering {
		if s.relost == nil {
			s.relost = cause
		}
		return
	}
	s.recovering = true
	defer func() { s.recovering, s.relost = false, nil }()
	for try := 1; ; try++ {
		s.stats.losses++
		s.noteLoss()
		if debugChecks {
			log.Printf("win32: GPU device lost (%v); recreating", cause)
		}
		s.releaseGPU()
		s.relost = nil
		for sf := range s.allWindows() {
			if sf.dead {
				continue
			}
			sf.invalid = true
			if s.relost != nil { // lost again: the rest wait for the next try or paint
				sf.renderErr = s.relost
				continue
			}
			err := sf.createChain()
			if isDeviceLost(err) {
				s.relost = err
			}
			sf.renderErr = err
		}
		if s.relost == nil || try >= recoverTries {
			break
		}
		cause = s.relost
	}
	if len(s.surfaces) > 0 {
		s.clock.RequestFrame()
	}
}

// noteLoss records a loss and switches the shell to WARP at the lossLimit-th
// loss within lossWindow (see lost).
func (s *Shell) noteLoss() {
	now := time.Now()
	copy(s.lossAt[:], s.lossAt[1:])
	s.lossAt[len(s.lossAt)-1] = now
	first := s.lossAt[0]
	if s.fallback || first.IsZero() || now.Sub(first) > lossWindow {
		return
	}
	s.fallback = true
	if debugChecks {
		log.Printf("win32: %d GPU device losses within %v; using WARP from now on", lossLimit, lossWindow)
	}
}

// fault is the test hook for device loss: the next call named op
// ("createChain", "createBitmap", "ResizeBuffers", "EndDraw", "Present" or
// "Commit") fails with hr. Shell.hooks.hooks.faults is a queue; only its first entry
// can be hit.
type fault struct {
	op string
	hr com.HRESULT
}

// inject returns err, or the failure injected for op once if it is first
// in the queue.
func (s *Shell) inject(op string, err error) error {
	if len(s.hooks.faults) == 0 || s.hooks.faults[0].op != op {
		return err
	}
	hr := s.hooks.faults[0].hr
	s.hooks.faults = s.hooks.faults[1:]
	return &com.Error{Op: op + " (injected)", HR: hr}
}
