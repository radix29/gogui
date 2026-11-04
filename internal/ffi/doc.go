// Package ffi holds the helpers that every native binding shares
// (PLAN.md §7.2):
//
//   - the handle table (Handles), which maps an integer Handle stored in
//     native memory (an ObjC ivar, a GObject user_data pointer,
//     GWLP_USERDATA) back to its Go peer. Go pointers are never stored in
//     native memory; native objects carry a Handle instead;
//   - the fixed trampoline registry (RegisterCallback, Callbacks,
//     MaxCallbacks): every native callback is created once by name at
//     start-up, a second registration of a name panics, and signatures are
//     checked up front. On Windows it uses syscall.NewCallback; on darwin
//     and linux purego.NewCallback. The budget is 1024 callbacks on every
//     OS, which leaves room below purego's 2000 for purego/objc's own, and
//     nothing outside this package may create callbacks (scripts/check.sh
//     enforces it);
//   - struct-layout test helpers (CheckLayout, CheckNoGoPointers) that
//     compare a Go mirror of a C struct with constants from the headers;
//   - on darwin and linux, shared libraries through purego (Dlopen, Lib
//     with Sym, MustSym, Bind and Close; BindAddr for a bare function
//     pointer such as a vfunc slot; both reject Go func arguments, for
//     which purego would create a new callback on every call) and C
//     strings in libc memory (CString, Free, GoString, GoStringN);
//   - raw access to native memory: words (ReadUintptr, WriteUintptr), for a
//     handle in an ObjC ivar or a vfunc slot in a GObject class struct, and
//     32-bit C ints (ReadInt32, WriteInt32), for int fields and int
//     out-parameters such as those of GtkWidgetClass.measure. The address
//     must never be Go memory. This works on every OS;
//   - At[T], which turns the address of a native struct (a Win32 lParam
//     pointing at a MINMAXINFO, NCCALCSIZE_PARAMS, …) into a *T, so the
//     drivers need no uintptr→unsafe.Pointer conversion of their own;
//   - CopyBytes, which copies a native buffer (a mapped staging texture,
//     S2.4b) into a Go slice.
//
// purego is imported only from darwin and linux files; the Windows build
// uses the standard library alone.
package ffi
