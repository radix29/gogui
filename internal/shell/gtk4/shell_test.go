package gtk4

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/radix29/gogui/assets/fonts"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/mainthread"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// fakeLoop is the loop of the host tests, faithful to the GLib semantics
// the shell relies on: idle may be called from any goroutine and queues a
// one-shot source whose user_data is a dispatch handle; run dispatches the
// queued sources in order through the real sourceFunc (a source that
// returns G_SOURCE_CONTINUE is queued again) until quit; quit outside run
// is lost, as g_main_loop_quit before g_main_loop_run; remove drops a
// source that has not run (an unknown id is what g_source_remove reports
// with g_critical, so it is recorded as an error). Every method but idle
// must run on the UI thread, which is checked once q is set.
type fakeLoop struct {
	initErr  error
	failIdle bool   // idle adds no source
	onInit   func() // called in init, on the UI thread
	// newWin creates the windows of newWindow (nil: errNoFakeWindow).
	newWin func(cfg windowConfig, p windowPeer) (window, error)

	q *mainthread.Queue // the shell's queue, for the UI-thread checks

	mu       sync.Mutex
	cond     *sync.Cond
	sources  []fakeSource // queued, in order
	nextID   uint32
	running  bool
	quitting bool

	inits, runs, releases, idles, removes, quits, timeouts int
	errs                                                   []string
}

type fakeSource struct {
	id uint32
	h  ffi.Handle
}

func newFakeLoop() *fakeLoop {
	l := &fakeLoop{}
	l.cond = sync.NewCond(&l.mu)
	return l
}

// uiOnly records an error if the caller is not on the UI thread. l.mu is
// held.
func (l *fakeLoop) uiOnly(what string) {
	if l.q != nil && !l.q.OnUIThread() {
		l.errs = append(l.errs, what+" off the UI thread")
	}
}

func (l *fakeLoop) init() error {
	l.mu.Lock()
	l.uiOnly("init")
	l.inits++
	if l.inits > 1 {
		l.errs = append(l.errs, "init called twice")
	}
	l.mu.Unlock()
	if l.onInit != nil {
		l.onInit()
	}
	return l.initErr
}

func (l *fakeLoop) run() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.uiOnly("run")
	if l.inits != 1 || l.releases != 0 {
		l.errs = append(l.errs, "run outside init…release")
	}
	l.runs++
	l.running, l.quitting = true, false
	defer func() { l.running = false }()
	for {
		for !l.quitting && len(l.sources) == 0 {
			l.cond.Wait()
		}
		if l.quitting {
			return
		}
		src := l.sources[0]
		l.sources = l.sources[1:]
		l.mu.Unlock()
		r := sourceFunc(uintptr(src.h))
		l.mu.Lock()
		if r == gtk.SourceContinue {
			l.sources = append(l.sources, src)
		}
	}
}

func (l *fakeLoop) quit() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.uiOnly("quit")
	l.quits++
	if l.running {
		l.quitting = true
		l.cond.Broadcast()
	}
}

func (l *fakeLoop) idle(p sourcePeer, k kind) (uint32, ffi.Handle) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.failIdle {
		return 0, 0
	}
	l.nextID++
	h := dispatch.New(entry{peer: p, kind: k})
	l.sources = append(l.sources, fakeSource{l.nextID, h})
	l.idles++
	l.cond.Signal()
	return l.nextID, h
}

func (l *fakeLoop) remove(id uint32) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.uiOnly("remove")
	i := slices.IndexFunc(l.sources, func(s fakeSource) bool { return s.id == id })
	if i < 0 {
		l.errs = append(l.errs, fmt.Sprintf("g_source_remove(%d): source not found", id))
		return
	}
	l.sources = slices.Delete(l.sources, i, i+1)
	l.removes++
}

func (l *fakeLoop) release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.uiOnly("release")
	l.releases++
}

// The clockLoop methods: monotonic is the real CLOCK_MONOTONIC-like time
// since base; timeout queues a one-shot source like idle (the delay is
// ignored: the loop runs sources in order); there is no widget, so no
// tick ever fires.

func (l *fakeLoop) monotonic() int64 { return time.Since(fakeBase).Microseconds() + 1_000_000 }

func (l *fakeLoop) timeout(_ uint32, p sourcePeer, k kind) (uint32, ffi.Handle) {
	l.mu.Lock()
	l.uiOnly("timeout")
	l.timeouts++
	l.mu.Unlock()
	return l.idle(p, k)
}

