package gtk4

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/radix29/gogui/assets/fonts"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/mainthread"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/fake"
)

// The text host tests (S4.2g): font resolution, the byte stand-ins, the
// wrap-width mapping and the metrics conversion of textSystem through a
// fake pangoLayouter; the fallbacks to shell/fake's layouts (no Pango,
// Pango failing to open or to measure, after release); and the shell's
// text system (fake layouts on the host, released at the end of Run).

// txMeasure is one call of txLayouter.measure.
type txMeasure struct {
	text  string
	font  pangoFont
	width int32
}

// txLayouter is the fake pangoLayouter: it has the families in has, reads
// ui as gtk-font-name, measures every text as m (or fails with err) and
// records its calls. When m has no lines, measure gives the geometry of
// the text unwrapped (tgRunes: a line per "\n", a cluster per rune, cw
// points wide, lines lh points tall), unless noGeom is set.
type txLayouter struct {
	ui     string
	has    map[string]bool
	m      pangoMetrics
	err    error
	cw, lh float64
	noGeom bool

	uiCalls  int
	asked    map[string]int // hasFamily calls per name
	measures []txMeasure
	// layouts are the native layouts layout made (S4.2i), in order;
	// layoutErr makes layout fail; layoutCalls counts the calls, failed
	// ones included.
	layouts     []*txNative
	layoutErr   error
	layoutCalls int
	releases    int
	// added are the paths of each addFonts call (S4.2j), addErr makes it
	// fail, adds are the families it adds, and measuredBeforeAdd counts
	// the measures made before an addFonts call.
	added             [][]string
	addErr            error
	adds              []string
	measuredBeforeAdd int
	// afterRelease counts the calls made after release.
	afterRelease int
}

func (p *txLayouter) uiFamily() string {
	p.note()
	p.uiCalls++
	return p.ui
}

func (p *txLayouter) hasFamily(name string) bool {
	p.note()
	if p.asked == nil {
		p.asked = make(map[string]int)
	}
	p.asked[name]++
	return p.has[name]
}

func (p *txLayouter) measure(text string, f pangoFont, width int32) (pangoMetrics, error) {
	p.note()
	p.measures = append(p.measures, txMeasure{text, f, width})
	m := p.m
	if m.lines == nil && !p.noGeom {
		lines, cs := tgRunes(text)
		m.lines, m.clusters = tgLTR(text, lines, cs, p.cw, p.lh)
	}
	return m, p.err
}

// txNative is a native layout txLayouter made: its number (from 1), the
// arguments it was made with, and how often it was released.
type txNative struct {
	id       int
	args     txMeasure
	released int
}

func (n *txNative) release() { n.released++ }

func (p *txLayouter) layout(text string, f pangoFont, width int32) (nativeLayout, error) {
	p.layoutCalls++
	p.note()
	if p.layoutErr != nil {
		return nil, p.layoutErr
	}
	n := &txNative{id: len(p.layouts) + 1, args: txMeasure{text, f, width}}
	p.layouts = append(p.layouts, n)
	return n, nil
}

func (p *txLayouter) release() {
	p.note()
	p.releases++
}

// addFonts records the paths (and how many layouts were measured before
// them), fails with addErr, or else gives the font map the families in
// adds.
func (p *txLayouter) addFonts(paths []string) error {
	p.note()
	p.added = append(p.added, slices.Clone(paths))
	p.measuredBeforeAdd += len(p.measures)
	if p.addErr != nil {
		return p.addErr
	}
	for _, f := range p.adds {
		if p.has == nil {
			p.has = make(map[string]bool)
		}
		p.has[f] = true
	}
	return nil
}

func (p *txLayouter) note() {
	if p.releases > 0 {
		p.afterRelease++
	}
}

// txLogs records the messages of a textSystem's logf.
type txLogs struct{ msgs []string }

func (lg *txLogs) logf(format string, args ...any) {
	lg.msgs = append(lg.msgs, fmt.Sprintf(format, args...))
}

