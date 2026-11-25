package gtk4

import (
	"math"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
)

// fakeSettings is a settingsSource over maps; a missing entry is unset.
type fakeSettings struct {
	portal map[string]any
	gtk    map[string]bool
	ints   map[string]int32 // the int GtkSettings
}

func (f *fakeSettings) portalValue(key string) (any, bool) {
	v, ok := f.portal[key]
	return v, ok
}

func (f *fakeSettings) gtkBool(name string) (bool, bool) {
	v, ok := f.gtk[name]
	return v, ok
}

func (f *fakeSettings) gtkInt(name string) (int32, bool) {
	v, ok := f.ints[name]
	return v, ok
}

func TestReadAppearance(t *testing.T) {
	def := driver.DefaultAppearance()
	with := func(fn func(a *driver.Appearance)) driver.Appearance {
		a := def
		fn(&a)
		return a
	}
	dark := with(func(a *driver.Appearance) { a.Dark = true })
	reduced := with(func(a *driver.Appearance) { a.ReduceMotion = true })
	tests := []struct {
		name string
		src  fakeSettings
		want driver.Appearance
	}{
		{"nothing readable (no portal, no GtkSettings)", fakeSettings{}, def},

		// color-scheme
		{"color-scheme 0 is light", fakeSettings{portal: map[string]any{keyColorScheme: uint32(0)}}, def},
		{"color-scheme 1 is dark", fakeSettings{portal: map[string]any{keyColorScheme: uint32(1)}}, dark},
		{"color-scheme 2 is light", fakeSettings{portal: map[string]any{keyColorScheme: uint32(2)}}, def},
		{"color-scheme 3 is unset", fakeSettings{portal: map[string]any{keyColorScheme: uint32(3)}}, def},
		{"color-scheme of another type is unset", fakeSettings{portal: map[string]any{keyColorScheme: int32(1)}}, def},
		{"color-scheme beats prefer-dark", fakeSettings{
			portal: map[string]any{keyColorScheme: uint32(0)},
			gtk:    map[string]bool{gtkPreferDark: true},
		}, def},
		{"color-scheme 1 beats prefer-dark false", fakeSettings{
			portal: map[string]any{keyColorScheme: uint32(1)},
			gtk:    map[string]bool{gtkPreferDark: false},
		}, dark},
		{"no portal: prefer-dark", fakeSettings{gtk: map[string]bool{gtkPreferDark: true}}, dark},
		{"no portal: prefer-dark false", fakeSettings{gtk: map[string]bool{gtkPreferDark: false}}, def},
		{"unknown color-scheme: prefer-dark", fakeSettings{
			portal: map[string]any{keyColorScheme: uint32(7)},
			gtk:    map[string]bool{gtkPreferDark: true},
		}, dark},

		// accent-color
		{"accent-color", fakeSettings{portal: map[string]any{keyAccentColor: [3]float64{0.25, 0.5, 1}}},
			with(func(a *driver.Appearance) { a.Accent = driver.RGBA(0.25, 0.5, 1, 1) })},
		{"accent-color bounds", fakeSettings{portal: map[string]any{keyAccentColor: [3]float64{0, 1, 0}}},
			with(func(a *driver.Appearance) { a.Accent = driver.RGBA(0, 1, 0, 1) })},
		{"accent-color unset (out of range)", fakeSettings{portal: map[string]any{keyAccentColor: [3]float64{-1, -1, -1}}}, def},
		{"accent-color one component above 1", fakeSettings{portal: map[string]any{keyAccentColor: [3]float64{0.5, 1.01, 0.5}}}, def},
		{"accent-color one component negative", fakeSettings{portal: map[string]any{keyAccentColor: [3]float64{0.5, 0.5, -0.01}}}, def},
		{"accent-color NaN", fakeSettings{portal: map[string]any{keyAccentColor: [3]float64{math.NaN(), 0.5, 0.5}}}, def},
		{"accent-color +Inf", fakeSettings{portal: map[string]any{keyAccentColor: [3]float64{0.5, math.Inf(1), 0.5}}}, def},
		{"accent-color of another type", fakeSettings{portal: map[string]any{keyAccentColor: []float64{0.5, 0.5, 0.5}}}, def},

		// contrast
		{"contrast 0", fakeSettings{portal: map[string]any{keyContrast: uint32(0)}}, def},
		{"contrast 1", fakeSettings{portal: map[string]any{keyContrast: uint32(1)}},
			with(func(a *driver.Appearance) { a.IncreaseContrast = true })},
		{"contrast 2 is unset", fakeSettings{portal: map[string]any{keyContrast: uint32(2)}}, def},
		{"contrast of another type", fakeSettings{portal: map[string]any{keyContrast: true}}, def},

		// reduced-motion and gtk-enable-animations
		{"reduced-motion 1", fakeSettings{portal: map[string]any{keyReducedMotion: uint32(1)}}, reduced},
		{"reduced-motion 0", fakeSettings{portal: map[string]any{keyReducedMotion: uint32(0)}}, def},
		{"reduced-motion 0 beats animations off", fakeSettings{
			portal: map[string]any{keyReducedMotion: uint32(0)},
			gtk:    map[string]bool{gtkEnableAnimations: false},
		}, def},
		{"reduced-motion 1 beats animations on", fakeSettings{
			portal: map[string]any{keyReducedMotion: uint32(1)},
			gtk:    map[string]bool{gtkEnableAnimations: true},
		}, reduced},
		{"older portal: animations off", fakeSettings{
			portal: map[string]any{keyColorScheme: uint32(2)},
			gtk:    map[string]bool{gtkEnableAnimations: false},
		}, reduced},
		{"no portal: animations on", fakeSettings{gtk: map[string]bool{gtkEnableAnimations: true}}, def},
		{"reduced-motion 2: animations off", fakeSettings{
			portal: map[string]any{keyReducedMotion: uint32(2)},
			gtk:    map[string]bool{gtkEnableAnimations: false},
		}, reduced},

		// gtk-overlay-scrolling
		{"overlay scrolling on", fakeSettings{gtk: map[string]bool{gtkOverlayScrolling: true}}, def},
		{"overlay scrolling off", fakeSettings{gtk: map[string]bool{gtkOverlayScrolling: false}},
			with(func(a *driver.Appearance) { a.LegacyScrollers = true })},

		// everything at once
		{"dark, accent, everything reduced", fakeSettings{
			portal: map[string]any{
				keyColorScheme:   uint32(1),
				keyAccentColor:   [3]float64{1, 0, 0},
				keyContrast:      uint32(1),
				keyReducedMotion: uint32(1),
			},
			gtk: map[string]bool{gtkPreferDark: false, gtkEnableAnimations: true, gtkOverlayScrolling: false},
		}, driver.Appearance{Dark: true, Accent: driver.RGBA(1, 0, 0, 1), IncreaseContrast: true,
			ReduceMotion: true, LegacyScrollers: true, CaretBlink: driver.DefaultCaretBlink}},
		{"unrelated keys ignored", fakeSettings{
			portal: map[string]any{"cursor-size": uint32(48), "color-scheme-x": uint32(1)},
			gtk:    map[string]bool{"gtk-keynav-use-caret": true},
		}, def},
	}
	for _, tt := range tests {
		got := readAppearance(&tt.src)
		if got != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
		if got.Accent.A != 1 || got.Accent.Semantic != 0 {
			t.Errorf("%s: accent %+v is not an opaque concrete color", tt.name, got.Accent)
		}
		if got.ReduceTransparency {
			t.Errorf("%s: ReduceTransparency set; Linux has no source for it", tt.name)
		}
	}
}

