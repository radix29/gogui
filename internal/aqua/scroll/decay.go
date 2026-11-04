package scroll

import (
	"math"
	"time"
)

// Decay is momentum after a release (PLAN.md §5.6): the content leaves with
// the velocity V0 (points per second) and slows down exponentially at the
// rate K (per second), v(t) = V0·e^(−K·t), so the distance travelled is
// x(t) = V0/K·(1 − e^(−K·t)). The motion stops when the speed falls to the
// stop velocity: at Duration, at the offset Rest from the release point.
// After that At reports Rest and zero velocity, so the motion ends exactly
// at Rest instead of creeping on towards the asymptote V0/K.
//
// Positions are offsets from the release point; the caller adds them to
// the offset at the release.
type Decay struct {
	V0 float64 // velocity at the release, points per second
	K  float64 // decay rate, per second

	stop     time.Duration // Duration
	stopSecs float64       // the same in seconds, unrounded
	rest     float64       // Rest
}

// NewDecay returns the decay of the release velocity v0 (points per second)
// at the rate k (per second), stopping when the speed falls to vs (points
// per second). k must be positive and finite, or NewDecay panics, as an
// invalid anim.Spring does: a rate of zero would never slow down.
//
// The stop time is ln(|v0|/vs)/k, and 0 when |v0| ≤ vs: such a release has
// no momentum, and its Rest is 0. A vs that is not positive (or NaN) means
// no stop velocity: the motion runs to the asymptote, Duration is the
// largest time.Duration and Rest is V0/K. A Duration too long for a
// time.Duration is capped likewise; Rest is then still the exact position
// at the stop velocity.
func NewDecay(v0, k, vs float64) Decay {
	if !(k > 0) || math.IsInf(k, 1) {
		panic("scroll: decay rate must be positive and finite")
	}
	d := Decay{V0: v0, K: k}
	speed := math.Abs(v0)
	switch {
	case !(vs > 0):
		d.stopSecs = math.Inf(1)
		d.stop = math.MaxInt64
		d.rest = v0 / k
	case speed <= vs:
		// No momentum: stop, stopSecs and rest stay 0.
	default:
		d.stopSecs = math.Log(speed/vs) / k
		if s := d.stopSecs * float64(time.Second); s < math.MaxInt64 {
			d.stop = time.Duration(s)
		} else {
			d.stop = math.MaxInt64
		}
		// x(T) with e^(−K·T) = vs/|V0|: the speed shed, over K.
		d.rest = math.Copysign(speed-vs, v0) / k
	}
	return d
}

// Duration returns the stop time: how long after the release the speed
// reaches the stop velocity.
func (d Decay) Duration() time.Duration { return d.stop }

// Rest returns the offset from the release point at which the motion
// stops: the position at Duration.
func (d Decay) Rest() float64 { return d.rest }

// Asymptote returns V0/K, the offset the motion would approach if it never
// stopped. |Rest| is smaller by vs/K.
func (d Decay) Asymptote() float64 { return d.V0 / d.K }

// Done reports whether the motion has stopped at the time t after the
// release.
func (d Decay) Done(t time.Duration) bool { return t >= d.stop }

// At returns the offset from the release point and the velocity (points
// per second) at the time t after the release. t ≤ 0 gives 0 and V0
// (or 0 and 0 for a release with no momentum); from Duration on it gives
// Rest and 0.
func (d Decay) At(t time.Duration) (x, v float64) {
	if d.Done(t) {
		return d.rest, 0
	}
	if t <= 0 {
		return 0, d.V0
	}
	secs := t.Seconds()
	if secs >= d.stopSecs { // rounding of stop to whole nanoseconds
		return d.rest, 0
	}
	e := math.Exp(-d.K * secs)
	return d.V0 / d.K * -math.Expm1(-d.K*secs), d.V0 * e
}
