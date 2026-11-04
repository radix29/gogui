//go:build windows

package com

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// COINIT flags of CoInitializeEx.
const (
	COINIT_APARTMENTTHREADED = 0x2
	COINIT_DISABLE_OLE1DDE   = 0x4
)

// InitUIThread makes the calling thread a single-threaded COM apartment
// (CoInitializeEx with COINIT_APARTMENTTHREADED | COINIT_DISABLE_OLE1DDE),
// as the UI thread needs for the file dialogs, drag and drop and UI
// Automation later on. The caller must have locked its goroutine to the
// thread; win32's Run calls it right after LockOSThread.
//
// On success (S_OK, or S_FALSE when the thread was already an STA) it
// returns the matching CoUninitialize, to be called on the same thread
// before the thread is unlocked. A thread already initialized as a
// multithreaded apartment fails with RPC_E_CHANGED_MODE and needs no undo.
func InitUIThread() (uninit func(), err error) {
	hr := coInitializeEx(COINIT_APARTMENTTHREADED | COINIT_DISABLE_OLE1DDE)
	if err := hr.Err("CoInitializeEx"); err != nil {
		return nil, err
	}
	return windows.CoUninitialize, nil
}

func coInitializeEx(flags uint32) HRESULT {
	err := windows.CoInitializeEx(0, flags)
	if err == nil {
		return S_OK
	}
	if e, ok := err.(windows.Errno); ok {
		return HRESULT(int32(uint32(e)))
	}
	return E_FAIL
}

// CLSCTX_INPROC_SERVER is the class context of CreateInstance: the class's
// DLL is loaded into the process.
const CLSCTX_INPROC_SERVER = 0x1

var (
	ole32                = windows.NewLazySystemDLL("ole32.dll")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
)

// CreateInstance creates an object of the in-process class clsid
// (CoCreateInstance with CLSCTX_INPROC_SERVER) and returns its interface
// iid with one reference. The thread must be a COM apartment
// (InitUIThread); otherwise it fails with CO_E_NOTINITIALIZED.
func CreateInstance(clsid, iid *GUID) (Ptr, error) {
	var out Ptr
	r, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(clsid)), 0, CLSCTX_INPROC_SERVER,
		uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	if err := HRESULT(int32(r)).Err("CoCreateInstance(" + clsid.String() + ")"); err != nil {
		return 0, err
	}
	return out, nil
}
