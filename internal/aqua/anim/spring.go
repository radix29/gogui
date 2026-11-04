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
// both the largest displacement from To and the largest speed the motion
// can still reach from its state at t are at most eps. A damped spring's
// energy never grows, so these bounds hold for every later time too: once
// Settled is true it stays true, and an animation can jump to To.
func (m Motion) Settled(t time.Duration, eps float64) bool {
	x, v := m.At(t)
	x -= m.To
	// With the energy E = ½kx² + ½mv², the reachable displacement is
	// √(2E/k) = √(x² + v²/ω₀²) and the reachable speed √(2E/m) = √(ω₀²x² + v²).
	w2 := m.w0 * m.w0
	return math.Sqrt(x*x+v*v/w2) <= eps && math.Sqrt(w2*x*x+v*v) <= eps
}

// MaxSettleTime bounds SettleTime's search; no spring token comes near it.
const MaxSettleTime = time.Hour

// SettleTime returns the first time, to the microsecond (rounded up), from
// which the motion is Settled within eps: 0 when it starts at rest,
// MaxSettleTime when it has not settled by then. It panics when eps is not
// positive.
func (m Motion) SettleTime(eps float64) time.Duration {
	if !(eps > 0) {
		panic("anim: SettleTime needs a positive eps")
	}
	if m.Settled(0, eps) {
		return 0
	}
	lo, hi := time.Duration(0), time.Millisecond
	for !m.Settled(hi, eps) {
		if hi >= MaxSettleTime {
			return MaxSettleTime
		}
		lo, hi = hi, min(2*hi, MaxSettleTime)
	}
	// Settled is monotonic in t, so bisection finds its first true time.
	for hi-lo > time.Microsecond {
		mid := lo + (hi-lo)/2
		if m.Settled(mid, eps) {
			hi = mid
		} else {
			lo = mid
		}
	}
	return hi
}
