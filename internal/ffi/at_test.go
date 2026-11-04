package ffi

import (
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

// atRect mirrors a RECT-like struct for the At tests.
type atRect struct {
	Left, Top, Right, Bottom int32
}

// atWide has uintptr alignment, so a 4-byte-aligned address is misaligned
// for it on 64-bit.
type atWide struct {
	A uintptr
	B int32
}

// checkAtRoundTrip reads and writes an atRect through At at base, which
// must be native memory (or pinned Go memory) of at least 16 bytes, 8-byte
// aligned, and checks that both directions go to the same bytes.
func checkAtRoundTrip(t *testing.T, base uintptr) {
	t.Helper()
	for i := range uintptr(4) {
		WriteInt32(base, i*4, int32(10*(i+1)))
	}
	r := At[atRect](base)
	if *r != (atRect{10, 20, 30, 40}) {
		t.Fatalf("At read %+v, want {10 20 30 40}", *r)
	}
	r.Right, r.Bottom = -7, 1<<30
	if got := ReadInt32(base, 8); got != -7 {
		t.Fatalf("after writing Right through At: int 2 = %d, want -7", got)
	}
	if got := ReadInt32(base, 12); got != 1<<30 {
		t.Fatalf("after writing Bottom through At: int 3 = %d, want %d", got, 1<<30)
	}
	if got := At[int32](base + 4); *got != 20 {
		t.Fatalf("At[int32](base+4) = %d, want 20", *got)
	}
}

// TestAtPinned uses a pinned Go array as native memory, like
// TestReadWriteUintptrPinned; drivers pass native addresses only.
func TestAtPinned(t *testing.T) {
	arr := new([2]uint64)
	var pin runtime.Pinner
	pin.Pin(arr)
	defer pin.Unpin()
	checkAtRoundTrip(t, uintptr(unsafe.Pointer(arr)))
}

func TestAtPanics(t *testing.T) {
	arr := new([4]uint64)
	var pin runtime.Pinner
	pin.Pin(arr)
	defer pin.Unpin()
	base := uintptr(unsafe.Pointer(arr))

	cases := []struct {
		name string
		f    func()
		want string
	}{
		{"NULL", func() { At[atRect](0) }, "ffi: At: NULL address"},
		{"misaligned int32", func() { At[atRect](base + 2) }, "is not 4-byte aligned"},
	}
	if unsafe.Alignof(uintptr(0)) == 8 {
		cases = append(cases, struct {
			name string
			f    func()
			want string
		}{"misaligned word", func() { At[atWide](base + 4) }, "is not 8-byte aligned"})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				s, _ := r.(string)
				if !strings.Contains(s, tc.want) {
					t.Fatalf("panic = %v, want it to contain %q", r, tc.want)
				}
			}()
			tc.f()
		})
	}
	// Byte-aligned types accept any non-NULL address.
	if p := At[byte](base + 3); p == nil {
		t.Fatal("At[byte] returned nil")
	}
}
