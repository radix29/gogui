//go:build linux

package gtk4

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/mainthread"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// The text system over real Pango (S4.2g): the size and wrap contract of
// S2.5d's Win32 tests (shell/win32/text_test.go), fonts, the byte
// stand-ins, and the shell's text system in Ready. Measuring needs only
// gtk.Load (Pango and the default PangoCairo font map), not a display, so
// these run, on the GTK goroutine (onGTK), wherever the GTK libraries are
// installed; without a display the GtkSettings font name is not read (the
// UI family is then sans-serif). Only TestTextPangoShell needs a display.
// Measured values are logged with the prefix TEXT-RESULT.
//
// Pango's results depend on the installed fonts, so the checks are on the
// contract, not on numbers; where a check needs distinct faces (a bold, an
// italic with other advances) and fontconfig has none, it is skipped,
// with what was resolved logged.

// tlEps is the tolerance of width comparisons: sizes are whole Pango units
// (1/1024 px), so equal layouts give equal numbers.
const tlEps = 1e-6

// tlNoSettings is pangoNative without the GtkSettings lookup: without
// gtk_init (no display), GTK is not called.
type tlNoSettings struct{ pangoLayouter }

func (tlNoSettings) uiFamily() string { return "" }

// tlEnv is what a tlPango function gets: the text system, opened, and a
// way to skip (t.Skip may not run on the GTK goroutine).
type tlEnv struct {
	t    *testing.T
	ts   *textSystem
	skip string
}

func (e *tlEnv) skipf(format string, args ...any) { e.skip = fmt.Sprintf(format, args...) }

// lay is ts.NewLayout, failing the test when it is not a Pango layout.
func (e *tlEnv) lay(text string, f render.Font, maxWidth float64) render.TextLayout {
	e.t.Helper()
	l := e.ts.NewLayout(text, f, maxWidth)
	if _, ok := l.(*textLayout); !ok {
		e.t.Errorf("NewLayout(%q, %+v, %v) is a %T, not a Pango layout", text, f, maxWidth, l)
	}
	return l
}

// width is the unwrapped width of text in f.
func (e *tlEnv) width(text string, f render.Font) float64 {
	e.t.Helper()
	return e.lay(text, f, 0).Size().W
}

// native is the text system's pangoNative, nil if it has none.
func (e *tlEnv) native() *pangoNative {
	switch p := e.ts.pl.(type) {
	case *pangoNative:
		return p
	case tlNoSettings:
		return p.pangoLayouter.(*pangoNative)
	}
	return nil
}

// tlSystem returns a text system over Pango; without an initialised GTK
// the GtkSettings font name is not read. Failures it logs go to logs.
func tlSystem(logs *[]string) *textSystem {
	ts := newTextSystem(mainthread.New())
	ts.open = func() (pangoLayouter, error) {
		pl, err := openPango()
		if err != nil || gtkInit == nil {
			return pl, err
		}
		return tlNoSettings{pl}, nil
	}
	ts.logf = func(format string, args ...any) { *logs = append(*logs, fmt.Sprintf(format, args...)) }
	return ts
}

// tlPango runs f on the GTK goroutine with a fresh text system over Pango,
// released afterwards. It skips the test when the GTK libraries are
// missing, and fails it when the text system logged a failure (a layout
// fell back to fixed-width metrics).
func tlPango(t *testing.T, f func(e *tlEnv)) {
	t.Helper()
	if err := gtk.Load(); err != nil {
		t.Skipf("GTK 4 or Pango cannot be loaded: %v", err)
	}
	e := &tlEnv{t: t}
	onGTK(t, func() error {
		var logs []string
		e.ts = tlSystem(&logs)
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
	if e.skip != "" {
		t.Skip(e.skip)
	}
}

func tlNear(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// tlFinite reports whether every value is finite.
func tlFinite(vs ...float64) bool {
	for _, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// TestTextPangoBasics: "Hello" at 13 pt is a Pango layout of a plausible
// size with the baseline inside it; the zero font is 13 pt, 26 pt is about
// twice as large, and the size depends only on text, font and width (the
// same twice, after other layouts, and from a second text system).
func TestTextPangoBasics(t *testing.T) {
	var sz render.Size
	tlPango(t, func(e *tlEnv) {
		f := render.Font{Size: 13}
		l := e.lay("Hello", f, 0)
		sz = l.Size()
		t.Logf("TEXT-RESULT UI family %q (resolved %+v); Pango %d", e.ts.ui, e.ts.resolve(f), gtk.Pango.Version())
		t.Logf("TEXT-RESULT Hello 13pt: %v, baseline %v", sz, l.Baseline())
		if sz.W < 15 || sz.W > 60 || sz.H < 13 || sz.H > 26 || !tlFinite(sz.W, sz.H) {
			t.Errorf("Hello at 13 pt: %v, want about 30×16", sz)
		}
		if l.Text() != "Hello" || l.Font() != f || l.LineCount() != 1 {
			t.Errorf("Text %q, Font %+v, LineCount %d; want Hello, %+v, 1", l.Text(), l.Font(), l.LineCount(), f)
		}
		if b := l.Baseline(); !(b > 0 && b < sz.H) {
			t.Errorf("Baseline %v not inside (0, %v)", b, sz.H)
		}
		if z := e.lay("Hello", render.Font{}, 0).Size(); z != sz {
			t.Errorf("zero Font: %v, want the 13 pt %v", z, sz)
		}
		big := e.lay("Hello", render.Font{Size: 26}, 0).Size()
		t.Logf("TEXT-RESULT Hello 26pt: %v", big)
		if !tlNear(big.W, 2*sz.W, 2) || !tlNear(big.H, 2*sz.H, 2) {
			t.Errorf("26 pt: %v, want about %v", big, render.Size{W: 2 * sz.W, H: 2 * sz.H})
		}
		e.lay("Something else entirely, wrapped", render.Font{Size: 17, Weight: driver.FontWeightBold}, 40)
		if again := e.lay("Hello", f, 0); again.Size() != sz || again.Baseline() != l.Baseline() {
			t.Errorf("measured again: %v, baseline %v; want %v, %v", again.Size(), again.Baseline(), sz, l.Baseline())
		}
	})
	if t.Failed() || t.Skipped() {
		return
	}
	tlPango(t, func(e *tlEnv) {
		if got := e.lay("Hello", render.Font{Size: 13}, 0).Size(); got != sz {
			t.Errorf("a second text system: %v, want %v", got, sz)
		}
	})
}

// TestTextPangoEmpty: empty text is one line of the font's height, zero
// wide, with the baseline of non-empty text, wrapped or not.
func TestTextPangoEmpty(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		hello := e.lay("Hello", render.Font{}, 0)
		for _, w := range []float64{0, 50} {
			l := e.lay("", render.Font{}, w)
			sz := l.Size()
			t.Logf("TEXT-RESULT empty (width %v): %v, baseline %v; Hello %v, baseline %v", w, sz, l.Baseline(), hello.Size(), hello.Baseline())
			if sz.W != 0 || !tlNear(sz.H, hello.Size().H, 0.01) || l.LineCount() != 1 {
				t.Errorf("empty (width %v): %v, %d lines; want 0×%v, 1", w, sz, l.LineCount(), hello.Size().H)
			}
			if !tlNear(l.Baseline(), hello.Baseline(), 0.01) {
				t.Errorf("empty (width %v) baseline %v, want %v", w, l.Baseline(), hello.Baseline())
			}
			if l.Text() != "" {
				t.Errorf("empty Text() = %q", l.Text())
			}
		}
	})
}

// TestTextPangoLines: \n starts a line (a trailing one an empty line, \n\n
// three empty lines), \r\n is one break, the widest line gives the width,
// and the baseline is the first line's.
func TestTextPangoLines(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		f := render.Font{}
		one := e.lay("Hello", f, 0)
		h := one.Size().H
		two := e.lay("Hello\nWorld", f, 0)
		t.Logf("TEXT-RESULT Hello\\nWorld: %v, %d lines, baseline %v", two.Size(), two.LineCount(), two.Baseline())
		if two.LineCount() != 2 || !tlNear(two.Size().H, 2*h, 0.01) {
			t.Errorf("Hello\\nWorld: %d lines, height %v; want 2, %v", two.LineCount(), two.Size().H, 2*h)
		}
		if !tlNear(two.Baseline(), one.Baseline(), 0.01) {
			t.Errorf("Hello\\nWorld baseline %v, want the first line's %v", two.Baseline(), one.Baseline())
		}
		wide := e.lay("Hi\nHello there\nyo", f, 0)
		if want := e.width("Hello there", f); !tlNear(wide.Size().W, want, tlEps) || wide.LineCount() != 3 {
			t.Errorf("widest line: width %v (want %v), %d lines (want 3)", wide.Size().W, want, wide.LineCount())
		}
		if l := e.lay("a\n", f, 0); l.LineCount() != 2 || !tlNear(l.Size().H, 2*h, 0.01) {
			t.Errorf("a\\n: %d lines, height %v; want 2, %v", l.LineCount(), l.Size().H, 2*h)
		}
		if l := e.lay("\n\n", f, 0); l.LineCount() != 3 || l.Size().W != 0 || !tlNear(l.Size().H, 3*h, 0.01) {
			t.Errorf("\\n\\n: %d lines, %v; want 3, 0×%v", l.LineCount(), l.Size(), 3*h)
		}
		if l := e.lay("a\r\nb", f, 0); l.LineCount() != 2 {
			t.Errorf("a\\r\\nb: %d lines, want 2", l.LineCount())
		}
	})
}

