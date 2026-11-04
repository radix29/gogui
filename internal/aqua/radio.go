package aqua

import (
	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/render"
)

// Radio is the aqua driver.Radio (PLAN.md S3.3g): a circle drawn from the
// theme with its title after it. The embedded stub radio keeps the state
// (title, on, control size) and the click rule (UserClick turns it on and
// reports StateOn, even when it was on already, like AppKit).
// Exclusivity is not the radio's: the public RadioGroup turns the other
// radios of its group off through SetOn when one reports StateOn.
//
//   - Size: FittingSize is the circle (RadioSize of the control size),
//     then RadioGap and the title's width, rounded up to whole points; as
//     high as the circle or the title's line, whichever is higher. Without
//     a title it is the circle alone. A stub Fixed size still wins.
//   - Circle: at the leading edge, centred vertically and pixel-aligned.
//     Off: Theme.ToggleFill (BezelFill) and the 1 pt BezelBorder. On: the
//     accent fill of ToggleFill (AccentInactive, grey, in an inactive
//     window) and, centred in it, a dot of RadioDotSize (scaled with the
//     circle) in AccentText (DefaultButtonText); no border.
//   - Title: as the Checkbox's: one line in the system font of the control
//     size, after the circle and the gap, centred vertically and truncated
//     to the frame, in ColorControlText.
//   - States (ControlState): pressed while a primary press is over the
//     control (BezelPressedFill, or AccentPressed when on); disabled draws
//     the circle at ToggleDisabledAlpha, the title in
//     ColorDisabledControlText, and ignores the pointer.
//   - Focus: the focus ring around the circle (FocusControl). The arrow
//     keys do not move the selection inside a group (not in v0.1).
//   - Clicks go through a pressTracker over the whole frame, circle and
//     title alike: a release over it calls UserClick. Activate (Space, the
//     a11y press) does the same, refused while disabled.
//   - Accessibility: a radio button with the title, checked when on, and
//     the press action.
type Radio struct {
	*stub.Radio
	peerBase
	view  *radioButton
	title toggleTitle // the title layout
}

// NewRadio returns a radio button.
func (d *Driver) NewRadio(h driver.ToggleHandler) driver.Radio {
	r := &Radio{Radio: stub.NewRadio(h)}
	r.view = &radioButton{d: d, p: r}
	r.view.Init(r.view)
	r.view.SetFocusable(true) // FocusControl: Tab reaches it with full keyboard access
	r.initElement(r.Radio, &r.Radio.Base, r.view)
	return r
}

func (r *Radio) SetTitle(title string) { r.Radio.SetTitle(title); r.redraw() }
func (r *Radio) SetOn(on bool)         { r.Radio.SetOn(on); r.redraw() }
func (r *Radio) SetControlSize(cs driver.ControlSize) {
	r.Radio.SetControlSize(cs)
	r.redraw()
}

// UserClick turns the radio on (stub.Radio.UserClick, which reports
// StateOn to the handler) and repaints.
func (r *Radio) UserClick() { r.Radio.UserClick(); r.redraw() }

// TitleFont returns the font of the title: the system font size of the
// control size.
func (r *Radio) TitleFont() driver.Font { return toggleFont(r.ControlSize) }

// titleLayout returns the title on one line, untruncated, or nil for an
// empty title.
func (r *Radio) titleLayout() render.TextLayout {
	return r.title.get(r.view.d.sh.TextSystem(), r.Title, r.TitleFont())
}

// FittingSize implements driver.Widget: the circle and the title, clamped
// to max; a stub Fixed size wins.
func (r *Radio) FittingSize(max driver.Size) driver.Size {
	if !r.Fixed.IsZero() {
		return r.Radio.FittingSize(max)
	}
	cs := r.ControlSize
	return toggleSize(tokens.Controls.RadioSize.At(cs), tokens.Controls.RadioGap.At(cs), r.titleLayout()).Min(max)
}

// radioButton is the element of a Radio.
type radioButton struct {
	Base
	d     *Driver
	p     *Radio
	press pressTracker
	ring  FocusRing
}

func (e *radioButton) FittingSize(max driver.Size) driver.Size { return e.p.FittingSize(max) }

// circle returns the circle's square in the element's coordinates: the
// diameter (at most the frame's width and height) at the leading edge,
// centred vertically.
func (e *radioButton) circle() driver.Rect {
	b := e.Bounds()
	s := min(tokens.Controls.RadioSize.At(e.p.ControlSize), b.W, b.H)
	return pixelAlign(&e.Base, driver.Rect{Y: (b.H - s) / 2, W: s, H: s}, scaleIn(&e.Base))
}

// HandleEvent tracks the focus ring and the press; a release over the
// radio turns it on. A disabled radio drops a press and handles nothing.
func (e *radioButton) HandleEvent(ev Event) bool {
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

// Activate turns the radio on, refused while it is disabled.
func (e *radioButton) Activate() bool {
	if !e.EnabledInTree() {
		return false
	}
	e.p.UserClick()
	return true
}

// State returns the radio's control state, its press included.
func (e *radioButton) State() ControlState { return e.press.State(e) }

// Paint draws the circle with its border or dot, the title and the focus
// ring around the circle (nothing for an empty frame).
func (e *radioButton) Paint(c render.Canvas) {
	b := e.Bounds()
	r := e.circle()
	if b.Empty() || r.Empty() {
		return
	}
	st := e.State()
	a := appearanceIn(&e.Base, e.d)
	cs := e.p.ControlSize
	round := render.UniformRadii(min(r.W, r.H) / 2)
	on := e.p.IsOn

	if st.Disabled {
		c.PushOpacity(tokens.Controls.ToggleDisabledAlpha)
	}
	c.FillRoundedRect(r, round, render.Solid(tokens.ToggleFill(a, on, st.Interaction == InteractionPressed, st.Key)))
	if on {
		d := tokens.Controls.RadioDotSize.At(cs) * r.W / tokens.Controls.RadioSize.At(cs)
		dot := driver.Rect{X: r.X + (r.W-d)/2, Y: r.Y + (r.H-d)/2, W: d, H: d}
		c.FillRoundedRect(dot, render.UniformRadii(d/2), render.Solid(tokens.AccentText(a)))
	} else if bw := pixelWidth(1, scaleIn(&e.Base)); r.W > 2*bw && r.H > 2*bw {
		in := r.Inset(driver.UniformInsets(bw / 2))
		c.StrokeRoundedRect(in, render.UniformRadii(min(in.W, in.H)/2), bw,
			render.Solid(tokens.Controls.BezelBorder.Value(a)))
	}
	if st.Disabled {
		c.Pop()
	}

	paintToggleTitle(c, e.d.sh.TextSystem(), e.p.titleLayout(), e.p.Title, e.p.TitleFont(),
		tokens.Controls.RadioSize.At(cs)+tokens.Controls.RadioGap.At(cs), b, st.Disabled, a)
	e.ring.Paint(c, e, RectFocusShape(r, round))
}

// AccessibleNode implements Element: a radio button with the title,
// checked when on.
func (e *radioButton) AccessibleNode() a11y.Node {
	var s a11y.States
	if e.p.IsOn {
		s = a11y.StateChecked
	}
	return a11y.Node{Role: a11y.RoleRadioButton, Label: e.p.Title, States: s, Actions: a11y.ActionPress}
}
