package goldentest

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua/gallery/golden"
)

func TestVariants(t *testing.T) {
	var names []string
	for _, v := range Variants {
		names = append(names, v.Name())
	}
	if got, want := strings.Join(names, " "), "gallery-light-x1 gallery-dark-x1 gallery-light-x2 gallery-dark-x2"; got != want {
		t.Errorf("names %q, want %q", got, want)
	}
	// Every variant has its committed golden.
	for _, n := range names {
		if _, err := os.Stat(filepath.Join("..", "testdata", "golden", n+".png")); err != nil {
			t.Error(err)
		}
	}
	if d := filepath.ToSlash(DirFromShell); d != "../../aqua/gallery/testdata/golden" {
		t.Errorf("DirFromShell %q", d)
	}
}

func TestFromBGRA(t *testing.T) {
	// BGRA in, RGBA out.
	for _, c := range []struct{ in, want [4]byte }{
		{[4]byte{10, 20, 30, 255}, [4]byte{30, 20, 10, 255}},       // opaque: only reordered
		{[4]byte{0, 0, 0, 0}, [4]byte{0, 0, 0, 0}},                 // transparent
		{[4]byte{9, 9, 9, 0}, [4]byte{0, 0, 0, 0}},                 // invalid colour under alpha 0
		{[4]byte{64, 32, 128, 128}, [4]byte{255, 64, 128, 128}},    // 32·255/128 = 63.75 rounds to 64
		{[4]byte{0, 1, 2, 3}, [4]byte{170, 85, 0, 3}},              // 2·255/3 = 170, 1·255/3 = 85
		{[4]byte{200, 200, 200, 100}, [4]byte{255, 255, 255, 100}}, // over-range colour clamps
	} {
		img := FromBGRA(1, 1, 4, c.in[:])
		if got := [4]byte(img.Pix); got != c.want {
			t.Errorf("FromBGRA(BGRA %v) = RGBA %v, want %v", c.in, got, c.want)
		}
	}
}

func TestFromBGRAStride(t *testing.T) {
	// 2×2 px with a 12-byte stride (4 bytes of padding per row, 0xEE), and
	// no padding after the last row.
	in := []byte{
		1, 2, 3, 255, 4, 5, 6, 255, 0xEE, 0xEE, 0xEE, 0xEE,
		7, 8, 9, 255, 10, 11, 12, 255,
	}
	img := FromBGRA(2, 2, 12, in)
	if img.Bounds() != image.Rect(0, 0, 2, 2) || img.Stride != 8 {
		t.Fatalf("bounds %v, stride %d", img.Bounds(), img.Stride)
	}
	want := []byte{3, 2, 1, 255, 6, 5, 4, 255, 9, 8, 7, 255, 12, 11, 10, 255}
	if string(img.Pix) != string(want) {
		t.Errorf("pixels %v, want %v", img.Pix, want)
	}
	for _, c := range []struct {
		w, h, stride, n int
	}{{2, 2, 7, 20}, {2, 2, 12, 19}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("FromBGRA(%d, %d, %d, %d bytes) did not panic", c.w, c.h, c.stride, c.n)
				}
			}()
			FromBGRA(c.w, c.h, c.stride, make([]byte, c.n))
		}()
	}
	if img := FromBGRA(0, 0, 0, nil); img.Bounds().Size() != (image.Point{}) {
		t.Errorf("empty: %v", img.Bounds())
	}
}

// recorder is a testing.TB that records what Run reports.
type recorder struct {
	testing.TB
	tmp        string
	errs, logs []string
}

func (r *recorder) Helper()           {}
func (r *recorder) Error(args ...any) { r.errs = append(r.errs, fmt.Sprint(args...)) }
func (r *recorder) Errorf(format string, a ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, a...))
}
func (r *recorder) Logf(format string, args ...any) {
	r.logs = append(r.logs, fmt.Sprintf(format, args...))
}
func (r *recorder) TempDir() string { return r.tmp }

// solid is a w×h image of one colour.
func solid(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

// files lists the names in dir.
func files(t *testing.T, dir string) string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var ns []string
	for _, e := range es {
		ns = append(ns, e.Name())
	}
	return strings.Join(ns, " ")
}

