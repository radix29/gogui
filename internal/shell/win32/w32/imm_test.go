//go:build windows

package w32

import (
	"errors"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"
)

// TestImmContext gets and releases the input context of a real top-level
// window, reads the (empty) composition of it, sets the composition and
// candidate forms, and toggles the association. No IME composes on the
// host, so the composition strings are empty; their contents are tested
// through the shell's imm seam (S5.3f).
func TestImmContext(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	inst := newTestClass(t, "gogui.w32test.imm", DefWindowProcAddr())
	h, err := CreateWindowEx(0, "gogui.w32test.imm", "imm test", WS_OVERLAPPEDWINDOW,
		10, 20, 400, 300, 0, 0, inst, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer DestroyWindow(h)

	himc := ImmGetContext(h)
	if himc == 0 {
		t.Fatal("ImmGetContext(new window) = 0; want the default context")
	}
	for _, idx := range []uint32{GCS_COMPSTR, GCS_RESULTSTR, GCS_COMPREADSTR} {
		if s, err := ImmGetCompositionString(himc, idx); len(s) != 0 || err != nil {
			t.Errorf("ImmGetCompositionString(%#x) = %v, %v; want empty", idx, s, err)
		}
	}
	if a, err := ImmGetCompositionAttr(himc, GCS_COMPATTR); len(a) != 0 || err != nil {
		t.Errorf("ImmGetCompositionAttr = %v, %v; want empty", a, err)
	}
	if c, err := ImmGetCompositionCursorPos(himc); c != 0 || err != nil {
		t.Errorf("ImmGetCompositionCursorPos = %d, %v; want 0", c, err)
	}
	cf := CompositionForm{DwStyle: CFS_POINT, PtCurrentPos: Point{12, 34}}
	if err := ImmSetCompositionWindow(himc, &cf); err != nil {
		t.Error(err)
	}
	cand := CandidateForm{DwStyle: CFS_EXCLUDE, PtCurrentPos: Point{12, 34},
		RcArea: Rect{Left: 12, Top: 14, Right: 14, Bottom: 34}}
	if err := ImmSetCandidateWindow(himc, &cand); err != nil {
		t.Error(err)
	}
	// Without an open IME there is no composition to cancel; either result
	// is fine, the call must just not crash.
	if err := ImmNotifyIME(himc, NI_COMPOSITIONSTR, CPS_CANCEL, 0); err != nil {
		t.Logf("ImmNotifyIME(CPS_CANCEL) without an IME: %v (allowed)", err)
	}
	if err := ImmReleaseContext(h, himc); err != nil {
		t.Error(err)
	}

	// No context: the IME is off for the window.
	if err := ImmAssociateContextEx(h, 0, 0); err != nil {
		t.Fatal(err)
	}
	if c := ImmGetContext(h); c != 0 {
		ImmReleaseContext(h, c)
		t.Errorf("ImmGetContext after disassociating = %#x; want 0", c)
	}
	// The default context back.
	if err := ImmAssociateContextEx(h, 0, IACE_DEFAULT); err != nil {
		t.Fatal(err)
	}
	c := ImmGetContext(h)
	if c == 0 {
		t.Fatal("ImmGetContext after IACE_DEFAULT = 0")
	}
	if c != himc {
		t.Errorf("default context after re-association %#x, before %#x; want the same", c, himc)
	}
	if err := ImmReleaseContext(h, c); err != nil {
		t.Error(err)
	}

	// Context 0 reads fail, they do not crash.
	if _, err := ImmGetCompositionString(0, GCS_COMPSTR); err == nil {
		t.Log("ImmGetCompositionString(0) succeeded (allowed: empty)")
	} else if !errors.Is(err, ErrIMMGeneral) && !errors.Is(err, ErrIMMNoData) {
		t.Errorf("ImmGetCompositionString(0) = %v; want an IMM_ERROR_* error", err)
	}
	if err := ImmAssociateContextEx(windows.HWND(0xDEAD0), 0, IACE_DEFAULT); err == nil {
		t.Error("ImmAssociateContextEx(invalid HWND) succeeded")
	}
}
