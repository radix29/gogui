package scroll

import "time"

// DefaultEndGap is the EndGap for the shells: the Win32 shell's gesture
// timeout (100 ms after the last precise event, shell/win32 wheel.go) plus
// slack for a late WM_TIMER. GTK ends a gesture at the lift, so a longer
// gap there really is fingers at rest.
const DefaultEndGap = 150 * time.Millisecond

// VelocityTracker estimates the release velocity of a precise scroll
// gesture from its deltas (PLAN.md §5.6). The ScrollView feeds it the
// gesture's events and asks it for the velocity when the gesture ends, to
// start the momentum. Win32's PhaseEnded carries no velocity and arrives
// late, so the tracker has to work from the deltas alone.
//
// Add takes the moving events (PhaseBegan and PhaseChanged; a zero delta
// is fine and reports that the content is held still) and End the ending
// one. The velocity is the distance of the samples within Window before
// the latest sample, over the time they span: the oldest sample in the
// window anchors the span and its own delta (which covers the time before
// it) is left out, so a steady stream of deltas d every Δt gives exactly
// d/Δt, and a gesture that speeds up or slows down gives the velocity of
// its last Window, not of the whole gesture.
//
// The end rule: End(t) ignores the end event's delta and the gap between
// the latest sample and t when that gap is at most EndGap, and returns the
// velocity as of the latest sample. This is the Win32 case: the shell ends
// a gesture with a zero-delta PhaseEnded about 100 ms after the last
// precise event (its gesture timeout), whether the fingers were lifted
// while moving or not, so EndGap is that timeout plus some timer slack. A
// longer gap means the fingers rested before the end (an OS whose end
// event is the lift itself): End returns 0, no momentum. Fingers that
// slowed down and stopped before the lift show up in the deltas instead:
// the small (or zero) deltas of the last Window give a small velocity.
//
// Timestamps need not be strictly increasing: a sample at the same time
// as the latest one, or earlier (out of order), is merged into the latest
// one. Too few samples (fewer than two in the window) or a zero time span
// give 0, never a division by zero.
//
// The zero value has no window and always reports 0; set Window (and
// EndGap) first. A VelocityTracker is not safe for concurrent use; the
// ScrollView uses it on the UI thread.
type VelocityTracker struct {
	// Window is how far back from the latest sample the velocity looks
	// (the velocity-window token). Not positive: the velocity is 0.
	Window time.Duration
	// EndGap is the longest gap between the latest sample and End that
	// still counts as the OS's own end delay rather than fingers at rest.
	EndGap time.Duration

	samples []sample // oldest first; only the ones Velocity can still use
}

// sample is one moving event: its time and delta.
type sample struct {
	t time.Duration
	d float64
}

// Reset forgets every sample, for the start of a new gesture. Window and
// EndGap stay.
func (vt *VelocityTracker) Reset() { vt.samples = vt.samples[:0] }

// Add records a moving event of the gesture at the time t with the delta d
// (points, on this tracker's axis).
func (vt *VelocityTracker) Add(t time.Duration, d float64) {
	if n := len(vt.samples); n > 0 && t <= vt.samples[n-1].t {
		vt.samples[n-1].d += d
		return
	}
	vt.samples = append(vt.samples, sample{t, d})
	vt.prune()
}

// prune drops the samples the window has left behind: the ones older than
// the latest sample minus Window. The latest one always stays.
func (vt *VelocityTracker) prune() {
	n := len(vt.samples)
	start := vt.samples[n-1].t - vt.Window
	i := 0
	for i < n-1 && vt.samples[i].t < start {
		i++
	}
	if i > 0 {
		vt.samples = append(vt.samples[:0], vt.samples[i:]...)
	}
}

// Velocity returns the velocity (points per second) as of the latest
// sample: the deltas within Window of it, over the time they span.
func (vt *VelocityTracker) Velocity() float64 {
	n := len(vt.samples)
	if n < 2 || !(vt.Window > 0) {
		return 0
	}
	last := vt.samples[n-1].t
	start := last - vt.Window
	i := 0
	for vt.samples[i].t < start {
		i++
	}
	span := last - vt.samples[i].t
	if span <= 0 {
		return 0
	}
	var dist float64
	for _, s := range vt.samples[i+1:] {
		dist += s.d
	}
	return dist / span.Seconds()
}

// End returns the release velocity for a gesture that ended at the time t:
// Velocity, or 0 when t is more than EndGap after the latest sample (or
// when there are no samples). The end event's own delta is not used; a
// caller whose end events carry a delta adds it first.
func (vt *VelocityTracker) End(t time.Duration) float64 {
	n := len(vt.samples)
	if n == 0 || t-vt.samples[n-1].t > vt.EndGap {
		return 0
	}
	return vt.Velocity()
}
