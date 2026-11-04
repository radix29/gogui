#!/usr/bin/env bash
# check.sh runs the per-step checks of PLAN.md §11.1. It stops at the first failure.
#
#   bash scripts/check.sh          all checks, including the stress run (about 1.5 min)
#   bash scripts/check.sh -quick   everything except the stress run
#
# Every Go command runs with CGO_ENABLED=0 (D5: no cgo in shipped code; cgo test runs such
# as -race are allowed only in addition to these checks and are not part of this script). The script uses only bash built-ins, grep, go and gofmt, so it also runs in a
# restricted Git Bash on Windows.
set -euo pipefail

# Pinned tools. They are run by version and never added to go.mod (§11.1 rule 4).
STATICCHECK=honnef.co/go/tools/cmd/staticcheck@v0.8.2

# Packages with concurrency code, stress-tested instead of the race detector (§8.4).
# TestCallbackBudget is skipped there (S0.10a): each run starts two child processes that
# use up every callback slot, which costs time and tests nothing about concurrency.
# TestClockFrames, TestClockPaced, TestClockModal and TestClockStall (shell/win32, S2.4f)
# wait on real frames, the refresh and the swap chains' waitables; they are single-threaded
# and add only wall-clock time there. TestSurfaceLiveResize (S2.4f) resizes a shown window.
# TestSurfaceShowZoom and TestSurfaceFramePaint (S2.3c) show real windows on the desktop;
# 640 runs of them would only flash windows and steal the foreground.
# TestSurfaceWheel (S2.3e) waits on the real 100 ms gesture timer.
# TestSurfaceDeviceLoss (S2.4c_2) recreates the GPU devices eight times per run (about
# 2 s each time); it is single-threaded, and every other surface test still creates and
# releases the devices under the stress run. The run gets -timeout 30m: each shell test
# now creates a D3D11 device (about 0.25 s), so shell/win32 alone takes minutes.
STRESS_SKIP="TestCallbackBudget|TestClockFrames|TestClockPaced|TestClockModal|TestClockStall|TestSurfaceLiveResize|TestSurfaceShowZoom|TestSurfaceFramePaint|TestSurfaceWheel|TestSurfaceDeviceLoss"
STRESS_PKGS=(. ./internal/mainthread ./internal/ffi ./internal/driver/headless ./internal/shell/fake ./internal/aqua ./internal/shell/win32)

TARGETS=(darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64)

quick=0
for arg in "$@"; do
	case "$arg" in
	-quick | --quick) quick=1 ;;
	-h | --help)
		sed -n '2,9p' "$0"
		exit 0
		;;
	*)
		echo "check.sh: unknown argument $arg" >&2
		exit 2
		;;
	esac
done

cd "$(dirname "$0")/.."
export CGO_ENABLED=0
unset GOOS GOARCH

SECONDS=0
step() { printf '== %-37s' "$1"; step_start=$SECONDS; }
ok() { echo "ok ($((SECONDS - step_start))s)"; }
fail() {
	echo "FAIL"
	echo "check.sh: $1" >&2
	exit 1
}

step "gofmt"
out=$(gofmt -l .)
[[ -z "$out" ]] || fail "files need gofmt:"$'\n'"$out"
ok

step "go vet (6 targets)"
for t in "${TARGETS[@]}"; do
	GOOS=${t%/*} GOARCH=${t#*/} go vet ./... || fail "go vet failed for $t"
done
ok

step "go build (6 targets)"
for t in "${TARGETS[@]}"; do
	GOOS=${t%/*} GOARCH=${t#*/} go build ./... || fail "go build failed for $t"
done
ok

step "staticcheck (3 GOOS)"
# Build staticcheck once for the host, then lint each GOOS with it. "GOOS=linux go run"
# would build a Linux binary that a Windows host can't execute.
tooldir=$(mktemp -d)
trap 'rm -rf "$tooldir"' EXIT
GOBIN=$tooldir go install "$STATICCHECK" || fail "cannot install $STATICCHECK"
sc="$tooldir/staticcheck$(go env GOEXE)"
for os in darwin linux windows; do
	GOOS=$os GOARCH=amd64 "$sc" ./... || fail "staticcheck failed for GOOS=$os"
done
ok

step "go test -count=1"
go test -count=1 ./... >"$tooldir/test.log" 2>&1 || {
	cat "$tooldir/test.log" >&2
	fail "go test failed"
}
ok

step "no-cgo guard"
# With CGO_ENABLED=0, go list moves cgo files to IgnoredGoFiles, so CgoFiles alone sees
# nothing. The source scan below is the real guard; it also covers files for other OSes.
cgo=$(go list -f '{{if .CgoFiles}}{{.ImportPath}}: {{.CgoFiles}}{{end}}' ./...)
[[ -z "$cgo" ]] || fail "cgo files: $cgo"
cimports=$(grep -rlE --include='*.go' '^[[:space:]]*(import[[:space:]]+)?(_[[:space:]]+)?"C"[[:space:]]*(//.*)?$' . || true)
[[ -z "$cimports" ]] || fail "files import \"C\":"$'\n'"$cimports"
shopt -s globstar nullglob
native=(**/*.c **/*.h **/*.m **/*.mm **/*.cc **/*.cpp **/*.cxx **/*.S **/*.swig)
shopt -u globstar nullglob
[[ ${#native[@]} -eq 0 ]] || fail "native source files: ${native[*]}"
ok

step "hidden-callback guard"
# Native callbacks can't be freed and share one budget (S0.10b, PLAN.md §7.2), so only
# internal/ffi's RegisterCallback may create them; tests may measure them directly.
# purego/objc's NewIMP, NewBlock and RegisterClass create callbacks of their own.
callre='\b((syscall|windows|purego)\.NewCallback|(syscall|windows)\.NewCallbackCDecl|objc\.(NewIMP|NewBlock|RegisterClass))[[:space:]]*\('
hidden=$(grep -rnE --include='*.go' --exclude='*_test.go' "$callre" . | grep -v '^\./internal/ffi/' || true)
[[ -z "$hidden" ]] || fail "native callbacks created outside internal/ffi (use ffi.RegisterCallback):"$'\n'"$hidden"
ok

if [[ $quick -eq 1 ]]; then
	echo "== stress run                           skipped (-quick)"
else
	step "stress run (-count=20 -cpu=1,2,4,8)"
	go test -count=20 -cpu=1,2,4,8 -timeout 30m -skip "$STRESS_SKIP" "${STRESS_PKGS[@]}" >"$tooldir/stress.log" 2>&1 || {
		cat "$tooldir/stress.log" >&2
		fail "stress run failed"
	}
	ok
fi

echo "all checks passed in ${SECONDS}s"
