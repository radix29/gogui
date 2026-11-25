//go:build windows

package win32

import (
	"errors"
	"sort"

	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/win32/dwrite"
)

// The geometry of a layout for hit-testing, carets and selection (PLAN.md
// S2.5e), read from DirectWrite's line and cluster metrics in NewLayout and
// kept as Go data, so the queries make no native calls. Positions are UTF-16
// unit indices; layout converts them from and to UTF-8 offsets.
//
// Every query normalizes an index to the start of its cluster (a combining
// mark or a surrogate pair is never split), and a line's break ("\n",
// "\r\n", U+2028 …) counts as one cluster. Rules beyond DirectWrite's own
// answers, matching shellkit.FixedText (shell/fake's text):
//
//   - Right of a line's last cluster, HitTest gives the line's end with
//     trailing false (DirectWrite says "last cluster, trailing"). The end is
//     before the line's break; on a line wrapped at whitespace it is the
//     start of the last whitespace cluster, so the caret stays on that line;
//     on a line broken between clusters it is the next line's start.
//   - The caret at a wrap point is drawn at the next line's start.
//   - A selected line break shows as nlWidth past the line's end.
//
// Text is laid out left-aligned, so for left-to-right text a cluster's x is
// the sum of the widths before it on its line. When the layout has mixed
// directions, each cluster's box comes from DirectWrite
// (HitTestTextPosition); the caret is at a cluster's leading edge in its own
// direction, and a selection is one rect per line spanning its selected
// clusters.
type geometry struct {
	n       int // UTF-16 units
	bidi    bool
	nlWidth float64
	lines   []geoLine
	cl      []geoCluster // in text order; line breaks left out
}

// geoLine is one line: units [start, content) are its clusters (trailing
// whitespace included), [content, end) its break.
type geoLine struct {
	start, content, end int
	hitEnd              int     // the index HitTest gives right of the line
	endX                float64 // the caret x at the line's end
	first, last         int     // its clusters cl[first:last]
	top, height         float64
	baseline            float64 // from the layout's top
}

type geoCluster struct {
	start       int
	left, width float64
	rtl         bool
}

// newGeometry builds the geometry of a layout of n units. box, when not
// nil, gives the box of the cluster at pos (mixed-direction text).
func newGeometry(n int, lines []dwrite.LineMetrics, clusters []dwrite.ClusterMetrics,
	box func(pos uint32) (left, width float32, err error), nlWidth float64) (geometry, error) {
	if len(lines) == 0 {
		return geometry{}, errors.New("win32: text: layout without lines")
	}
	g := geometry{n: n, bidi: box != nil, nlWidth: nlWidth, lines: make([]geoLine, len(lines))}
	g.cl = make([]geoCluster, 0, len(clusters))
	pos, ci, top := 0, 0, 0.0
	for li, lm := range lines {
		end := pos + int(lm.Length)
		content := end - int(min(lm.NewlineLength, lm.Length))
		ln := geoLine{start: pos, content: content, end: end, first: len(g.cl), top: top, height: float64(lm.Height),
			baseline: top + float64(lm.Baseline)}
		x := 0.0
		for ; ci < len(clusters) && pos < end; ci++ {
			c := clusters[ci]
			if pos < content {
				gc := geoCluster{start: pos, left: x, width: float64(c.Width), rtl: c.IsRightToLeft()}
				if box != nil {
					l, w, err := box(uint32(pos))
					if err != nil {
						return geometry{}, err
					}
					gc.left, gc.width = float64(l), float64(w)
				}
				g.cl = append(g.cl, gc)
				x += float64(c.Width)
			}
			pos += int(c.Length)
		}
		pos = end
		ln.last = len(g.cl)
		if ln.last > ln.first {
			ln.endX = g.cl[ln.last-1].trailing()
		}
		if box != nil && (content < end || li == len(lines)-1) { // DirectWrite knows the visual end
			l, _, err := box(uint32(content))
			if err != nil {
				return geometry{}, err
			}
			ln.endX = float64(l)
		}
		switch {
		case li == len(lines)-1 || content < end:
			ln.hitEnd = content // the text's end, or before the break
		case lm.TrailingWhitespaceLength > 0 && ln.last > ln.first:
			ln.hitEnd = g.cl[ln.last-1].start // wrapped at whitespace
		default:
			ln.hitEnd = end // broken between clusters: the next line's start
		}
		g.lines[li] = ln
		top += ln.height
	}
	if pos != n || ci != len(clusters) {
		return geometry{}, errors.New("win32: text: line and cluster metrics do not cover the text")
	}
	return g, nil
}

// lineAt returns the line that holds the caret at u: the last one starting
// at or before it.
func (g *geometry) lineAt(u int) int {
	return max(sort.Search(len(g.lines), func(i int) bool { return g.lines[i].start > u })-1, 0)
}

