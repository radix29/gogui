package aqua

import (
	"math"
	"slices"
	"sort"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
)

// menuItem is one item of a menu (PLAN.md S5.5h): a title, or a separator
// (Separator set, the other fields ignored). A disabled item is drawn
// dimmed and is never highlighted; a checked one has a checkmark in the
// column left of its title. Shortcut is the display text of its shortcut
// ("Ctrl+S", menubar.Item.ShortcutText), drawn right-aligned in the
// shortcut column (S5.6b); a Submenu item shows a chevron there instead
// (and no shortcut). The menu does not disable a submenu item itself: the
// model does.
type menuItem struct {
	Title     string
	Shortcut  string
	Disabled  bool
	Checked   bool
	Submenu   bool
	Separator bool
}

// menu is the generic menu element (PLAN.md S5.5h, the Menu tokens): a
// rounded panel with a column of title and separator items, the content of
// a grab popup (popupRoot) for a pop-up button (S5.5k) and, from S5.6, the
// title-bar menus. It keeps no peer; its owner hears of a pick or a cancel
// through SetActions, once Begin started the interaction (S5.5i,
// menuinput.go: the hover, the clicks and press-drag-release, the
// keyboard, the pick's blink and the scrolling).
//
// Geometry, in the element's own points: the panel is the bounds less
// ShadowMargin. Items stack from PaddingY below the panel's top, a title
// item ItemHeight high for the control size, a separator SeparatorHeight,
// moved up by the scroll offset (ScrollOffset, 0 to MaxScroll: how much
// lower the panel is than its items). While the items can scroll up or
// down, a ScrollArrowHeight strip at the panel's top or bottom shows a
// scroll arrow over them (ArrowRect), and the items show only between the
// strips (ItemAt finds none on them).
// A title starts CheckColumnWidth right of the highlight's left edge
// (HighlightInsetX inside the panel) and is centred vertically in its
// item, in the system font of the control size; a title wider than the
// room left of TitlePaddingRight (and of the shortcut column with its
// ShortcutGap) is truncated with an ellipsis.
// The shortcut column (S5.6b) exists only when an item has a shortcut or
// a submenu, so a pop-up button's menu keeps its geometry: it is as wide as
// the widest shortcut (or a chevron) and ends TitlePaddingRight from the
// panel's right side; shortcuts are right-aligned in it in the titles'
// font and color, chevrons at its right edge. Shortcuts are never
// truncated.
//
// Paint: the menu's own shadow when SetShadow is on (for a shell that draws
// none behind the popup), the panel filled with Menu.Background when its
// popup is translucent (popupRoot.Translucent: the OS blurs behind it) and
// Reduce Transparency is off, Menu.OpaqueBackground otherwise (also
// outside a popup), the highlighted
// item as a rounded accent fill (Theme.MenuHighlight) with its title and
// checkmark in Theme.MenuHighlightedText, the other titles in Text or DisabledText,
// each item's shortcut or submenu chevron in its title's color,
// separators as pixel-aligned lines, and the border on top. In a panel
// lower than its items they are clipped to it and to the part between the
// scroll arrows, each arrow a triangle in Text.
type menu struct {
	Base
	ts    render.TextSystem
	marks scratchPath // the arrows', checkmarks' and chevrons' paths, one at a time

	items     []menuItem
	cs        driver.ControlSize
	minWidth  float64
	shadow    bool
	highlight int     // -1: none
	top       float64 // the scroll offset of the items, 0 to MaxScroll

	// The interaction (S5.5i, menuinput.go): its state, the reports and
	// the scroll arrows' ticker, made on first use.
	trk      menuTrack
	onPick   func(i int)
	onCancel func()
	ticker   *anim.Ticker
	// The owner's Left and Right (SetSideKeys, S5.6i) and whether the
	// last cancel came from a key (CancelledByKey).
	onSide    func(dir int)
	keyCancel bool

	// The laid-out titles (nil for separators and empty titles), made
	// lazily for the current font, and the truncated ones for the width
	// truncW.
	layouts []render.TextLayout
	trunc   []render.TextLayout
	truncW  float64

	// The laid-out shortcuts (nil where an item has none), made with the
	// titles, and the shortcut column's width (-1: not computed).
	shortcuts []render.TextLayout
	colW      float64

	// The tops of the items and the bottom of the last one (itemY), nil
	// until needed.
	offsets []float64
}

