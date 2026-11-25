//go:build linux

package gtk

import (
	"math"
	"reflect"
	"testing"
	"unsafe"

	"github.com/radix29/gogui/internal/ffi"
)

// These tests need the GTK 4 libraries but no display (Load does not call
// gtk_init_check). native.sh runs them on Linux with "go test ./...".

// TestLoad checks that every library opens, every symbol resolves and the
// versions pass the check, and that Load sets every func field.
func TestLoad(t *testing.T) {
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := Load(); err != nil {
		t.Fatalf("second Load: %v", err)
	}
	v := Version()
	t.Logf("GTK %d.%d.%d, Pango %d", v[0], v[1], v[2], PangoVersion())
	if v[0] != 4 || v[1] < MinGTKMinor {
		t.Errorf("Version() = %v after a successful Load", v)
	}
	if p := PangoVersion(); p < MinPangoMajor*10000+MinPangoMinor*100 {
		t.Errorf("PangoVersion() = %d after a successful Load", p)
	}
	optional := map[uintptr]string{}
	for _, l := range libraries() {
		for _, b := range l.optional {
			optional[reflect.ValueOf(b.fptr).Pointer()] = b.name
		}
	}
	for _, s := range boundStructs {
		rv := reflect.ValueOf(s.v).Elem()
		for i := range rv.NumField() {
			if name, ok := optional[rv.Field(i).Addr().Pointer()]; ok {
				t.Logf("optional %s bound: %v", name, !rv.Field(i).IsNil())
				continue
			}
			if rv.Field(i).IsNil() {
				t.Errorf("%s.%s is nil after Load", s.name, rv.Type().Field(i).Name)
			}
		}
	}
}

// testGsk holds the GSK node getters the node-tree tests use (S4.1g).
// They are test-only, so they are not in the shipped tables.
var testGsk struct {
	RenderNodeGetNodeType     func(n uintptr) int32
	RenderNodeGetBounds       func(n uintptr, bounds *GrapheneRect)
	ColorNodeGetColor         func(n uintptr) uintptr // const GdkRGBA *
	ContainerNodeGetNChildren func(n uintptr) uint32
	ContainerNodeGetChild     func(n uintptr, i uint32) uintptr
	RoundedClipNodeGetChild   func(n uintptr) uintptr
	RoundedClipNodeGetClip    func(n uintptr) uintptr // const GskRoundedRect *
	DebugNodeGetChild         func(n uintptr) uintptr
	ClipNodeGetChild          func(n uintptr) uintptr
}

func TestNodeGettersResolve(t *testing.T) {
	lib, err := ffi.Dlopen(GTKLib)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range []binding{
		{&testGsk.RenderNodeGetNodeType, "gsk_render_node_get_node_type"},
		{&testGsk.RenderNodeGetBounds, "gsk_render_node_get_bounds"},
		{&testGsk.ColorNodeGetColor, "gsk_color_node_get_color"},
		{&testGsk.ContainerNodeGetNChildren, "gsk_container_node_get_n_children"},
		{&testGsk.ContainerNodeGetChild, "gsk_container_node_get_child"},
		{&testGsk.RoundedClipNodeGetChild, "gsk_rounded_clip_node_get_child"},
		{&testGsk.RoundedClipNodeGetClip, "gsk_rounded_clip_node_get_clip"},
		{&testGsk.DebugNodeGetChild, "gsk_debug_node_get_child"},
		{&testGsk.ClipNodeGetChild, "gsk_clip_node_get_child"},
	} {
		if err := lib.Bind(b.fptr, b.name); err != nil {
			t.Error(err)
		}
	}
}

// TestReaders writes C floats into g_malloc0 memory and reads them back
// through ReadRGBA, ReadRect and ReadRoundedRect.
func TestReaders(t *testing.T) {
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	size := unsafe.Sizeof(RoundedRect{})
	p := GLib.Malloc0(size)
	defer GLib.Free(p)
	for i := uintptr(0); i < size/4; i++ {
		ffi.WriteInt32(p, i*4, int32(math.Float32bits(float32(i)+0.5)))
	}
	if got, want := ReadRGBA(p), (RGBA{R: 0.5, G: 1.5, B: 2.5, A: 3.5}); got != want {
		t.Errorf("ReadRGBA = %+v, want %+v", got, want)
	}
	if got, want := ReadRect(p, 4), Rect(1.5, 2.5, 3.5, 4.5); got != want {
		t.Errorf("ReadRect(p, 4) = %+v, want %+v", got, want)
	}
	r := ReadRoundedRect(p)
	want := RoundedRect{Bounds: Rect(0.5, 1.5, 2.5, 3.5)}
	for i := range want.Corner {
		f := float32(4+2*i) + 0.5
		want.Corner[i] = GrapheneSize{Width: f, Height: f + 1}
	}
	if r != want {
		t.Errorf("ReadRoundedRect = %+v, want %+v", r, want)
	}
}

// TestAttrType builds a PangoAttrClass and a PangoAttribute pointing at it
// in g_malloc0 memory and reads the type back through AttrType (S5.3h).
func TestAttrType(t *testing.T) {
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	klass := GLib.Malloc0(unsafe.Sizeof(PangoAttrClass{}))
	defer GLib.Free(klass)
	attr := GLib.Malloc0(unsafe.Sizeof(PangoAttribute{}))
	defer GLib.Free(attr)
	ffi.WriteInt32(klass, unsafe.Offsetof(PangoAttrClass{}.Type), AttrUnderline)
	ffi.WriteUintptr(attr, unsafe.Offsetof(PangoAttribute{}.Klass), klass)
	if got := AttrType(attr); got != AttrUnderline {
		t.Errorf("AttrType = %d, want %d", got, AttrUnderline)
	}
}
