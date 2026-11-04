//go:build windows

// Package w32 holds the raw Win32 bindings of the Windows shell (PLAN.md
// S2.3a): the procs it calls, their constants and the mirrored structs, so
// that internal/shell/win32 makes no raw syscalls of its own.
//
// golang.org/x/sys/windows has almost none of the window APIs (no MSG,
// WNDCLASSEXW, GetMessageW, DefWindowProcW, Get/SetWindowLongPtrW), so w32
// declares them itself on windows.LazyProc. Every DLL is loaded with
// windows.NewLazySystemDLL, so only System32 is searched. What x/sys has
// is reused: HWND, Handle, GetCurrentThreadId, DwmSetWindowAttribute and
// the registry package. Every proc exists on Windows 10 1809 (D7);
// DWMWA_WINDOW_CORNER_PREFERENCE is Windows 11 only, and its caller handles
// the failure.
//
// Rules:
//
//   - Every struct mirrors its C declaration field by field and has a
//     layout test (ffi.CheckLayout, ffi.CheckNoGoPointers) for amd64 and
//     arm64. Both are LLP64, so one table serves both. Fields that hold
//     native pointers are uintptr; handles are windows.HWND or
//     windows.Handle.
//   - A struct that Windows passes as an lParam address is read with
//     ffi.At[T], never with a conversion of its own (PLAN.md §7.2).
//   - A Go pointer only reaches native code as a call argument, for the
//     duration of the call. The one struct that carries a Go pointer
//     (WNDCLASSEXW's class name) has it pinned for the call by
//     RegisterClassEx.
//   - Wrappers return an error built from GetLastError (or the HRESULT)
//     where the API reports failure; errors wrap windows.Errno. The Go
//     runtime clears the thread's last error right before every syscall
//     and reads it right after, in the same assembly stub, so the value a
//     wrapper sees belongs to its own call. That makes the "returns 0
//     without failing" APIs (Get/SetWindowLongPtrW) safe: they report an
//     error only for a 0 result with a non-zero last error.
//   - The procs are declared through newProc, so a test can check that
//     every one of them resolves (TestProcsResolve).
//
// All calls that take an HWND must run on the thread that created the
// window; the package does not check that.
package w32
