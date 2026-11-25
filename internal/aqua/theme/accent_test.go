package theme_test

import (
	"math"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/driver"
)

// near reports whether a and b differ by at most tol in every channel.
func near(a, b driver.Color, tol float32) bool {
	d := func(x, y float32) bool { return float32(math.Abs(float64(x-y))) <= tol }
	return a.Semantic == b.Semantic && d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B) && d(a.A, b.A)
}

// TestAccentRulesReproduceTokens checks that the accent rules, applied to
// the default accent, give the reference values the spec keeps for the
// accent-derived semantic colors (within 1.5/255 per channel).
func TestAccentRulesReproduceTokens(t *testing.T) {
	th := &theme.Sequoia
	const tol = 1.5 / 255
	for _, dark := range []bool{false, true} {
		a := driver.Appearance{Dark: dark, Accent: driver.DefaultAccent}
		for _, s := range []driver.SemanticColor{driver.ColorSelectedTextBackground, driver.ColorSelectedControl} {
			want := th.Colors[s].Value(a)
			if got := th.Resolve(driver.Semantic(s), a); !near(got, want, tol) {
				t.Errorf("dark=%v: %d resolves to %+v, token %+v", dark, s, got, want)
			}
		}
	}
	light := driver.DefaultAppearance()
	if got, want := th.Resolve(driver.Semantic(driver.ColorKeyboardFocusIndicator), light), th.Colors[driver.ColorKeyboardFocusIndicator].Light; !near(got, want, tol) {
		t.Errorf("focus ring %+v, token %+v", got, want)
	}
	if got, want := th.Resolve(driver.Semantic(driver.ColorControlAccent), light), th.Colors[driver.ColorControlAccent].Light; !near(got, want, tol) {
		t.Errorf("control accent %+v, token %+v", got, want)
	}
}

