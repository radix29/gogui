//go:build darwin

package main

import (
	"testing"

	"github.com/radix29/gogui/internal/ffi"
)

// The expected values are the same on amd64 and arm64: CGFloat is a double
// on both (CoreGraphics CGGeometry.h, CGBase.h).

func TestNSPointLayout(t *testing.T) {
	ffi.CheckLayout(t, NSPoint{}, ffi.Layout{Size: 16, Align: 8, Fields: []ffi.Field{
		{Name: "X", Offset: 0, Size: 8}, {Name: "Y", Offset: 8, Size: 8},
	}})
	ffi.CheckNoGoPointers(t, NSPoint{})
}

func TestNSSizeLayout(t *testing.T) {
	ffi.CheckLayout(t, NSSize{}, ffi.Layout{Size: 16, Align: 8, Fields: []ffi.Field{
		{Name: "Width", Offset: 0, Size: 8}, {Name: "Height", Offset: 8, Size: 8},
	}})
	ffi.CheckNoGoPointers(t, NSSize{})
}

func TestNSRectLayout(t *testing.T) {
	ffi.CheckLayout(t, NSRect{}, ffi.Layout{Size: 32, Align: 8, Fields: []ffi.Field{
		{Name: "Origin", Offset: 0, Size: 16}, {Name: "Size", Offset: 16, Size: 16},
	}})
	ffi.CheckNoGoPointers(t, NSRect{})
}
