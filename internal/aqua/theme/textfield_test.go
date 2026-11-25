package theme_test

import (
	"math"
	"testing"

	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/driver"
)

// TestSequoiaTextField checks the ranges of the text field tokens (S5.2e):
// a thin border and caret, a radius and padding that fit the bezel of each
// control size and grow with it, and concrete visible colors.
func TestSequoiaTextField(t *testing.T) {
	th := &theme.Sequoia
	f := &th.TextField
	if f.FieldBorderWidth <= 0 || f.FieldBorderWidth > 1.5 {
		t.Errorf("FieldBorderWidth = %v", f.FieldBorderWidth)
	}
	if f.CaretWidth < 1 || f.CaretWidth > 3 {
		t.Errorf("CaretWidth = %v", f.CaretWidth)
	}
	if f.FieldInsetShadowBlur < 0 || f.FieldInsetShadowOffsetY < 0 || f.FieldInsetShadowOffsetY > 2 {
		t.Errorf("inset shadow offset %v, blur %v", f.FieldInsetShadowOffsetY, f.FieldInsetShadowBlur)
	}
	sizes := []driver.ControlSize{driver.ControlSizeMini, driver.ControlSizeSmall, driver.ControlSizeRegular, driver.ControlSizeLarge}
	prev := 0.0
	for _, cs := range sizes {
		h := th.Metrics.ControlHeight.At(cs)
		r, p := f.FieldCornerRadius.At(cs), f.FieldPaddingX.At(cs)
		if r < 0 || r >= h/2 {
			t.Errorf("size %v: radius %v for a %v bezel", cs, r, h)
		}
		// The padding clears the border and leaves room for the text.
		if p <= f.FieldBorderWidth || p > h/2 {
			t.Errorf("size %v: padding %v (border %v, height %v)", cs, p, f.FieldBorderWidth, h)
		}
		if p < prev {
			t.Errorf("size %v: padding %v below the smaller size's %v", cs, p, prev)
		}
		prev = p
		// The text line fits inside the bezel, with the border on both
		// sides.
		if line := th.Typography.LineHeight * th.Typography.FontSize.At(cs); line > h {
			t.Errorf("size %v: a %v pt line in a %v pt bezel", cs, line, h)
		}
	}
	for name, tok := range map[string]theme.ColorToken{"FieldBorder": f.FieldBorder, "FieldInsetShadow": f.FieldInsetShadow} {
		for _, v := range []driver.Color{tok.Light, tok.Dark, tok.ContrastLight, tok.ContrastDark} {
			if v.IsSemantic() {
				t.Errorf("%s has a semantic value", name)
			}
		}
		if tok.Light.A == 0 || tok.Dark.A == 0 || tok.Light.A >= 1 || tok.Dark.A >= 1 {
			t.Errorf("%s is invisible or opaque: %+v", name, tok)
		}
	}
}

// onLuma returns the luma difference fg makes when composited over the
// opaque bg.
func onLuma(fg, bg driver.Color) float64 {
	luma := func(x driver.Color) float64 { return 0.2126*float64(x.R) + 0.7152*float64(x.G) + 0.0722*float64(x.B) }
	mix := func(s, d float32) float32 { return s*fg.A + d*(1-fg.A) }
	out := driver.Color{R: mix(fg.R, bg.R), G: mix(fg.G, bg.G), B: mix(fg.B, bg.B), A: 1}
	return math.Abs(luma(out) - luma(bg))
}

// TestFieldBorderContrast checks the border on the field's background and
// on the window background, light and dark: it stands out, and stands out
// more with Increase Contrast.
func TestFieldBorderContrast(t *testing.T) {
	th := &theme.Sequoia
	tok := th.TextField.FieldBorder
	if !tok.HasContrast {
		t.Fatal("FieldBorder has no increased-contrast variant")
	}
	for _, dark := range []bool{false, true} {
		a := driver.DefaultAppearance()
		a.Dark = dark
		hc := a
		hc.IncreaseContrast = true
		for _, bg := range []driver.SemanticColor{driver.ColorTextBackground, driver.ColorWindowBackground} {
			b := th.Resolve(driver.Semantic(bg), a)
			n, h := onLuma(tok.Value(a), b), onLuma(tok.Value(hc), b)
			if n < 0.08 || h <= n {
				t.Errorf("dark=%v on %v: luma difference %.3f, with increased contrast %.3f", dark, bg, n, h)
			}
		}
	}
}

