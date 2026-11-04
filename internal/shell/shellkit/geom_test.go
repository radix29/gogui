package shellkit

import (
	"math"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

func TestToPxFromPx(t *testing.T) {
	px := []struct {
		v, scale float64
		want     int32
	}{
		{0, 1, 0}, {100, 1, 100}, {100, 1.25, 125}, {100, 1.5, 150}, {100.4, 1.5, 151},
		{100.3, 1.5, 150}, {0.5, 1, 1}, {0.4, 1, 0}, {-5, 1, 0}, {math.NaN(), 1, 0},
		{math.Inf(1), 2, MaxPx}, {1e12, 1, MaxPx}, {333.33, 1.75, 583},
	}
	for _, c := range px {
		if got := ToPx(c.v, c.scale); got != c.want {
			t.Errorf("ToPx(%v, %v) = %d, want %d", c.v, c.scale, got, c.want)
		}
	}
	back := []struct {
		w, h  int32
		scale float64
		hint  driver.Size
		want  driver.Size
	}{
		{150, 75, 1.5, driver.Size{}, driver.Size{W: 100, H: 50}},
		{151, 75, 1.5, driver.Size{W: 100.4, H: 50.2}, driver.Size{W: 100.4, H: 50.2}},
		{151, 76, 1.5, driver.Size{W: 100.4, H: 50.2}, driver.Size{W: 100.4, H: 76 / 1.5}},
		{125, 125, 1.25, driver.Size{W: 99, H: 100}, driver.Size{W: 100, H: 100}},
		{0, 0, 2, driver.Size{}, driver.Size{}},
	}
	for _, c := range back {
		if got := FromPx(c.w, c.h, c.scale, c.hint); got != c.want {
			t.Errorf("FromPx(%d, %d, %v, %v) = %v, want %v", c.w, c.h, c.scale, c.hint, got, c.want)
		}
	}
	// Every size on a 0.1 pt grid round-trips at the common scales.
	for _, scale := range []float64{1, 1.25, 1.5, 1.75, 2, 2.25, 3} {
		for v := 0.0; v < 50; v += 0.1 {
			p := ToPx(v, scale)
			if got := FromPx(p, p, scale, driver.Size{W: v, H: v}); got.W != v || got.H != v {
				t.Fatalf("round trip of %v at %v: %v", v, scale, got)
			}
		}
	}
}

func TestClampSize(t *testing.T) {
	cases := []struct {
		size, min, max, want driver.Size
	}{
		{driver.Size{W: 10, H: 10}, driver.Size{}, driver.Size{}, driver.Size{W: 10, H: 10}},
		{driver.Size{W: 10, H: 10}, driver.Size{W: 20}, driver.Size{}, driver.Size{W: 20, H: 10}},
		{driver.Size{W: 100, H: 100}, driver.Size{}, driver.Size{W: 50, H: 60}, driver.Size{W: 50, H: 60}},
		{driver.Size{W: 10, H: 10}, driver.Size{W: 80}, driver.Size{W: 50}, driver.Size{W: 80, H: 10}}, // min wins
		{driver.Size{W: -1, H: math.NaN()}, driver.Size{}, driver.Size{}, driver.Size{}},
	}
	for _, c := range cases {
		if got := ClampSize(c.size, c.min, c.max); got != c.want {
			t.Errorf("ClampSize(%v, %v, %v) = %v, want %v", c.size, c.min, c.max, got, c.want)
		}
	}
}

func TestCenterIn(t *testing.T) {
	work := Rect{Left: 100, Top: 50, Right: 1100, Bottom: 850}
	if x, y := CenterIn(work, 400, 200); x != 400 || y != 350 {
		t.Errorf("CenterIn = %d, %d; want 400, 350", x, y)
	}
	if x, y := CenterIn(work, 2000, 2000); x != 100 || y != 50 {
		t.Errorf("CenterIn of a too large window = %d, %d; want the work area's corner", x, y)
	}
}

// The regions of a 400 pt wide title bar as the aqua driver sets them:
// the bar, then the three lights (aqua/chrome.go).
var barRegions = []shell.ChromeRegion{
	{Rect: driver.Rect{W: 400, H: 28}, Kind: shell.RegionCaption},
	{Rect: driver.Rect{X: 8, Y: 8, W: 12, H: 12}, Kind: shell.RegionClose},
	{Rect: driver.Rect{X: 28, Y: 8, W: 12, H: 12}, Kind: shell.RegionMinimize},
	{Rect: driver.Rect{X: 48, Y: 8, W: 12, H: 12}, Kind: shell.RegionMaximize},
}

func TestHitRegion(t *testing.T) {
	over := append(append([]shell.ChromeRegion(nil), barRegions...),
		shell.ChromeRegion{Rect: driver.Rect{X: 100, W: 50, H: 28}, Kind: shell.RegionClient})
	cases := []struct {
		name    string
		regions []shell.ChromeRegion
		pos     driver.Point
		want    shell.RegionKind
		ok      bool
	}{
		{"close light", barRegions, driver.Point{X: 9, Y: 9}, shell.RegionClose, true},
		{"minimize light", barRegions, driver.Point{X: 30, Y: 15}, shell.RegionMinimize, true},
		{"maximize light, last point", barRegions, driver.Point{X: 59.5, Y: 19.5}, shell.RegionMaximize, true},
		{"right edge is outside", barRegions, driver.Point{X: 60, Y: 12}, shell.RegionCaption, true},
		{"between two lights", barRegions, driver.Point{X: 22, Y: 12}, shell.RegionCaption, true},
		{"title bar's right end", barRegions, driver.Point{X: 399.5, Y: 20}, shell.RegionCaption, true},
		{"below the title bar", barRegions, driver.Point{X: 200, Y: 28}, 0, false},
		{"no regions", nil, driver.Point{X: 9, Y: 9}, 0, false},
		{"later region wins", over, driver.Point{X: 120, Y: 10}, shell.RegionClient, true},
		{"earlier region under it", over, driver.Point{X: 160, Y: 10}, shell.RegionCaption, true},
		{"NaN is nowhere", barRegions, driver.Point{X: math.NaN(), Y: 9}, 0, false},
	}
	for _, c := range cases {
		if k, ok := HitRegion(c.regions, c.pos); k != c.want || ok != c.ok {
			t.Errorf("%s: HitRegion(%v) = %v %v, want %v %v", c.name, c.pos, k, ok, c.want, c.ok)
		}
	}
}
