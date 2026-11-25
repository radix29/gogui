package layout

import "math"

// span is one child's extent on a stack's (or form's) main axis, as input
// to distribute.
type span struct {
	natural float64 // the size the child wants (measured unbounded)
	min     float64 // never shrink below this (measured at 0)
	max     float64 // never grow beyond this; +Inf when unset
	expand  bool    // shares the leftover space
}

// distribute shares space among items along the main axis and returns one
// size per item; callers compute the positions. The caller has already
// subtracted spacing and padding from space.
//
// Inputs are normalized first: negative and NaN values count as 0, a min of
// +Inf counts as unset, a natural size of +Inf counts as 0, min wins over
// max, and each natural size is clamped into [min, max]. Then:
//
//   - With +Inf space, every item gets its natural size.
//   - With more space than the natural sizes need, the leftover is shared
//     equally among the expanding items. An item's max caps its share, and
//     the rest is shared out again among the others. If nothing expands, or
//     every expander is capped, the leftover stays at the trailing end.
//   - With less space, every item (expanders included) shrinks in proportion
//     to its flex, natural − min.
//   - Below the sum of the minima, every item gets its minimum and the
//     content overflows at the trailing edge.
//
// Whenever the space is used up (sharing or shrinking), the floating-point
// leftover goes to the last item that got a share, so the sizes summed in
// order add up to space exactly. When no value of that item hits space (a
// rounding gap, about 2 % of random inputs), the sum is the float just below
// space; it never exceeds it. The result depends only on the input.
func distribute(items []span, space float64) []float64 {
	return distributeIn(nil, items, space)
}

// distributeIn is distribute with its slices lent by p (P7): the sizes it
// returns are p's scratch, valid until the caller releases its mark.
func distributeIn(p *Pass, items []span, space float64) []float64 {
	buf := p.floatBuf(3 * len(items))
	n := len(items)
	sizes, lo, hi := buf[:n:n], buf[n:2*n:2*n], buf[2*n:]

	var natural, minimum float64
	for i, it := range items {
		lo[i] = finite(it.min)
		hi[i] = math.Max(lo[i], norm(it.max))
		sizes[i] = clamp(finite(it.natural), lo[i], hi[i])
		natural += sizes[i]
		minimum += lo[i]
	}
	space = norm(space)
	switch {
	case math.IsInf(space, 1):
		return sizes
	case space >= natural:
		grow(items, sizes, hi, space, space-natural)
	case space <= minimum:
		copy(sizes, lo)
	default:
		shrink(sizes, lo, space, natural-space, natural-minimum)
	}
	return sizes
}

// grow shares leftover among the expanding items, capped by hi.
func grow(items []span, sizes, hi []float64, space, leftover float64) {
	var buf [16]int // most stacks have fewer expanders
	active := buf[:0]
	for i, it := range items {
		if it.expand && sizes[i] < hi[i] {
			active = append(active, i)
		}
	}
	for len(active) > 0 {
		share := leftover / float64(len(active))
		next := active[:0] // filtered in place
		for _, i := range active {
			if sizes[i]+share >= hi[i] {
				leftover -= hi[i] - sizes[i]
				sizes[i] = hi[i]
			} else {
				next = append(next, i)
			}
		}
		if len(next) == len(active) {
			for _, i := range active {
				sizes[i] += share
			}
			fixSum(sizes, active[len(active)-1], space)
			return
		}
		active = next
	}
	// Every expander is capped: the rest stays at the trailing end.
}

// shrink takes deficit away from the items in proportion to their flex
// (sizes[i] − lo[i]). flex is the total flex and is larger than deficit, so
// at least one item has flex and gets the floating-point leftover.
func shrink(sizes, lo []float64, space, deficit, flex float64) {
	last := -1
	for i := range sizes {
		if f := sizes[i] - lo[i]; f > 0 {
			sizes[i] -= deficit * f / flex
			last = i
		}
	}
	fixSum(sizes, last, space)
}

// fixSum adjusts sizes[k] so that the sizes summed in order equal space.
// Adding the difference to sizes[k] usually suffices. When rounding makes
// that miss, it bisects sizes[k]: the in-order sum is monotone in it, and
// the adjustment is a few ulps at most. The result is the largest value
// whose sum is not above space, so if no value hits space exactly (a gap in
// the rounding), the sum is the float just below space. k must be valid.
func fixSum(sizes []float64, k int, space float64) {
	total := func(v float64) float64 {
		sizes[k] = v
		var s float64
		for _, x := range sizes {
			s += x
		}
		return s
	}
	v, d := sizes[k], 0.0
	for range 2 {
		if d = space - total(v); d == 0 {
			return
		}
		v += d
	}
	// Bracket and bisect, keeping total(lo) <= space < total(hi).
	lo, hi := v, v
	for s := math.Abs(d); total(lo) > space; s *= 2 {
		lo = v - s
	}
	for s := math.Abs(d); total(hi) <= space; s *= 2 {
		hi = v + s
	}
	for {
		mid := lo + (hi-lo)/2
		if mid == lo || mid == hi {
			break
		}
		if total(mid) <= space {
			lo = mid
		} else {
			hi = mid
		}
	}
	total(lo)
}
