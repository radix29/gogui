//go:build darwin

package main

import (
	"errors"
	"fmt"
	"math/bits"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego/objc"

	"github.com/radix29/gogui/internal/ffi"
)

// The class builder binds the class-pair functions of libobjc itself
// instead of using objc.RegisterClass, which creates a hidden purego
// callback per method and per ivar accessor (PLAN.md S0.8, "Hidden
// callbacks"). Methods get their IMP from the fixed trampolines in
// classes.go, so ffi.Callbacks() is the real callback count.

const (
	libobjcPath = "/usr/lib/libobjc.A.dylib"
	appKitPath  = "/System/Library/Frameworks/AppKit.framework/AppKit"

	// handleIvar is the ivar every GG class carries: an ffi.Handle into
	// peers, never a Go pointer (PLAN.md §7.2).
	handleIvar = "goHandle"
)

var rt struct {
	allocateClassPair func(super objc.Class, name string, extraBytes uintptr) objc.Class
	disposeClassPair  func(cls objc.Class)
	registerClassPair func(cls objc.Class)
	addIvar           func(cls objc.Class, name string, size uintptr, log2Align uint8, types string) bool
}

// loadRuntime binds the libobjc functions above and loads AppKit (which
// pulls in Foundation). The libraries stay open for the process.
func loadRuntime() error {
	lib, err := ffi.Dlopen(libobjcPath)
	if err != nil {
		return err
	}
	for _, b := range []struct {
		fptr any
		name string
	}{
		{&rt.allocateClassPair, "objc_allocateClassPair"},
		{&rt.disposeClassPair, "objc_disposeClassPair"},
		{&rt.registerClassPair, "objc_registerClassPair"},
		{&rt.addIvar, "class_addIvar"},
	} {
		if err := lib.Bind(b.fptr, b.name); err != nil {
			return err
		}
	}
	_, err = ffi.Dlopen(appKitPath)
	return err
}

var (
	selMu    sync.Mutex
	selCache = map[string]objc.SEL{}
)

// sel returns the cached selector for name. sel_registerName takes the
// runtime lock, so each name is registered once.
func sel(name string) objc.SEL {
	selMu.Lock()
	defer selMu.Unlock()
	s, ok := selCache[name]
	if !ok {
		s = objc.RegisterName(name)
		selCache[name] = s
	}
	return s
}

// class returns the class called name and panics if it doesn't exist (the
// self-test turns the panic into a failed check).
func class(name string) objc.Class {
	c := objc.GetClass(name)
	if c == 0 {
		panic("objc: class " + name + " not found")
	}
	return c
}

// alloc returns [[name alloc] init…] using the init selector and args.
func alloc(name, init string, args ...any) objc.ID {
	return objc.ID(class(name)).Send(sel("alloc")).Send(sel(init), args...)
}

func release(obj objc.ID) { obj.Send(sel("release")) }

// nsString returns an autoreleased NSString with a copy of s.
func nsString(s string) objc.ID {
	p := ffi.CString(s)
	defer ffi.Free(p)
	return objc.ID(class("NSString")).Send(sel("stringWithUTF8String:"), p)
}

// goString copies an NSString into Go.
func goString(str objc.ID) string {
	if str == 0 {
		return ""
	}
	return ffi.GoString(uintptr(str.Send(sel("UTF8String"))))
}

// withPool runs f inside an NSAutoreleasePool.
func withPool(f func()) {
	pool := objc.ID(class("NSAutoreleasePool")).Send(sel("new"))
	defer pool.Send(sel("drain"))
	f()
}

// methodDef is one method of a classDef. imp is a trampoline from
// ffi.RegisterCallback; types is its hand-written type encoding.
type methodDef struct {
	sel   string
	imp   uintptr
	types string
}

type classDef struct {
	name, super string
	methods     []methodDef
}

// defineClass allocates, fills and registers a class pair: the handle
// ivar (ivars can only be added before objc_registerClassPair) and the
// methods. A name that is already taken is an error, not a crash, and a
// half-built pair is disposed of.
func defineClass(d classDef) (objc.Class, error) {
	super := objc.GetClass(d.super)
	if super == 0 {
		return 0, fmt.Errorf("superclass %s of %s not found", d.super, d.name)
	}
	cls := rt.allocateClassPair(super, d.name, 0)
	if cls == 0 {
		return 0, fmt.Errorf("objc_allocateClassPair(%s, %s) returned Nil (name already in use?)", d.super, d.name)
	}
	fail := func(msg string) (objc.Class, error) {
		rt.disposeClassPair(cls)
		return 0, errors.New(d.name + ": " + msg)
	}
	size := unsafe.Sizeof(uintptr(0))
	if !rt.addIvar(cls, handleIvar, size, uint8(bits.TrailingZeros(uint(size))), "Q") {
		return fail("class_addIvar " + handleIvar + " failed")
	}
	for _, m := range d.methods {
		if !cls.AddMethod(sel(m.sel), objc.IMP(m.imp), m.types) {
			return fail("class_addMethod " + m.sel + " failed")
		}
	}
	rt.registerClassPair(cls)
	return cls, nil
}

var (
	offMu      sync.Mutex
	offByClass = map[objc.Class]uintptr{}
)

// handleOffset returns the offset of the handle ivar in obj, cached per
// class. It panics if obj's class has no handle ivar.
func handleOffset(obj objc.ID) uintptr {
	cls := obj.Class()
	offMu.Lock()
	defer offMu.Unlock()
	off, ok := offByClass[cls]
	if !ok {
		iv := cls.InstanceVariable(handleIvar)
		if iv == 0 {
			panic("objc: object has no " + handleIvar + " ivar")
		}
		off = iv.Offset()
		offByClass[cls] = off
	}
	return off
}

// The ivar holds a uintptr, not an object, so it is accessed at
// self + ivar_getOffset rather than with object_getIvar (PLAN.md S0.8).

func getHandle(obj objc.ID) ffi.Handle {
	return ffi.Handle(ffi.ReadUintptr(uintptr(obj), handleOffset(obj)))
}

func setHandle(obj objc.ID, h ffi.Handle) {
	ffi.WriteUintptr(uintptr(obj), handleOffset(obj), uintptr(h))
}
