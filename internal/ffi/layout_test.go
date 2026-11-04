package ffi

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

// fakeTB records what the layout helpers report.
type fakeTB struct {
	helpers int
	errs    []string
}

func (f *fakeTB) Helper() { f.helpers++ }
func (f *fakeTB) Errorf(format string, args ...any) {
	f.errs = append(f.errs, fmt.Sprintf(format, args...))
}

// Sample mirrors. Their expected layouts are written per pointer size, as a
// driver's tests would write them per GOARCH from the C headers.

// cgRect mirrors CGRect (four CGFloat, which is double on 64-bit).
type cgRect struct {
	X, Y, W, H float64
}

// padded mirrors struct { uint8_t a; uint32_t b; uint16_t c; void *d; }.
type padded struct {
	A uint8
	_ [3]byte
	B uint32
	C uint16
	D uintptr
}

// gdkRGBA mirrors GdkRGBA (four float).
type gdkRGBA struct {
	Red, Green, Blue, Alpha float32
}

const ptr = unsafe.Sizeof(uintptr(0))

func paddedLayout() Layout {
	if ptr == 8 {
		return Layout{Size: 24, Align: 8, Fields: []Field{
			{Name: "A", Offset: 0, Size: 1}, {Name: "B", Offset: 4, Size: 4},
			{Name: "C", Offset: 8, Size: 2}, {Name: "D", Offset: 16, Size: 8},
		}}
	}
	return Layout{Size: 16, Align: 4, Fields: []Field{
		{Name: "A", Offset: 0}, {Name: "B", Offset: 4}, {Name: "C", Offset: 8}, {Name: "D", Offset: 12},
	}}
}

func TestCheckLayoutMatches(t *testing.T) {
	cases := []struct {
		v    any
		want Layout
	}{
		{cgRect{}, Layout{Size: 32, Align: 8, Fields: []Field{
			{Name: "X", Offset: 0, Size: 8}, {Name: "Y", Offset: 8}, {Name: "W", Offset: 16}, {Name: "H", Offset: 24},
		}}},
		{&cgRect{}, Layout{Size: 32, Align: 8, Fields: []Field{
			{Name: "X", Offset: 0}, {Name: "Y", Offset: 8}, {Name: "W", Offset: 16}, {Name: "H", Offset: 24},
		}}},
		{padded{}, paddedLayout()},
		{gdkRGBA{}, Layout{Size: 16, Align: 4, Fields: []Field{
			{Name: "Red", Offset: 0, Size: 4}, {Name: "Green", Offset: 4},
			{Name: "Blue", Offset: 8}, {Name: "Alpha", Offset: 12},
		}}},
		{struct{}{}, Layout{Size: 0, Align: 1}},
	}
	if runtime.GOARCH == "386" || runtime.GOARCH == "arm" {
		// float64 is 4-aligned on 386 but 8-aligned on arm; skip cgRect there.
		cases = cases[2:]
	}
	for _, c := range cases {
		var f fakeTB
		if !CheckLayout(&f, c.v, c.want) || len(f.errs) != 0 {
			t.Errorf("CheckLayout(%T) failed: %v", c.v, f.errs)
		}
		if f.helpers == 0 {
			t.Errorf("CheckLayout(%T) didn't call Helper", c.v)
		}
		// The real *testing.T satisfies TB.
		CheckLayout(t, c.v, c.want)
	}
}

func TestCheckLayoutReportsEveryDifference(t *testing.T) {
	// Expected as if the header had uint64_t b at 8 and no c, and an extra e.
	want := Layout{Size: 40, Align: 4, Fields: []Field{
		{Name: "A", Offset: 0, Size: 1},
		{Name: "B", Offset: 8, Size: 8},
		{Name: "D", Offset: 16},
		{Name: "E", Offset: 24},
	}}
	var f fakeTB
	if CheckLayout(&f, padded{}, want) {
		t.Fatal("CheckLayout reported a match")
	}
	if len(f.errs) != 1 {
		t.Fatalf("got %d Errorf calls, want 1 with every difference: %q", len(f.errs), f.errs)
	}
	got := f.errs[0]
	u := func(x uintptr) string { return fmt.Sprint(x) }
	lines := []string{
		"layout of ffi.padded on " + runtime.GOARCH + " differs from the C header:",
		"size: got " + u(unsafe.Sizeof(padded{})) + ", want 40",
		"field B: offset got 4, want 8",
		"field B: size got 4 (uint32), want 8",
		"field E: missing from the Go struct (want offset 24)",
		"field C: not in the expected layout (Go offset 8, uint16)",
	}
	if ptr == 8 {
		lines = append(lines, "align: got 8, want 4")
	} else {
		lines = append(lines, "field D: offset got 12, want 16")
	}
	for _, l := range lines {
		if !strings.Contains(got, l) {
			t.Errorf("report lacks %q:\n%s", l, got)
		}
	}
	// One difference per line, after the header line.
	if n, wantN := strings.Count(got, "\n\t"), len(lines)-1; n != wantN {
		t.Errorf("report has %d difference lines, want %d:\n%s", n, wantN, got)
	}
}

