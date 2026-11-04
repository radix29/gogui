package gtk4

import (
	"log"
	"math"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/radix29/gogui/assets/fonts"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/mainthread"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/fake"
)

// The text system (PLAN.md S4.2g, S4.2h): render.TextSystem on Pango.
//
// As on Win32 (S2.5d), a layout is measured once, in NewLayout, and kept
// as Go data: its size, its first baseline, and the line and cluster
// geometry the queries answer from (textgeom.go). Its PangoLayout is
// unreffed right away, because TextLayout has no Release and nothing may
// rely on finalizers (PLAN.md §7.2).
// Drawing (S4.2i, canvas.go's DrawText) rebuilds a native layout from the
// text, font and width Pango measured (once per layout and paint).
// Everything Pango-specific sits behind pangoLayouter (text_linux.go), so
// the rules here are host-tested with a fake.
//
// Layouts come from one PangoContext of the GTK thread's default
// PangoCairo font map (the one GTK draws with), with
// round_glyph_positions off, so measurement does not depend on the
// scale. Sizes are absolute: 13 pt is 13 logical pixels. Wrapping is
// PANGO_WRAP_WORD_CHAR (at word boundaries, inside a word that does not
// fit), every paragraph left to right (auto_dir off, as DirectWrite),
// and metrics are not hinted (cairo font options). Pango takes valid
// UTF-8 without NUL only, so pangoText replaces
// each NUL and each invalid byte by one ASCII byte: byte offsets into the
// text Pango lays out are those of Text().
//
// Fonts: a named family the font map has is used as is; any other family
// is the UI font. The empty family is the UI font: Inter, the app font of
// assets/fonts (appfont.go, S4.2j), else the family of GtkSettings
// gtk-font-name, or sans-serif without one;
// with Monospaced it is monospace.
//
// Without Pango (the shell's tests on every host; open is nil there),
// after release, or once opening Pango failed, layouts come from
// shell/fake's fixed-width metrics, and the first failure is logged, so
// text never takes the application down.

const (
	// pangoScale is PANGO_SCALE: Pango units per logical pixel
	// (gtk.PangoScale, untyped here so the pure code needs no gtk import).
	pangoScale = 1024
	// minFontSize and maxFontSize clamp the sizes Pango is given (in
	// logical pixels), minWeight and maxWeight the weights (PangoWeight's
	// range).
	minFontSize = 0.1
	maxFontSize = 2048
	minWeight   = 100
	maxWeight   = 1000
	// maxFamilies is the size of the family lookup cache before it is
	// emptied.
	maxFamilies = 128
	// standIn replaces a NUL or an invalid byte in the text Pango is given.
	standIn = '?'
	// wrapSlack is what wrapWidth adds, in Pango units, before rounding
	// down: 1e-6 pt.
	wrapSlack = 1e-3
)

// Font families the system fonts resolve to (fontconfig aliases).
const (
	familyUIFallback = "sans-serif"
	familyMono       = "monospace"
)

// pangoFont is a resolved font as Pango is given it.
type pangoFont struct {
	family string
	size   float64 // absolute, in Pango units (logical pixels × pangoScale)
	weight int32   // a PangoWeight in [minWeight, maxWeight]
	italic bool    // PANGO_STYLE_ITALIC, else NORMAL
}

// pangoMetrics are the measurements of one PangoLayout, in Pango units.
type pangoMetrics struct {
	width, height int32 // the logical extents
	baseline      int32 // of the first line, from the top
	// lines (at least one, top to bottom) and clusters (one per cursor
	// position before the text's end, in text order) are the geometry
	// newGeometry reads.
	lines    []pangoLine
	clusters []pangoCluster
}

// pangoLayouter is what the text system needs of Pango: text_linux.go's
// pangoNative, a fake in the tests. It is used on the UI thread.
type pangoLayouter interface {
	// uiFamily returns the family of GtkSettings gtk-font-name, or "" when
	// there is none.
	uiFamily() string
	// hasFamily reports whether the font map has a family of that name (no
	// NUL in it).
	hasFamily(name string) bool
	// measure lays out text (valid UTF-8 without NUL, at most MaxInt32
	// bytes) in f, wrapped with PANGO_WRAP_WORD_CHAR at width (Pango
	// units; -1 for no wrapping), and measures it.
	measure(text string, f pangoFont, width int32) (pangoMetrics, error)
	// layout makes the native layout measure measured for the same
	// arguments, for drawing (S4.2i); the caller releases it.
	layout(text string, f pangoFont, width int32) (nativeLayout, error)
	// addFonts adds the font files at paths to the font map (the app
	// font, appfont.go), all of them or, on an error, maybe some.
	addFonts(paths []string) error
	// release frees what the layouter holds. It is called once, last.
	release()
}

