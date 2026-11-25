package theme_test

import (
	"testing"

	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/driver"
)

// TestSequoiaPopUpMetrics checks the geometry of the pop-up tokens
// (S5.5j) for every control size: the indicator fits the bezel with its
// inset, both chevrons and their gap fit the indicator, the strokes are
// thin, and the padding, the gap, the indicator and its inset together
// stay within the stub's 32 pt allowance over the widest title
// (stub.PopUpButton's fitting size, which the element keeps), so a
// fitted button shows its title whole where the title is as wide as the
// stub assumes.
func TestSequoiaPopUpMetrics(t *testing.T) {
	th := &theme.Sequoia
	p := &th.PopUp
	if p.IndicatorInset < 1 || p.IndicatorInset > 4 || p.IndicatorGap < 2 || p.IndicatorGap > 8 {
		t.Errorf("indicator inset %v, gap %v", p.IndicatorInset, p.IndicatorGap)
	}
	if p.ChevronStroke <= 0 || p.ChevronStroke > 2 || p.ChevronGap < 0 || p.ChevronGap > 4 {
		t.Errorf("chevron stroke %v, gap %v", p.ChevronStroke, p.ChevronGap)
	}
	sizes := []driver.ControlSize{driver.ControlSizeMini, driver.ControlSizeSmall, driver.ControlSizeRegular, driver.ControlSizeLarge}
	prev := 0.0
	for _, cs := range sizes {
		m := th.Metrics.Control(cs)
		iw, cw := p.IndicatorWidth.At(cs), p.ChevronWidth.At(cs)
		ih := m.Height - 2*p.IndicatorInset
		if ih <= 0 || iw <= 0 || iw > 1.5*ih || ih > 1.5*iw {
			t.Errorf("size %v: indicator %vx%v", cs, iw, ih)
		}
		if cw+p.ChevronStroke > iw || cw+p.ChevronGap+p.ChevronStroke > ih {
			t.Errorf("size %v: chevrons %v (gap %v) in a %vx%v indicator", cs, cw, p.ChevronGap, iw, ih)
		}
		if p.IndicatorInset >= m.CornerRadius+1 {
			t.Errorf("size %v: indicator inset %v against radius %v", cs, p.IndicatorInset, m.CornerRadius)
		}
		if pad := p.PaddingX.At(cs); pad <= 0 || pad+p.IndicatorGap+iw+p.IndicatorInset > 32 {
			t.Errorf("size %v: padding %v + gap + indicator %v + inset over the stub's 32 pt", cs, pad, iw)
		}
		if iw < prev {
			t.Errorf("size %v: indicator %v after %v", cs, iw, prev)
		}
		prev = iw
	}
}

// TestSequoiaPopUpColors checks the indicator's fill and the chevrons in
// light and dark, for several accents: the accent (pressed: AccentPressed)
// in a key window with AccentText chevrons that read on it; no fill in an
// inactive window or while disabled, with the chevrons in the title's
// color, dimmed when disabled.
func TestSequoiaPopUpColors(t *testing.T) {
	th := &theme.Sequoia
	accents := []driver.Color{driver.DefaultAccent, driver.RGB8(255, 59, 48), driver.RGB8(142, 142, 147), driver.RGB8(255, 204, 0)}
	for _, dark := range []bool{false, true} {
		for _, acc := range accents {
			a := driver.Appearance{Dark: dark, Accent: acc}
			fill, ok := th.PopUpIndicatorFill(a, false, true, false)
			if !ok || fill != acc {
				t.Errorf("dark=%v: key fill %v %v, want the accent %v", dark, fill, ok, acc)
			}
			if fill, ok := th.PopUpIndicatorFill(a, true, true, false); !ok || fill != th.AccentPressed(a) {
				t.Errorf("dark=%v: pressed fill %v %v", dark, fill, ok)
			}
			if d := onLuma(th.PopUpChevron(a, true, false), acc); d < 0.15 {
				t.Errorf("dark=%v accent %v: chevrons on the accent: %.3f", dark, acc, d)
			}
			for _, c := range []struct{ key, disabled bool }{{false, false}, {true, true}, {false, true}} {
				if _, ok := th.PopUpIndicatorFill(a, true, c.key, c.disabled); ok {
					t.Errorf("dark=%v %+v: a filled indicator", dark, c)
				}
				if got, want := th.PopUpChevron(a, c.key, c.disabled), th.PopUpText(a, c.disabled); got != want {
					t.Errorf("dark=%v %+v: chevrons %v, title %v", dark, c, got, want)
				}
			}
		}
		a := driver.Appearance{Dark: dark, Accent: driver.DefaultAccent}
		bezel := over(th.ButtonFill(a, false, false, true), th.Resolve(driver.Semantic(driver.ColorWindowBackground), a))
		text, disabled := th.PopUpText(a, false), th.PopUpText(a, true)
		if d, e := onLuma(text, bezel), onLuma(disabled, bezel); d < 0.45 || e < 0.1 || e >= d {
			t.Errorf("dark=%v: title %.3f, disabled %.3f on the bezel", dark, d, e)
		}
	}
}
