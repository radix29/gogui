package gtk4

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/render"
)

// The image host tests (S4.2e): validation and Size of imageSet.newImage,
// DrawImage over a fakeSnapshot (one texture made on first draw, then kept;
// the ignored cases draw and make nothing), the set's release, and the
// shell's set through surfaces over fake windows (kept across paints,
// released with the last surface, by Quit and at the end of Run).

// ihPix returns w×h RGBA pixels, no two bytes alike within 256.
func ihPix(w, h int) []byte {
	p := make([]byte, w*h*4)
	for i := range p {
		p[i] = byte(i*7 + 1)
	}
	return p
}

// ihNew returns a w×h image of set at scale.
func ihNew(t *testing.T, set *imageSet, w, h int, scale float64) *shellImage {
	t.Helper()
	im, err := set.newImage(ihPix(w, h), w, h, scale)
	if err != nil {
		t.Fatalf("newImage(%d×%d @%v) = %v", w, h, scale, err)
	}
	return im
}

// ihCanvas returns a canvas of the 100×50 test surface over a new
// fakeSnapshot, drawing the images of set.
func ihCanvas(set *imageSet, debug bool) (*canvas, *fakeSnapshot) {
	sn := &fakeSnapshot{}
	c := newCanvas(sn, render.Size{W: 100, H: 50}, 1, debug)
	c.images = set
	return c, sn
}

// ihDraw is cvDraw with images: without and with the recorder, each on a
// new set holding a new 4×2 image at scale 2, it paints, finishes and
// checks the snapshot balanced; the ops must be the same both times. It
// returns the ops, set, image and snapshot of the run without the recorder.
func ihDraw(t *testing.T, paint func(c *canvas, im *shellImage)) ([]string, *imageSet, *shellImage, *fakeSnapshot) {
	t.Helper()
	var (
		ops [2][]string
		set *imageSet
		im  *shellImage
		sn  *fakeSnapshot
	)
	for i, debug := range []bool{false, true} {
		is := &imageSet{}
		img := ihNew(t, is, 4, 2, 2)
		c, s := ihCanvas(is, debug)
		paint(c, img)
		if err := c.finish(); !debug && err != nil {
			t.Errorf("finish without a recorder = %v, want nil", err)
		}
		if err := s.balanced(); err != nil {
			t.Errorf("debug=%v: snapshot not balanced: %v\nops:\n%s", debug, err, strings.Join(s.ops, "\n"))
		}
		ops[i] = s.ops
		if i == 0 {
			set, im, sn = is, img, s
		}
	}
	if !slices.Equal(ops[0], ops[1]) {
		t.Errorf("ops differ with the recorder:\nwithout:\n%s\nwith:\n%s",
			strings.Join(ops[0], "\n"), strings.Join(ops[1], "\n"))
	}
	return ops[0], set, im, sn
}

