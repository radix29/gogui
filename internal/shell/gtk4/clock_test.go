package gtk4

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// fakeClockLoop is the loop of the frame clock's host tests: time is set
// by hand (now, in µs), and the timeouts and tick callbacks the clock adds
// are recorded and fired by the test through the real trampolines
// (sourceFunc, tickCallback), with GTK's ownership rules: a timeout's
// handle is freed by sourceFunc or by whoever removes the source; a tick
// callback's by destroyNotify, which the fake calls whenever GTK would
// (the callback returned G_SOURCE_REMOVE or was removed; a removal while
// the callback runs takes effect when it returns). Misuse that GLib or GTK
// would report with a g_critical is recorded in errs.
type fakeClockLoop struct {
	now int64 // g_get_monotonic_time, µs

	nextID   uint32
	timeouts []fakeTimeout // added and not yet fired or removed
	ticks    []*fakeTick   // added and not yet dropped
	idles    []fakeTimeout // idle sources added and not yet run or removed (ms 0)

	info       [3]int64 // what frameInfo returns: frame time, interval, presentation
	frameClock uintptr  // the frameInfo argument of the last call
	// counter is GDK's frame counter, one more at every fireTick; timings
	// are the frames' GdkFrameTimings by counter (none: out of the
	// history).
	counter int64
	timings map[int64]fakeTimings

	failTimeout, failTick, failIdle bool // timeout / addTick / idle add nothing

	// newWin creates the windows of newWindow (surface_test.go); nil:
	// newWindow is misuse.
	newWin func(cfg windowConfig, p windowPeer) (window, error)
	// newPop creates the popovers of newPopover (popup_test.go); nil:
	// newPopover is misuse.
	newPop func(parent window, keys windowPeer, grab bool, p windowPeer) (popover, error)

	timeoutsAdded, removes, ticksAdded, tickRemoves, destroys int
	errs                                                      []string
}

type fakeTimeout struct {
	id uint32
	ms uint32
	at int64 // now when added
	h  ffi.Handle
}

type fakeTick struct {
	widget  uintptr
	id      uint32
	h       ffi.Handle
	running bool // its callback runs
	removed bool // removed while running: dropped on return
}

func (l *fakeClockLoop) errorf(format string, args ...any) {
	l.errs = append(l.errs, fmt.Sprintf(format, args...))
}

func (l *fakeClockLoop) init() error { return nil }
func (l *fakeClockLoop) run()        { l.errorf("run called") }
func (l *fakeClockLoop) quit()       {}
func (l *fakeClockLoop) release()    {}

// newWindow calls l.newWin (the surface tests' window factory); without
// one it is misuse.
func (l *fakeClockLoop) newWindow(cfg windowConfig, p windowPeer) (window, error) {
	if l.newWin != nil {
		return l.newWin(cfg, p)
	}
	l.errorf("newWindow called")
	return nil, errNoFakeWindow
}

// newPopover calls l.newPop; without one it is misuse.
func (l *fakeClockLoop) newPopover(parent window, keys windowPeer, grab bool, p windowPeer) (popover, error) {
	if l.newPop != nil {
		return l.newPop(parent, keys, grab, p)
	}
	l.errorf("newPopover called")
	return nil, errNoFakeWindow
}

func (l *fakeClockLoop) idle(p sourcePeer, k kind) (uint32, ffi.Handle) {
	if l.failIdle {
		return 0, 0
	}
	l.nextID++
	h := dispatch.New(entry{peer: p, kind: k})
	l.idles = append(l.idles, fakeTimeout{id: l.nextID, at: l.now, h: h})
	return l.nextID, h
}

// runIdles runs the pending idle sources through sourceFunc, in order, as
// GLib does once no event is pending, and returns how many ran.
func (l *fakeClockLoop) runIdles(t *testing.T) int {
	t.Helper()
	n := 0
	for len(l.idles) > 0 {
		src := l.idles[0]
		l.idles = l.idles[1:]
		if sourceFunc(uintptr(src.h)) != gtk.SourceRemove {
			t.Error("a one-shot idle source returned G_SOURCE_CONTINUE")
		}
		n++
	}
	return n
}

func (l *fakeClockLoop) remove(id uint32) {
	if i := slices.IndexFunc(l.idles, func(s fakeTimeout) bool { return s.id == id }); i >= 0 {
		l.idles = slices.Delete(l.idles, i, i+1)
		l.removes++
		return
	}
	i := slices.IndexFunc(l.timeouts, func(s fakeTimeout) bool { return s.id == id })
	if i < 0 {
		l.errorf("g_source_remove(%d): source not found", id)
		return
	}
	l.timeouts = slices.Delete(l.timeouts, i, i+1)
	l.removes++
}

func (l *fakeClockLoop) monotonic() int64 { return l.now }

func (l *fakeClockLoop) timeout(ms uint32, p sourcePeer, k kind) (uint32, ffi.Handle) {
	if l.failTimeout {
		return 0, 0
	}
	l.nextID++
	h := dispatch.New(entry{peer: p, kind: k})
	l.timeouts = append(l.timeouts, fakeTimeout{id: l.nextID, ms: ms, at: l.now, h: h})
	l.timeoutsAdded++
	return l.nextID, h
}

func (l *fakeClockLoop) addTick(widget uintptr, p tickPeer, k kind) (uint32, ffi.Handle) {
	if widget == 0 {
		l.errorf("gtk_widget_add_tick_callback on NULL")
		return 0, 0
	}
	if l.failTick {
		return 0, 0
	}
	l.nextID++
	h := dispatch.New(entry{peer: p, kind: k})
	l.ticks = append(l.ticks, &fakeTick{widget: widget, id: l.nextID, h: h})
	l.ticksAdded++
	return l.nextID, h
}

// destroyWidget destroys widget: GTK drops its tick callbacks (destroy
// notify) without the clock being asked.
func (l *fakeClockLoop) destroyWidget(widget uintptr) {
	for {
		i := slices.IndexFunc(l.ticks, func(tk *fakeTick) bool { return tk.widget == widget })
		if i < 0 {
			return
		}
		l.drop(i)
	}
}

func (l *fakeClockLoop) removeTick(widget uintptr, id uint32) {
	i := slices.IndexFunc(l.ticks, func(tk *fakeTick) bool { return tk.id == id && !tk.removed })
	if i < 0 || l.ticks[i].widget != widget {
		l.errorf("gtk_widget_remove_tick_callback(%#x, %d): no such callback", widget, id)
		return
	}
	l.tickRemoves++
	if tk := l.ticks[i]; tk.running {
		tk.removed = true
		return
	}
	l.drop(i)
}

// drop drops tick i and calls its destroy notify, as GTK does.
func (l *fakeClockLoop) drop(i int) {
	h := l.ticks[i].h
	l.ticks = slices.Delete(l.ticks, i, i+1)
	l.destroys++
	destroyNotify(uintptr(h))
}

func (l *fakeClockLoop) frameInfo(frameClock uintptr) (int64, int64, int64) {
	l.frameClock = frameClock
	return l.info[0], l.info[1], l.info[2]
}

// fakeTimings is a frame's GdkFrameTimings: its presentation time and
// refresh interval (µs) and whether they are complete.
type fakeTimings struct {
	presented, interval int64
	complete            bool
}

func (l *fakeClockLoop) frameCounter(uintptr) int64 { return l.counter }

func (l *fakeClockLoop) presentation(_ uintptr, counter int64) (int64, int64, bool, bool) {
	ti, ok := l.timings[counter]
	if !ok {
		return 0, 0, false, true
	}
	if !ti.complete {
		return 0, 0, false, false
	}
	return ti.presented, ti.interval, true, false
}

// fireTimeout runs the one pending timeout through sourceFunc, first
// advancing the time to when it is due (GLib never fires one early). It
// returns the timeout fired.
func (l *fakeClockLoop) fireTimeout(t *testing.T) fakeTimeout {
	t.Helper()
	if len(l.timeouts) != 1 {
		t.Fatalf("%d timeouts pending, want 1", len(l.timeouts))
	}
	src := l.timeouts[0]
	l.timeouts = l.timeouts[1:]
	l.now = max(l.now, src.at+int64(src.ms)*1000)
	if sourceFunc(uintptr(src.h)) != gtk.SourceRemove {
		t.Error("the clock's one-shot timeout returned G_SOURCE_CONTINUE")
	}
	return src
}

// testFrameClock is the GdkFrameClock pointer the fake ticks pass.
const testFrameClock uintptr = 0xfc10c4

// fireTick runs the tick callback on widget through tickCallback with the
// frame clock reporting frame time ft, interval iv and presentation pr
// (µs), and drops it as GTK does if it returns G_SOURCE_REMOVE or was
// removed meanwhile. It returns tickCallback's result.
func (l *fakeClockLoop) fireTick(t *testing.T, widget uintptr, ft, iv, pr int64) int32 {
	t.Helper()
	i := slices.IndexFunc(l.ticks, func(tk *fakeTick) bool { return tk.widget == widget })
	if i < 0 {
		t.Fatalf("no tick callback on %#x", widget)
	}
	tk := l.ticks[i]
	l.counter++
	l.info = [3]int64{ft, iv, pr}
	tk.running = true
	r := tickCallback(widget, testFrameClock, uintptr(tk.h))
	tk.running = false
	if r == gtk.SourceRemove || tk.removed {
		l.drop(slices.Index(l.ticks, tk))
	}
	return r
}

