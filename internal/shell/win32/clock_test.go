//go:build windows

package win32

import (
	"os"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/shellkit"
	"github.com/radix29/gogui/internal/shell/win32/com"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// TestFrameTimes: the vblank grid floors before and after its origin, and
// targets step by whole periods with the hold at least half a period after
// the frame's start.
func TestFrameTimes(t *testing.T) {
	const p = 10 * time.Millisecond
	for _, c := range []struct{ t, vblank, want time.Duration }{
		{0, 0, p}, {1, 0, p}, {p - 1, 0, p}, {p, 0, 2 * p},
		{-1, 0, 0}, {-p, 0, 0}, {-p - 1, 0, -p},
		{3 * time.Millisecond, 105 * time.Millisecond, 5 * time.Millisecond},
		{5 * time.Millisecond, 105 * time.Millisecond, 15 * time.Millisecond},
		{200 * time.Millisecond, 105 * time.Millisecond, 205 * time.Millisecond},
	} {
		if got := nextVBlank(c.t, p, c.vblank); got != c.want {
			t.Errorf("nextVBlank(%v, %v, %v) = %v, want %v", c.t, p, c.vblank, got, c.want)
		}
	}
	ms := time.Millisecond
	for _, c := range []struct{ t, prev, target, hold time.Duration }{
		{2 * ms, -1, p, p},                   // first frame: the next vblank
		{9 * ms, -1, p, 14 * ms},             // just before a vblank: held half a period
		{12 * ms, p, 2 * p, 2 * p},           // the next vblank
		{19 * ms, 2 * p, 3 * p, 3 * p},       // late for target 2p: targets 3p, not 2p again
		{21 * ms, 2 * p, 3 * p, 3 * p},       // on time
		{45 * ms, 2 * p, 5 * p, 5 * p},       // frames skipped: still on the grid
		{20 * ms, 2 * p, 3 * p, 3 * p},       // exactly on a vblank: the next one
		{22 * ms, 26 * ms, 36 * ms, 36 * ms}, // the previous target off the grid: whole steps from it
	} {
		target, hold := frameTimes(c.t, c.prev, p, 0)
		if target != c.target || hold != c.hold {
			t.Errorf("frameTimes(%v, %v) = %v, %v; want %v, %v", c.t, c.prev, target, hold, c.target, c.hold)
		}
		if hold < c.t+p/2 {
			t.Errorf("frameTimes(%v, %v): hold %v within half a period", c.t, c.prev, hold)
		}
	}
}

// frameStats checks a run of frame starts and targets: no two starts
// within half a period, targets strictly increasing by whole periods (on
// DWM's grid), and the run not faster than one frame per refresh.
func frameStats(t *testing.T, s *Shell, starts, targets []time.Duration) {
	t.Helper()
	period, vblank := s.clock.grid()
	t.Logf("refresh period %v, %d frames", period, len(starts))
	const slack = 200 * time.Microsecond // DWM's vblank estimate moves a little
	onePeriod := 0
	for i := 1; i < len(starts); i++ {
		if gap := starts[i] - starts[i-1]; gap < period/2 {
			t.Errorf("frames %d and %d started %v apart, within half a period", i-1, i, gap)
		}
		d := targets[i] - targets[i-1]
		n := (d + period/2) / period
		if n < 1 || (d-n*period).Abs() > slack {
			t.Errorf("targets %d and %d %v apart, not a whole number of periods", i-1, i, d)
		}
		if n == 1 {
			onePeriod++
		}
	}
	for i, tg := range targets {
		if off := (tg - vblank) % period; off > slack && off < period-slack {
			t.Errorf("target %d at %v, %v off the vblank grid", i, tg, off)
		}
		if tg < starts[i] {
			t.Errorf("target %d at %v, before its start %v", i, tg, starts[i])
		}
	}
	if n := len(starts); n > 1 {
		d := starts[n-1] - starts[0]
		if d < time.Duration(n-2)*period {
			t.Errorf("%d frames in %v, faster than the refresh (%v)", n, d, period)
		}
		t.Logf("%d frames in %v; %d of %d steps one period", n, d, onePeriod, n-1)
	}
	// On an idle host nearly every step is one period; half is the
	// margin for a loaded one.
	if onePeriod < (len(starts)-1)/2 {
		t.Errorf("only %d of %d target steps were one period", onePeriod, len(starts)-1)
	}
}

// idleCheck waits 150 ms after the last frame, then checks that no frame
// ran, no wait was armed and the hold timer is not set, and quits.
func idleCheck(t *testing.T, s *Shell) {
	frames, waits := s.clock.stats.frames, s.clock.stats.waits
	time.AfterFunc(150*time.Millisecond, func() {
		s.Post(func() {
			if s.clock.stats.frames != frames || s.clock.stats.waits != waits {
				t.Errorf("idle: %d frames and %d armed waits", s.clock.stats.frames-frames, s.clock.stats.waits-waits)
			}
			if s.clock.timerDue >= 0 || s.clock.scheduled || s.clock.modalTimer {
				t.Errorf("idle: hold timer due %v, modal scheduled %v, WM_TIMER %v",
					s.clock.timerDue, s.clock.scheduled, s.clock.modalTimer)
			}
			s.Quit()
		})
	})
}

// TestClockFrames: with no surface the hold paces the frames: requests
// coalesce, a request from the frame function gets the following frame,
// targets lie on DWM's vblank grid, and an idle clock arms no wait.
func TestClockFrames(t *testing.T) {
	s := newTestShell()
	const n = 20
	var starts, targets []time.Duration
	err := run(t, s, &handler{ready: func() {
		c := s.Clock()
		if s.clock.timer == 0 {
			t.Error("no hold timer")
		}
		c.SetFrameFunc(func(ft time.Duration) {
			starts = append(starts, s.clock.last)
			targets = append(targets, ft)
			if len(starts) < n {
				c.RequestFrame()
				c.RequestFrame() // coalesced
				return
			}
			idleCheck(t, s) // no request: the clock goes idle
		})
		for range 10 {
			c.RequestFrame()
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(starts) != n {
		t.Fatalf("%d frames, want %d", len(starts), n)
	}
	frameStats(t, s, starts, targets)
	if s.clock.timer != 0 {
		t.Error("hold timer still open after Run")
	}
}

// TestClockPaced: an animation on a shown surface (Invalidate from every
// frame) is paced by the swap chain's waitable and the hold: one Present
// per frame, no frame within half a period, no debt left, and idle once it
// stops. The stress run skips it (a visible window, real frames).
func TestClockPaced(t *testing.T) {
	s := newTestShell()
	const n = 40
	var starts, targets []time.Duration
	var debt int
	var sf *surface
	r := &recorder{}
	err := run(t, s, &handler{ready: func() {
		sf = newSurface(t, s, resizable, r)
		if sf.waitable == 0 {
			t.Fatal("no waitable object")
		}
		c := s.Clock()
		c.SetFrameFunc(func(ft time.Duration) {
			starts = append(starts, s.clock.last)
			targets = append(targets, ft)
			if len(starts) < n {
				// As an animation: it requests the next frame itself, since an
				// Invalidate in the frame function only joins this frame's
				// paint pass (S6.2a).
				sf.Invalidate()
				c.RequestFrame()
				return
			}
			debt = sf.debt
			idleCheck(t, s)
		})
		sf.Show()
		sf.Invalidate()
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(starts) != n {
		t.Fatalf("%d frames, want %d", len(starts), n)
	}
	frameStats(t, s, starts, targets)
	// Show may paint once through WM_PAINT before the first frame.
	if r.paints < n-1 || r.paints > n+1 || sf.presents != r.paints {
		t.Errorf("%d paints and %d presents for %d frames", r.paints, sf.presents, n)
	}
	if s.clock.stats.waits < n/2 || s.clock.stats.chainWaits < n/2 {
		t.Errorf("%d armed waits, %d on the waitable, for %d frames", s.clock.stats.waits, s.clock.stats.chainWaits, n)
	}
	if debt != 0 {
		t.Errorf("debt %d at the last frame", debt)
	}
	// Every frame but the last presented, so the waitable alone paced its
	// successor (S6.2e); frameStats checks it was still once per refresh.
	if s.clock.stats.waitPaced < n-2 {
		t.Errorf("%d of %d frames left their successor to the waitable", s.clock.stats.waitPaced, n)
	}
	t.Logf("%d waits, %d on the waitable, %d stalls", s.clock.stats.waits, s.clock.stats.chainWaits, s.clock.stats.stalls)
}

// TestClockWaitPacedRule: only a frame that presented, on surfaces that
// all have a waitable, leaves its successor to the waitables (S6.2e): a
// frame that presents nothing, or one presented on a surface without a
// waitable, holds the next frame to its target vblank, so neither runs
// faster than the refresh.
func TestClockWaitPacedRule(t *testing.T) {
	for _, tc := range []struct {
		name       string
		paint      bool // the frames invalidate the surface
		noWaitable bool // the surface's waitable is hidden from the clock
		wantPaced  bool
	}{
		{"presents", true, false, true},
		{"presents nothing", false, false, false},
		{"no waitable", true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestShell()
			const n = 30
			var starts, targets []time.Duration
			err := run(t, s, &handler{ready: func() {
				sf := newSurface(t, s, resizable, &recorder{})
				sf.Show()
				var real windows.Handle
				if tc.noWaitable {
					real, sf.waitable = sf.waitable, 0
				}
				c := s.Clock()
				c.SetFrameFunc(func(ft time.Duration) {
					starts = append(starts, s.clock.last)
					targets = append(targets, ft)
					if tc.paint {
						sf.Invalidate()
					}
					if len(starts) < n {
						c.RequestFrame()
						return
					}
					c.SetFrameFunc(nil)
					if tc.noWaitable {
						sf.waitable = real
					}
					s.Quit()
				})
				c.RequestFrame()
			}})
			if err != nil {
				t.Fatal(err)
			}
			frameStats(t, s, starts, targets)
			if paced := s.clock.stats.waitPaced > 0; paced != tc.wantPaced {
				t.Errorf("%d frames left their successor to the waitable, want some: %v", s.clock.stats.waitPaced, tc.wantPaced)
			}
		})
	}
}

// TestClockInvalidateInFrame: an Invalidate inside the frame function joins
// that frame's paint pass and requests no frame (S6.2a), so the last step
// of an animation ends without an empty frame after it.
func TestClockInvalidateInFrame(t *testing.T) {
	s := newTestShell()
	r := &recorder{}
	var sf *surface
	var frames, paints int
	err := run(t, s, &handler{ready: func() {
		sf = newSurface(t, s, resizable, r)
		sf.Show()
		// Once the first paints of Show are over.
		time.AfterFunc(200*time.Millisecond, func() {
			s.Post(func() {
				c := s.Clock()
				c.SetFrameFunc(func(time.Duration) {
					frames++
					if frames == 1 {
						paints = r.paints
						sf.Invalidate()
						if s.clock.requested {
							t.Error("an Invalidate in the frame function requested a frame")
						}
						idleCheck(t, s)
					}
				})
				c.RequestFrame()
			})
		})
	}})
	if err != nil {
		t.Fatal(err)
	}
	if frames != 1 || r.paints-paints != 1 || sf.invalid {
		t.Errorf("%d frames and %d paints for one invalidating frame (invalid %v), want 1 and 1",
			frames, r.paints-paints, sf.invalid)
	}
}

// TestClockModal: inside a modal size/move loop the frames come from the
// posted msgFrame and the listener's WM_TIMER, still held half a period
// apart; after the loop the message loop paces them again, with nothing
// of the modal path left armed. The stress run skips it (real frames).
func TestClockModal(t *testing.T) {
	s := newTestShell()
	var starts, targets []time.Duration
	var modal, after int
	err := run(t, s, &handler{ready: func() {
		sf := newSurface(t, s, resizable, &recorder{})
		c := s.Clock()
		w32.SendMessage(sf.hwnd, w32.WM_ENTERSIZEMOVE, 0, 0)
		c.SetFrameFunc(func(ft time.Duration) {
			starts = append(starts, s.clock.last)
			targets = append(targets, ft)
			switch len(starts) {
			case 10:
				modal = s.clock.stats.modalFrames
				// Outside the frame, within its hold: the request arms the
				// modal path, and leaving the loop disarms it.
				s.Post(func() {
					c.RequestFrame()
					if !s.clock.scheduled {
						t.Error("a request in the modal loop scheduled nothing")
					}
					w32.SendMessage(sf.hwnd, w32.WM_EXITSIZEMOVE, 0, 0)
					if s.clock.scheduled || s.clock.modalTimer {
						t.Error("the modal path still armed after WM_EXITSIZEMOVE")
					}
				})
			case 15:
				after = s.clock.stats.modalFrames
				idleCheck(t, s)
			default:
				c.RequestFrame()
			}
		})
		c.RequestFrame()
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(starts) != 15 || modal != 10 || after != 10 {
		t.Fatalf("%d frames, %d of the first 10 from the modal path, %d after it (want 15, 10, 10)",
			len(starts), modal, after)
	}
	period, _ := s.clock.grid()
	for i := 1; i < len(starts); i++ {
		if gap := starts[i] - starts[i-1]; gap < period/2 {
			t.Errorf("frames %d and %d started %v apart, within half a period", i-1, i, gap)
		}
	}
}

// TestClockModalPaced: an animation on a shown surface inside a modal
// size/move loop starts each frame only once the surface's waitable has
// signaled (its ticket) or the wait stalled, as the message loop does, so
// no frame blocks the loop in Present or Commit and the loop's mouse input
// keeps flowing (a drag that reached a screen edge lost its snap, and the
// window lagged the pointer, while frames ran back to back). The stress
// run skips it (real frames).
func TestClockModalPaced(t *testing.T) {
	s := newTestShell()
	const n = 30
	var frames, unready, stalls int
	var sf *surface
	err := run(t, s, &handler{ready: func() {
		sf = newSurface(t, s, resizable, &recorder{})
		if sf.waitable == 0 {
			t.Fatal("no waitable object")
		}
		c := s.Clock()
		c.SetFrameFunc(func(time.Duration) {
			frames++
			if !sf.ticket && s.clock.stats.stalls == stalls {
				unready++
			}
			stalls = s.clock.stats.stalls
			if frames < n {
				sf.Invalidate()
				c.RequestFrame()
				return
			}
			w32.SendMessage(sf.hwnd, w32.WM_EXITSIZEMOVE, 0, 0)
			idleCheck(t, s)
		})
		sf.Show()
		w32.SendMessage(sf.hwnd, w32.WM_ENTERSIZEMOVE, 0, 0)
		sf.Invalidate()
	}})
	if err != nil {
		t.Fatal(err)
	}
	if frames != n || s.clock.stats.modalFrames != n {
		t.Fatalf("%d frames, %d from the modal path; want %d, %d", frames, s.clock.stats.modalFrames, n, n)
	}
	if unready != 0 {
		t.Errorf("%d of %d modal frames started before the waitable signaled", unready, n)
	}
	t.Logf("%d stalls", s.clock.stats.stalls)
}

// TestClockModalNoPost: a frame inside a modal loop whose hold has passed
// (a frame that took longer than a refresh, EndDraw waiting for the GPU)
// comes from the listener's WM_TIMER, never from a posted msgFrame: a
// posted message beats input in the modal loop's GetMessage, so frames
// posting their successors back to back kept the button-up of a title-bar
// drag from the move loop, which held the capture and froze the mouse for
// every window (user, 2026-11-19).
func TestClockModalNoPost(t *testing.T) {
	s := newTestShell()
	var frames, modal int
	err := run(t, s, &handler{ready: func() {
		sf := newSurface(t, s, resizable, &recorder{})
		c := s.Clock()
		w32.SendMessage(sf.hwnd, w32.WM_ENTERSIZEMOVE, 0, 0)
		c.SetFrameFunc(func(time.Duration) {
			frames++
			modal = s.clock.stats.modalFrames
			w32.SendMessage(sf.hwnd, w32.WM_EXITSIZEMOVE, 0, 0)
			idleCheck(t, s)
		})
		s.clock.hold = 0 // long passed
		c.RequestFrame()
		if !s.clock.scheduled || !s.clock.modalTimer {
			t.Errorf("a due frame in the modal loop: scheduled %v, WM_TIMER %v; want both",
				s.clock.scheduled, s.clock.modalTimer)
		}
		var m w32.Msg
		if w32.PeekMessage(&m, s.listener, msgFrame, msgFrame, w32.PM_NOREMOVE) {
			t.Error("a due frame in the modal loop posted a msgFrame")
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if frames != 1 || modal != 1 {
		t.Fatalf("%d frames, %d from the modal path; want 1, 1", frames, modal)
	}
}

// TestSurfaceLiveResize: on a shown surface every resize presents a frame
// at the new size before it returns, inside a modal size loop (the
// window resized directly, as a drag does), through SetSize, and through
// WM_DPICHANGED; the whole new buffer is drawn. The stress run skips it
// (a visible window).
func TestSurfaceLiveResize(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{}
		sf := newSurface(t, s, resizable, r)
		defer sf.Close()
		sf.Show()
		var drawn []string
		s.hooks.onDrawn = func(d *swapTarget) {
			if d != sf.swap() {
				return
			}
			w, h := clientPx(t, sf)
			if uint32(w) != sf.pxW || uint32(h) != sf.pxH {
				t.Errorf("drawn at %d×%d, client %d×%d", sf.pxW, sf.pxH, w, h)
			}
			checkPixels(t, "resize frame", readBack(t, sf.swap(), corners), background(s))
			drawn = append(drawn, "")
		}
		defer func() { s.hooks.onDrawn = nil }()
		step := func(what string, resize func()) {
			t.Helper()
			presents, paints := sf.presents, r.paints
			resize()
			if sf.renderErr != nil {
				t.Fatalf("%s: %v", what, sf.renderErr)
			}
			if sf.presents != presents+1 || r.paints != paints+1 || sf.invalid {
				t.Errorf("%s: %d presents, %d paints, invalid %v; want one of each and valid",
					what, sf.presents-presents, r.paints-paints, sf.invalid)
			}
		}

		w32.SendMessage(sf.hwnd, w32.WM_ENTERSIZEMOVE, 0, 0)
		if !sf.sizing || !s.clock.modal() {
			t.Fatal("WM_ENTERSIZEMOVE: not sizing")
		}
		wr, err := w32.GetWindowRect(sf.hwnd)
		if err != nil {
			t.Fatal(err)
		}
		const n = 12
		for i := 1; i <= n; i++ {
			step("drag", func() {
				_ = w32.SetWindowPos(sf.hwnd, 0, 0, 0, wr.Right-wr.Left+int32(17*i), wr.Bottom-wr.Top+int32(9*i),
					w32.SWP_NOMOVE|w32.SWP_NOZORDER|w32.SWP_NOACTIVATE)
				rethrow()
			})
		}
		w32.SendMessage(sf.hwnd, w32.WM_EXITSIZEMOVE, 0, 0)
		if sf.sizing || s.clock.modal() {
			t.Fatal("WM_EXITSIZEMOVE: still sizing")
		}
		for i := range n {
			step("SetSize", func() { sf.SetSize(driver.Size{W: 300 + float64(23*i), H: 200 + float64(11*i)}) })
		}

		// A synthetic WM_DPICHANGED, as in TestSurfaceChainResize.
		step("WM_DPICHANGED", func() {
			size := sf.Size()
			newDPI := sf.dpi * 2
			wr, _ := w32.GetWindowRect(sf.hwnd)
			nw, nh := sf.nonClient(w32.GetDpiForWindow(sf.hwnd))
			scale := float64(newDPI) / 96
			rc := w32.Rect{Left: wr.Left, Top: wr.Top,
				Right: wr.Left + shellkit.ToPx(size.W, scale) + nw, Bottom: wr.Top + shellkit.ToPx(size.H, scale) + nh}
			var pin runtime.Pinner
			pin.Pin(&rc)
			defer pin.Unpin()
			w32.SendMessage(sf.hwnd, w32.WM_DPICHANGED, uintptr(newDPI)<<16|uintptr(newDPI), uintptr(unsafe.Pointer(&rc)))
			rethrow()
		})
		if len(drawn) != 2*n+1 {
			t.Errorf("%d frames drawn, want %d", len(drawn), 2*n+1)
		}

		// A hidden surface is resized but not painted: it stays invalid.
		w32.ShowWindow(sf.hwnd, w32.SW_HIDE)
		presents := sf.presents
		sf.SetSize(driver.Size{W: 333, H: 222})
		if sf.presents != presents || !sf.invalid {
			t.Errorf("hidden SetSize: %d presents, invalid %v", sf.presents-presents, sf.invalid)
		}
	})
}

// TestSurfaceWaitable: each chain has a waitable object, recreated with
// the chain on device loss and closed with it; Presents outside the clock
// (WM_PAINT) become debts the waitable's next signals pay back before one
// is a ticket.
func TestSurfaceWaitable(t *testing.T) {
	inShell(t, func(s *Shell) {
		sf := newSurface(t, s, resizable, &recorder{onPaint: func(render.Canvas) {}})
		if sf.waitable == 0 {
			t.Fatal("no waitable")
		}
		for range 3 {
			if _, ok := paintAndRead(t, sf); !ok {
				t.Fatal("not drawn")
			}
		}
		if sf.debt != 3 || sf.ticket {
			t.Errorf("after 3 WM_PAINTs: debt %d, ticket %v; want 3, false", sf.debt, sf.ticket)
		}
		// The waitable has its initial signal and one per Present: the first
		// three pay the debt, the fourth is the ticket, and then there is none.
		sf.Show()
		for i := range 5 {
			ev, err := windows.WaitForSingleObject(sf.waitable, 1000)
			if i == 4 {
				if ev == w32.WAIT_OBJECT_0 {
					t.Error("a fifth signal for one initial signal and three Presents")
				}
				break
			}
			if err != nil || ev != w32.WAIT_OBJECT_0 {
				t.Fatalf("signal %d: %#x, %v", i, ev, err)
			}
			s.clock.signaled(sf.waitable)
			if want := 2 - i; sf.debt != max(want, 0) || sf.ticket != (i == 3) {
				t.Errorf("after signal %d: debt %d, ticket %v", i, sf.debt, sf.ticket)
			}
		}
		old := sf.waitable
		s.hooks.faults = []fault{{op: "EndDraw", hr: com.D2DERR_RECREATE_TARGET}}
		paintAndRead(t, sf)
		if s.stats.losses != 1 || sf.waitable == 0 || sf.debt != 0 || sf.ticket {
			t.Errorf("after device loss: %d losses, waitable %#x (was %#x), debt %d, ticket %v",
				s.stats.losses, sf.waitable, old, sf.debt, sf.ticket)
		}
		sf.Close()
		if sf.waitable != 0 {
			t.Error("waitable left open after Close")
		}
	})
}

// TestClockStall: a shown surface whose waitable never signals (an event
// that stays reset stands in for a stuck chain) still gets its frame,
// after stallTimeout however many messages arrive meanwhile, counted as a
// stall. The stress run skips it (a
// visible window, real waits).
func TestClockStall(t *testing.T) {
	s := newTestShell()
	var waited time.Duration
	var sf *surface
	var real windows.Handle
	err := run(t, s, &handler{ready: func() {
		sf = newSurface(t, s, resizable, &recorder{})
		sf.Show()
		stuck, err := windows.CreateEvent(nil, 1, 0, nil)
		if err != nil {
			t.Fatal(err)
		}
		real, sf.waitable = sf.waitable, stuck
		sf.ticket, sf.debt = false, 0
		c := s.Clock()
		var start time.Duration
		c.SetFrameFunc(func(time.Duration) {
			waited = c.Now() - start
			sf.waitable = real
			_ = windows.CloseHandle(stuck)
			c.SetFrameFunc(nil)
			s.Post(s.Quit)
		})
		s.Post(func() { // after Show's paint
			start = c.Now()
			sf.ticket = false
			c.RequestFrame()
			// Messages every 10 ms meanwhile don't push the deadline back.
			go func() {
				for range 30 {
					time.Sleep(10 * time.Millisecond)
					s.Post(func() {})
				}
			}()
		})
	}})
	if err != nil {
		t.Fatal(err)
	}
	if s.clock.stats.stalls != 1 || waited < stallTimeout*9/10 || waited > 2*stallTimeout {
		t.Errorf("%d stalls, frame after %v; want 1 after about %v", s.clock.stats.stalls, waited, stallTimeout)
	}
	t.Logf("frame after %v", waited)
}

// TestClockHidden: an animation (a frame function that requests the next
// frame) on a surface that is minimized runs at hiddenInterval, its frame
// times still moving; restored, it is back at the refresh rate at once.
// A surface created and not shown yet is off screen too. The stress run
// skips it (real windows, real time).
func TestClockHidden(t *testing.T) {
	s := newTestShell()
	type phase struct {
		name           string
		starts, frames []time.Duration
	}
	var phases []*phase
	var cur *phase
	var sf *surface
	var restoredAt time.Duration
	begin := func(name string) {
		cur = &phase{name: name}
		phases = append(phases, cur)
	}
	after := func(d time.Duration, fn func()) {
		time.AfterFunc(d, func() { s.Post(fn) })
	}
	err := run(t, s, &handler{ready: func() {
		sf = newSurface(t, s, resizable, &recorder{})
		c := s.Clock()
		c.SetFrameFunc(func(ft time.Duration) {
			cur.starts = append(cur.starts, s.clock.last)
			cur.frames = append(cur.frames, ft)
			c.RequestFrame()
		})
		begin("not shown")
		c.RequestFrame()
		after(time.Second, func() {
			sf.Show()
			begin("shown")
			after(500*time.Millisecond, func() {
				sf.Minimize()
				if !w32.IsIconic(sf.hwnd) {
					t.Error("Minimize left the window on screen")
				}
				begin("minimized")
				after(time.Second, func() {
					sf.Show() // restores
					restoredAt = c.Now()
					begin("restored")
					after(500*time.Millisecond, func() {
						c.SetFrameFunc(nil)
						s.Quit()
					})
				})
			})
		})
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range phases {
		if len(p.starts) == 0 {
			t.Fatalf("%s: no frame", p.name)
		}
		var minGap, maxGap time.Duration
		for i := 1; i < len(p.starts); i++ {
			g := p.starts[i] - p.starts[i-1]
			if i == 1 || g < minGap {
				minGap = g
			}
			maxGap = max(maxGap, g)
			if p.frames[i] <= p.frames[i-1] {
				t.Errorf("%s: frame time %v after %v", p.name, p.frames[i], p.frames[i-1])
			}
		}
		t.Logf("%s: %d frames, gaps %v–%v", p.name, len(p.starts), minGap, maxGap)
		switch p.name {
		case "not shown", "minimized": // 1 s at 4 Hz: 4 or 5 frames
			if len(p.starts) < 3 || len(p.starts) > 6 || minGap < hiddenInterval-2*time.Millisecond {
				t.Errorf("%s: %d frames in 1 s, gaps from %v; want about 4, none below %v",
					p.name, len(p.starts), minGap, hiddenInterval)
			}
			if span := p.frames[len(p.frames)-1] - p.frames[0]; span < hiddenInterval*5/2 {
				t.Errorf("%s: the frame times moved %v in 1 s", p.name, span)
			}
		default: // 0.5 s at the refresh rate (60 Hz or more)
			if len(p.starts) < 20 || maxGap > hiddenInterval/2 {
				t.Errorf("%s: %d frames in 0.5 s, gaps up to %v; want the refresh rate", p.name, len(p.starts), maxGap)
			}
		}
	}
	if r := phases[len(phases)-1]; len(r.starts) > 0 {
		if d := r.starts[0] - restoredAt; d > 100*time.Millisecond {
			t.Errorf("the first frame after the restore came %v later", d)
		}
	}
	if s.clock.stats.hiddenFrames < 6 || s.clock.stats.hiddenFrames > 12 {
		t.Errorf("%d frames off screen, want about 8", s.clock.stats.hiddenFrames)
	}
}

// TestClockFrameObserver: on a shown surface the observer gets one record
// per frame after its paint pass, with the frame function, the paint, the
// submit (EndDraw) and the present measured, and a synchronous resize
// paint outside a frame as a record of its own (Time -1); a frame
// painting nothing reports no surface (S6.2c).
func TestClockFrameObserver(t *testing.T) {
	inShell(t, func(s *Shell) {
		sf := newSurface(t, s, resizable, &recorder{})
		defer sf.Close()
		sf.Show()
		var got []shell.FrameTiming
		c := s.Clock()
		c.SetFrameObserver(func(ft shell.FrameTiming) { got = append(got, ft) })
		defer c.SetFrameObserver(nil)
		c.SetFrameFunc(func(time.Duration) {
			time.Sleep(time.Millisecond)
			sf.Invalidate()
		})
		defer c.SetFrameFunc(nil)
		s.clock.requested = true
		s.clock.frame()
		rethrow()
		if len(got) != 1 {
			t.Fatalf("one frame: %d records %+v", len(got), got)
		}
		if ft := got[0]; ft.Time != s.clock.target || ft.Surfaces != 1 || ft.FrameFunc < time.Millisecond ||
			ft.Paint <= 0 || ft.Submit <= 0 || ft.Present <= 0 {
			t.Errorf("a painting frame: %+v (target %v)", ft, s.clock.target)
		}
		got = nil
		c.SetFrameFunc(nil)
		s.clock.requested = true
		s.clock.frame()
		if len(got) != 1 || got[0].Surfaces != 0 || got[0].Paint != 0 {
			t.Errorf("a frame painting nothing: %+v", got)
		}
		got = nil
		sf.SetSize(driver.Size{W: 333, H: 222})
		if len(got) != 1 || got[0].Time != -1 || got[0].Surfaces != 1 || got[0].FrameFunc != 0 || got[0].Paint <= 0 {
			t.Errorf("a resize paint: %+v", got)
		}
	})
}

// TestClockPresentObserver: on a real HWND an animation of 90 frames is
// reported shown frame by frame (S6.2d): each report's target is one of
// the frames' (or -1, the paint of Show), its vblank count after the previous report's. The Pacing
// summary is logged, not judged: check.sh runs this on a busy host (the
// verdict is gogui's TestFramePacingNative, run by native.sh).
func TestClockPresentObserver(t *testing.T) {
	s := newTestShell()
	var got []shell.PresentTiming
	var p shell.Pacing
	targets := map[time.Duration]bool{}
	const frames = 90
	n := 0
	err := run(t, s, &handler{ready: func() {
		sf := newSurface(t, s, resizable, &recorder{})
		sf.Show()
		c := s.Clock()
		c.SetPresentObserver(func(pt shell.PresentTiming) {
			got = append(got, pt)
			p.Present(pt)
		})
		c.SetFrameFunc(func(ft time.Duration) {
			n++
			targets[ft] = true
			p.Frame(ft, n < frames)
			sf.Invalidate()
			if n < frames {
				c.RequestFrame()
				return
			}
			c.SetFrameFunc(nil)
			c.SetPresentObserver(nil)
			s.Quit()
		})
		c.RequestFrame()
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < frames/2 { // the statistics skip some (two shown between two reads)
		t.Fatalf("%d presents reported shown of %d frames", len(got), frames)
	}
	for i, pt := range got {
		if pt.Target == -1 {
			continue // the paint of Show, outside a frame
		}
		if !targets[pt.Target] {
			t.Errorf("report %d: target %v is no frame's", i, pt.Target)
		}
		if pt.Period <= 0 || pt.Shown <= pt.Target-pt.Period {
			t.Errorf("report %d: %+v shown before its target", i, pt)
		}
		if i > 0 && pt.Refresh <= got[i-1].Refresh {
			t.Errorf("report %d: vblank %d after %d", i, pt.Refresh, got[i-1].Refresh)
		}
	}
	sum := p.Summary()
	t.Logf("PACING-RESULT clock %v", sum)
}

// TestClockPacingWork: an animation on a real HWND with a fixed amount of
// CPU work in each frame function (0, 3, 6 and 10 ms, all inside a 60 Hz
// frame), 3 s each, its Pacing summary logged as a PACING-WORK line with
// the lead, from a frame's start to its target vblank, mean and least
// (S6.2d's finding: on the host the vblanks missed grew with the work,
// though every frame fit; S6.2e's before and after). Only with
// GOGUI_PACING=1 (native.sh), and never judged.
func TestClockPacingWork(t *testing.T) {
	if os.Getenv("GOGUI_PACING") != "1" {
		t.Skip("set GOGUI_PACING=1 for the pacing runs (native.sh does)")
	}
	for _, work := range []time.Duration{0, 3 * time.Millisecond, 6 * time.Millisecond, 10 * time.Millisecond} {
		s := newTestShell()
		var p shell.Pacing
		const frames = 180
		n := 0
		var lead, minLead time.Duration // from the frame's start to its target
		err := run(t, s, &handler{ready: func() {
			sf := newSurface(t, s, resizable, &recorder{})
			sf.Show()
			c := s.Clock()
			c.SetPresentObserver(p.Present)
			c.SetFrameFunc(func(ft time.Duration) {
				n++
				if l := ft - s.clock.last; n == 1 || l < minLead {
					minLead = l
				}
				lead += ft - s.clock.last
				p.Frame(ft, n < frames)
				for end := s.clock.now() + work; s.clock.now() < end; {
				}
				sf.Invalidate()
				if n < frames {
					c.RequestFrame()
					return
				}
				c.SetFrameFunc(nil)
				c.SetPresentObserver(nil)
				s.Quit()
			})
			c.RequestFrame()
		}})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("PACING-WORK %v of work per frame: %v lead-mean=%.2fms lead-min=%.2fms", work, p.Summary(),
			float64(lead)/float64(frames)/1e6, float64(minLead)/1e6)
	}
}