// TestImageNewInvalid: newImage refuses empty, negative and oversized
// pixel sizes, a scale that is not positive and finite, and pixels of the
// wrong length (nil included). Each error is that of the first check the
// arguments fail (the size, then the scale, then the length), so an
// oversized side is refused as such, not only for its pixels' length.
func TestImageNewInvalid(t *testing.T) {
	const (
		bySize   = "image of "
		byScale  = "image scale"
		byLength = "bytes of pixels"
	)
	var set imageSet
	for _, tc := range []struct {
		name  string
		pix   []byte
		w, h  int
		scale float64
		want  string // in the error
	}{
		{"zero width", nil, 0, 1, 1, bySize},
		{"zero height", nil, 1, 0, 1, bySize},
		{"zero both", []byte{}, 0, 0, 1, bySize},
		{"negative width", make([]byte, 4), -1, 1, 1, bySize},
		{"negative height", make([]byte, 4), 1, -1, 1, bySize},
		{"negative both", make([]byte, 4), -1, -1, 1, bySize},
		{"width too large", nil, maxImageSide + 1, 1, 1, bySize},
		{"height too large", nil, 1, maxImageSide + 1, 1, bySize},
		{"max side, short pixels", make([]byte, 4), maxImageSide, maxImageSide, 1, byLength},
		{"bad size before bad scale", nil, 0, 1, cvNaN, bySize},
		{"scale 0", make([]byte, 4), 1, 1, 0, byScale},
		{"scale negative", make([]byte, 4), 1, 1, -2, byScale},
		{"scale NaN", make([]byte, 4), 1, 1, math.NaN(), byScale},
		{"scale +Inf", make([]byte, 4), 1, 1, math.Inf(1), byScale},
		{"scale -Inf", make([]byte, 4), 1, 1, math.Inf(-1), byScale},
		{"bad scale before bad length", nil, 1, 1, 0, byScale},
		{"pixels nil", nil, 2, 2, 1, byLength},
		{"pixels short", make([]byte, 15), 2, 2, 1, byLength},
		{"pixels short by a pixel", make([]byte, 12), 2, 2, 1, byLength},
		{"pixels long", make([]byte, 17), 2, 2, 1, byLength},
		{"pixels long by a row", make([]byte, 24), 2, 2, 1, byLength},
	} {
		t.Run(tc.name, func(t *testing.T) {
			im, err := set.newImage(tc.pix, tc.w, tc.h, tc.scale)
			if err == nil || im != nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("newImage(%d bytes, %d×%d, scale %v) = %v, %v; want an error with %q",
					len(tc.pix), tc.w, tc.h, tc.scale, im, err, tc.want)
			}
		})
	}
	if set.held != nil || set.made != 0 {
		t.Errorf("failed newImage calls touched the set: %d held, %d made", len(set.held), set.made)
	}
}

// TestImageNew: a valid image belongs to its set, keeps a copy of the
// pixels, its pixel size and scale, and holds no texture until drawn.
func TestImageNew(t *testing.T) {
	var set imageSet
	src := ihPix(3, 2)
	want := slices.Clone(src)
	im, err := set.newImage(src, 3, 2, 1.5)
	if err != nil {
		t.Fatal(err)
	}
	if im.set != &set || im.w != 3 || im.h != 2 || im.scale != 1.5 || im.tex != nil || im.failed {
		t.Errorf("image %+v, want of the set, 3×2 @1.5, no texture", im)
	}
	if !slices.Equal(im.pix, want) {
		t.Errorf("pixels %v, want %v", im.pix, want)
	}
	for i := range src {
		src[i] = 0
	}
	if !slices.Equal(im.pix, want) {
		t.Error("changing the caller's pixels changed the image's")
	}
	if len(set.held) != 0 || set.made != 0 {
		t.Errorf("newImage held %d, made %d; want nothing before a draw", len(set.held), set.made)
	}
	// A one-pixel image is valid.
	if _, err := set.newImage(make([]byte, 4), 1, 1, 1); err != nil {
		t.Errorf("1×1 image: %v", err)
	}
}

// TestImageSize: Size is the pixel size over the scale; a typed-nil image
// has a zero Size.
func TestImageSize(t *testing.T) {
	var set imageSet
	for _, tc := range []struct {
		w, h  int
		scale float64
		want  render.Size
	}{
		{4, 2, 1, render.Size{W: 4, H: 2}},
		{4, 2, 2, render.Size{W: 2, H: 1}},
		{5, 3, 1.25, render.Size{W: 4, H: 3 / 1.25}},
		{3, 7, 0.5, render.Size{W: 6, H: 14}},
		{1, 1, 3, render.Size{W: 1.0 / 3, H: 1.0 / 3}},
	} {
		t.Run(fmt.Sprintf("%dx%d@%v", tc.w, tc.h, tc.scale), func(t *testing.T) {
			if got := ihNew(t, &set, tc.w, tc.h, tc.scale).Size(); got != tc.want {
				t.Errorf("Size = %+v, want %+v", got, tc.want)
			}
		})
	}
	if got := (*shellImage)(nil).Size(); got != (render.Size{}) {
		t.Errorf("Size of a nil image = %+v, want zero", got)
	}
}

