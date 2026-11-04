//go:build linux

package gtk4

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"testing"
	"unsafe"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// The class tests, ported from spike/gtk's phase-1 self-test (S0.9b). They
// need GTK 4 and a display for gtk_init_check, but no window and no main
// loop. native.sh runs them under xvfb; a plain "go test" without a
// display skips them.
//
// GTK must be called on one thread, and every test runs on its own
// goroutine, so the calls go to one locked goroutine (onGTK). Helpers
// running there return errors instead of calling t.Fatal.

var gtkCalls = make(chan func())

// gtkInit is the result of gtk.Load and gtk_init_check, made on the GTK
// goroutine; errNoDisplay means no display was set.
var gtkInit error

var errNoDisplay = errors.New("neither DISPLAY nor WAYLAND_DISPLAY is set")

func TestMain(m *testing.M) {
	ready := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		gtkInit = initGTK()
		close(ready)
		for f := range gtkCalls {
			f()
		}
	}()
	<-ready
	os.Exit(m.Run())
}

func initGTK() error {
	if err := gtk.Load(); err != nil {
		return err
	}
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return errNoDisplay
	}
	if gtk.Gtk.InitCheck() == gtk.False {
		return fmt.Errorf("gtk_init_check failed (DISPLAY=%q, WAYLAND_DISPLAY=%q)",
			os.Getenv("DISPLAY"), os.Getenv("WAYLAND_DISPLAY"))
	}
	return nil
}

// onGTK runs f on the GTK goroutine and reports its error, re-raising a
// pending panic there first so no test leaves one behind.
func onGTK(t *testing.T, f func() error) {
	t.Helper()
	var err error
	var pending *PanicError
	done := make(chan struct{})
	gtkCalls <- func() {
		defer close(done)
		err = f()
		pending, pendingPanic = pendingPanic, nil
	}
	<-done
	if pending != nil {
		t.Errorf("a callback panicked: %v", pending)
	}
	if err != nil {
		t.Error(err)
	}
}

// needGTK skips the test without a display and fails it if GTK is not
// usable; the trampolines and the class are registered once.
func needGTK(t *testing.T) {
	t.Helper()
	if errors.Is(gtkInit, errNoDisplay) {
		t.Skip("no display; native.sh runs the gtk4 tests under xvfb")
	}
	if gtkInit != nil {
		t.Fatal(gtkInit)
	}
	var err error
	onGTK(t, func() error { err = registerSurfaceClass(); return nil })
	if err != nil {
		t.Fatal(err)
	}
}

func typeOf(instance uintptr) uintptr { return ffi.ReadUintptr(ffi.ReadUintptr(instance, 0), 0) }

func typeName(t uintptr) string { return ffi.GoString(gtk.GObject.TypeName(t)) }

// TestCallbacksPinned pins ffi.Callbacks() to the trampoline set: the
// process creates exactly numTrampolines native callbacks, however often
// the set and the class are registered and however many signals are
// connected (S4.1c "Done when").
func TestCallbacksPinned(t *testing.T) {
	if gtkInit != nil && !errors.Is(gtkInit, errNoDisplay) {
		t.Fatal(gtkInit)
	}
	onGTK(t, func() error {
		registerTrampolines()
		registerTrampolines()
		return nil
	})
	if got := ffi.Callbacks(); got != numTrampolines {
		t.Errorf("ffi.Callbacks() = %d, want %d", got, numTrampolines)
	}
	seen := map[uintptr]string{}
	for _, tr := range trampolines() {
		if *tr.addr == 0 {
			t.Errorf("%s has no C function pointer", tr.name)
		} else if other, dup := seen[*tr.addr]; dup {
			t.Errorf("%s and %s share %#x", tr.name, other, *tr.addr)
		}
		seen[*tr.addr] = tr.name
	}
}

