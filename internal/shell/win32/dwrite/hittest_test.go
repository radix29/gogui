//go:build windows

package dwrite

import (
	"errors"
	"math"
	"testing"

	"github.com/radix29/gogui/internal/shell/win32/com"
)

// TestClusterFlags checks the bit-field masks of DWRITE_CLUSTER_METRICS:
// MSVC packs the five UINT16 bit fields from the least significant bit in
// declaration order.
func TestClusterFlags(t *testing.T) {
	get := []func(ClusterMetrics) bool{
		ClusterMetrics.CanWrapLineAfter, ClusterMetrics.IsWhitespace, ClusterMetrics.IsNewline,
		ClusterMetrics.IsSoftHyphen, ClusterMetrics.IsRightToLeft,
	}
	for bit := range 16 {
		c := ClusterMetrics{Flags: 1 << bit}
		for i, g := range get {
			if want := bit == i; g(c) != want {
				t.Errorf("Flags 1<<%d: getter %d = %v, want %v", bit, i, g(c), want)
			}
		}
	}
	all := ClusterMetrics{Flags: 0xffff}
	for i, g := range get {
		if !g(all) {
			t.Errorf("Flags 0xffff: getter %d false", i)
		}
	}
}

func clusters(t *testing.T, l TextLayout) []ClusterMetrics {
	t.Helper()
	cs, err := l.ClusterMetrics()
	if err != nil {
		t.Fatal(err)
	}
	return cs
}

func lengths(cs []ClusterMetrics) []uint16 {
	n := make([]uint16, len(cs))
	for i, c := range cs {
		n[i] = c.Length
	}
	return n
}

// TestClusters checks GetClusterMetrics: one cluster per letter of
// "Hello" with widths adding up to the line's, a combining mark and an
// emoji surrogate pair each joined to one cluster of two units, and the
// flags of whitespace, newlines, soft hyphens and right-to-left text.
func TestClusters(t *testing.T) {
	fac, err := CreateFactory()
	if err != nil {
		t.Fatal(err)
	}
	defer fac.Release()
	const size = 13

	hello := layout(t, fac, "Hello", size, FONT_WEIGHT_NORMAL, WORD_WRAPPING_WRAP, 1000, 500)
	defer hello.Release()
	m, _ := metrics(t, hello)
	cs := clusters(t, hello)
	var sum float32
	for i, c := range cs {
		// The text's last cluster may always end a line.
		var wantFlags uint16
		if i == len(cs)-1 {
			wantFlags = clusterCanWrapLineAfter
		}
		if c.Length != 1 || c.Width <= 0 || c.Flags != wantFlags {
			t.Errorf("Hello cluster %d: %+v, want flags %#x", i, c, wantFlags)
		}
		sum += c.Width
	}
	if len(cs) != 5 || !near(sum, m.Width, 0.01) {
		t.Errorf("Hello: %d clusters, widths %v, line %v", len(cs), sum, m.Width)
	}

	for _, c := range []struct {
		name, text string
		want       []uint16
	}{
		{"combining mark", "e\u0301x", []uint16{2, 1}},
		{"precomposed", "\u00e9x", []uint16{1, 1}},
		{"emoji", "a\U0001F600b", []uint16{1, 2, 1}},
	} {
		l := layout(t, fac, c.text, size, FONT_WEIGHT_NORMAL, WORD_WRAPPING_WRAP, 1000, 500)
		got := lengths(clusters(t, l))
		if len(got) != len(c.want) {
			t.Errorf("%s: cluster lengths %v, want %v", c.name, got, c.want)
		} else {
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("%s: cluster lengths %v, want %v", c.name, got, c.want)
					break
				}
			}
		}
		release(t, c.name, &l.Ptr, 0)
	}

	// Flags: "a b\nc", a soft hyphen, "d" and a Hebrew alef.
	fl := layout(t, fac, "a b\nc\u00add\u05d0", size, FONT_WEIGHT_NORMAL, WORD_WRAPPING_WRAP, 1000, 500)
	defer fl.Release()
	fc := clusters(t, fl)
	if len(fc) != 8 {
		t.Fatalf("flags text: %d clusters, %+v", len(fc), fc)
	}
	if c := fc[0]; c.IsWhitespace() || c.IsNewline() || c.CanWrapLineAfter() || c.IsRightToLeft() {
		t.Errorf("'a': %+v", c)
	}
	if c := fc[1]; !c.IsWhitespace() || !c.CanWrapLineAfter() || c.IsNewline() {
		t.Errorf("' ': %+v", c)
	}
	if c := fc[3]; !c.IsNewline() || !c.IsWhitespace() || !c.CanWrapLineAfter() {
		t.Errorf("'\\n': %+v", c)
	}
	if c := fc[5]; !c.IsSoftHyphen() || !c.CanWrapLineAfter() || c.IsWhitespace() {
		t.Errorf("soft hyphen: %+v", c)
	}
	if c := fc[7]; !c.IsRightToLeft() || c.IsWhitespace() {
		t.Errorf("alef: %+v", c)
	}
	for i, c := range fc[:7] {
		if c.IsRightToLeft() || c.IsSoftHyphen() != (i == 5) || c.IsNewline() != (i == 3) {
			t.Errorf("cluster %d: %+v", i, c)
		}
	}

	// "\r\n" is two clusters, not one: the '\r' is whitespace only, the
	// '\n' carries the newline (and the line holds both, NewlineLength 2).
	crlf := layout(t, fac, "a\r\nb", size, FONT_WEIGHT_NORMAL, WORD_WRAPPING_WRAP, 1000, 500)
	cc := clusters(t, crlf)
	_, cl := metrics(t, crlf)
	if len(cc) != 4 || !cc[1].IsWhitespace() || cc[1].IsNewline() || !cc[2].IsNewline() ||
		!cc[2].CanWrapLineAfter() || len(cl) != 2 || cl[0].NewlineLength != 2 {
		t.Errorf("CRLF: %+v, lines %+v", cc, cl)
	}
	release(t, "CRLF", &crlf.Ptr, 0)

	empty := layout(t, fac, "", size, FONT_WEIGHT_NORMAL, WORD_WRAPPING_WRAP, 1000, 500)
	if ec := clusters(t, empty); len(ec) != 0 {
		t.Errorf("empty text: clusters %+v", ec)
	}
	release(t, "empty", &empty.Ptr, 0)
}