// TestImageDraw: the first draw makes the image's texture from its pixels
// and draws it into dst inside the base clip; under Save, the transforms,
// a rounded clip and PushOpacity, the draw keeps its place among the
// state ops, dst passed as given (the transforms are the snapshot's).
func TestImageDraw(t *testing.T) {
	dst := render.Rect{X: 1, Y: 2, W: 3, H: 4}
	t.Run("plain", func(t *testing.T) {
		ops, set, im, sn := ihDraw(t, func(c *canvas, im *shellImage) { c.DrawImage(im, dst) })
		cvWantOps(t, ops, cvOps("texture #1 4x2", "draw texture #1 (1,2,3,4)"))
		if len(sn.textures) != 1 {
			t.Fatalf("%d textures made, want 1", len(sn.textures))
		}
		tex := sn.textures[0]
		if tex.w != 4 || tex.h != 2 || !slices.Equal(tex.pix, im.pix) {
			t.Errorf("texture %d×%d of %v, want 4×2 of the image's pixels %v", tex.w, tex.h, tex.pix, im.pix)
		}
		if im.tex != texture(tex) || set.made != 1 {
			t.Errorf("image texture %v, made %d; want the fake's, 1", im.tex, set.made)
		}
		if _, ok := set.held[im]; !ok || len(set.held) != 1 {
			t.Errorf("held %v, want the image", set.held)
		}
	})
	t.Run("fractional dst", func(t *testing.T) {
		ops, _, _, _ := ihDraw(t, func(c *canvas, im *shellImage) {
			c.DrawImage(im, render.Rect{X: -0.5, Y: 0.25, W: 200.5, H: 0.125})
		})
		cvWantOps(t, ops, cvOps("texture #1 4x2", "draw texture #1 (-0.5,0.25,200.5,0.125)"))
	})
	t.Run("under state", func(t *testing.T) {
		ops, _, _, _ := ihDraw(t, func(c *canvas, im *shellImage) {
			c.Save()
			c.Translate(5, 6)
			c.Scale(2, 3)
			c.ClipRoundedRect(render.Rect{X: 1, Y: 1, W: 20, H: 10}, render.UniformRadii(2))
			c.PushOpacity(0.5)
			c.DrawImage(im, dst)
			c.Pop()
			c.Restore()
			c.DrawImage(im, render.Rect{W: 8, H: 8})
		})
		cvWantOps(t, ops, cvOps("save", "translate 5,6", "scale 2,3", "clip (1,1,20,10) r=2",
			"opacity 0.5", "save", "texture #1 4x2", "draw texture #1 (1,2,3,4)", "restore", "pop",
			"pop", "restore", "draw texture #1 (0,0,8,8)"))
	})
}

