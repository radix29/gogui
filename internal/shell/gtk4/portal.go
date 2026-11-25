package gtk4

import (
	"math"
	"time"

	"github.com/radix29/gogui/internal/driver"
)

// The appearance on Linux (PLAN.md S4.3): the settings portal, with the
// GtkSettings filling what it lacks. Since S4.3a the mapping, since S4.3b
// the portal client (portalclient.go), since S4.3c the shell's source and
// cache (Shell.appearanceChanged), since S4.3d the live GtkSettings
// (gtksettings.go).

// The keys of the portal's org.freedesktop.appearance namespace
// (org.freedesktop.portal.Settings) that readAppearance reads.
const (
	keyColorScheme   = "color-scheme"   // u: 0 no preference, 1 dark, 2 light
	keyAccentColor   = "accent-color"   // (ddd): sRGB, each in 0..1
	keyContrast      = "contrast"       // u: 0 normal, 1 higher
	keyReducedMotion = "reduced-motion" // u: 0 no preference, 1 reduced (newer portals)
)

// The GtkSettings properties that readAppearance reads.
const (
	gtkEnableAnimations = "gtk-enable-animations"
	gtkOverlayScrolling = "gtk-overlay-scrolling"
	gtkPreferDark       = "gtk-application-prefer-dark-theme"

	gtkCursorBlink        = "gtk-cursor-blink"         // gboolean
	gtkCursorBlinkTime    = "gtk-cursor-blink-time"    // gint, ms, one full cycle
	gtkCursorBlinkTimeout = "gtk-cursor-blink-timeout" // gint, s, G_MAXINT: never stops
)

// settingsSource provides the values behind Shell.Appearance. The portal
// client and the GtkSettings provide the real ones; the tests inject
// values. Values are plain Go values, so no D-Bus or GLib type leaks in.
type settingsSource interface {
	// portalValue returns the value of key in org.freedesktop.appearance,
	// decoded from its variant: uint32 for u, [3]float64 for (ddd). ok is
	// false when it is unset: no session bus, no portal, or the key
	// NotFound.
	portalValue(key string) (v any, ok bool)
	// gtkBool returns the boolean property name of the default
	// GtkSettings; ok is false when it cannot be read.
	gtkBool(name string) (v, ok bool)
	// gtkInt returns the int property name of the default GtkSettings; ok
	// is false when it cannot be read.
	gtkInt(name string) (v int32, ok bool)
}

// readAppearance maps the settings onto a driver.Appearance, starting from
// driver.DefaultAppearance(). A portal value that is unset, of another
// type or out of range counts as unset; an unset one falls back to its
// GtkSettings property where there is one, else keeps its default:
//
//   - Dark: color-scheme 1 (0 and 2 are light), else
//     gtk-application-prefer-dark-theme.
//   - Accent: accent-color when every component is in 0..1 (NaN is not),
//     opaque; else driver.DefaultAccent.
//   - IncreaseContrast: contrast 1 (0 is normal).
//   - ReduceMotion: reduced-motion 1 (0 is no preference), else
//     gtk-enable-animations false.
//   - ReduceTransparency: no Linux source, always false.
//   - LegacyScrollers: gtk-overlay-scrolling false.
//   - CaretBlink: readCaretBlink (GtkSettings only; the portal has no
//     caret keys).
func readAppearance(src settingsSource) driver.Appearance {
	a := driver.DefaultAppearance()
	if v, ok := portalEnum(src, keyColorScheme, 2); ok {
		a.Dark = v == 1
	} else if dark, ok := src.gtkBool(gtkPreferDark); ok {
		a.Dark = dark
	}
	if c, ok := portalAccent(src); ok {
		a.Accent = c
	}
	if v, ok := portalEnum(src, keyContrast, 1); ok {
		a.IncreaseContrast = v == 1
	}
	if v, ok := portalEnum(src, keyReducedMotion, 1); ok {
		a.ReduceMotion = v == 1
	} else if on, ok := src.gtkBool(gtkEnableAnimations); ok {
		a.ReduceMotion = !on
	}
	if on, ok := src.gtkBool(gtkOverlayScrolling); ok {
		a.LegacyScrollers = !on
	}
	a.CaretBlink = readCaretBlink(src)
	return a
}

