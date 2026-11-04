//go:build windows

package win32

import (
	"fmt"
	"log"

	"golang.org/x/sys/windows"

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
func (sf *surface) chainSize() (w, h uint32) {
	rc, err := w32.GetClientRect(sf.hwnd)
	if err != nil || rc.Right <= rc.Left || rc.Bottom <= rc.Top {
		return max(sf.pxW, 1), max(sf.pxH, 1)
	}
	return uint32(rc.Right - rc.Left), uint32(rc.Bottom - rc.Top)
}

// createChain creates the surface's swap chain, composition target and
// visual and the target bitmap, creating the shell's device objects
// first if there are none. On failure nothing of the surface's is left.
func (sf *surface) createChain() (err error) {
	g, err := sf.s.device()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			sf.releaseChain()
		}
	}()
	w, h := sf.chainSize()
	sf.chain, err = g.factory.CreateSwapChainForComposition(g.d3d.Ptr, &dxgi.SwapChainDesc1{
		Width: w, Height: h, Format: dxgi.FORMAT_B8G8R8A8_UNORM,
		SampleDesc:  dxgi.SampleDesc{Count: 1},
		BufferUsage: dxgi.USAGE_RENDER_TARGET_OUTPUT, BufferCount: 2,
		Scaling: dxgi.SCALING_STRETCH, SwapEffect: dxgi.SWAP_EFFECT_FLIP_SEQUENTIAL,
		AlphaMode: dxgi.ALPHA_MODE_PREMULTIPLIED, Flags: chainFlags,
	})
	if err != nil {
		return err
	}
	sf.pxW, sf.pxH = w, h
	sf.openWaitable()
	if sf.ctarget, err = g.comp.CreateTargetForHwnd(sf.hwnd, false); err != nil {
		return err
	}
	if sf.visual, err = g.comp.CreateVisual(); err != nil {
		return err
	}
	if err = sf.visual.SetContent(sf.chain.Ptr); err != nil {
		return err
	}
	if err = sf.ctarget.SetRoot(sf.visual); err != nil {
		return err
	}
	if err = g.comp.Commit(); err != nil {
		return err
	}
	return sf.createBitmap()
}

// createBitmap creates the D2D target bitmap over back buffer 0 at the
// window's DPI.
func (sf *surface) createBitmap() error {
	g := sf.s.gpu
	surf, err := sf.chain.Buffer(0, &dxgi.IID_IDXGISurface)
	if err != nil {
		return err
	}
	defer surf.Release() // the bitmap holds its own references
	dpi := float32(sf.dpi)
	sf.bitmap, err = g.dc.CreateBitmapFromDxgiSurface(surf, &d2d.BitmapProperties1{
		PixelFormat:   d2d.PixelFormat{Format: dxgi.FORMAT_B8G8R8A8_UNORM, AlphaMode: d2d.ALPHA_MODE_PREMULTIPLIED},
		DpiX:          dpi,
		DpiY:          dpi,
		BitmapOptions: d2d.BITMAP_OPTIONS_TARGET | d2d.BITMAP_OPTIONS_CANNOT_DRAW,
	})
	if err != nil {
		return err
	}
	sf.bitmapDPI = sf.dpi
	return nil
}

// releaseChain releases the surface's rendering objects: the composition
// tree is emptied and committed first, then every object goes with its
// last reference. It does nothing without a chain.
func (sf *surface) releaseChain() {
	s := sf.s
	sf.backdrop.release(s.leak)
	s.release("ID2D1Bitmap1", &sf.bitmap.Ptr)
	if !sf.visual.IsNil() {
		_ = sf.visual.SetContent(0) // fails only on a lost device; released anyway
	}
	if !sf.ctarget.IsNil() {
		_ = sf.ctarget.SetRoot(dcomp.Visual{})
	}
	if s.gpu != nil && (!sf.visual.IsNil() || !sf.ctarget.IsNil()) {
		_ = s.gpu.comp.Commit()
	}
	s.release("IDCompositionVisual", &sf.visual.Ptr)
	s.release("IDCompositionTarget", &sf.ctarget.Ptr)
	sf.closeWaitable()
	s.release("IDXGISwapChain1", &sf.chain.Ptr)
	sf.pxW, sf.pxH, sf.bitmapDPI = 0, 0, 0
}