// TestHitTest checks HitTestPoint (both halves of every glyph of "Hello",
// points outside the text), HitTestTextPosition (leading and trailing
// edges at the cluster boundaries), HitTestTextRange (one rect within a
// line, two over a line break, the origin offset) and SetMaxWidth.
func TestHitTest(t *testing.T) {
	fac, err := CreateFactory()
	if err != nil {
		t.Fatal(err)
	}
	defer fac.Release()
	const size = 13

	hello := layout(t, fac, "Hello", size, FONT_WEIGHT_NORMAL, WORD_WRAPPING_WRAP, 1000, 500)
	defer hello.Release()
	m, lines := metrics(t, hello)
	cs := clusters(t, hello)
	if len(cs) != 5 {
		t.Fatalf("Hello: %d clusters", len(cs))
	}
	midY := m.Height / 2
	var x float32
	for i, c := range cs {
		for _, half := range []struct {
			frac     float32
			trailing bool
		}{{0.25, false}, {0.75, true}} {
			h, err := hello.HitTestPoint(x+half.frac*c.Width, midY)
			if err != nil {
				t.Fatal(err)
			}
			hm := h.Metrics
			if hm.TextPosition != uint32(i) || h.Trailing != half.trailing || !h.Inside || hm.Length != 1 ||
				!near(hm.Left, x, 0.01) || !near(hm.Width, c.Width, 0.01) || hm.Top != 0 ||
				!near(hm.Height, lines[0].Height, 0.01) || hm.IsText == 0 || hm.BidiLevel != 0 {
				t.Errorf("glyph %d at %.2f of its width: %+v", i, half.frac, h)
			}
		}
		// The caret edges of position i.
		lx, ly, lm, err := hello.HitTestTextPosition(uint32(i), false)
		if err != nil {
			t.Fatal(err)
		}
		tx, ty, _, err := hello.HitTestTextPosition(uint32(i), true)
		if err != nil {
			t.Fatal(err)
		}
		if !near(lx, x, 0.01) || !near(tx, x+c.Width, 0.01) || ly != 0 || ty != 0 ||
			lm.TextPosition != uint32(i) || lm.Length != 1 {
			t.Errorf("position %d: leading (%v, %v), trailing (%v, %v), %+v; want x %v and %v",
				i, lx, ly, tx, ty, lm, x, x+c.Width)
		}
		x += c.Width
	}

	// Outside the text: the nearest cluster, Inside false.
	for _, c := range []struct {
		name     string
		x, y     float32
		pos      uint32
		trailing bool
	}{
		{"right", m.Width + 50, midY, 4, true},
		{"left", -50, midY, 0, false},
		{"below", 1, m.Height + 50, 0, false},
		{"above", m.Width - 1, -50, 4, true},
	} {
		h, err := hello.HitTestPoint(c.x, c.y)
		if err != nil {
			t.Fatal(err)
		}
		if h.Inside || h.Metrics.TextPosition != c.pos || h.Trailing != c.trailing {
			t.Errorf("%s of the text: %+v; want position %d, trailing %v", c.name, h, c.pos, c.trailing)
		}
	}
	// A position past the end gives the end.
	if ex, _, _, err := hello.HitTestTextPosition(99, false); err != nil || !near(ex, m.Width, 0.01) {
		t.Errorf("position 99: x %v, %v; want %v", ex, err, m.Width)
	}

	// A range within one line: one rect from the start of 'e' to the end
	// of 'l' (the second); the origin moves it.
	r, err := hello.HitTestTextRange(1, 3, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	wantL, wantW := cs[0].Width, cs[1].Width+cs[2].Width+cs[3].Width
	if len(r) != 1 || r[0].TextPosition != 1 || r[0].Length != 3 || !near(r[0].Left, wantL, 0.01) ||
		!near(r[0].Width, wantW, 0.01) || r[0].Top != 0 || r[0].IsText == 0 {
		t.Errorf("range 1+3: %+v; want left %v, width %v", r, wantL, wantW)
	}
	ro, err := hello.HitTestTextRange(1, 3, 100.5, 20.25)
	if err != nil {
		t.Fatal(err)
	}
	if len(ro) != 1 || !near(ro[0].Left, wantL+100.5, 0.01) || !near(ro[0].Top, 20.25, 0.01) ||
		!near(ro[0].Width, wantW, 0.01) {
		t.Errorf("range 1+3 at (100.5, 20.25): %+v", ro)
	}

	// A range over a line break: two rects, one per line.
	two := layout(t, fac, "Hello\nWorld", size, FONT_WEIGHT_NORMAL, WORD_WRAPPING_WRAP, 1000, 500)
	defer two.Release()
	_, lines2 := metrics(t, two)
	r2, err := two.HitTestTextRange(3, 5, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("range over the line break: %+v", r2)
	if len(r2) != 2 || r2[0].Top != 0 || !near(r2[1].Top, lines2[0].Height, 0.01) ||
		r2[0].TextPosition != 3 || r2[0].Length != 3 || r2[1].TextPosition != 6 || r2[1].Length != 2 ||
		r2[1].Left != 0 || r2[0].Width <= 0 || r2[1].Width <= 0 {
		t.Errorf("range 3+5 over the line break: %+v", r2)
	}
	// On the second line, a point maps back into it.
	if h, err := two.HitTestPoint(1, lines2[0].Height+lines2[1].Height/2); err != nil ||
		h.Metrics.TextPosition != 6 || h.Trailing || !h.Inside {
		t.Errorf("second line start: %+v, %v", h, err)
	}

	// SetMaxWidth (float path): narrowing wraps "Hello World", widening
	// unwraps it; negative and NaN widths are refused.
	hw := layout(t, fac, "Hello World", size, FONT_WEIGHT_NORMAL, WORD_WRAPPING_WRAP, 1000, 500)
	defer hw.Release()
	if err := hw.SetMaxWidth(1.5 * m.Width); err != nil {
		t.Fatal(err)
	}
	if mw, _ := metrics(t, hw); mw.LineCount != 2 || hw.MaxWidth() != 1.5*m.Width {
		t.Errorf("SetMaxWidth(%v): %+v, MaxWidth %v", 1.5*m.Width, mw, hw.MaxWidth())
	}
	if err := hw.SetMaxWidth(1000); err != nil {
		t.Fatal(err)
	}
	if mw, _ := metrics(t, hw); mw.LineCount != 1 || hw.MaxWidth() != 1000 {
		t.Errorf("SetMaxWidth(1000): %+v", mw)
	}
	for _, w := range []float32{-1, float32(math.NaN())} {
		if err := hw.SetMaxWidth(w); !errors.Is(err, com.E_INVALIDARG) || hw.MaxWidth() != 1000 {
			t.Errorf("SetMaxWidth(%v) = %v, MaxWidth %v", w, err, hw.MaxWidth())
		}
	}

	// Empty text: the caret at 0, a point anywhere gives position 0.
	empty := layout(t, fac, "", size, FONT_WEIGHT_NORMAL, WORD_WRAPPING_WRAP, 1000, 500)
	defer empty.Release()
	if h, err := empty.HitTestPoint(10, 5); err != nil || h.Metrics.TextPosition != 0 || h.Inside {
		t.Errorf("empty text, point: %+v, %v", h, err)
	}
	if ex, ey, _, err := empty.HitTestTextPosition(0, false); err != nil || ex != 0 || ey != 0 {
		t.Errorf("empty text, position 0: (%v, %v), %v", ex, ey, err)
	}
	if er, err := empty.HitTestTextRange(0, 0, 0, 0); err != nil {
		t.Errorf("empty text, range: %v", err)
	} else {
		t.Logf("empty text, range 0+0: %+v", er)
	}
}
