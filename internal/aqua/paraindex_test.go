package aqua

import (
	"math"
	"math/rand"
	"testing"
)

// paraModel is the slice model of a paraIndex: the heights in units and
// the exact flags, in order.
type paraModel struct {
	h  []int64
	ex []bool
}

func (m *paraModel) set(i int, h float64, exact bool) {
	m.h[i], m.ex[i] = paraUnits(h), exact
}

func (m *paraModel) splice(i, removed, inserted int, height func(int) float64) {
	h := make([]int64, inserted)
	ex := make([]bool, inserted)
	for k := range h {
		if height != nil {
			h[k] = paraUnits(height(k))
		}
	}
	m.h = append(m.h[:i:i], append(h, m.h[i+removed:]...)...)
	m.ex = append(m.ex[:i:i], append(ex, m.ex[i+removed:]...)...)
}

// checkPara compares x with the model through every query, and checks the
// chunk bounds and the trees.
func checkPara(t *testing.T, x *paraIndex, m *paraModel) {
	t.Helper()
	n := len(m.h)
	if x.Len() != n {
		t.Fatalf("Len = %d, want %d", x.Len(), n)
	}
	var y int64
	inexact := 0
	for i := 0; i < n; i++ {
		if got := x.Y(i); got != paraPoints(y) {
			t.Fatalf("Y(%d) = %v, want %v", i, got, paraPoints(y))
		}
		if got := x.Height(i); got != paraPoints(m.h[i]) {
			t.Fatalf("Height(%d) = %v, want %v", i, got, paraPoints(m.h[i]))
		}
		if got := x.Exact(i); got != m.ex[i] {
			t.Fatalf("Exact(%d) = %v, want %v", i, got, m.ex[i])
		}
		if h := m.h[i]; h > 0 {
			for _, u := range []int64{y, y + h/2, y + h - 1} {
				if got := x.At(paraPoints(u)); got != i {
					t.Fatalf("At(%v) = %d, want %d", paraPoints(u), got, i)
				}
			}
		}
		y += m.h[i]
		if !m.ex[i] {
			inexact++
		}
	}
	if got := x.Y(n); got != paraPoints(y) {
		t.Fatalf("Y(n) = %v, want %v", got, paraPoints(y))
	}
	if x.Total() != paraPoints(y) || x.Inexact() != inexact {
		t.Fatalf("Total, Inexact = %v, %d, want %v, %d", x.Total(), x.Inexact(), paraPoints(y), inexact)
	}
	last := max(n-1, 0)
	for _, q := range []float64{-5, math.NaN(), math.Inf(-1)} {
		if got := x.At(q); got != 0 {
			t.Fatalf("At(%v) = %d, want 0", q, got)
		}
	}
	for _, q := range []float64{paraPoints(y), paraPoints(y) + 3, math.Inf(1)} {
		if got := x.At(q); got != last && !(y == 0 && got == 0) {
			t.Fatalf("At(%v) = %d, want %d", q, got, last)
		}
	}
	next := -1
	for from := n; from >= 0; from-- {
		if from < n && !m.ex[from] {
			next = from
		}
		if got := x.NextInexact(from); got != next {
			t.Fatalf("NextInexact(%d) = %d, want %d", from, got, next)
		}
	}
	if got := x.NextInexact(-3); n > 0 && got != x.NextInexact(0) {
		t.Fatalf("NextInexact(-3) = %d", got)
	}
	checkParaChunks(t, x)
}

