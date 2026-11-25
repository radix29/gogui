package text

import (
	"slices"
	"strings"
)

// Buffer is a piece table: the text is a sequence of pieces, each a range of
// either the original buffer (set by SetText, never changed afterwards) or
// the add buffer (only ever appended to). Insert and Delete split and trim
// pieces and never copy the text around them.
//
// The text never contains '\r': SetText and Insert turn "\r\n" and a lone
// '\r' into '\n'. Paragraphs are the runs of text between '\n's: a buffer
// with n newlines has n+1 paragraphs, the last one possibly empty, and a
// paragraph's range excludes its '\n'.
//
// Both buffers keep a sorted index of their newlines, and every piece its
// newline count, so the paragraph lookups are binary searches. The byte and
// newline offsets of the pieces are prefix sums that an edit invalidates from
// the edited piece on; the next lookup recomputes only that tail.
//
// An observer (SetObserver) hears of every change as a replaced range, so a
// cache over the paragraphs can drop only what an edit touched.
//
// Offsets out of range panic, as slice indexes do. The zero value is an empty
// buffer ready to use.
type Buffer struct {
	orig   string
	origNL []int // offsets of the '\n's in orig, ascending
	add    []byte
	addNL  []int // offsets of the '\n's in add, ascending

	pieces []piece
	valid  int // pieces[:valid] have correct off and nlOff

	length int
	lines  int // newlines in the text
	rev    uint64

	obs func(start, oldEnd, newEnd int)
}

// piece is a range [start, start+n) of the original (add false) or the add
// buffer, with nl newlines. off and nlOff are the byte offset of the piece in
// the text and the newlines before it; they are correct only below
// Buffer.valid.
type piece struct {
	add   bool
	start int
	n     int
	nl    int
	off   int
	nlOff int
}

// NewBuffer returns a buffer holding s (newlines normalized).
func NewBuffer(s string) *Buffer {
	b := &Buffer{}
	b.SetText(s)
	return b
}

// SetObserver sets the function called after every change to the text, or
// removes it (nil): the text that was [start, oldEnd) is now [start, newEnd).
// SetText reports (0, old Len, new Len), always; Insert (off, off, off+n),
// Delete (start, end, start) and Replace (start, end, start+n), once each and
// only when they change something. Reads never call it, and every change
// through an Editor (edits, undo, redo, SetText) is one report. It runs once
// the buffer is consistent, so it may read the buffer; it must not change it.
func (b *Buffer) SetObserver(f func(start, oldEnd, newEnd int)) { b.obs = f }

// notify reports a change to the observer.
func (b *Buffer) notify(start, oldEnd, newEnd int) {
	if b.obs != nil {
		b.obs(start, oldEnd, newEnd)
	}
}

// SetText replaces the whole text with s (newlines normalized) and drops the
// add buffer. It always advances the revision.
func (b *Buffer) SetText(s string) {
	oldLen := b.length
	s = normalizeNewlines(s)
	b.orig = s
	b.origNL = appendNewlines(b.origNL[:0], s, 0)
	b.add = b.add[:0]
	b.addNL = b.addNL[:0]
	b.pieces = b.pieces[:0]
	if len(s) > 0 {
		b.pieces = append(b.pieces, piece{start: 0, n: len(s), nl: len(b.origNL)})
	}
	b.valid = len(b.pieces)
	b.length = len(s)
	b.lines = len(b.origNL)
	b.rev++
	b.notify(0, oldLen, b.length)
}

// Len is the length of the text in bytes.
func (b *Buffer) Len() int { return b.length }

// Revision counts the changes to the text: SetText, and every Insert, Delete
// or Replace that changed something. A layout cache keyed on it is stale when
// it differs.
func (b *Buffer) Revision() uint64 { return b.rev }

// String returns the whole text.
func (b *Buffer) String() string { return b.Slice(0, b.length) }

// Slice returns the text in [start, end).
func (b *Buffer) Slice(start, end int) string {
	b.checkRange(start, end)
	if start == end {
		return ""
	}
	b.index()
	var sb strings.Builder
	sb.Grow(end - start)
	for i := b.pieceAt(start); i < len(b.pieces) && b.pieces[i].off < end; i++ {
		p := &b.pieces[i]
		lo := max(start-p.off, 0)
		hi := min(end-p.off, p.n)
		if p.add {
			sb.Write(b.add[p.start+lo : p.start+hi])
		} else {
			sb.WriteString(b.orig[p.start+lo : p.start+hi])
		}
	}
	return sb.String()
}

