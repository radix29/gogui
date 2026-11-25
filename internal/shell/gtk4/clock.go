package gtk4

import (
	"time"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell"
)

// The frame clock (S4.1e), over GdkFrameClock.
//
// Time: Now is g_get_monotonic_time (CLOCK_MONOTONIC), the timeline of
// GDK's frame times, so they convert onto it exactly; its 0 is when Run
// was called (begin back-dates it past GTK's loading).
//
// Pacing: a requested frame runs in one of two ways.
//
//   - With a pacing surface (Shell.pacingWidget: the first surface in
//     creation order whose widget can tick: a shown surface not known to be
//     minimized, surface.go), from
//     a tick callback on that widget (gtk_widget_add_tick_callback), in
//     the UPDATE phase of GDK's frame, so the paint pass's
//     gtk_widget_queue_draw calls are drawn in the same frame. The target
//     time is GDK's predicted presentation time
//     (gdk_frame_clock_get_refresh_info). The callback stays while frames
//     keep being requested and is removed (G_SOURCE_REMOVE) by the first
//     tick without a request, so an idle clock has no tick callback and
//     GDK's clock stops. While frames are requested a watchdog timeout
//     runs one if no tick came for stallTimeout: a compositor sends no
//     frame callbacks to a minimized or hidden window (Wayland), and the
//     frame function drives more than painting. The watchdog is armed once
//     and re-arms itself while ticks keep coming, so a running animation
//     costs no GLib source per frame. Once it has run a frame, and until
//     the next tick, it runs them hiddenInterval apart: a surface whose
//     ticks stopped is off screen as far as the clock can tell (S6.2b).
//   - Without one (before the first surface, or with every surface
//     hidden), from a one-shot g_timeout_add source on a 60 Hz grid: the
//     frame starts at its hold, at least half a period after the previous
//     frame's start, as on Win32 without a pacing surface. With open
//     surfaces and none of them pacing (each hidden or known to be
//     minimized: Shell.offScreen) the hold is at least hiddenInterval after
//     the previous frame's start (S6.2b), so a running animation costs 4
//     frames a second and its time keeps moving; a surface shown or
//     restored (surfacesChanged) moves the request to its tick at once.
//
// A frame runs the frame function, then the paint pass, with the frame's
// target time. Requests are coalesced; a request from inside the frame is
// scheduled after it. The clock's GLib sources and its tick callback hold
// dispatch handles to the clock (kindFrameTimeout, kindTick): a timeout's
// is freed by sourceFunc when the source goes or by cancelTimeout, a tick
// callback's by destroyNotify when GTK drops the callback, which also
// tells the clock (tickGone), so a widget destroyed without detach leaves
// no stale tick behind.

// defaultPeriod is the refresh period until GDK reports one with
// presentation feedback, and of the timeout fallback.
const defaultPeriod = time.Second / 60

// stallTimeout is how long requested frames wait for a tick before the
// watchdog runs one.
const stallTimeout = 100 * time.Millisecond

// hiddenInterval is the shortest time between two frame starts while no
// surface is on screen (S6.2b): 4 Hz, as on Win32.
const hiddenInterval = time.Second / 4

// clockLoop is the part of the loop the frame clock uses. Every method
// runs on the GTK thread.
type clockLoop interface {
	// monotonic returns g_get_monotonic_time, in microseconds.
	monotonic() int64
	// timeout adds a timeout source that calls p.source(k) through
	// sourceFunc after ms milliseconds (g_timeout_add). It returns the
	// source id and its handle, or 0, 0 if no source was added. The
	// clock removes it with loop.remove and frees the handle itself.
	timeout(ms uint32, p sourcePeer, k kind) (uint32, ffi.Handle)
	// addTick adds a tick callback to widget that calls p.tick(k, h, …)
	// through tickCallback at every frame of the widget's frame clock,
	// until it returns false. GTK frees its handle h (destroyNotify). It
	// returns the callback's id and h, or 0, 0 if none was added.
	addTick(widget uintptr, p tickPeer, k kind) (uint32, ffi.Handle)
	// removeTick removes the tick callback id from widget, which must
	// still exist (gtk_widget_remove_tick_callback).
	removeTick(widget uintptr, id uint32)
	// frameInfo returns the frame clock's frame time, its refresh interval
	// and the predicted presentation time of the first vblank at or after
	// the frame time (gdk_frame_clock_*), in microseconds on monotonic's
	// timeline. GDK reports its default interval (16667 µs) and no
	// presentation time (0) without recent presentation feedback.
	frameInfo(frameClock uintptr) (frameTime, interval, presentation int64)
	// frameCounter returns the frame clock's current frame counter
	// (gdk_frame_clock_get_frame_counter).
	frameCounter(frameClock uintptr) int64
	// presentation returns an earlier frame's presentation time (0: none)
	// and refresh interval in microseconds (gdk_frame_timings_*); complete
	// reports that they are final, gone that GDK's history no longer has
	// the frame.
	presentation(frameClock uintptr, counter int64) (presented, interval int64, complete, gone bool)
}

