//go:build darwin || linux

package ffi

import (
	"fmt"
	"sync"
	"testing"
	"unsafe"
)

// pthreadFuncs are the libc functions the foreign-thread test needs, bound
// once per process. pthread_t is an unsigned long on linux and a pointer on
// darwin, so a uintptr on both.
var pthreadFuncs = sync.OnceValues(func() (*pthreads, error) {
	lib, err := Dlopen(libcPath)
	if err != nil {
		return nil, err
	}
	p := &pthreads{}
	for name, fp := range map[string]any{
		"pthread_create": &p.create, "pthread_join": &p.join, "pthread_self": &p.self,
		"malloc": &p.malloc, "free": &p.free,
	} {
		if err := lib.Bind(fp, name); err != nil {
			return nil, err
		}
	}
	return p, nil
})

type pthreads struct {
	create func(thread, attr, start, arg uintptr) int32
	join   func(thread, retval uintptr) int32
	self   func() uintptr
	malloc func(n uintptr) uintptr
	free   func(p uintptr)
}

func mustPthreads() *pthreads {
	p, err := pthreadFuncs()
	if err != nil {
		panic(err)
	}
	return p
}

// runOnForeignThread runs start(arg) on a thread created with
// pthread_create, joins it and returns the start routine's result. The
// out-parameters live in malloc memory, not in Go memory.
func runOnForeignThread(start, arg uintptr) (uintptr, error) {
	p, err := pthreadFuncs()
	if err != nil {
		return 0, err
	}
	const word = unsafe.Sizeof(uintptr(0))
	mem := p.malloc(2 * word)
	if mem == 0 {
		return 0, fmt.Errorf("malloc failed")
	}
	defer p.free(mem)
	if rc := p.create(mem, 0, start, arg); rc != 0 {
		return 0, fmt.Errorf("pthread_create = %d", rc)
	}
	if rc := p.join(ReadUintptr(mem, 0), mem+word); rc != 0 {
		return 0, fmt.Errorf("pthread_join = %d", rc)
	}
	return ReadUintptr(mem, word), nil
}

// currentThreadID returns pthread_self, which on both OSes identifies the
// calling thread for as long as it runs. (Gettid would do on linux, but one
// path for both keeps the test the same.)
func currentThreadID() uint64 {
	return uint64(mustPthreads().self())
}

// newCaller binds the callback at addr with BindAddr, so calling it is a
// Go→C→Go round trip through purego.
func newCaller(addr uintptr) func(uintptr) uintptr {
	var f func(uintptr) uintptr
	if err := BindAddr(&f, addr); err != nil {
		panic(err)
	}
	return f
}

// benchmarkCallbackExtra adds libc qsort calling the Go comparator of
// TestCallbackQsort: a C caller doing real work between callbacks. The
// number is per comparator call.
func benchmarkCallbackExtra(b *testing.B) {
	b.Run("qsort", func(b *testing.B) {
		cmp := qsortCallback()
		p := mustPthreads()
		lib, err := Dlopen(libcPath)
		if err != nil {
			b.Fatal(err)
		}
		defer lib.Close()
		var qsort func(base, n, size, cmp uintptr)
		if err := lib.Bind(&qsort, "qsort"); err != nil {
			b.Fatal(err)
		}
		const n = 256
		mem := p.malloc(n * 4)
		if mem == 0 {
			b.Fatal("malloc failed")
		}
		defer p.free(mem)
		arr := unsafe.Slice((*int32)(cptr(mem)), n)
		var calls int64
		for b.Loop() {
			for i := range arr {
				arr[i] = int32((i * 7919) % n) // the same permutation each round
			}
			before := qsortCalls.Load()
			qsort(mem, n, 4, cmp)
			calls += qsortCalls.Load() - before
		}
		if calls > 0 {
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(calls), "ns/cmp")
		}
	})
}
