package gtk4

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/assets/fonts"
	"github.com/radix29/gogui/internal/render"
)

// The app font host tests (S4.2j): the cache files (written under their
// content hash, reused, a corrupt file replaced, an unwritable cache
// failing) and the text system's use of them through the hooks
// (fontFiles, cacheDir, appUI, debugf) and a fake pangoLayouter: Inter is
// the UI family once added and found, any failure is the system UI
// family, said once by debugf.

// afFiles are two small stand-in font files (their bytes don't matter to
// the cache).
var afFiles = []fonts.File{
	{Name: "Inter-Regular.ttf", Data: []byte("regular font bytes")},
	{Name: "Inter-Bold.TTF", Data: []byte("bold font bytes, longer")},
}

// afName is the cache file name of f.
func afName(f fonts.File) string {
	sum := sha256.Sum256(f.Data)
	return hex.EncodeToString(sum[:]) + strings.ToLower(filepath.Ext(f.Name))
}

// afDir returns the files of dir by name, temporary files included.
func afDir(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range es {
		names = append(names, e.Name())
	}
	return names
}

// TestAppFontCacheWrites: the files are written into a new directory under
// their hash with the extension in lower case, hold the embedded bytes,
// and nothing else is left there.
func TestAppFontCacheWrites(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "gogui", "fonts")
	paths, err := cacheFonts(dir, afFiles)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, afName(afFiles[0])), filepath.Join(dir, afName(afFiles[1]))}
	if !slices.Equal(paths, want) {
		t.Errorf("paths %q, want %q", paths, want)
	}
	if !strings.HasSuffix(paths[1], ".ttf") {
		t.Errorf("%s: the extension is not lower case", paths[1])
	}
	for i, p := range paths {
		got, err := os.ReadFile(p)
		if err != nil || !bytes.Equal(got, afFiles[i].Data) {
			t.Errorf("%s holds %q (%v), want %q", p, got, err, afFiles[i].Data)
		}
	}
	names := afDir(t, dir)
	slices.Sort(names)
	wantNames := []string{afName(afFiles[0]), afName(afFiles[1])}
	slices.Sort(wantNames)
	if !slices.Equal(names, wantNames) {
		t.Errorf("cache holds %q, want only %q", names, wantNames)
	}
}

// TestAppFontCacheReused: a file already in the cache with the embedded
// content is not written again (its modification time stays).
func TestAppFontCacheReused(t *testing.T) {
	dir := t.TempDir()
	paths, err := cacheFonts(dir, afFiles)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	for _, p := range paths {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	again, err := cacheFonts(dir, afFiles)
	if err != nil || !slices.Equal(again, paths) {
		t.Fatalf("second cacheFonts: %q, %v; want %q", again, err, paths)
	}
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil || !st.ModTime().Equal(old) {
			t.Errorf("%s was written again (mtime %v, %v)", p, st.ModTime(), err)
		}
	}
	if n := len(afDir(t, dir)); n != len(afFiles) {
		t.Errorf("cache holds %d files, want %d", n, len(afFiles))
	}
}

// TestAppFontCacheCorrupt: a cache file with other bytes (torn, of the
// same size, empty, longer) is replaced by the embedded content.
func TestAppFontCacheCorrupt(t *testing.T) {
	f := afFiles[0]
	for name, bad := range map[string][]byte{
		"truncated": f.Data[:5],
		"same size": bytes.Repeat([]byte{'x'}, len(f.Data)),
		"empty":     {},
		"longer":    append(slices.Clone(f.Data), "trailing"...),
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, afName(f))
			if err := os.WriteFile(p, bad, 0o600); err != nil {
				t.Fatal(err)
			}
			paths, err := cacheFonts(dir, []fonts.File{f})
			if err != nil || len(paths) != 1 || paths[0] != p {
				t.Fatalf("cacheFonts: %q, %v; want [%s]", paths, err, p)
			}
			if got, err := os.ReadFile(p); err != nil || !bytes.Equal(got, f.Data) {
				t.Errorf("after cacheFonts %s holds %q (%v), want %q", p, got, err, f.Data)
			}
			if n := len(afDir(t, dir)); n != 1 {
				t.Errorf("cache holds %d files, want 1", n)
			}
		})
	}
}

