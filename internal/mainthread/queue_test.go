package mainthread

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// loop is a minimal "native event loop" for tests: a goroutine bound as the
// UI goroutine that drains the queue whenever the wake function fires, the
// way the headless driver will.
type loop struct {
	q    *Queue
	wake chan struct{}
	stop chan struct{}
	done chan struct{}
}

func startLoop(t *testing.T, q *Queue) *loop {
	t.Helper()
	l := &loop{q: q, wake: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{})}
	ready := make(chan struct{})
	go func() {
		defer close(l.done)
		q.Bind()
		q.SetWake(func() {
			select {
			case l.wake <- struct{}{}:
			default:
			}
		})
		close(ready)
		for {
			select {
			case <-l.wake:
				q.Drain()
			case <-l.stop:
				q.Drain()
				return
			}
		}
	}()
	<-ready
	t.Cleanup(l.shutdown)
	return l
}

func (l *loop) shutdown() {
	select {
	case <-l.stop:
	default:
		close(l.stop)
	}
	<-l.done
}

// within fails the test if fn does not return within d (a deadlock or a
// lost wake-up).
func within(t *testing.T, d time.Duration, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("did not finish within %v (deadlock or lost wake-up)", d)
	}
}

const timeout = 10 * time.Second

// countingWake returns a wake function that only counts its calls.
func countingWake() (func(), *atomic.Int32) {
	var n atomic.Int32
	return func() { n.Add(1) }, &n
}

func TestDoRunsInOrderOnUIThread(t *testing.T) {
	q := New()
	startLoop(t, q)

	var got []int
	offThread := 0
	for i := range 1000 {
		q.Do(func() {
			if !q.OnUIThread() {
				offThread++
			}
			got = append(got, i)
		})
	}
	within(t, timeout, func() { q.DoSync(func() {}) })

	if offThread != 0 {
		t.Errorf("%d functions ran off the UI thread", offThread)
	}
	if len(got) != 1000 {
		t.Fatalf("ran %d functions, want 1000", len(got))
	}
	for i, v := range got {
		if v != i {
			t.Fatalf("got[%d] = %d: out of order", i, v)
		}
	}
}

func TestDoManyProducers(t *testing.T) {
	q := New()
	startLoop(t, q)

	const producers, perProducer = 8, 20000
	last := make([]int, producers) // touched only on the UI goroutine
	for i := range last {
		last[i] = -1
	}
	var bad atomic.Int32
	total := 0

	var wg sync.WaitGroup
	for p := range producers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range perProducer {
				q.Do(func() {
					if last[p] != i-1 {
						bad.Add(1) // per-producer FIFO violated
					}
					last[p] = i
					total++
				})
			}
		}()
	}
	within(t, timeout, func() {
		wg.Wait()
		q.DoSync(func() {}) // queued after every Do above
	})

	if bad.Load() != 0 {
		t.Errorf("%d per-producer ordering violations", bad.Load())
	}
	if total != producers*perProducer {
		t.Errorf("ran %d functions, want %d", total, producers*perProducer)
	}
}

func TestNoLostWakeups(t *testing.T) {
	q := New()
	startLoop(t, q)

	// Many goroutines each wait for their own function in tight rounds. A
	// lost wake-up leaves a goroutine blocked and trips the timeout.
	const goroutines, rounds = 16, 2000
	var ran atomic.Int64
	within(t, timeout, func() {
		var wg sync.WaitGroup
		for range goroutines {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for r := range rounds {
					if r%2 == 0 {
						q.DoSync(func() { ran.Add(1) })
						continue
					}
					ch := make(chan struct{})
					q.Do(func() { ran.Add(1); close(ch) })
					<-ch
				}
			}()
		}
		wg.Wait()
	})
	if got := ran.Load(); got != goroutines*rounds {
		t.Errorf("ran %d functions, want %d", got, goroutines*rounds)
	}
}

