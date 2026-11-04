//go:build linux

package gtk4

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// The frame clock over the real GLib and GDK (S4.1e). The pure-Go logic
// (frameTimes, coalescing, the watchdog) is host-tested in clock_test.go;
// these tests check what only GTK can say: that g_get_monotonic_time is
// the timeline GDK's frame times are on, that g_timeout_add paces the
// fallback, that a tick callback on a mapped GGSurface ticks, and that
// GTK calls the tick callback's GDestroyNotify in every case the clock
// relies on (G_SOURCE_REMOVE, gtk_widget_remove_tick_callback, widget
// disposal).
//
// GTK semantics assumed (gtk/gtkwidget.c, GTK 4.14+):
//   - gtk_widget_add_tick_callback always adds the callback and returns
//     its id; it only connects to the frame clock's "update" signal (and
//     calls gdk_frame_clock_begin_updating) if the widget is realized, else
//     gtk_widget_realize does that later. So callbacks only run while the
//     widget is realized, in the UPDATE phase of its frame clock.
//   - The callback info is refcounted; its GDestroyNotify runs when the
//     count drops to 0: right after the callback returns G_SOURCE_REMOVE,
//     inside gtk_widget_remove_tick_callback (or after the callback returns
//     if it is removed while it runs), and from gtk_widget_real_destroy
//     (destroy_tick_callbacks), which gtk_widget_dispose calls, i.e. when
//     the last reference is dropped (gtk_window_destroy for a toplevel; for
//     a child, the parent's unparent plus every other unref).
//     gtk_widget_unrealize alone only disconnects from the frame clock.
//   - gdk_frame_clock_get_refresh_info writes the default interval
//     (16667 µs) and a presentation time of 0 unless the frame timings
//     carry presentation feedback; a reported presentation time is at or
//     after the base time.
//
// The tests that do not go through Run drive the clock by hand on the GTK
// goroutine of TestMain (class_linux_test.go), over a shell that is never
// run: its queue is not bound and its checks are off, and the clock's
// begin is called directly. The main context is iterated with
// iterateUntil, bounded by a deadline.

// frameDeadline bounds every wait for the frame clock or for a window.
const frameDeadline = 5 * time.Second

// iterateUntil iterates the default main context (non-blocking, with a
// short sleep when nothing was ready) until done returns true or d has
// passed, and reports whether done returned true. GTK goroutine only.
func iterateUntil(d time.Duration, done func() bool) bool {
	deadline := time.Now().Add(d)
	for !done() {
		if time.Now().After(deadline) {
			return false
		}
		if gtk.GLib.MainContextIteration(0, gtk.False) == gtk.False {
			time.Sleep(time.Millisecond)
		}
	}
	return true
}

// presentSurface presents a GtkWindow with a GGSurface child for p and
// waits until the child is mapped and has been drawn once, so its frame
// clock runs. The caller destroys both with destroySurface. GTK goroutine
// only.
func presentSurface(p *fakeSurface) (win, w uintptr, err error) {
	if testGtk.WindowDestroy == nil {
		return 0, 0, errors.New("gtk_window_destroy did not bind")
	}
	win = gtk.Gtk.WindowNew()
	if win == 0 {
		return 0, 0, errors.New("gtk_window_new returned NULL")
	}
	w = newSurfaceWidget(p)
	if w == 0 {
		testGtk.WindowDestroy(win)
		return 0, 0, errors.New("newSurfaceWidget returned 0")
	}
	gtk.Gtk.WindowSetTitle(win, "gogui clock test")
	gtk.Gtk.WindowSetDefaultSize(win, 160, 100)
	gtk.Gtk.WindowSetChild(win, w)
	gtk.Gtk.WindowPresent(win)
	if !iterateUntil(frameDeadline, func() bool {
		return gtk.Gtk.WidgetGetMapped(w) != gtk.False && p.snapshots > 0
	}) {
		mapped := gtk.Gtk.WidgetGetMapped(w)
		destroySurface(win, w)
		return 0, 0, fmt.Errorf("the window was not mapped and drawn within %v (mapped %d, %d snapshots)",
			frameDeadline, mapped, p.snapshots)
	}
	return win, w, nil
}

