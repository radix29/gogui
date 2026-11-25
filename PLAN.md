# gogui: Plan for a Cross-Platform Go GUI Toolkit with a macOS Look and Feel

> **Goal:** a Go library for desktop apps on **macOS, Windows and Linux** from **one
> portable codebase**. On macOS the apps use **real AppKit controls**. On Windows and Linux
> they **look and behave like macOS apps**: the host OS provides windows, text, input, IME,
> dialogs, clipboard and accessibility wherever possible, and everything custom-drawn is
> drawn on the **GPU**.

This file holds the design, the decisions and **only the open work**. What is built is
described in [`ARCHITECTURE.md`](ARCHITECTURE.md); finished steps are not kept here (the
git history has them). Module path: `github.com/radix29/gogui`.

**Status (2026-11-20).** v0.1 is feature-complete on the aqua driver (phases 0, 2, 3, 5
and 6 built; no release, no tag). Windows is checked on the host. Linux (gtk4) is built
and host-tested, its native runs wait on the user's Linux log. macOS still runs the
headless driver: phase 1 waits on a Mac.

| Waiting on | Open work |
|---|---|
| Nothing (Claude) | **Next:** phase R, the review fixes of [`docs/review-plan.md`](docs/review-plan.md) (§11.1) |
| The user's Linux log | S4.3j and its gallery follow-up (phase 4 exit) |
| A Mac | Phase 1 (S1.1–S1.5), then S3.1t (phase 3 exit) |
| macOS and Linux logs, the user | S0.10c (phase 0.5 exit) |
| The user | The open checks of [`docs/parity.md`](docs/parity.md) §4 (phase 5 and 6 exits) |

---

## 1. Guiding Principles

1. **One API everywhere.** App code never uses build tags; the same `main.go` builds on all
   three platforms.
2. **macOS is the reference.** The public API follows AppKit (NSWindow, NSButton,
   NSTextField, …); on macOS each call maps almost 1:1 to a native object.
3. **Use the OS for everything except the widget pixels.** On Windows and Linux the OS
   provides windows, compositor effects (shadow, rounded corners, blur/Mica), font
   rasterisation and shaping, IME, clipboard, drag and drop, file dialogs, DPI and
   accessibility. The toolkit draws only the controls, because neither Win32 nor GTK
   controls can be restyled to behave like AppKit.
4. **Draw on the GPU with the platform's 2D API:** Direct2D/DirectWrite on Windows, GSK on
   Linux. No custom rasterizer, no WebView.
5. **Match behaviour, not only pixels:** focus rings, default/cancel buttons,
   inactive-window appearance, animation timing, keyboard navigation, text editing and
   menus are specified and tested alongside the visuals.
