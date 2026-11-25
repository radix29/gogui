package com

import "fmt"

// HRESULT is a COM status code. Negative values (severity bit set) are
// failures. It implements error, so a failed HRESULT can be returned as is
// or wrapped and found again with errors.As or errors.Is.
type HRESULT int32

// The codes the shell checks for. The device-loss codes of DXGI and D2D live
// here too, so every binding package and the shell share one set of names.
const (
	S_OK    HRESULT = 0
	S_FALSE HRESULT = 1

	E_NOTIMPL               HRESULT = -0x7fffbfff // 0x80004001
	E_NOINTERFACE           HRESULT = -0x7fffbffe // 0x80004002
	E_POINTER               HRESULT = -0x7fffbffd // 0x80004003
	E_ABORT                 HRESULT = -0x7fffbffc // 0x80004004
	E_FAIL                  HRESULT = -0x7fffbffb // 0x80004005
	E_UNEXPECTED            HRESULT = -0x7fff0001 // 0x8000FFFF
	E_ACCESSDENIED          HRESULT = -0x7ff8fffb // 0x80070005
	E_HANDLE                HRESULT = -0x7ff8fffa // 0x80070006
	E_OUTOFMEMORY           HRESULT = -0x7ff8fff2 // 0x8007000E
	E_INVALIDARG            HRESULT = -0x7ff8ffa9 // 0x80070057
	E_NOT_SUFFICIENT_BUFFER HRESULT = -0x7ff8ff86 // 0x8007007A
	RPC_E_CHANGED_MODE      HRESULT = -0x7ffefefa // 0x80010106
	CO_E_NOTINITIALIZED     HRESULT = -0x7ffbfe10 // 0x800401F0
	REGDB_E_CLASSNOTREG     HRESULT = -0x7ffbfeac // 0x80040154

	DXGI_ERROR_INVALID_CALL              HRESULT = -0x7785ffff // 0x887A0001
	DXGI_ERROR_NOT_FOUND                 HRESULT = -0x7785fffe // 0x887A0002
	DXGI_ERROR_DEVICE_REMOVED            HRESULT = -0x7785fffb // 0x887A0005
	DXGI_ERROR_DEVICE_HUNG               HRESULT = -0x7785fffa // 0x887A0006
	DXGI_ERROR_DEVICE_RESET              HRESULT = -0x7785fff9 // 0x887A0007
	DXGI_ERROR_WAS_STILL_DRAWING         HRESULT = -0x7785fff6 // 0x887A000A
	DXGI_ERROR_FRAME_STATISTICS_DISJOINT HRESULT = -0x7785fff5 // 0x887A000B
	D2DERR_RECREATE_TARGET               HRESULT = -0x7766fff4 // 0x8899000C
	DWRITE_E_FILEFORMAT                  HRESULT = -0x7767b000 // 0x88985000
)

var hresultNames = map[HRESULT]string{
	S_OK:                                 "S_OK",
	S_FALSE:                              "S_FALSE",
	E_NOTIMPL:                            "E_NOTIMPL",
	E_NOINTERFACE:                        "E_NOINTERFACE",
	E_POINTER:                            "E_POINTER",
	E_ABORT:                              "E_ABORT",
	E_FAIL:                               "E_FAIL",
	E_UNEXPECTED:                         "E_UNEXPECTED",
	E_ACCESSDENIED:                       "E_ACCESSDENIED",
	E_HANDLE:                             "E_HANDLE",
	E_OUTOFMEMORY:                        "E_OUTOFMEMORY",
	E_INVALIDARG:                         "E_INVALIDARG",
	E_NOT_SUFFICIENT_BUFFER:              "E_NOT_SUFFICIENT_BUFFER",
	RPC_E_CHANGED_MODE:                   "RPC_E_CHANGED_MODE",
	CO_E_NOTINITIALIZED:                  "CO_E_NOTINITIALIZED",
	REGDB_E_CLASSNOTREG:                  "REGDB_E_CLASSNOTREG",
	DXGI_ERROR_INVALID_CALL:              "DXGI_ERROR_INVALID_CALL",
	DXGI_ERROR_NOT_FOUND:                 "DXGI_ERROR_NOT_FOUND",
	DXGI_ERROR_DEVICE_REMOVED:            "DXGI_ERROR_DEVICE_REMOVED",
	DXGI_ERROR_DEVICE_HUNG:               "DXGI_ERROR_DEVICE_HUNG",
	DXGI_ERROR_DEVICE_RESET:              "DXGI_ERROR_DEVICE_RESET",
	DXGI_ERROR_WAS_STILL_DRAWING:         "DXGI_ERROR_WAS_STILL_DRAWING",
	DXGI_ERROR_FRAME_STATISTICS_DISJOINT: "DXGI_ERROR_FRAME_STATISTICS_DISJOINT",
	D2DERR_RECREATE_TARGET:               "D2DERR_RECREATE_TARGET",
	DWRITE_E_FILEFORMAT:                  "DWRITE_E_FILEFORMAT",
}

// Failed reports whether hr is a failure (FAILED in the SDK).
func (hr HRESULT) Failed() bool { return hr < 0 }

// Succeeded reports whether hr is a success code (SUCCEEDED), S_FALSE included.
func (hr HRESULT) Succeeded() bool { return hr >= 0 }

// Facility returns the facility field (HRESULT_FACILITY): 7 for Win32
// errors, 0x87A for DXGI, 0x899 for D2D.
func (hr HRESULT) Facility() uint16 { return uint16(uint32(hr)>>16) & 0x1fff }

// Code returns the low 16 bits (HRESULT_CODE); for a FACILITY_WIN32
// HRESULT that is the Win32 error code.
func (hr HRESULT) Code() uint16 { return uint16(uint32(hr)) }

// Error formats hr as "E_NOINTERFACE (0x80004002)" for a known code and as
// "HRESULT 0x887A0099 (facility 0x87A, code 153)" otherwise.
func (hr HRESULT) Error() string {
	if name, ok := hresultNames[hr]; ok {
		return fmt.Sprintf("%s (0x%08X)", name, uint32(hr))
	}
	return fmt.Sprintf("HRESULT 0x%08X (facility 0x%X, code %d)", uint32(hr), hr.Facility(), hr.Code())
}

// Err returns nil if hr succeeded, else an error naming op that wraps hr.
func (hr HRESULT) Err(op string) error {
	if hr.Succeeded() {
		return nil
	}
	return &Error{Op: op, HR: hr}
}

// Error is a failed COM call: the method or function and its HRESULT.
type Error struct {
	Op string
	HR HRESULT
}

func (e *Error) Error() string { return "com: " + e.Op + ": " + e.HR.Error() }

// Unwrap returns the HRESULT, so errors.Is(err, com.E_NOINTERFACE) works.
func (e *Error) Unwrap() error { return e.HR }
