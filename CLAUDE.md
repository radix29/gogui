# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Where things are

- **`PLAN.md`** is the source of truth for design, scope and roadmap. Finished steps (Results, findings) and the Q&A log are archived in **`PLAN_done.md`**: when a step is done, move its Result there (§ matching its phase) and leave a one-line ✅ pointer in PLAN.md. Read the relevant section before implementing anything. Project status is in its header and in the ✅ markers of §11.1.
- **`ARCHITECTURE.md`** describes the architecture as it is implemented today: packages, contracts, invariants and tests. Update it in the same step as any change to a package's API or behaviour. Its §0 holds the target architecture overview copied from PLAN.md §2/§2.1 (diagram and layer table). If an implementation step changes the overview (package renamed, layer added or dropped, responsibility moved), update §0 of ARCHITECTURE.md and PLAN.md §2 together so they stay identical.
- **`examples/README.md`** holds the conventions for examples. The examples are API design targets. Since S0.5 they compile on every platform and run on the headless driver, and `examples_test.go` checks their layouts.

## Process

- Work goes through the implementation steps in PLAN.md §11.1 (S0.1 … S6.3), one at a time. After each step the repo has to stay buildable and consistent, following the rules and checks at the top of §11.1. When a step is done, mark it ✅ in §11.1 with a short "Result" note, and update the PLAN.md status line and ARCHITECTURE.md.
- Decisions D1–D10 (PLAN.md §12) are settled. If a change goes against one of them, update §12 and every section it references. Open questions O3 and O4 are also listed in §12.
- If a step changes the public API, the same step updates PLAN.md §4, the examples and `examples/README.md` (§11.1 rule 6).

## Working environment (user instructions, updated 2026-10-07)

- **Claude can and should run the builds and tests itself.** Go (1.27) is installed locally. From now on, every step is verified by Claude: run the per-step checks from PLAN.md §11.1 (gofmt, vet, build for all six GOOS/GOARCH targets, tests, stress runs for concurrency code, no-cgo guard) before calling a step done. Run tests fresh (`-count=1`) when it matters, so cached results don't hide anything. Don't hand these checks to the user.
- **Go modules download through the LGT module mirror** (`GOPROXY` is already set). Claude may run `go get` and `go mod tidy`, and commits the resulting `go.mod` and `go.sum` changes. Dependencies still arrive only in the step that first imports them (§11.1 rule 4), and must stay pure Go and minimal (PLAN.md §9). Don't write local stand-ins or fakes for external libraries.
- **github.com itself does not resolve** (`git clone` / `git ls-remote` fail). Only the module proxy works, so fetch code through `go` commands, not git.
- **No CI and no GitHub workflows (user rule 2026-10-13).** Don't add `.github/` or any CI config. Native runs on macOS, Linux and Windows/arm64 go through `bash scripts/native.sh` on the user's machines (PLAN.md §8.4); the user brings back `native-logs/<goos>-<goarch>.log`. Claude runs `native.sh` itself for windows/amd64.
- **What Claude can't check:** running native GUI code on macOS or Linux (the host is Windows), and any manual "Done when" item that needs a specific OS or a human looking at a window. At the end of each task, say what was checked locally, and list exactly what the user still has to run or look at on the other OSes.

## Commands

The per-step checks are in PLAN.md §11.1. Since S0.6a, `bash scripts/check.sh` runs all of them (gofmt, vet and build for the six targets, staticcheck per GOOS, `go test -count=1`, the no-cgo guard, the hidden-callback guard (since S0.10b), the stress run). `-quick` skips the stress run, which takes about 85 s. Run the full script before marking a step ✅. Since S0.5k the checks run on `./...` (all five examples included).

```sh
bash scripts/check.sh                                           # every per-step check (about 2 min)
bash scripts/check.sh -quick                                    # the same without the stress run
CGO_ENABLED=0 go test -count=1 ./...                            # all unit tests (no display needed)
CGO_ENABLED=0 go test ./internal/mainthread -run TestDoSyncPanic   # single package / single test
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go test -count=20 -cpu=1,2,4,8 ./internal/mainthread  # stress run for concurrency code
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...      # cross-compile check (all six GOOS/GOARCH targets)
CGO_ENABLED=1 go test -race -count=1 ./internal/mainthread      # cgo test run (testing only; needs a C compiler, so the user's machines, not this host)
bash scripts/native.sh                                          # native tests, callback measurements, spike self-tests on the current OS; log in native-logs/
go list -f '{{.CgoFiles}}' ./...                                # must be empty everywhere (with CGO_ENABLED=0 cgo files are only ignored, so check.sh also scans for import "C")
```

## Hard rules

- **No cgo in shipped code; cgo only for testing (user rule 2026-10-06, relaxed for testing 2026-10-12).** The library, the examples, the spikes and every app build with `CGO_ENABLED=0`: never `import "C"` there, and never add `.m`/`.c` files. Native code is reached through `ebitengine/purego` (macOS, Linux) and `golang.org/x/sys/windows` (Windows). Dependencies stay pure Go and minimal (PLAN.md §9).
  - **cgo is allowed for testing purposes only:** test runs with `CGO_ENABLED=1`, above all `go test -race`. These runs come *in addition to* the `CGO_ENABLED=0` checks, never instead of them. What ships, and what `check.sh` builds and tests by default, stays `CGO_ENABLED=0`, and the no-cgo guard stays. Stress runs remain the default way to check concurrency code.
  - Test-only C code (for example a helper package that checks struct layouts against the real headers with `offsetof`) is also allowed in principle. It has to be behind its own build tag, must never be imported by non-test code, and the step that first adds it updates the no-cgo guard in `check.sh` and PLAN.md D5.
  - A cgo test run needs a C compiler. The Windows host has none (no gcc or clang on `PATH`), so such runs happen on the user's machines. Say so in the step's Result instead of skipping it silently.
  - Commands still show `CGO_ENABLED=0` everywhere except in a command that is explicitly a cgo test run.
- **One API, no build tags in app code.** Apps and examples import only `gogui`, never `internal/...`. Build tags live only in the driver-selection files, the platform packages and `spike/`. The aqua packages have no build tags.
- **Public API conventions** are in PLAN.md §4.2: chained setters, getters without `Get`, `On*` returns `Unsubscribe`, logical points with a top-left origin, `KeyPrimary`, NSColor-style semantic colors, and UI-thread callbacks with `gogui.Do` / `DoSync`.
- **FFI safety (PLAN.md §7.2):**
  - Never store Go pointers in native memory. Native objects carry an `ffi.Handle` into the driver's `ffi.Handles[T]` table, which is used on the UI thread.
  - Callbacks (`purego.NewCallback` / `syscall.NewCallback`) are a fixed set of trampolines created at start-up, one per signature. They dispatch through the handle in `user_data`, the ivar or `GWLP_USERDATA`. Never create a callback per widget.
  - No UI calls from finalizers, and no reliance on finalizers.
  - Any `uintptr`→`unsafe.Pointer` conversion outside `internal/ffi` needs a justified `//nolint`.
  - Mirrored C/ObjC structs need layout tests on amd64 and arm64.
  - Avoid Objective-C block APIs (use delegates or `performSelectorOnMainThread:`).
- **Licensing (PLAN.md §3.1):** never bundle SF Pro, SF Symbols, or any Apple bitmaps or assets. All aqua controls are drawn procedurally from theme tokens. The Windows/Linux UI font is Inter (OFL), decision D9.
