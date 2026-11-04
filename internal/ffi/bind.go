package ffi

import (
	"reflect"
	"strconv"
)

// checkBindFunc returns why a Go func of type t must not be bound to a C
// function with Bind or BindAddr, or "" if gogui allows it. purego itself
// checks the rest of the signature.
//
// A func-typed argument is rejected (PLAN.md §11.1 S0.10b): purego turns
// every Go func passed in it into a new native callback with NewCallback,
// on every call. Those callbacks can't be freed, ffi.Callbacks never sees
// them, and after about 2000 calls purego's limit is used up. A C function
// that takes a function pointer gets a trampoline from RegisterCallback,
// passed as a uintptr. A func-typed result only wraps the returned C
// pointer and costs no callback, so it stays allowed. A Go func passed at
// run time through a trailing ...any argument can't be seen here; the
// drivers don't bind C variadics that way.
//
// The check is portable so its rules are tested on every OS; only the
// darwin and linux Bind and BindAddr use it.
func checkBindFunc(t reflect.Type) string {
	for i := range t.NumIn() {
		if in := t.In(i); in.Kind() == reflect.Func {
			return "argument " + strconv.Itoa(i) + " is a Go func (" + in.String() +
				"), which purego would turn into a new native callback on every call; " +
				"register the trampoline once with RegisterCallback and pass its pointer as uintptr"
		}
	}
	return ""
}