func TestWakeIsCoalesced(t *testing.T) {
	q := New()
	wake, n := countingWake()
	q.SetWake(wake)
	q.Bind()

	for range 100 {
		q.Do(func() {})
	}
	if n.Load() != 1 {
		t.Fatalf("100 queued functions caused %d wake-ups, want 1", n.Load())
	}
	if got := q.Drain(); got != 100 {
		t.Fatalf("Drain ran %d, want 100", got)
	}
	if got := q.Drain(); got != 0 {
		t.Fatalf("empty Drain ran %d", got)
	}
	q.Do(func() {})
	if n.Load() != 2 {
		t.Errorf("first Do after Drain caused %d wake-ups in total, want 2", n.Load())
	}
}

func TestSetWakeWithQueuedWork(t *testing.T) {
	q := New()
	q.Do(func() {}) // queued before the event loop exists

	wake, n := countingWake()
	q.SetWake(wake)
	if n.Load() != 1 {
		t.Fatalf("SetWake with pending work woke %d times, want 1", n.Load())
	}

	q2 := New()
	q2.SetWake(wake)
	if n.Load() != 1 {
		t.Error("SetWake on an empty queue must not wake")
	}

	q.SetWake(nil) // removing the wake function is allowed
	q.Bind()
	if q.Drain() != 1 {
		t.Error("queued work was lost")
	}
}

func TestDrainRunsOnlyASnapshot(t *testing.T) {
	q := New()
	wake, n := countingWake()
	q.SetWake(wake)
	q.Bind()

	// A function that re-queues itself must not starve the event loop.
	runs := 0
	var again func()
	again = func() {
		runs++
		if runs < 5 {
			q.Do(again)
		}
	}
	q.Do(again)
	for i := 1; i <= 5; i++ {
		if got := q.Drain(); got != 1 {
			t.Fatalf("Drain #%d ran %d functions, want 1", i, got)
		}
	}
	if runs != 5 || n.Load() != 5 {
		t.Errorf("runs=%d wakes=%d, want 5 and 5", runs, n.Load())
	}
}

func TestNestedDrain(t *testing.T) {
	q := New()
	q.Bind()
	var got []string
	q.Do(func() {
		got = append(got, "A start")
		q.Drain() // e.g. a nested modal loop
		got = append(got, "A end")
	})
	q.Do(func() { got = append(got, "B") })
	q.Do(func() { got = append(got, "C") })

	if n := q.Drain(); n != 1 {
		t.Errorf("outer Drain ran %d functions itself, want 1", n)
	}
	if want := "A start,B,C,A end"; strings.Join(got, ",") != want {
		t.Errorf("order = %v, want %v", got, want)
	}
	if q.Drain() != 0 {
		t.Error("functions ran twice")
	}
}

func TestDrainPanicKeepsRemainingWork(t *testing.T) {
	q := New()
	wake, n := countingWake()
	q.SetWake(wake)
	q.Bind()

	ran := false
	q.Do(func() { panic("boom") })
	q.Do(func() { ran = true })
	wakesBefore := n.Load()

	func() {
		defer func() {
			if r := recover(); r != "boom" {
				t.Errorf("recovered %v, want boom", r)
			}
		}()
		q.Drain()
	}()
	if n.Load() != wakesBefore+1 {
		t.Error("a panicking Drain must request a wake-up for the remaining work")
	}
	if q.Drain() != 1 || !ran {
		t.Error("the function after the panic did not run")
	}
}

func TestDoSyncOnUIThreadRunsDirectly(t *testing.T) {
	q := New()
	q.Bind()

	var got []string
	q.Do(func() { got = append(got, "queued") })
	if !q.DoSync(func() { got = append(got, "sync") }) {
		t.Fatal("DoSync on the UI thread reported false")
	}
	q.Drain()
	if strings.Join(got, ",") != "sync,queued" {
		t.Errorf("order = %v: DoSync on the UI thread must not wait for the queue", got)
	}
}

func TestDoSyncFromInsideQueuedFunc(t *testing.T) {
	q := New()
	startLoop(t, q)
	within(t, timeout, func() {
		q.DoSync(func() {
			// Already on the UI thread: must not deadlock.
			q.DoSync(func() {})
		})
	})
}