// newMenu returns an empty menu laid out by ts. It is focusable, so Begin
// can give it its popup's keys.
func newMenu(ts render.TextSystem) *menu {
	m := &menu{ts: ts, highlight: -1, trk: menuTrack{picking: -1}, colW: -1}
	m.Init(m)
	m.SetFocusable(true)
	return m
}

// SetItems replaces the items (copied), clears the highlight and clamps
// the scroll offset; a pick blinking ends without a report.
func (m *menu) SetItems(items []menuItem) {
	if m.trk.picking >= 0 {
		m.End()
	}
	m.items = slices.Clone(items)
	m.highlight = -1
	m.dropLayouts()
	m.SetScrollOffset(m.top)
	m.Invalidate()
}

// ControlSize returns the control size the items are drawn at.
func (m *menu) ControlSize() driver.ControlSize { return m.cs }

// SetControlSize sets the control size: the item height and the font.
func (m *menu) SetControlSize(cs driver.ControlSize) {
	if cs != m.cs {
		m.cs = cs
		m.dropLayouts()
		m.SetScrollOffset(m.top)
		m.Invalidate()
	}
}

// SetMinWidth sets the least width of the panel (a pop-up button's menu is
// at least as wide as the button), the shadow margin not included.
func (m *menu) SetMinWidth(w float64) {
	m.minWidth = max(0, w)
}

// SetShadow sets whether the menu draws its own shadow, around the panel
// inside ShadowMargin (off by default: the Win32 popups have the OS's).
func (m *menu) SetShadow(on bool) {
	if on != m.shadow {
		m.shadow = on
		m.Invalidate()
	}
}

// ShadowMargin returns the space around the panel the menu's own shadow
// needs: zero without SetShadow, otherwise the blur on every side, less the
// offset above and more below.
func (m *menu) ShadowMargin() driver.Insets {
	if !m.shadow {
		return driver.Insets{}
	}
	return shadowInsets(tokens.Menu.ShadowBlur, tokens.Menu.ShadowOffsetY)
}

// Font returns the font of the titles: the system font of the control
// size.
func (m *menu) Font() driver.Font { return toggleFont(m.cs) }

// Highlighted returns the index of the highlighted item, or -1.
func (m *menu) Highlighted() int { return m.highlight }

// SetHighlighted highlights item i, or nothing for an index that is not a
// Selectable item.
func (m *menu) SetHighlighted(i int) {
	if !m.Selectable(i) {
		i = -1
	}
	if i != m.highlight {
		m.highlight = i
		m.Invalidate()
	}
}

// Selectable reports whether item i exists and can be highlighted and
// picked: an enabled title item.
func (m *menu) Selectable(i int) bool {
	return i >= 0 && i < len(m.items) && !m.items[i].Separator && !m.items[i].Disabled
}

// Panel returns the panel's rect: the bounds less ShadowMargin.
func (m *menu) Panel() driver.Rect {
	return m.Bounds().Inset(m.ShadowMargin())
}

// itemHeight returns the height of item i.
func (m *menu) itemHeight(i int) float64 {
	if m.items[i].Separator {
		return tokens.Menu.SeparatorHeight
	}
	return tokens.Menu.ItemHeight.At(m.cs)
}

// ContentHeight returns the height of every item with the padding above
// and below: the panel's height that shows them all.
func (m *menu) ContentHeight() float64 {
	return m.itemY(len(m.items)) + tokens.Menu.PaddingY
}

// itemY returns the top of item i in the unscrolled content, from the
// panel's top: the padding and every item above it (i = len(items): the
// bottom of the last item). The offsets are summed once per item list and
// control size (P12).
func (m *menu) itemY(i int) float64 {
	if m.offsets == nil {
		m.offsets = make([]float64, len(m.items)+1)
		y := tokens.Menu.PaddingY
		for j := range m.items {
			m.offsets[j] = y
			y += m.itemHeight(j)
		}
		m.offsets[len(m.items)] = y
	}
	return m.offsets[i]
}

