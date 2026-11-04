package ffi

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"unsafe"
)

// fakeRegistry returns a registry whose create function hands out fake
// addresses and counts its calls, so the tests don't use up real callbacks.
func fakeRegistry(limit int) (*callbackRegistry, *atomic.Int64) {
	var created atomic.Int64
	r := &callbackRegistry{limit: limit, create: func(any) uintptr {
		return uintptr(0x1000 + created.Add(1))
	}}
	return r, &created
}

// mustPanic runs f and returns its panic message, failing t if f doesn't
// panic or the message doesn't contain want.
func mustPanic(t *testing.T, want string, f func()) string {
	t.Helper()
	var msg string
	func() {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			msg = fmt.Sprint(r)
		}()
		f()
		msg = "<no panic>"
	}()
	if msg == "<no panic>" {
		t.Fatalf("no panic, want one containing %q", want)
	}
	if !strings.Contains(msg, want) {
		t.Fatalf("panic %q, want it to contain %q", msg, want)
	}
	return msg
}

func TestCallbackRegisterAndCount(t *testing.T) {
	r, created := fakeRegistry(10)
	if n := r.count(); n != 0 {
		t.Fatalf("count of empty registry = %d", n)
	}
	a := r.register("a", func(uintptr) uintptr { return 0 })
	b := r.register("b", func(h, msg, wp, lp uintptr) uintptr { return 0 })
	if a == 0 || b == 0 || a == b {
		t.Fatalf("addresses a=%#x b=%#x, want distinct non-zero", a, b)
	}
	if n := r.count(); n != 2 {
		t.Fatalf("count = %d, want 2", n)
	}
	if n := created.Load(); n != 2 {
		t.Fatalf("create called %d times, want 2", n)
	}
}

func TestCallbackDuplicateNamePanicsBeforeCreating(t *testing.T) {
	r, created := fakeRegistry(10)
	fn := func(uintptr) uintptr { return 0 }
	r.register("gtk.clicked", fn)
	msg := mustPanic(t, "registered twice", func() { r.register("gtk.clicked", fn) })
	if !strings.Contains(msg, `"gtk.clicked"`) {
		t.Errorf("panic %q doesn't name the callback", msg)
	}
	// A different function under the same name is still a duplicate.
	mustPanic(t, "registered twice", func() {
		r.register("gtk.clicked", func(uintptr) uintptr { return 1 })
	})
	if n := created.Load(); n != 1 {
		t.Fatalf("create called %d times, want 1 (duplicates must not create)", n)
	}
	if n := r.count(); n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
}

func TestCallbackEmptyNamePanics(t *testing.T) {
	r, created := fakeRegistry(10)
	mustPanic(t, "empty name", func() { r.register("", func(uintptr) uintptr { return 0 }) })
	if created.Load() != 0 || r.count() != 0 {
		t.Fatal("an empty name created a callback")
	}
}

func TestCallbackBadSignaturePanicsBeforeCreating(t *testing.T) {
	r, created := fakeRegistry(10)
	mustPanic(t, `callback "bad": fn is a string, not a function`,
		func() { r.register("bad", "nope") })
	mustPanic(t, `callback "bad": argument 0`,
		func() { r.register("bad", func(string) uintptr { return 0 }) })
	if created.Load() != 0 || r.count() != 0 {
		t.Fatal("a bad signature created a callback")
	}
	// The name stays free after a rejected registration.
	r.register("bad", func(uintptr) uintptr { return 0 })
}

func TestCallbackRegistryBudget(t *testing.T) {
	r, created := fakeRegistry(3)
	fn := func() uintptr { return 0 }
	for i := range 3 {
		r.register("cb"+strconv.Itoa(i), fn)
	}
	mustPanic(t, "budget of 3 native callbacks is used up", func() { r.register("cb3", fn) })
	if created.Load() != 3 || r.count() != 3 {
		t.Fatalf("created %d, count %d; want 3, 3", created.Load(), r.count())
	}
}

func TestCallbackCreatePanicRecordsNothing(t *testing.T) {
	fail := true
	r := &callbackRegistry{limit: 10, create: func(any) uintptr {
		if fail {
			panic("create failed")
		}
		return 1
	}}
	fn := func() uintptr { return 0 }
	mustPanic(t, "create failed", func() { r.register("x", fn) })
	if n := r.count(); n != 0 {
		t.Fatalf("count after a failed create = %d, want 0", n)
	}
	fail = false
	if p := r.register("x", fn); p != 1 {
		t.Fatalf("register after a failed create = %#x, want 1", p)
	}
	// The registry's mutex was released by the panicking create.
	if n := r.count(); n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
}

