//go:build linux

package gtk4

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/radix29/gogui/assets/fonts"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// The app font over real Pango and fontconfig (S4.2j): Inter from
// assets/fonts is the font the empty family loads, through
// pango_font_map_add_font_file (Pango 1.56+) and through the fontconfig
// path (forced); without files, or with a file that is not a font, the
// UI family is the system's. Like the text tests these need gtk.Load, not
// a display; they run on the GTK goroutine with the cache in a temporary
// directory. What was loaded is logged with the prefix TEXT-RESULT.

// afPangoFns are the Pango calls only these tests make: the font a
// description loads and the description of a font (the caller frees it).
var afPangoFns struct {
	FontMapLoadFont          func(fm, ctx, desc uintptr) uintptr // PangoFont *, transfer full
	FontDescribe             func(font uintptr) uintptr          // PangoFontDescription *, transfer full
	FontDescriptionGetWeight func(desc uintptr) int32
}

var afBindOnce = sync.OnceValue(func() error {
	lib, err := ffi.Dlopen(gtk.PangoLib)
	if err != nil {
		return err
	}
	return errors.Join(
		lib.Bind(&afPangoFns.FontMapLoadFont, "pango_font_map_load_font"),
		lib.Bind(&afPangoFns.FontDescribe, "pango_font_describe"),
		lib.Bind(&afPangoFns.FontDescriptionGetWeight, "pango_font_description_get_weight"),
	)
})

// afEnv is what an afPango function gets: the text system, opened, the
// messages of its debugf and its cache directory.
type afEnv struct {
	t     *testing.T
	ts    *textSystem
	debug []string
	cache string
}

// pango is the text system's pangoNative.
func (e *afEnv) pango() *pangoNative {
	switch p := e.ts.pl.(type) {
	case *pangoNative:
		return p
	case tlNoSettings:
		return p.pangoLayouter.(*pangoNative)
	}
	e.t.Fatalf("the text system's layouter is a %T", e.ts.pl)
	return nil
}

// loaded returns the family and weight of the font Pango loads for f as
// the text system resolves it.
func (e *afEnv) loaded(f render.Font) (string, int32) {
	e.t.Helper()
	pf, p := e.ts.resolve(f), e.pango()
	d := gtk.Pango.FontDescriptionNew()
	defer gtk.Pango.FontDescriptionFree(d)
	gtk.Pango.FontDescriptionSetFamily(d, pf.family)
	gtk.Pango.FontDescriptionSetAbsoluteSize(d, pf.size)
	gtk.Pango.FontDescriptionSetWeight(d, pf.weight)
	font := afPangoFns.FontMapLoadFont(p.fm, p.ctx, d)
	if font == 0 {
		e.t.Errorf("pango_font_map_load_font(%+v) returned NULL", pf)
		return "", 0
	}
	defer gtk.GObject.ObjectUnref(font)
	got := afPangoFns.FontDescribe(font)
	defer gtk.Pango.FontDescriptionFree(got)
	return ffi.GoString(gtk.Pango.FontDescriptionGetFamily(got)), afPangoFns.FontDescriptionGetWeight(got)
}

// afPango runs f on the GTK goroutine with a fresh text system over Pango
// whose app font cache is a temporary directory; setup runs before it is
// opened. It skips without the GTK libraries, and fails the test when the
// text system logged a failure through logf.
func afPango(t *testing.T, setup func(ts *textSystem), f func(e *afEnv)) {
	t.Helper()
	if err := gtk.Load(); err != nil {
		t.Skipf("GTK 4 or Pango cannot be loaded: %v", err)
	}
	if err := afBindOnce(); err != nil {
		t.Fatalf("binding the test's Pango calls: %v", err)
	}
	e := &afEnv{t: t, cache: t.TempDir()}
	onGTK(t, func() error {
		var logs []string
		e.ts = tlSystem(&logs)
		e.ts.cacheDir = func() (string, error) { return e.cache, nil }
		e.ts.debugf = func(format string, args ...any) { e.debug = append(e.debug, fmt.Sprintf(format, args...)) }
		if setup != nil {
			setup(e.ts)
		}
		defer e.ts.release()
		if err := e.ts.init(); err != nil {
			return fmt.Errorf("opening Pango: %w", err)
		}
		f(e)
		if len(logs) > 0 {
			return fmt.Errorf("the text system logged %q", logs)
		}
		return nil
	})
}

// afNeedFonts skips the test when assets/fonts has no font files.
func afNeedFonts(t *testing.T) {
	t.Helper()
	if len(fonts.Files()) == 0 {
		t.Skip("assets/fonts has no font files")
	}
}

