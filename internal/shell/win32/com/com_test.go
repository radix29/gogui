//go:build windows

package com

import (
	"errors"
	"math"
	"runtime"
	"testing"
	"unsafe"

	"github.com/radix29/gogui/internal/ffi"
	"golang.org/x/sys/windows"
)

// The real COM object of these tests is WIC's imaging factory: an
// in-process class of every Windows since Vista, created fresh per
// CoCreateInstance, so its reference count starts at 1.
var (
	clsidWICImagingFactory = MustGUID("{CACAF262-9370-4615-A13B-9F5539DA4C0A}")
	iidIWICImagingFactory  = MustGUID("{EC5EC8A9-C395-4314-9C77-54D7A935FF70}")
	iidIDispatch           = MustGUID("{00020400-0000-0000-C000-000000000046}")
)

// onFreshThread runs f on a goroutine locked to an OS thread that is never
// unlocked, so the thread ends with the goroutine and its COM state can't
// leak into another test.
func onFreshThread(t *testing.T, f func()) {
	t.Helper()
	done := make(chan any)
	go func() {
		runtime.LockOSThread()
		defer func() { done <- recover() }()
		f()
	}()
	if r := <-done; r != nil {
		t.Fatalf("panic: %v", r)
	}
}

// inApartment is onFreshThread on an initialized STA.
func inApartment(t *testing.T, f func()) {
	t.Helper()
	onFreshThread(t, func() {
		uninit, err := InitUIThread()
		if err != nil {
			t.Errorf("InitUIThread: %v", err)
			return
		}
		defer uninit()
		f()
	})
}

func TestGUIDLayout(t *testing.T) {
	ffi.CheckLayout(t, GUID{}, ffi.Layout{Size: 16, Align: 4, Fields: []ffi.Field{
		{Name: "Data1", Offset: 0, Size: 4}, {Name: "Data2", Offset: 4, Size: 2},
		{Name: "Data3", Offset: 6, Size: 2}, {Name: "Data4", Offset: 8, Size: 8}}})
	ffi.CheckNoGoPointers(t, GUID{})
	if unsafe.Sizeof(GUID{}) != unsafe.Sizeof(windows.GUID{}) {
		t.Error("GUID and windows.GUID differ in size")
	}
	// Same bytes as x/sys's parser.
	s := "{6B29FC40-CA47-1067-B31D-00DD010662DA}"
	xg, err := windows.GUIDFromString(s)
	if err != nil {
		t.Fatal(err)
	}
	if g := MustGUID(s); *(*windows.GUID)(unsafe.Pointer(&g)) != xg {
		t.Errorf("%v differs from windows.GUIDFromString", g)
	}
}

func TestApartment(t *testing.T) {
	onFreshThread(t, func() {
		// Without an apartment, CoCreateInstance fails.
		_, err := CreateInstance(&clsidWICImagingFactory, &iidIWICImagingFactory)
		if !errors.Is(err, CO_E_NOTINITIALIZED) {
			t.Errorf("CreateInstance before InitUIThread: %v", err)
		}
		uninit, err := InitUIThread()
		if err != nil {
			t.Fatalf("InitUIThread: %v", err)
		}
		// A second STA init on the thread is S_FALSE: success, one more uninit.
		uninit2, err := InitUIThread()
		if err != nil {
			t.Errorf("second InitUIThread: %v", err)
		} else {
			uninit2()
		}
		uninit()
	})
	onFreshThread(t, func() {
		// An MTA thread refuses the STA.
		if err := windows.CoInitializeEx(0, windows.COINIT_MULTITHREADED); err != nil {
			t.Fatal(err)
		}
		defer windows.CoUninitialize()
		if _, err := InitUIThread(); !errors.Is(err, RPC_E_CHANGED_MODE) {
			t.Errorf("InitUIThread on an MTA: %v", err)
		}
	})
}

func TestCreateInstanceUnknownClass(t *testing.T) {
	inApartment(t, func() {
		bogus := MustGUID("{01234567-89AB-CDEF-0123-456789ABCDEF}")
		p, err := CreateInstance(&bogus, &IID_IUnknown)
		if !errors.Is(err, REGDB_E_CLASSNOTREG) || p != 0 {
			t.Errorf("CreateInstance of an unknown class: %v, %v", p, err)
		}
	})
}