// TestTextPangoWrap: a width between two words wraps at the space; a
// trailing space does not push a line past the width; a word longer than
// the width breaks inside (PANGO_WRAP_WORD_CHAR); a width under one glyph
// gives one glyph per line; \n lines wrap separately; a wide box changes
// nothing.
func TestTextPangoWrap(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		f := render.Font{}
		const s = "hello world again"
		full := e.lay(s, f, 0)
		hello := e.width("hello", f)
		world := e.width("hello world", f)

		w := (world + full.Size().W) / 2
		l := e.lay(s, f, w)
		t.Logf("TEXT-RESULT %q at %v: %v, %d lines (unwrapped %v)", s, w, l.Size(), l.LineCount(), full.Size())
		if l.LineCount() != 2 || l.Size().W > w+tlEps || l.Size().W < world-0.01 {
			t.Errorf("wrap at %v: %d lines, width %v; want 2 lines, width in [%v, %v]", w, l.LineCount(), l.Size().W, world, w)
		}
		if !tlNear(l.Size().H, 2*full.Size().H, 0.01) {
			t.Errorf("wrapped height %v, want %v", l.Size().H, 2*full.Size().H)
		}

		l = e.lay("hello hello", f, hello+1)
		t.Logf("TEXT-RESULT hanging space: %v, %d lines (hello is %v wide)", l.Size(), l.LineCount(), hello)
		if l.LineCount() != 2 || l.Size().W > hello+1+tlEps {
			t.Errorf("hanging space: %d lines, width %v; want 2, at most %v", l.LineCount(), l.Size().W, hello+1)
		}

		long := strings.Repeat("W", 30)
		longW := e.width(long, f)
		l = e.lay(long, f, longW/3)
		t.Logf("TEXT-RESULT long word %v wide at %v: %v, %d lines", longW, longW/3, l.Size(), l.LineCount())
		if l.LineCount() < 3 || l.Size().W > longW/3+tlEps {
			t.Errorf("long word: %d lines, width %v (limit %v); want at least 3 lines within the limit", l.LineCount(), l.Size().W, longW/3)
		}

		if l = e.lay("abc", f, 0.5); l.LineCount() != 3 {
			t.Errorf("width 0.5: %d lines, want 3 (one glyph each)", l.LineCount())
		}
		if l = e.lay("hello hello\nhello hello", f, hello+1); l.LineCount() != 4 {
			t.Errorf("wrapped \\n lines: %d, want 4", l.LineCount())
		}
		if l = e.lay(s, f, 1000); l.LineCount() != 1 || l.Size() != full.Size() {
			t.Errorf("wide box: %d lines, %v; want 1, %v", l.LineCount(), l.Size(), full.Size())
		}
	})
}

// TestTextPangoWrapFits: over a sweep of widths, a wrapped layout is never
// wider than its width (unless one glyph is), and laying the text out
// again at the width it measured gives the same layout: the width
// rounding keeps measured text fitting.
func TestTextPangoWrapFits(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		f := render.Font{}
		const s = "The quick brown fox jumps over the lazy dog, then naps in the warm afternoon sun."
		glyph := e.width("W", f)
		n := 0
		for mw := 1.5 * glyph; mw < 400; mw += 7.3 {
			l := e.lay(s, f, mw)
			if l.Size().W > mw+tlEps {
				t.Errorf("at %v: width %v, wider than the box", mw, l.Size().W)
			}
			re := e.lay(s, f, l.Size().W)
			if re.LineCount() != l.LineCount() || re.Size() != l.Size() {
				t.Errorf("at %v: %v, %d lines; again at its width %v: %v, %d lines", mw, l.Size(), l.LineCount(), l.Size().W, re.Size(), re.LineCount())
			}
			n++
		}
		t.Logf("TEXT-RESULT %d widths from %v checked", n, 1.5*glyph)
	})
}

// TestTextPangoNoWrapWidths: zero, negative, NaN and infinite widths, and
// one past Pango's range, lay out as unwrapped; any positive width wraps.
func TestTextPangoNoWrapWidths(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		const s = "a fairly long line of text that would wrap in a narrow box"
		want := e.lay(s, render.Font{}, 0)
		if want.LineCount() != 1 {
			t.Errorf("unwrapped: %d lines, want 1", want.LineCount())
		}
		for _, w := range []float64{-1, -1e9, math.NaN(), math.Inf(1), math.Inf(-1), 1e300} {
			l := e.lay(s, render.Font{}, w)
			if l.LineCount() != 1 || l.Size() != want.Size() {
				t.Errorf("maxWidth %v: %d lines, %v; want 1, %v", w, l.LineCount(), l.Size(), want.Size())
			}
		}
		if l := e.lay(s, render.Font{}, 1e-9); l.LineCount() <= 1 {
			t.Errorf("tiny positive width: %d lines, want it wrapped", l.LineCount())
		}
	})
}

// TestTextPangoFractional: with hint metrics off, advances are not
// rounded to whole pixels: "iiii" and "Hello" at 13 pt are not a whole
// number of pixels wide.
func TestTextPangoFractional(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		for _, s := range []string{"iiii", "Hello"} {
			w := e.width(s, render.Font{Size: 13})
			u := w * pangoScale
			t.Logf("TEXT-RESULT %q at 13 pt: %v px = %v Pango units", s, w, u)
			if u != math.Trunc(u) {
				t.Errorf("%q: %v Pango units, not a whole number", s, u)
			}
			if math.Mod(u, pangoScale) == 0 {
				t.Errorf("%q: %v px, a whole number of pixels (hint metrics on?)", s, w)
			}
		}
	})
}

// TestTextPangoRTL: right-to-left text is laid out in an LTR base
// direction (auto_dir off), so a box wider than the text changes nothing:
// the same size as unwrapped, one line.
func TestTextPangoRTL(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		for _, s := range []string{"שלום עולם", "مرحبا بالعالم", "abc שלום def"} {
			u := e.lay(s, render.Font{}, 0)
			l := e.lay(s, render.Font{}, u.Size().W+50)
			t.Logf("TEXT-RESULT %q: unwrapped %v, in a box %v wide %v", s, u.Size(), u.Size().W+50, l.Size())
			if l.LineCount() != 1 || !tlNear(l.Size().W, u.Size().W, tlEps) || l.Size() != u.Size() {
				t.Errorf("%q in a wide box: %v, %d lines; want %v, 1", s, l.Size(), l.LineCount(), u.Size())
			}
		}
	})
}

