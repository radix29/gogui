// Package goldentest holds the OS-neutral parts of the gallery golden tests
// (PLAN.md S3.6, S4.3e, S6.1h) that every shell shares: the variant table
// (light and dark at 1× and 2×; key, inactive and with Increase Contrast;
// with their file names), the conversion of a premultiplied BGRA readback
// to straight-alpha RGBA, and the compare-and-report flow against the
// committed PNGs.
//
// It is imported only by tests (the win32 gallery test, and the gtk4 one
// from S4.3g). It has no build tags and imports only the standard library
// and aqua/gallery/golden.
package goldentest

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/radix29/gogui/internal/aqua/gallery/golden"
)

// Variant is one golden render: the appearance, the backing scale and,
// since S6.1h, the window's state.
type Variant struct {
	Dark  bool
	Scale float64 // 1 or 2

	// Inactive renders the gallery in a window that is not key (S6.1h).
	Inactive bool
	// Contrast renders it with Increase Contrast (S6.1h).
	Contrast bool
}

// Variants are the four golden renders of a key window without Increase
// Contrast, in the order the shells render them. Linux renders these.
var Variants = []Variant{{false, 1, false, false}, {true, 1, false, false}, {false, 2, false, false}, {true, 2, false, false}}

// PolishVariants are the inactive and the increased-contrast galleries
// (S6.1h), light and dark at 1× and 2×, in Variants' order within each.
// Windows renders them; the Linux ones join S4.3j's follow-up.
var PolishVariants = polish()

func polish() []Variant {
	var out []Variant
	for _, state := range []Variant{{Inactive: true}, {Contrast: true}} {
		for _, v := range Variants {
			v.Inactive, v.Contrast = state.Inactive, state.Contrast
			out = append(out, v)
		}
	}
	return out
}

// Name is the variant's golden file name without ".png", such as
// "gallery-dark-x2" or "gallery-light-inactive-x1".
func (v Variant) Name() string {
	mode := "light"
	if v.Dark {
		mode = "dark"
	}
	if v.Inactive {
		mode += "-inactive"
	}
	if v.Contrast {
		mode += "-contrast"
	}
	return fmt.Sprintf("gallery-%s-x%g", mode, v.Scale)
}

// DirFromShell is the directory of the committed goldens relative to a
// shell package (internal/shell/<name>), the working directory of its
// tests.
var DirFromShell = filepath.Join("..", "..", "aqua", "gallery", "testdata", "golden")

// FromBGRA converts premultiplied BGRA pixels (as both shells read them
// back: D3D11 staging, gdk_texture_download) to straight-alpha RGBA,
// rounding to the nearest value. Rows are stride bytes apart (at least
// 4·w). Fully transparent pixels become transparent black; colour over
// the alpha clamps to 255.
func FromBGRA(w, h, stride int, bgra []byte) *image.NRGBA {
	if stride < 4*w || (h > 0 && len(bgra) < (h-1)*stride+4*w) {
		panic(fmt.Sprintf("goldentest: %d bytes, stride %d for %d×%d px", len(bgra), stride, w, h))
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		src, dst := bgra[y*stride:y*stride+4*w], img.Pix[y*img.Stride:y*img.Stride+4*w]
		for i := 0; i < 4*w; i += 4 {
			b, g, r, a := uint32(src[i]), uint32(src[i+1]), uint32(src[i+2]), uint32(src[i+3])
			switch a {
			case 0:
				r, g, b = 0, 0, 0
			case 255:
			default:
				r, g, b = min((r*255+a/2)/a, 255), min((g*255+a/2)/a, 255), min((b*255+a/2)/a, 255)
			}
			dst[i], dst[i+1], dst[i+2], dst[i+3] = uint8(r), uint8(g), uint8(b), uint8(a)
		}
	}
	return img
}

