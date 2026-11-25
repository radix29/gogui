# gogui examples

Each sub-folder is a standalone `package main` app that uses only the public
`gogui` package. The same source builds on macOS, Windows and Linux with no build
tags. The examples are also the API design targets of `PLAN.md` §4: an API change
updates them in the same step.

| Example | Shows | Windows | Linux | macOS |
|---|---|---|---|---|
| [`hello/`](hello) | Minimal app: window, label, text field, default button | Runs | Built | Headless |
| [`controls/`](controls) | Every v0.1 control in its states, a status line reporting each action, a warning alert, tooltips on four controls, and a menu bar (App, Edit, View) whose View menu toggles the appearance and sets the app's accent color (`App.SetAccentColor`). The **parity gallery** to compare against native macOS | Runs; the screenshots in the root `README.md` are this window | Built | Headless |
| [`form/`](form) | Account form: `Form` layout, popup button, radio group, checkboxes, secure field, Cancel/OK buttons (Escape/Return), validation alerts as sheets | Runs | Built | Headless |
| [`converter/`](converter) | Temperature converter: live two-way updates between two text fields and a slider, a progress bar fed by a goroutine through `gogui.Do` | Runs | Built | Headless |
| [`notes/`](notes) | Plain-text editor: multi-line `TextView`, a `ScrollView` sidebar, File/Edit/Format menus with shortcuts, a word count on a goroutine; opens a file given as its argument | Runs | Built | Headless |
| [`gallery/`](gallery) | The scene of the golden-image tests (`internal/aqua/gallery`) through the public API, laid out with stacks: labels in their colors and fonts, push buttons in every control size, checkboxes in their states, a radio group in a box, sliders (one with tick marks), a progress bar, an indeterminate bar, a spinner and a scroll view scrolled off both axes | Runs | Built | Headless |

What the three states mean today (the open checks of every OS are collected in
[`docs/parity.md`](../docs/parity.md), the parity checklist):

* **Runs** (Windows 10 1809+ and 11, amd64 and arm64): the aqua driver on the Win32 shell,
  drawn on the GPU with the title-bar menu strip, the traffic lights, light and dark and
  the OS accent. Checked on windows/amd64: `scripts/native.sh` runs every example and
  measures its idle CPU (`hello`, `form` and `notes` below 1 % of one core; `controls`
  repaints for its looping indeterminate bar, `gallery` for its bar and spinner, and
  `converter` for its sync bar, ten times a second, by design). Windows/arm64 is built
  but not yet run on a machine; on Windows 10 the user has checked the window chrome,
  its other checks are open.
* **Built** (GTK ≥ 4.14, Wayland and X11): the same aqua driver on the GTK 4 shell, so the
  same windows, controls, menus and keys as on Windows. Cross-compiled and tested on the
  fake shell, not yet run on a Linux machine; `native.sh` on Linux runs each example for
  5 s under X11 and Wayland and measures its idle CPU. Without GTK 4 `Run` returns an
  error naming what is missing, shown in a message box too when `zenity` or `kdialog` is
  installed.
* **Headless** (macOS 13+, until phase 1 builds the AppKit driver): the program prints
  `gogui: headless driver, no windows are shown` and then blocks until it is killed,
  because no one can close the window whose `OnClose` would call `Quit`.

On every OS the six trees are also built through the public API by the example tests of
`gogui` (`examples_test.go`, `aqua_test.go`, `examples_keys_aqua_test.go`,
`examples_menu_aqua_test.go`), which check their layouts on the headless driver and, on
the aqua driver over the fake shell, pin their frames in `testdata/aqua`, open their
alerts, drive `form` and `controls` by the keyboard alone and test the menus of `controls`
and `notes`. The golden-image tests (PLAN.md S3.6) use their own fixed scene,
`internal/aqua/gallery`, not an example; a theme change that moves the pinned example
frames is rewritten as described in ARCHITECTURE.md §9.4.

## Running

```sh
go run ./examples/hello
go run ./examples/controls
go run ./examples/form
go run ./examples/converter
go run ./examples/notes [file]
go run ./examples/gallery
```

gogui uses **no cgo on any platform** (PLAN.md §9, decision D5), so every example builds with
only the Go toolchain, and cross-compiles from any OS:

```sh
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./examples/notes
```

With `GOGUI_DEBUG=1` set, a toolkit call off the UI thread panics with the method's name,
and on Windows and Linux every frame slower than 8.3 ms is logged (`native.sh` runs the
examples this way on Linux).

Runtime prerequisites:

* **Windows**: Windows 10 1809+. Nothing to install.
* **macOS**: macOS 13+. Nothing to install.
* **Linux**: GTK ≥ 4.14 installed (`libgtk-4-1` on Debian/Ubuntu, `gtk4` on Fedora). No
  `-dev` packages or C compiler needed.