6. **Idiomatic Go, no cgo.** Plain structs and methods, no code generation for users,
   callbacks always on the UI thread. macOS (Objective-C runtime) and Linux (GTK 4) go
   through [`ebitengine/purego`](https://github.com/ebitengine/purego), Windows through
   `golang.org/x/sys/windows` plus purego for COM calls with float arguments (D5, S2.4a).
   Every platform builds with `CGO_ENABLED=0` and cross-compiles from any OS (§9).

---

## 2. Architecture Overview

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

### 2.1 Layers

| Layer | Package | Responsibility |
|---|---|---|
| Public API | `gogui` | Widgets, windows, menus, layout, events, colors, fonts. The only package apps import. |
| Driver interfaces | `internal/driver` | "Peer" interfaces (`driver.Button`, `driver.Window` …), one peer per public widget, the same pattern AWT/SWT use. |
| macOS driver | `internal/appkit` | Implements the peers with AppKit objects, called through the Objective-C runtime with `purego/objc` (no `.m` files, no cgo). |
| Native bindings | `internal/ffi` | Shared purego helpers: library loading, a fixed set of callback trampolines, struct/ABI helpers, `objc` class registration (§6.1, §7.2). |
| Aqua driver | `internal/aqua` | Implements the peers as **drawn widgets** that replicate AppKit visuals and behaviour. Shared by Windows and Linux. |
| Theme | `internal/aqua/theme` | Design tokens: colors (light/dark/high-contrast), metrics per control size, fonts, radii, shadows, animation curves. |
| Render | `internal/render` | GPU-backed 2D `Canvas` interface plus a recording backend for tests. |
| Shell | `internal/shell` | OS integration interface for the aqua driver: native windows/popups, event loop, input, IME, text shaping, clipboard, dialogs, system settings, frame clock. |
| Shell: Windows | `internal/shell/win32` | Win32 + DWM + Direct2D + DirectWrite + DirectComposition via `golang.org/x/sys/windows` and hand-written COM vtables (`purego` only for COM calls with float arguments). **No cgo.** |
| Shell: Linux | `internal/shell/gtk4` | GTK 4, GLib and Pango loaded at runtime with `purego.Dlopen` (no cgo, no C shim). Drawing goes through `GtkSnapshot` → GSK render nodes (GPU). Portal settings use pure-Go D-Bus (`godbus/dbus/v5`). |
| Layout | `internal/layout` | Stack / form layout (a grid comes after v0.1) used by **all** drivers, so layouts behave the same on every OS. |


---

## 3. Technology Stack

| Concern | macOS | Windows | Linux |
|---|---|---|---|
| Binding technique | `purego/objc`: `objc_msgSend`, `objc.RegisterClass` for delegates/targets/flipped views; frameworks loaded with `purego.Dlopen` | Pure Go: `golang.org/x/sys/windows`, `syscall.SyscallN` on COM vtables; `purego.RegisterFunc` on the vtable slot for methods with float arguments | `purego.Dlopen("libgtk-4.so.1")` + `RegisterLibFunc`; GObject signals via callback trampolines |
| Event loop | `[NSApp run]` on main thread | `GetMessage` loop on locked main thread | `GApplication` / GLib main loop |
| UI-thread dispatch | `performSelectorOnMainThread:` on a registered class, or a `CFRunLoopSource` (avoids ObjC blocks) | `PostMessage(WM_APP_INVOKE)` | `g_idle_add` with a fixed trampoline |
| Windows | `NSWindow` | `HWND` with custom non-client area (`WM_NCCALCSIZE`, `WM_NCHITTEST`) | `GtkWindow` with custom titlebar (CSD) |
| Window chrome | Native | DWM: rounded corners (`DWMWA_WINDOW_CORNER_PREFERENCE`), shadow, dark mode (`DWMWA_USE_IMMERSIVE_DARK_MODE`), Mica/Acrylic (`DWMWA_SYSTEMBACKDROP_TYPE`, Win11 22H2+) | GTK CSD shadow and rounded corners. Blur only where the compositor supports it (KDE); otherwise a translucent solid fill. |
| Controls | AppKit | Drawn (aqua) | Drawn (aqua) |
| GPU drawing | Core Animation (native) | D3D11 device → Direct2D device context → DXGI flip-model swap chain → DirectComposition visual | `GtkSnapshot` → GSK render nodes (GL/Vulkan renderer) |
| Text | Core Text (native) | DirectWrite (`IDWriteTextLayout`), grayscale AA to match macOS | Pango + HarfBuzz + FreeType (`gtk_snapshot_append_layout`) |
| Fonts | SF Pro (system) | **Inter** (bundled, OFL) with SF-like optical tracking; fallback Segoe UI Variable | **Inter** (bundled); fallback system UI font |
| Icons / glyphs | SF Symbols | Procedural vector glyphs (chevrons, checkmarks …) plus an open icon set (Lucide / Phosphor) | Same as Windows |
| IME | Native | IMM32 (v1) → TSF (later) | `GtkIMContext` |
| Clipboard / DnD | `NSPasteboard` | `OpenClipboard` / OLE DnD | `GdkClipboard` / `GtkDropTarget` |
| File dialogs | `NSOpenPanel` / `NSSavePanel` | `IFileOpenDialog` (native) | `GtkFileDialog` (xdg portal) |
| Dark mode / accent | `NSAppearance`, `controlAccentColor` | Registry `AppsUseLightTheme`, DWM colorization / `UISettings` | xdg-desktop-portal `org.freedesktop.appearance` (`color-scheme`, `accent-color`) over pure-Go D-Bus |
| Scrolling | `NSScrollView` (native) | `WM_MOUSEWHEEL`/`WM_MOUSEHWHEEL` high-resolution deltas, `WM_POINTER` for touchpads; momentum and rubber-banding done in aqua | `GtkEventControllerScroll` (with `KINETIC` flag and `decelerate` signal); rubber-banding done in aqua |
| HiDPI | Backing scale factor | Per-Monitor-V2 DPI awareness, `WM_DPICHANGED` | GTK scale factor (incl. fractional on Wayland) |
| Frame clock / vsync | `CADisplayLink` / `CVDisplayLink` | DXGI waitable swap chain / DWM vblank timing | `GdkFrameClock` |
| Accessibility | Native (free) | UI Automation provider (or AccessKit) | AT-SPI via GtkAccessible (or AccessKit) |
| Packaging (later) | `.app` bundle + `Info.plist` | Embedded manifest (`.syso` via go-winres) | `.desktop` file, AppImage/Flatpak |

**Minimum OS targets (D7):** macOS 13+ (Intel and Apple Silicon), Windows 10 1809+ (Mica,
rounded corners and Snap Layouts only on Windows 11), Linux with GTK ≥ 4.14 and Pango ≥ 1.52
on Wayland or X11. amd64 and arm64 on all three.

### 3.1 Licensing and legal constraints

* **SF Pro and SF Symbols** are licensed for Apple platforms only: never bundle them.
  Use **Inter** (OFL, metrically close) with an SF-like tracking table (D9).
* **Never ship Apple bitmaps or assets.** Every aqua control is drawn procedurally from
  theme tokens and scales to any DPI.
* **gogui is MIT-licensed** (`LICENSE`, D12). Dependencies and the bundled Inter keep
  their licenses; `THIRD_PARTY.md` lists each with its version, and
  `TestThirdPartyNotices` (`internal/repocheck`) keeps it in step with `go.mod`.

---

## 4. Public API Design

The package docs (`go doc -all github.com/radix29/gogui`) and the examples are the
reference for the API as built; `examples/hello` is the hello world (§4.1 was its
sketch). A public-API change updates this section, the examples and `examples/README.md`
in the same step (§11.1 rule 6).

### 4.2 API conventions

* **Threading:** `gogui.Run` must be called once, from `main`; the main OS thread is
  locked in `init` (`runtime.LockOSThread`), and `Run` blocks until `Quit`. It returns
  an error if the backend can't start or if it is called twice, and library code never
  calls `os.Exit`. All callbacks run on the UI thread. Goroutines use `gogui.Do(func())`
  (async, in order) or `gogui.DoSync(func()) bool` (waits; inline on the UI thread; a
  panic in fn is re-raised as a `*gogui.PanicError` on every thread) to touch widgets.
  Outside `Run` there is no queue, so `Do` drops fn and `DoSync` returns false; `DoSync`
  from another goroutine also returns false after `Quit`. Constructors (`App.NewWindow`
  included) and `Alert.Show` always panic outside `Run` (before the ready callback or
  after `Run` returns). With `GOGUI_DEBUG=1`, every public method also panics off the UI thread;
  without it the check costs one atomic load. On Windows and Linux `GOGUI_DEBUG=1` also
  logs every frame whose work is over a 120 Hz frame (8.3 ms), split into the frame
  function, layout, paint and submit.
* **Units:** logical points everywhere (as in AppKit). Top-left origin. DPI is handled
  inside the shell.
* **Setters return the receiver** so calls can be chained (`SetPlaceholder(...).SetEnabled(false)`).
  The shared setters (`SetEnabled`, `SetHidden`, `SetTooltip`, `SetControlSize`, `Focus`)
  live once in an embedded generic type, so `go doc` shows them returning `T`, the
  widget's own type. Getters have no `Get` prefix. Actions (`Show`, `Close`, `Quit`)
  return nothing.
* **Events:** `OnClick`, `OnChange`, `OnSubmit` … take a single `func` and return an
  `Unsubscribe` function. An event can have several handlers, called in the order they
  subscribed; a handler added during a dispatch first runs on the next one.
  `Unsubscribe` is idempotent and safe during a dispatch. Setters never fire events
  (only the user or the OS does), and disabled controls fire none (a scroll view still
  reports scrolling, as in AppKit). For the veto events
  (`Window.OnShouldClose`, `App.OnQuitRequested`) the first handler that returns
  `false` vetoes, and the handlers after it are not called.
* **Lifetime (D13):** a widget lives until it is destroyed. Taking it out of a window or
  container (`SetContent`/`SetDocument` replacing it, `Stack.Remove`/`Clear`) does not
  destroy it, so it can be shown again. `Widget.Destroy()` detaches it and destroys its
  subtree bottom-up; closing a window destroys its tree, and `Run` destroys every widget
  still alive when it returns. A destroyed widget panics when used (setters, `On*`,
  actions, being placed or having something placed in it); its getters return the last
  values. A handler that destroys its own widget ends that event's dispatch. A closed
  window drops its content and handlers. The toolkit never relies on finalizers. Native objects refer back to Go
  peers through an integer handle table (`internal/ffi.Handles`), never through Go
  pointers.
* **Value types are aliases** of the internal driver types (`gogui.Size`, `Point`,
  `Rect`, `Insets`, `Color`, `Font`, `KeyShortcut`, `WindowConfig`, `AppConfig` …), and
  their constants are `gogui` names (`gogui.Titled`, `gogui.StateOn`,
  `gogui.AlertWarning`). Apps never need an internal package. A few methods and fields on
  these types exist for the drivers (`KeyShortcut.Resolve`, `Font.EffectiveSize`,
  `Color.Semantic`); apps can ignore them.
* **Modifiers:** `gogui.KeyPrimary` means ⌘ on macOS and Ctrl on Windows/Linux.
  Standard shortcuts (`gogui.ShortcutCopy()` …, functions so nothing can change them)
  resolve per platform.
* **Semantic colors and fonts** mirror NSColor/NSFont: `gogui.ColorLabel`,
  `gogui.ColorSecondaryLabel`, `gogui.ColorControlAccent`, `gogui.ColorWindowBackground`,
  `gogui.SystemFont(size)`, `gogui.BoldSystemFont(size)`. They resolve against the current
  appearance (light/dark) automatically.
* **Accent color** (D10): the OS accent by default; `app.SetAccentColor(c)`
  overrides it for the whole app (a semantic `c` follows light and dark; an accent is
  always opaque), `ColorControlAccent` or the zero `Color` clears it, and
  `app.AccentColor()` returns the effective, concrete accent. On a light accent the labels
  drawn on it turn dark (WCAG 3:1 against white). An accent change does not call
  `OnAppearanceChange`, which stays light/dark only.
* **Control sizes:** `gogui.ControlSizeMini`, `ControlSizeSmall`, `ControlSizeRegular`
  and `ControlSizeLarge` (NSControl.ControlSize).

### 4.3 Driver (peer) interfaces (internal)

`internal/driver` defines one peer interface (and one handler interface) per public
widget, created by a `Driver` factory (ARCHITECTURE.md §2). Contracts: setters never call
handlers (handlers report user/OS changes only); peers have getters only for
user-changeable state (`Text`, `Value`, `State`, `Selected`, `Offset` …); text offsets
are UTF-8 byte offsets; menu items carry integer IDs, not funcs. Every driver exposes its
UI-thread queue (`Driver.Queue`) and a frame-time hook (`ObserveFrames`) in the interface
itself, and `newPlatformDriver` returns `(Driver, error)`. `internal/driver/stub`
implements every peer interface and simulates input with `User…` methods; the headless
driver adds the run loop to its `stub.Driver`.

### 4.4 AppKit mapping (macOS) ↔ aqua equivalents (Windows/Linux)

| gogui | macOS (native) | aqua (drawn) notes |
|---|---|---|
| `Window` | `NSWindow` | Custom title bar with traffic lights, centered title, DWM/GTK shadow, inactive-state dimming |
| `Label` | `NSTextField labelWithString:` | Text layout, selectable option |
| `Button` | `NSButton` (`.push` bezel) | Gradient bezel, accent fill when default, pressed state, focus ring |
| `Checkbox` | `NSButton` (`.switch`) | 14 pt rounded box, animated checkmark, mixed state |
| `RadioGroup` (private radios) | `NSButton` (`.radio`) | Accent-filled circle with a white dot. The group handles exclusivity; one Tab stop, the arrows move the selection without wrapping |
| `TextField` (`NewTextField` / `NewSecureField`) | `NSTextField` / `NSSecureTextField` | Go text model, native shaping, IME, clipboard, undo, macOS editing keys |
| `Slider` | `NSSlider` | Track with accent fill, knob with shadow, optional tick marks |
| `ProgressBar` / `Spinner` | `NSProgressIndicator` | Determinate bar, indeterminate animation, spinning indicator |
| `PopUpButton` | `NSPopUpButton` | Menu opens **over the selected item** in a native popup surface |
| `Separator` | `NSBox` (`.separator`) | 1 px hairline, alpha-blended |
| `Box` | `NSBox` (`.primary`) | Rounded group background with title |
| `ScrollView` | `NSScrollView` + flipped document view | Overlay scrollers, momentum, rubber-banding, keyboard paging, scroll-to-focused (§5.6) |
| `TextView` | `NSTextView` inside `NSScrollView` | Piece-table text model in an aqua ScrollView; per-paragraph layouts, only visible lines painted (§5.6) |
| `VStack` / `HStack` (`*Stack`) / `Form` (`*FormView`; `Grid` after v0.1) | Flipped `NSView` containers, laid out by Go | The same Go layout engine |
| `MenuBar` / `Menu` (`MenuBarSpec` / `MenuSpec`) | `NSApp.mainMenu` (global menu bar) | Menu strip in the title bar of every titled window (D3, D11, §5.4.1) |
| `Alert` | `NSAlert` (sheet or modal) | The macOS 11+ panel, drawn: a sheet of its parent, or an app-modal surface of its own (§5.4) |
| `App.SetAccentColor` / `AccentColor` | No app-wide override in AppKit: the driver uses the color as the per-control tint (`bezelColor`, `contentTintColor`); `AccentColor` returns it, else `controlAccentColor` | Replaces `Appearance.Accent` before the theme, so every accent-derived color follows (D10) |

---

## 5. The Aqua Engine (Windows/Linux)

How it is built: ARCHITECTURE.md §9. This section is the spec it is checked against.

### 5.1 Element model

* **Retained element tree**, one `aqua.Element` per public widget. Each element is a
  `layout.Item` (`FittingSize`, `SetFrame`; `internal/layout` arranges the peers, elements
  never measure or arrange themselves) and has `Paint(canvas)`, `HandleEvent(ev)` and an
  `AccessibleNode()`.
* **State per control:** `normal / hover / pressed / focused / disabled` × `key /
  inactive window` × `light / dark` × `increased contrast`. As on macOS, an inactive window
  derives no color from the accent (accent fills grey, the default button plain, the
  selection unemphasized, focus ring and caret hidden). The first click into an inactive
  window only activates it (AppKit's `acceptsFirstMouse` = NO), except on the traffic
  lights and the menu strip.
* **Invalidation:** frames run only while something is requested (an animation, an
  invalidation); v1 repaints the whole window. Dirty rectangles and layer caching are v2.

### 5.2 Theme tokens

* `internal/aqua/theme/tokens.go` is generated from `spec/sequoia.json`: colors (light,
  dark, increased contrast), metrics per control size, typography, animation
  durations/curves. Generated code is committed.
* Tokens are **measured from reference screenshots** of native AppKit controls at 1× and
  2× (§8.3), never copied from Apple assets. Until S3.1t they are provisional
  (`source: "memory"`).
* **Accent:** the OS accent (D10) or the app's override; macOS blue by default. A light
  accent turns the labels on it dark (WCAG 3:1 against white).
* **Theme target:** macOS 14/15 (Sonoma/Sequoia). Liquid Glass (macOS 26+) comes later as
  a second token set plus shader effects (D2).

### 5.3 Rendering interface

`render.Canvas` (ARCHITECTURE.md §7) is what elements draw on: save/restore, transforms,
rounded clips, rounded-rect fills and strokes, paths (nonzero fill, strokes with caps,
joins, dashes), solid and linear-gradient paints, outer/inner shadows, text layouts,
images, group opacity and backdrop blur. The shell provides `TextSystem` /
`TextLayout` (UTF-8 byte offsets; hit-testing, carets, selection rects, line metrics).
Backends: Direct2D/DirectWrite on Windows, GSK/Pango on Linux, a recorder for tests.

### 5.4 Window chrome (the hard part of "looks like a Mac")

* **Windows:** no standard title bar (`WM_NCCALCSIZE`), but DWM resize borders, shadow
  and rounded corners. Traffic lights and title drawn in the client area; `WM_NCHITTEST`
  answers `HTCAPTION` for the drag area and `HTMAXBUTTON` over the green button so
  **Snap Layouts work**. Mica/Acrylic can show through transparent areas.
* **Linux:** a CSD `GtkWindow` with our own drawn title bar; dragging calls
  `gdk_toplevel_begin_move`, the buttons minimize / maximize / close.
* **Traffic lights:** 12 pt red/yellow/green circles; hovering the group shows ×/−/+;
  grey in an inactive window.
* **Menus and popups** (PopUpButton list, menus, tooltips) are **separate native popup
  surfaces**, so they can extend past the window edges. Menus are grab popups (the parent
  stays key), tooltips non-grab. Acrylic on Windows 11 22H2+, opaque elsewhere.
* **Sheets** are drawn inside their window: the panel hangs from the title bar and slides
  down over 0.2 s (fades with Reduce Motion); the content takes no input meanwhile, the
  close light and the Close/Edit items are disabled, the title bar still moves the window.
  An **app-modal alert** (or the sheet of a window too small for it) is a modal surface
  of its own that blocks every other window; only Quit stays enabled.

### 5.4.1 Menu bar in the title bar (decision D3)

On Windows and Linux the menu bar is drawn **inside the title-bar area of each window**,
right of the traffic lights:

```
┌──────────────────────────────────────────────────────────────────────┐
│ ● ● ●   Controls   File   Edit   View   Help           Controls      │  ← 28 pt title bar
├──────────────────────────────────────────────────────────────────────┤
```

* The first item is the **app menu** in bold (About, Settings…, Hide, Quit). The window
  title stays centered and is truncated or hidden when the menus need the room; a minimum
  drag area always stays.
* Each title is its own hit region (`HTCLIENT`); the space between stays `HTCAPTION`.
* **Mac behaviour:** a click opens a menu; with one open, hovering another title switches;
  press-drag-release picks in one gesture; items highlight with the accent; shortcuts
  (`⌘C`, shown as `Ctrl+C`) right-aligned, submenu chevrons. **Alt or F10** focuses the
  strip, the arrows navigate.
* Accelerators work with the menus closed and run before the focused control; items are
  validated when a menu opens or an accelerator fires (the Edit roles act on the focused
  text control). Paste is enabled in any editable text control whatever the clipboard
  holds (unlike AppKit, which disables it for an empty pasteboard): the shell reads the
  clipboard asynchronously, so the validation cannot look at it; pasting no text does
  nothing.
* Every titled window shows the strip, all of the one bar (D11); no "main window" API.
  Windows without a title bar have no strip, but accelerators work in every key window.
  An app that never calls `SetMenuBar` shows no strip.
* v0.1 has no cascading menus: a submenu item shows a chevron and stays disabled.
* On Linux a press on the strip while a menu is open dismisses it (GDK's popover grab), so
  a click on another title closes the menu instead of switching; hover-switching works.

### 5.5 Behaviour ("feel") checklist

The state of each behaviour per target and its open checks are rows of
[`docs/parity.md`](docs/parity.md) §3.2.

* **Focus ring:** accent-colored glow outside the control that animates in. Tab /
  Shift-Tab move focus; full keyboard access optional, as on macOS.
* **Default button** (Return) and **cancel button** (Escape). Space activates the focused
  button.
* **Pressed state follows the pointer:** dragging off un-highlights, releasing outside
  does not click.
* **Text fields:** macOS editing semantics (double-click word, triple-click line or
  paragraph, Option+arrows by word, Primary+arrows to line ends, OS caret blink rate),
  `Primary+A/C/V/X/Z`, `Shift+Primary+Z`. Primary is Ctrl on Windows/Linux. One deliberate
  deviation: plain Home/End and Page Up/Down move the caret. IME composition is drawn
  inline (clauses underlined); a blur, click or deactivation commits it. Not in v1:
  surrounding text, TSF, reconversion.
* **Pop-up buttons:** the menu opens over the selected item, at least the button's width;
  click-click or press-drag-release (a release within 300 ms and 3 pt keeps it open);
  arrows, Home/End, type-select (reset after 1 s; the same letter again steps to the next
  item with it, as in the title-bar strip), Return/Space pick, Escape cancels; a pick
  blinks once.
* **Tooltips:** after a 1 s rest below the pointer, then at once within a 500 ms grace;
  hidden by a press, key or scroll until the pointer leaves.
* **Title-bar menus:** §5.4.1; the pick blink and a 100 ms title flash on an accelerator.
* **Alerts:** Return presses the default button, Escape and Primary+. the Cancel button
  (100 ms flash); with full keyboard access Tab moves between buttons and Space presses.
  An alert answers once; a second alert on a window waits for the first; closing the
  window answers with Cancel.
* **Animations:** 0.2–0.25 s ease-in-out by default, springs where macOS uses them.
  **Reduce Motion** jumps animations to their end (the sheet fades, no rubber band or
  momentum, no pick blink); non-motion timers (caret blink, tooltip delay, auto-scroll)
  stay. **Reduce Transparency** makes menus and popups opaque.
* **Scrolling:** §5.6.
* **Window:** centered title, "zoom" on the green button, inactive appearance for every
  control, first click only activates.

### 5.6 ScrollView and TextView (decision D8)

**ScrollView**

* A clip, a content offset and one document element; the document is measured with an
  unbounded height (or width) and the offset is clamped.
* **Overlay scrollers:** thin, drawn over the content, shown while scrolling, fading out
  after about 1 s, wider on hover; click-to-page and knob dragging. When the OS asks for
  always-visible scroll bars (`Appearance.LegacyScrollers`) they become **legacy
  scrollers** (always visible, with a track, the viewport inset), switched live.
* **Physics:** precise deltas scroll 1:1; on release momentum decays exponentially; past
  the edges the offset is rubber-banded (`offset = limit + d·c/(d+c)`) and springs back.
  Wheel mice scroll in line steps with a short animation. A wheel step or a press on a
  scroller stops momentum. The view that takes a gesture's start stays latched until it
  ends. Reduce Motion: no rubber band, no momentum, steps jump.
* **Keyboard:** Page Up/Down (viewport minus a small overlap), Home/End, Space/Shift-Space
  when the focus is not a control; the arrows only when the scroll view has the focus. A
  key that cannot move bubbles to an enclosing scroll view. Focusing an element scrolls it
  (with its focus ring) into view by the least amount, innermost view first.
* **Rendering:** clip and translate on the GPU, paint only visible elements.

**TextView (multi-line)**

* Shares the editing core with TextField (`internal/aqua/text`): piece-table buffer,
  grapheme-aware moves (`rivo/uniseg`), selection, undo with typing coalescing, the macOS
  keybindings plus paragraph moves (Option-↑/↓) and Primary-↑/↓ to document start/end.
* **One layout per paragraph**, cached and rebuilt only when the paragraph or the wrap
  width changes; only visible paragraphs are laid out and painted, so a 1 MB document
  stays smooth.
* IME, clipboard, drag-selection with auto-scroll, caret blink, caret-into-view.
* v0.1 is **plain text only**, one font and color per view. Rich text, find/replace,
  spell checking and line numbers come later.
* macOS: `NSTextView` inside an `NSScrollView` (`scrollableTextView`).

---

## 6. Native Bindings (macOS driver and Linux shell)

### 6.1 Calling AppKit without cgo (decision D5) — for phase 1

* AppKit is reached through the Objective-C runtime with **`ebitengine/purego` and
  `purego/objc`**: `purego.Dlopen` loads AppKit, `objc.ID.Send(sel, args…)` wraps
  `objc_msgSend`. No `.m` files, no C compiler.
* **Subclasses and delegates** (app delegate, window delegate, button target, text field
  delegate, flipped container view) are registered at start-up with `objc.RegisterClass`
  (inside `internal/ffi`). Each instance stores a `uintptr` handle in an ivar; the method
  looks the Go peer up in `ffi.Handles` (§7.2).
* **Struct arguments and returns** (`NSRect`, `NSSize`, `NSPoint`) go through purego's
  struct support, with layout tests on amd64 and arm64 and an explicit wrapper where that
  isn't enough (`objc_msgSend_stret` on amd64).
* **No Objective-C blocks:** delegate-style sheet APIs or `runModal`, and
  `performSelectorOnMainThread:withObject:waitUntilDone:` for UI-thread dispatch.
* **Autorelease pools** around each event-loop turn and each Go callback.
* **Typed wrappers** in `internal/appkit/ns` (`ns.Window`, `ns.Button` …), selectors cached
  as package variables; app code never sees `objc.ID`.
* `spike/appkit` is the seed of S1.1 and is deleted by it.

### 6.2 Other macOS details

* Containers are **flipped `NSView`s**; the Go layout engine sets their frames; intrinsic
  sizes come from `fittingSize` / `intrinsicContentSize`.
* A standard **Edit menu** is always installed (without it ⌘C/⌘V/⌘X/⌘A don't work in text
  fields). The app menu gets About/Hide/Quit automatically.
* Unbundled apps call `setActivationPolicy:NSApplicationActivationPolicyRegular` and
  `activate` to get a Dock icon and menu bar.
* Later: `GOGUI_DRIVER=aqua` on macOS (aqua on a Metal/Core Graphics shell) to compare
  native and drawn controls side by side.

### 6.3 Linux without cgo (decision D5)

* GTK 4, GObject, GLib, Graphene, Pango (and pangocairo, pangoft2, cairo, fontconfig for
  text and the app font) are opened by soname at run time and bound into tables with
  `ffi.Bind`. No variadic C function is called. GTK < 4.14 and Pango < 1.52 are refused
  (D7); newer functions are looked up and used when present.
* If a library or symbol is missing, `Run` returns an error naming the soname and the
  package to install, also shown through `zenity` or `kdialog` when on `PATH`.
* **Callbacks:** a fixed set of trampolines, one per C signature, registered once per
  process; signals and sources carry an `ffi.Handle` in `user_data`. Every trampoline
  recovers a panic and re-raises it after the native call returns. `ffi.Bind` rejects Go
  func arguments (purego would make a new callback per call).
* **Custom widget class** `GGSurface`, registered after checking `sizeof(GtkWidgetClass)`,
  so the vfunc slots cannot be wrong silently; the instance holds the peer's handle.
* `gtk_init_check` plus a `GMainLoop` (no `GtkApplication`); `gogui.Do` wakes it with
  `g_idle_add`; frames come from the `GdkFrameClock`.
* Mirrored C structs use `structs.HostLayout`, have layout tests and are passed by pointer
  only.
* D-Bus (portal settings, later the KDE global menu) uses pure-Go `godbus/dbus/v5`.

---

## 7. Cross-Cutting Concerns

### 7.1 Threading and the event loop

* `gogui`'s `init()` locks the main goroutine to the OS main thread.
* `gogui.Do` pushes onto a lock-free queue (`internal/mainthread`) and wakes the native
  loop; wake-ups are coalesced, the driver drains the queue on the UI thread.
* No UI calls from finalizers.

### 7.2 Memory and FFI safety (no cgo)

* Go pointers are never stored in native memory. Native objects carry an **integer
  handle** into `internal/ffi.Handles`, touched only on the UI thread and freed in
  `Destroy()`.
* Buffers native code keeps beyond one call are pinned (`runtime.Pinner`) or allocated in
  native memory.
* **Callbacks are a limited resource** (`purego.NewCallback` / `syscall.NewCallback`
  cannot be freed): a **fixed set of trampolines** at start-up, one per signature,
  dispatching through the handle in `user_data`, the ivar or `GWLP_USERDATA`. Never one
  per widget. The budget (`ffi.MaxCallbacks`) is 1024; `check.sh` fails if code outside
  `internal/ffi` creates a callback.
* C strings through `ffi.CString` / `GoString` with explicit freeing; NSString through
  `stringWithUTF8String:` in an autorelease pool.
* Native memory is read and written through `ffi.ReadUintptr` / `WriteUintptr` /
  `ffi.At[T]`, so drivers need no `uintptr` → `unsafe.Pointer` conversion; any such
  conversion outside `internal/ffi` needs a justified `//nolint`.
* COM objects are released explicitly; tests track reference counts.

### 7.3 Appearance changes

Light/dark, accent, increased contrast, Reduce Motion, Reduce Transparency, legacy
scrollers and the caret blink are observed live (`effectiveAppearance` KVO,
`WM_SETTINGCHANGE` and friends, the portal's `SettingChanged` plus GtkSettings) and
reach every open window, sheet, alert and popup without a restart. `OnAppearanceChange`
reports light/dark only.

### 7.4 Localization and text

UTF-8 throughout. Right-to-left layout is **out of scope for v1**; the layout engine lays
out left to right only.

### 7.5 Accessibility (decision D4: after v0.1)

* The aqua element tree exposes roles, labels, values, states and actions; unit tests
  check every v0.1 control fills its node. On macOS VoiceOver works through the native
  controls.
* The Windows/Linux bridge ships in phase 7. Options under the no-cgo rule: (1) a
  pure-Go UI Automation provider plus AT-SPI over pure-Go D-Bus, or (2) AccessKit's C API
  loaded through purego (ships a `.dll`/`.so`). Chosen by a spike at the start of phase 7;
  option 1 is preferred at similar effort (single static binary).

---

## 8. Testing and Quality Strategy

What the tests pin down per package: ARCHITECTURE.md §10.

### 8.1 Unit tests (all platforms, no display)

Layout (including every example's frames at default, minimum and larger sizes), aqua
widgets over the **fake shell** and a **recording canvas** (state machines, focus, keys,
text editing, pinned `.states`/`.frames` dumps), scroll physics on a fake clock,
`internal/ffi` struct layouts on amd64 and arm64 (on the target OS through `native.sh`),
and performance guards (idle audit, allocations per frame, pacing).

### 8.2 Golden-image tests

The aqua `gallery` scene renders offscreen at 1× and 2×, light and dark (plus inactive and
increased-contrast variants, on Windows only until S4.3j's follow-up) and is compared with
committed PNGs. The PNGs are written on Windows only (WARP) and shared with Linux, which
compares under its own tolerance with the text boxes masked (calibrated in S4.3j) and
never rewrites them.

### 8.3 Parity tests (macOS reference)

* On macOS the same gallery renders natively (`cacheDisplayInRect:`) to produce the
  **reference images** the tokens are tuned against (S1.5, S3.1t).
* `examples/controls` is the manual parity check on all three OSes.
* **The parity checklist** is [`docs/parity.md`](docs/parity.md): the v0.1 scope and the
  §5.5 behaviours against five targets, and every open check by ID (M macOS, W Windows,
  L Linux, A any OS, G gaps, C closed). The user signs it off by filling it in;
  `TestParityChecklist` keeps it consistent.

### 8.4 Native runs (no CI)

* **No CI and no GitHub workflows (user rule 2026-10-13).** Everything runs with
  `CGO_ENABLED=0`:
  * **Windows host, by Claude:** `bash scripts/check.sh` (all per-step checks, §11.1) and
    `bash scripts/native.sh` for windows/amd64.
  * **User's machines:** `bash scripts/native.sh` runs what needs the target OS (vet,
    tests, example builds, callback measurements, struct layouts, the appkit spike on
    macOS; on Linux the gtk4 tests, every example and the golden comparison under xvfb/X11
    and headless weston/Wayland, plus the desktop session; performance and idle CPU on
    every OS) and writes `native-logs/<goos>-<goarch>.log` (not committed; on Linux also
    `native-logs/golden-*/`), which the user brings back.
* **Race detector:** allowed as an extra cgo test run (D5) on a machine with a C compiler;
  not scripted. Concurrency code is stress-tested (`-count=20 -cpu=1,2,4,8`) either way.

---

## 9. Build and Distribution Notes

**No cgo on any platform (D5).** Only the Go toolchain is needed, and any OS builds for any
other (`GOOS=… GOARCH=… CGO_ENABLED=0 go build ./examples/hello`). Runtime needs: Windows 10
1809+; macOS 13+ (AppKit loaded at run time; signing needs Apple tools); Linux with GTK ≥
4.14 (`libgtk-4.so.1`, loaded with `dlopen`).

* The library never imports `"C"` (tests may run with cgo in addition, D5); `check.sh`
  enforces it with `go list -f '{{.CgoFiles}}'` plus a source scan.
* Dependencies stay small and pure Go: `ebitengine/purego`, `golang.org/x/sys`,
  `godbus/dbus/v5`, `rivo/uniseg`. On Windows purego is used only for COM methods with
  float arguments (`syscall.SyscallN` passes no floats on windows/arm64).
* A new dependency gets a row in `THIRD_PARTY.md` in the step that adds it (D12).
* **Later:** a `cmd/gogui` packaging helper (`.app` + `Info.plist` on macOS, manifest +
  icon on Windows, `.desktop` + AppImage/Flatpak recipes on Linux).

---

## 10. Repository Layout

See ARCHITECTURE.md §1 for the packages. Top level: `PLAN.md`, `ARCHITECTURE.md`,
`CLAUDE.md`, `README.md`, `LICENSE`, `THIRD_PARTY.md`, the `gogui` package files,
`internal/` (driver, layout, mainthread, ffi, aqua, render, shell, and the test-only
repocheck), `assets/fonts/` (Inter), `docs/` (parity checklist, spikes, tokens, review
plan, images), `examples/` (hello, controls, form, converter, notes, gallery), `scripts/`
(check, native, idlecpu), `spike/appkit`, `testdata/`.

---

## 11. Roadmap

### v0.1 scope ("basic controls/windows")

**In:** App lifecycle, `Window` (titled, closable, miniaturizable, resizable, min size,
center, key/inactive state), `Label`, `Button` (push, default, cancel), `Checkbox`,
`RadioGroup`, `TextField`, `SecureField`, `Slider`, `ProgressBar` (determinate and
indeterminate), `PopUpButton`, `Separator`, `Box`, **`ScrollView`**, **`TextView`
(multi-line, plain text)**, `VStack` / `HStack` / `Spacer` / `Form`, a `MenuBar` (App,
Edit and custom menus; in the title bar on Windows/Linux), a modal `Alert`, light/dark
mode, accent color and HiDPI. No cgo on any platform.

**Out (later):** TableView/OutlineView, rich text, Toolbar, Sidebar/SplitView, TabView,
drag and drop, file dialogs API, tray/status items, notifications, RTL, the accessibility
bridge on Windows/Linux, the Liquid Glass theme, packaging CLI.

### Phases

Phases 2–3 ran **before** phase 1 (no Mac, user decision 2026-10-13). The exits and what
they wait on are tracked in [`docs/parity.md`](docs/parity.md) §5.

| # | Phase | Exit criterion | Open |
|---|---|---|---|
| 0 | Foundations | `go test` green; cross-compile matrix green; API reviewed | ✅ |
| 0.5 | purego spikes | All three spikes work; R-purego (§13) re-rated | S0.10c (macOS/Linux logs) |
| 1 | macOS native driver | `hello`, `controls`, `form`, `notes` run natively on macOS; reference screenshots captured | S1.1–S1.5 (a Mac) |
| 2 | Render + Windows shell | Aqua window with traffic lights, drag/resize/snap, crisp text at 100/150/200 % | ✅ |
| 3 | Aqua widgets I | Gallery on Windows passes visual review against macOS references; goldens; scroll physics tests | S3.1t (after S1.5) |
| 4 | Linux shell | Same gallery renders on Linux (Wayland + X11) identically to Windows (shared goldens) | S4.3j (Linux log) |
| 5 | Aqua widgets II | `form` fully usable by keyboard only; CJK IME works; `notes` edits a 1 MB file smoothly | User checks (parity.md) |
| 6 | Polish and v0.1 | Parity checklist signed off (no release, no tag) | User checks (parity.md) |
| R | Review fixes | The items of `docs/review-plan.md`, each with a reproducing test | All (**next**) |
| 7 | Post-v0.1 | Accessibility bridge (D4), TableView, Toolbar, Sidebar with vibrancy, file dialogs, `GOGUI_DRIVER=aqua` on macOS, Liquid Glass, packaging CLI | Planned at its start, after the a11y spike (§7.5) |

### 11.1 Implementation steps

Each step is one unit of work for Claude, who writes the code and runs the checks on the
Windows host. The user runs only what needs another OS or a person at the screen. **After
every step the repo is consistent and buildable.** When a step is done it is deleted from
this file (ARCHITECTURE.md and the commit record what it built); its open user checks go
into `docs/parity.md` §4 by ID.

#### How every step stays buildable

1. **Full API early.** The complete public API runs on a headless driver, so all five
   examples compile on every platform after every step.
2. **Stub peers.** `internal/driver/stub` has a no-op peer per interface; the headless
   driver uses them and a native driver embeds them as fallbacks for controls it lacks.
3. **Fixed driver-selection files.** Build tags in `gogui` only in `driver_darwin.go`
   (headless until S1.1) and `driver_other.go` (aqua over `native.New()` from
   `internal/shell/native`: Win32 on Windows, GTK 4 on Linux, `ErrNotImplemented`
   elsewhere).
4. **Dependencies arrive with their first import** (`go get`/`go mod tidy` through the
   module proxy), with `go.mod`/`go.sum` and the module's row in `THIRD_PARTY.md` in that
   step.
5. **Spikes stay isolated** in `spike/<name>/`, every file with an OS build tag, and are
   deleted by the step that replaces them.
6. **Examples follow the API:** a public-API change updates §4, the examples and
   `examples/README.md` in the same step.
7. **Generated code is committed** (theme tokens); a build never needs `go generate`.

#### Checks after every step (`bash scripts/check.sh`)

`check.sh` stops at the first failure; `-quick` skips the stress run; a step is done only
after a full run with fresh tests (`-count=1`).

* `gofmt -l .` prints nothing; `go vet ./...` and `go build ./...` pass with
  `CGO_ENABLED=0` for `{darwin,windows,linux} × {amd64,arm64}`; staticcheck per GOOS.
* `go test ./...` passes on the host.
* No-cgo guard: `go list -f '{{.CgoFiles}}'` is empty, and a source scan fails on
  `import "C"` and on `.c .h .m .mm .cc .cpp .cxx .S .swig` files.
* Hidden-callback guard: outside `internal/ffi` and tests, no `syscall.NewCallback`,
  `NewCallbackCDecl`, `windows.NewCallback`, `purego.NewCallback`, `objc.NewIMP`,
  `objc.NewBlock` or `objc.RegisterClass`.
* Concurrency code passes the stress run (`-count=20 -cpu=1,2,4,8`; `check.sh` lists the
  patterns for `aqua` and `shell/win32`).
* cgo test runs (D5) are extra, never instead, and on the user's machines.
* The step's "Done when" items are met; the step's report says what was checked locally
  and what the user still has to run on macOS or Linux.

#### Phase 0.5: open runs

**S0.10c `docs/spikes.md`, R-purego re-rated, phase 0.5 exit.** Built and checked on the
Windows host; what is left waits on the user:
* **S0.10c_2b** (waits on logs): from the user's `native-logs/<goos>-<goarch>.log` (at
  least one Mac and one Linux; windows/arm64 optional; each with `go test`, the callback
  measurements, the appkit self-test with the `isMultiThreaded` line, and on Linux the
  gtk4 tests' `BACKEND-RESULT`, `RESIZE-RESULT` and frame-clock lines under xvfb and
  weston): fill the ○ cells of the spike table in `docs/spikes.md`, turn ◐ into ✔ and
  re-rate R-purego (expected High → Medium; the unix per-call cost decides whether the
  per-motion-event callbacks need a note). A failing run gets its own fix step first;
  fewer than `MaxCallbacks` + 64 free callback slots on a target means revising the plan.
* **S0.10c_3** (waits on the user): the visual checks on both Macs and the user's look at
  `examples/hello` on Wayland and X11; record them in `docs/spikes.md`, drop its "State"
  preamble and "Open runs", mark phase 0.5 done here and in ARCHITECTURE.md.
* Done when: `check.sh` passes and no ◐ or ○ is left in `docs/spikes.md`.

#### Phase 1: macOS native driver (waits on a Mac)

* *Prerequisite for:* S1.5's reference captures (`docs/parity/`), then S3.1t and the
  phase 3 visual review. S1.5 renders the gallery scene (`internal/aqua/gallery`) natively
  through `gogui`.
* *Not a prerequisite:* phase 0.5's open runs.
* Phase 1 may still change `internal/driver` contracts; such a change goes into the aqua
  driver in the same step.

**S1.1 AppKit core**
* `internal/appkit/ns` wrappers: NSApplication, NSWindow, NSView, NSString, NSTextField
  (label), autorelease pools.
* Registered classes: app delegate, window delegate, flipped view, target. UI-thread wake
  via `performSelectorOnMainThread:`. Sets the activation policy.
* The appkit driver embeds stubs for every other control. `driver_darwin.go` selects
  appkit. Adds NSRect/NSSize/NSPoint layout tests and deletes `spike/appkit`, which is
  kept until then as the seed of this step (rule 5).
* Done when: `examples/hello` opens a native window with its label on macOS.

**S1.2 Buttons and toggles.** Push button (default, cancel), Checkbox (incl. mixed),
RadioGroup, Separator, Box.

**S1.3 Text and value controls.** TextField, SecureField (delegate for change and submit),
Slider, ProgressBar, PopUpButton. Done when: `hello` and `converter` work on macOS.

**S1.4 Menus, alert and appearance.** Main menu (app menu, Edit menu always installed,
custom items with shortcuts via a target class and item tag → handle), NSAlert without
blocks, appearance observation (KVO), `SetAppearance`, the accent override as the
per-control tint (§4.4). Done when: `controls` and `form` work on macOS.

**S1.5 ScrollView, TextView and reference captures.** NSScrollView with a flipped document
view, `scrollableTextView`, `Focus`, and a gallery snapshot helper (`cacheDisplayInRect:`)
writing reference PNGs to `docs/parity/`. Done when: `notes` works and the screenshots are
committed. **Phase 1 exit.**

#### Phase 3: open step

**S3.1t Token tuning against the macOS references** (after S1.5)
* Measure the S1.5 captures at 1× and 2×, light and dark; replace the provisional values
  in `sequoia.json`, regenerate `tokens.go`, update the goldens, and drop the "to be
  measured" marks.
* Done when: no provisional token is left, and the goldens pass with the new values.
  **Phase 3 exit** (with the visual review, M7 in `docs/parity.md`).

#### Phase 4: open step

**S4.3j Calibrate the Linux golden tolerance** (after the user's Linux log)
* From the log's `GOLDEN-RESULT` and `GOLDEN-SWEEP` lines and the diff PNGs in
  `native-logs/golden-*/` (masked pixels blue, plain red, anti-aliased yellow): set
  `goldenOptions`, `goldenMaskText` and `goldenMaskMargin` in
  `internal/shell/gtk4/gallery_linux_test.go` (or drop the mask if the tolerance alone
  holds without hiding real regressions); fix real differences in the gtk4 canvas, not in
  the goldens. Also settle the gtk4 resize mode (`mappedResize`/`resizeSizeRequest`,
  review-plan §4).
* Done when: the gallery matches the Windows goldens under those options on both headless
  backends. **Phase 4 exit.**
* **Follow-up: one gallery regeneration** (after the calibration holds; new PNGs before it
  would invalidate the log it calibrates from). The gallery scene gains:
  * the TextField and SecureField (empty with a placeholder, with text, focused with a
    selection, disabled, secure);
  * the TextView (unfocused with wrapped text, focused with a selection, read-only,
    disabled; overlay scroller hidden), from `aqua/testdata/textview.states`;
  * the PopUpButton, closed (plain, focused, disabled, no selection, small), from
    `aqua/testdata/popup.states`; open menus stay covered by `menu.states`;
  * a title bar with the menu strip (app menu bold, a window title; key with one title
    highlighted, and inactive; light and dark), from `menustrip.states`;
  * the alert panel drawn on its own as a sheet's, with its shadow (two buttons side by
    side with the informational icon, three stacked, the critical icon with its badge; key
    and inactive; light and dark), from `alert.states`;
  * on Linux, `gtk4/gallery_linux_test.go` also renders `goldentest.PolishVariants` (the
    inactive and increased-contrast galleries), against the same shared PNGs.
  
  The shared PNGs are regenerated on Windows (`TestGalleryDump`, `TestGalleryGolden`,
  ARCHITECTURE.md §9.4), and the next `native.sh` on Linux re-checks them (L18 in
  `docs/parity.md`).

#### Phase R: review fixes (next)

[`docs/review-plan.md`](docs/review-plan.md) (2026-11-20) lists bugs (B1–B17), fake-shell
divergences (V1–V8), performance items (P1–P19), refactors (R1–R14), breaking API changes
(A1–A9), dead code and docs/script hygiene (D1–D10); it now keeps only the open items, with the remaining order in its §6.
Before the work starts, turn that order into steps R1… here. Rules: every item starts
with a failing test that reproduces it (no test, no item); A1 (widget lifetime) needs the
user's decision first and becomes D13; the API items follow rule 6. W1 (D1–D3, D8, D9, the doc
cleanup), W2 (D6, D7, the script hardening) and W3 (R6, V1/B9, V4, V8: shared timers,
clipboard sanitizing, clamp/hit-test and the fixed-width text in `shellkit`) and W4 (V2, V3,
V6, V7: quit-first and idempotent `Quit` with `ErrQuit`, modality left before `Closed`,
the fake's `SetDeferPlaced`, the stub honouring disabled ancestors) and W5 (B1, B12, B14, B17: a press whose handler
removes its element is cancelled, a popup closed inside `NewPopup` closes its shell popup,
disabled widgets keep their cached value, the title-bar hairline is pixel-aligned) and W6 (B2: one Win32 device-loss policy with a
re-entrancy guard, a retry in `NewSurface`/`NewPopup` and a WARP fallback after repeated losses) and W7 (B6, B7, B15,
B8 minus its Linux run: `Curve.Slope` limits at flat ends, the bounce cap from `anim.Motion.Peak`, the shared
`render.ValidRect`/`ValidPoint`/`ValidPath` rules in all three canvases, `ArcTo` and golden `Threshold` finite checks) and W9
(D13 and A1–A9 with B3/B4: explicit `Widget.Destroy`, a destroyed widget panics when used, and the
breaking API changes A1–A9) and W11 (R1–R4: one `rootContext` on the window, popup and alert
roots, the shared `clickControl` click core with the AppKit key flash, one `broadcastState`, and the
shared drawing primitives in `primitives.go`) and W12 (R5, B5/P5, B13, P1: the `textEdit` text element base with the
`key`/`hitAt`/`selected`/`autoScroll` hooks, `textview.go` split into layout, paint and input files, `Buffer.Replace` with
one report, `systemFont`, one type-select for the menus and the strip, Select All without a scroll, the add buffer
given back on backspace, long paragraphs laid out in chunks) and W13 (P8, P9, P10, P12, P13, P14, P15, P18: aqua perf, copy-on-write children, the
`treecache.go` tree context memo, a spinner repainting per spoke step, cached menu offsets, two-entry label layouts, one group
stop lookup per parent, an allocation-free unchanged hover, the secure field's cluster table) and W14 (P7, P19: one reused
layout `Pass` per root with memoised `Expands` and fitting sizes and scratch slices, so a warm pass allocates nothing;
`Stack.Add` appending incrementally and batched detaches) and W10 (R7: `Queue()` and `ObserveFrames` in `driver.Driver`, no `Post`,
`newPlatformDriver` returning `(Driver, error)`, no stub run loop, `gogui` on `driver` alone) and W15 (R11, R12, P11, P16:
per-value `anim` tolerances (springs settle once they cannot move a visible amount, sub-tolerance frames are not shown),
`Curve.AtSlope`, `scroll.Decay` and `theme.Animation.Duration` removed, float32 alpha tokens, `Op.Points` and
`render.SegmentsBounds`, `Normalized` sharing normal slices, per-element path memos and the recorder's arenas: gallery 61→21
allocations per steady frame) and W16 (R9, P2, P3, P4, P17: `com.Ptr.CallOut`, the direct `SyscallN` path for the per-draw D2D
and DirectWrite methods with FLOATs in the integer slots on amd64, the device resource cache with one canvas per device,
transforms sent only when changed, the font-family memo, an intrusive LRU for drawn layouts and an allocation-free message
loop: gallery 1907→195 allocations and 55→12 KB per frame) and W17 (R8: a `swapTarget` embedded by win32 surfaces and popups
instead of a popup's internal `surface`, the test hooks and counters in `testHooks`/`shellStats`/`clockStats`, and
`w32.go`, `canvas.go`, `surface.go` and `clock.go` split by topic) and W19 (R10, the fake and win32 half: `shell.Shell` with `Caps()` instead of
`PopupShadow`, the optional `PopupShell` instead of `ErrUnsupported`, `Surface.SetSizeLimits(min, max)` and the text
system and clock handed over once in `Handler.Ready(Services)`; gtk4 ported in the same step to stay buildable) and W20 (the shared and win32 dead code of
review §4 removed or moved into `export_test.go`; R14, D4, D5: the test-only `internal/repocheck` with the parity,
license and examples checks, `TestExampleTreesMatch` keeping the example trees of `examples_test.go` equal to
`examples/*/main.go`, the file, package and build-tag anchors of the documents and the §2 overview copy; D10: Paste
documented as always enabled) are done; L2, L3, L4, L7, L19, M2, M9 and M10 wait on the user's Linux and macOS
`native.sh` runs, and P2 on windows/arm64 on the user's Windows/arm64 log; next is L8.

---

## 12. Decisions

Settled; changing one needs an update to this section and every section it references.
No question is open.

| ID | Question | Decision | Consequences |
|---|---|---|---|
| D1 | Linux: shared aqua engine or GTK 4 widgets + macOS CSS theme? | **Shared drawn aqua engine** on a GTK 4 window (GSK GPU rendering). | Windows and Linux look and behave identically; text input/IME is built once. No GTK-theme fallback. |
| D2 | Theme target for aqua | **Sonoma/Sequoia first, Liquid Glass later** as a second token set plus shader effects (post-v0.1). | `render.Canvas` and tokens must allow a second theme without widget changes. Native macOS shows whatever the OS provides. |
| D3 | Menu bar on Windows/Linux | **macOS-style menu strip in the title-bar area**, next to the traffic lights (§5.4.1). | Title-bar hit-testing mixes `HTCAPTION` and menu regions; keyboard access via Alt/F10. A KDE global-menu export is possible later. |
| D4 | When do screen readers work on Windows/Linux? | **After v0.1.** The accessibility tree is designed and tested from day one; the bridge ships in phase 7. | v0.1 is not accessible on Windows/Linux (it is on macOS). The bridge technology is chosen by a spike (§7.5). |
| D5 | cgo? | **No cgo in shipped code.** macOS via `purego/objc`, Linux via purego-loaded GTK, Windows via `x/sys/windows` plus purego for COM calls with float arguments on windows/arm64 (`syscall.SyscallN` passes no floats there; on windows/amd64 they go as bits in the integer slots, which the runtime mirrors into XMM0–3). **For testing only**, runs with `CGO_ENABLED=1` (above all `go test -race`) are allowed in addition to the `CGO_ENABLED=0` checks, never instead. Test-only C code needs its own build tag, is never imported by non-test code, and the step that first adds it updates the no-cgo guard (§9, §11.1). | `CGO_ENABLED=0` builds and cross-compilation from any OS. More binding work: ObjC class registration, GObject subclassing, a fixed trampoline set, mirrored struct layouts (§6, §7.2). Races are caught by stress runs. |
| D6 | Module path | **`github.com/radix29/gogui`** | Used in `go.mod`, all examples and docs. |
| D7 | Minimum OS versions | **macOS 13+, Windows 10 1809+, GTK ≥ 4.14** (Ubuntu 24.04, Fedora 40+, Debian 13) with Pango ≥ 1.52. amd64 and arm64. | Windows 10 gets square corners and no Mica. Runtime feature detection. `GskPath` is required, no cairo fallback. |
| D8 | Extra controls in v0.1 | **ScrollView and TextView.** TabView and file dialogs stay post-v0.1. | `examples/notes`. |
| D9 | Bundled UI font on Windows/Linux | **Inter** (OFL), bundled in `assets/fonts`, with an SF-like tracking table. The platform UI font is only the fallback when loading fails. | One font on Windows and Linux; the drawn controls are tuned against Inter (S3.1t). |
| D10 | Accent color on Windows/Linux | **Follow the OS accent** (Windows `DWM\AccentColor`, the portal `accent-color`), macOS blue when the OS has none. Accent-derived colors are computed by the theme, never stored. **App override** `App.SetAccentColor(c)` / `App.AccentColor()` (§4.2); on macOS the per-control tint (§4.4). A light accent turns the labels on it dark. | An inactive window derives no color from the accent; changes reach every open surface live. |
| D11 | Multi-window apps: menu strip in every window? | **Every titled window has the strip**, all from the one bar; no "main window" API in v0.1. Accelerators work in every key window. An app that never calls `SetMenuBar` shows no strip on Windows/Linux; `SetMenuBar(nil)` means the default App and Edit menus. | `Driver.SetMenuBar` rebuilds the strip of every titled window. |
| D12 | License of gogui | **MIT**, copyright Radu Colceriu, `LICENSE` at the root. Compatible with purego (Apache-2.0), godbus (BSD-2-Clause), uniseg (MIT), `x/sys` (BSD-3-Clause) and Inter (OFL 1.1). | `THIRD_PARTY.md` records each dependency; `TestThirdPartyNotices` fails until a new `require` has a row. |
| D13 | Widget lifetime (review-plan A1) | **Explicit `Destroy` only.** Replacing or removing content never destroys it (panels can be reused); `Widget.Destroy()` frees a widget and its subtree; closing a window and the end of `Run` destroy the rest. **A destroyed widget panics when used**; getters return the last values. | Apps that swap panels call `Destroy` on the old ones, or they stay alive until `Run` returns. B3 (re-parenting dead widgets) is a panic. §4.2 Lifetime, the package doc's Lifetime section, ARCHITECTURE §1.1. |

---

## 13. Key Risks

| Risk | Impact | Mitigation |
|---|---|---|
| **R-purego:** no-cgo bindings for AppKit and GTK (class registration, vfuncs, struct passing, the limited callback count) have rough edges | High (re-rating pending, S0.10c) | Spikes first; fixed trampolines, handle dispatch, struct layout tests on amd64 and arm64; track purego releases. |
| "Uncanny valley": almost a Mac, but slightly off | High | Tokens tuned against measured references (S1.5, S3.1t), goldens, the §5.5 checklist. |
| Text input/IME in drawn text fields, single- and multi-line | High | Shaping and hit-testing by DirectWrite/Pango, one editing core, IMM32 first; the native CJK checks are the user's (parity.md). |
| Scrolling feel differs from macOS | Medium | Physics constants tuned against native `NSScrollView` recordings (after phase 1); fake-clock tests; Reduce Motion path. |
| The gtk4 shell is built and host-tested only | Medium | Logic behind small interfaces and host-tested; linux tests through `native.sh`; S4.3j calibrates from the first log. |
| Smooth live resize and device loss on Windows | Medium | DirectComposition flip-model chains resized in `WM_SIZE`; one device-loss policy (review-plan B2). |
| GTK 4 missing or too old on the user's system | Medium | Version check at start-up with an error naming the package; packaging recipes (Flatpak bundles GTK). |
| Linux compositor differences (no blur on GNOME, X11 vs Wayland) | Low | Solid translucent fallbacks, feature detection. |
| Font/asset licensing | Legal | Inter only, procedural drawing, no Apple assets (§3.1). |
| No accessibility on Windows/Linux in v0.1 (D4) | Medium | The a11y tree is tested from day one; the bridge is the first post-v0.1 item. |