func TestCallbackSignatures(t *testing.T) {
	type small struct{ A, B int32 }
	type myUintptr uintptr
	var nilFn func() uintptr
	tests := []struct {
		name string
		fn   any
		// want is the expected message on windows and on unix; "" means
		// accepted. A substring is enough.
		windows, unix string
	}{
		{"wndproc", func(hwnd, msg, wp, lp uintptr) uintptr { return 0 }, "", ""},
		{"no args", func() uintptr { return 0 }, "", ""},
		{"named uintptr", func(myUintptr) myUintptr { return 0 }, "", ""},
		{"int and pointer result", func(*int, unsafe.Pointer, bool, int8, uint16, int32) int { return 0 }, "", ""},
		{"pointer result", func() *byte { return nil }, "", ""},
		{"unsafe.Pointer result", func() unsafe.Pointer { return nil }, "", ""},
		{"no result", func(uintptr) {}, "has no result", ""},
		{"int32 result", func() int32 { return 0 }, "is not uintptr-sized", ""},
		{"bool result", func() bool { return false }, "is not uintptr-sized", ""},
		{"float arg", func(float64) uintptr { return 0 }, "not supported on Windows", ""},
		{"float32 arg", func(uintptr, float32) uintptr { return 0 }, "argument 1", ""},
		{"float result", func() float64 { return 0 }, "not supported on Windows", ""},
		{"two results", func() (uintptr, uintptr) { return 0, 0 }, "at most one", "at most one"},
		{"string arg", func(string) uintptr { return 0 }, "can't be passed", "can't be passed"},
		{"slice arg", func([]byte) uintptr { return 0 }, "can't be passed", "can't be passed"},
		{"struct arg", func(small) uintptr { return 0 }, "can't be passed", "can't be passed"},
		{"interface arg", func(any) uintptr { return 0 }, "can't be passed", "can't be passed"},
		{"func arg", func(func()) uintptr { return 0 }, "can't be passed", "can't be passed"},
		{"string result", func() string { return "" }, "can't be passed", "can't be passed"},
		{"variadic", func(...uintptr) uintptr { return 0 }, "variadic", "variadic"},
		{"too many args", func(a, b, c, d, e, f, g, h, i, j, k, l, m uintptr) uintptr { return 0 },
			"more than 12", "more than 12"},
		{"twelve args", func(a, b, c, d, e, f, g, h, i, j, k, l uintptr) uintptr { return 0 }, "", ""},
		{"nil", nil, "fn is nil", "fn is nil"},
		{"nil func", nilFn, "fn is a nil func", "fn is a nil func"},
		{"not a func", 42, "not a function", "not a function"},
	}
	if unsafe.Sizeof(uintptr(0)) == 8 {
		tests = append(tests, struct {
			name          string
			fn            any
			windows, unix string
		}{"int64 arg", func(int64, uint64) uintptr { return 0 }, "", ""})
	}
	for _, tt := range tests {
		for _, c := range []struct{ goos, want string }{{"windows", tt.windows}, {"linux", tt.unix}, {"darwin", tt.unix}} {
			got := checkCallbackSignature(tt.fn, c.goos)
			switch {
			case c.want == "" && got != "":
				t.Errorf("%s on %s: rejected with %q, want accepted", tt.name, c.goos, got)
			case c.want != "" && !strings.Contains(got, c.want):
				t.Errorf("%s on %s: got %q, want a message containing %q", tt.name, c.goos, got, c.want)
			}
		}
	}
}

func TestCallbackConcurrentRegistration(t *testing.T) {
	const goroutines, perG = 8, 50
	// Room for every unique name plus one winner per shared name.
	r, created := fakeRegistry(goroutines*perG + perG)
	fn := func() uintptr { return 0 }
	var wg sync.WaitGroup
	var dupPanics atomic.Int64
	for g := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range perG {
				r.register("g"+strconv.Itoa(g)+"."+strconv.Itoa(i), fn)
				// Every goroutine also races for one shared name per round;
				// exactly one of them wins it.
				func() {
					defer func() {
						if recover() != nil {
							dupPanics.Add(1)
						}
					}()
					r.register("shared."+strconv.Itoa(i), fn)
				}()
			}
		}()
	}
	wg.Wait()
	want := goroutines*perG + perG
	if r.count() != want || created.Load() != int64(want) {
		t.Fatalf("count %d, created %d; want %d", r.count(), created.Load(), want)
	}
	if got, want := dupPanics.Load(), int64((goroutines-1)*perG); got != want {
		t.Fatalf("%d shared registrations panicked, want %d", got, want)
	}
}

func TestMaxCallbacks(t *testing.T) {
	// One budget on every OS (S0.10b): Windows' guarantee, with headroom
	// below purego's 2000 slots on darwin and linux.
	if MaxCallbacks != 1024 {
		t.Fatalf("MaxCallbacks = %d, want 1024", MaxCallbacks)
	}
	if callbacks.limit != MaxCallbacks {
		t.Fatalf("package registry limit = %d, want MaxCallbacks %d", callbacks.limit, MaxCallbacks)
	}
}

// TestCheckCallback checks the exported wrapper of the signature rules: it
// agrees with them per OS and creates no callback.
func TestCheckCallback(t *testing.T) {
	before := Callbacks()
	floaty := func(uintptr, float64) {}
	if err := CheckCallback(floaty, "linux"); err != nil {
		t.Errorf("CheckCallback(void(uintptr, float64), linux) = %v, want nil", err)
	}
	if err := CheckCallback(floaty, "windows"); err == nil || !strings.HasPrefix(err.Error(), "ffi: ") {
		t.Errorf("CheckCallback(void(uintptr, float64), windows) = %v, want an ffi: error", err)
	}
	if err := CheckCallback(nil, "linux"); err == nil {
		t.Error("CheckCallback(nil) = nil")
	}
	if got := Callbacks(); got != before {
		t.Errorf("CheckCallback created callbacks: %d → %d", before, got)
	}
}
