//go:build darwin

package main

import (
	"fmt"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/radix29/gogui/internal/ffi"
)

// Phase 2 of the self-test (S0.8c): the window and the event loop. The
// app delegate's applicationDidFinishLaunching: starts a worker thread
// (a goroutine locked to its own OS thread) that sends tick: to the
// window's target three times and then probe: to a probe target, all with
// performSelectorOnMainThread:withObject:waitUntilDone:NO. The probe runs
// the in-loop checks on the main thread, closes the window and stops
// [NSApp run]. A watchdog fails the test if run hasn't returned in time.

const (
	loopTimeout = 10 * time.Second
	workerTicks = 3
)

// The window is resized to this content size by the resize check.
const resizedWidth, resizedHeight = 640, 400

type loopState struct {
	r *report
	u *ui
	a *appPeer

	launched        atomic.Bool
	probed          atomic.Bool
	workerNotMain   atomic.Bool // set on the worker thread before it sends anything
	workerRan       atomic.Bool
	multiThreaded   atomic.Bool // [NSThread isMultiThreaded], read on the worker
	callbacksBefore int
}

// probePeer is the peer of the probe GGTarget.
type probePeer struct{ s *loopState }

func (p probePeer) action(self objc.ID, cmd objc.SEL, arg objc.ID) {
	if cmd == sel("tick:") && !p.s.probed.Swap(true) {
		p.s.inLoop()
	}
}

// runLoopPhase runs phase 2 and adds its checks to r.
func runLoopPhase(r *report) {
	s := &loopState{r: r, a: &appPeer{keepRunning: true}, callbacksBefore: ffi.Callbacks()}
	var probe, delegate objc.ID
	ok := true
	withPool(func() {
		ok = r.result("set up NSApp (activation policy, main menu, app delegate) and open the window", protect(func() error {
			delegate = newPeerObject(appDelegateClass, s.a)
			if err := setupApp(delegate); err != nil {
				return err
			}
			s.u = newUI()
			probe = newPeerObject(targetClass, probePeer{s})
			return nil
		}))
	})
	if !ok {
		fmt.Fprintf(r.w, "stopped: the remaining checks depend on this one\n")
		return
	}

	s.a.launched = func() {
		s.launched.Store(true)
		onWorkerThread(func() {
			s.workerNotMain.Store(!isMainThread())
			s.multiThreaded.Store(objc.Send[bool](objc.ID(class("NSThread")), sel("isMultiThreaded")))
			s.workerRan.Store(true)
			for range workerTicks {
				performOnMain(s.u.target, "tick:")
			}
			performOnMain(probe, "tick:")
		})
	}

	done := make(chan struct{})
	var wd sync.WaitGroup
	wd.Go(func() {
		select {
		case <-done:
		case <-time.After(loopTimeout):
			// TryLock: a main thread hung inside result must not block
			// the watchdog.
			r.mu.TryLock()
			fmt.Fprintf(r.w, "FAIL [NSApp run] did not return within %v (launched=%v, worker ran=%v, probed=%v)\n",
				loopTimeout, s.launched.Load(), s.workerRan.Load(), s.probed.Load())
			fmt.Fprintf(r.w, "%d of %d checks passed before the watchdog fired\n", r.pass, r.total)
			os.Exit(1)
		}
	})

	start := time.Now()
	runApp()
	close(done)
	wd.Wait()

	r.result("[NSApp run] returns after stop: and a posted event", func() error {
		if !s.probed.Load() {
			return fmt.Errorf("run returned before the probe ran")
		}
		r.note(fmt.Sprintf("the loop phase took %v", time.Since(start).Round(time.Millisecond)))
		return nil
	}())
	withPool(func() {
		r.result("ffi.Callbacks() is unchanged by the event loop", protect(func() error {
			if got := ffi.Callbacks(); got != s.callbacksBefore || got != trampolines.count {
				return fmt.Errorf("ffi.Callbacks() = %d, want %d", got, trampolines.count)
			}
			return nil
		}))
		freePeerObject(probe)
	})
}