func TestRegisterSurfaceClass(t *testing.T) {
	needGTK(t)
	onGTK(t, func() error {
		st := class.surfaceType
		var q gtk.TypeQuery
		gtk.GObject.TypeQuery(st, &q)
		switch {
		case typeName(st) != surfaceTypeName:
			return fmt.Errorf("g_type_name = %q", typeName(st))
		case gtk.GObject.TypeParent(st) != class.widgetType:
			return fmt.Errorf("parent type is %s, want GtkWidget", typeName(gtk.GObject.TypeParent(st)))
		case q.ClassSize != widgetClassSize:
			return fmt.Errorf("class_size = %d, want %d", q.ClassSize, widgetClassSize)
		case uintptr(q.InstanceSize) != class.handleOff+wordSize:
			return fmt.Errorf("instance_size = %d, want %d", q.InstanceSize, class.handleOff+wordSize)
		case class.handleOff%wordSize != 0:
			return fmt.Errorf("handle word at %d is not word-aligned", class.handleOff)
		case gtk.GObject.TypeFromName(surfaceTypeName) != st:
			return fmt.Errorf("g_type_from_name(%s) = %#x, want %#x", surfaceTypeName, gtk.GObject.TypeFromName(surfaceTypeName), st)
		}
		t.Logf("GTK %v: GType %#x, sizeof(GtkWidget) %d, handle word at %d", gtk.Version(), st, q.InstanceSize-wordSize, class.handleOff)
		// Registering again returns the first result, and a fresh
		// registration of the taken name fails cleanly.
		if err := registerSurfaceClass(); err != nil {
			return fmt.Errorf("second registerSurfaceClass: %v", err)
		}
		saved := class
		err := registerClass()
		class = saved
		if err == nil {
			return fmt.Errorf("registering %s a second time succeeded", surfaceTypeName)
		}
		if gtk.GObject.TypeFromName(surfaceTypeName) != st {
			return fmt.Errorf("the failed registration changed the type")
		}
		return nil
	})
}

// TestSurfaceClassSlots reads the class struct back: the three slots hold
// the trampolines, GtkWidget's own class does not, the CSS name is set and
// the chain-up target is GtkWidget's snapshot.
func TestSurfaceClassSlots(t *testing.T) {
	needGTK(t)
	onGTK(t, func() error {
		klass := gtk.GObject.TypeClassRef(class.surfaceType)
		parent := gtk.GObject.TypeClassRef(class.widgetType)
		if ffi.ReadUintptr(klass, 0) != class.surfaceType {
			return fmt.Errorf("g_type of the class = %#x, want %#x", ffi.ReadUintptr(klass, 0), class.surfaceType)
		}
		for _, s := range []struct {
			name      string
			off, want uintptr
		}{
			{"size_allocate", slotSizeAllocate, tramp.sizeAllocate},
			{"measure", slotMeasure, tramp.measure},
			{"snapshot", slotSnapshot, tramp.snapshot},
		} {
			if got := ffi.ReadUintptr(klass, s.off); got != s.want {
				return fmt.Errorf("GGSurface %s slot = %#x, want the trampoline %#x", s.name, got, s.want)
			}
			if ffi.ReadUintptr(parent, s.off) == s.want {
				return fmt.Errorf("GtkWidget's own %s slot holds the trampoline", s.name)
			}
		}
		if ffi.ReadUintptr(parent, slotSnapshot) != 0 && class.parentSnapshot == nil {
			return fmt.Errorf("GtkWidget has a snapshot vfunc but the chain-up is not bound")
		}
		if got := ffi.GoString(testGtk.WidgetClassGetCSSName(klass)); got != surfaceCSSName {
			return fmt.Errorf("CSS name %q, want %q", got, surfaceCSSName)
		}
		return nil
	})
}

// testGtk holds functions only the tests call.
var testGtk struct {
	WidgetClassGetCSSName func(klass uintptr) uintptr // const char *
	ValueGetBoolean       func(v uintptr) int32
	WindowDestroy         func(w uintptr)
}

func init() {
	for _, b := range []struct {
		lib, name string
		fptr      any
	}{
		{gtk.GTKLib, "gtk_widget_class_get_css_name", &testGtk.WidgetClassGetCSSName},
		{gtk.GObjectLib, "g_value_get_boolean", &testGtk.ValueGetBoolean},
		{gtk.GTKLib, "gtk_window_destroy", &testGtk.WindowDestroy},
	} {
		lib, err := ffi.Dlopen(b.lib)
		if err != nil {
			return // TestLoad-style failures show up through gtkInit
		}
		_ = lib.Bind(b.fptr, b.name)
	}
}

// fakeSurface is a surfacePeer for the native tests.
type fakeSurface struct {
	min, nat      [2]int32
	measures      []measureCall
	allocs        int
	width, height int32
	baseline      int32
	snapshots     int
	color         gtk.RGBA
}

type measureCall struct{ orientation, forSize int32 }

