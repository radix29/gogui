# gogui: completed work (archive of PLAN.md)

> Everything here was moved out of `PLAN.md` on 2026-10-22: finished implementation steps
> (with their Results and findings) and the Q&A log. `PLAN.md` keeps the design, the open
> and upcoming steps, the decisions (§12) and the risks. Section numbers (§…) and step ids
> (S0.1 …) refer to `PLAN.md`; nothing was reworded.

---

## A. Phase 0: Foundations (from PLAN.md §11.1)

Only the policies and findings later steps rely on are kept here.

**S0.1 Driver types and peer interfaces** ✅. `internal/driver`: geometry, `Color`, font,
`Appearance`, `ControlSize`, keys/shortcuts, `WindowConfig`, `MenuBarSpec`, `AlertSpec`,
`AppConfig`/`AppHandler` and every peer/handler interface of §4.3 (incl. `Container`).
`driver/stub`: a no-op peer per interface that stores state, has `User…` methods to
simulate input, and fakes deterministic metrics.

**S0.2 UI-thread queue** (`internal/mainthread`) ✅. Lock-free MPSC FIFO with a coalesced
wake (at most one per `Drain`); `Drain` runs a snapshot and is re-entrant; `DoSync` runs
inline on the UI goroutine, re-raises panics as `*PanicError` and returns false after
`Close`; `Check(what)` is gated by `SetChecks`.

**S0.3 Handle table** (`ffi.Handles[T]`) ✅. A `Handle` is a `uintptr`: 24-bit slot index
plus a generation, zero never valid; `Free` bumps the generation so stale or double-freed
handles fail `Get`. Mutex-protected, but drivers use it on the UI thread only (§7.2).

**S0.4 Layout engine** (`internal/layout`) ✅. Pure Go, 100 % coverage, imports only
`internal/driver`; exact-frame tests for all five examples at default, minimum and a bigger
size.
* **Node contract:**
  ```go
  type Node interface {
  	Props() *Props                      // min/max/fixed, alignment, expansion, hidden
  	Expands(a Axis) bool                // own flag, or propagated from the children
  	Measure(p *Pass, max Size) Size     // natural size within max, never below the minimum
  	Arrange(p *Pass, frame Rect)        // frame is final; the parent already aligned it
  }
  ```
  Measuring with the main axis unbounded gives the natural size, at 0 the minimum. Nodes
  measure children only through `p.Measure` (the cache). Leaves take a local
  `Item{FittingSize(max Size) Size; SetFrame(r Rect)}`; optional `FirstLine(p, max)` aligns
  form labels.
* **Policies:**
  * *Sizes.* Unset max is +Inf, max 0 is real, negative/NaN count as 0, min wins over max,
    fixed sets both. A leaf's default min is its fitting size; spacers are 0. Hidden nodes
    are neither measured nor framed and get no spacing.
  * *Height-for-width.* A VStack measures children at its inner width. An HStack measures
    natural widths unbounded, assigns widths with `distribute`, then measures heights.
  * *`distribute`.* Leftover is shared equally among expanders (spacers included), max
    caps re-share. Short space shrinks everything in proportion to (natural − min); below
    the minima content overflows at the trailing edge. The float leftover goes to the last
    sharing child; the sum is the float just below the space, never above.
  * *Expansion.* A spacer expands along its parent's main axis. Stacks, forms and boxes
    expand when a child does; a scroll view doesn't take on its document's. Text field,
    secure field, slider and progress bar expand horizontally; text view on both axes;
    scroll view per enabled scroller; separator along its line; labels, buttons,
    checkboxes, pop-ups and spinner don't. `RadioGroup` is a VStack with 6 pt spacing.
  * *Cross axis.* Per-child alignment (`Leading`, `Center`, `Trailing`, `Fill`); default
    `Leading` in a VStack, `Center` in an HStack; cross-axis expansion means `Fill`.
  * *Peers.* A stack or form may own a `Container` peer (children get frames relative to
    it); without one it is a transparent group. Box and ScrollView always own one; every
    public stack and form has one.
  * *Root* gets the full content size. `Root.MinSize` = measured width at max (0, +Inf),
    then the natural height at that width.
  * *Rounding.* Fractional logical points in the engine; drivers snap edges (X and MaxX
    rounded separately), never sizes.
  * *Cache.* Per pass, keyed by node and normalized max; a tree must not change during a
    pass. At most 9 `FittingSize` calls per leaf at any depth; `BenchmarkLayout1000` ≈ 1 ms.
  * *`RightToLeft`* is accepted but laid out as LTR (§7.4).
  * *Form.* Label column as wide as the widest visible label, never shrinks; labels are
    right-aligned and centered on the control's first line; controls share one x;
    expanders fill the control column; a row hides with its control. `FormLabelGap` and
    `FormRowSpacing` are 8 pt, **from memory, still to be checked against the HIG.**
  * *Box* reads `ContentInsets()` every pass. *ScrollView* reads `ViewportSize()`,
    measures the document unbounded on scrolling axes and calls
    `SetDocumentSize(max(measured, viewport))`; the viewport depends only on the frame,
    the enabled scrollers and the appearance, so this never causes `ViewportResized`.
  * Anchor frames pinned by tests: `hello` Greet button x = 257 in 320 pt; `form` controls
    x = 71, fields 349 wide; `converter` controls x = 85; `notes` sidebar document 180 ×
    796; `controls` busy bar 238.5. Minimum windows: `hello` 136 wide, `form` 346 × 332,
    `converter` 221 × 173, `controls` 420 × 487.

**S0.5 Public API skeleton and headless driver** ✅. `gogui` has the complete v0.1 API of
§4; `internal/driver/headless` (stub peers plus a `Run` loop on a `mainthread.Queue`)
backs it, and all five examples build for the six targets and run headlessly.
* **Policies:**
  * *Value types are aliases* of `internal/driver` types; `gogui` defines its own type
    only for `Appearance`, menu and alert descriptions (they hold funcs) and widgets.
  * *`Run(cfg, ready) error`* blocks until `Quit`; returns the driver's error or an error
    on a second call; library code never calls `os.Exit`. `init()` locks the OS thread.
    `Do` queues, `DoSync` waits (inline on the UI thread); outside `Run` `Do` drops fn
    and `DoSync` returns false. The UI-thread check runs in every public method (on in
    tests and with `GOGUI_DEBUG=1`).
  * *Peers are created eagerly*, so constructors panic outside `Run`; a driver error in
    `NewWindow` panics.
  * *Chaining.* Setters return the concrete type; shared setters live in a generic
    embedded `common[T]`; `Widget` is a sealed interface.
  * *Events.* `On*` takes one func and returns `Unsubscribe`; handlers run in subscription
    order; setters never fire events; disabled or destroyed widgets drop native events.
    Veto events (`OnShouldClose`, `OnQuitRequested`): first `false` wins; `Close`/`Quit`
    don't ask.
  * *Tree.* One parent per widget (adding elsewhere moves it; nil, duplicate and cyclic
    children panic). A peer goes into the nearest ancestor that owns a peer. `Row("Name:",
    c)` creates a label peer, `Row("", c)` none; `Spacer()` has none.
  * *Re-layout is deferred and coalesced.* A size-changing setter marks the window dirty
    and posts one pass; `ContentResized` and a dirty `Show` lay out at once. After each
    pass the window sets `SetMinSize(max(cfg.MinSize, root.MinSize()))` on change.
  * *Lifetime.* Closing a window destroys its tree bottom-up (`OnClose` first). Widgets
    never placed are destroyed when `Run` returns. Destroyed widgets only record setters.
  * *Appearance.* `App.Appearance()` is the effective value, never System;
    `OnAppearanceChange` fires only on a Light/Dark switch.
  * *Menus.* `SetMenuBar` assigns fresh never-repeating item IDs; items without an action
    are disabled. `Alert`: "OK" if no buttons; only the exact title "Cancel" gets Escape
    (§7.4); a nil window means app-modal.
  * *Radio/pop-up `OnChange`* fires only when the selection changes. Text-view setters
    don't invalidate; `TextView.OnChange` carries the text.
  * Left as is: `Button`/`Checkbox` have `SetTitle` but no `Title()`; `RadioGroup` has no
    `SetItems`; peers have getters only for user-changeable state.
* **Test hook:** `driver.go` has `var newDriver = func() driver.Driver {…}`; `gogui` tests
  swap in a quiet headless driver and reach stubs through widgets' `peer` fields;
  `runApp(t, func(app *App))` runs a body in `ready`, drains and quits.
* A headless `Run` with no other goroutine was killed by Go's deadlock detector, so it
  keeps a far-future timer pending.

**S0.6 Tooling** ✅ *phase 0 exit*. `scripts/check.sh` (≈2 min; `-quick` ≈70 s),
`staticcheck.conf`, `.gitattributes` forcing LF (CRLF breaks `bash` and `gofmt -l`).
* staticcheck v0.7.0 is pinned in `check.sh`, not `go.mod`; a cross-GOOS `go run` of it
  fails on Windows, so the script builds it once for the host and runs it per GOOS.
* Git Bash in the agent sandbox may block `find` and `sort`, so `check.sh` uses only bash
  built-ins, `go` and `gofmt`.
* The stress run (`.`, `mainthread`, `ffi`, `headless`, later `aqua`, `shell/fake`,
  `shell/win32`) takes ≈85 s, mostly in `headless`.
* CI (`ci.yml`) was built and then **dropped on 2026-10-13**; `native.sh` replaces it
  (§8.4). No `LICENSE` yet (O4).

## B. Phase 0.5: purego spikes, S0.7–S0.10b (from PLAN.md §11.1)

The macOS and Linux runs are still open; they stay in PLAN.md (S0.10c).

**S0.7 Native FFI helpers** (`internal/ffi`): S0.7a ✅, S0.7b ✅ *on Windows only*. S0.7 is
✅ when `go test -count=1 ./internal/ffi` has passed on macOS and Linux (both archs).
* **S0.7a (stdlib):** trampoline registry `RegisterCallback(name, fn) uintptr` (one per
  signature, §7.2; panics on empty/duplicate name or bad signature; `Callbacks()` counts),
  and layout helpers `CheckLayout`/`CheckNoGoPointers`. Signature rules: non-variadic, ≤ 12
  args of bool/integer/uintptr/pointer (floats too, except on Windows), ≤ 1 result
  (exactly one on Windows), no structs.
* **S0.7b (purego v0.11.1, unix files):** `Dlopen` → `*Lib` (`Sym`, `MustSym`, `Bind`,
  `Close`) with clear errors; `CString`/`Free`/`GoString`/`GoStringN` on libc `malloc`;
  libc is `libSystem.B.dylib` / `libc.so.6` (glibc only; musl out of scope for v0.1).
* **Findings:**
  * With `CGO_ENABLED=0` purego links its own `fakecgo` on darwin and linux. Windows
    didn't import purego until S2.4a (`com`, float COM calls); otherwise only
    `_darwin`/`_linux`/`_unix` files import it.
  * **Go 1.26 compiler bug** (1.26.4, .5, .9): ICE "disjointTypes" on darwin/linux when an
    inlined `*(*unsafe.Pointer)(unsafe.Pointer(&p))` feeds an `unsafe.Slice` that `copy`
    writes to (fixed in go1.27rc3). `cptr` is `//go:noinline`; keep that until `go.mod`
    requires Go ≥ 1.27. The single uintptr→pointer conversion lives in `cptr` (`mem.go`).
  * Without cgo a layout test can't ask a C compiler for `sizeof`/`offsetof`; the helpers
    compare a Go mirror against per-GOARCH constants from the headers.
* Not checked: the unix tests of S0.7/S0.8a/S0.9a on macOS and Linux.

**S0.8 Spike A: macOS** (`spike/appkit`, S0.8a–S0.8c): S0.8a ✅; S0.8b/c ✅ *on Windows
only*. S0.8 is ✅ when `go run ./spike/appkit -selftest` has passed on both Macs **and the
user has looked at `go run ./spike/appkit`** on arm64 and amd64 (window to the front with a
Dock icon, label and button, clicks update the counter, ticks count up, resizing keeps the
button top-right, closing quits).
* `ffi.ReadUintptr/WriteUintptr(addr, off)` for native memory only (ivars, GObject slots).
* The spike (≈1,300 lines): window, flipped content view, frames from Go, window and app
  delegates, a 1 s ticker waking the main thread with `performSelectorOnMainThread:`, and
  a two-phase `-selftest`.
* **Findings for S1.1:**
  * Only darwin files may import `purego/objc` (its `init` dlopens libobjc).
    **`objc.RegisterClass` hides callbacks** (one per method and ivar accessor), so S1.1
    builds classes itself: `objc_allocateClassPair`, `class_addIvar`,
    `objc_registerClassPair` via `ffi.Bind`, methods via `Class.AddMethod` with
    hand-written encodings. The handle ivar is read/written at `self + ivar_getOffset`.
    The `BOOL` encoding is `c` on amd64, `B` on arm64.
  * Three trampolines cover everything: `void (id, SEL, id)`, `BOOL (id, SEL)`,
    `BOOL (id, SEL, id)`, each running the peer inside an autorelease pool.
  * No struct callbacks: don't override `drawRect:` or `setFrameSize:`; relayout comes from
    `windowDidResize:`; the flipped view only overrides `isFlipped`.
  * `NSRect` passing (32 bytes: d0–d3 on arm64, `objc_msgSend_stret` on amd64) is checked
    by set/get round trips; mirrors use `structs.HostLayout`.
  * `[NSApp stop:]` takes effect after the next event, so post an
    `NSEventTypeApplicationDefined` event. Use `setReleasedWhenClosed:NO` while Go owns
    the window. AppKit holds targets and delegates weakly. Call `activate` if
    `respondsToSelector:` allows, else `activateIgnoringOtherApps:`.
  * Open: if `[NSThread isMultiThreaded]` is NO on the worker, S1.1 detaches a dummy
    `NSThread` at start-up.

**S0.9 Spike B: Linux** (`spike/gtk`, S0.9a–S0.9c_3): S0.9a ✅; the rest ✅ *on Windows
only*. S0.9 is ✅ when both self-tests (xvfb/X11, headless weston/Wayland) pass on amd64
and arm64 Linux **and the user has run `go run ./spike/gtk`** under Wayland and X11 (say
whether X11 was real or XWayland): drawn content, a click toggles the colour, tick bars
grow, resizing keeps the drawing filling the window without flicker, closing ends the
process. (Since S4.1l `spike/gtk` is deleted: these runs are now the `gtk4` tests in `native.sh` and the user's look at `go run ./examples/hello`, PLAN.md S0.10c_2b/S0.10c_3.)
* `ffi.ReadInt32/WriteInt32` and `ffi.BindAddr(fptr, addr)` (call a function pointer, e.g.
  to chain to a parent vfunc).
* The spike (≈2,200 lines): `GGSurface : GtkWidget` with `measure`, `size_allocate` and
  `snapshot` vfuncs, render nodes, a click gesture, a window, a `GMainLoop`, a cross-thread
  wake, renderer readback, resize, and a two-phase `-selftest`. `scripts/native.sh` needs
  `libgtk-4-1 libgl1-mesa-dri libegl1 xvfb weston` (Debian/Ubuntu).
* **Findings for S4.1:**
  * No new dependency: libs are opened by soname (`libgtk-4.so.1`, `libgobject-2.0.so.0`,
    `libglib-2.0.so.0`). puregotk was read only (it creates a callback per closure,
    breaking §7.2).
  * No variadic C functions (purego can't pass variadic floats): use
    `g_object_new_with_properties`, `g_signal_connect_data`, `g_signal_emitv`,
    `g_type_register_static_simple`.
  * `GtkWidgetClass` is 408 bytes on 64-bit; vfunc slots `size_allocate` 200, `measure`
    232, `snapshot` 320. Start-up check: `g_type_query(gtk_widget_get_type()).class_size`
    must be 408. The instance handle lives in one word after the parent's instance. Set
    the CSS name so theme padding doesn't change `measure`.
  * `measure`'s out-ints and `g_signal_emitv`'s `GValue`s must be native memory
    (`g_malloc0`). Structs (`GdkRGBA`, `graphene_rect_t`, `GskRoundedRect`) go by pointer.
  * **Seven trampolines** cover everything: `class_init`, `size_allocate`, `measure`,
    `snapshot`, `pressed`, `GSourceFunc`, `close-request`. No frame-clock trampoline: a
    new frame is detected by polling the snapshot count from a `g_timeout_add` source.
  * `gtk_init_check` plus a plain `GMainLoop` suffices (no `GtkApplication`, which needs
    the session bus). `g_idle_add` from a locked worker thread wakes the loop. GTK owns
    the window after `gtk_window_new`; `close-request` returning FALSE destroys it.
  * Readback: `gtk_widget_paintable_new` → `gsk_renderer_render_texture` →
    `gdk_texture_download`. Open until the Linux runs: which renderer each backend gets,
    and whether `gtk_window_set_default_size` or `gtk_widget_set_size_request` resizes a
    mapped window (S4.1's `SetSize`).

**S0.10 Spike C: callback budget** (S0.10a–S0.10c_3): S0.10a/b ✅ *on Windows only*,
S0.10c_1 and S0.10c_2a ✅. S0.10 is ✅ when S0.10c is done; phase 0.5 exits when S0.7–S0.10
are ✅.
* **Findings:**
  * *Windows:* 2000 callback slots shared by `syscall.NewCallback`, `NewCallbackCDecl`,
    `x/sys/windows.NewCallback` and purego's Windows wrapper; 2 taken at start-up. Exhaustion
    is a fatal `throw`. `syscall.NewCallback` deduplicates a top-level func.
  * *darwin/linux:* purego's `maxCB = 2000`, the 2001st call panics (recoverable), no
    dedup; `purego/objc` takes one slot (1999 free on darwin).
  * Hidden paths, closed by S0.10b: `objc.RegisterClass`, and **`Bind` of a func-typed
    argument, which creates a callback per call**.
  * Real use is tiny: 3 trampolines (appkit), 7 (gtk); a full driver should stay under 50.
  * Per call ≈165–204 ns (`SyscallN`) on the host; purego's unix `callbackWrap` takes a
    global mutex and uses `reflect.Value.Call`, so it is slower (matters only for
    per-motion-event callbacks, S4.1).
* **S0.10a:** tests `TestCallbackBudget` (child process: free slots, failure mode, dedup),
  `TestCallbackForeignThread`, `TestCallbackNested` (17 nested C→Go calls, result 36) and
  `BenchmarkCallbackCall`, run by `native.sh`; the stress run skips `TestCallbackBudget`.
  Windows: 1998 free, foreign threads and nesting pass.
* **S0.10b:** `MaxCallbacks = 1024` on every OS (leaves ≈975 slots on darwin for
  purego/objc), checked for headroom (free ≥ `MaxCallbacks` + 64). `Bind`/`BindAddr`
  reject func-typed arguments (a func passed through trailing `...any` can't be seen).
  `check.sh` has the hidden-callback guard (§11.1).
* Not checked: `TestBindRejectsFuncArgs` and the headroom check on the five other targets
  (expected 1999 free on darwin, 2000 on linux).

## C. Phase 1: S1.0 (from PLAN.md §11.1)

**S1.0 Driver-selection files** ✅ *2026-10-13*. `driver.go` keeps `uiDriver`,
`selectDriver` and `debugChecks`; `newDriver` calls `newPlatformDriver`, defined in
`driver_darwin.go` (`//go:build darwin`) and `driver_other.go` (`!darwin`). These are the
only build-tagged files in package `gogui`.

## D. Phase 2: S2.1–S2.4 (from PLAN.md §11.1)

**S2.1 Render and shell interfaces** ✅ *2026-10-13, S2.1a–S2.1d* (stdlib only). Popups
(S5.5), IME (S5.3) and clipboard (S5.2) extend `shell` in their own steps.
* **Decisions:**
  * *Geometry.* `render` reuses `driver.Point/Size/Rect` (float64); backends narrow to
    float32 at the native call.
  * *Colors.* `Paint` holds concrete straight-alpha `driver.Color`s; a semantic color
    reaching a canvas is a bug (the recorder fails the test).
  * *Text.* `TextSystem`/`TextLayout` live in `render` and come from
    `Shell.TextSystem()`; indices are UTF-8 byte offsets.
  * *Units.* Every shell value is in logical points, top-left origin; the surface reports
    its scale and backends apply it.
* **S2.1a `render` value types:** `Radii` (clamped to half the rect side), `Paint` (solid
  or linear gradient, stops sorted and clamped), `StrokeStyle`, `Shadow`, `Path` (HTML
  canvas rules, `ArcTo` as ≤ 2 cubics, tight `Bounds`), `Transform` (2×3 affine), `Image`
  (size only). All validation in `validate.go`.
* **S2.1b `Canvas`, text interfaces, `render/record`:** refinements of the §5.3 sketch:
  `Concat(Transform)`, one `Pop` for both layers, `Push*` also saves state, `Text`/`Font`/
  `LineCount` on `TextLayout`. The recorder resolves transform, clip and opacity, checks
  balance, rejects semantic colors, collects numbered violations, and prints a stable dump
  (`fillRRect (0,0,80,22) r=5 solid #007aff`).
* **S2.1c `internal/shell`:** `Shell` (`Run`, `Quit`, `Post`, `NewSurface`, `TextSystem`,
  `Appearance`, `Clock`), `Surface`, `SurfaceHandler`, input events, `FrameClock`. The
  drawable area is the whole client area (`SurfaceConfig.Size` includes the title bar aqua
  draws). Event modifiers are physical (no `ModPrimary`). Chrome regions are tested last
  to first; caption presses go to the OS, and only `RegionMaximize` feeds Snap Layouts.
  `Event` returns `handled`. Scroll deltas are points when precise, lines otherwise; OS
  momentum phases only on GTK. `RequestFrame` is one-shot and coalesced.
* **S2.1d `shell/fake`:** fake `Shell` on a `mainthread.Queue` with `Step`/`Flush`, fake
  surfaces that record state and paint into a `record.Canvas`, input helpers building real
  sequences (`Click`, `Drag`, `Wheel`, `Swipe`, `Type`, …), a fixed-width text system
  matching `driver/stub` (7 pt per rune and 16 pt per line at 13 pt) and a manual 60 Hz
  clock.

**S2.2 Aqua skeleton** ✅ *2026-10-14, S2.2a–S2.2e* (stdlib only, no build tags, tests run
everywhere).
* **Decisions:**
  * *Layout stays in `gogui`.* An aqua element is a `layout.Item` plus `Paint`,
    `HandleEvent` and `AccessibleNode`; it doesn't measure or arrange itself (§5.1).
  * *Content area.* A titled window's surface is the content size plus a 28 pt title bar;
    all `driver.Window` sizes are in content coordinates.
  * *Provisional colors.* Chrome and placeholders use `internal/aqua/provisional.go`
    (light/dark) until S3.1 replaces it.
  * *Placeholders embed the stub peers,* so example layouts equal the headless ones; S3.3+
    replace them with real measurements.
  * *`native.New()`* lives in `internal/shell/native` (an import cycle otherwise).
* **S2.2a Element core:** `Element` (sealed to types embedding `Base`, bound with
  `Init(self)`), tree operations, `PaintTree` (per element `Save`/`Translate`/`Paint`,
  optional children clip), invalidation walking up to a root callback once per
  clean→dirty transition (v1 repaints the whole window), `HitTest` (last child first,
  disabled elements are hit, point converted to local).
* **S2.2b Routing, focus, a11y:** aqua events (`PointerEvent`, `ScrollEvent`, `KeyEvent`,
  `TextEvent`, `FocusEvent`); `Router`: hover = enabled hit path, capture from press to
  release (`Over` reports drag-off), scroll/key/text bubble skipping disabled elements,
  unhandled Tab/Shift-Tab calls `FocusNext` (pre-order, wraps). Removing, hiding or
  disabling a focused/hovered element sends the cancel/leave/blur events. `a11y` package:
  `Role`, `States`, `Actions`, `Node` (`Walk`, `Find`, `Count`, stable `Dump`);
  `AccessibilityTree(root)` hoists `RoleNone` elements.
* **S2.2c Window and chrome:** `NewWindow(sh, cfg, h)` builds root → clipping content area
  (paints `ColorWindowBackground`) → title bar. `Window` implements `driver.Window`;
  `SetContentSize` always reports `ContentResized` once. Title bar: 28 pt, 13 pt semibold
  title centered and truncated with "…" (kept 68 pt from both edges, or hidden).
  Traffic lights: 12 pt circles at x = 8, 28, 48; hovering the group shows ×/−/+; pressed
  is 18 % darker; inactive is grey; a light without its style flag is half-transparent
  and stays caption. Chrome regions are recomputed on every resize. Releasing over a
  light calls `Minimize`, `Zoom`, or `ShouldClose` then `Close`. The OS close asks
  `ShouldClose` too (refused without `StyleClosable`).
* **S2.2d Aqua driver, `Container`, `shell/native`:** `aqua.New(sh)` implements
  `driver.Driver`: `Run` (shell `Ready` → app `Ready`, own `mainthread.Queue` woken by one
  `Shell.Post` drain), `Quit` (closes the queue, every window gets `Closed`), `Appearance`
  (forced Light/Dark merged over the OS value; changes repaint all windows), windows
  tracked until closed. `SetMenuBar` and `RunAlert` only record until S5.6/S5.7.
  `Container` re-parents aqua elements; non-element widgets are invisible. Other factories
  return stub peers.
* **S2.2e Placeholder controls:** a placeholder peer for each remaining control (embeds
  its stub; element paints a 4 pt rounded rect with an 11 pt "Kind: text" caption, with a
  real a11y node). `Box` hosts its content at the stub's insets; `ScrollView` clips and
  translates by the offset. `gogui.TestExamplesOnAqua` runs the five examples on the aqua
  driver over the fake shell: frames equal the headless ones, every window paints without
  recorder errors; `hello`'s dump is pinned.

**S2.3 Win32 window and input** (adds `golang.org/x/sys`) ✅ *2026-10-16, S2.3a–S2.3f*.
The manual window check of S2.3f is the user's. Native tests create real HWNDs on the host.
* **Decisions:**
  * *Bindings in one package.* `internal/shell/win32/w32` holds procs (via
    `NewLazySystemDLL`, so only System32 is searched), constants and mirrored structs;
    `x/sys/windows` lacks most window APIs, so `w32` declares them with `LazyProc.Call`.
    Every struct has a layout test for amd64 and arm64 (the arm64 run needs a machine).
  * *Structs that arrive as an `lParam`* (`MINMAXINFO`, `NCCALCSIZE_PARAMS`, `WINDOWPOS`,
    `CREATESTRUCTW`, the `WM_DPICHANGED` rect) are read with `ffi.At[T](p) *T` (built on
    `cptr`, panics on 0 or misalignment), so no `//nolint` outside `ffi`.
  * *Interim pieces until S2.4/S2.5,* each in an `interim_*.go` file its replacing step
    deletes: `shell/fake`'s fixed-width `TextSystem`, a thrown-away `record` canvas for
    `Paint`, GDI window-background fill on `WM_PAINT`, and a 60 Hz `time.Now` clock.
  * *Precise scrolling without Direct Manipulation.* A `WM_MOUSEWHEEL` delta that is not a
    multiple of 120 is precise; Began on the first, Ended ≈100 ms after the last (a
    surface timer). Momentum stays aqua's (§5.6).
* **S2.3a `w32`:** `golang.org/x/sys` v0.49.0 (`go get` rewrote `go 1.26` to `go 1.26.0`);
  ≈60 procs across user32, gdi32, dwmapi, shcore, kernel32, each resolved by
  `TestProcsResolve`; 15 mirrored structs; typed wrappers return the `GetLastError` error
  (`Get/SetWindowLongPtrW` may legitimately return 0; `GetMessageW` returns `(ok, err)`).
  The Go runtime's syscall stub already clears/reads the last error per call. `ffi.At[T]`
  plus Windows-memory round trips (`mem_windows_test.go`).
* **S2.3b Shell core** (`internal/shell/win32`): `Run` locks the thread, sets
  Per-Monitor-V2 awareness (a second call in a process fails with `ERROR_ACCESS_DENIED`,
  accepted when the context already equals PMv2), creates a hidden `WS_POPUP` tool window
  for the wake (not message-only: those get no broadcasts, and appearance needs
  `WM_SETTINGCHANGE`), runs `GetMessageW`. `Post` pushes onto a `mainthread.Queue` with one
  `PostMessageW` per drain. One `win32.wndproc` trampoline reads the `ffi.Handle` from
  `GWLP_USERDATA` (set in `WM_NCCREATE` from `lpCreateParams`); stale handles go to
  `DefWindowProcW`; panics are recovered at the trampoline and re-raised after the message
  returns. `Appearance`: `AppsUseLightTheme`, accent (`DWM\AccentColor` ABGR before
  `DwmGetColorizationColor`, always opaque), high contrast, `SPI_GETCLIENTAREAANIMATION`,
  `EnableTransparency`, legacy scrollers from `DynamicScrollbars` = 0; `AppearanceChanged`
  fires only on change.
* **S2.3c Surface:** one class (`CS_DBLCLKS`), hidden window created at the primary
  monitor's DPI then resized at its own. Points ↔ pixels: `toPx` rounds half away from
  zero; `fromPx` keeps the requested size when it maps to the same pixels. `SetSize`
  restores a maximized/minimized window first and reports sub-pixel changes itself;
  `WM_GETMINMAXINFO` always sets the minimum track size. `WM_DPICHANGED` holds back the
  suggested rect's `WM_SIZE` and gives one `Resized` with the new scale. During an
  interactive size/move, `Resized` paints at once. `Quit` closes surfaces newest first
  without `CloseRequested`; afterwards `NewSurface` returns `ErrQuit`. State-changing
  surface methods panic during `Paint`.
* **S2.3d Custom chrome** (`chrome.go`, `ncinput.go`): windows keep `WS_OVERLAPPEDWINDOW`
  (taskbar, system menu, Snap and animations stay native) while `WM_NCCALCSIZE` removes the
  caption; the frame is `SM_CXSIZEFRAME` + `SM_CXPADDEDBORDER` (maximized: frame on every
  side, client clipped to the work area). The top resize band lies inside the drawable
  area. A non-resizable window has no frame at all. `WM_GETDPISCALEDSIZE` keeps the
  logical size across DPIs. `DwmExtendFrameIntoClientArea` (1 px top) keeps the shadow;
  Windows 11 rounded corners and the dark frame follow the appearance (failures ignored on
  Windows 10, D7). `WM_NCHITTEST`: borders first, then regions last to first
  (`HTCAPTION`, `HTCLIENT`, `HTCLOSE`/`HTMINBUTTON`/`HTMAXBUTTON`), all inside the shell.
  Non-client mouse over a light becomes ordinary `PointerEvent`s with the default action
  suppressed (Snap Layouts opens on hover; `WM_NCMOUSEMOVE` still reaches
  `DefWindowProcW`).
* **S2.3e Input** (`input.go`, `wheel.go`, `keyboard.go`):
  * *Times:* now minus the message age (`GetTickCount − GetMessageTime`, wrapping uint32);
    an age over 5 s or from the future gives now; times never go backwards.
  * *Mouse:* five buttons, one capture while any is held, a `PointerUp` per held button
    when capture is lost, shell-counted clicks over message times (`DBLCLK` counts ≥ 2),
    repeated moves dropped. Handling right/X buttons means no `WM_CONTEXTMENU`/
    `WM_APPCOMMAND`: aqua does context menus and back/forward.
  * *Wheel:* notches in lines (`SPI_GETWHEELSCROLLLINES`/`CHARS`, per event; page = 20
    lines), precise deltas at 100/3 pt per line; positive horizontal delta moves content
    left.
  * *Keyboard:* VK table for named keys; character keys as the unshifted character of the
    active layout via `MapVirtualKeyExW`; `WM_CHAR` → `TextEvent` with surrogates joined
    and control characters dropped. A handled key-down removes its pending `WM_CHAR` and
    skips `DefWindowProcW` (and its key-up, so a consumed Alt doesn't enter menu mode); an
    unhandled one lets Alt+F4/Alt+Space through. `WM_SYSCHAR` is swallowed; `VK_PROCESSKEY`
    waits for S5.3. No key-ups on focus loss.
  * Alt+F4 is checked only as far as the shell goes (a synthetic message doesn't close
    the window); `TestSurfaceWheel` is skipped in the stress run (real timer).
* **S2.3f `shell/native` and the aqua switch:** `native_windows.go` returns `win32.New()`;
  `native.go` (untagged `ErrNotImplemented`) and `native_other.go` (`!windows`).
  `driver_other.go` selects `aqua.New(sh)`, or `failedDriver("aqua", err)` (in
  `driver.go`): a stub driver whose `Run` returns the error without calling the handler,
  so `gogui.Run` returns the phase-4 error on Linux. Smoke run: `examples/hello` opens a
  responding window titled "Hello" (empty until S2.4).
* **Checked on the host** (Windows 11, 100 % scaling): full `check.sh` and `native.sh` on
  windows/amd64 for each sub-step. **Still for the user, on Windows:** the manual window
  check (drag by the top 28 pt, resize from every edge, Win+arrows, edge snapping, Snap
  Layouts on the green light, moving between monitors with different scaling, Alt+F4 and
  taskbar close quit the app) and the hand checks left from S2.3d/e (scales other than
  100 %, precision touchpad, high-resolution wheel, dead-key and non-Latin layouts). Not
  run (no machine): windows/arm64 `native.sh`; Linux runs the new `gogui`/`native` tests
  with its next log.

**S2.4 Win32 GPU rendering** (S2.4a–S2.4f, split 2026-10-16) ✅ *2026-10-21*
* `shell/win32/com`, D3D11, a DXGI flip-model swap chain, a DComp target, a D2D device
  context implementing `render.Canvas`, a paced frame clock and synchronous resize.
* Done when: traffic lights and placeholders are drawn and live resize doesn't flicker.
* All of it runs on the Windows host (GPU and desktop; D3D11's WARP device where hardware
  is missing). windows/arm64 matters more than before (float arguments), so each Result
  lists its `native.sh` run as "not run (no machine)" until there is one.
* **Design decisions (confirmed by the user 2026-10-16: DComp composition swap chains,
  the `DrawText` placeholder box until S2.5, the complete canvas incl. S2.4e):**
  * *Float arguments go through purego (S2.4a).* D2D methods take `FLOAT`s and small float
    structs by value (`SetDpi`, stroke widths, `D2D1_POINT_2F`). `syscall.SyscallN`
    works on amd64 but its windows/arm64 trampoline loads only integer registers. `com`
    binds float-taking methods with `purego.RegisterFunc` on the vtable slot address;
    integer-only methods stay on `SyscallN`. Callbacks stay on `syscall.NewCallback`
    (`ffi`, S0.10b).
  * *Bindings by interface family.* `com` holds the `comptr` core; `dxgi`, `d3d11`,
    `dcomp`, `d2d` (and `dwrite` in S2.5) hold slot indices, methods, constants and
    mirrored structs, each with layout tests on amd64/arm64 and a slot table in header
    order (a wrong slot is the typical COM bug). Only what is used is bound.
  * *One device, one swap chain per surface (S2.4c).* The shell owns one D3D11 device
    (BGRA support, hardware then WARP), its DXGI device, one `ID2D1Device`/
    `ID2D1DeviceContext` and one `IDCompositionDevice`. Each surface has a composition
    swap chain (flip-sequential, `B8G8R8A8_UNORM`, premultiplied alpha, 2 buffers,
    waitable) on a DComp target and visual, and a D2D bitmap over the back buffer. Device
    loss (`DXGI_ERROR_DEVICE_REMOVED/RESET`, `D2DERR_RECREATE_TARGET`) recreates the
    device and every chain, then repaints.
  * *Canvas in two halves.* S2.4d: state, transforms, clips, solid/gradient fills and
    strokes, paths and `DrawText` (a thin placeholder box of the layout's size until
    S2.5), which is everything aqua chrome and placeholders call. S2.4e: `DrawShadow`
    (D2D shadow effect on a cached mask), `PushOpacity` (`PushLayer`), `PushBackdropBlur`
    (Gaussian blur on a copy of the target), `DrawImage` (only the shell's own bitmaps).
    The canvas narrows float64 to float32, applies the surface scale with `SetDpi`,
    ignores contract violations (§5.3), and with `GOGUI_DEBUG=1` also runs the recorder's
    checks.
  * *Tests compare pixels, not calls.* Canvas tests draw on an offscreen D2D bitmap and
    read it back through a CPU-readable staging texture, checking pixels with a small
    tolerance. Golden PNGs stay S3.6's.
  * *Clock and resize (S2.4f).* A frame waits on the swap chain's frame-latency waitable
    (`MsgWaitForMultipleObjectsEx` in the loop, so input keeps flowing), falling back to
    DWM's vblank timing (`DwmGetCompositionTimingInfo` on a timer, not the blocking
    `DwmFlush`; S2.4f) with no visible surface. `WM_SIZE` resizes the buffers synchronously
    and paints before returning (`ResizeBuffers`, paint, `Present`,
    `IDCompositionDevice::Commit`), also in the modal size loop (§13).

**S2.4a COM core and float call path** (`win32/com`) ✅ *2026-10-16*
* `Ptr` with `Slot`/`Call`/`CallHR`, `AddRef`, `Release` (clears, nil-safe), `RefCount`,
  `QueryInterface`; `Bind`/`BindFunc` (float path via `purego.RegisterFunc`, cached by
  func type and code address; rejects Go func, variadic, func and struct results, since
  MSVC returns member-function structs through a hidden pointer); `GUID`/`ParseGUID`/
  `MustGUID`; `HRESULT` as an error (`Failed`, `Facility`, `Code`, named codes incl. DXGI
  device loss and `D2DERR_RECREATE_TARGET`); `InitUIThread` (STA; `Shell.Run` calls it
  after `LockOSThread` and `CoUninitialize` on exit) and `CreateInstance`. Vtable reads go
  through `ffi.ReadUintptr`.
* Tests: ref counts through WIC's imaging factory; the float path against `ucrtbase`
  (`fmaf`, `fma`, `ldexp`, `scalbnf`) and `d2d1`'s flat exports (`D2D1Vec3Length`,
  `D2D1MakeRotateMatrix` with a by-value `D2D1_POINT_2F`).
* Findings: WIC answers a NULL `QueryInterface` out pointer with `E_INVALIDARG`; purego's
  windows/arm64 path has its own trampoline with float registers (it skips
  `syscall.SyscallN`), which makes the decision work. §1, §2.1, §3, §9, D5 and
  ARCHITECTURE.md updated for purego on Windows; `go.mod` unchanged.
* Not run (no machine): `native.sh` on windows/arm64, the only run of the float path's
  arm64 ABI.

**S2.4b DXGI, D3D11 and DComp bindings** (`win32/dxgi`, `d3d11`, `dcomp`) ✅ *2026-10-16*
* `dxgi`: `DeviceFrom` → `Adapter` (`Desc`, `IsWARP`) → `Factory` (`IDXGIFactory2` via
  `GetParent`, so no dxgi.dll proc), `CreateSwapChainForComposition`, `SwapChain1`
  (`Present` returning the `HRESULT` for `STATUS_OCCLUDED`, `Buffer`, `ResizeBuffers`,
  `Desc1`), `SwapChain2` (frame latency, waitable). `d3d11`: `CreateDevice` (hardware with
  BGRA, then WARP; `*CreateError` with both failures), `Device`, `DeviceContext`,
  `Texture2D`. `dcomp`: `CreateDevice`, `Commit`, `CreateTargetForHwnd`, `CreateVisual`,
  `SetRoot`, `SetContent`. All bound methods are integer-only.
* New helpers: `com.Vtable`/`CheckSlots` (each package's tests hold the full vtable of
  every bound interface in header order and check every slot constant); `ffi.CopyBytes`.
  No SDK headers on the host, so vtable orders came from two independent bindings in the
  module cache (ebiten, gio; read only) and every IID is checked live through
  `QueryInterface`.
* Smoke test (default hardware device, feature level 11_1, and WARP): a 64×48 chain on a
  hidden `WS_EX_NOREDIRECTIONBMP` popup, cleared, read back at five pixels, presented,
  committed, resized, torn down with the device at 0.
* Findings: `SetContent` doesn't change the chain's own count; the adapter holds its
  factory and the device its adapter, so both reach 0 only with the device;
  `ResizeBuffers` fails with `DXGI_ERROR_INVALID_CALL` while a back-buffer reference lives.
* Not run (no machine): windows/arm64 layout tests.

**S2.4c D2D bindings and the surface pipeline.** Split on 2026-10-16 into S2.4c_1 and
S2.4c_2 (≈1,500–2,500 lines with tests, in halves of different risk). Together the
done-when is: a surface presents its background in light and dark (read back from the
buffer), a resize recreates the buffer at the new pixel size, a simulated device loss
recreates the device and repaints, closing every surface leaves no live COM references,
`examples/hello` still opens its window (smoke run), and `check.sh` passes. ✅ when both
substeps are. ✅ *2026-10-19* (both substeps done).

**S2.4c_1 D2D bindings** (`win32/d2d`) ✅ *2026-10-19*
* `D2D1CreateFactory` (`ID2D1Factory1`, single-threaded), `Factory1::CreateDevice` (from
  S2.4b's `IDXGIDevice`), `ID2D1Device::CreateDeviceContext`, `ID2D1DeviceContext`
  (`CreateBitmapFromDxgiSurface`, `SetTarget`, `BeginDraw`, `EndDraw`, `SetDpi`, `GetDpi`,
  `Clear`), `ID2D1Bitmap1`. Constants (`D2D1_BITMAP_OPTIONS_TARGET | CANNOT_DRAW`,
  `D2DERR_RECREATE_TARGET`), structs `D2D1_FACTORY_OPTIONS`, `D2D1_PIXEL_FORMAT`,
  `D2D1_BITMAP_PROPERTIES1`, `D2D1_COLOR_F`. `SetDpi` (two floats) is the first real use
  of the float path; everything else, `Clear` included (color by pointer), uses
  `SyscallN`. The proc comes through `NewLazySystemDLL("d2d1.dll")`, resolved by a test.
  Only what S2.4c uses is bound.
* Done when: layout tests for every mirrored struct (arm64 via `native.sh`); slot tables
  checked with `com.CheckSlots`; a smoke test on a hidden window creating the S2.4b
  device and chain plus the D2D factory, device, context and a target bitmap over the back
  buffer, `SetDpi(144, 144)` read back with `GetDpi`, a clear to a known color, `EndDraw`,
  `Present`, the color read back, and every ref count at zero; ARCHITECTURE.md describes
  `d2d`; `check.sh` passes.
* **Result:** `d2d` (`doc.go`, `d2d.go`, `types.go`): `CreateFactory`, `Factory1.CreateDevice`,
  `Device.CreateDeviceContext`, `DeviceContext` (`CreateBitmapFromDxgiSurface`,
  `SetTarget`/`Target`, `BeginDraw`, `EndDraw`, `Clear`, `SetDpi` via `com.Bind`, `Dpi`),
  `Bitmap1` (`Options`, `Dpi`, `Surface`). Vtables from the `d2d1.h`/`d2d1_1.h`
  declaration order (no SDK headers and no D2D binding in the module cache), checked by
  `CheckSlots` and live: every IID, `GetFactory` (slot 3) and `GetDevice` (slot 73) by
  IUnknown identity. Smoke test on the default device and WARP: DPI 96 at start, 144 on
  the bitmap, `SetDpi` round trips, an opaque and a half-alpha clear read back
  premultiplied, `Present`, `Commit`, `ResizeBuffers` once the bitmap is gone, every
  count at 0. Findings: a bitmap set as target holds three back-buffer references
  (released by `SetTarget(0)` and the bitmap's `Release`); an unpaired `EndDraw` fails
  without `D2DERR_RECREATE_TARGET`. `go.mod` unchanged.
* Not run (no machine): `native.sh` on windows/arm64 (the layout tests and `SetDpi`, the
  first real arm64 float-path call).

**S2.4c_2 Surface pipeline and device loss** (`internal/shell/win32`) ✅ *2026-10-19*
* The shell's device objects (D3D11 device, DXGI device, `ID2D1Device`/`Context`,
  `IDCompositionDevice`) are created lazily with the first surface and released after the
  last. Each surface gets the composition swap chain, DComp target and visual, and the
  D2D target bitmap. Resize releases the target bitmap before `ResizeBuffers` and
  recreates it at the new pixel size; a DPI change updates `SetDpi`. `WM_PAINT` and the
  frame paint clear the back buffer through D2D to the provisional window background
  (light/dark) and present. `Paint` still draws on the thrown-away recorder
  (`interim_paint.go` keeps only that until S2.4d). Device loss (`DXGI_ERROR_DEVICE_REMOVED/
  RESET` from `Present` or `ResizeBuffers`, `D2DERR_RECREATE_TARGET` from `EndDraw`)
  releases every surface's chain and the device objects, recreates them and repaints every
  surface. A test-only hook injects the error.
* Done when: the S2.4c criteria above; ARCHITECTURE.md describes the rendering pipeline;
  `check.sh` passes.
* **Result:** `gpu.go` (the shell's devices: `newGPU`/`release`, `Shell.device`,
  `releaseGPU`, `isDeviceLost` (`DEVICE_HUNG` too), `deviceLost`, leak accounting, the
  `fault` hook) and `present.go` (per surface `createChain`, `createBitmap`,
  `releaseChain`, `fitChain`, `paint`/`render`). The windows are now
  `WS_EX_NOREDIRECTIONBMP`; `sync` fits the chain before `Resized`; a `NewSurface` whose
  chain fails destroys its window and returns the error; an appearance change
  invalidates every surface; the context drops its target after each frame.
  `interim_paint.go` keeps only `recordPaint`. Tests (`present_test.go`): light/dark
  background read back between `EndDraw` and `Present` (an `onDrawn` hook), chain and
  bitmap size/DPI inside `Resized`, after `SetSize` and a synthetic `WM_DPICHANGED`,
  device loss injected into `EndDraw`, `Present` (removed, reset) and `ResizeBuffers`
  with two surfaces, the device lifetime; every `inShell` test also requires no devices
  and no leftover references after `Run`. Smoke run: `examples/hello` opens a responding
  window titled "Hello" with nothing logged under `GOGUI_DEBUG=1`.
* Fixed on the way: the S2.4c_1/S2.4b smoke tests destroyed their window in `t.Cleanup`,
  which can run on another thread after `UnlockOSThread` (`DestroyWindow`: access
  denied; seen once in `native.sh`'s WARP run); `hiddenWindow` now returns a destroy
  func that the test defers while locked.
* `check.sh`: each shell test now creates a hardware D3D11 device (≈0.25 s), so the
  stress run gets `-timeout 30m` and skips `TestSurfaceDeviceLoss` (single-threaded,
  ≈2 s per run). One full stress run hit two 45 s stalls in `Run`
  (`TestSurfaceNCPointer`, `TestSurfaceGPULifetime`); they did not reproduce in 80
  targeted runs or a second full run, and `run`'s 10 s timeout now prints every
  goroutine's stack in case they come back. `go.mod` unchanged.
* Not run (no machine): `native.sh` on windows/arm64. **For the user, on Windows:** look
  at `examples/hello` in light and dark mode (the background follows a mode switch while
  open), resize it live, and drag it between monitors with different scaling.

**S2.4d D2D canvas I: state, geometry, fills and strokes** (`internal/shell/win32`) ✅ *2026-10-20* (as S2.4d_1–S2.4d_3)
* `render.Canvas` on the device context: `Save`/`Restore` (a Go-side stack over
  `SetTransform` and axis-aligned or layer clips), `Translate`/`Scale`/`Concat`,
  `ClipRoundedRect` (`PushAxisAlignedClip` for sharp untransformed rects, else a geometry
  layer), `FillRoundedRect`/`StrokeRoundedRect` (`FillRoundedRectangle` for uniform radii,
  a path geometry otherwise), `FillPath`/`StrokePath` (`ID2D1PathGeometry` from
  `render.Path`, nonzero fill, caps, joins, dashes), solid and linear-gradient brushes
  (cached per paint within a frame), and `DrawText` as the placeholder box until S2.5.
  `interim_paint.go` is deleted.
* Done when: offscreen pixel tests per primitive at scale 1 and 2 (fill colors, gradient
  ends, stroke centred on the outline, clip and transform nesting, Save/Restore balance
  and the ignored misuses of §5.3); `examples/hello` shows the title bar and traffic
  lights and `examples/controls` its placeholders (smoke run, screenshot taken by Claude
  through `PrintWindow` or a readback); `check.sh` passes.
* Split on 2026-10-19 into S2.4d_1–S2.4d_3 (≈2,000–3,000 lines with tests: about thirty
  new bindings, the canvas, and the switch of the surfaces to it, each with its own risk).
  The done-when above is the sum of the three; ✅ when all three are. Everything runs on
  the Windows host; no step needs macOS or Linux. Only windows/arm64 stays "not run (no
  machine)": S2.4d_1 adds the first float-path calls with a struct by value
  (`BeginFigure`, `AddLine`).

**S2.4d_1 D2D drawing bindings** (`win32/d2d`) ✅ *2026-10-19*
* Context (`ID2D1RenderTarget`/`ID2D1DeviceContext` slots): `SetTransform`/`GetTransform`,
  `PushAxisAlignedClip`/`PopAxisAlignedClip`, `PushLayer` (the `D2D1_LAYER_PARAMETERS1`
  overload, NULL layer, as allowed on a device context)/`PopLayer`, `CreateSolidColorBrush`,
  `CreateGradientStopCollection`, `CreateLinearGradientBrush`, `FillRectangle`,
  `FillRoundedRectangle`, `DrawRoundedRectangle`, `FillGeometry`, `DrawGeometry`. Factory:
  `CreatePathGeometry`, `CreateRoundedRectangleGeometry`, `CreateStrokeStyle`
  (`ID2D1Factory1`, `D2D1_STROKE_STYLE_PROPERTIES1`). Interfaces: `ID2D1Brush`,
  `SolidColorBrush`, `LinearGradientBrush`, `GradientStopCollection`, `Geometry`,
  `PathGeometry` (`Open`), `GeometrySink` (`BeginFigure`, `AddLine`, `AddBezier`,
  `AddQuadraticBezier`, `EndFigure`, `SetFillMode`, `Close`), `StrokeStyle`. Structs
  `D2D1_MATRIX_3X2_F`, `RECT_F`, `POINT_2F`, `ROUNDED_RECT`, `BEZIER_SEGMENT`,
  `QUADRATIC_BEZIER_SEGMENT`, `BRUSH_PROPERTIES`, `GRADIENT_STOP`,
  `LINEAR_GRADIENT_BRUSH_PROPERTIES`, `STROKE_STYLE_PROPERTIES1`, `LAYER_PARAMETERS1`.
* Float path (`com.Bind`): `DrawRoundedRectangle` and `DrawGeometry` (a `FLOAT` width),
  `BeginFigure` and `AddLine` (`D2D1_POINT_2F` by value). Everything else passes pointers
  and integers and stays on `SyscallN`. No method that returns a struct is bound.
* Enum traps, written down here so the canvas maps them and doesn't cast: `D2D1_CAP_STYLE`
  is FLAT, SQUARE, ROUND (not render's Butt, Round, Square order); `D2D1_LINE_JOIN` is
  MITER, BEVEL, ROUND, MITER_OR_BEVEL (render's Miter means MITER_OR_BEVEL, the CG
  behaviour); custom dashes and the dash offset are in units of the stroke width.
* Done when: layout tests for every new struct (arm64 via `native.sh`); slot tables
  extended and checked with `com.CheckSlots`, every new IID checked live; a native test
  on an offscreen target bitmap (a D3D11 render-target texture through
  `CreateBitmapFromDxgiSurface`, read back through staging) that fills a rect and a
  rounded rect, strokes a path with a dash style, fills through a gradient, clips with an
  axis-aligned clip and a geometry layer, and checks a few pixels each, every count at 0;
  ARCHITECTURE.md describes the additions; `check.sh` passes.
* **Result:** `draw.go` (the bindings above; `Brush` and `Geometry` are base views
  without their own reference, the concrete types convert with `.Brush()`/`.Geometry()`;
  getters `Transform`, `SetColor`, `Stops`, stop `Count`/`Gamma`/`ExtendMode`, path
  `FigureCount`/`SegmentCount`, stroke-style `Caps`/`LineJoin`/`DashStyle`/`DashesCount`/
  `TransformType` for the tests; helpers `InfiniteRect`, `IdentityMatrix`,
  `DefaultBrushProperties`), `bind.go` (`floatMethod`: each float-path method keeps its
  last bound func per code address behind an atomic pointer, so a call per path segment
  skips `com.Bind`'s lock and map; `SetDpi` uses it too), the enums and eleven structs in
  `types.go`. Tests (`draw_test.go`): full vtables of the seven new interfaces with
  every slot checked, layouts, every new IID live, and `TestDraw` on the default device
  and WARP (a 64×64 render-target texture, read back after each frame: rect fill and
  `SetColor`, rounded-rect corners, a 4-wide rounded-rect stroke centred on the outline,
  a translation, a clamped gradient's ends and middle, a path with line, cubic and
  quadratic segments, a `{2, 2}` custom dash at width 4, an axis-aligned clip with a
  nested circle-masked layer, a layer under a world transform), every count at 0. Each
  pixel check was mutation-tested (a wrong slot, a reversed gradient, other dashes, a
  wider clip, a square mask, a thinner stroke each fail the test).
* Fixed on the way: `d2d_test.go`'s `near` compared bytes with wraparound
  (`0-255 == 1`), so black matched white; it now compares in ints. S2.4c_1's smoke test
  still passes with it. `go.mod` unchanged.
* Not run (no machine): `native.sh` on windows/arm64 (the layout tests and the first
  float-path calls with a struct by value, `BeginFigure` and `AddLine`).

**S2.4d_2 The D2D canvas** (`internal/shell/win32`, `canvas.go`) ✅ *2026-10-19*
* `render.Canvas` on the shell's device context, used between `BeginDraw` and `EndDraw`:
  the Go-side state stack (transform, plus how many clips and layers each level pushed,
  popped in LIFO order on `Restore` and all at the end of the frame), `Translate`/`Scale`/
  `Concat` through `SetTransform` (the surface scale stays in `SetDpi`),
  `ClipRoundedRect` (`PushAxisAlignedClip` for sharp rects under an axis-preserving
  transform, else a layer with a rounded-rect geometry mask), `FillRoundedRect`/
  `StrokeRoundedRect` (`Radii.Clamped`; the D2D rounded rect for uniform radii, a path
  geometry otherwise), `FillPath`/`StrokePath` (`render.Path` → `ID2D1PathGeometry`, winding
  fill, caps/joins/miter/dashes mapped per S2.4d_1), brushes from `Paint.Normalized` (solid
  by color, gradient stop collections by stops, gamma 2.2, clamp extend; cached for one
  frame and released at its end), and `DrawText` as a thin box the layout's size until
  S2.5. S2.4e's methods are stubs that keep the stack balanced: `PushOpacity`/
  `PushBackdropBlur` save, `Pop` restores, `DrawShadow`/`DrawImage` draw nothing.
  Float64 is narrowed to float32 at the call. Contract violations (§5.3: non-finite
  values, negative sizes, semantic colors, unbalanced Restore/Pop, leftover saves) are
  ignored without D2D errors. With `GOGUI_DEBUG=1` every call is also sent to a
  `record.Canvas`, whose `End` errors are logged.
* Done when: offscreen pixel tests per primitive at scale 1 and 2 (DPI 96 and 192): fill
  colors, gradient ends and padding, stroke centred on the outline (inside and outside
  pixels), round caps and dashes, non-uniform radii, clip and transform nesting (a sharp
  clip under a rotation takes the layer path), Save/Restore balance, every ignored misuse
  (no failed `EndDraw`), brushes and geometries all released after a frame (counts); the
  canvas isn't hooked up to any surface yet; `check.sh` passes.
* **Result:** `canvas.go`: `newCanvas(dc, factory, size, debug)` after `BeginDraw`, `finish`
  before `EndDraw` (pops every open clip and layer, resets the transform, ends the canvas,
  returns the recorder's `End` errors), `release(leak)` after it; `failures` collects
  resource creation errors. Stack frames hold the transform and the number of D2D pushes;
  a `Restore` that doesn't close a `Save` (or a `Pop` a `Push*`) is ignored. Sharp clips
  under a transform without rotation or skew (`B = C = 0`) are `PushAxisAlignedClip`, the
  rest a layer with a rounded-rect (uniform) or path (per-corner, built with `ArcTo`)
  mask; a negative or NaN clip size clips everything away (the recorder's empty
  intersection). Fills use `FillRectangle`/`FillRoundedRectangle`/`FillGeometry`; strokes
  `DrawRoundedRectangle` or `DrawGeometry`. Caches per frame: solid brushes by clamped
  color (transparent ones draw nothing), stop collections by stops, gradient brushes by
  stops and end points, stroke styles by mapped cap/join/miter/dashes (dashes and offset
  divided by the width); geometries are per call. Every float64 is narrowed at the call,
  and a value that doesn't narrow to a finite float32 drops the call (`Translate(1e300, 0)`
  included). `DrawText` strokes a 1 pt box inside the layout's rect.
* Tests (`canvas_test.go`): a 128×128 px render-target texture on the shell's own
  `newGPU` devices at DPI 96 and 192, read back per frame: fill colors and half alpha,
  brush reuse, a clamped circle and a one-corner rounded rect (path), gradient padding at
  both ends, middle and monotony, single-stop and stopless gradients, a 4 pt stroke
  centred on the outline (inside and outside pixels), a per-corner stroke, butt/round/
  square caps, dashes with and without offset, the cap/join/dash mapping read back from
  the stroke styles, nonzero fill with an implicitly closed subpath, quadratic and cubic
  fills, nested transforms with Save/Restore, nested sharp clips, a rotated sharp clip
  taking the layer path (diamond), a rounded clip, open saves/clips closed by `finish`,
  the S2.4e stubs, the text box, every misuse (no failed `EndDraw`, the state intact,
  the recorder reporting them with the debug flag) and a clean debug paint; after each
  frame every resource is released at count 0. Mutation-checked: dashes not divided by
  the width, round caps mapped to square, and every clip taken as axis-aligned each fail.
  `go.mod` unchanged.
* Not run (no machine): `native.sh` on windows/arm64.

**S2.4d_3 Surfaces paint through the canvas** (`internal/shell/win32`) ✅ *2026-10-20*
* `surface.render` clears to the background, runs `Paint` on the S2.4d_2 canvas inside
  the same `BeginDraw`/`EndDraw`, and frees the frame's resources before `Present`.
  `paintErr` comes from the debug recorder only. `interim_paint.go` is deleted, along with
  its test. Device loss in the middle of a frame drops the frame's cache with the context.
* Screenshot: a test-only capture of the presented back buffer to PNG (the `onDrawn`
  readback), and as a cross-check `PrintWindow` with `PW_RENDERFULLCONTENT` on a visible
  `examples/hello` and `examples/controls`; Claude looks at both PNGs.
* Done when: present tests read back the title bar color, a traffic light and a
  placeholder's fill at scale 1 and 2; the S2.4c_2 device-loss and lifetime tests still
  pass, with no leftover references; the smoke runs of `examples/hello` (title bar and
  traffic lights) and `examples/controls` (placeholders) are screenshotted and checked;
  ARCHITECTURE.md (rendering pipeline, `interim_paint.go` gone), the PLAN.md status and
  S2.4d's ✅; `check.sh` passes. **For the user, on Windows:** look at both examples in
  light and dark at 100 % and one other scale.
* **Result:** `present.go`'s `render` clears, runs `Paint` on a `newCanvas` in the same
  `BeginDraw`/`EndDraw`, calls `finish`, `EndDraw`, `SetTarget(0)` and `release` (before
  `Present`, and before a lost device's recovery releases the context), so no frame
  resource outlives its frame; a panicking `Paint` still ends the draw and releases the
  frame. `render` returns `(paintErr, err)`: `paintErr` is the recorder's, only with the
  new `Shell.debug` (from `GOGUI_DEBUG=1`; every test shell sets it), and `err` includes
  the canvas's resource creation failures. `interim_paint.go` and `TestRecordPaint` are
  gone. `w32` gains the capture bindings for the tests (`PrintWindow`, `FindWindowExW`,
  `CreateCompatibleDC`/`DeleteDC`, `CreateDIBSection`, `SelectObject`, `GdiFlush`,
  `BITMAPINFOHEADER` with its layout test). Tests (`aqua_test.go`): an `aqua.Window`
  with a placeholder button on a real surface, read back between `EndDraw` and
  `Present` in light/dark × key/inactive at DPI 96 and 192: frame size, title-bar fill,
  close light (red or grey), the gap between the lights, content background,
  placeholder fill (white, or white at 25 % over 50 grey) and the background beside it,
  no recorder or rendering errors; `GOGUI_SCREENSHOTS=dir` writes every frame as a PNG.
  `TestCaptureWindow` (only with `GOGUI_CAPTURE=title=file.png`) captures another
  process's window through `PrintWindow(PW_CLIENTONLY | PW_RENDERFULLCONTENT)`. The
  S2.4c_2 background, resize, device-loss and lifetime tests pass unchanged, with no
  leftover references. Smoke runs: `examples/hello` and `examples/controls` with
  `GOGUI_DEBUG=1` (nothing logged), captured with `TestCaptureWindow`; Claude looked at
  both PNGs and the eight readback PNGs: title bar with the three lights and the title
  box, the placeholders with their outlines and text boxes, the content background, all
  in the host's dark mode, and light/dark/key/inactive at both scales in the readbacks.
  `screenshots/` is git-ignored. `go.mod` unchanged.
* Not run (no machine): `native.sh` on windows/arm64. **For the user, on Windows:** look
  at `examples/hello` and `examples/controls` in light and dark at 100 % and one other
  scale (title bar, traffic lights, placeholders; text is still boxes until S2.5).

**S2.4e D2D canvas II: shadows, layers, blur and images** (`internal/shell/win32`) ✅ *2026-10-20* (as S2.4e_1–S2.4e_4)
* `DrawShadow` (outer and inset, blur and spread, via the D2D shadow effect on a
  rounded-rect mask, cached per size and radii within a frame), `PushOpacity`
  (`PushLayer`), `PushBackdropBlur` (copy of the target region, Gaussian blur, clipped to
  the rounded rect), `Pop`, and `DrawImage` for bitmaps the shell creates (a `win32`
  `Image` type; other `Image`s are ignored).
* Done when: offscreen pixel tests (shadow outside but not inside the shape and the
  reverse for inset, falloff with blur radius, group opacity differing from per-shape
  opacity on overlaps, blurred backdrop inside the clip and sharp outside, image scaled
  to `dst`); `check.sh` passes.
* Split on 2026-10-20 into S2.4e_1–S2.4e_4 (≈2,500–3,000 lines with tests: about fifteen
  new bindings for effects and bitmaps, then three canvas features, each with its own
  risk). The done-when above is the sum of the four; ✅ when all four are. Everything
  runs on the Windows host; no step needs macOS or Linux. Only windows/arm64 stays "not
  run (no machine)": S2.4e_1 adds one new float-path call (`DrawBitmap`'s opacity).
* Settled for all four:
  * *Masks on a second device context.* Shadow masks are drawn on a mask-only
    `ID2D1DeviceContext` made from the shell's `ID2D1Device` (bitmaps are shared within
    a device), so the main context never changes target while clips or layers are
    pushed. It lives in `gpu` with the other device objects and is recreated on device
    loss.
  * *Backdrop copy.* The back-buffer bitmap is `TARGET | CANNOT_DRAW`, so it can't be an
    effect input. `PushBackdropBlur` copies the region into a scratch bitmap with
    `ID2D1Bitmap::CopyFromBitmap` (allowed from a `CANNOT_DRAW` bitmap) and blurs the
    copy. S2.4e_1 checked that the copy inside `BeginDraw` sees what was drawn before it:
    it does after a `Flush` (and doesn't without one), so the fallback (an intermediate
    bitmap copied to the back buffer) is not needed.
  * *No struct return values* (as in S2.4d_1): sizes come from the shell's own
    bookkeeping, not from `GetSize`/`GetPixelSize`.

**S2.4e_1 D2D effect and bitmap bindings** (`win32/d2d`) ✅ *2026-10-20*
* Context (`ID2D1DeviceContext` slots): `CreateBitmap` (`D2D1_SIZE_U` by value, two
  `UINT32`s in one integer register, with source pixels and `D2D1_BITMAP_PROPERTIES1`),
  `CreateEffect`, `DrawImage` (image, offset and source rect by pointer, interpolation,
  composite mode), `DrawBitmap` (dest/source rects by pointer, `FLOAT` opacity on the
  float path). Device: a second `CreateDeviceContext` (already bound). Interfaces:
  `ID2D1Image` (a base view like `Brush`; `Bitmap1` converts with `.Image()`),
  `ID2D1Effect` with its `ID2D1Properties` base (`SetValue` by index and
  `D2D1_PROPERTY_TYPE`, `SetInput`, `SetInputCount`, `GetOutput`), `ID2D1Bitmap`'s
  `CopyFromBitmap` (`D2D1_POINT_2U` and `D2D1_RECT_U` by pointer). `CLSID_D2D1Shadow` and
  `CLSID_D2D1GaussianBlur` with their property indices (standard deviation, color,
  optimization, border mode) and the enums `D2D1_INTERPOLATION_MODE`,
  `D2D1_COMPOSITE_MODE`, `D2D1_BORDER_MODE`, `D2D1_PROPERTY_TYPE`. Structs `SIZE_U`,
  `POINT_2U`, `RECT_U`, `VECTOR_4F`.
* Done when: layout tests for every new struct (arm64 via `native.sh`); slot tables
  extended and checked with `com.CheckSlots`, every new IID and CLSID checked live; a
  native test on the S2.4d_1 offscreen target, on the default device and WARP: a bitmap
  from CPU pixels drawn scaled with `DrawBitmap` (corner and centre pixels, half
  opacity), a shadow effect on a bitmap mask (dark outside the mask, falloff widening
  with the standard deviation), a Gaussian blur with hard borders on a sharp edge,
  `CopyFromBitmap` from the `CANNOT_DRAW` target inside `BeginDraw` (the copy matches
  what was drawn), a group-opacity `PushLayer`, a second device context drawing into a
  bitmap the first one then draws, every count at 0; each pixel check mutation-tested;
  ARCHITECTURE.md describes the additions; `check.sh` passes.
* **Result:** `effect.go` with the bindings above plus `ID2D1RenderTarget::Flush`
  (slot 42, see the finding), the typed property helpers `SetFloat`/`SetEnum`/
  `SetVector4`, `Float`/`Enum`/`Vector4`, `CLSID` (system property `PROPERTY_CLSID`),
  `PropertyCount`, `Type`, `InputCount`; `CreateBitmap` rejects too few source bytes
  before the call; `DrawBitmap` is `ID2D1DeviceContext`'s overload (slot 85, no
  perspective). Enums, property indices and the four structs in `types.go`, `SizeU`
  packed with Width in the low half. Tests (`effect_test.go`): full vtables of
  `ID2D1Properties` (14) and `ID2D1Effect` (19), the new context and bitmap slots, the
  layouts and the packing, every IID and CLSID live, and `TestEffects` on the default
  device and WARP: a 2×2 CPU bitmap through `DrawBitmap` (quadrants, border, half
  opacity, linear mixing, a one-texel source rect) and `DrawImage`; the shadow on a
  square mask (solid under it at σ 1, dark just outside, monotone falloff, farther at
  σ 4; property types and values read back, a wrongly typed set refused); a hard-border
  blur over red (edges keep the edge colours, the edge intermediate and increasing);
  `CopyFromBitmap` from the `CANNOT_DRAW` target inside `BeginDraw` with and without
  offsets; group against per-shape opacity on an overlap; a second context drawing a
  16×8 bitmap the first draws; every count at 0. Each check mutation-tested (17
  mutations, all fail). `go.mod` unchanged.
* **Finding, settles the backdrop copy:** inside `BeginDraw`, `CopyFromBitmap` from the
  context's own target reads what the GPU holds, not the batched commands; without a
  `Flush` the copy is the previous frame. With `Flush` first it matches what was drawn,
  on hardware and WARP, so S2.4e_3 flushes and copies; the intermediate-bitmap fallback
  is dropped. Also: `DrawImage`'s source rect lands with its corner at the offset.
* Not run (no machine): `native.sh` on windows/arm64 (the layouts and `DrawBitmap`'s
  float opacity).

**S2.4e_2 Group opacity and shadows** (`internal/shell/win32`, `canvas.go`, `gpu.go`) ✅ *2026-10-20*
* `PushOpacity`: a layer with `render.Opacity(alpha)` and infinite bounds, on the push
  stack so `Pop` and `finish` close it. An alpha of 1 pushes a state frame but no layer.
* `DrawShadow`: `Shadow.Normalized`, then on the mask context at the surface DPI a mask
  of the shape grown by `Spread` (inset: a ring, the shape shrunk by `Spread` cut out of
  a larger rect), padded by 3σ. The shadow effect gets σ = `Blur/2` and the color, and
  its output is drawn at `Offset` under the current transform, clipped by a geometry
  layer to the outside of the shape (outer) or its inside (inset). Masks are cached
  within the frame by size, radii, spread, blur and inset, effects by color, and
  `release` frees them. A transparent or semantic color or an empty shape draws nothing.
* Done when: offscreen pixel tests at DPI 96 and 192: group opacity differs from
  per-shape opacity on an overlap, nested opacity multiplies, `finish` closes an open
  opacity layer; an outer shadow is dark just outside and leaves the inside untouched
  (a translucent fill over it shows only the background), an inset one the reverse;
  the falloff is monotone and widens with the blur; spread grows and shrinks the edge;
  the offset shifts it; a shadow under a transform follows it; a repeated shadow reuses
  its mask (counted); every resource is released at count 0 after the frame;
  mutation-checked (no clip, σ = `Blur`, inset not inverted); `check.sh` passes.
* **Result:** `gpu.mask`, a second `ID2D1DeviceContext` of the shell's device, created
  and released with the others (so device loss recreates it). `newCanvas(gpu, size, dpi,
  debug)` replaces `newCanvas(dc, factory, size, debug)`. `PushOpacity` pushes a layer
  (`render.Opacity`, NaN is 0, infinite bounds) on the push stack, none at 1. New
  `shadow.go`: `DrawShadow` as specified, the outer mask a uniform rounded rect or a path
  (rounded corners grow with the spread, sharp ones stay sharp), the inset ring and both
  clips even-odd paths (`pathGeometryMode`, `addRoundedRect` split out of
  `roundedRectPath`); the ring's margin also covers the offset and a negative spread. The
  shadow effect's input and properties are set once per cache entry, never changed
  within a frame. Masks are capped at 16384 px a side. Tests (`shadow_test.go`, DPI 96
  and 192): group vs. per-shape opacity on an overlap, nested opacity, opacity 1 and NaN,
  `finish` closing an open layer; the outer shadow (dark just outside, a translucent fill
  over the shape shows only the background, monotone falloff, about 84 % white at 1σ and
  white at 3σ for σ = `Blur/2`, wider with a larger blur); the inset one (dark just
  inside, untouched outside, monotone, a spread band, an offset band on one side only);
  spread growing, shrinking with an offset and shrinking away; shadows under translate +
  scale and under a 45° rotation; two masks, three effects, two mask hits for four
  shadows, nothing cached for empty or invalid ones; a shadow inside opacity and a clip,
  clean for the recorder. `TestCanvasBalance` now covers `PushOpacity(1)` and a covered
  inset shadow instead of the stubs. Mutations (each fails at least one test): no clip,
  σ = `Blur`, inset clip not inverted, inset mask not a ring, the mask ignoring the DPI,
  no mask cache, the layer opacity ignored. `go.mod` unchanged.
* Not run (no machine): `native.sh` on windows/arm64 (no new float-path calls; the effect
  properties go through `SetValue` with byte buffers).

**S2.4e_3 Backdrop blur** (`internal/shell/win32`, `canvas.go`) ✅ *2026-10-20*
* `PushBackdropBlur`: the device-pixel bounds of the transformed rounded rect, padded by
  3σ and intersected with the target, are copied (`Flush`, then `CopyFromBitmap`, per S2.4e_1's
  finding) into a scratch bitmap kept per target, grown when needed and recreated on
  device loss and DPI change. The copy goes through the Gaussian blur (σ =
  `BlurRadius/2`, hard borders) and is drawn back under the identity transform at its
  pixel origin, inside a geometry layer of the shape under the current transform. That
  layer stays pushed until `Pop`, so content drawn on top is clipped to the shape too.
  A zero radius only pushes the clip; an empty shape clips everything away.
* Done when: offscreen pixel tests at DPI 96 and 192: a sharp stripe pattern behind the
  shape is blurred inside the clip (intermediate values) and exact outside it; the edge
  of the target doesn't darken (hard borders); content drawn after the push is clipped
  to the shape and content after the `Pop` isn't; blur nested in opacity and the
  reverse; blur under a scale and translate; the same stripe check on a real surface
  through the S2.4d_3 present readback (the swap-chain bitmap as the source); the
  scratch bitmap survives a simulated device loss with no leftover references;
  `check.sh` passes.
* **Result:** new `blur.go`. `PushBackdropBlur` as specified, with σ scaled by √|det| of
  the transform (so the blur radius is in user space). A crop effect (`CLSID_D2D1Crop`,
  added to `d2d` with its two properties and checked live) limits the scratch bitmap to
  the copied region, so stale pixels from a larger earlier copy never bleed in. The
  scratch bitmap is `surface.backdrop`, handed to the canvas with the target bitmap and
  its pixel size (`blurTarget`); it grows (the old one is retired to the frame's
  resources), is recreated for another DPI and is released by `releaseChain`, so also on
  device loss. The effects are made once per frame and changed only right after the
  `Flush`. Every D2D push is now a `canvasPush` that records how to push it again.
  Tests (`blur_test.go`, DPI 96 and 192): stripes blurred inside a rounded shape and
  exact outside and in its corners; content after the push clipped, after `Pop` not;
  hard borders at the target's corner and the copy's edges; zero/NaN radius and empty
  shapes; blur in opacity (also after the group's own content) and opacity in blur; blur
  under translate + scale and a second weaker blur; scratch reuse, growth mid-frame, DPI
  change, one reference between frames; stale content kept out at the target's edge; a
  real surface through the present readback, reused over frames, released and recreated
  by an injected device loss with no leaks. Mutations (each fails at least one test): no
  `Flush`, no layer reopen, σ = radius, σ unscaled, soft borders, no crop, no 3σ pad,
  drawn under the transform, no clip, rect clip for rounded radii, no scratch reuse, no
  release on device loss, a shifted origin. `go.mod` unchanged.
* **Finding:** inside an open D2D layer (`PushOpacity`, a rounded or rotated clip),
  `CopyFromBitmap` from the target reads the layer's own intermediate, not the composed
  target (after a `Flush` too). So the blur pops the open layers (and the clips above
  the lowest one), flushes, copies and pushes them again under their own transforms. An
  opacity group's content drawn before a blur inside it is thereby composed as a group of
  its own (the blur sees it at the group's opacity); axis-aligned clips need no reopen.
* Not run (no machine): `native.sh` on windows/arm64 (no new float-path calls).

**S2.4e_4 Shell images** (`internal/shell/win32`, `image.go`) ✅ *2026-10-20*
* An unexported image type implementing `render.Image`: premultiplied BGRA pixels on the
  CPU, a pixel size and a scale (`Size` in points), with a constructor for the shell and
  the tests (no public API; decoding and icons come later). The D2D bitmap is created on
  first draw for the current device generation and released on device loss and with
  the shell, with no finalizers. `DrawImage` draws it with `DrawBitmap` scaled to `dst`
  (linear interpolation) under the current transform, clip and opacity. Images of
  another shell, other `render.Image`s, nil and an empty `dst` are ignored.
* Done when: offscreen pixel tests at DPI 96 and 192: a 2×2 checker scaled to `dst`
  (each quadrant's colour at its centre), a scale-2 image keeping its size in points, an
  image under a clip and inside `PushOpacity`, foreign images ignored with no `EndDraw`
  failure; a device loss recreates the bitmap on the next draw; closing the last
  surface leaves no live references; the stub comments in `canvas.go` are gone;
  ARCHITECTURE.md (effects, images, the mask context), the PLAN.md status and S2.4e's
  ✅; `check.sh` passes.
* **Result:** new `image.go`. `shellImage` (pixels copied at creation, `Size` = pixels /
  scale) belongs to an `imageSet` (`Shell.images`, the canvas's `images`); `newImage`
  rejects empty or oversized sizes, a bad scale and a wrong pixel count. The bitmap
  (`CreateBitmap`, DPI 96 × scale, so its DIP size is `Size`) is made on first draw for
  the current `gpu` and kept across frames; the set tracks the images holding one, and
  `releaseGPU` releases them before the device objects (device loss and the last
  `Close`), so the next draw makes a new one. A failed `CreateBitmap` is a frame failure,
  not retried on the same devices. `DrawImage` is `DrawBitmap` into `dst` (linear,
  opacity 1); foreign sets, other `render.Image`s, nil (typed too), empty or invalid
  `dst` and a canvas without a set draw nothing. The stub comments are gone from
  `canvas.go`; `TestCanvasBalance`'s nil image stays as a misuse. Tests (`image_test.go`,
  DPI 96 and 192): validation and `Size`; a 2×2 checker scaled to `dst` (quadrant colours
  near the edges and at the centres, a linear mix between, nothing outside), under
  translate + scale, a clip, `PushOpacity` and a non-uniform scale, one bitmap for five
  frames; a scale-2 image keeping 4×4 pt, its bitmap at DPI 192, one-pixel columns sharp
  at DPI 192 and averaged at 96; every ignored case with no bitmap and no failure; the
  recorder clean for a valid draw and reporting a nil image; on a real surface the
  bitmap kept over frames at one reference, released by an injected device loss and
  recreated, released with the last surface with no leaks, made again for a new surface.
  Mutations (each fails at least one test): nearest-neighbour interpolation, no `dst`,
  the scale ignored, foreign images drawn, no release in `releaseGPU`, a wrong opacity, no
  bitmap cache. `go.mod` unchanged.
* **Finding:** a 2×2 bitmap scaled up has its texel centres at the quadrant centres, so
  with linear filtering a device pixel there (off by half a pixel) already mixes in its
  neighbours by a few percent; the exact colours are checked between `dst`'s edge and
  the texel centres, where the edge texel is clamped.
* Not run (no machine): `native.sh` on windows/arm64 (no new float-path calls;
  `DrawBitmap`'s opacity is S2.4e_1's).

**S2.4f Paced frame clock and live resize** (`internal/shell/win32`) ✅ *2026-10-21*
* The clock of the design decisions replaces `interim_clock.go`: frames run when the swap
  chain's waitable signals, `Now` stays on the monotonic timeline, idle means no waits
  armed. `WM_SIZE` and `WM_DPICHANGED` resize and paint synchronously, also inside
  `WM_ENTERSIZEMOVE`…`WM_EXITSIZEMOVE`; DComp commits after each present.
* Done when: native tests show frames paced (none within half a refresh period on the
  host's monitor), an idle shell has no armed wait (counted), and N resizes in a row
  present N frames at the new sizes; the stress run keeps skipping tests that wait on real
  frames; `check.sh` and `native.sh` on windows/amd64 pass; and **manually on Windows**
  (the user, who has two monitors with different scaling and can set a scale other than
  100 %): live resize of `examples/hello` and `examples/controls` from every edge doesn't
  flicker or stretch at 100 % and one other scale, and moving between monitors repaints
  sharply. No Windows 10 or windows/arm64 machine, so the D7 fallback and the arm64 float
  path are "not run (no machine)". S2.4 is ✅ when S2.4a–S2.4f are.
* **Result:** new `clock.go` replaces `interim_clock.go`. `Now` is QPC since `New` (DWM's
  timeline). A frame targets the first vblank after its start (whole periods after the
  previous target) and holds the next one until that target, at least half a refresh
  period after its start. `Run`'s loop now calls `wait` before each `PeekMessageW`:
  during the hold it waits on a high-resolution waitable timer, after it on the
  frame-latency waitables (latency 1) of the visible surfaces (only the invalid ones if
  any), all through `MsgWaitForMultipleObjectsEx(QS_ALLINPUT)`, so input keeps flowing;
  with no visible surface the hold alone paces. A signal taken is the surface's ticket for
  its next `Present`; a `Present` without one (resize paints, a `WM_PAINT` from Windows)
  is a debt the next signals pay first. A waitable silent for 100 ms from the start of
  the wait (a deadline, not reset by messages) counts as signaled. In the modal size/move
  and menu loops a request posts `msgFrame` after the hold, else uses the listener's
  `WM_TIMER`; leaving the loop disarms both. `sync` now paints and presents on every size
  or scale change of a visible surface, not only while sizing, so `WM_SIZE`,
  `WM_DPICHANGED` and `SetSize` return with the new frame on screen; DComp commits after
  every `Present` (already so since S2.4c_2). Deviation from the design decision: with no visible
  surface the hold runs on DWM's vblank grid on a timer instead of `DwmFlush`, which
  would block the thread and stall input. `w32` gained `MsgWaitForMultipleObjectsEx`,
  the waitable-timer calls, `QueryPerformanceCounter`/`Frequency`,
  `DwmGetCompositionTimingInfo` (read from the packed struct by offset) and the menu-loop
  messages. Tests (`clock_test.go`, `TestFrameClockCalls` in `w32`): the grid and
  frame-time tables; 20 hold-paced frames without a surface and 40 waitable-paced frames
  on a shown one (on the host 60.00 Hz: every target step one period, 39 of 39; no start
  within half a period; one `Present` per frame, no debt); idle afterwards with no frame,
  no armed wait and no timer for 150 ms; ten frames in a modal size loop and the hand-over
  back; a stuck waitable giving its frame at the deadline under a post every 10 ms;
  debts and tickets against the real waitable, its recreation on device loss and close;
  12 drag steps inside `WM_ENTERSIZEMOVE`, 12 `SetSize`s and a `WM_DPICHANGED` each
  presenting exactly one frame at the new pixel size before returning. 21 mutations, each
  fails at least one test (no hold, no half-period floor, targets not stepped, ticket or
  waitables ignored, no debt, debts not paid by signals, idle waits counted, no modal
  scheduling either way, modal path left armed, no modal hold, no DWM timing, no stall
  ticket, no stall timeout, a per-wait stall timeout, painting only while sizing, no synchronous
  `fitChain`, presents not counted, no waitable or not closed); two survivors were
  redundant code and removed. The new clock and live-resize tests join the stress run's
  skip list (real frames, shown windows). `check.sh` (stress run included) and
  `native.sh` on windows/amd64 pass. `go.mod` unchanged.
* **Finding:** the frame-latency waitable is a counting semaphore: it starts at 1, gains
  one signal per `Present` with no cap at the latency (5 presents without waiting gave
  5 signals), also for hidden and minimized windows, and `ResizeBuffers` neither resets
  nor signals it. Hence the ticket/debt bookkeeping instead of "wait once per frame".
* Not run (no machine): `native.sh` on windows/arm64 (no new float-path calls; the new
  `w32` calls take integers and pointers), and Windows 10 (D7: the standard-timer
  fallback for the hold). **For the user, on Windows:** live resize of `examples/hello`
  and `examples/controls` from every edge and corner at 100 % and one other scale (no
  flicker, no stretched or lagging content, no black or stale strips), moving a window
  between the two monitors with different scaling (sharp after the move, also mid-drag

## E. Phase 2: S2.5a–S2.5g, DirectWrite text (from PLAN.md §11.1)

**S2.5 DirectWrite text** ✅ *2026-10-22* (the step text as planned):
The `TextSystem` on DirectWrite. Fonts come from
`assets/fonts/` (it always holds a `README.md`, so `go:embed` works when empty); the user
adds the Inter TTFs and OFL license, because Claude reaches only the Go module proxy.
Without them it falls back to Segoe UI. Done when: text is crisp at 100/150/200 % and
across monitors. **Phase 2 exit.**
* Split on 2026-10-21 into S2.5a–S2.5g (a new binding package, a full `TextLayout` with
  UTF-8↔UTF-16 index mapping, real drawing, and embedded fonts). ✅ when all seven are.
  Everything is `//go:build windows`: macOS and Linux only cross-compile (as `check.sh`
  already does), nothing has to run there. New float-path calls (`CreateTextFormat`'s
  size, `CreateTextLayout`'s max width/height, `HitTestPoint`, `DrawTextLayout`'s
  `D2D1_POINT_2F` by value) keep windows/arm64 "not run (no machine)".
* **Design decisions (proposed, to confirm before S2.5a):**
  * *Fonts from memory without a Go COM object.* `IDWriteFactory5::CreateInMemoryFontFileLoader`,
    `IDWriteFontSetBuilder1::AddFontFile` and `CreateFontCollectionFromFontSet` (all on
    Windows 10 1703+, inside D7) load the embedded TTFs. A custom
    `IDWriteFontFileLoader` would be a COM object implemented in Go, with callbacks per
    method; this avoids it, so the hidden-callback guard stays untouched.
  * *Layouts in DIPs, the scale stays in `SetDpi`.* Layouts are made in logical points
    with `DWRITE_MEASURING_MODE_NATURAL`, so measurement is the same at every scale and
    on every monitor; crispness comes from the device context's DPI. Text antialiasing is
    `D2D1_TEXT_ANTIALIAS_MODE_GRAYSCALE` (§3 table: no ClearType fringes, as on macOS).
  * *Font resolution.* Empty family: Inter from the embedded collection, else Segoe UI
    Variable (Windows 11) or Segoe UI (Windows 10); `Monospaced`: Consolas. A named
    family is looked up in the embedded collection, then the system one, then falls back
    as for empty. DirectWrite's system fallback covers missing glyphs (emoji, CJK).
    `IDWriteTextFormat`s are cached per resolved font.
  * *No SF-like tracking table and no line-height tuning in S2.5.* Both are theme values
    measured against macOS, so they go to S3.1/S3.1t (was S3.1b); S2.5 uses the font's own metrics.


**S2.5a DirectWrite bindings I: factory, formats, layout metrics** (`win32/dwrite`) ✅ *2026-10-21*
* `DWriteCreateFactory` (shared, `IDWriteFactory` then `QueryInterface` to
  `IDWriteFactory5`; a missing 5 is an error naming Windows 10 1703), `GetSystemFontCollection`,
  `CreateTextFormat` (float path: the size), `IDWriteTextFormat` setters for word wrapping,
  alignment and paragraph alignment, `CreateTextLayout` (float path: max width and
  height), `IDWriteTextLayout::GetMetrics` and `GetLineMetrics` (structs by pointer).
  Structs `DWRITE_TEXT_METRICS`, `DWRITE_LINE_METRICS`; the weight/style/stretch enums.
* Done when: slot tables in header order checked with `com.CheckSlots`, layout tests on
  amd64/arm64, and a native test measuring "Hello" in Segoe UI at 13 pt (positive width,
  one line, height near 1.33 × size) and a two-line text with `\n`; `check.sh` passes.

* **Result:** new package `internal/shell/win32/dwrite` (`doc.go`, `dwrite.go`,
  `types.go`): `CreateFactory` (shared `IDWriteFactory`, then `IDWriteFactory5`; a
  missing 5 wraps `ErrNoFactory5`, which names Windows 10 1703), `SystemFontCollection`
  with `FamilyCount`, `CreateTextFormat` (float path: the size between integer
  arguments), `TextFormat` setters and getters for word wrapping, text and paragraph
  alignment, plus `FontWeight`/`FontStyle`/`FontStretch`/`FontFamilyName` and `FontSize`
  (a FLOAT result), `CreateTextLayout` (float path: max width and height; a terminated
  copy, so empty text is valid), `TextLayout` embedding `TextFormat` with `MaxWidth`/
  `MaxHeight` (FLOAT results), `Metrics` and `LineMetrics` (the count from an
  `E_NOT_SUFFICIENT_BUFFER` first call; that code joins `com`'s named HRESULTs). Structs
  `TextMetrics` (36 bytes), `LineMetrics` (24); the weight, style, stretch, wrapping
  and alignment enums. d2d's `floatMethod` (`bind.go`) moved to `com.Method`
  (`method.go`) so both packages share it. Tests: full vtables of `IDWriteFactory`–
  `IDWriteFactory5` (24–48 entries), `IDWriteFontCollection` (7), `IDWriteTextFormat`
  (28), `IDWriteTextLayout` (67) with every bound slot checked, layouts, procs, and live
  tests (shared factory identity and counts, every IID, a format read back, round
  trips and `E_INVALIDARG` refusals, layouts of "Hello", "Hello\nWorld", empty text,
  double size, bold, wrapping, no-wrap and centering, every count at 0). Measured:
  "Hello" in Segoe UI at 13 DIPs is 29.94 × 17.29 (1.33 × size), one line, baseline
  14.03; "Hello\nWorld" is two lines at twice the height. Mutation-checked: wrong slots
  (`GetMetrics`, `GetLineMetrics`, `GetMaxWidth`, `CreateTextLayout`, `GetFontSize`,
  `GetFontWeight`) and a fixed size or max width all fail; the survivors were
  equivalent (a larger retry buffer, `checkForUpdates` not observable). `check.sh`
  (stress run included) passes; `go.mod` unchanged. Not run (no machine): windows/arm64
  (new float-path calls), Windows 10.

**S2.5b DirectWrite bindings II: hit-testing and clusters** (`win32/dwrite`) ✅ *2026-10-21*
* `IDWriteTextLayout::HitTestPoint` (float path), `HitTestTextPosition`,
  `HitTestTextRange` (array out, with the "buffer too small" retry), `GetClusterMetrics`
  (same retry), `SetMaxWidth` (float path). Structs `DWRITE_HIT_TEST_METRICS`,
  `DWRITE_CLUSTER_METRICS` (bit fields read by mask, with tests).
* Done when: slot and layout tests; native tests on a fixed Segoe UI layout: a point in
  each half of a glyph gives the expected position and trailing flag, a range over a line
  break gives two rects, "é" (a combining mark) is one cluster of length 2, and an
  emoji surrogate pair is one cluster; `check.sh` passes.

* **Result:** `TextLayout` gains `SetMaxWidth` (slot 28, float path), `ClusterMetrics`
  (62), `HitTestPoint` (64, float path; `PointHit` with `Trailing`, `Inside` and the
  cluster's box), `HitTestTextPosition` (65; its FLOATs come back through pointers, so
  the integer path) and `HitTestTextRange` (66, float path: the origin). Both arrays and
  `LineMetrics` share one generic `fetchArray` with the "buffer too small" retry.
  Structs `HitTestMetrics` (36 bytes) and `ClusterMetrics` (8; the five UINT16 bit
  fields as `Flags`, read by masks LSB first through `CanWrapLineAfter`, `IsWhitespace`,
  `IsNewline`, `IsSoftHyphen`, `IsRightToLeft`). Tests (`hittest_test.go`): slots,
  layouts, every mask bit; live on Segoe UI 13 DIPs: both halves of each glyph of
  "Hello" give its position with trailing false/true and its box, leading and trailing
  caret edges at the cluster widths, points left/right/above/below give the nearest
  position with `Inside` false, a range in a line one rect (the origin offset applied),
  a range over `\n` two rects (3+3 on line 1, 6+2 at the second line's top), "e" + U+0301
  one cluster of 2, a precomposed "é" 1, U+1F600 one cluster of 2, the whitespace,
  newline, soft-hyphen and RTL flags, `SetMaxWidth` wrapping and unwrapping (negative and
  NaN refused), empty text (no clusters, position 0, one empty range rect). Measured
  DirectWrite facts S2.5e relies on: `\r\n` is **two** clusters (only the `\n` is a
  newline; the line's `NewlineLength` is 2), and the text's last cluster always has
  `CanWrapLineAfter`. Mutation-checked: each new slot, the point's y, both origin
  coordinates, the max width value, the trailing argument, a swapped trailing/inside
  and wrong flag bits all fail (13 of 13). `check.sh` (stress run included) passes;
  `go.mod` unchanged. Not run (no machine): windows/arm64 (new float-path calls),
  Windows 10.

**S2.5c In-memory fonts** (`win32/dwrite`) ✅ *2026-10-22*
* `CreateInMemoryFontFileLoader`, `RegisterFontFileLoader`/`UnregisterFontFileLoader`,
  `IDWriteInMemoryFontFileLoader::CreateInMemoryFontFileReference` (the loader copies the
  bytes, so no Go memory is held natively, §7.2), `CreateFontSetBuilder` (as
  `IDWriteFontSetBuilder1`), `AddFontFile`, `CreateFontSet`,
  `CreateFontCollectionFromFontSet`, and family lookup (`FindFamilyName`).
* Done when: a native test reads a system TTF (e.g. `arial.ttf`) into memory, builds a
  collection from it, finds its family, lays out text in it with the same width as through
  the system collection, and releases everything (reference counts back to the start, the
  loader unregistered); a corrupt byte slice is an error, not a crash; `check.sh` passes.

* **Result:** `fonts.go` in `dwrite`: `Factory.CreateInMemoryFontFileLoader` (slot 44),
  `RegisterFontFileLoader`/`UnregisterFontFileLoader` (13/14), `CreateFontSetBuilder`
  (43, the `IDWriteFontSetBuilder1` overload of `IDWriteFactory5`),
  `CreateFontCollectionFromFontSet` (37); `InMemoryFontFileLoader.CreateFontFileReference`
  (no owner object, so DirectWrite copies the bytes; empty data refused) and `FileCount`;
  `FontFile.Analyze`; `FontSetBuilder.AddFontFile`/`CreateFontSet`; `FontSet.FontCount`;
  `FontCollection.FindFamilyName`. A helper for S2.5g, `CreateMemoryFonts(files)`, builds
  one collection from several files and returns `*MemoryFonts` (collection, loader,
  factory reference) whose `Release` gives everything back; any unusable file or no
  font is an error with everything released. `DWRITE_E_FILEFORMAT` joins `com`'s named
  HRESULTs. All integer-path calls. Tests (`fonts_test.go`): full vtables of the loader
  (6), font file (6), `IDWriteFontSetBuilder1` (8), font set (13) and the factory's new
  slots; live: `arial.ttf` read into memory and the buffer zeroed after the reference
  is made (the copy works), one TrueType face, one font, one family, `FindFamilyName`
  case-insensitive and false for Segoe UI and the empty name, "Hello, World 0123" exactly
  as wide as through the system collection (103.09 DIPs at 13) and not as Segoe UI;
  double register and unregister refused; three files in two families; a Consolas
  format from it monospaced; garbage, one byte, a truncated Arial, an empty file and a
  good file followed by garbage all fail (`DWRITE_E_FILEFORMAT` from `AddFontFile`)
  with the factory count back where it was; `Analyze` of garbage says unsupported
  without an error. Measured DirectWrite facts: the font set holds no reference to the
  file; **a layout after `UnregisterFontFileLoader` kills the process** (no panic, exit
  127), so the loader stays registered until every format and layout from the
  collection is released (documented on both methods; S2.5g's collection lives for the
  shell's lifetime); the first in-memory collection laid out leaves three factory
  references held by DirectWrite's caches, so the count tests warm up first.
  Mutation-checked: 8 of 8 (`FindFamilyName`, `AddFontFile`, `Analyze`, `GetFontCount`,
  `GetFileCount`, `CreateFontSetBuilder` slots, the data length, the found flag) fail.
  `check.sh` (stress run included) passes; `go.mod` unchanged. Not run (no machine):
  windows/arm64, Windows 10.

**S2.5d The DirectWrite TextSystem: layout and metrics** (`internal/shell/win32`, `text.go`) ✅ *2026-10-22*
* `text.go` replaces `interim_text.go` (deleted): font resolution and the format cache
  (design decisions above), UTF-8→UTF-16 conversion with an index map both ways (invalid
  bytes become U+FFFD, one unit per byte), `NewLayout` with the contract's wrap rules
  (zero, negative, NaN or +Inf width means `DWRITE_WORD_WRAPPING_NO_WRAP`), `Text`, `Font`,
  `Size` (widest line without trailing whitespace by the sum of line heights; empty text
  one line high), `Baseline`, `LineCount`. `HitTest`, `CaretRect` and `SelectionRects`
  are S2.5e's; until then they answer as for empty text.
* Done when: unit tests of the index map (ASCII, 2-, 3- and 4-byte runes, invalid bytes,
  indices inside a rune); native tests for the size and wrap contract (wrapping at a
  space, a word longer than the width breaking between clusters, `\n` lines, no-wrap
  values, empty text), weight and italic changing the width, monospaced runes all equally
  wide, and an unknown family falling back; the aqua title bar and placeholders still lay
  out (their tests pass); `check.sh` passes.

* **Result:** `text.go` (`textSystem`, `layout`) and `textindex.go` (`indexMap`,
  `utf16Of`) in `internal/shell/win32`; `interim_text.go` is gone. The `Shell` owns a
  `*textSystem` (made in `newShell`, released in `shutdown`); the DirectWrite factory and
  system collection are created with the first layout. **Design choice (new):** a layout
  is measured once in `NewLayout` and kept as Go data (size, first baseline and line
  height, line count, the index map); its native `IDWriteTextLayout` is released at once,
  because `TextLayout` has no `Release` and nothing may rely on finalizers (§7.2). S2.5e
  and S2.5f rebuild a native layout from the stored text, font and width when they need
  one. Fonts: `resolve` gives Inter (embedded collection, S2.5g) → Segoe UI Variable Text
  → Segoe UI for the empty family, Consolas (then Courier New) for `Monospaced`, a named
  family from the embedded, then the system collection, else the UI font; `Family` wins
  over `Monospaced`; sizes clamp to 0.1…2048 and weights to 1…999. Formats are cached per
  key (at most 128, then emptied), and the wrap mode is set on each layout, never on the
  shared format. If DirectWrite fails, or after `release`, layouts come from `shell/fake`
  (logged once). Tests (`text_test.go`, `textindex_test.go`): the index map (ASCII
  identity, 1–4-byte runes, offsets inside runes and surrogate pairs, invalid bytes
  including an encoded surrogate, clamping, round trips); "Hello" at 13 pt (29.9 × 17.3)
  and 26 pt, zero size = 13, empty text (0 × one line, same baseline), `\n`, `\r\n`,
  blank lines, wrapping at a space, hanging trailing spaces, a long word breaking between
  clusters, a tiny width, wrapped `\n` lines, every no-wrap width (0, −1, NaN, ±Inf, 1e300)
  against a tiny positive one, bold wider than regular, Times New Roman italic differing,
  unknown family = UI font, case-insensitive lookup, absurd sizes and weights, monospaced
  runes equally wide, resolve keys, the format cache (shared, bounded, no wrap leaking),
  Unicode (emoji, combining mark, CJK, Hebrew, invalid byte = U+FFFD), a 60 000-character
  text, no growth in the system collection's reference count over 400 layouts, the fake
  fallback after release, the UI-thread check, and the shell's own system inside `Run`.
  Measured DirectWrite facts: Segoe UI Variable is installed as "Segoe UI Variable
  Text/Display/Small", not "Segoe UI Variable"; Segoe UI Variable has no italic face with
  different advances (so italic is checked on Times New Roman); `WORD_WRAPPING_WRAP` keeps
  trailing spaces out of `Width`; the shared system collection and factory keep references
  in DirectWrite's caches (the count settles after the first layouts), so `release` does
  not check them. The aqua title bar and placeholder tests still pass. `check.sh` (stress
  run included) passes; `go.mod` unchanged. Not run (no machine): windows/arm64,
  Windows 10.

**S2.5e The DirectWrite TextSystem: hit-testing, carets and selection** (`internal/shell/win32`, `text.go`) ✅ *2026-10-22*
* `HitTest`, `CaretRect`, `SelectionRects` per the `render.TextLayout` contract on top of
  S2.5b: indices are normalized to the cluster start (from `GetClusterMetrics`, so
  combining marks and emoji count as one); right of a line's last cluster gives the line
  end with trailing false (DirectWrite says "last character, trailing"); the caret at a
  wrap point is drawn at the next line's start; a selected `\n` gets a visible width.
* Done when: native tests for every sentence of the contract (above/below/left/right of
  lines, both halves of a cluster, wrap points, empty text, clamping, an index inside a
  rune or cluster), and a round trip: `HitTest` on each `CaretRect` gives back its index;
  `check.sh` passes.

* **Result:** new `textgeom.go` (`geometry`). **Design choice:** `NewLayout` also reads
  `GetClusterMetrics` and keeps lines (content and break ranges, top, height, the index
  `HitTest` gives right of the line, the end caret x) and clusters (start, left, width,
  direction) as Go data, so the three queries make no native calls and S2.5d's "no
  native object kept" rule holds. Left-to-right text places clusters by summing widths
  (left-aligned); with mixed directions (`MaxBidiReorderingDepth > 1`) every cluster's
  box and each line's end come from `HitTestTextPosition` in `NewLayout`, carets sit at a
  cluster's leading edge in its own direction, and a selection is one rect per line over
  its selected clusters. Indices: UTF-8 → UTF-16, clamped, moved to the cluster start; a
  line break (`\n`, `\r\n`, two clusters in DirectWrite) is one, so an index between `\r`
  and `\n` is the line's end. Right of a line: before its break; wrapped at whitespace,
  the start of the last whitespace cluster (the caret stays on that line, as in
  `shell/fake`); broken between clusters, the next line's start. A selected break is
  half an em wide (`newlineWidth`, 6.5 DIPs at 13). Tests (`textgeom_test.go`; expected x
  values measured from prefix layouts, no hard-coded metrics): both halves of each
  cluster of "Hello" and its carets; above/below/left/right, NaN and ±Inf points;
  `\n` lines with blank and empty last lines (carets, hits, selections across, up to and
  over breaks, reversed, empty, clamped); `\r\n`; wrap at spaces (caret at the wrap point
  on line 2, right of line 1 before the hanging space) and a long word broken between
  clusters; "xéy😀z" (normalize at every byte, both halves of the accent and the emoji
  clusters, selections from inside them); an invalid byte; empty text wrapped and not;
  Hebrew inside Latin; a `HitTest`↔`CaretRect` round trip on seven texts (incl. CJK and
  wrapped). `TestTextEmpty`'s S2.5d placeholder check now tests empty text. Measured
  DirectWrite facts: `HitTestPoint` right of a line says its last cluster with trailing
  true, and right of a `\n` line the `\n` itself; `HitTestTextPosition` at a wrap point
  gives the next line's start; a `\n` cluster has width 0. Mutation-checked: 8 of 8 (RTL
  trailing, the whitespace-wrap end, the break width, the left snap, line lookup, the
  break clamp, the y snap, the bidi line end) fail. `check.sh` (stress run included)
  passes; `go.mod` unchanged. Not run (no machine): windows/arm64, Windows 10.

**S2.5f Drawing text** (`internal/shell/win32`, `canvas.go`, `win32/d2d`) ✅ *2026-10-22*
* `ID2D1RenderTarget::DrawTextLayout` (float path: `D2D1_POINT_2F` by value) and
  `SetTextAntialiasMode` bound in `d2d`. The canvas draws the layout with a solid brush
  at the origin, under the current transform, clip and opacity layers, with grayscale
  antialiasing; semantic colors, `nil` layouts and layouts from another text system are
  ignored. The placeholder box goes.
* Done when: offscreen pixel tests: ink only inside the layout's box, black text on white
  without color fringes (R = G = B on every pixel), the same text at scale 1 and 2 with
  the ink box scaled by 2, the text clipped by a clip and moved by a translation, and
  device loss repainting text; `check.sh` and `native.sh` on windows/amd64 pass.

* **Result:** `d2d` binds `DrawTextLayout` (`ID2D1RenderTarget` slot 28, `D2D1_POINT_2F`
  by value on the float path), `SetTextAntialiasMode`/`TextAntialiasMode` and
  `DrawTextOptions`, slot-checked and pixel-tested offscreen. `canvas.DrawText` draws a
  `*layout` of the surface's text system from a native `IDWriteTextLayout` rebuilt once
  per layout and frame (`layout.native`, cached in `canvas.texts`, released with the
  frame's other resources) in the frame's solid brush, under the transform, clips and
  layers, with grayscale antialiasing (set in `newCanvas`) and color fonts; nil, fake,
  foreign and released layouts, empty text, semantic or transparent colors and
  non-finite origins draw nothing. `interim_text.go` and the placeholder box are gone.
  Tests (`textdraw_test.go`): ink only inside the layout's box, R = G = B on every pixel
  with dark cores, the ink box at scale 2 twice that at 1, brush color and alpha,
  translation, scale, clip and group opacity, wrapped text, every ignored case with no
  resources made, one native layout per layout and frame released to 0, a released
  system, and a real surface repainting the same ink after a device loss. With this step
  the module moved to Go 1.27 (`go 1.27.0`, `toolchain go1.27.3`, staticcheck v0.8.2;
  `ffi.cptr` lost its Go 1.26 `//go:noinline`). `check.sh` (stress run included) and
  `native.sh` on windows/amd64 pass. Not run (no machine): windows/arm64, Windows 10.

**S2.5g Embedded fonts and the phase 2 exit** (`assets/fonts`, `internal/shell/win32`) ✅ *2026-10-22*
* `assets/fonts/` with `README.md` (what to put there, the OFL license, the expected file
  names) and a small Go package embedding the directory (`//go:embed` on a pattern that
  always matches the README). The shell builds its embedded collection once from every
  `.ttf`/`.otf` there (S2.5c) and falls back to Segoe UI when there are none or they fail
  to load (logged once with `GOGUI_DEBUG=1`).
* Done when: tests with no font files (fallback), with a system TTF copied in by the test
  through a test-only hook (embedded path used), and with a corrupt file (fallback);
  ARCHITECTURE.md describes the text system; the PLAN.md status and S2.5's ✅.
  **For the user, on Windows:** add the Inter TTFs and `OFL.txt` to `assets/fonts/`; look at
  `examples/hello` and `examples/controls` at 100, 150 and 200 % (title and placeholder
  text sharp, grayscale, not clipped) and after moving a window between the two monitors.
  **Phase 2 exit.**

* **Result:** new package `assets/fonts` (`fonts.go`, untagged): `//go:embed *` over the
  directory (its `README.md` always matches, so it builds with no fonts), `Files()` returns
  the `.ttf`/`.otf` files (any case) sorted by name, `IsFontFile`. `README.md` lists the
  expected files (`Inter-Regular/Italic/Medium/SemiBold/Bold.ttf` from Inter's
  `extras/ttf`, the static family named exactly `Inter`, not `InterDisplay` or the
  variable font) and `OFL.txt`, the fallback rules, and the no-Apple-assets rule. Only
  `shell/win32` imports it, so macOS and Linux binaries carry no fonts. The text system
  (`text.go`) builds the embedded collection in `init`, once, through
  `dwrite.CreateMemoryFonts` from every file; no files or any failing file means no
  embedded collection and the system UI font, with the reason (the file names and the
  error) logged once by `debugf`, which is `log.Printf` only with `GOGUI_DEBUG=1`.
  `release` frees the `MemoryFonts` after the formats (the loader has to outlive them).
  Test-only hooks: `fontFiles` (the file source, `fonts.Files` by default) and
  `embeddedUI` (the family the empty family resolves to, `Inter`). Tests
  (`textfonts_test.go`): no files (fallback, one log line); Georgia regular and bold
  copied in from `%WINDIR%\Fonts` as the embedded collection (one family, the empty,
  named, case-different and unknown families resolve there, Arial and the monospaced font
  still from the system, the same size and baseline as the system's Georgia, bold wider,
  the drawing rebuild works, nothing logged); garbage, good-then-garbage, truncated and
  empty files (fallback, one log line naming the file); and the real `assets/fonts`
  (whatever it holds must load; Inter is the UI font when present). `newTestText` now
  ignores `assets/fonts`, so the older text tests stay on Segoe UI once Inter is added.
  Every test releases with no references left. `assets/fonts` has its own tests (the
  extension filter, sorting, the README embedded). `check.sh` (stress run included)
  passes; `go.mod` unchanged. Not run (no machine): windows/arm64, Windows 10.
  **Open (user, on Windows):** add the Inter TTFs and `OFL.txt` to `assets/fonts/`, then
  check `examples/hello` and `examples/controls` at 100, 150 and 200 % and across the two
  monitors (see PLAN.md §11.1 "Phase 2 exit"). Done, see below.

**Phase 2 exit** ✅ *2026-10-22*
* **Result:** the user added the five static Inter TTFs (`Inter-Regular`, `-Italic`,
  `-Medium`, `-SemiBold`, `-Bold`) and `OFL.txt` (plus an identical copy as `LICENSE.txt`)
  to `assets/fonts/`, and checked `examples/hello` and `examples/controls` at 100, 150 and
  200 % and after moving a window between the two monitors: the text is sharp, grayscale,
  not clipped, and in Inter. Locally, `TestTextEmbeddedDefault` loads the real files and
  resolves the UI font to Inter, and `check.sh` passes. Phase 2 is done.

---

## F. Phase 3: S3.1 (S3.1a–S3.1d), S3.2 (S3.2a–S3.2f), S3.3 (S3.3a–S3.3h), S3.4 (S3.4a–S3.4g), S3.5 (S3.5a–S3.5h) and S3.6 (S3.6a–S3.6e), from PLAN.md §11.1

**S3.1a Spec, generator, semantic colors** ✅ *2026-10-23* (the step text as planned):
* `internal/aqua/theme`: `Theme` with a `Colors` table (light and dark per
  `driver.SemanticColor`, an optional increased-contrast pair); the spec format (each
  value with `source: "memory" | "measured"`); the generator as a function plus a
  `go run` wrapper behind `//go:generate` (stdlib only, `go/format`), writing
  `tokens.go` and `docs/tokens.md` (every token, provisional ones marked).
* All 39 semantic colors from memory (the 12 provisional values carried over). Nothing
  uses the package yet.
* Done when: tests show the generator reproduces the committed `tokens.go` and
  `docs/tokens.md` byte for byte, and they reject a spec that is missing a color or an
  appearance or has an out-of-range channel or a bad `source`.

* **Result:** new packages `internal/aqua/theme` (imports only `driver`),
  `internal/aqua/theme/gen` and the command `internal/aqua/theme/gen/gentokens` (both
  stdlib only). `theme.go` (hand-written): `Theme{Name, Colors}`, `Colors` an array of
  `ColorToken` indexed by `driver.SemanticColor` (index 0 unused), `ColorToken{Light, Dark,
  HasContrast, ContrastLight, ContrastDark}`, `ColorToken.Value(a)` and
  `Theme.Color(s, a)` (false for `SemanticNone` and unknown colors; no accent rules, those
  move in S3.1b), and `//go:generate go run ./gen/gentokens -spec spec/sequoia.json -out
  tokens.go -doc ../../../docs/tokens.md`. `spec/sequoia.json`: all 39 colors, every one
  `source: "memory"`; the 12 of `provisional.go` unchanged (noted in the spec), the other
  27 from memory (system colors at the usual macOS values; the accent colors get the
  macOS blue values even though the driver resolves them to `Appearance.Accent`; no
  increased-contrast pairs yet, S3.1d). `gen.Parse` validates strictly: unknown fields,
  unknown or missing colors, a missing `light`/`dark`, a lone contrast value, channels
  that are not integers in 0..255 or not 3/4 of them, a missing/empty/bad `source`, a
  non-exported `name`, multi-line notes, trailing data. `gen.Generate` writes the
  gofmt-formatted `tokens.go` (`// provisional` per value from memory) and
  `docs/tokens.md` (one row per color, hex values, the provisional count). Tests: the
  committed files equal a fresh generation byte for byte; `gen.ColorNames` matches the
  constants parsed from `driver/color.go`; every rejection above; `Value`/`Color`; and
  `internal/aqua/theme_carry_test.go` pins the 12 carried values against `provisional`
  (deleted with it in S3.1b). Nothing in the driver uses the theme yet, so no paint dump
  changed. `check.sh` (stress run included) passes; `go.mod` unchanged. Nothing needs
  another OS beyond the cross-builds.

**S3.1b The aqua driver resolves through the theme** ✅ *2026-10-23* (the step text as planned):
* `theme.Resolve(c, appearance)` replaces `resolveColor`: the accent rules
  (`ColorControlAccent`, `ColorSystemBlue`, selection, focus ring at half alpha) and
  magenta for unknown colors move unchanged. The title-bar and traffic-light colors
  (key, inactive, pressed shade) go into the spec. `provisional.go` is deleted.
* `win32/present.go` clears to the theme's `ColorWindowBackground` (the shell may import
  the leaf package `theme`; if that layering is unwanted, the aqua window passes the
  color in instead).
* Done when: the pinned paint dumps and the win32 pixel readbacks are unchanged, or
  re-pinned where a newly added color changes them, with the reason in the Result.

* **Result:** `theme.Theme` gains `Resolve(c, a)` (the accent rules and `Unknown`, opaque
  magenta, moved unchanged from `provisional.go`) and `Chrome`: the `ColorToken`s
  `TitleBar`, `TitleBarInactive`, `TitleBarSeparator` (the light `ColorSeparator` value
  and the dark black hairline that `chrome.go` hard-coded), `Close`/`Minimize`/`Zoom`
  `Fill`/`Border`/`Glyph`, `LightInactiveFill`/`Border`, and the numbers
  `LightPressedShade` (0.82) and `LightDisabledAlpha` (0.5, also from `chrome.go`). The spec
  has a `chrome` section (`colors` in the semantic-color form, `numbers` with a finite
  float32 `value`), all 16 `source: "memory"` with the old values; `tokens.go` and
  `docs/tokens.md` count tokens now (55, all provisional) and list the chrome in its own
  section. `gen` checks the chrome names against the `Chrome` fields parsed from
  `theme.go`, with new rejection cases. In `aqua`, `provisional.go` and
  `theme_carry_test.go` are deleted; `colors.go` draws with `tokens = &theme.Sequoia`
  (`resolveColor` → `Theme.Resolve`, the chrome helpers read `Theme.Chrome`), so no color
  value is written in the package any more. `shell/win32` imports the leaf `aqua/theme`
  (layering accepted: it depends only on `driver`, and the shell still never imports
  `aqua`) and clears to `theme.Sequoia`'s `ColorWindowBackground` (`clearColor`); its
  tests keep the literal greys as an independent pin. No paint dump and no win32 pixel
  readback changed, so nothing was re-pinned. `check.sh` (stress run included) passes on
  the Windows host, including the win32 aqua readbacks; `go.mod` unchanged. Nothing needs
  another OS beyond the cross-builds.

**S3.1c Metrics and typography** ✅ *2026-10-23* (the step text as planned):
* Per control size (regular, small, mini, large): control heights, paddings, corner
  radii, bezel shadows, the focus-ring width and offset; window chrome
  (`TitleBarHeight`, traffic-light size, step and inset, the title gap) moves from
  `chrome.go`/`window.go` into the spec. Typography: font size per control size (in step
  with `driver.Font.EffectiveSize`), weights, the SF-like tracking table and line height
  that S2.5 left open.
* Done when: chrome reads its metrics from the theme with unchanged dumps, a test pins
  `EffectiveSize` against the typography tokens, and the stub layout metrics are
  untouched.

* **Result:** `theme.Theme` gains `Metrics` and `Typography`, and `Chrome` the metrics
  `TitleBarHeight` (28), `LightSize` (12), `LightStep` (20), `LightInset` (8), `TitleGap`
  (8), `LightBorderWidth` (0.5) and `LightGlyphStroke` (1.2), all from `chrome.go` and
  `window.go` unchanged. `Metrics`: `SizeTable`s (`[NumControlSizes]float64` indexed by
  `driver.ControlSize`, `At` maps unknown sizes to regular) for `ControlHeight` (20, 17,
  14, 26: the visible push-button bezel), `ControlPaddingX`, `ControlCornerRadius` and the
  bezel shadow's offset, blur and spread, plus the `BezelShadow` color and
  `FocusRingWidth` (3) / `FocusRingOffset` (0); `Control(cs)` collects one size.
  `Typography`: `FontSize` (13, 11, 9, 13), `LineHeight` (1.23 × the size),
  `WindowTitleWeight` (600) and `EmphasizedWeight` (700), and a 31-row `Tracking` table
  (the HIG's SF Pro values from 6 to 80 pt, in 1/1000 em, applied to Inter), with
  `TrackingAt` (linear, clamped), `TrackingPoints` and `LineHeightFor`. All new values
  are `source: "memory"`; `tokens.go` and `docs/tokens.md` have 76 tokens, all
  provisional (a size row and the tracking table count as one each). The generator now
  works from a schema, `gen.Groups` (per group the `Theme` field, JSON key, heading and
  field names by kind: colors, numbers, size rows, weights, tracking), replacing
  `ChromeColorNames`/`ChromeNumberNames`; the spec has the groups `chrome`, `metrics` and
  `typography`, with new validation (size rows need all four sizes, weights are integers
  in 1..1000, the tracking table only in `typography`, non-empty, by strictly ascending
  positive size) and the listing a section per group. A test checks `Groups` against the
  `Theme` fields and group structs parsed from `theme.go`. In `aqua`, `chrome.go` reads
  every metric and the title font from `tokens` (`lightTop`, `lightsWidth`,
  `titleMargin`, `titleFont` are functions now; the glyph paths are drawn on a 12 pt grid
  scaled to `LightSize`; the border stroke uses `LightBorderWidth`); `TitleFont` is gone
  and `TitleBarHeight` became a function (`TitleBarHeight()`, used by the win32 aqua
  test). Nothing draws with `Metrics` yet (S3.2/S3.3), and the win32 text system does not
  apply tracking or line height yet. `TestFontSizeMatchesEffectiveSize` pins the font
  sizes against `driver.Font.EffectiveSize`; `TestChromeReadsTheme` swaps in a theme
  with other chrome metrics and checks the bar, the lights, their regions, the border
  stroke, the title weight and the title margin follow it. No paint dump, win32 pixel
  readback or example layout changed, and `driver/stub/metrics.go` is untouched.
  `check.sh` (stress run included) passes; `go.mod` unchanged. Nothing needs another OS
  beyond the cross-builds.

**S3.1d Contrast, accent, animation; O1 and O2** ✅ *2026-10-23* (the step text as planned):
* Increased-contrast variants for the colors that have them (used when
  `Appearance.IncreaseContrast`). Accent-derived colors (pressed, inactive window,
  default button) computed from `Appearance.Accent` by documented rules, not stored.
  Animation tokens: durations, cubic-bezier curves, spring parameters (used from S3.2).
* Decide O1 (proposed: Inter, already bundled since the phase 2 exit) and O2 (proposed:
  follow the OS accent, as the Win32 shell already does; an app-level override is public
  API and joins phase 6's OS-accent work, so §4 does not change in S3.1). Update §12 and
  ARCHITECTURE.md; S3.1 ✅.

* **Result:** `theme.Theme` gains `Accent` and `Animation`. **Contrast:** 13 colors get an
  increased-contrast pair, all `source: "memory"`: `ColorLabel`, the secondary, tertiary
  and quaternary labels, `ColorPlaceholderText`, `ColorSeparator`, `ColorGrid`,
  `ColorLink`, `ColorControlText`, `ColorDisabledControlText`, and the chrome/metrics colors
  `TitleBarSeparator`, `LightInactiveBorder` and `BezelShadow` (stronger alpha, or darker /
  lighter values; backgrounds keep theirs). **Accent rules** (`accent.go`, hand-written):
  `FocusRing` (accent alpha × `FocusRingAlpha` 0.5, as before), `Selection` (the accent at
  `SelectionAlpha` 0.3 over the opaque `Base`, white light / 89 grey dark), `AccentPressed`
  (channels × `PressedShade` 0.85), `AccentInactive` (the accent's Rec. 709 luma as a grey
  at `InactiveAlpha` 0.35 over `Base`) and `AccentText` (`DefaultButtonText`, white).
  `Resolve` now sends `ColorSelectedTextBackground` and `ColorSelectedControl` through
  `Selection` and the focus indicator through `FocusRing`, so they follow the OS accent;
  their spec values stay as references, and a test checks the rules give them back from
  `driver.DefaultAccent` within 1.5/255. A default button in an inactive window is drawn
  as a plain push button (S3.3), so it needs no color of its own. **Animation:**
  `DurationFast`/`Normal`/`Slow` (100/250/400 ms), `FocusRingDuration` (200),
  `ScrollerFadeDelay`/`Duration` (1000/300), the Core Animation curves `EaseIn`,
  `EaseOut`, `EaseInOut`, `EaseDefault` (`Curve{X1, Y1, X2, Y2}`), and the springs
  `ControlSpring` (1/300/30, damping ratio 0.87) and `RubberBandSpring` (1/200/28.28,
  critical) with `Spring.DampingRatio`; `Animation.Duration(d, a)` is 0 with Reduce
  Motion. The generator learns the kinds `durations` (integer `ms` in 0..60000, generated
  as `n * time.Millisecond`), `curves` (`value` of four, x1/x2 in 0..1) and `springs`
  (positive `mass`, `stiffness`, `damping`), the groups `accent` and `animation`, and
  rejects each malformed case; `tokens.go` imports `time` now and has 94 tokens, all
  provisional. **O1 → D9** (Inter) and **O2 → D10** (follow the OS accent; the app
  override comes in phase 6); §12 and the decision summary updated, §4 unchanged. No paint
  dump, win32 pixel readback or example layout changed (nothing draws a selection, a
  contrast variant or an animation yet). `check.sh` (stress run included) passes; `go.mod`
  unchanged. Nothing needs another OS beyond the cross-builds.

**S3.2a `anim`: curves and springs (pure math)** ✅ *2026-10-26* (the step text as planned):
* `internal/aqua/anim`, imports only `theme` and the standard library: `Curve.At(t)`
  (cubic-bezier solved for x by Newton with a bisection fallback), a damped spring
  (`theme.Spring` → closed-form position and velocity for under-, critically and
  over-damped, `Settled(eps)`), and `Lerp` for `float64`, `driver.Color` (straight
  alpha, per channel), `driver.Point`/`Size`/`Rect`.
* Done when: tests pass for the curve end points, monotony and known CSS values
  (`ease-in-out` at 0.5), each spring regime against a numeric integration, and settling.

* **Result:** new package `internal/aqua/anim` (`doc.go curve.go spring.go lerp.go`),
  importing `aqua/theme`, `driver` (for `Color`/`Point`/`Size`/`Rect`, which the step text
  needs although it said "only `theme`") and the standard library; no build tags, nothing
  imports it yet. `Curve` and `Spring` are defined as `theme.Curve`/`theme.Spring`, so a
  token converts with `anim.Curve(an.EaseOut)` and the methods live in `anim`. `Curve.At`
  clamps t (NaN → 0, exact 0 and 1), runs Newton from s = t converging on **s** (step
  < 1e-14) and bisects to 1e-15 when the slope is below 1e-3 or a step leaves 0..1;
  `Linear`. `Spring.Motion(from, to, v0)` (panics on a non-positive or infinite value)
  gives `Motion.At(t) (x, v)` from one basis `C(t)`, `S(t)` for all three regimes
  (x = x0·C + (v0 + a·x0)·S, v = v0·C − (ω₀²x0 + a·v0)·S); the overdamped pair is written
  with e^(−(a−ω)t) and `Expm1`, so it neither cancels near ζ = 1 nor overflows. `Settled(t,
  eps)` bounds the reachable displacement and speed by the spring's energy, so it is
  monotonic in t; `SettleTime(eps)` (not in the step text, for S3.2b's "stop requesting
  frames") finds the first settled microsecond, capped at `MaxSettleTime` (1 h).
  `Lerp` is (1−t)·a + t·b (exact end points); `LerpColor` clamps each channel and panics
  on semantic colors; `LerpSize`/`LerpRect` clamp negative sizes. 15 tests and an example,
  100 % coverage, ~0.1 s. **Findings:** (1) a tolerance on x is the wrong stop for the
  curve solver: with a flat end (`{0, 1, 0, 1}`) x = 1e-12 passed at s = t while the true
  s is 1e-4 (y off by 3e-4); converging on s fixes it. Where x is flat in the middle
  (`{1, 0, 0, 1}`) float64 itself limits y to a few 1e-6, so that curve's test tolerance
  is 1e-5 (the theme's curves meet 1e-9). (2) `RubberBandSpring` (1/200/28.28) is
  ζ = 0.99985, so strictly underdamped, not critical as S3.1d's Result says; its
  overshoot (e^−181) is invisible, so the token stays. `check.sh` (stress run included)
  passes; `go.mod` unchanged. Nothing needs another OS beyond the cross-builds.

**S3.2b `anim`: animations on the frame clock** ✅ *2026-10-26* (the step text as planned):
* `anim.Value` (a float animated by a duration + curve, or by a spring; `Set` jumps,
  `AnimateTo` retargets from the current value and, for springs, the current velocity),
  and `anim.Timeline` (owns the running values, `Tick(frameTime)` advances them and
  calls each value's change func, requests the next frame only while something runs).
* The aqua `Driver` installs one frame func on `Shell.Clock()` at `Ready` and owns one
  `Timeline`; elements reach it through their window (`Window.Timeline`). Reduce Motion:
  every duration is 0 (`Animation.Duration`) and springs jump, so nothing runs.
* Done when: fake-clock tests show exact values per frame, whole-frame stepping with a
  late frame, retargeting mid-flight, an idle app requesting no frames, and Reduce
  Motion finishing at once.

* **Result:** `anim/timeline.go`. `NewTimeline(c anim.Clock)` takes a two-method
  interface (`Now`, `RequestFrame`) that `shell.FrameClock` satisfies, so `anim` still
  imports only `theme`, `driver` and the standard library. The two kinds of animation
  are two methods: `AnimateTo(x, d, curve)` and `SpringTo(x, spring)` (the step text
  named only `AnimateTo`), plus `Set`, `Stop`, `Value`, `Velocity`, `Target`, `Running`;
  `NewValue(x, onChange)` on the timeline. Values are sampled in closed form at the frame
  time, so a late frame is exact; an animation starts at the clock's `Now` (the frame
  time inside a `Tick`), and a retarget samples the running animation at that moment
  (not the last frame's value), handing its velocity to a spring, also from a tween
  (new `Curve.Slope(t)`, dy/dx). Animating to the running target, or to the resting
  value, is a no-op, so controls may call it on every event. A spring ends once
  `Motion.Settled` holds within `SettleEpsilon` (1e-3) at a frame; S3.2a's `SettleTime`
  is not needed for that and stays unused. Reduce Motion lives on the timeline
  (`SetReduceMotion`): `AnimateTo` and `SpringTo` jump, and turning it on finishes the
  running values; the driver follows `Appearance.ReduceMotion` at `Ready` and on each
  change (callers still pass `Animation.Duration`, which is 0 then). Change funcs run in
  start order and may start, retarget or stop any value; stopped values are unlisted
  after the loop. The `Driver` creates the timeline at `Ready` (`Driver.Timeline`, nil
  before) and installs `Tick` as the frame function; windows from `Driver.NewWindow`
  share it (`Window.Timeline`; nil for `aqua.NewWindow` used directly, as in the window
  tests). 10 new `anim` tests (100 % coverage) and `TestDriverTimeline`. **Finding:** an
  animation whose change func invalidates its element ends with one empty frame: both
  shells' `Surface.Invalidate` requests a frame even inside the frame function, whose
  paint pass already paints it (win32's `TestClockPaced` relies on that request to keep
  animating). One idle frame per finished animation is harmless, so the shell contract
  stays; S6.2 (idle CPU) can drop the request inside the frame function if it matters.
  No paint dump, a11y dump or example layout changed. `check.sh` (stress run included)
  passes; `go.mod` unchanged. Nothing needs another OS beyond the cross-builds.

**S3.2c Window state plumbing (key / inactive, appearance)** ✅ *2026-10-26* (the step text as planned):
* An element can ask for its window context from its root: `Base.Window()` (nil outside
  a window) with `IsKey`, `Appearance`, `Theme`, `Timeline` and `Router`.
* `KeyChanged` and an appearance change invalidate the whole tree and send a new
  `WindowStateEvent{Key, Appearance}` to every element (pre-order, also hidden ones, no
  bubbling), so controls can restart or drop animations. The focus is kept while the
  window is inactive; the router reports it as "focused but inactive".
* Done when: tests cover key → inactive → key, appearance changes, elements added to an
  inactive window seeing the right state, and the a11y snapshot.

* **Result:** new `aqua/state.go`. `Base.Window()` walks to the root and returns its
  `Window` when the root is a window's (nil when detached or under a plain `NewRouter`
  root; a closed window keeps its last state); `Window.Theme()` is new (the driver's
  `tokens`), `IsKey`, `Appearance`, `Timeline` and `Router` were there. `WindowStateEvent`
  joins the closed `Event` set; `Window.stateChanged` invalidates every element and sends
  it in pre-order (hidden and disabled included, from a snapshot of the tree: elements a
  handler moved out of the window are skipped, a handler closing the window stops the
  rest; each event carries the state at delivery). `KeyChanged` now does that for the
  whole tree instead of repainting only the title bar, and calls
  `WindowHandler.KeyChanged` after the elements (not if a handler closed the window);
  `SetAppearance` does it instead of only invalidating. "Focused but inactive": the
  `Router` gets `SetActive`/`Active` (a plain router starts active; a window's router
  starts inactive, follows `IsKey` and turns inactive on `Closed`) and
  `FocusState(e)`/`Base.FocusState()` returning `Unfocused`, `FocusedInactive` or
  `FocusedActive`; the focus itself, the hover and the routing do not change. **a11y
  decision** (not in the step text): the window node gets `StateFocused` while the window
  is key, and the focused element keeps `StateFocused` while it is inactive (as AX's
  focused element per window; the phase-7 bridge combines both), so no new a11y state.
  That changed two a11y dumps (`TestWindowContent` and
  `TestExampleHelloOnAquaDump`: `window "…" [focused]`), and `TestDriverWindowWithContainers`'
  probe now logs the state event of `Show` making the window key. Elements that join a
  window later get no event; they read `Base.Window`. 4 new tests in `state_test.go`
  (100 % coverage kept); no paint dump or example layout changed. `check.sh` (stress run
  included) passes; `go.mod` unchanged. Nothing needs another OS beyond the cross-builds.
  **Finding:** one stress run failed once in `win32`'s `TestSurfaceMouse` (an extra
  `WM_MOUSEMOVE` between the two releases while captured; `win32` does not import `aqua`);
  20×4 isolated reruns and the next full run passed. Likely an OS-synthesized move; if
  it recurs, the test should drop moves at an unchanged position while captured.

**S3.2d Focus ring** ✅ *2026-10-26* (the step text as planned):
* `aqua.FocusRing`: an embeddable helper that animates in on `FocusEvent{true}` (the
  `FocusRingDuration` token, `EaseOut`, the ring grows from a wider faint outline to its
  final width, as on macOS), disappears at once on blur or when the window becomes
  inactive, and paints around a rounded rect or a path with `FocusRingWidth`/`Offset`
  and `Theme.FocusRing(a)` (`PaintFocusRing(c, shape, progress)`), outside the element's
  bounds (so the ring is drawn in the parent's `Paint` order without being clipped by
  the element itself).
* The focusable placeholders (`TextField`, `TextView`; Button, Checkbox, Radio, Slider and
  PopUpButton become focusable here, see S3.2e) draw it, so it is visible before S3.3.
* Done when: recorder tests check the ring geometry and colour per frame (fake clock),
  light and dark, increased contrast, inactive window, Reduce Motion. Optional manual
  check on Windows: Tab through `examples/form` and see the ring.

* **Result:** new `aqua/focusring.go`. `FocusRing` (zero value ready) is tracked from the
  element's `HandleEvent` (`Track(self, ev)`, reacting to `FocusEvent` and
  `WindowStateEvent` and handling neither) and painted last in its `Paint`
  (`Paint(c, self, shape)`). It follows S3.2c's `FocusState`, not the event: on while
  `FocusedActive`, so it animates in on focus and again whenever the window becomes key
  (not in the step text, which named only `FocusEvent{true}`), and drops at once on blur,
  hiding, removal and an inactive window; an appearance change does not restart it. The
  animation is one `anim.Value` on the window's timeline: 0 → 1 over
  `Duration(FocusRingDuration, a)` with `EaseOut`; without a timeline (`aqua.NewWindow`
  used directly, a plain router) and with Reduce Motion it appears at once. Geometry
  (`FocusRingBand(p)`, provisional like the tokens): the inner edge
  `FocusRingOffset + (1−p)·FocusRingWidth` outside the shape and the width
  `FocusRingWidth·(2−p)`, alpha × p, so the ring starts one width further out, twice as
  wide and invisible, and settles on its 3 pt band. `PaintFocusRing(c, shape, p, a)`
  takes the appearance too (the color needs it): a rounded rect (`RectFocusShape`) is
  outset exactly, rounded corners growing by the outset and sharp ones staying sharp; a
  path (`PathFocusShape`) is scaled about its bounds' centre so they grow by the outset
  (exact for rects and ellipses, close for convex shapes; a flat direction is not
  scaled). **Theme change:** `Theme.FocusRing(a)` is the accent itself with
  `IncreaseContrast` (else × `FocusRingAlpha`), so increased contrast has an effect;
  no token added. The placeholders embed a ring; `Button`, `Checkbox`, `Radio`, `Slider`
  and `PopUpButton` are now focusable (full keyboard access; the macOS-default mode is
  S3.2e), which put `[focusable] {press focus}` on `TestExampleHelloOnAquaDump`'s
  "Greet" button and makes Tab reach them in the examples. A placeholder with an empty
  frame paints nothing, ring included. Tests (`focusring_test.go`, 4 tests, 100 %
  coverage kept; one theme test): the band and color against an independent
  computation in light, dark, increased contrast and another accent; a text field's ring
  frame by frame on the driver's timeline (exact at each frame), blur, inactive
  mid-flight and key again, dark, contrast, Reduce Motion, hiding; no timeline; every
  focusable placeholder. **Finding:** `fake.FrameInterval` is 1/60 s rounded down to
  16 666 666 ns, so 12 frames are 8 ns short of the 200 ms and the ring ends at the
  13th frame (then the one empty frame of S3.2b's finding); tests that count frames for
  a duration must allow for that. No other paint dump or example layout changed.
  `check.sh` (stress run included) passes; `go.mod` unchanged. Nothing needs another OS
  beyond the cross-builds; open: the optional manual check (Tab through `examples/form`
  on Windows and see the ring).

**S3.2e Tab order and keyboard access** ✅ *2026-10-26* (the step text as planned):
* Full keyboard access: aqua behaves as macOS with "Keyboard navigation" on (the
  Windows/Linux convention: Tab reaches every focusable control), and `Router` gets
  `SetFullKeyboardAccess(false)` for the macOS default (Tab only to elements that set
  `SetFocusKind(FocusText)`: text fields, text views, later lists). No public API yet.
* Initial focus: `Window.Show` focuses the first element of `FocusOrder` when nothing is
  focused (`Widget.Focus` before `Show` wins); `SetContent` drops a focus outside the new
  content. Focus survives key changes (S3.2c).
* `scroll-to-focused` is S3.5's.
* Done when: tests cover both keyboard-access modes, Shift-Tab, wrap-around, hidden and
  disabled controls, initial focus, and focus in a replaced content.

* **Result:** `Base.SetFocusKind`/`FocusKind` with `FocusControl` (the zero value) and
  `FocusText`; `Router.SetFullKeyboardAccess`/`FullKeyboardAccess` (on by default). The
  mode and the kind change only the Tab order: `FocusOrder` keeps the focusable, visible
  and enabled elements and, without full keyboard access, only the `FocusText` ones;
  `SetFocus` (a click, `Focus`) still focuses any focusable element, and a focused
  element keeps the focus when the mode or its kind changes. **Not in the step text:**
  `FocusNext` from a focus outside the Tab order (a button focused while only text
  elements are Tab stops) goes to the first Tab stop after it in tree order, Shift-Tab
  to the last one before it (as AppKit's key-view loop continues from the clicked view),
  instead of restarting at the first; and a Tab with no Tab stop at all stays unhandled,
  so it bubbles back to the shell. The placeholder `TextField` (secure too) and `TextView`
  are `FocusText`, the other focusable placeholders `FocusControl`. `Window.Show` focuses
  the first Tab stop when nothing is focused, before the surface shows, so the element
  gets `FocusEvent{true}` and then the `WindowStateEvent` of the key change (the ring
  animates in once); a `Focus` after `SetContent` and before `Show` wins and a second
  `Show` keeps the focus. A `Focus` before `SetContent` still does nothing (the element
  has no router yet). `SetContent` already dropped a focus in the old content through
  the tree validation; now documented and tested, and nothing new is focused until the
  next `Show` or Tab. Effects: `TestExampleHelloOnAquaDump`'s a11y dump has the name
  field `[focusable focused]`, and `TestFocusRingAnimation` clears the initial focus
  before it starts. Tests: `TestKeyboardAccess` (both modes, Tab and Shift-Tab with
  wrap-around, from a control before, inside and after the text elements, hidden and
  disabled text elements, a focus lost to hiding, `SetFocusKind` on the focused element,
  no Tab stop), `TestInitialFocus` (first enabled control, focus across key changes and
  a second `Show`, `Focus` before `Show`, the text-only mode, nothing focusable, a closed
  window, replaced and removed content), `TestPlaceholderFocusKinds`; 100 % coverage
  kept. `check.sh` (stress run included) passes; `go.mod` unchanged. Nothing needs
  another OS beyond the cross-builds; nothing manual.

**S3.2f Return and Escape** ✅ *2026-10-26* (the step text as planned):
* The window keeps one default and one cancel element (`Window.SetDefaultButton`,
  `SetCancelButton`; a `Button` placeholder's `SetDefault`/`SetCancel` register it, and
  a second one replaces the first). An unhandled key-down of Return or keypad Enter
  (no modifiers) presses the default button, Escape (and Primary+. as on macOS) the
  cancel button, only when it is visible and enabled in the tree; the press is the
  element's `Activate()` (`a11y` `press`), which the placeholder answers like
  `UserClick`. A focused element that handles Return (a text view) gets it first.
* Done when: tests cover routing, priority of the focused element, disabled or hidden
  buttons, replacing and removing them, and `form`'s OK/Cancel through the fake shell.

* **Result:** new `aqua/buttons.go`. **Changed from the step text:** the registration is a
  mark on the element (`Base.SetDefaultButton`/`SetCancelButton`, with `IsDefaultButton`/
  `IsCancelButton`) rather than `Window.SetDefaultButton`/`SetCancelButton`, because the
  public layer sets `SetDefault` before the button joins a window (`examples/form`) and
  elements get no event when they join one; the window resolves its buttons at each key
  (`Window.DefaultButton`/`CancelButton`: the element of its tree marked last, from a
  package-wide atomic serial). So a second mark replaces the first, and unmarking or
  removing the second makes the first, still marked, the button again. `Element` gets
  `Activate() bool` (Base: false); the placeholder `Button` forwards `SetDefault`/
  `SetCancel` to the stub and the element and answers `Activate` with `UserClick` (false
  when disabled in the tree); the other placeholders have no primary action yet. The
  surface handler passes a key-down the `Router` did not handle (so the focused element,
  its ancestors and Tab come first) to the window: Return without modifiers presses the
  default button, Escape without modifiers and Primary+. (resolved to Ctrl+. off macOS;
  ⌘. is not Primary there) the cancel button, only when it is in the tree, visible and
  enabled; there is no fall-back to an older mark. **Not in the step text:** keypad
  Enter needs nothing extra, the shells already report it as Return (win32's
  `VK_RETURN` covers both); an auto-repeated key presses nothing but is consumed when
  the button could be pressed; a missing, hidden or disabled button or an `Activate`
  returning false leaves the key to the shell (macOS would beep). Placeholder text fields
  take no keys yet, so Return in the form's name field presses the default button, as in
  an AppKit dialog. The test-side `exampleForm` now builds through `newFormExample`, which
  wires Cancel and Create Account as `examples/form` does. Tests: `buttons_test.go`
  (routing after the focused element and its ancestors, the focused element taking
  Return, modifiers, ⌘., other keys, key-ups and repeats, hidden and disabled buttons
  and ancestors, replacing, re-marking, unmarking and removing, one element as both, an
  `Activate` returning false, a cancel that closes the window; the `Button` placeholder),
  and in `gogui` `TestFormKeysOnAqua` (Return ignored while Create Account is disabled,
  the validation and success alerts once the terms are accepted, the success alert
  closing the window, Escape and Ctrl+. closing it); 100 % coverage kept; no paint dump,
  a11y dump or example layout changed. `check.sh` (stress run included) passes;
  `go.mod` unchanged. Nothing needs another OS beyond the cross-builds; nothing manual.

**S3.3a Control state machine and press tracking** ✅ *2026-10-26* (the step text as planned):
* `ControlState` (normal / hover / pressed, focused, disabled-in-tree, window key or
  inactive, appearance) computed from the element, its router and window, plus a reusable
  `pressTracker`: a primary press captures, dragging off un-presses, a release over the
  control acts, a release outside does nothing. The traffic lights move onto it (their
  tests unchanged).
* Space (no modifiers, not auto-repeat) on a focused `FocusControl` element calls
  `Activate` (PLAN §5.5); a text-kind element keeps Space.
* Done when: tests for every state transition (press/drag-off/drag-back/release-outside,
  disabled, inactive window, Space and its repeat) pass. No visual change.

* **Result:** new `aqua/control.go`. `ControlState{Interaction, Focus, Disabled, Key,
  Appearance}` (`Interaction`: `InteractionNormal`/`Hover`/`Pressed`; `String()` for test
  logs) is computed fresh from the element, its router and window, never cached:
  `Base.ControlState()` for the parts without a press, `pressTracker.State(e)` for a
  pressable control. Pressed is the tracker's press with the pointer over the control; a
  press dragged off is normal, not hover (the router freezes the hover chain during a
  capture, so it is not read then); hover is membership of the router's hover chain; a
  control disabled in the tree is never hovered or pressed. Outside a window `Key` is
  the router's `Active` (true without a router) and the appearance the zero value. The
  unexported `pressTracker` (zero value idle; `Track(self, ev) (handled, act)`, `Reset`,
  `Pressed`, `Down`) is the traffic lights' old logic moved out: a primary press captures
  and presses, `Over` drives pressed while captured, the primary release acts only when
  over the control, `PointerCancel` drops the press; other buttons are not handled when
  idle and swallowed during a press; each pressed-look change invalidates. The traffic
  lights embed it (`trafficLight.press`); their tests are unchanged except one line of
  `TestWindowEdgeCases` that read the removed `l.down`/`l.pressed` fields and now reads
  `l.press.Down()`/`Pressed()`. **Space:** the `Router` (not the window, so plain routers
  get it too) passes a Space key-down that no element handled to `spaceActivates`: no
  modifiers, a focused usable `FocusControl` element and an active router → `Activate`,
  consumed when it returns true; a repeat is consumed without a press; a `FocusText`
  element keeps Space (it reaches it first and the router does not take it, so it is
  typed); an inactive window or a missing focus leaves the key unhandled. **Not in the
  step text:** Space acts on key-down, like Return; AppKit highlights a button on Space
  down and acts on key-up, which the real push button (S3.3e) can add if it is wanted. A
  focused placeholder without a primary action (checkbox, radio …) does not consume
  Space. Tests: `control_test.go` (press, drag off, drag back, release over and outside,
  other buttons, a stray release, disabling mid-press and through an ancestor, the
  router's cancel on a shell leave; focus, key and appearance through a plain router,
  detached and in a fake-shell window, inactive and dark; Space, its repeat, typed Space
  without text, modifiers, a focused element handling it, a text element, an inactive
  window, a disabled element, the `Button` and `Checkbox` placeholders); 100 % coverage
  kept; no paint, a11y or layout dump changed. `check.sh` (stress run included) passes;
  `go.mod` unchanged. Nothing needs another OS beyond the cross-builds; nothing manual.

**S3.3b Tokens for the basic controls and the sizing switch** ✅ *2026-10-26* (the step text as planned):
* Add to `sequoia.json` (all "from memory, to be measured") and `Theme`: push button
  (min width, title font weight, pressed/default fills via the accent rules), checkbox
  (box size, radius, mark stroke, gap to title), radio (diameter, dot size, gap), Box
  (corner radius, border, fill, title font and gap, content insets), separator thickness.
  Regenerate `tokens.go`; theme tests for the new fields and their ranges.
* Switch `TestExamplesOnAqua` to the sizing policy above (pinned dumps, regenerated on
  purpose with a `-update` flag), still on the placeholders, so later steps show their
  frame changes as dump diffs.
* Done when: `check.sh` passes; the stub metrics are untouched.

* **Result:** a new token group `controls` in `sequoia.json` and the generator (`Groups`,
  the spec key) for a new `Theme.Controls` struct, added as the last `Theme` field so the
  existing groups keep their order; 22 tokens, all "memory": the bezel colors
  `BezelFill`, `BezelPressedFill`, `BezelBorder` (shared by the push button, the checkbox
  box and the radio circle when off) and `BoxFill`, `BoxBorder`; `BoxCornerRadius`,
  `BoxBorderWidth`, `BoxTitleSize`, `BoxTitleGap`, `BoxContentInsetX`/`Y`,
  `SeparatorThickness`; per control size `ButtonMinWidth`, `CheckboxSize`,
  `CheckboxRadius`, `CheckboxMarkStroke`, `CheckboxGap`, `RadioSize`, `RadioDotSize`,
  `RadioGap`; the weights `ButtonTitleWeight` and `BoxTitleWeight` (regular). `tokens.go`
  and `docs/tokens.md` regenerated (116 tokens, all provisional). **Not in the step
  text:** "pressed/default fills via the accent rules" became computed helpers in the new
  `theme/controls.go` instead of stored fills: `ButtonFill` (a default button in a key
  window is the accent, `AccentPressed` while pressed; in an inactive window it is a plain
  bezel, as accent.go already said), `ButtonText` and `ToggleFill` (on/mixed: accent,
  pressed, `AccentInactive`; off: the bezel fills), plus `Controls.BoxContentInsets()`.
  The Box title font is a size and a weight (the small system font, 11 pt regular) rather
  than a font value. Tests: `theme/controls_test.go` (`TestSequoiaControls`: ranges,
  growth mini ≤ small ≤ regular ≤ large, each part inside its control, concrete visible
  colors, a distinct pressed bezel; `TestControlFills`: every state of the three helpers,
  light and dark, with a non-default accent); the generator tests needed no change
  (they iterate `Groups`). **Sizing switch:** `TestExamplesOnAqua` now compares each
  example's aqua frame dump with `testdata/aqua/<example>.frames` (pinned now, rewritten
  with `go test -run TestExamplesOnAqua -update .`; CRLF tolerated; a line diff on
  failure). The example builders call `wantAnchor` instead of `wantFrame`: on headless
  the old exact numbers, on aqua only "no overlap with a sibling, nothing clipped by an
  ancestor" (a scroll view's document exempt; `TestMisplaced` checks the check); the
  notes example's document-size check is headless only. **Not in the step text:**
  `TestExamplesOnAquaMatchHeadless` keeps the old equality as an explicit pin while every
  control is a stub-sized placeholder; the step that first resizes a control drops the
  examples it changes from it. The stub metrics, every paint and a11y dump and the
  headless layout tests are unchanged. `check.sh` (stress run included) passes; `go.mod`
  unchanged. Nothing needs another OS beyond the cross-builds; nothing manual.

**S3.3c Label** ✅ *2026-10-27* (the step text as planned):
* A real label element: text in the control font through `TextSystem` (wrapping to the
  frame width, the `Font` and color of the peer, `ColorLabel` / disabled color),
  `FittingSize` from the layout, a11y `text` with the label. Replaces the placeholder.
* Done when: paint and a11y dumps of `hello` are re-pinned and the label tests pass.

* **Result:** new `aqua/label.go`. `Label` embeds its stub label (the state, as before)
  and a `peerBase`, which now binds any element (`initElement`; the field `elem`, with
  `el` only for placeholders), so the `driver.Widget` methods are shared; `Caption`
  reports false for it. Its `label` element: font = the peer's `Font` with a zero size
  replaced by `Typography.FontSize` for the control size (`TextFont`); `FittingSize` =
  the `TextSystem` layout (wrapped at `max.W` with `Wraps`, so real line widths instead
  of the stub's area estimate), clamped, a stub `Fixed` still winning; one layout cached
  per label. Paint: the text from the frame's top (as an NSTextField label), clipped to
  the frame only when it does not fit, in `TextColor` (`ColorLabel` when zero) for the
  window's appearance (`appearanceIn`, shared with the placeholders), and
  `ColorDisabledControlText` when disabled in the tree. Not focusable; a11y `text` with
  the text. Every setter that changes the look repaints (text, font, color, alignment,
  wraps, control size). **Not in the step text:** alignment is per line: leading draws
  the layout once; centre and trailing draw a single line offset by the free width (at
  least 0, so an overflow is clipped at the end) and several lines as one layout per
  visual line (`lines`: a line starts where the caret moves down; breaks and wrap spaces
  trimmed, empty lines skipped), as the `TextLayout` has no alignment. `AlignJustified`
  is drawn leading (no justification in the `TextSystem`); `SetSelectable` is recorded
  only (selection belongs to the text controls); the theme's line height and tracking
  are not applied (the `TextSystem` uses the font's own metrics). **Dumps:** the `hello`
  paint dump is re-pinned (`"Hello!"` in 15 pt bold at the frame's top-left instead of a
  placeholder rect with an 11 pt caption); its a11y dump is unchanged (the node was
  already `text "Hello!"`); the placeholder `ScrollView` test now looks for the real
  label's text. No frame changed: the fake text metrics equal the stub's for unwrapped
  text and no example wraps a label, so `testdata/aqua` and
  `TestExamplesOnAquaMatchHeadless` stay as they are (a wrapped label would now differ
  from headless on aqua, by design). Tests: `label_test.go` (not a placeholder, the
  node, fonts per control size, `FittingSize` cases, the layout cache, paint dumps for
  position, alignments, clipping, font and colors, disabled through an ancestor, empty
  frame or text, multi-line and wrapped alignment, `lines`, every setter invalidating,
  dark appearance, the a11y frame, `Focus`, hiding and `Destroy` in a fake-shell
  window); `internal/aqua` at 100 % coverage. `check.sh` (stress run included) passes;
  `go.mod` unchanged. Nothing needs another OS beyond the cross-builds; nothing manual
  (the user's visual check of the controls is in S3.3h).

**S3.3d Separator and Box** ✅ *2026-10-27* (the step text as planned):
* Separator: a hairline (`ColorSeparator`, pixel-aligned at every scale), horizontal or
  vertical by its frame. Box: the rounded group fill and border, its title above the
  content in the small font, content insets from the tokens (the stub `ContentInsets`
  stay for headless). a11y `separator` and `group` with the title.
* Done when: their paint, layout and a11y tests pass.

* **Result:** new `aqua/group.go`; the placeholder `Separator` and `Box` are gone from
  `controls.go`, and with them the placeholder's `plain` and `band` fields. Both are
  real elements built like the `Label` (stub state, `peerBase.initElement`). New pixel
  helpers: `scaleIn` (the window surface's `Scale`, 1 outside a live window),
  `pixelWidth` (whole device pixels, at least one) and `pixelAlign` (edges snapped in
  root coordinates, so it holds inside any parent). **Separator:** `SeparatorThickness`
  rounded to device pixels, along the frame's longer side and centred across it; a
  square frame follows `SetVertical` (**not in the step text:** the tie-break; otherwise
  `SetVertical` only records); `FittingSize` is the thickness on both axes, so 1 × 1 as
  on the stub. **Box:** title in the small font (`BoxTitleSize`, `BoxTitleWeight`; one
  line, truncated with "…", `ColorLabel` or the disabled color) at the frame's top left;
  the group starts `BoxTitleGap` below the title's line height **rounded up to whole
  points** (not in the step text: keeps the insets whole) and is drawn in `BoxFill` /
  `BoxBorder` (`BoxBorderWidth` in whole pixels), pixel-aligned. `ContentInsets` =
  `BoxContentInsets()` plus that band (26/10/8/10 titled with the fake metrics, 8/10/8/10
  untitled; the stub's 24/8/8/8 stay for headless); `FittingSize` is the insets. The
  title sits at x = 0 (provisional, for S3.1t), the tokens do not vary with the control
  size, and the Box border is pixel-aligned too (beyond the step text). **Frames:**
  `examples/controls` changes (each Box 2 pt taller, the content 4 pt narrower), so
  `testdata/aqua/controls.frames` is re-pinned and `TestExamplesOnAquaMatchHeadless`
  now skips `controls` (a `resized` set); the other four examples still match headless.
  Tests: `group_test.go` (nodes, `FittingSize`, orientation, centring, snapping of
  fractional frames and parents, token variants via `withControls`, scales 1, 2 and 1.5
  and dark in a fake-shell window; Box insets, the title cache, paint dumps with and
  without title and content, disabled, truncation, small and sub-pixel frames, border
  widths, `SetTitle` re-placing the content, the a11y group in a window); the placeholder
  tests lost their Separator/Box cases; `internal/aqua` at 100 % coverage. `check.sh`
  (stress run included) passes; `go.mod` unchanged. Nothing needs another OS beyond the
  cross-builds; nothing manual (the visual check is in S3.3h).

**S3.3e Push button** ✅ *2026-10-27* (the step text as planned; the visual check is open):
* Bezel (fill, border, shadow from `Metrics.Control`), centred title, per control size;
  states from S3.3a: pressed (darkened), default button (accent fill and
  `DefaultButtonText`, grey in an inactive window), disabled; focus ring kept. Click
  through the `pressTracker`; `Activate` unchanged (Return/Escape/Space/a11y press).
* Done when: tests of every state × light/dark × key/inactive (record dumps) pass, and
  `examples/hello` on Windows shows a real button (user, visual).

* **Result:** new `aqua/button.go`; the placeholder `Button` is gone from `controls.go`,
  and with it the placeholder's `activate` field (the remaining placeholders have no
  primary action). `Button` is built like the `Label` (stub state, `peerBase.initElement`)
  with a `pushButton` element, focusable as `FocusControl`. **Size:** `Height` of
  `Metrics.Control(cs)` high (20 regular), the title plus `PaddingX` on each side,
  rounded up to whole points, at least `ButtonMinWidth` wide; `Fixed` wins. The title is
  one line in `Typography.FontSize` of the control size with `ButtonTitleWeight`, cached
  per title and font. **Paint:** the bezel spans the frame's width, `Height` high (at most
  the frame's) and is centred vertically in a taller frame (**not in the step text**, as
  an NSButton keeps its bezel height), pixel-aligned like the S3.3d elements; then
  `BezelShadow` (`DrawShadow`), `Theme.ButtonFill`, the 1 pt `BezelBorder` in whole
  device pixels (left out on the accent fill), the title centred and truncated to the
  bezel minus the padding in `Theme.ButtonText`, and the focus ring around the bezel
  last. "Grey in an inactive window" follows the S3.3b `ButtonFill` rule: an inactive or
  disabled default button is drawn as a plain one (white bezel, control text), as on
  macOS. Pressed is the `pressTracker`'s state (`BezelPressedFill`, `AccentPressed` on a
  default button). A disabled button drops a press and handles no pointer event;
  `SetTitle`, `SetDefault` and `SetControlSize` repaint. **Frames:** buttons go from the
  stub's 22 pt (text + 28) to 20 pt (at least 70 wide), so `hello`, `form` and `controls`
  are re-pinned in `testdata/aqua` and `TestExamplesOnAquaMatchHeadless` now skips
  `hello` and `form` too (only `converter` and `notes`, which have no button, still match
  headless); the `hello` paint and a11y dumps are re-pinned (Greet: 70 × 20, shadow,
  accent fill, white 13 pt title; its bezel snaps from y 112.46 to 112). The win32
  `TestSurfaceAquaWindow` (real Direct2D read-back) now samples the real button's bezel
  fill, inside its top edge and above it, light/dark × key/inactive at 1× and 2×.
  Tests: `button_test.go` (node, focus kind, `FittingSize` per size, the title cache,
  paint dumps of the states and edge cases, setters, clicks through the fake shell with
  drag off/back, release outside, secondary button and disabling mid-press, the ring,
  a11y frame and 2× snapping in a window); `TestButtonStates` pins 8 states (plain,
  pressed, default, default pressed, disabled, disabled default, focused, focused
  default) × light/dark × key/inactive as record dumps in
  `internal/aqua/testdata/button.states` (`-args -update`). The placeholder paint and Box
  tests now use a Radio placeholder. `internal/aqua` at 100 % coverage. `check.sh`
  (stress run included) passes; `go.mod` unchanged. Nothing needs another OS beyond the
  cross-builds. Space still acts on key-down (S3.3a), without the AppKit highlight between
  Space down and up; left as is (the step text keeps `Activate` unchanged).
  **Open (user, visual):** `examples/hello` on Windows shows a real button.

**S3.3f Checkbox** ✅ *2026-10-27* (the step text as planned):
* Box (off: control fill and border; on and mixed: accent fill with a check or dash path
  in `DefaultButtonText`; grey accent when inactive), the title after it; a click on the
  box or the title toggles (mixed → on), pressed state, disabled. a11y checkbox with
  checked/mixed.
* Done when: the toggle and paint tests pass.

* **Result:** new `aqua/checkbox.go`; the placeholder `Checkbox` is gone from
  `controls.go`. `Checkbox` is built like the `Button` (stub state and toggle rule,
  `peerBase.initElement`) with a `checkBox` element, focusable as `FocusControl`.
  **Size:** the box (`CheckboxSize`, 14 regular), `CheckboxGap` and the title's width,
  rounded up to whole points; as high as the box or the title's line, whichever is higher
  (16 pt regular on the fake text system, where the stub had 18); the box alone without a
  title; `Fixed` wins. The title is one line in `Typography.FontSize` of the control size,
  cached per title and font. **Paint:** the box at the leading edge, centred vertically
  and pixel-aligned, `CheckboxRadius` rounded, filled with `Theme.ToggleFill` (mixed
  counts as on, so `AccentInactive` grey in an inactive window and `AccentPressed` while
  pressed); off: the 1 pt `BezelBorder` in whole device pixels; on: a check, mixed: a
  dash, both stroked `CheckboxMarkStroke` wide with round caps and joins in `AccentText`
  (`DefaultButtonText`) on a 14-unit grid of the box, no border. Then the title after the
  box and the gap in `ColorControlText`, truncated to the frame, and the focus ring
  around the box last. **Disabled** (not in the step text's detail): the box and its mark
  are drawn in a `PushOpacity` layer at a new token, `Controls.ToggleDisabledAlpha`
  (0.5, provisional; spec, generator list, `tokens.go` and `docs/tokens.md` regenerated;
  the radio of S3.3g uses it too), and the title in `ColorDisabledControlText`; a
  disabled checkbox drops a press and handles no pointer event. **Clicks:** the whole
  frame (box and title) goes through the `pressTracker`; a release over it calls
  `UserClick` (the stub's rule: off → on, on → off, mixed → on, then the handler's
  `StateChanged`) and repaints. `Activate` toggles too, so Space and the a11y press
  toggle a focused checkbox; refused while disabled. **Frames:** checkboxes go from 18 to
  16 pt high (and 2 pt wider: 14 + 6 instead of 18 before the title), so `controls`,
  `form` and `notes` are re-pinned in `testdata/aqua` and
  `TestExamplesOnAquaMatchHeadless` now skips `notes` too: only `converter` still matches
  headless. Tests: `checkbox_test.go` (node per state, focus kind, `FittingSize` per
  size and edge cases, the title cache, paint dumps of off, on, mixed, disabled and the
  edge cases, setters, clicks on the box and the title through the fake shell with drag
  off/back, release outside, secondary button and disabling mid-press, the ring and
  Space, a11y frame and 2× snapping in a window); `TestCheckboxStates` pins 9 states
  (off, on, mixed, each plain and pressed; disabled off and on; focused on) ×
  light/dark × key/inactive in `internal/aqua/testdata/checkbox.states`
  (`-args -update`). `TestSpaceOnButton` now also toggles a checkbox and uses a Radio
  placeholder for "not consumed"; the placeholder caption and ring tests dropped the
  checkbox. `internal/aqua` at 100 % coverage. `check.sh` (stress run included) passes;
  `go.mod` unchanged. Nothing needs another OS beyond the cross-builds; no visual check
  is part of this step (`examples/controls` and `examples/form` are looked at in S3.3h).

**S3.3g Radio** ✅ *2026-10-27* (the step text as planned):
* Circle and dot (accent when on, grey when inactive), title; a click turns it on and the
  public `RadioGroup` keeps exclusivity (no peer API change). Arrow keys inside a group are
  left out (not in v0.1; noted in ARCHITECTURE.md). a11y radio with checked.
* Done when: the radio and `RadioGroup`-on-aqua tests pass.

* **Result:** new `aqua/radio.go` and `aqua/toggle.go`; the placeholder `Radio` is gone
  from `controls.go`. `toggle.go` takes what the checkbox and the radio share (title font,
  cached one-line title layout, mark + gap + title size, title paint), so `checkbox.go`
  shrank. `Radio` is built like the `Checkbox` (stub state and click rule,
  `peerBase.initElement`) with a `radioButton` element, focusable as `FocusControl`.
  **Size:** the circle (`RadioSize`, 14 regular), `RadioGap` and the title's width,
  rounded up; as high as the circle or the title's line (16 pt regular on the fake text
  system, where the stub had 18); `Fixed` wins. **Paint:** the circle at the leading
  edge, centred vertically and pixel-aligned, filled with `Theme.ToggleFill` (accent when
  on, `AccentInactive` grey in an inactive window, pressed fills while pressed); off: the
  1 pt `BezelBorder`; on: a centred `RadioDotSize` dot in `AccentText`, scaled with the
  circle, no border; disabled: circle and dot in the `ToggleDisabledAlpha` layer and the
  title in `ColorDisabledControlText`. The title as the checkbox's, the focus ring around
  the circle last. **Clicks:** the whole frame through the `pressTracker`; a release
  calls `UserClick` (on, `StateChanged(StateOn)` even when already on, like AppKit);
  `Activate` (Space, the a11y press) the same, refused while disabled. Exclusivity stays
  in the public `RadioGroup` (`SetOn` on the others), no peer API change. Arrow keys are
  not handled (not in v0.1; ARCHITECTURE.md and the PLAN.md §4.4 table note it). a11y
  `radio` with the title, `checked` when on, `press`. **Frames:** radios go from 18 to
  16 pt high (2 pt wider), so `controls` and `form` are re-pinned in `testdata/aqua`
  (`TestExamplesOnAquaMatchHeadless` unchanged: both were already skipped). Tests:
  `radio_test.go` (node off/on, focus kind, `FittingSize` per size and edge cases, the
  title cache, paint dumps incl. disabled, truncation, shrunk circle and scaled dot,
  setters, clicks on the circle and the title through the fake shell with drag off/back,
  release outside, secondary button, disabling mid-press, `Activate`, the ring, arrow keys
  ignored and Space, a11y frame, 2× snapping, hide, `Destroy`); `TestRadioStates` pins 7
  states × light/dark × key/inactive in `internal/aqua/testdata/radio.states`
  (`-args -update`); `TestSpaceOnButton` turns a real radio on (the "not consumed" case
  uses the PopUpButton placeholder); `TestRadioGroupOnAqua` (root package) clicks the
  radios of a `RadioGroup` on aqua: exclusivity via the a11y `checked` states, `OnChange`
  once per new index, nothing for the selected radio or in a disabled group (drawn
  dimmed), Space on a focused radio. `internal/aqua` at 100 % coverage. `check.sh`
  (stress run included) passes; `go.mod` unchanged. Nothing needs another OS beyond the
  cross-builds; no visual check is part of this step (`examples/controls` and
  `examples/form` are looked at in S3.3h).

**S3.3h Wrap-up** ✅ *2026-10-27* (the step text as planned):
* A test that walks the five example trees on aqua and checks every control node has its
  role, label and states (§7.5); the placeholder code is left only for the controls of
  S3.4–S3.5 and text fields; ARCHITECTURE.md placeholder list updated.
* Done when: `check.sh` passes and the user has looked at `examples/controls` and
  `examples/form` on Windows in light and dark, key and inactive (visual).

* **Result:** test only, no library change. `TestExamplesAccessibleOnAqua` (root
  package, `aqua_test.go`) builds each of the five examples on aqua over the fake shell
  and walks its widget tree (radios of a `RadioGroup` and form row labels included). For
  every visible widget with a peer it builds the node of its element
  (`aqua.AccessibilityTree(ElementOf(peer))`, so the tree-level states count) and
  compares it with `wantNode`, derived from the widget's public getters (`Label.Text`,
  `Checkbox.State`, `RadioGroup.Selected`, `TextField.Text`/`Secure`, `Slider.Value`,
  `Indeterminate`, `PopUpButton.Item`, `Box.Title`, `TextView.Text`) and, where there is
  no getter, its stub state (button, checkbox and radio titles, placeholders,
  read-only): role, label, value, checked/mixed/read-only, disabled through an
  ancestor, focusable plus the focus action for enabled controls, focused for the
  router's focused element, and the actions. Stacks, forms and radio groups must stay
  layout helpers (role none). Each node must also be in the window's tree, and every
  exposed content node (title bar left out) must belong to a widget, so nothing is
  missing or extra. A mutation check (the radio's `checked` dropped) fails it in `form`
  and `controls`. **Placeholders left** (`controls.go`): `Slider`, `Progress` (S3.4),
  `ScrollView` (S3.5), `TextField`, `TextView` and `PopUpButton` (phase 5, the pop-up in
  S5.5); ARCHITECTURE.md lists them so. `check.sh` (stress run included) passes; `go.mod`
  unchanged. **Open (user):** the visual check of `examples/controls` and
  `examples/form` on Windows in light and dark, key and inactive (and the one of
  `examples/hello` from S3.3e).

### S3.4 Value controls (S3.4a–S3.4g)

**S3.4a Tokens for the value controls** ✅ *2026-10-28* (the step text as planned):
* Spec, generator list, `tokens.go` and `docs/tokens.md` (all provisional, "to be
  measured"): slider track thickness and radius, knob diameter, knob fill, border and
  shadow (offset, blur, color), track colors (filled and empty), tick-mark length, width,
  gap and color, per control size; progress bar track thickness and radius, track color,
  the indeterminate band (width, period); spinner diameter per control size, spoke count,
  length, width, period.
* Theme helpers for the accent rules: the filled part of a slider track and of a
  progress bar (accent; `AccentInactive` in an inactive window; disabled alpha).
* Done when: the theme tests (spec ↔ generated tokens, contrast where it applies) pass.

* **Result:** 27 new tokens in the `controls` group (`Theme.Controls`), all `memory`:
  the colors `SliderTrack` (the empty part), `SliderKnobFill`, `SliderKnobBorder`,
  `SliderKnobShadow`, `SliderTickMark`, `ProgressTrack` and `SpinnerSpoke`, each with an
  increased-contrast pair except the knob fill; the numbers `SliderKnobBorderWidth`
  (0.5), `ProgressBandFraction` (0.25, the indeterminate band's width as a fraction of the
  track), `SpinnerSpokes` (12, a whole number stored as a float), `SpinnerTailAlpha`
  (0.25, the faintest spoke relative to `SpinnerSpoke`, fading linearly) and
  `ValueDisabledAlpha` (0.5); per control size `SliderTrackThickness` (4 regular),
  `SliderTrackRadius`, `SliderKnobSize` (20, 14, 11, 24), `SliderKnobShadowOffsetY`/`Blur`,
  `SliderTickLength`, `SliderTickWidth`, `SliderTickGap`, `ProgressThickness` (6, 4, 3, 8),
  `ProgressRadius`, `SpinnerSize` (32, 16, 10, 32), `SpinnerSpokeLength` and
  `SpinnerSpokeWidth`; the durations `ProgressBandPeriod` (1.2 s, one crossing) and
  `SpinnerPeriod` (1 s, one turn), the first durations outside `Animation`. The generator
  already handled durations in any group, so only the `Groups` lists changed (the group's
  heading is now "Controls"); `tokens.go` and `docs/tokens.md` regenerated (144 tokens,
  all provisional). **As in S3.3b the filled track color is not stored:** one helper,
  `Theme.ValueFill(a, key, disabled)` (`theme/controls.go`), gives the filled part of a
  slider's track, of a progress bar and the indeterminate band: the accent in a key
  window, `AccentInactive` in an inactive one, its alpha × `ValueDisabledAlpha` when
  disabled (a token of its own beside `ToggleDisabledAlpha` so it can be measured
  separately); no pressed shade. **Not in the step text:** the spoke color and its tail
  fade, so the spinner steps need no new tokens. Tests (`theme/controls_test.go`):
  `TestSequoiaValueControls` (ranges, mini ≤ small ≤ regular ≤ large, pill tracks, the
  knob wider than twice the track, spokes that fit and stay apart, fractions in (0, 1), a
  whole spoke count, plausible periods, concrete colors, an opaque knob),
  `TestValueContrast` (tracks, tick marks and spoke on the window background, the knob
  border on the knob: visible, and more so with Increase Contrast, light and dark; the
  filled track differs from the empty one) and `TestValueFill` (key, inactive, disabled,
  both, with a non-default accent). `TestGeneratedUpToDate` and the gen tests pass
  unchanged. `check.sh` (stress run included) passes; `go.mod` unchanged. Nothing needs
  another OS beyond the cross-builds.

**S3.4b `anim`: repeating animations** ✅ *2026-10-28* (the step text as planned):
* A looping kind on `Value` (for example `Loop(period)` over 0…1, wrapping): runs on the
  frame clock until `Stop`, requests frames only while it runs, and doesn't run with
  Reduce Motion (it stays at its start, and turning Reduce Motion on stops it).
* Done when: fake-clock tests (phase at a frame time, wrap-around, stop, Reduce Motion,
  no frames requested once stopped) pass.

* **Result:** `anim/timeline.go`, a third kind of `Value` beside the tween and the
  spring: `Loop(period)`, a phase over 0…1 that wraps once per period. At a frame time t
  it is frac((t − start)/period) from the exact nanosecond remainder (not `math.Mod`), so
  a late frame, or one 1000 h later, is exact and never drifts; clamped below 1 (it could
  round up only beyond 2⁵³ ns). A loop starts at 0, its start: `Loop` jumps there at the
  clock's `Now` (the frame time inside a `Tick`) and moves from the next frame on. It
  never ends, so the timeline requests frames until `Stop` (keeps the phase), `Set`,
  `AnimateTo`/`SpringTo` or Reduce Motion. Calling `Loop` with the running period does
  nothing (owners may call it on every event); another period retimes it from the
  current phase without a jump. `Velocity` is 1/period seconds (the wrap not counted),
  `Target` the current value. A retarget from a loop always starts (even to the current
  value) from the loop's phase; a spring takes the loop's velocity. With Reduce Motion
  `Loop` acts like `Set(0)`, and `SetReduceMotion(true)` stops running loops at 0;
  turning it off restarts nothing, the owner calls `Loop` again (S3.4f). Panics on
  period ≤ 0. Tests (`anim/loop_test.go`): the phase per frame and its wrap-arounds, a
  late and a far frame, the clamp below 1, `Stop`/`Set` with frame counts, Reduce Motion
  both ways, same period / new period / from a tween, retargets to a tween and a spring,
  a loop started inside a `Tick`. Stress run of `anim` and `check.sh` pass; `go.mod`
  unchanged. Nothing needs another OS beyond the cross-builds.

**S3.4c Slider: size and paint** ✅ *2026-10-28* (the step text as planned):
* New `aqua/slider.go`; the placeholder `Slider` leaves `controls.go`. FittingSize from
  the tokens (knob and tick marks across, the stub default length along; vertical too;
  `Fixed` wins). Paint: the track, the accent-filled part from the minimum to the knob,
  the knob with its shadow, tick marks; pressed, disabled and inactive (grey) states;
  the focus ring around the knob; horizontal and vertical. a11y node unchanged (value,
  increment/decrement).
* Done when: the size and paint tests (recorded canvas, both orientations, light/dark,
  key/inactive, disabled) pass.

* **Result:** `aqua/slider.go`, a real `Slider` built like the `Checkbox` (stub state,
  `peerBase.initElement`, `FocusControl`); the placeholder and its caption test left
  `controls.go`/`controls_test.go`, and the slider left the focus-ring placeholder loop.
  FittingSize: 96 along (the stub's default), the knob across (20 regular) plus gap and
  marks with tick marks (28), rounded up, turned when vertical. Layout in
  `sliderGeometry`: the knob/marks band centred across, the track the whole length
  through the knob's centre line, the knob travelling half a knob in from each end
  (minimum left / bottom), marks spread over the travel (one in the middle) and skipped
  when they would leave the frame (found while testing). Paint: track (`SliderTrack`),
  filled part to the knob's centre (`ValueFill`, grey inactive), marks, knob shadow,
  fill (`BezelPressedFill` while dragged), inner border; disabled: every color at
  `ValueDisabledAlpha`, no layer; focus ring around the knob, last; pixel-aligned.
  Tests (`slider_test.go`): real peer and node, FittingSize per control size, marks,
  orientation, rounding and `Fixed`; paint at the value, the ends, an empty range,
  disabled (also through an ancestor), tall/low/short/empty frames, mini; tick marks
  (5, 1, cut off), vertical (marks from the bottom up), redraws, focus ring, the window
  (a11y frame, inactive grey, scale 2, hidden, destroyed), and the pinned
  `testdata/slider.states` (normal, pressed, disabled, focused, marks, vertical, vertical
  pressed with marks; light/dark × key/inactive). The converter's slider is now 20 pt
  high (was 22), so `testdata/aqua/converter.frames` and `controls.frames` were
  re-pinned, and `TestExamplesOnAquaMatchHeadless` was removed (converter was its last
  example). Nothing needs another OS beyond the cross-builds.

**S3.4d Slider: pointer and keyboard** ✅ *2026-10-28* (the step text as planned):
* Drag the knob with capture (the value follows the pointer, `ValueChanged` on every
  change); a press on the track jumps the knob there and keeps dragging; snap to tick
  marks when set; a disabled slider ignores the pointer. Keys while focused: arrows step
  (a small fraction of the range, or one tick when snapping), Page Up/Down a larger
  step, Home/End the ends. The a11y increment/decrement use the same step.
* Done when: the drag, snap and key tests pass, and `converter`'s handler sees the
  values.

* **Result:** in `slider.go`. A primary press captures; on the knob it grabs it where hit
  and leaves the value alone until the pointer moves along (a review found the
  value→pixel→value round trip changed it and fired `ValueChanged`); on the track the
  knob jumps first, and a handler that disables or hides the slider then ends the press
  (no stuck drag). Moves follow (clamped, off the frame too), `valueAt` gives the exact
  ends, snapping goes to the nearest mark (the last exactly the maximum); `userSet`
  reports only real changes. Keys (no modifiers): arrows by `Step` (range/20, or one
  mark when snapping to ≥ 2), Page Up/Down by `PageStep` (range/5, at least `Step`),
  Home/End; consumed at the ends too. `constrain` maps a value within 1e-9 of the range
  of an end to that end (steps can't stop just short of it) and NaN to the minimum.
  `Increment`/`Decrement` (a11y) use `Step`; `UserDrag` goes through `userSet` (snaps,
  no report for no change, ignored when disabled in the tree). Tests
  (`slider_input_test.go`): knob drag with grab offset, off the frame, clamping,
  swallowed buttons; track press; cancel by disable, hide and PointerCancel; vertical;
  snapping (5 marks, 1 mark, off); empty range; disabled; keys incl. modifiers;
  increment/decrement; exact ends; no round trip on a knob press; a handler disabling
  or hiding on the press; `UserDrag`. `TestSliderOnAqua` (root) wires a converter-like
  tree and drags and keys the real slider through the fake surface: `OnChange` sees each
  value and the fields follow. The user's check of the slider on Windows is in S3.4g.

**S3.4e ProgressBar and Spinner: determinate** ✅ *2026-10-28* (the step text as planned):
* New `aqua/progress.go`; the placeholder `Progress` leaves `controls.go`. Bar:
  FittingSize from the tokens (thickness across, the stub default length along), the
  track and the accent fill to the value fraction, grey when inactive. Spinner: the size
  of its control size, spokes drawn by fraction for a determinate value. a11y node
  unchanged (value; none when indeterminate).
* Done when: the size and paint tests (fractions 0, ½, 1, an empty range, both styles,
  light/dark, key/inactive) pass.

* **Result:** `progress.go`: one real peer for bar and spinner (stub state, `peerBase`
  with `initElement`), not focusable. Bar 96 × `ProgressThickness` (6 regular), spinner
  `SpinnerSize` square (32), rounded up, clamped, `Fixed` wins. The bar's track is
  centred vertically, the fill from the left to the fraction in `ValueFill` (grey when
  inactive, nothing for an empty range or NaN; its radius at most half its width),
  pixel-aligned. The spinner's 12 round-capped spokes start at twelve o'clock and go
  clockwise; a smaller frame scales it, a bigger one centres it; determinate, the first
  ⌊fraction × 12⌋ are in `SpinnerSpoke`, the rest at `SpinnerTailAlpha`. Disabled: every
  color at `ValueDisabledAlpha`, no layer. Tests (`progress_test.go`): the peer is real,
  a11y node, sizes per control size for both styles, bar paint (0, ½, 1, a short fill,
  empty range, disabled, taller frame, mini, key/inactive, dark, scale 2, hidden,
  `Destroy`), spinner paint (0, ½, 1, below one spoke, empty range, centred, scaled,
  small, disabled), redraw on each setter, and `testdata/progress.states` (bar and spinner,
  enabled and disabled, light/dark × key/inactive). The 6 pt bars re-pinned
  `testdata/aqua/converter.frames` and `controls.frames`; the placeholder caption test
  dropped `Progress`.

**S3.4f Indeterminate animation** ✅ *2026-10-28* (the step text as planned):
* The bar's moving band and the spinner's rotating spokes on an S3.4b loop. The loop runs
  only while the indicator is indeterminate, visible in the tree and in a shown window;
  it stops when any of those ends, and starts again when they return. With Reduce Motion
  a static look (the full band dimmed; the spinner at rest).
* Done when: fake-clock tests pass: frames advance the phase, hiding the indicator or its
  window, or `SetIndeterminate(false)`, stops the frame requests, Reduce Motion draws
  the static look, and an idle app requests no frames.

* **Result:** the phase is an `anim.Value.Loop` on the window's timeline
  (`ProgressBandPeriod` 1.2 s for the bar, `SpinnerPeriod` 1 s for the spinner, retimed
  on a style change). The band (a quarter of the track) enters at the left and leaves at
  the right, clipped to the track; the spinner's lead spoke goes round, the ones behind
  fading to the tail. New `Window.Shown` (after `Show`, until closed). The loop runs only
  while live (indeterminate, visible in the tree, in a shown window with a timeline).
  `SetIndeterminate(false)` and hiding the indicator stop it at once; a hidden ancestor,
  leaving the window and closing it stop it at the next frame (the change func checks);
  it restarts from the setters and from any paint that finds it live, which also covers
  Reduce Motion turning off (every window repaints). Reduce Motion: the whole track in
  `ValueFill` at half alpha, the spinner at rest with the lead at twelve o'clock. The
  driver interface has no window hide; a minimized window still counts as shown (the
  shell does not report it). Tests (`progress_anim_test.go`, a driver on the fake
  clock): band positions and wrap-around, spinner lead and fade, retiming, stopping by
  `SetIndeterminate(false)`, hiding the indicator and an ancestor, leaving the window,
  `Destroy`, `Close`, a window never shown; restarting on each return; Reduce Motion on
  and off for both styles; an idle app with determinate and hidden indicators requests
  no frames. Mutation checks: without the per-frame liveness check, or without the
  Reduce Motion look, the tests fail. Full `check.sh` passed (800 s with the stress run).
  Nothing needs another OS beyond the cross-builds; the user's visual check is in S3.4g.

**S3.4g Wrap-up** ✅ *2026-10-28* (the step text as planned):
* `converter` and `controls` on aqua: frames pinned, the a11y walk updated, no
  placeholders left for these peers (the remaining ones: ScrollView, TextField, TextView,
  PopUpButton). ARCHITECTURE.md (the slider, progress and loop contracts), the full
  `check.sh`, `native.sh` on windows/amd64.
* Done when: all checks pass; the user checks `converter` and `controls` on Windows
  (drag and keys on the slider, the sync bar, the indeterminate bar; light and dark, key
  and inactive).

* **Result:** test only, no library change. The frames of `converter` and `controls`
  were already re-pinned by S3.4c and S3.4e (`TestExamplesOnAqua` passes unchanged), and
  the a11y walk already covered the real slider and progress nodes. New in
  `TestExamplesAccessibleOnAqua`: every walked widget whose peer is still a placeholder
  (`aqua.Caption`) must be a `TextField`, `TextView`, `ScrollView` or `PopUpButton`; a
  mutation check (`PopUpButton` dropped from that list) fails it in `controls`. The
  slider, progress and loop contracts were already in ARCHITECTURE.md §9/§9.2 (S3.4b–f);
  §10 notes the new check, §0's status line marks S3.4 complete. **Finding:** the stress
  run hit two win32 flakes on the live desktop, neither in code this step touched:
  `TestSurfaceMouse` saw the extra captured `WM_MOUSEMOVE` again (first seen in S3.2c),
  so, as that finding proposed, the test now drops moves after the first release (a move
  while captured is valid; 160 isolated reruns passed); and once `TestSurfaceWaitable`
  timed out with the UI thread stuck inside `D3D11CreateDevice` (the OS, not our code;
  not reproduced). The third full `check.sh` (stress run included) passes; `native.sh` on
  windows/amd64 passes (`native-logs/windows-amd64.log`); `go.mod` unchanged. **Open
  (user):** the visual check of `examples/converter` and `examples/controls` on Windows:
  drag and arrow/Page/Home/End keys on the slider, the sync bar following it, the
  indeterminate bar moving; light and dark, key and inactive.

### S3.5 ScrollView (S3.5a–S3.5h)

**S3.5a Tokens for scrolling** ✅ *2026-10-28* (the step text as planned):
* Spec, generator list, `tokens.go` and `docs/tokens.md` (all provisional, "to be
  measured"): overlay scroller thickness (idle, hovered), inset from the edge, minimum
  knob length, knob color and alpha (light, dark, increased contrast), the hover-widen
  duration; legacy scroller thickness, track color and border; physics: wheel line height
  (points per line), the wheel step animation duration, the momentum decay rate (per
  second), the stop velocity, the velocity window, the rubber-band constant `c` (as a
  fraction of the viewport), the page overlap for paging.
* Done when: the theme tests (spec ↔ generated tokens, ranges, contrast of the knob on
  the window background) pass.

* **Result:** a new token group `scroll` (`Theme.Scroll`, `theme.go`, the last field;
  the generator's `Groups` and the raw spec gained it, nothing else in `gen` changed),
  16 tokens, all `memory`: the colors `ScrollerKnob` (translucent black/white at 45 %,
  contrast 75 %), `LegacyScrollerTrack` (opaque) and `LegacyScrollerBorder` (contrast
  pairs except the track); the numbers `ScrollerThickness` (6, idle overlay knob),
  `ScrollerHoverThickness` (9, hovered, and the legacy knob), `ScrollerInset` (2),
  `ScrollerMinKnobLength` (18), `LegacyScrollerThickness` (15), `WheelLineHeight` (16 pt
  per line), `MomentumDecayRate` (2 per second), `MomentumStopVelocity` (20 pt/s),
  `RubberBandFraction` (0.55, c as a fraction of the viewport) and `PageOverlap` (10);
  the durations `ScrollerHoverDuration` (150 ms), `WheelStepDuration` (150 ms) and
  `VelocityWindow` (80 ms). **Not as in the step text:** the knob alpha is part of the
  color token (light, dark, increased contrast), not a separate number; the hover widen
  is a duration in this group, while the fade timing and `RubberBandSpring` stay in
  `Animation` where S3.1d put them. The window is kept below the Win32 shell's 100 ms
  end gap, so the gap alone never reads as a stop. `tokens.go` and `docs/tokens.md`
  regenerated (160 tokens, all provisional). Tests (`theme/scroll_test.go`):
  `TestSequoiaScroll` (ranges, knob widths inside the legacy track, a pill-shaped
  minimum knob, a 5000 pt/s fling stopping within 0.5–6 s, the window between 30 and
  100 ms, concrete visible colors, a translucent knob) and `TestScrollContrast` (the
  knob on the window background, the text background and the legacy track, the border
  on the track: visible and stronger with Increase Contrast, light and dark).
  `TestGeneratedUpToDate` and `TestGroupsMatchTheme` pass; `check.sh` (stress run
  included) passes; `go.mod` unchanged. Nothing needs another OS
  beyond the cross-builds.

**S3.5b `scroll`: physics (pure math)** ✅ *2026-10-28* (the step text as planned):
* New package `internal/aqua/scroll` (no element code): `RubberBand(d, c)` with its
  inverse, so a drag past the edge maps 1:1 back; exponential decay (position and velocity
  at `t`, stop time at the stop velocity, rest offset); a `VelocityTracker` over
  (`Time`, `Delta`) samples in the token window that ignores a trailing zero-delta
  `PhaseEnded` and the gap before it (the Win32 timeout).
* Done when: table tests for the formula and its inverse (continuity at the edge,
  monotonic, bounded by `c`), decay against the closed form, and the tracker on synthetic
  Win32 bursts (steady, accelerating, stopped-then-ended → zero velocity) pass.


* **Result:** the new package `internal/aqua/scroll` (stdlib only, one axis at a time;
  nothing imports it before S3.5c). `RubberBand(d, c)` = d·c/(d+c), odd, slope 1 at the
  edge, below c, and `RubberBandInverse(x, c)` = x·c/(c−x) (±Inf at |x| ≥ c); c ≤ 0 means
  no overscroll. `Decay` (`NewDecay(v0, k, vs)`, panics unless k > 0 and finite): `At(t)`
  in closed form, `Duration` = ln(|v0|/vs)/k, **`Rest` is the position at the stop**
  ((|v0| − vs)/k, vs/k short of `Asymptote` v0/k), so the motion ends exactly there, then
  (`Rest`, 0); a release at or below vs has no momentum; vs ≤ 0 runs to the asymptote.
  `VelocityTracker` (`Window`, `EndGap`): `Add(t, d)` for the moving events, the velocity
  is the deltas within the window over the span they cover (the anchor's own delta left
  out, so steady d per Δt gives exactly d/Δt); equal or out-of-order times merge into the
  latest sample, too few samples give 0. **The end rule:** `End(t)` ignores the end
  event's delta and returns the velocity as of the last sample when t is at most `EndGap`
  after it (`DefaultEndGap` 150 ms: Win32's 100 ms `gestureTimeout` plus timer slack), 0
  when longer (fingers resting before the lift); fingers that stop while still touching
  show as zero deltas (Win32 keeps reporting), which give 0 once they fill the window.
  `EndGap` is a package constant, not a token: it follows the shell's timeout, not the
  look. Tests (`rubberband_test.go`, `decay_test.go`, `velocity_test.go`) as in "Done
  when", plus the edge cases; 96 % coverage. Written with a helper agent that stopped
  early (API error); the velocity tests and one decay tolerance (1 µs before the stop
  is vs·1 µs short of `Rest`) were finished by hand. `check.sh` (stress run included)
  passes; `go.mod` unchanged. Nothing needs another OS beyond the
  cross-builds.

**S3.5c `scroll`: the offset model on the frame clock** ✅ *2026-10-28* (the step text as planned):
* `anim`: a spring with an explicit start velocity (`SpringFrom(x, v0, Spring)`) and a
  decay motion on the `Timeline`, both jumping under Reduce Motion.
* `scroll.Axis` (one per axis, offset in `[0, max]` plus overscroll): precise drag 1:1
  with rubber-banding past the edges; on release momentum from the tracker, switching to
  the `RubberBandSpring` spring-back with the edge velocity; OS `Momentum` phases (GTK)
  followed instead of its own decay; wheel lines → line height → a short `AnimateTo`
  that accumulates further notches; `ScrollTo(x, animated)`, `Stop`, `SetMax` (clamps,
  spring-back if past the new edge). Reduce Motion: no momentum, no rubber-band, jumps.
* `fake`: a timed swipe (the clock advances between deltas, then the Win32-style late
  `PhaseEnded`).
* Done when: fake-clock tests pass for drag, fling and decay to rest, fling into an edge
  and spring back, overscroll drag and release, wheel steps (single, rapid notches), OS
  momentum, Reduce Motion, and `SetMax` shrinking under an overscrolled offset.

* **Result:** `anim` gained `Value.SpringFrom(x, v0, s)` (released with an explicit velocity
  from the position at the start; always restarts, except a value at rest at x with v0 = 0)
  and `Value.Decay(v0, rate, vs)` (a new kind: closed-form exponential momentum from the
  position at the start, ending exactly at its rest when the speed reaches vs; `Target` is
  the rest; |v0| ≤ vs stops in place; panics for vs ≤ 0, as a decay must end); both jump
  under Reduce Motion (100 % coverage; written by a helper agent that was resumed once after
  an API error). `scroll.Axis` (`axis.go`; `scroll` now imports `anim`, `theme` and `shell`):
  `NewAxis(tl, Params, onChange)`, `ThemeParams(th)`; one `anim.Value` holds the shown
  offset (overscroll included), `Clamped`/`Overscroll`/`Target`/`Moving`/`Dragging`;
  `Event(ev, d)` (shell direction) reports whether it took the delta. Drags map the fingers'
  raw offset through `RubberBand` (catching overscrolled content through the inverse); the
  release velocity is the tracker's times the rubber band's slope. **Decisions not in the
  step text:** a release past an edge springs back with its velocity unless it flings back
  into the range (then it decays); a decay that reaches an edge springs back from it with
  its speed (`SpringFrom`), capped at 0.9·c·ω₀·e so the critically damped overshoot stays
  below c; OS momentum stops the axis's own decay (started at `PhaseEnded`, since the axis
  cannot know whether momentum follows), and at an edge it springs back and ignores the rest
  (also when the release was springing back already); wheel steps never overscroll and are
  not taken at an edge (so they can bubble); a phase-less precise event scrolls 1:1,
  clamped; `SetMax` clamps a rested offset at once, re-shows a drag, retargets a spring and
  springs back a decay or momentum left past the new edge; `CanScroll` for the scroll view.
  `fake.Surface.TimedSwipe(p, interval, deltas…)` with `SwipeEndDelay` (100 ms, Win32's
  gesture timeout). Tests: `axis_test.go` (fake clock: everything in "Done when" plus
  catch during spring-back, fling back from overscroll, fractional notches, gestures
  without `PhaseBegan`, OS momentum during a spring-back, no viewport; 93 % coverage),
  `swipe_test.go`. `check.sh` (stress run included) passes; `go.mod` unchanged. Nothing
  needs another OS beyond the cross-builds.

**S3.5d ScrollView element: content scrolling** ✅ *2026-10-28* (the step text as planned):
* The real `ScrollView` peer (`scrollview.go` in `internal/aqua`), same peer API: the
  viewport and document hosts kept, the offset driven by two `scroll.Axis`; `ScrollEvent`
  → axis (only the axes that have a scroller, as in AppKit) → `Scrolled`; an unconsumed
  delta (at an edge, no scroller on that axis) bubbles to the parent; `SetOffset` and
  `ScrollRectToVisible` stop any animation and do not fire `Scrolled`.
* Scroll latching in the router: a precise gesture stays with the element that took its
  `PhaseBegan` until `PhaseEnded` (and its momentum) is over.
* Paint culling: `PaintTree` skips children outside a clipping ancestor's bounds.
* Done when: tests pass for the `notes` sidebar through the fake shell (swipe, wheel,
  edges, latching, culling via `render/record`), the a11y frames follow the offset, and
  the aqua frame dumps are unchanged or re-pinned (no scrollers drawn yet).

* **Result:** `internal/aqua/scrollview.go` replaces the placeholder (`controls.go`): the
  `scrollArea` element (a11y scroll area, draws nothing until S3.5e) holds the clipping
  viewport and the document host at minus the shown offset; two `scroll.Axis` on the
  driver's timeline (a still Reduce-Motion timeline when the driver is not running, e.g. in
  unit tests that never call `Run`: everything jumps). The stub's `Off` stays the clamped
  offset; `Scrolled` fires once per change of it from input or momentum; `SetOffset`,
  `ScrollRectToVisible`, `UserScroll` stop any motion; `SetFrame`, `SetDocumentSize`,
  `UserResizeViewport` only update the axes' viewport and range; none fires `Scrolled`.
  **Bubbling, not in the step text in this detail:** a wheel (or phase-less precise) event
  is taken when an axis with a scroller can move by it; a `PhaseBegan` is declined only when
  no axis can move by it *and* an enclosing scroll view can, else it is taken and rubber-
  bands, so a top-level view bounces at its edges. Router latching (`router.go`, helper
  agent): the element that handled `PhaseBegan` gets the rest of the gesture and the OS
  momentum after it, alone and unbubbled, until Momentum `PhaseEnded` or any other scroll
  event; `Router.Latched()`; a latched element that leaves the tree, is hidden or disabled
  drops the latch and the rest of that gesture reaches nobody. Paint culling (`paint.go`,
  helper agent): the clipping ancestors' clip carried down; an element whose frame grown by
  `paintOverflow()` (focus ring and widest shadow from the tokens plus 1 pt, 10 pt with
  Sequoia) misses it is skipped (a non-clipping one keeps visiting its children) and
  cleaned. The `aqua.ScrollEvent` doc mentions latching. Tests: `scroll_aqua_test.go` (the
  notes sidebar through the fake shell: wheel, edges, editor and horizontal axis, drag with
  momentum, fling into the end and back, rubber band, latching across the editor, OS
  momentum, culling via the paint dump, `SetOffset` without `OnScroll`, a11y frames
  following the offset), `paint_cull_test.go`, `router_scroll_test.go`,
  `scrollview_test.go` (nested views), `TestScrollViewFrames` (renamed from the placeholder
  test). The aqua frame dumps are unchanged (they record layout only);
  `TestExamplesAccessibleOnAqua` still allows `*ScrollView` until S3.5h. `check.sh` (stress
  run included) passes; `go.mod` unchanged. Nothing needs another OS beyond the
  cross-builds; the visual check is S3.5h's.

**S3.5e Overlay scrollers: paint and fade** ✅ *2026-10-29* (the step text as planned):
* Knob geometry (proportional, minimum length, shortened while rubber-banding), drawn on
  top of the content inside the viewport; shown when the user scrolls (not on
  `SetOffset`), faded out after `ScrollerFadeDelay`
  over `ScrollerFadeDuration`; a hidden scroller is not hit-tested. Reduce Motion: no
  fade animation (show/hide).
* Done when: state goldens (`testdata/scroll*.states`: idle, visible, fading, overscroll,
  dark, increased contrast) and fake-clock tests of the fade timing pass.

* **Result:** `internal/aqua/scroller.go` adds one `scroller` element per axis, children of
  the `scrollArea` after the viewport (drawn on top, no a11y node). Overlay knob: a strip
  inside the viewport's trailing edge (the two strips cut the corner when both axes are
  active), proportional length at least `ScrollerMinKnobLength`, shortened by the
  overscroll × track/viewport down to a round knob pinned to the pushed edge, no knob
  without range, `ScrollerKnob` × opacity. The opacity is an `anim.Value`: `Set(1)` on user
  scrolling (a taken scroll event, every offset change from input or momentum, `UserScroll`;
  not `SetOffset`, `ScrollRectToVisible` or layout), then the fade re-armed by each change so
  it counts from the end of the motion; at 0 the element is hidden (neither painted nor
  hit-tested). **New in `anim`, not in the step text:** `Value.AnimateAfter(x, delay, d, c)`,
  a delayed tween (frames requested during the delay, no timers; under Reduce Motion the
  delay still runs and the value jumps at its end), used for the fade. Tests:
  `scroller_test.go` (`testdata/scroll.states`: idle, visible from wheel and swipe, fading,
  overscroll top/bottom and round, dark, increased contrast, window and system, both
  scrollers; fade timing on the fake clock, after momentum, restart, not shown by program,
  no range, Reduce Motion, knob geometry), `anim/animateafter_test.go`, the shared harness
  `scrollenv_test.go`. The aqua frame dumps are unchanged (layout only). `check.sh` (stress
  run included) passes; `go.mod` unchanged. Nothing needs another OS beyond the
  cross-builds; the visual check is S3.5h's.

**S3.5f Scroller interaction and legacy scrollers** ✅ *2026-10-29* (the step text as planned):
* Overlay: hover over the scroller area shows it and widens it (animated); knob drag
  (pointer capture via the `pressTracker` pattern, offset follows 1:1, clamped);
  click on the track pages towards the click (macOS default).
* Legacy: when `Appearance.LegacyScrollers` is on, the scrollers are always visible with a
  track and take their thickness from the viewport (`ViewportInset` → `ViewportResized`,
  so layout re-runs); switching it at runtime (`WindowStateEvent`) does the same.
* Done when: tests for hover-widen, knob drag, track click, legacy layout (viewport size,
  `ViewportResized` fired once, frame dump of `notes` with legacy scrollers), the runtime
  switch, and the legacy state goldens pass.

* **Result:** in `scroller.go` and `scrollview.go`. Overlay hover: pointer moves bubbling to
  the `scrollArea` (which handles none) show the scroller over its strip and widen the knob
  to `ScrollerHoverThickness` over `ScrollerHoverDuration` (jump under Reduce Motion);
  `PointerLeave` ends it and arms the fade. A primary press on the knob captures and drags
  the offset 1:1 (clamped, momentum stopped, `Scrolled` per change); on the track it pages by
  max(viewport − `PageOverlap`, viewport/2), animated like a wheel step, accumulating from a
  running page's target. Release or `PointerCancel` ends it; disabled: no input. **Found in
  review (helper agent), fixed:** while a scroller is pressed the scroll view swallows scroll
  events (latched OS momentum moved the offset under a held knob), and a track click during
  momentum paged from the momentum's rest instead of the offset (new `scroll.Axis.Stepping`).
  Legacy: `Appearance.LegacyScrollers` sets `ViewportInset` to `LegacyScrollerThickness` on
  the right/bottom from the appearance and the scroller flags only (it cannot oscillate);
  read at creation, silently on `SetFrame`, and on `SetHas…Scroller` and `WindowStateEvent`
  with `ViewportResized` fired once through the stub's `UserResizeViewport`; the track
  (`LegacyScrollerTrack`, a device-pixel `LegacyScrollerBorder`, the corner filled), the knob
  `ScrollerHoverThickness` and opaque, always shown; `Legacy()`. Tests:
  `scroller_input_test.go` (hover-widen, knob drag, cancel and other buttons, drag stopping
  momentum and swallowing scroll, disabled, track click, during momentum, Reduce Motion),
  `scroller_legacy_test.go` (layout, runtime switch, `SetHas…Scroller`,
  `testdata/scroll_legacy.states`), `scroll_legacy_aqua_test.go` (the `notes` frame dump with
  legacy scrollers, `testdata/aqua/notes_legacy.frames`, and the live switch). Left as
  designed: a custom `UserResizeViewport` inset is replaced on the next legacy change; turning
  Reduce Motion off during a fade delay does not bring the fade back (`anim`'s "turning it off
  restarts nothing"). `check.sh` (stress run included) passes; `go.mod` unchanged. Nothing
  needs another OS beyond the cross-builds; the visual check is S3.5h's.

**S3.5g Keyboard paging and scroll-to-focused** ✅ *2026-10-29* (the step text as planned):
* Keys bubbling to the scroll view: Page Up/Down (viewport minus the page overlap),
  Home/End, Space and Shift-Space (only when the focused element is not a `FocusControl`,
  so Space still presses a focused button), the arrows by a line when the scroll view
  itself has the focus; animated like wheel steps, jumps under Reduce Motion.
* `Router`: a focus-changed hook; the nearest `ScrollView` ancestors (innermost first)
  bring the focused element's rect (plus the focus-ring outset) into view.
* Done when: tests pass for each key, Space on a focused checkbox (toggles, no paging),
  and Tab through the `notes` checkboxes keeping the focused one visible.

* **Result:** in `scrollview.go` (`ScrollView.key`, `reveal`) and `router.go` (the focus
  hook `revealFocus`, the `focusRevealer` interface the `scrollArea` implements). Keys that
  bubble to the scroll view (key-down and repeat, not while a scroller is pressed): Page
  Up/Down by viewport − `PageOverlap`, Home/End, Space/Shift-Space unless a `FocusControl`
  other than the scroll view is focused, arrows by `WheelLineHeight` only with the scroll
  view itself focused; paging moves the vertical axis (else the horizontal one), arrows only
  their own axis with a scroller. A step starts from a running step's target (repeats add
  up), animates like a wheel step (a jump under Reduce Motion), fires `Scrolled` and shows
  the overlay scrollers; any modifier except Shift on Space leaves the key alone, and a key
  that cannot move bubbles on to an enclosing scroll view. The focus hook runs after every
  focus move (`SetFocus`, `Focus`, `FocusNext`), after the `FocusEvent`, innermost scroll
  view first; each brings the frame grown by `FocusRingBand(1)` into its viewport by the
  least scroll, at once (so an outer one sees the inner one's final offset). Clearing the
  focus and `SetActive` scroll nothing. The driver interfaces and the public API are
  unchanged. Tests: `internal/aqua/scrollview_keys_test.go` (each key, repeats, edges,
  modifiers, Space on a checkbox toggling without paging and paging on a text element,
  arrows, Reduce Motion, a pressed scroller, Tab with least scroll, a tall element, nested
  scroll views for reveal and for End/Page Down bubbling); `scroll_keys_aqua_test.go`
  (helper agent: Tab and Shift-Tab through the 30 `notes` checkboxes keeping each revealed
  with the least scroll, the editor and a cleared focus scrolling nothing; Page Up/Down,
  End/Home and Space on the `notes` sidebar resized to 720 × 300, since at 720 × 480 a page
  is longer than the whole range). `check.sh` (stress run included) passes; `go.mod`
  unchanged. Nothing needs another OS beyond the cross-builds; the visual check is S3.5h's.

**S3.5h Wrap-up** ✅ *2026-10-29* (the step text as planned):
* `TestExamplesAccessibleOnAqua` no longer allows `*ScrollView` as a placeholder; the
  `notes` frames and goldens re-pinned; ARCHITECTURE.md (`scroll` package, the router's
  latching and focus hook, culling), `docs/tokens.md`, §5.6 if behaviour moved, and the
  §10 tree (momentum and rubber-band live in `scroll`, `anim` has the generic decay).
* Done when: `check.sh` passes. **User check on Windows** (open): `go run ./examples/notes`
  — the sidebar scrolls with a precision touchpad (1:1, momentum, rubber-band at both
  ends), with a wheel mouse (smooth line steps), overlay scrollers fade and widen on hover,
  knob drag and track click work, Settings → Accessibility → "Always show scrollbars"
  switches to legacy scrollers live, Page Up/Down, Home/End, and Tab scroll the focused
  checkbox into view; light and dark.

* **Result:** test and docs only, no library change. `TestExamplesAccessibleOnAqua` now
  allows only `TextField`, `TextView` and `PopUpButton` as placeholder peers (it passes,
  so the `notes` scroll view is a real peer). The `notes` frames (`notes.frames`,
  `notes_legacy.frames`) and the aqua goldens were already pinned by S3.5d–S3.5g: `-update`
  on `TestExamplesOnAqua` and on `internal/aqua` changes nothing. `docs/tokens.md` is
  generated from `spec/sequoia.json` and already lists every scroll token
  (`TestGeneratedUpToDate`); each token is used. A helper agent audited ARCHITECTURE.md
  against the code: it added the S3.5e–S3.5f tests to §10, which were missing, and filled
  smaller gaps (latch dropped by its own handler, `revealFocus` stopping on an unusable
  element, the scrollers' Reduce Motion and disabled behaviour, empty-clip culling, the
  `scroll` package in §9's dependency list). §0 still matches PLAN.md §2. PLAN.md §5.6 now
  describes what was built: the trigger for legacy scrollers and the live switch, the
  track-click page length, the split between `scroll` and `anim`, latching, which features
  Reduce Motion turns off, key bubbling, scroll-to-focused for any focused element, and
  per-element culling. §10's `anim`/`scroll` lines are corrected. The full `check.sh`
  (stress run included) passes; `native.sh` on windows/amd64 passes; `go.mod` unchanged.
  **Open (user):** the Windows check of `examples/notes` above.

### S3.6 Golden-image tests (S3.6a–S3.6e)

**S3.6a Gallery scene (`internal/aqua/gallery`, pure Go)** ✅ *2026-10-29* (the step text as planned):
* `Build(d driver.Driver)`: a fixed-frame grid of every v0.1 control that is real by then:
  labels (system and small fonts, primary/secondary/disabled colors), push buttons
  (normal, default, cancel, disabled, the three control sizes), checkboxes
  (off/on/mixed/disabled), a radio group, a slider, a determinate progress bar (and the
  indeterminate one only if Reduce Motion gives it a static frame), separators, a box,
  and a scroll view with content scrolled off both axes. TextField, TextView and
  PopUpButton are left out until phase 5, which adds them and regenerates the goldens.
* Tests on the aqua driver over the fake shell: no placeholder in the scene, no two
  frames overlap, everything inside the scene size, and a pinned recorder dump
  (`testdata/gallery.dump`, `-update`) in light and dark.

* **Result:** a new package `internal/aqua/gallery` (`gallery.go`), importing only
  `internal/driver`. `Build(d) (driver.Widget, driver.Size)` as designed: a container
  of 600 × 346 pt with fixed frames in three columns, split by a vertical separator:
  five labels (system, secondary, `ColorDisabledControlText`, 11 pt, bold), a horizontal
  separator, seven push buttons (plain, default, cancel, disabled, small, mini, large);
  five checkboxes (off, on, mixed, disabled on, "Focused") and a box "Radio Group" with
  four radios (the second on, the fourth disabled); three sliders (plain, five snapping
  tick marks, disabled), a determinate and an indeterminate bar and an indeterminate
  spinner (Reduce Motion gives both a static look, so both are in), and a scroll view with
  both scrollers over a 360 × 250 document of 3 × 8 labels at offset (60, 40). Beyond
  the plan: `Open(d, h)` creates the window (`Title`, content `Size`, `Style` with every
  title-bar button), shows it and focuses the "Focused" checkbox, because focus needs a
  window and `Build` has no window yet; S3.6c and S1.5 both use `Open`. The exported
  `Size`, `ScrollOffset`, `Title` and `Style` pin what the tests compare. Finding: the
  first test run showed the document's labels outside a 0 × 0 container, because a
  `ScrollView` frames only its document host; the layout engine arranges the document
  widget itself at (0, 0, size), so the scene now does too.
  Tests (`gallery_test.go`, aqua driver over the fake shell with Reduce Motion and legacy
  scrollers, through `Driver.Run`): `TestGalleryScene` (a recording driver wraps the
  aqua factories: no placeholder, the count per kind, the root at (0, 0, `Size`), in every
  container each child non-empty, inside and overlapping no sibling, the box content inside
  its content area, frames at least the fitting height and, for text controls, the fitting
  width of the fake metrics, the scroll offset, the focused checkbox in the key window, no
  running animation, no paint errors); `TestGalleryBuild`; `TestGalleryDump` (the whole
  window, title bar included, light and dark, `testdata/gallery.dump`). The driver
  interfaces and the public API are unchanged; `go.mod` unchanged. ARCHITECTURE.md: the
  package in §1 (tree, dependency rules, table), a new §9.4 and its §10 line; §0 is
  unchanged (PLAN.md §2 already lists `gallery/`). The full `check.sh` (stress run
  included) passes. Nothing needs another OS beyond the cross-builds.

**S3.6b Image comparison (pure Go)** ✅ *2026-10-29* (the step text as planned):
* `gallery` (or `gallery/golden`): read and write PNGs, and `Compare(want, got)` with a
  perceptual tolerance: a YIQ color distance per pixel (as pixelmatch does), a threshold
  for one pixel, and a maximum count of differing pixels. Anti-aliased edge pixels count
  less. It returns a diff image and a short report (the pixels over the threshold, the
  largest delta, the bounding box), and a size mismatch is reported as a failure.
* Unit tests on synthetic images: identical, a 1-unit noise everywhere (passes), a moved
  edge (fails), a changed color (fails), a size mismatch.

* **Result:** a new package `internal/aqua/gallery/golden` (`golden.go`, standard library
  only, no build tags), so the scene package stays free of image code. `ReadPNG`,
  `WritePNG` (creates the directory; deterministic bytes; straight alpha, RGB when every
  pixel is opaque), `NRGBA` (a compact straight-alpha image at the origin: the input
  itself when it already is one, else a converted copy, premultiplied images
  un-premultiplied). `Compare(want, got, Options) (Result, *image.NRGBA)` ports
  pixelmatch (ISC): squared YIQ distance against 35215 × `Threshold`², and its
  `antialiased`/`hasManySiblings` edge detection (border counted as one equal neighbour,
  x-outer scan order for ties). Two deliberate deviations: translucent pixels are blended
  over white and over black and the larger distance taken (an alpha-only change is not
  hidden), and anti-aliased pixels are counted with `AAWeight` instead of ignored;
  `Weighted = Diff + AA × AAWeight` passes when at most `MaxDiff` (+1e-9 for float
  rounding). `DefaultOptions` `{0.1, 0.1, 0}` tolerates no differing pixel; S3.6d may
  raise `MaxDiff` once it sees what WARP varies. `Result` carries both sizes,
  `SizeMismatch` (fails, no diff image), `Diff`, `AA`, `Weighted`, `MaxDelta` (threshold
  scale), `Bounds`, `Pass`, and a one-line `String`; the diff image shows unchanged
  pixels as faded grey, plain diffs red, anti-aliased diffs yellow. Tests
  (`golden_test.go`, 11): the five planned cases plus an anti-aliased edge column (counted
  only as AA, while the same change in a flat area counts plainly), a single pixel against
  `MaxDiff`, alpha-only and transparent-pixel cases, premultiplied images, sub-images and
  other strides, the float-rounding case, PNG round trips and byte determinism, and
  `ReadPNG` errors. A review sub-agent checked the port against pixelmatch and found no
  bugs; its three minor findings (the rounding margin, two doc comments, a stricter
  premultiplied test) are in. The driver interfaces and the public API are unchanged;
  `go.mod` unchanged. ARCHITECTURE.md: the package in §1 (tree, dependency rules, table),
  §9.4 and §10; §0 unchanged. The full `check.sh` (stress run included) passes. Nothing
  needs another OS beyond the cross-builds.

**S3.6c Offscreen gallery render on Windows** ✅ *2026-10-30* (the step text as planned):
* A test helper in `internal/shell/win32`: it builds the gallery in an aqua window on the
  hidden surface at 96 and 192 DPI, light and dark, on the WARP device, waits until no
  animation runs, reads the frame back and un-premultiplies it to RGBA. `newGPU` gets the
  WARP option for this; the shell's own device choice is unchanged.
* Tests: the frame size per scale, no render or recorder errors, no leaked references,
  and two renders in a row give identical pixels. Rewords §8.2 (no WIC) and ARCHITECTURE.md.

* **Result:** `newGPU(warp bool)`: with `warp` the D3D11 device is created with
  `CreateDeviceOfType(WARP, BGRA_SUPPORT)`, otherwise `CreateDevice` as before; the shell
  passes the new test-only field `Shell.warp` (also on device-loss recreation). Fixed on
  the way: `newGPU`'s failure path released through the named result after `return nil,
  err` had set it to nil (a nil dereference on any device failure); it now releases a
  local. Beyond the plan: `gallery.Fill(d, w)`, `Open` without creating or showing the
  window (they share `content`), because `Open` shows the window and the render must stay
  on a hidden one; `TestGalleryFill` on the fake shell. The helper
  (`internal/shell/win32/gallery_test.go`): `withGalleryRenderer` runs the aqua driver on a
  test shell with `warp` and an injected `fakeSys` that is the same on every machine
  (light, default accent, no high contrast, animations off so Reduce Motion, transparency
  on, "Always show scrollbars" so legacy scrollers), creates the gallery window, `Fill`s
  it, keeps it hidden and makes it key with a synthetic `WM_ACTIVATE`; `render(variant)`
  sets the DPI (synthetic `WM_DPICHANGED`), forces light/dark through
  `Driver.SetAppearance`, runs clock frames until `Timeline.Running()` is 0 (fails after
  20), paints through `WM_PAINT`, reads back with `framePx` and returns `unpremultiply`
  (rounded, clamped, transparent pixels as transparent black) as `*image.NRGBA`.
  `galleryVariants` and their names `gallery-{light,dark}-x{1,2}` are what S3.6d uses.
  Tests: `TestUnpremultiply` (six cases) and `TestGalleryRender`: the four variants at
  600 × (346 + title bar) pt in pixels, opaque content and title bar, no render or
  recorder errors, a second render byte-identical, light 1× again identical after dark
  2×, the window never visible, the appearance as injected, the device WARP, no leftover
  references after Run; `GOGUI_SCREENSHOTS` writes the four PNGs (looked at on the host:
  every control as in the recorder dump, focus ring on "Focused", legacy scrollers, Inter).
  The whole run takes about 0.3 s, so it stays in the stress run (renders identical across
  80 runs). §8.2 now says: the swap-chain back buffer on WARP, read back through staging,
  encoded with `image/png`, no WIC. The driver interfaces and the public API are
  unchanged; `go.mod` unchanged. ARCHITECTURE.md: §1 (status, `Fill` in the table), §8.3
  (devices), §9.4 (`Fill`, the Windows render) and §10; §0 unchanged. The full `check.sh`
  (stress run included) passes. Nothing needs another OS beyond the cross-builds.

**S3.6d Goldens and native.sh** ✅ *2026-10-30* (the step text as planned):
* `TestGalleryGolden` (windows): renders the four variants and compares each with
  `internal/aqua/gallery/testdata/golden/gallery-{light,dark}-x{1,2}.png`; `-update`
  rewrites them. On failure it writes the actual image and the diff to
  `GOGUI_GOLDEN_OUT` (or `t.TempDir`) and names the files in the message.
* The first goldens are generated on the host and committed. `native.sh` gets a verbose
  golden step on Windows, so the comparison report lands in the log.

* **Result:** `TestGalleryGolden` in `internal/shell/win32/gallery_test.go` renders the
  four variants through `withGalleryRenderer` (S3.6c) and compares each with its golden
  under `golden.DefaultOptions` (threshold 0.1, anti-aliased pixels 0.1, `MaxDiff` 0),
  logging the one-line report of every comparison; a missing golden fails with a hint to
  `-update`. `go test -run TestGalleryGolden ./internal/shell/win32 -args -update`
  rewrites the four PNGs (the flag goes after `-args`: before the package list `go test`
  passes it, and the package list, to the test binary of the current directory). On a
  mismatch `<name>.actual.png` and `<name>.diff.png` go to `GOGUI_GOLDEN_OUT` or
  `t.TempDir` and the error names both (checked by hand with the light 1× golden swapped
  for the dark one: 210 808 differing pixels reported, both files written). The four
  goldens (600 × 374 and 1200 × 748 px, 33–62 KB each, opaque RGB) were generated on the
  host (WARP, windows/amd64), looked at (every control as in the recorder dump, the focus
  ring on "Focused", legacy scrollers, Inter, light and dark) and are committed; a fresh
  render matches them exactly (max delta 0), so `MaxDiff` stays 0 until another WARP build
  shows a difference. `.gitattributes` already marks `*.png` binary. `native.sh`: a
  `windows)` case runs `go test -count=1 -v -run 'TestGalleryGolden$' ./internal/shell/win32`
  with `GOGUI_GOLDEN_OUT=native-logs/golden`, so the reports land in the log and failure
  images next to it (not committed); it passed on windows/amd64. PLAN.md §8.4 and
  ARCHITECTURE.md (§1 status and table, the `native.sh` paragraph, §9.4, §10) updated; §0
  unchanged. The driver interfaces and the public API are unchanged; `go.mod` unchanged.
  The full `check.sh` (stress run included) passes. Open, user: `native.sh` on
  windows/arm64 (optional; shows whether the goldens hold on another WARP build).

**S3.6e Wrap-up** ✅ *2026-10-30* (the step text as planned):
* ARCHITECTURE.md (gallery, the comparison, the golden flow, how to update after a token
  change, which S3.1t uses), `examples/README.md` if needed, the full `check.sh`, and
  `native.sh` on windows/amd64. Open, user: look at the four PNGs on Windows (optional),
  `native.sh` on windows/arm64, and after S1.5 the visual review against the macOS
  references (phase 3 exit, together with S3.1t).

* **Result:** docs only, no code change. ARCHITECTURE.md §9.4 gets "The golden flow": a
  table of the five kinds of pinned output that follow the theme (generated tokens and
  `docs/tokens.md`, the per-control `.states` dumps, the example frames in
  `testdata/aqua`, the gallery recorder dump, the four gallery PNGs), each with the exact
  command that rewrites it, and the order to follow after a token change (S3.1t):
  regenerate, `go test ./...`, review the text diffs, rewrite them, then rewrite and look
  at the goldens on windows/amd64 (old PNG from `git show HEAD:<path>` or the
  `<name>.diff.png` of a run without `-update`); refactors must pass without `-update`;
  the goldens are never edited by hand, and a Linux tolerance (S4.3) goes into its own
  `Options`. Every documented command was run on the host and rewrote nothing. §1 status
  marks S3.6 complete. `examples/README.md`: one paragraph saying `controls` is the manual
  parity check while the goldens use `internal/aqua/gallery`, with a pointer to §9.4.
  §0 and PLAN.md §2 unchanged; the driver interfaces and the public API unchanged;
  `go.mod` unchanged. The full `check.sh` (stress run included) passes; `native.sh` on
  windows/amd64 passes (all four goldens at max delta 0). **Open, user:** the four PNGs on
  Windows (optional), `native.sh` on windows/arm64, and after S1.5 the visual review
  against the macOS references (phase 3 exit, with S3.1t).

---

## G. Q&A log of 2026-09-30 (was Appendix A of PLAN.md)

Answers that settled the first draft (turned into decisions in §12):

| # | Question | Answer |
|---|---|---|
| 1 | Linux controls | Shared drawn engine → D1 |
| 2 | Which macOS look | Sequoia first, Liquid Glass later → D2 |
| 3 | Menu bar on Windows/Linux | Menu strip in the title bar → D3 |
| 4 | Module path | `github.com/radix29/gogui` → D6 |
| 5 | cgo acceptable? | No cgo anywhere (purego) → D5; added `internal/ffi`, §7.2, the no-cgo guard, spike phase 0.5 and R-purego |
| 6 | Screen readers | After v0.1 → D4 |
| 7 | Minimum OS | As planned → D7 |
| 8 | Extra v0.1 controls | ScrollView, TextView → D8 (§5.6, `examples/notes`) |

---

## H. Phase 4: S4.1 (S4.1a–S4.1l) (from PLAN.md §11.1)

**S4.1a `shellkit`: shared shell logic (pure Go, host-tested)** ✅ *2026-10-30* (the step text as planned):
* Move the OS-independent helpers listed above out of `win32` into
  `internal/shell/shellkit` with their table tests (`TestClickCounter`,
  `TestEventTimeAt`, `TestGesturePhases`, `TestClampSize`, `TestToPxFromPx`, the
  region part of `TestHitTest`). `win32` calls them.
* Done when: `check.sh` and `native.sh` (windows/amd64) pass unchanged; no behaviour
  change in `win32`.

* **Result:** new untagged package `internal/shell/shellkit` (`doc.go input.go geom.go`
  + tests), depending only on `driver` and `shell`. Exported: `EventTimeAt` and
  `MaxEventAge`, `Monotonic` (the never-backwards clamp that was `Shell.lastEvent`),
  `ClickCounter`/`ClickRules`/`Point`, `Gesture`, `ToPx`/`FromPx`/`MaxPx`, `ClampSize`,
  `CenterIn`/`Rect`, and `HitRegion` (last-to-first region lookup; `ok` false = client).
  `Point` and `Rect` have the field layout of `w32.Point`/`w32.Rect`, so `win32`
  converts directly. `win32` calls them everywhere it had its own copies (`input.go`,
  `ncinput.go`, `surface.go`, `wheel.go`, `chrome.go`: `hitTest` keeps the resize borders
  and the code mapping, and asks `HitRegion` for the region); the code bodies are
  unchanged, so `win32` behaves the same. The tests moved with them (`TestHitRegion` takes
  the region cases of `TestHitTest`, which keeps the border, band, light-code, maximized
  and scale cases); new: `TestMonotonic` and an odd-rectangle click case. `centerIn` stays
  on the work-area rectangle from `MONITORINFO`. ARCHITECTURE.md: §1 tree, dependency rule
  and table, new §8.1a, §8.3 and §10 references; §0 and PLAN.md §2 unchanged (a helper
  package of the shells, no new layer). The driver interfaces and the public API are
  unchanged; `go.mod` unchanged. The full `check.sh` (stress run included) and `native.sh`
  on windows/amd64 pass. No Linux code yet, so nothing to run on Linux for this step.

**S4.1b GTK bindings (`internal/shell/gtk4/gtk`)** ✅ *2026-10-30, built only* (the step text as planned):
* Dlopen by soname of GTK 4, GObject, GLib, Graphene and Pango (Pango only opened and
  version-checked; it is bound in S4.2). Port the spike's bind tables, minus the test-only
  GSK node getters (those move to `_test.go`). The version check (≥ 4.10) and the
  "GTK missing" error: stderr, plus a `zenity`/`kdialog` message box when one is on
  `PATH`, naming `libgtk-4-1` / `gtk4`.
* Struct mirrors (`GdkRGBA`, graphene point/size/rect, `GskRoundedRect`,
  `GtkAllocation`, `GTypeQuery`, `GValue`) and constants in untagged files, with layout
  tests that run on the host. The bind tables have their own `linux` test
  (every symbol resolves).
* Done when: `check.sh` passes; the layout tests run on the host.

* **Result:** new package `internal/shell/gtk4/gtk` (`doc.go bind.go types.go version.go
  report.go load_linux.go` + tests), depending only on `internal/ffi`; imported by nothing
  yet (`shell/gtk4` from S4.1d). `Load()` (once) dlopens `libgtk-4.so.1`, `libgobject-2.0`,
  `libglib-2.0`, `libgraphene-1.0` (opened only) and `libpango-1.0` (only `pango_version`),
  binds 65 functions and checks GTK ≥ 4.10 and Pango ≥ 1.50 (what GTK 4.10's meson
  requires). The tables are one func-field struct per C prefix (`Gtk`, `Gdk`, `Gsk`,
  `GObject`, `GLib`, `Pango`; field = C name in CamelCase); the spike's set, minus the nine
  GSK node getters, now bound in `load_linux_test.go`. Errors wrap `ErrMissing`:
  `*MissingError` (soname, dlerror text, packages to install: `libgtk-4-1` / `gtk4`, and
  Debian's `libglib2.0-0t64` for GLib) or `*VersionError`. `Report(err)` prints
  `gogui: …` to stderr and shows a `zenity --error --no-markup` (else `kdialog --error`) box
  when one is on `PATH`. **Deviation from the design:** the binding tables are untagged
  (`bind.go`), only `Load` is `linux`, so `TestBindingTables` (every field bound once, from
  its library, under the name it spells) runs on the host; the S4.1 design bullet says so
  now. Mirrors and constants as planned, layout tests on the host. A review agent checked
  every signature, struct and constant against the C headers by reading: no problems.
  `go.mod` unchanged; no API change. The full `check.sh` passes on the host.
  **Not run (no machine):** the linux tests `TestLoad`, `TestNodeGettersResolve` and
  `TestReaders`; they run in `native.sh`'s `go test ./...` on Linux and need only the GTK
  libraries, no display.

**S4.1c Trampolines and the GGSurface class** ✅ *2026-10-30, built only* (the step text as planned):
* The signature-named trampoline set with the dispatch table and the `GClosureNotify`
  free. Panic capture and re-raise. The class from the spike: the class-size check
  (408), the vfunc slots at 200/232/320, the handle word, the CSS name. `measure` honours
  `for_size`; `snapshot` chains to the parent.
* The dispatch table and the panic capture are pure Go and tested on the host. The class
  tests are ported from the spike's phase-1 self-test as `linux` Go tests (no window
  needed, `gtk_init_check` under xvfb).
* Done when: `check.sh` passes; `ffi.Callbacks()` is pinned by a test.

* **Result:** new package `internal/shell/gtk4` (`doc.go panic.go dispatch.go
  trampolines.go class.go` untagged, `trampolines_linux.go class_linux.go` `linux`, +
  tests), depending on `ffi` and `shell/gtk4/gtk`; imported by nothing yet (S4.1d). Eight
  trampolines (`numTrampolines`), registered once (`registerTrampolines`, `sync.OnceFunc`):
  the GGSurface `class_init`, `size_allocate`, `measure` and `snapshot`, one shared
  `GClosureNotify`, one `GSourceFunc`, and two signal signatures, `gboolean(gpointer)`
  (`sigBool`, e.g. close-request) and `void(gint, gdouble, gdouble)` (`sigIntDoubleDouble`,
  e.g. pressed/released). One `ffi.Handles[entry{peer, kind}]` dispatch table serves
  `user_data` and the instances' handle words; a peer interface per signature; stale
  handles reach nothing. `connect` checks the signal with `g_signal_lookup` first (an
  unknown name would be a `g_critical`), passes `closureNotify` as `destroy_data`;
  `addIdle`'s handle is freed by `sourceFunc` when the source goes. Panics: `trap` in every
  trampoline (first wins, nested `*PanicError` kept, an `onPanic` hook for S4.1d's loop quit,
  itself protected), `rethrow` after the native call. Class: `checkWidgetType` (408, the
  handle word after `GtkWidget` rounded to a word), the slots at 200/232/320, CSS name
  `ggsurface`, `g_type_class_ref` forcing `class_init` with a read-back, `GtkWidget`'s own
  snapshot bound with `ffi.BindAddr` for the chain-up (the peer draws first, the parent
  always runs). `measure` passes `for_size` through and always writes its outputs (zeros
  without a peer or after a panic). **Finding:** GTK forces `for_size` to -1 for
  CONSTANT_SIZE widgets, which GGSurface is (GtkWidget's default without children), so
  for now "honours `for_size`" only means it is plumbed through; a height-for-width
  surface needs a `get_request_mode` override (slot 224, a ninth trampoline). The linux
  test calls the slot directly to check it. New in `ffi`: `CheckCallback(fn, goos) error`
  (the signature rules for any OS, no callback created), so `TestTrampolineSet` checks
  the linux set on the Windows host. `native.sh` on Linux runs `go test -v
  ./internal/shell/gtk4/...` under xvfb (the plain `go test` skips the display tests).
  A review agent read the code against the GTK/GLib sources: no ABI or signature defect;
  its findings (a flaky handle count after main-loop iterations, the `onPanic` hook
  unwinding into C, the `g_critical` of an unknown signal, a handle leak in
  `newSurfaceWidget` without a class, a wrong test comment) are fixed. ARCHITECTURE.md:
  §0 status, §1 tree, dependency rules, build tags, table, §5 `CheckCallback`, new §8.5, §10.
  `go.mod` unchanged; no public API change. The full `check.sh` (stress run included)
  passes on the host. **Not run (no machine):** the linux tests of `shell/gtk4`
  (`TestCallbacksPinned`, `TestRegisterSurfaceClass`, `TestSurfaceClassSlots`, the native
  handle-word, measure, size-allocate and snapshot tests, `TestSignalPressed`,
  `TestSignalFinalizeFrees`, `TestSignalPanicRethrown`, `TestSignalBool`, `TestIdleSource`,
  `TestCallbacksStillPinned`); `native.sh` on linux/amd64 and linux/arm64 runs them under
  xvfb.

**S4.1d Shell core and selection** ✅ *2026-10-30, built only* (the step text as planned):
* `gtk4.Shell`: `Run` (thread lock, `gtk_init_check`, `GMainLoop`, `Ready` after the wake
  is installed), `Post` over `mainthread.Queue` with one `g_idle_add` per drain, `Quit`
  (idempotent, `ErrQuit` for later `NewSurface`, surfaces closed last to first), the
  ready guard and the teardown on every exit, panics included. `Appearance` returns light
  with `driver.DefaultAccent` until S4.3. `TextSystem` is the interim fake.
* `native_linux.go` returns `gtk4.New()`. `native_other.go` becomes `!windows && !linux`.
  The `native` and `gogui` driver-selection tests change (aqua on Linux).
* Done when: `check.sh` passes; on Linux, `examples/hello` reaches `Ready` (not run here).


* **Result:** `shell/gtk4` gets the `Shell` (`shell.go wake.go interim_text.go
  interim_clock.go` untagged, `loop_linux.go` `linux`, + `shell_test.go`,
  `shell_linux_test.go`). **Design choice:** the `Shell` is untagged and drives GTK
  through a six-method `loop` interface (`init`, `run`, `quit`, `idle`, `remove`,
  `release`); `New()` and the GTK loop are linux only. So the whole shell logic runs in
  host tests over a fake loop that dispatches through the real `sourceFunc`. `Run`: thread
  lock, queue bind, `init` (`gtk.Load`, with an `ErrMissing` error shown through
  `gtk.Report`; `gtk_init_check`, whose failure names `DISPLAY`/`WAYLAND_DISPLAY`;
  `registerSurfaceClass`; `g_main_loop_new`), then `onPanic` = the loop's quit, the wake,
  `Ready`, the loop, `rethrow`, one last drain. A `Quit` in `Ready` skips the loop, because
  `g_main_loop_quit` before `g_main_loop_run` is lost. Each wake (coalesced by
  `mainthread.Queue`) adds one `g_idle_add` source with its own `kindWake` handle (a new
  dispatch kind). The pending ones are kept in a mutex-guarded set, which the teardown
  removes (`g_source_remove`, handles freed), so no handle outlives `Run`. `Quit` is
  idempotent and closes the surfaces last to first, through an `openSurface` interface
  that S4.1f's surface implements. The teardown runs on every exit after `init`, panics
  included. `NewSurface` returns an error until S4.1f (`ErrQuit` after `Quit`).
  `Appearance` is `driver.DefaultAppearance()`, `TextSystem` is `fake.NewTextSystem()`,
  and `Clock` is a standalone `fake.Clock` (interim, replaced in S4.1e). Selection:
  `native_linux.go` returns `gtk4.New()`, `native_other.go` is `!windows && !linux`, and
  `ErrNotImplemented`'s text no longer mentions phase 4. `gogui`'s `TestPlatformDriver`
  checks aqua over `*gtk4.Shell` on Linux without running it. On Linux the examples now
  reach `ready`, and their first `NewWindow` panics with the S4.1f error
  (`examples/README.md`). A test agent wrote the 18 host tests and the 4 linux tests and
  checked that they fail when the wake teardown is removed. ARCHITECTURE.md: §0 status,
  §1 tree, dependency rules, table, §8, §8.5 `Shell`, §10; PLAN.md §11.1 rule 3 and the
  tree. `go.mod` unchanged; no public API change. The full `check.sh` (stress run
  included) passes on the host. **Not run (no machine):** `TestNewGTK4` (`shell/native`),
  `TestPlatformDriver`'s Linux case, and `TestShellGTKRunQuit`, `TestShellGTKQuitInReady`,
  `TestShellGTKPostMany` and `TestShellGTKPanicInPostedWork`, which `native.sh` runs under
  xvfb on linux/amd64 and linux/arm64. Also not run: `examples/hello` reaching `Ready`.

**S4.1e Frame clock** ✅ *2026-10-30, built only* (the step text as planned):
* `FrameClock` over `GdkFrameClock`: one tick-callback trampoline
  (`gtk_widget_add_tick_callback`, removed while idle), `gdk_frame_clock_get_frame_time`
  mapped onto `Now`, and the target time of the next frame. Before a surface exists, and
  for hidden ones, a `g_timeout_add` fallback at 60 Hz. Ordering: frame function, then
  the paint of every invalidated surface. The mapping and the request/coalescing logic
  are pure Go and host-tested.
* Done when: `check.sh` passes.

* **Result:** `shell/gtk4/clock.go` (untagged) replaces `interim_clock.go`. `Now` is
  `g_get_monotonic_time` (GDK's frame-time timeline), with its 0 back-dated to `Run`'s
  call. With a pacing surface (`Shell.pacingWidget`: the first open surface whose new
  `openSurface.tickWidget()` is nonzero; S4.1f decides when) the frame runs from a tick
  callback in GDK's UPDATE phase, with GDK's predicted presentation as its target; the
  first tick without a request returns `G_SOURCE_REMOVE`. A lazy 100 ms watchdog runs a
  frame when no tick came (a minimized Wayland window gets no frame callbacks); it is
  armed once per burst and measures from the later of the last tick and the request.
  Without a pacing surface: a one-shot `g_timeout_add` at the frame's hold on a 60 Hz
  grid. Targets (`frameTimes`, pure): the first vblank strictly after the frame start,
  at least half a period after the previous target, never more than two periods ahead,
  always increasing. Frame function, then the paint pass (`clock.paint`, set by S4.1f).
  Surfaces call `surfacesChanged`, and `detach` before destroying their widget (order
  documented on `openSurface`). **Two new trampolines** (8 → 10): `tickCallback`
  (`GtkTickCallback`) and `destroyNotify` (`GDestroyNotify`, which tells a `tickGonePeer`
  before freeing, so a widget destroyed without `detach` leaves no stale id); new kinds
  `kindTick`, `kindFrameTimeout`. Five new bindings: `gtk_widget_add_tick_callback`,
  `gtk_widget_remove_tick_callback`, `gdk_frame_clock_get_frame_time`,
  `gdk_frame_clock_get_refresh_info`, `g_get_monotonic_time`; `loop` gains the
  `clockLoop` methods. **Review findings (fixed):** GDK always writes its default
  16667 µs interval without presentation feedback, so the period is taken from GDK only
  when a presentation time is reported, and the two-period cap stops a wrong period
  from making targets run ahead (it did, by 2.4× at 144 Hz); GDK's prediction is the
  vblank *at or after* the frame time, so it is moved past the start; the timeline now
  starts at `Run`'s call, as the `FrameClock` contract says; the watchdog no longer adds
  and removes a GLib source per frame; and a leftover watchdog could run a new
  animation's first frame unpaced (found by the test agent). A review agent read the
  bindings and trampolines against the GTK sources: signatures fine, `*int64`
  out-params fine under purego, the tick `GDestroyNotify` runs exactly once in all
  three cases (returned FALSE, removed, widget destroyed). A test agent wrote 27 host
  tests (`clock_test.go`, mutation-checked: 27 mutations, the three survivors
  equivalent) and another 6 linux tests (`clock_linux_test.go`, real windows under
  xvfb). ARCHITECTURE.md: §0 status, §1 dependency rules and table, §8.5 (`Shell`,
  frame clock, trampolines), §10. `go.mod` unchanged; no public API change. The full
  `check.sh` (stress run included) passes on the host. **Not run (no machine):**
  `TestClockGTKMonotonic`, `TestClockGTKTimeoutFallback`, `TestClockGTKTick`,
  `TestClockGTKTickRemoveFrees`, `TestClockGTKTickWidgetDestroyed`,
  `TestClockGTKPanicInFrame`, and the pinned callback count (10) in the older linux
  tests; `native.sh` runs them under xvfb (Wayland only from S4.1l, when weston runs
  the gtk4 tests).

**S4.1f Surface: CSD window, sizes and lifecycle** ✅ *2026-10-30, built only* (the step text as planned):
* `GtkWindow` with client-side decorations and no visible GTK title bar
  (`gtk_window_set_titlebar` with an empty widget, so GTK keeps its shadow and resize
  borders). The GGSurface is the child. Size, min and max (`set_size_request` plus
  clamping), `SetSize` through the resize path (`set_default_size`, with
  `set_size_request` as the fallback, until the spike log says which works), `Show`,
  `Minimize`, `Zoom`/`Zoomed`, `Close`/`CloseRequested`/`Closed` (with GTK owning the
  window), `KeyChanged` (`notify::is-active`), the scale (design above), `Invalidate`
  → `gtk_widget_queue_draw`, `SetCursor` (CSS cursor names), and the Paint guard.
* Done when: `check.sh` passes; on Linux, `hello` opens an empty window (not run here).

* **Result:** `shell/gtk4` gets the surfaces: `surface.go` (untagged, the whole
  `shell.Surface` logic over a new unexported `window` interface, created by the new
  `loop.newWindow`) and `window_linux.go` (`gtkWindow` over GTK), so the logic runs in
  host tests over a fake window that drives the real trampolines. The window:
  `gtk_window_new`, a hidden, peerless GGSurface as the title bar (CSD on, no space
  taken), `gtk_window_set_resizable`, the GGSurface child (size request = the minimum, or
  the size when not resizable), `gtk_window_set_default_size`, `gtk_widget_realize`
  (the `GdkSurface` exists from `NewSurface` on). Signals: `close-request` (`sigBool`),
  `notify::is-active`, `notify::scale-factor` (GGSurface), `notify::state` and, on 4.12+,
  `notify::scale` (`GdkSurface`), through **one new trampoline** `sigPointer`
  (`void (gpointer, gpointer, gpointer)`, 10 → 11) and four new kinds. The `gtkWindow`
  keeps its own references to the window, the GGSurface and the `GdkSurface`, and
  `destroy` disconnects the handlers still connected, destroys the window, releases the
  GGSurface and unrefs. The surface destroys its window itself, also on a user close:
  `close-request` asks `CloseRequested` and always returns TRUE (also after a panic,
  through a preset named result in `sigBool`). Sizes are GTK logical pixels, with `want`
  round-tripping fractional sizes. `SetSize` reports the clamped size at once, and the
  next allocation corrects it if the window manager chose another size. A first
  allocation that misses the asked size (the window's own solid-csd padding without a
  compositor) is corrected once, by a posted default size that adds the window's border
  box (`gtk_widget_compute_bounds`) minus the GGSurface. `mappedResize` selects
  `resizeDefaultSize` (default) or `resizeSizeRequest` (also raises the size request while
  growing past the last allocation); which one works stays open until the Linux log
  (`RESIZE-RESULT` lines). **Deviations:** GTK 4 has no maximum-size hint, so the maximum
  only clamps `SetSize`/`SetMaxSize` and the user can still enlarge a resizable window.
  `Center` does nothing on X11 too, because GTK 4 cannot place toplevels; the S4.1 design
  bullet says so now. Scale: `gdk_surface_get_scale` (a new **optional binding**:
  `library.optional`, left nil on 4.10/4.11) else `gtk_widget_get_scale_factor`; on
  Wayland it may be 1 until the window is mapped. Pacing: a shown surface not minimized
  or suspended (`gdk_toplevel_get_state`) paces the clock. Painting: the paint pass
  queues a draw for invalid, shown surfaces; every snapshot calls `Paint` on a
  `render/record` canvas until S4.1g, under the Paint guard. A `Resized` from
  `size_allocate` runs with `Shell.inLayout` raised, and the native part of
  `SetSize`/`SetMinSize`/`SetMaxSize`, `Close` and `Shell.Quit` is then posted. New
  bindings: 13 in `Gtk`, 2 in `Gdk`, 2 in `GObject`, and `GdkToplevelState` constants.
  **Review findings (fixed):** a panic in `CloseRequested` let GTK destroy the window, and
  the teardown then touched freed objects; `Quit` lacked the Paint guard; native changes
  from `Resized` re-entered GTK's layout; `forced` went stale across `SetSize` calls;
  `SUSPENDED` windows still paced; a window leaked when a callback panicked inside
  `newWindow`. The test agent found two more: the padding compensation measured content
  boxes and so always added 0, and the first allocation was short under solid-csd. The
  review read GTK by memory only (gitlab.gnome.org was not reachable), so the GTK
  semantics are confirmed only by the Linux run. A host-test agent wrote 26 tests in
  `surface_test.go` (34 mutations, all killed); I added `TestSurfaceFirstAllocation`
  (mutation-checked). A linux-test agent wrote 11 tests in `surface_linux_test.go`
  (real windows under xvfb; outcomes logged with `SIZE-`, `RESIZE-`, `KEY-`, `ZOOM-`
  and `MINIMIZE-RESULT` prefixes). On Linux the examples now open empty CSD windows
  (`examples/README.md`). ARCHITECTURE.md: §0 status, §1 tree, dependency rules, table,
  §8.4 optional bindings, §8.5 (`Shell`, dispatch, trampolines, new Surfaces bullet).
  `go.mod` unchanged; no public API change. The full `check.sh` (stress run included)
  passes on the host. **Not run (no machine):** the 11 `TestSurfaceGTK*` tests, the
  binding checks of `TestLoad` (now 17 more functions, `gdk_surface_get_scale` optional),
  and `examples/hello` opening an empty window; `native.sh` runs the tests under xvfb on
  linux/amd64 and linux/arm64, and `grep RESULT:` in the log answers the open resize and
  CSD-size questions.

**S4.1g Interim snapshot canvas** ✅ *2026-11-02, built only* (the step text as planned):
* The interim canvas from the design, drawn in the `snapshot` vfunc at the surface's
  scale. Node-tree tests (`linux`) through `gtk_snapshot_free_to_node`, as in the spike.
  The placeholder-box geometry is pure Go and host-tested.
* Done when: `check.sh` passes; on Linux, `hello` shows the title bar, the traffic lights
  and the placeholder boxes (not run here).

* **Result:** `shell/gtk4/interim_canvas.go` (untagged) and `interim_canvas_linux.go`,
  both deleted by S4.2. The `canvas` is a `render.Canvas` over a new unexported
  `snapshotter` interface (the GtkSnapshot calls), so as with the surface its whole logic
  runs in host tests over a fake; `gtkSnapshot` is the GTK one, and the new
  `window.snapshotter(s)` gives the surface the one for the vfunc's snapshot. User space is
  GTK's (logical pixels; GTK applies the scale), so "at the surface's scale" means the
  placeholder outlines are one device pixel (`1/scale`) wide. Real: `Save`/`Restore`,
  `Translate`, `Scale`, `Concat` (translate + scale when there is no rotation or skew,
  else `gtk_snapshot_transform_matrix` with the matrix written into a `g_malloc` scratch,
  for graphene's 16-byte alignment), rounded clips (counted per state frame and popped
  before the frame's `gtk_snapshot_restore`, which does not pop them; the surface rect is
  the first clip), `PushOpacity` (push then save), fills (color or linear-gradient nodes
  in a rounded clip) and strokes (a border node whose outline is the rect grown by half the
  width; a gradient stroke uses the gradient's middle color, since border nodes take
  colors only). Placeholder boxes, outlined in the call's color: paths (bounds plus half the
  stroke width), shadows (the shadow shape), text (the layout box), images and the
  backdrop blur (whose clip is real). Contract violations are ignored as on Win32;
  `finish` closes whatever a paint left open, in a defer, so GTK always gets a balanced
  snapshot even when `Paint` panics. With `GOGUI_DEBUG=1` the recorder checks every paint
  and a new error text is logged once. New bindings: 8 in `Gtk` (`gtk_snapshot_save`,
  `_restore`, `_translate`, `_scale`, `_transform_matrix`, `_push_opacity`,
  `_append_border`, `_append_linear_gradient`), the `ColorStop` mirror (20 bytes, layout
  test), `MatrixSize`, four node-type constants. No new trampoline (still 11). A review
  agent read the bindings and the GtkSnapshot nesting against GTK 4.10–4.14 from memory
  (gitlab not reachable): no defect; its note that a debug error logged at the frame rate
  is fixed. Known interim limits: a matrix transform is category UNKNOWN in GSK, so later
  nodes sit under transform nodes (correct, maybe slower); equal-color gradients still go
  to GSK as gradients. A host-test agent wrote 19 tests (`interim_canvas_test.go`, 44
  mutations, the 6 survivors equivalent); a linux-test agent wrote 12 node-tree tests
  (`interim_canvas_linux_test.go`, test-only GSK getters, `CANVAS-RESULT:` lines). On Linux
  the examples now draw the title bar, the traffic lights and the controls' shapes, with
  text and glyphs as boxes (`examples/README.md`). ARCHITECTURE.md: §0 status, §1
  dependency rules and table, §8.4 mirrors, §8.5 (surfaces, new Interim canvas bullet),
  §10. `go.mod` unchanged; no public API change. The full `check.sh` (stress run included)
  passes on the host. **Not run (no machine):** the 12 `TestCanvasGTK*` tests, the new
  bindings in `TestLoad`, and `hello` showing the title bar, the traffic lights and the
  placeholder boxes; `native.sh` runs the tests under xvfb on linux/amd64 and
  linux/arm64. In the log, look at the `CANVAS-RESULT:` lines and at any `Gtk-WARNING` or
  `CRITICAL` near the `TestCanvasGTK*` tests (snapshot nesting).

**S4.1h Chrome regions** ✅ *2026-11-02, built only* (the step text as planned):
* `SetChromeRegions`: a press on `RegionCaption` → `gdk_toplevel_begin_move`, a
  double-click → `Zoom`. The traffic lights are ordinary pointer events. Resize borders
  stay GTK's. Hit-testing uses `shellkit`.
* Done when: `check.sh` passes.

* **Result:** `shell/gtk4/chrome.go` (untagged) with the native side in `window_linux.go`.
  The caption works as GTK's own title bar (`GtkWindowHandle`): two primary-button
  gestures on the GGSurface, a `GtkGestureDrag` and a `GtkGestureClick`
  (`addCaptionGestures`; the widget owns them, their handlers sit in `conns` and are
  disconnected in `destroy`). One refinement of the step text: the move starts once the
  press passes GTK's drag threshold (`gtk_drag_check_threshold` on the offset, as
  `GtkWindowHandle` does), not on the press itself. A press that does not move does
  nothing, so the window manager never takes a plain click and the double-click still
  arrives. `beginMove` claims the press, converts the start to `GdkSurface` coordinates
  (`gtk_widget_compute_point` to the window, plus `gtk_native_get_surface_transform` for
  the CSD shadow) and calls `gdk_toplevel_begin_move` with the gesture's device, button
  and event time. It then resets both gestures. The second press (`n_press` 2) on the
  caption is `Zoom`, which only acts on a shown, resizable surface, and that press starts
  no move. The double-click always zooms, as on macOS and Win32; GTK's
  `gtk-titlebar-double-click` setting is not read (a fidelity choice). GTK does not
  document the order in which the two gestures see a press, so they are matched by its
  event time (`gtk_event_controller_get_current_event_time`). A time of 0 (no current
  event) matches nothing. Hit-testing is `shellkit.HitRegion` at the press position; the
  GGSurface's widget coordinates are the drawable area's points. The resize borders lie in
  GTK's CSD shadow, outside the GGSurface, so nothing reaches it from them. New: one
  trampoline `void (gpointer, gdouble, gdouble, gpointer)` (`sigDoubleDouble`,
  `doubleDoublePeer`; `numTrampolines` 11 → 12), kinds `kindCaptionPress`,
  `kindCaptionDragBegin` and `kindCaptionDragUpdate`, and the `window` methods `eventTime`,
  `pastDragThreshold` and `beginMove`. Bindings: 10 in `Gtk` (`gtk_gesture_drag_new`,
  `_single_set_button`, `_single_get_current_button`, `_set_state`, `_get_device`,
  `gtk_event_controller_get_current_event_time`, `_reset`, `gtk_drag_check_threshold`,
  `gtk_widget_compute_point`, `gtk_native_get_surface_transform`) and
  `gdk_toplevel_begin_move`; constants `EventSequence*` and `ButtonPrimary`.
* **Review:** an independent review agent read the bindings, the signal signatures, the
  lifetimes and `beginMove` against GTK 4.10 and `gtkwindowhandle.c` from memory (gitlab
  is not reachable). It found no binding or lifetime defect, and four fixes were applied:
  a never-cleared zoom flag let a time-0 press match an old zoom (now `samePress` needs a
  known time); the threshold rounded the start and the current point separately (now the
  offset, as GTK does); a `rethrow` inside the handler only relabelled a panic (removed);
  and the linux test gained the below-caption and time-0 cases. A note for S4.1i is now
  in PLAN.md: claiming the press cancels the other gestures, so the pointer gesture must
  neither deliver nor claim primary caption presses.
* **Tests:** host tests `TestCaptionMove`, `TestCaptionDoubleClick` (both handler orders,
  time 0), `TestCaptionDoubleClickNoZoom` and `TestCaptionAfterClose`, plus
  `TestSigDoubleDouble`; all go through the real trampolines over the fake window. Linux:
  `TestSurfaceGTKCaption` (the gestures, their handlers, signals through
  `g_signal_emitv`). ARCHITECTURE.md: §0 status, §8.4 constants, §8.5 (surfaces, new
  Chrome regions bullet, trampolines), §10. `examples/README.md` (Linux: the title bar
  moves and zooms). `go.mod` unchanged; no public API change. The full `check.sh` (stress
  run included) passes on the host.
* **Not run (no machine):** `TestSurfaceGTKCaption`, the 11 new bindings in `TestLoad`,
  and the move and zoom by hand. `native.sh` runs the tests under xvfb. On a Linux
  desktop, check `hello` on Wayland and X11: a drag on the title bar moves the window, a
  click without a drag does not, a double-click maximizes and restores it, the traffic
  lights do not move it, and the window can still be resized at its borders.

**S4.1i Pointer input** ✅ *2026-11-02, built only* (the step text as planned):
* `GtkEventControllerMotion` (enter, leave, move with repeats dropped) and
  `GtkGestureClick` on every button (button 0). Capture while a button is held, with
  `PointerUp` per held button on cancel, plus modifiers and event times. The shell counts
  clicks itself (`shellkit`), using GTK's double-click time and distance. A primary press
  on `RegionCaption` is not delivered (S4.1h's caption gestures own it, `surface.captionAt`;
  this gesture must not claim it). When the caption drag claims a press (`beginMove`), the
  other gestures on the GGSurface get a cancel: no `PointerUp` is owed for a press that was
  never delivered.
* Done when: `check.sh` passes.

* **Result:** `shell/gtk4/input.go` (untagged), with the native side in `window_linux.go`.
  Two controllers sit on the GGSurface (`addPointerControllers`): a
  `GtkEventControllerMotion` and a `GtkGestureClick` with button 0. They map to events as
  follows:
  * `enter` gives `PointerEnter`.
  * `motion` gives `PointerMove`, after an Enter if the pointer was outside; a repeated
    position is dropped.
  * `leave` gives `PointerLeave`.
  * `pressed`/`released` of the gesture's current button give `PointerDown`/`PointerUp`
    (GDK 1/2/3/8/9 → primary/middle/secondary/back/forward, others ignored). A release
    whose button GTK no longer names ends the one held button.
  * `cancel` gives a `PointerUp` for every held button at the last position.
  Capture is GTK's implicit grab. A `leave` while a button is held, a move outside, or a
  last release or cancel outside the drawable area sends the Leave after the last Up.
  Clicks are counted by `shellkit.ClickCounter`, using `gtk-double-click-time` and
  `-distance` from the `GtkSettings` (`g_object_get_property`, read at each press;
  400 ms and 5 px when a setting reads 0 or less).

  Modifiers come from `gtk_event_controller_get_current_event_state`: Alt and Meta map to
  Option, Super and Hyper to Command. Times come from the event's ms time mapped against
  `g_get_monotonic_time` (`Shell.eventTime`, never backwards). A primary press on
  `RegionCaption` is not delivered, and neither is its release or cancel; hover there is.

  **A known limit (GTK):** a `GtkGestureSingle` resets on a second button. The held button
  then gets its `PointerUp`, and the second press is not reported, so chords are not
  available on Linux. Fixing that would need `GtkEventControllerLegacy` and GDK event
  parsing, which is not worth it for the aqua controls.

  New: one trampoline `void (gpointer, gpointer)` (`sigVoid`, `voidSignalPeer`;
  `numTrampolines` 12 → 13), six `kindPointer*` kinds, the `window` methods `eventState`,
  `currentButton` and `clickRules`, and five bindings (`gtk_event_controller_motion_new`,
  `gtk_event_controller_get_current_event_state`, `gtk_settings_get_default`,
  `g_object_get_property`, `g_value_get_int`).
* **Review:** an independent agent read the code against GTK 4.10–4.16 from memory. All
  signatures, `get_current_button` inside the handlers, "released" after "stopped", no
  double report after "cancel", the modifier bits and `settingInt` check out. Two
  defects were fixed:
  1. When the caption drag claims a press, GTK denies it to the pointer gesture without a
     "cancel" or "released". The caption flag stuck, and later leaves and releases were
     lost. `beginMove` now also resets the pointer gesture, and a new press clears a stale
     caption press.
  2. A move outside during the grab undid the pending leave, so a cancel outside sent no
     Leave. An `enter` or move now undoes the pending leave only inside the drawable area,
     and a cancel outside sends the Leave.
* **Tests:** a test agent wrote 17 host tests (`input_test.go`), all through the real
  trampolines over the fake window: `TestInputPointerButton`, `TestInputModifiersOf`,
  `TestPointerEnterMoveLeave`, `TestPointerClicks`, `TestPointerUnknownButtons`,
  `TestPointerReleaseFallback`, `TestPointerCapture`, `TestPointerCancel`,
  `TestPointerOutsideWhileHeld`, `TestPointerStaleCaption`, `TestPointerCaption`,
  `TestPointerEventTimes`, `TestPointerCloseInDown`, `TestPointerCloseInCancel`,
  `TestPointerDuringCreate`, `TestPointerAfterClose` and `TestPointerPanic`. Its one note:
  a panic in the Enter before a move loses that move, which is acceptable because the
  panic ends `Run`. The Linux test is `TestSurfaceGTKPointer`: the controllers and their
  handlers, signals through `g_signal_emitv`, and the double-click settings (logged as
  `POINTER-RESULT:`).

  Docs: ARCHITECTURE.md §0, §8.5 (surfaces, a new Pointer input bullet, trampolines) and
  §10; `examples/README.md` (Linux gets the mouse); the package doc. `go.mod` is
  unchanged and there is no public API change. The full `check.sh`, stress run included,
  passes on the host.
* **Not run (no machine):** `TestSurfaceGTKPointer`, the five new bindings in `TestLoad`,
  and the following checks by hand. On a Linux desktop (Wayland and X11), check
  `examples/controls`:
  * hover and press on buttons, checkboxes and the slider (a drag outside the window and
    back);
  * double- and triple-clicks;
  * the traffic lights (close, minimize, zoom by clicking);
  * a title-bar drag still moves the window, and the controls react normally afterwards
    (the denied-press fix);
  * right-click and the back/forward buttons arrive.
  In the log, look at `POINTER-RESULT:`.

**S4.1j Scroll input** ✅ *2026-11-02, built only* (the step text as planned):
* `GtkEventControllerScroll` (both axes, `KINETIC`): wheel units versus precise
  (`gtk_event_controller_scroll_get_unit`, 4.8+), lines-per-notch, and gesture phases from
  `scroll-begin`/`scroll-end`. OS momentum is synthesized from `decelerate`'s velocity as
  `Momentum` Began/Changed/Ended events on the frame clock (the decay is pure Go and
  host-tested), so aqua's router latching (S3.5c–f) works unchanged.
* Done when: `check.sh` passes.

* **Result:** `shell/gtk4/scroll.go` (untagged), with the native side in
  `window_linux.go`. A `GtkEventControllerScroll` (both axes, `KINETIC`) sits on the
  GGSurface.
  * **Events.** A `scroll` becomes a `ScrollEvent` at the pointer's last position, with
    both axes negated (GTK's positive deltas show later content). Natural scrolling is
    already applied by libinput or the compositor.
  * **Units.** A `SURFACE` unit is precise, in points 1:1. A `WHEEL` unit is notches, sent
    as lines at 3 per notch. GTK has no lines-per-notch setting, so "lines-per-notch" in
    the plan is this constant. A vertical wheel with Shift scrolls horizontally, as on
    macOS.
  * **Phases.** `scroll-begin`/`scroll-end` give `PhaseBegan`, `PhaseChanged`… and a
    zero-delta `PhaseEnded`. A precise event outside them has no phase, and neither does
    any wheel event.
  * **Momentum.** GTK leaves the kinetic motion to the widget, so the shell synthesizes
    it from `decelerate`'s velocity (units per second, `scroll_history_finish`). The
    decay is `momentum`, pure Go: `v0·e^(−4t)`, the deceleration friction of
    GtkScrolledWindow. It stops at 20 pt/s, and each axis is capped at 15000 pt/s.
    `Shell.stepMomentum` steps it in the frame clock's paint pass, before the draws are
    queued, as `Momentum` Began/Changed/Ended with Phase None. A momentum that stops
    within its first frame still begins. A new `scroll-begin`, a `scroll` or a pointer
    press stops it with a zero-delta `Momentum` Ended. Its times go through
    `Shell.events`, so they never go backwards.
  * **Plumbing.** One new trampoline, `gboolean (gpointer, gdouble, gdouble, gpointer)`
    (`sigBoolDoubleDouble`, `boolDoubleDoublePeer`): `numTrampolines` 13 → 14. Also four
    `kindScroll*` kinds, `window.scrollUnit`, the bindings `gtk_event_controller_scroll_new`
    and `_get_unit`, and the `GtkEventControllerScrollFlags`.
* **Review:** an independent agent read the code against GTK 4.10–4.16 from memory. The
  signatures, flags, units, signs, the begin → scroll → end → decelerate order and the
  per-second velocity check out. The stop event sends no `scroll`, so it causes no
  spurious phase. Returning TRUE does not suppress kinetic scrolling. Momentum in the
  paint pass is safe: `painting` is nil there and the surfaces are copied. It found two
  defects, both fixed: Shift+wheel was not horizontal, and the stop and momentum times
  bypassed the never-backwards clamp. The test agent found a third, also fixed: a
  momentum that ended on its first frame sent no `PhaseBegan`.
* **Open, for the Linux log (from the review):**
  * On X11, GDK's XI2 path probably reports touchpads as WHEEL with no stop event. That
    would mean fractional lines, no phases and no momentum: acceptable. If some backend
    gives SURFACE without a `scroll-end`, the gesture never ends and the router stays
    latched. A gap timeout (PhaseEnded after about 150 ms) would guard against that.
  * Wayland hi-res wheels emit `scroll-begin` without an end. The code copes.
  * Resting fingers do not stop the momentum (no event comes).
  * GTK's own wheel step is page^(2/3) px, a bit more than 3 lines.
* **Finding (Win32):** Win32 does not turn Shift+wheel horizontal either, and aqua does
  not. Windows users expect it, so this is noted for a later Win32 touch-up. It is
  outside S4.1j.
* **Tests:** a test agent wrote 16 host tests (`scroll_test.go`), all through the real
  trampolines and frames over the fakes: `TestMomentumNew`, `TestMomentumAt`,
  `TestScrollWheel`, `TestScrollWheelShift`, `TestScrollPrecise`,
  `TestScrollPosModsTime`, `TestScrollMomentum`, `TestScrollMomentumShort`,
  `TestScrollMomentumNotStarted`, `TestScrollMomentumInterrupted`,
  `TestScrollMomentumTimes`, `TestScrollMomentumPhases`, `TestScrollMomentumClose`,
  `TestScrollDuringCreate`, `TestScrollAfterClose` and `TestScrollPanic`. The fake clock
  loop cannot close a surface inside the tick of the widget that paces it, so
  `TestScrollMomentumClose` uses a hidden surface (fallback timeout); this is a limit of
  the fake. The Linux test is `TestSurfaceGTKScroll`: the controller, its four handlers,
  signals through `g_signal_emitv`, and `SCROLL-RESULT:` for the unit.
* **Docs and checks:** ARCHITECTURE.md §0, §8.4, §8.5 (surfaces, the new Scroll input
  bullet, trampolines) and §10; `examples/README.md`; the package doc. `go.mod` is
  unchanged; no public API change. The full `check.sh`, stress run included, passes on
  the host.
* **Not run (no machine):** `TestSurfaceGTKScroll`, the two new bindings in `TestLoad`,
  and the checks by hand. On a Linux desktop (Wayland and X11), open `examples/notes`
  and check:
  * wheel steps;
  * Shift+wheel scrolls sideways;
  * the touchpad scrolls 1:1, then glides with momentum, and rubber-bands at the edges;
  * a click or a new touch stops the glide;
  * nested scroll views keep the latched view during the glide.
  In the log, look at `SCROLL-RESULT:`.

**S4.1k Keyboard input** ✅ *2026-11-02, built only* (the step text as planned):
* `GtkEventControllerKey` (`key-pressed`/`key-released`, with the consumed return
  value). A keyval → `driver.Key` table, pure Go and host-tested; character keys are the
  unshifted keyval of the active layout (`gdk_display_translate_key` at level 0).
  Text comes from a `GtkIMContextSimple` commit (full IME in S5.3), with control
  characters dropped. A consumed key-down drops its text.
* Done when: `check.sh` passes.

* **Result:** `shell/gtk4/keyboard.go` (untagged), with the native side in
  `window_linux.go`.
  * **Key events.** A `GtkEventControllerKey` sits on the **window**, not the GGSurface,
    in the CAPTURE phase, so it sees every key with no focusable widget and before GTK's
    own bindings. `key-pressed` is a `KeyEvent{Down}`, and its gboolean is the handler's
    result. `key-released` is a `KeyEvent{Up}`. `Repeat` is a key-down of a keycode
    already held, since GTK 4 has no repeat flag; the held set is cleared when the
    surface stops being the key window.
  * **Key table.** `keyFor` is pure Go. Named keys come from a keyval table (`keyKeys`,
    with the keypad, `ISO_Left_Tab` and `KP_Enter`), along with F1–F12 and the keypad
    digits. These come from the event's keyval, or from the level-0 base when the
    modifiers made it another special keyval (Ctrl+Alt+F1). Character keys use the base:
    the keyval at level 0 of the keycode in the event's layout group
    (`gdk_display_translate_key`), through `gdk_keyval_to_unicode` and `NormalizeKey`.
    Dead keys map to their characters. Modifiers, F13 and up, and Insert give `KeyNone`.
  * **Mods.** GTK's state is the one before the key, so a modifier key adds its own
    modifier on its press and drops it on release, as Win32 reports it. AltGr is no
    modifier here (Win32 reports Control+Option).
  * **Text.** A `GtkIMContextSimple` handles text. It is deliberately *not* attached to
    the controller: `set_im_context` would let it take keys before `key-pressed`, and a
    consumed key could then still produce text. An unconsumed key-down goes to
    `gtk_im_context_filter_keypress` with the current event. Its synchronous `commit` is
    a `TextEvent` after the `KeyEvent`, with control characters dropped and invalid
    UTF-8 replaced (`commitText`). It carries the key's time, or now outside a key
    event. A consumed key-down resets the input method instead, so it drops its text.
    Key-ups go to the input method too. Focus changes focus the input method in and out
    (`keyFocus`, before `KeyChanged`).
  * **Plumbing.** Two new trampolines with guint arguments, `sigBoolUintUintUint` and
    `sigUintUintUint`: `numTrampolines` 14 → 16. Three kinds, six `window` methods, 13
    bindings (`gtk_event_controller_key_new`, `_set_propagation_phase`,
    `_get_current_event`, `gtk_im_context_simple_new`, `gtk_im_context_set_client_widget`,
    `_filter_keypress`, `_reset`, `_focus_in`, `_focus_out`, `gtk_widget_get_display`,
    `gdk_key_event_get_layout`, `gdk_display_translate_key`, `gdk_keyval_to_unicode`), and
    `GtkPropagationPhase`. The binding-name test gained "im" as an initialism.
* **Review:** an independent agent read the code against GTK 4.10–4.16 from memory. The
  signatures, the synchronous commit (and none for Ctrl/Alt+letter), dead keys, Ctrl+Shift+U,
  Repeat on X11 and Wayland, the keypad, Shift+Tab and the destroy order check out. It
  found three defects, all fixed:
  1. Modifier keys had Win32-inconsistent Mods.
  2. A surface closed from a commit handler could unref the input method while it was
     filtering; `imFilter` now holds a reference.
  3. Named keys were found only on the modified keyval.
  The test agent's note that later commits used a stale key time is fixed too.
* **Open, for the Linux log:**
  * The order of our CAPTURE controller against GTK's own: a GtkApplication accels
    controller would come first, but there is none.
  * Whether an unhandled Tab rings the bell.
  * On a Russian layout, whether Ctrl+C gives 'С' as on Win32, which uses the layout
    character too.
  * Whether `imReset` ever commits a pending dead key.
  * Missing dead keyvals (`dead_belowdot` and others) give `KeyNone`.
  * With both Shift keys held, releasing one reports no Shift.
* **Tests:** a test agent wrote 14 host tests (`keyboard_test.go`), all through the real
  trampolines over the fake window and a fake input method: `TestKeyFor`,
  `TestKeyForUnicode`, `TestTextCommitText`, `TestKeyDownText`, `TestKeyModifierKeys`,
  `TestKeyConsumed`, `TestKeyRepeat`, `TestKeyFocus`, `TestTextCommit`,
  `TestKeyEventTimes`, `TestKeyCloseInHandler`, `TestKeyDuringCreate`,
  `TestKeyAfterClose` and `TestKeyPanic`. The Linux test is `TestSurfaceGTKKeyboard`: the
  controller, the input method, their handlers, the GDK keyval mapping, and
  `KEY-RESULT:` for a dead key's Unicode.
* **Docs and checks:** ARCHITECTURE.md §0, §8.4, §8.5 (surfaces, the new Keyboard input
  bullet, trampolines) and §10; `examples/README.md`; the package doc. `go.mod` is
  unchanged; no public API change. The full `check.sh`, stress run included, passes on
  the host.
* **Not run (no machine):** `TestSurfaceGTKKeyboard`, the 13 new bindings in `TestLoad`,
  and the checks by hand. On a Linux desktop (Wayland and X11), open `examples/form` and
  `controls` and check:
  * typing, including dead keys (US-intl or German) and compose;
  * Tab and Shift+Tab focus;
  * Space on a focused button;
  * Ctrl+A/C/V shortcuts, with no stray text;
  * held keys repeat;
  * Alt+F4 closes the window through the window manager.
  In the log, look at `KEY-RESULT:`.

**S4.1l Wrap-up** ✅ *2026-11-02, built only* (the step text as planned):
* Delete `spike/gtk`. `native.sh` on Linux runs the `gtk4` tests under xvfb and headless
  weston instead of the spike, and runs `examples/hello` for a few seconds on each
  backend. Update ARCHITECTURE.md, §6.3 and `docs/spikes.md` ("S4.1 inherits" →
  done).
* Done when: `check.sh` passes; the user's Linux log (both backends) passes, and the user
  has looked at `hello` on Wayland and X11. **Waits on a Linux machine.**

* **Result:**
  * **`spike/gtk` deleted** (ten files). Before that, its checks were compared with the
    `gtk4` linux tests. Class, slots, measure/allocate, snapshot, signals, idle sources,
    worker wake, frames, resize, close-request and the readback all had ports. Only the
    display-type check against `GDK_BACKEND` and the renderer's info line were missing.
    They are now `TestShellGTKBackend` (`backend_linux_test.go`): with `GDK_BACKEND=x11`
    or `wayland` the display has to be `GdkX11Display` or `GdkWaylandDisplay`, so the
    weston run can't silently test X11, and a shown window needs a GSK renderer.
    Display, renderer, GTK version and scale are logged as `BACKEND-RESULT:`.
  * **`native.sh` on Linux** builds `examples/hello` once. Under xvfb (X11) and under
    headless weston (Wayland), and again on the desktop session when `WAYLAND_DISPLAY` or
    `DISPLAY` is set, it runs `go test -count=1 -v ./internal/shell/gtk4/...` and
    `hello` for 5 s with `GOGUI_DEBUG=1`. `hello` passes only if `timeout` still had to
    stop it (status 124); an earlier exit with any status fails. `weston_selftest` became
    `weston_run`, which runs any command on a private headless weston. A missing xvfb or
    weston is recorded as one failed step for that backend.
  * **Docs:** PLAN.md §6.3 rewritten for the implementation (sonames, `ffi.Bind`
    tables, the 4.10 floor with runtime lookup of newer functions, the "GTK missing"
    error with zenity/kdialog, 16 trampolines by signature, the handle table, panics,
    GGSurface with the class-size check, `GMainLoop` without `GtkApplication`, CSD
    windows, the frame clock, struct mirrors). Also PLAN.md §8.4 and S0.10c_2b/S0.10c_3:
    the Linux answers and the desktop check now come from the `gtk4` tests and `hello`.
    `docs/spikes.md`: S0.9 is marked deleted, its open points point to the tests that
    answer them, "S4.1 inherits" became "S4.1 inherited (done)" with where each piece
    lives, and open runs 4 and 5 were rewritten. ARCHITECTURE.md: §0 status, the
    `shell/gtk4` package row (it was missing the S4.1h–k files and still said "no input
    yet"), the native-code sentence (gtk4 was missing), Tooling (`native.sh`), Spikes,
    §8.4, §8.5 and §10. No public API change. `go.mod` is unchanged: purego is still
    imported by `internal/ffi`.
  * **Checks:** the full `check.sh` passes on the host, stress run included.
    `timeout` can't be executed on this host, so the `hello_for` exit-status logic
    (124 = pass) is checked only by reading it; `bash -n` passes.
  * **Not run (no machine):** `native.sh` on linux/amd64 and linux/arm64: every `gtk4`
    linux test of S4.1b–S4.1l under xvfb and headless weston, and `hello` for 5 s on
    both. **Phase 4's S4.1 "Done when" waits on that log** and on the user's look at
    `go run ./examples/hello` on a Wayland and an X11 desktop: the window opens with the
    traffic lights and placeholder boxes, hover and clicks react, the title bar moves
    the window and a double-click zooms it, resizing keeps the content filling the window
    without flicker, typing works, and closing quits. Say whether X11 was a real session
    or XWayland. In the log, look at `BACKEND-RESULT:` and the other `*-RESULT:` lines.

---

## I. Phase 4: S4.2 (S4.2a–S4.2k) (from PLAN.md §11.1)

**S4.2a GTK 4.14 floor** ✅ *2026-11-03, built only* (the step text as planned):
* `Load` refuses GTK < 4.14 (`VersionError`), with the matching Pango minimum (what GTK
  4.14 requires). `gdk_surface_get_scale` becomes a required binding, so the integer
  `notify::scale-factor` path for 4.10/4.11 goes (the scale is always the GdkSurface's).
  D7 in §12 and every place that names 4.10 (header table, §3, §6.3, §9, S4.1's design,
  ARCHITECTURE.md) are updated.
* Done when: host tests of the version check at 4.13.x (refused) and 4.14.0 (accepted);
  the binding-table test with no optional 4.12 entry left; `check.sh` passes.

* **Result:**
  * **Floor:** `gtk.MinGTKMinor` = 14, `MinPangoMinor` = 52 (GTK 4.14's `meson.build`
    asks for `pango >= 1.52.0`; taken from memory, because gitlab.gnome.org and GitHub
    can't be reached from the host; worth a glance on the Linux machine). The error
    messages name "GTK 4.14" and "Pango 1.52".
  * **Scale:** `gdk_surface_get_scale` moved from the optional list to the required
    bindings; `gtk_widget_get_scale_factor` and the GGSurface's `notify::scale-factor`
    connection are gone. `gtkWindow.scale` is the GdkSurface's scale (1 without one, or
    for a non-positive value); `notify::scale` of the GdkSurface is always connected, so
    a window still has four signal handlers (close-request, is-active, state, scale).
  * The `optional` mechanism of the binding tables stays, now empty, for functions newer
    than the floor (S4.2j's `pango_font_map_add_font_file`, Pango 1.56).
  * **Tests (host):** `TestCheckGTK` accepts 4.14.0, 4.14.5, 4.18.2 and refuses 4.13.0,
    4.13.99, 4.12.4, 4.10.0 and older; `TestCheckPango` accepts 1.52.0 to 1.56.0 and
    refuses 1.51.99, 1.50.6 and older; new `TestFloorBindings`: no optional binding
    left, `gdk_surface_get_scale` required, `gtk_widget_get_scale_factor` not bound. The
    linux `TestSurfaceCreate` expects 2 + 2 handlers without the optional branch.
  * **Docs:** PLAN.md header table, §3, §6.3, §9 table, S4.1's design (Scale) and open
    point, D7 (amended: Fedora 40+, Debian 13; Fedora 39 with GTK 4.12 dropped);
    ARCHITECTURE.md §0 status and the `gtk`/`gtk4` sections; `examples/README.md`.
    `docs/spikes.md` keeps "4.10" (it records the spike as it was). No public API
    change.
  * **Checks:** the full `check.sh` passes on the host, stress run included.
  * **Not run (no machine):** `native.sh` on linux/amd64 and linux/arm64: `TestLoad`
    against a real GTK ≥ 4.14 (and that the distro's Pango is ≥ 1.52), and the surface
    tests with the GdkSurface scale on both backends.

**S4.2b The canvas leaves interim** ✅ *2026-11-03, built only* (the step text as planned):
* `interim_canvas*.go` become `canvas*.go` (the tests too); the placeholder boxes stay
  only for what later sub-steps replace. Gradient strokes of rounded rects become real:
  `push_stroke` of a rounded-rect `GskPath` with a linear-gradient node inside (in
  place of the border node in the gradient's middle color). Binds `gsk_path_builder_*`
  (new, add_rounded_rect, free_to_path), `gsk_path_unref`, `gsk_stroke_new/free`,
  `gtk_snapshot_push_stroke`.
* Done when: host tests of the gradient stroke through the fake snapshotter (nesting
  checked); a linux node-tree test (stroke node, gradient child); the S4.1g tests pass
  under their new names; `check.sh` passes.

* **Result:**
  * **Renames:** `interim_canvas.go`, `interim_canvas_linux.go` and their tests are
    `canvas.go`, `canvas_linux.go`, `canvas_test.go`, `canvas_linux_test.go`; the host
    tests are `TestCanvas*` (helpers `cv*`), the comments in `doc.go`, `surface.go` and
    the tests no longer say "interim canvas". `interim_text.go` stays until S4.2g. The
    placeholder boxes (paths, shadows, text, images, backdrop blur) stay, each comment
    naming the sub-step that replaces it.
  * **Gradient strokes:** `snapshotter.pushStroke(outline, width)`: `gtkSnapshot` builds
    the rounded rect's `GskPath` (`gsk_path_builder_add_rect` when every corner is sharp,
    so the corners are miter joins for certain, else `add_rounded_rect`), a
    `gsk_stroke_new(width)` (butt caps, miter joins, limit 4) and calls
    `gtk_snapshot_push_stroke`, which refs the path and copies the stroke, so both are
    dropped at once. `StrokeRoundedRect` with a linear gradient is `pushStroke` of the
    rect with its clamped radii, the gradient node over the rect grown by half the width
    (a 90° miter ends exactly on that corner), and `pop`, so it nests in any open state
    and never stays open. Solid strokes stay border nodes.
  * **Paints:** `filler`'s paint resolution moved into `paintArgs` (shared by fills and
    strokes), so a degenerate gradient strokes as fills do: one stop is that stop's
    color, a zero-length line the last stop's (CSS), a border node in that color. Before
    S4.2b a gradient stroke was always the middle color; the middle color is kept only for
    a rect without area (GSK would stroke that path without the border's square ends).
  * **Bindings:** `Gsk.PathBuilderNew`, `PathBuilderAddRect`, `PathBuilderAddRoundedRect`,
    `PathBuilderFreeToPath`, `PathUnref`, `StrokeNew`, `StrokeFree`;
    `Gtk.SnapshotPushStroke`; `gtk.NodeStroke` = 29 (4.14). `add_rect` is one more than the
    step listed (from the review below).
  * **Tests (host):** `TestCanvasStrokeGradient` (22 cases: sharp, rounded, clamped and
    per-corner radii, hairline, unsorted and three stops, inside save + clip + opacity,
    left open and closed by `finish`, one stop, zero-length line, empty rect, transparent
    middle, all stops transparent, semantic stop, NaN/overflowing points, widths, NaN
    rect); the old "gradient: the middle color" case of `TestCanvasStroke` went;
    `TestStrokeBindings` in `gtk`. All S4.1g tests pass under the new names.
  * **Tests (linux, not run, no machine):** `TestCanvasGTKGradientStroke`: two stroke
    nodes (one under an opacity node), each with the line width read back through
    `gsk_stroke_node_get_stroke` + `gsk_stroke_get_line_width` and the rect as its path
    bounds (`gsk_path_get_bounds`), its own bounds within the outer rect, holding one
    linear-gradient node over the outer rect with the points and sorted stops, and no
    border node. `cnBuild` descends into stroke nodes, the dump prints width and path
    bounds. `TestCanvasGTKLinearGradient` no longer draws a gradient stroke.
  * **Review** (a second agent, against GTK 4.14 from memory, as the host cannot reach
    gitlab.gnome.org): signatures, ownership and `GSK_STROKE_NODE` = 29 confirmed; no
    blocking finding. Two follow-ups applied: `add_rect` for sharp rects (in case
    `add_rounded_rect` with zero radii joins those corners differently) and a stale
    comment. For later tests: GTK turns a gradient whose stops are all one color into a
    color node.
  * **Checks:** the full `check.sh` passes on the host, stress run included.
  * **Not run (no machine):** `TestCanvasGTKGradientStroke` and the renamed
    `TestCanvasGTK*` through `native.sh` on both backends; a look at gradient strokes
    with mixed sharp and rounded corners (whether the sharp ones are mitered).


**S4.2c Paths** ✅ *2026-11-03, built only* (the step text as planned):
* `FillPath`/`StrokePath` over `GskPath`: a pure-Go conversion of `render.Path` (move,
  line, quad, cubic, close; open subpaths closed implicitly for fills) into builder calls
  behind the `snapshotter`; nonzero fill rule; `GskStroke` with width, caps, joins, miter
  limit, dashes and dash offset from `StrokeStyle.Normalized`; solid paints by
  `append_fill`/`append_stroke`, gradients by `push_fill`/`push_stroke` around a gradient
  node over the path's bounds. Binds the rest of the builder, `gsk_stroke_set_*`,
  `gtk_snapshot_append_fill/append_stroke/push_fill`.
* Done when: host tests of the conversion (every op, empty and nil paths, implicit close,
  non-finite points ignored) and of the stroke parameters; linux node-tree tests (fill
  and stroke nodes with their path's bounds, stroke parameters read back with
  `gsk_stroke_node_get_stroke`); `check.sh` passes.

* **Result:**
  * **Conversion (pure, `canvas.go`):** `pathCmds(path, fill)` turns a `render.Path` into
    `pathCmd`s (the op and its points narrowed to float32). A subpath with nothing but
    its move (and maybe a close) is dropped (no segment; whether caps make a dot of it
    differs between backends); a fill closes every open subpath with an explicit close, a
    stroke keeps open ones open; a point that is not finite as a float32 drops the whole
    path (as Win32's `pathGeometryMode`). `strokeArgs` makes a `pathStroke` from the width
    and `StrokeStyle.Normalized`: `GskLineCap`/`GskLineJoin` (same order as render), the
    miter limit (one that overflows float32 is `DefaultMiterLimit`, as on Win32), the
    dashes and offset in user-space lengths (GSK's unit; Win32 divides by the width for
    D2D); a dash length or offset that doesn't narrow drops the stroke. `pathBox`,
    `paintColor` and `outlinePaint` went with the placeholder.
  * **Drawing:** a solid fill is `appendFill` (`gtk_snapshot_append_fill`,
    `GSK_FILL_RULE_WINDING`), a solid stroke `appendStroke`; a gradient is
    `pushFill`/`pushPathStroke` (`push_fill`/`push_stroke`) around the linear-gradient
    node, then `pop`, so it nests in any open state. The gradient covers
    `gradientCover`: the path's bounds (tight for curves), for strokes grown by
    `strokeReach` (w/2 × the largest of 1, √2 for square caps, the miter limit for miter
    joins), and by one device pixel more for the antialiased edge and GSK's offset-curve
    approximation, its coordinates clamped to ±1e30 so a huge reach still narrows.
    Paints resolve through `paintArgs` as for rounded rects (one stop, a zero-length
    line). A fill whose bounds have no area draws nothing. `gtkSnapshot` builds the
    `GskPath` and `GskStroke` per call and drops them after it (GTK keeps a reference
    and a copy).
  * **Bindings:** `Gsk.PathBuilderMoveTo/LineTo/QuadTo/CubicTo/Close`,
    `StrokeSetLineCap/SetLineJoin/SetMiterLimit/SetDash/SetDashOffset`;
    `Gtk.SnapshotAppendFill`, `SnapshotPushFill`, `SnapshotAppendStroke`; constants
    `gtk.LineCap*`, `LineJoin*`, `FillRule*`, `NodeFill` = 28.
  * **Tests (host):** `TestCanvasPathCmds` (22 paths × fill and stroke: every op, nil,
    empty, lone moves, implicit close, drawing after a close, moves without a current
    point, a zero-length line kept, Inf/NaN/float32-overflowing points);
    `TestCanvasStrokeArgs` (caps, joins, miter limits incl. below 1 and overflowing,
    dashes odd, negative, NaN and all zero, offsets, ignored widths and lengths, and
    `strokeReach` for each); `TestCanvasFillPath` and `TestCanvasStrokePath` (solid and
    gradient, curves' tight bounds, the cover at scale 2, nested in save + clip +
    opacity and closed by `finish`, degenerate paints, a clamped huge reach, every
    ignored case); `TestPathBindings` in `gtk`. The path cases left
    `TestCanvasPlaceholders`; `TestCanvasHairline` outlines a shadow in place of a path.
  * **Tests (linux, not run, no machine; written by a second agent):**
    `TestCanvasGTKFillPath` (fill nodes found by their path bounds, winding rule, closed
    contours when the optional test binding `gsk_path_is_closed` resolves, a color child
    in the paint's color or the gradient over the cover, one under an opacity node, a
    cubic's GSK bounds between its tight bounds and its control-point hull, logged) and
    `TestCanvasGTKStrokePath` (six strokes: width, cap, join, miter limit, dashes and
    offset read back through `gsk_stroke_node_get_stroke` and `gsk_stroke_get_*`, an
    odd dash pattern doubled, open paths open, the gradient over the reach plus a pixel).
    `cnBuild` descends into fill nodes; the dump prints fill rules and stroke styles.
  * **Review** (a third agent, against GTK 4.14 from memory, as the host cannot reach
    gitlab.gnome.org): signatures, enum values, ownership and FFI use confirmed; no
    blocking finding. Applied: the miter-limit fallback (it dropped the stroke, unlike
    Win32), the clamped cover (a huge but finite reach dropped gradient strokes) and the
    one-pixel pad. Open, for the Linux machine: whether GSK draws a dot for a zero-length
    line or dash with round or square caps (D2D does), and whether GSK compares the
    miter limit as cairo does (tip at ≤ limit × w/2); both need pixels, not node trees.
  * **Checks:** the full `check.sh` passes on the host, stress run included.
  * **Not run (no machine):** `TestCanvasGTKFillPath` and `TestCanvasGTKStrokePath`
    through `native.sh` on both backends, and `TestLoad` resolving the new functions in
    a real GTK 4.14; a look at stroked and filled paths (checkmarks, curves, dashes).

**S4.2d Shadows** ✅ *2026-11-03, built only* (the step text as planned):
* `DrawShadow` with `gtk_snapshot_append_outset_shadow` / `append_inset_shadow` from
  `Shadow.Normalized` (offset, blur, spread, color; GSK's blur radius is the CSS one, so
  σ = blur/2 as in render). Binds both.
* Done when: host tests of the arguments (outer, inset, zero blur, negative spread,
  ignored values); a linux node-tree test (outset/inset shadow nodes with their outline,
  offset, spread and blur); `check.sh` passes.


* **Result:**
  * **Canvas (pure, `canvas.go`):** `shadowArgs(r, radii, Shadow.Normalized)` makes a
    `shadowCall` (outline with radii clamped to r, color, dx, dy, spread, blur, inset);
    `DrawShadow` hands it to the new `snapshotter.appendShadow`, one node that nests in
    any open state and opens none. GSK's shadow nodes have CSS box-shadow semantics as
    `render.Shadow` does (outset only outside the outline, inset only inside it; the
    spread grows each rounded radius, sharp corners stay sharp; the blur radius is the
    CSS one, σ = blur/2), so the normalized values pass through unchanged. Ignored, as
    on Win32: semantic or transparent colors, a shape without area or not finite, an
    outer shape shrunk away by a negative spread, and an offset, spread or blur beyond
    ±1e6 pt (`maxShadow`; from the review: GSK adds them up in float32, so finite but huge
    values could make Inf node bounds, where Win32 drops masks past 16384 px). An inset
    shadow whose hole is shrunk away covers the shape (CSS). `shadowBox` and the
    shadow placeholder went.
  * **Bindings:** `Gtk.SnapshotAppendOutsetShadow`, `SnapshotAppendInsetShadow` (both
    GTK 4.0, required; three pointers then four float32s); constants
    `gtk.NodeInsetShadow` = 11, `NodeOutsetShadow` = 12.
  * **Tests (host):** `TestCanvasShadowArgs` (32 cases: outer, inset, zero, negative and
    NaN blur, negative spread up to and past nothing, an inset hole shrunk away, inset
    negative spread, clamped and per-corner radii, NaN/Inf normalized to 0, translucent,
    transparent and semantic colors, empty, negative, non-finite and float32-overflowing
    rects and values, values past `maxShadow` and at it); `TestCanvasDrawShadow` (outset
    and inset nodes, nested in save + translate + clip + opacity, left open and closed by
    `finish`, ignored cases); `TestShadowBindings` in `gtk`. The shadow cases left
    `TestCanvasPlaceholders`; `TestCanvasHairline` outlines a text box in place of a
    shadow.
  * **Tests (linux, not run, no machine; written by a second agent):**
    `TestCanvasGTKShadow`: seven shadows (rounded per-corner outset, inset, zero blur,
    negative spread, an inset hole shrunk away, one translated under an opacity, one
    translated inset with clamped radii), each exactly one node of its type with its
    outline and radii, color, dx, dy, spread and blur read back through
    `gsk_{outset,inset}_shadow_node_get_*` (twelve test-only getters) in root
    coordinates (through any transform node above); inset bounds equal the outline,
    outset bounds contain the moved and grown shape; four ignored shadows make no node,
    and no border node is left. `TestCanvasGTKPlaceholders` keeps text and the backdrop
    blur. The dump prints shadow nodes.
  * **Review** (a third agent, against GTK 4.14 from memory: docs.gtk.org is blocked from
    the host as well): signatures, argument order, enum values, ownership (outline and
    color copied), purego float32 passing and the semantics (blur radius, clipping,
    spread on radii; radii cannot overlap after spread since they are clamped first)
    confirmed; no blocking finding. Applied: the `maxShadow` bound. Noted: neither GSK
    nor Win32 uses CSS-B3's spread-radius curve for radii below the spread (they
    agree); GSK's GPU box shadow approximates per-corner radii, Win32 blurs a true mask,
    so small visual differences are expected.
  * **Checks:** the full `check.sh` passes on the host, stress run included.
  * **Not run (no machine):** `TestCanvasGTKShadow` through `native.sh` on both backends
    (also confirms purego's float registers on amd64 and arm64), `TestLoad` resolving the
    two new functions; a look at shadows (window and control shadows, a zero-blur shadow
    with an offset, an inset with the hole shrunk away) under GTK 4.14's default renderer.

**S4.2e Images** ✅ *2026-11-03, built only* (the step text as planned):
* As S2.4e_4: an unexported `render.Image` with straight-alpha RGBA pixels copied at
  creation, a pixel size and a scale; its `GdkTexture` (`g_bytes_new` +
  `gdk_memory_texture_new`, `GDK_MEMORY_R8G8B8A8`) made on first draw and kept, released
  with the shell (no finalizers). `DrawImage` is `gtk_snapshot_append_scaled_texture`
  (linear) into `dst`; foreign images, nil and empty `dst` draw nothing.
* Done when: host tests of validation, `Size` and the ignored cases; linux tests (one
  texture node of the right bounds and filter, the texture made once over frames and
  released with the last surface); `check.sh` passes.

* **Result:**
  * **Images (pure, new `image.go`):** `shellImage` (straight-alpha RGBA copied, `w`,
    `h`, `scale`; `Size` = pixels / scale, zero for a typed nil so the recorder's dump
    can't panic) belongs to an `imageSet` (`Shell.images`; the canvas's new `images`
    field, set by `surface.snapshot`). `newImage` rejects empty sizes, sides past
    `MaxInt32/4` (`maxImageSide`), a scale that is not positive and finite, and a wrong
    pixel count. `imageSet.texture` makes the texture through the new
    `snapshotter.newTexture` on first draw and keeps it; a texture GDK doesn't make draws
    nothing and isn't retried until the next release (Win32 counts it as a frame failure;
    `gdk_memory_texture_new` only returns NULL on a failed precondition, which
    `newTexture` checks first). `imageSet.release` unrefs every texture and clears the
    failures; it runs when the last surface leaves (`removeSurface`) and in `shutdown`,
    and the next draw makes a new one. `DrawImage` is `snapshotter.appendTexture` into
    `dst`; images of another set, other `render.Image`s, nil (typed too), an empty,
    negative or non-finite `dst` (also after narrowing), a canvas without a set and calls
    after `finish` draw nothing. The image placeholder went (`placeholderColor` stays for
    the blur until S4.2f).
  * **Native (`canvas_linux.go`):** `gdkTexture` (release = `g_object_unref`);
    `newTexture` = `g_bytes_new` (copies the Go pixels during the call, no Go pointer
    kept) + `gdk_memory_texture_new(w, h, GDK_MEMORY_R8G8B8A8, bytes, w·4)` + the
    GBytes' own reference dropped; `appendTexture` =
    `gtk_snapshot_append_scaled_texture(…, GSK_SCALING_FILTER_LINEAR, &r)` (the node keeps
    its own texture reference, so a release while a node lives is safe).
  * **Bindings:** `Gtk.SnapshotAppendScaledTexture` (4.10), `Gdk.MemoryTextureNew`,
    `GLib.BytesNew`, `GLib.BytesUnref` (all required); constants `gtk.NodeTexture` = 10,
    `NodeTextureScale` = 26, `ScalingFilterLinear/Nearest/Trilinear` = 0/1/2,
    `MemoryR8G8B8A8` = 5.
  * **Tests (host, written by a second agent):** `TestImageNewInvalid` (21 cases, the
    error of each check), `TestImageNew`, `TestImageSize`, `TestImageDraw` (ops, the
    pixels passed, nested in save/translate/scale/clip/opacity), `TestImageTextureOnce`,
    `TestImageIgnored` (26 cases, after `finish` too), `TestImageTextureFails`,
    `TestImageRelease`, `TestImageShellSurfaces` (kept while a surface is open, released
    with the last, made again for a new one), `TestImageShellQuit`,
    `TestImageShellRunEnd` (normal and panicking Run); `TestImageBindings` in `gtk`.
    `TestCanvasPlaceholders` and `TestCanvasHairline` lost their image placeholder.
    Mutation-checked: 17 mutations, all caught.
  * **Tests (linux, not run, no machine; written by a third agent):** `TestImageGTKNode`
    (one texture or texture-scale node per draw at `dst` in root coordinates, also under
    translate + scale, the linear filter when it is a texture-scale node, the image's
    texture, 2×2, its download as premultiplied BGRA; the ignored cases make no node; one
    under its clip and a 0.5 opacity), `TestImageGTKOnceOverFrames` (one texture over
    three paints, a new one after release), `TestImageGTKSurfaceRelease` (a real surface:
    one texture over frames, kept when another surface closes, released with the last,
    made again for a new surface, released at the end of Run; the GObject ref count
    logged). Three test-only GSK getters (`gsk_texture_node_get_texture`,
    `gsk_texture_scale_node_get_texture`, `_get_filter`).
  * **Review** (a fourth agent, GTK 4.14 from memory; docs.gtk.org blocked): signatures,
    enum values, ownership and the purego pixel pass confirmed; applied: the nil-safe
    `Size`. **Open points** (not changed, the plan text holds): (1) the two shells take
    different pixel formats (Win32 premultiplied BGRA, GTK straight RGBA); the image
    producer that comes with decoding and icons must pick one contract (premultiplied
    RGBA would match Go's `image.RGBA`, `GDK_MEMORY_R8G8B8A8_PREMULTIPLIED` = 2).
    (2) Images stay held until the last surface closes (no per-image release yet, as
    Win32's `release`), to revisit with the public image API. (3) Very large images
    (past the GPU's texture limit) under GTK 4.14's new renderers are unchecked.
  * **Checks:** the full `check.sh` passes on the host, stress run included.
  * **Not run (no machine):** the three `TestImageGTK*` through `native.sh` on both
    backends (which node GTK makes for the linear filter, the downloaded bytes, the ref
    count line), `TestLoad` resolving the four new functions.

**S4.2f Backdrop blur** ✅ *2026-11-03, built only* (the step text as planned):
* `PushBackdropBlur` by segments (design above): the canvas draws into a child snapshot,
  ends it at a blur, re-appends it blurred and clipped, and reopens its open pushes (each
  push records what it takes to push it again). The placeholder box and its
  `placeholderColor` go.
* Done when: host tests through the fake (the backdrop node appended twice, the reopened
  state equal to the one before, nested blurs, a blur inside `PushOpacity`, `finish`
  balanced); a linux node-tree test (blur node under the rounded clip holding the earlier
  content); `check.sh` passes.

* **Result:**
  * **Segments (`canvas.go`, new `blur.go`):** the snapshotter gets `begin`, `cut`, `end`,
    `appendNode` and `pushBlur`, and a `renderNode` type (release = unref). `newCanvas`
    begins the first segment, `finish` ends the last one into the paint's snapshot and
    releases the kept nodes. Every state frame now records its transforms and clips in
    order (`stateOp`), its opacity and the transform it was opened with; the canvas keeps
    the current transform from the paint's root (`ctm`; `Concat`'s translate/scale/matrix
    split is `transformOps`, shared). `PushBackdropBlur` unwinds every frame (innermost
    first), cuts (the node appended to the paint's snapshot and kept), rewinds the frames
    in the next segment, opens the blur frame with the shape's rounded clip and then, in
    a save, appends every node so far under the inverse transform, a blur push of the
    radius × √|det| (GSK's radius is render's: σ = radius/2) and a clip to the cover (the
    root bounds padded by 3σ, within the surface padded the same). So nested blurs and a
    blur after an earlier one read everything drawn so far, and an opacity group's content
    before a blur is composed on its own, as Win32's finding in S2.4e_3.
  * **Edge rule:** GSK reads transparency past a node, so where the cover leaves the
    surface the nodes are appended again mirrored at that edge (and at the corner between
    two), which for even content is Win32's hard border.
  * **Clip only, no cut** (as Win32): a zero, negative, NaN or infinite radius, a shape
    with no area or not finite, a singular transform, a shape outside the surface, a root
    radius past 1e6 pt (`maxBlur`) or narrowing to 0. **Deviation from the design:** its
    fallback "a fill in the blur's tint" has nothing to draw (`render.Canvas`'s blur has
    no tint; the caller's tint fill on top still draws) and nothing after the cut can fail
    (GLib aborts on OOM), so the fallback is the clip; noted in the design bullet.
  * **Native (`canvas_linux.go`):** `gtkSnapshot` is now a struct (`paint`, `cur`;
    `newGtkSnapshot`, used by `gtkWindow.snapshotter` and the linux tests): `begin` =
    `gtk_snapshot_new`; `cut`/`end` = `gtk_snapshot_free_to_node` of `cur` and
    `gtk_snapshot_append_node` into `paint` (`end` unrefs, `cut` hands its reference to the
    canvas as a `gskNode`); `appendNode` and `pushBlur` act on `cur`. Every paint now goes
    through one child snapshot, blur or not.
  * **Bindings:** `Gtk.SnapshotAppendNode`, `Gtk.SnapshotPushBlur` (both 4.0, required);
    `gtk.NodeBlur` = 23. `TestBlurBindings` in `gtk`.
  * **Tests (host, `blur_test.go`):** `TestCanvasBlur` (9 cases: the backdrop cut and
    appended twice; the reopened state through translate, save, clip, opacity and scale,
    with the inverse and the scaled radius; nested blurs; blur in opacity and opacity in
    blur; a rotation; a transform ended by `Restore`; nothing drawn before; two blurs in a
    row), `TestCanvasBlurEdges` (5), `TestCanvasBlurClipOnly` (13),
    `TestCanvasBlurEmptySurface`, `TestCanvasBlurFinish`. The fake checks that segments are
    cut and ended balanced, the last one ended and every node released once. The state
    tests in `canvas_test.go` keep the clip-only path (radius 0). Mutation-checked: no
    mirrors, no inverse, unscaled radius, a 2σ pad, only the last node, the transform not
    restored at a frame's close, the opacity lost on rewind, the cut after the clip; all
    caught.
  * **Tests (linux, not run, no machine; written by a second agent):** `TestCanvasGTKBackdropBlur`
    (at scale 1 and 2: one blur node of the radius under the rounded clip of the shape,
    radii exact, holding the earlier content, which also lies once outside it at the same
    root position; the fill inside the blur clipped and not blurred, the one after `Pop`
    neither; under translate + scale the radius × √|det| and the copy back at the
    original's root position; the left-edge mirror at negative x; nested blurs; a blur in
    `PushOpacity`; radius 0); `TestCanvasGTKPlaceholders` now covers text only. Test-only
    getters `gsk_blur_node_get_child` and `_get_radius`; the tree walk descends into blur
    nodes.
  * **Review** (the second agent, GSK from memory): ownership (`free_to_node` transfer
    full, `append_node` taking its own reference, one node several times in a tree),
    root-space consistency and the radius semantics confirmed; the original staying under
    its blurred copy and an opacity group's copy drawn at the group's opacity again match
    Win32. **Notes:** `gtk_snapshot_append_node` under ctm × inverse wraps each copy in a
    near-identity transform node (harmless); each blur re-appends every earlier segment
    (with up to three mirrors), so many or nested blurs multiply blur passes, to watch in
    heavy UIs.
  * **Checks:** the full `check.sh` passes on the host, stress run included.
  * **Not run (no machine):** `TestCanvasGTKBackdropBlur` and `TestCanvasGTKPlaceholders`
    through `native.sh` on both backends (the nesting GTK makes, logged as
    `CANVAS-RESULT`), `TestLoad` resolving the two new functions, and a look at a blurred
    sidebar or menu once the aqua controls use one.

**S4.2g Pango text system: layout and metrics** ✅ *2026-11-03, built only* (the step text as planned):
* Binds `libpangocairo-1.0.so.0` (default font map) and the Pango calls for contexts,
  font descriptions and layouts; reads `gtk-font-name` with `g_object_get_property`.
  `text.go` replaces `interim_text.go` (deleted): font resolution (design above), the
  byte stand-ins, `NewLayout` with the contract's wrap rules, `Text`, `Font`, `Size`,
  `Baseline`, `LineCount`. Hit-testing answers as for empty text until S4.2h.
* Done when: host tests of resolution, the stand-ins and the wrap-width mapping (through
  a fake layouter); linux tests for the size and wrap contract as S2.5d's (wrapping at a
  space, a long word breaking, `\n` lines, no-wrap values, empty text one line high),
  weight and italic changing the width, monospaced runes equally wide, an unknown family
  falling back; the aqua title bar and placeholders still lay out; `check.sh` passes.

* **Result:**
  * **Text system (pure, new `text.go`; `interim_text.go` deleted):** `Shell.text` is a
    `*textSystem` (`newTextSystem(q)`); `New()` sets `open = openPango`, which runs with the
    first layout (after Run loaded GTK). Pango sits behind `pangoLayouter` (`uiFamily`,
    `hasFamily`, `measure`, `release`). `NewLayout` measures once into a `textLayout` (size,
    first baseline, line count, mean line height) and keeps no native object. Rules:
    `resolve` (sizes absolute, ×1024; zero/negative/NaN = 13 pt, clamped to [0.1, 2048];
    weight clamped to [100, 1000]; italic); `wrapWidth` (-1 for 0, negative, NaN, ±Inf and
    widths past Pango's int; else `floor(maxWidth×1024 + 1e-3)`, at least 1, so wrapped
    text is never wider than maxWidth and a width measured before fits again);
    `pangoText` (each NUL and invalid byte → one `?`, offsets 1:1, valid text untouched).
    Fonts: a named family the font map has is kept (cached, 128 names), generic names
    (`sans-serif`, `monospace`, `serif`, `system-ui`, … in any case) count as present, any
    other is the UI family (gtk-font-name's family, else `sans-serif`; Inter in S4.2j);
    empty + `Monospaced` is `monospace`. Without Pango (`open` nil, the host tests), once
    opening fails (final), when measuring fails (logged once) or after `release` (in
    `Shell.shutdown`) layouts are `shell/fake`'s; text over 2 GiB too, without a log.
    Hit-testing answers as for empty text until S4.2h.
  * **Native (`text_linux.go`, `pangoNative`):** one `PangoContext` of the GTK thread's
    `pango_cairo_font_map_get_default()`, `round_glyph_positions` off and **cairo font
    options with `CAIRO_HINT_METRICS_OFF`** (the default counts as hinted and rounds
    advances to whole logical pixels; GTK's own contexts turn it off with
    gtk-hint-font-metrics' default). Layouts: a font description (family, absolute size,
    weight, style), **`auto_dir` off** (LTR base direction, as DirectWrite; otherwise an RTL
    paragraph under a width is right-aligned at x > 0, which `get_size` hides),
    `PANGO_WRAP_WORD_CHAR`, the width, the text; `get_size`, `get_baseline`,
    `get_line_count`, unref. `hasFamily`: `pango_font_map_get_family`, then a
    case-insensitive match over `pango_font_map_list_families` (listed once).
    `uiFamily`: gtk-font-name through a string GValue, parsed with
    `pango_font_description_from_string`.
  * **Bindings:** `Pango.*` (context, font map families, font description, layout; 21
    functions), `PangoCairo.FontMapGetDefault`, `ContextSetFontOptions` from the new
    `libpangocairo-1.0.so.0`, `Cairo.FontOptionsCreate/SetHintMetrics/Destroy` from the new
    `libcairo.so.2` (GTK links both), `GObject.ValueGetString`; constants `PangoScale`,
    `PangoStyle*`, `PangoWrap*`, `HintMetrics*`. Package hints for both libraries.
  * **Deviations from the design (review findings):** libcairo is loaded after all, for the
    font options only (the design's "no `libcairo`" was about a cairo path fallback, which
    there still isn't; design bullet amended); `auto_dir` off and the generic/case-insensitive
    family rules are additions.
  * **Tests (host, `text_test.go`, written by a second agent):** `TestTextResolveFamily`
    (17 cases, generic names included), `TestTextFamilyCache`, `TestTextResolveSize`,
    `TestTextResolveWeightItalic`, `TestTextPangoText` (stand-ins: NUL, lone, truncated,
    surrogate, overlong and out-of-range bytes; valid text unchanged and not copied),
    `TestTextWrapWidth` (no-wrap values, floor, the slack, the int boundary),
    `TestTextLayoutMetrics`, `TestTextLayoutNoLines`, `TestTextNoPango`,
    `TestTextOpenFails`, `TestTextMeasureFails`, `TestTextRelease`, `TestTextShell`,
    `TestTextShellRunEnd` (released at the end of Run, normal and panicking).
    Mutation-checked: 27 mutations, 25 caught, 2 equivalent.
  * **Tests (linux, not run, no machine; written by a third agent):** `TestTextPangoBasics`,
    `…Empty` (one line high, zero wide), `…Lines` (`\n`, trailing `\n`, `\n\n`, `\r\n`),
    `…Wrap` (at a space, a hanging space, a long word breaking, per paragraph),
    `…WrapFits` (W ≤ maxWidth; re-laid at its own width the same), `…NoWrapWidths`,
    `…Fractional` (unhinted widths not whole pixels), `…RTL` (no offset under a width),
    `…WeightItalic` (skips, listing the families, without distinct faces),
    `…Monospaced`, `…Families` (unknown falls back, generics, case-insensitive),
    `…Bytes`, `…Shell` (real layouts from `Shell.TextSystem` in Ready),
    `…Chrome` (the aqua title bar's and placeholders' layouts: Sequoia's title font,
    truncated titles growing with the prefix, `aqua.PlaceholderFont` captions; the test
    imports `internal/aqua` and `aqua/theme`, test only). No existing linux test laid aqua
    out over real text, hence the last two. Values logged as `TEXT-RESULT`.
  * **Review** (a fourth agent, Pango from memory; docs.gtk.org blocked): signatures, enum
    values, ownership (per-thread font map not unreffed, context and layouts unreffed,
    descriptions freed after the copy, the GValue string copied before `unset`), Go's and
    GLib's UTF-8 validity agreeing, and the shutdown order confirmed; applied: hint metrics
    off, `auto_dir` off, generic and case-insensitive families, the oversize text not using
    up the log, floor + slack instead of ceil. **Open points:** (1) S4.2j adds fonts after
    the context exists: it must clear `families`/`folded` and call
    `pango_context_changed` (or add Inter before `openPango`). (2) Hint style and
    antialiasing follow cairo's defaults, not the display's; S4.2i checks GTK draws the
    layouts the same. (3) Whether Pango counts a hanging space at a wrap
    (`TestTextPangoWrap` will tell).
  * **Checks:** the full `check.sh` passes on the host, stress run included.
  * **Not run (no machine):** the fourteen `TestTextPango*` through `native.sh` on both
    backends (no display needed except `TestTextPangoShell`), `TestLoad` resolving the new
    functions and the two new libraries.

**S4.2h Hit-testing, carets and selection** ✅ *2026-11-03, built only* (the step text as planned):
* The geometry is read once in `NewLayout` from Pango (line starts and lengths from the
  iterator, cluster boxes from `index_to_pos`, cursor positions from
  `get_log_attrs_readonly`, character→byte mapped) into Go data, untagged, with the same
  rules as Win32's `textgeom.go` and `shell/fake` (right of a line, the caret at a wrap,
  a selected line break's width). `HitTest`, `CaretRect`, `SelectionRects` and index
  normalization to cluster starts.
* Done when: host tests of the geometry on hand-made line/cluster data (every rule above,
  combining marks, a ZWJ emoji, clamping); linux tests on real layouts (ASCII, wrapped,
  multi-line, combining marks, an emoji); `check.sh` passes.

* **Result:**
  * **Geometry (pure, new `textgeom.go`):** Win32's rules over byte offsets (Pango indexes
    the UTF-8 it is given and `pangoText` keeps `Text()`'s offsets, so no index map).
    `pangoMetrics` carries `[]pangoLine` (start, length, y range, end x) and
    `[]pangoCluster` (start, box from the leading edge with a negative width for RTL,
    `is_white`) instead of a line count; `newGeometry` builds `geoLine`s/`geoCluster`s and
    refuses data that does not cover the text (the layout is then a fake one, logged once,
    as a measure failure). A cluster is a grapheme; every query normalizes to its start;
    a paragraph delimiter is outside Pango's line content, and U+2028 (the one forced break
    Pango keeps in the content) is trimmed from a non-last line. `textLayout` keeps the
    geometry (the mean line height is gone); `LineCount` is its line count; `normalize`
    as on Win32.
  * **Native (`text_linux.go`):** **deviation from the step text (review finding):**
    `index_to_pos` per cluster is quadratic (it walks the lines and counts characters from
    the text's start each call), so `measure` reads in linear time: the lines from one
    iterator pass (`get_start_index`/`get_length`, `get_line_yrange`,
    `line_index_to_x` at the content's end plus the line's logical x from
    `get_line_extents`, so end x and boxes share layout coordinates), the character boxes
    from a second (`next_char`, `get_char_extents`, the direction from the run's bidi level
    read through new `GlyphItem`/`PangoItem` mirrors, `gtk.RunLevel`), and the clusters
    from the cursor positions of `get_log_attrs_readonly` plus every line start (so the
    geometry's "a cluster at each line start" never depends on Pango), each the union of
    its characters' boxes.
  * **Bindings:** `pango_layout_get_iter`, `iter_free`, `iter_next_line`, `iter_next_char`,
    `iter_get_line_readonly`, `iter_get_run_readonly`, `iter_get_index`,
    `iter_get_line_yrange`, `iter_get_line_extents`, `iter_get_char_extents`,
    `line_get_start_index`, `line_get_length`, `line_index_to_x`,
    `get_log_attrs_readonly`; mirrors `PangoRectangle`, `GlyphItem`, `PangoItem` (layout
    tests); `LogAttr*` constants. `pango_layout_get_line_count` is no longer bound.
  * **Differences from Win32 (documented):** Pango does not break at VT, FF or NEL (they
    are ordinary clusters); the last space of a line wrapped at it has no width in Pango
    (`zero_line_final_space`), so selecting only it shows no rect.
  * **Tests (host, written by a second agent):** `textgeom_test.go`: `TestGeomOneLine`,
    `…Empty`, `…Clusters` (a two-byte rune, a combining mark, a ZWJ emoji), `…Newlines`,
    `…CRLF`, `…WrapWhite`, `…WrapBroken`, `…ForcedBreak` (U+2028 with and without a box,
    VT/FF/NEL ordinary), `TestIsForcedBreak`, `TestGeomRTL`, `TestGeomErrors` (20 refusals,
    3 near-valid cases), `TestGeomNewLayout`; `text_test.go`'s fake layouter builds a
    geometry, `TestTextLayoutMetrics` checks the queries, `TestTextLayoutNoLines` the
    fallback on bad geometry.
  * **Tests (linux, not run, no machine; written by a third agent):**
    `TestTextPangoCaretASCII`, `…HitTestASCII`, `…SelectionASCII`, `…WrapGeometry`,
    `…Newlines` (`\n`, `\r\n`, the empty last line, the break's width), `…Combining`,
    `…Emoji` (single, ZWJ, skin tone, flag), `…Clamp`, `…LineSeparator` (U+2028; VT only
    logged), `…RTLCarets` (Hebrew; skips without a font), `…LargeText` (~100 KB under 2 s,
    timing logged as `TEXT-RESULT`).
  * **Review** (a fourth agent, Pango from memory; gitlab.gnome.org and docs.gtk.org not
    reachable): signatures, `PangoLogAttr` bits and size, paragraph and line-content rules
    confirmed; applied: linear-time reading, clusters at line starts, U+2028 only, end x in
    layout coordinates. Open: whether a trailing U+2028 adds an empty line (it does on
    DirectWrite); a line starting with an RTL run hit left of x = 0 gives the line's start
    (as Win32).
  * **Checks:** the full `check.sh` passes on the host, stress run included.
  * **Not run (no machine):** the eleven new `TestTextPango*` and the existing ones through
    `native.sh` on both backends, `TestLoad` resolving the new functions.

**S4.2i Drawing text** ✅ *2026-11-04, built only* (the step text as planned):
* `DrawText` rebuilds the native `PangoLayout` once per layout and frame (released with
  the frame) and draws it with `gtk_snapshot_translate` + `gtk_snapshot_append_layout`
  under the current state; nil, fake, foreign layouts, empty text, semantic or
  transparent colors and non-finite origins draw nothing. The text placeholder box goes.
* Done when: host tests of the ignored cases and the per-frame cache; linux node-tree
  tests (a text node inside the layout's box, its color, translated and clipped; one
  native layout per layout and frame); `check.sh` passes.

* **Result:**
  * **Canvas (`canvas.go`):** `DrawText` draws only a `*textLayout` of the canvas's own
    text system (`canvas.text`, set by the surface's snapshot to the shell's), so
    `shell/fake` layouts (Pango missing or failed) and other shells' layouts draw nothing,
    as do nil (typed too), empty text, semantic or transparent colors, origins not finite
    as float32, a released text system and calls after `finish`. The draw is `save`,
    `translate` to the origin, the new snapshotter call `appendLayout` (color copied) and
    `restore`, so it nests in any transform, clip, opacity and blur segment. The per-paint
    cache `texts` (one native layout per layout; a failure is cached too, so it is not
    retried within the paint) is released by `finish`. The placeholder (`textBox`,
    `outline`) is gone; `hair` remains for the gradient cover; `boxOK` stays for the blur.
  * **Text system (`text.go`, `text_linux.go`):** `textLayout` keeps its text system and
    what Pango measured (`ptext` with the stand-ins, the resolved `pfont`, the wrap
    `width`), so drawing rebuilds exactly the measured layout; `pangoLayouter` gained
    `layout` (on Linux `pangoNative.layout`, the same `newLayout` as `measure`, returned as
    a `pangoLayout` whose `release` unrefs it); `textSystem.native` returns nil when closed
    and logs the first failure once (`drawFailed`). No finalizers.
  * **Binding:** `gtk_snapshot_append_layout` (GTK 4.0, required); `TestTextDrawBindings`.
  * **Tests (host, written by a second agent):** new `canvastext_test.go`:
    `TestCanvasTextOps`, `…Ignored`, `…Cache`, `…LayoutArgs`, `…LayoutFails`, `…Blur`;
    `TestCanvasPlaceholders` became `TestCanvasForeignContent`, `TestCanvasTextBox` is
    gone, `TestCanvasHairline` checks `hair`; `TestSurfaceSnapshotPaint` draws through the
    shell's text system over the fake layouter (one native layout, released once).
  * **Tests (linux, not run, no machine; written by a third agent):** `TestCanvasGTKText`
    (text nodes in the exact color, the first run's baseline at origin + `Baseline` in
    root coordinates within 0.01 pt, ink bounds in the layout's box ± 2 pt; translated and
    scaled, under a cutting rounded clip, under opacity as a node or folded into the
    color, logged), `TestCanvasGTKTextIgnored`, `TestCanvasGTKTextOneLayoutPerPaint` (a
    counting layouter and snapshotter wrapper: one native layout per layout and paint, all
    released); `TestCanvasGTKPlaceholders` is gone, `TestCanvasGTKSurfacePaint` expects a
    Pango text node. Risks for the log: the 0.01 pt baseline tolerance (if a GTK version
    rounds glyph positions) and the 2 pt ink tolerance.
  * **Found, not fixed (outside the step):** `render/record`'s `DrawText` checks only an
    untyped nil and calls `Size()`, so a typed-nil layout panics with the recorder on
    (`GOGUI_DEBUG=1`), on Win32 too.
  * **Checks:** the full `check.sh` passes on the host, stress run included.
  * **Not run (no machine):** the new and changed linux tests through `native.sh` on both
    backends, `TestLoad` resolving `gtk_snapshot_append_layout`.

**S4.2j Inter as an app font** ✅ *2026-11-04, built only* (the step text as planned):
* Binds `libfontconfig.so.1` (`FcConfigGetCurrent`, `FcConfigAppFontAddFile`) and
  `libpangoft2-1.0.so.0` (`pango_fc_font_map_config_changed`), plus the optional
  `pango_font_map_add_font_file`. The embedded files are written once to the cache
  (atomic rename, reused when present, the name from the content hash) and added before
  the first layout; any failure means the system UI font, logged once with
  `GOGUI_DEBUG=1`. `pango_font_map_get_family("Inter")` checks the result.
  `assets/fonts/README.md` and ARCHITECTURE.md's dependency list get the Linux shell.
* Done when: host tests of the cache file (written, reused, a corrupt or unwritable cache
  falling back) through a test hook; linux tests (Inter resolved for the empty family,
  the fallback without files); `check.sh` passes.

* **Result:**
  * **Bindings (`shell/gtk4/gtk`):** two new libraries, `libpangoft2-1.0.so.0`
    (`PangoFc`: `pango_fc_font_map_get_config`, `pango_fc_font_map_config_changed`) and
    `libfontconfig.so.1` (`Fc`: `FcConfigGetCurrent`, `FcConfigAppFontAddFile`), both
    required (libpangocairo is built on them on Linux), with their distribution packages in
    the missing-library message; `g_error_free`; the one optional binding,
    `pango_font_map_add_font_file` (Pango 1.56; `TestFloorBindings` now expects exactly
    it); the `GError` mirror (16 bytes, layout test). `TestAppFontBindings`.
  * **Cache (`appfont.go`, untagged):** `cacheFonts` writes each file to
    `os.UserCacheDir()/gogui/fonts/<sha256><ext>` through `os.CreateTemp` in the same
    directory and `os.Rename`; a file is reused only when it holds exactly the embedded
    bytes (size first), so a torn or foreign file is replaced, not trusted. Beyond the
    step text (from the review): temporary files older than an hour, left by a process
    that died before its rename, are removed.
  * **Text system (`text.go`):** `init` reads `gtk-font-name`, then `loadAppFont`: files →
    cache → `pangoLayouter.addFonts` → `hasFamily("Inter")`; on success the UI family is
    Inter, on any failure it stays the system one and `debugf` (log.Printf with
    `GOGUI_DEBUG=1`) says why, once per text system. Hooks: `fontFiles`, `cacheDir`,
    `appUI`, `debugf`; the shell's host tests run without files, so they never write to
    the user's cache.
  * **Native (`text_linux.go`):** `pangoNative.addFonts` uses
    `pango_font_map_add_font_file` when bound, else `FcConfigAppFontAddFile` on the map's
    FcConfig (its own, else the current one) and `pango_fc_font_map_config_changed`
    (also after a partial failure). Files already added to the same font map *and*
    FcConfig are skipped (`addedFonts`), so later text systems (another `Run`, tests) don't
    pile up duplicates, while a new FcConfig gets them again.
  * **Found (review agent), known limitation:** GTK calls `FcInitReinitialize` when
    fontconfig's files change while the app runs (a font installed); the new config has no
    app fonts, so a running text system's Inter is substituted by Pango until the next text
    system re-adds it. A private font map with its own FcConfig would avoid it; not done
    (rare, and GTK's own text would still use the default map).
  * **Tests (host):** `appfont_test.go`: `TestAppFontCacheWrites`, `…Reused`, `…Corrupt`,
    `…Unwritable`, `…Sweep`, `TestAppFontUsed`, `TestAppFontFallback` (9 failure cases),
    `TestAppFontNoDebug`; the fake layouter got `addFonts`.
  * **Tests (linux, not run, no machine):** `appfont_linux_test.go`:
    `TestAppFontPangoInter` (Inter is the UI family, found by `pango_font_map_get_family`,
    and what `pango_font_map_load_font` loads for the empty family at regular and bold;
    a second text system too), `TestAppFontPangoFontconfig` (the fontconfig path, forced)
    and `TestAppFontPangoFallback` (no files; a file that is not a font). The existing
    linux text tests now run with Inter as the UI family (their checks use `ts.ui`).
  * **Checks:** the full `check.sh` passes on the host, stress run included.
  * **Not run (no machine):** the linux tests above through `native.sh` on both backends,
    `TestLoad` resolving the new libraries and symbols (and logging whether
    `pango_font_map_add_font_file` is bound).

**S4.2k Wrap-up** ✅ *2026-11-04, built only* (the step text as planned):
* `shell/fake` and `render/record` stay only where still needed (the debug recorder),
  every `interim` name and S4.2 forward reference is gone; `native.sh` runs the new
  linux tests on both backends; ARCHITECTURE.md describes the canvas and the text
  system; PLAN.md status and S4.2's ✅ (*built only*).
* Done when: `check.sh` passes; **for the user, on Linux (Wayland and X11):**
  `bash scripts/native.sh` and the log back in `native-logs/`, then a look at
  `examples/hello`, `controls`, `form` and `notes` (real text in Inter, shadows,
  rounded controls, crisp at scale 1 and 2).
* **Result:**
  * **Audit (an Explore agent over `shell/gtk4`, `gtk4/gtk`, `shellkit`, `render/record`,
    `shell/fake`):** no `interim` identifier, file or comment is left in the gtk4 shell,
    no "until S4.2"-style forward reference, no TODO, no 4.10/4.12 fallback. Two stale
    test failure messages ("a path/shadow fell back to a placeholder outline", a fallback
    that no longer exists) now say "drew as a border"; the headers of `canvas_test.go`
    and `canvas_linux_test.go` name every S4.2 sub-step they cover; `doc.go` names S4.2.
    The forward references left are to later steps (S4.3 portal appearance, S5.3 IME,
    the public image API) and accurate. "interim" survives only in `shell/win32`'s own
    history comments (S2.3b GDI paint and clock bindings), outside this step.
  * **`shell/fake` and `render/record`:** each has one non-test importer in gtk4, both
    needed: `canvas.go` uses `render/record` only as the `GOGUI_DEBUG=1` recorder;
    `text.go` uses `shell/fake`'s fixed-width layouts when Pango is missing (the host
    tests), failed to open or measure, was released, or for text over 2 GiB. Tests also
    use `fake` for foreign layouts. Nothing removed.
  * **`native.sh`:** the gtk4 linux tests were already covered by
    `go test -count=1 -v ./internal/shell/gtk4/...` (the S4.2 canvas, text and app-font
    tests included). New: on Linux every example (`hello`, `controls`, `form`, `notes`,
    `converter`) is built once and run for 5 s with `GOGUI_DEBUG=1` on each backend
    (`examples_on`/`example_for`, replacing `hello_for`; xvfb, headless weston, and the
    desktop session's Wayland/X11 when set), one step per example and backend.
  * **Docs:** ARCHITECTURE.md §0's status, §1 tooling (`native.sh`), §8.5 (the shell's
    step list, `shell/fake`'s role, the canvas bullet) and §10 (the canvas tests) lose
    their interim wording; the canvas and text system bullets (written in S4.2b–S4.2j)
    stand as the description. PLAN.md §8.4, the status line, S4.2's ✅.
  * **Checks:** the full `check.sh` passes on the host, stress run included;
    `bash -n scripts/native.sh`.
  * **Not run (no machine):** `native.sh` on Linux (both backends), every gtk4 linux test
    since S4.1b, and the user's look at `hello`, `controls`, `form` and `notes`.

## J. Phase 4: S4.3 (S4.3a–S4.3i) (from PLAN.md §11.1)

**S4.3a Portal mapping (pure Go, host-tested)** ✅ *2026-11-04* (the step text as planned):
* An untagged `portal.go` in `shell/gtk4`: the `settingsSource` interface (portal values
  by key, GtkSettings values by name, each with "unset"), and `readAppearance(src)` with
  the key rules above. Variant decoding takes plain Go values (`uint32`, `[3]float64`,
  `bool`), so no godbus type leaks in.
* Tests: table-driven, as `TestReadAppearance` on Win32 (each key, out-of-range accent,
  unknown values, portal absent, GtkSettings fallbacks).
* No new dependency yet; nothing is wired into the shell.

* **Result:**
  * **`portal.go` (untagged):** `settingsSource` has two methods, `portalValue(key) (any,
    bool)` (a key of `org.freedesktop.appearance`, already decoded: `uint32` for `u`,
    `[3]float64` for `(ddd)`; S4.3b's client does the variant decoding) and
    `gtkBool(name) (bool, bool)`. The key and property names are constants; the bus name,
    object path and interface arrive with the client in S4.3b (unused constants would
    fail staticcheck).
  * **Rules (`readAppearance`):** from `driver.DefaultAppearance()`; a portal value that
    is unset, of another type or out of range (`color-scheme` > 2, `contrast` or
    `reduced-motion` > 1, an accent component outside 0..1, NaN, ±Inf) counts as unset,
    and an unset one falls back to its GtkSettings property where there is one
    (`color-scheme` → `gtk-application-prefer-dark-theme`, `reduced-motion` →
    `gtk-enable-animations` false), else keeps the default. So the "without a portal"
    fallback of the design also covers a portal that lacks the key (older portals) or
    reports an unknown value, and a set portal value wins over the GtkSettings one (portal
    `reduced-motion` 0 with animations off is not reduced). `LegacyScrollers` comes from
    `gtk-overlay-scrolling` false only; `ReduceTransparency` stays false. The accent is
    always opaque.
  * **Tests (host):** `portal_test.go`: `TestReadAppearance`, 34 cases over a map-backed
    fake source (each key and value, unknown values and other types, the accent's bounds,
    the unset triple, NaN and +Inf, the portal absent, the fallbacks and their precedence,
    everything at once, unrelated keys); every result has an opaque concrete accent and
    no `ReduceTransparency`.
  * **Checks:** the full `check.sh` passes on the host, stress run included. Nothing
    linux-only in this step, so it is fully checked here.

**S4.3b Portal client over godbus (adds `godbus/dbus/v5`)** ✅ *2026-11-04, built only* (the step text as planned):
* `go get github.com/godbus/dbus/v5` (v5.2.x via the proxy; go.mod and go.sum in this
  step; ARCHITECTURE.md's dependency list). A `portal` type: session bus connect,
  `ReadAll` with the `ReadOne`/`Read` fallbacks, the 250 ms timeout, a `SettingChanged`
  match rule filtered to `org.freedesktop.appearance`, a change callback, `Close`.
  It builds on every GOOS (pure Go); only the shell on Linux uses it.
* Tests: the decoding of portal replies into the S4.3a values (host); a linux test
  against a fake portal that the test itself exports on a private bus (godbus
  `Export` + `RequestName` under `dbus-run-session`, skipped without it): read, then a
  `SettingChanged` emitted by the test reaches the callback, and an absent portal gives
  "unset" within the timeout.

* **Result:**
  * **Dependency:** `github.com/godbus/dbus/v5` v5.2.4 (go.mod, go.sum; its only
    requirement, `golang.org/x/sys`, was already there). Imported only by `shell/gtk4`, so
    `go list -deps ./examples/hello` shows it on linux and not on windows or darwin.
  * **`portalclient.go` (untagged):** `openPortal(cfg, onChange) (*portal, error)` with a
    `portalConfig` (`dest`, `timeout`, `dial`, `auth`; `defaultPortalConfig` uses
    `SessionBusPrivateNoAutoStartup`, so an app never launches a session bus). The whole
    start-up is bounded by the 250 ms `portalTimeout`: godbus's dial, `Auth` and `Hello`
    take no context, so they run on a goroutine and a timeout closes the connection
    under them. The match rule (sender, path, interface, member, arg0
    `org.freedesktop.appearance`) is added **before** the read, with godbus's sequential
    signal handler; the `watch` goroutine starts only after the read, so queued changes
    apply after it in order. Read: `ReadAll`; when the portal itself answers that with
    an error (anything but its absence, `NoReply`/`Timeout`/`Disconnected`, a timeout or a
    broken connection), `ReadOne` per key, and `Read` (doubly wrapped) on `UnknownMethod`;
    `NotFound` is unset. Outcomes: no bus or a failed connect/subscribe → nil portal and
    the error; only the read failed → the portal (still subscribed, so a portal that
    starts late or is activated slowly still reports changes) and the error; errors are
    for S4.3c's debug log. A nil `*portal` is a valid empty source. Values are decoded
    once (`decodeValue`: variants unwrapped, a wire `(ddd)`, a `[]any` of three
    `float64`, → `[3]float64`) into a mutex-guarded map read by `portalValue` (the
    `settingsSource` half). `onChange` runs on godbus's goroutine after each
    `SettingChanged` of the namespace; `Close` is idempotent and waits for `watch`, so no
    callback comes after it.
  * **Deviation (more than planned):** besides the decoding, the whole client runs on
    every host against `fakeBus`, a godbus connection over a `net.Pipe` that does the
    daemon's half of SASL (ANONYMOUS) by hand and then answers `Hello` and `AddMatch`
    and exports the fake portal (its own client `Auth` runs against a scripted daemon).
    So the connect, the match rule, all three read paths, the timeouts and the signal
    path are checked here, not only built. The fake portals and helpers are shared with
    the linux test.
  * **Tests (host, `portalclient_test.go`):** `TestDecodeValue`, `TestDecodeReadAll`,
    `TestDecodedReadAppearance`, `TestPortalApply`, `TestPortalErrors`, `TestPortalNil`,
    `TestOpenPortalReadAll` (values, the rule's parts, three changes in order with one
    `onChange` each, another namespace ignored, none after `Close`),
    `TestOpenPortalFallbacks` (ReadOne, Read, ReadOne after AccessDenied),
    `TestOpenPortalUnread` (no Settings object, ServiceUnknown: unset, error, still
    subscribed), `TestOpenPortalHungPortal`, `TestOpenPortalHungBus` (back within the
    timeout, the abandoned connection closed), `TestOpenPortalNoBus`,
    `TestOpenPortalAccentBounds`. Stress: `-count=10 -cpu=1,2,4,8` passes.
  * **Tests (linux, `portalclient_linux_test.go`; not run, no machine):** the fake portals
    on the real session bus under a test-unique name (so no real xdg-desktop-portal is
    needed or disturbed; skipped without `DBUS_SESSION_BUS_ADDRESS`):
    `TestPortalSessionBus` (read; a change from a non-owner and one of another namespace
    filtered by the rule, the owner's arrives), `TestPortalSessionBusFallbacks`,
    `TestPortalSessionBusAbsent` and `TestPortalSessionBusHung` (unset within 250 ms).
    `native.sh` does not wrap them in `dbus-run-session` yet (S4.3d); on a desktop session
    they run against the user's bus.
  * **Checks:** the full `check.sh` passes on the host, stress run included. ARCHITECTURE.md
    (dependency list, the gtk4 package row and files, the client, its tests) and the
    gtk4 package doc updated.

**S4.3c Shell wiring** ✅ *2026-11-04* (the step text as planned):
* `Shell` caches its appearance: read before `Ready` (portal + GtkSettings source),
  `Appearance()` returns the cache, `appearanceChanged()` re-reads, dedupes and notifies
  (Win32's `appearanceChanged` shape). The portal callback goes through `Shell.Post`;
  the portal is closed in `shutdown()`. The source is injectable, so tests use a fake
  one (also what S4.3f's golden render uses to force Reduce Motion and legacy scrollers).
* Tests: host tests with the fake loop and a fake source (`TestShellServices` updated;
  a `TestAppearanceChanged` as on Win32: dark, then accent, exactly two callbacks, none
  before `Ready` or after quit, a post after quit dropped).

* **Result:**
  * **The source (`portal.go`):** `appearanceSource` is a `settingsSource` with `close()`;
    `openSource func(changed func()) appearanceSource` opens it, `changed` may be called
    on any goroutine and must not block. `Shell.openSettings` is the opener:
    `newShell` defaults to `openNoSettings` (every value unset, so the default appearance
    and the host tests never touch a session bus); `New` (linux) sets
    `openSystemSettings(defaultPortalConfig(), logf)`, whose `systemSettings` reads the
    `*portal` (nil without a bus) and, until S4.3d, reports every GtkSettings value as
    unset. A failed `openPortal` is logged once, through `log.Printf` only under
    `GOGUI_DEBUG=1` (the design's debug level).
  * **The shell (`shell.go`):** `Run` opens the source after the loop's `init` (GtkSettings
    need GTK) and after the deferred `shutdown`, so it is closed on every way out, and
    reads the cache before `Ready` (bounded by the portal's 250 ms). The source's
    `changed` is `Post(appearanceChanged)`: a change during the open runs after `Ready`
    and dedupes against the first read; after `Quit` the closed queue drops it.
    `appearanceChanged` returns after shutdown, re-reads, compares with `==` and calls
    `Handler.AppearanceChanged` only when ready and not quitting. `shutdown` closes the
    source first (`portal.Close` waits for its goroutine, so no change after it).
    Unlike Win32 nothing is invalidated: the gtk4 shell clears to no background of its
    own, and the aqua driver repaints on `AppearanceChanged`.
  * **Tests (host):** `shell_test.go`: a `fakeAppSource` (maps, opener recording
    `changed`, open/close counts); `TestShellServices` reads dark and reduced motion
    from it; `TestShellDefaultSettings` (default source → `DefaultAppearance`);
    `TestAppearanceChanged` (initial read with legacy scrollers and contrast, a change
    reported while opening gives no callback, changes from another goroutine: dark then
    the accent, two callbacks on the UI thread, a change after `Quit` and one after
    `Run` dropped, opened and closed once, torn down); `TestAppearanceSourceClosedOnPanic`;
    `checkTornDown` checks the source is gone. `portalclient_test.go`:
    `TestOpenSystemSettings` (over `fakeBus`: the portal's values, a `SettingChanged`
    through `changed`, nothing logged) and `TestOpenSystemSettingsNoBus` (default
    appearance, one log line, a nil `logf`). Stress: the new tests `-count=50
    -cpu=1,2,4,8` pass.
  * **Checks:** the full `check.sh` passes on the host, stress run included. Nothing new
    is linux-only beyond the two lines in `New`, which are cross-compiled; the real
    session-bus path waits on the Linux log with S4.3b's linux tests.

**S4.3d GtkSettings: live values** ✅ *2026-11-04, built only* (the step text as planned):
* The GtkSettings half of the source (`g_object_get_property` as for
  `gtk-double-click-time`), and `notify::gtk-enable-animations`,
  `notify::gtk-overlay-scrolling`, `notify::gtk-application-prefer-dark-theme` connected
  once on the default GtkSettings through `sigPointer`, each calling
  `appearanceChanged()`. Disconnected at shutdown.
* Tests: host (dispatch of the notify to `appearanceChanged`); linux: setting the
  property with `g_object_set_property` triggers one `AppearanceChanged`.
* `native.sh` runs the gtk4 tests under `dbus-run-session` when it is installed (logged
  either way).

* **Result:**
  * **`gtksettings.go` (untagged):** `gtkSettings`, the GtkSettings half of
    `systemSettings`, over a small `gtkProps` interface (`boolProp`, `watch`, `unwatch`).
    `openGtkSettings(props, changed)` watches `gtkWatched` (the three properties) with
    `kindSettingNotify`, the `*gtkSettings` as the `pointerSignalPeer`; a notify calls the
    source's `changed`, which posts `appearanceChanged` as a portal change does (so the
    "calling `appearanceChanged()`" of the plan goes through `Post` and dedupes). `close`
    unwatches once; nothing is read or reported after it. A nil `*gtkSettings`, or one
    without props, is every value unset.
  * **`gtksettings_linux.go`:** `newNativeGtkProps` refs `gtk_settings_get_default()` (none
    without a display) until `unwatch`, which disconnects every handler (`closureNotify`
    frees the handles) and unrefs. A property is looked up first with
    `g_object_class_find_property` (`g_object_get_property` and the notify would warn on
    a name a GTK lacks), then read through a `G_TYPE_BOOLEAN` GValue in `g_malloc` memory.
  * **Wiring:** `openSystemSettings(cfg, newProps, logf)` takes the props factory
    (`newNativeGtkProps` in `New`; nil: none, so the portal-only tests stay as they were),
    opens the GtkSettings before the portal and closes them after it; a property it
    cannot watch is logged once under `GOGUI_DEBUG=1`, like a missing bus.
  * **Bindings (`gtk`):** `g_object_set_property` (the linux test),
    `g_object_class_find_property`, `g_value_set_boolean`, `g_value_get_boolean`;
    `TestBindingTables` covers them.
  * **`native.sh`:** the gtk4 tests under xvfb and headless weston run inside
    `dbus-run-session --` when it is on `PATH` (the portal client's session-bus tests then
    run on a private bus; they export their own fake portal), and the log says which; the
    desktop-session runs keep the user's bus. PLAN.md §8.4 and ARCHITECTURE.md say so.
  * **Tests (host, `gtksettings_test.go`):** `TestGtkSettingsWatch`,
    `TestGtkSettingsMissing`, `TestOpenSystemSettingsGtk`,
    `TestGtkSettingsAppearanceChanged` (a `fakeGtkProps` keeps a real dispatch handle per
    watched property, so the notify goes through the real `sigPointer` into a running
    shell: one `AppearanceChanged` for a changed value, none for notifies without one,
    every handle freed at teardown).
  * **Tests (linux, `gtksettings_linux_test.go`; not run, no machine):**
    `TestGtkSettingsNotify` over the real default GtkSettings with no bus (so the portal
    does not mask them): the three properties read (`SETTINGS-RESULT` lines) and watched,
    flipping `gtk-overlay-scrolling` with `g_object_set_property` gives exactly one
    `AppearanceChanged`, restored after `Run` with no handler left.
  * **Checks:** the full `check.sh` passes on the host, stress run included; the new host
    tests also pass `-count=50 -cpu=1,2,4,8`. The linux test and the `dbus-run-session`
    path wait on the Linux log.

**S4.3e Shared golden harness (Windows host)** ✅ *2026-11-04* (the step text as planned):
* The OS-neutral parts of `win32/gallery_test.go` move into an untagged helper (in
  `internal/aqua/gallery/golden`, or a `goldentest` package next to it, imported only by
  tests): the variant table (light/dark × 1×/2×, names), BGRA-premultiplied → NRGBA,
  and the compare-and-report flow (log the `Result`, write `<name>.actual.png` and
  `<name>.diff.png` to `GOGUI_GOLDEN_OUT`, `-update` only where the caller allows it).
* Tests: the helper's own unit tests; `TestGalleryGolden` on Windows still passes at
  `DefaultOptions` with max delta 0 (no PNG changes).

* **Result:**
  * **`internal/aqua/gallery/goldentest` (new, untagged, imported only by tests;
    standard library and `golden` only):** `Variant{Dark, Scale}` with `Name()`
    (`gallery-{light,dark}-x{1,2}`), `Variants` (the four, in render order),
    `DirFromShell` (`../../aqua/gallery/testdata/golden`, the goldens from a shell
    package's test directory), `FromBGRA(w, h, stride, bgra)` (premultiplied BGRA →
    straight-alpha NRGBA, rounded, transparent as transparent black, clamped; a stride
    because `gdk_texture_download` takes one; panics on a short buffer) and `Check{Dir,
    Options, Update, Out, LogOnly}.Run(t, v, got)`: with `Update` it writes the golden
    (the caller decides; the package registers no flag, so the gtk4 tests can never
    rewrite through it), else it logs the `Result` and on a failure writes
    `<name>.actual.png` (and `<name>.diff.png` when the sizes match) to `Out`, else
    `$GOGUI_GOLDEN_OUT`, else `t.TempDir` (kept in `Out` for the next variant), and
    reports through `t.Errorf`, or `t.Logf` with `LogOnly` (for S4.3g's uncalibrated
    run; a missing golden or an unwritable file still fails). It returns the `Result`.
    Chose a separate package over `golden` so that `golden` stays free of `testing`.
  * **`win32/gallery_test.go`:** the variant type, `unpremultiply` and its test, and the
    compare loop are gone; it uses `goldentest.Variants` (the DPI is `96 × Scale`),
    `FromBGRA` with a tight stride and one `Check` with `golden.DefaultOptions` and its
    own `-update` flag.
  * **Tests (`goldentest_test.go`):** `TestVariants` (names, order, every golden
    committed, `DirFromShell`), `TestFromBGRA` (the old `TestUnpremultiply` cases),
    `TestFromBGRAStride` (padded rows, short buffers panic, empty image),
    `TestCheckRun` over a recording `testing.TB` (update writes only the golden; a match
    logs and writes nothing; one changed pixel fails naming both files; `LogOnly` logs
    the same report and still writes; a size mismatch writes only the actual image;
    `GOGUI_GOLDEN_OUT` and the temp-dir fallback; a missing golden fails even with
    `LogOnly`).
  * **Checks:** the full `check.sh` passes on the host, stress run included;
    `TestGalleryGolden` passes with all four at max delta 0 and the PNGs unchanged.
    Fully checked on the Windows host; nothing waits on another OS.
