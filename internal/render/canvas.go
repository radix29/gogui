package render

// Canvas is what the aqua elements draw on (PLAN.md §5.3). The shell hands one
// to SurfaceHandler.Paint for each frame; the backends are Direct2D
// (shell/win32), GSK (shell/gtk4) and the recorder (render/record).
//
// A canvas is used on the UI thread during one paint call only and is not
// safe for concurrent use.
//
// # State
//
// A canvas has a current transform, clip and opacity. At the start of a paint
// the transform is the identity in logical points with a top-left origin (the
// backend applies the surface scale underneath, invisibly to callers), the
// clip is the whole surface and the opacity is 1.
//
//   - Save pushes the state and Restore pops it. PushOpacity and
//     PushBackdropBlur also push the state, and Pop restores it, so a
//     transform or clip set inside a layer ends with the layer.
//   - Save/Restore and Push*/Pop pairs nest strictly with each other: Restore
//     must close a Save and Pop a Push*. Every pair is closed before the paint
//     returns. Breaking these rules is a bug that the recorder reports;
//     backends ignore the offending call.
//   - Translate, Scale and Concat act in the current user space (the new
//     transform applies first, Transform.Pre).
//   - ClipRoundedRect intersects the clip with a rounded rect in user space.
//
// # Values
//
// Callers may pass values as built; the canvas applies the validation rules
// of validate.go itself (Radii.Clamped against the rect, StrokeWidth,
// Paint.Normalized, StrokeStyle.Normalized, Shadow.Normalized, Opacity,
// BlurRadius). Colors must be concrete: a semantic color reaching a canvas is
// a bug (the recorder reports it, backends draw nothing for that call).
// Non-finite arguments to Translate, Scale and Concat are ignored (and
// reported by the recorder).
//
// # Drawing
//
// Strokes are centered on the geometry, so a crisp 1 pt border inside a rect
// is stroked on the rect inset by 0.5 pt. Paths are filled with the nonzero
// winding rule, open subpaths closed implicitly. A nil path is empty.
type Canvas interface {
	// Save pushes the current transform, clip and opacity.
	Save()
	// Restore pops the state pushed by the matching Save.
	Restore()

	// Translate moves the user space by (dx, dy).
	Translate(dx, dy float64)
	// Scale scales the user space by (sx, sy) around its origin.
	Scale(sx, sy float64)
	// Concat applies t in the current user space (t first, then the current
	// transform).
	Concat(t Transform)

	// ClipRoundedRect intersects the clip with r rounded by radii. Zero radii
	// clip to a sharp rect.
	ClipRoundedRect(r Rect, radii Radii)

	// FillRoundedRect fills r rounded by radii with p (solid or gradient).
	FillRoundedRect(r Rect, radii Radii, p Paint)
	// StrokeRoundedRect strokes the outline of r rounded by radii, centered on
	// the outline, width wide.
	StrokeRoundedRect(r Rect, radii Radii, width float64, p Paint)
	// FillPath fills path (nonzero rule) with p.
	FillPath(path *Path, p Paint)
	// StrokePath strokes path, width wide, with p and the style s.
	StrokePath(path *Path, width float64, p Paint, s StrokeStyle)
	// DrawShadow draws the shadow of r rounded by radii, not the shape itself:
	// an outer shadow only outside the shape, an inset one only inside it
	// (CSS box-shadow).
	DrawShadow(r Rect, radii Radii, s Shadow)
	// DrawText draws a layout made by the shell's TextSystem with its top-left
	// corner at origin (the first baseline is at origin.Y + l.Baseline()), in
	// color.
	DrawText(l TextLayout, origin Point, color Color)
	// DrawImage draws img scaled to dst.
	DrawImage(img Image, dst Rect)

	// PushOpacity starts a layer: everything drawn until the matching Pop is
	// composed first and then drawn with alpha (clamped to [0, 1]) as group
	// opacity. It also saves the state, like Save.
	PushOpacity(alpha float64)
	// PushBackdropBlur blurs what is already drawn inside r rounded by radii
	// by radius (the Gaussian's standard deviation is radius/2, as for
	// Shadow.Blur), then clips everything drawn until the matching Pop to that
	// shape (menus, sidebars). It also saves the state, like Save.
	PushBackdropBlur(r Rect, radii Radii, radius float64)
	// Pop ends the layer started by the matching Push* and restores the state
	// saved there.
	Pop()
}
