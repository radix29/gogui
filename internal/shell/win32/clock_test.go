//go:build windows

package win32

import (
	"runtime"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
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
	frames, waits := s.clock.frames, s.clock.waits
	time.AfterFunc(150*time.Millisecond, func() {
		s.Post(func() {
			if s.clock.frames != frames || s.clock.waits != waits {
				t.Errorf("idle: %d frames and %d armed waits", s.clock.frames-frames, s.clock.waits-waits)
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
				sf.Invalidate()
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
	if s.clock.waits < n/2 || s.clock.chainWaits < n/2 {
		t.Errorf("%d armed waits, %d on the waitable, for %d frames", s.clock.waits, s.clock.chainWaits, n)
	}
	if debt != 0 {
		t.Errorf("debt %d at the last frame", debt)
	}
	t.Logf("%d waits, %d on the waitable, %d stalls", s.clock.waits, s.clock.chainWaits, s.clock.stalls)
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
				modal = s.clock.modalFrames
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
				after = s.clock.modalFrames
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
		s.onDrawn = func(d *surface) {
			if d != sf {
				return
			}
			w, h := clientPx(t, sf)
			if uint32(w) != sf.pxW || uint32(h) != sf.pxH {
				t.Errorf("drawn at %d×%d, client %d×%d", sf.pxW, sf.pxH, w, h)
			}
			checkPixels(t, "resize frame", readBack(t, sf, corners), background(s))
			drawn = append(drawn, "")
		}
		defer func() { s.onDrawn = nil }()
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
		s.fault = fault{op: "EndDraw", hr: com.D2DERR_RECREATE_TARGET}
		paintAndRead(t, sf)
		if s.losses != 1 || sf.waitable == 0 || sf.debt != 0 || sf.ticket {
			t.Errorf("after device loss: %d losses, waitable %#x (was %#x), debt %d, ticket %v",
				s.losses, sf.waitable, old, sf.debt, sf.ticket)
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
	if s.clock.stalls != 1 || waited < stallTimeout*9/10 || waited > 2*stallTimeout {
		t.Errorf("%d stalls, frame after %v; want 1 after about %v", s.clock.stalls, waited, stallTimeout)
	}
	t.Logf("frame after %v", waited)
}