// afCheckInter checks that Inter is the UI family and what Pango loads for
// the empty family, regular and bold, and that the cache has the files.
func afCheckInter(e *afEnv) {
	t := e.t
	if e.ts.ui != familyApp || len(e.debug) != 0 {
		t.Errorf("UI family %q (debug %q), want %q", e.ts.ui, e.debug, familyApp)
	}
	if gtk.Pango.FontMapGetFamily(e.pango().fm, familyApp) == 0 {
		t.Errorf("pango_font_map_get_family(%q) is NULL", familyApp)
	}
	if got := e.ts.resolve(render.Font{}).family; got != familyApp {
		t.Errorf("the empty family resolves to %q, want %q", got, familyApp)
	}
	for _, w := range []driver.FontWeight{driver.FontWeightRegular, driver.FontWeightBold} {
		fam, weight := e.loaded(render.Font{Size: 13, Weight: w})
		t.Logf("TEXT-RESULT empty family, weight %d: Pango loads %q, weight %d", w, fam, weight)
		if fam != familyApp {
			t.Errorf("weight %d: Pango loads %q, want %q", w, fam, familyApp)
		}
	}
	files, _ := filepath.Glob(filepath.Join(e.cache, appFontDir, "*"))
	if len(files) != len(fonts.Files()) {
		t.Errorf("cache holds %q, want %d font files", files, len(fonts.Files()))
	}
	if l := e.ts.NewLayout("Hello", render.Font{}, 0); l.Size().W <= 0 {
		t.Errorf("Hello in Inter: %v", l.Size())
	}
}

// TestAppFontPangoInter: Inter is loaded the way this Pango does it
// (pango_font_map_add_font_file from 1.56), is the empty family's font in
// regular and bold, and a second text system (the files already added)
// has it too.
func TestAppFontPangoInter(t *testing.T) {
	afNeedFonts(t)
	afPango(t, nil, func(e *afEnv) {
		t.Logf("TEXT-RESULT Pango %d, pango_font_map_add_font_file bound: %v", gtk.PangoVersion(), gtk.Pango.FontMapAddFontFile != nil)
		afCheckInter(e)
	})
	afPango(t, nil, afCheckInter)
}

// TestAppFontPangoFontconfig: the fontconfig path (FcConfigAppFontAddFile
// and pango_fc_font_map_config_changed, forced by unbinding
// pango_font_map_add_font_file) loads Inter as well. The cache is a new
// directory, so the files are new to the font map; whether Inter was
// already there (an earlier test, or installed) is logged.
func TestAppFontPangoFontconfig(t *testing.T) {
	afNeedFonts(t)
	saved := gtk.Pango.FontMapAddFontFile
	gtk.Pango.FontMapAddFontFile = nil
	defer func() { gtk.Pango.FontMapAddFontFile = saved }()
	afPango(t, func(ts *textSystem) {
		// The map's state before the text system adds anything.
		fm := gtk.PangoCairo.FontMapGetDefault()
		t.Logf("TEXT-RESULT Inter in the font map before: %v", fm != 0 && gtk.Pango.FontMapGetFamily(fm, familyApp) != 0)
	}, afCheckInter)
}

// TestAppFontPangoFallback: without font files the UI family is the
// system's (said once by debugf); a file that is not a font, under a
// family nobody has, falls back the same way; text is laid out in both.
func TestAppFontPangoFallback(t *testing.T) {
	afPango(t, func(ts *textSystem) {
		ts.fontFiles = func() []fonts.File { return nil }
	}, func(e *afEnv) {
		t.Logf("TEXT-RESULT without font files: UI family %q, debug %q", e.ts.ui, e.debug)
		if e.ts.ui == familyApp || len(e.debug) != 1 {
			t.Errorf("UI family %q, debug %q; want the system's, one message", e.ts.ui, e.debug)
		}
		if l, ok := e.ts.NewLayout("Hello", render.Font{}, 0).(*textLayout); !ok || l.pfont.family != e.ts.ui {
			t.Errorf("Hello: %T in %+v, want a Pango layout in %q", l, l, e.ts.ui)
		}
	})
	const none = "GoguiNoSuchFamily"
	afPango(t, func(ts *textSystem) {
		ts.fontFiles = func() []fonts.File { return []fonts.File{{Name: "Broken.ttf", Data: []byte("not a font at all")}} }
		ts.appUI = none
	}, func(e *afEnv) {
		t.Logf("TEXT-RESULT a broken font file: UI family %q, debug %q", e.ts.ui, e.debug)
		if e.ts.ui == none || len(e.debug) != 1 {
			t.Errorf("UI family %q, debug %q; want the system's, one message", e.ts.ui, e.debug)
		}
		if _, err := os.Stat(e.cache); err != nil {
			t.Errorf("cache: %v", err)
		}
		if _, ok := e.ts.NewLayout("Hello", render.Font{}, 0).(*textLayout); !ok {
			t.Error("no Pango layout after the fallback")
		}
	})
}
