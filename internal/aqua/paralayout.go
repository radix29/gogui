package aqua

import (
	"math"
	"sort"

	"github.com/radix29/gogui/internal/aqua/text"
	"github.com/radix29/gogui/internal/render"
)

// paraLayouts is the TextView's paragraph layout cache (PLAN.md S5.4e): one
// render.TextLayout per paragraph of a text.Buffer (the paragraph without
// its '\n'), in one font and wrapped to one width, made only when a
// paragraph is asked for (painted, hit, or passed by a move) and kept in an
// LRU list of at most bound layouts, so a 1 MB text never holds a layout
// per paragraph. It keeps the paragraphs' heights in a paraIndex: an
// estimate from the byte length until the paragraph is laid out, then the
// exact height of its layout.
//
// It owns the buffer's observer (Buffer.SetObserver): every change reaches
// it as a replaced range, which it turns into a paragraph splice. The
// layouts of the paragraphs the change touched are dropped and their
// heights become estimates; the other layouts stay, moved to their new
// paragraph numbers, with their exact heights. A width or font change drops
// every layout and re-estimates every height.
//
// A paragraph longer than longParaMin is laid out in chunks
// (parachunk.go, review P1): an edit inside it keeps its cached layout and
// lays out only the chunks around the edit, and its composition reuses the
// chunks away from the preedit.
//
// It implements text.Lines over the layouts, across paragraph boundaries:
// a vertical move from a paragraph's last line goes to the first line of
// the next. x is in points from the left of the layouts.
//
// Scroll anchoring is the caller's: a Layout can change the height of the
// paragraph it lays out (estimate to exact), and the TextView compares the
// Y of its first visible paragraph before and after.
//
// The IME's composition (S5.4j) is spliced into its paragraph's layout
// only: SetPreedit makes a second layout of that paragraph with the preedit
// inserted, kept apart from the LRU list and rebuilt on a width or font
// change, and the paragraph's height in the index is that layout's while it
// lasts. Shown returns it for painting and the caret; Layout and the
// text.Lines methods keep working on the text without the preedit (the
// TextView commits the composition before every move and edit). A change of
// the buffer that touches the preedit's paragraph drops the preedit.
type paraLayouts struct {
	ts    render.TextSystem
	buf   *text.Buffer
	font  render.Font
	width float64 // the wrap width; 0: no wrapping

	lineH float64 // the font's line height
	adv   float64 // the font's average advance per byte, for the estimates
	page  int     // PageLines
	bound int     // the most layouts kept

	idx  paraIndex
	m    map[int]*paraLayout // by paragraph
	lru  paraLayout          // sentinel: lru.next is the most recently used
	made int                 // layouts made, for the tests and benchmarks

	pre paraPreedit // the composition spliced into one paragraph, if any
}

// paraPreedit is the IME's composition in a paraLayouts: the preedit s
// inserted at the byte at of paragraph para, and that paragraph's layout
// with it (l nil: no composition).
type paraPreedit struct {
	para, at int
	s        string
	l        render.TextLayout
}

// paraLayout is one cached layout, an entry of the LRU list.
type paraLayout struct {
	para       int
	l          render.TextLayout
	prev, next *paraLayout
}

const (
	paraLayoutsMinBound = 64
	// paraSample is the text the average advance is measured on.
	paraSample = "The quick brown fox jumps over the lazy dog, 0123456789."
)

var _ text.Lines = (*paraLayouts)(nil)

// newParaLayouts makes the cache over buf, laid out in font and wrapped to
// width, and sets itself as buf's observer. Every height starts as an
// estimate; the page is one line and the bound paraLayoutsMinBound until
// SetViewport.
func newParaLayouts(ts render.TextSystem, buf *text.Buffer, font render.Font, width float64) *paraLayouts {
	c := &paraLayouts{ts: ts, buf: buf, font: font, width: wrapWidth(width), page: 1,
		bound: paraLayoutsMinBound, m: map[int]*paraLayout{}}
	c.lru.prev, c.lru.next = &c.lru, &c.lru
	c.measure()
	c.idx.Splice(0, 0, buf.ParagraphCount(), c.estimateAt(0))
	buf.SetObserver(c.changed)
	return c
}

// wrapWidth normalizes a wrap width: zero, negative, NaN and +Inf mean no
// wrapping (0), as for TextSystem.NewLayout.
func wrapWidth(w float64) float64 {
	if !(w > 0) || math.IsInf(w, 1) {
		return 0
	}
	return w
}

// measure reads the font's line height and average advance.
func (c *paraLayouts) measure() {
	c.lineH = c.ts.NewLayout("", c.font, 0).Size().H
	c.adv = c.ts.NewLayout(paraSample, c.font, 0).Size().W / float64(len(paraSample))
}

