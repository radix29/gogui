package aqua

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
)

// The legacy scrollers of a ScrollView (PLAN.md S3.5f, scroller.go and
// the "Legacy scrollers" bullet of ScrollView): the layout, the runtime
// switch and the pinned paint states. All of them run on scrollEnv
// (scrollenv_test.go): a 150 × 100 scroll view at (10, 10) of the content
// with a vertical scroller and a 150 × 400 document.

// legacySys is the shell's appearance with "Always show scrollbars" on.
var legacySys = driver.Appearance{Accent: driver.DefaultAccent, LegacyScrollers: true}

// overlaySys is the default appearance (overlay scrollers).
var overlaySys = driver.Appearance{Accent: driver.DefaultAccent}

// legacyLines returns what the content of the 300 pt wide scroll window
// draws: the lines between the content background and the title bar,
// clips cut off.
func legacyLines(dump string) string {
	var out []string
	for _, l := range strings.Split(dump, "\n")[1:] {
		if strings.HasPrefix(l, "fillRRect (0,0,300,28)") {
			break
		}
		l, _, _ = strings.Cut(l, " clip=")
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// wantResized checks the ViewportResized calls since the last check and
// clears them.
func (e *scrollEnv) wantResized(t *testing.T, what string, want ...driver.Size) {
	t.Helper()
	if !slices.Equal(e.h.resized, want) {
		t.Errorf("%s: ViewportResized %v, want %v", what, e.h.resized, want)
	}
	e.h.resized = nil
}

// fadeTime is well past an overlay scroller's fade.
func fadeTime() time.Duration {
	return tokens.Animation.ScrollerFadeDelay + tokens.Animation.ScrollerFadeDuration + time.Second
}

func TestScrollerLegacyLayout(t *testing.T) {
	scrollRun(t, legacySys, func(e *scrollEnv) {
		sc := &tokens.Scroll
		lt := sc.LegacyScrollerThickness
		if !e.sv.Legacy() {
			t.Fatal("not legacy with LegacyScrollers on from the start")
		}
		if got, want := e.sv.ViewportInset, (driver.Insets{Right: lt}); got != want {
			t.Errorf("viewport inset %v, want %v", got, want)
		}
		if got, want := e.sv.ViewportSize(), (driver.Size{W: 150 - lt, H: 100}); got != want {
			t.Errorf("viewport %v, want %v", got, want)
		}
		if got, want := e.sv.viewport.Frame(), (driver.Rect{W: 150 - lt, H: 100}); got != want {
			t.Errorf("viewport host %v, want %v", got, want)
		}
		if got, want := e.sv.MaxOffset(), (driver.Point{X: lt, Y: 300}); got != want {
			t.Errorf("max offset %v, want %v", got, want)
		}
		if got := e.sv.y.Max(); got != 300 {
			t.Errorf("vertical axis max %g, want 300", got)
		}
		// The strip is outside the viewport, at its right.
		if got, want := e.vbar().Frame(), (driver.Rect{X: 150 - lt, W: lt, H: 100}); got != want {
			t.Errorf("vertical strip %v, want %v", got, want)
		}
		if e.vbar().Hidden() {
			t.Error("legacy scroller hidden before any scrolling")
		}
		if !e.hbar().Hidden() {
			t.Error("horizontal scroller shown without SetHasHorizontalScroller")
		}
		// The knob: ScrollerHoverThickness centred across the 15 pt track,
		// ScrollerInset from the ends; 96 × 100/400 = 24 long.
		g := e.vbar().geometry()
		if !g.ok || g.thickness != sc.ScrollerHoverThickness || g.across0 != (lt-sc.ScrollerHoverThickness)/2 ||
			g.track0 != sc.ScrollerInset || g.track != 100-2*sc.ScrollerInset || g.knobLen != 24 || g.knob0 != 2 {
			t.Errorf("geometry %+v", g)
		}
		knob := "fillRRect (148,40,9,24) r=4.5 solid #00000073"
		track := "fillRRect (145,38,15,100) solid #fafafa"
		dump := legacyLines(paint(t, e.sh, e.sf))
		for _, l := range []string{track, "fillRRect (145,38,1,100) solid #00000014", knob} {
			if !strings.Contains(dump, l) {
				t.Errorf("no %q in\n%s", l, dump)
			}
		}

		// Scrolling (the user) and then waiting does not fade it; the knob
		// keeps its opaque color.
		e.sf.Wheel(e.at(50, 50), driver.Point{Y: -1})
		e.clk.Advance(fadeTime())
		if e.vbar().Hidden() {
			t.Error("legacy scroller hidden after the fade time")
		}
		if e.sv.Offset().Y == 0 {
			t.Fatal("the wheel did not scroll")
		}
		e.sv.SetOffset(driver.Point{})
		e.clk.Advance(fadeTime())
		if e.vbar().Hidden() {
			t.Error("legacy scroller hidden after SetOffset and the fade time")
		}
		if dump := legacyLines(paint(t, e.sh, e.sf)); !strings.Contains(dump, knob) {
			t.Errorf("knob faded:\n%s", dump)
		}

		// Hover does not widen it.
		e.sf.Move(e.at(150-lt/2, 50))
		e.clk.Advance(time.Second)
		if b := e.vbar(); b.hovered || b.wide.Value() != 0 {
			t.Errorf("hovered %v, wide %g over a legacy scroller", b.hovered, b.wide.Value())
		}
		if g := e.vbar().geometry(); g.thickness != sc.ScrollerHoverThickness || g.across0 != 3 {
			t.Errorf("hovered geometry %+v", g)
		}
		if dump := legacyLines(paint(t, e.sh, e.sf)); !strings.Contains(dump, knob) {
			t.Errorf("hovered knob changed:\n%s", dump)
		}
		e.sf.Move(e.at(50, 50))
		e.clk.Advance(fadeTime())
		if e.vbar().Hidden() {
			t.Error("legacy scroller hidden after the pointer left it")
		}

		// The document fits: no knob, the track is still drawn (and the
		// viewport keeps the inset: it never depends on the document).
		e.sv.SetDocumentSize(driver.Size{W: 150 - lt, H: 100})
		e.wantResized(t, "document fits")
		if e.sv.ViewportSize() != (driver.Size{W: 150 - lt, H: 100}) || e.sv.MaxOffset() != (driver.Point{}) {
			t.Errorf("fitting document: viewport %v, max %v", e.sv.ViewportSize(), e.sv.MaxOffset())
		}
		if e.vbar().Hidden() || e.vbar().geometry().ok {
			t.Errorf("fitting document: hidden %v, knob %v", e.vbar().Hidden(), e.vbar().geometry().ok)
		}
		dump = legacyLines(paint(t, e.sh, e.sf))
		if !strings.Contains(dump, track) || strings.Contains(dump, "r=4.5") {
			t.Errorf("fitting document: want the track and no knob:\n%s", dump)
		}

		// Both scrollers: the horizontal strip under the viewport, the
		// vertical one stops at it, the corner is filled.
		e.sv.SetDocumentSize(driver.Size{W: 150, H: 400})
		e.sv.SetHasHorizontalScroller(true)
		e.wantResized(t, "horizontal scroller", driver.Size{W: 150 - lt, H: 100 - lt})
		if got, want := e.sv.MaxOffset(), (driver.Point{X: lt, Y: 300 + lt}); got != want {
			t.Errorf("both: max offset %v, want %v", got, want)
		}
		if got, want := e.hbar().Frame(), (driver.Rect{Y: 100 - lt, W: 150 - lt, H: lt}); got != want {
			t.Errorf("horizontal strip %v, want %v", got, want)
		}
		if got, want := e.vbar().Frame(), (driver.Rect{X: 150 - lt, W: lt, H: 100 - lt}); got != want {
			t.Errorf("both: vertical strip %v, want %v", got, want)
		}
		if e.hbar().Hidden() || e.vbar().Hidden() {
			t.Error("both: a legacy scroller hidden")
		}
		if dump := legacyLines(paint(t, e.sh, e.sf)); !strings.Contains(dump, "fillRRect (145,123,15,15) solid #fafafa") {
			t.Errorf("no corner fill:\n%s", dump)
		}
	})
}

func TestScrollerLegacySwitch(t *testing.T) {
	scrollRun(t, overlaySys, func(e *scrollEnv) {
		lt := tokens.Scroll.LegacyScrollerThickness
		e.sv.SetHasHorizontalScroller(true)
		e.wantResized(t, "overlay: SetHasHorizontalScroller") // overlay scrollers take nothing
		if e.sv.Legacy() || e.sv.ViewportSize() != (driver.Size{W: 150, H: 100}) {
			t.Fatalf("overlay: legacy %v, viewport %v", e.sv.Legacy(), e.sv.ViewportSize())
		}
		// Show the overlay scrollers by scrolling.
		e.sf.Wheel(e.at(50, 50), driver.Point{Y: -1})
		e.clk.Advance(100 * time.Millisecond)
		if e.vbar().Hidden() {
			t.Fatal("overlay scroller not shown by the wheel")
		}
		e.sv.SetOffset(driver.Point{Y: 300})
		e.h.scrolled = nil

		// A non-legacy change fires nothing.
		dark := overlaySys
		dark.Dark = true
		e.sh.SetAppearance(dark)
		e.wantResized(t, "overlay: dark")
		e.sh.SetAppearance(overlaySys)
		e.wantResized(t, "overlay: light")

		// On: the viewport loses 15 pt on each axis with a scroller, once.
		e.sh.SetAppearance(legacySys)
		if !e.sv.Legacy() {
			t.Fatal("not legacy after the switch")
		}
		e.wantResized(t, "on", driver.Size{W: 150 - lt, H: 100 - lt})
		if got, want := e.sv.ViewportInset, (driver.Insets{Right: lt, Bottom: lt}); got != want {
			t.Errorf("on: inset %v, want %v", got, want)
		}
		if e.vbar().Hidden() || e.hbar().Hidden() {
			t.Error("on: a legacy scroller hidden")
		}
		if e.vbar().wide.Value() != 0 || e.vbar().hovered {
			t.Error("on: the overlay state was not reset")
		}
		if got := e.sv.Offset(); got != (driver.Point{Y: 300}) {
			t.Errorf("on: offset %v, want (0, 300)", got)
		}
		e.sv.SetOffset(driver.Point{X: lt, Y: 300 + lt})
		if got := e.sv.Offset(); got != (driver.Point{X: lt, Y: 300 + lt}) {
			t.Fatalf("on: offset %v after SetOffset to the max", got)
		}
		// Not LegacyScrollers: nothing.
		dark = legacySys
		dark.Dark = true
		e.sh.SetAppearance(dark)
		e.wantResized(t, "legacy: dark")
		hc := legacySys
		hc.IncreaseContrast = true
		e.sh.SetAppearance(hc)
		e.wantResized(t, "legacy: increased contrast")
		e.sh.SetAppearance(legacySys)
		e.wantResized(t, "legacy: back to light")
		e.sh.Deactivate()
		e.sf.Activate()
		e.wantResized(t, "legacy: key change")

		// Off: the viewport grows back once, the offset is clamped, the
		// overlay scrollers are hidden at once (no fade).
		e.sh.SetAppearance(overlaySys)
		if e.sv.Legacy() {
			t.Fatal("legacy after switching off")
		}
		e.wantResized(t, "off", driver.Size{W: 150, H: 100})
		if e.sv.ViewportInset != (driver.Insets{}) {
			t.Errorf("off: inset %v", e.sv.ViewportInset)
		}
		if got := e.sv.Offset(); got != (driver.Point{Y: 300}) {
			t.Errorf("off: offset %v, want clamped to (0, 300)", got)
		}
		if !e.vbar().Hidden() || !e.hbar().Hidden() {
			t.Errorf("off: overlay scrollers shown (vertical hidden %v, horizontal hidden %v)", e.vbar().Hidden(), e.hbar().Hidden())
		}
		e.clk.Advance(fadeTime())
		if got := e.sv.Offset(); got != (driver.Point{Y: 300}) {
			t.Errorf("off, settled: offset %v, want (0, 300)", got)
		}
		if got := e.sv.document.Frame(); got != (driver.Rect{Y: -300, W: 150, H: 400}) {
			t.Errorf("off, settled: document host %v", got)
		}
		if len(e.h.scrolled) != 0 {
			t.Errorf("Scrolled %v by the switches", e.h.scrolled)
		}
		if !e.vbar().Hidden() {
			t.Error("off, settled: overlay scroller shown")
		}
		dump := legacyLines(paint(t, e.sh, e.sf))
		if strings.Contains(dump, "#fafafa") || strings.Contains(dump, "r=") {
			t.Errorf("off: legacy track or a knob drawn:\n%s", dump)
		}
	})
}

func TestScrollerLegacySetHasScroller(t *testing.T) {
	scrollRun(t, legacySys, func(e *scrollEnv) {
		lt := tokens.Scroll.LegacyScrollerThickness
		e.sv.SetHasVerticalScroller(false)
		e.wantResized(t, "vertical off", driver.Size{W: 150, H: 100})
		if !e.vbar().Hidden() {
			t.Error("vertical scroller shown after SetHasVerticalScroller(false)")
		}
		e.sv.SetHasVerticalScroller(false)
		e.wantResized(t, "vertical off again")
		e.sv.SetHasVerticalScroller(true)
		e.wantResized(t, "vertical on", driver.Size{W: 150 - lt, H: 100})
		if e.vbar().Hidden() {
			t.Error("vertical scroller hidden after SetHasVerticalScroller(true)")
		}
		e.sv.SetHasHorizontalScroller(true)
		e.wantResized(t, "horizontal on", driver.Size{W: 150 - lt, H: 100 - lt})
		e.sv.SetHasHorizontalScroller(false)
		e.wantResized(t, "horizontal off", driver.Size{W: 150 - lt, H: 100})
		// SetFrame reads the kind but does not report (the layout runs).
		e.sv.SetFrame(driver.Rect{X: 10, Y: 10, W: 120, H: 80})
		e.wantResized(t, "SetFrame")
		if got := e.sv.ViewportSize(); got != (driver.Size{W: 120 - lt, H: 80}) {
			t.Errorf("after SetFrame: viewport %v", got)
		}
	})
}

// TestScrollerLegacyStates pins the legacy scrollers' paint states in
// testdata/scroll_legacy.states (rewrite with -update).
func TestScrollerLegacyStates(t *testing.T) {
	var out strings.Builder
	dark := legacySys
	dark.Dark = true
	hc := legacySys
	hc.IncreaseContrast = true
	darkHC := dark
	darkHC.IncreaseContrast = true
	for _, st := range []struct {
		name string
		sys  driver.Appearance
		set  func(e *scrollEnv)
	}{
		{"vertical only", legacySys, nil},
		{"both scrollers", legacySys, func(e *scrollEnv) { e.sv.SetHasHorizontalScroller(true) }},
		{"both scrollers, scale 2", legacySys, func(e *scrollEnv) {
			e.sf.SetScale(2)
			e.sv.SetHasHorizontalScroller(true)
		}},
		{"no knob (document fits)", legacySys, func(e *scrollEnv) {
			e.sv.SetDocumentSize(driver.Size{W: 135, H: 100})
		}},
		{"at max offset", legacySys, func(e *scrollEnv) { e.sv.SetOffset(driver.Point{Y: 300}) }},
		{"both at max offset", legacySys, func(e *scrollEnv) {
			e.sv.SetHasHorizontalScroller(true)
			e.sv.SetOffset(driver.Point{X: 15, Y: 315})
		}},
		{"dark", dark, nil},
		{"dark, both scrollers", dark, func(e *scrollEnv) { e.sv.SetHasHorizontalScroller(true) }},
		{"increased contrast", hc, nil},
		{"dark, increased contrast", darkHC, nil},
		{"inactive", legacySys, func(e *scrollEnv) { e.sh.Deactivate() }},
		{"inactive, increased contrast", hc, func(e *scrollEnv) { e.sh.Deactivate() }},
		{"dark, inactive, increased contrast", darkHC, func(e *scrollEnv) { e.sh.Deactivate() }},
	} {
		scrollRun(t, st.sys, func(e *scrollEnv) {
			if st.set != nil {
				st.set(e)
			}
			fmt.Fprintf(&out, "## %s\n%s\n", st.name, legacyLines(paint(t, e.sh, e.sf)))
		})
	}
	got := out.String()
	path := filepath.Join("testdata", "scroll_legacy.states")
	if *updateGolden {
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
		t.Errorf("legacy scroller states differ from %s (rerun with -update if the change is intended):\n%s", path, got)
	}
}
