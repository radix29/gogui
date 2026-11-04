//go:build darwin || linux

package ffi

import (
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"unsafe"
)

func openLibc(t *testing.T) *Lib {
	t.Helper()
	lib, err := Dlopen(libcPath)
	if err != nil {
		t.Fatalf("Dlopen(%q): %v", libcPath, err)
	}
	t.Cleanup(func() {
		if err := lib.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return lib
}

func TestDlopenBind(t *testing.T) {
	lib := openLibc(t)
	if lib.Path() != libcPath {
		t.Fatalf("Path() = %q", lib.Path())
	}
	var getpid func() int32
	if err := lib.Bind(&getpid, "getpid"); err != nil {
		t.Fatal(err)
	}
	if got, want := int(getpid()), os.Getpid(); got != want {
		t.Fatalf("getpid() = %d, want %d", got, want)
	}
	var strlen func(s uintptr) uintptr
	if err := lib.Bind(&strlen, "strlen"); err != nil {
		t.Fatal(err)
	}
	p := CString("hello")
	defer Free(p)
	if n := strlen(p); n != 5 {
		t.Fatalf("strlen = %d, want 5", n)
	}
	if p := lib.MustSym("strlen"); p == 0 {
		t.Fatal("MustSym returned 0")
	}
}

func TestDlopenErrors(t *testing.T) {
	if _, err := Dlopen(""); err == nil || !strings.Contains(err.Error(), "empty path") {
		t.Fatalf("Dlopen(\"\") error = %v", err)
	}
	const missing = "/nonexistent/libgogui-missing.so"
	_, err := Dlopen(missing)
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("Dlopen(missing) error = %v, want one naming %s", err, missing)
	}

	lib := openLibc(t)
	_, err = lib.Sym("gogui_no_such_symbol")
	if err == nil || !strings.Contains(err.Error(), "gogui_no_such_symbol") ||
		!strings.Contains(err.Error(), libcPath) {
		t.Fatalf("Sym(missing) error = %v, want one naming symbol and library", err)
	}
	var f func()
	err = lib.Bind(&f, "gogui_no_such_symbol")
	if err == nil || !strings.Contains(err.Error(), "gogui_no_such_symbol") || f != nil {
		t.Fatalf("Bind(missing) error = %v, f set: %v", err, f != nil)
	}
	mustPanic(t, "gogui_no_such_symbol", func() { lib.MustSym("gogui_no_such_symbol") })

	// Bad fptr values are errors, not panics.
	for _, bad := range []any{nil, f, new(int), (*func())(nil)} {
		if err := lib.Bind(bad, "strlen"); err == nil || !strings.Contains(err.Error(), "fptr") {
			t.Fatalf("Bind(%T) error = %v", bad, err)
		}
	}
	// A func type purego rejects (two results) is an error naming the symbol.
	var two func() (int, int)
	if err := lib.Bind(&two, "getpid"); err == nil || !strings.Contains(err.Error(), "getpid") {
		t.Fatalf("Bind(two results) error = %v", err)
	}

	// After Close, Sym fails and a second Close does nothing.
	lib2, err := Dlopen(libcPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := lib2.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lib2.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := lib2.Sym("strlen"); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("Sym after Close error = %v", err)
	}
}

func TestCStringRoundTrip(t *testing.T) {
	lib := openLibc(t)
	var strlen func(s uintptr) uintptr
	if err := lib.Bind(&strlen, "strlen"); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"", "a", "hello, world", "Grüße, 日本語, 🙂", strings.Repeat("x", 1<<16)} {
		p := CString(s)
		if p == 0 {
			t.Fatalf("CString(%q) = 0", s)
		}
		if n := strlen(p); n != uintptr(len(s)) {
			t.Errorf("strlen(CString(%.20q)) = %d, want %d", s, n, len(s))
		}
		if got := GoString(p); got != s {
			t.Errorf("GoString(CString(%.20q)) = %.20q", s, got)
		}
		if got := GoStringN(p, len(s)); got != s {
			t.Errorf("GoStringN(CString(%.20q)) = %.20q", s, got)
		}
		Free(p)
	}
	mustPanic(t, "NUL byte at offset 3", func() { CString("abc\x00def") })

	if GoString(0) != "" || GoStringN(0, 5) != "" {
		t.Fatal("NULL pointers must give empty strings")
	}
	Free(0) // must not crash

	// GoStringN keeps embedded NULs; GoString stops at the first one.
	p := CString("abcdef")
	defer Free(p)
	b := unsafe.Slice((*byte)(cptr(p)), 7)
	b[2] = 0
	if got := GoString(p); got != "ab" {
		t.Fatalf("GoString = %q, want \"ab\"", got)
	}
	if got := GoStringN(p, 6); got != "ab\x00def" {
		t.Fatalf("GoStringN = %q", got)
	}
	if GoStringN(p, 0) != "" || GoStringN(p, -1) != "" {
		t.Fatal("GoStringN with n <= 0 must be empty")
	}
	// The result is a copy: changing C memory doesn't change it.
	s := GoStringN(p, 6)
	b[0] = 'X'
	if s[0] != 'a' {
		t.Fatal("GoStringN did not copy")
	}
}

