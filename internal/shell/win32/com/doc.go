// Package com is the COM core of the Windows shell (PLAN.md S2.4a): interface
// pointers with reference counting, GUIDs, HRESULT errors, apartment setup
// for the UI thread, and the two call paths into vtable methods.
//
// The bindings of the interface families (dxgi, d3d11, dcomp, d2d; S2.4b and
// S2.4c) are built on it. Rules:
//
//   - A Ptr is the address of a native COM object, whose first word points
//     to its vtable. Vtable words are read with ffi.ReadUintptr, so the
//     package needs no uintptr→unsafe.Pointer conversion of its own. A Ptr
//     owns one reference; Release drops it and clears the Ptr, so a second
//     Release is a no-op. Nothing is released by a finalizer (PLAN.md §7.2).
//   - Integer-only methods (every argument and the result an integer, a
//     handle or an address) go through syscall.SyscallN on the slot
//     address: Ptr.Call and Ptr.CallHR.
//   - Methods that take FLOATs or small float structs by value go through
//     purego.RegisterFunc on the slot address (Method). syscall.SyscallN
//     copies the first four arguments into the XMM registers on amd64, but
//     its windows/arm64 trampoline loads only the integer registers (Go
//     1.26 and 1.27), so floats would arrive as garbage there; purego uses its own
//     arm64 trampoline with the float registers. A bound func is cached per
//     signature and code address, never per object: objects of one class
//     share their vtable, so the cache holds one entry per method and
//     implementation. RegisterFunc builds the func with reflect.MakeFunc
//     and creates no native callback, so the hidden-callback guard of
//     check.sh stays green; Go func arguments are rejected.
//   - MSVC returns a struct from a member function through a hidden pointer
//     that follows this (D2D's GetSize, GetPixelSize), unlike a plain C
//     function. Method therefore rejects struct results; such a method is
//     declared with that out pointer as its second argument.
//   - CoInitializeEx with COINIT_APARTMENTTHREADED runs on the UI thread
//     (InitUIThread, called by win32's Run); every interface pointer is used
//     on that thread only.
//
// guid.go and hresult.go have no build tag, so their pure tests run on every
// OS; everything that calls Windows is //go:build windows.
package com
