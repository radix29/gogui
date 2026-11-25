package gogui

import (
	"runtime"
	"testing"

	"github.com/radix29/gogui/internal/aqua"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/layout"
	fakeshell "github.com/radix29/gogui/internal/shell/fake"
)

// The tests of stack batching (review P19): Add syncs only what it
// appends, and moving or removing many children syncs each stack once, so
// building or emptying a stack of n children costs O(n), not O(n²).

// allocBytes returns the bytes f allocates.
func allocBytes(f func()) uint64 {
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	f()
	runtime.ReadMemStats(&b)
	return b.TotalAlloc - a.TotalAlloc
}

// checkStack fails unless s's container peer and layout node hold ws, in
// order.
func checkStack(t *testing.T, what string, s *Stack, ws []Widget) {
	t.Helper()
	nodes := s.node().(*layout.Stack).Children()
	if len(nodes) != len(ws) || len(s.Children()) != len(ws) {
		t.Fatalf("%s: %d layout nodes, %d children, want %d", what, len(nodes), len(s.Children()), len(ws))
	}
	var peers []driver.Widget
	switch c := s.peerWidget().(type) {
	case *aqua.Container:
		peers = c.Widgets()
	default:
		peers = children(s)
	}
	if len(peers) != len(ws) {
		t.Fatalf("%s: %d peers, want %d", what, len(peers), len(ws))
	}
	for i, w := range ws {
		if nodes[i] != w.node() || peers[i] != w.peerWidget() || w.base().parent != Widget(s) {
			t.Fatalf("%s: child %d out of sync", what, i)
		}
	}
}

// stackBatching runs the checks on the current driver.
func stackBatching(t *testing.T) {
	const n = 3000
	s := VStack(0)
	var ws []Widget
	add := func(k int) {
		for range k {
			w := NewLabel("x")
			ws = append(ws, w)
			s.Add(w)
		}
	}
	add(n - 200)
	// Old: every Add resent all n children (tens of KB per Add here).
	per := allocBytes(func() { add(200) }) / 200
	checkStack(t, "built by Add", s, ws)
	if per > 4<<10 {
		t.Errorf("%d bytes per Add on a stack of %d children: the whole list is resent", per, n)
	}

	// Re-adding the first child moves it to the end.
	s.Add(ws[0])
	ws = append(ws[1:], ws[0])
	checkStack(t, "re-added", s, ws)

	// Moving every child in one Add syncs the old stack once.
	other := HStack(0)
	moved := allocBytes(func() { other.Add(ws...) })
	checkStack(t, "moved", other, ws)
	checkStack(t, "emptied", s, nil)
	if moved > 4<<20 {
		t.Errorf("moving %d children allocated %d bytes: one sync per child", n, moved)
	}

	// Removing them all at once syncs once too.
	removed := allocBytes(func() { other.Remove(ws...) })
	checkStack(t, "removed", other, nil)
	for _, w := range ws {
		if w.base().parent != nil {
			t.Fatal("a removed child kept its parent")
		}
	}
	if removed > 4<<20 {
		t.Errorf("removing %d children allocated %d bytes: one sync per child", n, removed)
	}
}

func TestStackBatchingHeadless(t *testing.T) {
	runApp(t, func(*App) { stackBatching(t) })
}

func TestStackBatchingOnAqua(t *testing.T) {
	runAqua(t, func(app *App, sh *fakeshell.Shell) {
		stackBatching(t)
		// The element tree follows the incremental sync.
		a, b, c := NewLabel("a"), NewLabel("b"), NewLabel("c")
		s := VStack(0, a)
		s.Add(b, c)
		s.Add(a)
		cont := s.c.(*aqua.Container)
		want := []Widget{b, c, a}
		if cont.ChildCount() != len(want) {
			t.Fatalf("%d elements, want %d", cont.ChildCount(), len(want))
		}
		for i, w := range want {
			if cont.Child(i) != aqua.ElementOf(w.peerWidget()) {
				t.Fatalf("element %d out of order", i)
			}
		}
	})
}
