# gogui examples

Each sub-folder is a standalone `package main` app that uses only the public
`gogui` package. The same source builds on macOS, Windows and Linux with no build
tags.

| Example | Shows | Target phase |
|---|---|---|
| [`hello/`](hello) | Minimal app: window, label, text field, default button | 0 (headless driver), 2.3 (Win32 window), 1+ |
| [`controls/`](controls) | Every v0.1 control, in all states. Used as the **parity gallery** to compare against native macOS | 1 / 3 / 5 |
| [`form/`](form) | Account form: `Form` layout, popup button, checkboxes, radio group, secure field, alert, Cancel/OK buttons (Escape/Return) | 5 |
| [`converter/`](converter) | Temperature converter: live two-way updates, slider + text fields, progress bar, goroutine → `gogui.Do` | 3 / 5 |
| [`notes/`](notes) | Plain-text editor: multi-line `TextView`, `ScrollView` sidebar, File/Edit/Format menus with shortcuts (title-bar menu strip on Windows/Linux) | 3 / 5 |

`controls` is the manual parity check. The golden-image tests (PLAN.md S3.6) use their own
fixed scene, `internal/aqua/gallery`, not an example; a theme change that moves the pinned
example frames in `testdata/aqua` is rewritten as described in ARCHITECTURE.md §9.4.

## Running

```sh
go run ./examples/hello
go run ./examples/controls
```

gogui uses **no cgo on any platform** (PLAN.md §9, decision D5), so every example builds with
only the Go toolchain, and cross-compiles from any OS:

```sh
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./examples/notes
```

Runtime prerequisites:

* **Windows**: Windows 10 1809+. Nothing to install.
* **macOS**: macOS 13+. Nothing to install.
* **Linux**: GTK ≥ 4.14 installed (`libgtk-4-1` on Debian/Ubuntu, `gtk4` on Fedora). No
  `-dev` packages or C compiler needed.

## Conventions for new examples

1. One folder per example, containing a single `main.go` (split into more files only if needed).
2. Use only the public `gogui` package, never an `internal/` package.
3. No platform-specific code. Use `gogui.KeyPrimary` for ⌘/Ctrl.
4. Touch widgets from goroutines only through `gogui.Do`.
5. Call `gogui.Run` from `main` and handle its error:
   `if err := gogui.Run(...); err != nil { log.Fatal(err) }`. It fails, for example,
   when GTK 4 is missing on Linux.
6. Add the example to the table above (`go build ./examples/...` in `scripts/native.sh`
   and `check.sh` picks it up).

> **Status:** the public API is complete (S0.5, PLAN.md §11.1) and was reviewed against
> `PLAN.md` §4 in S0.5k. All five examples compile on every platform, and
> `examples_test.go` in `gogui` checks their layouts. How they run depends on the OS:
>
> * **Windows** (since S2.3f): the aqua driver on the Win32 shell. Real windows open with
>   the custom chrome and can be moved, resized, snapped and closed; their content is
>   still empty until GPU rendering arrives (S2.4). Closing the last window quits the
>   app where the example does so.
> * **Linux** (phase 4 in progress, since S4.1d): the aqua driver on the GTK 4 shell.
>   Since S4.1f real windows open (client-side decorations without GTK's title bar, so
>   GTK's shadow and resize borders stay), can be resized at those borders and closed
>   through the window manager (Alt+F4). Since S4.1g they draw the title bar with the
>   traffic lights and the controls through the snapshot canvas: rounded rects are real
>   (gradient strokes too since S4.2b), and so are paths (glyphs such as checkmarks)
>   since S4.2c, shadows since S4.2d and images since S4.2e, while text
>   and backdrop blurs are outlined placeholder boxes until S4.2f–S4.2i.
>   Since S4.1h a drag on the title bar moves the window and a double-click there
>   zooms it. Since S4.1i the controls get the mouse (hover, press, drag, clicks);
>   since S4.1j they scroll (wheel steps, touchpad 1:1 with momentum and rubber-band);
>   since S4.1k they get the keyboard (shortcuts, Tab, typing with dead keys and
>   compose through GTK's simple input method; full IME in S5.3). Not run on Linux yet (no
>   machine; since S4.1l `scripts/native.sh` runs `hello` for 5 s on X11 and Wayland). Without GTK 4 `Run` returns the "GTK 4 is missing" error (also shown in a message box when
>   `zenity` or `kdialog` is installed).
> * **macOS** (until phase 1, S1.1): the headless driver. No windows are shown, the
>   program prints `gogui: headless driver, no windows are shown` and then blocks until
>   it is killed, because no one can close the window that would call `Quit`.
>
> These programs stay the
> **API design targets** from `PLAN.md` §4; API changes are reflected here.
