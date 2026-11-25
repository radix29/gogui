package fake

import (
	"time"

	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/shellkit"
)

// FrameInterval is the frame period of Clock: 60 Hz. Frames run at whole
// multiples of it on the Now timeline.
const FrameInterval = time.Second / 60

// HiddenInterval is the shortest time between two frames of the Shell's
// clock while it has open surfaces and none on screen (every one hidden or
// minimized; S6.2b): 15 frame intervals, 4 Hz, as the real shells.
const HiddenInterval = 15 * FrameInterval

// Clock is a manual shell.FrameClock. Time starts at 0 and moves only with
// Advance. Create one with NewClock, or use the one of a Shell, which also
// paints the invalidated surfaces in each frame. A standalone Clock is used
// from one goroutine; the Shell's checks that it is the UI goroutine.
type Clock struct {
	now       time.Duration
	fn        func(frameTime time.Duration)
	requested bool
	inFrame   bool
	inFn      bool // the frame function runs: Invalidate requests no frame
	frames    int
	last      time.Duration // the time of the last frame; ran: one has run
	ran       bool

	// The timers of Shell.After (timer.go); round is the time of the last
	// round in this Advance (noRound before the first); inTimer is set
	// while one runs.
	timers  shellkit.Timers
	round   time.Duration
	inTimer bool

	check     func(what string)             // UI-thread check, nil when standalone
	afterFunc func(frameTime time.Duration) // the shell's paint pass, nil when standalone
	// offScreen reports whether the shell has open surfaces and none on
	// screen (the low rate); nil when standalone.
	offScreen func() bool

	// timer measures each frame for the observer (SetFrameObserver).
	frameTimer shell.FrameTimer
	// presentObserver receives a PresentTiming per paint of a frame
	// (SetPresentObserver).
	presentObserver func(shell.PresentTiming)
}

var _ shell.FrameClock = (*Clock)(nil)

// NewClock returns a standalone clock at time 0 with no frame requested.
func NewClock() *Clock { return &Clock{} }

func (c *Clock) checkUI(what string) {
	if c.check != nil {
		c.check(what)
	}
}

// Now implements shell.FrameClock.
func (c *Clock) Now() time.Duration {
	c.checkUI("Clock.Now")
	return c.now
}

// SetFrameFunc implements shell.FrameClock.
func (c *Clock) SetFrameFunc(fn func(frameTime time.Duration)) {
	c.checkUI("Clock.SetFrameFunc")
	c.fn = fn
}

// SetFrameObserver implements shell.FrameClock. The timings are wall
// time, not the clock's.
func (c *Clock) SetFrameObserver(fn func(shell.FrameTiming)) {
	c.checkUI("Clock.SetFrameObserver")
	c.frameTimer.SetObserver(fn)
}

// SetPresentObserver implements shell.FrameClock. Each surface or popup
// painted in a frame is shown at once, on the frame's target: Shown is
// the frame time, Period FrameInterval and Refresh the frame time in
// frame intervals. A paint outside a frame (Flush paints are in one)
// reports nothing.
func (c *Clock) SetPresentObserver(fn func(shell.PresentTiming)) {
	c.checkUI("Clock.SetPresentObserver")
	c.presentObserver = fn
}

// presented reports a paint to the present observer, if any.
func (c *Clock) presented() {
	if c.presentObserver == nil || !c.inFrame {
		return
	}
	c.presentObserver(shell.PresentTiming{Target: c.last, Shown: c.last, Period: FrameInterval,
		Refresh: uint64(c.last / FrameInterval)})
}

// RequestFrame implements shell.FrameClock. The frame runs at the next
// multiple of FrameInterval after Now (during Advance), or at once with
// Flush.
func (c *Clock) RequestFrame() {
	c.checkUI("Clock.RequestFrame")
	c.requested = true
}

// invalidated requests the frame that paints a surface or a popup just
// invalidated, except while the frame function runs: the paint pass of
// that frame paints it, so an animation whose last change invalidates
// ends without an empty frame after it (PLAN.md S6.2a).
func (c *Clock) invalidated() {
	if !c.inFn {
		c.RequestFrame()
	}
}

