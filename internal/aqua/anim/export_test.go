package anim

import "time"

// SettleTime is test-only: the tests use it to check that Settled is
// monotonic in t and agrees with the spring tokens.

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
