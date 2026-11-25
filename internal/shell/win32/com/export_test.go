//go:build windows

package com

import (
	"fmt"
	"strconv"
	"unsafe"
)

// Test-only bindings (review §4): nothing outside this package's tests
// calls them.

// CreateInstance creates an object of the in-process class clsid
// (CoCreateInstance with CLSCTX_INPROC_SERVER) and returns its interface
// iid with one reference. The thread must be a COM apartment
// (InitUIThread); otherwise it fails with CO_E_NOTINITIALIZED.
func CreateInstance(clsid, iid *GUID) (Ptr, error) {
	var out Ptr
	r, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(clsid)), 0, CLSCTX_INPROC_SERVER,
		uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	if err := HRESULT(int32(r)).Err("CoCreateInstance(" + clsid.String() + ")"); err != nil {
		return 0, err
	}
	return out, nil
}

// BindFunc sets *fptr to a func that calls the C function at addr through
// the same path as Bind, with the same rules except the one for this. It is
// for flat exports that take or return floats; the package's tests use it
// to check the float path against the C runtime.
func BindFunc(fptr any, addr uintptr) error {
	op := "bind function 0x" + strconv.FormatUint(uint64(addr), 16)
	if addr == 0 {
		return fmt.Errorf("com: bind function: NULL address")
	}
	fv, err := funcVar(op, fptr)
	if err != nil {
		return err
	}
	return bindAddr(op, fv, addr)
}
