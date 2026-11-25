//go:build windows

package win32

import (
	"fmt"
	"log"
	"time"

	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell/win32/d2d"
	"github.com/radix29/gogui/internal/shell/win32/d3d11"
	"github.com/radix29/gogui/internal/shell/win32/dcomp"
	"github.com/radix29/gogui/internal/shell/win32/dxgi"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// A surface's rendering objects (S2.4c_2): a composition swap chain
// (flip-sequential, B8G8R8A8_UNORM, premultiplied alpha, two buffers,
// waitable), the window's DComp target with one visual showing the chain,
// and a D2D target bitmap over the back buffer. The swap chain has the
// client area's pixel size and the bitmap the window's DPI; fitChain keeps
// them there. WM_PAINT clears the back buffer through the shell's D2D
// context to the window background, runs Paint on the D2D canvas
// (canvas.go, S2.4d_3) and presents.

// chainFlags are the swap chain's flags, repeated by every ResizeBuffers.
const chainFlags = dxgi.SWAP_CHAIN_FLAG_FRAME_LATENCY_WAITABLE_OBJECT

// clearColor is the color a frame is cleared to before Paint: the aqua
// theme's ColorWindowBackground for the appearance a (the leaf package
// theme imports only internal/driver, PLAN.md S3.1b).
func clearColor(a driver.Appearance) d2d.ColorF {
	c, _ := theme.Sequoia.Color(driver.ColorWindowBackground, a)
	return colorF(c)
}

// chainSize returns the client area's size in pixels. An empty client
// area (minimized) keeps the chain's current size, or 1×1 for a new chain
// (a swap chain has no empty buffers).
func (t *swapTarget) chainSize() (w, h uint32) {
	rc, err := w32.GetClientRect(t.hwnd)
	if err != nil || rc.Right <= rc.Left || rc.Bottom <= rc.Top {
		return max(t.pxW, 1), max(t.pxH, 1)
	}
	return uint32(rc.Right - rc.Left), uint32(rc.Bottom - rc.Top)
}

// createChain creates the surface's swap chain, composition target and
// visual and the target bitmap, creating the shell's device objects
// first if there are none. On failure nothing of the surface's is left.
// It does not classify its error (lost does, in its callers: deviceLost
// calls it too).
func (t *swapTarget) createChain() (err error) {
	g, err := t.s.device()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			t.releaseChain()
		}
	}()
	w, h := t.chainSize()
	t.chain, err = g.factory.CreateSwapChainForComposition(g.d3d.Ptr, &dxgi.SwapChainDesc1{
		Width: w, Height: h, Format: dxgi.FORMAT_B8G8R8A8_UNORM,
		SampleDesc:  dxgi.SampleDesc{Count: 1},
		BufferUsage: dxgi.USAGE_RENDER_TARGET_OUTPUT, BufferCount: 2,
		Scaling: dxgi.SCALING_STRETCH, SwapEffect: dxgi.SWAP_EFFECT_FLIP_SEQUENTIAL,
		AlphaMode: dxgi.ALPHA_MODE_PREMULTIPLIED, Flags: chainFlags,
	})
	if err = t.s.inject("createChain", err); err != nil {
		return err
	}
	t.pxW, t.pxH = w, h
	t.openWaitable()
	t.presentLog, t.lastShown = [presentLogSize]presentRecord{}, 0 // a new chain counts from 1
	if t.ctarget, err = g.comp.CreateTargetForHwnd(t.hwnd, false); err != nil {
		return err
	}
	if t.visual, err = g.comp.CreateVisual(); err != nil {
		return err
	}
	if err = t.visual.SetContent(t.chain.Ptr); err != nil {
		return err
	}
	if err = t.ctarget.SetRoot(t.visual); err != nil {
		return err
	}
	if err = g.comp.Commit(); err != nil {
		return err
	}
	return t.createBitmap()
}

