//go:build linux

package gtk4

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"unsafe"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// Images over a real GtkSnapshot and a real surface (S4.2e). The host
// tests (image_test.go) check validation, Size and the calls the canvas
// makes over the fake snapshotter; these check what GTK makes of them:
// one texture node per draw with dst's bounds (in the root's
// coordinates), the image's own GdkTexture holding its pixels, made once
// over frames and released with the shell's last surface.
//
// gtk_snapshot_append_scaled_texture with GSK_SCALING_FILTER_LINEAR may
// append a plain texture node (linear is a texture node's filter) rather
// than a texture-scale node; both are accepted and which one GTK made is
// logged (CANVAS-RESULT). A texture-scale node must carry the linear
// filter.

// igGsk holds the texture-node getters only these tests call.
var igGsk struct {
	TextureNodeGetTexture      func(n uintptr) uintptr // GdkTexture * (const GskRenderNode *), transfer none
	TextureScaleNodeGetTexture func(n uintptr) uintptr // GdkTexture * (const GskRenderNode *), transfer none (4.10)
	TextureScaleNodeGetFilter  func(n uintptr) int32   // GskScalingFilter (const GskRenderNode *) (4.10)
}

var igBindOnce = sync.OnceValue(func() error {
	lib, err := ffi.Dlopen(gtk.GTKLib)
	if err != nil {
		return err
	}
	return errors.Join(
		lib.Bind(&igGsk.TextureNodeGetTexture, "gsk_texture_node_get_texture"),
		lib.Bind(&igGsk.TextureScaleNodeGetTexture, "gsk_texture_scale_node_get_texture"),
		lib.Bind(&igGsk.TextureScaleNodeGetFilter, "gsk_texture_scale_node_get_filter"),
	)
})

// igNeed is cnNeed plus the texture-node getters.
func igNeed(t *testing.T) {
	t.Helper()
	cnNeed(t)
	if err := igBindOnce(); err != nil {
		t.Fatalf("binding the texture-node getters: %v", err)
	}
}

// igPix is a 2×2 straight-alpha RGBA image: a half-transparent red, an
// opaque green, an opaque blue and a quarter-transparent white.
var igPix = []byte{
	255, 0, 0, 128, 0, 255, 0, 255,
	0, 0, 255, 255, 255, 255, 255, 64,
}

// igPremulBGRA is what gdk_texture_download gives for straight RGBA pix:
// premultiplied Cairo ARGB32, so B, G, R, A bytes on little-endian.
func igPremulBGRA(pix []byte) []byte {
	out := make([]byte, len(pix))
	for i := 0; i+3 < len(pix); i += 4 {
		a := float64(pix[i+3])
		p := func(c byte) byte { return byte(math.Round(float64(c) * a / 255)) }
		out[i], out[i+1], out[i+2], out[i+3] = p(pix[i+2]), p(pix[i+1]), p(pix[i]), pix[i+3]
	}
	return out
}

// igDownload returns the w×h texture tex as gdk_texture_download writes
// it (stride w·4), through g_malloc memory: no Go pointer reaches GDK.
func igDownload(tex uintptr, w, h int) ([]byte, error) {
	n := w * h * 4
	buf := gtk.GLib.Malloc0(uintptr(n))
	if buf == 0 {
		return nil, errors.New("g_malloc0 returned NULL")
	}
	defer gtk.GLib.Free(buf)
	gtk.Gdk.TextureDownload(tex, buf, uintptr(w*4))
	return ffi.CopyBytes(buf, n), nil
}

// igTexNodes returns the texture and texture-scale nodes under root.
func igTexNodes(root *cnNode) []*cnNode {
	return root.find(func(k *cnNode) bool { return k.typ == gtk.NodeTexture || k.typ == gtk.NodeTextureScale })
}

// igTexOf returns the GdkTexture of a texture or texture-scale node.
func igTexOf(n *cnNode) uintptr {
	if n.typ == gtk.NodeTextureScale {
		return igGsk.TextureScaleNodeGetTexture(n.ptr)
	}
	return igGsk.TextureNodeGetTexture(n.ptr)
}

// igTexPtr is im's GdkTexture, 0 if it has none.
func igTexPtr(im *shellImage) uintptr {
	if g, ok := im.tex.(gdkTexture); ok {
		return uintptr(g)
	}
	return 0
}