// destroySurface destroys the window (GTK drops its reference, the child
// is unparented) and drops the test's reference to the child, which
// disposes it; then it lets GTK process what that queued.
func destroySurface(win, w uintptr) {
	if win != 0 {
		testGtk.WindowDestroy(win)
	}
	releaseSurfaceWidget(w)
	for i := 0; i < 100 && gtk.GLib.MainContextIteration(0, gtk.False) != gtk.False; i++ {
	}
}

// gtkPacer is an openSurface over a real window: its GGSurface paces
// until gone is set (the surface no longer offers its widget).
type gtkPacer struct {
	s      *Shell
	win, w uintptr
	gone   bool
}

func (p *gtkPacer) tickWidget() uintptr {
	if p.gone {
		return 0
	}
	return p.w
}

// closeNow follows openSurface's contract: the surface stops pacing and
// leaves Shell.surfaces, detaches its widget from the clock, then
// destroys it.
func (p *gtkPacer) closeNow(bool) {
	p.gone = true
	if i := slices.Index(p.s.surfaces, openSurface(p)); i >= 0 {
		p.s.surfaces = slices.Delete(p.s.surfaces, i, i+1)
	}
	if p.w != 0 {
		p.s.clock.detach(p.w)
		destroySurface(p.win, p.w)
		p.win, p.w = 0, 0
	}
}

// infoLoop is the real gtkLoop, recording what GDK's frameInfo returned
// and the handles of the tick callbacks added.
type infoLoop struct {
	*gtkLoop
	infos []frameInfo
	ticks []ffi.Handle
}

type frameInfo struct{ frameTime, interval, presentation int64 }

func (l *infoLoop) frameInfo(fc uintptr) (int64, int64, int64) {
	ft, iv, pr := l.gtkLoop.frameInfo(fc)
	l.infos = append(l.infos, frameInfo{ft, iv, pr})
	return ft, iv, pr
}

func (l *infoLoop) addTick(w uintptr, p tickPeer, k kind) (uint32, ffi.Handle) {
	id, h := l.gtkLoop.addTick(w, p, k)
	if id != 0 {
		l.ticks = append(l.ticks, h)
	}
	return id, h
}

// newGTKClock returns a shell over infoLoop that is never run, for the
// tests that drive the clock by hand; its clock has not begun.
func newGTKClock(t *testing.T) (*Shell, *clock, *infoLoop) {
	t.Helper()
	needGTK(t)
	resetPanics(t)
	l := &infoLoop{gtkLoop: &gtkLoop{report: func(err error) { t.Errorf("gtk.Report(%v)", err) }}}
	s := newShell(l)
	return s, s.clock, l
}

// frameRec is one call of a test's frame function.
type frameRec struct {
	target time.Duration // the frame time passed in
	start  time.Duration // the frame's start (clock.last)
	now    time.Duration // Now at the call
	wall   time.Time
	period time.Duration // the clock's period at the call
	// The clock's counters at the call (they count this frame already).
	ticks, timeouts, stalls int
	info                    frameInfo // GDK's, for a tick frame
}

func record(c *clock, l *infoLoop, recs *[]frameRec) {
	r := frameRec{
		target: c.target, start: c.last, now: c.now(), wall: time.Now(), period: c.period,
		ticks: c.tickFrames, timeouts: c.timeoutFrames, stalls: c.stalls,
	}
	if l != nil && len(l.infos) > 0 {
		r.info = l.infos[len(l.infos)-1]
	}
	*recs = append(*recs, r)
}

