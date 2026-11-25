//go:build windows

package win32

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radix29/gogui/assets/fonts"
	"github.com/radix29/gogui/internal/mainthread"
	"github.com/radix29/gogui/internal/render"
)

// The embedded-font tests (S2.5g) replace the text system's font files
// through its fontFiles hook: none, a system TTF copied in, or a corrupt
// file. debugf records what the system logs.

// fontText returns a text system (released with the test, no references
// left) whose embedded fonts are files and whose log goes to *logged.
func fontText(t *testing.T, logged *[]string, files ...fonts.File) *textSystem {
	t.Helper()
	ts := newTestText(t)
	ts.fontFiles = func() []fonts.File { return files }
	ts.debugf = func(format string, args ...any) { *logged = append(*logged, fmt.Sprintf(format, args...)) }
	return ts
}

// winFont reads a font of the Windows fonts directory, as a font file of
// assets/fonts would be.
func winFont(t *testing.T, name string) fonts.File {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(os.Getenv("WINDIR"), "Fonts", name))
	if err != nil {
		t.Fatalf("reading the system font %s: %v", name, err)
	}
	return fonts.File{Name: name, Data: data}
}

// TestTextEmbeddedNone: without font files there is no embedded
// collection, the UI font is the system's, and that is logged once.
func TestTextEmbeddedNone(t *testing.T) {
	var logged []string
	ts := fontText(t, &logged)
	mustDW(t, ts)
	ts.NewLayout("Hello", render.Font{}, 0)
	if ts.fonts != nil || !ts.embedded.IsNil() {
		t.Error("an embedded collection without font files")
	}
	if k := ts.resolve(render.Font{}); k.family != ts.uiFamily || k.embedded {
		t.Errorf("UI font %+v, want %s from the system", k, ts.uiFamily)
	}
	if len(logged) != 1 || !strings.Contains(logged[0], "no embedded fonts") {
		t.Errorf("logged %q, want one line about no embedded fonts", logged)
	}
}

// TestTextEmbeddedUsed: a system TTF copied in becomes the embedded
// collection; its family resolves there, for a named family and (as Inter
// would) for the empty one, and lays out exactly as the system's copy.
func TestTextEmbeddedUsed(t *testing.T) {
	var logged []string
	ts := fontText(t, &logged, winFont(t, "georgia.ttf"), winFont(t, "georgiab.ttf"))
	ts.embeddedUI = "Georgia" // stands in for Inter
	mustDW(t, ts)
	if ts.fonts == nil || ts.embedded.IsNil() {
		t.Fatalf("no embedded collection; logged %q", logged)
	}
	if len(logged) != 0 {
		t.Errorf("logged %q for usable fonts", logged)
	}
	if n := ts.embedded.FamilyCount(); n != 1 {
		t.Errorf("embedded families %d, want 1", n)
	}
	for _, f := range []render.Font{{}, {Family: "Georgia"}, {Family: "georgia"}, {Family: "nonexistent family"}} {
		if k := ts.resolve(f); !k.embedded || !strings.EqualFold(k.family, "Georgia") {
			t.Errorf("resolve(%+v) = %+v, want Georgia from the embedded collection", f, k)
		}
	}
	// Other families and the monospaced font still come from the system.
	if k := ts.resolve(render.Font{Family: "Arial"}); k.embedded || k.family != "Arial" {
		t.Errorf("Arial: %+v", k)
	}
	if k := ts.resolve(render.Font{Monospaced: true}); k.embedded || k.family != familyMono {
		t.Errorf("monospaced: %+v", k)
	}

	const text = "Hello, World 0123"
	emb := ts.NewLayout(text, render.Font{}, 0)
	bold := ts.NewLayout(text, render.Font{Weight: 700}, 0)
	sysTS := newTestText(t)
	sys := sysTS.NewLayout(text, render.Font{Family: "Georgia"}, 0)
	ui := sysTS.NewLayout(text, render.Font{}, 0)
	if emb.Size() != sys.Size() || emb.Baseline() != sys.Baseline() {
		t.Errorf("embedded Georgia %v (baseline %v), system Georgia %v (%v)", emb.Size(), emb.Baseline(), sys.Size(), sys.Baseline())
	}
	if emb.Size() == ui.Size() {
		t.Errorf("embedded Georgia as wide as the system UI font (%v): not used", ui.Size())
	}
	if !(bold.Size().W > emb.Size().W) {
		t.Errorf("Georgia bold %v not wider than regular %v (the bold file is in the collection)", bold.Size(), emb.Size())
	}
	// A layout from the embedded collection rebuilds for drawing.
	dl, err := emb.(*layout).native()
	if err != nil {
		t.Fatal(err)
	}
	dl.Release()
}

