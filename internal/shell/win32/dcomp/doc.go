//go:build windows

// Package dcomp holds the DirectComposition bindings of the Windows shell
// (PLAN.md S2.4b): DCompositionCreateDevice, IDCompositionDevice
// (CreateTargetForHwnd, CreateVisual, Commit), IDCompositionTarget::SetRoot
// and IDCompositionVisual::SetContent, which put a composition swap chain
// on a window.
//
// The rules are dxgi's: every interface embeds com.Ptr, returned
// interfaces carry one reference, the slot constants follow dcomp.h and
// are checked by slots_test.go, and everything runs on the UI thread.
// IDCompositionVisual declares overloads (SetOffsetX(IDCompositionAnimation*)
// and SetOffsetX(float), …); MSVC puts each pair in the vtable in reverse
// declaration order, which the vtable table in the tests spells out.
// SetContent, the one visual method used, has no overload. Nothing bound
// takes a float.
//
// The tree is the same for every surface: one target per HWND (not
// topmost), one visual as its root, the surface's swap chain as the
// visual's content. Changes show only after Commit.
package dcomp
