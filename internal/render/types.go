package render

import "github.com/radix29/gogui/internal/driver"

// Geometry and color types are shared with internal/driver (PLAN.md S2.1a):
// layout and the aqua elements work in float64 logical points, and only the
// backends narrow to float32.
type (
	// Point is a position in logical points, Y growing downwards.
	Point = driver.Point
	// Size is a width and a height in logical points.
	Size = driver.Size
	// Rect is an axis-aligned rectangle with a top-left origin.
	Rect = driver.Rect
	// Color is an sRGB color with straight alpha. A canvas only accepts
	// concrete colors; semantic ones are resolved by the theme first.
	Color = driver.Color
)

// Radii are the corner radii of a rounded rectangle, one per corner, in
// logical points. The zero value is a sharp rectangle. Radii.Clamped fits
// them to a rect.
type Radii struct {
	TopLeft, TopRight, BottomRight, BottomLeft float64
}

// UniformRadii returns radii of r on every corner.
func UniformRadii(r float64) Radii { return Radii{r, r, r, r} }

// IsZero reports whether every corner is sharp (zero, negative or NaN).
func (rd Radii) IsZero() bool {
	return length(rd.TopLeft) == 0 && length(rd.TopRight) == 0 &&
		length(rd.BottomRight) == 0 && length(rd.BottomLeft) == 0
}

// IsUniform reports whether all four corners have the same radius, so a
// backend can use its single-radius call (D2D FillRoundedRectangle).
func (rd Radii) IsUniform() bool {
	return rd.TopLeft == rd.TopRight && rd.TopRight == rd.BottomRight &&
		rd.BottomRight == rd.BottomLeft
}

// Image is a bitmap a canvas can draw. Decoding and the backends' image
// types come in later steps; for now only the natural size is known.
type Image interface {
	// Size returns the natural size of the image in logical points.
	Size() Size
}
