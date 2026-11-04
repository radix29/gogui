package gtk4

import (
	"testing"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// Test kinds; the shell's own kinds start at kindSurface.
const (
	kindTestA kind = 200 + iota
	kindTestB
)

// recPeer implements every signal peer interface and records the calls.
type recPeer struct {
	calls  []recCall
	result bool // what boolSignal and source return
	panics any  // panicked with, if non-nil
}

type recCall struct {
	sig      string
	k        kind
	instance uintptr
	n        int32
	x, y     float64
}

func (p *recPeer) boolSignal(k kind, instance uintptr) bool {
	p.calls = append(p.calls, recCall{sig: "bool", k: k, instance: instance})
	if p.panics != nil {
		panic(p.panics)
	}
	return p.result
}

func (p *recPeer) intDoubleDouble(k kind, instance uintptr, n int32, x, y float64) {
	p.calls = append(p.calls, recCall{sig: "idd", k: k, instance: instance, n: n, x: x, y: y})
	if p.panics != nil {
		panic(p.panics)
	}
}

func (p *recPeer) doubleDouble(k kind, instance uintptr, x, y float64) {
	p.calls = append(p.calls, recCall{sig: "dd", k: k, instance: instance, x: x, y: y})
	if p.panics != nil {
		panic(p.panics)
	}
}

func (p *recPeer) source(k kind) bool {
	p.calls = append(p.calls, recCall{sig: "source", k: k})
	if p.panics != nil {
		panic(p.panics)
	}
	return p.result
}

// newEntry adds {peer, k} to the dispatch table for the test's duration.
func newEntry(t *testing.T, peer any, k kind) ffi.Handle {
	t.Helper()
	h := dispatch.New(entry{peer: peer, kind: k})
	t.Cleanup(func() { dispatch.Free(h) })
	return h
}

func checkLen(t *testing.T, want int) {
	t.Helper()
	if got := dispatch.Len(); got != want {
		t.Errorf("dispatch.Len() = %d, want %d", got, want)
	}
}

func TestSigBool(t *testing.T) {
	resetPanics(t)
	base := dispatch.Len()
	p := &recPeer{result: true}
	ha := newEntry(t, p, kindTestA)
	hb := newEntry(t, p, kindTestB)

	if r := sigBool(0x1000, uintptr(ha)); r != gtk.True {
		t.Errorf("sigBool with a true peer = %d, want TRUE", r)
	}
	p.result = false
	if r := sigBool(0x2000, uintptr(hb)); r != gtk.False {
		t.Errorf("sigBool with a false peer = %d, want FALSE", r)
	}
	want := []recCall{{sig: "bool", k: kindTestA, instance: 0x1000}, {sig: "bool", k: kindTestB, instance: 0x2000}}
	if len(p.calls) != 2 || p.calls[0] != want[0] || p.calls[1] != want[1] {
		t.Errorf("calls %+v, want %+v", p.calls, want)
	}

	// A zero, stale or foreign handle reaches no peer.
	stale := dispatch.New(entry{peer: p, kind: kindTestA})
	dispatch.Free(stale)
	for _, h := range []ffi.Handle{0, stale, 12345} {
		if r := sigBool(0x3000, uintptr(h)); r != gtk.False {
			t.Errorf("sigBool(%v) = %d, want FALSE", h, r)
		}
	}
	if len(p.calls) != 2 {
		t.Errorf("a stale handle reached the peer: %+v", p.calls[2:])
	}

	// A panic is kept and returns FALSE.
	p.result, p.panics = true, "close"
	if r := sigBool(0x4000, uintptr(ha)); r != gtk.False {
		t.Errorf("a panicking sigBool = %d, want FALSE", r)
	}
	if pendingPanic == nil || pendingPanic.Value != "close" || pendingPanic.Callback != nameSigBool {
		t.Errorf("pendingPanic = %+v", pendingPanic)
	}
	checkLen(t, base+2) // signal handles are freed by closureNotify only
}

func TestSigIntDoubleDouble(t *testing.T) {
	resetPanics(t)
	p := &recPeer{}
	h := newEntry(t, p, kindTestB)
	sigIntDoubleDouble(0x10, 2, 12.5, -3.25, uintptr(h))
	sigIntDoubleDouble(0x10, -7, 1e300, 0.1, uintptr(h))
	sigIntDoubleDouble(0x10, 1, 0, 0, 0)
	want := []recCall{
		{sig: "idd", k: kindTestB, instance: 0x10, n: 2, x: 12.5, y: -3.25},
		{sig: "idd", k: kindTestB, instance: 0x10, n: -7, x: 1e300, y: 0.1},
	}
	if len(p.calls) != 2 || p.calls[0] != want[0] || p.calls[1] != want[1] {
		t.Errorf("calls %+v, want %+v", p.calls, want)
	}
	if pendingPanic != nil {
		t.Errorf("unexpected panic %v", pendingPanic)
	}
}

// TestSigDoubleDouble: exact doubles with the entry's kind; a stale handle
// reaches no peer; a panic is kept.
func TestSigDoubleDouble(t *testing.T) {
	resetPanics(t)
	p := &recPeer{}
	h := newEntry(t, p, kindTestA)
	sigDoubleDouble(0x20, 12.5, -3.25, uintptr(h))
	sigDoubleDouble(0x20, 1e300, 0.1, uintptr(h))
	sigDoubleDouble(0x20, 1, 1, 0)
	want := []recCall{
		{sig: "dd", k: kindTestA, instance: 0x20, x: 12.5, y: -3.25},
		{sig: "dd", k: kindTestA, instance: 0x20, x: 1e300, y: 0.1},
	}
	if len(p.calls) != 2 || p.calls[0] != want[0] || p.calls[1] != want[1] {
		t.Errorf("calls %+v, want %+v", p.calls, want)
	}
	if pendingPanic != nil {
		t.Errorf("unexpected panic %v", pendingPanic)
	}
	p.panics = "drag"
	sigDoubleDouble(0x20, 0, 0, uintptr(h))
	if pendingPanic == nil || pendingPanic.Value != "drag" || pendingPanic.Callback != nameSigDoubleDouble {
		t.Errorf("pendingPanic = %+v", pendingPanic)
	}
}

// boolOnly implements only boolSignalPeer.
type boolOnly struct{}

func (boolOnly) boolSignal(kind, uintptr) bool { return true }

// TestWrongPeer: a peer connected through a trampoline whose interface it
// lacks panics inside the trampoline, which keeps the panic.
func TestWrongPeer(t *testing.T) {
	resetPanics(t)
	h := newEntry(t, boolOnly{}, kindTestA)
	sigIntDoubleDouble(0, 1, 2, 3, uintptr(h))
	if pendingPanic == nil || pendingPanic.Callback != nameSigIntDoubleDouble {
		t.Fatalf("pendingPanic = %+v, want the failed type assertion", pendingPanic)
	}
	if _, ok := pendingPanic.Value.(error); !ok {
		t.Errorf("panic value %T, want a runtime error", pendingPanic.Value)
	}
}

func TestSourceFunc(t *testing.T) {
	resetPanics(t)
	base := dispatch.Len()
	p := &recPeer{result: true}
	h := dispatch.New(entry{peer: p, kind: kindTestA})
	defer dispatch.Free(h)

	if r := sourceFunc(uintptr(h)); r != gtk.SourceContinue {
		t.Errorf("a source that stays = %d, want G_SOURCE_CONTINUE", r)
	}
	if !dispatch.Valid(h) {
		t.Fatal("a source that stays lost its handle")
	}
	p.result = false
	if r := sourceFunc(uintptr(h)); r != gtk.SourceRemove {
		t.Errorf("a source that goes = %d, want G_SOURCE_REMOVE", r)
	}
	if dispatch.Valid(h) {
		t.Error("a source that goes kept its handle")
	}
	if r := sourceFunc(uintptr(h)); r != gtk.SourceRemove {
		t.Errorf("a stale source = %d, want G_SOURCE_REMOVE", r)
	}
	if len(p.calls) != 2 || p.calls[0].k != kindTestA {
		t.Errorf("calls %+v, want two with kindTestA", p.calls)
	}

	// A panicking source is removed and its handle freed.
	p.result, p.panics = true, "tick"
	h2 := dispatch.New(entry{peer: p, kind: kindTestB})
	if r := sourceFunc(uintptr(h2)); r != gtk.SourceRemove {
		t.Errorf("a panicking source = %d, want G_SOURCE_REMOVE", r)
	}
	if dispatch.Valid(h2) {
		dispatch.Free(h2)
		t.Error("a panicking source kept its handle")
	}
	if pendingPanic == nil || pendingPanic.Value != "tick" || pendingPanic.Callback != nameSourceFunc {
		t.Errorf("pendingPanic = %+v", pendingPanic)
	}
	checkLen(t, base)
}

func TestClosureNotify(t *testing.T) {
	resetPanics(t)
	base := dispatch.Len()
	h := dispatch.New(entry{peer: &recPeer{}, kind: kindTestA})
	closureNotify(uintptr(h), 0xc105)
	if dispatch.Valid(h) {
		t.Error("closureNotify left the handle live")
	}
	closureNotify(uintptr(h), 0xc105) // a second notify, or a stale one, does nothing
	closureNotify(0, 0)
	checkLen(t, base)
	if pendingPanic != nil {
		t.Errorf("unexpected panic %v", pendingPanic)
	}
}
