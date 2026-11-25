package aqua

import (
	"math"
	"slices"
)

// paraIndex keeps the height of every paragraph of a text, exact (from its
// layout) or estimated, with prefix sums, so the TextView (PLAN.md S5.4)
// maps a y to its paragraph and a paragraph to its y in logarithmic time
// without laying the whole text out (S5.4d). The zero value is an empty
// index.
//
// Heights are kept in fixed point (paraUnit units per point, rounded), so
// the sums are exact integers: Y(i+1)-Y(i) is exactly Height(i), whatever
// the order of the edits, and the paragraphs tile the document without
// gaps or overlaps. Negative and NaN heights count as 0.
//
// The paragraphs are stored in chunks of paraChunkMin to paraChunkMax
// (only a lone chunk may be smaller), with three Fenwick trees over the
// chunks: their paragraph counts, their heights and their counts of
// estimated paragraphs. A lookup walks a tree, then scans one chunk; a Set
// updates one chunk and the trees; a Splice inside one chunk does the same,
// and one that crosses chunks, or would leave a chunk too large or too
// small, re-chunks the paragraphs around it and rebuilds the trees (linear
// in the chunk count, about n/48).
type paraIndex struct {
	chunks  []*paraChunk
	n       int   // paragraphs
	total   int64 // height in units
	inexact int   // estimated paragraphs

	// Fenwick trees over chunks, 1-based: fn counts, fh heights, fi
	// estimated counts. len = len(chunks)+1.
	fn []int
	fh []int64
	fi []int
}

// paraChunk is a run of consecutive paragraphs.
type paraChunk struct {
	h       []int64 // heights in units
	exact   []bool
	sum     int64
	inexact int
}

const (
	paraUnit        = 1 << 16 // fixed-point units per point
	paraChunkMin    = 32
	paraChunkTarget = 64
	paraChunkMax    = 128
)

// paraUnits converts a height in points to units.
func paraUnits(h float64) int64 {
	if !(h > 0) { // negative, zero and NaN
		return 0
	}
	return int64(math.Round(h * paraUnit))
}

func paraPoints(u int64) float64 { return float64(u) / paraUnit }

// Len is the number of paragraphs.
func (x *paraIndex) Len() int { return x.n }

// Total is the sum of the heights, the document's height.
func (x *paraIndex) Total() float64 { return paraPoints(x.total) }

// Inexact is the number of paragraphs whose height is an estimate.
func (x *paraIndex) Inexact() int { return x.inexact }

// Height is paragraph i's height (as stored: rounded to the unit).
func (x *paraIndex) Height(i int) float64 {
	x.check(i, x.n-1)
	c, off, _ := x.locate(i)
	return paraPoints(x.chunks[c].h[off])
}

// Exact reports whether paragraph i's height is exact, not an estimate.
func (x *paraIndex) Exact(i int) bool {
	x.check(i, x.n-1)
	c, off, _ := x.locate(i)
	return x.chunks[c].exact[off]
}

// Y is the top of paragraph i, the sum of the heights before it; Y(Len())
// is Total().
func (x *paraIndex) Y(i int) float64 {
	x.check(i, x.n)
	if i == x.n {
		return paraPoints(x.total)
	}
	c, off, y := x.locate(i)
	for _, h := range x.chunks[c].h[:off] {
		y += h
	}
	return paraPoints(y)
}

// At is the paragraph at y: the i with Y(i) <= y < Y(i+1), so paragraphs
// of zero height are never hit. A y above the top is paragraph 0, one at
// or below the bottom the last paragraph; an empty index answers 0.
func (x *paraIndex) At(y float64) int {
	if x.n == 0 || !(y >= 0) { // also NaN
		return 0
	}
	u := y * paraUnit
	if u >= float64(x.total) {
		return x.n - 1
	}
	rem := int64(math.Floor(u))
	// Walk fh to the chunk holding rem, summing the counts on the way.
	pos, i := 0, 0
	for step := topBit(len(x.chunks)); step > 0; step >>= 1 {
		if next := pos + step; next < len(x.fh) && x.fh[next] <= rem {
			pos = next
			rem -= x.fh[next]
			i += x.fn[next]
		}
	}
	ch := x.chunks[pos] // rem < ch.sum, as rem < total
	for j, h := range ch.h {
		if rem < h {
			return i + j
		}
		rem -= h
	}
	return i + len(ch.h) - 1 // not reached
}

