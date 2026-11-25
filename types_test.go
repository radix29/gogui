package gogui

import (
	"math"
	"testing"

	"github.com/radix29/gogui/internal/driver"
)

// semanticColors maps every driver.SemanticColor to its public variable.
// TestSemanticColorsComplete fails when a semantic color is added to the
// driver without a public name here and in color.go.
var semanticColors = map[driver.SemanticColor]Color{
	driver.ColorLabel:                                 ColorLabel,
	driver.ColorSecondaryLabel:                        ColorSecondaryLabel,
	driver.ColorTertiaryLabel:                         ColorTertiaryLabel,
	driver.ColorQuaternaryLabel:                       ColorQuaternaryLabel,
	driver.ColorText:                                  ColorText,
	driver.ColorPlaceholderText:                       ColorPlaceholderText,
	driver.ColorSelectedText:                          ColorSelectedText,
	driver.ColorTextBackground:                        ColorTextBackground,
	driver.ColorSelectedTextBackground:                ColorSelectedTextBackground,
	driver.ColorUnemphasizedSelectedTextBackground:    ColorUnemphasizedSelectedTextBackground,
	driver.ColorWindowBackground:                      ColorWindowBackground,
	driver.ColorUnderPageBackground:                   ColorUnderPageBackground,
	driver.ColorControlBackground:                     ColorControlBackground,
	driver.ColorSelectedContentBackground:             ColorSelectedContentBackground,
	driver.ColorUnemphasizedSelectedContentBackground: ColorUnemphasizedSelectedContentBackground,
	driver.ColorSeparator:                             ColorSeparator,
	driver.ColorGrid:                                  ColorGrid,
	driver.ColorLink:                                  ColorLink,
	driver.ColorFindHighlight:                         ColorFindHighlight,
	driver.ColorControl:                               ColorControl,
	driver.ColorControlText:                           ColorControlText,
	driver.ColorDisabledControlText:                   ColorDisabledControlText,
	driver.ColorSelectedControl:                       ColorSelectedControl,
	driver.ColorSelectedControlText:                   ColorSelectedControlText,
	driver.ColorControlAccent:                         ColorControlAccent,
	driver.ColorKeyboardFocusIndicator:                ColorKeyboardFocusIndicator,
	driver.ColorSystemRed:                             ColorSystemRed,
	driver.ColorSystemOrange:                          ColorSystemOrange,
	driver.ColorSystemYellow:                          ColorSystemYellow,
	driver.ColorSystemGreen:                           ColorSystemGreen,
	driver.ColorSystemMint:                            ColorSystemMint,
	driver.ColorSystemTeal:                            ColorSystemTeal,
	driver.ColorSystemCyan:                            ColorSystemCyan,
	driver.ColorSystemBlue:                            ColorSystemBlue,
	driver.ColorSystemIndigo:                          ColorSystemIndigo,
	driver.ColorSystemPurple:                          ColorSystemPurple,
	driver.ColorSystemPink:                            ColorSystemPink,
	driver.ColorSystemBrown:                           ColorSystemBrown,
	driver.ColorSystemGray:                            ColorSystemGray,
}

func TestSemanticColorsComplete(t *testing.T) {
	for s := driver.SemanticColor(1); s.Valid(); s++ {
		c, ok := semanticColors[s]
		if !ok {
			t.Errorf("driver.SemanticColor %d has no public Color variable", s)
			continue
		}
		if c != driver.Semantic(s) {
			t.Errorf("public color for %d = %+v, want Semantic(%d)", s, c, s)
		}
	}
	if len(semanticColors) != driver.NumSemanticColors {
		t.Errorf("%d public semantic colors, driver has %d", len(semanticColors), driver.NumSemanticColors)
	}
}

func TestConcreteColors(t *testing.T) {
	if got := RGB8(255, 0, 51); got != (Color{R: 1, G: 0, B: 0.2, A: 1}) || got.IsSemantic() {
		t.Errorf("RGB8 = %+v", got)
	}
	if got := RGBA(0.1, 0.2, 0.3, 0.4); got != (Color{R: 0.1, G: 0.2, B: 0.3, A: 0.4}) {
		t.Errorf("RGBA = %+v", got)
	}
}

