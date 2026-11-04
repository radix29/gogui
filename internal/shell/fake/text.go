package fake

import (
	"math"
	"unicode/utf8"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/render"
)

// TextSystem is a render.TextSystem with fixed-width metrics, the same as
// driver/stub's: every rune is one cluster, RuneWidth(font) wide, and every
// line is LineHeight(font) high. The first baseline is at 3/4 of the line
// height. It has no state and may be used from any goroutine.
//
// Wrapping (positive, finite maxWidth) is greedy: a line takes as many runes
// as fit, and breaks after the last run of spaces that starts within them
// (spaces hang past maxWidth and do not count towards the line's width). A
// line without such a space is broken between runes, keeping at least one
// rune per line. Unlike driver/stub's area estimate (ceil(width/maxWidth)
// lines, each maxWidth wide), the result has real line widths; unwrapped
// text measures exactly as the stub does.
//
// Caret positions at a break follow the engines without caret affinity: the
// index where a line starts is drawn at that line's start. Hit-testing right
// of a wrapped line returns the index before its last trailing space (so
// the caret stays on that line), or the next line's start when the line was
// broken between runes.
type TextSystem struct{}

var _ render.TextSystem = (*TextSystem)(nil)

// NewTextSystem returns a fake text system.
func NewTextSystem() *TextSystem { return &TextSystem{} }

// RuneWidth returns the fake width of one rune in f (7 pt at 13 pt).
func RuneWidth(f render.Font) float64 {
	return stub.TextWidth("x", f, driver.ControlSizeRegular)
}

// LineHeight returns the fake line height of f (16 pt at 13 pt).
func LineHeight(f render.Font) float64 {
	return stub.LineHeight(f, driver.ControlSizeRegular)
}

// line is one laid-out line of a layout.
type line struct {
	start, end int     // content [start, end): no '\n', trailing spaces of a wrapped line included
	next       int     // start of the next line; len(text)+1 for the last one
	hitEnd     int     // index HitTest returns right of the line
	newline    bool    // the line ends at a '\n'
	width      float64 // visible width (hanging spaces excluded)
}

type layout struct {
	text   string
	font   render.Font
	cw, lh float64
	lines  []line
	width  float64
}

// NewLayout implements render.TextSystem.
func (*TextSystem) NewLayout(text string, font render.Font, maxWidth float64) render.TextLayout {
	l := &layout{text: text, font: font, cw: RuneWidth(font), lh: LineHeight(font)}
	wrap := maxWidth > 0 && !math.IsInf(maxWidth, 1) // NaN fails the first test
	maxRunes := 0
	if wrap {
		maxRunes = max(1, int(math.Floor(maxWidth/l.cw+1e-9)))
	}
	for ps := 0; ; {
		pe := len(text)
		nl := false
		for i := ps; i < len(text); i++ {
			if text[i] == '\n' {
				pe, nl = i, true
				break
			}
		}
		l.paragraph(ps, pe, nl, maxRunes)
		if !nl {
			break
		}
		ps = pe + 1
	}
	l.lines[len(l.lines)-1].next = len(text) + 1
	for _, ln := range l.lines {
		l.width = max(l.width, ln.width)
	}
	return l
}

// paragraph lays out text[ps:pe], which ends at a '\n' when nl is set.
// maxRunes is the number of runes per line, 0 for no wrapping.
func (l *layout) paragraph(ps, pe int, nl bool, maxRunes int) {
	next := pe
	if nl {
		next = pe + 1
	}
	for pos := ps; ; {
		limit, fits := l.advance(pos, pe, maxRunes)
		if maxRunes == 0 || fits {
			l.lines = append(l.lines, line{start: pos, end: pe, next: next, hitEnd: pe, newline: nl, width: l.x(pos, pe)})
			return
		}
		// The last space run starting within the fitting runes (or right
		// after them, since spaces hang), with content before it.
		vis := -1
		for j := limit; j > pos; j-- {
			if j < pe && l.text[j] == ' ' && l.text[j-1] != ' ' {
				vis = j
				break
			}
		}
		if vis < 0 { // no space to break at: break between runes
			l.lines = append(l.lines, line{start: pos, end: limit, next: limit, hitEnd: limit, width: l.x(pos, limit)})
			pos = limit
			continue
		}
		brk := vis
		for brk < pe && l.text[brk] == ' ' {
			brk++
		}
		if brk == pe { // only spaces after the break: the paragraph's last line
			l.lines = append(l.lines, line{start: pos, end: pe, next: next, hitEnd: pe, newline: nl, width: l.x(pos, vis)})
			return
		}
		l.lines = append(l.lines, line{start: pos, end: brk, next: brk, hitEnd: brk - 1, width: l.x(pos, vis)})
		pos = brk
	}
}

