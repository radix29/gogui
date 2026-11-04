package ffi

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"unsafe"
	"weak"
)

func TestHandleZeroValueTable(t *testing.T) {
	var tab Handles[string]
	if n := tab.Len(); n != 0 {
		t.Fatalf("Len of empty table = %d, want 0", n)
	}
	if v, ok := tab.Get(0); ok || v != "" {
		t.Fatalf("Get(0) on empty table = %q, %v; want \"\", false", v, ok)
	}
	if tab.Free(0) {
		t.Fatal("Free(0) on empty table = true")
	}
}

func TestHandleNewGetFree(t *testing.T) {
	var tab Handles[string]
	a := tab.New("a")
	b := tab.New("b")
	if a == 0 || b == 0 {
		t.Fatalf("New returned the zero Handle: a=%v b=%v", a, b)
	}
	if a == b {
		t.Fatalf("two live values share handle %v", a)
	}
	for h, want := range map[Handle]string{a: "a", b: "b"} {
		if v, ok := tab.Get(h); !ok || v != want {
			t.Errorf("Get(%v) = %q, %v; want %q, true", h, v, ok, want)
		}
		if !tab.Valid(h) {
			t.Errorf("Valid(%v) = false", h)
		}
	}
	if n := tab.Len(); n != 2 {
		t.Fatalf("Len = %d, want 2", n)
	}

	if !tab.Free(a) {
		t.Fatal("Free(a) = false for a live handle")
	}
	if v, ok := tab.Get(a); ok || v != "" {
		t.Errorf("Get(a) after Free = %q, %v; want \"\", false", v, ok)
	}
	if tab.Valid(a) {
		t.Error("Valid(a) after Free = true")
	}
	if tab.Free(a) {
		t.Error("second Free(a) = true")
	}
	if v, ok := tab.Get(b); !ok || v != "b" {
		t.Errorf("Get(b) after Free(a) = %q, %v; want \"b\", true", v, ok)
	}
	if n := tab.Len(); n != 1 {
		t.Fatalf("Len = %d, want 1", n)
	}
}

func TestHandleStaleAfterReuse(t *testing.T) {
	var tab Handles[string]
	old := tab.New("old")
	tab.Free(old)
	cur := tab.New("new")

	if cur.index() != old.index() {
		t.Fatalf("freed slot was not reused: old=%v new=%v", old, cur)
	}
	if cur == old {
		t.Fatalf("reused slot kept its generation: %v", cur)
	}
	if cur.gen() != old.gen()+1 {
		t.Errorf("generation after reuse = %d, want %d", cur.gen(), old.gen()+1)
	}
	if v, ok := tab.Get(old); ok {
		t.Errorf("stale Get(old) = %q, true; want false", v)
	}
	if tab.Free(old) {
		t.Error("stale Free(old) = true")
	}
	if v, ok := tab.Get(cur); !ok || v != "new" {
		t.Errorf("Get(new) = %q, %v; want \"new\", true (stale Free must not touch it)", v, ok)
	}
}

func TestHandleForeignAndBogus(t *testing.T) {
	var tab Handles[int]
	h := tab.New(1)
	bogus := []Handle{
		makeHandle(h.index()+1, 1),       // slot beyond the table
		makeHandle(h.index(), h.gen()+1), // future generation
		makeHandle(h.index(), 0),         // generation 0 is never issued
		Handle(^uintptr(0)),              // all bits set
		makeHandle(indexMask, maxGen),    // largest index
	}
	for _, b := range bogus {
		if b == h {
			continue
		}
		if _, ok := tab.Get(b); ok {
			t.Errorf("Get(%v) = true for a handle that was never issued", b)
		}
		if tab.Free(b) {
			t.Errorf("Free(%v) = true for a handle that was never issued", b)
		}
	}
	if v, ok := tab.Get(h); !ok || v != 1 {
		t.Fatalf("Get(h) = %d, %v after bogus calls; want 1, true", v, ok)
	}
}

func TestHandleTablesAreIndependent(t *testing.T) {
	var t1, t2 Handles[string]
	h := t1.New("one")
	if _, ok := t2.Get(h); ok {
		t.Errorf("handle %v of one table is valid in another, empty table", h)
	}
}

func TestHandleFreeReleasesValue(t *testing.T) {
	// A non-zero size, so p is a real heap object and not the shared zero-size address.
	type peer struct{ buf [64]byte }
	var tab Handles[*peer]
	p := &peer{}
	p.buf[0] = 1
	w := weak.Make(p)
	h := tab.New(p)
	p = nil
	if !tab.Free(h) {
		t.Fatal("Free = false")
	}
	for i := 0; i < 10 && w.Value() != nil; i++ {
		runtime.GC()
	}
	if w.Value() != nil {
		t.Fatal("value is still reachable after Free; the table keeps a reference")
	}
}

func TestHandleSlotRetiresAtMaxGeneration(t *testing.T) {
	var tab Handles[string]
	h := tab.New("x")
	tab.Free(h)

	// Jump the free slot to its last generation instead of cycling 2^40
	// times.
	tab.slots[h.index()].gen = maxGen
	last := tab.New("last")
	if last.index() != h.index() || last.gen() != maxGen {
		t.Fatalf("New = %v, want slot %d with generation %d", last, h.index(), uint64(maxGen))
	}
	tab.Free(last)

	next := tab.New("next")
	if next.index() == h.index() {
		t.Fatalf("retired slot was reused: %v", next)
	}
	if _, ok := tab.Get(last); ok {
		t.Error("handle of a retired slot is still valid")
	}
	if len(tab.free) != 0 {
		t.Errorf("free list = %v, want empty (retired slot must not be listed)", tab.free)
	}
}