// Insert inserts text at off (newlines normalized) and returns the number of
// bytes inserted, which differs from len(text) when text has "\r\n". Typing
// at the end of the previous insert extends its piece instead of adding one.
func (b *Buffer) Insert(off int, text string) int {
	b.checkOffset(off)
	n := b.insert(off, normalizeNewlines(text))
	if n > 0 {
		b.rev++
		b.notify(off, off, off+n)
	}
	return n
}

// Delete removes the text in [start, end).
func (b *Buffer) Delete(start, end int) {
	b.checkRange(start, end)
	if start == end {
		return
	}
	b.delete(start, end)
	b.rev++
	b.notify(start, end, start)
}

// Replace replaces the text in [start, end) with text (newlines normalized)
// as one change: Delete then Insert, with one revision and one report to
// the observer (start, end, start+n). It returns n, the bytes inserted; a
// replace of nothing with nothing changes nothing.
func (b *Buffer) Replace(start, end int, text string) int {
	b.checkRange(start, end)
	text = normalizeNewlines(text)
	if start == end && text == "" {
		return 0
	}
	if start < end {
		b.delete(start, end)
	}
	n := b.insert(start, text)
	b.rev++
	b.notify(start, end, start+n)
	return n
}

// insert inserts the normalized text at off, without a revision or a
// report, and returns its length.
func (b *Buffer) insert(off int, text string) int {
	if text == "" {
		return 0
	}
	addStart := len(b.add)
	b.add = append(b.add, text...)
	before := len(b.addNL)
	b.addNL = appendNewlines(b.addNL, text, addStart)
	np := piece{add: true, start: addStart, n: len(text), nl: len(b.addNL) - before}

	b.index()
	i := len(b.pieces)
	if off < b.length {
		i = b.pieceAt(off)
	}
	switch {
	case i < len(b.pieces) && off > b.pieces[i].off:
		// Inside piece i: split it around the new piece.
		p := b.pieces[i]
		k := off - p.off
		left := b.sub(p, 0, k)
		right := b.sub(p, k, p.n)
		b.pieces = slices.Insert(b.pieces, i+1, np, right)
		b.pieces[i] = left // sub leaves off and nlOff zero
		b.invalidate(i)
	case i > 0 && b.pieces[i-1].add && b.pieces[i-1].start+b.pieces[i-1].n == addStart:
		// Right after the previous insert: extend its piece.
		b.pieces[i-1].n += np.n
		b.pieces[i-1].nl += np.nl
		b.invalidate(i)
	default:
		b.pieces = slices.Insert(b.pieces, i, np)
		b.invalidate(i)
	}
	b.length += np.n
	b.lines += np.nl
	return np.n
}

// delete removes [start, end), start < end, without a revision or a report.
// When the removed text ends the add buffer (a backspace over what was just
// typed), that tail of the add buffer goes too: no piece refers to it any
// more, and the next insert there extends the piece before it again instead
// of adding one (review B13).
func (b *Buffer) delete(start, end int) {
	b.index()
	i := b.pieceAt(start)
	j := b.pieceAt(end - 1)
	pj := b.pieces[j]
	trim := -1 // the add buffer's new length, or -1 to keep it
	if pj.add && pj.start+pj.n == len(b.add) && end == pj.off+pj.n {
		trim = pj.start + max(start-pj.off, 0)
	}
	var keep [2]piece
	kept := keep[:0]
	if pi := b.pieces[i]; start > pi.off {
		kept = append(kept, b.sub(pi, 0, start-pi.off))
	}
	if end < pj.off+pj.n {
		kept = append(kept, b.sub(pj, end-pj.off, pj.n))
	}
	removedNL := 0
	for k := i; k <= j; k++ {
		removedNL += b.pieces[k].nl
	}
	for _, p := range kept {
		removedNL -= p.nl
	}
	b.pieces = slices.Replace(b.pieces, i, j+1, kept...)
	b.invalidate(i)
	b.length -= end - start
	b.lines -= removedNL
	if trim >= 0 {
		b.add = b.add[:trim]
		k, _ := slices.BinarySearch(b.addNL, trim)
		b.addNL = b.addNL[:k]
	}
}

// ParagraphCount is the number of paragraphs: the newlines plus one (an
// empty buffer has one empty paragraph).
func (b *Buffer) ParagraphCount() int { return b.lines + 1 }

