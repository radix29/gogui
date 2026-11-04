package theme_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/aqua/theme/gen"
	"github.com/radix29/gogui/internal/driver"
)

// TestGeneratedUpToDate regenerates tokens.go and docs/tokens.md from the
// spec and compares them byte for byte with the committed files, so a spec
// edit without "go generate" fails.
func TestGeneratedUpToDate(t *testing.T) {
	data, err := os.ReadFile("spec/sequoia.json")
	if err != nil {
		t.Fatal(err)
	}
	s, err := gen.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	goSrc, doc, err := gen.Generate(s, "spec/sequoia.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []struct {
		path string
		want []byte
	}{{"tokens.go", goSrc}, {"../../../docs/tokens.md", doc}} {
		got, err := os.ReadFile(f.path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, f.want) {
			t.Errorf("%s is out of date: run go generate ./internal/aqua/theme", f.path)
		}
	}
}

func TestSequoiaComplete(t *testing.T) {
	if theme.Sequoia.Name != "Sequoia" {
		t.Errorf("Name = %q", theme.Sequoia.Name)
	}
	if len(theme.Sequoia.Colors) != driver.NumSemanticColors+1 {
		t.Fatalf("Colors has %d entries, want %d", len(theme.Sequoia.Colors), driver.NumSemanticColors+1)
	}
	if theme.Sequoia.Colors[driver.SemanticNone] != (theme.ColorToken{}) {
		t.Error("the SemanticNone entry is not zero")
	}
	for s := driver.SemanticColor(1); s.Valid(); s++ {
		tok := theme.Sequoia.Colors[s]
		if tok == (theme.ColorToken{}) {
			t.Errorf("semantic color %d has no token", s)
		}
		for _, c := range []driver.Color{tok.Light, tok.Dark, tok.ContrastLight, tok.ContrastDark} {
			if c.IsSemantic() {
				t.Errorf("semantic color %d has a semantic value", s)
			}
		}
	}
}

func TestValue(t *testing.T) {
	l, d := driver.RGB8(1, 1, 1), driver.RGB8(2, 2, 2)
	cl, cd := driver.RGB8(3, 3, 3), driver.RGB8(4, 4, 4)
	plain := theme.ColorToken{Light: l, Dark: d, ContrastLight: cl, ContrastDark: cd}
	contrast := plain
	contrast.HasContrast = true
	tests := []struct {
		tok  theme.ColorToken
		a    driver.Appearance
		want driver.Color
	}{
		{plain, driver.Appearance{}, l},
		{plain, driver.Appearance{Dark: true}, d},
		{plain, driver.Appearance{IncreaseContrast: true}, l},
		{plain, driver.Appearance{Dark: true, IncreaseContrast: true}, d},
		{contrast, driver.Appearance{}, l},
		{contrast, driver.Appearance{Dark: true}, d},
		{contrast, driver.Appearance{IncreaseContrast: true}, cl},
		{contrast, driver.Appearance{Dark: true, IncreaseContrast: true}, cd},
	}
	for i, tt := range tests {
		if got := tt.tok.Value(tt.a); got != tt.want {
			t.Errorf("%d: Value(%+v) = %+v, want %+v", i, tt.a, got, tt.want)
		}
	}
}

func TestColor(t *testing.T) {
	dark := driver.Appearance{Dark: true}
	got, ok := theme.Sequoia.Color(driver.ColorWindowBackground, dark)
	if !ok || got != driver.RGB8(50, 50, 50) {
		t.Errorf("Color(ColorWindowBackground, dark) = %+v, %v", got, ok)
	}
	for _, s := range []driver.SemanticColor{driver.SemanticNone, driver.SemanticColor(driver.NumSemanticColors + 1), 0xffff} {
		if _, ok := theme.Sequoia.Color(s, dark); ok {
			t.Errorf("Color(%d) reported ok", s)
		}
	}
}

