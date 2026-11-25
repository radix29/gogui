package layout

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
)

// The tests of pass reuse (review P7) and stack appends (P19).

// passTree is a form beside a column of rows of expanding and fixed leaves
// with spacers, as a window's tree is.
func passTree() (Node, []*probe) {
	var probes []*probe
	leaf := func(w, h float64) *Leaf {
		l, p := pl(w, h)
		probes = append(probes, p)
		return l
	}
	var rows []Node
	for i := range 12 {
		a, b := leaf(40, 20), leaf(60, 22)
		a.Props().SetExpand(Horizontal, i%2 == 0)
		rows = append(rows, NewHStack(8, a, NewSpacer(), b))
	}
	var frows []Row
	for range 6 {
		frows = append(frows, Row{Label: &probe{fit: driver.Size{W: 50, H: 16}}, Control: leaf(120, 22)})
	}
	col := NewVStack(6, rows...)
	return NewHStack(12, NewForm(frows...), NewVStack(4, col, NewSpacer())).SetPadding(driver.UniformInsets(20)), probes
}

// TestRootReusesPass: a root lays out with the same results through its
// reused pass, without allocating once warm, and asks each leaf for its
// unbounded size once per pass.
func TestRootReusesPass(t *testing.T) {
	n, probes := passTree()
	r := NewRoot(n)
	size := driver.Size{W: 640, H: 480}
	min := r.LayoutMin(size)
	frames := make([]driver.Rect, len(probes))
	for i, p := range probes {
		frames[i] = p.frame
	}

	fresh, fprobes := passTree()
	if got := NewRoot(fresh).LayoutMin(size); got != min {
		t.Fatalf("fresh root min %v, reused %v", got, min)
	}
	for i, p := range fprobes {
		if p.frame != frames[i] {
			t.Fatalf("leaf %d: fresh %v, reused %v", i, p.frame, frames[i])
		}
	}

	if a := testing.AllocsPerRun(20, func() { r.LayoutMin(size) }); a != 0 {
		t.Errorf("%v allocations per warm LayoutMin", a)
	}
	for i, p := range probes {
		if p.frame != frames[i] {
			t.Fatalf("leaf %d moved on a repeated layout", i)
		}
	}

	before := measures(probes)
	r.LayoutMin(size)
	// Unbounded once per leaf, plus height-for-width where a width
	// differs from the fitting one.
	if got := measures(probes) - before; got > 2*len(probes) {
		t.Errorf("%d FittingSize calls for %d leaves in one pass", got, len(probes))
	}
}

// TestStackAppendChildren: AppendChildren is SetChildren with the old
// children first.
func TestStackAppendChildren(t *testing.T) {
	a, _ := pl(1, 1)
	b, _ := pl(2, 2)
	c, _ := pl(3, 3)
	s := NewVStack(0, a)
	s.AppendChildren(b, c)
	if got := s.Children(); len(got) != 3 || got[0] != Node(a) || got[1] != Node(b) || got[2] != Node(c) {
		t.Fatalf("children %v", got)
	}
	defer func() {
		if recover() == nil {
			t.Error("a nil child did not panic")
		}
	}()
	s.AppendChildren(nil)
}

func BenchmarkRootLayoutMin(b *testing.B) {
	n, _ := passTree()
	r := NewRoot(n)
	b.ReportAllocs()
	for b.Loop() {
		r.LayoutMin(driver.Size{W: 640, H: 480})
	}
}