// TestClockGTKMonotonic: g_get_monotonic_time never goes back, moves with
// Go's monotonic clock (both read CLOCK_MONOTONIC), and the clock's Now
// starts at begin's back-dating. It needs GTK loaded, not a display.
func TestClockGTKMonotonic(t *testing.T) {
	if gtkInit != nil && !errors.Is(gtkInit, errNoDisplay) {
		t.Fatal(gtkInit)
	}
	mono := gtk.GLib.GetMonotonicTime
	prev := mono()
	if prev <= 0 {
		t.Fatalf("g_get_monotonic_time = %d", prev)
	}
	for i := range 10000 {
		m := mono()
		if m < prev {
			t.Fatalf("call %d: g_get_monotonic_time went back from %d to %d", i, prev, m)
		}
		prev = m
	}
	const sleep = 50 * time.Millisecond
	m0, t0 := mono(), time.Now()
	time.Sleep(sleep)
	m1, t1 := mono(), time.Now()
	glib, goDelta := time.Duration(m1-m0)*time.Microsecond, t1.Sub(t0)
	if glib < sleep-time.Millisecond { // µs truncation at both ends
		t.Errorf("g_get_monotonic_time moved %v over a %v sleep", glib, sleep)
	}
	if d := glib - goDelta; d < -2*time.Millisecond || d > 2*time.Millisecond {
		t.Errorf("g_get_monotonic_time moved %v, Go's monotonic clock %v", glib, goDelta)
	}

	s := newShell(&gtkLoop{report: func(error) {}})
	c := s.clock
	c.begin(0)
	if n := c.Now(); n < 0 || n > 20*time.Millisecond {
		t.Errorf("Now right after begin(0) = %v, want about 0", n)
	}
	time.Sleep(20 * time.Millisecond)
	if n := c.Now(); n < 20*time.Millisecond-time.Microsecond || n > time.Second {
		t.Errorf("Now 20 ms after begin(0) = %v", n)
	}
	c.close()

	s2 := newShell(&gtkLoop{report: func(error) {}})
	s2.clock.begin(300 * time.Millisecond) // Run back-dates by the time init took
	if n := s2.clock.Now(); n < 300*time.Millisecond || n > 320*time.Millisecond {
		t.Errorf("Now right after begin(300ms) = %v, want about 300ms", n)
	}
	s2.clock.close()
}

// TestClockGTKTimeoutFallback: in a real Run with no surface, requested
// frames run from the g_timeout_add fallback, on the 60 Hz grid through 0,
// each after its start, at least half a period after the previous one,
// and a request from inside the frame function is honoured. Quit from the
// last frame ends Run with nil and leaves no handle.
func TestClockGTKTimeoutFallback(t *testing.T) {
	const n = 8
	s, reports := newGTKShell(t)
	base := dispatch.Len()
	c := s.clock
	var recs []frameRec
	var readyNow time.Duration
	p, err := runGTK(t, s, &handler{ready: func() {
		clk := s.Clock()
		readyNow = clk.Now()
		clk.SetFrameFunc(func(time.Duration) {
			record(c, nil, &recs)
			if len(recs) < n {
				clk.RequestFrame()
				clk.RequestFrame() // coalesced
			} else {
				s.Quit()
			}
		})
		clk.RequestFrame()
	}})
	if p != nil || err != nil {
		t.Fatalf("Run = %v, panicked with %v", err, p)
	}
	if len(recs) != n {
		t.Fatalf("%d frames ran, want %d", len(recs), n)
	}
	if c.timeoutFrames != n || c.tickFrames != 0 || c.stalls != 0 || c.ticksAdded != 0 {
		t.Errorf("%d timeout, %d tick, %d watchdog frames and %d tick callbacks; want %d timeout frames only",
			c.timeoutFrames, c.tickFrames, c.stalls, c.ticksAdded, n)
	}
	if readyNow < 0 || readyNow > 5*time.Second {
		t.Errorf("Now in Ready = %v", readyNow)
	}
	for i, r := range recs {
		if r.timeouts != i+1 {
			t.Errorf("frame %d did not run from the fallback timeout (counters %+v)", i, r)
		}
		// frameTimes: the first grid slot after the start; the grid is
		// through the previous target, through 0 before the first, and
		// every target is on it while the period is defaultPeriod.
		if r.target <= r.start {
			t.Errorf("frame %d: frame time %v not after its start %v", i, r.target, r.start)
		}
		if r.target%defaultPeriod != 0 {
			t.Errorf("frame %d: frame time %v is not on the 60 Hz grid through 0", i, r.target)
		}
		if r.target > r.start+defaultPeriod {
			t.Errorf("frame %d: frame time %v more than a period after its start %v", i, r.target, r.start)
		}
		// Now at the call is after the start; the target is the next slot,
		// so normally ahead of it too. The slack allows for a stall of the
		// thread between the source and the frame function.
		if r.now < r.start || r.target+2*time.Millisecond < r.now {
			t.Errorf("frame %d: start %v, Now at the call %v, frame time %v", i, r.start, r.now, r.target)
		}
		if i == 0 {
			if r.start < readyNow {
				t.Errorf("the first frame started at %v, before Ready's Now %v", r.start, readyNow)
			}
			continue
		}
		prev := recs[i-1]
		if r.target <= prev.target {
			t.Errorf("frame %d: frame time %v not after %v", i, r.target, prev.target)
		}
		// The hold: the next fallback frame starts at or after the target,
		// and at least half a period after the previous start. GLib fires
		// a timeout at or after its expiry on the same clock, so this is
		// exact on the clock's timeline; the wall time is read later in
		// the frame function, so it gets a little slack.
		if d := r.start - prev.start; d < defaultPeriod/2 || r.start < prev.target {
			t.Errorf("frame %d started %v after frame %d (at %v, the previous target %v)", i, d, i-1, r.start, prev.target)
		}
		if d := r.wall.Sub(prev.wall); d < defaultPeriod/2-time.Millisecond {
			t.Errorf("frame %d ran %v of wall time after frame %d, want ≥ half a period", i, d, i-1)
		}
	}
	t.Logf("frame times %v", func() (ts []time.Duration) {
		for _, r := range recs {
			ts = append(ts, r.target)
		}
		return ts
	}())
	if c.timeoutID != 0 || c.tickID != 0 {
		t.Errorf("after Run: timeout %d, tick %d left", c.timeoutID, c.tickID)
	}
	checkGTKTornDown(t, s, reports, base)
}

