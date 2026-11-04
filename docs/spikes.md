# Phase 0.5 spike findings

This file collects what the three purego spikes of phase 0.5 proved (PLAN.md §11.1,
S0.8–S0.10): the numbers, what failed or surprised, and what the drivers inherit (S1.1
for AppKit, S4.1 for GTK). The spikes themselves are throw-away code in `spike/` and are
deleted by the steps that replace them (`spike/gtk` by S4.1l; `spike/appkit` stays until S1.1).

**State (2026-10-13).** Everything here was designed, built, vetted and linted for all
six targets on the Windows host, and the Windows half of S0.10 ran there. The runs on
macOS and Linux have **not** come back yet. Every finding below is marked:

* ✔ **measured**: ran and passed (so far only on windows/amd64);
* ◐ **built, run pending**: the code and its self-test or test exist and compile for the
  target, but have not run there yet;
* ○ **open**: the number or answer only comes from a run that hasn't happened.

The gaps are collected in "Open runs" at the end. S0.10c_2a (2026-10-13) re-ran the
host's log and recorded the other five targets as "not run (no machine)"; S0.10c_2b
fills in what the other `scripts/native.sh` logs answer (and re-rates R-purego, PLAN.md §13), S0.10c_3 the two
manual checks; S0.10, and with it phase 0.5, gets its ✅ only after both. There is no CI
(dropped 2026-10-13, PLAN.md §8.4): the user runs `bash scripts/native.sh` on each
target and brings back `native-logs/<goos>-<goarch>.log`.

Toolchain: Go 1.26.4, purego v0.11.1, `CGO_ENABLED=0` everywhere.

---

## S0.8 Spike A: macOS (`spike/appkit`)

A window with a label and a button whose target is a registered `NSObject` subclass, in
a flipped `NSView` content view, `NSRect` passing, and the UI-thread wake. Checked by
`go run ./spike/appkit -selftest` (two phases: ten runtime checks without a run loop,
then a live `[NSApp run]`) on an arm64 and an amd64 Mac (`scripts/native.sh`), and
by the user looking at the window on both Macs.

**Proven by design and build (◐ until the Mac runs pass):**

* *Three trampolines cover an app.* `appkit.void(id,SEL,id)` (button action,
  `windowDidResize:`, `windowWillClose:`, `tick:`, `applicationDidFinishLaunching:`),
  `appkit.BOOL(id,SEL)` (`isFlipped`) and `appkit.BOOL(id,SEL,id)`
  (`applicationShouldTerminateAfterLastWindowClosed:`). Four classes and eight methods
  share them; each IMP dispatches through the handle in the class's `goHandle` ivar.
  `ffi.Callbacks()` is 3 for the whole spike.
* *Classes are built by hand, not with `objc.RegisterClass`.* `RegisterClass` creates a
  hidden purego callback per method and per ivar accessor, per class, which breaks the
  fixed trampoline set (PLAN.md §7.2) and isn't visible in `ffi.Callbacks()`. The spike
  binds `objc_allocateClassPair`, `class_addIvar` and `objc_registerClassPair` itself
  and adds methods with `Class.AddMethod` and a registry IMP. Since S0.10b
  `scripts/check.sh` fails on any `objc.RegisterClass`, `NewIMP` or `NewBlock` outside
  `internal/ffi`.
* *The handle ivar* is read and written at `self + ivar_getOffset` with
  `ffi.ReadUintptr`/`WriteUintptr`, so the spike has no `unsafe` conversion of its own.
* *No struct callbacks.* The registry rejects struct arguments, so there is no
  `drawRect:` or `setFrameSize:` override. Frames are computed in Go and recomputed in
  `windowDidResize:`; the self-test checks the button stays top-right after a resize.
* *Struct passing.* `NSRect` (32 bytes) as argument and result: in d0–d3 on arm64, through
  `objc_msgSend_stret` on amd64. Checked by set-then-get round trips with non-integer
  values and by `convertRect:toView:` / `convertPoint:toView:` on the flipped view.
