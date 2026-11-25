//go:build windows

package win32

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/win32/d2d"
)

// cachePaint draws one of each cached resource: a solid brush, a gradient
// (stop collection and brush) and a dashed stroke style.
func cachePaint(c *canvas) {
	c.FillRoundedRect(rect(0, 0, 8, 8), render.Radii{}, red)
	c.FillRoundedRect(rect(8, 0, 32, 8), render.Radii{}, cacheGrad)
	var line render.Path
	line.MoveTo(render.Point{X: 0, Y: 20}).LineTo(render.Point{X: 40, Y: 20})
	c.StrokePath(&line, 2, black, render.StrokeStyle{Dash: []float64{4, 2}})
}

var cacheGrad = render.LinearGradient(render.Point{X: 8}, render.Point{X: 40},
	render.GradientStop{Offset: 0, Color: driver.RGB8(255, 0, 0)},
	render.GradientStop{Offset: 1, Color: driver.RGB8(0, 0, 255)})

// TestCanvasResourcesAcrossFrames: the device keeps brushes, stop
// collections and stroke styles across frames, and one canvas (review P3):
// a second frame of the same scene creates none of them, and the cache
// hits allocate nothing.
func TestCanvasResourcesAcrossFrames(t *testing.T) {
	e := newCanvasEnv(t)
	_, c, _ := e.frame(1, false, cachePaint)
	c1 := c // the pointer: the device's one canvas
	made := e.g.res.made
	if made != 5 {
		t.Errorf("first frame made %d resources, want 5 (2 solid brushes, stops, gradient, stroke style)", made)
	}
	s, c2, _ := e.frame(1, false, func(c *canvas) {
		cachePaint(c)
		grad, dashed := cacheGrad.Normalized(), render.StrokeStyle{Dash: []float64{4, 2}}.Normalized()
		col := driver.RGB8(255, 0, 0)
		if n := testing.AllocsPerRun(20, func() {
			c.brush(grad)
			c.solid(col)
			c.strokeStyle(dashed, 2)
		}); n != 0 {
			t.Errorf("cache hits: %v allocations, want 0", n)
		}
	})
	if n := e.g.res.made - made; n != 0 {
		t.Errorf("second frame made %d resources, want 0", n)
	}
	if c1 != c2 {
		t.Error("a new canvas for the second frame, want the device's one")
	}
	s.is("solid", cRed, 1, 4, 4)
	s.is("dash", cBlack, 1, 2, 20)
	s.is("gap", cWhite, 1, 5, 20)
}

// TestCanvasResourceBounds: an entry unused for resKeep paints goes, and
// beyond resMax entries of a kind the least recently used go; a gradient's
// stop collection goes with the last gradient brush over it. The env's
// cleanup checks that every one is released exactly once.
func TestCanvasResourceBounds(t *testing.T) {
	e := newCanvasEnv(t)
	e.frame(1, false, cachePaint)
	solids, stops, grads, strokes := e.g.res.len()
	if solids != 2 || stops != 1 || grads != 1 || strokes != 1 {
		t.Fatalf("cached %d solids, %d stops, %d gradients, %d strokes; want 2, 1, 1, 1", solids, stops, grads, strokes)
	}
	e.g.res.paint += resKeep
	e.frame(1, false, func(c *canvas) {
		c.FillRoundedRect(rect(0, 0, 8, 8), render.Radii{}, green)
	})
	if solids, stops, grads, strokes = e.g.res.len(); solids != 1 || stops != 0 || grads != 0 || strokes != 0 {
		t.Errorf("after %d idle paints: %d solids, %d stops, %d gradients, %d strokes; want 1, 0, 0, 0",
			resKeep, solids, stops, grads, strokes)
	}
	e.frame(1, false, func(c *canvas) {
		for i := range resMax + 20 {
			c.FillRoundedRect(rect(0, 0, 8, 8), render.Radii{}, render.Solid(driver.RGBA(float32(i)/1024, 0, 0, 1)))
		}
	})
	if solids, _, _, _ = e.g.res.len(); solids != resMax {
		t.Errorf("%d solid brushes cached, want the bound %d", solids, resMax)
	}
}

// TestCanvasTransformCalls: the canvas sends the transform to D2D only
// when a call that draws or pushes needs another one than D2D has (review
// P4): transforms set and restored without drawing in between cost no
// SetTransform.
func TestCanvasTransformCalls(t *testing.T) {
	e := newCanvasEnv(t)
	var sets int
	_, c, _ := e.frame(1, false, func(c *canvas) {
		base := c.xfSets // newCanvas's identity
		for range 10 {
			c.Save()
			c.Translate(4, 4)
			c.Scale(2, 2)
			c.Restore()
		}
		if n := c.xfSets - base; n != 0 {
			t.Errorf("%d SetTransforms for transforms nothing drew under, want 0", n)
		}
		c.Save()
		c.Translate(4, 4)
		c.Translate(-4, -4) // back to the identity D2D has
		c.FillRoundedRect(rect(0, 0, 8, 8), render.Radii{}, red)
		if n := c.xfSets - base; n != 0 {
			t.Errorf("%d SetTransforms for a fill under the identity, want 0", n)
		}
		c.Translate(16, 0)
		c.FillRoundedRect(rect(0, 0, 8, 8), render.Radii{}, red)
		c.FillRoundedRect(rect(0, 8, 8, 8), render.Radii{}, red)
		c.Restore()
		sets = c.xfSets - base
	})
	if sets != 1 {
		t.Errorf("%d SetTransforms for two fills under one translation, want 1", sets)
	}
	if m := c.sent; m != d2d.IdentityMatrix() {
		t.Errorf("D2D left at %+v after finish, want the identity", m)
	}
}
