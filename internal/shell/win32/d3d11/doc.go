//go:build windows

// Package d3d11 holds the Direct3D 11 bindings of the Windows shell
// (PLAN.md S2.4b): D3D11CreateDevice with the hardware-then-WARP fallback,
// the device and immediate-context methods the shell uses (textures,
// render-target views, clears, copies, Map/Unmap for the tests' readback,
// Flush), the constants and the mirrored structs.
//
// The rules are dxgi's: every interface embeds com.Ptr, returned
// interfaces carry one reference, the slot constants follow the header
// order of d3d11.h and are checked by slots_test.go, and everything runs on
// the UI thread. ClearRenderTargetView takes its color as a pointer to four
// FLOATs, so it too is an integer-only call; no method here needs the
// float path. The procs are declared through newProc, so TestProcsResolve
// finds every one.
//
// The shell draws on the device only through D2D (S2.4c); D3D11 itself
// only clears and copies, in the tests. The device is created without
// CREATE_DEVICE_SINGLETHREADED, because DComp and D2D may touch it from
// threads of their own.
package d3d11
