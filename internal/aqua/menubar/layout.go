package menubar

// Span is a horizontal extent in the title bar's coordinates.
type Span struct {
	X, W float64
}

// End returns X + W.
func (s Span) End() float64 { return s.X + s.W }

// Metrics are the inputs of Layout besides the title widths, all in
// points. The aqua strip takes them from the theme (Chrome and, from S5.6c,
// MenuBar tokens).
type Metrics struct {
	// Width is the title bar's width.
	Width float64
	// Start is the x of the first title's slot: right of the traffic lights
	// and their gap.
	Start float64
	// Pad is the horizontal padding on each side of a title inside its slot
	// (the highlight covers the slot).
	Pad float64
	// MinDrag is the least caption width left right of the last shown
	// title, so the window can still be dragged by its title bar.
	MinDrag float64
	// TitleGap is the least distance between the strip and the window
	// title.
	TitleGap float64
	// TitleMargin is the window title's least distance from either edge of
	// the bar without a strip (aqua's titleMargin: the lights plus their
	// gap).
	TitleMargin float64
}

// Strip is the result of Layout.
type Strip struct {
	// Titles holds the slots of the shown titles, in strip order: the first
	// len(Titles) menus are shown, the rest are dropped.
	Titles []Span
	// End is the right edge of the last shown slot, Start when none is.
	End float64
	// TitleRoom is the width the window title may take, centred in the
	// bar and the same distance from both edges; the title is truncated to
	// it, and hidden when it is not positive.
	TitleRoom float64
}

// Layout places the menu titles of the given text widths left to right
// from m.Start, each in a slot of its width plus m.Pad on both sides. The
// titles that do not fit are dropped from the right, so that m.MinDrag
// stays free between the last shown slot and the bar's right edge (all of
// them when not even the first fits). The window title keeps its symmetric
// margin: m.TitleMargin, or End + m.TitleGap when the strip reaches further.
func Layout(widths []float64, m Metrics) Strip {
	s := Strip{End: m.Start}
	x := m.Start
	for _, w := range widths {
		slot := Span{X: x, W: w + 2*m.Pad}
		if slot.End()+m.MinDrag > m.Width {
			break
		}
		s.Titles = append(s.Titles, slot)
		x = slot.End()
	}
	margin := m.TitleMargin
	if len(s.Titles) > 0 {
		s.End = x
		margin = max(margin, s.End+m.TitleGap)
	}
	s.TitleRoom = m.Width - 2*margin
	return s
}
