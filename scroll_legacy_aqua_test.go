package gogui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua"
	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/driver"
	fakeshell "github.com/radix29/gogui/internal/shell/fake"
)

// The notes sidebar with legacy scrollers ("Always show scrollbars") on
// the aqua driver through the fake shell (PLAN.md S3.5f): the vertical
// scroller takes LegacyScrollerThickness from the viewport, and the
// document, laid out at the viewport's width, follows it, from the start
// and after a runtime switch.

// runAquaSys is runAqua with the fake shell's OS appearance set to sys
// before the driver starts.
func runAquaSys(t *testing.T, sys driver.Appearance, body func(app *App, sh *fakeshell.Shell)) {
	t.Helper()
	sh := fakeshell.New()
	sh.SetAppearance(sys)
	d := aqua.New(sh)
	setDriver(t, d)
	err := runTest(t, func(app *App) {
		body(app, sh)
		for d.Queue().Drain() > 0 {
		}
		app.Quit()
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// findScrollView returns the first scroll view under w, depth first.
func findScrollView(w Widget) *ScrollView {
	if s, ok := w.(*ScrollView); ok {
		return s
	}
	if p, ok := w.(parentWidget); ok {
		for _, c := range p.childWidgets() {
			if s := findScrollView(c); s != nil {
				return s
			}
		}
	}
	return nil
}

// wantLegacyNotes checks the notes sidebar's viewport, document and range
// for the viewport width vpW (the sidebar is 180 × 451).
func wantLegacyNotes(t *testing.T, what string, sidebar *ScrollView, vpW float64) {
	t.Helper()
	sv := sidebar.peerWidget().(*aqua.ScrollView)
	if got := frameOf(sidebar); got != (Rect{W: 180, H: 451}) {
		t.Errorf("%s: sidebar frame %v, want 180 × 451", what, got)
	}
	if got := sv.ViewportSize(); got != (driver.Size{W: vpW, H: 451}) {
		t.Errorf("%s: viewport %v, want %g × 451", what, got, vpW)
	}
	doc := frameOf(sidebar.Document())
	if doc.W != vpW {
		t.Errorf("%s: document frame %v, want %g wide (the viewport's width)", what, doc, vpW)
	}
	ds := sv.DocumentSize
	if ds.W != vpW || ds.H != doc.H {
		t.Errorf("%s: document size %v, document frame %v", what, ds, doc)
	}
	if got, want := sv.MaxOffset(), (driver.Point{X: max(0, ds.W-vpW), Y: max(0, ds.H-451)}); got != want {
		t.Errorf("%s: max offset %v, want %v", what, got, want)
	}
}

// TestNotesLegacyScrollersOnAqua opens the notes example with legacy
// scrollers from the start: the sidebar's viewport is 15 pt narrower than
// its frame, the layout laid the document out at that width, and the
// window's frames are pinned in testdata/aqua/notes_legacy.frames
// (rewrite with -run Legacy -update).
func TestNotesLegacyScrollersOnAqua(t *testing.T) {
	sys := driver.DefaultAppearance()
	sys.LegacyScrollers = true
	lt := theme.Sequoia.Scroll.LegacyScrollerThickness
	if lt != 15 {
		t.Fatalf("LegacyScrollerThickness %g, the test expects 15", lt)
	}
	var got string
	runAquaSys(t, sys, func(app *App, sh *fakeshell.Shell) {
		win := exampleNotes(t, app)
		sidebar := findScrollView(win.content)
		if sidebar == nil {
			t.Fatal("no sidebar")
		}
		sv := sidebar.peerWidget().(*aqua.ScrollView)
		if !sv.Legacy() {
			t.Fatal("the sidebar does not show legacy scrollers")
		}
		wantLegacyNotes(t, "legacy", sidebar, 180-lt)
		if m := sv.MaxOffset().Y; m <= 0 {
			t.Errorf("max offset %g: the sidebar cannot scroll", m)
		}
		got = frameDump(win)
		sf := paintWindow(t, sh, win)
		// The track and the knob of the sidebar's scroller (the window is
		// centred: the dump is in surface coordinates, where the sidebar
		// starts at (0, 28)).
		dump := sf.Dump()
		for _, l := range []string{"fillRRect (165,28,15,451) solid #fafafa", "fillRRect (168,30,9,"} {
			if !strings.Contains(dump, l) {
				t.Errorf("no %q in the paint:\n%s", l, dump)
			}
		}
	})
	path := filepath.Join("testdata", "aqua", "notes_legacy.frames")
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to pin the dump)", err)
	}
	if w := strings.ReplaceAll(string(want), "\r\n", "\n"); got != w {
		t.Errorf("aqua frames differ from %s (rerun with -update if the change is intended):\n%s", path, lineDiff(w, got))
	}
}

// TestNotesLegacySwitchOnAqua switches "Always show scrollbars" at
// runtime: the sidebar's viewport changes at once, the document follows
// after the relayout the switch posts, and switching back restores both.
func TestNotesLegacySwitchOnAqua(t *testing.T) {
	runNotes(t, func(e *notesEnv) {
		lt := theme.Sequoia.Scroll.LegacyScrollerThickness
		wantLegacyNotes(t, "overlay", e.sidebar, 180)
		e.sidebar.SetOffset(Point{Y: notesMax})
		drain(e.app)
		e.scrolls = nil

		sys := e.sh.Appearance()
		sys.LegacyScrollers = true
		e.sh.SetAppearance(sys)
		if !e.sv.Legacy() {
			t.Fatal("not legacy after the switch")
		}
		if got := e.sv.ViewportSize(); got != (driver.Size{W: 180 - lt, H: 451}) {
			t.Errorf("on: viewport %v right after the switch", got)
		}
		if w := frameOf(e.sidebar.Document()).W; w != 180 {
			t.Errorf("on: document %g wide before the relayout ran", w)
		}
		drain(e.app)
		wantLegacyNotes(t, "on", e.sidebar, 180-lt)
		if got := e.sidebar.Offset(); got != (Point{Y: notesMax}) {
			t.Errorf("on: offset %v, want (0, %d)", got, notesMax)
		}

		sys.LegacyScrollers = false
		e.sh.SetAppearance(sys)
		if e.sv.Legacy() {
			t.Fatal("legacy after switching off")
		}
		drain(e.app)
		wantLegacyNotes(t, "off", e.sidebar, 180)
		if got := e.sidebar.Offset(); got != (Point{Y: notesMax}) {
			t.Errorf("off: offset %v, want (0, %d)", got, notesMax)
		}
		if len(e.scrolls) != 0 {
			t.Errorf("OnScroll %v by the switches", e.scrolls)
		}
	})
}