// TestClockGTKTick: with a mapped GGSurface pacing, requested frames run
// from its tick callback in GDK's UPDATE phase: the frame starts at GDK's
// frame time, the frame time is what frameTimes gives for GDK's
// prediction (under xvfb usually none, so the grid), frame times strictly
// increase; after the requests stop, the first idle tick returns
// G_SOURCE_REMOVE, GTK calls the GDestroyNotify, and the handle is freed.
func TestClockGTKTick(t *testing.T) {
	const n = 12
	s, c, l := newGTKClock(t)
	onGTK(t, func() error {
		base := dispatch.Len()
		p := &fakeSurface{min: [2]int32{160, 100}, nat: [2]int32{160, 100}, color: gtk.RGBA{R: 0.2, G: 0.4, B: 0.8, A: 1}}
		win, w, err := presentSurface(p)
		if err != nil {
			return err
		}
		ps := &gtkPacer{s: s, win: win, w: w}
		s.surfaces = append(s.surfaces, ps)
		defer func() {
			c.close()
			ps.closeNow(true)
		}()
		c.begin(0)
		var recs []frameRec
		c.SetFrameFunc(func(time.Duration) {
			record(c, l, &recs)
			if len(recs) < n {
				c.RequestFrame()
			}
		})
		c.RequestFrame()
		if c.tickID == 0 || len(l.ticks) != 1 {
			return fmt.Errorf("RequestFrame added no tick callback (tickID %d, %d added)", c.tickID, len(l.ticks))
		}
		h := l.ticks[0]
		if !iterateUntil(frameDeadline, func() bool { return len(recs) >= n }) {
			return fmt.Errorf("%d of %d frames within %v (%d from ticks, %d from the watchdog): GDK's frame clock did not tick for the mapped window",
				len(recs), n, frameDeadline, c.tickFrames, c.stalls)
		}
		// Idle: the next tick finds no request and removes the callback;
		// the lazy watchdog goes when it next fires.
		if !iterateUntil(frameDeadline, func() bool { return c.tickID == 0 && c.timeoutID == 0 }) {
			return fmt.Errorf("after the last request: tick callback %d, timeout %d still there after %v", c.tickID, c.timeoutID, frameDeadline)
		}
		var errs []error
		if dispatch.Valid(h) {
			errs = append(errs, errors.New("the removed tick callback's handle is still live: GTK did not call its GDestroyNotify"))
		}
		if got := dispatch.Len(); got != base+1 { // the GGSurface's own handle
			errs = append(errs, fmt.Errorf("idle: dispatch.Len() = %d, want %d", got, base+1))
		}
		if len(recs) != n || c.frames != n {
			errs = append(errs, fmt.Errorf("%d frames recorded, %d run; want %d", len(recs), c.frames, n))
		}
		if c.tickFrames == 0 {
			errs = append(errs, fmt.Errorf("no frame ran from a tick (%d from the watchdog)", c.stalls))
		} else if c.stalls > n/2 {
			errs = append(errs, fmt.Errorf("%d of %d frames ran from the watchdog: ticks came less often than every %v", c.stalls, n, stallTimeout))
		}
		if c.ticksAdded != 1 {
			errs = append(errs, fmt.Errorf("%d tick callbacks added, want 1 for a run of frames", c.ticksAdded))
		}
		prevTarget, withPrediction := time.Duration(-1), 0
		for i, r := range recs {
			if i > 0 && r.target <= prevTarget {
				errs = append(errs, fmt.Errorf("frame %d: frame time %v not after %v", i, r.target, prevTarget))
			}
			if r.target <= r.start {
				errs = append(errs, fmt.Errorf("frame %d: frame time %v not after its start %v", i, r.target, r.start))
			}
			isTick := r.ticks > 0 && (i == 0 || r.ticks > recs[i-1].ticks)
			if isTick {
				in := r.info
				if in.frameTime <= 0 {
					errs = append(errs, fmt.Errorf("frame %d: GDK frame time %d", i, in.frameTime))
				}
				// GDK writes its default 16667 µs without presentation
				// feedback (gdk_frame_clock_get_refresh_info).
				if in.interval <= 0 {
					errs = append(errs, fmt.Errorf("frame %d: GDK refresh interval %d µs, want > 0", i, in.interval))
				}
				if in.presentation != 0 && in.presentation < in.frameTime {
					errs = append(errs, fmt.Errorf("frame %d: GDK presentation %d before its frame time %d", i, in.presentation, in.frameTime))
				}
				if start := c.at(in.frameTime); r.start != start {
					errs = append(errs, fmt.Errorf("frame %d: start %v, want GDK's frame time %v", i, r.start, start))
				}
				if d := r.now - r.start; d < -time.Second || d > time.Second {
					errs = append(errs, fmt.Errorf("frame %d: GDK's frame time %v is %v away from Now", i, r.start, d))
				}
				predicted := time.Duration(-1)
				if in.presentation > 0 {
					predicted = c.at(in.presentation)
					withPrediction++
					if in.interval > 0 && r.period != time.Duration(in.interval)*time.Microsecond {
						errs = append(errs, fmt.Errorf("frame %d: period %v, want GDK's %d µs", i, r.period, in.interval))
					}
				}
				if want, _ := frameTimes(r.start, predicted, prevTarget, r.period); r.target != want {
					errs = append(errs, fmt.Errorf("frame %d: frame time %v, want %v (start %v, GDK %+v, previous %v, period %v)",
						i, r.target, want, r.start, in, prevTarget, r.period))
				}
			}
			prevTarget = r.target
		}
		if withPrediction == 0 && c.period != defaultPeriod {
			errs = append(errs, fmt.Errorf("period %v without any presentation feedback, want %v", c.period, defaultPeriod))
		}
		if len(recs) > 0 {
			t.Logf("%d frames: %d from ticks (%d with GDK's prediction), %d from the watchdog; GDK's last info %+v; period %v; frame times %v … %v",
				len(recs), c.tickFrames, withPrediction, c.stalls, recs[len(recs)-1].info, c.period, recs[0].target, recs[len(recs)-1].target)
		}
		return errors.Join(errs...)
	})
	checkCallbacksPinned(t)
}