// Check is the compare-and-report flow of one golden test.
type Check struct {
	// Dir holds the goldens (DirFromShell from a shell package).
	Dir string

	// Options is the tolerance (golden.DefaultOptions on Windows; Linux
	// gets its own, calibrated in S4.3j; with the text boxes in Mask).
	Options golden.Options

	// Update rewrites the goldens instead of comparing. Only the caller
	// decides whether it may (Windows, from its -update flag); Linux never
	// sets it.
	Update bool

	// Out is where a failing comparison writes <name>.actual.png and
	// <name>.diff.png. Empty means $GOGUI_GOLDEN_OUT, else t.TempDir.
	Out string

	// LogOnly reports a failing comparison with t.Logf instead of
	// t.Errorf (the images are still written), for a tolerance that is not
	// calibrated yet (S4.3g). A missing golden and an unwritable file
	// still fail.
	LogOnly bool
}

// Run compares got with the golden of v, or rewrites the golden when
// c.Update is set. It logs every Result; on a failure it writes the
// actual image and, when the sizes match, the diff, and names them in the
// message. It returns the Result (the zero Result after an update or when
// the golden cannot be read).
func (c *Check) Run(t testing.TB, v Variant, got *image.NRGBA) golden.Result {
	t.Helper()
	name := v.Name()
	path := filepath.Join(c.Dir, name+".png")
	if c.Update {
		if err := golden.WritePNG(path, got); err != nil {
			t.Error(err)
		} else {
			t.Logf("%s: wrote %s", name, path)
		}
		return golden.Result{}
	}
	want, err := golden.ReadPNG(path)
	if err != nil {
		t.Errorf("%s: %v (run with -update on Windows to create the golden)", name, err)
		return golden.Result{}
	}
	res, diff := golden.Compare(want, got, c.Options)
	t.Logf("%s: %v", name, res)
	if res.Pass {
		return res
	}
	out := c.Out
	if out == "" {
		out = os.Getenv("GOGUI_GOLDEN_OUT")
	}
	if out == "" {
		out = t.TempDir()
	}
	c.Out = out // one directory for every variant of the test
	actual := filepath.Join(out, name+".actual.png")
	files := actual
	if err := golden.WritePNG(actual, got); err != nil {
		t.Error(err)
	}
	if diff != nil {
		dp := filepath.Join(out, name+".diff.png")
		if err := golden.WritePNG(dp, diff); err != nil {
			t.Error(err)
		}
		files += ", " + dp
	}
	report := t.Errorf
	if c.LogOnly {
		report = t.Logf
	}
	report("%s differs from %s: %v; wrote %s", name, path, res, files)
	return res
}

// SweepThresholds are the thresholds Sweep compares at: around
// pixelmatch's 0.1, up to a tolerance that would hide real colour changes.
var SweepThresholds = []float64{0.05, 0.1, 0.15, 0.2, 0.3}

// Sweep is the calibration material of a tolerance (S4.3i): it compares
// got with the golden of v under c.Options at each of SweepThresholds,
// once without a mask and, when c.Options has one, once with it, and
// returns one line per comparison ("t=0.1 mask=off: <Result>"). It
// neither fails nor writes files; it errs only when the golden cannot be
// read.
func (c *Check) Sweep(v Variant, got *image.NRGBA) ([]string, error) {
	want, err := golden.ReadPNG(filepath.Join(c.Dir, v.Name()+".png"))
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, th := range SweepThresholds {
		opt := c.Options
		opt.Threshold = th
		masks := [][]image.Rectangle{nil}
		if len(c.Options.Mask) > 0 {
			masks = append(masks, c.Options.Mask)
		}
		for _, m := range masks {
			opt.Mask = m
			res, _ := golden.Compare(want, got, opt)
			on := "off"
			if m != nil {
				on = fmt.Sprintf("on (%d rects)", len(m))
			}
			lines = append(lines, fmt.Sprintf("t=%g mask=%s: %v", th, on, res))
		}
	}
	return lines, nil
}