// clock is the shell's shell.FrameClock.
type clock struct {
	s *Shell
	l loop

	started bool  // begin has run (Run, after init)
	closed  bool  // close has run (shutdown)
	startUS int64 // Now's 0 on monotonic's timeline

	fn func(frameTime time.Duration)
	// paint runs after the frame function: the paint pass over the
	// invalidated surfaces (Shell.paintInvalid).
	paint func(frameTime time.Duration)

	requested bool // a frame is requested
	inFrame   bool // the frame function or the paint pass runs
	inFn      bool // the frame function or the momentum step runs: Invalidate requests no frame

	last     time.Duration // start of the last frame; -1 before the first
	target   time.Duration // target time of the last frame; -1 before the first
	hold     time.Duration // a timeout frame starts at or after this time
	period   time.Duration // GDK's refresh interval, or defaultPeriod
	lastTick time.Duration // when the last tick came; -1 before the first
	// requestedAt is when the pending request was made (its first
	// RequestFrame); the watchdog waits stallTimeout from the later of it
	// and lastTick, so a watchdog left over from an earlier burst cannot
	// run a new burst's first frame early.
	requestedAt time.Duration

	// The tick callback, if one is added: its widget, id and handle.
	tickWidget uintptr
	tickID     uint32
	tickH      ffi.Handle

	// The timeout source, if one is added: the fallback frame, or with
	// stall set the watchdog of the tick callback.
	timeoutID uint32
	timeoutH  ffi.Handle
	stall     bool
	// stalled: the watchdog ran the last frame and no tick has come since,
	// so it waits hiddenInterval (S6.2b).
	stalled bool

	// Counters (tests): frames run, those run from a tick, from the
	// fallback timeout and from the watchdog, and tick callbacks added.
	frames, tickFrames, timeoutFrames, stalls, ticksAdded int

	// frameTimer measures each frame for the observer (SetFrameObserver):
	// the paint pass expects a paint per queued draw, which the snapshots
	// report (surface.go).
	frameTimer shell.FrameTimer
	// fb is the presentation feedback (SetPresentObserver, pacing.go).
	fb feedback
}

var (
	_ shell.FrameClock = (*clock)(nil)
	_ tickPeer         = (*clock)(nil)
	_ tickGonePeer     = (*clock)(nil)
	_ sourcePeer       = (*clock)(nil)
)

func newClock(s *Shell, l loop) *clock {
	return &clock{s: s, l: l, last: -1, target: -1, lastTick: -1, period: defaultPeriod}
}

// begin starts the timeline (Run, after a successful init) at since
// before now, the time init took: Run's call is Now's 0. Requests are
// scheduled from then on.
func (c *clock) begin(since time.Duration) {
	c.startUS = c.l.monotonic() - max(since, 0).Microseconds()
	c.started = true
	c.schedule()
}

// close removes the tick callback and the timeout (shutdown). Requests are
// no longer scheduled. The tick widget must still exist: a surface whose
// widget paces detaches it before destroying it.
func (c *clock) close() {
	c.closed = true
	c.removeTick()
	c.cancelTimeout()
}

// at converts a time in microseconds on monotonic's timeline to Now's.
func (c *clock) at(us int64) time.Duration {
	return time.Duration(us-c.startUS) * time.Microsecond
}

func (c *clock) now() time.Duration { return c.at(c.l.monotonic()) }

// Now implements shell.FrameClock.
func (c *clock) Now() time.Duration {
	c.s.q.Check("gtk4.Clock.Now")
	return c.now()
}

// SetFrameFunc implements shell.FrameClock.
func (c *clock) SetFrameFunc(fn func(frameTime time.Duration)) {
	c.s.q.Check("gtk4.Clock.SetFrameFunc")
	c.fn = fn
}

// SetFrameObserver implements shell.FrameClock.
func (c *clock) SetFrameObserver(fn func(shell.FrameTiming)) {
	c.s.q.Check("gtk4.Clock.SetFrameObserver")
	c.frameTimer.SetObserver(fn)
}

// RequestFrame implements shell.FrameClock.
func (c *clock) RequestFrame() {
	c.s.q.Check("gtk4.Clock.RequestFrame")
	if c.requested {
		return
	}
	c.requested = true
	if c.started {
		c.requestedAt = c.now()
	}
	c.schedule()
}