// tickRec is a raw tickPeer: it counts its calls and returns keep, and
// records the handles destroyNotify reports.
type tickRec struct {
	calls int
	keep  bool
	gone  []ffi.Handle
}

func (p *tickRec) tick(kind, ffi.Handle, uintptr) bool { p.calls++; return p.keep }
func (p *tickRec) tickGone(_ kind, h ffi.Handle)       { p.gone = append(p.gone, h) }

// TestClockGTKTickRemoveFrees: GTK calls the tick callback's
// GDestroyNotify (which frees the handle and tells the peer) in each way
// it drops the callback: gtk_widget_remove_tick_callback, the disposal of
// an unrealized widget, the destruction of a window that carries
// callbacks on itself and on its child, and a callback that returns
// G_SOURCE_REMOVE on a mapped widget.
func TestClockGTKTickRemoveFrees(t *testing.T) {
	needGTK(t)
	resetPanics(t)
	onGTK(t, func() error {
		base := dispatch.Len()
		add := func(w uintptr, p *tickRec) (uint32, ffi.Handle, error) {
			id, h := addTick(w, p, kindTestA)
			if id == 0 || h == 0 || !dispatch.Valid(h) {
				return 0, 0, fmt.Errorf("addTick = %d, %v", id, h)
			}
			return id, h, nil
		}
		// gone checks that GTK dropped the callbacks hs (destroyNotify told
		// p and freed their handles) and that extra handles besides the
		// base are live.
		gone := func(what string, p *tickRec, extra int, hs ...ffi.Handle) error {
			for _, h := range hs {
				if dispatch.Valid(h) {
					return fmt.Errorf("%s: the handle %v is still live: GTK did not call the GDestroyNotify", what, h)
				}
				if !slices.Contains(p.gone, h) {
					return fmt.Errorf("%s: tickGone got %v, not %v", what, p.gone, h)
				}
			}
			if got := dispatch.Len(); got != base+extra {
				return fmt.Errorf("%s: dispatch.Len() = %d, want %d", what, got, base+extra)
			}
			return nil
		}

		// gtk_widget_remove_tick_callback on an unrealized widget.
		w := newSurfaceWidget(nil)
		p := &tickRec{keep: true}
		id, h, err := add(w, p)
		if err != nil {
			releaseSurfaceWidget(w)
			return err
		}
		gtk.Gtk.WidgetRemoveTickCallback(w, id)
		if err := gone("gtk_widget_remove_tick_callback", p, 0, h); err != nil {
			releaseSurfaceWidget(w)
			return err
		}
		// The disposal of the widget (its last reference) drops a live
		// callback.
		if _, h, err = add(w, p); err != nil {
			releaseSurfaceWidget(w)
			return err
		}
		releaseSurfaceWidget(w)
		if err := gone("the widget's last unref", p, 0, h); err != nil {
			return err
		}
		if p.calls != 0 {
			return fmt.Errorf("a tick callback on an unrealized widget ran %d times", p.calls)
		}

		// gtk_window_destroy of a window that was never shown, with
		// callbacks on the window and on its child, which the window holds
		// the only reference to.
		if testGtk.WindowDestroy == nil {
			return errors.New("gtk_window_destroy did not bind")
		}
		win := gtk.Gtk.WindowNew()
		child := newSurfaceWidget(nil)
		gtk.Gtk.WindowSetChild(win, child)
		gtk.GObject.ObjectUnref(child) // the window's reference remains
		pw, pc := &tickRec{keep: true}, &tickRec{keep: true}
		_, hw, err := add(win, pw)
		if err != nil {
			testGtk.WindowDestroy(win)
			return err
		}
		_, hc, err := add(child, pc)
		if err != nil {
			testGtk.WindowDestroy(win)
			return err
		}
		testGtk.WindowDestroy(win)
		// The last reference goes inside gtk_window_destroy; iterating
		// first only tolerates a GTK that defers it to the main loop.
		for i := 0; i < 100 && gtk.GLib.MainContextIteration(0, gtk.False) != gtk.False; i++ {
		}
		if err := gone("gtk_window_destroy (the window's callback)", pw, 0, hw); err != nil {
			return err
		}
		if err := gone("gtk_window_destroy (the child's callback)", pc, 0, hc); err != nil {
			return err
		}

		// G_SOURCE_REMOVE from a callback on a mapped widget: it runs
		// once, then GTK drops it.
		fs := &fakeSurface{min: [2]int32{100, 60}, nat: [2]int32{100, 60}}
		win, w, err = presentSurface(fs)
		if err != nil {
			return err
		}
		pr := &tickRec{keep: false}
		_, h, err = add(w, pr)
		if err != nil {
			destroySurface(win, w)
			return err
		}
		ok := iterateUntil(frameDeadline, func() bool { return pr.calls > 0 && !dispatch.Valid(h) })
		for range 10 { // more frames, to see it does not run again
			gtk.GLib.MainContextIteration(0, gtk.False)
		}
		var errRemove error
		switch {
		case !ok:
			errRemove = fmt.Errorf("G_SOURCE_REMOVE: within %v the callback ran %d times, its handle live %v",
				frameDeadline, pr.calls, dispatch.Valid(h))
		case pr.calls != 1:
			errRemove = fmt.Errorf("a callback that returned G_SOURCE_REMOVE ran %d times", pr.calls)
		default:
			errRemove = gone("G_SOURCE_REMOVE", pr, 1, h) // 1: the GGSurface's handle
		}
		destroySurface(win, w)
		if errRemove != nil {
			return errRemove
		}
		if got := dispatch.Len(); got != base {
			return fmt.Errorf("at the end dispatch.Len() = %d, want %d", got, base)
		}
		return nil
	})
	checkCallbacksPinned(t)
}