func TestDoSyncWaitsForResult(t *testing.T) {
	q := New()
	startLoop(t, q)
	x := 0
	within(t, timeout, func() {
		if !q.DoSync(func() { time.Sleep(10 * time.Millisecond); x = 42 }) {
			t.Error("DoSync reported false")
		}
	})
	if x != 42 {
		t.Errorf("DoSync returned before fn finished: x = %d", x)
	}
}

func TestDoSyncBeforeLoopStarts(t *testing.T) {
	q := New()
	result := make(chan bool, 1)
	go func() { result <- q.DoSync(func() {}) }()

	time.Sleep(20 * time.Millisecond) // let DoSync queue and block
	select {
	case <-result:
		t.Fatal("DoSync returned before any Drain")
	default:
	}
	startLoop(t, q) // SetWake sees the pending work and wakes the loop
	select {
	case ok := <-result:
		if !ok {
			t.Error("DoSync reported false")
		}
	case <-time.After(timeout):
		t.Fatal("DoSync never ran after the loop started")
	}
}

func TestDoSyncPanic(t *testing.T) {
	q := New()
	startLoop(t, q)

	sentinel := errors.New("sentinel")
	for _, tc := range []struct {
		value   any
		wantErr bool
	}{{"boom", false}, {fmt.Errorf("wrapped: %w", sentinel), true}} {
		var pe *PanicError
		within(t, timeout, func() {
			defer func() {
				r := recover()
				var ok bool
				if pe, ok = r.(*PanicError); !ok {
					t.Errorf("recovered %T %v, want *PanicError", r, r)
				}
			}()
			q.DoSync(func() { panic(tc.value) })
		})
		if pe == nil {
			continue
		}
		if pe.Value != tc.value || len(pe.Stack) == 0 || !strings.Contains(pe.Error(), "UI thread") {
			t.Errorf("PanicError = %+v", pe)
		}
		if got := errors.Is(pe, sentinel); got != tc.wantErr {
			t.Errorf("errors.Is(sentinel) = %v, want %v", got, tc.wantErr)
		}
	}

	// The UI goroutine survived the panics.
	within(t, timeout, func() {
		if !q.DoSync(func() {}) {
			t.Error("queue dead after a DoSync panic")
		}
	})
}

func TestClose(t *testing.T) {
	q := New()
	ranDo := false
	q.Do(func() { ranDo = true })

	// A DoSync that is still queued gives up when the queue closes.
	result := make(chan bool, 1)
	ranSync := atomic.Bool{}
	go func() { result <- q.DoSync(func() { ranSync.Store(true) }) }()
	time.Sleep(20 * time.Millisecond)

	q.Close()
	q.Close() // idempotent
	select {
	case ok := <-result:
		if ok {
			t.Error("abandoned DoSync reported true")
		}
	case <-time.After(timeout):
		t.Fatal("DoSync did not return after Close")
	}

	if q.Do(func() {}) {
		t.Error("Do after Close reported true")
	}
	if q.DoSync(func() {}) {
		t.Error("DoSync off the UI thread after Close reported true")
	}
	if !q.Closed() {
		t.Error("Closed() = false")
	}

	// Drain still runs queued Do functions, but not the abandoned DoSync.
	q.Bind()
	q.Drain()
	if !ranDo || ranSync.Load() {
		t.Errorf("after Close: ranDo=%v ranSync=%v, want true and false", ranDo, ranSync.Load())
	}
}

func TestCloseWaitsForRunningDoSync(t *testing.T) {
	q := New()
	startLoop(t, q)

	started, release := make(chan struct{}), make(chan struct{})
	result := make(chan bool, 1)
	go func() {
		result <- q.DoSync(func() { close(started); <-release })
	}()
	<-started
	q.Close()
	select {
	case <-result:
		t.Fatal("DoSync returned while its function was still running")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case ok := <-result:
		if !ok {
			t.Error("DoSync whose function ran reported false")
		}
	case <-time.After(timeout):
		t.Fatal("DoSync did not return")
	}
}

