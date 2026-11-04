package gtk4

import (
	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// kind tells apart what one peer receives through trampolines of the same
// signature (a surface's "pressed" and "released" share one), and marks
// the entry a GGSurface's handle word points to.
type kind uint8

const (
	// kindSurface is the entry of a GGSurface instance's handle word; its
	// peer is a surfacePeer.
	kindSurface kind = iota + 1
	// kindWake is the entry of a wake idle source (wake.go); its peer is a
	// *wakeSource.
	kindWake
	// kindTick is the entry of the frame clock's tick callback (clock.go); its
	// peer is the *clock.
	kindTick
	// kindFrameTimeout is the entry of the frame clock's timeout source, the
	// fallback without a ticking surface; its peer is the *clock.
	kindFrameTimeout
	// The surface's window signals (surface.go); their peer is the
	// *surface: GtkWindow::close-request (sigBool), notify::is-active of the
	// window and notify::scale of the GdkSurface (sigPointer), and
	// notify::state of the GdkSurface.
	kindCloseRequest
	kindActive
	kindScale
	kindState
	// The caption gestures of a surface (chrome.go, S4.1h); their peer is
	// the *surface: GtkGestureClick::pressed (sigIntDoubleDouble), and
	// GtkGestureDrag::drag-begin and ::drag-update (sigDoubleDouble).
	kindCaptionPress
	kindCaptionDragBegin
	kindCaptionDragUpdate
	// The pointer controllers of a surface (input.go, S4.1i); their peer is
	// the *surface: GtkEventControllerMotion::enter and ::motion
	// (sigDoubleDouble) and ::leave (sigVoid), GtkGestureClick::pressed and
	// ::released (sigIntDoubleDouble) and ::cancel (sigPointer).
	kindPointerEnter
	kindPointerMotion
	kindPointerLeave
	kindPointerPressed
	kindPointerReleased
	kindPointerCancel
	// The scroll controller of a surface (scroll.go, S4.1j); its peer is the
	// *surface: GtkEventControllerScroll::scroll (sigBoolDoubleDouble),
	// ::scroll-begin and ::scroll-end (sigVoid) and ::decelerate
	// (sigDoubleDouble).
	kindScroll
	kindScrollBegin
	kindScrollEnd
	kindScrollDecelerate
	// The keyboard of a surface (keyboard.go, S4.1k); its peer is the
	// *surface: GtkEventControllerKey::key-pressed (sigBoolUintUintUint) and
	// ::key-released (sigUintUintUint), GtkIMContext::commit (sigPointer).
	kindKeyPressed
	kindKeyReleased
	kindIMCommit
	// kindSettingNotify is notify::<name> of the default GtkSettings
	// (gtksettings.go, S4.3d), through sigPointer; its peer is the
	// *gtkSettings.
	kindSettingNotify
)

// entry is what a handle in the dispatch table stands for: the Go peer
// that handles the callback and which of its callbacks it is.
type entry struct {
	peer any
	kind kind
}

// dispatch maps the handles in user_data and in GGSurface handle words to
// their entries. The trampolines are process-wide, so the table is too. It
// is used on the GTK thread (ffi.Handles stays consistent if a stray
// destroy notify comes from another thread all the same).
var dispatch ffi.Handles[entry]

// The peer interfaces, one per trampoline signature. A trampoline asserts
// its entry's peer to its interface; a peer connected through the wrong
// trampoline panics there, and the trampoline captures that like any other
// panic of Go code.

// boolSignalPeer handles signals of the C signature
// gboolean (*)(gpointer instance, gpointer user_data), such as
// GtkWindow::close-request.
type boolSignalPeer interface {
	boolSignal(k kind, instance uintptr) bool
}

// intDoubleDoublePeer handles signals of the C signature
// void (*)(gpointer instance, gint, gdouble, gdouble, gpointer user_data),
// such as GtkGestureClick::pressed and ::released.
type intDoubleDoublePeer interface {
	intDoubleDouble(k kind, instance uintptr, n int32, x, y float64)
}

// pointerSignalPeer handles signals of the C signature
// void (*)(gpointer instance, gpointer arg, gpointer user_data), such as
// GObject::notify (arg is the GParamSpec).
type pointerSignalPeer interface {
	pointerSignal(k kind, instance, arg uintptr)
}

// boolDoubleDoublePeer handles signals of the C signature
// gboolean (*)(gpointer instance, gdouble, gdouble, gpointer user_data),
// such as GtkEventControllerScroll::scroll.
type boolDoubleDoublePeer interface {
	boolDoubleDouble(k kind, instance uintptr, x, y float64) bool
}

// boolUintUintUintPeer handles signals of the C signature
// gboolean (*)(gpointer instance, guint, guint, guint, gpointer user_data),
// such as GtkEventControllerKey::key-pressed (keyval, keycode, state).
type boolUintUintUintPeer interface {
	boolUintUintUint(k kind, instance uintptr, a, b, c uint32) bool
}

// uintUintUintPeer handles signals of the C signature
// void (*)(gpointer instance, guint, guint, guint, gpointer user_data), such
// as GtkEventControllerKey::key-released.
type uintUintUintPeer interface {
	uintUintUint(k kind, instance uintptr, a, b, c uint32)
}

// voidSignalPeer handles signals of the C signature
// void (*)(gpointer instance, gpointer user_data), such as
// GtkEventControllerMotion::leave.
type voidSignalPeer interface {
	voidSignal(k kind, instance uintptr)
}

// doubleDoublePeer handles signals of the C signature
// void (*)(gpointer instance, gdouble, gdouble, gpointer user_data), such
// as GtkGestureDrag::drag-begin and ::drag-update.
type doubleDoublePeer interface {
	doubleDouble(k kind, instance uintptr, x, y float64)
}

// sourcePeer handles the GSourceFunc, gboolean (*)(gpointer user_data), of
// an idle or timeout source. It returns whether the source stays.
type sourcePeer interface {
	source(k kind) bool
}

// sigBool is the trampoline of every gboolean (gpointer, gpointer) signal.
// A stale handle returns FALSE. A panic returns the kind's default, set
// before the peer runs (the named result survives trap's recover): TRUE
// for close-request, so GTK never destroys a surface's window behind its
// back; FALSE otherwise.
func sigBool(instance, data uintptr) (r int32) {
	defer trap(nameSigBool)
	e, ok := dispatch.Get(ffi.Handle(data))
	if !ok {
		return gtk.False
	}
	r = gtk.False
	if e.kind == kindCloseRequest {
		r = gtk.True
	}
	if e.peer.(boolSignalPeer).boolSignal(e.kind, instance) {
		return gtk.True
	}
	return gtk.False
}

// sigIntDoubleDouble is the trampoline of every
// void (gpointer, gint, gdouble, gdouble, gpointer) signal.
func sigIntDoubleDouble(instance uintptr, n int32, x, y float64, data uintptr) {
	defer trap(nameSigIntDoubleDouble)
	e, ok := dispatch.Get(ffi.Handle(data))
	if !ok {
		return
	}
	e.peer.(intDoubleDoublePeer).intDoubleDouble(e.kind, instance, n, x, y)
}

// sigPointer is the trampoline of every void (gpointer, gpointer, gpointer)
// signal.
func sigPointer(instance, arg, data uintptr) {
	defer trap(nameSigPointer)
	e, ok := dispatch.Get(ffi.Handle(data))
	if !ok {
		return
	}
	e.peer.(pointerSignalPeer).pointerSignal(e.kind, instance, arg)
}

// sigDoubleDouble is the trampoline of every
// void (gpointer, gdouble, gdouble, gpointer) signal.
func sigDoubleDouble(instance uintptr, x, y float64, data uintptr) {
	defer trap(nameSigDoubleDouble)
	e, ok := dispatch.Get(ffi.Handle(data))
	if !ok {
		return
	}
	e.peer.(doubleDoublePeer).doubleDouble(e.kind, instance, x, y)
}

// sourceFunc is the GSourceFunc of every idle and timeout source; data is
// the handle of its entry. g_idle_add has no destroy notify, so the
// handle is freed here when the source goes: when the peer returns false
// or panics. A stale handle removes the source. Whoever removes a source
// itself (g_source_remove) frees its handle too.
func sourceFunc(data uintptr) int32 {
	defer trap(nameSourceFunc)
	h := ffi.Handle(data)
	e, ok := dispatch.Get(h)
	if !ok {
		return gtk.SourceRemove
	}
	keep := false
	defer func() { // runs before trap, also when the peer panics
		if !keep {
			dispatch.Free(h)
		}
	}()
	keep = e.peer.(sourcePeer).source(e.kind)
	if keep {
		return gtk.SourceContinue
	}
	return gtk.SourceRemove
}

// closureNotify is the GClosureNotify, void (gpointer data, GClosure *),
// of every connected signal handler: GLib calls it once the handler is
// gone (disconnected, or its instance finalized), and it frees the handle
// in data. Freeing a handle that is no longer live does nothing.
func closureNotify(data, _ uintptr) {
	defer trap(nameClosureNotify)
	dispatch.Free(ffi.Handle(data))
}

// tickPeer handles a GtkTickCallback,
// gboolean (*)(GtkWidget *, GdkFrameClock *, gpointer user_data); h is the
// callback's handle. It returns whether the callback stays.
type tickPeer interface {
	tick(k kind, h ffi.Handle, frameClock uintptr) bool
}

// tickGonePeer is told by destroyNotify that GTK dropped the callback
// whose handle is h (just before the handle is freed): it returned
// G_SOURCE_REMOVE, it was removed, or its widget was destroyed.
type tickGonePeer interface {
	tickGone(k kind, h ffi.Handle)
}

// tickCallback is the GtkTickCallback of every tick callback; data is the
// handle of its entry, freed by destroyNotify when GTK drops the callback.
// A stale handle or a panic returns G_SOURCE_REMOVE.
func tickCallback(_, frameClock, data uintptr) int32 {
	defer trap(nameTick)
	h := ffi.Handle(data)
	e, ok := dispatch.Get(h)
	if !ok {
		return gtk.SourceRemove
	}
	if e.peer.(tickPeer).tick(e.kind, h, frameClock) {
		return gtk.SourceContinue
	}
	return gtk.SourceRemove
}

// destroyNotify is the GDestroyNotify, void (gpointer data), of every
// user_data that GTK frees itself (the tick callbacks): it tells a
// tickGonePeer, then frees the handle in data. Freeing a handle that is
// no longer live does nothing. The handle is freed even if the peer
// panics.
func destroyNotify(data uintptr) {
	defer trap(nameDestroyNotify)
	h := ffi.Handle(data)
	defer dispatch.Free(h)
	if e, ok := dispatch.Get(h); ok {
		if p, ok := e.peer.(tickGonePeer); ok {
			p.tickGone(e.kind, h)
		}
	}
}

// sigVoid is the trampoline of every void (gpointer, gpointer) signal.
func sigVoid(instance, data uintptr) {
	defer trap(nameSigVoid)
	e, ok := dispatch.Get(ffi.Handle(data))
	if !ok {
		return
	}
	e.peer.(voidSignalPeer).voidSignal(e.kind, instance)
}

// sigBoolDoubleDouble is the trampoline of every
// gboolean (gpointer, gdouble, gdouble, gpointer) signal. A stale handle
// or a panic returns FALSE.
func sigBoolDoubleDouble(instance uintptr, x, y float64, data uintptr) (r int32) {
	defer trap(nameSigBoolDoubleDouble)
	e, ok := dispatch.Get(ffi.Handle(data))
	if !ok {
		return gtk.False
	}
	if e.peer.(boolDoubleDoublePeer).boolDoubleDouble(e.kind, instance, x, y) {
		return gtk.True
	}
	return gtk.False
}

// sigBoolUintUintUint is the trampoline of every
// gboolean (gpointer, guint, guint, guint, gpointer) signal. A stale handle
// or a panic returns FALSE.
func sigBoolUintUintUint(instance uintptr, a, b, c uint32, data uintptr) (r int32) {
	defer trap(nameSigBoolUintUintUint)
	e, ok := dispatch.Get(ffi.Handle(data))
	if !ok {
		return gtk.False
	}
	if e.peer.(boolUintUintUintPeer).boolUintUintUint(e.kind, instance, a, b, c) {
		return gtk.True
	}
	return gtk.False
}

// sigUintUintUint is the trampoline of every
// void (gpointer, guint, guint, guint, gpointer) signal.
func sigUintUintUint(instance uintptr, a, b, c uint32, data uintptr) {
	defer trap(nameSigUintUintUint)
	e, ok := dispatch.Get(ffi.Handle(data))
	if !ok {
		return
	}
	e.peer.(uintUintUintPeer).uintUintUint(e.kind, instance, a, b, c)
}
