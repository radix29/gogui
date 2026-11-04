package gogui

import "github.com/radix29/gogui/internal/driver"

// Geometry is in logical points with a top-left origin, as in flipped
// AppKit views. DPI scaling happens inside the platform layer.

// Point is a position in logical points; Y grows downwards.
type Point = driver.Point

// Size is a width and a height in logical points.
type Size = driver.Size

// Rect is an origin and a size in logical points.
type Rect = driver.Rect

// Insets are distances from the four edges of a rectangle, used for
// padding.
type Insets = driver.Insets

// UniformInsets returns insets of v on every edge.
func UniformInsets(v float64) Insets { return driver.UniformInsets(v) }
