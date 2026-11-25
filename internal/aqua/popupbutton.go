package aqua

import (
	"math"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
)

// PopUpButton is the aqua driver.PopUpButton (PLAN.md S5.5j): a pop-up
// button drawn from the theme, a push-button bezel with the selected title
// and the up/down indicator of macOS 11+. The embedded stub pop-up button
// keeps the state (items, selection, control size) and the User…
// simulations.
//
//   - Size: FittingSize is the stub's (the widest item in the fake metrics
//     plus 32 pt, the fake control height), so no example frame moves; the
//     PopUp tokens keep the title, the gap and the indicator within those
//     32 pt (a theme test). A stub Fixed size wins, as there.
//   - Bezel: as the push button's (Metrics.Control, never accent-filled):
//     the frame's width, ControlHeight high (at most the frame's height),
//     centred vertically and pixel-aligned, with its shadow, fill
//     (ButtonFill) and 1 pt BezelBorder.
//   - Title: the selected item on one line in the system font of the
//     control size, PopUp.PaddingX from the bezel's left edge, centred
//     vertically and truncated with an ellipsis at IndicatorGap before the
//     indicator, in PopUpText. No selection draws no title.
//   - Indicator: a rounded rect IndicatorWidth wide, inset IndicatorInset
//     from the bezel's top, right and bottom (its radius the bezel's minus
//     the inset), filled per PopUpIndicatorFill (the accent in a key
//     window), with two chevrons stacked in its centre, ChevronGap apart,
//     stroked in PopUpChevron; with Increase Contrast a filled indicator
//     gets the BezelOutline (S6.1e). Dropped when the bezel is too small for it;
//     the chevrons alone when the indicator is too low for them.
//   - States (ControlState): pressed while a primary press is over it
//     (BezelPressedFill, AccentPressed indicator); in an inactive window
//     the indicator loses its fill; disabled draws the disabled title and
//     chevrons, no fill, and ignores the pointer.
//   - Focus: the focus ring around the bezel (FocusControl).
//   - Menu (S5.5k, openMenu): a primary press, Space (Activate), Up or
//     Down opens the menu over the selected item, and the button draws
//     pressed while it is open. A pick reports SelectionChanged (also for
//     the selected item); a cancel reports nothing. SetItems, SetSelected,
//     SetControlSize, disabling, hiding, Destroy and the window's close or
//     key loss close it without a report, and so does a pick after the
//     button left its window or was hidden or disabled through a parent.
//   - Accessibility: a pop-up button with the selected item as its value
//     and the show-menu action.
type PopUpButton struct {
	*stub.PopUpButton
	peerBase
	view *popUpView

	title layoutMemo // the selected item's layout

	// The menu (S5.5k): its element, made at the first opening and reused,
	// the open popup or nil, the index its items were opened over, and
	// the popup's top as asked, in the window's surface points.
	menu  *menu
	open  *popupRoot
	over  int
	wantY float64
}

// NewPopUpButton returns a pop-up button.
func (d *Driver) NewPopUpButton(h driver.PopUpHandler) driver.PopUpButton {
	p := &PopUpButton{PopUpButton: stub.NewPopUpButton(h)}
	p.view = &popUpView{d: d, p: p}
	p.view.Init(p.view)
	p.view.SetFocusable(true) // FocusControl: Tab reaches it with full keyboard access
	p.initElement(p.PopUpButton, &p.PopUpButton.Base, p.view)
	return p
}

// SetItems implements driver.PopUpButton; it closes an open menu.
func (p *PopUpButton) SetItems(items []string) {
	p.CloseMenu()
	p.PopUpButton.SetItems(items)
	p.redraw()
}

// SetSelected implements driver.PopUpButton; it closes an open menu.
func (p *PopUpButton) SetSelected(index int) {
	p.CloseMenu()
	p.PopUpButton.SetSelected(index)
	p.redraw()
}

// SetControlSize implements driver.Control; it closes an open menu.
func (p *PopUpButton) SetControlSize(cs driver.ControlSize) {
	p.CloseMenu()
	p.PopUpButton.SetControlSize(cs)
	p.redraw()
}

// SetEnabled implements driver.Widget; disabling closes an open menu.
func (p *PopUpButton) SetEnabled(enabled bool) {
	if !enabled {
		p.CloseMenu()
	}
	p.peerBase.SetEnabled(enabled)
}

