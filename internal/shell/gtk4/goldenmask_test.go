package gtk4

import (
	"image"
	"math"
	"slices"
	"testing"

	"github.com/radix29/gogui/internal/render"
)

// textMask turns the text boxes of a paint (surface points, as
// surface.textBoxes keeps them) into the pixel rectangles of a frame of
// w×h pixels at scale that the Linux golden test leaves out of the
// comparison (S4.3i): each box grown by margin points on every side,
// rounded outward to whole pixels and clipped to the frame. Boxes that
// end up empty are dropped. Untagged, so it is tested on every host; only
// gallery_linux_test.go uses it.
func textMask(boxes []render.Rect, scale, margin float64, w, h int) []image.Rectangle {
	frame := image.Rect(0, 0, w, h)
	var m []image.Rectangle
	for _, b := range boxes {
		r := image.Rect(
			int(math.Floor((b.X-margin)*scale)), int(math.Floor((b.Y-margin)*scale)),
			int(math.Ceil((b.X+b.W+margin)*scale)), int(math.Ceil((b.Y+b.H+margin)*scale)),
		).Intersect(frame)
		if !r.Empty() {
			m = append(m, r)
		}
	}
	return m
}

func TestTextMask(t *testing.T) {
	boxes := []render.Rect{
		{X: 10.25, Y: 5, W: 20.5, H: 16}, // fractional edges round outward
		{X: -4, Y: 90, W: 10, H: 20},     // clipped to the frame
		{X: 200, Y: 0, W: 10, H: 10},     // outside: dropped
		{X: 50, Y: 50, W: 0, H: 0},       // empty, but the margin gives it pixels
	}
	if got, want := textMask(boxes, 1, 0, 100, 100), []image.Rectangle{
		image.Rect(10, 5, 31, 21), image.Rect(0, 90, 6, 100),
	}; !slices.Equal(got, want) {
		t.Errorf("1×, no margin: %v, want %v", got, want)
	}
	if got, want := textMask(boxes, 2, 1, 200, 200), []image.Rectangle{
		image.Rect(18, 8, 64, 44), image.Rect(0, 178, 14, 200), image.Rect(98, 98, 102, 102),
	}; !slices.Equal(got, want) {
		t.Errorf("2×, 1 pt margin: %v, want %v", got, want)
	}
	if got := textMask(nil, 1, 1, 10, 10); got != nil {
		t.Errorf("no boxes: %v", got)
	}
}