// inLoop runs on the main thread inside [NSApp run], from the probe.
func (s *loopState) inLoop() {
	r, u := s.r, s.u
	run := func(name string, fn func() error) { r.result(name, protect(fn)) }

	run("applicationDidFinishLaunching: ran before the probe", func() error {
		if !s.launched.Load() {
			return fmt.Errorf("the app delegate was not told")
		}
		return nil
	})
	run("performSelectorOnMainThread: from a worker thread runs the Go IMP on the main thread", func() error {
		if !s.workerNotMain.Load() {
			return fmt.Errorf("the worker goroutine reported [NSThread isMainThread] = YES")
		}
		if !isMainThread() {
			return fmt.Errorf("the probe runs off the main thread")
		}
		// A finding for S1.1, not a failure: Cocoa expects an NSThread to
		// exist before plain pthreads call into it.
		r.note(fmt.Sprintf("info: [NSThread isMultiThreaded] on the worker = %v", s.multiThreaded.Load()))
		return nil
	})
	run(fmt.Sprintf("the %d tick: messages sent before the probe arrived first, in order", workerTicks), func() error {
		want := fmt.Sprintf("Ticks: %d", workerTicks)
		if u.ticks != workerTicks || text(u.ticksLabel) != want {
			return fmt.Errorf("ticks = %d, label %q; want %d, %q", u.ticks, text(u.ticksLabel), workerTicks, want)
		}
		return nil
	})
	run("the window is visible, with the GGFlippedView as its content view", func() error {
		if !objc.Send[bool](u.window, sel("isVisible")) {
			return fmt.Errorf("[window isVisible] = NO")
		}
		if cv := u.window.Send(sel("contentView")); cv != u.content {
			return fmt.Errorf("contentView = %#x, want %#x", cv, u.content)
		}
		if !objc.Send[bool](u.content, sel("isFlipped")) {
			return fmt.Errorf("the content view is not flipped")
		}
		return nil
	})
	run(fmt.Sprintf("labels top-left, button top-right (%d pt padding) in window coordinates", padding), func() error {
		return s.checkPlacement(windowWidth, windowHeight)
	})
	run("performClick: on the button updates the click counter", func() error {
		u.button.Send(sel("performClick:"), objc.ID(0))
		if u.clicks != 1 || text(u.clicksLabel) != "Clicks: 1" {
			return fmt.Errorf("clicks = %d, label %q; want 1, %q", u.clicks, text(u.clicksLabel), "Clicks: 1")
		}
		return nil
	})
	run(fmt.Sprintf("resizing to %d×%d sends windowDidResize: and the button stays top-right", resizedWidth, resizedHeight), func() error {
		before := u.resizes
		old := s.buttonInWindow()
		u.window.Send(sel("setContentSize:"), NSSize{Width: resizedWidth, Height: resizedHeight})
		if u.resizes == before {
			return fmt.Errorf("windowDidResize: was not sent")
		}
		if now := s.buttonInWindow(); now.Origin == old.Origin {
			return fmt.Errorf("the button did not move (still at %v)", now.Origin)
		}
		return s.checkPlacement(resizedWidth, resizedHeight)
	})
	run("applicationShouldTerminateAfterLastWindowClosed: (BOOL IMP) answers through the peer", func() error {
		app := nsApp()
		d := app.Send(sel("delegate"))
		if d == 0 {
			return fmt.Errorf("[NSApp delegate] is nil")
		}
		ask := func() bool {
			return objc.Send[bool](d, sel("applicationShouldTerminateAfterLastWindowClosed:"), app)
		}
		s.a.keepRunning = false
		yes := ask()
		s.a.keepRunning = true
		no := ask()
		if !yes || no {
			return fmt.Errorf("got %v with keepRunning=false and %v with keepRunning=true; want true, false", yes, no)
		}
		return nil
	})
	run("closing the window sends windowWillClose:", func() error {
		u.window.Send(sel("close"))
		if !u.closed {
			return fmt.Errorf("windowWillClose: was not sent")
		}
		if objc.Send[bool](u.window, sel("isVisible")) {
			return fmt.Errorf("the window is still visible")
		}
		return nil
	})
	run("[NSApp stop:] and post a dummy event", func() error {
		stopApp()
		return nil
	})
}

// buttonInWindow returns the button's frame in window coordinates (origin
// at the bottom-left of the content area, y up).
func (s *loopState) buttonInWindow() NSRect {
	return objc.Send[NSRect](s.u.button, sel("convertRect:toView:"),
		objc.Send[NSRect](s.u.button, sel("bounds")), objc.ID(0))
}

// checkPlacement checks, in window coordinates, that the button's top and
// right edges are padding pt from the content area's top-right corner, and
// that both labels start padding pt from the left with the clicks label
// padding pt below the top and the ticks label below it.
func (s *loopState) checkPlacement(w, h float64) error {
	u := s.u
	toWin := func(v objc.ID) NSRect {
		return objc.Send[NSRect](v, sel("convertRect:toView:"), objc.Send[NSRect](v, sel("bounds")), objc.ID(0))
	}
	cs := objc.Send[NSRect](u.content, sel("bounds")).Size
	if !near(cs.Width, w) || !near(cs.Height, h) {
		return fmt.Errorf("content size %v×%v, want %v×%v", cs.Width, cs.Height, w, h)
	}
	b := toWin(u.button)
	if !near(b.Origin.X+b.Size.Width, w-padding) || !near(b.Origin.Y+b.Size.Height, h-padding) {
		return fmt.Errorf("button %v: top-right corner at (%v, %v), want (%v, %v)",
			b, b.Origin.X+b.Size.Width, b.Origin.Y+b.Size.Height, w-padding, h-padding)
	}
	c, t := toWin(u.clicksLabel), toWin(u.ticksLabel)
	if !near(c.Origin.X, padding) || !near(c.Origin.Y+c.Size.Height, h-padding) {
		return fmt.Errorf("clicks label %v: top-left corner at (%v, %v), want (%v, %v)",
			c, c.Origin.X, c.Origin.Y+c.Size.Height, float64(padding), h-padding)
	}
	if !near(t.Origin.X, padding) || !near(t.Origin.Y+t.Size.Height, c.Origin.Y-spacing) {
		return fmt.Errorf("ticks label %v: top edge at %v, want %v (%d pt below the clicks label)",
			t, t.Origin.Y+t.Size.Height, c.Origin.Y-spacing, spacing)
	}
	if c.Origin.X+c.Size.Width > b.Origin.X {
		return fmt.Errorf("the clicks label (%v) overlaps the button (%v)", c, b)
	}
	return nil
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }
