package anim

import (
	"math"
	"slices"
	"time"
)

// Clock is the part of shell.FrameClock a Timeline uses: the current time
// (the timeline animations start on) and a one-shot, coalesced request for
// the next frame. shell.FrameClock and shell/fake's Clock implement it.
type Clock interface {
	Now() time.Duration
	RequestFrame()
}

// The tolerances of a Value (SetTolerance): the largest error, in the
// value's units, that does not show. A spring that can no longer move
// farther than its value's tolerance from its target is settled (it jumps
// there and stops), and a frame that moves a running value by less than
// its tolerance from what was last shown does not call its change func.
const (
	// DefaultTolerance is NewValue's: finer than any unit shows.
	DefaultTolerance = 1e-3
	// AlphaTolerance is for alphas and 0…1 fractions that scale an alpha or
	// a few points: half an 8-bit step.
	AlphaTolerance = 1.0 / 510
	// PointTolerance is for positions in logical points: an eighth of a
	// device pixel at 4×.
	PointTolerance = 1.0 / 32
)

// Timeline runs the animated Values of one driver on a frame clock. The
// driver installs Tick as the clock's frame function; Tick advances every
// running value to the frame time, calls the change funcs and the running
// Tickers, and requests the next frame only while a value or a ticker is
// still running, so an idle app requests no frames.
//
// With Reduce Motion (SetReduceMotion) nothing animates: AnimateTo,
// SpringTo and SpringFrom jump to their target like Set, Decay jumps to its
// rest position, Loop stays at its start (0), and turning it on finishes
// the running values at once (a decay at its rest position, a loop stops
// at 0). Tickers are not motion and run on.
//
// A Timeline, its Values and its Tickers are used on the UI thread only.
type Timeline struct {
	clock   Clock
	running []*Value
	scratch []*Value // the snapshot Tick iterates

	tickers  []*Ticker // the running tickers, and those stopped during Tick
	tscratch []*Ticker // the snapshot Tick iterates

	inTick    bool
	frameTime time.Duration // of the Tick in progress
	dirty     bool          // values stopped during Tick are still listed
	reduce    bool
}

// NewTimeline returns an idle timeline on c. It panics when c is nil.
func NewTimeline(c Clock) *Timeline {
	if c == nil {
		panic("anim: NewTimeline with a nil clock")
	}
	return &Timeline{clock: c}
}

// NewValue returns a value at x that is not running. onChange, which may be
// nil, is called with the new value whenever it changes: in Tick, and
// directly from Set, from the jumps of AnimateTo, SpringTo, SpringFrom and
// Decay (no duration, Reduce Motion), from Decay without momentum (it stops
// where the running animation is at that moment), from Loop (the jump to
// its start) and from SetReduceMotion. It is typically the owning element's
// Invalidate. The value's tolerance is DefaultTolerance (SetTolerance).
func (tl *Timeline) NewValue(x float64, onChange func(x float64)) *Value {
	checkFinite(x, "NewValue")
	return &Value{tl: tl, x: x, onChange: onChange, tol: DefaultTolerance}
}

// Running returns the number of running values and tickers.
func (tl *Timeline) Running() int {
	n := 0
	for _, v := range tl.running {
		if v.kind != idle {
			n++
		}
	}
	for _, t := range tl.tickers {
		if t.on {
			n++
		}
	}
	return n
}

// ReduceMotion reports whether Reduce Motion is on.
func (tl *Timeline) ReduceMotion() bool { return tl.reduce }

// SetReduceMotion turns Reduce Motion on or off (the driver follows
// Appearance.ReduceMotion). Turning it on finishes every running value at
// once: each jumps to its target (a decay to its rest position), and a
// loop stops at its start (0); an AnimateAfter still in its delay keeps
// the delay and jumps at its end.
// Turning it off restarts nothing: a stopped loop stays at 0 until its
// owner calls Loop again.
func (tl *Timeline) SetReduceMotion(on bool) {
	if on == tl.reduce {
		return
	}
	tl.reduce = on
	if !on {
		return
	}
	now := tl.now()
	for _, v := range slices.Clone(tl.running) {
		switch v.kind {
		case idle:
		case tween:
			if v.delayed && v.start > now {
				v.dur = 0 // an AnimateAfter still in its delay: it jumps at the end of it
				continue
			}
			v.Set(v.to)
		case loop:
			v.Set(0)
		default:
			v.Set(v.to)
		}
	}
}

