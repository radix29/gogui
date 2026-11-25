//go:build windows

package win32

import (
	"errors"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/shellkit"
	"github.com/radix29/gogui/internal/shell/win32/com"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// popRec is a shell.PopupHandler that records its calls; log, if set,
// gets "placed", "paint", "dismissed" and "closed" in order.
type popRec struct {
	placed    []driver.Rect
	scales    []float64
	paints    int
	closed    int
	dismissed int
	events    []shell.Event
	handled   bool // Event's answer
	log       *[]string

	onPlaced    func()
	onPaint     func(c render.Canvas)
	onDismissed func()
	onEvent     func(ev shell.Event)
}

func (r *popRec) note(s string) {
	if r.log != nil {
		*r.log = append(*r.log, s)
	}
}

func (r *popRec) Paint(c render.Canvas) {
	r.paints++
	r.note("paint")
	if r.onPaint != nil {
		r.onPaint(c)
	}
}

func (r *popRec) Placed(rect driver.Rect, scale float64) {
	r.placed = append(r.placed, rect)
	r.scales = append(r.scales, scale)
	r.note("placed")
	if r.onPlaced != nil {
		r.onPlaced()
	}
}

func (r *popRec) Event(ev shell.Event) bool {
	r.events = append(r.events, ev)
	if pe, ok := ev.(shell.PointerEvent); ok {
		r.note([...]string{"move", "down", "up", "enter", "leave"}[pe.Kind])
	}
	if r.onEvent != nil {
		r.onEvent(ev)
	}
	return r.handled
}
func (r *popRec) Dismissed() {
	r.dismissed++
	r.note("dismissed")
	if r.onDismissed != nil {
		r.onDismissed()
	}
}
func (r *popRec) Closed() { r.closed++; r.note("closed") }

// shownParent returns a shown surface centered on its monitor.
func shownParent(t *testing.T, s *Shell, r *recorder) *surface {
	t.Helper()
	cfg := resizable
	cfg.Center = true
	sf := newSurface(t, s, cfg, r)
	sf.Show()
	return sf
}

func newPopup(t *testing.T, s *Shell, parent *surface, cfg shell.PopupConfig, r *popRec) *popup {
	t.Helper()
	p, err := s.NewPopup(parent, cfg, r)
	if err != nil {
		t.Fatalf("NewPopup(%+v) = %v", cfg, err)
	}
	return p.(*popup)
}

// workArea returns the work area of the monitor of sf in screen pixels.
func workArea(t *testing.T, sf *surface) w32.Rect {
	t.Helper()
	mi, err := w32.GetMonitorInfo(w32.MonitorFromWindow(sf.hwnd, w32.MONITOR_DEFAULTTONEAREST))
	if err != nil {
		t.Fatal(err)
	}
	return mi.RcWork
}

// TestShownPopupErrors: the parent and grab checks of NewPopup.
func TestShownPopupErrors(t *testing.T) {
	inShell(t, func(s *Shell) {
		var r recorder
		hidden := newSurface(t, s, shell.SurfaceConfig{Size: driver.Size{W: 200, H: 100}}, &r)
		mustPanic(t, "nil handler", func() { s.NewPopup(hidden, shell.PopupConfig{}, nil) })
		type other struct{ shell.Surface }
		for _, c := range []struct {
			what   string
			parent shell.Surface
		}{{"nil", nil}, {"nil surface", (*surface)(nil)}, {"foreign", other{}}, {"hidden", hidden}} {
			if p, err := s.NewPopup(c.parent, shell.PopupConfig{}, &popRec{}); p != nil || !errors.Is(err, shell.ErrPopupParent) {
				t.Errorf("%s parent: NewPopup = %v, %v, want ErrPopupParent", c.what, p, err)
			}
		}
		sf := shownParent(t, s, &recorder{})
		g := newPopup(t, s, sf, shell.PopupConfig{Size: driver.Size{W: 50, H: 50}, Grab: true}, &popRec{})
		if p, err := s.NewPopup(sf, shell.PopupConfig{Grab: true}, &popRec{}); p != nil || !errors.Is(err, shell.ErrPopupGrab) {
			t.Errorf("second grab popup: NewPopup = %v, %v, want ErrPopupGrab", p, err)
		}
		tip := newPopup(t, s, sf, shell.PopupConfig{Size: driver.Size{W: 20, H: 10}}, &popRec{}) // no grab: allowed
		g.Close()
		g2 := newPopup(t, s, sf, shell.PopupConfig{Grab: true}, &popRec{}) // the grab is free again
		tip.Close()
		g2.Close()
		sf.Minimize()
		if _, err := s.NewPopup(sf, shell.PopupConfig{}, &popRec{}); !errors.Is(err, shell.ErrPopupParent) {
			t.Errorf("minimized parent: %v, want ErrPopupParent", err)
		}
		sf.Close()
		if _, err := s.NewPopup(sf, shell.PopupConfig{}, &popRec{}); !errors.Is(err, shell.ErrPopupParent) {
			t.Errorf("closed parent: %v, want ErrPopupParent", err)
		}
		hidden.Close()
	})
}

// TestShownPopupWindow: the popup's window is a shown WS_POPUP of the popup
// class owned by the parent, a tool window that is never activated, so
// the parent stays key; it is placed at the anchor plus the offset at the
// monitor's DPI and reports that through Placed before its first paint.
func TestShownPopupWindow(t *testing.T) {
	inShell(t, func(s *Shell) {
		before := targets.Len()
		r := &recorder{}
		sf := shownParent(t, s, r)
		defer sf.Close()
		sf.key = true // as if WM_ACTIVATE had come (the foreground lock may refuse it)
		r.keys = nil
		var log []string
		pr := &popRec{log: &log}
		cfg := shell.PopupConfig{Anchor: driver.Rect{X: 40, Y: 30, W: 120, H: 22},
			Size: driver.Size{W: 150, H: 80}, Offset: driver.Point{X: -10, Y: -20}, Grab: true}
		p := newPopup(t, s, sf, cfg, pr)
		h := p.hwnd

		if owner, _ := w32.GetWindowLongPtr(h, w32.GWLP_HWNDPARENT); owner != uintptr(sf.hwnd) {
			t.Errorf("owner %#x, want the parent %#x", owner, sf.hwnd)
		}
		st, _ := w32.GetWindowLongPtr(h, w32.GWL_STYLE)
		ex, _ := w32.GetWindowLongPtr(h, w32.GWL_EXSTYLE)
		if uint32(st)&w32.WS_POPUP == 0 || uint32(st)&(w32.WS_CAPTION|w32.WS_THICKFRAME) != 0 ||
			uint32(ex)&popupExStyle != popupExStyle || uint32(ex)&w32.WS_EX_APPWINDOW != 0 {
			t.Errorf("style %#x, ex style %#x", st, ex)
		}
		if got := w32.FindWindowEx(0, popupClassName, ""); got == 0 {
			t.Error("no window of the popup class")
		}
		if !w32.IsWindowVisible(h) || !targets.Valid(handleOf(h)) || targets.Len() != before+2 {
			t.Errorf("visible %v, handle valid %v, %d new handles", w32.IsWindowVisible(h),
				targets.Valid(handleOf(h)), targets.Len()-before)
		}
		if got := w32.SendMessage(h, w32.WM_MOUSEACTIVATE, uintptr(sf.hwnd), w32.HTCLIENT|w32.WM_LBUTTONDOWN<<16); got != w32.MA_NOACTIVATE {
			t.Errorf("WM_MOUSEACTIVATE = %d, want MA_NOACTIVATE", got)
		}
		if len(r.keys) != 0 || !sf.key {
			t.Errorf("parent KeyChanged %v after the popup opened", r.keys)
		}

		// The place: the anchor in screen pixels plus the offset, all at
		// the parent's scale (the popup is on the parent's monitor).
		if len(pr.placed) != 1 || len(log) != 1 || log[0] != "placed" {
			t.Fatalf("Placed %v, log %v", pr.placed, log)
		}
		want := driver.Rect{X: 30, Y: 10, W: 150, H: 80}
		if pr.placed[0] != want || pr.scales[0] != sf.scale || p.dpi != sf.dpi {
			t.Errorf("Placed %v at %v (dpi %d), want %v at %v", pr.placed[0], pr.scales[0], p.dpi, want, sf.scale)
		}
		cl, err := sf.screenClient()
		if err != nil {
			t.Fatal(err)
		}
		wr, _ := w32.GetWindowRect(h)
		x, y := cl.Left+shellkit.ToPx(30, sf.px()), cl.Top+shellkit.ToPx(10, sf.px())
		if wr != (w32.Rect{Left: x, Top: y, Right: x + shellkit.ToPx(150, sf.px()), Bottom: y + shellkit.ToPx(80, sf.px())}) {
			t.Errorf("window %+v, client of the parent at %d,%d", wr, cl.Left, cl.Top)
		}
		if w, hh := clientPx(t, p); w != wr.Right-wr.Left || hh != wr.Bottom-wr.Top || p.size != (driver.Size{W: 150, H: 80}) {
			t.Errorf("client %d×%d, size %v", w, hh, p.size)
		}
		chainPx(t, "popup", p)

		// The first paint follows Placed; the parent paints first.
		r.paints = 0
		sf.invalid = true
		s.paintInvalid(0)
		if pr.paints != 1 || r.paints != 1 || log[len(log)-1] != "paint" || p.invalid {
			t.Errorf("paint pass: popup %d paints, parent %d, log %v", pr.paints, r.paints, log)
		}

		// Reposition: the same config reports nothing, a new offset a new
		// rect, a new size invalidates and fits the chain.
		p.Reposition(cfg)
		if len(pr.placed) != 1 {
			t.Errorf("Reposition to the same place: Placed %v", pr.placed)
		}
		cfg.Offset = driver.Point{X: 0, Y: 22}
		cfg.Grab = false // ignored
		p.Reposition(cfg)
		if len(pr.placed) != 2 || pr.placed[1] != (driver.Rect{X: 40, Y: 52, W: 150, H: 80}) || p.invalid || !p.grab {
			t.Errorf("Reposition by the offset: Placed %v, invalid %v, grab %v", pr.placed, p.invalid, p.grab)
		}
		cfg.Size = driver.Size{W: 90, H: 120}
		p.Reposition(cfg)
		if len(pr.placed) != 3 || pr.placed[2].W != 90 || pr.placed[2].H != 120 || !p.invalid {
			t.Errorf("Reposition by the size: Placed %v, invalid %v", pr.placed, p.invalid)
		}
		chainPx(t, "resized popup", p)

		p.SetCursor(shell.CursorPointingHand)
		if p.cursor != shell.CursorPointingHand || p.Translucent() != wantsAcrylic(s.backdrop.build, s.appearance) {
			t.Errorf("cursor %v, translucent %v", p.cursor, p.Translucent())
		}
		p.Close()
		if pr.closed != 1 || pr.dismissed != 0 || !p.dead || len(s.popups) != 0 || s.grab != nil ||
			targets.Len() != before+1 || !p.chain.IsNil() {
			t.Errorf("Close: closed %d, dismissed %d, dead %v, %d popups, grab %v, chain %v",
				pr.closed, pr.dismissed, p.dead, len(s.popups), s.grab != nil, !p.chain.IsNil())
		}
		p.Close()
		p.Invalidate()
		p.Reposition(cfg)
		if pr.closed != 1 || len(pr.placed) != 3 || p.invalid {
			t.Errorf("a closed popup reacted: closed %d, Placed %v", pr.closed, pr.placed)
		}
	})
}

// TestShownPopupPlacement: in the work area of the primary monitor, the slides
// keep a popup inside, the resize cuts its height to the work area, and
// without adjustments it stays where its offset put it.
func TestShownPopupPlacement(t *testing.T) {
	inShell(t, func(s *Shell) {
		sf := shownParent(t, s, &recorder{})
		defer sf.Close()
		work := workArea(t, sf)
		cl, err := sf.screenClient()
		if err != nil {
			t.Fatal(err)
		}
		px := sf.px()
		// The anchor far right and below, beyond the work area.
		far := driver.Rect{X: float64(work.Right-cl.Left)/px + 100, Y: float64(work.Bottom-cl.Top)/px + 100, W: 10, H: 10}
		inside := func(what string, p *popup, wantH int32) {
			t.Helper()
			wr, _ := w32.GetWindowRect(p.hwnd)
			if wr.Left < work.Left || wr.Top < work.Top || wr.Right > work.Right || wr.Bottom > work.Bottom ||
				(wantH > 0 && wr.Bottom-wr.Top != wantH) {
				t.Errorf("%s: window %+v, work area %+v (height want %d)", what, wr, work, wantH)
			}
		}

		pr := &popRec{}
		p := newPopup(t, s, sf, shell.PopupConfig{Anchor: far, Size: driver.Size{W: 100, H: 60},
			Adjust: shell.AdjustSlideX | shell.AdjustSlideY}, pr)
		inside("slid", p, shellkit.ToPx(60, px))
		wr, _ := w32.GetWindowRect(p.hwnd)
		if wr.Right != work.Right || wr.Bottom != work.Bottom {
			t.Errorf("slid: window %+v, want its bottom-right at the work area's %+v", wr, work)
		}
		got := pr.placed[0]
		if x := float64(wr.Left-cl.Left) / px; got.X != x || got.W != 100 || got.H != 60 {
			t.Errorf("slid: Placed %v, want X %v", got, x)
		}
		p.Close()

		tall := float64(work.Bottom-work.Top)/px + 200
		pr = &popRec{}
		p = newPopup(t, s, sf, shell.PopupConfig{Anchor: driver.Rect{X: 10, Y: 10}, Size: driver.Size{W: 100, H: tall},
			Offset: driver.Point{Y: -100}, Adjust: shell.AdjustSlideX | shell.AdjustSlideY | shell.AdjustResizeY}, pr)
		inside("resized", p, work.Bottom-work.Top)
		if got := pr.placed[0]; got.H >= tall || got.W != 100 || p.size.H != got.H {
			t.Errorf("resized: Placed %v, size %v", got, p.size)
		}
		chainPx(t, "resized", p)
		p.Close()

		pr = &popRec{}
		p = newPopup(t, s, sf, shell.PopupConfig{Anchor: far, Size: driver.Size{W: 100, H: 60}}, pr)
		if got := pr.placed[0]; got != (driver.Rect{X: far.X, Y: far.Y, W: 100, H: 60}) {
			t.Errorf("unadjusted: Placed %v, want at the anchor %v", got, far)
		}
		p.Close()

		// A zero-size popup is allowed.
		pr = &popRec{}
		p = newPopup(t, s, sf, shell.PopupConfig{Anchor: driver.Rect{X: 5, Y: 5}}, pr)
		if got := pr.placed[0]; got != (driver.Rect{X: 5, Y: 5}) || p.chain.IsNil() {
			t.Errorf("empty: Placed %v, chain %v", got, !p.chain.IsNil())
		}
		p.Close()
	})
}

// TestShownPopupPaint: the popup's frame is cleared to transparent, so what
// its handler leaves empty (a rounded corner) shows what is behind it,
// and its paint is in the popup's points at its scale.
func TestShownPopupPaint(t *testing.T) {
	inShell(t, func(s *Shell) {
		sf := shownParent(t, s, &recorder{})
		defer sf.Close()
		fill := driver.RGB8(40, 80, 160)
		pr := &popRec{onPaint: func(c render.Canvas) {
			c.FillRoundedRect(rect(0, 0, 120, 60), render.UniformRadii(10), render.Solid(fill))
		}}
		p := newPopup(t, s, sf, shell.PopupConfig{Size: driver.Size{W: 120, H: 60}}, pr)
		w, h, pix := framePx(t, p)
		if pw, ph := clientPx(t, p); int32(w) != pw || int32(h) != ph {
			t.Fatalf("frame %d×%d, client %d×%d", w, h, pw, ph)
		}
		px := p.px()
		if c := framePxAt(w, h, pix, px, 0.5, 0.5); c != ([4]byte{}) {
			t.Errorf("corner pixel BGRA %v, want transparent", c)
		}
		if c := framePxAt(w, h, pix, px, 60, 30); !nearPx(c, [4]byte{160, 80, 40, 255}, 2) {
			t.Errorf("centre pixel BGRA %v, want the fill", c)
		}
		if c := framePxAt(w, h, pix, px, 119.5, 59.5); c != ([4]byte{}) {
			t.Errorf("bottom-right corner pixel BGRA %v, want transparent", c)
		}
		if pr.paints != 1 || p.presents != 1 {
			t.Errorf("%d paints, %d presents", pr.paints, p.presents)
		}
		savePNG(t, "popup", w, h, pix)

		// A lost device recreates the popup's chain with the surfaces'.
		s.deviceLost(errors.New("test"))
		if p.chain.IsNil() || !p.invalid || p.renderErr != nil {
			t.Fatalf("after a device loss: chain %v, invalid %v, %v", !p.chain.IsNil(), p.invalid, p.renderErr)
		}
		_, _, pix = framePx(t, p)
		if c := framePxAt(w, h, pix, px, 60, 30); !nearPx(c, [4]byte{160, 80, 40, 255}, 2) {
			t.Errorf("centre pixel after a device loss BGRA %v, want the fill", c)
		}
		p.Close()
	})
}

// TestShownPopupDeviceLossOnCreate: a device lost while a new popup creates
// its chain recovers (the parent's chain too) and the popup opens (B2).
func TestShownPopupDeviceLossOnCreate(t *testing.T) {
	inShell(t, func(s *Shell) {
		sf := shownParent(t, s, &recorder{})
		defer sf.Close()
		gen := s.stats.gpuGen
		s.hooks.faults = []fault{{op: "createChain", hr: com.DXGI_ERROR_DEVICE_REMOVED}}
		p := newPopup(t, s, sf, shell.PopupConfig{Anchor: driver.Rect{X: 20, Y: 20}, Size: driver.Size{W: 80, H: 40}}, &popRec{})
		defer p.Close()
		if len(s.hooks.faults) != 0 || s.stats.losses != 1 || s.stats.gpuGen != gen+1 || len(s.leaks) != 0 {
			t.Fatalf("faults %v, losses %d, generation %d (was %d), leaks %v", s.hooks.faults, s.stats.losses, s.stats.gpuGen, gen, s.leaks)
		}
		if p.chain.IsNil() || p.renderErr != nil || sf.chain.IsNil() || !sf.invalid {
			t.Fatalf("popup chain %v (%v), parent chain %v, invalid %v",
				!p.chain.IsNil(), p.renderErr, !sf.chain.IsNil(), sf.invalid)
		}
		chainPx(t, "popup", p)
	})
}

// TestShownPopupCloseWithParent: closing the parent closes its popups first,
// each with Closed before the parent's; Quit closes them too; a popup
// closed from its first Placed is never shown.
func TestShownPopupCloseWithParent(t *testing.T) {
	inShell(t, func(s *Shell) {
		var log []string
		r := &recorder{onClosed: func() { log = append(log, "parent closed") }}
		sf := shownParent(t, s, r)
		a := &popRec{log: &log}
		b := &popRec{log: &log}
		pa := newPopup(t, s, sf, shell.PopupConfig{Size: driver.Size{W: 40, H: 40}, Grab: true}, a)
		pb := newPopup(t, s, sf, shell.PopupConfig{Size: driver.Size{W: 40, H: 40}}, b)
		log = nil
		sf.Close()
		if a.closed != 1 || b.closed != 1 || len(log) != 3 || log[2] != "parent closed" ||
			!pa.dead || !pb.dead || len(s.popups) != 0 || s.grab != nil {
			t.Errorf("parent Close: log %v, %d popups, grab %v", log, len(s.popups), s.grab != nil)
		}

		sf = shownParent(t, s, &recorder{})
		c := &popRec{}
		c.onPlaced = func() { s.popups[0].Close() }
		pc := newPopup(t, s, sf, shell.PopupConfig{Size: driver.Size{W: 40, H: 40}}, c)
		if c.closed != 1 || !pc.dead || len(s.popups) != 0 || c.paints != 0 {
			t.Errorf("closed from Placed: closed %d, dead %v, %d popups", c.closed, pc.dead, len(s.popups))
		}
		d := &popRec{}
		newPopup(t, s, sf, shell.PopupConfig{Size: driver.Size{W: 40, H: 40}}, d)
		s.Quit()
		if d.closed != 1 || len(s.popups) != 0 {
			t.Errorf("Quit: closed %d, %d popups", d.closed, len(s.popups))
		}
	})
}
