//go:build windows

package w32

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/ffi"
)

// TestProcsResolve finds every declared proc, so a misspelled name or a
// proc missing from this Windows fails here and not at its first call.
func TestProcsResolve(t *testing.T) {
	// Every newProc call in procs.go is registered (none bypasses allProcs).
	src, err := os.ReadFile("procs.go")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(src), "newProc(") - 1; len(allProcs) != n {
		t.Fatalf("%d procs registered, %d newProc calls in procs.go", len(allProcs), n)
	}
	if !strings.Contains(string(src), ".NewProc(") {
		t.Fatal("procs.go: newProc no longer wraps NewProc")
	}
	seen := map[string]bool{}
	for _, p := range allProcs {
		key := p.Name
		if seen[key] {
			t.Errorf("proc %s declared twice", key)
		}
		seen[key] = true
		if err := p.Find(); err != nil {
			t.Errorf("proc %s: %v", key, err)
		}
	}
}

func TestSimpleCalls(t *testing.T) {
	if dpi := GetDpiForSystem(); dpi < 96 || dpi > 96*5 {
		t.Errorf("GetDpiForSystem = %d, want 96…480", dpi)
	}
	if windows.GetCurrentThreadId() == 0 {
		t.Error("GetCurrentThreadId = 0")
	}
	if inst, err := GetModuleHandle(); err != nil || inst == 0 {
		t.Errorf("GetModuleHandle = %#x, %v", inst, err)
	}
	if GetDoubleClickTime() == 0 {
		t.Error("GetDoubleClickTime = 0")
	}
	if n := GetSystemMetricsForDpi(SM_CXPADDEDBORDER, 96); n < 0 {
		t.Errorf("SM_CXPADDEDBORDER at 96 DPI = %d", n)
	}
	if _, err := SystemParametersInfoUint32(SPI_GETWHEELSCROLLLINES); err != nil {
		t.Error(err)
	}
	if hc, err := SystemParametersInfoHighContrast(); err != nil || hc.CbSize != 16 {
		t.Errorf("SystemParametersInfoHighContrast = %+v, %v", hc, err)
	}
	if c, err := LoadCursor(IDC_ARROW); err != nil || c == 0 {
		t.Errorf("LoadCursor(IDC_ARROW) = %#x, %v", c, err)
	}
	mon := MonitorFromPoint(Point{0, 0}, MONITOR_DEFAULTTOPRIMARY)
	mi, err := GetMonitorInfo(mon)
	if err != nil || mi.DwFlags&MONITORINFOF_PRIMARY == 0 || mi.RcMonitor.Right <= mi.RcMonitor.Left {
		t.Errorf("GetMonitorInfo(primary) = %+v, %v", mi, err)
	}
	if x, y, err := GetDpiForMonitor(mon, MDT_EFFECTIVE_DPI); err != nil || x == 0 || x != y {
		t.Errorf("GetDpiForMonitor(primary) = %d, %d, %v", x, y, err)
	}
	if _, _, err := DwmGetColorizationColor(); err != nil {
		t.Logf("DwmGetColorizationColor: %v (allowed: composition may be off)", err)
	}
	// An error from GetLastError wraps the Errno.
	err = DestroyWindow(0)
	if !errors.Is(err, windows.ERROR_INVALID_WINDOW_HANDLE) {
		t.Errorf("DestroyWindow(0) = %v, want ERROR_INVALID_WINDOW_HANDLE", err)
	}
	// An HRESULT failure wraps the HRESULT.
	if err := hresult("x", uintptr(0x80070057)); !errors.Is(err, windows.Errno(0x80070057)) {
		t.Errorf("hresult(E_INVALIDARG) = %v", err)
	}
	if err := hresult("x", 1); err != nil { // S_FALSE is a success
		t.Errorf("hresult(S_FALSE) = %v", err)
	}
}

