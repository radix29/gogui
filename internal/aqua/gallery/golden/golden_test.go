package golden

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var (
	white = color.NRGBA{255, 255, 255, 255}
	black = color.NRGBA{0, 0, 0, 255}
)

// scene is a synthetic test image: a white background with a grey
// gradient band, a blue square, a black bar with an anti-aliased left edge
// and a translucent strip.
func scene() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 48))
	fill(img, img.Rect, white)
	for x := range 64 {
		v := uint8(x * 4)
		fill(img, image.Rect(x, 0, x+1, 8), color.NRGBA{v, v, v, 255})
	}
	fill(img, image.Rect(8, 12, 24, 28), color.NRGBA{0, 122, 255, 255})
	bar(img, 40, 128)
	fill(img, image.Rect(0, 40, 64, 44), color.NRGBA{255, 0, 0, 96})
	return img
}

// bar draws the black bar at x 41..55, rows 12..35, with the edge column x
// in the grey g.
func bar(img *image.NRGBA, x int, g uint8) {
	fill(img, image.Rect(x+1, 12, 56, 36), black)
	fill(img, image.Rect(x, 12, x+1, 36), color.NRGBA{g, g, g, 255})
}

func fill(img *image.NRGBA, r image.Rectangle, c color.NRGBA) {
	draw.Draw(img, r, &image.Uniform{c}, image.Point{}, draw.Src)
}

func clone(img *image.NRGBA) *image.NRGBA {
	c := image.NewNRGBA(img.Rect)
	copy(c.Pix, img.Pix)
	return c
}

func TestCompareIdentical(t *testing.T) {
	want := scene()
	r, diff := Compare(want, clone(want), DefaultOptions)
	if !r.Pass || r.SizeMismatch || r.Diff != 0 || r.AA != 0 || r.Weighted != 0 || r.MaxDelta != 0 || !r.Bounds.Empty() {
		t.Fatalf("identical images: %+v", r)
	}
	if r.WantSize != (image.Point{64, 48}) || r.GotSize != r.WantSize {
		t.Errorf("sizes %v %v", r.WantSize, r.GotSize)
	}
	if diff == nil || diff.Rect != want.Rect {
		t.Fatalf("diff image %v", diff)
	}
	for i := 0; i < len(diff.Pix); i += 4 {
		p := diff.Pix[i : i+4]
		if p[0] != p[1] || p[1] != p[2] || p[3] != 255 || p[0] < 229 {
			t.Fatalf("diff pixel %d = %v, want a faded opaque grey", i/4, p)
		}
	}
	if !strings.HasPrefix(r.String(), "pass: 0 differing pixels, 0 anti-aliased") {
		t.Errorf("report %q", r)
	}
}

func TestCompareNoise(t *testing.T) {
	want := scene()
	got := clone(want)
	rng := rand.New(rand.NewPCG(1, 2))
	for i := range got.Pix {
		v := int(got.Pix[i]) + rng.IntN(3) - 1 // -1, 0 or +1, alpha included
		got.Pix[i] = uint8(min(max(v, 0), 255))
	}
	r, _ := Compare(want, got, DefaultOptions)
	if !r.Pass || r.Diff != 0 || r.AA != 0 {
		t.Fatalf("1-unit noise: %v", r)
	}
	if r.MaxDelta == 0 || r.MaxDelta >= DefaultOptions.Threshold {
		t.Errorf("max delta %v, want in (0, %v)", r.MaxDelta, DefaultOptions.Threshold)
	}
	// An exact comparison sees the noise.
	if r, _ := Compare(want, got, Options{}); r.Pass || r.Diff == 0 {
		t.Errorf("threshold 0: %v", r)
	}
}

