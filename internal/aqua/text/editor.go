package text

import "strings"

// Unit is how far a move or a delete goes.
type Unit uint8

const (
	// Grapheme is one grapheme cluster (← →, ⌫ ⌦). A '\n' is one cluster.
	Grapheme Unit = iota
	// Word is Option-← and Option-→ (and Option-⌫): to the start of the word
	// before or the end of the word after, skipping spaces and punctuation
	// and crossing paragraphs.
	Word
	// Line is Primary-← and Primary-→ (and Primary-⌫): to the start or end
	// of the visual line, through Lines.
	Line
	// Paragraph is Option-↑ and Option-↓: to the start or end of the
	// paragraph, or of the one before or after when already there.
	Paragraph
	// Document is Primary-↑ and Primary-↓: to the start or end of the text.
	Document
	// Vertical is ↑ and ↓: one visual line up or down at the remembered goal
	// x, through Lines; on the first or last line to the document start or
	// end.
	Vertical
	// Page is Page Up and Page Down: Lines.PageLines visual lines at the goal
	// x.
	Page
)

// Direction is the direction of a move or a delete.
type Direction uint8

const (
	Backward Direction = iota // ←, ↑, ⌫: towards the start
	Forward                   // →, ↓, ⌦: towards the end
)

// Lines is the visual line geometry of the laid-out text. The widget
// implements it over render.TextLayout (S5.2, S5.4); the Editor asks it only
// for the Line, Vertical and Page units. Offsets are into the whole text.
//
// A soft line break (inside a paragraph) is an offset that ends one visual
// line and starts the next; upstream picks the earlier line there (the caret
// drawn at the end of the line, after Primary-→). Elsewhere it is ignored.
type Lines interface {
	// LineRange returns the visual line holding the caret at off: its first
	// offset and the offset where the next line of the paragraph starts, or
	// the paragraph end (its '\n' or Len) on its last line.
	LineRange(off int, upstream bool) (start, end int)
	// CaretX returns the x of the caret at off, in any unit OffsetAt reads
	// back.
	CaretX(off int, upstream bool) float64
	// OffsetAt returns the offset nearest to x on the visual line n lines
	// below (n < 0: above) the line holding off, and whether it is the
	// upstream end of a soft-wrapped line. Fewer than |n| lines that way
	// clamp to the first or last line; ok is false only when the line
	// holding off already is the first (n < 0) or the last (n > 0).
	OffsetAt(off int, upstream bool, n int, x float64) (target int, targetUpstream bool, ok bool)
	// PageLines is the number of lines a Page move goes.
	PageLines() int
}

// defaultSpan is the reach of a span around an offset (see Editor.around):
// paragraphs up to twice as long are read whole.
const defaultSpan = 2048

// Editor is the selection, the moves and the edits over a Buffer, with the
// macOS semantics of PLAN.md §5.5 and §5.6. The selection is an anchor (where
// it started) and a head (where the caret is); both are always grapheme
// boundaries of the whole text. Every boundary comes from the paragraph
// around the offset (a stretch of it, for long ones), never from the whole
// buffer.
//
// In single-line mode (TextField) the text never holds a '\n': SetText,
// Insert and Paste turn newlines into spaces, so the text is one paragraph
// and the vertical moves go to its start or end, as in an NSTextField.
//
// Every edit goes through replace, which records it for undo (undo.go).
type Editor struct {
	buf          *Buffer
	single       bool
	anchor, head int
	upstream     bool // the head is the upstream end of a soft-wrapped line
	goalX        float64
	hasGoal      bool // goalX holds; a vertical move keeps it, all else drops it
	spanMin      int  // the reach of around; 0 means defaultSpan (tests shrink it)
	hist         history
}

// NewEditor returns an editor over text with the caret at 0.
func NewEditor(text string, singleLine bool) *Editor {
	e := &Editor{buf: &Buffer{}, single: singleLine, hist: history{limit: DefaultUndoLimit}}
	e.SetText(text)
	return e
}

// Buffer returns the text for reading (layout, painting). Editing it
// directly breaks the Editor's invariants; use the Editor's methods.
func (e *Editor) Buffer() *Buffer { return e.buf }

