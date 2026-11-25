package aqua

import (
	"math"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
)

// Tooltips (PLAN.md S5.5l). Every element has a tooltip text
// (Base.SetTooltip; the peers and the Container pass driver.Widget's
// SetTooltip on to their element). A window's tooltips controller shows
// the text of the element under the pointer in a non-grab popup (it takes
// no input) below the pointer, as AppKit's tool-tip manager does:
//
//   - The tipped element is the deepest element under the pointer (HitTest
//     over the window's tree) with a tooltip, or its nearest ancestor with
//     one; a disabled element shows its tooltip too, as in AppKit.
//   - Delay: the tip shows once the pointer has rested on the tipped
//     element for Tooltip.Delay; each move over it while the tip is not
//     shown restarts the wait. A shown tip stays where it opened while the
//     pointer moves over its element.
//   - Grace: moving off a shown tip's element hides the tip; for
//     Tooltip.Grace after that the tip of the next tipped element shows at
//     once (and the grace starts anew when that one hides).
//   - Hidden on a press, a key-down, a text or preedit event and a scroll,
//     which also mute the element: its tip does not come back until the
//     pointer has left it. A drag (a move with a button held) hides the
//     tip and arms nothing. The pointer leaving the window, the window
//     losing the key state or closing, and a grab popup (a menu) opening
//     hide it and end the grace.
//   - Only a key window shows tooltips (a window's popups close when it
//     stops being key), never while a grab popup is open.
//   - Place: the panel's top-left corner Tooltip.OffsetX/OffsetY from the
//     pointer's position when the tip shows, sliding on both axes to stay
//     on the screen (AdjustSlideX|AdjustSlideY).
//   - Changes: a new text for the shown tip's element updates the tip in
//     place (empty: hides it); a tip whose element leaves the tree or is
//     hidden, or loses its text, hides at once, and a pending wait for such
//     an element is dropped.
//
// The tip is a tooltipView: Tooltip.Background with its Border, the text
// in the system font of Tooltip.FontSize wrapped to Tooltip.MaxWidth, and
// where the shell draws no popup shadow (shell.Caps.PopupShadow) its own shadow
// around the panel, which grows the popup by its margin.

// SetTooltip sets the element's tooltip text; an empty text removes it. A
// change reaches the tip shown for the element at once (see tooltips).
func (b *Base) SetTooltip(text string) {
	if text == b.tooltip {
		return
	}
	b.tooltip = text
	if w := b.Window(); w != nil && b.Root() == Element(w.root) {
		w.tips.changed()
	}
}

// Tooltip returns the element's tooltip text.
func (b *Base) Tooltip() string { return b.tooltip }

// SetTooltip implements driver.Widget: the stub's text and the element's.
func (p *peerBase) SetTooltip(text string) {
	p.sb.SetTooltip(text)
	p.elem.base().SetTooltip(text)
}

// tooltips is a window's tooltip controller (see above). The zero value
// with w set is ready.
type tooltips struct {
	w *Window

	pos    driver.Point // the pointer, in the surface's points
	inside bool         // the pointer is over the surface

	target Element // the tipped element under the pointer, or nil
	muted  Element // the target a press, key or scroll hid, or nil
	wait   func()  // cancels the pending Delay, or nil

	warm  bool   // within the Grace (or a tip is shown)
	grace func() // cancels the Grace timer, or nil

	open   *popupRoot   // the shown tip, or nil
	shown  Element      // the element the tip shows for
	anchor driver.Point // where it opened
	view   *tooltipView // made at the first tip, reused
}

// event follows one shell event of the window, before it is routed.
func (t *tooltips) event(ev shell.Event) {
	switch ev := ev.(type) {
	case shell.PointerEvent:
		t.pos = ev.Pos
		switch ev.Kind {
		case shell.PointerEnter, shell.PointerMove:
			t.inside = true
			if ev.Buttons != 0 {
				t.cancelWait()
				t.hide(false)
				return
			}
			t.track(true)
		case shell.PointerDown:
			t.inside = true
			t.mute()
		case shell.PointerLeave:
			t.inside = false
			t.reset()
		}
	case shell.ScrollEvent:
		t.mute()
	case shell.KeyEvent:
		if ev.Down {
			t.mute()
		}
	case shell.TextEvent, shell.PreeditEvent:
		t.mute()
	}
}

