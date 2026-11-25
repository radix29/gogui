package theme_test

import (
	"math"
	"testing"

	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/driver"
)

// lumaDiff returns the luma difference between fg composited over the
// opaque bg and bg itself.
func lumaDiff(fg, bg driver.Color) float64 {
	luma := func(x driver.Color) float64 { return 0.2126*float64(x.R) + 0.7152*float64(x.G) + 0.0722*float64(x.B) }
	return math.Abs(luma(over(fg, bg)) - luma(bg))
}

// TestContrastOutlines checks the outline tokens of Increase Contrast
// (S6.1d): transparent without it, so the normal look keeps no outline,
// and with it a visible line on every fill it is drawn around (the accent
// fill of a default button, a checked box and an on radio, the pop-up's
// indicator; the slider's empty and filled track), light and dark, with
// the default and with a light and a dark accent.
func TestContrastOutlines(t *testing.T) {
	th := &theme.Sequoia
	c := &th.Controls
	for _, accent := range []driver.Color{driver.DefaultAccent, driver.RGB8(255, 214, 10), driver.RGB8(60, 20, 90)} {
		for _, dark := range []bool{false, true} {
			a := driver.DefaultAppearance()
			a.Dark, a.Accent = dark, accent
			hc := a
			hc.IncreaseContrast = true
			window := th.Resolve(driver.Semantic(driver.ColorWindowBackground), a)
			for _, tt := range []struct {
				name string
				tok  theme.ColorToken
				bgs  []driver.Color // the fills the outline is drawn on
			}{
				{"BezelOutline", c.BezelOutline, []driver.Color{
					over(th.ButtonFill(hc, true, false, true), window),
					over(th.ToggleFill(hc, true, false, true), window),
					over(th.ToggleFill(hc, true, false, false), window),
				}},
				{"SliderTrackOutline", c.SliderTrackOutline, []driver.Color{
					over(c.SliderTrack.Value(hc), window),
					over(th.ValueFill(hc, true, false), window),
					over(th.ValueFill(hc, false, false), window),
				}},
			} {
				if !tt.tok.HasContrast {
					t.Fatalf("%s has no increased-contrast values", tt.name)
				}
				for _, v := range []driver.Color{tt.tok.Light, tt.tok.Dark, tt.tok.ContrastLight, tt.tok.ContrastDark} {
					if v.IsSemantic() {
						t.Errorf("%s has a semantic value", tt.name)
					}
				}
				if v := tt.tok.Value(a); v.A != 0 {
					t.Errorf("dark=%v: %s visible without Increase Contrast: %+v", dark, tt.name, v)
				}
				out := tt.tok.Value(hc)
				if out.A < 0.5 {
					t.Errorf("dark=%v: %s alpha %v, want a solid outline", dark, tt.name, out.A)
				}
				if d := lumaDiff(out, window); d < 0.1 {
					t.Errorf("accent=%v dark=%v: %s on the window: luma difference %.3f", accent, dark, tt.name, d)
				}
				// On the fill it is drawn on, unless the fill itself already
				// stands out strongly from the window (a dark accent in light
				// mode, where a dark outline adds little and needs nothing).
				for i, bg := range tt.bgs {
					if d := lumaDiff(out, bg); d < 0.1 && lumaDiff(bg, window) < 0.3 {
						t.Errorf("accent=%v dark=%v: %s on fill %d: luma difference %.3f", accent, dark, tt.name, i, d)
					}
				}
			}
		}
	}
	// The outline of an accent fill matches the off bezel's border with
	// Increase Contrast, so a default and a plain button read alike.
	for _, dark := range []bool{false, true} {
		hc := driver.Appearance{Dark: dark, IncreaseContrast: true, Accent: driver.DefaultAccent}
		if c.BezelOutline.Value(hc) != c.BezelBorder.Value(hc) {
			t.Errorf("dark=%v: BezelOutline %+v, BezelBorder %+v", dark, c.BezelOutline.Value(hc), c.BezelBorder.Value(hc))
		}
	}
}

