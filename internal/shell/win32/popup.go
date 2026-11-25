//go:build windows

package win32

import (
	"math"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/shellkit"
	"github.com/radix29/gogui/internal/shell/win32/d2d"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// popup is the Windows shell.Popup (S5.5b): a WS_POPUP window of
// popupClassName (CS_DROPSHADOW, the same window procedure) owned by the
// parent's window, never in the taskbar (WS_EX_TOOLWINDOW) and never
// activated (WS_EX_NOACTIVATE, SW_SHOWNOACTIVATE, MA_NOACTIVATE), so the
// parent stays key and draws active. Its window and rendering state are the
// embedded swapTarget (swaptarget.go), as a surface's: its own composition
// swap chain with premultiplied alpha, cleared to transparent, painted by
// the frame clock after the surfaces. WS_EX_LAYERED does not mix with
// DComp, so the transparent corners come from the alpha of the chain.
//
// Placement (place): the anchor goes to screen pixels at the parent's
// scale; the popup's size and offset at the DPI of the anchor's monitor
// (MonitorFromRect), in whose work area shellkit.PlacePopup adjusts it.
// The rect reported through Placed has its X and Y in the parent's points
// and its size in the popup's own points (the size Paint gets); the two
// differ only when the popup's monitor has another scale than the parent.
// If the window lands at another DPI than its monitor's (a popup that is
// not slid into the work area may straddle two monitors), it is placed
// again at the window's DPI.
//
// Input, the grab and dismissal are in grab.go (S5.5c); Acrylic, the
// small rounded corners and Translucent in backdrop.go (S5.5d).
type popup struct {
	swapTarget // the window and its rendering state
	h          shell.PopupHandler
	parent     *surface

	cfg   shell.PopupConfig
	grab  bool
	rect  driver.Rect // the rect last reported through Placed (with swapTarget.scale)
	track driver.Rect // the grab's track area in the popup's points (grab.go); zero: none

	cursor shell.Cursor
	in     popupInput // the pointer during the grab (grab.go)

	translucent bool // the system backdrop is on (backdrop.go)

	opening    bool // inside NewPopup: a failure closes it without Closed
	placing    bool // inside place: WM_DPICHANGED is left to place
	dismissed  bool // Dismissed was called
	placements int  // Placed calls (tests)
}

var _ shell.Popup = (*popup)(nil)

// popupExStyle is the popups' extended window style: no taskbar button,
// never activated, no redirection bitmap (the chain alone shows).
const popupExStyle = w32.WS_EX_TOOLWINDOW | w32.WS_EX_NOACTIVATE | w32.WS_EX_NOREDIRECTIONBMP

// NewPopup implements shell.PopupShell. The popup's window is created
// hidden at its place, gets its swap chain, reports Placed and is shown
// with SW_SHOWNOACTIVATE; its first paint comes with the next frame. Without a
// chain (the GPU failed; a lost device is recovered and the chain tried
// once more, newChain) the window is destroyed again and the error
// returned, without Closed.
func (s *Shell) NewPopup(parent shell.Surface, cfg shell.PopupConfig, h shell.PopupHandler) (shell.Popup, error) {
	s.checkReady("NewPopup")
	if h == nil {
		panic("win32: NewPopup with a nil handler")
	}
	s.notPainting("NewPopup")
	ps, ok := parent.(*surface)
	if !ok || ps == nil || ps.s != s || ps.dead || s.quit ||
		!w32.IsWindowVisible(ps.hwnd) || w32.IsIconic(ps.hwnd) {
		return nil, shell.ErrPopupParent
	}
	if cfg.Grab && s.grab != nil {
		return nil, shell.ErrPopupGrab
	}
	inst, err := registerPopupClass()
	if err != nil {
		return nil, err
	}
	p := &popup{swapTarget: swapTarget{s: s, invalid: true}, h: h, parent: ps, cfg: cfg, grab: cfg.Grab, opening: true}
	p.win = p
	pl := p.layout(cfg, 0)
	p.dpi = pl.dpi
	p.id = targets.New(p)
	hwnd, err := w32.CreateWindowEx(popupExStyle, popupClassName, "", w32.WS_POPUP,
		pl.px.Left, pl.px.Top, pl.px.Right-pl.px.Left, pl.px.Bottom-pl.px.Top,
		ps.hwnd, 0, inst, uintptr(p.id))
	rethrow()
	if err != nil {
		targets.Free(p.id)
		return nil, err
	}
	p.hwnd = hwnd
	s.popups = append(s.popups, p)
	p.place(pl)
	if err := p.newChain(); err != nil {
		p.destroy() // still opening: no Closed
		rethrow()
		return nil, err
	}
	p.opening = false
	_, _ = p.applyBackdrop() // its failures leave it opaque
	if p.grab {
		s.grab = p
	}
	p.placements++
	p.h.Placed(p.rect, p.scale)
	if p.dead {
		return p, nil
	}
	w32.ShowWindow(hwnd, w32.SW_SHOWNOACTIVATE)
	rethrow()
	if p.dead {
		return p, nil
	}
	s.clock.RequestFrame()
	if p.grab {
		p.startGrab()
	}
	return p, nil
}

// placement is where a popup goes: its window in screen pixels, its DPI,
// and the rect for Placed.
type placement struct {
	px   w32.Rect
	dpi  uint32
	rect driver.Rect
}

// layout places cfg at dpi, or at the DPI of the anchor's monitor for 0
// (see popup).
func (p *popup) layout(cfg shell.PopupConfig, dpi uint32) placement {
	par := p.parent
	var o w32.Point
	_ = w32.ClientToScreen(par.hwnd, &o) // fails only for a destroyed window
	ps := par.px()
	ax, ay := float64(o.X)+pxRound(cfg.Anchor.X*ps), float64(o.Y)+pxRound(cfg.Anchor.Y*ps)
	aw, ah := float64(shellkit.ToPx(cfg.Anchor.W, ps)), float64(shellkit.ToPx(cfg.Anchor.H, ps))

	arc := w32.Rect{Left: int32(ax), Top: int32(ay), Right: int32(ax + max(aw, 1)), Bottom: int32(ay + max(ah, 1))}
	mon := w32.MonitorFromRect(&arc, w32.MONITOR_DEFAULTTONEAREST)
	var work driver.Rect // empty: unknown, nothing is adjusted
	if mi, err := w32.GetMonitorInfo(mon); err == nil {
		rw := mi.RcWork
		work = driver.Rect{X: float64(rw.Left), Y: float64(rw.Top),
			W: float64(rw.Right - rw.Left), H: float64(rw.Bottom - rw.Top)}
	}
	if dpi == 0 {
		dpi = par.dpi
		if x, _, err := w32.GetDpiForMonitor(mon, w32.MDT_EFFECTIVE_DPI); err == nil && x != 0 {
			dpi = x
		}
	}
	q := float64(dpi) / w32.USER_DEFAULT_SCREEN_DPI

	hint := driver.Size{W: length(cfg.Size.W), H: length(cfg.Size.H)}
	size := driver.Size{W: float64(shellkit.ToPx(hint.W, q)), H: float64(shellkit.ToPx(hint.H, q))}
	ox, oy := pxRound(cfg.Offset.X*q), pxRound(cfg.Offset.Y*q)
	r := shellkit.PlacePopup(driver.Rect{X: ax, Y: ay, W: aw, H: ah}, size,
		driver.Point{X: ox, Y: oy}, cfg.Adjust, work)

	pl := placement{dpi: dpi}
	pl.px = w32.Rect{Left: int32(r.X), Top: int32(r.Y), Right: int32(r.X + r.W), Bottom: int32(r.Y + r.H)}
	// Unadjusted, the rect is exactly the anchor plus the offset.
	pl.rect.X = (r.X - float64(o.X)) / ps
	if r.X == ax+ox {
		pl.rect.X = finite(cfg.Anchor.X) + finite(cfg.Offset.X)*q/ps
	}
	pl.rect.Y = (r.Y - float64(o.Y)) / ps
	if r.Y == ay+oy {
		pl.rect.Y = finite(cfg.Anchor.Y) + finite(cfg.Offset.Y)*q/ps
	}
	sz := shellkit.FromPx(int32(r.W), int32(r.H), q, hint)
	pl.rect.W, pl.rect.H = sz.W, sz.H
	return pl
}

// pxRound rounds a pixel position, with NaN and infinities as 0 and the
// result within ±shellkit.MaxPx.
func pxRound(v float64) float64 {
	v = finite(math.Round(v))
	return math.Max(-shellkit.MaxPx, math.Min(v, shellkit.MaxPx))
}

func finite(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

// length is a size component: negative, NaN and infinite ones count as 0.
func length(v float64) float64 {
	if !(v > 0) || math.IsInf(v, 1) {
		return 0
	}
	return v
}

// place moves the window to pl and, if it landed at another DPI, places
// it again at the window's DPI. It keeps the rect and the scale for the
// next Placed and the window's DPI and size for the rendering.
func (p *popup) place(pl placement) {
	p.placing = true
	defer func() { p.placing = false }()
	p.setPos(pl)
	if p.dead {
		return
	}
	if d := w32.GetDpiForWindow(p.hwnd); d != 0 && d != pl.dpi {
		pl = p.layout(p.cfg, d)
		p.setPos(pl)
	}
	p.dpi = pl.dpi
	p.scale = p.px()
	p.size = driver.Size{W: pl.rect.W, H: pl.rect.H}
	p.want = p.size
	p.rect = pl.rect
	p.track = p.trackRect()
}

func (p *popup) setPos(pl placement) {
	// Fails only for a destroyed window.
	_ = w32.SetWindowPos(p.hwnd, 0, pl.px.Left, pl.px.Top, pl.px.Right-pl.px.Left, pl.px.Bottom-pl.px.Top,
		w32.SWP_NOZORDER|w32.SWP_NOACTIVATE|w32.SWP_NOOWNERZORDER)
	rethrow()
}

// replace places the popup again from cfg: a new size fits the chain and
// invalidates, a new rect or scale is reported through Placed.
func (p *popup) replace(cfg shell.PopupConfig) {
	rect, scale := p.rect, p.scale
	p.cfg = cfg
	p.place(p.layout(cfg, 0))
	if p.dead {
		return
	}
	if p.in.seen {
		// The pointer stays where it is on the screen: its last position
		// follows the move into the new place's points (S5.6f), so the
		// events a handler's Reposition is followed by (an Enter's Move)
		// are in them.
		p.in.pos = p.in.pos.Add(rect.Origin()).Sub(p.rect.Origin())
	}
	if p.rect.W != rect.W || p.rect.H != rect.H || p.scale != scale {
		p.renderErr = p.fitChain()
		p.Invalidate()
	}
	if p.rect != rect || p.scale != scale {
		p.placements++
		p.h.Placed(p.rect, p.scale)
	}
}

// live checks the thread and reports whether the popup is open.
func (p *popup) live(what string) bool {
	p.s.q.Check("win32.Popup." + what)
	return !p.dead
}

// change is live for the methods that must not run during a Paint.
func (p *popup) change(what string) bool {
	if !p.live(what) {
		return false
	}
	p.s.notPainting("Popup." + what)
	return true
}

// Invalidate implements shell.Popup.
func (p *popup) Invalidate() {
	if p.live("Invalidate") {
		p.invalid = true
		p.s.clock.invalidated()
	}
}

// SetCursor implements shell.Popup. The cursor shows at once while the
// pointer is over a grab popup, else when it next moves there.
func (p *popup) SetCursor(c shell.Cursor) {
	if p.live("SetCursor") {
		p.cursor = c
		if p.grabbing() && p.in.inside {
			w32.SetCursor(cursorHandle(c))
		}
	}
}

// Reposition implements shell.Popup.
func (p *popup) Reposition(cfg shell.PopupConfig) {
	if p.change("Reposition") {
		cfg.Grab, cfg.Track = p.grab, p.cfg.Track
		p.replace(cfg)
	}
}

// Translucent implements shell.Popup: whether the system backdrop is on
// (backdrop.go). It changes only with the appearance, before the popup is
// invalidated.
func (p *popup) Translucent() bool {
	p.s.q.Check("win32.Popup.Translucent")
	return p.translucent
}

// Close implements shell.Popup: DestroyWindow, whose WM_DESTROY calls
// Closed.
func (p *popup) Close() {
	if p.change("Close") {
		p.destroy()
		rethrow()
	}
}

// destroy destroys the window; a window gone without WM_DESTROY reaching
// the popup is closed all the same.
func (p *popup) destroy() {
	// Fails only for a window that is already gone.
	_ = w32.DestroyWindow(p.hwnd)
	if !p.dead {
		p.destroyed()
	}
}

// destroyed handles WM_DESTROY: the popup is dead, leaves the shell,
// releases its rendering objects and gets Closed once (not while opening,
// nor in the teardown after a panic). The handle is freed in WM_NCDESTROY.
func (p *popup) destroyed() {
	if p.dead {
		return
	}
	p.dead, p.invalid = true, false
	s := p.s
	p.endGrab()
	for i, o := range s.popups {
		if o == p {
			s.popups = append(s.popups[:i], s.popups[i+1:]...)
			break
		}
	}
	p.releaseChain()
	if s.silentClose || p.opening {
		return
	}
	p.h.Closed()
}

// closePopupsOf closes the popups of parent, the last opened first.
func (s *Shell) closePopupsOf(parent *surface) {
	for {
		var last *popup
		for _, p := range s.popups {
			if p.parent == parent {
				last = p
			}
		}
		if last == nil {
			return
		}
		last.destroy()
		rethrow()
	}
}

// handle is the popup window's procedure.
func (p *popup) handle(h windows.HWND, msg uint32, wp, lp uintptr) (uintptr, bool) {
	if p.s.silentClose && msg != w32.WM_DESTROY {
		return 0, false // the teardown after a panic calls no handlers
	}
	switch msg {
	case w32.WM_NCCREATE:
		p.hwnd = h
	case w32.WM_MOUSEACTIVATE:
		return w32.MA_NOACTIVATE, true // a click leaves the parent key
	case w32.WM_NCHITTEST:
		if !p.grab {
			return htTransparent, true // the mouse goes to the window below
		}
	case w32.WM_MOUSEMOVE, w32.WM_CAPTURECHANGED,
		w32.WM_LBUTTONDOWN, w32.WM_LBUTTONUP, w32.WM_LBUTTONDBLCLK,
		w32.WM_RBUTTONDOWN, w32.WM_RBUTTONUP, w32.WM_RBUTTONDBLCLK,
		w32.WM_MBUTTONDOWN, w32.WM_MBUTTONUP, w32.WM_MBUTTONDBLCLK,
		w32.WM_XBUTTONDOWN, w32.WM_XBUTTONUP, w32.WM_XBUTTONDBLCLK:
		return p.mouse(h, msg, wp, lp)
	case w32.WM_MOUSEWHEEL, w32.WM_MOUSEHWHEEL:
		p.wheel(msg, wp, lp)
		return 0, true
	case w32.WM_TIMER:
		if wp == timerGesture {
			p.endGesture()
			return 0, true
		}
	case w32.WM_CANCELMODE:
		p.dismiss()
		return 0, false // DefWindowProcW releases a capture still held
	case w32.WM_SETCURSOR:
		if windows.HWND(wp) == h && loword(lp) == w32.HTCLIENT {
			w32.SetCursor(cursorHandle(p.cursor))
			return 1, true
		}
	case w32.WM_DPICHANGED:
		// The popup keeps its own place (no suggested rect); outside place
		// (a display change) it is placed again at the new DPI.
		if !p.placing && !p.opening && !p.dead {
			p.replace(p.cfg)
		}
		return 0, true
	case w32.WM_ERASEBKGND:
		return 1, true // WM_PAINT paints everything
	case w32.WM_PAINT:
		p.paint()
		return 0, true
	case w32.WM_DESTROY:
		p.destroyed()
		return 0, true
	}
	return 0, false
}

// Caps implements shell.Shell: PopupShadow, every popup has the OS's
// shadow (CS_DROPSHADOW, and DWM's with the rounded corners).
func (s *Shell) Caps() shell.Caps { return shell.Caps{PopupShadow: true} }

// paintContent implements window: the popup handler's Paint.
func (p *popup) paintContent(c *canvas) { p.h.Paint(c) }

// background implements window: transparent (S5.5b), the handler paints
// its own fill and leaves its rounded corners empty.
func (p *popup) background() d2d.ColorF { return d2d.ColorF{} }

// scrollEvent implements window: the popup takes only the scroll events
// over it during its grab (grab.go).
func (p *popup) scrollEvent(ev shell.ScrollEvent) { p.scroll(ev) }
