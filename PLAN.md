# gogui: Plan for a Cross-Platform Go GUI Toolkit with a macOS Look and Feel

> **Goal:** a Go library for desktop apps on **macOS, Windows and Linux** from **one
> portable codebase**. On macOS the apps use **real AppKit controls**. On Windows and Linux
> they **look and behave like macOS apps**: the host OS provides windows, text, input, IME,
> dialogs, clipboard and accessibility wherever possible, and everything custom-drawn is
> drawn on the **GPU**.

Status (2026-11-04): phases 0 and 0.5 are built and checked on the Windows host (the macOS
and Linux runs of phase 0.5 are still open, S0.10c). Phases 2–3 run **before** phase 1
(no Mac, user decision 2026-10-13). Done: S1.0, S2.1, S2.2, S2.3, S2.4 (S2.4a–S2.4f), S2.5 (S2.5a–S2.5g); **phase 2 is done**, its exit (Inter added, the visual check at 100/150/200 % and across monitors) confirmed by the user on 2026-10-22. Phase 3: S3.1 (S3.1a–S3.1d; O1 and O2 decided as D9 and D10), S3.2 (S3.2a–S3.2f), S3.3 (S3.3a–S3.3h; the visual checks of `examples/hello`, `controls` and `form` on Windows are open, user), S3.4 (S3.4a–S3.4g; the visual check of `converter` and `controls` on Windows, slider and progress indicators included, is open, user), S3.5 (S3.5a–S3.5h; the visual check of `notes` on Windows is open, user), S3.6 (S3.6a–S3.6e; the goldens pass on the provisional tokens; the review against macOS waits on S1.5). Phase 4: S4.1a (`shell/shellkit`), S4.1b (`shell/gtk4/gtk`), S4.1c (`shell/gtk4` trampolines and the GGSurface class), S4.1d (the gtk4 `Shell` and its selection on Linux), S4.1e (the gtk4 frame clock), S4.1f (the gtk4 surfaces: CSD window, sizes, lifecycle), S4.1g (the interim snapshot canvas), S4.1h (the gtk4 chrome regions: title-bar move and double-click zoom), S4.1i (gtk4 pointer input), S4.1j (gtk4 scroll input and momentum), S4.1k (gtk4 keyboard input), S4.1l (wrap-up: `spike/gtk` deleted, `native.sh` runs the gtk4 tests and `examples/hello` on X11 and Wayland); **S4.1 is built**, S4.1b–S4.1l built only: their linux tests and the S4.1 "Done when" (`hello` on Wayland and X11) wait on a Linux log and the user. S4.2a (GTK floor raised to 4.14, D7 amended; built only), S4.2b (the canvas leaves interim: `canvas*.go`, real gradient strokes through a `GskPath` stroke push; built only), S4.2c (`FillPath`/`StrokePath` over `GskPath`: fills, strokes with caps, joins, miter limit and dashes, gradient paints; built only), S4.2d (`DrawShadow` as outset and inset shadow nodes; built only), S4.2e (`DrawImage` as scaled texture nodes of the shell's GdkTextures; built only), S4.2f (`PushBackdropBlur` by segments: child snapshots re-appended under a blur push, edge mirrors for hard borders; built only), S4.2g (the Pango text system: layout and metrics, unhinted, `interim_text.go` gone; built only), S4.2h (hit-testing, carets and selection from Pango's geometry, read in linear time; built only), S4.2i (drawing text: a native `PangoLayout` per layout and paint through `gtk_snapshot_append_layout`, the text placeholder gone; built only), S4.2j (Inter as an app font: written once to the user cache, added through `pango_font_map_add_font_file` or fontconfig, the system UI font on any failure; built only), S4.2k (wrap-up: no `interim` names left, `native.sh` runs every example on both backends, ARCHITECTURE.md on the canvas and text system); **S4.2 is built**, S4.2a–S4.2k built only: their linux tests and the S4.2 "Done when" (`hello`, `controls`, `form`, `notes` in Inter on Wayland and X11) wait on a Linux log and the user. S4.3a (the appearance mapping: `readAppearance` over the portal keys with GtkSettings fallbacks, pure Go, fully checked on the host), S4.3b (the portal client over `godbus/dbus/v5` v5.2.4: ReadAll with the ReadOne/Read fallbacks, the 250 ms bound, `SettingChanged`; host-tested end to end against an in-process fake bus; built only: its linux tests on a real session bus wait on a Linux log), S4.3c (the shell wiring: the appearance cached before `Ready` from an injectable source, re-read on the portal's changes through `Shell.Post`, deduped, the source closed in `shutdown`; host-tested), S4.3d (the live GtkSettings: `notify::` of the three properties on the default GtkSettings through `sigPointer`, re-read through `Post`, disconnected at shutdown; `native.sh` runs the gtk4 tests under `dbus-run-session`; built only: its linux test waits on a Linux log), S4.3e (the shared golden harness `aqua/gallery/goldentest`: the variants, premultiplied BGRA → NRGBA, the compare-and-report `Check`; `TestGalleryGolden` on Windows unchanged at max delta 0; fully checked on the host).
**Next: S4.3f** (phase 4: the Linux render seams and readback: the fake appearance source, a scale seam for 2×, a key surface without input, the readback through `gsk_renderer_render_texture` and `gdk_texture_download`; built only; then S4.3g, the Linux gallery golden test over `goldentest`; S4.3 is portal settings (dark mode, accent) and Linux goldens shared with Windows, the phase 4 exit; split into S4.3a–S4.3i on 2026-11-04, S4.3i needs the user's Linux log; the user's Linux log and look at the S4.1/S4.2 examples are still open; no Linux machine here; phase 3 is built, its exit waits on S1.5 and S3.1t, and the token values stay provisional while phase 1 waits). Waiting on a Mac: S1.1–S1.5, S3.1t
and the phase 3 exit. The ✅
marks in §11.1 are authoritative. Decisions: §12. Module path: `github.com/radix29/gogui`.
Finished steps with their Results and findings (S0.1–S0.10b, S1.0, S2.1–S2.5, S3.1–S3.6, S4.1, S4.2, S4.3a–S4.3e) and the Q&A log live in [`PLAN_done.md`](PLAN_done.md); this file keeps what is open or ahead.

### Decision summary (Q&A of 2026-09-30, details in §12)

| Topic | Decision |
|---|---|
| Linux controls | The **shared drawn "aqua" engine** (same as Windows) on a GTK 4 window with GSK GPU rendering |
| macOS look on Windows/Linux | **Sonoma/Sequoia first.** Liquid Glass (macOS 26+) later as a second theme |
| Menu bar on Windows/Linux | **macOS-style menu strip in the title-bar area**, next to the traffic lights |
| cgo | **None in shipped code.** `purego` on macOS and Linux, `x/sys/windows` on Windows (plus purego for COM calls with float arguments, S2.4a). `CGO_ENABLED=0` everywhere. cgo only for test runs such as `go test -race` (2026-10-12) |
| Accessibility | Tree designed from day one; the screen-reader bridge ships **after v0.1** |
| Minimum OS | macOS 13+, Windows 10 1809+, GTK ≥ 4.14 (raised from 4.10 on 2026-11-02) |
| Extra v0.1 controls | **ScrollView** and **TextView** (multi-line) |
| UI font on Windows/Linux | **Inter**, bundled (2026-10-23) |
| Accent on Windows/Linux | **Follows the OS**; an app-level override comes in phase 6 (2026-10-23) |

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

### 2.2 Why this split?

* **Native Win32 / GTK controls with a macOS theme don't work.** Win32 common controls
  can't be themed like AppKit (owner-draw leaves Windows behaviour: focus, hover, popups,
  animation), and WinUI 3 needs WinRT/XAML and packaging. A GTK 4 CSS theme (WhiteSur)
  *is* feasible but was **rejected** (D1): two independent "fake Aqua" implementations
  drift apart. The **shared aqua engine** implements the look once on a thin native shell,
  so Windows and Linux look identical and fixes land on both.
* **No single portable renderer (OpenGL/WebGPU/Skia).** DirectWrite and Pango/HarfBuzz
  give correct shaping, fallback, bidi, emoji, hinting and hit-testing for free; Direct2D
  and GSK are GPU-accelerated and match what AppKit controls need; no Skia/wgpu binaries,
  and the Windows build stays pure Go. `render.Canvas` keeps a portable backend possible.
* **No WebView:** not native, memory-hungry, slow to start, hard to make feel like AppKit.

### 2.3 Prior art (learn from, don't depend on)

**Gio** (pure-Go Win32/Wayland/X11 and GPU; reference for Win32 without cgo; immediate-mode,
own look), **Fyne** (portable, own look), **andlabs/ui** (the peer/driver design; unmaintained),
**ebitengine/purego** (the core dependency), **AccessKit** (cross-platform a11y for drawn
UIs; a candidate for the post-v0.1 bridge, §7.5), **WhiteSur** (visual reference for Linux).

---

## 3. Technology Stack

| Concern | macOS | Windows | Linux |
|---|---|---|---|
| Binding technique | `purego/objc`: `objc_msgSend`, `objc.RegisterClass` for delegates/targets/flipped views; frameworks loaded with `purego.Dlopen` | Pure Go: `golang.org/x/sys/windows`, `syscall.SyscallN` on COM vtables; `purego.RegisterFunc` on the vtable slot for methods with float arguments (S2.4a) | `purego.Dlopen("libgtk-4.so.1")` + `RegisterLibFunc`; GObject signals via callback trampolines |
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

**Minimum OS targets (v1, confirmed D7):** macOS 13+ (Intel and Apple Silicon), Windows
10 1809+ (Mica, rounded corners and Snap Layouts only on Windows 11; Windows 10 gets a
solid background and square corners), and Linux with GTK ≥ 4.14 (Ubuntu 24.04,
Fedora 40+, Debian 13; raised from 4.10 on 2026-11-02 for `GskPath`, S4.2a) on Wayland
or X11. Architectures: amd64 and arm64 on all three.

### 3.1 Licensing and legal constraints

* **SF Pro and SF Symbols** are licensed for Apple platforms only: never bundle them.
  Use **Inter** (OFL, metrically close) with an SF-like tracking table.
* **Never ship Apple bitmaps or assets.** Every aqua control (traffic lights, focus rings, …)
  is drawn procedurally from our own measurements and scales to any DPI.

---

## 4. Public API Design (v1 sketch)

### 4.1 Hello world

```go
package main

import (
	"log"

	"github.com/radix29/gogui"
)

func main() {
	if err := gogui.Run(gogui.AppConfig{Name: "Hello"}, func(app *gogui.App) {
		greeting := gogui.NewLabel("Hello!").SetFont(gogui.BoldSystemFont(15))
		name := gogui.NewTextField().SetPlaceholder("Your name")

		greet := gogui.NewButton("Greet").SetDefault(true) // Return key triggers it
		greet.OnClick(func() {
			greeting.SetText("Hello, " + name.Text() + "!")
		})

		win := app.NewWindow(gogui.WindowConfig{
			Title:  "Hello",
			Size:   gogui.Size{W: 360, H: 180},
			Style:  gogui.Titled | gogui.Closable | gogui.Miniaturizable | gogui.Resizable,
			Center: true,
		})
		win.SetContent(
			gogui.VStack(12,
				greeting,
				name,
				gogui.HStack(8, gogui.Spacer(), greet),
			).Padding(20),
		)
		win.OnClose(app.Quit) // closing the window ends Run
		win.Show()
	}); err != nil {
		log.Fatal(err) // for example, GTK 4 missing on Linux
	}
}
```

### 4.2 API conventions

* **Threading:** `gogui.Run` must be called once, from `main`; the main OS thread is
  locked in `init` (`runtime.LockOSThread`), and `Run` blocks until `Quit`. It returns
  an error if the backend can't start or if it is called twice, and library code never
  calls `os.Exit`. All callbacks run on the UI thread. Goroutines use `gogui.Do(func())`
  (async, in order) or `gogui.DoSync(func()) bool` (waits; inline on the UI thread) to
  touch widgets. Outside `Run` there is no queue, so `Do` drops fn and `DoSync` returns
  false; `DoSync` from another goroutine also returns false after `Quit`. Constructors
  and `Alert.Show` always panic outside `Run` (before the ready callback or after `Run`
  returns). With `GOGUI_DEBUG=1`, every public method also panics off the UI thread;
  without it the check costs one atomic load.
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
* **Lifetime:** widgets belong to their window and are destroyed with it, bottom-up.
  Widgets that were never placed in a window (or were taken out of one) are destroyed
  when `Run` returns. On a destroyed widget, setters only record and getters return the
  last value. The toolkit never relies on finalizers. Native objects refer back to Go
  peers through an integer handle table (`internal/ffi.Handles`), never through Go
  pointers.
* **Value types are aliases** of the internal driver types (`gogui.Size`, `Point`,
  `Rect`, `Insets`, `Color`, `Font`, `KeyShortcut`, `WindowConfig`, `AppConfig` …), and
  their constants are `gogui` names (`gogui.Titled`, `gogui.StateOn`,
  `gogui.AlertWarning`). Apps never need an internal package. A few methods and fields on
  these types exist for the drivers (`KeyShortcut.Resolve`, `Font.EffectiveSize`,
  `Color.Semantic`); apps can ignore them.
* **Modifiers:** `gogui.KeyPrimary` means ⌘ on macOS and Ctrl on Windows/Linux.
  Standard shortcuts (`gogui.ShortcutCopy` …) resolve per platform.
