package ffi

import (
	"syscall"
	"testing"
)

var (
	procGetProcessHeap = kernel32.NewProc("GetProcessHeap")
	procHeapAlloc      = kernel32.NewProc("HeapAlloc")
	procHeapFree       = kernel32.NewProc("HeapFree")
)

// heapAlloc returns n zeroed bytes of native memory from the process heap
// (malloc's Windows counterpart; at least 8-byte aligned) and a func that
// frees them.
func heapAlloc(t *testing.T, n uintptr) (uintptr, func()) {
	t.Helper()
	const heapZeroMemory = 0x8
	heap, _, err := procGetProcessHeap.Call()
	if heap == 0 {
		t.Fatalf("GetProcessHeap: %v", err)
	}
	p, _, _ := procHeapAlloc.Call(heap, heapZeroMemory, n)
	if p == 0 {
		t.Fatal("HeapAlloc failed")
	}
	return p, func() {
		if r, _, err := procHeapFree.Call(heap, 0, p); r == 0 {
			t.Errorf("HeapFree: %v", err.(syscall.Errno))
		}
	}
}

func TestAtHeapAlloc(t *testing.T) {
	p, free := heapAlloc(t, 16)
	defer free()
	checkAtRoundTrip(t, p)
}

func TestReadWriteHeapAlloc(t *testing.T) {
	p, free := heapAlloc(t, 16*wordSize)
	defer free()
	checkWordRoundTrip(t, p, 16)
	checkInt32RoundTrip(t, p, 16)
}

func TestWriteBytesHeapAlloc(t *testing.T) {
	p, free := heapAlloc(t, 32)
	defer free()
	in := []byte("native bytes, odd offset")
	WriteBytes(p+1, in)
	if got := CopyBytes(p+1, len(in)); string(got) != string(in) {
		t.Errorf("round trip = %q, want %q", got, in)
	}
	if got := CopyBytes(p, 1); got[0] != 0 {
		t.Errorf("byte before the write = %d, want 0", got[0])
	}
}
