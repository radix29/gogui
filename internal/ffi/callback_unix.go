//go:build darwin || linux

package ffi

import "github.com/ebitengine/purego"

// nativeCallback creates a C-ABI callback with purego.NewCallback. fn's
// signature has been checked, so purego's own panics are not expected.
func nativeCallback(fn any) uintptr {
	return purego.NewCallback(fn)
}
