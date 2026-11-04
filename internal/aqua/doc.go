// Package aqua is the drawn driver of Windows and Linux (PLAN.md §5): a
// retained tree of elements that paint themselves on a render.Canvas and run
// on top of the shell interfaces (internal/shell). It has no build tags, so
// its tests run on every OS against the fake shell and the recording canvas.
//
// It holds the element core (S2.2a), input, focus and accessibility
// (S2.2b), windows with their chrome (S2.2c), the driver with its
// Container (S2.2d), and placeholder controls (S2.2e):
//
//   - Element is the interface every drawn part implements, and Base is the
//     embeddable implementation of the tree: an ordered list of children with
//     one parent each, a frame in the parent's coordinates, hidden and
//     enabled flags, ClipsChildren, and invalidation that walks up to a root
//     callback.
//   - An Element is a layout.Item (FittingSize, SetFrame): the public layer
//     runs internal/layout over the peers and calls SetFrame, so elements
//     never measure or arrange themselves (PLAN.md §5.1).
//   - PaintTree paints a tree: each element in its own coordinates, children
//     in order (later ones on top), hidden subtrees skipped.
//   - HitTest finds the topmost visible element under a point and returns the
//     point in that element's coordinates.
//   - Router routes a window's shell input events into the tree: hover
//     enter/leave, pointer capture from press to release, scroll events to
//     the element under the pointer, key and text events to the focused
//     element, all bubbling up until handled and skipping disabled
//     elements. It also owns the focus (one element per window, Tab and
//     Shift-Tab in tree order) and drops focus, hover and capture when
//     their element is removed, hidden or disabled.
//   - AccessibilityTree builds the a11y.Node tree of a window from each
//     element's AccessibleNode (no screen-reader bridge yet, D4).
//   - Window is a shell.Surface plus a root element: the content area and,
//     for titled windows, the 28 pt title bar with the traffic lights and the
//     centered, truncated title. It implements driver.Window in content
//     coordinates and sets the chrome regions for the OS hit-tests. Colors,
//     chrome metrics and the title font come from the theme tokens
//     (aqua/theme, theme.Sequoia; colors.go).
//   - Driver implements driver.Driver on a shell.Shell: Run, Quit and Post
//     through its own mainthread.Queue (the wake posts one Drain through
//     Shell.Post), the appearance (the forced mode merged over the OS one),
//     windows, and a recorded menu bar and alerts. Container hosts the
//     elements of its child widgets (ElementOf).
//   - The other factories return placeholder peers: each embeds its
//     driver/stub peer (state, User… simulations, fake metrics, so layouts
//     equal the headless ones) and owns an element that draws a labelled
//     rounded rectangle with an a11y node. S3.3 onward replace them.
//
// Everything in the package is used on the UI thread only and is not safe for
// concurrent use, except Driver.Post and Driver.Queue.
//
// The package uses only the standard library and the module's internal
// driver, driver/stub, layout, mainthread, render and shell packages, plus
// its own a11y and theme packages; the tests also use render/record and
// shell/fake.
package aqua
