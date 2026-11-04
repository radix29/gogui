// Package driver defines the boundary between the public gogui API and the
// platform backends (PLAN.md §2.1, §4.3).
//
// It holds two kinds of declarations:
//
//   - Shared value types: geometry, colors, fonts, appearance, control sizes,
//     keys and shortcuts, and the specs passed to drivers (AppConfig,
//     WindowConfig, MenuSpec, AlertSpec).
//   - Peer interfaces in the AWT/SWT style: one peer per public widget
//     (Window, Button, TextField …), the Driver factory that creates them, and
//     handler interfaces through which a backend reports user and OS events
//     back to the public layer.
//
// The public gogui package talks only to these interfaces. The backends are
// the appkit driver (macOS), the aqua driver (Windows, Linux) and the
// headless driver (tests, and the default until a native driver exists).
// Package stub provides a no-op implementation of every peer.
//
// Contracts shared by every backend:
//
//   - Units are logical points with a top-left origin. A widget frame is
//     relative to the content area of its parent (container, box, scroll
//     view document or window).
//   - Every peer method and every handler call happens on the UI thread. The
//     only exception is Driver.Post, which may be called from any goroutine.
//   - Setters that mirror a value (SetText, SetValue, SetState, SetSelected …)
//     never call a handler. Handlers report changes made by the user or the
//     OS, plus the few consequences documented on a method (Window.Close
//     calls WindowHandler.Closed).
//   - Peers have getters only for state the user can change (Text, Value,
//     State, Selected, ContentSize, Offset …). Everything else is remembered
//     by the public layer.
//   - Layout is done in Go (internal/layout) for every driver. Peers report
//     their intrinsic size through Widget.FittingSize and receive frames
//     through Widget.SetFrame.
//   - Peers never store Go pointers in native memory (PLAN.md §7.2).
//   - Text indices (TextView selection) are byte offsets into the UTF-8
//     text. Drivers convert to native units (UTF-16 on macOS and Windows).
//
// The package uses only the standard library.
package driver
