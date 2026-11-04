package layout

import "github.com/radix29/gogui/internal/driver"

// Pass is the state of one layout pass. All child measuring goes through
// Pass.Measure, which caches each result by node and normalized max size,
// so nested stacks and forms don't measure the same subtree over and over.
// The cache lives only as long as the Pass: Root starts a fresh Pass for
// every Layout and MinSize call, so nothing is reused across passes, and
// the nodes themselves hold no per-pass state. A tree must not change while
// a Pass that has measured it is still in use.
//
// The zero value is ready to use and caches. A nil *Pass also works but
// never caches (every call measures again).
type Pass struct {
	// cache maps a node, identified by its Props pointer (unique per node,
	// see Node.Props, and cheaper to hash than the Node interface), to its
	// measurements. A node is measured at only a few distinct max sizes
	// per pass, so scanning its short list beats hashing the size too.
	cache map[*Props][]measured
}

// measured is one cached measurement.
type measured struct{ max, size driver.Size }

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
	for _, m := range p.cache[props] {
		if m.max == max {
			return m.size
		}
	}
	s := n.Measure(p, max) // may cache more sizes, for n too
	if p.cache == nil {
		p.cache = make(map[*Props][]measured)
	}
	p.cache[props] = append(p.cache[props], measured{max, s})
	return s
}