// TestTextEmbeddedCorrupt: a corrupt file, alone or after a good one,
// drops the embedded collection: the UI font is the system's, and the
// reason is logged once, naming the files.
func TestTextEmbeddedCorrupt(t *testing.T) {
	good := winFont(t, "georgia.ttf")
	garbage := fonts.File{Name: "Broken.ttf", Data: []byte(strings.Repeat("not a font ", 100))}
	truncated := fonts.File{Name: "Short.ttf", Data: good.Data[:len(good.Data)/3]}
	for _, files := range [][]fonts.File{{garbage}, {good, garbage}, {truncated}, {{Name: "Empty.otf"}}} {
		var logged []string
		ts := fontText(t, &logged, files...)
		ts.embeddedUI = "Georgia"
		l := ts.NewLayout("Hello", render.Font{}, 0)
		if _, ok := l.(*layout); !ok {
			t.Errorf("%s: layout is %T, want DirectWrite's", files[len(files)-1].Name, l)
		}
		if ts.fonts != nil || !ts.embedded.IsNil() {
			t.Errorf("%s: embedded collection kept", files[len(files)-1].Name)
		}
		if k := ts.resolve(render.Font{}); k.embedded || k.family != ts.uiFamily {
			t.Errorf("%s: UI font %+v, want %s", files[len(files)-1].Name, k, ts.uiFamily)
		}
		ts.NewLayout("again", render.Font{}, 0)
		if len(logged) != 1 || !strings.Contains(logged[0], files[len(files)-1].Name) || !strings.Contains(logged[0], "not used") {
			t.Errorf("logged %q, want one line naming the files", logged)
		}
	}
}

// TestTextEmbeddedDefault: a shell's text system reads assets/fonts, looks
// for Inter and logs only with GOGUI_DEBUG=1; whatever the directory holds
// must load (with no font files, the fallback).
func TestTextEmbeddedDefault(t *testing.T) {
	ts := newTextSystem(mainthread.New())
	defer ts.release(func(what string, left uint32) { t.Errorf("%s: %d references left", what, left) })
	if ts.embeddedUI != familyEmbedded || (ts.debugf != nil) != debugChecks {
		t.Errorf("embeddedUI %q, debugf set %v with GOGUI_DEBUG %v", ts.embeddedUI, ts.debugf != nil, debugChecks)
	}
	var logged []string
	ts.debugf = func(format string, args ...any) { logged = append(logged, fmt.Sprintf(format, args...)) }
	mustDW(t, ts)
	files := fonts.Files()
	switch {
	case len(files) == 0 && (ts.fonts != nil || ts.resolve(render.Font{}).embedded):
		t.Error("an embedded font without font files")
	case len(files) > 0 && ts.fonts == nil:
		t.Errorf("the font files of assets/fonts don't load: %q", logged)
	case len(files) > 0 && ts.hasFamily(ts.embedded, familyEmbedded) && ts.resolve(render.Font{}).family != familyEmbedded:
		t.Error("Inter is embedded but not the UI font")
	}
}
