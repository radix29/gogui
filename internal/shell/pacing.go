package shell

import (
	"fmt"
	"slices"
	"time"
)

// Pacing measures how a run of frames met the display's vertical blanks
// (PLAN.md S6.2d), from a FrameClock's two observers: Frame for each frame
// of the run (FrameTiming.Time, and whether its animation goes on) and
// Present for each PresentTiming.
//
// A vblank is missed when the screen showed no new frame at it while an
// animation ran: between two consecutive reports of presents whose frames
// are all of one running animation (every frame from the first up to the
// one before the second goes on), the display moved on by more vblanks
// than the frames did. However it came about, one stutter on screen counts
// once: a present that waited a vblank in the compositor's queue makes the
// next frame start a vblank late too (the swap chain's waitable signals
// late), which the frame step absorbs. Skipped counts, as a diagnosis,
// the vblanks the frame clock's targets skipped between frames of a
// running animation (the clock fell behind, a frame or the queue too
// slow); it is not added to Missed.
//
// The display's vblanks are counted with PresentTiming.Refresh where both
// reports have one, else from their Shown times (rounded to periods). The
// time from a frame's target to its showing is not compared: the frame
// clock's grid (DWM's refresh period on Windows) and the display's own
// vblank clock (DXGI's statistics) differ by a few microseconds a period
// (measured on the host: 16.680 against 16.666 ms), so it drifts by
// milliseconds over a run and is only logged (Latency). Reports more than
// maxStep frames apart are not compared (a gap in the statistics); a
// shell skipping reports between two is harmless.
//
// Only frames passed to Frame count, and only presents of those frames
// (by their target time). The zero value is ready; it is not safe for
// concurrent use (both observers run on the UI thread).
type Pacing struct {
	frames []pacedFrame
	index  map[time.Duration]int // frames by target
	// stops[i] is the number of frames before i whose animation did not go
	// on.
	stops []int

	presents []pacedPresent // the counted presents, in report order
	period   time.Duration  // the latest present's Period
}

type pacedFrame struct {
	target time.Duration
	goesOn bool
}

type pacedPresent struct {
	PresentTiming
	frame int // index in frames
}

// maxStep is the most frames two reports may be apart to be compared.
const maxStep = 8

// Frame counts the frame with target time target; goesOn reports that its
// animation goes on, so the next frame must come one period later.
func (p *Pacing) Frame(target time.Duration, goesOn bool) {
	if p.index == nil {
		p.index = map[time.Duration]int{}
	}
	stops := 0
	if n := len(p.frames); n > 0 {
		stops = p.stops[n-1]
		if !p.frames[n-1].goesOn {
			stops++
		}
	}
	p.index[target] = len(p.frames)
	p.frames = append(p.frames, pacedFrame{target, goesOn})
	p.stops = append(p.stops, stops)
}

// Break ends a sequence of frames: the next Frame is not compared with
// the previous one (a scripted run's pause between two scenarios).
func (p *Pacing) Break() {
	if n := len(p.frames); n > 0 {
		p.frames[n-1].goesOn = false
	}
}

// Present counts pt if its frame was counted.
func (p *Pacing) Present(pt PresentTiming) {
	if pt.Period > 0 {
		p.period = pt.Period
	}
	if pt.Target < 0 {
		return
	}
	if i, ok := p.index[pt.Target]; ok {
		p.presents = append(p.presents, pacedPresent{pt, i})
	}
}

// PacingSummary is the result of a Pacing run.
type PacingSummary struct {
	Frames   int           // frames counted
	Presents int           // their presents reported shown
	Period   time.Duration // the refresh period (the last present's; 60 Hz without one)
	// Latency is the smallest time from a frame's target to its showing
	// (informative only: it drifts, see Pacing).
	Latency time.Duration
	// Missed is the vblanks at which a running animation showed no new
	// frame, Stutters the places they were missed at (one or more vblanks
	// in a row), MaxMissed the most in one place.
	Missed, Stutters, MaxMissed int
	// Compared is the vblanks the run was checked over: the display's
	// steps between the compared reports.
	Compared int
	// Skipped is the vblanks the frame clock's targets skipped between
	// frames of a running animation (a diagnosis, see Pacing).
	Skipped int
	// MeanInterval and MaxInterval are the time between two consecutive
	// reports of the run, pauses and reports a shell skipped included (a
	// run of one continuous animation whose every present is reported
	// shows its frame interval).
	MeanInterval, MaxInterval time.Duration
}

