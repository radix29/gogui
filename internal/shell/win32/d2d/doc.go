//go:build windows

// Package d2d holds the Direct2D bindings of the Windows shell (PLAN.md
// S2.4c_1): D2D1CreateFactory (ID2D1Factory1, single-threaded),
// ID2D1Factory1::CreateDevice over the shell's IDXGIDevice,
// ID2D1Device::CreateDeviceContext, the device context's target methods
// (CreateBitmapFromDxgiSurface, SetTarget, BeginDraw, EndDraw, SetDpi,
// GetDpi, Clear) and ID2D1Bitmap1, with their constants and mirrored
// structs.
//
// S2.4d_1 adds the drawing bindings (draw.go): SetTransform/Transform,
// PushAxisAlignedClip/PopAxisAlignedClip, PushLayer (the
// D2D1_LAYER_PARAMETERS1 overload with a NULL layer)/PopLayer, solid and
// linear-gradient brushes with their stop collections, FillRectangle,
// Fill/DrawRoundedRectangle, Fill/DrawGeometry, and from the factory path
// and rounded-rectangle geometries (with ID2D1GeometrySink) and
// ID2D1StrokeStyle1. Brush and Geometry are base-interface views that
// carry no reference of their own; the concrete types convert to them.
//
// The rules are dxgi's: every interface embeds com.Ptr, returned
// interfaces carry one reference, the slot constants follow the header
// order of d2d1.h and d2d1_1.h and are checked against the full vtables by
// the tests, and everything runs on the UI thread (the factory is
// single-threaded). Only what is used is bound.
//
// S2.4e_1 adds the effect and bitmap bindings (effect.go): CreateBitmap
// from CPU pixels (D2D1_SIZE_U by value, packed into one integer
// register), CreateEffect, DrawImage, DrawBitmap (ID2D1DeviceContext's
// overload), Flush, Bitmap1.CopyFromBitmap, the Image base view, and
// Effect (ID2D1Effect with its ID2D1Properties base: typed SetValue and
// GetValue helpers, SetInput, SetInputCount, InputCount, Output) for the
// shadow and Gaussian-blur effects. CopyFromBitmap from the context's own
// target inside BeginDraw reads the GPU's pixels, not the batch, so the
// caller flushes first. S2.4e_3 adds CLSID_D2D1Crop and its properties for
// the backdrop blur.
//
// S2.5f adds DrawTextLayout (an IDWriteTextLayout pointer: d2d does not
// import dwrite; the origin D2D1_POINT_2F by value on the float path) and
// Set/GetTextAntialiasMode, with D2D1_TEXT_ANTIALIAS_MODE and
// D2D1_DRAW_TEXT_OPTIONS.
//
// Float path (com.Bind, purego): SetDpi (two FLOATs), DrawRoundedRectangle,
// DrawGeometry (a FLOAT width), DrawBitmap (a FLOAT opacity), and
// DrawTextLayout and the sink's
// BeginFigure and AddLine (D2D1_POINT_2F by value: one integer register on
// amd64, an HFA in s0/s1 on arm64). Each keeps its bound func per code
// address (com.Method), so a call per segment doesn't go through Bind's
// lock. Every other bound method takes integers and pointers only and goes
// through com.Ptr.Call, structs by pointer included (and CreateBitmap's
// D2D1_SIZE_U packed into one integer). D2D methods that return a struct
// (GetSize, GetPixelSize, GetPixelFormat) come back through a hidden
// pointer in MSVC's member-function ABI; none is bound.
//
// Enum traps the canvas maps rather than casts: CAP_STYLE is FLAT, SQUARE,
// ROUND (render: Butt, Round, Square); render's miter join is
// LINE_JOIN_MITER_OR_BEVEL; custom dashes and the dash offset are in units
// of the stroke width.
package d2d
