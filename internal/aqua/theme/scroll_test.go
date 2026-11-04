package theme_test

import (
	"math"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/driver"
)

// TestSequoiaScroll checks the ranges of the scroll-view tokens (S3.5a):
// scroller sizes that fit inside a legacy track, plausible physics
// constants and short durations, and concrete colors.
func TestSequoiaScroll(t *testing.T) {
	s := &theme.Sequoia.Scroll
	for name, v := range map[string]float64{
		"ScrollerThickness": s.ScrollerThickness, "ScrollerHoverThickness": s.ScrollerHoverThickness,
		"ScrollerMinKnobLength": s.ScrollerMinKnobLength, "LegacyScrollerThickness": s.LegacyScrollerThickness,
		"WheelLineHeight": s.WheelLineHeight, "MomentumDecayRate": s.MomentumDecayRate,
		"MomentumStopVelocity": s.MomentumStopVelocity, "RubberBandFraction": s.RubberBandFraction,
	} {
		if !(v > 0) || math.IsInf(v, 0) {
			t.Errorf("%s = %v, want > 0", name, v)
		}
	}
	if s.ScrollerInset < 0 || s.PageOverlap < 0 {
		t.Errorf("inset %v, page overlap %v", s.ScrollerInset, s.PageOverlap)
	}
	// The hovered knob is wider than the idle one, and the widest knob with
	// its inset on both sides fits in the legacy track.
	if s.ScrollerHoverThickness <= s.ScrollerThickness ||
		s.ScrollerHoverThickness+2*s.ScrollerInset > s.LegacyScrollerThickness || s.LegacyScrollerThickness > 20 {
		t.Errorf("knob %v / %v, inset %v, legacy track %v", s.ScrollerThickness, s.ScrollerHoverThickness, s.ScrollerInset, s.LegacyScrollerThickness)
	}
	// The minimum knob is longer than wide (a pill, not a dot).
	if s.ScrollerMinKnobLength < 2*s.ScrollerHoverThickness {
		t.Errorf("min knob length %v for a %v knob", s.ScrollerMinKnobLength, s.ScrollerHoverThickness)
	}
	// A wheel line is about a line of text; a page keeps less than a
	// typical viewport visible.
	if s.WheelLineHeight < 4 || s.WheelLineHeight > 64 || s.PageOverlap > 60 {
		t.Errorf("line height %v, page overlap %v", s.WheelLineHeight, s.PageOverlap)
	}
	// Momentum: the stop velocity is reached from a 5000 pt/s fling within
	// a few seconds; the overscroll asymptote is a fraction of the
	// viewport.
	if stop := math.Log(5000/s.MomentumStopVelocity) / s.MomentumDecayRate; stop < 0.5 || stop > 6 {
		t.Errorf("a fast fling decays for %.2f s (k %v, stop %v)", stop, s.MomentumDecayRate, s.MomentumStopVelocity)
	}
	if s.MomentumStopVelocity > 100 || s.RubberBandFraction >= 1 {
		t.Errorf("stop velocity %v, rubber band %v", s.MomentumStopVelocity, s.RubberBandFraction)
	}
	for name, d := range map[string]time.Duration{
		"ScrollerHoverDuration": s.ScrollerHoverDuration, "WheelStepDuration": s.WheelStepDuration,
		"VelocityWindow": s.VelocityWindow,
	} {
		if d < 10*time.Millisecond || d > 500*time.Millisecond {
			t.Errorf("%s = %v", name, d)
		}
	}
	// The window covers several precise events (about 8 ms apart) but is
	// shorter than the Win32 gap before the late PhaseEnded (about 100 ms),
	// so the gap alone never reads as a stop.
	if s.VelocityWindow < 30*time.Millisecond || s.VelocityWindow >= 100*time.Millisecond {
		t.Errorf("velocity window %v", s.VelocityWindow)
	}
	for name, tok := range map[string]theme.ColorToken{
		"ScrollerKnob": s.ScrollerKnob, "LegacyScrollerTrack": s.LegacyScrollerTrack,
		"LegacyScrollerBorder": s.LegacyScrollerBorder,
	} {
		for _, v := range []driver.Color{tok.Light, tok.Dark, tok.ContrastLight, tok.ContrastDark} {
			if v.IsSemantic() {
				t.Errorf("%s has a semantic value", name)
			}
		}
		if tok.Light.A == 0 || tok.Dark.A == 0 {
			t.Errorf("%s is invisible: %+v", name, tok)
		}
	}
	// An overlay knob is translucent, so the content shows through it.
	if k := s.ScrollerKnob; k.Light.A >= 1 || k.Dark.A >= 1 {
		t.Errorf("the knob is opaque: %+v", k)
	}
}

// TestScrollContrast checks the scroller colors on their backdrops: the
// knob on the window background and on the text background (the content
// it is drawn over), and on a legacy track; the legacy border on the
// track. Each has an increased-contrast variant, stands out, and stands out
// more with Increase Contrast, light and dark.
func TestScrollContrast(t *testing.T) {
	th := &theme.Sequoia
	s := &th.Scroll
	luma := func(x driver.Color) float64 { return 0.2126*float64(x.R) + 0.7152*float64(x.G) + 0.0722*float64(x.B) }
	on := func(fg, bg driver.Color) float64 {
		mix := func(s, d float32) float32 { return s*fg.A + d*(1-fg.A) }
		out := driver.Color{R: mix(fg.R, bg.R), G: mix(fg.G, bg.G), B: mix(fg.B, bg.B), A: 1}
		return math.Abs(luma(out) - luma(bg))
	}
	for _, dark := range []bool{false, true} {
		a := driver.DefaultAppearance()
		a.Dark = dark
		hc := a
		hc.IncreaseContrast = true
		window := th.Resolve(driver.Semantic(driver.ColorWindowBackground), a)
		text := th.Resolve(driver.Semantic(driver.ColorTextBackground), a)
		track := s.LegacyScrollerTrack.Value(a)
		if track.A != 1 {
			t.Errorf("dark=%v: the legacy track is not opaque: %v", dark, track)
		}
		for _, tt := range []struct {
			name string
			tok  theme.ColorToken
			bg   driver.Color
			min  float64
		}{
			{"ScrollerKnob on the window", s.ScrollerKnob, window, 0.2},
			{"ScrollerKnob on the text background", s.ScrollerKnob, text, 0.2},
			{"ScrollerKnob on the legacy track", s.ScrollerKnob, track, 0.2},
			{"LegacyScrollerBorder on the track", s.LegacyScrollerBorder, track, 0.03},
		} {
			if !tt.tok.HasContrast {
				t.Errorf("%s: no increased-contrast variant", tt.name)
				continue
			}
			n, h := on(tt.tok.Value(a), tt.bg), on(tt.tok.Value(hc), tt.bg)
			if n < tt.min || h <= n {
				t.Errorf("dark=%v %s: luma difference %.3f, with increased contrast %.3f", dark, tt.name, n, h)
			}
		}
	}
}