func TestFontHelpers(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  Font
		want Font
	}{
		{"SystemFont(13)", SystemFont(13), Font{Size: 13}},
		{"SystemFont(0)", SystemFont(0), Font{}},
		{"SystemFont(-2)", SystemFont(-2), Font{}},
		{"SystemFont(NaN)", SystemFont(math.NaN()), Font{}},
		{"BoldSystemFont(15)", BoldSystemFont(15), Font{Size: 15, Weight: FontWeightBold}},
		{"BoldSystemFont(0)", BoldSystemFont(0), Font{Weight: FontWeightBold}},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %+v, want %+v", tc.name, tc.got, tc.want)
		}
		if !tc.got.IsSystem() {
			t.Errorf("%s is not a system font", tc.name)
		}
	}
	// The zero size falls back to the control size's default.
	if got := SystemFont(0).EffectiveSize(ControlSizeSmall); got != 11 {
		t.Errorf("SystemFont(0) at small size = %v, want 11", got)
	}
	if got := SystemFont(0).EffectiveWeight(); got != FontWeightRegular {
		t.Errorf("SystemFont weight = %v, want regular", got)
	}
}

func TestShortcutNormalizes(t *testing.T) {
	for _, tc := range []struct {
		got, want KeyShortcut
	}{
		{Shortcut(KeyPrimary, 's'), KeyShortcut{Mods: KeyPrimary, Key: 'S'}},
		{Shortcut(KeyPrimary|KeyShift, 'D'), KeyShortcut{Mods: KeyPrimary | KeyShift, Key: 'D'}},
		{Shortcut(KeyPrimary, '+'), KeyShortcut{Mods: KeyPrimary, Key: '+'}},
		{Shortcut(KeyOption, 'é'), KeyShortcut{Mods: KeyOption, Key: 'É'}},
		{Shortcut(0, KeyF5), KeyShortcut{Key: driver.KeyF1 + 4}},
		{Shortcut(KeyControl, KeyLeft), KeyShortcut{Mods: KeyControl, Key: KeyLeft}},
	} {
		if tc.got != tc.want {
			t.Errorf("Shortcut = %+v, want %+v", tc.got, tc.want)
		}
	}
	if got := Shortcut(KeyPrimary, 'q'); got != Shortcut(KeyPrimary, 'Q') {
		t.Errorf("letter case matters: %+v", got)
	}
	if !(KeyShortcut{}).IsZero() {
		t.Error("zero KeyShortcut is not IsZero")
	}
	if KeyF12-KeyF1 != 11 || !KeyF12.IsFunctionKey() {
		t.Errorf("KeyF1..KeyF12 not contiguous: %#x %#x", KeyF1, KeyF12)
	}
	// The public modifiers are the driver's, including the logical one.
	if got := Shortcut(KeyPrimary|KeyShift, 'd').Display(true); got != "⇧⌘D" {
		t.Errorf("mac display = %q", got)
	}
	if got := Shortcut(KeyPrimary|KeyShift, 'd').Display(false); got != "Ctrl+Shift+D" {
		t.Errorf("other display = %q", got)
	}
	if got := Shortcut(KeyCommand|KeyControl|KeyOption, 'K').Display(true); got != "⌃⌥⌘K" {
		t.Errorf("physical modifiers display = %q", got)
	}
}

