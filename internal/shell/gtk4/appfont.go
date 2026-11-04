package gtk4

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/radix29/gogui/assets/fonts"
)

// The app font (PLAN.md S4.2j): Inter, the .ttf/.otf files of
// assets/fonts, as the UI font of the empty family.
//
// Fontconfig loads fonts only from files, so the embedded files are
// written once to the user's cache, os.UserCacheDir()/gogui/fonts, each
// under the SHA-256 of its content (with its extension, lower case). A
// file already there is reused when its content is the embedded one; any
// other (a torn write, a corrupt or foreign file) is replaced. A file is
// written to a temporary name in the same directory and renamed into
// place, so another process never sees half a font, and two writing at
// once leave the same bytes (a temporary file a dead process left is
// removed after staleTemp). textSystem.init then adds the files to the
// font map (pangoLayouter.addFonts) before the first layout, and checks
// that the map has the family "Inter".
//
// Any failure (no font files, no cache directory, a file that can't be
// written, Pango or fontconfig refusing a file, no Inter afterwards)
// means the system UI font: the family of gtk-font-name, else sans-serif.
// The reason is logged once with GOGUI_DEBUG=1.

// familyApp is the family the empty family resolves to when the app font
// is loaded.
const familyApp = "Inter"

// appFontDir is the cache directory of the app font, under
// os.UserCacheDir.
var appFontDir = filepath.Join("gogui", "fonts")

// cacheFonts writes files into dir (created if needed) and returns their
// paths, in the order of files, after removing stale temporary files. It
// stops at the first failure.
func cacheFonts(dir string, files []fonts.File) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	sweepTemp(dir, time.Now())
	paths := make([]string, 0, len(files))
	for _, f := range files {
		p, err := cacheFont(dir, f)
		if err != nil {
			return nil, fmt.Errorf("caching %s: %w", f.Name, err)
		}
		paths = append(paths, p)
	}
	return paths, nil
}

// cacheFont returns the path of f in dir, writing it unless a file with
// its content is already there.
func cacheFont(dir string, f fonts.File) (string, error) {
	sum := sha256.Sum256(f.Data)
	name := hex.EncodeToString(sum[:]) + strings.ToLower(path.Ext(f.Name))
	p := filepath.Join(dir, name)
	if cached(p, f.Data) {
		return p, nil
	}
	tmp, err := os.CreateTemp(dir, name+".*.tmp")
	if err != nil {
		return "", err
	}
	_, werr := tmp.Write(f.Data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	return p, nil
}

// cached reports whether p is a regular file holding exactly data. The
// size is compared first, so a large foreign file is not read.
func cached(p string, data []byte) bool {
	st, err := os.Stat(p)
	if err != nil || !st.Mode().IsRegular() || st.Size() != int64(len(data)) {
		return false
	}
	got, err := os.ReadFile(p)
	return err == nil && bytes.Equal(got, data)
}

// loadAppFont caches the app font's files and adds them to the font map
// (see the top of this file). It reports whether the map has familyApp
// afterwards; on false, debugf (when set) has said why. It runs in init,
// once per text system, before the first layout.
func (ts *textSystem) loadAppFont() bool {
	err := ts.addAppFont()
	if err != nil && ts.debugf != nil {
		ts.debugf("gtk4: text: app font %s not used, using %s: %v", ts.appUI, ts.ui, err)
	}
	return err == nil
}

func (ts *textSystem) addAppFont() error {
	files := ts.fontFiles()
	if len(files) == 0 {
		return errors.New("no font files in assets/fonts")
	}
	base, err := ts.cacheDir()
	if err != nil {
		return fmt.Errorf("no cache directory: %w", err)
	}
	if base == "" {
		return errors.New("no cache directory")
	}
	paths, err := cacheFonts(filepath.Join(base, appFontDir), files)
	if err != nil {
		return err
	}
	if err := ts.pl.addFonts(paths); err != nil {
		return err
	}
	if !ts.pl.hasFamily(ts.appUI) {
		return fmt.Errorf("the font map has no family %q after adding %d files", ts.appUI, len(paths))
	}
	return nil
}

// staleTemp is how old a temporary file in the cache must be before
// cacheFonts removes it: one left by a process that died between writing
// and renaming it (a live writer renames its file within milliseconds).
const staleTemp = time.Hour

// sweepTemp removes the stale temporary files (*.tmp) of dir, ignoring
// errors: they only take space.
func sweepTemp(dir string, now time.Time) {
	tmps, _ := filepath.Glob(filepath.Join(dir, "*.tmp"))
	for _, p := range tmps {
		if st, err := os.Lstat(p); err == nil && st.Mode().IsRegular() && now.Sub(st.ModTime()) > staleTemp {
			_ = os.Remove(p)
		}
	}
}
