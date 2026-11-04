// Package gtk4 is the Linux shell (PLAN.md S4.1, S4.2): GTK 4 windows, input and
// GSK drawing for the aqua driver, called through the raw bindings of
// internal/shell/gtk4/gtk. It was built step by step (S4.1c–S4.1l, then
// the canvas and the text system in S4.2a–S4.2k): since
// S4.1c it holds the native callbacks and the GGSurface widget class, since
// S4.1d the Shell (shell.go, wake.go): Run over gtk_init_check and a
// GMainLoop (loop_linux.go), Post with one g_idle_add per drain, Quit and
// the teardown; since S4.1e the frame clock (clock.go): GDK tick
// callbacks on the pacing surface, a g_timeout_add fallback without one;
// since S4.1f the surfaces (surface.go over window_linux.go): a GtkWindow
// with client-side decorations and a hidden title bar around a GGSurface;
// since S4.1g the canvas (canvas.go over canvas_linux.go): rounded rects
// (gradient strokes through a GskPath stroke push since S4.2b), paths
// (GskPath fills and strokes since S4.2c), shadows (outset and inset
// shadow nodes since S4.2d), images (scaled texture nodes of the shell's
// GdkTextures since S4.2e, image.go), backdrop blurs (by segments, child
// snapshots re-appended under a blur push, since S4.2f, blur.go), clips,
// transforms and opacity into the GtkSnapshot, and text (Pango layouts
// rebuilt once per layout and paint, gtk_snapshot_append_layout, since
// S4.2i);
// since S4.1h the chrome regions (chrome.go): a drag on RegionCaption moves
// the window (gdk_toplevel_begin_move), a double-click there zooms it;
// since S4.1i pointer input (input.go): a motion controller and a click
// gesture for every button on the GGSurface; since S4.1j scroll input
// (scroll.go): a kinetic scroll controller, with the momentum after a
// touchpad gesture synthesized on the frame clock; since S4.1k keyboard
// input (keyboard.go): a key controller on the window and GTK's simple
// input method; since S4.3a the appearance mapping (portal.go):
// readAppearance over the settings portal's values, with the GtkSettings
// filling what it lacks; since S4.3b the portal client (portalclient.go):
// the org.freedesktop.appearance values over godbus, kept current by
// SettingChanged; since S4.3c the Shell's appearance cache: read from an
// injectable source before Ready, re-read on its changes through Post;
// since S4.3d the live GtkSettings (gtksettings.go): notify:: of the
// default GtkSettings through sigPointer, re-read the same way.
//
// The Shell is untagged and drives the native side through the small loop
// interface, the surfaces through the window interface, so their logic
// (the ready guard, the wake, Quit, the teardown on every exit, panics
// included; sizes, lifecycle, painting, the caption, the pointer,
// scrolling and the keyboard) is tested on every host with fakes; New,
// the GTK loop, the GTK window and the GtkSnapshot calls are linux only.
// The text system is Pango (text.go, S4.2g; text_linux.go), measured once
// per layout and kept as Go data, the line and cluster geometry for
// hit-testing, carets and selection included (textgeom.go, S4.2h); its UI
// font is Inter from assets/fonts, added through a cache file for
// fontconfig (appfont.go, S4.2j), else the system's; without
// Pango it is shell/fake's.
//
// Callbacks (PLAN.md §6.3, §7.2). Every native callback is one of a fixed
// set of trampolines (trampolines.go), registered once per process with
// ffi.RegisterCallback:
//
//   - the GGSurface vfuncs (class_init, size_allocate, measure, snapshot),
//     which find their Go peer through the handle word in the instance;
//   - the signal handlers and the GSourceFunc, one per C signature, not one
//     per signal, which find theirs through user_data: an ffi.Handle into
//     the dispatch table of {peer, kind} entries (dispatch.go). The kind
//     tells one peer's callbacks of the same signature apart;
//   - the shared GClosureNotify, which frees a signal handler's handle when
//     GLib drops the handler (disconnected, or its instance finalized);
//   - the GtkTickCallback of the frame clock, and the shared GDestroyNotify,
//     which frees a tick callback's handle when GTK drops the callback.
//
// Every trampoline recovers a panic and keeps it, the first one winning,
// until Go code re-raises it with rethrow after the native call that led
// to it has returned (panic.go), as shell/win32's window procedure does: a
// panic never unwinds through GTK's C frames.
//
// The dispatch table, the panic capture, the trampolines' Go side and the
// class constants have no build tag, so their tests run on every host.
// What calls GTK (registering the callbacks and the class, connecting
// signals, creating instances) is in *_linux.go files; those tests need
// GTK and a display (native.sh runs them under xvfb). Everything runs on
// the GTK thread.
package gtk4
