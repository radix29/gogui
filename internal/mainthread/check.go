package mainthread

import (
	"bytes"
	"runtime"
	"strconv"
)

// SetChecks turns UI-thread checks on or off. They are off by default.
// Package gogui turns them on in debug mode (PLAN.md §4.2: using a widget
// off the UI thread panics in debug builds) and in its own tests.
func (q *Queue) SetChecks(on bool) { q.checks.Store(on) }

// Checks reports whether UI-thread checks are on.
func (q *Queue) Checks() bool { return q.checks.Load() }

// Check panics if checks are on and the caller is not on the UI goroutine
// (or the queue is not bound). what names the operation, for example
// "Button.SetTitle". With checks off, Check costs one atomic load.
func (q *Queue) Check(what string) {
	if q.checks.Load() && !q.OnUIThread() {
		panic("gogui: " + what + " called off the UI thread; use gogui.Do or gogui.DoSync")
	}
}

var goroutinePrefix = []byte("goroutine ")

// goid returns the ID of the calling goroutine.
//
// Go has no public API for this, so it is parsed from the first line of the
// goroutine's stack trace ("goroutine 123 [running]:"). runtime.Stack with
// all == false does not stop the world. The format has been stable since
// Go 1.0; TestGoid fails if it ever changes.
func goid() uint64 {
	var buf [64]byte
	b := buf[:runtime.Stack(buf[:], false)]
	b = bytes.TrimPrefix(b, goroutinePrefix)
	if i := bytes.IndexByte(b, ' '); i > 0 {
		b = b[:i]
	}
	id, err := strconv.ParseUint(string(b), 10, 64)
	if err != nil || id == 0 {
		panic("mainthread: cannot parse the goroutine ID from runtime.Stack")
	}
	return id
}
