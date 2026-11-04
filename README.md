# gogui

A Go GUI toolkit for desktop apps that run on **macOS, Windows and Linux** from one
portable codebase. On macOS apps use real AppKit controls; on Windows and Linux they
look and behave like macOS apps, with the custom-drawn "aqua" controls rendered on the
GPU. Windows, text, input, IME, dialogs, clipboard and accessibility come from the host
OS wherever possible.

```go
import "github.com/radix29/gogui"
```

## Status

**Phase 0 (foundations) and phase 2 (render and Windows shell) are done.** The complete
v0.1 public API, the layout engine, the UI-thread queue and a headless driver exist and
are tested. On Windows the examples open real windows through the aqua driver (drawn
title bar and traffic lights, GPU rendering, DirectWrite text; the controls are still
placeholders until phase 3). On macOS they still run on the headless driver (they print
a notice and block), and on Linux `Run` returns an error until phase 4. See the roadmap
in [`PLAN.md`](PLAN.md) §11.

## Building

No cgo in gogui or its apps, on any platform (cgo is used only for optional test runs such as `go test -race`). Only the Go toolchain (1.27) is needed, and any OS can build
for any other:

```sh
GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build ./examples/hello
GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build ./examples/hello
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./examples/hello
```

## Checks

`scripts/check.sh` runs every check a change must pass: gofmt, `go vet` and `go build`
for all six GOOS/GOARCH targets, staticcheck, the tests, the no-cgo guard and a stress
run of the concurrency code. There is no CI: `scripts/native.sh` runs the checks that
need the target OS (native tests, callback measurements, spike self-tests) on macOS,
Linux and Windows machines and writes `native-logs/<goos>-<goarch>.log`.

```sh
bash scripts/check.sh          # all checks (about 2 minutes)
bash scripts/check.sh -quick   # without the stress run
```

## Documentation

* [`PLAN.md`](PLAN.md): design, scope, decisions and roadmap.
* [`ARCHITECTURE.md`](ARCHITECTURE.md): the architecture as implemented today.
* [`examples/README.md`](examples/README.md): the examples and their conventions.
