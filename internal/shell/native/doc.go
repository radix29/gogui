// Package native selects the shell of the current OS for the aqua driver
// (PLAN.md S2.2d, S2.3f, S4.1d, §11.1 rule 3).
//
// It is a platform package, so build tags are allowed here. It can't live in
// internal/shell, because the platform shells import shell (an import
// cycle). On Windows New returns the Win32 shell (shell/win32,
// native_windows.go, since S2.3f), on Linux the GTK 4 shell (shell/gtk4,
// native_linux.go, since S4.1d). On every other OS it returns
// ErrNotImplemented (native_other.go): macOS uses the appkit driver and
// never asks for a shell.
//
// The package uses only the standard library, internal/shell and, on
// Windows, internal/shell/win32, on Linux internal/shell/gtk4.
package native
