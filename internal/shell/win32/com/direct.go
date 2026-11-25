//go:build windows

package com

import (
	"math"
	"runtime"
	"syscall"
)

// The direct call path (review P2). com.Ptr.Call is //go:uintptrescapes:
// every Go pointer passed through it escapes to the heap, so a per-draw
// call that passes a rect or a matrix by pointer allocates, and the float
// path (Bind, purego's reflect) allocates on every call. The per-draw
// methods of the binding packages call syscall.SyscallN directly instead,
// with the pointer conversions in the call expression: SyscallN is
// //go:uintptrkeepalive, so the pointed-to values stay alive and on the
// stack. That is safe only for methods that never call back into Go (a
// callback could grow the goroutine's stack and move them); the drawing,
// geometry and text-layout methods don't (no Go-implemented COM object
// takes part in them).
//
// On windows/amd64 a FLOAT argument goes in the integer slot of its
// position too: the runtime's stdcall loads the first four arguments
// into both RCX/RDX/R8/R9 and XMM0–3, and a FLOAT beyond them is the low
// half of its 8-byte stack slot; a small struct by value (D2D1_POINT_2F)
// is one integer. A FLOAT result is XMM0, which SyscallN returns as r2.
// windows/arm64 keeps floats in s0–s7, apart from the integer registers,
// so it stays on Bind.

// DirectFloats reports whether float arguments and results go through
// syscall.SyscallN's integer slots (windows/amd64).
const DirectFloats = runtime.GOARCH == "amd64"

// Float returns f as the integer slot that carries it on the direct path.
func Float(f float32) uintptr { return uintptr(math.Float32bits(f)) }

// Float2 returns a struct of two FLOATs (D2D1_POINT_2F, D2D1_SIZE_F) as
// the one integer that carries it by value on the direct path.
func Float2(x, y float32) uintptr {
	return uintptr(math.Float32bits(x)) | uintptr(math.Float32bits(y))<<32
}

// FloatResult returns the FLOAT result of a direct call from its r2.
func FloatResult(r2 uintptr) float32 { return math.Float32frombits(uint32(r2)) }

// CallFloat is Call for a method with a FLOAT result on the direct path
// (DirectFloats): the result comes back in XMM0, which SyscallN returns as
// r2. Elsewhere the method goes through Bind.
//
//go:uintptrescapes
func (p Ptr) CallFloat(i int, args ...uintptr) float32 {
	fn := p.Slot(i)
	var r2 uintptr
	switch len(args) {
	case 0:
		_, r2, _ = syscall.SyscallN(fn, uintptr(p))
	default:
		all := make([]uintptr, 0, 1+len(args))
		all = append(all, uintptr(p))
		all = append(all, args...)
		_, r2, _ = syscall.SyscallN(fn, all...)
	}
	return FloatResult(r2)
}
