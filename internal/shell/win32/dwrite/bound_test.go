//go:build windows

package dwrite

import "testing"

// TestBound runs the tests of the float-path methods (CreateTextFormat,
// CreateTextLayout, FontSize, MaxWidth/MaxHeight, SetMaxWidth and the hit
// tests) through com.Bind (purego), the path windows/arm64 takes, so it is
// checked on amd64 too (review P2).
func TestBound(t *testing.T) {
	if !directFloats {
		t.Skip("the bound path is the only path here")
	}
	directFloats = false
	defer func() { directFloats = true }()
	for name, test := range map[string]func(*testing.T){
		"Format": TestFormat, "Layout": TestLayout, "Clusters": TestClusters, "HitTest": TestHitTest,
	} {
		t.Run(name, test)
	}
}
