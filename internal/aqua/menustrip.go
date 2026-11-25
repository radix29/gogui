package aqua

import (
	"math"
	"time"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/aqua/menubar"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
)

// menuStrip is the title-bar menu strip of a titled window on Windows and
// Linux (PLAN.md §5.4.1, S5.6c): the menus of the driver's menubar.Bar as
// titles right of the traffic lights, the app menu's in the emphasized
// weight, the others regular, in MenuBar.Text (MenuBar.InactiveText in an
// inactive window). The open or keyboard-focused title has a rounded,
// accent-free MenuBar.Highlight behind it (SetHighlighted). A press on a
// title opens its menu (S5.6f, below).
//
// Geometry: menubar.Layout over the titles' widths and the Chrome and
// MenuBar tokens places the titles in slots from the lights on and drops
// the ones that do not fit from the right, so MinDragWidth of caption
// stays; its TitleRoom is the room of the centred window title, which the
// title bar truncates to it. The strip's frame covers the shown slots (it
// is hidden when none is shown), each title is a child element covering
// its slot, and the title bar exposes each shown slot as a RegionClient
// chrome region between its caption and its lights, so a press there is
// delivered instead of moving the window. The window can still be dragged
// between and right of the titles.
//
// Accessibility: the strip is a RoleMenuBar node, each shown title a
// RoleMenuBarItem labelled with its title.
type menuStrip struct {
	Base
	tb  *titleBar
	bar *menubar.Bar

	all       []*menuStripTitle // a title per menu, laid out once per bar
	room      float64           // the window title's room (menubar.Strip.TitleRoom)
	highlight int               // the highlighted menu's index, -1: none

	// The open menu (S5.6f): the menu element, made at the first opening
	// and reused, its popup or nil, the open menu's index and the time of
	// the press that opened it (or of the last one on the strip).
	menu    *menu
	open    *popupRoot
	openAt  int
	pressAt time.Duration

	// The accelerator flash (S5.6h): the timer that ends it, or nil.
	flashEnd func()

	// The keyboard access (S5.6i, menukeys.go): the strip has the keys,
	// the type-select (typeselect.go), and
	// keepKbd while a key's cancel closes a menu of the focused strip.
	kbd     bool
	typed   typeSelect
	keepKbd bool
}

// menuStripTitle is one title of the strip, covering its slot.
type menuStripTitle struct {
	Base
	strip *menuStrip
	index int // the menu's index in the bar
	text  render.TextLayout
}

// menuBarFont returns the font of a strip title: the regular system font,
// the app menu's in the emphasized weight.
func menuBarFont(app bool) driver.Font {
	f := systemFont(driver.ControlSizeRegular)
	if app {
		f.Weight = tokens.Typography.EmphasizedWeight
	}
	return f
}

// newMenuStrip returns the strip of bar (non-nil) in tb, with its titles
// laid out and none shown until layout.
func newMenuStrip(tb *titleBar, bar *menubar.Bar) *menuStrip {
	s := &menuStrip{tb: tb, bar: bar, highlight: -1, openAt: -1}
	s.Init(s)
	ts := tb.w.text
	for i, m := range bar.Menus {
		t := &menuStripTitle{strip: s, index: i}
		t.Init(t)
		t.text = ts.NewLayout(oneLine(m.Title), menuBarFont(m.Role == driver.MenuApp), 0)
		s.all = append(s.all, t)
	}
	return s
}

// menuStripMetrics returns the inputs of menubar.Layout for a title bar width wide.
func menuStripMetrics(width float64) menubar.Metrics {
	mb := &tokens.MenuBar
	return menubar.Metrics{
		Width:       width,
		Start:       tokens.Chrome.LightInset + lightsWidth() + mb.StartGap,
		Pad:         mb.TitlePaddingX,
		MinDrag:     mb.MinDragWidth,
		TitleGap:    mb.WindowTitleGap,
		TitleMargin: titleMargin(),
	}
}

