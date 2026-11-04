package ffi

import (
	"errors"
	"reflect"
	"runtime"
	"strconv"
	"sync"
	"unsafe"
)

// Native callbacks are a limited resource (PLAN.md §7.2): neither
// syscall.NewCallback (Windows) nor purego.NewCallback (darwin, linux) can
// free a callback, and both have a fixed maximum. The drivers therefore
// create a fixed set of trampolines at start-up, one per signal, vfunc or
// window-proc signature, and dispatch through the Handle stored in
// user_data, an ivar or GWLP_USERDATA. They never create a callback per
// widget.
//
// RegisterCallback is the only way the drivers create native callbacks. It
// names every callback, refuses a second registration of the same name and
// counts what it created, so a driver that registers in a loop fails at once
// instead of slowly using up the process-wide budget.

// MaxCallbacks is the most native callbacks RegisterCallback creates, the
// budget the drivers must stay under. It is the same on every OS (PLAN.md
// §11.1 S0.10b):
//
//   - Windows guarantees 1024 callbacks for syscall.NewCallback and
//     NewCallbackCDecl together. The runtime currently has 2000 slots and
//     takes two itself, and running out is an unrecoverable fatal error, so
//     this budget is the only clean error there.
//   - purego on darwin and linux has 2000 slots (maxCB), shared with
//     everything else in the process that uses purego. purego/objc takes one
//     at init for its block dispose callback, and more for each block
//     signature and for every method and ivar accessor of objc.RegisterClass.
//     1024 leaves about 975 of them for that.
//
// A driver needs about ten trampolines, so 1024 is still about a hundred
// times what it uses. The budget only matters if something creates
// callbacks per widget or per call, which the registry's names catch.
const MaxCallbacks = 1024

// maxCallbackArgs is the most arguments a callback may take. It keeps every
// callback well inside the argument frames of syscall.NewCallback (64 words)
// and purego (15 to 32 words, depending on the architecture). No GTK signal,
// ObjC method or window procedure the drivers use comes close.
const maxCallbackArgs = 12

// callbackRegistry is a set of named callbacks. The package has one, used
// through RegisterCallback and Callbacks; tests make their own with a fake
// create function so they don't use up real callbacks.
type callbackRegistry struct {
	mu    sync.Mutex
	names map[string]uintptr
	// limit is the most callbacks the registry creates.
	limit int
	// create makes the native callback for fn, whose signature has been
	// checked. It is nativeCallback outside tests.
	create func(fn any) uintptr
}

var callbacks = callbackRegistry{limit: MaxCallbacks, create: nativeCallback}

// RegisterCallback creates a native callback for fn and returns its C
// function pointer. name identifies the callback in the process (for
// example "gtk.clicked" or "win32.wndproc") and may be registered only once:
// a second registration panics before any callback is created, which is
// how a driver that creates callbacks per widget is caught. RegisterCallback
// also panics if fn is not a function with a signature that every platform's
// callback mechanism supports (see checkCallbackSignature), or if the
// MaxCallbacks budget is used up.
//
// The returned pointer stays valid for the life of the process. fn runs on
// whatever thread native code calls it from; callbacks that touch the UI
// must check that it is the UI thread.
//
// RegisterCallback is safe for concurrent use, but drivers register their
// callbacks at start-up on the UI thread.
func RegisterCallback(name string, fn any) uintptr {
	return callbacks.register(name, fn)
}

// Callbacks returns the number of native callbacks RegisterCallback has
// created. It never exceeds MaxCallbacks.
func Callbacks() int {
	return callbacks.count()
}