// tipAt returns the tipped element at p (see tooltips), or nil.
func (t *tooltips) tipAt(p driver.Point) Element {
	if !t.inside {
		return nil
	}
	hit, _ := HitTest(t.w.root, p)
	for e := hit; e != nil; e = parentOf(e) {
		if e.base().tooltip != "" {
			return e
		}
	}
	return nil
}

// track follows the pointer at t.pos: a new target hides the shown tip
// and waits for its own (or shows it at once within the grace); a move
// over the same, unshown target restarts the wait when moved is set.
func (t *tooltips) track(moved bool) {
	tip := t.tipAt(t.pos)
	if tip != t.target {
		t.cancelWait()
		t.hide(true)
		t.target, t.muted = tip, nil
		switch {
		case tip == nil:
		case t.warm:
			t.show()
		default:
			t.arm()
		}
		return
	}
	if moved && tip != nil && t.open == nil && t.muted != tip {
		t.arm()
	}
}

// mute hides the tip and keeps the target's from coming back until the
// pointer leaves it (a press, a key, a scroll).
func (t *tooltips) mute() {
	t.cancelWait()
	t.hide(false)
	t.muted = t.target
}

// reset hides the tip and forgets the target (the pointer left, the
// window stopped being key or closed, a grab popup opens).
func (t *tooltips) reset() {
	t.cancelWait()
	t.hide(false)
	t.target, t.muted = nil, nil
}

// arm (re)starts the Delay for the target.
func (t *tooltips) arm() {
	t.cancelWait()
	w := t.w
	if w.closed {
		return
	}
	t.wait = w.sh.After(tokens.Tooltip.Delay, func() {
		t.wait = nil
		t.show()
	})
}

func (t *tooltips) cancelWait() {
	if t.wait != nil {
		t.wait()
		t.wait = nil
	}
}

// cool ends the grace.
func (t *tooltips) cool() {
	if t.grace != nil {
		t.grace()
		t.grace = nil
	}
	t.warm = false
}

// live reports whether e can show its tip: in the window's tree, visible,
// with a text.
func (t *tooltips) live(e Element) bool {
	b := e.base()
	return b.Root() == Element(t.w.root) && b.VisibleInTree() && b.tooltip != ""
}

// show opens the target's tip at the pointer, unless the window cannot
// show one now.
func (t *tooltips) show() {
	w := t.w
	e := t.target
	if t.open != nil || e == nil || !t.inside || w.closed || !w.key || w.grab != nil || w.grabOpening || !t.live(e) {
		return
	}
	if t.view == nil {
		t.view = newTooltipView(w.text)
	}
	v := t.view
	v.SetShadow(!w.sh.Caps().PopupShadow)
	v.SetText(e.base().tooltip)
	t.anchor = t.pos
	root, err := w.openPopup(popupOptions{
		Config:  t.config(),
		Content: v,
		Closed:  t.closed,
	})
	if err != nil || root.Closed() {
		return
	}
	t.open, t.shown = root, e
	if t.grace != nil {
		t.grace()
		t.grace = nil
	}
	t.warm = true
}

// config returns the placement of the view's tip at the anchor.
func (t *tooltips) config() shell.PopupConfig {
	v := t.view
	in := v.ShadowMargin()
	tt := &tokens.Tooltip
	return shell.PopupConfig{
		Anchor: driver.Rect{X: t.anchor.X, Y: t.anchor.Y},
		Size:   v.FittingSize(driver.Size{W: math.Inf(1), H: math.Inf(1)}),
		Offset: driver.Point{X: tt.OffsetX - in.Left, Y: tt.OffsetY - in.Top},
		Adjust: shell.AdjustSlideX | shell.AdjustSlideY,
	}
}

// closed is the tip popup's Closed hook (any close, the window's included).
func (t *tooltips) closed() {
	if t.open != nil && t.open.Closed() {
		t.open, t.shown = nil, nil
	}
}

// hide closes the shown tip; with graceful the grace starts (moving off),
// otherwise it ends.
func (t *tooltips) hide(graceful bool) {
	if t.open == nil {
		if !graceful {
			t.cool()
		}
		return
	}
	o := t.open
	t.open, t.shown = nil, nil
	o.Close()
	t.cool()
	if graceful && !t.w.closed {
		t.warm = true
		t.grace = t.w.sh.After(tokens.Tooltip.Grace, func() {
			t.grace = nil
			t.warm = false
		})
	}
}

