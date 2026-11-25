package gtk4

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/radix29/gogui/internal/driver"
)

// TestAppearanceChangedEveryKey (S6.1h): each portal key and each watched
// GtkSettings property, changed alone while the shell runs, re-reads the
// whole appearance: the shell's source (the portal over a fake bus, the
// GtkSettings over fake properties, as openSystemSettings joins them in
// New) reports it, the shell posts the re-read and gives exactly one
// AppearanceChanged with exactly that field's new value. Every key the
// portal client watches and every property the GtkSettings watch is
// covered. Reduce Transparency has no Linux source.
func TestAppearanceChangedEveryKey(t *testing.T) {
	// The portal's values before a change: the GtkSettings cases leave
	// color-scheme and reduced-motion unset, so their fallbacks count.
	full := func() map[string]dbus.Variant {
		return map[string]dbus.Variant{
			keyColorScheme:   dbus.MakeVariant(uint32(2)),
			keyAccentColor:   dbus.MakeVariant(struct{ R, G, B float64 }{0.2, 0.4, 0.6}),
			keyContrast:      dbus.MakeVariant(uint32(0)),
			keyReducedMotion: dbus.MakeVariant(uint32(0)),
		}
	}
	partial := func() map[string]dbus.Variant {
		v := full()
		delete(v, keyColorScheme)
		delete(v, keyReducedMotion)
		return v
	}
	type change struct {
		key    string // the portal key or the GtkSettings property
		portal bool   // a SettingChanged, else a notify
		value  any
		want   func(a *driver.Appearance)
	}
	changes := []change{
		{keyColorScheme, true, uint32(1), func(a *driver.Appearance) { a.Dark = true }},
		{keyAccentColor, true, struct{ R, G, B float64 }{0.8, 0.1, 0.3}, func(a *driver.Appearance) { a.Accent = driver.RGBA(0.8, 0.1, 0.3, 1) }},
		{keyContrast, true, uint32(1), func(a *driver.Appearance) { a.IncreaseContrast = true }},
		{keyReducedMotion, true, uint32(1), func(a *driver.Appearance) { a.ReduceMotion = true }},
		{gtkPreferDark, false, true, func(a *driver.Appearance) { a.Dark = true }},
		{gtkEnableAnimations, false, false, func(a *driver.Appearance) { a.ReduceMotion = true }},
		{gtkOverlayScrolling, false, false, func(a *driver.Appearance) { a.LegacyScrollers = true }},
		{gtkCursorBlink, false, false, func(a *driver.Appearance) { a.CaretBlink.Period = 0 }},
		{gtkCursorBlinkTime, false, int32(600), func(a *driver.Appearance) { a.CaretBlink.Period = 600 * time.Millisecond }},
		{gtkCursorBlinkTimeout, false, int32(math.MaxInt32), func(a *driver.Appearance) { a.CaretBlink.Timeout = 0 }},
	}
	var covered []string
	for _, c := range changes {
		covered = append(covered, c.key)
	}
	for _, k := range slices.Concat(portalKeys[:], gtkWatched) {
		if !slices.Contains(covered, k) {
			t.Errorf("%s is watched but has no case", k)
		}
	}
	for _, c := range changes {
		t.Run(c.key, func(t *testing.T) {
			s, l := newTestShell(t)
			base := dispatch.Len()
			values := partial()
			if c.portal {
				values = full()
			}
			cfg, served := fakeBusConfig(t, &fakePortalAll{values: values})
			props := newFakeGtkProps(map[string]bool{
				gtkEnableAnimations: true, gtkOverlayScrolling: true, gtkPreferDark: false, gtkCursorBlink: true,
			}).withInts(map[string]int32{gtkCursorBlinkTime: 1200, gtkCursorBlinkTimeout: 10})
			s.openSettings = openSystemSettings(cfg, func() gtkProps { return props }, nil)
			var want driver.Appearance
			var got []driver.Appearance
			err := run(t, s, &handler{
				ready: func() {
					want = s.Appearance()
					c.want(&want)
					if want == s.Appearance() {
						t.Fatalf("the case changes nothing: %+v", want)
					}
					if c.portal {
						b := <-served
						emitSetting(t, b.srv, portalNamespace, c.key, c.value)
						return // the change arrives from godbus's goroutine
					}
					switch v := c.value.(type) {
					case bool:
						props.set(c.key, v)
					case int32:
						props.setInt(c.key, v)
					}
					props.notify(c.key)
				},
				appearance: func(a driver.Appearance) {
					got = append(got, a)
					// A second report would be posted behind this Quit's
					// drain, so it would still be counted.
					s.Post(s.Quit)
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 || got[0] != want {
				t.Errorf("AppearanceChanged calls %+v, want one with %+v", got, want)
			}
			checkTornDown(t, s, l, base)
		})
	}
}
