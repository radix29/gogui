package aqua

import (
	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/render"
)

// Checkbox is the aqua driver.Checkbox (PLAN.md S3.3f): a box drawn from
// the theme with its title after it. The embedded stub checkbox keeps the
// state (title, check state, control size) and the toggle rule
// (UserClick: off → on, on → off, mixed → on).
//
//   - Size: FittingSize is the box (CheckboxSize of the control size), then
//     CheckboxGap and the title's width, rounded up to whole points; as
//     high as the box or the title's line, whichever is higher. Without a
//     title it is the box alone. A stub Fixed size still wins.
//   - Box: at the leading edge, centred vertically and pixel-aligned,
//     CheckboxRadius rounded. Off: Theme.ToggleFill (BezelFill) and the
//     1 pt BezelBorder. On and mixed: the accent fill of ToggleFill
//     (AccentInactive, grey, in an inactive window) and, stroked in
//     AccentText (DefaultButtonText) CheckboxMarkStroke wide with round
//     caps and joins, a check (on) or a dash (mixed); no border.
//   - Title: one line in the system font of the control size, after the
//     box and the gap, centred vertically and truncated with an ellipsis
//     to the frame, in ColorControlText.
//   - States (ControlState): pressed while a primary press is over the
//     control (BezelPressedFill, or AccentPressed when on); disabled draws
//     the box at ToggleDisabledAlpha, the title in
//     ColorDisabledControlText, and ignores the pointer.
//   - Focus: the focus ring around the box (FocusControl).
//   - Clicks go through a pressTracker over the whole frame, box and
//     title alike: a release over it toggles. Activate (Space, the a11y
//     press) toggles too, refused while disabled.
//   - Accessibility: a checkbox with the title, checked or mixed, and the
//     press action.
type Checkbox struct {
	*stub.Checkbox
	peerBase
	view *checkBox

	title toggleTitle // the title layout
}

// NewCheckbox returns a checkbox.
func (d *Driver) NewCheckbox(h driver.ToggleHandler) driver.Checkbox {
	c := &Checkbox{Checkbox: stub.NewCheckbox(h)}
	c.view = &checkBox{d: d, p: c}
	c.view.Init(c.view)
	c.view.SetFocusable(true) // FocusControl: Tab reaches it with full keyboard access
	c.initElement(c.Checkbox, &c.Checkbox.Base, c.view)
	return c
}

func (c *Checkbox) SetTitle(title string)        { c.Checkbox.SetTitle(title); c.redraw() }
func (c *Checkbox) SetState(s driver.CheckState) { c.Checkbox.SetState(s); c.redraw() }
func (c *Checkbox) SetControlSize(cs driver.ControlSize) {
	c.Checkbox.SetControlSize(cs)
	c.redraw()
}

// UserClick toggles the checkbox (stub.Checkbox.UserClick) and repaints.
func (c *Checkbox) UserClick() { c.Checkbox.UserClick(); c.redraw() }

// TitleFont returns the font of the title: the system font size of the
// control size.
func (c *Checkbox) TitleFont() driver.Font { return toggleFont(c.ControlSize) }

// titleLayout returns the title on one line, untruncated, or nil for an
// empty title.
func (c *Checkbox) titleLayout() render.TextLayout {
	return c.title.get(c.view.d.sh.TextSystem(), c.Title, c.TitleFont())
}

// FittingSize implements driver.Widget: the box and the title, clamped to
// max; a stub Fixed size wins.
func (c *Checkbox) FittingSize(max driver.Size) driver.Size {
	if !c.Fixed.IsZero() {
		return c.Checkbox.FittingSize(max)
	}
	cs := c.ControlSize
	return toggleSize(tokens.Controls.CheckboxSize.At(cs), tokens.Controls.CheckboxGap.At(cs), c.titleLayout()).Min(max)
}

// checkBox is the element of a Checkbox.
type checkBox struct {
	Base
	d     *Driver
	p     *Checkbox
	press pressTracker
	ring  FocusRing
}

func (e *checkBox) FittingSize(max driver.Size) driver.Size { return e.p.FittingSize(max) }

