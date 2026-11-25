package aqua

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
)

// TestPathMemo (review P16): a memo builds its path once per key, in the
// same storage, and a scratch path reuses it, so a Paint that draws them
// allocates nothing once they are built.
func TestPathMemo(t *testing.T) {
	var m pathMemo[checkKey]
	builds := 0
	get := func(k checkKey) *render.Path {
		return m.get(k, func(p *render.Path) { builds++; checkMark(p, k.box, k.mixed) })
	}
	k := checkKey{box: driver.Rect{W: 14, H: 14}}
	p := get(k)
	if get(k) != p || builds != 1 || p.Len() != 3 {
		t.Fatalf("same key: %d builds, %d segments", builds, p.Len())
	}
	k.mixed = true
	if get(k) != p || builds != 2 || p.Len() != 2 {
		t.Fatalf("new key: %d builds, %d segments", builds, p.Len())
	}
	if p.Bounds() != (driver.Rect{X: 3.5, Y: 7, W: 7}) {
		t.Errorf("mixed mark %v", p.Bounds())
	}
	var sp scratchPath
	n := testing.AllocsPerRun(100, func() {
		k.mixed = !k.mixed
		get(k)
		checkMark(sp.scratch(), k.box, false)
	})
	if n != 0 {
		t.Errorf("%v allocations per rebuild", n)
	}
}
