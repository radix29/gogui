# gogui architecture

The architecture **as implemented today**: packages, contracts and invariants. The target
design, scope and roadmap are in [`PLAN.md`](PLAN.md). Where this file and the code
disagree, the code wins and this file is fixed in the same change.

## 0. Target architecture overview

Copied from PLAN.md §2 and §2.1. This is the **target** design; section 1 onwards shows
what exists. When the implementation changes the overview (a package is renamed, a layer
is added or dropped, a responsibility moves), change this section and PLAN.md §2 together;
`TestOverviewMatchesPlan` (`internal/repocheck`) fails while they differ.

```
┌──────────────────────────────────────────────────────────────────────────┐
│                     Application code (portable Go)                       │
│      gogui.Run(...)  NewWindow  NewButton  VStack  MenuBar  Alert …      │
└──────────────────────────────────────────────────────────────────────────┘
                                    │
┌──────────────────────────────────────────────────────────────────────────┐
│  Public API  (package gogui)                                             │
│  • Widget objects, properties, events     • Layout engine (Go, shared)   │
│  • Semantic colors / fonts / appearance   • Main-thread dispatcher       │
└──────────────────────────────────────────────────────────────────────────┘
                                    │  internal/driver  (peer interfaces)
              ┌─────────────────────┴───────────────────────┐
              ▼                                             ▼
┌──────────────────────────────┐        ┌──────────────────────────────────┐
│  appkit driver  (darwin)     │        │  aqua driver  (windows, linux)   │
│  purego/objc (no cgo)        │        │  Pure Go "drawn AppKit":         │
│  NSWindow, NSButton,         │        │  • widget state machines         │
│  NSTextField, NSSlider,      │        │  • macOS theme tokens            │
│  NSPopUpButton, NSMenu,      │        │    (light, dark, accent, sizes)  │
│  NSAlert …                   │        │  • animation engine              │
│  = 100 % native              │        │  • focus / keyboard navigation   │
└──────────────────────────────┘        │  • text editing model            │
                                        │  • accessibility tree            │
                                        └──────────────────────────────────┘
                                                         │ render.Canvas + shell.Shell
                                  ┌──────────────────────┴───┐
                                  ▼                          ▼
                        ┌───────────────────┐   ┌─────────────────────────┐
                        │ win32 shell       │   │ gtk4 shell              │
                        │ (pure Go, no cgo) │   │ (purego/dlopen, no cgo) │
                        │ HWND + DWM        │   │ GtkWindow (CSD)         │
                        │ Direct2D/D3D11    │   │ GtkSnapshot → GSK       │
                        │ DirectComposition │   │   (OpenGL / Vulkan)     │
                        │ DirectWrite       │   │ Pango + HarfBuzz        │
                        │ IMM32/TSF, UIA    │   │ GtkIMContext, AT-SPI    │
                        │ IFileDialog       │   │ xdg-desktop-portal      │
                        └───────────────────┘   └─────────────────────────┘
```

| Layer | Package | Responsibility |
|---|---|---|
| Public API | `gogui` | Widgets, windows, menus, layout, events, colors, fonts. The only package apps import. |
| Driver interfaces | `internal/driver` | "Peer" interfaces (`driver.Button`, `driver.Window` …), one peer per public widget, the same pattern AWT/SWT use. |
| macOS driver | `internal/appkit` | Implements the peers with AppKit objects, called through the Objective-C runtime with `purego/objc` (no `.m` files, no cgo). |
| Native bindings | `internal/ffi` | Shared purego helpers: library loading, a fixed set of callback trampolines, struct/ABI helpers, `objc` class registration (PLAN.md §6.1, §7.2). |
| Aqua driver | `internal/aqua` | Implements the peers as **drawn widgets** that replicate AppKit visuals and behaviour. Shared by Windows and Linux. |
| Theme | `internal/aqua/theme` | Design tokens: colors (light/dark/high-contrast), metrics per control size, fonts, radii, shadows, animation curves. |
| Render | `internal/render` | GPU-backed 2D `Canvas` interface plus a recording backend for tests. |
| Shell | `internal/shell` | OS integration interface for the aqua driver: native windows/popups, event loop, input, IME, text shaping, clipboard, dialogs, system settings, frame clock. |
| Shell: Windows | `internal/shell/win32` | Win32 + DWM + Direct2D + DirectWrite + DirectComposition via `golang.org/x/sys/windows` and hand-written COM vtables (`purego` only for COM calls with float arguments). **No cgo.** |
| Shell: Linux | `internal/shell/gtk4` | GTK 4, GLib and Pango loaded at runtime with `purego.Dlopen` (no cgo, no C shim). Drawing goes through `GtkSnapshot` → GSK render nodes (GPU). Portal settings use pure-Go D-Bus (`godbus/dbus/v5`). |
| Layout | `internal/layout` | Stack / form layout (a grid comes after v0.1) used by **all** drivers, so layouts behave the same on every OS. |

### Implemented today

| Target | Driver | State |
|---|---|---|
| Windows (amd64, arm64) | aqua over `shell/win32` | All v0.1 controls, menus (title-bar strip), alerts, IME, clipboard, appearance, goldens. Tested on the windows/amd64 host (`check.sh`, `native.sh`); arm64 built only. |
| Linux (amd64, arm64) | aqua over `shell/gtk4` | Same feature set; built and host-tested (fake/portable tests, cross-compiled). Native runs (X11, Wayland, goldens vs the Windows PNGs) wait on the user's Linux log. |
| macOS (arm64, amd64) | headless | `internal/appkit` (phase 1) is not built; apps print a notice and open no window. |
| tests, any OS | headless / stub | `driver/headless` and `shell/fake` run the public-layer and aqua tests without a display. |

Known limits:

* Theme tokens are provisional until measured on a real Mac (`docs/tokens.md`).
* No screen-reader bridge on Windows/Linux (D4): the a11y tree exists and is tested; UIA and
  AT-SPI come after v0.1.
* No public query or event for a window's key state (`KeyChanged` is ignored by `gogui`).
* Form spacing constants (§6) are from memory of the HIG and still to be checked.

## 1. Layers

File names in this document are anchors into the code, not complete file lists (each
package directory is one); `internal/repocheck` keeps every name real.

```
  examples/*              app code, imports only gogui
  gogui                   public API, widget tree, layout calls
  internal/driver         peer + handler interfaces, shared types (imports nothing)
    ├── driver/stub       no-op recording peers, fake metrics, User… simulations
    ├── driver/headless   display-less driver (stub + mainthread)
    ├── appkit            macOS native peers (purego/objc), not built yet
    └── aqua              drawn peers for Windows/Linux: element tree, paint, hit testing,
        │                 input routing, focus, windows with title bar, controls
        ├── a11y  anim  menubar  scroll  text  theme (+ theme/gen)
        └── gallery (+ gallery/golden, gallery/goldentest)   golden-image tests
  internal/render         Canvas, text interfaces, value types (+ render/record)
  internal/shell          Shell, Surface, events, frame clock (interfaces)
    ├── shell/fake  shell/native  shell/shellkit
    ├── shell/win32       (+ w32, com, dxgi, d3d11, dcomp, d2d, dwrite)
    └── shell/gtk4        (+ gtk4/gtk)
  internal/layout         stack / form / box / scroll layout engine
  internal/mainthread     UI-thread queue
  internal/ffi            handle table, callback registry, layout checks, raw memory
  internal/repocheck      repository consistency tests (test-only, review R14)
  assets/fonts            embedded Inter files (used by shell/win32 and shell/gtk4)
```

Dependency rules:

* `gogui` reaches backends **only** through `internal/driver`; apps never import
  `internal/...`. `gogui` imports `driver`, `layout` and `mainthread`; only the driver-selection
  files (`driver_darwin.go`, `driver_other.go`) import `headless`, `aqua` and `shell/native`
  (`TestDriverBoundary`).
* `mainthread`, `ffi`, `assets/fonts` import nothing in the module; `driver` imports only
  `mainthread` (for `Driver.Queue`). `layout`,
  `render` and `aqua/theme` import only `driver`; `layout` never calls a peer factory.
* `aqua` imports `driver`, `stub`, `layout`, `mainthread`, `render`, `shell` and its own
  subpackages, never a shell implementation. Shells never import `aqua` (`shell/win32` uses
  the leaf `aqua/theme` for its clear color). `shell/native` picks `win32` on Windows,
  `gtk4` on Linux, `ErrNotImplemented` elsewhere. Drivers are never imported by the packages
  they use.
