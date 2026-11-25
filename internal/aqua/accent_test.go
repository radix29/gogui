package aqua

import (
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/render/record"
	"github.com/radix29/gogui/internal/shell/fake"
)

// TestDriverAccentOverride checks the app's accent (S6.1g): set before Run
// it is the accent at Ready, unreported; it replaces the OS accent of the
// effective appearance and of every window, made opaque, and survives the
// OS accent changing; a semantic override resolves against the merged
// appearance (light and dark, increased contrast) with the default accent
// for the accent colors, so ColorSystemBlue is blue and not the OS accent;
// the zero Color brings the OS accent back. Every change is reported once.
func TestDriverAccentOverride(t *testing.T) {
	sh := fake.New()
	osAccent := driver.RGB8(1, 2, 3)
	osAp := driver.DefaultAppearance()
	osAp.Accent = osAccent
	sh.SetAppearance(osAp)
	d := New(sh)
	pink := driver.RGBA(1, 45.0/255, 85.0/255, 0.5)
	opaque := pink
	opaque.A = 1
	d.SetAccentColor(pink)
	if d.Appearance().Accent != opaque || d.AccentOverride() != pink {
		t.Fatalf("before Run: accent %v, override %v", d.Appearance().Accent, d.AccentOverride())
	}
	h := &appLog{}
	h.onReady = func() {
		if d.Appearance().Accent != opaque || len(h.appearances) != 0 {
			t.Fatalf("at Ready: accent %v, %d changes", d.Appearance().Accent, len(h.appearances))
		}
		w, err := d.NewWindow(driver.WindowConfig{Title: "W", Size: driver.Size{W: 100, H: 50}, Style: allStyles}, &winLog{})
		if err != nil {
			t.Fatal(err)
		}
		aw := w.(*Window)
		w.Show()
		if aw.Appearance().Accent != opaque {
			t.Errorf("window accent %v, want %v", aw.Appearance().Accent, opaque)
		}

		// The OS accent changes under the override: nothing to report.
		os2 := osAp
		os2.Accent = driver.RGB8(9, 9, 9)
		sh.SetAppearance(os2)
		if d.Appearance().Accent != opaque || len(h.appearances) != 0 {
			t.Errorf("OS accent change: accent %v, %d changes", d.Appearance().Accent, len(h.appearances))
		}

		// A semantic override follows light, dark and increased contrast.
		d.SetAccentColor(driver.Semantic(driver.ColorSystemPurple))
		light := d.Appearance().Accent
		want := tokens.Colors[driver.ColorSystemPurple].Light
		want.A = 1
		if light != want || aw.Appearance().Accent != want {
			t.Errorf("purple light %v (window %v), want %v", light, aw.Appearance().Accent, want)
		}
		d.SetAppearance(driver.AppearanceDark)
		want = tokens.Colors[driver.ColorSystemPurple].Dark
		want.A = 1
		if got := d.Appearance().Accent; got != want || got == light {
			t.Errorf("purple dark %v, want %v", got, want)
		}
		hc := os2
		hc.IncreaseContrast = true
		sh.SetAppearance(hc)
		want = tokens.Colors[driver.ColorSystemPurple].Value(d.Appearance())
		want.A = 1
		if got := d.Appearance().Accent; got != want {
			t.Errorf("purple dark contrast %v, want %v", got, want)
		}
		sh.SetAppearance(os2)
		d.SetAppearance(driver.AppearanceSystem)

		// The colors the theme derives from the accent resolve with the
		// default one, not the OS one.
		for _, s := range []driver.SemanticColor{driver.ColorSystemBlue, driver.ColorControlAccent, driver.ColorKeyboardFocusIndicator} {
			d.SetAccentColor(driver.Semantic(s))
			if got := d.Appearance().Accent; got != driver.DefaultAccent {
				t.Errorf("semantic %d: accent %v, want the default accent", s, got)
			}
		}

		// Cleared: the OS accent again.
		d.SetAccentColor(driver.Color{})
		if d.Appearance().Accent != os2.Accent || aw.Appearance().Accent != os2.Accent || d.AccentOverride() != (driver.Color{}) {
			t.Errorf("cleared: accent %v, window %v", d.Appearance().Accent, aw.Appearance().Accent)
		}
		d.Quit()
	}
	if err := runDriver(t, d, h); err != nil {
		t.Fatal(err)
	}
	// Purple, dark, dark with contrast, dark again, light (purple), the
	// default accent (blue; ControlAccent and the focus color give the same),
	// the OS accent.
	if n := len(h.appearances); n != 7 {
		t.Errorf("%d AppearanceChanged calls, want 7: %+v", n, h.appearances)
	}
	for _, a := range h.appearances {
		if a.Accent.A != 1 {
			t.Errorf("reported accent %v not opaque", a.Accent)
		}
	}
}