// layout places the titles for a title bar width wide and returns the
// window title's room.
func (s *menuStrip) layout(width float64) float64 {
	widths := make([]float64, len(s.all))
	for i, t := range s.all {
		widths[i] = t.text.Size().W
	}
	m := menuStripMetrics(width)
	st := menubar.Layout(widths, m)
	h := tokens.Chrome.TitleBarHeight
	shown := make([]Element, len(st.Titles))
	for i, sp := range st.Titles {
		t := s.all[i]
		t.SetFrame(driver.Rect{X: sp.X - m.Start, W: sp.W, H: h})
		shown[i] = t
	}
	if s.open != nil && s.openAt >= len(shown) {
		s.CloseMenu() // its title was dropped
	}
	if s.kbd && s.highlight >= len(shown) {
		s.leaveKeyboard() // the highlighted title was dropped
	}
	s.SetChildren(shown...)
	s.SetFrame(driver.Rect{X: m.Start, W: st.End - m.Start, H: h})
	s.SetHidden(len(shown) == 0)
	s.room = st.TitleRoom
	s.Invalidate()
	return st.TitleRoom
}

// Shown returns the number of shown titles: the first Shown menus of the
// bar.
func (s *menuStrip) Shown() int { return s.ChildCount() }

// TitleRect returns the slot of shown title i in surface coordinates, or
// an empty rect when it is not shown.
func (s *menuStrip) TitleRect(i int) driver.Rect {
	if i < 0 || i >= s.ChildCount() {
		return driver.Rect{}
	}
	t := s.Child(i)
	return driver.RectOf(ToRoot(t, driver.Point{}), t.base().Frame().Size())
}

// Highlighted returns the highlighted menu's index, or -1.
func (s *menuStrip) Highlighted() int { return s.highlight }

// SetHighlighted highlights menu i (-1, or an index out of range: none).
func (s *menuStrip) SetHighlighted(i int) {
	if i < 0 || i >= len(s.all) {
		i = -1
	}
	if i == s.highlight {
		return
	}
	s.highlight = i
	s.Invalidate()
	for _, t := range s.all {
		t.Invalidate()
	}
}

// regions appends a RegionClient chrome region per shown title, in
// surface coordinates.
func (s *menuStrip) regions(rs []shell.ChromeRegion) []shell.ChromeRegion {
	for i := range s.ChildCount() {
		rs = append(rs, shell.ChromeRegion{Rect: s.TitleRect(i), Kind: shell.RegionClient})
	}
	return rs
}

func (s *menuStrip) AccessibleNode() a11y.Node {
	return a11y.Node{Role: a11y.RoleMenuBar}
}

func (t *menuStripTitle) Paint(c render.Canvas) {
	w := t.strip.tb.w
	a := w.appearance
	mb := &tokens.MenuBar
	b := t.Bounds()
	if t.strip.highlight == t.index {
		h := mb.HighlightHeight
		c.FillRoundedRect(driver.Rect{Y: (b.H - h) / 2, W: b.W, H: h},
			render.UniformRadii(mb.HighlightRadius), render.Solid(mb.Highlight.Value(a)))
	}
	col := mb.Text
	if !w.key {
		col = mb.InactiveText
	}
	c.DrawText(t.text, driver.Point{X: mb.TitlePaddingX, Y: (b.H - t.text.Size().H) / 2}, col.Value(a))
}

func (t *menuStripTitle) AccessibleNode() a11y.Node {
	return a11y.Node{Role: a11y.RoleMenuBarItem, Label: t.strip.bar.Menus[t.index].Title}
}

// The strip's menus (PLAN.md S5.6f), as the macOS menu bar's by mouse:
//
//   - A primary press on a title opens its menu as a grab popup below the
//     title, the panel's left edge on the title's highlight, sliding
//     horizontally to stay on the screen and cut (and scrolled) to it
//     vertically (menuStripAdjust), the title highlighted while it is
//     open. The strip is the popup's track (shell.PopupConfig.Track), so
//     the pointer over it still reaches the menu's popup.
//   - Moving over another title switches menus: the open popup is
//     repositioned under that title with its menu's items (one grab
//     popup throughout, so a button held across the titles stays held
//     for it); a held press goes on as a dragged press-drag-release in
//     the new menu.
//   - A press on the open title closes the menu; a press on another title
//     (a move has normally switched to it already) opens that one as a
//     new press. Releasing the press over the strip leaves the menu open
//     for a click, at any time.
//   - The menu element does the rest (menu.Begin): press-drag-release
//     with its release delay, clicks, hover and the keyboard. A pick
//     closes the menu and reports the item to the window's menu handler
//     (the driver: an item with an ID to MenuItemSelected, a role item
//     without one to its action, S5.6g); a cancel closes it without a
//     report. The items are validated each time a menu opens or is
//     switched to (menuItems).
//   - The menu closes without a report on a dismissal by the OS (on gtk4
//     also a press on the strip: GDK's grab drops it, S5.6e), on the
//     window's key loss or close, when SetMenuBar replaces the strip and
//     when a relayout drops the open title.