// SetHidden implements driver.Widget; hiding closes an open menu.
func (p *PopUpButton) SetHidden(hidden bool) {
	if hidden {
		p.CloseMenu()
	}
	p.peerBase.SetHidden(hidden)
}

// Destroy implements driver.Widget; it closes an open menu first.
func (p *PopUpButton) Destroy() {
	p.CloseMenu()
	p.peerBase.Destroy()
}

// MenuOpen reports whether the menu is open.
func (p *PopUpButton) MenuOpen() bool { return p.open != nil }

// CloseMenu closes an open menu without a report.
func (p *PopUpButton) CloseMenu() {
	if p.open != nil {
		p.open.Close() // the menu loses its focus and ends; Closed clears p.open
	}
}

// UserSelect simulates a choice (stub.PopUpButton.UserSelect) and repaints.
func (p *PopUpButton) UserSelect(index int) { p.PopUpButton.UserSelect(index); p.redraw() }

// item returns the selected item, or "".
func (p *PopUpButton) item() string {
	if p.Index < 0 || p.Index >= len(p.Items) {
		return ""
	}
	return p.Items[p.Index]
}

// TitleFont returns the font of the title: the system font of the control
// size.
func (p *PopUpButton) TitleFont() driver.Font {
	return systemFont(p.ControlSize)
}

// titleLayout returns the selected item on one line, untruncated, or nil
// without one.
func (p *PopUpButton) titleLayout() render.TextLayout {
	return p.title.get(p.view.d.text, p.item(), p.TitleFont())
}

// popUpView is the element of a PopUpButton.
type popUpView struct {
	Base
	d     *Driver
	p     *PopUpButton
	press pressTracker
	ring  FocusRing
	trunc truncMemo // the title truncated to the room left
	chev  pathMemo[chevronKey]
}

// FittingSize is the stub's (see PopUpButton).
func (e *popUpView) FittingSize(max driver.Size) driver.Size { return e.p.FittingSize(max) }

// bezel returns the bezel rect in the element's coordinates and its corner
// radius, as the push button's.
func (e *popUpView) bezel() (driver.Rect, float64) {
	b := e.Bounds()
	m := tokens.Metrics.Control(e.p.ControlSize)
	h := min(m.Height, b.H)
	r := pixelAlign(&e.Base, driver.Rect{Y: (b.H - h) / 2, W: b.W, H: h}, scaleIn(&e.Base))
	return r, min(m.CornerRadius, r.W/2, r.H/2)
}

// indicator returns the indicator's rect inside bezel r (radius radius),
// its corner radius, and false when the bezel has no room for it. Its left
// edge is where the title's room ends (minus IndicatorGap) either way.
func (e *popUpView) indicator(r driver.Rect, radius float64) (driver.Rect, float64, bool) {
	pt := &tokens.PopUp
	inset, w := pt.IndicatorInset, pt.IndicatorWidth.At(e.p.ControlSize)
	ind := driver.Rect{X: r.X + r.W - inset - w, Y: r.Y + inset, W: w, H: r.H - 2*inset}
	if ind.H <= 0 || ind.X < r.X+inset {
		return ind, 0, false
	}
	ind = pixelAlign(&e.Base, ind, scaleIn(&e.Base))
	return ind, max(0, min(radius-inset, ind.W/2, ind.H/2)), true
}

// HandleEvent tracks the focus ring and the press; a primary press, Up
// and Down (without modifiers) open the menu. A disabled pop-up button
// drops a press and handles nothing.
func (e *popUpView) HandleEvent(ev Event) bool {
	e.ring.Track(e, ev)
	if !e.EnabledInTree() {
		e.press.Cancel(e)
		return false
	}
	switch ev := ev.(type) {
	case PointerEvent:
		handled, _ := e.press.Track(e, ev)
		if ev.Kind == PointerDown && ev.Button == shell.ButtonPrimary && e.press.Down() {
			// Opening cancels the press (Router.CancelPointer): its
			// release goes to the menu.
			e.openMenu(menuOpening{Press: true, Time: ev.Time, Pos: ev.Pos})
		}
		return handled
	case KeyEvent:
		if ev.Down && ev.Mods == 0 && (ev.Key == driver.KeyUp || ev.Key == driver.KeyDown) {
			if !ev.Repeat {
				e.openMenu(menuOpening{Time: ev.Time})
			}
			return true
		}
	}
	return false
}

// Activate implements Element: Space on the focused pop-up button opens
// the menu.
func (e *popUpView) Activate() bool { return e.openMenu(menuOpening{}) }

