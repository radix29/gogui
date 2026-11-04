package ffi

import (
	"math"
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

const wordSize = unsafe.Sizeof(uintptr(0))

// checkWordRoundTrip writes a distinct value into each of n words at base
// with WriteUintptr, reads them back with ReadUintptr and checks that every
// write touched only its own word. base must be native memory or pinned Go
// memory of at least n words.
func checkWordRoundTrip(t *testing.T, base uintptr, n int) {
	t.Helper()
	for i := range n {
		WriteUintptr(base, uintptr(i)*wordSize, 0)
	}
	for i := range n {
		off := uintptr(i) * wordSize
		v := uintptr(0xA5A5_0000) | uintptr(i+1)
		WriteUintptr(base, off, v)
		for j := range n {
			want := uintptr(0)
			if j <= i {
				want = uintptr(0xA5A5_0000) | uintptr(j+1)
			}
			if got := ReadUintptr(base, uintptr(j)*wordSize); got != want {
				t.Fatalf("after writing word %d: word %d = %#x, want %#x", i, j, got, want)
			}
		}
	}
	// The full range of a uintptr survives, and an offset can be reached
	// from a base in the middle of the block.
	WriteUintptr(base, wordSize, ^uintptr(0))
	if got := ReadUintptr(base+wordSize, 0); got != ^uintptr(0) {
		t.Fatalf("ReadUintptr(base+word, 0) = %#x, want %#x", got, ^uintptr(0))
	}
}

// TestReadWriteUintptrPinned stands in for native memory with a pinned Go
// array. This test is the only place where a Go address goes into these
// helpers; drivers pass native addresses only (PLAN.md §7.2).
func TestReadWriteUintptrPinned(t *testing.T) {
	arr := new([8]uintptr)
	var pin runtime.Pinner
	pin.Pin(arr)
	defer pin.Unpin()
	base := uintptr(unsafe.Pointer(arr))

	checkWordRoundTrip(t, base, len(arr))
	for i, v := range arr {
		want := uintptr(0xA5A5_0000) | uintptr(i+1)
		if i == 1 {
			want = ^uintptr(0)
		}
		if v != want {
			t.Errorf("arr[%d] = %#x, want %#x (the helpers must write the words in place)", i, v, want)
		}
	}
}

func TestReadWriteUintptrPanics(t *testing.T) {
	arr := new([2]uintptr)
	var pin runtime.Pinner
	pin.Pin(arr)
	defer pin.Unpin()
	base := uintptr(unsafe.Pointer(arr))

	for _, tc := range []struct {
		name string
		f    func()
		want string
	}{
		{"read NULL", func() { ReadUintptr(0, 8) }, "ffi: ReadUintptr: NULL address"},
		{"write NULL", func() { WriteUintptr(0, 0, 1) }, "ffi: WriteUintptr: NULL address"},
		{"read misaligned offset", func() { ReadUintptr(base, 1) }, "is not uintptr-aligned"},
		{"write misaligned offset", func() { WriteUintptr(base, 3, 1) }, "is not uintptr-aligned"},
		{"read misaligned base", func() { ReadUintptr(base+1, 0) }, "+ offset 0 is not uintptr-aligned"},
	} {
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
	if arr[0] != 0 || arr[1] != 0 {
		t.Fatalf("a rejected write changed memory: %#x", *arr)
	}
}

// int32Values are the values checkInt32RoundTrip writes: both ends of the
// range, negatives (for_size = -1 must stay -1) and a few patterns.
var int32Values = []int32{-1, math.MinInt32, math.MaxInt32, 0, 1, -2, 0x1234_5678, -0x0F0F_0F0F}

// checkInt32RoundTrip writes a value into each of n 32-bit ints at base
// with WriteInt32, reads them back with ReadInt32 and checks that every
// write touched only its own int. Writing every value at every offset
// covers both halves of each word on 64-bit. base must be native memory or
// pinned Go memory of at least n*4 bytes, 4-byte aligned.
func checkInt32RoundTrip(t *testing.T, base uintptr, n int) {
	t.Helper()
	const fill = int32(0x5A5A_5A5A)
	for _, v := range int32Values {
		for i := range n {
			for j := range n {
				WriteInt32(base, uintptr(j)*4, fill)
			}
			WriteInt32(base, uintptr(i)*4, v)
			for j := range n {
				want := fill
				if j == i {
					want = v
				}
				if got := ReadInt32(base, uintptr(j)*4); got != want {
					t.Fatalf("after writing %d to int %d: int %d = %d (%#x), want %d",
						v, i, j, got, uint32(got), want)
				}
			}
		}
	}
	// An offset can be reached from a base in the middle of the block.
	WriteInt32(base, 4, math.MinInt32)
	if got := ReadInt32(base+4, 0); got != math.MinInt32 {
		t.Fatalf("ReadInt32(base+4, 0) = %d, want %d", got, int32(math.MinInt32))
	}
}

// TestReadWriteInt32Pinned uses a pinned Go array as native memory, like
// TestReadWriteUintptrPinned.
func TestReadWriteInt32Pinned(t *testing.T) {
	arr := new([8]int32)
	var pin runtime.Pinner
	pin.Pin(arr)
	defer pin.Unpin()
	base := uintptr(unsafe.Pointer(arr))

	checkInt32RoundTrip(t, base, len(arr))
	if arr[1] != math.MinInt32 {
		t.Fatalf("arr[1] = %d, want %d (the helpers must write in place)", arr[1], int32(math.MinInt32))
	}

	// The bug WriteInt32 guards against: a 32-bit out-parameter next to
	// another one in the same word. Writing one half must leave the other
	// half, and the words around them, as they were.
	words := new([3]uint64)
	pin.Pin(words)
	wbase := uintptr(unsafe.Pointer(words))
	const sentinel = uint64(0xDEAD_BEEF_CAFE_F00D)
	for half := range uintptr(2) {
		*words = [3]uint64{sentinel, sentinel, sentinel}
		WriteInt32(wbase, 8+half*4, -1)
		if words[0] != sentinel || words[2] != sentinel {
			t.Fatalf("half %d: a neighbouring word changed: %#x", half, *words)
		}
		other := ReadInt32(wbase, 8+(1-half)*4)
		s := sentinel
		if want := (*[2]int32)(unsafe.Pointer(&s))[1-half]; other != want {
			t.Fatalf("half %d: the other half = %#x, want %#x", half, uint32(other), uint32(want))
		}
		if got := ReadInt32(wbase, 8+half*4); got != -1 {
			t.Fatalf("half %d: read back %d, want -1", half, got)
		}
	}
}

func TestReadWriteInt32Panics(t *testing.T) {
	arr := new([2]int32)
	var pin runtime.Pinner
	pin.Pin(arr)
	defer pin.Unpin()
	base := uintptr(unsafe.Pointer(arr))

	for _, tc := range []struct {
		name string
		f    func()
		want string
	}{
		{"read NULL", func() { ReadInt32(0, 4) }, "ffi: ReadInt32: NULL address"},
		{"write NULL", func() { WriteInt32(0, 0, 1) }, "ffi: WriteInt32: NULL address"},
		{"read misaligned offset", func() { ReadInt32(base, 2) }, "is not 4-byte aligned"},
		{"write misaligned offset", func() { WriteInt32(base, 1, 1) }, "is not 4-byte aligned"},
		{"write misaligned base", func() { WriteInt32(base+3, 0, 1) }, "+ offset 0 is not 4-byte aligned"},
	} {
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
	if arr[0] != 0 || arr[1] != 0 {
		t.Fatalf("a rejected write changed memory: %v", *arr)
	}
}

func TestCopyBytesPinned(t *testing.T) {
	arr := new([16]byte)
	for i := range arr {
		arr[i] = byte(i * 3)
	}
	var pin runtime.Pinner
	pin.Pin(arr)
	defer pin.Unpin()
	base := uintptr(unsafe.Pointer(arr))
	got := CopyBytes(base+3, 10)
	if len(got) != 10 {
		t.Fatalf("len %d", len(got))
	}
	for i, b := range got {
		if b != byte((i+3)*3) {
			t.Errorf("got[%d] = %d", i, b)
		}
	}
	got[0] = 0xFF
	if arr[3] == 0xFF {
		t.Error("CopyBytes returned a view, not a copy")
	}
	if b := CopyBytes(0, 0); b == nil || len(b) != 0 {
		t.Errorf("CopyBytes(0, 0) = %v", b)
	}
	for _, c := range []struct {
		p    uintptr
		n    int
		want string
	}{{0, 1, "NULL address"}, {base, -1, "negative length"}} {
		func() {
			defer func() {
				if r := recover(); r == nil || !strings.Contains(r.(string), c.want) {
					t.Errorf("CopyBytes(%#x, %d) panic %v, want %q", c.p, c.n, r, c.want)
				}
			}()
			CopyBytes(c.p, c.n)
		}()
	}
}
