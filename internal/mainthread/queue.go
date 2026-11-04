// Package mainthread implements the UI-thread queue (PLAN.md §4.2, §7.1).
//
// gogui runs all UI code on one goroutine, the UI goroutine. Package gogui
// locks it to the main OS thread in init, and the native event loop runs on
// it. Native callbacks (window procedures, ObjC methods, GObject signals)
// re-enter Go on the goroutine that called into the native loop, so they
// run on the UI goroutine too. "UI thread" in this package therefore means
// "the goroutine passed to Bind".
//
// Other goroutines hand work to the UI goroutine with Do (asynchronous) or
// DoSync (waits for the result). The queue does not run anything by itself:
// a driver installs a wake function with SetWake ("wake the native loop":
// PostMessage on Windows, g_idle_add on Linux, performSelectorOnMainThread:
// on macOS, a channel in the headless driver), and calls Drain on the UI
// goroutine whenever it is woken.
//
// The queue is a lock-free multi-producer, single-consumer FIFO. Wake-ups are
// coalesced: however many functions are queued, the wake function is called
// at most once until the next Drain starts, so a busy goroutine cannot flood
// the native message queue.
//
// The package uses only the standard library.
package mainthread

import (
	"sync"
	"sync/atomic"
)

// node is one queued function. The queue is the non-intrusive variant of
// Dmitry Vyukov's MPSC queue: tail always points at the most recently
// consumed node (initially a stub), whose fn is already cleared.
type node struct {
	next atomic.Pointer[node]
	fn   func()
}

// Queue is a UI-thread queue. Create one with New.
//
// Do, DoSync, SetWake, Close, Closed, Bind, OnUIThread, SetChecks and Check
// are safe for concurrent use. Drain must be called only on the UI
// goroutine.
type Queue struct {
	// Producer side.
	head        atomic.Pointer[node] // most recently pushed node
	wakePending atomic.Bool          // a wake was requested and no Drain has started since
	wake        atomic.Pointer[func()]
	closed      atomic.Bool
	closeOnce   sync.Once
	closedCh    chan struct{}

	ui     atomic.Uint64 // goroutine ID of the UI goroutine; 0 = not bound
	checks atomic.Bool

	// Consumer side, touched only by Drain on the UI goroutine.
	tail     *node
	drainSeq uint64
}

// New returns an empty, unbound queue without a wake function.
func New() *Queue {
	stub := &node{}
	q := &Queue{tail: stub, closedCh: make(chan struct{})}
	q.head.Store(stub)
	return q
}

// Bind makes the calling goroutine the UI goroutine, replacing any earlier
// binding. Package gogui calls it from init, which runs on the main
// goroutine. Tests may bind another goroutine.
func (q *Queue) Bind() { q.ui.Store(goid()) }

// Bound reports whether Bind has been called.
func (q *Queue) Bound() bool { return q.ui.Load() != 0 }

// OnUIThread reports whether the caller runs on the UI goroutine. It is
// false while the queue is not bound. It parses the goroutine ID from a
// stack trace and costs a few microseconds, so hot paths should not call it
// unconditionally (see Check, which costs one atomic load while checks are
// off).
func (q *Queue) OnUIThread() bool {
	id := q.ui.Load()
	return id != 0 && id == goid()
}

// SetWake installs the function that wakes the native event loop. The
// queue calls it, from whatever goroutine queued the work, when the first
// function is queued after a Drain has started. It must not block and must
// not call Drain itself; it must cause a later Drain call on the UI
// goroutine.
//
// If work is already waiting when SetWake is called (for example functions
// queued before the event loop started), fn is called right away. A nil fn
// removes the wake function; queued work then waits for the next SetWake or
// Drain.
func (q *Queue) SetWake(fn func()) {
	if fn == nil {
		q.wake.Store(nil)
		return
	}
	q.wake.Store(&fn)
	// Pairs with the CompareAndSwap + Load in requestWake: either the
	// producer sees the new wake function, or this Load sees its flag (or
	// both, which only costs a spare wake-up).
	if q.wakePending.Load() {
		fn()
	}
}

// Do queues fn to run on the UI goroutine and returns immediately. Functions
// run in the order they were queued. Do called on the UI goroutine also
// queues fn; it runs after the current event has been handled.
//
// Do reports whether fn was queued. It returns false after Close. Functions
// queued shortly before Close may never run.
func (q *Queue) Do(fn func()) bool {
	if fn == nil {
		panic("mainthread: Do called with a nil func")
	}
	if q.closed.Load() {
		return false
	}
	q.push(fn)
	q.requestWake()
	return true
}

func (q *Queue) push(fn func()) {
	n := &node{fn: fn}
	prev := q.head.Swap(n)
	// Between the Swap and this Store the queue is briefly "broken": the
	// consumer cannot reach n yet. Drain then stops early, and the
	// requestWake that follows every push makes sure it comes back.
	prev.next.Store(n)
}

func (q *Queue) requestWake() {
	if q.wakePending.CompareAndSwap(false, true) {
		if w := q.wake.Load(); w != nil {
			(*w)()
		}
	}
}

// Drain runs queued functions on the UI goroutine and returns how many it
// ran. Drivers call it whenever the wake function has fired.
//
// Drain runs at most the functions that were queued when it started.
// Functions queued while it runs (including by the functions it runs) wait
// for the next Drain, and their wake-up is requested as usual, so a
// function that re-queues itself cannot starve the native event loop.
//
// Drain may be re-entered from a function it runs, for example when that
// function runs a nested modal event loop. The outer Drain then returns as
// soon as the function returns.
//
// If a function panics, the panic propagates out of Drain, and a wake-up is
// requested so that the remaining functions run later.
func (q *Queue) Drain() int {
	q.Check("mainthread.Queue.Drain")
	// Clear the flag before looking at the queue, so that any push we do
	// not see requests a new wake-up.
	q.wakePending.Store(false)
	q.drainSeq++
	seq := q.drainSeq
	last := q.head.Load()

	n := 0
	completed := false
	defer func() {
		if !completed {
			q.requestWake()
		}
	}()
	for q.tail != last {
		next := q.tail.next.Load()
		if next == nil {
			break // a producer is mid-push; it requests a wake-up when done
		}
		q.tail = next
		fn := next.fn
		next.fn = nil // the node stays reachable as the new stub
		n++
		fn()
		if q.drainSeq != seq {
			break // a nested Drain ran and took over
		}
	}
	completed = true
	return n
}

// Close stops the queue: later Do and DoSync calls from other goroutines
// return false without queuing, and DoSync calls that are waiting for a
// function that has not started yet return false. A function that is
// already running is waited for.
//
// Close does not discard queued functions: a driver may still call Drain,
// which runs the queued Do functions but skips DoSync functions whose
// callers have given up. Close may be called from any goroutine, and more
// than once.
func (q *Queue) Close() {
	q.closeOnce.Do(func() {
		q.closed.Store(true)
		close(q.closedCh)
	})
}

// Closed reports whether Close has been called.
func (q *Queue) Closed() bool { return q.closed.Load() }
