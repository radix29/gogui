package shell

import "time"

// FrameClock paces painting and animation to the display (DXGI waitable
// swap chains and DWM's vblank timing on Windows, GdkFrameClock on Linux,
// manual Advance in shell/fake). Frames happen only while requested, so an idle app uses no
// CPU (PLAN.md S6.2).
//
// A frame runs in this order: the frame function (animations advance and may
// invalidate surfaces), then SurfaceHandler.Paint of every surface that was
// invalidated, each at most once.
type FrameClock interface {
	// Now returns the current time on the clock's monotonic timeline, which
	// starts at or before Shell.Run. Event times use the same timeline.
	Now() time.Duration

	// SetFrameFunc sets the function called at the start of each requested
	// frame with the frame's target time (the expected presentation time on
	// the Now timeline, so animations step by whole frames even when the
	// call is late). nil removes it.
	SetFrameFunc(fn func(frameTime time.Duration))

	// RequestFrame asks for one call of the frame function at the next
	// frame. Requests are one-shot and coalesced: requesting several times
	// before the frame gives one call, and an animation that wants to keep
	// running requests again from inside the frame function, which schedules
	// the following frame. Surface.Invalidate requests a frame by itself.
	RequestFrame()

	// SetFrameObserver sets the function called with the timing of each
	// frame (FrameTiming) after it, on the UI thread; nil removes it.
	// Without an observer the clock measures nothing. It is the debug
	// frame-time log's and the frame benchmarks' hook (PLAN.md S6.2c);
	// the observer must not paint, and may request frames.
	SetFrameObserver(fn func(FrameTiming))

	// SetPresentObserver sets the function called with the PresentTiming of
	// each present the display reports shown, on the UI thread, some frames
	// after it; nil removes it. Without an observer the clock reads no
	// presentation feedback. It is the frame-pacing run's hook (PLAN.md
	// S6.2d); the observer must not paint, and may request frames.
	SetPresentObserver(fn func(PresentTiming))
}