// paceSurface is an openSurface whose widget paces while w is nonzero.
type paceSurface struct {
	s *Shell
	w uintptr
}

func (p *paceSurface) closeNow(bool) {
	if i := slices.Index(p.s.surfaces, openSurface(p)); i >= 0 {
		p.s.surfaces = slices.Delete(p.s.surfaces, i, i+1)
	}
}

func (p *paceSurface) tickWidget() uintptr { return p.w }

func addPacers(s *Shell, ws ...uintptr) []*paceSurface {
	var ps []*paceSurface
	for _, w := range ws {
		p := &paceSurface{s: s, w: w}
		s.surfaces = append(s.surfaces, p)
		ps = append(ps, p)
	}
	return ps
}

const clockStart = 7_000_000 // monotonic at begin, µs

// newClockTest returns a shell over a fakeClockLoop, its UI thread bound
// to the test goroutine with the checks on, and Ready passed (so Clock
// works); the clock has not begun. At the end of the test the clock is
// closed and nothing may be left: no timeout or tick callback in the loop,
// no handle in the dispatch table, no misuse of the loop.
func newClockTest(t *testing.T) (*Shell, *clock, *fakeClockLoop) {
	t.Helper()
	resetPanics(t)
	base := dispatch.Len()
	l := &fakeClockLoop{now: clockStart}
	s := newShell(l)
	s.Queue().SetChecks(true)
	s.Queue().Bind()
	s.ready = true
	t.Cleanup(func() {
		s.clock.close()
		if len(l.timeouts) != 0 || len(l.ticks) != 0 || len(l.idles) != 0 {
			t.Errorf("after close: %d timeouts, %d tick callbacks and %d idle sources left", len(l.timeouts), len(l.ticks), len(l.idles))
		}
		for _, e := range l.errs {
			t.Errorf("fake loop: %s", e)
		}
		checkLen(t, base)
	})
	return s, s.clock, l
}

// recordFrames sets c's frame function and paint pass to log their calls
// with the target time, and returns the log.
func recordFrames(c *clock) *[]string {
	var log []string
	c.SetFrameFunc(func(ft time.Duration) { log = append(log, fmt.Sprintf("fn %v", ft)) })
	c.paint = func(ft time.Duration) { log = append(log, fmt.Sprintf("paint %v", ft)) }
	return &log
}

func frameLog(targets ...time.Duration) []string {
	var log []string
	for _, ft := range targets {
		log = append(log, fmt.Sprintf("fn %v", ft), fmt.Sprintf("paint %v", ft))
	}
	return log
}

func checkLog(t *testing.T, log *[]string, targets ...time.Duration) {
	t.Helper()
	if want := frameLog(targets...); !slices.Equal(*log, want) {
		t.Errorf("frames %v, want %v", *log, want)
	}
	*log = nil
}

const ms = time.Millisecond

// fireStale fires the one pending timeout, a watchdog left behind after
// the clock went idle, and checks that it runs no frame and adds nothing.
func fireStale(t *testing.T, c *clock, l *fakeClockLoop) {
	t.Helper()
	frames, added := c.frames, l.timeoutsAdded
	l.fireTimeout(t)
	if c.frames != frames || l.timeoutsAdded != added || c.timeoutID != 0 {
		t.Errorf("a stale watchdog ran %d frames and added %d timeouts", c.frames-frames, l.timeoutsAdded-added)
	}
}

func TestGridAfter(t *testing.T) {
	const p = 10 * ms
	for _, c := range []struct{ t, anchor, want time.Duration }{
		{0, 0, p}, {1, 0, p}, {p - 1, 0, p}, {p, 0, 2 * p},
		{-1, 0, 0}, {-p, 0, 0}, {-p - 1, 0, -p}, {-2*p - 1, 0, -2 * p},
		{3 * ms, 5 * ms, 5 * ms}, {5 * ms, 5 * ms, 15 * ms}, {205 * ms, 5 * ms, 215 * ms},
		{-3 * ms, 5 * ms, 5 * ms}, {-5 * ms, 5 * ms, 5 * ms}, {-6 * ms, 5 * ms, -5 * ms},
	} {
		if got := gridAfter(c.t, c.anchor, p); got != c.want {
			t.Errorf("gridAfter(%v, %v, %v) = %v, want %v", c.t, c.anchor, p, got, c.want)
		}
	}
}

func TestClockFrameTimes(t *testing.T) {
	const p = 10 * ms
	dp := defaultPeriod
	us := time.Microsecond
	for _, c := range []struct {
		name                       string
		t, predicted, prev, period time.Duration
		target, hold               time.Duration
	}{
		{"first frame: grid slot after t from 0", 2 * ms, -1, -1, p, p, p},
		{"first frame just before a slot: held half a period", 9 * ms, -1, -1, p, p, 14 * ms},
		{"first frame on a slot: the next one", p, -1, -1, p, 2 * p, 2 * p},
		{"predicted after t is used, off the grid", 5 * ms, 23 * ms, -1, p, 23 * ms, 23 * ms},
		{"predicted after t, hold half a period after t", 5 * ms, 7 * ms, -1, p, 7 * ms, 10 * ms},
		{"predicted == t: t + period", 5 * ms, 5 * ms, -1, p, 15 * ms, 15 * ms},
		{"predicted before t: whole periods past t", 5 * ms, 3 * ms, -1, p, 13 * ms, 13 * ms},
		{"predicted far before t", 100 * ms, 3 * ms, -1, p, 103 * ms, 105 * ms},
		{"predicted periods before t, on t: the next", 23 * ms, 3 * ms, -1, p, 33 * ms, 33 * ms},
		{"predicted 0 is a prediction", 5 * ms, 0, -1, p, p, p},
		{"next slot after prev", 12 * ms, -1, p, p, 2 * p, 2 * p},
		{"grid anchored at prev off the 0 grid", 40 * ms, -1, 26 * ms, p, 46 * ms, 46 * ms},
		{"frames skipped: still on prev's grid", 45 * ms, -1, 2 * p, p, 5 * p, 5 * p},
		{"late for target prev: prev + period, not prev again", 19 * ms, -1, 2 * p, p, 3 * p, 3 * p},
		{"exactly on prev: the next", 20 * ms, -1, 2 * p, p, 3 * p, 3 * p},
		{"predicted within half a period of prev: prev + period", 11 * ms, 12 * ms, p, p, 2 * p, 2 * p},
		{"predicted half a period after prev is kept", 11 * ms, 15 * ms, p, p, 15 * ms, 16 * ms},
		{"predicted before prev: prev + period", 11 * ms, 12 * ms, 14 * ms, p, 24 * ms, 24 * ms},
		{"cap: a prediction more than 2 periods ahead", 5 * ms, 100 * ms, -1, p, 25 * ms, 25 * ms},
		{"cap: prev + period more than 2 periods ahead", 5 * ms, -1, 20 * ms, p, 25 * ms, 25 * ms},
		{"cap exactly at t + 2 periods", 5 * ms, 25 * ms, -1, p, 25 * ms, 25 * ms},
		{"backward t: prev + 1µs", 5 * ms, -1, 30 * ms, p, 30*ms + us, 30*ms + us},
		{"backward t with a prediction: prev + 1µs", 5 * ms, 12 * ms, 40 * ms, p, 40*ms + us, 40*ms + us},
		{"period 0: defaultPeriod", 2 * ms, -1, -1, 0, dp, dp},
		{"negative period: defaultPeriod", 2 * ms, -1, -1, -p, dp, dp},
		{"period 0 with prev", dp + ms, -1, dp, 0, 2 * dp, 2 * dp},
		{"negative t: floor to slot 0", -1 * ms, -1, -1, p, 0, 4 * ms},
		{"negative t on a slot", -p, -1, -1, p, 0, 0},
		{"negative t below a slot", -p - ms, -1, -1, p, -p, -6 * ms},
	} {
		target, hold := frameTimes(c.t, c.predicted, c.prev, c.period)
		if target != c.target || hold != c.hold {
			t.Errorf("%s: frameTimes(%v, %v, %v, %v) = %v, %v; want %v, %v",
				c.name, c.t, c.predicted, c.prev, c.period, target, hold, c.target, c.hold)
		}
	}

	// Invariants over random inputs: the target is after t, after prev,
	// at least half a period after prev unless that is beyond the cap, and
	// not beyond t + 2 periods unless it must pass prev; the hold is at or
	// after the target and at least half a period after t; a prediction
	// after t that the rules allow is used as it is.
	r := rand.New(rand.NewPCG(1, 2))
	for range 50000 {
		period := time.Duration(1+r.IntN(40)) * ms / 2
		tt := time.Duration(r.Int64N(int64(2*time.Second))) - 100*ms
		prev := time.Duration(-1)
		if r.IntN(4) != 0 {
			prev = max(tt+time.Duration(r.Int64N(int64(5*period)))-2*period, 0)
		}
		predicted := time.Duration(-1)
		if r.IntN(2) == 0 {
			predicted = max(tt+time.Duration(r.Int64N(int64(4*period)))-period, 0)
		}
		target, hold := frameTimes(tt, predicted, prev, period)
		bad := target <= tt || hold < target || hold < tt+period/2 ||
			(target > tt+2*period && (prev < 0 || target != prev+time.Microsecond))
		if prev >= 0 {
			bad = bad || target <= prev || (target < prev+period/2 && prev+period/2 <= tt+2*period)
		}
		if bad {
			t.Fatalf("frameTimes(%v, %v, %v, %v) = %v, %v", tt, predicted, prev, period, target, hold)
		}
		if predicted >= 0 && predicted > tt && predicted <= tt+2*period && (prev < 0 || predicted >= prev+period/2) && target != predicted {
			t.Fatalf("frameTimes(%v, %v, %v, %v) = %v: prediction not used", tt, predicted, prev, period, target)
		}
	}
}

