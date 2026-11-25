//go:build windows

package win32

import (
	"errors"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// The modal surface tests (S5.7e) on real windows. Like the grab tests they
// send the messages Windows would deliver, so the real cursor does not mix
// in; whether Windows lets the test process take the foreground depends on
// the desktop, so the key hand-off is checked on the thread's active
// window, which SetActiveWindow sets either way.

// alertCfg is a modal surface the size of an alert.
var alertCfg = shell.SurfaceConfig{Size: driver.Size{W: 260, H: 180},
	Style: driver.StyleTitled | driver.StyleResizable | driver.StyleMiniaturizable, Modal: true}

// ownedBy returns alertCfg owned by o.
func ownedBy(o *surface) shell.SurfaceConfig {
	cfg := alertCfg
	cfg.Owner = o
	return cfg
}

// windowOwner returns the owner window of h.
func windowOwner(t *testing.T, h windows.HWND) windows.HWND {
	t.Helper()
	v, err := w32.GetWindowLongPtr(h, w32.GWLP_HWNDPARENT)
	if err != nil {
		t.Fatal(err)
	}
	return windows.HWND(v)
}

// windowLong returns the window value at index of h.
func windowLong(t *testing.T, h windows.HWND, index int32) uint32 {
	t.Helper()
	v, err := w32.GetWindowLongPtr(h, index)
	if err != nil {
		t.Fatal(err)
	}
	return uint32(v)
}

// TestModalOwnerErrors: Owner without Modal, a closed owner and a surface of
// another kind are ErrSurfaceOwner; a modal surface with an open owner or
// none is created.
func TestModalOwnerErrors(t *testing.T) {
	inShell(t, func(s *Shell) {
		o := newSurface(t, s, resizable, &recorder{})
		defer o.Close()
		bad := resizable
		bad.Owner = o
		if _, err := s.NewSurface(bad, &recorder{}); !errors.Is(err, shell.ErrSurfaceOwner) {
			t.Errorf("Owner without Modal: %v", err)
		}
		gone := newSurface(t, s, resizable, &recorder{})
		gone.Close()
		if _, err := s.NewSurface(ownedBy(gone), &recorder{}); !errors.Is(err, shell.ErrSurfaceOwner) {
			t.Errorf("closed Owner: %v", err)
		}
		other := alertCfg
		other.Owner = fakeSurface{}
		if _, err := s.NewSurface(other, &recorder{}); !errors.Is(err, shell.ErrSurfaceOwner) {
			t.Errorf("foreign Owner: %v", err)
		}
		m := newSurface(t, s, ownedBy(o), &recorder{})
		a := newSurface(t, s, alertCfg, &recorder{})
		if m.owner != o || a.owner != nil {
			t.Errorf("owners %p %p, want %p and nil", m.owner, a.owner, o)
		}
		m.Close()
		a.Close()
	})
}

// fakeSurface is a shell.Surface of no shell.
type fakeSurface struct{ shell.Surface }

// TestModalWindow: a modal surface is an owned WS_POPUP top level with a
// caption style (the DWM shadow) but no frame, no taskbar button and no
// resize, minimize or maximize; its drawable area is the whole window. It
// is owned by Owner's window, or by the listener when app-modal. Minimize,
// Zoom and their system commands do nothing, Center before the first Show
// does nothing, and a hidden modal blocks nothing.
func TestModalWindow(t *testing.T) {
	inShell(t, func(s *Shell) {
		o := newSurface(t, s, resizable, &recorder{})
		defer o.Close()
		m := newSurface(t, s, ownedBy(o), &recorder{})
		a := newSurface(t, s, alertCfg, &recorder{})
		for _, sf := range []*surface{m, a} {
			st := windowLong(t, sf.hwnd, w32.GWL_STYLE)
			ex := windowLong(t, sf.hwnd, w32.GWL_EXSTYLE)
			if st&w32.WS_POPUP == 0 || st&w32.WS_CAPTION != w32.WS_CAPTION ||
				st&(w32.WS_THICKFRAME|w32.WS_MINIMIZEBOX|w32.WS_MAXIMIZEBOX) != 0 {
				t.Errorf("style %#x", st)
			}
			if ex&(w32.WS_EX_APPWINDOW|w32.WS_EX_TOOLWINDOW) != 0 {
				t.Errorf("extended style %#x", ex)
			}
			if f := sf.frameAt(sf.dpi); f != (frame{}) || sf.resizable {
				t.Errorf("frame %+v, resizable %v", f, sf.resizable)
			}
			wr, _ := w32.GetWindowRect(sf.hwnd)
			if cl, err := sf.screenClient(); err != nil || cl != wr {
				t.Errorf("client %+v (%v), window %+v", cl, err, wr)
			}
			if sf.Size() != alertCfg.Size {
				t.Errorf("size %v", sf.Size())
			}
		}
		if got := windowOwner(t, m.hwnd); got != o.hwnd {
			t.Errorf("owned modal's owner %#x, want %#x", got, o.hwnd)
		}
		if got := windowOwner(t, a.hwnd); got != s.listener {
			t.Errorf("app-modal owner %#x, want the listener %#x", got, s.listener)
		}
		before, _ := w32.GetWindowRect(a.hwnd)
		a.Center()
		if after, _ := w32.GetWindowRect(a.hwnd); after != before {
			t.Errorf("Center before Show moved %+v to %+v", before, after)
		}
		if s.activeModal() != nil || o.blocked || !w32.IsWindowEnabled(o.hwnd) {
			t.Error("a hidden modal blocks")
		}
		a.Show()
		defer a.Close()
		a.Minimize()
		a.Zoom()
		sendTo(a.hwnd, w32.WM_SYSCOMMAND, w32.SC_MINIMIZE, 0)
		sendTo(a.hwnd, w32.WM_SYSCOMMAND, w32.SC_MAXIMIZE, 0)
		if w32.IsIconic(a.hwnd) || w32.IsZoomed(a.hwnd) || a.Size() != alertCfg.Size {
			t.Errorf("modal minimized %v, zoomed %v, size %v", w32.IsIconic(a.hwnd), w32.IsZoomed(a.hwnd), a.Size())
		}
		m.Close()
	})
}

// TestShownModalBlocks: the first Show of a modal surface ends the held
// press and the presence of the pointer on the other surfaces and disables
// them (one created later too); a blocked surface takes no input, refuses
// WM_CLOSE and the system commands that move, size or close it without
// asking CloseRequested, reports no activation and is shown without taking
// the key; closing the modal enables them again and gives the key to the
// owner.
func TestShownModalBlocks(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{}
		o := shownParent(t, s, r)
		defer o.Close()
		r2 := &recorder{}
		o2 := shownParent(t, s, r2)
		defer o2.Close()
		L := shell.ButtonPrimary
		sendTo(o.hwnd, w32.WM_MOUSEMOVE, 0, parentLP(o, 50, 40))
		sendTo(o.hwnd, w32.WM_LBUTTONDOWN, w32.MK_LBUTTON, parentLP(o, 50, 40))
		r.events = nil

		mr := &recorder{}
		m := newSurface(t, s, ownedBy(o), mr)
		m.Show()
		want := []pev{{pUp, L, 0, 0}, {pLeave, 0, 0, 0}}
		if got := compact(pointerEvents(t, r)); !samePevs(got, want) {
			t.Fatalf("owner on blocking:\n got %+v\nwant %+v", got, want)
		}
		if o.buttons != 0 || w32.GetCapture() == o.hwnd {
			t.Errorf("owner buttons %v, capture %#x", o.buttons, w32.GetCapture())
		}
		if s.activeModal() != m || !o.blocked || !o2.blocked || m.blocked ||
			w32.IsWindowEnabled(o.hwnd) || w32.IsWindowEnabled(o2.hwnd) || !w32.IsWindowEnabled(m.hwnd) {
			t.Fatalf("active %p (modal %p), blocked %v %v %v", s.activeModal(), m, o.blocked, o2.blocked, m.blocked)
		}
		if got := w32.GetActiveWindow(); got != m.hwnd {
			t.Errorf("active window %#x, want the modal %#x", got, m.hwnd)
		}
		later := newSurface(t, s, resizable, &recorder{})
		if !later.blocked || w32.IsWindowEnabled(later.hwnd) {
			t.Error("a surface created under the modal is not blocked")
		}
		later.Close()

		// Input, closes, moves and activations of a blocked surface.
		r.events, r.keys = nil, nil
		sendTo(o.hwnd, w32.WM_MOUSEMOVE, 0, parentLP(o, 60, 40))
		sendTo(o.hwnd, w32.WM_LBUTTONDOWN, w32.MK_LBUTTON, parentLP(o, 60, 40))
		sendTo(o.hwnd, w32.WM_LBUTTONUP, 0, parentLP(o, 60, 40))
		sendTo(o.hwnd, w32.WM_KEYDOWN, 'A', 0)
		sendTo(o.hwnd, w32.WM_CHAR, 'a', 0)
		sendTo(o.hwnd, w32.WM_CLOSE, 0, 0)
		sendTo(o.hwnd, w32.WM_SYSCOMMAND, w32.SC_CLOSE, 0)
		sendTo(o.hwnd, w32.WM_SYSCOMMAND, w32.SC_MAXIMIZE, 0)
		sendTo(o.hwnd, w32.WM_ACTIVATE, w32.WA_CLICKACTIVE, 0)
		hterror := uintptr(uint16(0xFFFE)) | uintptr(w32.WM_LBUTTONDOWN)<<16
		if got := sendTo(o.hwnd, w32.WM_SETCURSOR, uintptr(o.hwnd), hterror); got != 1 {
			t.Errorf("WM_SETCURSOR HTERROR = %d, want 1", got)
		}
		o.Show()
		pump() // the posted msgModalKey
		if len(r.events) != 0 || len(r.keys) != 0 || r.requests != 0 || o.dead || w32.IsZoomed(o.hwnd) {
			t.Fatalf("blocked owner: events %v, keys %v, CloseRequested %d, dead %v, zoomed %v",
				r.events, r.keys, r.requests, o.dead, w32.IsZoomed(o.hwnd))
		}
		if got := w32.GetActiveWindow(); got != m.hwnd {
			t.Errorf("active window %#x after the blocked owner's activation, want the modal %#x", got, m.hwnd)
		}

		m.Close()
		if mr.closed != 1 || s.activeModal() != nil || o.blocked || o2.blocked ||
			!w32.IsWindowEnabled(o.hwnd) || !w32.IsWindowEnabled(o2.hwnd) {
			t.Fatalf("after close: Closed %d, active %p, blocked %v %v", mr.closed, s.activeModal(), o.blocked, o2.blocked)
		}
		if got := w32.GetActiveWindow(); got != o.hwnd {
			t.Errorf("active window %#x after the modal closed, want the owner %#x", got, o.hwnd)
		}
		sendTo(o.hwnd, w32.WM_MOUSEMOVE, 0, parentLP(o, 70, 40))
		if len(r.events) == 0 {
			t.Error("the owner takes no input after the modal closed")
		}
	})
}

