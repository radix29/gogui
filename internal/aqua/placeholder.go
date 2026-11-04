package aqua

import (
	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/render"
)

// Placeholder metrics, in points.
const (
	placeholderRadius = 4
	placeholderPad    = 4 // horizontal text inset
)

// PlaceholderFont is the font of the text drawn on placeholders (11 pt).
var PlaceholderFont = driver.Font{Size: 11}

// hosted is implemented by the placeholder peers (PLAN.md S2.2e): widgets
// that are not elements themselves but own one. Container and Window find
// a widget's element through ElementOf.
type hosted interface {
	AquaElement() Element
}

// ElementOf returns the element of widget w: w itself when it is an
// element, the element it owns when it is a placeholder peer, or nil for
// any other widget (nil included), which is then not drawn.
func ElementOf(w driver.Widget) Element {
	switch e := w.(type) {
	case Element:
		return e.base().mustSelf()
	case hosted:
		return e.AquaElement()
	}
	return nil
}

// placeholder is the element of a placeholder peer: a labelled rounded
// rectangle that shows the control's kind and its title or text, dimmed
// when disabled, with a focus ring while it is focused in the key window.
// Its accessible node comes from the peer.
type placeholder struct {
	Base
	d     *Driver
	label func() caption   // what is drawn
	node  func() a11y.Node // the accessible node
	ring  FocusRing
}

// caption is the text of a placeholder: its kind ("Button") and its title,
// text or value ("" for none).
type caption struct{ kind, text string }

func labelled(kind, text string) caption { return caption{kind, text} }

// String returns "kind: text", or the kind alone for an empty text.
func (c caption) String() string {
	if c.text == "" {
		return c.kind
	}
	return c.kind + ": " + c.text
}

// fit lays out the caption to fit width: "kind: text" when it fits whole,
// otherwise the text alone (or the kind, without a text) truncated with an
// ellipsis. It returns nil when not even the ellipsis fits.
func (c caption) fit(ts render.TextSystem, width float64) render.TextLayout {
	if l := ts.NewLayout(oneLine(c.String()), PlaceholderFont, 0); l.Size().W <= width {
		return l
	}
	short := c.text
	if short == "" {
		short = c.kind
	}
	return truncate(ts, short, PlaceholderFont, width)
}

// Caption returns the text the placeholder element of peer w would draw
// with unlimited room ("Button: OK"), and false for a widget that is not a
// placeholder peer.
func Caption(w driver.Widget) (string, bool) {
	if p, ok := w.(interface{ placeholderElement() *placeholder }); ok && p.placeholderElement() != nil {
		return p.placeholderElement().label().String(), true
	}
	return "", false
}

// appearance returns the appearance the placeholder is drawn in.
func (p *placeholder) appearance() driver.Appearance { return appearanceIn(&p.Base, p.d) }

// appearanceIn returns the appearance of the window b is shown in, or d's
// outside a window.
func appearanceIn(b *Base, d *Driver) driver.Appearance {
	if r, ok := b.Root().(*windowRoot); ok {
		return r.w.appearance
	}
	return d.current
}

// radii returns the corner radii of the placeholder's rounded rect.
func (p *placeholder) radii() render.Radii {
	b := p.Bounds()
	return render.UniformRadii(min(placeholderRadius, b.W/2, b.H/2))
}

// HandleEvent tracks the focus ring and handles nothing.
func (p *placeholder) HandleEvent(ev Event) bool {
	p.ring.Track(p, ev)
	return false
}

// Paint draws the placeholder, then its focus ring around the rounded rect
// (nothing for an empty frame).
func (p *placeholder) Paint(c render.Canvas) {
	if p.Bounds().Empty() {
		return
	}
	p.paintBody(c)
	p.ring.Paint(c, p, RectFocusShape(p.Bounds(), p.radii()))
}

