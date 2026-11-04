//go:build linux

package gtk4

import (
	"fmt"
	"sync"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// surfaceTypeName is the GType name of the class.
const surfaceTypeName = "GGSurface"

// registerSurfaceClass registers GGSurface as a subclass of GtkWidget, once
// per process, and returns the first result to every caller. It needs a
// successful gtk.Load and runs after gtk_init_check on the GTK thread. It
// checks GtkWidgetClass's size, takes GtkWidget's own snapshot vfunc for
// the chain-up, registers the type, then forces class_init
// (g_type_class_ref) and reads the three slots back, so a class that
// GTK would not call through fails here and not at the first draw.
var registerSurfaceClass = sync.OnceValue(registerClass)

func registerClass() error {
	registerTrampolines()
	wt := gtk.Gtk.WidgetGetType()
	var q gtk.TypeQuery
	gtk.GObject.TypeQuery(wt, &q)
	off, err := checkWidgetType(q)
	if err != nil {
		return err
	}
	// g_type_register_static_simple would only print a warning and return
	// 0 for a name that is taken.
	if t := gtk.GObject.TypeFromName(surfaceTypeName); t != 0 {
		return fmt.Errorf("gtk4: the type name %s is already registered (GType %#x)", surfaceTypeName, t)
	}
	parent := gtk.GObject.TypeClassRef(wt) // never unreffed: GtkWidget's class lives as long as GTK
	if parent == 0 {
		return fmt.Errorf("gtk4: g_type_class_ref(GtkWidget) returned NULL")
	}
	var parentSnapshot func(w, s uintptr)
	if addr := ffi.ReadUintptr(parent, slotSnapshot); addr != 0 {
		if err := ffi.BindAddr(&parentSnapshot, addr); err != nil {
			return fmt.Errorf("gtk4: GtkWidget's snapshot vfunc: %w", err)
		}
	}
	st := gtk.GObject.TypeRegisterStaticSimple(wt, surfaceTypeName, widgetClassSize,
		tramp.classInit, uint32(off+wordSize), 0, 0)
	if st == 0 {
		return fmt.Errorf("gtk4: g_type_register_static_simple(%s) returned G_TYPE_INVALID", surfaceTypeName)
	}
	class.widgetType, class.surfaceType, class.handleOff = wt, st, off
	class.parentSnapshot = parentSnapshot

	klass := gtk.GObject.TypeClassRef(st) // runs class_init; kept for the life of the process
	rethrow()
	if klass == 0 {
		return fmt.Errorf("gtk4: g_type_class_ref(%s) returned NULL", surfaceTypeName)
	}
	for _, s := range []struct {
		name      string
		off, want uintptr
	}{
		{"size_allocate", slotSizeAllocate, tramp.sizeAllocate},
		{"measure", slotMeasure, tramp.measure},
		{"snapshot", slotSnapshot, tramp.snapshot},
	} {
		if got := ffi.ReadUintptr(klass, s.off); got != s.want {
			return fmt.Errorf("gtk4: after class_init the %s slot of %s is %#x, want the trampoline %#x",
				s.name, surfaceTypeName, got, s.want)
		}
	}
	return nil
}

// newSurfaceWidget creates a GGSurface whose handle word points to a new
// kindSurface entry for p (none if p is nil). The floating reference is
// sunk, so the caller owns one reference and releases it with
// releaseSurfaceWidget. It returns 0, and keeps no handle, before the
// class is registered or if GLib returns no instance.
func newSurfaceWidget(p surfacePeer) uintptr {
	if class.surfaceType == 0 {
		return 0
	}
	w := gtk.GObject.ObjectNewWithProperties(class.surfaceType, 0, 0, 0)
	if w == 0 {
		return 0
	}
	w = gtk.GObject.ObjectRefSink(w)
	if p != nil {
		setSurfaceHandle(w, dispatch.New(entry{peer: p, kind: kindSurface}))
	}
	return w
}

// releaseSurfaceWidget frees w's handle, so a late vfunc call finds no
// peer, and drops the caller's reference. A 0 w does nothing.
func releaseSurfaceWidget(w uintptr) {
	if w == 0 {
		return
	}
	dispatch.Free(surfaceHandle(w))
	setSurfaceHandle(w, 0)
	gtk.GObject.ObjectUnref(w)
}