// Requested reports whether a frame has been requested and not run yet.
func (c *Clock) Requested() bool {
	c.checkUI("Clock.Requested")
	return c.requested
}

// Frames returns the number of frames run so far.
func (c *Clock) Frames() int {
	c.checkUI("Clock.Frames")
	return c.frames
}

// Advance moves Now forward by d and runs every requested frame on the way:
// while a frame is requested and the next multiple of FrameInterval after
// Now is at most the end time, Now jumps there and the frame runs with that
// frame time; while the Shell has open surfaces and none on screen, a
// frame runs no sooner than HiddenInterval after the previous one, on the
// grid (S6.2b), and showing or restoring one ends that at once. A frame
// function that requests again gets the following
// frame. The Shell's timers (After) due by the end time fire on the way
// too, in deadline order, Now at each deadline; a timer due at or before a
// pending frame's time fires first. A timer made by a timer's fn and due
// at once waits for the next time Now moves (timer.go's rounds). Now ends at the old Now + d. Advance
// returns the number of frames it ran. It panics for a negative d and when
// called from inside a frame or a timer.
func (c *Clock) Advance(d time.Duration) int {
	c.checkUI("Clock.Advance")
	if d < 0 {
		panic("fake: Clock.Advance with a negative duration")
	}
	c.notInFrame("Advance")
	end := c.now + d
	c.round = noRound
	n := 0
	for {
		frame := time.Duration(-1) // the pending frame's time, if by end
		late := false              // frame is held back by the low rate
		if c.requested {
			next := (c.now/FrameInterval + 1) * FrameInterval
			if c.ran && c.offScreen != nil && c.offScreen() {
				// Off screen: on the grid, HiddenInterval after the last.
				if h := (c.last + HiddenInterval + FrameInterval - 1) / FrameInterval * FrameInterval; h > next {
					next, late = h, true
				}
			}
			if next <= end {
				frame = next
			}
		}
		if frame < 0 {
			// No frame by end: the timers up to end; one may request a
			// frame, which is then looked at again.
			if c.fireDue(end) && c.requested {
				continue
			}
			break
		}
		// Nothing withdraws a request, so the frame still runs; but a
		// timer that shows a surface ends the low rate, so a late frame
		// is looked at again.
		if c.fireDue(frame) && late {
			continue
		}
		c.now = frame
		c.runFrame(frame)
		n++
	}
	c.now = end
	return n
}

// Flush runs one frame now, with Now as the frame time, if one is
// requested, and reports whether it did. Time does not move, and frames
// requested during this one wait for the next Flush or Advance; no timer
// fires. It panics when called from inside a frame or a timer.
func (c *Clock) Flush() bool {
	c.checkUI("Clock.Flush")
	c.notInFrame("Flush")
	if !c.requested {
		return false
	}
	c.runFrame(c.now)
	return true
}

func (c *Clock) notInFrame(what string) {
	if c.inFrame {
		panic("fake: Clock." + what + " called from inside a frame")
	}
	if c.inTimer {
		panic("fake: Clock." + what + " called from inside a timer")
	}
}

// runFrame runs one frame: the frame function, then the shell's paints.
func (c *Clock) runFrame(t time.Duration) {
	c.requested = false
	c.inFrame = true
	defer func() { c.inFrame = false }()
	c.frames++
	c.last, c.ran = t, true
	c.frameTimer.Begin(t)
	if c.fn != nil {
		var start time.Time
		if c.frameTimer.Active() {
			start = time.Now()
		}
		c.inFn = true
		func() {
			defer func() { c.inFn = false }()
			c.fn(t)
		}()
		if c.frameTimer.Active() {
			c.frameTimer.FrameFunc(time.Since(start))
		}
	}
	if c.afterFunc != nil {
		c.afterFunc(t)
	}
	c.frameTimer.End()
}