// State returns the pop-up button's control state, its press included:
// pressed while the menu is open.
func (e *popUpView) State() ControlState {
	s := e.press.State(e)
	if e.p.open != nil && !s.Disabled {
		s.Interaction = InteractionPressed
	}
	return s
}

// The menu's placement constraints: it slides to stay on the screen and
// gets shorter (and scrolls) when taller than the screen.
const popUpMenuAdjust = shell.AdjustSlideX | shell.AdjustSlideY | shell.AdjustResizeY

// openMenu opens the menu in a grab popup of the window (PLAN.md S5.5k)
// and begins its interaction as op says; it reports whether the menu is
// open. With op.Press, op.Pos is the press in the button's points. Nothing opens without items, outside a shown window, while the
// menu is open, or when the shell refuses the popup.
//
// The menu lists the items with a checkmark on the selected one, at the
// button's control size, at least as wide as the button (its panel's
// right edge not left of the bezel's), and opens over the selected item
// (the first without a selection), highlighted: its title on the
// button's title (both left edges, centred on the bezel). Where the shell
// draws no popup shadow (shell.Caps.PopupShadow) the menu draws its own, and the
// popup grows by its margin around the panel. The popup slides to stay
// on the screen; when it got shorter than its items (AdjustResizeY), the
// items scroll by what was cut off above, so they stay where they were,
// and then the least that shows the item it opened over.
func (e *popUpView) openMenu(op menuOpening) bool {
	p := e.p
	w := e.Window()
	if p.open != nil || len(p.Items) == 0 || w == nil || w.closed || !e.VisibleInTree() || !e.EnabledInTree() {
		return false
	}
	r, _ := e.bezel()
	if r.Empty() {
		return false
	}
	if p.menu == nil {
		p.menu = newMenu(e.d.text)
	}
	m := p.menu
	items := make([]menuItem, len(p.Items))
	for i, t := range p.Items {
		items[i] = menuItem{Title: t, Checked: i == p.Index}
	}
	m.SetControlSize(p.ControlSize)
	m.SetShadow(!e.d.sh.Caps().PopupShadow)
	m.SetItems(items)
	over := max(p.Index, 0)
	pad := tokens.PopUp.PaddingX.At(p.ControlSize)
	m.SetMinWidth(r.W - pad + titleX())
	size := m.FittingSize(driver.Size{W: math.Inf(1), H: math.Inf(1)})
	m.SetFrame(driver.Rect{W: size.W, H: size.H})
	m.SetScrollOffset(0)
	m.SetHighlighted(over)

	// The offset of the popup from the button's top-left: the item's title
	// on the button's.
	ir := m.ItemRect(over)
	off := driver.Point{
		X: r.X + pad - (m.Panel().X + titleX()),
		Y: r.Y + r.H/2 - (ir.Y + ir.H/2),
	}
	anchor := driver.RectOf(ToRoot(e, driver.Point{}), e.Bounds().Size())
	p.over, p.wantY = over, anchor.Y+off.Y
	m.SetActions(e.picked, e.cancelled)
	root, err := w.openPopup(popupOptions{
		Config: shell.PopupConfig{
			Anchor: anchor, Size: size, Offset: off,
			Adjust: popUpMenuAdjust, Grab: true,
		},
		Content: m,
		Placed:  e.placed,
		Closed:  e.closed,
	})
	if err != nil || root.Closed() {
		return false
	}
	p.open = root
	if op.Press {
		// The press, from the button's points to the menu's: the popup is
		// where it was placed, or (gtk4 places it later) where it was asked.
		o := anchor.Origin().Add(off)
		if pr := root.Rect(); !pr.Empty() {
			o = pr.Origin()
		}
		op.Pos, op.HasPos = ToRoot(e, op.Pos).Sub(o), true
	}
	m.Begin(op)
	e.Invalidate()
	return true
}

// placed scrolls the menu after a placement (see openMenu).
func (e *popUpView) placed(rect driver.Rect, _ float64) {
	m := e.p.menu
	if m.MaxScroll() == 0 {
		return
	}
	m.SetScrollOffset(rect.Y - e.p.wantY)
	m.ScrollToItem(e.p.over)
}

// closed is the popup's Closed hook.
func (e *popUpView) closed() {
	e.p.open = nil
	e.Invalidate()
}