// SingleLine reports single-line mode.
func (e *Editor) SingleLine() bool { return e.single }

// Text returns the whole text.
func (e *Editor) Text() string { return e.buf.String() }

// Len is the length of the text in bytes.
func (e *Editor) Len() int { return e.buf.Len() }

// SetText replaces the text (newlines normalized, in single-line mode turned
// into spaces), puts the caret at 0 and clears the undo history.
func (e *Editor) SetText(s string) {
	e.buf.SetText(e.clean(s))
	e.setCaret(0, false)
	e.hist.clear()
}

// Selection returns the selected range [start, end); start == end is a
// caret.
func (e *Editor) Selection() (start, end int) {
	return min(e.anchor, e.head), max(e.anchor, e.head)
}

// Anchor is the fixed end of the selection, Head the end that moves (the
// caret).
func (e *Editor) Anchor() int { return e.anchor }

// Head is the moving end of the selection, where the caret is drawn.
func (e *Editor) Head() int { return e.head }

// Upstream reports that the head is the end of a soft-wrapped visual line
// rather than the start of the next (after Primary-→ or a vertical move to a
// line end); the widget draws the caret and asks Lines with it.
func (e *Editor) Upstream() bool { return e.upstream }

// SetSelection sets the anchor and the head, each clamped into the text and
// to the start of its grapheme cluster.
func (e *Editor) SetSelection(anchor, head int) {
	e.hist.seal()
	e.anchor = e.clusterStart(e.clamp(anchor))
	e.head = e.clusterStart(e.clamp(head))
	e.upstream, e.hasGoal = false, false
}

// SetSelectionUpstream is SetSelection with the head upstream when
// upstream is set (see Upstream): a click or a drag at the end of a
// soft-wrapped visual line, where the caller's layout knows the line
// ends. The flag is kept only when the head stays where it was asked for
// (not moved by the clamp or to a cluster start).
func (e *Editor) SetSelectionUpstream(anchor, head int, upstream bool) {
	e.SetSelection(anchor, head)
	e.upstream = upstream && e.head == head
}

// SelectAll selects the whole text.
func (e *Editor) SelectAll() {
	e.hist.seal()
	e.setRange(0, e.buf.Len())
}

// SelectWord selects what a double-click at off selects: the word there, a
// run of whitespace, or one other cluster (WordAt); nothing in an empty
// paragraph.
func (e *Editor) SelectWord(off int) {
	e.hist.seal()
	off = e.clamp(off)
	ps, pe := e.buf.ParagraphAt(off)
	if ps == pe {
		e.setCaret(off, false)
		return
	}
	var start, end int
	e.around(off, ps, pe, func(s string, lo int) bool {
		a, b := WordAt(s, off-lo)
		start, end = lo+a, lo+b
		// A word or a single cluster ends at word boundaries, but a
		// whitespace run may go on past a span end.
		return !isSpaceCluster(s[a:]) || (a > 0 || lo == ps) && (b < len(s) || lo+len(s) == pe)
	})
	e.setRange(start, end)
}

// SelectParagraph selects what a triple-click at off selects: the paragraph
// with its '\n'.
func (e *Editor) SelectParagraph(off int) {
	e.hist.seal()
	ps, pe := e.buf.ParagraphAt(e.clamp(off))
	if pe < e.buf.Len() {
		pe++
	}
	e.setRange(ps, pe)
}

// Move moves the head by u in direction d; extend keeps the anchor (Shift),
// else the selection collapses to the new caret. Without extend a selection
// first collapses to its start (Backward) or end (Forward): a Grapheme move
// stops there, the other units go on from there. lines is used for Line,
// Vertical and Page; nil treats each paragraph as one visual line.
func (e *Editor) Move(u Unit, d Direction, extend bool, lines Lines) {
	e.hist.seal()
	from, up := e.head, e.upstream
	if start, end := e.Selection(); !extend && start != end {
		if d == Backward {
			from = start
		} else {
			from = end
		}
		if from != e.head {
			up = false
		}
		if u == Grapheme {
			e.setCaret(from, false)
			return
		}
	}
	vertical := u == Vertical || u == Page
	g := goal{e.goalX, e.hasGoal}
	to, toUp := e.target(from, up, u, d, lines, &g)
	if extend {
		e.head, e.upstream, e.hasGoal = to, toUp, false
	} else {
		e.setCaret(to, toUp)
	}
	if vertical {
		e.goalX, e.hasGoal = g.x, g.ok
	}
}

