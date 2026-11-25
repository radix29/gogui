package gtk4

import "slices"

// The GtkSettings half of the appearance source (PLAN.md S4.3d, the caret
// blink since S5.2d): the boolean and int properties readAppearance reads,
// kept live through notify::<name> on the default GtkSettings, connected
// once through the sigPointer trampoline (no trampoline of its own).

// gtkBoolProps and gtkIntProps are the GtkSettings properties
// readAppearance reads, by type.
var (
	gtkBoolProps = [...]string{gtkEnableAnimations, gtkOverlayScrolling, gtkPreferDark, gtkCursorBlink}
	gtkIntProps  = [...]string{gtkCursorBlinkTime, gtkCursorBlinkTimeout}
)

// gtkWatched are gtkBoolProps and gtkIntProps; a notify of any of them
// re-reads the appearance.
var gtkWatched = slices.Concat(gtkBoolProps[:], gtkIntProps[:])

// gtkProps is the native side of the GtkSettings: the default GtkSettings
// over GTK (gtksettings_linux.go), a fake in the host tests. Every method
// runs on the GTK thread.
type gtkProps interface {
	// boolProp reads the boolean property name; ok is false without a
	// default GtkSettings or when it has no such property.
	boolProp(name string) (v, ok bool)
	// intProp reads the int property name; ok is false without a default
	// GtkSettings or when it has no such property.
	intProp(name string) (v int32, ok bool)
	// watch connects notify::name to p's pointerSignal with kind k; it
	// returns false when nothing was connected (no GtkSettings, no such
	// property, no such signal).
	watch(name string, p pointerSignalPeer, k kind) bool
	// unwatch disconnects every handler watch connected (their handles are
	// freed by closureNotify) and lets go of the GtkSettings. Calling it
	// again does nothing.
	unwatch()
}

// gtkSettings reads and watches the GtkSettings for systemSettings. A nil
// *gtkSettings or one without props is a valid source with every value
// unset.
type gtkSettings struct {
	props   gtkProps
	changed func() // the source's change report; called on the GTK thread
	watched int    // properties connected (tests, the debug log)
	closed  bool
}

// openGtkSettings opens the GtkSettings over props (nil: none) and
// watches gtkWatched, each notify calling changed.
func openGtkSettings(props gtkProps, changed func()) *gtkSettings {
	g := &gtkSettings{props: props, changed: changed}
	if props == nil {
		return g
	}
	for _, name := range gtkWatched {
		if props.watch(name, g, kindSettingNotify) {
			g.watched++
		}
	}
	return g
}

// gtkBool implements the GtkSettings half of settingsSource.
func (g *gtkSettings) gtkBool(name string) (bool, bool) {
	if g == nil || g.props == nil || g.closed {
		return false, false
	}
	return g.props.boolProp(name)
}

// gtkInt implements the GtkSettings half of settingsSource.
func (g *gtkSettings) gtkInt(name string) (int32, bool) {
	if g == nil || g.props == nil || g.closed {
		return 0, false
	}
	return g.props.intProp(name)
}

// pointerSignal is notify::<name> of a watched property (arg is its
// GParamSpec): it reports a change. A notify after close (a stale handle
// does not get here) is dropped all the same.
func (g *gtkSettings) pointerSignal(k kind, _, _ uintptr) {
	if k != kindSettingNotify || g.closed || g.changed == nil {
		return
	}
	g.changed()
}

// close disconnects the handlers; changed is not called after it.
func (g *gtkSettings) close() {
	if g == nil || g.closed {
		return
	}
	g.closed = true
	if g.props != nil {
		g.props.unwatch()
	}
}
