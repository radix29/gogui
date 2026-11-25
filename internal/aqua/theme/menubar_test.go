package theme_test

import (
	"testing"

	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/driver"
)

// TestSequoiaMenuBarMetrics checks the menu strip tokens (S5.6c): the
// highlight fits the title bar and its radius fits the highlight, the
// padding keeps the text off the highlight's corners, and the drag area
// and the title gap are usable but leave room for some titles.
func TestSequoiaMenuBarMetrics(t *testing.T) {
	th := &theme.Sequoia
	mb := &th.MenuBar
	bar := th.Chrome.TitleBarHeight
	if mb.HighlightHeight <= th.Typography.LineHeight*th.Typography.FontSize.At(driver.ControlSizeRegular)-4 ||
		mb.HighlightHeight > bar-2 {
		t.Errorf("highlight height %v in a %v bar", mb.HighlightHeight, bar)
	}
	if mb.HighlightRadius < 0 || mb.HighlightRadius > mb.HighlightHeight/2 {
		t.Errorf("highlight radius %v", mb.HighlightRadius)
	}
	if mb.TitlePaddingX < mb.HighlightRadius/2 || mb.TitlePaddingX > 16 {
		t.Errorf("title padding %v", mb.TitlePaddingX)
	}
	if mb.StartGap < 0 || mb.StartGap > 32 {
		t.Errorf("start gap %v", mb.StartGap)
	}
	if mb.MinDragWidth < 32 || mb.MinDragWidth > 200 || mb.WindowTitleGap < 4 || mb.WindowTitleGap > 48 {
		t.Errorf("drag width %v, title gap %v", mb.MinDragWidth, mb.WindowTitleGap)
	}
}

// TestSequoiaMenuBarColors: readable titles on the title bar, dimmer ones
// in an inactive window, and a visible but light highlight, in light,
// dark and increased contrast.
func TestSequoiaMenuBarColors(t *testing.T) {
	th := &theme.Sequoia
	mb := &th.MenuBar
	for _, dark := range []bool{false, true} {
		for _, hc := range []bool{false, true} {
			a := driver.DefaultAppearance()
			a.Dark, a.IncreaseContrast = dark, hc
			bg := th.Chrome.TitleBar.Value(a)
			text, inactive := mb.Text.Value(a), mb.InactiveText.Value(a)
			if d := onLuma(text, bg); d < 0.6 {
				t.Errorf("dark=%v hc=%v: text luma difference %.3f", dark, hc, d)
			}
			if inactive.A >= text.A {
				t.Errorf("dark=%v hc=%v: inactive text %v not dimmer than %v", dark, hc, inactive, text)
			}
			if h := mb.Highlight.Value(a); h.A <= 0 || h.A > 0.4 {
				t.Errorf("dark=%v hc=%v: highlight %v", dark, hc, h)
			}
		}
	}
}
