#!/usr/bin/env bash
# native.sh runs the checks that need the target OS itself (PLAN.md §8.4): the native
# tests, the callback measurements, the macOS spike self-test, on Linux the gtk4 tests
# and the examples on both GDK backends, and on Windows the gallery goldens. There is
# no CI (dropped 2026-10-13); the user runs this on each macOS, Linux and Windows
# machine and brings back the log it writes to native-logs/<goos>-<goarch>.log.
#
#   bash scripts/native.sh
#
# Every step runs even if an earlier one failed; the summary at the end lists the
# failures and the exit status is 1 if there were any. On Linux the gtk4 tests and
# the examples run under xvfb (X11) and headless weston (Wayland), and also on the
# desktop session if there is one; they need: libgtk-4-1 libgl1-mesa-dri libegl1 xvfb
# weston (Debian/Ubuntu names). Bash 3.2 compatible (macOS).
set -uo pipefail

cd "$(dirname "$0")/.."
export CGO_ENABLED=0
unset GOOS GOARCH

goos=$(go env GOOS) || exit 1
goarch=$(go env GOARCH) || exit 1
mkdir -p native-logs
log="native-logs/$goos-$goarch.log"
exec > >(tee "$log") 2>&1

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

# weston_run starts weston's headless backend on a private socket in a private
# XDG_RUNTIME_DIR (mode 0700, as libwayland requires) and runs its arguments as a
# command on it with GDK_BACKEND=wayland. weston's log is printed after the run.
weston_run() {
	local dir wlog pid status=0
	dir=$(mktemp -d) || return 1
	chmod 0700 "$dir"
	wlog="$dir/weston.log"
	XDG_RUNTIME_DIR=$dir weston --backend=headless --socket=gogui-wayland --idle-time=0 >"$wlog" 2>&1 &
	pid=$!
	for _ in $(seq 100); do
		if [ -S "$dir/gogui-wayland" ] || ! kill -0 "$pid" 2>/dev/null; then
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
	kill "$pid" 2>/dev/null
	wait "$pid" 2>/dev/null
	echo "--- weston log ---"
	cat "$wlog"
	rm -rf "$dir"
	return "$status"
}

# example_for runs the binary of examples/$1 ($example_dir/$1) for $example_secs
# seconds with GOGUI_DEBUG=1 (the UI-thread and canvas checks on); its other arguments
# are the command that wraps it (env, xvfb-run, weston_run). Still running when timeout
# stops it (status 124) passes; exiting earlier, with any status, fails.
example_secs=5
example_for() {
	local name=$1 status=0
	shift
	"$@" env GOGUI_DEBUG=1 timeout "$example_secs" "$example_dir/$name" || status=$?
	if [ "$status" -eq 124 ]; then
		echo "$name ran for $example_secs s and was stopped"
		return 0
	fi
	echo "$name exited within $example_secs s with status $status"
	return 1
}

# examples_on runs every example of $examples with example_for, one step each; $1 is
# the backend's label, the other arguments the wrapping command.
examples_on() {
	local label=$1 e
	shift
	for e in $examples; do
		run "examples/$e for $example_secs s $label" example_for "$e" "$@"
	done
}

echo "native.sh on $goos/$goarch, $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
run "go version" go version
run "go env" go env GOOS GOARCH CGO_ENABLED
run "go vet" go vet ./...
run "go test" go test -count=1 ./...
run "build examples" go build ./examples/...
# S0.10a: verbose, so the measured numbers appear in the log.
run "callback measurements (S0.10a)" go test -count=1 -v \
	-run 'TestCallback(Budget|ForeignThread|Nested)' \
	-bench BenchmarkCallbackCall -benchtime 200000x ./internal/ffi

case "$goos" in
darwin)
	# S0.8b/S0.8c; needs a logged-in GUI session. Deleted with the spike in S1.1.
	run "spike/appkit self-test" go run ./spike/appkit -selftest
	;;
linux)
	# S4.1l: the gtk4 tests that need gtk_init_check and a display (the plain go test
	# above skips them without one; since S4.2 also the canvas over a real GtkSnapshot,
	# the Pango text system and the app font), verbose so the GTK version, the backend
	# (BACKEND-RESULT) and the other *-RESULT lines land in the log; then every example
	# for a few seconds on each backend (S4.2k; hello only until then).
	gtk4_tests="go test -count=1 -v ./internal/shell/gtk4/..."
	examples="hello controls form notes converter"
	example_dir=$(mktemp -d) || exit 1
	for e in $examples; do
		run "build examples/$e" go build -o "$example_dir/$e" "./examples/$e"
	done
	# S4.3d: under xvfb and weston there is no session bus, so the gtk4 tests run in a
	# private one (dbus-run-session) where it is installed: the portal client's
	# session-bus tests export their own fake portal there. Without it they skip.
	if command -v dbus-run-session >/dev/null; then
		gtk4_bus="dbus-run-session --"
		echo "gtk4 tests under xvfb and weston: in a private session bus ($(command -v dbus-run-session))"
	else
		gtk4_bus=""
		echo "gtk4 tests under xvfb and weston: dbus-run-session not found (install dbus-daemon or dbus-broker), no session bus, the session-bus tests skip"
	fi
	if command -v xvfb-run >/dev/null; then
		run "gtk4 tests under xvfb (X11)" env GDK_BACKEND=x11 xvfb-run -a $gtk4_bus $gtk4_tests
		examples_on "under xvfb (X11)" env GDK_BACKEND=x11 xvfb-run -a
	else
		missing "gtk4 tests and examples under xvfb (X11)" xvfb-run xvfb
	fi
	if command -v weston >/dev/null; then
		run "gtk4 tests under headless weston (Wayland)" weston_run $gtk4_bus $gtk4_tests
		examples_on "under headless weston (Wayland)" weston_run
	else
		missing "gtk4 tests and examples under headless weston (Wayland)" weston weston
	fi
	if [ -n "${WAYLAND_DISPLAY:-}" ]; then
		run "gtk4 tests on the desktop session (Wayland)" env GDK_BACKEND=wayland $gtk4_tests
		examples_on "on the desktop session (Wayland)" env GDK_BACKEND=wayland
	fi
	if [ -n "${DISPLAY:-}" ]; then
		run "gtk4 tests on the desktop session (X11, $DISPLAY)" env GDK_BACKEND=x11 $gtk4_tests
		examples_on "on the desktop session (X11, $DISPLAY)" env GDK_BACKEND=x11
	fi
	rm -rf "$example_dir"
	;;
windows)
	# S3.6d: verbose, so the comparison report of each golden lands in the log. On a
	# failure the actual image and the diff go to native-logs/golden/.
	run "gallery goldens (S3.6d)" env GOGUI_GOLDEN_OUT="$PWD/native-logs/golden" \
		go test -count=1 -v -run 'TestGalleryGolden$' ./internal/shell/win32
	;;
esac

echo
if [ ${#failed[@]} -eq 0 ]; then
	echo "native.sh: all steps passed on $goos/$goarch; log: $log"
	exit 0
fi
echo "native.sh: ${#failed[@]} step(s) failed on $goos/$goarch:"
for f in "${failed[@]}"; do
	echo "  - $f"
done
echo "log: $log"
exit 1
