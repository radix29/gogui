//go:build darwin || linux

package ffi

import "testing"

func TestReadWriteUintptrMalloc(t *testing.T) {
	const n = 16
	f := mustLibc()
	p := f.malloc(n * wordSize)
	if p == 0 {
		t.Fatal("malloc failed")
	}
	defer f.free(p)
	checkWordRoundTrip(t, p, n)
}

func TestReadWriteInt32Malloc(t *testing.T) {
	const n = 16
	f := mustLibc()
	p := f.malloc(n * 4)
	if p == 0 {
		t.Fatal("malloc failed")
	}
	defer f.free(p)
	checkInt32RoundTrip(t, p, n)
}

func TestAtMalloc(t *testing.T) {
	f := mustLibc()
	p := f.malloc(16)
	if p == 0 {
		t.Fatal("malloc failed")
	}
	defer f.free(p)
	checkAtRoundTrip(t, p)
}
