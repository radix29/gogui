package shellkit

import (
	"math"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// PlacePopup places a popup as the Wayland xdg_positioner does, without
// flips (shell.PopupConfig): the requested rect is size at anchor's
// top-left plus offset; then, if it does not fit work, AdjustSlideX and
// AdjustSlideY move it inside (a popup larger than work aligned with work's
// left or top edge) and AdjustResizeY cuts its height to work (unless it
// lies wholly above or below work). Without an adjustment for an axis the
// popup stays where the offset put it. Negative, NaN and infinite sizes
// count as zero, NaN and infinite positions too. An empty work area (zero
// or negative width or height) means "unknown": nothing is adjusted.
//
// Every value is in one coordinate space, logical points or pixels; the
// shells turn the anchor into screen coordinates first and the result
// back into the parent's points.
func PlacePopup(anchor driver.Rect, size driver.Size, offset driver.Point, adjust shell.PopupAdjust, work driver.Rect) driver.Rect {
	r := driver.Rect{
		X: finite(anchor.X) + finite(offset.X),
		Y: finite(anchor.Y) + finite(offset.Y),
		W: length(size.W),
		H: length(size.H),
	}
	if !(work.W > 0 && work.H > 0) {
		return r
	}
	if adjust&shell.AdjustSlideX != 0 {
		r.X = slide(r.X, r.W, work.X, work.W)
	}
	if adjust&shell.AdjustSlideY != 0 {
		r.Y = slide(r.Y, r.H, work.Y, work.H)
	}
	if adjust&shell.AdjustResizeY != 0 {
		top := math.Max(r.Y, work.Y)
		bottom := math.Min(r.Y+r.H, work.Y+work.H)
		if bottom > top {
			r.Y, r.H = top, bottom-top
		}
	}
	return r
}

// slide moves a span [x, x+w) into [lo, lo+n); a span longer than n starts
// at lo.
func slide(x, w, lo, n float64) float64 {
	if x+w > lo+n {
		x = lo + n - w
	}
	return math.Max(x, lo)
}

func finite(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

func length(v float64) float64 {
	if !(v > 0) || math.IsInf(v, 1) {
		return 0
	}
	return v
}

// IsTrack reports whether r is a grab's track area
// (shell.PopupConfig.Track): a positive, finite size at a finite position.
// Any other rect is no track.
func IsTrack(r driver.Rect) bool {
	return length(r.W) > 0 && length(r.H) > 0 && finite(r.X) == r.X && finite(r.Y) == r.Y
}

// InRect reports whether q lies in r, its left and top edges included, its
// right and bottom edges not.
func InRect(r driver.Rect, q driver.Point) bool {
	return q.X >= r.X && q.Y >= r.Y && q.X < r.X+r.W && q.Y < r.Y+r.H
}