func TestReadCaretBlink(t *testing.T) {
	const ms = time.Millisecond
	def := driver.DefaultCaretBlink
	tests := []struct {
		name string
		src  fakeSettings
		want driver.CaretBlink
	}{
		{"nothing readable", fakeSettings{}, def},
		{"GTK's defaults", fakeSettings{
			gtk:  map[string]bool{gtkCursorBlink: true},
			ints: map[string]int32{gtkCursorBlinkTime: 1200, gtkCursorBlinkTimeout: 10},
		}, driver.CaretBlink{Period: 1200 * ms, Timeout: 10 * time.Second}},
		{"blink off", fakeSettings{
			gtk:  map[string]bool{gtkCursorBlink: false},
			ints: map[string]int32{gtkCursorBlinkTime: 1200, gtkCursorBlinkTimeout: 3},
		}, driver.CaretBlink{Timeout: 3 * time.Second}},
		{"blink off, nothing else readable", fakeSettings{gtk: map[string]bool{gtkCursorBlink: false}},
			driver.CaretBlink{Timeout: def.Timeout}},
		{"blink time without gtk-cursor-blink", fakeSettings{ints: map[string]int32{gtkCursorBlinkTime: 500}},
			driver.CaretBlink{Period: 500 * ms, Timeout: def.Timeout}},
		{"timeout G_MAXINT: never stops", fakeSettings{ints: map[string]int32{gtkCursorBlinkTimeout: math.MaxInt32}},
			driver.CaretBlink{Period: def.Period}},
		{"0 and negative values keep the defaults", fakeSettings{
			ints: map[string]int32{gtkCursorBlinkTime: 0, gtkCursorBlinkTimeout: -1},
		}, def},
		{"the portal does not count", fakeSettings{
			portal: map[string]any{gtkCursorBlink: false, gtkCursorBlinkTime: uint32(300)},
		}, def},
	}
	for _, tt := range tests {
		if got := readCaretBlink(&tt.src); got != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
		if got := readAppearance(&tt.src).CaretBlink; got != tt.want {
			t.Errorf("%s: readAppearance's CaretBlink %+v, want %+v", tt.name, got, tt.want)
		}
	}
}