// tlHave returns the candidates that resolve to themselves (the font map
// has them).
func (e *tlEnv) tlHave(cands ...string) []string {
	var have []string
	for _, c := range cands {
		if c != "" && e.ts.resolve(render.Font{Family: c}).family == c {
			have = append(have, c)
		}
	}
	return have
}

// TestTextPangoWeightItalic: bold is wider than regular, italic changes
// the width (in a family with distinct faces; serif italics have their own
// advances, sans obliques often not), and an explicit regular weight is
// the zero weight.
func TestTextPangoWeightItalic(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		const s = "Hello, World 0123"
		if r := e.width(s, render.Font{Weight: driver.FontWeightRegular}); r != e.width(s, render.Font{}) {
			t.Errorf("explicit regular weight %v differs from the zero weight", r)
		}
		var why []string
		boldOK := false
		for _, fam := range e.tlHave(e.ts.ui, "Cantarell", "DejaVu Sans", "Liberation Sans", "Noto Sans", "Ubuntu", "sans-serif") {
			reg := e.width(s, render.Font{Family: fam})
			bold := e.width(s, render.Font{Family: fam, Weight: driver.FontWeightBold})
			t.Logf("TEXT-RESULT %q: regular %v, bold %v", fam, reg, bold)
			if bold > reg {
				boldOK = true
				break
			}
			why = append(why, fmt.Sprintf("%s bold %v ≤ regular %v", fam, bold, reg))
		}
		if !boldOK {
			why = append(why, "no family with a wider bold")
		}
		italicOK := false
		for _, fam := range e.tlHave("Liberation Serif", "DejaVu Serif", "Noto Serif", "FreeSerif", "serif", e.ts.ui, "DejaVu Sans") {
			reg := e.width(s, render.Font{Family: fam})
			it := e.width(s, render.Font{Family: fam, Italic: true})
			t.Logf("TEXT-RESULT %q: regular %v, italic %v", fam, reg, it)
			if it != reg {
				italicOK = true
				break
			}
		}
		if !italicOK {
			why = append(why, "no family whose italic changes the width")
		}
		if !boldOK || !italicOK {
			e.skipf("fontconfig lacks distinct faces: %s", strings.Join(why, "; "))
		}
	})
}

// TestTextPangoMonospaced: Monospaced resolves to monospace, whose runes
// are equally wide ("iii" as "WWW"), unlike the UI font's; with a family
// set, Monospaced is ignored.
func TestTextPangoMonospaced(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		mono := render.Font{Monospaced: true}
		if got := e.ts.resolve(mono).family; got != familyMono {
			t.Errorf("Monospaced resolves to %q, want %q", got, familyMono)
		}
		i, w := e.width("iii", mono), e.width("WWW", mono)
		ui, uw := e.width("iii", render.Font{}), e.width("WWW", render.Font{})
		t.Logf("TEXT-RESULT monospace iii %v, WWW %v; UI font %q iii %v, WWW %v", i, w, e.ts.ui, ui, uw)
		if !tlNear(i, w, 0.01) || i <= 0 {
			t.Errorf("monospaced iii %v, WWW %v; want equal and positive", i, w)
		}
		if ui >= uw {
			t.Errorf("UI font iii %v not narrower than WWW %v", ui, uw)
		}
		ff := e.ts.ui
		if got := e.width("iii", render.Font{Family: ff, Monospaced: true}); got != e.width("iii", render.Font{Family: ff}) {
			t.Errorf("Family %q with Monospaced: iii %v, want the family's %v", ff, got, e.width("iii", render.Font{Family: ff}))
		}
	})
}

// TestTextPangoFamilies: an unknown family is the UI font (hasFamily false,
// the UI font's metrics); an installed family is kept, also spelled in
// lower case; the generic names are there.
func TestTextPangoFamilies(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		const unknown = "NoSuchFamily-gogui"
		n := e.native()
		if n == nil {
			t.Errorf("the text system's layouter is a %T", e.ts.pl)
			return
		}
		if e.ts.hasFamily(unknown) || n.hasFamily(unknown) {
			t.Errorf("hasFamily(%q) true", unknown)
		}
		if got := e.ts.resolve(render.Font{Family: unknown}).family; got != e.ts.ui {
			t.Errorf("%q resolves to %q, want the UI family %q", unknown, got, e.ts.ui)
		}
		const s = "Hello, World"
		if got, want := e.lay(s, render.Font{Family: unknown}, 0), e.lay(s, render.Font{}, 0); got.Size() != want.Size() || got.Baseline() != want.Baseline() {
			t.Errorf("%q: %v, baseline %v; want the UI font's %v, %v", unknown, got.Size(), got.Baseline(), want.Size(), want.Baseline())
		}
		for _, g := range []string{"monospace", "Monospace", "sans-serif"} {
			if !e.ts.hasFamily(g) {
				t.Errorf("hasFamily(%q) false, want a generic family", g)
			}
		}
		known := ""
		for _, c := range []string{"DejaVu Sans", "Cantarell", "Liberation Sans", "Noto Sans", "Ubuntu", e.ts.ui} {
			if c != familyUIFallback && n.hasFamily(c) {
				known = c
				break
			}
		}
		t.Logf("TEXT-RESULT UI family %q, installed family used %q", e.ts.ui, known)
		if known == "" {
			e.skipf("none of the candidate families is installed (UI family %q)", e.ts.ui)
			return
		}
		if !e.ts.hasFamily(known) {
			t.Errorf("textSystem.hasFamily(%q) false, pangoNative's true", known)
		}
		if got := e.ts.resolve(render.Font{Family: known}).family; got != known {
			t.Errorf("%q resolves to %q, want it kept", known, got)
		}
		for _, spelt := range []string{strings.ToLower(known), strings.ToUpper(known)} {
			if spelt == known {
				continue
			}
			if !n.hasFamily(spelt) || !e.ts.hasFamily(spelt) {
				t.Errorf("hasFamily(%q) false (pangoNative %v), but %q is installed", spelt, n.hasFamily(spelt), known)
			}
		}
		if !e.ts.hasFamily("MONOSPACE") || !e.ts.hasFamily("Sans-Serif") {
			t.Error("a generic family in another case is not there")
		}
	})
}

// TestTextPangoBytes: NUL and invalid UTF-8 are laid out (no crash), keep
// the original Text(), and measure as with '?' in their place.
func TestTextPangoBytes(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		for _, s := range []string{"a\x00b", "a\xffb", "\xff\xfe", "x\x00", "\x00", "é\xc3", "ok\x00\nnext"} {
			l := e.lay(s, render.Font{}, 0)
			want := e.lay(pangoText(s), render.Font{}, 0)
			t.Logf("TEXT-RESULT %q: %v, %d lines (as %q: %v)", s, l.Size(), l.LineCount(), pangoText(s), want.Size())
			if l.Text() != s {
				t.Errorf("%q: Text() = %q", s, l.Text())
			}
			if l.Size() != want.Size() || l.LineCount() != want.LineCount() || l.Size().W <= 0 {
				t.Errorf("%q: %v, %d lines; want %q's %v, %d lines (positive width)", s, l.Size(), l.LineCount(), pangoText(s), want.Size(), want.LineCount())
			}
		}
	})
}

