package gtk4

// The trampoline set (PLAN.md §7.2): every native callback of the package,
// created once per process by registerTrampolines (trampolines_linux.go)
// and never per widget or per signal. Signal handlers are named by their
// C signature (without the leading instance and the trailing user_data),
// not by a signal: GtkGestureClick's "pressed" and "released" share
// nameSigIntDoubleDouble. The vfuncs and class_init belong to the
// GGSurface class (class.go).
const (
	nameClassInit           = "gtk4.GGSurface.class_init"
	nameSizeAllocate        = "gtk4.GtkWidgetClass.size_allocate"
	nameMeasure             = "gtk4.GtkWidgetClass.measure"
	nameSnapshot            = "gtk4.GtkWidgetClass.snapshot"
	nameClosureNotify       = "gtk4.GClosureNotify"
	nameSourceFunc          = "gtk4.GSourceFunc"
	nameSigBool             = "gtk4.signal gboolean()"
	nameSigIntDoubleDouble  = "gtk4.signal void(gint, gdouble, gdouble)"
	nameSigPointer          = "gtk4.signal void(gpointer)"
	nameSigDoubleDouble     = "gtk4.signal void(gdouble, gdouble)"
	nameSigVoid             = "gtk4.signal void()"
	nameSigBoolDoubleDouble = "gtk4.signal gboolean(gdouble, gdouble)"
	nameSigBoolUintUintUint = "gtk4.signal gboolean(guint, guint, guint)"
	nameSigUintUintUint     = "gtk4.signal void(guint, guint, guint)"
	nameTick                = "gtk4.GtkTickCallback"
	nameDestroyNotify       = "gtk4.GDestroyNotify"
	nameAsyncReady          = "gtk4.GAsyncReadyCallback"
)

// numTrampolines is how many native callbacks the package creates. A
// test pins ffi.Callbacks() to it (S4.1c), so a step that adds a
// trampoline changes it on purpose (S4.1e: 8 → 10, the tick callback and
// the GDestroyNotify; S4.1f: 11, the notify signals' void(gpointer);
// S4.1h: 12, the drag gesture's void(gdouble, gdouble); S4.1i: 13, the
// motion controller's void() leave; S4.1j: 14, the scroll controller's
// gboolean(gdouble, gdouble) scroll; S4.1k: 16, the key controller's
// gboolean(guint, guint, guint) key-pressed and void(guint, guint, guint)
// key-released; S5.2b: 17, the clipboard read's one-shot
// GAsyncReadyCallback).
const numTrampolines = 17

// tramp holds the C function pointers of the trampolines, zero until
// registerTrampolines has run.
var tramp struct {
	classInit, sizeAllocate, measure, snapshot    uintptr
	closureNotify, sourceFunc                     uintptr
	sigBool, sigIntDoubleDouble, sigPointer       uintptr
	sigDoubleDouble, sigVoid, sigBoolDoubleDouble uintptr
	sigBoolUintUintUint, sigUintUintUint          uintptr
	tick, destroyNotify, asyncReady               uintptr
}

// trampoline is one native callback: its registry name, its Go function
// and the field of tramp that receives its C function pointer.
type trampoline struct {
	name string
	fn   any
	addr *uintptr
}

// trampolines returns the set in registration order.
func trampolines() []trampoline {
	return []trampoline{
		{nameClassInit, surfaceClassInit, &tramp.classInit},
		{nameSizeAllocate, surfaceSizeAllocate, &tramp.sizeAllocate},
		{nameMeasure, surfaceMeasure, &tramp.measure},
		{nameSnapshot, surfaceSnapshot, &tramp.snapshot},
		{nameClosureNotify, closureNotify, &tramp.closureNotify},
		{nameSourceFunc, sourceFunc, &tramp.sourceFunc},
		{nameSigBool, sigBool, &tramp.sigBool},
		{nameSigIntDoubleDouble, sigIntDoubleDouble, &tramp.sigIntDoubleDouble},
		{nameSigPointer, sigPointer, &tramp.sigPointer},
		{nameSigDoubleDouble, sigDoubleDouble, &tramp.sigDoubleDouble},
		{nameSigVoid, sigVoid, &tramp.sigVoid},
		{nameSigBoolDoubleDouble, sigBoolDoubleDouble, &tramp.sigBoolDoubleDouble},
		{nameSigBoolUintUintUint, sigBoolUintUintUint, &tramp.sigBoolUintUintUint},
		{nameSigUintUintUint, sigUintUintUint, &tramp.sigUintUintUint},
		{nameTick, tickCallback, &tramp.tick},
		{nameDestroyNotify, destroyNotify, &tramp.destroyNotify},
		{nameAsyncReady, asyncReady, &tramp.asyncReady},
	}
}