func TestCompareMovedEdge(t *testing.T) {
	want := scene()
	got := clone(want)
	fill(got, image.Rect(8, 12, 24, 28), white)
	fill(got, image.Rect(11, 12, 27, 28), color.NRGBA{0, 122, 255, 255}) // 3 px right
	r, diff := Compare(want, got, DefaultOptions)
	if r.Pass || r.Diff != 2*3*16 {
		t.Fatalf("moved edge: %v, want 96 differing pixels", r)
	}
	if r.Bounds != image.Rect(8, 12, 27, 28) {
		t.Errorf("bounds %v", r.Bounds)
	}
	if r.MaxDelta <= DefaultOptions.Threshold {
		t.Errorf("max delta %v", r.MaxDelta)
	}
	if c := diff.NRGBAAt(9, 20); c != diffColor {
		t.Errorf("diff at a moved pixel = %v, want red", c)
	}
	if c := diff.NRGBAAt(15, 20); c == diffColor || c == aaColor {
		t.Errorf("diff at an unchanged pixel = %v", c)
	}
	if !strings.HasPrefix(r.String(), "FAIL: 96 differing pixels") {
		t.Errorf("report %q", r)
	}
}

func TestCompareColor(t *testing.T) {
	want := scene()
	got := clone(want)
	fill(got, image.Rect(8, 12, 24, 28), color.NRGBA{52, 199, 89, 255})
	r, _ := Compare(want, got, DefaultOptions)
	if r.Pass || r.Diff != 16*16 || r.AA != 0 {
		t.Fatalf("changed color: %v, want 256 differing pixels", r)
	}
	if r.Bounds != image.Rect(8, 12, 24, 28) {
		t.Errorf("bounds %v", r.Bounds)
	}
	// One pixel over the limit fails; MaxDiff lets it pass.
	got = clone(want)
	got.SetNRGBA(30, 30, black)
	if r, _ := Compare(want, got, DefaultOptions); r.Pass || r.Diff != 1 || r.Bounds != image.Rect(30, 30, 31, 31) {
		t.Errorf("one pixel: %v", r)
	}
	if r, _ := Compare(want, got, Options{Threshold: 0.1, MaxDiff: 1}); !r.Pass {
		t.Errorf("one pixel with MaxDiff 1: %v", r)
	}
}

// TestCompareAntialiased moves the grey of the bar's anti-aliased edge:
// those pixels count as anti-aliased, weighted by AAWeight.
func TestCompareAntialiased(t *testing.T) {
	want := scene()
	got := clone(want)
	bar(got, 40, 100)
	r, diff := Compare(want, got, DefaultOptions)
	if r.Diff != 0 || r.AA != 24 {
		t.Fatalf("edge column: %v, want 24 anti-aliased pixels", r)
	}
	if r.Bounds != image.Rect(40, 12, 41, 36) || math.Abs(r.Weighted-2.4) > 1e-9 || r.Pass {
		t.Errorf("edge column: %v", r)
	}
	if c := diff.NRGBAAt(40, 20); c != aaColor {
		t.Errorf("diff at the edge = %v, want yellow", c)
	}
	if r, _ := Compare(want, got, Options{Threshold: 0.1, AAWeight: 0.1, MaxDiff: 2.5}); !r.Pass {
		t.Errorf("MaxDiff 2.5: %v", r)
	}
	if r, _ := Compare(want, got, Options{Threshold: 0.1}); !r.Pass || r.AA != 24 {
		t.Errorf("AAWeight 0: %v", r)
	}
	// The same change in a flat area is not an edge.
	got = clone(want)
	fill(got, image.Rect(20, 30, 21, 38), color.NRGBA{100, 100, 100, 255})
	want2 := clone(want)
	fill(want2, image.Rect(20, 30, 21, 38), color.NRGBA{128, 128, 128, 255})
	if r, _ := Compare(want2, got, DefaultOptions); r.AA != 0 || r.Diff != 8 {
		t.Errorf("grey line on white: %v, want 8 plain differing pixels", r)
	}
}

func TestCompareAlpha(t *testing.T) {
	want := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	fill(want, want.Rect, white)
	got := clone(want)
	got.SetNRGBA(1, 1, color.NRGBA{255, 255, 255, 128}) // same over white, not over black
	if r, _ := Compare(want, got, DefaultOptions); r.Pass || r.Diff != 1 {
		t.Errorf("alpha only: %v", r)
	}
	// Fully transparent pixels of different colors differ only in hidden
	// channels: equal over any background.
	a := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	b := clone(a)
	fill(b, b.Rect, color.NRGBA{255, 0, 0, 0})
	if r, _ := Compare(a, b, Options{}); !r.Pass || r.MaxDelta != 0 {
		t.Errorf("transparent: %v", r)
	}
}

