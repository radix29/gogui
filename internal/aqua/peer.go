package aqua

import (
	"strconv"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
)

// The peers of the controls (PLAN.md S2.2e, S3.3 onward). Each embeds its
// driver/stub peer, which keeps the state, the getters, the User…
// simulations and, for most controls, the fake FittingSize (§11.1 rule 2),
// and a peerBase, which binds it to its drawn element. Every setter that
// changes what the element shows updates the stub and repaints. Since
// S5.5j every control is a real one (the PopUpButton was the last
// placeholder); the labelled placeholder rectangle of S2.2e is gone.

func fmtNumber(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

// hosted is implemented by the peers: widgets that are not elements
// themselves but own one. Container and Window find a widget's element
// through ElementOf.
type hosted interface {
	AquaElement() Element
}

// ElementOf returns the element of widget w: w itself when it is an
// element, the element it owns when it is a peer, or nil for any other
// widget (nil included), which is then not drawn.
func ElementOf(w driver.Widget) Element {
	switch e := w.(type) {
	case Element:
		return e.base().mustSelf()
	case hosted:
		return e.AquaElement()
	}
	return nil
}

// peerBase is embedded by every peer next to its stub peer. Its methods
// win over the stub's promoted driver.Widget methods: each updates the
// stub state and the element.
type peerBase struct {
	sp   driver.Widget // the stub peer
	sb   *stub.Base
	elem Element // the control's element
}

// initElement binds the peer to its stub and to its (initialised) element.
func (p *peerBase) initElement(sp driver.Widget, sb *stub.Base, e Element) {
	p.sp, p.sb, p.elem = sp, sb, e
}

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
