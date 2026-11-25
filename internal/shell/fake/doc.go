// Package fake is the test shell of the aqua driver (PLAN.md S2.1d): an
// implementation of the internal/shell interfaces without an OS.
//
//   - Shell runs posted work from a mainthread.Queue. Run blocks until Quit,
//     like a native loop; Start calls Handler.Ready and returns, and Step
//     then drains the queue without blocking, so a test can drive the shell
//     from its own goroutine. Quit is idempotent and, as on the real
//     shells, marks the shell quitting before it closes the surfaces, so
//     NewSurface (also from a Closed handler) returns ErrQuit.
//   - Surface records its state (title, sizes, scale, visibility, key
//     state, cursor, chrome regions) and paints into a record.Canvas when a
//     frame runs. Its input methods (Move, Click, Type, Scroll …) turn
//     simple calls into the event sequences a real shell delivers, with
//     enter/leave, capture, click counts and chrome-region hit-testing. A
//     press into a surface that is not key activates it first and is
//     flagged shell.PointerEvent.Activating (S6.1b).
//   - Clock is a manual frame clock: Advance moves time and runs the
//     requested frames at fixed multiples of FrameInterval; Flush runs one
//     requested frame at once.
//   - TextSystem lays out text with the fixed-width metrics of driver/stub
//     (7 pt per rune and 16 pt per line at 13 pt, scaled by the font size),
//     wrapping at spaces, so aqua measurements match the stub's.
//   - The clipboard is a string (SetClipboard, Clipboard); ReadClipboardText
//     delivers on the next Step, as the real shells deliver through their
//     loops.
//   - After's one-shot timers live on the Clock: Advance fires the ones due
//     by its end in deadline order, between the frames; Timers lists them.
//   - The IME is a recorder and a driver: SetTextInput and
//     CancelComposition are recorded (TextInput, Cancels), and Preedit,
//     EndPreedit and CommitPreedit send the events of a composition while
//     a text input is enabled.
//   - Popups (S5.5a) are placed by shellkit.PlacePopup in a settable work
//     area (SetWorkArea, in screen points, with the parent at its
//     UserMove position) and painted after the surfaces. A grab popup gets
//     its parent's input through the parent's input methods: pointer events
//     in its coordinates, the parent's capture cancelled when it opens, an
//     outside press eaten and dismissing it, keys, text and preedits routed
//     to it; deactivation, a move, a resize, a minimize, CancelCapture and
//     UserDismiss dismiss it. A popup without Grab takes nothing.
//     SetDeferPlaced posts Placed (coalesced) instead of calling it inside
//     NewPopup and Reposition, as gtk4 on Wayland; a popup waiting for it
//     is not painted.
//   - Modal surfaces (S5.7d, modal.go) block the others from their first
//     Show until they close: a blocked surface drops its input, its grab
//     popup and capture end, Activate and Show make the modal key, and
//     UserMove, UserResize and UserClose are refused. The modal is
//     centered on its owner or the work area; a closing modal leaves
//     modality before its Closed (as the real shells) and hands the key on
//     after it (as gtk4), and closing the owner closes it first.
//
// Like the real shells, every method of Shell, Surface and the shell's
// Clock must be called on the UI goroutine (the one that called Run or
// Start), except Shell.Post. The fake checks this and panics on a
// violation, and it panics on other contract violations too (resizing,
// showing or closing a surface during Paint, ModPrimary in an event).
//
// The package uses only the standard library and the module's internal
// driver, driver/stub (metrics), mainthread, render, render/record, shell and
// shell/shellkit (popup placement) packages. It has no build tags.
package fake