func TestSequoiaChrome(t *testing.T) {
	ch := theme.Sequoia.Chrome
	tokens := []theme.ColorToken{
		ch.TitleBar, ch.TitleBarInactive, ch.TitleBarSeparator,
		ch.CloseFill, ch.CloseBorder, ch.CloseGlyph,
		ch.MinimizeFill, ch.MinimizeBorder, ch.MinimizeGlyph,
		ch.ZoomFill, ch.ZoomBorder, ch.ZoomGlyph,
		ch.LightInactiveFill, ch.LightInactiveBorder,
	}
	for i, tok := range tokens {
		if tok == (theme.ColorToken{}) {
			t.Errorf("chrome color %d has no token", i)
		}
		for _, c := range []driver.Color{tok.Light, tok.Dark, tok.ContrastLight, tok.ContrastDark} {
			if c.IsSemantic() {
				t.Errorf("chrome color %d has a semantic value", i)
			}
		}
	}
	if !(ch.LightPressedShade > 0 && ch.LightPressedShade < 1) || !(ch.LightDisabledAlpha > 0 && ch.LightDisabledAlpha < 1) {
		t.Errorf("shade %v, disabled alpha %v", ch.LightPressedShade, ch.LightDisabledAlpha)
	}
}

func TestResolve(t *testing.T) {
	th := &theme.Sequoia
	light := driver.DefaultAppearance()
	dark := driver.Appearance{Dark: true, Accent: driver.RGBA(0.2, 0.4, 0.6, 0.8)}
	for _, a := range []driver.Appearance{light, dark} {
		if c := th.Resolve(driver.RGB8(1, 2, 3), a); c != driver.RGB8(1, 2, 3) {
			t.Errorf("concrete color changed: %+v", c)
		}
		for _, s := range []driver.SemanticColor{driver.ColorControlAccent, driver.ColorSelectedContentBackground, driver.ColorSystemBlue} {
			if c := th.Resolve(driver.Semantic(s), a); c != a.Accent {
				t.Errorf("%d: %+v, want the accent %+v", s, c, a.Accent)
			}
		}
		f := a.Accent
		f.A *= 0.5
		if c := th.Resolve(driver.Semantic(driver.ColorKeyboardFocusIndicator), a); c != f {
			t.Errorf("focus ring %+v, want %+v", c, f)
		}
		for s := driver.SemanticColor(1); s.Valid(); s++ {
			c := th.Resolve(driver.Semantic(s), a)
			if c.IsSemantic() || c == theme.Unknown {
				t.Errorf("%d resolves to %+v", s, c)
			}
		}
		for _, s := range []driver.SemanticColor{driver.SemanticColor(driver.NumSemanticColors + 1), 0xffff} {
			if c := th.Resolve(driver.Semantic(s), a); c != theme.Unknown {
				t.Errorf("unknown %d resolves to %+v", s, c)
			}
		}
	}
	if c := th.Resolve(driver.Semantic(driver.ColorLabel), dark); c != th.Colors[driver.ColorLabel].Dark {
		t.Errorf("dark label %+v", c)
	}
	if c := th.Resolve(driver.Semantic(driver.ColorLabel), light); c != th.Colors[driver.ColorLabel].Light {
		t.Errorf("light label %+v", c)
	}
}

// TestFontSizeMatchesEffectiveSize pins the FontSize tokens against
// driver.Font.EffectiveSize, which the shells and the stub metrics use for a
// zero Size: the two must move together.
func TestFontSizeMatchesEffectiveSize(t *testing.T) {
	ty := &theme.Sequoia.Typography
	sizes := []driver.ControlSize{driver.ControlSizeRegular, driver.ControlSizeSmall, driver.ControlSizeMini, driver.ControlSizeLarge}
	if len(sizes) != theme.NumControlSizes {
		t.Fatalf("NumControlSizes = %d, want %d", theme.NumControlSizes, len(sizes))
	}
	for _, cs := range sizes {
		if got, want := ty.FontSize.At(cs), (driver.Font{}).EffectiveSize(cs); got != want {
			t.Errorf("FontSize[%d] = %v, EffectiveSize = %v", cs, got, want)
		}
	}
	if ty.FontSize.At(driver.ControlSizeRegular) != driver.DefaultFontSize {
		t.Errorf("regular font size %v, want DefaultFontSize", ty.FontSize.At(driver.ControlSizeRegular))
	}
	if got := ty.FontSize.At(driver.ControlSize(9)); got != ty.FontSize.At(driver.ControlSizeRegular) {
		t.Errorf("unknown control size gets %v", got)
	}
}

