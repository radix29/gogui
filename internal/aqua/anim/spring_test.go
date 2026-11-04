package anim_test

import (
	"math"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/aqua/theme"
)

// springs covers every regime: the theme's two springs (underdamped and
// nearly critical), an exactly critical one, strongly and barely
// overdamped ones, and a lightly damped one.
func springs() map[string]anim.Spring {
	an := theme.Sequoia.Animation
	return map[string]anim.Spring{
		"Control":       anim.Spring(an.ControlSpring),
		"RubberBand":    anim.Spring(an.RubberBandSpring),
		"critical":      {Mass: 1, Stiffness: 100, Damping: 20},
		"overdamped":    {Mass: 1, Stiffness: 100, Damping: 50},
		"nearOver":      {Mass: 1, Stiffness: 100, Damping: 20.0001},
		"lightDamping":  {Mass: 2, Stiffness: 400, Damping: 4},
		"heavyMassOver": {Mass: 5, Stiffness: 50, Damping: 60},
	}
}

// starts are the initial conditions (from, to, v0) the springs run from.
var starts = [][3]float64{
	{0, 1, 0},
	{1, 0, 0},
	{0, 100, 0},
	{0, 1, 20},  // thrown towards the target: critical/over cross it once
	{0, 1, -20}, // thrown away from it
	{-50, 50, 3},
	{5, 5, 40}, // at the target with a velocity
}

// rk4 integrates m·x” = −k·(x − to) − c·x' with the classic Runge-Kutta
// method and the step dt, calling f after each sample interval.
func rk4(s anim.Spring, from, to, v0, dt float64, steps, every int, f func(i int, x, v float64)) {
	acc := func(x, v float64) float64 {
		return (-s.Stiffness*(x-to) - s.Damping*v) / s.Mass
	}
	x, v := from, v0
	for i := 1; i <= steps; i++ {
		k1x, k1v := v, acc(x, v)
		k2x, k2v := v+dt/2*k1v, acc(x+dt/2*k1x, v+dt/2*k1v)
		k3x, k3v := v+dt/2*k2v, acc(x+dt/2*k2x, v+dt/2*k2v)
		k4x, k4v := v+dt*k3v, acc(x+dt*k3x, v+dt*k3v)
		x += dt / 6 * (k1x + 2*k2x + 2*k3x + k4x)
		v += dt / 6 * (k1v + 2*k2v + 2*k3v + k4v)
		if i%every == 0 {
			f(i, x, v)
		}
	}
}

func TestSpringRegimes(t *testing.T) {
	for _, tc := range []struct {
		name string
		want string
	}{
		{"Control", "under"}, {"RubberBand", "under"}, {"critical", "critical"},
		{"overdamped", "over"}, {"nearOver", "over"}, {"lightDamping", "under"}, {"heavyMassOver", "over"},
	} {
		z := springs()[tc.name].Motion(0, 1, 0).DampingRatio()
		got := "under"
		switch {
		case z == 1:
			got = "critical"
		case z > 1:
			got = "over"
		}
		if got != tc.want {
			t.Errorf("%s: ζ = %v (%s), want %s", tc.name, z, got, tc.want)
		}
	}
}

func TestSpringAgainstIntegration(t *testing.T) {
	const dt = 1e-4 // s
	for name, s := range springs() {
		for _, st := range starts {
			from, to, v0 := st[0], st[1], st[2]
			m := s.Motion(from, to, v0)
			scale := math.Max(math.Abs(from-to), math.Abs(v0)/m.Spring().Stiffness) + 1
			worst := 0.0
			rk4(s, from, to, v0, dt, 30000, 10, func(i int, x, v float64) {
				at := time.Duration(float64(i) * dt * float64(time.Second))
				gx, gv := m.At(at)
				worst = math.Max(worst, math.Abs(gx-x)/scale)
				worst = math.Max(worst, math.Abs(gv-v)/(scale*100))
			})
			if worst > 1e-7 {
				t.Errorf("%s from %v: worst relative error %g", name, st, worst)
			}
		}
	}
}

func TestSpringStart(t *testing.T) {
	m := springs()["Control"].Motion(3, 7, -2)
	for _, at := range []time.Duration{0, -time.Second} {
		if x, v := m.At(at); x != 3 || v != -2 {
			t.Errorf("At(%v) = %v, %v; want 3, -2", at, x, v)
		}
	}
	if m.From != 3 || m.To != 7 || m.V0 != -2 {
		t.Errorf("fields = %v %v %v", m.From, m.To, m.V0)
	}
	if got := m.Spring(); got != springs()["Control"] {
		t.Errorf("Spring() = %v", got)
	}
	if z, want := m.DampingRatio(), theme.Sequoia.Animation.ControlSpring.DampingRatio(); math.Abs(z-want) > 1e-15 {
		t.Errorf("DampingRatio() = %v, want %v", z, want)
	}
}

