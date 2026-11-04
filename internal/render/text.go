package render

import "github.com/radix29/gogui/internal/driver"

// Font is driver.Font: the zero Font is the regular system font at
// driver.DefaultFontSize (a text system has no control size, so a zero size
// means 13 pt).
type Font = driver.Font

// TextSystem makes text layouts. The shell provides one
// (Shell.TextSystem(), S2.1c): DirectWrite on Windows, Pango on Linux, and
// fixed-width fake metrics in shell/fake. It is used on the UI thread.
type TextSystem interface {
	// NewLayout lays out text (UTF-8) in font. Lines break at '\n' and, when
	// maxWidth is positive and finite, wrap to fit maxWidth (at word
	// boundaries where possible). A maxWidth of zero, a negative one, NaN or
	// +Inf means no wrapping.
	NewLayout(text string, font Font, maxWidth float64) TextLayout
}

// TextLayout is laid-out, immutable text. Geometry is in logical points
// relative to the layout's top-left corner (where Canvas.DrawText puts its
// origin). Text indices are UTF-8 byte offsets into Text(), as for TextView
// (PLAN.md §4.3); an index inside a rune or a grapheme cluster is moved back
// to the cluster's start, and one outside [0, len(Text())] is clamped.
type TextLayout interface {
	// Text returns the laid-out text.
	Text() string
	// Font returns the font passed to NewLayout.
	Font() Font

	// Size returns the size of the laid-out text: the widest line by the sum
	// of the line heights. Empty text has the height of one line and zero
	// width.
	Size() Size
	// Baseline returns the distance from the top to the first line's
	// baseline.
	Baseline() float64
	// LineCount returns the number of lines after breaking and wrapping
	// (at least 1).
	LineCount() int

	// HitTest returns the cluster nearest to p: index is its start, and
	// trailing reports whether p lies in its trailing half, so the caret
	// belongs after it. Points above the first line hit the first line,
	// points below the last line the last one, and points left or right of a
	// line its first or last cluster. Right of a line's last cluster, index
	// is the line's end (before its '\n') with trailing false; in empty text
	// it is 0.
	HitTest(p Point) (index int, trailing bool)
	// CaretRect returns the caret at index as a zero-width rect spanning the
	// line height; backends give it their own width.
	CaretRect(index int) Rect
	// SelectionRects returns the highlight of the text between start and end
	// (in either order), one rect per line touched, top to bottom. It is
	// empty when start and end are equal.
	SelectionRects(start, end int) []Rect
}
