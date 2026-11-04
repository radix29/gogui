// Package scroll holds the scroll physics of the drawn aqua driver (PLAN.md
// §5.6). The pure one-dimensional math: the rubber band past the edges
// (RubberBand and its inverse RubberBandInverse), the exponential decay of
// momentum after a release (Decay) and the release-velocity estimate from
// the deltas of a gesture (VelocityTracker). On top of it, Axis (S3.5c) is
// the offset of one scrolling axis on the frame clock (an anim.Timeline):
// precise drags with rubber-banding, momentum and the spring-back, OS
// momentum, smooth wheel steps, and Reduce Motion. The ScrollView runs one
// Axis per scrolling axis.
//
// Positions and distances are in logical points, velocities in points per
// second, times are time.Duration on the frame clock (shell.ScrollEvent.Time).
// The token values (decay rate, stop velocity, velocity window, rubber-band
// constant, line height, wheel step, spring) come from the theme
// (ThemeParams); the math takes them as arguments.
//
// The package imports anim, theme and shell; it has no build tags.
package scroll
