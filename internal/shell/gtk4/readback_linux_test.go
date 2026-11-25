//go:build linux

package gtk4

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/radix29/gogui/internal/aqua/gallery/goldentest"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
)

// The readback over a real window (S4.3f): what the Linux gallery golden
// test (S4.3g) builds on. The window's own GSK renderer draws the
// GGSurface's last frame into a texture, and gdk_texture_download reads
// it back (readback.go, window_linux.go).

// rbPainter paints a plain surface: rbBg over all of it, and an rbSquare
// square at (10,10), 20×20 points, both on whole points so their edges are
// sharp at any whole scale.
type rbPainter struct{ *surfRec }

var (
	rbBg     = driver.RGBA(0.2, 0.4, 0.6, 1) // 51, 102, 153
	rbSquare = driver.RGBA(1, 0, 0, 1)
	rbSize   = driver.Size{W: 120, H: 80}
)

func (h *rbPainter) Paint(c render.Canvas) {
	h.surfRec.Paint(c)
	c.FillRoundedRect(render.Rect{W: 4000, H: 4000}, render.Radii{}, render.Solid(rbBg))
	c.FillRoundedRect(render.Rect{X: 10, Y: 10, W: 20, H: 20}, render.Radii{}, render.Solid(rbSquare))
}

// rbNRGBA is c as 8-bit straight alpha.
func rbNRGBA(c driver.Color) color.NRGBA {
	b := func(v float32) uint8 { return uint8(v*255 + 0.5) }
	return color.NRGBA{R: b(c.R), G: b(c.G), B: b(c.B), A: b(c.A)}
}

// rbNear reports whether a and b differ by at most 2 in every channel (a
// renderer may convert colors on the way).
func rbNear(a, b color.NRGBA) bool {
	d := func(x, y uint8) bool { return max(x, y)-min(x, y) <= 2 }
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B) && d(a.A, b.A)
}

// rbCheck checks a frame of rbPainter at scale sc: its size, and the
// background and the square on both sides of the square's edges, in
// pixels.
func rbCheck(img *image.NRGBA, sc int) error {
	sz := img.Bounds().Size()
	if sz.X != int(rbSize.W)*sc || sz.Y != int(rbSize.H)*sc {
		return fmt.Errorf("at %d×: the image is %d×%d, want %d×%d", sc, sz.X, sz.Y, int(rbSize.W)*sc, int(rbSize.H)*sc)
	}
	bg, sq := rbNRGBA(rbBg), rbNRGBA(rbSquare)
	in, out := 10*sc, 30*sc // the square's first pixel and the first pixel past it
	for _, p := range []struct {
		x, y int
		want color.NRGBA
	}{
		{0, 0, bg}, {sz.X - 1, sz.Y - 1, bg}, {sz.X / 2, sz.Y / 2, bg},
		{in, in, sq}, {out - 1, out - 1, sq}, {20 * sc, 20 * sc, sq},
		{in - 1, in, bg}, {in, in - 1, bg}, {out, out - 1, bg}, {out - 1, out, bg},
	} {
		if got := img.NRGBAAt(p.x, p.y); !rbNear(got, p.want) {
			return fmt.Errorf("at %d×: pixel (%d,%d) is %v, want %v", sc, p.x, p.y, got, p.want)
		}
	}
	return nil
}

// TestReadbackGTK: on a shell over the golden renders' appearance source
// (the appearance is the fixed one), a shown surface fixed as key reports
// KeyChanged(true); its frame reads back at a fixed 1× and 2× with the
// size and pixels of what it painted (rbCheck), twice the same, and the
// renderer is logged (READBACK-RESULT).
func TestReadbackGTK(t *testing.T) {
	needSurfaceGTK(t)
	runSurfaceTestWith(t, func(s *Shell) { s.openSettings = gallerySource().open }, func(e *surfTest) error {
		t := e.t
		if a := e.s.Appearance(); a != galleryAppearance() {
			t.Errorf("Appearance() = %+v, want the golden renders' %+v", a, galleryAppearance())
		}
		h := &rbPainter{surfRec: &surfRec{t: t, name: "readback", order: &e.order}}
		ss, err := e.s.NewSurface(shell.SurfaceConfig{Title: "gogui readback", Size: rbSize, Style: driver.StyleTitled}, h)
		if err != nil {
			return fmt.Errorf("NewSurface: %w", err)
		}
		e.recs = append(e.recs, h.surfRec)
		sf := ss.(*surface)
		g, ok := sf.w.(*gtkWindow)
		if !ok {
			return fmt.Errorf("the window is a %T, not a *gtkWindow", sf.w)
		}
		if _, err := sf.readback(); err != errReadbackHidden {
			t.Errorf("readback before Show = %v, want errReadbackHidden", err)
		}
		if err := show(sf, g, h.surfRec); err != nil {
			return err
		}
		native := sf.Scale()
		sf.setFixedKey(keyOn)
		if !sf.key || len(h.keys) == 0 || !h.keys[len(h.keys)-1] {
			t.Errorf("after setFixedKey(keyOn): key %v, KeyChanged calls %v", sf.key, h.keys)
		}
		for _, sc := range []int{1, 2} {
			sf.setFixedScale(float64(sc))
			if !iterateUntil(showWait, func() bool { return sf.dead || !sf.invalid }) || sf.dead {
				return fmt.Errorf("at %d×: no paint within %v of the scale change (closed %v)", sc, showWait, sf.dead)
			}
			iterateFor(settle)
			if sf.Size() != rbSize {
				t.Errorf("at %d×: Size() = %v, want %v", sc, sf.Size(), rbSize)
			}
			f, err := sf.readback()
			if err != nil {
				return fmt.Errorf("at %d×: %w", sc, err)
			}
			img := goldentest.FromBGRA(f.w, f.h, f.stride, f.pix)
			t.Logf("READBACK-RESULT: at %d× (native scale %v): %d×%d px, stride %d, renderer %s",
				sc, native, f.w, f.h, f.stride, f.renderer)
			if err := rbCheck(img, sc); err != nil {
				t.Error(err)
			}
			again, err := sf.readback()
			if err != nil {
				return fmt.Errorf("at %d×, again: %w", sc, err)
			}
			if !bytes.Equal(again.pix, f.pix) {
				t.Errorf("at %d×: a second readback differs", sc)
			}
		}
		sf.setFixedScale(0)
		sf.setFixedKey(keyNative)
		if sf.Scale() != native {
			t.Errorf("Scale() = %v after setFixedScale(0), want the native %v", sf.Scale(), native)
		}
		return nil
	})
}
