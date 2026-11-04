//go:build linux

package gtk4

import (
	"strings"
	"sync"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// registerTrampolines creates the trampoline set, once per process, and
// fills tramp. ffi.RegisterCallback panics on a second registration of a
// name, so a repeated call would fail loudly even without the Once.
var registerTrampolines = sync.OnceFunc(func() {
	for _, t := range trampolines() {
		*t.addr = ffi.RegisterCallback(t.name, t.fn)
	}
})

// connect connects the trampoline tr (a tramp field of the signal's C
// signature) to signal on the GObject instance, for peer's callback k. The
// handler's user_data is a new handle to {peer, k}, freed by closureNotify
// when GLib drops the handler. It returns the handler id, or 0 for a
// signal the instance's type does not have. That case is checked here
// with g_signal_lookup (a detail after "::" aside), because
// g_signal_connect_data would report it with g_critical, which
// G_DEBUG=fatal-criticals turns into an abort; no handle is kept.
func connect(instance uintptr, signal string, tr uintptr, peer any, k kind) uintptr {
	registerTrampolines()
	name, _, _ := strings.Cut(signal, "::")
	itype := ffi.ReadUintptr(ffi.ReadUintptr(instance, 0), 0) // G_TYPE_FROM_INSTANCE
	if gtk.GObject.SignalLookup(name, itype) == 0 {
		return 0
	}
	h := dispatch.New(entry{peer: peer, kind: k})
	id := gtk.GObject.SignalConnectData(instance, signal, tr, uintptr(h), tramp.closureNotify, 0)
	if id == 0 {
		dispatch.Free(h)
	}
	return id
}

// addIdle adds an idle source to the default main context that calls
// peer's source(k) until it returns false. It returns the source id, or 0
// if GLib refused it. sourceFunc frees the handle when the source goes; a
// caller that removes the source itself must free it (it gets the handle
// for that).
func addIdle(peer sourcePeer, k kind) (uint32, ffi.Handle) {
	registerTrampolines()
	h := dispatch.New(entry{peer: peer, kind: k})
	id := gtk.GLib.IdleAdd(tramp.sourceFunc, uintptr(h))
	if id == 0 {
		dispatch.Free(h)
		return 0, 0
	}
	return id, h
}

// addTimeout adds a timeout source to the default main context that calls
// peer's source(k) after ms milliseconds, and again every ms while it
// returns true. It returns the source id, or 0 if GLib refused it. As for
// addIdle, sourceFunc frees the handle when the source goes; a caller that
// removes the source itself frees it.
func addTimeout(ms uint32, peer sourcePeer, k kind) (uint32, ffi.Handle) {
	registerTrampolines()
	h := dispatch.New(entry{peer: peer, kind: k})
	id := gtk.GLib.TimeoutAdd(ms, tramp.sourceFunc, uintptr(h))
	if id == 0 {
		dispatch.Free(h)
		return 0, 0
	}
	return id, h
}

// addTick adds a tick callback to widget that calls peer's tick(k, …) at
// every frame of the widget's frame clock until it returns false. The
// handle in user_data is freed by destroyNotify, which GTK calls when it
// drops the callback (returned false, removed, or the widget destroyed).
// It returns the callback id and its handle, or 0, 0 if GTK returned no
// id (no handle kept).
func addTick(widget uintptr, peer tickPeer, k kind) (uint32, ffi.Handle) {
	registerTrampolines()
	h := dispatch.New(entry{peer: peer, kind: k})
	id := gtk.Gtk.WidgetAddTickCallback(widget, tramp.tick, uintptr(h), tramp.destroyNotify)
	if id == 0 {
		dispatch.Free(h)
		return 0, 0
	}
	return id, h
}
