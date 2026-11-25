package gtk4

import (
	"time"

	"github.com/radix29/gogui/internal/shell"
)

// Presentation feedback (PLAN.md S6.2d), over GdkFrameTimings. With a
// present observer set, every frame run from a tick is logged with GDK's
// frame counter of the tick (gdk_frame_clock_get_frame_counter) and the
// frame's target time; at each tick the logged frames before the current
// one are looked up in GDK's history (gdk_frame_clock_get_timings): a
// complete timings with a presentation time is reported, Shown its
// presentation time and Period its refresh interval; one without a
// presentation time (nothing drawn, or no presentation feedback from the
// compositor) or out of the history is dropped; the first one not complete
// yet ends the look-up until the next tick. GDK has no vblank count
// (Refresh is 0). A change of the pacing widget's frame clock clears the
// log. Frames run from the timeout or the watchdog have no GDK frame and
// are not logged, and the last frames of a run are reported only if
// another tick comes.

// feedbackLogSize is how many tick frames are logged: GDK keeps 16 frames
// of history.
const feedbackLogSize = 16

// feedbackRecord is a logged tick frame: GDK's frame counter (0: an empty
// slot) and the frame's target time.
type feedbackRecord struct {
	counter int64
	target  time.Duration
}

// feedback is the clock's presentation feedback state.
type feedback struct {
	observer   func(shell.PresentTiming)
	frameClock uintptr // the frame clock the log is about
	log        [feedbackLogSize]feedbackRecord
	next       int64 // the next counter to look up
}

// SetPresentObserver implements shell.FrameClock.
func (c *clock) SetPresentObserver(fn func(shell.PresentTiming)) {
	c.s.q.Check("gtk4.Clock.SetPresentObserver")
	c.fb.observer = fn
}

// feedbackTick runs at a tick of frameClock before its frame: it reports
// the logged frames GDK has presented since the last tick.
func (c *clock) feedbackTick(frameClock uintptr) {
	fb := &c.fb
	if fb.observer == nil {
		return
	}
	cur := c.l.frameCounter(frameClock)
	if frameClock != fb.frameClock {
		fb.frameClock, fb.log, fb.next = frameClock, [feedbackLogSize]feedbackRecord{}, cur
		return
	}
	fb.next = max(fb.next, cur-feedbackLogSize)
	for ; fb.next < cur && fb.observer != nil; fb.next++ {
		r := fb.log[fb.next%feedbackLogSize]
		if r.counter != fb.next {
			continue // not a logged frame
		}
		presented, interval, complete, gone := c.l.presentation(frameClock, fb.next)
		if gone {
			continue
		}
		if !complete {
			return
		}
		if presented == 0 {
			continue
		}
		period := c.period
		if interval > 0 {
			period = time.Duration(interval) * time.Microsecond
		}
		fb.observer(shell.PresentTiming{Target: r.target, Shown: c.at(presented), Period: period})
	}
}

// feedbackFrame logs the frame just run from a tick of frameClock.
func (c *clock) feedbackFrame(frameClock uintptr) {
	fb := &c.fb
	if fb.observer == nil || frameClock != fb.frameClock {
		return
	}
	n := c.l.frameCounter(frameClock)
	if n > 0 {
		fb.log[n%feedbackLogSize] = feedbackRecord{counter: n, target: c.target}
	}
}