## Keys and menus

Primary is ⌘ on macOS and Ctrl on Windows and Linux (`gogui.KeyPrimary`). On Windows and
Linux every window behaves as macOS with keyboard navigation turned on: Tab and Shift-Tab
reach every control, a focused button, checkbox or popup button answers Space, a radio
group moves its selection with the arrows, and a Tab into a text field selects its text.
Text fields and the text view edit with the macOS keymap (Alt/Option-arrows by word,
Primary-arrows to the line ends, Primary-A/C/V/X/Z, Shift-Primary-Z), with the IME and the
OS clipboard. A menu strip is reached by a lone Alt or F10, then the arrows, Return and
Escape. Each alert answers Return (its default button), Escape (its Cancel button, or
its only one) and Space (the focused button).

* **`hello`**: no menu bar, so on Windows and Linux no strip (decision D11). Return clicks Greet, which greets the name typed.
  Closing the window quits.
* **`controls`**: *App* (About, Hide, Hide Others, Show All, Quit Primary-Q), *Edit* (Undo
  Primary-Z, Redo Shift-Primary-Z, Cut, Copy, Paste, Delete, Select All), *View* (Toggle
  Appearance Shift-Primary-D; System, Purple, Green and Yellow Accent, the light ones
  turning the labels on the accent dark). On Windows and Linux Hide minimizes the app's
  windows, Show All restores them, Hide Others stays disabled and About shows an
  app-modal alert. Return clicks Default; Return in the text field also reports the
  submitted text. Show Alert… opens a sheet: Return deletes, Escape cancels, and the
  status line reports the answer. Default, Disabled, the radio group and the slider have
  tooltips (a 1 s rest; the disabled button shows its tip too). Closing the window quits.
* **`form`**: no menu bar. Tab walks name, e-mail, password, role, plan, the two
  checkboxes and the buttons; Space opens the role menu (Up/Down, then Return). Create
  Account stays disabled until the terms checkbox is on; Return then submits: an empty
  name or an e-mail without `@` shows a critical sheet, otherwise a success sheet whose
  OK closes the window. Escape (Cancel) closes the window, and closing it quits.
* **`converter`**: no menu bar. Typing a number in either field updates the other and the
  slider; the slider (dragged, or its arrows when focused) updates both fields. The
  progress bar fills every 5 s from a goroutine, and the label shows the time of the last
  sync. Closing the window quits.
* **`notes`**: *App* as above, *File* (New Primary-N, which empties the text), *Edit* (the
  standard items, acting on the focused text view), *Format* (Bigger Primary-+, Smaller
  Primary--). The text view has the focus at start; Return and Tab insert, Shift-Tab
  leaves it, Ctrl-Tab moves on as well, Page Up/Down move the caret by a page. The
  sidebar scrolls by wheel and touchpad (momentum, rubber-band). `notes file` opens the
  file's text (it is not saved back). Closing the window quits.
* **`gallery`**: no menu bar and no actions of its own; every control answers its usual
  keys (Tab walks the enabled controls, starting from the focused Focused checkbox), and
  the scroll view scrolls by wheel and touchpad. Closing the window quits.

## Conventions for new examples

1. One folder per example, containing a single `main.go` (split into more files only if needed).
2. Use only the public `gogui` package, never an `internal/` package.
3. No platform-specific code. Use `gogui.KeyPrimary` for ⌘/Ctrl.
4. Touch widgets from goroutines only through `gogui.Do`.
   A goroutine that outlives its window checks first: the widgets of a closed window are
   destroyed, and a destroyed widget panics when used (D13). A panel the example swaps
   out for good is freed with `Destroy()`.
5. Call `gogui.Run` from `main` and handle its error:
   `if err := gogui.Run(...); err != nil { log.Fatal(err) }`. It fails, for example,
   when GTK 4 is missing on Linux.
6. Colors the example picks itself are semantic (`ColorSecondaryLabel`,
   `ColorControlAccent` …), so they follow light and dark and the accent, the app's
   own from `App.SetAccentColor` included (PLAN.md §4.2).
7. Register it everywhere the six are listed; `TestExamplesListed` (in `internal/repocheck`)
   fails until it is: a row in the table above and an entry under *Keys and menus*; its tree in
   `examples_test.go` (built as its `main.go` builds it, which `TestExampleTreesMatch`
   checks; add its builder to `treeBuilders` there) and an entry in the `examples` list of
   `aqua_test.go`, with its frames pinned by
   `CGO_ENABLED=0 go test -run TestExamplesOnAqua -update .`. `scripts/native.sh` (which
   runs it and measures its idle CPU) and `go build ./...` in `check.sh` pick it up by
   themselves.