func (p *fakeSurface) measure(o, forSize int32) (int32, int32) {
	p.measures = append(p.measures, measureCall{o, forSize})
	extra := max(forSize, 0) / 10 // honours for_size
	return p.min[o] + extra, p.nat[o] + extra
}

func (p *fakeSurface) sizeAllocate(w, h, b int32) {
	p.allocs++
	p.width, p.height, p.baseline = w, h, b
}

func (p *fakeSurface) snapshot(s uintptr) {
	p.snapshots++
	c, r := p.color, gtk.Rect(0, 0, float32(p.width), float32(p.height))
	gtk.Gtk.SnapshotAppendColor(s, &c, &r)
}

func TestSurfaceHandleWordNative(t *testing.T) {
	needGTK(t)
	onGTK(t, func() error {
		base := dispatch.Len()
		w := newSurfaceWidget(nil)
		defer releaseSurfaceWidget(w)
		if typeOf(w) != class.surfaceType {
			return fmt.Errorf("instance type is %s", typeName(typeOf(w)))
		}
		if h := surfaceHandle(w); h != 0 {
			return fmt.Errorf("a new instance has handle %v, want 0", h)
		}
		words := class.handleOff / wordSize
		before := make([]uintptr, words)
		for i := range before {
			before[i] = ffi.ReadUintptr(w, uintptr(i)*wordSize)
		}
		p := &fakeSurface{}
		w2 := newSurfaceWidget(p)
		if got, ok := surfacePeerOf(w2); !ok || got != p {
			return fmt.Errorf("surfacePeerOf = %v, %v; want the peer", got, ok)
		}
		if dispatch.Len() != base+1 {
			return fmt.Errorf("dispatch.Len() = %d, want %d", dispatch.Len(), base+1)
		}
		releaseSurfaceWidget(w2)
		if dispatch.Len() != base {
			return fmt.Errorf("releaseSurfaceWidget left the handle: Len %d, want %d", dispatch.Len(), base)
		}
		// Writing the handle word leaves the GtkWidget instance alone.
		h := dispatch.New(entry{peer: p, kind: kindSurface})
		defer dispatch.Free(h)
		setSurfaceHandle(w, h)
		for i := range before {
			if got := ffi.ReadUintptr(w, uintptr(i)*wordSize); got != before[i] {
				return fmt.Errorf("word %d of the GtkWidget instance changed from %#x to %#x", i, before[i], got)
			}
		}
		setSurfaceHandle(w, 0)
		return nil
	})
}

