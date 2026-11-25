//go:build linux

package gtk4

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua"
	"github.com/radix29/gogui/internal/aqua/gallery"
	"github.com/radix29/gogui/internal/aqua/gallery/golden"
	"github.com/radix29/gogui/internal/aqua/gallery/goldentest"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
	"github.com/radix29/gogui/internal/shell/shellkit"
)

// The Linux gallery golden test (S4.3g): the first aqua driver on the gtk4
// shell in a test. The gallery scene (internal/aqua/gallery) in an aqua
// window on a shell over the golden renders' appearance source
// (gallerySource: light, the default accent, normal contrast, Reduce
// Motion, legacy scrollers), shown (GTK draws only mapped widgets), fixed
// as key and at each variant's scale through the render seams (S4.3f),
// painted until no animation runs and read back with the window's own GSK
// renderer. The images are compared with the goldens Windows generated
// (goldentest, S4.3e); this test never rewrites them and registers no
// -update flag.
//
// Text (Pango/GSK against DirectWrite) and anti-aliased edges differ from
// the Windows render, so the comparison runs under its own Options
// (goldenOptions) and leaves the text out (goldenMaskText): the boxes of
// every text the settled paint drew (the recorder's, surface.keepText),
// grown by goldenMaskMargin, are the comparison's Mask. Since S4.3i the
// check is strict (goldenStrict): a variant over goldenOptions fails, as
// do a frame of the wrong size, a second render of a variant that differs
// from the first (the render must be deterministic run to run), recorder
// errors of the canvas and a missing golden. Every comparison is logged
// as a GOLDEN-RESULT line with the backend and the renderer, followed by
// GOLDEN-SWEEP lines (goldentest.Check.Sweep: the result at each sweep
// threshold, with and without the mask), the material that calibrates
// goldenOptions (S4.3j). native.sh runs this test on each headless backend
// with GOGUI_GOLDEN_OUT=native-logs/golden-<backend>, where the actual
// image and the diff of every variant that fails land; without
// GOGUI_GOLDEN_OUT it skips, so a plain go test ./... on a desktop session
// (the user's fonts and scale) does not run it.

// goldenOptions is the Linux tolerance. Provisional until S4.3j sets it
// from a Linux log's GOLDEN-SWEEP lines: the Windows one
// (golden.DefaultOptions, no differing pixel outside the mask), so the
// first Linux run fails on every variant that differs and writes its
// actual image and diff.
var goldenOptions = golden.DefaultOptions

// goldenMaskText leaves the text boxes out of the comparison; goldenMaskMargin
// grows each box by that many points on every side (DirectWrite and Pango
// differ in advance widths and in how far glyphs reach out of the box).
// Both provisional until S4.3j.
const (
	goldenMaskText   = true
	goldenMaskMargin = 1.0
)

// goldenStrict turns a comparison over goldenOptions into a failure (on
// since S4.3i).
const goldenStrict = true

// galleryWait bounds every wait of a render: the first paint after Show,
// a variant's repaint, the animations settling.
const galleryWait = 5 * time.Second

// galleryWin is the gallery window's driver.WindowHandler: it does
// nothing.
type galleryWin struct{}

func (galleryWin) ContentResized(driver.Size) {}
func (galleryWin) ShouldClose() bool          { return true }
func (galleryWin) Closed()                    {}
func (galleryWin) KeyChanged(bool)            {}

// galleryApp is the driver.AppHandler of runGallery.
type galleryApp struct{ ready func() }

func (a galleryApp) Ready()                            { a.ready() }
func (galleryApp) AppearanceChanged(driver.Appearance) {}
func (galleryApp) QuitRequested() bool                 { return true }
func (galleryApp) MenuItemSelected(int)                {}

// galleryGTK renders the gallery on the GTK thread of a running aqua
// driver over a gtk4 shell.
type galleryGTK struct {
	t   *testing.T
	s   *Shell
	d   *aqua.Driver
	win *aqua.Window
	sf  *surface
	g   *gtkWindow

	backend  string            // the GDK display type
	renderer string            // the GSK renderer type of the last readback
	mask     []image.Rectangle // the text mask of the last render (goldenMaskText)
}

