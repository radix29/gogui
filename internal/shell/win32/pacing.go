//go:build windows

package win32

import (
	"time"

	"github.com/radix29/gogui/internal/shell"
)

// Presentation feedback (PLAN.md S6.2d). With a present observer set,
// every Present of a surface's chain is logged with its frame's target
// (presentLog, by the chain's present count, GetLastPresentCount), and
// the chain's frame statistics (GetFrameStatistics) are read right after
// it: they describe the most recent present the display has shown, about
// two presents earlier on a composition chain (measured on the host), with
// its vblank count (PresentRefreshCount) and the time of the vblank they
// were sampled at (SyncQPCTime, SyncRefreshCount). A present shown since
// the last report is reported once, Shown being its vblank on the clock's
// timeline: SyncQPCTime moved back by the refreshes between the two
// counts. Presents the statistics skip (two shown between two reads) are
// not reported; nor are the last ones of a run, until a later present
// reads them. The statistics are DISJOINT before the chain's first shown
// present and after a display mode change: nothing is reported then. A
// new chain counts from 1 again (createChain clears the log).

// presentLogSize is how many presents a surface remembers: more than the
// statistics' delay.
const presentLogSize = 8

// presentRecord is a logged present: the chain's present count and the
// target time of its frame (-1: a paint outside a frame).
type presentRecord struct {
	count  uint32
	target time.Duration
}

// SetPresentObserver implements shell.FrameClock.
func (c *clock) SetPresentObserver(fn func(shell.PresentTiming)) {
	c.s.q.Check("win32.Clock.SetPresentObserver")
	c.presentObserver = fn
}

// presentFeedback logs the present just made and reports the newest
// present the display has shown, if not reported yet (render, with an
// observer).
func (t *swapTarget) presentFeedback() {
	c := t.s.clock
	n, err := t.chain.LastPresentCount()
	if err != nil {
		return
	}
	target := time.Duration(-1)
	if c.inFrame {
		target = c.target
	}
	t.presentLog[n%presentLogSize] = presentRecord{count: n, target: target}
	st, err := t.chain.FrameStatistics()
	if err != nil || st.PresentCount == 0 || st.PresentCount <= t.lastShown {
		return
	}
	t.lastShown = st.PresentCount
	r := t.presentLog[st.PresentCount%presentLogSize]
	if r.count != st.PresentCount {
		return // older than the log
	}
	period, _ := c.grid()
	shown := c.ticks(st.SyncQPCTime-c.start) - time.Duration(st.SyncRefreshCount-st.PresentRefreshCount)*period
	c.presentObserver(shell.PresentTiming{Target: r.target, Shown: shown, Period: period,
		Refresh: uint64(st.PresentRefreshCount)})
}