// newTestClass registers a window class with wndproc under name and
// unregisters it at the end of the test.
func newTestClass(t *testing.T, name string, wndproc uintptr) windows.Handle {
	t.Helper()
	inst, err := GetModuleHandle()
	if err != nil {
		t.Fatal(err)
	}
	wc := WndClassEx{LpfnWndProc: wndproc, HInstance: inst}
	atom, err := RegisterClassEx(&wc, name)
	if err != nil || atom == 0 {
		t.Fatalf("RegisterClassEx(%s) = %d, %v", name, atom, err)
	}
	if wc.CbSize != 80 || wc.LpszClassName != 0 {
		t.Errorf("after RegisterClassEx: CbSize %d, LpszClassName %#x; want 80, 0", wc.CbSize, wc.LpszClassName)
	}
	if _, err := RegisterClassEx(&wc, name); !errors.Is(err, windows.ERROR_CLASS_ALREADY_EXISTS) {
		t.Errorf("second RegisterClassEx(%s) = %v, want ERROR_CLASS_ALREADY_EXISTS", name, err)
	}
	t.Cleanup(func() {
		if err := UnregisterClass(name, inst); err != nil {
			t.Errorf("UnregisterClass(%s): %v", name, err)
		}
	})
	return inst
}

// TestWindowLongPtrZero checks the 0-return rule of Get/SetWindowLongPtrW
// on a message-only window, and the message-loop wrappers.
func TestWindowLongPtrZero(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	inst := newTestClass(t, "gogui.w32test.plain", DefWindowProcAddr())
	h, err := CreateWindowEx(0, "gogui.w32test.plain", "", 0, 0, 0, 0, 0,
		windows.HWND(HWND_MESSAGE), 0, inst, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := DestroyWindow(h); err != nil {
			t.Error(err)
		}
	}()

	// A stale non-zero last error must not turn a 0 result into a failure.
	SetLastError(uint32(windows.ERROR_ACCESS_DENIED))
	if v, err := GetWindowLongPtr(h, GWLP_USERDATA); v != 0 || err != nil {
		t.Fatalf("GetWindowLongPtr(new window) = %#x, %v; want 0, nil", v, err)
	}
	SetLastError(uint32(windows.ERROR_ACCESS_DENIED))
	if prev, err := SetWindowLongPtr(h, GWLP_USERDATA, 0x1234); prev != 0 || err != nil {
		t.Fatalf("SetWindowLongPtr(0 → 0x1234) = %#x, %v; want 0, nil", prev, err)
	}
	if prev, err := SetWindowLongPtr(h, GWLP_USERDATA, ^uintptr(0)>>1); prev != 0x1234 || err != nil {
		t.Fatalf("SetWindowLongPtr(0x1234 → max) = %#x, %v", prev, err)
	}
	if v, err := GetWindowLongPtr(h, GWLP_USERDATA); v != ^uintptr(0)>>1 || err != nil {
		t.Fatalf("GetWindowLongPtr = %#x, %v; want the full 64-bit value", v, err)
	}
	if prev, err := SetWindowLongPtr(h, GWLP_USERDATA, 0); prev != ^uintptr(0)>>1 || err != nil {
		t.Fatalf("SetWindowLongPtr(→ 0) = %#x, %v", prev, err)
	}
	// A real failure: an invalid window.
	if _, err := GetWindowLongPtr(0, GWLP_USERDATA); !errors.Is(err, windows.ERROR_INVALID_WINDOW_HANDLE) {
		t.Errorf("GetWindowLongPtr(0) = %v, want ERROR_INVALID_WINDOW_HANDLE", err)
	}
	if _, err := SetWindowLongPtr(0, GWLP_USERDATA, 1); !errors.Is(err, windows.ERROR_INVALID_WINDOW_HANDLE) {
		t.Errorf("SetWindowLongPtr(0) = %v, want ERROR_INVALID_WINDOW_HANDLE", err)
	}

	// PostMessage → GetMessage, then WM_QUIT → (false, nil), then -1.
	if err := PostMessage(h, WM_APP+1, 7, 9); err != nil {
		t.Fatal(err)
	}
	var m Msg
	if !PeekMessage(&m, h, WM_APP+1, WM_APP+1, PM_NOREMOVE) || m.Message != WM_APP+1 {
		t.Fatalf("PeekMessage = %+v", m)
	}
	ok, err := GetMessage(&m, h, 0, 0)
	if !ok || err != nil || m.Hwnd != h || m.Message != WM_APP+1 || m.WParam != 7 || m.LParam != 9 {
		t.Fatalf("GetMessage = %v, %v, %+v", ok, err, m)
	}
	PostQuitMessage(3)
	if ok, err := GetMessage(&m, 0, 0, 0); ok || err != nil || m.Message != WM_QUIT || m.WParam != 3 {
		t.Fatalf("GetMessage after PostQuitMessage = %v, %v, %+v; want WM_QUIT 3", ok, err, m)
	}
	if ok, err := GetMessage(&m, windows.HWND(0xDEAD0), 0, 0); ok || err == nil {
		t.Fatalf("GetMessage(invalid HWND) = %v, %v; want the -1 error", ok, err)
	}
	if err := PostMessage(windows.HWND(0xDEAD0), WM_APP, 0, 0); !errors.Is(err, windows.ERROR_INVALID_WINDOW_HANDLE) {
		t.Errorf("PostMessage(invalid HWND) = %v", err)
	}
}

