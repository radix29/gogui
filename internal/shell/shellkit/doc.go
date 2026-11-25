// Package shellkit holds the logic the native shells share and that does
// not touch the OS (PLAN.md S4.1a): click counting, mapping event times
// onto the clock, the phases of precise scroll gestures, points ↔ pixels,
// size limits, centering, chrome-region hit-testing, popup placement
// (PlacePopup, S5.5a), and the shared shell semantics of review R6: the
// After timers and their round rule (Timers), clipboard text sanitizing
// (ClipboardText) and the fixed-width fallback text system (FixedText).
// The shells (shell/win32, shell/gtk4) feed it what their OS reports, and
// shell/fake uses the same code, so the test shell runs what ships. Its
// tests run on every host.
//
// The package uses only the standard library and the module's internal
// driver, driver/stub, render and shell packages. It has no build tags.
package shellkit
