#!/usr/bin/env bash
# native.sh runs the checks that need the target OS itself (PLAN.md §8.4). There is no CI
# (dropped 2026-10-13); the user runs this on each macOS, Linux and Windows machine and
# brings back the log it writes to native-logs/<goos>-<goarch>.log (on Linux also
# native-logs/golden-*/). The open checks it serves are in docs/parity.md.
#
#   bash scripts/native.sh
#
# Every OS: vet, the tests, the example builds, the callback measurements (S0.10a) and the
# fake-shell frame benchmarks (S6.2c). macOS: the spike/appkit self-test. Linux: the gtk4
# tests, the gallery goldens, a smoke run, the frame benchmarks, the frame pacing and the
# idle CPU of every example, under xvfb (X11) and headless weston (Wayland), and on the
# desktop session if there is one. Windows: the verbose test groups over real windows, the
# goldens, the frame benchmarks, the frame pacing and the idle CPU of every example.
# ARCHITECTURE.md §10 says what the tests check and which *-RESULT lines they log.
#
# Every step runs even if an earlier one failed; the summary at the end lists the failures
# and the exit status is 1 if there were any. Linux needs: libgtk-4-1 libgl1-mesa-dri
# libegl1 xvfb weston, and dbus-run-session for the session-bus tests (Debian/Ubuntu
# names). Bash 3.2 compatible (macOS).
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1
export CGO_ENABLED=0
unset GOOS GOARCH

goos=$(go env GOOS) || exit 1
goarch=$(go env GOARCH) || exit 1
exe=""
if [ "$goos" = windows ]; then
	exe=.exe
fi
mkdir -p native-logs || exit 1
log="native-logs/$goos-$goarch.log"