// tlChromeLayouts lays out text as aqua's title bar and controls do
// (internal/aqua chrome.go: the window title in the title font, truncated
// with an ellipsis; control titles in the small system font, 11 pt, which
// the placeholders' captions used until S5.5j) and checks that every
// layout has a positive, finite size, one line and the baseline inside
// it, and that widths grow with the truncated prefix (truncate's binary
// search relies on it).
func tlChromeLayouts(t *testing.T, ts render.TextSystem) {
	t.Helper()
	ty := &theme.Sequoia.Typography
	title := driver.Font{Size: ty.FontSize.At(driver.ControlSizeRegular), Weight: ty.WindowTitleWeight}
	check := func(what string, l render.TextLayout) {
		t.Helper()
		sz, b := l.Size(), l.Baseline()
		t.Logf("TEXT-RESULT %s %q: %v, baseline %v", what, l.Text(), sz, b)
		if _, ok := l.(*textLayout); !ok {
			t.Errorf("%s %q: a %T, not a Pango layout", what, l.Text(), l)
		}
		if !tlFinite(sz.W, sz.H, b) || sz.W <= 0 || sz.H <= 0 || l.LineCount() != 1 || !(b > 0 && b < sz.H) {
			t.Errorf("%s %q: %v, baseline %v, %d lines; want a positive finite size, one line, the baseline inside", what, l.Text(), sz, b, l.LineCount())
		}
	}
	const name = "gogui — Untitled Document"
	check("title", ts.NewLayout(name, title, 0))
	prev := 0.0
	for _, n := range []int{0, 1, 5, 10} {
		l := ts.NewLayout(name[:n]+"…", title, 0)
		check("truncated title", l)
		if l.Size().W <= prev {
			t.Errorf("truncated title %q: width %v not above the shorter prefix's %v", l.Text(), l.Size().W, prev)
		}
		prev = l.Size().W
	}
	small := driver.Font{Size: ty.FontSize.At(driver.ControlSizeSmall)}
	for _, c := range []string{"OK", "Medium", "Hello, World", "Custom size…"} {
		check("small control title", ts.NewLayout(c, small, 0))
	}
}

// TestTextPangoShell: in Ready, the shell's TextSystem gives Pango layouts
// (not shell/fake's) with the UI family from GtkSettings, and the aqua
// title bar's and controls' text lays out.
func TestTextPangoShell(t *testing.T) {
	s, reports := newGTKShell(t)
	base := dispatch.Len()
	p, err := runGTK(t, s, &handler{ready: func() {
		ts := s.TextSystem()
		l := ts.NewLayout("Hello", render.Font{}, 0)
		if _, ok := l.(*textLayout); !ok {
			t.Errorf("Ready: NewLayout gives a %T, not a Pango layout", l)
		}
		t.Logf("TEXT-RESULT shell UI family %q (GtkSettings), Hello %v", s.text.ui, l.Size())
		if s.text.failed || s.text.pl == nil {
			t.Errorf("the shell's text system failed or has no Pango (failed %v)", s.text.failed)
		}
		tlChromeLayouts(t, ts)
		go s.Post(s.Quit)
	}})
	if p != nil || err != nil {
		t.Fatalf("Run = %v, panicked with %v", err, p)
	}
	checkGTKTornDown(t, s, reports, base)
}

// TestTextPangoChrome: the aqua title bar's and controls' text lays
// out over Pango without a display (TestTextPangoShell does it through the
// shell, with one).
func TestTextPangoChrome(t *testing.T) {
	tlPango(t, func(e *tlEnv) { tlChromeLayouts(t, e.ts) })
}

// Hit-testing, carets and selection over real Pango (S4.2h): the rules of
// textgeom.go on the geometry pangoNative.measure reads. As above, the
// checks are on the contract: positions are compared with the widths of
// the same text's prefixes (within tlKern, as kerning and shaping across
// a prefix's end may move them a little), never with fixed numbers.

// tlKern is the tolerance of a caret x against a prefix's width.
const tlKern = 1.0

// geo is e.lay as a *textLayout (nil, with the test failed, when it is
// not a Pango layout).
func (e *tlEnv) geo(text string, f render.Font, maxWidth float64) *textLayout {
	e.t.Helper()
	l, _ := e.lay(text, f, maxWidth).(*textLayout)
	return l
}

// tlMidY is the y of the middle of the line the caret at index is on.
func tlMidY(l render.TextLayout, index int) float64 {
	c := l.CaretRect(index)
	return c.Y + c.H/2
}

// tlCaretShape checks that a caret rect is zero wide, finite and of a
// positive height.
func tlCaretShape(t *testing.T, what string, c render.Rect) {
	t.Helper()
	if c.W != 0 || !(c.H > 0) || !tlFinite(c.X, c.Y, c.H) {
		t.Errorf("%s: caret %v, want a zero-width rect of a positive finite height", what, c)
	}
}

// TestTextPangoCaretASCII: in "Hello world" every byte is a cluster; the
// caret's x grows with the index and is the width of the prefix before
// it, on the first line (y 0, the line's height); the caret at the end is
// at the text's width.
func TestTextPangoCaretASCII(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		f := render.Font{}
		const s = "Hello world"
		l := e.geo(s, f, 0)
		if l == nil {
			return
		}
		prev := math.Inf(-1)
		for i := 0; i <= len(s); i++ {
			c := l.CaretRect(i)
			tlCaretShape(t, fmt.Sprintf("CaretRect(%d)", i), c)
			want := e.width(s[:i], f)
			if !tlNear(c.X, want, tlKern) {
				t.Errorf("CaretRect(%d).X = %v, want about %q's width %v", i, c.X, s[:i], want)
			}
			if c.X <= prev {
				t.Errorf("CaretRect(%d).X = %v, not right of index %d's %v", i, c.X, i-1, prev)
			}
			if c.Y != 0 || !tlNear(c.H, l.Size().H, 0.01) {
				t.Errorf("CaretRect(%d) = %v, want y 0 and the line's height %v", i, c, l.Size().H)
			}
			if got := l.normalize(i); got != i {
				t.Errorf("normalize(%d) = %d, want it kept (an ASCII cluster)", i, got)
			}
			prev = c.X
		}
		if end := l.CaretRect(len(s)).X; !tlNear(end, l.Size().W, 0.01) {
			t.Errorf("caret at the end x %v, want the width %v", end, l.Size().W)
		}
		t.Logf("TEXT-RESULT %q carets from %v to %v", s, l.CaretRect(0).X, l.CaretRect(len(s)).X)
	})
}

// TestTextPangoHitTestASCII: a point in the left half of a cluster hits
// it with trailing false, in the right half with trailing true; left of
// the text is index 0, right of it the end, both with trailing false, on
// any y inside the line.
func TestTextPangoHitTestASCII(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		const s = "Hello, Wim!"
		l := e.geo(s, render.Font{}, 0)
		if l == nil {
			return
		}
		h := l.Size().H
		for i := 0; i < len(s); i++ {
			x0, x1 := l.CaretRect(i).X, l.CaretRect(i+1).X
			w := x1 - x0
			for _, y := range []float64{0.5, h / 2, h - 0.5} {
				if got, tr := l.HitTest(render.Point{X: x0 + w/4, Y: y}); got != i || tr {
					t.Errorf("HitTest(left half of %q at %d, y %v) = %d, %v; want %d, false", s[i], i, y, got, tr, i)
				}
				if got, tr := l.HitTest(render.Point{X: x0 + 3*w/4, Y: y}); got != i || !tr {
					t.Errorf("HitTest(right half of %q at %d, y %v) = %d, %v; want %d, true", s[i], i, y, got, tr, i)
				}
			}
		}
		for _, x := range []float64{-5, 0} {
			if got, tr := l.HitTest(render.Point{X: x, Y: h / 2}); got != 0 || tr {
				t.Errorf("HitTest(x %v) = %d, %v; want 0, false", x, got, tr)
			}
		}
		if got, tr := l.HitTest(render.Point{X: l.Size().W + 5, Y: h / 2}); got != len(s) || tr {
			t.Errorf("HitTest(right of the text) = %d, %v; want %d, false", got, tr, len(s))
		}
	})
}

