package shellkit

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

const ms = time.Millisecond

// TestTimersOrder: timers fire in deadline order, equal deadlines in the
// order they were added; Next, NextAfter and Deadlines follow the list.
func TestTimersOrder(t *testing.T) {
	var ts Timers
	var got []string
	add := func(at time.Duration, name string) *Timer {
		return ts.Add(at, func() { got = append(got, name) })
	}
	add(3*ms, "c1")
	add(1*ms, "a")
	add(3*ms, "c2")
	add(2*ms, "b")
	if next, ok := ts.Next(); !ok || next != 1*ms {
		t.Errorf("Next = %v, %v; want 1ms", next, ok)
	}
	if next, ok := ts.NextAfter(1 * ms); !ok || next != 2*ms {
		t.Errorf("NextAfter(1ms) = %v, %v; want 2ms", next, ok)
	}
	if _, ok := ts.NextAfter(3 * ms); ok {
		t.Error("NextAfter(3ms) found a deadline")
	}
	if d := ts.Deadlines(); !slices.Equal(d, []time.Duration{1 * ms, 2 * ms, 3 * ms, 3 * ms}) {
		t.Errorf("Deadlines = %v", d)
	}
	if ts.Fire(0, nil) {
		t.Error("Fire before the first deadline reported a due timer")
	}
	if !ts.Fire(3*ms, nil) || !slices.Equal(got, []string{"a", "b", "c1", "c2"}) || ts.Len() != 0 {
		t.Errorf("fired %v, %d left", got, ts.Len())
	}
	if _, ok := ts.Next(); ok {
		t.Error("Next on an empty list reported a deadline")
	}
}

// TestTimersRound (review B9): a timer a fn adds, even one already due,
// waits for the next round, so a fn that re-arms itself runs once per
// round; older timers due at the same time still fire in the round.
func TestTimersRound(t *testing.T) {
	var ts Timers
	n := 0
	var tick func()
	tick = func() {
		n++
		ts.Add(0, tick) // due at once
	}
	ts.Add(1*ms, tick)
	other := false
	ts.Add(1*ms, func() { other = true })
	if !ts.Fire(1*ms, nil) || n != 1 || !other {
		t.Fatalf("first round: tick %d times, other %v; want 1, true", n, other)
	}
	for round := 2; round <= 4; round++ {
		ts.Fire(1*ms, nil)
		if n != round || ts.Len() != 1 {
			t.Fatalf("round %d: tick %d times, %d pending", round, n, ts.Len())
		}
	}
}

// TestTimersRemoveStopPanic: Remove of a pending timer, of a fired one, and
// from inside a round; stop ends a round early; a panicking fn leaves the
// rest pending.
func TestTimersRemoveStopPanic(t *testing.T) {
	var ts Timers
	var got []string
	var b *Timer
	a := ts.Add(1*ms, func() {
		got = append(got, "a")
		if !ts.Remove(b) {
			t.Error("Remove from inside a round found no b")
		}
	})
	b = ts.Add(1*ms, func() { got = append(got, "b") })
	c := ts.Add(1*ms, func() { got = append(got, "c") })
	ts.Fire(1*ms, nil)
	if !slices.Equal(got, []string{"a", "c"}) || ts.Remove(a) || ts.Remove(c) {
		t.Errorf("fired %v; Remove after firing must report false", got)
	}

	got = nil
	stop := false
	ts.Add(1*ms, func() { got = append(got, "x"); stop = true })
	ts.Add(1*ms, func() { got = append(got, "y") })
	ts.Fire(1*ms, func() bool { return stop })
	if !slices.Equal(got, []string{"x"}) || ts.Len() != 1 {
		t.Errorf("stop: fired %v, %d left; want [x], 1", got, ts.Len())
	}
	ts.Clear()

	ts.Add(1*ms, func() { panic("boom") })
	ts.Add(2*ms, func() {})
	func() {
		defer func() {
			if r := recover(); r != "boom" {
				t.Errorf("recover = %v, want boom", r)
			}
		}()
		ts.Fire(2*ms, nil)
	}()
	if ts.Len() != 1 {
		t.Errorf("%d timers left after the panic, want 1", ts.Len())
	}
	ts.Clear()
	if ts.Len() != 0 {
		t.Error("Clear left timers")
	}
}

// TestShellsDoNotImportFake (review R6): the shipped files of the native
// shells do not import shell/fake, so the test double is not linked into
// apps; they use this package's FixedText as their fallback.
func TestShellsDoNotImportFake(t *testing.T) {
	for _, dir := range []string{"../win32", "../gtk4"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("%s: %d files, %v", dir, len(files), err)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range file.Imports {
				if p, _ := strconv.Unquote(imp.Path.Value); strings.HasSuffix(p, "/internal/shell/fake") {
					t.Errorf("%s imports %s", f, p)
				}
			}
		}
	}
}
