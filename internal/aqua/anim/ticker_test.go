package anim_test

import (
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua/anim"
)

// TestTicker: a ticker is called once per frame with the time since its
// previous call (since Start for the first), requests frames only while it
// runs, and is counted by Running.
func TestTicker(t *testing.T) {
	r := newRig()
	var got []time.Duration
	tk := r.tl.NewTicker(func(dt time.Duration) { got = append(got, dt) })
	if tk.Running() || r.clk.Requested() || r.tl.Running() != 0 {
		t.Fatal("a new ticker runs")
	}
	r.clk.Advance(frame / 4)
	tk.Start()
	tk.Start() // running: nothing
	if !tk.Running() || !r.clk.Requested() || r.tl.Running() != 1 {
		t.Fatal("Start did not start it")
	}
	r.step(t)
	r.step(t)
	r.clk.Advance(3 * frame) // three more frames
	if len(got) != 5 || got[0] != frame-frame/4 || got[1] != frame || got[4] != frame {
		t.Fatalf("calls %v", got)
	}
	tk.Stop()
	tk.Stop()
	if tk.Running() || r.tl.Running() != 0 {
		t.Fatal("Stop did not stop it")
	}
	if r.clk.Advance(time.Second) > 1 || len(got) != 5 || r.clk.Requested() {
		t.Fatalf("frames after Stop: calls %v, requested %v", got, r.clk.Requested())
	}
	// Started again: the first call counts from the new Start.
	got = nil
	r.clk.Advance(frame / 2)
	start := r.clk.Now()
	tk.Start()
	if next := r.step(t); len(got) != 1 || got[0] != next-start {
		t.Fatalf("after a restart: %v", got)
	}
}

// TestTickerReduceMotion: Reduce Motion stops values but not tickers.
func TestTickerReduceMotion(t *testing.T) {
	r := newRig()
	n := 0
	tk := r.tl.NewTicker(func(time.Duration) { n++ })
	r.tl.SetReduceMotion(true)
	tk.Start()
	r.step(t)
	r.step(t)
	r.tl.SetReduceMotion(false)
	r.tl.SetReduceMotion(true)
	r.step(t)
	if n != 3 || !tk.Running() {
		t.Fatalf("%d calls with Reduce Motion, running %v", n, tk.Running())
	}
}

// TestTickerInTick: a ticker may stop itself and start others from its
// call or from a change func; one started in a Tick is called from the
// next frame on, in the order of the list, and one stopped is not called
// again.
func TestTickerInTick(t *testing.T) {
	r := newRig()
	var order []string
	var a, b, c *anim.Ticker
	a = r.tl.NewTicker(func(time.Duration) {
		order = append(order, "a")
		a.Stop()
		b.Start()
	})
	b = r.tl.NewTicker(func(dt time.Duration) {
		order = append(order, "b")
		if dt != frame {
			t.Errorf("b's first dt %v", dt)
		}
		b.Stop()
		c.Stop()
	})
	c = r.tl.NewTicker(func(time.Duration) { order = append(order, "c") })
	a.Start()
	c.Start()
	r.step(t) // a stops itself and starts b; c runs
	r.step(t) // c runs (listed before b), then b stops itself and c
	if r.clk.Requested() || r.tl.Running() != 0 {
		t.Error("frames requested with every ticker stopped")
	}
	if got := len(order); got != 4 || order[0] != "a" || order[1] != "c" || order[2] != "c" || order[3] != "b" {
		t.Errorf("calls %v", order)
	}
	// From a value's change func: started in the Tick, called from the next.
	n := 0
	d := r.tl.NewTicker(func(time.Duration) { n++ })
	v := r.tl.NewValue(0, func(float64) { d.Start() })
	v.AnimateTo(1, frame, anim.Linear)
	r.step(t)
	if n != 0 || !d.Running() || !r.clk.Requested() {
		t.Fatalf("started from a change func: %d calls, running %v", n, d.Running())
	}
	r.step(t)
	if n != 1 {
		t.Errorf("%d calls a frame later", n)
	}
	d.Stop()
}

// TestTickerPanic: a panicking ticker leaves the timeline usable.
func TestTickerPanic(t *testing.T) {
	r := newRig()
	boom := true
	n := 0
	tk := r.tl.NewTicker(func(time.Duration) {
		n++
		if boom {
			boom = false
			panic("boom")
		}
	})
	tk.Start()
	func() {
		defer func() { _ = recover() }()
		r.clk.Advance(frame)
	}()
	if !tk.Running() || !r.clk.Requested() {
		t.Fatal("the ticker stopped after a panic")
	}
	r.step(t)
	if n != 2 {
		t.Errorf("%d calls", n)
	}
	mustPanicWith(t, "nil function", func() { r.tl.NewTicker(nil) })
}
