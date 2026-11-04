// Package stub provides a no-op implementation of every peer interface in
// package driver (PLAN.md §11.1, "Stub peers").
//
// Each stub stores the state last set through its peer methods in exported
// fields, so tests can inspect it. Methods named User… simulate user input:
// they update the state the way the real control would and call the peer's
// handler. Like real controls, the User… methods of controls do nothing
// while the control is disabled. Setters never call a handler.
//
// The stubs serve three purposes:
//
//   - The headless driver (S0.5) is built from them.
//   - The appkit and aqua drivers embed them as fallbacks, so a driver
//     compiles before all of its controls exist.
//   - Tests of the public gogui layer drive them through the User… methods.
//
// FittingSize returns fake but deterministic metrics (see metrics.go), so
// layout results in tests do not depend on fonts. A non-zero Base.Fixed
// overrides them.
//
// Stubs are not safe for concurrent use, except Driver.Post, which may be
// called from any goroutine.
package stub