* **Semantic colors and fonts** mirror NSColor/NSFont: `gogui.ColorLabel`,
  `gogui.ColorSecondaryLabel`, `gogui.ColorControlAccent`, `gogui.ColorWindowBackground`,
  `gogui.SystemFont(size)`, `gogui.BoldSystemFont(size)`. They resolve against the current
  appearance (light/dark) automatically.
* **Control sizes:** `gogui.ControlSizeMini`, `ControlSizeSmall`, `ControlSizeRegular`
  and `ControlSizeLarge` (NSControl.ControlSize).

### 4.3 Driver (peer) interfaces (internal)

Implemented in S0.1 (`internal/driver`). This is an excerpt; the package docs hold the
full contracts.

```go
package driver

type Driver interface {
	Name() string                            // "appkit", "aqua", "headless"
	Run(cfg AppConfig, h AppHandler) error   // blocks on the main thread; h.Ready() once running
	Quit()
	Post(fn func())                          // marshal to UI thread (any goroutine)
	Appearance() Appearance                  // dark mode, accent, reduce motion …
	SetAppearance(mode AppearanceMode)       // system / light / dark
	SetMenuBar(bar *MenuBarSpec)             // nil = default App + Edit menus
	RunAlert(parent Window, a AlertSpec, done func(result int))
	NewWindow(cfg WindowConfig, h WindowHandler) (Window, error)
	NewContainer() Container                 // flipped NSView; stacks and forms
	NewLabel() Label
	NewButton(h ButtonHandler) Button
	NewCheckbox(h ToggleHandler) Checkbox
	NewRadio(h ToggleHandler) Radio          // RadioGroup exclusivity lives in gogui
	NewTextField(secure bool, h TextFieldHandler) TextField
	NewSlider(h SliderHandler) Slider
	NewProgress() Progress
	NewPopUpButton(h PopUpHandler) PopUpButton
	NewSeparator() Separator
	NewBox() Box
	NewScrollView(h ScrollHandler) ScrollView  // hosts one document view
	NewTextView(h TextViewHandler) TextView    // multi-line, scrolls itself
}

type AppHandler interface {
	Ready()
	AppearanceChanged(a Appearance)
	QuitRequested() bool                     // Quit item, ⌘Q, logout
	MenuItemSelected(id int)                 // items carry integer IDs, not funcs
}

type Widget interface {
	SetFrame(r Rect)                         // set by the shared layout engine
	FittingSize(max Size) Size               // intrinsic size from native/aqua metrics
	SetEnabled(bool)
	SetHidden(bool)
	SetControlSize(ControlSize)
	SetTooltip(string)
	Focus()
	Destroy()
}

type Button interface {
	Widget
	SetTitle(string)
	SetDefault(bool)                         // pulsing/accent default button, Return key
	SetCancel(bool)                          // Escape key
}
type ButtonHandler interface{ Clicked() }
// … one interface (and one handler) per control
```

Contracts: setters never call handlers (handlers report user/OS changes only); peers have
getters only for user-changeable state (`Text`, `Value`, `State`, `Selected`, `Offset` …);
`TextView` selection offsets are UTF-8 byte offsets. `internal/driver/stub` implements
every interface, records the last-set state and offers `User…` methods that simulate input.

### 4.4 AppKit mapping (macOS) ↔ aqua equivalents (Windows/Linux)

| gogui | macOS (native) | aqua (drawn) notes |
|---|---|---|
| `Window` | `NSWindow` | Custom title bar with traffic lights, centered title, DWM/GTK shadow, inactive-state dimming |
| `Label` | `NSTextField labelWithString:` | Text layout, selectable option |
| `Button` | `NSButton` (`.push` bezel) | Gradient bezel, accent fill when default, pressed state, focus ring |
| `Checkbox` | `NSButton` (`.switch`) | 14 pt rounded box, animated checkmark, mixed state |
| `RadioGroup` (private radios) | `NSButton` (`.radio`) | Accent-filled circle with a white dot. The group handles exclusivity and arrow keys (aqua: no arrow keys in v0.1, S3.3g). |
| `TextField` (`NewTextField` / `NewSecureField`) | `NSTextField` / `NSSecureTextField` | Go text model, native shaping, IME, clipboard, undo, macOS editing keys |
| `Slider` | `NSSlider` | Track with accent fill, knob with shadow, optional tick marks |
| `ProgressBar` / `Spinner` | `NSProgressIndicator` | Determinate bar, indeterminate animation, spinning indicator |
| `PopUpButton` | `NSPopUpButton` | Menu opens **over the selected item** (true Mac behaviour) in a native popup surface |
| `Separator` | `NSBox` (`.separator`) | 1 px hairline, alpha-blended |
| `Box` | `NSBox` (`.primary`) | Rounded group background with title |
| `ScrollView` | `NSScrollView` + flipped document view | Clip + offset; overlay scrollers that fade in while scrolling and grow on hover; momentum, rubber-banding at edges; Page Up/Down, Home/End, space-bar paging; scroll-to-focused-control |
| `TextView` | `NSTextView` inside `NSScrollView` | Multi-line text model (piece table) inside an aqua ScrollView; wrapping and hit-testing through DirectWrite/Pango; selection, IME, clipboard, undo; paints only visible lines |
| `VStack` / `HStack` (`*Stack`) / `Form` (`*FormView`; `Grid` after v0.1) | Flipped `NSView` containers, laid out by Go | The same Go layout engine |
| `MenuBar` / `Menu` (functions returning `MenuBarSpec` / `MenuSpec`) | `NSApp.mainMenu` (global menu bar) | In-window menu strip in the title-bar area. Menus open in rounded popup surfaces with blur/Acrylic. |
| `Alert` | `NSAlert` (sheet or modal) | Sheet that slides out of the title bar, or a centered modal |

---

## 5. The Aqua Engine (Windows/Linux)

### 5.1 Element model