// fitChain gives the swap chain the client area's pixel size and the
// target bitmap the window's DPI. The bitmap is released before
// ResizeBuffers (it holds the back buffer) and recreated after it. A lost
// device starts the recovery and returns its error.
func (sf *surface) fitChain() error {
	if sf.chain.IsNil() {
		return nil
	}
	w, h := sf.chainSize()
	if w == sf.pxW && h == sf.pxH && sf.dpi == sf.bitmapDPI && !sf.bitmap.IsNil() {
		return nil
	}
	sf.s.release("ID2D1Bitmap1", &sf.bitmap.Ptr)
	if w != sf.pxW || h != sf.pxH {
		err := sf.s.inject("ResizeBuffers", sf.chain.ResizeBuffers(0, w, h, dxgi.FORMAT_UNKNOWN, chainFlags))
		if err != nil {
			if isDeviceLost(err) {
				sf.s.deviceLost(err)
			}
			return err
		}
		sf.pxW, sf.pxH = w, h
	}
	return sf.createBitmap()
}

// paint handles WM_PAINT: the update region is validated and the frame is
// drawn and presented (render).
func (sf *surface) paint() {
	sf.invalid = false
	var ps w32.PaintStruct
	if _, err := w32.BeginPaint(sf.hwnd, &ps); err == nil {
		defer w32.EndPaint(sf.hwnd, &ps)
	}
	if sf.dead {
		return
	}
	sf.paintErr, sf.renderErr = sf.render()
	if debugChecks {
		if sf.paintErr != nil {
			log.Print(sf.paintErr)
		}
		if sf.renderErr != nil {
			log.Printf("win32: rendering: %v", sf.renderErr)
		}
	}
}

// render draws the frame and presents it: the back buffer is cleared to
// the window background and SurfaceHandler.Paint draws on the D2D canvas,
// inside one BeginDraw/EndDraw. The canvas's resources are released after
// EndDraw, before Present (and before a device-loss recovery releases the
// context). paintErr is the recorder's contract errors (Shell.debug only);
// err is the rendering error. A lost device (from ResizeBuffers, EndDraw
// or Present) recreates every chain and returns its error; the repaint
// follows at the next frame. Without a chain Paint is not called.
func (sf *surface) render() (paintErr, err error) {
	s := sf.s
	if sf.chain.IsNil() {
		if err := sf.createChain(); err != nil {
			return nil, err
		}
	}
	if err := sf.fitChain(); err != nil {
		return nil, err
	}
	g := s.gpu
	dc := g.dc
	dc.SetTarget(sf.bitmap.Ptr)
	dpi := float32(sf.dpi)
	dc.SetDpi(dpi, dpi)
	dc.BeginDraw()
	dc.Clear(clearColor(s.appearance))
	c := newCanvas(g, sf.size, dpi, s.debug)
	c.target = blurTarget{bmp: sf.bitmap, w: sf.pxW, h: sf.pxH, scratch: &sf.backdrop}
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
	func() {
		s.painting = sf
		defer func() { s.painting = nil }()
		sf.h.Paint(c)
	}()
	paintErr = c.finish()
	err = s.inject("EndDraw", dc.EndDraw())
	dc.SetTarget(0) // the context lets go of the back buffer between frames
	failures := c.failures()
	c.release(s.leak)
	ended = true
	if err != nil {
		if isDeviceLost(err) {
			s.deviceLost(err)
		}
		return paintErr, err
	}
	if s.onDrawn != nil {
		s.onDrawn(sf)
	}
	_, err = sf.chain.Present(1, 0)
	if err = s.inject("Present", err); err != nil {
		if isDeviceLost(err) {
			s.deviceLost(err)
		}
		return paintErr, err
	}
	sf.presented()
	if err := g.comp.Commit(); err != nil {
		return paintErr, fmt.Errorf("win32: committing the composition: %w", err)
	}
	if failures != nil {
		return paintErr, fmt.Errorf("win32: creating drawing resources: %w", failures)
	}
	return paintErr, nil
}

// backBuffer returns back buffer 0 as a texture (tests read it back).
func (sf *surface) backBuffer() (d3d11.Texture2D, error) {
	p, err := sf.chain.Buffer(0, &d3d11.IID_ID3D11Texture2D)
	return d3d11.Texture2D{Ptr: p}, err
}

// openWaitable keeps the chain's frame-latency waitable object for the
// clock, with a frame latency of 1; the waitable starts with one signal.
// Without one (no IDXGISwapChain2), the clock's hold alone paces the
// surface.
func (sf *surface) openWaitable() {
	sf.ticket, sf.debt = false, 0
	c2, err := sf.chain.SwapChain2()
	if err != nil {
		return
	}
	defer c2.Release()
	if c2.SetMaximumFrameLatency(1) == nil {
		sf.waitable = c2.FrameLatencyWaitableObject()
	}
}

// closeWaitable closes the waitable object, if any.
func (sf *surface) closeWaitable() {
	if sf.waitable != 0 {
		_ = windows.CloseHandle(sf.waitable)
		sf.waitable = 0
	}
	sf.ticket, sf.debt = false, 0
}
