// Package fake is the test shell of the aqua driver (PLAN.md S2.1d): an
// implementation of the internal/shell interfaces without an OS.
//
//   - Shell runs posted work from a mainthread.Queue. Run blocks until Quit,
//     like a native loop; Start calls Handler.Ready and returns, and Step
//     then drains the queue without blocking, so a test can drive the shell
//     from its own goroutine.
//   - Surface records its state (title, sizes, scale, visibility, key
//     state, cursor, chrome regions) and paints into a record.Canvas when a
//     frame runs. Its input methods (Move, Click, Type, Scroll …) turn
//     simple calls into the event sequences a real shell delivers, with
//     enter/leave, capture, click counts and chrome-region hit-testing.
//   - Clock is a manual frame clock: Advance moves time and runs the
//     requested frames at fixed multiples of FrameInterval; Flush runs one
//     requested frame at once.
//   - TextSystem lays out text with the fixed-width metrics of driver/stub
//     (7 pt per rune and 16 pt per line at 13 pt, scaled by the font size),
//     wrapping at spaces, so aqua measurements match the stub's.
//
// Like the real shells, every method of Shell, Surface and the shell's
// Clock must be called on the UI goroutine (the one that called Run or
// Start), except Shell.Post. The fake checks this and panics on a
// violation, and it panics on other contract violations too (resizing,
// showing or closing a surface during Paint, ModPrimary in an event).
//
// The package uses only the standard library and the module's internal
// driver, driver/stub (metrics), mainthread, render, render/record and shell
// packages. It has no build tags.
package fake