* *`BOOL`* is encoded `c` on amd64 and `B` on arm64; a Go `bool` result works for both.
* *App set-up.* The regular activation policy (Dock icon, menu-bar name) and a main menu
  with "Quit <name>" (⌘Q, `terminate:`) are set before `[NSApp run]`.
* *Event loop.* `stop:` only takes effect after the next event, so the spike posts an
  `NSEventTypeApplicationDefined` event after it. `activate` is used when the app
  responds to it, `activateIgnoringOtherApps:` otherwise.
* *Ownership.* The window gets `setReleasedWhenClosed:NO`, since Go still holds it.
  AppKit holds targets and delegates weakly, so the Go side keeps them alive.
* *UI-thread wake.* `performSelectorOnMainThread:withObject:waitUntilDone:NO` from a
  goroutine locked to its own OS thread runs the Go IMP on the main thread, in order
  (the self-test sends three ticks and a probe). This is the macOS `gogui.Do` path.

**Surprises:**

* *A Go 1.26 compiler bug.* Go 1.26 (1.26.4, 1.26.5, 1.26.9) crashes with "internal
  compiler error: disjointTypes: one of arguments is not a pointer" for darwin and linux
  when an inlined `*(*unsafe.Pointer)(unsafe.Pointer(&p))` feeds an `unsafe.Slice` that a
  `copy` then writes to. go1.27rc3 compiles it. `ffi.cptr` was `//go:noinline` until go.mod
  moved to Go 1.27 (2026-10-22); the workaround is gone since then.
* *`purego/objc` takes a callback slot at init* (its block `dispose` callback), so every
  binary that imports it has one slot less (see S0.10).

**○ Open:** both self-test phases on both Macs; the `[NSThread isMultiThreaded]` answer
the worker prints (if NO, S1.1 detaches a dummy `NSThread` at start-up so Cocoa sees a
multi-threaded app); the user's visual check (front window with a Dock icon and menu-bar
name, top-left label, clicks counting, ticks counting, the button staying top-right on
resize, closing the window quitting).

**S1.1 inherits:** hand-built classes with one IMP per signature and a `goHandle` ivar;
frames computed in Go plus `windowDidResize:`; `stop:` plus a posted event;
`setReleasedWhenClosed:NO`; strong references on the Go side for targets and delegates;
`performSelectorOnMainThread:` for `gogui.Do`; the `cptr` workaround.

---

## S0.9 Spike B: Linux (`spike/gtk`, deleted in S4.1l)

A dlopen'd GTK 4 window with a custom `GtkWidget` subclass whose `snapshot` vfunc draws
through a trampoline, clicks, ticks from another thread, GPU readback and a resize.
Checked by `go run ./spike/gtk -selftest` (phase 1 without a main loop, phase 2 with a
window and a `GMainLoop`) under xvfb (`GDK_BACKEND=x11`) and headless weston
(`GDK_BACKEND=wayland`) on linux/amd64 and linux/arm64 (`scripts/native.sh`), and by
the user on a Linux desktop under Wayland and X11.

**Proven by design and build (◐ until the Linux runs pass):**

* *GTK 4.10 or later* (PLAN.md D7). The spike opens the libraries by soname
  (`libgtk-4.so.1`, `libgobject-2.0.so.0`, `libglib-2.0.so.0`) and refuses an older GTK;
  a missing GTK names the package to install (`libgtk-4-1` or `gtk4`). No new
  dependency: puregotk was read only to cross-check layouts (it creates a callback per
  closure, which breaks PLAN.md §7.2).
* *Seven trampolines cover a custom-drawn window.* `GClassInitFunc`, the
  `size_allocate`, `measure` and `snapshot` vfuncs, `GtkGestureClick::pressed`,
  `GSourceFunc` (idle wake, timeouts, frame polling) and `GtkWindow::close-request`.
  `ffi.Callbacks()` stays at 7. The vfuncs dispatch through a handle word in the
  instance, the signals and sources through a handle in `user_data`.