// newChain creates the chain of a new window (NewSurface, NewPopup; t is
// already among s.allWindows()). A lost device starts the recovery, which
// creates t's chain with every other one; if t still has none, it is
// tried once more. An error means t has no chain.
func (t *swapTarget) newChain() error {
	err := t.createChain()
	if err == nil || !isDeviceLost(err) {
		return err
	}
	t.s.deviceLost(err)
	if t.chain.IsNil() {
		return t.createChain()
	}
	t.renderErr = nil
	return nil
}

// createBitmap creates the D2D target bitmap over back buffer 0 at the
// window's DPI.
func (t *swapTarget) createBitmap() error {
	g := t.s.gpu
	surf, err := t.chain.Buffer(0, &dxgi.IID_IDXGISurface)
	if err != nil {
		return err
	}
	defer surf.Release() // the bitmap holds its own references
	dpi := float32(t.dpi)
	t.bitmap, err = g.dc.CreateBitmapFromDxgiSurface(surf, &d2d.BitmapProperties1{
		PixelFormat:   d2d.PixelFormat{Format: dxgi.FORMAT_B8G8R8A8_UNORM, AlphaMode: d2d.ALPHA_MODE_PREMULTIPLIED},
		DpiX:          dpi,
		DpiY:          dpi,
		BitmapOptions: d2d.BITMAP_OPTIONS_TARGET | d2d.BITMAP_OPTIONS_CANNOT_DRAW,
	})
	if err = t.s.inject("createBitmap", err); err != nil {
		t.s.release("ID2D1Bitmap1", &t.bitmap.Ptr) // an injected failure created one
		return err
	}
	t.bitmapDPI = t.dpi
	return nil
}

// releaseChain releases the surface's rendering objects: the composition
// tree is emptied and committed first, then every object goes with its
// last reference. It does nothing without a chain.
func (t *swapTarget) releaseChain() {
	s := t.s
	t.backdrop.release(s.leak)
	s.release("ID2D1Bitmap1", &t.bitmap.Ptr)
	if !t.visual.IsNil() {
		_ = t.visual.SetContent(0) // fails only on a lost device; released anyway
	}
	if !t.ctarget.IsNil() {
		_ = t.ctarget.SetRoot(dcomp.Visual{})
	}
	if s.gpu != nil && (!t.visual.IsNil() || !t.ctarget.IsNil()) {
		_ = s.gpu.comp.Commit()
	}
	s.release("IDCompositionVisual", &t.visual.Ptr)
	s.release("IDCompositionTarget", &t.ctarget.Ptr)
	t.closeWaitable()
	s.release("IDXGISwapChain1", &t.chain.Ptr)
	t.pxW, t.pxH, t.bitmapDPI = 0, 0, 0
}

// fitChain gives the swap chain the client area's pixel size and the
// target bitmap the window's DPI. The bitmap is released before
// ResizeBuffers (it holds the back buffer) and recreated after it. A lost
// device (from ResizeBuffers or the bitmap) starts the recovery and
// returns its error.
func (t *swapTarget) fitChain() error {
	if t.chain.IsNil() {
		return nil
	}
	w, h := t.chainSize()
	if w == t.pxW && h == t.pxH && t.dpi == t.bitmapDPI && !t.bitmap.IsNil() {
		return nil
	}
	t.s.release("ID2D1Bitmap1", &t.bitmap.Ptr)
	if w != t.pxW || h != t.pxH {
		err := t.s.inject("ResizeBuffers", t.chain.ResizeBuffers(0, w, h, dxgi.FORMAT_UNKNOWN, chainFlags))
		if err != nil {
			return t.s.lost(err)
		}
		t.pxW, t.pxH = w, h
	}
	return t.s.lost(t.createBitmap())
}

