package gtk4

import (
	"errors"
	"strings"
	"testing"
)

// resetPanics clears the panic state around a test.
func resetPanics(t *testing.T) {
	t.Helper()
	pendingPanic, onPanic = nil, nil
	t.Cleanup(func() { pendingPanic, onPanic = nil, nil })
}

// trapped runs fn the way a trampoline does, with trap deferred.
func trapped(name string, fn func()) {
	defer trap(name)
	fn()
}

// rethrown runs rethrow and returns what it raised.
func rethrown() (v any) {
	defer func() { v = recover() }()
	rethrow()
	return nil
}

func TestTrapFirstPanicWins(t *testing.T) {
	resetPanics(t)
	calls := 0
	onPanic = func() { calls++ }

	trapped("gtk4.first", func() {})
	if pendingPanic != nil || calls != 0 {
		t.Fatalf("a callback without a panic left %v, onPanic ran %d times", pendingPanic, calls)
	}
	cause := errors.New("boom")
	trapped("gtk4.first", func() { panic(cause) })
	trapped("gtk4.second", func() { panic("later") })
	if calls != 1 {
		t.Errorf("onPanic ran %d times, want once", calls)
	}
	p := pendingPanic
	if p == nil || p.Value != cause || p.Callback != "gtk4.first" || len(p.Stack) == 0 {
		t.Fatalf("pendingPanic = %+v, want the first panic with its stack", p)
	}
	if !errors.Is(p, cause) {
		t.Error("PanicError does not unwrap to the error value")
	}
	if msg := p.Error(); !strings.Contains(msg, "gtk4.first") || !strings.Contains(msg, "boom") {
		t.Errorf("Error() = %q", msg)
	}

	if got := rethrown(); got != p {
		t.Fatalf("rethrow raised %v, want the pending *PanicError", got)
	}
	if pendingPanic != nil {
		t.Error("rethrow left the panic pending")
	}
	if got := rethrown(); got != nil {
		t.Errorf("a second rethrow raised %v", got)
	}
}

// TestTrapNested: a *PanicError that rethrow raises inside a nested
// callback is kept as it is, not wrapped again.
func TestTrapNested(t *testing.T) {
	resetPanics(t)
	trapped("gtk4.outer", func() {
		trapped("gtk4.inner", func() { panic("inner") })
		rethrow() // Go code in the outer callback after its native call
		t.Error("rethrow returned")
	})
	p := pendingPanic
	if p == nil || p.Value != "inner" || p.Callback != "gtk4.inner" {
		t.Fatalf("pendingPanic = %+v, want the inner panic unchanged", p)
	}
	if p.Unwrap() != nil {
		t.Error("a non-error value unwraps to an error")
	}
}

// TestTrapHookPanics: a panicking onPanic hook does not escape trap, and
// the first panic stays pending.
func TestTrapHookPanics(t *testing.T) {
	resetPanics(t)
	onPanic = func() { panic("hook") }
	trapped("gtk4.cb", func() { panic("first") })
	if pendingPanic == nil || pendingPanic.Value != "first" {
		t.Errorf("pendingPanic = %+v, want the first panic", pendingPanic)
	}
}