// Index is the paragraph height index (read only: the cache keeps it).
func (c *paraLayouts) Index() *paraIndex { return &c.idx }

// Font is the layouts' font.
func (c *paraLayouts) Font() render.Font { return c.font }

// Width is the wrap width (0: no wrapping).
func (c *paraLayouts) Width() float64 { return c.width }

// Made is the number of layouts made so far.
func (c *paraLayouts) Made() int { return c.made }

// SetFont lays the text out in f from now on: a different font drops
// every layout and re-estimates every height.
func (c *paraLayouts) SetFont(f render.Font) {
	if f == c.font {
		return
	}
	c.font = f
	c.measure()
	c.reset()
}

// SetWidth wraps the text to w from now on (see wrapWidth): a different
// width drops every layout and re-estimates every height.
func (c *paraLayouts) SetWidth(w float64) {
	if w = wrapWidth(w); w == c.width {
		return
	}
	c.width = w
	c.reset()
}

// SetViewport sets the height of the visible text: a page is as many lines
// as fit in it (at least one), and the bound three pages of lines (the
// visible one and one either way, for paging back and forth), at least
// paraLayoutsMinBound.
func (c *paraLayouts) SetViewport(h float64) {
	lines := 1
	if c.lineH > 0 && h > 0 && !math.IsInf(h, 1) {
		lines = max(1, int(math.Floor(h/c.lineH+1e-9)))
	}
	c.page = lines
	c.SetBound(max(paraLayoutsMinBound, 3*(lines+1)))
}

// SetBound sets the most layouts kept (at least 1), dropping the least
// recently used ones over it.
func (c *paraLayouts) SetBound(n int) {
	c.bound = max(1, n)
	for len(c.m) > c.bound {
		c.drop(c.lru.prev)
	}
}

// reset drops every layout and re-estimates every height.
func (c *paraLayouts) reset() {
	c.dropAll()
	c.idx.Splice(0, c.idx.Len(), c.buf.ParagraphCount(), c.estimateAt(0))
	if c.pre.l != nil {
		c.layoutPreedit()
	}
}

// estimate is the estimated height of a paragraph of n bytes: as many lines
// as its average width takes at the wrap width.
func (c *paraLayouts) estimate(n int) float64 {
	lines := 1
	if c.width > 0 && n > 0 {
		lines = max(1, int(math.Ceil(float64(n)*c.adv/c.width)))
	}
	return float64(lines) * c.lineH
}

// estimateAt is the height function of a splice of new paragraphs from
// paragraph i.
func (c *paraLayouts) estimateAt(i int) func(k int) float64 {
	return func(k int) float64 {
		p := i + k
		return c.estimate(c.buf.ParagraphEnd(p) - c.buf.ParagraphStart(p))
	}
}

// Layout returns paragraph i's layout, made if it is not cached, and makes
// its height exact.
func (c *paraLayouts) Layout(i int) render.TextLayout {
	if e := c.m[i]; e != nil {
		c.unlink(e)
		c.pushFront(e)
		return e.l
	}
	for len(c.m) >= c.bound {
		c.drop(c.lru.prev)
	}
	ps, pe := c.buf.ParagraphStart(i), c.buf.ParagraphEnd(i)
	l := newParaLayout(c.ts, c.buf.Slice(ps, pe), c.font, c.width)
	c.made++
	e := &paraLayout{para: i, l: l}
	c.m[i] = e
	c.pushFront(e)
	if c.composes(i) {
		return l // the height is the composed layout's
	}
	if h := l.Size().H; !c.idx.Exact(i) || c.idx.Height(i) != paraPoints(paraUnits(h)) {
		c.idx.Set(i, h, true)
	}
	return l
}

// composes reports whether paragraph i shows the composition.
func (c *paraLayouts) composes(i int) bool { return c.pre.l != nil && c.pre.para == i }

// SetPreedit splices the IME's preedit s into paragraph i at the byte at
// of the paragraph (see paraLayouts), replacing any composition shown
// before; the paragraph's height becomes the composed layout's. An empty s
// is ClearPreedit.
func (c *paraLayouts) SetPreedit(i, at int, s string) {
	if s == "" {
		c.ClearPreedit()
		return
	}
	if c.composes(i) && c.pre.at == at && c.pre.s == s {
		return
	}
	if c.pre.l != nil && c.pre.para != i {
		c.ClearPreedit()
	}
	c.pre = paraPreedit{para: i, at: at, s: s}
	c.layoutPreedit()
}

