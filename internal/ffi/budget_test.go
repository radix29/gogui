package ffi

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// The callback-budget measurement (PLAN.md §11.1 S0.10a). Native callbacks
// can't be freed, so measuring how many a process can create uses them up.
// TestCallbackBudget therefore re-executes the test binary as a child
// process that creates callbacks until creation fails, and the parent reads
// the result from the child's output.

// budgetChildEnv marks the child process and says what it measures:
// "slots" (create fresh callbacks until creation fails) or "dedup" (the
// same function twice).
const budgetChildEnv = "GOGUI_FFI_BUDGET_CHILD"

// budgetExpect is what the analysis of 2026-10-12 found per OS: the free
// slots in a fresh test process, the slots taken before main (the Windows
// runtime's two; purego/objc's block dispose callback on darwin, imported by
// budget_darwin_test.go), how creation fails, and whether the same function
// twice gives the same pointer.
type budgetExpect struct {
	free, taken int
	failure     string // in the child's panic line (unix) or stderr (Windows)
	dedup       bool
}

// budgetHeadroom is how many free slots MaxCallbacks must leave on every
// target (S0.10b).
const budgetHeadroom = 64

func budgetExpectation(goos string) budgetExpect {
	switch goos {
	case "windows":
		return budgetExpect{free: 1998, taken: 2, failure: "too many callback functions", dedup: true}
	case "darwin":
		return budgetExpect{free: 1999, taken: 1, failure: "the maximum number of callbacks has been reached"}
	default:
		return budgetExpect{free: 2000, taken: 0, failure: "the maximum number of callbacks has been reached"}
	}
}

// budgetTopLevel is a top-level func for the dedup check: passing it twice
// passes the same funcval.
func budgetTopLevel(x uintptr) uintptr { return x }

func TestCallbackBudget(t *testing.T) {
	switch os.Getenv(budgetChildEnv) {
	case "slots":
		budgetChildSlots()
		return
	case "dedup":
		budgetChildDedup()
		return
	}
	want := budgetExpectation(runtime.GOOS)

	// Free slots and the failure mode.
	stdout, stderr, err := runBudgetChild(t, "slots")
	free := 0
	var panicMsg string
	sc := bufio.NewScanner(bytes.NewReader(stdout))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "budget-cb "):
			n, convErr := strconv.Atoi(strings.TrimPrefix(line, "budget-cb "))
			if convErr != nil || n != free+1 {
				t.Fatalf("child line %q after %d callbacks", line, free)
			}
			free = n
		case strings.HasPrefix(line, "budget-panic "):
			panicMsg = strings.TrimPrefix(line, "budget-panic ")
		}
	}
	if runtime.GOOS == "windows" {
		// The runtime throws: an unrecoverable fatal error that kills the child.
		if err == nil {
			t.Errorf("child exited cleanly after %d callbacks, want it killed by the runtime", free)
		}
		if !strings.Contains(string(stderr), "fatal error: "+want.failure) {
			t.Errorf("child stderr doesn't contain %q:\n%s", "fatal error: "+want.failure, firstLines(stderr, 5))
		}
	} else {
		// purego panics, which the child recovers and prints.
		if err != nil {
			t.Errorf("child failed: %v\nstderr:\n%s", err, firstLines(stderr, 20))
		}
		if !strings.Contains(panicMsg, want.failure) {
			t.Errorf("child's panic = %q, want one containing %q", panicMsg, want.failure)
		}
	}
	t.Logf("%s/%s: %d free callback slots, %d taken before main, so %d in total; MaxCallbacks = %d",
		runtime.GOOS, runtime.GOARCH, free, want.taken, free+want.taken, MaxCallbacks)
	if free != want.free {
		t.Errorf("%d free callback slots, want %d", free, want.free)
	}
	// The headroom (S0.10b): the budget leaves at least budgetHeadroom slots
	// free for callbacks the registry never sees (purego/objc's blocks and
	// RegisterClass, any other purego user in the process).
	if free < MaxCallbacks+budgetHeadroom {
		t.Errorf("only %d free slots, fewer than MaxCallbacks + %d = %d: the budget leaves too little headroom",
			free, budgetHeadroom, MaxCallbacks+budgetHeadroom)
	}

	// Deduplication.
	stdout, stderr, err = runBudgetChild(t, "dedup")
	if err != nil {
		t.Fatalf("dedup child failed: %v\nstderr:\n%s", err, firstLines(stderr, 20))
	}
	var same, fresh string
	sc = bufio.NewScanner(bytes.NewReader(stdout))
	for sc.Scan() {
		if f := strings.Fields(sc.Text()); len(f) == 3 && f[0] == "budget-dedup" {
			same, fresh = f[1], f[2]
		}
	}
	if same == "" {
		t.Fatalf("dedup child printed no result:\n%s", firstLines(stdout, 20))
	}
	t.Logf("%s/%s: same top-level func twice gives the same pointer: %s; a fresh closure gives a new one: %s",
		runtime.GOOS, runtime.GOARCH, same, fresh)
	if same != strconv.FormatBool(want.dedup) {
		t.Errorf("same func twice gives the same pointer: %s, want %v", same, want.dedup)
	}
	if fresh != "true" {
		t.Errorf("a fresh closure didn't get a new pointer")
	}
}

// runBudgetChild runs this test binary as a child that runs only
// TestCallbackBudget in the given mode, and returns its stdout and stderr.
// err is the child's exit error, if any.
func runBudgetChild(t *testing.T, mode string) (stdout, stderr []byte, err error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCallbackBudget$", "-test.count=1", "-test.v=false")
	cmd.Env = append(os.Environ(), budgetChildEnv+"="+mode)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err = cmd.Run()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		t.Fatalf("cannot run the child: %v", err)
	}
	return out.Bytes(), errOut.Bytes(), err
}

// budgetChildSlots creates fresh closures with nativeCallback (bypassing
// the registry, which would stop at MaxCallbacks) until creation fails. It
// writes a line after each one straight to stdout, so the count survives
// the Windows runtime killing the process.
func budgetChildSlots() {
	defer func() {
		if r := recover(); r != nil {
			msg := strings.ReplaceAll(panicString(r), "\n", " ")
			os.Stdout.WriteString("budget-panic " + msg + "\n")
		}
	}()
	for i := 1; ; i++ {
		// A new closure per iteration (Go 1.22 loop variables), so the
		// Windows runtime's deduplication by funcval doesn't apply.
		nativeCallback(func(x uintptr) uintptr { return x + uintptr(i) })
		os.Stdout.WriteString("budget-cb " + strconv.Itoa(i) + "\n")
		if i > 100000 {
			os.Stdout.WriteString("budget-panic no limit after 100000 callbacks\n")
			return
		}
	}
}

// budgetChildDedup passes the same top-level func twice, then a fresh
// closure, and prints whether the first two pointers are equal and the
// third differs.
func budgetChildDedup() {
	a := nativeCallback(budgetTopLevel)
	b := nativeCallback(budgetTopLevel)
	n := uintptr(len(os.Args))
	c := nativeCallback(func(x uintptr) uintptr { return x + n })
	os.Stdout.WriteString("budget-dedup " + strconv.FormatBool(a == b) + " " +
		strconv.FormatBool(c != a && c != b) + "\n")
}

func panicString(r any) string {
	switch v := r.(type) {
	case string:
		return v
	case error:
		return v.Error()
	}
	return "non-string panic"
}

func firstLines(b []byte, n int) string {
	lines := strings.SplitN(string(b), "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
