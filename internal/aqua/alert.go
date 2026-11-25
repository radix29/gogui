package aqua

import (
	"math"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
)

// alert is the drawn alert element (PLAN.md S5.7a), the content of a sheet
// (S5.7c, sheet.go) or of a modal surface (S5.7g, alertsurface.go),
// modelled on the macOS 11+ NSAlert. S5.7a builds and paints it, S5.7b
// gives it its input (below and alertRoot), S5.7c shows it as a sheet,
// S5.7g in a modal surface.
//
// Geometry, in the element's own points (the Alert tokens): the panel is
// the bounds less ShadowMargin, FittingSize gives it the fixed Width. From
// PaddingTop below its top: the icon, IconSize square and centred; IconGap
// below it the message, in the system font of MessageFontSize with
// Typography.EmphasizedWeight; DetailGap below that the detail in the
// system font of DetailFontSize (left out with its gap when empty); both
// are Label peers, wrapped to the panel's width less PaddingX on both
// sides and centred line by line, in ColorLabel. ButtonsGap below the
// texts the buttons, regular push Button peers: two buttons whose titles
// both fit half the row (less ButtonGap) sit side by side with equal
// widths, the first (the default) on the right; otherwise every button is
// stacked full-width, ButtonGap apart, the first on top. PaddingBottom
// below the last row is the panel's bottom edge. The first button is the
// default button (accent-filled in a key window); a title wider than its
// button is truncated by the button.
//
// The icon is procedural (§3.1): a neutral app placeholder (a rounded
// square in a vertical gradient from IconTop to IconBottom with an
// IconBorder hairline and a window glyph in IconGlyph). A critical alert
// shows a caution triangle (CautionFill, its exclamation mark in
// CautionMark) with the placeholder as a BadgeSize badge at its bottom
// right. A warning alert looks like an informational one, as NSAlert's
// does (decided in S5.7a).
//
// Paint: the panel's own shadow when SetShadow is on (a sheet, drawn over
// the window's content), the panel filled with Background, the icon, the
// border (Border, whole device pixels) and then the children: the texts
// and the buttons. In a modal surface (S5.7g, setInSurface) the panel is
// square and has no border: the OS gives the surface its corners, border
// and shadow, as it does a window's.
//
// Input (S5.7b): the alert is answered once, by the first press of a
// button; its index in AlertSpec.Buttons goes to the press func
// (SetPressFunc), and every later press does nothing (Result). A button
// is pressed by a click (its press tracker: a release over it) or by a key
// that reaches the alert (from its focused button, bubbling, or as the
// root's key target): Return presses the default button (the first),
// Escape and Primary+. the Cancel button (AlertSpec.Cancel; nothing, the
// key unhandled, when it is -1 or out of range), Space the focused button
// in an active router. A key press flashes its button (drawn pressed for
// keyFlash, as a click shows it) and answers at once; an auto-repeat
// answers nothing but is consumed. Tab and Shift-Tab between the buttons
// are the router's (FocusNext over the buttons, in spec order: side by
// side that is the left one to the default on the right, stacked top to
// bottom, as AppKit's key view loop runs). An alert gets its own router
// from alertRoot; inside a window's tree it is only painted.
//
// Accessibility: a RoleDialog node labelled with the message, its children
// the message and the detail as static texts and the buttons, in the
// order of AlertSpec.Buttons.
type alert struct {
	Base
	d       *Driver
	spec    driver.AlertSpec
	message *Label
	detail  *Label
	glyphs  scratchPath // the icon's paths
	buttons []*Button   // in AlertSpec.Buttons order
	shadow  bool
	surface bool // the whole content of a modal surface (setInSurface)
	onPress func(i int)

	// The answer (answer): whether a button was pressed, and which.
	answered bool
	result   int

	// The laid-out geometry, from layout: the panel's content height and
	// whether the buttons are stacked.
	height  float64
	stacked bool
}

// alertButton is the handler of an alert's button i.
type alertButton struct {
	a *alert
	i int
}

func (h alertButton) Clicked() { h.a.answer(h.i) }

// answer records the first press, of button i, and reports it to the
// press func; later presses do nothing.
func (a *alert) answer(i int) {
	if a.answered {
		return
	}
	a.answered, a.result = true, i
	if a.onPress != nil {
		a.onPress(i)
	}
}

// Result returns the index of the pressed button; ok is false while the
// alert is unanswered.
func (a *alert) Result() (i int, ok bool) { return a.result, a.answered }

