package gtk4

import (
	"errors"
	"sort"
	"unicode/utf8"

	"github.com/radix29/gogui/internal/render"
)

// The geometry of a layout for hit-testing, carets and selection (PLAN.md
// S4.2h), read from Pango in NewLayout (pangoNative.measure) and kept as Go
// data, so the queries make no native calls. Positions are byte offsets:
// Pango indexes the UTF-8 text it is given, and pangoText keeps the
// offsets of Text().
//
// The rules are Win32's (win32/textgeom.go) and shell/fake's. A cluster is
// a grapheme, from one Pango cursor position to the next (and every line
// starts one), so a combining mark or a ZWJ emoji sequence is never split;
// every query normalizes an index to the start of its cluster. A line's
// break counts as one cluster: Pango leaves a paragraph delimiter ("\n",
// "\r\n", "\r", U+2029) out of the line's content, and the one forced
// break inside a paragraph it knows (U+2028, LINE SEPARATOR) ends the
// content of a line that is not the last one. Unlike DirectWrite, Pango
// does not break at VT, FF or NEL: they are ordinary clusters.
// Beyond Pango's own boxes:
//
//   - Right of a line's last cluster, HitTest gives the line's end with
//     trailing false. The end is before the line's break; on a line
//     wrapped at whitespace it is the start of the last whitespace
//     cluster, so the caret stays on that line; on a line broken between
//     clusters it is the next line's start.
//   - The caret at a wrap point is drawn at the next line's start.
//   - A selected line break shows as nlWidth past the line's end.
//
// Every cluster's box is Pango's, in its direction (pangoNative unites
// the character extents of the layout's iterator), so mixed directions
// need no second path: the caret is at a cluster's leading edge in its
// own direction, and when any cluster is right to left a selection is one
// rect per line spanning its selected clusters. Pango gives the last
// space of a line wrapped at it no width, so selecting just that space
// shows no rect (DirectWrite's has one).

// pangoLine is one line of a PangoLayout as its iterator gives it, in
// Pango units and byte offsets.
type pangoLine struct {
	start, length int32 // its content: the paragraph delimiter is left out
	top, bottom   int32 // pango_layout_iter_get_line_yrange
	endX          int32 // the x of its content's end, in layout coordinates
}

// pangoCluster is one grapheme of a PangoLayout: it starts at a cursor
// position (or a line's start) and runs to the next one.
type pangoCluster struct {
	start    int32 // byte offset
	x, width int32 // from its leading edge: width < 0 right to left
	white    bool  // its first character is white space (is_white)
}

// geometry is the queryable form of a layout's lines and clusters.
type geometry struct {
	n       int // bytes
	bidi    bool
	nlWidth float64
	lines   []geoLine
	cl      []geoCluster // in text order; line breaks left out
}

// geoLine is one line: bytes [start, content) are its clusters (trailing
// whitespace included), [content, end) its break.
type geoLine struct {
	start, content, end int
	hitEnd              int     // the index HitTest gives right of the line
	endX                float64 // the caret x at the line's end
	first, last         int     // its clusters cl[first:last]
	top, height         float64
}

type geoCluster struct {
	start       int
	left, width float64
	rtl, white  bool // right to left; white space (a wrap at it keeps the caret on its line)
}

var errGeometry = errors.New("gtk4: text: Pango's lines and clusters do not cover the text")

// isForcedBreak reports whether r is a forced line break that Pango keeps
// in a line's content: U+2028 only (pango-layout.c's LINE_SEPARATOR).
func isForcedBreak(r rune) bool { return r == 0x2028 }

// newGeometry builds the geometry of text (the bytes Pango laid out) from
// its lines and clusters. The lines must be in order, the first starting
// at 0, each one's content ending at or before the next one's start, and
// the last one's at the text's end; the clusters in order, inside the
// text, one starting at every non-empty line's start. nlWidth is in
// logical points.
func newGeometry(text string, lines []pangoLine, clusters []pangoCluster, nlWidth float64) (geometry, error) {
	n := len(text)
	if len(lines) == 0 || lines[0].start != 0 {
		return geometry{}, errGeometry
	}
	g := geometry{n: n, nlWidth: nlWidth, lines: make([]geoLine, len(lines)), cl: make([]geoCluster, 0, len(clusters))}
	ci := 0
	for li, pl := range lines {
		start, content, end := int(pl.start), int(pl.start)+int(pl.length), n
		last := li == len(lines)-1
		if !last {
			end = int(lines[li+1].start)
		}
		if pl.length < 0 || content > end || end > n || (last && content != n) {
			return geometry{}, errGeometry
		}
		// endX is Pango's; a forced break moves it to the break's leading
		// edge (or the last cluster's trailing one, or 0, without a box).
		endX, forced := points(pl.endX), false
		if !last && content == end && content > start {
			if r, size := utf8.DecodeLastRuneInString(text[start:content]); isForcedBreak(r) {
				content -= size
				forced = true
			}
		}
		ln := geoLine{start: start, content: content, end: end, first: len(g.cl),
			top: points(pl.top), height: points(pl.bottom - pl.top)}
		if ci < len(clusters) && int(clusters[ci].start) < start {
			return geometry{}, errGeometry // out of order, or inside the last line's break
		}
		if content > start && (ci == len(clusters) || int(clusters[ci].start) != start) {
			return geometry{}, errGeometry
		}
		breakBox := false
		for ; ci < len(clusters) && int(clusters[ci].start) < end; ci++ {
			pc := clusters[ci]
			if ci > 0 && pc.start <= clusters[ci-1].start {
				return geometry{}, errGeometry
			}
			gc := geoCluster{start: int(pc.start), left: points(pc.x), width: points(pc.width), white: pc.white}
			if pc.width < 0 {
				gc.left, gc.width, gc.rtl = points(pc.x+pc.width), -gc.width, true
				g.bidi = true
			}
			switch {
			case gc.start < content:
				g.cl = append(g.cl, gc)
			case gc.start == content && forced:
				endX, breakBox = gc.leading(), true
			}
		}
		ln.last = len(g.cl)
		switch {
		case !forced || breakBox:
		case ln.last > ln.first:
			endX = g.cl[ln.last-1].trailing()
		default:
			endX = 0
		}
		ln.endX = endX
		switch {
		case last || content < end:
			ln.hitEnd = content // the text's end, or before the break
		case ln.last > ln.first && g.cl[ln.last-1].white:
			ln.hitEnd = g.cl[ln.last-1].start // wrapped at whitespace
		default:
			ln.hitEnd = end // broken between clusters: the next line's start
		}
		g.lines[li] = ln
	}
	if ci != len(clusters) {
		return geometry{}, errGeometry
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
// size logical points: half an em, about one average glyph (as Win32;
// shell/fake uses one rune).
func newlineWidth(size float64) float64 { return size / 2 }