func TestAccentRules(t *testing.T) {
	th := &theme.Sequoia
	ac := th.Accent
	for _, a := range []driver.Appearance{
		{Accent: driver.RGB8(255, 0, 0)},
		{Dark: true, Accent: driver.RGB8(0, 200, 100)},
		{Dark: true, Accent: driver.RGBA(0.2, 0.4, 0.6, 0.8)},
	} {
		base := ac.Base.Value(a)
		f := th.FocusRing(a)
		if f.R != a.Accent.R || f.G != a.Accent.G || f.B != a.Accent.B || f.A != a.Accent.A*ac.FocusRingAlpha {
			t.Errorf("FocusRing(%+v) = %+v", a.Accent, f)
		}
		if c := th.Resolve(driver.Semantic(driver.ColorKeyboardFocusIndicator), a); c != f {
			t.Errorf("Resolve focus %+v, FocusRing %+v", c, f)
		}
		k := a.Accent.A * ac.SelectionAlpha
		sel := th.Selection(a)
		if !near(sel, driver.Color{
			R: base.R + (a.Accent.R-base.R)*k,
			G: base.G + (a.Accent.G-base.G)*k,
			B: base.B + (a.Accent.B-base.B)*k,
			A: 1,
		}, 1e-6) {
			t.Errorf("Selection(%+v) = %+v over %+v", a.Accent, sel, base)
		}
		for _, s := range []driver.SemanticColor{driver.ColorSelectedTextBackground, driver.ColorSelectedControl} {
			if c := th.Resolve(driver.Semantic(s), a); c != sel {
				t.Errorf("Resolve %d = %+v, Selection %+v", s, c, sel)
			}
		}
		p := th.AccentPressed(a)
		if p.R != a.Accent.R*ac.PressedShade || p.G != a.Accent.G*ac.PressedShade || p.B != a.Accent.B*ac.PressedShade || p.A != a.Accent.A {
			t.Errorf("AccentPressed(%+v) = %+v", a.Accent, p)
		}
		in := th.AccentInactive(a)
		if !near(in, driver.Color{R: in.R, G: in.R, B: in.R, A: 1}, 1e-6) {
			t.Errorf("AccentInactive(%+v) = %+v, want an opaque grey", a.Accent, in)
		}
		want := ac.DefaultButtonText.Value(a)
		if theme.ContrastRatio(want, a.Accent) < float64(ac.AccentTextMinContrast) {
			want = ac.AccentDarkText.Value(a)
		}
		if th.AccentText(a) != want {
			t.Errorf("AccentText(%+v) = %+v, want %+v", a.Accent, th.AccentText(a), want)
		}
	}
	// A different accent changes every derived color.
	a1 := driver.Appearance{Accent: driver.RGB8(255, 0, 0)}
	a2 := driver.Appearance{Accent: driver.RGB8(0, 0, 255)}
	if th.Selection(a1) == th.Selection(a2) || th.AccentPressed(a1) == th.AccentPressed(a2) ||
		th.FocusRing(a1) == th.FocusRing(a2) {
		t.Error("derived colors do not follow the accent")
	}
	// The inactive grey does not (S6.1a): for every accent it is the
	// default accent's luma at InactiveAlpha over Base.
	def := th.AccentInactive(driver.Appearance{Accent: driver.DefaultAccent})
	if th.AccentInactive(a1) != def || th.AccentInactive(a2) != def ||
		th.AccentInactive(driver.Appearance{Accent: driver.RGB8(0, 0, 0)}) != def {
		t.Error("the inactive grey follows the accent")
	}
	da := driver.DefaultAccent
	y, k := 0.2126*da.R+0.7152*da.G+0.0722*da.B, ac.InactiveAlpha
	base := ac.Base.Value(driver.Appearance{})
	if !near(def, driver.Color{R: y*k + base.R*(1-k), G: y*k + base.G*(1-k), B: y*k + base.B*(1-k), A: 1}, 1e-6) {
		t.Errorf("inactive grey %+v over %+v", def, base)
	}
	if ac.SelectionAlpha <= 0 || ac.SelectionAlpha >= 1 || ac.FocusRingAlpha <= 0 || ac.FocusRingAlpha > 1 ||
		ac.PressedShade <= 0 || ac.PressedShade >= 1 || ac.InactiveAlpha <= 0 || ac.InactiveAlpha >= 1 {
		t.Errorf("accent numbers %+v", ac)
	}
	for _, c := range []driver.Color{ac.Base.Light, ac.Base.Dark} {
		if c.A != 1 {
			t.Errorf("Base %+v is not opaque", c)
		}
	}
}

