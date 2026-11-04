//go:build darwin

package main

import (
	"fmt"
	"io"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego/objc"

	"github.com/radix29/gogui/internal/ffi"
)

// The self-test, in two phases. Phase 1 (S0.8b) checks classes, ivars and
// struct passing inside [NSApplication sharedApplication], with no window
// and no run loop. Phase 2 (S0.8c, selftest_loop.go) opens the window and
// runs [NSApp run]. Each check prints one line; runSelfTest reports
// whether all passed.

type check struct {
	name string
	fn   func() error
}

// report prints check results and counts them.
type report struct {
	mu          sync.Mutex // the watchdog prints from another goroutine
	w           io.Writer
	pass, total int
	notes       []string // printed under the next result line
}

// note queues an indented line to print under the next result.
func (r *report) note(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.notes = append(r.notes, s)
}

func (r *report) result(name string, err error) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer func() {
		for _, n := range r.notes {
			fmt.Fprintf(r.w, "     (%s)\n", n)
		}
		r.notes = nil
	}()
	r.total++
	if err != nil {
		fmt.Fprintf(r.w, "FAIL %s: %v\n", name, err)
		return false
	}
	r.pass++
	fmt.Fprintf(r.w, "ok   %s\n", name)
	return true
}

// runSelfTest runs phase 1, then phase 2. It stops at the first check that
// leaves nothing for the rest to test (loading the runtime, NSApp,
// defining the classes, opening the window). A panic in a check is a
// failure of that check.
func runSelfTest(w io.Writer) bool {
	fmt.Fprintf(w, "appkit self-test (S0.8b, S0.8c) on darwin/%s\n", runtime.GOARCH)
	r := &report{w: w}
	checks := []check{
		{"load libobjc and AppKit", loadRuntime},
		{"NSApplication sharedApplication", checkSharedApp},
		{"define GGTarget, GGFlippedView, GGWindowDelegate and GGAppDelegate", defineClasses},
		{"defining a class again fails cleanly", checkRedefine},
		{"handle ivar round-trips through ffi.Handles", checkHandleIvar},
		{"setFrame:/frame on NSView", func() error { return checkFrame("NSView") }},
		{"setFrame:/frame on GGFlippedView", func() error { return checkFrame(flippedClass) }},
		{"GGFlippedView is flipped", checkFlipped},
		{"performClick: runs the Go action once", checkClick},
		{"ffi.Callbacks() equals the spike's trampolines", checkCallbackCount},
	}
	essential := map[int]bool{0: true, 1: true, 2: true}
	for i, c := range checks {
		var err error
		if i == 0 {
			// The pool class is in Foundation, which this check loads.
			err = protect(c.fn)
		} else {
			withPool(func() { err = protect(c.fn) })
		}
		if !r.result(c.name, err) && essential[i] {
			fmt.Fprintf(w, "stopped: the remaining checks depend on this one\n")
			return r.summary()
		}
	}
	runLoopPhase(r)
	return r.summary()
}

func (r *report) summary() bool {
	fmt.Fprintf(r.w, "%d of %d checks passed\n", r.pass, r.total)
	return r.pass == r.total && r.total > 0
}

func protect(fn func() error) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic: %v", rec)
		}
	}()
	return fn()
}

func checkSharedApp() error {
	app := objc.ID(class("NSApplication")).Send(sel("sharedApplication"))
	if app == 0 {
		return fmt.Errorf("sharedApplication returned nil")
	}
	if !objc.Send[bool](objc.ID(class("NSThread")), sel("isMainThread")) {
		return fmt.Errorf("not on the main thread")
	}
	return nil
}

func checkRedefine() error {
	for _, d := range classDefs() {
		if _, err := defineClass(d); err == nil {
			return fmt.Errorf("second definition of %s succeeded", d.name)
		}
		if objc.GetClass(d.name) == 0 {
			return fmt.Errorf("the failed redefinition removed %s", d.name)
		}
	}
	return nil
}

// recorder is a GGTarget peer that records its calls.
type recorder struct {
	calls  int
	self   objc.ID
	cmd    objc.SEL
	sender objc.ID
}

func (r *recorder) action(self objc.ID, cmd objc.SEL, sender objc.ID) {
	r.calls++
	r.self, r.cmd, r.sender = self, cmd, sender
}

func checkHandleIvar() error {
	obj := alloc(targetClass, "init")
	defer release(obj)
	if h := getHandle(obj); h != 0 {
		return fmt.Errorf("fresh object has handle %v, want 0", h)
	}
	if _, ok := peers.Get(getHandle(obj)); ok {
		return fmt.Errorf("the zero handle resolves to a peer")
	}
	p := &recorder{}
	h := peers.New(p)
	defer peers.Free(h)
	setHandle(obj, h)
	if got := getHandle(obj); got != h {
		return fmt.Errorf("handle read back as %v, want %v", got, h)
	}
	got, ok := peers.Get(getHandle(obj))
	if !ok || got != actionPeer(p) {
		return fmt.Errorf("peers.Get(ivar) = %v, %v; want the stored peer", got, ok)
	}
	// The ivar must not overlap the object's isa.
	if off := handleOffset(obj); off < unsafe.Sizeof(uintptr(0)) {
		return fmt.Errorf("ivar offset %d overlaps isa", off)
	}
	return nil
}