// wndprocLog is what testWndProc saw; the test runs it on one locked
// thread only.
var wndprocLog struct {
	createParams uintptr
	createCx     int32
	createCy     int32
	createStyle  int32
	calcSize     int // WM_NCCALCSIZE with wParam TRUE
	calcLppos    bool
	minMax       int
	minTrack     Point
}

var testWndProc = ffi.RegisterCallback("w32test.wndproc", func(h windows.HWND, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case WM_NCCREATE:
		cs := ffi.At[CreateStruct](lp)
		wndprocLog.createParams = cs.LpCreateParams
		wndprocLog.createCx, wndprocLog.createCy = cs.Cx, cs.Cy
		wndprocLog.createStyle = cs.Style
	case WM_NCCALCSIZE:
		if wp != 0 {
			p := ffi.At[NCCalcSizeParams](lp)
			wndprocLog.calcSize++
			if p.Lppos != 0 && ffi.At[WindowPos](p.Lppos).Hwnd == h {
				wndprocLog.calcLppos = true
			}
		}
	case WM_GETMINMAXINFO:
		mmi := ffi.At[MinMaxInfo](lp)
		wndprocLog.minMax++
		mmi.PtMinTrackSize = Point{321, 123}
	}
	return DefWindowProc(h, msg, wp, lp)
})