// surfacesChanged follows a change of the pacing surface (a surface shown,
// minimized or closed): a pending request moves to the new one.
func (c *clock) surfacesChanged() { c.schedule() }

// detach removes the tick callback if widget carries it, before the
// widget is destroyed, and reschedules a pending request.
func (c *clock) detach(widget uintptr) {
	if widget != 0 && c.tickWidget == widget {
		c.removeTick()
	}
	c.schedule()
}

// schedule arranges the requested frame: a tick callback on the pacing
// surface with its watchdog, else the fallback timeout. Inside a frame it
// waits for the frame's end.
func (c *clock) schedule() {
	if !c.requested || c.inFrame || !c.started || c.closed || c.s.quit {
		return
	}
	w := c.s.pacingWidget()
	if c.tickID != 0 && c.tickWidget != w {
		c.removeTick() // the pacing surface changed
	}
	if w != 0 && c.tickID == 0 {
		if id, h := c.l.addTick(w, c, kindTick); id != 0 {
			c.tickWidget, c.tickID, c.tickH = w, id, h
			c.ticksAdded++
		}
	}
	if c.tickID != 0 {
		if c.timeoutID != 0 && !c.stall {
			c.cancelTimeout()
		}
		if c.timeoutID == 0 {
			d := stallTimeout
			if c.stalled {
				d = max(d, c.last+hiddenInterval-c.now())
			}
			c.addTimeout(d, true)
		}
		return
	}
	if c.timeoutID != 0 && c.stall {
		c.cancelTimeout()
	}
	if c.timeoutID == 0 {
		c.addTimeout(max(c.due()-c.now(), 0), false)
	}
}

// due returns when the fallback frame may start: the hold, but with open
// surfaces and none on screen at least hiddenInterval after the previous
// frame's start (S6.2b).
func (c *clock) due() time.Duration {
	if c.last >= 0 && c.s.offScreen() {
		return max(c.hold, c.last+hiddenInterval)
	}
	return c.hold
}

// watchdogDue returns when the watchdog runs the requested frame:
// stallTimeout after the later of the last tick and the request, and
// after a watchdog frame with no tick since, hiddenInterval after it.
func (c *clock) watchdogDue() time.Duration {
	d := max(c.lastTick, c.requestedAt) + stallTimeout
	if c.stalled {
		d = max(d, c.last+hiddenInterval)
	}
	return d
}

// addTimeout adds the timeout source after d (rounded up to whole ms).
func (c *clock) addTimeout(d time.Duration, stall bool) {
	ms := uint32((d + time.Millisecond - 1) / time.Millisecond)
	c.timeoutID, c.timeoutH = c.l.timeout(ms, c, kindFrameTimeout)
	c.stall = stall && c.timeoutID != 0
}

func (c *clock) cancelTimeout() {
	if c.timeoutID == 0 {
		return
	}
	c.l.remove(c.timeoutID)
	dispatch.Free(c.timeoutH)
	c.timeoutID, c.timeoutH, c.stall = 0, 0, false
}

// removeTick removes the tick callback. GTK frees its handle; tickGone
// then finds it is no longer the clock's.
func (c *clock) removeTick() {
	if c.tickID == 0 {
		return
	}
	w, id := c.tickWidget, c.tickID
	c.clearTick()
	c.l.removeTick(w, id)
}

func (c *clock) clearTick() { c.tickWidget, c.tickID, c.tickH = 0, 0, 0 }

// tickGone implements tickGonePeer: GTK dropped a tick callback. If it is
// the clock's (its widget was destroyed without detach), the clock forgets
// it, so it never calls GTK on that widget again, and moves a pending
// request elsewhere.
func (c *clock) tickGone(_ kind, h ffi.Handle) {
	if h == 0 || h != c.tickH {
		return
	}
	c.clearTick()
	c.schedule()
}

// tick implements tickPeer: a frame of the pacing surface's frame clock.
// It runs the requested frame and keeps the callback while frames are
// requested and the widget still paces. A callback that is not the
// clock's current one (h) goes at once.
func (c *clock) tick(_ kind, h ffi.Handle, frameClock uintptr) bool {
	if h == 0 || h != c.tickH {
		return false
	}
	done := false
	defer func() {
		if !done && c.tickH == h { // the frame panicked: tickCallback returns FALSE
			c.clearTick()
		}
	}()
	c.feedbackTick(frameClock)
	if c.requested && !c.inFrame && !c.closed && !c.s.quit {
		ft, iv, pr := c.l.frameInfo(frameClock)
		predicted := time.Duration(-1)
		if pr > 0 { // with presentation feedback GDK's interval is measured
			predicted = c.at(pr)
			if iv > 0 {
				c.period = time.Duration(iv) * time.Microsecond
			}
		}
		c.lastTick = c.now()
		c.stalled = false
		c.tickFrames++
		c.frame(c.at(ft), predicted)
		c.feedbackFrame(frameClock)
	}
	keep := c.requested && !c.closed && !c.s.quit && c.tickH == h && c.s.pacingWidget() == c.tickWidget
	if !keep && c.tickH == h {
		c.clearTick() // GTK removes it on return
	}
	c.schedule()
	done = true
	return keep
}