// cancelIndex returns the index of the Cancel button, or -1 for none.
func (a *alert) cancelIndex() int {
	if c := a.spec.Cancel; c >= 0 && c < len(a.buttons) {
		return c
	}
	return -1
}

// focusedButton returns the index of the button that holds the focus of
// an active router, or -1.
func (a *alert) focusedButton() int {
	for i, b := range a.buttons {
		if b.view.FocusState() == FocusedActive {
			return i
		}
	}
	return -1
}

// HandleEvent implements Element: the alert's keys (see alert). The key
// of an enabled button flashes it and answers; once answered, the keys
// are still consumed.
func (a *alert) HandleEvent(ev Event) bool {
	k, ok := ev.(KeyEvent)
	if !ok || !k.Down {
		return false
	}
	i := -1
	switch {
	case isDefaultKey(k.KeyEvent):
		i = 0
	case isCancelKey(k.KeyEvent):
		i = a.cancelIndex()
	case k.Key == driver.KeySpace && k.Mods == 0:
		i = a.focusedButton()
	}
	if i < 0 || !a.buttons[i].view.EnabledInTree() {
		return false
	}
	if !k.Repeat && !a.answered {
		a.buttons[i].view.keyFlash()
		a.answer(i)
	}
	return true
}

// neighbour returns the index of the button the focus starts on: the
// default button's neighbour in the key view loop (the second button),
// or the default itself when it is the only one (decided in S5.7b).
func (a *alert) neighbour() int { return min(1, len(a.buttons)-1) }

// newAlert returns the alert of spec, its children made by d. A spec
// without buttons gets an "OK" button (the public layer always passes
// one).
func newAlert(d *Driver, spec driver.AlertSpec) *alert {
	if len(spec.Buttons) == 0 {
		spec.Buttons = []string{"OK"}
	}
	spec.Buttons = append([]string(nil), spec.Buttons...)
	a := &alert{d: d, spec: spec}
	a.Init(a)

	at := &tokens.Alert
	a.message = d.NewLabel().(*Label)
	a.message.SetText(spec.Message)
	a.message.SetFont(driver.Font{Size: at.MessageFontSize, Weight: tokens.Typography.EmphasizedWeight})
	a.detail = d.NewLabel().(*Label)
	a.detail.SetText(spec.Detail)
	a.detail.SetFont(driver.Font{Size: at.DetailFontSize})
	children := []Element{ElementOf(a.message), ElementOf(a.detail)}
	for _, l := range []*Label{a.message, a.detail} {
		l.SetAlignment(driver.AlignCenter)
		l.SetWraps(true)
		l.SetHidden(l.Text == "")
	}
	for i, title := range spec.Buttons {
		b := d.NewButton(alertButton{a, i}).(*Button)
		b.SetTitle(title)
		b.SetDefault(i == 0)
		a.buttons = append(a.buttons, b)
		children = append(children, ElementOf(b))
	}
	a.SetChildren(children...)
	a.layout()
	return a
}

// Spec returns the alert's spec (with the "OK" button supplied).
func (a *alert) Spec() driver.AlertSpec { return a.spec }

// SetPressFunc sets what the answer reports: the index in
// AlertSpec.Buttons of the button pressed first, by click or key.
func (a *alert) SetPressFunc(f func(i int)) { a.onPress = f }

// Button returns the peer of button i (in AlertSpec.Buttons order).
func (a *alert) Button(i int) *Button { return a.buttons[i] }

// SetShadow sets whether the alert draws its own shadow around the panel,
// inside ShadowMargin (off by default).
func (a *alert) SetShadow(on bool) {
	if on != a.shadow {
		a.shadow = on
		a.layout()
		a.Invalidate()
	}
}

// setInSurface sets whether the alert is the whole content of a modal
// surface (S5.7g, alertsurface.go): the panel square and without border
// (see Paint). It turns the own shadow off.
func (a *alert) setInSurface(on bool) {
	if on {
		a.SetShadow(false)
	}
	if on != a.surface {
		a.surface = on
		a.Invalidate()
	}
}

// ShadowMargin returns the space around the panel the own shadow needs:
// zero without SetShadow, otherwise the blur on every side, less the
// offset above and more below (as the menu's).
func (a *alert) ShadowMargin() driver.Insets {
	if !a.shadow {
		return driver.Insets{}
	}
	at := &tokens.Alert
	return shadowInsets(at.ShadowBlur, at.ShadowOffsetY)
}