* **Build tags** (a `//go:build` line or a GOOS file name) only in `gogui`'s
  `driver_darwin.go`/`driver_other.go`, `ffi`'s per-OS files, `shell/native`'s per-OS files,
  the Windows packages under `shell/win32` (all their files except the `doc.go` files and
  `com`'s portable `guid.go`, `hresult.go`, `vtable.go`), the `*_linux.go` files of
  `shell/gtk4[/gtk]`, and `spike/` (`TestBuildTagsWhereDocumented`).
  Everything builds and tests with `CGO_ENABLED=0`.
* **External dependencies** (pure Go; versions in `go.mod`, rows in `THIRD_PARTY.md`):
  `ebitengine/purego` (`ffi` on unix, `shell/win32/com` for float-argument COM calls,
  `spike/appkit`), `golang.org/x/sys` (Windows packages), `godbus/dbus/v5` (`shell/gtk4`
  only: the settings portal), `rivo/uniseg` (`aqua/text` only: UAX #29).

### Tooling

* **`scripts/check.sh`**: every per-step check of PLAN.md §11.1, in parallel phases, stopping
  at the first failing phase: gofmt; vet and build for the six GOOS/GOARCH targets;
  staticcheck (pinned in the script) per GOOS; `go test -count=1 ./...`; the **no-cgo guard**
  (empty `CgoFiles`, no `import "C"`, no C/C++/ObjC/asm/SWIG files); the **hidden-callback
  guard** (no `syscall.NewCallback`, `windows.NewCallback`, `purego.NewCallback`,
  `objc.NewIMP`/`NewBlock`/`RegisterClass` outside `internal/ffi` and tests); the **stress
  run** (`-count=20 -cpu=1,2,4,8` over the cross-thread code: `.`, `mainthread`, `ffi`,
  `headless`, `shell/fake`, aqua's `TestDriver*`, win32's run/post/wake/timer/clipboard
  tests). The source scans share one exclude list (`.git`, `native-logs`, `screenshots`);
  a failing tool (`gofmt`, `go list`, a grep error) fails its phase, never reads as "nothing
  found". About 3 min; `-quick` skips the stress run.
* **`scripts/native.sh`**: what needs the target OS, `CGO_ENABLED=0`: vet, tests, example
  builds, callback measurements, frame benchmarks; on Windows verbose steps for the goldens,
  IME, text, popups, menu strip, modal surfaces, appearance, native frame benchmarks, frame
  pacing and idle CPU; on Linux the gtk4 tests and a 5 s smoke run of every example under
  xvfb (X11) and headless weston (Wayland), inside `dbus-run-session`, plus the desktop
  session if present (without goldens and clipboard); on macOS `spike/appkit -selftest`.
  The examples are every folder of `examples/`, each built once into a temporary directory
  that a `trap` removes. Every step runs; the output goes through `main | tee` to
  `native-logs/<goos>-<goarch>.log` (not committed); tests print `*-RESULT` lines. Bash 3.2
  compatible.
* **`scripts/idlecpu.sh`**: an example's process CPU over 10 s after start-up; on Windows
  `hello`, `form`, `notes` fail above 1 % of a core, the rest is logged only. The binary
  reaches PowerShell through `IDLECPU_BIN`; the app is stopped however the script ends.
* **`docs/parity.md`**: the v0.1 parity checklist (PLAN.md §8.3), a scope × target matrix
  and every open check by ID (M/W/L/A/G/C); `TestParityChecklist` keeps it consistent.
  `docs/tokens.md` is generated with the theme (§9.1). `README.md` and `examples/README.md`
  describe the state as built (`TestExamplesListed` checks the latter).
* `.gitattributes` forces LF. No CI (PLAN.md §8.4).

### Spikes

Throw-away, never imported, build-tagged; results in [`docs/spikes.md`](docs/spikes.md).
Only **`spike/appkit`** (darwin) is left, until the appkit driver replaces it: AppKit via
`purego/objc` with classes built through `objc_allocateClassPair`/`class_addIvar` (no
`objc.RegisterClass`, which hides a callback per method), a `goHandle` ivar holding an
`ffi.Handle`, three shared trampolines, layout-tested `NSRect` mirrors, and
`performSelectorOnMainThread:` for cross-thread work. `-selftest` exercises it.

### 1.1 `gogui`: the public package

The v0.1 API; conventions in PLAN.md §4.2; `doc.go` is the package overview, and
`TestEveryExportDocumented` requires a doc comment (with driver differences) on every export.

* **Start-up.** `init` locks the main OS thread. `Run(cfg, ready)` works once per process.
  It takes the driver from the `newDriver` hook, `newPlatformDriver() (driver.Driver, error)`:
  headless on darwin (`driver_darwin.go`), else `aqua.New(native.New())`, or the
  `native.New` error, which `Run` returns without calling `ready` (`driver_other.go`). Only
  these two files import a backend; every other non-test file depends on `internal/driver`,
  `layout` and `mainthread` alone (`TestDriverBoundary`, R7). After the
  driver returns, `Run` destroys every widget still alive (`destroyLeftovers`: each tree from
  its topmost live ancestor, so a widget with a parent is never skipped, B3).
* **`App`** is the `driver.AppHandler`: `Ready` makes the app current (`cur`, so
  constructors and `Do` work from the ready callback on, not while the driver starts, A6),
  enables UI-thread checks with `GOGUI_DEBUG=1` and calls `ready`; `OnQuitRequested` handlers run in order, the first
  `false` vetoes (`App.Quit` doesn't ask); `OnAppearanceChange` fires only on a real
  Light/Dark switch. With `GOGUI_DEBUG=1` on a driver with a frame clock (aqua; through
  `Driver.ObserveFrames`), `framelog.go` logs every frame over the
  1/120 s budget with its split.
* **`Do`/`DoSync`** forward to the app's queue (§4); without an app they drop / return false.
  A panic in a `DoSync` function is always a `*PanicError` (alias of the `mainthread` type).
  Every public method begins with the UI-thread check (panics when checks are on).
* **Value types alias** the `driver` types (`Point`, `Size`, `Rect`, `Insets`, `Color`,
  `Font`, `Modifiers`, `Key`, `KeyShortcut`, `ControlSize`, `WindowConfig` …), so nothing is
  converted at the peer boundary. `Appearance` is `gogui`'s own type (System/Light/Dark).

**Widgets** (`widget.go`): `Widget` is sealed; each widget embeds `widget` (peer, layout
node, parent, `self`, destroyed/disabled flags, the tooltip) and `common[T]` (`SetEnabled`,
`SetHidden`, `SetTooltip`/`Tooltip`, `SetControlSize`, `Focus`, returning `T`). `Widget.Destroy`
is on `widget`, so every widget (`Spacer` included) has it.

* Constructors panic outside `Run` and create the peer immediately.
* Size-changing setters call `invalidate()`, which reaches the window's deferred layout.
* **Lifetime (D13).** `destroy()` is idempotent and frees only the own peer; trees are
  destroyed bottom-up (`destroyTree`). `Destroy()` = `detach` + `destroyTree`. Replacing or
  removing content (`SetContent`, `SetDocument`, `Stack.Remove`/`Clear`) never destroys it.
  `check(method)` panics on a destroyed widget (setters, `On*`, actions); getters use
  `get(method)` and return the last value. `adopt` and `Window.SetContent` panic on a
  destroyed child or parent (B3). Dispatch (`eachLive`) stops once a handler has destroyed
  the widget.
* Default expansion by kind: text fields and progress bars horizontal, text views both,
  sliders/separators along their length, scroll views on scrolling axes; containers only via
  children; other controls none.
* `On*` returns `Unsubscribe` (idempotent); dispatch iterates a snapshot. Events are dropped
  on disabled or destroyed widgets. A change the native layer reports for a disabled widget
  still updates the cached `Text`/`Value`/`State`/`Selected`; only the handlers are skipped
  (nothing at all after destroy). **Setters never fire events.**

**Controls and containers** (public state the peer doesn't own lives here):

* `RadioGroup` is a container of private radios and owns exclusivity: first item selected
  initially, out-of-range `SetSelected` selects none (-1), `OnChange` only on a real change.
* `Checkbox`: mixed counts as on. `Slider`/`ProgressBar`/`Spinner` keep AppKit's default
  ranges (0…1, 0…100) and clamp values (NaN → min). `PopUpButton` selects item 0 initially,
  keeps an in-range selection across `SetItems`, drops out-of-range native indices.
* `ScrollView` keeps peer scrollers, layout node and expansion in sync; `OnScroll` fires also
  while disabled (as in AppKit). `TextView` selections are UTF-8 byte offsets; the peer's
  `TextChanged` carries no text, so `Text()` is read once per edit.
* `VStack`/`HStack`, `Spacer`, `Form`/`Row`, `Box`, `ScrollView`: each change re-syncs the
  container peer's `SetChildren` (in order; forms: label then control) and the layout node.
  Adding a widget elsewhere first detaches it; nil, duplicate and cyclic children panic. A
  control moved out of (or destroyed in) a form takes its row's label with it; hiding it
  hides the label. `FormRow` is plain data (label text, control); `Form` creates each row's
  label, so a reused row gets one label per form (A4). `Stack.SetPadding`/`SetPaddingInsets`
  (A2), `Remove`/`Clear` detach only, `Children()`. Batching (review P19): `Add` sends only
  the appended children (`layout.Stack.AppendChildren`, and the container peer's
  `AppendChildren` where it has one: aqua and stub; else `SetChildren`), and moving or
  removing many children from one stack or form syncs it once (`detachAll`,
  `batchRemover`), so building or emptying a stack of n children is O(n).

**Menus and alerts.** `SetMenuBar` gives each action item a fresh, never-reused ID (role
items, separators and action-less items get 0; action-less items are disabled). `Alert.Show`
defaults to `["OK"]`, maps `Alert.Cancel` to Escape (zero: the first button titled
"Cancel"; `AlertNoCancel` or out of range: none; index 0 is the default button, A5), and is app-modal for a
nil or closed window. `SetAccentColor(ColorControlAccent)` clears the override.

**Windows.** `App.NewWindow(cfg)` creates the hidden peer and panics outside `Run`
(`driverFor`, A6). Layout is deferred and coalesced
(one pass posted per batch of invalidations; a resize lays out at once; `Show` flushes);
each pass uses `Root.LayoutMin` and updates the peer's min size to
`max(cfg.MinSize, content minimum)` only on change. `OnShouldClose` can veto a user close
(not `Close`/`Quit`); on `Closed`, `OnClose` runs, then the tree is destroyed and the window
drops its content, root and handlers (A9; `Content()` is nil, later `On*` store nothing).

**Tests** swap `newDriver` for a silent headless driver and call `Run` on the test goroutine
(`runApp(t, body)`).

## 2. `internal/driver`: the peer boundary

AWT/SWT-style peers: one interface per public widget, created by a `Driver` factory, with a
handler interface through which the backend reports user and OS events. The package docs hold
the full contracts; PLAN.md §4.3 has an excerpt.

### 2.1 Interfaces

* **`Driver`**: `Name`, `Run(cfg, AppHandler)`, `Quit`, `Queue() *mainthread.Queue` (nil
  before `Run`, closed after `Quit`; a compile-time part of the interface, so `gogui` needs
  no type assertion, R7), `ObserveFrames(fn) (now)` (the frame-time hook; nil `now` when the
  driver has no frame clock, as headless), `Appearance`,
  `SetAppearance`, `SetAccentColor` (zero clears; semantic colors resolve per appearance),
  `SetMenuBar`, `RunAlert(parent, spec, done)`, `NewWindow`, and one factory per peer
  (`NewContainer`, `NewLabel`, `NewButton`, `NewCheckbox`, `NewRadio`, `NewTextField(secure)`,
  `NewSlider`, `NewProgress`, `NewPopUpButton`, `NewSeparator`, `NewBox`, `NewScrollView`,
  `NewTextView`).
* **`AppHandler`**: `Ready`, `AppearanceChanged`, `QuitRequested() bool`,
  `MenuItemSelected(id)` (items carry integer IDs, not funcs).
* **`Widget`**: `SetFrame`, `FittingSize(max)`, `SetEnabled`, `SetHidden`, `SetControlSize`,
  `SetTooltip`, `Focus`, `Destroy`. **`Container`** adds `SetChildren` (order = paint and
  focus order; dropped children are removed, not destroyed).
* **Controls** each pair a peer with a handler (`ButtonHandler.Clicked`,
  `ToggleHandler.StateChanged`, `TextFieldHandler`, `SliderHandler`, `PopUpHandler`,
  `ScrollHandler` with `Scrolled`/`ViewportResized`, `TextViewHandler.TextChanged()` without
  argument). `Box` exposes `ContentInsets`.
* **`Window`**/`WindowHandler` (`ContentResized`, `ShouldClose`, `Closed`, `KeyChanged`); a
  window is not a `Widget` and holds one content widget.

### 2.2 Shared value types

* Geometry `Point`, `Size` (`Unbounded()` = +Inf), `Rect` (top-left origin, half-open),
  `Insets`. `Color`: straight-alpha RGBA or one of 39 NSColor-named semantic colors; zero
  means "default" where documented. `Font`: size 0 = the control size's system size
  (13/11/9/13 pt for regular/small/mini/large).
* `Appearance`: dark, accent, increase contrast, reduce motion/transparency, legacy
  scrollers, `CaretBlink{Period, Timeout}` (0 = no blink / never stops).
* Keys: `Modifiers.Primary` (⌘ on macOS, Ctrl elsewhere), `Key` runes with AppKit values for
  special keys, `Shortcut.Display(mac)`.
* Menus: `MenuBarSpec` → `MenuSpec` → `MenuItemSpec` with roles and default items; a role
  item with a non-zero ID is reported instead of running its standard action.
* `AlertSpec` (first button default, `Cancel` index), `WindowConfig`, `AppConfig`.

`ControlSize`, `WindowStyle` and `CheckState` match AppKit bit for bit (tested);
`AlertStyle` deliberately does not.

### 2.3 Contracts every backend follows

* **Units**: logical points, top-left origin, frames relative to the parent's content area.
* **Threading**: all peer and handler calls on the UI thread, except `Driver.Queue` (any
  goroutine; its `Do`/`DoSync` too, ordered). `Run` on the locked main thread; factories
  only after `Ready`.
* **Setters never call handlers**, except documented consequences: `Window.Close` →
  `Closed`, `SetContentSize` → `ContentResized`, `SetAppearance` → `AppearanceChanged`.
  `Quit` closes all windows without `QuitRequested`.
* **Getters only for user-changeable state**; the public layer remembers the rest.
* **Layout is in Go**: peers report `FittingSize` and receive `SetFrame`.
* **Scroll viewport** depends only on frame, scrollers and appearance (no auto-hide); a change
  without `SetFrame` is reported as `ViewportResized`.
* **Parenting**: at most one parent; `Destroy` detaches and frees the widget, not its
  children; nothing may be called after it.
* **Text offsets** are UTF-8 byte offsets, clamped to rune boundaries.
* Peers never store Go pointers in native memory (§5).

## 3. `internal/driver/stub`: fallback and test peers

Every peer interface has a stub (checked in `assert.go`). Stubs record state in exported
fields (setters only record; sliders/progress clamp) and offer `User…` methods that simulate
input through the handler like the real control (ignored while disabled, except scrolling).
`Base` tracks parents like real drivers. `stub.Driver` is not a `driver.Driver` by itself: it
has the factories, appearance, menu bar, alerts and `UserSelectMenuItem`, but no run loop,
queue or `Quit` (R7); the headless driver embeds it and adds them.

**Fake metrics** (`metrics.go`) are stable on purpose; layout and public-layer tests depend
on these numbers:

| Item | Fake size |
|---|---|
| Text | 7 pt per rune, 16 pt line height at 13 pt, scaled by font size |
| Control height | 22 regular, 19 small, 16 mini, 28 large |
| Button | text + 28 × control height |
| Checkbox, radio | 18 + text × 18 |
| Text field, slider, progress bar | 96 × control height (vertical slider swapped) |
| Spinner | 32 × 32 |
| Pop-up button | widest item + 32 × control height |
| Separator | 1 × 1 |
| Box | its content insets: 8, plus 16 on top when titled |
| Container, scroll view, text view | 0 × 0 (sized by the layout) |
| Scroll viewport | the frame size minus `ScrollView.ViewportInset` (zero by default) |

Not simulated: key-status moves on close, min/max re-clamping, Return/Escape default and
cancel buttons, menu role actions.

### 3.1 `internal/driver/headless`: the display-less driver

`headless.New(notice)` embeds `*stub.Driver` with a `mainthread.Queue`. It is the macOS
driver until appkit exists and the `gogui` test driver. `Run` (once) binds the queue, prints
`headless.Notice`, calls `Ready` and drains on every wake until `Quit`. An un-quit `Run`
blocks forever like a native loop. `Quit` closes the windows (each gets `Closed`) and the
queue. Panics in `Ready` or queued work propagate out of `Run`, and the queue is closed on
the way out. `ObserveFrames` returns nil (no frame clock).

## 4. `internal/mainthread`: the UI-thread queue

`New()` creates a `Queue` (no zero value, no global); each driver owns one.

| Method | Meaning |
|---|---|
| `Bind()` / `Bound()` | Marks the calling goroutine as the UI goroutine. |
| `SetWake(fn)` | Native-loop wake hook; called on the producer, at most once per `Drain`, at once if work is pending. Must not block or drain. |
| `Do(fn) bool` | Queues fn (FIFO), even from the UI goroutine; `false` after `Close`. |
| `DoSync(fn) bool` | Inline on the UI goroutine; elsewhere queues and blocks. A panic is re-raised as `*PanicError` on both (a nested one is not wrapped again). `false` if closed before fn starts. |
| `Drain() int` | Runs a **snapshot** (work queued meanwhile waits for the next drain, so self-requeuing work can't starve the loop). Re-entrant. |
| `Close()` | Rejects new work, releases `DoSync` waiters; queued `Do` work still runs. |
| `OnUIThread()`, `Check(what)`, `SetChecks(on)` | Debug guard; off by default (one atomic load). |

Lock-free MPSC queue (Vyukov) with coalesced wakes. Gotchas: a panic in `Do` work leaves
the rest queued and re-requests a wake; `DoSync` on the UI goroutine **before `Bind`**
deadlocks, so drivers `Bind` before any user code runs.

## 5. `internal/ffi`: FFI helpers

**Go pointers are never stored in native memory** (PLAN.md §7.2). Native objects (ObjC
ivars, GObject `user_data`, `GWLP_USERDATA`) store an integer `ffi.Handle`.

* **`Handles[T]`**, one per driver: `New`, `Get`, `Valid`, `Free`, `Len`. A handle is a slot
  index plus a generation, so stale handles from late native events don't resolve. The zero
  handle is never valid. Handles carry no table ID, so tables must never be mixed. The table
  is mutex-guarded; drivers use it on the UI thread. Never turn a handle into an
  `unsafe.Pointer`.
* **Callbacks**: `RegisterCallback(name, fn) uintptr` is the only way to create native
  callbacks. Drivers register a fixed set of trampolines once at start-up and dispatch
  through a handle. It panics before creating anything on a duplicate name (this catches
  per-widget callbacks), a signature outside the `syscall`/purego intersection, or an
  exhausted budget. **`MaxCallbacks` = 1024 on every OS.** `CheckCallback(fn, goos)`
  validates a signature for any OS. `Bind`/`BindAddr` reject func-typed arguments (purego
  would make a hidden callback per call). A callback runs on whatever thread calls it.
* **Struct layouts**: `CheckLayout` and `CheckNoGoPointers` let tests check mirrored C
  structs against header constants per GOARCH, without cgo.
* **Unix libraries**: `Dlopen`, `Lib.Sym`/`MustSym`/`Bind` (errors, not panics), `BindAddr`
  for bare function pointers (GObject vfuncs). `CString`/`Free`/`GoString[N]` use libc
  memory.
* **Raw native memory**: `Read/WriteUintptr`, `Read/WriteInt32`, `At[T]` (a native struct in
  place, such as a Win32 `lParam`), `CopyBytes`, `WriteBytes`; NULL or misaligned addresses
  panic. `cptr` is the package's only uintptr→`unsafe.Pointer` conversion, so callers need no
  `//nolint`.

## 6. `internal/layout`: the layout engine

One Go engine lays out every driver's widgets (complete for v0.1).

* **`Node`**: `Props`, `Expands(Axis)`, `Measure(p, max)`, `Arrange(p, frame)`. `Measure`
  shrinks the natural size towards the minimum when `max` is smaller, **never below the
  minimum**, so the result may exceed `max`. The frame given to `Arrange` is final.
* **`Pass`** caches measurements by node and max; children are measured only through
  `p.Measure`, which skips hidden nodes. It also memoises per node the `Expands` answers
  (`p.Expands`; stacks, forms and boxes ask their children through it, so a tree is walked
  once per pass) and a leaf's unbounded `FittingSize`, and lends the nodes LIFO scratch
  slices (spans, distribute's sizes, form rows) (review P7). Entries keep their memory
  across `reset`. **Invariants:** a tree must not change while a `Pass` that measured it is
  in use, and a `Root`'s methods must not be called from inside its own pass.
* **`Props`** per axis: min, max, fixed, align, expand, hidden. Bad values → 0, +Inf = unset,
  min wins over max. `Align`: default, leading, center, trailing, fill. RTL is reserved
  (laid out as LTR).
* **`Leaf`** wraps an `Item` (`FittingSize` + `SetFrame`, which every peer satisfies). Its
  default min is the fitting size, so a control never shrinks unless the app sets a min.
  The height is re-measured at the resolved width (wrapping labels).
* **Distribution**: extra space goes equally to expanders (capped by max, the rest
  re-shared); short space shrinks in proportion to (natural − min), never below the minima.
  Nothing is rounded; drivers snap edges.
* **`Stack`**: children get exactly their main-axis share. Across, a child uses its explicit
  alignment, else `Fill` if it expands, else leading (VStack) or center (HStack). With a peer
  the stack starts a new coordinate space; without one it is a transparent group. A `Spacer`
  expands on its stack's main axis. `AppendChildren` appends without copying the old list (P19).
* **`Form`**: right-aligned labels in a column as wide as the widest label (never shrunk or
  wrapped), controls `FormLabelGap` (8 pt) after it, rows `FormRowSpacing` (8 pt) apart. A
  row hides with its control; each label is centered on its control's first line. The
  constants are still to be checked against the HIG.
* **`Root`**: `LayoutMin(size)` lays the tree out and returns its minimum size in one pass
  (width at minimum, then the natural height at that width, so content is never cut off);
  windows use it. The tests split it into `Layout` and `MinSize` (`export_test.go`).
  A root keeps one `Pass` and clears it at the start of each call, so a warm pass allocates
  nothing (P7).
* **`Box`** re-reads the peer's `ContentInsets` on every pass. **`ScrollView`** measures
  only its own props. It frames the peer, reads `ViewportSize`, and sets the document size
  to max(measured, viewport) on scrolling axes and the viewport elsewhere. It expands only
  by its own flag.

Nodes are UI-thread only. They call only `FittingSize`/`SetFrame` on leaves, plus
`ContentInsets`, `ViewportSize` and `SetDocumentSize` on box and scroll peers.

## 7. `internal/render`: drawing vocabulary and the recorder

What aqua hands to a canvas (PLAN.md §5.3). It depends only on `driver` and has no build
tags.

* Geometry and colors alias `driver` (float64 logical points, Y down, straight alpha).
  Angles are radians, clockwise. `Radii` (per corner), `Paint` (solid or linear gradient),
  `StrokeStyle`, `Shadow` (CSS box-shadow semantics, σ = Blur/2), `Path` (HTML-canvas
  subpath rules; tight `Bounds`, also `SegmentsBounds(segs)` for recorded segments, both
  allocation-free; `Op.Points()` is the number of `Segment.Pts` a kind uses), `Transform`
  (2×3 affine; zero ≠ identity; `Then` applies
  the receiver first, `Pre` the argument first), `Image` (`Size()` only).
* **Validation** (`validate.go`) is applied by backends and the recorder, never by
  constructors; callers pass values as built. **Semantic colors must never reach a canvas.**
  Geometry must be valid: `ValidRect` (finite, far edges included, size ≥ 0), `ValidPoint`
  (finite) and `ValidPath` (every point finite) are the shared rules of all three canvases;
  an invalid call draws nothing, and an invalid clip or backdrop-blur rect clips everything
  away. `StrokeWidth` maps negative, NaN and ±Inf to 0. `Path.ArcTo` falls back to a line to
  p1 for non-finite points, so NaN never becomes cubics. `Paint.Normalized` and
  `StrokeStyle.Normalized` return the caller's stops and dashes when they already follow
  the rules (no allocation per draw call) and a fixed copy otherwise; the caller's slices are
  never changed.

### 7.1 `Canvas` (`canvas.go`)

* Used on the UI thread within one paint call. The state is transform, clip and opacity,
  starting in logical points (the backend applies the scale) with a full clip.
* `Save`/`Restore` and `PushOpacity`/`PushBackdropBlur`/`Pop` nest strictly and are all
  closed before the paint returns. Violations, semantic colors, non-finite transforms and invalid geometry are
  bugs: the recorder reports them and backends skip the call.
* Drawing calls: rounded rects (fill and stroke), paths (nonzero fill, stroke), `DrawShadow`
  (the shadow only), `DrawText(layout, origin, color)` (first baseline at
  `origin.Y + Baseline()`), `DrawImage`.

### 7.2 `TextSystem` and `TextLayout` (`text.go`)

* `shell.Services.Text.NewLayout(text, font, maxWidth)` breaks at `'\n'` and wraps at words
  for a positive finite width. A font size of 0 means 13 pt. `shellkit.FixedText` is a fixed-width
  system.
* `TextLayout` is immutable and relative to its top-left. It offers `Size`, `Baseline`,
  `LineCount`, `HitTest`, `CaretRect` and `CaretRectUpstream` (the two sides of a soft
  break), `SelectionRects` (one per line) and `Line(i)` metrics (the lines tile the layout).
  Indices are UTF-8 byte offsets, snapped to cluster starts and clamped.

### 7.3 `render/record`: the recording canvas

`record.New(size)` records every drawing and layer call as an `Op` with normalized values
and the resolved transform, clip box and opacity. The ops' path segments, gradient stops
and dash patterns live in three arenas of the canvas (one capped sub-slice per op, so a
later op never writes into an earlier one; normalized stops and dashes are copied there,
so a recording never aliases the caller's slices); `Op.Bounds` is `render.SegmentsBounds`
under the op's transform. Contract violations become numbered
errors (the call is ignored; invalid geometry included, so a NaN from layout fails a recorder test instead of passing and drawing nothing on screen); `End()` reports unclosed pairs. `Dump()` prints one stable line
per op (format in `dump.go`). The aqua frame-dump tests pin it.

## 8. `internal/shell`: the OS boundary of the aqua driver

Interfaces and value types only (`shell.go`, `surface.go`, `popup.go`, `event.go`, `clock.go`, `pacing.go`), no build tags, depends only on `internal/driver` and `internal/render`. Implementations: `shell/fake` (§8.1), `shell/win32` (§8.2–8.3), `shell/gtk4` (§8.4–8.5); the IME path across them is §8.6. `internal/shell/native` picks one per OS (`native.New()`: `win32.New()` on Windows, `gtk4.New()` on Linux, `ErrNotImplemented` elsewhere); `gogui`'s `driver_other.go` builds the aqua driver on it.

**Shared contracts**
- All geometry is in logical points, top-left origin, relative to the surface's drawable area (the whole client area, title bar included). The surface reports `Scale`; the backends apply it.
- Every method and handler call is on the UI thread, except `Shell.Post`.
- Modifiers in events are physical: `ModPrimary` never appears; `ModOption` = Alt, `ModCommand` = Win/Super.
- Event times and frame times share the `FrameClock.Now` timeline.
- The shell never calls into Go for `WM_NCHITTEST` (it uses the stored chrome regions) and paints at most once per frame.

**`Shell`**
- `Run(Handler)` on the locked main thread, once: calls `Ready`, then returns nil after `Quit` (or an error, e.g. GTK missing).
- `Quit` marks the shell quitting, then closes the open surfaces (`Closed`, no `CloseRequested`); a second `Quit` does nothing, and `NewSurface` after it (also from a `Closed` handler) returns the shell's `ErrQuit`. Every shell, the fake included.
- `Post` works from any goroutine: FIFO, coalesced wake (the contract of `driver.Driver.Queue().Do`).
- `NewSurface(cfg, h)` creates a hidden surface. Also `Appearance()` (the OS setting only; forcing light/dark is the aqua driver's job). `Handler` gets `Ready(Services)` and `AppearanceChanged(a)`: `Ready` hands over the text system and the frame clock once (`Services{Text, Clock}`; the shell has no getters for them, the concrete shells keep `TextSystem()`/`Clock()` as test accessors).
- `Caps()` (fixed, any goroutine, before `Run` too) returns `Caps{PopupShadow}`.
- Popups are optional: a shell with them also implements `PopupShell` (`Shell` plus `NewPopup`). All three shells do; on a shell without it aqua opens no menus or tooltips (`openPopup` fails with `errNoPopups`).
- Clipboard: `SetClipboardText(s)` sets plain text (a failure is dropped). `ReadClipboardText(fn(text, ok))` always delivers later from the shell's queue, never inside the call, because GTK reads asynchronously.
- `After(d, fn) (cancel)` is a one-shot timer. `fn` runs once on the UI thread at least `d` later, never inside the call. Timers fire in (deadline, creation) order. `cancel` does nothing once `fn` has started. Timers still pending at `Quit` are dropped. It is for work that must not keep the frame clock running (the caret blink).

**`SurfaceConfig`**: `Title` (the native title for the taskbar and Alt-Tab; aqua draws the visible one), `Size` (the drawable area including the title bar, unlike `driver.WindowConfig.Size`), `MinSize`/`MaxSize` (zero = no limit), `Style` (without `StyleResizable` there is no resize, maximize or Snap), `Center`, `Modal`, `Owner`.
- **Modal surfaces:**
  - No taskbar entry; never resizable, minimizable or zoomable.
  - Each placement (first `Show`, `Center`) centers the modal on `Owner` if that is shown and not minimized, else on the key surface's screen, kept inside the work area.
  - From its first `Show` until it closes, a modal **blocks** every other surface: no input, no user move, resize or close, and the OS activating a blocked surface makes the modal key instead. Blocking ends a grab popup and the capture (a `PointerUp` per held button, then `PointerLeave`). Programmatic calls and paints still work.
  - A closing key modal hands the key to the next modal, else to `Owner`, else to the surface that was key before it. Win32 hands over before `Closed`; fake and gtk4 after. On every shell the modal leaves modality (the others are unblocked) before its `Closed`.
  - Closing an owner closes its modals first.
  - `Owner` must be nil without `Modal`; otherwise it must be an open surface of the shell (`ErrSurfaceOwner`).

**`Surface`**
- `SetTitle`; `Size`/`SetSize` (clamped, `Resized` before it returns); `SetSizeLimits(min, max)` (both at once: one clamp against both and at most one `Resized` if the current size breaks them); `Scale` (valid from `NewSurface`).
- `Center`; `Show` (to the front and key); `Minimize`; `Zoom`/`Zoomed`; `Close` (no `CloseRequested`, then `Closed`, idempotent; closing the last surface does not quit).
- `Invalidate` repaints the whole surface: one `Paint` per frame. Called from inside the frame function it requests no new frame and joins that frame's paint pass, so an animation ends without an extra empty frame.
- `SetCursor`, `SetChromeRegions`.
- IME: `SetTextInput(TextInput{Enabled, Caret})` turns the IME on only while `Enabled` (it starts off). `Caret` places the candidate window. Disabling drops a composition without events; the call is allowed during `Paint`. `CancelComposition()` drops the composition without any event, leaves the input enabled, and is not allowed during `Paint`. Neither calls the handler.
- After `Closed` every method does nothing.

**Chrome regions** `{Rect, Kind}`
- `RegionClient`.
- `RegionCaption`: a primary press moves the window and a double click maximizes. These presses are not delivered.
- `RegionClose`/`RegionMinimize`/`RegionMaximize` (the traffic lights): their pointer events are delivered normally and aqua calls `Close`/`Minimize`/`Zoom` itself. `RegionMaximize` gives Windows 11 Snap Layouts.
- Regions are tested last to first and the first hit wins; no hit means client. So aqua lists the title bar first, then the menu titles as client (PLAN.md §5.4.1), then the buttons.
- The slice is copied; nil means all client. The shell's resize borders win over every region.

**`SurfaceHandler`**
- `Paint(render.Canvas)`: draw only; `Invalidate` is allowed.
- `Resized(size, scale)`: not sent for the initial values. In a live resize the shell paints before returning to the OS.
- `CloseRequested() bool`: Alt+F4, taskbar, logout. Aqua's own close button calls `Close` instead.
- `Closed` (once), `KeyChanged(bool)`, `Event(Event) bool`.

**Events** (a closed set: `EventTime()` plus an unexported marker)
- `PointerEvent{Kind, Pos, Button, Buttons, Clicks, Mods, Activating, Time}`: move, down, up, enter, leave.
  - `Buttons` is the set of held buttons after the event.
  - The surface keeps an implicit capture while a button is held. If the OS cancels it, a `PointerUp` comes for each held button.
  - Moves may be coalesced to the latest before each frame.
  - `Activating` marks the `PointerDown` that made a non-key surface key. Fake and Win32 send it after `KeyChanged(true)`; on gtk4, `KeyChanged(true)` may come after it.
  - A press whose activation the OS hands to a modal is not delivered.
- `ScrollEvent{Pos, Delta, Precise, Phase, Momentum, Mods, Time}`:
  - Natural direction: positive Y moves the content down.
  - The delta is in points when precise, in lines otherwise.
  - Gesture phases are began/changed/ended. OS momentum phases follow on GTK; there are none on Windows (aqua runs its own). Wheel notches have no phase.
- `KeyEvent{Down, Key, Shifted, Modifier, Repeat, Mods, Time}`:
  - `Key` is a canonical `driver.Key`; `KeyNone` for a lone modifier.
  - `Modifier` is the one modifier a modifier key stands for, on its press and release, and 0 otherwise. It tells a lone Alt from other `KeyNone` keys.
  - `Mods` is the state after the event.
  - `Shifted` (key-downs of character keys only) is the character the key gives with Shift alone on the active layout, normalized and read without disturbing a pending dead key. Menu accelerators use it.
  - Order: key-down, `TextEvent`, key-up. A handled key-down drops its text and the OS default handling.
- `TextEvent{Text, Time}`: committed, non-empty UTF-8 without control characters (typing, dead keys, IME commits).
- `PreeditEvent{Text, Cursor, Segments, Time}`: the IME composition, only while text input is enabled.
  - Byte offsets fall on rune starts. `Segments` (`{Start, End, Selected}`) are ordered and do not overlap; `Selected` is the target clause.
  - Each event replaces the preedit; an empty `Text` ends the composition (and may come when none runs).
  - A `TextEvent` during a composition is its commit and empties the preedit. It may come before or after the ending empty preedit; on Win32 both come in one message, commit first.
  - Keys the IME takes are not delivered.

**`FrameClock`**
- `Now()` is monotonic. `SetFrameFunc(fn(frameTime))`: the frame time is the target presentation time. `RequestFrame()` is one-shot and coalesced; animations request again from inside the frame function.
- A frame runs the frame function, then `Paint` once for each invalidated surface. No requests means no frames.
- **Off screen:** if surfaces are open but none is on screen (all hidden or minimized, so no popup is shown either), requested frames start at least 250 ms apart (`hiddenInterval`; the fake's `HiddenInterval`). Frame times keep moving, and showing or restoring a surface ends the low rate at once. With no surface at all the vsync rate stays. A fully occluded Win32 window keeps vsync (a composition chain reports no `DXGI_STATUS_OCCLUDED`).
- **Idle inventory:** every source of frames and timers in the shells and the aqua driver is listed in `shellIdleInventory`/`idleInventory` (`internal/aqua/idle_test.go`). `TestIdleInventory` scans the code for them.
- **`SetFrameObserver(fn(FrameTiming))`** gets each frame's wall times on the UI thread after the frame (`shell.FrameTiming` is an alias of `driver.FrameTiming`, so the aqua driver's `ObserveFrames` installs the observer as is):
  - `Time` is the target, or -1 for a paint outside a frame (a synchronous resize paint, a redraw GTK runs itself).
  - The phases are `FrameFunc`, `Paint`, `Submit` (canvas setup and finish; on Win32 `BeginDraw`…`EndDraw`) and `Present` (Win32 `Present` + `Commit`, which may wait for the display), plus `Surfaces`.
  - `shell.FrameTimer` (`frametiming.go`) is shared by all clocks. Win32 measures with QPC, because `time.Now` on Windows has coarse resolution.
  - Nothing is measured without an observer.
- **`SetPresentObserver(fn(PresentTiming))`** reports, some frames after the present, when the display showed it: `Target`, `Shown` (vblank on the `Now` timeline), `Period`, `Refresh` (vblank count, 0 on gtk4).
  - Win32 maps presents by `GetLastPresentCount` and reads `GetFrameStatistics` after each `Present`.
  - gtk4 maps GDK frame counters to `gdk_frame_clock_get_timings`.
  - The fake reports each paint at once, shown on its target.
  - Reports may be skipped. Without an observer no feedback is read.
- **`shell.Pacing`** turns both observers into a run's numbers. A vblank counts as missed when a running animation showed no new frame at it. `PacingTolerance`/`PacingRunTolerance` are the verdict of the pacing run.

**Cursors**: `Arrow`, `IBeam`, `PointingHand`, `ResizeLeftRight`, `ResizeUpDown`, `Crosshair`, `NotAllowed`, `Hidden`.

**Popups** (`popup.go`)
- `PopupShell.NewPopup(parent, PopupConfig, PopupHandler) (Popup, error)` creates and shows a borderless surface owned by `parent`.
  - Never in the taskbar, never key.
  - Closed with its parent (before the parent's `Closed`) and at `Quit`.
  - Not allowed during `Paint`.
  - Errors: `ErrPopupParent` (the parent is not an open, shown, not minimized surface of this shell), `ErrPopupGrab`.
- `PopupConfig{Anchor, Size, Offset, Adjust, Grab, Track}` follows Wayland's `xdg_positioner` without flips: top-left = anchor top-left + `Offset` (parent points). When the popup does not fit the anchor screen's work area, `Adjust` (`AdjustSlideX`, `AdjustSlideY`, `AdjustResizeY`) allows sliding first, then cutting the height.
- `PopupHandler`:
  - `Placed(rect, scale)`: parent points; called before the first paint and on every change, never during a paint. Fake and Win32 call it inside `NewPopup`/`Reposition`, gtk4 later; the fake's `SetDeferPlaced` posts it (coalesced, no paint before it) to test that path.
  - `Paint`: painted after the surfaces in a frame.
  - `Event` (popup coordinates).
  - `Dismissed()`: grab popups only, at most once, after the grab ended. The popup stays shown until `Close`.
  - `Closed()` once.
- `Popup`: `Invalidate`, `SetCursor`, `Close` (no `Dismissed`).
  - `Reposition(cfg)`: `Grab` and `Track` stay as at creation. A grab popup's pointer stays put on screen and later events use the new position's points.
  - `Translucent()`: whether the OS blurs behind it. It changes only with the appearance, before the popup is invalidated and `AppearanceChanged` runs.
- **Grab** (at most one per shell):
  - Opening it cancels the parent's capture (`PointerUp` with `Clicks` 0 per held button, then `PointerLeave`). The buttons stay held for the popup, so a release arrives as a `PointerUp` without a `PointerDown` (press-drag-release).
  - All pointer events go to the popup, none to the parent: enter/move/leave over its area, moves anywhere while a button is held, presses and scrolls over its area.
  - A press outside the area is eaten together with its release and dismisses the popup.
  - The area is the popup plus `Track`, a rect in parent points (a menu strip). Moves, presses (delivered, not dismissing) and scrolls over the track reach the popup in its coordinates. Enter and leave apply to the whole area. An empty or non-finite `Track` means none; without `Grab` it is ignored. On gtk4 a press on the track dismisses instead (GDK's autohide).
  - The parent's key, text and preedit events go to the popup while the parent stays key.
  - The OS dismisses the popup on an outside press, the parent losing key, a move or resize of the parent, a mode cancel or a lost capture. A button held on the popup gets its `PointerUp` first.
  - A popup without `Grab` takes no input and is never dismissed by the OS.
- `Caps().PopupShadow` (fixed per shell, any goroutine) reports whether the shell shadows every popup itself: true on Win32 (`CS_DROPSHADOW`), false on gtk4, where a menu draws its own shadow inside the popup.

### 8.1 `shell/fake`: the test shell

`fake.New()` is a `*Shell` without an OS; the aqua tests run on it. No build tags.

- **Loop:** `Post` goes to a `mainthread.Queue` (`Queue()`).
  - `Run(h)` binds the goroutine, calls `Ready` and drains until `Quit`.
  - `Start(h)` binds and calls `Ready`, then returns. The test then drives the shell with `Step()` (one non-blocking drain), `Flush()` (run the requested frame now) and `FakeClock().Advance(d)`.
  - A second `Run`/`Start` returns `ErrRunTwice`. No frames run by themselves.
- **Checks:** these calls panic:
  - Every method except `Post` called off the UI goroutine (`SetChecks(false)` turns this off; the check is costly).
  - API use before `Ready`.
  - Resizing, showing, closing or creating surfaces, input and `Quit` during `Paint`.
  - `ModPrimary` in an event, key input to a non-key surface, invalid buttons.
- **Clock** (also standalone via `NewClock`):
  - Time starts at 0 and moves only with `Advance(d)`, which runs each requested frame at the next multiple of `FrameInterval` (1/60 s) up to `Now+d`. `Flush()` runs one frame at `Now`.
  - A frame is the frame function, then one `Paint` of each invalidated, visible, not minimized surface in creation order.
  - The shell's clock applies `HiddenInterval` (15 intervals) while no surface is on screen.
  - Calling `Advance`/`Flush` inside a frame or timer panics.
- **Surfaces:**
  - They start hidden and invalidated, clamped to their limits, at the screen scale.
  - All state is recorded and readable after close: title, sizes, style, scale, `Centered()` count, visible, minimized, zoomed, key, cursor, regions.
  - `HitTest(p)` answers like the real shells, without resize borders.
  - Simulation helpers: `Activate`/`Deactivate`; `Zoom` toggles to `SetScreen`'s size (default 1440 × 900); `SetScale` (a monitor move); `UserResize` (paints before returning); `UserClose` (asks `CloseRequested`); `UserMove`/`Position`.
  - Each paint gets a fresh `record.Canvas` (`Paints`, `LastPaint`, `Dump`, `PaintErr`).
- **Input helpers** (dropped for hidden, minimized or closed surfaces):
  - Pointer: `Move`, `Press`, `Release`, `Click`, `DoubleClick`, `Drag`, `CancelCapture`, `SetModifiers`. They produce enter/leave, implicit capture and click counts (same button within `DoubleClickTime` 500 ms and `DoubleClickDistance` 4 pt).
  - A caption press is not delivered; a double click there zooms.
  - A press on a non-key surface makes it key first and sets `Activating`. A press on a blocked surface makes the active modal key and delivers nothing.
  - Scroll: `Wheel` (lines), `Swipe` (Windows-style phases), `Momentum` (GTK-style), `TimedSwipe` (advances the clock between deltas; `PhaseEnded` comes `SwipeEndDelay` = 100 ms after the last delta).
  - Keys: `KeyDown`/`KeyRepeat`/`KeyUp`/`Key`, with `Shifted` from a US layout (`SetShiftLayout`), and `ModifierDown`/`ModifierUp`/`TapModifier`.
  - Text: `Type(text)` (key-down, text, key-up per rune), `Commit`, and `Send(ev)` for raw events.
  - IME: `TextInput()`, `TextInputSets()` and `Cancels()` record the calls. `Preedit`, `EndPreedit` and `CommitPreedit` (the GTK order) drive a composition and panic unless the surface is key with input enabled and the event follows the `PreeditEvent` rules. `Composing()` tells whether a composition runs.
- **Text:** `shellkit.FixedText` (§8.1a), the native shells' fallback, with the fixed metrics of `driver/stub`; `TextSystem`, `RuneWidth` and `LineHeight` are aliases:
  - Every rune is one cluster `RuneWidth` wide (7 pt at 13 pt); every line is `LineHeight` high (16 pt); the baseline is at 3/4 of the line.
  - Wrapping is greedy after the last space run; trailing spaces hang. Without a space, text breaks between runes.
  - A caret at a break is at the next line's start. A selected `\n` is one rune wide.
- **Clipboard:** a string plus an ok flag, sanitized as on the real shells (`shellkit.ClipboardText`, also for `SetClipboard`). Reads are delivered through `Post`. Test side: `SetClipboard`, `Clipboard`, `ClipboardReads`.
- **Timers:** a `shellkit.Timers` on the clock. `Advance` fires each due timer at its deadline in (deadline, creation) order, interleaved with frames (a timer at or before a frame's time fires first). Timers fire in rounds as on the real shells: a timer made by a firing `fn` and due at once waits for the next time `Now` moves in the `Advance` (the next deadline, the next frame, which it precedes, or the end time), so a self-re-arming `After(0)` cannot hang `Advance`. `Flush`/`Step` fire none. `Timers()` lists the pending deadlines.
- **Popups:**
  - Placed with `shellkit.PlacePopup` in `SetWorkArea` (default: the screen), relative to the parent's `Position`. `Placed` is called before `NewPopup` returns.
  - Painted after the surfaces.
  - Grabs are driven through the parent's input helpers with the real routing. Deactivation, moves, size changes, `Minimize`, `CancelCapture` and `UserDismiss` dismiss.
  - `SetPopupTranslucent` and `SetCaps` (`PopupShadow` true by default; a test knob, a real shell's caps never change) set what `Translucent` and `Caps()` report.
- **Modals** (`modal.go`): the contract above. The last shown modal is active (`Shell.Modal()`) and blocks the others; `Blocked()` is computed, not stored. Blocked surfaces drop input silently. A modal ignores `Minimize` and `Zoom`.

### 8.1a `shell/shellkit`: shared shell logic

Pure Go: no build tags, no OS calls, depends only on `driver`, `driver/stub` (the fixed metrics), `render` and `shell`. Holds the computations and semantics that `win32`, `gtk4` and `fake` share (review R6), tested on every host, so the test shell runs the shipped code.

- **`EventTimeAt(now, tick, evTime)`** maps a wrapping uint32 ms OS time onto the clock: `now` minus the age. An age over `MaxEventAge` (5000 ms) or from the future gives `now`; the result is never negative. `Monotonic.Next` keeps times from going backwards.
- **`ClickCounter.Press(b, p, t, rules, atLeast2)`** counts a click as the next one in a series when it is the same button within `ClickRules.Time` (wrap-safe) and within half of `W × H` device pixels on each axis. `atLeast2` comes from a Windows `DBLCLK`. `Reset` restarts the count (after a caption press).
- **`Gesture.Next(precise)`**: inside a gesture, every event is precise with `PhaseChanged`. A precise event outside one gives `PhaseBegan`; anything else gives `PhaseNone`. `End` reports whether a gesture was active, so the shell knows to send `PhaseEnded`.
- **Sizes:** `ToPx` rounds half away from zero, clamped to [0, `MaxPx` = 2²⁰], NaN → 0. `FromPx(…, hint)` returns `hint` when it maps to the same pixels, so fractional sizes round-trip. `ClampSize`: zero = no limit, min wins over max.
- **`CenterIn`**, **`HitRegion`** (last to first, half-open rects, NaN nowhere; ok=false means client). The fake clamps sizes with `ClampSize` and answers `HitTest` with `HitRegion`, as the real shells do. `Rect`/`Point` convert directly from `w32`.
- **`PlacePopup(anchor, size, offset, adjust, work)`** works in any one coordinate space. It slides on X/Y into `work` (a popup larger than `work` is aligned to its left/top), then `AdjustResizeY` cuts the height unless the popup is wholly outside. An axis without an adjustment may stay off screen. Bad values are 0; an empty `work` adjusts nothing. Used by fake and Win32 (GTK places popups itself). `IsTrack`/`InRect` are the track tests.
- **`Timers`** (`timers.go`): the pending `After` timers of all three shells. `Add`/`Remove`, `Next`/`NextAfter` for arming the OS timer, `Deadlines`, `Clear`. `Fire(now, stop)` runs **one round**: the timers due at `now` that were pending when it started, in (deadline, creation) order, each removed before its `fn`; a timer a `fn` adds waits for the next round even when due (so a self-re-arming `After(0)` cannot starve a loop or hang the fake's `Advance`), one a `fn` removes is skipped, a panic leaves the rest pending. The shells own the clock and the OS timer (USER timer, GLib timeout, the fake's `Advance`).
- **`ClipboardText`** (`clipboard.go`): what every clipboard takes: cut at the first NUL, each invalid UTF-8 byte U+FFFD (as Go's `[]rune` decoding, so Win32's UTF-16 conversion agrees).
- **`FixedText`** (`fixedtext.go`): the fixed-width `render.TextSystem` (metrics of `driver/stub`, `FixedRuneWidth`/`FixedLineHeight`; rules in §8.1 "Text"). It is `shell/fake`'s text system and the fallback of the win32 and gtk4 text systems, so no shipped binary links `shell/fake` (`TestShellsDoNotImportFake`).

### 8.2 `shell/win32/w32`: raw Win32 bindings

`//go:build windows`. The only place `shell/win32` gets raw syscalls from; only the shell imports it.

- **Procs** (`procs.go`): user32, gdi32, dwmapi, shcore, kernel32 and imm32, loaded with `windows.NewLazySystemDLL` (System32 only). Each proc is declared through `newProc` (recorded in `allProcs`). Every proc exists on Windows 10 1809 (D7). `x/sys/windows` supplies `HWND`, `Handle`, `DwmSetWindowAttribute` and `registry`.
- **Constants** (`consts.go`) use SDK names. **Structs** (`types.go`) mirror the SDK. Native pointers are `uintptr`. Every layout is pinned by `ffi.CheckLayout`/`CheckNoGoPointers`; one table serves amd64 and arm64 (both LLP64). Structs arriving in an `lParam` are read with `ffi.At[T]`.
- **Wrappers** (by topic: `w32.go` (error helpers, kernel32), `window.go` (classes, lifecycle, message loop, modal), `geometry.go` (placement, monitors, DPI, metrics), `input.go` (timers, capture, cursor, keyboard layout, chrome), `paint.go` (GDI painting, window capture), `dwm.go`, `clock.go` (waits, waitable timers, QPC, DWM timing), `clipboard.go`, `reg.go`, `imm.go`) fill `cbSize` fields, convert strings to UTF-16 and wrap `windows.Errno`/HRESULT errors. Rules worth knowing:
  - `RegisterClassEx` pins the class name for the call and clears it afterwards, so no Go pointer stays in the struct.
  - `Get/SetWindowLongPtr` call `SetLastError(0)` first and fail only for a 0 result with a non-zero last error.
  - `DwmSetWindowAttributeInt` returns `E_INVALIDARG` for an attribute the OS doesn't know (the corner preference on Windows 10); callers ignore it.
  - `MonitorFromPoint`/`WindowFromPoint` pass the `POINT` packed into one register.
  - `MsgWaitForMultipleObjectsEx` keeps its handle slice alive. Waitable timers come from `CreateWaitableTimerExW` (high-resolution or standard, auto-reset).
  - IMM32: an HIMC is a `windows.Handle`. Every `ImmGetContext` is released in the same message. Composition strings and attributes are sized with a NULL call, then copied into a Go buffer passed for the call only.
  - `DwmGetCompositionTimingInfo` returns the refresh period and vblank in QPC ticks. `DWM_TIMING_INFO` is packed, so it is read from a byte buffer at fixed offsets.
  - `RegDWORD` reads an HKCU DWORD.
  - Calls with value arguments only (handles, integers, flags) go through `callV` (`call.go`, `syscall.SyscallN` on the proc address) instead of `LazyProc.Call`, whose `//go:uintptrescapes` and `error` result allocated on every call; the window procedure and the clock's idle steps allocate nothing (review P17, `TestMessageLoopAllocs`). A call that passes a pointer stays on `Call`, except `MsgWaitForMultipleObjectsEx` and `QueryPerformanceCounter`, which run no Go code.
- Calls that take an `HWND` must run on the window's thread; the package doesn't check.

### 8.2a `shell/win32/com`: the COM core

The base of the D3D11, DXGI, DComp, D2D and DirectWrite bindings. `guid.go`, `hresult.go` and `vtable.go` are untagged and tested everywhere; the rest is `//go:build windows`.

In every binding package (`w32`, `com`, `dxgi`, `d3d11`, `dcomp`, `d2d`, `dwrite`), a method that
only the package's own tests call lives in its `export_test.go` (review §4). The calls that
the tests of several packages share stay exported: the D3D11 readback and clear, `Desc1`,
`w32.UnregisterClass`/`FindWindowEx`/`DefWindowProcAddr`, `com.CheckSlots`/`Vtable` and
`Ptr.RefCount`.

- **`Ptr`** is an interface pointer (`uintptr`). A non-zero `Ptr` held by Go owns one reference.
  - `Slot(i)` reads a vtable entry through `ffi.ReadUintptr`.
  - `AddRef`; `Release(*Ptr)` clears the pointer and is a no-op on nil, so it can be deferred.
  - `QueryInterface` returns a new reference; a missing interface wraps `E_NOINTERFACE`.
  - `RefCount` is for tests. No finalizers.
- **Integer calls:** `Call`/`CallHR` go through `syscall.SyscallN` (`//go:uintptrescapes`). `CallOut(op, slot, args...)`/`GetOut` call a creation method or getter with the out pointer appended and return it (review R9); the flat exports and `GetSystemFontCollection` keep the explicit form.
- **Direct path** (`direct.go`, review P2): the per-draw methods of d2d and dwrite call `syscall.SyscallN` themselves with the pointer conversions in the call expression (`//go:uintptrkeepalive`: no heap copy), safe only because they run no Go code. With `DirectFloats` (windows/amd64) a FLOAT goes as its bits in its integer slot (`Float`), a `D2D1_POINT_2F` by value as one integer (`Float2`), and a FLOAT result comes back as r2 (`CallFloat`).
- **Float calls (windows/arm64):** `Bind(fptr, p, slot)` (and the test-only `BindFunc`) use `purego.RegisterFunc`, so floats and HFAs (`D2D1_POINT_2F`) follow the C ABI, which on windows/arm64 keeps them in s0–s7 where `SyscallN` loads only integer registers. On amd64 the bindings use the direct path; their tests run the bound path too.
  - Bound funcs are cached by func type and code address. `Method[F]` caches the last binding per method, so hot calls skip the lock.
  - `Bind` rejects: nil `this`, a negative slot, a non-uintptr first parameter, func parameters or results (they would create a native callback, which §7.2 forbids), variadics, and struct results (MSVC returns those through a hidden pointer after `this`; bindings declare that out pointer instead).
- **`Vtable`/`CheckSlots`:** slot-table checks for the binding packages (`Extend` builds a table from the parent interface).
- **`GUID`** (16 bytes): `ParseGUID`, `MustGUID`, `String`.
- **`HRESULT`** implements `error` (named codes include device loss, `D2DERR_RECREATE_TARGET` and `DWRITE_E_FILEFORMAT`). `hr.Err(op)` returns an `*Error{Op, HR}` that unwraps to the code, so `errors.Is(err, com.E_…)` works.
- **Apartment:** `InitUIThread` calls `CoInitializeEx(APARTMENTTHREADED|DISABLE_OLE1DDE)` and returns the matching `CoUninitialize`. `win32.Shell.Run` calls it right after `LockOSThread`.

### 8.2b `shell/win32/dxgi`, `d3d11`, `dcomp`: the composition bindings

Three `//go:build windows` packages on `com`, each with only what the shell uses.
- Every interface is a struct embedding `com.Ptr`, and a returned interface carries one reference.
- Methods take integers and pointers only.
- Slot constants are unexported. The tests hold each bound interface's full vtable in header order and check it with `com.CheckSlots` (MSVC reverses the order of overload pairs). The smoke test checks every IID live.
- **`dxgi`:**
  - The chain from the D3D device is `DeviceFrom(d3d11) → Adapter → Factory2`. A composition chain needs the device's own factory.
  - `Factory2.CreateSwapChainForComposition`.
  - `SwapChain1`: `Present` returns the HRESULT, so `STATUS_OCCLUDED` is visible. Also `Buffer`, `ResizeBuffers`, `FrameStatistics` (`DISJOINT` before the first shown present), `LastPresentCount`, `Desc1`.
  - `SwapChain2`: `SetMaximumFrameLatency`, `FrameLatencyWaitableObject` (the caller closes the handle).
- **`d3d11`:**
  - `CreateDevice` tries hardware with BGRA support first, then WARP; when both fail, a `*CreateError` holds both HRESULTs. `CreateDeviceOfType` picks one type. Feature levels run from 11_1 down to 9_1.
  - Device, context, texture and RTV methods for rendering and readback.
  - The device is not single-threaded, because DComp and D2D may use it from their own threads.
- **`dcomp`:** `CreateDevice`, `Commit`, `CreateTargetForHwnd`, `CreateVisual`, `Target.SetRoot`, `Visual.SetContent`.
- **Reference rules:**
  - An adapter keeps its factory alive and the device keeps its adapter, so they all go with the device's last `Release`.
  - `ResizeBuffers` fails with `DXGI_ERROR_INVALID_CALL` while any back-buffer reference is alive.

### 8.2c `shell/win32/d2d`: the Direct2D bindings

`//go:build windows`, on `com`, with dxgi's rules (UI thread only). Vtables follow `d2d1.h`/`d2d1_1.h` declaration order: a method a derived interface redeclares (`CreateBitmap1`, …) takes a new slot, inline overloads take none. The smoke test checks them against live objects.

- **Pipeline** (`d2d.go`):
  - `CreateFactory` makes a single-threaded `ID2D1Factory1`, then `CreateDevice(dxgi.Device)` and `CreateDeviceContext`.
  - `DeviceContext`: `CreateBitmapFromDxgiSurface`, `SetTarget`, `BeginDraw`/`EndDraw` (`D2DERR_RECREATE_TARGET` means device loss), `Clear`, `SetDpi`, `Flush`.
  - Back-buffer bitmaps are `TARGET|CANNOT_DRAW`, B8G8R8A8 premultiplied.
- **Drawing** (`draw.go`):
  - Transform, axis-aligned clips, and `PushLayer` (the `ID2D1DeviceContext` overload with a NULL layer).
  - Brushes (solid, linear gradient), rect and rounded-rect fill/draw, geometry fill/draw, path geometry with `GeometrySink`, stroke styles (`ID2D1StrokeStyle1`).
  - `Brush`, `Geometry` and `Image` are base views without their own reference.
  - `DefaultBrushProperties` exists because the zero `BrushProperties` is invisible.
- **Call paths:** the per-draw methods (`Clear`, `SetDpi`, transform, clips and layers, fills and draws, `DrawTextLayout`, `DrawImage`, `DrawBitmap`, the sink's figure methods) take the direct path (§8.2a) and allocate nothing (`TestDrawAllocs`). Their float and `Point2F` arguments use the integer slots on amd64 and `com.Bind`/`Method` on arm64 (`directFloats`; `TestDrawBound` runs the bound path on amd64). No struct-returning method is bound.
- **Enum traps** (the canvas maps them; never cast):
  - D2D's `CapStyle` order is FLAT, SQUARE, ROUND.
  - render's miter join is `MITER_OR_BEVEL`.
  - Dashes and their offset are in stroke-width units.
  - Nonzero fill is `FILL_MODE_WINDING`.
- **Effects and bitmaps** (`effect.go`):
  - `CreateBitmap` (`SizeU` packed in one register; short pixel data is rejected before the call), `CreateEffect`, `DrawImage`, `DrawBitmap`.
  - `Effect` property helpers.
  - The effects used: Shadow, GaussianBlur, Crop.
  - A second context of the same device can draw into bitmaps the first one uses. This is the basis of the shadow-mask context.
- **Text:** `DrawTextLayout` takes the layout as a `com.Ptr`, so d2d doesn't import dwrite. Also `SetTextAntialiasMode`.
- **Structs** are layout-tested (e.g. `LayerParameters1` is 72 bytes, align 8).
- **Reference rules:**
  - A target bitmap holds extra back-buffer references until `SetTarget(0)` and its `Release`; only then can `ResizeBuffers` succeed.
  - A fresh context has 96 DPI and no target.
  - `CopyFromBitmap` from the context's own target inside `BeginDraw` reads what the GPU already has, not the batched commands. A `Flush` must come before it.

### 8.2d `shell/win32/dwrite`: the DirectWrite bindings

`//go:build windows`, on `com`, with d2d's rules. Text crosses as UTF-16; the UTF-8 mapping belongs to the shell.

- **Factory:** `CreateFactory` makes a shared factory (one per process; each call adds a reference) and requires `IDWriteFactory5`, failing with `ErrNoFactory5` (Windows 10 1703, D7).
- **`TextFormat`:** wrapping and font settings the shell uses. Out-of-range values give `E_INVALIDARG`. A family the collection lacks is no error at format time.
- **`TextLayout`** (embeds `TextFormat`): `Metrics`, `LineMetrics`, `ClusterMetrics`, `HitTestTextPosition`.
  - The array calls share a "buffer too small" retry.
  - `CreateTextLayout` passes a terminated copy, and DirectWrite copies the text.
- **Behaviour the shell relies on:**
  - `\r\n` is two clusters; only the `\n` is `IsNewline`, and the line's `NewlineLength` is 2.
  - A combining mark or a surrogate pair forms one cluster of 2 units.
  - The last cluster always has `CanWrapLineAfter`.
  - Points outside the text give the nearest position with `Inside` false.
  - Empty text has no clusters, one line of font height, the caret at (0,0) and one empty range box.
- **In-memory fonts** (`fonts.go`):
  - `Factory.CreateMemoryFonts(files)` builds one collection from several font files and returns a `*MemoryFonts`.
  - `CreateFontFileReference` passes no owner, so DirectWrite copies the bytes and no Go memory is held natively.
  - Unusable data or no fonts (`ErrNoFonts`) is an error, with everything released.
  - `MemoryFonts.Release` releases the collection, unregisters the loader and drops the factory reference. It is idempotent and nil-safe.
  - **Invariant:** the loader stays registered while any collection, format or layout from its files lives. Using a layout after `UnregisterFontFileLoader` kills the process.
- **Float path:** methods with FLOAT arguments or results pass them in the integer slots on amd64 (`com.Float`, `CallFloat`) and go through `com.Method` on arm64 (`directFloats`; `TestBound` runs the bound path on amd64).
- **Structs:** layout-tested. `ClusterMetrics` packs its flags as UINT16 bit fields, LSB first, read by mask.

### 8.3 `shell/win32`: the Windows shell

`win32.New()` is a `*Shell` implementing `shell.Shell`. Every file except `doc.go` is `//go:build windows`.

- **Run:**
  - Locks the OS thread and makes it a COM STA. Binds the `mainthread.Queue`.
  - Sets Per-Monitor-V2 DPI awareness. `ERROR_ACCESS_DENIED` is accepted only if the thread already has that context.
  - Registers the window class `gogui.win32` once per process (`CS_DBLCLKS`).
  - Creates the listener, reads the appearance, installs the wake and the clock's hold timer, then calls `Ready`.
  - Each loop round is the clock's `wait`, then one `PeekMessageW(PM_REMOVE)`/`TranslateMessage`/`DispatchMessageW`.
  - On exit (also after a panic) the queue is drained once, closed and the listener destroyed.
  - A foreign `WM_QUIT` acts as `Quit`. A second `Run` returns `ErrRunTwice`.
- **Listener:** a hidden `WS_POPUP` tool window, not a message-only window, because message-only windows get no broadcasts. It receives `WM_SETTINGCHANGE`/`WM_THEMECHANGED`/`WM_DWMCOLORIZATIONCOLORCHANGED` even with no surface open, plus the private `msgWake` (drain) and `msgFrame` messages and the shell's USER timers.
- **Post:** `Queue.Do`. The wake is one `PostMessageW(msgWake)` per drain, so it stays coalesced across goroutines. `Queue().SetChecks` also turns on the shell's UI-thread checks.
- **Window procedure** (`wndproc.go`):
  - One `ffi.RegisterCallback("win32.wndproc")` trampoline serves both classes (`gogui.win32`, `gogui.win32.popup`).
  - `WM_NCCREATE` copies the `ffi.Handle` from `lpCreateParams` into `GWLP_USERDATA`. Each message looks the target up in `ffi.Handles[target]`; an unknown or stale handle goes to `DefWindowProcW`. `WM_NCDESTROY` frees the handle.
  - Panics are recovered at the trampoline (they must not unwind through native frames) and kept as a `*PanicError`. The loop re-raises it after `DispatchMessageW`; Go code that sends messages calls `rethrow`.
- **Appearance** (`appearance.go`), all read through a replaceable `sysReader`:
  - `Dark`: `AppsUseLightTheme`.
  - `Accent`: `DWM\AccentColor`, else `DwmGetColorizationColor`, else the default; always opaque.
  - `IncreaseContrast`: high contrast. `ReduceMotion`: client-area animation off. `ReduceTransparency`: `EnableTransparency` = 0. `LegacyScrollers`: `DynamicScrollbars` = 0.
  - `CaretBlink`: the period is twice `GetCaretBlinkTime` (`INFINITE` = no blink); the timeout comes from `SPI_GETCARETTIMEOUT`.
  - Broadcasts trigger a re-read; `AppearanceChanged` is called only on a change, and every surface is invalidated.
- **Clipboard** (`clipboard.go`): `CF_UNICODETEXT`, with the listener as owner.
  - Writes go through a `GMEM_MOVEABLE` block (`ffi.WriteBytes`), which is freed on any failure. Text is sanitized by `shellkit.ClipboardText` (cut at an embedded NUL, each invalid UTF-8 byte U+FFFD).
  - Reads copy the locked block. `\r\n` passes through unchanged.
  - Opening retries 8 × 4 ms while another window holds the clipboard.
  - `ReadClipboardText` reads at once and delivers through `Post`.
- **Timers** (`timer.go`):
  - `After` keeps the timers in a `shellkit.Timers` on the QPC timeline and arms **one** USER timer on the listener (`timerAfter`) for the earliest deadline. One timer, rather than one per `After`, because Windows delivers several expired timers in its own order. It also fires inside modal loops.
  - `fireTimers` fires one round of the due timers in order and re-arms, also after a panic. An early tick just re-arms.
  - A timer created by a firing `fn` waits for the next round, so a self-re-arming `fn` cannot starve the loop.
- **IME** (`ime.go`), IMM32 only (no TSF, reconversion or surrounding text):
  - Surfaces start with no input context (`ImmAssociateContextEx(h,0,0)`), so the IME is off. `SetTextInput` on associates the default context and places the IME windows; off cancels (`CPS_CANCEL`) and disassociates.
  - `WM_IME_SETCONTEXT` clears `ISC_SHOWUICOMPOSITIONWINDOW`: aqua draws the preedit, the IME draws its candidate window.
  - While input is enabled, composition messages never reach `DefWindowProcW`, so no `WM_IME_CHAR` repeats a commit. `WM_IME_COMPOSITION` delivers `GCS_RESULTSTR` first as a `TextEvent`, then `GCS_COMPSTR`, attributes and cursor as a `PreeditEvent`. Segments come from runs of equal `ATTR_*` bytes; the target attributes are selected.
  - An empty composition or `WM_IME_ENDCOMPOSITION` ends a running composition with one empty preedit. Each end is sent once.
  - `imeText` maps UTF-16 to UTF-8 byte offsets and drops lone surrogates and control characters.
  - `placeIME` runs on enable, on a caret change, on `WM_DPICHANGED` and on `WM_IME_STARTCOMPOSITION`. The composition window gets `CFS_POINT` at the caret. The candidate window gets `CFS_EXCLUDE` around the caret (Japanese, Korean), or `CFS_CANDIDATEPOS` below it when the keyboard layout is Chinese.
  - Every IMM32 call by the shell runs **muted**: the composition messages it causes produce no events and do not reach `DefWindowProcW`. This makes `SetTextInput` safe during `Paint`. IMM32 errors are ignored. `VK_PROCESSKEY` is not delivered.
  - IMM32 calls go through the `imm` seam, which tests replace.

#### Popups (`popup.go`, `grab.go`, `backdrop.go`)

- **Windows:**
  - A popup is a `WS_POPUP` of class `gogui.win32.popup` (`CS_DBLCLKS|CS_DROPSHADOW`, same trampoline), owned by the parent's HWND.
  - Styles: `WS_EX_TOOLWINDOW|NOACTIVATE|NOREDIRECTIONBMP`, shown with `SW_SHOWNOACTIVATE`, `WM_MOUSEACTIVATE` → `MA_NOACTIVATE`. So it never activates and the parent stays key.
  - Its window and rendering state are a `swapTarget` (`swaptarget.go`, review R8), embedded as in a surface; the popup itself is the window's `target`. Its premultiplied composition chain is cleared to **transparent** (`popup.background`): the handler paints the fill, the rounded corners stay empty.
  - It is painted, paced and recovered from device loss together with the surfaces (`Shell.allWindows` yields the surfaces' swap targets, then the popups').
- **Placement:**
  - The anchor maps to screen pixels at the parent's scale. Size and offset use the DPI of the anchor's monitor, and `PlacePopup` adjusts in whole pixels within that monitor's work area. If the window lands at another DPI, it is placed again.
  - `Placed` reports X/Y in parent points and the size in the popup's points.
  - `Reposition` reports only real changes. `WM_DPICHANGED` re-places the popup.
- **Closing:** `Close` is `DestroyWindow`. `WM_DESTROY` ends the grab, releases the chain and calls `Closed` once. `closeAll` and `surface.destroy` close a surface's popups **before** destroying it, because Windows hides and deactivates the owner first, which would dismiss a grab on the way.
- **Grab** (`grab.go`):
  - Opening ends the parent's pointer state as a capture cancel. The popup takes `SetCapture`; within the thread, the capture gets every mouse message, so the parent sees none.
  - Mouse messages become popup `PointerEvent`s with the fake's rules and the shell's `ClickCounter`. The cursor is set on every move, because no `WM_SETCURSOR` reaches a capture.
  - `Track` is converted to popup points through whole pixels at each placement.
  - Mouse messages that still reach the parent (including non-client ones) are forwarded to the popup (`fromParent`). So a caption press counts as an outside press and starts no window drag.
  - The wheel goes to the popup's scroll handling, which runs its own gesture timer. The parent's key, character and IME messages go to the popup handler through `surface.keySink`.
  - **Dismissal** (`popup.dismiss`, once) comes from: an outside press; the parent going inactive (before its `KeyChanged(false)`); `WM_ACTIVATEAPP` to another app; a `WM_WINDOWPOSCHANGING` that moves, sizes or hides the parent (z-order changes don't count); `WM_CANCELMODE`; `WM_ENTERSIZEMOVE`/`WM_ENTERMENULOOP`; losing the capture.
  - Dismissal order: grab dropped and capture released, then a `PointerUp` per held button, `PointerLeave`, `Dismissed`. `Close` ends a grab without events.
  - A popup without `Grab` answers `WM_NCHITTEST` with `HTTRANSPARENT`, so the mouse goes to the window below it in the same thread. Click-through to other processes is unsupported: `WS_EX_TRANSPARENT` needs `WS_EX_LAYERED`, which does not mix with DComp.
- **Backdrop** (`backdrop.go`), applied before showing and again on appearance change:
  - Rounded corners (`ROUNDSMALL`; default corners in high contrast).
  - `DWMWA_USE_IMMERSIVE_DARK_MODE`, which sets the backdrop tint.
  - From build 22621 (`acrylicBuild`): Acrylic (`DwmExtendFrameIntoClientArea(-1)` + `DWMSBT_TRANSIENTWINDOW`) unless increase-contrast or reduce-transparency is on; otherwise `DWMSBT_NONE`.
  - `Translucent()` is true only when Acrylic was wanted and both calls succeeded.
  - The DWM calls sit behind a seam (`backdropSys`).

#### Modal surfaces (`modal.go`)

- **Window:**
  - An owned top level of the surface class: `WS_POPUP|WS_CAPTION|WS_SYSMENU`, no `WS_EX_APPWINDOW`. It is owned by `Owner`'s window, or by the listener for app-modal.
  - Not resizable, so it has no frame. The caption style keeps the DWM shadow and corners.
  - `SC_SIZE`/`SC_MINIMIZE`/`SC_MAXIMIZE` are refused, and so are `Minimize` and `Zoom`.
- **First `Show`:** records the key surface, places the modal (`placeModal`: on the owner, else on the key or primary monitor's work area; placed again if the DPI changed) and appends it to `Shell.modals`.
- **`applyBlocking`:** disables every other open surface with `EnableWindow(false)` and enables the active modal. Becoming blocked first dismisses the grab popup and cancels the pointer (`endPointer`, shared with `startGrab`).
- **A blocked surface:**
  - Its activation reports no `KeyChanged` and posts `msgModalKey`, which activates the modal.
  - A button-down over it activates the modal.
  - `WM_CLOSE` and the system move/size/close commands are refused, and input sent to it is dropped.
  - `Show` uses `SW_SHOWNA`.
- **Closing:**
  - Owned modals close first (newest first), then popups.
  - A closing modal re-enables the others **before** `DestroyWindow` (otherwise Windows activates another app), then hands the key on per the contract, before its `Closed`.

#### Text system (`text.go`, `textindex.go`, `textgeom.go`, `textcache.go`)

The text system (handed to `Ready` in `shell.Services`) is used on the UI thread. The DirectWrite factory and system collection are created with the first layout and released in `shutdown`. If that fails, or a layout is requested after release, it falls back to `shellkit.FixedText` layouts (logged once), so text never takes the app down.

- **Fonts:**
  - The empty family resolves to Inter from the embedded collection, else `Segoe UI Variable Text`/`Segoe UI`. `Monospaced` resolves to Consolas, else Courier New.
  - Named families are looked up in the embedded collection, then the system one. `Family` wins over `Monospaced`.
  - Size 0 → 13; sizes clamp to 0.1…2048, weights to 1…999.
  - Formats are cached per key (at most 128, then emptied). The wrap mode is set per layout, never on the shared format.
  - The family lookups (`FindFamilyName` in the embedded and system collections) are memoized per family and `Monospaced` (`memo`, at most 128, then emptied), so a layout in a family already seen makes no COM call to resolve it (review P17).
- **Embedded fonts:** every file of `assets/fonts` goes into one in-memory collection. If any file fails, there is no embedded collection and the reason is logged once (`GOGUI_DEBUG=1`). `release` frees the fonts after the formats (the loader invariant, §8.2d).
- **Layouts:**
  - Measured once in `NewLayout` (DIPs, natural measuring) and kept as Go data: text, font, index map, size, baseline, line height, geometry.
  - A layout owns no native object, because nothing relies on finalizers. The native `IDWriteTextLayout` lives in the **drawn-layout cache**, which holds up to `maxDrawn` = 256 entries. The least recently used entry is evicted first (an intrusive recency list, so an eviction takes the tail instead of scanning; review P17), never one drawn in the current frame (D2D may hold it until `EndDraw`). An evicted layout is rebuilt from text, font and width when drawn.
  - A positive finite `maxWidth` wraps; any other value does not.
  - `Size` is DirectWrite's width (trailing whitespace excluded) × height. Empty text is one line high.
- **Index map:** UTF-8 ↔ UTF-16, invalid bytes as U+FFFD. Indices move back to their rune's start and are clamped. ASCII uses the identity map.
- **Geometry** (`textgeom.go`): line and cluster metrics are read in `NewLayout`, so `HitTest`, `CaretRect`, `CaretRectUpstream`, `Line` and `SelectionRects` make no native calls. The rules match `shellkit.FixedText`:
  - Indices snap to cluster starts; a line break (`\n`, `\r\n`) is one cluster.
  - Right of a line, hit-testing gives the line's end. On a line wrapped at whitespace that is the start of its trailing whitespace, so the caret stays on the line.
  - A caret at a wrap point is at the next line's start. A selected break is half an em wide.
  - Points above or below snap to the first or last line.
  - LTR text places clusters by summing widths. Mixed-direction text uses `HitTestTextPosition` per cluster, and a selection is one rect per line.
- **Drawing:**
  - `DrawText` accepts only a live, non-empty `*layout` of this text system, a concrete non-transparent color and a finite origin; it ignores everything else.
  - It draws with `DrawTextLayout`, `ENABLE_COLOR_FONT` and grayscale antialiasing (no ClearType fringes).
  - Crispness at every scale comes from the context DPI; nothing is snapped by hand.

#### Surfaces (`surface.go`, `surfaceproc.go`, `cursor.go`, `swaptarget.go`)

- **Shared window state** (`swaptarget.go`, review R8): `swapTarget` holds what a surface and a popup share: the HWND and its handle, size, scale and DPI, `dead`/`invalid`, the swap chain and its bitmap, the waitable's ticket and debt, the present log and the precise-scroll gesture. Both embed it; what differs goes through the `window` interface (`win`): `paintContent`, `background` (the window background or transparent) and `scrollEvent`. The chain code (`present.go`), the pacing (`waitable.go`, `pacing.go`), the wheel and the coordinate helpers (`screenPoint`, `clickRules`, `frameDPI`) are `swapTarget` methods, so there are no popup branches in them.
- **Files:** `surface.go` (creation, sizes, the `shell.Surface` methods), `surfaceproc.go` (the window procedure, `WM_DESTROY`, `closeAll`), `cursor.go` (the system cursors).

`NewSurface` (`ErrQuit` after `Quit`; panics during `Paint`) creates a hidden window of class `gogui.win32`.
- Styles: `WS_OVERLAPPEDWINDOW`, minus `WS_THICKFRAME|WS_MAXIMIZEBOX` without `StyleResizable` and `WS_MINIMIZEBOX` without `StyleMiniaturizable`; `WS_EX_APPWINDOW|WS_EX_NOREDIRECTIONBMP`.
- The handle goes in `lpCreateParams`.
- Sized at the primary monitor's DPI, then again at the window's own, so the initial size sends no `Resized`.
- The drawable area is the client area, which the custom chrome extends to the window's top edge.

- **Sizes:**
  - Points ↔ pixels at DPI/96 through `shellkit.ToPx`/`FromPx`.
  - `SetSize` restores a maximized or minimized window, clamps, and reports `Resized` before it returns (also for sub-pixel changes).
  - `SetSizeLimits` resizes only when the current size breaks the new limits. `WM_GETMINMAXINFO` always sets the minimum track size.
- **Resized:**
  - `WM_SIZE` (not while minimized) calls `Resized` on a change of size or scale, then invalidates. If the surface is on screen it paints and presents at once (`UpdateWindow`), so buffers, content and window change in the same message.
  - `WM_DPICHANGED` applies the suggested rect (holding back its `WM_SIZE`) and sends one `Resized` with the new scale, keeping the logical size.
- **Lifecycle:**
  - `Show`: `SW_SHOW` or `SW_RESTORE`, then `SetForegroundWindow` (Windows may refuse).
  - `Minimize` and `Zoom` (`SW_MAXIMIZE`/`SW_RESTORE`) do nothing while hidden or not permitted. `Center` uses the work area.
  - `WM_CLOSE` asks `CloseRequested`; `Close` destroys directly. `WM_DESTROY` marks the surface dead and calls `Closed` once.
  - `Quit` closes every surface newest first. A `Closed` handler's `NewSurface` then gets `ErrQuit`.
  - Surfaces a panic left open are destroyed in teardown without handler calls.
- **Key, cursor:** `WM_ACTIVATE` → `KeyChanged` once per change. `WM_SETCURSOR` over `HTCLIENT` sets the `SetCursor` cursor and starts `TME_LEAVE`; `SetCursor` applies at once while the pointer is over the client area.
- **Paint:**
  - `Invalidate` marks the surface and requests a frame.
  - The clock's paint pass paints each invalidated, visible, not minimized window (`InvalidateRect` + `UpdateWindow` → `WM_PAINT` → `render`). Hidden or minimized surfaces stay invalid until shown.
  - `WM_ERASEBKGND` is swallowed.

#### Custom chrome (`chrome.go`, `ncinput.go`)

The windows keep `WS_OVERLAPPEDWINDOW` (taskbar entry, system menu, Snap, minimize/maximize animations) but lose the native caption.
- **Frame:**
  - `WM_NCCALCSIZE` keeps only the side and bottom resize frame (`SM_C?SIZEFRAME` + `SM_CXPADDEDBORDER` at the window DPI, invisible on Windows 10/11); the client area reaches the top edge.
  - Maximized: no frame, clipped to the work area. Not resizable: no frame at all.
  - Sizes, limits and `WM_GETDPISCALEDSIZE` all use this frame.
- **DWM:** a 1 px top `DwmExtendFrameIntoClientArea` keeps the shadow and border. Dark mode follows `Appearance.Dark`; corners are rounded except in high contrast. Applied on create, on `WM_DWMCOMPOSITIONCHANGED` and on appearance change; failures are ignored.
- **Hit-test:** `hitTest` is a pure function over screen pixels that answers `WM_NCHITTEST` without calling Go handlers. In order:
  1. Outside the window: `HTNOWHERE`.
  2. Resize borders, when resizable and not maximized: sides, bottom, and a top band inside the client area; corners reach twice the border along each edge.
  3. The chrome regions, last to first: `HTCAPTION`, `HTCLOSE`, `HTMINBUTTON`, `HTMAXBUTTON`, `HTCLIENT`.
  4. Otherwise `HTCLIENT`.
- **Pointer over the chrome:**
  - `WM_NCMOUSEMOVE` over a region becomes `PointerMove` (after an enter). It also goes on to `DefWindowProcW`, which keeps the Snap Layouts flyout on `HTMAXBUTTON`.
  - `WM_NCLBUTTONDOWN`/`DBLCLK` on a light becomes a `PointerDown` that captures the mouse. The native action is suppressed, so aqua's light calls `Close`/`Minimize`/`Zoom`. The captured moves and the release become pointer events, and `WM_CAPTURECHANGED` ends the press with a `PointerUp`.
  - Caption presses go to `DefWindowProcW` (move or maximize) and are not delivered.
  - `PointerLeave` comes when the pointer leaves both the client area and the regions (`WindowFromPoint` + hit-test).

#### Input (`input.go`, `wheel.go`, `keyboard.go`)

Modifiers come from `GetKeyState` (AltGr = Control+Option). Event times are `GetMessageTime` mapped onto the clock through `shellkit.EventTimeAt` against `GetTickCount`, never going backwards. Both sources are replaceable in tests.

- **Mouse:**
  - `WM_MOUSEMOVE` becomes `PointerMove` (repeats of the same position are dropped) and starts `TME_LEAVE`.
  - Five buttons. The first held button sets the capture and the last release frees it. Inconsistent presses or releases are dropped.
  - `WM_CAPTURECHANGED` with buttons held sends a `PointerUp` for each.
  - Clicks are counted with `ClickCounter`: `GetDoubleClickTime` and the `SM_CXDOUBLECLK` rect at the window DPI, so triple clicks work. A `PointerUp` carries its press's count.
  - Handling the right and X buttons means no `WM_CONTEXTMENU`/`WM_APPCOMMAND`.
- **First click:** `WM_MOUSEACTIVATE` over the client area or a light of a non-key, unblocked surface marks the next button-down `Activating`, if `WM_ACTIVATE` made the surface key by then. `DefWindowProcW` still answers `MA_ACTIVATE`; aqua's router drops the press (§9).
- **Wheel:**
  - Multiples of 120 are notches in lines (× `SPI_GETWHEELSCROLLLINES`/`CHARS`; page scroll counts 20 lines) with no phase.
  - Other deltas are precise points (lines × 100/3) and form a gesture (`shellkit.Gesture`). A 100 ms timer after the last delta sends `PhaseEnded` with a zero delta.
  - No momentum: that is aqua's job.
  - Positive vertical deltas move the content down; positive horizontal ones (tilt right) move it left.
- **Keyboard:**
  - `WM_(SYS)KEYDOWN/UP` become `KeyEvent`s.
  - `keyFor` maps a VK table of named keys, F1–F12 and the keypad; modifiers, F13–F24 and Insert are `KeyNone`; other keys use the layout's unshifted character (`MapVirtualKeyExW`), normalized.
  - `Shifted` comes from `ToUnicodeEx` over a Shift-only state with `TOUNICODE_NOSTATE`, which leaves a pending dead key alone.
  - `Repeat` is lParam bit 30. `VK_PROCESSKEY` is not delivered.
  - A handled key-down removes the pending `WM_(SYS)CHAR`/`DEADCHAR` and does not reach `DefWindowProcW`, nor does its key-up. Unhandled keys go on, so Alt+F4 and Alt+Space keep working.
  - `WM_CHAR` becomes a `TextEvent` (surrogates joined, control characters dropped). `WM_SYSCHAR` is never text.
  - No synthetic key-ups are sent on focus loss.

#### Rendering pipeline (`gpu.go`, `present.go`, `waitable.go`)

All on the UI thread.
- **Shell devices** (`gpu`):
  - One D3D11 device: hardware with BGRA support, else WARP. Tests can force WARP with `Shell.hooks.warp` for the golden renders.
  - Its DXGI device and factory, a single-threaded D2D factory and device, the main `ID2D1DeviceContext`, a second context for shadow masks (`gpu.mask`, so the main context never retargets while clips or layers are pushed), and one DComp device.
  - Created for the first surface and released after the last one: dependants first, then `ClearState` + `Flush`, then the device.
  - Releases that should be the last are checked; leftovers go to `Shell.leaks`, which tests require to stay empty.
- **Per window:** `createChain` makes:
  - A composition swap chain: B8G8R8A8, premultiplied, 2 buffers, flip-sequential, stretch, `FRAME_LATENCY_WAITABLE_OBJECT`, latency 1, at least 1×1.
  - A DComp target and root visual showing the chain.
  - A D2D target bitmap over buffer 0 at the window DPI.
  - If the chain fails, `NewSurface` destroys the window and returns the error. `releaseChain` empties the visual tree and releases everything.
- **Resize:** `fitChain` runs before `Resized`, so the handler already sees buffers of the new size. It releases the bitmap, calls `ResizeBuffers` if the pixel size changed (minimized keeps the old size), and makes a new bitmap.
- **Frame** (`render`, from `WM_PAINT` after `BeginPaint`/`EndPaint`):
  1. Ensure and fit the chain.
  2. `SetTarget(bitmap)`, `SetDpi`, `BeginDraw`.
  3. `Clear` to the theme's `ColorWindowBackground` (transparent for popups).
  4. `Paint` on a fresh canvas, then `finish`, `EndDraw`, `SetTarget(0)`. The context keeps no back-buffer reference between frames.
  5. Release the canvas resources.
  6. `Present(1, 0)` + `Commit`.

  A panicking `Paint` still ends the draw and releases the frame. With `GOGUI_DEBUG=1`, `paintErr` (recorder contract errors) and `renderErr` are logged.
- **Device loss (one policy, `gpu.go`):** `DXGI_ERROR_DEVICE_REMOVED/RESET/HUNG` or `D2DERR_RECREATE_TARGET` from any create or commit step goes through `Shell.lost`, which calls `deviceLost`. The steps: creating the chain (`createChain`, its composition target/visual and `Commit`), the target bitmap, `ResizeBuffers`, `EndDraw`, `Present` and the frame's `Commit`.
  - `deviceLost` releases every chain, image bitmap and device, recreates the devices and the chains of all open windows, invalidates everything and requests a frame. A re-entrancy guard (`recovering`) makes a nested call only record its cause.
  - A loss while it recreates the chains stops that round (the remaining windows are only invalidated) and rebuilds once more (`recoverTries` = 2 rounds). After that, a window without a chain retries at its next paint.
  - `NewSurface` and `NewPopup` create their chain through `newChain`: on a lost device the recovery runs (it creates the new window's chain with the others), and if the chain is still missing it is tried once more before the window is destroyed and the error returned.
  - Backoff: at the `lossLimit` = 3rd loss within `lossWindow` = 30 s the shell sets `fallback` and runs on WARP from then on (also for devices created after the last surface closes), so a GPU that keeps hanging is not rebuilt every frame.
  - Test hooks are in `Shell.hooks` (`testHooks`, zero outside tests): `faults` (a queue of injected HRESULTs for `createChain`, `createBitmap`, `ResizeBuffers`, `EndDraw`, `Present`, `Commit`; only the first entry can hit), `onDrawn` (back-buffer readback between `EndDraw` and `Present`) and `warp`. The counters are in `Shell.stats` (`wakes`, `drains`, `gpuGen`, `losses`).
  - Releases that must be the last reference go through `leakFunc.releaseLast` (the gpu, canvas, backdrop, image and text-layout caches); a reference left over goes to `Shell.leak`.

#### Frame clock (`clock.go`, `clockmodal.go`, `waitable.go`)

All on the UI thread.
- **Time:** `Now` is QPC since `New`, which is DWM's timeline, so `DwmGetCompositionTimingInfo` vblanks convert onto it exactly. Without DWM timing the grid is 60 Hz.
- **Targets and hold:**
  - A frame starting at `t` targets the first vblank after `t`. If that would be less than half a period after the previous target, it takes a whole period after it instead.
  - The **hold** is the earliest start of the next frame: the target, or `t` + half a period if that is later. So no two frames start within half a period.
- **Waiting** (`wait`, before every dispatched message):
  - With a request and the hold not passed, it waits on a high-resolution waitable timer set for the hold.
  - After the hold it waits on the frame-latency waitables of the **pacing surfaces**: visible, not minimized windows with a waitable, only the invalid ones if any are.
  - A surface with a ticket, or no pacing surface at all, runs the frame at once via a `msgFrame` sent to the listener, so panics follow the window-procedure path.
  - Every wait is `MsgWaitForMultipleObjectsEx(QS_ALLINPUT, MWMO_INPUTAVAILABLE)`, so messages end it. Idle (no request) waits on the queue alone.
- **Off screen:** when nothing is paintable, the hold is at least `hiddenInterval` (250 ms) after the previous frame's start. `arm` recomputes it before every wait, so a restore returns to vsync at once.
- **Waitable pacing:** after a frame that presented, with all pacing surfaces having waitables, on screen and outside a modal loop, the hold shrinks to half a period. The waitables alone then gate the next frame. This matters with several monitors, where DWM's grid differs from the displays' and holding to it starts frames late. A frame that presented nothing is still held to its target.
- **Tickets and debts:** the waitable is a semaphore that starts at 1 and gains one signal per `Present`. A signal the loop takes is the surface's ticket, spent by its next `Present`. A `Present` without a ticket (a resize paint, an OS `WM_PAINT`) is a debt, which later signals pay off first. So the semaphore and the bookkeeping stay equal.
- **Stalls:** if the waitables stay unsignaled for `stallTimeout` (100 ms) from the start of the wait, every waited surface gets a ticket and its debt is cleared. The deadline is not extended by messages.
- **Modal loops** (size/move and menu loops, where `wait` doesn't run):
  - A request starts the listener's `WM_TIMER` for the rest of the hold. A posted `msgFrame` is only a fallback for when `SetTimer` fails.
  - `modalFrame` runs a frame only when the hold has passed and a pacing waitable has signaled (`chainReady`, a zero-wait poll with the stall deadline). Otherwise `pollModal` re-checks on the next tick.
  - Invariant: modal frames never block in `Present` and are never posted back to back. Posted messages beat input in the modal loop and would starve the drag (frozen mouse, lost snap). `WM_TIMER` comes only when no input is queued.
  - Leaving the loop kills the timer. The paints that matter in a size loop come from `WM_SIZE`.
- **Observers:** `FrameTiming` is measured with QPC. `pacing.go` maps presents (`GetLastPresentCount`) to `GetFrameStatistics` for `PresentTiming`. On a composition chain the newest shown present is about two presents back.
- **No allocations between frames** (review P17): the pacing set, the wait handles and the paint pass's window list are scratch slices on the shell and clock, and the idle steps iterate the windows with `allWindows` instead of copying them (`TestMessageLoopAllocs`).
- **Files:** `clock.go` (time, targets, `arm`, `frame`, `wait`), `clockmodal.go` (the modal loops), `waitable.go` (the pacing set, tickets and debts, the waitable objects), `pacing.go` (present feedback).
- **Test counters:** `clock.stats` (`clockStats`: `frames`, `waits`, `chainWaits`, `stalls`, `modalFrames`, `hiddenFrames`, `waitPaced`); per window `presents`, `ticket`, `debt`. `timing` is replaceable.

#### D2D canvas (`canvas.go`, `canvasdraw.go`, `canvasres.go`, `rescache.go`, `shadow.go`, `blur.go`, `image.go`)

`canvas` implements `render.Canvas` on the shell's device context for one frame. A device has one canvas (`gpu.canvas`), reused by every paint; `newCanvas` panics while it is live (review P3):
- `newCanvas` runs after `BeginDraw`.
- `finish` runs before `EndDraw`: it pops every open D2D push, resets the transform, disables the canvas and returns the recorder's errors.
- `release(leak)` runs after `EndDraw`, frees the frame's geometries and trims the device caches.
- `failures` returns resource-creation errors.
- Files: `canvas.go` (lifecycle, state, transforms, clips, opacity), `canvasdraw.go` (fills, strokes, text), `canvasres.go` (brushes, stroke styles, geometries, the float32 narrowing).

- **State:**
  - The transform lives in Go (`want`) and is sent with `SetTransform` only before a draw or push and only when it differs from what D2D has (`sent`, `drawDC`; review P4, `TestCanvasTransformCalls`). The blur's own transforms go through `sendXf`, so `sent` stays true. User space is logical points; the scale stays in `SetDpi`.
  - Each `Save`/`PushOpacity`/`PushBackdropBlur` frame records the transform and the count of open D2D pushes. `Restore`/`Pop` close only a frame of their own kind and pop the pushes made since.
  - `ClipRoundedRect`: a sharp rect under an axis-aligned transform uses `PushAxisAlignedClip`; anything else a `PushLayer` with a geometry mask. An invalid rect (`render.ValidRect`) or one that does not narrow to float32 clips everything away.
  - Every push is recorded as a `canvasPush`, so it can be pushed again (needed by the backdrop blur).
- **Drawing:**
  - Sharp, uniform-radius and per-corner shapes use `FillRectangle`, `FillRoundedRectangle` and geometries; strokes use `DrawRoundedRectangle` or `DrawGeometry`.
  - Paths become `ID2D1PathGeometry` with winding fill.
  - Stroke styles are mapped per the §8.2c enum traps; dashes are divided by the width.
- **Group opacity:** `PushOpacity` is a `PushLayer` with infinite bounds on the same stack; nested layers multiply. An opacity of 1 pushes no layer.
- **Shadows** (`shadow.go`):
  - Invalid or empty shadows draw nothing.
  - An opaque white mask is drawn on `gpu.mask` at the surface DPI. Outer: the shape grown by `Spread`, padded by 3σ + 1. Inset: a ring around the shape shrunk by `Spread`.
  - A `CLSID_D2D1Shadow` effect (σ = `Blur/2`) is **baked** once into a bitmap of the mask's size, then drawn with `DrawImage` under the current transform. It is clipped to the outside of the shape (outer) or the shape itself (inset).
  - Masks and baked shadows are cached across frames in `gpu.shadows`. An entry unused for `shadowKeep` = 240 paints is dropped. Beyond `shadowMaxMasks` = 256 or `shadowMaxPixels` = 8 Mpx, the least recently used masks go with their shadows, never one used in the current paint. The cache is freed on device loss or when the last surface closes. A steady frame creates no bitmap and runs no effect.
- **Backdrop blur** (`blur.go`):
  - σ = `BlurRadius/2` × √|det| of the transform. The region is the transformed shape's device bounds padded by 3σ, clipped to the target.
  - `CopyFromBitmap` inside an open layer reads the layer's own content, so the open pushes are popped, the context is flushed, the region is copied into a persistent scratch bitmap (`surface.backdrop`), and the pushes are reopened.
  - Crop and Gaussian blur effects (hard borders) blur the copy. It is drawn into the shape's clip, which stays open until `Pop`.
  - A zero or invalid radius only clips. Without a `blurTarget`, blurs only clip.
  - The scratch bitmap grows as needed (the old one is retired to the frame's resources), is recreated for another DPI, and is freed by `releaseChain`.
- **Images** (`image.go`):
  - `shellImage` holds premultiplied BGRA on the CPU, a pixel size and a scale. There is no public API yet; the shell makes them with `Shell.newImage`.
  - The D2D bitmap (96 × scale DPI) is created on first draw per `gpu` and kept. `releaseGPU` drops it on device loss and after the last surface; no finalizers.
  - Drawn with `DrawBitmap` (linear) into `dst`. Images of another set, nil, or an invalid `dst` draw nothing.
- **Device resource cache** (`rescache.go`, `gpu.res`, review P3): solid brushes, gradient stop collections (gamma 2.2, clamp), gradient brushes (no stops draws nothing; one stop is a solid brush) and stroke styles are kept across paints, so a steady frame creates none (`TestCanvasResourcesAcrossFrames`). `release` trims it after `EndDraw`: an entry unused for `resKeep` = 240 paints goes, beyond `resMax` = 256 per kind the least recently used go (`TestCanvasResourceBounds`). A stop collection is ref-counted by its gradient brushes. Lookups don't allocate (gradient keys are built in a scratch buffer, dash keys interned). The cache goes with the device. Geometries are made per call and freed in `release`.
- **Misuse is ignored** before it reaches D2D: semantic colors, non-finite values (including float32 overflow), negative sizes, nil layouts and paths, mismatched `Restore`/`Pop`, leftover saves. With `debug` every call is also mirrored to a `record.Canvas`, which reports them.

### 8.4 `shell/gtk4/gtk`: raw GTK bindings

The C bindings the gtk4 shell calls. Depends only on `internal/ffi`.

* **Loading:** `Load()` runs once and keeps the libraries open. It dlopens GTK 4, GObject, GLib, graphene, Pango, PangoCairo, cairo, PangoFT2 and fontconfig by soname and binds every function. It requires **GTK ≥ 4.14** (D7) and **Pango ≥ 1.52**. The one optional binding is Pango 1.56's `pango_font_map_add_font_file`, nil when absent.
* **Errors:** a `Load` error wraps `ErrMissing`. It is a `*MissingError` (names the soname and the distro packages) or a `*VersionError`. `Report(err)` writes the message to stderr, and also shows it with `zenity` or `kdialog` if one is on `PATH` (no shell involved).
* **Tables** (`bind.go`): one struct of func fields per C prefix. A field is the C name in CamelCase (`Gtk.WindowNew` = `gtk_window_new`). No C variadic is bound: the shell uses `g_object_new_with_properties` and `g_signal_connect_data` instead. **Only `GLib.IdleAdd` may be called off the GTK thread.**
* **Mirrors** (`types.go`, LP64): the C structs the shell reads or passes, e.g. `RGBA`, graphene rects, `RoundedRect`, `GValue`, the Pango attribute headers. They are `structs.HostLayout` and hold no Go pointers. A `graphene_matrix_t` (64 bytes, 16-aligned) goes to GTK in `g_malloc` memory.

### 8.5 `shell/gtk4`: the Linux shell

`shell.Shell` over GTK 4, without `GtkApplication`. The logic is in untagged files behind small unexported interfaces (`loop`, `window`, `popover`, `snapshotter`, `pangoLayouter`, …), so it is host-tested over fakes on every OS. The `*_linux.go` files call GTK. `godbus/dbus/v5` is used only for the settings portal. **Built and host-tested only. The Linux-native tests in `scripts/native.sh` wait on a run by the user.**

**FFI core**

* **Trampolines** (`trampolines.go`): a fixed set of 17 callbacks, created once per process. Signal handlers are grouped **by C signature**, not by signal (`sigBool`, `sigVoid`, `sigPointer`, `sigDoubleDouble`, …). The other callbacks are:
  * the GGSurface `class_init`, `size_allocate`, `measure` and `snapshot`;
  * one `GClosureNotify` and one `GSourceFunc`;
  * the tick callback and its `GDestroyNotify`;
  * a one-shot `GAsyncReadyCallback`.

  A test pins the set, so a new signal must reuse a signature where it can.
* **Dispatch** (`dispatch.go`): one process-wide `ffi.Handles[entry{peer, kind}]`. A handler's `user_data` and the GGSurface's handle word are handles into it, and `kind` tells apart callbacks of the same signature. A stale or zero handle reaches no peer and returns the zero result. The handle is freed by `closureNotify` when GLib drops a handler, by `sourceFunc` when a source ends, or by whoever calls `g_source_remove`. Signals are checked with `g_signal_lookup` before connecting.
* **Panics** (`panic.go`): every trampoline defers `trap`. It keeps the first panic and quits the loop, and `rethrow()` re-raises it after the native call returns. No panic unwinds through GTK. A panicking callback returns the zero result, except `close-request`, which returns TRUE, so GTK never destroys a window on its own.
* **GGSurface** (`class.go`): a `GtkWidget` subclass (CSS name `ggsurface`) with a handle word after its instance struct.
  * **Layout guard:** registration refuses a `GtkWidgetClass` size other than 408 (LP64, GTK 4.0–4.18).
  * **Vfuncs:** `class_init` writes the slots at 200 (`size_allocate`), 232 (`measure`) and 320 (`snapshot`).
  * **Snapshot** chains up to `GtkWidget`'s, also after a panic.

**Shell, clock, timers**

* **`Shell`** (`shell.go`, `wake.go`):
  * **Run** (only once) locks the thread and binds the queue. It then runs `gtk.Load`, `gtk_init_check` (whose error names `DISPLAY`/`WAYLAND_DISPLAY`), class registration and `g_main_loop_new`. After that it opens the appearance source, calls `Ready` and runs the loop.
  * **Wake:** each coalesced wake adds one `g_idle_add` source (callable from any goroutine) that drains the queue once.
  * **`Quit`** sets quit first, so a `NewSurface` gets `ErrQuit`. It then closes the surfaces last to first with `Closed`, and quits the loop.
  * **Teardown** (deferred, also on panic) removes every source and frees its handle, closes the appearance source and releases textures and Pango.
  * **Layout reentrancy:** inside a `Resized` from `size_allocate`, the native parts of sizing, `Close` and `Quit` are posted. The Paint guard is as on Win32.
* **Frame clock** (`clock.go`): on `g_get_monotonic_time`, the timeline of GDK's frame times.
  * **On screen:** frames run from a **tick callback** on the first shown, non-minimized surface. It runs in the UPDATE phase of GDK's frame, so the paint pass draws in the same frame. The target is GDK's predicted presentation time. The first tick without a request removes the callback, so **an idle clock has no tick**.
  * **Watchdog:** compositors send no frame callbacks to hidden windows. If ticks stop for 100 ms, a lazy watchdog runs the frame. It costs no GLib source per frame.
  * **Hidden or no pacing widget:** a one-shot timeout on a 60 Hz grid. With surfaces open but none on screen, or after a stall, frames are at least 250 ms apart (4 Hz). A show or restore returns to ticks at once.
  * **Targets** (pure `frameTimes`) are the next vblank, always increasing, at most two periods ahead.
  * **Requests** are coalesced. A request from inside a frame runs after it.
  * **Present feedback:** `pacing.go` feeds `GdkFrameTimings` to `SetPresentObserver`.
* **Timers** (`timer.go`): a `shellkit.Timers` and **one** GLib timeout for the earliest deadline (GLib orders ready sources of equal priority as it likes). One round of the due timers fires in (deadline, creation) order, then the timeout is re-armed.

**Surfaces and input**

* **Surfaces** (`surface.go`, `window_linux.go`): a `GtkWindow` whose title bar is an invisible GGSurface. That keeps GTK's CSD shadow and resize borders and takes no space. The content is a GGSurface whose peer is the `*surface`.
  * **References:** the window holds references to its GTK objects until `destroy`, which disconnects every handler first.
  * **Sizes:** logical pixels are points. The drawable size is the GGSurface's allocation. `SetSize` reports the clamped size at once.
  * **Not available:** GTK 4 has no max-size hint and cannot place toplevels, so `Center` is a no-op.
  * **Scale:** `gdk_surface_get_scale`.
  * **Close:** `close-request` asks `CloseRequested`, closes the surface itself and returns TRUE.
  * **Paint:** each invalidated, shown surface gets a `queue_draw`, and GTK snapshots it in the same frame. The snapshot calls `Paint` on the canvas, kept balanced if `Paint` panics.
* **Chrome regions** (`chrome.go`): `RegionCaption` acts like `GtkWindowHandle`, using a drag gesture and a click gesture on the GGSurface.
  * A press past the drag threshold calls `gdk_toplevel_begin_move`.
  * A double press zooms. The two gestures are matched by event time, since their order is undocumented.
  * Resize borders are GTK's, in the CSD shadow.
* **Pointer** (`input.go`): a motion controller and an all-buttons click gesture.
  * **Buttons and capture:** GDK buttons 1/2/3/8/9 map to the shell's buttons. Capture is GTK's implicit grab. A second button resets the gesture, so chords end the first button.
  * **Clicks:** `shellkit.ClickCounter` with `gtk-double-click-time` and `gtk-double-click-distance`.
  * **Modifiers:** Super/Hyper → Command, Alt → Option.
  * **Event times** map monotonically onto the clock.
  * **First click:** GTK has no `WM_MOUSEACTIVATE`, so the first press while not key, or within 500 ms after `is-active`, is `Activating`.
* **Scroll** (`scroll.go`): a kinetic two-axis scroll controller.
  * **Units:** deltas are negated. Surface units are precise points; wheel notches are 3 lines. Shift+wheel scrolls horizontally. Precise gestures get phases.
  * **Momentum:** the shell runs it itself, since GTK leaves kinetic motion to widgets. It is `v0·e^(−4t)`, capped at 15000 pt/s, stopping at 20 pt/s, and stepped in each paint pass.
* **Keyboard** (`keyboard.go`): a key controller on the **GtkWindow** in the CAPTURE phase, ahead of GTK's bindings.
  * **Key events:** a consumed `KeyEvent` returns TRUE and resets the input method. An unconsumed key goes to the input method, whose `commit` becomes a `TextEvent`.
  * **Key mapping** (`keyFor`, pure Go): from the level-0 keyval of the keycode in the event's layout group. `Shifted` comes from the Shift level. `Modifier` comes from the base keyval; AltGr is no modifier.
  * **Repeat:** a key-down of a keycode already held.
* **IME** (`ime.go`): a `GtkIMMulticontext` (IBus, Fcitx or GTK's simple context) with use-preedit. Aqua draws the preedit.
  * **Preedit:** the preedit signals become `PreeditEvent`s. `preeditOf` (pure Go) cuts segments at the attribute bounds, and a background or foreground attribute marks a segment `Selected`.
  * **Input on:** `SetTextInput` attaches the context to the key controller, so GTK filters every key through it first. It also sets the cursor location from the caret and focuses the context in the key window. Turning input off resets, unfocuses and detaches.
  * **Muting:** `SetTextInput`, `CancelComposition` and key loss run muted, because aqua commits its own copy.
  * **Release:** the context is released after the current event, since a handler may close the surface.
  * **Not supported:** surrounding text is not answered.
* **Popups** (`popup.go`, `popover_linux.go`): a `GtkPopover` without arrow, parented to the parent's GGSurface. Its child is a second GGSurface acting as a surface of its own. One display-wide CSS class clears the popover's padding, background and shadow.
  * **Placement:** GTK places it (`GTK_POS_BOTTOM`, halign START, `pointing_to` the anchor). GTK's `FLIP_Y | SLIDE_X | RESIZE` hints stand in for `Adjust`. `Placed` comes from the child's allocation and `gdk_popup_get_position_*`, and the first paint waits for it.
  * **Grab:** autohide. GTK propagates grabbed events only downward, so the pointer, scroll and key controllers sit on the popover. Parent events that still arrive are forwarded, keys deduplicated. `closed`, the parent's key loss, minimize and resize count as `Dismissed`.
  * **Track:** moves and scrolls over the track are forwarded to the popup, but **GDK's autohide eats a press on the track and dismisses** (a known limitation).
  * **Non-grab:** autohide off, `can-target` false.
  * **Close** disconnects before popdown, so it is no dismissal. `Translucent` and `Caps().PopupShadow` are false.
* **Modal surfaces** (`modal.go`): a non-resizable GtkWindow with `gtk_window_set_modal`, transient for `Owner` or (app-modal) for the key surface at the first `Show`. The window manager places it.
  * **Blocking:** every other surface is `set_sensitive(false)`. Their leftover input is dropped in Go, and their activation and close-request present the modal.
  * **Closing:** the others are re-enabled before the window is destroyed. The key is handed on after `Closed`; on Wayland that is only a request.

**Drawing and text**

* **Canvas** (`canvas.go`, `blur.go`, `image.go`): `render.Canvas` over GtkSnapshot in logical pixels; GTK applies the scale. Clips are rounded-clip pushes, popped before `gtk_snapshot_restore`.
  * **Fills, strokes, paths:** color or gradient nodes, border nodes for solid strokes, and a `GskPath`/`GskStroke` per path call.
  * **Shadows:** GSK shadow nodes, with CSS box-shadow semantics like `render.Shadow`.
  * **Images:** a `GdkMemoryTexture` made on first draw, kept per shell and released when the last surface goes. No finalizers.
  * **Backdrop blur:** GSK has none. The canvas draws in segments and re-appends the earlier nodes under `push_blur`, mirrored at the surface edges as on Win32.
  * **Text:** the `PangoLayout` is rebuilt per paint and drawn with `gtk_snapshot_append_layout`.
  * **Contract violations** (non-finite values, invalid geometry by `render.ValidRect`/`ValidPoint`, semantic colors, unbalanced pops) never reach GTK; an invalid clip or blur rect clips everything away.
* **Text system** (`text.go`, `textgeom.go`): UI-thread only.
  * **Context:** one `PangoContext` of the default PangoCairo font map, with glyph rounding off, hint metrics off and LTR base (as DirectWrite).
  * **Layouts:** a layout is laid out once and kept as Go data (lines, grapheme clusters, baselines) over byte offsets, with Win32's hit-test, caret and selection rules.
  * **Fonts:** a family name the font map lacks becomes the UI family: Inter, else `gtk-font-name`, else `sans-serif`.
  * **Fallback:** without Pango or on failure, layouts are `shellkit.FixedText`'s.
* **App font** (`appfont.go`): fontconfig loads only files, so Inter is cached as `os.UserCacheDir()/gogui/fonts/<sha256><ext>`, written atomically. It is added with `pango_font_map_add_font_file`, else with `FcConfigAppFontAddFile`. On any failure the system font stays.
* **Clipboard** (`clipboard.go`, the GDK calls in `clipboard_linux.go`): the display's `GdkClipboard`. Writes are sanitized by `shellkit.ClipboardText`. Reads are `gdk_clipboard_read_text_async` with a handle as `user_data`. The trampoline always finishes the read and frees the handle, and the result is delivered through `Post`.

**Appearance** (`portal.go`, `portalclient.go`, `gtksettings.go`)

* **Sources:** the settings portal (`org.freedesktop.appearance`, over godbus) and the default GtkSettings for what the portal lacks.
* **Start-up:** both are read once before `Ready`; the portal read is bounded by 250 ms.
* **Changes:** `SettingChanged` (godbus goroutine) and the watched `notify::` signals (GTK thread) only post a re-read. The re-read runs on the UI thread, is deduplicated with `==`, and calls `AppearanceChanged` after `Ready`.
* **Missing sources:** no bus, portal or property means the defaults.
* **Portal client:** the match rule is added before reading, so no change is lost. `Close` waits for the watch goroutine.
* **Mapping:** a portal value that is unset or invalid falls back to GtkSettings, then to `driver.DefaultAppearance()`.

  | Field | Portal | GtkSettings |
  |---|---|---|
  | `Dark` | `color-scheme` = 1 | `gtk-application-prefer-dark-theme` |
  | `Accent` | `accent-color` (in 0..1, opaque) | — |
  | `IncreaseContrast` | `contrast` = 1 | — |
  | `ReduceMotion` | `reduced-motion` = 1 | `gtk-enable-animations` false |
  | `LegacyScrollers` | — | `gtk-overlay-scrolling` false |
  | `CaretBlink` | — | `gtk-cursor-blink`, `-time`, `-timeout` |
  | `ReduceTransparency` | — (always false) | — |

**Golden tests** (`readback.go`, test-only):
* **Fixed state:** `setFixedScale` and `setFixedKey` fix the scale and key state.
* **Readback:** `readback()` re-renders the GGSurface's last render node with the window's renderer into BGRA `g_malloc` memory.
* **Gallery goldens:** they run only when `native.sh` sets `GOGUI_GOLDEN_OUT`, and their Linux tolerance is still provisional.

### 8.6 The IME path end to end

* **Shell contract (§8):** text input stays off until `Surface.SetTextInput(TextInput{Enabled, Caret})`. The caret is in surface points and places the candidate window.
  * The composition arrives as `PreeditEvent`s; an empty one ends it. The commit is an ordinary `TextEvent`, which also empties the preedit, so either order works.
  * Keys the IME takes are not delivered. `CancelComposition` delivers nothing.
  * Both calls are UI-thread only, emit nothing inside themselves, and are safe during `Paint`.
* **Aqua (§9):** input is on only while the window is key and focused on an enabled `TextInputClient` (an editable, non-secure TextField, or a TextView). It is published after paint, focus and key changes, deduplicated.
  * The field draws the preedit; the selected clause gets a 2 pt underline.
  * On blur, click, key loss, paste or `SetEditable(false)`, the field cancels the composition and commits its own copy, so no late shell commit lands elsewhere.
* **Win32 (§8.3):** IMM32. `WM_IME_COMPOSITION` becomes the events and never reaches `DefWindowProcW`.
* **gtk4 (§8.5):** a `GtkIMMulticontext` filtering every key while on.
* **Not supported (v1):** surrounding text, reconversion, TSF, inline candidates, IME in SecureField.

### 8.7 The popup path end to end

* **Shell contract (§8):** `PopupShell.NewPopup(parent, PopupConfig, PopupHandler)` opens a borderless surface owned by a shown parent. It is never key and closes with the parent.
  * **Placement** works like `xdg_positioner` without flips: the anchor plus `Offset`, with `Adjust` allowing slides and shrinking in the work area. `Placed` is reported before the first paint.
  * **Grab** (one per shell): the popup takes the pointer and the parent's keys, text and preedit, counts its `Track` as its own area, eats an outside press, and is `Dismissed` once. A non-grab popup takes no input.
  * **Flags:** `Translucent` says whether the OS blurs behind it, `Caps().PopupShadow` whether the OS draws its shadow.
  * **Fake and Win32** share `shellkit.PlacePopup`.
* **Win32 (§8.3):** a never-activated `WS_POPUP` with a drop shadow. The grab is mouse capture plus the parent routing its input. It has an Acrylic backdrop from build 22621 on.
* **gtk4 (§8.5):** a GTK-placed `GtkPopover` with autohide as its grab. A press on the track dismisses it.
* **Aqua (§9):** a `popupRoot` per popup holds its own tree, router and paint, and shares the window's context. The generic `menu` is the grab content, also opened by `PopUpButton`. Tooltips are non-grab popups.

### 8.8 The menu strip path end to end

* **Model (§9.3a):** `menubar.Build(spec, appName)` returns the `Bar`: titles, shortcuts with Windows/Linux text, roles and disabled flags. There are no cascading menus in v0.1.
  * `Bar.Lookup` indexes accelerators: exact matches before shifted ones, the first item wins.
  * `Layout` drops titles from the right so a drag area stays.
  * Every titled window shows the bar (D11).
* **Shell contract (§8):**
  * **`PopupConfig.Track`:** the strip counts as the grab popup's area.
  * **`Popup.Reposition`:** moves the popup and keeps the pointer.
  * **`KeyEvent.Shifted`:** the Shift-only character, for shortcuts like Primary+'+'.
  * **`KeyEvent.Modifier`:** a lone Alt versus Insert or F13.
  * **Regions:** titles are `RegionClient` inside the `RegionCaption` bar.
* **Win32 (§8.3):** titles answer `HTCLIENT`, the rest `HTCAPTION`. `Shifted` comes from `ToUnicodeEx` with `TOUNICODE_NOSTATE`. A handled lone Alt or F10 never reaches `DefWindowProcW`.
* **gtk4 (§8.5):** **a press on the strip dismisses the open menu**, so a second click opens the other menu.
* **Aqua (§9):** `Driver.SetMenuBar` puts a `menuStrip` into every titled window.
  * **Mouse:** a press opens the `menu` in a grab popup with the strip as `Track`. Hover `Reposition`s it.
  * **Picks:** enablement is checked at opening and before a pick. A pick goes to `MenuItemSelected` or `runRole`.
  * **Paste is always enabled** in editable text (`textEdit.canEdit`), whatever the clipboard holds: `ReadClipboardText` delivers later from the shell queue, so the validation at opening cannot read it, and a paste of no text does nothing (review D10; AppKit disables Paste for an empty pasteboard, a deliberate difference).
  * **Keys:** first `menuAccess` (Alt/F10), then the accelerators (run, flash 100 ms, consume), then the router.

### 8.9 The alert path end to end

* **API and driver:** `Show` panics outside `Run`. The `AlertSpec` defaults to `["OK"]`. `Cancel` comes from `Alert.Cancel` (zero: the first button titled exactly "Cancel"), else -1. `done` runs once on the UI thread with the button index.
* **Element (`alert.go`):** the NSAlert panel from `Theme.Alert`, 260 pt wide, with its own router.
  * **Keys:** Return presses the default button; Escape and Primary+. press Cancel.
  * **Accessibility:** `RoleDialog`.
* **Sheet (`sheet.go`):** one shown at a time, sliding from the title bar in 200 ms (a fade with Reduce Motion).
  * **While shown:** the window's content is modal, and close and the Edit roles are disabled. The title bar and the strip keep working.
  * **Closing the window** answers pending alerts with Cancel.
  * **Small window:** a window too small for the panel uses an owned modal surface.
* **Shell contract (§8):** `SurfaceConfig.Modal` and `Owner` (nil = app-modal). The surface is fixed-size and centred, with no taskbar entry.
  * **Blocking:** from its first `Show` it blocks every other surface, and their activation is redirected to it.
  * **Closing:** the key goes to the next modal, else the owner, else the previous key surface. The owner's modals close first.
* **Win32 (§8.3):** `EnableWindow(false)`. The others are re-enabled **before** `DestroyWindow`.
* **gtk4 (§8.5):** modal plus transient-for, with the other windows set insensitive.
* **App-modal alerts (`alertsurface.go`):** with no window, the alert opens in a modal surface sized to the panel. Only Quit stays enabled while it shows. About is one of these.

### 8.10 The appearance path end to end

* **Value (§2.2):** `driver.Appearance{Dark, Accent, IncreaseContrast, ReduceMotion, ReduceTransparency, LegacyScrollers, CaretBlink}`, compared with `!=`.
* **Win32 (§8.3):** the theme, accent, high contrast, animation, transparency, scrollbar and caret settings, re-read on `WM_SETTINGCHANGE`, `WM_DWMCOLORIZATIONCOLORCHANGED` and `WM_THEMECHANGED`.
* **gtk4 (§8.5):** portal plus GtkSettings. There is no Reduce Transparency source.
* **Driver (`driver.go`):** `merged` applies the forced mode and the app accent. `update` sets the timeline's Reduce Motion, and every window and alert surface repaints and sends a `WindowStateEvent` to every element. Live equals a fresh start (`TestLiveAppearance`).
* **Theme (§9.1):** the ring, selection, pressed, inactive and text colors derive from the accent. Outlines show with Increase Contrast.
* **Key state:**
  * **Inactive window:** no accent colors, no ring and no caret.
  * **First click:** the activating press is `PointerEvent.Activating`, and the router drops it except in the title bar.
* **Motion and transparency:**
  * **Animations:** all of them run on the timeline or a listed path (`TestMotionInventory`).
  * **Reduce Transparency:** makes menus opaque and turns the Win32 backdrop off.

### 8.11 The frame budget end to end

* **No requests, no frames:**
  * **Clocks:** all of them are one-shot and coalesced.
  * **Invalidate in a frame:** joins that frame.
  * **Idle audit:** `TestIdle*` checks that every example, in every state, has nothing requested or armed after 11 s. Only indeterminate indicators loop.
* **Off screen:** at most 4 Hz (250 ms) while surfaces are open but none is visible. Showing a surface returns to vsync at once.
* **On screen:** frames target vblanks. v1 repaints the whole window (PLAN.md §5.1) and caches unchanged work: shadows, bitmaps, text layouts, truncations, and layout that runs only after a change. Dirty rectangles are v2.
* **Budget:** **8.3 ms of CPU per frame** (120 Hz).
  * **Frame log:** `SetFrameObserver` times each part, and `GOGUI_DEBUG=1` logs every frame over `frameBudget`.
  * **Allocations:** guarded by `TestFrameAllocs`.
* **Pacing:** `SetPresentObserver` and `shell.Pacing`. A scenario may miss a fifth of its vblanks (`PacingTolerance`), a run 2 % (`PacingRunTolerance`).
* **Idle CPU:** `scripts/idlecpu.sh`. On Windows, `native.sh` fails `hello`, `form` and `notes` above 1 % of a core. On Linux it only logs.

## 9. `internal/aqua`: the drawn driver

The element core of the drawn driver for Windows and Linux (PLAN.md §5.1). No build tags; depends only on `driver`, `driver/stub`, `layout`, `mainthread`, `render`, `shell`, `aqua/a11y`, `aqua/anim`, `aqua/scroll`, `aqua/menubar`, `aqua/text` and `aqua/theme`. UI thread only, except `Driver.Queue`.

**Element model**

* **`Element`** = `layout.Item` (`FittingSize`, `SetFrame`) + `Paint(render.Canvas)` + `HitSelf(p)` + `HandleEvent(Event) bool` + `Activate() bool` (the primary action, as a click: a11y press, Space, Return/Escape on a default/cancel button) + `AccessibleNode()` + `Cursor()`. Sealed: a type embeds **`Base`** by value and calls `Init(self)` once (tree ops, `PaintTree` and `HitTest` panic otherwise). `Base` defaults: zero size, no paint, `HitSelf` = `Bounds().Contains`, no events, `Activate` false, `RoleNone`, `CursorArrow`. Elements never lay themselves out: the public layer runs `internal/layout` and calls `SetFrame`.
* **Tree:** ordered children (paint order), one parent each: `AddChild`, `InsertChild`, `SetChildren` (detaches the dropped, steals the rest), `RemoveChild`, `RemoveFromParent`, `Parent`, `Root`, `Children`, `Child`, `IndexOf`. Nil, uninitialised, repeated, self or ancestor children panic before anything changes. The children slice is copy-on-write (review P8: an append writes past every snapshot's length, every other change builds a new slice), so `PaintTree` reads it without a copy; `SetChildren` validates each old router once and checks long lists for repeats with a map (P19). `Container.AppendChildren` appends elements without resending the list (P19).
* **State:** `Frame` (parent coordinates), `Bounds` (origin 0), `SetHidden`, `SetEnabled`, `SetClipsChildren`, `VisibleInTree`, `EnabledInTree`, `SetTooltip`, `SetAcceptsFirstMouse` (inherited), `SetFocusable`, `SetFocusKind`, default/cancel marks. **Tree context memo** (`treecache.go`, review P9): each `Base` memoises its root, root-space origin, `EnabledInTree` and `VisibleInTree`, refilled from its parent's memo; a process-wide atomic `treeEpoch` (several drivers may run trees in one test process), bumped by every parent-link, frame, hidden, disabled or router change, makes all memos stale at once. `Root`, `ToRoot`/`FromRoot`, `routedBy`, `rootContext` (so `Window`, `appearanceIn`, `scaleIn`) read it: O(1) per query between changes instead of a walk to the root. Tests that poke `disabled` directly call `treeChange()`.
* **Invalidation:** setters invalidate only on a change; tree changes invalidate old and new parent. `Invalidate` marks the element and its ancestors dirty (invariant: a dirty element's ancestors are dirty) and calls the root's `SetInvalidateFunc` only on the root's clean→dirty transition, so one callback per frame. New elements start dirty. The whole window repaints; the marks only coalesce. The dirty flag is cleared before `Paint`, so invalidating during `Paint` schedules another frame (animations).
* **`PaintTree(c, root)`:** per visible element `Save`, translate to the frame origin, `Paint` in its own `Save`/`Restore`, clip to `Bounds` if `ClipsChildren` (never the element itself), children in order (list read once, so a `Paint` that edits the tree is safe), `Restore`. The canvas ends balanced. **Culling:** below a clipping ancestor, an element whose frame grown by `paintOverflow()` (focus ring / widest shadow from the tokens, plus margin) misses the accumulated clip is skipped — a clipping or childless element with its subtree, a non-clipping one only its own `Paint`. Unclipped trees are never culled. Culled and hidden elements end clean.
* **`HitTest(root, p)`:** topmost visible element under `p` (children last-first before the parent, `HitSelf` decides, overflowing children hit unless an ancestor clips, disabled elements hit — the router skips them), plus `p` in its coordinates. `ToRoot`/`FromRoot` convert.
* **Events** (`event.go`): `PointerEvent` (`PointerEnter/Leave/Down/Move/Up/Cancel`, `Pos` in element coordinates, `Over`, buttons, clicks, mods, time), `ScrollEvent`, `KeyEvent`, `TextEvent`, `PreeditEvent`, `FocusEvent{Focused, Tab}` (`Tab` when moved by `FocusNext`), and the unrouted `WindowStateEvent{Key, Appearance}`.
* **`AccessibilityTree(root)`:** a snapshot, one node per visible exposed element with `Frame` in root-parent coordinates and `StateDisabled`/`StateFocusable`/`ActionFocus`/`StateFocused` added; `RoleNone` elements are hoisted away, hidden subtrees dropped, the root always present. Elements whose children are not elements implement `accessibleItems` (the menu's items); overlays implement `accessibleOverlay` (the shown sheet). **`aqua/a11y`**: `Role`, `States`, `Actions`, `Node`, `Walk`/`Find`/`Count`/`Dump`. No OS accessibility bridge (D4).

**Router, focus, input** (`router.go`; `NewRouter(root)`, one per window, popup, sheet and alert surface; the root can never get a parent)

* **Hover:** the chain is the enabled part of the root→`HitTest` path. `hoverAt` fills a scratch buffer; an unchanged chain (a move inside the same element) sends nothing and allocates nothing (review P15). On change: `PointerLeave` deepest first to leavers, `PointerEnter` root side first to joiners; a handler that changes the chain stops the rest. `RefreshHover` after layout/tree moves (not during a capture).
* **Press/capture:** `PointerDown` goes to the deepest hovered element and bubbles; the first handler captures; if that handler removed, hid or disabled its own element (or called `CancelPointer`), nothing captures and the element gets `PointerCancel` at once, so it never stays pressed. Captured events go only to it with `Over`; hover and cursor are frozen. The last release ends the capture; a shell `PointerLeave` sends `PointerCancel`. `CancelPointer` (a grab popup opening) ends capture, hover and scroll latch at once (a press in flight captures nothing and gets `PointerCancel`). `SetStrayReleases` (grab popups) routes a release without a press like a move, or to the root with `Over` false.
* **First click:** a `PointerDown` marked `Activating` (it made the window key, §8.10) only activates: hover updates, no element gets it or its release. Subtrees with `AcceptsFirstMouse` (the title bar: lights and menu titles) are exempt. Same rule in sheets and alert surfaces.
* **Scroll** bubbles from the topmost element under the pointer. A precise gesture is **latched**: the element that handled `PhaseBegan` alone gets the rest and the OS momentum, wherever the pointer is, until the momentum ends or another scroll event starts; removing/hiding/disabling it drops the latch (the rest reaches nobody). `Router.Latched()`.
* **Keys, text, preedit** bubble from the focused element (or the root) through enabled ancestors; an unhandled preedit goes back to the shell. Unhandled Tab (± Shift only) → `FocusNext`; unhandled Space without modifiers on a usable `FocusControl` in an active router → `Activate` (consumed if true; repeats consumed, press nothing; `FocusText` keeps Space). Acts on key-down.
* **Disabled** elements and subtrees get no input.
* **Cursor:** the deepest hovered element's `Cursor`, applied through `SetCursorFunc` only on change.
* **State follows the tree:** removing, hiding or disabling an element or ancestor at once sends `PointerCancel` to the capturer, `PointerLeave` to dropped hovered elements and `FocusEvent{false}` to the focus (also when made unfocusable). Moves inside a tree keep state. `SetTreeChangedFunc` runs after every validation (must not change the tree).
* **Focus:** `Router.SetFocus` (nil clears; only focusable, visible, enabled, in-tree), `Focused`, `Base.Focus`/`HasFocus`. Old element gets `FocusEvent{false}` before the new one `{true}`; a handler that moves the focus wins. `FocusOrder` = pre-order Tab stops, hidden/disabled skipped, filtered by keyboard-access mode, a radio group counted once (`groupTabStop.groupStop`, looked up once per parent, review P14). `FocusNext(backward)` wraps; from a focus outside the order goes to the next stop in tree order. **Keyboard access:** `FocusControl` (zero) or `FocusText`; with `FullKeyboardAccess` off only `FocusText` elements are Tab stops (default on; no public API yet). `SetFocus` works in both modes. After a focus moves, ancestors implementing `focusRevealer` (scroll areas) bring it into view, innermost first; then the `SetFocusChangedFunc` hook runs (the window publishes text input there).
* **Active state:** `Router.SetActive` follows the window's key state (a window router starts inactive). It changes only `FocusState`: `Unfocused`, `FocusedInactive`, `FocusedActive`; the focus survives deactivation.

**Window context and look rules** (`state.go`, `contrast.go`)

* **Root context** (`rootctx.go`, review R1): `windowRoot`, `popupRoot` and `alertRoot` implement `rootContext` (`rootWindow`, `rootAppearance`, `rootScale`); `Base.rootContext()` reads the root from the tree memo. `Base.Window()` (nil when detached, under a bare router or in a windowless alert), `appearanceIn` (the driver's outside a root), `scaleIn` (1 without a live surface) and `FocusRing.Paint` all read it, so the rules for a closed window, an unplaced popup and an app-modal alert live next to each root (an app-modal alert's focus ring paints in the driver's appearance, as its buttons do). Elements read `IsKey`, `Appearance`, `Theme`, `Timeline`, `Router` through `Window`.
* On a key or appearance change the window invalidates everything and sends `WindowStateEvent` to every element in pre-order (hidden/disabled included, no bubbling), then to each popup and sheet; one `broadcastState(root, router, state, gone)` (review R3) does it for windows, popups, sheets and alert roots: it skips an element a handler moved out of the tree, stops once the tree closed, and builds each event from the current state; `WindowHandler.KeyChanged` comes after. Elements joining later read the state from `Base.Window`.
* **Live appearance:** `Driver.update` hands every OS appearance change to every window, popup, sheet and alert surface; elements resolve tokens at paint time. `TestLiveAppearance`: live equals a fresh start.
* **Inactive look:** in an inactive window nothing derives from the accent: accent fills become `AccentInactive` grey, the default button is plain, the pop-up indicator loses its fill, the selection is unemphasized, focus ring and caret are hidden. App-chosen `ColorControlAccent` keeps the accent, and so does an open menu's highlight. Pinned by `TestInactiveWindowMatrix`.
* **Increase Contrast:** borders/separators use their contrast values (`ColorToken.Value`); `paintOutline` adds a one-device-pixel inside outline (`BezelOutline`, `SliderTrackOutline`) around accent fills; focus ring = opaque accent, emphasized selection stronger (`Theme.FocusRing`, `Theme.Selection`). Pinned by `TestContrastMatrix`.
* **Reduce Motion:** every animation runs on the driver's `anim.Timeline` (jumps with Reduce Motion; running ones finish when it turns on) or a documented own path (sheet slide → fade, menu pick blink → none, accelerator flash → none, `scroll.Axis.FollowReduceMotion`). Not motion, so unaffected: tickers following the pointer (menu scroll arrows, text auto-scroll), caret blink, key flashes, tooltip delays, idle layout. `motionInventory` + `TestMotionInventory` (a `go/ast` scan) fail on any unlisted call that starts an animation or timer. **Reduce Transparency:** menus paint `Menu.OpaqueBackground` even in translucent popups.
* **Focus ring** (`focusring.go`): embeddable `FocusRing`; the element calls `Track(self, ev)` in `HandleEvent` and `Paint(c, self, shape)` last. On while `FocusedActive`: animates 0→1 over `FocusRingDuration` with `EaseOut` on the timeline (instant without timeline or with Reduce Motion), off at once. `FocusRingBand(p)` starts one width further out and twice as wide; `PaintFocusRing` strokes it outside the shape in `Theme.FocusRing(a)` × p.

**Control state** (`control.go`)

* `ControlState{Interaction, Focus, Disabled, Key, Appearance}` is computed fresh on each paint (`Base.ControlState`, `pressTracker.State`): pressed while holding a primary press with the pointer over it, else hover if in the hover chain; disabled-in-tree is always normal. `String()` for test dumps.
* `pressTracker.Track(self, ev) (handled, act)`: primary down captures, `Over` sets pressed, release over the control acts, `PointerCancel` drops; other buttons swallowed during a press. Controls with their own press rules (slider, scroller) use `Begin`/`End`/`Cancel` and `Down` instead.
* **Click core** (`clickControl`, review R2): embedded by the push button, checkbox, radio and traffic lights; `handleClick(self, ev, keys)` is their `HandleEvent` (ring, disabled drops the press, optional key hook, release over clicks), `activate(self)` their `Activate` (refused while disabled). **Key flash:** `activateByKey` (Return/Escape on the default/cancel button, Space on the focused control, the alert's keys) first flashes a `keyFlasher` (button, checkbox, radio) pressed for `keyFlash` (100 ms), as AppKit's `performClick:`, then activates; a click or a repeat does not flash.
* **Default/cancel buttons** (`buttons.go`): marks on elements with a global serial; `Window.DefaultButton/CancelButton` = the last-marked one in the tree. A key-down no element handled: Return → default, Escape or Primary+`.` → cancel, via `activateByKey` (flash, then `Activate`), only if visible and enabled; repeats consumed but press nothing.
* **Pixel helpers:** `scaleIn` (surface scale), `pixelWidth`, `pixelAlign` (snap to device pixels in root coordinates). Truncating controls keep a `truncMemo` (text system, text, font, width) so titles are not re-laid out each paint. Paths drawn in `Paint` live in a `pathMemo[K]` (`pathmemo.go`) on the element: rebuilt in place only when their key changes (checkbox mark, traffic-light glyph, pop-up chevrons), or reused as a `scratchPath` one draw at a time (menu arrows, checkmarks and chevrons; alert icons), since a `*render.Path` handed to a canvas escapes and canvases read it only during the call (review P16).
* **Drawing primitives** (`primitives.go`, review R4): `layoutMemo` (the one-line title of a button, checkbox, radio, pop-up button, box and a field's placeholder), `strokeInside(c, r, radius, bw, col)` (every inside border), `dimmed` (the value controls' disabled fade), `shadowInsets` (the own-shadow margin of menus, alerts, tooltips), `hairline` (the separator element and the menu separator), `systemFont(cs)` and `sizedFont(f, cs)` (the system font of a control size; a zero size filled in for labels, fields and text views; review R5).

**Roots: windows, popups, sheets, alert surfaces**

* **`Window`** (`window.go`, `NewWindow(sh, sv, cfg, h)`, `sv` the shell's `Services`): surface = content + `TitleBarHeight` (28 pt) for titled windows. Root children: content area (paints `ColorWindowBackground`, clips content), sheet layer, title bar (on top). `driver.Window` works in content coordinates. `Show` focuses the first Tab stop if nothing is focused. `SetContent` takes an aqua element; the old content's focus is dropped. After `Closed` every call is a no-op. The `shell.SurfaceHandler` adapter: `Paint` = `PaintTree` + `publishTextInput`; `Resized` re-lays the chrome and regions; `KeyChanged` → `SetActive`, repaint, `WindowStateEvent`, report; `CloseRequested` asks `ShouldClose` (refused while modal). Key-down order: menu keyboard access → accelerators → router → default/cancel buttons.
* **Text input** (`textinput.go`): `TextInputClient` (`TextInput() (enabled, caret)`). The window publishes `Surface.SetTextInput` (on with caret in surface points only for a key window whose focus is an enabled client) only on change, after each paint, focus change and key change (after the `WindowStateEvent`s, so compositions commit before the IME goes off).
* **Popup roots** (`popuproot.go`, unexported): `Window.openPopup(popupOptions{Config, Content, Placed, Closed, TrackEvent})` opens a `shell.Popup` with its own tree (root coordinates = popup points), router, cursor and paint. Its elements count as in the window (appearance, key, theme, timeline). Errors `ErrPopupParent`, `ErrPopupGrab` (one grab popup per window). A **grab popup** gets the window's pointer and keys; opening it calls `CancelPointer` on the window router, which then ignores pointer/scroll until it closes. `TrackEvent` forwards events over `Config.Track` (the menu strip) in surface points. `Close` is once, detaches the content and runs `Closed`; a popup closed from inside the shell's `NewPopup` (a `Placed` or `Dismissed` handler) also closes its shell popup before `openPopup` returns. A window closes all popups when it loses key and on close.
* **Tooltips** (`tooltip.go`): a per-window controller sees every shell event before routing (not during a grab popup). The tipped element is the deepest hit element (disabled included) or ancestor with a tooltip. Shown after `Tooltip.Delay` of rest, at pointer + offset, in a non-grab popup; `Grace` after leaving shows the next one at once. Press, key, text, preedit, scroll hide and mute it until the pointer leaves; key loss, close, grab popup end it. Only key windows show tips. No a11y node.
* **Sheets** (`sheet.go`): per-window alert queue, one shown at a time in `sheetLayer` (between content and title bar). The panel hangs centred from the title bar, slides over `sheetDuration` with `EaseInOut` on a ticker (Reduce Motion: fades in place). **Modal** from queueing to answer: popups closed, content router inactive (focus kept), content input swallowed, close light and Close/Edit roles disabled, no dimming. Pointer events go to the window router (title bar), then the sheet's; keys to the sheet only. `Window.Close` answers every unanswered alert with its Cancel index (0 without). A sheet larger than the content area opens in an owned modal alert surface instead.
* **Alert surfaces** (`alertsurface.go`): `Driver.openAlertSurface(root, owner)` shows an `alertRoot` as the whole content of a modal surface (owner nil = app-modal), no own shadow/border. Key-down → `Driver.accelerator(nil, k)` → router. `CloseRequested` presses Cancel and refuses. While any is open, only the Quit menu role is enabled. App-modal alerts queue in `Driver.appAlerts` (built lazily, after `Ready`); `Quit` answers the rest with Cancel.
* **Alert element** (`alert.go`): `newAlert(d, spec)` — NSAlert (macOS 11+) layout: icon, message (`Label`, emphasized), detail, push buttons (first is default; "OK" if none). Exactly two buttons that fit go side by side (default right), else stacked (default on top). Answered once (`answer`/`SetPressFunc`): click, Return (default), Escape/Primary+. (spec's Cancel), Space on the focused button; key presses flash the button. `alertRoot` has its own router; initial focus is the default's neighbour with full keyboard access; Tab cycles buttons. Icon is procedural (`paintAppIcon`, `paintCaution` for `AlertCritical`; `AlertWarning` draws as informational). A11y `RoleDialog`.

**Chrome and menus**

* **Chrome** (`chrome.go`): title bar fill (key/inactive), bottom hairline (whole device pixels on the pixel-aligned bottom edge, like every hairline), centred truncated title in `titleFont()`. Traffic lights (`Theme.Chrome` metrics: 12 pt circles at x = 8/28/48) in a hover group showing glyphs; grey when inactive unless hovered; disabled when the style flag is missing; `clickControl` → minimize, zoom, `ShouldClose`+close (also through `Activate`). Chrome regions: bar `RegionCaption`, menu titles `RegionClient`, enabled lights `RegionClose/Minimize/Maximize`.
* **Menu element** (`menu.go`, `menuinput.go`): generic over `[]menuItem` (title, shortcut text, disabled, checked, submenu chevron, separator) at a control size; geometry (`Panel`, `ItemRect`, `ItemAt`, shortcut column), the item tops summed once per item list and control size, `ItemAt` a binary search (review P12). Background `Menu.Background` when its popup is translucent, else `OpaqueBackground`; accent highlight. Optional own shadow (`SetShadow`) where the shell has none (gtk4). Interaction (`Begin(menuOpening)`): press-drag-release (release picks past `menuDragSlop` or `menuReleaseDelay`, else stays open for a click), hover highlight, keys (Up/Down, Home/End, Page, type-select, Return/Space pick, Escape cancel; Left/Right to the owner via `SetSideKeys`). A pick blinks (`Shell.After`, none with Reduce Motion) then reports. Panels shorter than their items scroll with arrow strips (ticker at `menuArrowSpeed`) and the wheel.
* **PopUpButton** (`popupbutton.go`): `popUpView`, `FocusControl`; push-button bezel, title, accent indicator with two chevrons. Press, Space or Up/Down opens a reused `menu` in a grab popup positioned so the selected item's title sits over the button's. A pick reports `SelectionChanged` through the stub; state changes close the menu without report.
* **Menu strip** (`menustrip.go`): `Window.setMenuBar(bar)` (titled windows only, D11) puts a `menuStrip` into the title bar after the lights; titles laid out by `menubar.Layout`, hidden ones dropped from the right. Press on a title opens its menu in one grab popup (with `Track` = strip), moving over another title `Reposition`s it with the other items. Items are validated at every open (`Window.menuEnabled`); a pick calls `Window.menuPicked`. A11y `RoleMenuBar`/`RoleMenuBarItem`/`RoleMenu`/`RoleMenuItem`.
* **Menu roles** (`menuaction.go`): role items without an ID run `Driver.runRole`: App (Quit always; Hide, Show All; About → app-modal alert), Window (Minimize/Zoom/Close by style), Edit on `Window.editTarget()` (focused `editActions`: TextField, SecureField, TextView; enabled per selection, editability, secure). Enabled state is computed at open and again before running.
* **Accelerators:** every key window (also untitled) gives key-downs to `Driver.accelerator` before the router (not during a grab popup): `menubar.Bar.Lookup`, validate, flash the title (`menuFlash`), run as a pick, consume. Disabled or no match passes on.
* **Menu keyboard access** (`menukeys.go`): a lone Alt or F10 toggles strip keyboard focus (window router inactive, content focus kept); Left/Right/Down/Return/Space/Escape/type-select drive it; chords with Ctrl/Alt/Win leave it. In a strip with a menu open, Left/Right switch menus.
* **Type-select** (`typeselect.go`, review R5): one rule for the menus and the strip's titles: a prefix grows within `menuTypeSelectReset` from the highlighted item on (wrapping), else the character alone picks the next item after the highlighted one, so the same letter again steps through the items with it; a space only grows a prefix.

**Driver** (`driver.go`, `New(sh)`, `Name` "aqua")

* `Run` binds the goroutine to a `mainthread.Queue`, runs the shell; on `Ready` keeps the shell's `Services` (text system and clock), reads the appearance, installs the queue wake (`Shell.Post`), creates the one `anim.Timeline` on the clock and calls `AppHandler.Ready`. Second `Run` → `ErrRunTwice`. `Quit` closes the queue (later `Do` calls dropped) and the shell. `ObserveFrames` sets the frame observer of that clock and returns its `Now`. `NewWithServices(sh, sv)` is a driver on a shell that is running already, for tests that build windows and controls without `Run`.
* **Appearance:** forced `Mode` overrides only `Dark`; an app accent (`SetAccentColor`) replaces `Accent` (semantic colors resolved against `driver.DefaultAccent`, result opaque) before anything sees it. Changes update the timeline's Reduce Motion first, then repaint every window and call `AppearanceChanged`.
* `SetMenuBar` stores a copy (nil = `driver.DefaultMenuBar`), builds `menubar.Build(spec, AppConfig.Name)` and gives it to every window. `RunAlert` queues a sheet on an open parent window, else an app-modal alert; `Alert.Dismiss(i)` answers through the element.
* Factories return real peers for every control (`peer.go`): each embeds its `driver/stub` peer (state, `User…` simulations and, where kept, the stub's `FittingSize`, so example layouts match headless) and owns an element (`ElementOf(w)`). `Container` is an element that paints nothing.
* **Colors** (`colors.go`): `tokens = &theme.Sequoia`; everything goes through `Theme.Resolve`. No color value or chrome metric is hard-coded in the package.

**Controls** (each pins its looks in `testdata/<control>.states`, light/dark × key/inactive)

* **Label:** `TextSystem` layout (wrapped at `max.W` when `Wraps`), cached per text/font/width in two entries (the measure's and the paint's width, review P13); leading draws once, centre/trailing per visual line (the line layouts cached with the entry). `ColorLabel` default, disabled color when disabled. A11y `text`.
* **Separator / Box** (`group.go`): pixel-aligned hairline along the longer side; Box = NSBox with optional title band, rounded `BoxFill`/`BoxBorder`, content host at `ContentInsets`. A11y `separator`/`group`.
* **Button:** `ControlHeight` × (title + 2·`PaddingX`, ≥ `ButtonMinWidth`); bezel shadow, `Theme.ButtonFill` (accent only for an enabled default in a key window), `BezelBorder`, truncated title, focus ring. Release over it → `UserClick`. A11y `button`.
* **Checkbox / Radio** (`checkbox.go`, `radio.go`, shared `toggle.go`): mark at the leading edge with `Theme.ToggleFill`; check/dash/dot in `ToggleMark`; disabled in a `PushOpacity(ToggleDisabledAlpha)` layer; whole frame hit. Radio `UserClick` always reports `StateOn`; exclusivity is the public `RadioGroup`'s. A radio group is one Tab stop (selected or first usable radio); arrows move and click within it without wrap.
* **Slider:** track, `ValueFill` up to the knob, tick marks, knob; press on the knob grabs, on the track jumps; drag follows (snapping with `SnapToTick`); keys by `Step` (1/20 range or one mark), `PageStep`, Home/End; `constrain` snaps near-ends exactly, NaN → min. `ValueChanged` only on change. A11y increment/decrement.
* **Progress / Spinner:** determinate fill or spokes; indeterminate phase is an `anim.Value.Loop` that runs only while live (indeterminate, visible, in a shown window with a timeline), restarted from `Paint`; static looks with Reduce Motion. No live indicator → no frames. The indeterminate spinner repaints only when its lead spoke changes (12× per period, not every frame) and caches its spoke paths per geometry (review P10).
* **ScrollView** (`scrollview.go`): `scrollArea` → clipping viewport → document host at minus the shown offset. Two `scroll.Axis` on the timeline. Scroll events: wheel/phase-less taken only if an axis can move, else bubble; `PhaseBegan` declined only if this view can't move and an enclosing one can. Programmatic changes (`SetOffset`, layout) never fire `Scrolled` nor show scrollers. **Legacy scrollers** (`Appearance.LegacyScrollers`) inset the viewport by `LegacyScrollerThickness` from the appearance and flags only (no oscillation); a change fires `ViewportResized`. **Keys** bubbling to the area: Page Up/Down, Home/End, Space (only when the focus is neither control nor text), arrows (only when the view itself is focused); steps accumulate from a running step's target. **Scroll-to-focused** (focus hook): least scroll showing the frame plus the full focus ring, immediate.
* **Scrollers** (`scroller.go`): overlay (inside the viewport, widening on hover, fading via `AnimateAfter(ScrollerFadeDelay)`, hidden at 0 opacity so content gets the pointer) or legacy (outside, opaque, always shown). Knob length/position from viewport/document, shortened by overscroll. Knob drag 1:1; track click pages.
* **TextField** (`textfield.go`, one peer for TextField and SecureField): `FocusText`, owns a single-line `text.Editor` mirrored to the stub's `Value`. Text scrolled to keep the head visible; secure shows one `•` per cluster; the text and its cluster starts are memoised per buffer revision (`textMemo`, review P18), so a paint, a key and the offset mappings neither copy the text again nor recount the clusters. Caret only focused, in a key window, editable, empty selection. **Keys:** `text.Lookup` single-line → `Editor.Do`; edits taken but refused when read-only; Return fires `Submitted` and is **not** taken (the default button still gets it). Tab focus selects all. **Mouse:** click/Shift/double (word)/triple (all), drag by unit. **Clipboard:** paste is async and dropped if the focus count moved, the field became disabled/read-only or the window closed; secure fields don't copy/cut. **Caret blink:** a `Shell.After` timer only while the caret shows, stopping after `Appearance.CaretBlink.Timeout`, so idle stays idle. **IME:** enabled, editable, non-secure fields take preedits, spliced into the layout at the caret with clause underlines; a `TextEvent` while composing commits (own undo group); blur, press, key loss, key, paste, `SetEditable(false)` commit via `CancelComposition`; `SetText`/`UserType` drop it. `TextChanged` after every user edit, never for `SetText`.
* **Text element base** (`textedit.go`, review R5): `textEdit` holds the editor, the focus ring, blink, focus count, clipboard with late-paste guard, composition and drag state, and the shared behaviour: `handleEvent` (focus, window state, keys after committing the composition, typed text, preedits, mouse; a Tab focus of a single-line editor selects all), `typeText`, `pointer`/`press`/`dragTo`/`endDrag` (click, Shift, double: word or all when secure, triple: all in a single-line editor, the paragraph in a multi-line one), `caretShown`, `textCursor`. Elements plug in through `editHost`: `base`, `takesPreedit`, `secureText`, `readOnlyText`, `key` (the key map), `edited`, `command`, `preeditChanged`, and three mouse hooks `hitAt`, `selected`, `autoScroll`.
* **TextView** (`textview.go`; layout in `textview_layout.go`, painting in `textview_paint.go`, keys and mouse in `textview_input.go`): multi-line `text.Editor` (text and selection live only in the editor) inside an internal `ScrollView` (vertical scroller, hidden from a11y) over one `textDoc`. Geometry from `Theme.TextView` (`ContainerInsetY`, `LineFragmentPadding`). **Settling:** every frame/viewport/offset/text/font change lays out only the paragraphs overlapping the viewport and sizes the document from the height index, repeated until stable. **Scroll anchoring:** width/font changes keep the top visual line. Keys via `text.Lookup` multi-line with `paraLayouts` as `text.Lines`; Return/Tab insert, Ctrl-Tab moves focus; the caret scrolls into view once per key or edit, except for Select All (review B5/P5). Clipboard, blink, IME as the TextField. **Idle layout pass:** while heights are estimated, a `Shell.After` timer measures ≤ ~16 KiB / 4 ms per tick without moving visible lines; none once all are exact. **Mouse:** click/word/paragraph selection, drag by unit, auto-scroll ticker past the edges.
* **Paragraph height index** (`paraindex.go`): per-paragraph heights (exact or estimated) in fixed point (2^16/pt) so prefix sums are exact; chunks of 32–128 with three Fenwick trees; `Y(i)`, `At(y)`, `Set`, `Splice`, `NextInexact`.
* **Paragraph layout cache** (`paralayout.go`): one `render.TextLayout` per paragraph in an LRU of ~three pages (≥ 64); heights estimated from byte length until laid out. Owns the buffer observer: each change becomes a paragraph splice dropping only touched layouts. Implements `text.Lines` across paragraphs. `Measure(i)` makes a height exact without caching. `SetPreedit` keeps a composed layout for one paragraph outside the LRU.
* **Long paragraphs in chunks** (`parachunk.go`, review P1): a wrapped paragraph over `longParaMin` (4 KiB) is a `chunkedLayout`, a `render.TextLayout` over layouts of about `longParaChunk` (1 KiB). Each chunk shows all its lines but the last and the next one starts where that hidden line starts, so with greedy wrapping the lines equal one layout of the paragraph (pinned against it by `TestParaChunksSameAsWhole`; the base direction is per chunk). An edit inside the paragraph keeps the cached layout: `relayoutPara` lays out from the line before the edited word and takes over the old chunks once a new one starts where a moved old one does; a composition reuses the chunks the same way. `drawParagraph` draws only the chunks in view, each clipped to its shown lines. A keystroke in a 1 MB line with its frame costs about 2.3 ms on DirectWrite and Direct2D (463 ms before), most of it the frame's `Present`; the chunk layouts take about 0.35 ms (`BenchmarkTextViewKeystrokeLongParagraph` in aqua and win32, `BenchmarkParaLayoutsKeystrokeLongParagraph`).

### 9.1 `internal/aqua/theme`: design tokens

The tokens of the drawn controls (PLAN.md §5.2). Imports only `driver` (+ `math`, `time`); no build tags. Used by `aqua` and `shell/win32` (clear color).

* **`Theme`** (`theme.go`): `Name`, `Colors` (a `ColorToken` per `driver.SemanticColor`) and groups `Chrome`, `Metrics`, `Typography`, `Accent`, `Animation`, `Controls`, `Scroll`, `TextField`, `TextView`, `Menu`, `PopUp`, `MenuBar`, `Tooltip`, `Alert`. `ColorToken{Light, Dark, ContrastLight, ContrastDark, HasContrast}`; `Value(a)` picks dark/light and the contrast variant with `IncreaseContrast`.
* **`Theme.Resolve(c, a)`** (what the driver uses): concrete colors unchanged; `ColorControlAccent`, `ColorSelectedContentBackground`, `ColorSystemBlue` → `a.Accent`; focus indicator → `FocusRing(a)`; selected text/control → `Selection(a)`; other semantics → their token; unknown → opaque magenta.
* **Accent rules** (`accent.go`, D10: accent follows the OS): accent-derived colors are computed, never stored: `FocusRing` (accent × 0.5 alpha; opaque with contrast), `Selection` (accent over `Base` at 0.3; 0.5 with contrast), `AccentPressed` (× 0.85), `AccentInactive` (luma grey of `driver.DefaultAccent`, ignores `a.Accent`), `AccentText` (white, or dark when the accent's WCAG contrast with white is below `AccentTextMinContrast`), `ToggleMark`, `MenuHighlightedText`.
* **Computed control colors** (`controls.go`, `textfield.go`, `popup.go`, `menu.go`): `ButtonFill`, `ButtonText`, `ToggleFill`, `ValueFill`, `Caret` (accent), `TextSelection(a, focused, key)`, `PopUpIndicatorFill`, `PopUpChevron`, `PopUpText`, `MenuHighlight`.
* **Increase Contrast rule** (as macOS): borders and separators darken; fills and window backgrounds keep theirs (the opaque panel backgrounds of menus, tooltips and alerts excepted). `BezelOutline` and `SliderTrackOutline` are transparent except in contrast mode.
* **Number tokens:** `SizeTable` rows indexed by `driver.ControlSize` (`At` maps unknown to regular); `Metrics.Control(cs)`; `Typography` (font sizes 13/11/9/13, `LineHeight` factor, weights, SF-like `Tracking` table with `TrackingAt`); `Animation` (durations, cubic-bezier curves, springs; Reduce Motion is the `anim.Timeline`'s rule alone). Every alpha and shade token (`…Alpha`, `…Shade`) is a `float32`, the type of the color channel it scales (`TestAlphaTokensFloat32`).
* **`tokens.go`** (generated, committed): `var Sequoia = Theme{…}`, one line per token, `// provisional` on values from memory (all of them until measured against macOS).
* **Spec** (`spec/sequoia.json`): `colors` (`light`/`dark` as `[r,g,b(,a)]`, optional `contrastLight`+`contrastDark`, required `source` `"memory"`|`"measured"`, optional `note`) and one group per `Theme` field with typed sections `colors`, `numbers`, `sizes` (regular/small/mini/large), `weights`, `durations` (ms), `curves`, `springs`, `tracking`. Unknown or missing fields/groups/tokens are errors.
* **Generator** (`gen`, stdlib only; `go generate` runs its command `aqua/theme/gen/gentokens`): `Parse` validates; `Generate` returns the gofmt'd Go file and `docs/tokens.md` (tables per group, hex, contrast, source, provisional count). `Groups` is the schema. Run with `go generate ./internal/aqua/theme`; `TestGeneratedUpToDate` guards it.

### 9.2 `internal/aqua/anim`: animation math and timeline

Animations of the drawn driver (PLAN.md §5.5). Imports `aqua/theme`, `driver` and the standard library; no build tags. The aqua `Driver` owns the one `Timeline`.

* **`Curve`** (= `theme.Curve`): cubic bezier (0,0)→(1,1). `At(t)` clamps t (NaN → 0), exact at 0 and 1, Newton with bisection fallback. `Slope(t)` = dy/dx (`AtSlope(t)` returns both from one solve, as a tween's frame needs); at a flat end the l'Hôpital limit (EaseOut starts at Y2/X2), 0 only where it is unbounded. X1/X2 in 0..1; Y may overshoot.
* **`Spring`** (= `theme.Spring`): `Motion(from, to, v0)` (panics unless mass, stiffness, damping positive and finite); `Motion.At(t) (x, v)` in closed form for every damping regime, stable for all t. `Settled(t, eps)`: the displacement the motion can still reach (energy bound) is at most eps, so once true it stays true and a jump to `To` does not show; `Peak()` is the largest |x − To| over t ≥ 0 (start or first turn, closed form per regime).
* **`Timeline`** (`NewTimeline(clock)`; clock = `Now` + `RequestFrame`): `Tick(frameTime)` samples every running value at the frame time (closed form: late frames land exactly), finishes values at their exact target, runs change funcs in start order (they may start/stop values), then tickers. Requests the next frame only while something runs (idle app requests none). Re-entrant `Tick` panics; a panicking callback leaves it usable. `SetReduceMotion(on)`: tweens/springs jump, `Decay` jumps to rest, `Loop` stays 0; turning on finishes everything running.
* **`Value`** (`NewValue(x, onChange)`): `Value`, `Velocity`, `Target`, `Running`, `Set` (jump), `Stop`. `SetTolerance(tol)` (`DefaultTolerance` 1e-3, `AlphaTolerance` 1/510, `PointTolerance` 1/32 pt): a spring settles once `Settled(t, tol)`, and a `Tick` that moves a running value by less than tol from its last shown value leaves `Value` there and skips the change func (the velocity still follows); the end is always shown exactly. The scroll axis uses `PointTolerance`, the focus ring and the overlay scrollers `AlphaTolerance` (review P11: the rubber band from 50 pt stops after 43 frames instead of 70). `AnimateTo(x, d, curve)` and `SpringTo(x, spring)` start at the clock's `Now` (frame time inside `Tick`) and retarget from the current sampled position and velocity; animating to the current target does nothing. `SpringFrom(x, v0, s)` (explicit velocity, always restarts). `Decay(v0, rate, vs)` exponential momentum ending exactly at its rest. `AnimateAfter(x, delay, d, c)`: hold, then tween (the delay is not motion: it still runs under Reduce Motion). `Loop(period)`: drift-free phase in [0,1) until stopped; same period is a no-op, another retimes without a jump. Non-finite inputs panic.
* **`Ticker`** (`ticker.go`, `NewTicker(fn(dt))`): per-frame non-motion work; `Start`/`Stop` idempotent; not stopped by Reduce Motion.
* **Interpolation:** `Lerp` (exact ends).

### 9.3 `internal/aqua/scroll`: scroll physics

Pure one-dimensional scroll physics plus the per-axis offset model (PLAN.md §5.6). Imports `aqua/anim`, `aqua/theme`, `shell`; no build tags. Points, points/second, `time.Duration` on the frame clock; token values passed in by the caller.

* **`RubberBand(d, c)`** = d·c/(d+c) (slope 1 at the edge, < c, odd); `RubberBandInverse` maps back so a drag caught mid-spring continues 1:1. c ≤ 0 or NaN: no overscroll.
* **`VelocityTracker`**: `Add(t, d)` for moving events; `Velocity()` over the samples within `Window` of the latest (steady stream d per Δt gives exactly d/Δt). `End(t)` returns the velocity if the end event comes within `EndGap` (`DefaultEndGap` 150 ms, covers Win32's late zero-delta `PhaseEnded`), else 0.
* **`Axis`** (`NewAxis(tl, Params, onChange)`; `ThemeParams(th)`): one `anim.Value` (tolerance `anim.PointTolerance`) holds the shown offset (overscroll included); momentum is `anim.Value.Decay` (the package has no decay of its own); `Clamped`, `Overscroll`, `Target`, `Moving`, `Dragging`, `Stepping`. `Event(ev, d)` (delta in `shell.ScrollEvent` direction) reports whether it was taken:
  * precise gestures always taken; fingers move 1:1, shown through `RubberBand` past edges (c = `RubberBandFraction` × viewport);
  * release: tracker velocity × rubber-band slope → `SpringFrom` the edge if past it outward/slow, else `Decay`; a decay hitting an edge springs back with its speed (capped from `anim.Motion.Peak` in the spring's actual regime so the overshoot stays within 0.9·c; content shown ≥ c past an edge is caught 0.9·c past it, not at the edge);
  * OS momentum (GTK) replaces the own decay and springs back at an edge;
  * wheel: `Delta × LineHeight` animated over `WheelStep`, notches extend the target, clamped, not taken when it cannot move; phase-less precise events scroll 1:1, clamped.
  * `ScrollTo(x, animated)`, `Stop`, `CanScroll(d)`, `SetViewport`, `SetMax` (re-clamps or springs back against the new edge).
  * Reduce Motion: no rubber band, no momentum (OS momentum taken but moves nothing), steps jump; `FollowReduceMotion` clamps a held drag.

### 9.3a `internal/aqua/menubar`: the title-bar menu model

The pure model of the title-bar menu strip (PLAN.md §5.4.1); the elements (`menustrip.go`, `menu.go`, `popuproot.go`) stay in `aqua`. Imports only `driver`; no build tags.

* **`Build(spec, appName) *Bar`**: nil spec → nil bar (no strip). Each `Menu` has its effective title (app name for `MenuApp`), role and items (`ID`, effective `Title`, `Role`, `Shortcut` resolved with Primary = Ctrl, `ShortcutText` like "Ctrl+Shift+D", `Separator`, `Disabled`, `Checked`, `Submenu` — always disabled, no cascading menus in v0.1). The bar is a copy. `Bar.Item(Ref)`, nil-safe.
* **`Bar.Lookup(key, shifted, mods) (Ref, bool)`**: exact match (key with exactly mods) or shifted match (shortcut without Shift, mods = its mods + Shift, and `shifted` — the layout's shifted character — equals its key), so Primary+'+' works on US and German layouts and Ctrl+Shift+A never matches Ctrl+A. Exact beats shifted, then menu order. Separators, submenu titles and shortcut-less items never match; disabled items do (the caller validates).
* **`Layout(widths, Metrics) Strip`**: slots of width + 2·`Pad` from `Start`; a title shows only while slot end + `MinDrag` ≤ `Width` (drops from the right, keeps a drag area). `Strip.End`, `Strip.TitleRoom` (room for the centred window title with symmetric margins max(`TitleMargin`, `End` + `TitleGap`)).
* **Which windows** (D11): every titled window shows the strip of the one bar; accelerators work in every key window; no `SetMenuBar` → no strip.

### 9.4 `internal/aqua/gallery`: the gallery scene and the image comparison

The scene of the golden-image tests. Imports only `driver` (not `gogui`, which would cycle through `shell/native` into the win32 tests; not `aqua`), so the appkit driver can build the same scene for macOS references. No build tags; UI thread only.

* `Build(d) (Widget, Size)`: fixed frames (no layout engine), 600 × 346 pt. `Open(d, h)` creates and shows a "Gallery" window and focuses the "Focused" checkbox; `Fill(d, w)` fills a caller's window without showing it (the Windows renders use hidden windows).
* Scene in three columns: labels, separators, push buttons (plain/default/cancel/disabled/sizes), checkboxes, a box of radios, sliders, progress bars, a spinner, and a scroll view over a 3 × 8 label document scrolled to (60, 40). Only states the driver API can set; hover/pressed are in the `.states` dumps. TextField, TextView and PopUpButton are not in the scene yet (their states are pinned by `.states`). Renders use Reduce Motion (static indeterminate looks) and legacy scrollers, so the scene is still once painted.
* **Windows render** (`internal/shell/win32/gallery_test.go`): the aqua driver on a test shell forced to WARP with a fixed injected appearance (light, default accent, no high contrast, animations off, legacy scrollers). Per variant it sets DPI (`WM_DPICHANGED`), high contrast (`WM_SETTINGCHANGE`), key/inactive (`WM_ACTIVATE`) and mode (`Driver.SetAppearance`), runs frames until the timeline is idle, paints, reads the back buffer through a staging texture and converts it with `goldentest.FromBGRA`. Output: the whole window including the title bar.
* **Variants** (12 goldens in `internal/aqua/gallery/testdata/golden/`): `gallery-{light,dark}-x{1,2}` (`goldentest.Variants`), plus `-inactive-` and `-contrast-` versions (`PolishVariants`, Windows only for now).
* **Linux** (`shell/gtk4`'s `TestGalleryGolden`, run by `native.sh` on both headless backends) compares the four `Variants` against the same PNGs, strictly, with text boxes masked (Pango/GSK text differs from DirectWrite). It has no `-update`; a real difference is fixed in the gtk4 canvas, never in the PNGs.

**The golden flow.** Five kinds of pinned output follow the theme tokens; `go test ./...` fails until every one that moved is rewritten:

| What | Where | Rewrite with |
|---|---|---|
| Generated tokens and their doc (`TestGeneratedUpToDate`) | `internal/aqua/theme/tokens.go`, `docs/tokens.md` | `go generate ./internal/aqua/theme` |
| Per-control state dumps (hover and pressed included) | `internal/aqua/testdata/*.states` | `go test ./internal/aqua -args -update` |
| Aqua frames of the examples, their title bars and sheets (`TestExamplesOnAqua`; metrics only) | `testdata/aqua/*.frames` | `go test -run TestExamplesOnAqua . -args -update` |
| Gallery recorder dump (`TestGalleryDump`) | `internal/aqua/gallery/testdata/gallery.dump` | `go test -run TestGalleryDump ./internal/aqua/gallery -args -update` |
| Gallery goldens (`TestGalleryGolden`; pixels; written on Windows only, compared on Windows and Linux) | `internal/aqua/gallery/testdata/golden/gallery-*.png` | `go test -run TestGalleryGolden ./internal/shell/win32 -args -update` (never on Linux) |

After a token or theme change: regenerate; run `go test ./...` to see what moved; review the dump text diffs (a token change should show only changed values) and rewrite them; then rewrite the goldens on the Windows host (WARP, windows/amd64) and inspect the PNGs. To see pixel changes compare with `git show HEAD:<path>`, or run without `-update` and open the `<name>.diff.png` written to `GOGUI_GOLDEN_OUT`. A refactor must pass without `-update`. Goldens are only written from a clean render, never edited by hand. The long-term target is the macOS reference images; the goldens pin the Windows result.

**`aqua/gallery/golden`** (pure Go, stdlib only):

* `ReadPNG`, `WritePNG` (straight alpha, 8-bit, RGB when fully opaque; deterministic bytes), `NRGBA(img)`.
* `Compare(want, got, Options) (Result, diffImage)`, after pixelmatch: squared YIQ distance vs `Threshold`²; translucent pixels blended over white and black (larger distance counts); differing anti-aliased pixels count `AAWeight`; pass when the weighted count ≤ `MaxDiff`. `Threshold` is clamped to [0, 1] (NaN → 0). `DefaultOptions` = `{Threshold 0.1, AAWeight 0.1, MaxDiff 0}`: on the host a fresh render matches exactly. `Mask` rectangles are excluded (counted as `Masked`).
* `Result`: sizes, `SizeMismatch` (fails, no diff image), `Diff`, `AA`, `Masked`, `Weighted`, `MaxDelta`, `Bounds`, `Pass`, `String`. Diff image: faded grey unchanged, red differing, yellow anti-aliased, blue masked.

**`aqua/gallery/goldentest`** (imported only by tests): the flow the win32 and gtk4 gallery tests share.

* `Variant{Dark, Scale, Inactive, Contrast}`, `Name()`, `Variants`, `PolishVariants`, `DirFromShell`.
* `FromBGRA(w, h, stride, bgra)`: premultiplied BGRA (D3D11 staging, `gdk_texture_download`) to rounded straight-alpha `*image.NRGBA`.
* `Check{Dir, Options, Update, Out, LogOnly}.Run(t, v, got)`: with `Update` rewrites the PNG (the caller owns the flag: only win32 registers `-update`); otherwise compares and on failure writes `<name>.actual.png` and `<name>.diff.png` to `Out` / `$GOGUI_GOLDEN_OUT` / `t.TempDir`. A missing golden always fails. `Check.Sweep` reports the comparison at several thresholds, with and without the mask, for calibration.

### 9.5 `internal/aqua/text`: the text editing core

The editing core shared by the aqua TextField and TextView (PLAN.md §5.5, §5.6): buffer, boundaries, `Editor`, undo, keymap. No widget, layout or clipboard code; imports only `rivo/uniseg` and `driver`; no build tags; single-threaded (the owner's UI thread).

**Contracts** (what a widget over the core relies on):

* **Byte offsets.** Every offset is a UTF-8 byte offset, as in `driver.TextView` and `render.TextLayout`, passed to layouts unchanged. Out-of-range offsets panic.
* **Grapheme boundaries.** Every offset the `Editor` holds or returns is a grapheme boundary of the whole text, also after edits that join clusters; `SetSelection` clamps to one. Boundary functions work on one paragraph and never read the whole buffer.
* **Newlines.** The text never holds `\r` (`\r\n` and `\r` → `\n` on input); in single-line mode newlines become spaces.
* **`Lines`.** The only geometry the core sees, implemented by the widget over its layouts (visual lines, upstream affinity at soft breaks); nil means one line per paragraph. The core never lays out, measures or draws.
* **Undo groups.** Every text change goes through one recorded `replace`; typing and same-direction deletes at the caret coalesce, anything else seals the group. `Undo` restores text and selection before the group, `Redo` text and caret after. `SetText` clears history. `Revision()` advances on every `SetText` and every changing edit (layout cache key).
* **Keymap.** `Lookup` maps a key and its *physical* modifiers to a `Command` with macOS semantics (Primary = Ctrl); `Editor.Do` returns false for what the widget must handle or pass on (clipboard commands, unbound keys, newline in single-line mode).
* **Cost budget.** On 1 MB of text (80-column lines or one paragraph) each keystroke's move, edit or undo stays in the low microseconds; only whole-megabyte edits (select-all-delete and its undo) reach about 1 ms. Benchmarks in the package track it.

**Buffer** (`NewBuffer(s)`; zero value empty)

* Piece table over the immutable original and an append-only add buffer. `Insert` splits at most one piece (typing at one spot extends the last piece); `Delete` trims end pieces, and a delete that ends the add buffer gives that tail back, so typing and backspace cycles neither add pieces nor grow it (review B13); `Replace(start, end, text)` is a delete and an insert as one change. `Len`, `Slice`, `String`, `SetText` (drops the add buffer).
* Paragraphs are runs between `\n`s (n newlines → n+1 paragraphs; a range excludes its `\n`): `ParagraphCount`, `ParagraphIndex`, `ParagraphStart`, `ParagraphEnd`, `ParagraphAt`. Lookups are binary searches over per-piece newline indexes with lazily recomputed prefix sums.
* **Change observer:** `SetObserver(func(start, oldEnd, newEnd))` is called exactly once per `Revision` step after the buffer is consistent (may read, must not change it). `SetText` reports `(0, oldLen, newLen)`; `Insert`/`Delete`/`Replace` once each and only when they change something. Every Editor edit, undo and redo step is one `Replace`, so one report (review R5). The TextView turns reports into paragraph splices.

**Boundaries** (`boundary.go`; pure functions over one paragraph `s`)

* UAX #29 extended grapheme clusters and word segments from `rivo/uniseg`; an offset inside a cluster counts as that cluster's. A segment is a word if it holds a letter or digit ("don't", "3.14", "foo_bar" are one word; each CJK ideograph/hiragana its own word).
* `NextGrapheme`, `PrevGrapheme`, `ClusterStart`; `NextWordEnd`/`PrevWordStart` (Option-→/←; return false at the paragraph end for the caller to cross); `WordAt` (double-click: word, else whitespace run, else one cluster).
* **Local scans:** each function starts segmentation at the nearest provable boundary before the offset (between ASCII, CJK ideograph or hiragana runes), so results equal a full scan (pinned by tests). Worst case: one long run with no such point (a huge word, Thai, katakana) scans the run.

**Editor** (`editor.go`, `NewEditor(text, singleLine)`)

* **Selection:** anchor and head (always grapheme boundaries), `Selection()`, `Upstream()` (head at the end of a soft-wrapped line). `SelectAll`, `SelectWord`, `SelectParagraph` (with its `\n`), `SetSelection`, `SetSelectionUpstream`.
* **`Move(unit, dir, extend, lines)`:** `Grapheme`, `Word` (across paragraphs), `Line` (visual line ends), `Paragraph`, `Document`, `Vertical`, `Page` (`Lines.PageLines`). Without extend a selection collapses first (a grapheme move stops there, the macOS ←/→ rule). Vertical moves keep a goal x; on the first/last line they go to the document start/end.
* **`Lines`:** `LineRange(off, upstream)`, `CaretX`, `OffsetAt(off, upstream, n, x)`, `PageLines()`.
* **Edits:** `Insert` replaces the selection; `DeleteBackward`/`DeleteForward(unit, lines)` delete the selection or to a move target; `Copy`, `Cut`, `Paste(s)` work on strings (the widget owns the clipboard); `SetText` (caret at 0). When an edit joins clusters the caret moves to the joined cluster's end (insert) or start (delete).
* **Single-line mode:** newlines → spaces; `Vertical`/`Page` go to the text's start/end.
* **Long paragraphs** (`around`): boundary questions run on a ~2 KB stretch either side of the offset, trimmed to provably safe boundaries, widened ×4 when the answer reaches a trimmed end (whole paragraph at worst); tests run with tiny stretches against the default.

**Undo** (`undo.go`)

* `Undo`, `Redo`, `CanUndo`, `CanRedo`, `SetUndoLimit(n)` (`DefaultUndoLimit` 1000 groups, 0 = unlimited), `BreakUndoGroup()` (seals the open group; used around IME commits).
* Records `{start, old, new}` per edit into the top group with the selection before and caret after; no-ops record nothing.
* **Coalescing:** kinds typing / backward delete / forward delete / other (paste, cut, newline: always own group). An edit joins the open group if same kind and the caret is where the group left it. Moves, selection changes, undo and redo seal the group. Adjacent edits in a group grow in place up to 256 bytes.
* Any new edit clears redo. The append-only buffer re-appends undone/redone text (bounded by user actions, cleared by `SetText`).

**Keymap** (`keymap.go`)

* `Command`: move commands with `Select…` variants per unit, deletes (grapheme/word/to line start/end), `CmdInsertNewline`, `CmdInsertTab`, `CmdSelectAll`, `CmdCopy/Cut/Paste`, `CmdUndo/Redo`; `IsClipboard`, `Edits` (what a read-only field refuses).
* **`Lookup(key, mods, multiline)`**: macOS bindings on every OS with Primary = Ctrl, Option = Alt: ←/→ cluster, Option word, Primary line ends; ↑/↓ visual line, Option paragraph, Primary document ends; Home/End document ends; Page Up/Down; Shift extends; ⌫/⌦ cluster/word/to line end; Primary+A/C/X/V/Z, Shift+Primary+Z. Return/Shift-Return and Tab only when `multiline`. Unbound, so they pass on: Shift-Tab, Ctrl-Tab, Escape, Ctrl-Return, Emacs chords, anything with the Windows/Super key or Ctrl+Alt (AltGr), Ctrl+Y.
* **Deviation from macOS:** plain Home/End and Page Up/Down move the caret (macOS only scrolls), so a field without a scroll view reacts to them.
* **`Editor.Do(cmd, lines)`** runs a command; returns false for `CmdNone`, clipboard commands and newline in single-line mode. `Undo`/`Redo` with nothing to do are still handled.

## 10. Tests

All tests run without a display, with one exception: the Windows packages create real windows (mostly hidden) and GPU devices on the host. `bash scripts/check.sh` runs every check (gofmt, vet, build for six targets, staticcheck, `go test -count=1 ./...`, the guards, the stress runs).

### 10.1 Cross-cutting mechanisms

- **Stub and headless drivers.** The `gogui` widget tests run against `driver/stub`, which records every setter and simulates user input through its `User…` methods. `driver/headless` runs the examples and the testable `Example…` functions. In the test binary, `example_driver_test.go` makes the default driver a fresh, silent headless driver, so each Example may call `Run`.
- **Fake shell and recording canvas.** `shell/fake` is a deterministic in-memory shell. It provides surfaces, a manual frame clock and timers, input helpers (clicks, drags, keys, IME preedits, wheel and swipe gestures), popups, modal surfaces, a clipboard delivered on `Step`, and fake text metrics that match the stub's. The aqua driver runs on top of it. Paints go into `render/record`, which normalizes the ops and dumps them as stable text. Every test window also checks for recorder errors, such as unbalanced state or rejected calls.
- **Pinned dumps (`-update`).** Expected output lives in committed files. A failure prints a line diff, and comparisons tolerate CRLF line endings.
  - `testdata/aqua/<example>.frames` (root package, `TestExamplesOnAqua`): each example window's frame tree at its narrowest width. It also holds the title bar (the accessibility subtree of the menu strip, the drawn title, the chrome regions) and, for `form` and `controls`, the slid-in alert sheet. `notes_legacy.frames` is the same dump with legacy scrollers. Rewrite with `go test -run TestExamplesOnAqua -update .`.
  - `internal/aqua/testdata/*.states`: one paint dump per control state, covering every combination of light/dark, key/inactive and normal/Increase Contrast. Files exist for button, checkbox, radio, slider, progress, textfield, textview, scroll, scroll_legacy, menu, menustrip, popup, alert and sheet. Rewrite with `go test ./internal/aqua -run Test<Name>States -args -update`.
  - `internal/aqua/gallery/testdata/gallery.dump`: the recorder dump of the whole gallery window.
- **Golden images.** `aqua/gallery/golden` compares images with a threshold, extra weight for anti-aliased pixels and an optional mask. `aqua/gallery/goldentest` is the shared harness: it writes `<name>.actual.png` and `<name>.diff.png` to `GOGUI_GOLDEN_OUT` (falling back to `t.TempDir`), and logs a threshold sweep for each comparison. The goldens are in `internal/aqua/gallery/testdata/golden/gallery-{light,dark}[-contrast|-inactive]-x{1,2}.png`.
  - Win32 `TestGalleryGolden` renders the gallery on WARP and compares the result. Rewrite the goldens with `-args -update`.
  - The GTK `TestGalleryGolden` runs the same comparison on Linux through `native.sh`, with a text mask.
  - `GOGUI_SCREENSHOTS=dir` writes every frame of a Windows test as a PNG.
- **Native-only tests.** Linux GTK tests (they need GTK, Pango, fontconfig, D-Bus or xvfb), Windows arm64 layout and float-path tests, the native frame benchmarks and the pacing tests all run through `bash scripts/native.sh`, which writes `native-logs/<goos>-<goarch>.log`. They log machine-readable `XXX-RESULT:` lines for open platform questions. Without the library or display they need, they skip. Most GTK native tests have **not been run yet**, because there is no Linux machine. In a normal run on this host, each GTK test runs against fakes that drive the real trampolines.
- **Struct layout tests.** Every mirrored C, ObjC or Win32 struct has a test of its size and offsets on amd64 and arm64. The test also checks that its table covers every struct, using `ffi` layout helpers that report every difference. Packages: `w32`, `com`, `dxgi`, `d3d11`, `dcomp`, `d2d`, `dwrite`, `gtk`. Vtable slot constants are checked against the full vtables.
- **Mutation checks.** The pixel and node tests of the Win32 canvas, text, `d2d` and `dwrite` packages, and the GTK surface, clock and canvas tests, were checked by mutating the code under test. Coverage is near 100 % for `render/record`, `aqua/a11y`, `aqua/anim`, `shell/fake` and `aqua/text`.
- **Stress runs.** `CGO_ENABLED=0 go test -count=20 -cpu=1,2,4,8` runs over `.`, `internal/mainthread`, `internal/ffi`, `internal/driver/headless` and `internal/shell/fake`, and over the cross-thread tests only of `internal/aqua` (`TestDriver*`) and `internal/shell/win32`. Tests that show windows or wait on real frames are skipped. The skip and `-run` patterns are in `check.sh`. `go test -race` (cgo) is allowed as an extra run on the user's machines, but no script runs it yet.
- **Benchmarks and allocation guards.** Benchmarks run only with `-bench`, and `native.sh` runs them.
  - `BenchmarkFrame` (root package) runs each scene (the six examples and the fixed scene of `internal/aqua/gallery`, named `golden`) on the fake shell at 1×/2× and light/dark, as a steady frame and as a layout frame.
  - **`TestFrameAllocs`** fails when a steady frame's allocations exceed `frameAllocBudget` for its scene.
  - `BenchmarkFrameNative` measures presented frames on the real shell and splits each into work, paint, submit and present times.
  - Package benchmarks cover the layout (`BenchmarkLayout1000`), the text buffer, boundaries, editor and undo (1 MB texts), the paragraph index and layout cache, and the TextView (`BenchmarkTextView*` on a 1 MB text; the budget is one frame per operation and under 2 ms per keystroke). Win32 has `BenchmarkGalleryFrame` and `BenchmarkCallbackCall`.
- **Frame pacing.** These tests run only with `GOGUI_PACING=1`, which `native.sh` sets, because they need a quiet host. `TestFramePacingNative` runs eight scripted scenarios (scrolling, a momentum fling, a rubber band in `notes`, the indeterminate progress bar and an alert sheet in `controls`) and feeds `shell.Pacing`. It logs `PACING-RESULT`/`PACING-TOTAL`. On Windows it fails beyond `shell.PacingTolerance` per scenario and beyond `PacingRunTolerance` per run; on Linux it only logs. Win32 `TestClockPacingWork` logs how much frame lead remains under extra busy work per frame and never fails.
- **Repository and documentation checks** (`internal/repocheck`, test-only, outside the stress run; its `TestMain` runs from the repository root, review R14):
  - `TestThirdPartyNotices` (`license_test.go`): every `go.mod` require has a row with its version in `THIRD_PARTY.md`, and every row has a require. `LICENSE` is MIT, and the Inter license files exist.
  - `TestExamplesListed` (`examples_test.go`): every `examples/` folder must appear in the example list of the root package's `aqua_test.go` (read from its source), under a matching `AppConfig.Name`, in both lists of `examples/README.md`. (`scripts/native.sh` derives its list from the folders.) Nothing may be listed that has no folder.
  - `TestExampleTreesMatch` (`trees_test.go`): the trees the root package's `examples_test.go` builds are the trees of `examples/*/main.go`. Both sources are parsed (`go/ast`) into the multiset of their public-API construction calls (each exported `gogui` call with its chained methods, every `WindowConfig`), literal arguments kept and anything else read as `_`; a `_` in `main.go` (a value computed at run time, such as notes' title) matches any value in the test. `TestTreeSignatures` checks the reader.
  - `TestParityChecklist` (`parity_test.go`): every check named in `docs/parity.md` is defined in its §4, defined only once, and used. Every matrix cell has a state mark. `README.md`, `examples/README.md` and `native.sh` must still link to the checklist.
  - `TestOverviewMatchesPlan` (`overview_test.go`): the diagram and the layer table of §0 are those of PLAN.md §2 and §2.1 (up to the `PLAN.md ` before a section reference).
  - `TestDocsNameExistingFiles`, `TestPackagesNamed`, `TestBuildTagsWhereDocumented` (`files_test.go`): every Go file that ARCHITECTURE.md, PLAN.md, CLAUDE.md, the READMEs, `THIRD_PARTY.md` and `docs/parity.md` name exists; every package outside `examples/` and `spike/` is named here; OS constraints are only where §1 lists them.
- **Export docs** (root package): `TestEveryExportDocumented` (`doc_test.go`, over `go/doc` for all build tags) checks for a package doc and a doc comment on every exported identifier, including methods promoted from embedded generics. Struct fields need a comment or a mention in the type doc. Aliases of `driver` types need a doc that names every field and method of the target.
- **Testable examples** (root package; `example_test.go`, written as app code with checked output): `ExampleRun`, `ExampleVStack`, `ExampleDo`, `ExampleDoSync`, `ExampleApp_SetMenuBar`, `ExampleAlert_Show`.

### 10.2 Per package

- **`gogui`** (root):
  - **Public API.** Semantic colors, fonts, normalization and display of `Shortcut`, enum values pinned to AppKit. `Run` and `Do`/`DoSync` semantics, including the panics when called off the UI thread.
  - **Widgets and windows.** Widget setters reaching the peer, invalidation, and widget destruction. Stacks and forms laid out through `layout.Root`. Windows (deferred layout, minimum size, close veto, bottom-up teardown). Controls, hosts and app-level features (menu bar, alerts, appearance, quit veto, the accent override `TestAccentColor`), driven through the stub.
  - **Driver selection** (`driver_test.go`): headless on macOS, aqua on Windows, aqua over GTK on Linux (`TestPlatformDriver`).
  - **The examples on aqua** (`aqua_test.go`, `examples_*_aqua_test.go`, `scroll_*_aqua_test.go`):
    - Pinned `.frames` dumps. Anchor widgets are checked for overlaps and clipping (`misplaced`).
    - Paint and a11y dumps of `hello`.
    - `TestExamplesAccessibleOnAqua`: every visible widget's a11y node matches its getters.
    - Menus driven end to end by mouse, Alt/F10 and shortcuts, including keyboard layouts (`TestExampleControlsMenusOnAqua`, `TestExampleNotesMenusOnAqua`).
    - Keyboard-only runs (`TestFormByKeyboardOnAqua`, `TestControlsByKeyboardOnAqua`, `TestFormKeysOnAqua`, `TestHelloTypingOnAqua`).
    - Radio groups, tooltips, the accent menu, and the `notes` sidebar's wheel, momentum, rubber band, legacy scrollers and keyboard paging.
- **`driver`**: key normalization and modifier resolution, shortcut display, menu helpers, geometry, colors, font defaults, enum values matching AppKit.
- **`driver/stub`**: recorded state, exact fake metrics, every `User…` simulation (disabled controls, and controls under a disabled container, box or scroll view, ignore input: `Base.Enabled`), scroll clamping, UTF-8 selection boundaries, windows, menu bar, alerts (no run loop since R7).
- **`driver/headless`**: the queue contract (`Ready` once, FIFO across producers, `Quit` from anywhere, no `Queue` before `Run` and a closed one after it, nested `Run`, panics, no frame clock). A `Run` that nothing quits blocks; this is tested in a child process.
- **`mainthread`**: FIFO order, lost wake-ups under stress, coalesced wakes, nested drains, panics, `DoSync` in every situation, the off-thread check, benchmarks.
- **`ffi`**:
  - **Handles:** reuse, stale and foreign handles, slot retirement, bit layout, concurrent stress.
  - **Callback registry:** tested on fakes (budget, signatures, a per-OS signature table) and with real callbacks (`qsort` on unix, a window-proc-shaped callback on Windows, foreign threads, deep nesting).
  - **Native access:** Dlopen/Sym/Bind, C strings, raw-memory readers and writers on pinned memory, layout helpers.
- **`layout`**: measuring, clamping, alignment, `distribute` (a table of cases, bit-exact sums, generated inputs), stacks, forms, hosts, the measure cache. Each example layout is checked at its design size, at its minimum and at larger sizes, with frames compared to within 1e-9.
- **`render`**: path iteration, `Bounds` against dense sampling, `SegmentsBounds` against `Bounds`, `ArcTo`, normalization of radii, paint, stroke and shadow (normal values shared, no allocation), transforms. **`render/record`**: dumps of every op kind, transform and clip tracking, balance errors, dump stability under float noise, ops that keep their own paths, stops and dashes.
- **`shell`**: event types pinned at compile time. `TestFrameTimer` covers frame records. `TestPacing`/`TestPacingTolerance` run synthetic vblank traces: late and missed frames, animation ends, drifting clocks.
- **`shell/fake`**:
  - Text metrics, including line and caret queries at soft and hard breaks.
  - The clock and timers (`After`: ordering, cancellation, panics, stress).
  - The shell loop, surfaces (clamping, `Resized` only on change, one paint per frame, chrome hit-testing), every input helper, and first-click activation.
  - The clipboard: reads arrive on `Step`, never inside the call.
  - Popups: grab routing, dismissals, tracks, placement on the work area, shadows.
  - Modal surfaces: blocking, placement, key hand-off on close, owners closing modals first.
- **`shell/shellkit`**: event time arithmetic, the click counter, gesture phases, point and pixel round trips at seven scales, size clamping, centring, chrome hit regions, popup placement (sliding, cutting, larger than the work area), the `Timers` order and round rule, `ClipboardText`, the `FixedText` layouts, and that the native shells' shipped files do not import `shell/fake`.
- **`shell/gtk4/gtk`**: the binding tables (every func field bound once, from the right library, under the C name its field spells), the layouts of mirrored structs, GTypes and enums, the version checks, and the "missing library" error and report. On Linux: every library loads, every symbol resolves, and the readers work on `g_malloc0` memory.
- **`shell/gtk4`**: on every host, over fakes that follow GLib/GTK semantics and fire the real trampolines:
  - **Trampolines:** the fixed set, their signatures, and panic trapping.
  - **Widget class:** the vfunc slots, measure, allocate, snapshot.
  - **Shell loop and teardown.** After every `Run`, no source or handle is left and the wake and queue are closed.
  - **Frame clock:** tick and timeout fallback, stalls, the watchdog.
  - **Surfaces, chrome and input:** caption move and double-click, pointer with capture and first click, scroll with momentum, keyboard with the Shift level, IME preedit and text-input ordering.
  - **Canvas:** fills, paths, strokes, shadows, gradients, images, the backdrop blur with edge mirrors, Pango text, all checked against a recording snapshotter that verifies nesting.
  - **Text:** geometry, family resolution, the fallback to fake metrics, and the Inter app-font cache with its fallback.
  - **Appearance:** the portal and GtkSettings mapping, the D-Bus portal client against an in-process fake bus (`fakeBus` over `net.Pipe`), live changes (`TestAppearanceChangedEveryKey`: each key gives exactly one change).
  - **Clipboard, timers, popups (popovers) and modal windows.**
  - **Golden seams:** fixed scale, fixed key state, readback.

  On Linux, under xvfb through `native.sh`, the same areas run against real GTK. Canvas node trees are read back with test-only GSK getters, and the tests log `*-RESULT:` lines.
- **`shell/native`**: `New` returns a fresh Win32 or GTK shell per call, and `ErrNotImplemented` elsewhere.
- **`shell/win32/w32`**: struct layouts, procs resolving, error wrapping. On real windows: window-long rules, the message loop, `WM_QUIT`, `CREATESTRUCTW`/`NCCALCSIZE`/`MINMAXINFO`, the IMM context (`TestImmContext`) and the frame-clock calls.
- **`shell/win32/com`**: vtables and slot checks, GUID parsing, HRESULTs and `errors.Is/As`. On Windows: `InitUIThread` modes, reference counts on a real WIC factory, `Bind` on real vtable slots, and the float-argument path against `ucrtbase`/`d2d1` (the arm64 path runs only through `native.sh`).
- **`shell/win32/dxgi`, `d3d11`, `dcomp`**: slot constants, layouts, error codes. One smoke test on the default device and on WARP covers the swap chain, the DComp target, clear-and-readback, present/commit/resize rules, and teardown with every reference count at its expected value.
- **`shell/win32/d2d`**: slot constants and layouts. Smoke tests on the default device and WARP. `TestDraw` and `TestEffects` read pixels back through staging to check fills, strokes, dashes, clips, layers, shadow and blur effects, bitmaps and text layout.
- **`shell/win32/dwrite`**: factories and IIDs; text formats and layouts with metrics checked against ranges; hit-testing and clusters (combining marks, emoji, CRLF, flags); in-memory fonts, with invalid data rejected without leaks.
- **`shell/win32`**: real windows. Every test shell has the recorder on. `inShell` requires each shell to end with no GPU devices and no leftover references. Areas:
  - **Core:** `Run`/`Ready`/`Quit` on a Per-Monitor-V2 STA thread, the post queue, panics in the window procedure.
  - **Appearance:** `TestAppearanceChangedEveryField`; each OS setting changed alone gives exactly its field and one event. Caret blink is included.
  - **Clipboard:** codec and a real round trip that saves and restores the user's clipboard.
  - **Timers:** real `WM_TIMER`s.
  - **Frame clock:** vblank grid, stuck waitables, device loss, live resize presenting exactly one frame per step.
  - **Presentation:** light and dark readback, DPI changes, injected device loss in `EndDraw`, `Present` and `ResizeBuffers`. The device-loss policy (`TestSurfaceDeviceLossPolicy`, `TestShownPopupDeviceLossOnCreate`): a loss while a new surface or popup creates its chain or bitmap, a loss during the recovery (rebuilt once, then deferred to the next paint), a lost `Commit`, the re-entrancy guard, and the WARP fallback after 3 recent losses (older ones don't count). A real TDR is not run.
  - **Canvas:** offscreen 128×128 readback at 96 and 192 DPI, covering shadows, blur, images and every misuse.
  - **Text:** DirectWrite contract, carets and lines, draw cache bounds, embedded fonts. Expected x positions come from measuring prefixes, never hard-coded metrics.
  - **Surfaces and chrome:** `hitTest`/`NCCALCSIZE`/DWM attributes, maximized layout.
  - **Input:** key mapping with injected layouts, the live layout, IME composition through a faked `imm` seam, first click.
  - **Popups:** shown popups, the grab with messages sent synchronously, the DWM backdrop.
  - **Modal windows.**
  - **Gallery render and golden** on WARP. `TestCaptureWindow` (`GOGUI_CAPTURE`) captures another process's window.
- **`aqua`** (over the fake shell, mostly through a running driver):
  - **Element tree and router:** paint dumps, hit tests, hover and capture, scroll latching, key bubbling, preedit routing, Tab order and keyboard access, initial focus, default and cancel buttons, first click, window key and appearance state, the a11y tree, the driver's timeline (`TestDriverTimeline`), paint culling.
  - **Each control** (label, separator, box, button, checkbox, radio, slider, progress, pop-up button, text field, text view, scroll view and scrollers, menus, menu strip, tooltips, alerts, sheets, app-modal alert surfaces):
    - fitting size and caching;
    - paint cases, including truncation, degenerate frames and pixel snapping at 2×/1.5×;
    - mouse and keyboard interaction, the focus ring and its animation, the a11y node;
    - a pinned `*.states` table.
  - **Text editing:** keymap, mouse selection, clipboard, caret blink, IME composition and the published text-input caret. For the TextView also the paragraph height index (model-checked, fuzzed), the paragraph layout cache, idle layout, autoscroll, and 1 MB texts that lay out only what is visible.
  - **Menus:** geometry, interaction, accelerators with Shift-level rules, keyboard access to the strip, role actions (App, Window, Edit).
  - **Cross-state matrices:**
    - `TestInactiveWindowMatrix` (every control state in key and inactive windows, with several accents).
    - `TestContrastMatrix` (Increase Contrast adds outlines only where needed).
    - **`TestMotionInventory`.** Every animation or timer start must be listed with its Reduce Motion path, and each listed entry has its own Reduce Motion test.
    - `TestLiveAppearance` (each `driver.Appearance` field changed live with windows, sheets, menus and tooltips open must paint exactly what a fresh scene under the new appearance paints).
    - The accent override and light-accent labels.
  - **Theme use:** `TestThemeColors` and `TestChromeReadsTheme` check that the driver draws from `theme.Sequoia` and that the chrome follows its metrics.
- **`aqua/gallery`**: `TestGalleryScene` (widget counts per kind; every child inside its container with no sibling overlap; frames at least the fitting size; no animation or paint errors), `TestGalleryBuild`, `TestGalleryFill`, `TestGalleryDump` (pinned `gallery.dump`).
- **`aqua/gallery/golden`**: synthetic images cover the threshold, anti-aliased weighting, alpha-only and hidden-channel changes, size mismatches, premultiplied and strided images, masks (`TestCompareMask`), and deterministic PNG round trips. **`goldentest`**: the variant list (each golden committed), BGRA conversion, `Check` update/match/fail/`LogOnly` paths over a recording `testing.TB`, and the threshold sweep.
- **`aqua/a11y`**: role, state and action names, `Dump`, `Walk`, `Find`.
- **`aqua/anim`**: Bézier curves against an independent bisection, springs against RK4 integration, `Lerp`. Timeline behaviour on a fake clock covers exact per-frame values, late frames, retargeting, `SpringFrom`, `Decay`, `AnimateAfter`, tickers, tolerances (settling, skipped invisible frames), Reduce Motion and panics.
- **`aqua/scroll`**: rubber band and its inverse, decay against the closed form, `VelocityTracker` on synthetic Win32 bursts. `Axis` on a fake clock covers drags, flings, edges, spring-back, wheel steps, OS momentum and Reduce Motion.
- **`aqua/menubar`**: building bars from the `controls` and `notes` specs (role titles, default shortcuts), shortcut lookup (exact before shifted, keypad `+`, conflicts), strip layout and title-drop thresholds.
- **`aqua/text`**:
  - **Piece-table buffer:** edits and the observer.
  - **Grapheme and word boundaries:** checked against uniseg, including invalid UTF-8.
  - **Editor:** moves, edits and wrapped lines, each case run with several read stretches.
  - **Undo and redo:** grouping and limits.
  - **Keymap:** the whole map is swept under all 16 modifier sets against an independently written table, and every command is checked to match its method.
  - Random runs and fuzzers (`FuzzBuffer`, `FuzzBoundaries`, `FuzzEditor`, `FuzzUndo`) compare everything with a `string` model and replay the observer's reports.
- **`aqua/theme`**:
  - **Generated files:** the committed `tokens.go` and `docs/tokens.md` must equal a fresh generation byte for byte.
  - **Token completeness:** every semantic and chrome color has a concrete token, and `Resolve` follows the accent and focus-ring rules.
  - **Sanity rules for every token group:** sizes grow from mini to large, colors are concrete and visible, contrast variants are stronger.
  - **Readability rules:** text must be readable on its fills (WCAG ratio, light-accent text), and Increase Contrast borders must stand out more while fills stay unchanged.
  - **Animation tokens:** in range, and zero under Reduce Motion.
  - `TestFontSizeMatchesEffectiveSize` ties `Typography` to `driver.Font`.
- **`aqua/theme/gen`**: `ColorNames` and `Groups` are checked against the constants and structs parsed from `driver/color.go` and `theme.go`. The spec parser accepts valid specs and rejects every malformed field.
- **`assets/fonts`**: `IsFontFile`, `Files` sorted with data, the README embedded.

