// Package gtk holds the raw bindings of the gtk4 shell (PLAN.md S4.1b):
// GTK 4 (with GDK and GSK, which live inside libgtk-4), GObject, GLib,
// Graphene and Pango, dlopen'd by soname at run time through
// internal/ffi and purego. There is no cgo and no link-time dependency;
// a machine without GTK 4 gets a clear error from Load and Report.
//
// Only Load (load_linux.go), which opens the libraries, is linux-only.
// The binding tables (bind.go), the C struct mirrors, the constants, the
// version check and the "GTK missing" report are untagged, so their tests
// run on every host. The mirrors assume a 64-bit target (LP64), which
// every gogui target is.
//
// No variadic C function is bound (PLAN.md S0.9): g_object_new,
// g_signal_connect and g_signal_emit_by_name are replaced by
// g_object_new_with_properties, g_signal_connect_data and g_signal_emitv.
// Function-pointer arguments are uintptr (trampolines from
// ffi.RegisterCallback); ffi.Bind rejects Go func arguments.
//
// Every function must be called on the GTK thread (the one that called
// gtk_init_check), except GLib.IdleAdd, which GLib documents as
// thread-safe.
package gtk