// TestShownModalGrab: blocking a surface dismisses its grab popup and ends
// the popup's held buttons; the capture is gone.
func TestShownModalGrab(t *testing.T) {
	inShell(t, func(s *Shell) {
		o := shownParent(t, s, &recorder{})
		defer o.Close()
		pr := &popRec{}
		p := newPopup(t, s, o, menuCfg, pr)
		sendTo(p.hwnd, w32.WM_MOUSEMOVE, 0, popLP(p, 10, 10))
		sendTo(p.hwnd, w32.WM_LBUTTONDOWN, w32.MK_LBUTTON, popLP(p, 10, 10))
		pr.events = nil
		m := newSurface(t, s, ownedBy(o), &recorder{})
		m.Show()
		if pr.dismissed != 1 || s.grab != nil || w32.GetCapture() == p.hwnd {
			t.Errorf("Dismissed %d, grab %p, capture %#x", pr.dismissed, s.grab, w32.GetCapture())
		}
		if got := compact(popEvents(t, pr)); len(got) == 0 || got[0] != (pev{pUp, shell.ButtonPrimary, 0, 0}) {
			t.Errorf("popup events %+v, want the release first", got)
		}
		p.Close()
		m.Close()
	})
}

// TestShownModalPlacement: an owned modal is centered on its owner's
// drawable area, again on Center after the owner moved; an app-modal one,
// or one whose owner is hidden, on the work area of the key surface's
// monitor; it stays inside the work area when the owner is near its edge.
func TestShownModalPlacement(t *testing.T) {
	inShell(t, func(s *Shell) {
		o := shownParent(t, s, &recorder{})
		defer o.Close()
		work := workArea(t, o)
		centered := func(sf *surface, area w32.Rect) bool {
			wr, _ := w32.GetWindowRect(sf.hwnd)
			cx, cy := (wr.Left+wr.Right)/2, (wr.Top+wr.Bottom)/2
			ax, ay := (area.Left+area.Right)/2, (area.Top+area.Bottom)/2
			return abs32(cx-ax) <= 1 && abs32(cy-ay) <= 1
		}
		inside := func(sf *surface) bool {
			wr, _ := w32.GetWindowRect(sf.hwnd)
			mi, err := w32.GetMonitorInfo(w32.MonitorFromWindow(sf.hwnd, w32.MONITOR_DEFAULTTONEAREST))
			if err != nil {
				return false
			}
			work := mi.RcWork // the monitor it landed on (the owner may straddle two)
			return wr.Left >= work.Left && wr.Top >= work.Top && wr.Right <= work.Right && wr.Bottom <= work.Bottom
		}
		m := newSurface(t, s, ownedBy(o), &recorder{})
		m.Show()
		if cl, _ := o.screenClient(); !centered(m, cl) {
			wr, _ := w32.GetWindowRect(m.hwnd)
			t.Errorf("modal %+v not centered on the owner %+v", wr, cl)
		}
		// The owner moves (programmatically: it is blocked), then Center.
		_ = w32.SetWindowPos(o.hwnd, 0, work.Left+10, work.Top+10, 0, 0,
			w32.SWP_NOSIZE|w32.SWP_NOZORDER|w32.SWP_NOACTIVATE)
		m.Center()
		if cl, _ := o.screenClient(); !centered(m, cl) {
			t.Error("Center did not follow the owner")
		}
		// The owner at the work area's top-left corner, larger than the
		// modal: the modal stays inside.
		_ = w32.SetWindowPos(o.hwnd, 0, work.Left-300, work.Top-250, 0, 0,
			w32.SWP_NOSIZE|w32.SWP_NOZORDER|w32.SWP_NOACTIVATE)
		m.Center()
		if !inside(m) {
			wr, _ := w32.GetWindowRect(m.hwnd)
			t.Errorf("modal %+v outside its monitor's work area", wr)
		}
		m.Close()

		a := newSurface(t, s, alertCfg, &recorder{})
		a.Show()
		if mon := w32.MonitorFromWindow(a.hwnd, w32.MONITOR_DEFAULTTONEAREST); mon == 0 {
			t.Fatal("no monitor")
		}
		if mi, err := w32.GetMonitorInfo(w32.MonitorFromWindow(a.hwnd, w32.MONITOR_DEFAULTTONEAREST)); err != nil || !centered(a, mi.RcWork) {
			wr, _ := w32.GetWindowRect(a.hwnd)
			t.Errorf("app-modal %+v not centered on the work area %+v (%v)", wr, mi.RcWork, err)
		}
		a.Close()
	})
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// TestShownModalStack: modals stack, the newest blocking the older ones
// and every surface; closing the newest unblocks the older modal only and
// gives it the key; closing the owner closes its modal first, and the
// other surfaces take input again.
func TestShownModalStack(t *testing.T) {
	inShell(t, func(s *Shell) {
		var log []string
		r := &recorder{onClosed: func() { log = append(log, "owner") }}
		o := shownParent(t, s, r)
		other := shownParent(t, s, &recorder{})
		defer other.Close()
		mr := &recorder{onClosed: func() { log = append(log, "modal") }}
		m := newSurface(t, s, ownedBy(o), mr)
		m.Show()
		a := newSurface(t, s, alertCfg, &recorder{})
		a.Show()
		if s.activeModal() != a || !m.blocked || !o.blocked || !other.blocked || a.blocked {
			t.Fatalf("stacked: active %p, blocked %v %v %v %v", s.activeModal(), m.blocked, o.blocked, other.blocked, a.blocked)
		}
		a.Close()
		if s.activeModal() != m || m.blocked || !o.blocked || !other.blocked || !w32.IsWindowEnabled(m.hwnd) {
			t.Fatalf("after the newest closed: active %p, blocked %v %v %v", s.activeModal(), m.blocked, o.blocked, other.blocked)
		}
		if got := w32.GetActiveWindow(); got != m.hwnd {
			t.Errorf("active window %#x, want the older modal %#x", got, m.hwnd)
		}
		o.Close()
		if len(log) != 2 || log[0] != "modal" || log[1] != "owner" {
			t.Errorf("Closed order %v, want [modal owner]", log)
		}
		if s.activeModal() != nil || other.blocked || !w32.IsWindowEnabled(other.hwnd) {
			t.Errorf("after the owner closed: active %p, other blocked %v", s.activeModal(), other.blocked)
		}
	})
}

// TestShownModalQuit: Quit with a modal shown closes everything, each
// surface getting Closed once.
func TestShownModalQuit(t *testing.T) {
	s := newTestShell()
	r, mr := &recorder{}, &recorder{}
	err := run(t, s, &handler{ready: func() {
		o := shownParent(t, s, r)
		m := newSurface(t, s, ownedBy(o), mr)
		m.Show()
		s.Quit()
	}})
	if err != nil {
		t.Fatal(err)
	}
	if r.closed != 1 || mr.closed != 1 || len(s.modals) != 0 {
		t.Errorf("Closed %d %d, modals %d", r.closed, mr.closed, len(s.modals))
	}
}
