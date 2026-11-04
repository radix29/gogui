// Package golden reads and writes the golden PNGs of the gallery (PLAN.md
// S3.6) and compares two images with a perceptual tolerance.
//
// Compare follows pixelmatch (github.com/mapbox/pixelmatch, ISC): the
// difference of two pixels is a weighted distance in the YIQ color space,
// a pixel differs when it is over Options.Threshold, and differing pixels
// on an anti-aliased edge (found from their neighbours in both images) are
// counted apart and weigh only Options.AAWeight. Unlike pixelmatch, a
// translucent pixel is blended over both white and black and the larger
// distance is taken, so a change of alpha alone is not hidden.
//
// Images are straight-alpha (image.NRGBA); other images are converted
// first. The package is pure Go, imports only the standard library and has
// no build tags.
package golden

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// ReadPNG reads a PNG file as a straight-alpha image.
func ReadPNG(path string) (*image.NRGBA, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("golden: %s: %w", path, err)
	}
	return NRGBA(img), nil
}

// WritePNG writes img as a straight-alpha PNG (8 bits per channel; RGB when
// every pixel is opaque, RGBA otherwise), creating the directory. The
// encoding is deterministic: the same pixels give the same bytes.
func WritePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, NRGBA(img)); err != nil {
		return fmt.Errorf("golden: %s: %w", path, err)
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// NRGBA returns img as a compact straight-alpha image with its origin at
// (0, 0) and a stride of four bytes per pixel: img itself when it already
// is one, a converted copy otherwise (premultiplied images are
// un-premultiplied by the color model).
func NRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) && n.Stride == 4*n.Rect.Dx() && len(n.Pix) == n.Stride*n.Rect.Dy() {
		return n
	}
	b := img.Bounds()
	n := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(n, n.Rect, img, b.Min, draw.Src)
	return n
}

// Options set the tolerance of Compare.
type Options struct {
	// Threshold is the largest YIQ distance of two pixels that still
	// counts as equal, from 0 (exact) to 1 (anything); pixelmatch's
	// default is 0.1.
	Threshold float64

	// AAWeight is what a differing pixel on an anti-aliased edge counts
	// for, from 0 (ignored, as pixelmatch does by default) to 1 (as much
	// as any other pixel).
	AAWeight float64

	// MaxDiff is the largest weighted count of differing pixels that still
	// passes: plain differing pixels count 1, anti-aliased ones AAWeight.
	MaxDiff float64
}

// DefaultOptions is the tolerance of the gallery goldens: pixelmatch's
// threshold and no differing pixel at all. Anti-aliased pixels weigh a
// tenth, so raising MaxDiff to n tolerates 10n of them or n plain ones.
var DefaultOptions = Options{Threshold: 0.1, AAWeight: 0.1, MaxDiff: 0}

// Result is the outcome of Compare.
type Result struct {
	// WantSize and GotSize are the sizes of the two images; SizeMismatch
	// is set when they differ, and the pixels are then not compared.
	WantSize, GotSize image.Point
	SizeMismatch      bool

	// Diff counts the pixels over the threshold that are not on an
	// anti-aliased edge, AA those that are.
	Diff, AA int

	// Weighted is Diff + AA × AAWeight, compared with MaxDiff.
	Weighted float64

	// MaxDelta is the largest distance of any pixel pair, on the
	// Threshold scale (0 for identical images).
	MaxDelta float64

	// Bounds is the bounding box of the pixels over the threshold, plain
	// and anti-aliased (empty when there are none).
	Bounds image.Rectangle

	// Pass reports whether the images match within the options.
	Pass bool
}

// String is a one-line report.
func (r Result) String() string {
	if r.SizeMismatch {
		return fmt.Sprintf("size mismatch: want %dx%d, got %dx%d", r.WantSize.X, r.WantSize.Y, r.GotSize.X, r.GotSize.Y)
	}
	verdict := "pass"
	if !r.Pass {
		verdict = "FAIL"
	}
	return fmt.Sprintf("%s: %d differing pixels, %d anti-aliased (weighted %.4g), max delta %.4f, bounds %v",
		verdict, r.Diff, r.AA, r.Weighted, r.MaxDelta, r.Bounds)
}

// maxYIQ is the largest YIQ distance, as in pixelmatch: thresholds are
// relative to it, squared.
const maxYIQ = 35215

// The colors of the diff image (pixelmatch's defaults).
var (
	diffColor = color.NRGBA{R: 255, A: 255}         // a differing pixel
	aaColor   = color.NRGBA{R: 255, G: 255, A: 255} // a differing anti-aliased pixel
)

// Compare compares got with want. It returns the result and a diff image of
// want's size: unchanged pixels as a faded grey of want, differing pixels
// red, differing anti-aliased ones yellow. On a size mismatch the result
// fails and the diff image is nil.
func Compare(want, got image.Image, opt Options) (Result, *image.NRGBA) {
	a, b := NRGBA(want), NRGBA(got)
	r := Result{WantSize: a.Rect.Size(), GotSize: b.Rect.Size()}
	if r.WantSize != r.GotSize {
		r.SizeMismatch = true
		return r, nil
	}
	w, h := r.WantSize.X, r.WantSize.Y
	limit := maxYIQ * opt.Threshold * opt.Threshold
	diff := image.NewNRGBA(a.Rect)
	var maxD float64
	for y := range h {
		for x := range w {
			pa := pix(a, x, y)
			d := pixelDelta(pa, pix(b, x, y))
			maxD = max(maxD, d)
			if d <= limit {
				diff.SetNRGBA(x, y, faded(pa))
				continue
			}
			if antialiased(a, x, y, b) || antialiased(b, x, y, a) {
				r.AA++
				diff.SetNRGBA(x, y, aaColor)
			} else {
				r.Diff++
				diff.SetNRGBA(x, y, diffColor)
			}
			r.Bounds = r.Bounds.Union(image.Rect(x, y, x+1, y+1))
		}
	}
	r.MaxDelta = math.Sqrt(maxD / maxYIQ)
	r.Weighted = float64(r.Diff) + float64(r.AA)*opt.AAWeight
	r.Pass = r.Weighted <= opt.MaxDiff+1e-9 // 3 × 0.1 is not exactly 0.3
	return r, diff
}

