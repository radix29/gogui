// Package shell is the OS integration boundary of the aqua driver (PLAN.md
// §2.1, §5): native top-level windows (surfaces), the event loop and the
// UI-thread wake, input events, text layout, the system appearance and the
// frame clock. The aqua driver draws everything inside a surface itself and
// talks to the OS only through these interfaces.
//
// The implementations are shell/win32 (Win32, DWM, Direct2D, DirectWrite),
// shell/gtk4 (GTK 4, GSK, Pango) and the test shell shell/fake (S2.1d). The
// clipboard (S5.2), IME composition (S5.3) and popups (S5.5a: Popup, a
// borderless surface owned by a parent, for menus and tooltips) extended
// the interfaces in their own steps.
//
// Contracts shared by every implementation:
//
//   - Units: every value is in logical points with a top-left origin, Y
//     growing downwards, relative to the surface's drawable area (the whole
//     client area, including the title bar the aqua driver draws). A surface
//     reports its scale (device pixels per point, Surface.Scale); the
//     backends apply it, so nothing above the shell sees device pixels.
//   - Threading: every method and every handler call happens on the UI
//     thread, the goroutine that called Shell.Run. The only exception is
//     Shell.Post, which may be called from any goroutine. Handlers may call
//     back into the shell, except where a method says otherwise.
//   - Calls the shell makes into Go are bounded: chrome hit-testing is
//     answered from the regions set with Surface.SetChromeRegions, so no Go
//     call happens per WM_NCHITTEST, and paints happen at most once per
//     frame.
//   - Modifiers in events are physical (driver.ModShift, ModControl,
//     ModOption, ModCommand); ModPrimary never appears. ModOption is Alt and
//     ModCommand the Windows/Super key.
//   - Times are durations on the frame clock's monotonic timeline
//     (FrameClock.Now), so input times and frame times compare directly.
//
// The package declares interfaces and value types only and has no behaviour.
// It uses only the standard library, internal/driver and internal/render.
package shell
