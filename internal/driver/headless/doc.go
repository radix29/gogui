// Package headless is the display-less driver (PLAN.md §11.1, S0.5).
//
// It is the driver gogui selects on every platform until a native driver
// exists there, and the driver the gogui tests run on. Its peers are the
// stubs from package stub, so nothing is ever shown: widgets record their
// state, and tests simulate input through the stubs' User… methods.
//
// stub.Driver has no run loop; the headless driver adds one. Its Run loop
// drains a mainthread.Queue, the same queue the native drivers use.
// Driver.Queue exposes it, so gogui.Do and gogui.DoSync can forward to it.
package headless