// TestLParamStructs reads the lParam structs through ffi.At in a real
// window procedure: CREATESTRUCTW in WM_NCCREATE (with lpCreateParams
// carrying a handle), NCCALCSIZE_PARAMS with its WINDOWPOS, and a written
// MINMAXINFO that Windows then applies.
func TestLParamStructs(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	inst := newTestClass(t, "gogui.w32test.lparam", testWndProc)
	const handle = 0x0000_00AB_0000_0042 // shaped like an ffi.Handle
	style := uint32(WS_OVERLAPPEDWINDOW)
	h, err := CreateWindowEx(0, "gogui.w32test.lparam", "w32 test", style,
		10, 20, 400, 300, 0, 0, inst, handle)
	if err != nil {
		t.Fatal(err)
	}
	defer DestroyWindow(h)

	if wndprocLog.createParams != handle || wndprocLog.createCx != 400 || wndprocLog.createCy != 300 ||
		uint32(wndprocLog.createStyle) != style {
		t.Errorf("WM_NCCREATE saw %+v; want params %#x, 400×300, style %#x", wndprocLog, handle, style)
	}
	if err := SetWindowPos(h, 0, 0, 0, 100, 50, SWP_NOMOVE|SWP_NOZORDER|SWP_NOACTIVATE|SWP_FRAMECHANGED); err != nil {
		t.Fatal(err)
	}
	if wndprocLog.calcSize == 0 || !wndprocLog.calcLppos {
		t.Errorf("WM_NCCALCSIZE(TRUE) count %d, lppos ok %v", wndprocLog.calcSize, wndprocLog.calcLppos)
	}
	if wndprocLog.minMax == 0 {
		t.Fatal("no WM_GETMINMAXINFO")
	}
	rc, err := GetWindowRect(h)
	if err != nil {
		t.Fatal(err)
	}
	if w, hh := rc.Right-rc.Left, rc.Bottom-rc.Top; w != 321 || hh != 123 {
		t.Errorf("window is %d×%d after SetWindowPos(100×50); want the 321×123 minimum written into MINMAXINFO", w, hh)
	}
	if IsZoomed(h) || IsIconic(h) {
		t.Error("hidden window reported zoomed or iconic")
	}
	if dpi := GetDpiForWindow(h); dpi == 0 {
		t.Error("GetDpiForWindow = 0")
	}
	wp, err := GetWindowPlacement(h)
	if err != nil || wp.Length != 44 {
		t.Errorf("GetWindowPlacement = %+v, %v", wp, err)
	}
	if err := SetWindowText(h, "renamed"); err != nil {
		t.Error(err)
	}
}

// TestRegDWORDAndDpiContext reads a registry value that exists on every
// Windows 10 1809+ user profile, a missing one, and compares DPI contexts.
func TestRegDWORDAndDpiContext(t *testing.T) {
	const personalize = `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`
	if v, err := RegDWORD(personalize, "AppsUseLightTheme"); err != nil {
		t.Logf("AppsUseLightTheme: %v (allowed: a fresh profile may lack it)", err)
	} else if v > 1 {
		t.Errorf("AppsUseLightTheme = %d, want 0 or 1", v)
	}
	if _, err := RegDWORD(personalize, "gogui-does-not-exist"); !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		t.Errorf("missing value: %v, want ERROR_FILE_NOT_FOUND", err)
	}
	if _, err := RegDWORD(`Software\gogui-does-not-exist`, "x"); !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		t.Errorf("missing key: %v, want ERROR_FILE_NOT_FOUND", err)
	}
	if !AreDpiAwarenessContextsEqual(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2, DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2) ||
		AreDpiAwarenessContextsEqual(DPI_AWARENESS_CONTEXT_UNAWARE, DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2) {
		t.Error("AreDpiAwarenessContextsEqual gives wrong answers")
	}
	if GetThreadDpiAwarenessContext() == 0 {
		t.Error("GetThreadDpiAwarenessContext = 0")
	}
	// SetTimer/KillTimer on a thread timer-less window: KillTimer of an
	// unknown ID fails.
	if err := KillTimer(0, 12345); err == nil {
		t.Error("KillTimer(0, unknown) succeeded")
	}
}

