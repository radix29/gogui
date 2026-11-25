package theme_test

import (
	"testing"

	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/driver"
)

// TestSequoiaAlertMetrics checks the alert tokens (S5.7a): a panel about
// as wide as NSAlert's, room for two buttons side by side, an icon that
// fits with its badge, corners that stay off the padding, and a detail
// font smaller than the message's.
func TestSequoiaAlertMetrics(t *testing.T) {
	th := &theme.Sequoia
	at := &th.Alert
	if at.Width < 220 || at.Width > 340 {
		t.Errorf("width %v", at.Width)
	}
	if at.BorderWidth <= 0 || at.BorderWidth > 1.5 || at.CornerRadius < 4 || at.CornerRadius > at.PaddingX+at.PaddingTop {
		t.Errorf("border %v, radius %v", at.BorderWidth, at.CornerRadius)
	}
	if at.ShadowBlur <= 0 || at.ShadowOffsetY < 0 || at.ShadowOffsetY > at.ShadowBlur {
		t.Errorf("shadow offset %v, blur %v", at.ShadowOffsetY, at.ShadowBlur)
	}
	if at.PaddingX < at.CornerRadius || at.PaddingTop <= 0 || at.PaddingBottom < at.CornerRadius/2 {
		t.Errorf("padding %v, %v, %v against radius %v", at.PaddingX, at.PaddingTop, at.PaddingBottom, at.CornerRadius)
	}
	if at.IconSize < 32 || at.IconSize > at.Width/2 || at.IconCornerRadius <= 0 || at.IconCornerRadius > at.IconSize/2 ||
		at.BadgeSize <= 0 || at.BadgeSize >= at.IconSize {
		t.Errorf("icon %v, radius %v, badge %v", at.IconSize, at.IconCornerRadius, at.BadgeSize)
	}
	regular := th.Typography.FontSize.At(driver.ControlSizeRegular)
	if at.MessageFontSize < regular || at.MessageFontSize > 2*regular || at.DetailFontSize < 9 || at.DetailFontSize > at.MessageFontSize {
		t.Errorf("font sizes %v, %v", at.MessageFontSize, at.DetailFontSize)
	}
	minButton := th.Controls.ButtonMinWidth.At(driver.ControlSizeRegular)
	if room := at.Width - 2*at.PaddingX - at.ButtonGap; room < 2*minButton {
		t.Errorf("room %v for two buttons of at least %v", room, minButton)
	}
	for name, v := range map[string]float64{"IconGap": at.IconGap, "DetailGap": at.DetailGap, "ButtonsGap": at.ButtonsGap, "ButtonGap": at.ButtonGap} {
		if v <= 0 || v > 32 {
			t.Errorf("%s %v", name, v)
		}
	}
}

// TestSequoiaAlertColors: an opaque panel with a visible
// border, a placeholder icon distinct from the panel with a visible glyph,
// and a caution mark readable on its triangle, in light, dark and
// increased contrast.
func TestSequoiaAlertColors(t *testing.T) {
	at := &theme.Sequoia.Alert
	for _, dark := range []bool{false, true} {
		for _, hc := range []bool{false, true} {
			a := driver.DefaultAppearance()
			a.Dark, a.IncreaseContrast = dark, hc
			bg := at.Background.Value(a)
			if bg.A != 1 {
				t.Errorf("dark=%v hc=%v: Background %v is not opaque", dark, hc, bg)
			}
			if d := onLuma(at.Border.Value(a), bg); d < 0.1 {
				t.Errorf("dark=%v hc=%v: border luma difference %.3f", dark, hc, d)
			}
			if s := at.Shadow.Value(a); s.A <= 0 || s.A > 0.6 {
				t.Errorf("dark=%v: shadow %v", dark, s)
			}
			top, bottom := at.IconTop.Value(a), at.IconBottom.Value(a)
			if top.A != 1 || bottom.A != 1 || top == bottom {
				t.Errorf("dark=%v: icon gradient %v to %v", dark, top, bottom)
			}
			if d := onLuma(at.IconGlyph.Value(a), bottom); d < 0.2 {
				t.Errorf("dark=%v hc=%v: glyph luma difference %.3f", dark, hc, d)
			}
			fill := at.CautionFill.Value(a)
			if fill.A != 1 || fill.R < fill.B+0.5 {
				t.Errorf("dark=%v: caution fill %v is not an opaque yellow", dark, fill)
			}
			if d := onLuma(at.CautionMark.Value(a), fill); d < 0.4 {
				t.Errorf("dark=%v: caution mark luma difference %.3f", dark, d)
			}
		}
	}
	for name, tok := range map[string]theme.ColorToken{
		"Background": at.Background, "Border": at.Border,
		"IconBorder": at.IconBorder, "IconGlyph": at.IconGlyph,
	} {
		if !tok.HasContrast {
			t.Errorf("%s has no increased-contrast variant", name)
		}
	}
}