* **Retained element tree** with one `aqua.Element` per public widget. Each element is a
  `layout.Item` (`FittingSize`, `SetFrame`; the public layer runs `internal/layout` over
  the peers, so elements don't measure or arrange themselves, S2.2a) and has
  `Paint(canvas)`, `HandleEvent(ev)` and an `AccessibleNode()`.
* **State machine per control:** `normal / hover / pressed / focused / disabled` ×
  `window key / window inactive` × `light / dark`. macOS greys out accent colors and
  default buttons in inactive windows, and aqua must do the same.
* **Invalidation:** state changes mark elements dirty. v1 repaints the whole window
  (the GPU keeps this cheap). v2 adds dirty rectangles and layer caching.

### 5.2 Theme tokens

* The file `internal/aqua/theme/tokens.go` is generated from a JSON spec (`spec/sequoia.json`)
  and holds colors (light/dark/increased contrast), metrics per control size (heights,
  paddings, radii, bezel shadows), typography (sizes, weights, tracking) and animation
  durations/curves.
* Tokens are **measured from reference screenshots** of native AppKit controls at
  1× and 2× (see §8.3), never copied from Apple assets.
* **Accent color** comes from the OS (Windows accent / portal accent-color) or is
  overridden by the app. The default is macOS blue.
* **Theme target:** v1 targets the **macOS 14/15 (Sonoma/Sequoia)** look. The **macOS 26+
  "Liquid Glass"** look (refraction/specular glass) needs custom shaders and comes later
  as a second token set plus shader effects (§12, decision D2).

### 5.3 Rendering interface

```go
package render

// Point, Size, Rect and Color are driver.Point/Size/Rect/Color (float64 logical
// points, straight-alpha colors); backends narrow to float32 at the native call (S2.1a).

type Canvas interface {
	Save(); Restore()                                         // Push* also save; Pop restores
	Translate(dx, dy float64); Scale(sx, sy float64); Concat(t Transform)
	ClipRoundedRect(r Rect, radii Radii)

	FillRoundedRect(r Rect, radii Radii, p Paint)            // solid or linear gradient
	StrokeRoundedRect(r Rect, radii Radii, width float64, p Paint)
	FillPath(path *Path, p Paint)                             // nonzero winding
	StrokePath(path *Path, width float64, p Paint, s StrokeStyle)
	DrawShadow(r Rect, radii Radii, s Shadow)                 // outer/inner, blur, spread
	DrawText(l TextLayout, origin Point, color Color)         // origin = layout's top-left
	DrawImage(img Image, dst Rect)

	PushOpacity(alpha float64)                                // group opacity
	PushBackdropBlur(r Rect, radii Radii, radius float64)     // menus, sidebars
	Pop()                                                     // ends either layer
}

type TextSystem interface {                                   // provided by the shell
	NewLayout(text string, font Font, maxWidth float64) TextLayout // Font = driver.Font
}
type TextLayout interface {                                   // indices: UTF-8 byte offsets
	Text() string
	Font() Font
	Size() Size
	Baseline() float64
	LineCount() int
	HitTest(p Point) (index int, trailing bool)
	CaretRect(index int) Rect
	SelectionRects(start, end int) []Rect
}
```

The value types (`Radii`, `Paint`, `StrokeStyle`, `Shadow`, `Path`, `Transform`, `Image`)
exist since S2.1a, the interfaces since S2.1b (ARCHITECTURE.md §7). S2.1b refined the
first sketch: `Concat` for arbitrary transforms, one `Pop` for both layers (the separate
`PopOpacity` was dropped), and `Text`, `Font` and `LineCount` on `TextLayout` (the
recorder dumps text and font; the fake shell and tests count lines).

| Primitive | Direct2D (Windows) | GSK (Linux) |
|---|---|---|
| Rounded rect fill | `FillRoundedRectangle` | `gtk_snapshot_append_color` inside a rounded clip |
| Gradient | `ID2D1LinearGradientBrush` | `gtk_snapshot_append_linear_gradient` |
| Border | `DrawRoundedRectangle` | `gtk_snapshot_append_border` |
| Shadow | Shadow effect / cached blurred bitmap | `gtk_snapshot_append_outset_shadow` / `inset_shadow` |
| Text | `DrawTextLayout` (DirectWrite) | `gtk_snapshot_append_layout` (Pango) |
| Blur | Gaussian blur effect | `gtk_snapshot_push_blur` |
| Opacity / transform | Layers / `SetTransform` | `push_opacity` / `transform` |

### 5.4 Window chrome (the hard part of "looks like a Mac")

* **Windows:** remove the standard title bar with `WM_NCCALCSIZE` but keep DWM resize
  borders, shadow and rounded corners. Draw the traffic lights and title in the client
  area and answer `WM_NCHITTEST` with `HTCAPTION` for the drag area. Return
  `HTMAXBUTTON` over the green button so **Windows 11 Snap Layouts still work**. Mica or
  Acrylic can show through transparent areas (sidebars) via DirectComposition with
  premultiplied alpha.
* **Linux:** `gtk_window_set_titlebar()` with our own drawn widget. Dragging calls
  `gdk_toplevel_begin_move`. The buttons call `gtk_window_minimize` / `maximize` /
  `close`.
* **Traffic lights:** red/yellow/green 12 pt circles. Hovering the group shows the ×/−/+
  glyphs. They turn grey when the window is inactive.
* **Menus and popups** (PopUpButton list, menus, tooltips) are **separate native popup
  surfaces** (a layered `WS_POPUP` HWND on Windows, `GtkPopover`/popup surface on
  Linux), so they can extend past the window edges like real macOS menus.

### 5.4.1 Menu bar in the title bar (decision D3)

macOS always uses the global menu bar at the top of the screen. On Windows and Linux the
menu bar is drawn **inside the title-bar area of each window**, to the right of the
traffic lights:

```
┌──────────────────────────────────────────────────────────────────────┐
│ ● ● ●   Controls   File   Edit   View   Help           Controls      │  ← 28 pt title bar
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
```

* The first item is the **app menu**, shown in bold with the app name (About, Settings…,
  Hide, Quit), as in macOS. The window title stays centered and is truncated or hidden
  when the menus need the space.
* Each menu title is its own hit-test region (`HTCLIENT`), and the empty space in between
  stays `HTCAPTION`, so the window can still be dragged by the title bar.
* **Mac behaviour:** a click opens a menu. With a menu open, hovering over the other
  titles switches menus. Press-drag-release selects an item in one gesture. Items are
  highlighted with the accent color. Shortcut hints (`⌘C` on macOS, rendered as
  `Ctrl+C` on Windows/Linux) are right-aligned, with submenu chevrons. Pressing **Alt or
  F10** focuses the strip for keyboard users (the platform convention), and arrow keys
  navigate.
* Shortcuts work even while the menu is closed. The aqua driver handles accelerators
  before key events reach the focused control.
* Menus are rendered with blur/Acrylic backgrounds where available (§3).
* Windows without a title bar (`Style` without `Titled`) have no menu strip. The app can
  mark one window as the "main" window if it has several.

### 5.5 Behaviour ("feel") checklist

* Focus ring: accent-colored glow outside the control that animates in. Tab / Shift-Tab
  move focus. Full keyboard access is optional, as on macOS.
* Default button (Return) and cancel button (Escape). Space activates the focused button.
* Pressed state follows the pointer: dragging off a button un-highlights it, and releasing
  outside does not click.
* Text fields: macOS editing semantics (word selection with double-click, triple-click
  for the line, Alt/Option + arrows move by word, Primary + arrows jump to line ends,
  caret blink rate from the OS). `Primary+A/C/V/X/Z` and `Shift+Primary+Z` work.
* Animations: default 0.2–0.25 s ease-in-out curves with spring options. **Reduce Motion**
  is honoured (OS setting).
* Scrolling (**v0.1**, §5.6): overlay scrollbars that fade out, elastic rubber-banding and
  momentum.
* Window: centered title, "zoom" semantics on the green button, and inactive-window
  appearance for every control.

### 5.6 ScrollView and TextView (added to v0.1, decision D8)

**ScrollView**

* A clip rectangle, a content offset and one document element. Layout measures the
  document with an unbounded height (or width) and the ScrollView clamps the offset.
* **Overlay scrollers** (the macOS default): thin, rounded, drawn on top of the content,
  shown while scrolling and fading out after about 1 s. They widen on hover and support
  click-to-page (by the viewport minus a small overlap, at least half of it) and dragging
  the knob. When the OS asks for always-visible scroll bars
  (`Appearance.LegacyScrollers`; on Windows Settings → Accessibility → "Always show
  scrollbars"), they fall back to **legacy scrollers** (always visible, with a track, the
  viewport inset by their width), as on macOS, switched live.
* **Physics:** precise (touchpad) deltas scroll 1:1. On release, momentum decays
  exponentially. Past the edges the offset is rubber-banded (`offset = limit + d·c/(d+c)`)
  and springs back. Wheel mice scroll in line steps with a short smooth animation. A wheel
  step or a press on a scroller stops a running momentum. Reduce Motion turns off the
  rubber band and the momentum, wheel and key steps jump, and the scrollers hide without a
  fade. The math and the per-axis offset model on the
  frame clock (`Axis`) live in `internal/aqua/scroll`; `anim` has the generic decay and
  the spring with a start velocity. Win32 sends no OS momentum, so it is computed from the
  release velocity; GTK's momentum phases are followed as they come. The scroll view that
  takes a gesture's start stays latched until the gesture ends.
* Keyboard: Page Up/Down (by the viewport minus a small overlap), Home/End, and space and
  Shift-space when the focused element is not a control (so space still presses a focused
  button); the arrows only when the scroll view itself has the focus. A key that cannot
  move the offset bubbles on to an enclosing scroll view. Focusing an element scrolls it,
  with its focus ring, into view by the least amount, innermost scroll view first.
* Rendering: v0.1 clips and translates on the GPU and paints only the visible
  elements (`PaintTree` skips the paint of elements outside the clip, and every subtree
  below a clipping element outside it). Layer caching of the document is a later
  optimisation (§5.1).

**TextView (multi-line)**

* Shares the editing core with TextField (`internal/aqua/text`): a **piece-table** buffer,
  grapheme-aware cursor movement (`rivo/uniseg`), selection, an undo stack with typing
  coalescing, and the macOS keybindings from §5.5 plus paragraph moves (Option-↑/↓) and
  Primary-↑/↓ to document start and end.
* **Layout per paragraph:** one DirectWrite/Pango layout per paragraph, cached and rebuilt
  only when the paragraph changes or the wrap width changes. Only the paragraphs in the
  visible range are laid out and painted, so large documents stay fast.
* IME composition, clipboard, drag-selection with auto-scroll, caret blinking, and
  "scroll caret into view".
* v0.1 is **plain text only**, with one font and color per view. Rich text, find/replace,
  spell checking and line numbers come later.
* macOS: `NSTextView` inside an `NSScrollView` (`scrollableTextView`), so everything is
  native, including spell checking.

---

## 6. Native Bindings (macOS driver and Linux shell)

### 6.1 Calling AppKit without cgo (decision D5)

* AppKit is reached through the Objective-C runtime with **`github.com/ebitengine/purego`
  and `purego/objc`**: `purego.Dlopen` loads `/System/Library/Frameworks/AppKit.framework/AppKit`
  and `objc.ID.Send(sel, args…)` wraps `objc_msgSend`. There are no `.m` files and no
  C compiler is needed.
* **Subclasses and delegates** (the application delegate, window delegate, button target,
  text field delegate, flipped container view) are created at start-up with
  `objc.RegisterClass`. Their methods are Go functions. Each instance stores a `uintptr`
  handle in an ivar, and the method looks the Go peer up in `internal/ffi.Handles`
  (§7.2).
* **Struct arguments and returns** (`NSRect`, `NSSize`, `NSPoint`) go through purego's
  struct support on amd64 and arm64. A small `internal/ffi` test suite checks every
  signature we use on both architectures. For the few calls where this isn't enough
  (`objc_msgSend_stret` on amd64), there is an explicit wrapper.
* **No Objective-C blocks.** APIs that take blocks are avoided. Examples: sheet completion
  uses `beginSheetModalForWindow:modalDelegate:…` style delegates or `runModal`, and
  UI-thread dispatch uses `performSelectorOnMainThread:withObject:waitUntilDone:`. If a
  block API becomes unavoidable, a minimal block literal is built in Go memory following
  the documented block ABI.
* **Autorelease pools** are created around each event-loop turn and each Go callback.
* **Typed wrapper layer:** `internal/appkit/ns` holds hand-written, typed Go wrappers
  (`ns.Window`, `ns.Button` …), one per class we use. Selectors are cached as package-level
  variables, so application code never sees `objc.ID`.

### 6.2 Other details

* Containers are **flipped `NSView`s** (`isFlipped` returns `YES` in the registered
  subclass). The shared Go layout engine sets their frames. Intrinsic sizes come from
  `fittingSize` / `intrinsicContentSize`.
* A standard **Edit menu** is always installed. Without it, ⌘C/⌘V/⌘X/⌘A don't work in
  text fields on macOS. The app menu gets About/Hide/Quit automatically.
* Apps built with `go build` (without a bundle) call
  `setActivationPolicy:NSApplicationActivationPolicyRegular` and `activate` so they get a
  Dock icon and menu bar.
* Later: an optional `GOGUI_DRIVER=aqua` mode on macOS (aqua engine on a Metal/Core
  Graphics shell). This lets you compare native and drawn controls side by side on one
  Mac, which is very useful for pixel-parity work.

### 6.3 Linux without cgo (decision D5)

* `libgtk-4.so.1` (GDK and GSK included), `libgobject-2.0.so.0`, `libglib-2.0.so.0`,
  `libgraphene-1.0.so.0`, `libpango-1.0.so.0`, `libpangocairo-1.0.so.0` and `libcairo.so.2` (both since S4.2g), `libpangoft2-1.0.so.0` and `libfontconfig.so.1` (both since S4.2j, for the app font) are opened by soname at runtime
  (`ffi.Dlopen`, over purego) and their functions bound into tables with `ffi.Bind`
  (`internal/shell/gtk4/gtk`, S4.1b). No variadic C function is called
  (`g_object_new_with_properties`, `g_signal_connect_data`, `g_signal_emitv`,
  `g_type_register_static_simple` instead). GTK older than 4.14 and Pango older than
  1.52 (what GTK 4.14 requires) are refused (D7, S4.2a); functions up to the floor, such
  as the fractional `gdk_surface_get_scale` (4.12) and `GskPath` (4.14), are required.
  Newer ones are looked up at start-up and used when present (so far only Pango 1.56's
  `pango_font_map_add_font_file`, S4.2j, with a fontconfig path below it).
* If GTK 4 (or any of the libraries or symbols) is missing, `Run` returns an error that
  names the soname and the package to install (`libgtk-4-1`, `gtk4`), written to stderr
  and shown in a message box through `zenity` or else `kdialog` if one is on `PATH`. The
  Go binary itself has no link-time dependencies.
* **Signals and callbacks:** a **fixed set of trampolines**, one per C signature, not one
  per signal (16 since S4.1k: the GGSurface's `class_init` and its three vfuncs, a
  `GSourceFunc`, the `GtkTickCallback`, a shared `GClosureNotify` and `GDestroyNotify`
  that free handles, and the signal-handler signatures), registered once per process
  with `ffi.RegisterCallback` and never per widget; a test pins `ffi.Callbacks()` to
  the set. Signals and sources carry an `ffi.Handle` in `user_data` into one dispatch
  table of `{peer, kind}` entries. Every trampoline recovers a panic and re-raises it
  after the native call returns, so no panic unwinds through GTK's C frames. C
  functions are bound with `uintptr` for function-pointer arguments: `ffi.Bind` rejects
  Go func arguments, which purego would turn into a new callback on every call.
* **Custom widget class:** the aqua surface is one `GtkWidget` subclass, `GGSurface`,
  registered with `g_type_register_static_simple` after checking
  `sizeof(GtkWidgetClass)` (408 on 64-bit), so the vfunc slots `class_init` writes
  (`size_allocate`, `measure`, `snapshot`) can't be wrong silently. The instance is a
  `GtkWidget` plus one word holding the peer's `ffi.Handle`.
* **Windows and the loop:** `gtk_init_check` plus a `GMainLoop` (no `GtkApplication`);
  `gogui.Do` wakes the loop with `g_idle_add` from any thread. Each surface is a
  `GtkWindow` with client-side decorations and an invisible GGSurface title bar around
  the GGSurface; GTK owns the window. Frames come from the `GdkFrameClock` tick callback.
* Plain C structs (`GdkRGBA`, `graphene_rect_t`, `GskRoundedRect`, `GTypeQuery`,
  `GValue` …) are mirrored in Go with `structs.HostLayout` and layout tests, and are only
  passed by pointer.
* D-Bus (portal settings for dark mode and accent color, and later the KDE global menu)
  uses pure-Go `github.com/godbus/dbus/v5`.

---

## 7. Cross-Cutting Concerns

### 7.1 Threading and the event loop
* `init()` in package `gogui` calls `runtime.LockOSThread()`, so the main goroutine stays
  on the OS main thread (required by AppKit and strongly preferred by Win32 and GTK).
* `gogui.Do` pushes onto a lock-free queue (`internal/mainthread`) and wakes the native
  loop. Wake-ups are coalesced, and the driver drains the queue on the UI thread.
* No UI calls from finalizers.

### 7.2 Memory and FFI safety (no cgo)
* Go pointers are never stored in native memory. Native objects carry an **integer
  handle** (`uintptr`) that indexes `internal/ffi.Handles`, a map from handle to Go peer
  that is only touched on the UI thread. Handles are freed in `Destroy()`.
* Buffers handed to native code for longer than one call are pinned with `runtime.Pinner`
  or allocated in native memory (`malloc`/`CoTaskMemAlloc`/`g_malloc`).
* **Callbacks are a limited resource.** `purego.NewCallback` and `syscall.NewCallback`
  cannot free callbacks and have a fixed maximum. gogui creates a **fixed set of
  trampolines at start-up** (one per signal/vfunc/window-proc signature) and dispatches
  through the handle in `user_data`, the ivar or `GWLP_USERDATA`. It never creates a
  callback per widget.
  The budget (`ffi.MaxCallbacks`) is 1024 on every OS, shared on darwin/linux with
  purego/objc's own callbacks, and `scripts/check.sh` fails if code outside
  `internal/ffi` creates a callback.
* C strings: `internal/ffi.CString` / `GoString` helpers with explicit freeing. NSString
  goes through `stringWithUTF8String:` inside an autorelease pool.
* Native memory (an ivar holding a handle, a GObject vfunc slot) is read and written
  with `internal/ffi.ReadUintptr` / `WriteUintptr`, and a native struct passed by
  address (a Win32 `lParam`) is read through `ffi.At[T]` (S2.3a), so drivers need no
  uintptr → `unsafe.Pointer` conversion of their own.
* On Windows, COM objects are released explicitly. A `comptr` helper with `Release()` is
  used, and tests track reference counts.
* `go vet`'s `unsafeptr` check stays enabled. Any `uintptr` → `unsafe.Pointer` conversion
  outside `internal/ffi` needs a justified `//nolint` comment.

### 7.3 Appearance changes
* Live switching between light and dark mode and accent-color changes are observed
  (`NSApp effectiveAppearance` KVO, `WM_SETTINGCHANGE`, the portal `SettingChanged`
  signal) and trigger a theme refresh and repaint.

### 7.4 Localization and text
* UTF-8 strings throughout. Right-to-left layout is **out of scope for v1**, but the
  layout engine is designed with a `LayoutDirection` in mind.

### 7.5 Accessibility (decision D4: after v0.1)
* The aqua element tree exposes roles, labels, values, states and actions from day one
  (it costs little to design in and a lot to retrofit). Unit tests check that every v0.1
  control fills in its accessible node.
* On macOS the controls are native, so VoiceOver works in v0.1.
* The Windows/Linux bridge (Narrator/NVDA, Orca) ships **after v0.1** (phase 7). Because
  of the no-cgo rule, the options are:
  1. a **pure-Go UI Automation provider** (COM `IRawElementProviderSimple`/`Fragment`)
     plus **AT-SPI over pure-Go D-Bus**, or
  2. **AccessKit's C API** loaded as a shared library through purego. This means shipping
     `accesskit.dll`/`.so` next to the app.
  The choice is made at the start of phase 7 after a spike. Option 1 is preferred if the
  effort is similar, because it keeps a single static binary.

---

## 8. Testing and Quality Strategy

### 8.1 Unit tests (all platforms, no display)
* Layout engine: main-axis distribution, stacks, spacers, padding, min/max sizes,
  alignment, expansion, forms, Box and ScrollView hosts, the minimum window size, the
  measure cache, and the exact frames of every example layout at its default size, at
  its minimum window size (max(`Root.MinSize`, `WindowConfig.MinSize`)) and at a bigger
  size (S0.4).
* Aqua widgets with a **fake shell** and a **recording canvas**: state machines, focus
  order, keyboard handling, and the text editing model (selection, word boundaries, undo,
  piece-table edits, paragraph layout cache).
* ScrollView physics: momentum decay, rubber-band clamping and spring-back, simulated
  with a fake frame clock.
* `internal/ffi`: struct layout and size checks for every mirrored C/ObjC struct on
  amd64 and arm64. These run on the target OS (`scripts/native.sh`, §8.4), together with a smoke test for each
  trampoline signature.

### 8.2 Golden-image tests (per platform)
* Render the aqua `gallery` scene offscreen: on Windows into the window's swap-chain back
  buffer on the WARP device, read back through a D3D11 staging texture between `EndDraw`
  and `Present` and encoded with Go's `image/png` (no WIC; S3.6c), and
  `gsk_renderer_render_texture` on Linux. Compare against committed PNGs with a
  perceptual tolerance, at 1× and 2×, in light and dark.

### 8.3 Parity tests (macOS reference)
* On macOS, render the same gallery natively (`cacheDisplayInRect:`) to produce the
  **reference images** that the aqua tokens are tuned against.
* The `examples/controls` app is the manual parity check: run it on all three OSes and
  compare.

### 8.4 Native runs (no CI)

* **No CI and no GitHub workflows (user rule 2026-10-13).** Checks run in two places, both
  with `CGO_ENABLED=0`:
  * **Windows host, by Claude:** `bash scripts/check.sh` (gofmt; vet and build for
    `{darwin,windows,linux} × {amd64,arm64}`, which guards the no-cgo rule; staticcheck per
    GOOS; tests; the no-cgo source scan; the hidden-callback guard; the stress run), plus
    `bash scripts/native.sh` for windows/amd64.
  * **User's machines:** `bash scripts/native.sh` runs what needs the target OS: vet,
    `go test -count=1 ./...`, example builds, the callback measurements (S0.10a), the
    spike self-test on macOS (`spike/appkit`), and on Linux (since S4.1l, which deleted
    `spike/gtk`) the verbose `gtk4` tests and `examples/hello` for 5 s (every example since
    S4.2k), each under
    xvfb/X11 and headless weston/Wayland (the gtk4 tests there inside `dbus-run-session`
    when it is installed, since S4.3d) (and on the desktop session if there is one). It runs every step even after a failure and writes
    `native-logs/<goos>-<goarch>.log` (not committed), which the user brings back. The six
    targets are those of §9; arm64 matters for struct-layout tests (§8.1). A target
    without a machine is recorded as "not run (no machine)" in the step's Result. The
    golden tests (§8.2) run there verbosely on Windows since S3.6d (the comparison reports
    in the log), on Linux from S4.3.
* **Race detector: allowed, not scripted.** `go test -race` needs cgo (D5 allows it for
  testing only) and a C compiler, which the Windows host lacks. Concurrency code
  (`internal/mainthread`, `internal/ffi.Handles`) is stress-tested with
  `go test -count=20 -cpu=1,2,4,8` under `CGO_ENABLED=0` either way.

---

## 9. Build and Distribution Notes

**No cgo on any platform (decision D5).** Only the Go toolchain is needed to build, and
any OS can build for any other:

| Platform | Build needs | Runtime needs | Cross-compile? |
|---|---|---|---|
| Windows | Go only | Windows 10 1809+ (all APIs used ship with the OS) | ✅ from any OS |
| macOS | Go only | macOS 13+ (AppKit loaded at runtime) | ✅ from any OS (signing/notarization still needs Apple tools) |
| Linux | Go only | GTK ≥ 4.14 installed (`libgtk-4.so.1`), loaded with `dlopen` | ✅ from any OS |

```sh
GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build ./examples/hello
GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build ./examples/hello
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./examples/hello
```

* The library **must not** import `"C"` anywhere (tests may run with cgo, for example
  `go test -race`, but only in addition to the `CGO_ENABLED=0` runs; D5). `scripts/check.sh` enforces this (§8.4): the
  `go list -f '{{.CgoFiles}}' ./...` check plus a source scan for `import "C"` and native
  source files in `scripts/check.sh` (the scan is needed because with `CGO_ENABLED=0` the
  `go` command silently ignores cgo files instead of listing them).
* Dependencies stay small and pure Go: `ebitengine/purego`, `golang.org/x/sys`,
  `godbus/dbus/v5`, `rivo/uniseg`. On Windows, purego is used only by
  `internal/shell/win32/com` for COM methods that take floats (`syscall.SyscallN` passes
  no floats on windows/arm64; D5, S2.4a); everything else goes through `x/sys/windows`
  and `syscall`.
* **Later:** a `cmd/gogui` helper for packaging (`gogui bundle`): `.app` + `Info.plist` +
  icon (and ad-hoc signing) on macOS, embedded manifest (Per-Monitor-V2 DPI, Win11
  styles) + icon on Windows, and `.desktop` + icon (+ AppImage/Flatpak recipes) on Linux.

---

## 10. Repository Layout

```
gogui/
├── PLAN.md                     ← this file
├── ARCHITECTURE.md             what is implemented today: packages, contracts, invariants
├── CLAUDE.md                   working rules for Claude Code
├── README.md
├── go.mod                      module github.com/radix29/gogui
├── gogui.go                    Run, App, Do/DoSync
├── window.go                   Window, WindowConfig, styles
├── widget.go                   common Widget base (enabled, hidden, tooltip, control size)
├── label.go button.go checkbox.go radio.go textfield.go
├── slider.go progress.go popup.go separator.go box.go
├── scrollview.go textview.go
├── stack.go                    VStack, HStack, Spacer, Form (wraps internal/layout)
├── menu.go alert.go
├── color.go font.go appearance.go keys.go geometry.go
├── driver_darwin.go            selects appkit driver
├── driver_other.go             selects aqua driver (windows || linux)
├── scripts/check.sh            per-step checks (§11.1)
├── testdata/aqua/              pinned aqua frame dumps of the examples (S3.3b, -update)
├── spike/                      throw-away phase 0.5 spikes, deleted once replaced (§11.1)
├── internal/
│   ├── driver/                 peer interfaces + shared types
│   │   ├── stub/               no-op peers (fallbacks while a driver is incomplete)
│   │   └── headless/           display-less driver (tests; default until a native driver exists)
│   ├── layout/                 layout engine (pure Go, fully unit-tested)
│   ├── mainthread/             UI-thread queue
│   ├── ffi/                    handle table, trampolines, layout helpers, Dlopen, C strings
│   ├── appkit/                 //go:build darwin: purego/objc, no .m files
│   │   └── ns/                 typed wrappers (ns.Window, ns.Button …), registered classes
│   ├── aqua/                   drawn widgets (used on windows || linux; no build tags, tests run everywhere)
│   │   ├── theme/              tokens + spec JSON + generator
│   │   ├── anim/               animation engine (curves, springs, timeline, generic decay)
│   │   ├── text/               editing core: piece table, cursor, undo (TextField + TextView)
│   │   ├── scroll/             ScrollView physics: momentum, rubber-band, velocity, Axis (scrollers: aqua/scroller.go)
│   │   ├── menubar/            title-bar menu strip + menu popups
│   │   ├── gallery/            shared gallery scene for golden-image tests
│   │   └── a11y/               accessibility tree
│   ├── render/                 Canvas + TextSystem interfaces, Path, Paint, …
│   │   └── record/             recording Canvas for tests
│   └── shell/
│       ├── shell.go …          Shell / Surface / event / FrameClock interfaces (TextSystem is in render)
│       ├── fake/               test shell
│       ├── native/             native.New(): the shell of the current OS (native_windows.go: win32; native_linux.go: gtk4; native_other.go: ErrNotImplemented)
│       ├── win32/              //go:build windows: pure Go
│       │   ├── w32/            raw Win32 procs, constants and mirrored structs (S2.3a)
│       │   ├── d2d/ dwrite/ dcomp/ dxgi/ com/   (COM vtable bindings)
│       └── gtk4/               //go:build linux: purego-loaded GTK/GLib/Pango, godbus portal
├── assets/fonts/               Inter (OFL) + LICENSE
├── docs/                       design notes, token measurements, parity screenshots
└── examples/                   example applications (one package main per folder)
    ├── README.md
    ├── hello/
    ├── controls/               gallery of every v0.1 control (parity tool)
    ├── form/                   account form: layout, text fields, validation
    ├── converter/              temperature converter: two-way binding, live updates
    └── notes/                  TextView + ScrollView + menus: a minimal plain-text editor
```


---

## 11. Roadmap and Milestones

### v0.1 scope ("basic controls/windows")

**In:** App lifecycle, `Window` (titled, closable, miniaturizable, resizable, min size,
center, key/inactive state), `Label`, `Button` (push, default, cancel), `Checkbox`,
`RadioGroup`, `TextField`, `SecureField`, `Slider`, `ProgressBar` (determinate and
indeterminate), `PopUpButton`, `Separator`, `Box`, **`ScrollView`**, **`TextView`
(multi-line, plain text)**, `VStack` / `HStack` / `Spacer` / `Form`, a `MenuBar` (App,
Edit and custom menus; in the title bar on Windows/Linux), a modal `Alert`, light/dark
mode, accent color and HiDPI. No cgo on any platform.

**Out (later):** TableView/OutlineView, rich text, Toolbar, Sidebar/SplitView, TabView,
sheets, drag and drop, file dialogs API, tray/status items, notifications, RTL, the
accessibility bridge on Windows/Linux, the Liquid Glass theme, packaging CLI.

### Phases

| # | Phase | Deliverables | Exit criteria |
|---|---|---|---|
| **0** | Foundations ✅ | `go.mod`, repo layout, checks (`check.sh`; CI replaced by `native.sh`, §8.4), `internal/driver` interfaces, `internal/layout`, `mainthread`, `internal/ffi`, complete public API skeleton, all examples compiling against a headless driver | `go test ./...` green; cross-compile matrix green; API reviewed |
| **0.5** | purego spikes | Three throw-away proofs: (a) macOS: registered `NSObject` subclass as button target + flipped `NSView`, `NSRect` on arm64 and amd64; (b) Linux: dlopen'd GTK window with a custom `GtkWidget` subclass whose `snapshot` vfunc draws through a trampoline; (c) callback budget measured on both | All three work; R-purego (§13) re-rated |
| **1** | macOS native driver | AppKit driver via `purego/objc` for **all** v0.1 controls (incl. `NSScrollView`, `NSTextView`), menu bar with Edit menu, NSAlert, appearance observation | `hello`, `controls`, `form`, `notes` run natively on macOS; reference screenshots captured |
| **2** | Render + Windows shell ✅ | `render.Canvas`; win32 shell: window, custom chrome, DWM effects, D3D11/D2D/DComp, DirectWrite text, mouse/keyboard, precise wheel/touchpad input, DPI, frame clock | Empty aqua window with traffic lights, drag/resize/snap, crisp text at 100/150/200 % |
| **3** | Aqua widgets I | Theme tokens (light/dark), animation engine, focus system, `Label`, `Button`, `Checkbox`, `Radio`, `Slider`, `ProgressBar`, `Separator`, `Box`, **`ScrollView`** | Gallery on Windows passes visual review against macOS references; golden tests in `native.sh`; scroll physics tests green |
| **4** | Linux shell | gtk4 shell via purego: CSD window, GSK canvas, Pango text, input (incl. kinetic scroll), scale, portal settings via godbus, "GTK missing" error path | Same gallery renders on Linux (Wayland + X11) identically to Windows (shared goldens) |
| **5** | Aqua widgets II | Text editing core; `TextField`/`SecureField`; **`TextView`**; IME on both OSes, clipboard, undo; popup surfaces; `PopUpButton`; **title-bar menu strip**; `Alert` | `form` fully usable by keyboard only; CJK IME works; `notes` edits a 1 MB file smoothly |
| **6** | Polish and v0.1 | Inactive-window states, OS accent, Reduce Motion, perf pass (idle CPU ≈ 0 %, 60/120 fps), docs, all examples | Parity checklist signed off; tagged `v0.1.0` |
| **7** | Post-v0.1 | Accessibility bridge (D4), TableView, Toolbar, Sidebar with vibrancy, sheets, file dialogs, `GOGUI_DRIVER=aqua` on macOS, Liquid Glass theme, packaging CLI | — |

### 11.1 Implementation steps

Each step is one unit of work for Claude, who writes the code and runs the checks below on
the Windows host. The user runs only what needs another OS or a person at the screen.
**After every step the repo is consistent and buildable.** A ✅ step keeps a short
"Result"; the code, tests and ARCHITECTURE.md are the record.

#### How every step stays buildable

1. **Full API early.** S0.5 added the complete public API on a headless driver, so all five
   examples compile on every platform after every step.
2. **Stub peers.** `internal/driver/stub` has a no-op peer per interface. The headless
   driver uses them; the appkit and aqua drivers embed them as fallbacks, so a missing
   control is invisible (appkit) or a labelled placeholder rectangle (aqua).
3. **Fixed driver-selection files.** `gogui` has build tags only in `driver_darwin.go`
   (headless until S1.1) and `driver_other.go` (aqua over `native.New()` from
   `internal/shell/native`, a platform package of its own because the shells import
   `shell`). `native.New()` returns the Win32 shell on Windows, the GTK 4 shell on Linux (since
   S4.1d) and `ErrNotImplemented` elsewhere.
4. **Dependencies arrive with their first import.** A `require` is added only in the step
   that first imports the module (`go get`/`go mod tidy` through the module proxy;
   github.com is unreachable); `go.mod`/`go.sum` are committed in that step.
5. **Spikes stay isolated** in `spike/<name>/`, every file with an OS build tag, and are
   deleted in the step that replaces them.
6. **Examples follow the API:** a public-API change updates §4, the examples and
   `examples/README.md` in the same step.
7. **Generated code is committed** (theme tokens); a build never needs `go generate`.

#### Checks after every step (`bash scripts/check.sh`)

`check.sh` stops at the first failure; `-quick` skips the stress run; a step is ✅ only
after a full run with fresh tests (`-count=1`).

* `gofmt -l .` prints nothing; `go vet ./...` and `go build ./...` pass with
  `CGO_ENABLED=0` for `{darwin,windows,linux} × {amd64,arm64}`; staticcheck per GOOS.
* `go test ./...` passes on the host.
* No-cgo guard: `go list -f '{{.CgoFiles}}'` is empty, and a source scan fails on
  `import "C"` and on `.c .h .m .mm .cc .cpp .cxx .S .swig` files (`CGO_ENABLED=0` makes
  `go` ignore cgo files silently, so the scan is needed).
* Hidden-callback guard: outside `internal/ffi` and tests, no `syscall.NewCallback`,
  `NewCallbackCDecl`, `windows.NewCallback`, `purego.NewCallback`, `objc.NewIMP`,
  `objc.NewBlock` or `objc.RegisterClass`.
* Concurrency code also passes the stress run (`-count=20 -cpu=1,2,4,8`).
* cgo is for test runs only (D5): extra, never instead of the `CGO_ENABLED=0` checks, and
  on the user's machines (the host has no C compiler).
* The step's "Done when" items are met; the Result says what was checked locally and what
  the user still has to run on macOS or Linux.

#### Phase 0: Foundations ✅ *2026-10-09 (S0.1–S0.6, standard library only)*

Results, policies and findings: `PLAN_done.md` §A.

#### Phase 0.5: purego spikes (first external dependency: `ebitengine/purego`)

All built and checked on the Windows host; the macOS and Linux runs are open (see "Open
runs" in `docs/spikes.md`, S0.10c). S0.7–S0.10b (FFI helpers, the three spikes, the
callback budget) are ✅ *on Windows only*; their Results and findings are in
`PLAN_done.md` §B. S0.7–S0.10 are ✅ when S0.10c is.

**S0.10c `docs/spikes.md`, R-purego re-rated, phase 0.5 exit.** `docs/spikes.md` (one
section per spike: proven, numbers, surprises, what S1.1 and S4.1 inherit; ✔ measured / ◐
built, run pending / ○ open) is split by what each part waits on:
* **S0.10c_1** ✅ *2026-10-13*: draft reviewed against S0.8–S0.10 and wired into
  ARCHITECTURE.md and §13; no drift in the numbers.
* **S0.10c_2a** ✅ *2026-10-13*: `native.sh` passed on windows/amd64 (go vet, tests, example
  builds, callback measurements: 1998 free, `MaxCallbacks` 1024, `direct` 165 ns/op); the
  other five targets are "not run (no machine)".
* **S0.10c_2b** (waits on logs): from the user's `native-logs/<goos>-<goarch>.log` (at
  least one Mac and one Linux; windows/arm64 optional; each log has `go test`, the
  callback measurements, the appkit self-test with the `isMultiThreaded` line, the GTK
  runs under xvfb and weston with renderer, resize-path and frame-wait lines; since
  S4.1l these come from the `gtk4` tests, `BACKEND-RESULT`, `RESIZE-RESULT` and the
  clock tests' frame info, as `spike/gtk` is deleted): fill
  the ○ cells of the S0.10 table, turn ◐ into ✔, give S0.7 its ✅, and re-rate R-purego
  (expected High → Medium; the unix per-call cost decides whether S4.1's per-motion-event
  callbacks need a note). A failing run gets its own fix step first (e.g. S0.9c_4); fewer
  than `MaxCallbacks` + 64 free slots on a target means revising the plan.
* **S0.10c_3** (waits on the user): the report of the visual checks on both Macs and of
  the Linux desktop check (since S4.1l the user's look at `examples/hello` on Wayland and
  X11, which replaces `go run ./spike/gtk`). Record in `docs/spikes.md`, drop its "State" preamble and
  "Open runs", give S0.8/S0.9/S0.10 their ✅, mark phase 0.5 done in §11 and ARCHITECTURE.md.
  Done when `check.sh` passes and no ◐ or ○ is left.

#### Phase 1: macOS native driver

**Order change (user decision 2026-10-13):** no Mac for now, so phases 2 (from S2.1) and 3
run **before** phase 1, which resumes when a Mac is available.
* *Waits for a Mac:* S1.5's reference captures (`docs/parity/`). S3.1 ships its tokens as
  *provisional* ("from memory, to be measured"), **S3.1t** tunes them after S1.5, and
  S3.6 builds goldens on the provisional tokens; its visual review and the phase 3 exit
  wait on S1.5. S3.6 defines the gallery scene (`internal/aqua/gallery`) that S1.5
  renders natively through `gogui`.
* *Not a prerequisite:* the appkit code (S1.1–S1.4), the NSRect layout tests (darwin
  only), and phase 0.5's open runs.
* Phase 1 may still change `internal/driver` contracts; such a change goes into the aqua
  driver in the same step.
**S1.0 Driver-selection files** ✅ *2026-10-13* (`PLAN_done.md` §C).

**S1.1 AppKit core**
* `internal/appkit/ns` wrappers: NSApplication, NSWindow, NSView, NSString, NSTextField
  (label), autorelease pools.
* Registered classes: app delegate, window delegate, flipped view, target. UI-thread wake
  via `performSelectorOnMainThread:`. Sets the activation policy.
* The appkit driver embeds stubs for every other control. `driver_darwin.go` selects
  appkit. Adds NSRect/NSSize/NSPoint layout tests and deletes `spike/appkit`.
* Done when: `examples/hello` opens a native window with its label on macOS.

**S1.2 Buttons and toggles.** Push button (default, cancel), Checkbox (incl. mixed),
RadioGroup, Separator, Box.

**S1.3 Text and value controls.** TextField, SecureField (delegate for change and submit),
Slider, ProgressBar, PopUpButton. Done when: `hello` and `converter` work on macOS.

**S1.4 Menus, alert and appearance.** Main menu (app menu, Edit menu always installed,
custom items with shortcuts via a target class and item tag → handle), NSAlert without
blocks, appearance observation (KVO), `SetAppearance`. Done when: `controls` and `form`
work on macOS.

**S1.5 ScrollView, TextView and reference captures.** NSScrollView with a flipped document
view, `scrollableTextView`, `Focus`, and a gallery snapshot helper (`cacheDisplayInRect:`)
writing reference PNGs to `docs/parity/`. Done when: `notes` works and the screenshots are
committed. **Phase 1 exit.**

#### Phase 2: Render and Windows shell

**S2.1–S2.4** ✅ (render and shell interfaces, aqua skeleton, Win32 window and input, Win32 GPU rendering incl. the paced frame clock): Results in `PLAN_done.md` §D.

**S2.5 DirectWrite text** ✅ *2026-10-22* (S2.5a–S2.5g: DirectWrite bindings: factory, formats, layout metrics, hit-testing, clusters, in-memory fonts; the TextSystem: layout and metrics, hit-testing, carets and selection; drawing text; fonts embedded from `assets/fonts` with the Segoe UI fallback): Results in `PLAN_done.md` §E.

**Phase 2 exit** ✅ *2026-10-22* (Inter in `assets/fonts`; the visual check at 100/150/200 % and across monitors, by the user): Result in `PLAN_done.md` §E.

#### Phase 3: Aqua widgets I

**S3.1 Theme tokens** ✅ *2026-10-23* (S3.1a–S3.1d; `PLAN_done.md` §F)
* `internal/aqua/theme/spec/sequoia.json`, a generator (`go generate`) and the committed
  `tokens.go`: light and dark, control sizes and accent. Decide O1 and O2 and update §12.
* While phase 1 waits (order change above), the values are provisional: the spec marks
  each one "from memory, to be measured", and `docs/` lists them for S3.1t.
* Split into S3.1a–S3.1d (2026-10-23). All of it is pure Go with tests on the host;
  nothing needs a Mac or Linux beyond the cross-builds in `check.sh` (only S3.1t does).
  The `Theme` type is hand-written and `tokens.go` only fills one value of it
  (`theme.Sequoia`), so a second token set (D2) needs no widget change. `theme` imports
  only `internal/driver`. The stub metrics stay the layout source of the placeholders
  until S3.3 (the aqua example frames must keep matching the headless run).

**S3.1a Spec, generator, semantic colors** ✅ *2026-10-23* (`PLAN_done.md` §F).

**S3.1b The aqua driver resolves through the theme** ✅ *2026-10-23* (`PLAN_done.md` §F).

**S3.1c Metrics and typography** ✅ *2026-10-23* (`PLAN_done.md` §F).

**S3.1d Contrast, accent, animation; O1 and O2** ✅ *2026-10-23* (`PLAN_done.md` §F; O1 → D9, O2 → D10).

**S3.1t Token tuning against the macOS references** (after S1.5; was S3.1b)
* Measure the S1.5 captures at 1× and 2×, light and dark; replace the provisional values
  in `sequoia.json`, regenerate `tokens.go`, update the S3.6 goldens, and drop the
  "to be measured" marks.
* Done when: no provisional token is left, and the goldens pass with the new values.

**S3.2 Animation and focus** ✅ *2026-10-26* (S3.2a–S3.2f; `PLAN_done.md` §F; the optional manual check of S3.2d is open)
* `internal/aqua/anim` (curves, springs, fake-clock tests), the focus ring, Tab order,
  Return/Escape routing to the default and cancel buttons, and key/inactive window state
  plumbing.
* Split into S3.2a–S3.2f (2026-10-26). Already there from S2.2: `Router` focus
  (`SetFocus`, `FocusOrder`, `FocusNext`, Tab/Shift-Tab, `FocusEvent`), `Window.IsKey`
  and `KeyChanged`, the `Theme.Animation` tokens and `Duration(d, a)` (S3.1d), and
  `shell.FrameClock` with the fake clock. Since S3.2b the driver's `anim.Timeline` runs
  on the clock; since S3.2c elements reach their window (`Base.Window`), get
  `WindowStateEvent` and can tell `FocusedInactive` from `FocusedActive`; since S3.2d the
  focusable placeholders draw the animated focus ring; since S3.2e `Router` has the two
  keyboard-access modes (`SetFullKeyboardAccess`, `FocusKind`) and `Window.Show` the
  initial focus; since S3.2f Return and Escape press the default and cancel buttons.
* All of it is pure Go, tested on the host through `shell/fake` and `render/record`;
  nothing needs a Mac or Linux beyond the cross-builds in `check.sh`. The only manual
  check is optional and on Windows (the focus ring in `examples/form`, S3.2d).
* Order: a → b → c → d; e and f need only c (`Base.Window`), so they can go before d.

**S3.2a `anim`: curves and springs (pure math)** ✅ *2026-10-26* (`PLAN_done.md` §F).

**S3.2b `anim`: animations on the frame clock** ✅ *2026-10-26* (`PLAN_done.md` §F).

**S3.2c Window state plumbing (key / inactive, appearance)** ✅ *2026-10-26* (`PLAN_done.md` §F).

**S3.2d Focus ring** ✅ *2026-10-26* (`PLAN_done.md` §F; the optional manual check in `examples/form` is open).

**S3.2e Tab order and keyboard access** ✅ *2026-10-26* (`PLAN_done.md` §F).

**S3.2f Return and Escape** ✅ *2026-10-26* (`PLAN_done.md` §F).

**S3.3 Basic controls** ✅ *2026-10-27* (S3.3a–S3.3h; `PLAN_done.md` §F; the visual checks of `examples/hello`, `controls` and `form` on Windows are open, user)
* Label, Button, Checkbox, RadioGroup, Separator and Box, each with a state machine and an
  a11y node, plus tests.
* Split into S3.3a–S3.3h (2026-10-26). Already there: the placeholder peers with their
  a11y nodes, focus ring, `FocusKind`, default/cancel marks and `Activate` (S3.2), the
  press-and-capture logic of the traffic lights (`chrome.go`), the `Theme` bezel metrics,
  accent rules (pressed, inactive) and animation tokens (S3.1). Each control step replaces
  one placeholder with a real element and keeps the peer API unchanged. Since S3.3a the
  controls have `ControlState`, the `pressTracker` (the traffic lights use it) and Space
  activation (`control.go`); since S3.3b their tokens (`Theme.Controls`) and the pinned
  aqua frame dumps (`testdata/aqua`); since S3.3c the label is real (`label.go`), since
  S3.3d the separator and the box (`group.go`), since S3.3e the push button (`button.go`),
  since S3.3f the checkbox (`checkbox.go`), since S3.3g the radio (`radio.go`); since S3.3h
  a test walks the a11y nodes of the five examples on aqua, and the placeholders left are
  the controls of S3.4 (Slider, ProgressBar), S3.5 (ScrollView) and phase 5 (TextField,
  TextView, PopUpButton).
* **Sizing policy (from S3.3b):** real controls size themselves from the theme tokens and
  the shell's `TextSystem`, no longer from the stub metrics, so the aqua frames may drift
  from the headless ones. `TestExamplesOnAqua` then checks pinned aqua frame dumps per
  example (plus "no overlap, nothing clipped" on the anchor widgets) instead of equality
  with the headless run; the headless layout tests stay as they are.
* All of it is pure Go, tested on the host through `shell/fake` and `render/record`;
  nothing needs a Mac or Linux beyond the cross-builds in `check.sh` (Linux has no shell
  until S4.1). The visual checks are on Windows and done by the user.
* Order: a → b; then c, d, e, f, g in any order (each needs a and b); h last.

**S3.3a Control state machine and press tracking** ✅ *2026-10-26* (`PLAN_done.md` §F).

**S3.3b Tokens for the basic controls and the sizing switch** ✅ *2026-10-26* (`PLAN_done.md` §F).

**S3.3c Label** ✅ *2026-10-27* (`PLAN_done.md` §F).

**S3.3d Separator and Box** ✅ *2026-10-27* (`PLAN_done.md` §F).

**S3.3e Push button** ✅ *2026-10-27* (`PLAN_done.md` §F; the visual check of `examples/hello` on Windows is open, user).

**S3.3f Checkbox** ✅ *2026-10-27* (`PLAN_done.md` §F).

**S3.3g Radio** ✅ *2026-10-27* (`PLAN_done.md` §F).

**S3.3h Wrap-up** ✅ *2026-10-27* (`PLAN_done.md` §F; the visual check of `examples/controls` and `examples/form` on Windows, light and dark, key and inactive, is open, user).

**S3.4 Value controls**
* Slider and ProgressBar (including the indeterminate animation).
* Done when: `converter` works on Windows, with the text fields still drawn as
  placeholders.
* Split into S3.4a–S3.4g (2026-10-27). Already there: the placeholder `Slider` and
  `Progress` peers in `controls.go` (state, getters and `UserDrag` from the stub, a11y
  nodes, the slider focusable as `FocusControl`), `ControlState` and the `pressTracker`
  (S3.3a), `Theme.Controls` and the accent rules (S3.3b), key events routed to the
  focused element, `Base.VisibleInTree`, and `anim.Timeline` with Reduce Motion (S3.2b).
  Since S3.4a the value-control tokens and `Theme.ValueFill`; since S3.4b `anim.Value.Loop`;
  since S3.4c–S3.4d the real `Slider` (`slider.go`); since S3.4e–S3.4f the real `Progress`
  (`progress.go`, bar and spinner, with the indeterminate loop); since S3.4g the a11y walk
  checks that only ScrollView, TextField, TextView and PopUpButton are still placeholders.
  The public API (`Slider`, `ProgressBar`, `Spinner` in `value.go`) and the driver
  interfaces stay as they are. One `Progress` peer serves `ProgressBar` and `Spinner`
  (`driver.ProgressSpinner`), so the spinner is drawn here too; otherwise it would stay
  the last placeholder of that peer.
* Each control step replaces its placeholder with a real element, follows the S3.3b
  sizing policy, and re-pins the aqua frame dumps (`testdata/aqua`, `-update`) that
  change (`controls` and `converter`).
* All of it is pure Go, tested on the host through `shell/fake` and `render/record`;
  nothing needs a Mac or Linux beyond the cross-builds in `check.sh` (Linux has no shell
  until S4.1). The visual checks are on Windows and done by the user.
* Order: a and b first (independent of each other); c → d; e after a; f after b and e;
  g last. c/d and e can go in either order.

**S3.4a Tokens for the value controls** ✅ *2026-10-28* (`PLAN_done.md` §F).

**S3.4b `anim`: repeating animations** ✅ *2026-10-28* (`PLAN_done.md` §F).

**S3.4c Slider: size and paint** ✅ *2026-10-28* (`PLAN_done.md` §F).

**S3.4d Slider: pointer and keyboard** ✅ *2026-10-28* (`PLAN_done.md` §F; the user's check of the slider on Windows is part of S3.4g).

**S3.4e ProgressBar and Spinner: determinate** ✅ *2026-10-28* (`PLAN_done.md` §F).

**S3.4f Indeterminate animation** ✅ *2026-10-28* (`PLAN_done.md` §F).

**S3.4g Wrap-up** ✅ *2026-10-28* (`PLAN_done.md` §F; the visual check of `converter` and `controls` on Windows is open, user).

**S3.5 ScrollView** ✅ *2026-10-29* (S3.5a–S3.5h; `PLAN_done.md` §F; the visual check of `examples/notes` on Windows is open, user)
* `internal/aqua/scroll`: momentum, rubber-banding, overlay and legacy scrollers, keyboard
  paging and scroll-to-focused-control.
* Done when: the physics tests pass and the `notes` sidebar scrolls on Windows.
* Split into S3.5a–S3.5h (2026-10-28). Already there: the placeholder `ScrollView` peer
  (`controls.go`: a clipping `viewport` host, the `document` host framed at `-Off`, a11y
  `RoleScrollArea`), the stub's offset logic (`MaxOffset`, clamping, `ScrollRectToVisible`,
  `UserScroll`, `UserResizeViewport`), `layout.ScrollView` (measures the document unbounded
  on the scrolling axes), the public API (`scrollview.go`), `shell.ScrollEvent` (precise
  points vs wheel lines, natural direction applied, `Phase`/`Momentum`, `Time` on the frame
  clock; Win32 invents the phases and sends `PhaseEnded` with a zero delta ~100 ms after the
  last precise event, never `Momentum`), `fake.Surface.Wheel`/`Swipe`/`Momentum`,
  `Appearance.LegacyScrollers` (Win32: "Always show scrollbars"), the router's scroll
  bubbling and key bubbling, `anim.Timeline` with Reduce Motion, and the tokens
  `ScrollerFadeDelay`, `ScrollerFadeDuration`, `RubberBandSpring`. Since S3.5a the scroller
  and physics tokens (`Theme.Scroll`); since S3.5b `internal/aqua/scroll` (`RubberBand` and
  its inverse, `Decay`, the `VelocityTracker` with the Win32 end rule); since S3.5c `anim`'s
  `SpringFrom` and `Decay`, `scroll.Axis` and `fake.Surface.TimedSwipe`; since S3.5d the real
  `ScrollView` peer (`scrollview.go`), the router's scroll latching (`Router.Latched`) and paint
  culling in `PaintTree`; since S3.5e the overlay scrollers (`scroller.go`) and `anim`'s
  `AnimateAfter` (the fade); since S3.5f hover, knob drag, track click and the legacy
  scrollers (`LegacyScrollers` read, `ViewportInset`); since S3.5g keyboard paging and
  scroll-to-focused (the router's focus hook, `focusRevealer`); since S3.5h the a11y walk
  checks that only TextField, TextView and PopUpButton are still placeholders.
* Missing: nothing; S3.5 is complete since S3.5h (the user's check of `notes` on Windows
  is open). (The focus-changed hook in `Router` came with
  S3.5g; the decay motion, the spring with a start velocity, timed fake swipes, paint
  culling, scroll latching and reading `LegacyScrollers` came with S3.5c–S3.5f.)
* The driver interfaces and the public API stay as they are (no public
  `ScrollRectToVisible`; it stays a driver method. Scroll-to-focused does the same far-edge,
  near-edge scroll on the axes itself, since S3.5g, so it fires `Scrolled` and starts from a
  running step's target).
* All of it is pure Go, tested on the host through `shell/fake`, the fake clock and
  `render/record`; nothing needs a Mac or Linux beyond the cross-builds in `check.sh`. The
  GTK path (OS `Momentum` phases) is covered by the fake `Momentum` helper and runs for
  real only from S4.1. The visual check is on Windows and done by the user (S3.5h).
* Order: a and b first (independent); c after a and b; d after c; e after d; f after e;
  g after d (any order with e/f); h last.

**S3.5a Tokens for scrolling** ✅ *2026-10-28* (`PLAN_done.md` §F).

**S3.5b `scroll`: physics (pure math)** ✅ *2026-10-28* (`PLAN_done.md` §F).

**S3.5c `scroll`: the offset model on the frame clock** ✅ *2026-10-28* (`PLAN_done.md` §F).

**S3.5d ScrollView element: content scrolling** ✅ *2026-10-28* (`PLAN_done.md` §F).

**S3.5e Overlay scrollers: paint and fade** ✅ *2026-10-29* (`PLAN_done.md` §F).

**S3.5f Scroller interaction and legacy scrollers** ✅ *2026-10-29* (`PLAN_done.md` §F).

**S3.5g Keyboard paging and scroll-to-focused** ✅ *2026-10-29* (`PLAN_done.md` §F).

**S3.5h Wrap-up** ✅ *2026-10-29* (`PLAN_done.md` §F; the user's check of `examples/notes` on Windows is open: touchpad 1:1 with momentum and rubber-band, wheel steps, overlay fade and hover, knob drag, track click, the live "Always show scrollbars" switch, Page/Home/End and Tab reveal; light and dark).

**S3.6 Golden-image tests** ✅ *2026-10-30* (S3.6a–S3.6e; `PLAN_done.md` §F; built and passing on the provisional tokens; the visual review against macOS, S3.1t and the phase 3 exit wait on S1.5)
* A shared gallery scene (`internal/aqua/gallery`), offscreen D2D rendering read back
  through staging, and golden PNGs at 1× and 2× in light and dark, compared with a
  tolerance. Adds them to `scripts/native.sh` (§8.4).
* Done when: visual review against the macOS references is done. **Phase 3 exit.** With
  phase 1 deferred, S3.6 is built and its goldens pass on the provisional tokens first;
  the review, S3.1t and the phase 3 exit follow S1.5.
* Split into S3.6a–S3.6e (2026-10-29). Already there: an aqua window on a real, hidden
  Win32 surface with the frame read back between EndDraw and Present (`framePx`,
  `readAll`, `setDPI` for 1× and 2×) and a PNG writer (`writePNG`, `GOGUI_SCREENSHOTS`),
  all in `internal/shell/win32/aqua_test.go` (S2.4d_3); an offscreen render-target texture
  with staging readback (`canvas_test.go`, S2.4d_2); `aqua.New(shell)` as the widget
  factory over any shell; `Appearance.ReduceMotion` (animations jump to their end) and
  `Appearance.LegacyScrollers` (scrollers that do not fade); grayscale text antialiasing;
  the per-control state dumps (`internal/aqua/testdata/*.states`), which already cover
  hover and pressed states through the recorder. Since S3.6a the gallery scene
  (`gallery.Build`, `gallery.Open`) and its pinned recorder dump; since S3.6b the PNG files and
  the image comparison (`gallery/golden`: `ReadPNG`, `WritePNG`, `Compare`); since S3.6c
  the offscreen render on WARP (`gallery.Fill`, `newGPU(warp)`, `withGalleryRenderer` in
  `internal/shell/win32/gallery_test.go`); since S3.6d the four committed goldens,
  `TestGalleryGolden` and its verbose step in `native.sh` on Windows.
* **Design (2026-10-29):**
  * *The scene is built on `driver.Driver`*, not on the aqua elements or the public API:
    `gallery.Build(d driver.Driver) (driver.Widget, driver.Size)` with fixed frames
    (`SetFrame`, no layout engine), so S1.5 can build the same scene on the appkit driver
    and get the same frames. `gallery` imports only `internal/driver` (no `gogui`, which
    would make an import cycle through `shell/native` into `win32`'s tests). It holds only
    states that the driver API can set (enabled, default/cancel, checked/mixed, values,
    control sizes, focus, key/inactive window, appearance); hover and pressed stay in the
    state dumps.
  * *Readback instead of WIC:* the frame is read back through a D3D11 staging texture
    (the code exists) and encoded with Go's `image/png`. That needs no WIC bindings
    (§8.2 says so since S3.6c).
  * *Determinism:* golden renders use the WARP device (`newGPU` gets a test option), use
    Reduce Motion and legacy scrollers, and are taken only once no animation is running.
    Text is Inter from `assets/fonts`, so it does not depend on the installed fonts.
  * *Images:* the whole window (title bar and traffic lights included) at 1× and 2×,
    light and dark: four PNGs in `internal/aqua/gallery/testdata/golden/`, shared with
    Linux in S4.3. They are straight-alpha RGBA, with no premultiplied bytes in them.
* All of it is tested on the Windows host. `gallery` and the comparison are pure Go and
  also run on the fake shell everywhere. Nothing needs a Mac or Linux beyond the
  cross-builds in `check.sh`. On the user's side: `native.sh` on windows/arm64 (optional;
  it shows whether the tolerance holds across WARP builds), and the visual review against
  macOS after S1.5.
* Order: a and b first (independent); c after a; d after b and c; e last.

**S3.6a Gallery scene (`internal/aqua/gallery`, pure Go)** ✅ *2026-10-29* (`PLAN_done.md` §F).

**S3.6b Image comparison (pure Go)** ✅ *2026-10-29* (`PLAN_done.md` §F).

**S3.6c Offscreen gallery render on Windows** ✅ *2026-10-30* (`PLAN_done.md` §F).

**S3.6d Goldens and native.sh** ✅ *2026-10-30* (`PLAN_done.md` §F).

**S3.6e Wrap-up** ✅ *2026-10-30* (`PLAN_done.md` §F; open, user: the four PNGs on Windows (optional), `native.sh` on windows/arm64, and after S1.5 the visual review against the macOS references, the phase 3 exit together with S3.1t).

#### Phase 4: Linux shell (adds `godbus/dbus/v5` in S4.3)

**S4.1 GTK window and input** ✅ *2026-11-02, built only* (S4.1a–S4.1l; `PLAN_done.md` §H; the "Done when" waits on a Linux log of both backends and the user's look at `hello` on Wayland and X11)
* Dlopen of GTK, GLib, GObject, Pango and Graphene. A version check, and the
  "GTK missing" error path.
* A CSD window and the custom `GtkWidget` class (from spike B) with
  measure/size_allocate/snapshot trampolines. The main loop with a `g_idle_add` wake.
* Pointer, keyboard and kinetic scroll input, and scale handling. Deletes `spike/gtk`.
* Done when: `hello` opens a window with traffic lights and placeholders on Wayland and
  X11.
* Split into S4.1a–S4.1l (2026-10-30). **No Linux machine (user, 2026-10-30), and the
  spike has never run on Linux** (S0.10c_2b open, `native-logs/` has only Windows). So
  each sub-step keeps as much as possible pure Go and untagged, so its tests run on the
  Windows host. The GTK calls are built and cross-compiled (`check.sh`), but they cannot
  run here. Their tests are `//go:build linux`, run by `native.sh` under xvfb and
  headless weston, and each Result lists them as "not run (no machine)". Every sub-step
  that adds native code is ✅ *built only* until a Linux log comes back; the step-wide
  "Done when" waits on it.
* **Design (2026-10-30):**
  * *Package layout.* `internal/shell/gtk4` (the shell, `linux`), plus
    `internal/shell/gtk4/gtk` (bindings; only `Load` is `linux`; the binding tables, struct mirrors and
    constants untagged with host tests, 64-bit only; changed in S4.1b so the table check runs on the host). `internal/shell/shellkit`
    (untagged, pure Go) gets the OS-independent logic now inside `win32`: click counter,
    event-time mapping with the monotonic clamp, precise-scroll gesture phases,
    `clampSize`/points↔pixels with the hint, last-to-first region hit-testing,
    `centerIn`. `win32` switches to it, with no change in behaviour.
  * *Trampolines by signature, not by signal.* `user_data` is an `ffi.Handle` into one
    dispatch table of `{peer, kind}` entries. A shared `GClosureNotify` (`destroy_data`)
    frees the handle. The GGSurface vfuncs keep the handle word in the instance. Every
    trampoline recovers panics and re-raises them after the native frame returns, the
    first panic winning (as `win32/wndproc.go`).
  * *No `GtkApplication`.* `gtk_init_check` plus a `GMainLoop`, as in the spike. App ID
    and single-instance wait until they are needed.
  * *Scale.* Fractional `gdk_surface_get_scale` with `notify::scale` of the GdkSurface.
    (Until S4.2a, on the 4.10 floor, it was an optional binding with the integer
    `gtk_widget_get_scale_factor` and `notify::scale-factor` as the 4.10/4.11 fallback;
    the 4.14 floor made it required and the fallback went.)
  * *Interim pieces* (one `interim_*.go` file each; both gone since S4.2b and S4.2g):
    `fake.NewTextSystem()`, and a minimal snapshot canvas. Fill, stroke and clip of
    rounded rects go through color/border nodes plus rounded clips, which is enough
    for the traffic lights. `Save`/`Restore`/`Translate`/`PushOpacity` are real. Paths,
    shadows, blur, images and text draw as outlined placeholder boxes (as Win32 did
    before S2.4e/S2.5).
  * *Not settable:* `Center` and the initial position (the compositor places windows on
    Wayland, and GTK 4 has no `gtk_window_move` on X11 either). This is documented, and
    on GTK both do nothing (S4.1f).
* Open points for S4.2 (noted here so they are not lost): `GskPath` needs GTK 4.14, so
  with the D7 floor of 4.10, `FillPath`/`StrokePath` need a fallback (a cairo node via
  `libcairo`, or raising the floor). Decided in S4.2: the floor is raised to 4.14 (S4.2a). Also (S4.1c finding):
  GTK passes `for_size` -1 to a CONSTANT_SIZE widget, which GGSurface is; a
  height-for-width surface (text views) needs a `get_request_mode` override (slot 224).
* Order: a and b first (independent); c after b; d after c; e after d; f after a and e;
  g after f; h, i, j and k after f (any order; h after g if regions must be visible);
  l last.

**S4.1a `shellkit`: shared shell logic (pure Go, host-tested)** ✅ *2026-10-30* (`PLAN_done.md` §H).

**S4.1b GTK bindings (`internal/shell/gtk4/gtk`)** ✅ *2026-10-30, built only* (`PLAN_done.md` §H; the linux tests wait on a Linux log).

**S4.1c Trampolines and the GGSurface class** ✅ *2026-10-30, built only* (`PLAN_done.md` §H; the linux tests wait on a Linux log).

**S4.1d Shell core and selection** ✅ *2026-10-30, built only* (`PLAN_done.md` §H; the linux tests wait on a Linux log).

**S4.1e Frame clock** ✅ *2026-10-30, built only* (`PLAN_done.md` §H; the linux tests wait on a Linux log).

**S4.1f Surface: CSD window, sizes and lifecycle** ✅ *2026-10-30, built only* (`PLAN_done.md` §H; the linux tests wait on a Linux log).

**S4.1g Interim snapshot canvas** ✅ *2026-11-02, built only* (`PLAN_done.md` §H; the linux tests wait on a Linux log).

**S4.1h Chrome regions** ✅ *2026-11-02, built only* (`PLAN_done.md` §H; the linux test waits on a Linux log).

**S4.1i Pointer input** ✅ *2026-11-02, built only* (`PLAN_done.md` §H; the linux test waits on a Linux log).

**S4.1j Scroll input** ✅ *2026-11-02, built only* (`PLAN_done.md` §H; the linux test waits on a Linux log).

**S4.1k Keyboard input** ✅ *2026-11-02, built only* (`PLAN_done.md` §H; the linux test waits on a Linux log).

**S4.1l Wrap-up** ✅ *2026-11-02, built only* (`PLAN_done.md` §H; the Linux log of both backends and the user's look at `hello` on Wayland and X11 are open).

**S4.2 GSK canvas and Pango text** ✅ *2026-11-04, built only* (S4.2a–S4.2k; `PLAN_done.md` §I; the "Done when" waits on a Linux log of both backends and the user's look at `hello`, `controls`, `form` and `notes` on Wayland and X11)
* `render.Canvas` over `GtkSnapshot`, the `TextSystem` over Pango, and Inter loaded as an
  app font.
* Split into S4.2a–S4.2k (2026-11-02; Windows needed thirteen sub-steps for the same
  ground, S2.4d–S2.5g). As in S4.1, there is **no Linux machine**: the logic stays untagged
  behind small interfaces (`snapshotter`, and a `pangoLayouter` for text) so it is
  host-tested, the GTK/Pango calls are cross-compiled, their `//go:build linux` tests
  (render-node tree checks as in S4.1g, run by `native.sh` on both backends) are "not
  run (no machine)", and each sub-step is ✅ *built only* until a Linux log comes back.
  Bindings arrive with the sub-step that first calls them (as in S4.1h–S4.1k).
* **Design (2026-11-02):**
  * *GTK floor raised to 4.14 (user decision 2026-11-02, amends D7; applied in S4.2a).*
    `GskPath` (paths, `push_fill`/`push_stroke` for gradient paints) is 4.14+; every
    distro D7 names ships 4.14 or is end of life, so there is no cairo fallback and no
    `libcairo` for paths (S4.2g loads `libcairo.so.2` only for the text context's font
    options: hint metrics off). Fractional `gdk_surface_get_scale` (4.12) becomes required.
  * *State as in S4.1g.* The S4.1g canvas's state model (frames counting their pushes,
    the matrix scratch, `finish`) becomes the real canvas; only the placeholder boxes go.
  * *Backdrop blur by re-appending the backdrop* (Win32 closes and reopens its layers the
    same way, S2.4e_3). GSK has no backdrop filter, so the canvas draws into a child
    `GtkSnapshot` per segment: at `PushBackdropBlur` it ends the segment
    (`gtk_snapshot_free_to_node`), appends that node to the parent, appends it again
    under `push_rounded_clip` + `push_blur` (with the S2.4e_3 edge rule), and reopens the
    open transforms, clips and opacities in a new segment. The fallback, when a step
    fails, is a fill in the blur's tint only. (S4.2f: `render.Canvas`'s blur has no tint
    and no step after the cut can fail, so a blur that can't be done pushes only its
    clip, as on Win32.)
  * *Text kept as Go data, as on Win32 (S2.5d).* `TextLayout` has no `Release` and nothing
    may rely on finalizers, so `NewLayout` measures once (size, baseline, lines, the
    cluster geometry for hit-testing) and unrefs its `PangoLayout`; `DrawText` rebuilds one
    per layout and frame. One `PangoContext` from `pango_cairo_font_map_get_default()`
    (the font map GTK draws with), with `round_glyph_positions` off; sizes are absolute
    (13 pt = 13 logical px, `set_absolute_size`); wrapping is `PANGO_WRAP_WORD_CHAR`. NUL
    and invalid bytes are replaced by one-byte stand-ins, so byte offsets map 1:1.
  * *Fonts.* Empty family: Inter (S4.2j; the system UI font when loading it fails), else the family of GtkSettings `gtk-font-name`,
    else `sans-serif`; `Monospaced`: `monospace`. Inter is written once to
    `os.UserCacheDir()/gogui/fonts/<sha256>.ttf` (fontconfig cannot load from memory),
    then added with `pango_font_map_add_font_file` (Pango 1.56, looked up at runtime)
    or `FcConfigAppFontAddFile` + `pango_fc_font_map_config_changed`; `assets/fonts`
    gets its second importer.
  * *Not in S4.2:* the S4.1c `get_request_mode` override (surfaces don't size to their
    text; it comes back when a surface needs height-for-width). Radial gradients don't
    exist in `render.Paint`.
* Order: a first; b–f in order (canvas); g after a; h and i after g (any order); j after
  g; k last.

**S4.2a GTK 4.14 floor** ✅ *2026-11-03, built only* (`PLAN_done.md` §I; the linux tests wait on a Linux log).

**S4.2b The canvas leaves interim** ✅ *2026-11-03, built only* (`PLAN_done.md` §I; the linux tests wait on a Linux log).

**S4.2c Paths** ✅ *2026-11-03, built only* (`PLAN_done.md` §I; the linux tests wait on a Linux log).

**S4.2d Shadows** ✅ *2026-11-03, built only* (`PLAN_done.md` §I; the linux tests wait on a Linux log).

**S4.2e Images** ✅ *2026-11-03, built only* (`PLAN_done.md` §I; the linux tests wait on a Linux log).

**S4.2f Backdrop blur** ✅ *2026-11-03, built only* (`PLAN_done.md` §I; the linux tests wait on a Linux log).

**S4.2g Pango text system: layout and metrics** ✅ *2026-11-03, built only* (`PLAN_done.md` §I; the linux tests wait on a Linux log).

**S4.2h Hit-testing, carets and selection** ✅ *2026-11-03, built only* (`PLAN_done.md` §I; the linux tests wait on a Linux log).

**S4.2i Drawing text** ✅ *2026-11-04, built only* (`PLAN_done.md` §I; the linux tests wait on a Linux log).

**S4.2j Inter as an app font** ✅ *2026-11-04, built only* (`PLAN_done.md` §I; the linux tests wait on a Linux log).

**S4.2k Wrap-up** ✅ *2026-11-04, built only* (`PLAN_done.md` §I; the Linux log of both backends and the user's look at `hello`, `controls`, `form` and `notes` are open).

**S4.3 Portal settings and goldens**
* Dark mode and accent color through the portal (`SettingChanged`), and golden tests on
  Linux that share their reference PNGs with Windows.
* Done when: the gallery matches the Windows goldens (under the Linux `golden.Options`
  that S4.3i calibrates). **Phase 4 exit.**
* Split into S4.3a–S4.3i (2026-11-04). As in S4.1 and S4.2, there is **no Linux machine**:
  the logic stays untagged behind small interfaces and is host-tested, the GTK and D-Bus
  calls are cross-compiled, their `//go:build linux` tests run through `native.sh`, and
  each such sub-step is ✅ *built only* until a Linux log comes back. S4.3a and S4.3e are
  fully checked on the Windows host.
* **Design (2026-11-04):**
  * *Model: the Win32 appearance path* (`win32/appearance.go`, S2.3). A pure
    `readAppearance(src)` over a small `settingsSource` interface, starting from
    `driver.DefaultAppearance()`; every missing, unreadable or out-of-range value keeps its
    default. The shell caches the result, re-reads on any change signal and dedupes with
    `==`, then calls `Handler.AppearanceChanged` (after `Ready`, not after quit).
  * *Portal keys* (`org.freedesktop.portal.Settings` on
    `org.freedesktop.portal.Desktop` `/org/freedesktop/portal/desktop`, namespace
    `org.freedesktop.appearance`): `color-scheme` u (1 dark; 0 and 2 light), `accent-color`
    (ddd) sRGB 0..1 (out of range means unset → `DefaultAccent`, alpha always opaque),
    `contrast` u (1 → `IncreaseContrast`), `reduced-motion` u (1 → `ReduceMotion`, newer
    portals only). `ReadAll` first, `ReadOne`/the deprecated doubly wrapped `Read` as
    fallback; `NotFound` is "unset".
  * *GtkSettings fill what the portal lacks:* `gtk-enable-animations` false →
    `ReduceMotion`, `gtk-overlay-scrolling` false → `LegacyScrollers`, and without a
    portal `gtk-application-prefer-dark-theme` → `Dark`. Live through `notify::<name>` on
    the existing `sigPointer` trampoline (no new trampoline). `ReduceTransparency` has no
    Linux source and stays false.
  * *No session bus or no portal* is not an error: the GtkSettings values, else the
    defaults, logged once at debug level. The start-up read is synchronous before `Ready`
    with a short timeout (250 ms) so a hung bus cannot block start-up; `SettingChanged`
    arrives on godbus's goroutine and is handed to the UI thread with `Shell.Post`. The
    connection is closed in `shutdown()` on every way out.
  * *Goldens:* the reference PNGs are generated on Windows only (WARP); the Linux test
    never rewrites them (`-update` is refused there). Text rasterization (DirectWrite vs
    Pango/GSK) and AA edges differ, so `DefaultOptions` (`MaxDiff 0`) cannot hold: Linux
    gets its own `Options`, provisional until a Linux log calibrates it (S4.3i); masking
    the text boxes from the dump is the fallback if a tolerance alone is too loose.
    The render uses the window's own GSK renderer (what users see) and logs its type.
* Order: a first; b after a; c after b; d after c; e independent (any time); f after c;
  g after e and f; h after d and g; i last (needs the user's Linux log).

**S4.3a Portal mapping (pure Go, host-tested)** ✅ *2026-11-04* (`PLAN_done.md` §J; fully checked on the host).

**S4.3b Portal client over godbus (adds `godbus/dbus/v5`)** ✅ *2026-11-04, built only* (`PLAN_done.md` §J; the client runs on the host against an in-process fake bus; its linux tests on a real session bus wait on a Linux log).

**S4.3c Shell wiring** ✅ *2026-11-04* (`PLAN_done.md` §J; the shell logic is host-tested with a fake source and the real client over the fake bus; only `New`'s two lines are linux-only, and the real session bus waits on the Linux log with S4.3b's tests).

**S4.3d GtkSettings: live values** ✅ *2026-11-04, built only* (`PLAN_done.md` §J; the notify dispatch is host-tested through the real `sigPointer`; the linux test over the real GtkSettings and `native.sh`'s `dbus-run-session` path wait on a Linux log).

**S4.3e Shared golden harness (Windows host)** ✅ *2026-11-04* (`PLAN_done.md` §J; fully checked on the host).

**S4.3f Linux render seams and readback**
* What a golden render on GTK needs: the fake appearance source (S4.3c), a scale seam on
  `gtkWindow.scale()` for 2× on a 1× display, a way to make the surface key without user
  input (show it under xvfb/weston and wait for `kindActive`, or a test hook), and the
  readback chain from the spike: widget paintable → node →
  `gsk_renderer_render_texture` (already bound, unused) → `gdk_texture_download` (BGRA
  premultiplied, the S4.3e conversion). Bindings arrive here only if a missing one is
  needed (e.g. `gtk_widget_get_native`).
* Tests: host tests of the seams; a linux test that reads back a plain filled surface
  and checks its pixels and size at 1× and 2×.

**S4.3g Linux gallery golden test**
* `gtk4/gallery_linux_test.go`: the first aqua driver on the gtk4 shell in a test.
  `gallery.Fill`, the four variants, settle until no animation runs, read back, then
  the S4.3e flow against the shared PNGs with a provisional Linux `Options` (it logs
  every `Result` and passes while uncalibrated only if the size matches and the render is
  deterministic run to run; the strict check turns on in S4.3i). Renderer type logged.
* `native.sh`'s linux case runs it once per backend (xvfb and headless weston) with
  `GOGUI_GOLDEN_OUT=native-logs/golden-<backend>`; the log lists the `Result` lines.

**S4.3h Wrap-up (built only)**
* ARCHITECTURE.md (the gtk4 appearance path, the portal, the Linux goldens), PLAN.md §8
  (native.sh on Linux runs the goldens), the S4.3 Results to `PLAN_done.md`.
* Open, user: `native.sh` on linux/amd64 (and arm64 if available) on Wayland and X11,
  bringing back the log and `native-logs/golden-*/`; switching dark mode and the accent
  in GNOME or KDE settings while `examples/controls` runs.

**S4.3i Calibrate the Linux tolerance (after the Linux log)**
* From the logged `Result`s and diff PNGs: set the Linux `Options` (or add text masking
  to `golden` if a tolerance alone would hide real regressions), turn the strict check
  on, fix any real differences the diffs show (in the gtk4 canvas, not in the goldens).
* Done when: the gallery matches the Windows goldens under those `Options` on both
  backends. **Phase 4 exit.**

#### Phase 5: Aqua widgets II (adds `rivo/uniseg` in S5.1)

**S5.1 Text editing core** (`internal/aqua/text`)
* Piece table, grapheme-aware cursor, word boundaries, selection, undo with typing
  coalescing, and the keybinding map from §5.5.
* It depends only on uniseg, so it **may be pulled forward** to any point after S0.6
  (risk table, §13).

**S5.2 TextField and SecureField**
* Caret blink, selection painting, mouse selection (double- and triple-click), clipboard
  through both shells, and undo. No IME yet.

**S5.3 IME**
* IMM32 composition on Windows and `GtkIMContext` on Linux, with a preedit display.
* Done when: CJK input works on both OSes.

**S5.4 TextView**
* A paragraph layout cache, visible-range painting inside the aqua ScrollView,
  drag-select with auto-scroll, and scroll-caret-into-view.
* Done when: `notes` edits a 1 MB file smoothly.

**S5.5 Popup surfaces and PopUpButton**
* A shell popup API (layered `WS_POPUP` and a GTK popup surface), and the menu element
  that opens over the selected item.

**S5.6 Title-bar menu strip** (`internal/aqua/menubar`)
* Menu-title hit regions inside the caption area, menus, accelerators handled before focus
  dispatch, and Alt/F10 keyboard access. Decide O3.

**S5.7 Alert**
* A sheet out of the title bar, or a centered modal.
* Done when: `form` is fully usable by keyboard only. **Phase 5 exit.**

#### Phase 6: Polish and v0.1

**S6.1 Platform polish**
* Inactive-window states for every control, the OS accent color, Reduce Motion, and the
  increased-contrast tokens.

**S6.2 Performance**
* Idle CPU about 0 % (the frame clock only runs while something animates), and
  60/120 fps for scrolling and animations.

**S6.3 Release**
* Docs and README, a final pass over `examples/README.md`, the parity checklist, and a
  decision on O4 (license).
* Done when: `v0.1.0` is tagged. **Phase 6 exit.**

Steps for phase 7 are planned at its start, after the accessibility spike (§7.5).

---

## 12. Decisions

D1–D8 were resolved in the Q&A of **2026-09-30**, D9 and D10 (formerly O1 and O2) in S3.1d. Changing any of these later needs an update to this
section and the sections it references.

| ID | Question | Decision | Consequences |
|---|---|---|---|
| D1 | Linux: shared aqua engine or GTK 4 widgets + macOS CSS theme? | ✅ **Shared drawn aqua engine** on a GTK 4 window (GSK GPU rendering). | Windows and Linux look and behave identically. Text input/IME must be built once, carefully (phase 5). The GTK-theme route is no longer a fallback. |
| D2 | Theme target for aqua | ✅ **Sonoma/Sequoia first, Liquid Glass later** as a second token set plus shader effects (post-v0.1). | Simpler v0.1 renderer (no refraction shaders). `render.Canvas` and tokens must allow adding a theme without widget changes. Native macOS shows whatever the OS provides. |
| D3 | Menu bar on Windows/Linux | ✅ **macOS-style menu strip in the title-bar area**, next to the traffic lights (§5.4.1). | Title-bar hit-testing must mix `HTCAPTION` and menu regions. Keyboard access via Alt/F10. A KDE global-menu export is possible later. |
| D4 | When do screen readers work on Windows/Linux? | ✅ **After v0.1.** The accessibility tree is designed and tested from day one; the bridge ships in phase 7. | v0.1 is not accessible on Windows/Linux (it is on macOS). Bridge technology (pure-Go UIA + AT-SPI vs AccessKit shared library) is chosen by a spike in phase 7 (§7.5). |
| D5 | cgo? | ✅ **No cgo in shipped code.** macOS via `purego/objc`, Linux via purego-loaded GTK, Windows via `x/sys/windows` (**amended 2026-10-16:** from S2.4a also purego, for COM calls with float arguments, because `syscall.SyscallN` passes no floats on windows/arm64; §1, §2.1, §3 and §9 updated in S2.4a). Reaffirmed 2026-10-06 as a hard project rule. **Relaxed on 2026-10-12 for testing only:** test runs may use `CGO_ENABLED=1` (above all `go test -race`), in addition to the `CGO_ENABLED=0` checks, never instead of them. The library, the examples, the spikes and apps never `import "C"` and always build with `CGO_ENABLED=0`. Test-only C code needs its own build tag, is never imported by non-test code, and the step that first adds it updates the no-cgo guard (§9, §11.1). | `CGO_ENABLED=0` builds and cross-compilation from any OS. More binding work: ObjC class registration, GObject subclassing, a fixed trampoline set, mirrored struct layouts (§6.1, §6.3, §7.2). Adds spike phase 0.5. Data races are caught by stress tests in every `CGO_ENABLED=0` run, and the race detector may run in addition as a cgo test run (§8.4, §11.1). |
| D6 | Module path | ✅ **`github.com/radix29/gogui`** | Used in `go.mod`, all examples and docs. |
| D7 | Minimum OS versions | ✅ **macOS 13+, Windows 10 1809+, GTK ≥ 4.14** (Ubuntu 24.04, Fedora 40+, Debian 13). amd64 and arm64. **Amended 2026-11-02 (user, applied in S4.2a):** the GTK floor was 4.10 (Fedora 39+); it is 4.14 for `GskPath` (paths and gradient paints, S4.2), so there is no cairo fallback, and the fractional `gdk_surface_get_scale` (4.12) is required. Pango ≥ 1.52 (what GTK 4.14 requires). §3, §6.3, §9 and S4.1's design updated. | Windows 10 falls back to square corners and no Mica. Feature detection is needed at runtime. Fedora 39 (GTK 4.12, end of life) is dropped. |
| D8 | Extra controls in v0.1 | ✅ **ScrollView and TextView** added to v0.1. TabView and file dialogs stay post-v0.1. | Phase 3 grows (scroll physics) and phase 5 grows (multi-line text). New `examples/notes`. |
| D9 | Bundled UI font on Windows/Linux (was O1) | ✅ **Inter** (OFL), bundled in `assets/fonts` since the phase 2 exit, with the theme's SF-like tracking table (decided in S3.1d, 2026-10-23). The platform font (Segoe UI Variable / Segoe UI; the system UI font on Linux) is only the fallback when no font is bundled or loading fails. | One font on Windows and Linux, metrically close to SF Pro; the drawn controls are tuned against Inter (S3.1t). Binaries carry the font files (the Windows shell since S2.5g, the Linux shell since S4.2j, through a cache file for fontconfig). §3 and §3.1 unchanged. |
| D10 | Accent color on Windows/Linux (was O2) | ✅ **Follow the OS accent** (Windows `DWM\AccentColor`, the portal `accent-color` on Linux), macOS blue (`driver.DefaultAccent`) when the OS has none. Every accent-derived color (focus ring, selection, pressed, inactive) is computed from `Appearance.Accent` by the theme's accent rules, never stored (decided in S3.1d, 2026-10-23). An **app-level override** is public API and comes with phase 6's OS-accent work (§4 unchanged until then). | The Win32 shell already reads the accent (S2.3); an override only has to replace `Appearance.Accent`. §5.2 unchanged. |

### Still open

| ID | Question | Current thinking | Decide by |
|---|---|---|---|
| O3 | Multi-window apps: menu strip in every window, or only in the "main" window? | Every titled window (as macOS menus apply to every window) | Phase 5 |
| O4 | License of gogui itself | BSD-3-Clause or MIT (compatible with Inter's OFL and purego's Apache-2.0) | Before first public commit |

---

## 13. Key Risks

| Risk | Impact | Mitigation |
|---|---|---|
| Text input/IME in drawn text fields is hard (TSF, GtkIMContext, selection, undo), and now also multi-line (TextView) | High | Delegate shaping and hit-testing to DirectWrite/Pango. Start with IMM32 on Windows. One shared editing core for TextField and TextView. Budget a whole phase for it. There is no GTK-widget fallback (D1), so start the text core early, in parallel with phase 3. |
| **R-purego:** no-cgo bindings for AppKit and GTK (class registration, vfuncs, struct passing, the limited callback count) have rough edges | High (re-rating pending, see `docs/spikes.md`) | Phase 0.5 spikes before committing. A fixed trampoline set, handle-based dispatch, and struct layout tests on amd64 and arm64. Track purego releases, and contribute fixes upstream if needed. |
| "Uncanny valley": looks almost like a Mac, but slightly off | High | Token-driven design tuned against measured references, golden and parity tests, and behaviour checklist §5.5. |
| Scrolling feel (momentum, rubber-banding) differs from macOS | Medium | Physics constants tuned against recordings of native `NSScrollView`. Fake-clock unit tests. Reduce Motion path. |
| Title-bar menu strip competes with the title and the drag area | Medium | Title truncation rules, hit-test tests, and a minimum drag area (§5.4.1). |
| Pure-Go COM bindings for D2D/DWrite/DComp are verbose | Medium | Bind only the methods we use. Borrow patterns from Gio and go-ole. Track reference counts in tests. |
| Smooth live resize on Windows (flicker, stretching) | Medium | DirectComposition with a flip-model swap chain; resize buffers synchronously in `WM_SIZE`. |
| GTK 4 missing or too old on the user's Linux system (runtime dlopen) | Medium | Version check at start-up with a clear error message naming the package. Document it in the packaging recipes (Flatpak includes GTK). |
| Linux compositor differences (no blur on GNOME, X11 vs Wayland) | Low | Degrade gracefully: solid translucent backgrounds, feature detection. |
| Font/asset licensing | Legal | Inter only, procedural drawing, no Apple assets (§3.1). |
| Accessibility is missing in v0.1 on Windows/Linux (D4) | Medium | The a11y tree is designed and tested from day one; the bridge is the first post-v0.1 item. |

---

## Appendix A. Q&A Log (2026-09-30)

Moved to `PLAN_done.md` §G. It only records how decisions D1–D8 came about (D9 and D10 were decided in S3.1d).
