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
	var out Ptr
	err := p.CallHR("QueryInterface("+iid.String()+")", slotQueryInterface,
		uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	if err != nil {
		return 0, err
	}
	return out, nil
}
