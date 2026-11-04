package gtk4

import (
	"runtime"
	"testing"
	"unsafe"

	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

func TestCheckWidgetType(t *testing.T) {
	for _, c := range []struct {
		q       gtk.TypeQuery
		off     uintptr
		wantErr bool
	}{
		{gtk.TypeQuery{Type: 0x55, ClassSize: 408, InstanceSize: 32}, 32, false},
		{gtk.TypeQuery{Type: 0x55, ClassSize: 408, InstanceSize: 28}, 32, false},
		{gtk.TypeQuery{Type: 0x55, ClassSize: 408, InstanceSize: 1}, 8, false},
		{gtk.TypeQuery{Type: 0x55, ClassSize: 416, InstanceSize: 32}, 0, true},
		{gtk.TypeQuery{Type: 0x55, ClassSize: 408, InstanceSize: 0}, 0, true},
		{gtk.TypeQuery{ClassSize: 408, InstanceSize: 32}, 0, true},
	} {
		off, err := checkWidgetType(c.q)
		if (err != nil) != c.wantErr || off != c.off {
			t.Errorf("checkWidgetType(%+v) = %d, %v; want %d, error %v", c.q, off, err, c.off, c.wantErr)
		}
	}
}

// The slot offsets are GtkWidgetClass's on LP64: a GObjectClass of 136
// bytes, then the vfuncs in header order (show at 0 … size_allocate at
// index 8, measure at 12, snapshot at 23), then priv and padding[8].
func TestWidgetClassSlots(t *testing.T) {
	const objectClass = 136
	for _, c := range []struct {
		name      string
		off, slot uintptr
	}{{"size_allocate", slotSizeAllocate, 8}, {"measure", slotMeasure, 12}, {"snapshot", slotSnapshot, 23}} {
		if want := objectClass + c.slot*wordSize; c.off != want {
			t.Errorf("%s slot at %d, want %d", c.name, c.off, want)
		}
	}
	if want := objectClass + (25+1+8)*wordSize; widgetClassSize != want {
		t.Errorf("widgetClassSize = %d, want %d", widgetClassSize, want)
	}
}

// fakeInstance is pinned Go memory standing in for a GGSurface instance
// in the host tests, with its handle word at class.handleOff.
var lastInstance *fakeInstance

type fakeInstance struct {
	words [8]uintptr
}

// withFakeClass sets class.handleOff and parentSnapshot for a test, and
// returns the address of a pinned fake instance (lastInstance keeps its
// Go pointer for the checks).
func withFakeClass(t *testing.T, parent func(w, s uintptr)) uintptr {
	t.Helper()
	saved := class
	t.Cleanup(func() { class = saved })
	class.handleOff = 5 * wordSize
	class.parentSnapshot = parent
	inst := new(fakeInstance)
	var pin runtime.Pinner
	pin.Pin(inst)
	t.Cleanup(pin.Unpin)
	lastInstance = inst
	return uintptr(unsafe.Pointer(inst))
}

// recSurface is a surfacePeer that records its calls.
type recSurface struct {
	log      *[]string
	min, nat [2]int32
	forSizes []int32
	alloc    [3]int32
	panics   any
}

func (p *recSurface) measure(o, forSize int32) (int32, int32) {
	p.forSizes = append(p.forSizes, forSize)
	if p.panics != nil {
		panic(p.panics)
	}
	// Honour for_size: the size along o grows with the other one.
	extra := max(forSize, 0) / 10
	return p.min[o] + extra, p.nat[o] + extra
}

func (p *recSurface) sizeAllocate(w, h, b int32) { p.alloc = [3]int32{w, h, b} }

func (p *recSurface) snapshot(uintptr) {
	*p.log = append(*p.log, "peer")
	if p.panics != nil {
		panic(p.panics)
	}
}

func TestSurfaceHandleWord(t *testing.T) {
	resetPanics(t)
	w := withFakeClass(t, nil)
	if _, ok := surfacePeerOf(w); ok {
		t.Error("an instance with handle 0 has a peer")
	}
	p := &recSurface{log: new([]string)}
	h := newEntry(t, p, kindSurface)
	setSurfaceHandle(w, h)
	if got := surfaceHandle(w); got != h {
		t.Fatalf("handle word read back as %v, want %v", got, h)
	}
	if got, ok := surfacePeerOf(w); !ok || got != p {
		t.Errorf("surfacePeerOf = %v, %v; want the peer", got, ok)
	}
	inst := lastInstance
	for i, v := range inst.words {
		if i != 5 && v != 0 {
			t.Errorf("word %d changed to %#x", i, v)
		}
	}
	// A handle of another kind is not a surface.
	other := newEntry(t, &recPeer{}, kindTestA)
	setSurfaceHandle(w, other)
	if _, ok := surfacePeerOf(w); ok {
		t.Error("a non-surface entry resolved as a surface peer")
	}
	// Before the class is registered every instance is peerless.
	setSurfaceHandle(w, h)
	class.handleOff = 0
	if _, ok := surfacePeerOf(w); ok {
		t.Error("an instance has a peer before the class is registered")
	}
}

// sentinel surrounds the ints measure writes, so a write wider than 32
// bits shows up.
const sentinel int32 = 0x5a5a5a5a

func TestSurfaceMeasure(t *testing.T) {
	resetPanics(t)
	w := withFakeClass(t, nil)
	p := &recSurface{log: new([]string), min: [2]int32{40, 30}, nat: [2]int32{120, 80}}
	setSurfaceHandle(w, newEntry(t, p, kindSurface))

	// [s, min, s, nat, s, minBaseline, s, natBaseline, s]
	buf := new([9]int32)
	var pin runtime.Pinner
	pin.Pin(buf)
	defer pin.Unpin()
	at := func(i uintptr) uintptr { return uintptr(unsafe.Pointer(buf)) + 4*i }
	reset := func() {
		for i := range buf {
			buf[i] = sentinel
		}
	}
	checkSentinels := func(what string) {
		t.Helper()
		for _, i := range []int{0, 2, 4, 6, 8} {
			if buf[i] != sentinel {
				t.Errorf("%s: the int at %d changed to %#x", what, i, buf[i])
			}
		}
	}

	for _, c := range []struct {
		o, forSize, mn, nt int32
	}{
		{gtk.OrientationHorizontal, -1, 40, 120},
		{gtk.OrientationVertical, -1, 30, 80},
		{gtk.OrientationVertical, 300, 60, 110},
	} {
		reset()
		surfaceMeasure(w, c.o, c.forSize, at(1), at(3), at(5), at(7))
		if buf[1] != c.mn || buf[3] != c.nt || buf[5] != -1 || buf[7] != -1 {
			t.Errorf("measure(%d, %d) wrote %d, %d, baselines %d, %d; want %d, %d, -1, -1",
				c.o, c.forSize, buf[1], buf[3], buf[5], buf[7], c.mn, c.nt)
		}
		checkSentinels("measure")
	}
	if got := p.forSizes; len(got) != 3 || got[0] != -1 || got[2] != 300 {
		t.Errorf("the peer saw for_size %v, want [-1 -1 300]", got)
	}

	// NULL pointers are skipped.
	reset()
	surfaceMeasure(w, gtk.OrientationHorizontal, -1, 0, at(3), 0, 0)
	if buf[1] != sentinel || buf[3] != 120 || buf[5] != sentinel {
		t.Errorf("with NULL pointers: %v", buf)
	}

	// A panicking peer still gets zeros written, and the panic is kept.
	p.panics = "measure"
	reset()
	surfaceMeasure(w, gtk.OrientationHorizontal, -1, at(1), at(3), at(5), at(7))
	if buf[1] != 0 || buf[3] != 0 || buf[5] != -1 || buf[7] != -1 {
		t.Errorf("after a panic: %v, want zero sizes", buf)
	}
	checkSentinels("panic")
	if pendingPanic == nil || pendingPanic.Value != "measure" || pendingPanic.Callback != nameMeasure {
		t.Errorf("pendingPanic = %+v", pendingPanic)
	}

	// Without a peer the sizes are 0.
	setSurfaceHandle(w, 0)
	reset()
	surfaceMeasure(w, gtk.OrientationHorizontal, -1, at(1), at(3), 0, 0)
	if buf[1] != 0 || buf[3] != 0 {
		t.Errorf("without a peer: min %d, nat %d; want 0, 0", buf[1], buf[3])
	}
}

func TestSurfaceSizeAllocate(t *testing.T) {
	resetPanics(t)
	w := withFakeClass(t, nil)
	p := &recSurface{log: new([]string)}
	setSurfaceHandle(w, newEntry(t, p, kindSurface))
	surfaceSizeAllocate(w, 173, 61, -1)
	if p.alloc != [3]int32{173, 61, -1} {
		t.Errorf("sizeAllocate got %v, want [173 61 -1]", p.alloc)
	}
	setSurfaceHandle(w, 0)
	surfaceSizeAllocate(w, 1, 2, 3) // no peer: nothing happens
	if p.alloc != [3]int32{173, 61, -1} {
		t.Errorf("a peerless size_allocate reached the peer: %v", p.alloc)
	}
}

// TestSurfaceSnapshot: the peer draws, then GtkWidget's snapshot runs on
// the same widget and snapshot, also when the peer panicked or there is
// no peer.
func TestSurfaceSnapshot(t *testing.T) {
	resetPanics(t)
	var log []string
	var parentArgs [2]uintptr
	w := withFakeClass(t, func(w, s uintptr) {
		log = append(log, "parent")
		parentArgs = [2]uintptr{w, s}
	})
	p := &recSurface{log: &log}
	setSurfaceHandle(w, newEntry(t, p, kindSurface))

	surfaceSnapshot(w, 0x5a)
	if len(log) != 2 || log[0] != "peer" || log[1] != "parent" || parentArgs != [2]uintptr{w, 0x5a} {
		t.Errorf("snapshot order %v, parent got %#x; want [peer parent] with (w, 0x5a)", log, parentArgs)
	}

	log = nil
	p.panics = "draw"
	surfaceSnapshot(w, 0x5b)
	if len(log) != 2 || log[1] != "parent" {
		t.Errorf("after a panicking peer: %v, want the parent to run", log)
	}
	if pendingPanic == nil || pendingPanic.Value != "draw" || pendingPanic.Callback != nameSnapshot {
		t.Errorf("pendingPanic = %+v", pendingPanic)
	}

	log = nil
	setSurfaceHandle(w, 0)
	surfaceSnapshot(w, 0x5c)
	if len(log) != 1 || log[0] != "parent" {
		t.Errorf("without a peer: %v, want only the parent", log)
	}

	class.parentSnapshot = nil
	log = nil
	surfaceSnapshot(w, 0x5d) // a NULL parent slot is skipped
	if len(log) != 0 {
		t.Errorf("with no parent and no peer: %v", log)
	}
}
