//go:build windows

package win32

import (
	"time"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// The paced frame clock (S2.4f).
//
// Time: Now is the performance counter (QPC) since New, the timeline of
// DWM's vblank timing, so vblanks convert onto it exactly.
//
// Pacing: the message loop (Shell.Run, wait) runs a requested frame once
//
//  1. its hold has passed: the previous frame's target vblank
//     (DwmGetCompositionTimingInfo), but at least half a refresh period
//     after the previous frame's start, on a high-resolution waitable
//     timer. So no two frames start within half a period, and frames that
//     present nothing (the frame function only) still run once per
//     refresh, on DWM's vblank grid; and
//  2. one of the pacing surfaces can take a frame: the visible surfaces
//     with a chain (only the invalid ones, if any are), through their swap
//     chains' frame-latency waitable objects. A signal taken from a
//     waitable is the surface's ticket, spent by its next Present; a
//     Present without a ticket is a debt, paid back by the waitable's next
//     signals before one counts as a ticket again, so the
//     waitables stay in step with the presents however a frame got there
//     (a synchronous resize paint, WM_PAINT). The waitable is a semaphore:
//     it starts at 1 and gains one signal per Present, uncapped (measured
//     on the host); ResizeBuffers leaves it alone.
//
// Without a pacing surface (none visible) the hold alone paces the frames.
// After a frame that presented, on surfaces that all have a waitable and
// outside a modal loop, the hold is only half a period after the frame's
// start (S6.2e): the waitables pace the next frame, which starts as soon
// as the display has taken this one, not only at DWM's next vblank. With
// several monitors DWM's grid (16.680 ms on the host's four in S6.2d) is
// not the displays' (16.666 ms), and a frame held to it started late in
// the composition's period for seconds at a time, missing vblanks with
// work that fitted the frame; with one monitor the waitable signals about
// 0.2 ms after the vblank, so nothing changes there. A frame that
// presented nothing still holds the next one to its target vblank, so it
// runs once per refresh.
// With open surfaces but no window on screen (every surface hidden or
// minimized, no popup shown: Shell.offScreen) the hold is at least
// hiddenInterval after the previous frame's start, so a running animation
// costs 4 frames a second, not one per refresh; its time keeps moving, so
// a value still ends (S6.2b). Showing or restoring a window ends the low
// rate at once: the next arm uses the vsync hold again. A fully occluded
// window keeps the vsync rate: the Present of a composition swap chain
// reports no DXGI_STATUS_OCCLUDED (DirectComposition, not DXGI, knows what
// is visible), and nothing else tells it cheaply.
// The loop waits in MsgWaitForMultipleObjectsEx on the timer or the
// waitables and the message queue, so input keeps flowing; a waitable that
// stays unsignaled for stallTimeout counts as signaled. Idle means no
// request, no timer set and no handle in the wait (waits counts the armed
// waits).
//
// Modal loops (the size/move loop between WM_ENTERSIZEMOVE and
// WM_EXITSIZEMOVE, the menu loop) run their own message loop, so there a
// frame comes from the listener's WM_TIMER, never a posted msgFrame,
// which would come before the loop's input (coarse; WM_SIZE paints
// synchronously anyway). A frame there still waits for a pacing surface's
// waitable (chainReady), polled on the WM_TIMER, since a frame started
// early blocks the loop.

// defaultPeriod is the refresh period when DWM reports no timing.
const defaultPeriod = time.Second / 60

// stallTimeout is how long the loop waits on the waitables for a frame
// before it takes the frame anyway (a stuck chain). It is a deadline, not a
// per-wait timeout, so messages arriving meanwhile don't extend it.
const stallTimeout = 100 * time.Millisecond

// hiddenInterval is the shortest time between two frame starts while no
// window is on screen (S6.2b): 4 Hz.
const hiddenInterval = time.Second / 4

// timerFrame is the listener's WM_TIMER ID for frames in a modal loop.
const timerFrame = 1

type clock struct {
	s *Shell

	freq, start int64 // QPC ticks per second, QPC at New
	fn          func(frameTime time.Duration)

	// paint runs after the frame function: the paints of the invalidated
	// surfaces (S2.3c).
	paint func(frameTime time.Duration)

	requested bool // a frame is requested
	inFrame   bool // the frame function or the paint pass runs
	inFn      bool // the frame function runs: Invalidate requests no frame

	last   time.Duration // start of the last frame; -1 before the first
	target time.Duration // target time of the last frame
	hold   time.Duration // the next frame starts at or after this time
	// chainSince is when the wait on the waitables for the requested frame
	// began (-1: not waiting), the base of the stall deadline.
	chainSince time.Duration

	// The high-resolution waitable timer for the hold (Run creates it; 0
	// without one) and the time it is set for (-1: not set).
	timer    windows.Handle
	timerDue time.Duration

	// Modal loops: a msgFrame is posted or the WM_TIMER runs.
	scheduled, modalTimer bool

	// timing returns DWM's refresh period and a vblank on the Now
	// timeline (tests replace it).
	timing func() (period, vblank time.Duration, ok bool)

	// The presents of the running frame and whether one was on a window
	// without a waitable (S6.2e).
	framePresents int
	unpaced       bool

	stats clockStats // counters (tests)

	// timer measures each frame for the observer (SetFrameObserver).
	frameTimer shell.FrameTimer
	// presentObserver receives the presentation feedback
	// (SetPresentObserver, pacing.go).
	presentObserver func(shell.PresentTiming)

	// handles is the scratch of arm's and chainReady's wait handles
	// (review P17): their callers are done with them before the next
	// idle step.
	handles []windows.Handle
}

var _ shell.FrameClock = (*clock)(nil)

func newClock(s *Shell) *clock {
	c := &clock{s: s, freq: w32.QueryPerformanceFrequency(), start: w32.QueryPerformanceCounter(),
		last: -1, timerDue: -1, chainSince: -1}
	c.timing = c.dwmTiming
	return c
}

// ticks converts QPC ticks to a duration.
func (c *clock) ticks(t int64) time.Duration {
	return time.Duration(t/c.freq)*time.Second + time.Duration(t%c.freq)*time.Second/time.Duration(c.freq)
}

func (c *clock) now() time.Duration { return c.ticks(w32.QueryPerformanceCounter() - c.start) }

// Now implements shell.FrameClock.
func (c *clock) Now() time.Duration {
	c.s.q.Check("win32.Clock.Now")
	return c.now()
}

// SetFrameFunc implements shell.FrameClock.
func (c *clock) SetFrameFunc(fn func(frameTime time.Duration)) {
	c.s.q.Check("win32.Clock.SetFrameFunc")
	c.fn = fn
}

// SetFrameObserver implements shell.FrameClock.
func (c *clock) SetFrameObserver(fn func(shell.FrameTiming)) {
	c.s.q.Check("win32.Clock.SetFrameObserver")
	c.frameTimer.SetObserver(fn)
}

// RequestFrame implements shell.FrameClock. Outside a modal loop the
// message loop picks the request up once the current message returns.
func (c *clock) RequestFrame() {
	c.s.q.Check("win32.Clock.RequestFrame")
	if c.requested {
		return
	}
	c.requested = true
	if !c.inFrame && c.modal() {
		c.scheduleModal()
	}
}

// startTimer creates the hold timer (Run): a high-resolution one, else a
// standard one (Windows before 10 1803), else none (the wait's timeout
// holds instead).
func (c *clock) startTimer() {
	t, err := w32.CreateWaitableTimer(true)
	if err != nil {
		t, _ = w32.CreateWaitableTimer(false)
	}
	c.timer = t
}

// close stops the timers and closes the hold timer (Run's teardown).
func (c *clock) close() {
	c.cancelTimer()
	c.stopModal()
	if c.timer != 0 {
		_ = windows.CloseHandle(c.timer)
		c.timer = 0
	}
}

// dwmTiming reads DWM's composition timing.
func (c *clock) dwmTiming() (period, vblank time.Duration, ok bool) {
	ti, err := w32.DwmGetCompositionTimingInfo()
	if err != nil || ti.RefreshPeriod == 0 || ti.RefreshPeriod > uint64(c.freq) {
		return 0, 0, false
	}
	return c.ticks(int64(ti.RefreshPeriod)), c.ticks(int64(ti.VBlank) - c.start), true
}

// grid returns the refresh period and a vblank: DWM's, or 60 Hz from 0.
func (c *clock) grid() (period, vblank time.Duration) {
	if p, v, ok := c.timing(); ok && p > 0 {
		return p, v
	}
	return defaultPeriod, 0
}

// nextVBlank returns the first vblank of the grid (period, through vblank)
// after t.
func nextVBlank(t, period, vblank time.Duration) time.Duration {
	d := t - vblank
	n := d / period
	if d < 0 && d%period != 0 {
		n-- // floor
	}
	return vblank + (n+1)*period
}

// frameTimes returns the target and the next hold of a frame starting at
// t: the target is the first vblank after t, but a whole period after the
// previous target prev (-1: none) if it would be less than half a period
// after it, so targets step by whole periods; the hold is the target, or
// half a period after t if that is later (a frame just before a vblank).
func frameTimes(t, prev, period, vblank time.Duration) (target, hold time.Duration) {
	target = nextVBlank(t, period, vblank)
	if prev >= 0 && target < prev+period/2 {
		target = prev + period
	}
	return target, max(target, t+period/2)
}

// arm prepares the message loop's wait. It returns the handles to wait on
// (the hold timer, or the pacing surfaces' waitables) and the timeout in
// ms, or now when the requested frame can start at once. Without a request
// it returns no handles and INFINITE (the timer is cancelled by frame,
// modalChanged and close).
func (c *clock) arm() (handles []windows.Handle, ms uint32, now bool) {
	if !c.requested || c.inFrame || c.s.quit || c.modal() {
		return nil, w32.INFINITE, false
	}
	t := c.now()
	if hold := c.due(); t < hold {
		if c.setTimer(hold, t) {
			c.handles = append(c.handles[:0], c.timer)
			return c.handles, w32.INFINITE, false
		}
		return nil, uint32((hold - t + time.Millisecond - 1) / time.Millisecond), false
	}
	c.cancelTimer()
	pacing := c.s.pacingSurfaces()
	if len(pacing) == 0 {
		return nil, 0, true
	}
	handles = c.handles[:0]
	for _, sf := range pacing {
		if sf.ticket {
			return nil, 0, true
		}
		handles = append(handles, sf.waitable)
	}
	c.handles = handles
	c.stats.chainWaits++
	if c.chainSince < 0 {
		c.chainSince = t
	}
	left := c.chainSince + stallTimeout - t
	if left <= 0 {
		c.stalled(handles)
		return nil, 0, true
	}
	return handles, uint32((left + time.Millisecond - 1) / time.Millisecond), false
}

// due returns when the requested frame may start: the hold, but with no
// window on screen at least hiddenInterval after the previous frame's
// start (S6.2b).
func (c *clock) due() time.Duration {
	if c.last >= 0 && c.s.offScreen() {
		return max(c.hold, c.last+hiddenInterval)
	}
	return c.hold
}

// signaled handles a signaled handle of arm's wait: a waitable pays its
// surface's debt or gives it a ticket; the timer only ends the hold.
func (c *clock) signaled(h windows.Handle) {
	if h == c.timer {
		c.timerDue = -1
		return
	}
	for sf := range c.s.allWindows() {
		if sf.waitable == h {
			sf.signal()
			return
		}
	}
}

// stalled handles a wait on the waitables that timed out: every surface
// waited on gets a ticket.
func (c *clock) stalled(handles []windows.Handle) {
	c.stats.stalls++
	for sf := range c.s.allWindows() {
		for _, h := range handles {
			if sf.waitable == h {
				sf.debt, sf.ticket = 0, true
			}
		}
	}
}

// setTimer sets the hold timer for due (t is now) and reports whether it
// is set.
func (c *clock) setTimer(due, t time.Duration) bool {
	if c.timer == 0 {
		return false
	}
	if c.timerDue == due {
		return true
	}
	if err := w32.SetWaitableTimer(c.timer, int64((due-t)/100)); err != nil {
		return false
	}
	c.timerDue = due
	return true
}

func (c *clock) cancelTimer() {
	if c.timerDue >= 0 {
		_ = w32.CancelWaitableTimer(c.timer)
		c.timerDue = -1
	}
}

// frame runs the requested frame: the frame function, then the paint
// pass, with the frame's target time (frameTimes).
func (c *clock) frame() {
	if !c.requested || c.inFrame || c.s.quit {
		return
	}
	c.cancelTimer()
	t := c.now()
	period, vblank := c.grid()
	prev := time.Duration(-1)
	if c.last >= 0 {
		prev = c.target
	}
	c.target, c.hold = frameTimes(t, prev, period, vblank)
	c.last = t
	c.requested = false
	c.chainSince = -1
	c.stats.frames++
	c.framePresents, c.unpaced = 0, false
	if c.s.offScreen() {
		c.stats.hiddenFrames++
	}
	c.inFrame = true
	defer func() {
		c.inFrame, c.inFn = false, false
		if c.requested && c.modal() {
			c.scheduleModal()
		}
	}()
	c.frameTimer.Begin(c.target)
	if c.fn != nil {
		var start time.Duration // QPC, as in render
		if c.frameTimer.Active() {
			start = c.now()
		}
		c.inFn = true
		c.fn(c.target)
		c.inFn = false
		if c.frameTimer.Active() {
			c.frameTimer.FrameFunc(c.now() - start)
		}
	}
	if c.paint != nil {
		c.paint(c.target)
	}
	if c.framePresents > 0 && !c.unpaced && !c.s.offScreen() && !c.modal() {
		// The waitables pace the next frame (S6.2e): it may start as soon
		// as the display has taken this one, not only at the next vblank.
		c.hold = c.last + period/2
		c.stats.waitPaced++
	}
	c.frameTimer.End()
}

// invalidated requests the frame that paints a window just invalidated,
// except while the frame function runs: the paint pass of that frame
// paints it, so an animation whose last change invalidates ends without
// an empty frame after it (PLAN.md S6.2a).
func (c *clock) invalidated() {
	if !c.inFn {
		c.RequestFrame()
	}
}

// offScreen reports whether the shell has an open surface and no window
// (surface or popup) on screen, the clock's low rate (S6.2b). Without any
// surface (before the first, after the last) it is false, so the frames of
// start-up keep the vsync rate.
func (s *Shell) offScreen() bool {
	open := false
	for _, sf := range s.surfaces {
		if !sf.dead {
			open = true
			break
		}
	}
	if !open {
		return false
	}
	for sf := range s.allWindows() {
		if sf.paintable() {
			return false
		}
	}
	return true
}

// wait is one idle step of the message loop, with the queue empty: the
// requested frame if it can start, else a wait for the clock's handles
// (arm) or the next message.
func (s *Shell) wait() error {
	c := s.clock
	handles, ms, now := c.arm()
	if now {
		// Sent through the listener, so a panic is recovered and re-raised
		// as from any window procedure.
		w32.SendMessage(s.listener, msgFrame, 0, 0)
		rethrow()
		return nil
	}
	if len(handles) > 0 || ms != w32.INFINITE {
		c.stats.waits++
	}
	r, err := w32.MsgWaitForMultipleObjectsEx(handles, ms, w32.QS_ALLINPUT, w32.MWMO_INPUTAVAILABLE)
	switch {
	case err != nil:
		return err
	case r < uint32(len(handles)):
		c.signaled(handles[r])
	case r == w32.WAIT_TIMEOUT && len(handles) > 0 && handles[0] != c.timer:
		c.stalled(handles)
	}
	return nil
}

// clockStats are the clock's counters, read by tests.
type clockStats struct {
	frames      int // frames run
	waits       int // waits armed with a timer, a timeout or a waitable
	chainWaits  int // waits on waitables
	stalls      int // waits on waitables that timed out
	modalFrames int // frames run from a modal loop
	// hiddenFrames are the frames run with no window on screen (S6.2b).
	hiddenFrames int
	// waitPaced are the frames whose successor the waitables pace alone (S6.2e).
	waitPaced int
}
