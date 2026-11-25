package aqua

import (
	"math/rand"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell/fake"
)

// The 1 MB benchmarks of the TextView (PLAN.md S5.4k) over the fake shell:
// the view at the size it has in examples/notes (539×451), focused in the
// key window, on megaText(1) (about 1 MB in 20 000 paragraphs), each
// operation with its frame (the paint into the fake's recorder). The
// handler reads the whole text on every change, as the gogui TextView
// does. The same operations over real DirectWrite and Direct2D are in
// internal/shell/win32 (textview_bench_test.go); the budget is one frame
// (16 ms) for each and 2 ms for a keystroke.

// textReader is a TextViewHandler that reads the whole text on every
// change, as gogui's textViewEvents does.
type textReader struct {
	v *TextView
	n int
}

func (r *textReader) TextChanged() { r.n += len(r.v.Text()) }

// benchView opens the notes-sized view, focused, with megaText(1) loaded,
// the caret in the middle and the frame painted.
func benchView(b *testing.B) (*TextView, *fake.Surface, *fake.Shell) {
	b.Helper()
	s := startShell(b)
	d := newDriver(s)
	w, sf, _ := openWindow(b, s, driver.WindowConfig{Size: driver.Size{W: 720, H: 480}, Style: allStyles})
	h := &textReader{}
	v := d.NewTextView(h).(*TextView)
	h.v = v
	w.SetContent(v)
	v.SetFrame(driver.Rect{X: 181, W: 539, H: 451})
	if !v.view.Focus() {
		b.Fatal("view not focused")
	}
	v.SetText(megaText(1))
	mid := v.Editor().Len() / 2
	v.SetSelection(mid, mid)
	benchFrame(b, s, sf)
	return v, sf, s
}

// benchFrame runs the pending frame and checks that it painted cleanly.
func benchFrame(b *testing.B, s *fake.Shell, sf *fake.Surface) {
	s.FakeClock().Flush()
	if err := sf.PaintErr(); err != nil {
		b.Fatal(err)
	}
}

// BenchmarkTextViewLoad: SetText of 1 MB and the first frame.
func BenchmarkTextViewLoad(b *testing.B) {
	v, sf, s := benchView(b)
	txt := megaText(1)
	b.ReportAllocs()
	for b.Loop() {
		v.SetText(txt)
		benchFrame(b, s, sf)
	}
}

// BenchmarkTextViewKeystroke: a typed character in the middle (the edit,
// the relayout of its paragraph, TextChanged reading the text, the frame),
// every other one deleted by Backspace.
func BenchmarkTextViewKeystroke(b *testing.B) {
	v, sf, s := benchView(b)
	b.ReportAllocs()
	n := 0
	for b.Loop() {
		if n%2 == 0 {
			sf.Type("x")
		} else {
			sf.Key(driver.KeyBackspace, 0)
		}
		n++
		benchFrame(b, s, sf)
	}
	if v.Editor().Len() > len(megaText(1))+1 {
		b.Fatal("the keystrokes did not edit")
	}
}

// BenchmarkTextViewPageDown: Page Down from the top to the end, over
// estimated heights, then back to the top.
func BenchmarkTextViewPageDown(b *testing.B) {
	v, sf, s := benchView(b)
	v.SetSelection(0, 0)
	benchFrame(b, s, sf)
	b.ReportAllocs()
	for b.Loop() {
		if ed := v.Editor(); ed.Head() == ed.Len() {
			v.SetSelection(0, 0)
		}
		sf.Key(driver.KeyPageDown, 0)
		benchFrame(b, s, sf)
	}
}

// BenchmarkTextViewEnd: End from the top of a freshly loaded text (every
// height below an estimate) to its end.
func BenchmarkTextViewEnd(b *testing.B) {
	v, sf, s := benchView(b)
	txt := megaText(1)
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		v.SetText(txt)
		v.SetSelection(0, 0)
		benchFrame(b, s, sf)
		b.StartTimer()
		sf.Key(driver.KeyEnd, 0)
		benchFrame(b, s, sf)
	}
}

// BenchmarkTextViewResize: the view 100 pt narrower or wider in the middle
// of the text (every layout dropped, the visible ones made again, the
// scroll anchored).
func BenchmarkTextViewResize(b *testing.B) {
	v, sf, s := benchView(b)
	b.ReportAllocs()
	n := 0
	for b.Loop() {
		w := 539.0
		if n%2 == 0 {
			w = 439
		}
		n++
		v.SetFrame(driver.Rect{X: 181, W: w, H: 451})
		benchFrame(b, s, sf)
	}
}

// BenchmarkTextViewSelectAll: Ctrl-A in the middle of the text, every
// other one collapsed by Left.
func BenchmarkTextViewSelectAll(b *testing.B) {
	_, sf, s := benchView(b)
	b.ReportAllocs()
	n := 0
	for b.Loop() {
		if n%2 == 0 {
			sf.Key('a', ctrl)
		} else {
			sf.Key(driver.KeyLeft, 0)
		}
		n++
		benchFrame(b, s, sf)
	}
}

// BenchmarkTextViewIdleTick: one tick of the idle layout pass on a freshly
// loaded text (reloaded, untimed, once every height is exact).
func BenchmarkTextViewIdleTick(b *testing.B) {
	v, sf, s := benchView(b)
	txt := megaText(1)
	b.ReportAllocs()
	for b.Loop() {
		if v.IdleLayoutTick() == 0 {
			b.StopTimer()
			v.SetText(txt)
			benchFrame(b, s, sf)
			b.StartTimer()
		}
		benchFrame(b, s, sf)
	}
}

// BenchmarkTextViewKeystrokeLongParagraph: BenchmarkTextViewKeystroke in
// one 1 MB paragraph (a minified file, a log line; review P1): the
// relayout of the chunks around the caret and the frame.
func BenchmarkTextViewKeystrokeLongParagraph(b *testing.B) {
	v, sf, s := benchView(b)
	v.SetText(longParagraph(rand.New(rand.NewSource(3)), 1<<20))
	mid := v.Editor().Len() / 2
	v.SetSelection(mid, mid)
	benchFrame(b, s, sf)
	b.ReportAllocs()
	n := 0
	for b.Loop() {
		if n%2 == 0 {
			sf.Type("x")
		} else {
			sf.Key(driver.KeyBackspace, 0)
		}
		n++
		benchFrame(b, s, sf)
	}
}
