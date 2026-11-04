//go:build windows

// Package dxgi holds the DXGI bindings of the Windows shell (PLAN.md
// S2.4b): the device, adapter and factory interfaces that lead from a
// D3D11 device to its factory, the composition swap chain
// (IDXGIFactory2::CreateSwapChainForComposition, IDXGISwapChain1/2), their
// constants and the mirrored structs.
//
// Every interface is a struct embedding com.Ptr, so it has AddRef, Release
// (which clears it), RefCount and QueryInterface; a func or method that
// returns an interface hands over one reference. The slot constants are
// the methods' vtable indices in SDK header order (dxgi.h, dxgi1_2.h,
// dxgi1_3.h), each checked against the full vtable by slots_test.go. Every
// method bound so far takes integers and pointers only, so all go through
// com.Ptr.Call; none needs the float path.
//
// The factory of a composition swap chain must be the one that created the
// device's adapter, so it comes from DeviceFrom → Adapter → Factory, not
// from CreateDXGIFactory; dxgi.dll therefore has no proc here. Like every
// COM pointer, all objects are used on the UI thread only.
package dxgi