// TestImageTextureOnce: an image's texture is made on its first draw only,
// for draws in one paint and over later paints into other snapshots (the
// later ones draw the first snapshot's texture); two images make two.
func TestImageTextureOnce(t *testing.T) {
	r := render.Rect{W: 4, H: 2}
	t.Run("one paint", func(t *testing.T) {
		ops, set, _, _ := ihDraw(t, func(c *canvas, im *shellImage) {
			c.DrawImage(im, r)
			c.DrawImage(im, render.Rect{X: 10, W: 8, H: 4})
		})
		cvWantOps(t, ops, cvOps("texture #1 4x2", "draw texture #1 (0,0,4,2)", "draw texture #1 (10,0,8,4)"))
		if set.made != 1 {
			t.Errorf("made %d, want 1", set.made)
		}
	})
	t.Run("frames", func(t *testing.T) {
		set := &imageSet{}
		im := ihNew(t, set, 4, 2, 2)
		for frame := range 3 {
			for _, debug := range []bool{false, true} {
				c, sn := ihCanvas(set, debug)
				c.DrawImage(im, r)
				c.finish()
				want := cvOps("draw texture #1 (0,0,4,2)")
				if frame == 0 && !debug {
					want = cvOps("texture #1 4x2", "draw texture #1 (0,0,4,2)")
				}
				cvWantOps(t, sn.ops, want)
			}
		}
		if set.made != 1 || len(set.held) != 1 {
			t.Errorf("made %d, held %d; want 1, 1", set.made, len(set.held))
		}
	})
	t.Run("same snapshot", func(t *testing.T) {
		set := &imageSet{}
		im := ihNew(t, set, 4, 2, 2)
		sn := &fakeSnapshot{}
		for range 2 {
			c := newCanvas(sn, render.Size{W: 100, H: 50}, 1, false)
			c.images = set
			c.DrawImage(im, r)
			c.finish()
		}
		cvWantOps(t, sn.ops, append(cvOps("texture #1 4x2", "draw texture #1 (0,0,4,2)"),
			cvOps("draw texture #1 (0,0,4,2)")...))
		if len(sn.textures) != 1 || set.made != 1 {
			t.Errorf("%d textures, made %d; want 1, 1", len(sn.textures), set.made)
		}
	})
	t.Run("two images", func(t *testing.T) {
		ops, set, _, _ := ihDraw(t, func(c *canvas, im *shellImage) {
			other := ihNew(t, c.images, 1, 3, 1)
			c.DrawImage(im, r)
			c.DrawImage(other, r)
			c.DrawImage(im, r)
			c.DrawImage(other, r)
		})
		cvWantOps(t, ops, cvOps("texture #1 4x2", "draw texture #1 (0,0,4,2)", "texture #2 1x3",
			"draw texture #2 (0,0,4,2)", "draw texture #1 (0,0,4,2)", "draw texture #2 (0,0,4,2)"))
		if set.made != 2 || len(set.held) != 2 {
			t.Errorf("made %d, held %d; want 2, 2", set.made, len(set.held))
		}
	})
}

// ihWrapped is a render.Image that wraps a shell image of the set but is
// not a *shellImage (cvImage is another type altogether).
type ihWrapped struct{ *shellImage }