// txSystem returns a text system over pl (opened once, counted in *opens)
// that logs into the returned txLogs.
func txSystem(pl *txLayouter, openErr error, opens *int) (*textSystem, *txLogs) {
	ts := newTextSystem(mainthread.New())
	lg := &txLogs{}
	ts.logf = lg.logf
	ts.fontFiles = func() []fonts.File { return nil } // no app font unless a test sets one
	ts.open = func() (pangoLayouter, error) {
		if opens != nil {
			*opens++
		}
		if openErr != nil {
			return nil, openErr
		}
		return pl, nil
	}
	return ts, lg
}

// txResolve returns the font f resolves to on a text system over pl.
func txResolve(t *testing.T, pl *txLayouter, f render.Font) pangoFont {
	t.Helper()
	ts, _ := txSystem(pl, nil, nil)
	if err := ts.init(); err != nil {
		t.Fatal(err)
	}
	return ts.resolve(f)
}

// txIsFake reports whether l is a shell/fake layout equal to the one fake
// makes of the same arguments.
func txIsFake(t *testing.T, l render.TextLayout, text string, f render.Font, maxWidth float64) {
	t.Helper()
	if _, ok := l.(*textLayout); ok || l == nil {
		t.Fatalf("layout %T, want a shell/fake layout", l)
	}
	want := fake.NewTextSystem().NewLayout(text, f, maxWidth)
	if l.Size() != want.Size() || l.LineCount() != want.LineCount() || l.Text() != text || l.Font() != f {
		t.Errorf("layout %v, %d lines, %q, %+v; want fake's %v, %d lines",
			l.Size(), l.LineCount(), l.Text(), l.Font(), want.Size(), want.LineCount())
	}
}

// TestTextResolveFamily: the empty family is the UI family (gtk-font-name,
// else sans-serif; one with NUL is ignored), monospace with Monospaced; a
// family the font map has is itself, any other the UI family; a family
// with NUL is the UI family without asking the layouter.
func TestTextResolveFamily(t *testing.T) {
	has := map[string]bool{"DejaVu Serif": true, "Cantarell": true}
	for _, tc := range []struct {
		name   string
		ui     string
		font   render.Font
		want   string
		noAsks bool // hasFamily must not be called
	}{
		{"empty family", "Cantarell", render.Font{}, "Cantarell", true},
		{"no gtk-font-name", "", render.Font{}, familyUIFallback, true},
		{"gtk-font-name with NUL", "Can\x00tarell", render.Font{}, familyUIFallback, true},
		{"monospaced", "Cantarell", render.Font{Monospaced: true}, familyMono, true},
		{"monospaced without gtk-font-name", "", render.Font{Monospaced: true}, familyMono, true},
		{"monospaced ignored with a family", "Cantarell", render.Font{Family: "DejaVu Serif", Monospaced: true}, "DejaVu Serif", false},
		{"monospaced ignored with an unknown family", "Cantarell", render.Font{Family: "Nope", Monospaced: true}, "Cantarell", false},
		{"known family", "Cantarell", render.Font{Family: "DejaVu Serif"}, "DejaVu Serif", false},
		{"known family is the UI's", "Cantarell", render.Font{Family: "Cantarell"}, "Cantarell", false},
		{"unknown family", "Cantarell", render.Font{Family: "Helvetica Neue"}, "Cantarell", false},
		{"unknown family without gtk-font-name", "", render.Font{Family: "Helvetica Neue"}, familyUIFallback, false},
		{"family with NUL", "Cantarell", render.Font{Family: "DejaVu Serif\x00"}, "Cantarell", true},
		{"family with NUL inside", "Cantarell", render.Font{Family: "DejaVu\x00 Serif"}, "Cantarell", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pl := &txLayouter{ui: tc.ui, has: has}
			pf := txResolve(t, pl, tc.font)
			if pf.family != tc.want {
				t.Errorf("family %q, want %q", pf.family, tc.want)
			}
			if tc.noAsks && len(pl.asked) != 0 {
				t.Errorf("hasFamily asked %v, want no call", pl.asked)
			}
			if !tc.noAsks && pl.asked[tc.font.Family] != 1 {
				t.Errorf("hasFamily asked %v, want %q once", pl.asked, tc.font.Family)
			}
			if pl.uiCalls != 1 {
				t.Errorf("uiFamily called %d times, want once (in init)", pl.uiCalls)
			}
		})
	}
}

