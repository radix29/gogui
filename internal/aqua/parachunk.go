package aqua

import (
	"sort"
	"unicode/utf8"

	"github.com/radix29/gogui/internal/render"
)

// Long paragraphs in chunks (review P1). A TextView paragraph longer than
// longParaMin bytes (minified JSON, a log line) is laid out as a
// chunkedLayout: a column of layouts of about longParaChunk each, so that
// an edit lays out again only the chunks around it, not the whole
// paragraph (one layout of a 1 MB line takes about a second).
//
// The chunks start at line starts of the whole paragraph's wrapping, so the
// lines are the ones one layout would make: a chunk lays out its text up to
// about longParaChunk and shows all its lines but the last, and the next
// chunk starts where that hidden last line starts. Wrapping is greedy in
// every text engine (DirectWrite, Pango, the fixed-width one), so the lines
// from a line start on depend only on the text from there, and a cut text
// changes at most its last line. A chunk that is one line (a wrap width
// wider than longParaChunk of text) cannot hide one and ends at its cut, a
// space where it can.
//
// After an edit, relayoutPara keeps the chunks before the line ahead of the
// edited word, lays out from there and stops as soon as a new chunk starts
// where an old one after the edit starts (moved by the edit): from there on
// the text and so the lines are the old ones.
//
// The paragraph's base direction comes from each chunk's own first strong
// character, not the paragraph's; mixed right-to-left text in one long
// paragraph can differ from one layout there.
const (
	longParaChunk = 1024
	longParaMin   = 4 * longParaChunk
)

// chunkedLayout is a render.TextLayout over a paragraph's chunks. Canvases
// draw only their own layouts, so the TextView draws it chunk by chunk
// (drawParagraph). Size().W is the widest chunk layout, which can count a
// hidden last line; the TextView reads only the height.
type chunkedLayout struct {
	text   string
	font   render.Font
	width  float64
	chunks []layoutChunk
	size   render.Size
	lines  int
}

// layoutChunk is one chunk: its layout of text[off:], cut about
// longParaChunk on, showing lines lines (all but a hidden last one except
// in the last chunk) that end at end, the next chunk's off.
type layoutChunk struct {
	l        render.TextLayout
	off, end int
	lines    int
	line0    int     // the paragraph's index of its first line
	y, h     float64 // the top and the height of its shown lines in the paragraph
}

var _ render.TextLayout = (*chunkedLayout)(nil)

// newParaLayout lays out the paragraph s in font wrapped to width: in
// chunks when it is long and wrapped, else as one layout.
func newParaLayout(ts render.TextSystem, s string, font render.Font, width float64) render.TextLayout {
	if width <= 0 || len(s) <= longParaMin {
		return ts.NewLayout(s, font, width)
	}
	cl := &chunkedLayout{text: s, font: font, width: width}
	cl.build(ts, nil, 0, nil)
	return cl
}

// relayoutPara returns the layout of the paragraph s, which was old's text
// with [start, oldEnd) replaced by s[start:newEnd], in font wrapped to
// width: a chunked old layout keeps its chunks away from the edit.
func relayoutPara(ts render.TextSystem, old render.TextLayout, s string, font render.Font, width float64, start, oldEnd, newEnd int) render.TextLayout {
	ol, ok := old.(*chunkedLayout)
	if !ok || width <= 0 || len(s) <= longParaMin || ol.font != font || ol.width != width {
		return newParaLayout(ts, s, font, width)
	}
	// Restart at the chunk holding the start of the line before the one
	// that holds the start of the edited word: the edit can pull that word
	// back onto the line before.
	lo := start
	for k := 0; lo > 0 && k < 256 && ol.text[lo-1] != ' ' && ol.text[lo-1] != '\t'; k++ {
		lo--
	}
	ln := max(lineAt(ol, max(lo-1, 0), false)-1, 0)
	r := ol.chunkAtOffset(ol.Line(ln).Start)
	// The old chunks after the edit, moved by it, may be taken over.
	delta := newEnd - oldEnd
	var tail []layoutChunk
	for _, c := range ol.chunks[r:] {
		if c.off >= oldEnd {
			c.off += delta
			c.end += delta
			tail = append(tail, c)
		}
	}
	cl := &chunkedLayout{text: s, font: font, width: width}
	cl.build(ts, ol.chunks[:r], ol.chunks[r].off, tail)
	return cl
}

// build lays out the chunks from pos on after the kept chunks prefix,
// taking over the chunks of tail (in this text's offsets) once one starts
// where a new chunk would, and sums up the chunks.
func (cl *chunkedLayout) build(ts render.TextSystem, prefix []layoutChunk, pos int, tail []layoutChunk) {
	s := cl.text
	cl.chunks = append(make([]layoutChunk, 0, len(prefix)+len(tail)+4), prefix...)
	j := 0
	for pos < len(s) {
		for j < len(tail) && tail[j].off < pos {
			j++
		}
		if j < len(tail) && tail[j].off == pos {
			cl.chunks = append(cl.chunks, tail[j:]...)
			break
		}
		cut := chunkCut(s, pos)
		l := ts.NewLayout(s[pos:cut], cl.font, cl.width)
		c := layoutChunk{l: l, off: pos, end: cut, lines: l.LineCount()}
		if n := l.LineCount(); cut < len(s) && n > 1 {
			if next := l.Line(n - 1).Start; next > 0 {
				c.lines, c.end = n-1, pos+next
			}
		}
		cl.chunks = append(cl.chunks, c)
		pos = c.end
	}
	cl.sum()
}

