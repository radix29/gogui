//go:build windows

package com

import (
	"fmt"
	"reflect"
	"strconv"
	"sync"

	"github.com/ebitengine/purego"
)

// The float call path. purego.RegisterFunc builds a Go func that places
// each argument by the platform's C ABI: on windows/amd64 positionally, so
// a FLOAT in argument n goes to XMMn (syscall.SyscallN loads both register
// files there), and on windows/arm64 through purego's own trampoline, with
// floats in s0–s7 and small float structs (D2D1_POINT_2F) as HFAs.

type bindKey struct {
	typ  reflect.Type
	addr uintptr
}

var (
	bindMu    sync.Mutex
	bindCache = map[bindKey]reflect.Value{}
)

// Bind sets *fptr, a pointer to a func variable, to a func that calls the
// method in vtable slot i of p. The func's first parameter is this: a Ptr
// (or another uintptr-kind type); the rest follow the method's C
// parameters, with float32 for FLOAT, a mirrored struct for a struct passed
// by value, uintptr or a Go pointer for a pointer. The result is an HRESULT
// (or another integer kind), a float type, or nothing.
//
// The bound func depends only on the func type and the slot's code
// address, so it is created once per method implementation and cached:
// binding the same method of a thousand objects of one class costs one
// RegisterFunc. Calling the bound func on another object is fine as long
// as that object's slot i holds the same code (same class). Bind is meant
// for float-taking methods on windows/arm64 (and as the fallback the tests
// compare with): on windows/amd64 the binding packages pass FLOATs as bits
// in the integer slots (DirectFloats, direct.go), which allocates nothing,
// and integer-only methods are cheaper through Call everywhere.
//
// Rejected with an error (and *fptr left unchanged): a nil p, a negative
// slot, a func type without a uintptr-kind first parameter, a Go func or a
// variadic parameter (purego would create a native callback per call), a
// struct result (MSVC returns those through a hidden pointer after this,
// which purego doesn't know; declare the out pointer instead), and any
// type purego refuses.
func Bind(fptr any, p Ptr, i int) error {
	op := "bind vtable slot " + strconv.Itoa(i)
	if p == 0 {
		return fmt.Errorf("com: %s: nil interface pointer", op)
	}
	if i < 0 {
		return fmt.Errorf("com: %s: negative slot", op)
	}
	fv, err := funcVar(op, fptr)
	if err != nil {
		return err
	}
	t := fv.Type()
	if t.NumIn() == 0 || t.In(0).Kind() != reflect.Uintptr {
		return fmt.Errorf("com: %s as %s: the first parameter must be this (a Ptr)", op, t)
	}
	if t.NumOut() == 1 && t.Out(0).Kind() == reflect.Struct {
		return fmt.Errorf("com: %s as %s: a struct result comes back through a hidden pointer "+
			"after this in MSVC's member-function ABI; declare that out pointer as a parameter", op, t)
	}
	return bindAddr(op, fv, p.Slot(i))
}

// funcVar checks fptr and the parameter rules common to Bind and BindFunc,
// and returns the func variable.
func funcVar(op string, fptr any) (reflect.Value, error) {
	v := reflect.ValueOf(fptr)
	if !v.IsValid() || v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Func {
		return reflect.Value{}, fmt.Errorf("com: %s: fptr is %T, not a non-nil pointer to a func variable", op, fptr)
	}
	t := v.Elem().Type()
	if t.IsVariadic() {
		return reflect.Value{}, fmt.Errorf("com: %s as %s: variadic funcs are not allowed "+
			"(a Go func passed in them would become a new native callback)", op, t)
	}
	for k := range t.NumIn() {
		if t.In(k).Kind() == reflect.Func {
			return reflect.Value{}, fmt.Errorf("com: %s as %s: parameter %d is a Go func, which purego "+
				"would turn into a new native callback on every call; pass a trampoline from "+
				"ffi.RegisterCallback as uintptr", op, t, k)
		}
	}
	if t.NumOut() == 1 && t.Out(0).Kind() == reflect.Func {
		return reflect.Value{}, fmt.Errorf("com: %s as %s: func results are not allowed", op, t)
	}
	return v.Elem(), nil
}

func bindAddr(op string, fv reflect.Value, addr uintptr) (err error) {
	key := bindKey{fv.Type(), addr}
	bindMu.Lock()
	defer bindMu.Unlock()
	if f, ok := bindCache[key]; ok {
		fv.Set(f)
		return nil
	}
	f := reflect.New(fv.Type())
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("com: %s as %s: %v", op, fv.Type(), r)
		}
	}()
	purego.RegisterFunc(f.Interface(), addr)
	bindCache[key] = f.Elem()
	fv.Set(f.Elem())
	return nil
}

// boundFuncs returns the number of cached bound funcs, for tests.
func boundFuncs() int {
	bindMu.Lock()
	defer bindMu.Unlock()
	return len(bindCache)
}