* *The class-size check.* `g_type_query(gtk_widget_get_type()).class_size` must be 408
  (64-bit) before `class_init` writes the vfunc slots (`size_allocate` at 200, `measure`
  at 232, `snapshot` at 320). Otherwise the spike stops with a clear error. A taken type
  name is found with `g_type_from_name` and is an error, not a crash. `class_init` also
  sets the CSS name `ggsurface`, so theme padding doesn't change `measure`.
* *The handle word.* The subclass's `instance_size` is GtkWidget's plus one word; the
  handle sits at GtkWidget's size rounded up to a word. No `g_object_set_data`.
* *No variadic C functions.* `g_object_new_with_properties`, `g_signal_connect_data`,
  `g_signal_emitv` and `g_type_register_static_simple` instead of the variadic ones.
* *Callback shapes.* Negative `int32` arguments (`for_size` = −1), `gboolean` results
  and `float64` arguments through GLib's generic (libffi) signal marshaller (`pressed` x and y, with
  values like 1e300 that don't fit a float32).
* *Out-parameters.* `measure` writes `int`s with `ffi.WriteInt32`; the two baseline
  pointers may be NULL.
* *Structs by pointer only.* `GdkRGBA`, `graphene_rect_t`, `GskRoundedRect`, `GTypeQuery`
  and `GValue` are Go mirrors with `structs.HostLayout` and layout tests; `GValue`s for
  `g_signal_emitv` live in `g_malloc0` memory, since GLib holds them while calling back.
* *No `GtkApplication`.* `gtk_init_check` plus a plain `GMainLoop` is enough, and works
  without a session bus. Whether S4.1 wants `GtkApplication` (app ID, single instance)
  is S4.1's decision.
* *Frame detection by polling.* A `g_timeout_add` source polls the snapshot count every
  10 ms; no frame-clock trampoline is needed.
* *Ownership on close.* GTK owns the window after `gtk_window_new` and destroys it when
  `close-request` returns FALSE; Go frees only its handles and the surface reference.
* *UI-thread wake.* `g_idle_add` from a worker OS thread runs the Go `GSourceFunc` on the
  main thread (checked by thread ids). This is the Linux `gogui.Do` path.
* *GPU readback.* The surface's node through GTK's own path (`gtk_widget_paintable_new` +
  `gdk_paintable_snapshot`), rendered by the window's renderer (`gtk_native_get_renderer`
  → `gsk_renderer_render_texture`) and read back with `gdk_texture_download`
  (premultiplied `0xAARRGGBB` in `g_malloc0` memory), with pixel checks (±1 per channel) on the fill, the clip's content and the
  transparent rounded corner, before and after a click and after a resize.

**○ Open:** both self-test phases under X11 and Wayland on both Linux targets; the
**renderer GSK picks per backend** (an info line; Mesa llvmpipe GL expected under xvfb and headless weston); the
**resize path** that works on each backend (`gtk_window_set_default_size`, or the
`gtk_widget_set_size_request` fallback), which S4.1's `Window.SetSize` depends on;
whether headless weston needs longer frame waits than 5 s / 2 s; and the user's desktop
runs (window drawn, clicks toggle the colour, tick bars grow, resize fills the window
without flicker, closing ends the process) under Wayland and under X11.
**Since S4.1l `spike/gtk` is deleted** and never ran on Linux; its checks were ported
into the `gtk4` tests, which `native.sh` runs under xvfb and headless weston, so these
answers now come from them: the backend and renderer from `TestShellGTKBackend`
(`BACKEND-RESULT:`), the resize path from `TestSurfaceGTKResize` (`RESIZE-RESULT:`), the
frame waits from the clock tests and `showWait`/`resizeWait`; the desktop runs from the
user's look at `examples/hello` under Wayland and X11.