func TestCheckRun(t *testing.T) {
	grey := color.NRGBA{128, 128, 128, 255}
	v := Variant{Dark: true, Scale: 2}
	dir := t.TempDir()

	// Update writes the golden and compares nothing.
	r := &recorder{TB: t, tmp: t.TempDir()}
	c := &Check{Dir: dir, Options: golden.DefaultOptions, Update: true}
	if res := c.Run(r, v, solid(4, 3, grey)); res != (golden.Result{}) || len(r.errs) != 0 {
		t.Fatalf("update: %v, errors %q", res, r.errs)
	}
	if got := files(t, dir); got != "gallery-dark-x2.png" {
		t.Fatalf("golden dir after update: %q", got)
	}

	// A match: logged, nothing written.
	out := t.TempDir()
	r = &recorder{TB: t, tmp: t.TempDir()}
	c = &Check{Dir: dir, Options: golden.DefaultOptions, Out: out}
	if res := c.Run(r, v, solid(4, 3, grey)); !res.Pass || len(r.errs) != 0 {
		t.Errorf("match: %v, errors %q", res, r.errs)
	}
	if len(r.logs) != 1 || !strings.HasPrefix(r.logs[0], "gallery-dark-x2: pass") {
		t.Errorf("match logs %q", r.logs)
	}
	if got := files(t, out); got != "" {
		t.Errorf("match wrote %q", got)
	}

	// A different pixel: an error naming both files, which are written.
	changed := solid(4, 3, grey)
	changed.SetNRGBA(1, 1, color.NRGBA{255, 0, 0, 255})
	r = &recorder{TB: t, tmp: t.TempDir()}
	if res := c.Run(r, v, changed); res.Pass || res.Diff != 1 {
		t.Errorf("differ: %v", res)
	}
	if got := files(t, out); got != "gallery-dark-x2.actual.png gallery-dark-x2.diff.png" {
		t.Errorf("differ wrote %q", got)
	}
	if len(r.errs) != 1 || !strings.Contains(r.errs[0], "gallery-dark-x2 differs from") ||
		!strings.Contains(r.errs[0], filepath.Join(out, "gallery-dark-x2.diff.png")) {
		t.Errorf("differ errors %q", r.errs)
	}
	if act, err := golden.ReadPNG(filepath.Join(out, "gallery-dark-x2.actual.png")); err != nil ||
		string(act.Pix) != string(changed.Pix) {
		t.Errorf("actual.png: %v", err)
	}

	// LogOnly: the same report as a log; the images are still written.
	out = t.TempDir()
	r = &recorder{TB: t, tmp: t.TempDir()}
	c = &Check{Dir: dir, Options: golden.DefaultOptions, Out: out, LogOnly: true}
	c.Run(r, v, changed)
	if len(r.errs) != 0 || len(r.logs) != 2 || !strings.Contains(r.logs[1], "differs from") {
		t.Errorf("log only: errors %q, logs %q", r.errs, r.logs)
	}
	if got := files(t, out); got != "gallery-dark-x2.actual.png gallery-dark-x2.diff.png" {
		t.Errorf("log only wrote %q", got)
	}

	// A size mismatch: only the actual image (there is no diff).
	out = t.TempDir()
	r = &recorder{TB: t, tmp: t.TempDir()}
	c = &Check{Dir: dir, Options: golden.DefaultOptions, Out: out}
	if res := c.Run(r, v, solid(5, 3, grey)); !res.SizeMismatch || len(r.errs) != 1 {
		t.Errorf("size: %v, errors %q", res, r.errs)
	}
	if got := files(t, out); got != "gallery-dark-x2.actual.png" {
		t.Errorf("size wrote %q", got)
	}

	// No Out: $GOGUI_GOLDEN_OUT, else the test's temp dir, kept for the
	// next variant.
	env := t.TempDir()
	t.Setenv("GOGUI_GOLDEN_OUT", env)
	r = &recorder{TB: t, tmp: t.TempDir()}
	c = &Check{Dir: dir, Options: golden.DefaultOptions}
	c.Run(r, v, changed)
	if c.Out != env || files(t, env) != "gallery-dark-x2.actual.png gallery-dark-x2.diff.png" {
		t.Errorf("GOGUI_GOLDEN_OUT: out %q, files %q", c.Out, files(t, env))
	}
	t.Setenv("GOGUI_GOLDEN_OUT", "")
	r = &recorder{TB: t, tmp: t.TempDir()}
	c = &Check{Dir: dir, Options: golden.DefaultOptions}
	c.Run(r, v, changed)
	if c.Out != r.tmp || files(t, r.tmp) != "gallery-dark-x2.actual.png gallery-dark-x2.diff.png" {
		t.Errorf("TempDir: out %q, files %q", c.Out, files(t, r.tmp))
	}

	// A missing golden fails, even with LogOnly, and writes nothing.
	r = &recorder{TB: t, tmp: t.TempDir()}
	c = &Check{Dir: dir, Options: golden.DefaultOptions, Out: t.TempDir(), LogOnly: true}
	if res := c.Run(r, Variant{Scale: 1}, changed); res != (golden.Result{}) || len(r.errs) != 1 ||
		!strings.Contains(r.errs[0], "gallery-light-x1") || files(t, c.Out) != "" {
		t.Errorf("missing: %v, errors %q", res, r.errs)
	}
}