// TestAppFontCacheUnwritable: a cache that can't be created (its parent is
// a file) or a cache file that can't be replaced (a directory holds its
// name) fails, without leaving a temporary file.
func TestAppFontCacheUnwritable(t *testing.T) {
	t.Run("parent is a file", func(t *testing.T) {
		base := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(base, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if paths, err := cacheFonts(filepath.Join(base, "fonts"), afFiles); err == nil {
			t.Errorf("cacheFonts under a file: %q, want an error", paths)
		}
	})
	t.Run("directory in the way", func(t *testing.T) {
		dir := t.TempDir()
		blocker := filepath.Join(dir, afName(afFiles[1]))
		if err := os.MkdirAll(filepath.Join(blocker, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
		paths, err := cacheFonts(dir, afFiles)
		if err == nil {
			t.Fatalf("cacheFonts with a directory at %s: %q, want an error", blocker, paths)
		}
		if !strings.Contains(err.Error(), afFiles[1].Name) {
			t.Errorf("error %q does not name %s", err, afFiles[1].Name)
		}
		for _, n := range afDir(t, dir) {
			if strings.HasSuffix(n, ".tmp") {
				t.Errorf("temporary file %s left behind", n)
			}
		}
	})
}

// afSystem returns a text system over pl with the app font files and a
// cache under a temporary directory, and the messages of its debugf.
func afSystem(t *testing.T, pl *txLayouter, files []fonts.File) (*textSystem, *txLogs, string) {
	t.Helper()
	ts, _ := txSystem(pl, nil, nil)
	base := t.TempDir()
	ts.fontFiles = func() []fonts.File { return files }
	ts.cacheDir = func() (string, error) { return base, nil }
	dbg := &txLogs{}
	ts.debugf = dbg.logf
	return ts, dbg, base
}

// TestAppFontUsed: with the files added and Inter in the font map, the
// empty family is Inter (Monospaced is still monospace, a named family
// itself); the files were added from the cache, once, before the first
// measure, and nothing is logged.
func TestAppFontUsed(t *testing.T) {
	pl := &txLayouter{ui: "Cantarell", has: map[string]bool{"DejaVu Serif": true}, adds: []string{familyApp},
		m: pangoMetrics{width: 1024, height: 1024}}
	ts, dbg, base := afSystem(t, pl, afFiles)
	if _, ok := ts.NewLayout("Hello", render.Font{}, 0).(*textLayout); !ok {
		t.Fatal("no Pango layout")
	}
	ts.NewLayout("again", render.Font{}, 0)
	if len(pl.added) != 1 || pl.measuredBeforeAdd != 0 {
		t.Fatalf("addFonts calls %q, %d measures before them; want one call before any measure", pl.added, pl.measuredBeforeAdd)
	}
	dir := filepath.Join(base, "gogui", "fonts")
	want := []string{filepath.Join(dir, afName(afFiles[0])), filepath.Join(dir, afName(afFiles[1]))}
	if !slices.Equal(pl.added[0], want) {
		t.Errorf("added %q, want %q", pl.added[0], want)
	}
	for _, p := range want {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("cache file: %v", err)
		}
	}
	for _, tc := range []struct {
		font render.Font
		want string
	}{
		{render.Font{}, familyApp},
		{render.Font{Weight: 700, Italic: true}, familyApp},
		{render.Font{Monospaced: true}, familyMono},
		{render.Font{Family: "DejaVu Serif"}, "DejaVu Serif"},
		{render.Font{Family: "Helvetica Neue"}, familyApp},
		{render.Font{Family: familyApp}, familyApp},
	} {
		if got := ts.resolve(tc.font).family; got != tc.want {
			t.Errorf("%+v resolves to %q, want %q", tc.font, got, tc.want)
		}
	}
	if pl.measures[0].font.family != familyApp {
		t.Errorf("first layout measured in %q, want %q", pl.measures[0].font.family, familyApp)
	}
	if len(dbg.msgs) != 0 {
		t.Errorf("debugf said %q, want nothing", dbg.msgs)
	}
}

// TestAppFontFallback: every failure means the system UI family
// (gtk-font-name, else sans-serif), said once by debugf with its reason;
// the text system works on (layouts are Pango's, logf says nothing).
func TestAppFontFallback(t *testing.T) {
	errAdd := errors.New("fontconfig refused the file")
	for _, tc := range []struct {
		name   string
		files  []fonts.File
		setup  func(ts *textSystem, pl *txLayouter, base string)
		ui     string // gtk-font-name's family
		want   string
		reason string // in the debug message
		adds   bool   // addFonts is called
	}{
		{name: "no font files", want: "Cantarell", ui: "Cantarell", reason: "no font files"},
		{name: "no font files, no gtk-font-name", want: familyUIFallback, reason: "no font files"},
		{name: "no cache directory", files: afFiles, ui: "Cantarell", want: "Cantarell", reason: "no cache directory",
			setup: func(ts *textSystem, _ *txLayouter, _ string) {
				ts.cacheDir = func() (string, error) { return "", errors.New("$XDG_CACHE_HOME and $HOME are unset") }
			}},
		{name: "empty cache directory", files: afFiles, ui: "Cantarell", want: "Cantarell", reason: "no cache directory",
			setup: func(ts *textSystem, _ *txLayouter, _ string) {
				ts.cacheDir = func() (string, error) { return "", nil }
			}},
		{name: "unwritable cache", files: afFiles, ui: "Cantarell", want: "Cantarell", reason: "Inter-Regular.ttf",
			setup: func(ts *textSystem, _ *txLayouter, base string) {
				// A directory where the first cache file goes.
				if err := os.MkdirAll(filepath.Join(base, "gogui", "fonts", afName(afFiles[0]), "x"), 0o755); err != nil {
					panic(err)
				}
			}},
		{name: "cache under a file", files: afFiles, ui: "Cantarell", want: "Cantarell", reason: "gogui",
			setup: func(ts *textSystem, _ *txLayouter, base string) {
				if err := os.WriteFile(filepath.Join(base, "gogui"), []byte("x"), 0o600); err != nil {
					panic(err)
				}
			}},
		{name: "addFonts fails", files: afFiles, ui: "Cantarell", want: "Cantarell", reason: errAdd.Error(), adds: true,
			setup: func(_ *textSystem, pl *txLayouter, _ string) { pl.addErr = errAdd }},
		{name: "no Inter after adding", files: afFiles, want: familyUIFallback, reason: `no family "Inter"`, adds: true,
			setup: func(_ *textSystem, pl *txLayouter, _ string) { pl.adds = []string{"Inter Display"} }},
		{name: "another app family", files: afFiles, ui: "Cantarell", want: "Cantarell", reason: `no family "GoguiNone"`, adds: true,
			setup: func(ts *textSystem, _ *txLayouter, _ string) { ts.appUI = "GoguiNone" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pl := &txLayouter{ui: tc.ui, adds: []string{familyApp}, m: pangoMetrics{width: 1024, height: 1024}}
			ts, dbg, base := afSystem(t, pl, tc.files)
			lg := &txLogs{}
			ts.logf = lg.logf
			if tc.setup != nil {
				tc.setup(ts, pl, base)
			}
			for range 2 {
				if _, ok := ts.NewLayout("Hello", render.Font{}, 0).(*textLayout); !ok {
					t.Fatal("no Pango layout")
				}
			}
			if got := ts.resolve(render.Font{}).family; got != tc.want {
				t.Errorf("empty family resolves to %q, want %q", got, tc.want)
			}
			if len(dbg.msgs) != 1 || !strings.Contains(dbg.msgs[0], tc.reason) || !strings.Contains(dbg.msgs[0], tc.want) {
				t.Errorf("debugf said %q, want once, with %q and %q", dbg.msgs, tc.reason, tc.want)
			}
			if (len(pl.added) == 1) != tc.adds || len(pl.added) > 1 {
				t.Errorf("addFonts called %d times, want called: %v (once)", len(pl.added), tc.adds)
			}
			if len(lg.msgs) != 0 {
				t.Errorf("logf said %q, want nothing", lg.msgs)
			}
		})
	}
}

// TestAppFontNoDebug: without debugf (GOGUI_DEBUG unset) a failure is
// silent, and the default text system's hooks are the real ones.
func TestAppFontNoDebug(t *testing.T) {
	pl := &txLayouter{ui: "Cantarell", m: pangoMetrics{width: 1024, height: 1024}}
	ts, _, _ := afSystem(t, pl, nil)
	ts.debugf = nil
	if err := ts.init(); err != nil {
		t.Fatal(err)
	}
	if ts.ui != "Cantarell" {
		t.Errorf("UI family %q, want Cantarell", ts.ui)
	}

	def := newTextSystem(nil)
	if def.appUI != familyApp || def.fontFiles == nil || def.cacheDir == nil {
		t.Errorf("newTextSystem: appUI %q, fontFiles set %v, cacheDir set %v; want %q, true, true",
			def.appUI, def.fontFiles != nil, def.cacheDir != nil, familyApp)
	}
	if (def.debugf != nil) != debugChecks {
		t.Errorf("debugf set: %v, want %v (GOGUI_DEBUG=1)", def.debugf != nil, debugChecks)
	}
	if got, want := len(def.fontFiles()), len(fonts.Files()); got != want {
		t.Errorf("fontFiles gives %d files, want fonts.Files' %d", got, want)
	}
}

// TestAppFontCacheSweep: cacheFonts removes temporary files older than
// staleTemp and keeps fresh ones (a writer that is still at work) and the
// font files.
func TestAppFontCacheSweep(t *testing.T) {
	dir := t.TempDir()
	if _, err := cacheFonts(dir, afFiles); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, afName(afFiles[0])+".123.tmp")
	fresh := filepath.Join(dir, afName(afFiles[0])+".456.tmp")
	for _, p := range []string{stale, fresh} {
		if err := os.WriteFile(p, []byte("half a font"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * staleTemp)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	// A font file older than staleTemp stays.
	if err := os.Chtimes(filepath.Join(dir, afName(afFiles[1])), old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := cacheFonts(dir, afFiles); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stale %s: %v, want removed", stale, err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("fresh %s: %v, want kept", fresh, err)
	}
	if n := len(afDir(t, dir)); n != len(afFiles)+1 {
		t.Errorf("cache holds %d files, want the %d fonts and the fresh temporary file", n, len(afFiles))
	}
}