// TestIncreaseContrast checks the contrast variants: Resolve picks them
// with IncreaseContrast, they differ from the normal values and are at
// least as opaque, and colors without one keep their value.
func TestIncreaseContrast(t *testing.T) {
	th := &theme.Sequoia
	for s := driver.SemanticColor(1); s.Valid(); s++ {
		tok := th.Colors[s]
		if !tok.HasContrast {
			continue
		}
		for _, dark := range []bool{false, true} {
			a := driver.Appearance{Dark: dark, Accent: driver.DefaultAccent}
			hc := a
			hc.IncreaseContrast = true
			normal, contrast := th.Resolve(driver.Semantic(s), a), th.Resolve(driver.Semantic(s), hc)
			want := tok.ContrastLight
			if dark {
				want = tok.ContrastDark
			}
			if contrast != want || contrast.IsSemantic() {
				t.Errorf("%d dark=%v: contrast %+v, want %+v", s, dark, contrast, want)
			}
			if contrast == normal {
				t.Errorf("%d dark=%v: contrast variant equals the normal value", s, dark)
			}
			if contrast.A < normal.A {
				t.Errorf("%d dark=%v: contrast alpha %v below %v", s, dark, contrast.A, normal.A)
			}
		}
	}
	for _, s := range []driver.SemanticColor{
		driver.ColorLabel, driver.ColorSecondaryLabel, driver.ColorTertiaryLabel, driver.ColorQuaternaryLabel,
		driver.ColorPlaceholderText, driver.ColorSeparator, driver.ColorGrid, driver.ColorLink,
		driver.ColorControlText, driver.ColorDisabledControlText,
	} {
		if !th.Colors[s].HasContrast {
			t.Errorf("%d has no contrast variant", s)
		}
	}
	hc := driver.Appearance{IncreaseContrast: true, Accent: driver.DefaultAccent}
	bg := th.Colors[driver.ColorWindowBackground]
	if bg.HasContrast || th.Resolve(driver.Semantic(driver.ColorWindowBackground), hc) != bg.Light {
		t.Error("window background changed with IncreaseContrast")
	}
	if !th.Chrome.TitleBarSeparator.HasContrast || !th.Chrome.LightInactiveBorder.HasContrast || !th.Metrics.BezelShadow.HasContrast {
		t.Error("title-bar separator, inactive light border or bezel shadow has no contrast variant")
	}
	// The focus ring is the full accent with IncreaseContrast (S3.2d).
	for _, dark := range []bool{false, true} {
		a := driver.Appearance{Dark: dark, IncreaseContrast: true, Accent: driver.RGBA(0.1, 0.5, 0.9, 0.9)}
		if f := th.FocusRing(a); f != a.Accent || th.Resolve(driver.Semantic(driver.ColorKeyboardFocusIndicator), a) != f {
			t.Errorf("dark=%v: contrast focus ring %+v, want the accent", dark, f)
		}
	}
}

func TestAnimationTokens(t *testing.T) {
	an := &theme.Sequoia.Animation
	if !(0 < an.DurationFast && an.DurationFast < an.DurationNormal && an.DurationNormal < an.DurationSlow) {
		t.Errorf("durations %v %v %v", an.DurationFast, an.DurationNormal, an.DurationSlow)
	}
	if an.DurationNormal < 200*time.Millisecond || an.DurationNormal > 250*time.Millisecond {
		t.Errorf("DurationNormal %v outside the 0.2-0.25 s of PLAN.md §5.5", an.DurationNormal)
	}
	for _, d := range []time.Duration{an.FocusRingDuration, an.ScrollerFadeDelay, an.ScrollerFadeDuration} {
		if d <= 0 {
			t.Errorf("duration %v", d)
		}
	}
	for _, c := range []theme.Curve{an.EaseIn, an.EaseOut, an.EaseInOut, an.EaseDefault} {
		if c.X1 < 0 || c.X1 > 1 || c.X2 < 0 || c.X2 > 1 {
			t.Errorf("curve %+v", c)
		}
	}
	if an.EaseInOut != (theme.Curve{X1: 0.42, Y1: 0, X2: 0.58, Y2: 1}) {
		t.Errorf("EaseInOut %+v", an.EaseInOut)
	}
	for _, s := range []theme.Spring{an.ControlSpring, an.RubberBandSpring} {
		if s.Mass <= 0 || s.Stiffness <= 0 || s.Damping <= 0 {
			t.Errorf("spring %+v", s)
		}
	}
	if r := an.RubberBandSpring.DampingRatio(); math.Abs(r-1) > 0.01 {
		t.Errorf("rubber band damping ratio %v, want critically damped", r)
	}
	if r := an.ControlSpring.DampingRatio(); r <= 0.5 || r >= 1 {
		t.Errorf("control spring damping ratio %v, want slightly underdamped", r)
	}
	if r := (theme.Spring{Mass: 1, Stiffness: 100, Damping: 20}).DampingRatio(); r != 1 {
		t.Errorf("DampingRatio = %v, want 1", r)
	}
}