// layoutPreedit lays out the composed paragraph and sets its height. A
// long paragraph's composed layout takes over the chunks of its cached
// layout away from the preedit (relayoutPara).
func (c *paraLayouts) layoutPreedit() {
	p := &c.pre
	para := c.buf.Slice(c.buf.ParagraphStart(p.para), c.buf.ParagraphEnd(p.para))
	s := para[:p.at] + p.s + para[p.at:]
	if base := c.cached(p.para); base != nil {
		p.l = relayoutPara(c.ts, base, s, c.font, c.width, p.at, p.at, p.at+len(p.s))
	} else {
		p.l = newParaLayout(c.ts, s, c.font, c.width)
	}
	c.made++
	c.idx.Set(p.para, p.l.Size().H, true)
}

// ClearPreedit ends the composition shown: its paragraph's height is that
// of its layout without the preedit again.
func (c *paraLayouts) ClearPreedit() {
	if c.pre.l == nil {
		return
	}
	i := c.pre.para
	c.pre = paraPreedit{}
	c.idx.Set(i, c.Layout(i).Size().H, true) // a cached layout sets no height
}

// Preedit returns the composition shown: its paragraph, its byte in the
// paragraph and its text (ok false: none).
func (c *paraLayouts) Preedit() (para, at int, s string, ok bool) {
	return c.pre.para, c.pre.at, c.pre.s, c.pre.l != nil
}

// Shown returns paragraph i's layout as shown: the composed one for the
// preedit's paragraph, else Layout(i).
func (c *paraLayouts) Shown(i int) render.TextLayout {
	if c.composes(i) {
		return c.pre.l
	}
	return c.Layout(i)
}

// ShownOffset maps the byte off of paragraph i's text to its shown layout:
// moved past the preedit when it is at or after the preedit's place.
func (c *paraLayouts) ShownOffset(i, off int) int {
	if c.composes(i) && off >= c.pre.at {
		return off + len(c.pre.s)
	}
	return off
}

// TextOffset maps the byte off of paragraph i's shown layout back to its
// text: inside the preedit to its place, after it moved back over it.
func (c *paraLayouts) TextOffset(i, off int) int {
	if !c.composes(i) || off <= c.pre.at {
		return off
	}
	return max(off-len(c.pre.s), c.pre.at)
}

// Measure makes paragraph i's height exact without caching a layout (the
// TextView's idle layout pass, S5.4h), so measuring the whole text never
// pushes the visible layouts out of the LRU list. An exact height stays;
// an empty paragraph is one line high (the height measure read) and needs
// no layout. It reports whether it made a layout.
func (c *paraLayouts) Measure(i int) bool {
	if c.idx.Exact(i) || c.composes(i) {
		return false
	}
	ps, pe := c.buf.ParagraphStart(i), c.buf.ParagraphEnd(i)
	if ps == pe {
		c.idx.Set(i, c.lineH, true)
		return false
	}
	l := newParaLayout(c.ts, c.buf.Slice(ps, pe), c.font, c.width)
	c.made++
	c.idx.Set(i, l.Size().H, true)
	return true
}

// cached returns paragraph i's layout if it is cached, without making it
// or touching the LRU order.
func (c *paraLayouts) cached(i int) render.TextLayout {
	if e := c.m[i]; e != nil {
		return e.l
	}
	return nil
}

// changed is the buffer's observer: the text that was [start, oldEnd) is
// now [start, newEnd). The paragraphs it touched are those from start's
// to oldEnd's in the old text, and they became those from start's to
// newEnd's; the newlines removed follow from the paragraph counts. An edit
// inside one long paragraph whose chunked layout is cached lays out again
// only the chunks around it (relayoutPara) and keeps the layout, its
// height exact (review P1).
func (c *paraLayouts) changed(start, oldEnd, newEnd int) {
	b := c.buf
	i := b.ParagraphIndex(start)
	added := b.ParagraphIndex(newEnd) - i // newlines inserted
	removed := added + c.idx.Len() - b.ParagraphCount()
	if added == 0 && removed == 0 {
		if e := c.m[i]; e != nil {
			if _, long := e.l.(*chunkedLayout); long && !c.composes(i) {
				ps := b.ParagraphStart(i)
				e.l = relayoutPara(c.ts, e.l, b.Slice(ps, b.ParagraphEnd(i)), c.font, c.width, start-ps, oldEnd-ps, newEnd-ps)
				c.made++
				c.idx.Set(i, e.l.Size().H, true)
				return
			}
		}
	}
	c.splice(i, removed+1, added+1)
}

