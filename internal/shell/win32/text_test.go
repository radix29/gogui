//go:build windows

package win32

import (
	"math"
	"strings"
	"testing"

	"github.com/radix29/gogui/assets/fonts"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/mainthread"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/fake"
)

// newTestText returns a text system with no embedded fonts (whatever
// assets/fonts holds; textfonts_test.go tests those) whose DirectWrite
// objects are released, with no references left, when the test ends.
func newTestText(t *testing.T) *textSystem {
	t.Helper()
	ts := newTextSystem(mainthread.New())
	ts.fontFiles = func() []fonts.File { return nil }
	t.Cleanup(func() {
		ts.release(func(what string, left uint32) { t.Errorf("%s: %d references left", what, left) })
	})
	return ts
}

func mustDW(t *testing.T, ts *textSystem) {
	t.Helper()
	if err := ts.init(); err != nil {
		t.Fatal(err)
	}
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestTextBasics(t *testing.T) {
	ts := newTestText(t)
	f := render.Font{Size: 13}
	l := ts.NewLayout("Hello", f, 0)
	if _, ok := l.(*layout); !ok {
		t.Fatalf("layout is %T, want DirectWrite's", l)
	}
	sz := l.Size()
	if sz.W < 20 || sz.W > 45 || !near(sz.H, 13*1.33, 1.5) {
		t.Errorf("Hello at 13 pt: %v, want about 30×17", sz)
	}
	if l.Text() != "Hello" || l.Font() != f || l.LineCount() != 1 {
		t.Errorf("Text %q, Font %+v, LineCount %d", l.Text(), l.Font(), l.LineCount())
	}
	if b := l.Baseline(); b <= 0 || b >= sz.H {
		t.Errorf("Baseline %v not inside the height %v", b, sz.H)
	}
	// A zero size means 13 pt.
	if z := ts.NewLayout("Hello", render.Font{}, 0).Size(); z != sz {
		t.Errorf("zero Font: %v, want %v", z, sz)
	}
	// Twice the size, twice the width and height (±hinting).
	big := ts.NewLayout("Hello", render.Font{Size: 26}, 0).Size()
	if !near(big.W, 2*sz.W, 2) || !near(big.H, 2*sz.H, 2) {
		t.Errorf("26 pt: %v, want about %v", big, render.Size{W: 2 * sz.W, H: 2 * sz.H})
	}
	// Same layout, same numbers.
	if ts.NewLayout("Hello", f, 0).Size() != sz {
		t.Error("measurement is not repeatable")
	}
}

func TestTextEmpty(t *testing.T) {
	ts := newTestText(t)
	hello := ts.NewLayout("Hello", render.Font{}, 0)
	for _, w := range []float64{0, 50} {
		l := ts.NewLayout("", render.Font{}, w)
		if sz := l.Size(); sz.W != 0 || sz.H != hello.Size().H || l.LineCount() != 1 {
			t.Errorf("empty (width %v): %v, %d lines; want 0×%v, 1", w, sz, l.LineCount(), hello.Size().H)
		}
		if l.Baseline() != hello.Baseline() {
			t.Errorf("empty baseline %v, want %v", l.Baseline(), hello.Baseline())
		}
	}
	// Empty text: every point hits 0, the caret is at 0 one line high, and
	// nothing can be selected (TestHitEmpty has the rest).
	e := ts.NewLayout("", render.Font{}, 0)
	if i, tr := e.HitTest(render.Point{X: 5, Y: 5}); i != 0 || tr {
		t.Errorf("empty HitTest = %d, %v", i, tr)
	}
	if e.SelectionRects(0, 3) != nil {
		t.Error("empty SelectionRects not empty")
	}
}

func TestTextLines(t *testing.T) {
	ts := newTestText(t)
	one := ts.NewLayout("Hello", render.Font{}, 0)
	h := one.Size().H
	two := ts.NewLayout("Hello\nWorld", render.Font{}, 0)
	if two.LineCount() != 2 || !near(two.Size().H, 2*h, 0.01) {
		t.Errorf("Hello\\nWorld: %d lines, height %v, want 2, %v", two.LineCount(), two.Size().H, 2*h)
	}
	// The widest line gives the width.
	wide := ts.NewLayout("Hi\nHello there\nyo", render.Font{}, 0)
	if want := ts.NewLayout("Hello there", render.Font{}, 0).Size().W; wide.Size().W != want || wide.LineCount() != 3 {
		t.Errorf("widest line: width %v (want %v), %d lines", wide.Size().W, want, wide.LineCount())
	}
	// A trailing newline starts an empty line; blank lines count.
	if l := ts.NewLayout("a\n", render.Font{}, 0); l.LineCount() != 2 {
		t.Errorf("a\\n: %d lines, want 2", l.LineCount())
	}
	if l := ts.NewLayout("\n\n", render.Font{}, 0); l.LineCount() != 3 || l.Size().W != 0 {
		t.Errorf("\\n\\n: %d lines, width %v; want 3, 0", l.LineCount(), l.Size().W)
	}
	// \r\n is one break.
	if l := ts.NewLayout("a\r\nb", render.Font{}, 0); l.LineCount() != 2 {
		t.Errorf("a\\r\\nb: %d lines, want 2", l.LineCount())
	}
}

func TestTextWrap(t *testing.T) {
	ts := newTestText(t)
	f := render.Font{}
	const s = "hello world again"
	full := ts.NewLayout(s, f, 0)
	hello := ts.NewLayout("hello", f, 0).Size().W
	const twice = "hello hello"
	world := ts.NewLayout("hello world", f, 0).Size().W

	// A width that fits "hello world" but not all: wraps at a space.
	w := (world + full.Size().W) / 2
	l := ts.NewLayout(s, f, w)
	if l.LineCount() != 2 || l.Size().W > w || l.Size().W < world-0.01 {
		t.Errorf("wrap at %v: %d lines, width %v; want 2 lines, width in [%v, %v]", w, l.LineCount(), l.Size().W, world, w)
	}
	if !near(l.Size().H, 2*full.Size().H, 0.01) {
		t.Errorf("wrapped height %v, want %v", l.Size().H, 2*full.Size().H)
	}
	// Trailing whitespace at a wrap point does not count: "hello " fits in
	// a width just over "hello".
	l = ts.NewLayout(twice, f, hello+1)
	if l.LineCount() != 2 || !near(l.Size().W, hello, 0.01) {
		t.Errorf("hanging space: %d lines, width %v; want 2, %v", l.LineCount(), l.Size().W, hello)
	}
	// A word longer than the width breaks between clusters.
	long := strings.Repeat("W", 30)
	longW := ts.NewLayout(long, f, 0).Size().W
	l = ts.NewLayout(long, f, longW/3)
	if l.LineCount() < 3 || l.Size().W > longW/3+0.01 {
		t.Errorf("long word: %d lines, width %v (limit %v)", l.LineCount(), l.Size().W, longW/3)
	}
	// A width smaller than any glyph still gives one cluster per line.
	if l = ts.NewLayout("abc", f, 0.5); l.LineCount() != 3 {
		t.Errorf("tiny width: %d lines, want 3", l.LineCount())
	}
	// \n lines wrap separately.
	if l = ts.NewLayout(twice+"\n"+twice, f, hello+1); l.LineCount() != 4 {
		t.Errorf("wrapped \\n lines: %d, want 4", l.LineCount())
	}
	// Wider than the text: no change.
	if l = ts.NewLayout(s, f, 1000); l.LineCount() != 1 || l.Size() != full.Size() {
		t.Errorf("wide box: %d lines, %v", l.LineCount(), l.Size())
	}
}

func TestTextNoWrapWidths(t *testing.T) {
	ts := newTestText(t)
	const s = "a fairly long line of text that would wrap in a narrow box"
	want := ts.NewLayout(s, render.Font{}, 0)
	if want.LineCount() != 1 {
		t.Fatalf("unwrapped: %d lines", want.LineCount())
	}
	for _, w := range []float64{0, -1, -1e9, math.NaN(), math.Inf(1), math.Inf(-1), 1e300} {
		l := ts.NewLayout(s, render.Font{}, w)
		if l.LineCount() != 1 || l.Size() != want.Size() {
			t.Errorf("maxWidth %v: %d lines, %v; want 1, %v", w, l.LineCount(), l.Size(), want.Size())
		}
	}
	// A positive width wraps, however small or odd.
	if l := ts.NewLayout(s, render.Font{}, 1e-9); l.LineCount() <= 1 {
		t.Errorf("tiny positive width: %d lines", l.LineCount())
	}
}

func TestTextFonts(t *testing.T) {
	ts := newTestText(t)
	const s = "Hello, World 0123"
	w := func(f render.Font) float64 { return ts.NewLayout(s, f, 0).Size().W }
	base := w(render.Font{})
	if bold := w(render.Font{Weight: driver.FontWeightBold}); bold <= base {
		t.Errorf("bold %v not wider than regular %v", bold, base)
	}
	// Segoe UI Variable has no italic face (DirectWrite slants it without
	// changing the advances), so Times New Roman shows the style.
	times := render.Font{Family: "Times New Roman"}
	if it := w(render.Font{Family: times.Family, Italic: true}); it == w(times) {
		t.Errorf("Times New Roman italic %v same as regular", it)
	}
	if w(render.Font{Weight: driver.FontWeightRegular}) != base {
		t.Error("explicit regular weight differs from the zero weight")
	}
	// An unknown family falls back to the system UI font.
	if got := w(render.Font{Family: "No Such Family 12345"}); got != base {
		t.Errorf("unknown family: %v, want the UI font's %v", got, base)
	}
	if got := w(render.Font{Family: "Arial"}); got == base {
		t.Errorf("Arial: %v, same as the UI font", got)
	}
	// Case-insensitive family names (DirectWrite's lookup).
	if w(render.Font{Family: "arial"}) != w(render.Font{Family: "Arial"}) {
		t.Error("family lookup is case-sensitive")
	}
	// Absurd sizes and weights are clamped, not refused.
	for _, f := range []render.Font{{Size: -5}, {Size: 1e9}, {Weight: 5000}, {Weight: -3}, {Size: math.NaN()}} {
		l := ts.NewLayout(s, f, 0)
		if _, ok := l.(*layout); !ok {
			t.Errorf("%+v fell back to the fake metrics", f)
		}
		if l.Size().H <= 0 || math.IsNaN(l.Size().W) {
			t.Errorf("%+v: size %v", f, l.Size())
		}
	}
}

func TestTextMonospaced(t *testing.T) {
	ts := newTestText(t)
	mono := render.Font{Monospaced: true}
	ref := ts.NewLayout("i", mono, 0).Size().W
	if ref <= 0 {
		t.Fatalf("width of i = %v", ref)
	}
	for _, s := range []string{"W", "m", "0", ".", "iiiiii", "WWWWWW", "a b"} {
		want := ref * float64(len([]rune(s)))
		if got := ts.NewLayout(s, mono, 0).Size().W; !near(got, want, 0.01) {
			t.Errorf("%q: width %v, want %v", s, got, want)
		}
	}
	// Monospaced is not the proportional UI font, and a Family wins over it.
	if ts.NewLayout("iiiiWWWW", mono, 0).Size().W == ts.NewLayout("iiiiWWWW", render.Font{}, 0).Size().W {
		t.Error("monospaced text measures like the UI font")
	}
	if a, b := ts.NewLayout("iiiiWWWW", render.Font{Family: "Arial", Monospaced: true}, 0).Size().W,
		ts.NewLayout("iiiiWWWW", render.Font{Family: "Arial"}, 0).Size().W; a != b {
		t.Errorf("Family with Monospaced: %v, without: %v", a, b)
	}
}

func TestTextResolve(t *testing.T) {
	ts := newTestText(t)
	mustDW(t, ts)
	if ts.uiFamily != familyUIWin11 && ts.uiFamily != familyUIWin10 {
		t.Errorf("UI family %q", ts.uiFamily)
	}
	cases := []struct {
		f      render.Font
		family string
	}{
		{render.Font{}, ts.uiFamily},
		{render.Font{Family: "nonexistent family"}, ts.uiFamily},
		{render.Font{Monospaced: true}, familyMono},
		{render.Font{Family: "Arial"}, "Arial"},
	}
	for _, c := range cases {
		if k := ts.resolve(c.f); k.family != c.family || k.embedded || k.size != 13 || k.weight != 400 {
			t.Errorf("resolve(%+v) = %+v, want family %q at 13/400", c.f, k, c.family)
		}
	}
	if k := ts.resolve(render.Font{Size: 11, Weight: driver.FontWeightSemibold, Italic: true}); k.size != 11 || k.weight != 600 || !k.italic {
		t.Errorf("size/weight/italic: %+v", k)
	}
}

// TestTextFormatCache: one format per resolved font, shared by layouts, and
// the cache is bounded.
func TestTextFormatCache(t *testing.T) {
	ts := newTestText(t)
	ts.NewLayout("a", render.Font{}, 0)
	ts.NewLayout("b", render.Font{}, 30)
	ts.NewLayout("c", render.Font{Size: 13}, 0)
	if len(ts.formats) != 1 {
		t.Errorf("%d formats for one font, want 1", len(ts.formats))
	}
	ts.NewLayout("d", render.Font{Size: 14}, 0)
	ts.NewLayout("e", render.Font{Weight: driver.FontWeightBold}, 0)
	if len(ts.formats) != 3 {
		t.Errorf("%d formats for three fonts, want 3", len(ts.formats))
	}
	// The wrapping set on a layout does not leak into the shared format:
	// an unwrapped layout after a wrapped one stays on one line.
	ts.NewLayout("hello world", render.Font{}, 10)
	if l := ts.NewLayout("hello world", render.Font{}, 0); l.LineCount() != 1 {
		t.Errorf("unwrapped layout after a wrapped one: %d lines", l.LineCount())
	}
	for i := range 2 * maxFormats {
		ts.NewLayout("x", render.Font{Size: 5 + float64(i)/4}, 0)
	}
	if n := len(ts.formats); n == 0 || n > maxFormats {
		t.Errorf("%d formats after many sizes, want 1…%d", n, maxFormats)
	}
}

func TestTextUnicode(t *testing.T) {
	ts := newTestText(t)
	f := render.Font{}
	for _, s := range []string{"é€😀", "é", "日本語のテキスト", "שלום", "a\xffb"} {
		l := ts.NewLayout(s, f, 0)
		if _, ok := l.(*layout); !ok || l.Text() != s || l.Size().W <= 0 || l.LineCount() != 1 {
			t.Errorf("%q: %T, size %v, %d lines", s, l, l.Size(), l.LineCount())
		}
	}
	// A combining mark adds no width.
	if a, b := ts.NewLayout("e", f, 0).Size().W, ts.NewLayout("é", f, 0).Size().W; !near(a, b, 0.5) {
		t.Errorf("e: %v, e+U+0301: %v", a, b)
	}
	// An invalid byte measures as U+FFFD, and the text is kept as given.
	if a, b := ts.NewLayout("\xff", f, 0).Size().W, ts.NewLayout("�", f, 0).Size().W; a != b {
		t.Errorf("invalid byte %v, U+FFFD %v", a, b)
	}
	if l := ts.NewLayout("a\xffb", f, 0); l.Text() != "a\xffb" {
		t.Errorf("Text() = %q", l.Text())
	}
	// A very long text.
	if l := ts.NewLayout(strings.Repeat("lorem ipsum ", 5000), f, 400); l.LineCount() < 100 {
		t.Errorf("long text: %d lines", l.LineCount())
	}
}

// TestTextNoGrowth: many layouts leave the shared system collection's
// reference count where the first ones put it (DirectWrite's caches hold a
// few) once the drawn-layout cache, which holds at most maxDrawn of their
// native layouts, is emptied, and the format cache holds one reference per
// format.
func TestTextNoGrowth(t *testing.T) {
	ts := newTestText(t)
	for i := range 3 {
		ts.NewLayout("warm up", render.Font{Size: 13 + float64(i)}, 50)
		ts.NewLayout("warm up", render.Font{Monospaced: true}, 0)
	}
	noLeak := func(what string, n uint32) { t.Errorf("%s: %d references left", what, n) }
	ts.releaseDrawn(noLeak)
	before := ts.system.RefCount()
	for range 200 {
		ts.NewLayout("Hello, World\nsecond line", render.Font{}, 60)
		ts.NewLayout("Monospaced", render.Font{Monospaced: true}, 0)
	}
	if n := len(ts.drawn.m); n != maxDrawn {
		t.Errorf("%d native layouts cached, want %d", n, maxDrawn)
	}
	ts.releaseDrawn(noLeak)
	if after := ts.system.RefCount(); after != before {
		t.Errorf("system collection references: %d after 400 layouts, %d before", after, before)
	}
}

// TestTextFallback: after release (or when DirectWrite fails) layouts come
// from the fake metrics instead of failing.
func TestTextFallback(t *testing.T) {
	ts := newTextSystem(mainthread.New())
	if _, ok := ts.NewLayout("abc", render.Font{}, 0).(*layout); !ok {
		t.Fatal("no DirectWrite layout")
	}
	ts.release(func(what string, left uint32) { t.Errorf("%s: %d references left", what, left) })
	l := ts.NewLayout("abcd", render.Font{}, 0)
	if _, ok := l.(*layout); ok {
		t.Error("layout after release is DirectWrite's")
	}
	if want := fake.NewTextSystem().NewLayout("abcd", render.Font{}, 0).Size(); l.Size() != want {
		t.Errorf("fallback size %v, want %v", l.Size(), want)
	}
	ts.release(nil) // twice is fine
}

// TestTextUIThread: with the checks on, a layout off the UI thread panics.
func TestTextUIThread(t *testing.T) {
	q := mainthread.New()
	q.SetChecks(true)
	ts := newTextSystem(q)
	mustPanic(t, "NewLayout on an unbound queue", func() { ts.NewLayout("a", render.Font{}, 0) })
}

// TestShellText: the shell's text system is the DirectWrite one, the aqua
// title bar's font lays out, and the system is released with the shell.
func TestShellText(t *testing.T) {
	inShell(t, func(s *Shell) {
		ts := s.TextSystem()
		if _, ok := ts.NewLayout("Title", render.Font{Size: 13, Weight: driver.FontWeightSemibold}, 0).(*layout); !ok {
			t.Errorf("shell text system gives %T", ts.NewLayout("x", render.Font{}, 0))
		}
		if !s.text.inited {
			t.Error("text system not started after a layout")
		}
	})
}

// TestTextResolveMemo (review P17): a family is looked up in the font
// collections once; the next layouts in it, at any size, weight or style,
// make no lookups.
func TestTextResolveMemo(t *testing.T) {
	ts := newTestText(t)
	mustDW(t, ts)
	fonts := []render.Font{{}, {Family: "Arial"}, {Family: "nonexistent family"}, {Monospaced: true}}
	for _, f := range fonts {
		ts.NewLayout("x", f, 0)
	}
	first := ts.lookups
	for _, f := range fonts {
		g := f
		g.Size, g.Weight, g.Italic = 20, driver.FontWeightBold, true
		ts.NewLayout("y", g, 0)
		memo := ts.resolve(g)
		if fresh := ts.resolveFamily(familyQuery{g.Family, g.Monospaced}); fresh.family != memo.family || fresh.embedded != memo.embedded {
			t.Errorf("resolve(%+v): memo %+v, fresh %+v", g, memo, fresh)
		}
	}
	if first == 0 {
		t.Error("no lookups for the first layouts")
	}
	before := ts.lookups
	for _, f := range fonts {
		ts.NewLayout("z", render.Font{Family: f.Family, Monospaced: f.Monospaced, Size: 17}, 0)
	}
	if n := ts.lookups - before; n != 0 {
		t.Errorf("%d lookups for families already resolved, want 0", n)
	}
}
