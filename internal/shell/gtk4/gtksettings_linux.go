//go:build linux

package gtk4

import (
	"unsafe"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// nativeGtkProps is the gtkProps over the default GtkSettings (S4.3d). It
// holds a reference to the GtkSettings from newNativeGtkProps to unwatch,
// so the handlers' instance stays valid however long the display lives.
type nativeGtkProps struct {
	st    uintptr   // GtkSettings *, 0 without one or after unwatch
	conns []uintptr // the notify handler ids
}

// newNativeGtkProps returns the props of the default GtkSettings (none
// without a default display). It runs on the GTK thread after
// gtk_init_check.
func newNativeGtkProps() gtkProps {
	n := &nativeGtkProps{st: gtk.Gtk.SettingsGetDefault()}
	if n.st != 0 {
		gtk.GObject.ObjectRef(n.st)
	}
	return n
}

// has reports whether the GtkSettings have the property name (looked up
// in the instance's class, G_OBJECT_GET_CLASS).
func (n *nativeGtkProps) has(name string) bool {
	return n.st != 0 && gtk.GObject.ObjectClassFindProperty(ffi.ReadUintptr(n.st, 0), name) != 0
}

// boolProp reads a boolean property through a GValue in g_malloc memory
// (GLib writes it), as settingInt does.
func (n *nativeGtkProps) boolProp(name string) (bool, bool) {
	if !n.has(name) {
		return false, false
	}
	v := gtk.GLib.Malloc0(unsafe.Sizeof(gtk.GValue{}))
	defer gtk.GLib.Free(v)
	gtk.GObject.ValueInit(v, gtk.TypeBoolean)
	defer gtk.GObject.ValueUnset(v)
	gtk.GObject.ObjectGetProperty(n.st, name, v)
	return gtk.GObject.ValueGetBoolean(v) != gtk.False, true
}

// intProp reads an int property as boolProp does.
func (n *nativeGtkProps) intProp(name string) (int32, bool) {
	if !n.has(name) {
		return 0, false
	}
	v := gtk.GLib.Malloc0(unsafe.Sizeof(gtk.GValue{}))
	defer gtk.GLib.Free(v)
	gtk.GObject.ValueInit(v, gtk.TypeInt)
	defer gtk.GObject.ValueUnset(v)
	gtk.GObject.ObjectGetProperty(n.st, name, v)
	return gtk.GObject.ValueGetInt(v), true
}

func (n *nativeGtkProps) watch(name string, p pointerSignalPeer, k kind) bool {
	if !n.has(name) {
		return false
	}
	id := connect(n.st, "notify::"+name, tramp.sigPointer, p, k)
	if id != 0 {
		n.conns = append(n.conns, id)
	}
	return id != 0
}

func (n *nativeGtkProps) unwatch() {
	if n.st == 0 {
		return
	}
	for _, id := range n.conns {
		gtk.GObject.SignalHandlerDisconnect(n.st, id) // closureNotify frees its handle
	}
	n.conns = nil
	gtk.GObject.ObjectUnref(n.st)
	n.st = 0
}
