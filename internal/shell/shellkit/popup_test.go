package shellkit

import (
	"math"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

func TestPlacePopup(t *testing.T) {
	const (
		sx  = shell.AdjustSlideX
		sy  = shell.AdjustSlideY
		ry  = shell.AdjustResizeY
		all = sx | sy | ry
	)
	r := func(x, y, w, h float64) driver.Rect { return driver.Rect{X: x, Y: y, W: w, H: h} }
	work := r(0, 0, 1000, 800)
	nan, inf := math.NaN(), math.Inf(1)
	cases := []struct {
		name   string
		anchor driver.Rect
		size   driver.Size
		offset driver.Point
		adjust shell.PopupAdjust
		work   driver.Rect
		want   driver.Rect
	}{
		{"fits", r(100, 100, 80, 20), driver.Size{W: 120, H: 200}, driver.Point{X: -10, Y: -40}, all, work, r(90, 60, 120, 200)},
		{"below the anchor", r(100, 100, 80, 20), driver.Size{W: 120, H: 200}, driver.Point{Y: 20}, all, work, r(100, 120, 120, 200)},
		{"zero anchor", r(0, 0, 0, 0), driver.Size{W: 50, H: 30}, driver.Point{}, all, work, r(0, 0, 50, 30)},
		{"zero anchor offset", r(0, 0, 0, 0), driver.Size{W: 50, H: 30}, driver.Point{X: 5, Y: 6}, 0, work, r(5, 6, 50, 30)},

		// Every edge, sliding.
		{"right edge slides", r(950, 100, 40, 20), driver.Size{W: 120, H: 50}, driver.Point{}, sx, work, r(880, 100, 120, 50)},
		{"left edge slides", r(-30, 100, 40, 20), driver.Size{W: 120, H: 50}, driver.Point{}, sx, work, r(0, 100, 120, 50)},
		{"top edge slides", r(100, 10, 40, 20), driver.Size{W: 120, H: 200}, driver.Point{Y: -60}, sy, work, r(100, 0, 120, 200)},
		{"bottom edge slides", r(100, 700, 40, 20), driver.Size{W: 120, H: 200}, driver.Point{}, sy, work, r(100, 600, 120, 200)},

		// Every edge, not adjusted: off-screen as requested.
		{"right edge stays", r(950, 100, 40, 20), driver.Size{W: 120, H: 50}, driver.Point{}, sy | ry, work, r(950, 100, 120, 50)},
		{"left edge stays", r(-30, 100, 40, 20), driver.Size{W: 120, H: 50}, driver.Point{}, 0, work, r(-30, 100, 120, 50)},
		{"bottom edge stays", r(100, 700, 40, 20), driver.Size{W: 120, H: 200}, driver.Point{}, sx, work, r(100, 700, 120, 200)},

		// Resizing cuts the parts outside, top or bottom.
		{"top edge resizes", r(100, 50, 80, 20), driver.Size{W: 120, H: 300}, driver.Point{Y: -150}, ry, work, r(100, 0, 120, 200)},
		{"bottom edge resizes", r(100, 700, 80, 20), driver.Size{W: 120, H: 300}, driver.Point{}, ry, work, r(100, 700, 120, 100)},
		{"both edges resize", r(100, 50, 80, 20), driver.Size{W: 120, H: 1200}, driver.Point{Y: -200}, ry, work, r(100, 0, 120, 800)},
		{"slide wins over resize", r(100, 700, 80, 20), driver.Size{W: 120, H: 300}, driver.Point{}, sy | ry, work, r(100, 500, 120, 300)},
		{"wholly below: no cut", r(100, 900, 80, 20), driver.Size{W: 120, H: 50}, driver.Point{}, ry, work, r(100, 900, 120, 50)},
		{"wholly above: no cut", r(100, -100, 80, 20), driver.Size{W: 120, H: 50}, driver.Point{}, ry, work, r(100, -100, 120, 50)},

		// Larger than the work area.
		{"taller: slide to top", r(100, 400, 80, 20), driver.Size{W: 120, H: 1000}, driver.Point{}, sy, work, r(100, 0, 120, 1000)},
		{"taller: slide and resize", r(100, 400, 80, 20), driver.Size{W: 120, H: 1000}, driver.Point{}, sy | ry, work, r(100, 0, 120, 800)},
		{"wider: slide to left", r(100, 100, 80, 20), driver.Size{W: 1500, H: 50}, driver.Point{}, all, work, r(0, 100, 1500, 50)},

		// A work area not at the origin (a second monitor, a taskbar on top).
		{"offset work area", r(1900, 1050, 40, 20), driver.Size{W: 200, H: 100}, driver.Point{}, all, r(1000, 40, 1000, 1040), r(1800, 980, 200, 100)},
		{"negative work area", r(-500, -20, 40, 20), driver.Size{W: 200, H: 100}, driver.Point{}, sx | sy, r(-1280, 0, 1280, 1024), r(-500, 0, 200, 100)},

		// Degenerate input.
		{"empty work area", r(950, 790, 40, 20), driver.Size{W: 120, H: 50}, driver.Point{}, all, r(0, 0, 0, 800), r(950, 790, 120, 50)},
		{"bad sizes", r(10, 10, 40, 20), driver.Size{W: -5, H: nan}, driver.Point{}, all, work, r(10, 10, 0, 0)},
		{"infinite size", r(10, 10, 40, 20), driver.Size{W: inf, H: 10}, driver.Point{}, all, work, r(10, 10, 0, 10)},
		{"bad positions", r(nan, 10, 40, 20), driver.Size{W: 10, H: 10}, driver.Point{X: 3, Y: inf}, 0, work, r(3, 10, 10, 10)},
		{"zero size at the corner", r(1000, 800, 0, 0), driver.Size{}, driver.Point{}, all, work, r(1000, 800, 0, 0)},
	}
	for _, c := range cases {
		if got := PlacePopup(c.anchor, c.size, c.offset, c.adjust, c.work); got != c.want {
			t.Errorf("%s: PlacePopup = %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestIsTrackAndInRect(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)
	for _, c := range []struct {
		r    driver.Rect
		want bool
	}{
		{driver.Rect{X: 10, Y: 0, W: 200, H: 28}, true},
		{driver.Rect{X: -5, Y: -3, W: 1, H: 1}, true},
		{driver.Rect{}, false},
		{driver.Rect{W: 10}, false},
		{driver.Rect{W: -10, H: 5}, false},
		{driver.Rect{W: nan, H: 5}, false},
		{driver.Rect{W: inf, H: 5}, false},
		{driver.Rect{X: nan, W: 5, H: 5}, false},
		{driver.Rect{Y: -inf, W: 5, H: 5}, false},
	} {
		if got := IsTrack(c.r); got != c.want {
			t.Errorf("IsTrack(%+v) = %v, want %v", c.r, got, c.want)
		}
	}
	r := driver.Rect{X: 10, Y: 20, W: 30, H: 40}
	for _, c := range []struct {
		q    driver.Point
		want bool
	}{
		{driver.Point{X: 10, Y: 20}, true},
		{driver.Point{X: 39.9, Y: 59.9}, true},
		{driver.Point{X: 40, Y: 30}, false},
		{driver.Point{X: 20, Y: 60}, false},
		{driver.Point{X: 9.9, Y: 30}, false},
		{driver.Point{X: 20, Y: 19.9}, false},
		{driver.Point{X: nan, Y: 30}, false},
	} {
		if got := InRect(r, c.q); got != c.want {
			t.Errorf("InRect(%v) = %v, want %v", c.q, got, c.want)
		}
	}
}
