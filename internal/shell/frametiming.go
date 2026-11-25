package shell

import (
	"time"

	"github.com/radix29/gogui/internal/driver"
)

// FrameTiming is the timing of one frame, as a FrameClock's observer
// receives it: driver.FrameTiming, which documents the fields.
type FrameTiming = driver.FrameTiming

// FrameTimer collects the FrameTiming of each frame for a FrameClock's
// observer; the three shells share it. Its zero value has no observer,
// and then every method but SetObserver and Active does nothing, so a
// shell measures (calls time.Now) only while Active. UI thread only.
type FrameTimer struct {
	observer func(FrameTiming)
	cur      FrameTiming
	open     bool // cur is the record of a frame not reported yet
	expect   int  // paints still expected for cur (gtk4's queued draws)
}

// SetObserver sets the function that receives each frame's timing (nil:
// none). A record still open is dropped.
func (ft *FrameTimer) SetObserver(fn func(FrameTiming)) {
	ft.observer = fn
	ft.open, ft.expect = false, 0
}

// Active reports whether an observer is set, so the shell measures.
func (ft *FrameTimer) Active() bool { return ft.observer != nil }

// Begin opens the record of the frame with target time t; a record still
// open (a draw that never came) is reported first.
func (ft *FrameTimer) Begin(t time.Duration) {
	if ft.observer == nil {
		return
	}
	ft.flush()
	ft.cur = FrameTiming{Time: t}
	ft.open = true
}

// FrameFunc adds d to the open record's frame function.
func (ft *FrameTimer) FrameFunc(d time.Duration) {
	if ft.open {
		ft.cur.FrameFunc += d
	}
}

// Expect says that n more paints of the open record come after the frame
// (gtk4's queued draws); End then waits for them.
func (ft *FrameTimer) Expect(n int) {
	if ft.open {
		ft.expect += n
	}
}

// End ends the frame: its record is reported now, or after the paints
// Expect announced.
func (ft *FrameTimer) End() {
	if ft.open && ft.expect <= 0 {
		ft.flush()
	}
}

// Painted adds a surface's paint to the open record, which it reports if
// it was the last paint Expect announced. Without an open record the
// paint is reported on its own, with Time -1.
func (ft *FrameTimer) Painted(paint, submit, present time.Duration) {
	if ft.observer == nil {
		return
	}
	if !ft.open {
		ft.observer(FrameTiming{Time: -1, Paint: paint, Submit: submit, Present: present, Surfaces: 1})
		return
	}
	ft.cur.Paint += paint
	ft.cur.Submit += submit
	ft.cur.Present += present
	ft.cur.Surfaces++
	if ft.expect > 0 {
		ft.expect--
		if ft.expect == 0 {
			ft.flush()
		}
	}
}

// flush reports the open record, if any.
func (ft *FrameTimer) flush() {
	if !ft.open {
		return
	}
	ft.open, ft.expect = false, 0
	ft.observer(ft.cur)
}

// PresentTiming is when the display showed a presented frame (PLAN.md
// S6.2d), as a FrameClock's present observer receives it: some frames
// after the present (Win32: IDXGISwapChain::GetFrameStatistics, about two
// frames later; gtk4: GdkFrameTimings once complete; the fake shell: at
// once, on the frame's target). A shell may skip some: the record of a
// present the display reports no time for is never made.
type PresentTiming struct {
	// Target is the target time of the frame whose paint pass presented
	// it (FrameTiming.Time), -1 for a paint outside a frame.
	Target time.Duration
	// Shown is when it reached the screen, on the clock's Now timeline: the
	// vertical blank it was shown at.
	Shown time.Duration
	// Period is the display's refresh period then.
	Period time.Duration
	// Refresh is the display's vblank count at Shown (Win32's
	// PresentRefreshCount), 0 where the shell has none (gtk4).
	Refresh uint64
}