**S4.1 inherited (✅ done, S4.1a–S4.1l, built only until a Linux log):**
`g_type_register_static_simple` with the class-size check and a handle word
(`gtk4/class.go`); vfunc slots written in `class_init`; the trampoline set, one per C
signature (seven in the spike, 16 in the shell since S4.1k, with the input signatures);
no variadics; structs by pointer; `g_idle_add` for `gogui.Do` (`wake.go`); one
`GtkTickCallback` trampoline on the `GdkFrameClock` instead of polling (`clock.go`); GTK
owning the window (`window_linux.go`); and both resize paths, `gtk_window_set_default_size`
with the size-request fallback (`surface.go`), until the Linux log says which one works.

---

## S0.10 Spike C: the callback budget

Measured by tests in `internal/ffi` (S0.10a): `TestCallbackBudget` (a child process
creating fresh closures with `nativeCallback` until creation fails, and a dedup child),
`TestCallbackForeignThread`, `TestCallbackNested` and `BenchmarkCallbackCall`.
`scripts/native.sh` runs them verbosely on each target in the step "callback
measurements (S0.10a)".

| Target | Free slots | Taken before `main` | Total | When used up | Same func twice | Foreign threads | Nesting (17 deep) | Per call |
|---|---|---|---|---|---|---|---|---|
| windows/amd64 | ✔ 1998 | 2 (runtime: console ctrl handler, suspend/resume) | 2000 | ✔ `fatal error: too many callback functions` (unrecoverable) | ✔ same pointer | ✔ 5 × `CreateThread` | ✔ result 36 | ✔ 165–204 ns (`direct`, `SyscallN`, Core Ultra 7 165H; `qsort` is unix-only) |
| windows/arm64 | ○ 1998 expected | 2 | 2000 | ○ fatal throw expected | ○ same expected | ○ | ○ | ○ |
| linux/amd64 | ○ 2000 expected | 0 | 2000 | ○ purego panic (recoverable) expected | ○ new pointer expected | ○ `pthread_create` through fakecgo | ○ | ○ `direct` and `qsort` |
| linux/arm64 | ○ 2000 expected | 0 | 2000 | ○ purego panic expected | ○ new pointer expected | ○ | ○ | ○ |
| darwin/amd64 | ○ 1999 expected | 1 (purego/objc's block dispose callback) | 2000 | ○ purego panic expected | ○ new pointer expected | ○ | ○ | ○ |
| darwin/arm64 | ○ 1999 expected | 1 | 2000 | ○ purego panic expected | ○ new pointer expected | ○ | ○ | ○ |

**Not run (no machine), 2026-10-13:** windows/arm64, linux/amd64, linux/arm64,
darwin/amd64 and darwin/arm64. Their ○ cells are expectations from the sources, not
measurements. The windows/amd64 row was re-measured by `scripts/native.sh` on the host
on 2026-10-13 (all steps passed).

On Windows the 2000 slots are shared by `syscall.NewCallback`, `NewCallbackCDecl`,
`x/sys/windows.NewCallback` and purego's Windows wrapper; the count comes from a child
process, since using up the slots is fatal there.

The expectations come from the sources (`runtime/zcallback_windows.go`: `cb_max = 2000`;
purego `syscall_unix.go`: `maxCB = 2000`; `purego/objc`'s `init`). The test fails if a
target differs from its expected count, or has fewer than `MaxCallbacks` + 64 = 1088
free slots (S0.10b). The foreign-thread and nesting tests also run on unix with the same
checks; the benchmark numbers are only logged. purego's unix `callbackWrap` takes a
global mutex and calls through `reflect.Value.Call`, so the unix per-call cost is
expected to be higher than Windows'; it matters only for per-frame or per-motion-event
callbacks.

**Hidden ways to spend the budget, and how S0.10b closes them:**

* *`objc.RegisterClass`, `objc.NewIMP`, `objc.NewBlock`* create purego callbacks the
  registry never sees (per method, per ivar accessor, per block signature).
  → `scripts/check.sh`'s hidden-callback guard fails on them, and on `syscall`/`windows`/
  `purego` `NewCallback` and `NewCallbackCDecl`, in any non-test file outside
  `internal/ffi`.
* *`ffi.Bind` with a Go func argument*: purego's `RegisterFunc` makes a new callback for
  that argument on **every call**, so about 2000 calls would exhaust the process.
  → `Bind` and `BindAddr` reject such func types with an error pointing to
  `RegisterCallback`. Func-typed results stay allowed (they only wrap a C pointer). A func
  passed through a trailing `...any` can't be seen by the check, so drivers never pass
  one there.
* *Other purego users in the process* share the 2000 slots.
  → `MaxCallbacks` is 1024 on every OS (Windows' guarantee), leaving about 975 slots on
  darwin and linux.

**The rule that follows:** each driver registers about ten trampolines, one per
signature, at start-up (the spikes need 3 and 7); the budget is 1024 for the whole
process; nothing outside `internal/ffi` creates callbacks, and `check.sh` enforces it.
With that, the callback count is not a capacity risk; running out would take a driver
bug that creates callbacks per widget or per call, which the registry's names and the
guard catch.

---

## Open runs

These have to pass, and their numbers go into the sections above, before S0.10 and
phase 0.5 get their ✅. Items 1–4 come from `bash scripts/native.sh` on each target, which
writes `native-logs/<goos>-<goarch>.log` (S0.10c_2; windows/amd64 is run on the host);
item 5 is the user's (S0.10c_3). A target without a machine is recorded as "not run".
As of 2026-10-13 (S0.10c_2a) only windows/amd64 has run; every item below is
**not run (no machine)** for the other five targets and is picked up by S0.10c_2b when
the logs arrive.

1. **S0.7** (step `go test` on darwin/arm64, darwin/amd64, linux/amd64 and
   linux/arm64): `go test -count=1 ./...`, which runs `./internal/ffi` with the
   S0.8a/S0.9a memory tests, the S0.9a `BindAddr` tests and the S0.10b `Bind` guard
   tests (`TestBindRejectsFuncArgs`).
2. **S0.10** (step `callback measurements (S0.10a)` on all six targets, windows/amd64
   and windows/arm64 included): the `TestCallbackBudget` log line (free, taken,
   total, `MaxCallbacks`), the failure-mode and dedup results,
   `TestCallbackForeignThread`, `TestCallbackNested`, and the `BenchmarkCallbackCall`
   `direct` and `qsort` ns/op: the ○ cells of the table above.
3. **S0.8** (step `spike/appkit self-test` on both Macs, from a logged-in GUI
   session): the check lines of both phases and the line
   `info: [NSThread isMultiThreaded] on the worker = …`.
4. **S0.9** (since S4.1l, which deleted `spike/gtk`: steps `gtk4 tests under xvfb (X11)`
   and `gtk4 tests under headless weston (Wayland)` on both Linux targets; needs
   `libgtk-4-1 libgl1-mesa-dri libegl1 xvfb weston`): the ported class, signal, source,
   shell, surface and canvas tests passing, and the lines `BACKEND-RESULT:` (display type,
   renderer), `RESIZE-RESULT:` (the resize path) and the clock tests' frame info; weston's
   log is in the same step. The `examples/hello for 5 s` steps on both backends too.
5. **Manual checks**: `go run ./spike/appkit` on an arm64 and an amd64 Mac (front window
   with a Dock icon and menu-bar name, label top-left and button top-right, clicks and
   ticks counting, the button staying top-right on resize, closing quits); on a Linux
   desktop under Wayland and under X11, saying whether X11 was a real session or
   XWayland, `go run ./examples/hello` (since S4.1l, instead of `spike/gtk`): the window
   with traffic lights and placeholder boxes, hover and clicks, moving by the title bar,
   resizing without flicker, closing ends the process.