// TestClockNow: Now is 0 at begin (back-dated by its argument) and follows
// the monotonic time in µs; the clock's methods panic off the UI thread.
func TestClockNow(t *testing.T) {
	s, c, l := newClockTest(t)
	if s.Clock() != c {
		t.Fatal("Clock() is not the shell's clock")
	}
	c.begin(0)
	if got := c.Now(); got != 0 {
		t.Errorf("Now at begin = %v, want 0", got)
	}
	l.now += 1234
	if got := c.Now(); got != 1234*time.Microsecond {
		t.Errorf("Now = %v, want 1.234ms", got)
	}
	l.now += 3_000_000
	if got := c.Now(); got != 3001234*time.Microsecond {
		t.Errorf("Now = %v, want 3.001234s", got)
	}
	offThread := func(v any) bool {
		msg, _ := v.(string)
		return strings.Contains(msg, "off the UI thread")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		mustPanicWith(t, "Now off the UI thread", offThread, func() { c.Now() })
		mustPanicWith(t, "SetFrameFunc off the UI thread", offThread, func() { c.SetFrameFunc(nil) })
		mustPanicWith(t, "RequestFrame off the UI thread", offThread, func() { c.RequestFrame() })
	}()
	<-done
	if c.requested || len(l.timeouts) != 0 {
		t.Error("RequestFrame off the UI thread requested a frame")
	}

	for _, b := range []struct{ since, want time.Duration }{
		{5 * ms, 5 * ms}, {1500 * time.Microsecond, 1500 * time.Microsecond}, {-ms, 0},
	} {
		_, c, _ := newClockTest(t)
		c.begin(b.since)
		if got := c.Now(); got != b.want {
			t.Errorf("Now after begin(%v) = %v, want %v", b.since, got, b.want)
		}
	}
}

// TestClockFallback: without a pacing surface, a request is one timeout,
// due at the hold, and its frame runs the frame function then the paint
// pass on the 60 Hz grid; several requests are one frame, and a request
// from inside the frame is scheduled after it.
func TestClockFallback(t *testing.T) {
	_, c, l := newClockTest(t)
	log := recordFrames(c)
	p := defaultPeriod

	c.RequestFrame() // before begin: waits for it
	if len(l.timeouts) != 0 {
		t.Fatal("a request before begin added a timeout")
	}
	c.begin(0)
	if len(l.timeouts) != 1 || l.timeouts[0].ms != 0 || c.stall {
		t.Fatalf("begin with a request: timeouts %+v (stall %v), want one of 0 ms", l.timeouts, c.stall)
	}
	c.RequestFrame()
	c.RequestFrame()
	if l.timeoutsAdded != 1 {
		t.Errorf("coalesced requests added %d timeouts, want 1", l.timeoutsAdded)
	}

	l.now += 2500
	l.fireTimeout(t)
	checkLog(t, log, p)
	if c.frames != 1 || c.timeoutFrames != 1 || c.tickFrames != 0 || c.stalls != 0 {
		t.Errorf("frames %d, timeout %d, tick %d, stalls %d; want 1, 1, 0, 0", c.frames, c.timeoutFrames, c.tickFrames, c.stalls)
	}
	if c.timeoutID != 0 || c.timeoutH != 0 || len(l.timeouts) != 0 {
		t.Errorf("after the frame: timeout %d/%d, %d pending", c.timeoutID, c.timeoutH, len(l.timeouts))
	}

	// The next frame waits for the hold, the target 16.67 ms: 13.67 ms
	// after now, rounded up.
	l.now = clockStart + 3000
	c.RequestFrame()
	if len(l.timeouts) != 1 || l.timeouts[0].ms != 14 {
		t.Fatalf("timeouts %+v, want one of 14 ms", l.timeouts)
	}
	hold := c.hold
	l.fireTimeout(t) // at 17 ms
	if start := c.last; start < hold {
		t.Errorf("frame started at %v, before the hold %v", start, hold)
	}
	checkLog(t, log, 2*p)

	// A request from inside the frame: no timeout and no frame inside it,
	// one timeout after it, at the next hold.
	depth := 0
	c.SetFrameFunc(func(ft time.Duration) {
		depth++
		defer func() { depth-- }()
		if depth > 1 {
			t.Error("nested frame")
		}
		*log = append(*log, fmt.Sprintf("fn %v", ft))
		c.RequestFrame()
		c.RequestFrame()
		if len(l.timeouts) != 0 {
			t.Error("a request inside the frame added a timeout")
		}
	})
	c.RequestFrame()
	if l.timeouts[0].ms != 17 { // hold 33.33 ms - now 17 ms
		t.Errorf("timeout of %d ms, want 17", l.timeouts[0].ms)
	}
	l.fireTimeout(t) // 34 ms
	checkLog(t, log, 3*p)
	if len(l.timeouts) != 1 || !c.requested {
		t.Fatalf("after a frame that requested one: %d timeouts, requested %v", len(l.timeouts), c.requested)
	}
	// hold = max(50 ms, 34 ms + 8.33 ms) = 50 ms (49.999998 ms): 16 ms.
	if got := l.timeouts[0].ms; got != 16 {
		t.Errorf("timeout of %d ms after the frame, want 16", got)
	}
	l.fireTimeout(t) // 50 ms
	checkLog(t, log, 4*p)
	c.SetFrameFunc(nil)
	l.fireTimeout(t)
	if c.frames != 5 || c.timeoutFrames != 5 || len(l.timeouts) != 0 {
		t.Errorf("frames %d, timeout frames %d, %d timeouts; want 5, 5, 0", c.frames, c.timeoutFrames, len(l.timeouts))
	}
	if want := []string{fmt.Sprintf("paint %v", 5*p)}; !slices.Equal(*log, want) {
		t.Errorf("frame without a frame function: %v, want %v", *log, want)
	}
}

// TestClockFallbackLate: a timeout that fires late runs the frame at once,
// on the next grid slot, and a request made long after the last frame is
// due at once.
func TestClockFallbackLate(t *testing.T) {
	_, c, l := newClockTest(t)
	log := recordFrames(c)
	p := defaultPeriod
	c.begin(0)
	c.RequestFrame()
	l.now += 40_000 // 40 ms late
	l.fireTimeout(t)
	checkLog(t, log, 3*p)
	l.now += 1_000_000
	c.RequestFrame()
	if l.timeouts[0].ms != 0 {
		t.Errorf("a request long after the hold waits %d ms, want 0", l.timeouts[0].ms)
	}
	l.fireTimeout(t)
	checkLog(t, log, gridAfter(c.last, 3*p, p))
	if c.target%p != 0 || c.target <= c.last {
		t.Errorf("target %v for a frame at %v", c.target, c.last)
	}
}