// Tick advances every running value to frameTime (the frame clock's target
// presentation time, so values step by whole frames even when the call is
// late): a value whose animation has ended, or whose spring has settled
// within the value's tolerance, takes its exact target and stops; a loop
// never ends (it takes its phase at frameTime). A frame that moves a value
// that runs on by less than its tolerance from its last shown value leaves
// Value at that shown value and does not call its change func (Velocity
// still follows the frame). Change funcs
// are called in the order the values started; they may start, retarget or
// stop any value, including their own. A value started during Tick starts
// at frameTime and moves from the next frame on. The running tickers are
// called after the change funcs (see Ticker). If a value or a ticker is
// still running afterwards, Tick requests the next frame. It panics when
// called from inside a Tick.
func (tl *Timeline) Tick(frameTime time.Duration) {
	if tl.inTick {
		panic("anim: Timeline.Tick called from inside a Tick")
	}
	tl.inTick, tl.frameTime = true, frameTime
	defer tl.endTick()

	tl.scratch = append(tl.scratch[:0], tl.running...)
	for _, v := range tl.scratch {
		if v.kind == idle {
			continue // stopped by an earlier change func
		}
		x, vel, done := v.sample(frameTime)
		v.vel = vel
		if done {
			v.stop()
		} else if math.Abs(x-v.x) < v.tol {
			continue // does not show
		}
		v.change(x)
	}
	clear(tl.scratch)

	tl.tscratch = append(tl.tscratch[:0], tl.tickers...)
	for _, t := range tl.tscratch {
		if t.on && !t.fresh { // not stopped by an earlier call, nor started in this Tick
			t.tick(frameTime)
		}
	}
	clear(tl.tscratch)
}

// endTick drops the values and tickers stopped during the Tick and
// requests the next frame while a value or a ticker runs. It also runs
// when a change func or a ticker panics, so the timeline stays usable.
func (tl *Timeline) endTick() {
	tl.inTick = false
	if tl.dirty {
		tl.dirty = false
		tl.running = slices.DeleteFunc(tl.running, func(v *Value) bool {
			if v.kind == idle {
				v.listed = false
				return true
			}
			return false
		})
	}
	tl.tickers = slices.DeleteFunc(tl.tickers, func(t *Ticker) bool {
		t.fresh = false
		if !t.on {
			t.listed = false
			return true
		}
		return false
	})
	if len(tl.running) > 0 || len(tl.tickers) > 0 {
		tl.clock.RequestFrame()
	}
}

// now is the start time of an animation begun now: the frame time inside a
// Tick, else the clock's Now.
func (tl *Timeline) now() time.Duration {
	if tl.inTick {
		return tl.frameTime
	}
	return tl.clock.Now()
}

// add lists a value that has started and asks for a frame (Tick does that
// itself at its end).
func (tl *Timeline) add(v *Value) {
	if !v.listed {
		v.listed = true
		tl.running = append(tl.running, v)
	}
	if !tl.inTick {
		tl.clock.RequestFrame()
	}
}

// remove unlists a value that has stopped (a running value is always
// listed); during a Tick only once it ends.
func (tl *Timeline) remove(v *Value) {
	if tl.inTick {
		tl.dirty = true
		return
	}
	v.listed = false
	i := slices.Index(tl.running, v)
	tl.running = slices.Delete(tl.running, i, i+1)
}

type kind uint8

const (
	idle kind = iota
	tween
	spring
	loop
	decay
)

// Value is a float animated on a Timeline (NewValue): by a duration and a
// timing curve (AnimateTo), by a spring released with the current velocity
// (SpringTo) or with an explicit one (SpringFrom), by momentum that slows
// down exponentially to a rest position (Decay), or as a repeating phase
// over 0…1 (Loop). Value returns what the last frame showed; between frames
// it does not move. Retargeting mid-flight starts from where the running
// animation (tween, spring, decay or loop) is at that moment, with its
// velocity for SpringTo, so a spring retargets without a kink.
type Value struct {
	tl       *Timeline
	x, vel   float64
	onChange func(float64)
	listed   bool    // in tl.running
	tol      float64 // SetTolerance

	kind     kind
	start    time.Duration
	from, to float64
	dur      time.Duration // tween; the period of a loop; a decay's stop time
	delayed  bool          // tween: started by AnimateAfter (start is the end of its delay)
	curve    Curve         // tween
	motion   Motion        // spring
	v0, rate float64       // decay: release velocity, rate per second
	stopSecs float64       // decay: the stop time in seconds, unrounded
}

// Value returns the current value: the one the last frame (or jump) set.
func (v *Value) Value() float64 { return v.x }

