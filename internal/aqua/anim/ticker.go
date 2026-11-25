package anim

import (
	"slices"
	"time"
)

// Ticker calls a function once per frame while it runs, from Start to
// Stop, on its Timeline's frame clock, with the time since the previous
// call (the first call: since Start). It is for work that follows the
// frames while an input lasts rather than motion, such as a text view's
// auto-scroll during a drag (PLAN.md S5.4i), so Reduce Motion does not
// stop it. The timeline requests frames while a ticker runs and none for a
// stopped one, so an idle ticker costs nothing.
//
// Tickers are called in Tick after the values' change funcs, in the order
// they started; the function may start or stop any ticker or value,
// including its own ticker. A ticker started during Tick is called from
// the next frame on.
type Ticker struct {
	tl     *Timeline
	fn     func(dt time.Duration)
	on     bool
	listed bool          // in tl.tickers
	fresh  bool          // started during the Tick in progress
	last   time.Duration // the time of the previous call, or of Start
}

// NewTicker returns a stopped ticker on the timeline that calls fn on
// every frame while it runs. It panics when fn is nil.
func (tl *Timeline) NewTicker(fn func(dt time.Duration)) *Ticker {
	if fn == nil {
		panic("anim: NewTicker with a nil function")
	}
	return &Ticker{tl: tl, fn: fn}
}

// Start starts the ticker and requests a frame (inside a Tick the
// timeline requests the next one at its end). Starting a running ticker
// does nothing: its next call still gets the time since its previous one.
func (t *Ticker) Start() {
	if t.on {
		return
	}
	t.on = true
	t.last = t.tl.now()
	t.fresh = t.tl.inTick
	if !t.listed {
		t.listed = true
		t.tl.tickers = append(t.tl.tickers, t)
	}
	if !t.tl.inTick {
		t.tl.clock.RequestFrame()
	}
}

// Stop stops the ticker; it is not called again until Start. Stopping a
// stopped ticker does nothing.
func (t *Ticker) Stop() {
	if !t.on {
		return
	}
	t.on = false
	if !t.tl.inTick { // during a Tick, endTick drops it
		t.listed = false
		t.tl.tickers = slices.DeleteFunc(t.tl.tickers, func(u *Ticker) bool { return u == t })
	}
}

// Running reports whether the ticker runs.
func (t *Ticker) Running() bool { return t.on }

// tick calls the function of a running ticker for the frame at frameTime.
func (t *Ticker) tick(frameTime time.Duration) {
	dt := max(frameTime-t.last, 0)
	t.last = frameTime
	t.fn(dt)
}
