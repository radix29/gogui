//go:build linux

package gtk4

import (
	"errors"
	"math"

	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// gtkPopover is the popover of a popup over GTK (popup.go): a GtkPopover
// parented to the parent's GGSurface, whose child is a GGSurface. The
// embedded gtkWindow holds the child (surf), its pointer and scroll
// controllers and the signal handlers, so the window methods of the child
// (cursor, drawing, the controllers' event state) are the surfaces'; it
// has no GtkWindow, GdkSurface, key controller or input method, and the
// methods that would need them are overridden here. The gtkPopover holds
// one reference to the popover until destroy.
type gtkPopover struct {
	*gtkWindow
	pop      uintptr    // the GtkPopover
	parent   *gtkWindow // the parent surface's window
	parented bool       // WidgetSetParent ran, WidgetUnparent not yet
}

var _ popover = (*gtkPopover)(nil)

// newPopover implements loop: the GtkPopover (sunk, so it is ours; the
// parent takes its own reference) with the popoverClass CSS class, no
// arrow, GTK_POS_BOTTOM, halign START and LTR (popup.go), autohide for a
// grab popup and can-target false for the others, the child GGSurface
// with p as its peer and, for a grab popup, the pointer and scroll
// controllers with p as their peer and a key controller with keys (the
// parent surface) as its peer, all on the popover, and "closed"; then
// the parent.
func (l *gtkLoop) newPopover(parent window, keys windowPeer, grab bool, p windowPeer) (popover, error) {
	pw, ok := parent.(*gtkWindow)
	if !ok || pw.surf == 0 {
		return nil, errors.New("gtk4: newPopover: the parent has no GGSurface")
	}
	pop := gtk.Gtk.PopoverNew()
	if pop == 0 {
		return nil, errors.New("gtk4: gtk_popover_new returned NULL")
	}
	g := &gtkPopover{gtkWindow: &gtkWindow{}, pop: gtk.GObject.ObjectRefSink(pop), parent: pw}
	g.surf = newSurfaceWidget(p)
	if g.surf == 0 {
		g.destroy()
		return nil, errors.New("gtk4: could not create the popup's GGSurface")
	}
	l.addPopoverCSS(gtk.Gtk.WidgetGetDisplay(pw.surf))
	gtk.Gtk.WidgetAddCSSClass(g.pop, popoverClass)
	gtk.Gtk.PopoverSetHasArrow(g.pop, gtk.False)
	gtk.Gtk.PopoverSetPosition(g.pop, gtk.PosBottom)
	gtk.Gtk.WidgetSetHalign(g.pop, gtk.AlignStart)
	gtk.Gtk.WidgetSetDirection(g.pop, gtk.TextDirLTR)
	gtk.Gtk.PopoverSetAutohide(g.pop, gbool(grab))
	if !grab {
		gtk.Gtk.WidgetSetCanTarget(g.pop, gtk.False)
	} else {
		// On the popover, not the child: an autohide popover holds a GTK
		// grab, and events of the grab outside the child (the X11 pointer
		// grab reports every event on the popup surface, GTK redirects
		// events of other widgets to the grab widget) reach the popover
		// but not its child. The popover's coordinates are the child's:
		// popoverCSS clears its margins, borders and padding.
		g.addPointerControllers(g.pop, p)
		g.addScrollController(g.pop, p)
		g.addPopoverKeys(keys)
	}
	gtk.Gtk.PopoverSetChild(g.pop, g.surf) // the popover takes its own reference
	if grab && !g.connect(g.pop, "closed", tramp.sigVoid, p, kindPopoverClosed) {
		g.destroy()
		return nil, errors.New("gtk4: could not connect GtkPopover::closed")
	}
	gtk.Gtk.WidgetSetParent(g.pop, pw.surf)
	g.parented = true
	return g, nil
}

// addPopoverCSS adds the provider of popoverCSS for display, once per
// display, at the user priority so neither the theme nor gtk.css put a
// frame back. The display keeps its own reference.
func (l *gtkLoop) addPopoverCSS(display uintptr) {
	if display == 0 || l.cssDisplays[display] {
		return
	}
	prov := gtk.Gtk.CSSProviderNew()
	if prov == 0 {
		return
	}
	gtk.Gtk.CSSProviderLoadFromString(prov, popoverCSS)
	gtk.Gtk.StyleContextAddProviderForDisplay(display, prov, gtk.StyleProviderPriorityUser)
	gtk.GObject.ObjectUnref(prov)
	if l.cssDisplays == nil {
		l.cssDisplays = map[uintptr]bool{}
	}
	l.cssDisplays[display] = true
}

// place queues a resize of the child (its natural size comes from the
// peer's measure), then sets the offset and pointing_to, which presents a
// shown popover.
func (g *gtkPopover) place(pl popoverPlacement) {
	gtk.Gtk.WidgetQueueResize(g.surf)
	gtk.Gtk.PopoverSetOffset(g.pop, pl.dx, pl.dy)
	r := pl.pointing
	gtk.Gtk.PopoverSetPointingTo(g.pop, &r)
}

func (g *gtkPopover) popup()   { gtk.Gtk.PopoverPopup(g.pop) }
func (g *gtkPopover) present() { gtk.Gtk.PopoverPresent(g.pop) }

// origin adds up where the GdkPopup landed relative to the parent's
// GdkSurface, where the child sits in the popover's surface (the
// popover's surface transform and the child's offset in the popover), and
// takes away where the parent's GGSurface sits in its window's surface.
func (g *gtkPopover) origin() (x, y float64, ok bool) {
	gdk := gtk.Gtk.NativeGetSurface(g.pop)
	if gdk == 0 {
		return 0, 0, false
	}
	x, y = float64(gtk.Gdk.PopupGetPositionX(gdk)), float64(gtk.Gdk.PopupGetPositionY(gdk))
	cx, cy := widgetIn(g.surf, g.pop)
	x, y = x+cx, y+cy
	px, py := widgetIn(g.parent.surf, g.parent.win)
	return x - px, y - py, true
}

// widgetIn returns where w's top-left corner lies in the GdkSurface of
// native: w's offset in native plus native's surface transform.
func widgetIn(w, native uintptr) (x, y float64) {
	var out gtk.GraphenePoint
	if gtk.Gtk.WidgetComputePoint(w, native, &gtk.GraphenePoint{}, &out) == gtk.False {
		out = gtk.GraphenePoint{}
	}
	var nx, ny float64
	gtk.Gtk.NativeGetSurfaceTransform(native, &nx, &ny)
	return math.Round(float64(out.X) + nx), math.Round(float64(out.Y) + ny)
}

// scale is the popover surface's fractional scale, its parent's before it
// has a surface.
func (g *gtkPopover) scale() float64 {
	if gdk := gtk.Gtk.NativeGetSurface(g.pop); gdk != 0 {
		if s := gtk.Gdk.SurfaceGetScale(gdk); s > 0 {
			return s
		}
	}
	return g.parent.scale()
}

// The toplevel's methods: a popover has no title, default size, state or
// keyboard of its own.
func (g *gtkPopover) setTitle(string)                     {}
func (g *gtkPopover) setDefaultSize(int32, int32)         {}
func (g *gtkPopover) minimize()                           {}
func (g *gtkPopover) setMaximized(bool)                   {}
func (g *gtkPopover) maximized() bool                     { return false }
func (g *gtkPopover) active() bool                        { return false }
func (g *gtkPopover) keyBase(uintptr, uint32) uint32      { return 0 }
func (g *gtkPopover) keyShifted(uintptr, uint32) uint32   { return 0 }
func (g *gtkPopover) beginMove(uintptr, float64, float64) {}
func (g *gtkPopover) readback(float64) (readbackFrame, error) {
	return readbackFrame{}, errors.New("gtk4: readback: not for a popup")
}

// destroy disconnects the handlers ("closed" first, so the popdown is not
// a dismissal), pops the popover down, unparents it from the parent's
// GGSurface (which drops the parent's reference; the parent GGSurface is
// still alive: its surface closes its popups first), releases the child
// (its handle, then the gtkPopover's reference) and drops the reference to
// the popover, which finalizes it. It may run on a half-built popover.
func (g *gtkPopover) destroy() {
	for _, c := range g.conns {
		if gtk.GObject.SignalHandlerIsConnected(c.inst, c.id) != gtk.False {
			gtk.GObject.SignalHandlerDisconnect(c.inst, c.id)
		}
	}
	g.conns = nil
	if g.pop != 0 {
		gtk.Gtk.PopoverPopdown(g.pop)
		if g.parented {
			gtk.Gtk.WidgetUnparent(g.pop)
			g.parented = false
		}
	}
	releaseSurfaceWidget(g.surf)
	if g.pop != 0 {
		gtk.GObject.ObjectUnref(g.pop)
	}
	g.pop, g.surf, g.motion, g.pointer, g.scroll = 0, 0, 0, 0, 0
}

// addPopoverKeys adds a key controller in the capture phase to the
// popover, for the parent surface keys ("key-pressed", "key-released" of
// its keyboard, keyboard.go): GTK's grab propagates the key events only
// from the grab widget down, so the parent window's own key controller
// may not see them while the popover is up. A key both controllers see
// is delivered once (keyState.last).
func (g *gtkPopover) addPopoverKeys(keys windowPeer) {
	k := gtk.Gtk.EventControllerKeyNew()
	if k == 0 {
		return
	}
	gtk.Gtk.EventControllerSetPropagationPhase(k, gtk.PhaseCapture)
	gtk.Gtk.WidgetAddController(g.pop, k) // transfer full
	g.connect(k, "key-pressed", tramp.sigBoolUintUintUint, keys, kindKeyPressed)
	g.connect(k, "key-released", tramp.sigUintUintUint, keys, kindKeyReleased)
}

// popoverClass is the CSS class that clears a popup's popover (the
// display-wide provider of addPopoverCSS).
const popoverClass = "gogui-popup"

// popoverCSS clears the popover's margins, padding, border, corner radius
// (which would give it a minimum size), background and shadow, and its
// contents node's too, so the child fills the popover's surface.
const popoverCSS = `popover.gogui-popup, popover.gogui-popup > contents {
  margin: 0; padding: 0; border: none; border-radius: 0; box-shadow: none;
  background: none; min-width: 0; min-height: 0; outline: none;
}
`