func (l *fakeLoop) addTick(uintptr, tickPeer, kind) (uint32, ffi.Handle) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errs = append(l.errs, "addTick without a widget")
	return 0, 0
}

func (l *fakeLoop) removeTick(uintptr, uint32) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errs = append(l.errs, "removeTick without a widget")
}

func (l *fakeLoop) frameInfo(uintptr) (int64, int64, int64) { return 0, 0, 0 }
func (l *fakeLoop) frameCounter(uintptr) int64              { return 0 }
func (l *fakeLoop) presentation(uintptr, int64) (int64, int64, bool, bool) {
	return 0, 0, false, true
}

// errNoFakeWindow is newWindow's error when the test set no newWin.
var errNoFakeWindow = errors.New("fake loop: no window factory")

// newPopover fails with errNoFakeWindow: the shell tests open no popups.
func (l *fakeLoop) newPopover(window, windowPeer, bool, windowPeer) (popover, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.uiOnly("newPopover")
	return nil, errNoFakeWindow
}

// newWindow calls l.newWin, or fails with errNoFakeWindow without one.
func (l *fakeLoop) newWindow(cfg windowConfig, p windowPeer) (window, error) {
	l.mu.Lock()
	l.uiOnly("newWindow")
	f := l.newWin
	l.mu.Unlock()
	if f == nil {
		return nil, errNoFakeWindow
	}
	return f(cfg, p)
}

// fakeBase is the zero of fakeLoop.monotonic.
var fakeBase = time.Now()

func (l *fakeLoop) idleCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.idles
}

// handler is a shell.Handler made of funcs; nil funcs do nothing.
type handler struct {
	ready      func()
	services   shell.Services // what Ready got
	appearance func(a driver.Appearance)
}

func (h *handler) Ready(sv shell.Services) {
	h.services = sv
	if h.ready != nil {
		h.ready()
	}
}

func (h *handler) AppearanceChanged(a driver.Appearance) {
	if h.appearance != nil {
		h.appearance(a)
	}
}

// nopSurfaceHandler is a non-nil shell.SurfaceHandler for NewSurface's
// argument checks; its methods are never called.
type nopSurfaceHandler struct{ shell.SurfaceHandler }

// newTestShell returns a shell over a fake loop with the UI-thread checks
// on, and resets the panic state around the test.
func newTestShell(t *testing.T) (*Shell, *fakeLoop) {
	t.Helper()
	resetPanics(t)
	l := newFakeLoop()
	s := newShell(l)
	s.Queue().SetChecks(true)
	s.text.fontFiles = func() []fonts.File { return nil } // no app font: the tests stay out of the user's cache
	l.q = s.Queue()
	return s, l
}

// run runs s with h on the calling goroutine and fails the test if Run
// does not return within 10 s.
func run(t *testing.T, s *Shell, h shell.Handler) error {
	t.Helper()
	done := make(chan struct{})
	timer := time.AfterFunc(10*time.Second, func() {
		select {
		case <-done:
		default:
			buf := make([]byte, 1<<20)
			buf = buf[:runtime.Stack(buf, true)]
			s.Post(s.Quit)
			t.Errorf("Run did not return within 10 s; posted Quit. Goroutines:\n%s", buf)
		}
	})
	defer timer.Stop()
	defer close(done)
	return s.Run(h)
}

// runPanics runs s with h and returns what Run panicked with.
func runPanics(t *testing.T, s *Shell, h shell.Handler) (v any) {
	t.Helper()
	defer func() { v = recover() }()
	err := run(t, s, h)
	t.Errorf("Run returned %v, want a panic", err)
	return nil
}

func mustPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	mustPanicWith(t, what, func(any) bool { return true }, fn)
}

func mustPanicWith(t *testing.T, what string, ok func(any) bool, fn func()) {
	t.Helper()
	defer func() {
		t.Helper()
		v := recover()
		if v == nil {
			t.Errorf("%s did not panic", what)
		} else if !ok(v) {
			t.Errorf("%s panicked with %v", what, v)
		}
	}()
	fn()
}