// TestClockTick: with a pacing surface a request is a tick callback on its
// widget plus the stall watchdog; the tick runs the frame with GDK's
// prediction and refresh interval and removes itself when no frame is
// requested any more; the watchdog is armed once for a run of ticks and
// runs no frame when it fires idle.
func TestClockTick(t *testing.T) {
	s, c, l := newClockTest(t)
	base := dispatch.Len()
	log := recordFrames(c)
	addPacers(s, 0x100)
	c.begin(0)
	if len(l.ticks) != 0 || len(l.timeouts) != 0 {
		t.Fatal("begin without a request added a tick or a timeout")
	}
	c.RequestFrame()
	c.RequestFrame()
	if len(l.ticks) != 1 || l.ticks[0].widget != 0x100 || c.tickWidget != 0x100 || c.tickID != l.ticks[0].id || c.tickH != l.ticks[0].h {
		t.Fatalf("ticks %+v, clock tick %#x/%d/%d; want one on 0x100", l.ticks, c.tickWidget, c.tickID, c.tickH)
	}
	if len(l.timeouts) != 1 || l.timeouts[0].ms != uint32(stallTimeout/ms) || !c.stall {
		t.Fatalf("timeouts %+v (stall %v), want the 100 ms watchdog", l.timeouts, c.stall)
	}
	if c.ticksAdded != 1 || l.ticksAdded != 1 || l.timeoutsAdded != 1 {
		t.Errorf("%d ticks and %d timeouts added, want 1 and 1", l.ticksAdded, l.timeoutsAdded)
	}
	checkLen(t, base+2)

	// GDK's frame: started 4 ms in, 120 Hz, presented at 12 ms.
	l.now += 5000
	if r := l.fireTick(t, 0x100, clockStart+4000, 8333, clockStart+12000); r != gtk.SourceRemove {
		t.Error("an idle tick returned G_SOURCE_CONTINUE")
	}
	checkLog(t, log, 12*ms)
	if l.frameClock != testFrameClock {
		t.Errorf("frameInfo(%#x), want the tick's frame clock", l.frameClock)
	}
	if c.period != 8333*time.Microsecond {
		t.Errorf("period %v, want GDK's 8.333ms", c.period)
	}
	if c.last != 4*ms || c.lastTick != 5*ms {
		t.Errorf("frame start %v, last tick %v; want GDK's frame time 4ms and Now 5ms", c.last, c.lastTick)
	}
	if c.tickFrames != 1 || c.frames != 1 || c.stalls != 0 || c.timeoutFrames != 0 {
		t.Errorf("tick frames %d, frames %d, stalls %d, timeout frames %d", c.tickFrames, c.frames, c.stalls, c.timeoutFrames)
	}
	if len(l.ticks) != 0 || c.tickID != 0 || c.tickWidget != 0 || c.tickH != 0 {
		t.Errorf("idle tick kept: %d ticks, clock %#x/%d/%d", len(l.ticks), c.tickWidget, c.tickID, c.tickH)
	}
	// The watchdog stays until it fires, idle.
	if len(l.timeouts) != 1 || l.removes != 0 {
		t.Errorf("%d timeouts, %d removes; want the watchdog left", len(l.timeouts), l.removes)
	}
	fireStale(t, c, l)
	checkLen(t, base)

	// Continued requests: the tick stays, and the one watchdog with it.
	n := 0
	c.SetFrameFunc(func(ft time.Duration) {
		*log = append(*log, fmt.Sprintf("fn %v", ft))
		if n++; n < 3 {
			c.RequestFrame()
		}
	})
	c.RequestFrame()
	if l.ticksAdded != 2 || len(l.ticks) != 1 || len(l.timeouts) != 1 || l.timeoutsAdded != 2 {
		t.Fatalf("request after idle: %d ticks added, %d live, %d timeouts (%d added)", l.ticksAdded, len(l.ticks), len(l.timeouts), l.timeoutsAdded)
	}
	for i, want := range []int32{gtk.SourceContinue, gtk.SourceContinue, gtk.SourceRemove} {
		l.now += 8333
		ft := l.now - 1000
		if r := l.fireTick(t, 0x100, ft, 8333, ft+8333); r != want {
			t.Errorf("tick %d returned %d, want %d", i, r, want)
		}
		if len(l.timeouts) != 1 || l.timeoutsAdded != 2 {
			t.Errorf("tick %d: %d timeouts, %d added; want the one watchdog", i, len(l.timeouts), l.timeoutsAdded)
		}
		if want == gtk.SourceContinue && len(l.ticks) != 1 {
			t.Errorf("tick %d: %d ticks, want the tick kept", i, len(l.ticks))
		}
	}
	if len(*log) != 6 || c.tickFrames != 4 || l.ticksAdded != 2 {
		t.Errorf("log %v, %d tick frames, %d ticks added", *log, c.tickFrames, l.ticksAdded)
	}
	if len(l.ticks) != 0 {
		t.Errorf("idle clock left %d ticks", len(l.ticks))
	}
	fireStale(t, c, l)
	checkLen(t, base)
}

// TestClockTickNoPrediction: a tick without a predicted presentation time
// targets the grid after GDK's frame time, and keeps the period whatever
// interval GDK reports (its default without presentation feedback); a
// prediction before the frame time moves on by whole periods.
func TestClockTickNoPrediction(t *testing.T) {
	s, c, l := newClockTest(t)
	log := recordFrames(c)
	addPacers(s, 0x100)
	c.begin(0)
	c.RequestFrame()
	l.now += 21000
	l.fireTick(t, 0x100, clockStart+20000, 6944, 0)
	checkLog(t, log, 2*defaultPeriod)
	if c.period != defaultPeriod {
		t.Errorf("period %v after an interval without presentation time, want %v", c.period, defaultPeriod)
	}
	c.RequestFrame()
	l.now += 10000
	l.fireTick(t, 0x100, clockStart+30000, 0, clockStart+25000)
	checkLog(t, log, 25*ms+defaultPeriod)
	if c.period != defaultPeriod {
		t.Errorf("period %v after interval 0, want %v", c.period, defaultPeriod)
	}
}

// TestClockTickCap: GDK reports its default 16.67 ms without presentation
// feedback while the display runs at 144 Hz: the targets do not run ahead
// by a period per frame but stay within two periods of each frame's start
// and step by the real frame interval.
func TestClockTickCap(t *testing.T) {
	s, c, l := newClockTest(t)
	addPacers(s, 0x100)
	c.begin(0)
	var targets, starts []time.Duration
	c.SetFrameFunc(func(ft time.Duration) {
		targets = append(targets, ft)
		starts = append(starts, c.last)
		c.RequestFrame()
	})
	c.RequestFrame()
	const iv = 6944 // µs, 144 Hz
	for i := range 40 {
		l.now = clockStart + int64(i+1)*iv
		if r := l.fireTick(t, 0x100, l.now, 16667, 0); r != gtk.SourceContinue {
			t.Fatalf("tick %d returned %d", i, r)
		}
	}
	p := defaultPeriod
	for i, tg := range targets {
		if tg <= starts[i] || tg > starts[i]+2*p {
			t.Errorf("frame %d at %v targets %v, beyond two periods", i, starts[i], tg)
		}
		if i > 0 && tg <= targets[i-1] {
			t.Errorf("targets %d and %d not increasing: %v, %v", i-1, i, targets[i-1], tg)
		}
	}
	for i := 30; i < len(targets); i++ {
		if d := targets[i] - targets[i-1]; d != iv*time.Microsecond {
			t.Errorf("steady targets %d and %d %v apart, want the 144 Hz interval", i-1, i, d)
		}
	}
	c.SetFrameFunc(nil)
	l.now += iv
	l.fireTick(t, 0x100, l.now, 16667, 0)
	fireStale(t, c, l)
}

// TestClockStall: the watchdog runs a frame whose tick does not come; the
// tick stays, and when it comes without a request it removes itself. A
// request from the watchdog's frame arms a new watchdog, the same tick.
func TestClockStall(t *testing.T) {
	s, c, l := newClockTest(t)
	base := dispatch.Len()
	log := recordFrames(c)
	addPacers(s, 0x100)
	c.begin(0)
	c.RequestFrame()
	l.fireTimeout(t) // the watchdog, 100 ms later
	if c.stalls != 1 || c.timeoutFrames != 0 || c.tickFrames != 0 || c.frames != 1 {
		t.Errorf("stalls %d, timeout frames %d, tick frames %d, frames %d; want 1, 0, 0, 1", c.stalls, c.timeoutFrames, c.tickFrames, c.frames)
	}
	if c.Now() != stallTimeout {
		t.Errorf("watchdog fired at %v", c.Now())
	}
	checkLog(t, log, 7*defaultPeriod) // the slot after 100 ms
	if len(l.ticks) != 1 || c.tickID == 0 || len(l.timeouts) != 0 || c.stall {
		t.Errorf("after the watchdog: %d ticks (clock %d), %d timeouts, stall %v; want the tick kept and nothing else", len(l.ticks), c.tickID, len(l.timeouts), c.stall)
	}
	l.now += 3000
	frames := c.frames
	if r := l.fireTick(t, 0x100, l.now, 0, 0); r != gtk.SourceRemove || c.frames != frames {
		t.Errorf("idle tick returned %d and ran %d frames", r, c.frames-frames)
	}
	if len(l.ticks) != 0 || c.tickID != 0 || len(l.timeouts) != 0 {
		t.Error("idle tick not removed")
	}
	checkLen(t, base)

	c.SetFrameFunc(func(time.Duration) { c.RequestFrame() })
	c.RequestFrame()
	l.fireTimeout(t)
	if c.stalls != 2 || len(l.timeouts) != 1 || !c.stall || len(l.ticks) != 1 || l.ticksAdded != 2 {
		t.Errorf("after a requesting watchdog frame: stalls %d, %d timeouts (stall %v), %d ticks, %d added", c.stalls, len(l.timeouts), c.stall, len(l.ticks), l.ticksAdded)
	}
	c.SetFrameFunc(nil)
	l.now += 1000
	if r := l.fireTick(t, 0x100, l.now, 0, 0); r != gtk.SourceRemove {
		t.Error("tick kept")
	}
	if c.tickFrames != 1 || c.frames != 3 {
		t.Errorf("tick frames %d, frames %d; want 1, 3", c.tickFrames, c.frames)
	}
	fireStale(t, c, l)
	checkLen(t, base)
}

