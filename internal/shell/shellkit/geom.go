package shellkit

import (
	"math"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// MaxPx bounds a converted pixel length (the OSes reject larger windows
// long before this).
const MaxPx = 1 << 20

// ToPx converts a logical length to whole pixels at scale (round half away
// from zero), clamped to [0, MaxPx]; NaN gives 0.
func ToPx(v, scale float64) int32 {
	p := math.Round(v * scale)
	if !(p > 0) {
		return 0
	}
	if p > MaxPx {
		return MaxPx
	}
	return int32(p)
}

// FromPx converts a pixel size to logical points at scale. If hint
// converts to exactly these pixels, it is returned unchanged, so a size
// round-trips through the window even when it is not a whole number of
// pixels.
func FromPx(w, h int32, scale float64, hint driver.Size) driver.Size {
	sz := driver.Size{W: float64(w) / scale, H: float64(h) / scale}
	if ToPx(hint.W, scale) == w {
		sz.W = hint.W
	}
	if ToPx(hint.H, scale) == h {
		sz.H = hint.H
	}
	return sz
}

// ClampSize limits size to the limits (zero components: no limit; min wins
// over max) and to finite non-negative values.
func ClampSize(size, min, max driver.Size) driver.Size {
	lim := func(v, lo, hi float64) float64 {
		if math.IsNaN(v) || v < 0 {
			v = 0
		}
		if hi > 0 {
			v = math.Min(v, hi)
		}
		return math.Max(v, lo)
	}
	return driver.Size{W: lim(size.W, min.W, max.W), H: lim(size.H, min.H, max.H)}
}

// Rect is a rectangle in device pixels, right and bottom exclusive. It
// converts directly from a struct of four int32 fields Left, Top, Right and
// Bottom (a Win32 RECT).
type Rect struct{ Left, Top, Right, Bottom int32 }

// CenterIn returns the top-left corner of a w × h rectangle centered in
// work; a rectangle larger than work starts at its top-left corner.
func CenterIn(work Rect, w, h int32) (x, y int32) {
	x = work.Left + max(0, (work.Right-work.Left-w)/2)
	y = work.Top + max(0, (work.Bottom-work.Top-h)/2)
	return x, y
}

// HitRegion returns the kind of the chrome region containing pos, the
// regions tested from last to first (a later region lies on top of the
// earlier ones). ok is false when no region contains pos. Regions and pos
// are in points of the drawable area.
func HitRegion(regions []shell.ChromeRegion, pos driver.Point) (kind shell.RegionKind, ok bool) {
	for i := len(regions) - 1; i >= 0; i-- {
		if regions[i].Rect.Contains(pos) {
			return regions[i].Kind, true
		}
	}
	return 0, false
}
