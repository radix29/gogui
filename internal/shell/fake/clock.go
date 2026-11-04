package fake

import (
	"time"

	"github.com/radix29/gogui/internal/shell"
)

// FrameInterval is the frame period of Clock: 60 Hz. Frames run at whole
// multiples of it on the Now timeline.
const FrameInterval = time.Second / 60

// Clock is a manual shell.FrameClock. Time starts at 0 and moves only with
// Advance. Create one with NewClock, or use the one of a Shell, which also
// paints the invalidated surfaces in each frame. A standalone Clock is used
// from one goroutine; the Shell's checks that it is the UI goroutine.
type Clock struct {
	now       time.Duration
	fn        func(frameTime time.Duration)
	requested bool
	inFrame   bool
	frames    int

	check     func(what string)             // UI-thread check, nil when standalone
	afterFunc func(frameTime time.Duration) // the shell's paint pass, nil when standalone
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

// RequestFrame implements shell.FrameClock. The frame runs at the next
// multiple of FrameInterval after Now (during Advance), or at once with
// Flush.
func (c *Clock) RequestFrame() {
	c.checkUI("Clock.RequestFrame")
	c.requested = true
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
// frame time. A frame function that requests again gets the following
// frame. Now ends at the old Now + d. Advance returns the number of frames
// it ran. It panics for a negative d and when called from inside a frame.
func (c *Clock) Advance(d time.Duration) int {
	c.checkUI("Clock.Advance")
	if d < 0 {
		panic("fake: Clock.Advance with a negative duration")
	}
	c.notInFrame("Advance")
	end := c.now + d
	n := 0
	for c.requested {
		next := (c.now/FrameInterval + 1) * FrameInterval
		if next > end {
			break
		}
		c.now = next
		c.runFrame(next)
		n++
	}
	c.now = end
	return n
}

// Flush runs one frame now, with Now as the frame time, if one is
// requested, and reports whether it did. Time does not move, and frames
// requested during this one wait for the next Flush or Advance. It panics
// when called from inside a frame.
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
}

// runFrame runs one frame: the frame function, then the shell's paints.
func (c *Clock) runFrame(t time.Duration) {
	c.requested = false
	c.inFrame = true
	defer func() { c.inFrame = false }()
	c.frames++
	if c.fn != nil {
		c.fn(t)
	}
	if c.afterFunc != nil {
		c.afterFunc(t)
	}
}