// TestTextFamilyCache: hasFamily asks the layouter once per name, known or
// not, and empties the cache when it holds maxFamilies names.
func TestTextFamilyCache(t *testing.T) {
	pl := &txLayouter{ui: "Cantarell", has: map[string]bool{"A": true}}
	ts, _ := txSystem(pl, nil, nil)
	if err := ts.init(); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if pf := ts.resolve(render.Font{Family: "A"}); pf.family != "A" {
			t.Errorf("A resolved to %q", pf.family)
		}
		if pf := ts.resolve(render.Font{Family: "B"}); pf.family != "Cantarell" {
			t.Errorf("B resolved to %q", pf.family)
		}
	}
	if pl.asked["A"] != 1 || pl.asked["B"] != 1 {
		t.Errorf("asked %v, want A and B once each", pl.asked)
	}

	// Fill the cache to maxFamilies: A and B are still cached.
	for i := len(ts.families); i < maxFamilies; i++ {
		ts.hasFamily(fmt.Sprint("F", i))
	}
	if len(ts.families) != maxFamilies {
		t.Fatalf("cache holds %d, want %d", len(ts.families), maxFamilies)
	}
	ts.resolve(render.Font{Family: "A"})
	if pl.asked["A"] != 1 {
		t.Errorf("A asked %d times with the cache full, want still once", pl.asked["A"])
	}
	// A new name empties it first.
	ts.resolve(render.Font{Family: "C"})
	if len(ts.families) != 1 || !slices.Equal(txKeys(ts.families), []string{"C"}) {
		t.Errorf("cache after a new name past the limit: %v, want only C", ts.families)
	}
	ts.resolve(render.Font{Family: "A"})
	if pl.asked["A"] != 2 || pl.asked["C"] != 1 {
		t.Errorf("asked A %d, C %d times; want 2 (asked again after the reset), 1", pl.asked["A"], pl.asked["C"])
	}
}

// txKeys returns the keys of m, sorted.
func txKeys(m map[string]bool) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	return ks
}

// TestTextResolveSize: zero, negative and NaN sizes are 13 pt; sizes are
// clamped to [minFontSize, maxFontSize] and given in Pango units.
func TestTextResolveSize(t *testing.T) {
	for _, tc := range []struct {
		size, want float64 // want in points
	}{
		{0, 13},
		{-1, 13},
		{math.Inf(-1), 13},
		{math.NaN(), 13},
		{13, 13},
		{11.5, 11.5},
		{0.1, 0.1},
		{0.01, 0.1},
		{math.SmallestNonzeroFloat64, 0.1},
		{2048, 2048},
		{2049, 2048},
		{1e300, 2048},
		{math.Inf(1), 2048},
	} {
		pf := txResolve(t, &txLayouter{}, render.Font{Size: tc.size})
		if want := tc.want * pangoScale; pf.size != want {
			t.Errorf("size %v: %v Pango units, want %v", tc.size, pf.size, want)
		}
	}
}

// TestTextResolveWeightItalic: weight 0 is 400, weights are clamped to
// [minWeight, maxWeight]; Italic is passed on.
func TestTextResolveWeightItalic(t *testing.T) {
	for _, tc := range []struct {
		weight driver.FontWeight
		want   int32
	}{
		{0, 400},
		{-1, 100},
		{-32768, 100},
		{1, 100},
		{99, 100},
		{100, 100},
		{700, 700},
		{950, 950},
		{1000, 1000},
		{1001, 1000},
		{32767, 1000},
	} {
		pf := txResolve(t, &txLayouter{}, render.Font{Weight: tc.weight})
		if pf.weight != tc.want || pf.italic {
			t.Errorf("weight %d: %d, italic %v; want %d, false", tc.weight, pf.weight, pf.italic, tc.want)
		}
	}
	if pf := txResolve(t, &txLayouter{}, render.Font{Italic: true, Weight: 700}); !pf.italic || pf.weight != 700 {
		t.Errorf("italic bold: %+v", pf)
	}
}