// advance returns the byte index after maxRunes runes from pos (at most pe)
// and whether text[pos:pe] has at most maxRunes runes.
func (l *layout) advance(pos, pe, maxRunes int) (int, bool) {
	i := pos
	for n := 0; n < maxRunes && i < pe; n++ {
		_, size := utf8.DecodeRuneInString(l.text[i:pe])
		i += size
	}
	return i, i >= pe
}

// x returns the width of text[from:to] (both on one line).
func (l *layout) x(from, to int) float64 {
	if to <= from {
		return 0
	}
	return float64(utf8.RuneCountInString(l.text[from:to])) * l.cw
}

// norm clamps i to [0, len(text)] and moves it back to a rune start.
func (l *layout) norm(i int) int {
	i = max(0, min(i, len(l.text)))
	for i > 0 && i < len(l.text) && !utf8.RuneStart(l.text[i]) {
		i--
	}
	return i
}

// lineOf returns the index of the line that shows the caret at index i:
// the last line starting at or before it.
func (l *layout) lineOf(i int) int {
	li := 0
	for k := 1; k < len(l.lines) && l.lines[k].start <= i; k++ {
		li = k
	}
	return li
}

func (l *layout) Text() string           { return l.text }
func (l *layout) Font() render.Font      { return l.font }
func (l *layout) Size() render.Size      { return render.Size{W: l.width, H: float64(len(l.lines)) * l.lh} }
func (l *layout) Baseline() float64      { return l.lh * 0.75 }
func (l *layout) LineCount() int         { return len(l.lines) }
func (l *layout) lineTop(li int) float64 { return float64(li) * l.lh }
func (l *layout) lineRuns(ln line) int   { return utf8.RuneCountInString(l.text[ln.start:ln.hitEnd]) }
func (l *layout) runeStart(ln line, k int) int {
	i := ln.start
	for ; k > 0; k-- {
		_, size := utf8.DecodeRuneInString(l.text[i:])
		i += size
	}
	return i
}

func (l *layout) HitTest(p render.Point) (int, bool) {
	li := 0
	if y := p.Y / l.lh; y > 0 { // NaN and negative hit the first line
		li = min(int(y), len(l.lines)-1)
	}
	ln := l.lines[li]
	if !(p.X > 0) {
		return ln.start, false
	}
	col := p.X / l.cw
	n := l.lineRuns(ln)
	if col >= float64(n) {
		return ln.hitEnd, false
	}
	k := int(col)
	return l.runeStart(ln, k), col-float64(k) >= 0.5
}

func (l *layout) CaretRect(index int) render.Rect {
	i := l.norm(index)
	li := l.lineOf(i)
	return render.Rect{X: l.x(l.lines[li].start, i), Y: l.lineTop(li), H: l.lh}
}

func (l *layout) SelectionRects(start, end int) []render.Rect {
	a, b := l.norm(start), l.norm(end)
	if a > b {
		a, b = b, a
	}
	if a == b {
		return nil
	}
	var rs []render.Rect
	for li, ln := range l.lines {
		if a >= ln.next || b <= ln.start {
			continue
		}
		lo, hi := max(a, ln.start), min(b, ln.end)
		w := l.x(lo, hi)
		if ln.newline && b > ln.end {
			w += l.cw // the selected '\n' shows as one rune
		}
		if w == 0 {
			continue
		}
		rs = append(rs, render.Rect{X: l.x(ln.start, lo), Y: l.lineTop(li), W: w, H: l.lh})
	}
	return rs
}