// paint handles WM_PAINT: the update region is validated and the frame is
// drawn and presented (render).
func (t *swapTarget) paint() {
	t.invalid = false
	var ps w32.PaintStruct
	if _, err := w32.BeginPaint(t.hwnd, &ps); err == nil {
		defer w32.EndPaint(t.hwnd, &ps)
	}
	if t.dead {
		return
	}
	t.paintErr, t.renderErr = t.render()
	if debugChecks {
		if t.paintErr != nil {
			log.Print(t.paintErr)
		}
		if t.renderErr != nil {
			log.Printf("win32: rendering: %v", t.renderErr)
		}
	}
}

// render draws the frame and presents it: the back buffer is cleared to
// the window background and SurfaceHandler.Paint draws on the D2D canvas,
// inside one BeginDraw/EndDraw. The canvas's resources are released after
// EndDraw, before Present (and before a device-loss recovery releases the
// context). paintErr is the recorder's contract errors (Shell.debug only);
// err is the rendering error. A lost device (from creating the chain,
// fitChain, EndDraw, Present or Commit: the policy in gpu.go) recreates
// every chain and returns its error; the repaint follows at the next
// frame. Without a chain Paint is not called.
func (t *swapTarget) render() (paintErr, err error) {
	s := t.s
	if t.chain.IsNil() {
		if err := s.lost(t.createChain()); err != nil {
			return nil, err
		}
	}
	if err := t.fitChain(); err != nil {
		return nil, err
	}
	g := s.gpu
	dc := g.dc
	ft := &s.clock.frameTimer
	// QPC: time.Now on Windows moves only at the interrupt timer's rate.
	var start, begun, painted, drawn time.Duration
	if ft.Active() {
		start = s.clock.now()
	}
	dc.SetTarget(t.bitmap.Ptr)
	dpi := float32(t.dpi)
	dc.SetDpi(dpi, dpi)
	dc.BeginDraw()
	dc.Clear(t.win.background())
	c := newCanvas(g, t.size, dpi, s.debug)
	c.target = blurTarget{bmp: t.bitmap, w: t.pxW, h: t.pxH, scratch: &t.backdrop}
	c.images = &s.images
	c.text = s.text
	// A panicking Paint still ends the frame and frees its resources; the
	// panic goes on to the window procedure.
	ended := false
	defer func() {
		if !ended {
			_ = c.finish()
			_ = dc.EndDraw()
			dc.SetTarget(0)
			c.release(s.leak)
		}
	}()
	if ft.Active() {
		begun = s.clock.now()
	}
	func() {
		s.painting = t
		defer func() { s.painting = nil }()
		t.win.paintContent(c)
	}()
	if ft.Active() {
		painted = s.clock.now()
	}
	paintErr = c.finish()
	err = s.inject("EndDraw", dc.EndDraw())
	dc.SetTarget(0) // the context lets go of the back buffer between frames
	failures := c.failures()
	c.release(s.leak)
	ended = true
	if ft.Active() {
		drawn = s.clock.now()
	}
	if err != nil {
		return paintErr, s.lost(err)
	}
	if s.hooks.onDrawn != nil {
		s.hooks.onDrawn(t)
	}
	_, err = t.chain.Present(1, 0)
	if err = s.inject("Present", err); err != nil {
		return paintErr, s.lost(err)
	}
	t.presented()
	if err := s.lost(s.inject("Commit", g.comp.Commit())); err != nil { // g is gone after a loss
		return paintErr, fmt.Errorf("win32: committing the composition: %w", err)
	}
	if s.clock.presentObserver != nil {
		t.presentFeedback()
	}
	if ft.Active() {
		ft.Painted(painted-begun, begun-start+drawn-painted, s.clock.now()-drawn)
	}
	if failures != nil {
		return paintErr, fmt.Errorf("win32: creating drawing resources: %w", failures)
	}
	return paintErr, nil
}

// backBuffer returns back buffer 0 as a texture (tests read it back).
func (t *swapTarget) backBuffer() (d3d11.Texture2D, error) {
	p, err := t.chain.Buffer(0, &d3d11.IID_ID3D11Texture2D)
	return d3d11.Texture2D{Ptr: p}, err
}