// nativeLayout is a native layout a pangoLayouter made for drawing (a
// PangoLayout on Linux); release drops it.
type nativeLayout interface{ release() }

// textSystem implements render.TextSystem. It is used on the UI thread.
// Pango is opened with the first layout, which happens after Run has
// loaded GTK.
type textSystem struct {
	q *mainthread.Queue
	// open opens Pango (openPango on Linux); nil means there is none, so
	// every layout is a fake one.
	open func() (pangoLayouter, error)
	// logf logs the first failure (log.Printf; the tests' hook).
	logf func(format string, args ...any)
	// The app font (appfont.go): fontFiles returns its files (fonts.Files),
	// cacheDir the directory its cache goes under (os.UserCacheDir), appUI
	// is the family the empty family resolves to once they are added
	// (familyApp), and debugf logs why they are not used (log.Printf with
	// GOGUI_DEBUG=1, else nil). All four are the tests' hooks.
	fontFiles func() []fonts.File
	cacheDir  func() (string, error)
	appUI     string
	debugf    func(format string, args ...any)

	pl                     pangoLayouter // nil until inited and after release
	inited, closed, failed bool
	drawFailed             bool            // a native layout for drawing failed (logged once)
	ui                     string          // the UI family
	families               map[string]bool // hasFamily's answers
}

var _ render.TextSystem = (*textSystem)(nil)

func newTextSystem(q *mainthread.Queue) *textSystem {
	ts := &textSystem{q: q, logf: log.Printf, fontFiles: fonts.Files, cacheDir: os.UserCacheDir, appUI: familyApp}
	if debugChecks {
		ts.debugf = log.Printf
	}
	return ts
}

// init opens Pango, reads the system UI family and adds the app font,
// which is the UI family when that works. A failure to open is final: the
// text system is closed, and its layouts are fake ones from then on.
func (ts *textSystem) init() error {
	if ts.inited {
		return nil
	}
	pl, err := ts.open()
	if err != nil {
		ts.closed = true
		return err
	}
	ts.pl, ts.inited = pl, true
	ts.ui = familyUIFallback
	if f := pl.uiFamily(); f != "" && strings.IndexByte(f, 0) < 0 {
		ts.ui = f
	}
	if ts.loadAppFont() {
		ts.ui = ts.appUI
	}
	ts.families = make(map[string]bool)
	return nil
}

// release frees Pango's objects (Shell.shutdown). Later layouts are fake
// ones.
func (ts *textSystem) release() {
	ts.closed = true
	if ts.pl != nil {
		ts.pl.release()
		ts.pl = nil
	}
	ts.families = nil
	ts.inited = false
}

// genericFamilies are the CSS and fontconfig generic names, in lower case:
// fontconfig resolves them, but the font map may not list them (or lists
// them capitalised), so they count as present without asking Pango.
var genericFamilies = map[string]bool{
	"sans-serif": true, "sans": true, "serif": true, "monospace": true, "mono": true,
	"system-ui": true, "emoji": true, "cursive": true, "fantasy": true,
}

// hasFamily reports, cached, whether Pango has the family name. A name
// with a NUL byte has none (C would read it cut short); a generic name
// (genericFamilies, in any case) has one.
func (ts *textSystem) hasFamily(name string) bool {
	if strings.IndexByte(name, 0) >= 0 {
		return false
	}
	if genericFamilies[strings.ToLower(name)] {
		return true
	}
	if ok, found := ts.families[name]; found {
		return ok
	}
	if len(ts.families) >= maxFamilies {
		clear(ts.families)
	}
	ok := ts.pl.hasFamily(name)
	ts.families[name] = ok
	return ok
}

// resolve maps f to the font Pango is given (see the top of this file).
// The size is clamped to [minFontSize, maxFontSize] (zero, negative and NaN
// sizes are the default 13 pt, EffectiveSize), the weight to
// [minWeight, maxWeight].
func (ts *textSystem) resolve(f render.Font) pangoFont {
	pf := pangoFont{
		size:   min(max(f.EffectiveSize(driver.ControlSizeRegular), minFontSize), maxFontSize) * pangoScale,
		weight: int32(min(max(int(f.EffectiveWeight()), minWeight), maxWeight)),
		italic: f.Italic,
	}
	switch {
	case f.Family != "" && ts.hasFamily(f.Family):
		pf.family = f.Family
	case f.Family == "" && f.Monospaced:
		pf.family = familyMono
	default:
		pf.family = ts.ui
	}
	return pf
}

// wrapWidth returns the PangoLayout width for maxWidth: -1 (no wrapping)
// unless maxWidth is positive and finite (NaN fails the first test), else
// maxWidth in Pango units, rounded down (so wrapped text is never wider
// than maxWidth) after adding wrapSlack (so a width measured before fits
// again when the caller's arithmetic lands an ulp below it), and at least
// 1. A width past Pango's int range does not wrap either.
func wrapWidth(maxWidth float64) int32 {
	if !(maxWidth > 0) || math.IsInf(maxWidth, 1) {
		return -1
	}
	w := math.Floor(maxWidth*pangoScale + wrapSlack)
	if w > math.MaxInt32 {
		return -1
	}
	return max(int32(w), 1)
}