// Insert is typing: it replaces the selection with text (newlines
// normalized, in single-line mode turned into spaces) and puts the caret
// after it. Consecutive inserts at the caret are one undo group; an insert
// holding a newline is a group of its own.
func (e *Editor) Insert(text string) {
	start, end := e.Selection()
	text = e.clean(text)
	k := kindTyping
	if strings.IndexByte(text, '\n') >= 0 {
		k = kindOther
	}
	e.replace(start, end, text, k)
}

// DeleteBackward is ⌫ (Grapheme), Option-⌫ (Word) and Primary-⌫ (Line):
// it deletes the selection, or else from the target of a backward move by u
// to the caret. A Line delete at the start of a line deletes the cluster
// before instead (joining the paragraphs at a paragraph start). Consecutive
// backward deletes at the caret are one undo group.
func (e *Editor) DeleteBackward(u Unit, lines Lines) { e.delete(u, Backward, lines) }

// DeleteForward is ⌦ and its Option and Primary variants, mirroring
// DeleteBackward (consecutive forward deletes are one undo group).
func (e *Editor) DeleteForward(u Unit, lines Lines) { e.delete(u, Forward, lines) }

// Copy returns the selected text.
func (e *Editor) Copy() string {
	start, end := e.Selection()
	return e.buf.Slice(start, end)
}

// Cut deletes the selection and returns it.
func (e *Editor) Cut() string {
	start, end := e.Selection()
	s := e.buf.Slice(start, end)
	if start != end {
		e.replace(start, end, "", kindOther)
	}
	return s
}

// Paste replaces the selection with s, as Insert does, but always as an
// undo group of its own.
func (e *Editor) Paste(s string) {
	start, end := e.Selection()
	e.replace(start, end, e.clean(s), kindOther)
}

func (e *Editor) delete(u Unit, d Direction, lines Lines) {
	start, end := e.Selection()
	k := kindDeleteBackward
	if d == Forward {
		k = kindDeleteForward
	}
	if start == end {
		to, _ := e.target(e.head, e.upstream, u, d, lines, &goal{e.goalX, e.hasGoal})
		if to == e.head && u == Line {
			to, _ = e.target(e.head, e.upstream, Grapheme, d, nil, nil)
		}
		start, end = min(to, e.head), max(to, e.head)
		if start == end {
			return
		}
	}
	e.replace(start, end, "", k)
}

// replace is the one edit: [start, end) becomes text (already cleaned),
// recorded for undo as an edit of kind k (a no-op is not recorded). The
// caret goes after the inserted text, or to start; an edit can join clusters
// around it (a combining mark next to a new base), so the caret moves to the
// end of the cluster it fell into after an insert, its start after a delete.
func (e *Editor) replace(start, end int, text string, k editKind) {
	if start == end && text == "" {
		return
	}
	e.hist.begin(k, e.anchor, e.head)
	old := e.buf.Slice(start, end)
	n := e.buf.Replace(start, end, text) // one change, one report to the observer
	c := start + n
	if cs := e.clusterStart(c); cs != c {
		c = cs
		if n > 0 {
			c = e.nextGrapheme(cs)
		}
	}
	e.setCaret(c, false)
	e.hist.record(edit{start: start, old: old, new: text[:n]}, c)
}

