package gtk4

import (
	"reflect"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/ffi"
)

// TestTrampolineSet checks the set on every host: numTrampolines entries
// with distinct "gtk4." names, every field of tramp filled by exactly one
// of them, and every function a valid native callback on linux (and
// darwin, which has the same rules), checked without creating one.
func TestTrampolineSet(t *testing.T) {
	set := trampolines()
	if len(set) != numTrampolines {
		t.Errorf("%d trampolines, want numTrampolines = %d", len(set), numTrampolines)
	}
	fields := map[*uintptr]string{}
	v := reflect.ValueOf(&tramp).Elem()
	for i := range v.NumField() {
		fields[(*uintptr)(v.Field(i).Addr().UnsafePointer())] = v.Type().Field(i).Name
	}
	if len(fields) != numTrampolines {
		t.Errorf("tramp has %d fields, want %d", len(fields), numTrampolines)
	}
	names := map[string]bool{}
	filled := map[*uintptr]bool{}
	for _, tr := range set {
		if !strings.HasPrefix(tr.name, "gtk4.") || names[tr.name] {
			t.Errorf("name %q is not a unique gtk4. name", tr.name)
		}
		names[tr.name] = true
		if _, ok := fields[tr.addr]; !ok {
			t.Errorf("%s fills something that is not a field of tramp", tr.name)
		} else if filled[tr.addr] {
			t.Errorf("%s fills tramp.%s, which another trampoline fills too", tr.name, fields[tr.addr])
		}
		filled[tr.addr] = true
		for _, goos := range []string{"linux", "darwin"} {
			if err := ffi.CheckCallback(tr.fn, goos); err != nil {
				t.Errorf("%s on %s: %v", tr.name, goos, err)
			}
		}
	}
	for p, name := range fields {
		if !filled[p] {
			t.Errorf("no trampoline fills tramp.%s", name)
		}
	}
}

// TestTrampolineSignatures pins each trampoline's Go signature to the C
// signature it stands for (gboolean and GtkOrientation are int32, every
// pointer a uintptr).
func TestTrampolineSignatures(t *testing.T) {
	want := map[string]any{
		nameClassInit:           func(klass, data uintptr) {},
		nameSizeAllocate:        func(w uintptr, width, height, baseline int32) {},
		nameMeasure:             func(w uintptr, o, forSize int32, mn, nt, mnb, ntb uintptr) {},
		nameSnapshot:            func(w, s uintptr) {},
		nameClosureNotify:       func(data, closure uintptr) {},
		nameSourceFunc:          func(data uintptr) int32 { return 0 },
		nameSigBool:             func(inst, data uintptr) int32 { return 0 },
		nameSigIntDoubleDouble:  func(inst uintptr, n int32, x, y float64, data uintptr) {},
		nameSigPointer:          func(inst, arg, data uintptr) {},
		nameSigDoubleDouble:     func(inst uintptr, x, y float64, data uintptr) {},
		nameSigVoid:             func(inst, data uintptr) {},
		nameSigBoolDoubleDouble: func(inst uintptr, x, y float64, data uintptr) int32 { return 0 },
		nameSigBoolUintUintUint: func(inst uintptr, a, b, c uint32, data uintptr) int32 { return 0 },
		nameSigUintUintUint:     func(inst uintptr, a, b, c uint32, data uintptr) {},
		nameTick:                func(w, frameClock, data uintptr) int32 { return 0 },
		nameDestroyNotify:       func(data uintptr) {},
		nameAsyncReady:          func(source, result, data uintptr) {},
	}
	for _, tr := range trampolines() {
		w, ok := want[tr.name]
		if !ok {
			t.Errorf("%s has no expected signature", tr.name)
			continue
		}
		if got, wt := reflect.TypeOf(tr.fn), reflect.TypeOf(w); got != wt {
			t.Errorf("%s is %v, want %v", tr.name, got, wt)
		}
	}
}
