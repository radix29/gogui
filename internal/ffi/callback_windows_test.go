package ffi

import (
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
)

// The smoke test's callbacks are registered once per process: -count=N runs
// the test N times, and a second registration would (correctly) panic.
var (
	smokeOnce                 sync.Once
	smokeWndproc, smokeNarrow uintptr
	smokeGrew                 int
	smokeSeen                 [4]atomic.Uintptr
)

func smokeCallbacks() {
	smokeOnce.Do(func() {
		before := Callbacks()
		smokeWndproc = RegisterCallback("test.wndproc", func(hwnd, msg, wp, lp uintptr) uintptr {
			for i, v := range [4]uintptr{hwnd, msg, wp, lp} {
				smokeSeen[i].Store(v)
			}
			return hwnd + msg + wp + lp
		})
		// Narrow integer arguments arrive in the low bits of their word.
		smokeNarrow = RegisterCallback("test.narrow", func(a int32, b uint16, c bool) uintptr {
			if !c {
				return 0
			}
			return uintptr(int64(a) + int64(b))
		})
		smokeGrew = Callbacks() - before
	})
}

// TestCallbackWindowsSmoke registers real callbacks (two of the process's
// budget) and calls them through syscall.SyscallN, the way native code calls
// a window procedure.
func TestCallbackWindowsSmoke(t *testing.T) {
	smokeCallbacks()
	if smokeWndproc == 0 || smokeNarrow == 0 {
		t.Fatal("RegisterCallback returned 0")
	}
	r, _, _ := syscall.SyscallN(smokeWndproc, 1, 0x10, 100, 1000)
	if r != 1117 {
		t.Fatalf("wndproc returned %d, want 1117", r)
	}
	var seen [4]uintptr
	for i := range smokeSeen {
		seen[i] = smokeSeen[i].Load()
	}
	if seen != [4]uintptr{1, 0x10, 100, 1000} {
		t.Fatalf("wndproc saw %v", seen)
	}
	if r, _, _ := syscall.SyscallN(smokeNarrow, uintptr(0xFFFFFFFF), 7, 1); r != 6 {
		t.Fatalf("narrow returned %d, want 6 (-1 + 7)", r)
	}
	if r, _, _ := syscall.SyscallN(smokeNarrow, 5, 7, 0); r != 0 {
		t.Fatalf("narrow with false returned %d, want 0", r)
	}

	if smokeGrew != 2 {
		t.Fatalf("registering grew Callbacks() by %d, want 2", smokeGrew)
	}
	before := Callbacks()
	mustPanic(t, "registered twice", func() {
		RegisterCallback("test.wndproc", func(uintptr) uintptr { return 0 })
	})
	if n := Callbacks(); n != before {
		t.Fatalf("Callbacks() after a duplicate = %d, want %d", n, before)
	}
}