func TestStandardShortcuts(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  KeyShortcut
		role driver.ItemRole
		mac  string
	}{
		{"Undo", ShortcutUndo(), driver.ItemUndo, "⌘Z"},
		{"Redo", ShortcutRedo(), driver.ItemRedo, "⇧⌘Z"},
		{"Cut", ShortcutCut(), driver.ItemCut, "⌘X"},
		{"Copy", ShortcutCopy(), driver.ItemCopy, "⌘C"},
		{"Paste", ShortcutPaste(), driver.ItemPaste, "⌘V"},
		{"SelectAll", ShortcutSelectAll(), driver.ItemSelectAll, "⌘A"},
		{"Hide", ShortcutHide(), driver.ItemHide, "⌘H"},
		{"HideOthers", ShortcutHideOthers(), driver.ItemHideOthers, "⌥⌘H"},
		{"Quit", ShortcutQuit(), driver.ItemQuit, "⌘Q"},
		{"Minimize", ShortcutMinimize(), driver.ItemMinimize, "⌘M"},
		{"Close", ShortcutClose(), driver.ItemClose, "⌘W"},
	} {
		if want := tc.role.DefaultShortcut(); tc.got != want || want.IsZero() {
			t.Errorf("Shortcut%s = %+v, want %+v", tc.name, tc.got, want)
		}
		if tc.got.Mods&KeyPrimary == 0 {
			t.Errorf("Shortcut%s does not use KeyPrimary", tc.name)
		}
		if got := tc.got.Display(true); got != tc.mac {
			t.Errorf("Shortcut%s on macOS = %q, want %q", tc.name, got, tc.mac)
		}
	}
	if got := ShortcutCopy().Display(false); got != "Ctrl+C" {
		t.Errorf("ShortcutCopy() elsewhere = %q, want Ctrl+C", got)
	}
}

func TestAppearanceToggled(t *testing.T) {
	for _, tc := range []struct {
		in, want Appearance
		str      string
		mode     driver.AppearanceMode
	}{
		{AppearanceSystem, AppearanceSystem, "system", driver.AppearanceSystem},
		{AppearanceLight, AppearanceDark, "light", driver.AppearanceLight},
		{AppearanceDark, AppearanceLight, "dark", driver.AppearanceDark},
		{Appearance(9), Appearance(9), "Appearance(9)", driver.AppearanceSystem},
	} {
		if got := tc.in.Toggled(); got != tc.want {
			t.Errorf("%v.Toggled() = %v, want %v", tc.in, got, tc.want)
		}
		if got := tc.in.String(); got != tc.str {
			t.Errorf("String() = %q, want %q", got, tc.str)
		}
		if got := tc.in.mode(); got != tc.mode {
			t.Errorf("%v.mode() = %v, want %v", tc.in, got, tc.mode)
		}
	}
	if got := effectiveAppearance(driver.Appearance{Dark: true}); got != AppearanceDark {
		t.Errorf("effective dark = %v", got)
	}
	if got := effectiveAppearance(driver.DefaultAppearance()); got != AppearanceLight {
		t.Errorf("effective light = %v", got)
	}
}

// TestEnumValues pins the public constants to the driver values that the
// native drivers map (NSWindowStyleMask, NSControlStateValue, NSControlSize).
func TestEnumValues(t *testing.T) {
	if Titled != 1 || Closable != 2 || Miniaturizable != 4 || Resizable != 8 {
		t.Errorf("window styles = %d %d %d %d", Titled, Closable, Miniaturizable, Resizable)
	}
	if StateOff != 0 || StateOn != 1 || StateMixed != -1 {
		t.Errorf("check states = %d %d %d", StateOff, StateOn, StateMixed)
	}
	if ControlSizeRegular != 0 || ControlSizeSmall != 1 || ControlSizeMini != 2 || ControlSizeLarge != 3 {
		t.Error("control sizes do not match NSControlSize")
	}
	if AlertInformational != 0 || AlertWarning == AlertCritical || AlignLeading != 0 {
		t.Error("alert styles or text alignment changed")
	}
	if KeyPrimary&(KeyShift|KeyOption|KeyControl|KeyCommand) != 0 {
		t.Error("KeyPrimary overlaps a physical modifier")
	}
	if got := UniformInsets(20); got != (Insets{Top: 20, Left: 20, Bottom: 20, Right: 20}) {
		t.Errorf("UniformInsets = %+v", got)
	}
	_ = Rect{X: 1, Y: 2, W: 3, H: 4}.Contains(Point{X: 1, Y: 2})
}