// TestSurfaceMeasureNative measures through gtk_widget_measure into
// native ints laid out as [sentinel, min, sentinel, nat, sentinel], with
// NULL baselines, and calls the slot with a for_size.
func TestSurfaceMeasureNative(t *testing.T) {
	needGTK(t)
	onGTK(t, func() error {
		p := &fakeSurface{min: [2]int32{40, 30}, nat: [2]int32{120, 80}}
		w := newSurfaceWidget(p)
		defer releaseSurfaceWidget(w)
		buf := gtk.GLib.Malloc0(5 * 4)
		defer gtk.GLib.Free(buf)
		at := func(i uintptr) uintptr { return buf + 4*i }
		for _, c := range []struct{ o, forSize, mn, nt int32 }{
			{gtk.OrientationHorizontal, -1, 40, 120},
			{gtk.OrientationVertical, -1, 30, 80},
		} {
			for i := range uintptr(5) {
				ffi.WriteInt32(buf, 4*i, sentinel)
			}
			n := len(p.measures)
			gtk.Gtk.WidgetMeasure(w, c.o, c.forSize, at(1), at(3), 0, 0)
			if len(p.measures) != n+1 {
				return fmt.Errorf("the Go measure ran %d times, want once", len(p.measures)-n)
			}
			if got := p.measures[n]; got != (measureCall{c.o, c.forSize}) {
				return fmt.Errorf("measure got %+v, want orientation %d, for_size %d", got, c.o, c.forSize)
			}
			if mn, nt := ffi.ReadInt32(buf, 4), ffi.ReadInt32(buf, 12); mn != c.mn || nt != c.nt {
				return fmt.Errorf("gtk_widget_measure(%d, %d) gave %d, %d; want %d, %d", c.o, c.forSize, mn, nt, c.mn, c.nt)
			}
			for _, i := range []uintptr{0, 2, 4} {
				if got := ffi.ReadInt32(buf, 4*i); got != sentinel {
					return fmt.Errorf("the int next to the results changed to %#x", got)
				}
			}
		}
		// GGSurface keeps GtkWidget's request mode, CONSTANT_SIZE for a
		// widget without children, so gtk_widget_measure passes for_size
		// -1. A for_size reaches the peer through the slot itself, as a
		// height-for-width request mode would pass it.
		var measure func(w uintptr, o, forSize int32, mn, nt, mnb, ntb uintptr)
		if err := ffi.BindAddr(&measure, ffi.ReadUintptr(ffi.ReadUintptr(w, 0), slotMeasure)); err != nil {
			return err
		}
		ffi.WriteInt32(buf, 4, sentinel)
		ffi.WriteInt32(buf, 12, sentinel)
		measure(w, gtk.OrientationVertical, 250, at(1), at(3), 0, 0)
		if got := p.measures[len(p.measures)-1]; got != (measureCall{gtk.OrientationVertical, 250}) {
			return fmt.Errorf("the slot passed %+v, want for_size 250", got)
		}
		if mn, nt := ffi.ReadInt32(buf, 4), ffi.ReadInt32(buf, 12); mn != 55 || nt != 105 {
			return fmt.Errorf("measure at for_size 250 gave %d, %d; want 55, 105", mn, nt)
		}
		// Without a peer the sizes are 0.
		bare := newSurfaceWidget(nil)
		defer releaseSurfaceWidget(bare)
		ffi.WriteInt32(buf, 4, sentinel)
		ffi.WriteInt32(buf, 12, sentinel)
		gtk.Gtk.WidgetMeasure(bare, gtk.OrientationHorizontal, -1, at(1), at(3), 0, 0)
		if mn, nt := ffi.ReadInt32(buf, 4), ffi.ReadInt32(buf, 12); mn != 0 || nt != 0 {
			return fmt.Errorf("without a peer: %d, %d; want 0, 0", mn, nt)
		}
		return nil
	})
}

func TestSurfaceSizeAllocateNative(t *testing.T) {
	needGTK(t)
	onGTK(t, func() error {
		p := &fakeSurface{min: [2]int32{10, 10}, nat: [2]int32{20, 20}}
		w := newSurfaceWidget(p)
		defer releaseSurfaceWidget(w)
		gtk.Gtk.WidgetMeasure(w, gtk.OrientationHorizontal, -1, 0, 0, 0, 0)
		gtk.Gtk.WidgetMeasure(w, gtk.OrientationVertical, -1, 0, 0, 0, 0)
		alloc := gtk.Allocation{Width: 173, Height: 61}
		gtk.Gtk.WidgetSizeAllocate(w, &alloc, -1)
		if p.allocs != 1 || p.width != 173 || p.height != 61 || p.baseline != -1 {
			return fmt.Errorf("size_allocate ran %d times with %d × %d, baseline %d; want once with 173 × 61, -1",
				p.allocs, p.width, p.height, p.baseline)
		}
		return nil
	})
}

// TestSurfaceSnapshotNative calls the snapshot slot directly on a fresh
// GtkSnapshot, as the spike did, and finds the peer's colour node; the
// chain-up to GtkWidget's snapshot (no children) adds nothing and does
// not crash.
func TestSurfaceSnapshotNative(t *testing.T) {
	needGTK(t)
	bindNodeGetters(t)
	onGTK(t, func() error {
		p := &fakeSurface{color: gtk.RGBA{R: 0.25, G: 0.5, B: 0.75, A: 1}, width: 200, height: 100}
		w := newSurfaceWidget(p)
		defer releaseSurfaceWidget(w)
		var snapshot func(w, s uintptr)
		if err := ffi.BindAddr(&snapshot, ffi.ReadUintptr(ffi.ReadUintptr(w, 0), slotSnapshot)); err != nil {
			return err
		}
		s := gtk.Gtk.SnapshotNew()
		snapshot(w, s)
		node := gtk.Gtk.SnapshotFreeToNode(s)
		if node == 0 {
			return fmt.Errorf("gtk_snapshot_free_to_node returned NULL (snapshot ran %d times)", p.snapshots)
		}
		defer gtk.Gsk.RenderNodeUnref(node)
		if p.snapshots != 1 {
			return fmt.Errorf("the Go snapshot ran %d times, want once", p.snapshots)
		}
		var colors []gtk.RGBA
		var bounds []gtk.GrapheneRect
		walk(node, func(n uintptr) {
			var b gtk.GrapheneRect
			nodes.RenderNodeGetBounds(n, &b)
			colors = append(colors, gtk.ReadRGBA(nodes.ColorNodeGetColor(n)))
			bounds = append(bounds, b)
		})
		if len(colors) != 1 || colors[0] != p.color || bounds[0] != gtk.Rect(0, 0, 200, 100) {
			return fmt.Errorf("colour nodes %v at %v, want %v at 0,0 200×100", colors, bounds, p.color)
		}
		return nil
	})
}