// chunkCut returns where a chunk from pos is cut: the paragraph's end when
// less than one and a half chunks are left, else after the last space in
// the second half of longParaChunk from pos, or at a rune start there.
func chunkCut(s string, pos int) int {
	if len(s)-pos <= longParaChunk*3/2 {
		return len(s)
	}
	end := pos + longParaChunk
	for i := end; i > pos+longParaChunk/2; i-- {
		if s[i-1] == ' ' && s[i] != ' ' {
			return i
		}
	}
	for end < len(s) && !utf8.RuneStart(s[end]) {
		end++
	}
	return end
}

// sum sets the chunks' line and y offsets and the layout's size.
func (cl *chunkedLayout) sum() {
	y, lines, w := 0.0, 0, 0.0
	for i := range cl.chunks {
		c := &cl.chunks[i]
		first, last := c.l.Line(0), c.l.Line(c.lines-1)
		c.line0, c.y, c.h = lines, y, last.Top+last.Height-first.Top
		y += c.h
		lines += c.lines
		w = max(w, c.l.Size().W)
	}
	cl.size, cl.lines = render.Size{W: w, H: y}, lines
}

// dy is what moves chunk c's own coordinates into the paragraph's.
func (c *layoutChunk) dy() float64 { return c.y - c.l.Line(0).Top }

// chunkAtOffset returns the chunk holding the caret at off: the one whose
// shown text [off, end) holds it, the last one at the paragraph's end.
func (cl *chunkedLayout) chunkAtOffset(off int) int {
	i := sort.Search(len(cl.chunks), func(i int) bool { return cl.chunks[i].end > off })
	return min(i, len(cl.chunks)-1)
}

// chunkAtY returns the chunk whose shown lines hold y: the first above
// them, the last below.
func (cl *chunkedLayout) chunkAtY(y float64) int {
	i := sort.Search(len(cl.chunks), func(i int) bool { return cl.chunks[i].y > y }) - 1
	return max(i, 0)
}

func (cl *chunkedLayout) Text() string      { return cl.text }
func (cl *chunkedLayout) Font() render.Font { return cl.font }
func (cl *chunkedLayout) Size() render.Size { return cl.size }
func (cl *chunkedLayout) LineCount() int    { return cl.lines }
func (cl *chunkedLayout) Baseline() float64 { return cl.Line(0).Baseline }

// Line implements render.TextLayout.
func (cl *chunkedLayout) Line(k int) render.LineMetrics {
	k = min(max(k, 0), cl.lines-1)
	i := sort.Search(len(cl.chunks), func(i int) bool { return cl.chunks[i].line0 > k }) - 1
	c := &cl.chunks[i]
	m := c.l.Line(k - c.line0)
	dy := c.dy()
	m.Start += c.off
	m.End += c.off
	m.Top += dy
	m.Baseline += dy
	return m
}

// HitTest implements render.TextLayout.
func (cl *chunkedLayout) HitTest(p render.Point) (int, bool) {
	c := &cl.chunks[cl.chunkAtY(p.Y)]
	i, trailing := c.l.HitTest(render.Point{X: p.X, Y: p.Y - c.dy()})
	return min(c.off+i, c.end), trailing
}

// CaretRect implements render.TextLayout.
func (cl *chunkedLayout) CaretRect(off int) render.Rect {
	c := &cl.chunks[cl.chunkAtOffset(off)]
	r := c.l.CaretRect(off - c.off)
	r.Y += c.dy()
	return r
}

// CaretRectUpstream implements render.TextLayout: at a chunk's start, the
// end of the line before, in the chunk before.
func (cl *chunkedLayout) CaretRectUpstream(off int) render.Rect {
	i := cl.chunkAtOffset(off)
	if i > 0 && off == cl.chunks[i].off {
		i--
	}
	c := &cl.chunks[i]
	r := c.l.CaretRectUpstream(off - c.off)
	r.Y += c.dy()
	return r
}

// SelectionRects implements render.TextLayout, chunk by chunk, without a
// chunk's hidden last line.
func (cl *chunkedLayout) SelectionRects(start, end int) []render.Rect {
	if start > end {
		start, end = end, start
	}
	if start == end {
		return nil
	}
	var rs []render.Rect
	for i := cl.chunkAtOffset(start); i < len(cl.chunks); i++ {
		c := &cl.chunks[i]
		if c.off >= end {
			break
		}
		lo, hi := max(start, c.off), min(end, c.end)
		if lo >= hi {
			continue
		}
		dy := c.dy()
		bottom := c.y + c.h
		for _, r := range c.l.SelectionRects(lo-c.off, hi-c.off) {
			if r.Y += dy; r.Y < bottom-1e-9 {
				rs = append(rs, r)
			}
		}
	}
	return rs
}

// drawParagraph draws the paragraph layout l at origin in col: a chunked
// one chunk by chunk, only the chunks that overlap [top, bottom) (in the
// canvas's y), each clipped to its shown lines.
func drawParagraph(c render.Canvas, l render.TextLayout, origin render.Point, col render.Color, top, bottom float64) {
	cl, ok := l.(*chunkedLayout)
	if !ok {
		c.DrawText(l, origin, col)
		return
	}
	for i := cl.chunkAtY(top - origin.Y); i < len(cl.chunks); i++ {
		k := &cl.chunks[i]
		if origin.Y+k.y >= bottom {
			break
		}
		at := render.Point{X: origin.X, Y: origin.Y + k.dy()}
		if k.end == len(cl.text) || k.lines == k.l.LineCount() {
			c.DrawText(k.l, at, col)
			continue
		}
		c.Save()
		// Clipped to the shown lines only vertically: the text may hang
		// past the wrap width.
		w := max(k.l.Size().W, cl.width)
		c.ClipRoundedRect(render.Rect{X: origin.X - w, Y: origin.Y + k.y, W: 3 * w, H: k.h}, render.Radii{})
		c.DrawText(k.l, at, col)
		c.Restore()
	}
}
