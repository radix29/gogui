package gtk4

import (
	"fmt"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// GGSurface is the shell's GtkWidget subclass, from spike B (S0.9b): the
// widget the aqua engine draws into. Its class_init writes three vfunc
// slots of GtkWidgetClass (size_allocate, measure, snapshot); every
// instance carries an ffi.Handle into the dispatch table in one extra word
// after the GtkWidget instance struct. The class is registered by
// registerSurfaceClass (class_linux.go).

// GtkWidgetClass on LP64 (gtkwidget.h, unchanged since GTK 4.0): a
// GObjectClass (136 bytes), 25 vfunc pointers, priv and padding[8]. The
// offsets are of the three slots GGSurface overrides. checkWidgetType
// refuses a GTK whose class size differs, so a different ABI can't make
// class_init write into the wrong slot (PLAN.md §6.3).
const (
	widgetClassSize  = 408
	slotSizeAllocate = 200
	slotMeasure      = 232
	slotSnapshot     = 320
	wordSize         = 8
)

// surfaceCSSName is the CSS name of GGSurface. A name no theme knows gets
// no margins, borders or padding, so gtk_widget_measure reports exactly
// what measure wrote and the allocation is the whole widget.
const surfaceCSSName = "ggsurface"

// surfacePeer is the Go side of a GGSurface instance, the peer of its
// kindSurface entry.
type surfacePeer interface {
	// measure returns the minimum and natural size along orientation
	// (gtk.OrientationHorizontal or Vertical) for the given size along the
	// other one; forSize is -1 when GTK does not know it.
	measure(orientation, forSize int32) (minimum, natural int32)
	// sizeAllocate tells the new allocation, in logical pixels; baseline is
	// -1 for none.
	sizeAllocate(width, height, baseline int32)
	// snapshot appends the surface's render nodes to the GtkSnapshot s.
	snapshot(s uintptr)
}

// class is what registerSurfaceClass learned about GtkWidget and set up
// for GGSurface. It is written once, on the GTK thread, before the first
// instance exists.
var class struct {
	widgetType  uintptr // GtkWidget
	surfaceType uintptr // GGSurface; 0 until registered
	// handleOff is the offset of the handle word in a GGSurface instance:
	// sizeof(GtkWidget) rounded up to a word. 0 until registered, which
	// makes every instance peerless.
	handleOff uintptr
	// parentSnapshot is GtkWidget's own snapshot vfunc, which GGSurface's
	// chains to (it snapshots the widget's children); nil if the slot is
	// NULL.
	parentSnapshot func(w, s uintptr)
}

// checkWidgetType checks what g_type_query said about GtkWidget and
// returns the offset of GGSurface's handle word.
func checkWidgetType(q gtk.TypeQuery) (handleOff uintptr, err error) {
	switch {
	case q.Type == gtk.TypeInvalid:
		return 0, fmt.Errorf("gtk4: g_type_query(GtkWidget) found no type")
	case q.ClassSize != widgetClassSize:
		return 0, fmt.Errorf("gtk4: sizeof(GtkWidgetClass) = %d, want %d: the vfunc slot offsets don't match this GTK; refusing to subclass",
			q.ClassSize, widgetClassSize)
	case q.InstanceSize == 0:
		return 0, fmt.Errorf("gtk4: sizeof(GtkWidget) = 0")
	}
	return (uintptr(q.InstanceSize) + wordSize - 1) &^ (wordSize - 1), nil
}

// surfaceHandle returns the handle word of the GGSurface instance w, or 0
// before the class is registered.
func surfaceHandle(w uintptr) ffi.Handle {
	if class.handleOff == 0 || w == 0 {
		return 0
	}
	return ffi.Handle(ffi.ReadUintptr(w, class.handleOff))
}

// setSurfaceHandle stores h in w's handle word.
func setSurfaceHandle(w uintptr, h ffi.Handle) {
	ffi.WriteUintptr(w, class.handleOff, uintptr(h))
}

// surfacePeerOf returns the peer of the GGSurface instance w. An instance
// without a handle, with a stale one, or with one of another kind has none.
func surfacePeerOf(w uintptr) (surfacePeer, bool) {
	e, ok := dispatch.Get(surfaceHandle(w))
	if !ok || e.kind != kindSurface {
		return nil, false
	}
	return e.peer.(surfacePeer), true
}

// surfaceClassInit is GGSurface's class_init,
// void (gpointer g_class, gpointer class_data). GLib has copied
// GtkWidgetClass into the new class, so only the overridden slots change.
func surfaceClassInit(klass, _ uintptr) {
	defer trap(nameClassInit)
	ffi.WriteUintptr(klass, slotSizeAllocate, tramp.sizeAllocate)
	ffi.WriteUintptr(klass, slotMeasure, tramp.measure)
	ffi.WriteUintptr(klass, slotSnapshot, tramp.snapshot)
	gtk.Gtk.WidgetClassSetCSSName(klass, surfaceCSSName)
}

// surfaceSizeAllocate is the size_allocate vfunc,
// void (GtkWidget *, int width, int height, int baseline).
func surfaceSizeAllocate(w uintptr, width, height, baseline int32) {
	defer trap(nameSizeAllocate)
	if p, ok := surfacePeerOf(w); ok {
		p.sizeAllocate(width, height, baseline)
	}
}

// surfaceMeasure is the measure vfunc, void (GtkWidget *, GtkOrientation,
// int for_size, int *minimum, int *natural, int *minimum_baseline,
// int *natural_baseline). It asks the peer for the sizes at for_size and
// writes them through the int pointers, 32 bits each; any pointer may be
// NULL. The baselines are -1 (none). Without a peer, or when the peer
// panics, the sizes are 0: the results are written in every case, so GTK
// never reads uninitialised ints.
func surfaceMeasure(w uintptr, orientation, forSize int32, minimum, natural, minBaseline, natBaseline uintptr) {
	defer trap(nameMeasure)
	var mn, nt int32
	func() {
		defer trap(nameMeasure)
		if p, ok := surfacePeerOf(w); ok {
			mn, nt = p.measure(orientation, forSize)
		}
	}()
	for _, out := range []struct {
		p uintptr
		v int32
	}{{minimum, mn}, {natural, nt}, {minBaseline, -1}, {natBaseline, -1}} {
		if out.p != 0 {
			ffi.WriteInt32(out.p, 0, out.v)
		}
	}
}

// surfaceSnapshot is the snapshot vfunc, void (GtkWidget *, GtkSnapshot *).
// The peer draws first; then it chains to GtkWidget's snapshot, which
// draws the widget's children on top (none until menus or text views need
// them). The parent runs even if the peer panicked.
func surfaceSnapshot(w, s uintptr) {
	defer trap(nameSnapshot)
	func() {
		defer trap(nameSnapshot)
		if p, ok := surfacePeerOf(w); ok {
			p.snapshot(s)
		}
	}()
	if f := class.parentSnapshot; f != nil {
		f(w, s)
	}
}