// ItemRect returns the rect of item i in the element's points, at the
// current scroll offset: the panel's width, the item's height (an empty
// rect for an unknown index). Part of it may lie outside the panel or
// under a scroll arrow.
func (m *menu) ItemRect(i int) driver.Rect {
	if i < 0 || i >= len(m.items) {
		return driver.Rect{}
	}
	p := m.Panel()
	return driver.Rect{X: p.X, Y: p.Y + m.itemY(i) - m.top, W: p.W, H: m.itemHeight(i)}
}

// ItemAt returns the index of the item at p (in the element's points), or
// -1 outside the items: in the padding, on a scroll arrow, outside the
// panel or beside it.
func (m *menu) ItemAt(p driver.Point) int {
	area := m.itemsArea()
	if p.X < area.X || p.X >= area.X+area.W || p.Y < area.Y || p.Y >= area.Y+area.H {
		return -1
	}
	// The last item whose top is at or above y (binary search, P12; a
	// zero-height item before it is skipped, as it holds no point).
	y := p.Y - m.Panel().Y + m.top
	m.itemY(0) // the offsets
	i := sort.Search(len(m.items), func(j int) bool { return m.offsets[j] > y }) - 1
	if i < 0 || y >= m.offsets[i+1] {
		return -1
	}
	return i
}

// MaxScroll returns the largest scroll offset: how much lower the panel
// is than its items with their padding (zero when they all fit).
func (m *menu) MaxScroll() float64 {
	return max(0, m.ContentHeight()-m.Panel().H)
}

// ScrollOffset returns how far the items are scrolled up, from 0 to
// MaxScroll.
func (m *menu) ScrollOffset() float64 { return m.top }

// SetScrollOffset scrolls the items to y, clamped to [0, MaxScroll].
func (m *menu) SetScrollOffset(y float64) {
	y = min(max(y, 0), m.MaxScroll())
	if y != m.top {
		m.top = y
		m.Invalidate()
	}
}

// SetFrame implements Element: a new height clamps the scroll offset.
func (m *menu) SetFrame(r driver.Rect) {
	m.Base.SetFrame(r)
	m.SetScrollOffset(m.top)
}

// ScrollToItem scrolls the least that shows all of item i between the
// scroll arrows (nothing for an unknown index or when it shows already).
func (m *menu) ScrollToItem(i int) {
	if i < 0 || i >= len(m.items) || m.MaxScroll() == 0 {
		return
	}
	ah := m.arrowHeight()
	y, h, ph := m.itemY(i), m.itemHeight(i), m.Panel().H
	switch {
	case y < m.top+m.arrowStrip(-1):
		// At the top, under the up arrow unless that scrolls to 0.
		m.SetScrollOffset(max(0, y-ah))
	case y+h > m.top+ph-m.arrowStrip(1):
		top := y + h + ah - ph
		if top >= m.MaxScroll() {
			top = m.MaxScroll()
		}
		m.SetScrollOffset(top)
	}
}

// arrowHeight returns the height of a scroll arrow's strip: the token, at
// most a third of the panel.
func (m *menu) arrowHeight() float64 {
	return min(tokens.Menu.ScrollArrowHeight, m.Panel().H/3)
}

// arrowStrip returns the height of the scroll arrow at the top (dir < 0)
// or the bottom (dir > 0): arrowHeight while the items can scroll that
// way, zero otherwise.
func (m *menu) arrowStrip(dir int) float64 {
	if dir < 0 && m.top > 0 || dir > 0 && m.top < m.MaxScroll() {
		return m.arrowHeight()
	}
	return 0
}

// ArrowRect returns the strip of the scroll arrow at the top (dir < 0) or
// the bottom (dir > 0) of the panel, empty while it is not shown.
func (m *menu) ArrowRect(dir int) driver.Rect {
	h := m.arrowStrip(dir)
	if h == 0 {
		return driver.Rect{}
	}
	p := m.Panel()
	if dir < 0 {
		return driver.Rect{X: p.X, Y: p.Y, W: p.W, H: h}
	}
	return driver.Rect{X: p.X, Y: p.Y + p.H - h, W: p.W, H: h}
}

// itemsArea returns the part of the panel the items show in: the panel
// less the shown scroll arrows.
func (m *menu) itemsArea() driver.Rect {
	p := m.Panel()
	up, down := m.arrowStrip(-1), m.arrowStrip(1)
	p.Y += up
	p.H = max(0, p.H-up-down)
	return p
}