// cluster returns the index in cl of the last cluster of line ln starting
// at or before u (u inside the line's clusters).
func (g *geometry) cluster(ln *geoLine, u int) int {
	return ln.first + max(sort.Search(ln.last-ln.first, func(i int) bool { return g.cl[ln.first+i].start > u })-1, 0)
}

// clusterStart clamps u to the text and moves it to its cluster's start.
func (g *geometry) clusterStart(u int) int {
	u = min(max(u, 0), g.n)
	ln := &g.lines[g.lineAt(u)]
	if u >= ln.content || ln.first == ln.last {
		return min(u, ln.content) // inside the break, or at the text's end
	}
	return g.cl[g.cluster(ln, u)].start
}

// leading and trailing are a cluster's edges in its own direction.
func (c geoCluster) leading() float64 {
	if c.rtl {
		return c.left + c.width
	}
	return c.left
}

func (c geoCluster) trailing() float64 {
	if c.rtl {
		return c.left
	}
	return c.left + c.width
}

// x returns the caret's x at the cluster start u on line ln.
func (g *geometry) x(ln *geoLine, u int) float64 {
	if u >= ln.content || ln.first == ln.last {
		return ln.endX
	}
	return g.cl[g.cluster(ln, u)].leading()
}

// caret returns the caret rect at the cluster start u.
func (g *geometry) caret(u int) render.Rect {
	ln := &g.lines[g.lineAt(u)]
	return render.Rect{X: g.x(ln, u), Y: ln.top, H: ln.height}
}

// line returns line i, clamped to the lines.
func (g *geometry) line(i int) *geoLine { return &g.lines[max(0, min(i, len(g.lines)-1))] }

// caretUpstream returns the caret rect at the cluster start u with the
// upstream affinity: at a soft break (u starts a line and the line before
// ends without a break) at that earlier line's end, else caret(u).
func (g *geometry) caretUpstream(u int) render.Rect {
	li := g.lineAt(u)
	if li == 0 || g.lines[li].start != u || g.lines[li-1].content != g.lines[li-1].end {
		return g.caret(u)
	}
	prev := &g.lines[li-1]
	return render.Rect{X: prev.endX, Y: prev.top, H: prev.height}
}

// hitTest returns the cluster start nearest to (x, y) and whether the point
// is in its trailing half.
func (g *geometry) hitTest(x, y float64) (int, bool) {
	li := 0
	if y > 0 { // NaN and negative hit the first line
		li = len(g.lines) - 1
		for li > 0 && g.lines[li].top > y {
			li--
		}
	}
	ln := &g.lines[li]
	if !(x > 0) {
		return ln.start, false
	}
	for k := ln.first; k < ln.last && g.cl[k].start < ln.hitEnd; k++ {
		c := g.cl[k]
		if x >= c.left && x < c.left+c.width {
			mid := c.left + c.width/2
			return c.start, (x >= mid) != c.rtl
		}
	}
	return ln.hitEnd, false
}

// selection returns the highlight rects of the cluster starts [a, b).
func (g *geometry) selection(a, b int) []render.Rect {
	if a >= b {
		return nil
	}
	var rs []render.Rect
	for li := range g.lines {
		ln := &g.lines[li]
		next := ln.end
		if li == len(g.lines)-1 {
			next = g.n + 1
		}
		if a >= next || b <= ln.start {
			continue
		}
		lo, hi := max(a, ln.start), min(b, ln.content)
		left, right := ln.endX, ln.endX
		if lo < hi {
			if g.bidi {
				left, right = g.span(ln, lo, hi)
			} else {
				left, right = g.x(ln, lo), g.x(ln, hi)
			}
		}
		if ln.content < ln.end && b > ln.content { // the selected break
			end := ln.endX
			left, right = min(left, end), max(right, end+g.nlWidth)
		}
		if right > left {
			rs = append(rs, render.Rect{X: left, Y: ln.top, W: right - left, H: ln.height})
		}
	}
	return rs
}

// span returns the horizontal extent of the clusters of ln in [lo, hi).
func (g *geometry) span(ln *geoLine, lo, hi int) (left, right float64) {
	first := true
	for k := g.cluster(ln, lo); k < ln.last && g.cl[k].start < hi; k++ {
		c := g.cl[k]
		if first || c.left < left {
			left = c.left
		}
		if first || c.left+c.width > right {
			right = c.left + c.width
		}
		first = false
	}
	return left, right
}

// newlineWidth is the width a selected line break shows with, for a font of
// size DIPs: half an em, about one average glyph (shellkit.FixedText uses one rune).
func newlineWidth(size float32) float64 { return float64(size) / 2 }