// TestClockGTKTickWidgetDestroyed: the pacing widget is destroyed while
// the clock's tick callback is live, without detach (the surface stopped
// offering it but did not tell the clock). GTK's GDestroyNotify reaches
// the clock (tickGone): it forgets the callback (tickID 0, handle freed)
// and the pending request falls back to the timeout, so the frame still
// runs.
func TestClockGTKTickWidgetDestroyed(t *testing.T) {
	s, c, l := newGTKClock(t)
	onGTK(t, func() error {
		base := dispatch.Len()
		p := &fakeSurface{min: [2]int32{160, 100}, nat: [2]int32{160, 100}}
		win, w, err := presentSurface(p)
		if err != nil {
			return err
		}
		ps := &gtkPacer{s: s, win: win, w: w}
		s.surfaces = append(s.surfaces, ps)
		destroyed := false
		defer func() {
			c.close()
			if !destroyed {
				ps.closeNow(true)
			}
		}()
		c.begin(0)
		var recs []frameRec
		stop := false
		c.SetFrameFunc(func(time.Duration) {
			record(c, l, &recs)
			if !stop {
				c.RequestFrame()
			}
		})
		c.RequestFrame()
		if !iterateUntil(frameDeadline, func() bool { return c.tickFrames >= 2 }) {
			return fmt.Errorf("%d tick frames within %v (%d from the watchdog)", c.tickFrames, frameDeadline, c.stalls)
		}
		if c.tickID == 0 || !c.requested || len(l.ticks) != 1 {
			return fmt.Errorf("no live tick with a pending request: tickID %d, requested %v, %d added", c.tickID, c.requested, len(l.ticks))
		}
		h := l.ticks[0]
		// The surface stops pacing without telling the clock, and its
		// window goes: gtk_window_destroy unrealizes and unparents the
		// GGSurface, the test's unref disposes it, which drops the
		// callback. No iteration until the checks, so the fallback has
		// not fired yet; the frame function stops requesting, so exactly
		// one frame is left.
		ps.gone = true
		s.surfaces = nil
		stop = true
		testGtk.WindowDestroy(win)
		releaseSurfaceWidget(w)
		destroyed = true
		switch {
		case c.tickID != 0 || c.tickWidget != 0 || c.tickH != 0:
			return fmt.Errorf("after the destroy the clock still has tick %d on %#x (handle %v): tickGone did not reach it",
				c.tickID, c.tickWidget, c.tickH)
		case dispatch.Valid(h):
			return errors.New("the destroyed widget's tick handle is still live")
		case !c.requested:
			return errors.New("the pending request was lost")
		case c.timeoutID == 0 || c.stall:
			return fmt.Errorf("the pending request did not fall back to the timeout (timeout %d, stall %v)", c.timeoutID, c.stall)
		}
		frames := len(recs)
		if !iterateUntil(frameDeadline, func() bool { return len(recs) > frames }) {
			return fmt.Errorf("the pending frame did not run within %v after the pacing widget went", frameDeadline)
		}
		for range 10 {
			gtk.GLib.MainContextIteration(0, gtk.False)
		}
		if r := recs[len(recs)-1]; len(recs) != frames+1 || r.timeouts != 1 || c.timeoutFrames != 1 {
			return fmt.Errorf("after the destroy %d frames ran, the last one with counters %+v; want one from the fallback timeout",
				len(recs)-frames, r)
		}
		if c.requested || c.timeoutID != 0 {
			return fmt.Errorf("after the last frame: requested %v, timeout %d", c.requested, c.timeoutID)
		}
		if c.ticksAdded != 1 {
			return fmt.Errorf("%d tick callbacks added, want 1 (none after the destroy)", c.ticksAdded)
		}
		c.close()
		if got := dispatch.Len(); got != base {
			return fmt.Errorf("after close dispatch.Len() = %d, want %d", got, base)
		}
		return nil
	})
	checkCallbacksPinned(t)
}