// Panel returns the panel's rect: the bounds less ShadowMargin.
func (a *alert) Panel() driver.Rect { return a.Bounds().Inset(a.ShadowMargin()) }

// FittingSize is the panel at its fixed Width and the content's height,
// plus ShadowMargin, clamped to max.
func (a *alert) FittingSize(max driver.Size) driver.Size {
	in := a.ShadowMargin()
	w := tokens.Alert.Width
	return driver.Size{W: w + in.Left + in.Right, H: a.contentHeight(w) + in.Top + in.Bottom}.Min(max)
}

// SetFrame implements Element: the children are laid out in the new
// panel.
func (a *alert) SetFrame(r driver.Rect) {
	a.Base.SetFrame(r)
	a.layout()
}

// IconRect returns the icon's square in the element's points.
func (a *alert) IconRect() driver.Rect {
	at := &tokens.Alert
	p := a.Panel()
	return driver.Rect{X: p.X + (p.W-at.IconSize)/2, Y: p.Y + at.PaddingTop, W: at.IconSize, H: at.IconSize}
}

// sideBySide reports whether the buttons fit side by side in a row width
// wide: exactly two, both titles fitting half of it less ButtonGap.
func (a *alert) sideBySide(width float64) bool {
	if len(a.buttons) != 2 {
		return false
	}
	half := (width - tokens.Alert.ButtonGap) / 2
	for _, b := range a.buttons {
		if b.FittingSize(driver.Size{W: math.Inf(1), H: math.Inf(1)}).W > half {
			return false
		}
	}
	return true
}

// textHeight returns the height of l wrapped to width, rounded up to whole
// points (so the panel is), zero when hidden.
func textHeight(l *Label, width float64) float64 {
	if l.Text == "" {
		return 0
	}
	return math.Ceil(l.FittingSize(driver.Size{W: width, H: math.Inf(1)}).H)
}

// contentHeight returns the panel's height at width: the paddings, the
// icon, the texts with their gaps and the button rows.
func (a *alert) contentHeight(width float64) float64 {
	at := &tokens.Alert
	inner := max(0, width-2*at.PaddingX)
	h := at.PaddingTop + at.IconSize + at.IconGap + textHeight(a.message, inner)
	if a.detail.Text != "" {
		h += at.DetailGap + textHeight(a.detail, inner)
	}
	bh := tokens.Metrics.Control(driver.ControlSizeRegular).Height
	rows := len(a.buttons)
	if a.sideBySide(inner) {
		rows = 1
	}
	return h + at.ButtonsGap + float64(rows)*bh + float64(rows-1)*at.ButtonGap + at.PaddingBottom
}

// layout frames the texts and the buttons in the panel.
func (a *alert) layout() {
	at := &tokens.Alert
	p := a.Panel()
	inner := max(0, p.W-2*at.PaddingX)
	x := p.X + at.PaddingX
	y := p.Y + at.PaddingTop + at.IconSize + at.IconGap
	h := textHeight(a.message, inner)
	a.message.SetFrame(driver.Rect{X: x, Y: y, W: inner, H: h})
	y += h
	if a.detail.Text != "" {
		y += at.DetailGap
		h = textHeight(a.detail, inner)
		a.detail.SetFrame(driver.Rect{X: x, Y: y, W: inner, H: h})
		y += h
	}
	y += at.ButtonsGap
	bh := tokens.Metrics.Control(driver.ControlSizeRegular).Height
	a.stacked = !a.sideBySide(inner)
	if a.stacked {
		for _, b := range a.buttons {
			b.SetFrame(driver.Rect{X: x, Y: y, W: inner, H: bh})
			y += bh + at.ButtonGap
		}
		y -= at.ButtonGap
	} else {
		bw := (inner - at.ButtonGap) / 2
		a.buttons[1].SetFrame(driver.Rect{X: x, Y: y, W: bw, H: bh})
		a.buttons[0].SetFrame(driver.Rect{X: x + bw + at.ButtonGap, Y: y, W: bw, H: bh})
		y += bh
	}
	a.height = y + at.PaddingBottom - p.Y
}

// ContentHeight returns the panel's height that shows everything, at the
// current panel width.
func (a *alert) ContentHeight() float64 { return a.height }

