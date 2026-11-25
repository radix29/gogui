package aqua

import (
	"fmt"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"
)

// The Increase Contrast outlines of the matrix's cases (S6.1e): a case not
// listed draws none.
const (
	outlineKey  = 1 // in a key window only (the accent fill goes there)
	outlineBoth = 2 // in a key and an inactive window
)

var contrastOutlines = map[string]int{
	"button/default":          outlineKey,
	"button/focused default":  outlineKey,
	"button/small default":    outlineKey,
	"button/mini default":     outlineKey,
	"button/large default":    outlineKey,
	"checkbox/on":             outlineBoth,
	"checkbox/mixed":          outlineBoth,
	"checkbox/focused on":     outlineBoth,
	"checkbox/disabled on":    outlineBoth,
	"checkbox/disabled mixed": outlineBoth,
	"radio/on":                outlineBoth,
	"radio/focused on":        outlineBoth,
	"radio/disabled on":       outlineBoth,
	"slider/normal":           outlineBoth,
	"slider/ticks":            outlineBoth,
	"slider/focused":          outlineBoth,
	"slider/disabled":         outlineBoth,
	"slider/disabled ticks":   outlineBoth,
	"pop-up button/normal":    outlineKey,
	"pop-up button/focused":   outlineKey,
}

// TestContrastMatrix checks Increase Contrast over every case of the
// inactive-window matrix, light and dark, in a key and an inactive window
// (S6.1e), against the same case without it:
//
//   - No shape moves, appears or goes, except the outlines: the fills are
//     the same rectangles in the same order, and the strokes are the same
//     plus one outline where the normal look has no border around an
//     accent fill (contrastOutlines), drawn in BezelOutline or
//     SliderTrackOutline.
//   - Without Increase Contrast no outline is drawn.
//   - A focused control's ring in a key window is the opaque accent, and a
//     selection in a key window is the stronger contrast selection; an
//     inactive window's unemphasized selection keeps its color.
func TestContrastMatrix(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, c := range inactiveCases {
			id := c.kind + "/" + c.state
			t.Run(fmt.Sprintf("%s/dark=%v", id, dark), func(t *testing.T) {
				n := inactiveDumps(t, c, dark, false, c.focus)
				h := inactiveDumps(t, c, dark, true, c.focus)
				ap := driver.Appearance{Dark: dark, Accent: driver.DefaultAccent}
				hc := ap
				hc.IncreaseContrast = true
				outline := tokens.Controls.BezelOutline.Value(hc)
				if c.kind == "slider" {
					outline = tokens.Controls.SliderTrackOutline.Value(hc)
				}
				for _, w := range []struct {
					name string
					n, h string
					want int
					key  bool
				}{
					{"key", n.key, h.key, min(contrastOutlines[id], 1), true},
					{"inactive", n.inactive, h.inactive, contrastOutlines[id] / outlineBoth, false},
				} {
					if a, b := fillShapes(w.n), fillShapes(w.h); a != b {
						t.Errorf("%s: the fills moved:\n%s", w.name, lineDiff(a, b))
					}
					ns, hs := strokes(w.n), strokes(w.h)
					if got := len(hs) - len(ns); got != w.want {
						t.Errorf("%s: %d more strokes with Increase Contrast, want %d:\n%s", w.name, got, w.want, lineDiff(strings.Join(ns, "\n"), strings.Join(hs, "\n")))
					}
					if w.want > 0 && !hasStroke(w.h, outlineHex(outline, c)) {
						t.Errorf("%s: no outline in %s:\n%s", w.name, outlineHex(outline, c), w.h)
					}
					if c.focus && w.key && !hasStroke(w.h, " w=3 solid "+hexColor(tokens.FocusRing(hc))) {
						t.Errorf("%s: no opaque focus ring %s:\n%s", w.name, hexColor(tokens.FocusRing(hc)), w.h)
					}
					if c.keepsFocusLook {
						sel := hexColor(tokens.TextSelection(hc, true, w.key))
						if w.key && sel == hexColor(tokens.TextSelection(ap, true, true)) {
							t.Fatal("the contrast selection is the normal one")
						}
						if !strings.Contains(w.h, " solid "+sel+" ") && !strings.Contains(w.h, " solid "+sel+"\n") {
							t.Errorf("%s: no selection in %s:\n%s", w.name, sel, w.h)
						}
					}
				}
			})
		}
	}
}

// outlineHex is the color an outline is drawn in, as a dump prints it: a
// disabled slider dims its outline as its track.
func outlineHex(col driver.Color, c inactiveCase) string {
	if c.kind == "slider" && strings.HasPrefix(c.state, "disabled") {
		col.A *= float32(tokens.Controls.ValueDisabledAlpha)
	}
	return hexColor(col)
}

// fillShapes returns a dump's fill operations without their paint, one per
// line.
func fillShapes(dump string) string {
	var out []string
	for _, l := range strings.Split(dump, "\n") {
		if !strings.HasPrefix(l, "fill") {
			continue
		}
		for _, p := range []string{" solid ", " linear ", " radial "} {
			if i := strings.Index(l, p); i >= 0 {
				l = l[:i]
				break
			}
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// strokes returns a dump's stroke operations.
func strokes(dump string) []string {
	var out []string
	for _, l := range strings.Split(dump, "\n") {
		if strings.HasPrefix(l, "stroke") {
			out = append(out, l)
		}
	}
	return out
}

// hasStroke reports whether a stroke of the dump ends in the paint suffix
// (or is the color hex, painted solid).
func hasStroke(dump, suffix string) bool {
	if strings.HasPrefix(suffix, "#") {
		suffix = " solid " + suffix
	}
	for _, l := range strokes(dump) {
		for _, p := range []string{" clip=", " alpha="} {
			if i := strings.Index(l, p); i >= 0 {
				l = l[:i]
			}
		}
		if strings.HasSuffix(l, suffix) {
			return true
		}
	}
	return false
}