// igCheckNode checks a texture node: bounds want in the root's
// coordinates, the texture tex of w×h pixels, the linear filter if it is
// a texture-scale node. It logs the node type GTK made.
func igCheckNode(t *testing.T, what string, n *cnNode, want gtk.GrapheneRect, tex uintptr, w, h int) error {
	t.Logf("CANVAS-RESULT: image %s: gtk_snapshot_append_scaled_texture (linear) made a %s node", what, cnTypeNames[n.typ])
	if !cnRectNear(n.global, want) {
		return fmt.Errorf("%s: the texture node is at %s, want %s", what, cnRectString(n.global), cnRectString(want))
	}
	if n.typ == gtk.NodeTextureScale {
		if f := igGsk.TextureScaleNodeGetFilter(n.ptr); f != gtk.ScalingFilterLinear {
			return fmt.Errorf("%s: the texture-scale node's filter is %d, want linear (%d)", what, f, gtk.ScalingFilterLinear)
		}
	}
	got := igTexOf(n)
	if tex == 0 || got != tex {
		return fmt.Errorf("%s: the node's texture is %#x, want the image's %#x", what, got, tex)
	}
	if gw, gh := gtk.Gdk.TextureGetWidth(got), gtk.Gdk.TextureGetHeight(got); int(gw) != w || int(gh) != h {
		return fmt.Errorf("%s: the texture is %d×%d, want %d×%d", what, gw, gh, w, h)
	}
	return nil
}

// igRelease releases set on the GTK goroutine.
func igRelease(t *testing.T, set *imageSet) {
	t.Helper()
	onGTK(t, func() error { set.release(); return nil })
}

