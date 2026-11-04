package ffi

import (
	"math/bits"
	"strconv"
	"sync"
)

// Handle is an integer that identifies a Go value in a Handles table. It is
// what native objects store in place of a Go pointer: an ObjC ivar, the
// user_data of a GObject signal or idle source, GWLP_USERDATA of an HWND.
//
// A Handle packs a slot index (the low indexBits bits) and the generation of
// that slot (the remaining bits). Freeing a handle bumps the generation of
// its slot, so a stale copy that native code still holds (a late event for a
// destroyed widget) no longer matches and Get reports it as invalid.
//
// The zero Handle is never valid, so a native field that was never set
// (NULL user_data, a zeroed ivar) never resolves to a value.
//
// A Handle is an integer, not a pointer. Pass it to native code as a
// uintptr argument and never convert it to unsafe.Pointer: its value may
// fall inside the Go heap, and the garbage collector rejects such bogus
// pointers.
type Handle uintptr

const (
	// indexBits is the number of bits of a Handle that hold the slot index.
	// It limits a table to 1<<24 - 1 (about 16.7 million) live handles.
	indexBits = 24
	// genBits is the number of bits that hold the generation: 40 on 64-bit
	// platforms, 8 on 32-bit ones.
	genBits   = bits.UintSize - indexBits
	indexMask = 1<<indexBits - 1
	// maxGen is the last generation a slot can have. A slot whose handle
	// with generation maxGen is freed is retired (never reused), so
	// generations never wrap around and a stale handle can never match a
	// newer value.
	maxGen = 1<<genBits - 1
	// maxSlots is the number of slots a table can have.
	maxSlots = indexMask
)

func makeHandle(index uint32, gen uintptr) Handle {
	return Handle(gen<<indexBits | uintptr(index))
}

func (h Handle) index() uint32 { return uint32(h & indexMask) }
func (h Handle) gen() uintptr  { return uintptr(h) >> indexBits }

// String returns "ffi.Handle(index#generation)", or "ffi.Handle(0)" for the
// zero Handle. It is meant for debug output and panics.
func (h Handle) String() string {
	if h == 0 {
		return "ffi.Handle(0)"
	}
	return "ffi.Handle(" + strconv.FormatUint(uint64(h.index()), 10) + "#" +
		strconv.FormatUint(uint64(h.gen()), 10) + ")"
}

type slot[T any] struct {
	val T
	// gen is the generation of the live handle, or of the next handle
	// allocated in this slot while it is free. It starts at 1, which keeps
	// every handle non-zero.
	gen  uintptr
	live bool
}

// Handles maps Handles to Go values of type T (PLAN.md §7.2). The zero value
// is an empty table ready to use. A Handles must not be copied after first
// use.
//
// The drivers use their tables on the UI thread. All methods are
// nevertheless safe for concurrent use, so a stray native callback on
// another thread (a destroy-notify, a late COM event) cannot corrupt the
// table. No method calls back into user code, so a method may be called
// while another value of the same table is being dispatched.
type Handles[T any] struct {
	mu    sync.Mutex
	slots []slot[T]
	free  []uint32 // indices of free, non-retired slots (used as a stack)
	live  int
	// limit overrides maxSlots in tests; 0 means maxSlots.
	limit int
}

// New stores v and returns a new, valid handle for it. It panics if the
// table already holds the maximum number of live handles (about 16.7
// million), which only a handle leak can cause.
func (t *Handles[T]) New(v T) Handle {
	t.mu.Lock()
	defer t.mu.Unlock()
	var i uint32
	if n := len(t.free); n > 0 {
		i = t.free[n-1]
		t.free = t.free[:n-1]
	} else {
		limit := t.limit
		if limit == 0 {
			limit = maxSlots
		}
		if len(t.slots) >= limit {
			panic("ffi: handle table is full (" + strconv.Itoa(t.live) +
				" live handles); are handles being leaked?")
		}
		i = uint32(len(t.slots))
		t.slots = append(t.slots, slot[T]{gen: 1})
	}
	s := &t.slots[i]
	s.val = v
	s.live = true
	t.live++
	return makeHandle(i, s.gen)
}

// lookup returns the slot of h if h is live, or nil. t.mu must be held.
func (t *Handles[T]) lookup(h Handle) *slot[T] {
	i := h.index()
	if int(i) >= len(t.slots) {
		return nil
	}
	s := &t.slots[i]
	if !s.live || s.gen != h.gen() {
		return nil
	}
	return s
}

// Get returns the value stored for h. If h is the zero Handle, has been
// freed, or does not belong to this table, it returns the zero T and false.
// Native callbacks must ignore events whose handle is not valid: they can
// arrive after the peer was destroyed.
func (t *Handles[T]) Get(h Handle) (T, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s := t.lookup(h); s != nil {
		return s.val, true
	}
	var zero T
	return zero, false
}

// Valid reports whether h is a live handle of this table.
func (t *Handles[T]) Valid(h Handle) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lookup(h) != nil
}

// Free invalidates h and drops the table's reference to its value, so the
// value can be garbage collected. Peers call it from Destroy. It reports
// whether h was live; freeing the zero Handle, a stale handle or a handle
// that was already freed does nothing and returns false.
//
// The slot is reused by a later New with the next generation. A slot whose
// generation is exhausted is retired instead.
func (t *Handles[T]) Free(h Handle) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.lookup(h)
	if s == nil {
		return false
	}
	var zero T
	s.val = zero
	s.live = false
	t.live--
	if s.gen < maxGen {
		s.gen++
		t.free = append(t.free, h.index())
	}
	return true
}

// Len returns the number of live handles. Tests use it to detect leaks.
func (t *Handles[T]) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.live
}