// titleX returns the left edge of the titles from the panel's left edge.
func titleX() float64 { return tokens.Menu.HighlightInsetX + tokens.Menu.CheckColumnWidth }

// FittingSize returns the size that shows every item with its whole
// title: the widest title with the check column and the paddings, and the
// shortcut column with its gap when there is one (at least the minimum
// width), by ContentHeight, plus ShadowMargin, clamped to max.
func (m *menu) FittingSize(max driver.Size) driver.Size {
	w := 0.0
	for i := range m.items {
		if l := m.layout(i); l != nil {
			w = math.Max(w, l.Size().W)
		}
	}
	w = math.Max(math.Ceil(titleX()+w+m.columnSpace()+tokens.Menu.TitlePaddingRight), m.minWidth)
	in := m.ShadowMargin()
	return driver.Size{W: w + in.Left + in.Right, H: m.ContentHeight() + in.Top + in.Bottom}.Min(max)
}

// ShortcutColumn returns the width of the shortcut column: the widest
// shortcut or, with a submenu item, at least a chevron's width; zero when
// no item has either.
func (m *menu) ShortcutColumn() float64 {
	if m.colW >= 0 {
		return m.colW
	}
	w := 0.0
	for i, it := range m.items {
		switch {
		case it.Separator:
		case it.Submenu:
			w = math.Max(w, chevronWidth())
		case m.shortcutLayout(i) != nil:
			w = math.Max(w, m.shortcutLayout(i).Size().W)
		}
	}
	m.colW = w
	return w
}

// columnSpace returns the room the shortcut column takes right of the
// titles: the column with its gap, or zero without one.
func (m *menu) columnSpace() float64 {
	if c := m.ShortcutColumn(); c > 0 {
		return c + tokens.Menu.ShortcutGap
	}
	return 0
}

// chevronWidth returns the width of a submenu chevron: half its height.
func chevronWidth() float64 { return tokens.Menu.SubmenuChevronSize / 2 }

// ShortcutOrigin returns the top-left corner of item i's shortcut layout in
// the element's points, right-aligned in the shortcut column, or the zero
// point for an item without a shortcut (a submenu item has none).
func (m *menu) ShortcutOrigin(i int) driver.Point {
	l := m.shortcutLayout(i)
	if l == nil || m.items[i].Submenu {
		return driver.Point{}
	}
	r := m.ItemRect(i)
	s := l.Size()
	return driver.Point{X: r.X + r.W - tokens.Menu.TitlePaddingRight - s.W, Y: r.Y + (r.H-s.H)/2}
}

// ChevronRect returns the box of item i's submenu chevron in the element's
// points: chevronWidth wide and SubmenuChevronSize high, at the shortcut
// column's right edge and centred vertically; empty for an item that is no
// submenu.
func (m *menu) ChevronRect(i int) driver.Rect {
	if i < 0 || i >= len(m.items) || m.items[i].Separator || !m.items[i].Submenu {
		return driver.Rect{}
	}
	r := m.ItemRect(i)
	w, h := chevronWidth(), tokens.Menu.SubmenuChevronSize
	return driver.Rect{X: r.X + r.W - tokens.Menu.TitlePaddingRight - w, Y: r.Y + (r.H-h)/2, W: w, H: h}
}

// dropLayouts forgets the laid-out titles and shortcuts and the item
// offsets.
func (m *menu) dropLayouts() {
	m.offsets = nil
	m.layouts, m.trunc, m.shortcuts, m.colW = nil, nil, nil, -1
}

// shortcutLayout returns item i's shortcut on one line, or nil for an item
// without one, a separator or an unknown index.
func (m *menu) shortcutLayout(i int) render.TextLayout {
	if i < 0 || i >= len(m.items) || m.items[i].Separator || m.items[i].Shortcut == "" {
		return nil
	}
	if m.shortcuts == nil {
		m.shortcuts = make([]render.TextLayout, len(m.items))
	}
	if m.shortcuts[i] == nil {
		m.shortcuts[i] = m.ts.NewLayout(oneLine(m.items[i].Shortcut), m.Font(), 0)
	}
	return m.shortcuts[i]
}

