# Parity checklist for v0.1

This is the sign-off sheet of v0.1 (PLAN.md §11, the phase 6 exit), built in S6.3f from
S6.3a's release audit. It has one row per item of the v0.1 scope (PLAN.md §11) and one
per behaviour of PLAN.md §5.5, with a column per target, and it collects every open
check of the finished steps in one list (§4); a step finished later adds its checks there.
`bash scripts/native.sh` prints the path of this file at the end of every run.

**Sign-off** means the user works through the open checks of §4 on the machines they have,
then turns each cell they checked into ✔ with "user" and the date, and ticks the check in
§4. A failing check gets its own fix step in PLAN.md §11.1 first. There is no release and
no tag (user decision 2026-11-19), so nothing waits on the open cells but the phase exits
of §5.

## 1. Where v0.1 stands

* **Windows** runs the aqua driver over Direct3D 11, Direct2D, DirectComposition and
  DirectWrite. Claude checks it on the Windows 11 host (windows/amd64): `check.sh`,
  `native.sh`, the goldens and the tests on real windows. The looks and the hands-on
  checks are the user's. Windows 10 and windows/arm64 have no host here.
* **Linux** runs the aqua driver over GTK 4. It is built and cross-compiled, and its
  logic is tested on the host over fakes. No Linux log has come back yet, so every Linux
  cell is ◐ and waits on L1 first.
* **macOS** runs on the headless driver until phase 1 (S1.1–S1.5) builds the AppKit
  driver on a Mac. Until then the macOS column is ○ everywhere except the build rows.

## 2. Legend