// rgb is a pixel blended over an opaque background, in 0..255.
type rgb struct{ r, g, b float64 }

// over blends the straight-alpha pixel p over the grey background bg.
func over(p []uint8, bg float64) rgb {
	a := float64(p[3]) / 255
	f := func(c uint8) float64 { return bg + (float64(c)-bg)*a }
	return rgb{f(p[0]), f(p[1]), f(p[2])}
}

func (c rgb) y() float64 { return c.r*0.29889531 + c.g*0.58662247 + c.b*0.11448223 }
func (c rgb) i() float64 { return c.r*0.59597799 - c.g*0.27417610 - c.b*0.32180189 }
func (c rgb) q() float64 { return c.r*0.21147017 - c.g*0.52261711 + c.b*0.31114694 }

// yiq is the squared YIQ distance of two opaque colors (pixelmatch's
// colorDelta).
func yiq(p, q rgb) float64 {
	dy, di, dq := p.y()-q.y(), p.i()-q.i(), p.q()-q.q()
	return 0.5053*dy*dy + 0.299*di*di + 0.1957*dq*dq
}

// pixelDelta is the squared YIQ distance of two straight-alpha pixels: over
// white and over black, the larger. Equal pixels give exactly 0.
func pixelDelta(p, q []uint8) float64 {
	if same(p, q) {
		return 0
	}
	d := yiq(over(p, 255), over(q, 255))
	if p[3] < 255 || q[3] < 255 {
		d = max(d, yiq(over(p, 0), over(q, 0)))
	}
	return d
}

// brightness is the signed luma difference of two pixels over white: > 0
// when p is brighter. Equal pixels give exactly 0.
func brightness(p, q []uint8) float64 {
	if same(p, q) {
		return 0
	}
	return over(p, 255).y() - over(q, 255).y()
}

// faded is the diff image's look of an unchanged pixel: its luma, faded
// toward white.
func faded(p []uint8) color.NRGBA {
	v := uint8(math.Round(255 + (over(p, 255).y()-255)*0.1))
	return color.NRGBA{R: v, G: v, B: v, A: 255}
}

func pix(img *image.NRGBA, x, y int) []uint8 {
	i := img.PixOffset(x, y)
	return img.Pix[i : i+4 : i+4]
}

func same(p, q []uint8) bool {
	return p[0] == q[0] && p[1] == q[1] && p[2] == q[2] && p[3] == q[3]
}

// window is the 3 × 3 window around (x, y), clipped to img, and 1 when
// (x, y) is on the image border, which pixelmatch counts as one equal
// neighbour.
func window(img *image.NRGBA, x, y int) (x0, y0, x1, y1, border int) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	x0, y0, x1, y1 = max(x-1, 0), max(y-1, 0), min(x+1, w-1), min(y+1, h-1)
	if x == x0 || x == x1 || y == y0 || y == y1 {
		border = 1
	}
	return x0, y0, x1, y1, border
}

// antialiased reports whether (x, y) in img lies on an anti-aliased edge
// (pixelmatch's antialiased, after Vysniauskas 2009): at most two of its
// neighbours equal it, it has both a darker and a brighter neighbour, and
// the darkest or the brightest of them sits in a flat area in both img and
// other.
func antialiased(img *image.NRGBA, x, y int, other *image.NRGBA) bool {
	p := pix(img, x, y)
	x0, y0, x1, y1, zeroes := window(img, x, y)
	var lo, hi float64
	var loX, loY, hiX, hiY int
	for nx := x0; nx <= x1; nx++ { // x outer, y inner: pixelmatch's order for ties
		for ny := y0; ny <= y1; ny++ {
			if nx == x && ny == y {
				continue
			}
			d := brightness(p, pix(img, nx, ny))
			switch {
			case d == 0:
				zeroes++
				if zeroes > 2 {
					return false
				}
			case d < lo:
				lo, loX, loY = d, nx, ny
			case d > hi:
				hi, hiX, hiY = d, nx, ny
			}
		}
	}
	if lo == 0 || hi == 0 {
		return false
	}
	return manySiblings(img, loX, loY) && manySiblings(other, loX, loY) ||
		manySiblings(img, hiX, hiY) && manySiblings(other, hiX, hiY)
}

// manySiblings reports whether more than two neighbours of (x, y) equal it
// exactly (the image border counts as one).
func manySiblings(img *image.NRGBA, x, y int) bool {
	p := pix(img, x, y)
	x0, y0, x1, y1, zeroes := window(img, x, y)
	for nx := x0; nx <= x1; nx++ {
		for ny := y0; ny <= y1; ny++ {
			if (nx != x || ny != y) && same(p, pix(img, nx, ny)) {
				zeroes++
				if zeroes > 2 {
					return true
				}
			}
		}
	}
	return false
}
