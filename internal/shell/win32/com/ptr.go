//go:build windows

package com

import (
	"syscall"
	"unsafe"

	"github.com/radix29/gogui/internal/ffi"
)

// Ptr is a COM interface pointer: the address of a native object whose
// first word points to its vtable. A non-zero Ptr held by Go code owns one
// reference. The zero Ptr is nil.
//
// Slots are indices into the vtable in declaration order, inherited
// interfaces first: 0–2 are IUnknown's QueryInterface, AddRef and Release.
type Ptr uintptr

// IUnknown slots.
const (
	slotQueryInterface = 0
	slotAddRef         = 1
	slotRelease        = 2
)

// IsNil reports whether p is the nil interface pointer.
func (p Ptr) IsNil() bool { return p == 0 }

// Slot returns the code address of vtable slot i. It panics if p is nil.
func (p Ptr) Slot(i int) uintptr {
	if p == 0 {
		panic("com: method call on a nil interface pointer")
	}
	if i < 0 {
		panic("com: negative vtable slot")
	}
	vtbl := ffi.ReadUintptr(uintptr(p), 0)
	return ffi.ReadUintptr(vtbl, uintptr(i)*unsafe.Sizeof(uintptr(0)))
}

// Call calls the integer-only method in vtable slot i with p as this and
// returns its integer result. Every argument must be an integer, a handle,
// a native address, or a Go pointer converted to uintptr in the call
// expression itself (it stays alive and in place for the call). Methods
// with float arguments or a float result go through Bind.
//
//go:uintptrescapes
func (p Ptr) Call(i int, args ...uintptr) uintptr {
	fn := p.Slot(i)
	var r uintptr
	switch len(args) {
	case 0:
		r, _, _ = syscall.SyscallN(fn, uintptr(p))
	case 1:
		r, _, _ = syscall.SyscallN(fn, uintptr(p), args[0])
	case 2:
		r, _, _ = syscall.SyscallN(fn, uintptr(p), args[0], args[1])
	case 3:
		r, _, _ = syscall.SyscallN(fn, uintptr(p), args[0], args[1], args[2])
	default:
		all := make([]uintptr, 0, 1+len(args))
		all = append(all, uintptr(p))
		all = append(all, args...)
		r, _, _ = syscall.SyscallN(fn, all...)
	}
	return r
}

// CallHR is Call for a method that returns an HRESULT: nil on success, else
// an *Error naming op.
//
//go:uintptrescapes
func (p Ptr) CallHR(op string, i int, args ...uintptr) error {
	return HRESULT(int32(p.Call(i, args...))).Err(op)
}

// AddRef adds a reference and returns the new count. The count is meant for
// tests and diagnostics only (as the SDK says); the caller of AddRef owns
// one more Release.
func (p Ptr) AddRef() uint32 {
	return uint32(p.Call(slotAddRef))
}

// Release drops the reference *p owns, sets *p to nil and returns the count
// left. Releasing a nil Ptr does nothing and returns 0, so Release can be
// deferred on a Ptr that may never have been set.
func (p *Ptr) Release() uint32 {
	if *p == 0 {
		return 0
	}
	n := uint32(p.Call(slotRelease))
	*p = 0
	return n
}

// RefCount returns the object's current reference count, through an
// AddRef/Release pair. It is for tests that check that every reference is
// given back.
func (p Ptr) RefCount() uint32 {
	p.AddRef()
	q := p
	return q.Release()
}

// QueryInterface asks the object for the interface iid and returns it with
// a reference of its own. An interface the object doesn't implement returns
// an error wrapping E_NOINTERFACE.
func (p Ptr) QueryInterface(iid *GUID) (Ptr, error) {
	return p.CallOut("QueryInterface("+iid.String()+")", slotQueryInterface, uintptr(unsafe.Pointer(iid)))
}

// CallOut calls the method in vtable slot i with args followed by the
// address of an interface pointer the method fills (the creation pattern
// CreateX(…, &out)), and returns that pointer: with the reference the
// method gave it on success, nil with an *Error naming op otherwise (review
// R9). args follow Call's rules; at most 4 of them (more panics).
//
//go:uintptrescapes
func (p Ptr) CallOut(op string, i int, args ...uintptr) (Ptr, error) {
	out, r := p.callOut(i, args)
	if err := HRESULT(int32(r)).Err(op); err != nil {
		return 0, err
	}
	return out, nil
}

// GetOut is CallOut for a getter without an HRESULT (GetTarget,
// GetImmediateContext, …): it returns the interface pointer the method
// filled, nil when it filled none.
//
//go:uintptrescapes
func (p Ptr) GetOut(i int, args ...uintptr) Ptr {
	out, _ := p.callOut(i, args)
	return out
}

// callOut calls slot i with args and &out. The conversion of &out has to
// be in the call expression of Call (//go:uintptrescapes): that moves out
// to the heap, where it stays put while native code writes it, and tells
// the compiler the call may write it. Built any other way (a uintptr kept
// in a slice), the compiler may keep out on the stack and read it back as
// the zero it was initialized to.
func (p Ptr) callOut(i int, args []uintptr) (out Ptr, r uintptr) {
	switch len(args) {
	case 0:
		r = p.Call(i, uintptr(unsafe.Pointer(&out)))
	case 1:
		r = p.Call(i, args[0], uintptr(unsafe.Pointer(&out)))
	case 2:
		r = p.Call(i, args[0], args[1], uintptr(unsafe.Pointer(&out)))
	case 3:
		r = p.Call(i, args[0], args[1], args[2], uintptr(unsafe.Pointer(&out)))
	case 4:
		r = p.Call(i, args[0], args[1], args[2], args[3], uintptr(unsafe.Pointer(&out)))
	default:
		panic("com: CallOut with more than 4 arguments before the out pointer")
	}
	return out, r
}