// TestImageGTKNode: DrawImage is one texture node per draw with dst's
// bounds, also under Translate and Scale, holding the image's GdkTexture
// (2×2, its pixels premultiplied when downloaded); both draws share one
// texture. Nil, a typed nil, an image of another set, an empty or invalid
// dst and a canvas without a set add no texture node. Under a rounded
// clip and PushOpacity the node is under both.
func TestImageGTKNode(t *testing.T) {
	igNeed(t)
	set, other := &imageSet{}, &imageSet{}
	im, err := set.newImage(igPix, 2, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := other.newImage(igPix, 2, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer igRelease(t, other)
	defer igRelease(t, set)

	cnPaint(t, "image", cnSize, 1, func(c *canvas) {
		c.images = set
		c.DrawImage(im, render.Rect{X: 10, Y: 20, W: 40, H: 30})
		c.Save()
		c.Translate(100, 10)
		c.Scale(2, 0.5)
		c.DrawImage(im, render.Rect{X: 5, Y: 4, W: 20, H: 40}) // (110,12 40×20) in the root
		c.Restore()
	}, func(root *cnNode, ferr error) error {
		nodes := igTexNodes(root)
		if len(nodes) != 2 {
			return fmt.Errorf("%d texture nodes, want 2", len(nodes))
		}
		if set.made != 1 {
			return fmt.Errorf("the set made %d textures for two draws of one image, want 1", set.made)
		}
		tex := igTexPtr(im)
		for _, w := range []struct {
			name string
			r    gtk.GrapheneRect
		}{{"plain", cnRect(10, 20, 40, 30)}, {"translated and scaled", cnRect(110, 12, 40, 20)}} {
			var at []*cnNode
			for _, n := range nodes {
				if cnRectNear(n.global, w.r) {
					at = append(at, n)
				}
			}
			n, err := cnOne(w.name+" texture nodes at "+cnRectString(w.r), at)
			if err != nil {
				return err
			}
			if err := igCheckNode(t, w.name, n, w.r, tex, 2, 2); err != nil {
				return err
			}
		}
		got, err := igDownload(tex, 2, 2)
		if err != nil {
			return err
		}
		want := igPremulBGRA(igPix)
		for i := range want {
			if d := int(got[i]) - int(want[i]); d < -1 || d > 1 {
				return fmt.Errorf("gdk_texture_download gave %v, want %v (premultiplied BGRA, ±1)", got, want)
			}
		}
		t.Logf("CANVAS-RESULT: image: gdk_texture_download of the straight RGBA %v gave %v", igPix, got)
		return cnNoErr(ferr)
	})

	cnPaint(t, "image ignored", cnSize, 1, func(c *canvas) {
		c.images = set
		dst := render.Rect{X: 10, Y: 10, W: 20, H: 20}
		c.DrawImage(nil, dst) // the recorder reports a nil image
		c.DrawImage((*shellImage)(nil), dst)
		c.DrawImage(foreign, dst)
		c.DrawImage(im, render.Rect{X: 10, Y: 10, W: 0, H: 20})
		c.DrawImage(im, render.Rect{X: 10, Y: 10, W: 20, H: 0})
		c.DrawImage(im, render.Rect{X: 10, Y: 10, W: -20, H: 20})
		c.DrawImage(im, render.Rect{X: math.NaN(), Y: 10, W: 20, H: 20})
		c.FillRoundedRect(render.Rect{X: 60, Y: 10, W: 20, H: 20}, render.Radii{}, render.Solid(cnGreen))
	}, func(root *cnNode, ferr error) error {
		t.Logf("CANVAS-RESULT: image ignored: the recorder reported %v", ferr)
		if n := len(igTexNodes(root)); n != 0 {
			return fmt.Errorf("the ignored draws made %d texture nodes, want none", n)
		}
		if other.made != 0 || foreign.tex != nil {
			return fmt.Errorf("drawing a foreign image made %d textures in its set (texture %v), want none", other.made, foreign.tex)
		}
		if _, err := cnOne("green color nodes (after the ignored draws)", root.colorNodes(cnGreen)); err != nil {
			return err
		}
		return nil
	})

	cnPaint(t, "image without a set", cnSize, 1, func(c *canvas) {
		c.DrawImage(foreign, render.Rect{X: 10, Y: 10, W: 20, H: 20})
	}, func(root *cnNode, ferr error) error {
		if n := len(igTexNodes(root)); n != 0 || other.made != 0 {
			return fmt.Errorf("a canvas without a set made %d texture nodes and %d textures, want none", n, other.made)
		}
		return cnNoErr(ferr)
	})

	clip := render.Rect{X: 20, Y: 20, W: 30, H: 30}
	cnPaint(t, "image clipped", cnSize, 1, func(c *canvas) {
		c.images = set
		c.Save()
		c.ClipRoundedRect(clip, render.UniformRadii(6))
		c.PushOpacity(0.5)
		c.DrawImage(im, render.Rect{X: 10, Y: 10, W: 60, H: 60})
		c.Pop()
		c.Restore()
	}, func(root *cnNode, ferr error) error {
		n, err := cnOne("texture nodes", igTexNodes(root))
		if err != nil {
			return err
		}
		if err := igCheckNode(t, "clipped", n, cnRect(10, 10, 60, 60), igTexPtr(im), 2, 2); err != nil {
			return err
		}
		op := n.ancestor(gtk.NodeOpacity)
		if op == nil {
			return errors.New("the image inside PushOpacity(0.5) is under no opacity node")
		}
		if got := cnOpacityOf(op); !cnNear(got, 0.5) {
			return fmt.Errorf("the image's opacity node has opacity %g, want 0.5", got)
		}
		if n.clipper(cnRectF(clip)) == nil {
			return fmt.Errorf("the image is under no clip of %s", cnRectString(cnRectF(clip)))
		}
		if set.made != 1 {
			return fmt.Errorf("the set has made %d textures, want still 1", set.made)
		}
		return cnNoErr(ferr)
	})
}

// TestImageGTKOnceOverFrames: one image painted in three snapshots is one
// GdkTexture, made once and in every tree; after a release the next paint
// makes a new one.
func TestImageGTKOnceOverFrames(t *testing.T) {
	igNeed(t)
	set := &imageSet{}
	im, err := set.newImage(igPix, 2, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer igRelease(t, set)
	dst := render.Rect{X: 30, Y: 10, W: 16, H: 16}
	paint := func(frame string) uintptr {
		var tex uintptr
		cnPaint(t, "image "+frame, cnSize, 2, func(c *canvas) {
			c.images = set
			c.DrawImage(im, dst)
		}, func(root *cnNode, ferr error) error {
			n, err := cnOne("texture nodes", igTexNodes(root))
			if err != nil {
				return err
			}
			if err := igCheckNode(t, frame, n, cnRectF(dst), igTexPtr(im), 2, 2); err != nil {
				return err
			}
			tex = igTexOf(n)
			return cnNoErr(ferr)
		})
		return tex
	}
	var first uintptr
	for i := range 3 {
		tex := paint(fmt.Sprintf("frame %d", i))
		if i == 0 {
			first = tex
		} else if tex != first {
			t.Errorf("frame %d: the node's texture is %#x, want the first frame's %#x", i, tex, first)
		}
	}
	if set.made != 1 {
		t.Errorf("three frames made %d textures, want 1", set.made)
	}
	if len(set.held) != 1 {
		t.Errorf("the set holds %d images, want 1", len(set.held))
	}

	igRelease(t, set)
	if im.tex != nil || len(set.held) != 0 {
		t.Errorf("after release: texture %v, %d images held; want none", im.tex, len(set.held))
	}
	paint("after release")
	if set.made != 2 || im.tex == nil {
		t.Errorf("a paint after release: %d textures made (want 2), texture %v", set.made, im.tex)
	}
}

// igPainter is a surface handler that draws one image and records the
// image's texture after every paint.
type igPainter struct {
	*surfRec
	im   *shellImage
	texs []uintptr
}

var igSurfDst = render.Rect{X: 20, Y: 20, W: 40, H: 40}

func (h *igPainter) Paint(c render.Canvas) {
	h.surfRec.Paint(c)
	c.DrawImage(h.im, igSurfDst)
	h.texs = append(h.texs, igTexPtr(h.im))
}

// igRefCount is the GObject's ref_count (the guint after the GTypeInstance
// pointer); o must be a live GObject the caller holds a reference to.
func igRefCount(o uintptr) uint32 {
	return uint32(ffi.ReadInt32(o, unsafe.Sizeof(uintptr(0))))
}

// TestImageGTKSurfaceRelease: a real surface drawing an image of the
// shell makes its texture once over several frames; closing another
// surface keeps it, closing the last one releases it, and a new surface
// makes a new one, released when Run ends.
func TestImageGTKSurfaceRelease(t *testing.T) {
	needSurfaceGTK(t)
	var im *shellImage
	e := runSurfaceTest(t, func(e *surfTest) error {
		t := e.t
		var err error
		if im, err = e.s.newImage(igPix, 2, 2, 2); err != nil {
			return err
		}
		cfg := shell.SurfaceConfig{Title: "gogui image", Size: driver.Size{W: 120, H: 100}, Style: driver.StyleTitled}
		open := func(name string) (*surface, *gtkWindow, *igPainter, error) {
			h := &igPainter{surfRec: &surfRec{t: t, name: name, order: &e.order}, im: im}
			ss, err := e.s.NewSurface(cfg, h)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("NewSurface(%s): %w", name, err)
			}
			e.recs = append(e.recs, h.surfRec)
			sf := ss.(*surface)
			g, ok := sf.w.(*gtkWindow)
			if !ok {
				return nil, nil, nil, fmt.Errorf("NewSurface(%s): the window is a %T, not a *gtkWindow", name, sf.w)
			}
			return sf, g, h, nil
		}

		sf, g, h, err := open("imaged")
		if err != nil {
			return err
		}
		hidden, _, _, err := e.open("hidden", cfg)
		if err != nil {
			return err
		}
		if err := show(sf, g, h.surfRec); err != nil {
			return err
		}
		for i := range 3 {
			n := h.paints
			sf.Invalidate()
			if !iterateUntil(showWait, func() bool { return h.paints > n }) {
				t.Errorf("Invalidate %d: no paint within %v (%d paints)", i, showWait, h.paints)
			}
		}
		tex := igTexPtr(im)
		if e.s.images.made != 1 || tex == 0 {
			return fmt.Errorf("after %d paints: %d textures made, texture %#x; want 1, non-NULL", h.paints, e.s.images.made, tex)
		}
		for i, p := range h.texs {
			if p != tex {
				t.Errorf("paint %d drew texture %#x, want %#x", i, p, tex)
			}
		}
		if len(e.s.images.held) != 1 {
			t.Errorf("the shell holds %d images, want 1", len(e.s.images.held))
		}
		t.Logf("CANVAS-RESULT: image surface: after %d paints the texture's GObject ref_count is %d (the shell's reference included)",
			h.paints, igRefCount(tex))

		hidden.Close()
		if igTexPtr(im) != tex || len(e.s.images.held) != 1 {
			t.Errorf("closing another surface released the image: texture %v, %d held", im.tex, len(e.s.images.held))
		}
		sf.Close()
		if im.tex != nil || len(e.s.images.held) != 0 || im.failed {
			t.Errorf("after the last surface closed: texture %v, %d held, failed %v; want none", im.tex, len(e.s.images.held), im.failed)
		}

		sf2, g2, h2, err := open("second")
		if err != nil {
			return err
		}
		if err := show(sf2, g2, h2.surfRec); err != nil {
			return err
		}
		if e.s.images.made != 2 || im.tex == nil {
			t.Errorf("a new surface's paint: %d textures made (want 2), texture %v", e.s.images.made, im.tex)
		}
		return nil // Quit closes "second"
	})
	if im == nil {
		return
	}
	if im.tex != nil || len(e.s.images.held) != 0 {
		t.Errorf("after Run: texture %v, %d images held; want none", im.tex, len(e.s.images.held))
	}
}