// Non-integer values with exact binary fractions, all different, so a
// swapped, truncated or misplaced field shows up.
var (
	initRect = rect(1.5, 2.25, 300.75, 200.125)
	setRect  = rect(12.25, 34.5, 101.75, 56.625)
)

func checkFrame(cls string) error {
	v := alloc(cls, "initWithFrame:", initRect)
	defer release(v)
	if got := objc.Send[NSRect](v, sel("frame")); got != initRect {
		return fmt.Errorf("initWithFrame: %v, then frame = %v", initRect, got)
	}
	v.Send(sel("setFrame:"), setRect)
	if got := objc.Send[NSRect](v, sel("frame")); got != setRect {
		return fmt.Errorf("setFrame: %v, then frame = %v", setRect, got)
	}
	// The 16-byte structs as arguments (registers on both architectures).
	v.Send(sel("setFrameOrigin:"), pt(7.5, 8.75))
	v.Send(sel("setFrameSize:"), NSSize{Width: 64.25, Height: 32.5})
	if got, want := objc.Send[NSRect](v, sel("frame")), rect(7.5, 8.75, 64.25, 32.5); got != want {
		return fmt.Errorf("setFrameOrigin:/setFrameSize:, then frame = %v, want %v", got, want)
	}
	return nil
}

// checkFlipped builds root (NSView, 200×100) ⊃ flipped (GGFlippedView,
// filling root) ⊃ sub (NSView at (10, 10), 30×20) and converts sub's
// bounds to root, which is not flipped. With isFlipped honoured, sub sits
// 10 pt below the top of flipped, so in root it spans y = 70…90. Without
// it, sub would be 10 pt above the bottom (y = 10…30).
func checkFlipped() error {
	plain := alloc("NSView", "initWithFrame:", rect(0, 0, 10, 10))
	defer release(plain)
	if objc.Send[bool](plain, sel("isFlipped")) {
		return fmt.Errorf("a plain NSView reports isFlipped = YES")
	}

	root := alloc("NSView", "initWithFrame:", rect(0, 0, 200, 100))
	defer release(root)
	flipped := alloc(flippedClass, "initWithFrame:", rect(0, 0, 200, 100))
	defer release(flipped)
	sub := alloc("NSView", "initWithFrame:", rect(10, 10, 30, 20))
	defer release(sub)
	root.Send(sel("addSubview:"), flipped)
	flipped.Send(sel("addSubview:"), sub)

	if !objc.Send[bool](flipped, sel("isFlipped")) {
		return fmt.Errorf("[GGFlippedView isFlipped] = NO")
	}
	bounds := objc.Send[NSRect](sub, sel("bounds"))
	got := objc.Send[NSRect](sub, sel("convertRect:toView:"), bounds, root)
	want := rect(10, 70, 30, 20)
	if got != want {
		return fmt.Errorf("sub in root coordinates = %v, want %v (top edge %.2f pt from the top, want 10)",
			got, want, 100-(got.Origin.Y+got.Size.Height))
	}
	// The same through convertPoint: sub's top-left corner (0, 20 in its
	// own, unflipped coordinates) is (10, 10) in flipped.
	if p := objc.Send[NSPoint](sub, sel("convertPoint:toView:"), pt(0, 20), flipped); p != pt(10, 10) {
		return fmt.Errorf("sub's top-left corner in GGFlippedView = %v, want {10 10}", p)
	}
	return nil
}

func checkClick() error {
	target := alloc(targetClass, "init")
	defer release(target)
	p := &recorder{}
	h := peers.New(p)
	defer peers.Free(h)
	setHandle(target, h)

	button := alloc("NSButton", "initWithFrame:", rect(0, 0, 100, 32))
	defer release(button)
	button.Send(sel("setTitle:"), nsString("Click"))
	button.Send(sel("setTarget:"), target)
	button.Send(sel("setAction:"), sel("buttonClicked:"))
	if got := button.Send(sel("target")); got != target {
		return fmt.Errorf("[button target] = %#x, want %#x", got, target)
	}
	if got := goString(button.Send(sel("title"))); got != "Click" {
		return fmt.Errorf("[button title] = %q, want %q", got, "Click")
	}

	button.Send(sel("performClick:"), objc.ID(0))
	switch {
	case p.calls != 1:
		return fmt.Errorf("the action ran %d times, want 1", p.calls)
	case p.self != target:
		return fmt.Errorf("self = %#x, want the target %#x", p.self, target)
	case getHandle(p.self) != h:
		return fmt.Errorf("handle of self = %v, want %v", getHandle(p.self), h)
	case p.cmd != sel("buttonClicked:"):
		return fmt.Errorf("_cmd = %#x, want %#x (buttonClicked:)", p.cmd, sel("buttonClicked:"))
	case p.sender != button:
		return fmt.Errorf("sender = %#x, want the button %#x", p.sender, button)
	}

	// After the peer is freed, a late action is ignored, not dispatched.
	peers.Free(h)
	button.Send(sel("performClick:"), objc.ID(0))
	if p.calls != 1 {
		return fmt.Errorf("the action ran after the peer was freed")
	}
	return nil
}

func checkCallbackCount() error {
	if got, want := ffi.Callbacks(), trampolines.count; got != want {
		return fmt.Errorf("ffi.Callbacks() = %d, want %d", got, want)
	}
	return nil
}
