package ffi

import (
	"strconv"
	"unsafe"
)

// cptr turns the address of native (non-Go) memory into an unsafe.Pointer.
//
// This is the one uintptr→unsafe.Pointer conversion of the package (PLAN.md
// §7.2). It is sound because p always comes from native code (malloc, a C
// string a native API returned, an ObjC instance, a GObject class struct),
// never from a Go object, so the garbage collector neither moves nor frees
// what it points to. The double indirection keeps go vet's unsafeptr check
// quiet, which can't know where p came from.
//
// Under Go 1.26 it had to be //go:noinline (compiler crash "disjointTypes"
// when the inlined conversion fed an unsafe.Slice that a copy then wrote
// to; TestCallbackQsort has that pattern). Go 1.27 compiles it inlined.
func cptr(p uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&p))
}

// ReadUintptr returns the uintptr-sized word at addr+off.
//
// addr must be the address of native memory that stays valid for the call:
// an ObjC instance (off from ivar_getOffset), a GObject class struct (off of
// a vfunc slot), memory from malloc. It must never be the address of Go
// memory, which the garbage collector may move or free. ReadUintptr panics
// if addr is 0 or addr+off is not aligned to a uintptr; any other bad
// address crashes the process.
func ReadUintptr(addr, off uintptr) uintptr {
	checkWord("ReadUintptr", addr, off)
	return *(*uintptr)(unsafe.Add(cptr(addr), off))
}

// WriteUintptr stores v in the uintptr-sized word at addr+off. The rules
// for addr and off are those of ReadUintptr. v must not be a Go pointer
// (PLAN.md §7.2); store an ffi.Handle or a native address instead.
func WriteUintptr(addr, off, v uintptr) {
	checkWord("WriteUintptr", addr, off)
	*(*uintptr)(unsafe.Add(cptr(addr), off)) = v
}

// ReadInt32 returns the 32-bit C int at addr+off. The rules for addr are
// those of ReadUintptr, except that addr+off must be 4-byte aligned. It is
// meant for int fields and int out-parameters in native memory
// (GtkWidgetClass.measure writes ints through pointers), where a
// word-sized access would also touch the neighbouring value.
func ReadInt32(addr, off uintptr) int32 {
	checkAligned("ReadInt32", addr, off, 4, "4-byte aligned")
	return *(*int32)(unsafe.Add(cptr(addr), off))
}

// WriteInt32 stores v in the 32-bit C int at addr+off and leaves the bytes
// around it unchanged. The rules for addr and off are those of ReadInt32.
func WriteInt32(addr, off uintptr, v int32) {
	checkAligned("WriteInt32", addr, off, 4, "4-byte aligned")
	*(*int32)(unsafe.Add(cptr(addr), off)) = v
}

func checkWord(op string, addr, off uintptr) {
	checkAligned(op, addr, off, unsafe.Alignof(uintptr(0)), "uintptr-aligned")
}

// checkAligned panics if addr is 0 or addr+off is not a multiple of align.
// what describes the alignment in the message.
func checkAligned(op string, addr, off, align uintptr, what string) {
	if addr == 0 {
		panic("ffi: " + op + ": NULL address")
	}
	if (addr+off)%align != 0 {
		panic("ffi: " + op + ": address 0x" + strconv.FormatUint(uint64(addr), 16) +
			" + offset " + strconv.FormatUint(uint64(off), 10) + " is not " + what)
	}
}

// At returns p, the address of a native T, as a *T (S2.3a). It is for a
// native struct passed by address: a Win32 lParam that points at a
// MINMAXINFO, NCCALCSIZE_PARAMS, WINDOWPOS, CREATESTRUCTW or RECT, which
// the window procedure reads or writes in place. T must be a mirror whose
// layout test passes (CheckLayout, CheckNoGoPointers).
//
// p must be native memory that stays valid while the *T is used, never the
// address of Go memory, and the *T must not be kept after the call that
// handed out p returns. At panics if p is 0 or not aligned to T's
// alignment; any other bad address crashes on the first access.
func At[T any](p uintptr) *T {
	var zero T
	align := unsafe.Alignof(zero)
	checkAligned("At", p, 0, align, strconv.FormatUint(uint64(align), 10)+"-byte aligned")
	return (*T)(cptr(p))
}

// CopyBytes copies n bytes of native memory at p into a new Go slice
// (S2.4b). It is for native buffers handed out for the length of a call or
// a map/unmap pair, such as a mapped D3D11 staging texture, whose rows the
// caller copies out before the memory goes away. The rules for p are those
// of ReadUintptr, except that any alignment is fine; n 0 returns an empty
// slice without touching p. CopyBytes panics if p is 0 and n is not, or if
// n is negative.
func CopyBytes(p uintptr, n int) []byte {
	if n < 0 {
		panic("ffi: CopyBytes: negative length")
	}
	if n == 0 {
		return []byte{}
	}
	checkAligned("CopyBytes", p, 0, 1, "aligned")
	out := make([]byte, n)
	copy(out, unsafe.Slice((*byte)(cptr(p)), n))
	return out
}

// WriteBytes copies b into native memory at p (S5.2a), the counterpart of
// CopyBytes. It is for native buffers the caller fills before handing them
// to the OS, such as a locked GlobalAlloc block that becomes clipboard
// data. p must be native memory of at least len(b) bytes that stays valid
// for the call, never the address of Go memory; any alignment is fine. An
// empty b does nothing and does not touch p. WriteBytes panics if p is 0
// and b is not empty.
func WriteBytes(p uintptr, b []byte) {
	if len(b) == 0 {
		return
	}
	checkAligned("WriteBytes", p, 0, 1, "aligned")
	copy(unsafe.Slice((*byte)(cptr(p)), len(b)), b)
}
