package ffi

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

// Callbacks on threads Go didn't create, nested callbacks and the per-call
// cost (PLAN.md §11.1 S0.10a). The per-OS pieces are in thread_unix_test.go
// (libc pthreads, BindAddr) and thread_windows_test.go (kernel32,
// syscall.SyscallN):
//
//   - runOnForeignThread(start, arg) starts a native thread running the C
//     function start(arg), waits for it and returns its result;
//   - currentThreadID() identifies the calling OS thread;
//   - newCaller(addr) returns a Go func that calls the C function
//     uintptr(*)(uintptr) at addr.
//
// All callbacks here are registered once per process (sync.Once), so
// -count=N works, and they count against Callbacks() like the smoke test's.

var (
	threadOnce  sync.Once
	threadStart uintptr
	threadRuns  atomic.Int64
	threadSeen  atomic.Uint64 // currentThreadID() inside the start routine
)

func foreignThreadCallback() uintptr {
	threadOnce.Do(func() {
		threadStart = RegisterCallback("test.threadStart", func(arg uintptr) uintptr {
			threadRuns.Add(1)
			threadSeen.Store(currentThreadID())
			return arg*2 + 1
		})
	})
	return threadStart
}

func TestCallbackForeignThread(t *testing.T) {
	start := foreignThreadCallback()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	self := currentThreadID()
	// Several runs, so both the first call on a new thread and the later
	// ones take their path (on unix purego's fakecgo binds an M per thread).
	for i := range uintptr(5) {
		before := threadRuns.Load()
		got, err := runOnForeignThread(start, 100+i)
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if threadRuns.Load() != before+1 {
			t.Fatalf("run %d: the start routine ran %d times, want once", i, threadRuns.Load()-before)
		}
		if want := (100+i)*2 + 1; got != want {
			t.Fatalf("run %d: result %d, want %d", i, got, want)
		}
		if seen := threadSeen.Load(); seen == self {
			t.Fatalf("run %d: the start routine ran on the test's own thread %#x", i, seen)
		}
	}
	t.Logf("%s/%s: 5 callbacks on foreign threads ran and returned their results",
		runtime.GOOS, runtime.GOARCH)
}

var (
	nestOnce                 sync.Once
	nestOuter, nestInner     uintptr
	callOuter, callInner     func(uintptr) uintptr
	nestMaxDepth, nestActive atomic.Int64
)

// nestedCallbacks registers two callbacks that call each other through
// native code: outer(n) = n + inner(n), inner(n) = outer(n-1), outer(0) = 0.
// So outer(n) is n(n+1)/2 after 2n C→Go transitions, each nested inside
// the previous one.
func nestedCallbacks(t testing.TB) {
	nestOnce.Do(func() {
		nestOuter = RegisterCallback("test.nestOuter", func(n uintptr) uintptr {
			d := nestActive.Add(1)
			defer nestActive.Add(-1)
			for {
				m := nestMaxDepth.Load()
				if d <= m || nestMaxDepth.CompareAndSwap(m, d) {
					break
				}
			}
			if n == 0 {
				return 0
			}
			return n + callInner(n)
		})
		nestInner = RegisterCallback("test.nestInner", func(n uintptr) uintptr {
			nestActive.Add(1)
			defer nestActive.Add(-1)
			return callOuter(n - 1)
		})
		callOuter, callInner = newCaller(nestOuter), newCaller(nestInner)
	})
	if callOuter == nil || callInner == nil {
		t.Fatal("binding the nested callbacks failed")
	}
}

func TestCallbackNested(t *testing.T) {
	nestedCallbacks(t)
	const depth = 8
	nestMaxDepth.Store(0)
	if got, want := callOuter(depth), uintptr(depth*(depth+1)/2); got != want {
		t.Fatalf("outer(%d) = %d, want %d", depth, got, want)
	}
	// outer runs depth+1 times and inner depth times, all nested; at the
	// innermost outer call 2*depth+1 callbacks are active, of which the
	// counter sees the outer ones and the inner ones alike.
	if got, want := nestMaxDepth.Load(), int64(2*depth+1); got != want {
		t.Fatalf("deepest nesting = %d active callbacks, want %d", got, want)
	}
	if n := nestActive.Load(); n != 0 {
		t.Fatalf("%d callbacks still active after the outermost returned", n)
	}
	t.Logf("%s/%s: callbacks nested %d deep through native code, result %d",
		runtime.GOOS, runtime.GOARCH, 2*depth+1, depth*(depth+1)/2)
}

var (
	benchOnce sync.Once
	benchCall func(uintptr) uintptr
)

// BenchmarkCallbackCall measures one call of a registered callback from
// native code. On Windows it is syscall.SyscallN straight into the
// callback; on unix the callback is called through a BindAddr func, so the
// number is a Go→C→Go round trip. The unix file adds a sub-benchmark with
// libc qsort calling a Go comparator. The numbers are logged, not checked.
func BenchmarkCallbackCall(b *testing.B) {
	benchOnce.Do(func() {
		p := RegisterCallback("test.bench", func(x uintptr) uintptr { return x + 1 })
		benchCall = newCaller(p)
	})
	b.Run("direct", func(b *testing.B) {
		var sum uintptr
		for i := 0; b.Loop(); i++ {
			sum = benchCall(uintptr(i))
		}
		if sum == 0 {
			b.Fatal("callback returned 0")
		}
	})
	benchmarkCallbackExtra(b)
}