// changed follows a tooltip text change in the window's tree: the shown
// tip takes its element's new text, and the target is found again.
func (t *tooltips) changed() {
	if t.open != nil && t.live(t.shown) && t.tipAt(t.pos) == t.shown {
		if text := t.shown.base().tooltip; text != t.view.Text() {
			t.view.SetText(text)
			t.open.Reposition(t.config())
		}
		return
	}
	t.track(false)
}

// validate follows a tree change of the window (Router.SetTreeChangedFunc):
// a tip or a wait for an element that can no longer show it ends.
func (t *tooltips) validate() {
	if t.open != nil && !t.live(t.shown) {
		t.hide(false)
		t.target, t.muted = nil, nil
	}
	if t.wait != nil && (t.target == nil || !t.live(t.target)) {
		t.cancelWait()
		t.target, t.muted = nil, nil
	}
}

// tooltipView is the content of a tooltip popup (see tooltips).
type tooltipView struct {
	Base
	ts     render.TextSystem
	text   string
	layout render.TextLayout // nil for an empty text
	shadow bool
}

func newTooltipView(ts render.TextSystem) *tooltipView {
	v := &tooltipView{ts: ts}
	v.Init(v)
	return v
}

// Font returns the tooltip font: the system font of Tooltip.FontSize.
func (v *tooltipView) Font() driver.Font { return driver.Font{Size: tokens.Tooltip.FontSize} }

// Text returns the text shown.
func (v *tooltipView) Text() string { return v.text }

// SetText sets the text, wrapped to Tooltip.MaxWidth less the padding.
func (v *tooltipView) SetText(text string) {
	if text == v.text && (v.layout != nil || text == "") {
		return
	}
	v.text, v.layout = text, nil
	if text != "" {
		v.layout = v.ts.NewLayout(text, v.Font(), max(1, tokens.Tooltip.MaxWidth-2*tokens.Tooltip.PaddingX))
	}
	v.Invalidate()
}

// SetShadow sets whether the view draws its own shadow around the panel,
// inside ShadowMargin.
func (v *tooltipView) SetShadow(on bool) {
	if on != v.shadow {
		v.shadow = on
		v.Invalidate()
	}
}

// ShadowMargin returns the space around the panel the own shadow needs
// (zero without SetShadow), as the menu's.
func (v *tooltipView) ShadowMargin() driver.Insets {
	if !v.shadow {
		return driver.Insets{}
	}
	return shadowInsets(tokens.Tooltip.ShadowBlur, tokens.Tooltip.ShadowOffsetY)
}

// Panel returns the panel's rect: the bounds less ShadowMargin.
func (v *tooltipView) Panel() driver.Rect { return v.Bounds().Inset(v.ShadowMargin()) }

// FittingSize is the text's size plus the padding, in whole points, plus
// ShadowMargin, clamped to max.
func (v *tooltipView) FittingSize(max driver.Size) driver.Size {
	tt := &tokens.Tooltip
	var s driver.Size
	if v.layout != nil {
		s = v.layout.Size()
	}
	in := v.ShadowMargin()
	return driver.Size{
		W: math.Ceil(s.W+2*tt.PaddingX) + in.Left + in.Right,
		H: math.Ceil(s.H+2*tt.PaddingY) + in.Top + in.Bottom,
	}.Min(max)
}

// Paint draws the shadow, the panel, the text and the border.
func (v *tooltipView) Paint(c render.Canvas) {
	p := v.Panel()
	if p.Empty() {
		return
	}
	a := driver.DefaultAppearance()
	if w := v.Window(); w != nil {
		a = w.Appearance()
	}
	tt := &tokens.Tooltip
	scale := scaleIn(&v.Base)
	radii := render.UniformRadii(min(tt.CornerRadius, p.W/2, p.H/2))
	if v.shadow {
		c.DrawShadow(p, radii, render.Shadow{Offset: driver.Point{Y: tt.ShadowOffsetY}, Blur: tt.ShadowBlur, Color: tt.Shadow.Value(a)})
	}
	c.FillRoundedRect(p, radii, render.Solid(tt.Background.Value(a)))
	if v.layout != nil {
		c.Save()
		c.ClipRoundedRect(p, radii)
		c.DrawText(v.layout, driver.Point{X: p.X + tt.PaddingX, Y: p.Y + tt.PaddingY}, tt.Text.Value(a))
		c.Restore()
	}
	strokeInside(c, p, radii.TopLeft, pixelWidth(tt.BorderWidth, scale), tt.Border.Value(a))
}
