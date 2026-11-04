package ffi

import "syscall"

// nativeCallback creates a stdcall callback (the only convention on amd64
// and arm64) with syscall.NewCallback. fn's signature has been checked.
func nativeCallback(fn any) uintptr {
	return syscall.NewCallback(fn)
}