// NextInexact is the first paragraph at or after from whose height is an
// estimate, or -1 when there is none (the idle layout pass, S5.4h).
func (x *paraIndex) NextInexact(from int) int {
	if from < 0 {
		from = 0
	}
	if from >= x.n || x.inexact == 0 {
		return -1
	}
	c, off, _ := x.locate(from)
	if ch := x.chunks[c]; ch.inexact > 0 {
		for j := off; j < len(ch.h); j++ {
			if !ch.exact[j] {
				return from + j - off
			}
		}
	}
	// The first chunk after c with an estimate: the one where the prefix of
	// the estimated counts passes their prefix up to c.
	target := 0
	for k := c + 1; k > 0; k -= k & -k {
		target += x.fi[k]
	}
	if target == x.inexact {
		return -1
	}
	pos, i := 0, 0
	for step := topBit(len(x.chunks)); step > 0; step >>= 1 {
		if next := pos + step; next < len(x.fi) && x.fi[next] <= target {
			pos = next
			target -= x.fi[next]
			i += x.fn[next]
		}
	}
	for j, e := range x.chunks[pos].exact {
		if !e {
			return i + j
		}
	}
	panic("aqua: paraIndex: inexact count out of step") // not reached
}

// Set sets paragraph i's height and whether it is exact.
func (x *paraIndex) Set(i int, h float64, exact bool) {
	x.check(i, x.n-1)
	c, off, _ := x.locate(i)
	ch := x.chunks[c]
	u := paraUnits(h)
	dh := u - ch.h[off]
	di := 0
	if ch.exact[off] != exact {
		if exact {
			di = -1
		} else {
			di = 1
		}
	}
	ch.h[off], ch.exact[off] = u, exact
	ch.sum += dh
	ch.inexact += di
	x.total += dh
	x.inexact += di
	x.add(c, 0, dh, di)
}

// Splice replaces the removed paragraphs from i with inserted new ones,
// estimated, of height(k) for the k-th new one (k from 0; a nil height
// makes them 0). It is how an edit reaches the index: the paragraphs it
// touched go, the ones it made come in, and the caller sets their exact
// heights as it lays them out.
func (x *paraIndex) Splice(i, removed, inserted int, height func(k int) float64) {
	x.check(i, x.n)
	if removed < 0 || removed > x.n-i || inserted < 0 {
		panic("aqua: paraIndex.Splice: bad count")
	}
	if removed == 0 && inserted == 0 {
		return
	}
	if x.n == 0 {
		x.respan(0, -1, nil, nil, inserted, height, nil, nil)
		return
	}
	c, off := x.locateEnd(i)
	ch := x.chunks[c]
	newLen := len(ch.h) - removed + inserted
	if off+removed <= len(ch.h) && newLen <= paraChunkMax &&
		(newLen >= paraChunkMin || len(x.chunks) == 1) {
		x.spliceChunk(c, ch, off, removed, inserted, height)
		return
	}
	// Across chunks or out of the chunk bounds: re-chunk from the chunk of
	// i to the chunk of the first paragraph kept after the removed ones,
	// with a neighbour when that run would be too short.
	c1, off1 := x.locateEnd(i + removed)
	lo, hi := c, c1
	preH, preEx := ch.h[:off], ch.exact[:off]
	last := x.chunks[c1]
	sufH, sufEx := last.h[off1:], last.exact[off1:]
	if off1 == 0 && c1 > c {
		// The run ends at a chunk boundary: c1 is kept whole after it.
		hi = c1 - 1
		sufH, sufEx = nil, nil
	}
	m := len(preH) + inserted + len(sufH)
	var leftH, rightH []int64
	var leftEx, rightEx []bool
	if m < paraChunkMin {
		if lo > 0 {
			lo--
			leftH, leftEx = x.chunks[lo].h, x.chunks[lo].exact
		} else if hi+1 < len(x.chunks) {
			hi++
			rightH, rightEx = x.chunks[hi].h, x.chunks[hi].exact
		}
	}
	x.respan(lo, hi, cat(leftH, preH), cat(leftEx, preEx), inserted, height,
		cat(sufH, rightH), cat(sufEx, rightEx))
}

// spliceChunk is Splice inside chunk c, which stays within its bounds.
func (x *paraIndex) spliceChunk(c int, ch *paraChunk, off, removed, inserted int, height func(int) float64) {
	var dh int64
	di := 0
	for j := off; j < off+removed; j++ {
		dh -= ch.h[j]
		if !ch.exact[j] {
			di--
		}
	}
	ch.h = spliceN(ch.h, off, removed, inserted)
	ch.exact = spliceN(ch.exact, off, removed, inserted)
	for k := 0; k < inserted; k++ {
		var u int64
		if height != nil {
			u = paraUnits(height(k))
		}
		ch.h[off+k], ch.exact[off+k] = u, false
		dh += u
	}
	di += inserted
	dn := inserted - removed
	ch.sum += dh
	ch.inexact += di
	x.n += dn
	x.total += dh
	x.inexact += di
	if x.n == 0 {
		*x = paraIndex{}
		return
	}
	x.add(c, dn, dh, di)
}