// menuStripAdjust: the menu slides horizontally and gets shorter (and
// scrolls) when taller than the screen; it never slides up over the strip.
const menuStripAdjust = shell.AdjustSlideX | shell.AdjustResizeY

// OpenMenu returns the index of the open menu, or -1.
func (s *menuStrip) OpenMenu() int {
	if s.open == nil {
		return -1
	}
	return s.openAt
}

// titleAt returns the index of the shown title at p (surface points), or
// -1.
func (s *menuStrip) titleAt(p driver.Point) int {
	for i := range s.Shown() {
		if s.TitleRect(i).Contains(p) {
			return i
		}
	}
	return -1
}

// menuItems returns the menu element's items for menu i of the bar, each
// validated now (Window.menuEnabled, S5.6g): a role item is enabled by
// what it can act on at the opening, as AppKit validates its menus.
func (s *menuStrip) menuItems(i int) []menuItem {
	src := s.bar.Menus[i].Items
	enabled := s.tb.w.menuEnabled
	items := make([]menuItem, len(src))
	for j, it := range src {
		off := it.Disabled
		if !off && !it.Separator && enabled != nil {
			off = !enabled(menubar.Ref{Menu: i, Item: j})
		}
		items[j] = menuItem{
			Title: it.Title, Shortcut: it.ShortcutText, Disabled: off,
			Checked: it.Checked, Submenu: it.Submenu, Separator: it.Separator,
		}
	}
	return items
}

// openMenu opens menu i (a shown title) below its title, or switches the
// open menu to it, and begins its interaction as op says, op.Pos in
// surface points; it reports whether the menu is open.
func (s *menuStrip) openMenu(i int, op menuOpening) bool {
	w := s.tb.w
	if i < 0 || i >= s.Shown() || w.closed || !s.VisibleInTree() {
		return false
	}
	s.stopFlash()
	if s.menu == nil {
		s.menu = newMenu(w.text)
	}
	m := s.menu
	m.End()
	m.SetControlSize(driver.ControlSizeRegular)
	m.SetShadow(!w.sh.Caps().PopupShadow)
	m.SetItems(s.menuItems(i))
	size := m.FittingSize(driver.Size{W: math.Inf(1), H: math.Inf(1)})
	m.SetFrame(driver.Rect{W: size.W, H: size.H})
	m.SetScrollOffset(0)
	anchor := s.TitleRect(i)
	panel := m.Panel()
	cfg := shell.PopupConfig{
		Anchor: anchor, Size: size,
		Offset: driver.Point{X: -panel.X, Y: anchor.H - panel.Y},
		Adjust: menuStripAdjust, Grab: true,
		Track: driver.RectOf(ToRoot(s, driver.Point{}), s.Frame().Size()),
	}
	m.SetActions(func(j int) { s.picked(i, j) }, s.menuCancelled)
	m.SetSideKeys(s.side)
	if s.open != nil {
		s.open.Reposition(cfg)
	} else {
		root, err := w.openPopup(popupOptions{
			Config:     cfg,
			Content:    m,
			Closed:     s.menuClosed,
			TrackEvent: s.trackEvent,
		})
		if err != nil || root.Closed() {
			return false
		}
		s.open = root
	}
	s.openAt = i
	s.SetHighlighted(i)
	if op.Press && op.HasPos {
		op.Pos = op.Pos.Sub(s.open.origin())
	}
	m.Begin(op)
	return true
}

// CloseMenu closes the open menu without a report.
func (s *menuStrip) CloseMenu() {
	if s.open != nil {
		s.open.Close() // menuClosed clears the state
	}
}