// TestTextPangoSelectionASCII: a range on one line is one rect on it from
// the caret at its start, as wide as the selected substring; the whole
// text is the layout's width; the order of start and end does not
// matter, and an empty range is no rect.
func TestTextPangoSelectionASCII(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		f := render.Font{}
		const s = "Hello world"
		l := e.geo(s, f, 0)
		if l == nil {
			return
		}
		for _, r := range [][2]int{{6, 11}, {0, 5}, {2, 8}, {4, 5}} {
			rs := l.SelectionRects(r[0], r[1])
			want := e.width(s[r[0]:r[1]], f)
			t.Logf("TEXT-RESULT %q selection %v: %v (substring %v wide)", s, r, rs, want)
			if len(rs) != 1 {
				t.Errorf("SelectionRects%v = %v, want one rect", r, rs)
				continue
			}
			if !tlNear(rs[0].W, want, tlKern) || !tlNear(rs[0].X, l.CaretRect(r[0]).X, 0.01) {
				t.Errorf("SelectionRects%v = %v, want x %v and about %q's width %v", r, rs[0], l.CaretRect(r[0]).X, s[r[0]:r[1]], want)
			}
			if rs[0].Y != 0 || !tlNear(rs[0].H, l.Size().H, 0.01) {
				t.Errorf("SelectionRects%v = %v, want y 0 and the line's height %v", r, rs[0], l.Size().H)
			}
			if back := l.SelectionRects(r[1], r[0]); len(back) != 1 || back[0] != rs[0] {
				t.Errorf("SelectionRects(%d, %d) = %v, want %v", r[1], r[0], back, rs)
			}
		}
		if rs := l.SelectionRects(0, len(s)); len(rs) != 1 || rs[0].X != 0 || !tlNear(rs[0].W, l.Size().W, 0.01) {
			t.Errorf("SelectionRects(all) = %v, want one rect 0..%v", rs, l.Size().W)
		}
		for _, i := range []int{0, 3, len(s)} {
			if rs := l.SelectionRects(i, i); len(rs) != 0 {
				t.Errorf("SelectionRects(%d, %d) = %v, want none", i, i, rs)
			}
		}
	})
}

// TestTextPangoWrapGeometry: "hello world again" wrapped after "world ":
// the caret at the wrap index is at the start of the second line (x 0, y
// below the first); right of the first line is the index of its last
// space with trailing false; a selection across the wrap is one rect per
// line, the first up to the line's end, the second from x 0.
func TestTextPangoWrapGeometry(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		f := render.Font{}
		const s = "hello world again"
		const wrap = 12 // "again"
		full := e.width(s, f)
		world := e.width("hello world", f)
		l := e.geo(s, f, (world+full)/2)
		if l == nil {
			return
		}
		if l.LineCount() != 2 {
			t.Errorf("%q at %v: %d lines, want 2", s, (world+full)/2, l.LineCount())
			return
		}
		c0, cw := l.CaretRect(0), l.CaretRect(wrap)
		tlCaretShape(t, "caret at the wrap", cw)
		t.Logf("TEXT-RESULT wrapped %q: caret 0 %v, at the wrap %v, at the space %v", s, c0, cw, l.CaretRect(wrap-1))
		if !tlNear(cw.X, 0, 0.01) || !(cw.Y >= c0.Y+c0.H-0.01) {
			t.Errorf("CaretRect(%d) = %v, want x 0 on the second line (below %v)", wrap, cw, c0)
		}
		if sp := l.CaretRect(wrap - 1); sp.Y != c0.Y || !tlNear(sp.X, world, tlKern) {
			t.Errorf("CaretRect(%d) (the space) = %v, want on the first line at about %v", wrap-1, sp, world)
		}
		for _, x := range []float64{world + 0.5, l.Size().W + 20, 1000} {
			if got, tr := l.HitTest(render.Point{X: x, Y: tlMidY(l, 0)}); got != wrap-1 || tr {
				t.Errorf("HitTest(x %v on the first line) = %d, %v; want the space %d, false", x, got, tr, wrap-1)
			}
		}
		if got, tr := l.HitTest(render.Point{X: 1000, Y: tlMidY(l, wrap)}); got != len(s) || tr {
			t.Errorf("HitTest(right of the second line) = %d, %v; want %d, false", got, tr, len(s))
		}
		if got, tr := l.HitTest(render.Point{X: -1, Y: tlMidY(l, wrap)}); got != wrap || tr {
			t.Errorf("HitTest(left of the second line) = %d, %v; want %d, false", got, tr, wrap)
		}
		rs := l.SelectionRects(6, 15) // "world " + "aga"
		t.Logf("TEXT-RESULT wrapped selection [6, 15): %v", rs)
		if len(rs) != 2 {
			t.Errorf("SelectionRects(6, 15) = %v, want a rect per line", rs)
			return
		}
		if !tlNear(rs[0].X, l.CaretRect(6).X, 0.01) || rs[0].X+rs[0].W < world-0.01 || rs[0].Y != c0.Y {
			t.Errorf("first line's rect %v, want from %v to at least %v at y %v", rs[0], l.CaretRect(6).X, world, c0.Y)
		}
		if !tlNear(rs[1].X, 0, 0.01) || !tlNear(rs[1].W, e.width("aga", f), tlKern) || rs[1].Y != cw.Y || !(rs[1].Y > rs[0].Y) {
			t.Errorf("second line's rect %v, want x 0, about %v wide, at y %v", rs[1], e.width("aga", f), cw.Y)
		}
		if rs := l.SelectionRects(wrap, len(s)); len(rs) != 1 || rs[0].Y != cw.Y {
			t.Errorf("SelectionRects(%d, %d) = %v, want one rect on the second line", wrap, len(s), rs)
		}
	})
}

