#!/usr/bin/env bash
# check.sh runs the per-step checks of PLAN.md §11.1. It stops at the first failing phase.
#
#   bash scripts/check.sh          all checks, including the stress run (about 3 min)
#   bash scripts/check.sh -quick   everything except the stress run (about 1.5 min)
#
# Every Go command runs with CGO_ENABLED=0 (D5: no cgo in shipped code; cgo test runs such
# as -race are allowed only in addition to these checks and are not part of this script).
# Besides bash built-ins, go and gofmt it uses only grep, sed, mktemp, cat, dirname and rm,
# so it also runs in a restricted Git Bash on Windows (no find, no sort). A tool that fails
# (gofmt, go list, grep with status 2) fails its phase with its output; it is never taken
# for "nothing found".
#
# Independent checks run in parallel: vet and build for the six targets and staticcheck for
# the three GOOS together, then the tests, then the guards, then the three stress runs
# together. A failing job's output is printed after its phase.
set -euo pipefail

# Pinned tools. They are run by version and never added to go.mod (§11.1 rule 4).
STATICCHECK=honnef.co/go/tools/cmd/staticcheck@v0.8.2

# Packages with concurrency code, stress-tested instead of the race detector (§8.4).
# The stress run repeats every test 80 times (-count=20 -cpu=1,2,4,8), so it skips tests
# that are single-threaded and only cost wall-clock time there (they still run once in
# "go test -count=1"):
# TestCallbackBudget (S0.10a) and TestRunBlocksWithoutWork (headless) start child
# processes (the callback slots; a one-second wait that Run blocks), which tests nothing
# about concurrency. The TestNotes*OnAqua scroll tests (gogui) replay wheel, swipe and key
# sequences in real time on the fake shell; its timers posting to the queue are stressed by
# shell/fake's own tests. The TestIdle*OnAqua audits (S6.2a) run every example through
# minutes of fake-clock frames on the UI goroutine alone, and TestFrameAllocs (S6.2c) counts
# the allocations of every scene's frames there (AllocsPerRun pins GOMAXPROCS to 1).
STRESS_SKIP="TestCallbackBudget|TestRunBlocksWithoutWork|TestNotes(Wheel|Swipe|Latching|Culling|PageKeys)OnAqua|TestIdle[A-Za-z]*OnAqua|TestFrameAllocs"
STRESS_PKGS=(. ./internal/mainthread ./internal/ffi ./internal/driver/headless ./internal/shell/fake)

# aqua is stressed on its own, only over its driver tests: driver.go (the queue pointer,
# Post, DoSync, the wake through the shell, Quit) is its only cross-thread code. Its other
# 315 tests are element, layout and text tests on the UI thread; 80 runs of them were
# about 2 minutes. They run once in "go test -count=1".
AQUA_STRESS_RUN="^TestDriver"

# shell/win32 is stressed on its own, only over the tests of its cross-thread code (shell.go:
# the queue, Post, the wake message, Quit; the timers and the clipboard, which post back
# from other goroutines; the wndproc dispatch). Every other win32 test is single-threaded on
# the UI thread and creates a D3D11 device (about 0.07 s): 80 runs of 146 of them were
# about 11 minutes and tested nothing about concurrency. The surface, clock and shown
# window tests run once in "go test -count=1"; the stress run never showed windows.
WIN32_STRESS_RUN="^(TestRun|TestQuit|TestPost|TestWake|TestPanicIn|TestWndproc|TestAppearanceChanged|TestTimer|TestAfter|TestClipboard)"

TARGETS=(darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64)

quick=0
for arg in "$@"; do
	case "$arg" in
	-quick | --quick) quick=1 ;;
	-h | --help)
		sed -n '2,16p' "$0"
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

tooldir=$(mktemp -d)
trap 'rm -rf "$tooldir"' EXIT

SECONDS=0
step() { printf '== %-37s' "$1"; step_start=$SECONDS; }
ok() { echo "ok ($((SECONDS - step_start))s)"; }
fail() {
	echo "FAIL"
	echo "check.sh: $1" >&2
	exit 1
}

