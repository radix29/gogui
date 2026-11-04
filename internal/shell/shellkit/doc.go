// Package shellkit holds the logic the native shells share and that does
// not touch the OS (PLAN.md S4.1a): click counting, mapping event times
// onto the clock, the phases of precise scroll gestures, points ↔ pixels,
// size limits, centering and chrome-region hit-testing. The shells
// (shell/win32, shell/gtk4) feed it what their OS reports, so its tests
// run on every host.
//
// The package uses only the standard library and the module's internal
// driver and shell packages. It has no build tags.
package shellkit