// TestClockGTKPanicInFrame: a panic in the frame function, run from the
// fallback timeout inside a real Run, ends Run with a *PanicError from
// the GSourceFunc trampoline and leaves no handle; run from a tick, it is
// trapped in the GtkTickCallback trampoline, GTK drops the callback (it
// returned FALSE) and frees its handle.
func TestClockGTKPanicInFrame(t *testing.T) {
	boom := errors.New("boom in the frame")
	t.Run("timeout", func(t *testing.T) {
		s, reports := newGTKShell(t)
		base := dispatch.Len()
		frames := 0
		p, err := runGTK(t, s, &handler{ready: func() {
			clk := s.Clock()
			clk.SetFrameFunc(func(time.Duration) { frames++; panic(boom) })
			clk.RequestFrame()
		}})
		pe, ok := p.(*PanicError)
		if !ok {
			t.Fatalf("Run = %v, panicked with %T %v; want a *PanicError", err, p, p)
		}
		if !errors.Is(pe, boom) || pe.Callback != nameSourceFunc {
			t.Errorf("PanicError = %v in %q, want the frame's panic in %s", pe.Value, pe.Callback, nameSourceFunc)
		}
		if frames != 1 || s.clock.inFrame || s.clock.timeoutID != 0 {
			t.Errorf("%d frames, inFrame %v, timeout %d left", frames, s.clock.inFrame, s.clock.timeoutID)
		}
		checkGTKTornDown(t, s, reports, base)
	})
	t.Run("tick", func(t *testing.T) {
		s, c, l := newGTKClock(t)
		var pe *PanicError
		onGTK(t, func() error {
			base := dispatch.Len()
			p := &fakeSurface{min: [2]int32{160, 100}, nat: [2]int32{160, 100}}
			win, w, err := presentSurface(p)
			if err != nil {
				return err
			}
			ps := &gtkPacer{s: s, win: win, w: w}
			s.surfaces = append(s.surfaces, ps)
			defer func() {
				c.close()
				ps.closeNow(true)
			}()
			c.begin(0)
			c.SetFrameFunc(func(time.Duration) { panic(boom) })
			c.RequestFrame()
			if len(l.ticks) != 1 {
				return fmt.Errorf("%d tick callbacks added, want 1", len(l.ticks))
			}
			h := l.ticks[0]
			ok := iterateUntil(frameDeadline, func() bool { return pendingPanic != nil })
			pe, pendingPanic = pendingPanic, nil
			if !ok {
				return fmt.Errorf("the frame did not run within %v", frameDeadline)
			}
			// More frames: GTK has dropped the callback after its FALSE.
			for range 10 {
				gtk.GLib.MainContextIteration(0, gtk.False)
			}
			switch {
			case c.tickID != 0 || c.tickH != 0:
				return fmt.Errorf("after the panic the clock still has tick %d", c.tickID)
			case dispatch.Valid(h):
				return errors.New("the panicked tick callback's handle is still live: GTK kept the callback or did not call its GDestroyNotify")
			case c.inFrame || c.frames != 1:
				return fmt.Errorf("inFrame %v after %d frames", c.inFrame, c.frames)
			}
			c.close() // the lazy watchdog may still be armed

			if got := dispatch.Len(); got != base+1 { // the GGSurface's handle
				return fmt.Errorf("dispatch.Len() = %d, want %d", got, base+1)
			}
			return nil
		})
		switch {
		case pe == nil:
		case !errors.Is(pe, boom) || pe.Callback != nameTick:
			t.Errorf("PanicError = %v in %q, want the frame's panic in %s (a watchdog frame would be %s)",
				pe.Value, pe.Callback, nameTick, nameSourceFunc)
		}
		checkCallbacksPinned(t)
	})
}

func checkCallbacksPinned(t *testing.T) {
	t.Helper()
	if got := ffi.Callbacks(); got != numTrampolines {
		t.Errorf("ffi.Callbacks() = %d, want %d", got, numTrampolines)
	}
}