// TestClockWatchdogLazy: while ticks keep coming the watchdog runs no
// frame: it re-arms for the rest of stallTimeout after the last tick; when
// the ticks stop it runs the frame stallTimeout after the last one.
func TestClockWatchdogLazy(t *testing.T) {
	s, c, l := newClockTest(t)
	base := dispatch.Len()
	addPacers(s, 0x100)
	c.begin(0)
	c.SetFrameFunc(func(time.Duration) { c.RequestFrame() })
	c.RequestFrame()
	for _, at := range []int64{10_000, 26_000, 42_000, 58_000, 74_000, 90_000} {
		l.now = clockStart + at
		l.fireTick(t, 0x100, l.now, 0, 0)
	}
	if l.timeoutsAdded != 1 || len(l.timeouts) != 1 {
		t.Fatalf("%d timeouts added for 6 ticks, want 1", l.timeoutsAdded)
	}
	l.fireTimeout(t) // due at 100 ms; the last tick at 90 ms
	if c.stalls != 0 || c.frames != 6 {
		t.Errorf("the watchdog ran a frame 10 ms after a tick: stalls %d, frames %d", c.stalls, c.frames)
	}
	if len(l.timeouts) != 1 || !c.stall || l.timeouts[0].ms != 90 {
		t.Fatalf("watchdog re-armed as %+v (stall %v), want 90 ms", l.timeouts, c.stall)
	}
	// One more tick inside: the re-armed watchdog re-arms again.
	l.now = clockStart + 150_000
	l.fireTick(t, 0x100, l.now, 0, 0)
	l.fireTimeout(t) // at 190 ms
	if c.stalls != 0 || len(l.timeouts) != 1 || l.timeouts[0].ms != 60 {
		t.Fatalf("stalls %d, timeouts %+v; want a re-arm of 60 ms", c.stalls, l.timeouts)
	}
	l.fireTimeout(t) // at 250 ms, 100 ms after the last tick: the frame
	if c.stalls != 1 || c.frames != 8 || c.Now() != 250*ms {
		t.Errorf("stalls %d, frames %d at %v; want the watchdog's frame at 250ms", c.stalls, c.frames, c.Now())
	}
	if len(l.timeouts) != 1 || !c.stall || len(l.ticks) != 1 {
		t.Errorf("after the watchdog's frame: %d timeouts (stall %v), %d ticks", len(l.timeouts), c.stall, len(l.ticks))
	}
	c.SetFrameFunc(nil)
	l.now += 5000
	l.fireTick(t, 0x100, l.now, 0, 0)
	fireStale(t, c, l)
	checkLen(t, base)
}

// TestClockWatchdogAfterIdle: a watchdog left from an earlier run of ticks
// must not run a frame before stallTimeout has passed since a new request
// whose tick is still awaited.
func TestClockWatchdogAfterIdle(t *testing.T) {
	s, c, l := newClockTest(t)
	addPacers(s, 0x100)
	c.begin(0)
	c.RequestFrame() // watchdog due at 100 ms
	l.now = clockStart + 5000
	l.fireTick(t, 0x100, l.now, 0, 0) // frame, idle: the tick goes
	l.now = clockStart + 95_000
	c.RequestFrame() // a new tick; GDK's next frame is ~16 ms away
	for len(l.timeouts) == 1 {
		src := l.timeouts[0]
		if src.at+int64(src.ms)*1000 >= clockStart+95_000+int64(stallTimeout/time.Microsecond) {
			break
		}
		l.fireTimeout(t)
		if c.stalls != 0 {
			t.Fatalf("the watchdog ran a frame at %v, %v after the request", c.Now(), c.Now()-95*ms)
		}
	}
	l.fireTick(t, 0x100, l.now, 0, 0)
	fireStale(t, c, l)
}

// TestClockPacerChange: a request follows the pacing surface: a new pacer
// moves the tick (keeping the watchdog), none moves the request to the
// fallback timeout, and a pacer again moves it back.
func TestClockPacerChange(t *testing.T) {
	s, c, l := newClockTest(t)
	base := dispatch.Len()
	log := recordFrames(c)
	ps := addPacers(s, 0x100, 0x200)
	c.begin(0)
	c.RequestFrame()
	watchdog := c.timeoutID
	ps[0].w = 0 // hidden
	c.surfacesChanged()
	if l.tickRemoves != 1 || l.destroys != 1 || len(l.ticks) != 1 || l.ticks[0].widget != 0x200 || c.tickWidget != 0x200 || c.tickH != l.ticks[0].h {
		t.Fatalf("pacer change: %d removes, %d destroys, ticks %+v, clock on %#x", l.tickRemoves, l.destroys, l.ticks, c.tickWidget)
	}
	if len(l.timeouts) != 1 || c.timeoutID != watchdog || !c.stall {
		t.Errorf("watchdog replaced: %+v", l.timeouts)
	}
	checkLen(t, base+2)

	// No pacer: the fallback timeout.
	ps[1].w = 0
	c.surfacesChanged()
	if len(l.ticks) != 0 || c.tickID != 0 || len(l.timeouts) != 1 || c.stall || c.timeoutID == watchdog || l.timeouts[0].ms != 0 {
		t.Fatalf("no pacer: %d ticks, timeouts %+v (stall %v)", len(l.ticks), l.timeouts, c.stall)
	}
	checkLen(t, base+1)

	// A pacer again: the fallback goes, a tick and a watchdog come.
	ps[0].w = 0x300
	c.surfacesChanged()
	if len(l.ticks) != 1 || l.ticks[0].widget != 0x300 || len(l.timeouts) != 1 || !c.stall || l.timeouts[0].ms != 100 {
		t.Fatalf("pacer back: ticks %+v, timeouts %+v (stall %v)", l.ticks, l.timeouts, c.stall)
	}
	checkLen(t, base+2)
	l.fireTick(t, 0x300, l.now, 0, 0)
	checkLog(t, log, defaultPeriod)
	if len(l.ticks) != 0 {
		t.Error("left a tick")
	}
	fireStale(t, c, l)

	// Without a request a pacer change adds nothing.
	ps[0].w = 0x400
	c.surfacesChanged()
	if len(l.ticks) != 0 || len(l.timeouts) != 0 {
		t.Error("surfacesChanged without a request scheduled something")
	}
	checkLen(t, base)
}

// TestClockAddTickFails: if GTK adds no tick callback the request falls
// back to the timeout; if GLib adds no timeout the clock holds none.
func TestClockAddTickFails(t *testing.T) {
	s, c, l := newClockTest(t)
	base := dispatch.Len()
	log := recordFrames(c)
	addPacers(s, 0x100)
	l.failTick = true
	c.begin(0)
	c.RequestFrame()
	if c.tickID != 0 || c.tickH != 0 || len(l.timeouts) != 1 || c.stall || l.timeouts[0].ms != 0 {
		t.Fatalf("addTick failed: tick %d, timeouts %+v (stall %v); want the fallback", c.tickID, l.timeouts, c.stall)
	}
	l.fireTimeout(t)
	checkLog(t, log, defaultPeriod)

	l.failTimeout = true
	c.RequestFrame()
	if c.timeoutID != 0 || c.timeoutH != 0 || c.stall {
		t.Errorf("timeout failed: clock holds %d/%d (stall %v)", c.timeoutID, c.timeoutH, c.stall)
	}
	l.failTick, l.failTimeout = false, false
	c.surfacesChanged() // the next schedule succeeds
	if c.tickID == 0 || len(l.timeouts) != 1 || !c.stall {
		t.Errorf("reschedule: tick %d, %d timeouts (stall %v)", c.tickID, len(l.timeouts), c.stall)
	}
	l.fireTick(t, 0x100, l.now, 0, 0)
	checkLog(t, log, 2*defaultPeriod)
	fireStale(t, c, l)
	checkLen(t, base)
}

// TestClockDetach: detach removes the tick callback of the widget about
// to be destroyed and moves a pending request on; detach of another widget
// keeps it.
func TestClockDetach(t *testing.T) {
	s, c, l := newClockTest(t)
	base := dispatch.Len()
	log := recordFrames(c)
	ps := addPacers(s, 0x100, 0x200)
	c.begin(0)
	c.RequestFrame()
	c.detach(0x200) // not the tick's
	c.detach(0)
	if l.tickRemoves != 0 || c.tickWidget != 0x100 {
		t.Fatalf("detach of another widget removed the tick (%d removes)", l.tickRemoves)
	}
	ps[0].closeNow(true) // the surface goes, then detaches its widget
	c.detach(0x100)
	if l.tickRemoves != 1 || len(l.ticks) != 1 || l.ticks[0].widget != 0x200 || len(l.timeouts) != 1 {
		t.Fatalf("detach: %d removes, ticks %+v, %d timeouts", l.tickRemoves, l.ticks, len(l.timeouts))
	}
	ps[1].closeNow(true)
	c.detach(0x200)
	if len(l.ticks) != 0 || len(l.timeouts) != 1 || c.stall {
		t.Fatalf("detach of the last pacer: %d ticks, %d timeouts (stall %v); want the fallback", len(l.ticks), len(l.timeouts), c.stall)
	}
	l.fireTimeout(t)
	checkLog(t, log, defaultPeriod)

	// Without a request: the tick goes and nothing comes.
	addPacers(s, 0x500)
	c.RequestFrame()
	c.fn = func(time.Duration) {}
	l.fireTimeout(t) // the watchdog; the tick stays
	c.detach(0x500)
	if len(l.ticks) != 0 || c.tickID != 0 || len(l.timeouts) != 0 {
		t.Errorf("idle detach: %d ticks, %d timeouts", len(l.ticks), len(l.timeouts))
	}
	checkLen(t, base)
}

