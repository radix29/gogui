// Package anim is the animation engine of the drawn aqua driver (PLAN.md
// §5.5): cubic-bezier timing curves (Curve), damped springs in closed form
// (Spring, Motion) and linear interpolation (Lerp), all pure; and floats
// animated by them (Value: tweens, delayed tweens, springs released with
// the current or an explicit velocity, exponential momentum decays that
// end at a rest position, and repeating 0…1 loops) on a Timeline that the
// frame clock ticks. The aqua Driver owns one Timeline and installs its
// Tick as the frame function. The token values (theme.Animation) come from
// the theme.
//
// The package imports only internal/aqua/theme, internal/driver (its value
// types) and the standard library; it has no build tags.
package anim
