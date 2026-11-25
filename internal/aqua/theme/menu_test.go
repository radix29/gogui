package theme_test

import (
	"testing"

	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/driver"
)

// TestSequoiaMenuMetrics checks the ranges and the geometry of the menu
// tokens (S5.5h): every item holds its title line, the highlight and the
// separator stay inside the panel, the checkmark fits its column, and the
// item heights grow with the control size.
func TestSequoiaMenuMetrics(t *testing.T) {
	th := &theme.Sequoia
	m := &th.Menu
	if m.BorderWidth <= 0 || m.BorderWidth > 1.5 || m.SeparatorThickness <= 0 || m.SeparatorThickness > 1.5 {
		t.Errorf("border %v, separator %v", m.BorderWidth, m.SeparatorThickness)
	}
	if m.CornerRadius < 0 || m.CornerRadius > 12 || m.HighlightRadius < 0 {
		t.Errorf("corner radius %v, highlight radius %v", m.CornerRadius, m.HighlightRadius)
	}
	if m.ShadowBlur < 0 || m.ShadowOffsetY < 0 || m.ShadowOffsetY > m.ShadowBlur {
		t.Errorf("shadow offset %v, blur %v", m.ShadowOffsetY, m.ShadowBlur)
	}
	// The rounded corners do not cut into the first and last item, nor
	// into the highlight's sides.
	if m.PaddingY < m.BorderWidth || m.PaddingY > 10 || m.HighlightInsetX <= m.BorderWidth || m.HighlightInsetX > 10 {
		t.Errorf("padding %v, highlight inset %v", m.PaddingY, m.HighlightInsetX)
	}
	if m.CheckmarkSize <= 0 || m.CheckmarkSize > m.CheckColumnWidth || m.CheckmarkStroke <= 0 || m.CheckmarkStroke > m.CheckmarkSize/3 {
		t.Errorf("checkmark %v (stroke %v) in a %v column", m.CheckmarkSize, m.CheckmarkStroke, m.CheckColumnWidth)
	}
	if m.TitlePaddingRight < m.HighlightInsetX {
		t.Errorf("title padding %v inside the highlight inset %v", m.TitlePaddingRight, m.HighlightInsetX)
	}
	if m.SeparatorHeight < m.SeparatorThickness+2 || m.SeparatorHeight > 16 || m.SeparatorInsetX < m.HighlightInsetX {
		t.Errorf("separator height %v, inset %v", m.SeparatorHeight, m.SeparatorInsetX)
	}
	// A scroll arrow fits its strip, which is lower than the least item
	// with the padding, so a scrolled menu of two strips still shows one.
	if m.ScrollArrowSize <= 0 || m.ScrollArrowSize/2 >= m.ScrollArrowHeight || m.ScrollArrowHeight > m.PaddingY+m.ItemHeight.At(driver.ControlSizeMini) {
		t.Errorf("scroll arrow %v in a %v strip", m.ScrollArrowSize, m.ScrollArrowHeight)
	}
	// The shortcut column keeps clear of the titles, and a submenu chevron
	// fits the least item and the right padding (S5.6b).
	if m.ShortcutGap < m.TitlePaddingRight/2 || m.ShortcutGap > 48 {
		t.Errorf("shortcut gap %v", m.ShortcutGap)
	}
	if m.SubmenuChevronSize <= 0 || m.SubmenuChevronSize > m.ItemHeight.At(driver.ControlSizeMini)-4 ||
		m.SubmenuChevronStroke <= 0 || m.SubmenuChevronStroke > m.SubmenuChevronSize/3 {
		t.Errorf("submenu chevron %v (stroke %v)", m.SubmenuChevronSize, m.SubmenuChevronStroke)
	}
	sizes := []driver.ControlSize{driver.ControlSizeMini, driver.ControlSizeSmall, driver.ControlSizeRegular, driver.ControlSizeLarge}
	prev := 0.0
	for _, cs := range sizes {
		h := m.ItemHeight.At(cs)
		if line := th.Typography.LineHeight * th.Typography.FontSize.At(cs); line > h {
			t.Errorf("size %v: a %v line in a %v item", cs, line, h)
		}
		if h <= prev || h > 30 {
			t.Errorf("size %v: item height %v after %v", cs, h, prev)
		}
		prev = h
	}
}

