//go:build darwin || linux

package ffi

import (
	"strconv"
	"strings"
	"sync"
	"unsafe"
)

// libc holds the C allocator CString and Free use. It is loaded on first
// use.
var libc = sync.OnceValues(func() (*libcFuncs, error) {
	lib, err := Dlopen(libcPath)
	if err != nil {
		return nil, err
	}
	f := &libcFuncs{lib: lib}
	if err := lib.Bind(&f.malloc, "malloc"); err != nil {
		return nil, err
	}
	if err := lib.Bind(&f.free, "free"); err != nil {
		return nil, err
	}
	return f, nil
})

type libcFuncs struct {
	lib    *Lib
	malloc func(n uintptr) uintptr
	free   func(p uintptr)
}

func mustLibc() *libcFuncs {
	f, err := libc()
	if err != nil {
		panic(err)
	}
	return f
}

// CString copies s into a NUL-terminated string in C (malloc) memory and
// returns its address. The caller owns it and releases it with Free. It
// panics if s contains a NUL byte, which C would silently truncate at (the
// drivers strip or reject NULs where text enters gogui), or if malloc
// fails.
func CString(s string) uintptr {
	if i := strings.IndexByte(s, 0); i >= 0 {
		panic("ffi: CString: string contains a NUL byte at offset " + strconv.Itoa(i))
	}
	p := mustLibc().malloc(uintptr(len(s)) + 1)
	if p == 0 {
		panic("ffi: CString: malloc of " + strconv.Itoa(len(s)+1) + " bytes failed")
	}
	b := unsafe.Slice((*byte)(cptr(p)), len(s)+1)
	copy(b, s)
	b[len(s)] = 0
	return p
}

// Free releases memory from CString (or any other libc malloc). Free(0)
// does nothing.
func Free(p uintptr) {
	if p == 0 {
		return
	}
	mustLibc().free(p)
}

// GoString copies the NUL-terminated C string at p into a Go string. It
// returns "" for a NULL p. The C memory is not freed.
func GoString(p uintptr) string {
	if p == 0 {
		return ""
	}
	base := cptr(p)
	n := 0
	for *(*byte)(unsafe.Add(base, n)) != 0 {
		n++
	}
	return string(unsafe.Slice((*byte)(base), n))
}

// GoStringN copies n bytes at p into a Go string, NUL bytes included (for
// APIs that return a pointer and a length). It returns "" for a NULL p or
// n <= 0. The C memory is not freed.
func GoStringN(p uintptr, n int) string {
	if p == 0 || n <= 0 {
		return ""
	}
	return string(unsafe.Slice((*byte)(cptr(p)), n))
}