// vblanks returns d in whole periods, rounded to the nearest.
func vblanks(d, period time.Duration) int {
	return int((d + period/2) / period)
}

// running reports whether frames i up to j-1 all go on.
func (p *Pacing) running(i, j int) bool {
	if j <= i {
		return false
	}
	before := p.stops[j-1]
	if !p.frames[j-1].goesOn {
		before++
	}
	return before == p.stops[i]
}

// Summary returns the run's numbers so far.
func (p *Pacing) Summary() PacingSummary {
	s := PacingSummary{Frames: len(p.frames), Presents: len(p.presents), Period: p.period}
	if s.Period <= 0 {
		s.Period = time.Second / 60
	}
	for i := 1; i < len(p.frames); i++ {
		if f := p.frames[i-1]; f.goesOn {
			if n := vblanks(p.frames[i].target-f.target, s.Period) - 1; n > 0 {
				s.Skipped += n
			}
		}
	}
	for i, pt := range p.presents {
		if lat := pt.Shown - pt.Target; i == 0 || lat < s.Latency {
			s.Latency = lat
		}
		if i == 0 {
			continue
		}
		q := p.presents[i-1]
		frames := pt.frame - q.frame
		if frames > maxStep || !p.running(q.frame, pt.frame) {
			continue
		}
		steps := vblanks(pt.Shown-q.Shown, s.Period)
		if pt.Refresh != 0 && q.Refresh != 0 {
			steps = int(int64(pt.Refresh) - int64(q.Refresh))
		}
		s.Compared += max(steps, 0)
		if n := steps - frames; n > 0 {
			s.Missed += n
			s.Stutters++
			s.MaxMissed = max(s.MaxMissed, n)
		}
	}
	if len(p.presents) > 1 {
		ts := make([]time.Duration, len(p.presents))
		for i, pt := range p.presents {
			ts[i] = pt.Shown
		}
		slices.Sort(ts)
		var sum time.Duration
		for i := 1; i < len(ts); i++ {
			d := ts[i] - ts[i-1]
			sum += d
			s.MaxInterval = max(s.MaxInterval, d)
		}
		s.MeanInterval = sum / time.Duration(len(ts)-1)
	}
	return s
}

// String formats the summary for a log line.
func (s PacingSummary) String() string {
	return fmt.Sprintf("frames=%d presents=%d period=%.3fms compared=%d missed=%d stutters=%d max-missed=%d skipped=%d latency=%.2fms mean-interval=%.2fms max-interval=%.2fms",
		s.Frames, s.Presents, ms(s.Period), s.Compared, s.Missed, s.Stutters, s.MaxMissed, s.Skipped, ms(s.Latency), ms(s.MeanInterval), ms(s.MaxInterval))
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

// PacingTolerance and PacingRunTolerance are how many vblanks a pacing run
// on real windows may miss (PLAN.md S6.2d), checked over n vblanks
// (PacingSummary.Compared): one scenario a fifth, at least 4, and the
// scenarios of a run together a fiftieth (2 %, S6.2e), at least 4. The
// scenario's fifth is S6.2d's, from the host's numbers then (24 scenario
// runs: a median of 0 to 1 %, the worst scenario 14.5 %, a whole run 1.7
// to 3.1 %, with four monitors on one iGPU); S6.2e brought the run down
// from a twentieth to 2 % (the host with one monitor: 0 % in every run,
// before and after its clock change).
func PacingTolerance(n int) int { return max(4, n/5) }

// PacingRunTolerance: see PacingTolerance.
func PacingRunTolerance(n int) int { return max(4, n/50) }