func TestSpringBehaviour(t *testing.T) {
	sp := springs()
	// From rest, critically and overdamped motions never overshoot, and an
	// underdamped one overshoots by e^(−πζ/√(1−ζ²)). RubberBand (ζ ≈ 0.99985)
	// is underdamped, but its overshoot (e^−181) is below float64 precision.
	for name, s := range sp {
		m := s.Motion(0, 1, 0)
		maxX := 0.0
		for ms := range 5000 {
			x, _ := m.At(time.Duration(ms) * time.Millisecond)
			maxX = math.Max(maxX, x)
		}
		over := maxX > 1+1e-12
		if want := m.DampingRatio() < 1 && name != "RubberBand"; over != want {
			t.Errorf("%s: overshoot %v (max %v), want %v", name, over, maxX, want)
		}
	}
	// Far in the future every motion is at its target, with no NaN or Inf
	// (the overdamped form must not overflow).
	for name, s := range sp {
		for _, st := range starts {
			m := s.Motion(st[0], st[1], st[2])
			for _, at := range []time.Duration{time.Minute, time.Hour, 1000 * time.Hour} {
				x, v := m.At(at)
				if math.IsNaN(x) || math.IsNaN(v) || math.Abs(x-st[1]) > 1e-9 || math.Abs(v) > 1e-9 {
					t.Errorf("%s from %v at %v: %v, %v", name, st, at, x, v)
				}
			}
		}
	}
}

func TestSpringSettled(t *testing.T) {
	const eps = 1e-3
	for name, s := range springs() {
		for _, st := range starts {
			m := s.Motion(st[0], st[1], st[2])
			st0 := m.SettleTime(eps)
			if st0 <= 0 || st0 > 30*time.Second {
				t.Errorf("%s from %v: SettleTime = %v", name, st, st0)
				continue
			}
			if !m.Settled(st0, eps) || m.Settled(st0-time.Microsecond, eps) {
				t.Errorf("%s from %v: SettleTime %v is not the first settled time", name, st, st0)
			}
			// Once settled, settled for good, and really within eps.
			for at := st0; at < st0+2*time.Second; at += 250 * time.Microsecond {
				x, v := m.At(at)
				if !m.Settled(at, eps) || math.Abs(x-st[1]) > eps || math.Abs(v) > eps {
					t.Fatalf("%s from %v: unsettled again at %v (%v, %v)", name, st, at, x, v)
				}
			}
			// Before, it was not (SettleTime rounds up to the microsecond).
			for at := time.Duration(0); at < st0-time.Microsecond; at += st0 / 100 {
				if m.Settled(at, eps) {
					t.Fatalf("%s from %v: settled at %v before %v", name, st, at, st0)
				}
			}
		}
	}
	// The control spring settles within a typical control animation.
	m := springs()["Control"].Motion(0, 1, 0)
	if st := m.SettleTime(1e-3); st < 100*time.Millisecond || st > time.Second {
		t.Errorf("Control spring SettleTime = %v", st)
	}
}

func TestSpringSettledEdges(t *testing.T) {
	s := springs()["Control"]
	if got := s.Motion(4, 4, 0).SettleTime(1e-6); got != 0 {
		t.Errorf("at rest: SettleTime = %v, want 0", got)
	}
	if !s.Motion(4, 4, 0).Settled(0, 1e-12) {
		t.Error("at rest: not Settled")
	}
	// A barely damped spring has not settled within an hour.
	slow := anim.Spring{Mass: 1, Stiffness: 1, Damping: 1e-9}
	if got := slow.Motion(0, 1, 0).SettleTime(1e-6); got != anim.MaxSettleTime {
		t.Errorf("barely damped: SettleTime = %v, want %v", got, anim.MaxSettleTime)
	}
}

func TestSpringPanics(t *testing.T) {
	inf, nan := math.Inf(1), math.NaN()
	for _, s := range []anim.Spring{
		{0, 1, 1}, {1, 0, 1}, {1, 1, 0}, {-1, 1, 1}, {1, -1, 1}, {1, 1, -1},
		{nan, 1, 1}, {1, nan, 1}, {1, 1, nan}, {inf, 1, 1}, {1, inf, 1}, {1, 1, inf},
	} {
		mustPanic(t, func() { s.Motion(0, 1, 0) })
	}
	m := springs()["Control"].Motion(0, 1, 0)
	for _, eps := range []float64{0, -1, nan} {
		mustPanic(t, func() { m.SettleTime(eps) })
	}
}

func mustPanic(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("no panic")
		}
	}()
	f()
}
