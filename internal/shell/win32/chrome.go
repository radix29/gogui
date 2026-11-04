//go:build windows

package win32

import (
	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/shellkit"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// Custom chrome (S2.3d). A surface keeps its WS_CAPTION style, so Windows
// still gives it the taskbar entry, the system menu, Snap and the
// minimize and maximize animations, but WM_NCCALCSIZE takes the caption
// away: the client area (the drawable area) reaches the top edge of the
// window, and only the OS frame on the left, right and bottom stays
// non-client. On Windows 10 and 11 that frame is the invisible resize
// border in the shadow, so the window shows nothing but the drawable area
// and the DWM border. The top resize band lies inside the client area;
// WM_NCHITTEST answers it, the other borders and the chrome regions in the
// shell, without calling a handler.

// frame is the OS window frame of a surface in pixels.
type frame struct {
	side, bottom int32 // the non-client frame left and right, and at the bottom
	band         int32 // the resize band inside the top of the client area (0: none)
}

// frameAt returns the frame of the surface at dpi. A resizable surface
// keeps the OS resize frame (SM_CXSIZEFRAME + SM_CXPADDEDBORDER, invisible
// on Windows 10 and 11) on the left, right and bottom, and gets a top
// resize band of the same height inside the client area. Without
// StyleResizable there is nothing to resize, so the client area is the
// whole window.
func (sf *surface) frameAt(dpi uint32) frame {
	if !sf.resizable {
		return frame{}
	}
	pad := w32.GetSystemMetricsForDpi(w32.SM_CXPADDEDBORDER, dpi)
	side := w32.GetSystemMetricsForDpi(w32.SM_CXSIZEFRAME, dpi) + pad
	bottom := w32.GetSystemMetricsForDpi(w32.SM_CYSIZEFRAME, dpi) + pad
	return frame{side: side, bottom: bottom, band: bottom}
}

// nonClient returns the width and height the window adds around the
// drawable area at dpi: the side frames and the bottom frame.
func (sf *surface) nonClient(dpi uint32) (w, h int32) {
	f := sf.frameAt(dpi)
	return 2 * f.side, f.bottom
}

// frameDPI returns the DPI of the window's frame: the window's own DPI,
// or the one the surface converts with when Windows doesn't know it yet.
func (sf *surface) frameDPI() uint32 {
	if sf.hwnd != 0 {
		if d := w32.GetDpiForWindow(sf.hwnd); d != 0 {
			return d
		}
	}
	return sf.dpi
}

// clientRect returns the client rectangle for the window rectangle wr:
// wr without the side and bottom frames or, maximized, without the frame
// on every side (Windows places a maximized window with its frame beyond
// the monitor's edges), clipped to work.
func clientRect(wr w32.Rect, f frame, maximized bool, work w32.Rect) w32.Rect {
	rc := w32.Rect{Left: wr.Left + f.side, Top: wr.Top, Right: wr.Right - f.side, Bottom: wr.Bottom - f.bottom}
	if maximized {
		rc.Top += f.bottom
		rc = w32.Rect{Left: max(rc.Left, work.Left), Top: max(rc.Top, work.Top),
			Right: min(rc.Right, work.Right), Bottom: min(rc.Bottom, work.Bottom)}
	}
	rc.Right, rc.Bottom = max(rc.Right, rc.Left), max(rc.Bottom, rc.Top)
	return rc
}

// ncCalcSize handles WM_NCCALCSIZE: lp is a NCCALCSIZE_PARAMS (wp TRUE)
// or a RECT (wp FALSE), whose first rectangle turns from the window into
// the client rectangle. The result is 0: the old client contents stay at
// the top left.
func (sf *surface) ncCalcSize(h windows.HWND, wp, lp uintptr) {
	var rc *w32.Rect
	if wp != 0 {
		rc = &ffi.At[w32.NCCalcSizeParams](lp).Rgrc[0]
	} else {
		rc = ffi.At[w32.Rect](lp)
	}
	maximized := w32.IsZoomed(h)
	var work w32.Rect
	if maximized {
		mi, err := w32.GetMonitorInfo(w32.MonitorFromWindow(h, w32.MONITOR_DEFAULTTONEAREST))
		if err != nil {
			maximized = false
		}
		work = mi.RcWork
	}
	*rc = clientRect(*rc, sf.frameAt(sf.frameDPI()), maximized, work)
}

// dpiScaledSize handles WM_GETDPISCALEDSIZE: the window size at the new
// DPI is the same logical drawable size plus the frame at that DPI, so
// the rectangle WM_DPICHANGED suggests keeps the logical size. (Windows
// would otherwise scale the window linearly, frame included.)
func (sf *surface) dpiScaledSize(dpi uint32, sz *w32.Size) bool {
	if dpi == 0 {
		return false
	}
	nw, nh := sf.nonClient(dpi)
	scale := float64(dpi) / w32.USER_DEFAULT_SCREEN_DPI
	sz.Cx, sz.Cy = shellkit.ToPx(sf.size.W, scale)+nw, shellkit.ToPx(sf.size.H, scale)+nh
	return true
}

// hitInput is what hitTest needs, rectangles and points in screen pixels.
type hitInput struct {
	pt     w32.Point
	window w32.Rect
	client w32.Rect
	band   int32   // the top resize band inside the client area
	corner int32   // how far a corner reaches along each edge
	scale  float64 // pixels per point
	resize bool    // resize borders: resizable and not maximized
	// outside is the answer for a point in the window but outside the
	// client area without resize borders: HTNOWHERE for the frame a
	// maximized window has beyond the monitor's edges.
	outside uintptr
	regions []shell.ChromeRegion
}

// hitTest answers WM_NCHITTEST: the resize borders first (only with
// resize; the top one is the band inside the client area, and a corner
// reaches corner pixels along both of its edges), then the chrome regions
// from last to first, else HTCLIENT. Regions are in points relative to the
// client area, compared with the point pointAt gives the pixel, the same
// one the pointer events carry.
func hitTest(in hitInput) uintptr {
	p, w, cl := in.pt, in.window, in.client
	if p.X < w.Left || p.X >= w.Right || p.Y < w.Top || p.Y >= w.Bottom {
		return w32.HTNOWHERE
	}
	if in.resize {
		left, right := p.X < cl.Left, p.X >= cl.Right
		top, bottom := p.Y < cl.Top+in.band, p.Y >= cl.Bottom
		if top || bottom {
			left = left || p.X < cl.Left+in.corner
			right = right || p.X >= cl.Right-in.corner
		}
		if left || right {
			top = top || p.Y < cl.Top+in.corner
			bottom = bottom || p.Y >= cl.Bottom-in.corner
		}
		switch {
		case top && left:
			return w32.HTTOPLEFT
		case top && right:
			return w32.HTTOPRIGHT
		case bottom && left:
			return w32.HTBOTTOMLEFT
		case bottom && right:
			return w32.HTBOTTOMRIGHT
		case top:
			return w32.HTTOP
		case bottom:
			return w32.HTBOTTOM
		case left:
			return w32.HTLEFT
		case right:
			return w32.HTRIGHT
		}
	} else if p.X < cl.Left || p.X >= cl.Right || p.Y < cl.Top || p.Y >= cl.Bottom {
		return in.outside
	}
	pos := pointAt(p, cl, in.scale)
	if k, ok := shellkit.HitRegion(in.regions, pos); ok {
		return regionCode(k)
	}
	return w32.HTCLIENT
}

// pointAt converts the screen pixel p to logical points relative to the
// client rectangle cl: the pixel's top-left corner, as Windows reports
// mouse positions.
func pointAt(p w32.Point, cl w32.Rect, scale float64) driver.Point {
	return driver.Point{X: float64(p.X-cl.Left) / scale, Y: float64(p.Y-cl.Top) / scale}
}

// regionCode maps a region kind to its hit-test code.
func regionCode(k shell.RegionKind) uintptr {
	switch k {
	case shell.RegionCaption:
		return w32.HTCAPTION
	case shell.RegionClose:
		return w32.HTCLOSE
	case shell.RegionMinimize:
		return w32.HTMINBUTTON
	case shell.RegionMaximize:
		return w32.HTMAXBUTTON
	}
	return w32.HTCLIENT
}

// isChrome reports whether a hit-test code is a chrome region (the caption
// or a traffic light), and isLight whether it is a traffic light.
func isChrome(code uintptr) bool { return code == w32.HTCAPTION || isLight(code) }

func isLight(code uintptr) bool {
	return code == w32.HTCLOSE || code == w32.HTMINBUTTON || code == w32.HTMAXBUTTON
}

// hitAt hit-tests the screen pixel p against the window as it is now.
func (sf *surface) hitAt(p w32.Point) uintptr {
	wr, err := w32.GetWindowRect(sf.hwnd)
	if err != nil {
		return w32.HTNOWHERE
	}
	cl, err := sf.screenClient()
	if err != nil {
		return w32.HTNOWHERE
	}
	maximized := w32.IsZoomed(sf.hwnd)
	f := sf.frameAt(sf.frameDPI())
	in := hitInput{pt: p, window: wr, client: cl, band: f.band, corner: 2 * f.band,
		scale: sf.px(), resize: sf.resizable && !maximized, outside: w32.HTNOWHERE,
		regions: sf.regions}
	return hitTest(in)
}

// screenClient returns the client rectangle in screen pixels.
func (sf *surface) screenClient() (w32.Rect, error) {
	rc, err := w32.GetClientRect(sf.hwnd)
	if err != nil {
		return w32.Rect{}, err
	}
	var o w32.Point
	if err := w32.ClientToScreen(sf.hwnd, &o); err != nil {
		return w32.Rect{}, err
	}
	return w32.Rect{Left: o.X, Top: o.Y, Right: o.X + rc.Right, Bottom: o.Y + rc.Bottom}, nil
}

// frameAttrs are the DWM settings of a surface's frame for an appearance.
type frameAttrs struct {
	dark   int32 // DWMWA_USE_IMMERSIVE_DARK_MODE
	corner int32 // DWMWA_WINDOW_CORNER_PREFERENCE
}

// frameAttrsFor returns the frame settings for a: the dark frame in dark
// mode, and rounded corners unless high contrast is on (Windows squares
// them then, as for its own windows).
func frameAttrsFor(a driver.Appearance) frameAttrs {
	fa := frameAttrs{corner: w32.DWMWCP_ROUND}
	if a.Dark {
		fa.dark = 1
	}
	if a.IncreaseContrast {
		fa.corner = w32.DWMWCP_DEFAULT
	}
	return fa
}

// applyFrame extends the DWM frame 1 px into the top of the client area,
// which keeps the DWM shadow and border of a window without a caption,
// and sets the dark frame and the corners for the current appearance.
// Windows 10 does not know the corner attribute (nor, before 20H1, the
// dark one), so their failures are ignored (D7); the errors are returned
// for the tests.
func (sf *surface) applyFrame() (extend, dark, corner error) {
	fa := frameAttrsFor(sf.s.appearance)
	extend = w32.DwmExtendFrameIntoClientArea(sf.hwnd, &w32.Margins{CyTopHeight: 1})
	dark = w32.DwmSetWindowAttributeInt(sf.hwnd, w32.DWMWA_USE_IMMERSIVE_DARK_MODE, fa.dark)
	corner = w32.DwmSetWindowAttributeInt(sf.hwnd, w32.DWMWA_WINDOW_CORNER_PREFERENCE, fa.corner)
	return extend, dark, corner
}
