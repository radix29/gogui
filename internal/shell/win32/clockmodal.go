//go:build windows

package win32

import (
	"time"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// modal reports whether a surface is in a modal size/move or menu loop.
func (c *clock) modal() bool {
	for _, sf := range c.s.surfaces {
		if (sf.sizing || sf.menuLoop) && !sf.dead {
			return true
		}
	}
	return false
}

// modalChanged follows a surface entering or leaving a modal loop.
func (c *clock) modalChanged() {
	if !c.modal() {
		c.stopModal() // the message loop takes over
		return
	}
	c.cancelTimer()
	if c.requested && !c.inFrame {
		c.scheduleModal()
	}
}

// scheduleModal arranges a frame inside a modal loop: always the
// listener's WM_TIMER, even once the hold has passed. A posted message
// comes before input in the modal loop's GetMessage, so frames that each
// take a refresh (EndDraw waiting for the GPU) and post their successor
// would starve the loop of its mouse input: the window stops following the
// pointer, the button-up never arrives and the loop keeps the capture,
// which freezes the mouse for every window. WM_TIMER comes only when no
// input (and no posted message) is queued.
func (c *clock) scheduleModal() {
	if c.scheduled || c.s.quit || c.s.listener == 0 {
		return
	}
	c.scheduled = true
	ms := uint32(1)
	if wait := c.due() - c.now(); wait > 0 {
		ms = uint32((wait + time.Millisecond - 1) / time.Millisecond)
	}
	if w32.SetTimer(c.s.listener, timerFrame, ms) == nil {
		c.modalTimer = true
		return
	}
	// Fails only once the listener is gone.
	_ = w32.PostMessage(c.s.listener, msgFrame, frameModal, 0)
}

func (c *clock) stopModal() {
	if c.modalTimer {
		_ = w32.KillTimer(c.s.listener, timerFrame)
		c.modalTimer = false
	}
	c.scheduled = false
}

// modalFrame runs on the posted msgFrame and the WM_TIMER: the frame, if
// still in a modal loop (else the message loop runs it).
func (c *clock) modalFrame() {
	c.stopModal()
	if !c.modal() || !c.requested || c.inFrame {
		return
	}
	if c.now() < c.due() {
		c.scheduleModal()
		return
	}
	if !c.chainReady() {
		c.pollModal()
		return
	}
	c.stats.modalFrames++
	c.frame()
}

// chainReady reports whether a frame in a modal loop may start: as arm
// does for the message loop, once a pacing surface's waitable has
// signaled (its ticket) or the wait on them stalled. The modal loop can't
// wait on the waitables, so it polls them; a frame started before its
// waitable signals blocks the thread in Present or Commit for up to a
// refresh period, and frames back to back starve the size/move loop of
// its mouse input (the window lags the pointer and a drag to a screen edge
// loses its snap).
func (c *clock) chainReady() bool {
	pacing := c.s.pacingSurfaces()
	if len(pacing) == 0 {
		return true
	}
	handles := c.handles[:0]
	for _, sf := range pacing {
		for !sf.ticket {
			if ev, err := windows.WaitForSingleObject(sf.waitable, 0); err != nil || ev != windows.WAIT_OBJECT_0 {
				break
			}
			sf.signal()
		}
		if sf.ticket {
			return true
		}
		handles = append(handles, sf.waitable)
	}
	c.handles = handles
	t := c.now()
	if c.chainSince < 0 {
		c.chainSince = t
	}
	if t >= c.chainSince+stallTimeout {
		c.stalled(handles)
		return true
	}
	return false
}

// pollModal checks the waitables again on the listener's next WM_TIMER
// (USER_TIMER_MINIMUM, coarse: the system timer's tick).
func (c *clock) pollModal() {
	if c.scheduled || c.s.quit || c.s.listener == 0 {
		return
	}
	c.scheduled = true
	if w32.SetTimer(c.s.listener, timerFrame, 1) == nil {
		c.modalTimer = true
		return
	}
	c.scheduled = false
}
