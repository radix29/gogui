package com

import "github.com/radix29/gogui/internal/ffi"

// Vtable lists the methods of a COM interface in vtable order, inherited
// interfaces first, as the SDK header declares them (S2.4b). The binding
// packages keep one per interface in their tests and check their slot
// constants against it with CheckSlots, because a wrong slot index is the
// typical COM binding bug: it calls another method with the same this.
//
// MSVC places overloads of one name next to each other in reverse
// declaration order (IDCompositionVisual's SetOffsetX(animation) comes
// before SetOffsetX(float)); a Vtable lists them as MSVC lays them out,
// with a suffix on the name.
type Vtable []string

// IUnknownVtable is the root of every interface.
var IUnknownVtable = Vtable{"QueryInterface", "AddRef", "Release"}

// Extend returns a new Vtable: v followed by names.
func (v Vtable) Extend(names ...string) Vtable {
	out := make(Vtable, 0, len(v)+len(names))
	return append(append(out, v...), names...)
}

// Slot returns the index of name in v, or -1.
func (v Vtable) Slot(name string) int {
	for i, n := range v {
		if n == name {
			return i
		}
	}
	return -1
}

// CheckSlots reports through t every slot constant that differs from the
// index of its method in v, every method of slots that v lacks, and every
// name v lists twice. iface names the interface in the messages. It
// returns whether everything matched.
func CheckSlots(t ffi.TB, iface string, v Vtable, slots map[string]int) bool {
	t.Helper()
	ok := true
	seen := map[string]bool{}
	for _, n := range v {
		if seen[n] {
			t.Errorf("%s: method %s listed twice", iface, n)
			ok = false
		}
		seen[n] = true
	}
	for name, slot := range slots {
		want := v.Slot(name)
		switch {
		case want < 0:
			t.Errorf("%s: no method %s in the vtable", iface, name)
			ok = false
		case want != slot:
			t.Errorf("%s::%s: slot %d, want %d", iface, name, slot, want)
			ok = false
		}
	}
	return ok
}