// TestTextPangoNewlines: in "ab\ncd\n" each line's caret is lower than the
// one before; right of a line is the index before its "\n"; the caret
// after the trailing "\n" is on the empty last line at x 0; a selection
// that includes a "\n" is wider by newlineWidth; an index inside "\r\n"
// is before it.
func TestTextPangoNewlines(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		f := render.Font{}
		const s = "ab\ncd\n"
		l := e.geo(s, f, 0)
		if l == nil {
			return
		}
		if l.LineCount() != 3 {
			t.Errorf("%q: %d lines, want 3", s, l.LineCount())
			return
		}
		prevY := math.Inf(-1)
		for _, i := range []int{0, 3, 6} { // the lines' starts
			c := l.CaretRect(i)
			tlCaretShape(t, fmt.Sprintf("CaretRect(%d)", i), c)
			if !(c.Y > prevY) || !tlNear(c.X, 0, 0.01) {
				t.Errorf("CaretRect(%d) = %v, want x 0 below the line before (y %v)", i, c, prevY)
			}
			if got, tr := l.HitTest(render.Point{X: -1, Y: c.Y + c.H/2}); got != i || tr {
				t.Errorf("HitTest(left of the line at y %v) = %d, %v; want %d, false", c.Y, got, tr, i)
			}
			prevY = c.Y
		}
		t.Logf("TEXT-RESULT %q carets %v %v %v", s, l.CaretRect(0), l.CaretRect(3), l.CaretRect(6))
		for _, w := range [][2]int{{0, 2}, {3, 5}} { // a line's start, its "\n"
			if got, tr := l.HitTest(render.Point{X: 1000, Y: tlMidY(l, w[0])}); got != w[1] || tr {
				t.Errorf("HitTest(right of the line at %d) = %d, %v; want %d, false", w[0], got, tr, w[1])
			}
			if c := l.CaretRect(w[1]); c.Y != l.CaretRect(w[0]).Y || !tlNear(c.X, e.width(s[w[0]:w[1]], f), tlKern) {
				t.Errorf("CaretRect(%d) (before \\n) = %v, want on its line at about %v", w[1], c, e.width(s[w[0]:w[1]], f))
			}
		}
		if got, tr := l.HitTest(render.Point{X: 1000, Y: l.Size().H + 50}); got != len(s) || tr {
			t.Errorf("HitTest(below the text) = %d, %v; want %d, false", got, tr, len(s))
		}
		nl := newlineWidth(e.ts.resolve(f).size / pangoScale)
		ab := l.CaretRect(2).X
		if rs := l.SelectionRects(0, 2); len(rs) != 1 || !tlNear(rs[0].W, ab, 0.01) {
			t.Errorf("SelectionRects(0, 2) = %v, want one rect %v wide (no newline)", rs, ab)
		}
		rs := l.SelectionRects(0, 4)
		t.Logf("TEXT-RESULT %q selection [0, 4): %v (newline width %v)", s, rs, nl)
		if len(rs) != 2 || !tlNear(rs[0].W, ab+nl, 0.01) || rs[0].X != 0 ||
			!tlNear(rs[1].X, 0, 0.01) || !tlNear(rs[1].W, e.width("c", f), tlKern) || !(rs[1].Y > rs[0].Y) {
			t.Errorf("SelectionRects(0, 4) = %v, want [0, %v] on the first line and \"c\" (%v) on the second", rs, ab+nl, e.width("c", f))
		}
		if rs := l.SelectionRects(5, 6); len(rs) != 1 || !tlNear(rs[0].X, l.CaretRect(5).X, 0.01) || !tlNear(rs[0].W, nl, 0.01) {
			t.Errorf("SelectionRects(5, 6) (the second \\n) = %v, want one rect %v wide at %v", rs, nl, l.CaretRect(5).X)
		}
		if rs := l.SelectionRects(0, len(s)); len(rs) != 2 {
			t.Errorf("SelectionRects(all) = %v, want two rects (the empty last line has none)", rs)
		}

		crlf := e.geo("ab\r\ncd", f, 0)
		if crlf == nil {
			return
		}
		for i, want := range map[int]int{2: 2, 3: 2, 4: 4} {
			if got := crlf.normalize(i); got != want {
				t.Errorf("ab\\r\\ncd: normalize(%d) = %d, want %d", i, got, want)
			}
		}
		if crlf.CaretRect(3) != crlf.CaretRect(2) || !(crlf.CaretRect(4).Y > crlf.CaretRect(2).Y) {
			t.Errorf("ab\\r\\ncd: carets 2 %v, 3 %v, 4 %v; want 3 at 2, 4 on the next line", crlf.CaretRect(2), crlf.CaretRect(3), crlf.CaretRect(4))
		}
	})
}

// tlClusterCase is text with one multi-byte cluster [start, end).
type tlClusterCase struct {
	name       string
	text       string
	start, end int
}

// tlCheckCluster checks that every index inside a cluster normalizes to
// its start (normalize, CaretRect, SelectionRects), that its start and
// the next cluster's are kept, and that the cluster is hit as one.
func tlCheckCluster(t *testing.T, l *textLayout, c tlClusterCase) {
	t.Helper()
	if got := l.normalize(c.start); got != c.start {
		t.Errorf("%s: normalize(%d) (the start) = %d", c.name, c.start, got)
	}
	if got := l.normalize(c.end); got != c.end {
		t.Errorf("%s: normalize(%d) (the next cluster) = %d", c.name, c.end, got)
	}
	start := l.CaretRect(c.start)
	whole := fmt.Sprint(l.SelectionRects(c.start, c.end))
	for i := c.start + 1; i < c.end; i++ {
		if got := l.normalize(i); got != c.start {
			t.Errorf("%s: normalize(%d) = %d, want the cluster's start %d", c.name, i, got, c.start)
		}
		if got := l.CaretRect(i); got != start {
			t.Errorf("%s: CaretRect(%d) = %v, want the cluster start's %v", c.name, i, got, start)
		}
		if got := fmt.Sprint(l.SelectionRects(i, c.end)); got != whole {
			t.Errorf("%s: SelectionRects(%d, %d) = %s, want the cluster's %s", c.name, i, c.end, got, whole)
		}
	}
	next := l.CaretRect(c.end)
	if !(next.X > start.X) {
		t.Errorf("%s: caret after the cluster x %v, not right of its start %v", c.name, next.X, start.X)
		return
	}
	w, y := next.X-start.X, start.Y+start.H/2
	if rs := l.SelectionRects(c.start, c.end); len(rs) != 1 || !tlNear(rs[0].W, w, 0.01) {
		t.Errorf("%s: SelectionRects(%d, %d) = %v, want one rect %v wide", c.name, c.start, c.end, rs, w)
	}
	for _, q := range []float64{0.1, 0.4} {
		if got, tr := l.HitTest(render.Point{X: start.X + q*w, Y: y}); got != c.start || tr {
			t.Errorf("%s: HitTest(%v of the cluster) = %d, %v; want %d, false", c.name, q, got, tr, c.start)
		}
	}
	for _, q := range []float64{0.6, 0.9} {
		if got, tr := l.HitTest(render.Point{X: start.X + q*w, Y: y}); got != c.start || !tr {
			t.Errorf("%s: HitTest(%v of the cluster) = %d, %v; want %d, true", c.name, q, got, tr, c.start)
		}
	}
}

// TestTextPangoCombining: a base letter with combining marks (and a
// precomposed letter, two bytes) is one cluster: indices inside it move
// to its start.
func TestTextPangoCombining(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		for _, c := range []tlClusterCase{
			{"e + U+0301", "ae\u0301b", 1, 4},
			{"a + U+0308 + U+0323", "xa\u0308\u0323y", 1, 6},
			{"precomposed U+00E9", "a\u00e9b", 1, 3},
			{"e + U+0301 at the end", "ae\u0301", 1, 4},
		} {
			l := e.geo(c.text, render.Font{}, 0)
			if l == nil {
				continue
			}
			t.Logf("TEXT-RESULT %s %q: carets %v %v, size %v", c.name, c.text, l.CaretRect(c.start), l.CaretRect(c.end), l.Size())
			tlCheckCluster(t, l, c)
		}
	})
}

// TestTextPangoEmoji: an emoji, a single code point or a sequence (ZWJ,
// skin tone modifier, regional indicator pair), is one cluster. Grapheme
// boundaries do not depend on the fonts (Pango 1.52 segments them by UAX
// #29), and without an emoji font each code point still gets a box, so
// the checks hold either way; the emoji families the font map has are
// logged.
func TestTextPangoEmoji(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		fonts := e.tlHave("Noto Color Emoji", "Twemoji", "Apple Color Emoji", "EmojiOne Color", "Symbola")
		t.Logf("TEXT-RESULT emoji families installed: %q", fonts)
		for _, c := range []tlClusterCase{
			{"U+1F600", "a\U0001F600b", 1, 5},
			{"woman technologist (ZWJ)", "a\U0001F469\u200d\U0001F4BBb", 1, 12},
			{"thumbs up + skin tone", "a\U0001F44D\U0001F3FDb", 1, 9},
			{"flag (regional indicators)", "a\U0001F1EB\U0001F1F7b", 1, 9},
		} {
			l := e.geo(c.text, render.Font{}, 0)
			if l == nil {
				continue
			}
			t.Logf("TEXT-RESULT %s %q: carets %v %v, size %v", c.name, c.text, l.CaretRect(c.start), l.CaretRect(c.end), l.Size())
			tlCheckCluster(t, l, c)
		}
	})
}