func TestCompareSizeMismatch(t *testing.T) {
	want := scene()
	got := image.NewNRGBA(image.Rect(0, 0, 64, 47))
	r, diff := Compare(want, got, Options{Threshold: 1, AAWeight: 1, MaxDiff: 1e9})
	if r.Pass || !r.SizeMismatch || diff != nil {
		t.Fatalf("size mismatch: %+v, diff %v", r, diff)
	}
	if r.WantSize != (image.Point{64, 48}) || r.GotSize != (image.Point{64, 47}) {
		t.Errorf("sizes %v %v", r.WantSize, r.GotSize)
	}
	if s := r.String(); s != "size mismatch: want 64x48, got 64x47" {
		t.Errorf("report %q", s)
	}
}

// TestCompareOtherImages compares a premultiplied image and a sub-image
// with another stride and origin against the straight-alpha original.
func TestCompareOtherImages(t *testing.T) {
	want := scene()
	pre := image.NewRGBA(want.Rect)
	draw.Draw(pre, pre.Rect, want, image.Point{}, draw.Src)
	if r, _ := Compare(want, pre, Options{Threshold: 0.1, AAWeight: 1}); !r.Pass || r.Diff != 0 || r.AA != 0 {
		t.Errorf("premultiplied: %v", r)
	}
	big := image.NewNRGBA(image.Rect(0, 0, 100, 60))
	draw.Draw(big, image.Rect(5, 3, 69, 51), want, image.Point{}, draw.Src)
	sub := big.SubImage(image.Rect(5, 3, 69, 51))
	if r, _ := Compare(want, sub, Options{}); !r.Pass || r.MaxDelta != 0 {
		t.Errorf("sub-image: %v", r)
	}
	big2 := image.NewNRGBA(image.Rect(0, 0, 100, 60))
	draw.Draw(big2, want.Rect, want, image.Point{}, draw.Src)
	sub2 := big2.SubImage(want.Rect) // origin (0, 0), stride 400
	if r, _ := Compare(sub2, want, Options{}); !r.Pass || r.MaxDelta != 0 {
		t.Errorf("sub-image at the origin: %v", r)
	}
	if n := NRGBA(want); n != want {
		t.Error("NRGBA copied a straight-alpha image at the origin")
	}
	if n := NRGBA(sub2); n.Stride != 4*64 || n.Rect != want.Rect {
		t.Errorf("NRGBA(sub-image) = %v, stride %d", n.Rect, n.Stride)
	}
}

func TestPNGRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "img.png")
	want := scene()
	if err := WritePNG(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPNG(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Rect != want.Rect || !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("PNG round trip changed the pixels")
	}
	// Deterministic bytes, also from a premultiplied image of the same pixels.
	first, _ := os.ReadFile(path)
	pre := image.NewRGBA(want.Rect)
	fill(want, image.Rect(0, 40, 64, 44), white) // opaque only: premultiplying loses nothing
	draw.Draw(pre, pre.Rect, want, image.Point{}, draw.Src)
	p1, p2 := filepath.Join(dir, "1.png"), filepath.Join(dir, "2.png")
	if err := WritePNG(p1, want); err != nil {
		t.Fatal(err)
	}
	if err := WritePNG(p2, pre); err != nil {
		t.Fatal(err)
	}
	b1, _ := os.ReadFile(p1)
	b2, _ := os.ReadFile(p2)
	if !bytes.Equal(b1, b2) || bytes.Equal(first, b1) {
		t.Error("PNG bytes are not determined by the pixels")
	}
	if err := WritePNG(path, scene()); err != nil {
		t.Fatal(err)
	}
	if again, _ := os.ReadFile(path); !bytes.Equal(again, first) {
		t.Error("writing the same image twice gave different bytes")
	}
}

func TestReadPNGErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadPNG(filepath.Join(dir, "missing.png")); !os.IsNotExist(err) {
		t.Errorf("missing file: %v", err)
	}
	bad := filepath.Join(dir, "bad.png")
	if err := os.WriteFile(bad, []byte("not a png"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPNG(bad); err == nil || !strings.Contains(err.Error(), "bad.png") {
		t.Errorf("bad file: %v", err)
	}
}

// TestCompareWeightedRounding: 3 anti-aliased pixels at 0.1 pass a MaxDiff
// of 0.3, although 3 × 0.1 is 0.30000000000000004 in float64.
func TestCompareWeightedRounding(t *testing.T) {
	want := scene()
	got := clone(want)
	bar(got, 40, 100)
	fill(got, image.Rect(40, 15, 41, 36), color.NRGBA{128, 128, 128, 255}) // 3 changed rows left
	r, _ := Compare(want, got, Options{Threshold: 0.1, AAWeight: 0.1, MaxDiff: 0.3})
	if r.AA != 3 || r.Diff != 0 || !r.Pass {
		t.Errorf("%v", r)
	}
}

// TestCompareMask: the moved square of TestCompareMovedEdge inside the
// mask passes and counts as masked; a mask covering half of it leaves the
// other half to fail; differing masked pixels are blue in the diff and
// unchanged ones inside the mask stay faded.
func TestCompareMask(t *testing.T) {
	want := scene()
	got := clone(want)
	fill(got, image.Rect(8, 12, 24, 28), white)
	fill(got, image.Rect(11, 12, 27, 28), color.NRGBA{0, 122, 255, 255})
	opt := DefaultOptions
	opt.Mask = []image.Rectangle{image.Rect(0, 10, 4, 12), image.Rect(6, 10, 30, 30)}
	r, diff := Compare(want, got, opt)
	if !r.Pass || r.Diff != 0 || r.AA != 0 || r.Masked != 96 || r.MaxDelta != 0 || !r.Bounds.Empty() {
		t.Fatalf("masked moved edge: %+v", r)
	}
	if c := diff.NRGBAAt(9, 20); c != maskColor {
		t.Errorf("diff at a masked moved pixel = %v, want blue", c)
	}
	if c := diff.NRGBAAt(15, 20); c == maskColor || c == diffColor {
		t.Errorf("diff at a masked unchanged pixel = %v", c)
	}
	if !strings.HasSuffix(r.String(), ", 96 masked") {
		t.Errorf("report %q", r)
	}
	opt.Mask = []image.Rectangle{image.Rect(0, 12, 64, 20)} // rows 12..19 of 12..27
	r, _ = Compare(want, got, opt)
	if r.Pass || r.Diff != 48 || r.Masked != 48 || r.Bounds != image.Rect(8, 20, 27, 28) {
		t.Errorf("half masked: %+v", r)
	}
	// Without a mask the result is TestCompareMovedEdge's.
	if r, _ := Compare(want, got, DefaultOptions); r.Masked != 0 || r.Diff != 96 || strings.Contains(r.String(), "masked") {
		t.Errorf("no mask: %v", r)
	}
}

// TestCompareThresholdClamped is B15: a NaN threshold counts as 0 (exact)
// instead of failing identical images, and one above 1 counts as 1.
func TestCompareThresholdClamped(t *testing.T) {
	want := scene()
	if r, _ := Compare(want, clone(want), Options{Threshold: math.NaN()}); !r.Pass || r.Diff != 0 {
		t.Errorf("NaN threshold, identical images: %+v", r)
	}
	got := clone(want)
	fill(got, image.Rect(8, 12, 24, 28), black) // a different square
	exact, _ := Compare(want, got, Options{Threshold: math.NaN()})
	zero, _ := Compare(want, got, Options{Threshold: 0})
	if exact.Diff != zero.Diff || exact.Diff == 0 {
		t.Errorf("NaN threshold: %d differing, threshold 0: %d", exact.Diff, zero.Diff)
	}
	if r, _ := Compare(want, got, Options{Threshold: 7}); !r.Pass {
		t.Errorf("threshold 7 (clamped to 1): %+v", r)
	}
	if r, _ := Compare(want, got, Options{Threshold: -1}); r.Diff != zero.Diff {
		t.Errorf("threshold -1: %d differing, want %d", r.Diff, zero.Diff)
	}
}
