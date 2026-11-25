package aqua

import (
	"testing"
	"time"
)

// TestTypeSelect (review R5): the menus' and the strip's one type-select.
// A prefix grows within menuTypeSelectReset from the highlighted item on;
// the same letter again, or a letter no grown prefix matches, steps to the
// next item it starts, wrapping; disabled items are skipped, and a space
// only grows a prefix.
func TestTypeSelect(t *testing.T) {
	titles := []string{"Apple", "Banana", "Blueberry", "Cherry", "Cranberry", "C D", "berry"}
	disabled := map[int]bool{3: true}
	var s typeSelect
	cur := -1
	now := time.Duration(0)
	steps := []struct {
		r    rune
		dt   time.Duration
		want int
	}{
		{'b', 0, 1},                      // the first b
		{'L', 10 * time.Millisecond, 2},  // "bl"
		{'b', menuTypeSelectReset, 6},    // a new b after Blueberry
		{'b', 10 * time.Millisecond, 1},  // "bb" matches nothing: the next b, wrapping
		{'b', 10 * time.Millisecond, 2},  // again
		{'c', menuTypeSelectReset, 4},    // past the disabled Cherry
		{' ', 10 * time.Millisecond, 5},  // "c " grows to C D
		{'d', 10 * time.Millisecond, 5},  // "c d"
		{'x', 10 * time.Millisecond, -1}, // no match: the highlight stays
		{' ', menuTypeSelectReset, -1},   // a space starts nothing
		{'A', 10 * time.Millisecond, 0},  // case-insensitive
	}
	for k, st := range steps {
		now += st.dt
		got := s.next(st.r, now, len(titles), cur, func(i int) string { return titles[i] }, func(i int) bool { return !disabled[i] })
		if got != st.want {
			t.Fatalf("step %d (%q): %d, want %d", k, st.r, got, st.want)
		}
		if got >= 0 {
			cur = got
		}
	}
	if !s.growing(now) || s.growing(now+menuTypeSelectReset) {
		t.Error("growing does not follow menuTypeSelectReset")
	}
	s.reset()
	if s.growing(now) {
		t.Error("growing after reset")
	}
}