// TestTextPangoClamp: indices before the text are 0, past it the end;
// points far outside hit the nearest line's ends, NaN the first line's
// start; in empty text everything is index 0.
func TestTextPangoClamp(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		const s = "ab\ncd"
		l := e.geo(s, render.Font{}, 0)
		if l == nil {
			return
		}
		for _, i := range []int{-1, -1000, math.MinInt} {
			if l.normalize(i) != 0 || l.CaretRect(i) != l.CaretRect(0) {
				t.Errorf("index %d: normalize %d, caret %v; want 0, %v", i, l.normalize(i), l.CaretRect(i), l.CaretRect(0))
			}
		}
		for _, i := range []int{len(s) + 1, 1 << 20, math.MaxInt} {
			if l.normalize(i) != len(s) || l.CaretRect(i) != l.CaretRect(len(s)) {
				t.Errorf("index %d: normalize %d, caret %v; want %d, %v", i, l.normalize(i), l.CaretRect(i), len(s), l.CaretRect(len(s)))
			}
		}
		all := fmt.Sprint(l.SelectionRects(0, len(s)))
		for _, r := range [][2]int{{-5, 1000}, {math.MaxInt, math.MinInt}, {-1, len(s)}} {
			if got := fmt.Sprint(l.SelectionRects(r[0], r[1])); got != all {
				t.Errorf("SelectionRects%v = %s, want the whole text's %s", r, got, all)
			}
		}
		for _, r := range [][2]int{{-5, -1}, {100, 200}} {
			if rs := l.SelectionRects(r[0], r[1]); len(rs) != 0 {
				t.Errorf("SelectionRects%v = %v, want none", r, rs)
			}
		}
		inf, nan := math.Inf(1), math.NaN()
		for _, h := range []struct {
			p     render.Point
			index int
		}{
			{render.Point{X: -1e9, Y: -1e9}, 0},
			{render.Point{X: 1e9, Y: -1e9}, 2},
			{render.Point{X: -1e9, Y: 1e9}, 3},
			{render.Point{X: 1e9, Y: 1e9}, len(s)},
			{render.Point{X: inf, Y: inf}, len(s)},
			{render.Point{X: -inf, Y: -inf}, 0},
			{render.Point{X: nan, Y: nan}, 0},
			{render.Point{X: 1e9, Y: nan}, 2},
		} {
			if got, tr := l.HitTest(h.p); got != h.index || tr {
				t.Errorf("HitTest(%v) = %d, %v; want %d, false", h.p, got, tr, h.index)
			}
		}
		empty := e.geo("", render.Font{}, 0)
		if empty == nil {
			return
		}
		for _, p := range []render.Point{{X: -10, Y: -10}, {X: 10, Y: 5}, {X: 1e9, Y: 1e9}} {
			if got, tr := empty.HitTest(p); got != 0 || tr {
				t.Errorf("empty text: HitTest(%v) = %d, %v; want 0, false", p, got, tr)
			}
		}
		if c := empty.CaretRect(5); c.X != 0 || c.Y != 0 || !(c.H > 0) || len(empty.SelectionRects(-1, 5)) != 0 {
			t.Errorf("empty text: CaretRect(5) = %v, selection %v; want x 0 of the line height, none", c, empty.SelectionRects(-1, 5))
		}
	})
}

// TestTextPangoLineSeparator: U+2028 (LINE SEPARATOR) is a forced break
// inside a paragraph: "a", U+2028, "b" is two lines, right of the first
// line is the index before the separator (1) with trailing false, and an
// index inside the separator is before it. VT, which DirectWrite breaks
// at, is an ordinary cluster in Pango (textgeom.go): what Pango does with
// "a", VT, "b" is logged, not checked.
func TestTextPangoLineSeparator(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		ls := "a" + string(rune(0x2028)) + "b" // the separator is bytes 1..3
		l := e.geo(ls, render.Font{}, 0)
		if l == nil {
			return
		}
		t.Logf("TEXT-RESULT a U+2028 b: %d lines, %v; carets 0 %v, 1 %v, 4 %v", l.LineCount(), l.Size(), l.CaretRect(0), l.CaretRect(1), l.CaretRect(4))
		if l.LineCount() != 2 {
			t.Errorf("a U+2028 b: %d lines, want 2", l.LineCount())
			return
		}
		if got, tr := l.HitTest(render.Point{X: 1000, Y: tlMidY(l, 0)}); got != 1 || tr {
			t.Errorf("a U+2028 b: HitTest(right of line 0) = %d, %v; want 1, false", got, tr)
		}
		if got, tr := l.HitTest(render.Point{X: 1000, Y: tlMidY(l, 4)}); got != len(ls) || tr {
			t.Errorf("a U+2028 b: HitTest(right of line 1) = %d, %v; want %d, false", got, tr, len(ls))
		}
		for _, i := range []int{2, 3} {
			if got := l.normalize(i); got != 1 {
				t.Errorf("a U+2028 b: normalize(%d) = %d, want 1 (before the separator)", i, got)
			}
		}
		if c0, c1, c4 := l.CaretRect(0), l.CaretRect(1), l.CaretRect(4); c1.Y != c0.Y || !(c1.X > c0.X) || !(c4.Y > c0.Y) || !tlNear(c4.X, 0, 0.01) {
			t.Errorf("a U+2028 b: carets 0 %v, 1 %v, 4 %v; want 1 right of 0 on line 0, 4 at x 0 on line 1", c0, c1, c4)
		}

		vt := "a" + string(rune(0x0b)) + "b"
		v := e.geo(vt, render.Font{}, 0)
		if v == nil {
			return
		}
		hit, tr := v.HitTest(render.Point{X: 1000, Y: tlMidY(v, 0)})
		t.Logf("TEXT-RESULT a VT b: %d lines (one expected), %v; normalize 1 %d, 2 %d; HitTest right of line 0 %d, %v",
			v.LineCount(), v.Size(), v.normalize(1), v.normalize(2), hit, tr)
		if v.LineCount() != 1 {
			t.Logf("TEXT-RESULT a VT b: Pango broke the line at VT (%d lines), unlike textgeom.go's assumption", v.LineCount())
		}
	})
}

// TestTextPangoRTLCarets: in Hebrew text (an RTL run in the LTR base
// direction), each letter is a cluster whose leading edge is its right:
// the caret at index 0 is at the text's right edge and the carets move
// left with the index; the right half of the first letter hits it with
// trailing false, its left half with trailing true. Skipped when the
// font map has none of the usual Hebrew fonts.
func TestTextPangoRTLCarets(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		fonts := e.tlHave("Noto Sans Hebrew", "DejaVu Sans", "Liberation Sans", "FreeSans", "Noto Sans", "Arial")
		if len(fonts) == 0 {
			e.skipf("no Hebrew font found (UI family %q)", e.ts.ui)
			return
		}
		f := render.Font{Family: fonts[0]}
		const s = "שלום" // four letters of two bytes
		l := e.geo(s, f, 0)
		if l == nil {
			return
		}
		t.Logf("TEXT-RESULT %q in %q: %v; carets %v %v %v %v %v", s, fonts[0], l.Size(),
			l.CaretRect(0), l.CaretRect(2), l.CaretRect(4), l.CaretRect(6), l.CaretRect(8))
		if c := l.CaretRect(0); !tlNear(c.X, l.Size().W, 0.01) {
			t.Errorf("CaretRect(0) = %v, want the right edge %v", c, l.Size().W)
		}
		prev := math.Inf(1)
		for i := 0; i < len(s); i += 2 {
			c := l.CaretRect(i)
			tlCaretShape(t, fmt.Sprintf("CaretRect(%d)", i), c)
			if !(c.X < prev) {
				t.Errorf("CaretRect(%d).X = %v, not left of the previous index's %v", i, c.X, prev)
			}
			if got := l.normalize(i + 1); got != i {
				t.Errorf("normalize(%d) = %d, want %d", i+1, got, i)
			}
			prev = c.X
		}
		x0, x1 := l.CaretRect(2).X, l.CaretRect(0).X // the first letter's box
		y := tlMidY(l, 0)
		if got, tr := l.HitTest(render.Point{X: x0 + 3*(x1-x0)/4, Y: y}); got != 0 || tr {
			t.Errorf("HitTest(right half of the first letter) = %d, %v; want 0, false", got, tr)
		}
		if got, tr := l.HitTest(render.Point{X: x0 + (x1-x0)/4, Y: y}); got != 0 || !tr {
			t.Errorf("HitTest(left half of the first letter) = %d, %v; want 0, true", got, tr)
		}
		if rs := l.SelectionRects(0, len(s)); len(rs) != 1 || !tlNear(rs[0].W, l.Size().W, 0.01) {
			t.Errorf("SelectionRects(all) = %v, want one rect %v wide", rs, l.Size().W)
		}
	})
}