// TestFrameClockCalls (S2.4f): DWM's timing read from the packed
// DWM_TIMING_INFO (a refresh period of 30–500 Hz and a vblank within a
// few periods of now: DWM reports the next one, after now on the QPC timeline), a high-resolution waitable
// timer signaled no earlier than set, and MsgWaitForMultipleObjectsEx
// returning the handle's index, WAIT_TIMEOUT and the queue's input.
func TestFrameClockCalls(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var m Msg
	for PeekMessage(&m, 0, 0, 0, PM_REMOVE) { // what earlier tests left on this thread
	}
	freq := QueryPerformanceFrequency()
	if freq <= 0 {
		t.Fatalf("QueryPerformanceFrequency = %d", freq)
	}
	a := QueryPerformanceCounter()
	if b := QueryPerformanceCounter(); b < a {
		t.Errorf("QueryPerformanceCounter went back: %d, %d", a, b)
	}
	ti, err := DwmGetCompositionTimingInfo()
	now := QueryPerformanceCounter()
	if err != nil {
		t.Logf("DwmGetCompositionTimingInfo: %v (allowed: no composition)", err)
	} else {
		hz := float64(freq) / float64(ti.RefreshPeriod)
		if hz < 30 || hz > 500 {
			t.Errorf("refresh period %d ticks (%.1f Hz)", ti.RefreshPeriod, hz)
		}
		if d := now - int64(ti.VBlank); d < -4*int64(ti.RefreshPeriod) || d > 4*int64(ti.RefreshPeriod) {
			t.Errorf("vblank %d, now %d: %d ticks apart", ti.VBlank, now, d)
		}
		t.Logf("%.3f Hz, vblank %d ticks before now", hz, now-int64(ti.VBlank))
	}

	timer, err := CreateWaitableTimer(true)
	if err != nil {
		t.Logf("high-resolution timer: %v; trying a standard one", err)
		if timer, err = CreateWaitableTimer(false); err != nil {
			t.Fatal(err)
		}
	}
	defer windows.CloseHandle(timer)
	other, err := CreateWaitableTimer(false)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(other)
	hs := []windows.Handle{other, timer}
	if r, err := MsgWaitForMultipleObjectsEx(hs, 0, QS_ALLINPUT, MWMO_INPUTAVAILABLE); err != nil || r != WAIT_TIMEOUT {
		t.Errorf("unset timers: %#x, %v; want WAIT_TIMEOUT", r, err)
	}
	start := QueryPerformanceCounter()
	if err := SetWaitableTimer(timer, 30_000); err != nil { // 3 ms
		t.Fatal(err)
	}
	r, err := MsgWaitForMultipleObjectsEx(hs, 1000, QS_ALLINPUT, MWMO_INPUTAVAILABLE)
	el := float64(QueryPerformanceCounter()-start) / float64(freq) * 1000
	if err != nil || r != WAIT_OBJECT_0+1 || el < 2.5 || el > 100 {
		t.Errorf("timer of 3 ms: %#x, %v after %.2f ms; want WAIT_OBJECT_0+1", r, err, el)
	}
	t.Logf("3 ms timer signaled after %.2f ms", el)
	if err := SetWaitableTimer(timer, 10_000_000); err != nil {
		t.Fatal(err)
	}
	if err := CancelWaitableTimer(timer); err != nil {
		t.Fatal(err)
	}
	if r, _ := MsgWaitForMultipleObjectsEx(hs, 20, QS_ALLINPUT, MWMO_INPUTAVAILABLE); r != WAIT_TIMEOUT {
		t.Errorf("cancelled timer: %#x, want WAIT_TIMEOUT", r)
	}
	if _, err := MsgWaitForMultipleObjectsEx([]windows.Handle{0}, 0, QS_ALLINPUT, 0); err == nil {
		t.Error("waiting on handle 0 succeeded")
	}

	// Input in the queue (a posted thread message) ends the wait with
	// index len(handles); with no handles that is WAIT_OBJECT_0.
	tid := windows.GetCurrentThreadId()
	post := user32.NewProc("PostThreadMessageW") // test-only, not in allProcs
	if r, _, err := post.Call(uintptr(tid), WM_APP+7, 0, 0); r == 0 {
		t.Fatal(err)
	}
	if r, err := MsgWaitForMultipleObjectsEx(hs, 1000, QS_ALLINPUT, MWMO_INPUTAVAILABLE); err != nil || r != WAIT_OBJECT_0+2 {
		t.Errorf("posted message with two handles: %#x, %v; want WAIT_OBJECT_0+2", r, err)
	}
	if r, err := MsgWaitForMultipleObjectsEx(nil, 1000, QS_ALLINPUT, MWMO_INPUTAVAILABLE); err != nil || r != WAIT_OBJECT_0 {
		t.Errorf("posted message, no handles: %#x, %v; want WAIT_OBJECT_0", r, err)
	}
	if !PeekMessage(&m, 0, WM_APP+7, WM_APP+7, PM_REMOVE) {
		t.Error("the posted message is gone")
	}
}