// TestSequoiaMenuColors checks the menu colors in light and dark, with and
// without Increase Contrast: the opaque background is opaque and the
// translucent one is not, the texts read on both (disabled ones less),
// the highlighted text on the accent of every accent, and the border and
// separator are visible on the opaque background, more so with Increase
// Contrast.
func TestSequoiaMenuColors(t *testing.T) {
	th := &theme.Sequoia
	m := &th.Menu
	accents := []driver.Color{driver.DefaultAccent, driver.RGB8(255, 59, 48), driver.RGB8(40, 205, 65), driver.RGB8(142, 142, 147), driver.RGB8(255, 204, 0)}
	for _, dark := range []bool{false, true} {
		for _, hc := range []bool{false, true} {
			a := driver.DefaultAppearance()
			a.Dark, a.IncreaseContrast = dark, hc
			opaque := m.OpaqueBackground.Value(a)
			if opaque.A != 1 {
				t.Errorf("dark=%v hc=%v: OpaqueBackground %v is not opaque", dark, hc, opaque)
			}
			bg := m.Background.Value(a)
			if bg.A <= 0.2 || bg.A >= 1 {
				t.Errorf("dark=%v hc=%v: Background alpha %v", dark, hc, bg.A)
			}
			// The tint over a backdrop of the other appearance's extreme
			// still leaves the text readable.
			base := driver.RGB8(255, 255, 255)
			if !dark {
				base = driver.RGB8(0, 0, 0)
			}
			tinted := over(bg, base)
			text, disabled := m.Text.Value(a), m.DisabledText.Value(a)
			for _, b := range []driver.Color{opaque, tinted} {
				if d := onLuma(text, b); d < 0.45 {
					t.Errorf("dark=%v hc=%v: text on %v: luma difference %.3f", dark, hc, b, d)
				}
				if d, e := onLuma(disabled, b), onLuma(text, b); d < 0.12 || d >= e {
					t.Errorf("dark=%v hc=%v: disabled text on %v: %.3f (text %.3f)", dark, hc, b, d, e)
				}
			}
			for _, acc := range accents {
				a.Accent = acc
				hl := th.MenuHighlight(a)
				if hl != acc || hl.A != 1 {
					t.Errorf("dark=%v: MenuHighlight %v, want the accent %v", dark, hl, acc)
				}
				if d := onLuma(m.HighlightedText.Value(a), hl); d < 0.15 {
					t.Errorf("dark=%v accent %v: highlighted text luma difference %.3f", dark, acc, d)
				}
			}
			for name, tok := range map[string]theme.ColorToken{"Border": m.Border, "Separator": m.Separator} {
				if !tok.HasContrast {
					t.Errorf("%s has no increased-contrast variant", name)
				}
				d := onLuma(tok.Value(a), opaque)
				if d < 0.04 {
					t.Errorf("dark=%v hc=%v: %s on the background: %.3f", dark, hc, name, d)
				}
				if hc {
					plain := a
					plain.IncreaseContrast = false
					if p := onLuma(tok.Value(plain), m.OpaqueBackground.Value(plain)); d <= p {
						t.Errorf("dark=%v: %s with Increase Contrast %.3f, without %.3f", dark, name, d, p)
					}
				}
			}
			if s := m.Shadow.Value(a); s.A <= 0 || s.A >= 1 || s.R != 0 || s.G != 0 || s.B != 0 {
				t.Errorf("dark=%v: shadow %v", dark, s)
			}
		}
	}
}

// over composites fg over the opaque bg.
func over(fg, bg driver.Color) driver.Color {
	mix := func(s, d float32) float32 { return s*fg.A + d*(1-fg.A) }
	return driver.Color{R: mix(fg.R, bg.R), G: mix(fg.G, bg.G), B: mix(fg.B, bg.B), A: 1}
}
