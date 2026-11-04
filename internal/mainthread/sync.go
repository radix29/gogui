package mainthread

import (
	"fmt"
	"runtime/debug"
	"sync/atomic"
)

// States of a DoSync call.
const (
	callPending   int32 = iota // queued, not started
	callRunning                // started on the UI goroutine
	callAbandoned              // the caller gave up because the queue closed
)

type syncCall struct {
	state atomic.Int32
	done  chan struct{} // closed when fn has returned or panicked
	err   *PanicError
}

// DoSync runs fn on the UI goroutine and waits until it has returned.
//
// On the UI goroutine itself, DoSync calls fn directly, ahead of any queued
// functions, so it never deadlocks. From any other goroutine it queues fn
// like Do and blocks until a Drain has run it. If the event loop has not
// started yet, it blocks until it does.
//
// If fn panics, the panic is recovered on the UI goroutine (which keeps
// running) and re-raised in the caller as a *PanicError.
//
// DoSync reports whether fn ran. It returns false, without running fn, if
// the queue is closed before fn starts.
func (q *Queue) DoSync(fn func()) bool {
	if fn == nil {
		panic("mainthread: DoSync called with a nil func")
	}
	if q.OnUIThread() {
		fn()
		return true
	}
	if q.closed.Load() {
		return false
	}
	c := &syncCall{done: make(chan struct{})}
	q.push(func() { c.run(fn) })
	q.requestWake()

	select {
	case <-c.done:
	case <-q.closedCh:
		if c.state.CompareAndSwap(callPending, callAbandoned) {
			return false
		}
		<-c.done // fn is running; wait for it
	}
	if c.err != nil {
		panic(c.err)
	}
	return true
}

func (c *syncCall) run(fn func()) {
	if !c.state.CompareAndSwap(callPending, callRunning) {
		return // the caller has given up
	}
	defer close(c.done)
	defer func() {
		if r := recover(); r != nil {
			c.err = &PanicError{Value: r, Stack: debug.Stack()}
		}
	}()
	fn()
}

// PanicError is the panic value DoSync re-raises in its caller when the
// function panicked on the UI goroutine.
type PanicError struct {
	// Value is the original panic value.
	Value any
	// Stack is the stack trace of the UI goroutine at the time of the panic.
	Stack []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("panic on the UI thread in DoSync: %v\n\n%s", e.Value, e.Stack)
}

// Unwrap returns the original panic value if it is an error.
func (e *PanicError) Unwrap() error {
	if err, ok := e.Value.(error); ok {
		return err
	}
	return nil
}