// TestClockTickGone: GTK drops the clock's tick callback without the
// clock asking (its widget destroyed without detach): the clock forgets
// it, never calls GTK on that widget again, and moves the request to the
// next pacer. A destroy notify of another handle changes nothing.
func TestClockTickGone(t *testing.T) {
	s, c, l := newClockTest(t)
	base := dispatch.Len()
	log := recordFrames(c)
	ps := addPacers(s, 0x100, 0x200)
	c.begin(0)
	c.RequestFrame()
	tk := *l.ticks[0]

	c.tickGone(kindTick, 0)
	c.tickGone(kindTick, tk.h+1000)
	if c.tickH != tk.h || c.tickID != tk.id || c.tickWidget != 0x100 {
		t.Fatalf("tickGone of another handle cleared the tick: %#x/%d/%d", c.tickWidget, c.tickID, c.tickH)
	}

	ps[0].closeNow(true)
	l.destroyWidget(0x100)
	if l.tickRemoves != 0 {
		t.Error("the clock removed the tick of a destroyed widget")
	}
	if len(l.ticks) != 1 || l.ticks[0].widget != 0x200 || c.tickWidget != 0x200 || c.tickH != l.ticks[0].h {
		t.Fatalf("after the widget went: ticks %+v, clock on %#x", l.ticks, c.tickWidget)
	}
	if len(l.timeouts) != 1 || !c.stall {
		t.Errorf("%d timeouts (stall %v), want the watchdog kept", len(l.timeouts), c.stall)
	}
	checkLen(t, base+2)

	// The last pacer destroyed: the request falls back to the timeout.
	ps[1].closeNow(true)
	l.destroyWidget(0x200)
	if len(l.ticks) != 0 || c.tickID != 0 || c.tickH != 0 || len(l.timeouts) != 1 || c.stall {
		t.Fatalf("after the last pacer went: %d ticks, clock %d, %d timeouts (stall %v)", len(l.ticks), c.tickID, len(l.timeouts), c.stall)
	}
	l.fireTimeout(t)
	checkLog(t, log, defaultPeriod)

	// Without a request it only forgets the tick.
	ps = addPacers(s, 0x300)
	c.RequestFrame()
	c.fn = func(time.Duration) {}
	l.fireTimeout(t) // the watchdog's frame; the tick stays
	ps[0].closeNow(true)
	l.destroyWidget(0x300)
	if c.tickID != 0 || len(l.ticks) != 0 || len(l.timeouts) != 0 {
		t.Errorf("idle: clock tick %d, %d ticks, %d timeouts", c.tickID, len(l.ticks), len(l.timeouts))
	}
	checkLen(t, base)
}

// TestClockOrphanTick: a tick callback that is not the clock's current one
// (its handle is not tickH) goes at once and runs no frame.
func TestClockOrphanTick(t *testing.T) {
	s, c, l := newClockTest(t)
	base := dispatch.Len()
	addPacers(s, 0x100)
	c.begin(0)
	c.RequestFrame()
	id, h := l.addTick(0x200, c, kindTick) // a second callback, not the clock's
	if id == 0 {
		t.Fatal("addTick failed")
	}
	tickH, tickID := c.tickH, c.tickID
	if r := l.fireTick(t, 0x200, l.now, 0, 0); r != gtk.SourceRemove {
		t.Error("an orphan tick returned G_SOURCE_CONTINUE")
	}
	if c.frames != 0 || !c.requested || c.tickH != tickH || c.tickID != tickID || len(l.timeouts) != 1 {
		t.Errorf("an orphan tick changed the clock: %d frames, requested %v, tick %d/%d", c.frames, c.requested, c.tickID, c.tickH)
	}
	if _, ok := dispatch.Get(h); ok {
		t.Error("the orphan's handle was not freed")
	}
	if r := tickCallback(0x100, testFrameClock, uintptr(h)); r != gtk.SourceRemove || c.frames != 0 {
		t.Error("a stale handle ran a frame")
	}
	l.fireTick(t, 0x100, l.now, 0, 0)
	if c.frames != 1 {
		t.Errorf("the clock's own tick ran %d frames", c.frames)
	}
	fireStale(t, c, l)
	checkLen(t, base)
}

// TestClockPacerChangeInFrame: the pacing surface changes during a tick's
// frame that requests another: the tick returns G_SOURCE_REMOVE, a tick
// on the new pacer is added, and the clock keeps the new one (regression:
// the old callback's return and its destroy notify must not clear the new
// tick).
func TestClockPacerChangeInFrame(t *testing.T) {
	s, c, l := newClockTest(t)
	base := dispatch.Len()
	log := recordFrames(c)
	ps := addPacers(s, 0x100, 0x200)
	c.begin(0)
	c.SetFrameFunc(func(ft time.Duration) {
		*log = append(*log, fmt.Sprintf("fn %v", ft))
		ps[0].w = 0
		c.surfacesChanged() // inside the frame: waits for its end
		c.RequestFrame()
		if len(l.ticks) != 1 || l.tickRemoves != 0 {
			t.Error("the pacer change was handled inside the frame")
		}
	})
	c.RequestFrame()
	if r := l.fireTick(t, 0x100, l.now, 0, 0); r != gtk.SourceRemove {
		t.Fatal("the tick of the old pacer returned G_SOURCE_CONTINUE")
	}
	checkLog(t, log, defaultPeriod)
	if len(l.ticks) != 1 || l.ticks[0].widget != 0x200 {
		t.Fatalf("ticks %+v, want one on 0x200", l.ticks)
	}
	if c.tickWidget != 0x200 || c.tickID != l.ticks[0].id || c.tickH != l.ticks[0].h {
		t.Fatalf("clock tick %#x/%d/%d, want 0x200/%d/%d", c.tickWidget, c.tickID, c.tickH, l.ticks[0].id, l.ticks[0].h)
	}
	if len(l.timeouts) != 1 || !c.stall {
		t.Errorf("%d timeouts (stall %v), want the watchdog", len(l.timeouts), c.stall)
	}
	checkLen(t, base+2)
	c.SetFrameFunc(nil)
	if r := l.fireTick(t, 0x200, l.now+16666, 0, 0); r != gtk.SourceRemove {
		t.Error("idle tick kept")
	}
	if c.frames != 2 || len(l.ticks) != 0 {
		t.Errorf("frames %d, %d ticks", c.frames, len(l.ticks))
	}
	fireStale(t, c, l)
	checkLen(t, base)
}

// TestClockDetachInFrame: the tick's widget is detached during its own
// frame (its surface closed): the callback is removed when it returns,
// and a request from the frame moves to the fallback.
func TestClockDetachInFrame(t *testing.T) {
	s, c, l := newClockTest(t)
	base := dispatch.Len()
	ps := addPacers(s, 0x100)
	c.begin(0)
	c.SetFrameFunc(func(time.Duration) {
		ps[0].closeNow(true)
		c.detach(0x100)
		c.RequestFrame()
	})
	c.RequestFrame()
	l.fireTick(t, 0x100, l.now, 0, 0)
	if len(l.ticks) != 0 || c.tickID != 0 || l.destroys != 1 || l.tickRemoves != 1 {
		t.Errorf("after detach in the frame: %d ticks, clock %d, %d destroys, %d removes", len(l.ticks), c.tickID, l.destroys, l.tickRemoves)
	}
	if len(l.timeouts) != 1 || c.stall {
		t.Errorf("%d timeouts (stall %v), want the fallback", len(l.timeouts), c.stall)
	}
	c.SetFrameFunc(nil)
	l.fireTimeout(t)
	if c.frames != 2 || c.timeoutFrames != 1 {
		t.Errorf("frames %d, timeout frames %d", c.frames, c.timeoutFrames)
	}
	checkLen(t, base)
}

