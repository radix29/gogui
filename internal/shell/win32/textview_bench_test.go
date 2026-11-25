//go:build windows

package win32

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// The 1 MB benchmarks of the aqua TextView over real DirectWrite and
// Direct2D (PLAN.md S5.4k): the same operations as the fake-shell ones in
// internal/aqua (textview_bench_test.go), each with its frame: the
// driver's frame on the shell's clock and a WM_PAINT that draws through
// the D2D canvas and presents (on the default device, the GPU when there
// is one). The view has its size in examples/notes (539×451) in a hidden,
// key window; the handler reads the whole text on every change, as the
// gogui TextView does. The budget is one frame (16 ms) for each and 2 ms
// for a keystroke.

// megaText is about 1 MB of paragraphs of 0 to 80 words (as megaText in
// internal/aqua).
func megaText(seed int64) string {
	r := rand.New(rand.NewSource(seed))
	words := []string{"the", "quick", "brown", "fox", "jumps", "over", "a", "lazy", "dog", "lorem", "ipsum"}
	var sb strings.Builder
	for sb.Len() < 1<<20 {
		for n := r.Intn(81); n > 0; n-- {
			sb.WriteString(words[r.Intn(len(words))])
			if n > 1 {
				sb.WriteByte(' ')
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// viewReader is a TextViewHandler that reads the whole text on every
// change, as gogui's textViewEvents does.
type viewReader struct {
	v *aqua.TextView
	n int
}

func (r *viewReader) TextChanged() { r.n += len(r.v.Text()) }

// textBench is a running aqua driver with the 1 MB text view.
type textBench struct {
	b  *testing.B
	s  *Shell
	sf *surface
	v  *aqua.TextView
}

// frame runs a frame on the shell's clock and paints the window through
// WM_PAINT, which must draw without errors. The time from EndDraw to the
// end of WM_PAINT (Present, which waits for the vertical blank with sync
// interval 1, and the composition's Commit) is not timed: it is the frame
// pacing, about one display period whatever was drawn.
func (tb *textBench) frame() { tb.paint(true) }

// setupFrame is frame for the untimed parts of a benchmark.
func (tb *textBench) setupFrame() { tb.paint(false) }

func (tb *textBench) paint(timed bool) {
	tb.s.clock.requested = true
	tb.s.clock.frame()
	rethrow()
	drawn := false
	tb.s.hooks.onDrawn = func(d *swapTarget) {
		if d == tb.sf.swap() {
			drawn = true
			if timed {
				tb.b.StopTimer()
			}
		}
	}
	w32.SendMessage(tb.sf.hwnd, w32.WM_PAINT, 0, 0)
	tb.s.hooks.onDrawn = nil
	if timed {
		tb.b.StartTimer()
	}
	rethrow()
	if !drawn || tb.sf.renderErr != nil || tb.sf.paintErr != nil {
		tb.b.Fatalf("drawn %v: render error %v, paint error %v", drawn, tb.sf.renderErr, tb.sf.paintErr)
	}
}

func (tb *textBench) key(k driver.Key, mods driver.Modifiers) {
	tb.sf.h.Event(shell.KeyEvent{Down: true, Key: k, Mods: mods})
	tb.sf.h.Event(shell.KeyEvent{Key: k, Mods: mods})
}

func (tb *textBench) typeText(s string) { tb.sf.h.Event(shell.TextEvent{Text: s}) }

// withTextBench runs an aqua driver on the shell (the recorder checks off,
// as in an app), opens the window with the view focused, the 1 MB text
// loaded, the caret in the middle and the frame painted, and calls fn on
// the UI goroutine.
func withTextBench(b *testing.B, fn func(tb *textBench)) {
	b.Helper()
	s := New()
	d := aqua.New(s)
	err := d.Run(driver.AppConfig{Name: "TextBench"}, galleryApp{ready: func() {
		defer d.Quit()
		w, err := d.NewWindow(driver.WindowConfig{Title: "Notes", Size: driver.Size{W: 720, H: 480},
			Style: driver.StyleTitled | driver.StyleClosable | driver.StyleResizable}, winHandler{})
		if err != nil {
			b.Error(err)
			return
		}
		defer w.Close()
		win := w.(*aqua.Window)
		sf := win.Surface().(*surface)
		w32.SendMessage(sf.hwnd, w32.WM_ACTIVATE, w32.WA_ACTIVE, 0)
		rethrow()
		h := &viewReader{}
		v := d.NewTextView(h).(*aqua.TextView)
		h.v = v
		win.SetContent(v)
		v.SetFrame(driver.Rect{X: 181, W: 539, H: 451})
		v.Focus()
		v.SetText(megaText(1))
		mid := v.Editor().Len() / 2
		v.SetSelection(mid, mid)
		tb := &textBench{b: b, s: s, sf: sf, v: v}
		tb.setupFrame()
		fn(tb)
	}})
	if err != nil {
		b.Fatal(err)
	}
}

// BenchmarkTextViewLoad: SetText of 1 MB and the first frame.
func BenchmarkTextViewLoad(b *testing.B) {
	txt := megaText(1)
	withTextBench(b, func(tb *textBench) {
		b.ReportAllocs()
		for b.Loop() {
			tb.v.SetText(txt)
			tb.frame()
		}
	})
}

// BenchmarkTextViewKeystroke: a typed character in the middle (the edit,
// the relayout of its paragraph, TextChanged reading the text, the frame),
// every other one deleted by Backspace.
func BenchmarkTextViewKeystroke(b *testing.B) {
	withTextBench(b, func(tb *textBench) {
		n0 := tb.v.Editor().Len()
		b.ReportAllocs()
		n := 0
		for b.Loop() {
			if n%2 == 0 {
				tb.typeText("x")
			} else {
				tb.key(driver.KeyBackspace, 0)
			}
			n++
			tb.frame()
		}
		if d := tb.v.Editor().Len() - n0; d != n%2 {
			b.Fatalf("the keystrokes changed the length by %d", d)
		}
	})
}

// BenchmarkTextViewPageDown: Page Down from the top to the end, over
// estimated heights, then back to the top.
func BenchmarkTextViewPageDown(b *testing.B) {
	withTextBench(b, func(tb *textBench) {
		tb.v.SetSelection(0, 0)
		tb.setupFrame()
		b.ReportAllocs()
		for b.Loop() {
			if ed := tb.v.Editor(); ed.Head() == ed.Len() {
				tb.v.SetSelection(0, 0)
			}
			tb.key(driver.KeyPageDown, 0)
			tb.frame()
		}
	})
}

// BenchmarkTextViewEnd: End from the top of a freshly loaded text (every
// height below an estimate) to its end.
func BenchmarkTextViewEnd(b *testing.B) {
	txt := megaText(1)
	withTextBench(b, func(tb *textBench) {
		b.ReportAllocs()
		for b.Loop() {
			b.StopTimer()
			tb.v.SetText(txt)
			tb.v.SetSelection(0, 0)
			tb.setupFrame()
			b.StartTimer()
			tb.key(driver.KeyEnd, 0)
			tb.frame()
		}
		if ed := tb.v.Editor(); ed.Head() != ed.Len() {
			b.Fatal("End did not reach the end")
		}
	})
}

// BenchmarkTextViewResize: the view 100 pt narrower or wider in the middle
// of the text (every layout dropped, the visible ones made again, the
// scroll anchored).
func BenchmarkTextViewResize(b *testing.B) {
	withTextBench(b, func(tb *textBench) {
		b.ReportAllocs()
		n := 0
		for b.Loop() {
			w := 539.0
			if n%2 == 0 {
				w = 439
			}
			n++
			tb.v.SetFrame(driver.Rect{X: 181, W: w, H: 451})
			tb.frame()
		}
	})
}

// BenchmarkTextViewSelectAll: Ctrl-A in the middle of the text, every
// other one collapsed by Left.
func BenchmarkTextViewSelectAll(b *testing.B) {
	withTextBench(b, func(tb *textBench) {
		b.ReportAllocs()
		n := 0
		for b.Loop() {
			if n%2 == 0 {
				tb.key('A', driver.ModControl)
			} else {
				tb.key(driver.KeyLeft, 0)
			}
			n++
			tb.frame()
		}
	})
}

// BenchmarkTextViewIdleTick: one tick of the idle layout pass on a freshly
// loaded text (reloaded, untimed, once every height is exact).
func BenchmarkTextViewIdleTick(b *testing.B) {
	txt := megaText(1)
	withTextBench(b, func(tb *textBench) {
		b.ReportAllocs()
		for b.Loop() {
			if tb.v.IdleLayoutTick() == 0 {
				b.StopTimer()
				tb.v.SetText(txt)
				tb.setupFrame()
				b.StartTimer()
			}
			tb.frame()
		}
	})
}

// longLine is about 1 MB of words without a newline: one paragraph (a
// minified file, a log line).
func longLine(seed int64) string {
	r := rand.New(rand.NewSource(seed))
	words := []string{"the", "quick", "brown", "fox", "jumps", "over", "a", "lazy", "dog", "lorem", "ipsum"}
	var sb strings.Builder
	for sb.Len() < 1<<20 {
		sb.WriteString(words[r.Intn(len(words))])
		sb.WriteByte(' ')
	}
	return sb.String()
}

// BenchmarkTextViewKeystrokeLongParagraph: BenchmarkTextViewKeystroke in
// one 1 MB paragraph (review P1: the chunks around the caret are laid out
// again, not the paragraph).
func BenchmarkTextViewKeystrokeLongParagraph(b *testing.B) {
	withTextBench(b, func(tb *textBench) {
		tb.v.SetText(longLine(1))
		mid := tb.v.Editor().Len() / 2
		tb.v.SetSelection(mid, mid)
		tb.setupFrame()
		b.ReportAllocs()
		n := 0
		for b.Loop() {
			if n%2 == 0 {
				tb.typeText("x")
			} else {
				tb.key(driver.KeyBackspace, 0)
			}
			n++
			tb.frame()
		}
	})
}