// TestImageIgnored: nil images (typed too), images of another set and
// other render.Images, a canvas without a set, and an empty, negative,
// non-finite or float32-overflowing dst draw nothing and make no texture;
// nor does a draw after finish.
func TestImageIgnored(t *testing.T) {
	ok := render.Rect{X: 1, Y: 2, W: 3, H: 4}
	foreign := ihNew(t, &imageSet{}, 4, 2, 2)
	for _, tc := range []struct {
		name  string
		paint func(c *canvas, im *shellImage)
	}{
		{"nil", func(c *canvas, _ *shellImage) { c.DrawImage(nil, ok) }},
		{"typed nil", func(c *canvas, _ *shellImage) { c.DrawImage((*shellImage)(nil), ok) }},
		{"another set", func(c *canvas, _ *shellImage) { c.DrawImage(foreign, ok) }},
		{"another set's empty image", func(c *canvas, _ *shellImage) {
			c.DrawImage(&shellImage{w: 1, h: 1, scale: 1, pix: make([]byte, 4)}, ok)
		}},
		{"other render.Image", func(c *canvas, _ *shellImage) { c.DrawImage(cvImage{}, ok) }},
		{"wrapped image", func(c *canvas, im *shellImage) { c.DrawImage(ihWrapped{im}, ok) }},
		{"no set", func(c *canvas, im *shellImage) { c.images = nil; c.DrawImage(im, ok) }},
		{"no set, image of none", func(c *canvas, _ *shellImage) {
			c.images = nil
			c.DrawImage(&shellImage{w: 1, h: 1, scale: 1, pix: make([]byte, 4)}, ok)
		}},
		{"zero dst", func(c *canvas, im *shellImage) { c.DrawImage(im, render.Rect{}) }},
		{"zero W", func(c *canvas, im *shellImage) { c.DrawImage(im, render.Rect{X: 1, Y: 1, H: 4}) }},
		{"zero H", func(c *canvas, im *shellImage) { c.DrawImage(im, render.Rect{X: 1, Y: 1, W: 4}) }},
		{"negative W", func(c *canvas, im *shellImage) { c.DrawImage(im, render.Rect{W: -1, H: 4}) }},
		{"negative H", func(c *canvas, im *shellImage) { c.DrawImage(im, render.Rect{W: 4, H: -1}) }},
		{"NaN X", func(c *canvas, im *shellImage) { c.DrawImage(im, render.Rect{X: cvNaN, W: 4, H: 4}) }},
		{"NaN Y", func(c *canvas, im *shellImage) { c.DrawImage(im, render.Rect{Y: cvNaN, W: 4, H: 4}) }},
		{"NaN W", func(c *canvas, im *shellImage) { c.DrawImage(im, render.Rect{W: cvNaN, H: 4}) }},
		{"NaN H", func(c *canvas, im *shellImage) { c.DrawImage(im, render.Rect{W: 4, H: cvNaN}) }},
		{"Inf X", func(c *canvas, im *shellImage) { c.DrawImage(im, render.Rect{X: cvInf, W: 4, H: 4}) }},
		{"-Inf Y", func(c *canvas, im *shellImage) { c.DrawImage(im, render.Rect{Y: -cvInf, W: 4, H: 4}) }},
		{"Inf W", func(c *canvas, im *shellImage) { c.DrawImage(im, render.Rect{W: cvInf, H: 4}) }},
		{"Inf H", func(c *canvas, im *shellImage) { c.DrawImage(im, render.Rect{W: 4, H: cvInf}) }},
		{"float32 overflow X", func(c *canvas, im *shellImage) { c.DrawImage(im, render.Rect{X: cvBigF32, W: 4, H: 4}) }},
		{"float32 overflow W", func(c *canvas, im *shellImage) { c.DrawImage(im, render.Rect{W: cvBigF32, H: 4}) }},
		{"float32 overflow -H", func(c *canvas, im *shellImage) { c.DrawImage(im, render.Rect{W: 4, H: -cvBigF32}) }},
		{"float32 underflow to 0", func(c *canvas, im *shellImage) { c.DrawImage(im, render.Rect{W: 1e-50, H: 4}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ops, set, im, sn := ihDraw(t, tc.paint)
			cvWantOps(t, ops, cvOps())
			if set.made != 0 || len(set.held) != 0 || len(sn.textures) != 0 || im.tex != nil {
				t.Errorf("made %d, held %d, %d textures, image texture %v; want none",
					set.made, len(set.held), len(sn.textures), im.tex)
			}
		})
	}
	t.Run("after finish", func(t *testing.T) {
		for _, debug := range []bool{false, true} {
			set := &imageSet{}
			im := ihNew(t, set, 4, 2, 2)
			c, sn := ihCanvas(set, debug)
			c.finish()
			c.DrawImage(im, ok)
			cvWantOps(t, sn.ops, cvOps())
			if set.made != 0 || im.tex != nil {
				t.Errorf("debug=%v: made %d after finish", debug, set.made)
			}
		}
	})
}

// TestImageTextureFails: a texture GDK can't make draws nothing and is not
// tried again, in the same paint or later ones, until the set is released;
// then it is, and a texture made then is drawn.
func TestImageTextureFails(t *testing.T) {
	r := render.Rect{W: 4, H: 2}
	set := &imageSet{}
	im := ihNew(t, set, 4, 2, 2)
	paint := func(fail bool) *fakeSnapshot {
		c, sn := ihCanvas(set, false)
		sn.failTextures = fail
		c.DrawImage(im, r)
		c.DrawImage(im, r)
		c.finish()
		return sn
	}
	cvWantOps(t, paint(true).ops, cvOps("texture 4x2 failed"))
	if !im.failed || im.tex != nil || set.made != 0 {
		t.Errorf("after the failure: failed %v, texture %v, made %d", im.failed, im.tex, set.made)
	}
	// Not retried, even by a snapshot that could make it.
	cvWantOps(t, paint(false).ops, cvOps())
	set.release()
	if im.failed {
		t.Error("release kept the failure")
	}
	cvWantOps(t, paint(true).ops, cvOps("texture 4x2 failed"))
	set.release()
	cvWantOps(t, paint(false).ops, cvOps("texture #1 4x2", "draw texture #1 (0,0,4,2)", "draw texture #1 (0,0,4,2)"))
	if set.made != 1 || im.failed || im.tex == nil {
		t.Errorf("after the retry: made %d, failed %v, texture %v", set.made, im.failed, im.tex)
	}
}

// TestImageRelease: release drops each texture made exactly once and
// forgets the images; the next draw makes a new texture; releasing an
// empty set, or twice, is harmless.
func TestImageRelease(t *testing.T) {
	var empty imageSet
	empty.release()
	empty.release()

	set := &imageSet{}
	a, b, idle := ihNew(t, set, 4, 2, 2), ihNew(t, set, 2, 2, 1), ihNew(t, set, 1, 1, 1)
	c, sn := ihCanvas(set, false)
	c.DrawImage(a, render.Rect{W: 1, H: 1})
	c.DrawImage(b, render.Rect{W: 1, H: 1})
	c.finish()
	if len(sn.textures) != 2 || set.made != 2 {
		t.Fatalf("%d textures, made %d; want 2", len(sn.textures), set.made)
	}
	set.release()
	set.release()
	for i, tex := range sn.textures {
		if tex.released != 1 {
			t.Errorf("texture #%d released %d times, want once", i+1, tex.released)
		}
	}
	if a.tex != nil || b.tex != nil || idle.tex != nil || len(set.held) != 0 {
		t.Errorf("after release: textures %v %v %v, %d held", a.tex, b.tex, idle.tex, len(set.held))
	}

	c, sn2 := ihCanvas(set, false)
	c.DrawImage(a, render.Rect{W: 1, H: 1})
	c.finish()
	cvWantOps(t, sn2.ops, cvOps("texture #1 4x2", "draw texture #1 (0,0,1,1)"))
	if set.made != 3 || a.tex != texture(sn2.textures[0]) {
		t.Errorf("after a new draw: made %d, texture %v", set.made, a.tex)
	}
	set.release()
	if sn.textures[0].released != 1 || sn2.textures[0].released != 1 {
		t.Errorf("released %d, %d; want the old texture once and the new one once",
			sn.textures[0].released, sn2.textures[0].released)
	}
}

// ihTextureOps returns the image ops of sn: the textures made and drawn.
func ihTextureOps(sn *fakeSnapshot) []string {
	var ops []string
	for _, op := range sn.ops {
		if strings.HasPrefix(op, "texture") || strings.HasPrefix(op, "draw texture") {
			ops = append(ops, op)
		}
	}
	return ops
}

// TestImageShellSurfaces: a surface's paint draws the shell's images
// through the shell's set; the texture is kept across paints and surfaces,
// kept while a surface is open, released when the last one closes, and
// made again by the next paint.
func TestImageShellSurfaces(t *testing.T) {
	s, f := newSurfaceTest(t)
	im, err := s.newImage(ihPix(4, 2), 4, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if im.set != &s.images {
		t.Fatal("Shell.newImage made an image of another set")
	}
	paint := func(c render.Canvas) { c.DrawImage(im, render.Rect{X: 1, Y: 2, W: 2, H: 1}) }
	a, fa := f.open(t, sfResizable, &sfHandler{onPaint: paint})
	b, fb := f.open(t, sfResizable, &sfHandler{onPaint: paint})
	draw := "draw texture #1 (1,2,2,1)"

	fa.snap()
	fa.snap()
	fb.snap()
	wantLog(t, "a's first paint", ihTextureOps(fa.snaps[0]), "texture #1 4x2", draw)
	wantLog(t, "a's second paint", ihTextureOps(fa.snaps[1]), draw)
	wantLog(t, "b's paint", ihTextureOps(fb.snaps[0]), draw)
	tex := fa.snaps[0].textures[0]
	if s.images.made != 1 || im.tex != texture(tex) {
		t.Fatalf("made %d, texture %v; want 1, a's first", s.images.made, im.tex)
	}

	a.Close()
	if tex.released != 0 || im.tex == nil {
		t.Errorf("closing one of two surfaces released the texture (%d)", tex.released)
	}
	fb.snap()
	wantLog(t, "b's paint after a closed", ihTextureOps(fb.snaps[1]), draw)

	b.Close()
	if tex.released != 1 || im.tex != nil || len(s.images.held) != 0 {
		t.Errorf("after the last close: released %d, texture %v, %d held; want 1, nil, 0",
			tex.released, im.tex, len(s.images.held))
	}

	_, fc := f.open(t, sfResizable, &sfHandler{onPaint: paint})
	fc.snap()
	wantLog(t, "a new surface's paint", ihTextureOps(fc.snaps[0]), "texture #1 4x2", draw)
	if s.images.made != 2 || im.tex != texture(fc.snaps[0].textures[0]) || tex.released != 1 {
		t.Errorf("made %d, old texture released %d; want 2, 1", s.images.made, tex.released)
	}
}

// TestImageShellQuit: Quit, closing the surfaces, releases the textures.
func TestImageShellQuit(t *testing.T) {
	s, f := newSurfaceTest(t)
	im, err := s.newImage(ihPix(1, 1), 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	paint := func(c render.Canvas) { c.DrawImage(im, render.Rect{W: 1, H: 1}) }
	_, fa := f.open(t, sfResizable, &sfHandler{onPaint: paint})
	f.open(t, sfResizable, &sfHandler{})
	fa.snap()
	tex := fa.snaps[0].textures[0]
	s.Quit()
	if tex.released != 1 || im.tex != nil {
		t.Errorf("after Quit: released %d, texture %v; want 1, nil", tex.released, im.tex)
	}
}

// TestImageShellRunEnd: the end of Run (shutdown) releases the shell's
// textures, also those no surface holds and after a panic. The image is
// drawn on a canvas of the shell's set in Ready, as no surface is open.
func TestImageShellRunEnd(t *testing.T) {
	for _, panics := range []bool{false, true} {
		t.Run(fmt.Sprint("panic=", panics), func(t *testing.T) {
			s, l := newTestShell(t)
			base := dispatch.Len()
			var tex *fakeTexture
			var im *shellImage
			ready := func() {
				var err error
				if im, err = s.newImage(ihPix(2, 1), 2, 1, 1); err != nil {
					t.Error(err)
					return
				}
				c, sn := ihCanvas(&s.images, false)
				c.DrawImage(im, render.Rect{W: 2, H: 1})
				c.finish()
				if len(sn.textures) == 1 {
					tex = sn.textures[0]
				}
				if panics {
					panic("in ready")
				}
				s.Post(s.Quit)
			}
			if panics {
				if got := runPanics(t, s, &handler{ready: ready}); got != "in ready" {
					t.Errorf("Run panicked with %v", got)
				}
			} else if err := run(t, s, &handler{ready: ready}); err != nil {
				t.Fatal(err)
			}
			if tex == nil {
				t.Fatal("no texture made")
			}
			if tex.released != 1 || im.tex != nil || len(s.images.held) != 0 {
				t.Errorf("after Run: released %d, texture %v, %d held; want 1, nil, 0",
					tex.released, im.tex, len(s.images.held))
			}
			checkTornDown(t, s, l, base)
		})
	}
}
