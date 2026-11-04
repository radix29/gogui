package com

import (
	"errors"
	"fmt"
	"strconv"
)

// GUID mirrors the SDK's GUID (and IID, CLSID): 16 bytes, 4-byte aligned.
// It has the layout of windows.GUID.
type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// IID_IUnknown is the interface ID of IUnknown.
var IID_IUnknown = MustGUID("{00000000-0000-0000-C000-000000000046}")

var errGUIDSyntax = errors.New("want {XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX} in hex, braces optional")

// ParseGUID parses the SDK string form of a GUID,
// "{6B29FC40-CA47-1067-B31D-00DD010662DA}", in either case, with or
// without the braces.
func ParseGUID(s string) (GUID, error) {
	in := s
	if len(s) == 38 && s[0] == '{' && s[37] == '}' {
		s = s[1:37]
	}
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return GUID{}, fmt.Errorf("com: GUID %q: %w", in, errGUIDSyntax)
	}
	hex := func(part string) (uint64, bool) {
		for i := 0; i < len(part); i++ {
			c := part[i]
			if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F') {
				return 0, false
			}
		}
		v, err := strconv.ParseUint(part, 16, 64)
		return v, err == nil
	}
	d1, ok1 := hex(s[0:8])
	d2, ok2 := hex(s[9:13])
	d3, ok3 := hex(s[14:18])
	d4, ok4 := hex(s[19:23])
	d5, ok5 := hex(s[24:36])
	if !(ok1 && ok2 && ok3 && ok4 && ok5) {
		return GUID{}, fmt.Errorf("com: GUID %q: %w", in, errGUIDSyntax)
	}
	g := GUID{Data1: uint32(d1), Data2: uint16(d2), Data3: uint16(d3)}
	g.Data4[0] = byte(d4 >> 8)
	g.Data4[1] = byte(d4)
	for i := range 6 {
		g.Data4[2+i] = byte(d5 >> (8 * (5 - i)))
	}
	return g, nil
}

// MustGUID is ParseGUID for the package-level IIDs and CLSIDs; it panics on
// a malformed string.
func MustGUID(s string) GUID {
	g, err := ParseGUID(s)
	if err != nil {
		panic(err)
	}
	return g
}

// String returns the SDK form with braces and upper-case hex digits.
func (g GUID) String() string {
	return fmt.Sprintf("{%08X-%04X-%04X-%02X%02X-%02X%02X%02X%02X%02X%02X}",
		g.Data1, g.Data2, g.Data3, g.Data4[0], g.Data4[1],
		g.Data4[2], g.Data4[3], g.Data4[4], g.Data4[5], g.Data4[6], g.Data4[7])
}
