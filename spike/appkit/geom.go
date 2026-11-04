//go:build darwin

package main

import "structs"

// Go mirrors of the Foundation geometry structs. CGFloat is a double on
// every 64-bit Mac, so all three are plain float64 aggregates: NSPoint and
// NSSize are 16 bytes, NSRect 32 (geom_test.go checks the layouts).
//
// purego passes and returns them by value through reflection. NSRect is
// the interesting case: on arm64 it is a homogeneous float aggregate in
// d0–d3, on amd64 it is returned in memory (objc_msgSend_stret).

// NSPoint mirrors NSPoint (CGPoint).
type NSPoint struct {
	_    structs.HostLayout
	X, Y float64
}

// NSSize mirrors NSSize (CGSize).
type NSSize struct {
	_             structs.HostLayout
	Width, Height float64
}

// NSRect mirrors NSRect (CGRect).
type NSRect struct {
	_      structs.HostLayout
	Origin NSPoint
	Size   NSSize
}

func pt(x, y float64) NSPoint { return NSPoint{X: x, Y: y} }

func rect(x, y, w, h float64) NSRect {
	return NSRect{Origin: NSPoint{X: x, Y: y}, Size: NSSize{Width: w, Height: h}}
}