// TestTextPangoLargeText: about 100 KB of wrapped text is a Pango layout
// (no fallback), and NewLayout with its geometry takes well under the
// generous bound of 2 s; the time is logged.
func TestTextPangoLargeText(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		s := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 2250)
		start := time.Now()
		l := e.ts.NewLayout(s, render.Font{}, 300)
		d := time.Since(start)
		t.Logf("TEXT-RESULT %d bytes at width 300: %v, %d lines, in %v", len(s), l.Size(), l.LineCount(), d)
		if _, ok := l.(*textLayout); !ok {
			t.Errorf("a %d-byte layout is a %T, not a Pango layout", len(s), l)
			return
		}
		if d > 2*time.Second {
			t.Errorf("NewLayout of %d bytes took %v, want well under 2 s", len(s), d)
		}
		if l.LineCount() < 100 || l.Size().W > 300+tlEps {
			t.Errorf("%d lines, width %v; want many lines within 300", l.LineCount(), l.Size().W)
		}
		mid := len(s) / 2
		if c := l.CaretRect(mid); !(c.Y > 0) || !tlFinite(c.X, c.Y, c.H) {
			t.Errorf("CaretRect(%d) = %v, want on a later line", mid, c)
		}
	})
}

// tlCheckLines checks the render.LineMetrics contract on a Pango layout
// (as win32's checkLines): the lines cover the text in order, tile the
// layout's height and hold their baselines, the first one's is
// Baseline(), Line clamps, and CaretRect at a line's start is on it. At a
// soft break (a line starting where the one before ends) the upstream
// caret is on the earlier line at its end, the downstream one on the
// later line; elsewhere CaretRectUpstream is CaretRect. It returns the
// soft breaks.
func tlCheckLines(t *testing.T, l *textLayout) []int {
	t.Helper()
	n, text := l.LineCount(), l.Text()
	var soft []int
	top := 0.0
	for i := range n {
		m := l.Line(i)
		t.Logf("TEXT-RESULT %q Line(%d) = %+v", text, i, m)
		if i == 0 && (m.Start != 0 || !tlNear(m.Baseline, l.Baseline(), 0.01)) {
			t.Errorf("%q: Line(0) = %+v, Baseline() %v", text, m, l.Baseline())
		}
		if m.Start > m.End || m.End > len(text) || !tlNear(m.Top, top, 0.01) || !(m.Height > 0) ||
			!(m.Baseline > m.Top && m.Baseline < m.Top+m.Height) {
			t.Errorf("%q: Line(%d) = %+v (top so far %v)", text, i, m, top)
		}
		if i > 0 {
			p := l.Line(i - 1)
			switch {
			case p.End == m.Start:
				soft = append(soft, m.Start)
				up, down := l.CaretRectUpstream(m.Start), l.CaretRect(m.Start)
				if !tlNear(up.Y, p.Top, 0.01) || !tlNear(up.H, p.Height, 0.01) || !tlNear(down.Y, m.Top, 0.01) {
					t.Errorf("%q: at the soft break %d upstream %v, downstream %v; lines %+v %+v", text, m.Start, up, down, p, m)
				}
			case p.End < m.Start && strings.ContainsAny(text[p.End:m.Start], "\r\n\u2028\u2029"):
			default:
				t.Errorf("%q: Line(%d) %+v does not follow %+v", text, i, m, p)
			}
		}
		if c := l.CaretRect(m.Start); !tlNear(c.Y, m.Top, 0.01) {
			t.Errorf("%q: CaretRect(%d) = %v, not on line %d %+v", text, m.Start, c, i, m)
		}
		top += m.Height
	}
	if last := l.Line(n - 1); last.End != len(text) {
		t.Errorf("%q: the last line ends at %d, want %d", text, last.End, len(text))
	}
	if !tlNear(top, l.Size().H, 0.01) {
		t.Errorf("%q: the lines are %v high, the layout %v", text, top, l.Size().H)
	}
	if l.Line(-1) != l.Line(0) || l.Line(n) != l.Line(n-1) {
		t.Errorf("%q: Line does not clamp", text)
	}
	isSoft := map[int]bool{}
	for _, s := range soft {
		isSoft[s] = true
	}
	for i := 0; i <= len(text); i++ {
		if !isSoft[l.normalize(i)] && l.CaretRectUpstream(i) != l.CaretRect(i) {
			t.Errorf("%q: CaretRectUpstream(%d) = %v, CaretRect %v", text, i, l.CaretRectUpstream(i), l.CaretRect(i))
		}
	}
	return soft
}

// TestTextPangoLineMetrics (S5.4b): Line and CaretRectUpstream on wrapped
// Latin with trailing spaces at the wrap, CJK, a bidi line, newlines and
// empty text.
func TestTextPangoLineMetrics(t *testing.T) {
	tlPango(t, func(e *tlEnv) {
		f := render.Font{}
		const s = "hello world  again\nnext"
		world := e.width("hello world", f)
		l := e.geo(s, f, world+1)
		if l == nil {
			return
		}
		soft := tlCheckLines(t, l)
		if l.LineCount() != 3 || len(soft) != 1 || soft[0] != 13 {
			t.Errorf("%q at %v: %d lines, soft breaks %v; want 3 and [13]", s, world+1, l.LineCount(), soft)
		} else {
			up := l.CaretRectUpstream(13)
			t.Logf("TEXT-RESULT upstream caret at the wrap %v (\"hello world\" %v, with the spaces %v)", up, world, e.width("hello world  ", f))
			if !(up.X >= world-tlKern) || l.Line(0).End != 13 || l.Line(1).End != 18 {
				t.Errorf("upstream caret %v, lines %+v %+v", up, l.Line(0), l.Line(1))
			}
		}

		const cjk = "日本語のテキストを折り返す"
		if c := e.geo(cjk, f, e.width("日本語の", f)+0.5); c != nil {
			soft := tlCheckLines(t, c)
			if c.LineCount() < 3 || len(soft) != c.LineCount()-1 {
				t.Errorf("%q: %d lines, soft breaks %v", cjk, c.LineCount(), soft)
			}
			for i := range c.LineCount() {
				if m := c.Line(i); m.Start%3 != 0 || m.End%3 != 0 {
					t.Errorf("%q: Line(%d) = %+v, not at ideograph starts", cjk, i, m)
				}
			}
		}

		const bidi = "ab שלום עולם cd\nאבג"
		for _, w := range []float64{0, e.width("ab שלום", f) + 1} {
			if b := e.geo(bidi, f, w); b != nil {
				soft := tlCheckLines(t, b)
				if w == 0 && (b.LineCount() != 2 || len(soft) != 0) {
					t.Errorf("%q unwrapped: %d lines, soft breaks %v", bidi, b.LineCount(), soft)
				}
				if w > 0 && len(soft) == 0 {
					t.Errorf("%q at %v: no soft break", bidi, w)
				}
			}
		}

		for _, c := range []struct {
			s string
			w float64
		}{{"", 0}, {"", 50}, {"ab\n\ncd\n", 0}, {"a\r\nb", 0}, {"a\u2028b", 0}, {"abcdefghij", e.width("abc", f) - 0.5}} {
			if l := e.geo(c.s, f, c.w); l != nil {
				tlCheckLines(t, l)
			}
		}
	})
}