// TestClockPanicInTick: a panic in the frame function of a tick's frame is
// trapped; the clock holds no tick, is not in a frame, the watchdog left
// runs nothing, every handle is freed, and the next request schedules
// afresh.
func TestClockPanicInTick(t *testing.T) {
	s, c, l := newClockTest(t)
	base := dispatch.Len()
	quits := 0
	onPanic = func() { quits++ }
	addPacers(s, 0x100)
	c.begin(0)
	c.SetFrameFunc(func(time.Duration) { panic("in frame") })
	c.RequestFrame()
	if r := l.fireTick(t, 0x100, l.now, 0, 0); r != gtk.SourceRemove {
		t.Error("a panicking tick returned G_SOURCE_CONTINUE")
	}
	if pendingPanic == nil || pendingPanic.Value != "in frame" || pendingPanic.Callback != nameTick || quits != 1 {
		t.Fatalf("pendingPanic %v, %d onPanic calls; want the frame's panic from %s", pendingPanic, quits, nameTick)
	}
	pendingPanic = nil
	if c.tickID != 0 || c.tickWidget != 0 || c.tickH != 0 || c.inFrame || c.requested {
		t.Errorf("clock state after the panic: tick %#x/%d/%d, inFrame %v, requested %v",
			c.tickWidget, c.tickID, c.tickH, c.inFrame, c.requested)
	}
	if len(l.ticks) != 0 {
		t.Errorf("%d ticks left", len(l.ticks))
	}
	fireStale(t, c, l)
	checkLen(t, base)

	log := recordFrames(c)
	c.RequestFrame()
	if len(l.ticks) != 1 || len(l.timeouts) != 1 || c.tickID != l.ticks[0].id {
		t.Fatalf("request after the panic: %d ticks, %d timeouts", len(l.ticks), len(l.timeouts))
	}
	l.now += 16666
	l.fireTick(t, 0x100, l.now, 0, 0)
	if len(*log) != 2 {
		t.Errorf("frame after the panic: %v", *log)
	}
	fireStale(t, c, l)
	checkLen(t, base)
}

// TestClockPanicInTimeout: a panic in the paint pass of a fallback frame
// is trapped; sourceFunc frees the timeout's handle and the clock holds
// none, so the next request schedules afresh.
func TestClockPanicInTimeout(t *testing.T) {
	_, c, l := newClockTest(t)
	base := dispatch.Len()
	c.begin(0)
	c.paint = func(time.Duration) { panic("in paint") }
	c.RequestFrame()
	l.fireTimeout(t)
	if pendingPanic == nil || pendingPanic.Value != "in paint" || pendingPanic.Callback != nameSourceFunc {
		t.Fatalf("pendingPanic %v, want the paint pass's panic from %s", pendingPanic, nameSourceFunc)
	}
	pendingPanic = nil
	if c.timeoutID != 0 || c.timeoutH != 0 || c.inFrame || c.requested {
		t.Errorf("clock state after the panic: timeout %d/%d, inFrame %v, requested %v", c.timeoutID, c.timeoutH, c.inFrame, c.requested)
	}
	checkLen(t, base)
	log := recordFrames(c)
	c.RequestFrame()
	if len(l.timeouts) != 1 {
		t.Fatalf("request after the panic: %d timeouts", len(l.timeouts))
	}
	l.fireTimeout(t)
	if len(*log) != 2 {
		t.Errorf("frame after the panic: %v", *log)
	}
	checkLen(t, base)
}

// TestClockPanicInWatchdog: a panic in the watchdog's frame leaves the
// tick callback, which removes itself when it comes idle.
func TestClockPanicInWatchdog(t *testing.T) {
	s, c, l := newClockTest(t)
	base := dispatch.Len()
	addPacers(s, 0x100)
	c.begin(0)
	c.SetFrameFunc(func(time.Duration) { panic("in watchdog frame") })
	c.RequestFrame()
	l.fireTimeout(t)
	if pendingPanic == nil || pendingPanic.Callback != nameSourceFunc {
		t.Fatalf("pendingPanic %v", pendingPanic)
	}
	pendingPanic = nil
	if c.timeoutID != 0 || c.stall || c.inFrame {
		t.Errorf("timeout %d, stall %v, inFrame %v", c.timeoutID, c.stall, c.inFrame)
	}
	if r := l.fireTick(t, 0x100, l.now, 0, 0); r != gtk.SourceRemove || len(l.ticks) != 0 {
		t.Error("idle tick after the panic not removed")
	}
	checkLen(t, base)
}

// TestClockPanicInTickGone: a panic in the reschedule of tickGone (here,
// the frame clock's loop panics in addTick) still frees the handle.
func TestClockPanicInTickGone(t *testing.T) {
	s, c, l := newClockTest(t)
	base := dispatch.Len()
	ps := addPacers(s, 0x100, 0x200)
	c.begin(0)
	c.RequestFrame()
	h := c.tickH
	ps[1].w = 0
	ps[0].closeNow(true)
	s.surfaces = append(s.surfaces, &panicSurface{})
	l.destroyWidget(0x100)
	if pendingPanic == nil || pendingPanic.Callback != nameDestroyNotify {
		t.Fatalf("pendingPanic %v, want one from %s", pendingPanic, nameDestroyNotify)
	}
	pendingPanic = nil
	if _, ok := dispatch.Get(h); ok {
		t.Error("destroyNotify did not free the handle after a panic")
	}
	if c.tickH != 0 {
		t.Error("the clock kept the tick of the destroyed widget")
	}
	s.surfaces = nil
	c.close()
	checkLen(t, base)
}

// panicSurface is an openSurface whose tickWidget panics.
type panicSurface struct{}

func (*panicSurface) closeNow(bool)       {}
func (*panicSurface) tickWidget() uintptr { panic("tickWidget") }

// TestClockClose: close removes the tick callback and the timeout and
// frees their handles; nothing is scheduled afterwards.
func TestClockClose(t *testing.T) {
	s, c, l := newClockTest(t)
	base := dispatch.Len()
	ps := addPacers(s, 0x100)
	c.begin(0)
	c.RequestFrame()
	checkLen(t, base+2)
	c.close()
	if l.tickRemoves != 1 || l.removes != 1 || len(l.ticks) != 0 || len(l.timeouts) != 0 {
		t.Errorf("close: %d tick removes, %d removes, %d ticks, %d timeouts", l.tickRemoves, l.removes, len(l.ticks), len(l.timeouts))
	}
	if c.tickID != 0 || c.tickH != 0 || c.timeoutID != 0 || c.timeoutH != 0 {
		t.Errorf("close left tick %d/%d, timeout %d/%d", c.tickID, c.tickH, c.timeoutID, c.timeoutH)
	}
	checkLen(t, base)
	c.close() // again: nothing
	c.requested = false
	c.RequestFrame()
	ps[0].w = 0x200
	c.surfacesChanged()
	c.detach(0x200)
	if len(l.ticks) != 0 || len(l.timeouts) != 0 || l.ticksAdded != 1 || l.timeoutsAdded != 1 {
		t.Errorf("after close: %d ticks, %d timeouts scheduled", len(l.ticks), len(l.timeouts))
	}
	checkLen(t, base)
}

// TestClockCloseFallback: close removes a pending fallback timeout.
func TestClockCloseFallback(t *testing.T) {
	_, c, l := newClockTest(t)
	base := dispatch.Len()
	c.begin(0)
	c.RequestFrame()
	c.close()
	if l.removes != 1 || len(l.timeouts) != 0 || c.timeoutID != 0 {
		t.Errorf("close: %d removes, %d timeouts", l.removes, len(l.timeouts))
	}
	checkLen(t, base)
}

// TestClockQuit: after Quit no frame runs, from a tick or a timeout, and
// requests schedule nothing.
func TestClockQuit(t *testing.T) {
	s, c, l := newClockTest(t)
	base := dispatch.Len()
	log := recordFrames(c)
	addPacers(s, 0x100)
	c.begin(0)
	c.RequestFrame()
	s.quit = true
	if r := l.fireTick(t, 0x100, l.now, 0, 0); r != gtk.SourceRemove {
		t.Error("tick kept after Quit")
	}
	l.fireTimeout(t) // the watchdog
	if len(*log) != 0 || c.frames != 0 {
		t.Errorf("frames after Quit: %v", *log)
	}
	c.requested = false
	c.RequestFrame()
	if len(l.ticks) != 0 || len(l.timeouts) != 0 {
		t.Error("a request after Quit scheduled something")
	}
	checkLen(t, base)

	// Fallback: a pending timeout fires after Quit without a frame.
	_, c2, l2 := newClockTest(t)
	recordFrames(c2)
	c2.begin(0)
	c2.RequestFrame()
	c2.s.quit = true
	l2.fireTimeout(t)
	if c2.frames != 0 || len(l2.timeouts) != 0 {
		t.Errorf("fallback after Quit: %d frames, %d timeouts", c2.frames, len(l2.timeouts))
	}
}

