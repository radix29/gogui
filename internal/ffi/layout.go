package ffi

import (
	"reflect"
	"runtime"
	"strconv"
	"strings"
)

// Struct-layout test helpers (PLAN.md §7.2, §8.2).
//
// Without cgo a test can't ask the C compiler for sizeof or offsetof. Every
// Go struct that mirrors a C or ObjC struct (NSRect, RECT, GdkRGBA, …)
// therefore gets a layout test that compares it with constants taken from
// the headers, chosen per GOARCH by the caller:
//
//	func TestRectLayout(t *testing.T) {
//		ffi.CheckLayout(t, Rect{}, ffi.Layout{Size: 32, Align: 8, Fields: []ffi.Field{
//			{Name: "X", Offset: 0}, {Name: "Y", Offset: 8},
//			{Name: "W", Offset: 16}, {Name: "H", Offset: 24},
//		}})
//		ffi.CheckNoGoPointers(t, Rect{})
//	}
//
// The helpers live in a non-test file so the driver packages' tests can use
// them. They take a TB, not a *testing.T, so the package doesn't link the
// testing package into apps.

// TB is the part of testing.TB the layout helpers use. *testing.T and
// *testing.B satisfy it.
type TB interface {
	Helper()
	Errorf(format string, args ...any)
}

// Layout is the expected memory layout of a struct, taken from the C
// headers for one GOARCH.
type Layout struct {
	// Size is sizeof(struct), including trailing padding.
	Size uintptr
	// Align is _Alignof(struct).
	Align uintptr
	// Fields lists every named field of the Go struct in declaration order.
	// Blank (_) fields are explicit padding and are not listed.
	Fields []Field
}

// Field is the expected position of one struct field.
type Field struct {
	// Name is the Go field name.
	Name string
	// Offset is offsetof(struct, field).
	Offset uintptr
	// Size is sizeof(field). Zero means the size isn't checked; the offsets
	// of the following fields and the struct size usually catch a wrong
	// field type anyway, except for a last field hidden by padding.
	Size uintptr
}

// CheckLayout reports through t.Errorf, with one line per difference, how
// the layout of v's type differs from want. v is a struct or a pointer to
// one. It returns whether the layout matched.
func CheckLayout(t TB, v any, want Layout) bool {
	t.Helper()
	typ, name := structType(v)
	if typ == nil {
		t.Errorf("ffi.CheckLayout: %s is not a struct", name)
		return false
	}
	diffs := diffLayout(typ, want)
	if len(diffs) == 0 {
		return true
	}
	t.Errorf("layout of %s on %s differs from the C header:\n\t%s",
		name, runtime.GOARCH, strings.Join(diffs, "\n\t"))
	return false
}

// CheckNoGoPointers reports through t.Errorf every field of v's type that
// holds a Go pointer: a pointer, unsafe.Pointer, string, slice, map, chan,
// func or interface, also inside nested structs and arrays. A struct passed
// to native code by value, or stored in native memory, must not contain
// any (PLAN.md §7.2); native addresses are held as uintptr. v is a struct
// or a pointer to one. It returns whether v's type is free of Go pointers.
func CheckNoGoPointers(t TB, v any) bool {
	t.Helper()
	typ, name := structType(v)
	if typ == nil {
		t.Errorf("ffi.CheckNoGoPointers: %s is not a struct", name)
		return false
	}
	var found []string
	goPointers(typ, "", &found)
	if len(found) == 0 {
		return true
	}
	t.Errorf("%s contains Go pointers, which must not reach native memory:\n\t%s",
		name, strings.Join(found, "\n\t"))
	return false
}

// structType returns the struct type of v (dereferencing one pointer) and a
// name for messages, or nil and the name of what v is instead.
func structType(v any) (reflect.Type, string) {
	typ := reflect.TypeOf(v)
	if typ == nil {
		return nil, "nil"
	}
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return nil, typ.String()
	}
	return typ, typ.String()
}

// diffLayout returns the differences between typ's layout and want, one
// per line, in a fixed order: size, alignment, then fields in the order of
// want, then fields of typ that want doesn't list.
func diffLayout(typ reflect.Type, want Layout) []string {
	var diffs []string
	u := func(x uintptr) string { return strconv.FormatUint(uint64(x), 10) }
	if typ.Size() != want.Size {
		diffs = append(diffs, "size: got "+u(typ.Size())+", want "+u(want.Size))
	}
	if a := uintptr(typ.Align()); a != want.Align {
		diffs = append(diffs, "align: got "+u(a)+", want "+u(want.Align))
	}

	// The named fields of the Go struct, in declaration order.
	type goField struct {
		f   reflect.StructField
		pos int
	}
	got := map[string]goField{}
	var order []string
	for i := range typ.NumField() {
		f := typ.Field(i)
		if f.Name == "_" {
			continue
		}
		got[f.Name] = goField{f, len(order)}
		order = append(order, f.Name)
	}

	listed := map[string]bool{}
	last := -1 // position in order of the previous expected field found
	for _, w := range want.Fields {
		if listed[w.Name] {
			diffs = append(diffs, "field "+w.Name+": listed twice in the expected layout")
			continue
		}
		listed[w.Name] = true
		g, ok := got[w.Name]
		if !ok {
			diffs = append(diffs, "field "+w.Name+": missing from the Go struct (want offset "+
				u(w.Offset)+")")
			continue
		}
		if g.f.Offset != w.Offset {
			diffs = append(diffs, "field "+w.Name+": offset got "+u(g.f.Offset)+
				", want "+u(w.Offset))
		}
		if w.Size != 0 && g.f.Type.Size() != w.Size {
			diffs = append(diffs, "field "+w.Name+": size got "+u(g.f.Type.Size())+
				" ("+g.f.Type.String()+"), want "+u(w.Size))
		}
		if g.pos < last {
			diffs = append(diffs, "field "+w.Name+": declared out of order in the Go struct")
		}
		last = max(last, g.pos)
	}
	for _, n := range order {
		if !listed[n] {
			g := got[n]
			diffs = append(diffs, "field "+n+": not in the expected layout (Go offset "+
				u(g.f.Offset)+", "+g.f.Type.String()+")")
		}
	}
	return diffs
}

// goPointers appends to found the path and type of every part of typ that
// holds a Go pointer. path is the field path of typ ("" for the root).
func goPointers(typ reflect.Type, path string, found *[]string) {
	switch typ.Kind() {
	case reflect.Pointer, reflect.UnsafePointer, reflect.String, reflect.Slice,
		reflect.Map, reflect.Chan, reflect.Func, reflect.Interface:
		*found = append(*found, path+" ("+typ.String()+")")
	case reflect.Array:
		if typ.Len() > 0 {
			goPointers(typ.Elem(), path+"[0]", found)
		}
	case reflect.Struct:
		for i := range typ.NumField() {
			f := typ.Field(i)
			p := f.Name
			if path != "" {
				p = path + "." + f.Name
			}
			goPointers(f.Type, p, found)
		}
	}
}