# job NAME CMD... starts CMD in the background, its output in $tooldir/NAME.log; wait_jobs
# waits for every started job and fails with the logs of the ones that failed.
job_pids=()
job_names=()
job() {
	local name=$1
	shift
	"$@" >"$tooldir/$name.log" 2>&1 &
	job_pids+=($!)
	job_names+=("$name")
}
wait_jobs() {
	local failed=() i
	for i in "${!job_pids[@]}"; do
		wait "${job_pids[$i]}" || failed+=("${job_names[$i]}")
	done
	job_pids=()
	job_names=()
	[[ ${#failed[@]} -eq 0 ]] && return 0
	echo "FAIL"
	for i in "${failed[@]}"; do
		echo "---- $i" >&2
		cat "$tooldir/$i.log" >&2
	done
	echo "check.sh: failed: ${failed[*]}" >&2
	exit 1
}
for_target() { GOOS=${1%/*} GOARCH=${1#*/} "${@:2}"; }

# scan GREP-ARGS... greps the tree without the folders that hold no source (EXCLUDE), so
# every source scan below skips the same ones. No match (status 1) is success with empty
# output; a grep error (status 2) fails.
EXCLUDE=(--exclude-dir=.git --exclude-dir=native-logs --exclude-dir=screenshots)
scan() {
	local s=0
	grep "${EXCLUDE[@]}" "$@" || s=$?
	[[ $s -le 1 ]]
}

step "gofmt"
out=$(gofmt -l . 2>&1) || fail "gofmt failed:"$'\n'"$out"
[[ -z "$out" ]] || fail "files need gofmt:"$'\n'"$out"
ok

step "vet, build (6 targets), staticcheck"
# Build staticcheck once for the host, then lint each GOOS with it. "GOOS=linux go run"
# would build a Linux binary that a Windows host can't execute.
GOBIN=$tooldir go install "$STATICCHECK" >"$tooldir/install.log" 2>&1 || {
	cat "$tooldir/install.log" >&2
	fail "cannot install $STATICCHECK"
}
goexe=$(go env GOEXE) || fail "go env GOEXE failed"
sc="$tooldir/staticcheck$goexe"
for t in "${TARGETS[@]}"; do
	job "vet-${t/\//-}" for_target "$t" go vet ./...
	job "build-${t/\//-}" for_target "$t" go build ./...
done
for os in darwin linux windows; do
	job "staticcheck-$os" for_target "$os/amd64" "$sc" ./...
done
wait_jobs
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
cgo=$(go list -f '{{if .CgoFiles}}{{.ImportPath}}: {{.CgoFiles}}{{end}}' ./... 2>&1) ||
	fail "go list failed:"$'\n'"$cgo"
[[ -z "$cgo" ]] || fail "cgo files: $cgo"
cimports=$(scan -rlE --include='*.go' '^[[:space:]]*(import[[:space:]]+)?(_[[:space:]]+)?"C"[[:space:]]*(//.*)?$' .) ||
	fail "grep for import \"C\" failed"
[[ -z "$cimports" ]] || fail "files import \"C\":"$'\n'"$cimports"
# One grep walk lists every native source file: -l the ones with a line, -L the empty ones
# (a bash globstar walk per pattern took 16 s on Windows).
natinc=(--include='*.c' --include='*.h' --include='*.m' --include='*.mm' --include='*.cc'
	--include='*.cpp' --include='*.cxx' --include='*.S' --include='*.swig')
native=$(scan -rl "${natinc[@]}" -e '' . && scan -rL "${natinc[@]}" -e '' .) ||
	fail "grep for native source files failed"
[[ -z "$native" ]] || fail "native source files:"$'\n'"$native"
ok

step "hidden-callback guard"
# Native callbacks can't be freed and share one budget (S0.10b, PLAN.md §7.2), so only
# internal/ffi's RegisterCallback may create them; tests may measure them directly.
# purego/objc's NewIMP, NewBlock and RegisterClass create callbacks of their own.
callre='\b((syscall|windows|purego)\.NewCallback|(syscall|windows)\.NewCallbackCDecl|objc\.(NewIMP|NewBlock|RegisterClass))[[:space:]]*\('
hidden=$(scan -rnE --include='*.go' --exclude='*_test.go' "$callre" .) || fail "grep for native callbacks failed"
hidden=$(grep -v '^\./internal/ffi/' <<<"$hidden" || true)
[[ -z "$hidden" ]] || fail "native callbacks created outside internal/ffi (use ffi.RegisterCallback):"$'\n'"$hidden"
ok

if [[ $quick -eq 1 ]]; then
	echo "== stress run                           skipped (-quick)"
else
	step "stress run (-count=20 -cpu=1,2,4,8)"
	job stress go test -count=20 -cpu=1,2,4,8 -timeout 30m -skip "$STRESS_SKIP" "${STRESS_PKGS[@]}"
	job stress-aqua go test -count=20 -cpu=1,2,4,8 -timeout 30m -run "$AQUA_STRESS_RUN" ./internal/aqua
	job stress-win32 go test -count=20 -cpu=1,2,4,8 -timeout 30m -run "$WIN32_STRESS_RUN" ./internal/shell/win32
	wait_jobs
	ok
fi

echo "all checks passed in ${SECONDS}s"
