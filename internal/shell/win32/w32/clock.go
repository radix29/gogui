//go:build windows

package w32

import (
	"encoding/binary"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ---- the paced frame clock (S2.4f) ----

// MsgWaitForMultipleObjectsEx waits until one of handles is signaled, the
// thread's queue has input of wakeMask (QS_*), or ms milliseconds have
// passed (INFINITE: no limit). It returns WAIT_OBJECT_0+i for handles[i],
// WAIT_OBJECT_0+len(handles) for input and WAIT_TIMEOUT; it dispatches
// nothing. At most MAXIMUM_WAIT_OBJECTS-1 handles.
func MsgWaitForMultipleObjectsEx(handles []windows.Handle, ms, wakeMask, flags uint32) (uint32, error) {
	var p uintptr
	if len(handles) > 0 {
		p = uintptr(unsafe.Pointer(&handles[0]))
	}
	// callV with a pointer: the wait dispatches nothing, so no Go code runs
	// during it to move the stack, and KeepAlive holds handles.
	r, _, err := callV(procMsgWaitForMultipleObjectsEx, uintptr(len(handles)), p, uintptr(ms),
		uintptr(wakeMask), uintptr(flags))
	runtime.KeepAlive(handles)
	if uint32(r) == WAIT_FAILED {
		return WAIT_FAILED, lastError("MsgWaitForMultipleObjectsEx", err)
	}
	return uint32(r), nil
}

// CreateWaitableTimer creates an unnamed auto-reset waitable timer, a
// high-resolution one (Windows 10 1803 and later) if highRes is set. Close
// it with windows.CloseHandle.
func CreateWaitableTimer(highRes bool) (windows.Handle, error) {
	var flags uint32
	if highRes {
		flags = CREATE_WAITABLE_TIMER_HIGH_RESOLUTION
	}
	r, _, err := callV(procCreateWaitableTimerExW, 0, 0, uintptr(flags), TIMER_ALL_ACCESS)
	if r == 0 {
		return 0, lastError("CreateWaitableTimerExW", err)
	}
	return windows.Handle(r), nil
}

// SetWaitableTimer arms timer h to be signaled once, after100ns units of
// 100 ns from now (at least 1).
func SetWaitableTimer(h windows.Handle, after100ns int64) error {
	due := -max(after100ns, 1) // negative: relative
	r, _, err := procSetWaitableTimer.Call(uintptr(h), uintptr(unsafe.Pointer(&due)), 0, 0, 0, 0)
	if r == 0 {
		return lastError("SetWaitableTimer", err)
	}
	return nil
}

// CancelWaitableTimer disarms timer h (its signaled state is unchanged).
func CancelWaitableTimer(h windows.Handle) error {
	r, _, err := callV(procCancelWaitableTimer, uintptr(h))
	if r == 0 {
		return lastError("CancelWaitableTimer", err)
	}
	return nil
}

// QueryPerformanceCounter returns the performance counter, the timeline
// of DWM's timing (QueryPerformanceFrequency ticks per second).
func QueryPerformanceCounter() int64 {
	var v int64
	// SyscallN with the conversion in the call: v stays on the stack (the
	// clock reads it on every idle step; review P17), which is safe because
	// QueryPerformanceCounter runs no Go code that could move the stack.
	syscall.SyscallN(procQueryPerformanceCounter.Addr(), uintptr(unsafe.Pointer(&v)))
	return v
}

// QueryPerformanceFrequency returns the performance counter's ticks per
// second (fixed at boot).
func QueryPerformanceFrequency() int64 {
	var v int64
	procQueryPerformanceFrequency.Call(uintptr(unsafe.Pointer(&v)))
	return v
}

// CompositionTiming is the part of DWM_TIMING_INFO the frame clock uses,
// in performance-counter ticks.
type CompositionTiming struct {
	RefreshPeriod uint64 // qpcRefreshPeriod: the monitor's refresh period
	VBlank        uint64 // qpcVBlank: the time of a recent vblank
}

// dwmTimingInfoSize is sizeof(DWM_TIMING_INFO). dwmapi.h declares it under
// #pragma pack(1), so its 64-bit fields are unaligned and it is read from
// bytes rather than mirrored as a Go struct.
const dwmTimingInfoSize = 292

// Offsets in DWM_TIMING_INFO (packed): cbSize 0, rateRefresh 4,
// qpcRefreshPeriod 12, rateCompose 20, qpcVBlank 28.
const (
	dwmTimingRefreshPeriod = 12
	dwmTimingVBlank        = 28
)

// DwmGetCompositionTimingInfo returns DWM's composition timing for the
// desktop (hwnd NULL, as Windows 8.1 and later require).
func DwmGetCompositionTimingInfo() (CompositionTiming, error) {
	var buf [dwmTimingInfoSize]byte
	binary.LittleEndian.PutUint32(buf[0:], dwmTimingInfoSize)
	r, _, _ := procDwmGetCompositionTimingInfo.Call(0, uintptr(unsafe.Pointer(&buf[0])))
	if err := hresult("DwmGetCompositionTimingInfo", r); err != nil {
		return CompositionTiming{}, err
	}
	return CompositionTiming{
		RefreshPeriod: binary.LittleEndian.Uint64(buf[dwmTimingRefreshPeriod:]),
		VBlank:        binary.LittleEndian.Uint64(buf[dwmTimingVBlank:]),
	}, nil
}
