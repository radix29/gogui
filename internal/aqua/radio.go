package aqua

import (
	"slices"

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
//     circle) in Theme.ToggleMark (AccentText; DefaultButtonText when
//     inactive); no border, only the
//     BezelOutline with Increase Contrast (S6.1e).
//   - Title: as the Checkbox's: one line in the system font of the control
//     size, after the circle and the gap, centred vertically and truncated
//     to the frame, in ColorControlText.
//   - States (ControlState): pressed while a primary press is over the
//     control (BezelPressedFill, or AccentPressed when on); disabled draws
//     the circle at ToggleDisabledAlpha, the title in
//     ColorDisabledControlText, and ignores the pointer.
//   - Focus: the focus ring around the circle (FocusControl). The group
//     (the radios of one parent, a RadioGroup's container) is one Tab
//     stop, its selected radio or, with none selected, its first usable
//     one (as AppKit's radio groups). The arrow keys (no modifiers) move
//     inside the group (S5.7h): Down and Right to the next usable radio,
//     Up and Left to the previous one, without wrapping; the radio moved
//     to is focused and clicked, so the group selects it and reports it.
//   - Clicks go through the clickControl core over the whole frame,
//     circle and title alike: a release over it calls UserClick. Activate
//     (Space, the a11y press) does the same, refused while disabled; Space
//     flashes the circle pressed for keyFlash first.
//   - Accessibility: a radio button with the title, checked when on, and
//     the press action.
type Radio struct {
	*stub.Radio
	peerBase
	view  *radioButton
	title layoutMemo // the title layout
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
	return r.title.get(r.view.d.text, r.Title, r.TitleFont())
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
	clickControl
	d     *Driver
	p     *Radio
	trunc truncMemo // the title truncated to the width left
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
// radio turns it on, and the arrow keys move in the group. A disabled
// radio drops a press and handles nothing.
func (e *radioButton) HandleEvent(ev Event) bool { return e.handleClick(e, ev, e.arrow) }

// Activate turns the radio on, refused while it is disabled.
func (e *radioButton) Activate() bool { return e.activate(e) }

func (e *radioButton) click() { e.p.UserClick() }

// State returns the radio's control state, its press and a key flash
// included.
func (e *radioButton) State() ControlState { return e.state(e) }

// keyFlash shows a press by key (clickControl.flash).
func (e *radioButton) keyFlash() { e.flash(e, e.d) }

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
		c.PushOpacity(float64(tokens.Controls.ToggleDisabledAlpha))
	}
	c.FillRoundedRect(r, round, render.Solid(tokens.ToggleFill(a, on, st.Interaction == InteractionPressed, st.Key)))
	if on {
		paintOutline(c, r, min(r.W, r.H)/2, scaleIn(&e.Base), a, tokens.Controls.BezelOutline.Value(a))
		d := tokens.Controls.RadioDotSize.At(cs) * r.W / tokens.Controls.RadioSize.At(cs)
		dot := driver.Rect{X: r.X + (r.W-d)/2, Y: r.Y + (r.H-d)/2, W: d, H: d}
		c.FillRoundedRect(dot, render.UniformRadii(d/2), render.Solid(tokens.ToggleMark(a, st.Key)))
	} else if bw := pixelWidth(1, scaleIn(&e.Base)); r.W > 2*bw && r.H > 2*bw {
		in := r.Inset(driver.UniformInsets(bw / 2))
		c.StrokeRoundedRect(in, render.UniformRadii(min(in.W, in.H)/2), bw,
			render.Solid(tokens.Controls.BezelBorder.Value(a)))
	}
	if st.Disabled {
		c.Pop()
	}

	paintToggleTitle(c, &e.trunc, e.d.text, e.p.titleLayout(), e.p.Title, e.p.TitleFont(),
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

// group returns the radios of the radio's group, in tree order: the
// radio buttons among its parent's children (a RadioGroup's container
// holds only its radios), or the radio alone without a parent.
func (e *radioButton) group() []*radioButton {
	p := e.Parent()
	if p == nil {
		return []*radioButton{e}
	}
	var g []*radioButton
	for _, c := range p.base().children {
		if r, ok := c.(*radioButton); ok {
			g = append(g, r)
		}
	}
	return g
}

// usable reports whether the radio is visible and enabled in its tree.
func (e *radioButton) usable() bool { return e.VisibleInTree() && e.EnabledInTree() }

// groupStop implements groupTabStop: a group is one Tab stop, its
// selected radio, or its first usable one when none is selected; nil
// when no radio of the group is usable.
func (e *radioButton) groupStop() Element {
	var first *radioButton
	visit := func(r *radioButton) bool {
		if !r.usable() {
			return false
		}
		if r.p.IsOn {
			first = r
			return true
		}
		if first == nil {
			first = r
		}
		return false
	}
	if p := e.Parent(); p == nil {
		visit(e)
	} else {
		for _, c := range p.base().children {
			if r, ok := c.(*radioButton); ok && visit(r) {
				break
			}
		}
	}
	if first == nil {
		return nil
	}
	return first
}

// arrow moves the selection inside the group by an arrow key: Down and
// Right to the next usable radio, Up and Left to the previous one, without
// wrapping. The radio moved to is focused and clicked (it reports StateOn,
// so the group selects it). It reports whether k is such a key; at either
// end the key is consumed and nothing moves.
func (e *radioButton) arrow(k KeyEvent) bool {
	if !k.Down || k.Mods != 0 {
		return false
	}
	step := 0
	switch k.Key {
	case driver.KeyDown, driver.KeyRight:
		step = 1
	case driver.KeyUp, driver.KeyLeft:
		step = -1
	default:
		return false
	}
	g := e.group()
	for i := slices.Index(g, e) + step; i >= 0 && i < len(g); i += step {
		if r := g[i]; r.usable() {
			if rt := r.routedBy(); rt != nil {
				rt.SetFocus(r)
			}
			r.p.UserClick()
			break
		}
	}
	return true
}