# Every example folder, in the order of the file system.
examples=()
for d in examples/*/; do
	d=${d%/}
	examples+=("${d#examples/}")
done

failed=()
run() {
	local name=$1
	shift
	echo
	echo "== $name"
	echo "\$ $*"
	if "$@"; then
		echo "-- ok: $name"
	else
		echo "-- FAIL ($?): $name"
		failed+=("$name")
	fi
}
missing() {
	echo
	echo "== $1"
	echo "-- FAIL: $2 not found; install $3"
	failed+=("$1")
}

# tmp holds every temporary file of the run (the example binaries in $tmp/bin, weston's
# runtime directories); cleanup removes it and stops a weston that is still running, also
# when the run is interrupted.
tmp=""
weston_pid=""
cleanup() {
	if [ -n "$weston_pid" ]; then
		kill "$weston_pid" 2>/dev/null
		wait "$weston_pid" 2>/dev/null
	fi
	if [ -n "$tmp" ]; then
		rm -rf "$tmp"
	fi
}

# weston_run starts weston's headless backend on a private socket in a private
# XDG_RUNTIME_DIR (mode 0700, as libwayland requires) and runs its arguments as a
# command on it with GDK_BACKEND=wayland. weston's log is printed after the run.
weston_run() {
	local dir wlog status=0
	dir=$(mktemp -d "$tmp/weston.XXXXXX") || return 1
	chmod 0700 "$dir"
	wlog="$dir/weston.log"
	XDG_RUNTIME_DIR=$dir weston --backend=headless --socket=gogui-wayland --idle-time=0 >"$wlog" 2>&1 &
	weston_pid=$!
	for _ in $(seq 100); do
		if [ -S "$dir/gogui-wayland" ] || ! kill -0 "$weston_pid" 2>/dev/null; then
			break
		fi
		sleep 0.1
	done
	if [ ! -S "$dir/gogui-wayland" ]; then
		echo "weston did not create $dir/gogui-wayland within 10 s"
		status=1
	else
		XDG_RUNTIME_DIR=$dir WAYLAND_DISPLAY=gogui-wayland GDK_BACKEND=wayland "$@" || status=$?
	fi
	kill "$weston_pid" 2>/dev/null
	wait "$weston_pid" 2>/dev/null
	weston_pid=""
	echo "--- weston log ---"
	cat "$wlog"
	rm -rf "$dir"
	return "$status"
}

# example_for runs the binary of examples/$1 for $example_secs seconds with GOGUI_DEBUG=1
# (the UI-thread and canvas checks on); its other arguments are the command that wraps it
# (env, xvfb-run, weston_run). Still running when timeout stops it (status 124) passes;
# exiting earlier, with any status, fails.
example_secs=5
example_for() {
	local name=$1 status=0
	shift
	"$@" env GOGUI_DEBUG=1 timeout "$example_secs" "$tmp/bin/$name$exe" || status=$?
	if [ "$status" -eq 124 ]; then
		echo "$name ran for $example_secs s and was stopped"
		return 0
	fi
	echo "$name exited within $example_secs s with status $status"
	return 1
}

# examples_on runs every example with example_for, one step each; $1 is the backend's
# label, the other arguments the wrapping command.
examples_on() {
	local label=$1 e
	shift
	for e in "${examples[@]}"; do
		run "examples/$e for $example_secs s $label" example_for "$e" "$@"
	done
}

# idle_on measures the idle CPU of every example (S6.2f) with scripts/idlecpu.sh: the
# process CPU time over idle_secs seconds after 2 s to show (IDLE-RESULT lines). $1 is the
# backend's label, $2 the limit in percent of one core ("-" logs only), the other
# arguments the wrapping command. controls and gallery (their indeterminate indicators loop by design) and
# converter (its sync bar moves ten times a second) are logged only.
idle_secs=10
idle_on() {
	local label=$1 limit=$2 e l
	shift 2
	for e in "${examples[@]}"; do
		l=$limit
		case "$e" in controls | converter | gallery) l=- ;; esac
		run "idle CPU of examples/$e over $idle_secs s $label (S6.2f)" \
			"$@" bash scripts/idlecpu.sh "$tmp/bin/$e$exe" "$l" 2 "$idle_secs"
	done
}

linux() {
	echo "input method: GTK_IM_MODULE=${GTK_IM_MODULE:-} XMODIFIERS=${XMODIFIERS:-} QT_IM_MODULE=${QT_IM_MODULE:-}"
	# The headless runs get a private session bus where dbus-run-session is installed:
	# the portal client's session-bus tests export their own fake portal there and skip
	# without one. env is the no-op wrapper otherwise.
	local bus=(env)
	if command -v dbus-run-session >/dev/null; then
		bus=(dbus-run-session --)
		echo "gtk4 tests under xvfb and weston: in a private session bus ($(command -v dbus-run-session))"
	else
		echo "gtk4 tests under xvfb and weston: dbus-run-session not found (install dbus-daemon or dbus-broker), the session-bus tests skip"
	fi
	# The goldens run as their own step per headless backend, strictly, and skip without
	# GOGUI_GOLDEN_OUT; the desktop session also skips the clipboard round-trip, which
	# would overwrite the user's clipboard. Pacing is logged only on Linux.
	local tests=("${bus[@]}" go test -count=1 -v -skip 'TestGalleryGolden$' ./internal/shell/gtk4/...)
	local golden=("${bus[@]}" go test -count=1 -v -run 'TestGalleryGolden$' ./internal/shell/gtk4)
	local desktop=(go test -count=1 -v -skip 'TestGalleryGolden$|TestClipboardRoundTripGTK$' ./internal/shell/gtk4/...)
	local frames=(go test -count=1 -v -run '^$' -bench BenchmarkFrameNative -benchtime 120x .)
	local pacing=(env GOGUI_PACING=1 go test -count=1 -v -run 'TestFramePacingNative$' .)
	local x11=(env GDK_BACKEND=x11 xvfb-run -a)
	if command -v xvfb-run >/dev/null; then
		run "gtk4 tests under xvfb (X11)" "${x11[@]}" "${tests[@]}"
		run "gallery goldens under xvfb (X11) (S4.3g)" env GOGUI_GOLDEN_OUT="$PWD/native-logs/golden-x11" \
			"${x11[@]}" "${golden[@]}"
		examples_on "under xvfb (X11)" "${x11[@]}"
		run "frame benchmarks under xvfb (X11) (S6.2c)" "${x11[@]}" "${frames[@]}"
		run "frame pacing under xvfb (X11) (S6.2d)" "${x11[@]}" "${pacing[@]}"
		idle_on "under xvfb (X11)" - "${x11[@]}"
	else
		missing "gtk4 tests and examples under xvfb (X11)" xvfb-run xvfb
	fi
	if command -v weston >/dev/null; then
		run "gtk4 tests under headless weston (Wayland)" weston_run "${tests[@]}"
		run "gallery goldens under headless weston (Wayland) (S4.3g)" weston_run \
			env GOGUI_GOLDEN_OUT="$PWD/native-logs/golden-wayland" "${golden[@]}"
		examples_on "under headless weston (Wayland)" weston_run
		run "frame benchmarks under headless weston (Wayland) (S6.2c)" weston_run "${frames[@]}"
		run "frame pacing under headless weston (Wayland) (S6.2d)" weston_run "${pacing[@]}"
		idle_on "under headless weston (Wayland)" - weston_run
	else
		missing "gtk4 tests and examples under headless weston (Wayland)" weston weston
	fi
	local label wrap
	for label in wayland x11; do
		if [ "$label" = wayland ]; then
			[ -n "${WAYLAND_DISPLAY:-}" ] || continue
			label="on the desktop session (Wayland)"
			wrap=(env GDK_BACKEND=wayland)
		else
			[ -n "${DISPLAY:-}" ] || continue
			label="on the desktop session (X11, $DISPLAY)"
			wrap=(env GDK_BACKEND=x11)
		fi
		run "gtk4 tests $label" "${wrap[@]}" "${desktop[@]}"
		examples_on "$label" "${wrap[@]}"
		run "frame benchmarks $label (S6.2c)" "${wrap[@]}" "${frames[@]}"
		run "frame pacing $label (S6.2d)" "${wrap[@]}" "${pacing[@]}"
		idle_on "$label" - "${wrap[@]}"
	done
}

windows() {
	local w32=./internal/shell/win32
	# On a failure the actual image and the diff go to native-logs/golden/.
	run "gallery goldens (S3.6d)" env GOGUI_GOLDEN_OUT="$PWD/native-logs/golden" \
		go test -count=1 -v -run 'TestGalleryGolden$' "$w32"
	run "IME over IMM32 (S5.3e-S5.3g)" go test -count=1 -v \
		-run 'TestImmContext$|TestLiveIMM$|TestIMEForms$|TestSurface(Composition|CompositionHandlerReacts|TextInputContext|IMEMuted)$' \
		"$w32" "$w32/w32"
	# The 1 MB benchmarks: budget one frame (16 ms), a keystroke 2 ms.
	run "TextView over DirectWrite (S5.4b, S5.4k)" go test -count=1 -v \
		-run 'TestLineMetrics(Wrap|CJK|Bidi|Shapes)$|TestCanvasDrawTextCache$' \
		-bench 'BenchmarkTextView' -benchtime 100x "$w32"
	run "popups on real windows (S5.5b-S5.5d)" go test -count=1 -v \
		-run 'TestShownPopup|TestPopupBackdrop$' "$w32"
	run "menu strip on real windows (S5.6d-S5.6i)" go test -count=1 -v \
		-run 'TestShownPopupGrabTrack$|TestShownPopupRepositionKeepsPointer$|TestShiftedFor$|TestShiftedCharLive$|TestModifierFor$|TestSurfaceKeyboard$' \
		"$w32"
	run "modal surfaces on real windows (S5.7e)" go test -count=1 -v \
		-run 'TestModalOwnerErrors$|TestModalWindow$|TestShownModal' "$w32"
	run "platform polish on real windows (S6.1c, S6.1h)" go test -count=1 -v \
		-run 'TestSurfaceFirstClick$|TestSurfaceAquaFirstClick$|TestReadAppearanceLive$|TestAppearanceChanged$|TestAppearanceChangedEveryField$' \
		"$w32"
	# One op is one presented frame; the budget is a 120 Hz frame, 8.3 ms of work.
	run "frame benchmarks over Direct2D (S6.2c)" go test -count=1 -v -run '^$' \
		-bench 'BenchmarkFrameNative' -benchtime 120x .
	run "gallery frame over Direct2D at 1x and 2x (S6.2c)" go test -count=1 -v -run '^$' \
		-bench 'BenchmarkGalleryFrame' -benchtime 100x "$w32"
	# Fails beyond shell.PacingTolerance per scenario and PacingRunTolerance per run.
	run "frame pacing on real windows (S6.2d)" env GOGUI_PACING=1 go test -count=1 -v \
		-run 'TestClockPresentObserver$|TestClockPacingWork$' "$w32"
	run "frame pacing of the examples (S6.2d)" env GOGUI_PACING=1 go test -count=1 -v \
		-run 'TestFramePacingNative$' .
	# hello, form and notes fail above 1 % of one core.
	idle_on "on the desktop" 1
}

main() {
	trap cleanup EXIT
	trap 'exit 130' INT TERM
	tmp=$(mktemp -d) || exit 1
	mkdir "$tmp/bin" || exit 1

	echo "native.sh on $goos/$goarch, $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
	run "go version" go version
	run "go env" go env GOOS GOARCH CGO_ENABLED
	run "go vet" go vet ./...
	run "go test" go test -count=1 ./...
	local e
	for e in "${examples[@]}"; do
		run "build examples/$e" go build -o "$tmp/bin/$e$exe" "./examples/$e"
	done
	# Verbose, so the measured numbers land in the log.
	run "callback measurements (S0.10a)" go test -count=1 -v \
		-run 'TestCallback(Budget|ForeignThread|Nested)' \
		-bench BenchmarkCallbackCall -benchtime 200000x ./internal/ffi
	run "frame benchmarks on the fake shell (S6.2c)" go test -count=1 -v -run 'TestFrameAllocs$' \
		-bench 'BenchmarkFrame$' -benchtime 200x .

	case "$goos" in
	darwin)
		# Needs a logged-in GUI session; the spike is the seed of S1.1.
		run "spike/appkit self-test" go run ./spike/appkit -selftest
		;;
	linux) linux ;;
	windows) windows ;;
	esac

	echo
	local parity="docs/parity.md (the v0.1 parity checklist: bring this log back, tick the rows it closes)"
	if [ ${#failed[@]} -eq 0 ]; then
		echo "native.sh: all steps passed on $goos/$goarch; log: $log"
		echo "checklist: $parity"
		return 0
	fi
	echo "native.sh: ${#failed[@]} step(s) failed on $goos/$goarch:"
	local f
	for f in "${failed[@]}"; do
		echo "  - $f"
	done
	echo "log: $log"
	echo "checklist: $parity"
	return 1
}

# main runs in a pipeline, so tee has written the whole log when the script exits (an
# exec > >(tee) process substitution could still be writing its tail).
main 2>&1 | tee "$log"
exit "${PIPESTATUS[0]}"
