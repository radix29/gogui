//go:build windows

package win32

import (
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// Registry locations of the appearance settings, all under
// HKEY_CURRENT_USER.
const (
	regPersonalize   = `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`
	regDWM           = `Software\Microsoft\Windows\DWM`
	regAccessibility = `Control Panel\Accessibility`
)

// sysReader reads the OS settings behind Shell.Appearance. liveSys reads
// the real ones; the tests inject values.
type sysReader interface {
	// regDWORD reads an HKCU DWORD; ok is false when the key or value is
	// missing.
	regDWORD(path, name string) (v uint32, ok bool)
	// colorization is DwmGetColorizationColor (0xAARRGGBB).
	colorization() (argb uint32, ok bool)
	// highContrast reports SPI_GETHIGHCONTRAST's HCF_HIGHCONTRASTON.
	highContrast() bool
	// clientAreaAnimation is SPI_GETCLIENTAREAANIMATION.
	clientAreaAnimation() (on, ok bool)
}

type liveSys struct{}

func (liveSys) regDWORD(path, name string) (uint32, bool) {
	v, err := w32.RegDWORD(path, name)
	return v, err == nil
}

func (liveSys) colorization() (uint32, bool) {
	c, _, err := w32.DwmGetColorizationColor()
	return c, err == nil
}

func (liveSys) highContrast() bool {
	hc, err := w32.SystemParametersInfoHighContrast()
	return err == nil && hc.DwFlags&w32.HCF_HIGHCONTRASTON != 0
}

func (liveSys) clientAreaAnimation() (bool, bool) {
	v, err := w32.SystemParametersInfoUint32(w32.SPI_GETCLIENTAREAANIMATION)
	return v != 0, err == nil
}

// readAppearance maps the OS settings onto a driver.Appearance:
//
//   - Dark: Personalize\AppsUseLightTheme is 0 (missing: light).
//   - Accent: DWM\AccentColor (0xAABBGGRR, the accent the user picked), else
//     the DWM colorization color (0xAARRGGBB), else driver.DefaultAccent.
//     The accent is always opaque.
//   - IncreaseContrast: a high-contrast theme is on.
//   - ReduceMotion: "Animation effects" is off (SPI_GETCLIENTAREAANIMATION
//     is false; unreadable: motion on).
//   - ReduceTransparency: Personalize\EnableTransparency is 0 (missing:
//     transparency on).
//   - LegacyScrollers: "Always show scrollbars" is on, that is
//     Accessibility\DynamicScrollbars is 0 (missing, before Windows 10
//     1803's setting existed: overlay scrollers).
func readAppearance(r sysReader) driver.Appearance {
	a := driver.Appearance{Accent: driver.DefaultAccent}
	if v, ok := r.regDWORD(regPersonalize, "AppsUseLightTheme"); ok {
		a.Dark = v == 0
	}
	if v, ok := r.regDWORD(regDWM, "AccentColor"); ok {
		a.Accent = driver.RGB8(uint8(v), uint8(v>>8), uint8(v>>16))
	} else if v, ok := r.colorization(); ok {
		a.Accent = driver.RGB8(uint8(v>>16), uint8(v>>8), uint8(v))
	}
	a.IncreaseContrast = r.highContrast()
	if on, ok := r.clientAreaAnimation(); ok {
		a.ReduceMotion = !on
	}
	if v, ok := r.regDWORD(regPersonalize, "EnableTransparency"); ok {
		a.ReduceTransparency = v == 0
	}
	if v, ok := r.regDWORD(regAccessibility, "DynamicScrollbars"); ok {
		a.LegacyScrollers = v == 0
	}
	return a
}