// pangoText returns s with every NUL byte and every byte that is not part
// of a valid UTF-8 sequence replaced by standIn, so the result has the
// same length and the same byte offsets. Valid text without NUL is
// returned as is.
func pangoText(s string) string {
	if strings.IndexByte(s, 0) < 0 && utf8.ValidString(s) {
		return s
	}
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if r == 0 || (r == utf8.RuneError && n == 1) {
			b = append(b, standIn)
		} else {
			b = append(b, s[i:i+n]...)
		}
		i += n
	}
	return string(b)
}

// points converts Pango units to logical points.
func points(u int32) float64 { return float64(u) / pangoScale }

// NewLayout implements render.TextSystem.
func (ts *textSystem) NewLayout(text string, font render.Font, maxWidth float64) render.TextLayout {
	ts.q.Check("TextSystem.NewLayout")
	// Pango takes a text length as an int: longer text is fake, which is
	// not Pango failing (no log).
	if !ts.closed && ts.open != nil && len(text) <= math.MaxInt32 {
		l, err := ts.layout(text, font, maxWidth)
		if err == nil {
			return l
		}
		if !ts.failed {
			ts.failed = true
			ts.logf("gtk4: text: Pango failed, using fixed-width metrics: %v", err)
		}
	}
	return fake.NewTextSystem().NewLayout(text, font, maxWidth)
}

func (ts *textSystem) layout(text string, font render.Font, maxWidth float64) (*textLayout, error) {
	if err := ts.init(); err != nil {
		return nil, err
	}
	pt, pf, w := pangoText(text), ts.resolve(font), wrapWidth(maxWidth)
	m, err := ts.pl.measure(pt, pf, w)
	if err != nil {
		return nil, err
	}
	g, err := newGeometry(pt, m.lines, m.clusters, newlineWidth(pf.size/pangoScale))
	if err != nil {
		return nil, err
	}
	return &textLayout{
		ts: ts, text: text, font: font, maxWidth: maxWidth,
		ptext: pt, pfont: pf, width: w,
		size:     render.Size{W: points(m.width), H: points(m.height)},
		baseline: points(m.baseline),
		geom:     g,
	}, nil
}

// native returns a native layout of l for drawing (canvas.go), or nil
// when the text system is closed or Pango fails; the first failure is
// logged. The caller releases it.
func (ts *textSystem) native(l *textLayout) nativeLayout {
	if ts.closed || ts.pl == nil {
		return nil
	}
	nl, err := ts.pl.layout(l.ptext, l.pfont, l.width)
	if err != nil || nl == nil {
		if !ts.drawFailed {
			ts.drawFailed = true
			ts.logf("gtk4: text: Pango failed to make a layout to draw, text is not drawn: %v", err)
		}
		return nil
	}
	return nl
}

// textLayout is a render.TextLayout: the measurements of one PangoLayout,
// kept as Go data (see the top of this file).
type textLayout struct {
	ts       *textSystem // the system that made it (the canvas draws only its own)
	text     string
	font     render.Font
	maxWidth float64 // as given to NewLayout
	// ptext, pfont and width are what Pango measured: the text with its
	// stand-ins, the resolved font and the wrap width, so drawing rebuilds
	// the very layout that was measured.
	ptext    string
	pfont    pangoFont
	width    int32
	size     render.Size
	baseline float64  // of the first line
	geom     geometry // lines and clusters for the queries (textgeom.go)
}

var _ render.TextLayout = (*textLayout)(nil)

func (l *textLayout) Text() string      { return l.text }
func (l *textLayout) Font() render.Font { return l.font }
func (l *textLayout) Size() render.Size { return l.size }
func (l *textLayout) Baseline() float64 { return l.baseline }
func (l *textLayout) LineCount() int    { return len(l.geom.lines) }

// HitTest implements render.TextLayout.
func (l *textLayout) HitTest(p render.Point) (int, bool) { return l.geom.hitTest(p.X, p.Y) }

// CaretRect implements render.TextLayout.
func (l *textLayout) CaretRect(index int) render.Rect {
	return l.geom.caret(l.geom.clusterStart(index))
}

// SelectionRects implements render.TextLayout.
func (l *textLayout) SelectionRects(start, end int) []render.Rect {
	a, b := l.geom.clusterStart(start), l.geom.clusterStart(end)
	return l.geom.selection(min(a, b), max(a, b))
}

// normalize returns index clamped to the text and moved to its cluster's
// start.
func (l *textLayout) normalize(index int) int { return l.geom.clusterStart(index) }