// splice replaces the removed paragraphs from i with inserted new ones:
// their layouts go, their heights are estimated, and the layouts after
// them move to their new paragraph numbers.
func (c *paraLayouts) splice(i, removed, inserted int) {
	c.idx.Splice(i, removed, inserted, c.estimateAt(i))
	delta := inserted - removed
	if c.pre.l != nil {
		switch {
		case c.pre.para >= i+removed:
			c.pre.para += delta
		case c.pre.para >= i:
			c.pre = paraPreedit{} // its paragraph changed: the composition is gone
		}
	}
	if delta == 0 && removed <= len(c.m) {
		for p := i; p < i+removed; p++ {
			if e := c.m[p]; e != nil {
				c.drop(e)
			}
		}
		return
	}
	m := make(map[int]*paraLayout, len(c.m))
	for e := c.lru.next; e != &c.lru; {
		next := e.next
		switch {
		case e.para >= i+removed:
			e.para += delta
			m[e.para] = e
		case e.para >= i:
			c.unlink(e)
		default:
			m[e.para] = e
		}
		e = next
	}
	c.m = m
}

func (c *paraLayouts) dropAll() {
	c.m = map[int]*paraLayout{}
	c.lru.prev, c.lru.next = &c.lru, &c.lru
}

func (c *paraLayouts) drop(e *paraLayout) {
	c.unlink(e)
	delete(c.m, e.para)
}

func (c *paraLayouts) unlink(e *paraLayout) {
	e.prev.next, e.next.prev = e.next, e.prev
	e.prev, e.next = nil, nil
}

func (c *paraLayouts) pushFront(e *paraLayout) {
	e.prev, e.next = &c.lru, c.lru.next
	c.lru.next.prev = e
	c.lru.next = e
}

// at finds the visual line holding the caret at off: its paragraph i, the
// paragraph's start ps, its layout and the line k in it. upstream picks the
// earlier line at a soft break.
func (c *paraLayouts) at(off int, upstream bool) (i, ps int, l render.TextLayout, k int) {
	i = c.buf.ParagraphIndex(off)
	ps = c.buf.ParagraphStart(i)
	l = c.Layout(i)
	return i, ps, l, lineAt(l, off-ps, upstream)
}

// lineAt is the line of l holding the caret at off: the last line starting
// at or before it, or the one before at a soft break when upstream.
func lineAt(l render.TextLayout, off int, upstream bool) int {
	n := l.LineCount()
	k := max(sort.Search(n, func(k int) bool { return l.Line(k).Start > off })-1, 0)
	if upstream && k > 0 && off == l.Line(k).Start && l.Line(k-1).End == off {
		k--
	}
	return k
}

// LineRange implements text.Lines.
func (c *paraLayouts) LineRange(off int, upstream bool) (start, end int) {
	_, ps, l, k := c.at(off, upstream)
	m := l.Line(k)
	return ps + m.Start, ps + m.End
}

// CaretX implements text.Lines.
func (c *paraLayouts) CaretX(off int, upstream bool) float64 {
	ps := c.buf.ParagraphStart(c.buf.ParagraphIndex(off))
	l := c.Layout(c.buf.ParagraphIndex(off))
	if upstream { // the same as CaretRect except at a soft break
		return l.CaretRectUpstream(off - ps).X
	}
	return l.CaretRect(off - ps).X
}

// OffsetAt implements text.Lines: it walks n lines from the caret's,
// through the layouts of the paragraphs on the way, and hits x in the
// middle of the line it lands on.
func (c *paraLayouts) OffsetAt(off int, upstream bool, n int, x float64) (int, bool, bool) {
	i, _, l, k := c.at(off, upstream)
	moved := false
	for ; n > 0; n-- {
		if k+1 < l.LineCount() {
			k++
		} else if i+1 < c.buf.ParagraphCount() {
			i++
			l, k = c.Layout(i), 0
		} else {
			break
		}
		moved = true
	}
	for ; n < 0; n++ {
		if k > 0 {
			k--
		} else if i > 0 {
			i--
			l = c.Layout(i)
			k = l.LineCount() - 1
		} else {
			break
		}
		moved = true
	}
	if !moved {
		return off, false, false
	}
	m := l.Line(k)
	hit, trailing := l.HitTest(render.Point{X: x, Y: m.Top + m.Height/2})
	if trailing {
		hit = text.NextGrapheme(l.Text(), hit)
	}
	hit = min(max(hit, m.Start), m.End)
	return c.buf.ParagraphStart(i) + hit, hit == m.End && k < l.LineCount()-1, true
}

// PageLines implements text.Lines: the lines of the viewport (SetViewport).
func (c *paraLayouts) PageLines() int { return c.page }