func checkParaChunks(t *testing.T, x *paraIndex) {
	t.Helper()
	if x.n == 0 {
		if len(x.chunks) != 0 || x.total != 0 || x.inexact != 0 {
			t.Fatalf("empty index keeps state: %d chunks", len(x.chunks))
		}
		return
	}
	saved := *x
	for c, ch := range x.chunks {
		l := len(ch.h)
		if len(ch.exact) != l || l > paraChunkMax || l == 0 || (l < paraChunkMin && len(x.chunks) > 1) {
			t.Fatalf("chunk %d of %d has %d paragraphs (%d flags)", c, len(x.chunks), l, len(ch.exact))
		}
		var sum int64
		inexact := 0
		for j, h := range ch.h {
			sum += h
			if !ch.exact[j] {
				inexact++
			}
		}
		if sum != ch.sum || inexact != ch.inexact {
			t.Fatalf("chunk %d: sum, inexact = %d, %d, want %d, %d", c, ch.sum, ch.inexact, sum, inexact)
		}
	}
	var fresh paraIndex
	fresh.chunks = x.chunks
	fresh.rebuild()
	if fresh.n != saved.n || fresh.total != saved.total || fresh.inexact != saved.inexact {
		t.Fatalf("totals %d %d %d, rebuilt %d %d %d", saved.n, saved.total, saved.inexact, fresh.n, fresh.total, fresh.inexact)
	}
	for k := range fresh.fn {
		if fresh.fn[k] != saved.fn[k] || fresh.fh[k] != saved.fh[k] || fresh.fi[k] != saved.fi[k] {
			t.Fatalf("tree entry %d differs from a rebuild", k)
		}
	}
}

// paraHeight returns a height source for spliced paragraphs: mostly line
// multiples and fractions, some zero, negative or NaN (stored as 0).
func paraHeight(seed int) func(int) float64 {
	return func(k int) float64 {
		switch v := (seed*31 + k*7) % 23; {
		case v == 0:
			return 0
		case v == 1:
			return -4
		case v == 2:
			return math.NaN()
		case v < 12:
			return 17 * float64(v)
		default:
			return float64(v)*3.3 + 1.0/3
		}
	}
}

func TestParaIndexBasics(t *testing.T) {
	var x paraIndex
	m := &paraModel{}
	checkPara(t, &x, m)

	x.Splice(0, 0, 3, func(k int) float64 { return []float64{10, 20.5, 30}[k] })
	m.splice(0, 0, 3, func(k int) float64 { return []float64{10, 20.5, 30}[k] })
	checkPara(t, &x, m)
	if x.Y(2) != 30.5 || x.Total() != 60.5 || x.At(30.4) != 1 || x.At(30.5) != 2 {
		t.Fatalf("Y(2) %v, Total %v, At %d %d", x.Y(2), x.Total(), x.At(30.4), x.At(30.5))
	}
	if x.NextInexact(0) != 0 || x.Inexact() != 3 {
		t.Fatalf("NextInexact %d, Inexact %d", x.NextInexact(0), x.Inexact())
	}

	x.Set(0, 12, true)
	m.set(0, 12, true)
	x.Set(1, 20.5, true)
	m.set(1, 20.5, true)
	checkPara(t, &x, m)
	if x.NextInexact(0) != 2 {
		t.Fatalf("NextInexact = %d, want 2", x.NextInexact(0))
	}
	x.Set(2, 1, true)
	m.set(2, 1, true)
	checkPara(t, &x, m)
	if x.NextInexact(0) != -1 || x.Inexact() != 0 {
		t.Fatalf("all exact: NextInexact %d, Inexact %d", x.NextInexact(0), x.Inexact())
	}

	// Zero heights are never hit; a nil height source makes zeros.
	x.Splice(1, 0, 2, nil)
	m.splice(1, 0, 2, nil)
	checkPara(t, &x, m)
	if x.At(12) != 3 {
		t.Fatalf("At(12) = %d, want 3 (past the zero-height paragraphs)", x.At(12))
	}

	// Rounding: 1/3 is stored to the unit and the sums stay exact.
	x.Set(0, 1.0/3, false)
	m.set(0, 1.0/3, false)
	checkPara(t, &x, m)
	if d := x.Height(0) - 1.0/3; math.Abs(d) > 1.0/paraUnit || x.Y(1) != x.Height(0) {
		t.Fatalf("Height(0) = %v, Y(1) = %v", x.Height(0), x.Y(1))
	}

	// Removing everything empties the index.
	x.Splice(0, x.Len(), 0, nil)
	m.splice(0, len(m.h), 0, nil)
	checkPara(t, &x, m)
}