// layout returns item i's title on one line, or nil for a separator, an
// empty title or an unknown index.
func (m *menu) layout(i int) render.TextLayout {
	if i < 0 || i >= len(m.items) || m.items[i].Separator || m.items[i].Title == "" {
		return nil
	}
	if m.layouts == nil {
		m.layouts = make([]render.TextLayout, len(m.items))
	}
	if m.layouts[i] == nil {
		m.layouts[i] = m.ts.NewLayout(oneLine(m.items[i].Title), m.Font(), 0)
	}
	return m.layouts[i]
}

// shownLayout returns item i's title as drawn in a panel width wide: the
// layout, truncated when it does not fit (nil when not even the ellipsis
// does).
func (m *menu) shownLayout(i int, width float64) render.TextLayout {
	l := m.layout(i)
	room := width - titleX() - m.columnSpace() - tokens.Menu.TitlePaddingRight
	if l == nil || l.Size().W <= room {
		return l
	}
	if m.trunc == nil || m.truncW != width {
		m.trunc, m.truncW = make([]render.TextLayout, len(m.items)), width
	}
	if m.trunc[i] == nil {
		m.trunc[i] = truncate(m.ts, m.items[i].Title, m.Font(), max(0, room))
	}
	return m.trunc[i]
}

// translucent reports whether the menu's popup is blurred behind by the
// OS and Reduce Transparency is off: with it the menu paints opaque even
// on a shell that still blurs behind the popup (S6.1f).
func (m *menu) translucent() bool {
	if p, ok := m.Parent().(*popupRoot); ok {
		return p.Translucent() && !m.appearance().ReduceTransparency
	}
	return false
}

// appearance returns the appearance of the menu's window, or the default
// outside one.
func (m *menu) appearance() driver.Appearance {
	if w := m.Window(); w != nil {
		return w.Appearance()
	}
	return driver.DefaultAppearance()
}

// Paint draws the menu (see menu).
func (m *menu) Paint(c render.Canvas) {
	p := m.Panel()
	if p.Empty() {
		return
	}
	a := m.appearance()
	mt := &tokens.Menu
	scale := scaleIn(&m.Base)
	radii := render.UniformRadii(min(mt.CornerRadius, p.W/2, p.H/2))

	if m.shadow {
		c.DrawShadow(p, radii, render.Shadow{Offset: driver.Point{Y: mt.ShadowOffsetY}, Blur: mt.ShadowBlur, Color: mt.Shadow.Value(a)})
	}
	bg := mt.OpaqueBackground
	if m.translucent() {
		bg = mt.Background
	}
	c.FillRoundedRect(p, radii, render.Solid(bg.Value(a)))

	clip := m.ContentHeight() > p.H
	area := m.itemsArea()
	if clip {
		c.Save()
		c.ClipRoundedRect(p, radii)
		c.ClipRoundedRect(area, render.Radii{})
	}
	for i := range m.items {
		r := m.ItemRect(i)
		if r.Y+r.H <= area.Y {
			continue
		}
		if r.Y >= area.Y+area.H {
			break
		}
		if m.items[i].Separator {
			m.paintSeparator(c, r, scale, a)
			continue
		}
		m.paintItem(c, i, r, a)
	}
	if clip {
		c.Restore()
		m.paintArrow(c, -1, a)
		m.paintArrow(c, 1, a)
	}

	strokeInside(c, p, radii.TopLeft, pixelWidth(mt.BorderWidth, scale), mt.Border.Value(a))
}

// paintArrow draws the scroll arrow at the top (dir < 0) or the bottom
// (dir > 0) while it is shown: a triangle in Text centred in its strip.
func (m *menu) paintArrow(c render.Canvas, dir int, a driver.Appearance) {
	r := m.ArrowRect(dir)
	if r.Empty() {
		return
	}
	w := tokens.Menu.ScrollArrowSize
	h := w / 2
	x, y := r.X+(r.W-w)/2, r.Y+(r.H-h)/2
	path := m.marks.scratch()
	if dir < 0 {
		path.MoveTo(driver.Point{X: x, Y: y + h}).LineTo(driver.Point{X: x + w/2, Y: y}).LineTo(driver.Point{X: x + w, Y: y + h})
	} else {
		path.MoveTo(driver.Point{X: x, Y: y}).LineTo(driver.Point{X: x + w/2, Y: y + h}).LineTo(driver.Point{X: x + w, Y: y})
	}
	c.FillPath(path.Close(), render.Solid(tokens.Menu.Text.Value(a)))
}

