package theme_test

import (
	"math"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/driver"
)

// TestSequoiaControls checks the ranges of the basic-control tokens
// (S3.3b): every length positive, each control's parts fitting inside it,
// the per-size rows growing mini ≤ small ≤ regular ≤ large, concrete
// colors and standard weights.
func TestSequoiaControls(t *testing.T) {
	c := &theme.Sequoia.Controls
	m := &theme.Sequoia.Metrics
	order := []driver.ControlSize{driver.ControlSizeMini, driver.ControlSizeSmall, driver.ControlSizeRegular, driver.ControlSizeLarge}
	rows := map[string]*theme.SizeTable{
		"ButtonMinWidth": &c.ButtonMinWidth,
		"CheckboxSize":   &c.CheckboxSize, "CheckboxRadius": &c.CheckboxRadius,
		"CheckboxMarkStroke": &c.CheckboxMarkStroke, "CheckboxGap": &c.CheckboxGap,
		"RadioSize": &c.RadioSize, "RadioDotSize": &c.RadioDotSize, "RadioGap": &c.RadioGap,
	}
	for name, row := range rows {
		for i, cs := range order {
			if row.At(cs) <= 0 {
				t.Errorf("%s at size %d is %v", name, cs, row.At(cs))
			}
			if i > 0 && row.At(cs) < row.At(order[i-1]) {
				t.Errorf("%s shrinks from size %d to %d", name, order[i-1], cs)
			}
		}
	}
	for cs := driver.ControlSize(0); cs < theme.NumControlSizes; cs++ {
		ctl := m.Control(cs)
		if c.ButtonMinWidth.At(cs) < 2*ctl.Height || c.ButtonMinWidth.At(cs) <= 2*ctl.PaddingX {
			t.Errorf("size %d: button min width %v too small for height %v, padding %v", cs, c.ButtonMinWidth.At(cs), ctl.Height, ctl.PaddingX)
		}
		box, radius, stroke := c.CheckboxSize.At(cs), c.CheckboxRadius.At(cs), c.CheckboxMarkStroke.At(cs)
		if box > ctl.Height || radius > box/2 || stroke >= box/4 {
			t.Errorf("size %d: checkbox %v, radius %v, stroke %v (bezel %v)", cs, box, radius, stroke, ctl.Height)
		}
		if d, dot := c.RadioSize.At(cs), c.RadioDotSize.At(cs); d > ctl.Height || dot >= d-2 {
			t.Errorf("size %d: radio %v, dot %v (bezel %v)", cs, d, dot, ctl.Height)
		}
		if c.CheckboxGap.At(cs) > c.CheckboxSize.At(cs) || c.RadioGap.At(cs) > c.RadioSize.At(cs) {
			t.Errorf("size %d: gaps %v, %v", cs, c.CheckboxGap.At(cs), c.RadioGap.At(cs))
		}
	}
	if c.CheckboxSize.At(driver.ControlSize(9)) != c.CheckboxSize.At(driver.ControlSizeRegular) {
		t.Error("unknown control size does not get the regular checkbox")
	}
	if c.BoxCornerRadius <= 0 || c.BoxCornerRadius > 12 || c.BoxBorderWidth <= 0 || c.BoxBorderWidth > 2 ||
		c.BoxTitleGap < 0 || c.BoxContentInsetX <= 0 || c.BoxContentInsetY <= 0 {
		t.Errorf("box metrics %+v", c)
	}
	if fs := theme.Sequoia.Typography.FontSize; c.BoxTitleSize < fs.At(driver.ControlSizeMini) || c.BoxTitleSize > fs.At(driver.ControlSizeRegular) {
		t.Errorf("box title size %v", c.BoxTitleSize)
	}
	if c.SeparatorThickness <= 0 || c.SeparatorThickness > 2 {
		t.Errorf("separator thickness %v", c.SeparatorThickness)
	}
	for _, w := range []driver.FontWeight{c.ButtonTitleWeight, c.BoxTitleWeight} {
		if w < driver.FontWeightUltraLight || w > driver.FontWeightBlack || w%100 != 0 {
			t.Errorf("weight %d", w)
		}
	}
	for name, tok := range map[string]theme.ColorToken{
		"BezelFill": c.BezelFill, "BezelPressedFill": c.BezelPressedFill, "BezelBorder": c.BezelBorder,
		"BoxFill": c.BoxFill, "BoxBorder": c.BoxBorder,
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
	// A pressed bezel differs from the normal one in both appearances.
	for _, dark := range []bool{false, true} {
		a := driver.Appearance{Dark: dark}
		if c.BezelFill.Value(a) == c.BezelPressedFill.Value(a) {
			t.Errorf("dark=%v: pressed bezel equals the normal one", dark)
		}
	}
	in := c.BoxContentInsets()
	if in != (driver.Insets{Top: c.BoxContentInsetY, Left: c.BoxContentInsetX, Bottom: c.BoxContentInsetY, Right: c.BoxContentInsetX}) {
		t.Errorf("BoxContentInsets = %+v", in)
	}
}

// TestControlFills checks ButtonFill, ButtonText and ToggleFill against
// the accent rules and the bezel tokens, with another accent than the
// default so a stored accent value would show.
func TestControlFills(t *testing.T) {
	th := &theme.Sequoia
	c := &th.Controls
	for _, dark := range []bool{false, true} {
		a := driver.DefaultAppearance()
		a.Dark = dark
		a.Accent = driver.RGB8(200, 60, 120)
		bezel, pressed := c.BezelFill.Value(a), c.BezelPressedFill.Value(a)
		for _, tt := range []struct {
			name                 string
			isDefault, down, key bool
			want                 driver.Color
		}{
			{"plain", false, false, true, bezel},
			{"plain pressed", false, true, true, pressed},
			{"plain inactive", false, false, false, bezel},
			{"plain pressed inactive", false, true, false, pressed},
			{"default", true, false, true, a.Accent},
			{"default pressed", true, true, true, th.AccentPressed(a)},
			{"default inactive", true, false, false, bezel},
			{"default pressed inactive", true, true, false, pressed},
		} {
			if got := th.ButtonFill(a, tt.isDefault, tt.down, tt.key); got != tt.want {
				t.Errorf("dark=%v ButtonFill %s = %v, want %v", dark, tt.name, got, tt.want)
			}
		}
		ctlText := th.Resolve(driver.Semantic(driver.ColorControlText), a)
		disText := th.Resolve(driver.Semantic(driver.ColorDisabledControlText), a)
		for _, tt := range []struct {
			name                     string
			isDefault, key, disabled bool
			want                     driver.Color
		}{
			{"plain", false, true, false, ctlText},
			{"default", true, true, false, th.AccentText(a)},
			{"default inactive", true, false, false, ctlText},
			{"disabled", false, true, true, disText},
			{"default disabled", true, true, true, disText},
		} {
			if got := th.ButtonText(a, tt.isDefault, tt.key, tt.disabled); got != tt.want {
				t.Errorf("dark=%v ButtonText %s = %v, want %v", dark, tt.name, got, tt.want)
			}
		}
		for _, tt := range []struct {
			name          string
			on, down, key bool
			want          driver.Color
		}{
			{"off", false, false, true, bezel},
			{"off pressed", false, true, true, pressed},
			{"off inactive", false, false, false, bezel},
			{"on", true, false, true, a.Accent},
			{"on pressed", true, true, true, th.AccentPressed(a)},
			{"on inactive", true, false, false, th.AccentInactive(a)},
			{"on pressed inactive", true, true, false, th.AccentInactive(a)},
		} {
			if got := th.ToggleFill(a, tt.on, tt.down, tt.key); got != tt.want {
				t.Errorf("dark=%v ToggleFill %s = %v, want %v", dark, tt.name, got, tt.want)
			}
		}
	}
}

// TestSequoiaValueControls checks the ranges of the value-control tokens
// (S3.4a): positive lengths growing mini ≤ small ≤ regular ≤ large, pill
// tracks, a knob wider than its track, spokes that fit inside the spinner,
// fractions and alphas in 0..1, a whole spoke count, positive periods and
// concrete, visible colors.
func TestSequoiaValueControls(t *testing.T) {
	c := &theme.Sequoia.Controls
	order := []driver.ControlSize{driver.ControlSizeMini, driver.ControlSizeSmall, driver.ControlSizeRegular, driver.ControlSizeLarge}
	rows := map[string]*theme.SizeTable{
		"SliderTrackThickness": &c.SliderTrackThickness, "SliderTrackRadius": &c.SliderTrackRadius,
		"SliderKnobSize": &c.SliderKnobSize, "SliderKnobShadowOffsetY": &c.SliderKnobShadowOffsetY,
		"SliderKnobShadowBlur": &c.SliderKnobShadowBlur, "SliderTickLength": &c.SliderTickLength,
		"SliderTickWidth": &c.SliderTickWidth, "SliderTickGap": &c.SliderTickGap,
		"ProgressThickness": &c.ProgressThickness, "ProgressRadius": &c.ProgressRadius,
		"SpinnerSize": &c.SpinnerSize, "SpinnerSpokeLength": &c.SpinnerSpokeLength,
		"SpinnerSpokeWidth": &c.SpinnerSpokeWidth,
	}
	for name, row := range rows {
		for i, cs := range order {
			if row.At(cs) <= 0 {
				t.Errorf("%s at size %d is %v", name, cs, row.At(cs))
			}
			if i > 0 && row.At(cs) < row.At(order[i-1]) {
				t.Errorf("%s shrinks from size %d to %d", name, order[i-1], cs)
			}
		}
	}
	for cs := driver.ControlSize(0); cs < theme.NumControlSizes; cs++ {
		track, radius, knob := c.SliderTrackThickness.At(cs), c.SliderTrackRadius.At(cs), c.SliderKnobSize.At(cs)
		if radius > track/2 || knob <= 2*track || c.SliderKnobShadowBlur.At(cs) > knob/4 || c.SliderKnobShadowOffsetY.At(cs) > knob/4 {
			t.Errorf("size %d: slider track %v, radius %v, knob %v, shadow %v/%v", cs, track, radius, knob,
				c.SliderKnobShadowOffsetY.At(cs), c.SliderKnobShadowBlur.At(cs))
		}
		if c.SliderTickLength.At(cs) > knob/2 || c.SliderTickWidth.At(cs) > track || c.SliderTickGap.At(cs) > knob/4 {
			t.Errorf("size %d: ticks %v × %v, gap %v (knob %v, track %v)", cs,
				c.SliderTickLength.At(cs), c.SliderTickWidth.At(cs), c.SliderTickGap.At(cs), knob, track)
		}
		if th, r := c.ProgressThickness.At(cs), c.ProgressRadius.At(cs); r > th/2 || th > theme.Sequoia.Metrics.ControlHeight.At(cs) {
			t.Errorf("size %d: progress thickness %v, radius %v", cs, th, r)
		}
		d, l, w := c.SpinnerSize.At(cs), c.SpinnerSpokeLength.At(cs), c.SpinnerSpokeWidth.At(cs)
		// The spokes stay apart at their inner end: the circumference
		// there leaves room for each spoke's width.
		if inner := d/2 - l; inner <= 0 || w >= l || 2*math.Pi*inner <= c.SpinnerSpokes*w {
			t.Errorf("size %d: spinner %v, spoke %v × %v, %v spokes", cs, d, l, w, c.SpinnerSpokes)
		}
	}
	if c.SliderKnobBorderWidth <= 0 || c.SliderKnobBorderWidth > 1 {
		t.Errorf("knob border width %v", c.SliderKnobBorderWidth)
	}
	for name, f := range map[string]float64{
		"ProgressBandFraction": c.ProgressBandFraction, "SpinnerTailAlpha": float64(c.SpinnerTailAlpha),
		"ValueDisabledAlpha": float64(c.ValueDisabledAlpha),
	} {
		if !(f > 0 && f < 1) {
			t.Errorf("%s = %v, want in (0, 1)", name, f)
		}
	}
	if n := c.SpinnerSpokes; n < 6 || n > 16 || n != math.Trunc(n) {
		t.Errorf("SpinnerSpokes = %v, want a whole number in 6..16", n)
	}
	for name, d := range map[string]time.Duration{"ProgressBandPeriod": c.ProgressBandPeriod, "SpinnerPeriod": c.SpinnerPeriod} {
		if d < 250*time.Millisecond || d > 5*time.Second {
			t.Errorf("%s = %v", name, d)
		}
	}
	for name, tok := range map[string]theme.ColorToken{
		"SliderTrack": c.SliderTrack, "SliderKnobFill": c.SliderKnobFill, "SliderKnobBorder": c.SliderKnobBorder,
		"SliderKnobShadow": c.SliderKnobShadow, "SliderTickMark": c.SliderTickMark,
		"ProgressTrack": c.ProgressTrack, "SpinnerSpoke": c.SpinnerSpoke,
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
	if c.SliderKnobFill.Light.A != 1 || c.SliderKnobFill.Dark.A != 1 {
		t.Errorf("the knob is not opaque: %+v", c.SliderKnobFill)
	}
}

// TestValueContrast checks the colors drawn as lines or thin shapes on a
// backdrop (the tracks, the tick marks and the spokes on the window
// background, the knob's border on the knob): each has an increased-
// contrast variant, both stand out from the backdrop, and the variant
// stands out more, light and dark.
func TestValueContrast(t *testing.T) {
	th := &theme.Sequoia
	c := &th.Controls
	luma := func(x driver.Color) float64 { return 0.2126*float64(x.R) + 0.7152*float64(x.G) + 0.0722*float64(x.B) }
	// on returns the luma difference between fg composited over the
	// opaque bg and bg itself.
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
		for _, tt := range []struct {
			name string
			tok  theme.ColorToken
			bg   driver.Color
		}{
			{"SliderTrack", c.SliderTrack, window},
			{"SliderTickMark", c.SliderTickMark, window},
			{"ProgressTrack", c.ProgressTrack, window},
			{"SpinnerSpoke", c.SpinnerSpoke, window},
			{"SliderKnobBorder", c.SliderKnobBorder, c.SliderKnobFill.Value(a)},
		} {
			if !tt.tok.HasContrast {
				t.Errorf("%s has no increased-contrast variant", tt.name)
				continue
			}
			n, h := on(tt.tok.Value(a), tt.bg), on(tt.tok.Value(hc), tt.bg)
			if n < 0.05 || h <= n {
				t.Errorf("dark=%v %s: luma difference %.3f, with increased contrast %.3f", dark, tt.name, n, h)
			}
		}
		// The filled part stands out from the empty track.
		if fill := th.ValueFill(a, true, false); on(c.SliderTrack.Value(a), window) == on(fill, window) {
			t.Errorf("dark=%v: the filled track looks like the empty one", dark)
		}
	}
}

// TestValueFill checks ValueFill against the accent rules, with another
// accent than the default so a stored accent value would show.
func TestValueFill(t *testing.T) {
	th := &theme.Sequoia
	alpha := float32(th.Controls.ValueDisabledAlpha)
	faded := func(c driver.Color) driver.Color { c.A *= alpha; return c }
	for _, dark := range []bool{false, true} {
		a := driver.DefaultAppearance()
		a.Dark = dark
		a.Accent = driver.RGB8(200, 60, 120)
		for _, tt := range []struct {
			name          string
			key, disabled bool
			want          driver.Color
		}{
			{"key", true, false, a.Accent},
			{"inactive", false, false, th.AccentInactive(a)},
			{"disabled", true, true, faded(a.Accent)},
			{"disabled inactive", false, true, faded(th.AccentInactive(a))},
		} {
			if got := th.ValueFill(a, tt.key, tt.disabled); got != tt.want {
				t.Errorf("dark=%v ValueFill %s = %v, want %v", dark, tt.name, got, tt.want)
			}
		}
		if got := th.ValueFill(a, true, true); got.A >= a.Accent.A || got.A <= 0 {
			t.Errorf("dark=%v: disabled alpha %v", dark, got.A)
		}
	}
}
