//go:build windows

package com

import "sync/atomic"

// Method holds the bound func of one float-path method (S2.4d_1, moved here
// from d2d in S2.5a so every binding package shares it). Bind already
// caches by func type and code address, but behind a mutex and a map; a
// drawing or layout method is called many times per frame, so each one
// keeps the last bound func and the code address it was bound for, and goes
// back to Bind only when an object of another class (another slot address)
// comes along. Since review P2 the binding packages use it on
// windows/arm64 only (and in the tests' bound-path runs); windows/amd64
// takes the direct path of direct.go.
//
// A binding package declares one per method as a package variable:
//
//	var methodSetDpi = com.Method[func(this com.Ptr, x, y float32)]{Slot: slotRTSetDpi}
//	methodSetDpi.Fn(c.Ptr)(c.Ptr, x, y)
type Method[F any] struct {
	Slot int
	last atomic.Pointer[boundMethod[F]]
}

type boundMethod[F any] struct {
	addr uintptr
	fn   F
}

// Fn returns the bound func for p's slot. The signatures are fixed by the
// binding packages and valid, so Bind can't refuse them for a non-nil p; a
// refusal is a bug in the binding and panics, and a nil p panics in Slot,
// like every Call on a nil Ptr.
func (m *Method[F]) Fn(p Ptr) F {
	addr := p.Slot(m.Slot)
	if b := m.last.Load(); b != nil && b.addr == addr {
		return b.fn
	}
	var fn F
	if err := Bind(&fn, p, m.Slot); err != nil {
		panic(err)
	}
	m.last.Store(&boundMethod[F]{addr, fn})
	return fn
}