func TestRefCounts(t *testing.T) {
	inApartment(t, func() {
		f, err := CreateInstance(&clsidWICImagingFactory, &iidIWICImagingFactory)
		if err != nil {
			t.Fatalf("CreateInstance: %v", err)
		}
		if f.IsNil() {
			t.Fatal("nil factory")
		}
		start := f.RefCount()
		if start != 1 {
			t.Errorf("start count %d, want 1", start)
		}
		if n := f.AddRef(); n != start+1 {
			t.Errorf("AddRef = %d", n)
		}
		g := f
		if n := g.Release(); n != start || g != 0 {
			t.Errorf("Release = %d, Ptr %v", n, g)
		}
		if n := g.Release(); n != 0 {
			t.Errorf("Release of nil = %d", n)
		}

		// QueryInterface: success adds a reference, the same object answers
		// IUnknown identity, and a missing interface is E_NOINTERFACE.
		u, err := f.QueryInterface(&IID_IUnknown)
		if err != nil || u.IsNil() {
			t.Fatalf("QueryInterface(IUnknown): %v, %v", u, err)
		}
		if n := f.RefCount(); n != start+1 {
			t.Errorf("count after QueryInterface %d, want %d", n, start+1)
		}
		u2, err := u.QueryInterface(&IID_IUnknown)
		if err != nil || u2 != u {
			t.Errorf("IUnknown identity: %v vs %v, %v", u2, u, err)
		}
		u2.Release()
		self, err := u.QueryInterface(&iidIWICImagingFactory)
		if err != nil || self != f {
			t.Errorf("back to the factory: %v vs %v, %v", self, f, err)
		}
		self.Release()
		u.Release()

		d, err := f.QueryInterface(&iidIDispatch)
		if !errors.Is(err, E_NOINTERFACE) || d != 0 {
			t.Errorf("QueryInterface(IDispatch) = %v, %v; want E_NOINTERFACE", d, err)
		}
		var ce *Error
		if !errors.As(err, &ce) || ce.Op != "QueryInterface("+iidIDispatch.String()+")" {
			t.Errorf("error op: %v", err)
		}

		// CallHR returns the HRESULT as an *Error: QueryInterface with a NULL
		// out pointer fails (E_POINTER by the SDK, E_INVALIDARG from WIC).
		if err := f.CallHR("QueryInterface", slotQueryInterface, uintptr(unsafe.Pointer(&IID_IUnknown)), 0); !errors.Is(err, E_POINTER) && !errors.Is(err, E_INVALIDARG) {
			t.Errorf("QueryInterface(NULL out) = %v", err)
		}

		if n := f.RefCount(); n != start {
			t.Errorf("count %d before the last Release, want %d", n, start)
		}
		if n := f.Release(); n != 0 {
			t.Errorf("last Release = %d", n)
		}
	})
}

func TestNilPtrPanics(t *testing.T) {
	for name, f := range map[string]func(){
		"Slot":     func() { Ptr(0).Slot(0) },
		"AddRef":   func() { Ptr(0).AddRef() },
		"negative": func() { Ptr(8).Slot(-1) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s did not panic", name)
				}
			}()
			f()
		}()
	}
}

// d2d1Matrix mirrors D2D1_MATRIX_3X2_F.
type d2d1MatrixF struct{ M11, M12, M21, M22, Dx, Dy float32 }

// d2d1Point mirrors D2D1_POINT_2F, a float HFA passed by value.
type d2d1Point2F struct{ X, Y float32 }

