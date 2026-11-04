//go:build windows

package win32

import (
	"slices"
	"testing"
	"unicode/utf16"
)

func TestIndexMap(t *testing.T) {
	// Each rune: 1 byte/1 unit (a), 2/1 (é), 3/1 (€), 4/2 (😀).
	const s = "aé€😀b"
	u, m := utf16Of(s)
	if want := utf16.Encode([]rune(s)); !slices.Equal(u, want) {
		t.Fatalf("units %x, want %x", u, want)
	}
	if m.n8 != 11 || m.n16 != 6 {
		t.Fatalf("lengths %d, %d; want 11, 6", m.n8, m.n16)
	}
	// byte offset → unit (offsets inside a rune go to its start).
	wantUnit := []int{0, 1, 1, 2, 2, 2, 3, 3, 3, 3, 5, 6}
	for b, want := range wantUnit {
		if got := m.unit(b); got != want {
			t.Errorf("unit(%d) = %d, want %d", b, got, want)
		}
	}
	// unit → byte offset (inside a surrogate pair goes to the rune).
	wantByte := []int{0, 1, 3, 6, 6, 10, 11}
	for u, want := range wantByte {
		if got := m.byteOffset(u); got != want {
			t.Errorf("byteOffset(%d) = %d, want %d", u, got, want)
		}
	}
	// Clamping.
	if m.unit(-5) != 0 || m.unit(99) != 6 || m.byteOffset(-1) != 0 || m.byteOffset(99) != 11 {
		t.Errorf("clamping: %d %d %d %d", m.unit(-5), m.unit(99), m.byteOffset(-1), m.byteOffset(99))
	}
}

func TestIndexMapASCII(t *testing.T) {
	u, m := utf16Of("hello")
	if len(u) != 5 || u[0] != 'h' || m.b2u != nil || m.u2b != nil {
		t.Errorf("ASCII: units %v, tables %v %v", u, m.b2u, m.u2b)
	}
	for i := range 6 {
		if m.unit(i) != i || m.byteOffset(i) != i {
			t.Errorf("ASCII identity broken at %d", i)
		}
	}
	if m.unit(9) != 5 || m.byteOffset(-2) != 0 {
		t.Error("ASCII clamping")
	}
	u, m = utf16Of("")
	if len(u) != 0 || m.unit(3) != 0 || m.byteOffset(3) != 0 {
		t.Errorf("empty: %v %+v", u, m)
	}
}

func TestIndexMapInvalid(t *testing.T) {
	// 0xff and the lone continuation byte are one U+FFFD each; "\xe2\x82" is
	// a truncated euro sign: two invalid bytes, two U+FFFD.
	const s = "a\xffb\x80\xe2\x82c"
	u, m := utf16Of(s)
	want := []uint16{'a', 0xfffd, 'b', 0xfffd, 0xfffd, 0xfffd, 'c'}
	if !slices.Equal(u, want) {
		t.Fatalf("units %x, want %x", u, want)
	}
	if m.n8 != len(s) || m.n16 != len(want) {
		t.Fatalf("lengths %d, %d", m.n8, m.n16)
	}
	for i := 0; i <= len(s); i++ {
		if m.unit(i) != i || m.byteOffset(i) != i { // one unit per byte
			t.Errorf("index %d: unit %d, byte %d", i, m.unit(i), m.byteOffset(i))
		}
	}
	// An invalid byte before a valid multi-byte rune.
	u, m = utf16Of("\xffé")
	if !slices.Equal(u, []uint16{0xfffd, 'é'}) || m.unit(2) != 1 || m.byteOffset(2) != 3 {
		t.Errorf("mixed: %x, unit(2)=%d, byteOffset(2)=%d", u, m.unit(2), m.byteOffset(2))
	}
	// A UTF-8 encoded surrogate (invalid in UTF-8) is three bad bytes.
	u, _ = utf16Of("\xed\xa0\x80")
	if !slices.Equal(u, []uint16{0xfffd, 0xfffd, 0xfffd}) {
		t.Errorf("encoded surrogate: %x", u)
	}
}

// TestIndexMapRoundTrip: for valid text every rune boundary maps there and
// back to itself.
func TestIndexMapRoundTrip(t *testing.T) {
	const s = "x€é😀\n😀ß"
	_, m := utf16Of(s)
	b := 0
	for _, r := range s {
		if got := m.byteOffset(m.unit(b)); got != b {
			t.Errorf("round trip at byte %d = %d", b, got)
		}
		b += len(string(r))
	}
}