func TestCheckLayoutOrderAndDuplicates(t *testing.T) {
	l := paddedLayout()
	// Swap B and C in the expected list: the offsets still match, but the
	// Go struct declares them in another order.
	l.Fields[1], l.Fields[2] = l.Fields[2], l.Fields[1]
	l.Fields = append(l.Fields, Field{Name: "A", Offset: 0})
	var f fakeTB
	if CheckLayout(&f, padded{}, l) {
		t.Fatal("CheckLayout reported a match")
	}
	got := strings.Join(f.errs, "\n")
	for _, w := range []string{"field B: declared out of order", "field A: listed twice"} {
		if !strings.Contains(got, w) {
			t.Errorf("report lacks %q:\n%s", w, got)
		}
	}
}

func TestCheckLayoutNotAStruct(t *testing.T) {
	for _, v := range []any{nil, 42, new(int), []cgRect{}} {
		var f fakeTB
		if CheckLayout(&f, v, Layout{}) {
			t.Errorf("CheckLayout(%T) = true", v)
		}
		if len(f.errs) != 1 || !strings.Contains(f.errs[0], "is not a struct") {
			t.Errorf("CheckLayout(%T) reported %q", v, f.errs)
		}
	}
}

func TestCheckNoGoPointers(t *testing.T) {
	type inner struct {
		N uint32
		P *int
	}
	type clean struct {
		A    uintptr
		B    [4]float32
		R    cgRect
		H    Handle
		Arr  [2]gdkRGBA
		None [0]*int // zero-length arrays hold nothing
	}
	type dirty struct {
		A   uintptr
		S   string
		Sl  []byte
		M   map[int]int
		C   chan int
		F   func()
		I   any
		U   unsafe.Pointer
		In  inner
		Arr [3]*byte
	}
	var f fakeTB
	if !CheckNoGoPointers(&f, clean{}) || len(f.errs) != 0 {
		t.Fatalf("clean struct reported: %q", f.errs)
	}
	if !CheckNoGoPointers(&f, &clean{}) {
		t.Fatal("pointer to clean struct reported")
	}
	CheckNoGoPointers(t, clean{})

	f = fakeTB{}
	if CheckNoGoPointers(&f, dirty{}) {
		t.Fatal("dirty struct passed")
	}
	if len(f.errs) != 1 {
		t.Fatalf("got %d Errorf calls, want 1: %q", len(f.errs), f.errs)
	}
	got := f.errs[0]
	for _, w := range []string{
		"ffi.dirty contains Go pointers",
		"S (string)", "Sl ([]uint8)", "M (map[int]int)", "C (chan int)", "F (func())",
		"I (interface {})", "U (unsafe.Pointer)", "In.P (*int)", "Arr[0] (*uint8)",
	} {
		if !strings.Contains(got, w) {
			t.Errorf("report lacks %q:\n%s", w, got)
		}
	}
	if strings.Contains(got, "\tA ") || strings.Contains(got, "In.N") {
		t.Errorf("report lists pointer-free fields:\n%s", got)
	}
	if f.helpers == 0 {
		t.Error("CheckNoGoPointers didn't call Helper")
	}

	f = fakeTB{}
	if CheckNoGoPointers(&f, "x") || len(f.errs) != 1 || !strings.Contains(f.errs[0], "is not a struct") {
		t.Errorf("non-struct reported %q", f.errs)
	}
}