// The qsort comparator is registered once per process: -count=N runs the
// test N times, and a second registration would (correctly) panic.
// qsortGrew is how much Callbacks() grew through the registration; other
// tests register their own callbacks later, so a later Callbacks() can't be
// compared with a value from the first run.
var (
	qsortOnce  sync.Once
	qsortCmp   uintptr
	qsortGrew  int
	qsortCalls atomic.Int64
)

// qsortCallback registers the comparator on first use and returns it. The
// qsort benchmark (thread_unix_test.go) uses it too.
func qsortCallback() uintptr {
	qsortOnce.Do(func() {
		before := Callbacks()
		qsortCmp = RegisterCallback("test.qsort", func(a, b uintptr) int32 {
			qsortCalls.Add(1)
			x, y := *(*int32)(cptr(a)), *(*int32)(cptr(b))
			switch {
			case x < y:
				return -1
			case x > y:
				return 1
			}
			return 0
		})
		qsortGrew = Callbacks() - before
	})
	return qsortCmp
}

// TestCallbackQsort registers a real callback (one of the process's budget)
// and lets C call it: libc qsort sorts an int32 array in C memory with a Go
// comparator, the shape of every driver callback (pointer arguments, an
// int result, called from C on the calling thread).
func TestCallbackQsort(t *testing.T) {
	qsortCallback()
	if qsortCmp == 0 {
		t.Fatal("RegisterCallback returned 0")
	}
	lib := openLibc(t)
	var malloc func(n uintptr) uintptr
	var free func(p uintptr)
	var qsort func(base, n, size, cmp uintptr)
	for name, fp := range map[string]any{"malloc": &malloc, "free": &free, "qsort": &qsort} {
		if err := lib.Bind(fp, name); err != nil {
			t.Fatal(err)
		}
	}
	in := []int32{42, -7, 0, 1 << 30, -1 << 31, 3, 3, 17}
	mem := malloc(uintptr(len(in)) * 4)
	if mem == 0 {
		t.Fatal("malloc failed")
	}
	defer free(mem)
	arr := unsafe.Slice((*int32)(cptr(mem)), len(in))
	copy(arr, in)
	before := qsortCalls.Load()
	qsort(mem, uintptr(len(arr)), 4, qsortCmp)
	if qsortCalls.Load() == before {
		t.Fatal("comparator was never called")
	}
	want := []int32{-1 << 31, -7, 0, 3, 3, 17, 42, 1 << 30}
	for i := range want {
		if arr[i] != want[i] {
			t.Fatalf("sorted = %v, want %v", arr, want)
		}
	}
	if qsortGrew != 1 {
		t.Fatalf("registering the comparator grew Callbacks() by %d, want 1", qsortGrew)
	}
	count := Callbacks()
	mustPanic(t, "registered twice", func() {
		RegisterCallback("test.qsort", func(a, b uintptr) int32 { return 0 })
	})
	if n := Callbacks(); n != count {
		t.Fatalf("Callbacks() after a duplicate = %d, want %d", n, count)
	}
}

