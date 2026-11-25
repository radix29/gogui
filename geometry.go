package gogui

import "github.com/radix29/gogui/internal/driver"

// Geometry is in logical points with a top-left origin, as in flipped
// AppKit views. DPI scaling happens inside the platform layer.

// Point is a position in logical points, X and Y; Y grows downwards.
// p.Add(q) returns p moved by q, p.Sub(q) p moved by -q.
type Point = driver.Point

// Size is a width W and a height H in logical points. IsZero reports
// whether both are zero and Empty whether the area is zero; Min and Max
// are component-wise; Inset(in) shrinks the size by insets (never below
// zero) and Outset(in) grows it.
type Size = driver.Size

// Rect is an origin X, Y (the top-left corner) and a size W, H in logical
// points. Origin, Size, MaxX and MaxY return its parts and edges;
// Contains(p) reports whether p is inside (the left and top edges are
// inside, the right and bottom ones outside); Empty reports a zero area;
// Inset(in) shrinks it by insets, Offset(d) moves it, and Intersect(o)
// returns the overlap with o (the zero Rect when there is none).
type Rect = driver.Rect

// Insets are the distances Top, Left, Bottom and Right from the four edges
// of a rectangle, used for padding. Horizontal returns Left+Right,
// Vertical Top+Bottom, and Add the edge-wise sum of two insets.
type Insets = driver.Insets

// UniformInsets returns insets of v on every edge.
func UniformInsets(v float64) Insets { return driver.UniformInsets(v) }