// TestTextFieldColors checks the computed colors: the caret is the accent
// (any accent), the selection is Selection only while focused in a key
// window and the unemphasized token otherwise; both selections differ from
// the field's background and keep ColorText and ColorSelectedText
// readable, and so does the caret.
func TestTextFieldColors(t *testing.T) {
	th := &theme.Sequoia
	accents := []driver.Color{driver.DefaultAccent, driver.RGB8(255, 59, 48), driver.RGB8(40, 205, 65), driver.RGB8(142, 142, 147)}
	for _, dark := range []bool{false, true} {
		for _, acc := range accents {
			a := driver.DefaultAppearance()
			a.Dark, a.Accent = dark, acc
			if got := th.Caret(a); got != acc {
				t.Errorf("dark=%v: Caret = %v, want the accent %v", dark, got, acc)
			}
			bg := th.Resolve(driver.Semantic(driver.ColorTextBackground), a)
			unemph := th.Resolve(driver.Semantic(driver.ColorUnemphasizedSelectedTextBackground), a)
			for _, c := range []struct{ focused, key bool }{{true, true}, {true, false}, {false, true}, {false, false}} {
				got := th.TextSelection(a, c.focused, c.key)
				want := unemph
				if c.focused && c.key {
					want = th.Selection(a)
				}
				if got != want {
					t.Errorf("dark=%v accent %v focused=%v key=%v: %v, want %v", dark, acc, c.focused, c.key, got, want)
				}
				if got.A != 1 {
					t.Errorf("dark=%v accent %v: the selection is not opaque: %v", dark, acc, got)
				}
				if d := onLuma(got, bg); d < 0.05 && colorDistance(got, bg) < 0.1 {
					t.Errorf("dark=%v accent %v focused=%v key=%v: selection %v too close to the background %v", dark, acc, c.focused, c.key, got, bg)
				}
				for _, txt := range []driver.SemanticColor{driver.ColorText, driver.ColorSelectedText} {
					if d := onLuma(th.Resolve(driver.Semantic(txt), a), got); d < 0.4 {
						t.Errorf("dark=%v accent %v: %v on the selection %v: luma difference %.3f", dark, acc, txt, got, d)
					}
				}
			}
		}
		// The default caret stands out on the field's background.
		a := driver.DefaultAppearance()
		a.Dark = dark
		if d := onLuma(th.Caret(a), th.Resolve(driver.Semantic(driver.ColorTextBackground), a)); d < 0.2 {
			t.Errorf("dark=%v: the caret's luma difference is %.3f", dark, d)
		}
	}
}

// colorDistance returns the largest channel difference of two colors.
func colorDistance(x, y driver.Color) float64 {
	return math.Max(math.Abs(float64(x.R-y.R)), math.Max(math.Abs(float64(x.G-y.G)), math.Abs(float64(x.B-y.B))))
}

// TestSequoiaTextView checks the ranges of the text view tokens (S5.4f):
// a small container inset and line padding, as NSTextView's 5 pt defaults.
func TestSequoiaTextView(t *testing.T) {
	v := &theme.Sequoia.TextView
	if v.ContainerInsetY < 0 || v.ContainerInsetY > 12 {
		t.Errorf("ContainerInsetY = %v", v.ContainerInsetY)
	}
	if v.LineFragmentPadding < 0 || v.LineFragmentPadding > 12 {
		t.Errorf("LineFragmentPadding = %v", v.LineFragmentPadding)
	}
}