// TestShellRunClockFallback runs frames through the real Run with the
// shell's fakeLoop: requested in Ready, they run from the fallback timeout
// inside the loop; requests from the frame run more frames, and Quit from
// the third ends Run with nothing left.
func TestShellRunClockFallback(t *testing.T) {
	s, l := newTestShell(t)
	base := dispatch.Len()
	var targets []time.Duration
	err := run(t, s, &handler{ready: func() {
		c := s.Clock()
		if now := c.Now(); now < 0 || now > time.Second {
			t.Errorf("Now in Ready = %v", now)
		}
		c.SetFrameFunc(func(ft time.Duration) {
			if !s.running {
				t.Error("frame outside the loop")
			}
			targets = append(targets, ft)
			if len(targets) < 3 {
				c.RequestFrame()
			} else {
				s.Quit()
				c.RequestFrame() // after Quit: nothing
			}
		})
		c.RequestFrame()
		c.RequestFrame()
	}})
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	if len(targets) != 3 || s.clock.timeoutFrames != 3 || l.timeouts != 3 {
		t.Errorf("targets %v, %d timeout frames, %d timeouts; want 3 frames", targets, s.clock.timeoutFrames, l.timeouts)
	}
	for i := 1; i < len(targets); i++ {
		if targets[i] <= targets[i-1] {
			t.Errorf("targets %v not increasing", targets)
			break
		}
	}
	checkTornDown(t, s, l, base)
}

// TestShellRunClockQuitPending: a frame requested in Ready and still
// pending at Quit never runs; the shutdown removes its timeout.
func TestShellRunClockQuitPending(t *testing.T) {
	s, l := newTestShell(t)
	base := dispatch.Len()
	frames := 0
	err := run(t, s, &handler{ready: func() {
		s.Clock().SetFrameFunc(func(time.Duration) { frames++ })
		s.Clock().RequestFrame()
		s.Quit()
	}})
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	if frames != 0 || l.timeouts != 1 || l.removes != 1 {
		t.Errorf("%d frames, %d timeouts, %d removes; want 0, 1, 1", frames, l.timeouts, l.removes)
	}
	checkTornDown(t, s, l, base)
}

// TestClockHidden: with an open surface and none pacing (hidden or known
// to be minimized) a running animation's fallback frames come
// hiddenInterval apart, their target times still moving; the surface shown
// again moves the pending request to its tick at once. With no surface at
// all the fallback keeps the 60 Hz grid (TestClockFallback) (S6.2b).
func TestClockHidden(t *testing.T) {
	s, c, l := newClockTest(t)
	ps := addPacers(s, 0) // open, not pacing
	var targets []time.Duration
	c.SetFrameFunc(func(ft time.Duration) {
		targets = append(targets, ft)
		c.RequestFrame()
	})
	c.begin(0)
	c.RequestFrame()
	l.fireTimeout(t) // the first frame: at once
	for i := range 4 {
		if len(l.timeouts) != 1 || c.stall {
			t.Fatalf("frame %d: timeouts %+v (stall %v), want the fallback", i, l.timeouts, c.stall)
		}
		if got, want := l.timeouts[0].ms, uint32(hiddenInterval/ms); got != want {
			t.Errorf("frame %d: fallback after %d ms, want %d", i, got, want)
		}
		l.fireTimeout(t)
	}
	if c.timeoutFrames != 5 || c.Now() != 4*hiddenInterval {
		t.Errorf("%d fallback frames by %v, want 5 by %v", c.timeoutFrames, c.Now(), 4*hiddenInterval)
	}
	for i := 1; i < len(targets); i++ {
		if d := targets[i] - targets[i-1]; d < hiddenInterval-defaultPeriod || d > hiddenInterval+defaultPeriod {
			t.Errorf("target %d moved %v, want about %v", i, d, hiddenInterval)
		}
	}
	// Shown 10 ms after a frame: the request moves to the tick, no wait.
	l.now += 10_000
	ps[0].w = 0x100
	c.surfacesChanged()
	if len(l.ticks) != 1 || len(l.timeouts) != 1 || !c.stall || l.timeouts[0].ms != uint32(stallTimeout/ms) {
		t.Fatalf("after show: %d ticks, timeouts %+v (stall %v); want the tick and its watchdog", len(l.ticks), l.timeouts, c.stall)
	}
	l.now += 6_000
	l.fireTick(t, 0x100, l.now, 0, 0)
	if c.tickFrames != 1 {
		t.Errorf("%d tick frames after show, want 1", c.tickFrames)
	}
	// Minimized again (no pacer): the fallback waits for the low rate.
	ps[0].w = 0
	c.surfacesChanged()
	if len(l.ticks) != 0 || len(l.timeouts) != 1 || c.stall || l.timeouts[0].ms != uint32(hiddenInterval/ms) {
		t.Fatalf("after minimize: %d ticks, timeouts %+v (stall %v); want a fallback of %v", len(l.ticks), l.timeouts, c.stall, hiddenInterval)
	}
	c.SetFrameFunc(nil)
	l.fireTimeout(t)
}

// TestClockStallHidden: a pacing surface whose ticks stop (a minimized
// window on Wayland, which GTK does not report) gets the watchdog's frame
// after stallTimeout, then frames hiddenInterval apart until a tick comes;
// after the tick the watchdog waits stallTimeout again (S6.2b).
func TestClockStallHidden(t *testing.T) {
	s, c, l := newClockTest(t)
	addPacers(s, 0x100)
	c.SetFrameFunc(func(time.Duration) { c.RequestFrame() })
	c.begin(0)
	c.RequestFrame()
	l.fireTimeout(t) // the watchdog's first frame, 100 ms
	if c.stalls != 1 || c.Now() != stallTimeout {
		t.Fatalf("stalls %d at %v, want 1 at %v", c.stalls, c.Now(), stallTimeout)
	}
	for i := range 3 {
		if len(l.timeouts) != 1 || !c.stall || l.timeouts[0].ms != uint32(hiddenInterval/ms) {
			t.Fatalf("stall %d: timeouts %+v (stall %v), want a watchdog of %v", i, l.timeouts, c.stall, hiddenInterval)
		}
		l.fireTimeout(t)
	}
	if c.stalls != 4 || c.Now() != stallTimeout+3*hiddenInterval {
		t.Errorf("stalls %d by %v, want 4 by %v", c.stalls, c.Now(), stallTimeout+3*hiddenInterval)
	}
	// A tick (restored) 20 ms before the armed watchdog: it re-arms for
	// stallTimeout after the tick.
	l.now += int64((hiddenInterval - 20*ms) / time.Microsecond)
	l.fireTick(t, 0x100, l.now, 0, 0)
	tick := c.Now()
	l.fireTimeout(t)
	if c.stalls != 4 || len(l.timeouts) != 1 {
		t.Fatalf("after a tick: stalls %d, timeouts %+v", c.stalls, l.timeouts)
	}
	l.fireTimeout(t)
	if c.stalls != 5 || c.Now() != tick+stallTimeout {
		t.Errorf("stalls %d at %v, want 5 at %v", c.stalls, c.Now(), tick+stallTimeout)
	}
	c.SetFrameFunc(nil)
	l.now += 1000
	l.fireTick(t, 0x100, l.now, 0, 0)
	fireStale(t, c, l)
}

// TestClockPresentFeedback: with a present observer, each tick frame is
// reported once GDK's timings of its frame are complete with a
// presentation time (S6.2d): an incomplete one waits for the next tick,
// one without a presentation time or out of the history is dropped, a new
// frame clock starts afresh; without the observer nothing is read.
func TestClockPresentFeedback(t *testing.T) {
	s, c, l := newClockTest(t)
	addPacers(s, 0x100)
	c.begin(0)
	var targets []time.Duration
	c.SetFrameFunc(func(ft time.Duration) {
		targets = append(targets, ft)
		c.RequestFrame()
	})
	defer c.SetFrameFunc(nil)
	var got []shell.PresentTiming
	c.SetPresentObserver(func(pt shell.PresentTiming) { got = append(got, pt) })
	l.timings = map[int64]fakeTimings{}
	tick := func() {
		t.Helper()
		l.now += 16667
		l.fireTick(t, 0x100, l.now, 16667, l.now+16667)
	}
	want := func(step string, pts ...shell.PresentTiming) {
		t.Helper()
		if !slices.Equal(got, pts) {
			t.Errorf("%s: reported %+v, want %+v", step, got, pts)
		}
		got = nil
	}
	shown := func(counter int, us int64) shell.PresentTiming {
		return shell.PresentTiming{Target: targets[counter-1], Shown: c.at(us), Period: 16667 * time.Microsecond}
	}
	c.RequestFrame()
	tick() // counter 1: a new frame clock, logged
	want("the first tick")
	l.timings[1] = fakeTimings{presented: l.now + 20000, interval: 16667, complete: true}
	tick() // 2
	want("frame 1 presented", shown(1, l.now-16667+20000))
	l.timings[2] = fakeTimings{}
	tick() // 3
	want("frame 2 not complete")
	l.timings[2] = fakeTimings{complete: true} // nothing presented
	l.timings[3] = fakeTimings{presented: l.now + 5000, interval: 8333, complete: true}
	tick() // 4
	want("frame 2 without a presentation, frame 3 presented",
		shell.PresentTiming{Target: targets[2], Shown: c.at(l.now - 16667 + 5000), Period: 8333 * time.Microsecond})
	tick() // 5: frame 4 is out of the history
	want("frame 4 gone")
	c.SetPresentObserver(nil)
	l.timings[5] = fakeTimings{presented: l.now, interval: 16667, complete: true}
	tick() // 6
	want("without the observer")
	if len(targets) != 6 {
		t.Errorf("%d frames, want 6", len(targets))
	}
}