| Mark | Meaning |
|---|---|
| ✔ | Checked, nothing open in this cell. *host*: tests on the Windows 11 host (real windows, Direct2D, DirectWrite, IMM32 where named). *fake*: host tests over the fake shell, the same on every OS. *user*: checked by the user (with the check's ID). |
| ◐ | Built, check open. The cell names what is checked so far and the open checks of §4 by ID. |
| ○ | Not built for this target. The cell names the step that builds it. |
| — | Does not apply to this target. |

The columns are the targets of D7 (PLAN.md §12): macOS 13+ native (AppKit), Windows 10
1809+, Windows 11, Linux X11 and Linux Wayland (GTK ≥ 4.14). A Windows check in §4 runs
on both Windows columns unless it names one. A Linux check runs on both X11 and Wayland.
Scales (100/150/200 % on Windows, 1 and 2 on Linux) and light/dark are part of every check
whose text names them.

## 3. The matrix

### 3.1 v0.1 scope (PLAN.md §11)

| Item | macOS native | Windows 10 | Windows 11 | Linux X11 | Linux Wayland |
|---|---|---|---|---|---|
| App lifecycle: start, `Do`/`DoSync`, quit | ○ S1.1 | ◐ W3 | ✔ host, fake; user C2 | ◐ fake; L1, L2 | ◐ fake; L1, L2 |
| `Window`: titled, closable, miniaturizable, resizable, min size, centred; drag, resize, snap | ○ S1.1 | ✔ user C5 | ✔ host; user C2, C3 | ◐ fake; L1, L2 | ◐ fake; L1, L2 |
| Key and inactive state of the window and its controls | ○ S1.1 | ◐ W14 | ◐ host, fake; W14 | ◐ fake; L1, L14, L15 | ◐ fake; L1, L14, L15 |
| `Label` | ○ S1.1 | ◐ W3 | ◐ host (goldens); W3 | ◐ fake; L1, L3 | ◐ fake; L1, L3 |
| `Button`: push, default, cancel | ○ S1.2 | ◐ W3 | ◐ host (goldens); W3 | ◐ fake; L3, L4 | ◐ fake; L3, L4 |
| `Checkbox` (mixed included) | ○ S1.2 | ◐ W3 | ◐ host (goldens); W3 | ◐ fake; L3, L4 | ◐ fake; L3, L4 |
| `RadioGroup` | ○ S1.2 | ◐ W3, W13 | ◐ host (goldens); W3, W13 | ◐ fake; L3, L12 | ◐ fake; L3, L12 |
| `TextField` | ○ S1.3 | ◐ W6, W7 | ◐ host, fake; W6, W7 | ◐ fake; L5, L6, L7 | ◐ fake; L5, L6, L7 |
| `SecureField` | ○ S1.3 | ◐ W6, W7 | ◐ host, fake; W6, W7 | ◐ fake; L6, L7 | ◐ fake; L6, L7 |
| `Slider` | ○ S1.3 | ◐ W4 | ◐ host (goldens), fake; W4 | ◐ fake; L3, L4 | ◐ fake; L3, L4 |
| `ProgressBar`: determinate and indeterminate | ○ S1.3 | ◐ W4 | ◐ host (goldens), fake; W4 | ◐ fake; L3 | ◐ fake; L3 |
| `PopUpButton` | ○ S1.3 | ◐ W1, W10 | ◐ host (popups on real windows); W2, W10 | ◐ fake; L1, L10 | ◐ fake; L1, L10 |
| `Separator`, `Box` | ○ S1.2 | ◐ W3 | ◐ host (goldens); W3 | ◐ fake; L3 | ◐ fake; L3 |
| `ScrollView` | ○ S1.5 | ◐ W9 | ◐ host, fake; W9 | ◐ fake; L1, L9 | ◐ fake; L1, L9 |
| `TextView` (multi-line, plain text) | ○ S1.5 | ◐ W8 | ◐ host (1 MB benchmarks over DirectWrite); W8 | ◐ fake; L1, L8 | ◐ fake; L1, L8 |
| `VStack` / `HStack` / `Spacer` / `Form` | ○ S1.1 | ◐ W3 | ✔ fake (layout tests of every example), host (goldens) | ◐ fake; L1 | ◐ fake; L1 |
| `MenuBar`: App, Edit and custom menus, in the title bar | ○ S1.4 | ◐ W11 | ◐ host (strip on real windows), fake; W11 | ◐ fake; L1, L11 | ◐ fake; L1, L11 |
| Modal `Alert`: sheets, app-modal, About | ○ S1.4 | ◐ W12 | ◐ host (modal surfaces on real windows), fake; W12 | ◐ fake; L1, L12, L13 | ◐ fake; L1, L12, L13 |
| Light and dark mode, live switch | ○ S1.4 | ◐ W3 | ◐ host (goldens); user C3; W3 | ◐ fake; L1, L15 | ◐ fake; L1, L15 |
| Accent color: the OS's, live, and `App.SetAccentColor` | ○ S1.4 | ◐ W18 | ◐ host, fake; W18 | ◐ fake; L15 | ◐ fake; L15 |
| HiDPI | ○ S1.1 | ◐ W3 | ✔ user C1 (100/150/200 %, two monitors) | ◐ fake; L3 | ◐ fake; L3 |
| No cgo | ✔ host (`check.sh` builds darwin/amd64 and arm64) | ✔ host | ✔ host | ✔ host (linux builds) | ✔ host (linux builds) |

### 3.2 Behaviour (PLAN.md §5.5)

| Behaviour | macOS native | Windows 10 | Windows 11 | Linux X11 | Linux Wayland |
|---|---|---|---|---|---|
| Focus ring that animates in; Tab and Shift-Tab | ○ S1.2 | ◐ W5, W13 | ◐ fake; W5, W13 | ◐ fake; L5, L12 | ◐ fake; L5, L12 |
| Default button (Return), cancel (Escape), Space on the focused button | ○ S1.2 | ◐ W13 | ◐ fake; W13 | ◐ fake; L5, L12 | ◐ fake; L5, L12 |
| Pressed state follows the pointer; a release outside does not click | ○ S1.2 | ◐ W3 | ◐ fake; W3 | ◐ fake; L4 | ◐ fake; L4 |
| Text editing semantics: words, lines, selections, clipboard, undo, caret blink from the OS | ○ S1.3 | ◐ W6 | ◐ host (clipboard, timer, blink setting), fake; W6 | ◐ fake; L1, L5, L6 | ◐ fake; L1, L5, L6 |
| IME composition (CJK), inline preedit, candidate window at the caret | ○ S1.3 | ◐ W7, W8 | ◐ host (IMM32 on real windows), fake; W7, W8 | ◐ fake; L1, L7 | ◐ fake; L1, L7 |
| Pop-up button menus: placement over the item, mouse, keys, type-select, pick blink | ○ S1.3 | ◐ W1, W10 | ◐ host, fake; W2, W10 | ◐ fake; L10 | ◐ fake; L10 |
| Tooltips after a 1 s rest, the 500 ms grace | ○ S1.2 | ◐ fake; W24 | ✔ fake; user W24 (2026-11-19) | ◐ fake; L19 | ◐ fake; L19 |
| Title-bar menus: mouse, hover-switching, accelerators, Alt/F10 and arrows | — (the macOS menu bar, S1.4) | ◐ W11 | ◐ host, fake; W11 | ◐ fake; L11 | ◐ fake; L11 |
| Alerts by keys: Return, Escape, Primary+., Tab between buttons; one at a time | ○ S1.4 | ◐ W12, W13 | ◐ fake; W12, W13 | ◐ fake; L12 | ◐ fake; L12 |
| `form` by keyboard only (the phase 5 exit) | ○ S1.4 | ◐ W13 | ◐ fake (driven by keys in tests); W13 | ◐ fake; L12 | ◐ fake; L12 |
| Animations; Reduce Motion, also switched mid-animation | ○ S1.4 | ◐ W16 | ◐ fake; W16 | ◐ fake; L15 | ◐ fake; L15 |
| Reduce Transparency: opaque menus, no backdrop | ○ S1.4 | ◐ W17 | ◐ host, fake; W17 | ◐ fake; L15 | ◐ fake; L15 |
| Increased contrast (S6.1d–S6.1e) | ○ S1.4 | ◐ W15 | ◐ host (goldens), fake; W15 | ◐ fake; L1, L15 | ◐ fake; L1, L15 |
| Scrolling: overlay scrollers that fade, rubber band, momentum | ○ S1.5 | ◐ W9 | ◐ fake (physics tests); user C4; W9 | ◐ fake; L9 | ◐ fake; L9 |
| Window: centred title, zoom on the green light, Snap Layouts | ○ S1.1 | ◐ user C5 (no Snap Layouts on 10); W25 | ◐ host; user C2; W25 | ◐ fake; L2 | ◐ fake; L2 |
| First click into an inactive window only activates it (not the lights and strip titles) | ○ S1.1 | ◐ W14 | ◐ host, fake; W14 | ◐ fake; L1, L14 | ◐ fake; L1, L14 |
| Live resize without flicker or stale strips | ○ S1.1 | ✔ user C5 | ✔ user C3 | ◐ L2 | ◐ L2 |
| Text: crisp, in Inter (D9), at every scale | — (the system font) | ◐ W3 | ✔ user C1 | ◐ fake; L3 | ◐ fake; L3 |

### 3.3 Performance and goldens (phase 3, 4 and 6 exits)

| Item | macOS native | Windows 10 | Windows 11 | Linux X11 | Linux Wayland |
|---|---|---|---|---|---|
| Idle CPU ≈ 0 % (`hello`, `form`, `notes` under 1 % of one core) | ○ S1.1 | ◐ W19 | ✔ host (`native.sh`, 0 to 0.16 %); W19 | ◐ L16, L17 | ◐ L16, L17 |
| Frame pacing at 60 Hz, every frame inside 8.3 ms | ○ S1.1 | ◐ W20 | ✔ host (one monitor: `PACING-TOTAL` within 2 %); W20 | ◐ L16 | ◐ L16 |
| 120 Hz and 2× | ○ S1.1 | ◐ W21 | ◐ W21 | ◐ L16 | ◐ L16 |
| Gallery goldens (twelve PNGs, light, dark, inactive, contrast) | — (references, S1.5) | ◐ W22, W23 | ✔ host (`native.sh`); W22 optional | ◐ L1, L18 (S4.3j) | ◐ L1, L18 (S4.3j) |
| The gallery against the macOS references (phase 3 exit) | ○ M6 | ○ M7 | ○ M7 | ○ M7 | ○ M7 |
| "GTK missing" error path (stderr and a message box) | — | — | — | ✔ fake (report and version tests); G2 optional | ✔ fake; G2 optional |

### 3.4 `native.sh` logs

| Target | State |
|---|---|
| windows/amd64 | ✔ host, run by Claude in every wrap-up step (last full run S6.3f, 2026-11-19) |
| windows/arm64 | ◐ W23 |
| linux/amd64 | ◐ L1 |
| linux/arm64 | ◐ L1 (if a machine is available) |
| darwin/arm64 | ◐ M1 |
| darwin/amd64 | ◐ M1 |
| any machine with a C compiler: the cgo race run | ◐ A1 |

## 4. The checks

Each check names where it came from in brackets (the steps that listed it; the code and
ARCHITECTURE.md hold the details). A later wrap-up that restated a check is folded into it.

### 4.1 macOS (needs a Mac)

- **M1** `bash scripts/native.sh` on an arm64 Mac and an amd64 Mac, from a logged-in GUI
  session; bring back `native-logs/darwin-*.log`. It has `go test ./...` (the ffi memory,
  `BindAddr` and `Bind` guard tests), the callback measurements (`TestCallbackBudget`
  free/taken/total/`MaxCallbacks`, `BenchmarkCallbackCall` direct and qsort) and both
  phases of the `spike/appkit -selftest` check lines, `[NSThread isMultiThreaded] on the
  worker = …` included. Feeds S0.10c_2b. [S0.7, S0.8, S0.10, S0.10c_2b]
- **M2** `go run ./spike/appkit` on both Macs: the window comes to the front with a Dock
  icon and the menu-bar name; the label is top-left and the button top-right; clicks and
  ticks count up, the button stays top-right on resize, and closing quits. Feeds
  S0.10c_3. [S0.8, S0.10c_3]
- **M3** After S1.1: `examples/hello` opens a native AppKit window with its label. [S1.1]
- **M4** After S1.3: `hello` and `converter` work natively (text fields, slider, progress
  bar, pop-up). [S1.3]
- **M5** After S1.4: `controls` and `form` work natively (the main menu with the app and
  Edit menus and shortcuts, NSAlert, a live appearance switch). [S1.4]
- **M6** After S1.5: `notes` works natively (NSScrollView, NSTextView), and the gallery
  reference PNGs are captured into `docs/parity/` at 1× and 2×, light and dark. [S1.5]
- **M7** After M6: the Windows gallery goldens reviewed against the macOS references;
  S3.1t then tunes the tokens from the measurements and regenerates the goldens. This is
  the phase 3 exit. [S3.6e, S3.1t]

### 4.2 Windows

- **W1** Windows 10: the popups (pop-up button menus, strip menus) have an opaque fill
  and the drop shadow, light and dark. [S5.5d, S5.5m]
- **W2** Windows 11 22H2 or later: is the popups' Acrylic blur visible, or is it the flat
  fallback colour (the host readback can't tell)? The small rounded corners, light and
  dark; opaque with Transparency effects off, no backdrop in high contrast. If it is the
  flat fallback, a fix step follows. [S5.5d, S5.5m]
- **W3** `hello`, `controls` and `form` in light and dark, key and inactive: push buttons
  (default and cancel), checkboxes (mixed included), radios, box, separator; a press
  dragged off a button and released outside does not click. [S3.3e, S3.3h]
- **W4** `converter` and `controls`: dragging the slider, its arrow, Page, Home and End
  keys, the sync bar following it, the indeterminate bar moving; light and dark, key and
  inactive. [S3.4d, S3.4g]
- **W5** *(optional)* Tab through `form` and see the focus ring. [S3.2d]
- **W6** The fields of `hello`, `form`, `converter` and `controls`: typing; moves by
  character, word (Alt) and line (Ctrl); Shift selections; click, Shift-click, drag past
  either edge, double- and triple-click; Ctrl+C/X/V, also with another app; Ctrl+Z and
  Ctrl+Shift+Z; the I-beam; the caret blink (solid after about 10 s, none when inactive);
  the SecureField shows bullets and refuses copy; Return reaches the default button.
  [S5.2i, S5.2j]
- **W7** Japanese (Microsoft IME) and Chinese (Microsoft Pinyin) in `form` and
  `controls`: the inline preedit underlined, the converting clause thicker; the candidate
  window under the caret, also at 150 % and after a move to a monitor with another scale;
  Space converts, Return commits, Esc cancels, a click elsewhere commits; no IME in the
  SecureField; one undo removes one commit. [S5.3g, S5.3k]
- **W8** `go run ./examples/notes <1 MB file>`: typing in the middle without lag; wheel,
  touchpad and scroller (the knob settles as the idle pass runs, no visible jump); Page
  Up/Down, Ctrl-Home/End; drag-select with auto-scroll past the top and bottom; double-
  and triple-click; Copy/Cut/Paste, undo; Japanese and Chinese IME inline with the
  candidate window at the caret. [S5.4j, S5.4k, S5.4l]
- **W9** The `notes` sidebar: the precision touchpad 1:1 with momentum and rubber band at
  both ends; smooth wheel line steps; the overlay scrollers fade and widen on hover; knob
  drag; track click; the live switch to legacy scrollers through "Always show
  scrollbars"; Page Up/Down and Home/End; Tab scrolls the focused checkbox into view;
  light and dark. [S3.5h]
- **W10** The pop-up buttons of `controls` and `form`: each opens over its selected item
  with the titles aligned, at 100/150/200 %; near each screen edge it slides; a tall menu
  is cut and scrolls with the arrows and the wheel; across two monitors of different
  scales; click then click, press-drag-release; Space/Up/Down open it, then arrows,
  Home/End, type-select (since W12 the same letter again steps to the next item with it),
  Return, Escape; the pick blink. [S5.5m]
- **W11** The title-bar strip of `controls` and `notes`, at 100/150/200 %, on two monitors
  and with two windows: click, hover-switching, press-drag-release; Alt and F10 focus the
  strip; arrows, Return, Escape; shortcuts with the menus closed (Ctrl+Shift+D; Ctrl+N,
  Ctrl++ and Ctrl+-, also on a German layout); Alt+Space and Alt+F4 still work; the window
  drags between the titles and the window title; windows at minimum width. [S5.6j, S5.6k]
- **W12** The alerts of `form` and `controls`, at 100/150/200 % and on two monitors:
  sheets (the slide, a fade with Reduce Motion; the content refuses input; the title bar
  still moves the window; the close light is disabled); a window narrower than the panel
  (the alert goes into a modal window it owns); two windows; About (app-modal, the other
  windows disabled, only Quit in the menus, Alt+F4 answers Cancel). [S5.7g, S5.7i]
- **W13** `form` from start to end by keyboard only: Tab, typing; the pop-up button by
  Space and arrows, the radios by arrows, the checkboxes by Space; Return; the alerts by
  Return. [S5.7h, S5.7i]
- **W14** `controls` and `form`, two windows side by side. In the inactive one: accent
  fills grey, the default button plain, no focus ring or caret, a grey selection. A first
  click into the inactive window only activates it, while a strip title and the traffic
  lights act at once. [S6.1a, S6.1c, S6.1i]
- **W15** A high-contrast theme switched on while they run: darker borders, outlines
  around the accent fills and the slider track. [S6.1e, S6.1i]
- **W16** Animation effects off (Settings > Accessibility > Visual effects), also switched
  in the middle of an animation: no sheet slide but a fade; the focus ring, scrolls and
  the menu blink go without animation. [S6.1f, S6.1i]
- **W17** Transparency effects off: opaque menus, no backdrop. [S6.1f, S6.1i]
- **W18** The OS accent changed live, also to a light colour (dark labels on it), and the
  four accent items of `controls`' View menu. [S6.1g, S6.1i]
- **W19** Task Manager with `controls`, `form` and `notes` open and left alone: `form` and
  `notes` at about 0 %, `controls` only its indeterminate bar's loop. [S6.2f]
- **W20** `native.sh` with the four monitors reconnected: `PACING-TOTAL` within 2 %, and
  do the `PACING-WORK` lines show that S6.2e's waitable pacing removes S6.2d's misses?
  [S6.2d, S6.2e, S6.2f]
- **W21** `native.sh` on a 120 or 144 Hz monitor (`period=` in `PACING-RESULT`, the
  8.3 ms budget), and on a 200 % monitor (2×). [S6.2d, S6.2f]
- **W22** *(optional)* A look at the twelve gallery PNGs on Windows. [S3.6e, S6.1h]
- **W23** `bash scripts/native.sh` on windows/arm64: the arm64 float-call trampolines,
  the callback budget, the layout cases, IMM32, the TextView benchmarks, the popups, the
  strip and modal steps, and the twelve goldens (do they hold on another WARP build?). Since
  review W16 (P2) windows/arm64 is the only target where the d2d and dwrite FLOAT methods
  go through purego (`com.Bind`): their tests there are its only native check.
  [S0.10c_2b, S2.4, S3.6d, S3.6e, S5.3k, S5.4l, S5.5m, S5.6k, S5.7i, S6.1i, review W16]
- **W24** The tooltips of `controls` (Default, Disabled, the radio group, the slider), at
  100/150/200 %, light and dark: the tip after a 1 s rest, below the pointer and inside
  the screen near its edges; the next one at once when the pointer moves to another
  tipped control within 500 ms; it goes on a press, on a key and when the pointer
  leaves; the disabled button shows its tip too; the panel's text, border and shadow.
  [S5.5l, S6.3h] *Reported done 2026-11-19 on Windows 11:* the tips of all four (the
  disabled button's included, none on the others), light and dark, 100/150/200 %, kept
  on the screen, the grace (the next tip at once), hiding on a press and on a key. Open:
  Windows 10.
- **W25** The snap fix of S6.2g: with `controls` running (its bar animating), drag the
  window to the left, right and top edges until the snap preview shows, then (a) let go
  there: it snaps; (b) pull back with part of the window still off screen and let go:
  it snaps if the preview still shows (as a WinForms window does), else stays where it
  was dropped; either way the preview goes, the app reacts at once, and Alt+Tab
  or a click elsewhere does not move it back. Then drop it partly off the bottom and off
  the right and drag it back, and resize it from the top edge: the mouse never freezes.
  Also on a second monitor and at 150 %. [S6.2g] *Reported 2026-11-19 on Windows 11:* the
  re-drags from the bottom and the right and the top-edge resize, no freeze. Open: the
  snap steps (a), (b), a second monitor, 150 %, Windows 10.

### 4.3 Linux (each on X11 and on Wayland)

- **L1** `bash scripts/native.sh` on linux/amd64, and on arm64 if available, under xvfb,
  headless weston and the desktop session; bring back `native-logs/linux-*.log` and
  `native-logs/golden-*/`. The log feeds S0.10c_2b (the R-purego re-rating, the callback
  budget), S4.3j (the golden calibration) and every built-only gtk4 test since S4.1b.
  Claude reads in it: `TestLoad`, `TestAttrType`, `TestClipboardRoundTripGTK`,
  `TestAfterRealGTK`, `TestGtkSettingsNotify`, `TestPortalSessionBus*`,
  `TestTextPangoLineMetrics`, `TestSurfaceGTKKeyboard`, `TestSurfaceGTKTextInput`,
  `TestSurfaceGTKNew`, `TestPopupGTK`, `TestModalGTK`, `TestSurfaceGTKFirstClick`; the
  `*-RESULT` lines (BACKEND, RESIZE, CANVAS, POINTER, SCROLL, KEY, TEXTINPUT, POPUP,
  MODAL, FIRSTCLICK, SETTINGS, GOLDEN-RESULT and GOLDEN-SWEEP with their diff PNGs); the
  logged IM environment variables. [S0.10c_2b, S4.1d–S4.1l, S4.2k, S4.3g–S4.3j,
  S5.2b–S5.2d, S5.3k, S5.4l, S5.5m, S5.6k, S5.7i, S6.1c, S6.1i]
- **L2** `hello` on a real X11 session (say whether it was real X11 or XWayland) and on a
  Wayland session (mutter and/or kwin): the window opens with the traffic lights; hover
  and clicks react; a title-bar drag moves the window, a click without a drag doesn't; a
  double-click zooms and restores it; the lights don't move it; a border resize keeps the
  content filling the window without flicker; typing works; closing quits.
  [S0.10c_3, S4.1h, S4.1l]
- **L3** `hello`, `controls`, `form` and `notes`: real text in Inter, shadows, rounded
  controls, crisp at scale 1 and 2. [S4.2k]
- **L4** `controls`: hover and press on buttons, checkboxes and the slider (a drag outside
  the window and back); double- and triple-clicks; the traffic lights (close, minimize,
  zoom); the controls still react normally after a title-bar drag; right-click and the
  back/forward buttons arrive. [S4.1i]
- **L5** `form` and `controls` by keyboard: typing with dead keys (US-intl or German) and
  compose; Tab and Shift+Tab; Space on a focused button; Ctrl+A/C/V with no stray text;
  held keys repeat; Alt+F4 closes the window through the window manager. [S4.1k]
- **L6** The field checks of W6 (typing, moves, selections, clipboard, undo, caret blink,
  the secure field). [S5.2j]
- **L7** IME typing in `form` with Mozc or Anthy and with Pinyin, on GNOME (IBus) and on
  KDE or another Fcitx 5 desktop: the preedit, the candidate window at the caret, commit,
  cancel, a click commits, no IME in the SecureField, one undo removes one commit.
  [S5.3j, S5.3k]
- **L8** `notes` with a 1 MB file: typing, scrolling, drag-select, IME. [S5.4l]
- **L9** `notes`: wheel steps, Shift+wheel scrolls sideways; the touchpad scrolls 1:1,
  then glides with momentum and rubber-bands at the edges; a click or a new touch stops
  the glide; nested views keep the latched view. [S4.1j]
- **L10** The pop-up menus of `controls` and `form`: the checks of W10. A menu may flip
  there when it fits nowhere else. [S5.5m]
- **L11** The strip of `controls` and `notes`: the checks of W11. Expected degradation: a
  press on another title closes the open menu, with no press-drag-release from there.
  [S5.6e, S5.6k]
- **L12** The alerts of `form` and `controls`, and `form` by keyboard only: the checks of
  W12 and W13. [S5.7i]
- **L13** Where the window manager (X11, mutter) places the app-modal alert, and whether
  it gets the key. [S5.7i]
- **L14** The first click into an inactive window, under the X11 window manager and under
  mutter and kwin. [S6.1c, S6.1i]
- **L15** GNOME's or KDE's settings switched while `controls` and `form` run: dark style,
  accent colour, high contrast, reduce animations; the inactive window too.
  [S4.3h, S6.1g, S6.1i]
- **L16** From L1's log: the `BenchmarkFrameNative`, `PACING-RESULT`/`PACING-TOTAL` and
  `IDLE-RESULT` lines from xvfb, weston and the desktop session (a high-refresh monitor
  if there is one). The Linux idle limit comes from these numbers. [S6.2c, S6.2d, S6.2f]
- **L17** `top` with `controls`, `form` and `notes` open and left alone. [S6.2f]
- **L18** After S4.3j calibrates and its follow-ups regenerate the twelve shared PNGs
  (TextField, SecureField, TextView, PopUpButton, the strip and the alert panel added,
  with the inactive and contrast variants): `native.sh` on Linux again to re-check the
  Linux goldens. This is the phase 4 exit. [S4.3j and its follow-ups from S5.2j, S5.4l,
  S5.5m, S5.6k, S5.7i, S6.1i]
- **L19** The tooltips of `controls`: the checks of W24 at scale 1 and 2; where the
  compositor places the tip near a screen edge. [S5.5l, S6.3h]

### 4.4 Any OS

- **A1** A cgo test run, `CGO_ENABLED=1 go test -race -count=1` on the concurrency
  packages (`internal/mainthread`, `aqua`, `shell/win32`, others as appropriate), on a
  machine with a C compiler. Extra to the `CGO_ENABLED=0` checks (D5). [D5, S5.7i]
- **A2** Sign off this file: fill in the cells of the checks done. [S6.3f, phase 6]

### 4.5 Gaps found while building this list

G1, that no example set a tooltip, was closed by S6.3h (2026-11-19): `controls` sets four,
`TestControlsTooltipsOnAqua` shows each over the fake shell, and W24 and L19 are the hand
checks.

- **G2** *(optional)* Start an example on a Linux machine without GTK 4, or with one
  older than 4.14: the error on stderr names the library and the packages to install,
  and a `zenity`/`kdialog` box shows it when one is installed. No Result named this
  check; the report and the version check are host-tested. [S4.1d, S4.2a]

### 4.6 Closed by the user

- **C1** ✔ *2026-10-22* `hello` and `controls` at 100, 150 and 200 % and after a move
  between two monitors: the text sharp, grayscale, not clipped, in Inter (the phase 2
  exit, Windows 11). [S2.5g]
- **C2** ✔ *(reported 2026-11-19)* The window by hand: drag by the top 28 pt, resize
  from every edge, Win+arrows, edge snapping, Snap Layouts on the green light, moves
  between monitors of different scales; Alt+F4 and taskbar close quit the app. [S2.3]
- **C3** ✔ *(reported 2026-11-19)* Live resize of `hello` and `controls` from every edge
  and corner at 100 % and another scale, without flicker, stretching, lag or stale
  strips; the background follows a live light/dark switch. [S2.4c, S2.4d, S2.4f]
- **C4** ✔ *(reported 2026-11-19)* The input hand checks: a high-resolution wheel, the
  precision touchpad, dead-key and non-Latin keyboard layouts. [S2.3d, S2.3e]
- **C5** ✔ *(reported 2026-11-19)* Windows 10 (the D7 fallback): square corners and no
  Mica, the window chrome still correct; the frame clock's standard-timer hold paces
  without stutter. [S2.3, S2.4f]

## 5. Phase exits

| Phase | Exit criterion | State | Waits on |
|---|---|---|---|
| 0 | `go test` green, cross-compile matrix green, API reviewed | ✔ 2026-10-09 | — |
| 0.5 | All three spikes work; R-purego re-rated | ◐ built and checked on Windows only | M1, M2, L1, L2 (S0.10c_2b, S0.10c_3) |
| 1 | `hello`, `controls`, `form`, `notes` run natively on macOS; reference screenshots captured | ○ S1.1–S1.5 not built | A Mac: M3, M4, M5, M6 |
| 2 | Empty aqua window with traffic lights, drag/resize/snap, crisp text at 100/150/200 % | ✔ 2026-10-22 (C1; C2–C5 since) | — (W23 for windows/arm64) |
| 3 | The gallery on Windows passes visual review against the macOS references; goldens in `native.sh`; scroll physics tests green | ◐ goldens and physics ✔, the review open | M6, M7 (S3.1t) |
| 4 | The same gallery renders on Linux (Wayland and X11) identically to Windows (shared goldens) | ◐ S4.1–S4.3i built only, S4.3j not started | L1 → S4.3j → L18 |
| 5 | `form` fully usable by keyboard only; CJK IME works; `notes` edits a 1 MB file smoothly | ◐ met on the host over the fake shell (S5.7i) | W6–W8, W13; L6–L8, L12 |
| 6 | Parity checklist signed off (no release or tag: S6.3g dropped, 2026-11-19) | ◐ S6.1, S6.2 and S6.3 built | A2 |
