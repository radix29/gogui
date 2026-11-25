package layout

import "github.com/radix29/gogui/internal/driver"

// Pass is the state of one layout pass. All child measuring goes through
// Pass.Measure, which caches each result by node and normalized max size,
// so nested stacks and forms don't measure the same subtree over and over.
// It also memoises, per node, the Expands answers and a leaf's unbounded
// fitting size (review P7), and lends the nodes scratch slices.
//
// The cache lives only as long as one pass: a Root keeps one Pass and
// clears it at the start of every LayoutMin call
// (P7), so nothing is reused across passes but the memory is, and the
// nodes themselves hold no per-pass state. So a Root's methods must not
// be called from inside its own pass (an item's FittingSize or SetFrame). A tree must not change while a
// Pass that has measured it is still in use.
//
// The zero value is ready to use and caches. A nil *Pass also works but
// never caches (every call measures again).
type Pass struct {
	// index maps a node, identified by its Props pointer (unique per node,
	// see Node.Props, and cheaper to hash than the Node interface), to its
	// entry in entries. Both keep their memory when the pass is cleared.
	index   map[*Props]int32
	entries []entry

	// The scratch stacks lent by spanBuf, floatBuf and rowBuf, LIFO: a node takes
	// what it needs and gives it back (by length) before it returns.
	spans  []span
	floats []float64
	rows   []formRow
}

// entry is what a pass knows about one node. A node is measured at only a
// few distinct max sizes per pass, so scanning a short inline list beats
// hashing the size too; more go to the overflow slice.
type entry struct {
	n        int
	inline   [4]measured
	more     []measured
	expands  [2]int8 // per Axis: 0 unknown, 1 false, 2 true
	fit      driver.Size
	fitKnown bool
}

// measured is one cached measurement.
type measured struct{ max, size driver.Size }

// reset clears the pass for a new one, keeping its memory.
func (p *Pass) reset() {
	clear(p.index)
	for i := range p.entries {
		p.entries[i].more = p.entries[i].more[:0]
	}
	p.entries = p.entries[:0]
	p.spans, p.floats, p.rows = p.spans[:0], p.floats[:0], p.rows[:0]
}

// entry returns the entry of the node with props, made on first use. The
// pointer is valid until the next call (entries may grow).
func (p *Pass) entry(props *Props) *entry {
	if i, ok := p.index[props]; ok {
		return &p.entries[i]
	}
	if p.index == nil {
		p.index = make(map[*Props]int32)
	}
	p.index[props] = int32(len(p.entries))
	if len(p.entries) < cap(p.entries) {
		p.entries = p.entries[:len(p.entries)+1]
		e := &p.entries[len(p.entries)-1]
		*e = entry{more: e.more[:0]}
		return e
	}
	p.entries = append(p.entries, entry{})
	return &p.entries[len(p.entries)-1]
}

// Measure measures n within max. Negative and NaN components of max count
// as 0, so cached keys never hold NaN and compare with ==. A hidden node is
// not measured and has size zero.
func (p *Pass) Measure(n Node, max driver.Size) driver.Size {
	props := n.Props()
	if props.Hidden() {
		return driver.Size{}
	}
	max = normSize(max)
	if p == nil {
		return n.Measure(p, max)
	}
	if e, ok := p.lookup(props); ok {
		if s, ok := e.find(max); ok {
			return s
		}
	}
	s := n.Measure(p, max) // may cache more sizes, for n too
	p.entry(props).add(measured{max, s})
	return s
}

// lookup returns the entry of props if there is one.
func (p *Pass) lookup(props *Props) (*entry, bool) {
	i, ok := p.index[props]
	if !ok {
		return nil, false
	}
	return &p.entries[i], true
}

func (e *entry) find(max driver.Size) (driver.Size, bool) {
	for _, m := range e.inline[:min(e.n, len(e.inline))] {
		if m.max == max {
			return m.size, true
		}
	}
	for _, m := range e.more {
		if m.max == max {
			return m.size, true
		}
	}
	return driver.Size{}, false
}

func (e *entry) add(m measured) {
	if e.n < len(e.inline) {
		e.inline[e.n] = m
	} else {
		e.more = append(e.more, m)
	}
	e.n++
}

// Expands reports whether n expands on a (Node.Expands), memoised per node
// for the pass. Nodes with children ask theirs through the pass too, so a
// tree is walked once per pass, not once per ancestor.
func (p *Pass) Expands(n Node, a Axis) bool {
	if p == nil {
		return expandsIn(nil, n, a)
	}
	props := n.Props()
	if e, ok := p.lookup(props); ok && e.expands[a] != 0 {
		return e.expands[a] == 2
	}
	x := expandsIn(p, n, a)
	v := int8(1)
	if x {
		v = 2
	}
	p.entry(props).expands[a] = v
	return x
}

// passExpander is a node whose Expands depends on its children: it asks
// them through the pass.
type passExpander interface {
	expandsIn(p *Pass, a Axis) bool
}

func expandsIn(p *Pass, n Node, a Axis) bool {
	if e, ok := n.(passExpander); ok {
		return e.expandsIn(p, a)
	}
	return n.Expands(a)
}

// fitting returns item's unbounded fitting size (finite), asked once per
// pass for the node with props (P7: a leaf is measured at several max
// sizes per pass, and text shaping is the costly part).
func (p *Pass) fitting(props *Props, item Item) driver.Size {
	if p == nil {
		return finiteSize(item.FittingSize(driver.Unbounded()))
	}
	if e, ok := p.lookup(props); ok && e.fitKnown {
		return e.fit
	}
	s := finiteSize(item.FittingSize(driver.Unbounded()))
	e := p.entry(props)
	e.fit, e.fitKnown = s, true
	return s
}

// lend lends n zeroed elements from the scratch stack st; the caller gives
// them back by restoring its length (Pass.mark, Pass.release). Growing
// makes a new array, so slices lent earlier keep the old one.
func lend[T any](st *[]T, n int) []T {
	l := len(*st)
	if l+n > cap(*st) {
		*st = append(make([]T, 0, 2*(l+n)), *st...)
	}
	*st = (*st)[:l+n]
	out := (*st)[l : l+n : l+n]
	clear(out)
	return out
}

// spanBuf lends n spans (a nil pass allocates).
func (p *Pass) spanBuf(n int) []span {
	if p == nil {
		return make([]span, n)
	}
	return lend(&p.spans, n)
}

// floatBuf lends n float64s (distribute's sizes and bounds).
func (p *Pass) floatBuf(n int) []float64 {
	if p == nil {
		return make([]float64, n)
	}
	return lend(&p.floats, n)
}

// rowBuf lends room for n form rows, empty with capacity n.
func (p *Pass) rowBuf(n int) []formRow {
	if p == nil {
		return make([]formRow, 0, n)
	}
	return lend(&p.rows, n)[:0]
}

// scratch is the lengths of the scratch stacks, for release.
type scratch struct{ spans, floats, rows int }

// mark returns the scratch stacks' lengths.
func (p *Pass) mark() scratch {
	if p == nil {
		return scratch{}
	}
	return scratch{len(p.spans), len(p.floats), len(p.rows)}
}

// release gives back the scratch lent since m.
func (p *Pass) release(m scratch) {
	if p != nil {
		p.spans, p.floats, p.rows = p.spans[:m.spans], p.floats[:m.floats], p.rows[:m.rows]
	}
}