// Paint draws the shadow, the panel, the icon and the border (see alert).
func (a *alert) Paint(c render.Canvas) {
	p := a.Panel()
	if p.Empty() {
		return
	}
	at := &tokens.Alert
	ap := appearanceIn(&a.Base, a.d)
	scale := scaleIn(&a.Base)
	radii := render.UniformRadii(min(at.CornerRadius, p.W/2, p.H/2))
	if a.surface {
		radii = render.Radii{}
	}
	if a.shadow {
		c.DrawShadow(p, radii, render.Shadow{Offset: driver.Point{Y: at.ShadowOffsetY}, Blur: at.ShadowBlur, Color: at.Shadow.Value(ap)})
	}
	c.FillRoundedRect(p, radii, render.Solid(at.Background.Value(ap)))
	icon := a.IconRect()
	if a.spec.Style == driver.AlertCritical {
		paintCaution(c, &a.glyphs, icon, ap)
		s := at.BadgeSize
		paintAppIcon(c, &a.glyphs, driver.Rect{X: icon.X + icon.W - s, Y: icon.Y + icon.H - s, W: s, H: s}, ap, scale)
	} else {
		paintAppIcon(c, &a.glyphs, icon, ap, scale)
	}
	if !a.surface {
		strokeInside(c, p, radii.TopLeft, pixelWidth(at.BorderWidth, scale), at.Border.Value(ap))
	}
}

// paintAppIcon draws the app placeholder in the square r: a rounded square
// (IconCornerRadius at IconSize, scaled with the side) in the IconTop to
// IconBottom gradient with an IconBorder hairline, and a window glyph in
// IconGlyph: a rounded outline with a title-bar line, centred.
func paintAppIcon(c render.Canvas, sp *scratchPath, r driver.Rect, a driver.Appearance, scale float64) {
	at := &tokens.Alert
	s := r.W
	radii := render.UniformRadii(min(at.IconCornerRadius*s/at.IconSize, s/2))
	c.FillRoundedRect(r, radii, render.LinearGradient(
		driver.Point{X: r.X, Y: r.Y}, driver.Point{X: r.X, Y: r.Y + s},
		render.GradientStop{Offset: 0, Color: at.IconTop.Value(a)},
		render.GradientStop{Offset: 1, Color: at.IconBottom.Value(a)}))
	strokeInside(c, r, radii.TopLeft, pixelWidth(1, scale), at.IconBorder.Value(a))
	glyph := render.Solid(at.IconGlyph.Value(a))
	lw := max(1, s/32)
	win := driver.Rect{X: r.X + 0.22*s, Y: r.Y + 0.28*s, W: 0.56 * s, H: 0.44 * s}
	c.StrokeRoundedRect(win, render.UniformRadii(0.06*s), lw, glyph)
	bar := win.Y + 0.12*s
	c.StrokePath(sp.scratch().MoveTo(driver.Point{X: win.X, Y: bar}).LineTo(driver.Point{X: win.X + win.W, Y: bar}),
		lw, glyph, render.StrokeStyle{})
}

// paintCaution draws the caution triangle in the square r: an equilateral
// triangle with rounded corners, as wide as r and standing on its bottom,
// in CautionFill, with an exclamation mark (a round-capped bar and a dot)
// in CautionMark.
func paintCaution(c render.Canvas, sp *scratchPath, r driver.Rect, a driver.Appearance) {
	at := &tokens.Alert
	s := r.W
	cx := r.X + s/2
	h := s * math.Sqrt(3) / 2
	top := driver.Point{X: cx, Y: r.Y + (s-h)/2}
	bl := driver.Point{X: r.X, Y: top.Y + h}
	br := driver.Point{X: r.X + s, Y: top.Y + h}
	rad := 0.1 * s
	tri := sp.scratch().MoveTo(driver.Point{X: cx, Y: bl.Y}).
		ArcTo(br, top, rad).ArcTo(top, bl, rad).ArcTo(bl, br, rad).Close()
	c.FillPath(tri, render.Solid(at.CautionFill.Value(a)))

	mark := render.Solid(at.CautionMark.Value(a))
	w := 0.09 * s
	y0, y1 := top.Y+0.36*h, top.Y+0.7*h
	c.StrokePath(sp.scratch().MoveTo(driver.Point{X: cx, Y: y0}).LineTo(driver.Point{X: cx, Y: y1}),
		w, mark, render.StrokeStyle{Cap: render.CapRound})
	dot := driver.Rect{X: cx - w*0.6, Y: top.Y + 0.84*h - w*0.6, W: 1.2 * w, H: 1.2 * w}
	c.FillRoundedRect(dot, render.UniformRadii(dot.W/2), mark)
}