// Velocity returns the value's velocity in units per second at the last
// frame: 0 when it is not running, 1/period seconds for a loop (the
// wrap-around is not counted).
func (v *Value) Velocity() float64 { return v.vel }

// Target returns where the value is going: the running animation's end
// value (a decay's rest position), else the current value. A loop has no end, so its Target is the
// current value (the last frame's phase).
func (v *Value) Target() float64 {
	if v.kind == idle || v.kind == loop {
		return v.x
	}
	return v.to
}

// Running reports whether an animation runs.
func (v *Value) Running() bool { return v.kind != idle }

// Tolerance returns the value's tolerance (SetTolerance).
func (v *Value) Tolerance() float64 { return v.tol }

// SetTolerance sets the largest error of the value, in its units, that
// does not show (DefaultTolerance until then; AlphaTolerance and
// PointTolerance for the usual units): a spring settles once it can no
// longer move farther than tol from its target, and Tick does not show a
// move by less than tol. It returns v. It panics when tol is not positive
// and finite.
func (v *Value) SetTolerance(tol float64) *Value {
	if !(tol > 0) || math.IsInf(tol, 1) {
		panic("anim: Value.SetTolerance with a tolerance that is not positive and finite")
	}
	v.tol = tol
	return v
}

// Set jumps to x: any animation stops, the velocity becomes 0, and the
// change func is called if the value changed. It panics when x is not
// finite.
func (v *Value) Set(x float64) {
	checkFinite(x, "Set")
	v.stop()
	v.vel = 0
	v.change(x)
}

// Stop stops the animation where the last frame left the value, with no
// velocity (a loop keeps its last phase; Set(0) puts it back at its start).
// It does nothing when the value is not running.
func (v *Value) Stop() {
	if v.kind != idle {
		v.stop()
		v.vel = 0
	}
}

// AnimateTo animates from the current position to x over d along c: at a
// frame time t the value is Lerp(from, x, c.At((t − start)/d)), and at the
// end exactly x. The start is the clock's Now (the frame time inside a
// Tick); a running animation is retargeted from where it is at the start.
// With d ≤ 0 or Reduce Motion it jumps like Set. Animating to the target of
// the running animation, or to the value of a value at rest, does nothing,
// so calling AnimateTo on every event is cheap and does not restart the
// curve. From a loop it always starts, from the loop's phase at the start
// (the loop ends). It panics when x is not finite.
func (v *Value) AnimateTo(x float64, d time.Duration, c Curve) {
	checkFinite(x, "AnimateTo")
	if d <= 0 || v.tl.reduce {
		v.Set(x)
		return
	}
	if v.kind != loop && v.Target() == x {
		return
	}
	start := v.tl.now()
	from, _ := v.position(start)
	v.kind, v.start, v.from, v.to, v.dur, v.curve = tween, start, from, x, d, c
	v.delayed = false
	v.tl.add(v)
}

// AnimateAfter holds the value where it is for delay, then animates to x
// over d along c, as AnimateTo does from there. The value stops where the
// running animation is at the start (the clock's Now, the frame time
// inside a Tick) and keeps that position, with no velocity, until
// start + delay; Target is x from the call on. Unlike AnimateTo it always
// restarts, so calling it again restarts the delay (an overlay scroller
// re-arms its fade-out on every scroll); only a value at rest at exactly x
// does nothing. A delay ≤ 0 is AnimateTo. The delay is not motion: with
// Reduce Motion it still runs, and the value then jumps to x (d counts as
// 0). The timeline requests frames during the delay (it has no timers),
// but the change func is not called before the value moves. It panics
// when x is not finite.
func (v *Value) AnimateAfter(x float64, delay, d time.Duration, c Curve) {
	checkFinite(x, "AnimateAfter")
	if delay <= 0 {
		v.AnimateTo(x, d, c)
		return
	}
	if v.kind == idle && v.x == x {
		return
	}
	now := v.tl.now()
	from, _ := v.position(now)
	if v.tl.reduce || d < 0 {
		d = 0
	}
	v.vel = 0
	v.kind, v.start, v.from, v.to, v.dur, v.curve = tween, now+delay, from, x, d, c
	v.delayed = true
	v.change(from)
	v.tl.add(v)
}