func (r *callbackRegistry) register(name string, fn any) uintptr {
	if name == "" {
		panic("ffi: RegisterCallback with an empty name")
	}
	if msg := checkCallbackSignature(fn, runtime.GOOS); msg != "" {
		panic("ffi: callback " + strconv.Quote(name) + ": " + msg)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.names[name]; dup {
		panic("ffi: callback " + strconv.Quote(name) + " registered twice; " +
			"create each trampoline once at start-up and dispatch through a Handle")
	}
	if len(r.names) >= r.limit {
		panic("ffi: callback " + strconv.Quote(name) + ": the budget of " +
			strconv.Itoa(r.limit) + " native callbacks is used up")
	}
	// create may panic (the S0.7a placeholder on darwin and linux, or a
	// runtime limit); the name is recorded only after it succeeds.
	p := r.create(fn)
	if r.names == nil {
		r.names = make(map[string]uintptr)
	}
	r.names[name] = p
	return p
}

// CheckCallback returns an error if fn could not be registered with
// RegisterCallback on goos ("windows", "linux", "darwin"), and nil if it
// could. It creates no callback, so a driver's tests can check the
// trampolines of another OS on any host (the gtk4 shell's set, which has
// float arguments and void results, is checked on Windows this way).
func CheckCallback(fn any, goos string) error {
	if msg := checkCallbackSignature(fn, goos); msg != "" {
		return errors.New("ffi: " + msg)
	}
	return nil
}

func (r *callbackRegistry) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.names)
}

// checkCallbackSignature returns why fn can't be a native callback on goos,
// or "" if it can. It applies gogui's own rules, which are the intersection
// of what syscall.NewCallback and purego.NewCallback support, so a bad
// signature fails here with a clear message instead of deep inside the
// runtime or purego:
//
//   - fn is a non-nil, non-variadic function with at most maxCallbackArgs
//     arguments;
//   - every argument is a bool, an integer, a uintptr, a pointer or an
//     unsafe.Pointer no larger than a uintptr, or (not on Windows) a float32
//     or float64;
//   - on Windows there is exactly one result, a non-float integer, uintptr
//     or pointer exactly as large as a uintptr (a window procedure returns
//     LRESULT);
//   - elsewhere there is at most one result, of the argument kinds above.
//
// Struct arguments and results are not allowed: purego supports some, but
// syscall.NewCallback doesn't, and the drivers don't need them.
func checkCallbackSignature(fn any, goos string) string {
	if fn == nil {
		return "fn is nil"
	}
	v := reflect.ValueOf(fn)
	t := v.Type()
	if t.Kind() != reflect.Func {
		return "fn is a " + t.String() + ", not a function"
	}
	if v.IsNil() {
		return "fn is a nil " + t.String()
	}
	if t.IsVariadic() {
		return "variadic function " + t.String() + " is not supported"
	}
	if t.NumIn() > maxCallbackArgs {
		return t.String() + " has " + strconv.Itoa(t.NumIn()) +
			" arguments, more than " + strconv.Itoa(maxCallbackArgs)
	}
	windows := goos == "windows"
	for i := range t.NumIn() {
		in := t.In(i)
		if msg := checkCallbackScalar(in, !windows); msg != "" {
			return "argument " + strconv.Itoa(i) + " of " + t.String() + ": " + msg
		}
	}
	switch {
	case t.NumOut() > 1:
		return t.String() + " has " + strconv.Itoa(t.NumOut()) +
			" results; a callback has at most one"
	case t.NumOut() == 0 && windows:
		return t.String() + " has no result; Windows callbacks return a uintptr-sized value"
	case t.NumOut() == 1:
		out := t.Out(0)
		if msg := checkCallbackScalar(out, !windows); msg != "" {
			return "result of " + t.String() + ": " + msg
		}
		if windows && out.Size() != unsafe.Sizeof(uintptr(0)) {
			return "result of " + t.String() + ": " + out.String() +
				" is not uintptr-sized, which Windows callbacks require"
		}
	}
	return ""
}

// checkCallbackScalar returns why a value of type t can't cross a callback
// boundary, or "" if it can.
func checkCallbackScalar(t reflect.Type, floats bool) string {
	switch t.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr, reflect.Pointer, reflect.UnsafePointer:
		if t.Size() > unsafe.Sizeof(uintptr(0)) {
			return t.String() + " is larger than a uintptr"
		}
		return ""
	case reflect.Float32, reflect.Float64:
		if !floats {
			return t.String() + ": float arguments and results are not supported on Windows"
		}
		return ""
	default:
		return t.String() + " (" + t.Kind().String() + ") can't be passed to or from native code"
	}
}