func TestFloatPath(t *testing.T) {
	ucrt := windows.NewLazySystemDLL("ucrtbase.dll")
	d2d1 := windows.NewLazySystemDLL("d2d1.dll")
	addr := func(dll *windows.LazyDLL, name string) uintptr {
		p := dll.NewProc(name)
		if err := p.Find(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return p.Addr()
	}

	// float(float, float, float) and double(double, double, double).
	var fmaf func(x, y, z float32) float32
	if err := BindFunc(&fmaf, addr(ucrt, "fmaf")); err != nil {
		t.Fatal(err)
	}
	if got := fmaf(1.5, -2.25, 0.125); got != 1.5*-2.25+0.125 {
		t.Errorf("fmaf = %v", got)
	}
	var fma func(x, y, z float64) float64
	if err := BindFunc(&fma, addr(ucrt, "fma")); err != nil {
		t.Fatal(err)
	}
	if got := fma(3, 1e10, -0.5); got != 3e10-0.5 {
		t.Errorf("fma = %v", got)
	}
	// Mixed integer and float arguments: double ldexp(double, int).
	var ldexp func(x float64, e int32) float64
	if err := BindFunc(&ldexp, addr(ucrt, "ldexp")); err != nil {
		t.Fatal(err)
	}
	if got := ldexp(0.75, 5); got != 24 {
		t.Errorf("ldexp = %v", got)
	}
	// Integer first, float after it (amd64 is positional: XMM1).
	var scalbnf func(x float32, n int32) float32
	if err := BindFunc(&scalbnf, addr(ucrt, "scalbnf")); err != nil {
		t.Fatal(err)
	}
	if got := scalbnf(-3, 3); got != -24 {
		t.Errorf("scalbnf = %v", got)
	}

	// D2D's own flat exports: FLOAT D2D1Vec3Length(FLOAT, FLOAT, FLOAT) and
	// void D2D1MakeRotateMatrix(FLOAT, D2D1_POINT_2F, D2D1_MATRIX_3X2_F*),
	// the shape of the float-taking D2D methods (a FLOAT and a float struct
	// by value next to a pointer).
	var vec3Length func(x, y, z float32) float32
	if err := BindFunc(&vec3Length, addr(d2d1, "D2D1Vec3Length")); err != nil {
		t.Fatal(err)
	}
	if got := vec3Length(2, 3, 6); got != 7 {
		t.Errorf("D2D1Vec3Length = %v", got)
	}
	var rotate func(angle float32, center d2d1Point2F, m *d2d1MatrixF)
	if err := BindFunc(&rotate, addr(d2d1, "D2D1MakeRotateMatrix")); err != nil {
		t.Fatal(err)
	}
	var m d2d1MatrixF
	rotate(90, d2d1Point2F{10, 20}, &m)
	// Rotating by 90° about (10, 20): x' = -y + 30, y' = x + 10.
	near := func(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-4 }
	if !near(m.M11, 0) || !near(m.M12, 1) || !near(m.M21, -1) || !near(m.M22, 0) || !near(m.Dx, 30) || !near(m.Dy, 10) {
		t.Errorf("D2D1MakeRotateMatrix(90, (10,20)) = %+v", m)
	}
	var maxScale func(m *d2d1MatrixF) float32
	if err := BindFunc(&maxScale, addr(d2d1, "D2D1ComputeMaximumScaleFactor")); err != nil {
		t.Fatal(err)
	}
	if got := maxScale(&d2d1MatrixF{M11: 2, M22: 3}); !near(got, 3) {
		t.Errorf("D2D1ComputeMaximumScaleFactor = %v", got)
	}
}

func TestBind(t *testing.T) {
	inApartment(t, func() {
		f, err := CreateInstance(&clsidWICImagingFactory, &iidIWICImagingFactory)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Release()

		// The vtable path of Bind on real slots: AddRef and Release.
		var addRef, release func(this Ptr) uint32
		if err := Bind(&addRef, f, slotAddRef); err != nil {
			t.Fatal(err)
		}
		if err := Bind(&release, f, slotRelease); err != nil {
			t.Fatal(err)
		}
		if n := addRef(f); n != 2 {
			t.Errorf("bound AddRef = %d", n)
		}
		if n := release(f); n != 1 {
			t.Errorf("bound Release = %d", n)
		}
		var qi func(this Ptr, iid *GUID, out *Ptr) HRESULT
		if err := Bind(&qi, f, slotQueryInterface); err != nil {
			t.Fatal(err)
		}
		var d Ptr
		if hr := qi(f, &iidIDispatch, &d); hr != E_NOINTERFACE || d != 0 {
			t.Errorf("bound QueryInterface = %v, %v", hr, d)
		}

		// Cached per signature and code address, not per object.
		before := boundFuncs()
		g, err := CreateInstance(&clsidWICImagingFactory, &iidIWICImagingFactory)
		if err != nil {
			t.Fatal(err)
		}
		defer g.Release()
		var addRef2 func(this Ptr) uint32
		if err := Bind(&addRef2, g, slotAddRef); err != nil {
			t.Fatal(err)
		}
		if boundFuncs() != before {
			t.Errorf("binding a second object's AddRef added a cache entry (%d -> %d)", before, boundFuncs())
		}
		var addRef3 func(this Ptr) uint64
		if err := Bind(&addRef3, g, slotAddRef); err != nil {
			t.Fatal(err)
		}
		if boundFuncs() != before+1 {
			t.Errorf("a new signature did not add an entry (%d -> %d)", before, boundFuncs())
		}
		if n := addRef2(g); n != 2 {
			t.Errorf("AddRef through the cached func = %d", n)
		}
		g2 := g
		g2.Release()

		// Rejections leave *fptr unchanged.
		var keep func(this Ptr) uint32
		bad := []struct {
			name string
			fptr any
			p    Ptr
			slot int
		}{
			{"nil this", &keep, 0, 1},
			{"negative slot", &keep, f, -1},
			{"not a pointer", keep, f, 1},
			{"nil pointer", (*func(Ptr) uint32)(nil), f, 1},
			{"not a func", new(int), f, 1},
			{"no this", new(func() uint32), f, 1},
			{"float this", new(func(float32) uint32), f, 1},
			{"struct result", new(func(Ptr) d2d1Point2F), f, 1},
			{"Go func argument", new(func(Ptr, func()) uint32), f, 1},
			{"variadic", new(func(Ptr, ...uintptr) uint32), f, 1},
			{"func result", new(func(Ptr) func()), f, 1},
			{"two results", new(func(Ptr) (uint32, uint32)), f, 1},
		}
		for _, b := range bad {
			if err := Bind(b.fptr, b.p, b.slot); err == nil {
				t.Errorf("Bind %s: no error", b.name)
			}
		}
		if keep != nil {
			t.Error("a rejected Bind set the func")
		}
		if err := BindFunc(&keep, 0); err == nil {
			t.Error("BindFunc(0): no error")
		}
	})
}
