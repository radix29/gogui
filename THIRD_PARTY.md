# Third-party software

gogui itself is licensed under the MIT License (`LICENSE`, decision D12 in PLAN.md §12).
It depends on the Go modules and bundles the font listed below. Each keeps its own
license; an app that ships a gogui binary passes these notices on with it.

`internal/repocheck/license_test.go` keeps this file in step with `go.mod`: every module that `go.mod`
requires (direct or indirect) needs a row in the modules table with the same version, and
every row needs a module in `go.mod`.

## Go modules

| Module | Version | License | Used for |
|---|---|---|---|
| `github.com/ebitengine/purego` | v0.11.1 | Apache-2.0 | calling C libraries without cgo: GTK 4 (Linux), the AppKit spike `spike/appkit` (macOS; the appkit driver from phase 1), COM methods with float arguments (Windows) |
| `github.com/godbus/dbus/v5` | v5.2.4 | BSD-2-Clause | the xdg-desktop-portal appearance settings on Linux |
| `github.com/rivo/uniseg` | v0.4.7 | MIT | grapheme cluster and word boundaries in text editing |
| `golang.org/x/sys` | v0.49.0 | BSD-3-Clause | Win32 and COM calls on Windows |

The license texts are in each module's source (`LICENSE` or `LICENSE.txt`, in the module
cache under `$(go env GOMODCACHE)`, or at the module's repository). purego ships no
`NOTICE` file. A Go binary also contains the Go runtime and standard library (BSD-3-Clause,
<https://go.dev/LICENSE>).

## Bundled assets

| Asset | Where | License | Notice |
|---|---|---|---|
| Inter (UI font on Windows and Linux, decision D9) | `assets/fonts/Inter-*.ttf`, embedded into Windows and Linux binaries | SIL Open Font License 1.1 | `assets/fonts/OFL.txt`, `assets/fonts/LICENSE.txt` |

No Apple font, symbol, bitmap or other asset is part of gogui (PLAN.md §3.1); every control
is drawn from theme tokens.

## Tools not shipped

`scripts/check.sh` installs and runs staticcheck (`honnef.co/go/tools`, MIT) by version with `go install`;
it is never added to `go.mod` and is not part of any binary.
