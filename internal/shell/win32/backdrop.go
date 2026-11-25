//go:build windows

package win32

import (
	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// The popups' system backdrop and corners (S5.5d).
//
//   - The corners: DWMWA_WINDOW_CORNER_PREFERENCE ROUNDSMALL, as Windows
//     11's own menus, or DEFAULT in high contrast (Windows squares them
//     then, as for the surfaces' frames). Windows 10 does not know the
//     attribute; its failure is ignored (the menu's own rounded fill on the
//     transparent chain keeps the shape).
//   - The tint: DWMWA_USE_IMMERSIVE_DARK_MODE follows Appearance.Dark, as
//     for the surfaces' frames; without it the backdrop is light in dark
//     mode (seen on the host). Its failure (before Windows 10 20H1) is
//     ignored.
//   - The backdrop: DWMWA_SYSTEMBACKDROP_TYPE TRANSIENTWINDOW (Acrylic),
//     with the frame extended over the whole client area (margins -1) so
//     DWM draws it behind the chain's transparent pixels. It is wanted from
//     build acrylicBuild (Windows 11 22H2) on, outside high contrast
//     (SPI_GETHIGHCONTRAST) and with transparency effects on
//     (Personalize\EnableTransparency): the IncreaseContrast and
//     ReduceTransparency of the shell's appearance, re-read on
//     WM_SETTINGCHANGE. Popup.Translucent is true only when it is wanted
//     and both calls succeeded; otherwise the backdrop is set to NONE and
//     the frame margins to 0 (from acrylicBuild on; earlier builds are
//     never touched) and the menu paints its opaque fill.
//   - Applied before the window is shown and again for every appearance
//     change, before the popups are invalidated, so the next paint sees
//     the new Translucent.

// acrylicBuild is the first build with DWMWA_SYSTEMBACKDROP_TYPE.
const acrylicBuild = 22621

// backdropSys is the seam behind the backdrop: the OS build and the DWM
// calls. liveBackdrop has the real ones; the tests inject others.
type backdropSys struct {
	build  uint32
	set    func(h windows.HWND, attr uint32, v int32) error
	extend func(h windows.HWND, m *w32.Margins) error
}

func liveBackdrop() backdropSys {
	return backdropSys{
		build:  windows.RtlGetVersion().BuildNumber,
		set:    w32.DwmSetWindowAttributeInt,
		extend: w32.DwmExtendFrameIntoClientArea,
	}
}

// wantsAcrylic reports whether a popup should get the system backdrop on
// build for appearance a.
func wantsAcrylic(build uint32, a driver.Appearance) bool {
	return build >= acrylicBuild && !a.IncreaseContrast && !a.ReduceTransparency
}

// popupCorner is the corner preference of the popups for a.
func popupCorner(a driver.Appearance) int32 {
	if a.IncreaseContrast {
		return w32.DWMWCP_DEFAULT
	}
	return w32.DWMWCP_ROUNDSMALL
}

// applyBackdrop sets the popup's corners and backdrop for the shell's
// appearance and keeps whether it is translucent. The errors are returned
// for the tests: the corner's, and the first of the backdrop's.
func (p *popup) applyBackdrop() (corner, backdrop error) {
	b, a, h := p.s.backdrop, p.s.appearance, p.hwnd
	corner = b.set(h, w32.DWMWA_WINDOW_CORNER_PREFERENCE, popupCorner(a))
	var dark int32
	if a.Dark {
		dark = 1
	}
	_ = b.set(h, w32.DWMWA_USE_IMMERSIVE_DARK_MODE, dark) // the backdrop's tint
	p.translucent = false
	if b.build < acrylicBuild {
		return corner, nil // no backdrop attribute: opaque, nothing touched
	}
	if wantsAcrylic(b.build, a) {
		backdrop = b.extend(h, &w32.Margins{CxLeftWidth: -1, CxRightWidth: -1, CyTopHeight: -1, CyBottomHeight: -1})
		if backdrop == nil {
			backdrop = b.set(h, w32.DWMWA_SYSTEMBACKDROP_TYPE, w32.DWMSBT_TRANSIENTWINDOW)
		}
		if backdrop == nil {
			p.translucent = true
			return corner, nil
		}
	}
	// Opaque: no backdrop, no extended frame (failures leave it opaque all
	// the same; the first one is reported).
	e1 := b.set(h, w32.DWMWA_SYSTEMBACKDROP_TYPE, w32.DWMSBT_NONE)
	e2 := b.extend(h, &w32.Margins{})
	if backdrop == nil {
		backdrop = firstErr(e1, e2)
	}
	return corner, backdrop
}