// target returns where a move by u in direction d from (from, up) lands, and
// whether it is upstream. g is the goal x of a Vertical or Page move, read
// from Lines when it does not hold yet (nil for the other units).
func (e *Editor) target(from int, up bool, u Unit, d Direction, lines Lines, g *goal) (int, bool) {
	back := d == Backward
	switch u {
	case Grapheme:
		if back {
			return e.prevGrapheme(from), false
		}
		return e.nextGrapheme(from), false
	case Word:
		if back {
			return e.prevWordStart(from), false
		}
		return e.nextWordEnd(from), false
	case Line:
		start, end := e.lines(lines).LineRange(from, up)
		if back {
			return e.clusterStart(e.clamp(start)), false
		}
		end = e.clusterStart(e.clamp(end))
		_, pe := e.buf.ParagraphAt(from)
		return end, end < pe // a soft break: stay on this line
	case Paragraph:
		i := e.buf.ParagraphIndex(from)
		if back {
			if ps := e.buf.ParagraphStart(i); from > ps || i == 0 {
				return ps, false
			}
			return e.buf.ParagraphStart(i - 1), false
		}
		if pe := e.buf.ParagraphEnd(i); from < pe || i == e.buf.ParagraphCount()-1 {
			return pe, false
		}
		return e.buf.ParagraphEnd(i + 1), false
	case Document:
		if back {
			return 0, false
		}
		return e.buf.Len(), false
	case Vertical, Page:
		edge := e.buf.Len()
		if back {
			edge = 0
		}
		if e.single {
			return edge, false
		}
		l := e.lines(lines)
		n := 1
		if u == Page {
			n = max(l.PageLines(), 1)
		}
		if back {
			n = -n
		}
		if pl, isPara := l.(paraLines); isPara && !pl.hasLine(from, n) {
			return edge, false // no counting of clusters for nothing
		}
		if !g.ok {
			g.x, g.ok = l.CaretX(from, up), true
		}
		to, toUp, ok := l.OffsetAt(from, up, n, g.x)
		if !ok {
			return edge, false
		}
		return e.clusterStart(e.clamp(to)), toUp
	}
	panic("text: unknown unit")
}

// lines returns l, or the fallback that treats each paragraph as one line.
func (e *Editor) lines(l Lines) Lines {
	if l == nil {
		return paraLines{e}
	}
	return l
}

func (e *Editor) setCaret(off int, upstream bool) {
	e.anchor, e.head, e.upstream, e.hasGoal = off, off, upstream, false
}

func (e *Editor) setRange(start, end int) {
	e.anchor, e.head, e.upstream, e.hasGoal = start, end, false, false
}

func (e *Editor) clamp(off int) int { return min(max(off, 0), e.buf.Len()) }

// clean normalizes the newlines of s and, in single-line mode, turns them
// into spaces.
func (e *Editor) clean(s string) string {
	s = normalizeNewlines(s)
	if e.single {
		s = strings.ReplaceAll(s, "\n", " ")
	}
	return s
}

// around calls f with a stretch s of the paragraph [ps, pe) holding off,
// starting at lo, with lo ≤ off ≤ lo+len(s). Short paragraphs are passed
// whole. In a long one the stretch reaches about spanMin bytes either way and
// its ends are paragraph ends or provable word and grapheme boundaries (see
// wordSafeAt) strictly before and after off, so the boundary functions give
// the same answers inside it as over the whole paragraph. f returns false
// when its answer depends on text past an end that is not the paragraph's;
// around then retries with a stretch four times wider, the whole paragraph
// at worst.
func (e *Editor) around(off, ps, pe int, f func(s string, lo int) bool) {
	w := e.spanMin
	if w <= 0 {
		w = defaultSpan
	}
	for ; ; w *= 4 {
		if pe-ps <= 2*w {
			f(e.buf.Slice(ps, pe), ps)
			return
		}
		a, b := max(ps, off-w), min(pe, off+w)
		raw := e.buf.Slice(a, b)
		lo, hi := ps, pe
		if a > ps {
			p := off - a - 1
			for p > 0 && !wordSafeAt(raw, p) {
				p--
			}
			if p <= 0 {
				continue
			}
			lo = a + p
		}
		if b < pe {
			q := off - a + 1
			for q < len(raw) && !wordSafeAt(raw, q) {
				q++
			}
			if q >= len(raw) {
				continue
			}
			hi = a + q
		}
		if f(raw[lo-a:hi-a], lo) {
			return
		}
	}
}

