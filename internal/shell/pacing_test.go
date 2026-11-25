package shell

import (
	"testing"
	"time"
)

const p60 = time.Second / 60

// TestPacing: the vblanks missed and skipped in synthetic runs.
func TestPacing(t *testing.T) {
	at := func(n int) time.Duration { return time.Duration(n) * p60 }
	present := func(p *Pacing, target, shown int) {
		p.Present(PresentTiming{Target: at(target), Shown: at(shown), Period: p60, Refresh: uint64(100 + shown)})
	}

	// A clean run: one frame per vblank, each shown two vblanks later;
	// the statistics skip one report.
	var p Pacing
	for i := range 10 {
		p.Frame(at(i), i < 9)
		if i != 4 {
			present(&p, i, i+2)
		}
	}
	s := p.Summary()
	if s.Frames != 10 || s.Presents != 9 || s.Missed != 0 || s.Skipped != 0 || s.Compared != 9 ||
		s.Latency != at(2) || s.MaxInterval != at(2) || s.Period != p60 {
		t.Errorf("clean run: %v", s)
	}

	// A present that waits a vblank in the queue (frame 2 shown at 5, not
	// 4), so the next frame starts a vblank late (its target 4, not 3):
	// one vblank missed on screen, one skipped by the clock. Then a frame
	// that comes two vblanks late with no queueing: two missed in one
	// place. A present of an uncounted frame and one outside a frame are
	// ignored.
	p = Pacing{}
	for _, i := range []int{0, 1, 2, 4, 5, 8, 9} {
		p.Frame(at(i), i < 9)
	}
	present(&p, 0, 2)
	present(&p, 1, 3)
	present(&p, 2, 5)
	present(&p, 4, 6)
	present(&p, 5, 7)
	present(&p, 8, 10)
	present(&p, 9, 11)
	present(&p, 3, 12)
	p.Present(PresentTiming{Target: -1, Shown: at(13), Period: p60})
	s = p.Summary()
	if s.Frames != 7 || s.Presents != 7 || s.Missed != 3 || s.Stutters != 2 || s.MaxMissed != 2 ||
		s.Skipped != 3 || s.Compared != 9 {
		t.Errorf("missed run: %v", s)
	}

	// Frames across an animation's end or a Break are not compared; nor
	// reports more than maxStep frames apart.
	p = Pacing{}
	p.Frame(at(0), false)
	p.Frame(at(5), true)
	p.Break()
	p.Frame(at(9), true)
	p.Frame(at(10), false)
	present(&p, 0, 2)
	present(&p, 5, 7)
	present(&p, 9, 11)
	present(&p, 10, 15) // compared: 3 missed
	if s := p.Summary(); s.Skipped != 0 || s.Missed != 3 || s.Compared != 4 {
		t.Errorf("ended and broken runs: %v", s)
	}
	p = Pacing{}
	for i := range 20 {
		p.Frame(at(i), true)
	}
	present(&p, 0, 2)
	present(&p, 12, 20)
	if s := p.Summary(); s.Missed != 0 || s.Compared != 0 {
		t.Errorf("reports far apart: %v", s)
	}
	if s := (&Pacing{}).Summary(); s.Frames != 0 || s.Missed != 0 {
		t.Errorf("empty run: %v", s)
	}

	// The display's vblank clock slower than the frame clock's grid by 14
	// µs a period (the host's): nothing missed over 300 frames, with
	// vblank counts and (gtk4) without; a slip of one vblank is found
	// either way.
	for _, counts := range []bool{true, false} {
		const grid, display = 16680 * time.Microsecond, 16666 * time.Microsecond
		p = Pacing{}
		slip := 0
		for i := range 300 {
			if i == 200 {
				slip = 1
			}
			target := time.Duration(i) * grid
			p.Frame(target, true)
			n := i + 2 + slip
			pt := PresentTiming{Target: target, Shown: time.Duration(n) * display, Period: grid}
			if counts {
				pt.Refresh = uint64(1000 + n)
			}
			p.Present(pt)
		}
		if s := p.Summary(); s.Missed != 1 || s.Stutters != 1 || s.Skipped != 0 || s.Compared != 300 {
			t.Errorf("drifting clocks (vblank counts %v): %v", counts, s)
		}
	}
	t.Log(s)
}

func TestPacingTolerance(t *testing.T) {
	for _, c := range [][3]int{{0, 4, 4}, {35, 7, 4}, {100, 20, 4}, {700, 140, 14}} {
		if got := PacingTolerance(c[0]); got != c[1] {
			t.Errorf("PacingTolerance(%d) = %d, want %d", c[0], got, c[1])
		}
		if got := PacingRunTolerance(c[0]); got != c[2] {
			t.Errorf("PacingRunTolerance(%d) = %d, want %d", c[0], got, c[2])
		}
	}
}