// TestContrastRatio checks the WCAG 2 formula on known pairs.
func TestContrastRatio(t *testing.T) {
	white, black := driver.RGB8(255, 255, 255), driver.RGB8(0, 0, 0)
	for _, c := range []struct {
		a, b driver.Color
		want float64
	}{
		{white, black, 21},
		{black, white, 21},
		{white, white, 1},
		{white, driver.RGB8(118, 118, 118), 4.54}, // WCAG's AA grey
		{white, driver.DefaultAccent, 4.02},
	} {
		if got := theme.ContrastRatio(c.a, c.b); math.Abs(got-c.want) > 0.01 {
			t.Errorf("ContrastRatio(%v, %v) = %.3f, want %.2f", c.a, c.b, got, c.want)
		}
	}
}

// TestAccentTextLightAccent checks the light-accent rule (S6.1g): the
// label of an accent fill (the default button's title, the check mark,
// the radio dot, the pop-up indicator's chevrons) and a highlighted menu
// item's title stay DefaultButtonText on the default accent and on dark
// or saturated ones, and turn AccentDarkText on light ones, where white
// falls below AccentTextMinContrast (3:1); either way the label has at
// least 3:1 on the fill. In an inactive window
// the grey fill keeps DefaultButtonText for every accent.
func TestAccentTextLightAccent(t *testing.T) {
	th := &theme.Sequoia
	ac := th.Accent
	if ac.AccentTextMinContrast != 3 {
		t.Fatalf("AccentTextMinContrast %v, want 3 (decided in S6.1g)", ac.AccentTextMinContrast)
	}
	for _, c := range []struct {
		accent driver.Color
		dark   bool // AccentDarkText expected
	}{
		{driver.DefaultAccent, false},
		{driver.RGB8(0, 120, 215), false},  // the Windows default
		{driver.RGB8(149, 61, 150), false}, // macOS purple
		{driver.RGB8(255, 45, 85), false},  // the inactive matrix's other accent
		{driver.RGB8(60, 20, 90), false},
		{driver.RGB8(255, 214, 10), true}, // yellow
		{driver.RGB8(247, 130, 25), true}, // orange: 2.6:1
		{driver.RGB8(40, 205, 65), true},  // green
		{driver.RGB8(255, 255, 255), true},
		{driver.RGB8(200, 230, 255), true}, // a pale blue
	} {
		for _, darkMode := range []bool{false, true} {
			a := driver.DefaultAppearance()
			a.Dark, a.Accent = darkMode, c.accent
			want := ac.DefaultButtonText.Value(a)
			if c.dark {
				want = ac.AccentDarkText.Value(a)
			}
			got := th.AccentText(a)
			if got != want {
				t.Errorf("accent %v dark=%v: AccentText %v, want %v", c.accent, darkMode, got, want)
			}
			if th.ButtonText(a, true, true, false) != got || th.ToggleMark(a, true) != got || th.PopUpChevron(a, true, false) != got {
				t.Errorf("accent %v dark=%v: the default button, the toggles or the pop-up indicator do not use AccentText", c.accent, darkMode)
			}
			if m := th.ToggleMark(a, false); m != ac.DefaultButtonText.Value(a) {
				t.Errorf("accent %v dark=%v: inactive mark %v, want DefaultButtonText", c.accent, darkMode, m)
			}
			wantMenu := th.Menu.HighlightedText.Value(a)
			if c.dark {
				wantMenu = ac.AccentDarkText.Value(a)
			}
			if m := th.MenuHighlightedText(a); m != wantMenu {
				t.Errorf("accent %v dark=%v: menu highlighted text %v, want %v", c.accent, darkMode, m, wantMenu)
			}
			// The label over the fill (straight alpha over the opaque accent).
			on := func(fg driver.Color) driver.Color {
				k := fg.A
				return driver.Color{R: fg.R*k + c.accent.R*(1-k), G: fg.G*k + c.accent.G*(1-k), B: fg.B*k + c.accent.B*(1-k), A: 1}
			}
			if r := theme.ContrastRatio(on(got), c.accent); r < 3 {
				t.Errorf("accent %v dark=%v: label contrast %.2f, want at least 3", c.accent, darkMode, r)
			}
		}
	}
}