// readCaretBlink maps the GtkSettings' caret properties onto a
// driver.CaretBlink, starting from driver.DefaultCaretBlink:
//
//   - Period: gtk-cursor-blink-time ms (GTK's full cycle) when above 0,
//     and 0 (no blink) when gtk-cursor-blink is false.
//   - Timeout: gtk-cursor-blink-timeout s when above 0; G_MAXINT never
//     stops blinking (Timeout 0).
//
// An unreadable property, or one at 0 or below, keeps its default.
func readCaretBlink(src settingsSource) driver.CaretBlink {
	b := driver.DefaultCaretBlink
	if ms, ok := src.gtkInt(gtkCursorBlinkTime); ok && ms > 0 {
		b.Period = time.Duration(ms) * time.Millisecond
	}
	if on, ok := src.gtkBool(gtkCursorBlink); ok && !on {
		b.Period = 0
	}
	if s, ok := src.gtkInt(gtkCursorBlinkTimeout); ok && s > 0 {
		if s == math.MaxInt32 {
			b.Timeout = 0
		} else {
			b.Timeout = time.Duration(s) * time.Second
		}
	}
	return b
}

// portalEnum returns the u value of key when it is set and at most last.
func portalEnum(src settingsSource, key string, last uint32) (uint32, bool) {
	v, ok := src.portalValue(key)
	if !ok {
		return 0, false
	}
	u, ok := v.(uint32)
	if !ok || u > last {
		return 0, false
	}
	return u, true
}

// portalAccent returns accent-color as an opaque color when it is set and
// every component is in 0..1. The portal reports an unset accent with
// components out of range.
func portalAccent(src settingsSource) (driver.Color, bool) {
	v, ok := src.portalValue(keyAccentColor)
	if !ok {
		return driver.Color{}, false
	}
	rgb, ok := v.([3]float64)
	if !ok {
		return driver.Color{}, false
	}
	for _, c := range rgb {
		if !(c >= 0 && c <= 1) { // NaN fails both
			return driver.Color{}, false
		}
	}
	return driver.RGBA(float32(rgb[0]), float32(rgb[1]), float32(rgb[2]), 1), true
}

// appearanceSource is the settingsSource behind the shell's cached
// appearance: opened once in Run before Ready, closed in shutdown.
type appearanceSource interface {
	settingsSource
	// close stops the source: changed is not called after it returns.
	close()
}

// openSource opens the shell's appearance source. changed is called when a
// value may have changed, on any goroutine (the shell posts the re-read to
// the UI thread), and must not block. The opener runs on the UI thread
// after the loop's init and returns within a short bound; without
// settings it returns a source with every value unset.
type openSource func(changed func()) appearanceSource

// noSettings is the source with every value unset, so the appearance is
// driver.DefaultAppearance(). It is newShell's default, which keeps the
// host tests off the session bus.
type noSettings struct{}

func openNoSettings(func()) appearanceSource { return noSettings{} }

func (noSettings) portalValue(string) (any, bool) { return nil, false }
func (noSettings) gtkBool(string) (bool, bool)    { return false, false }
func (noSettings) gtkInt(string) (int32, bool)    { return 0, false }
func (noSettings) close()                         {}

// systemSettings is New's source: the settings portal, and the GtkSettings
// filling what it lacks (gtksettings.go).
type systemSettings struct {
	p  *portal      // nil: no session bus
	gs *gtkSettings // never nil
}

func (s systemSettings) portalValue(key string) (any, bool) { return s.p.portalValue(key) }
func (s systemSettings) gtkBool(name string) (bool, bool)   { return s.gs.gtkBool(name) }
func (s systemSettings) gtkInt(name string) (int32, bool)   { return s.gs.gtkInt(name) }

// close closes the portal (its goroutine is gone after it), then
// disconnects the GtkSettings handlers.
func (s systemSettings) close() {
	s.p.Close()
	s.gs.close()
}

// openSystemSettings returns the opener of systemSettings over cfg and the
// GtkSettings newProps returns (newProps nil: none). The portal reports
// its changes on godbus's goroutine, the GtkSettings on the GTK thread;
// both through the opener's changed. No session bus or no portal is not an
// error: the portal's values are then unset, and logf (when set) logs why,
// once per open; as it does a GtkSettings property it cannot watch.
func openSystemSettings(cfg portalConfig, newProps func() gtkProps, logf func(format string, args ...any)) openSource {
	return func(changed func()) appearanceSource {
		var props gtkProps
		if newProps != nil {
			props = newProps()
		}
		gs := openGtkSettings(props, changed)
		if props != nil && gs.watched < len(gtkWatched) && logf != nil {
			logf("gtk4: appearance: %d of the %d GtkSettings properties are watched", gs.watched, len(gtkWatched))
		}
		p, err := openPortal(cfg, changed)
		if err != nil && logf != nil {
			logf("gtk4: appearance: %v (the GtkSettings and the defaults fill in)", err)
		}
		return systemSettings{p, gs}
	}
}