// nextGrapheme returns the first grapheme boundary after off (Len at the
// end); the '\n' ending a paragraph is one cluster.
func (e *Editor) nextGrapheme(off int) int {
	ps, pe := e.buf.ParagraphAt(off)
	if off == pe {
		return min(off+1, e.buf.Len())
	}
	r := off
	e.around(off, ps, pe, func(s string, lo int) bool {
		r = lo + NextGrapheme(s, off-lo)
		return true
	})
	return r
}

// prevGrapheme returns the last grapheme boundary before off (0 at the
// start).
func (e *Editor) prevGrapheme(off int) int {
	if off == 0 {
		return 0
	}
	ps, pe := e.buf.ParagraphAt(off)
	if off == ps {
		return off - 1
	}
	r := off
	e.around(off, ps, pe, func(s string, lo int) bool {
		r = lo + PrevGrapheme(s, off-lo)
		return true
	})
	return r
}

// clusterStart clamps off to the start of its grapheme cluster.
func (e *Editor) clusterStart(off int) int {
	ps, pe := e.buf.ParagraphAt(off)
	if off == ps || off == pe {
		return off
	}
	r := off
	e.around(off, ps, pe, func(s string, lo int) bool {
		r = lo + ClusterStart(s, off-lo)
		return true
	})
	return r
}

// nextWordEnd is Option-→ over the whole text: the end of the word off is in
// or of the next one, in a later paragraph if need be, else Len.
func (e *Editor) nextWordEnd(off int) int {
	for {
		ps, pe := e.buf.ParagraphAt(off)
		r, found := pe, false
		if off < pe {
			e.around(off, ps, pe, func(s string, lo int) bool {
				x, ok := NextWordEnd(s, off-lo)
				if ok {
					r, found = lo+x, true
				}
				return ok || lo+len(s) == pe
			})
		}
		if found || pe == e.buf.Len() {
			return r
		}
		off = pe + 1
	}
}

// prevWordStart is Option-← over the whole text: the start of the word
// before off, in an earlier paragraph if need be, else 0.
func (e *Editor) prevWordStart(off int) int {
	for {
		ps, pe := e.buf.ParagraphAt(off)
		r, found := ps, false
		if off > ps {
			e.around(off, ps, pe, func(s string, lo int) bool {
				x, ok := PrevWordStart(s, off-lo)
				if ok {
					r, found = lo+x, true
				}
				return ok || lo == ps
			})
		}
		if found || ps == 0 {
			return r
		}
		off = ps - 1
	}
}

// paraLines is the Lines of unwrapped text: each paragraph is one visual
// line, x counts its grapheme clusters, and a page is one line.
type paraLines struct{ e *Editor }

func (l paraLines) LineRange(off int, _ bool) (int, int) { return l.e.buf.ParagraphAt(off) }

func (l paraLines) CaretX(off int, _ bool) float64 {
	ps, pe := l.e.buf.ParagraphAt(off)
	s, n := l.e.buf.Slice(ps, pe), 0
	for p := 0; p < off-ps; p = NextGrapheme(s, p) {
		n++
	}
	return float64(n)
}

func (l paraLines) OffsetAt(off int, _ bool, n int, x float64) (int, bool, bool) {
	b := l.e.buf
	i := b.ParagraphIndex(off)
	j := min(max(i+n, 0), b.ParagraphCount()-1)
	if j == i {
		return off, false, false
	}
	ps := b.ParagraphStart(j)
	s, p := b.Slice(ps, b.ParagraphEnd(j)), 0
	for k := 0; float64(k)+0.5 <= x && p < len(s); k++ {
		p = NextGrapheme(s, p)
	}
	return ps + p, false, true
}

func (paraLines) PageLines() int { return 1 }

// hasLine reports whether there is a paragraph n paragraphs from off's.
func (l paraLines) hasLine(off, n int) bool {
	i := l.e.buf.ParagraphIndex(off)
	return n < 0 && i > 0 || n > 0 && i < l.e.buf.ParagraphCount()-1
}

// goal is the goal x of the vertical moves; ok is false until it is read.
type goal struct {
	x  float64
	ok bool
}