// SpringTo pulls the value to x with the spring s, released from where it
// is at the start (the clock's Now, the frame time inside a Tick) with its
// velocity there: the last frame's velocity for a value at rest is 0, a
// running tween's or spring's comes from its curve or motion. Once the
// spring settles within the value's tolerance the value takes x exactly and stops.
// With Reduce Motion it jumps like Set. Springing to the target of the
// running animation (a decay's rest position included), or to the value of
// a value at rest, does nothing; from a loop it always starts, released at
// the loop's phase with velocity 1/period seconds. It panics when x is not
// finite or s is not valid (Spring.Motion).
func (v *Value) SpringTo(x float64, s Spring) {
	checkFinite(x, "SpringTo")
	s.Motion(0, 0, 0) // validate s even when it is not used
	if v.tl.reduce {
		v.Set(x)
		return
	}
	if v.kind != loop && v.Target() == x {
		return
	}
	start := v.tl.now()
	from, vel := v.position(start)
	v.startSpring(start, from, x, vel, s)
}

// SpringFrom is SpringTo with an explicit release velocity: the spring s
// pulls the value to x, released from where it is at the start (the
// clock's Now, the frame time inside a Tick: the running animation's
// position there, or the current value at rest) with the velocity v0
// (units per second) instead of the current one. A scroller uses it to
// spring back from an edge with the velocity the content hit it with.
//
// Unlike SpringTo it always (re)starts, even when x is the running
// animation's target, since the velocity is new; only a value at rest at
// exactly x released with v0 == 0 does nothing (it would not move). Once
// the spring settles within the value's tolerance the value takes x exactly and
// stops. With Reduce Motion it jumps to x like Set. It panics when x or v0
// is not finite or s is not valid (Spring.Motion).
func (v *Value) SpringFrom(x, v0 float64, s Spring) {
	checkFinite(x, "SpringFrom")
	checkFinite(v0, "SpringFrom")
	s.Motion(0, 0, 0) // validate s even when it is not used
	if v.tl.reduce {
		v.Set(x)
		return
	}
	if v.kind == idle && v.x == x && v0 == 0 {
		return
	}
	start := v.tl.now()
	from, _ := v.position(start)
	v.startSpring(start, from, x, v0, s)
}

// startSpring starts the spring s at start, released at from with the
// velocity vel and pulled to x.
func (v *Value) startSpring(start time.Duration, from, x, vel float64, s Spring) {
	v.kind, v.start, v.from, v.to = spring, start, from, x
	v.motion = s.Motion(from, x, vel)
	v.tl.add(v)
}

// Decay releases the value with the velocity v0 (units per second) from
// where it is at the start (the clock's Now, the frame time inside a Tick:
// the running animation's position there, or the current value at rest)
// and slows it down exponentially at the rate (per second): at the time t
// after the start the value is from + v0/rate·(1 − e^(−rate·t)) with the
// velocity v0·e^(−rate·t), in closed form from the frame time, so late
// frames land where punctual ones would have. It is the momentum of a
// flung scroll view (PLAN.md §5.6).
//
// The decay ends when the speed falls to stopVelocity, at the time
// ln(|v0|/stopVelocity)/rate: the value then takes its rest position
// from + sign(v0)·(|v0| − stopVelocity)/rate exactly and stops, instead of
// creeping on towards the asymptote from + v0/rate. Target returns the
// rest position while it runs. A release with |v0| ≤ stopVelocity has no
// momentum: Decay then stops any running animation and leaves the value
// where that animation is at the start, with no velocity (calling the
// change func if it moved).
//
// Decay always (re)starts: called while decaying, it releases the value
// again from its current position with the new v0. Retargeting a decay
// (AnimateTo, SpringTo, SpringFrom, Loop) starts from its position at that
// moment, with its velocity for SpringTo. With Reduce Motion it does not
// animate: the value jumps to the rest position like Set.
//
// It panics when v0 is not finite, when rate is not positive and finite
// (a rate of zero would never slow down), when stopVelocity is not
// positive or NaN (a decay must end, and without a stop velocity it never
// would), and when the rest position is not finite.
func (v *Value) Decay(v0, rate, stopVelocity float64) {
	checkFinite(v0, "Decay")
	if !(rate > 0) || math.IsInf(rate, 1) {
		panic("anim: Value.Decay with a rate that is not positive and finite")
	}
	if !(stopVelocity > 0) {
		panic("anim: Value.Decay with a stop velocity that is not positive")
	}
	start := v.tl.now()
	from, _ := v.position(start)
	speed := math.Abs(v0)
	if speed <= stopVelocity { // no momentum
		v.stop()
		v.vel = 0
		v.change(from)
		return
	}
	// x(T) with e^(−rate·T) = vs/|v0|: the speed shed, over the rate.
	rest := from + math.Copysign(speed-stopVelocity, v0)/rate
	if math.IsInf(rest, 0) {
		panic("anim: Value.Decay comes to rest at a position that is not finite")
	}
	if v.tl.reduce {
		v.Set(rest)
		return
	}
	stopSecs := math.Log(speed/stopVelocity) / rate
	stop := time.Duration(math.MaxInt64)
	if ns := stopSecs * float64(time.Second); ns < math.MaxInt64 {
		stop = time.Duration(ns)
	}
	v.kind, v.start, v.from, v.to = decay, start, from, rest
	v.v0, v.rate, v.stopSecs, v.dur = v0, rate, stopSecs, stop
	v.tl.add(v)
}