// runGallery runs an aqua driver on a fresh gtk4 shell over gallerySource
// with the canvas's recorder checks on, builds the gallery in a window
// (gallery.Fill), shows it, waits for its first paint, fixes it as key
// and calls fn on the GTK thread inside the running main loop; then it
// quits. fn's error fails the test. After Run it checks that the window
// closed and that everything is torn down.
func runGallery(t *testing.T, fn func(r *galleryGTK) error) {
	t.Helper()
	needSurfaceGTK(t)
	old := debugChecks
	debugChecks = true // sf.paintErr: the recorder errors of every paint
	t.Cleanup(func() { debugChecks = old })
	s, reports := newGTKShell(t)
	s.openSettings = gallerySource().open
	base := dispatch.Len()
	d := aqua.New(s)
	ran := false
	var w driver.Window
	scenario := func() error {
		var err error
		w, err = d.NewWindow(driver.WindowConfig{Title: gallery.Title, Size: gallery.Size, Style: gallery.Style}, galleryWin{})
		if err != nil {
			return fmt.Errorf("NewWindow: %w", err)
		}
		gallery.Fill(d, w)
		win := w.(*aqua.Window)
		sf, ok := win.Surface().(*surface)
		if !ok {
			return fmt.Errorf("the aqua window's surface is a %T, not a gtk4 *surface", win.Surface())
		}
		g, ok := sf.w.(*gtkWindow)
		if !ok {
			return fmt.Errorf("the window is a %T, not a *gtkWindow", sf.w)
		}
		sf.keepText = true // the text boxes of every paint: the mask
		if a := s.Appearance(); a != galleryAppearance() {
			return fmt.Errorf("Appearance() = %+v, want the golden renders' %+v", a, galleryAppearance())
		}
		w.Show()
		if !iterateUntil(galleryWait, func() bool { return sf.dead || (mapped(g.surf) && sf.paints > 0) }) || sf.dead {
			return fmt.Errorf("the gallery window was not mapped and painted within %v (closed %v)", galleryWait, sf.dead)
		}
		// Key without a window manager activating it: the focus ring and
		// the traffic lights draw as in the active window.
		sf.setFixedKey(keyOn)
		if !win.IsKey() {
			return errors.New("the gallery window is not key after setFixedKey(keyOn)")
		}
		r := &galleryGTK{t: t, s: s, d: d, win: win, sf: sf, g: g, renderer: "none"}
		if disp := gtk.Gtk.WidgetGetDisplay(g.win); disp != 0 {
			r.backend = typeName(typeOf(disp))
		}
		err = fn(r)
		sf.setFixedScale(0)
		sf.setFixedKey(keyNative)
		return err
	}
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
	var err error
	var panicked any
	onGTK(t, func() error {
		defer func() { panicked = recover() }()
		err = d.Run(driver.AppConfig{Name: "Gallery"}, galleryApp{ready: func() {
			s.Post(func() {
				defer d.Quit()
				ran = true
				if err := scenario(); err != nil {
					t.Error(err)
				}
			})
		}})
		return nil
	})
	close(done)
	timer.Stop()
	if panicked != nil || err != nil {
		t.Fatalf("Run = %v, panicked with %v", err, panicked)
	}
	if !ran {
		t.Error("the scenario did not run")
	}
	if w != nil && !w.(*aqua.Window).Closed() {
		t.Error("the gallery window did not close by the end of Run")
	}
	if len(s.surfaces) != 0 {
		t.Errorf("%d surfaces left open after Run", len(s.surfaces))
	}
	checkGTKTornDown(t, s, reports, base)
}

