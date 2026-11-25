#!/usr/bin/env bash
# idlecpu.sh measures the CPU time an app spends left alone (PLAN.md S6.2f): it starts
# the binary, waits settle seconds for the window to show and paint, reads the process's
# CPU time (user and kernel), waits secs seconds, reads it again and stops the app. It
# prints one line,
#
#   IDLE-RESULT <name> cpu=<ms>ms wall=<ms>ms (<percent> % of one core)
#
# and exits 1 when the percentage is above limit (a number; "-" logs only). Nothing
# touches the window, so it stays in the state it starts in (a focused field's caret
# blinks until its timeout, an indeterminate indicator loops). native.sh runs it per
# example, on Linux inside its display wrappers (xvfb-run, weston_run, env).
#
#   bash scripts/idlecpu.sh <binary> <limit> [settle] [secs]
#
# Windows: through PowerShell (Start-Process, TotalProcessorTime); Linux: /proc/<pid>/stat
# (utime and stime). The app is stopped however the script ends. Bash 3.2 compatible.
set -uo pipefail

if [ $# -lt 2 ]; then
	echo "usage: idlecpu.sh <binary> <limit> [settle] [secs]" >&2
	exit 2
fi
bin=$1
limit=$2
settle=${3:-2}
secs=${4:-10}
name=$(basename "$bin" .exe)
for n in "$settle" "$secs"; do
	case "$n" in
	'' | *[!0-9]*)
		echo "idlecpu.sh: settle and secs must be whole seconds, got $n" >&2
		exit 2
		;;
	esac
done

case "$(uname -s)" in
MINGW* | MSYS* | CYGWIN*)
	# The path goes to PowerShell through the environment, never spliced into the
	# script, so quotes or $ in it can't change the command. The finally block stops
	# the app also when PowerShell is interrupted. Integers only, so the host's number
	# format does not matter.
	IDLECPU_BIN=$(cygpath -w "$bin") || exit 1
	export IDLECPU_BIN
	out=$(powershell -NoProfile -NonInteractive -Command "
		\$p = Start-Process -FilePath \$env:IDLECPU_BIN -PassThru
		try {
			Start-Sleep -Seconds $settle
			\$p.Refresh()
			if (\$p.HasExited) { 'exited'; exit }
			\$a = \$p.TotalProcessorTime.TotalMilliseconds
			\$t = [Diagnostics.Stopwatch]::StartNew()
			Start-Sleep -Seconds $secs
			\$p.Refresh()
			if (\$p.HasExited) { 'exited'; exit }
			\$b = \$p.TotalProcessorTime.TotalMilliseconds
			\$w = \$t.Elapsed.TotalMilliseconds
			'{0} {1}' -f [int64](\$b - \$a), [int64]\$w
		} finally {
			if (-not \$p.HasExited) { Stop-Process -Id \$p.Id -Force }
		}
	" | tr -d '\r')
	;;
Linux)
	tck=$(getconf CLK_TCK)
	cpu() { # the process's utime + stime in ms; the fields after the ")" of comm
		local rest
		rest=$(sed 's/^.*) //' "/proc/$1/stat") || return 1
		echo "$rest" | awk -v t="$tck" '{ printf "%d", ($12 + $13) * 1000 / t }'
	}
	now() { date +%s%N | awk '{ printf "%d", $1 / 1000000 }'; }
	"$bin" &
	pid=$!
	trap 'kill "$pid" 2>/dev/null' EXIT
	trap 'exit 130' INT TERM
	sleep "$settle"
	if a=$(cpu "$pid"); then
		t0=$(now)
		sleep "$secs"
		if b=$(cpu "$pid"); then
			out="$((b - a)) $(($(now) - t0))"
		else
			out=exited
		fi
	else
		out=exited
	fi
	kill "$pid" 2>/dev/null
	wait "$pid" 2>/dev/null
	;;
*)
	echo "idlecpu.sh: not on $(uname -s)"
	exit 1
	;;
esac

if [ "$out" = exited ] || [ -z "$out" ]; then
	echo "IDLE-RESULT $name exited before the measurement ended ($out)"
	exit 1
fi
set -- $out
pct=$(awk -v c="$1" -v w="$2" 'BEGIN { printf "%.2f", 100 * c / w }')
echo "IDLE-RESULT $name cpu=${1}ms wall=${2}ms ($pct % of one core)"
if [ "$limit" != - ] && awk -v p="$pct" -v l="$limit" 'BEGIN { exit !(p > l) }'; then
	echo "idlecpu.sh: $name used $pct % of one core left alone, limit $limit %"
	exit 1
fi