// AccessibleNode implements Element: a dialog labelled with the message;
// its children are the texts and the buttons.
func (a *alert) AccessibleNode() a11y.Node {
	return a11y.Node{Role: a11y.RoleDialog, Label: a.spec.Message}
}

// alertRoot is the root of an alert's own element tree (S5.7b), so that
// no input of the alert reaches the parent's content: the alert is its
// only child, framed to the whole root, and the root owns the tree's
// router. The sheet (S5.7c) and the modal surface (S5.7g) host one; the
// host passes the shell's input events to Event, in root points, and does
// not pass unhandled ones on.
//
// The focus starts on the alert's neighbour button (alert.neighbour) with
// full keyboard access, the router's default, and there is none without
// it (Tab then reaches no button: the buttons are FocusControl); the
// router starts active. Without a window the tree's key state is the
// router's (SetActive), its appearance the driver's and its scale the
// modal surface's (scale, S5.7g, alertsurface.go); a sheet's root
// (S5.7c, sheet.go) belongs to its window (w), as a popup's tree does:
// Base.Window of its elements is that window, so they paint with its
// appearance, key state, scale and timeline. A key with no button focused
// (no full keyboard access) goes from the root to the alert.
type alertRoot struct {
	Base
	alert  *alert
	router *Router
	w      *Window       // the sheet's window, or nil
	surf   *alertSurface // the modal surface, or nil (S5.7g)
	scale  float64       // the modal surface's scale, 0 outside one (S5.7g)
}

// HandleEvent implements Element: a key the router sends the root (nothing
// focused) goes to the alert.
func (r *alertRoot) HandleEvent(ev Event) bool {
	if _, ok := ev.(KeyEvent); ok && r.router.Focused() == nil {
		return r.alert.HandleEvent(ev)
	}
	return false
}

// newAlertRoot returns the root of a, which must be detached.
func newAlertRoot(a *alert) *alertRoot {
	r := &alertRoot{alert: a}
	r.Init(r)
	r.AddChild(a)
	r.router = NewRouter(r)
	r.focusStart()
	return r
}

// Router returns the tree's router.
func (r *alertRoot) Router() *Router { return r.router }

// FittingSize is the alert's.
func (r *alertRoot) FittingSize(max driver.Size) driver.Size { return r.alert.FittingSize(max) }

// SetFrame implements Element: the alert fills the root.
func (r *alertRoot) SetFrame(f driver.Rect) {
	r.Base.SetFrame(f)
	r.alert.SetFrame(driver.RectOf(driver.Point{}, f.Size()))
}

// Event routes one shell input event through the router and reports
// whether the alert handled it.
func (r *alertRoot) Event(ev shell.Event) bool { return r.router.Event(ev) }

// SetActive records whether the alert's window is key (Router.SetActive),
// repaints the tree and sends every element a WindowStateEvent, as a
// window does on its key change.
func (r *alertRoot) SetActive(active bool) {
	if active == r.router.Active() {
		return
	}
	r.router.SetActive(active)
	r.stateChanged()
}

// stateChanged repaints the tree and sends its elements a WindowStateEvent
// with the router's key state and the tree's appearance (rootContext),
// stopping once the sheet's window or the modal surface closes.
func (r *alertRoot) stateChanged() {
	broadcastState(r, r.router, func() WindowStateEvent {
		return WindowStateEvent{Key: r.router.Active(), Appearance: r.rootAppearance()}
	}, r.gone)
}

// gone reports whether the window or the modal surface the alert is shown
// in has closed.
func (r *alertRoot) gone() bool {
	return (r.w != nil && r.w.closed) || (r.surf != nil && r.surf.closed)
}

// SetFullKeyboardAccess sets the router's keyboard access mode
// (Router.SetFullKeyboardAccess): turned on without a focus, the focus
// goes to the neighbour button; turned off, the focus is cleared.
func (r *alertRoot) SetFullKeyboardAccess(on bool) {
	r.router.SetFullKeyboardAccess(on)
	if on {
		r.focusStart()
	} else {
		r.router.SetFocus(nil)
	}
}

// focusStart focuses the neighbour button with full keyboard access and
// no focus yet.
func (r *alertRoot) focusStart() {
	if r.router.FullKeyboardAccess() && r.router.Focused() == nil {
		r.router.SetFocus(r.alert.buttons[r.alert.neighbour()].view)
	}
}