func TestParaIndexPanics(t *testing.T) {
	var x paraIndex
	x.Splice(0, 0, 5, nil)
	for name, f := range map[string]func(){
		"Y(-1)":           func() { x.Y(-1) },
		"Y(n+1)":          func() { x.Y(6) },
		"Height(n)":       func() { x.Height(5) },
		"Exact(-1)":       func() { x.Exact(-1) },
		"Set(n)":          func() { x.Set(5, 1, true) },
		"Splice past end": func() { x.Splice(6, 0, 1, nil) },
		"Splice removed":  func() { x.Splice(3, 3, 0, nil) },
		"Splice negative": func() { x.Splice(0, -1, 0, nil) },
		"Splice inserted": func() { x.Splice(0, 0, -1, nil) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s did not panic", name)
				}
			}()
			f()
		}()
	}
}

// TestParaIndexLarge walks the chunk paths: a large load, single-paragraph
// edits that grow and shrink chunks past their bounds, splices across
// chunk boundaries and whole chunks, and a clear.
func TestParaIndexLarge(t *testing.T) {
	var x paraIndex
	m := &paraModel{}
	do := func(i, removed, inserted int) {
		t.Helper()
		h := paraHeight(i + removed + inserted)
		x.Splice(i, removed, inserted, h)
		m.splice(i, removed, inserted, h)
		checkPara(t, &x, m)
	}
	do(0, 0, 1000)
	if len(x.chunks) < 1000/paraChunkTarget {
		t.Fatalf("%d chunks for 1000 paragraphs", len(x.chunks))
	}
	for k := 0; k < 300; k++ { // one chunk grows past paraChunkMax
		do(500, 0, 1)
	}
	for k := 0; k < 400; k++ { // and shrinks below paraChunkMin
		do(400, 1, 0)
	}
	do(0, 0, 1)                    // at the front
	do(x.Len(), 0, 1)              // at the end
	do(x.Len()-1, 1, 0)            // the last one
	do(10, 200, 3)                 // across chunks
	do(0, 100, 0)                  // whole chunks off the front
	do(x.Len()-100, 100, 0)        // and off the back
	do(x.Len()/2, 0, 5000)         // a large paste
	do(x.Len()/3, x.Len()/3, 1)    // a large delete
	for i := 0; i < x.Len(); i++ { // the idle pass making all exact
		if i%3 == 0 {
			x.Set(i, float64(i%50), true)
			m.set(i, float64(i%50), true)
		}
	}
	checkPara(t, &x, m)
	do(0, x.Len(), 0)
}

func TestParaIndexRandom(t *testing.T) {
	for seed := int64(1); seed <= 20; seed++ {
		rng := rand.New(rand.NewSource(seed))
		var x paraIndex
		m := &paraModel{}
		for op := 0; op < 400; op++ {
			n := len(m.h)
			switch r := rng.Intn(10); {
			case r < 4 && n > 0:
				i := rng.Intn(n)
				h := paraHeight(rng.Intn(1000))(rng.Intn(50))
				exact := rng.Intn(3) > 0
				x.Set(i, h, exact)
				m.set(i, h, exact)
			default:
				i := rng.Intn(n + 1)
				removed, inserted := 0, 0
				switch rng.Intn(4) {
				case 0:
					removed, inserted = rng.Intn(3), rng.Intn(3)
				case 1:
					removed, inserted = rng.Intn(150), rng.Intn(150)
				case 2:
					inserted = rng.Intn(400)
				default:
					removed = rng.Intn(400)
				}
				removed = min(removed, n-i)
				h := paraHeight(rng.Intn(1000))
				x.Splice(i, removed, inserted, h)
				m.splice(i, removed, inserted, h)
			}
			checkPara(t, &x, m)
		}
	}
}