// box returns the box rect in the element's coordinates and its corner
// radius: the box side (at most the frame's width and height) at the
// leading edge, centred vertically.
func (e *checkBox) box() (driver.Rect, float64) {
	b := e.Bounds()
	cs := e.p.ControlSize
	s := min(tokens.Controls.CheckboxSize.At(cs), b.W, b.H)
	r := pixelAlign(&e.Base, driver.Rect{Y: (b.H - s) / 2, W: s, H: s}, scaleIn(&e.Base))
	return r, min(tokens.Controls.CheckboxRadius.At(cs), r.W/2, r.H/2)
}

// HandleEvent tracks the focus ring and the press; a release over the
// checkbox toggles it. A disabled checkbox drops a press and handles
// nothing.
func (e *checkBox) HandleEvent(ev Event) bool {
	e.ring.Track(e, ev)
	if !e.EnabledInTree() {
		e.press.Reset(e)
		return false
	}
	handled, act := e.press.Track(e, ev)
	if act {
		e.p.UserClick()
	}
	return handled
}

// Activate toggles the checkbox, refused while it is disabled.
func (e *checkBox) Activate() bool {
	if !e.EnabledInTree() {
		return false
	}
	e.p.UserClick()
	return true
}

// State returns the checkbox's control state, its press included.
func (e *checkBox) State() ControlState { return e.press.State(e) }

// Paint draws the box with its border or mark, the title and the focus
// ring around the box (nothing for an empty frame).
func (e *checkBox) Paint(c render.Canvas) {
	b := e.Bounds()
	r, radius := e.box()
	if b.Empty() || r.Empty() {
		return
	}
	st := e.State()
	a := appearanceIn(&e.Base, e.d)
	cs := e.p.ControlSize
	radii := render.UniformRadii(radius)
	on := e.p.Value != driver.StateOff

	if st.Disabled {
		c.PushOpacity(tokens.Controls.ToggleDisabledAlpha)
	}
	c.FillRoundedRect(r, radii, render.Solid(tokens.ToggleFill(a, on, st.Interaction == InteractionPressed, st.Key)))
	if on {
		c.StrokePath(checkMark(r, e.p.Value == driver.StateMixed), tokens.Controls.CheckboxMarkStroke.At(cs),
			render.Solid(tokens.AccentText(a)), render.StrokeStyle{Cap: render.CapRound, Join: render.JoinRound})
	} else if bw := pixelWidth(1, scaleIn(&e.Base)); r.W > 2*bw && r.H > 2*bw {
		c.StrokeRoundedRect(r.Inset(driver.UniformInsets(bw/2)), render.UniformRadii(max(0, radius-bw/2)), bw,
			render.Solid(tokens.Controls.BezelBorder.Value(a)))
	}
	if st.Disabled {
		c.Pop()
	}

	paintToggleTitle(c, e.d.sh.TextSystem(), e.p.titleLayout(), e.p.Title, e.p.TitleFont(),
		tokens.Controls.CheckboxSize.At(cs)+tokens.Controls.CheckboxGap.At(cs), b, st.Disabled, a)
	e.ring.Paint(c, e, RectFocusShape(r, radii))
}

// checkMark returns the mark of a checked box r: a check, or for mixed a
// horizontal dash. The points are on a 14-unit grid of the box.
func checkMark(r driver.Rect, mixed bool) *render.Path {
	u := r.W / 14
	at := func(x, y float64) driver.Point { return driver.Point{X: r.X + x*u, Y: r.Y + y*u} }
	p := &render.Path{}
	if mixed {
		return p.MoveTo(at(3.5, 7)).LineTo(at(10.5, 7))
	}
	return p.MoveTo(at(3.5, 7.5)).LineTo(at(6, 10)).LineTo(at(10.5, 4))
}

// AccessibleNode implements Element: a checkbox with the title, checked
// or mixed.
func (e *checkBox) AccessibleNode() a11y.Node {
	return a11y.Node{Role: a11y.RoleCheckBox, Label: e.p.Title,
		States: toggleStates(e.p.Value), Actions: a11y.ActionPress}
}