// picked closes the menu and reports the pick of item i, unless the
// button can no longer take it (left its window, hidden or disabled).
func (e *popUpView) picked(i int) {
	p := e.p
	p.CloseMenu()
	w := e.Window()
	if w == nil || w.closed || !e.VisibleInTree() || !e.EnabledInTree() {
		return
	}
	p.UserSelect(i)
}

// cancelled closes the menu without a report.
func (e *popUpView) cancelled() { e.p.CloseMenu() }

// Paint draws the shadow, the bezel, its border, the title, the indicator
// with its chevrons and the focus ring (nothing for an empty frame).
func (e *popUpView) Paint(c render.Canvas) {
	r, radius := e.bezel()
	if e.Bounds().Empty() || r.Empty() {
		return
	}
	st := e.State()
	a := appearanceIn(&e.Base, e.d)
	m := tokens.Metrics.Control(e.p.ControlSize)
	pt := &tokens.PopUp
	radii := render.UniformRadii(radius)
	pressed := st.Interaction == InteractionPressed

	c.DrawShadow(r, radii, render.Shadow{
		Offset: driver.Point{Y: m.ShadowOffsetY}, Blur: m.ShadowBlur, Spread: m.ShadowSpread,
		Color: tokens.Metrics.BezelShadow.Value(a),
	})
	c.FillRoundedRect(r, radii, render.Solid(tokens.ButtonFill(a, false, pressed, st.Key)))
	strokeInside(c, r, radius, pixelWidth(1, scaleIn(&e.Base)), tokens.Controls.BezelBorder.Value(a))

	ind, indRadius, hasInd := e.indicator(r, radius)
	x := r.X + pt.PaddingX.At(e.p.ControlSize)
	if l := e.p.titleLayout(); l != nil {
		if avail := ind.X - pt.IndicatorGap - x; avail > 0 {
			if l.Size().W > avail {
				l = e.trunc.truncate(e.d.text, e.p.item(), e.p.TitleFont(), avail)
			}
			if l != nil && l.Size().H <= r.H {
				c.DrawText(l, driver.Point{X: x, Y: r.Y + (r.H-l.Size().H)/2}, tokens.PopUpText(a, st.Disabled))
			}
		}
	}

	if hasInd {
		if fill, ok := tokens.PopUpIndicatorFill(a, pressed, st.Key, st.Disabled); ok {
			c.FillRoundedRect(ind, render.UniformRadii(indRadius), render.Solid(fill))
			paintOutline(c, ind, indRadius, scaleIn(&e.Base), a, tokens.Controls.BezelOutline.Value(a))
		}
		w := pt.ChevronWidth.At(e.p.ControlSize)
		if chevronsFit(ind, w, pt.ChevronGap) {
			p := e.chev.get(chevronKey{ind, w, pt.ChevronGap}, func(p *render.Path) { chevrons(p, ind, w, pt.ChevronGap) })
			c.StrokePath(p, pt.ChevronStroke, render.Solid(tokens.PopUpChevron(a, st.Key, st.Disabled)),
				render.StrokeStyle{Cap: render.CapRound, Join: render.JoinRound})
		}
	}
	e.ring.Paint(c, e, RectFocusShape(r, radii))
}

// chevronKey is what a pop-up button's chevrons are built from.
type chevronKey struct {
	ind    driver.Rect
	w, gap float64
}

// chevronsFit reports whether chevrons w wide, gap apart, fit r.
func chevronsFit(r driver.Rect, w, gap float64) bool {
	return w > 0 && w <= r.W && w+gap <= r.H
}

// chevrons adds to p the indicator's two chevrons centred in r: one
// pointing up above one pointing down, each w wide and w/2 high, their
// open sides gap apart. They must fit r (chevronsFit).
func chevrons(p *render.Path, r driver.Rect, w, gap float64) {
	h := w / 2
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	top, bottom := cy-gap/2, cy+gap/2
	p.MoveTo(driver.Point{X: cx - w/2, Y: top}).LineTo(driver.Point{X: cx, Y: top - h}).LineTo(driver.Point{X: cx + w/2, Y: top})
	p.MoveTo(driver.Point{X: cx - w/2, Y: bottom}).LineTo(driver.Point{X: cx, Y: bottom + h}).LineTo(driver.Point{X: cx + w/2, Y: bottom})
}

// AccessibleNode implements Element: a pop-up button with the selected
// item as its value and the show-menu action.
func (e *popUpView) AccessibleNode() a11y.Node {
	return a11y.Node{Role: a11y.RolePopUpButton, Value: e.p.item(), Actions: a11y.ActionShowMenu}
}
