//go:build windows

package win32

import (
	"fmt"
	"runtime/debug"
	"sync"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// target is a window of the package: the shell's listener and the surfaces. handle returns the message's result and whether it handled the
// message; unhandled messages go to DefWindowProcW.
type target interface {
	handle(h windows.HWND, msg uint32, wp, lp uintptr) (result uintptr, handled bool)
}

// targets maps the handles in GWLP_USERDATA to their windows. The window
// procedure is one process-wide trampoline, so the table is process-wide
// too; the aqua driver creates one shell per process, and the tests run
// their shells one after another. It is used on the UI thread.
var targets ffi.Handles[target]

// className is the one window class of the package, for the listener and the
// surfaces (CS_DBLCLKS, no CS_HREDRAW|CS_VREDRAW: a resize repaints through
// the frame, not by invalidating the whole window). It is registered once
// per process and never unregistered.
const className = "gogui.win32"

var registerClass = sync.OnceValues(func() (windows.Handle, error) {
	inst, err := w32.GetModuleHandle()
	if err != nil {
		return 0, err
	}
	wc := w32.WndClassEx{
		Style:       w32.CS_DBLCLKS,
		LpfnWndProc: ffi.RegisterCallback("win32.wndproc", wndproc),
		HInstance:   inst,
	}
	if _, err := w32.RegisterClassEx(&wc, className); err != nil {
		return 0, err
	}
	return inst, nil
})

// wndproc is the window procedure of every window of className.
func wndproc(h windows.HWND, msg uint32, wp, lp uintptr) uintptr {
	if msg == w32.WM_NCCREATE {
		cs := ffi.At[w32.CreateStruct](lp)
		// Only an error for an invalid h, which can't happen here.
		_, _ = w32.SetWindowLongPtr(h, w32.GWLP_USERDATA, cs.LpCreateParams)
	}
	v, _ := w32.GetWindowLongPtr(h, w32.GWLP_USERDATA)
	t, ok := targets.Get(ffi.Handle(v))
	if !ok {
		return w32.DefWindowProc(h, msg, wp, lp)
	}
	r, handled := dispatch(t, h, msg, wp, lp)
	if msg == w32.WM_NCDESTROY {
		targets.Free(ffi.Handle(v))
		_, _ = w32.SetWindowLongPtr(h, w32.GWLP_USERDATA, 0)
	}
	if !handled {
		return w32.DefWindowProc(h, msg, wp, lp)
	}
	return r
}

// dispatch calls t, recovering a panic into pendingPanic. The message then
// counts as handled with result 0; for a panic during WM_NCCREATE that makes
// CreateWindowExW fail, which is what should happen.
func dispatch(t target, h windows.HWND, msg uint32, wp, lp uintptr) (r uintptr, handled bool) {
	defer func() {
		if v := recover(); v != nil {
			if pendingPanic == nil { // the first panic wins
				if pe, ok := v.(*PanicError); ok {
					pendingPanic = pe // re-raised by rethrow in a nested message
				} else {
					pendingPanic = &PanicError{Value: v, Msg: msg, Stack: debug.Stack()}
				}
			}
			r, handled = 0, true
		}
	}()
	return t.handle(h, msg, wp, lp)
}

// pendingPanic is the panic recovered in a window procedure that has not
// been re-raised yet. UI thread only.
var pendingPanic *PanicError

// rethrow re-raises the pending panic, if any. The message loop calls it
// after every GetMessageW and DispatchMessageW, and Go code calls it after
// every Win32 call that sends messages to the package's windows.
func rethrow() {
	if p := pendingPanic; p != nil {
		pendingPanic = nil
		panic(p)
	}
}

// PanicError is the panic value Run re-raises when a handler panicked inside
// the window procedure: in posted work, a frame function or a shell or
// surface handler.
type PanicError struct {
	// Value is the original panic value.
	Value any
	// Msg is the window message being handled (w32.WM_*, or the shell's
	// private WM_APP messages).
	Msg uint32
	// Stack is the stack of the UI thread at the time of the panic.
	Stack []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("panic in the win32 window procedure (message %#x): %v\n\n%s", e.Msg, e.Value, e.Stack)
}

// Unwrap returns the original panic value if it is an error.
func (e *PanicError) Unwrap() error {
	if err, ok := e.Value.(error); ok {
		return err
	}
	return nil
}
