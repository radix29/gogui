//go:build windows

package win32

import (
	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// pacingSurfaces returns the windows (Shell.allWindows: surfaces and popups)
// whose waitables pace the next frame: the visible ones with a waitable,
// only the invalid ones if any are, at most MAXIMUM_WAIT_OBJECTS-1. The
// slice is the shell's scratch (pacingBuf), valid until the next call.
func (s *Shell) pacingSurfaces() []*swapTarget {
	all, invalid := s.pacingBuf[:0], false
	for sf := range s.allWindows() {
		if sf.waitable == 0 || !sf.paintable() {
			continue
		}
		switch {
		case sf.invalid && !invalid: // the valid ones so far drop out
			invalid, all = true, all[:0]
		case invalid && !sf.invalid:
			continue
		}
		all = append(all, sf)
	}
	s.pacingBuf = all
	return all[:min(len(all), w32.MAXIMUM_WAIT_OBJECTS-1)]
}

// signal takes a signal from the surface's waitable: a debt is paid,
// else the surface gets its ticket.
func (t *swapTarget) signal() {
	if t.debt > 0 {
		t.debt--
		return
	}
	t.ticket = true
}

// presented accounts for a Present: it spends the ticket or adds a debt,
// and tells the frame running, if any, what paces its successor.
func (t *swapTarget) presented() {
	t.presents++
	if c := t.s.clock; c.inFrame {
		c.framePresents++
		if t.waitable == 0 {
			c.unpaced = true
		}
	}
	if t.ticket {
		t.ticket = false
		return
	}
	t.debt++
}

// openWaitable keeps the chain's frame-latency waitable object for the
// clock, with a frame latency of 1; the waitable starts with one signal.
// Without one (no IDXGISwapChain2), the clock's hold alone paces the
// surface.
func (t *swapTarget) openWaitable() {
	t.ticket, t.debt = false, 0
	c2, err := t.chain.SwapChain2()
	if err != nil {
		return
	}
	defer c2.Release()
	if c2.SetMaximumFrameLatency(1) == nil {
		t.waitable = c2.FrameLatencyWaitableObject()
	}
}

// closeWaitable closes the waitable object, if any.
func (t *swapTarget) closeWaitable() {
	if t.waitable != 0 {
		_ = windows.CloseHandle(t.waitable)
		t.waitable = 0
	}
	t.ticket, t.debt = false, 0
}