// TestTextPangoText: NUL bytes and every byte outside a valid UTF-8
// sequence (lone continuation and lead bytes, truncated sequences,
// surrogate and overlong encodings, bytes past U+10FFFF) become one '?'
// each, so the length is kept; valid sequences are kept; valid text is
// returned as the same string.
func TestTextPangoText(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"NUL", "\x00", "?"},
		{"NULs", "a\x00b\x00\x00", "a?b??"},
		{"lone continuation", "a\x80b", "a?b"},
		{"lone lead", "\xc3", "?"},
		{"lead before ASCII", "\xc3a", "?a"},
		{"invalid bytes", "\xff\xfe", "??"},
		{"truncated 3-byte", "\xe2\x82", "??"},
		{"truncated 4-byte", "x\xf0\x9f\x98", "x???"},
		{"truncated before valid", "\xe2\x82€", "??€"},
		{"surrogate", "\xed\xa0\x80", "???"},
		{"low surrogate", "\xed\xbf\xbf", "???"},
		{"overlong NUL", "\xc0\x80", "??"},
		{"overlong slash", "\xe0\x80\xaf", "???"},
		{"overlong 4-byte", "\xf0\x80\x80\x80", "????"},
		{"past U+10FFFF", "\xf4\x90\x80\x80", "????"},
		{"valid multi-byte around", "é\x00€\xff😀", "é?€?😀"},
		{"U+FFFD itself is valid", "�\x00", "�?"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := pangoText(tc.in)
			if got != tc.want || len(got) != len(tc.in) {
				t.Errorf("pangoText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
	for _, s := range []string{"", "hello", "é€😀 ok\n\ttab"} {
		in := strings.Clone(s)
		var got string
		if n := testing.AllocsPerRun(10, func() { got = pangoText(in) }); got != s || n != 0 {
			t.Errorf("pangoText(%q) = %q with %v allocations, want the same string, none", s, got, n)
		}
	}
}

// TestTextWrapWidth: no wrapping (-1) unless the width is positive and
// finite and fits Pango's int; else the width in Pango units, rounded up,
// at least 1.
func TestTextWrapWidth(t *testing.T) {
	const limit = float64(math.MaxInt32) / pangoScale // the largest width that fits
	for _, tc := range []struct {
		w    float64
		want int32
	}{
		{0, -1},
		{math.Copysign(0, -1), -1},
		{-1, -1},
		{-1e-300, -1},
		{math.NaN(), -1},
		{math.Inf(1), -1},
		{math.Inf(-1), -1},
		{1, 1024},
		{100, 102400},
		{10.5, 10752},
		{1.0 / 1024, 1},
		{1.5 / 1024, 1},     // floor, not round or ceil
		{2.99 / 1024, 2},    // floor
		{30 - 1e-12, 30720}, // an ulp-ish below a measured width still fits it (wrapSlack)
		{30 - 2e-6, 30719},  // past wrapSlack: rounded down
		{1.01 / 1024, 1},    // floor
		{1e-9, 1},
		{math.SmallestNonzeroFloat64, 1},
		{limit, math.MaxInt32},
		{limit + 1.0/pangoScale, -1},
		{float64(math.MaxInt32), -1},
		{1e300, -1},
	} {
		if got := wrapWidth(tc.w); got != tc.want {
			t.Errorf("wrapWidth(%v) = %d, want %d", tc.w, got, tc.want)
		}
	}
}

// TestTextLayoutMetrics: NewLayout measures the stand-in text in the
// resolved font at the wrap width; the layout keeps the original text, the
// font, the width, converts the metrics from Pango units, and answers
// LineCount and the queries from the geometry of Pango's lines and
// clusters (textgeom_test.go has the rules), with the break width of the
// font's size.
func TestTextLayoutMetrics(t *testing.T) {
	pl := &txLayouter{ui: "Cantarell", m: pangoMetrics{width: 51200, height: 3 * 16384, baseline: 12800}, cw: 10, lh: 16}
	ts, lg := txSystem(pl, nil, nil)
	font := render.Font{Size: 20, Weight: 700, Italic: true}
	text := "a\x00\nb\xff\nc"
	l := ts.NewLayout(text, font, 30)
	tl, ok := l.(*textLayout)
	if !ok {
		t.Fatalf("layout %T, want *textLayout", l)
	}
	want := txMeasure{"a?\nb?\nc", pangoFont{family: "Cantarell", size: 20 * pangoScale, weight: 700, italic: true}, 30 * pangoScale}
	if len(pl.measures) != 1 || pl.measures[0] != want {
		t.Errorf("measure calls %+v, want [%+v]", pl.measures, want)
	}
	if l.Text() != text || l.Font() != font || tl.maxWidth != 30 {
		t.Errorf("Text %q, Font %+v, maxWidth %v; want %q, %+v, 30", l.Text(), l.Font(), tl.maxWidth, text, font)
	}
	if got := l.Size(); got != (render.Size{W: 50, H: 48}) {
		t.Errorf("Size %v, want 50×48", got)
	}
	if got := l.Baseline(); got != 12.5 {
		t.Errorf("Baseline %v, want 12.5", got)
	}
	if got := l.LineCount(); got != 3 {
		t.Errorf("LineCount %d, want 3", got)
	}
	tgHit(t, l, 40, 30, 5, false) // right of the second line: before its "\n"
	tgHit(t, l, 4, 40, 6, false)
	tgHit(t, l, 14, 1, 1, false)
	for _, c := range []struct {
		i    int
		want render.Rect
	}{
		{0, rc(0, 0, 0, 16)}, {3, rc(0, 16, 0, 16)}, {5, rc(20, 16, 0, 16)},
		{100, rc(10, 32, 0, 16)}, {-1, rc(0, 0, 0, 16)},
	} {
		tgCaret(t, l, c.i, c.want)
	}
	tgSel(t, l, 0, 5, rc(0, 0, 20+10, 16), rc(0, 16, 20, 16)) // the "\n" half of 20 pt wide
	tgSel(t, l, 5, 0, rc(0, 0, 20+10, 16), rc(0, 16, 20, 16))
	tgSel(t, l, 7, 7)
	tgNorm(t, tl, 4, 4)
	tgNorm(t, tl, 100, 7)
	if len(lg.msgs) != 0 {
		t.Errorf("logged %q", lg.msgs)
	}

	// Pango units that are not whole points; no wrapping.
	pl.m = pangoMetrics{width: 1, height: 1536, baseline: 512}
	l = ts.NewLayout("x", render.Font{}, 0)
	if l.Size() != (render.Size{W: 1.0 / 1024, H: 1.5}) || l.Baseline() != 0.5 || l.LineCount() != 1 {
		t.Errorf("Size %v, Baseline %v, LineCount %d; want {1/1024 1.5}, 0.5, 1", l.Size(), l.Baseline(), l.LineCount())
	}
	if m := pl.measures[len(pl.measures)-1]; m.width != -1 || m.font.family != "Cantarell" || m.font.size != 13*pangoScale || m.font.weight != 400 {
		t.Errorf("measure %+v, want no wrapping at 13 pt regular Cantarell", m)
	}
}

// TestTextLayoutNoLines: lines and clusters that do not cover the text (no
// lines at all, or a last line short of the text's end) are a geometry
// error: the layout is a fake one and the error is logged once, as a
// measure error is; the next layout asks Pango again, and gets a real
// layout once the geometry is valid.
func TestTextLayoutNoLines(t *testing.T) {
	for _, tc := range []struct {
		name string
		pl   *txLayouter
	}{
		{"no lines", &txLayouter{m: pangoMetrics{height: 16 * pangoScale}, noGeom: true}},
		{"short last line", &txLayouter{m: pangoMetrics{height: 16 * pangoScale,
			lines: []pangoLine{{start: 0, length: 1}}, clusters: []pangoCluster{{start: 0}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pl := tc.pl
			ts, lg := txSystem(pl, nil, nil)
			for range 2 {
				txIsFake(t, ts.NewLayout("ab", render.Font{}, 0), "ab", render.Font{}, 0)
			}
			if len(pl.measures) != 2 {
				t.Errorf("measure called %d times, want 2 (retried)", len(pl.measures))
			}
			if len(lg.msgs) != 1 || !strings.Contains(lg.msgs[0], errGeometry.Error()) {
				t.Errorf("logged %q, want one message with %q", lg.msgs, errGeometry)
			}
			pl.m, pl.noGeom, pl.lh = pangoMetrics{height: 16 * pangoScale}, false, 16
			l, ok := ts.NewLayout("ab", render.Font{}, 0).(*textLayout)
			if !ok || l.LineCount() != 1 || l.CaretRect(0) != (render.Rect{H: 16}) {
				t.Errorf("layout once the geometry is valid: %v, want a Pango layout of one line", l)
			}
			if len(lg.msgs) != 1 {
				t.Errorf("logged %q, want still one message", lg.msgs)
			}
		})
	}
}

// TestTextNoPango: without open (the shell on the host), layouts are fake
// ones and nothing is logged.
func TestTextNoPango(t *testing.T) {
	ts := newTextSystem(mainthread.New())
	lg := &txLogs{}
	ts.logf = lg.logf
	for _, tc := range []struct {
		text string
		w    float64
	}{{"hello world", 0}, {"hello world", 30}, {"a\nb", math.NaN()}} {
		txIsFake(t, ts.NewLayout(tc.text, render.Font{Size: 20}, tc.w), tc.text, render.Font{Size: 20}, tc.w)
	}
	if len(lg.msgs) != 0 {
		t.Errorf("logged %q, want nothing", lg.msgs)
	}
	ts.release() // harmless without Pango
}

// TestTextOpenFails: when Pango does not open, layouts are fake ones, the
// failure is logged once and open is not tried again.
func TestTextOpenFails(t *testing.T) {
	opens := 0
	errOpen := errors.New("no libpangocairo")
	ts, lg := txSystem(nil, errOpen, &opens)
	for range 3 {
		txIsFake(t, ts.NewLayout("abc", render.Font{}, 0), "abc", render.Font{}, 0)
	}
	if opens != 1 {
		t.Errorf("open called %d times, want once", opens)
	}
	if len(lg.msgs) != 1 || !strings.Contains(lg.msgs[0], errOpen.Error()) {
		t.Errorf("logged %q, want one message with %q", lg.msgs, errOpen)
	}
	ts.release()
}

// TestTextMeasureFails: a failing measure gives a fake layout and is
// logged once; the next layout asks Pango again, and gets a real layout
// once measure works.
func TestTextMeasureFails(t *testing.T) {
	opens := 0
	errMeasure := errors.New("measure failed")
	pl := &txLayouter{err: errMeasure, m: pangoMetrics{width: 1024, height: 1024}}
	ts, lg := txSystem(pl, nil, &opens)
	for range 2 {
		txIsFake(t, ts.NewLayout("abc", render.Font{}, 0), "abc", render.Font{}, 0)
	}
	if len(pl.measures) != 2 {
		t.Errorf("measure called %d times, want 2 (retried)", len(pl.measures))
	}
	if len(lg.msgs) != 1 || !strings.Contains(lg.msgs[0], errMeasure.Error()) {
		t.Errorf("logged %q, want one message with %q", lg.msgs, errMeasure)
	}
	pl.err = nil
	if l := ts.NewLayout("abc", render.Font{}, 0); l.Size() != (render.Size{W: 1, H: 1}) {
		t.Errorf("layout once measure works: %T %v, want Pango's 1×1", l, l.Size())
	}
	pl.err = errMeasure
	ts.NewLayout("abc", render.Font{}, 0)
	if opens != 1 || pl.uiCalls != 1 || len(lg.msgs) != 1 {
		t.Errorf("open %d, uiFamily %d, logged %d; want 1, 1, 1", opens, pl.uiCalls, len(lg.msgs))
	}
}

// TestTextRelease: release frees the layouter once; later layouts are fake
// ones and do not call it; releasing again, or before the first layout,
// is harmless.
func TestTextRelease(t *testing.T) {
	opens := 0
	pl := &txLayouter{m: pangoMetrics{width: 1024, height: 1024}}
	ts, lg := txSystem(pl, nil, &opens)
	if _, ok := ts.NewLayout("a", render.Font{Family: "X"}, 0).(*textLayout); !ok {
		t.Fatal("no Pango layout before release")
	}
	ts.release()
	ts.release()
	if pl.releases != 1 {
		t.Errorf("layouter released %d times, want once", pl.releases)
	}
	txIsFake(t, ts.NewLayout("abc", render.Font{Family: "X"}, 10), "abc", render.Font{Family: "X"}, 10)
	if pl.afterRelease != 0 || opens != 1 || len(lg.msgs) != 0 {
		t.Errorf("after release: %d layouter calls, %d opens, logged %q; want 0, 1, nothing", pl.afterRelease, opens, lg.msgs)
	}

	// Before the first layout: nothing opened, nothing released, fake
	// layouts after.
	opens = 0
	pl = &txLayouter{}
	ts, lg = txSystem(pl, nil, &opens)
	ts.release()
	txIsFake(t, ts.NewLayout("abc", render.Font{}, 0), "abc", render.Font{}, 0)
	if opens != 0 || pl.releases != 0 || len(pl.measures) != 0 || len(lg.msgs) != 0 {
		t.Errorf("release before init: %d opens, %d releases, %d measures, logged %q; want none",
			opens, pl.releases, len(pl.measures), lg.msgs)
	}
}

// TestTextShell: the shell's TextSystem is its *textSystem, which has no
// Pango over the fake loop and so gives fake layouts.
func TestTextShell(t *testing.T) {
	s, _ := newTestShell(t)
	err := run(t, s, &handler{ready: func() {
		ts, ok := s.TextSystem().(*textSystem)
		if !ok || ts != s.text {
			t.Errorf("TextSystem() = %T, want the shell's *textSystem", s.TextSystem())
		}
		txIsFake(t, s.TextSystem().NewLayout("hi there", render.Font{}, 20), "hi there", render.Font{}, 20)
		s.Post(s.Quit)
	}})
	if err != nil {
		t.Fatal(err)
	}
}

// TestTextShellRunEnd: the end of Run (shutdown) releases the text
// system, also after a panic; later layouts are fake ones.
func TestTextShellRunEnd(t *testing.T) {
	for _, panics := range []bool{false, true} {
		t.Run(fmt.Sprint("panic=", panics), func(t *testing.T) {
			s, l := newTestShell(t)
			base := dispatch.Len()
			pl := &txLayouter{m: pangoMetrics{width: 1024, height: 1024}}
			s.text.open = func() (pangoLayouter, error) { return pl, nil }
			lg := &txLogs{}
			s.text.logf = lg.logf
			ready := func() {
				if _, ok := s.TextSystem().NewLayout("a", render.Font{}, 0).(*textLayout); !ok {
					t.Error("no Pango layout in Ready")
				}
				if panics {
					panic("in ready")
				}
				s.Post(s.Quit)
			}
			if panics {
				if got := runPanics(t, s, &handler{ready: ready}); got != "in ready" {
					t.Errorf("Run panicked with %v", got)
				}
			} else if err := run(t, s, &handler{ready: ready}); err != nil {
				t.Fatal(err)
			}
			if pl.releases != 1 || s.text.pl != nil {
				t.Errorf("after Run: layouter released %d times, held %v; want once, nil", pl.releases, s.text.pl)
			}
			s.Queue().SetChecks(false) // Run is over: no UI thread to be on
			txIsFake(t, s.text.NewLayout("a", render.Font{}, 0), "a", render.Font{}, 0)
			if pl.afterRelease != 0 || len(lg.msgs) != 0 {
				t.Errorf("after Run: %d layouter calls, logged %q", pl.afterRelease, lg.msgs)
			}
			checkTornDown(t, s, l, base)
		})
	}
}
