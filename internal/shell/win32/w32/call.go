//go:build windows

package w32

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// callV calls p with value arguments only (handles, integers, flags; no
// pointers) and returns its result and the last error. LazyProc.Call is
// //go:uintptrescapes, so its arguments go to the heap on every call, and
// it returns the Errno as an error, which allocates too: the message
// loop's calls (GetWindowLongPtr, DefWindowProc, IsWindowVisible…) did
// both on every message (review P17). syscall.SyscallN keeps the argument
// slice on the stack. A call that passes a pointer stays on Call: the
// native side may call back into Go, which can move the goroutine's stack
// and a value on it.
func callV(p *windows.LazyProc, args ...uintptr) (r1, r2 uintptr, err windows.Errno) {
	r1, r2, e := syscall.SyscallN(p.Addr(), args...)
	return r1, r2, windows.Errno(e)
}
