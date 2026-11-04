package gtk4

import (
	"fmt"
	"runtime/debug"
)

// PanicError is the panic value the shell re-raises when Go code called
// from GTK panicked: a vfunc, a signal handler, an idle source. The panic
// was recovered inside the trampoline, so it did not unwind through GTK's
// C frames, and re-raised (rethrow) once the native call had returned.
type PanicError struct {
	// Value is the original panic value.
	Value any
	// Callback names the trampoline that recovered it, as registered with
	// ffi.RegisterCallback ("gtk4.GtkWidgetClass.snapshot").
	Callback string
	// Stack is the stack of the GTK thread at the time of the panic.
	Stack []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("panic in the gtk4 callback %s: %v\n\n%s", e.Callback, e.Value, e.Stack)
}

// Unwrap returns the original panic value if it is an error.
func (e *PanicError) Unwrap() error {
	if err, ok := e.Value.(error); ok {
		return err
	}
	return nil
}

// pendingPanic is the panic a trampoline recovered that has not been
// re-raised yet. GTK thread only: the one trampoline GLib may call from
// another thread, closureNotify (an object finalized there), calls only
// dispatch.Free, which does not panic.
var pendingPanic *PanicError

// onPanic, if set, runs once when a trampoline keeps a pending panic. The
// shell sets it to quit its main loop (S4.1d), so Run returns to Go and
// re-raises the panic.
var onPanic func()

// trap is deferred, directly, by every trampoline (recover only works in
// the deferred function itself): it recovers a panic of the trampoline's
// Go code and keeps it in pendingPanic. The first panic wins; a
// *PanicError that rethrow raised in a nested callback is kept as it is.
// The trampoline then returns its zero result to GTK.
func trap(callback string) {
	v := recover()
	if v == nil || pendingPanic != nil {
		return
	}
	if pe, ok := v.(*PanicError); ok {
		pendingPanic = pe
	} else {
		pendingPanic = &PanicError{Value: v, Callback: callback, Stack: debug.Stack()}
	}
	if onPanic != nil {
		// A panic of the hook itself must not unwind into GTK either; the
		// first panic is the one that matters, so the hook's is dropped.
		func() {
			defer func() { _ = recover() }()
			onPanic()
		}()
	}
}

// rethrow re-raises the pending panic, if any. Go code calls it after
// every GTK call that may have run a trampoline (the main loop, a signal
// emission, a measure), once that call has returned.
func rethrow() {
	if p := pendingPanic; p != nil {
		pendingPanic = nil
		panic(p)
	}
}