func FuzzParaIndex(f *testing.F) {
	f.Add([]byte{0, 0, 200, 1, 3, 9, 2, 100, 50, 3, 7, 0})
	f.Add([]byte{0, 0, 255, 0, 10, 255, 2, 5, 200, 2, 0, 255})
	f.Add([]byte{0, 0, 1, 2, 0, 1, 1, 0, 0})
	f.Fuzz(func(t *testing.T, ops []byte) {
		var x paraIndex
		m := &paraModel{}
		for len(ops) >= 3 {
			kind, a, b := ops[0], int(ops[1]), int(ops[2])
			ops = ops[3:]
			n := len(m.h)
			switch kind % 4 {
			case 0: // insert b paragraphs (times 4 for odd kinds above 3)
				i := a % (n + 1)
				ins := b
				if kind >= 4 {
					ins *= 4
				}
				h := paraHeight(a + b)
				x.Splice(i, 0, ins, h)
				m.splice(i, 0, ins, h)
			case 1: // remove up to b paragraphs
				if n == 0 {
					continue
				}
				i := a % n
				rem := min(b, n-i)
				x.Splice(i, rem, 0, nil)
				m.splice(i, rem, 0, nil)
			case 2: // replace: remove b%16, insert b/16
				i := a % (n + 1)
				rem := min(b%16, n-i)
				h := paraHeight(b)
				x.Splice(i, rem, b/16, h)
				m.splice(i, rem, b/16, h)
			default: // set
				if n == 0 {
					continue
				}
				i := (a*256 + b) % n
				h := paraHeight(b)(a)
				x.Set(i, h, kind&4 != 0)
				m.set(i, h, kind&4 != 0)
			}
			checkPara(t, &x, m)
		}
	})
}

// newParaBench is an index of 20 000 paragraphs, a third of them exact.
func newParaBench() *paraIndex {
	x := &paraIndex{}
	x.Splice(0, 0, 20000, paraHeight(1))
	for i := 0; i < x.Len(); i += 3 {
		x.Set(i, 17*float64(1+i%4), true)
	}
	return x
}

func BenchmarkParaIndexY(b *testing.B) {
	x := newParaBench()
	idx := rand.New(rand.NewSource(1)).Perm(x.Len())
	b.ResetTimer()
	var s float64
	for k := 0; k < b.N; k++ {
		s += x.Y(idx[k%len(idx)])
	}
	_ = s
}

func BenchmarkParaIndexAt(b *testing.B) {
	x := newParaBench()
	rng := rand.New(rand.NewSource(1))
	ys := make([]float64, 4096)
	for k := range ys {
		ys[k] = rng.Float64() * x.Total()
	}
	b.ResetTimer()
	s := 0
	for k := 0; k < b.N; k++ {
		s += x.At(ys[k%len(ys)])
	}
	_ = s
}

func BenchmarkParaIndexSet(b *testing.B) {
	x := newParaBench()
	idx := rand.New(rand.NewSource(1)).Perm(x.Len())
	b.ResetTimer()
	for k := 0; k < b.N; k++ {
		x.Set(idx[k%len(idx)], float64(17+k%40), k%2 == 0)
	}
}

func BenchmarkParaIndexNextInexact(b *testing.B) {
	x := newParaBench()
	idx := rand.New(rand.NewSource(1)).Perm(x.Len())
	b.ResetTimer()
	s := 0
	for k := 0; k < b.N; k++ {
		s += x.NextInexact(idx[k%len(idx)])
	}
	_ = s
}

// BenchmarkParaIndexSplice is the edits of one paragraph: Return (one
// paragraph becomes two), Backspace at a paragraph start (two become one)
// and a keystroke (one replaced by one), at random places, so chunks fill,
// split, shrink and merge on the way.
func BenchmarkParaIndexSplice(b *testing.B) {
	x := newParaBench()
	rng := rand.New(rand.NewSource(1))
	idx := make([]int, 4096)
	for k := range idx {
		idx[k] = rng.Intn(x.Len() - 1)
	}
	h := paraHeight(3)
	b.ResetTimer()
	for k := 0; k < b.N; k++ {
		i := idx[k%len(idx)]
		switch k % 3 {
		case 0:
			x.Splice(i, 1, 2, h)
		case 1:
			x.Splice(i, 2, 1, h)
		default:
			x.Splice(i, 1, 1, h)
		}
	}
}

// BenchmarkParaIndexSpliceAcross is the slow path: one paragraph replaced
// across a chunk boundary, which re-chunks and rebuilds the trees.
func BenchmarkParaIndexSpliceAcross(b *testing.B) {
	x := newParaBench()
	h := paraHeight(3)
	b.ResetTimer()
	for k := 0; k < b.N; k++ {
		i := 0 // the start of chunk c
		for c := 1 + k%(len(x.chunks)-1); c > 0; c -= c & -c {
			i += x.fn[c]
		}
		x.Splice(i-1, 2, 2, h)
	}
}