// render renders variant v: it fixes the scale, sets the appearance,
// invalidates the window (so every render is a paint of its own), lets
// GTK paint until the paint is done and no animation runs, and reads the
// frame back as straight-alpha RGBA. The frame must be the window's size
// in pixels (the content plus the title bar) and draw without recorder
// errors. r.mask becomes the text mask of that paint (none without
// goldenMaskText).
func (r *galleryGTK) render(v goldentest.Variant) (*image.NRGBA, error) {
	sf := r.sf
	sf.setFixedScale(v.Scale) // a no-op when it already is
	if sf.Scale() != v.Scale {
		return nil, fmt.Errorf("%s: Scale() = %v after setFixedScale", v.Name(), sf.Scale())
	}
	mode := driver.AppearanceLight
	if v.Dark {
		mode = driver.AppearanceDark
	}
	r.d.SetAppearance(mode)
	if a := r.win.Appearance(); a.Dark != v.Dark || !a.ReduceMotion || !a.LegacyScrollers ||
		a.Accent != driver.DefaultAccent || a.IncreaseContrast {
		return nil, fmt.Errorf("%s: appearance %+v", v.Name(), a)
	}
	sf.Invalidate()
	paints := sf.paints
	tl := r.d.Timeline()
	if !iterateUntil(galleryWait, func() bool {
		return sf.dead || (sf.paints > paints && !sf.invalid && tl.Running() == 0)
	}) || sf.dead {
		return nil, fmt.Errorf("%s: no settled paint within %v (closed %v, invalid %v, %d animations running)",
			v.Name(), galleryWait, sf.dead, sf.invalid, tl.Running())
	}
	if sf.paintErr != nil {
		return nil, fmt.Errorf("%s: the canvas recorded errors: %w", v.Name(), sf.paintErr)
	}
	f, err := sf.readback()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", v.Name(), err)
	}
	r.renderer = f.renderer
	img := goldentest.FromBGRA(f.w, f.h, f.stride, f.pix)
	ww, wh := int(shellkit.ToPx(gallery.Size.W, v.Scale)), int(shellkit.ToPx(gallery.Size.H+aqua.TitleBarHeight(), v.Scale))
	if sz := img.Bounds().Size(); sz.X != ww || sz.Y != wh {
		return nil, fmt.Errorf("%s: frame %d×%d px, want %d×%d", v.Name(), sz.X, sz.Y, ww, wh)
	}
	r.mask = nil
	if goldenMaskText {
		if len(sf.textBoxes) == 0 {
			return nil, fmt.Errorf("%s: the paint kept no text boxes (the gallery draws text)", v.Name())
		}
		r.mask = textMask(sf.textBoxes, v.Scale, goldenMaskMargin, ww, wh)
	}
	return img, nil
}

// TestGalleryGolden renders the four variants on the gtk4 shell, each
// twice (the two must be identical), and compares each with its golden
// PNG under goldenOptions with the text masked, logging every Result
// (GOLDEN-RESULT) and the sweep (GOLDEN-SWEEP) and failing, with
// goldenStrict, on a variant over the tolerance. Light at 1× is rendered
// once more after dark at 2× and must match its first render (nothing
// carries over between variants). It runs only with GOGUI_GOLDEN_OUT set
// (native.sh's golden steps on the headless backends).
func TestGalleryGolden(t *testing.T) {
	if os.Getenv("GOGUI_GOLDEN_OUT") == "" {
		t.Skip("GOGUI_GOLDEN_OUT not set: the Linux goldens run only from native.sh's golden steps (headless backends), not on a desktop session")
	}
	c := &goldentest.Check{Dir: goldentest.DirFromShell, Options: goldenOptions, LogOnly: !goldenStrict}
	gdkBackend := os.Getenv("GDK_BACKEND")
	runGallery(t, func(r *galleryGTK) error {
		var first *image.NRGBA
		for i, v := range goldentest.Variants {
			img, err := r.render(v)
			if err != nil {
				return err
			}
			again, err := r.render(v)
			if err != nil {
				return err
			}
			if !bytes.Equal(again.Pix, img.Pix) {
				res, _ := golden.Compare(img, again, golden.Options{})
				t.Errorf("%s: a second render differs: %v", v.Name(), res)
			}
			if i == 0 {
				first = img
			}
			c.Options.Mask = r.mask
			res := c.Run(t, v, img)
			t.Logf("GOLDEN-RESULT: %s on %s (GDK_BACKEND=%q), renderer %s, strict %v, mask %d rects (margin %g pt): %v",
				v.Name(), r.backend, gdkBackend, r.renderer, goldenStrict, len(r.mask), goldenMaskMargin, res)
			lines, err := c.Sweep(v, img)
			if err != nil {
				return fmt.Errorf("%s: sweep: %w", v.Name(), err)
			}
			for _, l := range lines {
				t.Logf("GOLDEN-SWEEP: %s on %s: %s", v.Name(), r.backend, l)
			}
		}
		v0, v3 := goldentest.Variants[0], goldentest.Variants[len(goldentest.Variants)-1]
		again, err := r.render(v0)
		if err != nil {
			return err
		}
		if !bytes.Equal(again.Pix, first.Pix) {
			res, _ := golden.Compare(first, again, golden.Options{})
			t.Errorf("%s after %s differs: %v", v0.Name(), v3.Name(), res)
		}
		return nil
	})
}