func (p *placeholder) paintBody(c render.Canvas) {
	b := p.Bounds()
	a := p.appearance()
	radii := p.radii()
	c.FillRoundedRect(b, radii, render.Solid(semantic(driver.ColorControl, a)))
	if b.W > 1 && b.H > 1 {
		c.StrokeRoundedRect(b.Inset(driver.UniformInsets(0.5)), radii, 1,
			render.Solid(semantic(driver.ColorTertiaryLabel, a)))
	}
	avail := b.W - 2*placeholderPad
	if avail <= 0 {
		return
	}
	l := p.label().fit(p.d.sh.TextSystem(), avail)
	if l == nil || l.Size().H > b.H {
		return
	}
	col := driver.ColorControlText
	if !p.EnabledInTree() {
		col = driver.ColorDisabledControlText
	}
	c.DrawText(l, driver.Point{X: placeholderPad, Y: (b.H - l.Size().H) / 2}, semantic(col, a))
}

func (p *placeholder) AccessibleNode() a11y.Node { return p.node() }

// peerBase is embedded by every peer next to its stub peer: the placeholder
// peers and the real controls that still keep their state in a stub (the
// Label since S3.3c, the Separator, the Box, the Button and the Checkbox).
// Its methods win over the stub's promoted driver.Widget methods: each
// updates the stub state and the element.
type peerBase struct {
	sp   driver.Widget // the stub peer
	sb   *stub.Base
	elem Element      // the element: el, or a real control's
	el   *placeholder // nil for a real control
}

// init makes the peer a placeholder peer.
func (p *peerBase) init(d *Driver, sp driver.Widget, sb *stub.Base, label func() caption, node func() a11y.Node) {
	p.el = &placeholder{d: d, label: label, node: node}
	p.el.Init(p.el)
	p.initElement(sp, sb, p.el)
}

// initElement binds the peer to its stub and to its (initialised) element.
func (p *peerBase) initElement(sp driver.Widget, sb *stub.Base, e Element) {
	p.sp, p.sb, p.elem = sp, sb, e
}

// placeholderElement returns the placeholder element, nil for a real
// control.
func (p *peerBase) placeholderElement() *placeholder { return p.el }

// AquaElement returns the element.
func (p *peerBase) AquaElement() Element { return p.elem }

// StubPeer returns the embedded driver/stub peer (*stub.Label …).
func (p *peerBase) StubPeer() driver.Widget { return p.sp }

// Stub returns the state of the embedded stub peer.
func (p *peerBase) Stub() *stub.Base { return p.sb }

// SetFrame implements driver.Widget: the stub's frame and the element's.
func (p *peerBase) SetFrame(r driver.Rect) {
	p.sb.SetFrame(r)
	p.elem.SetFrame(r)
}

// SetEnabled implements driver.Widget.
func (p *peerBase) SetEnabled(enabled bool) {
	p.sb.SetEnabled(enabled)
	p.elem.base().SetEnabled(enabled)
}

// SetHidden implements driver.Widget.
func (p *peerBase) SetHidden(hidden bool) {
	p.sb.SetHidden(hidden)
	p.elem.base().SetHidden(hidden)
}

// Focus implements driver.Widget: counted by the stub, and the element
// takes the focus when it is focusable and in a window.
func (p *peerBase) Focus() {
	p.sb.Focus()
	p.elem.base().Focus()
}

// Destroy implements driver.Widget: the stub leaves its stub parent and the
// element its parent element.
func (p *peerBase) Destroy() {
	p.sb.Destroy()
	p.elem.base().RemoveFromParent()
}

// redraw invalidates the element after a change of what it shows.
func (p *peerBase) redraw() { p.elem.base().Invalidate() }

// host is a plain element that positions the content of a Box or the
// document of a ScrollView.
type host struct{ Base }

func newHost() *host {
	h := &host{}
	h.Init(h)
	return h
}

// setChild makes the element of w (if any) h's only child.
func (h *host) setChild(w driver.Widget) {
	if e := ElementOf(w); e != nil {
		h.SetChildren(e)
	} else {
		h.SetChildren()
	}
}