// respan replaces chunks[lo..hi] (none when hi < lo) with the paragraphs
// preH, inserted new ones and sufH, cut into chunks of paraChunkMin to
// paraChunkTarget, and rebuilds the trees.
func (x *paraIndex) respan(lo, hi int, preH []int64, preEx []bool, inserted int, height func(int) float64, sufH []int64, sufEx []bool) {
	m := len(preH) + inserted + len(sufH)
	h := make([]int64, 0, m)
	ex := make([]bool, 0, m)
	h = append(h, preH...)
	ex = append(ex, preEx...)
	for k := 0; k < inserted; k++ {
		var u int64
		if height != nil {
			u = paraUnits(height(k))
		}
		h = append(h, u)
		ex = append(ex, false)
	}
	h = append(h, sufH...)
	ex = append(ex, sufEx...)

	k := (m + paraChunkTarget - 1) / paraChunkTarget
	made := make([]*paraChunk, k)
	for j, start := 0, 0; j < k; j++ {
		end := m * (j + 1) / k
		ch := &paraChunk{
			h:     append(make([]int64, 0, paraChunkMax), h[start:end]...),
			exact: append(make([]bool, 0, paraChunkMax), ex[start:end]...),
		}
		for t, u := range ch.h {
			ch.sum += u
			if !ch.exact[t] {
				ch.inexact++
			}
		}
		made[j] = ch
		start = end
	}
	x.chunks = slices.Replace(x.chunks, lo, hi+1, made...)
	x.rebuild()
}

// rebuild recomputes the totals and the trees from the chunks.
func (x *paraIndex) rebuild() {
	m := len(x.chunks)
	if m == 0 {
		*x = paraIndex{}
		return
	}
	x.fn = resize(x.fn, m+1)
	x.fh = resize(x.fh, m+1)
	x.fi = resize(x.fi, m+1)
	x.n, x.total, x.inexact = 0, 0, 0
	for c, ch := range x.chunks {
		x.fn[c+1] = len(ch.h)
		x.fh[c+1] = ch.sum
		x.fi[c+1] = ch.inexact
		x.n += len(ch.h)
		x.total += ch.sum
		x.inexact += ch.inexact
	}
	for k := 1; k <= m; k++ {
		if p := k + k&-k; p <= m {
			x.fn[p] += x.fn[k]
			x.fh[p] += x.fh[k]
			x.fi[p] += x.fi[k]
		}
	}
}

// add adds to chunk c's entries in the trees.
func (x *paraIndex) add(c, dn int, dh int64, di int) {
	for k := c + 1; k < len(x.fn); k += k & -k {
		x.fn[k] += dn
		x.fh[k] += dh
		x.fi[k] += di
	}
}

// locate finds paragraph i (< n): its chunk, its offset in the chunk and
// the height of the chunks before, in units.
func (x *paraIndex) locate(i int) (c, off int, y int64) {
	pos := 0
	for step := topBit(len(x.chunks)); step > 0; step >>= 1 {
		if next := pos + step; next < len(x.fn) && x.fn[next] <= i {
			pos = next
			i -= x.fn[next]
			y += x.fh[next]
		}
	}
	return pos, i, y
}

// locateEnd is locate for a position in [0, n] (n > 0): n is the end of
// the last chunk.
func (x *paraIndex) locateEnd(i int) (c, off int) {
	if i == x.n {
		c = len(x.chunks) - 1
		return c, len(x.chunks[c].h)
	}
	c, off, _ = x.locate(i)
	return c, off
}

func (x *paraIndex) check(i, max int) {
	if i < 0 || i > max {
		panic("aqua: paraIndex: paragraph out of range")
	}
}

// topBit is the highest power of two <= n (0 for 0).
func topBit(n int) int {
	b := 1
	for b<<1 <= n {
		b <<= 1
	}
	if n == 0 {
		return 0
	}
	return b
}

// spliceN replaces s[off:off+removed] with inserted zero entries, in
// place when the capacity allows.
func spliceN[T any](s []T, off, removed, inserted int) []T {
	n := len(s) - removed + inserted
	if n > cap(s) {
		t := make([]T, n, max(n, paraChunkMax))
		copy(t, s[:off])
		copy(t[off+inserted:], s[off+removed:])
		return t
	}
	old := len(s)
	s = s[:max(n, old)]
	copy(s[off+inserted:], s[off+removed:old])
	return s[:n]
}

// cat is a+b, a copy only when both are non-empty.
func cat[T any](a, b []T) []T {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	return append(append(make([]T, 0, len(a)+len(b)), a...), b...)
}

// resize is s with length n and every entry zero, reusing its array.
func resize[T int | int64](s []T, n int) []T {
	if cap(s) < n {
		return make([]T, n, n+n/4)
	}
	s = s[:n]
	clear(s)
	return s
}
