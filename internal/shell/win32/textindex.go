//go:build windows

package win32

import "unicode/utf8"

// indexMap maps between the UTF-8 byte offsets of a Go string and the
// UTF-16 unit indices of its DirectWrite copy. An invalid byte becomes one
// U+FFFD (one unit per byte). Offsets inside a rune, and indices inside a
// surrogate pair, move back to the start of the rune; values outside the
// text are clamped. Pure ASCII text has the identity map and no tables.
type indexMap struct {
	n8, n16 int
	b2u     []int32 // len n8+1: byte offset → unit index of its rune; nil when identity
	u2b     []int32 // len n16+1: unit index → byte offset of its rune; nil when identity
}

// utf16Of converts s to UTF-16 (invalid bytes become U+FFFD) and returns the
// index map between the two.
func utf16Of(s string) ([]uint16, indexMap) {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	if ascii {
		u := make([]uint16, len(s))
		for i := 0; i < len(s); i++ {
			u[i] = uint16(s[i])
		}
		return u, indexMap{n8: len(s), n16: len(s)}
	}
	u := make([]uint16, 0, len(s))
	b2u := make([]int32, len(s)+1)
	u2b := make([]int32, 0, len(s)+1)
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		for j := range size {
			b2u[i+j] = int32(len(u))
		}
		u2b = append(u2b, int32(i))
		if r >= 0x10000 {
			r -= 0x10000
			u = append(u, uint16(0xd800+(r>>10)), uint16(0xdc00+(r&0x3ff)))
			u2b = append(u2b, int32(i)) // the low surrogate maps back to the rune
		} else {
			u = append(u, uint16(r)) // U+FFFD for an invalid byte
		}
		i += size
	}
	b2u[len(s)] = int32(len(u))
	u2b = append(u2b, int32(len(s)))
	return u, indexMap{n8: len(s), n16: len(u), b2u: b2u, u2b: u2b}
}

// unit returns the UTF-16 index of the rune containing byte offset b.
func (m indexMap) unit(b int) int {
	b = min(max(b, 0), m.n8)
	if m.b2u == nil {
		return b
	}
	return int(m.b2u[b])
}

// byteOffset returns the UTF-8 offset of the rune containing UTF-16 index u.
func (m indexMap) byteOffset(u int) int {
	u = min(max(u, 0), m.n16)
	if m.u2b == nil {
		return u
	}
	return int(m.u2b[u])
}
