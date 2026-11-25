# gogui

A Go GUI toolkit for desktop apps that run on **macOS, Windows and Linux** from one
portable codebase, with no cgo. On macOS apps are meant to use real AppKit controls; on
Windows and Linux they look and behave like macOS apps, with custom-drawn "aqua" controls
rendered on the GPU. Windows, text, input, IME, clipboard and dialogs come from the host OS
wherever possible.

```go
import "github.com/radix29/gogui"
```

| Light | Dark |
|---|---|
| ![The controls example on Windows 11, light](docs/images/controls-light.png) | ![The controls example on Windows 11, dark](docs/images/controls-dark.png) |

*`examples/controls` on Windows 11 at 100 %, drawn by the aqua driver.*

## Status

The v0.1 scope ([`PLAN.md`](PLAN.md) §11) is built. There is no tagged release. What runs
where today:

| OS | Driver | State |
|---|---|---|
| **Windows** 10 1809+ / 11 (amd64, arm64) | aqua over Direct3D 11, Direct2D, DirectComposition and DirectWrite | Built; tested on windows/amd64 (`check.sh`, `native.sh`, golden renders). Windows/arm64 is built but not yet run on a machine; on Windows 10 the user checked the window chrome (square corners, no Mica), the other checks are open. |
| **Linux** GTK ≥ 4.14 (amd64, arm64; Wayland and X11) | aqua over GTK 4 (GSK, Pango), loaded with purego | Built and cross-compiled; the native runs and the goldens shared with Windows wait on a Linux machine. |
| **macOS** 13+ (arm64, amd64) | headless (no window) | The native AppKit driver (phase 1) is not built yet. Apps compile and run, but on the headless driver: they print a notice and open no window. |

The open checks of every OS are collected in the parity checklist [`docs/parity.md`](docs/parity.md),
one row per v0.1 item and behaviour with a column per target.

What v0.1 contains: the app lifecycle, windows (titled, closable, miniaturizable,
resizable, min size, centred, key and inactive looks), `Label`, `Button` (push, default,
cancel), `Checkbox`, `RadioGroup`, `TextField`, `SecureField`, `Slider`, `ProgressBar`
(determinate and indeterminate), `PopUpButton`, `Separator`, `Box`, `ScrollView`,
`TextView` (multi-line plain text), `VStack`, `HStack`, `Spacer`, `Form`, a menu bar (App,
Edit and custom menus, drawn in the title bar on Windows and Linux) with keyboard
shortcuts, a modal `Alert`, light and dark mode, the accent colour (the OS one or an app
override), increased contrast, Reduce Motion, HiDPI, IME and the clipboard.

## Requirements

* **Building:** Go 1.27, nothing else. No C compiler, no cgo (`CGO_ENABLED=0`), and any
  OS builds for any other.
* **Running** (decision D7): macOS 13+, Windows 10 1809+ (Windows 10 gets square corners
  and no Mica), or Linux with GTK 4.14+ and Pango 1.52+ installed (Ubuntu 24.04, Fedora
  40+, Debian 13). GTK is loaded at run time; without it `Run` returns an error instead of
  crashing. amd64 and arm64 everywhere.

## Quick start

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

		greet := gogui.NewButton("Greet").SetDefault(true) // Return triggers it
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
		win.OnClose(app.Quit)
		win.Show()
	}); err != nil {
		log.Fatal(err)
	}
}
```

```sh
CGO_ENABLED=0 go run ./examples/hello
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./examples/hello   # cross-compile
GOOS=linux   GOARCH=arm64 CGO_ENABLED=0 go build ./examples/hello
GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build ./examples/hello
```

Callbacks run on the UI thread; other goroutines reach the UI through `gogui.Do` and
`gogui.DoSync`. The package documentation (`go doc github.com/radix29/gogui`) covers the
model, the layout, the threading rules and the differences between drivers. Five examples
(`hello`, `controls`, `form`, `converter`, `notes`) are described in
[`examples/README.md`](examples/README.md).

## Known limitations of v0.1

* **No native macOS driver yet.** macOS runs the headless driver until phase 1 (AppKit
  through purego) is built.
* **No screen reader on Windows and Linux** (decision D4). The accessibility tree exists
  and is tested, but the bridge to UI Automation and AT-SPI comes after v0.1 (phase 7).
* **Theme tokens are provisional.** The aqua colours and metrics are not yet measured
  against a real Mac (`docs/tokens.md`); the look may still shift slightly.
* No public query or event for a window's key state (only its look follows it).
* Not in v0.1 ([`PLAN.md`](PLAN.md) §11): table and outline views, rich text, toolbars,
  sidebars and split views, tab views, window sheets beyond the alert, drag and drop, a
  file dialog API, tray and status items, notifications, right-to-left layout, the Liquid
  Glass theme, a packaging tool.

## Checks

`scripts/check.sh` runs every check a change must pass: gofmt, `go vet` and `go build`
for all six GOOS/GOARCH targets, staticcheck, the tests with `-count=1`, the no-cgo guard,
the hidden-callback guard and a stress run of the concurrency code. There is no CI:
`scripts/native.sh` runs what needs the target OS (native tests, golden renders, callback
measurements, frame and idle-CPU measurements, spike self-tests) and writes
`native-logs/<goos>-<goarch>.log`.

```sh
bash scripts/check.sh          # all checks (about 3 minutes)
bash scripts/check.sh -quick   # without the stress run
bash scripts/native.sh         # native checks on the current OS
```

## Documentation

* Package docs: `go doc -all github.com/radix29/gogui`.
* [`examples/README.md`](examples/README.md): the examples and their conventions.
* [`PLAN.md`](PLAN.md): design, scope, decisions and the open work.
* [`ARCHITECTURE.md`](ARCHITECTURE.md): the architecture as implemented today.

## License

MIT, see [`LICENSE`](LICENSE). The dependencies and the bundled Inter font keep their own
licenses, listed in [`THIRD_PARTY.md`](THIRD_PARTY.md). gogui bundles no Apple fonts,
symbols or images; every aqua control is drawn procedurally.
