package theme_test

import (
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/driver"
)

// TestSequoiaTooltipMetrics checks the tooltip tokens (S5.5l): a thin
// border inside a small radius, padding that keeps the text off the
// rounded corners, a font a little smaller than the regular system font,
// room for some words before the wrap, the panel below the arrow cursor,
// and a delay longer than the grace.
func TestSequoiaTooltipMetrics(t *testing.T) {
	th := &theme.Sequoia
	tt := &th.Tooltip
	if tt.BorderWidth <= 0 || tt.BorderWidth > 1.5 || tt.CornerRadius < 0 || tt.CornerRadius > 8 {
		t.Errorf("border %v, radius %v", tt.BorderWidth, tt.CornerRadius)
	}
	if tt.ShadowBlur < 0 || tt.ShadowOffsetY < 0 || tt.ShadowOffsetY > tt.ShadowBlur {
		t.Errorf("shadow offset %v, blur %v", tt.ShadowOffsetY, tt.ShadowBlur)
	}
	if tt.PaddingX < tt.BorderWidth || tt.PaddingX > 12 || tt.PaddingY < tt.BorderWidth || tt.PaddingY > 8 ||
		tt.PaddingX+tt.PaddingY < tt.CornerRadius {
		t.Errorf("padding %v×%v against radius %v", tt.PaddingX, tt.PaddingY, tt.CornerRadius)
	}
	if regular := th.Typography.FontSize.At(driver.ControlSizeRegular); tt.FontSize < 9 || tt.FontSize > regular {
		t.Errorf("font size %v (regular %v)", tt.FontSize, regular)
	}
	if tt.MaxWidth < 20*tt.FontSize || tt.MaxWidth > 600 {
		t.Errorf("max width %v", tt.MaxWidth)
	}
	if tt.OffsetY < 12 || tt.OffsetY > 32 || tt.OffsetX < -8 || tt.OffsetX > 16 {
		t.Errorf("offset %v,%v", tt.OffsetX, tt.OffsetY)
	}
	if tt.Delay < 300*time.Millisecond || tt.Delay > 2*time.Second || tt.Grace <= 0 || tt.Grace >= tt.Delay {
		t.Errorf("delay %v, grace %v", tt.Delay, tt.Grace)
	}
}

// TestSequoiaTooltipColors: an opaque panel with readable text and a
// visible border in light, dark and increased contrast.
func TestSequoiaTooltipColors(t *testing.T) {
	tt := &theme.Sequoia.Tooltip
	for _, dark := range []bool{false, true} {
		for _, hc := range []bool{false, true} {
			a := driver.DefaultAppearance()
			a.Dark, a.IncreaseContrast = dark, hc
			bg := tt.Background.Value(a)
			if bg.A != 1 {
				t.Errorf("dark=%v hc=%v: Background %v is not opaque", dark, hc, bg)
			}
			if d := onLuma(tt.Text.Value(a), bg); d < 0.6 {
				t.Errorf("dark=%v hc=%v: text luma difference %.3f", dark, hc, d)
			}
			if d := onLuma(tt.Border.Value(a), bg); d < 0.1 {
				t.Errorf("dark=%v hc=%v: border luma difference %.3f", dark, hc, d)
			}
			if s := tt.Shadow.Value(a); s.A <= 0 || s.A > 0.6 {
				t.Errorf("dark=%v: shadow %v", dark, s)
			}
		}
	}
	for name, tok := range map[string]theme.ColorToken{"Background": tt.Background, "Border": tt.Border, "Text": tt.Text} {
		if !tok.HasContrast {
			t.Errorf("%s has no increased-contrast variant", name)
		}
	}
}