// checkTornDown checks the state after Run, whichever way it ended: the
// loop freed once, no source left in the loop, no wake handle left in the
// dispatch table, the queue closed, the panic state cleared, and no loop
// method called off the UI thread.
func checkTornDown(t *testing.T, s *Shell, l *fakeLoop, base int) {
	t.Helper()
	l.mu.Lock()
	releases, errs, left := l.releases, slices.Clone(l.errs), len(l.sources)
	l.mu.Unlock()
	if releases != 1 {
		t.Errorf("release called %d times, want once", releases)
	}
	for _, e := range errs {
		t.Errorf("fake loop: %s", e)
	}
	if left != 0 {
		t.Errorf("%d sources left in the loop after Run", left)
	}
	checkLen(t, base)
	if len(s.wakePending) != 0 || !s.wakeClosed {
		t.Errorf("wake not stopped: %d pending, closed %v", len(s.wakePending), s.wakeClosed)
	}
	if !s.q.Closed() {
		t.Error("queue not closed after Run")
	}
	if s.settings != nil {
		t.Error("appearance source not closed after Run")
	}
	if onPanic != nil || pendingPanic != nil {
		t.Errorf("panic state left set: onPanic %v, pendingPanic %v", onPanic != nil, pendingPanic)
	}
	if len(s.surfaces) != 0 {
		t.Errorf("%d surfaces left open", len(s.surfaces))
	}
}

func TestShellRunReadyQuit(t *testing.T) {
	s, l := newTestShell(t)
	base := dispatch.Len()
	readies := 0
	err := run(t, s, &handler{ready: func() {
		readies++
		if !s.q.OnUIThread() {
			t.Error("Ready off the UI thread")
		}
		if l.inits != 1 || l.runs != 0 {
			t.Errorf("Ready with %d inits and %d runs, want 1 and 0", l.inits, l.runs)
		}
		if onPanic == nil {
			t.Error("onPanic not set in Ready")
		}
		s.Post(func() {
			if !s.running {
				t.Error("posted work did not run inside the loop")
			}
			s.Quit()
			s.Quit() // a second Quit does nothing
		})
	}})
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	if readies != 1 {
		t.Errorf("Ready called %d times", readies)
	}
	if l.runs != 1 || l.quits != 1 {
		t.Errorf("loop run %d times, quit %d times; want 1 and 1", l.runs, l.quits)
	}
	checkTornDown(t, s, l, base)

	if err := s.Run(&handler{}); !errors.Is(err, ErrRunTwice) {
		t.Errorf("second Run = %v, want ErrRunTwice", err)
	}
	if l.inits != 1 {
		t.Errorf("the second Run called init (%d inits)", l.inits)
	}
	// Post after Quit is dropped.
	ran := false
	s.Post(func() { ran = true })
	if ran || s.q.Drain() != 0 {
		t.Error("work posted after Quit ran")
	}
}

func TestShellRunNilHandler(t *testing.T) {
	s, l := newTestShell(t)
	mustPanic(t, "Run(nil)", func() { _ = s.Run(nil) })
	if l.inits != 0 {
		t.Error("Run(nil) called init")
	}
	// Run(nil) did not start the shell.
	if err := run(t, s, &handler{ready: func() { s.Quit() }}); err != nil {
		t.Errorf("Run after Run(nil) = %v", err)
	}
}

func TestShellRunInitError(t *testing.T) {
	s, l := newTestShell(t)
	base := dispatch.Len()
	cause := errors.New("no GTK")
	l.initErr = fmt.Errorf("loading: %w", cause)
	s.Post(func() { t.Error("posted work ran after a failed init") })
	readies := 0
	err := run(t, s, &handler{ready: func() { readies++ }})
	if !errors.Is(err, cause) {
		t.Fatalf("Run = %v, want the init error", err)
	}
	if readies != 0 || l.runs != 0 || l.releases != 0 {
		t.Errorf("after a failed init: %d readies, %d runs, %d releases; want none", readies, l.runs, l.releases)
	}
	if !s.q.Closed() {
		t.Error("queue not closed after a failed init")
	}
	if l.idleCount() != 0 {
		t.Error("a wake source was added although init failed")
	}
	checkLen(t, base)
	if onPanic != nil {
		t.Error("onPanic set after a failed init")
	}
	if err := s.Run(&handler{}); !errors.Is(err, ErrRunTwice) {
		t.Errorf("Run after a failed Run = %v, want ErrRunTwice", err)
	}
}

func TestShellQuitInReady(t *testing.T) {
	s, l := newTestShell(t)
	base := dispatch.Len()
	ran := 0
	err := run(t, s, &handler{ready: func() {
		s.Post(func() { ran++ })
		s.Quit()
		s.Post(func() { ran += 10 }) // dropped
	}})
	if err != nil || ran != 1 {
		t.Fatalf("Run = %v, ran = %d; want nil and the work posted before Quit run once", err, ran)
	}
	if l.runs != 0 {
		t.Errorf("loop run %d times after Quit in Ready, want 0", l.runs)
	}
	checkTornDown(t, s, l, base)
}