// ParagraphIndex returns the index of the paragraph holding off: the number
// of newlines before off. A '\n' at off belongs to the paragraph it ends; Len
// is in the last paragraph.
func (b *Buffer) ParagraphIndex(off int) int {
	b.checkOffset(off)
	if off == b.length {
		return b.lines
	}
	b.index()
	p := &b.pieces[b.pieceAt(off)]
	k := off - p.off
	return p.nlOff + b.countNL(p.add, p.start, p.start+k)
}

// ParagraphStart returns the offset where paragraph i starts.
func (b *Buffer) ParagraphStart(i int) int {
	if i < 0 || i > b.lines {
		panic("text: paragraph index out of range")
	}
	if i == 0 {
		return 0
	}
	return b.newlineOffset(i-1) + 1
}

// ParagraphEnd returns the offset where paragraph i ends: its '\n', or Len
// for the last paragraph.
func (b *Buffer) ParagraphEnd(i int) int {
	if i < 0 || i > b.lines {
		panic("text: paragraph index out of range")
	}
	if i == b.lines {
		return b.length
	}
	return b.newlineOffset(i)
}

// ParagraphAt returns the range [start, end) of the paragraph holding off
// (see ParagraphIndex), without its '\n'.
func (b *Buffer) ParagraphAt(off int) (start, end int) {
	i := b.ParagraphIndex(off)
	return b.ParagraphStart(i), b.ParagraphEnd(i)
}

// newlineOffset returns the text offset of the k-th '\n' (0-based).
func (b *Buffer) newlineOffset(k int) int {
	b.index()
	// The first piece whose newlines reach past k.
	i, _ := slices.BinarySearchFunc(b.pieces, k, func(p piece, k int) int {
		if p.nlOff+p.nl <= k {
			return -1
		}
		return 1
	})
	p := &b.pieces[i]
	idx := b.origNL
	if p.add {
		idx = b.addNL
	}
	lo, _ := slices.BinarySearch(idx, p.start)
	return p.off + idx[lo+k-p.nlOff] - p.start
}

// sub returns the part [lo, hi) of p (relative to p), its newlines counted.
func (b *Buffer) sub(p piece, lo, hi int) piece {
	s := p.start + lo
	return piece{add: p.add, start: s, n: hi - lo, nl: b.countNL(p.add, s, p.start+hi)}
}

// countNL counts the '\n's in [lo, hi) of the original or the add buffer.
func (b *Buffer) countNL(add bool, lo, hi int) int {
	idx := b.origNL
	if add {
		idx = b.addNL
	}
	i, _ := slices.BinarySearch(idx, lo)
	j, _ := slices.BinarySearch(idx, hi)
	return j - i
}

// pieceAt returns the index of the piece holding off, 0 <= off < Len. The
// prefix sums must be valid.
func (b *Buffer) pieceAt(off int) int {
	i, found := slices.BinarySearchFunc(b.pieces, off, func(p piece, off int) int {
		switch {
		case p.off+p.n <= off:
			return -1
		case p.off > off:
			return 1
		}
		return 0
	})
	if !found {
		panic("text: internal error: no piece at offset")
	}
	return i
}

// invalidate marks the prefix sums from piece i on as stale.
func (b *Buffer) invalidate(i int) { b.valid = min(b.valid, i) }

// index recomputes the stale prefix sums.
func (b *Buffer) index() {
	off, nl := 0, 0
	if b.valid > 0 {
		p := &b.pieces[b.valid-1]
		off, nl = p.off+p.n, p.nlOff+p.nl
	}
	for i := b.valid; i < len(b.pieces); i++ {
		p := &b.pieces[i]
		p.off, p.nlOff = off, nl
		off += p.n
		nl += p.nl
	}
	b.valid = len(b.pieces)
}

func (b *Buffer) checkOffset(off int) {
	if off < 0 || off > b.length {
		panic("text: offset out of range")
	}
}

func (b *Buffer) checkRange(start, end int) {
	if start < 0 || end > b.length || start > end {
		panic("text: range out of bounds")
	}
}

// normalizeNewlines turns "\r\n" and a lone '\r' into '\n'.
func normalizeNewlines(s string) string {
	if strings.IndexByte(s, '\r') < 0 {
		return s
	}
	var sb strings.Builder
	sb.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\r' {
			if i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
			c = '\n'
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

// appendNewlines appends base plus the offset of every '\n' in s to dst.
func appendNewlines(dst []int, s string, base int) []int {
	for i := 0; ; {
		j := strings.IndexByte(s[i:], '\n')
		if j < 0 {
			return dst
		}
		dst = append(dst, base+i+j)
		i += j + 1
	}
}
