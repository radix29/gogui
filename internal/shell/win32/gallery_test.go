//go:build windows

package win32

import (
	"bytes"
	"flag"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua"
	"github.com/radix29/gogui/internal/aqua/gallery"
	"github.com/radix29/gogui/internal/aqua/gallery/golden"
	"github.com/radix29/gogui/internal/aqua/gallery/goldentest"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell/shellkit"
	"github.com/radix29/gogui/internal/shell/win32/d3d11"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// The S3.6c offscreen gallery render: the gallery scene (internal/aqua/
// gallery) in an aqua window on a hidden surface, on the WARP device, with
// a fixed appearance (Reduce Motion, legacy scrollers, the default accent,
// no high contrast), at 96 and 192 DPI, light and dark. Each frame is read
// back between EndDraw and Present (framePx) and un-premultiplied to
// straight-alpha RGBA. The golden test (S3.6d) compares these images with
// the committed PNGs.

// galleryVariants are the twelve golden renders: the four of a key window
// (goldentest.Variants) and, since S6.1h, the inactive and the
// increased-contrast ones (goldentest.PolishVariants).
var galleryVariants = slices.Concat(goldentest.Variants, goldentest.PolishVariants)

// dpi is the window DPI of variant v.
func dpi(v goldentest.Variant) uint32 { return uint32(96 * v.Scale) }

// gallerySys is the OS appearance of the golden renders, the same on every
// machine: light, the default accent, no high contrast, animations off
// (Reduce Motion), transparency on and "Always show scrollbars" on (legacy
// scrollers).
func gallerySys() *fakeSys {
	return &fakeSys{reg: map[string]uint32{
		regPersonalize + "|AppsUseLightTheme":   1,
		regPersonalize + "|EnableTransparency":  1,
		regAccessibility + "|DynamicScrollbars": 0,
	}, anim: false, animOK: true}
}

// galleryRenderer renders the gallery on the UI goroutine of a running
// aqua driver over a test shell.
type galleryRenderer struct {
	t   *testing.T
	s   *Shell
	d   *aqua.Driver
	win *aqua.Window
	sf  *surface
}

// galleryApp is the driver.AppHandler of withGalleryRenderer.
type galleryApp struct{ ready func() }

func (a galleryApp) Ready()                            { a.ready() }
func (galleryApp) AppearanceChanged(driver.Appearance) {}
func (galleryApp) QuitRequested() bool                 { return true }
func (galleryApp) MenuItemSelected(int)                {}

// withGalleryRenderer runs an aqua driver on a test shell whose devices
// are WARP and whose OS appearance is gallerySys, builds the gallery in a
// hidden, key window (gallery.Fill) and calls fn on the UI goroutine. Once
// Run returns it checks that the devices were released with no references
// left over.
func withGalleryRenderer(t *testing.T, fn func(r *galleryRenderer)) {
	t.Helper()
	s := newTestShell()
	s.sys = gallerySys()
	s.hooks.warp = true
	d := aqua.New(s)
	done := make(chan struct{})
	timer := time.AfterFunc(60*time.Second, func() {
		select {
		case <-done:
		default:
			buf := make([]byte, 1<<20)
			buf = buf[:runtime.Stack(buf, true)]
			s.Post(d.Quit)
			t.Errorf("Run did not return within 60 s; posted Quit. Goroutines:\n%s", buf)
		}
	})
	err := d.Run(driver.AppConfig{Name: "Gallery"}, galleryApp{ready: func() {
		defer d.Quit()
		w, err := d.NewWindow(driver.WindowConfig{Title: gallery.Title, Size: gallery.Size, Style: gallery.Style},
			winHandler{})
		if err != nil {
			t.Error(err)
			return
		}
		defer w.Close()
		gallery.Fill(d, w)
		win := w.(*aqua.Window)
		sf := win.Surface().(*surface)
		if w32.IsWindowVisible(sf.hwnd) {
			t.Error("the gallery window is visible")
		}
		// Key without showing or activating it: the focus ring and the
		// traffic lights draw as in the active window.
		w32.SendMessage(sf.hwnd, w32.WM_ACTIVATE, w32.WA_ACTIVE, 0)
		rethrow()
		if !win.IsKey() {
			t.Error("the gallery window is not key")
		}
		fn(&galleryRenderer{t: t, s: s, d: d, win: win, sf: sf})
	}})
	close(done)
	timer.Stop()
	if err != nil {
		t.Fatal(err)
	}
	if s.gpu != nil || len(s.leaks) != 0 {
		t.Errorf("after Run: GPU devices %v, leaks %v", s.gpu != nil, s.leaks)
	}
}

// render renders variant v: it sets the DPI (a synthetic WM_DPICHANGED),
// the OS's high-contrast theme (a WM_SETTINGCHANGE re-reads it), the
// window's activation (a synthetic WM_ACTIVATE, the window stays hidden)
// and the appearance, runs frames until no animation runs, paints the
// window and returns the frame as straight-alpha RGBA. The frame must draw
// on WARP, without rendering or recorder errors.
func (r *galleryRenderer) render(v goldentest.Variant) *image.NRGBA {
	t := r.t
	t.Helper()
	if r.sf.dpi != dpi(v) {
		setDPI(t, r.sf, dpi(v))
	}
	if r.s.sys.(*fakeSys).hc != v.Contrast {
		r.s.sys.(*fakeSys).hc = v.Contrast
		w32.SendMessage(r.s.listener, w32.WM_SETTINGCHANGE, 0, 0)
		rethrow()
	}
	if r.win.IsKey() == v.Inactive {
		state := uintptr(w32.WA_ACTIVE)
		if v.Inactive {
			state = w32.WA_INACTIVE
		}
		w32.SendMessage(r.sf.hwnd, w32.WM_ACTIVATE, state, 0)
		rethrow()
	}
	if r.win.IsKey() == v.Inactive {
		t.Fatalf("%s: the window is key %v", v.Name(), r.win.IsKey())
	}
	mode := driver.AppearanceLight
	if v.Dark {
		mode = driver.AppearanceDark
	}
	r.d.SetAppearance(mode)
	if a := r.win.Appearance(); a.Dark != v.Dark || !a.ReduceMotion || !a.LegacyScrollers ||
		a.Accent != driver.DefaultAccent || a.IncreaseContrast != v.Contrast {
		t.Fatalf("%s: appearance %+v", v.Name(), a)
	}
	r.settle(v)
	if r.s.gpu == nil || r.s.gpu.driver != d3d11.DRIVER_TYPE_WARP {
		t.Fatalf("%s: not on the WARP device", v.Name())
	}
	w, h, pix := framePx(t, r.sf)
	return goldentest.FromBGRA(w, h, 4*w, pix)
}

// settle runs frames on the shell's clock (the driver's timeline ticks in
// them) until no animation runs. Reduce Motion makes every animation jump
// to its end, so one frame is normally enough.
func (r *galleryRenderer) settle(v goldentest.Variant) {
	t := r.t
	t.Helper()
	tl := r.d.Timeline()
	for range 20 {
		if tl.Running() == 0 {
			return
		}
		r.s.clock.requested = true // the frame below runs even if nothing asked for one
		r.s.clock.frame()
		rethrow()
	}
	t.Fatalf("%s: %d animations still run after 20 frames", v.Name(), tl.Running())
}

// TestGalleryRender: each variant renders on WARP to a frame of the
// window's size in pixels (the content plus the title bar) with no render
// or recorder errors, and a second render gives identical pixels; no
// references are left after Run (withGalleryRenderer). With
// GOGUI_SCREENSHOTS set to a directory, every image is also written there
// as a PNG.
func TestGalleryRender(t *testing.T) {
	withGalleryRenderer(t, func(r *galleryRenderer) {
		for _, v := range galleryVariants {
			img := r.render(v)
			sz := img.Bounds().Size()
			ww, wh := int(shellkit.ToPx(gallery.Size.W, v.Scale)), int(shellkit.ToPx(gallery.Size.H+aqua.TitleBarHeight(), v.Scale))
			if sz.X != ww || sz.Y != wh {
				t.Errorf("%s: frame %d×%d px, want %d×%d", v.Name(), sz.X, sz.Y, ww, wh)
			}
			// The content background and the title bar are opaque.
			if a := img.NRGBAAt(sz.X/2, sz.Y-int(10*v.Scale)).A; a != 255 {
				t.Errorf("%s: content alpha %d", v.Name(), a)
			}
			if a := img.NRGBAAt(sz.X/2, int(4*v.Scale)).A; a != 255 {
				t.Errorf("%s: title bar alpha %d", v.Name(), a)
			}
			if again := r.render(v); !bytes.Equal(again.Pix, img.Pix) {
				res, _ := golden.Compare(img, again, golden.Options{})
				t.Errorf("%s: a second render differs: %v", v.Name(), res)
			}
			if dir := os.Getenv("GOGUI_SCREENSHOTS"); dir != "" {
				path := filepath.Join(dir, v.Name()+".png")
				if err := golden.WritePNG(path, img); err != nil {
					t.Error(err)
				} else {
					t.Logf("wrote %s", path)
				}
			}
		}
		// Back to light at 1× after the last variant (dark, 2×, Increase
		// Contrast; the loop above ran the inactive ones): the same pixels
		// as the first render (nothing carries over between variants).
		first := r.render(galleryVariants[0])
		last := galleryVariants[len(galleryVariants)-1]
		r.render(last)
		if again := r.render(galleryVariants[0]); !bytes.Equal(again.Pix, first.Pix) {
			res, _ := golden.Compare(first, again, golden.Options{})
			t.Errorf("%s after %s differs: %v", galleryVariants[0].Name(), last.Name(), res)
		}
	})
}

// updateGolden rewrites the gallery goldens (go test -run TestGalleryGolden
// ./internal/shell/win32 -args -update).
var updateGolden = flag.Bool("update", false, "rewrite the gallery goldens in internal/aqua/gallery/testdata/golden")

// TestGalleryGolden (S3.6d; the inactive and increased-contrast variants
// since S6.1h) renders every variant and compares each with its golden PNG
// with golden.DefaultOptions; -update rewrites the
// goldens instead. Every comparison is logged (native.sh runs this test
// verbosely, so the report lands in its log). On a failure the actual image
// and the diff are written to GOGUI_GOLDEN_OUT (or t.TempDir) as
// <name>.actual.png and <name>.diff.png, and the message names them.
func TestGalleryGolden(t *testing.T) {
	c := &goldentest.Check{Dir: goldentest.DirFromShell, Options: golden.DefaultOptions, Update: *updateGolden}
	withGalleryRenderer(t, func(r *galleryRenderer) {
		for _, v := range galleryVariants {
			c.Run(t, v, r.render(v))
		}
	})
}