// nodes holds the GSK node getters the snapshot test walks with.
var nodes struct {
	RenderNodeGetNodeType     func(n uintptr) int32
	RenderNodeGetBounds       func(n uintptr, bounds *gtk.GrapheneRect)
	ColorNodeGetColor         func(n uintptr) uintptr
	ContainerNodeGetNChildren func(n uintptr) uint32
	ContainerNodeGetChild     func(n uintptr, i uint32) uintptr
	DebugNodeGetChild         func(n uintptr) uintptr
}

func bindNodeGetters(t *testing.T) {
	t.Helper()
	lib, err := ffi.Dlopen(gtk.GTKLib)
	if err != nil {
		t.Fatal(err)
	}
	for name, fptr := range map[string]any{
		"gsk_render_node_get_node_type":     &nodes.RenderNodeGetNodeType,
		"gsk_render_node_get_bounds":        &nodes.RenderNodeGetBounds,
		"gsk_color_node_get_color":          &nodes.ColorNodeGetColor,
		"gsk_container_node_get_n_children": &nodes.ContainerNodeGetNChildren,
		"gsk_container_node_get_child":      &nodes.ContainerNodeGetChild,
		"gsk_debug_node_get_child":          &nodes.DebugNodeGetChild,
	} {
		if err := lib.Bind(fptr, name); err != nil {
			t.Fatal(err)
		}
	}
}

// walk calls color for every colour node under n, through containers and
// debug nodes.
func walk(n uintptr, color func(uintptr)) {
	if n == 0 {
		return
	}
	switch nodes.RenderNodeGetNodeType(n) {
	case gtk.NodeColor:
		color(n)
	case gtk.NodeContainer:
		for i := range nodes.ContainerNodeGetNChildren(n) {
			walk(nodes.ContainerNodeGetChild(n, i), color)
		}
	case gtk.NodeDebug:
		walk(nodes.DebugNodeGetChild(n), color)
	}
}

// pressPeer records GtkGestureClick::pressed through sigIntDoubleDouble.
type pressPeer struct {
	calls  []recCall
	panics any
}

func (p *pressPeer) intDoubleDouble(k kind, instance uintptr, n int32, x, y float64) {
	p.calls = append(p.calls, recCall{sig: "idd", k: k, instance: instance, n: n, x: x, y: y})
	if p.panics != nil {
		panic(p.panics)
	}
}

// emitv emits signal sig on instance through g_signal_emitv with the
// extra arguments set by set (one per GValue after the instance), and
// returns the gboolean result if ret is a boolean signal. The GValues are
// in g_malloc0 memory, because the handler calls back into Go while GLib
// holds them.
func emitv(instance uintptr, sig uint32, boolResult bool, set ...func(v uintptr)) int32 {
	n := uintptr(1 + len(set))
	stride := unsafe.Sizeof(gtk.GValue{})
	vals := gtk.GLib.Malloc0((n + 1) * stride) // the last one is the result
	defer gtk.GLib.Free(vals)
	v := func(i uintptr) uintptr { return vals + i*stride }
	gtk.GObject.ValueInit(v(0), typeOf(instance))
	gtk.GObject.ValueSetObject(v(0), instance)
	for i, f := range set {
		f(v(uintptr(i) + 1))
	}
	var ret uintptr
	if boolResult {
		ret = v(n)
		gtk.GObject.ValueInit(ret, gtk.TypeBoolean)
	}
	gtk.GObject.SignalEmitv(vals, sig, 0, ret)
	var r int32
	if boolResult {
		r = testGtk.ValueGetBoolean(ret)
	}
	for i := range n + 1 {
		if ffi.ReadUintptr(v(i), 0) != 0 { // initialised
			gtk.GObject.ValueUnset(v(i))
		}
	}
	return r
}
