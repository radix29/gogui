package com

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestParseGUID(t *testing.T) {
	want := GUID{0x6B29FC40, 0xCA47, 0x1067, [8]byte{0xB3, 0x1D, 0x00, 0xDD, 0x01, 0x06, 0x62, 0xDA}}
	for _, s := range []string{
		"{6B29FC40-CA47-1067-B31D-00DD010662DA}",
		"6B29FC40-CA47-1067-B31D-00DD010662DA",
		"{6b29fc40-ca47-1067-b31d-00dd010662da}",
	} {
		g, err := ParseGUID(s)
		if err != nil || g != want {
			t.Errorf("ParseGUID(%q) = %v, %v; want %v", s, g, err, want)
		}
	}
	if got := want.String(); got != "{6B29FC40-CA47-1067-B31D-00DD010662DA}" {
		t.Errorf("String = %s", got)
	}
	if IID_IUnknown != (GUID{0, 0, 0, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}) {
		t.Errorf("IID_IUnknown = %v", IID_IUnknown)
	}
	for _, s := range []string{
		"", "{}", "6B29FC40CA471067B31D00DD010662DA",
		"{6B29FC40-CA47-1067-B31D-00DD010662DA", "6B29FC40-CA47-1067-B31D-00DD010662DA}",
		"{6B29FC40-CA47-1067-B31D-00DD010662DG}", "{6B29FC40-CA47-1067-B31D-00DD010662D}",
		"{+B29FC40-CA47-1067-B31D-00DD010662DA}", "{6B29FC40_CA47-1067-B31D-00DD010662DA}",
		"{ 6B29FC40-CA47-1067-B31D-00DD010662DA}",
	} {
		if g, err := ParseGUID(s); err == nil {
			t.Errorf("ParseGUID(%q) = %v, want an error", s, g)
		}
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("MustGUID of a bad string did not panic")
			}
		}()
		MustGUID("nope")
	}()
}

func TestGUIDRoundTrip(t *testing.T) {
	g := GUID{0xDEADBEEF, 0x0102, 0xA0B0, [8]byte{1, 2, 3, 4, 5, 6, 7, 0xFF}}
	if back := MustGUID(g.String()); back != g {
		t.Errorf("round trip %v -> %v", g, back)
	}
}

func TestHRESULT(t *testing.T) {
	cases := []struct {
		hr       HRESULT
		raw      uint32
		failed   bool
		facility uint16
		code     uint16
		text     string
	}{
		{S_OK, 0, false, 0, 0, "S_OK (0x00000000)"},
		{S_FALSE, 1, false, 0, 1, "S_FALSE (0x00000001)"},
		{E_NOINTERFACE, 0x80004002, true, 0, 0x4002, "E_NOINTERFACE (0x80004002)"},
		{E_FAIL, 0x80004005, true, 0, 0x4005, "E_FAIL (0x80004005)"},
		{E_NOTIMPL, 0x80004001, true, 0, 0x4001, "E_NOTIMPL (0x80004001)"},
		{E_POINTER, 0x80004003, true, 0, 0x4003, "E_POINTER (0x80004003)"},
		{E_ABORT, 0x80004004, true, 0, 0x4004, "E_ABORT (0x80004004)"},
		{E_UNEXPECTED, 0x8000FFFF, true, 0, 0xFFFF, "E_UNEXPECTED (0x8000FFFF)"},
		{E_ACCESSDENIED, 0x80070005, true, 7, 5, "E_ACCESSDENIED (0x80070005)"},
		{E_HANDLE, 0x80070006, true, 7, 6, "E_HANDLE (0x80070006)"},
		{E_OUTOFMEMORY, 0x8007000E, true, 7, 14, "E_OUTOFMEMORY (0x8007000E)"},
		{E_INVALIDARG, 0x80070057, true, 7, 0x57, "E_INVALIDARG (0x80070057)"},
		{E_NOT_SUFFICIENT_BUFFER, 0x8007007A, true, 7, 0x7A, "E_NOT_SUFFICIENT_BUFFER (0x8007007A)"},
		{RPC_E_CHANGED_MODE, 0x80010106, true, 1, 0x106, "RPC_E_CHANGED_MODE (0x80010106)"},
		{CO_E_NOTINITIALIZED, 0x800401F0, true, 4, 0x1F0, "CO_E_NOTINITIALIZED (0x800401F0)"},
		{REGDB_E_CLASSNOTREG, 0x80040154, true, 4, 0x154, "REGDB_E_CLASSNOTREG (0x80040154)"},
		{DXGI_ERROR_INVALID_CALL, 0x887A0001, true, 0x87A, 1, "DXGI_ERROR_INVALID_CALL (0x887A0001)"},
		{DXGI_ERROR_NOT_FOUND, 0x887A0002, true, 0x87A, 2, "DXGI_ERROR_NOT_FOUND (0x887A0002)"},
		{DXGI_ERROR_DEVICE_REMOVED, 0x887A0005, true, 0x87A, 5, "DXGI_ERROR_DEVICE_REMOVED (0x887A0005)"},
		{DXGI_ERROR_DEVICE_HUNG, 0x887A0006, true, 0x87A, 6, "DXGI_ERROR_DEVICE_HUNG (0x887A0006)"},
		{DXGI_ERROR_DEVICE_RESET, 0x887A0007, true, 0x87A, 7, "DXGI_ERROR_DEVICE_RESET (0x887A0007)"},
		{DXGI_ERROR_WAS_STILL_DRAWING, 0x887A000A, true, 0x87A, 10, "DXGI_ERROR_WAS_STILL_DRAWING (0x887A000A)"},
		{D2DERR_RECREATE_TARGET, 0x8899000C, true, 0x899, 12, "D2DERR_RECREATE_TARGET (0x8899000C)"},
		{DWRITE_E_FILEFORMAT, 0x88985000, true, 0x898, 0x5000, "DWRITE_E_FILEFORMAT (0x88985000)"},
		{HRESULT(-0x7785ff67), 0x887A0099, true, 0x87A, 0x99, "HRESULT 0x887A0099 (facility 0x87A, code 153)"},
		{HRESULT(2), 2, false, 0, 2, "HRESULT 0x00000002 (facility 0x0, code 2)"},
	}
	if len(hresultNames) != len(cases)-2 {
		t.Errorf("%d named codes, %d covered here", len(hresultNames), len(cases)-2)
	}
	for _, c := range cases {
		if uint32(c.hr) != c.raw {
			t.Errorf("%s: raw 0x%08X, want 0x%08X", c.text, uint32(c.hr), c.raw)
		}
		if c.hr.Failed() != c.failed || c.hr.Succeeded() == c.failed {
			t.Errorf("%s: Failed %v, Succeeded %v", c.text, c.hr.Failed(), c.hr.Succeeded())
		}
		if c.hr.Facility() != c.facility || c.hr.Code() != c.code {
			t.Errorf("%s: facility 0x%X code 0x%X, want 0x%X 0x%X", c.text, c.hr.Facility(), c.hr.Code(), c.facility, c.code)
		}
		if got := c.hr.Error(); got != c.text {
			t.Errorf("Error = %q, want %q", got, c.text)
		}
		err := c.hr.Err("Op")
		if (err != nil) != c.failed {
			t.Errorf("%s: Err = %v", c.text, err)
		}
	}
}

