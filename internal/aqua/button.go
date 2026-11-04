package aqua

import (
	"math"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/render"
)

// Button is the aqua driver.Button (PLAN.md S3.3e): a push button drawn
// from the theme. The embedded stub button keeps the state (title,
// default and cancel flags, control size) and the User… simulations.
//
//   - Size: FittingSize is the bezel of the control size
//     (Metrics.Control): ControlHeight high, the title's width plus
//     ControlPaddingX on each side wide (rounded up to whole points), at
//     least ButtonMinWidth. A stub Fixed size still wins.
//   - Bezel: the frame's width, ControlHeight high (at most the frame's
//     height), centred vertically and pixel-aligned; its shadow
//     (BezelShadow*), its fill (Theme.ButtonFill) and, unless it is filled
//     with the accent, its 1 pt BezelBorder (whole device pixels).
//   - Title: one line in the system font of the control size with
//     ButtonTitleWeight, centred in the bezel and truncated with an
//     ellipsis to the bezel minus its padding, in Theme.ButtonText.
//   - States (ControlState): pressed while a primary press is over it
//     (BezelPressedFill, or AccentPressed on a default button); a default
//     button is accent-filled with DefaultButtonText in a key window and
//     drawn as a plain one in an inactive window or while disabled;
//     disabled draws the disabled text color and ignores the pointer.
//   - Focus: the focus ring around the bezel (FocusControl).
//   - Clicks go through a pressTracker: a release over the button clicks
//     it. Activate (Return, Escape, Space, the a11y press) is a click too,
//     refused while disabled.
//   - Accessibility: a button with the title and the press action.
type Button struct {
	*stub.Button
	peerBase
	view *pushButton

	// The title layout and what it was made from.
	title    render.TextLayout
	titleKey buttonTitleKey
}

type buttonTitleKey struct {
	text string
	font driver.Font
}

// NewButton returns a push button.
func (d *Driver) NewButton(h driver.ButtonHandler) driver.Button {
	b := &Button{Button: stub.NewButton(h)}
	b.view = &pushButton{d: d, p: b}
	b.view.Init(b.view)
	b.view.SetFocusable(true) // FocusControl: Tab reaches it with full keyboard access
	b.initElement(b.Button, &b.Button.Base, b.view)
	return b
}

func (b *Button) SetTitle(title string) { b.Button.SetTitle(title); b.redraw() }

func (b *Button) SetControlSize(cs driver.ControlSize) {
	b.Button.SetControlSize(cs)
	b.redraw()
}

// SetDefault and SetCancel record the flag in the stub and mark the
// element (Base.SetDefaultButton, SetCancelButton), so Return or Escape
// presses it in its window; a default button is drawn with the accent.
func (b *Button) SetDefault(isDefault bool) {
	b.Button.SetDefault(isDefault)
	b.view.SetDefaultButton(isDefault)
	b.redraw()
}

func (b *Button) SetCancel(isCancel bool) {
	b.Button.SetCancel(isCancel)
	b.view.SetCancelButton(isCancel)
}

// TitleFont returns the font of the title: the system font size of the
// control size with ButtonTitleWeight.
func (b *Button) TitleFont() driver.Font {
	f := driver.Font{Size: tokens.Typography.FontSize.At(b.ControlSize), Weight: tokens.Controls.ButtonTitleWeight}
	if f.Weight == driver.FontWeightRegular {
		f.Weight = 0 // the same font; keeps the dumps short
	}
	return f
}

// titleLayout returns the title on one line, untruncated, or nil for an
// empty title.
func (b *Button) titleLayout() render.TextLayout {
	if b.Title == "" {
		return nil
	}
	k := buttonTitleKey{text: b.Title, font: b.TitleFont()}
	if b.title == nil || b.titleKey != k {
		b.title = b.view.d.sh.TextSystem().NewLayout(oneLine(k.text), k.font, 0)
		b.titleKey = k
	}
	return b.title
}