func TestSequoiaMetrics(t *testing.T) {
	m := &theme.Sequoia.Metrics
	for cs := driver.ControlSize(0); cs < theme.NumControlSizes; cs++ {
		c := m.Control(cs)
		if c.Height <= 0 || c.PaddingX <= 0 || c.CornerRadius <= 0 || c.CornerRadius > c.Height/2 || c.ShadowBlur < 0 {
			t.Errorf("control size %d: %+v", cs, c)
		}
		if c.Height != m.ControlHeight[cs] || c.ShadowOffsetY != m.BezelShadowOffsetY[cs] {
			t.Errorf("Control(%d) does not read the tables", cs)
		}
	}
	h := func(cs driver.ControlSize) float64 { return m.Control(cs).Height }
	if !(h(driver.ControlSizeMini) < h(driver.ControlSizeSmall) && h(driver.ControlSizeSmall) < h(driver.ControlSizeRegular) && h(driver.ControlSizeRegular) < h(driver.ControlSizeLarge)) {
		t.Error("control heights do not grow mini < small < regular < large")
	}
	if m.Control(driver.ControlSize(9)) != m.Control(driver.ControlSizeRegular) {
		t.Error("unknown control size does not get the regular metrics")
	}
	if m.FocusRingWidth <= 0 || m.BezelShadow.Light.IsSemantic() {
		t.Errorf("focus ring %v, shadow %+v", m.FocusRingWidth, m.BezelShadow)
	}
	ch := &theme.Sequoia.Chrome
	if ch.TitleBarHeight <= ch.LightSize || ch.LightStep <= ch.LightSize || ch.LightInset < 0 || ch.TitleGap < 0 ||
		ch.LightBorderWidth <= 0 || ch.LightGlyphStroke <= 0 {
		t.Errorf("chrome metrics %+v", ch)
	}
	ty := &theme.Sequoia.Typography
	if ty.LineHeight < 1 || ty.LineHeight > 2 || ty.WindowTitleWeight != driver.FontWeightSemibold || ty.EmphasizedWeight != driver.FontWeightBold {
		t.Errorf("typography %+v", ty)
	}
	for i := 1; i < len(ty.Tracking); i++ {
		if ty.Tracking[i].Size <= ty.Tracking[i-1].Size {
			t.Errorf("tracking row %d not ascending", i)
		}
	}
}

func TestTracking(t *testing.T) {
	ty := theme.Typography{LineHeight: 1.25, Tracking: []theme.TrackingStep{{10, 20}, {12, 0}, {20, -40}}}
	for _, tt := range []struct{ size, want float64 }{
		{5, 20}, {10, 20}, {11, 10}, {12, 0}, {16, -20}, {20, -40}, {30, -40},
	} {
		if got := ty.TrackingAt(tt.size); got != tt.want {
			t.Errorf("TrackingAt(%v) = %v, want %v", tt.size, got, tt.want)
		}
	}
	if got := ty.TrackingPoints(16); got != -20*16.0/1000 {
		t.Errorf("TrackingPoints(16) = %v", got)
	}
	if got := ty.LineHeightFor(16); got != 20 {
		t.Errorf("LineHeightFor(16) = %v", got)
	}
	if got := (&theme.Typography{}).TrackingAt(13); got != 0 {
		t.Errorf("empty table: %v", got)
	}
	sq := &theme.Sequoia.Typography
	if got := sq.TrackingAt(13); got != -6 {
		t.Errorf("Sequoia tracking at 13 pt = %v", got)
	}
}