func TestErrorWrapping(t *testing.T) {
	err := fmt.Errorf("creating: %w", E_NOINTERFACE.Err("QueryInterface"))
	if !errors.Is(err, E_NOINTERFACE) || errors.Is(err, E_FAIL) {
		t.Errorf("errors.Is on %v", err)
	}
	var ce *Error
	if !errors.As(err, &ce) || ce.HR != E_NOINTERFACE || ce.Op != "QueryInterface" {
		t.Errorf("errors.As on %v: %+v", err, ce)
	}
	var hr HRESULT
	if !errors.As(err, &hr) || hr != E_NOINTERFACE {
		t.Errorf("errors.As HRESULT on %v: %v", err, hr)
	}
	if got := err.Error(); !strings.Contains(got, "com: QueryInterface: E_NOINTERFACE (0x80004002)") {
		t.Errorf("text %q", got)
	}
}

type slotTB struct {
	errs []string
}

func (s *slotTB) Helper() {}
func (s *slotTB) Errorf(format string, args ...any) {
	s.errs = append(s.errs, fmt.Sprintf(format, args...))
}

func TestVtable(t *testing.T) {
	v := IUnknownVtable.Extend("A", "B")
	if len(IUnknownVtable) != 3 || v.Slot("QueryInterface") != 0 || v.Slot("Release") != 2 ||
		v.Slot("B") != 4 || v.Slot("C") != -1 {
		t.Errorf("Vtable %v", v)
	}
	w := v.Extend("C")
	if v.Slot("C") != -1 || w.Slot("C") != 5 {
		t.Error("Extend shares its backing array")
	}
	if !CheckSlots(t, "I", w, map[string]int{"A": 3, "C": 5}) {
		t.Error("CheckSlots rejected a good table")
	}
	var s slotTB
	bad := w.Extend("A")
	if CheckSlots(&s, "I", bad, map[string]int{"B": 3, "Z": 9}) || len(s.errs) != 3 {
		t.Errorf("CheckSlots on a bad table: %v", s.errs)
	}
	for _, want := range []string{"A listed twice", "I::B: slot 3, want 4", "no method Z"} {
		if !strings.Contains(strings.Join(s.errs, "\n"), want) {
			t.Errorf("missing %q in %v", want, s.errs)
		}
	}
}