func TestHandleTableFull(t *testing.T) {
	tab := Handles[int]{limit: 3}
	hs := []Handle{tab.New(0), tab.New(1), tab.New(2)}

	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("New on a full table did not panic")
			}
			if s, _ := r.(string); !strings.Contains(s, "full") {
				t.Fatalf("panic = %v, want a \"table is full\" message", r)
			}
		}()
		tab.New(3)
	}()

	// A failed New must leave the table unchanged and usable.
	if n := tab.Len(); n != 3 {
		t.Fatalf("Len after failed New = %d, want 3", n)
	}
	tab.Free(hs[1])
	h := tab.New(4)
	if v, ok := tab.Get(h); !ok || v != 4 {
		t.Fatalf("Get after reuse = %d, %v; want 4, true", v, ok)
	}
}

func TestHandleLayout(t *testing.T) {
	if size := 8 * int(unsafe.Sizeof(Handle(0))); indexBits+genBits != size {
		t.Fatalf("indexBits + genBits = %d, want %d", indexBits+genBits, size)
	}
	for _, c := range []struct {
		index uint32
		gen   uintptr
	}{{0, 1}, {1, 1}, {indexMask, 1}, {7, maxGen}, {indexMask, maxGen}} {
		h := makeHandle(c.index, c.gen)
		if h.index() != c.index || h.gen() != c.gen {
			t.Errorf("makeHandle(%d, %d) round-trips to (%d, %d)", c.index, c.gen, h.index(), h.gen())
		}
		if h == 0 {
			t.Errorf("makeHandle(%d, %d) = 0", c.index, c.gen)
		}
	}
	if runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64" {
		if genBits != 40 {
			t.Errorf("genBits = %d on %s, want 40", genBits, runtime.GOARCH)
		}
	}
}

func TestHandleString(t *testing.T) {
	if s := Handle(0).String(); s != "ffi.Handle(0)" {
		t.Errorf("Handle(0).String() = %q", s)
	}
	if s := makeHandle(5, 3).String(); s != "ffi.Handle(5#3)" {
		t.Errorf("makeHandle(5, 3).String() = %q, want \"ffi.Handle(5#3)\"", s)
	}
}

// TestHandleConcurrent hammers one table from several goroutines: each
// allocates, checks and frees its own handles while also probing handles
// that other goroutines may be freeing at the same moment. Run it as a
// stress test: go test -count=20 -cpu=1,2,4,8 ./internal/ffi
func TestHandleConcurrent(t *testing.T) {
	const (
		workers = 8
		rounds  = 2000
		batch   = 16
	)
	var tab Handles[[2]int]
	var shared sync.Map // index -> Handle, recently issued by some worker
	var wg sync.WaitGroup
	errs := make(chan error, workers)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			mine := make([]Handle, 0, batch)
			for r := 0; r < rounds; r++ {
				mine = mine[:0]
				for i := 0; i < batch; i++ {
					h := tab.New([2]int{w, r*batch + i})
					mine = append(mine, h)
					shared.Store(h.index(), h)
				}
				for i, h := range mine {
					v, ok := tab.Get(h)
					if !ok || v != [2]int{w, r*batch + i} {
						errs <- fmt.Errorf("worker %d: Get(%v) = %v, %v; want [%d %d], true", w, h, v, ok, w, r*batch+i)
						return
					}
				}
				// Probe someone else's handle. It may be live or stale;
				// if it is live, its value must be the one it was issued
				// for, which the owner checks. Here we only make sure the
				// probe does not corrupt anything or panic.
				if x, ok := shared.Load(uint32(r % (workers * batch))); ok {
					tab.Get(x.(Handle))
					tab.Valid(x.(Handle))
				}
				for _, h := range mine {
					if !tab.Free(h) {
						errs <- fmt.Errorf("worker %d: Free(%v) = false for its own live handle", w, h)
						return
					}
					if tab.Free(h) {
						errs <- fmt.Errorf("worker %d: double Free(%v) = true", w, h)
						return
					}
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if n := tab.Len(); n != 0 {
		t.Errorf("Len after all workers freed their handles = %d, want 0", n)
	}
	// Freed slots are reused, so the table never grows past the peak
	// number of live handles. (With the 8-bit generations of 32-bit
	// platforms, slots retire after 255 uses and the table does grow.)
	if n := len(tab.slots); genBits >= 32 && n > workers*batch {
		t.Errorf("table has %d slots, want at most %d (freed slots must be reused)", n, workers*batch)
	}
}

func BenchmarkHandleGet(b *testing.B) {
	var tab Handles[*int]
	x := 1
	h := tab.New(&x)
	b.ReportAllocs()
	for b.Loop() {
		if _, ok := tab.Get(h); !ok {
			b.Fatal("Get failed")
		}
	}
}

func BenchmarkHandleNewFree(b *testing.B) {
	var tab Handles[*int]
	x := 1
	tab.Free(tab.New(&x)) // warm up the slot slice and free list
	b.ReportAllocs()
	for b.Loop() {
		tab.Free(tab.New(&x))
	}
}