// Loop runs the value as a repeating phase over 0…1 that wraps around once
// per period: at a frame time t it is frac((t − start)/period), computed
// from the frame time alone (the exact remainder of the elapsed
// nanoseconds), so late frames and long runs do not drift, and always in
// [0, 1). Its velocity is 1/period seconds. A loop never ends: it runs,
// and the timeline requests frames, until Stop, Set, AnimateTo, SpringTo,
// SpringFrom or Decay, or until Reduce Motion is turned on.
//
// A loop starts at 0 (its start): Loop jumps the value to 0 at the clock's
// Now (the frame time inside a Tick), calling the change func if it moved,
// and the phase advances from the next frame on. Calling Loop with the
// period of the running loop does nothing, so owners may call it on every
// event; another period retimes the running loop from its current phase,
// without a jump. With Reduce Motion it does not run: the value jumps to 0
// like Set(0). It panics when period ≤ 0.
func (v *Value) Loop(period time.Duration) {
	if period <= 0 {
		panic("anim: Value.Loop with a period that is not positive")
	}
	if v.tl.reduce {
		v.Set(0)
		return
	}
	if v.kind == loop && v.dur == period {
		return
	}
	now := v.tl.now()
	start, phase := now, 0.0
	if v.kind == loop {
		phase, _ = v.position(now)
		start = now - time.Duration(phase*float64(period))
	}
	v.kind, v.start, v.dur = loop, start, period
	v.from, v.to = 0, 0 // unused by a loop
	v.vel = 1 / period.Seconds()
	v.change(phase)
	v.tl.add(v)
}

// position returns where the value is at t: the running animation's
// position and velocity, or the current value and velocity at rest.
func (v *Value) position(t time.Duration) (x, vel float64) {
	if v.kind == idle {
		return v.x, v.vel
	}
	x, vel, _ = v.sample(t)
	return x, vel
}

// sample returns the running animation's position and velocity at the
// frame time t and whether it has ended there (then x is the exact target
// and vel 0). The value must be running.
func (v *Value) sample(t time.Duration) (x, vel float64, done bool) {
	e := max(t-v.start, 0)
	switch v.kind {
	case loop:
		return loopPhase(e, v.dur), 1 / v.dur.Seconds(), false
	case tween:
		if v.delayed && t < v.start {
			return v.from, 0, false // AnimateAfter's delay
		}
		if e >= v.dur {
			return v.to, 0, true
		}
		p := float64(e) / float64(v.dur)
		y, slope := v.curve.AtSlope(p)
		return Lerp(v.from, v.to, y), (v.to - v.from) * slope / v.dur.Seconds(), false
	case decay:
		secs := e.Seconds()
		// The second test catches the rounding of the stop time to whole
		// nanoseconds.
		if e >= v.dur || secs >= v.stopSecs {
			return v.to, 0, true
		}
		k := -v.rate * secs
		return v.from + v.v0/v.rate*-math.Expm1(k), v.v0 * math.Exp(k), false
	}
	x, vel = v.motion.At(e)
	if v.motion.settledAt(x, vel, v.tol) {
		return v.to, 0, true
	}
	return x, vel, false
}

// belowOne is the largest float64 below 1.
var belowOne = math.Nextafter(1, 0)

// loopPhase returns frac(e/period) for e ≥ 0, in [0, 1): the remainder is
// exact in nanoseconds, and the clamp keeps the division from rounding up
// to 1 (possible only for periods beyond 2⁵³ ns, about 104 days).
func loopPhase(e, period time.Duration) float64 {
	return min(float64(e%period)/float64(period), belowOne)
}

// stop ends the animation and unlists the value.
func (v *Value) stop() {
	if v.kind == idle {
		return
	}
	v.kind = idle
	v.tl.remove(v)
}

// change sets the value and calls the change func if it moved.
func (v *Value) change(x float64) {
	if x == v.x {
		return
	}
	v.x = x
	if v.onChange != nil {
		v.onChange(x)
	}
}

func checkFinite(x float64, what string) {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		panic("anim: Value." + what + " with a value that is not finite")
	}
}