func TestShellPostBeforeRun(t *testing.T) {
	s, l := newTestShell(t)
	base := dispatch.Len()
	var order []string
	s.Post(func() { order = append(order, "posted") })
	if l.idleCount() != 0 {
		t.Error("Post before Run added a source")
	}
	err := run(t, s, &handler{ready: func() {
		order = append(order, "ready")
		s.Post(s.Quit)
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "ready" || order[1] != "posted" {
		t.Errorf("order = %v, want [ready posted]", order)
	}
	checkTornDown(t, s, l, base)
}

// TestShellPostFIFO posts from 8 goroutines: every goroutine's functions
// run in order, all of them run, and the wakes are coalesced.
func TestShellPostFIFO(t *testing.T) {
	const goroutines, per = 8, 1000
	s, l := newTestShell(t)
	base := dispatch.Len()
	next := make([]int, goroutines)
	total := 0
	err := run(t, s, &handler{ready: func() {
		var wg sync.WaitGroup
		for g := range goroutines {
			wg.Go(func() {
				for i := range per {
					s.Post(func() {
						if !s.q.OnUIThread() {
							t.Error("posted work off the UI thread")
						}
						if next[g] != i {
							t.Errorf("goroutine %d: got %d, want %d", g, i, next[g])
						}
						next[g] = i + 1
						total++
					})
				}
			})
		}
		go func() {
			wg.Wait()
			s.Post(s.Quit)
		}()
	}})
	if err != nil {
		t.Fatal(err)
	}
	if total != goroutines*per {
		t.Errorf("%d functions ran, want %d", total, goroutines*per)
	}
	wakes, drains := s.wakes.Load(), s.drains.Load()
	if wakes == 0 || wakes > drains+1 || wakes >= goroutines*per {
		t.Errorf("%d wakes for %d drains and %d functions; want coalesced wakes", wakes, drains, goroutines*per)
	}
	if int64(l.idleCount()) != wakes {
		t.Errorf("%d idle sources for %d wakes", l.idleCount(), wakes)
	}
	t.Logf("%d functions, %d wake sources, %d drains", total, wakes, drains)
	checkTornDown(t, s, l, base)
}

// TestShellWakeCoalescedOnUIThread posts 1000 functions from one posted
// function: one idle source is added for all of them.
func TestShellWakeCoalescedOnUIThread(t *testing.T) {
	s, l := newTestShell(t)
	base := dispatch.Len()
	ran := 0
	var before, after int64
	var idlesBefore, idlesAfter int
	err := run(t, s, &handler{ready: func() {
		s.Post(func() {
			before, idlesBefore = s.wakes.Load(), l.idleCount()
			for range 1000 {
				s.Post(func() { ran++ })
			}
			after, idlesAfter = s.wakes.Load(), l.idleCount()
			s.Post(s.Quit)
		})
	}})
	if err != nil {
		t.Fatal(err)
	}
	if after-before != 1 || idlesAfter-idlesBefore != 1 {
		t.Errorf("1000 posts from the UI thread made %d wakes and %d idle sources, want 1",
			after-before, idlesAfter-idlesBefore)
	}
	if ran != 1000 {
		t.Errorf("%d of 1000 functions ran", ran)
	}
	checkTornDown(t, s, l, base)
}

// TestShellWakeAfterShutdown: a wake after Run adds no source, and a wake
// source that runs after the shell stopped does nothing.
func TestShellWakeAfterShutdown(t *testing.T) {
	s, l := newTestShell(t)
	base := dispatch.Len()
	if err := run(t, s, &handler{ready: func() { s.Quit() }}); err != nil {
		t.Fatal(err)
	}
	idles, wakes, drains := l.idleCount(), s.wakes.Load(), s.drains.Load()
	s.wake()
	if l.idleCount() != idles || s.wakes.Load() != wakes {
		t.Error("a wake after shutdown added a source")
	}
	// A source being added while the shell stopped runs late.
	w := &wakeSource{s: s}
	if w.source(kindWake) {
		t.Error("a late wake source stays")
	}
	if s.drains.Load() != drains {
		t.Error("a late wake source drained the queue")
	}
	checkTornDown(t, s, l, base)
}

// TestShellWakeIdleFails: if GLib adds no source the wake is lost and
// nothing is recorded.
func TestShellWakeIdleFails(t *testing.T) {
	s, l := newTestShell(t)
	l.failIdle = true
	s.wake()
	if len(s.wakePending) != 0 || s.wakes.Load() != 0 {
		t.Errorf("a failed idle recorded %d pending, %d wakes", len(s.wakePending), s.wakes.Load())
	}
}

// TestShellPanicInPostedWork: a panic in posted work (inside the wake's
// idle source) is trapped, quits the loop, and Run re-raises it as a
// *PanicError after the teardown; the wake source that the failed Drain
// requested is removed and its handle freed, and DoSync callers give up.
func TestShellPanicInPostedWork(t *testing.T) {
	s, l := newTestShell(t)
	base := dispatch.Len()
	boom := errors.New("boom")
	syncDone := make(chan bool, 1)
	got := runPanics(t, s, &handler{ready: func() {
		go func() {
			// Waits for a function that never runs: the queue closes.
			time.Sleep(20 * time.Millisecond)
			syncDone <- s.q.DoSync(func() {})
		}()
		s.Post(func() {
			time.Sleep(50 * time.Millisecond)
			panic(boom)
		})
		s.Post(func() { t.Error("work queued behind the panic ran") })
	}})
	pe, ok := got.(*PanicError)
	if !ok {
		t.Fatalf("Run panicked with %T %v, want *PanicError", got, got)
	}
	if !errors.Is(pe, boom) || pe.Value != boom || pe.Callback != nameSourceFunc || len(pe.Stack) == 0 {
		t.Errorf("PanicError = %v in %q, want boom in %s with a stack", pe.Value, pe.Callback, nameSourceFunc)
	}
	if l.removes == 0 {
		t.Error("no pending wake source removed at shutdown")
	}
	checkTornDown(t, s, l, base)
	select {
	case ok := <-syncDone:
		if ok {
			t.Error("DoSync reported success after the panic")
		}
	case <-time.After(5 * time.Second):
		t.Error("DoSync still blocked after Run ended")
	}
}

// TestShellPanicInReady: a panic in Ready propagates out of Run as is,
// after the teardown.
func TestShellPanicInReady(t *testing.T) {
	s, l := newTestShell(t)
	base := dispatch.Len()
	got := runPanics(t, s, &handler{ready: func() {
		s.Post(func() { t.Error("posted work ran after a panic in Ready") })
		panic("in ready")
	}})
	if got != "in ready" {
		t.Fatalf("Run panicked with %v, want \"in ready\"", got)
	}
	if l.runs != 0 {
		t.Error("the loop ran after a panic in Ready")
	}
	checkTornDown(t, s, l, base)
}

// panicPeer is a sourcePeer that panics.
type panicPeer struct{ v any }

func (p panicPeer) source(kind) bool { panic(p.v) }

// TestShellPanicInCallback: a panic trapped in a callback that is not the
// wake (an idle source of its own) quits the loop through onPanic, and
// Run re-raises the *PanicError.
func TestShellPanicInCallback(t *testing.T) {
	s, l := newTestShell(t)
	base := dispatch.Len()
	got := runPanics(t, s, &handler{ready: func() {
		if id, _ := s.loop.idle(panicPeer{"in source"}, kindTestA); id == 0 {
			t.Error("idle added no source")
		}
	}})
	pe, ok := got.(*PanicError)
	if !ok || pe.Value != "in source" || pe.Callback != nameSourceFunc {
		t.Fatalf("Run panicked with %#v, want the *PanicError of the source", got)
	}
	if l.runs != 1 || l.quits != 1 {
		t.Errorf("loop run %d, quit %d times; want 1 and 1 (by onPanic)", l.runs, l.quits)
	}
	checkTornDown(t, s, l, base)
}

// TestShellPanicTrappedInReady: a callback that a GTK call made in Ready
// ran and that panicked is re-raised when Ready returns; the loop never
// runs (its quit, before run, would be lost).
func TestShellPanicTrappedInReady(t *testing.T) {
	s, l := newTestShell(t)
	base := dispatch.Len()
	got := runPanics(t, s, &handler{ready: func() {
		trapped("gtk4.test", func() { panic("nested") })
		if pendingPanic == nil {
			t.Error("trap kept no panic")
		}
	}})
	pe, ok := got.(*PanicError)
	if !ok || pe.Value != "nested" || pe.Callback != "gtk4.test" {
		t.Fatalf("Run panicked with %#v, want the trapped *PanicError", got)
	}
	if l.runs != 0 {
		t.Error("the loop ran after a panic trapped in Ready")
	}
	checkTornDown(t, s, l, base)
}

// closeProbe is an openSurface that logs closeNow and removes itself from
// the shell's list unless stubborn.
type closeProbe struct {
	name     string
	s        *Shell
	log      *[]string
	stubborn bool         // does not remove itself
	closed   func(*Shell) // run as its Closed handler, unless silent
}

func (f *closeProbe) closeNow(silent bool) {
	*f.log = append(*f.log, fmt.Sprintf("%s silent=%v", f.name, silent))
	if !f.stubborn {
		if i := slices.Index(f.s.surfaces, openSurface(f)); i >= 0 {
			f.s.surfaces = slices.Delete(f.s.surfaces, i, i+1)
		}
	}
	if !silent && f.closed != nil {
		f.closed(f.s)
	}
}

// tickWidget implements openSurface: a probe never paces.
func (f *closeProbe) tickWidget() uintptr { return 0 }

func addSurfaces(s *Shell, log *[]string, fs ...*closeProbe) {
	for _, f := range fs {
		f.s, f.log = s, log
		s.surfaces = append(s.surfaces, f)
	}
}

// TestShellQuitClosesSurfaces: Quit closes the surfaces last to first
// with Closed, drops one that does not remove itself, and NewSurface in a
// Closed handler returns ErrQuit; a second Quit closes nothing.
func TestShellQuitClosesSurfaces(t *testing.T) {
	s, l := newTestShell(t)
	base := dispatch.Len()
	var log []string
	var newErr error
	err := run(t, s, &handler{ready: func() {
		addSurfaces(s, &log,
			&closeProbe{name: "a"},
			&closeProbe{name: "b", stubborn: true},
			&closeProbe{name: "c", closed: func(s *Shell) {
				_, newErr = s.NewSurface(shell.SurfaceConfig{}, nopSurfaceHandler{})
			}})
		s.Post(func() {
			s.Quit()
			s.Quit()
		})
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"c silent=false", "b silent=false", "a silent=false"}
	if !slices.Equal(log, want) {
		t.Errorf("closed %v, want %v", log, want)
	}
	if !errors.Is(newErr, ErrQuit) {
		t.Errorf("NewSurface in Closed = %v, want ErrQuit", newErr)
	}
	checkTornDown(t, s, l, base)
}

// TestShellPanicInClosed: a panic a Closed handler left pending (trapped
// in a callback) is re-raised by Quit after its surface; it ends Run, and
// the shutdown closes the surfaces left open silently.
func TestShellPanicInClosed(t *testing.T) {
	s, l := newTestShell(t)
	base := dispatch.Len()
	var log []string
	got := runPanics(t, s, &handler{ready: func() {
		addSurfaces(s, &log,
			&closeProbe{name: "a"},
			&closeProbe{name: "b"},
			&closeProbe{name: "c", closed: func(*Shell) {
				trapped("gtk4.test.closed", func() { panic("closed") })
			}})
		s.Post(s.Quit)
	}})
	pe, ok := got.(*PanicError)
	if !ok || pe.Value != "closed" || pe.Callback != "gtk4.test.closed" {
		t.Fatalf("Run panicked with %#v, want the Closed handler's *PanicError", got)
	}
	want := []string{"c silent=false", "b silent=true", "a silent=true"}
	if !slices.Equal(log, want) {
		t.Errorf("closed %v, want %v", log, want)
	}
	checkTornDown(t, s, l, base)
}

// TestShellPanicLeavesSurfaces: surfaces a panic in Ready left open are
// closed silently, last to first, by the shutdown.
func TestShellPanicLeavesSurfaces(t *testing.T) {
	s, l := newTestShell(t)
	base := dispatch.Len()
	var log []string
	got := runPanics(t, s, &handler{ready: func() {
		addSurfaces(s, &log,
			&closeProbe{name: "a", closed: func(*Shell) { t.Error("Closed called by the shutdown") }},
			&closeProbe{name: "b", stubborn: true})
		panic("in ready")
	}})
	if got != "in ready" {
		t.Fatalf("Run panicked with %v", got)
	}
	want := []string{"b silent=true", "a silent=true"}
	if !slices.Equal(log, want) {
		t.Errorf("closed %v, want %v", log, want)
	}
	checkTornDown(t, s, l, base)
}

// TestShellReadyGuard: the UI-thread methods panic before Ready (here in
// init, on the UI thread, so the ready guard and not the thread check
// fires) and off the UI thread.
func TestShellReadyGuard(t *testing.T) {
	s, l := newTestShell(t)
	guarded := map[string]func(){
		"TextSystem": func() { s.TextSystem() },
		"Appearance": func() { s.Appearance() },
		"Clock":      func() { s.Clock() },
		"NewSurface": func() { _, _ = s.NewSurface(shell.SurfaceConfig{}, nopSurfaceHandler{}) },
	}
	isGuard := func(name string) func(any) bool {
		return func(v any) bool {
			msg, _ := v.(string)
			return strings.Contains(msg, "Shell."+name) && strings.Contains(msg, "before Handler.Ready")
		}
	}
	for name, fn := range guarded {
		mustPanic(t, name+" before Run", fn)
	}
	l.onInit = func() {
		for name, fn := range guarded {
			mustPanicWith(t, name+" in init", isGuard(name), fn)
		}
	}
	err := run(t, s, &handler{ready: func() {
		done := make(chan struct{})
		go func() {
			defer close(done)
			for name, fn := range guarded {
				mustPanicWith(t, name+" off the UI thread", func(v any) bool {
					msg, _ := v.(string)
					return strings.Contains(msg, "off the UI thread")
				}, fn)
			}
		}()
		<-done
		s.Quit()
	}})
	if err != nil {
		t.Fatal(err)
	}
}

// TestShellServices: Appearance is the source's reading, TextSystem and Clock
// are the same value on every call, and NewSurface checks its handler and
// fails with the loop's error (ErrQuit after Quit).
func TestShellServices(t *testing.T) {
	s, _ := newTestShell(t)
	src := newFakeAppSource()
	src.portal[keyColorScheme] = uint32(1)
	src.gtk[gtkEnableAnimations] = false
	s.openSettings = src.open
	h := &handler{}
	h.ready = func() {
		want := driver.DefaultAppearance()
		want.Dark, want.ReduceMotion = true, true
		if a := s.Appearance(); a != want {
			t.Errorf("Appearance() = %+v, want %+v", a, want)
		}
		if ts := s.TextSystem(); ts == nil || ts != s.TextSystem() {
			t.Error("TextSystem nil or not the same value on every call")
		}
		if c := s.Clock(); c == nil || c != s.Clock() {
			t.Error("Clock nil or not the same value on every call")
		}
		if h.services.Text != s.TextSystem() || h.services.Clock != s.Clock() {
			t.Errorf("Ready got %+v, want the text system and the clock", h.services)
		}
		if s.Caps().PopupShadow {
			t.Error("Caps with PopupShadow: gtk4 draws none")
		}
		mustPanic(t, "NewSurface with a nil handler", func() { _, _ = s.NewSurface(shell.SurfaceConfig{}, nil) })
		if sf, err := s.NewSurface(shell.SurfaceConfig{}, nopSurfaceHandler{}); sf != nil || !errors.Is(err, errNoFakeWindow) {
			t.Errorf("NewSurface = %v, %v; want nil, errNoFakeWindow", sf, err)
		}
		s.Quit()
		if sf, err := s.NewSurface(shell.SurfaceConfig{}, nopSurfaceHandler{}); sf != nil || !errors.Is(err, ErrQuit) {
			t.Errorf("NewSurface after Quit = %v, %v; want nil, ErrQuit", sf, err)
		}
		mustPanic(t, "NewSurface with a nil handler after Quit", func() { _, _ = s.NewSurface(shell.SurfaceConfig{}, nil) })
	}
	if err := run(t, s, h); err != nil {
		t.Fatal(err)
	}
}

// fakeAppSource is an appearanceSource whose values the test sets. Its
// opener records the changed func; changed may be called on any
// goroutine, as godbus does.
type fakeAppSource struct {
	mu      sync.Mutex
	portal  map[string]any
	gtk     map[string]bool
	ints    map[string]int32
	changed func()
	opens   int
	closes  int
	onOpen  func(changed func()) // called in the opener, on the UI thread
}

func newFakeAppSource() *fakeAppSource {
	return &fakeAppSource{portal: map[string]any{}, gtk: map[string]bool{}, ints: map[string]int32{}}
}

func (f *fakeAppSource) open(changed func()) appearanceSource {
	f.mu.Lock()
	f.opens++
	f.changed = changed
	f.mu.Unlock()
	if f.onOpen != nil {
		f.onOpen(changed)
	}
	return f
}

func (f *fakeAppSource) setPortal(key string, v any) {
	f.mu.Lock()
	f.portal[key] = v
	f.mu.Unlock()
}

func (f *fakeAppSource) portalValue(key string) (any, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.portal[key]
	return v, ok
}

func (f *fakeAppSource) gtkBool(name string) (bool, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.gtk[name]
	return v, ok
}

func (f *fakeAppSource) gtkInt(name string) (int32, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.ints[name]
	return v, ok
}

func (f *fakeAppSource) close() {
	f.mu.Lock()
	f.closes++
	f.mu.Unlock()
}

func (f *fakeAppSource) counts() (opens, closes int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.opens, f.closes
}

// TestShellDefaultSettings: newShell's source has every value unset, so
// the appearance is the default one.
func TestShellDefaultSettings(t *testing.T) {
	s, _ := newTestShell(t)
	err := run(t, s, &handler{ready: func() {
		defer s.Quit()
		if a := s.Appearance(); a != driver.DefaultAppearance() {
			t.Errorf("Appearance() = %+v, want driver.DefaultAppearance()", a)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
}

// TestAppearanceChanged: the appearance is read before Ready; the source's
// changes are posted to the UI thread, re-read and deduped: dark, then
// the accent, exactly two callbacks. A change reported while the source
// opens (before Ready) gives none, nor does one after Quit (its post is
// dropped) or after Run. The source is opened once and closed once.
func TestAppearanceChanged(t *testing.T) {
	s, l := newTestShell(t)
	base := dispatch.Len()
	src := newFakeAppSource()
	src.gtk[gtkOverlayScrolling] = false // legacy scrollers from the start
	src.onOpen = func(changed func()) {
		src.setPortal(keyContrast, uint32(1))
		changed() // before Ready: posted, then deduped against the read
	}
	s.openSettings = src.open
	accent := [3]float64{0.25, 0.5, 1}
	var got []driver.Appearance
	err := run(t, s, &handler{
		ready: func() {
			a := s.Appearance()
			if a.Dark || !a.LegacyScrollers || !a.IncreaseContrast {
				t.Errorf("initial Appearance() = %+v, want light, legacy scrollers, increased contrast", a)
			}
			go func() { // as godbus's goroutine
				// Each change waits for its re-read (posts run in order),
				// so the two changes are read apart.
				settle := func() { s.q.DoSync(func() {}) }
				src.changed() // no change
				src.setPortal(keyColorScheme, uint32(1))
				src.changed()
				src.changed() // no change
				settle()
				src.setPortal(keyAccentColor, accent)
				src.changed()
				s.Post(func() {
					a := s.Appearance()
					if !a.Dark || a.Accent != driver.RGBA(0.25, 0.5, 1, 1) {
						t.Errorf("Appearance() = %+v after the changes", a)
					}
					s.Quit()
					src.setPortal(keyColorScheme, uint32(0))
					src.changed() // after Quit: dropped
				})
			}()
		},
		appearance: func(a driver.Appearance) {
			if !s.q.OnUIThread() {
				t.Error("AppearanceChanged off the UI thread")
			}
			got = append(got, a)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	src.changed() // after Run: dropped
	if len(got) != 2 || !got[0].Dark || got[0].Accent != driver.DefaultAccent ||
		got[1].Accent != driver.RGBA(0.25, 0.5, 1, 1) || !got[1].LegacyScrollers {
		t.Errorf("AppearanceChanged calls %+v, want dark, then the new accent", got)
	}
	if !s.appearance.Dark {
		t.Error("the change posted after Quit was applied")
	}
	if opens, closes := src.counts(); opens != 1 || closes != 1 {
		t.Errorf("source opened %d and closed %d times, want once each", opens, closes)
	}
	checkTornDown(t, s, l, base)
}

// TestAppearanceSourceClosedOnPanic: a panic in Ready still closes the
// source.
func TestAppearanceSourceClosedOnPanic(t *testing.T) {
	s, l := newTestShell(t)
	base := dispatch.Len()
	src := newFakeAppSource()
	s.openSettings = src.open
	if got := runPanics(t, s, &handler{ready: func() { panic("in ready") }}); got != "in ready" {
		t.Fatalf("Run panicked with %v", got)
	}
	if opens, closes := src.counts(); opens != 1 || closes != 1 {
		t.Errorf("source opened %d and closed %d times, want once each", opens, closes)
	}
	checkTornDown(t, s, l, base)
}
