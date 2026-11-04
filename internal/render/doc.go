// Package render holds the drawing vocabulary of the aqua driver: the value
// types that the aqua elements hand to a Canvas, the Canvas interface itself,
// and the TextSystem and TextLayout interfaces the shell provides (PLAN.md
// §5.3).
//
// The backends are Direct2D/DirectWrite (shell/win32) and GSK/Pango
// (shell/gtk4), plus a recording backend for tests (render/record).
//
// Conventions shared by every type in the package:
//
//   - Geometry reuses driver.Point, driver.Size and driver.Rect: float64
//     logical points with a top-left origin, Y growing downwards. Only the
//     backends narrow to float32 at the native call (D2D FLOAT, graphene).
//   - Colors are driver.Colors with straight (not premultiplied) alpha.
//     Semantic colors are resolved by the theme before painting; a semantic
//     color reaching a canvas is a bug (IsSemantic on Paint and Shadow lets a
//     backend or the recorder detect it).
//   - Angles are radians, measured from the +X axis towards +Y, so a
//     positive angle turns clockwise on screen.
//   - Values are plain data and may be built as struct literals. The
//     Normalized methods (and Radii.Clamped) apply the validation rules,
//     which live in one place (validate.go): negative or NaN lengths (stroke
//     widths, blur, radii) count as 0, radii are clamped to half the shorter
//     side of their rect (the CSS/D2D rule), and gradient stops are clamped
//     to [0, 1] and sorted, and layer opacities are clamped to [0, 1]. A
//     canvas applies them itself, so callers may pass values as built.
//   - Text indices are UTF-8 byte offsets, as for TextView.
//
// The package uses only the standard library and internal/driver.
package render