// TestBindAddr binds libc strlen by its address, as a driver binds a vfunc
// slot it read from a class struct, and calls it like Bind's strlen.
func TestBindAddr(t *testing.T) {
	lib := openLibc(t)
	addr, err := lib.Sym("strlen")
	if err != nil {
		t.Fatal(err)
	}
	var strlen func(s uintptr) uintptr
	if err := BindAddr(&strlen, addr); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"", "hello", "Grüße"} {
		p := CString(s)
		n := strlen(p)
		Free(p)
		if n != uintptr(len(s)) {
			t.Fatalf("strlen(%q) via BindAddr = %d, want %d", s, n, len(s))
		}
	}

	// A NULL address is an error and leaves the func unset.
	var f func(s uintptr) uintptr
	err = BindAddr(&f, 0)
	if err == nil || !strings.Contains(err.Error(), "NULL function pointer") || f != nil {
		t.Fatalf("BindAddr(0) error = %v, f set: %v", err, f != nil)
	}
	// Bad fptr values are errors, not panics.
	for _, bad := range []any{nil, f, new(int), (*func())(nil)} {
		if err := BindAddr(bad, addr); err == nil || !strings.Contains(err.Error(), "fptr") {
			t.Fatalf("BindAddr(%T) error = %v", bad, err)
		}
	}
	// A func type purego rejects is an error naming the address.
	var two func() (int, int)
	err = BindAddr(&two, addr)
	if err == nil || !strings.Contains(err.Error(), "0x") || two != nil {
		t.Fatalf("BindAddr(two results) error = %v", err)
	}
}

// TestBindRejectsFuncArgs checks the hidden-callback guard (S0.10b): Bind
// and BindAddr refuse a func type with a Go func argument, which purego
// would turn into a new native callback on every call, before purego sees
// it, and leave the func unset. A func-typed result is still bound and
// works.
func TestBindRejectsFuncArgs(t *testing.T) {
	lib := openLibc(t)
	var qsort func(base, n, size uintptr, cmp func(a, b uintptr) int32)
	err := lib.Bind(&qsort, "qsort")
	if err == nil || qsort != nil ||
		!strings.Contains(err.Error(), "qsort") || !strings.Contains(err.Error(), "argument 3") ||
		!strings.Contains(err.Error(), "RegisterCallback") {
		t.Fatalf("Bind(qsort with a func comparator) error = %v, set: %v", err, qsort != nil)
	}
	addr := lib.MustSym("qsort")
	err = BindAddr(&qsort, addr)
	if err == nil || qsort != nil || !strings.Contains(err.Error(), "0x") ||
		!strings.Contains(err.Error(), "argument 3") || !strings.Contains(err.Error(), "RegisterCallback") {
		t.Fatalf("BindAddr(qsort with a func comparator) error = %v, set: %v", err, qsort != nil)
	}
	// The guard comes before the address check: a NULL address with a func
	// argument reports the func argument.
	if err := BindAddr(&qsort, 0); err == nil || !strings.Contains(err.Error(), "argument 3") {
		t.Fatalf("BindAddr(0, func argument) error = %v", err)
	}

	// A func-typed result is allowed: dlsym returns strlen's address, which
	// purego wraps with RegisterFunc (no callback slot).
	var dlsym func(handle, name uintptr) func(s uintptr) uintptr
	if err := lib.Bind(&dlsym, "dlsym"); err != nil {
		t.Fatalf("Bind(dlsym with a func result): %v", err)
	}
	name := CString("strlen")
	defer Free(name)
	strlen := dlsym(lib.h, name)
	if strlen == nil {
		t.Fatal("dlsym(strlen) returned nil")
	}
	s := CString("budget")
	defer Free(s)
	if n := strlen(s); n != 6 {
		t.Fatalf("strlen via a func result = %d, want 6", n)
	}
}