// menuClosed is the popup's Closed hook: the strip leaves the keyboard
// focus too, unless a key's cancel keeps it (menuCancelled), its title
// then still highlighted.
func (s *menuStrip) menuClosed() {
	s.open, s.openAt = nil, -1
	if s.keepKbd {
		return
	}
	s.leaveKeyboard()
	s.SetHighlighted(-1)
}

// picked closes the menu and reports the pick of item j of menu i.
func (s *menuStrip) picked(i, j int) {
	s.CloseMenu()
	w := s.tb.w
	if !w.closed && w.menuPicked != nil {
		w.menuPicked(menubar.Ref{Menu: i, Item: j})
	}
}

// trackEvent handles the open menu's pointer events over the strip, in
// surface points (popupOptions.TrackEvent).
func (s *menuStrip) trackEvent(ev shell.PointerEvent) {
	if s.open == nil || s.menu.Picking() {
		return // the pick's blink eats the input
	}
	i := s.titleAt(ev.Pos)
	switch ev.Kind {
	case shell.PointerEnter, shell.PointerMove:
		if i >= 0 && i != s.openAt {
			held := ev.Buttons&shell.ButtonPrimary != 0
			s.openMenu(i, menuOpening{Press: held, Time: s.pressAt, Dragged: true, Pos: ev.Pos, HasPos: held})
		}
	case shell.PointerDown:
		if ev.Button != shell.ButtonPrimary || i < 0 {
			return
		}
		if i == s.openAt {
			s.CloseMenu()
			return
		}
		s.pressAt = ev.Time
		s.openMenu(i, menuOpening{Press: true, Time: ev.Time, Pos: ev.Pos, HasPos: true})
	case shell.PointerUp:
		if ev.Buttons == 0 {
			s.menu.EndPress()
		}
	}
}

// HandleEvent implements Element: a primary press on a title opens its
// menu (the press goes on in the menu: press-drag-release).
func (t *menuStripTitle) HandleEvent(ev Event) bool {
	pe, ok := ev.(PointerEvent)
	if !ok || pe.Kind != PointerDown || pe.Button != shell.ButtonPrimary {
		return false
	}
	s := t.strip
	s.pressAt = pe.Time
	return s.openMenu(t.index, menuOpening{Press: true, Time: pe.Time, Pos: ToRoot(t, pe.Pos), HasPos: true})
}

// menuFlash is how long an accelerator highlights its menu's title
// (S5.6h; provisional like the menu timings, from memory of macOS).
const menuFlash = 100 * time.Millisecond

// flash highlights the title of menu i for menuFlash, as macOS flashes a
// menu title when its key equivalent runs (S5.6h). Nothing flashes with
// Reduce Motion, while a menu is open (its title is highlighted already)
// or when the title is not shown; a flash running already restarts (an
// auto-repeat). The flash ends on its own and leaves the highlight alone
// when a menu has opened since; Reduce Motion turned on ends it at once
// (HandleEvent).
func (s *menuStrip) flash(i int) {
	w := s.tb.w
	if w.closed || s.open != nil || i < 0 || i >= s.Shown() || w.appearance.ReduceMotion {
		return
	}
	s.stopFlash()
	s.SetHighlighted(i)
	s.flashEnd = w.sh.After(menuFlash, s.endFlash)
}

// endFlash ends a running flash as its timer does: the title's highlight
// goes unless a menu has opened since.
func (s *menuStrip) endFlash() {
	s.flashEnd = nil
	if s.open == nil {
		s.SetHighlighted(-1)
	}
}

// HandleEvent implements Element: Reduce Motion turned on during an
// accelerator flash (the window's WindowStateEvent) ends it at once
// (S6.1f).
func (s *menuStrip) HandleEvent(ev Event) bool {
	if se, ok := ev.(WindowStateEvent); ok && se.Appearance.ReduceMotion && s.flashEnd != nil {
		s.stopFlash()
		s.endFlash()
	}
	return false
}

// Flashing reports whether an accelerator flash runs.
func (s *menuStrip) Flashing() bool { return s.flashEnd != nil }

// stopFlash ends a running flash at once (its highlight stays for the
// caller to change).
func (s *menuStrip) stopFlash() {
	if s.flashEnd != nil {
		s.flashEnd()
		s.flashEnd = nil
	}
}