// source implements sourcePeer: the one-shot timeout, the fallback frame
// or the watchdog. The watchdog runs a frame only if neither a tick came nor the request
// was made within stallTimeout; otherwise it re-arms for the rest.
func (c *clock) source(kind) bool {
	stall := c.stall
	c.timeoutID, c.timeoutH, c.stall = 0, 0, false // sourceFunc frees the handle
	if !c.requested || c.inFrame || c.closed || c.s.quit {
		return false
	}
	now := c.now()
	if stall && c.tickID != 0 {
		if wait := c.watchdogDue() - now; wait > 0 {
			c.addTimeout(wait, true)
			return false
		}
	}
	c.stalled = stall
	if stall {
		c.stalls++
	} else {
		c.timeoutFrames++
	}
	c.frame(now, -1)
	c.schedule()
	return false
}

// frame runs the requested frame starting at t: the frame function, then
// the paint pass, with the target time frameTimes gives for GDK's
// prediction (-1: none).
func (c *clock) frame(t, predicted time.Duration) {
	c.target, c.hold = frameTimes(t, predicted, c.target, c.period)
	c.last = t
	c.requested = false
	c.frames++
	c.inFrame = true
	defer func() { c.inFrame, c.inFn = false, false }()
	c.frameTimer.Begin(c.target)
	if c.fn != nil {
		var start time.Time
		if c.frameTimer.Active() {
			start = time.Now()
		}
		c.inFn = true
		c.fn(c.target)
		c.inFn = false
		if c.frameTimer.Active() {
			c.frameTimer.FrameFunc(time.Since(start))
		}
	}
	if c.paint != nil {
		c.paint(c.target)
	}
	c.frameTimer.End()
}

// gridAfter returns the first time of the grid anchor + n·period after t.
func gridAfter(t, anchor, period time.Duration) time.Duration {
	d := t - anchor
	n := d / period
	if d < 0 && d%period != 0 {
		n-- // floor
	}
	return anchor + (n+1)*period
}

// frameTimes returns the target time and the hold of a frame starting at
// t, given the previous target prev (-1 before the first frame).
//
// The target is the first vblank after t: GDK's prediction (the vblank at
// or after its frame time) moved on by whole periods past t, or without
// one the first slot after t of the grid through prev (through 0 before
// the first frame). Targets step by at least half a period: one less than
// half a period after prev becomes prev + period. A target is never more
// than two periods after t, so a period that is wrong (GDK's default at a
// higher refresh rate) cannot make the targets run ahead: they then
// step by the real frame interval, two periods ahead of t. Targets always
// increase: a frame that starts before the previous one (a watchdog frame
// at Now, then a tick with GDK's older frame time) gets prev + 1 µs. The
// hold, the earliest start of the next fallback frame, is the target, or
// half a period after t if that is later.
func frameTimes(t, predicted, prev, period time.Duration) (target, hold time.Duration) {
	if period <= 0 {
		period = defaultPeriod
	}
	if predicted >= 0 {
		target = predicted
		if target <= t {
			target += ((t-target)/period + 1) * period
		}
	} else {
		target = gridAfter(t, max(prev, 0), period)
	}
	if prev >= 0 && target < prev+period/2 {
		target = prev + period
	}
	target = min(target, t+2*period)
	if prev >= 0 && target <= prev {
		target = prev + time.Microsecond
	}
	return target, max(target, t+period/2)
}

// invalidated requests the frame that paints a surface or a popup just
// invalidated, except while the frame function or the momentum step runs
// (deferInvalidate): the paint pass of that frame queues its draw, so an
// animation whose last change invalidates ends without an empty frame
// after it (PLAN.md S6.2a).
func (c *clock) invalidated() {
	if !c.inFn {
		c.RequestFrame()
	}
}

// deferInvalidate runs fn, part of the paint pass before its draws are
// queued, with Invalidate requesting no frame, as in the frame function.
func (c *clock) deferInvalidate(fn func()) {
	was := c.inFn
	c.inFn = true
	defer func() { c.inFn = was }()
	fn()
}