// TestContrastBorders checks the borders and separators Increase Contrast
// darkens (PLAN.md S6.1, the macOS rule): each has its own increased-
// contrast values and stands out more from its backdrop with them, light
// and dark. (Not necessarily more opaque: the dark title-bar separator
// turns from an opaque black hairline into a translucent white one.)
func TestContrastBorders(t *testing.T) {
	th := &theme.Sequoia
	for _, dark := range []bool{false, true} {
		a := driver.DefaultAppearance()
		a.Dark = dark
		hc := a
		hc.IncreaseContrast = true
		window := func(a driver.Appearance) driver.Color {
			return th.Resolve(driver.Semantic(driver.ColorWindowBackground), a)
		}
		text := func(a driver.Appearance) driver.Color {
			return th.Resolve(driver.Semantic(driver.ColorTextBackground), a)
		}
		on := func(fill theme.ColorToken, bg func(driver.Appearance) driver.Color) func(driver.Appearance) driver.Color {
			return func(a driver.Appearance) driver.Color { return over(fill.Value(a), bg(a)) }
		}
		fill := func(tok theme.ColorToken) func(driver.Appearance) driver.Color { return tok.Value }
		ch := &th.Chrome
		for _, tt := range []struct {
			name string
			tok  theme.ColorToken
			bg   func(driver.Appearance) driver.Color
		}{
			{"Controls.BezelBorder", th.Controls.BezelBorder, on(th.Controls.BezelFill, window)},
			{"Controls.BoxBorder", th.Controls.BoxBorder, on(th.Controls.BoxFill, window)},
			{"Controls.SliderKnobBorder", th.Controls.SliderKnobBorder, fill(th.Controls.SliderKnobFill)},
			{"TextField.FieldBorder", th.TextField.FieldBorder, text},
			{"Scroll.LegacyScrollerBorder", th.Scroll.LegacyScrollerBorder, fill(th.Scroll.LegacyScrollerTrack)},
			{"Menu.Border", th.Menu.Border, fill(th.Menu.OpaqueBackground)},
			{"Menu.Separator", th.Menu.Separator, fill(th.Menu.OpaqueBackground)},
			{"Tooltip.Border", th.Tooltip.Border, fill(th.Tooltip.Background)},
			{"Alert.Border", th.Alert.Border, fill(th.Alert.Background)},
			{"Alert.IconBorder", th.Alert.IconBorder, fill(th.Alert.Background)},
			{"Chrome.TitleBarSeparator", ch.TitleBarSeparator, fill(ch.TitleBar)},
			{"Chrome.CloseBorder", ch.CloseBorder, fill(ch.CloseFill)},
			{"Chrome.MinimizeBorder", ch.MinimizeBorder, fill(ch.MinimizeFill)},
			{"Chrome.ZoomBorder", ch.ZoomBorder, fill(ch.ZoomFill)},
			{"Chrome.LightInactiveBorder", ch.LightInactiveBorder, fill(ch.LightInactiveFill)},
			{"ColorSeparator", th.Colors[driver.ColorSeparator], window},
			{"ColorGrid", th.Colors[driver.ColorGrid], text},
		} {
			if !tt.tok.HasContrast {
				t.Errorf("%s has no increased-contrast values", tt.name)
				continue
			}
			n, h := tt.tok.Value(a), tt.tok.Value(hc)
			// Each value on its own backdrop: the panels of menus, tooltips
			// and alerts have a contrast background of their own.
			if nd, hd := lumaDiff(n, tt.bg(a)), lumaDiff(h, tt.bg(hc)); hd <= nd {
				t.Errorf("dark=%v %s: luma difference %.3f, with Increase Contrast %.3f", dark, tt.name, nd, hd)
			}
		}
	}
}