// paintSeparator draws a separator item's line in r, pixel-aligned.
func (m *menu) paintSeparator(c render.Canvas, r driver.Rect, scale float64, a driver.Appearance) {
	mt := &tokens.Menu
	line := hairline(&m.Base, driver.Rect{X: r.X + mt.SeparatorInsetX, Y: r.Y, W: r.W - 2*mt.SeparatorInsetX, H: r.H},
		mt.SeparatorThickness, scale, false)
	if line.W > 0 && line.H > 0 {
		c.FillRoundedRect(line, render.Radii{}, render.Solid(mt.Separator.Value(a)))
	}
}

// paintItem draws title item i in r: its highlight, checkmark and title.
func (m *menu) paintItem(c render.Canvas, i int, r driver.Rect, a driver.Appearance) {
	mt := &tokens.Menu
	it := &m.items[i]
	col := mt.Text.Value(a)
	switch {
	case i == m.highlight:
		hl := driver.Rect{X: r.X + mt.HighlightInsetX, Y: r.Y, W: r.W - 2*mt.HighlightInsetX, H: r.H}
		if hl.W > 0 {
			c.FillRoundedRect(hl, render.UniformRadii(min(mt.HighlightRadius, hl.W/2, hl.H/2)), render.Solid(tokens.MenuHighlight(a)))
		}
		col = tokens.MenuHighlightedText(a)
	case it.Disabled:
		col = mt.DisabledText.Value(a)
	}
	if it.Checked {
		s := mt.CheckmarkSize
		box := driver.Rect{
			X: r.X + mt.HighlightInsetX + (mt.CheckColumnWidth-s)/2,
			Y: r.Y + (r.H-s)/2,
			W: s, H: s,
		}
		c.StrokePath(checkMark(m.marks.scratch(), box, false), mt.CheckmarkStroke, render.Solid(col),
			render.StrokeStyle{Cap: render.CapRound, Join: render.JoinRound})
	}
	if l := m.shownLayout(i, r.W); l != nil {
		c.DrawText(l, driver.Point{X: r.X + titleX(), Y: r.Y + (r.H-l.Size().H)/2}, col)
	}
	if it.Submenu {
		b := m.ChevronRect(i)
		path := m.marks.scratch().MoveTo(driver.Point{X: b.X, Y: b.Y}).
			LineTo(driver.Point{X: b.X + b.W, Y: b.Y + b.H/2}).
			LineTo(driver.Point{X: b.X, Y: b.Y + b.H})
		c.StrokePath(path, mt.SubmenuChevronStroke, render.Solid(col),
			render.StrokeStyle{Cap: render.CapRound, Join: render.JoinRound})
	} else if l := m.shortcutLayout(i); l != nil {
		c.DrawText(l, m.ShortcutOrigin(i), col)
	}
}

// AccessibleNode implements Element: an open menu (S5.6c), its items
// exposed by AccessibleItems.
func (m *menu) AccessibleNode() a11y.Node { return a11y.Node{Role: a11y.RoleMenu} }

// AccessibleItems implements accessibleItems: a RoleMenuItem per title
// item, labelled with its title, with its shortcut text as the value,
// selected while highlighted, checked when checked, disabled when
// disabled and pressable otherwise; a RoleSeparator per separator. Frames
// are the items' rects at the current scroll offset.
func (m *menu) AccessibleItems() []a11y.Node {
	out := make([]a11y.Node, len(m.items))
	for i, it := range m.items {
		n := a11y.Node{Role: a11y.RoleSeparator, Frame: m.ItemRect(i)}
		if !it.Separator {
			n.Role, n.Label, n.Value = a11y.RoleMenuItem, it.Title, it.Shortcut
			if it.Disabled {
				n.States |= a11y.StateDisabled
			} else {
				n.Actions |= a11y.ActionPress
			}
			if it.Checked {
				n.States |= a11y.StateChecked
			}
			if i == m.highlight {
				n.States |= a11y.StateSelected
			}
		}
		out[i] = n
	}
	return out
}
