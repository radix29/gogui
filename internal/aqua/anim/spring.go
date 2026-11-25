package anim

import (
	"math"
	"time"

	"github.com/radix29/gogui/internal/aqua/theme"
)

// Spring is a damped spring: Mass, Stiffness and Damping as in
// CASpringAnimation, the same values as theme.Spring, so a token converts
// with Spring(th.Animation.ControlSpring). All three must be positive and
// finite.
type Spring theme.Spring

// Motion is a spring released at From with the velocity V0 (units per
// second) and pulled to To. Its position and velocity are closed-form
// functions of the time since the release (At): no integration and no
// accumulated error, so a late frame lands exactly where a punctual one
// would have.
//
// The regime follows from the damping ratio ζ: below 1 the motion
// oscillates around To with a decaying amplitude (underdamped), at 1 it
// returns as fast as possible without overshooting from rest (critically
// damped), above 1 it creeps back (overdamped). With a large enough
// velocity towards To, a critically damped or overdamped motion still
// crosses To once.
type Motion struct {
	From, To, V0 float64

	spring Spring
	w0     float64 // undamped angular frequency √(k/m)
	a      float64 // decay rate ζ·ω₀
	d      float64 // 1 − ζ²: > 0 underdamped, 0 critical, < 0 overdamped
	w      float64 // ω₀·√|1 − ζ²|
}

// Motion returns the motion of s released at from with the velocity v0
// (units per second), pulled to to. It panics when a spring value is not
// positive and finite.
func (s Spring) Motion(from, to, v0 float64) Motion {
	for _, v := range [...]float64{s.Mass, s.Stiffness, s.Damping} {
		if !(v > 0) || math.IsInf(v, 1) {
			panic("anim: spring mass, stiffness and damping must be positive and finite")
		}
	}
	w0 := math.Sqrt(s.Stiffness / s.Mass)
	zeta := theme.Spring(s).DampingRatio()
	d := 1 - zeta*zeta
	return Motion{
		From: from, To: to, V0: v0,
		spring: s,
		w0:     w0,
		a:      zeta * w0,
		d:      d,
		w:      w0 * math.Sqrt(math.Abs(d)),
	}
}

// Spring returns the spring the motion runs on.
func (m Motion) Spring() Spring { return m.spring }

// DampingRatio returns the spring's damping ratio ζ.
func (m Motion) DampingRatio() float64 { return m.a / m.w0 }

// At returns the position and the velocity (units per second) at the time
// t after the release. t ≤ 0 gives From and V0.
func (m Motion) At(t time.Duration) (x, v float64) {
	if t <= 0 {
		return m.From, m.V0
	}
	c, s := m.basis(t.Seconds())
	x0 := m.From - m.To
	x = m.To + x0*c + (m.V0+m.a*x0)*s
	v = m.V0*c - (m.w0*m.w0*x0+m.a*m.V0)*s
	return x, v
}

// basis returns the two functions C(t) and S(t) that make the displacement
// x0·C + (v0 + a·x0)·S and the velocity v0·C − (ω₀²·x0 + a·v0)·S in every
// regime (a = ζω₀, ω = ω₀√|1 − ζ²|):
//
//	underdamped  C = e^(−at)·cos(ωt)   S = e^(−at)·sin(ωt)/ω
//	critical     C = e^(−at)           S = t·e^(−at)
//	overdamped   C = e^(−at)·cosh(ωt)  S = e^(−at)·sinh(ωt)/ω
//
// Near the critical ratio ω is tiny: sin(ωt)/ω does not cancel, and the
// overdamped pair is computed from e^(−(a−ω)t) and Expm1, so it neither
// cancels nor overflows for any t.
func (m Motion) basis(t float64) (c, s float64) {
	switch {
	case m.d > 0:
		e := math.Exp(-m.a * t)
		sin, cos := math.Sincos(m.w * t)
		return e * cos, e * sin / m.w
	case m.d < 0:
		e := math.Exp(-(m.a - m.w) * t) // a > ω, so this decays
		q := math.Expm1(-2 * m.w * t)   // e^(−2ωt) − 1, in −1..0
		return e * (1 + q/2), -e * q / (2 * m.w)
	default:
		e := math.Exp(-m.a * t)
		return e, t * e
	}
}

// Settled reports whether the motion is at rest at the time t within eps:
// the largest displacement from To the motion can still reach from its
// state at t is at most eps. A damped spring's energy never grows, so the
// bound holds for every later time too: once Settled is true it stays
// true, and an animation can jump to To without a visible step (its speed
// does not matter, only where it can still go).
func (m Motion) Settled(t time.Duration, eps float64) bool {
	x, v := m.At(t)
	return m.settledAt(x, v, eps)
}

// settledAt is Settled for the state (x, v) the motion has at some time,
// so a frame that has sampled it does not sample it again.
func (m Motion) settledAt(x, v, eps float64) bool {
	x -= m.To
	// With the energy E = ½kx² + ½mv², the reachable displacement is
	// √(2E/k) = √(x² + v²/ω₀²).
	return math.Sqrt(x*x+v*v/(m.w0*m.w0)) <= eps
}

// Peak returns the largest distance from To the motion reaches at any
// time t ≥ 0: |From − To|, or the displacement at its first turn (where
// the velocity is 0) if that is farther. In every regime the later turns
// of a damped spring are closer, so this is the overshoot bound.
func (m Motion) Peak() float64 {
	x0, v0 := m.From-m.To, m.V0
	peak := math.Abs(x0)
	if t, ok := m.firstTurn(); ok {
		c, s := m.basis(t)
		peak = math.Max(peak, math.Abs(x0*c+(v0+m.a*x0)*s))
	}
	return peak
}

// firstTurn returns the first time t > 0 (in seconds) at which the
// velocity v0·C − K·S, K = ω₀²·x0 + a·v0, is 0, and whether there is one.
func (m Motion) firstTurn() (float64, bool) {
	x0, v0 := m.From-m.To, m.V0
	k := m.w0*m.w0*x0 + m.a*v0
	switch {
	case m.d > 0:
		// v ∝ v0·cos(ωt) − (K/ω)·sin(ωt), zero at ωt = φ + nπ.
		th := math.Mod(math.Atan2(v0*m.w, k), math.Pi)
		if th <= 0 {
			th += math.Pi
		}
		return th / m.w, true
	case m.d < 0:
		// v ∝ v0·cosh(ωt) − (K/ω)·sinh(ωt): tanh(ωt) = v0·ω/K.
		if k == 0 {
			return 0, false
		}
		if r := v0 * m.w / k; r > 0 && r < 1 {
			return math.Atanh(r) / m.w, true
		}
		return 0, false
	default:
		// v ∝ v0 − K·t.
		if k == 0 {
			return 0, false
		}
		if t := v0 / k; t > 0 {
			return t, true
		}
		return 0, false
	}
}