func TestOnUIThread(t *testing.T) {
	q := New()
	if q.Bound() || q.OnUIThread() {
		t.Fatal("unbound queue claims a UI thread")
	}
	bound := make(chan struct{})
	check := make(chan bool)
	go func() {
		q.Bind()
		close(bound)
		check <- q.OnUIThread()
	}()
	<-bound
	if !q.Bound() || q.OnUIThread() {
		t.Error("OnUIThread must be false off the bound goroutine")
	}
	if !<-check {
		t.Error("OnUIThread must be true on the bound goroutine")
	}

	q.Bind() // rebinding moves the UI thread
	if !q.OnUIThread() {
		t.Error("Bind did not rebind")
	}
}

func TestCheck(t *testing.T) {
	q := New()
	panics := func(fn func()) (msg string) {
		defer func() {
			if r := recover(); r != nil {
				msg = fmt.Sprint(r)
			}
		}()
		fn()
		return ""
	}

	// Off by default: no panic anywhere.
	if q.Checks() || panics(func() { q.Check("Button.SetTitle") }) != "" {
		t.Fatal("checks must be off by default")
	}

	q.SetChecks(true)
	if msg := panics(func() { q.Check("Button.SetTitle") }); !strings.Contains(msg, "Button.SetTitle") {
		t.Errorf("unbound check: %q", msg)
	}
	q.Bind()
	if msg := panics(func() { q.Check("Button.SetTitle") }); msg != "" {
		t.Errorf("check on the UI thread panicked: %q", msg)
	}
	off := make(chan string)
	go func() { off <- panics(func() { q.Check("Button.SetTitle") }) }()
	if msg := <-off; !strings.Contains(msg, "off the UI thread") {
		t.Errorf("off-thread check: %q", msg)
	}
	// Drain checks too.
	go func() { off <- panics(func() { q.Drain() }) }()
	if msg := <-off; msg == "" {
		t.Error("Drain off the UI thread did not panic with checks on")
	}
}

func TestGoid(t *testing.T) {
	a, b := goid(), goid()
	if a == 0 || a != b {
		t.Fatalf("goid unstable: %d, %d", a, b)
	}
	other := make(chan uint64)
	go func() { other <- goid() }()
	if o := <-other; o == a || o == 0 {
		t.Errorf("goid of another goroutine = %d, ours = %d", o, a)
	}
}

func TestNilFuncPanics(t *testing.T) {
	q := New()
	for name, fn := range map[string]func(){
		"Do":     func() { q.Do(nil) },
		"DoSync": func() { q.DoSync(nil) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s(nil) did not panic", name)
				}
			}()
			fn()
		}()
	}
}

func BenchmarkDo(b *testing.B) {
	q := New()
	q.Bind()
	fn := func() {}
	for b.Loop() {
		q.Do(fn)
		q.Drain()
	}
}

func BenchmarkDoParallel(b *testing.B) {
	q := New()
	var wake atomic.Int32
	q.SetWake(func() { wake.Add(1) })
	fn := func() {}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			q.Do(fn)
		}
	})
	b.StopTimer()
	q.Bind()
	q.Drain()
}

func BenchmarkOnUIThread(b *testing.B) {
	q := New()
	q.Bind()
	for b.Loop() {
		q.OnUIThread()
	}
}

// TestDoSyncPanicOnUIThread is A7: on the UI goroutine DoSync re-raises a
// panic as a *PanicError too, and a nested DoSync's *PanicError is not
// wrapped again.
func TestDoSyncPanicOnUIThread(t *testing.T) {
	q := New()
	startLoop(t, q)
	var got, nested any
	within(t, timeout, func() {
		q.DoSync(func() {
			func() {
				defer func() { got = recover() }()
				q.DoSync(func() { panic("boom") })
			}()
			func() {
				defer func() { nested = recover() }()
				q.DoSync(func() { q.DoSync(func() { panic("deep") }) })
			}()
		})
	})
	if pe, ok := got.(*PanicError); !ok || pe.Value != "boom" || len(pe.Stack) == 0 {
		t.Errorf("on the UI thread: recovered %T %v, want *PanicError{boom}", got, got)
	}
	if pe, ok := nested.(*PanicError); !ok || pe.Value != "deep" {
		t.Errorf("nested: recovered %T %v, want *PanicError{deep}", nested, nested)
	}
}