// FittingSize implements driver.Widget: the bezel size, clamped to max; a
// stub Fixed size wins.
func (b *Button) FittingSize(max driver.Size) driver.Size {
	if !b.Fixed.IsZero() {
		return b.Button.FittingSize(max)
	}
	m := tokens.Metrics.Control(b.ControlSize)
	w := 2 * m.PaddingX
	if l := b.titleLayout(); l != nil {
		w = math.Ceil(l.Size().W + 2*m.PaddingX)
	}
	w = math.Max(w, tokens.Controls.ButtonMinWidth.At(b.ControlSize))
	return driver.Size{W: w, H: m.Height}.Min(max)
}

// pushButton is the element of a Button.
type pushButton struct {
	Base
	d     *Driver
	p     *Button
	press pressTracker
	ring  FocusRing
}

func (e *pushButton) FittingSize(max driver.Size) driver.Size { return e.p.FittingSize(max) }

// bezel returns the bezel rect in the element's coordinates and its
// corner radius.
func (e *pushButton) bezel() (driver.Rect, float64) {
	b := e.Bounds()
	m := tokens.Metrics.Control(e.p.ControlSize)
	h := min(m.Height, b.H)
	r := pixelAlign(&e.Base, driver.Rect{Y: (b.H - h) / 2, W: b.W, H: h}, scaleIn(&e.Base))
	return r, min(m.CornerRadius, r.W/2, r.H/2)
}

// HandleEvent tracks the focus ring and the press; a release over the
// button clicks it. A disabled button drops a press and handles nothing.
func (e *pushButton) HandleEvent(ev Event) bool {
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

// Activate clicks the button, refused while it is disabled.
func (e *pushButton) Activate() bool {
	if !e.EnabledInTree() {
		return false
	}
	e.p.UserClick()
	return true
}

// State returns the button's control state, its press included.
func (e *pushButton) State() ControlState { return e.press.State(e) }

// Paint draws the shadow, the bezel, its border, the title and the focus
// ring (nothing for an empty frame).
func (e *pushButton) Paint(c render.Canvas) {
	r, radius := e.bezel()
	if e.Bounds().Empty() || r.Empty() {
		return
	}
	st := e.State()
	a := appearanceIn(&e.Base, e.d)
	m := tokens.Metrics.Control(e.p.ControlSize)
	radii := render.UniformRadii(radius)
	isDefault := e.p.IsDefault && !st.Disabled
	pressed := st.Interaction == InteractionPressed
	accent := isDefault && st.Key

	c.DrawShadow(r, radii, render.Shadow{
		Offset: driver.Point{Y: m.ShadowOffsetY}, Blur: m.ShadowBlur, Spread: m.ShadowSpread,
		Color: tokens.Metrics.BezelShadow.Value(a),
	})
	c.FillRoundedRect(r, radii, render.Solid(tokens.ButtonFill(a, isDefault, pressed, st.Key)))
	if !accent {
		bw := pixelWidth(1, scaleIn(&e.Base))
		if r.W > 2*bw && r.H > 2*bw {
			c.StrokeRoundedRect(r.Inset(driver.UniformInsets(bw/2)), render.UniformRadii(max(0, radius-bw/2)), bw,
				render.Solid(tokens.Controls.BezelBorder.Value(a)))
		}
	}
	if e.p.Title != "" {
		if avail := r.W - 2*m.PaddingX; avail > 0 {
			l := e.p.titleLayout()
			if l.Size().W > avail {
				l = truncate(e.d.sh.TextSystem(), e.p.Title, e.p.TitleFont(), avail)
			}
			if l != nil && l.Size().H <= r.H {
				s := l.Size()
				col := tokens.ButtonText(a, isDefault, st.Key, st.Disabled)
				c.DrawText(l, driver.Point{X: r.X + (r.W-s.W)/2, Y: r.Y + (r.H-s.H)/2}, col)
			}
		}
	}
	e.ring.Paint(c, e, RectFocusShape(r, radii))
}

// AccessibleNode implements Element: a button with the title.
func (e *pushButton) AccessibleNode() a11y.Node {
	return a11y.Node{Role: a11y.RoleButton, Label: e.p.Title, Actions: a11y.ActionPress}
}