// accentDarkText is AccentDarkText as the record dumps print it.
var accentDarkText = hexColor(tokens.Accent.AccentDarkText.Dark)

// TestLightAccentLabels paints the controls with a label on an accent fill
// with a light accent in a dark key window, where AccentDarkText cannot
// come from anything else (S6.1g): the default button's title, the check
// mark, the radio dot and the pop-up indicator's chevrons turn dark; with
// the default accent, and in an inactive window, none does.
func TestLightAccentLabels(t *testing.T) {
	for _, c := range []struct{ kind, state string }{
		{"button", "default"},
		{"checkbox", "on"},
		{"checkbox", "mixed"},
		{"radio", "on"},
		{"pop-up button", "normal"},
	} {
		t.Run(c.kind+"/"+c.state, func(t *testing.T) {
			ic := findCase(t, c.kind, c.state)
			s := startShell(t)
			d := newDriver(s)
			w, sf, _ := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 200, H: 120}, Style: allStyles})
			ap := driver.Appearance{Dark: true, Accent: driver.DefaultAccent, CaretBlink: driver.DefaultCaretBlink}
			w.SetAppearance(ap)
			e := ElementOf(ic.build(t, d, w))
			dump := func(accent driver.Color) string {
				a := ap
				a.Accent = accent
				w.SetAppearance(a)
				paint(t, s, sf)
				rc := record.New(render.Size{W: 200, H: 148})
				PaintTree(rc, e)
				if err := rc.End(); err != nil {
					t.Fatal(err)
				}
				return rc.Dump()
			}
			if got := dump(driver.DefaultAccent); strings.Contains(got, accentDarkText) {
				t.Errorf("dark text with the default accent:\n%s", got)
			}
			if got := dump(lightAccent); !strings.Contains(got, accentDarkText) {
				t.Errorf("no dark text with a light accent:\n%s", got)
			}
			s.Deactivate()
			if got := dump(lightAccent); strings.Contains(got, accentDarkText) {
				t.Errorf("dark text in an inactive window:\n%s", got)
			}
		})
	}
}

// TestLightAccentMenuHighlight: a highlighted menu item's title and
// checkmark are HighlightedText on the default accent and AccentDarkText
// on a light one, in a dark appearance (S6.1g).
func TestLightAccentMenuHighlight(t *testing.T) {
	for _, accent := range []driver.Color{driver.DefaultAccent, lightAccent} {
		var log []string
		w, _, s, _ := popupWindow(t, &log)
		w.SetAppearance(driver.Appearance{Dark: true, Accent: accent})
		m := newMenu(s.TextSystem())
		m.SetItems(sampleMenuItems)
		m.SetHighlighted(1) // checked
		p := openMenuPopupSized(t, w, m, 0)
		got := popupDump(t, s, p)
		if dark := strings.Contains(got, accentDarkText); dark != (accent == lightAccent) {
			t.Errorf("accent %v: dark text %v:\n%s", accent, dark, got)
		}
		p.Close()
	}
}
