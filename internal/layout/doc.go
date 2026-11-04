// Package layout is the layout engine shared by every driver (PLAN.md
// §11.1, S0.4). It computes the frames of a tree of nodes: leaves that wrap
// widgets, stacks, spacers, forms, boxes and scroll views. Peers only report
// FittingSize and receive SetFrame; all layout decisions are made here, in
// Go, so every platform lays out the same tree the same way.
//
// # Coordinate model
//
// All values are logical points with a top-left origin (driver.Point,
// driver.Size, driver.Rect). The frame passed to Node.Arrange is final: the
// parent has already resolved the node's alignment inside its slot (place).
// A frame is relative to the content area of the nearest ancestor that owns
// a peer. A stack or form without a peer is a transparent group: it offsets
// its children's frames by its own origin instead of starting a new
// coordinate space. The root node always gets (0, 0, contentSize).
//
// # Measuring
//
// Node.Measure returns the size a node wants within max: its natural size,
// shrunk towards its minimum when max is smaller, but never below its
// minimum. The result can therefore be larger than max (overflow). Measuring
// with the main axis unbounded gives the natural size, and measuring with the
// main axis at 0 gives the minimum, so nodes have no separate MinSize method.
// Nodes measure their children only through Pass.Measure, which is where the
// per-pass measure cache lives (pass.go). Root.MinSize gives the smallest
// content size a tree fits in. Stacks and forms share space along
// their main axis with distribute (S0.4b).
//
// # Size policies
//
//   - An unset max is +Inf. A max of 0 is a real value.
//   - Negative and NaN values count as 0. A min of +Inf counts as unset.
//   - Min wins over max. A fixed size sets min and max to the same value.
//   - A leaf's default min is its fitting size (FittingSize(Unbounded)), so
//     controls don't shrink unless the app sets a smaller min. A max below
//     the fitting size therefore only has an effect together with a min.
//   - Hidden nodes are neither measured nor framed, and they get no spacing.
//   - When space is short, the content overflows at the trailing edge; the
//     engine never clips and never places a node before its slot's origin.
//
// # Rounding
//
// The engine works in fractional logical points; sizes are never rounded to
// whole points. Drivers snap edges (X and MaxX rounded separately), never
// sizes, so no gaps appear between neighbours.
//
// # Direction
//
// Stacks and forms carry a LayoutDirection. RightToLeft is accepted but laid
// out as LeftToRight for now (PLAN.md §7.4).
//
// The package uses only the standard library and internal/driver (for the
// geometry types). Nodes are used on the UI thread only.
package layout