// TestContrastKeepsFills checks the other half of the macOS rule: the
// fills and backgrounds of the controls and the window keep their values
// with Increase Contrast (only the panels' opaque backgrounds, chosen in
// S5.5h, S5.5l and S5.7a, turn white or black).
func TestContrastKeepsFills(t *testing.T) {
	th := &theme.Sequoia
	for name, tok := range map[string]theme.ColorToken{
		"Controls.BezelFill":                      th.Controls.BezelFill,
		"Controls.BezelPressedFill":               th.Controls.BezelPressedFill,
		"Controls.BoxFill":                        th.Controls.BoxFill,
		"Controls.SliderKnobFill":                 th.Controls.SliderKnobFill,
		"Scroll.LegacyScrollerTrack":              th.Scroll.LegacyScrollerTrack,
		"Chrome.TitleBar":                         th.Chrome.TitleBar,
		"Chrome.TitleBarInactive":                 th.Chrome.TitleBarInactive,
		"Chrome.CloseFill":                        th.Chrome.CloseFill,
		"Chrome.MinimizeFill":                     th.Chrome.MinimizeFill,
		"Chrome.ZoomFill":                         th.Chrome.ZoomFill,
		"Chrome.LightInactiveFill":                th.Chrome.LightInactiveFill,
		"ColorWindowBackground":                   th.Colors[driver.ColorWindowBackground],
		"ColorControlBackground":                  th.Colors[driver.ColorControlBackground],
		"ColorTextBackground":                     th.Colors[driver.ColorTextBackground],
		"ColorControl":                            th.Colors[driver.ColorControl],
		"ColorUnderPageBackground":                th.Colors[driver.ColorUnderPageBackground],
		"ColorUnemphasizedSelectedTextBackground": th.Colors[driver.ColorUnemphasizedSelectedTextBackground],
	} {
		if tok.HasContrast {
			t.Errorf("%s has increased-contrast values; fills keep theirs", name)
		}
		for _, dark := range []bool{false, true} {
			a := driver.Appearance{Dark: dark}
			hc := a
			hc.IncreaseContrast = true
			if tok.Value(a) != tok.Value(hc) {
				t.Errorf("dark=%v: %s changes with Increase Contrast", dark, name)
			}
		}
	}
}

// TestContrastSelection checks the accent rules with Increase Contrast
// (S6.1e): the focus ring is the opaque accent, and the selection is the
// accent at the stronger SelectionContrastAlpha, still opaque, standing out
// more from the text background than the normal selection, with the text
// readable on it; the unemphasized selection keeps its token.
func TestContrastSelection(t *testing.T) {
	th := &theme.Sequoia
	if k, n := th.Accent.SelectionContrastAlpha, th.Accent.SelectionAlpha; k <= n || k >= 1 {
		t.Fatalf("SelectionContrastAlpha %v, want in (SelectionAlpha %v, 1)", k, n)
	}
	accents := []driver.Color{driver.DefaultAccent, driver.RGB8(255, 214, 10), driver.RGB8(60, 20, 90),
		driver.RGB8(255, 59, 48), driver.RGB8(40, 205, 65), driver.RGB8(142, 142, 147)}
	for _, dark := range []bool{false, true} {
		for _, acc := range accents {
			a := driver.DefaultAppearance()
			a.Dark, a.Accent = dark, acc
			hc := a
			hc.IncreaseContrast = true
			if f := th.FocusRing(hc); f != acc {
				t.Errorf("dark=%v accent %v: contrast focus ring %v, want the accent", dark, acc, f)
			}
			sel, norm := th.Selection(hc), th.Selection(a)
			if sel.A != 1 {
				t.Errorf("dark=%v accent %v: contrast selection %v not opaque", dark, acc, sel)
			}
			for _, s := range []driver.SemanticColor{driver.ColorSelectedTextBackground, driver.ColorSelectedControl} {
				if c := th.Resolve(driver.Semantic(s), hc); c != sel {
					t.Errorf("dark=%v: Resolve %d = %v, Selection %v", dark, s, c, sel)
				}
			}
			if got := th.TextSelection(hc, true, true); got != sel {
				t.Errorf("dark=%v: TextSelection %v, want %v", dark, got, sel)
			}
			if got, want := th.TextSelection(hc, false, true), th.TextSelection(a, false, true); got != want {
				t.Errorf("dark=%v: unemphasized selection changed with Increase Contrast: %v, want %v", dark, got, want)
			}
			bg := th.Resolve(driver.Semantic(driver.ColorTextBackground), hc)
			if colorDistance(sel, bg) <= colorDistance(norm, bg) {
				t.Errorf("dark=%v accent %v: contrast selection %v not stronger than %v on %v", dark, acc, sel, norm, bg)
			}
			for _, txt := range []driver.SemanticColor{driver.ColorText, driver.ColorSelectedText} {
				if d := onLuma(th.Resolve(driver.Semantic(txt), hc), sel); d < 0.4 {
					t.Errorf("dark=%v accent %v: %v on the contrast selection %v: luma difference %.3f", dark, acc, txt, sel, d)
				}
			}
		}
	}
}
