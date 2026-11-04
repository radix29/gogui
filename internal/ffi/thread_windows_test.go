package ffi

import (
	"fmt"
	"syscall"
	"testing"
	"unsafe"
)

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procCreateThread       = kernel32.NewProc("CreateThread")
	procGetCurrentThreadId = kernel32.NewProc("GetCurrentThreadId")
	procGetExitCodeThread  = kernel32.NewProc("GetExitCodeThread")
)

// runOnForeignThread runs start(arg) on a thread created with CreateThread
// and returns its exit code, which is a DWORD: results must fit 32 bits.
func runOnForeignThread(start, arg uintptr) (uintptr, error) {
	h, _, err := procCreateThread.Call(0, 0, start, arg, 0, 0)
	if h == 0 {
		return 0, fmt.Errorf("CreateThread: %w", err)
	}
	defer syscall.CloseHandle(syscall.Handle(h))
	ev, err := syscall.WaitForSingleObject(syscall.Handle(h), syscall.INFINITE)
	if ev != syscall.WAIT_OBJECT_0 {
		return 0, fmt.Errorf("WaitForSingleObject = %#x: %v", ev, err)
	}
	var code uint32
	if r, _, err := procGetExitCodeThread.Call(h, uintptr(unsafe.Pointer(&code))); r == 0 {
		return 0, fmt.Errorf("GetExitCodeThread: %w", err)
	}
	return uintptr(code), nil
}

func currentThreadID() uint64 {
	id, _, _ := procGetCurrentThreadId.Call()
	return uint64(id)
}

// newCaller calls the callback at addr the way a window procedure is
// called: straight through syscall.SyscallN.
func newCaller(addr uintptr) func(uintptr) uintptr {
	return func(x uintptr) uintptr {
		r, _, _ := syscall.SyscallN(addr, x)
		return r
	}
}

// benchmarkCallbackExtra has nothing to add on Windows: "direct" already is
// the window-procedure path.
func benchmarkCallbackExtra(*testing.B) {}
